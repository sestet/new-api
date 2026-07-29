package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageLogStatsSummarizeAndFilterUpstreamBillingStates(t *testing.T) {
	requestIDs := []string{
		"log-stat-exact",
		"log-stat-estimated",
		"log-stat-pending",
		"log-stat-failed",
		"log-stat-untracked",
		"log-stat-error",
	}
	require.NoError(t, LOG_DB.Where("request_id IN ?", requestIDs).Delete(&Log{}).Error)
	t.Cleanup(func() {
		_ = LOG_DB.Where("request_id IN ?", requestIDs).Delete(&Log{}).Error
	})

	now := time.Now().Unix()
	logs := []Log{
		{RequestId: requestIDs[0], CreatedAt: now, Type: LogTypeConsume, Quota: 100, Other: common.MapToJsonStr(map[string]interface{}{"upstream_billing_status": UpstreamBillingStatusExact})},
		{RequestId: requestIDs[1], CreatedAt: now, Type: LogTypeConsume, Quota: 200, Other: common.MapToJsonStr(map[string]interface{}{"upstream_billing_status": UpstreamBillingStatusEstimated})},
		{RequestId: requestIDs[2], CreatedAt: now, Type: LogTypeConsume, Quota: 300, Other: common.MapToJsonStr(map[string]interface{}{"upstream_billing_status": UpstreamBillingStatusPending})},
		{RequestId: requestIDs[3], CreatedAt: now, Type: LogTypeConsume, Quota: 400, Other: common.MapToJsonStr(map[string]interface{}{"upstream_billing_status": UpstreamBillingStatusFailed})},
		{RequestId: requestIDs[4], CreatedAt: now, Type: LogTypeConsume, Quota: 500, Other: common.MapToJsonStr(map[string]interface{}{})},
		{RequestId: requestIDs[5], CreatedAt: now, Type: LogTypeError, Quota: 999, Other: common.MapToJsonStr(map[string]interface{}{"upstream_billing_status": UpstreamBillingStatusFailed})},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	stat, err := SumUsedQuota(0, now-1, now+1, "", "", "", 0, "", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1500), stat.Quota)
	assert.Equal(t, int64(5), stat.RequestCount)
	assert.Equal(t, int64(1), stat.Exact)
	assert.Equal(t, int64(1), stat.Estimated)
	assert.Equal(t, int64(1), stat.Pending)
	assert.Equal(t, int64(1), stat.Failed)

	waiting, err := SumUsedQuota(0, now-1, now+1, "", "", "", 0, "", "", "", "waiting")
	require.NoError(t, err)
	assert.Equal(t, int64(500), waiting.Quota)
	assert.Equal(t, int64(2), waiting.RequestCount)
	assert.Equal(t, int64(1), waiting.Estimated)
	assert.Equal(t, int64(1), waiting.Pending)

	filtered, total, err := GetAllLogs(0, now-1, now+1, "", "", "", 0, 20, 0, "", "", "", UpstreamBillingStatusExact)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, filtered, 1)
	assert.Equal(t, requestIDs[0], filtered[0].RequestId)

	_, err = SumUsedQuota(0, now-1, now+1, "", "", "", 0, "", "", "", "not-a-status")
	require.Error(t, err)
}
