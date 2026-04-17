package amp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ampmanager/internal/translator"
)

func TestHandleNonStreamingResponseStoresUpstreamAndTranslatedBodiesSeparately(t *testing.T) {
	store := setupRequestDetailStoreForSemanticsTest(t)

	requestID := "req-nonstream"
	originalRequest := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}]}`)
	store.UpdateRequestData(requestID, http.Header{"Content-Type": []string{"application/json"}}, originalRequest)

	translatedRequest, err := translator.TranslateRequest(translator.FormatOpenAIChat, translator.FormatOpenAIResponses, "gpt-5.4", originalRequest, false)
	if err != nil {
		t.Fatalf("TranslateRequest returned error: %v", err)
	}

	rawUpstreamResponse := []byte(`{
		"id":"resp_123",
		"object":"response",
		"created_at":1776291683,
		"status":"completed",
		"model":"gpt-5.4",
		"output":[
			{
				"type":"message",
				"content":[{"type":"output_text","text":"OK"}]
			}
		],
		"usage":{"input_tokens":22,"output_tokens":5,"total_tokens":27}
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx := WithProviderInfo(context.Background(), ProviderInfo{Provider: ProviderOpenAIResponses})
	ctx = WithRequestDetailCaptureEnabled(ctx, true)
	req = req.WithContext(ctx)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(rawUpstreamResponse)),
		Request:    req,
	}

	trace := NewRequestTrace(requestID, "user-1", "key-1", http.MethodPost, "/v1/chat/completions")
	var responseParam any
	transInfo := &TranslationInfo{
		NeedsConversion:     true,
		IncomingFormat:      translator.FormatOpenAIChat,
		OutgoingFormat:      translator.FormatOpenAIResponses,
		OriginalRequestBody: originalRequest,
		ConvertedBody:       translatedRequest,
		Model:               "gpt-5.4",
		ResponseParam:       &responseParam,
	}

	if err := handleNonStreamingResponse(resp, trace, transInfo, "gpt-5.4", "gpt-5.4"); err != nil {
		t.Fatalf("handleNonStreamingResponse returned error: %v", err)
	}

	detail := store.Get(requestID)
	if detail == nil {
		t.Fatal("expected request detail to be stored")
	}
	if compactJSON(t, detail.ResponseBody) != compactJSON(t, rawUpstreamResponse) {
		t.Fatalf("responseBody should keep upstream payload\n got: %s\nwant: %s", detail.ResponseBody, rawUpstreamResponse)
	}
	if len(detail.TranslatedResponseBody) == 0 {
		t.Fatal("expected translatedResponseBody to store downstream payload")
	}
	if !strings.Contains(string(detail.TranslatedResponseBody), `"object":"chat.completion"`) {
		t.Fatalf("expected translatedResponseBody to be chat completion payload, got: %s", detail.TranslatedResponseBody)
	}

	finalBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll returned error: %v", err)
	}
	if compactJSON(t, detail.TranslatedResponseBody) != compactJSON(t, finalBody) {
		t.Fatalf("translatedResponseBody should match final downstream response\n got: %s\nwant: %s", detail.TranslatedResponseBody, finalBody)
	}

	if ptrToInt(trace.InputTokens) != 22 {
		t.Fatalf("input tokens = %d, want 22", ptrToInt(trace.InputTokens))
	}
	if ptrToInt(trace.OutputTokens) != 5 {
		t.Fatalf("output tokens = %d, want 5", ptrToInt(trace.OutputTokens))
	}
	if trace.CacheReadInputTokens != nil {
		t.Fatalf("expected cache read tokens to be nil, got %d", ptrToInt(trace.CacheReadInputTokens))
	}
}

