package filters

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ampmanager/internal/translator"

	"github.com/tidwall/gjson"
)

// --- Basic filter tests ---

func TestClaudeCodeSimulationFilterApplies(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	if !f.Applies(translator.FormatClaude) {
		t.Fatalf("expected filter to apply to claude format")
	}
}

func TestClaudeCodeSimulationFilterSkipsHaiku(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{"model":"claude-3-haiku","tools":[{"name":"webSearch2"}]}`)

	out, changed, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if changed {
		t.Fatalf("expected changed=false")
	}
	if string(out) != string(body) {
		t.Fatalf("expected body unchanged")
	}
}

// --- P0: thinking ---

func TestThinking_EnabledToAdaptive(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}}],
		"thinking":{"type":"enabled","budget_tokens":4000},
		"messages":[{"role":"user","content":"hi"}]
	}`)

	out, changed, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}

	if got := gjson.GetBytes(out, "thinking.type").String(); got != "adaptive" {
		t.Fatalf("expected thinking.type=adaptive, got %q", got)
	}
	if gjson.GetBytes(out, "thinking.budget_tokens").Exists() {
		t.Fatalf("expected budget_tokens removed")
	}
}

func TestThinking_AlreadyAdaptive(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}}],
		"thinking":{"type":"adaptive"},
		"messages":[{"role":"user","content":"hi"}]
	}`)

	out, _, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "adaptive" {
		t.Fatalf("expected thinking.type=adaptive, got %q", got)
	}
}

func TestThinking_MissingInjected(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"hi"}]
	}`)

	out, changed, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "adaptive" {
		t.Fatalf("expected thinking injected as adaptive, got %q", got)
	}
}

// --- P0: context_management & output_config ---

func TestContextManagementInjected(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"hi"}]
	}`)

	out, _, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := gjson.GetBytes(out, "context_management.edits.0.type").String(); got != "clear_thinking_20251015" {
		t.Fatalf("expected context_management injected, got %q", got)
	}
	if got := gjson.GetBytes(out, "context_management.edits.0.keep").String(); got != "all" {
		t.Fatalf("expected keep=all, got %q", got)
	}
}

func TestContextManagementPreserved(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}}],
		"context_management":{"edits":[{"type":"custom","keep":"none"}]},
		"messages":[{"role":"user","content":"hi"}]
	}`)

	out, _, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Should NOT overwrite existing
	if got := gjson.GetBytes(out, "context_management.edits.0.type").String(); got != "custom" {
		t.Fatalf("expected existing context_management preserved, got %q", got)
	}
}

func TestOutputConfigInjected(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"hi"}]
	}`)

	out, _, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := gjson.GetBytes(out, "output_config.effort").String(); got != "medium" {
		t.Fatalf("expected output_config.effort=medium, got %q", got)
	}
}

// --- P1: cache_control TTL stripping ---

func TestCacheControlTTLStripped(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral","ttl":"1h"}}],
		"tools":[{"name":"t1","cache_control":{"type":"ephemeral","ttl":"5m"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","ttl":"5m"}}]}]
	}`)

	out, changed, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}

	// system: ttl stripped
	if gjson.GetBytes(out, "system.0.cache_control.ttl").Exists() {
		t.Fatalf("expected system cache_control.ttl stripped")
	}
	if got := gjson.GetBytes(out, "system.0.cache_control.type").String(); got != "ephemeral" {
		t.Fatalf("expected cache_control.type=ephemeral, got %q", got)
	}

	// tools: ttl stripped
	if gjson.GetBytes(out, "tools.0.cache_control.ttl").Exists() {
		t.Fatalf("expected tools cache_control.ttl stripped")
	}

	// messages: ttl stripped
	// content[0] is the user text (no extraction since no skill blocks)
	if gjson.GetBytes(out, "messages.0.content.0.cache_control.ttl").Exists() {
		t.Fatalf("expected messages cache_control.ttl stripped")
	}
}

