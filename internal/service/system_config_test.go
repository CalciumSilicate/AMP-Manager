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
	if defaultCfg.AllowAmpProxySettings != defaultAllowAmpProxySettings {
		t.Fatalf("default amp proxy setting mismatch: got %v want %v", defaultCfg.AllowAmpProxySettings, defaultAllowAmpProxySettings)
	}

	disabled := false

	updatedCfg, err := svc.SetSiteConfig(model.SiteConfigRequest{
		SiteName:              "Ops Console",
		TimeZone:              "America/Los_Angeles",
		AllowAmpProxySettings: &disabled,
	})
	if err != nil {
		t.Fatalf("SetSiteConfig returned error: %v", err)
	}
	if updatedCfg.SiteName != "Ops Console" || updatedCfg.TimeZone != "America/Los_Angeles" || updatedCfg.AllowAmpProxySettings {
		t.Fatalf("unexpected updated config: %+v", updatedCfg)
	}

	reloadedCfg, err := svc.GetSiteConfig()
	if err != nil {
		t.Fatalf("GetSiteConfig after update returned error: %v", err)
	}
	if reloadedCfg != updatedCfg {
		t.Fatalf("reloaded config mismatch: got %+v want %+v", reloadedCfg, updatedCfg)
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
