package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const (
	upstreamBillingLookupAttempts = 3
	upstreamBillingLookupTimeout  = 15 * time.Second
	upstreamBillingReconcileBatch = 20
	upstreamBillingReconcileDays  = 30
	upstreamBillingTimeSlack      = 5 * time.Second
	sub2APIUsagePageSize          = 100
	sub2APIMaxUsagePages          = 100
	sub2APIMinRefreshAhead        = 5 * time.Minute
	sub2APIMaxRefreshAhead        = 6 * time.Hour
)

var (
	errUpstreamBillingRecordNotAvailable = errors.New("upstream billing record is not available yet")
	detectedUpstreamBillingProviders     sync.Map
	newAPIQuotaPerUnitCache              sync.Map
	upstreamBillingCredentialLocks       sync.Map
)

type newAPIQuotaCacheEntry struct {
	value     decimal.Decimal
	expiresAt time.Time
}

type upstreamBillingLookupResult struct {
	Provider          dto.UpstreamBillingProvider
	UpstreamRequestID string
	CostUSD           decimal.Decimal
	UpstreamQuota     int64
	IdentityAmbiguous bool
}

type upstreamBillingUsageMatch struct {
	ModelName         string
	PromptTokens      int
	CompletionTokens  int
	CreatedAt         int64
	StartedAtMs       int64
	FinishedAtMs      int64
	AllowTimeFallback bool
	CredentialID      int
	LocalRequestID    string
}

type upstreamBillingCandidate struct {
	RequestID     string
	CostUSD       decimal.Decimal
	UpstreamQuota int64
	CreatedAtMs   int64
}

func dedupeUpstreamBillingCandidates(candidates []upstreamBillingCandidate) []upstreamBillingCandidate {
	seen := make(map[string]struct{}, len(candidates))
	unique := make([]upstreamBillingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.RequestID == "" {
			continue
		}
		if _, exists := seen[candidate.RequestID]; exists {
			continue
		}
		seen[candidate.RequestID] = struct{}{}
		unique = append(unique, candidate)
	}
	return unique
}

func (match *upstreamBillingUsageMatch) timeWindowMs() (int64, int64, bool) {
	if match == nil {
		return 0, 0, false
	}
	startedAtMs := match.StartedAtMs
	finishedAtMs := match.FinishedAtMs
	if match.CreatedAt > 0 {
		legacyAtMs := match.CreatedAt * int64(time.Second/time.Millisecond)
		if startedAtMs <= 0 {
			startedAtMs = legacyAtMs
		}
		if finishedAtMs <= 0 {
			finishedAtMs = legacyAtMs
		}
	}
	if startedAtMs <= 0 && finishedAtMs <= 0 {
		return 0, 0, false
	}
	if startedAtMs <= 0 {
		startedAtMs = finishedAtMs
	}
	if finishedAtMs <= 0 {
		finishedAtMs = startedAtMs
	}
	if finishedAtMs < startedAtMs {
		startedAtMs = finishedAtMs
	}
	return startedAtMs, finishedAtMs, true
}

func selectUpstreamBillingTimeCandidates(candidates []upstreamBillingCandidate, match *upstreamBillingUsageMatch) []upstreamBillingCandidate {
	startedAtMs, finishedAtMs, ok := match.timeWindowMs()
	if !ok {
		return candidates
	}
	slackMs := upstreamBillingTimeSlack.Milliseconds()
	completionMatches := make([]upstreamBillingCandidate, 0, len(candidates))
	lifetimeMatches := make([]upstreamBillingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.CreatedAtMs <= 0 {
			continue
		}
		completionDelta := candidate.CreatedAtMs - finishedAtMs
		if completionDelta < 0 {
			completionDelta = -completionDelta
		}
		if completionDelta <= slackMs {
			completionMatches = append(completionMatches, candidate)
			continue
		}
		if candidate.CreatedAtMs >= startedAtMs-slackMs && candidate.CreatedAtMs <= finishedAtMs+slackMs {
			lifetimeMatches = append(lifetimeMatches, candidate)
		}
	}
	if len(completionMatches) > 0 {
		return completionMatches
	}
	return lifetimeMatches
}

func excludeClaimedUpstreamBillingCandidates(candidates []upstreamBillingCandidate, match *upstreamBillingUsageMatch) ([]upstreamBillingCandidate, error) {
	if len(candidates) == 0 || match == nil || match.CredentialID <= 0 || match.LocalRequestID == "" {
		return candidates, nil
	}
	requestIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.RequestID != "" {
			requestIDs = append(requestIDs, candidate.RequestID)
		}
	}
	claimed, err := model.GetClaimedUpstreamBillingRequestIDs(match.CredentialID, match.LocalRequestID, requestIDs)
	if err != nil {
		return nil, err
	}
	available := make([]upstreamBillingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, exists := claimed[candidate.RequestID]; !exists {
			available = append(available, candidate)
		}
	}
	return available, nil
}

func resolveUpstreamBillingCandidates(provider dto.UpstreamBillingProvider, candidates []upstreamBillingCandidate, unavailableMessage string) (upstreamBillingLookupResult, error) {
	if len(candidates) == 0 {
		return upstreamBillingLookupResult{}, fmt.Errorf("%s: %w", provider, errUpstreamBillingRecordNotAvailable)
	}
	if len(candidates) == 1 {
		candidate := candidates[0]
		return upstreamBillingLookupResult{
			Provider:          provider,
			UpstreamRequestID: candidate.RequestID,
			CostUSD:           candidate.CostUSD,
			UpstreamQuota:     candidate.UpstreamQuota,
		}, nil
	}
	cost := candidates[0].CostUSD
	for _, candidate := range candidates[1:] {
		if !candidate.CostUSD.Equal(cost) {
			return upstreamBillingLookupResult{}, fmt.Errorf("%s: %w", unavailableMessage, errUpstreamBillingRecordNotAvailable)
		}
	}
	return upstreamBillingLookupResult{
		Provider:          provider,
		CostUSD:           cost,
		UpstreamQuota:     candidates[0].UpstreamQuota,
		IdentityAmbiguous: true,
	}, nil
}

type sub2APIUsageItem struct {
	RequestID           string          `json:"request_id"`
	Model               string          `json:"model"`
	InputTokens         int             `json:"input_tokens"`
	OutputTokens        int             `json:"output_tokens"`
	CacheCreationTokens int             `json:"cache_creation_tokens"`
	CacheReadTokens     int             `json:"cache_read_tokens"`
	ActualCost          json.RawMessage `json:"actual_cost"`
	CreatedAt           string          `json:"created_at"`
}

type sub2APIUsagePage struct {
	Data struct {
		Items    []sub2APIUsageItem `json:"items"`
		Page     int                `json:"page"`
		PageSize int                `json:"page_size"`
		Pages    int                `json:"pages"`
		Total    int                `json:"total"`
	} `json:"data"`
}

type upstreamBillingHTTPError struct {
	StatusCode int
	Body       string
}

func (e *upstreamBillingHTTPError) Error() string {
	return fmt.Sprintf("upstream billing API returned %d: %s", e.StatusCode, e.Body)
}

type sub2APITokenPair struct {
	AccessToken  string
	RefreshToken string
	IssuedAt     int64
	ExpiresAt    int64
}

type UpstreamBillingDetectionResult struct {
	Provider             dto.UpstreamBillingProvider `json:"provider"`
	APIBaseURL           string                      `json:"api_base_url"`
	QuotaPerUnit         string                      `json:"quota_per_unit,omitempty"`
	AccessToken          string                      `json:"access_token,omitempty"`
	RefreshToken         string                      `json:"refresh_token,omitempty"`
	AccessTokenIssuedAt  int64                       `json:"access_token_issued_at,omitempty"`
	AccessTokenExpiresAt int64                       `json:"access_token_expires_at,omitempty"`
}

type UpstreamCostRateDetectionResult struct {
	Multiplier string                     `json:"multiplier"`
	Source     dto.UpstreamCostRateSource `json:"source"`
	ObservedAt int64                      `json:"observed_at"`
}

func parsePositiveCostRate(raw json.RawMessage) (decimal.Decimal, error) {
	rate, err := parseBillingDecimal(raw)
	if err != nil || !rate.IsPositive() {
		return decimal.Zero, errors.New("upstream cost rate multiplier must be positive")
	}
	return rate, nil
}

func probeSub2APICostRate(ctx context.Context, info *relaycommon.RelayInfo, baseURL string, apiKey string) (decimal.Decimal, error) {
	var response struct {
		Object                  string          `json:"object"`
		SchemaVersion           int             `json:"schema_version"`
		BillingScope            string          `json:"billing_scope"`
		EffectiveRateMultiplier json.RawMessage `json:"effective_rate_multiplier"`
	}
	requestURL := upstreamBillingURL(baseURL, "/v1/sub2api/billing", nil)
	if err := doUpstreamBillingRequest(ctx, info, requestURL, apiKey, 0, &response); err != nil {
		return decimal.Zero, err
	}
	if response.Object != "sub2api.key_billing" || response.SchemaVersion != 1 || response.BillingScope != "token" {
		return decimal.Zero, errors.New("upstream returned an unsupported sub2api billing declaration")
	}
	return parsePositiveCostRate(response.EffectiveRateMultiplier)
}

func probeNewAPIPublicCostRate(ctx context.Context, info *relaycommon.RelayInfo, baseURL string, apiKey string) (decimal.Decimal, error) {
	var response struct {
		Object                  string          `json:"object"`
		SchemaVersion           int             `json:"schema_version"`
		BillingScope            string          `json:"billing_scope"`
		EffectiveRateMultiplier json.RawMessage `json:"effective_rate_multiplier"`
	}
	requestURL := upstreamBillingURL(baseURL, "/v1/new-api/billing", nil)
	if err := doUpstreamBillingRequest(ctx, info, requestURL, apiKey, 0, &response); err != nil {
		return decimal.Zero, err
	}
	if response.Object != "new-api.key_billing" || response.SchemaVersion != 1 || response.BillingScope != "token" {
		return decimal.Zero, errors.New("upstream returned an unsupported new-api billing declaration")
	}
	return parsePositiveCostRate(response.EffectiveRateMultiplier)
}

