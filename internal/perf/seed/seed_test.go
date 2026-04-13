package seed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ampmanager/internal/database"

	"github.com/google/uuid"
)

func TestSeederRunCreatesUsersChannelsAndManifest(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "perf-seed.db")

	if err := database.InitWithOptions(database.Options{
		Type:       database.DBTypeSQLite,
		SQLitePath: dbPath,
	}); err != nil {
		t.Fatalf("init database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	manifestPath := filepath.Join(tempDir, "manifest.json")
	seeder := New(Options{
		UserCount:              3,
		UserPrefix:             "test-user-" + uuid.NewString(),
		UserPassword:           "user-pass",
		AdminUsername:          "test-admin-" + uuid.NewString(),
		AdminPassword:          "admin-pass",
		BalanceMicros:          5_000_000,
		UpstreamURL:            "http://mock-upstream:18080",
		UpstreamAPIKey:         "perf-upstream-key",
		RequestDetailEnabled:   false,
		PublicChatModel:        "bench-chat",
		PublicResponsesModel:   "bench-responses",
		UpstreamChatModel:      "bench-chat-upstream",
		UpstreamResponsesModel: "bench-responses-upstream",
		OutputPath:             manifestPath,
	})

	result, err := seeder.Run()
	if err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if result.UsersCreated != 3 {
		t.Fatalf("unexpected users created: %d", result.UsersCreated)
	}
	if result.ChannelsCreated != 2 {
		t.Fatalf("unexpected channels created: %d", result.ChannelsCreated)
	}

	db := database.GetDB()

	var userCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 4 {
		t.Fatalf("unexpected total users: %d", userCount)
	}

	var apiKeyCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_api_keys`).Scan(&apiKeyCount); err != nil {
		t.Fatalf("count api keys: %v", err)
	}
	if apiKeyCount != 3 {
		t.Fatalf("unexpected api key count: %d", apiKeyCount)
	}

	var channelCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM channels`).Scan(&channelCount); err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if channelCount != 2 {
		t.Fatalf("unexpected channel count: %d", channelCount)
	}

	var settingsCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_amp_settings`).Scan(&settingsCount); err != nil {
		t.Fatalf("count amp settings: %v", err)
	}
	if settingsCount != 3 {
		t.Fatalf("unexpected settings count: %d", settingsCount)
	}

	var requestDetailEnabled string
	if err := db.QueryRow(`SELECT value FROM system_config WHERE key = ?`, requestDetailEnabledKey).Scan(&requestDetailEnabled); err != nil {
		t.Fatalf("read request detail config: %v", err)
	}
	if requestDetailEnabled != "false" {
		t.Fatalf("unexpected request detail setting: %s", requestDetailEnabled)
	}

	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(manifest.APIKeys) != 3 {
		t.Fatalf("unexpected manifest key count: %d", len(manifest.APIKeys))
	}
}
