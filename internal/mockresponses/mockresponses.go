package mockresponses

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

var (
	supportedModels = []string{
		"gpt-5.4",
		"gpt-5.3-codex",
		"gpt-5.2",
		"gpt-5.4-mini",
	}
	supportedModelSet = map[string]struct{}{
		"gpt-5.4":       {},
		"gpt-5.3-codex": {},
		"gpt-5.2":       {},
		"gpt-5.4-mini":  {},
	}
	responseCounter uint64
	randCounter     uint64
)

type SimulationConfig struct {
	DefaultModel       string
	DefaultInputTokens int
	ChunkInterval      time.Duration
	TTFBMin            time.Duration
	TTFBMainMax        time.Duration
	TTFBTailMax        time.Duration
	TPSMin             float64
	TPSMax             float64
	OutputMin          time.Duration
	OutputMax          time.Duration
}

type RequestEnvelope struct {
	RequestType        string
	Model              string
	Stream             bool
	Generate           bool
	PreviousResponseID string
	InputTokens        int
}

type ResponseProfile struct {
	Request         RequestEnvelope
	Model           string
	InputTokens     int
	OutputTokens    int
	CachedTokens    int
	TTFB            time.Duration
	OutputDuration  time.Duration
	ChunkInterval   time.Duration
	ChunkCount      int
	TokensPerSecond float64
	ResponseID      string
	CreatedAt       time.Time
	OutputText      string
}

func DefaultSimulationConfig() SimulationConfig {
	return SimulationConfig{
		DefaultModel:       supportedModels[0],
		DefaultInputTokens: 128,
		ChunkInterval:      250 * time.Millisecond,
		TTFBMin:            1500 * time.Millisecond,
		TTFBMainMax:        6 * time.Second,
		TTFBTailMax:        7 * time.Second,
		TPSMin:             30,
		TPSMax:             60,
		OutputMin:          10 * time.Second,
		OutputMax:          200 * time.Second,
	}
}

func SupportedModels() []string {
	return slices.Clone(supportedModels)
}

func IsSupportedModel(modelName string) bool {
	_, ok := supportedModelSet[strings.TrimSpace(modelName)]
	return ok
}

func ParseRequest(body []byte, cfg SimulationConfig) (RequestEnvelope, error) {
	type requestPayload struct {
		Type               string          `json:"type"`
		Model              string          `json:"model"`
		Stream             bool            `json:"stream"`
		Generate           *bool           `json:"generate"`
		PreviousResponseID string          `json:"previous_response_id"`
		Input              json.RawMessage `json:"input"`
	}

	var payload requestPayload
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return RequestEnvelope{}, fmt.Errorf("invalid json body: %w", err)
		}
	}

	modelName := strings.TrimSpace(payload.Model)
	if modelName == "" {
		modelName = cfg.DefaultModel
	}
	if !IsSupportedModel(modelName) {
		return RequestEnvelope{}, fmt.Errorf("unsupported model %q", modelName)
	}

	generate := true
	if payload.Generate != nil {
		generate = *payload.Generate
	}

	return RequestEnvelope{
		RequestType:        strings.TrimSpace(payload.Type),
		Model:              modelName,
		Stream:             payload.Stream,
		Generate:           generate,
		PreviousResponseID: strings.TrimSpace(payload.PreviousResponseID),
		InputTokens:        estimateInputTokens(payload.Input, cfg.DefaultInputTokens),
	}, nil
}

