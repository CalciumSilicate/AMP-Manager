package amp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ampmanager/internal/billingstate"

	"github.com/gin-gonic/gin"
)

func TestEstimateReservationInputTokens_OpenAIChat(t *testing.T) {
	payload := mustEstimatePayload(t, `{
		"model": "gpt-4.1",
		"messages": [
			{"role": "system", "content": "You are a concise assistant."},
			{"role": "user", "content": [
				{"type": "text", "text": "Summarize 你好世界 and explain the diff."},
				{"type": "input_text", "text": "The patch adds validation and retries."}
			]}
		],
		"tools": [{
			"type": "function",
			"function": {
				"name": "lookup_diff",
				"description": "Fetch a diff summary",
				"parameters": {
					"type": "object",
					"properties": {
						"path": {"type": "string", "description": "target file"}
					}
				}
			}
		}],
		"response_format": {
			"type": "json_schema",
			"json_schema": {
				"name": "summary",
				"schema": {
					"type": "object",
					"properties": {
						"summary": {"type": "string"}
					}
				}
			}
		}
	}`)

	withoutSchemas := mustEstimatePayload(t, `{
		"messages": [{"role": "user", "content": "Summarize 你好世界 and explain the diff."}]
	}`)

	got := estimateReservationInputTokens(payload, billingRequestOpenAIChat)
	baseline := estimateReservationInputTokens(withoutSchemas, billingRequestOpenAIChat)
	if got <= baseline {
		t.Fatalf("expected structured chat estimate %d to exceed baseline %d", got, baseline)
	}
}

func TestEstimateReservationInputTokens_OpenAIResponses(t *testing.T) {
	payload := mustEstimatePayload(t, `{
		"model": "gpt-5",
		"instructions": "Answer as JSON only.",
		"input": [
			{
				"role": "user",
				"content": [
					{"type": "input_text", "text": "Compare alpha and beta."},
					{"type": "input_text", "text": "再补充中文说明"}
				]
			}
		],
		"tools": [{
			"type": "function",
			"name": "fetch_notes",
			"description": "Fetch extra notes",
			"parameters": {
				"type": "object",
				"properties": {
					"topic": {"type": "string"}
				}
			}
		}],
		"text": {
			"format": {
				"type": "json_schema",
				"name": "answer",
				"schema": {
					"type": "object",
					"properties": {
						"result": {"type": "string"}
					}
				}
			}
		}
	}`)

	got := estimateReservationInputTokens(payload, billingRequestOpenAIResponses)
	if got <= 0 {
		t.Fatalf("expected positive responses estimate, got %d", got)
	}
}

func TestEstimateReservationInputTokens_Anthropic(t *testing.T) {
	payload := mustEstimatePayload(t, `{
		"model": "claude-sonnet-4-5",
		"system": [{"type": "text", "text": "Keep answers short."}],
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "Explain the deployment plan."},
				{"type": "tool_result", "content": [{"type": "text", "text": "build passed"}]}
			]
		}],
		"tools": [{
			"name": "run_checks",
			"description": "Run validation checks",
			"input_schema": {
				"type": "object",
				"properties": {
					"scope": {"type": "string"}
				}
			}
		}]
	}`)

	got := estimateReservationInputTokens(payload, billingRequestAnthropic)
	if got <= 0 {
		t.Fatalf("expected positive anthropic estimate, got %d", got)
	}
}

func TestEstimateReservationInputTokens_Gemini(t *testing.T) {
	payload := mustEstimatePayload(t, `{
		"systemInstruction": {
			"parts": [{"text": "Return JSON only"}]
		},
		"contents": [{
			"role": "user",
			"parts": [
				{"text": "Summarize the release note."},
				{"functionCall": {"name": "lookup_docs", "args": {"slug": "billing"}}}
			]
		}],
		"tools": [{
			"functionDeclarations": [{
				"name": "lookup_docs",
				"description": "Read release docs",
				"parameters": {
					"type": "object",
					"properties": {"slug": {"type": "string"}}
				}
			}]
		}],
		"generationConfig": {
			"responseSchema": {
				"type": "object",
				"properties": {
					"summary": {"type": "string"}
				}
			}
		}
	}`)

	got := estimateReservationInputTokens(payload, billingRequestGemini)
	if got <= 0 {
		t.Fatalf("expected positive gemini estimate, got %d", got)
	}
}

