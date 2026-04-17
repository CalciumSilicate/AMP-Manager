package service

import (
	"encoding/json"
	"strings"

	"ampmanager/internal/model"
)

const userPanelRateLimitConfigKey = "user_panel_rate_limit_config"

func defaultUserPanelRateLimitConfig() model.UserPanelRateLimitConfig {
	return model.UserPanelRateLimitConfig{
		Enabled:  true,
		Backend:  "redis",
		FailOpen: true,
		Sections: model.UserPanelRateLimitSections{
			OverviewStatus: model.UserPanelRateLimitSectionConfig{
				RPS:       1,
				Burst:     3,
				MaxWaitMs: 4500,
			},
			AmpSettings: model.UserPanelRateLimitSectionConfig{
				RPS:       0.5,
				Burst:     2,
				MaxWaitMs: 9000,
			},
			APIKeys: model.UserPanelRateLimitSectionConfig{
				RPS:       0.5,
				Burst:     2,
				MaxWaitMs: 9000,
			},
			RequestLogsUsage: model.UserPanelRateLimitSectionConfig{
				RPS:       0.2,
				Burst:     1,
				MaxWaitMs: 15000,
			},
			Models: model.UserPanelRateLimitSectionConfig{
				RPS:       1,
				Burst:     2,
				MaxWaitMs: 3000,
			},
			AccountPurchase: model.UserPanelRateLimitSectionConfig{
				RPS:       0.5,
				Burst:     2,
				MaxWaitMs: 12000,
			},
		},
	}
}

func normalizeUserPanelRateLimitSectionConfig(cfg model.UserPanelRateLimitSectionConfig, fallback model.UserPanelRateLimitSectionConfig) model.UserPanelRateLimitSectionConfig {
	if cfg.RPS <= 0 {
		cfg.RPS = fallback.RPS
	}
	if cfg.Burst < 1 {
		cfg.Burst = fallback.Burst
	}
	if cfg.MaxWaitMs < 0 {
		cfg.MaxWaitMs = fallback.MaxWaitMs
	}
	return cfg
}

func normalizeUserPanelRateLimitConfig(cfg model.UserPanelRateLimitConfig) model.UserPanelRateLimitConfig {
	defaults := defaultUserPanelRateLimitConfig()
	cfg.Backend = defaults.Backend
	cfg.FailOpen = true
	cfg.Sections.OverviewStatus = normalizeUserPanelRateLimitSectionConfig(cfg.Sections.OverviewStatus, defaults.Sections.OverviewStatus)
	cfg.Sections.AmpSettings = normalizeUserPanelRateLimitSectionConfig(cfg.Sections.AmpSettings, defaults.Sections.AmpSettings)
	cfg.Sections.APIKeys = normalizeUserPanelRateLimitSectionConfig(cfg.Sections.APIKeys, defaults.Sections.APIKeys)
	cfg.Sections.RequestLogsUsage = normalizeUserPanelRateLimitSectionConfig(cfg.Sections.RequestLogsUsage, defaults.Sections.RequestLogsUsage)
	cfg.Sections.Models = normalizeUserPanelRateLimitSectionConfig(cfg.Sections.Models, defaults.Sections.Models)
	cfg.Sections.AccountPurchase = normalizeUserPanelRateLimitSectionConfig(cfg.Sections.AccountPurchase, defaults.Sections.AccountPurchase)
	return cfg
}

func (s *SystemConfigService) GetUserPanelRateLimitConfig() (model.UserPanelRateLimitConfig, error) {
	cfg := defaultUserPanelRateLimitConfig()
	value, err := s.repo.Get(userPanelRateLimitConfigKey)
	if err != nil {
		return cfg, err
	}
	if strings.TrimSpace(value) == "" {
		return cfg, nil
	}

	var stored model.UserPanelRateLimitConfigRequest
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return cfg, nil
	}

	cfg.Enabled = stored.Enabled
	cfg.Sections = stored.Sections
	return normalizeUserPanelRateLimitConfig(cfg), nil
}

func (s *SystemConfigService) SetUserPanelRateLimitConfig(req model.UserPanelRateLimitConfigRequest) (model.UserPanelRateLimitConfig, error) {
	cfg := normalizeUserPanelRateLimitConfig(model.UserPanelRateLimitConfig{
		Enabled:  req.Enabled,
		Sections: req.Sections,
	})
	payload, err := json.Marshal(model.UserPanelRateLimitConfigRequest{
		Enabled:  cfg.Enabled,
		Sections: cfg.Sections,
	})
	if err != nil {
		return model.UserPanelRateLimitConfig{}, err
	}
	if err := s.repo.Set(userPanelRateLimitConfigKey, string(payload)); err != nil {
		return model.UserPanelRateLimitConfig{}, err
	}
	return cfg, nil
}
