package amp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"nhooyr.io/websocket"
)

func TestNormalizeResponsesCreateRequestRequiresModel(t *testing.T) {
	_, err := normalizeResponsesCreateRequest([]byte(`{"type":"response.create","input":[]}`))
	if err == nil {
		t.Fatalf("expected error for missing model")
	}
}

func TestNormalizeResponsesSubsequentRequestPreservesPreviousResponseID(t *testing.T) {
	lastRequest := []byte(`{"model":"gpt-5-codex","instructions":"test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)
	raw := []byte(`{"type":"response.append","previous_response_id":"resp-1","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"next"}]}]}`)

	normalized, err := normalizeResponsesSubsequentRequest(raw, lastRequest, []byte(`[]`), false)
	if err != nil {
		t.Fatalf("normalizeResponsesSubsequentRequest() error = %v", err)
	}
	if got := gjson.GetBytes(normalized.request, "previous_response_id").String(); got != "resp-1" {
		t.Fatalf("previous_response_id = %q, want %q", got, "resp-1")
	}
	if got := gjson.GetBytes(normalized.request, "model").String(); got != "gpt-5-codex" {
		t.Fatalf("model = %q, want %q", got, "gpt-5-codex")
	}
	if !gjson.GetBytes(normalized.request, "stream").Bool() {
		t.Fatalf("stream should be true")
	}
}

func TestBuildResponsesUpstreamWebsocketHeadersEnsuresResponsesBeta(t *testing.T) {
	headers := buildResponsesUpstreamWebsocketHeaders(http.Header{
		"Originator": []string{"Codex Desktop"},
	}, &model.Channel{
		APIKey:      "sk-test",
		HeadersJSON: `{"X-Custom":"1"}`,
	})

	if got := headers.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := headers.Get("OpenAI-Beta"); got != "responses_websockets=2026-02-06" {
		t.Fatalf("OpenAI-Beta = %q", got)
	}
	if got := headers.Get("Originator"); got != "Codex Desktop" {
		t.Fatalf("Originator = %q", got)
	}
	if got := headers.Get("X-Custom"); got != "1" {
		t.Fatalf("X-Custom = %q", got)
	}
}

func TestPrepareResponsesWebsocketTurnReturnsServiceUnavailableWhenNoChannelAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeChannelRepo{
		channels: map[string]*model.Channel{},
		groups:   map[string][]string{},
	}

	originalService := responsesWebsocketChannelService
	responsesWebsocketChannelService = service.NewChannelServiceWithRepo(repo)
	defer func() { responsesWebsocketChannelService = originalService }()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","input":[]}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req.WithContext(WithProxyConfig(req.Context(), &ProxyConfig{}))

	prepared, errResp := prepareResponsesWebsocketTurn(c, &responsesWebsocketSession{}, []byte(`{"model":"gpt-5.4","input":[]}`), true)
	if prepared != nil {
		t.Fatal("expected no prepared turn when no channel is available")
	}
	if errResp == nil {
		t.Fatal("expected websocket error response")
	}
	if errResp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, errResp.StatusCode)
	}
	if errResp.Message != noAvailableChannelMessage {
		t.Fatalf("expected message %q, got %q", noAvailableChannelMessage, errResp.Message)
	}
}

