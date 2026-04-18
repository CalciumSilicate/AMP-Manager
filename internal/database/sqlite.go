package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	db        *sql.DB
	dbPath    string
	dbType    DBType
	dbOptions Options
	mu        sync.RWMutex
	inited    bool
)

func Init(path string) error {
	return InitWithOptions(Options{Type: DBTypeSQLite, SQLitePath: path})
}

func OpenWithOptions(options Options) (*sql.DB, error) {
	normalized, err := options.Normalize()
	if err != nil {
		return nil, err
	}

	databaseHandle, _, err := openDB(normalized)
	if err != nil {
		return nil, err
	}
	return databaseHandle, nil
}

func InitWithOptions(options Options) error {
	normalized, err := options.Normalize()
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	return initDB(normalized)
}

func initDB(options Options) error {
	newDB, resolvedPath, err := openDB(options)
	if err != nil {
		return err
	}

	if existing := db; existing != nil {
		_ = existing.Close()
	}

	db = newDB
	dbPath = resolvedPath
	dbType = options.Type
	dbOptions = options
	inited = true

	if err := createTables(); err != nil {
		return err
	}
	if err := runMigrations(); err != nil {
		return err
	}
	return ensureCriticalSchema()
}

func openDB(options Options) (*sql.DB, string, error) {
	switch options.Type {
	case DBTypeSQLite:
		return openSQLiteDB(options.SQLitePath)
	case DBTypePostgres:
		return openPostgresDB(options.DatabaseURL)
	default:
		return nil, "", fmt.Errorf("unsupported DB_TYPE: %s", options.Type)
	}
}

func openSQLiteDB(path string) (*sql.DB, string, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, "", err
		}
	}

	dsn := path + "?_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	newDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, "", err
	}
	if err = newDB.Ping(); err != nil {
		newDB.Close()
		return nil, "", err
	}

	newDB.SetMaxOpenConns(getEnvInt("DB_MAX_OPEN_CONNS", 32))
	newDB.SetMaxIdleConns(getEnvInt("DB_MAX_IDLE_CONNS", 16))
	newDB.SetConnMaxLifetime(time.Hour)

	return newDB, path, nil
}

func openPostgresDB(dsn string) (*sql.DB, string, error) {
	connector, err := openPostgresConnector(ensurePostgresTimezone(dsn))
	if err != nil {
		return nil, "", err
	}

	newDB := sql.OpenDB(connector)
	if err := newDB.Ping(); err != nil {
		newDB.Close()
		return nil, "", err
	}

	newDB.SetMaxOpenConns(getEnvInt("DB_MAX_OPEN_CONNS", 64))
	newDB.SetMaxIdleConns(getEnvInt("DB_MAX_IDLE_CONNS", 16))
	newDB.SetConnMaxLifetime(time.Hour)

	return newDB, "", nil
}

