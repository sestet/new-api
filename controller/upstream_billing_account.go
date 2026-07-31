package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type upstreamBillingAccountInput struct {
	Name                 string                      `json:"name"`
	Provider             dto.UpstreamBillingProvider `json:"provider"`
	APIBaseURL           string                      `json:"api_base_url"`
	AccessToken          string                      `json:"access_token"`
	RefreshToken         string                      `json:"refresh_token"`
	AccessTokenIssuedAt  int64                       `json:"access_token_issued_at"`
	AccessTokenExpiresAt int64                       `json:"access_token_expires_at"`
	UserID               int                         `json:"user_id"`
	Enabled              *bool                       `json:"enabled"`
}

type upstreamBillingAccountResponse struct {
	Id                     int    `json:"id"`
	Name                   string `json:"name"`
	Provider               string `json:"provider"`
	APIBaseURL             string `json:"api_base_url"`
	AccessTokenConfigured  bool   `json:"access_token_configured"`
	RefreshTokenConfigured bool   `json:"refresh_token_configured"`
	AccessTokenIssuedAt    int64  `json:"access_token_issued_at"`
	AccessTokenExpiresAt   int64  `json:"access_token_expires_at"`
	UserID                 int    `json:"user_id"`
	DetectedProvider       string `json:"detected_provider,omitempty"`
	Enabled                bool   `json:"enabled"`
	HealthStatus           string `json:"health_status"`
	HealthError            string `json:"health_error"`
	HealthCheckedAt        int64  `json:"health_checked_at"`
	ChannelCount           int    `json:"channel_count"`
	CreatedAt              int64  `json:"created_at"`
	UpdatedAt              int64  `json:"updated_at"`
}

func upstreamBillingAccountToResponse(account *model.UpstreamBillingAccount) (upstreamBillingAccountResponse, error) {
	channelCount, err := model.CountChannelsUsingUpstreamBillingAccount(account.Id)
	if err != nil {
		return upstreamBillingAccountResponse{}, err
	}
	return buildUpstreamBillingAccountResponse(account, channelCount), nil
}

func buildUpstreamBillingAccountResponse(account *model.UpstreamBillingAccount, channelCount int) upstreamBillingAccountResponse {
	return upstreamBillingAccountResponse{
		Id:                     account.Id,
		Name:                   account.Name,
		Provider:               account.Provider,
		APIBaseURL:             account.APIBaseURL,
		AccessTokenConfigured:  strings.TrimSpace(account.AccessToken) != "",
		RefreshTokenConfigured: strings.TrimSpace(account.RefreshToken) != "",
		AccessTokenIssuedAt:    account.AccessTokenIssuedAt,
		AccessTokenExpiresAt:   account.AccessTokenExpiresAt,
		UserID:                 account.UserID,
		DetectedProvider:       account.DetectedProvider,
		Enabled:                account.Enabled,
		HealthStatus:           model.NormalizeUpstreamBillingAccountHealthStatus(account.HealthStatus),
		HealthError:            account.HealthError,
		HealthCheckedAt:        account.HealthCheckedAt,
		ChannelCount:           channelCount,
		CreatedAt:              account.CreatedAt,
		UpdatedAt:              account.UpdatedAt,
	}
}

func applyUpstreamBillingAccountInput(account *model.UpstreamBillingAccount, input upstreamBillingAccountInput, creating bool) {
	account.Name = input.Name
	account.Provider = string(input.Provider)
	account.APIBaseURL = input.APIBaseURL
	if creating || strings.TrimSpace(input.AccessToken) != "" {
		account.AccessToken = input.AccessToken
	}
	if creating || strings.TrimSpace(input.RefreshToken) != "" {
		account.RefreshToken = input.RefreshToken
	}
	if input.AccessToken != "" || input.RefreshToken != "" {
		account.AccessTokenIssuedAt = input.AccessTokenIssuedAt
		account.AccessTokenExpiresAt = input.AccessTokenExpiresAt
	}
	account.UserID = input.UserID
	if input.Enabled != nil {
		account.Enabled = *input.Enabled
	} else if creating {
		account.Enabled = true
	}
}

func ListUpstreamBillingAccounts(c *gin.Context) {
	accounts, err := model.ListUpstreamBillingAccounts()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	responses := make([]upstreamBillingAccountResponse, 0, len(accounts))
	for index := range accounts {
		response, responseErr := upstreamBillingAccountToResponse(&accounts[index])
		if responseErr != nil {
			common.ApiError(c, responseErr)
			return
		}
		responses = append(responses, response)
	}
	common.ApiSuccess(c, responses)
}

