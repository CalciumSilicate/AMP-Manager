package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAdminScopedResponseCacheBucketsByQueryAndExpires(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	globalAdminResponseCache = &adminResponseCacheStore{
		entries: make(map[string]cachedUserResponse),
		inflight: make(map[string]chan struct{}),
	}

	const ttl = 50 * time.Millisecond
	callCount := 0

	router := gin.New()
	router.GET(
		"/api/admin/dashboard",
		func(c *gin.Context) {
			c.Set(ContextKeyUserID, "admin-1")
			c.Set(ContextKeyIsAdmin, true)
			c.Next()
		},
		AdminScopedResponseCache(ttl),
		func(c *gin.Context) {
			callCount++
			c.JSON(http.StatusOK, gin.H{
				"count":  callCount,
				"window": c.Query("throughputWindow"),
			})
		},
	)

	first := performAdminCacheRequest(t, router, "/api/admin/dashboard?throughputWindow=1h")
	if first["count"].(float64) != 1 {
		t.Fatalf("first count = %v, want 1", first["count"])
	}
	if callCount != 1 {
		t.Fatalf("handler call count after first request = %d, want 1", callCount)
	}

	cached := performAdminCacheRequest(t, router, "/api/admin/dashboard?throughputWindow=1h")
	if cached["count"].(float64) != 1 {
		t.Fatalf("cached count = %v, want 1", cached["count"])
	}
	if callCount != 1 {
		t.Fatalf("handler should not run on cache hit, got %d calls", callCount)
	}

	differentWindow := performAdminCacheRequest(t, router, "/api/admin/dashboard?throughputWindow=3h")
	if differentWindow["count"].(float64) != 2 {
		t.Fatalf("different window count = %v, want 2", differentWindow["count"])
	}
	if callCount != 2 {
		t.Fatalf("handler call count after different window = %d, want 2", callCount)
	}

	time.Sleep(ttl + 20*time.Millisecond)

	expired := performAdminCacheRequest(t, router, "/api/admin/dashboard?throughputWindow=1h")
	if expired["count"].(float64) != 3 {
		t.Fatalf("expired count = %v, want 3", expired["count"])
	}
	if callCount != 3 {
		t.Fatalf("handler call count after expiration = %d, want 3", callCount)
	}
}

func performAdminCacheRequest(t *testing.T, router *gin.Engine, target string) map[string]any {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	return payload
}
