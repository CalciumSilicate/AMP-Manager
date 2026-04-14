package billingstate

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"ampmanager/internal/database"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestSettleScriptUsesCurrentBillingOrderForExtraCharge(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	accountKey := "amp:billing:account:user-1"
	reservationKey := "amp:billing:reservation:req-1"
	expiryKey := "amp:billing:expiry"
	streamKey := "amp:billing:events"
	windowKey := "amp:billing:quota:sub-1:monthly:1"

	mr.HSet(accountKey, "primary_source", "balance", "secondary_source", "subscription", "balance_micros", "10")
	mr.HSet(windowKey, "remaining_micros", "5", "reserved_micros", "100", "used_micros", "0")
	mr.HSet(reservationKey,
		"status", "reserved",
		"request_id", "req-1",
		"user_id", "user-1",
		"user_subscription_id", "sub-1",
		"reserved_subscription_micros", "100",
		"reserved_balance_micros", "0",
		"window_key_list", windowKey,
		"window_refs_json", `[{"stateId":"sub-1:monthly:1"}]`,
	)
	mr.ZAdd(expiryKey, 123, "req-1")

	raw, err := settleScript.Run(context.Background(), client, []string{accountKey, reservationKey, expiryKey, streamKey}, int64(120), time.Now().Unix()).Result()
	if err != nil {
		t.Fatalf("settleScript.Run returned error: %v", err)
	}
	values := stringifyRedisResult(raw)
	if got, want := values[1], "overuse"; got != want {
		t.Fatalf("status = %q, want %q (values=%v)", got, want, values)
	}
	if got, want := values[2], "105"; got != want {
		t.Fatalf("charged sub = %q, want %q", got, want)
	}
	if got, want := values[3], "10"; got != want {
		t.Fatalf("charged balance = %q, want %q", got, want)
	}

	if got := mr.HGet(accountKey, "balance_micros"); got != "0" {
		t.Fatalf("balance after settle = %q, want 0", got)
	}
	if got := mr.HGet(windowKey, "used_micros"); got != "105" {
		t.Fatalf("used_micros after settle = %q, want 105", got)
	}
	if got := mr.HGet(windowKey, "remaining_micros"); got != "0" {
		t.Fatalf("remaining_micros after settle = %q, want 0", got)
	}
	if got := mr.HGet(windowKey, "reserved_micros"); got != "0" {
		t.Fatalf("reserved_micros after settle = %q, want 0", got)
	}
	if got := mr.HGet(reservationKey, "status"); got != "overuse" {
		t.Fatalf("reservation status = %q, want overuse", got)
	}
}

func TestApplySettleEventPersistsChargesBeyondReservedAmounts(t *testing.T) {
	setupBillingStateTestDB(t)

	now := time.Now().UTC()
	db := database.GetDB()
	mustExecBillingState(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 25)`, "user-1")
	mustExecBillingState(t, db, `INSERT INTO subscription_plans (id, name) VALUES (?, 'starter')`, "plan-1")
	mustExecBillingState(t, db, `INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, status) VALUES (?, ?, ?, ?, 'active')`, "sub-1", "user-1", "plan-1", now)
	mustExecBillingState(t, db, `INSERT INTO billing_account_state (user_id, primary_source, secondary_source, balance_micros, active_subscription_id, active_plan_id, subscription_starts_at, revision) VALUES (?, 'subscription', 'balance', 15, ?, 'plan-1', ?, 0)`, "user-1", "sub-1", now)
	mustExecBillingState(t, db, `INSERT INTO subscription_window_state (id, user_subscription_id, plan_id, limit_type, window_mode, window_start, window_end, limit_micros, used_micros, reserved_micros, remaining_micros, revision) VALUES (?, ?, 'plan-1', 'monthly', 'fixed', ?, ?, 200, 0, 100, 20, 0)`, "sub-1:monthly:1", "sub-1", now, now.Add(24*time.Hour))
	mustExecBillingState(t, db, `INSERT INTO billing_reservations (request_id, user_id, user_subscription_id, estimated_cost_micros, reserved_subscription_micros, reserved_balance_micros, status, window_refs_json, expires_at) VALUES (?, ?, ?, 110, 100, 10, 'reserved', ?, ?)`, "req-1", "user-1", "sub-1", `[{"stateId":"sub-1:monthly:1"}]`, now.Add(10*time.Minute))
	mustExecBillingState(t, db, `INSERT INTO request_logs (id, created_at, user_id, api_key_id, method, path, status_code, latency_ms, cost_micros, cost_usd, charged_subscription_micros, charged_balance_micros, billing_status) VALUES (?, ?, ?, 'key-1', 'POST', '/v1/responses', 200, 123, 125, '0.000125', 0, 0, 'none')`, "req-1", now, "user-1")

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("db.Begin returned error: %v", err)
	}
	defer tx.Rollback()

	err = applySettleEvent(tx, map[string]any{
		"request_id":                   "req-1",
		"user_id":                      "user-1",
		"status":                       "settled",
		"actual_cost_micros":           "125",
		"reserved_subscription_micros": "100",
		"reserved_balance_micros":      "10",
		"charged_subscription_micros":  "105",
		"charged_balance_micros":       "20",
		"released_subscription_micros": "0",
		"released_balance_micros":      "0",
		"window_refs_json":             `[{"stateId":"sub-1:monthly:1"}]`,
	})
	if err != nil {
		t.Fatalf("applySettleEvent returned error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("tx.Commit returned error: %v", err)
	}

	var balance int64
	if err := db.QueryRow(`SELECT balance_micros FROM billing_account_state WHERE user_id = ?`, "user-1").Scan(&balance); err != nil {
		t.Fatalf("query billing_account_state returned error: %v", err)
	}
	if balance != 5 {
		t.Fatalf("billing_account_state balance = %d, want 5", balance)
	}

	var used, reserved, remaining int64
	if err := db.QueryRow(`SELECT used_micros, reserved_micros, remaining_micros FROM subscription_window_state WHERE id = ?`, "sub-1:monthly:1").Scan(&used, &reserved, &remaining); err != nil {
		t.Fatalf("query subscription_window_state returned error: %v", err)
	}
	if used != 105 || reserved != 0 || remaining != 15 {
		t.Fatalf("window state = used:%d reserved:%d remaining:%d, want 105/0/15", used, reserved, remaining)
	}

	var chargedSub, chargedBal int64
	var status string
	if err := db.QueryRow(`SELECT charged_subscription_micros, charged_balance_micros, billing_status FROM request_logs WHERE id = ?`, "req-1").Scan(&chargedSub, &chargedBal, &status); err != nil {
		t.Fatalf("query request_logs returned error: %v", err)
	}
	if chargedSub != 105 || chargedBal != 20 || status != "settled" {
		t.Fatalf("request log billing = sub:%d bal:%d status:%s", chargedSub, chargedBal, status)
	}
}

func setupBillingStateTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "billing-state-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})
}

func mustExecBillingState(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("db.Exec failed: %v\nquery=%s", err, query)
	}
}
