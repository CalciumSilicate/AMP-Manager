package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ampmanager/internal/amp"
	"ampmanager/internal/database"
	"ampmanager/internal/middleware"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

func TestGetRequestLogDetailAllowsOwnFailedLogsWithoutRequestPayload(t *testing.T) {
	setupRequestLogDetailTestEnv(t)

	user := createRequestLogDetailTestUser(t, "detail-owner")
	key := createRequestLogDetailTestKey(t, user.ID, "detail-owner-key")
	insertRequestLogForDetailTest(t, "log-failed", user.ID, key.ID, http.StatusTooManyRequests, "error")
	seedRequestLogDetailTestData(t, "log-failed")

	rec := performUserRequestLogDetailRequest(t, user.ID, "/request-logs/log-failed/detail")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	for _, forbiddenField := range []string{"requestBody", "translatedRequestBody", "translatedRequestHeaders"} {
		if _, exists := payload[forbiddenField]; exists {
			t.Fatalf("expected %s to be omitted, payload = %s", forbiddenField, rec.Body.String())
		}
	}

	if payload["requestId"] != "log-failed" {
		t.Fatalf("requestId = %v, want log-failed", payload["requestId"])
	}
	if _, exists := payload["requestHeaders"]; !exists {
		t.Fatalf("expected requestHeaders to be present, payload = %s", rec.Body.String())
	}
	if _, exists := payload["responseHeaders"]; !exists {
		t.Fatalf("expected responseHeaders to be present, payload = %s", rec.Body.String())
	}
	if _, exists := payload["responseBody"]; !exists {
		t.Fatalf("expected responseBody to be present, payload = %s", rec.Body.String())
	}
	if _, exists := payload["translatedResponseBody"]; !exists {
		t.Fatalf("expected translatedResponseBody to be present, payload = %s", rec.Body.String())
	}
}