func CreateUpstreamBillingAccount(c *gin.Context) {
	var input upstreamBillingAccountInput
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorMsg(c, "无效的上游账号参数")
		return
	}
	account := &model.UpstreamBillingAccount{}
	applyUpstreamBillingAccountInput(account, input, true)
	if err := model.CreateUpstreamBillingAccount(account); err != nil {
		common.ApiError(c, err)
		return
	}
	response, err := upstreamBillingAccountToResponse(account)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func UpdateUpstreamBillingAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的上游账号 ID")
		return
	}
	var input upstreamBillingAccountInput
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorMsg(c, "无效的上游账号参数")
		return
	}
	account, err := model.GetUpstreamBillingAccountByID(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	previousProvider := account.Provider
	previousAPIBaseURL := account.APIBaseURL
	previousAccessToken := account.AccessToken
	previousRefreshToken := account.RefreshToken
	previousUserID := account.UserID
	applyUpstreamBillingAccountInput(account, input, false)
	if previousProvider != account.Provider {
		account.DetectedProvider = ""
		account.AccessTokenIssuedAt = 0
		account.AccessTokenExpiresAt = 0
	}
	if previousProvider != account.Provider ||
		previousAPIBaseURL != account.APIBaseURL ||
		previousAccessToken != account.AccessToken ||
		previousRefreshToken != account.RefreshToken ||
		previousUserID != account.UserID {
		account.HealthStatus = model.UpstreamBillingAccountHealthUnknown
		account.HealthError = ""
		account.HealthCheckedAt = 0
	}
	if err := model.UpdateUpstreamBillingAccount(account); err != nil {
		common.ApiError(c, err)
		return
	}
	response, err := upstreamBillingAccountToResponse(account)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, response)
}

func DeleteUpstreamBillingAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的上游账号 ID")
		return
	}
	if err := model.DeleteUpstreamBillingAccount(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func TestUpstreamBillingAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的上游账号 ID")
		return
	}
	account, err := model.GetUpstreamBillingAccountByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "上游账号不存在"})
			return
		}
		common.ApiError(c, err)
		return
	}
	settings := account.ToSettings()
	settings.Enabled = true
	channel := &model.Channel{BaseURL: common.GetPointer(account.APIBaseURL)}
	channel.SetOtherSettings(dto.ChannelOtherSettings{UpstreamBilling: settings})
	testContext, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	result, err := service.DetectUpstreamBillingProvider(testContext, channel, settings)
	if err != nil {
		if _, healthErr := model.UpdateUpstreamBillingAccountHealth(account.Id, model.UpstreamBillingAccountHealthError, err.Error()); healthErr != nil {
			common.SysError("failed to update upstream billing account health: " + healthErr.Error())
		}
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	recovered, healthErr := model.UpdateUpstreamBillingAccountHealth(account.Id, model.UpstreamBillingAccountHealthHealthy, "")
	if healthErr != nil {
		common.SysError("failed to update upstream billing account health: " + healthErr.Error())
	} else if recovered {
		createdAfter := common.GetTimestamp() - int64(service.UpstreamBillingReconcileLookback().Seconds())
		if requeueErr := model.RequeueFailedUpstreamBillingRecords(account.Id, createdAfter); requeueErr != nil {
			common.SysError("failed to requeue failed upstream billing records: " + requeueErr.Error())
		}
	}
	result.AccessToken = ""
	result.RefreshToken = ""
	common.ApiSuccess(c, result)
}

func ReconcileUpstreamBillingAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的上游账号 ID")
		return
	}
	if _, err := model.GetUpstreamBillingAccountByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "上游账号不存在"})
			return
		}
		common.ApiError(c, err)
		return
	}
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeUpstreamBillingReconcile, upstreamBillingReconcileTaskPayload{
		CredentialID: id,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "已有上游账单对账任务正在运行或等待中，请稍后再试",
			"data": gin.H{
				"task_id": task.TaskID,
				"status":  task.Status,
			},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "上游账单对账任务已启动",
		"data": gin.H{
			"task_id": task.TaskID,
			"status":  task.Status,
		},
	})
}

func GetUpstreamBillingAccountUsageStats(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的上游账号 ID")
		return
	}
	if _, err := model.GetUpstreamBillingAccountByID(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "上游账号不存在"})
			return
		}
		common.ApiError(c, err)
		return
	}
	days := 30
	if value := strings.TrimSpace(c.Query("days")); value != "" {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsed < 1 || parsed > 90 {
			common.ApiErrorMsg(c, "统计天数必须在 1 到 90 之间")
			return
		}
		days = parsed
	}
	stats, err := model.GetUpstreamBillingAccountUsageStats(id, days, time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}
