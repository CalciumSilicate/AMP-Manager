package amp

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestApplyClaudeCodeSystemPrompt_Basic(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-7-sonnet",
		"system":[{"type":"text","text":"You are a helpful assistant."},{"type":"text","text":"Be concise."}],
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello"}]}
		]
	}`)

	out := applyClaudeCodeSystemPrompt(body)
	if !json.Valid(out) {
		t.Fatalf("output is not valid JSON")
	}

	// System should now be the official Claude Code system prompt
	system := gjson.GetBytes(out, "system")
	if !system.IsArray() {
		t.Fatalf("expected system to be array, got %s", system.Type)
	}
	firstSystemText := gjson.GetBytes(out, "system.0.text").String()
	if firstSystemText != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Fatalf("unexpected first system text: %q", firstSystemText)
	}

	// First user message should now have system-reminder blocks prepended
	content := gjson.GetBytes(out, "messages.0.content")
	if !content.IsArray() {
		t.Fatalf("expected content to be array")
	}
	// Should be 3 items: 2 reminder blocks + original "hello"
	if content.Get("#").Int() != 3 {
		t.Fatalf("expected 3 content items, got %d", content.Get("#").Int())
	}

	// Check first reminder block wraps original system text
	first := content.Get("0.text").String()
	if first == "" {
		t.Fatalf("expected first content block to have text")
	}
	if !contains(first, "<system-reminder>") || !contains(first, "You are a helpful assistant.") {
		t.Fatalf("expected first block to contain system-reminder with original text, got %q", first)
	}

	// Check second reminder block
	second := content.Get("1.text").String()
	if !contains(second, "<system-reminder>") || !contains(second, "Be concise.") {
		t.Fatalf("expected second block to contain system-reminder with original text, got %q", second)
	}

	// Original user text should be last
	last := content.Get("2.text").String()
	if last != "hello" {
		t.Fatalf("expected last content to be 'hello', got %q", last)
	}
}

func TestApplyClaudeCodeSystemPrompt_StringSystem(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-7-sonnet",
		"system":"You are a helpful assistant.",
		"messages":[
			{"role":"user","content":"hello world"}
		]
	}`)

	out := applyClaudeCodeSystemPrompt(body)

	// System replaced
	firstSystemText := gjson.GetBytes(out, "system.0.text").String()
	if firstSystemText != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Fatalf("unexpected first system text: %q", firstSystemText)
	}

	// String content should be converted to array with reminder prepended
	content := gjson.GetBytes(out, "messages.0.content")
	if !content.IsArray() {
		t.Fatalf("expected content to be array")
	}
	if content.Get("#").Int() != 2 {
		t.Fatalf("expected 2 content items, got %d", content.Get("#").Int())
	}
	if !contains(content.Get("0.text").String(), "You are a helpful assistant.") {
		t.Fatalf("expected reminder block with original system text")
	}
	if content.Get("1.text").String() != "hello world" {
		t.Fatalf("expected original user text preserved")
	}
}

func TestApplyClaudeCodeSystemPrompt_NoSystem(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-7-sonnet",
		"messages":[{"role":"user","content":"hello"}]
	}`)

	out := applyClaudeCodeSystemPrompt(body)

	// System should be set to official prompt even without original
	firstSystemText := gjson.GetBytes(out, "system.0.text").String()
	if firstSystemText != "You are Claude Code, Anthropic's official CLI for Claude." {
		t.Fatalf("unexpected first system text: %q", firstSystemText)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