func probeNewAPIManagementCostRate(ctx context.Context, info *relaycommon.RelayInfo, baseURL string, apiKey string, settings *dto.UpstreamBillingSettings) (decimal.Decimal, error) {
	if settings == nil || strings.TrimSpace(settings.AccessToken) == "" {
		return decimal.Zero, errors.New("new-api management credential is unavailable for rate detection")
	}
	var userResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Group string `json:"group"`
			Role  int    `json:"role"`
		} `json:"data"`
	}
	if err := doUpstreamBillingRequest(ctx, info, upstreamBillingURL(baseURL, "/api/user/self", nil), settings.AccessToken, settings.UserID, &userResponse); err != nil {
		return decimal.Zero, err
	}
	if !userResponse.Success || strings.TrimSpace(userResponse.Data.Group) == "" {
		return decimal.Zero, errors.New("new-api management API did not return the upstream user group")
	}

	key := strings.TrimPrefix(strings.TrimSpace(apiKey), "sk-")
	var tokenResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Group string `json:"group"`
			} `json:"items"`
		} `json:"data"`
	}
	tokenQuery := url.Values{"token": []string{key}, "p": []string{"1"}, "page_size": []string{"2"}}
	if err := doUpstreamBillingRequest(ctx, info, upstreamBillingURL(baseURL, "/api/token/search", tokenQuery), settings.AccessToken, settings.UserID, &tokenResponse); err != nil {
		return decimal.Zero, err
	}
	if !tokenResponse.Success {
		return decimal.Zero, errors.New("new-api management API rejected the channel API key lookup")
	}
	if len(tokenResponse.Data.Items) == 0 {
		return decimal.Zero, errors.New("configured new-api account does not own the channel API key; deploy an upstream version with GET /v1/new-api/billing or use the key owner's root account")
	}
	if len(tokenResponse.Data.Items) > 1 {
		return decimal.Zero, errors.New("new-api management API returned multiple matches for the channel API key")
	}
	usingGroup := strings.TrimSpace(tokenResponse.Data.Items[0].Group)
	if usingGroup == "" {
		usingGroup = strings.TrimSpace(userResponse.Data.Group)
	}
	if usingGroup == "auto" {
		return decimal.Zero, errors.New("new-api auto-group keys do not have one stable upstream rate")
	}
	if userResponse.Data.Role < common.RoleRootUser {
		var logResponse struct {
			Success bool `json:"success"`
			Data    struct {
				Items []struct {
					Group string `json:"group"`
					Other string `json:"other"`
				} `json:"items"`
			} `json:"data"`
		}
		logQuery := url.Values{
			"p":         []string{"1"},
			"page_size": []string{"100"},
			"type":      []string{"2"},
			"group":     []string{usingGroup},
		}
		logErr := doUpstreamBillingRequest(ctx, info, upstreamBillingURL(baseURL, "/api/log/self", logQuery), settings.AccessToken, settings.UserID, &logResponse)
		if logErr == nil && logResponse.Success {
			for _, item := range logResponse.Data.Items {
				if strings.TrimSpace(item.Group) != usingGroup || strings.TrimSpace(item.Other) == "" {
					continue
				}
				var logDetails struct {
					GroupRatio json.RawMessage `json:"group_ratio"`
				}
				if err := common.UnmarshalJsonStr(item.Other, &logDetails); err != nil || len(logDetails.GroupRatio) == 0 {
					continue
				}
				rate, err := parsePositiveCostRate(logDetails.GroupRatio)
				if err == nil {
					return rate, nil
				}
			}
		}
		return decimal.Zero, errors.New("configured new-api account is not root and has no usable billing log for this key group; make one request in the upstream group and retry, keep a manual fallback rate, or deploy GET /v1/new-api/billing upstream")
	}

	var optionResponse struct {
		Success bool `json:"success"`
		Data    []struct {
			Key   string `json:"key"`
			Value any    `json:"value"`
		} `json:"data"`
	}
	if err := doUpstreamBillingRequest(ctx, info, upstreamBillingURL(baseURL, "/api/option/", nil), settings.AccessToken, settings.UserID, &optionResponse); err != nil {
		return decimal.Zero, err
	}
	if !optionResponse.Success {
		return decimal.Zero, errors.New("new-api management API rejected the ratio options request")
	}
	groupRatioJSON := "{}"
	groupGroupRatioJSON := "{}"
	for _, option := range optionResponse.Data {
		switch option.Key {
		case "GroupRatio":
			groupRatioJSON = common.Interface2String(option.Value)
		case "GroupGroupRatio":
			groupGroupRatioJSON = common.Interface2String(option.Value)
		}
	}
	groupRatios := map[string]float64{}
	if err := common.UnmarshalJsonStr(groupRatioJSON, &groupRatios); err != nil {
		return decimal.Zero, fmt.Errorf("invalid upstream GroupRatio: %w", err)
	}
	groupGroupRatios := map[string]map[string]float64{}
	if err := common.UnmarshalJsonStr(groupGroupRatioJSON, &groupGroupRatios); err != nil {
		return decimal.Zero, fmt.Errorf("invalid upstream GroupGroupRatio: %w", err)
	}
	if overrides, ok := groupGroupRatios[userResponse.Data.Group]; ok {
		if rate, exists := overrides[usingGroup]; exists {
			if rate <= 0 {
				return decimal.Zero, errors.New("upstream group override rate must be positive")
			}
			return decimal.NewFromFloat(rate), nil
		}
	}
	rate, ok := groupRatios[usingGroup]
	if !ok || rate <= 0 {
		return decimal.Zero, fmt.Errorf("upstream group rate is unavailable for %s", usingGroup)
	}
	return decimal.NewFromFloat(rate), nil
}

func DetectUpstreamCostRate(ctx context.Context, channel *model.Channel, settings *dto.UpstreamBillingSettings) (UpstreamCostRateDetectionResult, error) {
	if channel == nil || settings == nil {
		return UpstreamCostRateDetectionResult{}, errors.New("channel and upstream billing settings are required")
	}
	if !settings.IsCostRateAuto() {
		return UpstreamCostRateDetectionResult{
			Multiplier: settings.EffectiveCostRate().String(),
			Source:     dto.UpstreamCostRateSourceManual,
			ObservedAt: common.GetTimestamp(),
		}, nil
	}
	baseURL := ""
	if channel.BaseURL != nil {
		baseURL = *channel.BaseURL
	}
	keyBaseURL, err := upstreamBillingBaseURL(baseURL, &dto.UpstreamBillingSettings{})
	if err != nil {
		return UpstreamCostRateDetectionResult{}, err
	}
	resolvedSettings, err := model.ResolveChannelUpstreamBillingSettings(channel)
	if err != nil {
		return UpstreamCostRateDetectionResult{}, err
	}
	if resolvedSettings == nil {
		resolvedSettings = settings
	}
	managementBaseURL, managementBaseErr := upstreamBillingBaseURL(baseURL, resolvedSettings)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelId:      channel.Id,
		ChannelBaseUrl: baseURL,
		ChannelSetting: channel.GetSetting(),
	}}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return UpstreamCostRateDetectionResult{}, errors.New("channel API key is required for upstream rate detection")
	}
	provider := resolvedSettings.Provider
	if resolvedSettings.DetectedProvider != "" {
		provider = resolvedSettings.DetectedProvider
	}
	var detectedRate decimal.Decimal
	var source dto.UpstreamCostRateSource
	for index, rawKey := range keys {
		apiKey := strings.TrimSpace(rawKey)
		if apiKey == "" {
			continue
		}
		var rate decimal.Decimal
		var probeErr error
		switch provider {
		case dto.UpstreamBillingProviderSub2API:
			rate, probeErr = probeSub2APICostRate(ctx, info, keyBaseURL, apiKey)
			source = dto.UpstreamCostRateSourceSub2API
		case dto.UpstreamBillingProviderNewAPI:
			rate, probeErr = probeNewAPIPublicCostRate(ctx, info, keyBaseURL, apiKey)
			if probeErr != nil && managementBaseErr == nil {
				rate, probeErr = probeNewAPIManagementCostRate(ctx, info, managementBaseURL, apiKey, resolvedSettings)
			}
			source = dto.UpstreamCostRateSourceNewAPI
		default:
			rate, probeErr = probeSub2APICostRate(ctx, info, keyBaseURL, apiKey)
			source = dto.UpstreamCostRateSourceSub2API
			if probeErr != nil {
				rate, probeErr = probeNewAPIPublicCostRate(ctx, info, keyBaseURL, apiKey)
				source = dto.UpstreamCostRateSourceNewAPI
			}
			if probeErr != nil && managementBaseErr == nil {
				rate, probeErr = probeNewAPIManagementCostRate(ctx, info, managementBaseURL, apiKey, resolvedSettings)
				source = dto.UpstreamCostRateSourceNewAPI
			}
		}
		if probeErr != nil {
			return UpstreamCostRateDetectionResult{}, fmt.Errorf("failed to detect upstream cost rate for channel key %d: %w", index+1, probeErr)
		}
		if detectedRate.IsZero() {
			detectedRate = rate
			continue
		}
		if !detectedRate.Equal(rate) {
			return UpstreamCostRateDetectionResult{}, errors.New("channel API keys declare different upstream cost rates; split them into separate channels")
		}
	}
	if !detectedRate.IsPositive() {
		return UpstreamCostRateDetectionResult{}, errors.New("upstream cost rate detection returned no usable result")
	}
	return UpstreamCostRateDetectionResult{
		Multiplier: detectedRate.String(),
		Source:     source,
		ObservedAt: common.GetTimestamp(),
	}, nil
}

func DetectAndStoreUpstreamCostRate(ctx context.Context, channel *model.Channel) (UpstreamCostRateDetectionResult, error) {
	if channel == nil || channel.Id <= 0 {
		return UpstreamCostRateDetectionResult{}, errors.New("saved channel is required for upstream rate detection")
	}
	settings := channel.GetOtherSettings().UpstreamBilling
	if settings == nil || !settings.Enabled {
		return UpstreamCostRateDetectionResult{}, errors.New("upstream billing is not enabled for this channel")
	}
	result, detectErr := DetectUpstreamCostRate(ctx, channel, settings)
	_, updateErr := model.UpdateChannelUpstreamBillingCredentials(channel.Id, func(stored *dto.UpstreamBillingSettings) (bool, error) {
		if detectErr != nil {
			message := detectErr.Error()
			if len(message) > 1000 {
				message = message[:1000]
			}
			if stored.CostRateError == message {
				return false, nil
			}
			stored.CostRateError = message
			return true, nil
		}
		stored.CostRateMultiplier = result.Multiplier
		stored.CostRateSource = result.Source
		stored.CostRateUpdatedAt = result.ObservedAt
		stored.CostRateError = ""
		return true, nil
	})
	if updateErr != nil {
		return UpstreamCostRateDetectionResult{}, updateErr
	}
	if detectErr != nil {
		return UpstreamCostRateDetectionResult{}, detectErr
	}
	return result, nil
}

