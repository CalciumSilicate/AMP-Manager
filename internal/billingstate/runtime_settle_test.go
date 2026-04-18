package billingstate

import (
	"context"
	"errors"
	"testing"
	"time"

	"ampmanager/internal/database"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestSettleRequestRefillsHotReservationAfterNotFound(t *testing.T) {
	setupBillingStateTestDB(t)

	now := time.Now().UTC()
	db := database.GetDB()
	mustExecBillingState(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 10)`, "user-1")
	mustExecBillingState(t, db, `INSERT INTO billing_account_state (user_id, primary_source, secondary_source, balance_micros, revision, created_at, updated_at) VALUES (?, 'subscription', 'balance', 3, 0, ?, ?)`, "user-1", now, now)
	mustExecBillingState(t, db, `INSERT INTO billing_reservations (request_id, user_id, estimated_cost_micros, reserved_subscription_micros, reserved_balance_micros, charged_subscription_micros, charged_balance_micros, status, expires_at, created_at, updated_at) VALUES (?, ?, 7, 0, 7, 0, 0, 'reserved', ?, ?, ?)`, "req-1", "user-1", now.Add(10*time.Minute), now, now)

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	rt := &Runtime{
		client: client,
		cfg: Config{
			Prefix: "amp",
		},
	}

	result, err := rt.SettleRequest(context.Background(), "req-1", "user-1", 7, nil)
	if err != nil {
		t.Fatalf("SettleRequest returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("SettleRequest returned nil result")
	}
	if result.Status != "settled" {
		t.Fatalf("status = %q, want settled", result.Status)
	}
	if result.ChargedSubscriptionMicros != 0 || result.ChargedBalanceMicros != 7 {
		t.Fatalf("charges = sub:%d bal:%d, want 0/7", result.ChargedSubscriptionMicros, result.ChargedBalanceMicros)
	}

	if got := mr.HGet(rt.reservationKey("req-1"), "status"); got != "settled" {
		t.Fatalf("reservation status = %q, want settled", got)
	}
	if got := mr.HGet(rt.accountKey("user-1"), "balance_micros"); got != "3" {
		t.Fatalf("balance_micros = %q, want 3", got)
	}
	stream, err := mr.Stream(rt.streamKey())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	if len(stream) != 1 {
		t.Fatalf("stream length = %d, want 1", len(stream))
	}
	if got := streamValue(stream[0].Values, "event_type"); got != "settle" {
		t.Fatalf("event type = %q, want settle", got)
	}
}

func TestSettleRequestReturnsNotFoundWhenReservationCannotBeRefilled(t *testing.T) {
	setupBillingStateTestDB(t)

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	rt := &Runtime{
		client: client,
		cfg: Config{
			Prefix: "amp",
		},
	}

	_, err := rt.SettleRequest(context.Background(), "missing", "user-1", 7, nil)
	if !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("SettleRequest error = %v, want %v", err, ErrReservationNotFound)
	}
}

func streamValue(values []string, key string) string {
	for i := 0; i+1 < len(values); i += 2 {
		if values[i] == key {
			return values[i+1]
		}
	}
	return ""
}
