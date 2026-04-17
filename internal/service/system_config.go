package service

import (
	"ampmanager/internal/billingstate"
	"ampmanager/internal/config"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	retryConfigKey                       = "retry_config"
	requestDetailEnabledKey              = "request_detail_enabled"
	requestDetailTTLSecKey               = "request_detail_ttl_sec"
	requestDetailMaxEntriesKey           = "request_detail_max_entries"
	requestDetailMaxMemoryMBKey          = "request_detail_max_memory_mb"
	requestDetailBodyCapKBKey            = "request_detail_body_cap_kb"
	requestDetailPersistEnabledKey       = "request_detail_persist_enabled"
	requestDetailHighRPMModeKey          = "request_detail_high_rpm_mode"
	requestDetailHighRPMThresholdKey     = "request_detail_high_rpm_threshold"
	requestDetailHighRPMSamplePctKey     = "request_detail_high_rpm_sample_percent"
	requestPayloadMaxBytesKey            = "request_payload_max_bytes"
	timeoutConfigKey                     = "timeout_config"
	cacheTTLOverrideKey                  = "cache_ttl_override"
	sessionStickyEnabledKey              = "session_sticky_enabled"
	sessionStickyWindowMinutesKey        = "session_sticky_window_minutes"
	sessionStickyLogSearchMinCharsKey    = "session_sticky_log_search_min_chars"
	sessionStickyInjectPromptCacheKeyKey = "session_sticky_inject_prompt_cache_key_responses"
	siteNameKey                          = "site_name"
	allowAmpProxySettingsKey             = "allow_amp_proxy_settings"
	allowAmpSettingsKey                  = "allow_amp_settings"
	siteContactEnabledKey                = "site_contact_enabled"
	siteContactTitleKey                  = "site_contact_title"
	siteContactDescriptionKey            = "site_contact_description"
	siteContactLinkKey                   = "site_contact_link"
	billingDailyResetEnabledKey          = "billing_daily_reset_enabled"
	billingDailyResetMinDaysKey          = "billing_daily_reset_min_remaining_days"
	billingDailyResetThresholdPctKey     = "billing_daily_reset_usage_threshold_percent"
	billingDailyResetDailyLimitKey       = "billing_daily_reset_daily_limit"
	billingRuntimeConfigKey              = "billing_runtime_config"
	defaultSiteName                      = "AMP Manager"
	defaultRequestDetailTTLSec           = 120
	defaultRequestDetailMaxEntries       = 500
	defaultRequestDetailMaxMemoryMB      = 256
	defaultRequestDetailBodyCapKB        = 128
	defaultRequestDetailHighRPMMode      = "full"
	defaultRequestDetailHighRPMThreshold = 3000
	defaultRequestDetailHighRPMSamplePct = 10
	defaultRequestPayloadMaxBytes        = 128 * 1024 * 1024
	siteTimeZoneKey                      = "site_time_zone"
	defaultSiteTimeZone                  = "Asia/Shanghai"
	defaultAmpProxySettingsPolicy        = model.AmpProxySettingsPolicyAll
	defaultAmpSettingsPolicy             = model.AmpProxySettingsPolicyAll
	defaultBillingDailyResetEnabled      = true
	defaultBillingDailyResetMinDays      = 2
	defaultBillingDailyResetThresholdPct = 90
	defaultBillingDailyResetDailyLimit   = 2
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
	resp := defaultRequestDetailConfigResponse()

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
	if value, err := s.repo.Get(requestDetailHighRPMModeKey); err == nil && value != "" {
		resp.HighRPMMode = strings.TrimSpace(value)
	}
	if value, err := s.repo.Get(requestDetailHighRPMThresholdKey); err == nil && value != "" {
		if parsed, parseErr := parsePositiveInt(value); parseErr == nil {
			resp.HighRPMThreshold = parsed
		}
	}
	if value, err := s.repo.Get(requestDetailHighRPMSamplePctKey); err == nil && value != "" {
		if parsed, parseErr := parsePositiveInt(value); parseErr == nil {
			resp.HighRPMSamplePercent = parsed
		}
	}

	return normalizeRequestDetailConfigResponse(resp), nil
}