func TestApproximateTextTokenCount_IsUnicodeAware(t *testing.T) {
	ascii := approximateTextTokenCount("abcdefgh")
	cjk := approximateTextTokenCount("你好世界")
	if ascii != 2 {
		t.Fatalf("expected ascii estimate 2, got %d", ascii)
	}
	if cjk != 4 {
		t.Fatalf("expected cjk estimate 4, got %d", cjk)
	}
	if cjk <= ascii {
		t.Fatalf("expected cjk estimate %d to exceed ascii estimate %d", cjk, ascii)
	}
}

func TestExtractReservationMaxOutputTokens(t *testing.T) {
	t.Run("explicit value wins below metadata", func(t *testing.T) {
		got := extractReservationMaxOutputTokens(map[string]interface{}{
			"max_output_tokens": 1024.0,
		}, "gpt-4.1")
		if got != 1024 {
			t.Fatalf("expected explicit max_output_tokens, got %d", got)
		}
	})

	t.Run("metadata clamps oversized explicit value", func(t *testing.T) {
		got := extractReservationMaxOutputTokens(map[string]interface{}{
			"max_completion_tokens": 999999.0,
		}, "gpt-4.1")
		if got != 32768 {
			t.Fatalf("expected metadata clamp 32768, got %d", got)
		}
	})

	t.Run("metadata fallback when explicit absent", func(t *testing.T) {
		got := extractReservationMaxOutputTokens(nil, "claude-4")
		if got != 64000 {
			t.Fatalf("expected metadata fallback 64000, got %d", got)
		}
	})

	t.Run("o-series default remains", func(t *testing.T) {
		got := extractReservationMaxOutputTokens(nil, "o3-mini")
		if got != 65536 {
			t.Fatalf("expected o-series default 65536, got %d", got)
		}
	})

	t.Run("default fallback remains", func(t *testing.T) {
		got := extractReservationMaxOutputTokens(nil, "unknown-model")
		if got != defaultReservationMaxOutputTokens {
			t.Fatalf("expected default fallback %d, got %d", defaultReservationMaxOutputTokens, got)
		}
	})
}

func TestDetectBillingRequestKind(t *testing.T) {
	cases := map[string]billingRequestKind{
		"/v1/chat/completions":                                           billingRequestOpenAIChat,
		"/api/provider/openai/v1/responses":                              billingRequestOpenAIResponses,
		"/v1/messages":                                                   billingRequestAnthropic,
		"/v1beta/models/gemini-2.5-pro:generateContent":                  billingRequestGemini,
		"/v1beta1/publishers/google/models/gemini:streamGenerateContent": billingRequestGemini,
	}

	for path, want := range cases {
		if got := detectBillingRequestKind(path); got != want {
			t.Fatalf("path %s: expected %q, got %q", path, want, got)
		}
	}
}

func TestBillingEstimateMiddleware_SkipsBodyReadWhenRuntimeDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousRuntime := billingstate.Replace(nil)
	defer billingstate.Replace(previousRuntime)

	body := &trackingReadCloser{Reader: bytes.NewBufferString(`{"model":"gpt-4.1","messages":[{"role":"user","content":"hello"}]}`)}
	var estimate *BillingEstimate
	var payload *RequestPayload

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), &ProxyConfig{
			UserID:         "user-1",
			RateMultiplier: 1,
		}))
		c.Next()
	})
	router.Use(BillingEstimateMiddleware())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		estimate = GetBillingEstimate(c.Request.Context())
		payload = GetRequestPayload(c.Request.Context())
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Body = body
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if body.reads != 0 {
		t.Fatalf("expected request body to remain unread, got %d reads", body.reads)
	}
	if estimate != nil {
		t.Fatalf("expected no billing estimate when runtime is disabled, got %#v", estimate)
	}
	if payload != nil {
		t.Fatalf("expected request payload cache to remain empty when runtime is disabled")
	}
}