type UpstreamCostRateRefreshSummary struct {
	Scanned int `json:"scanned"`
	Updated int `json:"updated"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

func HasUpstreamCostRateRefreshWork() bool {
	channels, err := model.ListEnabledChannelsForUpstreamBillingCredentials()
	if err != nil {
		return false
	}
	for index := range channels {
		settings := channels[index].GetOtherSettings().UpstreamBilling
		if settings != nil && settings.Enabled && settings.IsCostRateAuto() {
			return true
		}
	}
	return false
}

func RefreshUpstreamCostRates(ctx context.Context, report func(processed, total int)) UpstreamCostRateRefreshSummary {
	channels, err := model.ListEnabledChannelsForUpstreamBillingCredentials()
	if err != nil {
		common.SysError("failed to list channels for upstream cost rate refresh: " + err.Error())
		return UpstreamCostRateRefreshSummary{Failed: 1}
	}
	summary := UpstreamCostRateRefreshSummary{}
	total := len(channels)
	for index := range channels {
		if ctx.Err() != nil {
			break
		}
		settings := channels[index].GetOtherSettings().UpstreamBilling
		if settings == nil || !settings.Enabled || !settings.IsCostRateAuto() {
			summary.Skipped++
			if report != nil {
				report(index+1, total)
			}
			continue
		}
		summary.Scanned++
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		_, probeErr := DetectAndStoreUpstreamCostRate(probeCtx, &channels[index])
		cancel()
		if probeErr != nil {
			summary.Failed++
		} else {
			summary.Updated++
		}
		if report != nil {
			report(index+1, total)
		}
	}
	return summary
}

func upstreamBillingSettings(info *relaycommon.RelayInfo) *dto.UpstreamBillingSettings {
	if info == nil || info.ChannelMeta == nil || !info.SupportsUpstreamBillingReconciliation() {
		return nil
	}
	settings := info.ChannelOtherSettings.UpstreamBilling
	if settings == nil || !settings.Enabled {
		return nil
	}
	return settings
}

// ApplyUpstreamCostRateToEstimatedQuota converts a local cost estimate into
// the upstream account's estimated cost. Exact upstream costs must bypass this
// helper because they already include the upstream account rate.
func ApplyUpstreamCostRateToEstimatedQuota(info *relaycommon.RelayInfo, estimatedQuota int64) int64 {
	settings := upstreamBillingSettings(info)
	if settings == nil {
		return estimatedQuota
	}
	quota, clamp := common.QuotaFromDecimalChecked(
		decimal.NewFromInt(estimatedQuota).Mul(settings.EffectiveCostRate()),
	)
	noteQuotaClamp(info, clamp)
	return quota
}

func upstreamBillingBaseURL(channelBaseURL string, settings *dto.UpstreamBillingSettings) (string, error) {
	baseURL := strings.TrimSpace(settings.APIBaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(channelBaseURL)
	}
	if baseURL == "" {
		return "", errors.New("upstream billing base URL is empty")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("invalid upstream billing base URL")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(parsed.Path, "/v1") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/v1")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func upstreamBillingURL(baseURL string, endpoint string, query url.Values) string {
	result := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(endpoint, "/")
	if len(query) > 0 {
		result += "?" + query.Encode()
	}
	return result
}

func doUpstreamBillingRequest(ctx context.Context, info *relaycommon.RelayInfo, requestURL string, token string, userID int, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Accept", "application/json")
	if userID > 0 {
		req.Header.Set("New-Api-User", strconv.Itoa(userID))
	}

	client := GetHttpClient()
	if info != nil && info.ChannelMeta != nil && info.ChannelSetting.Proxy != "" {
		client, err = GetHttpClientWithProxy(info.ChannelSetting.Proxy)
		if err != nil {
			return err
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer CloseResponseBodyGracefully(resp)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if resp.StatusCode == http.StatusUnauthorized && userID == 0 && strings.Contains(string(body), "New-Api-User header not provided") {
			return errors.New("this upstream new-api version requires the account user ID; configure Upstream Account User ID")
		}
		return &upstreamBillingHTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	return common.DecodeJson(resp.Body, target)
}

func requestSub2APITokenPair(ctx context.Context, info *relaycommon.RelayInfo, baseURL string, refreshToken string) (sub2APITokenPair, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return sub2APITokenPair{}, errors.New("sub2api refresh token is missing")
	}
	body, err := common.Marshal(map[string]string{"refresh_token": refreshToken})
	if err != nil {
		return sub2APITokenPair{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamBillingURL(baseURL, "/api/v1/auth/refresh", nil), bytes.NewReader(body))
	if err != nil {
		return sub2APITokenPair{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := GetHttpClient()
	if info != nil && info.ChannelMeta != nil && info.ChannelSetting.Proxy != "" {
		client, err = GetHttpClientWithProxy(info.ChannelSetting.Proxy)
		if err != nil {
			return sub2APITokenPair{}, err
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return sub2APITokenPair{}, err
	}
	defer CloseResponseBodyGracefully(resp)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return sub2APITokenPair{}, &upstreamBillingHTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(responseBody))}
	}
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int64  `json:"expires_in"`
		} `json:"data"`
	}
	if err := common.DecodeJson(resp.Body, &response); err != nil {
		return sub2APITokenPair{}, err
	}
	if response.Code != 0 {
		return sub2APITokenPair{}, fmt.Errorf("sub2api token refresh failed: %s", strings.TrimSpace(response.Message))
	}
	response.Data.AccessToken = strings.TrimSpace(response.Data.AccessToken)
	response.Data.RefreshToken = strings.TrimSpace(response.Data.RefreshToken)
	if response.Data.AccessToken == "" || response.Data.RefreshToken == "" || response.Data.ExpiresIn <= 0 {
		return sub2APITokenPair{}, errors.New("sub2api token refresh returned incomplete credentials")
	}
	now := common.GetTimestamp()
	return sub2APITokenPair{
		AccessToken:  response.Data.AccessToken,
		RefreshToken: response.Data.RefreshToken,
		IssuedAt:     now,
		ExpiresAt:    now + response.Data.ExpiresIn,
	}, nil
}

func sub2APIAccessTokenNeedsRefresh(settings *dto.UpstreamBillingSettings, now int64) bool {
	if settings == nil || strings.TrimSpace(settings.RefreshToken) == "" {
		return false
	}
	if strings.TrimSpace(settings.AccessToken) == "" || settings.AccessTokenExpiresAt <= 0 {
		return true
	}
	refreshAhead := sub2APIMaxRefreshAhead
	if settings.AccessTokenIssuedAt > 0 && settings.AccessTokenExpiresAt > settings.AccessTokenIssuedAt {
		refreshAhead = time.Duration(settings.AccessTokenExpiresAt-settings.AccessTokenIssuedAt) * time.Second / 4
		if refreshAhead < sub2APIMinRefreshAhead {
			refreshAhead = sub2APIMinRefreshAhead
		}
		if refreshAhead > sub2APIMaxRefreshAhead {
			refreshAhead = sub2APIMaxRefreshAhead
		}
	}
	return now >= settings.AccessTokenExpiresAt-int64(refreshAhead.Seconds())
}

func parseBillingDecimal(raw json.RawMessage) (decimal.Decimal, error) {
	value := strings.TrimSpace(common.JsonRawMessageToString(raw))
	if value == "" {
		return decimal.Zero, errors.New("billing amount is empty")
	}
	amount, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid billing amount %q: %w", value, err)
	}
	if amount.IsNegative() {
		return decimal.Zero, errors.New("upstream billing amount cannot be negative")
	}
	return amount, nil
}

func getNewAPIQuotaPerUnit(ctx context.Context, info *relaycommon.RelayInfo, baseURL string, token string, userID int) (decimal.Decimal, error) {
	if cached, ok := newAPIQuotaPerUnitCache.Load(baseURL); ok {
		entry := cached.(newAPIQuotaCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.value, nil
		}
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			QuotaPerUnit json.RawMessage `json:"quota_per_unit"`
		} `json:"data"`
	}
	if err := doUpstreamBillingRequest(ctx, info, upstreamBillingURL(baseURL, "/api/status", nil), token, userID, &response); err != nil {
		return decimal.Zero, err
	}
	if !response.Success {
		return decimal.Zero, errors.New("new-api status API rejected the request")
	}
	quotaPerUnit, err := parseBillingDecimal(response.Data.QuotaPerUnit)
	if err != nil || !quotaPerUnit.IsPositive() {
		return decimal.Zero, errors.New("new-api quota_per_unit is missing or invalid")
	}
	newAPIQuotaPerUnitCache.Store(baseURL, newAPIQuotaCacheEntry{value: quotaPerUnit, expiresAt: time.Now().Add(10 * time.Minute)})
	return quotaPerUnit, nil
}

func lookupNewAPIBilling(ctx context.Context, info *relaycommon.RelayInfo, baseURL string, token string, userID int, requestID string) (upstreamBillingLookupResult, error) {
	if requestID == "" {
		return upstreamBillingLookupResult{}, errors.New("new-api upstream request ID is missing")
	}
	query := url.Values{
		"p":          []string{"1"},
		"page_size":  []string{"20"},
		"type":       []string{"2"},
		"request_id": []string{requestID},
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Quota     int64  `json:"quota"`
				RequestID string `json:"request_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := doUpstreamBillingRequest(ctx, info, upstreamBillingURL(baseURL, "/api/log/self", query), token, userID, &response); err != nil {
		return upstreamBillingLookupResult{}, err
	}
	if !response.Success {
		return upstreamBillingLookupResult{}, errors.New("new-api billing API rejected the request")
	}
	matches := make([]int64, 0, 1)
	for _, item := range response.Data.Items {
		if item.RequestID == requestID {
			matches = append(matches, item.Quota)
		}
	}
	if len(matches) == 0 {
		return upstreamBillingLookupResult{}, fmt.Errorf("new-api: %w", errUpstreamBillingRecordNotAvailable)
	}
	if len(matches) != 1 {
		return upstreamBillingLookupResult{}, errors.New("new-api billing request ID is not unique")
	}
	if matches[0] < 0 {
		return upstreamBillingLookupResult{}, errors.New("new-api returned a negative quota")
	}
	quotaPerUnit, err := getNewAPIQuotaPerUnit(ctx, info, baseURL, token, userID)
	if err != nil {
		return upstreamBillingLookupResult{}, err
	}
	cost := decimal.NewFromInt(matches[0]).Div(quotaPerUnit)
	return upstreamBillingLookupResult{
		Provider:          dto.UpstreamBillingProviderNewAPI,
		UpstreamRequestID: requestID,
		CostUSD:           cost,
		UpstreamQuota:     matches[0],
	}, nil
}

