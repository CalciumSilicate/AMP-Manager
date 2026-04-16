package amp

import (
	"bytes"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestReadRequestBodyWithLimitRejectsOversizedBodies(t *testing.T) {
	body, err := readRequestBodyWithLimit(ioNopCloserString("abcdef"), 3)
	if err == nil {
		t.Fatalf("expected error, got body %q", string(body))
	}
	if !isRequestBodyTooLarge(err) {
		t.Fatalf("expected request body too large error, got %v", err)
	}
}

func TestRestoreRequestBodyUpdatesContentLengthHeader(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.test", bytes.NewBufferString("hello"))
	restoreRequestBody(req, []byte("updated-body"))

	if got, want := req.ContentLength, int64(len("updated-body")); got != want {
		t.Fatalf("ContentLength = %d, want %d", got, want)
	}
	if got, want := req.Header.Get("Content-Length"), strconv.Itoa(len("updated-body")); got != want {
		t.Fatalf("Content-Length header = %q, want %q", got, want)
	}
}

func ioNopCloserString(value string) *readCloserStub {
	return &readCloserStub{Buffer: bytes.NewBufferString(value)}
}

type readCloserStub struct {
	*bytes.Buffer
}

func (r *readCloserStub) Close() error { return nil }
