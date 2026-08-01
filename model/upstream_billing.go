package model

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	UpstreamBillingStatusPending   = "pending"
	UpstreamBillingStatusExact     = "exact"
	UpstreamBillingStatusEstimated = "estimated"
	UpstreamBillingStatusFailed    = "failed"
)

type UpstreamBillingRecord struct {
	Id                  int64  `json:"id"`
	LocalRequestId      string `json:"local_request_id" gorm:"type:varchar(64);uniqueIndex"`
	UpstreamRequestId   string `json:"upstream_request_id" gorm:"type:varchar(128);index"`
	RequestStartedAtMs  int64  `json:"request_started_at_ms" gorm:"type:bigint"`
	RequestFinishedAtMs int64  `json:"request_finished_at_ms" gorm:"type:bigint"`
	IdentityAmbiguous   bool   `json:"identity_ambiguous"`
	ChannelId           int    `json:"channel_id" gorm:"index"`
	CredentialId        int    `json:"credential_id" gorm:"index"`
	UserId              int    `json:"user_id" gorm:"index"`
	IsPlayground        bool   `json:"is_playground"`
	Provider            string `json:"provider" gorm:"type:varchar(32);index"`
	Status              string `json:"status" gorm:"type:varchar(32);index"`
	UpstreamCostUSD     string `json:"upstream_cost_usd" gorm:"type:varchar(64)"`
	UpstreamQuota       int64  `json:"upstream_quota" gorm:"type:bigint"`
	ChargedQuota        int64  `json:"charged_quota" gorm:"type:bigint"`
	EstimatedQuota      int64  `json:"estimated_quota" gorm:"type:bigint"`
	UserGroup           string `json:"user_group" gorm:"type:varchar(64)"`
	UsingGroup          string `json:"using_group" gorm:"type:varchar(64)"`
	GroupRatio          string `json:"group_ratio" gorm:"type:varchar(64)"`
	GroupRatioSource    string `json:"group_ratio_source" gorm:"type:varchar(32)"`
	QuotaPerUnit        string `json:"quota_per_unit" gorm:"type:varchar(64)"`
	CostRateMultiplier  string `json:"cost_rate_multiplier" gorm:"type:varchar(64)"`
	CostRateSource      string `json:"cost_rate_source" gorm:"type:varchar(32)"`
	ModelName           string `json:"model_name" gorm:"type:varchar(191);index"`
	PromptTokens        int64  `json:"prompt_tokens" gorm:"type:bigint"`
	CompletionTokens    int64  `json:"completion_tokens" gorm:"type:bigint"`
	TotalTokens         int64  `json:"total_tokens" gorm:"type:bigint"`
	Attempts            int    `json:"attempts"`
	Error               string `json:"error" gorm:"type:text"`
	AdjustmentApplied   bool   `json:"adjustment_applied" gorm:"index"`
	AdjustmentQuota     int64  `json:"adjustment_quota" gorm:"type:bigint"`
	WalletAdjustment    int64  `json:"wallet_adjustment" gorm:"type:bigint"`
	LogUpdated          bool   `json:"log_updated" gorm:"index"`
	RetryCount          int    `json:"retry_count"`
	NextRetryAt         int64  `json:"next_retry_at" gorm:"bigint;index"`
	LastRetryAt         int64  `json:"last_retry_at" gorm:"bigint"`
	RevisionCount       int    `json:"revision_count"`
	RecheckCount        int    `json:"recheck_count"`
	ExactAt             int64  `json:"exact_at" gorm:"bigint;index"`
	LastCheckedAt       int64  `json:"last_checked_at" gorm:"bigint"`
	NextRecheckAt       int64  `json:"next_recheck_at" gorm:"bigint;index"`
	RecheckUntil        int64  `json:"recheck_until" gorm:"bigint;index"`
	CreatedAt           int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt           int64  `json:"updated_at" gorm:"bigint"`
}

func (record *UpstreamBillingRecord) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if record.CreatedAt == 0 {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	return nil
}

