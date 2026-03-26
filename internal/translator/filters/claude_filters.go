package filters

import (
	"ampmanager/internal/translator"
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ClaudeSystemStringFilter converts system string to array format for Claude API
// When system is a string like "hello", converts to [{"type":"text","text":"hello"}]
type ClaudeSystemStringFilter struct{}

func (f *ClaudeSystemStringFilter) Name() string {
	return "claude_system_string_to_array"
}

func (f *ClaudeSystemStringFilter) Applies(outgoingFormat translator.Format) bool {
	return outgoingFormat == translator.FormatClaude
}

func (f *ClaudeSystemStringFilter) Apply(body []byte) ([]byte, bool, error) {
	if !gjson.ValidBytes(body) {
		return body, false, nil
	}

	systemResult := gjson.GetBytes(body, "system")
	if !systemResult.Exists() {
		return body, false, nil
	}

	// Only convert if system is a string
	if systemResult.Type != gjson.String {
		return body, false, nil
	}

	systemText := systemResult.String()

	// If the system string is empty, remove it entirely to avoid
	// Claude API error "text content blocks must be non-empty"
	if strings.TrimSpace(systemText) == "" {
		newBody, err := sjson.DeleteBytes(body, "system")
		if err != nil {
			return body, false, err
		}
		return newBody, true, nil
	}

	// Convert string to array format: [{"type":"text","text":"..."}]
	systemArray := []map[string]string{
		{
			"type": "text",
			"text": systemText,
		},
	}

	newBody, err := sjson.SetBytes(body, "system", systemArray)
	if err != nil {
		return body, false, err
	}

	return newBody, true, nil
}

// ClaudeSystemSanitizeFilter removes empty text blocks from system array.
// Claude API rejects requests with "system: text content blocks must be non-empty".
// This filter runs after all other system modifications to clean up any empty blocks.
type ClaudeSystemSanitizeFilter struct{}

func (f *ClaudeSystemSanitizeFilter) Name() string {
	return "claude_system_sanitize"
}

func (f *ClaudeSystemSanitizeFilter) Applies(outgoingFormat translator.Format) bool {
	return outgoingFormat == translator.FormatClaude
}

func (f *ClaudeSystemSanitizeFilter) Apply(body []byte) ([]byte, bool, error) {
	if !gjson.ValidBytes(body) {
		return body, false, nil
	}

	systemResult := gjson.GetBytes(body, "system")
	if !systemResult.Exists() || !systemResult.IsArray() {
		return body, false, nil
	}

	var items []any
	if err := json.Unmarshal([]byte(systemResult.Raw), &items); err != nil {
		return body, false, nil
	}

	var filtered []any
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		if t, _ := obj["type"].(string); t == "text" {
			if text, _ := obj["text"].(string); strings.TrimSpace(text) == "" {
				continue // skip empty text blocks
			}
		}
		filtered = append(filtered, item)
	}

	if len(filtered) == len(items) {
		return body, false, nil
	}

	if len(filtered) == 0 {
		newBody, err := sjson.DeleteBytes(body, "system")
		if err != nil {
			return body, false, err
		}
		return newBody, true, nil
	}

	newBody, err := sjson.SetBytes(body, "system", filtered)
	if err != nil {
		return body, false, err
	}
	return newBody, true, nil
}

// RegisterClaudeFilters registers all Claude-specific filters.
// NOTE: ClaudeCodeSimulationFilter is NOT registered here — it is applied
// conditionally in channel_router.go only when channel.SimulateCLI is true.
func RegisterClaudeFilters() {
	Register(translator.FormatClaude, &ClaudeSystemStringFilter{})
	Register(translator.FormatClaude, &ClaudeSystemSanitizeFilter{})
}
