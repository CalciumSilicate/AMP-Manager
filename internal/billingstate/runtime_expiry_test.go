package billingstate

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestReleaseExpiredReservationsBatchLimitsPageSize(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	rt := &Runtime{
		client: client,
		cfg: Config{
			Prefix:          "amp",
			StreamBatchSize: 2,
		},
	}

	seedExpiredReservation(t, mr, rt, "user-1", "req-1", 1, 7)
	seedExpiredReservation(t, mr, rt, "user-1", "req-2", 2, 11)
	seedExpiredReservation(t, mr, rt, "user-1", "req-3", 3, 13)

	count, err := rt.releaseExpiredReservationsBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("releaseExpiredReservationsBatch returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("processed count = %d, want 2", count)
	}

	if got := mr.HGet(rt.reservationKey("req-1"), "status"); got != "expired" {
		t.Fatalf("req-1 status = %q, want expired", got)
	}
	if got := mr.HGet(rt.reservationKey("req-2"), "status"); got != "expired" {
		t.Fatalf("req-2 status = %q, want expired", got)
	}
	if got := mr.HGet(rt.reservationKey("req-3"), "status"); got != "reserved" {
		t.Fatalf("req-3 status = %q, want reserved", got)
	}
	if _, err := mr.ZScore(rt.reservationExpiryKey(), "req-3"); err != nil {
		t.Fatalf("req-3 should remain in expiry zset after first batch")
	}
	if got := mr.HGet(rt.accountKey("user-1"), "balance_micros"); got != "18" {
		t.Fatalf("balance_micros = %q, want 18", got)
	}

	stream, err := mr.Stream(rt.streamKey())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	if len(stream) != 2 {
		t.Fatalf("stream length = %d, want 2", len(stream))
	}
}

func TestReleaseExpiredReservationsBatchCanContinueAcrossCalls(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	rt := &Runtime{
		client: client,
		cfg: Config{
			Prefix:          "amp",
			StreamBatchSize: 2,
		},
	}

	seedExpiredReservation(t, mr, rt, "user-1", "req-1", 1, 5)
	seedExpiredReservation(t, mr, rt, "user-1", "req-2", 2, 7)
	seedExpiredReservation(t, mr, rt, "user-1", "req-3", 3, 9)

	if _, err := rt.releaseExpiredReservationsBatch(context.Background(), 10); err != nil {
		t.Fatalf("first releaseExpiredReservationsBatch returned error: %v", err)
	}
	count, err := rt.releaseExpiredReservationsBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("second releaseExpiredReservationsBatch returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("second processed count = %d, want 1", count)
	}
	if got := mr.HGet(rt.reservationKey("req-3"), "status"); got != "expired" {
		t.Fatalf("req-3 status = %q, want expired", got)
	}
	if got := mr.HGet(rt.accountKey("user-1"), "balance_micros"); got != "21" {
		t.Fatalf("balance_micros = %q, want 21", got)
	}
	size, err := client.ZCard(context.Background(), rt.reservationExpiryKey()).Result()
	if err != nil {
		t.Fatalf("ZCard returned error: %v", err)
	}
	if size != 0 {
		t.Fatalf("expiry zset cardinality = %d, want 0", size)
	}
}

func seedExpiredReservation(t *testing.T, mr *miniredis.Miniredis, rt *Runtime, userID, requestID string, score int64, reservedBalance int64) {
	t.Helper()

	accountKey := rt.accountKey(userID)
	if !mr.Exists(accountKey) {
		mr.HSet(accountKey, "balance_micros", "0")
	}
	mr.HSet(rt.reservationKey(requestID),
		"status", "reserved",
		"request_id", requestID,
		"user_id", userID,
		"user_subscription_id", "",
		"reserved_subscription_micros", "0",
		"reserved_balance_micros", fmt.Sprintf("%d", reservedBalance),
		"window_key_list", "",
		"window_refs_json", "[]",
	)
	mr.ZAdd(rt.reservationExpiryKey(), float64(score), requestID)
}