func CreateUpstreamBillingRecord(record *UpstreamBillingRecord) error {
	if record == nil {
		return nil
	}
	return DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "local_request_id"}}, DoNothing: true}).Create(record).Error
}

func UpdateUpstreamBillingRecord(localRequestId string, updates map[string]interface{}) error {
	updates["updated_at"] = common.GetTimestamp()
	return DB.Model(&UpstreamBillingRecord{}).Where("local_request_id = ?", localRequestId).Updates(updates).Error
}

func GetClaimedUpstreamBillingRequestIDs(credentialId int, localRequestId string, requestIds []string) (map[string]struct{}, error) {
	claimed := make(map[string]struct{})
	if len(requestIds) == 0 {
		return claimed, nil
	}
	query := DB.Model(&UpstreamBillingRecord{}).
		Where("upstream_request_id IN ? AND local_request_id <> ?", requestIds, localRequestId)
	if credentialId > 0 {
		query = query.Where("credential_id = ?", credentialId)
	}
	var ids []string
	if err := query.Distinct().Pluck("upstream_request_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		claimed[id] = struct{}{}
	}
	return claimed, nil
}

func UpdateChannelUpstreamBillingCredentials(channelId int, update func(*dto.UpstreamBillingSettings) (bool, error)) (*dto.UpstreamBillingSettings, error) {
	if channelId <= 0 {
		return nil, errors.New("channel ID is required")
	}
	if update == nil {
		return nil, errors.New("upstream billing credential update is required")
	}
	var result *dto.UpstreamBillingSettings
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).Select("id", "settings").Where("id = ?", channelId).First(&channel).Error; err != nil {
			return err
		}
		otherSettings := dto.ChannelOtherSettings{}
		if channel.OtherSettings != "" {
			if err := common.UnmarshalJsonStr(channel.OtherSettings, &otherSettings); err != nil {
				return err
			}
		}
		if otherSettings.UpstreamBilling == nil {
			return errors.New("upstream billing settings are not configured")
		}
		changed, err := update(otherSettings.UpstreamBilling)
		if err != nil {
			return err
		}
		if changed {
			settingsBytes, err := common.Marshal(otherSettings)
			if err != nil {
				return err
			}
			if err := tx.Model(&Channel{}).Where("id = ?", channelId).Update("settings", string(settingsBytes)).Error; err != nil {
				return err
			}
		}
		settingsCopy := *otherSettings.UpstreamBilling
		result = &settingsCopy
		return nil
	})
	return result, err
}

func ListEnabledChannelsForUpstreamBillingCredentials() ([]Channel, error) {
	channels := make([]Channel, 0)
	err := DB.Select("id", "key", "base_url", "setting", "channel_info", "settings").
		Where("status = ?", common.ChannelStatusEnabled).
		Find(&channels).Error
	return channels, err
}

func FindUpstreamBillingRecordsForReconcile(now int64, createdAfter int64, limit int) ([]UpstreamBillingRecord, error) {
	return findUpstreamBillingRecordsForReconcile(now, createdAfter, limit, 0, nil, false)
}

func FindUpstreamBillingRecordsForCredentialReconcile(now int64, createdAfter int64, limit int, credentialID int) ([]UpstreamBillingRecord, error) {
	if credentialID <= 0 {
		return nil, errors.New("upstream billing account ID is required")
	}
	channelIDs, err := ListChannelIDsUsingUpstreamBillingAccount(credentialID)
	if err != nil {
		return nil, err
	}
	return findUpstreamBillingRecordsForReconcile(now, createdAfter, limit, credentialID, channelIDs, true)
}

