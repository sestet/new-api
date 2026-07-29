package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveUpstreamBillingQuotaUsesSub2APIActualCost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitHttpClient()
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.UpstreamBillingRecord{}).Error)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.EqualValues(t, "/api/v1/usage", r.URL.Path)
		assert.EqualValues(t, "provider-response-id", r.URL.Query().Get("request_id"))
		assert.EqualValues(t, "Bearer account-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[{"request_id":"provider-response-id","total_cost":"0.5","actual_cost":"0.012","rate_multiplier":"3"}]}}`))
	}))
	defer server.Close()

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set(common.UpstreamRequestIdKey, "provider-response-id")
	ctx.Set(common.UpstreamBillingRequestIdKey, "local-request")
	info := &relaycommon.RelayInfo{
		RequestId:   "local-request",
		RelayFormat: types.RelayFormatOpenAI,
		UserGroup:   "standard",
		UsingGroup:  "premium",
		PriceData: types.PriceData{GroupRatioInfo: types.GroupRatioInfo{
			GroupRatio: 2,
			Source:     "group_group_ratio",
		}},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:      0,
			ChannelBaseUrl: server.URL + "/v1",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				UpstreamBilling: &dto.UpstreamBillingSettings{
					Enabled:     true,
					Provider:    dto.UpstreamBillingProviderSub2API,
					AccessToken: "account-token",
				},
			},
		},
	}

	quota := ResolveUpstreamBillingQuota(ctx, info, 1234, nil)

	assert.EqualValues(t, 12000, quota)
	require.NotNil(t, info.UpstreamBillingAudit)
	assert.EqualValues(t, model.UpstreamBillingStatusExact, info.UpstreamBillingAudit.Status)
	assert.EqualValues(t, "0.012", info.UpstreamBillingAudit.UpstreamCostUSD)
	var record model.UpstreamBillingRecord
	require.NoError(t, model.DB.Where("local_request_id = ?", "local-request").First(&record).Error)
	assert.EqualValues(t, 12000, record.ChargedQuota)
	assert.EqualValues(t, 1234, record.EstimatedQuota)
	assert.EqualValues(t, "sub2api", record.Provider)
	assert.Equal(t, "standard", record.UserGroup)
	assert.Equal(t, "premium", record.UsingGroup)
	assert.Equal(t, "2", record.GroupRatio)
	assert.Equal(t, "group_group_ratio", record.GroupRatioSource)
	assert.Equal(t, "500000", record.QuotaPerUnit)
	assert.Positive(t, record.RequestStartedAtMs)
	assert.GreaterOrEqual(t, record.RequestFinishedAtMs, record.RequestStartedAtMs)
	assert.True(t, record.AdjustmentApplied)
	assert.True(t, record.LogUpdated)
	assert.Greater(t, record.NextRecheckAt, common.GetTimestamp())
}

func TestResolveUpstreamBillingQuotaContinuesAfterRequestContextCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitHttpClient()
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.UpstreamBillingRecord{}).Error)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[{"request_id":"stream-request","actual_cost":"0.0061"}]}}`))
	}))
	defer server.Close()

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	requestContext, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestContext)
	ctx.Set(common.UpstreamRequestIdKey, "stream-request")
	info := &relaycommon.RelayInfo{
		RequestId:   "stream-local-request",
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:      0,
			ChannelBaseUrl: server.URL,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				UpstreamBilling: &dto.UpstreamBillingSettings{
					Enabled:     true,
					Provider:    dto.UpstreamBillingProviderSub2API,
					AccessToken: "token",
				},
			},
		},
	}

	quota := ResolveUpstreamBillingQuota(ctx, info, 12200, nil)

	assert.EqualValues(t, 3050, quota)
	require.NotNil(t, info.UpstreamBillingAudit)
	assert.EqualValues(t, model.UpstreamBillingStatusExact, info.UpstreamBillingAudit.Status)
}

func TestLookupNewAPIBillingConvertsUpstreamQuotaToUSD(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.EqualValues(t, "42", r.Header.Get("New-Api-User"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/log/self":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"request_id":"upstream-id","quota":250000}]}}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := lookupNewAPIBilling(context.Background(), nil, server.URL, "token", 42, "upstream-id")

	require.NoError(t, err)
	assert.EqualValues(t, dto.UpstreamBillingProviderNewAPI, result.Provider)
	assert.EqualValues(t, "0.5", result.CostUSD.String())
	assert.EqualValues(t, 250000, result.UpstreamQuota)
}

func TestLookupNewAPIBillingFallsBackToUniqueUsageSignature(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/log/self":
			if r.URL.Query().Get("request_id") != "" {
				_, _ = w.Write([]byte(`{"success":true,"data":{"items":[]}}`))
				return
			}
			assert.EqualValues(t, "gpt-5.4", r.URL.Query().Get("model_name"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"request_id":"resolved-upstream-id","created_at":1700000001,"model_name":"gpt-5.4","prompt_tokens":578,"completion_tokens":11,"quota":201}]}}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelId:      99,
		ChannelBaseUrl: server.URL,
	}}
	settings := &dto.UpstreamBillingSettings{
		Enabled:     true,
		Provider:    dto.UpstreamBillingProviderNewAPI,
		AccessToken: "token",
		UserID:      42,
	}
	result, attempts, err := lookupUpstreamBilling(context.Background(), info, settings, "local-id", "provider-uuid", &upstreamBillingUsageMatch{
		ModelName:        "gpt-5.4",
		PromptTokens:     578,
		CompletionTokens: 11,
		CreatedAt:        1700000000,
	})

	require.NoError(t, err)
	assert.EqualValues(t, 1, attempts)
	assert.EqualValues(t, "resolved-upstream-id", result.UpstreamRequestID)
	assert.EqualValues(t, 201, result.UpstreamQuota)
	assert.EqualValues(t, "0.000402", result.CostUSD.String())
}

func TestLookupSub2APIBillingFallsBackToUniqueUsageSignature(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.EqualValues(t, "gpt-5.6-luna", r.URL.Query().Get("model"))
		assert.NotEmpty(t, r.URL.Query().Get("start_date"))
		assert.EqualValues(t, r.URL.Query().Get("start_date"), r.URL.Query().Get("end_date"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"request_id":"client:upstream-id","model":"gpt-5.6-luna","input_tokens":91,"cache_read_tokens":100,"output_tokens":44,"actual_cost":"0.00005915","created_at":"2026-07-22T17:41:47.058518+08:00"}]}}`))
	}))
	defer server.Close()

	createdAt, err := time.Parse(time.RFC3339Nano, "2026-07-22T17:41:48+08:00")
	require.NoError(t, err)
	result, err := lookupSub2APIBilling(context.Background(), nil, server.URL, "token", "", "different-request-id", &upstreamBillingUsageMatch{
		ModelName:        "gpt-5.6-luna",
		PromptTokens:     191,
		CompletionTokens: 44,
		CreatedAt:        createdAt.Unix(),
	})

	require.NoError(t, err)
	assert.EqualValues(t, "client:upstream-id", result.UpstreamRequestID)
	assert.EqualValues(t, "0.00005915", result.CostUSD.String())
}

