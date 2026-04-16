package amp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ampmanager/internal/billing"
	"ampmanager/internal/model"
	"ampmanager/internal/service"
	"ampmanager/internal/translator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"nhooyr.io/websocket"
)

const (
	responsesWebsocketRequestCreate = "response.create"
	responsesWebsocketRequestAppend = "response.append"
	responsesWebsocketTransportHTTP = "http"
	responsesWebsocketTransportWS   = "websocket"
	// Codex embeds tool schemas and turn metadata in response.create, which can
	// easily exceed nhooyr/websocket's default 32 KiB per-message read limit.
	responsesWebsocketReadLimit = 8 << 20
)

type responsesWebsocketSession struct {
	downstream          *websocket.Conn
	lastRequest         []byte
	lastCompletedOutput []byte
	pinnedChannelID     string
	upstreamConn        *websocket.Conn
	upstreamChannelID   string
}

type responsesNormalizedRequest struct {
	request      []byte
	stored       []byte
	rootCreate   bool
	localPrewarm bool
}

type responsesPreparedTurn struct {
	channel *model.Channel
	body    []byte
	trace   *RequestTrace
}

var (
	responsesWebsocketChannelService = service.NewChannelService()
	responsesWebsocketBillingService = service.NewBillingService()
)

func ResponsesWebsocketProxyHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		proxyCfg := GetProxyConfig(c.Request.Context())
		if proxyCfg == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, NewStandardError(http.StatusUnauthorized, "missing proxy configuration"))
			return
		}

		conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		setResponsesWebsocketReadLimit(conn)
		defer conn.Close(websocket.StatusInternalError, "")
		log.Infof("responses websocket: downstream connected")

		session := &responsesWebsocketSession{
			downstream:          conn,
			lastCompletedOutput: []byte("[]"),
		}
		defer closeResponsesUpstreamConn(session)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for {
			_, payload, errRead := conn.Read(ctx)
			if errRead != nil {
				closeStatus := websocket.CloseStatus(errRead)
				if closeStatus == websocket.StatusNormalClosure || closeStatus == websocket.StatusGoingAway {
					_ = conn.Close(websocket.StatusNormalClosure, "")
					return
				}
				log.Debugf("responses websocket: downstream read failed: %v", errRead)
				closeCode := websocket.StatusPolicyViolation
				if closeStatus == websocket.StatusMessageTooBig {
					closeCode = websocket.StatusMessageTooBig
				}
				_ = conn.Close(closeCode, "")
				return
			}

			normalized, errResp := normalizeResponsesWebsocketRequest(payload, session.lastRequest, session.lastCompletedOutput)
			if errResp != nil {
				if errWrite := writeResponsesWebsocketError(conn, errResp.StatusCode, errResp.Error()); errWrite != nil {
					return
				}
				continue
			}

			if normalized.rootCreate {
				closeResponsesUpstreamConn(session)
				session.lastRequest = nil
				session.lastCompletedOutput = []byte("[]")
				session.pinnedChannelID = ""
			}

			if normalized.localPrewarm {
				log.Infof("responses websocket: local prewarm request")
				if completedOutput, errWrite := writeResponsesWebsocketSyntheticPrewarm(conn, normalized.request); errWrite != nil {
					return
				} else {
					session.lastRequest = normalized.stored
					session.lastCompletedOutput = completedOutput
				}
				continue
			}

			prepared, errResp := prepareResponsesWebsocketTurn(c, session, normalized.request, normalized.rootCreate)
			if errResp != nil {
				if errWrite := writeResponsesWebsocketError(conn, errResp.StatusCode, errResp.Error()); errWrite != nil {
					return
				}
				continue
			}

			trace := prepared.trace
			log.Infof("responses websocket: turn channel=%s model=%s upstream=%s", prepared.channel.Name, trace.MappedModel, trace.UpstreamTransport)
			if writer := GetLogWriter(); writer != nil {
				writer.WritePendingFromTrace(trace)
			}
			StoreRequestDetail(trace.RequestID, sanitizeHeaders(c.Request.Header), normalized.request)

			responseHeaders, completedPayload, completedOutput, execErr := executeResponsesWebsocketTurn(ctx, c.Request.Header, proxyCfg, session, prepared)
			if execErr != nil {
				trace.SetError("upstream_error")
				trace.SetResponse(execErr.StatusCode)
				if writer := GetLogWriter(); writer != nil {
					if ok := writer.UpdateFromTrace(trace); !ok {
						if billingResult := trace.BillingResult(); billingResult != nil {
							billingSvc := service.NewBillingService()
							if err := billingSvc.ApplyBillingResult(trace.RequestID, billingResult); err != nil {
								log.Warnf("responses websocket: failed to apply billing fallback for request %s: %v", trace.RequestID, err)
							}
						}
					}
				}
				if errWrite := writeResponsesWebsocketError(conn, execErr.StatusCode, execErr.Error()); errWrite != nil {
					return
				}
				continue
			}

			trace.SetResponse(http.StatusOK)
			if assistantText := extractResponsesCompletedText(completedPayload); assistantText != "" {
				trace.SetResponseText(assistantText)
			}
			if usage, _, ok := (&openAIResponsesParser{}).ConsumeSSE("response.completed", completedPayload); ok && usage != nil {
				trace.SetUsage(usage.InputTokens, usage.OutputTokens, usage.CacheReadInputTokens, usage.CacheCreationInputTokens)
			}
			applyResponsesTraceCost(trace, c.Request.Context())
			if len(completedPayload) > 0 {
				StoreResponseDetail(trace.RequestID, sanitizeHeaders(responseHeaders), completedPayload)
			}
			if writer := GetLogWriter(); writer != nil {
				writer.UpdateFromTrace(trace)
			}

			session.lastRequest = normalized.stored
			session.lastCompletedOutput = completedOutput
			session.pinnedChannelID = prepared.channel.ID
		}
	}
}