func lookupNewAPIBillingByUsage(ctx context.Context, info *relaycommon.RelayInfo, baseURL string, token string, userID int, expected *upstreamBillingUsageMatch) (upstreamBillingLookupResult, error) {
	startedAtMs, finishedAtMs, hasTimeWindow := expected.timeWindowMs()
	if expected == nil || expected.ModelName == "" || !hasTimeWindow || expected.PromptTokens < 0 || expected.CompletionTokens < 0 {
		return upstreamBillingLookupResult{}, fmt.Errorf("new-api: %w", errUpstreamBillingRecordNotAvailable)
	}
	slackMs := upstreamBillingTimeSlack.Milliseconds()
	query := url.Values{
		"p":               []string{"1"},
		"page_size":       []string{"100"},
		"type":            []string{"2"},
		"start_timestamp": []string{strconv.FormatInt((startedAtMs-slackMs)/int64(time.Second/time.Millisecond), 10)},
		"end_timestamp":   []string{strconv.FormatInt((finishedAtMs+slackMs+999)/int64(time.Second/time.Millisecond), 10)},
		"model_name":      []string{expected.ModelName},
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Quota            int64  `json:"quota"`
				RequestID        string `json:"request_id"`
				ModelName        string `json:"model_name"`
				PromptTokens     int    `json:"prompt_tokens"`
				CompletionTokens int    `json:"completion_tokens"`
				CreatedAt        int64  `json:"created_at"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := doUpstreamBillingRequest(ctx, info, upstreamBillingURL(baseURL, "/api/log/self", query), token, userID, &response); err != nil {
		return upstreamBillingLookupResult{}, err
	}
	if !response.Success {
		return upstreamBillingLookupResult{}, errors.New("new-api billing API rejected the request")
	}
	quotaPerUnit, err := getNewAPIQuotaPerUnit(ctx, info, baseURL, token, userID)
	if err != nil {
		return upstreamBillingLookupResult{}, err
	}
	candidates := make([]upstreamBillingCandidate, 0, len(response.Data.Items))
	for _, item := range response.Data.Items {
		if item.RequestID == "" || item.ModelName != expected.ModelName || item.PromptTokens != expected.PromptTokens || item.CompletionTokens != expected.CompletionTokens {
			continue
		}
		if item.Quota < 0 {
			return upstreamBillingLookupResult{}, errors.New("new-api returned a negative quota")
		}
		candidates = append(candidates, upstreamBillingCandidate{
			RequestID:     item.RequestID,
			CostUSD:       decimal.NewFromInt(item.Quota).Div(quotaPerUnit),
			UpstreamQuota: item.Quota,
			CreatedAtMs:   item.CreatedAt * int64(time.Second/time.Millisecond),
		})
	}
	candidates = dedupeUpstreamBillingCandidates(candidates)
	candidates, err = excludeClaimedUpstreamBillingCandidates(candidates, expected)
	if err != nil {
		return upstreamBillingLookupResult{}, err
	}
	candidates = selectUpstreamBillingTimeCandidates(candidates, expected)
	return resolveUpstreamBillingCandidates(dto.UpstreamBillingProviderNewAPI, candidates, "new-api usage signature is not unique")
}

func lookupSub2APIBilling(ctx context.Context, info *relaycommon.RelayInfo, baseURL string, token string, upstreamTokenID string, requestID string, usageMatch *upstreamBillingUsageMatch) (upstreamBillingLookupResult, error) {
	if requestID == "" {
		return upstreamBillingLookupResult{}, errors.New("sub2api request ID is missing")
	}
	query := url.Values{
		"page_size": []string{strconv.Itoa(sub2APIUsagePageSize)},
	}
	if upstreamTokenID != "" {
		query.Set("api_key_id", upstreamTokenID)
	}
	startedAtMs, finishedAtMs, hasTimeWindow := usageMatch.timeWindowMs()
	if usageMatch != nil && usageMatch.ModelName != "" && hasTimeWindow {
		slackMs := upstreamBillingTimeSlack.Milliseconds()
		query.Set("model", usageMatch.ModelName)
		query.Set("start_date", time.UnixMilli(startedAtMs-slackMs).In(time.Local).Format(time.DateOnly))
		query.Set("end_date", time.UnixMilli(finishedAtMs+slackMs).In(time.Local).Format(time.DateOnly))
	} else {
		query.Set("request_id", requestID)
	}
	exactCandidates := make([]upstreamBillingCandidate, 0, 1)
	signatureCandidates := make([]upstreamBillingCandidate, 0, 1)
	timeCandidates := make([]upstreamBillingCandidate, 0, 1)
	totalPages := 1
	for page := 1; page <= totalPages && page <= sub2APIMaxUsagePages; page++ {
		query.Set("page", strconv.Itoa(page))
		var response sub2APIUsagePage
		if err := doUpstreamBillingRequest(ctx, info, upstreamBillingURL(baseURL, "/api/v1/usage", query), token, 0, &response); err != nil {
			return upstreamBillingLookupResult{}, err
		}
		if response.Data.Pages > totalPages {
			totalPages = response.Data.Pages
		}
		for _, item := range response.Data.Items {
			if item.RequestID == "" {
				continue
			}
			if item.RequestID == requestID {
				cost, err := parseBillingDecimal(item.ActualCost)
				if err != nil {
					return upstreamBillingLookupResult{}, err
				}
				createdAtMs := int64(0)
				if createdAt, parseErr := time.Parse(time.RFC3339Nano, item.CreatedAt); parseErr == nil {
					createdAtMs = createdAt.UnixMilli()
				}
				exactCandidates = append(exactCandidates, upstreamBillingCandidate{
					RequestID:   item.RequestID,
					CostUSD:     cost,
					CreatedAtMs: createdAtMs,
				})
				continue
			}
			if usageMatch == nil || usageMatch.ModelName == "" || item.Model != usageMatch.ModelName {
				continue
			}
			createdAt, parseErr := time.Parse(time.RFC3339Nano, item.CreatedAt)
			if parseErr != nil {
				continue
			}
			createdAtMs := createdAt.UnixMilli()
			if hasTimeWindow {
				slackMs := upstreamBillingTimeSlack.Milliseconds()
				if createdAtMs < startedAtMs-slackMs || createdAtMs > finishedAtMs+slackMs {
					continue
				}
			}
			cost, err := parseBillingDecimal(item.ActualCost)
			if err != nil {
				return upstreamBillingLookupResult{}, err
			}
			candidate := upstreamBillingCandidate{
				RequestID:   item.RequestID,
				CostUSD:     cost,
				CreatedAtMs: createdAtMs,
			}
			inputTokens := item.InputTokens + item.CacheCreationTokens + item.CacheReadTokens
			if inputTokens == usageMatch.PromptTokens &&
				item.OutputTokens == usageMatch.CompletionTokens {
				signatureCandidates = append(signatureCandidates, candidate)
				continue
			}
			if usageMatch.AllowTimeFallback {
				timeCandidates = append(timeCandidates, candidate)
			}
		}
	}

	var err error
	exactCandidates = dedupeUpstreamBillingCandidates(exactCandidates)
	exactCandidates, err = excludeClaimedUpstreamBillingCandidates(exactCandidates, usageMatch)
	if err != nil {
		return upstreamBillingLookupResult{}, err
	}
	if len(exactCandidates) > 0 {
		return resolveUpstreamBillingCandidates(dto.UpstreamBillingProviderSub2API, exactCandidates, "sub2api request ID is not unique")
	}

	signatureCandidates = dedupeUpstreamBillingCandidates(signatureCandidates)
	signatureCandidates, err = excludeClaimedUpstreamBillingCandidates(signatureCandidates, usageMatch)
	if err != nil {
		return upstreamBillingLookupResult{}, err
	}
	signatureCandidates = selectUpstreamBillingTimeCandidates(signatureCandidates, usageMatch)
	if len(signatureCandidates) > 0 {
		return resolveUpstreamBillingCandidates(dto.UpstreamBillingProviderSub2API, signatureCandidates, "sub2api billing signature is not unique")
	}

	timeCandidates = dedupeUpstreamBillingCandidates(timeCandidates)
	timeCandidates, err = excludeClaimedUpstreamBillingCandidates(timeCandidates, usageMatch)
	if err != nil {
		return upstreamBillingLookupResult{}, err
	}
	timeCandidates = selectUpstreamBillingTimeCandidates(timeCandidates, usageMatch)
	return resolveUpstreamBillingCandidates(dto.UpstreamBillingProviderSub2API, timeCandidates, "sub2api billing time window is not unique")
}

func resolveSub2APIUpstreamToken(ctx context.Context, info *relaycommon.RelayInfo, baseURL string, token string, settings *dto.UpstreamBillingSettings) (*dto.UpstreamBillingSettings, error) {
	if settings == nil || settings.UpstreamTokenID != "" || info == nil || info.ChannelMeta == nil {
		return settings, nil
	}
	channelKey := strings.TrimSpace(info.ChannelMeta.ApiKey)
	if channelKey == "" || strings.Contains(channelKey, "\n") {
		return settings, nil
	}
	type sub2APIKeyItem struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	var matched *sub2APIKeyItem
	totalPages := 1
	for page := 1; page <= totalPages && page <= sub2APIMaxUsagePages; page++ {
		query := url.Values{
			"page":      []string{strconv.Itoa(page)},
			"page_size": []string{strconv.Itoa(sub2APIUsagePageSize)},
		}
		var response struct {
			Code int `json:"code"`
			Data struct {
				Items []sub2APIKeyItem `json:"items"`
				Pages int              `json:"pages"`
			} `json:"data"`
		}
		if err := doUpstreamBillingRequest(ctx, info, upstreamBillingURL(baseURL, "/api/v1/keys", query), token, 0, &response); err != nil {
			return settings, err
		}
		if response.Code != 0 {
			return settings, errors.New("sub2api API key list rejected the request")
		}
		if response.Data.Pages > totalPages {
			totalPages = response.Data.Pages
		}
		for index := range response.Data.Items {
			if strings.TrimSpace(response.Data.Items[index].Key) == channelKey {
				item := response.Data.Items[index]
				matched = &item
				break
			}
		}
		if matched != nil {
			break
		}
	}
	if matched == nil || matched.ID <= 0 {
		return settings, nil
	}

	resolved := *settings
	resolved.UpstreamTokenID = strconv.FormatInt(matched.ID, 10)
	resolved.UpstreamTokenName = strings.TrimSpace(matched.Name)
	info.ChannelOtherSettings.UpstreamBilling = &resolved
	if info.ChannelId > 0 {
		_, err := model.UpdateChannelUpstreamBillingCredentials(info.ChannelId, func(current *dto.UpstreamBillingSettings) (bool, error) {
			if current.UpstreamTokenID == resolved.UpstreamTokenID && current.UpstreamTokenName == resolved.UpstreamTokenName {
				return false, nil
			}
			current.UpstreamTokenID = resolved.UpstreamTokenID
			current.UpstreamTokenName = resolved.UpstreamTokenName
			return true, nil
		})
		if err != nil {
			common.SysError(fmt.Sprintf("failed to save sub2api API key identity for channel %d: %s", info.ChannelId, err.Error()))
		}
	}
	return &resolved, nil
}

func resolveSub2APIAccessToken(ctx context.Context, info *relaycommon.RelayInfo, settings *dto.UpstreamBillingSettings, force bool, rejectedAccessToken string, preferProvidedRefreshToken bool) (*dto.UpstreamBillingSettings, error) {
	if settings == nil {
		return nil, errors.New("sub2api billing settings are required")
	}
	if info.ChannelId <= 0 && !force && !preferProvidedRefreshToken && !sub2APIAccessTokenNeedsRefresh(settings, common.GetTimestamp()) {
		resolved := *settings
		return &resolved, nil
	}
	refreshSettings := func(current *dto.UpstreamBillingSettings) (bool, error) {
		changed := current.NormalizeCredentialTypes()
		if preferProvidedRefreshToken && strings.TrimSpace(settings.RefreshToken) != "" && settings.RefreshToken != current.RefreshToken {
			current.RefreshToken = strings.TrimSpace(settings.RefreshToken)
			current.RefreshTokenConfigured = false
			current.AccessToken = ""
			current.AccessTokenIssuedAt = 0
			current.AccessTokenExpiresAt = 0
			changed = true
		}
		if force && rejectedAccessToken != "" && current.AccessToken != "" && current.AccessToken != rejectedAccessToken {
			return changed, nil
		}
		if !force && !sub2APIAccessTokenNeedsRefresh(current, common.GetTimestamp()) {
			return changed, nil
		}
		if strings.TrimSpace(current.RefreshToken) == "" {
			if current.AccessToken != "" && !force {
				return changed, nil
			}
			return false, errors.New("sub2api refresh token is required for automatic renewal")
		}
		baseURL, baseErr := upstreamBillingBaseURL(info.ChannelBaseUrl, current)
		if baseErr != nil {
			return false, baseErr
		}
		pair, refreshErr := requestSub2APITokenPair(ctx, info, baseURL, current.RefreshToken)
		if refreshErr != nil {
			return false, refreshErr
		}
		current.AccessToken = pair.AccessToken
		current.AccessTokenConfigured = false
		current.RefreshToken = pair.RefreshToken
		current.RefreshTokenConfigured = false
		current.AccessTokenIssuedAt = pair.IssuedAt
		current.AccessTokenExpiresAt = pair.ExpiresAt
		current.DetectedProvider = dto.UpstreamBillingProviderSub2API
		return true, nil
	}

	if settings.CredentialID > 0 {
		lockValue, _ := upstreamBillingCredentialLocks.LoadOrStore(
			settings.CredentialID,
			&sync.Mutex{},
		)
		credentialLock := lockValue.(*sync.Mutex)
		credentialLock.Lock()
		defer credentialLock.Unlock()
		resolved, err := model.UpdateUpstreamBillingAccountCredentials(settings.CredentialID, refreshSettings)
		if err != nil {
			return nil, err
		}
		if resolved != nil {
			resolved.Enabled = settings.Enabled && resolved.Enabled
			resolved.RecheckEnabled = settings.RecheckEnabled
			resolved.RecheckWindowHours = settings.RecheckWindowHours
			resolved.UpstreamTokenID = settings.UpstreamTokenID
			resolved.UpstreamTokenName = settings.UpstreamTokenName
			info.ChannelOtherSettings.UpstreamBilling = resolved
		}
		return resolved, nil
	}
	if info.ChannelId <= 0 {
		resolved := *settings
		if _, err := refreshSettings(&resolved); err != nil {
			return nil, err
		}
		return &resolved, nil
	}
	resolved, err := model.UpdateChannelUpstreamBillingCredentials(info.ChannelId, refreshSettings)
	if err != nil {
		return nil, err
	}
	if resolved != nil {
		info.ChannelOtherSettings.UpstreamBilling = resolved
	}
	return resolved, nil
}

func isUpstreamBillingUnauthorized(err error) bool {
	var httpErr *upstreamBillingHTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized
}

func upstreamBillingProviderCandidates(channelID int, settings *dto.UpstreamBillingSettings, localRequestID string, upstreamRequestID string) []dto.UpstreamBillingProvider {
	provider := settings.Provider
	if provider == "" {
		provider = dto.UpstreamBillingProviderAuto
	}
	if provider != dto.UpstreamBillingProviderAuto {
		return []dto.UpstreamBillingProvider{provider}
	}
	preferredProvider := dto.UpstreamBillingProvider("")
	if settings.DetectedProvider == dto.UpstreamBillingProviderNewAPI || settings.DetectedProvider == dto.UpstreamBillingProviderSub2API {
		preferredProvider = settings.DetectedProvider
	}
	if preferredProvider == "" && channelID > 0 {
		if cached, ok := detectedUpstreamBillingProviders.Load(channelID); ok {
			if cachedProvider, valid := cached.(dto.UpstreamBillingProvider); valid {
				preferredProvider = cachedProvider
			}
		}
	}
	if preferredProvider == dto.UpstreamBillingProviderNewAPI {
		return []dto.UpstreamBillingProvider{dto.UpstreamBillingProviderNewAPI, dto.UpstreamBillingProviderSub2API}
	}
	if preferredProvider == dto.UpstreamBillingProviderSub2API {
		return []dto.UpstreamBillingProvider{dto.UpstreamBillingProviderSub2API, dto.UpstreamBillingProviderNewAPI}
	}
	if upstreamRequestID != "" && upstreamRequestID != localRequestID {
		return []dto.UpstreamBillingProvider{dto.UpstreamBillingProviderNewAPI, dto.UpstreamBillingProviderSub2API}
	}
	return []dto.UpstreamBillingProvider{dto.UpstreamBillingProviderSub2API, dto.UpstreamBillingProviderNewAPI}
}

func lookupUpstreamBilling(ctx context.Context, info *relaycommon.RelayInfo, settings *dto.UpstreamBillingSettings, localRequestID string, upstreamRequestID string, usageMatch *upstreamBillingUsageMatch) (upstreamBillingLookupResult, int, error) {
	if info != nil && settings != nil && settings.Proxy != "" {
		info.ChannelSetting.Proxy = settings.Proxy
	}
	hasUsageMatch := usageMatch != nil
	if hasUsageMatch {
		usageCopy := *usageMatch
		usageCopy.CredentialID = settings.CredentialID
		usageCopy.LocalRequestID = localRequestID
		usageMatch = &usageCopy
	} else {
		usageMatch = &upstreamBillingUsageMatch{
			CredentialID:   settings.CredentialID,
			LocalRequestID: localRequestID,
		}
	}
	channelID := 0
	if info != nil {
		channelID = info.ChannelId
	}
	candidates := upstreamBillingProviderCandidates(channelID, settings, localRequestID, upstreamRequestID)
	baseURL, err := upstreamBillingBaseURL(info.ChannelBaseUrl, settings)
	if err != nil {
		recordUpstreamBillingAccountHealth(settings, err)
		return upstreamBillingLookupResult{}, 0, err
	}
	var result upstreamBillingLookupResult
	var lookupErr error
	for attempt := 1; attempt <= upstreamBillingLookupAttempts; attempt++ {
		for _, provider := range candidates {
			switch provider {
			case dto.UpstreamBillingProviderNewAPI:
				result, lookupErr = lookupNewAPIBilling(ctx, info, baseURL, settings.AccessToken, settings.UserID, upstreamRequestID)
				if lookupErr == nil && result.UpstreamRequestID != "" {
					available, claimErr := excludeClaimedUpstreamBillingCandidates([]upstreamBillingCandidate{{
						RequestID:     result.UpstreamRequestID,
						CostUSD:       result.CostUSD,
						UpstreamQuota: result.UpstreamQuota,
					}}, usageMatch)
					if claimErr != nil {
						lookupErr = claimErr
					} else if len(available) == 0 {
						lookupErr = fmt.Errorf("new-api: %w", errUpstreamBillingRecordNotAvailable)
					}
				}
				if errors.Is(lookupErr, errUpstreamBillingRecordNotAvailable) && hasUsageMatch {
					result, lookupErr = lookupNewAPIBillingByUsage(ctx, info, baseURL, settings.AccessToken, settings.UserID, usageMatch)
				}
			case dto.UpstreamBillingProviderSub2API:
				resolvedSettings, resolveErr := resolveSub2APIAccessToken(ctx, info, settings, false, "", false)
				if resolveErr != nil {
					lookupErr = resolveErr
					break
				}
				if tokenSettings, tokenErr := resolveSub2APIUpstreamToken(ctx, info, baseURL, resolvedSettings.AccessToken, resolvedSettings); tokenErr == nil {
					resolvedSettings = tokenSettings
				}
				resolvedBaseURL, baseErr := upstreamBillingBaseURL(info.ChannelBaseUrl, resolvedSettings)
				if baseErr != nil {
					lookupErr = baseErr
					break
				}
				result, lookupErr = lookupSub2APIBilling(ctx, info, resolvedBaseURL, resolvedSettings.AccessToken, resolvedSettings.UpstreamTokenID, upstreamRequestID, usageMatch)
				if isUpstreamBillingUnauthorized(lookupErr) && resolvedSettings.RefreshToken != "" {
					rejectedAccessToken := resolvedSettings.AccessToken
					resolvedSettings, resolveErr = resolveSub2APIAccessToken(ctx, info, resolvedSettings, true, rejectedAccessToken, false)
					if resolveErr != nil {
						lookupErr = resolveErr
						break
					}
					resolvedBaseURL, baseErr = upstreamBillingBaseURL(info.ChannelBaseUrl, resolvedSettings)
					if baseErr != nil {
						lookupErr = baseErr
						break
					}
					result, lookupErr = lookupSub2APIBilling(ctx, info, resolvedBaseURL, resolvedSettings.AccessToken, resolvedSettings.UpstreamTokenID, upstreamRequestID, usageMatch)
				}
			default:
				lookupErr = fmt.Errorf("unsupported upstream billing provider: %s", provider)
			}
			if lookupErr == nil {
				if channelID > 0 {
					detectedUpstreamBillingProviders.Store(channelID, result.Provider)
				}
				recordUpstreamBillingAccountHealth(settings, nil)
				return result, attempt, nil
			}
			if errors.Is(lookupErr, errUpstreamBillingRecordNotAvailable) {
				candidates = []dto.UpstreamBillingProvider{provider}
				break
			}
		}
		if attempt < upstreamBillingLookupAttempts {
			select {
			case <-ctx.Done():
				recordUpstreamBillingAccountHealth(settings, ctx.Err())
				return upstreamBillingLookupResult{}, attempt, ctx.Err()
			case <-time.After(time.Duration(attempt) * 150 * time.Millisecond):
			}
		}
	}
	if !errors.Is(lookupErr, errUpstreamBillingRecordNotAvailable) {
		recordUpstreamBillingAccountHealth(settings, lookupErr)
	}
	return upstreamBillingLookupResult{}, upstreamBillingLookupAttempts, lookupErr
}

func recordUpstreamBillingAccountHealth(settings *dto.UpstreamBillingSettings, healthErr error) {
	if settings == nil || settings.CredentialID <= 0 {
		return
	}
	status := model.UpstreamBillingAccountHealthHealthy
	message := ""
	if healthErr != nil {
		status = model.UpstreamBillingAccountHealthError
		message = healthErr.Error()
	}
	recovered, err := model.UpdateUpstreamBillingAccountHealth(settings.CredentialID, status, message)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to update upstream billing account %d health: %s", settings.CredentialID, err.Error()))
		return
	}
	if recovered {
		createdAfter := common.GetTimestamp() - int64(UpstreamBillingReconcileLookback().Seconds())
		if err := model.RequeueFailedUpstreamBillingRecords(settings.CredentialID, createdAfter); err != nil {
			common.SysError(fmt.Sprintf("failed to requeue failed upstream billing records for account %d: %s", settings.CredentialID, err.Error()))
		}
	}
}

func ResolveUpstreamBillingQuota(ctx *gin.Context, info *relaycommon.RelayInfo, estimatedQuota int64, usage *dto.Usage) int64 {
	settings := upstreamBillingSettings(info)
	if settings == nil {
		return estimatedQuota
	}
	costRate := settings.EffectiveCostRate()
	fallbackQuota := ApplyUpstreamCostRateToEstimatedQuota(info, estimatedQuota)
	costRateText := costRate.String()
	costRateSource := string(settings.EffectiveCostRateSource())
	requestFinishedAtMs := time.Now().UnixMilli()
	requestStartedAtMs := requestFinishedAtMs
	if !info.StartTime.IsZero() {
		requestStartedAtMs = info.StartTime.UnixMilli()
	}
	billingRequestID := common.GetContextKeyString(ctx, common.UpstreamBillingRequestIdKey)
	responseRequestID := common.GetContextKeyString(ctx, common.UpstreamRequestIdKey)
	upstreamRequestID := billingRequestID
	if responseRequestID != "" && responseRequestID != billingRequestID {
		upstreamRequestID = responseRequestID
	}
	if upstreamRequestID == "" {
		upstreamRequestID = info.RequestId
	}
	groupRatio := decimal.NewFromFloat(info.PriceData.BillingGroupRatio())
	groupRatioText := groupRatio.String()
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	quotaPerUnitText := quotaPerUnit.String()
	modelName := info.UpstreamModelName
	if modelName == "" {
		modelName = info.OriginModelName
	}
	var promptTokens, completionTokens, totalTokens int64
	if usage != nil {
		promptTokens = int64(usage.PromptTokens)
		completionTokens = int64(usage.CompletionTokens)
		totalTokens = int64(usage.TotalTokens)
	}
	audit := &relaycommon.UpstreamBillingAudit{
		Enabled:            true,
		CredentialId:       settings.CredentialID,
		Status:             model.UpstreamBillingStatusPending,
		UpstreamRequestId:  upstreamRequestID,
		EstimatedQuota:     fallbackQuota,
		ChargedQuota:       fallbackQuota,
		UserGroup:          info.UserGroup,
		UsingGroup:         info.UsingGroup,
		GroupRatio:         groupRatioText,
		GroupRatioSource:   info.PriceData.GroupRatioInfo.Source,
		QuotaPerUnit:       quotaPerUnitText,
		CostRateMultiplier: costRateText,
		CostRateSource:     costRateSource,
	}
	info.UpstreamBillingAudit = audit
	if err := model.CreateUpstreamBillingRecord(&model.UpstreamBillingRecord{
		LocalRequestId:      info.RequestId,
		UpstreamRequestId:   upstreamRequestID,
		RequestStartedAtMs:  requestStartedAtMs,
		RequestFinishedAtMs: requestFinishedAtMs,
		ChannelId:           info.ChannelId,
		CredentialId:        settings.CredentialID,
		UserId:              info.UserId,
		IsPlayground:        info.IsPlayground,
		Provider:            string(settings.Provider),
		Status:              model.UpstreamBillingStatusPending,
		ChargedQuota:        fallbackQuota,
		EstimatedQuota:      fallbackQuota,
		UserGroup:           info.UserGroup,
		UsingGroup:          info.UsingGroup,
		GroupRatio:          groupRatioText,
		GroupRatioSource:    info.PriceData.GroupRatioInfo.Source,
		QuotaPerUnit:        quotaPerUnitText,
		CostRateMultiplier:  costRateText,
		CostRateSource:      costRateSource,
		ModelName:           modelName,
		PromptTokens:        promptTokens,
		CompletionTokens:    completionTokens,
		TotalTokens:         totalTokens,
	}); err != nil {
		common.SysError(fmt.Sprintf("failed to create upstream billing record for request %s: %s", info.RequestId, err.Error()))
	}
	// Settlement can run after a streamed response has closed and Gin's request
	// context has already been canceled. Billing reconciliation must still finish
	// so the final charge reflects the upstream account instead of falling back
	// to the local estimate.
	lookupCtx, cancel := context.WithTimeout(context.Background(), upstreamBillingLookupTimeout)
	defer cancel()
	var usageMatch *upstreamBillingUsageMatch
	if usage != nil {
		usageMatch = &upstreamBillingUsageMatch{
			ModelName:        modelName,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			StartedAtMs:      requestStartedAtMs,
			FinishedAtMs:     requestFinishedAtMs,
			AllowTimeFallback: common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens) ||
				(info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd()),
			CredentialID:   settings.CredentialID,
			LocalRequestID: info.RequestId,
		}
	}
	result, attempts, lookupErr := lookupUpstreamBilling(lookupCtx, info, settings, info.RequestId, upstreamRequestID, usageMatch)
	audit.Attempts = attempts
	if lookupErr != nil {
		status := model.UpstreamBillingStatusFailed
		if errors.Is(lookupErr, errUpstreamBillingRecordNotAvailable) {
			status = model.UpstreamBillingStatusEstimated
		}
		finishUpstreamBillingFallback(info, audit, status, audit.Attempts, lookupErr)
		return fallbackQuota
	}

	upstreamCostQuota, baseClamp := common.QuotaFromDecimalChecked(result.CostUSD.Mul(quotaPerUnit))
	noteQuotaClamp(info, baseClamp)
	quota, clamp := common.QuotaFromDecimalChecked(result.CostUSD.Mul(groupRatio).Mul(quotaPerUnit))
	noteQuotaClamp(info, clamp)
	audit.Provider = string(result.Provider)
	audit.Status = model.UpstreamBillingStatusExact
	audit.IdentityAmbiguous = result.IdentityAmbiguous
	if result.UpstreamRequestID != "" {
		audit.UpstreamRequestId = result.UpstreamRequestID
	}
	audit.UpstreamCostUSD = result.CostUSD.String()
	audit.UpstreamQuota = result.UpstreamQuota
	audit.UpstreamCostQuota = upstreamCostQuota
	audit.ChargedQuota = quota
	audit.MarginQuota = quota - upstreamCostQuota
	now := common.GetTimestamp()
	updates := map[string]interface{}{
		"provider":             string(result.Provider),
		"status":               model.UpstreamBillingStatusExact,
		"identity_ambiguous":   result.IdentityAmbiguous,
		"upstream_cost_usd":    result.CostUSD.String(),
		"upstream_quota":       result.UpstreamQuota,
		"charged_quota":        quota,
		"user_group":           info.UserGroup,
		"using_group":          info.UsingGroup,
		"group_ratio":          groupRatioText,
		"group_ratio_source":   info.PriceData.GroupRatioInfo.Source,
		"quota_per_unit":       quotaPerUnitText,
		"cost_rate_multiplier": costRateText,
		"cost_rate_source":     costRateSource,
		"attempts":             audit.Attempts,
		"error":                "",
		"adjustment_applied":   true,
		"log_updated":          true,
		"exact_at":             now,
		"last_checked_at":      now,
	}
	if result.UpstreamRequestID != "" {
		updates["upstream_request_id"] = result.UpstreamRequestID
	}
	if settings.IsRecheckEnabled() {
		updates["recheck_until"] = now + int64(settings.RecheckWindow().Seconds())
		updates["next_recheck_at"] = now + int64((5 * time.Minute).Seconds())
	}
	if err := model.UpdateUpstreamBillingRecord(info.RequestId, updates); err != nil {
		common.SysError(fmt.Sprintf("failed to update exact upstream billing record for request %s: %s", info.RequestId, err.Error()))
	}
	return quota
}

type UpstreamBillingReconcileSummary struct {
	Scanned   int `json:"scanned"`
	Exact     int `json:"exact"`
	Adjusted  int `json:"adjusted"`
	Rechecked int `json:"rechecked"`
	Revised   int `json:"revised"`
	Retried   int `json:"retried"`
	LogOnly   int `json:"log_only"`
	Failed    int `json:"failed"`
}

type Sub2APICredentialRefreshSummary struct {
	Scanned   int `json:"scanned"`
	Refreshed int `json:"refreshed"`
	Failed    int `json:"failed"`
}

func channelUsesSub2APIBilling(settings *dto.UpstreamBillingSettings) bool {
	if settings == nil || !settings.Enabled || strings.TrimSpace(settings.RefreshToken) == "" {
		return false
	}
	return settings.Provider == dto.UpstreamBillingProviderSub2API ||
		settings.DetectedProvider == dto.UpstreamBillingProviderSub2API ||
		settings.Provider == dto.UpstreamBillingProviderAuto
}

func HasSub2APICredentialRefreshWork() bool {
	accounts, err := model.ListEnabledUpstreamBillingAccounts()
	if err != nil {
		common.SysError("failed to list upstream accounts for sub2api credential refresh: " + err.Error())
		return false
	}
	now := common.GetTimestamp()
	for index := range accounts {
		settings := accounts[index].ToSettings()
		if channelUsesSub2APIBilling(settings) && sub2APIAccessTokenNeedsRefresh(settings, now) {
			return true
		}
	}
	channels, err := model.ListEnabledChannelsForUpstreamBillingCredentials()
	if err != nil {
		common.SysError("failed to list channels for sub2api credential refresh: " + err.Error())
		return false
	}
	for index := range channels {
		settings := channels[index].GetOtherSettings().UpstreamBilling
		if settings != nil && settings.CredentialID == 0 && channelUsesSub2APIBilling(settings) && sub2APIAccessTokenNeedsRefresh(settings, now) {
			return true
		}
	}
	return false
}

func RefreshDueSub2APICredentials(ctx context.Context) Sub2APICredentialRefreshSummary {
	accounts, accountErr := model.ListEnabledUpstreamBillingAccounts()
	if accountErr != nil {
		common.SysError("failed to list upstream accounts for sub2api credential refresh: " + accountErr.Error())
		return Sub2APICredentialRefreshSummary{Failed: 1}
	}
	now := common.GetTimestamp()
	summary := Sub2APICredentialRefreshSummary{}
	for index := range accounts {
		if ctx.Err() != nil {
			return summary
		}
		settings := accounts[index].ToSettings()
		if !channelUsesSub2APIBilling(settings) || !sub2APIAccessTokenNeedsRefresh(settings, now) {
			continue
		}
		summary.Scanned++
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:       settings.APIBaseURL,
			ChannelSetting:       dto.ChannelSettings{Proxy: settings.Proxy},
			ChannelOtherSettings: dto.ChannelOtherSettings{UpstreamBilling: settings},
		}}
		previousAccessToken := settings.AccessToken
		resolved, refreshErr := resolveSub2APIAccessToken(ctx, info, settings, false, "", false)
		if refreshErr != nil {
			summary.Failed++
			recordUpstreamBillingAccountHealth(settings, refreshErr)
			common.SysError(fmt.Sprintf("failed to refresh sub2api billing credential for upstream account %d: %s", accounts[index].Id, refreshErr.Error()))
			continue
		}
		recordUpstreamBillingAccountHealth(settings, nil)
		if resolved != nil && resolved.AccessToken != "" && resolved.AccessToken != previousAccessToken {
			summary.Refreshed++
		}
	}
	channels, err := model.ListEnabledChannelsForUpstreamBillingCredentials()
	if err != nil {
		common.SysError("failed to list channels for sub2api credential refresh: " + err.Error())
		summary.Failed++
		return summary
	}
	for index := range channels {
		if ctx.Err() != nil {
			break
		}
		channel := &channels[index]
		settings := channel.GetOtherSettings().UpstreamBilling
		if settings == nil || settings.CredentialID > 0 || !channelUsesSub2APIBilling(settings) || !sub2APIAccessTokenNeedsRefresh(settings, now) {
			continue
		}
		summary.Scanned++
		baseURL := ""
		if channel.BaseURL != nil {
			baseURL = *channel.BaseURL
		}
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:            channel.Id,
			ChannelBaseUrl:       baseURL,
			ChannelSetting:       channel.GetSetting(),
			ChannelOtherSettings: channel.GetOtherSettings(),
		}}
		previousAccessToken := settings.AccessToken
		resolved, refreshErr := resolveSub2APIAccessToken(ctx, info, settings, false, "", false)
		if refreshErr != nil {
			summary.Failed++
			common.SysError(fmt.Sprintf("failed to refresh sub2api billing credential for channel %d: %s", channel.Id, refreshErr.Error()))
			continue
		}
		if resolved != nil && resolved.AccessToken != "" && resolved.AccessToken != previousAccessToken {
			summary.Refreshed++
		}
	}
	return summary
}

func UpstreamBillingReconcileLookback() time.Duration {
	days := common.GetEnvOrDefault("UPSTREAM_BILLING_RECONCILE_LOOKBACK_DAYS", upstreamBillingReconcileDays)
	if days < 1 {
		days = upstreamBillingReconcileDays
	}
	return time.Duration(days) * 24 * time.Hour
}

func upstreamBillingRetryDelay(retryCount int, age time.Duration) time.Duration {
	if age >= 7*24*time.Hour {
		return 24 * time.Hour
	}
	if age >= 24*time.Hour {
		return 6 * time.Hour
	}
	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount > 5 {
		retryCount = 5
	}
	delay := time.Minute * time.Duration(1<<retryCount)
	if delay > 30*time.Minute {
		return 30 * time.Minute
	}
	return delay
}

func upstreamBillingRecheckDelay(recheckCount int) time.Duration {
	switch recheckCount {
	case 0:
		return 5 * time.Minute
	case 1:
		return 30 * time.Minute
	case 2:
		return 2 * time.Hour
	case 3:
		return 6 * time.Hour
	case 4:
		return 12 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func scheduleUpstreamBillingRecheck(record *model.UpstreamBillingRecord, settings *dto.UpstreamBillingSettings, checked bool) error {
	if record == nil {
		return errors.New("upstream billing record is required")
	}
	now := common.GetTimestamp()
	if settings == nil || !settings.IsRecheckEnabled() {
		return model.UpdateUpstreamBillingRecord(record.LocalRequestId, map[string]interface{}{
			"next_recheck_at": 0,
			"recheck_until":   0,
			"last_checked_at": now,
		})
	}
	exactAt := record.ExactAt
	if exactAt == 0 {
		exactAt = now
	}
	count := record.RecheckCount
	if checked {
		count++
	}
	until := exactAt + int64(settings.RecheckWindow().Seconds())
	next := now + int64(upstreamBillingRecheckDelay(count).Seconds())
	if next > until || now >= until {
		next = 0
	}
	return model.UpdateUpstreamBillingRecord(record.LocalRequestId, map[string]interface{}{
		"exact_at":        exactAt,
		"recheck_count":   count,
		"last_checked_at": now,
		"next_recheck_at": next,
		"recheck_until":   until,
	})
}

func upstreamBillingOtherInt(other map[string]interface{}, key string) int {
	value, ok := other[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := strconv.Atoi(typed.String())
		return parsed
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		parsed, _ := strconv.Atoi(fmt.Sprint(typed))
		return parsed
	}
}

func updateReconciledUpstreamBillingLog(record *model.UpstreamBillingRecord, logEntry *model.Log) error {
	if record == nil || logEntry == nil {
		return errors.New("upstream billing record and consume log are required")
	}
	other, err := common.StrToMap(logEntry.Other)
	if err != nil || other == nil {
		other = map[string]interface{}{}
	}
	status := record.Status
	if status != model.UpstreamBillingStatusExact && status != model.UpstreamBillingStatusFailed {
		status = model.UpstreamBillingStatusExact
	}
	other["upstream_billing_status"] = status
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	if !ok || adminInfo == nil {
		adminInfo = map[string]interface{}{}
		other["admin_info"] = adminInfo
	}
	billingInfo := map[string]interface{}{
		"credential_id":         record.CredentialId,
		"provider":              record.Provider,
		"status":                status,
		"upstream_request_id":   record.UpstreamRequestId,
		"identity_ambiguous":    record.IdentityAmbiguous,
		"upstream_cost_usd":     record.UpstreamCostUSD,
		"estimated_quota":       record.EstimatedQuota,
		"charged_quota":         record.ChargedQuota,
		"user_group":            record.UserGroup,
		"using_group":           record.UsingGroup,
		"group_ratio":           record.GroupRatio,
		"group_ratio_source":    record.GroupRatioSource,
		"quota_per_unit":        record.QuotaPerUnit,
		"cost_rate_multiplier":  record.CostRateMultiplier,
		"cost_rate_source":      record.CostRateSource,
		"attempts":              record.Attempts,
		"adjustment_quota":      record.AdjustmentQuota,
		"wallet_adjustment":     record.WalletAdjustment,
		"revision_count":        record.RevisionCount,
		"last_checked_at":       record.LastCheckedAt,
		"recheck_until":         record.RecheckUntil,
		"error":                 record.Error,
		"background_reconciled": true,
	}
	if record.UpstreamQuota != 0 {
		billingInfo["upstream_quota"] = record.UpstreamQuota
	}
	if record.UpstreamCostUSD != "" {
		quotaPerUnit, parseErr := decimal.NewFromString(record.QuotaPerUnit)
		if parseErr == nil && quotaPerUnit.IsPositive() {
			costUSD, costErr := decimal.NewFromString(record.UpstreamCostUSD)
			if costErr == nil && !costUSD.IsNegative() {
				upstreamCostQuota, _ := common.QuotaFromDecimalChecked(costUSD.Mul(quotaPerUnit))
				billingInfo["upstream_cost_quota"] = upstreamCostQuota
				billingInfo["margin_quota"] = record.ChargedQuota - upstreamCostQuota
			}
		}
	}
	adminInfo["upstream_billing"] = billingInfo
	return model.UpdateConsumeLogUpstreamBilling(record.LocalRequestId, record.UserId, record.UpstreamRequestId, record.ChargedQuota, common.MapToJsonStr(other))
}

func markUpstreamBillingRetry(record *model.UpstreamBillingRecord, attempts int, err error) {
	if record == nil || err == nil {
		return
	}
	now := common.GetTimestamp()
	age := time.Duration(0)
	if record.CreatedAt > 0 && now > record.CreatedAt {
		age = time.Duration(now-record.CreatedAt) * time.Second
	}
	delay := upstreamBillingRetryDelay(record.RetryCount, age)
	if updateErr := model.MarkUpstreamBillingRetry(record.LocalRequestId, int64(delay.Seconds()), attempts, err.Error()); updateErr != nil {
		common.SysError(fmt.Sprintf("failed to schedule upstream billing retry for request %s: %s", record.LocalRequestId, updateErr.Error()))
	}
}

func RunUpstreamBillingReconcile(ctx context.Context, batchSize int, lookback time.Duration) UpstreamBillingReconcileSummary {
	return runUpstreamBillingReconcile(ctx, batchSize, lookback, 0, false)
}

func RunUpstreamBillingReconcileForCredential(ctx context.Context, credentialID int, batchSize int, lookback time.Duration) UpstreamBillingReconcileSummary {
	if credentialID <= 0 {
		return UpstreamBillingReconcileSummary{Failed: 1}
	}
	return runUpstreamBillingReconcile(ctx, batchSize, lookback, credentialID, true)
}

func runUpstreamBillingReconcile(ctx context.Context, batchSize int, lookback time.Duration, credentialID int, force bool) UpstreamBillingReconcileSummary {
	if batchSize <= 0 {
		batchSize = upstreamBillingReconcileBatch
	}
	if lookback <= 0 {
		lookback = UpstreamBillingReconcileLookback()
	}
	now := common.GetTimestamp()
	var records []model.UpstreamBillingRecord
	var err error
	if credentialID > 0 {
		records, err = model.FindUpstreamBillingRecordsForCredentialReconcile(now, now-int64(lookback.Seconds()), batchSize, credentialID)
	} else {
		records, err = model.FindUpstreamBillingRecordsForReconcile(now, now-int64(lookback.Seconds()), batchSize)
	}
	if err != nil {
		common.SysError("failed to list upstream billing records for reconciliation: " + err.Error())
		return UpstreamBillingReconcileSummary{Failed: 1}
	}
	summary := UpstreamBillingReconcileSummary{Scanned: len(records)}
	for index := range records {
		if ctx.Err() != nil {
			break
		}
		record := &records[index]
		logEntry, logErr := model.GetConsumeLogByRequestId(record.LocalRequestId, record.UserId)
		if logErr != nil {
			markUpstreamBillingRetry(record, 0, logErr)
			summary.Retried++
			continue
		}
		recheckDue := (force && record.Status == model.UpstreamBillingStatusExact && record.AdjustmentApplied) ||
			(!force && record.Status == model.UpstreamBillingStatusExact &&
				record.AdjustmentApplied &&
				record.NextRecheckAt > 0 &&
				record.NextRecheckAt <= now)
		if record.AdjustmentApplied && !record.LogUpdated {
			if err := updateReconciledUpstreamBillingLog(record, logEntry); err != nil {
				markUpstreamBillingRetry(record, 0, err)
				summary.Retried++
				continue
			}
			if err := model.MarkUpstreamBillingLogUpdated(record.LocalRequestId); err != nil {
				markUpstreamBillingRetry(record, 0, err)
				summary.Retried++
				continue
			}
			record.LogUpdated = true
			summary.LogOnly++
			if !recheckDue {
				continue
			}
		}

		channel, channelErr := model.GetChannelById(record.ChannelId, true)
		if channelErr != nil {
			markUpstreamBillingRetry(record, 0, channelErr)
			summary.Retried++
			continue
		}
		otherSettings := channel.GetOtherSettings()
		settings, resolveSettingsErr := model.ResolveChannelUpstreamBillingSettings(channel)
		if resolveSettingsErr != nil {
			markUpstreamBillingRetry(record, 0, resolveSettingsErr)
			summary.Retried++
			continue
		}
		if settings == nil {
			markUpstreamBillingRetry(record, 0, errors.New("upstream billing settings are no longer available"))
			summary.Retried++
			continue
		}
		resolvedSettings := *settings
		if record.CredentialId > 0 && resolvedSettings.CredentialID != record.CredentialId {
			account, accountErr := model.GetUpstreamBillingAccountByID(record.CredentialId)
			if accountErr != nil {
				markUpstreamBillingRetry(record, 0, accountErr)
				summary.Retried++
				continue
			}
			accountSettings := account.ToSettings()
			accountSettings.Enabled = true
			accountSettings.RecheckEnabled = resolvedSettings.RecheckEnabled
			accountSettings.RecheckWindowHours = resolvedSettings.RecheckWindowHours
			resolvedSettings = *accountSettings
		}
		if record.CredentialId <= 0 && resolvedSettings.CredentialID > 0 {
			if updateErr := model.UpdateUpstreamBillingRecord(record.LocalRequestId, map[string]interface{}{
				"credential_id": resolvedSettings.CredentialID,
			}); updateErr != nil {
				markUpstreamBillingRetry(record, 0, updateErr)
				summary.Retried++
				continue
			}
			record.CredentialId = resolvedSettings.CredentialID
		}
		if recheckDue && !force && !resolvedSettings.IsRecheckEnabled() {
			if err := scheduleUpstreamBillingRecheck(record, &resolvedSettings, true); err != nil {
				markUpstreamBillingRetry(record, 0, err)
				summary.Retried++
			}
			continue
		}
		resolvedSettings.Enabled = true
		if record.Provider == string(dto.UpstreamBillingProviderNewAPI) || record.Provider == string(dto.UpstreamBillingProviderSub2API) {
			resolvedSettings.Provider = dto.UpstreamBillingProvider(record.Provider)
		}
		if validateErr := resolvedSettings.Validate(); validateErr != nil {
			markUpstreamBillingRetry(record, 0, validateErr)
			summary.Retried++
			continue
		}
		baseURL := ""
		if channel.BaseURL != nil {
			baseURL = *channel.BaseURL
		}
		channelKey := ""
		channelKeys := channel.GetKeys()
		if len(channelKeys) == 1 {
			channelKey = strings.TrimSpace(channelKeys[0])
		}
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:            channel.Id,
			ChannelBaseUrl:       baseURL,
			ApiKey:               channelKey,
			ChannelSetting:       channel.GetSetting(),
			ChannelOtherSettings: otherSettings,
		}}
		lookupCtx, cancel := context.WithTimeout(ctx, upstreamBillingLookupTimeout)
		logOther, _ := common.StrToMap(logEntry.Other)
		allowTimeFallback := false
		if adminInfo, ok := logOther["admin_info"].(map[string]interface{}); ok {
			allowTimeFallback, _ = adminInfo["local_count_tokens"].(bool)
		}
		if streamStatus, ok := logOther["stream_status"].(map[string]interface{}); ok {
			endReason, _ := streamStatus["end_reason"].(string)
			if endReason != "" && endReason != string(relaycommon.StreamEndReasonDone) &&
				endReason != string(relaycommon.StreamEndReasonEOF) &&
				endReason != string(relaycommon.StreamEndReasonHandlerStop) {
				allowTimeFallback = true
			}
		}
		requestFinishedAtMs := record.RequestFinishedAtMs
		if requestFinishedAtMs <= 0 {
			requestFinishedAtMs = logEntry.CreatedAt * int64(time.Second/time.Millisecond)
		}
		requestStartedAtMs := record.RequestStartedAtMs
		if requestStartedAtMs <= 0 {
			requestStartedAtMs = (logEntry.CreatedAt - int64(logEntry.UseTime)) * int64(time.Second/time.Millisecond)
		}
		usageMatch := &upstreamBillingUsageMatch{
			ModelName:         logEntry.ModelName,
			PromptTokens:      logEntry.PromptTokens,
			CompletionTokens:  logEntry.CompletionTokens,
			StartedAtMs:       requestStartedAtMs,
			FinishedAtMs:      requestFinishedAtMs,
			AllowTimeFallback: allowTimeFallback,
			CredentialID:      resolvedSettings.CredentialID,
			LocalRequestID:    record.LocalRequestId,
		}
		lookupResult, attempts, lookupErr := lookupUpstreamBilling(lookupCtx, info, &resolvedSettings, record.LocalRequestId, record.UpstreamRequestId, usageMatch)
		cancel()
		if lookupErr != nil {
			if recheckDue {
				checkedAt := common.GetTimestamp()
				if updateErr := model.UpdateUpstreamBillingRecord(record.LocalRequestId, map[string]interface{}{
					"last_checked_at": checkedAt,
				}); updateErr != nil {
					common.SysError(fmt.Sprintf("failed to record upstream billing recheck failure for request %s: %s", record.LocalRequestId, updateErr.Error()))
				} else {
					record.LastCheckedAt = checkedAt
					record.Attempts += attempts
					record.Error = lookupErr.Error()
					if logErr := updateReconciledUpstreamBillingLog(record, logEntry); logErr != nil {
						common.SysError(fmt.Sprintf("failed to update upstream billing recheck log for request %s: %s", record.LocalRequestId, logErr.Error()))
					}
				}
			}
			markUpstreamBillingRetry(record, attempts, lookupErr)
			summary.Retried++
			continue
		}
		groupRatio := decimal.NewFromInt(1)
		if record.GroupRatioSource != "" {
			parsedRatio, parseErr := decimal.NewFromString(record.GroupRatio)
			if parseErr != nil || parsedRatio.IsNegative() {
				markUpstreamBillingRetry(record, attempts, fmt.Errorf("invalid group ratio snapshot %q", record.GroupRatio))
				summary.Failed++
				continue
			}
			groupRatio = parsedRatio
		}
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		if record.QuotaPerUnit != "" {
			parsedQuotaPerUnit, parseErr := decimal.NewFromString(record.QuotaPerUnit)
			if parseErr != nil || !parsedQuotaPerUnit.IsPositive() {
				markUpstreamBillingRetry(record, attempts, fmt.Errorf("invalid quota per unit snapshot %q", record.QuotaPerUnit))
				summary.Failed++
				continue
			}
			quotaPerUnit = parsedQuotaPerUnit
		}
		exactQuota, clamp := common.QuotaFromDecimalChecked(lookupResult.CostUSD.Mul(groupRatio).Mul(quotaPerUnit))
		if clamp != nil {
			common.SysError(fmt.Sprintf("upstream billing reconciliation quota saturation: request=%s op=%s kind=%s original=%g clamped=%d", record.LocalRequestId, clamp.Op, clamp.Kind, clamp.Original, clamp.Clamped))
		}
		billingSource, _ := logOther["billing_source"].(string)
		nodeName := common.NodeName
		if adminInfo, ok := logOther["admin_info"].(map[string]interface{}); ok {
			if loggedNodeName, ok := adminInfo["node_name"].(string); ok && strings.TrimSpace(loggedNodeName) != "" {
				nodeName = loggedNodeName
			}
		}
		previousChargedQuota := record.ChargedQuota
		wasExact := record.Status == model.UpstreamBillingStatusExact && record.AdjustmentApplied
		adjustment, applyErr := model.ApplyUpstreamBillingAdjustment(model.UpstreamBillingAdjustmentInput{
			LocalRequestId:    record.LocalRequestId,
			UpstreamRequestId: lookupResult.UpstreamRequestID,
			IdentityAmbiguous: lookupResult.IdentityAmbiguous,
			Provider:          string(lookupResult.Provider),
			UpstreamCostUSD:   lookupResult.CostUSD.String(),
			UpstreamQuota:     lookupResult.UpstreamQuota,
			ExactQuota:        exactQuota,
			CurrentCharged:    logEntry.Quota,
			BillingSource:     billingSource,
			SubscriptionId:    upstreamBillingOtherInt(logOther, "subscription_id"),
			TokenId:           logEntry.TokenId,
			IsPlayground:      record.IsPlayground,
			LogOnly:           billingSource == BillingSourceChannelTest,
			LookupAttempts:    attempts,
			QuotaData: &model.QuotaDataLogParams{
				UserID:    logEntry.UserId,
				Username:  logEntry.Username,
				ModelName: logEntry.ModelName,
				Quota:     logEntry.Quota,
				CreatedAt: logEntry.CreatedAt,
				TokenUsed: logEntry.PromptTokens + logEntry.CompletionTokens,
				UseGroup:  logEntry.Group,
				TokenID:   logEntry.TokenId,
				ChannelID: logEntry.ChannelId,
				NodeName:  nodeName,
			},
		})
		if applyErr != nil {
			markUpstreamBillingRetry(record, attempts, applyErr)
			summary.Failed++
			continue
		}
		record.Provider = string(lookupResult.Provider)
		if lookupResult.UpstreamRequestID != "" {
			record.UpstreamRequestId = lookupResult.UpstreamRequestID
		}
		record.Status = model.UpstreamBillingStatusExact
		record.IdentityAmbiguous = lookupResult.IdentityAmbiguous
		record.UpstreamCostUSD = lookupResult.CostUSD.String()
		record.UpstreamQuota = lookupResult.UpstreamQuota
		record.ChargedQuota = exactQuota
		record.Attempts += attempts
		record.Error = ""
		record.AdjustmentApplied = true
		record.AdjustmentQuota = adjustment.AdjustmentQuota
		record.WalletAdjustment = adjustment.WalletAdjustment
		if record.ExactAt == 0 {
			record.ExactAt = common.GetTimestamp()
		}
		if adjustment.Applied {
			summary.Adjusted++
		}
		if wasExact {
			summary.Rechecked++
			if exactQuota != previousChargedQuota {
				record.RevisionCount++
				summary.Revised++
			}
		}
		if err := updateReconciledUpstreamBillingLog(record, logEntry); err != nil {
			markUpstreamBillingRetry(record, 0, err)
			summary.Retried++
			continue
		}
		if err := model.MarkUpstreamBillingLogUpdated(record.LocalRequestId); err != nil {
			markUpstreamBillingRetry(record, 0, err)
			summary.Retried++
			continue
		}
		if err := scheduleUpstreamBillingRecheck(record, &resolvedSettings, wasExact); err != nil {
			markUpstreamBillingRetry(record, 0, err)
			summary.Retried++
			continue
		}
		summary.Exact++
	}
	return summary
}

func finishUpstreamBillingFallback(info *relaycommon.RelayInfo, audit *relaycommon.UpstreamBillingAudit, status string, attempts int, err error) {
	audit.Status = status
	audit.Attempts = attempts
	if err != nil {
		audit.Error = err.Error()
	}
	if updateErr := model.UpdateUpstreamBillingRecord(info.RequestId, map[string]interface{}{
		"status":        status,
		"charged_quota": audit.ChargedQuota,
		"attempts":      attempts,
		"error":         audit.Error,
	}); updateErr != nil {
		common.SysError(fmt.Sprintf("failed to update fallback upstream billing record for request %s: %s", info.RequestId, updateErr.Error()))
	}
}

func SetUpstreamBillingChargedQuota(info *relaycommon.RelayInfo, quota int64) {
	if info == nil || info.UpstreamBillingAudit == nil {
		return
	}
	info.UpstreamBillingAudit.ChargedQuota = quota
	if err := model.UpdateUpstreamBillingRecord(info.RequestId, map[string]interface{}{"charged_quota": quota}); err != nil {
		common.SysError(fmt.Sprintf("failed to update upstream billing charged quota for request %s: %s", info.RequestId, err.Error()))
	}
}

func DetectUpstreamBillingProvider(ctx context.Context, channel *model.Channel, settings *dto.UpstreamBillingSettings) (UpstreamBillingDetectionResult, error) {
	if channel == nil || settings == nil {
		return UpstreamBillingDetectionResult{}, errors.New("channel and upstream billing settings are required")
	}
	resolvedSettings := *settings
	if resolvedSettings.CredentialID > 0 {
		account, accountErr := model.GetUpstreamBillingAccountByID(resolvedSettings.CredentialID)
		if accountErr != nil {
			return UpstreamBillingDetectionResult{}, accountErr
		}
		accountSettings := account.ToSettings()
		accountSettings.Enabled = true
		accountSettings.RecheckEnabled = resolvedSettings.RecheckEnabled
		accountSettings.RecheckWindowHours = resolvedSettings.RecheckWindowHours
		accountSettings.UpstreamTokenID = resolvedSettings.UpstreamTokenID
		accountSettings.UpstreamTokenName = resolvedSettings.UpstreamTokenName
		accountSettings.CostRateAuto = resolvedSettings.CostRateAuto
		accountSettings.CostRateMultiplier = resolvedSettings.CostRateMultiplier
		accountSettings.CostRateSource = resolvedSettings.CostRateSource
		accountSettings.CostRateUpdatedAt = resolvedSettings.CostRateUpdatedAt
		accountSettings.CostRateError = resolvedSettings.CostRateError
		resolvedSettings = *accountSettings
	}
	storedSettings := channel.GetOtherSettings().UpstreamBilling
	if storedSettings != nil {
		if strings.TrimSpace(resolvedSettings.AccessToken) == "" && resolvedSettings.AccessTokenConfigured {
			resolvedSettings.AccessToken = storedSettings.AccessToken
		}
		if strings.TrimSpace(resolvedSettings.RefreshToken) == "" && resolvedSettings.RefreshTokenConfigured {
			resolvedSettings.RefreshToken = storedSettings.RefreshToken
			resolvedSettings.AccessTokenIssuedAt = storedSettings.AccessTokenIssuedAt
			resolvedSettings.AccessTokenExpiresAt = storedSettings.AccessTokenExpiresAt
			if resolvedSettings.AccessToken == "" {
				resolvedSettings.AccessToken = storedSettings.AccessToken
			}
		}
	}
	if err := resolvedSettings.Validate(); err != nil {
		return UpstreamBillingDetectionResult{}, err
	}
	baseURL := ""
	if channel.BaseURL != nil {
		baseURL = *channel.BaseURL
	}
	resolvedBaseURL, err := upstreamBillingBaseURL(baseURL, &resolvedSettings)
	if err != nil {
		return UpstreamBillingDetectionResult{}, err
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelId:            channel.Id,
		ChannelBaseUrl:       baseURL,
		ChannelSetting:       channel.GetSetting(),
		ChannelOtherSettings: dto.ChannelOtherSettings{UpstreamBilling: &resolvedSettings},
	}}
	providers := upstreamBillingProviderCandidates(channel.Id, &resolvedSettings, "", "")
	var lastErr error
	for _, provider := range providers {
		switch provider {
		case dto.UpstreamBillingProviderNewAPI:
			quotaPerUnit, detectErr := getNewAPIQuotaPerUnit(ctx, info, resolvedBaseURL, resolvedSettings.AccessToken, resolvedSettings.UserID)
			if detectErr == nil {
				var response struct {
					Success bool `json:"success"`
				}
				detectErr = doUpstreamBillingRequest(ctx, info, upstreamBillingURL(resolvedBaseURL, "/api/log/self", url.Values{"p": []string{"1"}, "page_size": []string{"1"}}), resolvedSettings.AccessToken, resolvedSettings.UserID, &response)
				if detectErr == nil && !response.Success {
					detectErr = errors.New("new-api billing API rejected the request")
				}
			}
			if detectErr == nil {
				if channel.Id > 0 {
					detectedUpstreamBillingProviders.Store(channel.Id, provider)
				}
				return UpstreamBillingDetectionResult{Provider: provider, APIBaseURL: resolvedBaseURL, QuotaPerUnit: quotaPerUnit.String()}, nil
			}
			lastErr = detectErr
		case dto.UpstreamBillingProviderSub2API:
			resolvedSub2Settings, resolveErr := resolveSub2APIAccessToken(ctx, info, &resolvedSettings, false, "", true)
			if resolveErr != nil {
				lastErr = resolveErr
				continue
			}
			var response struct {
				Data struct {
					Items []json.RawMessage `json:"items"`
				} `json:"data"`
			}
			resolvedSub2BaseURL, baseErr := upstreamBillingBaseURL(baseURL, resolvedSub2Settings)
			if baseErr != nil {
				lastErr = baseErr
				continue
			}
			detectErr := doUpstreamBillingRequest(ctx, info, upstreamBillingURL(resolvedSub2BaseURL, "/api/v1/usage", url.Values{"page": []string{"1"}, "page_size": []string{"1"}}), resolvedSub2Settings.AccessToken, 0, &response)
			if isUpstreamBillingUnauthorized(detectErr) {
				resolvedSub2Settings, resolveErr = resolveSub2APIAccessToken(ctx, info, resolvedSub2Settings, true, resolvedSub2Settings.AccessToken, true)
				if resolveErr == nil {
					resolvedSub2BaseURL, baseErr = upstreamBillingBaseURL(baseURL, resolvedSub2Settings)
					if baseErr != nil {
						detectErr = baseErr
					} else {
						detectErr = doUpstreamBillingRequest(ctx, info, upstreamBillingURL(resolvedSub2BaseURL, "/api/v1/usage", url.Values{"page": []string{"1"}, "page_size": []string{"1"}}), resolvedSub2Settings.AccessToken, 0, &response)
					}
				} else {
					detectErr = resolveErr
				}
			}
			if detectErr == nil {
				if channel.Id > 0 {
					detectedUpstreamBillingProviders.Store(channel.Id, provider)
				}
				return UpstreamBillingDetectionResult{
					Provider:             provider,
					APIBaseURL:           resolvedSub2BaseURL,
					AccessToken:          resolvedSub2Settings.AccessToken,
					RefreshToken:         resolvedSub2Settings.RefreshToken,
					AccessTokenIssuedAt:  resolvedSub2Settings.AccessTokenIssuedAt,
					AccessTokenExpiresAt: resolvedSub2Settings.AccessTokenExpiresAt,
				}, nil
			}
			lastErr = detectErr
		}
	}
	if lastErr == nil {
		lastErr = errors.New("unable to identify upstream billing provider")
	}
	return UpstreamBillingDetectionResult{}, lastErr
}
