package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageLogQueriesSeparateRequestAndAuditRecords(t *testing.T) {
	now := time.Now().Unix()
	requestIDs := []string{
		"usage-scope-consume",
		"usage-scope-error",
		"usage-scope-refund",
		"usage-scope-login",
		"usage-scope-manage",
		"usage-scope-system",
		"usage-scope-topup",
	}
	require.NoError(t, LOG_DB.Where("request_id IN ?", requestIDs).Delete(&Log{}).Error)
	t.Cleanup(func() {
		_ = LOG_DB.Where("request_id IN ?", requestIDs).Delete(&Log{}).Error
	})

	logs := []Log{
		{RequestId: requestIDs[0], CreatedAt: now, Type: LogTypeConsume},
		{RequestId: requestIDs[1], CreatedAt: now, Type: LogTypeError},
		{RequestId: requestIDs[2], CreatedAt: now, Type: LogTypeRefund},
		{RequestId: requestIDs[3], CreatedAt: now, Type: LogTypeLogin},
		{RequestId: requestIDs[4], CreatedAt: now, Type: LogTypeManage},
		{RequestId: requestIDs[5], CreatedAt: now, Type: LogTypeSystem},
		{RequestId: requestIDs[6], CreatedAt: now, Type: LogTypeTopup},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	usageLogs, usageTotal, err := GetAllUsageLogs(0, now-1, now+1, "", "", "", 0, 20, 0, "", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(3), usageTotal)
	for _, logEntry := range usageLogs {
		assert.Contains(t, usageLogTypeValues(), logEntry.Type)
	}

	auditLogs, auditTotal, err := GetAuditLogs(0, now-1, now+1, "", 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(3), auditTotal)
	for _, logEntry := range auditLogs {
		assert.Contains(t, []int{LogTypeLogin, LogTypeManage, LogTypeSystem}, logEntry.Type)
	}
}

func TestUsageAnalyticsUsesConsumeLogsForCostAndDimensions(t *testing.T) {
	now := time.Now().Unix()
	requestIDs := []string{
		"usage-analytics-exact",
		"usage-analytics-estimated",
		"usage-analytics-error",
		"usage-analytics-refund",
		"usage-analytics-topup",
	}
	require.NoError(t, LOG_DB.Where("request_id IN ?", requestIDs).Delete(&Log{}).Error)
	t.Cleanup(func() {
		_ = LOG_DB.Where("request_id IN ?", requestIDs).Delete(&Log{}).Error
	})

	logs := []Log{
		{
			RequestId: requestIDs[0], CreatedAt: now, Type: LogTypeConsume,
			ModelName: "gpt-test", Group: "dev", PromptTokens: 10, CompletionTokens: 5, Quota: 100,
			Other: common.MapToJsonStr(map[string]interface{}{"upstream_billing_status": UpstreamBillingStatusExact}),
		},
		{
			RequestId: requestIDs[1], CreatedAt: now, Type: LogTypeConsume,
			ModelName: "gpt-test", Group: "dev", PromptTokens: 20, CompletionTokens: 10, Quota: 200,
			Other: common.MapToJsonStr(map[string]interface{}{"upstream_billing_status": UpstreamBillingStatusEstimated}),
		},
		{RequestId: requestIDs[2], CreatedAt: now, Type: LogTypeError, ModelName: "gpt-test"},
		{RequestId: requestIDs[3], CreatedAt: now, Type: LogTypeRefund, ModelName: "gpt-test", Quota: 50},
		{RequestId: requestIDs[4], CreatedAt: now, Type: LogTypeTopup, Quota: 999},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	analytics, err := GetUsageAnalytics(UsageAnalyticsParams{
		StartTimestamp: now - 1,
		EndTimestamp:   now + 1,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), analytics.Summary.RequestCount)
	assert.Equal(t, int64(1), analytics.Summary.ErrorCount)
	assert.Equal(t, int64(1), analytics.Summary.RefundCount)
	assert.Equal(t, int64(45), analytics.Summary.TokenCount)
	assert.Equal(t, int64(300), analytics.Summary.Quota)
	assert.Equal(t, int64(1), analytics.Summary.Exact)
	assert.Equal(t, int64(1), analytics.Summary.Estimated)
	require.Len(t, analytics.Trend, 1)
	assert.Equal(t, int64(3), analytics.Trend[0].RequestCount)
	require.Len(t, analytics.Models, 1)
	assert.Equal(t, "gpt-test", analytics.Models[0].Name)
	require.Len(t, analytics.Groups, 1)
	assert.Equal(t, "dev", analytics.Groups[0].Name)

	errorAnalytics, err := GetUsageAnalytics(UsageAnalyticsParams{
		LogType:        LogTypeError,
		StartTimestamp: now - 1,
		EndTimestamp:   now + 1,
	})
	require.NoError(t, err)
	assert.Zero(t, errorAnalytics.Summary.RequestCount)
	assert.Equal(t, int64(1), errorAnalytics.Summary.ErrorCount)
	require.Len(t, errorAnalytics.Trend, 1)
	assert.Equal(t, int64(1), errorAnalytics.Trend[0].RequestCount)
}

func TestUsageFilterOptionsRespectUsageScopeAndUserIsolation(t *testing.T) {
	now := time.Now().Unix()
	requestIDs := []string{
		"usage-options-user-a",
		"usage-options-user-b",
		"usage-options-topup",
	}
	require.NoError(t, LOG_DB.Where("request_id IN ?", requestIDs).Delete(&Log{}).Error)
	t.Cleanup(func() {
		_ = LOG_DB.Where("request_id IN ?", requestIDs).Delete(&Log{}).Error
	})

	logs := []Log{
		{
			RequestId: requestIDs[0], UserId: 81001, Username: "usage-options-alice",
			CreatedAt: now, Type: LogTypeConsume, ModelName: "usage-options-model-a",
			Group: "usage-options-group-a", TokenName: "usage-options-token-a", ChannelId: 91001,
		},
		{
			RequestId: requestIDs[1], UserId: 81002, Username: "usage-options-bob",
			CreatedAt: now, Type: LogTypeError, ModelName: "usage-options-model-b",
			Group: "usage-options-group-b", TokenName: "usage-options-token-b", ChannelId: 91002,
		},
		{
			RequestId: requestIDs[2], UserId: 81001, Username: "usage-options-alice",
			CreatedAt: now, Type: LogTypeTopup, ModelName: "usage-options-topup-model",
		},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	adminOptions, err := GetUsageFilterOptions(0, now-1, now+1)
	require.NoError(t, err)
	assert.Contains(t, adminOptions.Models, "usage-options-model-a")
	assert.Contains(t, adminOptions.Models, "usage-options-model-b")
	assert.NotContains(t, adminOptions.Models, "usage-options-topup-model")
	assert.Contains(t, adminOptions.Users, "usage-options-alice")
	assert.Contains(t, adminOptions.Users, "usage-options-bob")
	assert.Contains(t, adminOptions.Channels, UsageFilterChannel{Id: 91001})

	userOptions, err := GetUsageFilterOptions(81001, now-1, now+1)
	require.NoError(t, err)
	assert.Contains(t, userOptions.Models, "usage-options-model-a")
	assert.NotContains(t, userOptions.Models, "usage-options-model-b")
	assert.Contains(t, userOptions.Groups, "usage-options-group-a")
	assert.Contains(t, userOptions.Tokens, "usage-options-token-a")
	assert.Empty(t, userOptions.Users)
	assert.Empty(t, userOptions.Channels)
}
