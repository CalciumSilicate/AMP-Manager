package amp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"sync"

	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Context keys for model mapping (gin.Context)
const (
	OriginalModelContextKey    = "original_model"
	MappedModelContextKey      = "mapped_model"
	PreferredChannelContextKey = "preferred_channel_id"
	ModelMappingAppliedKey     = "model_mapping_applied"
	ThinkingLevelContextKey    = "thinking_level"
	PseudoNonStreamContextKey  = "pseudo_non_stream"
)

// modelInfoKey 用于在 req.Context() 中存储模型信息
type modelInfoKey struct{}

// ModelInfo 存储在 context 中的模型信息
type ModelInfo struct {
	OriginalModel string
	MappedModel   string
}

// WithModelInfo 将模型信息存入 context
func WithModelInfo(ctx context.Context, original, mapped string) context.Context {
	return context.WithValue(ctx, modelInfoKey{}, &ModelInfo{
		OriginalModel: original,
		MappedModel:   mapped,
	})
}

// GetModelInfo 从 context 获取模型信息
func GetModelInfo(ctx context.Context) *ModelInfo {
	if val := ctx.Value(modelInfoKey{}); val != nil {
		if info, ok := val.(*ModelInfo); ok {
			return info
		}
	}
	return nil
}

// pseudoNonStreamKey 用于在 req.Context() 中传递 PseudoNonStream 标志
type pseudoNonStreamKey struct{}

// WithPseudoNonStream 将 PseudoNonStream 标志存入 context
func WithPseudoNonStream(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, pseudoNonStreamKey{}, enabled)
}

