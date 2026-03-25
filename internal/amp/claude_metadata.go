package amp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var claudeFieldOrder = []string{
	"model",
	"system",
	"messages",
	"tools",
	"metadata",
	"max_tokens",
	"temperature",
	"top_p",
	"top_k",
	"thinking",
	"stream",
}

func ensureClaudeMetadataUserID(body []byte, userAgent string, apiKey string) ([]byte, bool) {
	if len(body) == 0 {
		return body, false
	}
	if !gjson.ValidBytes(body) {
		return body, false
	}

	existing := gjson.GetBytes(body, "metadata.user_id")
	if existing.Exists() && strings.TrimSpace(existing.String()) != "" {
		return body, false
	}

	deviceID := generateClaudeDeviceID(userAgent, apiKey)
	sessionID := generateClaudeSessionUUID(gjson.GetBytes(body, "messages"))

	// Official format: JSON string with device_id, account_uuid, session_id
	userIDObj := map[string]string{
		"device_id":    deviceID,
		"account_uuid": "",
		"session_id":   sessionID,
	}
	userIDJSON, _ := json.Marshal(userIDObj)
	userID := string(userIDJSON)

	if out, ok := injectClaudeMetadataWithOrder(body, userID); ok {
		return out, true
	}

	// Fallback (should be rare): inject without reordering.
	newBody, err := sjson.SetBytes(body, "metadata.user_id", userID)
	if err != nil {
		return body, false
	}
	return newBody, true
}

// generateClaudeDeviceID generates a stable device fingerprint: SHA256(api_key + ":" + UA).
func generateClaudeDeviceID(userAgent string, apiKey string) string {
	ua := userAgent
	if ua == "" {
		ua = "unknown"
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", apiKey, ua)))
	return hex.EncodeToString(sum[:])
}

// generateClaudeSessionUUID matches amp_processor.rs:
// - with content: SHA256(first 3 messages text) -> UUID-ish format
// - empty content: random uuid v4
func generateClaudeSessionUUID(messages gjson.Result) string {
	var parts []string
	arr := messages.Array()
	if len(arr) > 3 {
		arr = arr[:3]
	}
	for _, m := range arr {
		c := m.Get("content")
		if c.Type == gjson.String {
			parts = append(parts, c.String())
			continue
		}
		if c.IsArray() {
			var b strings.Builder
			for _, item := range c.Array() {
				if item.Get("type").String() == "text" {
					text := item.Get("text").String()
					b.WriteString(text)
				}
			}
			parts = append(parts, b.String())
		}
	}

	content := strings.Join(parts, "|")
	if strings.TrimSpace(content) == "" {
		return uuid.NewString()
	}

	sum := sha256.Sum256([]byte(content))
	hex32 := hex.EncodeToString(sum[:])[:32]
	return fmt.Sprintf(
		"%s-%s-%s-%s-%s",
		hex32[0:8],
		hex32[8:12],
		hex32[12:16],
		hex32[16:20],
		hex32[20:32],
	)
}

func injectClaudeMetadataWithOrder(body []byte, userID string) ([]byte, bool) {
	if userID == "" {
		return body, false
	}
	if !json.Valid(body) {
		return body, false
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return body, false
	}

	used := make(map[string]struct{}, len(obj)+1)
	out := &bytes.Buffer{}
	out.WriteByte('{')
	first := true

	writeKV := func(k string, v []byte) {
		if !first {
			out.WriteByte(',')
		}
		first = false
		keyJSON, _ := json.Marshal(k)
		out.Write(keyJSON)
		out.WriteByte(':')
		out.Write(v)
		used[k] = struct{}{}
	}

	buildMetadata := func(raw json.RawMessage) []byte {
		if len(raw) == 0 {
			b, _ := json.Marshal(map[string]any{"user_id": userID})
			return b
		}
		var meta map[string]json.RawMessage
		if err := json.Unmarshal(raw, &meta); err != nil {
			b, _ := json.Marshal(map[string]any{"user_id": userID})
			return b
		}
		if _, ok := meta["user_id"]; !ok {
			b, _ := json.Marshal(userID)
			meta["user_id"] = b
		}
		b, _ := json.Marshal(meta)
		return b
	}

	for _, k := range claudeFieldOrder {
		if k == "metadata" {
			raw := obj["metadata"]
			writeKV("metadata", buildMetadata(raw))
			continue
		}
		if v, ok := obj[k]; ok {
			writeKV(k, v)
		}
	}

	for k, v := range obj {
		if _, ok := used[k]; ok {
			continue
		}
		writeKV(k, v)
	}

	out.WriteByte('}')
	return out.Bytes(), true
}
