package service

import (
	"path/filepath"
	"testing"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

func TestSystemConfigServiceSiteConfigDefaultsAndPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "system-config-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})

	svc := NewSystemConfigService()

	defaultCfg, err := svc.GetSiteConfig()
	if err != nil {
		t.Fatalf("GetSiteConfig returned error: %v", err)
	}
	if defaultCfg.SiteName != defaultSiteName {
		t.Fatalf("default site name mismatch: got %q want %q", defaultCfg.SiteName, defaultSiteName)
	}
	if defaultCfg.TimeZone != defaultSiteTimeZone {
		t.Fatalf("default time zone mismatch: got %q want %q", defaultCfg.TimeZone, defaultSiteTimeZone)
	}
	if defaultCfg.AmpProxySettingsPolicy != defaultAmpProxySettingsPolicy {
		t.Fatalf("default amp proxy policy mismatch: got %q want %q", defaultCfg.AmpProxySettingsPolicy, defaultAmpProxySettingsPolicy)
	}
	if defaultCfg.AmpSettingsPolicy != defaultAmpSettingsPolicy {
		t.Fatalf("default amp settings policy mismatch: got %q want %q", defaultCfg.AmpSettingsPolicy, defaultAmpSettingsPolicy)
	}
	if defaultCfg.Contact.Enabled {
		t.Fatalf("default contact enabled mismatch: got true want false")
	}
	if defaultCfg.Contact.Title != "" || defaultCfg.Contact.Description != "" || defaultCfg.Contact.Link != "" || defaultCfg.Contact.QRCodeImageDataURL != "" {
		t.Fatalf("default contact mismatch: %+v", defaultCfg.Contact)
	}

	adminOnly := model.AmpProxySettingsPolicyAdminOnly
	disabled := model.AmpProxySettingsPolicyDisabled

	updatedCfg, err := svc.SetSiteConfig(model.SiteConfigRequest{
		SiteName:               "Ops Console",
		TimeZone:               "America/Los_Angeles",
		AmpProxySettingsPolicy: &adminOnly,
		AmpSettingsPolicy:      &disabled,
		Contact: &model.SiteContactConfigRequest{
			Enabled:     true,
			Title:       "Telegram",
			Description: "加入交流群",
			Link:        "https://example.com/contact",
		},
	})
	if err != nil {
		t.Fatalf("SetSiteConfig returned error: %v", err)
	}
	if updatedCfg.SiteName != "Ops Console" ||
		updatedCfg.TimeZone != "America/Los_Angeles" ||
		updatedCfg.AmpProxySettingsPolicy != model.AmpProxySettingsPolicyAdminOnly ||
		updatedCfg.AmpSettingsPolicy != model.AmpProxySettingsPolicyDisabled {
		t.Fatalf("unexpected updated config: %+v", updatedCfg)
	}
	if !updatedCfg.Contact.Enabled ||
		updatedCfg.Contact.Title != "Telegram" ||
		updatedCfg.Contact.Description != "加入交流群" ||
		updatedCfg.Contact.Link != "https://example.com/contact" ||
		updatedCfg.Contact.QRCodeImageDataURL == "" {
		t.Fatalf("unexpected updated contact config: %+v", updatedCfg.Contact)
	}

	reloadedCfg, err := svc.GetSiteConfig()
	if err != nil {
		t.Fatalf("GetSiteConfig after update returned error: %v", err)
	}
	if reloadedCfg.SiteName != updatedCfg.SiteName ||
		reloadedCfg.TimeZone != updatedCfg.TimeZone ||
		reloadedCfg.AmpProxySettingsPolicy != updatedCfg.AmpProxySettingsPolicy ||
		reloadedCfg.AmpSettingsPolicy != updatedCfg.AmpSettingsPolicy ||
		reloadedCfg.Contact.Enabled != updatedCfg.Contact.Enabled ||
		reloadedCfg.Contact.Title != updatedCfg.Contact.Title ||
		reloadedCfg.Contact.Description != updatedCfg.Contact.Description ||
		reloadedCfg.Contact.Link != updatedCfg.Contact.Link ||
		reloadedCfg.Contact.QRCodeImageDataURL == "" {
		t.Fatalf("reloaded config mismatch: got %+v want %+v", reloadedCfg, updatedCfg)
	}
}

