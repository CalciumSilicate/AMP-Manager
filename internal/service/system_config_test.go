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

	updatedCfg, err := svc.SetSiteConfig(model.SiteConfigRequest{
		SiteName: "Ops Console",
		TimeZone: "America/Los_Angeles",
	})
	if err != nil {
		t.Fatalf("SetSiteConfig returned error: %v", err)
	}
	if updatedCfg.SiteName != "Ops Console" || updatedCfg.TimeZone != "America/Los_Angeles" {
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
