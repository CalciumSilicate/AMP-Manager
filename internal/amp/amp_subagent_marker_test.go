package amp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAmpSubagentInfo_ContextRoundtrip(t *testing.T) {
	info := &AmpSubagentInfo{ThreadID: "T-019d2a44-f848-7136-86cd-06e8bc8903ed", RootSessionID: "T-root"}
	ctx := WithAmpSubagentInfo(context.Background(), info)
	got := GetAmpSubagentInfo(ctx)
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.ThreadID != info.ThreadID || got.RootSessionID != info.RootSessionID {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestAmpSubagentInfo_ContextMissing(t *testing.T) {
	if GetAmpSubagentInfo(context.Background()) != nil {
		t.Fatal("expected nil")
	}
}

func TestParseAmpSubagentMarker_Subagent(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"__AMP_SESSION__:T-root-123\nDo something"}]}]}`)
	info := ParseAmpSubagentMarker(body, "T-sub-456")

	if !info.IsSubagent {
		t.Fatal("expected IsSubagent=true")
	}
	if info.RootSessionID != "T-root-123" {
		t.Fatalf("expected RootSessionID=T-root-123, got %s", info.RootSessionID)
	}
	if info.ThreadID != "T-sub-456" {
		t.Fatalf("expected ThreadID=T-sub-456, got %s", info.ThreadID)
	}
}

func TestParseAmpSubagentMarker_RootRequest(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	info := ParseAmpSubagentMarker(body, "T-thread-1")

	if info.IsSubagent {
		t.Fatal("expected IsSubagent=false for root request")
	}
	if info.RootSessionID != "T-thread-1" {
		t.Fatalf("expected RootSessionID=T-thread-1 (fallback to threadID), got %s", info.RootSessionID)
	}
}

func TestConvertAmpToCopilotAPIFormat_ArrayContent(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":[{"type":"text","text":"__AMP_SESSION__:T-root-abc\nPlease do the task"}]}],"stream":true}`)
	info := &AmpSubagentInfo{
		RootSessionID: "T-root-abc",
		ThreadID:      "T-sub-def",
		IsSubagent:    true,
	}

	newBody, converted := ConvertAmpToCopilotAPIFormat(body, info)
	if !converted {
		t.Fatal("expected conversion to occur")
	}

	// Verify __AMP_SESSION__ is stripped from raw JSON
	if strings.Contains(string(newBody), "__AMP_SESSION__") {
		t.Fatal("__AMP_SESSION__ should be stripped from output")
	}

	// Verify __SUBAGENT_MARKER__ is injected
	if !strings.Contains(string(newBody), copilotAPIMarkerPrefix) {
		t.Fatal("__SUBAGENT_MARKER__ should be present in output")
	}

	// Decode and verify content
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
		t.Fatalf("output should be valid JSON: %v", err)
	}

	firstText := parsed.Messages[0].Content[0].Text

	if !strings.Contains(firstText, "<system-reminder>") {
		t.Fatal("<system-reminder> wrapper should be present")
	}
	if !strings.Contains(firstText, "Please do the task") {
		t.Fatal("original prompt text should be preserved")
	}
	if !strings.Contains(firstText, `"session_id":"T-root-abc"`) {
		t.Fatal("marker should contain root session ID")
	}
	if !strings.Contains(firstText, `"agent_type":"amp-subagent"`) {
		t.Fatal("marker should contain agent_type")
	}
}

func TestConvertAmpToCopilotAPIFormat_StringContent(t *testing.T) {
	body := []byte(`{"model":"test","messages":[{"role":"user","content":"__AMP_SESSION__:T-root\nHello world"}]}`)
	info := &AmpSubagentInfo{
		RootSessionID: "T-root",
		ThreadID:      "T-sub",
		IsSubagent:    true,
	}

	newBody, converted := ConvertAmpToCopilotAPIFormat(body, info)
	if !converted {
		t.Fatal("expected conversion for string content")
	}
	if strings.Contains(string(newBody), "__AMP_SESSION__") {
		t.Fatal("__AMP_SESSION__ should be stripped")
	}
	if !strings.Contains(string(newBody), copilotAPIMarkerPrefix) {
		t.Fatal("__SUBAGENT_MARKER__ should be injected")
	}
}

func TestConvertAmpToCopilotAPIFormat_NotSubagent(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	info := &AmpSubagentInfo{
		RootSessionID: "T-thread",
		ThreadID:      "T-thread",
		IsSubagent:    false,
	}

	_, converted := ConvertAmpToCopilotAPIFormat(body, info)
	if converted {
		t.Fatal("should not convert when IsSubagent=false")
	}
}

func TestConvertAmpToCopilotAPIFormat_SkipsAssistantMessages(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":"I am ready"},{"role":"user","content":[{"type":"text","text":"__AMP_SESSION__:T-root\nTask prompt"}]}]}`)
	info := &AmpSubagentInfo{
		RootSessionID: "T-root",
		ThreadID:      "T-sub",
		IsSubagent:    true,
	}

	newBody, converted := ConvertAmpToCopilotAPIFormat(body, info)
	if !converted {
		t.Fatal("expected conversion")
	}
	if !strings.Contains(string(newBody), "I am ready") {
		t.Fatal("assistant message should be preserved")
	}
	if !strings.Contains(string(newBody), copilotAPIMarkerPrefix) {
		t.Fatal("marker should be in user message")
	}
}
