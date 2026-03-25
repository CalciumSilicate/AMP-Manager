package amp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestNonStreamingPipelineStripsMCPPrefixForAnthropic(t *testing.T) {
	pipeline := NewDefaultNonStreamingPipeline()

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"type":"tool_use","name":"mcp__tools__mcp-read"}`)),
	}
	ctx := &ResponseContext{
		Ctx:        WithClaudeToolNameMap(context.Background(), ClaudeToolNameMap{"mcp__tools__mcp-read": "Read"}),
		Provider:   ProviderInfo{Provider: ProviderAnthropic},
		Headers:    resp.Header,
		StatusCode: resp.StatusCode,
	}

	if err := pipeline.ProcessNonStreamingResponse(resp, ctx); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if got := gjson.GetBytes(out, "name").String(); got != "Read" {
		t.Fatalf("expected name=Read, got %q", got)
	}
}

func TestModifyResponseStripsMCPPrefixForAnthropicSSE(t *testing.T) {
	sse := "event: message\n" +
		"data: {\"type\":\"tool_use\",\"name\":\"mcp__tools__mcp-read\"}\n\n"

	req, _ := http.NewRequest("GET", "http://example.test", nil)
	req = req.WithContext(WithProviderInfo(req.Context(), ProviderInfo{Provider: ProviderAnthropic}))
	req = req.WithContext(WithClaudeToolNameMap(req.Context(), ClaudeToolNameMap{"mcp__tools__mcp-read": "Read"}))

	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
		Request:    req,
	}

	if err := modifyResponse(resp); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(string(out), `"name":"Read"`) {
		t.Fatalf("expected stripped name, got: %s", string(out))
	}
}
