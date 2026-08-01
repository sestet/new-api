package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTokenQuotaAPIErrorUsesTooManyRequestsForTimeWindowLimit(t *testing.T) {
	rateLimitErr := &model.TokenRateLimitError{Window: model.TokenRateLimitWindow5h, Limit: 100, Used: 90, Requested: 20, ResetAt: 2000}

	apiErr := newTokenQuotaAPIError(rateLimitErr)

	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCodePreConsumeTokenQuotaFailed, apiErr.GetErrorCode())
	assert.True(t, errors.Is(apiErr, rateLimitErr))
}

func TestNewTokenQuotaAPIErrorKeepsLifetimeQuotaFailureForbidden(t *testing.T) {
	apiErr := newTokenQuotaAPIError(errors.New("token quota is not enough"))

	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestBillingSessionReserveAllowsWalletArrearsAfterRetry(t *testing.T) {
	truncate(t)
	const (
		userID         = 9101
		remainingQuota = 50
		reservedQuota  = 100
		targetQuota    = 200
	)
	seedUser(t, userID, remainingQuota)

	funding := &WalletFunding{userId: userID, consumed: reservedQuota}
	session := &BillingSession{
		relayInfo:        &relaycommon.RelayInfo{UserId: userID, IsPlayground: true},
		funding:          funding,
		preConsumedQuota: reservedQuota,
		tokenConsumed:    reservedQuota,
	}

	require.NoError(t, session.Reserve(targetQuota))
	require.EqualValues(t, targetQuota, session.GetPreConsumedQuota())
	require.EqualValues(t, targetQuota, funding.consumed)

	quota, quotaErr := model.GetUserQuota(userID, true)
	require.NoError(t, quotaErr)
	require.EqualValues(t, remainingQuota-(targetQuota-reservedQuota), quota)
}

func TestBillingSessionSettleChargesSubscriptionOverflowToWalletOnce(t *testing.T) {
	truncate(t)
	const (
		userID          = 9201
		subscriptionID  = 9202
		preConsumed     = 5_210
		subscriptionCap = 1_000_000
		actualQuota     = 5_000_000
		walletDebt      = 4_000_000
		requestID       = "subscription-overflow-settlement"
	)
	seedUser(t, userID, 0)
	seedSubscription(t, subscriptionID, userID, subscriptionCap, preConsumed)
	require.NoError(t, model.DB.Create(&model.UpstreamBillingRecord{
		LocalRequestId:    requestID,
		UserId:            userID,
		Status:            model.UpstreamBillingStatusExact,
		ChargedQuota:      actualQuota,
		AdjustmentApplied: true,
	}).Error)

	relayInfo := &relaycommon.RelayInfo{
		UserId:                                userID,
		RequestId:                             requestID,
		IsPlayground:                          true,
		BillingSource:                         BillingSourceSubscription,
		SubscriptionId:                        subscriptionID,
		SubscriptionPreConsumed:               preConsumed,
		SubscriptionAmountTotal:               subscriptionCap,
		SubscriptionAmountUsedAfterPreConsume: preConsumed,
	}
	funding := &SubscriptionFunding{
		requestId:       requestID,
		userId:          userID,
		subscriptionId:  subscriptionID,
		preConsumed:     preConsumed,
		AmountTotal:     subscriptionCap,
		AmountUsedAfter: preConsumed,
	}
	session := &BillingSession{
		relayInfo:        relayInfo,
		funding:          funding,
		preConsumedQuota: preConsumed,
	}

	require.NoError(t, session.Settle(actualQuota))
	require.NoError(t, session.Settle(actualQuota))

	var subscription model.UserSubscription
	require.NoError(t, model.DB.First(&subscription, subscriptionID).Error)
	assert.Equal(t, int64(subscriptionCap), subscription.AmountUsed)
	quota, err := model.GetUserQuota(userID, true)
	require.NoError(t, err)
	assert.Equal(t, int64(-walletDebt), quota)
	assert.Equal(t, int64(subscriptionCap-preConsumed), relayInfo.SubscriptionPostDelta)
	assert.Equal(t, int64(walletDebt), relayInfo.WalletQuotaDeducted)
	other := map[string]interface{}{}
	appendBillingInfo(relayInfo, other)
	assert.Equal(t, int64(subscriptionCap), other["subscription_consumed"])
	assert.Equal(t, int64(walletDebt), other["wallet_quota_deducted"])
	assert.Equal(t, int64(0), other["subscription_remain"])

	var record model.UpstreamBillingRecord
	require.NoError(t, model.DB.Where("local_request_id = ?", requestID).First(&record).Error)
	assert.Equal(t, int64(walletDebt), record.WalletAdjustment)
}
