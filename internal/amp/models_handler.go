package amp

import (
	"net/http"
	"strings"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// ModelsHandler returns local model list based on provider
// This prevents Amp client from using upstream's context_length (which causes 968k issue)
func ModelsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		provider := strings.ToLower(c.Param("provider"))

		switch provider {
		case "google":
			handleGeminiModels(c)
		case "anthropic":
			handleClaudeModels(c)
		default:
			handleOpenAIModels(c)
		}
	}
}

// handleOpenAIModels returns OpenAI-compatible model list from channel_models
// Auto-detects provider by request headers: anthropic-version → claude, otherwise → openai
func handleOpenAIModels(c *gin.Context) {
	channelModelRepo := repository.NewChannelModelRepository()
	availableModels, err := channelModelRepo.ListAllWithChannel()
	if err != nil {
		log.Warnf("models handler: failed to list channel models: %v", err)
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": []gin.H{}})
		return
	}

	// Detect provider from request headers
	var filterType model.ChannelType
	if c.GetHeader("anthropic-version") != "" {
		filterType = model.ChannelTypeClaude
	} else {
		filterType = model.ChannelTypeOpenAI
	}

	data := make([]gin.H, 0)
	for _, m := range availableModels {
		if m.ChannelType != filterType {
			continue
		}
		if m.ModelWhitelist && !modelMatchesRules(m.ModelID, m.ModelsJSON) {
			continue
		}
		data = append(data, gin.H{
			"id":       m.ModelID,
			"object":   "model",
			"owned_by": string(m.ChannelType),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

// createOpenAIModelsHandler returns a handler for root-level /v1/models endpoint
func createOpenAIModelsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		handleOpenAIModels(c)
	}
}

// handleClaudeModels returns Anthropic-compatible model list from channel_models
func handleClaudeModels(c *gin.Context) {
	channelModelRepo := repository.NewChannelModelRepository()
	availableModels, err := channelModelRepo.ListAllWithChannel()
	if err != nil {
		log.Warnf("models handler: failed to list channel models: %v", err)
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": []gin.H{}})
		return
	}

	data := make([]gin.H, 0)
	for _, m := range availableModels {
		if m.ChannelType != model.ChannelTypeClaude {
			continue
		}
		if m.ModelWhitelist && !modelMatchesRules(m.ModelID, m.ModelsJSON) {
			continue
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = m.ModelID
		}
		data = append(data, gin.H{
			"id":           m.ModelID,
			"object":       "model",
			"display_name": displayName,
			"owned_by":     "anthropic",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

// createGeminiModelsHandler returns a handler for root-level /v1beta/models endpoint
func createGeminiModelsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		handleGeminiModels(c)
	}
}

// handleGeminiModels returns Gemini-compatible model list from channel_models
func handleGeminiModels(c *gin.Context) {
	channelModelRepo := repository.NewChannelModelRepository()
	availableModels, err := channelModelRepo.ListAllWithChannel()
	if err != nil {
		log.Warnf("models handler: failed to list channel models: %v", err)
		c.JSON(http.StatusOK, gin.H{"models": []gin.H{}})
		return
	}

	data := make([]gin.H, 0)
	for _, m := range availableModels {
		if m.ChannelType != model.ChannelTypeGemini {
			continue
		}
		if m.ModelWhitelist && !modelMatchesRules(m.ModelID, m.ModelsJSON) {
			continue
		}
		modelID := m.ModelID
		if !strings.HasPrefix(modelID, "models/") {
			modelID = "models/" + modelID
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = m.ModelID
		}

		data = append(data, gin.H{
			"name":                       modelID,
			"displayName":                displayName,
			"supportedGenerationMethods": []string{"generateContent", "streamGenerateContent"},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"models": data,
	})
}

// modelMatchesRules checks if a model ID matches any of the channel's model rules
func modelMatchesRules(modelID string, modelsJSON string) bool {
	return service.MatchChannelModelRules(modelID, modelsJSON)
}

// wildcardMatch supports simple * wildcard matching
func wildcardMatch(pattern, text string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(text, strings.Trim(pattern, "*"))
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(text, strings.TrimPrefix(pattern, "*"))
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(text, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == text
}

// modelInfo holds model metadata for handler responses
type modelInfo struct {
	ModelPattern        string
	DisplayName         string
	ContextLength       int
	MaxCompletionTokens int
	Provider            string
}

// getModelsForProvider returns models filtered by provider
func getModelsForProvider(provider string) []modelInfo {
	repo := repository.NewModelMetadataRepository()
	allModels, err := repo.List()
	if err != nil {
		log.Warnf("models handler: failed to list models: %v", err)
		return getBuiltinModelsForProvider(provider)
	}

	var result []modelInfo
	for _, m := range allModels {
		if strings.EqualFold(m.Provider, provider) {
			result = append(result, modelInfo{
				ModelPattern:        m.ModelPattern,
				DisplayName:         m.DisplayName,
				ContextLength:       m.ContextLength,
				MaxCompletionTokens: m.MaxCompletionTokens,
				Provider:            m.Provider,
			})
		}
	}

	if len(result) == 0 {
		return getBuiltinModelsForProvider(provider)
	}

	log.Debugf("models handler: returning %d models for provider %s", len(result), provider)
	return result
}

// getBuiltinModelsForProvider returns hardcoded models as fallback
func getBuiltinModelsForProvider(provider string) []modelInfo {
	builtins := GetBuiltinModelMetadata()
	var result []modelInfo

	for name, meta := range builtins {
		modelProvider := inferProvider(name)
		if strings.EqualFold(modelProvider, provider) {
			result = append(result, modelInfo{
				ModelPattern:        name,
				DisplayName:         name,
				ContextLength:       meta.ContextLength,
				MaxCompletionTokens: meta.MaxCompletionTokens,
				Provider:            modelProvider,
			})
		}
	}

	return result
}

// inferProvider guesses provider from model name
func inferProvider(modelName string) string {
	name := strings.ToLower(modelName)
	switch {
	case strings.HasPrefix(name, "claude"):
		return "anthropic"
	case strings.HasPrefix(name, "gemini"):
		return "google"
	case strings.HasPrefix(name, "gpt"):
		return "openai"
	case strings.HasPrefix(name, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(name, "qwen"):
		return "alibaba"
	default:
		return "openai"
	}
}
