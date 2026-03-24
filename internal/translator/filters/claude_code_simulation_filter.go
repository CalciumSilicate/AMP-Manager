package filters

import (
	"ampmanager/internal/translator"
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

const (
	claudeCodePreamble = "You are Claude Code, Anthropic's official CLI for Claude."
)

var claudeBrandSanitizeRe = regexp.MustCompile(`(?i)\b(?:opencode|amp(?:-?code)?)\b`)

// SetCacheTTLOverride is a no-op kept for backward compatibility.
// The official Claude Code format does not use TTL in cache_control.
func SetCacheTTLOverride(_ string) {}

// GetCacheTTLOverride is a no-op kept for backward compatibility.
func GetCacheTTLOverride() string { return "" }

type ClaudeCodeSimulationFilter struct{}

func (f *ClaudeCodeSimulationFilter) Name() string {
	return "claude_code_simulation"
}

func (f *ClaudeCodeSimulationFilter) Applies(outgoingFormat translator.Format) bool {
	return outgoingFormat == translator.FormatClaude
}

func sanitizeClaudeBrandText(s string) string {
	return claudeBrandSanitizeRe.ReplaceAllString(s, "Claude Code")
}

// --- P1: cache_control 统一为官方格式 {type:"ephemeral"}，去掉 ttl ---

// stripCacheControlTTL removes the non-official "ttl" field from cache_control
// and normalizes it to the official format: {"type":"ephemeral"}.
func stripCacheControlTTL(obj map[string]any) bool {
	cc, ok := obj["cache_control"]
	if !ok {
		return false
	}
	ccMap, ok := cc.(map[string]any)
	if !ok {
		return false
	}
	if _, hasTTL := ccMap["ttl"]; !hasTTL {
		return false
	}
	obj["cache_control"] = map[string]any{"type": "ephemeral"}
	return true
}

// ensureBlockCacheControl adds cache_control:{type:"ephemeral"} if missing.
func ensureBlockCacheControl(obj map[string]any) bool {
	if _, ok := obj["cache_control"]; ok {
		return false
	}
	obj["cache_control"] = map[string]any{"type": "ephemeral"}
	return true
}

// --- P2: 从 system[] 提取技能/AGENTS.md/环境信息 → <system-reminder> ---

var (
	skillBlockRe    = regexp.MustCompile(`(?i)<available_skills>|The following skills`)
	agentsMdBlockRe = regexp.MustCompile(`(?i)Contents of .*(?:AGENTS|CLAUDE)\.md|AGENTS\.md guidance files`)
	envBlockRe      = regexp.MustCompile(`(?i)^# Environment\b`)
)

type extractedBlockType int

const (
	blockSkill extractedBlockType = iota
	blockAgentsMd
	blockEnv
)

func classifyBlock(text string) (extractedBlockType, bool) {
	if skillBlockRe.MatchString(text) {
		return blockSkill, true
	}
	if agentsMdBlockRe.MatchString(text) {
		return blockAgentsMd, true
	}
	if envBlockRe.MatchString(text) {
		return blockEnv, true
	}
	return 0, false
}

func wrapAsSystemReminder(blockType extractedBlockType, text string) string {
	switch blockType {
	case blockSkill:
		return "<system-reminder>\n" + text + "\n</system-reminder>\n"
	case blockAgentsMd:
		return "<system-reminder>\nAs you answer the user's questions, you can use the following context:\n" + text + "\n</system-reminder>\n"
	case blockEnv:
		return "<system-reminder>\n" + text + "\n</system-reminder>\n"
	}
	return text
}

// extractSystemReminders extracts skill, AGENTS.md, and environment blocks from system[],
// removes them from system, and returns them as <system-reminder> wrapped content blocks.
func extractSystemReminders(root map[string]any) ([]any, bool) {
	systemArr, ok := root["system"].([]any)
	if !ok || len(systemArr) == 0 {
		return nil, false
	}

	var reminders []any
	var remaining []any

	for _, item := range systemArr {
		obj, ok := item.(map[string]any)
		if !ok {
			remaining = append(remaining, item)
			continue
		}
		text, ok := obj["text"].(string)
		if !ok {
			remaining = append(remaining, item)
			continue
		}

		if blockType, matched := classifyBlock(text); matched {
			reminders = append(reminders, map[string]any{
				"type": "text",
				"text": wrapAsSystemReminder(blockType, text),
			})
		} else {
			remaining = append(remaining, item)
		}
	}

	if len(reminders) == 0 {
		return nil, false
	}

	root["system"] = remaining
	return reminders, true
}

// wrapFirstUserMessage extracts extractable system blocks into <system-reminder>
// content blocks prepended to messages[0].content[].
func wrapFirstUserMessage(root map[string]any) bool {
	messages, ok := root["messages"].([]any)
	if !ok || len(messages) == 0 {
		return false
	}

	firstMsg, ok := messages[0].(map[string]any)
	if !ok || firstMsg["role"] != "user" {
		return false
	}

	changed := false

	reminders, extracted := extractSystemReminders(root)
	if extracted {
		changed = true
	}

	switch v := firstMsg["content"].(type) {
	case string:
		blocks := make([]any, 0, len(reminders)+1)
		blocks = append(blocks, reminders...)
		blocks = append(blocks, map[string]any{
			"type":          "text",
			"text":          v,
			"cache_control": map[string]any{"type": "ephemeral"},
		})
		firstMsg["content"] = blocks
		changed = true

	case []any:
		if len(v) == 0 {
			return changed
		}

		// If already has <system-reminder>, skip wrapping
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := obj["text"].(string); ok && strings.Contains(text, "<system-reminder>") {
				if ensureLastBlockCacheControl(v) {
					changed = true
				}
				return changed
			}
		}

		if len(reminders) > 0 {
			wrapped := make([]any, 0, len(reminders)+len(v))
			wrapped = append(wrapped, reminders...)
			wrapped = append(wrapped, v...)
			firstMsg["content"] = wrapped
			if ensureLastBlockCacheControl(wrapped) {
				changed = true
			}
		} else {
			if ensureLastBlockCacheControl(v) {
				changed = true
			}
		}
	}

	return changed
}

