package amp

import (
	"context"
	"encoding/json"

	"ampmanager/internal/translator"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// suppressThinkingIfToolUse suppresses thinking blocks when tool_use is detected
// Amp client has rendering issues when it sees both thinking and tool_use blocks
func suppressThinkingIfToolUse(data []byte) []byte {
	// Check if tool_use exists
	if !gjson.GetBytes(data, `content.#(type=="tool_use")`).Exists() {
		return data
	}

	// Filter out thinking and redacted_thinking blocks
	filtered := gjson.GetBytes(data, `content.#(type!="thinking")#`)
	if !filtered.Exists() {
		return data
	}

	// Also filter redacted_thinking
	result := filtered.Value()
	if arr, ok := result.([]interface{}); ok {
		var newArr []interface{}
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["type"].(string); ok && t == "redacted_thinking" {
					continue
				}
			}
			newArr = append(newArr, item)
		}
		result = newArr
	}

	originalCount := gjson.GetBytes(data, "content.#").Int()
	if arr, ok := result.([]interface{}); ok && int64(len(arr)) < originalCount {
		newData, err := sjson.SetBytes(data, "content", result)
		if err != nil {
			log.Warnf("response rewriter: failed to suppress thinking blocks: %v", err)
			return data
		}
		log.Debugf("response rewriter: suppressed %d thinking blocks due to tool usage", originalCount-int64(len(arr)))
		return newData
	}

	return data
}

// TransformResponseJSON applies safe response-side transformations to a single JSON payload.
// It intentionally only rewrites well-known metadata fields so user-visible content is left intact.
func TransformResponseJSON(ctx context.Context, data []byte, originalModel, mappedModel string) []byte {
	info := getProviderInfoOrDefault(ctx)
	if ctx != nil && info.Provider == ProviderAnthropic {
		if toolMap, ok := GetClaudeToolNameMap(ctx); ok && len(toolMap) > 0 {
			if unprefixed, changed := UnprefixClaudeToolNamesWithMap(data, toolMap); changed {
				data = unprefixed
			}
		}
	}

	return RewriteModelInResponseDataWithProvider(data, originalModel, mappedModel, info)
}

func TransformResponseJSONForFormat(ctx context.Context, data []byte, originalModel, mappedModel string, format translator.Format) []byte {
	info := providerInfoForTranslatorFormat(format)
	if ctx != nil && info.Provider == ProviderAnthropic {
		if toolMap, ok := GetClaudeToolNameMap(ctx); ok && len(toolMap) > 0 {
			if unprefixed, changed := UnprefixClaudeToolNamesWithMap(data, toolMap); changed {
				data = unprefixed
			}
		}
	}
	return RewriteModelInResponseDataWithProvider(data, originalModel, mappedModel, info)
}

// RewriteModelInResponseData rewrites model names in JSON response metadata fields only.
// It preserves free-form text, tool inputs, and other nested content by restricting writes
// to known protocol metadata paths.
func RewriteModelInResponseData(data []byte, originalModel, mappedModel string) []byte {
	return RewriteModelInResponseDataWithProvider(data, originalModel, mappedModel, ProviderInfo{})
}

func RewriteModelInResponseDataWithProvider(data []byte, originalModel, mappedModel string, info ProviderInfo) []byte {
	data = suppressThinkingIfToolUse(data)

	if originalModel == "" || mappedModel == "" || originalModel == mappedModel {
		return data
	}
	if !json.Valid(data) {
		return data
	}

	paths := modelRewritePaths(info)
	if len(paths) == 0 {
		return data
	}

	rewritten := data
	for _, path := range paths {
		value := gjson.GetBytes(rewritten, path)
		if !value.Exists() || value.Type != gjson.String || value.String() != mappedModel {
			continue
		}

		updated, err := sjson.SetBytes(rewritten, path, originalModel)
		if err != nil {
			log.Warnf("response rewriter: failed to rewrite model metadata at %s: %v", path, err)
			continue
		}
		rewritten = updated
	}

	return rewritten
}

func getProviderInfoOrDefault(ctx context.Context) ProviderInfo {
	if ctx == nil {
		return ProviderInfo{}
	}
	if info, ok := GetProviderInfo(ctx); ok {
		return info
	}
	return ProviderInfo{}
}

func modelRewritePaths(info ProviderInfo) []string {
	switch info.Provider {
	case ProviderAnthropic:
		return []string{"model", "message.model"}
	case ProviderOpenAIChat:
		return []string{"model"}
	case ProviderOpenAIResponses:
		return []string{"model", "response.model"}
	case ProviderGemini:
		return nil
	default:
		return []string{"model", "message.model", "response.model"}
	}
}

func providerInfoForTranslatorFormat(format translator.Format) ProviderInfo {
	switch {
	case translator.Equivalent(format, translator.FormatClaude):
		return ProviderInfo{Provider: ProviderAnthropic}
	case translator.Equivalent(format, translator.FormatOpenAIResponses):
		return ProviderInfo{Provider: ProviderOpenAIResponses}
	case translator.Equivalent(format, translator.FormatOpenAIChat), translator.Equivalent(format, translator.FormatOpenAI):
		return ProviderInfo{Provider: ProviderOpenAIChat}
	case translator.Equivalent(format, translator.FormatGemini):
		return ProviderInfo{Provider: ProviderGemini}
	default:
		return ProviderInfo{}
	}
}
