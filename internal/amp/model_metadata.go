package amp

import (
	"strings"
	"sync"
	"time"

	"ampmanager/internal/repository"

	log "github.com/sirupsen/logrus"
)

// ModelMetadata contains context size information for known models
type ModelMetadata struct {
	ContextLength       int
	MaxCompletionTokens int
}

// modelMetadataCache caches model metadata from database
type modelMetadataCache struct {
	mu       sync.RWMutex
	data     map[string]*ModelMetadata
	loadedAt time.Time
	cacheTTL time.Duration
}

var metadataCache = &modelMetadataCache{
	data:     make(map[string]*ModelMetadata),
	cacheTTL: 5 * time.Minute,
}

// knownModelMetadata is the fallback for when database is not available
var knownModelMetadata = map[string]ModelMetadata{
	// Claude models - 200k context
	"claude-4":      {ContextLength: 200000, MaxCompletionTokens: 64000},
	"claude-3":      {ContextLength: 200000, MaxCompletionTokens: 8192},
	"claude-sonnet": {ContextLength: 200000, MaxCompletionTokens: 64000},
	"claude-opus":   {ContextLength: 200000, MaxCompletionTokens: 64000},
	"claude-haiku":  {ContextLength: 200000, MaxCompletionTokens: 64000},

	// OpenAI GPT-5 models - 400k context
	"gpt-5":       {ContextLength: 400000, MaxCompletionTokens: 128000},
	"gpt-5.1":     {ContextLength: 400000, MaxCompletionTokens: 128000},
	"gpt-5.2":     {ContextLength: 400000, MaxCompletionTokens: 128000},
	"gpt-5-codex": {ContextLength: 400000, MaxCompletionTokens: 128000},

	// OpenAI GPT-4 models - 128k context
	"gpt-4":   {ContextLength: 128000, MaxCompletionTokens: 16384},
	"gpt-4o":  {ContextLength: 128000, MaxCompletionTokens: 16384},
	"gpt-4.1": {ContextLength: 1047576, MaxCompletionTokens: 32768},

	// Gemini models - 1M context
	"gemini-2.5":       {ContextLength: 1048576, MaxCompletionTokens: 65536},
	"gemini-3":         {ContextLength: 1048576, MaxCompletionTokens: 65536},
	"gemini-2.5-pro":   {ContextLength: 1048576, MaxCompletionTokens: 65536},
	"gemini-2.5-flash": {ContextLength: 1048576, MaxCompletionTokens: 65536},

	// DeepSeek models
	"deepseek-v3": {ContextLength: 128000, MaxCompletionTokens: 8192},
	"deepseek-r1": {ContextLength: 128000, MaxCompletionTokens: 8192},

	// Qwen models
	"qwen3":       {ContextLength: 32768, MaxCompletionTokens: 8192},
	"qwen3-coder": {ContextLength: 32768, MaxCompletionTokens: 8192},
}

// refreshCache reloads model metadata from database
func (c *modelMetadataCache) refreshCache() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if cache is still fresh
	if time.Since(c.loadedAt) < c.cacheTTL && len(c.data) > 0 {
		return
	}

	repo := repository.NewModelMetadataRepository()
	models, err := repo.List()
	if err != nil {
		log.Warnf("model metadata cache: failed to load from database: %v", err)
		return
	}

	newData := make(map[string]*ModelMetadata, len(models))
	for _, m := range models {
		newData[m.ModelPattern] = &ModelMetadata{
			ContextLength:       m.ContextLength,
			MaxCompletionTokens: m.MaxCompletionTokens,
		}
	}

	c.data = newData
	c.loadedAt = time.Now()
	log.Debugf("model metadata cache: loaded %d models from database", len(newData))
}

// get retrieves model metadata from cache
func (c *modelMetadataCache) get(modelName string) *ModelMetadata {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try exact match first
	if meta, ok := c.data[modelName]; ok {
		return meta
	}

	// Try pattern matching (supports * wildcard at end)
	for pattern, meta := range c.data {
		if matchPattern(pattern, modelName) {
			return meta
		}
	}

	return nil
}

// matchPattern checks if modelName matches the pattern
// Supports * wildcard at the end of pattern (e.g., "claude-*" matches "claude-sonnet-4")
func matchPattern(pattern, modelName string) bool {
	// If pattern ends with *, match prefix
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(modelName, prefix)
	}
	// Otherwise, check if modelName starts with pattern
	return len(modelName) >= len(pattern) && modelName[:len(pattern)] == pattern
}

// InvalidateModelMetadataCache forces cache refresh on next access
func InvalidateModelMetadataCache() {
	metadataCache.mu.Lock()
	defer metadataCache.mu.Unlock()
	metadataCache.loadedAt = time.Time{}
	clearReservationMaxCompletionTokensCache()
}

// GetModelMetadata looks up context size for a model name
// First tries database cache, then falls back to hardcoded values
func GetModelMetadata(modelName string) *ModelMetadata {
	if modelName == "" {
		return nil
	}

	// Try to refresh cache if needed
	metadataCache.refreshCache()

	// Try database cache first
	if meta := metadataCache.get(modelName); meta != nil {
		log.Debugf("model metadata: found %s in cache (context=%d)", modelName, meta.ContextLength)
		return meta
	}

	// Fallback to hardcoded values - exact match
	if meta, ok := knownModelMetadata[modelName]; ok {
		log.Debugf("model metadata: found %s in hardcoded (exact, context=%d)", modelName, meta.ContextLength)
		return &meta
	}

	// Try prefix matching on hardcoded values
	// Check if modelName starts with any known prefix
	for prefix, meta := range knownModelMetadata {
		if len(modelName) >= len(prefix) && modelName[:len(prefix)] == prefix {
			m := meta
			log.Debugf("model metadata: found %s via prefix %s (context=%d)", modelName, prefix, m.ContextLength)
			return &m
		}
	}

	// Also try reverse: check if any known model starts with modelName
	// This handles cases like "opus" matching "claude-opus"
	for knownModel, meta := range knownModelMetadata {
		if strings.Contains(knownModel, modelName) {
			m := meta
			log.Debugf("model metadata: found %s via contains %s (context=%d)", modelName, knownModel, m.ContextLength)
			return &m
		}
	}

	log.Debugf("model metadata: no match for %s", modelName)
	return nil
}

// GetBuiltinModelMetadata returns the hardcoded model metadata for database seeding
func GetBuiltinModelMetadata() map[string]ModelMetadata {
	return knownModelMetadata
}