func NewAutoProfile(body []byte, cfg SimulationConfig, rng *rand.Rand) (ResponseProfile, error) {
	request, err := ParseRequest(body, cfg)
	if err != nil {
		return ResponseProfile{}, err
	}
	if rng == nil {
		rng = NewRand()
	}

	ttfb := sampleTTFB(cfg, rng)
	if !request.Generate {
		ttfb = 75 * time.Millisecond
	}

	outputDuration := sampleOutputDuration(cfg, request.InputTokens)
	tps := sampleTPS(cfg, rng)
	outputTokens := int(math.Round(tps * outputDuration.Seconds()))
	if !request.Generate {
		outputDuration = 0
		outputTokens = 0
		tps = 0
	}

	profile := ResponseProfile{
		Request:         request,
		Model:           request.Model,
		InputTokens:     request.InputTokens,
		OutputTokens:    outputTokens,
		CachedTokens:    sampleCachedTokens(rng, request.InputTokens),
		TTFB:            ttfb,
		OutputDuration:  outputDuration,
		ChunkInterval:   cfg.ChunkInterval,
		ChunkCount:      deriveChunkCount(outputDuration, cfg.ChunkInterval),
		TokensPerSecond: tps,
		ResponseID:      nextResponseID(),
		CreatedAt:       time.Now().UTC(),
	}
	profile.OutputText = BuildRandomTokenText(rng, profile.OutputTokens)
	return profile, nil
}

func NewFixedProfile(modelName string, inputTokens, outputTokens, cachedTokens int, ttfb, chunkInterval time.Duration) ResponseProfile {
	if strings.TrimSpace(modelName) == "" {
		modelName = supportedModels[0]
	}
	if chunkInterval <= 0 {
		chunkInterval = time.Millisecond
	}
	return ResponseProfile{
		Request: RequestEnvelope{
			Model:    modelName,
			Generate: true,
		},
		Model:         modelName,
		InputTokens:   maxInt(inputTokens, 1),
		OutputTokens:  maxInt(outputTokens, 1),
		CachedTokens:  maxInt(cachedTokens, 0),
		TTFB:          ttfb,
		ChunkInterval: chunkInterval,
		ChunkCount:    3,
		ResponseID:    nextResponseID(),
		CreatedAt:     time.Now().UTC(),
	}
}

func NewRand() *rand.Rand {
	seed := time.Now().UnixNano() + int64(atomic.AddUint64(&randCounter, 1))
	return rand.New(rand.NewSource(seed))
}

func WriteJSONResponse(w http.ResponseWriter, profile ResponseProfile, gzipBody bool) error {
	payload := BuildResponseObject(profile)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if gzipBody {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(encoded); err != nil {
			_ = zw.Close()
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(buf.Bytes())
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(encoded)
	return err
}

func StreamSSE(ctx context.Context, w http.ResponseWriter, profile ResponseProfile) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("stream unsupported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	createdPayload, err := json.Marshal(buildCreatedEvent(profile))
	if err != nil {
		return err
	}
	if err := writeSSEFrame(w, flusher, "response.created", createdPayload); err != nil {
		return err
	}

	chunks := splitText(profile.outputText(), maxInt(profile.ChunkCount, 1))
	for _, chunk := range chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		payload, err := json.Marshal(map[string]any{
			"type":          "response.output_text.delta",
			"response_id":   profile.ResponseID,
			"output_index":  0,
			"content_index": 0,
			"delta":         chunk,
		})
		if err != nil {
			return err
		}
		if err := writeSSEFrame(w, flusher, "response.output_text.delta", payload); err != nil {
			return err
		}
		if profile.ChunkInterval > 0 {
			if err := SleepContext(ctx, profile.ChunkInterval); err != nil {
				return err
			}
		}
	}

	completedPayload, err := json.Marshal(map[string]any{
		"type":     "response.completed",
		"response": BuildResponseObject(profile),
	})
	if err != nil {
		return err
	}
	if err := writeSSEFrame(w, flusher, "response.completed", completedPayload); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func StreamWebsocket(ctx context.Context, conn *websocket.Conn, profile ResponseProfile) error {
	if err := writeWebsocketJSON(ctx, conn, buildCreatedEvent(profile)); err != nil {
		return err
	}

	chunks := splitText(profile.outputText(), maxInt(profile.ChunkCount, 1))
	for _, chunk := range chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := writeWebsocketJSON(ctx, conn, map[string]any{
			"type":          "response.output_text.delta",
			"response_id":   profile.ResponseID,
			"output_index":  0,
			"content_index": 0,
			"delta":         chunk,
		}); err != nil {
			return err
		}
		if profile.ChunkInterval > 0 {
			if err := SleepContext(ctx, profile.ChunkInterval); err != nil {
				return err
			}
		}
	}

	return writeWebsocketJSON(ctx, conn, map[string]any{
		"type":     "response.completed",
		"response": BuildResponseObject(profile),
	})
}

func WriteHTTPError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(httpErrorPayload(statusCode, message)))
}

