package amp

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"testing"
)

func TestHandleNonStreamingResponseDecompressesAndStripsMCPPrefix(t *testing.T) {
	plain := []byte(`{"type":"tool_use","name":"mcp__tools__mcp-read"}`)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(plain); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	req, _ := http.NewRequestWithContext(
		WithClaudeToolNameMap(
			WithProviderInfo(context.Background(), ProviderInfo{Provider: ProviderAnthropic}),
			ClaudeToolNameMap{"mcp__tools__mcp-read": "Read"},
		),
		http.MethodPost,
		"http://example.test/v1/messages",
		nil,
	)

	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Encoding": []string{"gzip"}, "Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
		Request:    req,
	}

	if err := handleNonStreamingResponse(resp, nil, nil, "claude-3-7-sonnet", "claude-3-7-sonnet"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if bytes.Contains(out, []byte("mcp__tools__mcp-read")) {
		t.Fatalf("expected prefix stripped, got: %s", string(out))
	}
	if !bytes.Contains(out, []byte(`"name":"Read"`)) {
		t.Fatalf("expected name=Read, got: %s", string(out))
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("expected Content-Encoding removed, got %q", got)
	}
}
