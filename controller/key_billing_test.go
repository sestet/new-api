package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetKeyBillingDeclarationReturnsResolvedOverride(t *testing.T) {
	originalRatios := ratio_setting.GroupRatio2JSONString()
	originalOverrides := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalOverrides))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"upstream":0.5}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"member":{"upstream":0.13}}`))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "member")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "upstream")

	GetKeyBillingDeclaration(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Object                  string  `json:"object"`
		SchemaVersion           int     `json:"schema_version"`
		BillingScope            string  `json:"billing_scope"`
		EffectiveRateMultiplier float64 `json:"effective_rate_multiplier"`
		ObservedAt              string  `json:"observed_at"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "new-api.key_billing", response.Object)
	assert.Equal(t, 1, response.SchemaVersion)
	assert.Equal(t, "token", response.BillingScope)
	assert.InDelta(t, 0.13, response.EffectiveRateMultiplier, 0.000001)
	assert.NotEmpty(t, response.ObservedAt)
}

func TestGetKeyBillingDeclarationRejectsAutoGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "member")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "auto")

	GetKeyBillingDeclaration(ctx)

	assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
}
