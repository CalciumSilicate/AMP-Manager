package service

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"ampmanager/internal/database"
)

func TestSettleRequestCostResultLegacyReturnsOveruseAfterBestEffortCharge(t *testing.T) {
	db := setupBillingServiceTestDB(t)
	now := time.Now().UTC()

	mustExecBillingService(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 10)`, "user-1")
	mustExecBillingService(t, db, `INSERT INTO subscription_plans (id, name) VALUES (?, 'starter')`, "plan-1")
	mustExecBillingService(t, db, `INSERT INTO subscription_plan_limits (id, plan_id, limit_type, window_mode, limit_micros) VALUES (?, ?, 'monthly', 'fixed', 100)`, "limit-1", "plan-1")
	mustExecBillingService(t, db, `INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, status) VALUES (?, ?, ?, ?, 'active')`, "sub-1", "user-1", "plan-1", now)
	mustExecBillingService(t, db, `INSERT INTO user_billing_settings (user_id, primary_source, secondary_source) VALUES (?, 'subscription', 'balance')`, "user-1")
	mustExecBillingService(t, db, `INSERT INTO request_logs (id, created_at, user_id, api_key_id, method, path, status_code, latency_ms, cost_micros, cost_usd, billing_status) VALUES (?, ?, ?, 'key-1', 'POST', '/v1/responses', 200, 123, 130, '0.000130', 'none')`, "req-1", now, "user-1")

	svc := NewBillingService()
	result, err := svc.settleRequestCostLegacy("req-1", "user-1", 130, nil)
	if err != nil {
		t.Fatalf("settleRequestCostLegacy returned error: %v", err)
	}
	if result == nil {
		t.Fatal("settleRequestCostLegacy returned nil result")
	}
	if result.ChargedSubscriptionMicros != 100 || result.ChargedBalanceMicros != 10 || result.Status != "overuse" {
		t.Fatalf("settle result = %+v", result)
	}

	var chargedSub, chargedBal int64
	var status string
	if err := db.QueryRow(`SELECT charged_subscription_micros, charged_balance_micros, billing_status FROM request_logs WHERE id = ?`, "req-1").Scan(&chargedSub, &chargedBal, &status); err != nil {
		t.Fatalf("query request_logs returned error: %v", err)
	}
	if chargedSub != 0 || chargedBal != 0 || status != "none" {
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

func TestSettleRequestCostFallbackStillMarksRequestLog(t *testing.T) {
	db := setupBillingServiceTestDB(t)
	now := time.Now().UTC()

	mustExecBillingService(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 5)`, "user-1")
	mustExecBillingService(t, db, `INSERT INTO user_billing_settings (user_id, primary_source, secondary_source) VALUES (?, 'balance', 'subscription')`, "user-1")
	mustExecBillingService(t, db, `INSERT INTO request_logs (id, created_at, user_id, api_key_id, method, path, status_code, latency_ms, cost_micros, cost_usd, billing_status) VALUES (?, ?, ?, 'key-1', 'POST', '/v1/responses', 200, 123, 5, '0.000005', 'none')`, "req-1", now, "user-1")

	svc := NewBillingService()
	if err := svc.SettleRequestCost("req-1", "user-1", 5); err != nil {
		t.Fatalf("SettleRequestCost returned error: %v", err)
	}

	var chargedSub, chargedBal int64
	var status string
	if err := db.QueryRow(`SELECT charged_subscription_micros, charged_balance_micros, billing_status FROM request_logs WHERE id = ?`, "req-1").Scan(&chargedSub, &chargedBal, &status); err != nil {
		t.Fatalf("query request_logs returned error: %v", err)
	}
	if chargedSub != 0 || chargedBal != 5 || status != "settled" {
		t.Fatalf("request log billing = sub:%d bal:%d status:%s", chargedSub, chargedBal, status)
	}
}

func TestSettleRequestCostLegacyFallsBackAcrossMultipleSubscriptions(t *testing.T) {
	db := setupBillingServiceTestDB(t)
	now := time.Now().UTC()

	mustExecBillingService(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 0)`, "user-1")
	mustExecBillingService(t, db, `INSERT INTO subscription_plans (id, name) VALUES (?, 'starter')`, "plan-a")
	mustExecBillingService(t, db, `INSERT INTO subscription_plans (id, name) VALUES (?, 'pro')`, "plan-b")
	mustExecBillingService(t, db, `INSERT INTO subscription_plan_limits (id, plan_id, limit_type, window_mode, limit_micros) VALUES (?, ?, 'monthly', 'fixed', 40)`, "limit-a", "plan-a")
	mustExecBillingService(t, db, `INSERT INTO subscription_plan_limits (id, plan_id, limit_type, window_mode, limit_micros) VALUES (?, ?, 'monthly', 'fixed', 50)`, "limit-b", "plan-b")
	mustExecBillingService(t, db, `INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, expires_at, status) VALUES (?, ?, ?, ?, ?, 'active')`, "sub-a", "user-1", "plan-a", now.AddDate(0, 0, -1), now.Add(24*time.Hour))
	mustExecBillingService(t, db, `INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, expires_at, status) VALUES (?, ?, ?, ?, ?, 'active')`, "sub-b", "user-1", "plan-b", now.AddDate(0, 0, -1), now.Add(7*24*time.Hour))
	mustExecBillingService(t, db, `INSERT INTO user_billing_settings (user_id, primary_source, secondary_source) VALUES (?, 'subscription', 'balance')`, "user-1")
	mustExecBillingService(t, db, `INSERT INTO request_logs (id, created_at, user_id, api_key_id, method, path, status_code, latency_ms, cost_micros, cost_usd, billing_status) VALUES (?, ?, ?, 'key-1', 'POST', '/v1/responses', 200, 123, 70, '0.000070', 'none')`, "req-2", now, "user-1")

	svc := NewBillingService()
	result, err := svc.settleRequestCostLegacy("req-2", "user-1", 70, nil)
	if err != nil {
		t.Fatalf("settleRequestCostLegacy returned error: %v", err)
	}
	if result == nil || result.ChargedSubscriptionMicros != 70 || result.ChargedBalanceMicros != 0 || result.Status != "settled" {
		t.Fatalf("unexpected settle result: %+v", result)
	}
}

func TestApplyBillingResultMarksRequestLog(t *testing.T) {
	db := setupBillingServiceTestDB(t)
	now := time.Now().UTC()

	mustExecBillingService(t, db, `INSERT INTO request_logs (id, created_at, user_id, api_key_id, method, path, status_code, latency_ms, cost_micros, cost_usd, billing_status) VALUES (?, ?, ?, 'key-1', 'POST', '/v1/responses', 200, 123, 5, '0.000005', 'none')`, "req-1", now, "user-1")

	svc := NewBillingService()
	if err := svc.ApplyBillingResult("req-1", &RequestBillingResult{
		Status:                    "settled",
		ChargedSubscriptionMicros: 0,
		ChargedBalanceMicros:      5,
	}); err != nil {
		t.Fatalf("ApplyBillingResult returned error: %v", err)
	}

	var chargedSub, chargedBal int64
	var status string
	if err := db.QueryRow(`SELECT charged_subscription_micros, charged_balance_micros, billing_status FROM request_logs WHERE id = ?`, "req-1").Scan(&chargedSub, &chargedBal, &status); err != nil {
		t.Fatalf("query request_logs returned error: %v", err)
	}
	if chargedSub != 0 || chargedBal != 5 || status != "settled" {
		t.Fatalf("request log billing = sub:%d bal:%d status:%s", chargedSub, chargedBal, status)
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
