package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
)

var supportedModels = []string{
	"gpt-5.4",
	"gpt-5.3-codex",
	"gpt-5.2",
	"gpt-5.4-mini",
}

type statsTracker struct {
	httpRequests            atomic.Uint64
	websocketTurns          atomic.Uint64
	activeHTTP              atomic.Int64
	activeWebsocketSessions atomic.Int64
	completedResponses      atomic.Uint64
	totalInputTokens        atomic.Uint64
	totalOutputTokens       atomic.Uint64
	totalSimulatedMs        atomic.Uint64
}

type responseSimulation struct {
	Model            string
	InputTokens      int
	OutputTokens     int
	TTFB             time.Duration
	ChunkDelay       time.Duration
	GenerationTime   time.Duration
	OutputText       string
	ResponseID       string
	OutputItemID     string
	MessageID        string
	CreatedAtUnixSec int64
}

func main() {
	var port int
	flag.IntVar(&port, "port", 18080, "listen port")
	flag.Parse()

	stats := &statsTracker{}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"models": supportedModels,
		})
	})
	mux.HandleFunc("/debug/stats", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"httpRequests":            stats.httpRequests.Load(),
			"websocketTurns":          stats.websocketTurns.Load(),
			"activeHTTP":              stats.activeHTTP.Load(),
			"activeWebsocketSessions": stats.activeWebsocketSessions.Load(),
			"completedResponses":      stats.completedResponses.Load(),
			"totalInputTokens":        stats.totalInputTokens.Load(),
			"totalOutputTokens":       stats.totalOutputTokens.Load(),
			"totalSimulatedMs":        stats.totalSimulatedMs.Load(),
			"models":                  supportedModels,
		})
	})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		data := make([]map[string]any, 0, len(supportedModels))
		for _, modelID := range supportedModels {
			data = append(data, map[string]any{
				"id":       modelID,
				"object":   "model",
				"created":  time.Now().Unix(),
				"owned_by": "ampmanager-fake",
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data":   data,
		})
	})
	mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		if websocketRequested(r) {
			handleResponsesWebsocket(w, r, stats)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		handleResponsesHTTP(w, r, stats)
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("fake responses upstream listening on %s with %d model(s)", addr, len(supportedModels))
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("fake responses upstream failed: %v", err)
	}
}

func handleResponsesHTTP(w http.ResponseWriter, r *http.Request, stats *statsTracker) {
	stats.httpRequests.Add(1)
	stats.activeHTTP.Add(1)
	defer stats.activeHTTP.Add(-1)

	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to read request"})
		return
	}

	sim := buildSimulation(body, time.Now())
	recordSimulation(stats, sim)

	stream := requestWantsStream(body)
	if stream {
		writeResponsesSSE(w, sim, stats)
		return
	}

	time.Sleep(sim.TTFB + sim.GenerationTime)
	writeJSON(w, http.StatusOK, buildCompletedResponseObject(sim))
	stats.completedResponses.Add(1)
}

func handleResponsesWebsocket(w http.ResponseWriter, r *http.Request, stats *statsTracker) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	stats.activeWebsocketSessions.Add(1)
	defer stats.activeWebsocketSessions.Add(-1)

	for {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
		_, payload, err := conn.Read(ctx)
		cancel()
		if err != nil {
			return
		}

		reqType := stringValue(payload, "type")
		if reqType != "response.create" && reqType != "response.append" {
			_ = conn.Write(r.Context(), websocket.MessageText, mustJSON(map[string]any{
				"type": "error",
				"error": map[string]any{
					"message": "unsupported websocket request type",
				},
			}))
			continue
		}

		stats.websocketTurns.Add(1)
		sim := buildSimulation(payload, time.Now())
		recordSimulation(stats, sim)

		time.Sleep(sim.TTFB)
		if err := conn.Write(r.Context(), websocket.MessageText, mustJSON(buildCreatedEvent(sim))); err != nil {
			return
		}

		chunkSize := max(16, int(math.Round(float64(sim.OutputTokens)*sim.ChunkDelay.Seconds()/sim.GenerationTime.Seconds())))
		if chunkSize <= 0 {
			chunkSize = 16
		}
		for offset := 0; offset < len(sim.OutputText); offset += chunkSize {
			end := min(len(sim.OutputText), offset+chunkSize)
			if err := conn.Write(r.Context(), websocket.MessageText, mustJSON(buildDeltaEvent(sim, sim.OutputText[offset:end]))); err != nil {
				return
			}
			if end < len(sim.OutputText) {
				time.Sleep(sim.ChunkDelay)
			}
		}

		if err := conn.Write(r.Context(), websocket.MessageText, mustJSON(buildCompletedEvent(sim))); err != nil {
			return
		}
		stats.completedResponses.Add(1)
	}
}

func writeResponsesSSE(w http.ResponseWriter, sim responseSimulation, stats *statsTracker) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "streaming unsupported"})
		return
	}

	time.Sleep(sim.TTFB)
	writeSSEEvent(w, "response.created", mustJSON(buildCreatedEvent(sim)))
	flusher.Flush()

	chunkSize := max(16, int(math.Round(float64(sim.OutputTokens)*sim.ChunkDelay.Seconds()/sim.GenerationTime.Seconds())))
	if chunkSize <= 0 {
		chunkSize = 16
	}
	for offset := 0; offset < len(sim.OutputText); offset += chunkSize {
		end := min(len(sim.OutputText), offset+chunkSize)
		writeSSEEvent(w, "response.output_text.delta", mustJSON(buildDeltaEvent(sim, sim.OutputText[offset:end])))
		flusher.Flush()
		if end < len(sim.OutputText) {
			time.Sleep(sim.ChunkDelay)
		}
	}
	writeSSEEvent(w, "response.completed", mustJSON(buildCompletedEvent(sim)))
	writeSSEDone(w)
	flusher.Flush()
	stats.completedResponses.Add(1)
}