// --- P1: all system blocks get cache_control ---

func TestAllSystemBlocksGetCacheControl(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[
			{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}},
			{"type":"text","text":"Some instructions without cache_control"},
			{"type":"text","text":"More instructions","cache_control":{"type":"ephemeral"}}
		],
		"messages":[{"role":"user","content":"hi"}]
	}`)

	out, _, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	systemCount := gjson.GetBytes(out, "system.#").Int()
	for i := int64(0); i < systemCount; i++ {
		path := fmt.Sprintf("system.%d.cache_control.type", i)
		if got := gjson.GetBytes(out, path).String(); got != "ephemeral" {
			t.Fatalf("expected system[%d] to have cache_control, got %q", i, got)
		}
	}
}

// --- P1: eager_input_streaming stripped ---

func TestEagerInputStreamingStripped(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}}],
		"tools":[
			{"name":"Bash","eager_input_streaming":true,"input_schema":{"type":"object"}},
			{"name":"Read","eager_input_streaming":true,"input_schema":{"type":"object"}}
		],
		"messages":[{"role":"user","content":"hi"}]
	}`)

	out, changed, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}

	toolCount := gjson.GetBytes(out, "tools.#").Int()
	for i := int64(0); i < toolCount; i++ {
		path := fmt.Sprintf("tools.%d.eager_input_streaming", i)
		if gjson.GetBytes(out, path).Exists() {
			t.Fatalf("expected tools[%d].eager_input_streaming stripped", i)
		}
	}
}

// --- P2: environment info extraction ---

