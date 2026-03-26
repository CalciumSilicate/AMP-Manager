package amp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
)

// ampSessionDetectRE matches __AMP_SESSION__:<thread-id> in raw JSON bytes.
// Thread IDs are T-{uuid} so we match T- followed by alphanumerics and hyphens.
var ampSessionDetectRE = regexp.MustCompile(`__AMP_SESSION__:(T-[a-zA-Z0-9-]+)`)

// ampSessionLineRE matches the full __AMP_SESSION__ line in decoded text (after JSON parsing).
var ampSessionLineRE = regexp.MustCompile(`(?m)^__AMP_SESSION__:.+$\n?`)

// copilotAPIMarkerPrefix is the marker prefix copilot-api recognises.
const copilotAPIMarkerPrefix = "__SUBAGENT_MARKER__"

// AmpSubagentInfo holds Amp session/subagent metadata extracted from the request.
type AmpSubagentInfo struct {
	RootSessionID string // root thread ID (from __AMP_SESSION__ marker)
	ThreadID      string // current thread ID (from X-Amp-Thread-Id header)
	IsSubagent    bool   // true when __AMP_SESSION__ marker was found
}

type ampSubagentInfoKey struct{}

// WithAmpSubagentInfo stores AmpSubagentInfo in context.
func WithAmpSubagentInfo(ctx context.Context, info *AmpSubagentInfo) context.Context {
	return context.WithValue(ctx, ampSubagentInfoKey{}, info)
}

// GetAmpSubagentInfo retrieves AmpSubagentInfo from context.
func GetAmpSubagentInfo(ctx context.Context) *AmpSubagentInfo {
	if v := ctx.Value(ampSubagentInfoKey{}); v != nil {
		if info, ok := v.(*AmpSubagentInfo); ok {
			return info
		}
	}
	return nil
}

// ParseAmpSubagentMarker extracts Amp session info from request body and header.
// threadID comes from the X-Amp-Thread-Id request header.
func ParseAmpSubagentMarker(body []byte, threadID string) *AmpSubagentInfo {
	info := &AmpSubagentInfo{
		ThreadID: threadID,
	}

	if match := ampSessionDetectRE.FindSubmatch(body); match != nil {
		info.RootSessionID = strings.TrimSpace(string(match[1]))
		info.IsSubagent = true
	} else if threadID != "" {
		// No subagent marker — this is a root thread request
		info.RootSessionID = threadID
	}

	return info
}

// ConvertAmpToCopilotAPIFormat converts Amp's __AMP_SESSION__ marker in the
// Anthropic Messages body to copilot-api's __SUBAGENT_MARKER__ format wrapped
// in <system-reminder> tags. Returns modified body and whether conversion occurred.
func ConvertAmpToCopilotAPIFormat(body []byte, info *AmpSubagentInfo) ([]byte, bool) {
	if !info.IsSubagent || len(body) == 0 {
		return body, false
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, false
	}

	messagesRaw, ok := payload["messages"]
	if !ok {
		return body, false
	}

	var messages []json.RawMessage
	if err := json.Unmarshal(messagesRaw, &messages); err != nil {
		return body, false
	}

	converted := false
	for i, msgRaw := range messages {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(msgRaw, &msg); err != nil || msg.Role != "user" {
			continue
		}

		newContent, ok := convertUserMessageContent(msg.Content, info)
		if !ok {
			continue
		}

		// Re-serialise preserving all original fields
		var fullMsg map[string]json.RawMessage
		if err := json.Unmarshal(msgRaw, &fullMsg); err != nil {
			continue
		}
		fullMsg["content"] = newContent
		newMsgRaw, err := json.Marshal(fullMsg)
		if err != nil {
			continue
		}
		messages[i] = newMsgRaw
		converted = true
		break // only first user message
	}

	if !converted {
		return body, false
	}

	newMessagesJSON, err := json.Marshal(messages)
	if err != nil {
		return body, false
	}
	payload["messages"] = newMessagesJSON

	newBody, err := json.Marshal(payload)
	if err != nil {
		return body, false
	}

	log.Debugf("amp-marker: converted __AMP_SESSION__ -> __SUBAGENT_MARKER__ (root=%s, thread=%s)", info.RootSessionID, info.ThreadID)
	return newBody, true
}

// convertUserMessageContent handles both string and array content formats.
func convertUserMessageContent(contentRaw json.RawMessage, info *AmpSubagentInfo) (json.RawMessage, bool) {
	// Try array of content blocks first
	var blocks []map[string]interface{}
	if err := json.Unmarshal(contentRaw, &blocks); err == nil {
		return convertContentBlocks(blocks, info)
	}

	// Try plain string
	var text string
	if err := json.Unmarshal(contentRaw, &text); err == nil {
		if !ampSessionDetectRE.MatchString(text) {
			return nil, false
		}
		newText := replaceAmpMarkerWithCopilotAPI(text, info)
		result, err := json.Marshal(newText)
		if err != nil {
			return nil, false
		}
		return result, true
	}

	return nil, false
}

// convertContentBlocks processes an array of Anthropic content blocks.
func convertContentBlocks(blocks []map[string]interface{}, info *AmpSubagentInfo) (json.RawMessage, bool) {
	converted := false
	for i, block := range blocks {
		blockType, _ := block["type"].(string)
		if blockType != "text" {
			continue
		}
		text, _ := block["text"].(string)
		if !ampSessionDetectRE.MatchString(text) {
			continue
		}
		blocks[i]["text"] = replaceAmpMarkerWithCopilotAPI(text, info)
		converted = true
		break
	}
	if !converted {
		return nil, false
	}
	result, err := json.Marshal(blocks)
	if err != nil {
		return nil, false
	}
	return result, true
}

// replaceAmpMarkerWithCopilotAPI strips __AMP_SESSION__ and prepends __SUBAGENT_MARKER__ in <system-reminder>.
func replaceAmpMarkerWithCopilotAPI(text string, info *AmpSubagentInfo) string {
	// Strip the __AMP_SESSION__:<root-id> line
	text = ampSessionLineRE.ReplaceAllString(text, "")
	text = strings.TrimLeft(text, "\n")

	// Build copilot-api format marker (matches opencode plugin output)
	markerPayload, _ := json.Marshal(map[string]string{
		"session_id": info.RootSessionID,
		"agent_id":   info.ThreadID,
		"agent_type": "amp-subagent",
	})

	marker := fmt.Sprintf(
		"<system-reminder>\nSubagentStart hook additional context: %s %s\n</system-reminder>",
		copilotAPIMarkerPrefix,
		string(markerPayload),
	)

	return marker + "\n" + text
}
