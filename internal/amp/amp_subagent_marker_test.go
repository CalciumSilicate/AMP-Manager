package amp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAmpSubagentInfo_ContextRoundtrip(t *testing.T) {
	info := &AmpSubagentInfo{ThreadID: "T-019d2a44-f848-7136-86cd-06e8bc8903ed"}
	ctx := WithAmpSubagentInfo(context.Background(), info)
	got := GetAmpSubagentInfo(ctx)
	if got == nil || got.ThreadID != info.ThreadID {
		t.Fatal("roundtrip mismatch")
	}
}

func TestAmpSubagentInfo_ContextMissing(t *testing.T) {
	if GetAmpSubagentInfo(context.Background()) != nil {
		t.Fatal("expected nil")
	}
}

func TestNormalizeUserContentToArray_StringContent(t *testing.T) {
	body := []byte(`{"model":"test","messages":[{"role":"user","content":"hello world"}]}`)

	newBody, changed := NormalizeUserContentToArray(body)
	if !changed {
		t.Fatal("expected normalization")
	}

	// Parse and verify structure
	var parsed struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(newBody, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(parsed.Messages))
	}
	if len(parsed.Messages[0].Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(parsed.Messages[0].Content))
	}
	block := parsed.Messages[0].Content[0]
	if block.Type != "text" || block.Text != "hello world" {
		t.Fatalf("unexpected block: %+v", block)
	}
}

func TestNormalizeUserContentToArray_AlreadyArray(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"already array"}]}]}`)

	_, changed := NormalizeUserContentToArray(body)
	if changed {
		t.Fatal("should not modify already-array content")
	}
}

func TestNormalizeUserContentToArray_AssistantUntouched(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":"I am ready"},{"role":"user","content":"do it"}]}`)

	newBody, changed := NormalizeUserContentToArray(body)
	if !changed {
		t.Fatal("expected user message normalization")
	}

	// Verify assistant message is still a string
	var raw map[string]json.RawMessage
	json.Unmarshal(newBody, &raw)
	var msgs []json.RawMessage
	json.Unmarshal(raw["messages"], &msgs)

	var assistantMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(msgs[0], &assistantMsg); err != nil {
		t.Fatal("assistant message should still have string content")
	}
	if assistantMsg.Content != "I am ready" {
		t.Fatalf("unexpected assistant content: %s", assistantMsg.Content)
	}
}

func TestNormalizeUserContentToArray_SubagentMarkerVisible(t *testing.T) {
	// Simulate what Amp sends after plugin injection
	marker := `<system-reminder>\nSubagentStart hook additional context: __SUBAGENT_MARKER__ {"session_id":"T-abc","agent_id":"T-abc","agent_type":"amp-subagent"}\n</system-reminder>\nDo the task`
	body := []byte(`{"messages":[{"role":"user","content":` + mustJSON(marker) + `}]}`)

	newBody, changed := NormalizeUserContentToArray(body)
	if !changed {
		t.Fatal("expected normalization")
	}

	// Verify the marker is in an array text block (copilot-api can now parse it)
	var parsed struct {
		Messages []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(newBody, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.Messages[0].Content[0].Type != "text" {
		t.Fatal("expected text block")
	}
	if text := parsed.Messages[0].Content[0].Text; text == "" {
		t.Fatal("text should not be empty")
	}
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
