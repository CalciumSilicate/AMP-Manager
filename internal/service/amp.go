package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"ampmanager/internal/config"
	"ampmanager/internal/crypto"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

var (
	ErrAPIKeyNotFound       = errors.New("API Key 不存在")
	ErrAPIKeyRevoked        = errors.New("API Key 已被撤销")
	ErrAPIKeyCircuitOpen    = errors.New("API Key 熔断中")
	ErrAPIKeyNotRetrievable = errors.New("API Key 只在创建时显示一次，无法再次获取")
	ErrNotOwner             = errors.New("无权操作此资源")
	ErrInvalidAPIKeyFormat  = errors.New("自定义 API Key 只能包含字母和数字，且长度至少为 16")
	ErrDuplicateAPIKey      = errors.New("API Key 已存在，请使用其他值")
)

type AmpService struct {
	settingsRepo *repository.AmpSettingsRepository
	apiKeyRepo   *repository.APIKeyRepository
}

func NewAmpService() *AmpService {
	return &AmpService{
		settingsRepo: repository.NewAmpSettingsRepository(),
		apiKeyRepo:   repository.NewAPIKeyRepository(),
	}
}

func (s *AmpService) GetSettings(userID string) (*model.AmpSettingsResponse, error) {
	settings, err := s.settingsRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	if settings == nil {
		return &model.AmpSettingsResponse{
			UpstreamURL:          "https://ampcode.com",
			ModelMappings:        []model.ModelMapping{},
			Enabled:              false,
			HasAPIKey:            false,
			WebSearchMode:        model.WebSearchModeUpstream,
			NativeMode:           false,
			RouteMappingsEnabled: true,
			ShowBalanceInAd:      false,
			HasSocks5Proxy:       false,
		}, nil
	}

	var mappings []model.ModelMapping
	if settings.ModelMappingsJSON != "" {
		_ = json.Unmarshal([]byte(settings.ModelMappingsJSON), &mappings)
	}
	if mappings == nil {
		mappings = []model.ModelMapping{}
	}

	return &model.AmpSettingsResponse{
		UpstreamURL:          settings.UpstreamURL,
		ModelMappings:        mappings,
		Enabled:              settings.Enabled,
		HasAPIKey:            settings.UpstreamAPIKey != "",
		WebSearchMode:        settings.WebSearchMode,
		NativeMode:           settings.NativeMode,
		RouteMappingsEnabled: settings.RouteMappingsEnabled,
		ShowBalanceInAd:      settings.ShowBalanceInAd,
		HasSocks5Proxy:       settings.Socks5Proxy != "",
		CreatedAt:            settings.CreatedAt,
		UpdatedAt:            settings.UpdatedAt,
	}, nil
}

