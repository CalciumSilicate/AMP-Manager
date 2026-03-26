package filters

import (
	"ampmanager/internal/translator"

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

	// Convert string to array format: [{"type":"text","text":"..."}]
	systemText := systemResult.String()
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

// RegisterClaudeFilters registers all Claude-specific filters.
// NOTE: ClaudeCodeSimulationFilter is NOT registered here — it is applied
// conditionally in channel_router.go only when channel.SimulateCLI is true.
func RegisterClaudeFilters() {
	Register(translator.FormatClaude, &ClaudeSystemStringFilter{})
}
