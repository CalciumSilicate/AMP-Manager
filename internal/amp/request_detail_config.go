package amp

import (
	"sync"
	"time"
)

const (
	DefaultRequestDetailTTL             = 120 * time.Second
	DefaultRequestDetailCleanupInterval = 30 * time.Second
	DefaultRequestDetailMaxEntries      = 500
	DefaultRequestDetailMaxMemoryBytes  = 256 * 1024 * 1024
	DefaultRequestDetailBodyCapBytes    = 128 * 1024
	DefaultRequestDetailPersistQueueCap = 256
	DefaultLastUsedUpdateInterval       = 60 * time.Second
)

type RequestDetailConfig struct {
	Enabled        bool
	TTL            time.Duration
	MaxEntries     int
	MaxMemoryBytes int64
	BodyCapBytes   int
	PersistEnabled bool
}

var (
	requestDetailConfigMu sync.RWMutex
	requestDetailConfig   = DefaultRequestDetailConfig()
)

func DefaultRequestDetailConfig() RequestDetailConfig {
	return RequestDetailConfig{
		Enabled:        true,
		TTL:            DefaultRequestDetailTTL,
		MaxEntries:     DefaultRequestDetailMaxEntries,
		MaxMemoryBytes: DefaultRequestDetailMaxMemoryBytes,
		BodyCapBytes:   DefaultRequestDetailBodyCapBytes,
		PersistEnabled: true,
	}
}

func normalizeRequestDetailConfig(cfg RequestDetailConfig) RequestDetailConfig {
	defaults := DefaultRequestDetailConfig()

	if cfg.TTL <= 0 {
		cfg.TTL = defaults.TTL
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = defaults.MaxEntries
	}
	if cfg.MaxMemoryBytes <= 0 {
		cfg.MaxMemoryBytes = defaults.MaxMemoryBytes
	}
	if cfg.BodyCapBytes <= 0 {
		cfg.BodyCapBytes = defaults.BodyCapBytes
	}

	return cfg
}

func GetRequestDetailConfig() RequestDetailConfig {
	requestDetailConfigMu.RLock()
	defer requestDetailConfigMu.RUnlock()
	return requestDetailConfig
}

func UpdateRequestDetailConfig(cfg RequestDetailConfig) {
	cfg = normalizeRequestDetailConfig(cfg)

	requestDetailConfigMu.Lock()
	requestDetailConfig = cfg
	requestDetailConfigMu.Unlock()

	if store := GetRequestDetailStore(); store != nil {
		store.ApplyConfig(cfg)
	}
}

// SetRequestDetailEnabled keeps backward compatibility with the existing toggle API.
func SetRequestDetailEnabled(enabled bool) {
	cfg := GetRequestDetailConfig()
	cfg.Enabled = enabled
	UpdateRequestDetailConfig(cfg)
}

func IsRequestDetailEnabled() bool {
	return GetRequestDetailConfig().Enabled
}