func (s *SystemConfigService) GetRequestPayloadLimit() (model.RequestPayloadLimitResponse, error) {
	value, err := s.repo.Get(requestPayloadMaxBytesKey)
	if err != nil {
		return model.RequestPayloadLimitResponse{}, err
	}
	if strings.TrimSpace(value) == "" {
		return model.RequestPayloadLimitResponse{MaxBytes: defaultRequestPayloadMaxBytes}, nil
	}
	parsed, err := parsePositiveInt64(value)
	if err != nil || parsed <= 0 {
		return model.RequestPayloadLimitResponse{MaxBytes: defaultRequestPayloadMaxBytes}, nil
	}
	return model.RequestPayloadLimitResponse{MaxBytes: parsed}, nil
}

func (s *SystemConfigService) SetRequestPayloadLimit(req model.RequestPayloadLimitRequest) (model.RequestPayloadLimitResponse, error) {
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultRequestPayloadMaxBytes
	}
	if maxBytes == defaultRequestPayloadMaxBytes {
		if err := s.repo.Delete(requestPayloadMaxBytesKey); err != nil {
			return model.RequestPayloadLimitResponse{}, err
		}
		return model.RequestPayloadLimitResponse{MaxBytes: defaultRequestPayloadMaxBytes}, nil
	}
	if err := s.repo.Set(requestPayloadMaxBytesKey, formatInt64(maxBytes)); err != nil {
		return model.RequestPayloadLimitResponse{}, err
	}
	return model.RequestPayloadLimitResponse{MaxBytes: maxBytes}, nil
}

func (s *SystemConfigService) SetRequestDetailConfig(req model.RequestDetailConfigResponse) error {
	req = normalizeRequestDetailConfigResponse(req)
	entries := map[string]string{
		requestDetailEnabledKey:          boolToConfigString(req.Enabled),
		requestDetailTTLSecKey:           formatInt64(req.TTLSec),
		requestDetailMaxEntriesKey:       formatInt(req.MaxEntries),
		requestDetailMaxMemoryMBKey:      formatInt64(req.MaxMemoryMB),
		requestDetailBodyCapKBKey:        formatInt(req.BodyCapKB),
		requestDetailPersistEnabledKey:   boolToConfigString(req.PersistEnabled),
		requestDetailHighRPMModeKey:      req.HighRPMMode,
		requestDetailHighRPMThresholdKey: formatInt(req.HighRPMThreshold),
		requestDetailHighRPMSamplePctKey: formatInt(req.HighRPMSamplePercent),
	}

	for key, value := range entries {
		if err := s.repo.Set(key, value); err != nil {
			return err
		}
	}

	return nil
}

func defaultRequestDetailConfigResponse() model.RequestDetailConfigResponse {
	return model.RequestDetailConfigResponse{
		Enabled:              true,
		TTLSec:               defaultRequestDetailTTLSec,
		MaxEntries:           defaultRequestDetailMaxEntries,
		MaxMemoryMB:          defaultRequestDetailMaxMemoryMB,
		BodyCapKB:            defaultRequestDetailBodyCapKB,
		PersistEnabled:       true,
		HighRPMMode:          defaultRequestDetailHighRPMMode,
		HighRPMThreshold:     defaultRequestDetailHighRPMThreshold,
		HighRPMSamplePercent: defaultRequestDetailHighRPMSamplePct,
	}
}

