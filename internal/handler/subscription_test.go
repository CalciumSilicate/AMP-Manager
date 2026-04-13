package handler

import (
	"testing"
	"time"
)

func TestParseSubscriptionExpiresAt(t *testing.T) {
	t.Run("supports RFC3339", func(t *testing.T) {
		parsed, err := parseSubscriptionExpiresAt("2026-04-30T00:00:00Z")
		if err != nil {
			t.Fatalf("parseSubscriptionExpiresAt returned error: %v", err)
		}

		want := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
		if !parsed.Equal(want) {
			t.Fatalf("parsed=%s want=%s", parsed.Format(time.RFC3339), want.Format(time.RFC3339))
		}
	})

	t.Run("supports datetime-local", func(t *testing.T) {
		parsed, err := parseSubscriptionExpiresAt("2026-04-30T00:00")
		if err != nil {
			t.Fatalf("parseSubscriptionExpiresAt returned error: %v", err)
		}

		want, err := time.ParseInLocation("2006-01-02T15:04", "2026-04-30T00:00", time.Local)
		if err != nil {
			t.Fatalf("time.ParseInLocation returned error: %v", err)
		}

		if !parsed.Equal(want.UTC()) {
			t.Fatalf("parsed=%s want=%s", parsed.Format(time.RFC3339), want.UTC().Format(time.RFC3339))
		}
	})

	t.Run("rejects invalid format", func(t *testing.T) {
		if _, err := parseSubscriptionExpiresAt("2026/04/30 00:00"); err == nil {
			t.Fatal("expected parseSubscriptionExpiresAt to reject invalid input")
		}
	})
}
