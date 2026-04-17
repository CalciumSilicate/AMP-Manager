package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

type manifestUser struct {
	UserID           string   `json:"user_id"`
	Username         string   `json:"username"`
	APIKey           string   `json:"api_key"`
	SupportedModels  []string `json:"supported_models,omitempty"`
	SubscriptionPlan string   `json:"subscription_plan_id,omitempty"`
}

type seedManifest struct {
	GeneratedAt     string         `json:"generated_at"`
	AppURL          string         `json:"app_url,omitempty"`
	UpstreamURL     string         `json:"upstream_url,omitempty"`
	UserPrefix      string         `json:"user_prefix,omitempty"`
	SupportedModels []string       `json:"supported_models,omitempty"`
	Users           []manifestUser `json:"users"`
}

type tierConfig struct {
	Name           string `json:"name"`
	UserCount      int    `json:"user_count"`
	MinConcurrency int    `json:"min_concurrency"`
	MaxConcurrency int    `json:"max_concurrency"`
}

type requestMetric struct {
	Duration     time.Duration
	TTFB         time.Duration
	TPS          float64
	StatusCode   int
	Err          string
	ErrCategory  string
	OutputTokens int
}

type tierAccumulator struct {
	name       string
	total      atomic.Int64
	success    atomic.Int64
	failures   atomic.Int64
	active     atomic.Int64
	durationMu sync.Mutex
	metrics    []requestMetric
}

type reportTier struct {
	Name           string           `json:"name"`
	Users          int              `json:"users"`
	MinConcurrency int              `json:"min_concurrency"`
	MaxConcurrency int              `json:"max_concurrency"`
	Requests       int64            `json:"requests"`
	Success        int64            `json:"success"`
	Failures       int64            `json:"failures"`
	TTFBP50Ms      float64          `json:"ttfb_p50_ms"`
	TTFBP95Ms      float64          `json:"ttfb_p95_ms"`
	TTFBP99Ms      float64          `json:"ttfb_p99_ms"`
	DurationP50Ms  float64          `json:"duration_p50_ms"`
	DurationP95Ms  float64          `json:"duration_p95_ms"`
	DurationP99Ms  float64          `json:"duration_p99_ms"`
	TPSAvg         float64          `json:"tps_avg"`
	ErrorCounts    map[string]int64 `json:"error_counts,omitempty"`
	ErrorSamples   []string         `json:"error_samples,omitempty"`
}

type loadtestReport struct {
	GeneratedAt       string       `json:"generated_at"`
	AppURL            string       `json:"app_url"`
	Transport         string       `json:"transport"`
	WebsocketShare    float64      `json:"websocket_share,omitempty"`
	ManifestPath      string       `json:"manifest_path"`
	UserPrefix        string       `json:"user_prefix,omitempty"`
	DurationSec       int          `json:"duration_sec"`
	RequestTimeoutSec int          `json:"request_timeout_sec"`
	Tiers             []reportTier `json:"tiers"`
	Overall           reportTier   `json:"overall"`
}

