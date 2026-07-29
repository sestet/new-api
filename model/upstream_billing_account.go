package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"gorm.io/gorm"
)

const (
	UpstreamBillingAccountHealthUnknown = "unknown"
	UpstreamBillingAccountHealthHealthy = "healthy"
	UpstreamBillingAccountHealthError   = "error"
)

type UpstreamBillingAccount struct {
	Id                   int    `json:"id"`
	Name                 string `json:"name" gorm:"type:varchar(128);index"`
	Provider             string `json:"provider" gorm:"type:varchar(32);index"`
	APIBaseURL           string `json:"api_base_url" gorm:"type:varchar(512)"`
	Proxy                string `json:"-" gorm:"type:varchar(512)"`
	AccessToken          string `json:"-" gorm:"type:text"`
	RefreshToken         string `json:"-" gorm:"type:text"`
	AccessTokenIssuedAt  int64  `json:"access_token_issued_at" gorm:"bigint"`
	AccessTokenExpiresAt int64  `json:"access_token_expires_at" gorm:"bigint;index"`
	UserID               int    `json:"user_id"`
	DetectedProvider     string `json:"detected_provider" gorm:"type:varchar(32)"`
	Enabled              bool   `json:"enabled" gorm:"index"`
	HealthStatus         string `json:"health_status" gorm:"type:varchar(32);index"`
	HealthError          string `json:"health_error" gorm:"type:text"`
	HealthCheckedAt      int64  `json:"health_checked_at" gorm:"bigint"`
	CreatedAt            int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt            int64  `json:"updated_at" gorm:"bigint"`
}

func (account *UpstreamBillingAccount) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if account.CreatedAt == 0 {
		account.CreatedAt = now
	}
	account.UpdatedAt = now
	if account.HealthStatus == "" {
		account.HealthStatus = UpstreamBillingAccountHealthUnknown
	}
	return nil
}

func NormalizeUpstreamBillingAccountHealthStatus(status string) string {
	switch status {
	case UpstreamBillingAccountHealthHealthy, UpstreamBillingAccountHealthError:
		return status
	default:
		return UpstreamBillingAccountHealthUnknown
	}
}

func (account *UpstreamBillingAccount) Validate() error {
	if account == nil {
		return errors.New("upstream billing account is required")
	}
	account.Name = strings.TrimSpace(account.Name)
	account.Provider = strings.TrimSpace(account.Provider)
	account.APIBaseURL = strings.TrimSpace(account.APIBaseURL)
	account.AccessToken = strings.TrimSpace(account.AccessToken)
	account.RefreshToken = strings.TrimSpace(account.RefreshToken)
	account.DetectedProvider = strings.TrimSpace(account.DetectedProvider)
	account.HealthStatus = NormalizeUpstreamBillingAccountHealthStatus(account.HealthStatus)
	account.HealthError = strings.TrimSpace(account.HealthError)
	if account.HealthStatus != UpstreamBillingAccountHealthError {
		account.HealthError = ""
	}
	if account.Name == "" {
		return errors.New("upstream billing account name is required")
	}
	if len(account.Name) > 128 {
		return errors.New("upstream billing account name is too long")
	}
	provider := dto.UpstreamBillingProvider(account.Provider)
	if provider != dto.UpstreamBillingProviderNewAPI && provider != dto.UpstreamBillingProviderSub2API {
		return fmt.Errorf("unsupported upstream billing provider: %s", account.Provider)
	}
	settings := account.ToSettings()
	settings.Enabled = true
	settings.CredentialID = 0
	return settings.Validate()
}

