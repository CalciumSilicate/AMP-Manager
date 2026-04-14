package service

import (
	"ampmanager/internal/billingstate"
	"ampmanager/internal/config"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	retryConfigKey                 = "retry_config"
	requestDetailEnabledKey        = "request_detail_enabled"
	requestDetailTTLSecKey         = "request_detail_ttl_sec"
	requestDetailMaxEntriesKey     = "request_detail_max_entries"
	requestDetailMaxMemoryMBKey    = "request_detail_max_memory_mb"
	requestDetailBodyCapKBKey      = "request_detail_body_cap_kb"
	requestDetailPersistEnabledKey = "request_detail_persist_enabled"
	timeoutConfigKey               = "timeout_config"
	cacheTTLOverrideKey            = "cache_ttl_override"
	siteNameKey                    = "site_name"
	billingRuntimeConfigKey        = "billing_runtime_config"
	defaultSiteName                = "AMP Manager"
)

type SystemConfigService struct {
	repo *repository.SystemConfigRepository
}

func NewSystemConfigService() *SystemConfigService {
	return &SystemConfigService{
		repo: repository.NewSystemConfigRepository(),
	}
}

// GetRetryConfigJSON 获取重试配置的 JSON 字符串
func (s *SystemConfigService) GetRetryConfigJSON() (string, error) {
	return s.repo.Get(retryConfigKey)
}

// SetRetryConfigJSON 保存重试配置的 JSON 字符串
func (s *SystemConfigService) SetRetryConfigJSON(value string) error {
	return s.repo.Set(retryConfigKey, value)
}

// GetRequestDetailEnabled 获取请求详情监控是否启用
func (s *SystemConfigService) GetRequestDetailEnabled() (bool, error) {
	cfg, err := s.GetRequestDetailConfig()
	if err != nil {
		return true, nil // 默认启用
	}
	return cfg.Enabled, nil
}

// SetRequestDetailEnabled 设置请求详情监控是否启用
func (s *SystemConfigService) SetRequestDetailEnabled(enabled bool) error {
	cfg, err := s.GetRequestDetailConfig()
	if err != nil {
		return err
	}
	cfg.Enabled = enabled
	return s.SetRequestDetailConfig(cfg)
}

func (s *SystemConfigService) GetRequestDetailConfig() (model.RequestDetailConfigResponse, error) {
	resp := model.RequestDetailConfigResponse{
		Enabled:        true,
		TTLSec:         120,
		MaxEntries:     500,
		MaxMemoryMB:    256,
		BodyCapKB:      128,
		PersistEnabled: true,
	}

	if value, err := s.repo.Get(requestDetailEnabledKey); err == nil && value != "" {
		resp.Enabled = value != "false"
	}
	if value, err := s.repo.Get(requestDetailTTLSecKey); err == nil && value != "" {
		if parsed, parseErr := time.ParseDuration(value + "s"); parseErr == nil {
			resp.TTLSec = int64(parsed / time.Second)
		}
	}
	if value, err := s.repo.Get(requestDetailMaxEntriesKey); err == nil && value != "" {
		if parsed, parseErr := parsePositiveInt(value); parseErr == nil {
			resp.MaxEntries = parsed
		}
	}
	if value, err := s.repo.Get(requestDetailMaxMemoryMBKey); err == nil && value != "" {
		if parsed, parseErr := parsePositiveInt64(value); parseErr == nil {
			resp.MaxMemoryMB = parsed
		}
	}
	if value, err := s.repo.Get(requestDetailBodyCapKBKey); err == nil && value != "" {
		if parsed, parseErr := parsePositiveInt(value); parseErr == nil {
			resp.BodyCapKB = parsed
		}
	}
	if value, err := s.repo.Get(requestDetailPersistEnabledKey); err == nil && value != "" {
		resp.PersistEnabled = value != "false"
	}

	return resp, nil
}

func (s *SystemConfigService) SetRequestDetailConfig(req model.RequestDetailConfigResponse) error {
	entries := map[string]string{
		requestDetailEnabledKey:        boolToConfigString(req.Enabled),
		requestDetailTTLSecKey:         formatInt64(req.TTLSec),
		requestDetailMaxEntriesKey:     formatInt(req.MaxEntries),
		requestDetailMaxMemoryMBKey:    formatInt64(req.MaxMemoryMB),
		requestDetailBodyCapKBKey:      formatInt(req.BodyCapKB),
		requestDetailPersistEnabledKey: boolToConfigString(req.PersistEnabled),
	}

	for key, value := range entries {
		if err := s.repo.Set(key, value); err != nil {
			return err
		}
	}

	return nil
}

// GetTimeoutConfigJSON 获取超时配置的 JSON 字符串
func (s *SystemConfigService) GetTimeoutConfigJSON() (string, error) {
	return s.repo.Get(timeoutConfigKey)
}

// GetCacheTTLOverride 获取缓存 TTL 覆盖配置
func (s *SystemConfigService) GetCacheTTLOverride() (string, error) {
	return s.repo.Get(cacheTTLOverrideKey)
}

func (s *SystemConfigService) GetSiteConfig() (model.SiteConfigResponse, error) {
	value, err := s.repo.Get(siteNameKey)
	if err != nil {
		return model.SiteConfigResponse{}, err
	}

	siteName := strings.TrimSpace(value)
	if siteName == "" {
		siteName = defaultSiteName
	}

	return model.SiteConfigResponse{SiteName: siteName}, nil
}