func main() {
	var manifestPath string
	var appURL string
	var durationSec int
	var requestTimeoutSec int
	var outputPath string
	var transport string
	var websocketShare float64
	var seed int64
	var singleUsers int
	var burstUsers int
	var burstMin int
	var burstMax int
	var hotUsers int
	var hotMin int
	var hotMax int

	flag.StringVar(&manifestPath, "manifest", "perf/runtime/seed-manifest.json", "seed manifest path")
	flag.StringVar(&appURL, "app-url", "", "amp manager base url, defaults to manifest app_url")
	flag.StringVar(&transport, "transport", "sse", "transport: sse, ws, or mixed")
	flag.Float64Var(&websocketShare, "ws-share", 0.3, "when transport=mixed, fraction of websocket requests")
	flag.IntVar(&durationSec, "duration-sec", 120, "load duration in seconds")
	flag.IntVar(&requestTimeoutSec, "request-timeout-sec", 240, "per-request timeout in seconds")
	flag.StringVar(&outputPath, "output", "", "optional report json path")
	flag.Int64Var(&seed, "seed", time.Now().UnixNano(), "random seed")
	flag.IntVar(&singleUsers, "single-users", 1000, "tier 1 users with single concurrency")
	flag.IntVar(&burstUsers, "burst-users", 500, "tier 2 users with ranged concurrency")
	flag.IntVar(&burstMin, "burst-min", 2, "tier 2 min concurrency")
	flag.IntVar(&burstMax, "burst-max", 4, "tier 2 max concurrency")
	flag.IntVar(&hotUsers, "hot-users", 100, "tier 3 users with high concurrency")
	flag.IntVar(&hotMin, "hot-min", 20, "tier 3 min concurrency")
	flag.IntVar(&hotMax, "hot-max", 50, "tier 3 max concurrency")
	flag.Parse()

	manifest, err := loadManifest(manifestPath)
	if err != nil {
		log.Fatalf("load manifest failed: %v", err)
	}
	if strings.TrimSpace(appURL) == "" {
		appURL = strings.TrimSpace(manifest.AppURL)
	}
	if appURL == "" {
		log.Fatal("app-url is required either via flag or manifest")
	}
	transport = strings.ToLower(strings.TrimSpace(transport))
	if transport != "sse" && transport != "ws" && transport != "mixed" {
		log.Fatal("transport must be sse, ws, or mixed")
	}
	if websocketShare < 0 || websocketShare > 1 {
		log.Fatal("ws-share must be between 0 and 1")
	}
	if len(manifest.Users) < singleUsers+burstUsers+hotUsers {
		log.Fatalf("manifest users=%d is less than requested tiers total=%d", len(manifest.Users), singleUsers+burstUsers+hotUsers)
	}

	rng := rand.New(rand.NewSource(seed))
	users := append([]manifestUser(nil), manifest.Users...)
	for idx := range users {
		if len(users[idx].SupportedModels) == 0 {
			users[idx].SupportedModels = append([]string(nil), manifest.SupportedModels...)
		}
	}
	rng.Shuffle(len(users), func(i, j int) {
		users[i], users[j] = users[j], users[i]
	})

	tierConfigs := []tierConfig{
		{Name: "single", UserCount: singleUsers, MinConcurrency: 1, MaxConcurrency: 1},
		{Name: "burst", UserCount: burstUsers, MinConcurrency: burstMin, MaxConcurrency: burstMax},
		{Name: "hot", UserCount: hotUsers, MinConcurrency: hotMin, MaxConcurrency: hotMax},
	}

	accumulators := make([]*tierAccumulator, 0, len(tierConfigs))
	for _, tier := range tierConfigs {
		accumulators = append(accumulators, &tierAccumulator{name: tier.Name, metrics: make([]requestMetric, 0, 1024)})
	}
	overall := &tierAccumulator{name: "overall", metrics: make([]requestMetric, 0, 4096)}

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        20000,
			MaxIdleConnsPerHost: 20000,
			MaxConnsPerHost:     0,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	loadCtx, cancelLoad := context.WithTimeout(context.Background(), time.Duration(durationSec)*time.Second)
	defer cancelLoad()

	var wg sync.WaitGroup
	cursor := 0
	for idx, tier := range tierConfigs {
		acc := accumulators[idx]
		selected := users[cursor : cursor+tier.UserCount]
		cursor += tier.UserCount

		for _, user := range selected {
			concurrency := tier.MinConcurrency
			if tier.MaxConcurrency > tier.MinConcurrency {
				concurrency += rng.Intn(tier.MaxConcurrency - tier.MinConcurrency + 1)
			}
			for workerIndex := 0; workerIndex < concurrency; workerIndex++ {
				wg.Add(1)
				workerSeed := rng.Int63()
				go func(user manifestUser, workerSeed int64) {
					defer wg.Done()
					runWorker(loadCtx, client, strings.TrimRight(appURL, "/"), transport, websocketShare, user, time.Duration(requestTimeoutSec)*time.Second, rand.New(rand.NewSource(workerSeed)), acc, overall)
				}(user, workerSeed)
			}
		}
	}

	wg.Wait()

	report := loadtestReport{
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		AppURL:            appURL,
		Transport:         transport,
		WebsocketShare:    websocketShare,
		ManifestPath:      manifestPath,
		UserPrefix:        manifest.UserPrefix,
		DurationSec:       durationSec,
		RequestTimeoutSec: requestTimeoutSec,
		Tiers:             make([]reportTier, 0, len(tierConfigs)),
	}
	for idx, tier := range tierConfigs {
		report.Tiers = append(report.Tiers, buildReportTier(tier, accumulators[idx]))
	}
	report.Overall = buildReportTier(tierConfig{Name: "overall"}, overall)

	printReport(report)
	if outputPath != "" {
		if err := writeReport(outputPath, &report); err != nil {
			log.Fatalf("write report failed: %v", err)
		}
	}
}