func findUpstreamBillingRecordsForReconcile(now int64, createdAfter int64, limit int, credentialID int, legacyChannelIDs []int, force bool) ([]UpstreamBillingRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	records := make([]UpstreamBillingRecord, 0, limit)
	query := DB.Where("created_at >= ?", createdAfter)
	if force {
		statuses := []string{UpstreamBillingStatusPending, UpstreamBillingStatusEstimated, UpstreamBillingStatusFailed, UpstreamBillingStatusExact}
		if len(legacyChannelIDs) > 0 {
			query = query.Where(
				"(credential_id = ? OR ((credential_id = ? OR credential_id IS NULL) AND channel_id IN ?)) AND status IN ?",
				credentialID,
				0,
				legacyChannelIDs,
				statuses,
			)
		} else {
			query = query.Where("credential_id = ? AND status IN ?", credentialID, statuses)
		}
	} else {
		query = query.Where(
			"((status IN ? AND (adjustment_applied = ? OR adjustment_applied IS NULL)) AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR (status = ? AND adjustment_applied = ? AND ((((log_updated = ? OR log_updated IS NULL)) AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR ((next_retry_at IS NULL OR next_retry_at <= ?) AND next_recheck_at > 0 AND next_recheck_at <= ? AND (recheck_until IS NULL OR recheck_until = 0 OR recheck_until >= ?)) OR ((next_retry_at IS NULL OR next_retry_at <= ?) AND (exact_at IS NULL OR exact_at = 0))))",
			[]string{UpstreamBillingStatusPending, UpstreamBillingStatusEstimated, UpstreamBillingStatusFailed, UpstreamBillingStatusExact},
			false,
			now,
			UpstreamBillingStatusExact,
			true,
			false,
			now,
			now,
			now,
			now,
			now,
		)
	}
	err := query.
		Order("id asc").
		Limit(limit).
		Find(&records).Error
	return records, err
}

func HasUpstreamBillingReconcileWork(createdAfter int64) bool {
	var count int64
	now := common.GetTimestamp()
	err := DB.Model(&UpstreamBillingRecord{}).
		Where(
			"created_at >= ? AND (((status IN ? AND (adjustment_applied = ? OR adjustment_applied IS NULL)) AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR (status = ? AND adjustment_applied = ? AND ((((log_updated = ? OR log_updated IS NULL)) AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR ((next_retry_at IS NULL OR next_retry_at <= ?) AND next_recheck_at > 0 AND next_recheck_at <= ? AND (recheck_until IS NULL OR recheck_until = 0 OR recheck_until >= ?)) OR ((next_retry_at IS NULL OR next_retry_at <= ?) AND (exact_at IS NULL OR exact_at = 0)))))",
			createdAfter,
			[]string{UpstreamBillingStatusPending, UpstreamBillingStatusEstimated, UpstreamBillingStatusFailed, UpstreamBillingStatusExact},
			false,
			now,
			UpstreamBillingStatusExact,
			true,
			false,
			now,
			now,
			now,
			now,
			now,
		).
		Limit(1).
		Count(&count).Error
	return err == nil && count > 0
}

type UpstreamBillingAdjustmentInput struct {
	LocalRequestId    string
	UpstreamRequestId string
	IdentityAmbiguous bool
	Provider          string
	UpstreamCostUSD   string
	UpstreamQuota     int64
	ExactQuota        int64
	CurrentCharged    int64
	BillingSource     string
	SubscriptionId    int
	TokenId           int
	IsPlayground      bool
	LogOnly           bool
	LookupAttempts    int
	QuotaData         *QuotaDataLogParams
}

type UpstreamBillingAdjustmentResult struct {
	Applied          bool
	AdjustmentQuota  int64
	WalletAdjustment int64
}

