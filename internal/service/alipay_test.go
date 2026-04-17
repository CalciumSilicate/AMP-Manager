package service

import (
	"context"
	"errors"
	"testing"
)

func TestIsRetryableAlipayError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{err: errors.New(`Post "https://openapi-sandbox.dl.alipaydev.com/gateway.do": dial tcp 110.75.132.25:443: i/o timeout`), want: true},
		{err: errors.New(`Post "https://openapi-sandbox.dl.alipaydev.com/gateway.do": read tcp 172.27.0.4:35528->110.75.132.25:443: read: connection reset by peer`), want: true},
		{err: errors.New("unexpected EOF"), want: true},
		{err: context.Canceled, want: false},
		{err: context.DeadlineExceeded, want: false},
		{err: errors.New("certificate verify failed"), want: false},
	}

	for _, tt := range tests {
		if got := isRetryableAlipayError(tt.err); got != tt.want {
			t.Fatalf("isRetryableAlipayError(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestRetryAlipayCallStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	attempts := 0
	_, err := retryAlipayCall(ctx, func(context.Context) (string, error) {
		attempts++
		return "", errors.New("i/o timeout")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}