func TestLookupSub2APIBillingUsesRequestCompletionWindow(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"request_id":"client:previous","model":"gpt-image-2","input_tokens":90,"output_tokens":1584,"actual_cost":"0.04","created_at":"2026-07-28T17:57:18+08:00"},{"request_id":"client:current","model":"gpt-image-2","input_tokens":90,"output_tokens":1584,"actual_cost":"0.05","created_at":"2026-07-28T17:57:31+08:00"}]}}`))
	}))
	defer server.Close()

	startedAt, err := time.Parse(time.RFC3339Nano, "2026-07-28T17:57:00+08:00")
	require.NoError(t, err)
	finishedAt, err := time.Parse(time.RFC3339Nano, "2026-07-28T17:57:31+08:00")
	require.NoError(t, err)
	result, err := lookupSub2APIBilling(context.Background(), nil, server.URL, "token", "8157", "response-request-id", &upstreamBillingUsageMatch{
		ModelName:        "gpt-image-2",
		PromptTokens:     90,
		CompletionTokens: 1584,
		StartedAtMs:      startedAt.UnixMilli(),
		FinishedAtMs:     finishedAt.UnixMilli(),
		CredentialID:     2,
		LocalRequestID:   "local-current",
	})

	require.NoError(t, err)
	assert.Equal(t, "client:current", result.UpstreamRequestID)
	assert.Equal(t, "0.05", result.CostUSD.String())
	assert.False(t, result.IdentityAmbiguous)
}

func TestLookupSub2APIBillingFallsBackByTimeForIncompleteStream(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"request_id":"client:aborted-stream","model":"gpt-5.6-sol","input_tokens":1410,"cache_read_tokens":3840,"output_tokens":339,"actual_cost":"0.0024882","created_at":"2026-07-23T21:34:00.756319+08:00"},{"request_id":"client:too-far-away","model":"gpt-5.6-sol","input_tokens":1100,"output_tokens":100,"actual_cost":"0.001","created_at":"2026-07-23T21:33:40+08:00"}]}}`))
	}))
	defer server.Close()

	createdAt, err := time.Parse(time.RFC3339Nano, "2026-07-23T21:34:01+08:00")
	require.NoError(t, err)
	result, err := lookupSub2APIBilling(context.Background(), nil, server.URL, "token", "8046", "local-request-id", &upstreamBillingUsageMatch{
		ModelName:         "gpt-5.6-sol",
		PromptTokens:      1183,
		CompletionTokens:  247,
		CreatedAt:         createdAt.Unix(),
		AllowTimeFallback: true,
		LocalRequestID:    "local-request-id",
	})

	require.NoError(t, err)
	assert.EqualValues(t, "client:aborted-stream", result.UpstreamRequestID)
	assert.EqualValues(t, "0.0024882", result.CostUSD.String())
}

