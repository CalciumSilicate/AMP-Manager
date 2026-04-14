package repository

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"ampmanager/internal/database"
)

func TestGetCacheHitRateByProviderUsesTotalInputAsDenominator(t *testing.T) {
	setupRequestLogTestDB(t)

	now := time.Now().UTC()
	insertRequestLogForCacheTest(t, "log-openai-1", "user-1", "gpt-5.4", 100, 80, 10, now)
	insertRequestLogForCacheTest(t, "log-claude-1", "user-1", "claude-3-5-sonnet", 50, 20, 5, now)
	insertRequestLogForCacheTest(t, "log-openai-2", "user-2", "gpt-5.4", 200, 50, 0, now)

	repo := NewRequestLogRepository()
	rates, err := repo.GetCacheHitRateByProvider("user-1")
	if err != nil {
		t.Fatalf("GetCacheHitRateByProvider returned error: %v", err)
	}

	byProvider := mapRatesByProvider(rates)

	openAI, ok := byProvider["OpenAI"]
	if !ok {
		t.Fatalf("expected OpenAI provider result")
	}
	if openAI.TotalInputTokens != 100 || openAI.CacheReadTokens != 80 {
		t.Fatalf("unexpected OpenAI totals: %+v", openAI)
	}
	assertFloatEquals(t, openAI.HitRate, 80.0)

	claude, ok := byProvider["Claude"]
	if !ok {
		t.Fatalf("expected Claude provider result")
	}
	assertFloatEquals(t, claude.HitRate, 40.0)
}

func TestGetAdminCacheHitRateByProviderUsesTotalInputAsDenominator(t *testing.T) {
	setupRequestLogTestDB(t)

	now := time.Now().UTC()
	insertRequestLogForCacheTest(t, "log-openai-1", "user-1", "gpt-5.4", 100, 80, 10, now)
	insertRequestLogForCacheTest(t, "log-openai-2", "user-2", "gpt-5.4", 200, 50, 0, now)

	repo := NewRequestLogRepository()
	rates, err := repo.GetAdminCacheHitRateByProvider()
	if err != nil {
		t.Fatalf("GetAdminCacheHitRateByProvider returned error: %v", err)
	}

	byProvider := mapRatesByProvider(rates)
	openAI, ok := byProvider["OpenAI"]
	if !ok {
		t.Fatalf("expected OpenAI provider result")
	}
	if openAI.TotalInputTokens != 300 || openAI.CacheReadTokens != 130 {
		t.Fatalf("unexpected OpenAI totals: %+v", openAI)
	}
	assertFloatEquals(t, openAI.HitRate, (130.0/300.0)*100.0)
}

func TestDisplayInputTokensSubtractsCacheReadForDisplay(t *testing.T) {
	inputTokens := 100
	cacheReadTokens := 80

	displayed := displayInputTokens(&inputTokens, &cacheReadTokens)
	if displayed == nil || *displayed != 20 {
		t.Fatalf("unexpected displayed input tokens: got %v want 20", displayed)
	}
}

func TestGetByIDWithJoinsDisplaysUncachedInputTokens(t *testing.T) {
	setupRequestLogTestDB(t)

	now := time.Now().UTC()
	insertRequestLogForCacheTest(t, "log-openai-1", "user-1", "gpt-5.4", 100, 80, 10, now)

	repo := NewRequestLogRepository()
	logEntry, err := repo.GetByIDWithJoins("log-openai-1")
	if err != nil {
		t.Fatalf("GetByIDWithJoins returned error: %v", err)
	}
	if logEntry == nil || logEntry.InputTokens == nil {
		t.Fatalf("expected log entry with input tokens")
	}
	if *logEntry.InputTokens != 20 {
		t.Fatalf("unexpected displayed input tokens: got %d want 20", *logEntry.InputTokens)
	}
	if logEntry.CacheReadInputTokens == nil || *logEntry.CacheReadInputTokens != 80 {
		t.Fatalf("unexpected cache read tokens: got %v want 80", logEntry.CacheReadInputTokens)
	}
}

func TestGetByIDWithJoinsIncludesBillingOutcomeAndGap(t *testing.T) {
	setupRequestLogTestDB(t)

	now := time.Now().UTC()
	db := database.GetDB()
	_, err := db.Exec(`
		INSERT INTO request_logs (
			id, created_at, user_id, api_key_id, original_model, method, path, status_code, latency_ms,
			is_streaming, input_tokens, output_tokens, cost_micros, cost_usd,
			charged_subscription_micros, charged_balance_micros, billing_status
		) VALUES (?, ?, ?, ?, ?, 'POST', '/v1/responses', 200, 123, 0, 100, 20, 140, '0.000140', 100, 10, 'overuse')
	`, "log-billing-1", now, "user-1", "key-user-1", "gpt-5.4")
	if err != nil {
		t.Fatalf("insert request log failed: %v", err)
	}

	repo := NewRequestLogRepository()
	logEntry, err := repo.GetByIDWithJoins("log-billing-1")
	if err != nil {
		t.Fatalf("GetByIDWithJoins returned error: %v", err)
	}
	if logEntry == nil {
		t.Fatal("expected log entry")
	}
	if logEntry.BillingStatus != "overuse" {
		t.Fatalf("billing status = %q, want overuse", logEntry.BillingStatus)
	}
	if logEntry.ChargedSubscriptionMicros != 100 || logEntry.ChargedBalanceMicros != 10 {
		t.Fatalf("charged micros = sub:%d bal:%d", logEntry.ChargedSubscriptionMicros, logEntry.ChargedBalanceMicros)
	}
	if logEntry.BillingGapMicros == nil || *logEntry.BillingGapMicros != 30 {
		t.Fatalf("billing gap = %v, want 30", logEntry.BillingGapMicros)
	}
}

