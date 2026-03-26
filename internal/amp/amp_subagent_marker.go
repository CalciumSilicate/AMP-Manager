package amp

import (
	"context"
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	log "github.com/sirupsen/logrus"
)

// AmpSubagentInfo holds the Amp thread ID for copilot-api session tracking.
type AmpSubagentInfo struct {
	ThreadID string // from X-Amp-Thread-Id header
}

type ampSubagentInfoKey struct{}

func WithAmpSubagentInfo(ctx context.Context, info *AmpSubagentInfo) context.Context {
	return context.WithValue(ctx, ampSubagentInfoKey{}, info)
}

func GetAmpSubagentInfo(ctx context.Context) *AmpSubagentInfo {
	if v := ctx.Value(ampSubagentInfoKey{}); v != nil {
		if info, ok := v.(*AmpSubagentInfo); ok {
			return info
		}
	}
	return nil
}

// NormalizeUserContentToArray converts string content in user messages to
// array block format so copilot-api's __SUBAGENT_MARKER__ parser can detect them.
//
// Before: {"role":"user","content":"<system-reminder>..."}
// After:  {"role":"user","content":[{"type":"text","text":"<system-reminder>..."}]}
//
// copilot-api's parseSubagentMarkerFromFirstUser only checks Array.isArray(content),
// so string content is invisible to it.
func NormalizeUserContentToArray(body []byte) ([]byte, bool) {
	if !gjson.ValidBytes(body) {
		return body, false
	}

	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body, false
	}

	changed := false
	var result = body

	for i, msg := range messages.Array() {
		role := msg.Get("role")
		content := msg.Get("content")

		if role.String() != "user" {
			continue
		}

		// Only convert if content is a plain string (not already an array)
		if content.Type != gjson.String {
			continue
		}

		text := content.String()
		if text == "" {
			continue
		}

		// Convert to array block format: [{"type":"text","text":"..."}]
		block := []map[string]string{{"type": "text", "text": text}}
		blockJSON, err := json.Marshal(block)
		if err != nil {
			continue
		}

		path := "messages." + itoa(i) + ".content"
		newBody, err := sjson.SetRawBytes(result, path, blockJSON)
		if err != nil {
			continue
		}
		result = newBody
		changed = true
	}

	if changed {
		log.Debugf("copilot-api: normalized user message string content to array blocks")
	}
	return result, changed
}

// itoa converts int to string without importing strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
