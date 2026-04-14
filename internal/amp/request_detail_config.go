package amp

import (
	"hash/fnv"
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
	DefaultHighRPMThreshold             = 3000
	DefaultHighRPMSamplePercent         = 10
	RequestDetailModeFull               = "full"
	RequestDetailModeOff                = "off"
	RequestDetailModeSample             = "sample"
)

type RequestDetailConfig struct {
	Enabled              bool
	TTL                  time.Duration
	MaxEntries           int
	MaxMemoryBytes       int64
	BodyCapBytes         int
	PersistEnabled       bool
	HighRPMMode          string
	HighRPMThreshold     int
	HighRPMSamplePercent int
}

var (
	requestDetailConfigMu sync.RWMutex
	requestDetailConfig   = DefaultRequestDetailConfig()
	requestDetailRPMState = newRequestDetailRPMCounter()
)

func DefaultRequestDetailConfig() RequestDetailConfig {
	return RequestDetailConfig{
		Enabled:              true,
		TTL:                  DefaultRequestDetailTTL,
		MaxEntries:           DefaultRequestDetailMaxEntries,
		MaxMemoryBytes:       DefaultRequestDetailMaxMemoryBytes,
		BodyCapBytes:         DefaultRequestDetailBodyCapBytes,
		PersistEnabled:       true,
		HighRPMMode:          RequestDetailModeFull,
		HighRPMThreshold:     DefaultHighRPMThreshold,
		HighRPMSamplePercent: DefaultHighRPMSamplePercent,
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
	switch cfg.HighRPMMode {
	case RequestDetailModeFull, RequestDetailModeOff, RequestDetailModeSample:
	default:
		cfg.HighRPMMode = defaults.HighRPMMode
	}
	if cfg.HighRPMThreshold <= 0 {
		cfg.HighRPMThreshold = defaults.HighRPMThreshold
	}
	if cfg.HighRPMSamplePercent <= 0 {
		cfg.HighRPMSamplePercent = defaults.HighRPMSamplePercent
	}
	if cfg.HighRPMSamplePercent > 100 {
		cfg.HighRPMSamplePercent = 100
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

func ShouldCaptureRequestDetail(requestID string) bool {
	cfg := GetRequestDetailConfig()
	if !cfg.Enabled {
		return false
	}
	if cfg.HighRPMMode == RequestDetailModeFull {
		return true
	}

	currentRPM := requestDetailRPMState.Record(time.Now().UTC())
	if currentRPM < cfg.HighRPMThreshold {
		return true
	}

	switch cfg.HighRPMMode {
	case RequestDetailModeOff:
		return false
	case RequestDetailModeSample:
		return shouldSampleRequestDetail(requestID, cfg.HighRPMSamplePercent)
	default:
		return true
	}
}

func shouldSampleRequestDetail(requestID string, samplePercent int) bool {
	if samplePercent >= 100 {
		return true
	}
	if samplePercent <= 0 {
		return false
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(requestID))
	return int(hasher.Sum32()%100) < samplePercent
}

type requestDetailRPMCounter struct {
	mu       sync.Mutex
	lastSeen int64
	buckets  [60]rpmBucket
	total    int
}

type rpmBucket struct {
	sec   int64
	count int
}

func newRequestDetailRPMCounter() *requestDetailRPMCounter {
	return &requestDetailRPMCounter{}
}

func (c *requestDetailRPMCounter) Record(now time.Time) int {
	sec := now.Unix()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lastSeen != 0 && sec-c.lastSeen >= int64(len(c.buckets)) {
		c.total = 0
		for i := range c.buckets {
			c.buckets[i] = rpmBucket{}
		}
	}
	c.lastSeen = sec

	idx := int(sec % int64(len(c.buckets)))
	bucket := &c.buckets[idx]
	if bucket.sec != sec {
		if bucket.sec != 0 && sec-bucket.sec < int64(len(c.buckets)) {
			c.total -= bucket.count
		}
		bucket.sec = sec
		bucket.count = 0
	}

	bucket.count++
	c.total++
	return c.total
}
