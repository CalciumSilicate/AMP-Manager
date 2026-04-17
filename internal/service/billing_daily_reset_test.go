package service

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

func TestResetDailyBillingResetsDailyWindowAndShortensExpiry(t *testing.T) {
	db := setupBillingDailyResetTestDB(t)
	svc := NewBillingService()

	location, err := time.LoadLocation(defaultSiteTimeZone)
	if err != nil {
		t.Fatalf("time.LoadLocation returned error: %v", err)
	}

	now := time.Now().UTC()
	localNow := now.In(location)
	dayStartLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	dayStart := dayStartLocal.UTC()
	expiresAt := now.Add(5 * 24 * time.Hour).Truncate(time.Second)
	startsAt := now.Add(-7 * 24 * time.Hour).Truncate(time.Second)

	mustExecBillingService(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 0)`, "user-1")
	mustExecBillingService(t, db, `INSERT INTO subscription_plans (id, name) VALUES (?, 'starter')`, "plan-1")
	mustExecBillingService(t, db, `INSERT INTO subscription_plan_limits (id, plan_id, limit_type, window_mode, limit_micros, fixed_reset_minute, created_at, updated_at) VALUES (?, ?, 'daily', 'fixed', 100000000, 0, ?, ?)`, "limit-daily", "plan-1", now, now)
	mustExecBillingService(t, db, `INSERT INTO subscription_plan_limits (id, plan_id, limit_type, window_mode, limit_micros, created_at, updated_at) VALUES (?, ?, 'total', 'fixed', 200000000, ?, ?)`, "limit-total", "plan-1", now, now)
	mustExecBillingService(t, db, `INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'active', ?, ?)`, "sub-1", "user-1", "plan-1", startsAt, expiresAt, now, now)
	mustExecBillingService(t, db, `INSERT INTO billing_events (id, user_id, user_subscription_id, source, event_type, amount_micros, created_at) VALUES (?, ?, ?, 'subscription', 'charge', 95000000, ?)`, "event-daily", "user-1", "sub-1", dayStart.Add(2*time.Hour))
	mustExecBillingService(t, db, `INSERT INTO billing_events (id, user_id, user_subscription_id, source, event_type, amount_micros, created_at) VALUES (?, ?, ?, 'subscription', 'charge', 50000000, ?)`, "event-total", "user-1", "sub-1", dayStart.Add(-24*time.Hour))

	stateBefore, err := svc.GetBillingState("user-1")
	if err != nil {
		t.Fatalf("GetBillingState before reset returned error: %v", err)
	}

	dailyBefore := findWindowByType(t, stateBefore.Windows, model.LimitTypeDaily)
	totalBefore := findWindowByType(t, stateBefore.Windows, model.LimitTypeTotal)
	if dailyBefore.UsedMicros != 95000000 {
		t.Fatalf("daily used before reset = %d, want 95000000", dailyBefore.UsedMicros)
	}
	if totalBefore.UsedMicros != 145000000 {
		t.Fatalf("total used before reset = %d, want 145000000", totalBefore.UsedMicros)
	}
	if !stateBefore.DailyReset.Supported || !stateBefore.DailyReset.Allowed {
		t.Fatalf("daily reset should be allowed before reset, got %+v", stateBefore.DailyReset)
	}

	stateAfter, err := svc.ResetDailyBilling("user-1")
	if err != nil {
		t.Fatalf("ResetDailyBilling returned error: %v", err)
	}

	dailyAfter := findWindowByType(t, stateAfter.Windows, model.LimitTypeDaily)
	totalAfter := findWindowByType(t, stateAfter.Windows, model.LimitTypeTotal)
	if dailyAfter.UsedMicros != 0 {
		t.Fatalf("daily used after reset = %d, want 0", dailyAfter.UsedMicros)
	}
	if totalAfter.UsedMicros != 145000000 {
		t.Fatalf("total used after reset = %d, want 145000000", totalAfter.UsedMicros)
	}
	if stateAfter.Subscription == nil || stateAfter.Subscription.ExpiresAt == nil {
		t.Fatal("expected active subscription with expiry after reset")
	}
	if !stateAfter.Subscription.ExpiresAt.UTC().Equal(expiresAt.AddDate(0, 0, -1)) {
		t.Fatalf("expiresAt after reset = %v, want %v", stateAfter.Subscription.ExpiresAt.UTC(), expiresAt.AddDate(0, 0, -1))
	}
	if stateAfter.DailyReset.UsedToday != 1 {
		t.Fatalf("daily reset count after reset = %d, want 1", stateAfter.DailyReset.UsedToday)
	}
	if stateAfter.DailyReset.Allowed {
		t.Fatalf("daily reset should not remain allowed immediately after reset, got %+v", stateAfter.DailyReset)
	}

	var recordCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_daily_reset_records WHERE user_id = ?`, "user-1").Scan(&recordCount); err != nil {
		t.Fatalf("query billing_daily_reset_records count returned error: %v", err)
	}
	if recordCount != 1 {
		t.Fatalf("billing daily reset record count = %d, want 1", recordCount)
	}

	var usedBeforeReset int64
	if err := db.QueryRow(`SELECT used_micros_before_reset FROM billing_daily_reset_records WHERE user_id = ?`, "user-1").Scan(&usedBeforeReset); err != nil {
		t.Fatalf("query billing_daily_reset_records returned error: %v", err)
	}
	if usedBeforeReset != 95000000 {
		t.Fatalf("used_micros_before_reset = %d, want 95000000", usedBeforeReset)
	}
}

