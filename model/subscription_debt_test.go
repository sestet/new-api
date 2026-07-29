package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateUserSubscriptionOffsetsExistingWalletDebt(t *testing.T) {
	truncateTables(t)
	user := User{
		Username: "subscription-debt-create",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Quota:    -800,
		AffCode:  "subscription-debt-create",
	}
	require.NoError(t, DB.Create(&user).Error)
	plan := SubscriptionPlan{
		Id:            9701,
		Title:         "Debt offset plan",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(&plan).Error)

	subscription, err := CreateUserSubscriptionFromPlanTx(DB, user.Id, &plan, "test")

	require.NoError(t, err)
	assert.EqualValues(t, 800, subscription.AmountUsed)
	assert.EqualValues(t, 800, subscription.DebtOffset)
	quota, err := GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Zero(t, quota)
}

func TestPreConsumeUserSubscriptionUsesDuePeriodsToClearDebtInOrder(t *testing.T) {
	truncateTables(t)
	now := GetDBTimestamp()
	user := User{
		Username: "subscription-debt-multiple",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Quota:    -1500,
		AffCode:  "subscription-debt-multiple",
	}
	require.NoError(t, DB.Create(&user).Error)
	plan := SubscriptionPlan{
		Id:               9702,
		Title:            "Daily debt offset",
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      1000,
		QuotaResetPeriod: SubscriptionResetDaily,
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	endTime := now + 7*24*3600
	lastReset := now - 2*24*3600
	subs := []UserSubscription{
		{Id: 9801, UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 400, StartTime: lastReset, EndTime: endTime, Status: "active", LastResetTime: lastReset, NextResetTime: now - 1},
		{Id: 9802, UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 300, StartTime: lastReset, EndTime: endTime + 1, Status: "active", LastResetTime: lastReset, NextResetTime: now - 1},
	}
	require.NoError(t, DB.Create(&subs).Error)

	result, err := PreConsumeUserSubscription("debt-cleared-request", user.Id, "gpt-test", 0, 100)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 9802, result.UserSubscriptionId)
	assert.EqualValues(t, 100, result.PreConsumed)
	first := getSubscriptionResetSub(t, 9801)
	second := getSubscriptionResetSub(t, 9802)
	assert.EqualValues(t, 1000, first.AmountUsed)
	assert.EqualValues(t, 1000, first.DebtOffset)
	assert.EqualValues(t, 600, second.AmountUsed)
	assert.EqualValues(t, 500, second.DebtOffset)
	quota, err := GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Zero(t, quota)
}

func TestPreConsumeUserSubscriptionCommitsDebtOffsetBeforeRejectingOutstandingDebt(t *testing.T) {
	truncateTables(t)
	now := GetDBTimestamp()
	user := User{
		Username: "subscription-debt-blocked",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Quota:    -1500,
		AffCode:  "subscription-debt-blocked",
	}
	require.NoError(t, DB.Create(&user).Error)
	plan := SubscriptionPlan{
		Id:               9703,
		Title:            "Insufficient debt offset",
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      1000,
		QuotaResetPeriod: SubscriptionResetDaily,
	}
	require.NoError(t, DB.Create(&plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	lastReset := now - 2*24*3600
	sub := UserSubscription{
		Id: 9901, UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000,
		AmountUsed: 900, DebtOffset: 200, StartTime: lastReset,
		EndTime: now + 7*24*3600, Status: "active",
		LastResetTime: lastReset, NextResetTime: now - 1,
	}
	require.NoError(t, DB.Create(&sub).Error)

	result, err := PreConsumeUserSubscription("debt-blocked-request", user.Id, "gpt-test", 0, 100)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSubscriptionDebtOutstanding))
	assert.Nil(t, result)
	updated := getSubscriptionResetSub(t, sub.Id)
	assert.EqualValues(t, 1000, updated.AmountUsed)
	assert.EqualValues(t, 1000, updated.DebtOffset)
	assert.Greater(t, updated.LastResetTime, lastReset)
	quota, quotaErr := GetUserQuota(user.Id, true)
	require.NoError(t, quotaErr)
	assert.EqualValues(t, -500, quota)
	var recordCount int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ?", "debt-blocked-request").
		Count(&recordCount).Error)
	assert.Zero(t, recordCount)
}