type responsesWebsocketError struct {
	StatusCode int
	Message    string
}

func (e *responsesWebsocketError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func normalizeResponsesWebsocketRequest(rawJSON, lastRequest, lastCompletedOutput []byte) (*responsesNormalizedRequest, *responsesWebsocketError) {
	requestType := strings.TrimSpace(gjson.GetBytes(rawJSON, "type").String())
	switch requestType {
	case responsesWebsocketRequestCreate:
		if len(lastRequest) == 0 {
			return normalizeResponsesCreateRequest(rawJSON)
		}
		return normalizeResponsesSubsequentRequest(rawJSON, lastRequest, lastCompletedOutput, true)
	case responsesWebsocketRequestAppend:
		if len(lastRequest) == 0 {
			return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "response.append requires a previous response.create"}
		}
		return normalizeResponsesSubsequentRequest(rawJSON, lastRequest, lastCompletedOutput, false)
	default:
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "unsupported websocket request type"}
	}
}

func normalizeResponsesCreateRequest(rawJSON []byte) (*responsesNormalizedRequest, *responsesWebsocketError) {
	normalized, errDelete := sjson.DeleteBytes(rawJSON, "type")
	if errDelete != nil {
		normalized = bytes.Clone(rawJSON)
	}
	normalized, _ = sjson.SetBytes(normalized, "stream", true)
	if !gjson.GetBytes(normalized, "input").Exists() {
		normalized, _ = sjson.SetRawBytes(normalized, "input", []byte("[]"))
	}

	modelName := strings.TrimSpace(gjson.GetBytes(normalized, "model").String())
	if modelName == "" {
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "missing model in response.create"}
	}

	return &responsesNormalizedRequest{
		request:      normalized,
		stored:       bytes.Clone(normalized),
		rootCreate:   true,
		localPrewarm: gjson.GetBytes(rawJSON, "generate").Exists() && !gjson.GetBytes(rawJSON, "generate").Bool(),
	}, nil
}

func normalizeResponsesSubsequentRequest(rawJSON, lastRequest, lastCompletedOutput []byte, rootCreate bool) (*responsesNormalizedRequest, *responsesWebsocketError) {
	nextInput := gjson.GetBytes(rawJSON, "input")
	if !nextInput.Exists() || !nextInput.IsArray() {
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "websocket request requires array field input"}
	}

	if prev := strings.TrimSpace(gjson.GetBytes(rawJSON, "previous_response_id").String()); prev != "" {
		normalized, errDelete := sjson.DeleteBytes(rawJSON, "type")
		if errDelete != nil {
			normalized = bytes.Clone(rawJSON)
		}
		normalized, _ = sjson.SetBytes(normalized, "stream", true)
		if !gjson.GetBytes(normalized, "model").Exists() {
			if modelName := strings.TrimSpace(gjson.GetBytes(lastRequest, "model").String()); modelName != "" {
				normalized, _ = sjson.SetBytes(normalized, "model", modelName)
			}
		}
		if !gjson.GetBytes(normalized, "instructions").Exists() {
			if instructions := gjson.GetBytes(lastRequest, "instructions"); instructions.Exists() {
				normalized, _ = sjson.SetRawBytes(normalized, "instructions", []byte(instructions.Raw))
			}
		}
		return &responsesNormalizedRequest{
			request:    normalized,
			stored:     bytes.Clone(normalized),
			rootCreate: rootCreate,
		}, nil
	}

	normalized, errDelete := sjson.DeleteBytes(rawJSON, "type")
	if errDelete != nil {
		normalized = bytes.Clone(rawJSON)
	}
	normalized, _ = sjson.DeleteBytes(normalized, "previous_response_id")

	if !gjson.GetBytes(normalized, "model").Exists() {
		if modelName := strings.TrimSpace(gjson.GetBytes(lastRequest, "model").String()); modelName != "" {
			normalized, _ = sjson.SetBytes(normalized, "model", modelName)
		}
	}
	if !gjson.GetBytes(normalized, "instructions").Exists() {
		if instructions := gjson.GetBytes(lastRequest, "instructions"); instructions.Exists() {
			normalized, _ = sjson.SetRawBytes(normalized, "instructions", []byte(instructions.Raw))
		}
	}

	mergedInput, errMerge := mergeJSONArrayRaw(gjson.GetBytes(lastRequest, "input").Raw, normalizeJSONArrayRaw(lastCompletedOutput))
	if errMerge != nil {
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "invalid previous response output"}
	}
	mergedInput, errMerge = mergeJSONArrayRaw(mergedInput, nextInput.Raw)
	if errMerge != nil {
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "invalid request input"}
	}
	if deduped, errDedupe := dedupeFunctionCallsByCallID(mergedInput); errDedupe == nil {
		mergedInput = deduped
	}

	normalized, _ = sjson.SetRawBytes(normalized, "input", []byte(mergedInput))
	normalized, _ = sjson.SetBytes(normalized, "stream", true)

	return &responsesNormalizedRequest{
		request:    normalized,
		stored:     bytes.Clone(normalized),
		rootCreate: rootCreate,
	}, nil
}