func ApplyUpstreamBillingAdjustment(input UpstreamBillingAdjustmentInput) (UpstreamBillingAdjustmentResult, error) {
	if input.LocalRequestId == "" {
		return UpstreamBillingAdjustmentResult{}, errors.New("local request ID is required")
	}
	if input.ExactQuota < 0 || input.CurrentCharged < 0 {
		return UpstreamBillingAdjustmentResult{}, errors.New("billing quota cannot be negative")
	}

	result := UpstreamBillingAdjustmentResult{}
	userId := 0
	tokenKey := ""
	var cachedQuotaData *QuotaData
	var quotaDataDelta int64
	quotaDataLocked := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var record UpstreamBillingRecord
		if err := lockForUpdate(tx).Where("local_request_id = ?", input.LocalRequestId).First(&record).Error; err != nil {
			return err
		}
		currentCharged := input.CurrentCharged
		if record.AdjustmentApplied {
			currentCharged = record.ChargedQuota
		}

		delta := input.ExactQuota - currentCharged
		walletDelta := delta
		if input.LogOnly {
			walletDelta = 0
		} else {
			userId = record.UserId
		}
		if !input.LogOnly && input.BillingSource == "subscription" {
			if input.SubscriptionId <= 0 {
				return errors.New("subscription ID is required for upstream billing adjustment")
			}
			var subscription UserSubscription
			if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", input.SubscriptionId, record.UserId).First(&subscription).Error; err != nil {
				return err
			}
			subscriptionDelta := delta
			walletDelta = 0
			if delta > 0 && subscription.AmountTotal > 0 {
				remaining := subscription.AmountTotal - subscription.AmountUsed
				if remaining < 0 {
					remaining = 0
				}
				if subscriptionDelta > remaining {
					subscriptionDelta = remaining
					walletDelta = delta - subscriptionDelta
				}
			} else if delta < 0 && record.WalletAdjustment > 0 {
				walletRefund := -delta
				if walletRefund > record.WalletAdjustment {
					walletRefund = record.WalletAdjustment
				}
				walletDelta = -walletRefund
				subscriptionDelta = delta - walletDelta
			}
			newUsed := subscription.AmountUsed + subscriptionDelta
			if newUsed < 0 {
				newUsed = 0
			}
			if err := tx.Model(&UserSubscription{}).Where("id = ?", subscription.Id).Update("amount_used", newUsed).Error; err != nil {
				return err
			}
		}

		if !input.LogOnly && walletDelta != 0 {
			if err := tx.Model(&User{}).Where("id = ?", record.UserId).
				Update("quota", gorm.Expr("quota - ?", walletDelta)).Error; err != nil {
				return err
			}
		}
		if !input.LogOnly && delta != 0 {
			if err := tx.Model(&User{}).Where("id = ?", record.UserId).
				Update("used_quota", gorm.Expr("used_quota + ?", delta)).Error; err != nil {
				return err
			}
			if record.ChannelId > 0 {
				if err := tx.Model(&Channel{}).Where("id = ?", record.ChannelId).
					Update("used_quota", gorm.Expr("used_quota + ?", delta)).Error; err != nil {
					return err
				}
			}
		}
		if !input.LogOnly && delta != 0 && input.TokenId > 0 && !input.IsPlayground {
			var token Token
			if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", input.TokenId, record.UserId).First(&token).Error; err != nil {
				return err
			}
			tokenKey = token.Key
			if token.HasRateLimits() {
				occurredAt := record.RequestStartedAtMs / 1000
				if err := applyTokenRateLimitDelta(&token, delta, occurredAt, false); err != nil {
					return err
				}
			}
			if token.RemainQuota < common.MinQuota+delta || token.RemainQuota > common.MaxQuota+delta ||
				token.UsedQuota > common.MaxQuota-delta || token.UsedQuota < common.MinQuota-delta {
				return errors.New("token quota adjustment exceeds the supported range")
			}
			newRemainQuota := token.RemainQuota - delta
			newUsedQuota := token.UsedQuota + delta
			if newUsedQuota < 0 {
				newUsedQuota = 0
			}
			if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
				"remain_quota":    newRemainQuota,
				"used_quota":      newUsedQuota,
				"accessed_time":   common.GetTimestamp(),
				"usage_5h":        token.Usage5h,
				"usage_1d":        token.Usage1d,
				"usage_7d":        token.Usage7d,
				"window_5h_start": token.Window5hStart,
				"window_1d_start": token.Window1dStart,
				"window_7d_start": token.Window7dStart,
			}).Error; err != nil {
				return err
			}
		}

		if common.DataExportEnabled && input.QuotaData != nil && delta != 0 {
			quotaData := &QuotaData{
				UserID:    input.QuotaData.UserID,
				Username:  input.QuotaData.Username,
				ModelName: input.QuotaData.ModelName,
				CreatedAt: input.QuotaData.CreatedAt - (input.QuotaData.CreatedAt % 3600),
				UseGroup:  input.QuotaData.UseGroup,
				TokenID:   input.QuotaData.TokenID,
				ChannelID: input.QuotaData.ChannelID,
				NodeName:  input.QuotaData.NodeName,
			}
			CacheQuotaDataLock.Lock()
			quotaDataLocked = true
			cachedQuotaData = CacheQuotaData[quotaDataCacheKey(quotaData)]

			var storedQuotaData QuotaData
			query := lockForUpdate(tx).Where(
				"user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
				quotaData.UserID,
				quotaData.Username,
				quotaData.ModelName,
				quotaData.CreatedAt,
				quotaData.UseGroup,
				quotaData.TokenID,
				quotaData.ChannelID,
				quotaData.NodeName,
			).First(&storedQuotaData)
			if query.Error != nil && !errors.Is(query.Error, gorm.ErrRecordNotFound) {
				return query.Error
			}

			if cachedQuotaData != nil {
				combinedQuota := storedQuotaData.Quota + cachedQuotaData.Quota
				if delta < 0 && combinedQuota < -delta {
					return errors.New("quota data adjustment would make the aggregate negative")
				}
				quotaDataDelta = delta
			} else if query.Error == nil {
				if delta < 0 && storedQuotaData.Quota < -delta {
					return errors.New("quota data adjustment would make the aggregate negative")
				}
				if err := tx.Model(&QuotaData{}).Where("id = ?", storedQuotaData.Id).
					Update("quota", gorm.Expr("quota + ?", delta)).Error; err != nil {
					return err
				}
			} else {
				quotaData.Count = 1
				quotaData.Quota = input.ExactQuota
				quotaData.TokenUsed = input.QuotaData.TokenUsed
				if err := tx.Create(quotaData).Error; err != nil {
					return err
				}
			}
		}

		now := common.GetTimestamp()
		exactAt := record.ExactAt
		if exactAt == 0 {
			exactAt = now
		}
		adjustmentQuota := delta
		walletAdjustment := walletDelta
		revisionCount := record.RevisionCount
		if record.AdjustmentApplied {
			adjustmentQuota = record.AdjustmentQuota + delta
			walletAdjustment = record.WalletAdjustment + walletDelta
			if delta != 0 {
				revisionCount++
			}
		}
		updates := map[string]interface{}{
			"provider":           input.Provider,
			"status":             UpstreamBillingStatusExact,
			"identity_ambiguous": input.IdentityAmbiguous,
			"upstream_cost_usd":  input.UpstreamCostUSD,
			"upstream_quota":     input.UpstreamQuota,
			"charged_quota":      input.ExactQuota,
			"attempts":           gorm.Expr("attempts + ?", input.LookupAttempts),
			"error":              "",
			"adjustment_applied": true,
			"adjustment_quota":   adjustmentQuota,
			"wallet_adjustment":  walletAdjustment,
			"revision_count":     revisionCount,
			"exact_at":           exactAt,
			"log_updated":        false,
			"next_retry_at":      0,
			"last_retry_at":      now,
			"updated_at":         now,
		}
		if input.UpstreamRequestId != "" {
			updates["upstream_request_id"] = input.UpstreamRequestId
		}
		if err := tx.Model(&UpstreamBillingRecord{}).Where("id = ?", record.Id).Updates(updates).Error; err != nil {
			return err
		}
		result.Applied = !record.AdjustmentApplied || delta != 0
		result.AdjustmentQuota = adjustmentQuota
		result.WalletAdjustment = walletAdjustment
		return nil
	})
	if quotaDataLocked {
		if err == nil && cachedQuotaData != nil {
			cachedQuotaData.Quota += quotaDataDelta
		}
		CacheQuotaDataLock.Unlock()
	}
	if err != nil {
		return UpstreamBillingAdjustmentResult{}, err
	}
	if result.Applied {
		if userId > 0 {
			_ = invalidateUserCache(userId)
		}
		if common.RedisEnabled && tokenKey != "" {
			_ = cacheDeleteToken(tokenKey)
		}
	}
	return result, nil
}