func TestResponsesWebsocketProxyHandlerAcceptsLargeCreatePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := ResponsesWebsocketProxyHandler()
	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), &ProxyConfig{}))
		handler(c)
	})

	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, testResponsesWebsocketURL(server.URL), nil)
	if err != nil {
		t.Fatalf("websocket.Dial() error = %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	reqBody, err := json.Marshal(map[string]any{
		"type":         responsesWebsocketRequestCreate,
		"model":        "gpt-5.4",
		"generate":     false,
		"input":        []any{},
		"instructions": strings.Repeat("A", 40<<10),
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if len(reqBody) <= 32768 {
		t.Fatalf("expected test payload larger than 32KiB, got %d bytes", len(reqBody))
	}

	if err := conn.Write(ctx, websocket.MessageText, reqBody); err != nil {
		t.Fatalf("conn.Write() error = %v", err)
	}

	if got := readResponsesWebsocketEventType(t, conn); got != "response.created" {
		t.Fatalf("first event type = %q, want %q", got, "response.created")
	}
	if got := readResponsesWebsocketEventType(t, conn); got != "response.completed" {
		t.Fatalf("second event type = %q, want %q", got, "response.completed")
	}
}

func TestEnsureResponsesUpstreamConnReadsLargeMessages(t *testing.T) {
	upstreamDone := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket.Accept() error = %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, payload, err := conn.Read(ctx)
		if err != nil {
			t.Errorf("conn.Read() error = %v", err)
			return
		}
		if got := gjson.GetBytes(payload, "type").String(); got != responsesWebsocketRequestCreate {
			t.Errorf("request type = %q, want %q", got, responsesWebsocketRequestCreate)
			return
		}

		respBody, err := json.Marshal(map[string]any{
			"type":    "response.completed",
			"padding": strings.Repeat("B", 40<<10),
		})
		if err != nil {
			t.Errorf("json.Marshal() error = %v", err)
			return
		}

		if err := conn.Write(ctx, websocket.MessageText, respBody); err != nil {
			t.Errorf("conn.Write() error = %v", err)
			return
		}
		upstreamDone <- struct{}{}
	}))
	defer upstream.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := &responsesWebsocketSession{}
	channel := &model.Channel{
		ID:      "channel-1",
		BaseURL: upstream.URL,
		APIKey:  "sk-test",
	}

	conn, _, errResp := ensureResponsesUpstreamConn(ctx, http.Header{}, &ProxyConfig{}, session, channel)
	if errResp != nil {
		t.Fatalf("ensureResponsesUpstreamConn() error = %v", errResp)
	}
	defer closeResponsesUpstreamConn(session)

	reqBody := []byte(`{"type":"response.create","model":"gpt-5.4","input":[]}`)
	if err := conn.Write(ctx, websocket.MessageText, reqBody); err != nil {
		t.Fatalf("conn.Write() error = %v", err)
	}

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("conn.Read() error = %v", err)
	}
	if len(payload) <= 32768 {
		t.Fatalf("expected payload larger than 32KiB, got %d bytes", len(payload))
	}
	if got := gjson.GetBytes(payload, "type").String(); got != "response.completed" {
		t.Fatalf("response type = %q, want %q", got, "response.completed")
	}

	select {
	case <-upstreamDone:
	case <-ctx.Done():
		t.Fatal("timed out waiting for upstream websocket handler")
	}
}

func TestClassifyResponsesUpstreamReadError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: "ws_read_failed"},
		{name: "normal", err: websocket.CloseError{Code: websocket.StatusNormalClosure, Reason: "done"}, want: "ws_closed_before_response_completed"},
		{name: "going away", err: websocket.CloseError{Code: websocket.StatusGoingAway, Reason: "bye"}, want: "ws_closed_before_response_completed"},
		{name: "too big", err: websocket.CloseError{Code: websocket.StatusMessageTooBig, Reason: "too big"}, want: "ws_message_too_big"},
		{name: "policy", err: websocket.CloseError{Code: websocket.StatusPolicyViolation, Reason: "policy"}, want: "ws_policy_violation"},
		{name: "other", err: websocket.CloseError{Code: websocket.StatusUnsupportedData, Reason: "bad"}, want: "ws_read_failed_close_1003"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyResponsesUpstreamReadError(tt.err); got != tt.want {
				t.Fatalf("classifyResponsesUpstreamReadError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func readResponsesWebsocketEventType(t *testing.T, conn *websocket.Conn) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("conn.Read() error = %v", err)
	}
	return gjson.GetBytes(payload, "type").String()
}

func testResponsesWebsocketURL(serverURL string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http") + "/v1/responses"
}