func TestLookupSub2APIBillingRejectsAmbiguousIncompleteStream(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"request_id":"client:first","model":"gpt-5.6-sol","input_tokens":1410,"output_tokens":339,"actual_cost":"0.0024","created_at":"2026-07-23T21:34:00+08:00"},{"request_id":"client:second","model":"gpt-5.6-sol","input_tokens":1100,"output_tokens":100,"actual_cost":"0.001","created_at":"2026-07-23T21:34:03+08:00"}]}}`))
	}))
	defer server.Close()

	createdAt, err := time.Parse(time.RFC3339Nano, "2026-07-23T21:34:01+08:00")
	require.NoError(t, err)
	_, err = lookupSub2APIBilling(context.Background(), nil, server.URL, "token", "8046", "local-request-id", &upstreamBillingUsageMatch{
		ModelName:         "gpt-5.6-sol",
		PromptTokens:      1183,
		CompletionTokens:  247,
		CreatedAt:         createdAt.Unix(),
		AllowTimeFallback: true,
		LocalRequestID:    "local-request-id",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUpstreamBillingRecordNotAvailable)
}

func TestLookupSub2APIBillingExcludesAlreadyClaimedTimeCandidate(t *testing.T) {
	InitHttpClient()
	const credentialID = 73501
	require.NoError(t, model.DB.Where("local_request_id IN ?", []string{"claimed-local", "current-local"}).Delete(&model.UpstreamBillingRecord{}).Error)
	t.Cleanup(func() {
		_ = model.DB.Where("local_request_id IN ?", []string{"claimed-local", "current-local"}).Delete(&model.UpstreamBillingRecord{}).Error
	})
	require.NoError(t, model.CreateUpstreamBillingRecord(&model.UpstreamBillingRecord{
		LocalRequestId:    "claimed-local",
		UpstreamRequestId: "client:claimed",
		CredentialId:      credentialID,
		Status:            model.UpstreamBillingStatusExact,
	}))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"request_id":"client:claimed","model":"gpt-5.6-sol","input_tokens":1410,"output_tokens":339,"actual_cost":"0.0024","created_at":"2026-07-23T21:34:00+08:00"},{"request_id":"client:available","model":"gpt-5.6-sol","input_tokens":1100,"output_tokens":100,"actual_cost":"0.001","created_at":"2026-07-23T21:34:03+08:00"}]}}`))
	}))
	defer server.Close()

	createdAt, err := time.Parse(time.RFC3339Nano, "2026-07-23T21:34:01+08:00")
	require.NoError(t, err)
	result, err := lookupSub2APIBilling(context.Background(), nil, server.URL, "token", "8046", "current-local", &upstreamBillingUsageMatch{
		ModelName:         "gpt-5.6-sol",
		PromptTokens:      1183,
		CompletionTokens:  247,
		CreatedAt:         createdAt.Unix(),
		AllowTimeFallback: true,
		CredentialID:      credentialID,
		LocalRequestID:    "current-local",
	})

	require.NoError(t, err)
	assert.EqualValues(t, "client:available", result.UpstreamRequestID)
}

func TestLookupSub2APIBillingExcludesAlreadyClaimedSignatureCandidate(t *testing.T) {
	InitHttpClient()
	const credentialID = 73502
	require.NoError(t, model.DB.Where("local_request_id IN ?", []string{"claimed-signature", "current-signature"}).Delete(&model.UpstreamBillingRecord{}).Error)
	t.Cleanup(func() {
		_ = model.DB.Where("local_request_id IN ?", []string{"claimed-signature", "current-signature"}).Delete(&model.UpstreamBillingRecord{}).Error
	})
	require.NoError(t, model.CreateUpstreamBillingRecord(&model.UpstreamBillingRecord{
		LocalRequestId:    "claimed-signature",
		UpstreamRequestId: "client:claimed-signature",
		CredentialId:      credentialID,
		Status:            model.UpstreamBillingStatusExact,
	}))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"request_id":"client:claimed-signature","model":"gpt-5.6-sol","input_tokens":100,"output_tokens":20,"actual_cost":"0.002","created_at":"2026-07-23T21:34:00+08:00"},{"request_id":"client:available-signature","model":"gpt-5.6-sol","input_tokens":100,"output_tokens":20,"actual_cost":"0.003","created_at":"2026-07-23T21:34:01+08:00"}]}}`))
	}))
	defer server.Close()

	finishedAt, err := time.Parse(time.RFC3339Nano, "2026-07-23T21:34:01+08:00")
	require.NoError(t, err)
	result, err := lookupSub2APIBilling(context.Background(), nil, server.URL, "token", "8046", "current-signature", &upstreamBillingUsageMatch{
		ModelName:        "gpt-5.6-sol",
		PromptTokens:     100,
		CompletionTokens: 20,
		StartedAtMs:      finishedAt.Add(-time.Second).UnixMilli(),
		FinishedAtMs:     finishedAt.UnixMilli(),
		CredentialID:     credentialID,
		LocalRequestID:   "current-signature",
	})

	require.NoError(t, err)
	assert.Equal(t, "client:available-signature", result.UpstreamRequestID)
	assert.Equal(t, "0.003", result.CostUSD.String())
}

func TestLookupSub2APIBillingAcceptsAmbiguousCandidatesWithEqualCost(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"request_id":"client:first","model":"gpt-image-2","input_tokens":90,"output_tokens":1584,"actual_cost":"0.05","created_at":"2026-07-28T17:57:30+08:00"},{"request_id":"client:second","model":"gpt-image-2","input_tokens":90,"output_tokens":1584,"actual_cost":"0.0500","created_at":"2026-07-28T17:57:31+08:00"}]}}`))
	}))
	defer server.Close()

	finishedAt, err := time.Parse(time.RFC3339Nano, "2026-07-28T17:57:31+08:00")
	require.NoError(t, err)
	result, err := lookupSub2APIBilling(context.Background(), nil, server.URL, "token", "8157", "response-request-id", &upstreamBillingUsageMatch{
		ModelName:        "gpt-image-2",
		PromptTokens:     90,
		CompletionTokens: 1584,
		StartedAtMs:      finishedAt.Add(-30 * time.Second).UnixMilli(),
		FinishedAtMs:     finishedAt.UnixMilli(),
		LocalRequestID:   "local-request",
	})

	require.NoError(t, err)
	assert.Empty(t, result.UpstreamRequestID)
	assert.Equal(t, "0.05", result.CostUSD.String())
	assert.True(t, result.IdentityAmbiguous)
}