func normalizeRequestDetailConfigResponse(req model.RequestDetailConfigResponse) model.RequestDetailConfigResponse {
	defaults := defaultRequestDetailConfigResponse()
	if req.TTLSec <= 0 {
		req.TTLSec = defaults.TTLSec
	}
	if req.MaxEntries <= 0 {
		req.MaxEntries = defaults.MaxEntries
	}
	if req.MaxMemoryMB <= 0 {
		req.MaxMemoryMB = defaults.MaxMemoryMB
	}
	if req.BodyCapKB <= 0 {
		req.BodyCapKB = defaults.BodyCapKB
	}
	switch strings.TrimSpace(req.HighRPMMode) {
	case "full", "off", "sample":
		req.HighRPMMode = strings.TrimSpace(req.HighRPMMode)
	default:
		req.HighRPMMode = defaults.HighRPMMode
	}
	if req.HighRPMThreshold <= 0 {
		req.HighRPMThreshold = defaults.HighRPMThreshold
	}
	if req.HighRPMSamplePercent <= 0 {
		req.HighRPMSamplePercent = defaults.HighRPMSamplePercent
	}
	if req.HighRPMSamplePercent > 100 {
		req.HighRPMSamplePercent = 100
	}
	return req
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
	siteNameValue, err := s.repo.Get(siteNameKey)
	if err != nil {
		return model.SiteConfigResponse{}, err
	}
	timeZoneValue, err := s.repo.Get(siteTimeZoneKey)
	if err != nil {
		return model.SiteConfigResponse{}, err
	}
	ampProxySettingsPolicy, err := s.GetAmpProxySettingsPolicy()
	if err != nil {
		return model.SiteConfigResponse{}, err
	}
	ampSettingsPolicy, err := s.GetAmpSettingsPolicy()
	if err != nil {
		return model.SiteConfigResponse{}, err
	}
	contact, err := s.getSiteContactConfig()
	if err != nil {
		return model.SiteConfigResponse{}, err
	}
	sessionSticky, err := s.GetSessionStickyConfig()
	if err != nil {
		return model.SiteConfigResponse{}, err
	}
	inviteEnabled := NewInviteService().IsPublicEnabled()

	siteName := strings.TrimSpace(siteNameValue)
	if siteName == "" {
		siteName = defaultSiteName
	}
	timeZone := strings.TrimSpace(timeZoneValue)
	if timeZone == "" {
		timeZone = defaultSiteTimeZone
	}

	return model.SiteConfigResponse{
		SiteName:               siteName,
		TimeZone:               timeZone,
		AmpProxySettingsPolicy: ampProxySettingsPolicy,
		AmpSettingsPolicy:      ampSettingsPolicy,
		Contact:                contact,
		InviteEnabled:          inviteEnabled,
		SessionSticky: &model.SessionStickyPublicConfig{
			Enabled:           sessionSticky.Enabled,
			LogSearchMinChars: sessionSticky.LogSearchMinChars,
		},
	}, nil
}

func (s *SystemConfigService) GetSessionStickyConfig() (model.SessionStickyConfigResponse, error) {
	resp := model.SessionStickyConfigResponse{
		Enabled:                       false,
		WindowMinutes:                 5,
		LogSearchMinChars:             4,
		InjectPromptCacheKeyResponses: false,
	}

	if value, err := s.repo.Get(sessionStickyEnabledKey); err == nil && value != "" {
		resp.Enabled = value == "true"
	}
	if value, err := s.repo.Get(sessionStickyWindowMinutesKey); err == nil && value != "" {
		if parsed, parseErr := parsePositiveInt(value); parseErr == nil && parsed > 0 {
			resp.WindowMinutes = parsed
		}
	}
	if value, err := s.repo.Get(sessionStickyLogSearchMinCharsKey); err == nil && value != "" {
		if parsed, parseErr := parsePositiveInt(value); parseErr == nil && parsed > 0 {
			resp.LogSearchMinChars = parsed
		}
	}
	if value, err := s.repo.Get(sessionStickyInjectPromptCacheKeyKey); err == nil && value != "" {
		resp.InjectPromptCacheKeyResponses = value == "true"
	}

	return resp, nil
}

func (s *SystemConfigService) SetSessionStickyConfig(req model.SessionStickyConfigRequest) (model.SessionStickyConfigResponse, error) {
	resp := model.SessionStickyConfigResponse{
		Enabled:                       req.Enabled,
		WindowMinutes:                 req.WindowMinutes,
		LogSearchMinChars:             req.LogSearchMinChars,
		InjectPromptCacheKeyResponses: req.InjectPromptCacheKeyResponses,
	}
	if resp.WindowMinutes <= 0 {
		resp.WindowMinutes = 5
	}
	if resp.LogSearchMinChars <= 0 {
		resp.LogSearchMinChars = 4
	}

	entries := map[string]string{
		sessionStickyEnabledKey:              boolToConfigString(resp.Enabled),
		sessionStickyWindowMinutesKey:        formatInt(resp.WindowMinutes),
		sessionStickyLogSearchMinCharsKey:    formatInt(resp.LogSearchMinChars),
		sessionStickyInjectPromptCacheKeyKey: boolToConfigString(resp.InjectPromptCacheKeyResponses),
	}
	for key, value := range entries {
		if err := s.repo.Set(key, value); err != nil {
			return model.SessionStickyConfigResponse{}, err
		}
	}

	return resp, nil
}