func runWorker(ctx context.Context, client *http.Client, appURL, transport string, websocketShare float64, user manifestUser, requestTimeout time.Duration, rng *rand.Rand, tierAcc, overallAcc *tierAccumulator) {
	models := user.SupportedModels
	if len(models) == 0 {
		models = []string{"gpt-5.4", "gpt-5.3-codex", "gpt-5.2", "gpt-5.4-mini"}
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		model := models[rng.Intn(len(models))]
		var metric requestMetric
		requestTransport := transport
		if transport == "mixed" {
			if rng.Float64() < websocketShare {
				requestTransport = "ws"
			} else {
				requestTransport = "sse"
			}
		}
		if requestTransport == "ws" {
			metric = issueWebsocketRequest(ctx, appURL, user.APIKey, model, rng, requestTimeout)
		} else {
			metric = issueStreamingRequest(ctx, client, appURL, user.APIKey, model, rng, requestTimeout)
		}
		recordMetric(tierAcc, metric)
		recordMetric(overallAcc, metric)
	}
}

func issueStreamingRequest(parent context.Context, client *http.Client, appURL, apiKey, model string, rng *rand.Rand, requestTimeout time.Duration) requestMetric {
	metric := requestMetric{}
	_ = parent
	reqCtx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	body := buildRequestBody(model, sampleInputTokens(rng))
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, appURL+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		metric.Err = err.Error()
		metric.ErrCategory = classifyMetricError(metric.StatusCode, metric.Err)
		return metric
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	startedAt := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		metric.Err = err.Error()
		metric.Duration = time.Since(startedAt)
		return metric
	}
	defer resp.Body.Close()
	metric.StatusCode = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		metric.Err = strings.TrimSpace(string(bodyBytes))
		metric.Duration = time.Since(startedAt)
		metric.ErrCategory = classifyMetricError(metric.StatusCode, metric.Err)
		return metric
	}

	reader := bufio.NewReader(resp.Body)
	firstByteAt := time.Time{}
	outputTokens := 0
	var currentEvent string

	for {
		line, err := reader.ReadString('\n')
		if line != "" && firstByteAt.IsZero() {
			firstByteAt = time.Now()
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			metric.Err = err.Error()
			metric.ErrCategory = classifyMetricError(metric.StatusCode, metric.Err)
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			currentEvent = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		if currentEvent == "response.completed" || strings.Contains(payload, `"type":"response.completed"`) {
			outputTokens = extractOutputTokens([]byte(payload))
		}
	}

	metric.Duration = time.Since(startedAt)
	if !firstByteAt.IsZero() {
		metric.TTFB = firstByteAt.Sub(startedAt)
	}
	metric.OutputTokens = outputTokens
	if outputTokens > 0 && metric.Duration > metric.TTFB {
		streamingDuration := metric.Duration - metric.TTFB
		metric.TPS = float64(outputTokens) / streamingDuration.Seconds()
	}
	return metric
}

func classifyMetricError(statusCode int, message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case lower == "":
		if statusCode >= 200 && statusCode < 300 {
			return ""
		}
		if statusCode == 502 {
			return "upstream_request_failed"
		}
		if statusCode >= 500 {
			return "server_error"
		}
		if statusCode >= 400 {
			return "client_error"
		}
		return "unknown"
	case strings.Contains(lower, "context deadline exceeded"):
		return "client_timeout"
	case strings.Contains(lower, "insufficient_quota") || strings.Contains(lower, "余额和订阅额度均不足"):
		return "insufficient_quota"
	case strings.Contains(lower, "invalid_api_key") || strings.Contains(lower, "invalid api key"):
		return "invalid_api_key"
	case strings.Contains(lower, "rate_limit_exceeded") || strings.Contains(lower, "rate limit"):
		return "rate_limited"
	case strings.Contains(lower, "upstream_request_failed"):
		return "upstream_request_failed"
	case statusCode == 502:
		return "upstream_request_failed"
	case statusCode >= 500:
		return "server_error"
	case statusCode >= 400:
		return "client_error"
	default:
		return "other_error"
	}
}