func TestLookupSub2APIBillingKeepsEstimateForAmbiguousDifferentCosts(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"request_id":"client:first","model":"gpt-image-2","input_tokens":90,"output_tokens":1584,"actual_cost":"0.04","created_at":"2026-07-28T17:57:30+08:00"},{"request_id":"client:second","model":"gpt-image-2","input_tokens":90,"output_tokens":1584,"actual_cost":"0.05","created_at":"2026-07-28T17:57:31+08:00"}]}}`))
	}))
	defer server.Close()

	finishedAt, err := time.Parse(time.RFC3339Nano, "2026-07-28T17:57:31+08:00")
	require.NoError(t, err)
	_, err = lookupSub2APIBilling(context.Background(), nil, server.URL, "token", "8157", "response-request-id", &upstreamBillingUsageMatch{
		ModelName:        "gpt-image-2",
		PromptTokens:     90,
		CompletionTokens: 1584,
		StartedAtMs:      finishedAt.Add(-30 * time.Second).UnixMilli(),
		FinishedAtMs:     finishedAt.UnixMilli(),
		LocalRequestID:   "local-request",
	})

	require.ErrorIs(t, err, errUpstreamBillingRecordNotAvailable)
	assert.Contains(t, err.Error(), "signature is not unique")
}

func TestLookupSub2APIBillingSearchesAllUsagePages(t *testing.T) {
	InitHttpClient()
	requestedPages := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPages = append(requestedPages, r.URL.Query().Get("page"))
		assert.EqualValues(t, "8046", r.URL.Query().Get("api_key_id"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write([]byte(`{"code":0,"data":{"page":1,"page_size":100,"pages":2,"total":101,"items":[]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"page":2,"page_size":100,"pages":2,"total":101,"items":[{"request_id":"client:older-request","model":"gpt-5.6-sol","input_tokens":891,"cache_read_tokens":3840,"output_tokens":19,"actual_cost":"0.00090285","created_at":"2026-07-23T13:43:20.662907+08:00"}]}}`))
	}))
	defer server.Close()

	createdAt, err := time.Parse(time.RFC3339Nano, "2026-07-23T13:43:20+08:00")
	require.NoError(t, err)
	result, err := lookupSub2APIBilling(context.Background(), nil, server.URL, "token", "8046", "local-request-id", &upstreamBillingUsageMatch{
		ModelName:        "gpt-5.6-sol",
		PromptTokens:     4731,
		CompletionTokens: 19,
		CreatedAt:        createdAt.Unix(),
	})

	require.NoError(t, err)
	assert.EqualValues(t, []string{"1", "2"}, requestedPages)
	assert.EqualValues(t, "client:older-request", result.UpstreamRequestID)
	assert.EqualValues(t, "0.00090285", result.CostUSD.String())
}

func TestLookupSub2APIBillingResolvesChannelAPIKeyID(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"page":1,"page_size":100,"pages":1,"total":1,"items":[{"id":8046,"name":"channel-key","key":"sk-channel"}]}}`))
		case "/api/v1/usage":
			assert.EqualValues(t, "8046", r.URL.Query().Get("api_key_id"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"page":1,"page_size":100,"pages":1,"total":1,"items":[{"request_id":"sub2-request","actual_cost":"0.002"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelId:      0,
		ChannelBaseUrl: server.URL,
		ApiKey:         "sk-channel",
	}}
	settings := &dto.UpstreamBillingSettings{
		Enabled:     true,
		Provider:    dto.UpstreamBillingProviderSub2API,
		AccessToken: "account-token",
	}
	result, _, err := lookupUpstreamBilling(context.Background(), info, settings, "local-request", "sub2-request", nil)

	require.NoError(t, err)
	assert.EqualValues(t, "sub2-request", result.UpstreamRequestID)
	assert.EqualValues(t, "8046", info.ChannelOtherSettings.UpstreamBilling.UpstreamTokenID)
}

func TestSub2APIAccessTokenNeedsRefreshAtQuarterLifetime(t *testing.T) {
	now := int64(1_700_000_000)
	settings := &dto.UpstreamBillingSettings{
		AccessToken:          "access",
		RefreshToken:         "refresh",
		AccessTokenIssuedAt:  now - int64(18*time.Hour/time.Second),
		AccessTokenExpiresAt: now + int64(6*time.Hour/time.Second) - 1,
	}

	assert.True(t, sub2APIAccessTokenNeedsRefresh(settings, now))
	settings.AccessTokenExpiresAt = now + int64(6*time.Hour/time.Second) + 1
	assert.False(t, sub2APIAccessTokenNeedsRefresh(settings, now))
}

func TestLookupSub2APIBillingRefreshesRejectedAccessTokenAndPersistsRotation(t *testing.T) {
	InitHttpClient()
	const channelID = 7399
	require.NoError(t, model.DB.Where("id = ?", channelID).Delete(&model.Channel{}).Error)
	t.Cleanup(func() {
		_ = model.DB.Where("id = ?", channelID).Delete(&model.Channel{}).Error
	})

	refreshCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/refresh":
			refreshCalls++
			var request map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &request))
			assert.EqualValues(t, "old-refresh", request["refresh_token"])
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"new-access","refresh_token":"new-refresh","expires_in":86400,"token_type":"Bearer"}}`))
		case "/api/v1/usage":
			if r.Header.Get("Authorization") != "Bearer new-access" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401,"message":"expired"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"request_id":"sub2-request","actual_cost":"0.002"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	now := common.GetTimestamp()
	channel := &model.Channel{
		Id:      channelID,
		Name:    "sub2-refresh-channel",
		Key:     "sk-test",
		Status:  common.ChannelStatusEnabled,
		BaseURL: common.GetPointer(server.URL),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamBilling: &dto.UpstreamBillingSettings{
		Enabled:              true,
		Provider:             dto.UpstreamBillingProviderSub2API,
		AccessToken:          "old-access",
		RefreshToken:         "old-refresh",
		AccessTokenIssuedAt:  now,
		AccessTokenExpiresAt: now + int64(7*time.Hour/time.Second),
	}})
	require.NoError(t, model.DB.Create(channel).Error)

	settings := channel.GetOtherSettings().UpstreamBilling
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelId:            channel.Id,
		ChannelBaseUrl:       server.URL,
		ChannelOtherSettings: channel.GetOtherSettings(),
	}}
	result, attempts, err := lookupUpstreamBilling(context.Background(), info, settings, "local-request", "sub2-request", nil)

	require.NoError(t, err)
	assert.EqualValues(t, 1, attempts)
	assert.EqualValues(t, "0.002", result.CostUSD.String())
	assert.EqualValues(t, 1, refreshCalls)
	storedChannel, err := model.GetChannelById(channelID, true)
	require.NoError(t, err)
	stored := storedChannel.GetOtherSettings().UpstreamBilling
	require.NotNil(t, stored)
	assert.EqualValues(t, "new-access", stored.AccessToken)
	assert.EqualValues(t, "new-refresh", stored.RefreshToken)
	assert.Greater(t, stored.AccessTokenExpiresAt, now)
}

func TestSharedSub2APIBillingAccountRefreshesOnceForMultipleChannels(t *testing.T) {
	InitHttpClient()
	refreshCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/refresh":
			refreshCalls++
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"access_token":"shared-new-access","refresh_token":"shared-new-refresh","expires_in":86400}}`))
		case "/api/v1/usage":
			if r.Header.Get("Authorization") != "Bearer shared-new-access" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401,"message":"expired"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"request_id":"shared-request","actual_cost":"0.003"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	account := &model.UpstreamBillingAccount{
		Name:         "shared-sub2-account",
		Provider:     string(dto.UpstreamBillingProviderSub2API),
		APIBaseURL:   server.URL,
		AccessToken:  "shared-old-access",
		RefreshToken: "shared-old-refresh",
		Enabled:      true,
	}
	require.NoError(t, model.CreateUpstreamBillingAccount(account))
	t.Cleanup(func() {
		_ = model.DB.Where("id = ?", account.Id).Delete(&model.UpstreamBillingAccount{}).Error
	})

	newChannel := func(name string) *model.Channel {
		channel := &model.Channel{
			Name:    name,
			Key:     "sk-" + name,
			Status:  common.ChannelStatusEnabled,
			BaseURL: common.GetPointer(server.URL),
		}
		channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamBilling: &dto.UpstreamBillingSettings{
			CredentialID: account.Id,
			Enabled:      true,
		}})
		require.NoError(t, model.DB.Create(channel).Error)
		t.Cleanup(func() {
			_ = model.DB.Where("id = ?", channel.Id).Delete(&model.Channel{}).Error
		})
		return channel
	}
	openAIChannel := newChannel("shared-openai")
	claudeChannel := newChannel("shared-claude")

	settings, err := model.ResolveChannelUpstreamBillingSettings(openAIChannel)
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelId:            openAIChannel.Id,
		ChannelBaseUrl:       server.URL,
		ChannelOtherSettings: dto.ChannelOtherSettings{UpstreamBilling: settings},
	}}
	result, attempts, err := lookupUpstreamBilling(context.Background(), info, settings, "local-request", "shared-request", nil)

	require.NoError(t, err)
	assert.EqualValues(t, 1, attempts)
	assert.EqualValues(t, "0.003", result.CostUSD.String())
	assert.EqualValues(t, 1, refreshCalls)
	storedAccount, err := model.GetUpstreamBillingAccountByID(account.Id)
	require.NoError(t, err)
	assert.EqualValues(t, "shared-new-access", storedAccount.AccessToken)
	assert.EqualValues(t, "shared-new-refresh", storedAccount.RefreshToken)
	assert.EqualValues(t, model.UpstreamBillingAccountHealthHealthy, storedAccount.HealthStatus)
	assert.Empty(t, storedAccount.HealthError)
	assert.Greater(t, storedAccount.HealthCheckedAt, int64(0))
	resolvedForClaude, err := model.ResolveChannelUpstreamBillingSettings(claudeChannel)
	require.NoError(t, err)
	assert.EqualValues(t, "shared-new-access", resolvedForClaude.AccessToken)
	assert.EqualValues(t, "shared-new-refresh", resolvedForClaude.RefreshToken)
	assert.Empty(t, claudeChannel.GetOtherSettings().UpstreamBilling.AccessToken)
}

