package amp

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetThinkingLevelReadsReasoningEffortFromRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","reasoning":{"effort":"medium"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	level := GetThinkingLevel(c)
	if level != "medium" {
		t.Fatalf("GetThinkingLevel() = %q, want medium", level)
	}
	if got := GetThinkingLevelFromContext(c.Request.Context()); got != "medium" {
		t.Fatalf("GetThinkingLevelFromContext() = %q, want medium", got)
	}
}

func TestGetThinkingLevelPrefersContextValueOverRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","reasoning":{"effort":"medium"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req.WithContext(WithThinkingLevel(req.Context(), "high"))
	c.Set(ThinkingLevelContextKey, "high")

	level := GetThinkingLevel(c)
	if level != "high" {
		t.Fatalf("GetThinkingLevel() = %q, want high", level)
	}
}

func TestGetThinkingLevelFromContextReadsReasoningEffortAliases(t *testing.T) {
	ctx := WithRequestPayload(
		context.Background(),
		&RequestPayload{Body: []byte(`{"model":"gpt-5.4","reasoning_effort":"LOW"}`)},
	)

	level := GetThinkingLevelFromContext(ctx)
	if level != "low" {
		t.Fatalf("GetThinkingLevelFromContext() = %q, want low", level)
	}
}

func TestGetThinkingLevelFromContextFallsBackToCapturedRequestBody(t *testing.T) {
	ctx := WithCaptureData(
		context.Background(),
		&CaptureData{RequestBody: []byte(`{"model":"gpt-5.4","reasoning":{"effort":"medium"}}`)},
	)

	level := GetThinkingLevelFromContext(ctx)
	if level != "medium" {
		t.Fatalf("GetThinkingLevelFromContext() = %q, want medium", level)
	}
}