func (s *AmpService) UpdateSettings(userID string, req *model.AmpSettingsRequest) (*model.AmpSettingsResponse, error) {
	existing, err := s.settingsRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	settings := &model.AmpSettings{
		UserID:               userID,
		WebSearchMode:        model.WebSearchModeUpstream,
		RouteMappingsEnabled: true,
	}
	if existing != nil {
		settings.UpstreamURL = existing.UpstreamURL
		settings.UpstreamAPIKey = existing.UpstreamAPIKey
		settings.ModelMappingsJSON = existing.ModelMappingsJSON
		settings.Enabled = existing.Enabled
		settings.WebSearchMode = existing.WebSearchMode
		settings.NativeMode = existing.NativeMode
		settings.RouteMappingsEnabled = existing.RouteMappingsEnabled
		settings.ShowBalanceInAd = existing.ShowBalanceInAd
		settings.Socks5Proxy = existing.Socks5Proxy
	}

	if req.UpstreamURL != nil {
		settings.UpstreamURL = *req.UpstreamURL
	}
	if req.Enabled != nil {
		settings.Enabled = *req.Enabled
	}
	if req.WebSearchMode != nil {
		settings.WebSearchMode = *req.WebSearchMode
	}
	if settings.WebSearchMode == "" {
		settings.WebSearchMode = model.WebSearchModeUpstream
	}
	if req.NativeMode != nil {
		settings.NativeMode = *req.NativeMode
	}
	if req.RouteMappingsEnabled != nil {
		settings.RouteMappingsEnabled = *req.RouteMappingsEnabled
	}
	if req.ShowBalanceInAd != nil {
		settings.ShowBalanceInAd = *req.ShowBalanceInAd
	}
	if req.Socks5Proxy != nil {
		settings.Socks5Proxy = *req.Socks5Proxy
	}

	if req.UpstreamAPIKey != nil {
		if strings.TrimSpace(*req.UpstreamAPIKey) == "" {
			settings.UpstreamAPIKey = ""
		} else {
			encKey := config.Get().GetEncryptionKey()
			if encKey != nil {
				encrypted, err := crypto.Encrypt([]byte(*req.UpstreamAPIKey), encKey)
				if err != nil {
					return nil, fmt.Errorf("failed to encrypt upstream API key: %w", err)
				}
				settings.UpstreamAPIKey = encrypted
			} else {
				log.Println("[WARN] DATA_ENCRYPTION_KEY not set, storing upstream API key in plaintext")
				settings.UpstreamAPIKey = *req.UpstreamAPIKey
			}
		}
	}

	if req.ModelMappings != nil {
		mappingsJSON, _ := json.Marshal(req.ModelMappings)
		settings.ModelMappingsJSON = string(mappingsJSON)
	}

	if err := s.settingsRepo.Upsert(settings); err != nil {
		return nil, err
	}

	var mappings []model.ModelMapping
	if settings.ModelMappingsJSON != "" {
		_ = json.Unmarshal([]byte(settings.ModelMappingsJSON), &mappings)
	}
	if mappings == nil {
		mappings = []model.ModelMapping{}
	}

	return &model.AmpSettingsResponse{
		UpstreamURL:          settings.UpstreamURL,
		ModelMappings:        mappings,
		Enabled:              settings.Enabled,
		HasAPIKey:            settings.UpstreamAPIKey != "",
		WebSearchMode:        settings.WebSearchMode,
		NativeMode:           settings.NativeMode,
		RouteMappingsEnabled: settings.RouteMappingsEnabled,
		ShowBalanceInAd:      settings.ShowBalanceInAd,
		HasSocks5Proxy:       settings.Socks5Proxy != "",
		CreatedAt:            settings.CreatedAt,
		UpdatedAt:            settings.UpdatedAt,
	}, nil
}

