package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTokenRateLimitTest(t *testing.T, token *Token) {
	t.Helper()
	db := useIsolatedSchemaDB(t)
	require.NoError(t, db.AutoMigrate(&Token{}))
	require.NoError(t, db.Create(token).Error)
}

func getTokenRateLimitTestToken(t *testing.T, id int) Token {
	t.Helper()
	var token Token
	require.NoError(t, DB.Where("id = ?", id).First(&token).Error)
	return token
}

func TestTokenRateLimitRejectsWholeChargeWhenAnyWindowWouldOverflow(t *testing.T) {
	token := &Token{
		Id:             101,
		UserId:         1,
		Key:            "rate-limit-reject",
		RemainQuota:    1000,
		RateLimit5h:    100,
		RateLimit1d:    200,
		RateLimit7d:    300,
		UnlimitedQuota: false,
	}
	setupTokenRateLimitTest(t, token)

	require.NoError(t, DecreaseTokenQuotaWithRateLimit(token.Id, token.Key, 60, 1000, true))
	err := DecreaseTokenQuotaWithRateLimit(token.Id, token.Key, 50, 1001, true)

	var rateLimitErr *TokenRateLimitError
	require.ErrorAs(t, err, &rateLimitErr)
	assert.Equal(t, TokenRateLimitWindow5h, rateLimitErr.Window)
	assert.EqualValues(t, 100, rateLimitErr.Limit)
	assert.EqualValues(t, 60, rateLimitErr.Used)
	assert.EqualValues(t, 50, rateLimitErr.Requested)
	assert.EqualValues(t, 1000+tokenRateLimitDuration5h, rateLimitErr.ResetAt)

	stored := getTokenRateLimitTestToken(t, token.Id)
	assert.EqualValues(t, 940, stored.RemainQuota)
	assert.EqualValues(t, 60, stored.UsedQuota)
	assert.EqualValues(t, 60, stored.Usage5h)
	assert.EqualValues(t, 60, stored.Usage1d)
	assert.EqualValues(t, 60, stored.Usage7d)
}

func TestTokenRateLimitResetsEachWindowIndependentlyAndRefundsOriginalWindows(t *testing.T) {
	token := &Token{
		Id:             102,
		UserId:         1,
		Key:            "rate-limit-windows",
		RemainQuota:    1000,
		RateLimit5h:    200,
		RateLimit1d:    200,
		RateLimit7d:    500,
		UnlimitedQuota: false,
	}
	setupTokenRateLimitTest(t, token)

	firstWindowStart := int64(1000)
	secondWindowStart := firstWindowStart + tokenRateLimitDuration5h
	require.NoError(t, DecreaseTokenQuotaWithRateLimit(token.Id, token.Key, 80, firstWindowStart, true))
	require.NoError(t, DecreaseTokenQuotaWithRateLimit(token.Id, token.Key, 90, secondWindowStart, true))

	stored := getTokenRateLimitTestToken(t, token.Id)
	assert.Equal(t, secondWindowStart, stored.Window5hStart)
	assert.EqualValues(t, 90, stored.Usage5h)
	assert.Equal(t, firstWindowStart, stored.Window1dStart)
	assert.EqualValues(t, 170, stored.Usage1d)
	assert.EqualValues(t, 170, stored.Usage7d)

	err := DecreaseTokenQuotaWithRateLimit(token.Id, token.Key, 40, secondWindowStart+1, true)
	var rateLimitErr *TokenRateLimitError
	require.ErrorAs(t, err, &rateLimitErr)
	assert.Equal(t, TokenRateLimitWindow1d, rateLimitErr.Window)

	require.NoError(t, IncreaseTokenQuotaWithRateLimit(token.Id, token.Key, 20, firstWindowStart))
	stored = getTokenRateLimitTestToken(t, token.Id)
	assert.EqualValues(t, 90, stored.Usage5h)
	assert.EqualValues(t, 150, stored.Usage1d)
	assert.EqualValues(t, 150, stored.Usage7d)
	assert.EqualValues(t, 1000, stored.UsedQuota+stored.RemainQuota)
	assert.EqualValues(t, 150, stored.UsedQuota)
}

func TestResetTokenRateLimitUsagePreservesLifetimeQuota(t *testing.T) {
	token := &Token{
		Id:             103,
		UserId:         7,
		Key:            "rate-limit-reset",
		RemainQuota:    700,
		UsedQuota:      300,
		RateLimit5h:    100,
		Usage5h:        80,
		Window5hStart:  1000,
		UnlimitedQuota: false,
	}
	setupTokenRateLimitTest(t, token)

	reset, err := ResetTokenRateLimitUsage(token.Id, token.UserId)
	require.NoError(t, err)
	assert.Zero(t, reset.Usage5h)
	assert.Positive(t, reset.Window5hStart)
	assert.EqualValues(t, 700, reset.RemainQuota)
	assert.EqualValues(t, 300, reset.UsedQuota)

	require.NoError(t, IncreaseTokenQuotaWithRateLimit(token.Id, token.Key, 50, 1000))
	stored := getTokenRateLimitTestToken(t, token.Id)
	assert.Zero(t, stored.Usage5h)
	assert.Equal(t, reset.Window5hStart, stored.Window5hStart)
	assert.EqualValues(t, 250, stored.UsedQuota)

	_, err = ResetTokenRateLimitUsage(token.Id, token.UserId+1)
	require.Error(t, err)
}
