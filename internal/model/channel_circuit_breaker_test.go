package model

import (
	"testing"
	"time"
)

func TestChannelCircuitBreakerIgnores429AndCountsFailures(t *testing.T) {
	channel := &Channel{
		CircuitBreakerThreshold:       2,
		CircuitBreakerOpenMinutes:     10,
		CircuitBreakerHalfOpenMinutes: 2,
	}
	now := time.Date(2026, time.April, 19, 10, 0, 0, 0, time.UTC)

	if changed := ApplyChannelCircuitBreakerOutcome(channel, ClassifyChannelCircuitBreakerOutcome(429, "upstream_error"), now); changed {
		t.Fatalf("429 should not change breaker state")
	}
	if channel.CircuitBreakerConsecutiveErrors != 0 || channel.CircuitBreakerState != ChannelCircuitBreakerStateClosed {
		t.Fatalf("unexpected state after 429: %+v", channel)
	}

	ApplyChannelCircuitBreakerOutcome(channel, ClassifyChannelCircuitBreakerOutcome(500, "upstream_error"), now)
	if channel.CircuitBreakerConsecutiveErrors != 1 || channel.CircuitBreakerState != ChannelCircuitBreakerStateClosed {
		t.Fatalf("first failure should increment only: %+v", channel)
	}

	ApplyChannelCircuitBreakerOutcome(channel, ClassifyChannelCircuitBreakerOutcome(502, "upstream_error"), now.Add(time.Second))
	if channel.CircuitBreakerState != ChannelCircuitBreakerStateOpen || channel.CircuitBreakerOpenedAt == nil {
		t.Fatalf("expected open breaker after threshold: %+v", channel)
	}
}

func TestChannelCircuitBreakerHalfOpenFailureReopensAndWindowSuccessCloses(t *testing.T) {
	base := time.Date(2026, time.April, 19, 10, 0, 0, 0, time.UTC)
	channel := &Channel{
		CircuitBreakerThreshold:       3,
		CircuitBreakerOpenMinutes:     10,
		CircuitBreakerHalfOpenMinutes: 2,
		CircuitBreakerState:           ChannelCircuitBreakerStateOpen,
		CircuitBreakerOpenedAt:        timePtr(base),
	}

	halfOpenStart := base.Add(10 * time.Minute)
	if !RefreshChannelCircuitBreakerState(channel, halfOpenStart) {
		t.Fatal("expected open breaker to move into half-open")
	}
	if channel.CircuitBreakerState != ChannelCircuitBreakerStateHalfOpen {
		t.Fatalf("expected half-open state, got %s", channel.CircuitBreakerState)
	}

	ApplyChannelCircuitBreakerOutcome(channel, ClassifyChannelCircuitBreakerOutcome(500, "upstream_error"), halfOpenStart.Add(30*time.Second))
	if channel.CircuitBreakerState != ChannelCircuitBreakerStateOpen || channel.CircuitBreakerOpenedAt == nil || !channel.CircuitBreakerOpenedAt.Equal(halfOpenStart.Add(30*time.Second)) {
		t.Fatalf("half-open failure should reopen breaker: %+v", channel)
	}

	channel.CircuitBreakerState = ChannelCircuitBreakerStateHalfOpen
	channel.CircuitBreakerOpenedAt = timePtr(base)
	channel.CircuitBreakerHalfOpenStartedAt = timePtr(halfOpenStart)
	channel.CircuitBreakerConsecutiveErrors = 3

	if changed := ApplyChannelCircuitBreakerOutcome(channel, ClassifyChannelCircuitBreakerOutcome(200, ""), halfOpenStart.Add(2*time.Minute)); !changed {
		t.Fatal("expected half-open window completion to close breaker")
	}
	if channel.CircuitBreakerState != ChannelCircuitBreakerStateClosed || channel.CircuitBreakerOpenedAt != nil || channel.CircuitBreakerHalfOpenStartedAt != nil {
		t.Fatalf("expected breaker to close after half-open window success: %+v", channel)
	}
}
