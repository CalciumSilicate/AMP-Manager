package config

import (
	"ampmanager/internal/database"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AdminUsername string
	AdminPassword string
	ServerPort    string
	ServerRole    string
	JWTSecret     string
	JWTIssuer     string
	JWTAudience   string
	DBType        string
	DatabaseURL   string
	SQLitePath    string

	// CORS 配置
	CORSAllowedOrigins string

	// 速率限制配置
	RateLimitAuthRPS  float64
	RateLimitProxyRPS float64

	// 数据加密密钥 (32 bytes for AES-256)
	DataEncryptionKey string

	RedisURL                     string
	RedisPrefix                  string
	BillingReservationTTLSec     int
	BillingReconcileIntervalSec  int
	BillingStreamBatchSize       int
	BillingReconcileBatchSize    int
	BillingExpiryBatchSize       int
	BillingProjectorWorkers      int
	BillingProjectorClaimIdleSec int
	BillingEnvExplicit           BillingEnvExplicit
}

type BillingEnvExplicit struct {
	ReconcileBatchSize bool
	ExpiryBatchSize    bool
}

var cfg *Config

var insecureDefaults = map[string]string{
	"ADMIN_PASSWORD": "admin123",
	"JWT_SECRET":     "amp-manager-default-secret-change-in-production",
}

func Load() *Config {
	runtimeOptions, hasRuntimeOptions, runtimeErr := loadRuntimeDatabaseOptions()
	if runtimeErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load runtime database config: %v\n", runtimeErr)
	}

	defaultDBType := string(database.DBTypeSQLite)
	defaultSQLitePath := "./data/data.db"
	defaultDatabaseURL := ""
	if hasRuntimeOptions {
		defaultDBType = string(runtimeOptions.Type)
		defaultSQLitePath = runtimeOptions.SQLitePath
		defaultDatabaseURL = runtimeOptions.DatabaseURL
	}

	cfg = &Config{
		AdminUsername:                getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:                getEnv("ADMIN_PASSWORD", "admin123"),
		ServerPort:                   getEnv("SERVER_PORT", "16823"),
		ServerRole:                   getEnv("SERVER_ROLE", string(ServerRoleAll)),
		JWTSecret:                    getEnv("JWT_SECRET", "amp-manager-default-secret-change-in-production"),
		JWTIssuer:                    getEnv("JWT_ISSUER", "ampmanager"),
		JWTAudience:                  getEnv("JWT_AUDIENCE", "ampmanager-users"),
		DBType:                       getEnv("DB_TYPE", defaultDBType),
		DatabaseURL:                  getEnv("DATABASE_URL", defaultDatabaseURL),
		SQLitePath:                   getEnv("SQLITE_PATH", defaultSQLitePath),
		CORSAllowedOrigins:           getEnv("CORS_ALLOWED_ORIGINS", "*"),
		RateLimitAuthRPS:             getEnvFloat("RATE_LIMIT_AUTH_RPS", 5),
		RateLimitProxyRPS:            getEnvFloat("RATE_LIMIT_PROXY_RPS", 100),
		DataEncryptionKey:            getEnv("DATA_ENCRYPTION_KEY", ""),
		RedisURL:                     getEnv("REDIS_URL", ""),
		RedisPrefix:                  getEnv("REDIS_PREFIX", "ampmanager"),
		BillingReservationTTLSec:     getEnvInt("BILLING_RESERVATION_TTL_SEC", 600),
		BillingReconcileIntervalSec:  getEnvInt("BILLING_RECONCILE_INTERVAL_SEC", 60),
		BillingStreamBatchSize:       getEnvInt("BILLING_STREAM_BATCH_SIZE", 100),
		BillingReconcileBatchSize:    getEnvInt("BILLING_RECONCILE_BATCH_SIZE", getEnvInt("BILLING_STREAM_BATCH_SIZE", 100)),
		BillingExpiryBatchSize:       getEnvInt("BILLING_EXPIRY_BATCH_SIZE", getEnvInt("BILLING_STREAM_BATCH_SIZE", 100)),
		BillingProjectorWorkers:      getEnvInt("BILLING_PROJECTOR_WORKERS", 1),
		BillingProjectorClaimIdleSec: getEnvInt("BILLING_PROJECTOR_CLAIM_IDLE_SEC", 30),
		BillingEnvExplicit: BillingEnvExplicit{
			ReconcileBatchSize: hasEnv("BILLING_RECONCILE_BATCH_SIZE"),
			ExpiryBatchSize:    hasEnv("BILLING_EXPIRY_BATCH_SIZE"),
		},
	}
	return cfg
}

func ValidateSecurityConfig(cfg *Config) error {
	if os.Getenv("ALLOW_INSECURE_DEFAULTS") == "true" {
		return nil
	}

	var issues []string

	if cfg.AdminPassword == insecureDefaults["ADMIN_PASSWORD"] {
		issues = append(issues, "ADMIN_PASSWORD is using insecure default 'admin123'")
	}

	if cfg.JWTSecret == insecureDefaults["JWT_SECRET"] {
		issues = append(issues, "JWT_SECRET is using insecure default value")
	}

	if len(cfg.JWTSecret) < 32 {
		issues = append(issues, "JWT_SECRET should be at least 32 characters")
	}

	if cfg.DataEncryptionKey != "" && len(cfg.DataEncryptionKey) != 32 {
		issues = append(issues, "DATA_ENCRYPTION_KEY must be exactly 32 characters for AES-256")
	}

	if len(issues) > 0 {
		return fmt.Errorf("security configuration errors:\n  - %s\n\nSet ALLOW_INSECURE_DEFAULTS=true to bypass (NOT recommended for production)", strings.Join(issues, "\n  - "))
	}

	return nil
}

func Get() *Config {
	return cfg
}

func (c *Config) GetEncryptionKey() []byte {
	if c.DataEncryptionKey == "" {
		return nil
	}
	return []byte(c.DataEncryptionKey)
}

func (c *Config) DatabaseOptions() database.Options {
	return database.Options{
		Type:        database.DBType(c.DBType),
		DatabaseURL: c.DatabaseURL,
		SQLitePath:  c.SQLitePath,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func hasEnv(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}