func TestBillingEstimateMiddleware_SkipsBodyReadForFreeRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousRuntime := billingstate.Replace(&billingstate.Runtime{})
	defer billingstate.Replace(previousRuntime)

	body := &trackingReadCloser{Reader: bytes.NewBufferString(`{"model":"gpt-4.1","messages":[{"role":"user","content":"hello"}]}`)}
	var estimate *BillingEstimate
	var payload *RequestPayload

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), &ProxyConfig{
			UserID:         "user-1",
			RateMultiplier: 0,
		}))
		c.Next()
	})
	router.Use(BillingEstimateMiddleware())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		estimate = GetBillingEstimate(c.Request.Context())
		payload = GetRequestPayload(c.Request.Context())
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Body = body
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if body.reads != 0 {
		t.Fatalf("expected request body to remain unread for free request, got %d reads", body.reads)
	}
	if estimate != nil {
		t.Fatalf("expected no billing estimate for free request, got %#v", estimate)
	}
	if payload != nil {
		t.Fatalf("expected request payload cache to remain empty for free request")
	}
}

func TestBillingEstimateMiddleware_ReadsBodyWhenReservationMayBeNeeded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousRuntime := billingstate.Replace(&billingstate.Runtime{})
	defer billingstate.Replace(previousRuntime)

	body := &trackingReadCloser{Reader: bytes.NewBufferString(`{"model":"gpt-4.1","messages":[{"role":"user","content":"hello"}],"max_tokens":42}`)}
	var estimate *BillingEstimate
	var payload *RequestPayload

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), &ProxyConfig{
			UserID:         "user-1",
			RateMultiplier: 1,
		}))
		c.Next()
	})
	router.Use(BillingEstimateMiddleware())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		estimate = GetBillingEstimate(c.Request.Context())
		payload = GetRequestPayload(c.Request.Context())
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Body = body
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if body.reads == 0 {
		t.Fatal("expected request body to be read when reservation may be needed")
	}
	if payload == nil || payload.JSON == nil {
		t.Fatal("expected request payload cache to be populated")
	}
	if estimate == nil {
		t.Fatal("expected billing estimate to be populated")
	}
	if estimate.PricingModel != "gpt-4.1" {
		t.Fatalf("expected pricing model gpt-4.1, got %q", estimate.PricingModel)
	}
	if estimate.EstimatedOutputTokens != 42 {
		t.Fatalf("expected max tokens 42, got %d", estimate.EstimatedOutputTokens)
	}
}

func TestEstimateReservationInputTokensForPayloadFallsBackOnLargeBody(t *testing.T) {
	largeText := strings.Repeat("hello world ", largeEstimateBodyThresholdBytes/12+32)
	raw := []byte(fmt.Sprintf(`{"model":"gpt-4.1","input":[{"role":"user","content":"%s"}],"max_tokens":42}`, largeText))

	tokens, usedFallback := estimateReservationInputTokensForPayload(
		mustEstimatePayload(t, string(raw)),
		raw,
		billingRequestOpenAIResponses,
	)

	if !usedFallback {
		t.Fatal("expected large payload to use fallback estimate")
	}
	if tokens != approximateBytesTokenCount(raw) {
		t.Fatalf("tokens = %d, want fallback estimate %d", tokens, approximateBytesTokenCount(raw))
	}
}

