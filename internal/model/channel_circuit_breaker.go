package model

import (
	"strings"
	"time"
)

type ChannelCircuitBreakerOutcome string

const (
	ChannelCircuitBreakerOutcomeSuccess ChannelCircuitBreakerOutcome = "success"
	ChannelCircuitBreakerOutcomeFailure ChannelCircuitBreakerOutcome = "failure"
	ChannelCircuitBreakerOutcomeIgnored ChannelCircuitBreakerOutcome = "ignored"
)

const (
	ChannelCircuitBreakerStateClosed   = "closed"
	ChannelCircuitBreakerStateOpen     = "open"
	ChannelCircuitBreakerStateHalfOpen = "half_open"
)

const (
	DefaultChannelCircuitBreakerThreshold       = 50
	DefaultChannelCircuitBreakerOpenMinutes     = 10
	DefaultChannelCircuitBreakerHalfOpenMinutes = 2
)

func ApplyDefaultChannelCircuitBreakerConfig(channel *Channel) {
	if channel == nil {
		return
	}
	if channel.CircuitBreakerThreshold <= 0 {
		channel.CircuitBreakerThreshold = DefaultChannelCircuitBreakerThreshold
	}
	if channel.CircuitBreakerOpenMinutes <= 0 {
		channel.CircuitBreakerOpenMinutes = DefaultChannelCircuitBreakerOpenMinutes
	}
	if channel.CircuitBreakerHalfOpenMinutes <= 0 {
		channel.CircuitBreakerHalfOpenMinutes = DefaultChannelCircuitBreakerHalfOpenMinutes
	}
	if strings.TrimSpace(channel.CircuitBreakerState) == "" {
		channel.CircuitBreakerState = ChannelCircuitBreakerStateClosed
	}
}

func RefreshChannelCircuitBreakerState(channel *Channel, now time.Time) bool {
	if channel == nil {
		return false
	}

	ApplyDefaultChannelCircuitBreakerConfig(channel)
	changed := false

	switch channel.CircuitBreakerState {
	case ChannelCircuitBreakerStateOpen:
		if channel.CircuitBreakerOpenedAt == nil {
			channel.CircuitBreakerState = ChannelCircuitBreakerStateClosed
			channel.CircuitBreakerConsecutiveErrors = 0
			channel.CircuitBreakerHalfOpenStartedAt = nil
			return true
		}

		halfOpenStart := channel.CircuitBreakerOpenedAt.Add(time.Duration(channel.CircuitBreakerOpenMinutes) * time.Minute)
		halfOpenEnd := halfOpenStart.Add(time.Duration(channel.CircuitBreakerHalfOpenMinutes) * time.Minute)
		switch {
		case !now.Before(halfOpenEnd):
			closeChannelCircuitBreaker(channel)
			changed = true
		case !now.Before(halfOpenStart):
			channel.CircuitBreakerState = ChannelCircuitBreakerStateHalfOpen
			channel.CircuitBreakerHalfOpenStartedAt = timePtr(halfOpenStart)
			changed = true
		}
	case ChannelCircuitBreakerStateHalfOpen:
		if channel.CircuitBreakerHalfOpenStartedAt == nil {
			if channel.CircuitBreakerOpenedAt != nil {
				startedAt := channel.CircuitBreakerOpenedAt.Add(time.Duration(channel.CircuitBreakerOpenMinutes) * time.Minute)
				channel.CircuitBreakerHalfOpenStartedAt = timePtr(startedAt)
			} else {
				channel.CircuitBreakerHalfOpenStartedAt = timePtr(now)
			}
			changed = true
		}
		if channel.CircuitBreakerHalfOpenStartedAt != nil {
			halfOpenEnd := channel.CircuitBreakerHalfOpenStartedAt.Add(time.Duration(channel.CircuitBreakerHalfOpenMinutes) * time.Minute)
			if !now.Before(halfOpenEnd) {
				closeChannelCircuitBreaker(channel)
				changed = true
			}
		}
	default:
		if channel.CircuitBreakerState != ChannelCircuitBreakerStateClosed {
			channel.CircuitBreakerState = ChannelCircuitBreakerStateClosed
			changed = true
		}
	}

	return changed
}