func extractOutputTokens(payload []byte) int {
	var envelope struct {
		Response struct {
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return 0
	}
	return envelope.Response.Usage.OutputTokens
}

func buildRequestBody(model string, inputTokens int) []byte {
	prompt := randomText(rand.New(rand.NewSource(time.Now().UnixNano())), inputTokens*4)
	payload := map[string]any{
		"model": model,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": prompt,
					},
				},
			},
		},
		"instructions": "Load test through AMP Manager with long-lived streaming responses.",
		"reasoning": map[string]any{
			"effort": "low",
		},
		"stream": true,
	}
	body, _ := json.Marshal(payload)
	return body
}

func buildWebsocketRequestBody(model string, inputTokens int) []byte {
	prompt := randomText(rand.New(rand.NewSource(time.Now().UnixNano())), inputTokens*4)
	payload := map[string]any{
		"type":  "response.create",
		"model": model,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": prompt,
					},
				},
			},
		},
		"instructions": "Load test through AMP Manager responses websocket path.",
		"reasoning": map[string]any{
			"effort": "low",
		},
	}
	body, _ := json.Marshal(payload)
	return body
}

func issueWebsocketRequest(parent context.Context, appURL, apiKey, model string, rng *rand.Rand, requestTimeout time.Duration) requestMetric {
	metric := requestMetric{}
	_ = parent
	reqCtx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	wsURL := websocketURL(appURL) + "/v1/responses"
	conn, _, err := websocket.Dial(reqCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + apiKey},
		},
	})
	if err != nil {
		metric.Err = err.Error()
		metric.ErrCategory = classifyMetricError(metric.StatusCode, metric.Err)
		return metric
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	body := buildWebsocketRequestBody(model, sampleInputTokens(rng))
	startedAt := time.Now()
	if err := conn.Write(reqCtx, websocket.MessageText, body); err != nil {
		metric.Err = err.Error()
		metric.ErrCategory = classifyMetricError(metric.StatusCode, metric.Err)
		metric.Duration = time.Since(startedAt)
		return metric
	}

	firstByteAt := time.Time{}
	outputTokens := 0
	for {
		_, payload, err := conn.Read(reqCtx)
		if len(payload) > 0 && firstByteAt.IsZero() {
			firstByteAt = time.Now()
		}
		if err != nil {
			metric.Err = err.Error()
			metric.ErrCategory = classifyMetricError(metric.StatusCode, metric.Err)
			break
		}

		eventType := websocketEventType(payload)
		if eventType == "error" {
			metric.Err = strings.TrimSpace(string(payload))
			metric.ErrCategory = classifyMetricError(metric.StatusCode, metric.Err)
			break
		}
		if eventType == "response.completed" {
			outputTokens = extractOutputTokens(payload)
			metric.StatusCode = 200
			break
		}
	}

	metric.Duration = time.Since(startedAt)
	if !firstByteAt.IsZero() {
		metric.TTFB = firstByteAt.Sub(startedAt)
	}
	metric.OutputTokens = outputTokens
	if outputTokens > 0 && metric.Duration > metric.TTFB {
		streamingDuration := metric.Duration - metric.TTFB
		metric.TPS = float64(outputTokens) / streamingDuration.Seconds()
	}
	return metric
}

func websocketEventType(payload []byte) string {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Type)
}

func websocketURL(appURL string) string {
	if strings.HasPrefix(appURL, "https://") {
		return "wss://" + strings.TrimPrefix(appURL, "https://")
	}
	return "ws://" + strings.TrimPrefix(appURL, "http://")
}

