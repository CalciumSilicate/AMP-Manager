package service

import (
	"testing"

	"ampmanager/internal/model"
)

func TestUpdateSettingsPartialUpdatePreservesExistingFields(t *testing.T) {
	setupAmpServiceTestDB(t)

	user := createAmpServiceTestUser(t, "user-settings-partial")
	svc := NewAmpService()

	upstreamURL := "https://example.com"
	webSearchMode := model.WebSearchModeBuiltinFree
	enabled := true
	nativeMode := true
	routeMappingsEnabled := true
	showBalanceInAd := true
	socks5Proxy := "socks5://127.0.0.1:1080"

	created, err := svc.UpdateSettings(user.ID, &model.AmpSettingsRequest{
		UpstreamURL:          &upstreamURL,
		ModelMappings:        []model.ModelMapping{{From: "gpt-4.1", To: "gpt-4o"}},
		Enabled:              &enabled,
		WebSearchMode:        &webSearchMode,
		NativeMode:           &nativeMode,
		RouteMappingsEnabled: &routeMappingsEnabled,
		ShowBalanceInAd:      &showBalanceInAd,
		Socks5Proxy:          &socks5Proxy,
	})
	if err != nil {
		t.Fatalf("initial UpdateSettings returned error: %v", err)
	}
	if !created.RouteMappingsEnabled {
		t.Fatal("expected route mappings to be enabled after initial save")
	}

	routeMappingsEnabled = false
	updated, err := svc.UpdateSettings(user.ID, &model.AmpSettingsRequest{
		ModelMappings:        []model.ModelMapping{{From: "gpt-5", To: "gpt-5.4"}},
		RouteMappingsEnabled: &routeMappingsEnabled,
	})
	if err != nil {
		t.Fatalf("partial UpdateSettings returned error: %v", err)
	}

	if updated.UpstreamURL != upstreamURL {
		t.Fatalf("expected upstream URL %q, got %q", upstreamURL, updated.UpstreamURL)
	}
	if !updated.Enabled {
		t.Fatal("expected enabled flag to be preserved")
	}
	if !updated.NativeMode {
		t.Fatal("expected native mode to be preserved")
	}
	if updated.WebSearchMode != webSearchMode {
		t.Fatalf("expected web search mode %q, got %q", webSearchMode, updated.WebSearchMode)
	}
	if !updated.ShowBalanceInAd {
		t.Fatal("expected showBalanceInAd to be preserved")
	}
	if updated.RouteMappingsEnabled {
		t.Fatal("expected route mappings to be disabled after partial update")
	}
	if len(updated.ModelMappings) != 1 || updated.ModelMappings[0].From != "gpt-5" {
		t.Fatalf("expected updated model mappings, got %#v", updated.ModelMappings)
	}

	stored, err := svc.settingsRepo.GetByUserID(user.ID)
	if err != nil {
		t.Fatalf("GetByUserID returned error: %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored settings to exist")
	}
	if stored.UpstreamURL != upstreamURL {
		t.Fatalf("expected stored upstream URL %q, got %q", upstreamURL, stored.UpstreamURL)
	}
	if !stored.Enabled || !stored.NativeMode {
		t.Fatalf("expected stored enabled/native mode to remain true, got enabled=%v native=%v", stored.Enabled, stored.NativeMode)
	}
	if stored.WebSearchMode != webSearchMode {
		t.Fatalf("expected stored web search mode %q, got %q", webSearchMode, stored.WebSearchMode)
	}
	if !stored.ShowBalanceInAd {
		t.Fatal("expected stored showBalanceInAd to remain true")
	}
	if stored.Socks5Proxy != socks5Proxy {
		t.Fatalf("expected stored socks5 proxy %q, got %q", socks5Proxy, stored.Socks5Proxy)
	}
	if stored.RouteMappingsEnabled {
		t.Fatal("expected stored routeMappingsEnabled to be false")
	}
}