func TestLookupUpstreamBillingRecordsSharedAccountFailure(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid access token"}`))
	}))
	defer server.Close()

	account := &model.UpstreamBillingAccount{
		Name:        "failed-health-account",
		Provider:    string(dto.UpstreamBillingProviderNewAPI),
		APIBaseURL:  server.URL,
		AccessToken: "invalid-token",
		UserID:      42,
		Enabled:     true,
	}
	require.NoError(t, model.CreateUpstreamBillingAccount(account))
	t.Cleanup(func() {
		_ = model.DB.Where("id = ?", account.Id).Delete(&model.UpstreamBillingAccount{}).Error
	})
	settings := account.ToSettings()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl:       server.URL,
		ChannelOtherSettings: dto.ChannelOtherSettings{UpstreamBilling: settings},
	}}

	_, _, err := lookupUpstreamBilling(context.Background(), info, settings, "local-request", "upstream-request", nil)

	require.Error(t, err)
	storedAccount, getErr := model.GetUpstreamBillingAccountByID(account.Id)
	require.NoError(t, getErr)
	assert.Equal(t, model.UpstreamBillingAccountHealthError, storedAccount.HealthStatus)
	assert.Contains(t, storedAccount.HealthError, "401")
	assert.Greater(t, storedAccount.HealthCheckedAt, int64(0))
}