func MarkUpstreamBillingRetry(localRequestId string, retryAfter int64, attempts int, message string) error {
	now := common.GetTimestamp()
	return DB.Model(&UpstreamBillingRecord{}).Where("local_request_id = ?", localRequestId).Updates(map[string]interface{}{
		"retry_count":   gorm.Expr("retry_count + 1"),
		"attempts":      gorm.Expr("attempts + ?", attempts),
		"last_retry_at": now,
		"next_retry_at": now + retryAfter,
		"error":         message,
		"updated_at":    now,
	}).Error
}

func MarkUpstreamBillingLogUpdated(localRequestId string) error {
	return UpdateUpstreamBillingRecord(localRequestId, map[string]interface{}{
		"log_updated": true,
	})
}

type UpstreamBillingChannelStats struct {
	ChannelId      int     `json:"channel_id"`
	Total          int64   `json:"total"`
	Exact          int64   `json:"exact"`
	Estimated      int64   `json:"estimated"`
	Pending        int64   `json:"pending"`
	Failed         int64   `json:"failed"`
	ExactQuota     int64   `json:"exact_quota"`
	EstimatedQuota int64   `json:"estimated_quota"`
	PendingQuota   int64   `json:"pending_quota"`
	Coverage       float64 `json:"coverage" gorm:"-"`
}

