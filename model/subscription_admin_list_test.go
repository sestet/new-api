package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchAdminUserSubscriptionsFiltersAndPaginates(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	users := []User{
		{Username: "alice", DisplayName: "Alice", Email: "alice@example.com", Password: "password", Status: common.UserStatusEnabled, AffCode: "sub-list-alice"},
		{Username: "bob", DisplayName: "Bob", Email: "bob@example.com", Password: "password", Status: common.UserStatusEnabled, AffCode: "sub-list-bob"},
	}
	require.NoError(t, DB.Create(&users).Error)
	plans := []SubscriptionPlan{
		{Title: "Starter", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: SubscriptionResetCustom, QuotaResetCustomSeconds: 3600},
		{Title: "Pro", DurationUnit: SubscriptionDurationMonth, DurationValue: 1},
	}
	require.NoError(t, DB.Create(&plans).Error)
	subscriptions := []UserSubscription{
		{UserId: users[0].Id, PlanId: plans[0].Id, Status: "active", AmountTotal: 1000, AmountUsed: 250, DebtOffset: 75, StartTime: now - 100, EndTime: now + 100, LastResetTime: now - 50, NextResetTime: now + 50},
		{UserId: users[0].Id, PlanId: plans[1].Id, Status: "active", AmountTotal: 2000, AmountUsed: 2000, StartTime: now - 200, EndTime: now - 1},
		{UserId: users[1].Id, PlanId: plans[1].Id, Status: "cancelled", AmountTotal: 3000, AmountUsed: 100, StartTime: now - 300, EndTime: now},
	}
	require.NoError(t, DB.Create(&subscriptions).Error)

	items, total, err := SearchAdminUserSubscriptions("alice", 0, "", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, items, 2)
	assert.Equal(t, "active", items[0].Status)
	assert.Equal(t, "Starter", items[0].PlanTitle)
	assert.Equal(t, SubscriptionResetCustom, items[0].QuotaResetPeriod)
	assert.Equal(t, int64(3600), items[0].QuotaResetCustomSeconds)
	assert.EqualValues(t, 75, items[0].DebtOffset)
	assert.Equal(t, now-50, items[0].LastResetTime)
	assert.Equal(t, now+50, items[0].NextResetTime)
	assert.Equal(t, "expired", items[1].Status)

	items, total, err = SearchAdminUserSubscriptions("", plans[1].Id, "cancelled", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, users[1].Id, items[0].UserId)

	items, total, err = SearchAdminUserSubscriptions("", 0, "", 1, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, items, 1)
}

func TestSearchAdminUserSubscriptionsMatchesExactUserId(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	user := User{Username: "numeric-search", Password: "password", Status: common.UserStatusEnabled, AffCode: "sub-list-numeric"}
	require.NoError(t, DB.Create(&user).Error)
	plan := SubscriptionPlan{Title: "ID Plan", DurationUnit: SubscriptionDurationMonth, DurationValue: 1}
	require.NoError(t, DB.Create(&plan).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		UserId: user.Id, PlanId: plan.Id, Status: "active", StartTime: now, EndTime: now + 100,
	}).Error)

	items, total, err := SearchAdminUserSubscriptions(strconv.Itoa(user.Id), 0, "active", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, user.Id, items[0].UserId)
}