func TestCanAccessAmpSettingsForPolicy(t *testing.T) {
	testCases := []struct {
		name    string
		policy  string
		isAdmin bool
		want    bool
	}{
		{name: "disabled admin", policy: model.AmpProxySettingsPolicyDisabled, isAdmin: true, want: false},
		{name: "disabled user", policy: model.AmpProxySettingsPolicyDisabled, isAdmin: false, want: false},
		{name: "admin only admin", policy: model.AmpProxySettingsPolicyAdminOnly, isAdmin: true, want: true},
		{name: "admin only user", policy: model.AmpProxySettingsPolicyAdminOnly, isAdmin: false, want: false},
		{name: "all admin", policy: model.AmpProxySettingsPolicyAll, isAdmin: true, want: true},
		{name: "all user", policy: model.AmpProxySettingsPolicyAll, isAdmin: false, want: true},
		{name: "legacy true", policy: "true", isAdmin: false, want: true},
		{name: "legacy false", policy: "false", isAdmin: true, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := CanAccessAmpSettingsForPolicy(tc.policy, tc.isAdmin)
			if got != tc.want {
				t.Fatalf("CanAccessAmpSettingsForPolicy(%q, %v) = %v, want %v", tc.policy, tc.isAdmin, got, tc.want)
			}
		})
	}
}

func TestSystemConfigServiceRequestPayloadLimitDefaultsAndPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "request-payload-limit-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})

	svc := NewSystemConfigService()

	defaultCfg, err := svc.GetRequestPayloadLimit()
	if err != nil {
		t.Fatalf("GetRequestPayloadLimit returned error: %v", err)
	}
	if defaultCfg.MaxBytes != defaultRequestPayloadMaxBytes {
		t.Fatalf("default request payload limit mismatch: got %d want %d", defaultCfg.MaxBytes, defaultRequestPayloadMaxBytes)
	}

	updatedCfg, err := svc.SetRequestPayloadLimit(model.RequestPayloadLimitRequest{MaxBytes: 64 * 1024 * 1024})
	if err != nil {
		t.Fatalf("SetRequestPayloadLimit returned error: %v", err)
	}
	if updatedCfg.MaxBytes != 64*1024*1024 {
		t.Fatalf("unexpected updated payload limit: %+v", updatedCfg)
	}

	reloadedCfg, err := svc.GetRequestPayloadLimit()
	if err != nil {
		t.Fatalf("GetRequestPayloadLimit after update returned error: %v", err)
	}
	if reloadedCfg.MaxBytes != updatedCfg.MaxBytes {
		t.Fatalf("reloaded payload limit mismatch: got %+v want %+v", reloadedCfg, updatedCfg)
	}
}

