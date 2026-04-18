package model

import (
	"testing"
	"time"
)

func TestAPIKeyCircuitBreakerIgnores429AndCountsFailures(t *testing.T) {
	key := &UserAPIKey{
		CircuitBreakerThreshold:       2,
		CircuitBreakerOpenMinutes:     10,
		CircuitBreakerHalfOpenMinutes: 2,
	}
	now := time.Date(2026, time.April, 19, 10, 0, 0, 0, time.UTC)

	if changed := ApplyAPIKeyCircuitBreakerOutcome(key, ClassifyAPIKeyCircuitBreakerOutcome(429, "upstream_error"), now); changed {
		t.Fatalf("429 should not change breaker state")
	}
	if key.CircuitBreakerConsecutiveErrors != 0 || key.CircuitBreakerState != APIKeyCircuitBreakerStateClosed {
		t.Fatalf("unexpected state after 429: %+v", key)
	}

	ApplyAPIKeyCircuitBreakerOutcome(key, ClassifyAPIKeyCircuitBreakerOutcome(500, "upstream_error"), now)
	if key.CircuitBreakerConsecutiveErrors != 1 || key.CircuitBreakerState != APIKeyCircuitBreakerStateClosed {
		t.Fatalf("first failure should increment only: %+v", key)
	}

	ApplyAPIKeyCircuitBreakerOutcome(key, ClassifyAPIKeyCircuitBreakerOutcome(502, "upstream_error"), now.Add(time.Second))
	if key.CircuitBreakerState != APIKeyCircuitBreakerStateOpen || key.CircuitBreakerOpenedAt == nil {
		t.Fatalf("expected open breaker after threshold: %+v", key)
	}
}

func TestAPIKeyCircuitBreakerSuccessResetsConsecutiveErrors(t *testing.T) {
	key := &UserAPIKey{
		CircuitBreakerThreshold:         50,
		CircuitBreakerOpenMinutes:       10,
		CircuitBreakerHalfOpenMinutes:   2,
		CircuitBreakerState:             APIKeyCircuitBreakerStateClosed,
		CircuitBreakerConsecutiveErrors: 7,
	}

	ApplyAPIKeyCircuitBreakerOutcome(key, ClassifyAPIKeyCircuitBreakerOutcome(200, ""), time.Now().UTC())
	if key.CircuitBreakerConsecutiveErrors != 0 {
		t.Fatalf("success should reset consecutive errors, got %d", key.CircuitBreakerConsecutiveErrors)
	}
}

func TestAPIKeyCircuitBreakerHalfOpenFailureReopensAndWindowSuccessCloses(t *testing.T) {
	base := time.Date(2026, time.April, 19, 10, 0, 0, 0, time.UTC)
	key := &UserAPIKey{
		CircuitBreakerThreshold:       3,
		CircuitBreakerOpenMinutes:     10,
		CircuitBreakerHalfOpenMinutes: 2,
		CircuitBreakerState:           APIKeyCircuitBreakerStateOpen,
		CircuitBreakerOpenedAt:        timePtr(base),
	}

	halfOpenStart := base.Add(10 * time.Minute)
	if !RefreshAPIKeyCircuitBreakerState(key, halfOpenStart) {
		t.Fatal("expected open breaker to move into half-open")
	}
	if key.CircuitBreakerState != APIKeyCircuitBreakerStateHalfOpen {
		t.Fatalf("expected half-open state, got %s", key.CircuitBreakerState)
	}

	ApplyAPIKeyCircuitBreakerOutcome(key, ClassifyAPIKeyCircuitBreakerOutcome(500, "upstream_error"), halfOpenStart.Add(30*time.Second))
	if key.CircuitBreakerState != APIKeyCircuitBreakerStateOpen || key.CircuitBreakerOpenedAt == nil || !key.CircuitBreakerOpenedAt.Equal(halfOpenStart.Add(30*time.Second)) {
		t.Fatalf("half-open failure should reopen breaker: %+v", key)
	}

	key.CircuitBreakerState = APIKeyCircuitBreakerStateHalfOpen
	key.CircuitBreakerOpenedAt = timePtr(base)
	key.CircuitBreakerHalfOpenStartedAt = timePtr(halfOpenStart)
	key.CircuitBreakerConsecutiveErrors = 3

	if changed := ApplyAPIKeyCircuitBreakerOutcome(key, ClassifyAPIKeyCircuitBreakerOutcome(200, ""), halfOpenStart.Add(2*time.Minute)); !changed {
		t.Fatal("expected half-open window completion to close breaker")
	}
	if key.CircuitBreakerState != APIKeyCircuitBreakerStateClosed || key.CircuitBreakerOpenedAt != nil || key.CircuitBreakerHalfOpenStartedAt != nil {
		t.Fatalf("expected breaker to close after half-open window success: %+v", key)
	}
}
