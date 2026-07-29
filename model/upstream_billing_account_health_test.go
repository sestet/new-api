package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecoveredUpstreamBillingAccountRequeuesRecentFailedRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&UpstreamBillingAccount{}, &UpstreamBillingRecord{}))

	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	account := &UpstreamBillingAccount{HealthStatus: UpstreamBillingAccountHealthError}
	require.NoError(t, DB.Create(account).Error)
	now := common.GetTimestamp()
	records := []UpstreamBillingRecord{
		{
			LocalRequestId: "recent-failed-requeue",
			CredentialId:   account.Id,
			Status:         UpstreamBillingStatusFailed,
			NextRetryAt:    now + int64((24 * time.Hour).Seconds()),
			CreatedAt:      now - int64((29 * 24 * time.Hour).Seconds()),
		},
		{
			LocalRequestId: "expired-failed-requeue",
			CredentialId:   account.Id,
			Status:         UpstreamBillingStatusFailed,
			NextRetryAt:    now + int64((24 * time.Hour).Seconds()),
			CreatedAt:      now - int64((31 * 24 * time.Hour).Seconds()),
		},
	}
	require.NoError(t, DB.Create(&records).Error)

	recovered, err := UpdateUpstreamBillingAccountHealth(account.Id, UpstreamBillingAccountHealthHealthy, "")
	require.NoError(t, err)
	assert.True(t, recovered)
	require.NoError(t, RequeueFailedUpstreamBillingRecords(account.Id, now-int64((30*24*time.Hour).Seconds())))

	var recent UpstreamBillingRecord
	require.NoError(t, DB.Where("local_request_id = ?", "recent-failed-requeue").First(&recent).Error)
	assert.LessOrEqual(t, recent.NextRetryAt, common.GetTimestamp())

	var expired UpstreamBillingRecord
	require.NoError(t, DB.Where("local_request_id = ?", "expired-failed-requeue").First(&expired).Error)
	assert.Equal(t, now+int64((24*time.Hour).Seconds()), expired.NextRetryAt)

	recovered, err = UpdateUpstreamBillingAccountHealth(account.Id, UpstreamBillingAccountHealthHealthy, "")
	require.NoError(t, err)
	assert.False(t, recovered)
}