func WriteWebsocketError(ctx context.Context, conn *websocket.Conn, statusCode int, message string) error {
	return conn.Write(ctx, websocket.MessageText, []byte(httpErrorPayload(statusCode, message)))
}

func BuildResponseObject(profile ResponseProfile) map[string]any {
	outputText := profile.outputText()
	return map[string]any{
		"id":         profile.ResponseID,
		"object":     "response",
		"created_at": profile.CreatedAt.Unix(),
		"model":      profile.Model,
		"status":     "completed",
		"output": []map[string]any{
			{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{
					{
						"type": "output_text",
						"text": outputText,
					},
				},
			},
		},
		"usage": map[string]any{
			"input_tokens":  profile.InputTokens,
			"output_tokens": profile.OutputTokens,
			"total_tokens":  profile.InputTokens + profile.OutputTokens,
			"input_tokens_details": map[string]any{
				"cached_tokens": profile.CachedTokens,
			},
		},
	}
}

func BuildTokenText(prefix string, tokens int) string {
	if tokens < 1 {
		tokens = 1
	}
	var builder strings.Builder
	builder.Grow(tokens * (len(prefix) + 6))
	for i := 0; i < tokens; i++ {
		if i > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(prefix)
		builder.WriteByte('_')
		builder.WriteString(fmt.Sprintf("%d", i%97))
	}
	return builder.String()
}

func BuildRandomTokenText(rng *rand.Rand, tokens int) string {
	if tokens <= 0 {
		return ""
	}
	if rng == nil {
		rng = NewRand()
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var builder strings.Builder
	builder.Grow(tokens * 5)
	for i := 0; i < tokens; i++ {
		if i > 0 {
			builder.WriteByte(' ')
		}
		tokenLen := 1 + rng.Intn(6)
		for j := 0; j < tokenLen; j++ {
			builder.WriteByte(alphabet[rng.Intn(len(alphabet))])
		}
	}
	return builder.String()
}

func SleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func escapeJSONString(value string) string {
	encoded, _ := json.Marshal(value)
	return strings.Trim(string(encoded), "\"")
}

func buildCreatedEvent(profile ResponseProfile) map[string]any {
	return map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         profile.ResponseID,
			"object":     "response",
			"created_at": profile.CreatedAt.Unix(),
			"model":      profile.Model,
			"status":     "in_progress",
			"output":     []any{},
		},
	}
}

func writeSSEFrame(w io.Writer, flusher http.Flusher, eventName string, payload []byte) error {
	frame := fmt.Sprintf("event: %s\ndata: %s\n\n", eventName, payload)
	if _, err := io.WriteString(w, frame); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeWebsocketJSON(ctx context.Context, conn *websocket.Conn, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, encoded)
}

func splitText(value string, chunkCount int) []string {
	if len(value) == 0 {
		return nil
	}
	if chunkCount <= 1 || len(value) == 0 {
		return []string{value}
	}
	if chunkCount > len(value) {
		chunkCount = len(value)
	}
	chunks := make([]string, 0, chunkCount)
	start := 0
	for remaining := chunkCount; remaining > 0; remaining-- {
		chunkLen := int(math.Ceil(float64(len(value)-start) / float64(remaining)))
		end := start + chunkLen
		if end > len(value) {
			end = len(value)
		}
		chunks = append(chunks, value[start:end])
		start = end
	}
	return chunks
}