func prepareResponsesWebsocketTurn(c *gin.Context, session *responsesWebsocketSession, normalized []byte, rootCreate bool) (*responsesPreparedTurn, *responsesWebsocketError) {
	proxyCfg := GetProxyConfig(c.Request.Context())
	if proxyCfg == nil {
		return nil, &responsesWebsocketError{StatusCode: http.StatusUnauthorized, Message: "missing proxy configuration"}
	}

	if proxyCfg.NativeMode {
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "responses websocket is not available in native mode"}
	}

	if resolveGroupMultiplier(proxyCfg) != 0 {
		canStart, err := responsesWebsocketBillingService.CanStartRequest(proxyCfg.UserID)
		if err != nil {
			return nil, &responsesWebsocketError{StatusCode: http.StatusInternalServerError, Message: "billing check failed"}
		}
		if !canStart {
			return nil, &responsesWebsocketError{StatusCode: http.StatusForbidden, Message: "余额和订阅额度均不足，请充值后再使用"}
		}
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(normalized, &payload); err != nil {
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "invalid request body"}
	}

	modelName, _ := payload["model"].(string)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "missing model"}
	}

	var mappings []model.ModelMapping
	if proxyCfg.ModelMappingsJSON != "" {
		_ = json.Unmarshal([]byte(proxyCfg.ModelMappingsJSON), &mappings)
	}
	result := applyMappingWithHeaders(modelName, mappings, c.GetHeader)
	if result.Applied {
		payload["model"] = result.MappedModel
		if result.ThinkingLevel != "" {
			applyThinkingLevelWithPath(payload, result.ThinkingLevel, "/v1/responses")
		}
		if result.FastMode {
			payload["service_tier"] = "priority"
		}
		if result.CustomInstructionsEnabled && result.CustomInstructions != "" {
			applyCustomInstructions(payload, result.CustomInstructions, "/v1/responses")
		}
	} else {
		result = MappingResult{
			OriginalModel: modelName,
			MappedModel:   modelName,
		}
	}

	body, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "failed to normalize request"}
	}
	continueWithPinned := !rootCreate || strings.TrimSpace(gjson.GetBytes(normalized, "previous_response_id").String()) != ""
	channel, errSelect := selectResponsesWebsocketChannel(session, proxyCfg, result, continueWithPinned)
	if errSelect != nil {
		return nil, errSelect
	}
	if channel == nil {
		return nil, &responsesWebsocketError{StatusCode: http.StatusServiceUnavailable, Message: noAvailableChannelMessage}
	}
	if channel.Type != model.ChannelTypeOpenAI || channel.Endpoint != model.ChannelEndpointResponses {
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadRequest, Message: "responses websocket requires an OpenAI responses channel"}
	}

	trace := NewRequestTrace(uuid.New().String(), proxyCfg.UserID, proxyCfg.APIKeyID, "WEBSOCKET", "/v1/responses")
	trace.SetChannel(channel.ID, string(channel.Type), string(channel.Endpoint))
	trace.SetFormatConversion(translator.FormatOpenAIResponses.String(), translator.FormatOpenAIResponses.String())
	trace.SetModels(result.OriginalModel, result.MappedModel)
	trace.SetStreaming(true)
	trace.SetDownstreamTransport(responsesWebsocketTransportWS)
	breakdown := computePricingBreakdown(c.Request.Context(), channel, body)
	trace.SetPricingBreakdown(breakdown.GroupMultiplier, breakdown.ChannelMultiplier, breakdown.SpecialMultiplier, breakdown.SpecialReason)
	if channel.CodexWebsocketEnabled {
		trace.SetUpstreamTransport(responsesWebsocketTransportWS)
	} else {
		trace.SetUpstreamTransport(responsesWebsocketTransportHTTP)
	}
	if result.ThinkingLevel != "" {
		trace.SetThinkingLevel(result.ThinkingLevel)
	}

	return &responsesPreparedTurn{
		channel: channel,
		body:    body,
		trace:   trace,
	}, nil
}

