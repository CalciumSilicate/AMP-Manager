package amp

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestPrefixAndUnprefixClaudeToolNamesWithMap(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-7-sonnet",
		"tools":[{"name":"webSearch2"},{"name":"mcp__context7__query_docs"}],
		"messages":[{"role":"user","content":[{"type":"tool_use","name":"extractWebPageContent"},{"type":"tool_use","name":"mcp__context7__query_docs"}]}]
	}`)

	prefixed, m, changed := PrefixClaudeToolNamesWithMap(body)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if gjson.GetBytes(prefixed, "tools.0.name").String() != "mcp__tools__mcp-websearch2" {
		t.Fatalf("expected tools[0] prefixed, got %q", gjson.GetBytes(prefixed, "tools.0.name").String())
	}
	if gjson.GetBytes(prefixed, "tools.1.name").String() != "mcp__context7__query_docs" {
		t.Fatalf("expected tools[1] unchanged, got %q", gjson.GetBytes(prefixed, "tools.1.name").String())
	}
	if gjson.GetBytes(prefixed, "messages.0.content.0.name").String() != "mcp__tools__mcp-extractwebpagecontent" {
		t.Fatalf("expected content[0] prefixed, got %q", gjson.GetBytes(prefixed, "messages.0.content.0.name").String())
	}
	if gjson.GetBytes(prefixed, "messages.0.content.1.name").String() != "mcp__context7__query_docs" {
		t.Fatalf("expected content[1] unchanged, got %q", gjson.GetBytes(prefixed, "messages.0.content.1.name").String())
	}
	if m["mcp__tools__mcp-websearch2"] != "webSearch2" {
		t.Fatalf("missing reverse map for webSearch2")
	}
	if _, ok := m["mcp__context7__query_docs"]; ok {
		t.Fatalf("should not map already-prefixed tool")
	}

	unprefixed, uChanged := UnprefixClaudeToolNamesWithMap(prefixed, m)
	if !uChanged {
		t.Fatalf("expected unprefix changed=true")
	}
	if gjson.GetBytes(unprefixed, "messages.0.content.0.name").String() != "extractWebPageContent" {
		t.Fatalf("expected unprefixed tool_use name")
	}
	if gjson.GetBytes(unprefixed, "messages.0.content.1.name").String() != "mcp__context7__query_docs" {
		t.Fatalf("expected original mcp__context7__query_docs untouched")
	}
}

func TestPrefixSkipsBuiltinTools(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-7-sonnet",
		"tools":[
			{"type":"web_search_20250305","name":"web_search"},
			{"type":"code_execution_20250522","name":"code_execution"},
			{"type":"text_editor_20250429","name":"text_editor"},
			{"type":"computer_20250124","name":"computer"},
			{"name":"myCustomTool"}
		],
		"messages":[{"role":"user","content":[
			{"type":"tool_use","name":"web_search"},
			{"type":"tool_use","name":"myCustomTool"}
		]}]
	}`)

	prefixed, m, changed := PrefixClaudeToolNamesWithMap(body)
	if !changed {
		t.Fatalf("expected changed=true (myCustomTool should be prefixed)")
	}

	// Built-in tools must remain unchanged
	if got := gjson.GetBytes(prefixed, "tools.0.name").String(); got != "web_search" {
		t.Fatalf("expected web_search unchanged, got %q", got)
	}
	if got := gjson.GetBytes(prefixed, "tools.1.name").String(); got != "code_execution" {
		t.Fatalf("expected code_execution unchanged, got %q", got)
	}
	if got := gjson.GetBytes(prefixed, "tools.2.name").String(); got != "text_editor" {
		t.Fatalf("expected text_editor unchanged, got %q", got)
	}
	if got := gjson.GetBytes(prefixed, "tools.3.name").String(); got != "computer" {
		t.Fatalf("expected computer unchanged, got %q", got)
	}

	// Custom tool should be prefixed
	if got := gjson.GetBytes(prefixed, "tools.4.name").String(); got != "mcp__tools__mcp-mycustomtool" {
		t.Fatalf("expected mcp__tools__mcp-mycustomtool, got %q", got)
	}

	// Built-in tool names in messages should NOT be prefixed
	if got := gjson.GetBytes(prefixed, "messages.0.content.0.name").String(); got != "web_search" {
		t.Fatalf("expected web_search in message unchanged, got %q", got)
	}
	// Custom tool names in messages should be prefixed
	if got := gjson.GetBytes(prefixed, "messages.0.content.1.name").String(); got != "mcp__tools__mcp-mycustomtool" {
		t.Fatalf("expected mcp__tools__mcp-mycustomtool in message, got %q", got)
	}

	// Reverse map should only contain custom tools
	if _, ok := m["mcp__tools__mcp-web-search"]; ok {
		t.Fatalf("reverse map should not contain built-in tool web_search")
	}
	if m["mcp__tools__mcp-mycustomtool"] != "myCustomTool" {
		t.Fatalf("reverse map missing myCustomTool")
	}
}
