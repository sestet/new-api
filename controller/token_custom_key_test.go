package controller

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func customTokenRequest(name string, customKey any) map[string]any {
	request := map[string]any{
		"name":            name,
		"expired_time":    -1,
		"remain_quota":    0,
		"unlimited_quota": true,
		"group":           "default",
	}
	if customKey != nil {
		request["custom_key"] = customKey
	}
	return request
}

func TestAddTokenCreatesCustomKeyWithOptionalPrefix(t *testing.T) {
	tests := []struct {
		name      string
		customKey string
		storedKey string
	}{
		{name: "with prefix", customKey: "sk-custom_key_1234567890", storedKey: "custom_key_1234567890"},
		{name: "without prefix", customKey: "custom-key-1234567890", storedKey: "custom-key-1234567890"},
		{name: "trimmed", customKey: "  sk-custom_key_0987654321  ", storedKey: "custom_key_0987654321"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupTokenControllerTestDB(t)
			ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", customTokenRequest(test.name, test.customKey), 101)

			AddToken(ctx)

			response := decodeAPIResponse(t, recorder)
			require.True(t, response.Success, response.Message)
			var token model.Token
			require.NoError(t, model.DB.Where("name = ?", test.name).First(&token).Error)
			assert.Equal(t, test.storedKey, token.Key)
		})
	}
}

func TestAddTokenRejectsInvalidCustomKey(t *testing.T) {
	tests := []struct {
		name      string
		customKey string
	}{
		{name: "empty", customKey: ""},
		{name: "prefix only", customKey: "sk-"},
		{name: "too short", customKey: "short_custom_ke"},
		{name: "too long", customKey: strings.Repeat("a", 129)},
		{name: "invalid characters", customKey: "custom.key.123456"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupTokenControllerTestDB(t)
			ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", customTokenRequest(test.name, test.customKey), 101)

			AddToken(ctx)

			response := decodeAPIResponse(t, recorder)
			assert.False(t, response.Success)
			var count int64
			require.NoError(t, model.DB.Model(&model.Token{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestAddTokenRejectsExistingCustomKeyIncludingDeletedRows(t *testing.T) {
	setupTokenControllerTestDB(t)
	existing := &model.Token{
		UserId:      101,
		Key:         "existing_key_123456",
		Name:        "existing",
		Status:      common.TokenStatusEnabled,
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, existing.Insert())
	require.NoError(t, existing.Delete())
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", customTokenRequest("duplicate", "sk-existing_key_123456"), 101)

	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	assert.False(t, response.Success)
	var count int64
	require.NoError(t, model.DB.Unscoped().Model(&model.Token{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestAddTokenStillGeneratesRandomKeyWhenCustomKeyIsOmitted(t *testing.T) {
	setupTokenControllerTestDB(t)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", customTokenRequest("random", nil), 101)

	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var token model.Token
	require.NoError(t, model.DB.Where("name = ?", "random").First(&token).Error)
	assert.Len(t, token.Key, 48)
}