func ApplyChannelCircuitBreakerOutcome(channel *Channel, outcome ChannelCircuitBreakerOutcome, now time.Time) bool {
	if channel == nil {
		return false
	}

	changed := RefreshChannelCircuitBreakerState(channel, now)
	switch outcome {
	case ChannelCircuitBreakerOutcomeIgnored:
		return changed
	case ChannelCircuitBreakerOutcomeSuccess:
		if channel.CircuitBreakerConsecutiveErrors != 0 {
			channel.CircuitBreakerConsecutiveErrors = 0
			changed = true
		}
		if channel.CircuitBreakerState == ChannelCircuitBreakerStateHalfOpen && channel.CircuitBreakerHalfOpenStartedAt != nil {
			halfOpenEnd := channel.CircuitBreakerHalfOpenStartedAt.Add(time.Duration(channel.CircuitBreakerHalfOpenMinutes) * time.Minute)
			if !now.Before(halfOpenEnd) {
				closeChannelCircuitBreaker(channel)
				changed = true
			}
		}
		return changed
	case ChannelCircuitBreakerOutcomeFailure:
		switch channel.CircuitBreakerState {
		case ChannelCircuitBreakerStateHalfOpen:
			openChannelCircuitBreaker(channel, now)
			return true
		case ChannelCircuitBreakerStateOpen:
			return changed
		default:
			channel.CircuitBreakerConsecutiveErrors++
			changed = true
			if channel.CircuitBreakerConsecutiveErrors >= channel.CircuitBreakerThreshold {
				openChannelCircuitBreaker(channel, now)
			}
			return true
		}
	default:
		return changed
	}
}

func ClassifyChannelCircuitBreakerOutcome(statusCode int, errorType string) ChannelCircuitBreakerOutcome {
	trimmedErrorType := strings.TrimSpace(errorType)

	switch {
	case statusCode >= 200 && statusCode < 300 && trimmedErrorType == "":
		return ChannelCircuitBreakerOutcomeSuccess
	case statusCode == 429 || statusCode == 499:
		return ChannelCircuitBreakerOutcomeIgnored
	case isChannelCircuitBreakerNetworkError(trimmedErrorType):
		return ChannelCircuitBreakerOutcomeFailure
	case statusCode < 200 || statusCode >= 300:
		return ChannelCircuitBreakerOutcomeFailure
	case trimmedErrorType != "":
		return ChannelCircuitBreakerOutcomeFailure
	default:
		return ChannelCircuitBreakerOutcomeIgnored
	}
}

func IsChannelCircuitBreakerBlocked(channel *Channel, now time.Time) bool {
	if channel == nil {
		return false
	}
	RefreshChannelCircuitBreakerState(channel, now)
	return channel.CircuitBreakerState == ChannelCircuitBreakerStateOpen
}

func openChannelCircuitBreaker(channel *Channel, now time.Time) {
	channel.CircuitBreakerState = ChannelCircuitBreakerStateOpen
	channel.CircuitBreakerOpenedAt = timePtr(now.UTC())
	channel.CircuitBreakerHalfOpenStartedAt = nil
	if channel.CircuitBreakerConsecutiveErrors < channel.CircuitBreakerThreshold {
		channel.CircuitBreakerConsecutiveErrors = channel.CircuitBreakerThreshold
	}
}

func closeChannelCircuitBreaker(channel *Channel) {
	channel.CircuitBreakerState = ChannelCircuitBreakerStateClosed
	channel.CircuitBreakerConsecutiveErrors = 0
	channel.CircuitBreakerOpenedAt = nil
	channel.CircuitBreakerHalfOpenStartedAt = nil
}

func isChannelCircuitBreakerNetworkError(errorType string) bool {
	if errorType == "" {
		return false
	}
	if strings.HasPrefix(errorType, "ws_") {
		return true
	}
	switch errorType {
	case "upstream_request_failed", "stream_timeout":
		return true
	default:
		return false
	}
}
