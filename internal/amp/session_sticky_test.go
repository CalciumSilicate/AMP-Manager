package amp

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"ampmanager/internal/config"
	"ampmanager/internal/model"

	"github.com/alicebob/miniredis/v2"
)

func TestResolveSessionIDPrefersExplicitHeader(t *testing.T) {
	StopSessionStickyRuntime()
	defer StopSessionStickyRuntime()

	headers := http.Header{}
	headers.Set("X-Session-Id", "sess-explicit")
	body := []byte(`{"messages":[{"content":"hello"}]}`)

	got := resolveSessionIDFromHeadersAndBody(context.Background(), headers, body, "", "key-1", "1.2.3.4")
	if got != "sess-explicit" {
		t.Fatalf("session id = %q, want explicit header", got)
	}
}

func TestResolveSessionIDReusesFingerprintSeed(t *testing.T) {
	mr := miniredis.RunT(t)
	StopSessionStickyRuntime()
	InitSessionStickyRuntime(&config.Config{
		RedisURL:    fmt.Sprintf("redis://%s/0", mr.Addr()),
		RedisPrefix: "session-stick-test",
	})
	UpdateSessionStickyConfig(model.SessionStickyConfigResponse{
		Enabled:           true,
		WindowMinutes:     5,
		LogSearchMinChars: 4,
	})
	t.Cleanup(func() {
		StopSessionStickyRuntime()
	})

	headers := http.Header{}
	headers.Set("User-Agent", "codex-test")
	body := []byte(`{"messages":[{"content":"hello"},{"content":"world"}]}`)

	first := resolveSessionIDFromHeadersAndBody(context.Background(), headers, body, "", "key-1", "1.2.3.4")
	second := resolveSessionIDFromHeadersAndBody(context.Background(), headers, body, "", "key-1", "1.2.3.4")

	if first == "" || second == "" {
		t.Fatalf("expected non-empty generated session ids: first=%q second=%q", first, second)
	}
	if first != second {
		t.Fatalf("expected fingerprint mapping reuse, got %q and %q", first, second)
	}
}

func TestFinalizeSessionStickyBindsChannelID(t *testing.T) {
	mr := miniredis.RunT(t)
	StopSessionStickyRuntime()
	InitSessionStickyRuntime(&config.Config{
		RedisURL:    fmt.Sprintf("redis://%s/0", mr.Addr()),
		RedisPrefix: "session-stick-test",
	})
	UpdateSessionStickyConfig(model.SessionStickyConfigResponse{
		Enabled:           true,
		WindowMinutes:     5,
		LogSearchMinChars: 4,
	})
	t.Cleanup(func() {
		StopSessionStickyRuntime()
	})

	trace := NewRequestTrace("req-1", "user-1", "key-1", http.MethodPost, "/v1/chat/completions")
	trace.SetSessionID("sess-1")
	trace.SetChannel("channel-1", "openai", string(model.ChannelEndpointChatCompletions))
	trace.SetResponse(http.StatusOK)

	FinalizeSessionSticky(context.Background(), trace)

	binding, ttl, ok := GetSessionStickyBinding(context.Background(), "sess-1")
	if !ok {
		t.Fatal("expected sticky binding to exist")
	}
	if binding != "channel-1" {
		t.Fatalf("binding = %q, want channel id", binding)
	}
	if ttl <= 0 {
		t.Fatalf("expected positive ttl, got %v", ttl)
	}
}

func TestBuildRequestSessionReadsLegacyProviderAndChannelBindings(t *testing.T) {
	mr := miniredis.RunT(t)
	StopSessionStickyRuntime()
	InitSessionStickyRuntime(&config.Config{
		RedisURL:    fmt.Sprintf("redis://%s/0", mr.Addr()),
		RedisPrefix: "session-stick-test",
	})
	UpdateSessionStickyConfig(model.SessionStickyConfigResponse{
		Enabled:           true,
		WindowMinutes:     5,
		LogSearchMinChars: 4,
	})
	t.Cleanup(func() {
		StopSessionStickyRuntime()
	})

	if err := mr.Set(sessionStickyBindingKey("sess-legacy"), "openai"); err != nil {
		t.Fatalf("set legacy binding: %v", err)
	}
	if err := mr.Set(sessionStickyBindingKey("sess-channel"), "channel-9"); err != nil {
		t.Fatalf("set channel binding: %v", err)
	}

	legacyHeaders := http.Header{}
	legacyHeaders.Set("X-Session-Id", "sess-legacy")
	legacySession := BuildRequestSession(context.Background(), legacyHeaders, nil, "")
	if legacySession == nil {
		t.Fatal("expected legacy session")
	}
	if legacySession.StickyProvider != "openai" {
		t.Fatalf("legacy sticky provider = %q, want openai", legacySession.StickyProvider)
	}
	if legacySession.StickyChannelID != "" {
		t.Fatalf("legacy sticky channel = %q, want empty", legacySession.StickyChannelID)
	}

	channelHeaders := http.Header{}
	channelHeaders.Set("X-Session-Id", "sess-channel")
	channelSession := BuildRequestSession(context.Background(), channelHeaders, nil, "")
	if channelSession == nil {
		t.Fatal("expected channel session")
	}
	if channelSession.StickyChannelID != "channel-9" {
		t.Fatalf("sticky channel = %q, want channel-9", channelSession.StickyChannelID)
	}
	if channelSession.StickyProvider != "" {
		t.Fatalf("sticky provider = %q, want empty", channelSession.StickyProvider)
	}
}
