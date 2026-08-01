package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Token struct {
	Id                 int            `json:"id"`
	UserId             int            `json:"user_id" gorm:"index"`
	Key                string         `json:"key" gorm:"type:varchar(128);uniqueIndex"`
	Status             int            `json:"status" gorm:"default:1"`
	Name               string         `json:"name" gorm:"index" `
	CreatedTime        int64          `json:"created_time" gorm:"bigint"`
	AccessedTime       int64          `json:"accessed_time" gorm:"bigint"`
	ExpiredTime        int64          `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	RemainQuota        int64          `json:"remain_quota" gorm:"type:bigint;default:0"`
	UnlimitedQuota     bool           `json:"unlimited_quota"`
	ModelLimitsEnabled bool           `json:"model_limits_enabled"`
	ModelLimits        string         `json:"model_limits" gorm:"type:text"`
	AllowIps           *string        `json:"allow_ips" gorm:"default:''"`
	UsedQuota          int64          `json:"used_quota" gorm:"type:bigint;default:0"` // used quota
	RateLimit5h        int64          `json:"rate_limit_5h" gorm:"column:rate_limit_5h;type:bigint;default:0"`
	RateLimit1d        int64          `json:"rate_limit_1d" gorm:"column:rate_limit_1d;type:bigint;default:0"`
	RateLimit7d        int64          `json:"rate_limit_7d" gorm:"column:rate_limit_7d;type:bigint;default:0"`
	Usage5h            int64          `json:"usage_5h" gorm:"column:usage_5h;type:bigint;default:0"`
	Usage1d            int64          `json:"usage_1d" gorm:"column:usage_1d;type:bigint;default:0"`
	Usage7d            int64          `json:"usage_7d" gorm:"column:usage_7d;type:bigint;default:0"`
	Window5hStart      int64          `json:"window_5h_start" gorm:"column:window_5h_start;type:bigint;default:0"`
	Window1dStart      int64          `json:"window_1d_start" gorm:"column:window_1d_start;type:bigint;default:0"`
	Window7dStart      int64          `json:"window_7d_start" gorm:"column:window_7d_start;type:bigint;default:0"`
	Reset5hAt          int64          `json:"reset_5h_at" gorm:"-"`
	Reset1dAt          int64          `json:"reset_1d_at" gorm:"-"`
	Reset7dAt          int64          `json:"reset_7d_at" gorm:"-"`
	Group              string         `json:"group" gorm:"default:''"`
	CrossGroupRetry    bool           `json:"cross_group_retry"` // 跨分组重试，仅auto分组有效
	AutoGroups         string         `json:"-" gorm:"type:text"`
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

const (
	TokenRateLimitWindow5h = "5h"
	TokenRateLimitWindow1d = "1d"
	TokenRateLimitWindow7d = "7d"

	tokenRateLimitDuration5h = int64(5 * 60 * 60)
	tokenRateLimitDuration1d = int64(24 * 60 * 60)
	tokenRateLimitDuration7d = int64(7 * 24 * 60 * 60)
)

type TokenRateLimitError struct {
	Window    string
	Limit     int64
	Used      int64
	Requested int64
	ResetAt   int64
}

func (e *TokenRateLimitError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("API key %s quota exceeded: used=%d, requested=%d, limit=%d, resets_at=%d",
		e.Window, e.Used, e.Requested, e.Limit, e.ResetAt)
}

func (token *Token) HasRateLimits() bool {
	return token != nil && (token.RateLimit5h > 0 || token.RateLimit1d > 0 || token.RateLimit7d > 0)
}

func (token *Token) PrepareRateLimitView(now int64) {
	if token == nil {
		return
	}
	prepareTokenRateLimitWindowView(&token.Usage5h, &token.Window5hStart, &token.Reset5hAt, token.RateLimit5h, tokenRateLimitDuration5h, now)
	prepareTokenRateLimitWindowView(&token.Usage1d, &token.Window1dStart, &token.Reset1dAt, token.RateLimit1d, tokenRateLimitDuration1d, now)
	prepareTokenRateLimitWindowView(&token.Usage7d, &token.Window7dStart, &token.Reset7dAt, token.RateLimit7d, tokenRateLimitDuration7d, now)
}

func prepareTokenRateLimitWindowView(usage *int64, start *int64, resetAt *int64, limit int64, duration int64, now int64) {
	if limit <= 0 || *start <= 0 || now >= *start+duration {
		*usage = 0
		*start = 0
		*resetAt = 0
		return
	}
	*resetAt = *start + duration
}

func (token *Token) GetAutoGroups() ([]string, error) {
	if token.AutoGroups == "" {
		return nil, nil
	}
	var groups []string
	if err := common.UnmarshalJsonStr(token.AutoGroups, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (token *Token) SetAutoGroups(groups []string) error {
	if len(groups) == 0 {
		token.AutoGroups = ""
		return nil
	}
	data, err := common.Marshal(groups)
	if err != nil {
		return err
	}
	token.AutoGroups = string(data)
	return nil
}

func (token *Token) Clean() {
	token.Key = ""
}

func MaskTokenKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return strings.Repeat("*", len(key))
	}
	if len(key) <= 8 {
		return key[:2] + "****" + key[len(key)-2:]
	}
	return key[:4] + "**********" + key[len(key)-4:]
}

func (token *Token) GetFullKey() string {
	return token.Key
}

func (token *Token) GetMaskedKey() string {
	return MaskTokenKey(token.Key)
}

func (token *Token) GetIpLimits() []string {
	// delete empty spaces
	//split with \n
	ipLimits := make([]string, 0)
	if token.AllowIps == nil {
		return ipLimits
	}
	cleanIps := strings.ReplaceAll(*token.AllowIps, " ", "")
	if cleanIps == "" {
		return ipLimits
	}
	ips := strings.Split(cleanIps, "\n")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		ip = strings.ReplaceAll(ip, ",", "")
		if ip != "" {
			ipLimits = append(ipLimits, ip)
		}
	}
	return ipLimits
}

func GetAllUserTokens(userId int, startIdx int, num int) ([]*Token, error) {
	var tokens []*Token
	var err error
	err = DB.Where("user_id = ?", userId).Order("id desc").Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, err
}

// sanitizeLikePattern 校验并清洗用户输入的 LIKE 搜索模式。
// 规则：
//  1. 转义 ! 和 _（使用 ! 作为 ESCAPE 字符，兼容 MySQL/PostgreSQL/SQLite）
//  2. 连续的 % 合并为单个 %
//  3. 最多允许 2 个 %
//  4. 含 % 时（模糊搜索），去掉 % 后关键词长度必须 >= 2
//  5. 不含 % 时按精确匹配
func sanitizeLikePattern(input string) (string, error) {
	// 1. 先转义 ESCAPE 字符 ! 自身，再转义 _
	//    使用 ! 而非 \ 作为 ESCAPE 字符，避免 MySQL 中反斜杠的字符串转义问题
	input = strings.ReplaceAll(input, "!", "!!")
	input = strings.ReplaceAll(input, `_`, `!_`)

	if err := validateLikePattern(input); err != nil {
		return "", err
	}

	// 5. 无 % 时，精确全匹配
	return input, nil
}

func validateLikePattern(input string) error {
	// 1. 连续的 % 直接拒绝
	if strings.Contains(input, "%%") {
		return errors.New("搜索模式中不允许包含连续的 % 通配符")
	}

	// 2. 统计 % 数量，不得超过 2
	count := strings.Count(input, "%")
	if count > 2 {
		return errors.New("搜索模式中最多允许包含 2 个 % 通配符")
	}

	// 3. 含 % 时，去掉 % 后关键词长度必须 >= 2
	if count > 0 {
		stripped := strings.ReplaceAll(input, "%", "")
		if len(stripped) < 2 {
			return errors.New("使用模糊搜索时，关键词长度至少为 2 个字符")
		}
	}

	return nil
}

const searchHardLimit = 100

func SearchUserTokens(userId int, keyword string, token string, offset int, limit int) (tokens []*Token, total int64, err error) {
	// model 层强制截断
	if limit <= 0 || limit > searchHardLimit {
		limit = searchHardLimit
	}
	if offset < 0 {
		offset = 0
	}

	if token != "" {
		token = strings.TrimPrefix(token, "sk-")
	}

	// 超量用户（令牌数超过上限）只允许精确搜索，禁止模糊搜索
	maxTokens := operation_setting.GetMaxUserTokens()
	hasFuzzy := strings.Contains(keyword, "%") || strings.Contains(token, "%")
	if hasFuzzy {
		count, err := CountUserTokens(userId)
		if err != nil {
			common.SysLog("failed to count user tokens: " + err.Error())
			return nil, 0, errors.New("获取令牌数量失败")
		}
		if int(count) > maxTokens {
			return nil, 0, errors.New("令牌数量超过上限，仅允许精确搜索，请勿使用 % 通配符")
		}
	}

	baseQuery := DB.Model(&Token{}).Where("user_id = ?", userId)

	// 非空才加 LIKE 条件，空则跳过（不过滤该字段）
	if keyword != "" {
		keywordPattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where("name LIKE ? ESCAPE '!'", keywordPattern)
	}
	if token != "" {
		tokenPattern, err := sanitizeLikePattern(token)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where(commonKeyCol+" LIKE ? ESCAPE '!'", tokenPattern)
	}

	// 先查匹配总数（用于分页，受 maxTokens 上限保护，避免全表 COUNT）
	err = baseQuery.Limit(maxTokens).Count(&total).Error
	if err != nil {
		common.SysError("failed to count search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}

	// 再分页查数据
	err = baseQuery.Order("id desc").Offset(offset).Limit(limit).Find(&tokens).Error
	if err != nil {
		common.SysError("failed to search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}
	return tokens, total, nil
}

func ValidateUserToken(key string) (token *Token, err error) {
	if key == "" {
		return nil, ErrTokenNotProvided
	}
	token, err = GetTokenByKey(key, false)
	if err == nil {
		if token.Status == common.TokenStatusExhausted ||
			token.Status == common.TokenStatusExpired ||
			token.Status != common.TokenStatusEnabled {
			return token, ErrTokenInvalid
		}
		if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExpired
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, ErrTokenInvalid
		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExhausted
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, ErrTokenInvalid
		}
		return token, nil
	}
	common.SysLog("ValidateUserToken: failed to get token: " + err.Error())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenInvalid
	}
	return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
}

func GetTokenByIds(id int, userId int) (*Token, error) {
	if id == 0 || userId == 0 {
		return nil, errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	var err error = nil
	err = DB.First(&token, "id = ? and user_id = ?", id, userId).Error
	return &token, err
}

func GetTokenById(id int) (*Token, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	token := Token{Id: id}
	var err error = nil
	err = DB.First(&token, "id = ?", id).Error
	if shouldUpdateRedis(true, err) {
		gopool.Go(func() {
			if err := cacheSetToken(token); err != nil {
				common.SysLog("failed to update user status cache: " + err.Error())
			}
		})
	}
	return &token, err
}

func GetTokenByKey(key string, fromDB bool) (token *Token, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) && token != nil {
			gopool.Go(func() {
				if err := cacheSetToken(*token); err != nil {
					common.SysLog("failed to update user status cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		// Try Redis first
		token, err := cacheGetTokenByKey(key)
		if err == nil {
			return token, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Where(commonKeyCol+" = ?", key).First(&token).Error
	return token, err
}

func TokenKeyExists(key string) (bool, error) {
	var count int64
	err := DB.Unscoped().Model(&Token{}).Where(&Token{Key: key}).Count(&count).Error
	return count > 0, err
}

func (token *Token) Insert() error {
	var err error
	err = DB.Create(token).Error
	return err
}

func (token *Token) InsertIfKeyAvailable() (bool, error) {
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(token)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (token *Token) Update() (err error) {
	err = DB.Model(token).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota",
		"model_limits_enabled", "model_limits", "allow_ips", "group", "cross_group_retry",
		"rate_limit_5h", "rate_limit_1d", "rate_limit_7d", "usage_5h", "usage_1d", "usage_7d",
		"window_5h_start", "window_1d_start", "window_7d_start", "auto_groups").Updates(token).Error
	if shouldUpdateRedis(true, err) {
		if cacheErr := cacheSetToken(*token); cacheErr != nil {
			common.SysLog("failed to update token cache: " + cacheErr.Error())
			if deleteErr := cacheDeleteToken(token.Key); deleteErr != nil {
				common.SysLog("failed to invalidate token cache after update: " + deleteErr.Error())
			}
		}
	}
	return err
}

func (token *Token) SelectUpdate() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheSetToken(*token)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	// This can update zero values
	return DB.Model(token).Select("accessed_time", "status").Updates(token).Error
}

func (token *Token) Delete() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheDeleteToken(token.Key)
				if err != nil {
					common.SysLog("failed to delete token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Delete(token).Error
	return err
}

func (token *Token) IsModelLimitsEnabled() bool {
	return token.ModelLimitsEnabled
}

func (token *Token) GetModelLimits() []string {
	if token.ModelLimits == "" {
		return []string{}
	}
	return strings.Split(token.ModelLimits, ",")
}

func (token *Token) GetModelLimitsMap() map[string]bool {
	limits := token.GetModelLimits()
	limitsMap := make(map[string]bool)
	for _, limit := range limits {
		limitsMap[limit] = true
	}
	return limitsMap
}

func DisableModelLimits(tokenId int) error {
	token, err := GetTokenById(tokenId)
	if err != nil {
		return err
	}
	token.ModelLimitsEnabled = false
	token.ModelLimits = ""
	return token.Update()
}

func DeleteTokenById(id int, userId int) (err error) {
	// Why we need userId here? In case user want to delete other's token.
	if id == 0 || userId == 0 {
		return errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	err = DB.Where(token).First(&token).Error
	if err != nil {
		return err
	}
	return token.Delete()
}

func IncreaseTokenQuota(tokenId int, key string, quota int64) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheIncrTokenQuota(key, quota)
			if err != nil {
				common.SysLog("failed to increase token quota: " + err.Error())
			}
		})
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, tokenId, quota)
		return nil
	}
	return increaseTokenQuota(tokenId, quota)
}

func increaseTokenQuota(id int, quota int64) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota - ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

func DecreaseTokenQuota(id int, key string, quota int64) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheDecrTokenQuota(key, quota)
			if err != nil {
				common.SysLog("failed to decrease token quota: " + err.Error())
			}
		})
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, -quota)
		return nil
	}
	return decreaseTokenQuota(id, quota)
}

func decreaseTokenQuota(id int, quota int64) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

func DecreaseTokenQuotaWithRateLimit(id int, key string, quota int64, occurredAt int64, enforce bool) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}
	if occurredAt <= 0 {
		occurredAt = common.GetTimestamp()
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx).Where("id = ?", id).First(&token).Error; err != nil {
			return err
		}
		if enforce && !token.UnlimitedQuota && token.RemainQuota < quota {
			return fmt.Errorf("token quota is not enough, token remain quota: %d, need quota: %d", token.RemainQuota, quota)
		}
		if err := applyTokenRateLimitDelta(&token, quota, occurredAt, enforce); err != nil {
			return err
		}
		if token.RemainQuota < common.MinQuota+quota || token.UsedQuota > common.MaxQuota-quota {
			return errors.New("token quota adjustment exceeds the supported range")
		}
		token.RemainQuota -= quota
		token.UsedQuota += quota
		token.AccessedTime = common.GetTimestamp()
		return tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
			"remain_quota":    token.RemainQuota,
			"used_quota":      token.UsedQuota,
			"accessed_time":   token.AccessedTime,
			"usage_5h":        token.Usage5h,
			"usage_1d":        token.Usage1d,
			"usage_7d":        token.Usage7d,
			"window_5h_start": token.Window5hStart,
			"window_1d_start": token.Window1dStart,
			"window_7d_start": token.Window7dStart,
		}).Error
	})
	if err == nil && common.RedisEnabled && key != "" {
		_ = cacheDeleteToken(key)
	}
	return err
}

func IncreaseTokenQuotaWithRateLimit(id int, key string, quota int64, occurredAt int64) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return nil
	}
	if occurredAt <= 0 {
		occurredAt = common.GetTimestamp()
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx).Where("id = ?", id).First(&token).Error; err != nil {
			return err
		}
		if err := applyTokenRateLimitDelta(&token, -quota, occurredAt, false); err != nil {
			return err
		}
		if token.RemainQuota > common.MaxQuota-quota {
			return errors.New("token quota adjustment exceeds the supported range")
		}
		token.RemainQuota += quota
		if quota >= token.UsedQuota {
			token.UsedQuota = 0
		} else {
			token.UsedQuota -= quota
		}
		token.AccessedTime = common.GetTimestamp()
		return tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
			"remain_quota":    token.RemainQuota,
			"used_quota":      token.UsedQuota,
			"accessed_time":   token.AccessedTime,
			"usage_5h":        token.Usage5h,
			"usage_1d":        token.Usage1d,
			"usage_7d":        token.Usage7d,
			"window_5h_start": token.Window5hStart,
			"window_1d_start": token.Window1dStart,
			"window_7d_start": token.Window7dStart,
		}).Error
	})
	if err == nil && common.RedisEnabled && key != "" {
		_ = cacheDeleteToken(key)
	}
	return err
}

func applyTokenRateLimitDelta(token *Token, delta int64, occurredAt int64, enforce bool) error {
	if token == nil || delta == 0 {
		return nil
	}
	windows := []struct {
		name     string
		limit    int64
		usage    *int64
		start    *int64
		duration int64
	}{
		{name: TokenRateLimitWindow5h, limit: token.RateLimit5h, usage: &token.Usage5h, start: &token.Window5hStart, duration: tokenRateLimitDuration5h},
		{name: TokenRateLimitWindow1d, limit: token.RateLimit1d, usage: &token.Usage1d, start: &token.Window1dStart, duration: tokenRateLimitDuration1d},
		{name: TokenRateLimitWindow7d, limit: token.RateLimit7d, usage: &token.Usage7d, start: &token.Window7dStart, duration: tokenRateLimitDuration7d},
	}

	for _, window := range windows {
		if window.limit <= 0 {
			continue
		}
		if *window.start <= 0 || occurredAt >= *window.start+window.duration {
			if delta < 0 {
				continue
			}
			*window.start = occurredAt
			*window.usage = 0
		}
		if occurredAt < *window.start {
			continue
		}
		if delta < 0 {
			refund := -delta
			if refund >= *window.usage {
				*window.usage = 0
			} else {
				*window.usage -= refund
			}
			continue
		}
		if *window.usage > common.MaxQuota-delta {
			return errors.New("token rate limit usage exceeds the supported range")
		}
		nextUsage := *window.usage + delta
		if enforce && nextUsage > window.limit {
			return &TokenRateLimitError{
				Window:    window.name,
				Limit:     window.limit,
				Used:      *window.usage,
				Requested: delta,
				ResetAt:   *window.start + window.duration,
			}
		}
		*window.usage = nextUsage
	}
	return nil
}

func ResetTokenRateLimitUsage(id int, userId int) (*Token, error) {
	var token Token
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", id, userId).First(&token).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		window5hStart := int64(0)
		window1dStart := int64(0)
		window7dStart := int64(0)
		if token.RateLimit5h > 0 {
			window5hStart = now
		}
		if token.RateLimit1d > 0 {
			window1dStart = now
		}
		if token.RateLimit7d > 0 {
			window7dStart = now
		}
		if err := tx.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
			"usage_5h":        0,
			"usage_1d":        0,
			"usage_7d":        0,
			"window_5h_start": window5hStart,
			"window_1d_start": window1dStart,
			"window_7d_start": window7dStart,
		}).Error; err != nil {
			return err
		}
		token.Usage5h = 0
		token.Usage1d = 0
		token.Usage7d = 0
		token.Window5hStart = window5hStart
		token.Window1dStart = window1dStart
		token.Window7dStart = window7dStart
		return nil
	})
	if err == nil && common.RedisEnabled && token.Key != "" {
		_ = cacheDeleteToken(token.Key)
	}
	return &token, err
}

// CountUserTokens returns total number of tokens for the given user, used for pagination
func CountUserTokens(userId int) (int64, error) {
	var total int64
	err := DB.Model(&Token{}).Where("user_id = ?", userId).Count(&total).Error
	return total, err
}

// BatchDeleteTokens 删除指定用户的一组令牌，返回成功删除数量
func BatchDeleteTokens(ids []int, userId int) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids 不能为空！")
	}

	tx := DB.Begin()

	var tokens []Token
	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Find(&tokens).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Delete(&Token{}).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}

	if common.RedisEnabled {
		gopool.Go(func() {
			for _, t := range tokens {
				_ = cacheDeleteToken(t.Key)
			}
		})
	}

	return len(tokens), nil
}

func GetTokenKeysByIds(ids []int, userId int) ([]Token, error) {
	var tokens []Token
	err := DB.Select("id", commonKeyCol).
		Where("user_id = ? AND id IN (?)", userId, ids).
		Find(&tokens).Error
	return tokens, err
}

// InvalidateUserTokensCache 清理指定用户所有令牌在 Redis 中的缓存，
// 配合 InvalidateUserCache 使用，可在用户被禁用/删除时立即阻断其令牌的请求。
// 下一次请求将从数据库重新加载令牌及用户状态，从而立即识别出被禁用的用户。
func InvalidateUserTokensCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	var tokens []Token
	if err := DB.Unscoped().
		Select("id", commonKeyCol).
		Where("user_id = ?", userId).
		Find(&tokens).Error; err != nil {
		return err
	}
	return invalidateTokensCache(tokens)
}

func invalidateTokensCache(tokens []Token) error {
	if !common.RedisEnabled {
		return nil
	}
	var firstErr error
	for _, t := range tokens {
		if t.Key == "" {
			continue
		}
		if err := cacheDeleteToken(t.Key); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