func (s *AmpService) TestConnection(userID string) (*model.TestConnectionResponse, error) {
	settings, err := s.settingsRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	if settings == nil || settings.UpstreamURL == "" {
		return &model.TestConnectionResponse{
			Success: false,
			Message: "未配置 Upstream URL",
		}, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", settings.UpstreamURL, nil)
	if err != nil {
		return &model.TestConnectionResponse{
			Success: false,
			Message: fmt.Sprintf("创建请求失败: %v", err),
		}, nil
	}

	if settings.UpstreamAPIKey != "" {
		apiKey := s.decryptUpstreamAPIKey(settings.UpstreamAPIKey)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("X-Api-Key", apiKey)
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return &model.TestConnectionResponse{
			Success:   false,
			Message:   fmt.Sprintf("连接失败: %v", err),
			LatencyMs: latency,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &model.TestConnectionResponse{
			Success:   true,
			Message:   fmt.Sprintf("连接成功 (HTTP %d)", resp.StatusCode),
			LatencyMs: latency,
		}, nil
	}

	if resp.StatusCode == 401 {
		if settings.UpstreamAPIKey == "" {
			return &model.TestConnectionResponse{
				Success:   true,
				Message:   fmt.Sprintf("上游可达，但需要 API Key 认证 (HTTP %d)", resp.StatusCode),
				LatencyMs: latency,
			}, nil
		}
		return &model.TestConnectionResponse{
			Success:   false,
			Message:   fmt.Sprintf("API Key 无效或已过期 (HTTP %d)", resp.StatusCode),
			LatencyMs: latency,
		}, nil
	}

	return &model.TestConnectionResponse{
		Success:   false,
		Message:   fmt.Sprintf("上游返回错误: HTTP %d", resp.StatusCode),
		LatencyMs: latency,
	}, nil
}

func (s *AmpService) CreateAPIKey(userID string, req *model.CreateAPIKeyRequest) (*model.CreateAPIKeyResponse, error) {
	rawKey := req.CustomKey
	if rawKey == "" {
		keyBytes := make([]byte, 16)
		if _, err := rand.Read(keyBytes); err != nil {
			return nil, err
		}
		rawKey = "sk-" + hex.EncodeToString(keyBytes)
	}

	keyHash, prefix, err := s.prepareAPIKeyValue(rawKey)
	if err != nil {
		return nil, err
	}

	apiKey := &model.UserAPIKey{
		UserID:                        userID,
		Name:                          req.Name,
		Prefix:                        prefix,
		KeyHash:                       keyHash,
		APIKey:                        rawKey,
		ExpiresAt:                     req.ExpiresAt,
		CircuitBreakerThreshold:       normalizeAPIKeyCircuitBreakerThreshold(req.CircuitBreakerThreshold),
		CircuitBreakerOpenMinutes:     normalizeAPIKeyCircuitBreakerOpenMinutes(req.CircuitBreakerOpenMinutes),
		CircuitBreakerHalfOpenMinutes: normalizeAPIKeyCircuitBreakerHalfOpenMinutes(req.CircuitBreakerHalfOpenMinutes),
		CircuitBreakerState:           model.APIKeyCircuitBreakerStateClosed,
		SplitChannelTargetsBySource:   req.SplitChannelTargetsBySource,
		ChannelTargetsJSON:            model.MustMarshalAPIKeyChannelTargets(req.ChannelTargets),
		SubscriptionChannelTargetsJSON: model.MustMarshalAPIKeyChannelTargets(req.SubscriptionChannelTargets),
		UsageChannelTargetsJSON:       model.MustMarshalAPIKeyChannelTargets(req.UsageChannelTargets),
	}

	if err := s.apiKeyRepo.Create(apiKey); err != nil {
		return nil, err
	}

	return &model.CreateAPIKeyResponse{
		ID:                            apiKey.ID,
		Name:                          apiKey.Name,
		Prefix:                        apiKey.Prefix,
		APIKey:                        rawKey,
		ExpiresAt:                     apiKey.ExpiresAt,
		CircuitBreakerThreshold:       apiKey.CircuitBreakerThreshold,
		CircuitBreakerOpenMinutes:     apiKey.CircuitBreakerOpenMinutes,
		CircuitBreakerHalfOpenMinutes: apiKey.CircuitBreakerHalfOpenMinutes,
		CircuitBreakerState:           apiKey.CircuitBreakerState,
		SplitChannelTargetsBySource:   apiKey.SplitChannelTargetsBySource,
		ChannelTargets:                model.ParseAPIKeyChannelTargets(apiKey.ChannelTargetsJSON),
		SubscriptionChannelTargets:    model.ParseAPIKeyChannelTargets(apiKey.SubscriptionChannelTargetsJSON),
		UsageChannelTargets:           model.ParseAPIKeyChannelTargets(apiKey.UsageChannelTargetsJSON),
		CreatedAt:                     apiKey.CreatedAt,
		Message:                       "API Key 创建成功，请妥善保存，可在列表中再次查看",
	}, nil
}

func (s *AmpService) UpdateAPIKey(userID, keyID string, req *model.UpdateAPIKeyRequest) (*model.APIKeyListItem, error) {
	key, err := s.getOwnedAPIKey(userID, keyID)
	if err != nil {
		return nil, err
	}

	expiresAt := req.ExpiresAt
	if req.ClearExpiry {
		expiresAt = nil
	}
	circuitBreakerThreshold := resolveAPIKeyCircuitBreakerThreshold(req.CircuitBreakerThreshold, key.CircuitBreakerThreshold)
	circuitBreakerOpenMinutes := resolveAPIKeyCircuitBreakerOpenMinutes(req.CircuitBreakerOpenMinutes, key.CircuitBreakerOpenMinutes)
	circuitBreakerHalfOpenMinutes := resolveAPIKeyCircuitBreakerHalfOpenMinutes(req.CircuitBreakerHalfOpenMinutes, key.CircuitBreakerHalfOpenMinutes)
	splitChannelTargetsBySource := key.SplitChannelTargetsBySource
	if req.SplitChannelTargetsBySource != nil {
		splitChannelTargetsBySource = *req.SplitChannelTargetsBySource
	}
	channelTargetsJSON := model.MustMarshalAPIKeyChannelTargets(req.ChannelTargets)
	subscriptionChannelTargetsJSON := model.MustMarshalAPIKeyChannelTargets(req.SubscriptionChannelTargets)
	usageChannelTargetsJSON := model.MustMarshalAPIKeyChannelTargets(req.UsageChannelTargets)

	if strings.TrimSpace(req.APIKey) != "" && req.APIKey != key.APIKey {
		keyHash, prefix, err := s.prepareAPIKeyValue(req.APIKey)
		if err != nil {
			return nil, err
		}
		if err := s.apiKeyRepo.UpdateKeyFields(key.ID, req.Name, prefix, keyHash, req.APIKey, expiresAt, circuitBreakerThreshold, circuitBreakerOpenMinutes, circuitBreakerHalfOpenMinutes); err != nil {
			return nil, err
		}
		key.APIKey = req.APIKey
		key.KeyHash = keyHash
		key.Prefix = prefix
	} else {
		if err := s.apiKeyRepo.UpdateEditableFields(key.ID, req.Name, expiresAt, circuitBreakerThreshold, circuitBreakerOpenMinutes, circuitBreakerHalfOpenMinutes); err != nil {
			return nil, err
		}
	}

	key.Name = req.Name
	key.ExpiresAt = expiresAt
	key.CircuitBreakerThreshold = circuitBreakerThreshold
	key.CircuitBreakerOpenMinutes = circuitBreakerOpenMinutes
	key.CircuitBreakerHalfOpenMinutes = circuitBreakerHalfOpenMinutes
	key.SplitChannelTargetsBySource = splitChannelTargetsBySource
	key.ChannelTargetsJSON = channelTargetsJSON
	key.SubscriptionChannelTargetsJSON = subscriptionChannelTargetsJSON
	key.UsageChannelTargetsJSON = usageChannelTargetsJSON
	if err := s.apiKeyRepo.UpdateTargetFields(key.ID, splitChannelTargetsBySource, channelTargetsJSON, subscriptionChannelTargetsJSON, usageChannelTargetsJSON); err != nil {
		return nil, err
	}
	return buildAPIKeyListItem(key), nil
}

func isValidCustomAPIKey(value string) bool {
	core := value
	if strings.HasPrefix(core, "sk-") {
		core = strings.TrimPrefix(core, "sk-")
	}

	if len(core) < 16 {
		return false
	}

	for _, r := range core {
		isDigit := r >= '0' && r <= '9'
		isLower := r >= 'a' && r <= 'z'
		isUpper := r >= 'A' && r <= 'Z'
		if !isDigit && !isLower && !isUpper {
			return false
		}
	}

	return true
}

func (s *AmpService) ListAPIKeys(userID string) ([]*model.APIKeyListItem, error) {
	keys, err := s.apiKeyRepo.ListByUserID(userID)
	if err != nil {
		return nil, err
	}

	items := make([]*model.APIKeyListItem, 0, len(keys))
	for _, k := range keys {
		if err := s.refreshAPIKeyCircuitBreakerState(k); err != nil {
			return nil, err
		}
		items = append(items, buildAPIKeyListItem(k))
	}
	return items, nil
}

func (s *AmpService) SetAPIKeyDisabled(userID, keyID string, disabled bool) (*model.APIKeyListItem, error) {
	key, err := s.getOwnedAPIKey(userID, keyID)
	if err != nil {
		return nil, err
	}

	if err := s.apiKeyRepo.SetRevoked(key.ID, disabled); err != nil {
		return nil, err
	}

	if disabled {
		now := time.Now().UTC()
		key.RevokedAt = &now
	} else {
		key.RevokedAt = nil
	}

	return buildAPIKeyListItem(key), nil
}

func (s *AmpService) DeleteAPIKey(userID, keyID string) error {
	if _, err := s.getOwnedAPIKey(userID, keyID); err != nil {
		return err
	}
	return s.apiKeyRepo.Delete(keyID)
}

func (s *AmpService) GetAPIKey(userID, keyID string) (*model.APIKeyRevealResponse, error) {
	key, err := s.getOwnedAPIKey(userID, keyID)
	if err != nil {
		return nil, err
	}
	if key.APIKey == "" {
		return nil, ErrAPIKeyNotRetrievable
	}
	return &model.APIKeyRevealResponse{
		ID:                            key.ID,
		Name:                          key.Name,
		Prefix:                        key.Prefix,
		APIKey:                        key.APIKey,
		ExpiresAt:                     key.ExpiresAt,
		CircuitBreakerThreshold:       key.CircuitBreakerThreshold,
		CircuitBreakerOpenMinutes:     key.CircuitBreakerOpenMinutes,
		CircuitBreakerHalfOpenMinutes: key.CircuitBreakerHalfOpenMinutes,
		CircuitBreakerState:           key.CircuitBreakerState,
		SplitChannelTargetsBySource:   key.SplitChannelTargetsBySource,
		ChannelTargets:                model.ParseAPIKeyChannelTargets(key.ChannelTargetsJSON),
		SubscriptionChannelTargets:    model.ParseAPIKeyChannelTargets(key.SubscriptionChannelTargetsJSON),
		UsageChannelTargets:           model.ParseAPIKeyChannelTargets(key.UsageChannelTargetsJSON),
		CreatedAt:                     key.CreatedAt,
	}, nil
}

func (s *AmpService) ListAPIKeysForAdmin(userID string) ([]*model.APIKeyListItem, error) {
	keys, err := s.ListAPIKeys(userID)
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		full, err := s.apiKeyRepo.GetByID(key.ID)
		if err != nil {
			return nil, err
		}
		if full != nil {
			key.APIKey = full.APIKey
		}
	}

	return keys, nil
}

func (s *AmpService) UpdateAPIKeyForAdmin(userID, keyID string, req *model.UpdateAPIKeyRequest) (*model.APIKeyListItem, error) {
	return s.UpdateAPIKey(userID, keyID, req)
}

func (s *AmpService) DeleteAPIKeyForAdmin(userID, keyID string) error {
	return s.DeleteAPIKey(userID, keyID)
}

func (s *AmpService) SetAPIKeyDisabledForAdmin(userID, keyID string, disabled bool) (*model.APIKeyListItem, error) {
	key, err := s.SetAPIKeyDisabled(userID, keyID, disabled)
	if err != nil {
		return nil, err
	}

	full, err := s.apiKeyRepo.GetByID(key.ID)
	if err != nil {
		return nil, err
	}
	if full != nil {
		key.APIKey = full.APIKey
	}

	return key, nil
}

func (s *AmpService) GetBootstrap(userID string) (*model.BootstrapResponse, error) {
	settings, err := s.settingsRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	hasAPIKey, err := s.apiKeyRepo.HasActiveByUserID(userID)
	if err != nil {
		return nil, err
	}

	return &model.BootstrapResponse{
		HasSettings: settings != nil && settings.UpstreamAPIKey != "",
		HasAPIKey:   hasAPIKey,
	}, nil
}

func (s *AmpService) GetClientUsage(userID string) ([]model.CCSwitchUsageItem, error) {
	billingState, err := NewBillingService().GetBillingState(userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	from := now.AddDate(0, 0, -30)
	usageSummary, err := NewRequestLogService().GetUsageSummary(userID, &from, &now, "day", "")
	if err != nil {
		return nil, err
	}

	var recentRequestCount int64
	var recentCostMicros int64
	for _, item := range usageSummary.Items {
		recentRequestCount += item.RequestCount
		recentCostMicros += item.CostMicrosSum
	}

	unitUSD := "USD"
	valid := true
	balancePlanName := "余额"
	balanceRemaining := microsToUSD(billingState.BalanceMicros)
	balanceExtra := fmt.Sprintf("近30天：%d 次请求，$%.6f", recentRequestCount, microsToUSDInt64(recentCostMicros))

	items := []model.CCSwitchUsageItem{
		{
			PlanName:  &balancePlanName,
			Extra:     &balanceExtra,
			IsValid:   &valid,
			Remaining: &balanceRemaining,
			Unit:      &unitUSD,
		},
	}

	subscriptionName := "订阅额度"
	if billingState.Subscription != nil && strings.TrimSpace(billingState.Subscription.PlanName) != "" {
		subscriptionName = billingState.Subscription.PlanName
	}

	for _, window := range billingState.Windows {
		planName := fmt.Sprintf("%s · %s", subscriptionName, formatWindowLabel(window))
		total := microsToUSD(window.LimitMicros)
		used := microsToUSD(window.UsedMicros)
		remaining := microsToUSD(window.LeftMicros)
		extra := fmt.Sprintf("重置时间：%s", window.WindowEnd.UTC().Format("2006-01-02 15:04 UTC"))

		items = append(items, model.CCSwitchUsageItem{
			PlanName:  &planName,
			Extra:     &extra,
			IsValid:   &valid,
			Total:     &total,
			Used:      &used,
			Remaining: &remaining,
			Unit:      &unitUSD,
		})
	}

	return items, nil
}

func (s *AmpService) GetSettingsInternal(userID string) (*model.AmpSettings, error) {
	settings, err := s.settingsRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	if settings != nil && settings.UpstreamAPIKey != "" {
		settings.UpstreamAPIKey = s.decryptUpstreamAPIKey(settings.UpstreamAPIKey)
	}
	return settings, nil
}

func microsToUSD(value int64) float64 {
	return microsToUSDInt64(value)
}

func microsToUSDInt64(value int64) float64 {
	return float64(value) / 1e6
}

func formatWindowLabel(window model.WindowRemaining) string {
	label := ""
	switch window.LimitType {
	case model.LimitTypeDaily:
		label = "日额度"
	case model.LimitTypeWeekly:
		label = "周额度"
	case model.LimitTypeMonthly:
		label = "月额度"
	case model.LimitTypeRolling5h:
		label = "5小时额度"
	case model.LimitTypeTotal:
		label = "总额度"
	default:
		label = "额度"
	}

	if window.WindowMode == model.WindowModeSliding && window.LimitType != model.LimitTypeTotal {
		return label + "（滑动）"
	}

	return label
}

func (s *AmpService) ValidateAPIKey(rawKey string) (*model.UserAPIKey, error) {
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	key, err := s.apiKeyRepo.GetByKeyHash(keyHash)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, ErrAPIKeyNotFound
	}
	if key.RevokedAt != nil {
		return nil, ErrAPIKeyRevoked
	}
	now := time.Now().UTC()
	if err := s.refreshAPIKeyCircuitBreakerStateAt(key, now); err != nil {
		return nil, err
	}
	if model.IsAPIKeyCircuitBreakerBlocked(key, now) {
		return nil, ErrAPIKeyCircuitOpen
	}

	go s.apiKeyRepo.UpdateLastUsedThrottled(key.ID, time.Minute)

	return key, nil
}

func (s *AmpService) prepareAPIKeyValue(rawKey string) (string, string, error) {
	if !isValidCustomAPIKey(rawKey) {
		return "", "", ErrInvalidAPIKeyFormat
	}

	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	existing, err := s.apiKeyRepo.GetByKeyHash(keyHash)
	if err != nil {
		return "", "", err
	}
	if existing != nil {
		return "", "", ErrDuplicateAPIKey
	}

	prefix := rawKey
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	return keyHash, prefix, nil
}

func (s *AmpService) decryptUpstreamAPIKey(storedKey string) string {
	if storedKey == "" {
		return ""
	}

	encKey := config.Get().GetEncryptionKey()
	if encKey == nil {
		return storedKey
	}

	decrypted, err := crypto.Decrypt(storedKey, encKey)
	if err != nil {
		log.Printf("[WARN] Failed to decrypt upstream API key (may be plaintext from before encryption was enabled): %v", err)
		return storedKey
	}
	return string(decrypted)
}

func (s *AmpService) getOwnedAPIKey(userID, keyID string) (*model.UserAPIKey, error) {
	key, err := s.apiKeyRepo.GetByID(keyID)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, ErrAPIKeyNotFound
	}
	if key.UserID != userID {
		return nil, ErrNotOwner
	}
	if err := s.refreshAPIKeyCircuitBreakerState(key); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *AmpService) refreshAPIKeyCircuitBreakerState(key *model.UserAPIKey) error {
	return s.refreshAPIKeyCircuitBreakerStateAt(key, time.Now().UTC())
}