func TestSystemConfigServiceBillingDailyResetDefaultsAndPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "billing-daily-reset-config-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})

	svc := NewSystemConfigService()

	defaultCfg, err := svc.GetBillingDailyResetConfig()
	if err != nil {
		t.Fatalf("GetBillingDailyResetConfig returned error: %v", err)
	}
	if defaultCfg.Enabled != defaultBillingDailyResetEnabled {
		t.Fatalf("default enabled mismatch: got %v want %v", defaultCfg.Enabled, defaultBillingDailyResetEnabled)
	}
	if defaultCfg.MinRemainingDays != defaultBillingDailyResetMinDays {
		t.Fatalf("default minRemainingDays mismatch: got %d want %d", defaultCfg.MinRemainingDays, defaultBillingDailyResetMinDays)
	}
	if defaultCfg.UsageThresholdPercent != defaultBillingDailyResetThresholdPct {
		t.Fatalf("default usageThresholdPercent mismatch: got %d want %d", defaultCfg.UsageThresholdPercent, defaultBillingDailyResetThresholdPct)
	}
	if defaultCfg.DailyLimit != defaultBillingDailyResetDailyLimit {
		t.Fatalf("default dailyLimit mismatch: got %d want %d", defaultCfg.DailyLimit, defaultBillingDailyResetDailyLimit)
	}

	updatedCfg, err := svc.SetBillingDailyResetConfig(model.BillingDailyResetConfigRequest{
		Enabled:               false,
		MinRemainingDays:      5,
		UsageThresholdPercent: 95,
		DailyLimit:            1,
	})
	if err != nil {
		t.Fatalf("SetBillingDailyResetConfig returned error: %v", err)
	}
	if updatedCfg.Enabled || updatedCfg.MinRemainingDays != 5 || updatedCfg.UsageThresholdPercent != 95 || updatedCfg.DailyLimit != 1 {
		t.Fatalf("unexpected updated billing daily reset config: %+v", updatedCfg)
	}

	reloadedCfg, err := svc.GetBillingDailyResetConfig()
	if err != nil {
		t.Fatalf("GetBillingDailyResetConfig after update returned error: %v", err)
	}
	if reloadedCfg != updatedCfg {
		t.Fatalf("reloaded billing daily reset config mismatch: got %+v want %+v", reloadedCfg, updatedCfg)
	}
}

func TestSystemConfigServiceErrorRulesDefaultsAndMerge(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "error-rules-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})

	svc := NewSystemConfigService()

	defaultRules, err := svc.GetErrorRules()
	if err != nil {
		t.Fatalf("GetErrorRules returned error: %v", err)
	}
	if len(defaultRules) == 0 {
		t.Fatal("expected default error rules")
	}

	updatedRules, err := svc.SetErrorRules([]model.ErrorRule{
		{
			ID:              "builtin-openai-an-error-occurred-responses",
			Name:            "OpenAI 风格 An Error Occurred（Responses）",
			BuiltIn:         true,
			Enabled:         false,
			RequestType:     model.ErrorRuleRequestTypeResponses,
			UpstreamStatus:  "200",
			Pattern:         "An Error Occurred",
			MatchMode:       model.ErrorRuleMatchModeSubstring,
			OverrideStatus:  502,
			OverrideMessage: "已禁用测试",
		},
		{
			ID:              "custom-upstream-503",
			Name:            "自定义 503",
			BuiltIn:         false,
			Enabled:         true,
			RequestType:     model.ErrorRuleRequestTypeGemini,
			UpstreamStatus:  "500-599",
			Pattern:         "backend overloaded",
			MatchMode:       model.ErrorRuleMatchModeSubstring,
			OverrideStatus:  503,
			OverrideMessage: "Gemini 上游过载",
		},
	})
	if err != nil {
		t.Fatalf("SetErrorRules returned error: %v", err)
	}

	if len(updatedRules) != len(defaultRules)+1 {
		t.Fatalf("unexpected merged rules count: got %d want %d", len(updatedRules), len(defaultRules)+1)
	}

	foundDisabledBuiltIn := false
	foundCustom := false
	for _, rule := range updatedRules {
		if rule.ID == "builtin-openai-an-error-occurred-responses" {
			foundDisabledBuiltIn = !rule.Enabled && rule.OverrideMessage == "已禁用测试"
		}
		if rule.ID == "custom-upstream-503" {
			foundCustom = !rule.BuiltIn && rule.OverrideStatus == 503
		}
	}
	if !foundDisabledBuiltIn {
		t.Fatal("expected built-in rule override to persist")
	}
	if !foundCustom {
		t.Fatal("expected custom rule to be merged")
	}
}
