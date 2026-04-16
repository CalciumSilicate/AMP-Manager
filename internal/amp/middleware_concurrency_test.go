package amp

import "testing"

func TestActiveRequestLimiter(t *testing.T) {
	limiter := newActiveRequestLimiter()

	if !limiter.TryAcquire("user-1", 2) {
		t.Fatalf("expected first acquire to succeed")
	}
	if !limiter.TryAcquire("user-1", 2) {
		t.Fatalf("expected second acquire to succeed")
	}
	if limiter.TryAcquire("user-1", 2) {
		t.Fatalf("expected third acquire to fail")
	}

	limiter.Release("user-1")

	if !limiter.TryAcquire("user-1", 2) {
		t.Fatalf("expected acquire after release to succeed")
	}
}
