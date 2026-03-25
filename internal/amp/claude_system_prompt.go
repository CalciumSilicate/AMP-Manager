package amp

import (
	_ "embed"
	"encoding/json"
	"strings"

	log "github.com/sirupsen/logrus"
)

//go:embed claude_code_system.json
var claudeCodeSystemJSON []byte

var claudeCodeSystemPrompt []any

func init() {
	if err := json.Unmarshal(claudeCodeSystemJSON, &claudeCodeSystemPrompt); err != nil {
		log.Fatalf("failed to parse embedded claude code system prompt: %v", err)
	}
}

// applyClaudeCodeSystemPrompt replaces the request's system prompt with the
// official Claude Code system prompt and moves the original system content
// into the first user message wrapped in <system-reminder> tags.
//
// This mirrors how real Claude Code structures its requests: the system array
// contains the official instructions, while user-specific context (CLAUDE.md,
// project instructions, etc.) is injected as <system-reminder> blocks inside
// the first user message.
func applyClaudeCodeSystemPrompt(body []byte) []byte {
	if len(body) == 0 || !json.Valid(body) {
		return body
	}

	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return body
	}

	// Extract original system content
	originalTexts := extractSystemTexts(root["system"])
	if len(originalTexts) == 0 {
		// No system prompt to move, just replace with official one
		root["system"] = claudeCodeSystemPrompt
		out, err := json.Marshal(root)
		if err != nil {
			return body
		}
		log.Debugf("claude system prompt: replaced system prompt (no original to move)")
		return out
	}

	// Build <system-reminder> blocks from original system content
	var reminderBlocks []any
	for _, text := range originalTexts {
		wrapped := "<system-reminder>\n" + strings.TrimSpace(text) + "\n</system-reminder>\n"
		reminderBlocks = append(reminderBlocks, map[string]any{
			"type": "text",
			"text": wrapped,
		})
	}

	// Find first user message and prepend reminder blocks to its content
	messages, ok := root["messages"].([]any)
	if !ok || len(messages) == 0 {
		return body
	}

	injected := false
	for _, msg := range messages {
		msgObj, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := msgObj["role"].(string); role != "user" {
			continue
		}

		// Normalize content to array form
		content := normalizeContentToArray(msgObj["content"])
		// Prepend system-reminder blocks
		newContent := make([]any, 0, len(reminderBlocks)+len(content))
		newContent = append(newContent, reminderBlocks...)
		newContent = append(newContent, content...)
		msgObj["content"] = newContent
		injected = true
		break
	}

	if !injected {
		return body
	}

	// Replace system with official Claude Code system prompt
	root["system"] = claudeCodeSystemPrompt

	out, err := json.Marshal(root)
	if err != nil {
		log.Warnf("claude system prompt: failed to marshal transformed body: %v", err)
		return body
	}
	log.Debugf("claude system prompt: moved %d system blocks to first user message, replaced system prompt", len(originalTexts))
	return out
}

// extractSystemTexts extracts text strings from the system field.
// Supports both string and array-of-objects formats.
func extractSystemTexts(system any) []string {
	if system == nil {
		return nil
	}

	// system is a plain string
	if s, ok := system.(string); ok && strings.TrimSpace(s) != "" {
		return []string{s}
	}

	// system is an array of objects with "text" fields
	arr, ok := system.([]any)
	if !ok {
		return nil
	}

	var texts []string
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := obj["text"].(string); ok && strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	return texts
}

// normalizeContentToArray ensures message content is in array form.
func normalizeContentToArray(content any) []any {
	if content == nil {
		return nil
	}
	// Already an array
	if arr, ok := content.([]any); ok {
		return arr
	}
	// String content — wrap in a text block
	if s, ok := content.(string); ok {
		return []any{map[string]any{
			"type": "text",
			"text": s,
		}}
	}
	return nil
}
