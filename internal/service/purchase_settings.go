package service

import (
	"strings"

	"ampmanager/internal/config"
	"ampmanager/internal/crypto"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	log "github.com/sirupsen/logrus"
)

const (
	purchaseEnabledKey          = "purchase_enabled"
	purchaseDebugAutoPaidKey    = "purchase_debug_auto_paid"
	purchaseAlipayAppIDKey      = "purchase_alipay_app_id"
	purchaseAlipayPIDKey        = "purchase_alipay_pid"
	purchaseAlipayEnvKey        = "purchase_alipay_environment"
	purchaseAlipayNotifyURLKey  = "purchase_alipay_notify_url"
	purchaseAlipayPublicKeyKey  = "purchase_alipay_public_key"
	purchaseAlipayPrivateKeyKey = "purchase_alipay_private_key"
)

type PurchaseSettingsService struct {
	repo *repository.SystemConfigRepository
}

func NewPurchaseSettingsService() *PurchaseSettingsService {
	return &PurchaseSettingsService{
		repo: repository.NewSystemConfigRepository(),
	}
}

func (s *PurchaseSettingsService) Get() (*model.PurchaseSettings, error) {
	settings := &model.PurchaseSettings{
		PurchaseEnabled:   false,
		DebugAutoPaid:     false,
		AlipayEnvironment: model.AlipayEnvironmentSandbox,
	}

	if value, err := s.repo.Get(purchaseEnabledKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		settings.PurchaseEnabled = value == "true"
	}

	if value, err := s.repo.Get(purchaseDebugAutoPaidKey); err != nil {
		return nil, err
	} else if strings.TrimSpace(value) != "" {
		settings.DebugAutoPaid = value == "true"
	}

	values := map[string]*string{
		purchaseAlipayAppIDKey:     &settings.AlipayAppID,
		purchaseAlipayPIDKey:       &settings.AlipayPID,
		purchaseAlipayNotifyURLKey: &settings.AlipayNotifyURL,
		purchaseAlipayPublicKeyKey: &settings.AlipayPublicKey,
	}
	for key, target := range values {
		value, err := s.repo.Get(key)
		if err != nil {
			return nil, err
		}
		*target = strings.TrimSpace(value)
	}

	envValue, err := s.repo.Get(purchaseAlipayEnvKey)
	if err != nil {
		return nil, err
	}
	switch model.AlipayEnvironment(strings.TrimSpace(envValue)) {
	case model.AlipayEnvironmentProduction:
		settings.AlipayEnvironment = model.AlipayEnvironmentProduction
	default:
		settings.AlipayEnvironment = model.AlipayEnvironmentSandbox
	}

	privateKeyValue, err := s.repo.Get(purchaseAlipayPrivateKeyKey)
	if err != nil {
		return nil, err
	}
	settings.AlipayPrivateKey = s.decryptPrivateKey(strings.TrimSpace(privateKeyValue))

	return settings, nil
}

