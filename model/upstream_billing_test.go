package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamBillingAccountSettingsIgnoreLegacyAccountProxy(t *testing.T) {
	account := UpstreamBillingAccount{
		Id:         1,
		Provider:   string(dto.UpstreamBillingProviderNewAPI),
		APIBaseURL: "https://billing.example.com",
		Proxy:      "http://legacy-proxy.example.com",
	}

	settings := account.ToSettings()

	require.NotNil(t, settings)
	assert.Empty(t, settings.Proxy)
}

func TestUpdateUpstreamBillingAccountHealthPersistsLatestResult(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UpstreamBillingAccount{}))
	account := &UpstreamBillingAccount{
		Name:        "health-status-account",
		Provider:    string(dto.UpstreamBillingProviderNewAPI),
		APIBaseURL:  "https://billing.example.com",
		AccessToken: "access-token",
		Enabled:     true,
	}
	require.NoError(t, CreateUpstreamBillingAccount(account))
	t.Cleanup(func() {
		_ = DB.Where("id = ?", account.Id).Delete(&UpstreamBillingAccount{}).Error
	})

	_, err := UpdateUpstreamBillingAccountHealth(account.Id, UpstreamBillingAccountHealthError, "  invalid refresh token  ")
	require.NoError(t, err)
	failed, err := GetUpstreamBillingAccountByID(account.Id)
	require.NoError(t, err)
	assert.Equal(t, UpstreamBillingAccountHealthError, failed.HealthStatus)
	assert.Equal(t, "invalid refresh token", failed.HealthError)
	assert.Greater(t, failed.HealthCheckedAt, int64(0))

	_, err = UpdateUpstreamBillingAccountHealth(account.Id, "invalid", "ignored")
	require.Error(t, err)
	recovered, err := UpdateUpstreamBillingAccountHealth(account.Id, UpstreamBillingAccountHealthHealthy, "stale error")
	require.NoError(t, err)
	assert.True(t, recovered)
	healthy, err := GetUpstreamBillingAccountByID(account.Id)
	require.NoError(t, err)
	assert.Equal(t, UpstreamBillingAccountHealthHealthy, healthy.HealthStatus)
	assert.Empty(t, healthy.HealthError)
}

func TestGetUpstreamBillingStatsSeparatesExactAndFallbackAmounts(t *testing.T) {
	require.NoError(t, DB.Where("1 = 1").Delete(&UpstreamBillingRecord{}).Error)
	t.Cleanup(func() {
		_ = DB.Where("1 = 1").Delete(&UpstreamBillingRecord{}).Error
	})
	records := []UpstreamBillingRecord{
		{LocalRequestId: "exact-1", ChannelId: 1, Status: UpstreamBillingStatusExact, ChargedQuota: 100},
		{LocalRequestId: "estimated-1", ChannelId: 1, Status: UpstreamBillingStatusEstimated, ChargedQuota: 200},
		{LocalRequestId: "failed-1", ChannelId: 2, Status: UpstreamBillingStatusFailed, ChargedQuota: 300},
		{LocalRequestId: "pending-1", ChannelId: 2, Status: UpstreamBillingStatusPending, ChargedQuota: 400},
	}
	require.NoError(t, DB.Create(&records).Error)

	stats, err := GetUpstreamBillingStats()

	require.NoError(t, err)
	assert.Equal(t, int64(4), stats.Total)
	assert.Equal(t, int64(1), stats.Exact)
	assert.Equal(t, int64(1), stats.Estimated)
	assert.Equal(t, int64(1), stats.Failed)
	assert.Equal(t, int64(1), stats.Pending)
	assert.Equal(t, int64(100), stats.ExactQuota)
	assert.Equal(t, int64(500), stats.EstimatedQuota)
	assert.Equal(t, int64(400), stats.PendingQuota)
	assert.InDelta(t, 0.25, stats.Coverage, 0.0001)
	require.Len(t, stats.Channels, 2)
	assert.Equal(t, 1, stats.Channels[0].ChannelId)
	assert.InDelta(t, 0.5, stats.Channels[0].Coverage, 0.0001)
	assert.Equal(t, 2, stats.Channels[1].ChannelId)
	assert.InDelta(t, 0.0, stats.Channels[1].Coverage, 0.0001)
}

