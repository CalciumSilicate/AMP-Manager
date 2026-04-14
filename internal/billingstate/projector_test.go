package billingstate

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ampmanager/internal/database"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestBuildProjectorConsumerNamesReturnsUniqueStableNames(t *testing.T) {
	names := buildProjectorConsumerNames("host-a", 1234, "instance-a", 3)
	want := []string{
		"host-a-1234-instance-a-projector-1",
		"host-a-1234-instance-a-projector-2",
		"host-a-1234-instance-a-projector-3",
	}

	if len(names) != len(want) {
		t.Fatalf("consumer names length = %d, want %d", len(names), len(want))
	}
	for idx := range want {
		if names[idx] != want[idx] {
			t.Fatalf("consumer name[%d] = %q, want %q", idx, names[idx], want[idx])
		}
	}
}

func TestBuildStartsConfiguredProjectorWorkers(t *testing.T) {
	mr := miniredis.RunT(t)
	rt, err := Build(Config{
		RedisURL:         fmt.Sprintf("redis://%s/0", mr.Addr()),
		Prefix:           "amp",
		StreamBatchSize:  2,
		ProjectorWorkers: 3,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if rt == nil {
		t.Fatal("Build returned nil runtime")
	}
	defer rt.Close()

	if got := len(rt.projectorConsumers); got != 3 {
		t.Fatalf("projector consumer count = %d, want 3", got)
	}
	seen := make(map[string]struct{}, len(rt.projectorConsumers))
	for _, name := range rt.projectorConsumers {
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate projector consumer name %q", name)
		}
		seen[name] = struct{}{}
	}
}

func TestProjectMessagesFallsBackToPerMessageAck(t *testing.T) {
	setupBillingStateTestDB(t)

	now := time.Now().UTC()
	db := database.GetDB()
	mustExecBillingState(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 20)`, "user-1")
	mustExecBillingState(t, db, `INSERT INTO billing_account_state (user_id, primary_source, secondary_source, balance_micros, revision, created_at, updated_at) VALUES (?, 'subscription', 'balance', 20, 0, ?, ?)`, "user-1", now, now)

	rt := &Runtime{}
	messages := []redis.XMessage{
		{
			ID: "1-0",
			Values: map[string]any{
				"event_type":                   "reserve",
				"request_id":                   "req-good",
				"user_id":                      "user-1",
				"user_subscription_id":         "",
				"status":                       "reserved",
				"estimated_cost_micros":        "5",
				"reserved_subscription_micros": "0",
				"reserved_balance_micros":      "5",
				"window_refs_json":             "[]",
				"expires_at_unix":              now.Add(5 * time.Minute).Unix(),
			},
		},
		{
			ID: "2-0",
			Values: map[string]any{
				"event_type":                   "reserve",
				"request_id":                   "req-bad",
				"user_id":                      "user-1",
				"user_subscription_id":         "",
				"status":                       "reserved",
				"estimated_cost_micros":        "3",
				"reserved_subscription_micros": "0",
				"reserved_balance_micros":      "3",
				"window_refs_json":             "{bad-json",
				"expires_at_unix":              now.Add(5 * time.Minute).Unix(),
			},
		},
	}

	acked := rt.projectMessages(context.Background(), messages)
	if len(acked) != 1 || acked[0] != "1-0" {
		t.Fatalf("acked = %v, want [1-0]", acked)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_projection_events`).Scan(&count); err != nil {
		t.Fatalf("query billing_projection_events returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("projection event count = %d, want 1", count)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_reservations WHERE request_id = ?`, "req-good").Scan(&count); err != nil {
		t.Fatalf("query good reservation returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("good reservation count = %d, want 1", count)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_reservations WHERE request_id = ?`, "req-bad").Scan(&count); err != nil {
		t.Fatalf("query bad reservation returned error: %v", err)
	}
	if count != 0 {
		t.Fatalf("bad reservation count = %d, want 0", count)
	}

	var balance int64
	if err := db.QueryRow(`SELECT balance_micros FROM billing_account_state WHERE user_id = ?`, "user-1").Scan(&balance); err != nil {
		t.Fatalf("query billing_account_state returned error: %v", err)
	}
	if balance != 15 {
		t.Fatalf("balance_micros = %d, want 15", balance)
	}
}
