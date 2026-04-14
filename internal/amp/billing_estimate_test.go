package amp

import (
	"encoding/json"
	"testing"
)

func TestEstimateReservationInputTokens_OpenAIChat(t *testing.T) {
	payload := mustEstimatePayload(t, `{
		"model": "gpt-4.1",
		"messages": [
			{"role": "system", "content": "You are a concise assistant."},
			{"role": "user", "content": [
				{"type": "text", "text": "Summarize 你好世界 and explain the diff."},
				{"type": "input_text", "text": "The patch adds validation and retries."}
			]}
		],
		"tools": [{
			"type": "function",
			"function": {
				"name": "lookup_diff",
				"description": "Fetch a diff summary",
				"parameters": {
					"type": "object",
					"properties": {
						"path": {"type": "string", "description": "target file"}
					}
				}
			}
		}],
		"response_format": {
			"type": "json_schema",
			"json_schema": {
				"name": "summary",
				"schema": {
					"type": "object",
					"properties": {
						"summary": {"type": "string"}
					}
				}
			}
		}
	}`)

	withoutSchemas := mustEstimatePayload(t, `{
		"messages": [{"role": "user", "content": "Summarize 你好世界 and explain the diff."}]
	}`)

	got := estimateReservationInputTokens(payload, billingRequestOpenAIChat)
	baseline := estimateReservationInputTokens(withoutSchemas, billingRequestOpenAIChat)
	if got <= baseline {
		t.Fatalf("expected structured chat estimate %d to exceed baseline %d", got, baseline)
	}
}

func TestEstimateReservationInputTokens_OpenAIResponses(t *testing.T) {
	payload := mustEstimatePayload(t, `{
		"model": "gpt-5",
		"instructions": "Answer as JSON only.",
		"input": [
			{
				"role": "user",
				"content": [
					{"type": "input_text", "text": "Compare alpha and beta."},
					{"type": "input_text", "text": "再补充中文说明"}
				]
			}
		],
		"tools": [{
			"type": "function",
			"name": "fetch_notes",
			"description": "Fetch extra notes",
			"parameters": {
				"type": "object",
				"properties": {
					"topic": {"type": "string"}
				}
			}
		}],
		"text": {
			"format": {
				"type": "json_schema",
				"name": "answer",
				"schema": {
					"type": "object",
					"properties": {
						"result": {"type": "string"}
					}
				}
			}
		}
	}`)

	got := estimateReservationInputTokens(payload, billingRequestOpenAIResponses)
	if got <= 0 {
		t.Fatalf("expected positive responses estimate, got %d", got)
	}
}

func TestEstimateReservationInputTokens_Anthropic(t *testing.T) {
	payload := mustEstimatePayload(t, `{
		"model": "claude-sonnet-4-5",
		"system": [{"type": "text", "text": "Keep answers short."}],
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Explain the deployment plan."},
				{"type": "tool_result", "content": [{"type": "text", "text": "build passed"}]}
			]
		}],
		"tools": [{
			"name": "run_checks",
			"description": "Run validation checks",
			"input_schema": {
				"type": "object",
				"properties": {
					"scope": {"type": "string"}
				}
			}
		}]
	}`)

	got := estimateReservationInputTokens(payload, billingRequestAnthropic)
	if got <= 0 {
		t.Fatalf("expected positive anthropic estimate, got %d", got)
	}
}

func TestEstimateReservationInputTokens_Gemini(t *testing.T) {
	payload := mustEstimatePayload(t, `{
		"systemInstruction": {
			"parts": [{"text": "Return JSON only"}]
		},
		"contents": [{
			"role": "user",
			"parts": [
				{"text": "Summarize the release note."},
				{"functionCall": {"name": "lookup_docs", "args": {"slug": "billing"}}}
			]
		}],
		"tools": [{
			"functionDeclarations": [{
				"name": "lookup_docs",
				"description": "Read release docs",
				"parameters": {
					"type": "object",
					"properties": {"slug": {"type": "string"}}
				}
			}]
		}],
		"generationConfig": {
			"responseSchema": {
				"type": "object",
				"properties": {
					"summary": {"type": "string"}
				}
			}
		}
	}`)

	got := estimateReservationInputTokens(payload, billingRequestGemini)
	if got <= 0 {
		t.Fatalf("expected positive gemini estimate, got %d", got)
	}
}

func TestApproximateTextTokenCount_IsUnicodeAware(t *testing.T) {
	ascii := approximateTextTokenCount("abcdefgh")
	cjk := approximateTextTokenCount("你好世界")
	if ascii != 2 {
		t.Fatalf("expected ascii estimate 2, got %d", ascii)
	}
	if cjk != 4 {
		t.Fatalf("expected cjk estimate 4, got %d", cjk)
	}
	if cjk <= ascii {
		t.Fatalf("expected cjk estimate %d to exceed ascii estimate %d", cjk, ascii)
	}
}

func TestExtractReservationMaxOutputTokens(t *testing.T) {
	t.Run("explicit value wins below metadata", func(t *testing.T) {
		got := extractReservationMaxOutputTokens(map[string]interface{}{
			"max_output_tokens": 1024.0,
		}, "gpt-4.1")
		if got != 1024 {
			t.Fatalf("expected explicit max_output_tokens, got %d", got)
		}
	})

	t.Run("metadata clamps oversized explicit value", func(t *testing.T) {
		got := extractReservationMaxOutputTokens(map[string]interface{}{
			"max_completion_tokens": 999999.0,
		}, "gpt-4.1")
		if got != 32768 {
			t.Fatalf("expected metadata clamp 32768, got %d", got)
		}
	})

	t.Run("metadata fallback when explicit absent", func(t *testing.T) {
		got := extractReservationMaxOutputTokens(nil, "claude-4")
		if got != 64000 {
			t.Fatalf("expected metadata fallback 64000, got %d", got)
		}
	})

	t.Run("o-series default remains", func(t *testing.T) {
		got := extractReservationMaxOutputTokens(nil, "o3-mini")
		if got != 65536 {
			t.Fatalf("expected o-series default 65536, got %d", got)
		}
	})

	t.Run("default fallback remains", func(t *testing.T) {
		got := extractReservationMaxOutputTokens(nil, "unknown-model")
		if got != defaultReservationMaxOutputTokens {
			t.Fatalf("expected default fallback %d, got %d", defaultReservationMaxOutputTokens, got)
		}
	})
}

func TestDetectBillingRequestKind(t *testing.T) {
	cases := map[string]billingRequestKind{
		"/v1/chat/completions":                                           billingRequestOpenAIChat,
		"/api/provider/openai/v1/responses":                              billingRequestOpenAIResponses,
		"/v1/messages":                                                   billingRequestAnthropic,
		"/v1beta/models/gemini-2.5-pro:generateContent":                  billingRequestGemini,
		"/v1beta1/publishers/google/models/gemini:streamGenerateContent": billingRequestGemini,
	}

	for path, want := range cases {
		if got := detectBillingRequestKind(path); got != want {
			t.Fatalf("path %s: expected %q, got %q", path, want, got)
		}
	}
}

func mustEstimatePayload(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}
