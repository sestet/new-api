package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinimumTopupUsesInt64QuotaPrecision(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalMinTopUp := operation_setting.MinTopUp
	originalStripeMinTopUp := setting.StripeMinTopUp
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.MinTopUp = originalMinTopUp
		setting.StripeMinTopUp = originalStripeMinTopUp
	})

	common.QuotaPerUnit = 100_000_000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	operation_setting.MinTopUp = 3
	setting.StripeMinTopUp = 5

	require.Equal(t, int64(300_000_000), getMinTopup())
	assert.Equal(t, int64(500_000_000), getStripeMinTopup())
}