func (s *SystemConfigService) SetSiteConfig(req model.SiteConfigRequest) (model.SiteConfigResponse, error) {
	siteName := strings.TrimSpace(req.SiteName)
	timeZone := strings.TrimSpace(req.TimeZone)
	ampProxySettingsPolicy, err := s.GetAmpProxySettingsPolicy()
	if err != nil {
		return model.SiteConfigResponse{}, err
	}
	ampSettingsPolicy, err := s.GetAmpSettingsPolicy()
	if err != nil {
		return model.SiteConfigResponse{}, err
	}
	if req.AmpProxySettingsPolicy != nil {
		ampProxySettingsPolicy = normalizeAmpProxySettingsPolicy(*req.AmpProxySettingsPolicy)
	}
	if req.AmpSettingsPolicy != nil {
		ampSettingsPolicy = normalizeAmpProxySettingsPolicy(*req.AmpSettingsPolicy)
	}
	contact, err := s.getSiteContactConfig()
	if err != nil {
		return model.SiteConfigResponse{}, err
	}
	if req.Contact != nil {
		contact.Enabled = req.Contact.Enabled
		contact.Title = strings.TrimSpace(req.Contact.Title)
		contact.Description = strings.TrimSpace(req.Contact.Description)
		contact.Link = strings.TrimSpace(req.Contact.Link)
	}

	if timeZone == "" {
		timeZone = defaultSiteTimeZone
	}

	if siteName == "" || siteName == defaultSiteName {
		if err := s.repo.Delete(siteNameKey); err != nil {
			return model.SiteConfigResponse{}, err
		}
		siteName = defaultSiteName
	} else {
		if err := s.repo.Set(siteNameKey, siteName); err != nil {
			return model.SiteConfigResponse{}, err
		}
	}

	if timeZone == defaultSiteTimeZone {
		if err := s.repo.Delete(siteTimeZoneKey); err != nil {
			return model.SiteConfigResponse{}, err
		}
	} else {
		if err := s.repo.Set(siteTimeZoneKey, timeZone); err != nil {
			return model.SiteConfigResponse{}, err
		}
	}

	if ampProxySettingsPolicy == defaultAmpProxySettingsPolicy {
		if err := s.repo.Delete(allowAmpProxySettingsKey); err != nil {
			return model.SiteConfigResponse{}, err
		}
	} else {
		if err := s.repo.Set(allowAmpProxySettingsKey, ampProxySettingsPolicy); err != nil {
			return model.SiteConfigResponse{}, err
		}
	}
	if ampSettingsPolicy == defaultAmpSettingsPolicy {
		if err := s.repo.Delete(allowAmpSettingsKey); err != nil {
			return model.SiteConfigResponse{}, err
		}
	} else {
		if err := s.repo.Set(allowAmpSettingsKey, ampSettingsPolicy); err != nil {
			return model.SiteConfigResponse{}, err
		}
	}
	if !contact.Enabled {
		if err := s.repo.Delete(siteContactEnabledKey); err != nil {
			return model.SiteConfigResponse{}, err
		}
	} else {
		if err := s.repo.Set(siteContactEnabledKey, boolToConfigString(true)); err != nil {
			return model.SiteConfigResponse{}, err
		}
	}
	if contact.Title == "" {
		if err := s.repo.Delete(siteContactTitleKey); err != nil {
			return model.SiteConfigResponse{}, err
		}
	} else {
		if err := s.repo.Set(siteContactTitleKey, contact.Title); err != nil {
			return model.SiteConfigResponse{}, err
		}
	}
	if contact.Description == "" {
		if err := s.repo.Delete(siteContactDescriptionKey); err != nil {
			return model.SiteConfigResponse{}, err
		}
	} else {
		if err := s.repo.Set(siteContactDescriptionKey, contact.Description); err != nil {
			return model.SiteConfigResponse{}, err
		}
	}
	if contact.Link == "" {
		if err := s.repo.Delete(siteContactLinkKey); err != nil {
			return model.SiteConfigResponse{}, err
		}
	} else {
		if err := s.repo.Set(siteContactLinkKey, contact.Link); err != nil {
			return model.SiteConfigResponse{}, err
		}
	}
	contact.QRCodeImageDataURL = buildSiteContactQRCodeDataURL(contact.Link)

	return model.SiteConfigResponse{
		SiteName:               siteName,
		TimeZone:               timeZone,
		AmpProxySettingsPolicy: ampProxySettingsPolicy,
		AmpSettingsPolicy:      ampSettingsPolicy,
		Contact:                contact,
		InviteEnabled:          NewInviteService().IsPublicEnabled(),
	}, nil
}