func selectResponsesWebsocketChannel(session *responsesWebsocketSession, proxyCfg *ProxyConfig, result MappingResult, continueWithPinned bool) (*model.Channel, *responsesWebsocketError) {
	incomingFormat := translator.FormatOpenAIResponses
	if continueWithPinned && session.pinnedChannelID != "" {
		channel, err := responsesWebsocketChannelService.SelectSpecificChannelForModelWithGroupsAndFormat(session.pinnedChannelID, result.MappedModel, proxyCfg.GroupIDs, incomingFormat, false)
		if err != nil {
			return nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "failed to resolve pinned channel"}
		}
		return channel, nil
	}

	if result.PreferredChannelID != "" {
		channel, err := responsesWebsocketChannelService.SelectSpecificChannelForModelWithGroupsAndFormat(result.PreferredChannelID, result.MappedModel, proxyCfg.GroupIDs, incomingFormat, false)
		if err != nil {
			return nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "failed to resolve preferred channel"}
		}
		return channel, nil
	}

	var (
		channel *model.Channel
		err     error
	)
	if len(proxyCfg.GroupIDs) > 0 {
		channel, err = responsesWebsocketChannelService.SelectChannelForModelWithGroupsAndFormat(result.MappedModel, proxyCfg.GroupIDs, incomingFormat, false)
	} else {
		channel, err = responsesWebsocketChannelService.SelectChannelForModelAndFormat(result.MappedModel, incomingFormat, false)
	}
	if err != nil {
		return nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "failed to select channel"}
	}
	return channel, nil
}

func executeResponsesWebsocketTurn(ctx context.Context, clientHeaders http.Header, proxyCfg *ProxyConfig, session *responsesWebsocketSession, prepared *responsesPreparedTurn) (http.Header, []byte, []byte, *responsesWebsocketError) {
	if prepared.channel.CodexWebsocketEnabled {
		log.Infof("responses websocket: attempting upstream websocket channel=%s", prepared.channel.ID)
		headers, completedPayload, completedOutput, errExec := executeResponsesOverUpstreamWebsocket(ctx, clientHeaders, proxyCfg, session, prepared)
		if errExec == nil {
			return headers, completedPayload, completedOutput, nil
		}

		log.Warnf("responses websocket: falling back to HTTP for channel %s: %v", prepared.channel.ID, errExec)
		prepared.trace.SetTransportFallbackReason(errExec.Message)
		prepared.trace.SetUpstreamTransport(responsesWebsocketTransportHTTP)
		closeResponsesUpstreamConn(session)
	}

	return executeResponsesOverHTTPFallback(ctx, clientHeaders, session, prepared)
}

func executeResponsesOverUpstreamWebsocket(ctx context.Context, clientHeaders http.Header, proxyCfg *ProxyConfig, session *responsesWebsocketSession, prepared *responsesPreparedTurn) (http.Header, []byte, []byte, *responsesWebsocketError) {
	conn, handshakeHeaders, errConn := ensureResponsesUpstreamConn(ctx, clientHeaders, proxyCfg, session, prepared.channel)
	if errConn != nil {
		return nil, nil, nil, errConn
	}

	requestPayload, errSet := sjson.SetBytes(bytes.Clone(prepared.body), "type", responsesWebsocketRequestCreate)
	if errSet != nil {
		requestPayload = bytes.Clone(prepared.body)
	}
	if errWrite := conn.Write(ctx, websocket.MessageText, requestPayload); errWrite != nil {
		return nil, nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "ws_write_failed"}
	}

	var (
		firstForwarded   bool
		completedPayload []byte
		completedOutput  = []byte("[]")
	)
	for {
		_, payload, errRead := conn.Read(ctx)
		if errRead != nil {
			message := classifyResponsesUpstreamReadError(errRead)
			log.Warnf("responses websocket: upstream read failed channel=%s status=%d err=%v", prepared.channel.ID, websocket.CloseStatus(errRead), errRead)
			return nil, nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: message}
		}

		payload = normalizeResponsesCompletedEvent(bytes.TrimSpace(payload))
		if len(payload) == 0 {
			continue
		}

		if payloadType := strings.TrimSpace(gjson.GetBytes(payload, "type").String()); payloadType == "error" {
			return nil, nil, nil, parseResponsesWebsocketUpstreamError(payload)
		}

		if !firstForwarded {
			prepared.trace.MarkFirstByte()
			firstForwarded = true
		}
		if errWrite := session.downstream.Write(ctx, websocket.MessageText, payload); errWrite != nil {
			return nil, nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "downstream_write_failed"}
		}

		payloadType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
		if payloadType == "response.completed" {
			completedPayload = payload
			completedOutput = extractResponsesCompletedOutput(payload)
			return handshakeHeaders, completedPayload, completedOutput, nil
		}
	}
}