type UpstreamBillingStats struct {
	Total          int64                         `json:"total"`
	Exact          int64                         `json:"exact"`
	Estimated      int64                         `json:"estimated"`
	Pending        int64                         `json:"pending"`
	Failed         int64                         `json:"failed"`
	ExactQuota     int64                         `json:"exact_quota"`
	EstimatedQuota int64                         `json:"estimated_quota"`
	PendingQuota   int64                         `json:"pending_quota"`
	Coverage       float64                       `json:"coverage"`
	Channels       []UpstreamBillingChannelStats `json:"channels"`
}

type UpstreamBillingUsageBucket struct {
	Key             string `json:"key"`
	Requests        int    `json:"requests"`
	Exact           int    `json:"exact"`
	TotalTokens     int64  `json:"total_tokens"`
	UpstreamCostUSD string `json:"upstream_cost_usd"`
	MemberChargeUSD string `json:"member_charge_usd"`
}

type UpstreamBillingAccountUsageStats struct {
	CredentialID    int                          `json:"credential_id"`
	Days            int                          `json:"days"`
	From            int64                        `json:"from"`
	To              int64                        `json:"to"`
	Requests        int                          `json:"requests"`
	Exact           int                          `json:"exact"`
	Estimated       int                          `json:"estimated"`
	Pending         int                          `json:"pending"`
	Failed          int                          `json:"failed"`
	TotalTokens     int64                        `json:"total_tokens"`
	UpstreamCostUSD string                       `json:"upstream_cost_usd"`
	MemberChargeUSD string                       `json:"member_charge_usd"`
	Coverage        float64                      `json:"coverage"`
	LastCheckedAt   int64                        `json:"last_checked_at"`
	Daily           []UpstreamBillingUsageBucket `json:"daily"`
	Models          []UpstreamBillingUsageBucket `json:"models"`
}