func TestGetRequestLogDetailRejectsOwnSuccessLog(t *testing.T) {
	setupRequestLogDetailTestEnv(t)

	user := createRequestLogDetailTestUser(t, "detail-success")
	key := createRequestLogDetailTestKey(t, user.ID, "detail-success-key")
	insertRequestLogForDetailTest(t, "log-success", user.ID, key.ID, http.StatusOK, "success")
	seedRequestLogDetailTestData(t, "log-success")

	rec := performUserRequestLogDetailRequest(t, user.ID, "/request-logs/log-success/detail")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "仅失败请求支持查看详情") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestGetRequestLogDetailRejectsOwnPendingLog(t *testing.T) {
	setupRequestLogDetailTestEnv(t)

	user := createRequestLogDetailTestUser(t, "detail-pending")
	key := createRequestLogDetailTestKey(t, user.ID, "detail-pending-key")
	insertRequestLogForDetailTest(t, "log-pending", user.ID, key.ID, 0, "pending")
	seedRequestLogDetailTestData(t, "log-pending")

	rec := performUserRequestLogDetailRequest(t, user.ID, "/request-logs/log-pending/detail")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGetRequestLogDetailHidesOtherUsersLogs(t *testing.T) {
	setupRequestLogDetailTestEnv(t)

	owner := createRequestLogDetailTestUser(t, "detail-owner-hidden")
	ownerKey := createRequestLogDetailTestKey(t, owner.ID, "detail-owner-hidden-key")
	viewer := createRequestLogDetailTestUser(t, "detail-viewer-hidden")
	insertRequestLogForDetailTest(t, "log-other", owner.ID, ownerKey.ID, http.StatusInternalServerError, "error")
	seedRequestLogDetailTestData(t, "log-other")

	rec := performUserRequestLogDetailRequest(t, viewer.ID, "/request-logs/log-other/detail")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminGetRequestLogDetailReturnsFullPayload(t *testing.T) {
	setupRequestLogDetailTestEnv(t)

	user := createRequestLogDetailTestUser(t, "detail-admin")
	key := createRequestLogDetailTestKey(t, user.ID, "detail-admin-key")
	insertRequestLogForDetailTest(t, "log-admin", user.ID, key.ID, http.StatusInternalServerError, "error")
	seedRequestLogDetailTestData(t, "log-admin")

	rec := performAdminRequestLogDetailRequest(t, "/request-logs/log-admin/detail")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	for _, requiredField := range []string{"requestBody", "translatedRequestBody", "translatedRequestHeaders", "responseBody"} {
		if _, exists := payload[requiredField]; !exists {
			t.Fatalf("expected %s to be present, payload = %s", requiredField, rec.Body.String())
		}
	}
}

func setupRequestLogDetailTestEnv(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "request-log-detail.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})

	prevCfg := amp.GetRequestDetailConfig()
	amp.UpdateRequestDetailConfig(amp.RequestDetailConfig{
		Enabled:              true,
		TTL:                  10 * time.Minute,
		MaxEntries:           50,
		MaxMemoryBytes:       8 * 1024 * 1024,
		BodyCapBytes:         16 * 1024,
		PersistEnabled:       false,
		HighRPMMode:          amp.RequestDetailModeFull,
		HighRPMThreshold:     amp.DefaultHighRPMThreshold,
		HighRPMSamplePercent: amp.DefaultHighRPMSamplePercent,
	})
	amp.ReinitRequestDetailStore(database.GetDB())
	t.Cleanup(func() {
		amp.UpdateRequestDetailConfig(prevCfg)
	})
}

func createRequestLogDetailTestUser(t *testing.T, username string) *model.User {
	t.Helper()

	user := &model.User{
		Username:     username,
		PasswordHash: "hash",
	}
	if err := repository.NewUserRepository().Create(user); err != nil {
		t.Fatalf("create user returned error: %v", err)
	}

	return user
}

func createRequestLogDetailTestKey(t *testing.T, userID, name string) *model.CreateAPIKeyResponse {
	t.Helper()

	created, err := service.NewAmpService().CreateAPIKey(userID, &model.CreateAPIKeyRequest{
		Name:      name,
		CustomKey: "sk-" + strings.ReplaceAll(name, "-", "") + "1234567890ABCDEF",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	return created
}

func insertRequestLogForDetailTest(t *testing.T, id, userID, apiKeyID string, statusCode int, status string) {
	t.Helper()

	_, err := database.GetDB().Exec(`
		INSERT INTO request_logs (
			id, created_at, user_id, api_key_id, original_model, method, path, status_code, latency_ms,
			is_streaming, status, billing_status
		) VALUES (?, ?, ?, ?, ?, 'POST', '/v1/responses', ?, 123, 0, ?, 'none')
	`, id, time.Now().UTC(), userID, apiKeyID, "gpt-5.4", statusCode, status)
	if err != nil {
		t.Fatalf("insert request log returned error: %v", err)
	}
}

func seedRequestLogDetailTestData(t *testing.T, requestID string) {
	t.Helper()

	store := amp.GetRequestDetailStore()
	if store == nil {
		t.Fatal("request detail store not initialized")
	}

	store.UpdateRequestData(requestID, http.Header{
		"Authorization": []string{"Bearer sk-test"},
		"Content-Type":  []string{"application/json"},
	}, []byte(`{"request":"body"}`))
	store.UpdateTranslatedRequestHeaders(requestID, http.Header{
		"X-Upstream-Auth": []string{"masked"},
	})
	store.UpdateTranslatedRequestBody(requestID, []byte(`{"translated":"request"}`))
	store.UpdateResponseData(requestID, http.Header{
		"Content-Type": []string{"application/json"},
	}, []byte(`{"error":"rate limit"}`))
	store.AppendTranslatedResponse(requestID, []byte(`{"translated":"response"}`))
}

func performUserRequestLogDetailRequest(t *testing.T, userID, target string) *httptest.ResponseRecorder {
	t.Helper()

	handler := NewRequestLogHandler()
	router := gin.New()
	router.GET("/request-logs/:id/detail", func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, userID)
		handler.GetRequestLogDetail(c)
	})

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func performAdminRequestLogDetailRequest(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()

	handler := NewRequestLogHandler()
	router := gin.New()
	router.GET("/request-logs/:id/detail", handler.AdminGetRequestLogDetail)

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