func executeResponsesOverHTTPFallback(ctx context.Context, clientHeaders http.Header, session *responsesWebsocketSession, prepared *responsesPreparedTurn) (http.Header, []byte, []byte, *responsesWebsocketError) {
	log.Infof("responses websocket: using HTTP fallback channel=%s", prepared.channel.ID)
	targetURL, errURL := buildResponsesHTTPURL(prepared.channel)
	if errURL != nil {
		return nil, nil, nil, &responsesWebsocketError{StatusCode: http.StatusInternalServerError, Message: "failed to build upstream URL"}
	}

	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(prepared.body))
	if errReq != nil {
		return nil, nil, nil, &responsesWebsocketError{StatusCode: http.StatusInternalServerError, Message: "failed to create fallback request"}
	}
	req.Header.Set("Content-Type", "application/json")
	applyChannelAuth(prepared.channel, req)
	applyResponsesHTTPHeaders(req, clientHeaders, prepared.channel)

	resp, errDo := sharedChannelTransport.RoundTrip(req)
	if errDo != nil {
		return nil, nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "http_fallback_failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return resp.Header.Clone(), nil, nil, &responsesWebsocketError{StatusCode: resp.StatusCode, Message: message}
	}

	return forwardResponsesHTTPBody(ctx, session, prepared, resp)
}

func forwardResponsesHTTPBody(ctx context.Context, session *responsesWebsocketSession, prepared *responsesPreparedTurn, resp *http.Response) (http.Header, []byte, []byte, *responsesWebsocketError) {
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		completedPayload, completedOutput, errStream := streamResponsesSSEPayloads(ctx, session.downstream, prepared.trace, resp.Body)
		if errStream != nil {
			return resp.Header.Clone(), nil, nil, errStream
		}
		return resp.Header.Clone(), completedPayload, completedOutput, nil
	}

	firstByte := make([]byte, 1)
	if _, errRead := resp.Body.Read(firstByte); errRead != nil {
		if errors.Is(errRead, io.EOF) {
			return resp.Header.Clone(), nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "empty fallback response"}
		}
		return resp.Header.Clone(), nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "failed to read fallback response"}
	}
	prepared.trace.MarkFirstByte()
	body, errRead := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if errRead != nil {
		return resp.Header.Clone(), nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "failed to read fallback response"}
	}
	payload := bytes.TrimSpace(append(firstByte, body...))
	if gjson.GetBytes(payload, "object").String() == "response" {
		completed := []byte(`{"type":"response.completed","response":{}}`)
		completed, _ = sjson.SetRawBytes(completed, "response", payload)
		if errWrite := session.downstream.Write(ctx, websocket.MessageText, completed); errWrite != nil {
			return resp.Header.Clone(), nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "downstream_write_failed"}
		}
		return resp.Header.Clone(), completed, extractResponsesCompletedOutput(completed), nil
	}

	return resp.Header.Clone(), nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "fallback response did not contain a response object"}
}

func streamResponsesSSEPayloads(ctx context.Context, downstream *websocket.Conn, trace *RequestTrace, reader io.Reader) ([]byte, []byte, *responsesWebsocketError) {
	var (
		completedPayload []byte
		completedOutput  = []byte("[]")
		forwardedAny     bool
		buffer           []byte
		chunk            = make([]byte, 4096)
	)
	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			if !forwardedAny {
				trace.MarkFirstByte()
				forwardedAny = true
			}
			buffer = append(buffer, chunk[:n]...)
			for {
				idx, delimLen := findSSEDelimiter(buffer)
				if idx < 0 {
					break
				}
				frame := append([]byte(nil), buffer[:idx+delimLen]...)
				buffer = buffer[idx+delimLen:]
				eventName, payload, done := parseSSEEvent(frame)
				if done || len(payload) == 0 {
					continue
				}
				payload = normalizeResponsesCompletedEvent(payload)
				if errWrite := downstream.Write(ctx, websocket.MessageText, payload); errWrite != nil {
					return nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "downstream_write_failed"}
				}
				if eventName == "response.completed" || gjson.GetBytes(payload, "type").String() == "response.completed" {
					completedPayload = payload
					completedOutput = extractResponsesCompletedOutput(payload)
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "failed to read fallback stream"}
		}
	}
	if len(completedPayload) == 0 {
		return nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "fallback stream ended without response.completed"}
	}
	return completedPayload, completedOutput, nil
}

