package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type upstreamBillingDetectionRequest struct {
	ChannelID int                         `json:"channel_id"`
	BaseURL   *string                     `json:"base_url"`
	Proxy     *string                     `json:"proxy"`
	Settings  dto.UpstreamBillingSettings `json:"settings"`
}

func DetectChannelUpstreamCostRate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的渠道 ID")
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	detectContext, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	result, err := service.DetectAndStoreUpstreamCostRate(detectContext, channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiSuccess(c, result)
}

func DetectChannelUpstreamBilling(c *gin.Context) {
	var request upstreamBillingDetectionRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "无效的上游账单检测参数")
		return
	}
	request.Settings.NormalizeCredentialTypes()
	if request.ChannelID <= 0 && request.Settings.RefreshToken != "" && request.Settings.Provider != dto.UpstreamBillingProviderNewAPI {
		common.ApiErrorMsg(c, "请先保存渠道再测试 Sub2API；测试会轮换并使旧 refresh token 失效")
		return
	}
	var channel *model.Channel
	var err error
	if request.ChannelID > 0 {
		channel, err = model.GetChannelById(request.ChannelID, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		channel = &model.Channel{}
	}
	if request.BaseURL != nil {
		channel.BaseURL = common.GetPointer(strings.TrimSpace(*request.BaseURL))
	}
	if request.Proxy != nil {
		setting := channel.GetSetting()
		setting.Proxy = strings.TrimSpace(*request.Proxy)
		channel.SetSetting(setting)
	}

	detectContext, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	result, err := service.DetectUpstreamBillingProvider(detectContext, channel, &request.Settings)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiSuccess(c, result)
}
