package amp

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestCaptureMiddlewareAssignsRequestIDBeforeSampling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prevCfg := GetRequestDetailConfig()
	prevRPMState := requestDetailRPMState
	t.Cleanup(func() {
		requestDetailRPMState = prevRPMState
		UpdateRequestDetailConfig(prevCfg)
	})

	resetRequestDetailRPMCounterForTest()
	UpdateRequestDetailConfig(RequestDetailConfig{
		Enabled:              true,
		TTL:                  10 * time.Minute,
		MaxEntries:           10,
		MaxMemoryBytes:       64 * 1024 * 1024,
		BodyCapBytes:         64,
		PersistEnabled:       false,
		HighRPMMode:          RequestDetailModeSample,
		HighRPMThreshold:     1,
		HighRPMSamplePercent: 100,
	})

	var requestID string
	var captureEnabled bool

	router := gin.New()
	router.Use(RequestCaptureMiddleware())
	router.POST("/v1/responses", func(c *gin.Context) {
		requestID = GetRequestID(c.Request.Context())
		captureEnabled = IsRequestDetailCaptureEnabled(c.Request.Context())
		c.Status(204)
	})

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-4.1-mini"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if requestID == "" {
		t.Fatal("expected request detail sampling to assign a request ID before capture gating")
	}
	if !captureEnabled {
		t.Fatal("expected capture to remain enabled when sample percent is 100")
	}
}