func forwardResponsesSSEPayloads(ctx context.Context, downstream *websocket.Conn, trace *RequestTrace, body []byte) ([]byte, []byte, *responsesWebsocketError) {
	var (
		completedPayload []byte
		completedOutput  = []byte("[]")
		forwardedAny     bool
	)
	remaining := body
	for len(remaining) > 0 {
		idx, delimLen := findSSEDelimiter(remaining)
		if idx < 0 {
			break
		}
		frame := remaining[:idx+delimLen]
		remaining = remaining[idx+delimLen:]
		eventName, payload, done := parseSSEEvent(frame)
		if done || len(payload) == 0 {
			continue
		}
		payload = normalizeResponsesCompletedEvent(payload)
		if !forwardedAny {
			trace.MarkFirstByte()
			forwardedAny = true
		}
		if errWrite := downstream.Write(ctx, websocket.MessageText, payload); errWrite != nil {
			return nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "downstream_write_failed"}
		}
		if eventName == "response.completed" || gjson.GetBytes(payload, "type").String() == "response.completed" {
			completedPayload = payload
			completedOutput = extractResponsesCompletedOutput(payload)
		}
	}
	if len(completedPayload) == 0 {
		return nil, nil, &responsesWebsocketError{StatusCode: http.StatusBadGateway, Message: "fallback stream ended without response.completed"}
	}
	return completedPayload, completedOutput, nil
}

func applyResponsesTraceCost(trace *RequestTrace, ctx context.Context) {
	if trace == nil {
		return
	}
	calc := billing.GetCostCalculator()
	if calc == nil {
		return
	}
	pricingModel := trace.MappedModel
	if pricingModel == "" {
		pricingModel = trace.OriginalModel
	}
	if pricingModel == "" {
		return
	}

	costResult := calc.CalculateFromPointers(
		pricingModel,
		trace.InputTokens,
		trace.OutputTokens,
		trace.CacheReadInputTokens,
		trace.CacheCreationInputTokens,
	)
	if !costResult.PriceFound {
		return
	}

	proxyCfg := GetProxyConfig(ctx)
	multiplier := traceMultiplier(trace)

	if multiplier == 0 {
		trace.SetCost(costResult.CostMicros, costResult.CostUsd, costResult.PricingModel, costResult.PricingRuleName)
		return
	}

	adjustedCostMicros := int64(float64(costResult.CostMicros) * multiplier)
	adjustedCostUsd := fmt.Sprintf("%.6f", float64(adjustedCostMicros)/1e6)
	trace.SetCost(adjustedCostMicros, adjustedCostUsd, costResult.PricingModel, costResult.PricingRuleName)

	if proxyCfg != nil && adjustedCostMicros > 0 {
		billingSvc := service.NewBillingService()
		result, err := billingSvc.SettleRequestCostResult(trace.RequestID, proxyCfg.UserID, adjustedCostMicros)
		if err != nil {
			log.Warnf("responses websocket: failed to settle cost for user %s: %v", proxyCfg.UserID, err)
		} else {
			trace.SetBillingResult(result)
		}
	}
}

func ensureResponsesUpstreamConn(ctx context.Context, clientHeaders http.Header, proxyCfg *ProxyConfig, session *responsesWebsocketSession, channel *model.Channel) (*websocket.Conn, http.Header, *responsesWebsocketError) {
	if session.upstreamConn != nil && session.upstreamChannelID == channel.ID {
		return session.upstreamConn, nil, nil
	}

	closeResponsesUpstreamConn(session)

	wsURL, errURL := buildResponsesWebsocketURL(channel)
	if errURL != nil {
		return nil, nil, &responsesWebsocketError{StatusCode: http.StatusInternalServerError, Message: "failed to build websocket URL"}
	}
	headers := buildResponsesUpstreamWebsocketHeaders(clientHeaders, channel)
	client := responsesWebsocketHTTPClient(proxyCfg)
	conn, resp, errDial := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient: client,
		HTTPHeader: headers,
	})
	if errDial != nil {
		statusCode := http.StatusBadGateway
		message := "ws_upgrade_failed"
		if resp != nil && resp.StatusCode == http.StatusUpgradeRequired {
			message = "ws_upgrade_rejected"
		}
		return nil, nil, &responsesWebsocketError{StatusCode: statusCode, Message: message}
	}
	setResponsesWebsocketReadLimit(conn)

	session.upstreamConn = conn
	session.upstreamChannelID = channel.ID
	if resp != nil {
		return conn, resp.Header.Clone(), nil
	}
	return conn, http.Header{}, nil
}