func TestGetBillingStateMarksSlidingDailyResetUnsupported(t *testing.T) {
	db := setupBillingDailyResetTestDB(t)
	svc := NewBillingService()
	now := time.Now().UTC()
	expiresAt := now.Add(5 * 24 * time.Hour).Truncate(time.Second)
	startsAt := now.Add(-48 * time.Hour).Truncate(time.Second)

	mustExecBillingService(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'bob', 'x', 0, 0)`, "user-2")
	mustExecBillingService(t, db, `INSERT INTO subscription_plans (id, name) VALUES (?, 'sliding')`, "plan-2")
	mustExecBillingService(t, db, `INSERT INTO subscription_plan_limits (id, plan_id, limit_type, window_mode, limit_micros, created_at, updated_at) VALUES (?, ?, 'daily', 'sliding', 100000000, ?, ?)`, "limit-daily-sliding", "plan-2", now, now)
	mustExecBillingService(t, db, `INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'active', ?, ?)`, "sub-2", "user-2", "plan-2", startsAt, expiresAt, now, now)
	mustExecBillingService(t, db, `INSERT INTO billing_events (id, user_id, user_subscription_id, source, event_type, amount_micros, created_at) VALUES (?, ?, ?, 'subscription', 'charge', 95000000, ?)`, "event-daily-sliding", "user-2", "sub-2", now.Add(-time.Hour))

	state, err := svc.GetBillingState("user-2")
	if err != nil {
		t.Fatalf("GetBillingState returned error: %v", err)
	}
	if state.DailyReset.Supported {
		t.Fatalf("daily reset should be unsupported for sliding daily window, got %+v", state.DailyReset)
	}
	if state.DailyReset.Allowed {
		t.Fatalf("daily reset should not be allowed for sliding daily window, got %+v", state.DailyReset)
	}
	if state.DailyReset.Message != "当前订阅没有固定日额度" {
		t.Fatalf("unexpected daily reset message: %q", state.DailyReset.Message)
	}
}

func setupBillingDailyResetTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "billing-daily-reset-test.sqlite")
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

func findWindowByType(t *testing.T, windows []model.WindowRemaining, limitType model.LimitType) model.WindowRemaining {
	t.Helper()

	for _, window := range windows {
		if window.LimitType == limitType {
			return window
		}
	}
	t.Fatalf("window %s not found", limitType)
	return model.WindowRemaining{}
}