func (s *PurchaseSettingsService) Update(req *model.PurchaseSettingsRequest) (*model.PurchaseSettingsResponse, error) {
	current, err := s.Get()
	if err != nil {
		return nil, err
	}

	settings := &model.PurchaseSettings{
		PurchaseEnabled:   req.PurchaseEnabled,
		DebugAutoPaid:     req.DebugAutoPaid,
		AlipayAppID:       strings.TrimSpace(req.AlipayAppID),
		AlipayPID:         strings.TrimSpace(req.AlipayPID),
		AlipayNotifyURL:   strings.TrimSpace(req.AlipayNotifyURL),
		AlipayPublicKey:   strings.TrimSpace(req.AlipayPublicKey),
		AlipayEnvironment: req.AlipayEnvironment,
		AlipayPrivateKey:  current.AlipayPrivateKey,
	}
	if settings.AlipayEnvironment == "" {
		settings.AlipayEnvironment = model.AlipayEnvironmentSandbox
	}
	if privateKey := strings.TrimSpace(req.AlipayPrivateKey); privateKey != "" {
		settings.AlipayPrivateKey = privateKey
	}

	if err := s.repo.Set(purchaseEnabledKey, boolToConfigString(settings.PurchaseEnabled)); err != nil {
		return nil, err
	}
	if err := s.repo.Set(purchaseDebugAutoPaidKey, boolToConfigString(settings.DebugAutoPaid)); err != nil {
		return nil, err
	}
	if err := s.repo.Set(purchaseAlipayAppIDKey, settings.AlipayAppID); err != nil {
		return nil, err
	}
	if err := s.repo.Set(purchaseAlipayPIDKey, settings.AlipayPID); err != nil {
		return nil, err
	}
	if err := s.repo.Set(purchaseAlipayNotifyURLKey, settings.AlipayNotifyURL); err != nil {
		return nil, err
	}
	if err := s.repo.Set(purchaseAlipayPublicKeyKey, settings.AlipayPublicKey); err != nil {
		return nil, err
	}
	if err := s.repo.Set(purchaseAlipayEnvKey, string(settings.AlipayEnvironment)); err != nil {
		return nil, err
	}

	encryptedPrivateKey := ""
	if settings.AlipayPrivateKey != "" {
		encryptedPrivateKey, err = s.encryptPrivateKey(settings.AlipayPrivateKey)
		if err != nil {
			return nil, err
		}
	}
	if err := s.repo.Set(purchaseAlipayPrivateKeyKey, encryptedPrivateKey); err != nil {
		return nil, err
	}

	return s.ToResponse(settings), nil
}

func (s *PurchaseSettingsService) ToResponse(settings *model.PurchaseSettings) *model.PurchaseSettingsResponse {
	if settings == nil {
		return &model.PurchaseSettingsResponse{
			AlipayEnvironment: model.AlipayEnvironmentSandbox,
		}
	}

	return &model.PurchaseSettingsResponse{
		PurchaseEnabled:   settings.PurchaseEnabled,
		DebugAutoPaid:     settings.DebugAutoPaid,
		AlipayAppID:       settings.AlipayAppID,
		AlipayPID:         settings.AlipayPID,
		AlipayEnvironment: settings.AlipayEnvironment,
		AlipayNotifyURL:   settings.AlipayNotifyURL,
		AlipayPublicKey:   settings.AlipayPublicKey,
		PrivateKeySet:     strings.TrimSpace(settings.AlipayPrivateKey) != "",
		PaymentConfigured: s.IsAlipayConfigured(settings),
	}
}

func (s *PurchaseSettingsService) IsAlipayConfigured(settings *model.PurchaseSettings) bool {
	if settings == nil {
		return false
	}
	return strings.TrimSpace(settings.AlipayAppID) != "" &&
		strings.TrimSpace(settings.AlipayNotifyURL) != "" &&
		strings.TrimSpace(settings.AlipayPublicKey) != "" &&
		strings.TrimSpace(settings.AlipayPrivateKey) != ""
}

func (s *PurchaseSettingsService) CanCreateOrders(settings *model.PurchaseSettings) bool {
	if settings == nil || !settings.PurchaseEnabled {
		return false
	}
	return settings.DebugAutoPaid || s.IsAlipayConfigured(settings)
}

func (s *PurchaseSettingsService) encryptPrivateKey(value string) (string, error) {
	key := config.Get().GetEncryptionKey()
	if key == nil {
		log.Warn("DATA_ENCRYPTION_KEY not set, storing Alipay private key in plaintext")
		return value, nil
	}
	return crypto.Encrypt([]byte(value), key)
}

func (s *PurchaseSettingsService) decryptPrivateKey(value string) string {
	if value == "" {
		return ""
	}

	key := config.Get().GetEncryptionKey()
	if key == nil {
		return value
	}

	decrypted, err := crypto.Decrypt(value, key)
	if err != nil {
		log.Warnf("failed to decrypt Alipay private key, assuming plaintext: %v", err)
		return value
	}
	return string(decrypted)
}