func (s *SystemConfigService) getSiteContactConfig() (model.SiteContactConfig, error) {
	enabledValue, err := s.repo.Get(siteContactEnabledKey)
	if err != nil {
		return model.SiteContactConfig{}, err
	}
	titleValue, err := s.repo.Get(siteContactTitleKey)
	if err != nil {
		return model.SiteContactConfig{}, err
	}
	descriptionValue, err := s.repo.Get(siteContactDescriptionKey)
	if err != nil {
		return model.SiteContactConfig{}, err
	}
	linkValue, err := s.repo.Get(siteContactLinkKey)
	if err != nil {
		return model.SiteContactConfig{}, err
	}

	link := strings.TrimSpace(linkValue)
	return model.SiteContactConfig{
		Enabled:            enabledValue == "true",
		Title:              strings.TrimSpace(titleValue),
		Description:        strings.TrimSpace(descriptionValue),
		Link:               link,
		QRCodeImageDataURL: buildSiteContactQRCodeDataURL(link),
	}, nil
}

func buildSiteContactQRCodeDataURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	data, err := qrcode.Encode(value, qrcode.Medium, 256)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

func (s *SystemConfigService) GetAmpProxySettingsPolicy() (string, error) {
	value, err := s.repo.Get(allowAmpProxySettingsKey)
	if err != nil {
		return defaultAmpProxySettingsPolicy, err
	}
	return normalizeAmpProxySettingsPolicy(value), nil
}

func (s *SystemConfigService) GetAmpSettingsPolicy() (string, error) {
	value, err := s.repo.Get(allowAmpSettingsKey)
	if err != nil {
		return defaultAmpSettingsPolicy, err
	}
	return normalizeAmpProxySettingsPolicy(value), nil
}

func (s *SystemConfigService) CanAccessAmpSettings(isAdmin bool) (bool, error) {
	policy, err := s.GetAmpProxySettingsPolicy()
	if err != nil {
		return false, err
	}
	return CanAccessAmpSettingsForPolicy(policy, isAdmin), nil
}

func (s *SystemConfigService) CanAccessAmpUpstreamSettings(isAdmin bool) (bool, error) {
	policy, err := s.GetAmpSettingsPolicy()
	if err != nil {
		return false, err
	}
	return CanAccessAmpSettingsForPolicy(policy, isAdmin), nil
}

func CanAccessAmpSettingsForPolicy(policy string, isAdmin bool) bool {
	switch normalizeAmpProxySettingsPolicy(policy) {
	case model.AmpProxySettingsPolicyDisabled:
		return false
	case model.AmpProxySettingsPolicyAdminOnly:
		return isAdmin
	default:
		return true
	}
}

func normalizeAmpProxySettingsPolicy(value string) string {
	switch strings.TrimSpace(value) {
	case "", "true", model.AmpProxySettingsPolicyAll:
		return model.AmpProxySettingsPolicyAll
	case "false", model.AmpProxySettingsPolicyDisabled:
		return model.AmpProxySettingsPolicyDisabled
	case model.AmpProxySettingsPolicyAdminOnly:
		return model.AmpProxySettingsPolicyAdminOnly
	default:
		return defaultAmpProxySettingsPolicy
	}
}