func (s *AmpService) refreshAPIKeyCircuitBreakerStateAt(key *model.UserAPIKey, now time.Time) error {
	if key == nil {
		return nil
	}
	var updateErr error
	WithAPIKeyCircuitBreakerLock(key.ID, func() {
		if model.RefreshAPIKeyCircuitBreakerState(key, now) {
			updateErr = s.apiKeyRepo.UpdateCircuitBreakerState(key)
		}
	})
	if updateErr != nil {
		return updateErr
	}
	return nil
}

func buildAPIKeyListItem(key *model.UserAPIKey) *model.APIKeyListItem {
	status := "active"
	isActive := true

	if key.RevokedAt != nil {
		status = "disabled"
		isActive = false
	} else if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		status = "expired"
		isActive = false
	}

	return &model.APIKeyListItem{
		ID:                              key.ID,
		Name:                            key.Name,
		Prefix:                          key.Prefix,
		CreatedAt:                       key.CreatedAt,
		RevokedAt:                       key.RevokedAt,
		LastUsed:                        key.LastUsed,
		ExpiresAt:                       key.ExpiresAt,
		Status:                          status,
		IsActive:                        isActive,
		CircuitBreakerThreshold:         key.CircuitBreakerThreshold,
		CircuitBreakerOpenMinutes:       key.CircuitBreakerOpenMinutes,
		CircuitBreakerHalfOpenMinutes:   key.CircuitBreakerHalfOpenMinutes,
		CircuitBreakerState:             key.CircuitBreakerState,
		CircuitBreakerOpenedAt:          key.CircuitBreakerOpenedAt,
		CircuitBreakerHalfOpenStartedAt: key.CircuitBreakerHalfOpenStartedAt,
		SplitChannelTargetsBySource:     key.SplitChannelTargetsBySource,
		ChannelTargets:                  model.ParseAPIKeyChannelTargets(key.ChannelTargetsJSON),
		SubscriptionChannelTargets:      model.ParseAPIKeyChannelTargets(key.SubscriptionChannelTargetsJSON),
		UsageChannelTargets:             model.ParseAPIKeyChannelTargets(key.UsageChannelTargetsJSON),
	}
}