func GetUpstreamBillingAccountUsageStats(credentialID int, days int, now time.Time) (UpstreamBillingAccountUsageStats, error) {
	if credentialID <= 0 {
		return UpstreamBillingAccountUsageStats{}, errors.New("upstream billing account ID is required")
	}
	if days < 1 || days > 90 {
		return UpstreamBillingAccountUsageStats{}, errors.New("usage statistics days must be between 1 and 90")
	}
	if now.IsZero() {
		now = time.Now()
	}
	to := now.Unix()
	fromTime := now.AddDate(0, 0, -(days - 1))
	fromTime = time.Date(fromTime.Year(), fromTime.Month(), fromTime.Day(), 0, 0, 0, 0, fromTime.Location())
	from := fromTime.Unix()
	stats := UpstreamBillingAccountUsageStats{
		CredentialID: credentialID,
		Days:         days,
		From:         from,
		To:           to,
		Daily:        make([]UpstreamBillingUsageBucket, 0, days),
		Models:       make([]UpstreamBillingUsageBucket, 0),
	}
	type accumulator struct {
		bucket       UpstreamBillingUsageBucket
		upstreamCost decimal.Decimal
		memberCharge decimal.Decimal
	}
	daily := make(map[string]*accumulator, days)
	for offset := 0; offset < days; offset++ {
		key := fromTime.AddDate(0, 0, offset).Format("2006-01-02")
		daily[key] = &accumulator{bucket: UpstreamBillingUsageBucket{Key: key}}
	}
	models := make(map[string]*accumulator)
	rows, err := DB.Model(&UpstreamBillingRecord{}).
		Select("created_at", "status", "upstream_cost_usd", "charged_quota", "quota_per_unit", "model_name", "total_tokens", "last_checked_at").
		Where("credential_id = ? AND created_at >= ? AND created_at <= ?", credentialID, from, to).
		Order("created_at asc").Rows()
	if err != nil {
		return UpstreamBillingAccountUsageStats{}, err
	}
	defer rows.Close()
	totalUpstreamCost := decimal.Zero
	totalMemberCharge := decimal.Zero
	for rows.Next() {
		var createdAt int64
		var status string
		var upstreamCostText, quotaPerUnitText, modelName sql.NullString
		var chargedQuota, totalTokens, lastCheckedAt sql.NullInt64
		if err := rows.Scan(&createdAt, &status, &upstreamCostText, &chargedQuota, &quotaPerUnitText, &modelName, &totalTokens, &lastCheckedAt); err != nil {
			return UpstreamBillingAccountUsageStats{}, err
		}
		dateKey := time.Unix(createdAt, 0).In(now.Location()).Format("2006-01-02")
		dayBucket, ok := daily[dateKey]
		if !ok {
			continue
		}
		resolvedModelName := strings.TrimSpace(modelName.String)
		if resolvedModelName == "" {
			resolvedModelName = "unknown"
		}
		modelBucket, ok := models[resolvedModelName]
		if !ok {
			modelBucket = &accumulator{bucket: UpstreamBillingUsageBucket{Key: resolvedModelName}}
			models[resolvedModelName] = modelBucket
		}
		stats.Requests++
		stats.TotalTokens += totalTokens.Int64
		dayBucket.bucket.Requests++
		dayBucket.bucket.TotalTokens += totalTokens.Int64
		modelBucket.bucket.Requests++
		modelBucket.bucket.TotalTokens += totalTokens.Int64
		if lastCheckedAt.Int64 > stats.LastCheckedAt {
			stats.LastCheckedAt = lastCheckedAt.Int64
		}
		switch status {
		case UpstreamBillingStatusExact:
			stats.Exact++
			dayBucket.bucket.Exact++
			modelBucket.bucket.Exact++
		case UpstreamBillingStatusEstimated:
			stats.Estimated++
		case UpstreamBillingStatusPending:
			stats.Pending++
		case UpstreamBillingStatusFailed:
			stats.Failed++
		}
		upstreamCost, costErr := decimal.NewFromString(strings.TrimSpace(upstreamCostText.String))
		if costErr == nil && !upstreamCost.IsNegative() {
			totalUpstreamCost = totalUpstreamCost.Add(upstreamCost)
			dayBucket.upstreamCost = dayBucket.upstreamCost.Add(upstreamCost)
			modelBucket.upstreamCost = modelBucket.upstreamCost.Add(upstreamCost)
		}
		quotaPerUnit, quotaErr := decimal.NewFromString(strings.TrimSpace(quotaPerUnitText.String))
		if quotaErr == nil && quotaPerUnit.IsPositive() && chargedQuota.Int64 >= 0 {
			memberCharge := decimal.NewFromInt(chargedQuota.Int64).Div(quotaPerUnit)
			totalMemberCharge = totalMemberCharge.Add(memberCharge)
			dayBucket.memberCharge = dayBucket.memberCharge.Add(memberCharge)
			modelBucket.memberCharge = modelBucket.memberCharge.Add(memberCharge)
		}
	}
	if err := rows.Err(); err != nil {
		return UpstreamBillingAccountUsageStats{}, err
	}
	if stats.Requests > 0 {
		stats.Coverage = float64(stats.Exact) / float64(stats.Requests)
	}
	stats.UpstreamCostUSD = totalUpstreamCost.String()
	stats.MemberChargeUSD = totalMemberCharge.String()
	for offset := 0; offset < days; offset++ {
		key := fromTime.AddDate(0, 0, offset).Format("2006-01-02")
		item := daily[key]
		item.bucket.UpstreamCostUSD = item.upstreamCost.String()
		item.bucket.MemberChargeUSD = item.memberCharge.String()
		stats.Daily = append(stats.Daily, item.bucket)
	}
	for _, item := range models {
		item.bucket.UpstreamCostUSD = item.upstreamCost.String()
		item.bucket.MemberChargeUSD = item.memberCharge.String()
		stats.Models = append(stats.Models, item.bucket)
	}
	sort.Slice(stats.Models, func(i, j int) bool {
		if stats.Models[i].Requests == stats.Models[j].Requests {
			return stats.Models[i].Key < stats.Models[j].Key
		}
		return stats.Models[i].Requests > stats.Models[j].Requests
	})
	return stats, nil
}