func (account *UpstreamBillingAccount) ToSettings() *dto.UpstreamBillingSettings {
	if account == nil {
		return nil
	}
	return &dto.UpstreamBillingSettings{
		CredentialID:         account.Id,
		Enabled:              account.Enabled,
		Provider:             dto.UpstreamBillingProvider(account.Provider),
		AccessToken:          account.AccessToken,
		RefreshToken:         account.RefreshToken,
		AccessTokenIssuedAt:  account.AccessTokenIssuedAt,
		AccessTokenExpiresAt: account.AccessTokenExpiresAt,
		UserID:               account.UserID,
		APIBaseURL:           account.APIBaseURL,
		DetectedProvider:     dto.UpstreamBillingProvider(account.DetectedProvider),
	}
}

func GetUpstreamBillingAccountByID(id int) (*UpstreamBillingAccount, error) {
	if id <= 0 {
		return nil, errors.New("upstream billing account ID is required")
	}
	var account UpstreamBillingAccount
	if err := DB.Where("id = ?", id).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func ListUpstreamBillingAccounts() ([]UpstreamBillingAccount, error) {
	accounts := make([]UpstreamBillingAccount, 0)
	err := DB.Order("id desc").Find(&accounts).Error
	return accounts, err
}

func ListEnabledUpstreamBillingAccounts() ([]UpstreamBillingAccount, error) {
	accounts := make([]UpstreamBillingAccount, 0)
	err := DB.Where("enabled = ?", true).Order("id asc").Find(&accounts).Error
	return accounts, err
}

func CreateUpstreamBillingAccount(account *UpstreamBillingAccount) error {
	if err := account.Validate(); err != nil {
		return err
	}
	return DB.Create(account).Error
}

func UpdateUpstreamBillingAccount(account *UpstreamBillingAccount) error {
	if account == nil || account.Id <= 0 {
		return errors.New("upstream billing account ID is required")
	}
	if err := account.Validate(); err != nil {
		return err
	}
	account.UpdatedAt = common.GetTimestamp()
	return DB.Model(&UpstreamBillingAccount{}).Where("id = ?", account.Id).Updates(map[string]interface{}{
		"name":                    account.Name,
		"provider":                account.Provider,
		"api_base_url":            account.APIBaseURL,
		"access_token":            account.AccessToken,
		"refresh_token":           account.RefreshToken,
		"access_token_issued_at":  account.AccessTokenIssuedAt,
		"access_token_expires_at": account.AccessTokenExpiresAt,
		"user_id":                 account.UserID,
		"detected_provider":       account.DetectedProvider,
		"enabled":                 account.Enabled,
		"health_status":           account.HealthStatus,
		"health_error":            account.HealthError,
		"health_checked_at":       account.HealthCheckedAt,
		"updated_at":              account.UpdatedAt,
	}).Error
}

func UpdateUpstreamBillingAccountHealth(id int, status string, message string) (bool, error) {
	if id <= 0 {
		return false, errors.New("upstream billing account ID is required")
	}
	if status != UpstreamBillingAccountHealthUnknown &&
		status != UpstreamBillingAccountHealthHealthy &&
		status != UpstreamBillingAccountHealthError {
		return false, fmt.Errorf("unsupported upstream billing account health status: %s", status)
	}
	message = strings.TrimSpace(message)
	if status != UpstreamBillingAccountHealthError {
		message = ""
	}
	recovered := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var account UpstreamBillingAccount
		if err := lockForUpdate(tx).Select("health_status").Where("id = ?", id).First(&account).Error; err != nil {
			return err
		}
		recovered = status == UpstreamBillingAccountHealthHealthy &&
			NormalizeUpstreamBillingAccountHealthStatus(account.HealthStatus) == UpstreamBillingAccountHealthError
		return tx.Model(&UpstreamBillingAccount{}).Where("id = ?", id).Updates(map[string]interface{}{
			"health_status":     status,
			"health_error":      message,
			"health_checked_at": common.GetTimestamp(),
		}).Error
	})
	return recovered, err
}

