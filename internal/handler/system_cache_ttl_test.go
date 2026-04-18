package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ampmanager/internal/database"
	"ampmanager/internal/repository"

	"github.com/gin-gonic/gin"
)

func TestGetCacheTTLConfigDefaultsToNoOverride(t *testing.T) {
	setupSystemCacheTTLTestDB(t)
	gin.SetMode(gin.TestMode)

	if err := repository.NewSystemConfigRepository().Delete(cacheTTLConfigKey); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	router := gin.New()
	router.GET("/cache-ttl", NewSystemHandler().GetCacheTTLConfig)

	req := httptest.NewRequest(http.MethodGet, "/cache-ttl", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		CacheTTL string `json:"cacheTTL"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if resp.CacheTTL != "" {
		t.Fatalf("cacheTTL = %q, want empty string", resp.CacheTTL)
	}
}

func TestGetCacheTTLConfigReturnsStoredOverride(t *testing.T) {
	setupSystemCacheTTLTestDB(t)
	gin.SetMode(gin.TestMode)

	if err := repository.NewSystemConfigRepository().Set(cacheTTLConfigKey, "5m"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	router := gin.New()
	router.GET("/cache-ttl", NewSystemHandler().GetCacheTTLConfig)

	req := httptest.NewRequest(http.MethodGet, "/cache-ttl", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		CacheTTL string `json:"cacheTTL"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if resp.CacheTTL != "5m" {
		t.Fatalf("cacheTTL = %q, want 5m", resp.CacheTTL)
	}
}

func setupSystemCacheTTLTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "system-cache-ttl.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}

	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})
}