// GetPseudoNonStream 从 context 获取 PseudoNonStream 标志
func GetPseudoNonStream(ctx context.Context) bool {
	if val := ctx.Value(pseudoNonStreamKey{}); val != nil {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

type auditKeywordsKey struct{}

func WithAuditKeywords(ctx context.Context, keywords []string) context.Context {
	return context.WithValue(ctx, auditKeywordsKey{}, keywords)
}

func GetAuditKeywords(ctx context.Context) []string {
	if val := ctx.Value(auditKeywordsKey{}); val != nil {
		if kw, ok := val.([]string); ok {
			return kw
		}
	}
	return nil
}

type MappingResult struct {
	OriginalModel             string
	MappedModel               string
	PreferredChannelID        string
	ThinkingLevel             string
	PseudoNonStream           bool
	AuditKeywords             []string
	FastMode                  bool
	CustomInstructions        string
	CustomInstructionsEnabled bool
	Applied                   bool
}

// channelService for checking model availability
var mappingChannelService = service.NewChannelService()
var compiledModelMappingsCache sync.Map

type compiledModelMapping struct {
	model.ModelMapping
	regex *regexp.Regexp
}

func getCompiledModelMappings(raw string) ([]compiledModelMapping, bool) {
	if raw == "" {
		return nil, false
	}
	if cached, ok := compiledModelMappingsCache.Load(raw); ok {
		if mappings, ok := cached.([]compiledModelMapping); ok {
			return mappings, true
		}
	}

	var mappings []model.ModelMapping
	if err := json.Unmarshal([]byte(raw), &mappings); err != nil {
		return nil, false
	}
	if len(mappings) == 0 {
		return nil, false
	}

	compiled := make([]compiledModelMapping, 0, len(mappings))
	for _, mapping := range mappings {
		entry := compiledModelMapping{ModelMapping: mapping}
		if mapping.Regex && mapping.From != "" {
			re, err := regexp.Compile("(?i)" + mapping.From)
			if err != nil {
				continue
			}
			entry.regex = re
		}
		compiled = append(compiled, entry)
	}
	if len(compiled) == 0 {
		return nil, false
	}
	compiledModelMappingsCache.Store(raw, compiled)
	return compiled, true
}

func ApplyModelMappingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := GetProxyConfig(c.Request.Context())
		if cfg == nil || cfg.ModelMappingsJSON == "" {
			c.Next()
			return
		}

		mappings, ok := getCompiledModelMappings(cfg.ModelMappingsJSON)
		if !ok {
			c.Next()
			return
		}

		// Try to extract model from URL path first (for Gemini)
		modelName, modelSource := extractModelFromRequestPath(c)

		// Read body for non-Gemini requests or if path extraction failed
		var bodyBytes []byte
		if requestPayload, err := ensureRequestBody(c); err == nil {
			bodyBytes = requestPayload.Body
			if modelName == "" {
				if bodyModel := strings.TrimSpace(gjson.GetBytes(bodyBytes, "model").String()); bodyModel != "" {
					modelName = bodyModel
					modelSource = "body"
				}
			}
		}

		// No model found, restore body and continue
		if modelName == "" {
			c.Next()
			return
		}

		// Apply mapping (pass header getter for AMP-only check)
		result := applyCompiledMappingWithHeaders(modelName, mappings, c.GetHeader)

		if !result.Applied {
			c.Next()
			return
		}

		// Validate explicit channel bindings first so the mapping stays deterministic.
		if result.PreferredChannelID != "" {
			channel, err := mappingChannelService.SelectSpecificChannelForModelWithGroups(result.PreferredChannelID, result.MappedModel, cfg.GroupIDs)
			if err != nil || channel == nil {
				log.Warnf("model mapping: preferred channel '%s' cannot serve model '%s', skipping mapping", result.PreferredChannelID, result.MappedModel)
				c.Next()
				return
			}
		} else if result.MappedModel != modelName {
			channel, err := mappingChannelService.SelectChannelForModel(result.MappedModel)
			if err != nil || channel == nil {
				log.Warnf("model mapping: target model '%s' has no available channel, skipping mapping", result.MappedModel)
				c.Next()
				return
			}
		}

		// Store original and mapped model in context (both gin.Context and req.Context)
		c.Set(OriginalModelContextKey, result.OriginalModel)
		c.Set(MappedModelContextKey, result.MappedModel)
		if result.PreferredChannelID != "" {
			c.Set(PreferredChannelContextKey, result.PreferredChannelID)
		}
		c.Set(ModelMappingAppliedKey, true)
		// Also store in req.Context for upstream layers
		ctx := WithModelInfo(c.Request.Context(), result.OriginalModel, result.MappedModel)

		// Store PseudoNonStream flag if enabled
		if result.PseudoNonStream {
			c.Set(PseudoNonStreamContextKey, true)
			ctx = WithPseudoNonStream(ctx, true)
		}

		// Store AuditKeywords if configured
		if len(result.AuditKeywords) > 0 {
			ctx = WithAuditKeywords(ctx, result.AuditKeywords)
		}

		c.Request = c.Request.WithContext(ctx)

		if result.PreferredChannelID != "" {
			log.Infof("model mapping: %s -> %s via channel %s (source: %s)", result.OriginalModel, result.MappedModel, result.PreferredChannelID, modelSource)
		} else {
			log.Infof("model mapping: %s -> %s (source: %s)", result.OriginalModel, result.MappedModel, modelSource)
		}

		// Apply mapping based on source
		if modelSource == "path" {
			// Rewrite URL path for Gemini requests
			newPath := rewriteModelInPath(c.Request.URL.Path, result.OriginalModel, result.MappedModel)
			if newPath != c.Request.URL.Path {
				log.Debugf("model mapping: path rewrite %s -> %s", c.Request.URL.Path, newPath)
				c.Request.URL.Path = newPath
				c.Request.RequestURI = newPath
				if c.Request.URL.RawQuery != "" {
					c.Request.RequestURI = newPath + "?" + c.Request.URL.RawQuery
				}
			}
		}

		// Also update body if it contains model field.
		if len(bodyBytes) > 0 && gjson.GetBytes(bodyBytes, "model").Exists() {
			if updated, err := sjson.SetBytes(bodyBytes, "model", result.MappedModel); err == nil {
				bodyBytes = updated
			}

			needsStructuredMutation := result.ThinkingLevel != "" || (result.CustomInstructionsEnabled && result.CustomInstructions != "")
			if !needsStructuredMutation {
				if result.FastMode {
					if updated, err := sjson.SetBytes(bodyBytes, "service_tier", "priority"); err == nil {
						bodyBytes = updated
						log.Infof("model mapping: applied fast mode (service_tier=priority)")
					}
				}
			} else if requestPayload, err := EnsureRequestPayload(c); err == nil && requestPayload.JSON != nil {
				payload := requestPayload.JSON
				payload["model"] = result.MappedModel

				thinkingLevel := result.ThinkingLevel
				if thinkingLevel != "" {
					applyThinkingLevelWithPath(payload, thinkingLevel, c.Request.URL.Path)
					c.Set(ThinkingLevelContextKey, thinkingLevel)
					log.Infof("model mapping: applied thinking level '%s'", thinkingLevel)
				}

				if result.FastMode {
					payload["service_tier"] = "priority"
					log.Infof("model mapping: applied fast mode (service_tier=priority)")
				}

				if result.CustomInstructionsEnabled && result.CustomInstructions != "" {
					applyCustomInstructions(payload, result.CustomInstructions, c.Request.URL.Path)
					log.Infof("model mapping: applied custom instructions")
				}

				if newBody, marshalErr := json.Marshal(payload); marshalErr == nil {
					bodyBytes = newBody
				}
			}

			if requestPayload := GetRequestPayload(c.Request.Context()); requestPayload != nil {
				requestPayload.Body = bodyBytes
				requestPayload.JSON = nil
				requestPayload.jsonParsed = false
			}
		}

		// Restore body
		if bodyBytes != nil {
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			c.Request.ContentLength = int64(len(bodyBytes))
		}

		c.Next()
	}
}

