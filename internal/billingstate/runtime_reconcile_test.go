package billingstate

import (
	"context"
	"testing"

	"ampmanager/internal/database"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestReconcileNextPageAdvancesByCursor(t *testing.T) {
	setupBillingStateTestDB(t)

	db := database.GetDB()
	mustExecBillingState(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'u1', 'x', 0, 10)`, "user-1")
	mustExecBillingState(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'u2', 'x', 0, 20)`, "user-2")
	mustExecBillingState(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'u3', 'x', 0, 30)`, "user-3")
	mustExecBillingState(t, db, `INSERT INTO billing_account_state (user_id, primary_source, secondary_source, balance_micros, revision) VALUES (?, 'subscription', 'balance', 10, 0)`, "user-1")
	mustExecBillingState(t, db, `INSERT INTO billing_account_state (user_id, primary_source, secondary_source, balance_micros, revision) VALUES (?, 'subscription', 'balance', 20, 0)`, "user-2")
	mustExecBillingState(t, db, `INSERT INTO billing_account_state (user_id, primary_source, secondary_source, balance_micros, revision) VALUES (?, 'subscription', 'balance', 30, 0)`, "user-3")

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

	repairs, err := rt.reconcileNextPage(context.Background())
	if err != nil {
		t.Fatalf("reconcileNextPage first pass returned error: %v", err)
	}
	if repairs != 2 {
		t.Fatalf("first repairs = %d, want 2", repairs)
	}
	if rt.reconcileCursor != "user-2" {
		t.Fatalf("cursor after first page = %q, want user-2", rt.reconcileCursor)
	}
	if !mr.Exists(rt.accountKey("user-1")) || !mr.Exists(rt.accountKey("user-2")) {
		t.Fatalf("expected first page users to be reconciled into redis")
	}
	if mr.Exists(rt.accountKey("user-3")) {
		t.Fatalf("did not expect third user to be reconciled on first page")
	}

	repairs, err = rt.reconcileNextPage(context.Background())
	if err != nil {
		t.Fatalf("reconcileNextPage second pass returned error: %v", err)
	}
	if repairs != 1 {
		t.Fatalf("second repairs = %d, want 1", repairs)
	}
	if rt.reconcileCursor != "" {
		t.Fatalf("cursor after second page = %q, want empty", rt.reconcileCursor)
	}
	if !mr.Exists(rt.accountKey("user-3")) {
		t.Fatalf("expected second page user to be reconciled into redis")
	}
}