func TestEstimateReservationInputTokensForPayloadUsesStructuredEstimateForSmallBody(t *testing.T) {
	raw := []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"hello world"}],"max_tokens":42}`)
	payload := mustEstimatePayload(t, string(raw))

	tokens, usedFallback := estimateReservationInputTokensForPayload(payload, raw, billingRequestOpenAIChat)

	if usedFallback {
		t.Fatal("expected small payload to use structured estimate")
	}
	want := estimateReservationInputTokens(payload, billingRequestOpenAIChat)
	if tokens != want {
		t.Fatalf("tokens = %d, want %d", tokens, want)
	}
}

func TestBillingEstimateMiddleware_UsesFallbackEstimateForLargeBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousRuntime := billingstate.Replace(&billingstate.Runtime{})
	defer billingstate.Replace(previousRuntime)

	largeText := strings.Repeat("hello world ", largeEstimateBodyThresholdBytes/12+32)
	raw := fmt.Sprintf(`{"model":"gpt-4.1","input":[{"role":"user","content":"%s"}],"max_tokens":42}`, largeText)
	body := &trackingReadCloser{Reader: bytes.NewBufferString(raw)}
	var estimate *BillingEstimate

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), &ProxyConfig{
			UserID:         "user-1",
			RateMultiplier: 1,
		}))
		c.Next()
	})
	router.Use(BillingEstimateMiddleware())
	router.POST("/v1/responses", func(c *gin.Context) {
		estimate = GetBillingEstimate(c.Request.Context())
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req.Body = body
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if estimate == nil {
		t.Fatal("expected billing estimate to be populated")
	}
	want := approximateBytesTokenCount([]byte(raw))
	if estimate.EstimatedInputTokens != want {
		t.Fatalf("EstimatedInputTokens = %d, want %d", estimate.EstimatedInputTokens, want)
	}
	if estimate.EstimatedOutputTokens != 42 {
		t.Fatalf("expected max tokens 42, got %d", estimate.EstimatedOutputTokens)
	}
}

func TestBillingEstimateMiddleware_LargeBodyDefersJSONParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousRuntime := billingstate.Replace(&billingstate.Runtime{})
	defer billingstate.Replace(previousRuntime)

	largeText := strings.Repeat("hello world ", largeEstimateBodyThresholdBytes/12+32)
	raw := fmt.Sprintf(`{"model":"gpt-4.1","input":[{"role":"user","content":"%s"}],"max_tokens":42}`, largeText)
	var payload *RequestPayload

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), &ProxyConfig{
			UserID:         "user-1",
			RateMultiplier: 1,
		}))
		c.Next()
	})
	router.Use(BillingEstimateMiddleware())
	router.POST("/v1/responses", func(c *gin.Context) {
		payload = GetRequestPayload(c.Request.Context())
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if payload == nil {
		t.Fatal("expected request payload to be cached")
	}
	if payload.JSON != nil {
		t.Fatal("expected large payload path to defer JSON parsing")
	}
	if payload.jsonParsed {
		t.Fatal("expected large payload path to leave jsonParsed false")
	}
}

func TestEnsureRequestPayloadParsesCachedLargeBodyOnDemand(t *testing.T) {
	gin.SetMode(gin.TestMode)

	largeText := strings.Repeat("hello world ", largeEstimateBodyThresholdBytes/12+32)
	raw := fmt.Sprintf(`{"model":"gpt-4.1","input":[{"role":"user","content":"%s"}],"max_tokens":42}`, largeText)

	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		payload, err := ensureRequestBody(c)
		if err != nil {
			t.Fatalf("ensureRequestBody returned error: %v", err)
		}
		if payload.JSON != nil || payload.jsonParsed {
			t.Fatalf("expected ensureRequestBody to avoid eager JSON parsing")
		}
		payload, err = EnsureRequestPayload(c)
		if err != nil {
			t.Fatalf("EnsureRequestPayload returned error: %v", err)
		}
		if payload.JSON == nil || !payload.jsonParsed {
			t.Fatalf("expected EnsureRequestPayload to parse cached body on demand")
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func mustEstimatePayload(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

type trackingReadCloser struct {
	io.Reader
	reads int
}

func (t *trackingReadCloser) Read(p []byte) (int, error) {
	t.reads++
	return t.Reader.Read(p)
}

func (t *trackingReadCloser) Close() error {
	return nil
}
