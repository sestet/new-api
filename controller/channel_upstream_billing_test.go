package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectChannelUpstreamBillingRejectsRotatingTokenBeforeSave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/upstream_billing/detect", strings.NewReader(`{
		"settings": {
			"enabled": true,
			"provider": "auto",
			"access_token": "rt_refresh-token"
		}
	}`))

	DetectChannelUpstreamBilling(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "请先保存渠道")
}

func TestClearChannelInfoMasksUpstreamBillingToken(t *testing.T) {
	channel := &model.Channel{}
	channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamBilling: &dto.UpstreamBillingSettings{
		Enabled:      true,
		AccessToken:  "secret-token",
		RefreshToken: "secret-refresh-token",
	}})

	clearChannelInfo(channel)

	settings := channel.GetOtherSettings().UpstreamBilling
	require.NotNil(t, settings)
	assert.Empty(t, settings.AccessToken)
	assert.True(t, settings.AccessTokenConfigured)
	assert.Empty(t, settings.RefreshToken)
	assert.True(t, settings.RefreshTokenConfigured)
}

func TestPreserveUpstreamBillingAccessTokenOnUnchangedUpdate(t *testing.T) {
	origin := &model.Channel{}
	origin.SetOtherSettings(dto.ChannelOtherSettings{UpstreamBilling: &dto.UpstreamBillingSettings{
		Enabled:      true,
		AccessToken:  "secret-token",
		RefreshToken: "secret-refresh-token",
	}})
	updated := &model.Channel{}
	updated.SetOtherSettings(dto.ChannelOtherSettings{UpstreamBilling: &dto.UpstreamBillingSettings{
		Enabled:                true,
		Provider:               dto.UpstreamBillingProviderAuto,
		AccessTokenConfigured:  true,
		RefreshTokenConfigured: true,
	}})

	preserveUpstreamBillingCredentials(updated, origin)

	settings := updated.GetOtherSettings().UpstreamBilling
	require.NotNil(t, settings)
	assert.Equal(t, "secret-token", settings.AccessToken)
	assert.False(t, settings.AccessTokenConfigured)
	assert.Equal(t, "secret-refresh-token", settings.RefreshToken)
	assert.False(t, settings.RefreshTokenConfigured)
}

func TestUpstreamBillingAccountResponseMasksCredentials(t *testing.T) {
	account := &model.UpstreamBillingAccount{
		Id:              987654,
		Name:            "masked-account",
		Provider:        string(dto.UpstreamBillingProviderSub2API),
		APIBaseURL:      "https://billing.example.com",
		Proxy:           "http://legacy-proxy.example.com",
		AccessToken:     "secret-access-token",
		RefreshToken:    "secret-refresh-token",
		Enabled:         true,
		HealthStatus:    model.UpstreamBillingAccountHealthError,
		HealthError:     "invalid refresh token",
		HealthCheckedAt: 1_700_000_000,
	}

	response := buildUpstreamBillingAccountResponse(account, 0)
	encoded, err := common.Marshal(response)
	require.NoError(t, err)

	assert.True(t, response.AccessTokenConfigured)
	assert.True(t, response.RefreshTokenConfigured)
	assert.Equal(t, model.UpstreamBillingAccountHealthError, response.HealthStatus)
	assert.Equal(t, "invalid refresh token", response.HealthError)
	assert.EqualValues(t, 1_700_000_000, response.HealthCheckedAt)
	assert.NotContains(t, string(encoded), "secret-access-token")
	assert.NotContains(t, string(encoded), "secret-refresh-token")
	assert.NotContains(t, string(encoded), "legacy-proxy.example.com")
	assert.NotContains(t, string(encoded), `"proxy"`)
}

func TestUpstreamBillingAccountResponseNormalizesLegacyHealthStatus(t *testing.T) {
	response := buildUpstreamBillingAccountResponse(&model.UpstreamBillingAccount{}, 0)

	assert.Equal(t, model.UpstreamBillingAccountHealthUnknown, response.HealthStatus)
}

func TestApplyUpstreamBillingAccountInputPreservesBlankCredentialsOnUpdate(t *testing.T) {
	enabled := false
	account := &model.UpstreamBillingAccount{
		AccessToken:  "stored-access-token",
		RefreshToken: "stored-refresh-token",
	}

	applyUpstreamBillingAccountInput(account, upstreamBillingAccountInput{
		Name:       "disabled-account",
		Provider:   dto.UpstreamBillingProviderSub2API,
		APIBaseURL: "https://billing.example.com",
		Enabled:    &enabled,
	}, false)

	assert.Equal(t, "stored-access-token", account.AccessToken)
	assert.Equal(t, "stored-refresh-token", account.RefreshToken)
	assert.False(t, account.Enabled)
}