func (s *SystemConfigService) SetSiteConfig(req model.SiteConfigRequest) (model.SiteConfigResponse, error) {
	siteName := strings.TrimSpace(req.SiteName)
	if siteName == "" || siteName == defaultSiteName {
		if err := s.repo.Delete(siteNameKey); err != nil {
			return model.SiteConfigResponse{}, err
		}
		return model.SiteConfigResponse{SiteName: defaultSiteName}, nil
	}

	if err := s.repo.Set(siteNameKey, siteName); err != nil {
		return model.SiteConfigResponse{}, err
	}

	return model.SiteConfigResponse{SiteName: siteName}, nil
}

func boolToConfigString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func formatInt(value int) string {
	return formatInt64(int64(value))
}

func formatInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}

func parsePositiveInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func parsePositiveInt64(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}

func defaultBillingRuntimeConfigRequest() model.BillingRuntimeConfigRequest {
	resp := model.BillingRuntimeConfigRequest{
		RedisPrefix:          "ampmanager",
		ReservationTTLSec:    600,
		ReconcileIntervalSec: 60,
		StreamBatchSize:      100,
	}

	if cfg := config.Get(); cfg != nil {
		resp.RedisURL = strings.TrimSpace(cfg.RedisURL)
		if strings.TrimSpace(cfg.RedisPrefix) != "" {
			resp.RedisPrefix = strings.TrimSpace(cfg.RedisPrefix)
		}
		if cfg.BillingReservationTTLSec > 0 {
			resp.ReservationTTLSec = cfg.BillingReservationTTLSec
		}
		if cfg.BillingReconcileIntervalSec > 0 {
			resp.ReconcileIntervalSec = cfg.BillingReconcileIntervalSec
		}
		if cfg.BillingStreamBatchSize > 0 {
			resp.StreamBatchSize = cfg.BillingStreamBatchSize
		}
	}

	return normalizeBillingRuntimeConfigRequest(resp)
}

func normalizeBillingRuntimeConfigRequest(req model.BillingRuntimeConfigRequest) model.BillingRuntimeConfigRequest {
	req.RedisURL = strings.TrimSpace(req.RedisURL)
	req.RedisPrefix = strings.TrimSpace(req.RedisPrefix)
	if req.RedisPrefix == "" {
		req.RedisPrefix = "ampmanager"
	}
	if req.ReservationTTLSec <= 0 {
		req.ReservationTTLSec = 600
	}
	if req.ReconcileIntervalSec <= 0 {
		req.ReconcileIntervalSec = 60
	}
	if req.StreamBatchSize <= 0 {
		req.StreamBatchSize = 100
	}
	return req
}

func (s *SystemConfigService) GetBillingRuntimeConfigRequest() (model.BillingRuntimeConfigRequest, error) {
	resp := defaultBillingRuntimeConfigRequest()

	value, err := s.repo.Get(billingRuntimeConfigKey)
	if err != nil {
		return resp, err
	}
	if value == "" {
		return resp, nil
	}

	var stored model.BillingRuntimeConfigRequest
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return resp, nil
	}

	resp.RedisURL = strings.TrimSpace(stored.RedisURL)
	if strings.TrimSpace(stored.RedisPrefix) != "" {
		resp.RedisPrefix = strings.TrimSpace(stored.RedisPrefix)
	}
	if stored.ReservationTTLSec > 0 {
		resp.ReservationTTLSec = stored.ReservationTTLSec
	}
	if stored.ReconcileIntervalSec > 0 {
		resp.ReconcileIntervalSec = stored.ReconcileIntervalSec
	}
	if stored.StreamBatchSize > 0 {
		resp.StreamBatchSize = stored.StreamBatchSize
	}

	return normalizeBillingRuntimeConfigRequest(resp), nil
}

func (s *SystemConfigService) GetBillingRuntimeConfig() (model.BillingRuntimeConfigResponse, error) {
	req, err := s.GetBillingRuntimeConfigRequest()
	if err != nil {
		return model.BillingRuntimeConfigResponse{}, err
	}

	resp := model.BillingRuntimeConfigResponse{
		RedisURL:             req.RedisURL,
		RedisURLMasked:       maskRedisURL(req.RedisURL),
		RedisPrefix:          req.RedisPrefix,
		ReservationTTLSec:    req.ReservationTTLSec,
		ReconcileIntervalSec: req.ReconcileIntervalSec,
		StreamBatchSize:      req.StreamBatchSize,
		RuntimeEnabled:       strings.TrimSpace(req.RedisURL) != "",
	}
	resp.RuntimeHealthy = resp.RuntimeEnabled && billingstate.Get() != nil
	return resp, nil
}

func (s *SystemConfigService) SetBillingRuntimeConfig(req model.BillingRuntimeConfigRequest) error {
	req = normalizeBillingRuntimeConfigRequest(req)
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return s.repo.Set(billingRuntimeConfigKey, string(data))
}

func maskRedisURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	re := regexp.MustCompile(`://([^:/?#]+):([^@/?#]+)@`)
	return re.ReplaceAllString(raw, `://$1:******@`)
}