func TestFindUpstreamBillingRecordsForCredentialReconcileScopesAndForcesRecentRecords(t *testing.T) {
	const (
		credentialID    = 9101
		otherCredential = 9102
		boundChannelID  = 9103
	)
	requestIDs := []string{
		"credential-reconcile-exact",
		"credential-reconcile-other",
		"credential-reconcile-expired",
		"credential-reconcile-legacy-bound-channel",
	}
	require.NoError(t, DB.Where("local_request_id IN ?", requestIDs).Delete(&UpstreamBillingRecord{}).Error)
	require.NoError(t, DB.Where("id = ?", boundChannelID).Delete(&Channel{}).Error)
	t.Cleanup(func() {
		_ = DB.Where("local_request_id IN ?", requestIDs).Delete(&UpstreamBillingRecord{}).Error
		_ = DB.Where("id = ?", boundChannelID).Delete(&Channel{}).Error
	})
	channel := &Channel{Id: boundChannelID, Name: "credential-reconcile-bound-channel", Status: common.ChannelStatusEnabled, Key: "sk-test"}
	channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamBilling: &dto.UpstreamBillingSettings{
		Enabled:      true,
		CredentialID: credentialID,
	}})
	require.NoError(t, DB.Create(channel).Error)

	now := common.GetTimestamp()
	records := []UpstreamBillingRecord{
		{
			LocalRequestId:    requestIDs[0],
			CredentialId:      credentialID,
			Status:            UpstreamBillingStatusExact,
			AdjustmentApplied: true,
			CreatedAt:         now - int64((2 * time.Hour).Seconds()),
			NextRetryAt:       now + int64(time.Hour.Seconds()),
			NextRecheckAt:     now + int64(time.Hour.Seconds()),
		},
		{
			LocalRequestId: requestIDs[1],
			CredentialId:   otherCredential,
			Status:         UpstreamBillingStatusFailed,
			CreatedAt:      now - int64(time.Hour.Seconds()),
		},
		{
			LocalRequestId: requestIDs[2],
			CredentialId:   credentialID,
			Status:         UpstreamBillingStatusFailed,
			CreatedAt:      now - int64((31 * 24 * time.Hour).Seconds()),
		},
		{
			LocalRequestId: requestIDs[3],
			ChannelId:      boundChannelID,
			CredentialId:   0,
			Status:         UpstreamBillingStatusEstimated,
			CreatedAt:      now - int64(time.Hour.Seconds()),
		},
	}
	require.NoError(t, DB.Create(&records).Error)

	matched, err := FindUpstreamBillingRecordsForCredentialReconcile(
		now,
		now-int64((30*24*time.Hour).Seconds()),
		20,
		credentialID,
	)
	require.NoError(t, err)
	require.Len(t, matched, 2)
	assert.Equal(t, requestIDs[0], matched[0].LocalRequestId)
	assert.Equal(t, requestIDs[3], matched[1].LocalRequestId)
}

func TestApplyUpstreamBillingAdjustmentUsesSubscriptionThenWalletAndIsIdempotent(t *testing.T) {
	const userID = 8101
	const subscriptionID = 8201
	const channelID = 8301
	require.NoError(t, DB.Where("local_request_id = ?", "subscription-adjustment").Delete(&UpstreamBillingRecord{}).Error)
	require.NoError(t, DB.Where("id = ?", subscriptionID).Delete(&UserSubscription{}).Error)
	require.NoError(t, DB.Where("id = ?", userID).Delete(&User{}).Error)
	require.NoError(t, DB.Where("id = ?", channelID).Delete(&Channel{}).Error)

	require.NoError(t, DB.Create(&User{
		Id:        userID,
		Username:  "subscription-adjustment",
		AffCode:   "subscription-adjustment",
		Quota:     10000,
		UsedQuota: 1000,
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id:          subscriptionID,
		UserId:      userID,
		AmountTotal: 2000,
		AmountUsed:  1900,
		Status:      "active",
	}).Error)
	require.NoError(t, DB.Create(&Channel{Id: channelID, Name: "subscription-adjustment", Key: "sk-adjustment", UsedQuota: 1000}).Error)
	require.NoError(t, DB.Create(&UpstreamBillingRecord{
		LocalRequestId: "subscription-adjustment",
		ChannelId:      channelID,
		UserId:         userID,
		Status:         UpstreamBillingStatusEstimated,
		ChargedQuota:   1000,
	}).Error)

	input := UpstreamBillingAdjustmentInput{
		LocalRequestId:  "subscription-adjustment",
		Provider:        "sub2api",
		UpstreamCostUSD: "0.003",
		ExactQuota:      1500,
		CurrentCharged:  1000,
		BillingSource:   "subscription",
		SubscriptionId:  subscriptionID,
	}
	result, err := ApplyUpstreamBillingAdjustment(input)

	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.Equal(t, int64(500), result.AdjustmentQuota)
	assert.Equal(t, int64(400), result.WalletAdjustment)

	var user User
	require.NoError(t, DB.First(&user, userID).Error)
	assert.Equal(t, int64(9600), user.Quota)
	assert.Equal(t, int64(1500), user.UsedQuota)
	var subscription UserSubscription
	require.NoError(t, DB.First(&subscription, subscriptionID).Error)
	assert.Equal(t, int64(2000), subscription.AmountUsed)

	revisedInput := input
	revisedInput.ExactQuota = 1200
	secondResult, err := ApplyUpstreamBillingAdjustment(revisedInput)
	require.NoError(t, err)
	assert.True(t, secondResult.Applied)
	assert.Equal(t, int64(200), secondResult.AdjustmentQuota)
	assert.Equal(t, int64(100), secondResult.WalletAdjustment)
	require.NoError(t, DB.First(&user, userID).Error)
	assert.Equal(t, int64(9900), user.Quota)
	assert.Equal(t, int64(1200), user.UsedQuota)
	require.NoError(t, DB.First(&subscription, subscriptionID).Error)
	assert.Equal(t, int64(2000), subscription.AmountUsed)

	thirdResult, err := ApplyUpstreamBillingAdjustment(revisedInput)
	require.NoError(t, err)
	assert.False(t, thirdResult.Applied)
	require.NoError(t, DB.First(&user, userID).Error)
	assert.Equal(t, int64(9900), user.Quota)
}

