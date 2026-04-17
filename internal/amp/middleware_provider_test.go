package amp

import (
	"net/http"
	"testing"

	"ampmanager/internal/model"
)

func TestDetectRequestedProvider(t *testing.T) {
	tests := []struct {
		method string
		path   string
		header http.Header
		want   ProviderKind
		ok     bool
	}{
		{method: http.MethodPost, path: "/v1/responses", header: http.Header{}, want: ProviderOpenAIResponses, ok: true},
		{method: http.MethodPost, path: "/v1/chat/completions", header: http.Header{}, want: ProviderOpenAIChat, ok: true},
		{method: http.MethodPost, path: "/v1/messages", header: http.Header{}, want: ProviderAnthropic, ok: true},
		{method: http.MethodPost, path: "/v1beta/models/gemini-2.5-pro:generateContent", header: http.Header{}, want: ProviderGemini, ok: true},
		{method: http.MethodGet, path: "/api/provider/openai/v1/models", header: http.Header{}, want: ProviderOpenAIChat, ok: true},
		{method: http.MethodGet, path: "/api/user", header: http.Header{}, want: "", ok: false},
	}

	for _, tt := range tests {
		got, ok := detectRequestedProvider(tt.method, tt.path, tt.header)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("detectRequestedProvider(%q, %q) = (%q, %v), want (%q, %v)", tt.method, tt.path, got, ok, tt.want, tt.ok)
		}
	}
}

func TestAPIKeyAllowsProvider(t *testing.T) {
	key := &model.UserAPIKey{AllowedProviders: []string{"gemini", "anthropic"}}

	if !apiKeyAllowsProvider(key, ProviderGemini) {
		t.Fatal("expected gemini to be allowed")
	}
	if apiKeyAllowsProvider(key, ProviderOpenAIChat) {
		t.Fatal("expected openai chat to be blocked")
	}
	if !apiKeyAllowsProvider(&model.UserAPIKey{}, ProviderOpenAIResponses) {
		t.Fatal("expected unrestricted key to allow provider")
	}
}
