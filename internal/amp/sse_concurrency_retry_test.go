package amp

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestSSEConcurrencyRetryWrapper_RetriesAfterKeepAliveFrames(t *testing.T) {
	oldWait := sseConcurrencyRetryBaseWait
	oldSleep := sseConcurrencyRetrySleep
	sseConcurrencyRetryBaseWait = 0
	sseConcurrencyRetrySleep = func(time.Duration) {}
	defer func() {
		sseConcurrencyRetryBaseWait = oldWait
		sseConcurrencyRetrySleep = oldSleep
	}()

	firstStream := strings.Join([]string{
		":",
		"",
		":",
		"",
		"event: error",
		"data: {\"error\":{\"message\":\"Concurrency limit exceeded for account, please retry later\",\"type\":\"rate_limit_error\"}}",
		"",
		"",
	}, "\n")

	secondStream := strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}",
		"",
		"",
	}, "\n")

	retryCalls := 0
	wrapped := NewSSEConcurrencyRetryWrapper(io.NopCloser(strings.NewReader(firstStream)), func() (io.ReadCloser, error) {
		retryCalls++
		return io.NopCloser(strings.NewReader(secondStream)), nil
	})

	out, err := io.ReadAll(wrapped)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if retryCalls != 1 {
		t.Fatalf("expected retry to be called once, got %d", retryCalls)
	}

	outStr := string(out)
	if strings.Contains(outStr, "Concurrency limit exceeded") {
		t.Fatalf("expected retryable error frame to be swallowed, got: %s", outStr)
	}
	if !strings.Contains(outStr, ":\n\n") {
		t.Fatalf("expected keepalive frames to be forwarded before retry, got: %s", outStr)
	}
	if !strings.Contains(outStr, "response.output_text.delta") {
		t.Fatalf("expected retried stream data, got: %s", outStr)
	}
}

func TestIsSSERetryableError_CaseInsensitiveConcurrencyMessage(t *testing.T) {
	frame := []byte("event: error\ndata: {\"error\":{\"message\":\"concurrency limit exceeded for account\",\"type\":\"rate_limit_error\"}}\n\n")
	if !isSSERetryableError(frame) {
		t.Fatalf("expected frame to be retryable")
	}
}

func TestSSEConcurrencyRetryWrapper_DoesNotRetryAfterPreludeFrame(t *testing.T) {
	oldWait := sseConcurrencyRetryBaseWait
	oldSleep := sseConcurrencyRetrySleep
	sseConcurrencyRetryBaseWait = 0
	sseConcurrencyRetrySleep = func(time.Duration) {}
	defer func() {
		sseConcurrencyRetryBaseWait = oldWait
		sseConcurrencyRetrySleep = oldSleep
	}()

	firstStream := strings.Join([]string{
		"event: response.created",
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}",
		"",
		"event: response.in_progress",
		"data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_1\"}}",
		"",
		"event: response.output_item.added",
		"data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"it_1\"}}",
		"",
		":",
		"",
		"data: {\"error\":{\"code\":\"stream_read_error\",\"message\":\"stream_read_error\",\"type\":\"upstream_error\"},\"sequence_number\":0,\"type\":\"error\"}",
		"",
		"",
	}, "\n")

	retryCalls := 0
	wrapped := NewSSEConcurrencyRetryWrapper(io.NopCloser(strings.NewReader(firstStream)), func() (io.ReadCloser, error) {
		retryCalls++
		return io.NopCloser(strings.NewReader("")), nil
	})

	out, err := io.ReadAll(wrapped)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if retryCalls != 0 {
		t.Fatalf("expected no retry after prelude frame was forwarded, got %d", retryCalls)
	}

	outStr := string(out)
	if !strings.Contains(outStr, "response.created") {
		t.Fatalf("expected prelude frame to be forwarded immediately, got: %s", outStr)
	}
	if !strings.Contains(outStr, "stream_read_error") {
		t.Fatalf("expected retryable error to pass through after prelude frame, got: %s", outStr)
	}
}

func TestSSEConcurrencyRetryWrapper_ForwardsPreludeFrameImmediately(t *testing.T) {
	stream := strings.Join([]string{
		"event: response.created",
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}",
		"",
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}",
		"",
		"",
	}, "\n")

	retryCalls := 0
	wrapped := NewSSEConcurrencyRetryWrapper(io.NopCloser(strings.NewReader(stream)), func() (io.ReadCloser, error) {
		retryCalls++
		return io.NopCloser(strings.NewReader("")), nil
	})

	buf := make([]byte, 256)
	n, err := wrapped.Read(buf)
	if err != nil {
		t.Fatalf("first read failed: %v", err)
	}

	firstChunk := string(buf[:n])
	if retryCalls != 0 {
		t.Fatalf("expected no retry while forwarding prelude frame, got %d", retryCalls)
	}
	if !strings.Contains(firstChunk, "response.created") {
		t.Fatalf("expected first read to return prelude frame, got: %s", firstChunk)
	}
	if strings.Contains(firstChunk, "response.output_text.delta") {
		t.Fatalf("expected prelude frame to flush before substantive delta, got: %s", firstChunk)
	}
}

func TestSSEConcurrencyRetryWrapper_DoesNotRetryAfterSubstantiveDelta(t *testing.T) {
	oldWait := sseConcurrencyRetryBaseWait
	oldSleep := sseConcurrencyRetrySleep
	sseConcurrencyRetryBaseWait = 0
	sseConcurrencyRetrySleep = func(time.Duration) {}
	defer func() {
		sseConcurrencyRetryBaseWait = oldWait
		sseConcurrencyRetrySleep = oldSleep
	}()

	stream := strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}",
		"",
		"data: {\"error\":{\"code\":\"stream_read_error\",\"message\":\"stream_read_error\",\"type\":\"upstream_error\"},\"type\":\"error\"}",
		"",
		"",
	}, "\n")

	retryCalls := 0
	wrapped := NewSSEConcurrencyRetryWrapper(io.NopCloser(strings.NewReader(stream)), func() (io.ReadCloser, error) {
		retryCalls++
		return io.NopCloser(strings.NewReader("")), nil
	})

	out, err := io.ReadAll(wrapped)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if retryCalls != 0 {
		t.Fatalf("expected no retry after substantive delta, got %d", retryCalls)
	}

	outStr := string(out)
	if !strings.Contains(outStr, "response.output_text.delta") || !strings.Contains(outStr, "stream_read_error") {
		t.Fatalf("expected original stream to pass through, got: %s", outStr)
	}
}