func normalizeAPIKeyCircuitBreakerThreshold(value *int) int {
	if value == nil || *value <= 0 {
		return model.DefaultAPIKeyCircuitBreakerThreshold
	}
	return *value
}

func normalizeAPIKeyCircuitBreakerOpenMinutes(value *int) int {
	if value == nil || *value <= 0 {
		return model.DefaultAPIKeyCircuitBreakerOpenMinutes
	}
	return *value
}

func normalizeAPIKeyCircuitBreakerHalfOpenMinutes(value *int) int {
	if value == nil || *value <= 0 {
		return model.DefaultAPIKeyCircuitBreakerHalfOpenMinutes
	}
	return *value
}

func resolveAPIKeyCircuitBreakerThreshold(value *int, current int) int {
	if value == nil {
		if current > 0 {
			return current
		}
		return model.DefaultAPIKeyCircuitBreakerThreshold
	}
	return normalizeAPIKeyCircuitBreakerThreshold(value)
}

func resolveAPIKeyCircuitBreakerOpenMinutes(value *int, current int) int {
	if value == nil {
		if current > 0 {
			return current
		}
		return model.DefaultAPIKeyCircuitBreakerOpenMinutes
	}
	return normalizeAPIKeyCircuitBreakerOpenMinutes(value)
}

func resolveAPIKeyCircuitBreakerHalfOpenMinutes(value *int, current int) int {
	if value == nil {
		if current > 0 {
			return current
		}
		return model.DefaultAPIKeyCircuitBreakerHalfOpenMinutes
	}
	return normalizeAPIKeyCircuitBreakerHalfOpenMinutes(value)
}
