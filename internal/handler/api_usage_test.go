package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ampmanager/internal/amp"
	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

func TestAPIUsageReturnsBalanceAndSubscriptionWindows(t *testing.T) {
	setupAPIUsageTestDB(t)
	gin.SetMode(gin.TestMode)

	user := createAPIUsageTestUser(t, "usage-active", 5_000_000)
	created := createAPIUsageTestKey(t, user.ID, "active-key", nil)

	plan := &model.SubscriptionPlan{
		Name:        "专业版",
		Description: "test plan",
		Enabled:     true,
	}
	limits := []model.SubscriptionPlanLimit{
		{
			LimitType:   model.LimitTypeMonthly,
			WindowMode:  model.WindowModeFixed,
			LimitMicros: 3_000_000,
		},
	}
	if err := repository.NewSubscriptionPlanRepository().Create(plan, limits); err != nil {
		t.Fatalf("create plan returned error: %v", err)
	}

	sub := &model.UserSubscription{
		UserID:   user.ID,
		PlanID:   plan.ID,
		StartsAt: time.Now().UTC().Add(-24 * time.Hour),
		Status:   model.SubscriptionStatusActive,
	}
	if err := repository.NewUserSubscriptionRepository().Assign(sub); err != nil {
		t.Fatalf("assign subscription returned error: %v", err)
	}

	if err := repository.NewBillingEventRepository().Create(&model.BillingEvent{
		UserID:             user.ID,
		UserSubscriptionID: &sub.ID,
		Source:             model.BillingSourceSubscription,
		EventType:          "charge",
		AmountMicros:       1_250_000,
	}); err != nil {
		t.Fatalf("create billing event returned error: %v", err)
	}

	insertAPIUsageRequestLog(t, "log-1", user.ID, created.ID, time.Now().UTC().Add(-2*time.Hour), 250_000)
	insertAPIUsageRequestLog(t, "log-2", user.ID, created.ID, time.Now().UTC().Add(-48*time.Hour), 500_000)

	router := gin.New()
	router.GET("/api/usage", amp.APIKeyAuthMiddleware(), NewAmpHandler().GetAPIUsage)

	req := httptest.NewRequest(http.MethodGet, "/api/usage", nil)
	req.Header.Set("Authorization", "Bearer "+created.APIKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	time.Sleep(20 * time.Millisecond)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var items []model.CCSwitchUsageItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 usage items, got %d", len(items))
	}

	if items[0].PlanName == nil || *items[0].PlanName != "余额" {
		t.Fatalf("expected first item to be balance, got %+v", items[0])
	}
	if items[0].Remaining == nil || *items[0].Remaining != 5 {
		t.Fatalf("expected balance remaining 5, got %+v", items[0].Remaining)
	}
	if items[0].Extra == nil || !strings.Contains(*items[0].Extra, "2 次请求") || !strings.Contains(*items[0].Extra, "$0.750000") {
		t.Fatalf("unexpected balance extra: %+v", items[0].Extra)
	}

	if items[1].PlanName == nil || !strings.Contains(*items[1].PlanName, "专业版") || !strings.Contains(*items[1].PlanName, "月额度") {
		t.Fatalf("unexpected subscription item name: %+v", items[1].PlanName)
	}
	if items[1].Total == nil || *items[1].Total != 3 {
		t.Fatalf("expected total 3, got %+v", items[1].Total)
	}
	if items[1].Used == nil || *items[1].Used != 1.25 {
		t.Fatalf("expected used 1.25, got %+v", items[1].Used)
	}
	if items[1].Remaining == nil || *items[1].Remaining != 1.75 {
		t.Fatalf("expected remaining 1.75, got %+v", items[1].Remaining)
	}
}