func TestGetUsageSummaryDisplaysUncachedInputTokens(t *testing.T) {
	setupRequestLogTestDB(t)

	now := time.Now().UTC()
	insertRequestLogForCacheTest(t, "log-openai-1", "user-1", "gpt-5.4", 100, 80, 10, now)
	insertRequestLogForCacheTest(t, "log-openai-2", "user-1", "gpt-5.4", 200, 50, 0, now)

	repo := NewRequestLogRepository()
	userID := "user-1"
	summaries, err := repo.GetUsageSummary(&userID, nil, nil, "model", "")
	if err != nil {
		t.Fatalf("GetUsageSummary returned error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary row, got %d", len(summaries))
	}

	summary := summaries[0]
	if summary.GroupKey != "gpt-5.4" {
		t.Fatalf("unexpected group key: %s", summary.GroupKey)
	}
	if summary.InputTokensSum != 170 {
		t.Fatalf("unexpected displayed input token sum: got %d want 170", summary.InputTokensSum)
	}
	if summary.CacheReadInputTokensSum != 130 {
		t.Fatalf("unexpected cache read sum: got %d want 130", summary.CacheReadInputTokensSum)
	}
}

func TestGetDashboardStatsDisplaysUncachedInputTokens(t *testing.T) {
	setupRequestLogTestDB(t)

	now := time.Now().UTC()
	insertRequestLogForCacheTest(t, "log-openai-1", "user-1", "gpt-5.4", 100, 80, 10, now)
	insertRequestLogForCacheTest(t, "log-openai-2", "user-1", "gpt-5.4", 200, 50, 0, now)

	repo := NewRequestLogRepository()
	today, week, month, _, _, err := repo.GetDashboardStats("user-1")
	if err != nil {
		t.Fatalf("GetDashboardStats returned error: %v", err)
	}

	if today.InputTokensSum != 170 || week.InputTokensSum != 170 || month.InputTokensSum != 170 {
		t.Fatalf("unexpected displayed dashboard input sums: today=%d week=%d month=%d", today.InputTokensSum, week.InputTokensSum, month.InputTokensSum)
	}
}

func TestGetDashboardStatsFillsMissingDaysWithZeroes(t *testing.T) {
	setupRequestLogTestDB(t)

	now := time.Now().UTC()
	insertRequestLogForCacheTest(t, "log-openai-filled", "user-1", "gpt-5.4", 100, 0, 0, now)

	repo := NewRequestLogRepository()
	_, _, _, _, dailyTrend, err := repo.GetDashboardStats("user-1")
	if err != nil {
		t.Fatalf("GetDashboardStats returned error: %v", err)
	}

	if len(dailyTrend) != 14 {
		t.Fatalf("expected 14 daily trend points, got %d", len(dailyTrend))
	}

	zeroDays := 0
	requestDays := 0
	for _, point := range dailyTrend {
		if point.Requests == 0 && point.CostMicros == 0 {
			zeroDays++
		}
		if point.Requests > 0 {
			requestDays++
		}
	}

	if zeroDays == 0 {
		t.Fatalf("expected zero-filled days in trend, got %+v", dailyTrend)
	}
	if requestDays != 1 {
		t.Fatalf("expected exactly one populated trend day, got %d", requestDays)
	}
}

func setupRequestLogTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "request-log-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})
}

func insertRequestLogForCacheTest(t *testing.T, id, userID, model string, inputTokens, cacheReadTokens, cacheCreationTokens int, createdAt time.Time) {
	t.Helper()

	db := database.GetDB()
	_, err := db.Exec(`
		INSERT INTO request_logs (
			id, created_at, user_id, api_key_id, original_model, method, path, status_code, latency_ms,
			is_streaming, input_tokens, output_tokens, cache_read_input_tokens, cache_creation_input_tokens
		) VALUES (?, ?, ?, ?, ?, 'POST', '/v1/responses', 200, 123, 0, ?, 0, ?, ?)
	`, id, createdAt, userID, fmt.Sprintf("key-%s", userID), model, inputTokens, cacheReadTokens, cacheCreationTokens)
	if err != nil {
		t.Fatalf("insert request log failed: %v", err)
	}
}

func mapRatesByProvider(rates []DashboardCacheHitRate) map[string]DashboardCacheHitRate {
	results := make(map[string]DashboardCacheHitRate, len(rates))
	for _, rate := range rates {
		results[rate.Provider] = rate
	}
	return results
}

func assertFloatEquals(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("unexpected float: got %.12f want %.12f", got, want)
	}
}