func deriveChunkCount(outputDuration, chunkInterval time.Duration) int {
	if outputDuration <= 0 || chunkInterval <= 0 {
		return 1
	}
	return maxInt(int(math.Ceil(outputDuration.Seconds()/chunkInterval.Seconds())), 1)
}

func estimateInputTokens(raw json.RawMessage, fallback int) int {
	if len(bytes.TrimSpace(raw)) == 0 {
		return maxInt(fallback, 1)
	}
	var input any
	if err := json.Unmarshal(raw, &input); err != nil {
		return maxInt(len(raw)/8, fallback)
	}
	chars := collectTextChars(input)
	if chars <= 0 {
		chars = len(raw) / 2
	}
	return maxInt(chars/4, fallback)
}

func collectTextChars(value any) int {
	switch typed := value.(type) {
	case string:
		return len(typed)
	case []any:
		total := 0
		for _, item := range typed {
			total += collectTextChars(item)
		}
		return total
	case map[string]any:
		total := 0
		for _, item := range typed {
			total += collectTextChars(item)
		}
		return total
	default:
		return 0
	}
}

func sampleTTFB(cfg SimulationConfig, rng *rand.Rand) time.Duration {
	primaryEnd := minDuration(4500*time.Millisecond, cfg.TTFBMainMax)
	midStart := maxDuration(primaryEnd, cfg.TTFBMin)
	roll := rng.Float64()
	switch {
	case roll < 0.85:
		return randomDurationBetween(rng, cfg.TTFBMin, primaryEnd)
	case roll < 0.99:
		return randomDurationBetween(rng, midStart, cfg.TTFBMainMax)
	default:
		return randomDurationBetween(rng, cfg.TTFBMainMax, cfg.TTFBTailMax)
	}
}

func sampleTPS(cfg SimulationConfig, rng *rand.Rand) float64 {
	return cfg.TPSMin + rng.Float64()*(cfg.TPSMax-cfg.TPSMin)
}

func sampleOutputDuration(cfg SimulationConfig, inputTokens int) time.Duration {
	seconds := 10 + int(math.Round(float64(maxInt(inputTokens, 1))/32.0))
	duration := time.Duration(seconds) * time.Second
	if duration < cfg.OutputMin {
		return cfg.OutputMin
	}
	if duration > cfg.OutputMax {
		return cfg.OutputMax
	}
	return duration
}

func sampleCachedTokens(rng *rand.Rand, inputTokens int) int {
	switch roll := rng.Intn(100); {
	case roll < 50:
		return 0
	case roll < 80:
		return inputTokens / 4
	default:
		return inputTokens / 2
	}
}

func nextResponseID() string {
	return fmt.Sprintf("resp_mock_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&responseCounter, 1))
}

func httpErrorPayload(statusCode int, message string) string {
	if strings.TrimSpace(message) == "" {
		message = http.StatusText(statusCode)
	}
	payload := map[string]any{
		"type":   "error",
		"status": statusCode,
		"error": map[string]any{
			"type":    "invalid_request_error",
			"message": message,
		},
	}
	if statusCode >= 500 {
		payload["error"].(map[string]any)["type"] = "server_error"
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func randomDurationBetween(rng *rand.Rand, start, end time.Duration) time.Duration {
	if end <= start {
		return start
	}
	return start + time.Duration(rng.Int63n(int64(end-start)+1))
}

func (p ResponseProfile) outputText() string {
	if p.OutputTokens <= 0 {
		return ""
	}
	if p.OutputText != "" {
		return p.OutputText
	}
	return BuildTokenText("answer", p.OutputTokens)
}

func maxInt(v, floor int) int {
	if v < floor {
		return floor
	}
	return v
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