func closeResponsesUpstreamConn(session *responsesWebsocketSession) {
	if session.upstreamConn != nil {
		_ = session.upstreamConn.Close(websocket.StatusNormalClosure, "")
	}
	session.upstreamConn = nil
	session.upstreamChannelID = ""
}

func buildResponsesWebsocketURL(channel *model.Channel) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(channel.BaseURL))
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/v1/responses"
	return parsed.String(), nil
}

func buildResponsesHTTPURL(channel *model.Channel) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(channel.BaseURL))
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/v1/responses"
	return parsed.String(), nil
}

func buildResponsesUpstreamWebsocketHeaders(clientHeaders http.Header, channel *model.Channel) http.Header {
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+channel.APIKey)
	headers.Set("OpenAI-Beta", "responses_websockets=2026-02-06")

	passThrough := []string{
		"Originator",
		"Version",
		"OpenAI-Beta",
		"X-Codex-Turn-State",
		"X-Codex-Turn-Metadata",
		"X-Client-Request-Id",
		"X-Codex-Beta-Features",
	}
	for _, key := range passThrough {
		if value := strings.TrimSpace(clientHeaders.Get(key)); value != "" {
			headers.Set(key, value)
		}
	}

	var customHeaders map[string]string
	if err := json.Unmarshal([]byte(channel.HeadersJSON), &customHeaders); err == nil {
		for key, value := range customHeaders {
			if strings.TrimSpace(key) == "" {
				continue
			}
			headers.Set(key, value)
		}
	}
	if beta := strings.TrimSpace(headers.Get("OpenAI-Beta")); beta == "" || !strings.Contains(beta, "responses_websockets=") {
		headers.Set("OpenAI-Beta", "responses_websockets=2026-02-06")
	}
	return headers
}

func applyResponsesHTTPHeaders(req *http.Request, clientHeaders http.Header, channel *model.Channel) {
	if req == nil {
		return
	}
	req.Header.Set("User-Agent", "codex_exec/0.98.0 (Mac OS 15.1.0; arm64) unknown")
	for _, key := range []string{"Originator", "Version", "X-Codex-Turn-State", "X-Codex-Turn-Metadata", "X-Client-Request-Id"} {
		if value := strings.TrimSpace(clientHeaders.Get(key)); value != "" {
			req.Header.Set(key, value)
		}
	}
	var customHeaders map[string]string
	if err := json.Unmarshal([]byte(channel.HeadersJSON), &customHeaders); err == nil {
		for key, value := range customHeaders {
			if strings.TrimSpace(key) != "" {
				req.Header.Set(key, value)
			}
		}
	}
}