func ensurePostgresTimezone(dsn string) string {
	if strings.Contains(strings.ToLower(dsn), "timezone=") {
		return dsn
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + "timezone=UTC"
}

func GetType() DBType {
	mu.RLock()
	defer mu.RUnlock()
	return dbType
}

func GetOptions() Options {
	mu.RLock()
	defer mu.RUnlock()
	return dbOptions
}

func IsSQLite() bool {
	mu.RLock()
	defer mu.RUnlock()
	return dbType == DBTypeSQLite
}

func IsPostgres() bool {
	mu.RLock()
	defer mu.RUnlock()
	return dbType == DBTypePostgres
}

func SupportsFileBackups() bool {
	return IsSQLite()
}

func DayBucketExpr(column string) string {
	return DayBucketExprInLocation(column, time.UTC)
}

func DayBucketExprInLocation(column string, location *time.Location) string {
	if location == nil {
		location = time.UTC
	}

	if IsPostgres() {
		return fmt.Sprintf("TO_CHAR(%s AT TIME ZONE '%s', 'YYYY-MM-DD')", column, escapeSQLStringLiteral(location.String()))
	}

	return fmt.Sprintf("strftime('%%Y-%%m-%%d', substr(%s, 1, 19), '%s')", column, sqliteTimezoneModifier(location))
}

func escapeSQLStringLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func sqliteTimezoneModifier(location *time.Location) string {
	_, offsetSec := time.Now().In(location).Zone()
	return fmt.Sprintf("%+d minutes", offsetSec/60)
}

func Rebind(query string) string {
	if IsPostgres() {
		return rewritePlaceholders(query)
	}
	return query
}

func PlaceholderList(count int) string {
	placeholders := make([]string, count)
	for index := 0; index < count; index++ {
		if IsPostgres() {
			placeholders[index] = fmt.Sprintf("$%d", index+1)
		} else {
			placeholders[index] = "?"
		}
	}
	return strings.Join(placeholders, ",")
}

func getEnvInt(key string, defaultValue int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultValue
}

// CloseAndRelease 关闭数据库连接并释放所有文件句柄，以便替换数据库文件
func CloseAndRelease() error {
	mu.Lock()
	defer mu.Unlock()
	if db != nil {
		if dbType == DBTypeSQLite {
			_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		}
		err := db.Close()
		db = nil
		return err
	}
	return nil
}

// Reopen 重新打开数据库（文件替换后调用）
func Reopen() error {
	mu.Lock()
	defer mu.Unlock()
	if !inited {
		return fmt.Errorf("database path not set, call Init first")
	}
	return initDB(dbOptions)
}

func GetDB() *sql.DB {
	mu.RLock()
	defer mu.RUnlock()
	return db
}

// GetPath returns the database file path.
func GetPath() string {
	mu.RLock()
	defer mu.RUnlock()
	return dbPath
}

func createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS groups (
		id TEXT PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		rate_multiplier REAL NOT NULL DEFAULT 1.0,
		rate_multiplier_ppm BIGINT NOT NULL DEFAULT 1000000,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		invite_code TEXT NOT NULL DEFAULT '',
		is_admin INTEGER DEFAULT 0,
		balance_micros BIGINT NOT NULL DEFAULT 0,
		concurrency_limit INTEGER NOT NULL DEFAULT 0,
		must_change_password INTEGER NOT NULL DEFAULT 0,
		must_change_username INTEGER NOT NULL DEFAULT 0,
		legacy_source TEXT NOT NULL DEFAULT '',
		legacy_ref_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS admin_management_keys (
		user_id TEXT PRIMARY KEY,
		key_hash TEXT NOT NULL DEFAULT '',
		key_ciphertext TEXT NOT NULL DEFAULT '',
		key_prefix TEXT NOT NULL DEFAULT '',
		previous_key_hash TEXT NOT NULL DEFAULT '',
		previous_key_ciphertext TEXT NOT NULL DEFAULT '',
		previous_key_prefix TEXT NOT NULL DEFAULT '',
		previous_valid_until DATETIME,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		rotated_at DATETIME,
		last_used_at DATETIME,
		last_auth_method TEXT NOT NULL DEFAULT '',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_admin_management_keys_hash ON admin_management_keys(key_hash);
	CREATE INDEX IF NOT EXISTS idx_admin_management_keys_previous_hash ON admin_management_keys(previous_key_hash);

	CREATE TABLE IF NOT EXISTS user_amp_settings (
		id TEXT PRIMARY KEY,
		user_id TEXT UNIQUE NOT NULL,
		upstream_url TEXT NOT NULL DEFAULT '',
		upstream_api_key TEXT NOT NULL DEFAULT '',
		model_mappings_json TEXT NOT NULL DEFAULT '[]',
		force_model_mappings INTEGER DEFAULT 0,
		enabled INTEGER DEFAULT 1,
			route_mappings_enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS user_api_keys (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		key_hash TEXT UNIQUE NOT NULL,
		api_key TEXT NOT NULL DEFAULT '',
		prefix TEXT NOT NULL,
		last_used_at DATETIME,
		expires_at DATETIME,
		revoked_at DATETIME,
		circuit_breaker_threshold INTEGER NOT NULL DEFAULT 50,
		circuit_breaker_open_minutes INTEGER NOT NULL DEFAULT 10,
		circuit_breaker_half_open_minutes INTEGER NOT NULL DEFAULT 2,
		circuit_breaker_state TEXT NOT NULL DEFAULT 'closed',
		circuit_breaker_consecutive_errors INTEGER NOT NULL DEFAULT 0,
		circuit_breaker_opened_at DATETIME,
		circuit_breaker_half_open_started_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_api_keys_user_created ON user_api_keys(user_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_api_keys_user_active ON user_api_keys(user_id, revoked_at);

	CREATE TABLE IF NOT EXISTS channels (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		endpoint TEXT NOT NULL DEFAULT 'chat_completions',
		name TEXT NOT NULL,
		base_url TEXT NOT NULL,
		api_key TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		split_groups_by_source INTEGER NOT NULL DEFAULT 0,
		circuit_breaker_threshold INTEGER NOT NULL DEFAULT 50,
		circuit_breaker_open_minutes INTEGER NOT NULL DEFAULT 10,
		circuit_breaker_half_open_minutes INTEGER NOT NULL DEFAULT 2,
		circuit_breaker_state TEXT NOT NULL DEFAULT 'closed',
		circuit_breaker_consecutive_errors INTEGER NOT NULL DEFAULT 0,
		circuit_breaker_opened_at DATETIME,
		circuit_breaker_half_open_started_at DATETIME,
		weight INTEGER NOT NULL DEFAULT 1,
		priority INTEGER NOT NULL DEFAULT 100,
		rate_multiplier REAL NOT NULL DEFAULT 1.0,
		rate_multiplier_ppm BIGINT NOT NULL DEFAULT 1000000,
		codex_websocket_enabled INTEGER NOT NULL DEFAULT 0,
		models_json TEXT NOT NULL DEFAULT '[]',
		headers_json TEXT NOT NULL DEFAULT '{}',
		translator_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_channels_type_enabled ON channels(type, enabled);
	CREATE INDEX IF NOT EXISTS idx_channels_enabled_priority ON channels(enabled, priority ASC, weight DESC);
	CREATE INDEX IF NOT EXISTS idx_channels_priority_created ON channels(priority ASC, created_at DESC);

	CREATE TABLE IF NOT EXISTS user_groups (
		user_id TEXT NOT NULL,
		group_id TEXT NOT NULL,
		PRIMARY KEY (user_id, group_id),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_user_groups_group ON user_groups(group_id);

	CREATE TABLE IF NOT EXISTS channel_groups (
		channel_id TEXT NOT NULL,
		group_id TEXT NOT NULL,
		PRIMARY KEY (channel_id, group_id),
		FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
		FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_channel_groups_group ON channel_groups(group_id);

	CREATE TABLE IF NOT EXISTS channel_subscription_groups (
		channel_id TEXT NOT NULL,
		group_id TEXT NOT NULL,
		PRIMARY KEY (channel_id, group_id),
		FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
		FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_channel_subscription_groups_group ON channel_subscription_groups(group_id);

	CREATE TABLE IF NOT EXISTS channel_usage_groups (
		channel_id TEXT NOT NULL,
		group_id TEXT NOT NULL,
		PRIMARY KEY (channel_id, group_id),
		FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
		FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_channel_usage_groups_group ON channel_usage_groups(group_id);

	CREATE TABLE IF NOT EXISTS channel_models (
		id TEXT PRIMARY KEY,
		channel_id TEXT NOT NULL,
		model_id TEXT NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_models_unique ON channel_models(channel_id, model_id);

	CREATE TABLE IF NOT EXISTS model_metadata (
		id TEXT PRIMARY KEY,
		model_pattern TEXT UNIQUE NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		context_length INTEGER NOT NULL DEFAULT 200000,
		max_completion_tokens INTEGER NOT NULL DEFAULT 32000,
		provider TEXT NOT NULL DEFAULT '',
		is_builtin INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_model_metadata_provider_pattern ON model_metadata(provider, model_pattern);

	CREATE TABLE IF NOT EXISTS request_logs (
		id TEXT PRIMARY KEY,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME,
		status TEXT NOT NULL DEFAULT 'success',
		user_id TEXT NOT NULL,
		api_key_id TEXT NOT NULL,
		original_model TEXT,
		mapped_model TEXT,
		session_id TEXT,
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
	CREATE INDEX IF NOT EXISTS idx_request_logs_user_time ON request_logs(user_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_request_logs_apikey_time ON request_logs(api_key_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_request_logs_model_time ON request_logs(mapped_model, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_request_logs_original_model_time ON request_logs(original_model, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_request_logs_session_time ON request_logs(session_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_request_logs_time ON request_logs(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_request_logs_status_code_time ON request_logs(status_code, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_request_logs_streaming_time ON request_logs(is_streaming, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_request_logs_effective_model ON request_logs(COALESCE(mapped_model, original_model));

		CREATE TABLE IF NOT EXISTS global_request_metric_projections (
			request_id TEXT PRIMARY KEY,
			minute_bucket DATETIME NOT NULL,
			request_count BIGINT NOT NULL DEFAULT 0,
			input_tokens BIGINT NOT NULL DEFAULT 0,
			output_tokens BIGINT NOT NULL DEFAULT 0,
			total_tokens BIGINT NOT NULL DEFAULT 0,
			latency_ms BIGINT NOT NULL DEFAULT 0,
			ttfb_ms BIGINT NOT NULL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	CREATE INDEX IF NOT EXISTS idx_global_request_metric_projections_minute ON global_request_metric_projections(minute_bucket DESC);

	CREATE TABLE IF NOT EXISTS global_request_minute_metrics (
		minute_bucket DATETIME PRIMARY KEY,
		request_count_sum BIGINT NOT NULL DEFAULT 0,
		input_tokens_sum BIGINT NOT NULL DEFAULT 0,
		output_tokens_sum BIGINT NOT NULL DEFAULT 0,
		total_tokens_sum BIGINT NOT NULL DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_global_request_minute_metrics_minute ON global_request_minute_metrics(minute_bucket DESC);

	CREATE TABLE IF NOT EXISTS model_prices (
		id TEXT PRIMARY KEY,
		model TEXT UNIQUE NOT NULL,
		provider TEXT,
		price_data TEXT NOT NULL DEFAULT '{}',
		source TEXT NOT NULL DEFAULT 'manual',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_model_prices_provider ON model_prices(provider);

	CREATE TABLE IF NOT EXISTS model_price_context_rules (
		id TEXT PRIMARY KEY,
		model TEXT NOT NULL,
		rule_name TEXT NOT NULL,
		min_tokens BIGINT NOT NULL DEFAULT 0,
		max_tokens BIGINT,
		input_micros_per_million BIGINT NOT NULL DEFAULT 0,
		output_micros_per_million BIGINT NOT NULL DEFAULT 0,
		cache_read_micros_per_million BIGINT NOT NULL DEFAULT 0,
		cache_creation_micros_per_million BIGINT NOT NULL DEFAULT 0,
		input_cost_per_token REAL NOT NULL DEFAULT 0,
		output_cost_per_token REAL NOT NULL DEFAULT 0,
		cache_read_input_per_token REAL NOT NULL DEFAULT 0,
		cache_creation_per_token REAL NOT NULL DEFAULT 0,
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_model_price_context_rules_model ON model_price_context_rules(model, sort_order ASC, created_at ASC);

	CREATE TABLE IF NOT EXISTS system_config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS invite_relationships (
		id TEXT PRIMARY KEY,
		inviter_user_id TEXT NOT NULL,
		inviter_code TEXT NOT NULL,
		invitee_user_id TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'rewarded')),
		first_paid_order_id TEXT NOT NULL DEFAULT '',
		first_paid_order_no TEXT NOT NULL DEFAULT '',
		first_paid_order_kind TEXT NOT NULL DEFAULT '',
		first_paid_amount_cny_cent BIGINT NOT NULL DEFAULT 0,
		first_paid_at DATETIME,
		rewarded_at DATETIME,
		last_reversed_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (inviter_user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (invitee_user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_users_invite_code_unique ON users(invite_code) WHERE invite_code <> '';
	CREATE INDEX IF NOT EXISTS idx_invite_relationships_inviter_status ON invite_relationships(inviter_user_id, status, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_invite_relationships_invitee ON invite_relationships(invitee_user_id);
	CREATE INDEX IF NOT EXISTS idx_invite_relationships_first_paid_order_no ON invite_relationships(first_paid_order_no);

	CREATE TABLE IF NOT EXISTS invite_reward_events (
		id TEXT PRIMARY KEY,
		relation_id TEXT NOT NULL,
		beneficiary_user_id TEXT NOT NULL,
		beneficiary_role TEXT NOT NULL CHECK (beneficiary_role IN ('inviter', 'invitee')),
		order_id TEXT NOT NULL DEFAULT '',
		order_no TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL CHECK (status IN ('granted', 'reversed')),
		amount_micros BIGINT NOT NULL DEFAULT 0,
		order_paid_amount_cny_cent BIGINT NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (relation_id) REFERENCES invite_relationships(id) ON DELETE CASCADE,
		FOREIGN KEY (beneficiary_user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_invite_reward_events_unique ON invite_reward_events(relation_id, beneficiary_role, status, order_no);
	CREATE INDEX IF NOT EXISTS idx_invite_reward_events_user_created ON invite_reward_events(beneficiary_user_id, created_at DESC);

	CREATE TABLE IF NOT EXISTS status_monitors (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		group_name TEXT NOT NULL DEFAULT '',
		target_type TEXT NOT NULL CHECK (target_type IN ('service_proxy', 'channel_direct', 'custom_http')),
		enabled INTEGER NOT NULL DEFAULT 1,
		sort_order INTEGER NOT NULL DEFAULT 0,
		timeout_ms BIGINT NOT NULL DEFAULT 0,
		degraded_threshold_ms BIGINT NOT NULL DEFAULT 10000,
		request_format TEXT NOT NULL DEFAULT 'responses',
		model TEXT NOT NULL DEFAULT '',
		channel_id TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '',
		method TEXT NOT NULL DEFAULT '',
		headers_json TEXT NOT NULL DEFAULT '',
		body_template TEXT NOT NULL DEFAULT '',
		expected_status_codes_json TEXT NOT NULL DEFAULT '[200]',
		expected_substring TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_status_monitors_enabled_sort ON status_monitors(enabled, sort_order ASC, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_status_monitors_target_enabled ON status_monitors(target_type, enabled);

	CREATE TABLE IF NOT EXISTS status_monitor_results (
		id TEXT PRIMARY KEY,
		monitor_id TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('operational', 'degraded', 'error', 'failed', 'unknown')),
		latency_ms BIGINT NOT NULL DEFAULT 0,
		ttfb_ms BIGINT NOT NULL DEFAULT 0,
		http_status_code INTEGER NOT NULL DEFAULT 0,
		message TEXT NOT NULL DEFAULT '',
		endpoint_label TEXT NOT NULL DEFAULT '',
		checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (monitor_id) REFERENCES status_monitors(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_status_monitor_results_monitor_checked ON status_monitor_results(monitor_id, checked_at DESC);
	CREATE INDEX IF NOT EXISTS idx_status_monitor_results_checked ON status_monitor_results(checked_at DESC);

		CREATE TABLE IF NOT EXISTS request_log_details (
			request_id TEXT PRIMARY KEY,
			request_headers TEXT,
		request_body TEXT,
		translated_request_body TEXT,
		translated_request_headers TEXT,
		response_headers TEXT,
		response_body TEXT,
		translated_response_body TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_request_log_details_created ON request_log_details(created_at DESC);

		CREATE TABLE IF NOT EXISTS error_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			request_type TEXT NOT NULL CHECK (request_type IN ('responses', 'chat_completions', 'gemini', 'messages')),
			upstream_status TEXT NOT NULL DEFAULT '*',
			pattern TEXT NOT NULL,
			match_type TEXT NOT NULL CHECK (match_type IN ('contains', 'exact', 'regex')),
			category TEXT NOT NULL DEFAULT '',
			priority INTEGER NOT NULL DEFAULT 0,
			is_enabled INTEGER NOT NULL DEFAULT 1,
			is_default INTEGER NOT NULL DEFAULT 0,
			override_status_code INTEGER,
			override_message TEXT NOT NULL DEFAULT '',
			override_response TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_error_rules_request_pattern ON error_rules(request_type, pattern, is_default);
	CREATE INDEX IF NOT EXISTS idx_error_rules_enabled_priority ON error_rules(is_enabled, priority ASC, id ASC);
	CREATE INDEX IF NOT EXISTS idx_error_rules_default_pattern ON error_rules(is_default, pattern);

		CREATE TABLE IF NOT EXISTS request_filters (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			scope TEXT NOT NULL CHECK (scope IN ('header', 'body')),
			action TEXT NOT NULL CHECK (action IN ('remove', 'set', 'json_path', 'text_replace')),
			match_type TEXT,
			target TEXT NOT NULL DEFAULT '',
			replacement TEXT NOT NULL DEFAULT '',
			priority INTEGER NOT NULL DEFAULT 0,
			is_enabled INTEGER NOT NULL DEFAULT 1,
			binding_type TEXT NOT NULL DEFAULT 'global' CHECK (binding_type IN ('global', 'channels', 'groups')),
			channel_ids_json TEXT NOT NULL DEFAULT '[]',
			group_ids_json TEXT NOT NULL DEFAULT '[]',
			rule_mode TEXT NOT NULL DEFAULT 'simple' CHECK (rule_mode IN ('simple', 'advanced')),
			execution_phase TEXT NOT NULL DEFAULT 'guard' CHECK (execution_phase IN ('guard', 'final')),
			operations_json TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_request_filters_enabled_priority ON request_filters(is_enabled, priority ASC, id ASC);
		CREATE INDEX IF NOT EXISTS idx_request_filters_binding_phase ON request_filters(binding_type, execution_phase, priority ASC);

		CREATE TABLE IF NOT EXISTS announcements (
			id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		audience TEXT NOT NULL CHECK (audience IN ('authenticated', 'new_user', 'public')),
		pinned INTEGER NOT NULL DEFAULT 0,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_announcements_audience_enabled ON announcements(audience, enabled, pinned DESC, created_at DESC);

	CREATE TABLE IF NOT EXISTS announcement_reads (
		announcement_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		read_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (announcement_id, user_id),
		FOREIGN KEY (announcement_id) REFERENCES announcements(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_announcement_reads_user ON announcement_reads(user_id, read_at DESC);

	CREATE TABLE IF NOT EXISTS subscription_plans (
		id TEXT PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		legacy_source TEXT NOT NULL DEFAULT '',
		legacy_ref_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_subscription_plans_enabled ON subscription_plans(enabled);

	CREATE TABLE IF NOT EXISTS subscription_plan_limits (
		id TEXT PRIMARY KEY,
		plan_id TEXT NOT NULL,
		limit_type TEXT NOT NULL CHECK (limit_type IN ('daily', 'weekly', 'monthly', 'rolling_5h', 'total')),
		window_mode TEXT NOT NULL DEFAULT 'fixed' CHECK (window_mode IN ('fixed', 'sliding')),
		limit_micros INTEGER NOT NULL CHECK (limit_micros >= 0),
		fixed_reset_minute INTEGER CHECK (fixed_reset_minute >= 0 AND fixed_reset_minute < 1440),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE CASCADE
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_plan_limits_unique ON subscription_plan_limits(plan_id, limit_type);

	CREATE TABLE IF NOT EXISTS user_subscriptions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		plan_id TEXT NOT NULL,
		starts_at DATETIME NOT NULL,
		expires_at DATETIME,
		status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'expired', 'cancelled')),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
	);
	CREATE INDEX IF NOT EXISTS idx_user_subs_user_created ON user_subscriptions(user_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_user_subs_plan_status ON user_subscriptions(plan_id, status);
	CREATE INDEX IF NOT EXISTS idx_user_subs_status ON user_subscriptions(status);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_user_subs_active_unique ON user_subscriptions(user_id) WHERE status = 'active';

	CREATE TABLE IF NOT EXISTS subscription_recharge_history (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		user_subscription_id TEXT NOT NULL,
		plan_id TEXT NOT NULL DEFAULT '',
		mode TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT '',
		source_type TEXT NOT NULL DEFAULT '',
		source_ref_id TEXT NOT NULL DEFAULT '',
		source_daily_limit_micros BIGINT,
		target_daily_limit_before_micros BIGINT,
		peak_daily_limit_micros BIGINT,
		target_expires_at_before DATETIME,
		target_expires_at_after DATETIME,
		preview_json TEXT NOT NULL DEFAULT '',
		confirmed_at DATETIME,
		applied_at DATETIME,
		legacy_source TEXT NOT NULL DEFAULT '',
		legacy_ref_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE CASCADE,
		FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
	);
	CREATE INDEX IF NOT EXISTS idx_subscription_recharge_history_user_created ON subscription_recharge_history(user_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_subscription_recharge_history_sub_created ON subscription_recharge_history(user_subscription_id, created_at DESC);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_recharge_history_legacy_ref ON subscription_recharge_history(legacy_source, legacy_ref_id) WHERE legacy_source <> '' AND legacy_ref_id <> '';

	CREATE TABLE IF NOT EXISTS subscription_timeline_phases (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		user_subscription_id TEXT NOT NULL,
		plan_id TEXT NOT NULL,
		phase_type TEXT NOT NULL CHECK (phase_type IN ('boost', 'restore', 'converted_extension', 'overwrite', 'extend_duration')),
		status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'active', 'completed', 'superseded', 'cancelled')),
		source_type TEXT NOT NULL DEFAULT '',
		source_ref_id TEXT NOT NULL DEFAULT '',
		daily_limit_micros BIGINT,
		weekly_limit_micros BIGINT,
		monthly_limit_micros BIGINT,
		rolling_5h_limit_micros BIGINT,
		total_limit_micros BIGINT,
		fixed_reset_minute INTEGER CHECK (fixed_reset_minute >= 0 AND fixed_reset_minute < 1440),
		starts_at DATETIME NOT NULL,
		ends_at DATETIME NOT NULL,
		final_expires_at DATETIME,
		preview_json TEXT NOT NULL DEFAULT '',
		applied_at DATETIME,
		legacy_source TEXT NOT NULL DEFAULT '',
		legacy_ref_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE CASCADE,
		FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
	);
	CREATE INDEX IF NOT EXISTS idx_subscription_timeline_phases_sub_status_start ON subscription_timeline_phases(user_subscription_id, status, starts_at);
	CREATE INDEX IF NOT EXISTS idx_subscription_timeline_phases_source_ref ON subscription_timeline_phases(source_type, source_ref_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_timeline_phases_legacy_ref ON subscription_timeline_phases(legacy_source, legacy_ref_id) WHERE legacy_source <> '' AND legacy_ref_id <> '';

	CREATE TABLE IF NOT EXISTS subscription_entitlements (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		plan_id TEXT NOT NULL,
		source_type TEXT NOT NULL CHECK (source_type IN ('legacy_snapshot', 'purchase', 'redeem', 'admin_assign', 'admin_adjust', 'upgrade')),
		source_ref_id TEXT NOT NULL DEFAULT '',
		valuation_cny_cent_per_day BIGINT NOT NULL DEFAULT 0 CHECK (valuation_cny_cent_per_day >= 0),
		starts_at DATETIME NOT NULL,
		expires_at DATETIME,
		status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'consumed', 'cancelled')),
		consumed_by_purchase_order_no TEXT NOT NULL DEFAULT '',
		consumed_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
	);
	CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_user_status_expiry ON subscription_entitlements(user_id, status, expires_at);
	CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_plan_status ON subscription_entitlements(plan_id, status);

	CREATE TABLE IF NOT EXISTS user_billing_settings (
		user_id TEXT PRIMARY KEY,
		primary_source TEXT NOT NULL DEFAULT 'subscription' CHECK (primary_source IN ('subscription', 'balance')),
		secondary_source TEXT NOT NULL DEFAULT 'balance' CHECK (secondary_source IN ('subscription', 'balance')),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		CHECK (primary_source != secondary_source),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS purchase_products (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		summary TEXT NOT NULL DEFAULT '',
		product_kind TEXT NOT NULL DEFAULT 'subscription' CHECK (product_kind IN ('subscription', 'balance_topup')),
		subscription_plan_id TEXT NOT NULL,
		duration_days INTEGER NOT NULL CHECK (duration_days > 0),
		price_cny_cent BIGINT NOT NULL CHECK (price_cny_cent > 0),
		action_snapshot_json TEXT NOT NULL DEFAULT '',
		legacy_source TEXT NOT NULL DEFAULT '',
		legacy_ref_id TEXT NOT NULL DEFAULT '',
		group_name TEXT NOT NULL DEFAULT '',
		group_sort INTEGER NOT NULL DEFAULT 0,
		is_recommended INTEGER NOT NULL DEFAULT 0,
		sort_order INTEGER NOT NULL DEFAULT 0,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
	);
	CREATE INDEX IF NOT EXISTS idx_purchase_products_enabled_sort ON purchase_products(enabled, is_recommended DESC, sort_order ASC, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_purchase_products_plan ON purchase_products(subscription_plan_id);

		CREATE TABLE IF NOT EXISTS purchase_orders (
			id TEXT PRIMARY KEY,
			order_no TEXT UNIQUE NOT NULL,
			user_id TEXT NOT NULL,
			product_id TEXT NOT NULL,
			subscription_plan_id TEXT NOT NULL,
			duration_days INTEGER NOT NULL CHECK (duration_days > 0),
			original_amount_cny_cent BIGINT NOT NULL DEFAULT 0,
			discount_cny_cent BIGINT NOT NULL DEFAULT 0,
		amount_cny_cent BIGINT NOT NULL CHECK (amount_cny_cent >= 0),
		order_kind TEXT NOT NULL DEFAULT 'subscription' CHECK (order_kind IN ('subscription', 'balance_topup')),
		delivery_mode TEXT NOT NULL DEFAULT 'account' CHECK (delivery_mode IN ('account', 'redeem_code')),
		balance_topup_micros BIGINT NOT NULL DEFAULT 0,
		payment_channel TEXT NOT NULL CHECK (payment_channel IN ('alipay')),
		generated_redeem_code_id TEXT NOT NULL DEFAULT '',
		coupon_campaign_id TEXT NOT NULL DEFAULT '',
		coupon_campaign_name TEXT NOT NULL DEFAULT '',
		coupon_code_id TEXT NOT NULL DEFAULT '',
		coupon_code_value TEXT NOT NULL DEFAULT '',
		coupon_discount_type TEXT NOT NULL DEFAULT '',
		coupon_percent_off_bps INTEGER NOT NULL DEFAULT 0,
		coupon_fixed_discount_cny_cent BIGINT NOT NULL DEFAULT 0,
		coupon_max_discount_cny_cent BIGINT NOT NULL DEFAULT 0,
		legacy_source TEXT NOT NULL DEFAULT '',
		legacy_ref_id TEXT NOT NULL DEFAULT '',
		payment_status TEXT NOT NULL DEFAULT 'pending' CHECK (payment_status IN ('pending', 'paid', 'expired', 'refunded', 'closed', 'failed')),
		fulfillment_status TEXT NOT NULL DEFAULT 'pending' CHECK (fulfillment_status IN ('pending', 'fulfilled', 'failed')),
		manual_settlement_done INTEGER NOT NULL DEFAULT 0,
		alipay_trade_no TEXT NOT NULL DEFAULT '',
		alipay_qr_code TEXT NOT NULL DEFAULT '',
		alipay_qr_url TEXT NOT NULL DEFAULT '',
		expires_at DATETIME,
		paid_at DATETIME,
		fulfilled_at DATETIME,
		failure_reason TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (product_id) REFERENCES purchase_products(id) ON DELETE RESTRICT,
		FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
	);
	CREATE INDEX IF NOT EXISTS idx_purchase_orders_user_created ON purchase_orders(user_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_purchase_orders_payment_created ON purchase_orders(payment_status, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_purchase_orders_fulfillment_created ON purchase_orders(fulfillment_status, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_purchase_orders_product_created ON purchase_orders(product_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_purchase_orders_trade_no ON purchase_orders(alipay_trade_no);
	CREATE INDEX IF NOT EXISTS idx_purchase_orders_manual_settlement_done ON purchase_orders(manual_settlement_done, created_at DESC);

	CREATE TABLE IF NOT EXISTS coupon_campaigns (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		code_mode TEXT NOT NULL CHECK (code_mode IN ('shared', 'single_use')),
		shared_code TEXT NOT NULL DEFAULT '',
		discount_type TEXT NOT NULL CHECK (discount_type IN ('fixed_amount', 'percentage')),
		fixed_discount_cny_cent BIGINT NOT NULL DEFAULT 0,
		percent_off_bps INTEGER NOT NULL DEFAULT 0,
		max_discount_cny_cent BIGINT NOT NULL DEFAULT 0,
		total_usage_limit INTEGER NOT NULL DEFAULT 0,
		used_count INTEGER NOT NULL DEFAULT 0,
		per_user_limit INTEGER NOT NULL DEFAULT 1,
		starts_at DATETIME,
		ends_at DATETIME,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_coupon_campaigns_enabled_window ON coupon_campaigns(enabled, starts_at, ends_at);

	CREATE TABLE IF NOT EXISTS coupon_code_batches (
		id TEXT PRIMARY KEY,
		campaign_id TEXT NOT NULL,
		name TEXT NOT NULL,
		prefix TEXT NOT NULL DEFAULT '',
		code_count INTEGER NOT NULL DEFAULT 0,
		code_length INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (campaign_id) REFERENCES coupon_campaigns(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_coupon_code_batches_campaign_created ON coupon_code_batches(campaign_id, created_at DESC);

	CREATE TABLE IF NOT EXISTS coupon_codes (
		id TEXT PRIMARY KEY,
		campaign_id TEXT NOT NULL,
		batch_id TEXT,
		code_mode TEXT NOT NULL CHECK (code_mode IN ('shared', 'single_use')),
		code_value TEXT NOT NULL UNIQUE,
		code_hash TEXT NOT NULL UNIQUE,
		code_mask TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'consumed')),
		max_usages INTEGER NOT NULL DEFAULT 1,
		used_count INTEGER NOT NULL DEFAULT 0,
		last_used_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (campaign_id) REFERENCES coupon_campaigns(id) ON DELETE CASCADE,
		FOREIGN KEY (batch_id) REFERENCES coupon_code_batches(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_coupon_codes_campaign_status_created ON coupon_codes(campaign_id, status, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_coupon_codes_batch_created ON coupon_codes(batch_id, created_at DESC);

	CREATE TABLE IF NOT EXISTS coupon_usages (
		id TEXT PRIMARY KEY,
		campaign_id TEXT NOT NULL,
		code_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		order_id TEXT NOT NULL,
		order_no TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('applied', 'reversed')),
		discount_cny_cent BIGINT NOT NULL DEFAULT 0,
		final_amount_cny_cent BIGINT NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (campaign_id) REFERENCES coupon_campaigns(id) ON DELETE CASCADE,
		FOREIGN KEY (code_id) REFERENCES coupon_codes(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (order_id) REFERENCES purchase_orders(id) ON DELETE CASCADE
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_coupon_usages_order_unique ON coupon_usages(order_id);
	CREATE INDEX IF NOT EXISTS idx_coupon_usages_user_campaign_status ON coupon_usages(user_id, campaign_id, status, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_coupon_usages_campaign_status ON coupon_usages(campaign_id, status, created_at DESC);

	CREATE TABLE IF NOT EXISTS redeem_campaigns (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		code_mode TEXT NOT NULL CHECK (code_mode IN ('single_use', 'shared')),
		subscription_plan_id TEXT,
		subscription_duration_days INTEGER NOT NULL DEFAULT 0 CHECK (subscription_duration_days >= 0),
		balance_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_micros >= 0),
		total_redemptions_limit INTEGER NOT NULL DEFAULT 0 CHECK (total_redemptions_limit >= 0),
		redeemed_count INTEGER NOT NULL DEFAULT 0 CHECK (redeemed_count >= 0),
		per_user_limit INTEGER NOT NULL DEFAULT 1 CHECK (per_user_limit > 0),
		starts_at DATETIME,
		ends_at DATETIME,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
	);
	CREATE INDEX IF NOT EXISTS idx_redeem_campaigns_mode_created ON redeem_campaigns(code_mode, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_redeem_campaigns_enabled_window ON redeem_campaigns(enabled, starts_at, ends_at);

	CREATE TABLE IF NOT EXISTS redeem_code_batches (
		id TEXT PRIMARY KEY,
		campaign_id TEXT NOT NULL,
		name TEXT NOT NULL,
		prefix TEXT NOT NULL DEFAULT '',
		code_count INTEGER NOT NULL CHECK (code_count > 0),
		code_length INTEGER NOT NULL CHECK (code_length >= 6),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (campaign_id) REFERENCES redeem_campaigns(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_redeem_code_batches_campaign_created ON redeem_code_batches(campaign_id, created_at DESC);

	CREATE TABLE IF NOT EXISTS redeem_codes (
		id TEXT PRIMARY KEY,
		campaign_id TEXT,
		batch_id TEXT,
		source_type TEXT NOT NULL DEFAULT 'campaign' CHECK (source_type IN ('campaign', 'free', 'purchase_order', 'legacy_delivery')),
		source_ref_id TEXT NOT NULL DEFAULT '',
		code_value TEXT NOT NULL UNIQUE,
		code_hash TEXT NOT NULL UNIQUE,
		code_mask TEXT NOT NULL,
		subscription_plan_id TEXT,
		subscription_duration_days INTEGER NOT NULL DEFAULT 0 CHECK (subscription_duration_days >= 0),
		balance_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_micros >= 0),
		reward_snapshot_json TEXT NOT NULL DEFAULT '',
		legacy_source TEXT NOT NULL DEFAULT '',
		legacy_ref_id TEXT NOT NULL DEFAULT '',
		per_user_limit INTEGER NOT NULL DEFAULT 1 CHECK (per_user_limit > 0),
		starts_at DATETIME,
		ends_at DATETIME,
		status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'consumed')),
		max_redemptions INTEGER NOT NULL DEFAULT 1 CHECK (max_redemptions >= 0),
		redeemed_count INTEGER NOT NULL DEFAULT 0 CHECK (redeemed_count >= 0),
		last_redeemed_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (campaign_id) REFERENCES redeem_campaigns(id) ON DELETE SET NULL,
		FOREIGN KEY (batch_id) REFERENCES redeem_code_batches(id) ON DELETE CASCADE,
		FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
	);
	CREATE INDEX IF NOT EXISTS idx_redeem_codes_campaign_status_created ON redeem_codes(campaign_id, status, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_redeem_codes_batch_created ON redeem_codes(batch_id, created_at DESC);

	CREATE TABLE IF NOT EXISTS redeem_user_counters (
		code_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		redeemed_count INTEGER NOT NULL DEFAULT 0 CHECK (redeemed_count >= 0),
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (code_id, user_id),
		FOREIGN KEY (code_id) REFERENCES redeem_codes(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_redeem_user_counters_user_updated ON redeem_user_counters(user_id, updated_at DESC);

	CREATE TABLE IF NOT EXISTS redeem_redemptions (
		id TEXT PRIMARY KEY,
		campaign_id TEXT,
		code_id TEXT,
		user_id TEXT NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		code_input TEXT NOT NULL DEFAULT '',
		code_mask TEXT NOT NULL DEFAULT '',
		subscription_plan_id TEXT,
		subscription_duration_days INTEGER NOT NULL DEFAULT 0 CHECK (subscription_duration_days >= 0),
		balance_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_micros >= 0),
		reward_snapshot_json TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL CHECK (status IN ('success', 'rejected')),
		failure_reason TEXT NOT NULL DEFAULT '',
		granted_subscription_id TEXT,
		granted_expires_at DATETIME,
		balance_after_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_after_micros >= 0),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (campaign_id) REFERENCES redeem_campaigns(id) ON DELETE SET NULL,
		FOREIGN KEY (code_id) REFERENCES redeem_codes(id) ON DELETE SET NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT,
		FOREIGN KEY (granted_subscription_id) REFERENCES user_subscriptions(id) ON DELETE SET NULL
	);
	CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_user_created ON redeem_redemptions(user_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_campaign_created ON redeem_redemptions(campaign_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_code_created ON redeem_redemptions(code_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_status_created ON redeem_redemptions(status, created_at DESC);

	CREATE TABLE IF NOT EXISTS billing_account_state (
		user_id TEXT PRIMARY KEY,
		primary_source TEXT NOT NULL DEFAULT 'subscription' CHECK (primary_source IN ('subscription', 'balance')),
		secondary_source TEXT NOT NULL DEFAULT 'balance' CHECK (secondary_source IN ('subscription', 'balance')),
		balance_micros BIGINT NOT NULL DEFAULT 0,
		active_subscription_id TEXT,
		active_plan_id TEXT,
		subscription_starts_at DATETIME,
		subscription_expires_at DATETIME,
		revision BIGINT NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS subscription_window_state (
		id TEXT PRIMARY KEY,
		user_subscription_id TEXT NOT NULL,
		plan_id TEXT NOT NULL,
		limit_type TEXT NOT NULL,
		window_mode TEXT NOT NULL,
		window_start DATETIME NOT NULL,
		window_end DATETIME NOT NULL,
		limit_micros BIGINT NOT NULL,
		used_micros BIGINT NOT NULL DEFAULT 0,
		reserved_micros BIGINT NOT NULL DEFAULT 0,
		remaining_micros BIGINT NOT NULL,
		revision BIGINT NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_subscription_id, limit_type, window_start),
		FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE CASCADE,
		FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_subscription_window_state_sub_window ON subscription_window_state(user_subscription_id, window_start DESC);
	CREATE INDEX IF NOT EXISTS idx_subscription_window_state_plan_limit ON subscription_window_state(plan_id, limit_type, window_start DESC);

		CREATE TABLE IF NOT EXISTS billing_reservations (
			request_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			user_subscription_id TEXT,
			pricing_model TEXT,
		estimated_cost_micros BIGINT NOT NULL DEFAULT 0,
		actual_cost_micros BIGINT NOT NULL DEFAULT 0,
		reserved_subscription_micros BIGINT NOT NULL DEFAULT 0,
		reserved_balance_micros BIGINT NOT NULL DEFAULT 0,
			charged_subscription_micros BIGINT NOT NULL DEFAULT 0,
			charged_balance_micros BIGINT NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			window_refs_json TEXT,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE SET NULL
	);
	CREATE INDEX IF NOT EXISTS idx_billing_reservations_user_status ON billing_reservations(user_id, status, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_billing_reservations_expires ON billing_reservations(status, expires_at);

	CREATE TABLE IF NOT EXISTS billing_projection_events (
		stream_id TEXT PRIMARY KEY,
		request_id TEXT,
		event_type TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_billing_projection_events_request ON billing_projection_events(request_id, created_at DESC);

	CREATE TABLE IF NOT EXISTS billing_daily_reset_records (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		user_subscription_id TEXT NOT NULL,
		window_start DATETIME NOT NULL,
		window_end DATETIME NOT NULL,
		used_micros_before_reset BIGINT NOT NULL DEFAULT 0,
		expires_at_before DATETIME NOT NULL,
		expires_at_after DATETIME NOT NULL,
		legacy_source TEXT NOT NULL DEFAULT '',
		legacy_ref_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_billing_daily_reset_user_created ON billing_daily_reset_records(user_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_billing_daily_reset_sub_window_created ON billing_daily_reset_records(user_subscription_id, window_start DESC, created_at DESC);

	CREATE TABLE IF NOT EXISTS billing_events (
		id TEXT PRIMARY KEY,
		request_log_id TEXT,
		user_id TEXT NOT NULL,
		user_subscription_id TEXT,
		source TEXT NOT NULL CHECK (source IN ('subscription', 'balance')),
		event_type TEXT NOT NULL CHECK (event_type IN ('charge', 'refund', 'adjustment')),
		amount_micros INTEGER NOT NULL CHECK (amount_micros >= 0),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_billing_events_usage_window ON billing_events(user_subscription_id, source, created_at);
	CREATE INDEX IF NOT EXISTS idx_billing_events_user_created ON billing_events(user_id, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_billing_events_request_created ON billing_events(request_log_id, created_at DESC);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_events_idempotent ON billing_events(request_log_id, source, event_type) WHERE request_log_id IS NOT NULL;
	`
	if dbType == DBTypePostgres {
		schema = strings.ReplaceAll(schema, "DATETIME", "TIMESTAMPTZ")
		replacer := strings.NewReplacer(
			"balance_micros INTEGER NOT NULL DEFAULT 0", "balance_micros BIGINT NOT NULL DEFAULT 0",
			"input_tokens INTEGER,", "input_tokens BIGINT,",
			"output_tokens INTEGER,", "output_tokens BIGINT,",
			"cache_read_input_tokens INTEGER,", "cache_read_input_tokens BIGINT,",
			"cache_creation_input_tokens INTEGER,", "cache_creation_input_tokens BIGINT,",
			"cost_micros INTEGER,", "cost_micros BIGINT,",
			"charged_subscription_micros INTEGER NOT NULL DEFAULT 0", "charged_subscription_micros BIGINT NOT NULL DEFAULT 0",
			"charged_balance_micros INTEGER NOT NULL DEFAULT 0", "charged_balance_micros BIGINT NOT NULL DEFAULT 0",
			"limit_micros INTEGER NOT NULL CHECK (limit_micros >= 0)", "limit_micros BIGINT NOT NULL CHECK (limit_micros >= 0)",
			"amount_micros INTEGER NOT NULL CHECK (amount_micros >= 0)", "amount_micros BIGINT NOT NULL CHECK (amount_micros >= 0)",
		)
		schema = replacer.Replace(schema)
		schema += `
		CREATE TABLE IF NOT EXISTS request_log_details_archive (
			request_id TEXT PRIMARY KEY,
			request_headers TEXT,
			request_body TEXT,
			translated_request_body TEXT,
			translated_request_headers TEXT,
			response_headers TEXT,
			response_body TEXT,
			translated_response_body TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_request_log_details_archive_created ON request_log_details_archive(created_at DESC);
		`
	}

	for _, statement := range splitStatements(schema) {
		if _, err := db.Exec(statement); err != nil {
			if shouldIgnoreCreateTableStatementError(statement, err) {
				continue
			}
			return err
		}
	}
	return nil
}

func runMigrations() error {
	if err := ensureSchemaMigrationsTable(); err != nil {
		return fmt.Errorf("ensure schema_migrations table failed: %w", err)
	}

	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "add_api_key_column",
			sql:  `ALTER TABLE user_api_keys ADD COLUMN api_key TEXT NOT NULL DEFAULT ''`,
		},
		{
			name: "migrate_proxy_tokens",
			sql: `INSERT INTO user_api_keys (id, user_id, name, key_hash, prefix, last_used_at, expires_at, revoked_at, created_at)
				SELECT id, user_id, name, token_hash, prefix, last_used_at, expires_at, revoked_at, created_at
				FROM user_proxy_tokens
				WHERE NOT EXISTS (SELECT 1 FROM user_api_keys WHERE user_api_keys.id = user_proxy_tokens.id)`,
		},
		{
			name: "create_admin_management_keys",
			sql: `CREATE TABLE IF NOT EXISTS admin_management_keys (
				user_id TEXT PRIMARY KEY,
				key_hash TEXT NOT NULL DEFAULT '',
				key_ciphertext TEXT NOT NULL DEFAULT '',
				key_prefix TEXT NOT NULL DEFAULT '',
				previous_key_hash TEXT NOT NULL DEFAULT '',
				previous_key_ciphertext TEXT NOT NULL DEFAULT '',
				previous_key_prefix TEXT NOT NULL DEFAULT '',
				previous_valid_until DATETIME,
				enabled INTEGER NOT NULL DEFAULT 1,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				rotated_at DATETIME,
				last_used_at DATETIME,
				last_auth_method TEXT NOT NULL DEFAULT '',
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_admin_management_keys_hash ON admin_management_keys(key_hash);
			CREATE INDEX IF NOT EXISTS idx_admin_management_keys_previous_hash ON admin_management_keys(previous_key_hash)`,
		},
		{
			name: "drop_api_key_allowed_providers_json",
			sql:  ``,
		},
		{
			name: "add_channels_endpoint",
			sql:  `ALTER TABLE channels ADD COLUMN endpoint TEXT NOT NULL DEFAULT 'chat_completions'`,
		},
		{
			name: "add_request_logs_status",
			sql:  `ALTER TABLE request_logs ADD COLUMN status TEXT NOT NULL DEFAULT 'success'`,
		},
		{
			name: "add_request_logs_updated_at",
			sql:  `ALTER TABLE request_logs ADD COLUMN updated_at DATETIME`,
		},
		{
			name: "add_request_logs_session_id",
			sql:  `ALTER TABLE request_logs ADD COLUMN session_id TEXT`,
		},
		{
			name: "add_request_logs_status_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_request_logs_status ON request_logs(status)`,
		},
		{
			name: "add_request_logs_session_time_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_request_logs_session_time ON request_logs(session_id, created_at DESC)`,
		},
		{
			name: "add_original_model_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_request_logs_original_model_time ON request_logs(original_model, created_at DESC)`,
		},
		{
			name: "add_request_logs_response_text",
			sql:  `ALTER TABLE request_logs ADD COLUMN response_text TEXT`,
		},
		{
			name: "add_request_log_details_translated_request_body",
			sql:  `ALTER TABLE request_log_details ADD COLUMN translated_request_body TEXT`,
		},
		{
			name: "add_request_log_details_translated_response_body",
			sql:  `ALTER TABLE request_log_details ADD COLUMN translated_response_body TEXT`,
		},
		{
			name: "add_request_log_details_translated_request_headers",
			sql:  `ALTER TABLE request_log_details ADD COLUMN translated_request_headers TEXT`,
		},
		{
			name: "add_channels_enabled_priority_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_channels_enabled_priority ON channels(enabled, priority ASC, weight DESC)`,
		},
		{
			name: "add_web_search_mode",
			sql:  `ALTER TABLE user_amp_settings ADD COLUMN web_search_mode TEXT NOT NULL DEFAULT 'upstream'`,
		},
		{
			name: "add_request_logs_cost_micros",
			sql:  `ALTER TABLE request_logs ADD COLUMN cost_micros INTEGER`,
		},
		{
			name: "add_request_logs_cost_usd",
			sql:  `ALTER TABLE request_logs ADD COLUMN cost_usd TEXT`,
		},
		{
			name: "add_request_logs_pricing_model",
			sql:  `ALTER TABLE request_logs ADD COLUMN pricing_model TEXT`,
		},
		{
			name: "add_request_logs_thinking_level",
			sql:  `ALTER TABLE request_logs ADD COLUMN thinking_level TEXT`,
		},
		{
			name: "add_request_logs_ttfb_ms",
			sql:  `ALTER TABLE request_logs ADD COLUMN ttfb_ms INTEGER`,
		},
		{
			name: "add_native_mode",
			sql:  `ALTER TABLE user_amp_settings ADD COLUMN native_mode INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "create_user_groups_table",
			sql: `CREATE TABLE IF NOT EXISTS user_groups (
					user_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				PRIMARY KEY (user_id, group_id),
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
				FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
			)`,
		},
		{
			name: "create_user_groups_indexes",
			sql: `CREATE INDEX IF NOT EXISTS idx_user_groups_user ON user_groups(user_id);
				  CREATE INDEX IF NOT EXISTS idx_user_groups_group ON user_groups(group_id)`,
		},
		{
			name: "create_channel_groups_table",
			sql: `CREATE TABLE IF NOT EXISTS channel_groups (
				channel_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				PRIMARY KEY (channel_id, group_id),
				FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
				FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
			)`,
		},
		{
			name: "create_channel_groups_indexes",
			sql: `CREATE INDEX IF NOT EXISTS idx_channel_groups_channel ON channel_groups(channel_id);
				  CREATE INDEX IF NOT EXISTS idx_channel_groups_group ON channel_groups(group_id)`,
		},
		{
			name: "add_group_rate_multiplier",
			sql:  `ALTER TABLE groups ADD COLUMN rate_multiplier REAL NOT NULL DEFAULT 1.0`,
		},
		{
			name: "add_group_rate_multiplier_ppm",
			sql: `ALTER TABLE groups ADD COLUMN rate_multiplier_ppm BIGINT NOT NULL DEFAULT 1000000;
				  UPDATE groups
				  SET rate_multiplier_ppm = CAST(ROUND(COALESCE(rate_multiplier, 1.0) * 1000000.0) AS INTEGER)`,
		},
		{
			name: "add_user_balance_micros",
			sql:  `ALTER TABLE users ADD COLUMN balance_micros INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_request_logs_rate_multiplier",
			sql:  `ALTER TABLE request_logs ADD COLUMN rate_multiplier REAL`,
		},
		{
			name: "add_request_logs_charged_subscription_micros",
			sql:  `ALTER TABLE request_logs ADD COLUMN charged_subscription_micros INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_request_logs_charged_balance_micros",
			sql:  `ALTER TABLE request_logs ADD COLUMN charged_balance_micros INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_user_concurrency_limit",
			sql:  `ALTER TABLE users ADD COLUMN concurrency_limit INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_channels_rate_multiplier",
			sql:  `ALTER TABLE channels ADD COLUMN rate_multiplier REAL NOT NULL DEFAULT 1.0`,
		},
		{
			name: "add_channels_rate_multiplier_ppm",
			sql: `ALTER TABLE channels ADD COLUMN rate_multiplier_ppm BIGINT NOT NULL DEFAULT 1000000;
				  UPDATE channels
				  SET rate_multiplier_ppm = CAST(ROUND(COALESCE(rate_multiplier, 1.0) * 1000000.0) AS INTEGER)`,
		},
		{
			name: "add_request_logs_channel_rate_multiplier",
			sql:  `ALTER TABLE request_logs ADD COLUMN channel_rate_multiplier REAL`,
		},
		{
			name: "add_request_logs_rate_multiplier_ppm",
			sql: `ALTER TABLE request_logs ADD COLUMN rate_multiplier_ppm BIGINT;
				  UPDATE request_logs
				  SET rate_multiplier_ppm = CAST(ROUND(rate_multiplier * 1000000.0) AS INTEGER)
				  WHERE rate_multiplier IS NOT NULL`,
		},
		{
			name: "add_request_logs_channel_rate_multiplier_ppm",
			sql: `ALTER TABLE request_logs ADD COLUMN channel_rate_multiplier_ppm BIGINT;
				  UPDATE request_logs
				  SET channel_rate_multiplier_ppm = CAST(ROUND(channel_rate_multiplier * 1000000.0) AS INTEGER)
				  WHERE channel_rate_multiplier IS NOT NULL`,
		},
		{
			name: "add_request_logs_group_rate_multiplier",
			sql:  `ALTER TABLE request_logs ADD COLUMN group_rate_multiplier REAL`,
		},
		{
			name: "add_request_logs_group_rate_multiplier_ppm",
			sql: `ALTER TABLE request_logs ADD COLUMN group_rate_multiplier_ppm BIGINT;
				  UPDATE request_logs
				  SET group_rate_multiplier_ppm = CAST(ROUND(group_rate_multiplier * 1000000.0) AS INTEGER)
				  WHERE group_rate_multiplier IS NOT NULL`,
		},
		{
			name: "add_request_logs_special_rate_multiplier",
			sql:  `ALTER TABLE request_logs ADD COLUMN special_rate_multiplier REAL`,
		},
		{
			name: "add_request_logs_special_rate_multiplier_ppm",
			sql: `ALTER TABLE request_logs ADD COLUMN special_rate_multiplier_ppm BIGINT;
				  UPDATE request_logs
				  SET special_rate_multiplier_ppm = CAST(ROUND(special_rate_multiplier * 1000000.0) AS INTEGER)
				  WHERE special_rate_multiplier IS NOT NULL`,
		},
		{
			name: "add_request_logs_special_rate_reason",
			sql:  `ALTER TABLE request_logs ADD COLUMN special_rate_reason TEXT`,
		},
		{
			name: "add_request_logs_pricing_rule_name",
			sql:  `ALTER TABLE request_logs ADD COLUMN pricing_rule_name TEXT`,
		},
		{
			name: "add_request_logs_billing_status",
			sql:  `ALTER TABLE request_logs ADD COLUMN billing_status TEXT NOT NULL DEFAULT 'none'`,
		},
		{
			name: "add_billing_events_usage_window_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_billing_events_usage_window ON billing_events(user_subscription_id, source, created_at)`,
		},
		{
			name: "add_show_balance_in_ad",
			sql:  `ALTER TABLE user_amp_settings ADD COLUMN show_balance_in_ad INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_channels_model_whitelist",
			sql:  `ALTER TABLE channels ADD COLUMN model_whitelist INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_channels_simulate_cli",
			sql:  `ALTER TABLE channels ADD COLUMN simulate_cli INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_channels_codex_websocket_enabled",
			sql:  `ALTER TABLE channels ADD COLUMN codex_websocket_enabled INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_socks5_proxy",
			sql:  `ALTER TABLE user_amp_settings ADD COLUMN socks5_proxy TEXT NOT NULL DEFAULT ''`,
		},
		{
			name: "add_route_mappings_enabled",
			sql:  `ALTER TABLE user_amp_settings ADD COLUMN route_mappings_enabled INTEGER NOT NULL DEFAULT 1`,
		},
		{
			name: "add_request_detail_retention_default",
			sql:  `INSERT OR IGNORE INTO system_config (key, value, updated_at) VALUES ('request_detail_retention_days', '30', CURRENT_TIMESTAMP)`,
		},
		{
			name: "rename_retention_to_archive_days",
			sql: `UPDATE system_config SET key = 'request_detail_archive_days' WHERE key = 'request_detail_retention_days'
				AND NOT EXISTS (SELECT 1 FROM system_config WHERE key = 'request_detail_archive_days')`,
		},
		{
			name: "add_request_logs_status_time_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_request_logs_status_time ON request_logs(status, created_at DESC)`,
		},
		{
			name: "add_request_logs_status_code_time_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_request_logs_status_code_time ON request_logs(status_code, created_at DESC)`,
		},
		{
			name: "add_request_logs_streaming_time_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_request_logs_streaming_time ON request_logs(is_streaming, created_at DESC)`,
		},
		{
			name: "add_request_logs_effective_model_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_request_logs_effective_model ON request_logs(COALESCE(mapped_model, original_model))`,
		},
		{
			name: "add_request_logs_downstream_transport",
			sql:  `ALTER TABLE request_logs ADD COLUMN downstream_transport TEXT`,
		},
		{
			name: "add_request_logs_upstream_transport",
			sql:  `ALTER TABLE request_logs ADD COLUMN upstream_transport TEXT`,
		},
		{
			name: "add_request_logs_transport_fallback_reason",
			sql:  `ALTER TABLE request_logs ADD COLUMN transport_fallback_reason TEXT`,
		},
		{
			name: "add_request_logs_request_format",
			sql:  `ALTER TABLE request_logs ADD COLUMN request_format TEXT`,
		},
		{
			name: "add_request_logs_upstream_format",
			sql:  `ALTER TABLE request_logs ADD COLUMN upstream_format TEXT`,
		},
		{
			name: "create_global_request_metric_projections_table",
			sql: `CREATE TABLE IF NOT EXISTS global_request_metric_projections (
					request_id TEXT PRIMARY KEY,
					minute_bucket DATETIME NOT NULL,
					request_count BIGINT NOT NULL DEFAULT 0,
					input_tokens BIGINT NOT NULL DEFAULT 0,
					output_tokens BIGINT NOT NULL DEFAULT 0,
					total_tokens BIGINT NOT NULL DEFAULT 0,
					latency_ms BIGINT NOT NULL DEFAULT 0,
					ttfb_ms BIGINT NOT NULL DEFAULT 0,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)`,
		},
		{
			name: "create_global_request_metric_projections_indexes",
			sql:  `CREATE INDEX IF NOT EXISTS idx_global_request_metric_projections_minute ON global_request_metric_projections(minute_bucket DESC)`,
		},
		{
			name: "add_global_request_metric_projections_latency_ms",
			sql:  `ALTER TABLE global_request_metric_projections ADD COLUMN latency_ms BIGINT NOT NULL DEFAULT 0`,
		},
		{
			name: "add_global_request_metric_projections_ttfb_ms",
			sql:  `ALTER TABLE global_request_metric_projections ADD COLUMN ttfb_ms BIGINT NOT NULL DEFAULT 0`,
		},
		{
			name: "create_global_request_minute_metrics_table",
			sql: `CREATE TABLE IF NOT EXISTS global_request_minute_metrics (
				minute_bucket DATETIME PRIMARY KEY,
				request_count_sum BIGINT NOT NULL DEFAULT 0,
				input_tokens_sum BIGINT NOT NULL DEFAULT 0,
				output_tokens_sum BIGINT NOT NULL DEFAULT 0,
				total_tokens_sum BIGINT NOT NULL DEFAULT 0,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
		},
		{
			name: "create_global_request_minute_metrics_indexes",
			sql:  `CREATE INDEX IF NOT EXISTS idx_global_request_minute_metrics_minute ON global_request_minute_metrics(minute_bucket DESC)`,
		},
		{
			name: "add_request_log_details_archive_translated_request_headers",
			sql:  `ALTER TABLE request_log_details_archive ADD COLUMN translated_request_headers TEXT`,
		},
		{
			name: "add_api_keys_user_created_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_api_keys_user_created ON user_api_keys(user_id, created_at DESC)`,
		},
		{
			name: "add_api_keys_user_active_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_api_keys_user_active ON user_api_keys(user_id, revoked_at)`,
		},
		{
			name: "add_channels_priority_created_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_channels_priority_created ON channels(priority ASC, created_at DESC)`,
		},
		{
			name: "add_model_metadata_provider_pattern_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_model_metadata_provider_pattern ON model_metadata(provider, model_pattern)`,
		},
		{
			name: "add_user_subs_user_created_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_user_subs_user_created ON user_subscriptions(user_id, created_at DESC)`,
		},
		{
			name: "add_user_subs_plan_status_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_user_subs_plan_status ON user_subscriptions(plan_id, status)`,
		},
		{
			name: "add_billing_events_user_created_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_billing_events_user_created ON billing_events(user_id, created_at DESC)`,
		},
		{
			name: "add_billing_events_request_created_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_billing_events_request_created ON billing_events(request_log_id, created_at DESC)`,
		},
		{
			name: "create_billing_account_state_table",
			sql: `CREATE TABLE IF NOT EXISTS billing_account_state (
				user_id TEXT PRIMARY KEY,
				primary_source TEXT NOT NULL DEFAULT 'subscription',
				secondary_source TEXT NOT NULL DEFAULT 'balance',
				balance_micros BIGINT NOT NULL DEFAULT 0,
				active_subscription_id TEXT,
				active_plan_id TEXT,
				subscription_starts_at DATETIME,
				subscription_expires_at DATETIME,
				revision BIGINT NOT NULL DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			)`,
		},
		{
			name: "create_subscription_window_state_table",
			sql: `CREATE TABLE IF NOT EXISTS subscription_window_state (
				id TEXT PRIMARY KEY,
				user_subscription_id TEXT NOT NULL,
				plan_id TEXT NOT NULL,
				limit_type TEXT NOT NULL,
				window_mode TEXT NOT NULL,
				window_start DATETIME NOT NULL,
				window_end DATETIME NOT NULL,
				limit_micros BIGINT NOT NULL,
				used_micros BIGINT NOT NULL DEFAULT 0,
				reserved_micros BIGINT NOT NULL DEFAULT 0,
				remaining_micros BIGINT NOT NULL,
				revision BIGINT NOT NULL DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(user_subscription_id, limit_type, window_start),
				FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE CASCADE,
				FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE CASCADE
			)`,
		},
		{
			name: "create_subscription_window_state_indexes",
			sql: `CREATE INDEX IF NOT EXISTS idx_subscription_window_state_sub_window ON subscription_window_state(user_subscription_id, window_start DESC);
				  CREATE INDEX IF NOT EXISTS idx_subscription_window_state_plan_limit ON subscription_window_state(plan_id, limit_type, window_start DESC)`,
		},
		{
			name: "create_billing_reservations_table",
			sql: `CREATE TABLE IF NOT EXISTS billing_reservations (
				request_id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				user_subscription_id TEXT,
				pricing_model TEXT,
				estimated_cost_micros BIGINT NOT NULL DEFAULT 0,
				actual_cost_micros BIGINT NOT NULL DEFAULT 0,
				reserved_subscription_micros BIGINT NOT NULL DEFAULT 0,
				reserved_balance_micros BIGINT NOT NULL DEFAULT 0,
				charged_subscription_micros BIGINT NOT NULL DEFAULT 0,
				charged_balance_micros BIGINT NOT NULL DEFAULT 0,
				status TEXT NOT NULL,
				window_refs_json TEXT,
				expires_at DATETIME NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
				FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE SET NULL
			)`,
		},
		{
			name: "add_billing_reservations_window_refs_json",
			sql:  `ALTER TABLE billing_reservations ADD COLUMN window_refs_json TEXT`,
		},
		{
			name: "create_billing_reservations_indexes",
			sql: `CREATE INDEX IF NOT EXISTS idx_billing_reservations_user_status ON billing_reservations(user_id, status, created_at DESC);
				  CREATE INDEX IF NOT EXISTS idx_billing_reservations_expires ON billing_reservations(status, expires_at)`,
		},
		{
			name: "create_billing_projection_events_table",
			sql: `CREATE TABLE IF NOT EXISTS billing_projection_events (
				stream_id TEXT PRIMARY KEY,
				request_id TEXT,
				event_type TEXT NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
		},
		{
			name: "create_billing_projection_events_request_index",
			sql:  `CREATE INDEX IF NOT EXISTS idx_billing_projection_events_request ON billing_projection_events(request_id, created_at DESC)`,
		},
		{
			name: "create_billing_daily_reset_records_table",
			sql: `CREATE TABLE IF NOT EXISTS billing_daily_reset_records (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				user_subscription_id TEXT NOT NULL,
				window_start DATETIME NOT NULL,
				window_end DATETIME NOT NULL,
				used_micros_before_reset BIGINT NOT NULL DEFAULT 0,
				expires_at_before DATETIME NOT NULL,
				expires_at_after DATETIME NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
				FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE CASCADE
			)`,
		},
		{
			name: "create_billing_daily_reset_records_indexes",
			sql: `CREATE INDEX IF NOT EXISTS idx_billing_daily_reset_user_created ON billing_daily_reset_records(user_id, created_at DESC);
				  CREATE INDEX IF NOT EXISTS idx_billing_daily_reset_sub_window_created ON billing_daily_reset_records(user_subscription_id, window_start DESC, created_at DESC)`,
		},
		{
			name: "postgres_widen_users_balance_micros",
			sql:  `ALTER TABLE users ALTER COLUMN balance_micros TYPE BIGINT`,
		},
		{
			name: "postgres_widen_request_logs_micros_columns",
			sql: `ALTER TABLE request_logs ALTER COLUMN cost_micros TYPE BIGINT;
				ALTER TABLE request_logs ALTER COLUMN charged_subscription_micros TYPE BIGINT;
				ALTER TABLE request_logs ALTER COLUMN charged_balance_micros TYPE BIGINT`,
		},
		{
			name: "postgres_widen_request_logs_token_columns",
			sql: `ALTER TABLE request_logs ALTER COLUMN input_tokens TYPE BIGINT;
				ALTER TABLE request_logs ALTER COLUMN output_tokens TYPE BIGINT;
				ALTER TABLE request_logs ALTER COLUMN cache_read_input_tokens TYPE BIGINT;
				ALTER TABLE request_logs ALTER COLUMN cache_creation_input_tokens TYPE BIGINT`,
		},
		{
			name: "postgres_widen_subscription_plan_limits_limit_micros",
			sql:  `ALTER TABLE subscription_plan_limits ALTER COLUMN limit_micros TYPE BIGINT`,
		},
		{
			name: "add_subscription_plan_limits_fixed_reset_minute",
			sql:  `ALTER TABLE subscription_plan_limits ADD COLUMN fixed_reset_minute INTEGER`,
		},
		{
			name: "postgres_widen_billing_events_amount_micros",
			sql:  `ALTER TABLE billing_events ALTER COLUMN amount_micros TYPE BIGINT`,
		},
		{
			name: "drop_legacy_users_group_id",
			sql:  `ALTER TABLE users DROP COLUMN group_id`,
		},
		{
			name: "drop_legacy_channels_group_id",
			sql:  `ALTER TABLE channels DROP COLUMN group_id`,
		},
		{
			name: "drop_redundant_indexes",
			sql: `
					DROP INDEX IF EXISTS idx_groups_name;
					DROP INDEX IF EXISTS idx_users_username;
					DROP INDEX IF EXISTS idx_amp_settings_user_id;
					DROP INDEX IF EXISTS idx_api_keys_hash;
					DROP INDEX IF EXISTS idx_api_keys_user_id;
					DROP INDEX IF EXISTS idx_channels_enabled;
					DROP INDEX IF EXISTS idx_user_groups_user;
					DROP INDEX IF EXISTS idx_channel_groups_channel;
					DROP INDEX IF EXISTS idx_channel_models_channel;
					DROP INDEX IF EXISTS idx_model_metadata_pattern;
					DROP INDEX IF EXISTS idx_model_metadata_provider;
					DROP INDEX IF EXISTS idx_request_logs_status;
					DROP INDEX IF EXISTS idx_model_prices_model;
					DROP INDEX IF EXISTS idx_plan_limits_plan;
					DROP INDEX IF EXISTS idx_user_subs_user;
					DROP INDEX IF EXISTS idx_user_subs_plan;
					DROP INDEX IF EXISTS idx_billing_events_user;
					DROP INDEX IF EXISTS idx_billing_events_sub;
					DROP INDEX IF EXISTS idx_billing_events_request;
				`,
		},
		{
			name: "add_channels_simulate_system_prompt",
			sql:  `ALTER TABLE channels ADD COLUMN simulate_system_prompt INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_channels_traditional_chinese",
			sql:  `ALTER TABLE channels ADD COLUMN traditional_chinese INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_channels_copilot_api",
			sql:  `ALTER TABLE channels ADD COLUMN copilot_api INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_channels_simulate_ua",
			sql:  `ALTER TABLE channels ADD COLUMN simulate_ua INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_channels_translator_json",
			sql:  `ALTER TABLE channels ADD COLUMN translator_json TEXT NOT NULL DEFAULT '{}'`,
		},
		{
			name: "add_channels_split_groups_by_source",
			sql:  `ALTER TABLE channels ADD COLUMN split_groups_by_source INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "add_channels_circuit_breaker_columns",
			sql: `ALTER TABLE channels ADD COLUMN circuit_breaker_threshold INTEGER NOT NULL DEFAULT 50;
				  ALTER TABLE channels ADD COLUMN circuit_breaker_open_minutes INTEGER NOT NULL DEFAULT 10;
				  ALTER TABLE channels ADD COLUMN circuit_breaker_half_open_minutes INTEGER NOT NULL DEFAULT 2;
				  ALTER TABLE channels ADD COLUMN circuit_breaker_state TEXT NOT NULL DEFAULT 'closed';
				  ALTER TABLE channels ADD COLUMN circuit_breaker_consecutive_errors INTEGER NOT NULL DEFAULT 0;
				  ALTER TABLE channels ADD COLUMN circuit_breaker_opened_at DATETIME;
				  ALTER TABLE channels ADD COLUMN circuit_breaker_half_open_started_at DATETIME`,
		},
		{
			name: "create_channel_subscription_groups_table",
			sql: `CREATE TABLE IF NOT EXISTS channel_subscription_groups (
				channel_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				PRIMARY KEY (channel_id, group_id),
				FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
				FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
			)`,
		},
		{
			name: "create_channel_subscription_groups_indexes",
			sql:  `CREATE INDEX IF NOT EXISTS idx_channel_subscription_groups_group ON channel_subscription_groups(group_id)`,
		},
		{
			name: "create_channel_usage_groups_table",
			sql: `CREATE TABLE IF NOT EXISTS channel_usage_groups (
				channel_id TEXT NOT NULL,
				group_id TEXT NOT NULL,
				PRIMARY KEY (channel_id, group_id),
				FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
				FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
			)`,
		},
		{
			name: "create_channel_usage_groups_indexes",
			sql:  `CREATE INDEX IF NOT EXISTS idx_channel_usage_groups_group ON channel_usage_groups(group_id)`,
		},
		{
			name: "backfill_channel_dual_groups",
			sql: `INSERT INTO channel_subscription_groups (channel_id, group_id)
				  SELECT channel_id, group_id FROM channel_groups;
				  INSERT INTO channel_usage_groups (channel_id, group_id)
				  SELECT channel_id, group_id FROM channel_groups`,
		},
		{
			name: "create_model_price_context_rules_table",
			sql: `CREATE TABLE IF NOT EXISTS model_price_context_rules (
				id TEXT PRIMARY KEY,
				model TEXT NOT NULL,
				rule_name TEXT NOT NULL,
				min_tokens BIGINT NOT NULL DEFAULT 0,
				max_tokens BIGINT,
				input_micros_per_million BIGINT NOT NULL DEFAULT 0,
				output_micros_per_million BIGINT NOT NULL DEFAULT 0,
				cache_read_micros_per_million BIGINT NOT NULL DEFAULT 0,
				cache_creation_micros_per_million BIGINT NOT NULL DEFAULT 0,
				input_cost_per_token REAL NOT NULL DEFAULT 0,
				output_cost_per_token REAL NOT NULL DEFAULT 0,
				cache_read_input_per_token REAL NOT NULL DEFAULT 0,
				cache_creation_per_token REAL NOT NULL DEFAULT 0,
				sort_order INTEGER NOT NULL DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
		},
		{
			name: "create_model_price_context_rules_indexes",
			sql:  `CREATE INDEX IF NOT EXISTS idx_model_price_context_rules_model ON model_price_context_rules(model, sort_order ASC, created_at ASC)`,
		},
		{
			name: "add_model_price_context_rules_micros_per_million",
			sql: `ALTER TABLE model_price_context_rules ADD COLUMN input_micros_per_million BIGINT NOT NULL DEFAULT 0;
				  ALTER TABLE model_price_context_rules ADD COLUMN output_micros_per_million BIGINT NOT NULL DEFAULT 0;
				  ALTER TABLE model_price_context_rules ADD COLUMN cache_read_micros_per_million BIGINT NOT NULL DEFAULT 0;
				  ALTER TABLE model_price_context_rules ADD COLUMN cache_creation_micros_per_million BIGINT NOT NULL DEFAULT 0;
				  UPDATE model_price_context_rules
				  SET input_micros_per_million = CAST(ROUND(input_cost_per_token * 1000000000000.0) AS INTEGER),
				      output_micros_per_million = CAST(ROUND(output_cost_per_token * 1000000000000.0) AS INTEGER),
				      cache_read_micros_per_million = CAST(ROUND(cache_read_input_per_token * 1000000000000.0) AS INTEGER),
				      cache_creation_micros_per_million = CAST(ROUND(cache_creation_per_token * 1000000000000.0) AS INTEGER)`,
		},
		{
			name: "create_announcements_table",
			sql: `CREATE TABLE IF NOT EXISTS announcements (
				id TEXT PRIMARY KEY,
				title TEXT NOT NULL,
				content TEXT NOT NULL,
				audience TEXT NOT NULL CHECK (audience IN ('authenticated', 'new_user', 'public')),
				pinned INTEGER NOT NULL DEFAULT 0,
				enabled INTEGER NOT NULL DEFAULT 1,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
		},
		{
			name: "create_announcements_indexes",
			sql:  `CREATE INDEX IF NOT EXISTS idx_announcements_audience_enabled ON announcements(audience, enabled, pinned DESC, created_at DESC)`,
		},
		{
			name: "create_announcement_reads_table",
			sql: `CREATE TABLE IF NOT EXISTS announcement_reads (
				announcement_id TEXT NOT NULL,
				user_id TEXT NOT NULL,
				read_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (announcement_id, user_id),
				FOREIGN KEY (announcement_id) REFERENCES announcements(id) ON DELETE CASCADE,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			)`,
		},
		{
			name: "create_announcement_reads_indexes",
			sql:  `CREATE INDEX IF NOT EXISTS idx_announcement_reads_user ON announcement_reads(user_id, read_at DESC)`,
		},
		{
			name: "create_error_rules_table",
			sql: `CREATE TABLE IF NOT EXISTS error_rules (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					request_type TEXT NOT NULL CHECK (request_type IN ('responses', 'chat_completions', 'gemini', 'messages')),
					upstream_status TEXT NOT NULL DEFAULT '*',
					pattern TEXT NOT NULL,
					match_type TEXT NOT NULL CHECK (match_type IN ('contains', 'exact', 'regex')),
					category TEXT NOT NULL DEFAULT '',
					priority INTEGER NOT NULL DEFAULT 0,
					is_enabled INTEGER NOT NULL DEFAULT 1,
					is_default INTEGER NOT NULL DEFAULT 0,
					override_status_code INTEGER,
					override_message TEXT NOT NULL DEFAULT '',
					override_response TEXT NOT NULL DEFAULT '',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)`,
		},
		{
			name: "create_error_rules_indexes",
			sql: `CREATE UNIQUE INDEX IF NOT EXISTS idx_error_rules_request_pattern ON error_rules(request_type, pattern, is_default);
					  CREATE INDEX IF NOT EXISTS idx_error_rules_enabled_priority ON error_rules(is_enabled, priority ASC, id ASC);
					  CREATE INDEX IF NOT EXISTS idx_error_rules_default_pattern ON error_rules(is_default, pattern)`,
		},
		{
			name: "create_request_filters_table",
			sql: `CREATE TABLE IF NOT EXISTS request_filters (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					scope TEXT NOT NULL CHECK (scope IN ('header', 'body')),
					action TEXT NOT NULL CHECK (action IN ('remove', 'set', 'json_path', 'text_replace')),
					match_type TEXT,
					target TEXT NOT NULL DEFAULT '',
					replacement TEXT NOT NULL DEFAULT '',
					priority INTEGER NOT NULL DEFAULT 0,
					is_enabled INTEGER NOT NULL DEFAULT 1,
					binding_type TEXT NOT NULL DEFAULT 'global' CHECK (binding_type IN ('global', 'channels', 'groups')),
					channel_ids_json TEXT NOT NULL DEFAULT '[]',
					group_ids_json TEXT NOT NULL DEFAULT '[]',
					rule_mode TEXT NOT NULL DEFAULT 'simple' CHECK (rule_mode IN ('simple', 'advanced')),
					execution_phase TEXT NOT NULL DEFAULT 'guard' CHECK (execution_phase IN ('guard', 'final')),
					operations_json TEXT NOT NULL DEFAULT '[]',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)`,
		},
		{
			name: "create_request_filters_indexes",
			sql: `CREATE INDEX IF NOT EXISTS idx_request_filters_enabled_priority ON request_filters(is_enabled, priority ASC, id ASC);
					  CREATE INDEX IF NOT EXISTS idx_request_filters_binding_phase ON request_filters(binding_type, execution_phase, priority ASC)`,
		},
		{
			name: "create_status_monitor_tables",
			sql: `CREATE TABLE IF NOT EXISTS status_monitors (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				group_name TEXT NOT NULL DEFAULT '',
				target_type TEXT NOT NULL CHECK (target_type IN ('service_proxy', 'channel_direct', 'custom_http')),
				enabled INTEGER NOT NULL DEFAULT 1,
				sort_order INTEGER NOT NULL DEFAULT 0,
				timeout_ms BIGINT NOT NULL DEFAULT 0,
				degraded_threshold_ms BIGINT NOT NULL DEFAULT 10000,
				request_format TEXT NOT NULL DEFAULT 'responses',
				model TEXT NOT NULL DEFAULT '',
				channel_id TEXT NOT NULL DEFAULT '',
				url TEXT NOT NULL DEFAULT '',
				method TEXT NOT NULL DEFAULT '',
				headers_json TEXT NOT NULL DEFAULT '',
				body_template TEXT NOT NULL DEFAULT '',
				expected_status_codes_json TEXT NOT NULL DEFAULT '[200]',
				expected_substring TEXT NOT NULL DEFAULT '',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE IF NOT EXISTS status_monitor_results (
				id TEXT PRIMARY KEY,
				monitor_id TEXT NOT NULL,
				status TEXT NOT NULL CHECK (status IN ('operational', 'degraded', 'error', 'failed', 'unknown')),
				latency_ms BIGINT NOT NULL DEFAULT 0,
				ttfb_ms BIGINT NOT NULL DEFAULT 0,
				http_status_code INTEGER NOT NULL DEFAULT 0,
				message TEXT NOT NULL DEFAULT '',
				endpoint_label TEXT NOT NULL DEFAULT '',
				checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (monitor_id) REFERENCES status_monitors(id) ON DELETE CASCADE
			)`,
		},
		{
			name: "create_status_monitor_indexes",
			sql: `CREATE INDEX IF NOT EXISTS idx_status_monitors_enabled_sort ON status_monitors(enabled, sort_order ASC, created_at DESC);
				  CREATE INDEX IF NOT EXISTS idx_status_monitors_target_enabled ON status_monitors(target_type, enabled);
				  CREATE INDEX IF NOT EXISTS idx_status_monitor_results_monitor_checked ON status_monitor_results(monitor_id, checked_at DESC);
				  CREATE INDEX IF NOT EXISTS idx_status_monitor_results_checked ON status_monitor_results(checked_at DESC)`,
		},
		{
			name: "create_purchase_products_and_orders",
			sql: `
				CREATE TABLE IF NOT EXISTS purchase_products (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					summary TEXT NOT NULL DEFAULT '',
					subscription_plan_id TEXT NOT NULL,
					duration_days INTEGER NOT NULL CHECK (duration_days > 0),
					price_cny_cent BIGINT NOT NULL CHECK (price_cny_cent > 0),
					is_recommended INTEGER NOT NULL DEFAULT 0,
					sort_order INTEGER NOT NULL DEFAULT 0,
					enabled INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
				);
				CREATE INDEX IF NOT EXISTS idx_purchase_products_enabled_sort ON purchase_products(enabled, is_recommended DESC, sort_order ASC, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_purchase_products_plan ON purchase_products(subscription_plan_id);
					CREATE TABLE IF NOT EXISTS purchase_orders (
						id TEXT PRIMARY KEY,
						order_no TEXT UNIQUE NOT NULL,
						user_id TEXT NOT NULL,
						product_id TEXT NOT NULL,
						subscription_plan_id TEXT NOT NULL,
						duration_days INTEGER NOT NULL CHECK (duration_days > 0),
						amount_cny_cent BIGINT NOT NULL CHECK (amount_cny_cent > 0),
						order_kind TEXT NOT NULL DEFAULT 'subscription' CHECK (order_kind IN ('subscription', 'balance_topup')),
						balance_topup_micros BIGINT NOT NULL DEFAULT 0,
						payment_channel TEXT NOT NULL CHECK (payment_channel IN ('alipay')),
					payment_status TEXT NOT NULL DEFAULT 'pending' CHECK (payment_status IN ('pending', 'paid', 'expired', 'closed', 'failed')),
					fulfillment_status TEXT NOT NULL DEFAULT 'pending' CHECK (fulfillment_status IN ('pending', 'fulfilled', 'failed')),
					alipay_trade_no TEXT NOT NULL DEFAULT '',
					alipay_qr_code TEXT NOT NULL DEFAULT '',
					alipay_qr_url TEXT NOT NULL DEFAULT '',
					expires_at DATETIME,
					paid_at DATETIME,
					fulfilled_at DATETIME,
					failure_reason TEXT NOT NULL DEFAULT '',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					FOREIGN KEY (product_id) REFERENCES purchase_products(id) ON DELETE RESTRICT,
					FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
				);
				CREATE INDEX IF NOT EXISTS idx_purchase_orders_user_created ON purchase_orders(user_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_purchase_orders_payment_created ON purchase_orders(payment_status, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_purchase_orders_fulfillment_created ON purchase_orders(fulfillment_status, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_purchase_orders_product_created ON purchase_orders(product_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_purchase_orders_trade_no ON purchase_orders(alipay_trade_no)
				`,
		},
		{
			name: "add_purchase_orders_order_kind",
			sql:  `ALTER TABLE purchase_orders ADD COLUMN order_kind TEXT NOT NULL DEFAULT 'subscription'`,
		},
		{
			name: "add_purchase_orders_balance_topup_micros",
			sql:  `ALTER TABLE purchase_orders ADD COLUMN balance_topup_micros BIGINT NOT NULL DEFAULT 0`,
		},
		{
			name: "add_subscription_plan_upgrade_fields",
			sql: `
				ALTER TABLE subscription_plans ADD COLUMN upgrade_rank INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE subscription_plans ADD COLUMN upgrade_valuation_cny_cent_per_day BIGINT NOT NULL DEFAULT 0
			`,
		},
		{
			name: "create_subscription_entitlements",
			sql: `
				CREATE TABLE IF NOT EXISTS subscription_entitlements (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					plan_id TEXT NOT NULL,
					source_type TEXT NOT NULL CHECK (source_type IN ('legacy_snapshot', 'purchase', 'redeem', 'admin_assign', 'admin_adjust', 'upgrade')),
					source_ref_id TEXT NOT NULL DEFAULT '',
					valuation_cny_cent_per_day BIGINT NOT NULL DEFAULT 0 CHECK (valuation_cny_cent_per_day >= 0),
					starts_at DATETIME NOT NULL,
					expires_at DATETIME,
					status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'consumed', 'cancelled')),
					consumed_by_purchase_order_no TEXT NOT NULL DEFAULT '',
					consumed_at DATETIME,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
				);
				CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_user_status_expiry ON subscription_entitlements(user_id, status, expires_at);
				CREATE INDEX IF NOT EXISTS idx_subscription_entitlements_plan_status ON subscription_entitlements(plan_id, status)
			`,
		},
		{
			name: "add_purchase_products_group_fields",
			sql: `
				ALTER TABLE purchase_products ADD COLUMN group_name TEXT NOT NULL DEFAULT '';
				ALTER TABLE purchase_products ADD COLUMN group_sort INTEGER NOT NULL DEFAULT 0
			`,
		},
		{
			name: "create_redeem_tables",
			sql: `
				CREATE TABLE IF NOT EXISTS redeem_campaigns (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					code_mode TEXT NOT NULL CHECK (code_mode IN ('single_use', 'shared')),
					subscription_plan_id TEXT,
					subscription_duration_days INTEGER NOT NULL DEFAULT 0 CHECK (subscription_duration_days >= 0),
					balance_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_micros >= 0),
					total_redemptions_limit INTEGER NOT NULL DEFAULT 0 CHECK (total_redemptions_limit >= 0),
					redeemed_count INTEGER NOT NULL DEFAULT 0 CHECK (redeemed_count >= 0),
					per_user_limit INTEGER NOT NULL DEFAULT 1 CHECK (per_user_limit > 0),
					starts_at DATETIME,
					ends_at DATETIME,
					enabled INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
				);
				CREATE INDEX IF NOT EXISTS idx_redeem_campaigns_mode_created ON redeem_campaigns(code_mode, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_redeem_campaigns_enabled_window ON redeem_campaigns(enabled, starts_at, ends_at);

				CREATE TABLE IF NOT EXISTS redeem_code_batches (
					id TEXT PRIMARY KEY,
					campaign_id TEXT NOT NULL,
					name TEXT NOT NULL,
					prefix TEXT NOT NULL DEFAULT '',
					code_count INTEGER NOT NULL CHECK (code_count > 0),
					code_length INTEGER NOT NULL CHECK (code_length >= 6),
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (campaign_id) REFERENCES redeem_campaigns(id) ON DELETE CASCADE
				);
				CREATE INDEX IF NOT EXISTS idx_redeem_code_batches_campaign_created ON redeem_code_batches(campaign_id, created_at DESC);

				CREATE TABLE IF NOT EXISTS redeem_codes (
					id TEXT PRIMARY KEY,
					campaign_id TEXT NOT NULL,
					batch_id TEXT,
					code_value TEXT NOT NULL UNIQUE,
					code_hash TEXT NOT NULL UNIQUE,
					code_mask TEXT NOT NULL,
					status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'consumed')),
					max_redemptions INTEGER NOT NULL DEFAULT 1 CHECK (max_redemptions >= 0),
					redeemed_count INTEGER NOT NULL DEFAULT 0 CHECK (redeemed_count >= 0),
					last_redeemed_at DATETIME,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (campaign_id) REFERENCES redeem_campaigns(id) ON DELETE CASCADE,
					FOREIGN KEY (batch_id) REFERENCES redeem_code_batches(id) ON DELETE CASCADE
				);
				CREATE INDEX IF NOT EXISTS idx_redeem_codes_campaign_status_created ON redeem_codes(campaign_id, status, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_redeem_codes_batch_created ON redeem_codes(batch_id, created_at DESC);

				CREATE TABLE IF NOT EXISTS redeem_user_counters (
					code_id TEXT NOT NULL,
					user_id TEXT NOT NULL,
					redeemed_count INTEGER NOT NULL DEFAULT 0 CHECK (redeemed_count >= 0),
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (code_id, user_id),
					FOREIGN KEY (code_id) REFERENCES redeem_codes(id) ON DELETE CASCADE,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				);
				CREATE INDEX IF NOT EXISTS idx_redeem_user_counters_user_updated ON redeem_user_counters(user_id, updated_at DESC);

				CREATE TABLE IF NOT EXISTS redeem_redemptions (
					id TEXT PRIMARY KEY,
					campaign_id TEXT,
					code_id TEXT,
					user_id TEXT NOT NULL,
					username TEXT NOT NULL DEFAULT '',
					code_input TEXT NOT NULL DEFAULT '',
					code_mask TEXT NOT NULL DEFAULT '',
					subscription_plan_id TEXT,
					subscription_duration_days INTEGER NOT NULL DEFAULT 0 CHECK (subscription_duration_days >= 0),
					balance_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_micros >= 0),
					status TEXT NOT NULL CHECK (status IN ('success', 'rejected')),
					failure_reason TEXT NOT NULL DEFAULT '',
					granted_subscription_id TEXT,
					granted_expires_at DATETIME,
					balance_after_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_after_micros >= 0),
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (campaign_id) REFERENCES redeem_campaigns(id) ON DELETE SET NULL,
					FOREIGN KEY (code_id) REFERENCES redeem_codes(id) ON DELETE SET NULL,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT,
					FOREIGN KEY (granted_subscription_id) REFERENCES user_subscriptions(id) ON DELETE SET NULL
				);
				CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_user_created ON redeem_redemptions(user_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_campaign_created ON redeem_redemptions(campaign_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_code_created ON redeem_redemptions(code_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_status_created ON redeem_redemptions(status, created_at DESC)
			`,
		},
		{
			name: "rebuild_purchase_orders_for_delivery_mode",
			sql:  ``,
		},
		{
			name: "rebuild_redeem_codes_for_free_codes",
			sql:  ``,
		},
		{
			name: "rebuild_redeem_reward_plan_nullable",
			sql:  ``,
		},
		{
			name: "rebuild_purchase_orders_for_purchase_admin_extensions",
			sql:  ``,
		},
	}

	for _, m := range migrations {
		applied, err := isMigrationApplied(m.name)
		if err != nil {
			return fmt.Errorf("check migration '%s' failed: %w", m.name, err)
		}
		if applied {
			continue
		}

		err = execStatements(db, adaptMigrationSQL(m.name, m.sql))
		if err != nil && !shouldIgnoreMigrationError(m.name, err) {
			return fmt.Errorf("migration '%s' failed: %w", m.name, err)
		}

		if err := markMigrationApplied(m.name); err != nil {
			return fmt.Errorf("mark migration '%s' applied failed: %w", m.name, err)
		}
	}

	if dbType == DBTypeSQLite {
		if err := migrateTimestampsToUTC(db); err != nil {
			return fmt.Errorf("migrate timestamps to UTC failed: %w", err)
		}
	}

	return nil
}

func ensureCriticalSchema() error {
	if err := ensureRequestLogsSessionSchema(); err != nil {
		return err
	}
	if err := ensureAPIKeyCircuitBreakerSchema(); err != nil {
		return err
	}
	if err := ensureUserBootstrapSchema(); err != nil {
		return err
	}
	if err := ensureErrorRuleSchema(); err != nil {
		return err
	}
	if err := ensureSubscriptionRuntimeSchema(); err != nil {
		return err
	}
	if err := ensurePurchaseSchema(); err != nil {
		return err
	}
	if err := ensureInviteCouponSchema(); err != nil {
		return err
	}
	return nil
}

func ensureAPIKeyCircuitBreakerSchema() error {
	if err := ensureColumnWithDefault("user_api_keys", "circuit_breaker_threshold", "INTEGER NOT NULL DEFAULT 50"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("user_api_keys", "circuit_breaker_open_minutes", "INTEGER NOT NULL DEFAULT 10"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("user_api_keys", "circuit_breaker_half_open_minutes", "INTEGER NOT NULL DEFAULT 2"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("user_api_keys", "circuit_breaker_state", "TEXT NOT NULL DEFAULT 'closed'"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("user_api_keys", "circuit_breaker_consecutive_errors", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("user_api_keys", "circuit_breaker_opened_at", "DATETIME"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("user_api_keys", "circuit_breaker_half_open_started_at", "DATETIME"); err != nil {
		return err
	}
	return nil
}

func ensureRequestLogsSessionSchema() error {
	exists, err := columnExists("request_logs", "session_id")
	if err != nil {
		return fmt.Errorf("check request_logs.session_id failed: %w", err)
	}
	if !exists {
		if _, err := db.Exec(`ALTER TABLE request_logs ADD COLUMN session_id TEXT`); err != nil && !shouldIgnoreMigrationError("add_request_logs_session_id", err) {
			return fmt.Errorf("add request_logs.session_id failed: %w", err)
		}
	}

	hasIndex, err := indexExists("request_logs", "idx_request_logs_session_time")
	if err != nil {
		return fmt.Errorf("check idx_request_logs_session_time failed: %w", err)
	}
	if !hasIndex {
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_request_logs_session_time ON request_logs(session_id, created_at DESC)`); err != nil {
			return fmt.Errorf("create idx_request_logs_session_time failed: %w", err)
		}
	}

	return nil
}

func ensureUserBootstrapSchema() error {
	if err := ensureColumnWithDefault("users", "must_change_password", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("users", "must_change_username", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("users", "legacy_source", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("users", "legacy_ref_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

func ensureErrorRuleSchema() error {
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_error_rules_request_pattern`); err != nil {
		return fmt.Errorf("drop idx_error_rules_request_pattern failed: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_error_rules_request_pattern ON error_rules(request_type, pattern, is_default)`); err != nil {
		return fmt.Errorf("create idx_error_rules_request_pattern failed: %w", err)
	}
	return nil
}

func ensureSubscriptionRuntimeSchema() error {
	if err := ensureColumnWithDefault("subscription_plans", "legacy_source", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("subscription_plans", "legacy_ref_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("billing_daily_reset_records", "legacy_source", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("billing_daily_reset_records", "legacy_ref_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS subscription_recharge_history (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			user_subscription_id TEXT NOT NULL,
			plan_id TEXT NOT NULL DEFAULT '',
			mode TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT '',
			source_type TEXT NOT NULL DEFAULT '',
			source_ref_id TEXT NOT NULL DEFAULT '',
			source_daily_limit_micros BIGINT,
			target_daily_limit_before_micros BIGINT,
			peak_daily_limit_micros BIGINT,
			target_expires_at_before DATETIME,
			target_expires_at_after DATETIME,
			preview_json TEXT NOT NULL DEFAULT '',
			confirmed_at DATETIME,
			applied_at DATETIME,
			legacy_source TEXT NOT NULL DEFAULT '',
			legacy_ref_id TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE CASCADE,
			FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_subscription_recharge_history_user_created ON subscription_recharge_history(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_subscription_recharge_history_sub_created ON subscription_recharge_history(user_subscription_id, created_at DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_recharge_history_legacy_ref ON subscription_recharge_history(legacy_source, legacy_ref_id) WHERE legacy_source <> '' AND legacy_ref_id <> ''`,
		`CREATE TABLE IF NOT EXISTS subscription_timeline_phases (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			user_subscription_id TEXT NOT NULL,
			plan_id TEXT NOT NULL,
			phase_type TEXT NOT NULL CHECK (phase_type IN ('boost', 'restore', 'converted_extension', 'overwrite', 'extend_duration')),
			status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'active', 'completed', 'superseded', 'cancelled')),
			source_type TEXT NOT NULL DEFAULT '',
			source_ref_id TEXT NOT NULL DEFAULT '',
			daily_limit_micros BIGINT,
			weekly_limit_micros BIGINT,
			monthly_limit_micros BIGINT,
			rolling_5h_limit_micros BIGINT,
			total_limit_micros BIGINT,
			fixed_reset_minute INTEGER CHECK (fixed_reset_minute >= 0 AND fixed_reset_minute < 1440),
			starts_at DATETIME NOT NULL,
			ends_at DATETIME NOT NULL,
			final_expires_at DATETIME,
			preview_json TEXT NOT NULL DEFAULT '',
			applied_at DATETIME,
			legacy_source TEXT NOT NULL DEFAULT '',
			legacy_ref_id TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (user_subscription_id) REFERENCES user_subscriptions(id) ON DELETE CASCADE,
			FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_subscription_timeline_phases_sub_status_start ON subscription_timeline_phases(user_subscription_id, status, starts_at)`,
		`CREATE INDEX IF NOT EXISTS idx_subscription_timeline_phases_source_ref ON subscription_timeline_phases(source_type, source_ref_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_timeline_phases_legacy_ref ON subscription_timeline_phases(legacy_source, legacy_ref_id) WHERE legacy_source <> '' AND legacy_ref_id <> ''`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(adaptCriticalSchemaSQL(statement)); err != nil {
			return err
		}
	}
	return nil
}

func ensurePurchaseSchema() error {
	if err := ensureColumnWithDefault("purchase_orders", "manual_settlement_done", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("purchase_products", "product_kind", "TEXT NOT NULL DEFAULT 'subscription'"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("purchase_products", "action_snapshot_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("purchase_products", "legacy_source", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("purchase_products", "legacy_ref_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("purchase_orders", "action_snapshot_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("purchase_orders", "timeline_preview_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("purchase_orders", "legacy_source", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("purchase_orders", "legacy_ref_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "reward_snapshot_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "legacy_source", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "legacy_ref_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "source_type", "TEXT NOT NULL DEFAULT 'campaign'"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "source_ref_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "subscription_duration_days", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "balance_micros", "BIGINT NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "per_user_limit", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "subscription_plan_id", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "starts_at", "DATETIME"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_codes", "ends_at", "DATETIME"); err != nil {
		return err
	}
	if err := ensureColumnWithDefault("redeem_redemptions", "reward_snapshot_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if dbType == DBTypePostgres {
		if _, err := db.Exec(`ALTER TABLE redeem_codes ALTER COLUMN campaign_id DROP NOT NULL`); err != nil && !shouldIgnoreMigrationError("alter_redeem_codes_campaign_id_nullable", err) {
			return fmt.Errorf("alter redeem_codes.campaign_id nullable failed: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_purchase_orders_manual_settlement_done ON purchase_orders(manual_settlement_done, created_at DESC)`); err != nil {
		return fmt.Errorf("create idx_purchase_orders_manual_settlement_done failed: %w", err)
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS purchase_webhook_targets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			target_url TEXT NOT NULL,
			body_template TEXT NOT NULL DEFAULT '',
			headers_template TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_purchase_webhook_targets_enabled ON purchase_webhook_targets(enabled, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS purchase_webhook_events (
			id TEXT PRIMARY KEY,
			order_id TEXT NOT NULL,
			order_no TEXT NOT NULL,
			target_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			response_status_code INTEGER NOT NULL DEFAULT 0,
			claim_token TEXT NOT NULL DEFAULT '',
			claimed_at DATETIME,
			claim_until DATETIME,
			next_attempt_at DATETIME,
			last_attempt_at DATETIME,
			delivered_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (order_id) REFERENCES purchase_orders(id) ON DELETE CASCADE,
			FOREIGN KEY (target_id) REFERENCES purchase_webhook_targets(id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_purchase_webhook_events_order_target ON purchase_webhook_events(order_id, target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_purchase_webhook_events_due ON purchase_webhook_events(status, next_attempt_at, created_at)`,
		`CREATE TABLE IF NOT EXISTS purchase_order_payment_status_history (
			id TEXT PRIMARY KEY,
			order_id TEXT NOT NULL,
			order_no TEXT NOT NULL,
			from_status TEXT NOT NULL,
			to_status TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (order_id) REFERENCES purchase_orders(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_purchase_order_payment_status_history_order_created ON purchase_order_payment_status_history(order_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS purchase_manual_settlement_batches (
			id TEXT PRIMARY KEY,
			batch_no TEXT NOT NULL UNIQUE,
			mode TEXT NOT NULL,
			debug_settlement INTEGER NOT NULL DEFAULT 0,
			created_by TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			order_count INTEGER NOT NULL DEFAULT 0,
			total_amount_cny_cent BIGINT NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS purchase_manual_settlement_batch_items (
			id TEXT PRIMARY KEY,
			batch_id TEXT NOT NULL,
			order_id TEXT NOT NULL,
			order_no TEXT NOT NULL,
			amount_cny_cent BIGINT NOT NULL DEFAULT 0,
			payment_status TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (batch_id) REFERENCES purchase_manual_settlement_batches(id) ON DELETE CASCADE,
			FOREIGN KEY (order_id) REFERENCES purchase_orders(id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_purchase_manual_settlement_item_order_unique ON purchase_manual_settlement_batch_items(order_id)`,
		`CREATE INDEX IF NOT EXISTS idx_purchase_manual_settlement_item_batch ON purchase_manual_settlement_batch_items(batch_id, created_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(adaptCriticalSchemaSQL(statement)); err != nil {
			return err
		}
	}
	if err := ensureColumnWithDefault("purchase_manual_settlement_batches", "debug_settlement", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return nil
}

func ensureInviteCouponSchema() error {
	if err := ensureColumnWithDefault("users", "invite_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_invite_code_unique ON users(invite_code) WHERE invite_code <> ''`); err != nil {
		return fmt.Errorf("create idx_users_invite_code_unique failed: %w", err)
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS invite_relationships (
			id TEXT PRIMARY KEY,
			inviter_user_id TEXT NOT NULL,
			inviter_code TEXT NOT NULL,
			invitee_user_id TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'pending',
			first_paid_order_id TEXT NOT NULL DEFAULT '',
			first_paid_order_no TEXT NOT NULL DEFAULT '',
			first_paid_order_kind TEXT NOT NULL DEFAULT '',
			first_paid_amount_cny_cent BIGINT NOT NULL DEFAULT 0,
			first_paid_at DATETIME,
			rewarded_at DATETIME,
			last_reversed_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (inviter_user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (invitee_user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_invite_relationships_inviter_status ON invite_relationships(inviter_user_id, status, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_invite_relationships_invitee ON invite_relationships(invitee_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_invite_relationships_first_paid_order_no ON invite_relationships(first_paid_order_no)`,
		`CREATE TABLE IF NOT EXISTS invite_reward_events (
			id TEXT PRIMARY KEY,
			relation_id TEXT NOT NULL,
			beneficiary_user_id TEXT NOT NULL,
			beneficiary_role TEXT NOT NULL,
			order_id TEXT NOT NULL DEFAULT '',
			order_no TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			amount_micros BIGINT NOT NULL DEFAULT 0,
			order_paid_amount_cny_cent BIGINT NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (relation_id) REFERENCES invite_relationships(id) ON DELETE CASCADE,
			FOREIGN KEY (beneficiary_user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_invite_reward_events_unique ON invite_reward_events(relation_id, beneficiary_role, status, order_no)`,
		`CREATE INDEX IF NOT EXISTS idx_invite_reward_events_user_created ON invite_reward_events(beneficiary_user_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS coupon_campaigns (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			code_mode TEXT NOT NULL,
			shared_code TEXT NOT NULL DEFAULT '',
			discount_type TEXT NOT NULL,
			fixed_discount_cny_cent BIGINT NOT NULL DEFAULT 0,
			percent_off_bps INTEGER NOT NULL DEFAULT 0,
			max_discount_cny_cent BIGINT NOT NULL DEFAULT 0,
			total_usage_limit INTEGER NOT NULL DEFAULT 0,
			used_count INTEGER NOT NULL DEFAULT 0,
			per_user_limit INTEGER NOT NULL DEFAULT 1,
			starts_at DATETIME,
			ends_at DATETIME,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coupon_campaigns_enabled_window ON coupon_campaigns(enabled, starts_at, ends_at)`,
		`CREATE TABLE IF NOT EXISTS coupon_code_batches (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL,
			name TEXT NOT NULL,
			prefix TEXT NOT NULL DEFAULT '',
			code_count INTEGER NOT NULL DEFAULT 0,
			code_length INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (campaign_id) REFERENCES coupon_campaigns(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coupon_code_batches_campaign_created ON coupon_code_batches(campaign_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS coupon_codes (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL,
			batch_id TEXT,
			code_mode TEXT NOT NULL,
			code_value TEXT NOT NULL UNIQUE,
			code_hash TEXT NOT NULL UNIQUE,
			code_mask TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			max_usages INTEGER NOT NULL DEFAULT 1,
			used_count INTEGER NOT NULL DEFAULT 0,
			last_used_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (campaign_id) REFERENCES coupon_campaigns(id) ON DELETE CASCADE,
			FOREIGN KEY (batch_id) REFERENCES coupon_code_batches(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coupon_codes_campaign_status_created ON coupon_codes(campaign_id, status, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_coupon_codes_batch_created ON coupon_codes(batch_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS coupon_usages (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL,
			code_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			order_id TEXT NOT NULL,
			order_no TEXT NOT NULL,
			status TEXT NOT NULL,
			discount_cny_cent BIGINT NOT NULL DEFAULT 0,
			final_amount_cny_cent BIGINT NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (campaign_id) REFERENCES coupon_campaigns(id) ON DELETE CASCADE,
			FOREIGN KEY (code_id) REFERENCES coupon_codes(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (order_id) REFERENCES purchase_orders(id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_coupon_usages_order_unique ON coupon_usages(order_id)`,
		`CREATE INDEX IF NOT EXISTS idx_coupon_usages_user_campaign_status ON coupon_usages(user_id, campaign_id, status, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_coupon_usages_campaign_status ON coupon_usages(campaign_id, status, created_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(adaptCriticalSchemaSQL(statement)); err != nil {
			return err
		}
	}

	orderColumns := []struct {
		name       string
		definition string
	}{
		{name: "original_amount_cny_cent", definition: "BIGINT NOT NULL DEFAULT 0"},
		{name: "discount_cny_cent", definition: "BIGINT NOT NULL DEFAULT 0"},
		{name: "coupon_campaign_id", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "coupon_campaign_name", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "coupon_code_id", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "coupon_code_value", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "coupon_discount_type", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "coupon_percent_off_bps", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "coupon_fixed_discount_cny_cent", definition: "BIGINT NOT NULL DEFAULT 0"},
		{name: "coupon_max_discount_cny_cent", definition: "BIGINT NOT NULL DEFAULT 0"},
	}
	for _, column := range orderColumns {
		if err := ensureColumnWithDefault("purchase_orders", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func adaptCriticalSchemaSQL(sqlText string) string {
	if dbType == DBTypePostgres {
		return strings.ReplaceAll(sqlText, "DATETIME", "TIMESTAMPTZ")
	}
	return sqlText
}

func ensureColumnWithDefault(tableName, columnName, definition string) error {
	exists, err := columnExists(tableName, columnName)
	if err != nil {
		return fmt.Errorf("check %s.%s failed: %w", tableName, columnName, err)
	}
	if exists {
		return nil
	}
	adaptedDefinition := definition
	if dbType == DBTypePostgres {
		adaptedDefinition = strings.ReplaceAll(adaptedDefinition, "DATETIME", "TIMESTAMPTZ")
	}
	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, adaptedDefinition)); err != nil && !shouldIgnoreMigrationError("ensure_column_"+tableName+"_"+columnName, err) {
		return fmt.Errorf("add %s.%s failed: %w", tableName, columnName, err)
	}
	return nil
}

func adaptMigrationSQL(name string, sqlText string) string {
	adapted := sqlText
	if dbType == DBTypePostgres {
		adapted = strings.ReplaceAll(adapted, "DATETIME", "TIMESTAMPTZ")
	}

	switch name {
	case "add_request_detail_retention_default":
		adapted = `INSERT INTO system_config (key, value, updated_at) VALUES ('request_detail_retention_days', '30', CURRENT_TIMESTAMP) ON CONFLICT (key) DO NOTHING`
	case "drop_api_key_allowed_providers_json":
		if dbType == DBTypePostgres {
			adapted = `ALTER TABLE user_api_keys DROP COLUMN IF EXISTS allowed_providers_json`
		} else {
			adapted = `
				PRAGMA foreign_keys = OFF;
				ALTER TABLE user_api_keys RENAME TO user_api_keys_old;
				CREATE TABLE user_api_keys (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					name TEXT NOT NULL,
					key_hash TEXT UNIQUE NOT NULL,
					api_key TEXT NOT NULL DEFAULT '',
					prefix TEXT NOT NULL,
					last_used_at DATETIME,
					expires_at DATETIME,
					revoked_at DATETIME,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				);
				INSERT INTO user_api_keys (
					id, user_id, name, key_hash, api_key, prefix, last_used_at, expires_at, revoked_at, created_at
				)
				SELECT
					id, user_id, name, key_hash, api_key, prefix, last_used_at, expires_at, revoked_at, created_at
				FROM user_api_keys_old;
				DROP TABLE user_api_keys_old;
				CREATE INDEX IF NOT EXISTS idx_api_keys_user_created ON user_api_keys(user_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_api_keys_user_active ON user_api_keys(user_id, revoked_at);
				PRAGMA foreign_keys = ON
			`
		}
	case "add_user_balance_micros":
		if dbType == DBTypePostgres {
			adapted = `ALTER TABLE users ADD COLUMN balance_micros BIGINT NOT NULL DEFAULT 0`
		}
	case "add_request_logs_pricing_rule_name":
		if dbType == DBTypePostgres {
			adapted = `ALTER TABLE request_logs ADD COLUMN pricing_rule_name TEXT`
		}
	case "add_request_logs_cost_micros":
		if dbType == DBTypePostgres {
			adapted = `ALTER TABLE request_logs ADD COLUMN cost_micros BIGINT`
		}
	case "add_request_logs_charged_subscription_micros":
		if dbType == DBTypePostgres {
			adapted = `ALTER TABLE request_logs ADD COLUMN charged_subscription_micros BIGINT NOT NULL DEFAULT 0`
		}
	case "add_request_logs_charged_balance_micros":
		if dbType == DBTypePostgres {
			adapted = `ALTER TABLE request_logs ADD COLUMN charged_balance_micros BIGINT NOT NULL DEFAULT 0`
		}
	case "add_request_log_details_archive_translated_request_headers":
		if dbType != DBTypePostgres {
			adapted = ""
		}
	case "rebuild_purchase_orders_for_delivery_mode":
		adapted = `
			ALTER TABLE purchase_orders RENAME TO purchase_orders_old;
			CREATE TABLE purchase_orders (
				id TEXT PRIMARY KEY,
				order_no TEXT UNIQUE NOT NULL,
				user_id TEXT NOT NULL,
				product_id TEXT NOT NULL,
				subscription_plan_id TEXT NOT NULL,
				duration_days INTEGER NOT NULL CHECK (duration_days > 0),
				amount_cny_cent BIGINT NOT NULL CHECK (amount_cny_cent >= 0),
				order_kind TEXT NOT NULL DEFAULT 'subscription' CHECK (order_kind IN ('subscription', 'balance_topup')),
				delivery_mode TEXT NOT NULL DEFAULT 'account' CHECK (delivery_mode IN ('account', 'redeem_code')),
				balance_topup_micros BIGINT NOT NULL DEFAULT 0,
				payment_channel TEXT NOT NULL CHECK (payment_channel IN ('alipay')),
				generated_redeem_code_id TEXT NOT NULL DEFAULT '',
				upgrade_source_plan_id TEXT NOT NULL DEFAULT '',
				upgrade_source_expires_at DATETIME,
				upgrade_credit_cny_cent BIGINT NOT NULL DEFAULT 0,
				upgrade_locked_target_seconds BIGINT NOT NULL DEFAULT 0,
				upgrade_state_token TEXT NOT NULL DEFAULT '',
				payment_status TEXT NOT NULL DEFAULT 'pending' CHECK (payment_status IN ('pending', 'paid', 'expired', 'closed', 'failed')),
				fulfillment_status TEXT NOT NULL DEFAULT 'pending' CHECK (fulfillment_status IN ('pending', 'fulfilled', 'failed')),
				alipay_trade_no TEXT NOT NULL DEFAULT '',
				alipay_qr_code TEXT NOT NULL DEFAULT '',
				alipay_qr_url TEXT NOT NULL DEFAULT '',
				expires_at DATETIME,
				paid_at DATETIME,
				fulfilled_at DATETIME,
				failure_reason TEXT NOT NULL DEFAULT '',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
				FOREIGN KEY (product_id) REFERENCES purchase_products(id) ON DELETE RESTRICT,
				FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
			);
			INSERT INTO purchase_orders (
				id, order_no, user_id, product_id, subscription_plan_id, duration_days, amount_cny_cent,
				order_kind, delivery_mode, balance_topup_micros, payment_channel, generated_redeem_code_id,
				upgrade_source_plan_id, upgrade_source_expires_at, upgrade_credit_cny_cent, upgrade_locked_target_seconds, upgrade_state_token,
				payment_status, fulfillment_status, alipay_trade_no, alipay_qr_code, alipay_qr_url,
				expires_at, paid_at, fulfilled_at, failure_reason, created_at, updated_at
			)
			SELECT
				id, order_no, user_id, product_id, subscription_plan_id, duration_days, amount_cny_cent,
				order_kind, 'account', balance_topup_micros, payment_channel, '',
				'', NULL, 0, 0, '',
				payment_status, fulfillment_status, alipay_trade_no, alipay_qr_code, alipay_qr_url,
				expires_at, paid_at, fulfilled_at, failure_reason, created_at, updated_at
			FROM purchase_orders_old;
			DROP TABLE purchase_orders_old;
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_user_created ON purchase_orders(user_id, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_payment_created ON purchase_orders(payment_status, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_fulfillment_created ON purchase_orders(fulfillment_status, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_product_created ON purchase_orders(product_id, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_trade_no ON purchase_orders(alipay_trade_no)
		`
	case "rebuild_redeem_codes_for_free_codes":
		if dbType == DBTypePostgres {
			adapted = `
				ALTER TABLE redeem_codes ALTER COLUMN campaign_id DROP NOT NULL;
				ALTER TABLE redeem_codes ADD COLUMN source_type TEXT NOT NULL DEFAULT 'campaign';
				ALTER TABLE redeem_codes ADD COLUMN source_ref_id TEXT NOT NULL DEFAULT '';
				ALTER TABLE redeem_codes ADD COLUMN subscription_plan_id TEXT;
				ALTER TABLE redeem_codes ADD COLUMN subscription_duration_days INTEGER NOT NULL DEFAULT 0;
				ALTER TABLE redeem_codes ADD COLUMN balance_micros BIGINT NOT NULL DEFAULT 0;
				ALTER TABLE redeem_codes ADD COLUMN per_user_limit INTEGER NOT NULL DEFAULT 1;
				ALTER TABLE redeem_codes ADD COLUMN starts_at TIMESTAMPTZ;
				ALTER TABLE redeem_codes ADD COLUMN ends_at TIMESTAMPTZ;
				ALTER TABLE redeem_codes ADD CONSTRAINT redeem_codes_source_type_check CHECK (source_type IN ('campaign', 'free', 'purchase_order'))
			`
		} else {
			adapted = `
				PRAGMA foreign_keys = OFF;
				CREATE TABLE redeem_codes_new (
					id TEXT PRIMARY KEY,
					campaign_id TEXT,
					batch_id TEXT,
					source_type TEXT NOT NULL DEFAULT 'campaign' CHECK (source_type IN ('campaign', 'free', 'purchase_order')),
					source_ref_id TEXT NOT NULL DEFAULT '',
					code_value TEXT NOT NULL UNIQUE,
					code_hash TEXT NOT NULL UNIQUE,
					code_mask TEXT NOT NULL,
					subscription_plan_id TEXT,
					subscription_duration_days INTEGER NOT NULL DEFAULT 0 CHECK (subscription_duration_days >= 0),
					balance_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_micros >= 0),
					per_user_limit INTEGER NOT NULL DEFAULT 1 CHECK (per_user_limit > 0),
					starts_at DATETIME,
					ends_at DATETIME,
					status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'consumed')),
					max_redemptions INTEGER NOT NULL DEFAULT 1 CHECK (max_redemptions >= 0),
					redeemed_count INTEGER NOT NULL DEFAULT 0 CHECK (redeemed_count >= 0),
					last_redeemed_at DATETIME,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (campaign_id) REFERENCES redeem_campaigns(id) ON DELETE SET NULL,
					FOREIGN KEY (batch_id) REFERENCES redeem_code_batches(id) ON DELETE CASCADE,
					FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
				);
				INSERT INTO redeem_codes_new (
					id, campaign_id, batch_id, source_type, source_ref_id, code_value, code_hash, code_mask,
					subscription_plan_id, subscription_duration_days, balance_micros, per_user_limit, starts_at, ends_at,
					status, max_redemptions, redeemed_count, last_redeemed_at, created_at, updated_at
				)
				SELECT
					id, campaign_id, batch_id, 'campaign', '', code_value, code_hash, code_mask,
					NULL, 0, 0, 1, NULL, NULL,
					status, max_redemptions, redeemed_count, last_redeemed_at, created_at, updated_at
				FROM redeem_codes;
				DROP TABLE redeem_codes;
				ALTER TABLE redeem_codes_new RENAME TO redeem_codes;
				CREATE INDEX IF NOT EXISTS idx_redeem_codes_campaign_status_created ON redeem_codes(campaign_id, status, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_redeem_codes_batch_created ON redeem_codes(batch_id, created_at DESC);
				PRAGMA foreign_keys = ON
			`
		}
	case "rebuild_redeem_reward_plan_nullable":
		if dbType == DBTypePostgres {
			adapted = `
				UPDATE redeem_campaigns SET subscription_plan_id = NULL WHERE BTRIM(subscription_plan_id) = '';
				ALTER TABLE redeem_campaigns ALTER COLUMN subscription_plan_id DROP DEFAULT;
				ALTER TABLE redeem_campaigns ALTER COLUMN subscription_plan_id DROP NOT NULL;
				UPDATE redeem_redemptions SET subscription_plan_id = NULL WHERE BTRIM(subscription_plan_id) = '';
				ALTER TABLE redeem_redemptions ALTER COLUMN subscription_plan_id DROP DEFAULT;
				ALTER TABLE redeem_redemptions ALTER COLUMN subscription_plan_id DROP NOT NULL;
				UPDATE redeem_redemptions SET granted_subscription_id = NULL WHERE BTRIM(granted_subscription_id) = '';
				ALTER TABLE redeem_redemptions ALTER COLUMN granted_subscription_id DROP DEFAULT;
				ALTER TABLE redeem_redemptions ALTER COLUMN granted_subscription_id DROP NOT NULL
			`
		} else {
			adapted = `
				PRAGMA foreign_keys = OFF;
				CREATE TABLE redeem_campaigns_new_reward_plan (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					code_mode TEXT NOT NULL CHECK (code_mode IN ('single_use', 'shared')),
					subscription_plan_id TEXT,
					subscription_duration_days INTEGER NOT NULL DEFAULT 0 CHECK (subscription_duration_days >= 0),
					balance_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_micros >= 0),
					total_redemptions_limit INTEGER NOT NULL DEFAULT 0 CHECK (total_redemptions_limit >= 0),
					redeemed_count INTEGER NOT NULL DEFAULT 0 CHECK (redeemed_count >= 0),
					per_user_limit INTEGER NOT NULL DEFAULT 1 CHECK (per_user_limit > 0),
					starts_at DATETIME,
					ends_at DATETIME,
					enabled INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
				);
				INSERT INTO redeem_campaigns_new_reward_plan (
					id, name, description, code_mode, subscription_plan_id, subscription_duration_days, balance_micros,
					total_redemptions_limit, redeemed_count, per_user_limit, starts_at, ends_at, enabled, created_at, updated_at
				)
				SELECT
					id, name, description, code_mode, NULLIF(TRIM(subscription_plan_id), ''), subscription_duration_days, balance_micros,
					total_redemptions_limit, redeemed_count, per_user_limit, starts_at, ends_at, enabled, created_at, updated_at
				FROM redeem_campaigns;
				DROP TABLE redeem_campaigns;
				ALTER TABLE redeem_campaigns_new_reward_plan RENAME TO redeem_campaigns;
				CREATE INDEX IF NOT EXISTS idx_redeem_campaigns_mode_created ON redeem_campaigns(code_mode, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_redeem_campaigns_enabled_window ON redeem_campaigns(enabled, starts_at, ends_at);

				CREATE TABLE redeem_redemptions_new_reward_plan (
					id TEXT PRIMARY KEY,
					campaign_id TEXT,
					code_id TEXT,
					user_id TEXT NOT NULL,
					username TEXT NOT NULL DEFAULT '',
					code_input TEXT NOT NULL DEFAULT '',
					code_mask TEXT NOT NULL DEFAULT '',
					subscription_plan_id TEXT,
					subscription_duration_days INTEGER NOT NULL DEFAULT 0 CHECK (subscription_duration_days >= 0),
					balance_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_micros >= 0),
					reward_snapshot_json TEXT NOT NULL DEFAULT '',
					status TEXT NOT NULL CHECK (status IN ('success', 'rejected')),
					failure_reason TEXT NOT NULL DEFAULT '',
					granted_subscription_id TEXT,
					granted_expires_at DATETIME,
					balance_after_micros BIGINT NOT NULL DEFAULT 0 CHECK (balance_after_micros >= 0),
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (campaign_id) REFERENCES redeem_campaigns(id) ON DELETE SET NULL,
					FOREIGN KEY (code_id) REFERENCES redeem_codes(id) ON DELETE SET NULL,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT,
					FOREIGN KEY (granted_subscription_id) REFERENCES user_subscriptions(id) ON DELETE SET NULL
				);
				INSERT INTO redeem_redemptions_new_reward_plan (
					id, campaign_id, code_id, user_id, username, code_input, code_mask,
					subscription_plan_id, subscription_duration_days, balance_micros, reward_snapshot_json, status, failure_reason,
					granted_subscription_id, granted_expires_at, balance_after_micros, created_at
				)
				SELECT
					id, campaign_id, code_id, user_id, username, code_input, code_mask,
					NULLIF(TRIM(subscription_plan_id), ''), subscription_duration_days, balance_micros, reward_snapshot_json, status, failure_reason,
					NULLIF(TRIM(granted_subscription_id), ''), granted_expires_at, balance_after_micros, created_at
				FROM redeem_redemptions;
				DROP TABLE redeem_redemptions;
				ALTER TABLE redeem_redemptions_new_reward_plan RENAME TO redeem_redemptions;
				CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_user_created ON redeem_redemptions(user_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_campaign_created ON redeem_redemptions(campaign_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_code_created ON redeem_redemptions(code_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_redeem_redemptions_status_created ON redeem_redemptions(status, created_at DESC);
				PRAGMA foreign_keys = ON
			`
		}
	case "rebuild_purchase_orders_for_purchase_admin_extensions":
		adapted = `
			ALTER TABLE purchase_orders RENAME TO purchase_orders_old2;
			CREATE TABLE purchase_orders (
				id TEXT PRIMARY KEY,
				order_no TEXT UNIQUE NOT NULL,
				user_id TEXT NOT NULL,
				product_id TEXT NOT NULL,
				subscription_plan_id TEXT NOT NULL,
				duration_days INTEGER NOT NULL CHECK (duration_days > 0),
				amount_cny_cent BIGINT NOT NULL CHECK (amount_cny_cent >= 0),
				order_kind TEXT NOT NULL DEFAULT 'subscription' CHECK (order_kind IN ('subscription', 'balance_topup')),
				delivery_mode TEXT NOT NULL DEFAULT 'account' CHECK (delivery_mode IN ('account', 'redeem_code')),
				balance_topup_micros BIGINT NOT NULL DEFAULT 0,
				payment_channel TEXT NOT NULL CHECK (payment_channel IN ('alipay')),
				generated_redeem_code_id TEXT NOT NULL DEFAULT '',
				upgrade_source_plan_id TEXT NOT NULL DEFAULT '',
				upgrade_source_expires_at DATETIME,
				upgrade_credit_cny_cent BIGINT NOT NULL DEFAULT 0,
				upgrade_locked_target_seconds BIGINT NOT NULL DEFAULT 0,
				upgrade_state_token TEXT NOT NULL DEFAULT '',
				payment_status TEXT NOT NULL DEFAULT 'pending' CHECK (payment_status IN ('pending', 'paid', 'expired', 'refunded', 'closed', 'failed')),
				fulfillment_status TEXT NOT NULL DEFAULT 'pending' CHECK (fulfillment_status IN ('pending', 'fulfilled', 'failed')),
				manual_settlement_done INTEGER NOT NULL DEFAULT 0,
				alipay_trade_no TEXT NOT NULL DEFAULT '',
				alipay_qr_code TEXT NOT NULL DEFAULT '',
				alipay_qr_url TEXT NOT NULL DEFAULT '',
				expires_at DATETIME,
				paid_at DATETIME,
				fulfilled_at DATETIME,
				failure_reason TEXT NOT NULL DEFAULT '',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
				FOREIGN KEY (product_id) REFERENCES purchase_products(id) ON DELETE RESTRICT,
				FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id) ON DELETE RESTRICT
			);
			INSERT INTO purchase_orders (
				id, order_no, user_id, product_id, subscription_plan_id, duration_days, amount_cny_cent,
				order_kind, delivery_mode, balance_topup_micros, payment_channel, generated_redeem_code_id,
				upgrade_source_plan_id, upgrade_source_expires_at, upgrade_credit_cny_cent, upgrade_locked_target_seconds, upgrade_state_token,
				payment_status, fulfillment_status, manual_settlement_done, alipay_trade_no, alipay_qr_code, alipay_qr_url,
				expires_at, paid_at, fulfilled_at, failure_reason, created_at, updated_at
			)
			SELECT
				id, order_no, user_id, product_id, subscription_plan_id, duration_days, amount_cny_cent,
				order_kind, COALESCE(delivery_mode, 'account'), balance_topup_micros, payment_channel, COALESCE(generated_redeem_code_id, ''),
				COALESCE(upgrade_source_plan_id, ''), upgrade_source_expires_at, COALESCE(upgrade_credit_cny_cent, 0), COALESCE(upgrade_locked_target_seconds, 0), COALESCE(upgrade_state_token, ''),
				payment_status, fulfillment_status, 0, alipay_trade_no, alipay_qr_code, alipay_qr_url,
				expires_at, paid_at, fulfilled_at, failure_reason, created_at, updated_at
			FROM purchase_orders_old2;
			DROP TABLE purchase_orders_old2;
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_user_created ON purchase_orders(user_id, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_payment_created ON purchase_orders(payment_status, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_fulfillment_created ON purchase_orders(fulfillment_status, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_product_created ON purchase_orders(product_id, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_trade_no ON purchase_orders(alipay_trade_no);
			CREATE INDEX IF NOT EXISTS idx_purchase_orders_manual_settlement_done ON purchase_orders(manual_settlement_done, created_at DESC)
		`
	case "postgres_widen_users_balance_micros", "postgres_widen_request_logs_micros_columns", "postgres_widen_request_logs_token_columns", "postgres_widen_subscription_plan_limits_limit_micros", "postgres_widen_billing_events_amount_micros":
		if dbType != DBTypePostgres {
			adapted = ""
		}
	}

	if dbType == DBTypePostgres {
		adapted = strings.ReplaceAll(adapted, "DATETIME", "TIMESTAMPTZ")
	}

	return adapted
}

func columnExists(tableName string, columnName string) (bool, error) {
	var query string
	switch dbType {
	case DBTypePostgres:
		query = `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`
	default:
		query = fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?", tableName)
	}

	var count int
	if dbType == DBTypePostgres {
		if err := db.QueryRow(query, tableName, columnName).Scan(&count); err != nil {
			return false, err
		}
	} else {
		if err := db.QueryRow(query, columnName).Scan(&count); err != nil {
			return false, err
		}
	}
	return count > 0, nil
}

func indexExists(tableName string, indexName string) (bool, error) {
	var (
		query string
		args  []any
	)

	switch dbType {
	case DBTypePostgres:
		query = `SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = ? AND indexname = ?`
		args = []any{tableName, indexName}
	default:
		query = `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?`
		args = []any{tableName, indexName}
	}

	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func execStatements(databaseHandle *sql.DB, sqlText string) error {
	for _, statement := range splitStatements(sqlText) {
		if _, err := databaseHandle.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func splitStatements(sqlText string) []string {
	parts := strings.Split(sqlText, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		statements = append(statements, trimmed)
	}
	return statements
}

func ensureSchemaMigrationsTable() error {
	columnType := "DATETIME"
	if dbType == DBTypePostgres {
		columnType = "TIMESTAMPTZ"
	}
	_, err := db.Exec(fmt.Sprintf(`
                CREATE TABLE IF NOT EXISTS schema_migrations (
                        name TEXT PRIMARY KEY,
                        applied_at %s NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
	`, columnType))
	return err
}

func isMigrationApplied(name string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func markMigrationApplied(name string) error {
	_, err := db.Exec(`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?) ON CONFLICT (name) DO NOTHING`, name, time.Now().UTC())
	return err
}

func shouldIgnoreMigrationError(name string, err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	if strings.Contains(msg, "duplicate column name") ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "duplicate key value") {
		return true
	}

	if name == "migrate_proxy_tokens" &&
		(strings.Contains(msg, "no such table") || strings.Contains(msg, "no such column") || strings.Contains(msg, "does not exist")) {
		return true
	}

	if name == "drop_legacy_users_group_id" || name == "drop_legacy_channels_group_id" {
		if strings.Contains(msg, "no such column") || strings.Contains(msg, "cannot drop") || strings.Contains(msg, "syntax error") || strings.Contains(msg, "does not exist") {
			return true
		}
	}

	if strings.HasPrefix(name, "postgres_widen_") {
		if strings.Contains(msg, "does not exist") || strings.Contains(msg, "cannot be cast automatically") {
			return true
		}
	}

	return false
}

func shouldIgnoreCreateTableStatementError(statement string, err error) bool {
	if err == nil {
		return false
	}

	normalizedStatement := strings.ToUpper(strings.TrimSpace(statement))
	if !strings.HasPrefix(normalizedStatement, "CREATE INDEX IF NOT EXISTS") &&
		!strings.HasPrefix(normalizedStatement, "CREATE UNIQUE INDEX IF NOT EXISTS") {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such column") || strings.Contains(msg, "does not exist")
}

// migrateTimestampsToUTC 将数据库中所有带时区偏移的 RFC3339 时间戳转换为 UTC
func migrateTimestampsToUTC(db *sql.DB) error {
	var done int
	err := db.QueryRow(`SELECT COUNT(*) FROM system_config WHERE key = 'migration_timestamps_utc'`).Scan(&done)
	if err != nil {
		return err
	}
	if done > 0 {
		return nil
	}

	type tableCol struct {
		table  string
		column string
	}
	targets := []tableCol{
		{"request_logs", "created_at"},
		{"request_logs", "updated_at"},
		{"request_log_details", "created_at"},
		{"users", "created_at"},
		{"users", "updated_at"},
		{"channels", "created_at"},
		{"channels", "updated_at"},
		{"groups", "created_at"},
		{"groups", "updated_at"},
		{"user_api_keys", "created_at"},
		{"user_api_keys", "last_used_at"},
		{"user_amp_settings", "created_at"},
		{"user_amp_settings", "updated_at"},
		{"channel_models", "created_at"},
		{"model_metadata", "created_at"},
		{"model_metadata", "updated_at"},
		{"model_prices", "created_at"},
		{"model_prices", "updated_at"},
		{"system_config", "updated_at"},
	}

	for _, tc := range targets {
		rows, err := db.Query(fmt.Sprintf(
			`SELECT rowid, %s FROM %s WHERE %s IS NOT NULL AND %s NOT LIKE '%%Z' AND %s LIKE '%%+%%'`,
			tc.column, tc.table, tc.column, tc.column, tc.column,
		))
		if err != nil {
			continue
		}

		type row struct {
			rowid int64
			ts    string
		}
		var toUpdate []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.rowid, &r.ts); err != nil {
				continue
			}
			toUpdate = append(toUpdate, r)
		}
		rows.Close()

		if len(toUpdate) == 0 {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return err
		}
		stmt, err := tx.Prepare(fmt.Sprintf(`UPDATE %s SET %s = ? WHERE rowid = ?`, tc.table, tc.column))
		if err != nil {
			tx.Rollback()
			return err
		}
		for _, r := range toUpdate {
			t, err := time.Parse(time.RFC3339, r.ts)
			if err != nil {
				continue
			}
			_, _ = stmt.Exec(t.UTC().Format(time.RFC3339), r.rowid)
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	_, err = db.Exec(`INSERT INTO system_config (key, value, updated_at) VALUES ('migration_timestamps_utc', '1', ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, time.Now().UTC())
	return err
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if db != nil {
		err := db.Close()
		db = nil
		return err
	}
	return nil
}