func sampleInputTokens(rng *rand.Rand) int {
	switch roll := rng.Intn(100); {
	case roll < 40:
		return 64 + rng.Intn(128)
	case roll < 75:
		return 256 + rng.Intn(512)
	case roll < 95:
		return 1024 + rng.Intn(2048)
	default:
		return 4096 + rng.Intn(4096)
	}
}

func randomText(rng *rand.Rand, length int) string {
	if length <= 0 {
		return "loadtest"
	}
	alphabet := "abcdefghijklmnopqrstuvwxyz0123456789 "
	var builder strings.Builder
	builder.Grow(length)
	for i := 0; i < length; i++ {
		builder.WriteByte(alphabet[rng.Intn(len(alphabet))])
	}
	return builder.String()
}

func recordMetric(acc *tierAccumulator, metric requestMetric) {
	acc.total.Add(1)
	if metric.Err == "" && metric.StatusCode >= 200 && metric.StatusCode < 300 {
		acc.success.Add(1)
	} else {
		acc.failures.Add(1)
	}
	acc.durationMu.Lock()
	acc.metrics = append(acc.metrics, metric)
	acc.durationMu.Unlock()
}

func buildReportTier(cfg tierConfig, acc *tierAccumulator) reportTier {
	acc.durationMu.Lock()
	metrics := append([]requestMetric(nil), acc.metrics...)
	acc.durationMu.Unlock()

	durations := make([]float64, 0, len(metrics))
	ttfbs := make([]float64, 0, len(metrics))
	tpsValues := make([]float64, 0, len(metrics))
	errorSamples := make([]string, 0, 5)
	errorCounts := make(map[string]int64)
	for _, metric := range metrics {
		durations = append(durations, float64(metric.Duration)/float64(time.Millisecond))
		if metric.TTFB > 0 {
			ttfbs = append(ttfbs, float64(metric.TTFB)/float64(time.Millisecond))
		}
		if metric.TPS > 0 {
			tpsValues = append(tpsValues, metric.TPS)
		}
		if metric.Err != "" && len(errorSamples) < 5 {
			errorSamples = append(errorSamples, metric.Err)
		}
		if metric.ErrCategory != "" {
			errorCounts[metric.ErrCategory]++
		}
	}

	return reportTier{
		Name:           cfg.Name,
		Users:          cfg.UserCount,
		MinConcurrency: cfg.MinConcurrency,
		MaxConcurrency: cfg.MaxConcurrency,
		Requests:       acc.total.Load(),
		Success:        acc.success.Load(),
		Failures:       acc.failures.Load(),
		TTFBP50Ms:      percentile(ttfbs, 0.50),
		TTFBP95Ms:      percentile(ttfbs, 0.95),
		TTFBP99Ms:      percentile(ttfbs, 0.99),
		DurationP50Ms:  percentile(durations, 0.50),
		DurationP95Ms:  percentile(durations, 0.95),
		DurationP99Ms:  percentile(durations, 0.99),
		TPSAvg:         average(tpsValues),
		ErrorCounts:    errorCounts,
		ErrorSamples:   errorSamples,
	}
}

func percentile(values []float64, ratio float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(ratio*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func loadManifest(path string) (*seedManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest seedManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func writeReport(path string, report *loadtestReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func printReport(report loadtestReport) {
	log.Printf("loadtest app=%s duration=%ds requests=%d success=%d failures=%d ttfb_p95=%.2fms duration_p95=%.2fms tps_avg=%.2f",
		report.AppURL,
		report.DurationSec,
		report.Overall.Requests,
		report.Overall.Success,
		report.Overall.Failures,
		report.Overall.TTFBP95Ms,
		report.Overall.DurationP95Ms,
		report.Overall.TPSAvg,
	)
	for _, tier := range report.Tiers {
		log.Printf("tier=%s users=%d concurrency=%d-%d requests=%d success=%d failures=%d ttfb_p95=%.2fms duration_p95=%.2fms tps_avg=%.2f",
			tier.Name,
			tier.Users,
			tier.MinConcurrency,
			tier.MaxConcurrency,
			tier.Requests,
			tier.Success,
			tier.Failures,
			tier.TTFBP95Ms,
			tier.DurationP95Ms,
			tier.TPSAvg,
		)
	}
}