func GetUpstreamBillingStats() (UpstreamBillingStats, error) {
	type groupedRow struct {
		ChannelId int
		Status    string
		Count     int64
		Quota     int64
	}
	rows := make([]groupedRow, 0)
	err := DB.Model(&UpstreamBillingRecord{}).
		Select("channel_id, status, count(*) as count, coalesce(sum(charged_quota), 0) as quota").
		Group("channel_id, status").
		Scan(&rows).Error
	if err != nil {
		return UpstreamBillingStats{}, err
	}

	stats := UpstreamBillingStats{Channels: make([]UpstreamBillingChannelStats, 0)}
	byChannel := make(map[int]*UpstreamBillingChannelStats)
	for _, row := range rows {
		channelStats := byChannel[row.ChannelId]
		if channelStats == nil {
			channelStats = &UpstreamBillingChannelStats{ChannelId: row.ChannelId}
			byChannel[row.ChannelId] = channelStats
		}
		channelStats.Total += row.Count
		stats.Total += row.Count
		switch row.Status {
		case UpstreamBillingStatusExact:
			channelStats.Exact += row.Count
			channelStats.ExactQuota += row.Quota
			stats.Exact += row.Count
			stats.ExactQuota += row.Quota
		case UpstreamBillingStatusEstimated:
			channelStats.Estimated += row.Count
			channelStats.EstimatedQuota += row.Quota
			stats.Estimated += row.Count
			stats.EstimatedQuota += row.Quota
		case UpstreamBillingStatusPending:
			channelStats.Pending += row.Count
			channelStats.PendingQuota += row.Quota
			stats.Pending += row.Count
			stats.PendingQuota += row.Quota
		case UpstreamBillingStatusFailed:
			channelStats.Failed += row.Count
			channelStats.EstimatedQuota += row.Quota
			stats.Failed += row.Count
			stats.EstimatedQuota += row.Quota
		}
	}
	for _, channelStats := range byChannel {
		if channelStats.Total > 0 {
			channelStats.Coverage = float64(channelStats.Exact) / float64(channelStats.Total)
		}
		stats.Channels = append(stats.Channels, *channelStats)
	}
	sort.Slice(stats.Channels, func(i, j int) bool {
		return stats.Channels[i].ChannelId < stats.Channels[j].ChannelId
	})
	if stats.Total > 0 {
		stats.Coverage = float64(stats.Exact) / float64(stats.Total)
	}
	return stats, nil
}