func TestAPIUsageReturnsBalanceOnlyWithoutSubscription(t *testing.T) {
	setupAPIUsageTestDB(t)
	gin.SetMode(gin.TestMode)

	user := createAPIUsageTestUser(t, "usage-balance-only", 2_500_000)
	created := createAPIUsageTestKey(t, user.ID, "balance-only-key", nil)
	insertAPIUsageRequestLog(t, "log-1", user.ID, created.ID, time.Now().UTC().Add(-time.Hour), 125_000)

	router := gin.New()
	router.GET("/api/usage", amp.APIKeyAuthMiddleware(), NewAmpHandler().GetAPIUsage)

	req := httptest.NewRequest(http.MethodGet, "/api/usage", nil)
	req.Header.Set("Authorization", "Bearer "+created.APIKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	time.Sleep(20 * time.Millisecond)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var items []model.CCSwitchUsageItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 usage item, got %d", len(items))
	}
	if items[0].PlanName == nil || *items[0].PlanName != "余额" {
		t.Fatalf("expected balance item, got %+v", items[0])
	}
	if items[0].Remaining == nil || *items[0].Remaining != 2.5 {
		t.Fatalf("expected remaining 2.5, got %+v", items[0].Remaining)
	}
}

func TestAPIUsageRejectsDisabledKey(t *testing.T) {
	setupAPIUsageTestDB(t)
	gin.SetMode(gin.TestMode)

	user := createAPIUsageTestUser(t, "usage-disabled", 1_000_000)
	created := createAPIUsageTestKey(t, user.ID, "disabled-key", nil)
	if err := repository.NewAPIKeyRepository().SetRevoked(created.ID, true); err != nil {
		t.Fatalf("SetRevoked returned error: %v", err)
	}

	router := gin.New()
	router.GET("/api/usage", amp.APIKeyAuthMiddleware(), NewAmpHandler().GetAPIUsage)

	req := httptest.NewRequest(http.MethodGet, "/api/usage", nil)
	req.Header.Set("Authorization", "Bearer "+created.APIKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAPIUsageRejectsExpiredKey(t *testing.T) {
	setupAPIUsageTestDB(t)
	gin.SetMode(gin.TestMode)

	user := createAPIUsageTestUser(t, "usage-expired", 1_000_000)
	expiresAt := time.Now().UTC().Add(-time.Hour)
	created := createAPIUsageTestKey(t, user.ID, "expired-key", &expiresAt)

	router := gin.New()
	router.GET("/api/usage", amp.APIKeyAuthMiddleware(), NewAmpHandler().GetAPIUsage)

	req := httptest.NewRequest(http.MethodGet, "/api/usage", nil)
	req.Header.Set("Authorization", "Bearer "+created.APIKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func setupAPIUsageTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "api-usage-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}

	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})
}

func createAPIUsageTestUser(t *testing.T, username string, balanceMicros int64) *model.User {
	t.Helper()

	user := &model.User{
		Username:      username,
		PasswordHash:  "hash",
		BalanceMicros: balanceMicros,
	}

	if err := repository.NewUserRepository().Create(user); err != nil {
		t.Fatalf("create user returned error: %v", err)
	}

	return user
}

func createAPIUsageTestKey(t *testing.T, userID, name string, expiresAt *time.Time) *model.CreateAPIKeyResponse {
	t.Helper()

	created, err := service.NewAmpService().CreateAPIKey(userID, &model.CreateAPIKeyRequest{
		Name:      name,
		CustomKey: "sk-" + strings.ReplaceAll(name, "-", "") + "1234567890ABCDEF",
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	return created
}

func insertAPIUsageRequestLog(t *testing.T, id, userID, apiKeyID string, createdAt time.Time, costMicros int64) {
	t.Helper()

	if _, err := database.GetDB().Exec(
		`INSERT INTO request_logs (id, created_at, user_id, api_key_id, method, path, status_code, latency_ms, cost_micros, cost_usd, billing_status)
		 VALUES (?, ?, ?, ?, 'POST', '/v1/responses', 200, 120, ?, ?, 'none')`,
		id,
		createdAt.UTC(),
		userID,
		apiKeyID,
		costMicros,
		formatUsageCost(costMicros),
	); err != nil {
		t.Fatalf("insert request log returned error: %v", err)
	}
}

func formatUsageCost(costMicros int64) string {
	return strings.TrimRight(strings.TrimRight(fmtFloat(float64(costMicros)/1e6), "0"), ".")
}

func fmtFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}
