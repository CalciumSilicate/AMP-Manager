package amp

import (
	"ampmanager/internal/model"
	"bytes"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestApplyClaudeCLISimulationMatchesCurrentFingerprint(t *testing.T) {
	req := httptest.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBufferString(`{"ok":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Anthropic-Beta", "custom-beta-1, interleaved-thinking-2025-05-14")
	req.Header.Set("User-Agent", "HW/JS 0.78.0")
	req.Header.Set("X-Amp-Client-Type", "cli")
	req.Header.Set("X-Amp-Thread-Id", "T-123")
	req.Header.Set("Cf-Connecting-Ip", "1.2.3.4")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-Ip", "1.2.3.4")
	req.Header.Set("X-Stainless-Helper-Method", "stream")
	req.Header.Set("X-Stainless-Package-Version", "0.78.0")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("X-Api-Key", "should-be-removed")

	applyClaudeCLISimulation(req, true)

	if got := req.Header.Get("User-Agent"); got != "claude-cli/2.1.81 (external, cli)" {
		t.Fatalf("unexpected User-Agent: %q", got)
	}
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("unexpected Accept: %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected Content-Type: %q", got)
	}
	if got := req.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("unexpected Content-Encoding: %q", got)
	}
	if got := req.Header.Get("Accept-Encoding"); got != "gzip, br" {
		t.Fatalf("unexpected Accept-Encoding: %q", got)
	}
	if got := req.Header.Get("Accept-Language"); got != "*" {
		t.Fatalf("unexpected Accept-Language: %q", got)
	}
	if got := req.Header.Get("Sec-Fetch-Mode"); got != "cors" {
		t.Fatalf("unexpected Sec-Fetch-Mode: %q", got)
	}
	if got := req.Header.Get("Anthropic-Dangerous-Direct-Browser-Access"); got != "true" {
		t.Fatalf("unexpected Anthropic-Dangerous-Direct-Browser-Access: %q", got)
	}
	if got := req.Header.Get("Anthropic-Beta"); got != "claude-code-20250219,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24" {
		t.Fatalf("unexpected Anthropic-Beta: %q", got)
	}
	if got := req.Header.Get("X-Stainless-Package-Version"); got != "0.74.0" {
		t.Fatalf("unexpected X-Stainless-Package-Version: %q", got)
	}
	if got := req.Header.Get("X-Stainless-Runtime-Version"); got != "v22.17.0" {
		t.Fatalf("unexpected X-Stainless-Runtime-Version: %q", got)
	}
	if got := req.Header.Get("X-Stainless-Runtime"); got != "node" {
		t.Fatalf("unexpected X-Stainless-Runtime: %q", got)
	}
	if got := req.Header.Get("X-Stainless-Lang"); got != "js" {
		t.Fatalf("unexpected X-Stainless-Lang: %q", got)
	}
	if got := req.Header.Get("X-Stainless-Timeout"); got != "600" {
		t.Fatalf("unexpected X-Stainless-Timeout: %q", got)
	}
	if got := req.Header.Get("X-Stainless-Helper-Method"); got != "" {
		t.Fatalf("expected X-Stainless-Helper-Method to be stripped, got %q", got)
	}
	if got := req.Header.Get("X-Amp-Client-Type"); got != "" {
		t.Fatalf("expected X-Amp-Client-Type to be stripped, got %q", got)
	}
	if got := req.Header.Get("X-Amp-Thread-Id"); got != "" {
		t.Fatalf("expected X-Amp-Thread-Id to be stripped, got %q", got)
	}
	if got := req.Header.Get("Cf-Connecting-Ip"); got != "" {
		t.Fatalf("expected Cf-Connecting-Ip to be stripped, got %q", got)
	}
	if got := req.Header.Get("X-Forwarded-For"); got != "" {
		t.Fatalf("expected X-Forwarded-For to be stripped, got %q", got)
	}
	if got := req.Header.Get("X-Real-Ip"); got != "" {
		t.Fatalf("expected X-Real-Ip to be stripped, got %q", got)
	}
	if got := req.Header.Get("X-Api-Key"); got != "" {
		t.Fatalf("expected X-Api-Key to be stripped, got %q", got)
	}

	expectedOS := mapStainlessOS()
	if got := req.Header.Get("X-Stainless-Os"); got != expectedOS {
		t.Fatalf("unexpected X-Stainless-Os: %q", got)
	}

	expectedArch := mapStainlessArch()
	if got := req.Header.Get("X-Stainless-Arch"); got != expectedArch {
		t.Fatalf("unexpected X-Stainless-Arch: %q", got)
	}

	if runtime.GOOS == "windows" && req.Header.Get("X-Stainless-Os") != "Windows" {
		t.Fatalf("expected Windows OS fingerprint on windows, got %q", req.Header.Get("X-Stainless-Os"))
	}
}

func TestApplyClaudeCLISimulationStrictWhitelistDropsNonEssentialHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBufferString(`{"ok":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Amp-Client-Application", "VS Code CLI")
	req.Header.Set("X-Stainless-Helper-Method", "stream")
	req.Header.Set("Cf-Ray", "abc")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Cdn-Loop", "cloudflare; loops=1")
	req.Header.Set("User-Agent", "HW/JS 0.78.0")
	req.Header.Set("X-Debug", "keep-me-if-not-strict")
	req.Header.Set("Traceparent", "00-abc-123-01")

	applyClaudeCLISimulation(req, true)

	for _, key := range []string{"X-Amp-Client-Application", "X-Stainless-Helper-Method", "Cf-Ray", "X-Forwarded-Proto", "Cdn-Loop", "X-Debug", "Traceparent"} {
		if got := req.Header.Get(key); got != "" {
			t.Fatalf("expected %s to be stripped, got %q", key, got)
		}
	}

	if got := req.Header.Get("User-Agent"); got != "claude-cli/2.1.81 (external, cli)" {
		t.Fatalf("expected User-Agent to be rewritten under strict whitelist, got %q", got)
	}

	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type to survive strict whitelist, got %q", got)
	}
}

func TestApplyClaudeCLISimulationThenChannelAuthKeepsAnthropicCredentials(t *testing.T) {
	req := httptest.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBufferString(`{"ok":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "client-key-should-be-replaced")

	applyClaudeCLISimulation(req, true)
	applyChannelAuth(&model.Channel{Type: model.ChannelTypeClaude, APIKey: "channel-secret"}, req)

	if got := req.Header.Get("x-api-key"); got != "channel-secret" {
		t.Fatalf("expected channel auth header to survive Claude CLI simulation, got %q", got)
	}
	if got := req.Header.Get("anthropic-version"); got != "2023-06-01" {
		t.Fatalf("expected anthropic-version to be present after channel auth, got %q", got)
	}
}