func TestStreamingResponseCaptureStoresUpstreamAndTranslatedBodiesSeparately(t *testing.T) {
	store := setupRequestDetailStoreForSemanticsTest(t)

	requestID := "req-stream"
	originalRequest := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	store.UpdateRequestData(requestID, http.Header{"Content-Type": []string{"application/json"}}, originalRequest)

	translatedRequest, err := translator.TranslateRequest(translator.FormatOpenAIChat, translator.FormatOpenAIResponses, "gpt-5.4", originalRequest, true)
	if err != nil {
		t.Fatalf("TranslateRequest returned error: %v", err)
	}

	rawSSE := strings.Join([]string{
		"event: response.created\n",
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":123,\"model\":\"gpt-5.4\",\"status\":\"in_progress\"}}\n\n",
		"event: response.output_text.delta\n",
		"data: {\"type\":\"response.output_text.delta\",\"response_id\":\"resp_1\",\"output_index\":0,\"delta\":\"hello\"}\n\n",
		"event: response.completed\n",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":123,\"model\":\"gpt-5.4\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":21,\"output_tokens\":8,\"total_tokens\":29}}}\n\n",
		"data: [DONE]\n\n",
	}, "")

	trace := NewRequestTrace(requestID, "user-1", "key-1", http.MethodPost, "/v1/chat/completions")
	body := io.ReadCloser(nopReadCloser{Reader: strings.NewReader(rawSSE)})
	body = NewResponseCaptureWrapper(body, requestID, http.Header{"Content-Type": []string{"text/event-stream"}})
	body = WrapResponseBodyForTokenExtraction(body, true, trace, ProviderInfo{Provider: ProviderOpenAIResponses})

	var responseParam any
	body = NewSSEJSONTransformWrapper(body, func(b []byte) [][]byte {
		payloads, err := translator.TranslateStream(
			context.Background(),
			translator.FormatOpenAIChat,
			translator.FormatOpenAIResponses,
			"gpt-5.4",
			originalRequest,
			translatedRequest,
			b,
			&responseParam,
		)
		if err != nil {
			t.Fatalf("TranslateStream returned error: %v", err)
		}
		out := make([][]byte, 0, len(payloads))
		for _, payload := range payloads {
			out = append(out, []byte(payload))
		}
		return out
	})
	body = NewTranslatedResponseCaptureWrapper(body, requestID)

	output, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("io.ReadAll returned error: %v", err)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	detail := store.Get(requestID)
	if detail == nil {
		t.Fatal("expected request detail to be stored")
	}
	if !strings.Contains(string(detail.ResponseBody), `"type":"response.completed"`) {
		t.Fatalf("responseBody should keep upstream Responses SSE, got: %s", detail.ResponseBody)
	}
	if !strings.Contains(string(detail.TranslatedResponseBody), `"object":"chat.completion.chunk"`) {
		t.Fatalf("translatedResponseBody should keep downstream compatible SSE, got: %s", detail.TranslatedResponseBody)
	}
	if string(detail.TranslatedResponseBody) != string(output) {
		t.Fatalf("translatedResponseBody should match final downstream SSE\n got: %s\nwant: %s", detail.TranslatedResponseBody, output)
	}

	if ptrToInt(trace.InputTokens) != 21 {
		t.Fatalf("input tokens = %d, want 21", ptrToInt(trace.InputTokens))
	}
	if ptrToInt(trace.OutputTokens) != 8 {
		t.Fatalf("output tokens = %d, want 8", ptrToInt(trace.OutputTokens))
	}
	if trace.CacheReadInputTokens != nil {
		t.Fatalf("expected cache read tokens to be nil, got %d", ptrToInt(trace.CacheReadInputTokens))
	}
}

func setupRequestDetailStoreForSemanticsTest(t *testing.T) *RequestDetailStore {
	t.Helper()

	prevCfg := GetRequestDetailConfig()
	prevStore := globalDetailStore

	UpdateRequestDetailConfig(RequestDetailConfig{
		Enabled:              true,
		TTL:                  10 * time.Minute,
		MaxEntries:           16,
		MaxMemoryBytes:       64 * 1024 * 1024,
		BodyCapBytes:         256 * 1024,
		PersistEnabled:       false,
		HighRPMMode:          RequestDetailModeFull,
		HighRPMThreshold:     DefaultHighRPMThreshold,
		HighRPMSamplePercent: DefaultHighRPMSamplePercent,
	})

	store := NewRequestDetailStore(nil, 0)
	globalDetailStore = store

	t.Cleanup(func() {
		store.Stop()
		globalDetailStore = prevStore
		UpdateRequestDetailConfig(prevCfg)
	})

	return store
}

func compactJSON(t *testing.T, raw []byte) string {
	t.Helper()

	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		t.Fatalf("compactJSON returned error: %v", err)
	}
	return out.String()
}