func (s *SystemConfigService) GetSiteLocation() (*time.Location, error) {
	cfg, err := s.GetSiteConfig()
	if err != nil {
		return nil, err
	}

	location, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		return time.LoadLocation(defaultSiteTimeZone)
	}

	return location, nil
}

func defaultBillingDailyResetConfig() model.BillingDailyResetConfigResponse {
	return model.BillingDailyResetConfigResponse{
		Enabled:               defaultBillingDailyResetEnabled,
		MinRemainingDays:      defaultBillingDailyResetMinDays,
		UsageThresholdPercent: defaultBillingDailyResetThresholdPct,
		DailyLimit:            defaultBillingDailyResetDailyLimit,
	}
}

func normalizeBillingDailyResetConfig(cfg model.BillingDailyResetConfigResponse) model.BillingDailyResetConfigResponse {
	defaults := defaultBillingDailyResetConfig()
	if cfg.MinRemainingDays < defaultBillingDailyResetMinDays {
		cfg.MinRemainingDays = defaults.MinRemainingDays
	}
	if cfg.UsageThresholdPercent < 0 {
		cfg.UsageThresholdPercent = 0
	}
	if cfg.UsageThresholdPercent > 100 {
		cfg.UsageThresholdPercent = 100
	}
	if cfg.DailyLimit < 0 {
		cfg.DailyLimit = 0
	}
	return cfg
}

func (s *SystemConfigService) GetBillingDailyResetConfig() (model.BillingDailyResetConfigResponse, error) {
	resp := defaultBillingDailyResetConfig()

	value, err := s.repo.Get(billingDailyResetEnabledKey)
	if err != nil {
		return resp, err
	}
	if value != "" {
		resp.Enabled = value != "false"
	}

	value, err = s.repo.Get(billingDailyResetMinDaysKey)
	if err != nil {
		return resp, err
	}
	if value != "" {
		if parsed, parseErr := parsePositiveInt(value); parseErr == nil {
			resp.MinRemainingDays = parsed
		}
	}

	value, err = s.repo.Get(billingDailyResetThresholdPctKey)
	if err != nil {
		return resp, err
	}
	if value != "" {
		if parsed, parseErr := parsePositiveInt(value); parseErr == nil {
			resp.UsageThresholdPercent = parsed
		}
	}

	value, err = s.repo.Get(billingDailyResetDailyLimitKey)
	if err != nil {
		return resp, err
	}
	if value != "" {
		if parsed, parseErr := strconv.Atoi(value); parseErr == nil {
			resp.DailyLimit = parsed
		}
	}

	return normalizeBillingDailyResetConfig(resp), nil
}

