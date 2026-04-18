package model

import (
	"strings"
	"time"
)

type APIKeyCircuitBreakerOutcome string

const (
	APIKeyCircuitBreakerOutcomeSuccess APIKeyCircuitBreakerOutcome = "success"
	APIKeyCircuitBreakerOutcomeFailure APIKeyCircuitBreakerOutcome = "failure"
	APIKeyCircuitBreakerOutcomeIgnored APIKeyCircuitBreakerOutcome = "ignored"
)

func ApplyDefaultAPIKeyCircuitBreakerConfig(key *UserAPIKey) {
	if key == nil {
		return
	}
	if key.CircuitBreakerThreshold <= 0 {
		key.CircuitBreakerThreshold = DefaultAPIKeyCircuitBreakerThreshold
	}
	if key.CircuitBreakerOpenMinutes <= 0 {
		key.CircuitBreakerOpenMinutes = DefaultAPIKeyCircuitBreakerOpenMinutes
	}
	if key.CircuitBreakerHalfOpenMinutes <= 0 {
		key.CircuitBreakerHalfOpenMinutes = DefaultAPIKeyCircuitBreakerHalfOpenMinutes
	}
	if strings.TrimSpace(key.CircuitBreakerState) == "" {
		key.CircuitBreakerState = APIKeyCircuitBreakerStateClosed
	}
}

func RefreshAPIKeyCircuitBreakerState(key *UserAPIKey, now time.Time) bool {
	if key == nil {
		return false
	}

	ApplyDefaultAPIKeyCircuitBreakerConfig(key)
	changed := false

	switch key.CircuitBreakerState {
	case APIKeyCircuitBreakerStateOpen:
		if key.CircuitBreakerOpenedAt == nil {
			key.CircuitBreakerState = APIKeyCircuitBreakerStateClosed
			key.CircuitBreakerConsecutiveErrors = 0
			key.CircuitBreakerHalfOpenStartedAt = nil
			return true
		}

		halfOpenStart := key.CircuitBreakerOpenedAt.Add(time.Duration(key.CircuitBreakerOpenMinutes) * time.Minute)
		halfOpenEnd := halfOpenStart.Add(time.Duration(key.CircuitBreakerHalfOpenMinutes) * time.Minute)
		switch {
		case !now.Before(halfOpenEnd):
			closeAPIKeyCircuitBreaker(key)
			changed = true
		case !now.Before(halfOpenStart):
			key.CircuitBreakerState = APIKeyCircuitBreakerStateHalfOpen
			key.CircuitBreakerHalfOpenStartedAt = timePtr(halfOpenStart)
			changed = true
		}
	case APIKeyCircuitBreakerStateHalfOpen:
		if key.CircuitBreakerHalfOpenStartedAt == nil {
			if key.CircuitBreakerOpenedAt != nil {
				startedAt := key.CircuitBreakerOpenedAt.Add(time.Duration(key.CircuitBreakerOpenMinutes) * time.Minute)
				key.CircuitBreakerHalfOpenStartedAt = timePtr(startedAt)
			} else {
				key.CircuitBreakerHalfOpenStartedAt = timePtr(now)
			}
			changed = true
		}
		if key.CircuitBreakerHalfOpenStartedAt != nil {
			halfOpenEnd := key.CircuitBreakerHalfOpenStartedAt.Add(time.Duration(key.CircuitBreakerHalfOpenMinutes) * time.Minute)
			if !now.Before(halfOpenEnd) {
				closeAPIKeyCircuitBreaker(key)
				changed = true
			}
		}
	default:
		if key.CircuitBreakerState != APIKeyCircuitBreakerStateClosed {
			key.CircuitBreakerState = APIKeyCircuitBreakerStateClosed
			changed = true
		}
	}

	return changed
}

func ApplyAPIKeyCircuitBreakerOutcome(key *UserAPIKey, outcome APIKeyCircuitBreakerOutcome, now time.Time) bool {
	if key == nil {
		return false
	}

	changed := RefreshAPIKeyCircuitBreakerState(key, now)
	switch outcome {
	case APIKeyCircuitBreakerOutcomeIgnored:
		return changed
	case APIKeyCircuitBreakerOutcomeSuccess:
		if key.CircuitBreakerConsecutiveErrors != 0 {
			key.CircuitBreakerConsecutiveErrors = 0
			changed = true
		}
		if key.CircuitBreakerState == APIKeyCircuitBreakerStateHalfOpen && key.CircuitBreakerHalfOpenStartedAt != nil {
			halfOpenEnd := key.CircuitBreakerHalfOpenStartedAt.Add(time.Duration(key.CircuitBreakerHalfOpenMinutes) * time.Minute)
			if !now.Before(halfOpenEnd) {
				closeAPIKeyCircuitBreaker(key)
				changed = true
			}
		}
		return changed
	case APIKeyCircuitBreakerOutcomeFailure:
		switch key.CircuitBreakerState {
		case APIKeyCircuitBreakerStateHalfOpen:
			openAPIKeyCircuitBreaker(key, now)
			return true
		case APIKeyCircuitBreakerStateOpen:
			return changed
		default:
			key.CircuitBreakerConsecutiveErrors++
			changed = true
			if key.CircuitBreakerConsecutiveErrors >= key.CircuitBreakerThreshold {
				openAPIKeyCircuitBreaker(key, now)
			}
			return true
		}
	default:
		return changed
	}
}

func ClassifyAPIKeyCircuitBreakerOutcome(statusCode int, errorType string) APIKeyCircuitBreakerOutcome {
	trimmedErrorType := strings.TrimSpace(errorType)

	switch {
	case statusCode >= 200 && statusCode < 300 && trimmedErrorType == "":
		return APIKeyCircuitBreakerOutcomeSuccess
	case statusCode == 429 || statusCode == 499:
		return APIKeyCircuitBreakerOutcomeIgnored
	case isAPIKeyCircuitBreakerNetworkError(trimmedErrorType):
		return APIKeyCircuitBreakerOutcomeFailure
	case statusCode < 200 || statusCode >= 300:
		return APIKeyCircuitBreakerOutcomeFailure
	case trimmedErrorType != "":
		return APIKeyCircuitBreakerOutcomeFailure
	default:
		return APIKeyCircuitBreakerOutcomeIgnored
	}
}

func IsAPIKeyCircuitBreakerBlocked(key *UserAPIKey, now time.Time) bool {
	if key == nil {
		return false
	}
	RefreshAPIKeyCircuitBreakerState(key, now)
	return key.CircuitBreakerState == APIKeyCircuitBreakerStateOpen
}

func openAPIKeyCircuitBreaker(key *UserAPIKey, now time.Time) {
	key.CircuitBreakerState = APIKeyCircuitBreakerStateOpen
	key.CircuitBreakerOpenedAt = timePtr(now.UTC())
	key.CircuitBreakerHalfOpenStartedAt = nil
	if key.CircuitBreakerConsecutiveErrors < key.CircuitBreakerThreshold {
		key.CircuitBreakerConsecutiveErrors = key.CircuitBreakerThreshold
	}
}

func closeAPIKeyCircuitBreaker(key *UserAPIKey) {
	key.CircuitBreakerState = APIKeyCircuitBreakerStateClosed
	key.CircuitBreakerConsecutiveErrors = 0
	key.CircuitBreakerOpenedAt = nil
	key.CircuitBreakerHalfOpenStartedAt = nil
}

func isAPIKeyCircuitBreakerNetworkError(errorType string) bool {
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

func timePtr(value time.Time) *time.Time {
	v := value.UTC()
	return &v
}