// extractModelFromRequestPath extracts model name from URL path (for Gemini requests)
// Returns the model name and source ("path" or "")
func extractModelFromRequestPath(c *gin.Context) (string, string) {
	path := c.Request.URL.Path

	// Handle v1beta1/publishers/google/models/ path (used by Amp CLI)
	// Example: /api/provider/google/v1beta1/publishers/google/models/gemini-3-flash-preview:generateContent
	if idx := strings.Index(path, "/v1beta1/publishers/google/models/"); idx >= 0 {
		modelPart := path[idx+len("/v1beta1/publishers/google/models/"):]
		if model := extractModelFromPathPart(modelPart); model != "" {
			return model, "path"
		}
	}

	// Handle v1beta/models/ path
	// Example: /v1beta/models/gemini-3-flash-preview:generateContent
	if idx := strings.Index(path, "/v1beta/models/"); idx >= 0 {
		modelPart := path[idx+len("/v1beta/models/"):]
		if model := extractModelFromPathPart(modelPart); model != "" {
			return model, "path"
		}
	}

	// Handle /models/ path (generic)
	if idx := strings.Index(path, "/models/"); idx >= 0 {
		modelPart := path[idx+len("/models/"):]
		if model := extractModelFromPathPart(modelPart); model != "" {
			return model, "path"
		}
	}

	return "", ""
}

// Note: extractModelFromPathPart is defined in channel_router.go and reused here

// rewriteModelInPath rewrites the model name in URL path
func rewriteModelInPath(path, oldModel, newModel string) string {
	if oldModel == newModel {
		return path
	}

	// Try different path patterns

	// Pattern 1: /v1beta1/publishers/google/models/{model}:method
	if idx := strings.Index(path, "/v1beta1/publishers/google/models/"); idx >= 0 {
		prefix := path[:idx+len("/v1beta1/publishers/google/models/")]
		rest := path[idx+len("/v1beta1/publishers/google/models/"):]
		return prefix + replaceModelInSegment(rest, oldModel, newModel)
	}

	// Pattern 2: /v1beta/models/{model}:method
	if idx := strings.Index(path, "/v1beta/models/"); idx >= 0 {
		prefix := path[:idx+len("/v1beta/models/")]
		rest := path[idx+len("/v1beta/models/"):]
		return prefix + replaceModelInSegment(rest, oldModel, newModel)
	}

	// Pattern 3: /models/{model}:method (generic)
	if idx := strings.Index(path, "/models/"); idx >= 0 {
		prefix := path[:idx+len("/models/")]
		rest := path[idx+len("/models/"):]
		return prefix + replaceModelInSegment(rest, oldModel, newModel)
	}

	return path
}