func TestApplyUpstreamBillingAdjustmentLogOnlyDoesNotChangeAccountBalances(t *testing.T) {
	const userID = 8401
	const channelID = 8402
	const requestID = "channel-test-log-only-adjustment"
	require.NoError(t, DB.Where("local_request_id = ?", requestID).Delete(&UpstreamBillingRecord{}).Error)
	require.NoError(t, DB.Where("id = ?", userID).Delete(&User{}).Error)
	require.NoError(t, DB.Where("id = ?", channelID).Delete(&Channel{}).Error)
	t.Cleanup(func() {
		_ = DB.Where("local_request_id = ?", requestID).Delete(&UpstreamBillingRecord{}).Error
		_ = DB.Where("id = ?", userID).Delete(&User{}).Error
		_ = DB.Where("id = ?", channelID).Delete(&Channel{}).Error
	})

	require.NoError(t, DB.Create(&User{
		Id:        userID,
		Username:  "channel-test-log-only",
		AffCode:   "channel-test-log-only",
		Quota:     10_000,
		UsedQuota: 1_000,
	}).Error)
	require.NoError(t, DB.Create(&Channel{
		Id:        channelID,
		Name:      "channel-test-log-only",
		Key:       "sk-channel-test",
		UsedQuota: 1_000,
	}).Error)
	require.NoError(t, DB.Create(&UpstreamBillingRecord{
		LocalRequestId:    requestID,
		UpstreamRequestId: "response-request-id",
		ChannelId:         channelID,
		UserId:            userID,
		Status:            UpstreamBillingStatusEstimated,
		ChargedQuota:      100,
		EstimatedQuota:    100,
	}).Error)

	result, err := ApplyUpstreamBillingAdjustment(UpstreamBillingAdjustmentInput{
		LocalRequestId:    requestID,
		IdentityAmbiguous: true,
		Provider:          "sub2api",
		UpstreamCostUSD:   "0.0000025",
		ExactQuota:        250,
		CurrentCharged:    100,
		LogOnly:           true,
	})

	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.Equal(t, int64(150), result.AdjustmentQuota)
	assert.Zero(t, result.WalletAdjustment)

	var user User
	require.NoError(t, DB.First(&user, userID).Error)
	assert.Equal(t, int64(10_000), user.Quota)
	assert.Equal(t, int64(1_000), user.UsedQuota)
	var channel Channel
	require.NoError(t, DB.First(&channel, channelID).Error)
	assert.Equal(t, int64(1_000), channel.UsedQuota)
	var record UpstreamBillingRecord
	require.NoError(t, DB.Where("local_request_id = ?", requestID).First(&record).Error)
	assert.Equal(t, UpstreamBillingStatusExact, record.Status)
	assert.Equal(t, int64(250), record.ChargedQuota)
	assert.Equal(t, int64(150), record.AdjustmentQuota)
	assert.Zero(t, record.WalletAdjustment)
	assert.True(t, record.IdentityAmbiguous)
	assert.Equal(t, "response-request-id", record.UpstreamRequestId)
}