func buildSimulation(body []byte, now time.Time) responseSimulation {
	modelName := stringValue(body, "model")
	if modelName == "" {
		modelName = supportedModels[0]
	}
	inputChars := estimateInputChars(body)
	inputTokens := max(16, inputChars/4)

	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(inputTokens)))
	ttfb := sampleTTFB(rng)
	tps := 30 + rng.Float64()*30
	generationSeconds := clampFloat64(10, 200, 10+float64(inputTokens)/40)
	outputTokens := max(64, int(math.Round(tps*generationSeconds)))
	chunkDelay := time.Duration(120+rng.Intn(180)) * time.Millisecond

	return responseSimulation{
		Model:            modelName,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		TTFB:             ttfb,
		ChunkDelay:       chunkDelay,
		GenerationTime:   time.Duration(generationSeconds * float64(time.Second)),
		OutputText:       randomTokenText(rng, outputTokens),
		ResponseID:       "resp_" + uuid.NewString(),
		OutputItemID:     "out_" + uuid.NewString(),
		MessageID:        "msg_" + uuid.NewString(),
		CreatedAtUnixSec: now.Unix(),
	}
}

func buildCreatedEvent(sim responseSimulation) map[string]any {
	return map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         sim.ResponseID,
			"object":     "response",
			"created_at": sim.CreatedAtUnixSec,
			"status":     "in_progress",
			"model":      sim.Model,
			"output":     []any{},
		},
	}
}

func buildDeltaEvent(sim responseSimulation, chunk string) map[string]any {
	return map[string]any{
		"type":          "response.output_text.delta",
		"response_id":   sim.ResponseID,
		"output_index":  0,
		"content_index": 0,
		"item_id":       sim.OutputItemID,
		"delta":         chunk,
	}
}

func buildCompletedEvent(sim responseSimulation) map[string]any {
	return map[string]any{
		"type":     "response.completed",
		"response": buildCompletedResponseObject(sim),
	}
}

func buildCompletedResponseObject(sim responseSimulation) map[string]any {
	return map[string]any{
		"id":         sim.ResponseID,
		"object":     "response",
		"created_at": sim.CreatedAtUnixSec,
		"status":     "completed",
		"model":      sim.Model,
		"output": []any{
			map[string]any{
				"id":     sim.MessageID,
				"type":   "message",
				"status": "completed",
				"role":   "assistant",
				"content": []any{
					map[string]any{
						"type": "output_text",
						"text": sim.OutputText,
					},
				},
			},
		},
		"usage": map[string]any{
			"input_tokens":  sim.InputTokens,
			"output_tokens": sim.OutputTokens,
			"total_tokens":  sim.InputTokens + sim.OutputTokens,
		},
	}
}

func recordSimulation(stats *statsTracker, sim responseSimulation) {
	stats.totalInputTokens.Add(uint64(sim.InputTokens))
	stats.totalOutputTokens.Add(uint64(sim.OutputTokens))
	stats.totalSimulatedMs.Add(uint64((sim.TTFB + sim.GenerationTime) / time.Millisecond))
}

func estimateInputChars(body []byte) int {
	payload := make(map[string]any)
	if err := json.Unmarshal(body, &payload); err != nil {
		return len(body)
	}
	total := 0
	collectStrings(payload, &total)
	return max(64, total)
}

func collectStrings(value any, total *int) {
	switch typed := value.(type) {
	case string:
		*total += len(typed)
	case []any:
		for _, item := range typed {
			collectStrings(item, total)
		}
	case map[string]any:
		for _, item := range typed {
			collectStrings(item, total)
		}
	}
}

func requestWantsStream(body []byte) bool {
	payload := make(map[string]any)
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	stream, _ := payload["stream"].(bool)
	return stream
}

func stringValue(body []byte, key string) string {
	payload := make(map[string]any)
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func sampleTTFB(rng *rand.Rand) time.Duration {
	switch roll := rng.Intn(100); {
	case roll < 90:
		return time.Duration(1500+rng.Intn(4500)) * time.Millisecond
	case roll < 99:
		return time.Duration(6000+rng.Intn(900)) * time.Millisecond
	default:
		return 7 * time.Second
	}
}

func randomTokenText(rng *rand.Rand, outputTokens int) string {
	alphabet := "abcdefghijklmnopqrstuvwxyz0123456789 "
	var builder strings.Builder
	builder.Grow(outputTokens)
	for i := 0; i < outputTokens; i++ {
		builder.WriteByte(alphabet[rng.Intn(len(alphabet))])
	}
	return builder.String()
}

func websocketRequested(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func writeSSEEvent(w http.ResponseWriter, event string, payload []byte) {
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
}

func writeSSEDone(w http.ResponseWriter) {
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	encoded := mustJSON(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(encoded)
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte(`{"error":"json_marshal_failed"}`)
	}
	return encoded
}

func clampFloat64(minValue, maxValue, value float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
