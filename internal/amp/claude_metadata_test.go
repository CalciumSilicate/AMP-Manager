package amp

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestEnsureClaudeMetadataUserIDOfficialFormat(t *testing.T) {
	body := []byte(`{"model":"claude-3-7-sonnet","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"world"}]}`)
	ua := "test-agent"
	key := "sk-test"

	out, changed := ensureClaudeMetadataUserID(body, ua, key)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	userIDRaw := gjson.GetBytes(out, "metadata.user_id").String()
	if userIDRaw == "" {
		t.Fatalf("expected metadata.user_id to be set")
	}

	// user_id should be a JSON string containing device_id, account_uuid, session_id
	var parsed struct {
		DeviceID    string `json:"device_id"`
		AccountUUID string `json:"account_uuid"`
		SessionID   string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(userIDRaw), &parsed); err != nil {
		t.Fatalf("user_id is not valid JSON: %v\nraw: %s", err, userIDRaw)
	}

	// device_id should be 64-char hex (SHA256)
	if len(parsed.DeviceID) != 64 {
		t.Fatalf("expected device_id to be 64 hex chars, got %d: %s", len(parsed.DeviceID), parsed.DeviceID)
	}

	// account_uuid should be empty
	if parsed.AccountUUID != "" {
		t.Fatalf("expected account_uuid to be empty, got %q", parsed.AccountUUID)
	}

	// session_id should be UUID format
	if len(parsed.SessionID) != 36 || parsed.SessionID[8] != '-' {
		t.Fatalf("expected session_id in UUID format, got %q", parsed.SessionID)
	}

	// Idempotent: second pass should not modify
	out2, changed2 := ensureClaudeMetadataUserID(out, ua, key)
	if changed2 {
		t.Fatalf("expected changed=false on second pass")
	}
	if string(out2) != string(out) {
		t.Fatalf("expected body unchanged on second pass")
	}
}

func TestEnsureClaudeMetadataUserIDNoopIfPresent(t *testing.T) {
	body := []byte(`{"metadata":{"user_id":"abc"}}`)
	out, changed := ensureClaudeMetadataUserID(body, "ua", "key")
	if changed {
		t.Fatalf("expected changed=false")
	}
	if string(out) != string(body) {
		t.Fatalf("expected body unchanged")
	}
}