// replaceModelInSegment replaces model name in a path segment like "gemini-3-flash:generateContent"
func replaceModelInSegment(segment, oldModel, newModel string) string {
	// Check if segment starts with the old model name
	if strings.HasPrefix(segment, oldModel) {
		suffix := segment[len(oldModel):]
		// Ensure it's a complete model name (followed by : or / or end)
		if suffix == "" || suffix[0] == ':' || suffix[0] == '/' {
			return newModel + suffix
		}
	}
	// Fallback: try case-insensitive replacement
	if strings.HasPrefix(strings.ToLower(segment), strings.ToLower(oldModel)) {
		suffix := segment[len(oldModel):]
		if suffix == "" || suffix[0] == ':' || suffix[0] == '/' {
			return newModel + suffix
		}
	}
	return segment
}

func applyMapping(modelName string, mappings []model.ModelMapping) MappingResult {
	return applyMappingWithHeaders(modelName, mappings, nil)
}

func applyMappingWithHeaders(modelName string, mappings []model.ModelMapping, header func(string) string) MappingResult {
	compiled := make([]compiledModelMapping, 0, len(mappings))
	for _, mapping := range mappings {
		entry := compiledModelMapping{ModelMapping: mapping}
		if mapping.Regex && mapping.From != "" {
			re, err := regexp.Compile("(?i)" + mapping.From)
			if err == nil {
				entry.regex = re
			}
		}
		compiled = append(compiled, entry)
	}
	return applyCompiledMappingWithHeaders(modelName, compiled, header)
}

func applyCompiledMappingWithHeaders(modelName string, mappings []compiledModelMapping, header func(string) string) MappingResult {
	for _, m := range mappings {
		if m.From == "" {
			continue
		}

		// If ampOnly is set, skip this mapping when request is not from AMP
		if m.AmpOnly {
			isAmp := false
			if header != nil {
				isAmp = header("X-Amp-Feature") == "amp.chat"
			}
			if !isAmp {
				continue
			}
		}

		matched := false
		if m.Regex {
			if m.regex != nil && m.regex.MatchString(modelName) {
				matched = true
			}
		} else {
			if strings.EqualFold(m.From, modelName) || m.From == modelName {
				matched = true
			}
		}

		if matched {
			targetModel := m.To
			if targetModel == "" {
				targetModel = modelName
			}

			return MappingResult{
				OriginalModel:             modelName,
				MappedModel:               targetModel,
				PreferredChannelID:        m.ChannelID,
				ThinkingLevel:             m.ThinkingLevel,
				PseudoNonStream:           m.PseudoNonStream,
				AuditKeywords:             m.AuditKeywords,
				FastMode:                  m.FastMode,
				CustomInstructions:        m.CustomInstructions,
				CustomInstructionsEnabled: m.CustomInstructionsEnabled,
				Applied:                   true,
			}
		}
	}

	return MappingResult{
		OriginalModel: modelName,
		MappedModel:   modelName,
		Applied:       false,
	}
}

func applyThinkingLevel(payload map[string]interface{}, level string) {
	applyThinkingLevelWithPath(payload, level, "")
}