func RequeueFailedUpstreamBillingRecords(credentialID int, createdAfter int64) error {
	if credentialID <= 0 {
		return errors.New("upstream billing account ID is required")
	}
	now := common.GetTimestamp()
	query := DB.Model(&UpstreamBillingRecord{}).
		Where("credential_id = ? AND status = ?", credentialID, UpstreamBillingStatusFailed)
	if createdAfter > 0 {
		query = query.Where("created_at >= ?", createdAfter)
	}
	return query.Updates(map[string]interface{}{
		"next_retry_at": now,
		"updated_at":    now,
	}).Error
}

func DeleteUpstreamBillingAccount(id int) error {
	if id <= 0 {
		return errors.New("upstream billing account ID is required")
	}
	count, err := CountChannelsUsingUpstreamBillingAccount(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("upstream billing account is used by %d channel(s)", count)
	}
	return DB.Delete(&UpstreamBillingAccount{}, id).Error
}

func CountChannelsUsingUpstreamBillingAccount(id int) (int, error) {
	channelIDs, err := ListChannelIDsUsingUpstreamBillingAccount(id)
	return len(channelIDs), err
}

func ListChannelIDsUsingUpstreamBillingAccount(id int) ([]int, error) {
	if id <= 0 {
		return nil, errors.New("upstream billing account ID is required")
	}
	var channels []Channel
	if err := DB.Select("id", "settings").Find(&channels).Error; err != nil {
		return nil, err
	}
	channelIDs := make([]int, 0)
	for index := range channels {
		settings := channels[index].GetOtherSettings().UpstreamBilling
		if settings != nil && settings.CredentialID == id {
			channelIDs = append(channelIDs, channels[index].Id)
		}
	}
	return channelIDs, nil
}

func ResolveChannelUpstreamBillingSettings(channel *Channel) (*dto.UpstreamBillingSettings, error) {
	if channel == nil {
		return nil, errors.New("channel is required")
	}
	channelSettings := channel.GetOtherSettings().UpstreamBilling
	if channelSettings == nil {
		return nil, nil
	}
	resolved := *channelSettings
	if channelSettings.CredentialID <= 0 {
		return &resolved, nil
	}
	account, err := GetUpstreamBillingAccountByID(channelSettings.CredentialID)
	if err != nil {
		return nil, err
	}
	accountSettings := account.ToSettings()
	accountSettings.Enabled = channelSettings.Enabled && account.Enabled
	accountSettings.RecheckEnabled = channelSettings.RecheckEnabled
	accountSettings.RecheckWindowHours = channelSettings.RecheckWindowHours
	accountSettings.UpstreamTokenID = channelSettings.UpstreamTokenID
	accountSettings.UpstreamTokenName = channelSettings.UpstreamTokenName
	return accountSettings, nil
}

func UpdateUpstreamBillingAccountCredentials(id int, update func(*dto.UpstreamBillingSettings) (bool, error)) (*dto.UpstreamBillingSettings, error) {
	if id <= 0 {
		return nil, errors.New("upstream billing account ID is required")
	}
	if update == nil {
		return nil, errors.New("upstream billing credential update is required")
	}
	var result *dto.UpstreamBillingSettings
	err := DB.Transaction(func(tx *gorm.DB) error {
		var account UpstreamBillingAccount
		if err := lockForUpdate(tx).Where("id = ?", id).First(&account).Error; err != nil {
			return err
		}
		settings := account.ToSettings()
		changed, err := update(settings)
		if err != nil {
			return err
		}
		if changed {
			if err := tx.Model(&UpstreamBillingAccount{}).Where("id = ?", id).Updates(map[string]interface{}{
				"access_token":            settings.AccessToken,
				"refresh_token":           settings.RefreshToken,
				"access_token_issued_at":  settings.AccessTokenIssuedAt,
				"access_token_expires_at": settings.AccessTokenExpiresAt,
				"detected_provider":       string(settings.DetectedProvider),
				"updated_at":              common.GetTimestamp(),
			}).Error; err != nil {
				return err
			}
		}
		copySettings := *settings
		result = &copySettings
		return nil
	})
	return result, err
}
