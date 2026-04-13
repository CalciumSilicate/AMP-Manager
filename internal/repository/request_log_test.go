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
