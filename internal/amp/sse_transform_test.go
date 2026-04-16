package amp

import (
	"context"
	"io"
	"strings"
	"testing"

	"ampmanager/internal/translator"
)

type nopReadCloser struct{ io.Reader }

func (n nopReadCloser) Close() error { return nil }

func TestSSETransformWrapperStripsMCPPrefix(t *testing.T) {
	sse := "event: message\n" +
		"data: {\"type\":\"tool_use\",\"name\":\"mcp__tools__mcp-extractwebpagecontent\"}\n\n"

	rc := nopReadCloser{Reader: strings.NewReader(sse)}
	wrapped := NewSSETransformWrapper(rc, func(b []byte) []byte {
		out, _ := UnprefixClaudeToolNamesWithMap(b, ClaudeToolNameMap{"mcp__tools__mcp-extractwebpagecontent": "extractWebPageContent"})
		return out
	})
	defer wrapped.Close()

	out, err := io.ReadAll(wrapped)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(string(out), `"name":"extractWebPageContent"`) {
		t.Fatalf("expected prefix stripped, got: %s", string(out))
	}
}

func TestStreamingTranslationExtractsUsageFromUpstreamBeforeCompatibilityRewrite(t *testing.T) {
	request := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	translatedRequest, err := translator.TranslateRequest(translator.FormatOpenAIChat, translator.FormatOpenAIResponses, "gpt-5.4", request, true)
	if err != nil {
		t.Fatalf("TranslateRequest returned error: %v", err)
	}

	rawSSE := strings.Join([]string{
		"event: response.created\n",
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":123,\"model\":\"gpt-5.4\",\"status\":\"in_progress\"}}\n\n",
		"event: response.output_text.delta\n",
		"data: {\"type\":\"response.output_text.delta\",\"response_id\":\"resp_1\",\"output_index\":0,\"delta\":\"hello\"}\n\n",
		"event: response.completed\n",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":123,\"model\":\"gpt-5.4\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":21,\"output_tokens\":8,\"total_tokens\":29,\"input_tokens_details\":{\"cached_tokens\":3}}}}\n\n",
		"data: [DONE]\n\n",
	}, "")

	trace := NewRequestTrace("req-1", "user-1", "key-1", "POST", "/v1/chat/completions")
	body := WrapResponseBodyForTokenExtraction(
		nopReadCloser{Reader: strings.NewReader(rawSSE)},
		true,
		trace,
		ProviderInfo{Provider: ProviderOpenAIResponses},
	)

	var (
		responseParam any
		transformErr  error
	)
	body = NewSSEJSONTransformWrapper(body, func(b []byte) [][]byte {
		payloads, err := translator.TranslateStream(
			context.Background(),
			translator.FormatOpenAIChat,
			translator.FormatOpenAIResponses,
			"gpt-5.4",
			request,
			translatedRequest,
			b,
			&responseParam,
		)
		if err != nil {
			transformErr = err
			return [][]byte{b}
		}
		out := make([][]byte, 0, len(payloads))
		for _, payload := range payloads {
			out = append(out, []byte(payload))
		}
		return out
	})
	defer body.Close()

	out, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if transformErr != nil {
		t.Fatalf("TranslateStream returned error: %v", transformErr)
	}

	if ptrToInt(trace.InputTokens) != 21 {
		t.Fatalf("input tokens = %d, want 21", ptrToInt(trace.InputTokens))
	}
	if ptrToInt(trace.OutputTokens) != 8 {
		t.Fatalf("output tokens = %d, want 8", ptrToInt(trace.OutputTokens))
	}
	if ptrToInt(trace.CacheReadInputTokens) != 3 {
		t.Fatalf("cache read tokens = %d, want 3", ptrToInt(trace.CacheReadInputTokens))
	}

	output := string(out)
	if !strings.Contains(output, `"object":"chat.completion.chunk"`) {
		t.Fatalf("expected translated chat completion chunks, got: %s", output)
	}
	if !strings.Contains(output, `"prompt_tokens":21`) {
		t.Fatalf("expected translated usage.prompt_tokens in output, got: %s", output)
	}
	if !strings.Contains(output, `"completion_tokens":8`) {
		t.Fatalf("expected translated usage.completion_tokens in output, got: %s", output)
	}
}
