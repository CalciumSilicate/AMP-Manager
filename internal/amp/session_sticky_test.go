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