// applyThinkingLevelWithPath applies thinking level based on model and request path
func applyThinkingLevelWithPath(payload map[string]interface{}, level string, requestPath string) {
	if level == "" {
		return
	}

	modelName, _ := payload["model"].(string)
	modelLower := strings.ToLower(modelName)

	// OpenAI reasoning models (o1, o3, o4) and GPT models
	if strings.HasPrefix(modelLower, "o1") || strings.HasPrefix(modelLower, "o3") || strings.HasPrefix(modelLower, "o4") || strings.HasPrefix(modelLower, "gpt") {
		// Check if using /v1/responses endpoint (new API format)
		if strings.Contains(requestPath, "/responses") {
			// New format: reasoning: { effort: "..." }
			reasoning := map[string]interface{}{
				"effort": level,
			}
			payload["reasoning"] = reasoning
		} else {
			// Old format for /v1/chat/completions: reasoning_effort: "..."
			payload["reasoning_effort"] = level
		}
		return
	}

	if strings.HasPrefix(modelLower, "claude") {
		budgetTokens := thinkingLevelToBudget(level, "claude")
		if budgetTokens > 0 {
			thinking := map[string]interface{}{
				"type":          "enabled",
				"budget_tokens": budgetTokens,
			}
			payload["thinking"] = thinking
		}
		return
	}

	if strings.HasPrefix(modelLower, "gemini") {
		budgetTokens := thinkingLevelToBudget(level, "gemini")
		generationConfig, ok := payload["generationConfig"].(map[string]interface{})
		if !ok {
			generationConfig = make(map[string]interface{})
		}
		thinkingConfig := map[string]interface{}{
			"thinkingBudget": budgetTokens,
		}
		generationConfig["thinkingConfig"] = thinkingConfig
		payload["generationConfig"] = generationConfig
		return
	}

	payload["reasoning_effort"] = level
}

func thinkingLevelToBudget(level string, provider string) int {
	levelLower := strings.ToLower(level)

	if provider == "claude" {
		switch levelLower {
		case "low":
			return 1024
		case "medium":
			return 8192
		case "high":
			return 32768
		case "xhigh":
			return 100000
		default:
			return 0
		}
	}

	if provider == "gemini" {
		switch levelLower {
		case "low":
			return 1024
		case "medium":
			return 8192
		case "high":
			return 24576
		case "xhigh":
			return 32768
		default:
			return 0
		}
	}

	return 0
}

// applyCustomInstructions injects or replaces the instructions field in the request payload.
// For OpenAI Responses API (/v1/responses): sets the "instructions" field directly.
// For OpenAI Chat Completions API: injects/replaces the system message in "messages".
func applyCustomInstructions(payload map[string]interface{}, instructions string, requestPath string) {
	if strings.Contains(requestPath, "/responses") {
		// OpenAI Responses API: directly set the "instructions" field
		payload["instructions"] = instructions
		return
	}

	// OpenAI Chat Completions API: inject/replace system message
	messages, ok := payload["messages"].([]interface{})
	if !ok {
		return
	}

	// Look for existing system message
	for i, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		if role == "system" || role == "developer" {
			// Replace existing system/developer message content
			msgMap["content"] = instructions
			messages[i] = msgMap
			payload["messages"] = messages
			return
		}
	}

	// No system message found, prepend one
	systemMsg := map[string]interface{}{
		"role":    "system",
		"content": instructions,
	}
	payload["messages"] = append([]interface{}{systemMsg}, messages...)
}

// GetOriginalModel returns the original model name from context (before mapping)
func GetOriginalModel(c *gin.Context) string {
	if val, exists := c.Get(OriginalModelContextKey); exists {
		if model, ok := val.(string); ok {
			return model
		}
	}
	return ""
}

// GetMappedModel returns the mapped model name from context
func GetMappedModel(c *gin.Context) string {
	if val, exists := c.Get(MappedModelContextKey); exists {
		if model, ok := val.(string); ok {
			return model
		}
	}
	return ""
}

// GetPreferredChannelID returns the preferred channel ID from context.
func GetPreferredChannelID(c *gin.Context) string {
	if val, exists := c.Get(PreferredChannelContextKey); exists {
		if channelID, ok := val.(string); ok {
			return channelID
		}
	}
	return ""
}

// IsModelMappingApplied returns whether model mapping was applied
func IsModelMappingApplied(c *gin.Context) bool {
	if val, exists := c.Get(ModelMappingAppliedKey); exists {
		if applied, ok := val.(bool); ok {
			return applied
		}
	}
	return false
}

// GetThinkingLevel returns the thinking level from context
func GetThinkingLevel(c *gin.Context) string {
	if val, exists := c.Get(ThinkingLevelContextKey); exists {
		if level, ok := val.(string); ok {
			return level
		}
	}
	return ""
}