// ensureLastBlockCacheControl adds cache_control to the last content block if missing.
func ensureLastBlockCacheControl(content []any) bool {
	if len(content) == 0 {
		return false
	}
	last, ok := content[len(content)-1].(map[string]any)
	if !ok {
		return false
	}
	if _, has := last["cache_control"]; has {
		return false
	}
	last["cache_control"] = map[string]any{"type": "ephemeral"}
	return true
}

// --- P0: thinking / context_management / output_config ---

// normalizeThinking converts thinking from {type:"enabled", budget_tokens:N}
// to the official {type:"adaptive"} format.
func normalizeThinking(root map[string]any) bool {
	thinking, ok := root["thinking"]
	if !ok {
		// No thinking → inject official default
		root["thinking"] = map[string]any{"type": "adaptive"}
		return true
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		root["thinking"] = map[string]any{"type": "adaptive"}
		return true
	}
	// Already adaptive → no change
	if t, _ := thinkingMap["type"].(string); t == "adaptive" {
		if _, hasBudget := thinkingMap["budget_tokens"]; !hasBudget {
			return false
		}
	}
	root["thinking"] = map[string]any{"type": "adaptive"}
	return true
}

// ensureContextManagement injects context_management if not present.
func ensureContextManagement(root map[string]any) bool {
	if _, ok := root["context_management"]; ok {
		return false
	}
	root["context_management"] = map[string]any{
		"edits": []any{
			map[string]any{
				"type": "clear_thinking_20251015",
				"keep": "all",
			},
		},
	}
	return true
}

// ensureOutputConfig injects output_config if not present.
func ensureOutputConfig(root map[string]any) bool {
	if _, ok := root["output_config"]; ok {
		return false
	}
	root["output_config"] = map[string]any{
		"effort": "medium",
	}
	return true
}

// --- P1: tools 清理 eager_input_streaming ---

// stripToolsEagerInputStreaming removes the non-official eager_input_streaming field from tools.
func stripToolsEagerInputStreaming(root map[string]any) bool {
	tools, ok := root["tools"].([]any)
	if !ok {
		return false
	}
	changed := false
	for _, tool := range tools {
		obj, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		if _, has := obj["eager_input_streaming"]; has {
			delete(obj, "eager_input_streaming")
			changed = true
		}
	}
	return changed
}

// --- P2: tools input_schema 补 $schema ---

const jsonSchemaURI = "https://json-schema.org/draft/2020-12/schema"

// ensureToolsInputSchemaURI adds "$schema" to every tool's input_schema if missing.
func ensureToolsInputSchemaURI(root map[string]any) bool {
	tools, ok := root["tools"].([]any)
	if !ok {
		return false
	}
	changed := false
	for _, tool := range tools {
		obj, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		schema, ok := obj["input_schema"].(map[string]any)
		if !ok {
			continue
		}
		if _, has := schema["$schema"]; !has {
			schema["$schema"] = jsonSchemaURI
			changed = true
		}
	}
	return changed
}

// --- 字段排序：匹配官方 claudecode.json 的字段顺序 ---

// officialFieldOrder defines the key order matching the official Claude Code request format.
var officialFieldOrder = []string{
	"model",
	"max_tokens",
	"system",
	"messages",
	"tools",
	"metadata",
	"temperature",
	"top_p",
	"top_k",
	"thinking",
	"context_management",
	"output_config",
	"stream",
}