func TestExtractEnvironmentBlock(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[
			{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}},
			{"type":"text","text":"# Environment\n\nToday's date: 2026-03-24\n\nWorking directory: /home/user"}
		],
		"messages":[{"role":"user","content":"hello"}]
	}`)

	out, changed, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}

	// system[] should only have preamble
	if got := gjson.GetBytes(out, "system.#").Int(); got != 1 {
		t.Fatalf("expected 1 system block, got %d", got)
	}

	// messages[0].content should have: [env-reminder, user-text]
	contentCount := gjson.GetBytes(out, "messages.0.content.#").Int()
	if contentCount != 2 {
		t.Fatalf("expected 2 content blocks, got %d", contentCount)
	}

	c0 := gjson.GetBytes(out, "messages.0.content.0.text").String()
	if !strings.Contains(c0, "<system-reminder>") || !strings.Contains(c0, "# Environment") {
		t.Fatalf("content[0] should be env <system-reminder>, got %q", c0)
	}
}

// --- Full Amp-style request conversion ---

func TestFullAmpToClaudeCodeConversion(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-opus-4-6",
		"max_tokens":32000,
		"stream":true,
		"thinking":{"type":"enabled","budget_tokens":4000},
		"system":[
			{"type":"text","text":"You are Amp, a powerful AI coding agent.","cache_control":{"type":"ephemeral","ttl":"1h"}},
			{"type":"text","text":"AGENTS.md guidance files are delivered dynamically..."},
			{"type":"text","text":"Contents of AGENTS.md (project instructions):\n\n<instructions>\ngo test ./...\n</instructions>"},
			{"type":"text","text":"# Environment\n\nToday's date: 2026-03-24\nOS: windows"},
			{"type":"text","text":"The following skills...\n<available_skills>\n<skill><name>code-review</name></skill>\n</available_skills>"},
			{"type":"text","text":"You MUST answer concisely.","cache_control":{"type":"ephemeral","ttl":"1h"}}
		],
		"tools":[
			{"name":"Bash","eager_input_streaming":true,"input_schema":{"type":"object","properties":{"cmd":{"type":"string"}}}},
			{"name":"Read","eager_input_streaming":true,"input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}
		],
		"messages":[{"role":"user","content":[
			{"type":"text","text":"# User State\n\n"},
			{"type":"text","text":"你好","cache_control":{"type":"ephemeral","ttl":"5m"}}
		]}]
	}`)

	out, changed, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}

	// --- Verify output structure ---
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// P0: thinking → adaptive
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "adaptive" {
		t.Fatalf("thinking.type: expected adaptive, got %q", got)
	}
	if gjson.GetBytes(out, "thinking.budget_tokens").Exists() {
		t.Fatalf("thinking.budget_tokens should be removed")
	}

	// P0: context_management injected
	if !gjson.GetBytes(out, "context_management").Exists() {
		t.Fatalf("context_management should be injected")
	}

	// P0: output_config injected
	if got := gjson.GetBytes(out, "output_config.effort").String(); got != "medium" {
		t.Fatalf("output_config.effort: expected medium, got %q", got)
	}

	// System: brand sanitized, only preamble + cleaned core instructions + concise rule remain
	// (skills, AGENTS.md, env extracted)
	systemCount := gjson.GetBytes(out, "system.#").Int()
	if got := gjson.GetBytes(out, "system.0.text").String(); got != claudeCodePreamble {
		t.Fatalf("system[0] should be preamble, got %q", got)
	}
	// "Amp" → "Claude Code" in system[1]
	sys1 := gjson.GetBytes(out, "system.1.text").String()
	if strings.Contains(sys1, "Amp") {
		t.Fatalf("system[1] should have brand sanitized, still contains 'Amp': %q", sys1)
	}
	// All system blocks should have cache_control
	for i := int64(0); i < systemCount; i++ {
		if got := gjson.GetBytes(out, fmt.Sprintf("system.%d.cache_control.type", i)).String(); got != "ephemeral" {
			t.Fatalf("system[%d] missing cache_control", i)
		}
		if gjson.GetBytes(out, fmt.Sprintf("system.%d.cache_control.ttl", i)).Exists() {
			t.Fatalf("system[%d] should NOT have ttl", i)
		}
	}

	// P1: tools - no eager_input_streaming, no cache_control ttl
	toolCount := gjson.GetBytes(out, "tools.#").Int()
	for i := int64(0); i < toolCount; i++ {
		if gjson.GetBytes(out, fmt.Sprintf("tools.%d.eager_input_streaming", i)).Exists() {
			t.Fatalf("tools[%d] should not have eager_input_streaming", i)
		}
	}

	// Messages: <system-reminder> blocks prepended, user text last
	contentCount := gjson.GetBytes(out, "messages.0.content.#").Int()
	if contentCount < 4 {
		t.Fatalf("expected at least 4 content blocks (3 reminders + user blocks), got %d", contentCount)
	}

	// First few blocks should be <system-reminder>
	reminderCount := 0
	for i := int64(0); i < contentCount; i++ {
		text := gjson.GetBytes(out, fmt.Sprintf("messages.0.content.%d.text", i)).String()
		if strings.Contains(text, "<system-reminder>") {
			reminderCount++
		}
	}
	if reminderCount < 3 {
		t.Fatalf("expected at least 3 <system-reminder> blocks (skills + agents + env), got %d", reminderCount)
	}

	// Last content block should have cache_control without ttl
	lastIdx := contentCount - 1
	lastCC := gjson.GetBytes(out, fmt.Sprintf("messages.0.content.%d.cache_control", lastIdx))
	if !lastCC.Exists() {
		t.Fatalf("last content block should have cache_control")
	}
	if gjson.GetBytes(out, fmt.Sprintf("messages.0.content.%d.cache_control.ttl", lastIdx)).Exists() {
		t.Fatalf("last content block cache_control should NOT have ttl")
	}
}

// --- Existing behavior: brand sanitize + preamble ---

