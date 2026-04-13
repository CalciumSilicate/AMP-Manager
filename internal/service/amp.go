package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"ampmanager/internal/config"
	"ampmanager/internal/crypto"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

var (
	ErrAPIKeyNotFound       = errors.New("API Key 不存在")
	ErrAPIKeyRevoked        = errors.New("API Key 已被撤销")
	ErrAPIKeyNotRetrievable = errors.New("API Key 只在创建时显示一次，无法再次获取")
	ErrNotOwner             = errors.New("无权操作此资源")
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
			UpstreamURL:     "https://ampcode.com",
			ModelMappings:   []model.ModelMapping{},
			Enabled:         false,
			HasAPIKey:       false,
			WebSearchMode:   model.WebSearchModeUpstream,
			NativeMode:      false,
			ShowBalanceInAd: false,
			HasSocks5Proxy:  false,
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
		UpstreamURL:     settings.UpstreamURL,
		ModelMappings:   mappings,
		Enabled:         settings.Enabled,
		HasAPIKey:       settings.UpstreamAPIKey != "",
		WebSearchMode:   settings.WebSearchMode,
		NativeMode:      settings.NativeMode,
		ShowBalanceInAd: settings.ShowBalanceInAd,
		HasSocks5Proxy:  settings.Socks5Proxy != "",
		CreatedAt:       settings.CreatedAt,
		UpdatedAt:       settings.UpdatedAt,
	}, nil
}

func (s *AmpService) UpdateSettings(userID string, req *model.AmpSettingsRequest) (*model.AmpSettingsResponse, error) {
	existing, err := s.settingsRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	settings := &model.AmpSettings{
		UserID:        userID,
		UpstreamURL:   req.UpstreamURL,
		Enabled:       req.Enabled,
		WebSearchMode: req.WebSearchMode,
		NativeMode:    req.NativeMode,
	}

	// 处理 ShowBalanceInAd（*bool 指针，nil 表示不修改）
	if req.ShowBalanceInAd != nil {
		settings.ShowBalanceInAd = *req.ShowBalanceInAd
	} else if existing != nil {
		settings.ShowBalanceInAd = existing.ShowBalanceInAd
	}

	// 处理 Socks5Proxy
	if existing != nil && req.Socks5Proxy == "" {
		settings.Socks5Proxy = existing.Socks5Proxy
	} else {
		settings.Socks5Proxy = req.Socks5Proxy
	}

	// 处理 WebSearchMode 默认值
	if settings.WebSearchMode == "" {
		if existing != nil {
			settings.WebSearchMode = existing.WebSearchMode
		} else {
			settings.WebSearchMode = model.WebSearchModeUpstream
		}
	}

	if existing != nil && req.UpstreamAPIKey == "" {
		settings.UpstreamAPIKey = existing.UpstreamAPIKey
	} else if req.UpstreamAPIKey != "" {
		encKey := config.Get().GetEncryptionKey()
		if encKey != nil {
			encrypted, err := crypto.Encrypt([]byte(req.UpstreamAPIKey), encKey)
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt upstream API key: %w", err)
			}
			settings.UpstreamAPIKey = encrypted
		} else {
			log.Println("[WARN] DATA_ENCRYPTION_KEY not set, storing upstream API key in plaintext")
			settings.UpstreamAPIKey = req.UpstreamAPIKey
		}
	}

	if req.ModelMappings != nil {
		mappingsJSON, _ := json.Marshal(req.ModelMappings)
		settings.ModelMappingsJSON = string(mappingsJSON)
	} else if existing != nil {
		settings.ModelMappingsJSON = existing.ModelMappingsJSON
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
		UpstreamURL:     settings.UpstreamURL,
		ModelMappings:   mappings,
		Enabled:         settings.Enabled,
		HasAPIKey:       settings.UpstreamAPIKey != "",
		WebSearchMode:   settings.WebSearchMode,
		NativeMode:      settings.NativeMode,
		ShowBalanceInAd: settings.ShowBalanceInAd,
		HasSocks5Proxy:  settings.Socks5Proxy != "",
		CreatedAt:       settings.CreatedAt,
		UpdatedAt:       settings.UpdatedAt,
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
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}
	rawKey := base64.RawURLEncoding.EncodeToString(keyBytes)

	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	prefix := rawKey[:8]

	apiKey := &model.UserAPIKey{
		UserID:  userID,
		Name:    req.Name,
		Prefix:  prefix,
		KeyHash: keyHash,
		APIKey:  rawKey,
	}

	if err := s.apiKeyRepo.Create(apiKey); err != nil {
		return nil, err
	}

	return &model.CreateAPIKeyResponse{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Prefix:    apiKey.Prefix,
		APIKey:    rawKey,
		CreatedAt: apiKey.CreatedAt,
		Message:   "API Key 创建成功，请妥善保存，可在列表中再次查看",
	}, nil
}

func (s *AmpService) ListAPIKeys(userID string) ([]*model.APIKeyListItem, error) {
	keys, err := s.apiKeyRepo.ListByUserID(userID)
	if err != nil {
		return nil, err
	}

	items := make([]*model.APIKeyListItem, 0, len(keys))
	for _, k := range keys {
		if k.RevokedAt != nil {
			continue
		}
		items = append(items, &model.APIKeyListItem{
			ID:        k.ID,
			Name:      k.Name,
			Prefix:    k.Prefix,
			CreatedAt: k.CreatedAt,
			RevokedAt: k.RevokedAt,
			LastUsed:  k.LastUsed,
			IsActive:  k.RevokedAt == nil,
		})
	}
	return items, nil
}

func (s *AmpService) DeleteAPIKey(userID, keyID string) error {
	key, err := s.apiKeyRepo.GetByID(keyID)
	if err != nil {
		return err
	}
	if key == nil {
		return ErrAPIKeyNotFound
	}
	if key.UserID != userID {
		return ErrNotOwner
	}
	return s.apiKeyRepo.Delete(keyID)
}

func (s *AmpService) GetAPIKey(userID, keyID string) (*model.APIKeyRevealResponse, error) {
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
	if key.APIKey == "" {
		return nil, ErrAPIKeyNotRetrievable
	}
	return &model.APIKeyRevealResponse{
		ID:        key.ID,
		Name:      key.Name,
		Prefix:    key.Prefix,
		APIKey:    key.APIKey,
		CreatedAt: key.CreatedAt,
	}, nil
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

	go s.apiKeyRepo.UpdateLastUsedThrottled(key.ID, time.Minute)

	return key, nil
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