// marshalOrderedJSON serializes root with official field ordering and no HTML escaping.
func marshalOrderedJSON(root map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	buf.WriteByte('{')
	first := true
	written := make(map[string]struct{})

	writeField := func(key string, val any) error {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		keyBytes, _ := json.Marshal(key)
		buf.Write(keyBytes)
		buf.WriteByte(':')

		// Use a sub-encoder for the value to preserve SetEscapeHTML(false)
		var vBuf bytes.Buffer
		vEnc := json.NewEncoder(&vBuf)
		vEnc.SetEscapeHTML(false)
		if err := vEnc.Encode(val); err != nil {
			return err
		}
		buf.Write(bytes.TrimRight(vBuf.Bytes(), "\n"))
		written[key] = struct{}{}
		return nil
	}

	// Write fields in official order
	for _, key := range officialFieldOrder {
		if val, ok := root[key]; ok {
			if err := writeField(key, val); err != nil {
				return nil, err
			}
		}
	}

	// Write remaining fields not in the official order
	for key, val := range root {
		if _, done := written[key]; done {
			continue
		}
		if err := writeField(key, val); err != nil {
			return nil, err
		}
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// --- Main Apply ---

func (f *ClaudeCodeSimulationFilter) Apply(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	if !json.Valid(body) {
		return body, false, nil
	}

	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return body, false, nil
	}

	if model, ok := root["model"].(string); ok {
		if strings.Contains(strings.ToLower(model), "haiku") {
			return body, false, nil
		}
	}

	changed := false

	// 0) system 清洗 + 注入 Claude Code 身份声明
	systemVal, hasSystem := root["system"]
	if !hasSystem {
		root["system"] = []any{map[string]any{
			"type":          "text",
			"text":          claudeCodePreamble,
			"cache_control": map[string]any{"type": "ephemeral"},
		}}
		changed = true
	} else {
		switch v := systemVal.(type) {
		case string:
			cleaned := sanitizeClaudeBrandText(v)
			items := []any{map[string]any{
				"type":          "text",
				"text":          claudeCodePreamble,
				"cache_control": map[string]any{"type": "ephemeral"},
			}}

			rest := cleaned
			if strings.HasPrefix(rest, claudeCodePreamble) {
				rest = strings.TrimPrefix(rest, claudeCodePreamble)
				rest = strings.TrimLeft(rest, "\r\n")
			}
			if rest != "" {
				items = append(items, map[string]any{
					"type":          "text",
					"text":          rest,
					"cache_control": map[string]any{"type": "ephemeral"},
				})
			}
			root["system"] = items
			changed = true
		case []any:
			for _, item := range v {
				obj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				// P1: strip ttl from existing cache_control
				if stripCacheControlTTL(obj) {
					changed = true
				}
				if t, ok := obj["type"].(string); ok && t == "text" {
					if text, ok := obj["text"].(string); ok {
						cleaned := sanitizeClaudeBrandText(text)
						if cleaned != text {
							obj["text"] = cleaned
							changed = true
						}
					}
				}
			}

			alreadyPrefixed := false
			if len(v) > 0 {
				if first, ok := v[0].(map[string]any); ok {
					if t, _ := first["type"].(string); t == "text" {
						if text, _ := first["text"].(string); text == claudeCodePreamble {
							alreadyPrefixed = true
						}
					}
				}
			}
			if !alreadyPrefixed {
				root["system"] = append(
					[]any{map[string]any{
						"type":          "text",
						"text":          claudeCodePreamble,
						"cache_control": map[string]any{"type": "ephemeral"},
					}},
					v...,
				)
				changed = true
			}
		}
	}

	// 1) tools: strip eager_input_streaming + strip cache_control ttl + ensure $schema
	if stripToolsEagerInputStreaming(root) {
		changed = true
	}
	if ensureToolsInputSchemaURI(root) {
		changed = true
	}
	if tools, ok := root["tools"].([]any); ok {
		for _, tool := range tools {
			obj, ok := tool.(map[string]any)
			if !ok {
				continue
			}
			if stripCacheControlTTL(obj) {
				changed = true
			}
		}
	}

	// 2) 首条 user message: 提取技能/AGENTS.md/环境信息 → <system-reminder>
	if wrapFirstUserMessage(root) {
		changed = true
	}

	// 3) P1: system[] 每个 block 补齐 cache_control（提取后 remaining blocks）
	if systemArr, ok := root["system"].([]any); ok {
		for _, item := range systemArr {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if ensureBlockCacheControl(obj) {
				changed = true
			}
		}
	}

	// 4) messages[].content[] cache_control: strip ttl
	if messages, ok := root["messages"].([]any); ok {
		for _, msg := range messages {
			msgObj, ok := msg.(map[string]any)
			if !ok {
				continue
			}
			content, ok := msgObj["content"].([]any)
			if !ok {
				continue
			}
			for _, item := range content {
				itemObj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if stripCacheControlTTL(itemObj) {
					changed = true
				}
			}
		}
	}

	// 5) P0: thinking → adaptive
	if normalizeThinking(root) {
		changed = true
	}

	// 6) P0: inject context_management & output_config
	if ensureContextManagement(root) {
		changed = true
	}
	if ensureOutputConfig(root) {
		changed = true
	}

	if !changed {
		return body, false, nil
	}

	out, err := marshalOrderedJSON(root)
	if err != nil {
		return body, false, nil
	}
	if bytes.Equal(out, body) {
		return body, false, nil
	}
	return out, true, nil
}
