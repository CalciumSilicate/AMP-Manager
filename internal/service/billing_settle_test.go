package service

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"ampmanager/internal/database"
)

func TestSettleRequestCostLegacyMarksOveruseAfterBestEffortCharge(t *testing.T) {
	db := setupBillingServiceTestDB(t)
	now := time.Now().UTC()

	mustExecBillingService(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 10)`, "user-1")
	mustExecBillingService(t, db, `INSERT INTO subscription_plans (id, name) VALUES (?, 'starter')`, "plan-1")
	mustExecBillingService(t, db, `INSERT INTO subscription_plan_limits (id, plan_id, limit_type, window_mode, limit_micros) VALUES (?, ?, 'monthly', 'fixed', 100)`, "limit-1", "plan-1")
	mustExecBillingService(t, db, `INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, status) VALUES (?, ?, ?, ?, 'active')`, "sub-1", "user-1", "plan-1", now)
	mustExecBillingService(t, db, `INSERT INTO user_billing_settings (user_id, primary_source, secondary_source) VALUES (?, 'subscription', 'balance')`, "user-1")
	mustExecBillingService(t, db, `INSERT INTO request_logs (id, created_at, user_id, api_key_id, method, path, status_code, latency_ms, cost_micros, cost_usd, billing_status) VALUES (?, ?, ?, 'key-1', 'POST', '/v1/responses', 200, 123, 130, '0.000130', 'none')`, "req-1", now, "user-1")

	svc := NewBillingService()
	if err := svc.settleRequestCostLegacy("req-1", "user-1", 130); err != nil {
		t.Fatalf("settleRequestCostLegacy returned error: %v", err)
	}

	var chargedSub, chargedBal int64
	var status string
	if err := db.QueryRow(`SELECT charged_subscription_micros, charged_balance_micros, billing_status FROM request_logs WHERE id = ?`, "req-1").Scan(&chargedSub, &chargedBal, &status); err != nil {
		t.Fatalf("query request_logs returned error: %v", err)
	}
	if chargedSub != 100 || chargedBal != 10 || status != "overuse" {
		t.Fatalf("request log billing = sub:%d bal:%d status:%s", chargedSub, chargedBal, status)
	}

	var balance int64
	if err := db.QueryRow(`SELECT balance_micros FROM users WHERE id = ?`, "user-1").Scan(&balance); err != nil {
		t.Fatalf("query users returned error: %v", err)
	}
	if balance != 0 {
		t.Fatalf("balance after settle = %d, want 0", balance)
	}
}

func setupBillingServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "billing-service-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})
	return database.GetDB()
}

func mustExecBillingService(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("db.Exec failed: %v\nquery=%s", err, query)
	}
}
