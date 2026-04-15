package amp

import "testing"

func TestGetParsedChannelHeadersCachesValidJSON(t *testing.T) {
	raw := `{"X-Test":"one","X-Trace":"two"}`

	first := getParsedChannelHeaders(raw)
	second := getParsedChannelHeaders(raw)

	if first == nil || len(first) != 2 {
		t.Fatalf("expected parsed headers, got %#v", first)
	}
	if first["X-Test"] != "one" || first["X-Trace"] != "two" {
		t.Fatalf("unexpected headers content: %#v", first)
	}
	if first["X-Test"] != second["X-Test"] || first["X-Trace"] != second["X-Trace"] {
		t.Fatal("expected cached header content to be stable")
	}
	cached, ok := channelHeadersCache.Load(raw)
	if !ok {
		t.Fatal("expected cache entry to exist")
	}
	entry := cached.(*parsedChannelHeaders)
	if entry.headers["X-Test"] != first["X-Test"] || entry.headers["X-Trace"] != first["X-Trace"] {
		t.Fatal("expected helper to reuse cached parsed headers")
	}
}

func TestGetParsedChannelHeadersReturnsNilForInvalidJSON(t *testing.T) {
	if headers := getParsedChannelHeaders("{bad-json"); headers != nil {
		t.Fatalf("expected nil headers for invalid json, got %#v", headers)
	}
}
