package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWithLegacyRequestLogsWithoutSessionID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-request-logs.db")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}

	legacySchema := `
	CREATE TABLE request_logs (
		id TEXT PRIMARY KEY,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME,
		status TEXT NOT NULL DEFAULT 'success',
		user_id TEXT NOT NULL,
		api_key_id TEXT NOT NULL,
		original_model TEXT,
		mapped_model TEXT,
		provider TEXT,
		channel_id TEXT,
		endpoint TEXT,
		request_format TEXT,
		upstream_format TEXT,
		method TEXT NOT NULL,
		path TEXT NOT NULL,
		status_code INTEGER NOT NULL,
		latency_ms INTEGER NOT NULL,
		ttfb_ms INTEGER,
		is_streaming INTEGER NOT NULL DEFAULT 0,
		input_tokens INTEGER,
		output_tokens INTEGER,
		cache_read_input_tokens INTEGER,
		cache_creation_input_tokens INTEGER,
		error_type TEXT,
		request_id TEXT,
		cost_micros INTEGER,
		cost_usd TEXT,
		pricing_model TEXT,
		thinking_level TEXT,
		downstream_transport TEXT,
		upstream_transport TEXT,
		transport_fallback_reason TEXT,
		response_text TEXT,
		rate_multiplier REAL,
		rate_multiplier_ppm BIGINT,
		channel_rate_multiplier REAL,
		channel_rate_multiplier_ppm BIGINT,
		group_rate_multiplier REAL,
		group_rate_multiplier_ppm BIGINT,
		special_rate_multiplier REAL,
		special_rate_multiplier_ppm BIGINT,
		special_rate_reason TEXT,
		pricing_rule_name TEXT,
		charged_subscription_micros INTEGER NOT NULL DEFAULT 0,
		charged_balance_micros INTEGER NOT NULL DEFAULT 0,
		billing_status TEXT NOT NULL DEFAULT 'none'
	);
	CREATE INDEX idx_request_logs_user_time ON request_logs(user_id, created_at DESC);
	CREATE TABLE schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	INSERT INTO schema_migrations (name, applied_at) VALUES
		('add_request_logs_session_id', CURRENT_TIMESTAMP),
		('add_request_logs_session_time_index', CURRENT_TIMESTAMP);
	`
	if _, err := legacyDB.Exec(legacySchema); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("creating legacy schema returned error: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("closing legacy db returned error: %v", err)
	}

	if err := InitWithOptions(Options{
		Type:       DBTypeSQLite,
		SQLitePath: dbPath,
	}); err != nil {
		t.Fatalf("InitWithOptions returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = CloseAndRelease()
	})

	rows, err := GetDB().Query(`PRAGMA table_info(request_logs)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info returned error: %v", err)
	}
	defer rows.Close()

	hasSessionID := false
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scanning table_info row returned error: %v", err)
		}
		if name == "session_id" {
			hasSessionID = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating table_info rows returned error: %v", err)
	}
	if !hasSessionID {
		t.Fatal("expected request_logs.session_id to be added during init")
	}

	var indexCount int
	if err := GetDB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_request_logs_session_time'`).Scan(&indexCount); err != nil {
		t.Fatalf("querying sqlite_master returned error: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("expected idx_request_logs_session_time to exist, got count=%d", indexCount)
	}
}

func TestAdaptMigrationSQLPostgresRewritesDatetimeInRebuiltMigrations(t *testing.T) {
	previousDBType := dbType
	dbType = DBTypePostgres
	t.Cleanup(func() {
		dbType = previousDBType
	})

	adapted := adaptMigrationSQL("rebuild_purchase_orders_for_delivery_mode", "")
	if adapted == "" {
		t.Fatal("expected rebuilt migration SQL for Postgres")
	}
	if strings.Contains(strings.ToUpper(adapted), "DATETIME") {
		t.Fatalf("expected Postgres migration SQL to rewrite DATETIME, got: %s", adapted)
	}
	if !strings.Contains(strings.ToUpper(adapted), "TIMESTAMPTZ") {
		t.Fatalf("expected Postgres migration SQL to contain TIMESTAMPTZ, got: %s", adapted)
	}
}

func TestAdaptCriticalSchemaSQLPostgresRewritesDatetime(t *testing.T) {
	previousDBType := dbType
	dbType = DBTypePostgres
	t.Cleanup(func() {
		dbType = previousDBType
	})

	adapted := adaptCriticalSchemaSQL(`CREATE TABLE demo (created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME)`)
	if strings.Contains(strings.ToUpper(adapted), "DATETIME") {
		t.Fatalf("expected critical schema SQL to rewrite DATETIME, got: %s", adapted)
	}
	if !strings.Contains(strings.ToUpper(adapted), "TIMESTAMPTZ") {
		t.Fatalf("expected critical schema SQL to contain TIMESTAMPTZ, got: %s", adapted)
	}
}