func TestDetectUpstreamBillingProviderUsesStoredAccessToken(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.EqualValues(t, "Bearer stored-token", r.Header.Get("Authorization"))
		assert.EqualValues(t, "/api/v1/usage", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"items":[]}}`))
	}))
	defer server.Close()

	channel := &model.Channel{BaseURL: common.GetPointer(server.URL)}
	channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamBilling: &dto.UpstreamBillingSettings{
		Enabled:     true,
		AccessToken: "stored-token",
	}})
	settings := &dto.UpstreamBillingSettings{
		Enabled:               true,
		Provider:              dto.UpstreamBillingProviderAuto,
		AccessTokenConfigured: true,
	}

	result, err := DetectUpstreamBillingProvider(context.Background(), channel, settings)

	require.NoError(t, err)
	assert.EqualValues(t, dto.UpstreamBillingProviderSub2API, result.Provider)
}

func TestUpstreamBillingProviderCandidatesFallsBackAfterDetectedProvider(t *testing.T) {
	settings := &dto.UpstreamBillingSettings{
		Provider:         dto.UpstreamBillingProviderAuto,
		DetectedProvider: dto.UpstreamBillingProviderNewAPI,
	}

	candidates := upstreamBillingProviderCandidates(0, settings, "local", "upstream")

	assert.EqualValues(t, []dto.UpstreamBillingProvider{
		dto.UpstreamBillingProviderNewAPI,
		dto.UpstreamBillingProviderSub2API,
	}, candidates)
}

func TestResolveUpstreamBillingQuotaClassifiesFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitHttpClient()
	cases := []struct {
		name           string
		statusCode     int
		body           string
		expectedStatus string
	}{
		{
			name:           "record not available uses estimate",
			statusCode:     http.StatusOK,
			body:           `{"data":{"items":[]}}`,
			expectedStatus: model.UpstreamBillingStatusEstimated,
		},
		{
			name:           "API rejection is failed",
			statusCode:     http.StatusUnauthorized,
			body:           `{"message":"unauthorized"}`,
			expectedStatus: model.UpstreamBillingStatusFailed,
		},
	}
	for index, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(testCase.statusCode)
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer server.Close()

			requestID := fmt.Sprintf("fallback-request-%d", index)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				RequestId:   requestID,
				RelayFormat: types.RelayFormatOpenAI,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelId:      0,
					ChannelBaseUrl: server.URL,
					ChannelOtherSettings: dto.ChannelOtherSettings{
						UpstreamBilling: &dto.UpstreamBillingSettings{
							Enabled:     true,
							Provider:    dto.UpstreamBillingProviderSub2API,
							AccessToken: "token",
						},
					},
				},
			}

			quota := ResolveUpstreamBillingQuota(ctx, info, 321, nil)

			assert.EqualValues(t, 321, quota)
			require.NotNil(t, info.UpstreamBillingAudit)
			assert.EqualValues(t, testCase.expectedStatus, info.UpstreamBillingAudit.Status)
		})
	}
}

func TestRunUpstreamBillingReconcileAdjustsWalletAndIsIdempotent(t *testing.T) {
	InitHttpClient()
	require.NoError(t, model.DB.Where("1 = 1").Delete(&model.UpstreamBillingRecord{}).Error)
	require.NoError(t, model.DB.Where("id IN ?", []int{7101, 7102}).Delete(&model.User{}).Error)
	require.NoError(t, model.DB.Where("id IN ?", []int{7201, 7202}).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Where("id = ?", 7301).Delete(&model.Channel{}).Error)
	require.NoError(t, model.LOG_DB.Where("request_id IN ?", []string{"reconcile-more", "reconcile-refund"}).Delete(&model.Log{}).Error)

	moreCost := "0.005"
	var costMu sync.RWMutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.URL.Query().Get("request_id")
		costMu.RLock()
		cost := moreCost
		costMu.RUnlock()
		if requestID == "reconcile-refund" {
			cost = "0.001"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":{"items":[{"request_id":%q,"actual_cost":%q}]}}`, requestID, cost)
	}))
	defer server.Close()

	channel := &model.Channel{
		Id:        7301,
		Name:      "reconcile-channel",
		Key:       "sk-reconcile",
		Status:    common.ChannelStatusEnabled,
		BaseURL:   common.GetPointer(server.URL),
		UsedQuota: 2000,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamBilling: &dto.UpstreamBillingSettings{
		Enabled:     true,
		Provider:    dto.UpstreamBillingProviderSub2API,
		AccessToken: "account-token",
	}})
	require.NoError(t, model.DB.Create(channel).Error)

	cases := []struct {
		requestID      string
		userID         int
		tokenID        int
		groupRatio     string
		expectedQuota  int64
		expectedUser   int64
		expectedRemain int64
		expectedUsed   int64
		expectedDelta  int64
	}{
		{requestID: "reconcile-more", userID: 7101, tokenID: 7201, groupRatio: "2", expectedQuota: 5000, expectedUser: 96000, expectedRemain: 95000, expectedUsed: 5000, expectedDelta: 4000},
		{requestID: "reconcile-refund", userID: 7102, tokenID: 7202, groupRatio: "0.5", expectedQuota: 250, expectedUser: 100750, expectedRemain: 99750, expectedUsed: 250, expectedDelta: -750},
	}
	for _, testCase := range cases {
		require.NoError(t, model.DB.Create(&model.User{
			Id:        testCase.userID,
			Username:  testCase.requestID,
			AffCode:   testCase.requestID,
			Status:    common.UserStatusEnabled,
			Quota:     100000,
			UsedQuota: 1000,
		}).Error)
		require.NoError(t, model.DB.Create(&model.Token{
			Id:          testCase.tokenID,
			UserId:      testCase.userID,
			Key:         "sk-" + testCase.requestID,
			Name:        testCase.requestID,
			Status:      common.TokenStatusEnabled,
			RemainQuota: 99000,
			UsedQuota:   1000,
		}).Error)
		require.NoError(t, model.LOG_DB.Create(&model.Log{
			UserId:    testCase.userID,
			Type:      model.LogTypeConsume,
			Quota:     1000,
			ChannelId: channel.Id,
			TokenId:   testCase.tokenID,
			RequestId: testCase.requestID,
			CreatedAt: common.GetTimestamp(),
			Other:     common.MapToJsonStr(map[string]interface{}{"billing_source": BillingSourceWallet}),
		}).Error)
		require.NoError(t, model.DB.Create(&model.UpstreamBillingRecord{
			LocalRequestId:    testCase.requestID,
			UpstreamRequestId: testCase.requestID,
			ChannelId:         channel.Id,
			UserId:            testCase.userID,
			Provider:          string(dto.UpstreamBillingProviderSub2API),
			Status:            model.UpstreamBillingStatusEstimated,
			ChargedQuota:      1000,
			EstimatedQuota:    1000,
			UserGroup:         "standard",
			UsingGroup:        "premium",
			GroupRatio:        testCase.groupRatio,
			GroupRatioSource:  "group_group_ratio",
			QuotaPerUnit:      "500000",
			Error:             "previous lookup did not find the upstream bill",
		}).Error)
	}

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 900000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":9}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)) })

	summary := RunUpstreamBillingReconcile(context.Background(), 20, 24*time.Hour)

	assert.EqualValues(t, 2, summary.Scanned)
	assert.EqualValues(t, 2, summary.Exact)
	assert.EqualValues(t, 2, summary.Adjusted)
	for _, testCase := range cases {
		var user model.User
		require.NoError(t, model.DB.First(&user, testCase.userID).Error)
		assert.EqualValues(t, testCase.expectedUser, user.Quota)
		assert.EqualValues(t, testCase.expectedQuota, user.UsedQuota)

		var token model.Token
		require.NoError(t, model.DB.First(&token, testCase.tokenID).Error)
		assert.EqualValues(t, testCase.expectedRemain, token.RemainQuota)
		assert.EqualValues(t, testCase.expectedUsed, token.UsedQuota)

		var logEntry model.Log
		require.NoError(t, model.LOG_DB.Where("request_id = ?", testCase.requestID).First(&logEntry).Error)
		assert.EqualValues(t, testCase.expectedQuota, logEntry.Quota)
		assert.EqualValues(t, testCase.requestID, logEntry.UpstreamRequestId)

		var record model.UpstreamBillingRecord
		require.NoError(t, model.DB.Where("local_request_id = ?", testCase.requestID).First(&record).Error)
		assert.EqualValues(t, model.UpstreamBillingStatusExact, record.Status)
		assert.Empty(t, record.Error)
		assert.True(t, record.AdjustmentApplied)
		assert.True(t, record.LogUpdated)
		assert.EqualValues(t, testCase.expectedDelta, record.AdjustmentQuota)

		other, err := common.StrToMap(logEntry.Other)
		require.NoError(t, err)
		adminInfo, ok := other["admin_info"].(map[string]interface{})
		require.True(t, ok)
		billingInfo, ok := adminInfo["upstream_billing"].(map[string]interface{})
		require.True(t, ok)
		assert.Empty(t, billingInfo["error"])
	}

	var adjustedChannel model.Channel
	require.NoError(t, model.DB.First(&adjustedChannel, channel.Id).Error)
	assert.EqualValues(t, int64(5250), adjustedChannel.UsedQuota)

	costMu.Lock()
	moreCost = "0.004"
	costMu.Unlock()
	require.NoError(t, model.UpdateUpstreamBillingRecord("reconcile-more", map[string]interface{}{
		"next_recheck_at": common.GetTimestamp(),
	}))

	revisionSummary := RunUpstreamBillingReconcile(context.Background(), 20, 24*time.Hour)
	assert.EqualValues(t, 1, revisionSummary.Scanned)
	assert.EqualValues(t, 1, revisionSummary.Rechecked)
	assert.EqualValues(t, 1, revisionSummary.Revised)
	assert.EqualValues(t, 1, revisionSummary.Adjusted)
	var revisedUser model.User
	require.NoError(t, model.DB.First(&revisedUser, 7101).Error)
	assert.EqualValues(t, 97000, revisedUser.Quota)
	assert.EqualValues(t, 4000, revisedUser.UsedQuota)
	var revisedLog model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "reconcile-more").First(&revisedLog).Error)
	assert.EqualValues(t, 4000, revisedLog.Quota)

	secondSummary := RunUpstreamBillingReconcile(context.Background(), 20, 24*time.Hour)
	assert.Zero(t, secondSummary.Scanned)
}

