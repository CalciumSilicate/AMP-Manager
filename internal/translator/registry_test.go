package translator

import (
	"context"
	"testing"

	"github.com/tidwall/gjson"
)

func TestTranslateRequestUsesCLIProxyAPISDK(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-4.1",
		"input":[
			{
				"role":"user",
				"content":[{"type":"input_text","text":"hello"}]
			}
		],
		"stream":false
	}`)

	translated, err := TranslateRequest(FormatOpenAIResponses, FormatOpenAIChat, "gpt-4.1", raw, false)
	if err != nil {
		t.Fatalf("TranslateRequest returned error: %v", err)
	}
	if !gjson.GetBytes(translated, "messages.0").Exists() {
		t.Fatalf("expected chat-completions messages in translated payload, got %s", string(translated))
	}
	if got := gjson.GetBytes(translated, "messages.0.role").String(); got != "user" {
		t.Fatalf("messages.0.role = %q, want user", got)
	}
}

func TestTranslateRequestSupportsOpenAIChatToResponses(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-5.4",
		"messages":[
			{"role":"system","content":"be terse"},
			{"role":"user","content":"hello"}
		],
		"stream":false
	}`)

	if !HasResponseTransformer(FormatOpenAIChat, FormatOpenAIResponses) {
		t.Fatal("expected chat -> responses response transformer to be available")
	}
	if !SupportsTranslation(FormatOpenAIChat, FormatOpenAIResponses) {
		t.Fatal("expected chat -> responses to be considered translatable")
	}

	translated, err := TranslateRequest(FormatOpenAIChat, FormatOpenAIResponses, "gpt-5.4", raw, false)
	if err != nil {
		t.Fatalf("TranslateRequest returned error: %v", err)
	}

	if got := gjson.GetBytes(translated, "instructions").String(); got != "be terse" {
		t.Fatalf("instructions = %q, want %q", got, "be terse")
	}
	if got := gjson.GetBytes(translated, "input.0.role").String(); got != "user" {
		t.Fatalf("input.0.role = %q, want user", got)
	}
	if got := gjson.GetBytes(translated, "input.0.content.0.text").String(); got != "hello" {
		t.Fatalf("input.0.content.0.text = %q, want hello", got)
	}
}

func TestTranslateNonStreamSupportsOpenAIResponsesToChat(t *testing.T) {
	request := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}]}`)
	translatedRequest, err := TranslateRequest(FormatOpenAIChat, FormatOpenAIResponses, "gpt-5.4", request, false)
	if err != nil {
		t.Fatalf("TranslateRequest returned error: %v", err)
	}

	rawResponse := []byte(`{
		"id":"resp_123",
		"object":"response",
		"created_at":1776291683,
		"status":"completed",
		"model":"gpt-5.4",
		"output":[
			{
				"type":"message",
				"content":[{"type":"output_text","text":"OK"}]
			}
		],
		"usage":{"input_tokens":22,"output_tokens":5,"total_tokens":27}
	}`)

	converted, err := TranslateNonStream(
		context.Background(),
		FormatOpenAIChat,
		FormatOpenAIResponses,
		"gpt-5.4",
		request,
		translatedRequest,
		rawResponse,
		nil,
	)
	if err != nil {
		t.Fatalf("TranslateNonStream returned error: %v", err)
	}

	if got := gjson.Get(converted, "object").String(); got != "chat.completion" {
		t.Fatalf("object = %q, want chat.completion", got)
	}
	if got := gjson.Get(converted, "choices.0.message.content").String(); got != "OK" {
		t.Fatalf("choices.0.message.content = %q, want OK", got)
	}
	if got := gjson.Get(converted, "usage.total_tokens").Int(); got != 27 {
		t.Fatalf("usage.total_tokens = %d, want 27", got)
	}
}

func TestTranslateRequestSupportsClaudeToResponses(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-5.4-mini",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hello from claude"}]
	}`)

	if !SupportsTranslation(FormatClaude, FormatOpenAIResponses) {
		t.Fatal("expected claude -> responses to be considered translatable")
	}

	translated, err := TranslateRequest(FormatClaude, FormatOpenAIResponses, "gpt-5.4-mini", raw, false)
	if err != nil {
		t.Fatalf("TranslateRequest returned error: %v", err)
	}
	if got := gjson.GetBytes(translated, "input.0.role").String(); got != "user" {
		t.Fatalf("input.0.role = %q, want user", got)
	}
	if got := gjson.GetBytes(translated, "model").String(); got != "gpt-5.4-mini" {
		t.Fatalf("model = %q, want gpt-5.4-mini", got)
	}
}

func TestTranslateRequestSupportsGeminiToResponses(t *testing.T) {
	raw := []byte(`{
		"contents":[
			{"role":"user","parts":[{"text":"hello from gemini"}]}
		]
	}`)

	if !SupportsTranslation(FormatGemini, FormatOpenAIResponses) {
		t.Fatal("expected gemini -> responses to be considered translatable")
	}

	translated, err := TranslateRequest(FormatGemini, FormatOpenAIResponses, "gpt-5.4-mini", raw, false)
	if err != nil {
		t.Fatalf("TranslateRequest returned error: %v", err)
	}
	if got := gjson.GetBytes(translated, "input.0.role").String(); got != "user" {
		t.Fatalf("input.0.role = %q, want user", got)
	}
	if got := gjson.GetBytes(translated, "input.0.content.0.text").String(); got != "hello from gemini" {
		t.Fatalf("input.0.content.0.text = %q, want hello from gemini", got)
	}
}
