package mockupstream

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleChatCompletionsNonStreamIncludesUsage(t *testing.T) {
	server := httptest.NewServer(New(Config{
		APIKey:               "perf-upstream-key",
		Models:               []string{"bench-chat-upstream"},
		ResponseText:         "hello from mock",
		ChatPromptTokens:     11,
		ChatCompletionTokens: 22,
	}).Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"bench-chat-upstream","stream":false}`))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer perf-upstream-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		t.Fatalf("missing usage payload: %#v", payload)
	}
	if usage["prompt_tokens"] != float64(11) {
		t.Fatalf("unexpected prompt tokens: %#v", usage)
	}
	if usage["completion_tokens"] != float64(22) {
		t.Fatalf("unexpected completion tokens: %#v", usage)
	}
}

func TestHandleResponsesStreamIncludesCompletedEventAndDone(t *testing.T) {
	server := httptest.NewServer(New(Config{
		APIKey:                "perf-upstream-key",
		Models:                []string{"bench-responses-upstream"},
		ResponseText:          "stream body",
		ResponsesInputTokens:  33,
		ResponsesOutputTokens: 44,
		StreamChunks:          2,
	}).Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", strings.NewReader(`{"model":"bench-responses-upstream","stream":true}`))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer perf-upstream-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("unexpected content type: %s", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "event: response.completed") {
		t.Fatalf("missing completed event: %s", text)
	}
	if !strings.Contains(text, `"input_tokens":33`) {
		t.Fatalf("missing usage payload: %s", text)
	}
	if !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("missing done sentinel: %s", text)
	}
}

func TestHandleModelsCanInjectRetryableFailures(t *testing.T) {
	server := httptest.NewServer(New(Config{
		APIKey:          "perf-upstream-key",
		RetryableRate:   1,
		RetryableStatus: http.StatusTooManyRequests,
	}).Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/models", nil)
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer perf-upstream-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}