func responsesWebsocketHTTPClient(proxyCfg *ProxyConfig) *http.Client {
	transport := http.RoundTripper(NewStreamingTransport())
	if proxyCfg != nil && strings.TrimSpace(proxyCfg.Socks5Proxy) != "" {
		if socksTransport, err := getSocks5Transport(proxyCfg.Socks5Proxy); err == nil {
			transport = socksTransport
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   0,
	}
}

func setResponsesWebsocketReadLimit(conn *websocket.Conn) {
	if conn == nil {
		return
	}
	conn.SetReadLimit(responsesWebsocketReadLimit)
}

func classifyResponsesUpstreamReadError(err error) string {
	if err == nil {
		return "ws_read_failed"
	}
	switch websocket.CloseStatus(err) {
	case websocket.StatusNormalClosure, websocket.StatusGoingAway:
		return "ws_closed_before_response_completed"
	case websocket.StatusMessageTooBig:
		return "ws_message_too_big"
	case websocket.StatusPolicyViolation:
		return "ws_policy_violation"
	case -1:
		return "ws_read_failed"
	default:
		return fmt.Sprintf("ws_read_failed_close_%d", websocket.CloseStatus(err))
	}
}

func normalizeResponsesCompletedEvent(payload []byte) []byte {
	if strings.TrimSpace(gjson.GetBytes(payload, "type").String()) == "response.done" {
		if updated, err := sjson.SetBytes(payload, "type", "response.completed"); err == nil {
			return updated
		}
	}
	return payload
}

func extractResponsesCompletedOutput(payload []byte) []byte {
	output := gjson.GetBytes(payload, "response.output")
	if output.Exists() {
		return []byte(output.Raw)
	}
	output = gjson.GetBytes(payload, "output")
	if output.Exists() {
		return []byte(output.Raw)
	}
	return []byte("[]")
}

func extractResponsesCompletedText(payload []byte) string {
	response := gjson.GetBytes(payload, "response")
	if response.Exists() && response.IsObject() {
		return ExtractOpenAIResponsesOutputText([]byte(response.Raw))
	}
	return ExtractOpenAIResponsesOutputText(payload)
}

func parseResponsesWebsocketUpstreamError(payload []byte) *responsesWebsocketError {
	status := int(gjson.GetBytes(payload, "status").Int())
	if status <= 0 {
		status = int(gjson.GetBytes(payload, "status_code").Int())
	}
	if status <= 0 {
		status = http.StatusBadGateway
	}
	message := strings.TrimSpace(gjson.GetBytes(payload, "error.message").String())
	if message == "" {
		message = strings.TrimSpace(gjson.GetBytes(payload, "message").String())
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return &responsesWebsocketError{StatusCode: status, Message: message}
}

func writeResponsesWebsocketError(conn *websocket.Conn, statusCode int, message string) error {
	if conn == nil {
		return nil
	}
	payload := []byte(`{"type":"error","status":0,"error":{"type":"invalid_request_error","message":""}}`)
	payload, _ = sjson.SetBytes(payload, "status", statusCode)
	payload, _ = sjson.SetBytes(payload, "error.message", message)
	if statusCode >= 500 {
		payload, _ = sjson.SetBytes(payload, "error.type", "server_error")
	}
	return conn.Write(context.Background(), websocket.MessageText, payload)
}

func writeResponsesWebsocketSyntheticPrewarm(conn *websocket.Conn, requestJSON []byte) ([]byte, error) {
	responseID := "resp_prewarm_" + uuid.NewString()
	createdAt := time.Now().Unix()
	modelName := strings.TrimSpace(gjson.GetBytes(requestJSON, "model").String())

	createdPayload := []byte(`{"type":"response.created","response":{"id":"","object":"response","created_at":0,"status":"in_progress","background":false,"error":null,"output":[]}}`)
	createdPayload, _ = sjson.SetBytes(createdPayload, "response.id", responseID)
	createdPayload, _ = sjson.SetBytes(createdPayload, "response.created_at", createdAt)
	if modelName != "" {
		createdPayload, _ = sjson.SetBytes(createdPayload, "response.model", modelName)
	}

	completedPayload := []byte(`{"type":"response.completed","response":{"id":"","object":"response","created_at":0,"status":"completed","background":false,"error":null,"output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`)
	completedPayload, _ = sjson.SetBytes(completedPayload, "response.id", responseID)
	completedPayload, _ = sjson.SetBytes(completedPayload, "response.created_at", createdAt)
	if modelName != "" {
		completedPayload, _ = sjson.SetBytes(completedPayload, "response.model", modelName)
	}

	if err := conn.Write(context.Background(), websocket.MessageText, createdPayload); err != nil {
		return nil, err
	}
	if err := conn.Write(context.Background(), websocket.MessageText, completedPayload); err != nil {
		return nil, err
	}
	return []byte("[]"), nil
}

func mergeJSONArrayRaw(existingRaw, appendRaw string) (string, error) {
	existingRaw = strings.TrimSpace(existingRaw)
	appendRaw = strings.TrimSpace(appendRaw)
	if existingRaw == "" {
		existingRaw = "[]"
	}
	if appendRaw == "" {
		appendRaw = "[]"
	}

	var existing []json.RawMessage
	if err := json.Unmarshal([]byte(existingRaw), &existing); err != nil {
		return "", err
	}
	var appendItems []json.RawMessage
	if err := json.Unmarshal([]byte(appendRaw), &appendItems); err != nil {
		return "", err
	}

	merged := append(existing, appendItems...)
	out, err := json.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func normalizeJSONArrayRaw(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "[]"
	}
	result := gjson.Parse(trimmed)
	if result.Type == gjson.JSON && result.IsArray() {
		return trimmed
	}
	return "[]"
}

func dedupeFunctionCallsByCallID(rawArray string) (string, error) {
	rawArray = strings.TrimSpace(rawArray)
	if rawArray == "" {
		return "[]", nil
	}
	var items []json.RawMessage
	if errUnmarshal := json.Unmarshal([]byte(rawArray), &items); errUnmarshal != nil {
		return "", errUnmarshal
	}

	seenCallIDs := make(map[string]struct{}, len(items))
	filtered := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		itemType := strings.TrimSpace(gjson.GetBytes(item, "type").String())
		if itemType == "function_call" || itemType == "custom_tool_call" {
			callID := strings.TrimSpace(gjson.GetBytes(item, "call_id").String())
			if callID != "" {
				if _, ok := seenCallIDs[callID]; ok {
					continue
				}
				seenCallIDs[callID] = struct{}{}
			}
		}
		filtered = append(filtered, item)
	}

	out, errMarshal := json.Marshal(filtered)
	if errMarshal != nil {
		return "", errMarshal
	}
	return string(out), nil
}
