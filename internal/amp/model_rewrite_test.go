package amp

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

type testReadCloser struct{ io.Reader }

func (n testReadCloser) Close() error { return nil }

func TestTransformResponseJSON_RewritesOnlyTopLevelModelMetadata(t *testing.T) {
	ctx := WithProviderInfo(context.Background(), ProviderInfo{Provider: ProviderOpenAIChat})
	mappedModel := "gpt-4.1-mini"
	originalModel := "gpt-4.1-mini-safe"
	input := []byte(`{"id":"chatcmpl_123","object":"chat.completion","model":"gpt-4.1-mini","choices":[{"message":{"role":"assistant","content":"The upstream model is gpt-4.1-mini."}}],"tool_input":{"model":"gpt-4.1-mini"}}`)

	output := TransformResponseJSON(ctx, input, originalModel, mappedModel)

	if got := gjson.GetBytes(output, "model").String(); got != originalModel {
		t.Fatalf("expected top-level model to be rewritten to %q, got %q", originalModel, got)
	}
	if got := gjson.GetBytes(output, "choices.0.message.content").String(); got != "The upstream model is gpt-4.1-mini." {
		t.Fatalf("expected assistant content to remain unchanged, got %q", got)
	}
	if got := gjson.GetBytes(output, "tool_input.model").String(); got != mappedModel {
		t.Fatalf("expected nested tool_input.model to remain %q, got %q", mappedModel, got)
	}
}

func TestTransformResponseJSON_RewritesAnthropicMessageModelOnly(t *testing.T) {
	ctx := WithProviderInfo(context.Background(), ProviderInfo{Provider: ProviderAnthropic})
	mappedModel := "claude-3-7-sonnet-20250219"
	originalModel := "claude-3-7-sonnet"
	input := []byte(`{"type":"message_start","message":{"id":"msg_123","type":"message","model":"claude-3-7-sonnet-20250219","content":[{"type":"text","text":"The upstream model is claude-3-7-sonnet-20250219."}],"tool_input":{"model":"claude-3-7-sonnet-20250219"}}}`)

	output := TransformResponseJSON(ctx, input, originalModel, mappedModel)

	if got := gjson.GetBytes(output, "message.model").String(); got != originalModel {
		t.Fatalf("expected nested message.model to be rewritten to %q, got %q", originalModel, got)
	}
	if got := gjson.GetBytes(output, "message.content.0.text").String(); got != "The upstream model is claude-3-7-sonnet-20250219." {
		t.Fatalf("expected Anthropic content text to remain unchanged, got %q", got)
	}
	if got := gjson.GetBytes(output, "message.tool_input.model").String(); got != mappedModel {
		t.Fatalf("expected nested message.tool_input.model to remain %q, got %q", mappedModel, got)
	}
}

func TestSSETransformWrapper_RewritesOnlyResponseModelMetadata(t *testing.T) {
	ctx := WithProviderInfo(context.Background(), ProviderInfo{Provider: ProviderOpenAIResponses})
	mappedModel := "gpt-4.1-mini"
	originalModel := "gpt-4.1-requested"
	sse := strings.Join([]string{
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-4.1-mini\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"Mention gpt-4.1-mini in the answer body.\"}]}],\"tool_input\":{\"model\":\"gpt-4.1-mini\"}}}",
		"",
	}, "\n")

	wrapped := NewSSETransformWrapper(testReadCloser{Reader: strings.NewReader(sse)}, func(b []byte) []byte {
		return TransformResponseJSON(ctx, b, originalModel, mappedModel)
	})
	defer wrapped.Close()

	out, err := io.ReadAll(wrapped)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	_, payload, done := parseSSEEvent(out)
	if done {
		t.Fatalf("expected a normal SSE payload, got DONE")
	}
	if got := gjson.GetBytes(payload, "response.model").String(); got != originalModel {
		t.Fatalf("expected response.model to be rewritten to %q, got %q", originalModel, got)
	}
	if got := gjson.GetBytes(payload, "response.output.0.content.0.text").String(); got != "Mention gpt-4.1-mini in the answer body." {
		t.Fatalf("expected output text to remain unchanged, got %q", got)
	}
	if got := gjson.GetBytes(payload, "response.tool_input.model").String(); got != mappedModel {
		t.Fatalf("expected nested response.tool_input.model to remain %q, got %q", mappedModel, got)
	}
}