func TestRunUpstreamBillingReconcilePreservesExactRecordWhenRecheckFails(t *testing.T) {
	InitHttpClient()
	const (
		requestID = "recheck-failure-kept"
		userID    = 7501
		channelID = 7502
		tokenID   = 7503
	)
	require.NoError(t, model.DB.Where("local_request_id = ?", requestID).Delete(&model.UpstreamBillingRecord{}).Error)
	require.NoError(t, model.DB.Where("id = ?", userID).Delete(&model.User{}).Error)
	require.NoError(t, model.DB.Where("id = ?", channelID).Delete(&model.Channel{}).Error)
	require.NoError(t, model.DB.Where("id = ?", tokenID).Delete(&model.Token{}).Error)
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Delete(&model.Log{}).Error)
	t.Cleanup(func() {
		_ = model.DB.Where("local_request_id = ?", requestID).Delete(&model.UpstreamBillingRecord{}).Error
		_ = model.DB.Where("id = ?", userID).Delete(&model.User{}).Error
		_ = model.DB.Where("id = ?", channelID).Delete(&model.Channel{}).Error
		_ = model.DB.Where("id = ?", tokenID).Delete(&model.Token{}).Error
		_ = model.LOG_DB.Where("request_id = ?", requestID).Delete(&model.Log{}).Error
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"expired billing token"}`))
	}))
	defer server.Close()

	channel := &model.Channel{
		Id:      channelID,
		Name:    "recheck-failure-channel",
		Key:     "sk-recheck-failure",
		Status:  common.ChannelStatusEnabled,
		BaseURL: common.GetPointer(server.URL),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamBilling: &dto.UpstreamBillingSettings{
		Enabled:     true,
		Provider:    dto.UpstreamBillingProviderSub2API,
		AccessToken: "expired-token",
	}})
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: requestID, AffCode: requestID, Status: common.UserStatusEnabled, Quota: 10000}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: tokenID, UserId: userID, Key: "sk-" + requestID, Status: common.TokenStatusEnabled}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:    userID,
		Type:      model.LogTypeConsume,
		Quota:     1000,
		ChannelId: channelID,
		TokenId:   tokenID,
		RequestId: requestID,
		CreatedAt: common.GetTimestamp(),
	}).Error)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.UpstreamBillingRecord{
		LocalRequestId:    requestID,
		UpstreamRequestId: requestID,
		ChannelId:         channelID,
		UserId:            userID,
		Provider:          string(dto.UpstreamBillingProviderSub2API),
		Status:            model.UpstreamBillingStatusExact,
		ChargedQuota:      1000,
		AdjustmentApplied: true,
		LogUpdated:        true,
		ExactAt:           now - 3600,
		NextRecheckAt:     now - 1,
		RecheckUntil:      now + 3600,
	}).Error)

	summary := RunUpstreamBillingReconcile(context.Background(), 20, 24*time.Hour)

	assert.EqualValues(t, 1, summary.Scanned)
	assert.EqualValues(t, 1, summary.Retried)
	var record model.UpstreamBillingRecord
	require.NoError(t, model.DB.Where("local_request_id = ?", requestID).First(&record).Error)
	assert.Equal(t, model.UpstreamBillingStatusExact, record.Status)
	assert.True(t, record.AdjustmentApplied)
	assert.EqualValues(t, 1000, record.ChargedQuota)
	assert.Greater(t, record.NextRecheckAt, int64(0))
	assert.Greater(t, record.RecheckUntil, now)
	assert.Contains(t, record.Error, "401")
	var updatedLog model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).First(&updatedLog).Error)
	logOther, err := common.StrToMap(updatedLog.Other)
	require.NoError(t, err)
	assert.Equal(t, model.UpstreamBillingStatusExact, logOther["upstream_billing_status"])
	require.NoError(t, model.UpdateUpstreamBillingRecord(requestID, map[string]interface{}{
		"next_retry_at": common.GetTimestamp(),
	}))
	retryable, err := model.FindUpstreamBillingRecordsForReconcile(common.GetTimestamp(), common.GetTimestamp()-int64((24*time.Hour).Seconds()), 20)
	require.NoError(t, err)
	retryableRequestIDs := make([]string, 0, len(retryable))
	for index := range retryable {
		retryableRequestIDs = append(retryableRequestIDs, retryable[index].LocalRequestId)
	}
	assert.Contains(t, retryableRequestIDs, requestID)
}

func TestUpstreamBillingRetryDelayUsesLongerIntervalsAsRecordAges(t *testing.T) {
	assert.Equal(t, time.Minute, upstreamBillingRetryDelay(0, time.Hour))
	assert.Equal(t, 30*time.Minute, upstreamBillingRetryDelay(10, 23*time.Hour))
	assert.Equal(t, 6*time.Hour, upstreamBillingRetryDelay(10, 2*24*time.Hour))
	assert.Equal(t, 24*time.Hour, upstreamBillingRetryDelay(10, 8*24*time.Hour))
}