func TestBrandSanitizeAndPreamble(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-3-7-sonnet",
		"system":[{"type":"text","text":"Use opencode with amp-code","cache_control":{"type":"ephemeral","ttl":"1m"}}],
		"tools":[{"name":"webSearch2","cache_control":{"type":"ephemeral","ttl":"1m"}}],
		"messages":[{"role":"user","content":[{"type":"tool_use","name":"extractWebPageContent","cache_control":{"type":"ephemeral","ttl":"1m"}}]}]
	}`)

	out, changed, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}

	// Preamble injected
	if got := gjson.GetBytes(out, "system.0.text").String(); got != claudeCodePreamble {
		t.Fatalf("expected preamble at system[0], got %q", got)
	}
	// Brand sanitized
	if got := gjson.GetBytes(out, "system.1.text").String(); got != "Use Claude Code with Claude Code" {
		t.Fatalf("expected sanitized system text, got %q", got)
	}
	// TTL stripped (was "1m")
	if gjson.GetBytes(out, "system.1.cache_control.ttl").Exists() {
		t.Fatalf("expected ttl stripped from system cache_control")
	}
	if gjson.GetBytes(out, "tools.0.cache_control.ttl").Exists() {
		t.Fatalf("expected ttl stripped from tools cache_control")
	}
}

// --- Idempotent: already official format ---

func TestAlreadyOfficialFormat(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[
			{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}},
			{"type":"text","text":"core instructions","cache_control":{"type":"ephemeral"}}
		],
		"thinking":{"type":"adaptive"},
		"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},
		"output_config":{"effort":"medium"},
		"messages":[{"role":"user","content":[
			{"type":"text","text":"<system-reminder>\nSkills...\n</system-reminder>"},
			{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}}
		]}]
	}`)

	out, _, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Should not add extra <system-reminder> blocks
	if got := gjson.GetBytes(out, "messages.0.content.#").Int(); got != 2 {
		t.Fatalf("expected 2 content blocks (no double wrap), got %d", got)
	}
}

// --- P2: $schema on tool input_schema ---

func TestToolInputSchemaURIAdded(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}}],
		"tools":[
			{"name":"Bash","input_schema":{"type":"object","properties":{"cmd":{"type":"string"}}}},
			{"name":"Read","input_schema":{"type":"object","$schema":"https://json-schema.org/draft/2020-12/schema"}}
		],
		"messages":[{"role":"user","content":"hi"}]
	}`)

	out, changed, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}

	// Bash: $schema should be added
	if got := gjson.GetBytes(out, `tools.0.input_schema.\$schema`).String(); got != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("expected $schema added to tools[0], got %q", got)
	}
	// Read: already had $schema, should be preserved
	if got := gjson.GetBytes(out, `tools.1.input_schema.\$schema`).String(); got != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("expected $schema preserved on tools[1], got %q", got)
	}
}

// --- Field ordering ---

func TestFieldOrdering(t *testing.T) {
	f := &ClaudeCodeSimulationFilter{}
	body := []byte(`{
		"stream":true,
		"tools":[],
		"model":"claude-sonnet-4-6",
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":"hi"}],
		"max_tokens":16000
	}`)

	out, _, err := f.Apply(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Verify field order: model should come before system, system before messages, etc.
	outStr := string(out)
	modelIdx := strings.Index(outStr, `"model"`)
	systemIdx := strings.Index(outStr, `"system"`)
	messagesIdx := strings.Index(outStr, `"messages"`)
	toolsIdx := strings.Index(outStr, `"tools"`)
	thinkingIdx := strings.Index(outStr, `"thinking"`)
	streamIdx := strings.Index(outStr, `"stream"`)

	if modelIdx > systemIdx {
		t.Fatalf("model should come before system")
	}
	if systemIdx > messagesIdx {
		t.Fatalf("system should come before messages")
	}
	if messagesIdx > toolsIdx {
		t.Fatalf("messages should come before tools")
	}
	if thinkingIdx > streamIdx {
		t.Fatalf("thinking should come before stream")
	}
}