func (s *SystemConfigService) SetBillingDailyResetConfig(req model.BillingDailyResetConfigRequest) (model.BillingDailyResetConfigResponse, error) {
	resp := normalizeBillingDailyResetConfig(model.BillingDailyResetConfigResponse{
		Enabled:               req.Enabled,
		MinRemainingDays:      req.MinRemainingDays,
		UsageThresholdPercent: req.UsageThresholdPercent,
		DailyLimit:            req.DailyLimit,
	})
	defaults := defaultBillingDailyResetConfig()

	if resp.Enabled == defaults.Enabled {
		if err := s.repo.Delete(billingDailyResetEnabledKey); err != nil {
			return model.BillingDailyResetConfigResponse{}, err
		}
	} else if err := s.repo.Set(billingDailyResetEnabledKey, boolToConfigString(resp.Enabled)); err != nil {
		return model.BillingDailyResetConfigResponse{}, err
	}

	if resp.MinRemainingDays == defaults.MinRemainingDays {
		if err := s.repo.Delete(billingDailyResetMinDaysKey); err != nil {
			return model.BillingDailyResetConfigResponse{}, err
		}
	} else if err := s.repo.Set(billingDailyResetMinDaysKey, formatInt(resp.MinRemainingDays)); err != nil {
		return model.BillingDailyResetConfigResponse{}, err
	}

	if resp.UsageThresholdPercent == defaults.UsageThresholdPercent {
		if err := s.repo.Delete(billingDailyResetThresholdPctKey); err != nil {
			return model.BillingDailyResetConfigResponse{}, err
		}
	} else if err := s.repo.Set(billingDailyResetThresholdPctKey, formatInt(resp.UsageThresholdPercent)); err != nil {
		return model.BillingDailyResetConfigResponse{}, err
	}

	if resp.DailyLimit == defaults.DailyLimit {
		if err := s.repo.Delete(billingDailyResetDailyLimitKey); err != nil {
			return model.BillingDailyResetConfigResponse{}, err
		}
	} else if err := s.repo.Set(billingDailyResetDailyLimitKey, formatInt(resp.DailyLimit)); err != nil {
		return model.BillingDailyResetConfigResponse{}, err
	}

	return resp, nil
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
		RedisPrefix:           "ampmanager",
		ReservationTTLSec:     600,
		ReconcileIntervalSec:  60,
		StreamBatchSize:       100,
		ProjectorClaimIdleSec: 30,
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
		if cfg.BillingReconcileBatchSize > 0 {
			resp.ReconcileBatchSize = cfg.BillingReconcileBatchSize
		}
		if cfg.BillingExpiryBatchSize > 0 {
			resp.ExpiryBatchSize = cfg.BillingExpiryBatchSize
		}
		if cfg.BillingProjectorWorkers > 0 {
			resp.ProjectorWorkers = cfg.BillingProjectorWorkers
		}
		if cfg.BillingProjectorClaimIdleSec > 0 {
			resp.ProjectorClaimIdleSec = cfg.BillingProjectorClaimIdleSec
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
	if req.ReconcileBatchSize <= 0 {
		req.ReconcileBatchSize = req.StreamBatchSize
	}
	if req.ExpiryBatchSize <= 0 {
		req.ExpiryBatchSize = req.StreamBatchSize
	}
	if req.ProjectorWorkers <= 0 {
		req.ProjectorWorkers = 1
	}
	if req.ProjectorClaimIdleSec <= 0 {
		req.ProjectorClaimIdleSec = 30
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
	cfg := config.Get()
	if stored.ReconcileBatchSize > 0 {
		resp.ReconcileBatchSize = stored.ReconcileBatchSize
	} else if cfg == nil || !cfg.BillingEnvExplicit.ReconcileBatchSize {
		resp.ReconcileBatchSize = resp.StreamBatchSize
	}
	if stored.ExpiryBatchSize > 0 {
		resp.ExpiryBatchSize = stored.ExpiryBatchSize
	} else if cfg == nil || !cfg.BillingEnvExplicit.ExpiryBatchSize {
		resp.ExpiryBatchSize = resp.StreamBatchSize
	}
	if stored.ProjectorWorkers > 0 {
		resp.ProjectorWorkers = stored.ProjectorWorkers
	}
	if stored.ProjectorClaimIdleSec > 0 {
		resp.ProjectorClaimIdleSec = stored.ProjectorClaimIdleSec
	}

	return normalizeBillingRuntimeConfigRequest(resp), nil
}

func (s *SystemConfigService) GetBillingRuntimeConfig() (model.BillingRuntimeConfigResponse, error) {
	req, err := s.GetBillingRuntimeConfigRequest()
	if err != nil {
		return model.BillingRuntimeConfigResponse{}, err
	}

	resp := model.BillingRuntimeConfigResponse{
		RedisURL:              req.RedisURL,
		RedisURLMasked:        maskRedisURL(req.RedisURL),
		RedisPrefix:           req.RedisPrefix,
		ReservationTTLSec:     req.ReservationTTLSec,
		ReconcileIntervalSec:  req.ReconcileIntervalSec,
		StreamBatchSize:       req.StreamBatchSize,
		ReconcileBatchSize:    req.ReconcileBatchSize,
		ExpiryBatchSize:       req.ExpiryBatchSize,
		ProjectorWorkers:      req.ProjectorWorkers,
		ProjectorClaimIdleSec: req.ProjectorClaimIdleSec,
		RuntimeEnabled:        strings.TrimSpace(req.RedisURL) != "",
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
