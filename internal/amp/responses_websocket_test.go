package amp

import (
	"net/http"
	"testing"

	"ampmanager/internal/model"

	"github.com/tidwall/gjson"
)

func TestNormalizeResponsesCreateRequestRequiresModel(t *testing.T) {
	_, err := normalizeResponsesCreateRequest([]byte(`{"type":"response.create","input":[]}`))
	if err == nil {
		t.Fatalf("expected error for missing model")
	}
}

func TestNormalizeResponsesSubsequentRequestPreservesPreviousResponseID(t *testing.T) {
	lastRequest := []byte(`{"model":"gpt-5-codex","instructions":"test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)
	raw := []byte(`{"type":"response.append","previous_response_id":"resp-1","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"next"}]}]}`)

	normalized, err := normalizeResponsesSubsequentRequest(raw, lastRequest, []byte(`[]`), false)
	if err != nil {
		t.Fatalf("normalizeResponsesSubsequentRequest() error = %v", err)
	}
	if got := gjson.GetBytes(normalized.request, "previous_response_id").String(); got != "resp-1" {
		t.Fatalf("previous_response_id = %q, want %q", got, "resp-1")
	}
	if got := gjson.GetBytes(normalized.request, "model").String(); got != "gpt-5-codex" {
		t.Fatalf("model = %q, want %q", got, "gpt-5-codex")
	}
	if !gjson.GetBytes(normalized.request, "stream").Bool() {
		t.Fatalf("stream should be true")
	}
}

func TestBuildResponsesUpstreamWebsocketHeadersEnsuresResponsesBeta(t *testing.T) {
	headers := buildResponsesUpstreamWebsocketHeaders(http.Header{
		"Originator": []string{"Codex Desktop"},
	}, &model.Channel{
		APIKey:      "sk-test",
		HeadersJSON: `{"X-Custom":"1"}`,
	})

	if got := headers.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := headers.Get("OpenAI-Beta"); got != "responses_websockets=2026-02-06" {
		t.Fatalf("OpenAI-Beta = %q", got)
	}
	if got := headers.Get("Originator"); got != "Codex Desktop" {
		t.Fatalf("Originator = %q", got)
	}
	if got := headers.Get("X-Custom"); got != "1" {
		t.Fatalf("X-Custom = %q", got)
	}
}
