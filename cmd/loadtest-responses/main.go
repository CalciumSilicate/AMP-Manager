package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ampmanager/internal/amp"
	"ampmanager/internal/billing"
	"ampmanager/internal/billingstate"
	"ampmanager/internal/config"
	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/translator"
	"ampmanager/internal/translator/filters"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type benchmarkEnv struct {
	apiKey  string
	appURL  string
	mockURL string
	cleanup func()
	model   string
	mode    string
	rand    *rand.Rand
	client  *http.Client
}

type scenario struct {
	name             string
	clientStream     bool
	upstreamGzip     bool
	inputTokens      int
	outputTokens     int
	cachedTokens     int
	ttfbDelay        time.Duration
	chunkDelay       time.Duration
	expectedHTTPCode int
}

type stageMetrics struct {
	rpm               int
	total             int64
	success           int64
	failures          int64
	elapsed           time.Duration
	endToEndP50       time.Duration
	endToEndP95       time.Duration
	endToEndP99       time.Duration
	admissionP50      time.Duration
	admissionP95      time.Duration
	admissionP99      time.Duration
	settleP50         time.Duration
	settleP95         time.Duration
	settleP99         time.Duration
	projectP50        time.Duration
	projectP95        time.Duration
	projectP99        time.Duration
	throughputRPS     float64
	errorRate         float64
	validationFail    int64
	admissionFailures int64
	settleFailures    int64
	projectFailures   int64
	reconcileFailures int64
	reconcileRepairs  int64
	errorSamples      []string
}

func main() {
	var (
		stageSeconds int
		timeout      time.Duration
		seed         int64
		databaseURL  string
		redisURL     string
		redisPrefix  string
		legacyLocal  bool
	)

	flag.IntVar(&stageSeconds, "stage-seconds", 8, "duration of each RPM stage in seconds")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "per-request timeout")
	flag.Int64Var(&seed, "seed", 42, "random seed")
	flag.StringVar(&databaseURL, "database-url", os.Getenv("DATABASE_URL"), "postgres database url for shared billing benchmark")
	flag.StringVar(&redisURL, "redis-url", os.Getenv("REDIS_URL"), "redis url for shared billing benchmark")
	flag.StringVar(&redisPrefix, "redis-prefix", os.Getenv("REDIS_PREFIX"), "redis key prefix for shared billing benchmark")
	flag.BoolVar(&legacyLocal, "legacy-local", false, "use self-contained sqlite smoke mode instead of real shared billing")
	flag.Parse()

	log.SetOutput(io.Discard)
	log.SetLevel(log.ErrorLevel)
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard

	env, err := setupBenchmarkEnv(timeout, seed, databaseURL, redisURL, redisPrefix, legacyLocal)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup failed: %v\n", err)
		os.Exit(1)
	}
	defer env.cleanup()

	fmt.Printf("loadtest mode=%s target=%s mock=%s model=%s stage_duration=%ds\n", env.mode, env.appURL, env.mockURL, env.model, stageSeconds)
	if env.mode == "shared-billing" {
		fmt.Printf("billing path enabled with Redis + PostgreSQL, metrics include admission/settle/projector\n")
	} else {
		fmt.Printf("legacy-local mode: only for smoke verification, not representative of real billing throughput\n")
	}

	stages := []int{100, 300, 1000, 3000, 6000, 10000}
	overallOK := true

	for _, rpm := range stages {
		metrics := runStage(env, rpm, time.Duration(stageSeconds)*time.Second)
		fmt.Printf(
			"stage rpm=%5d total=%4d ok=%4d err=%3d err_rate=%5.2f%% throughput=%6.1f rps e2e[p50=%7s p95=%7s p99=%7s] admission[p50=%7s p95=%7s p99=%7s] settle[p50=%7s p95=%7s p99=%7s] projector[p50=%7s p95=%7s p99=%7s] validation_err=%d reserve_err=%d settle_err=%d projector_err=%d reconcile_err=%d repairs=%d\n",
			metrics.rpm,
			metrics.total,
			metrics.success,
			metrics.failures,
			metrics.errorRate*100,
			metrics.throughputRPS,
			metrics.endToEndP50.Round(time.Millisecond),
			metrics.endToEndP95.Round(time.Millisecond),
			metrics.endToEndP99.Round(time.Millisecond),
			metrics.admissionP50.Round(time.Millisecond),
			metrics.admissionP95.Round(time.Millisecond),
			metrics.admissionP99.Round(time.Millisecond),
			metrics.settleP50.Round(time.Millisecond),
			metrics.settleP95.Round(time.Millisecond),
			metrics.settleP99.Round(time.Millisecond),
			metrics.projectP50.Round(time.Millisecond),
			metrics.projectP95.Round(time.Millisecond),
			metrics.projectP99.Round(time.Millisecond),
			metrics.validationFail,
			metrics.admissionFailures,
			metrics.settleFailures,
			metrics.projectFailures,
			metrics.reconcileFailures,
			metrics.reconcileRepairs,
		)
		for _, sample := range metrics.errorSamples {
			fmt.Printf("  error_sample: %s\n", sample)
		}
		if metrics.failures > 0 {
			overallOK = false
		}
	}

	if overallOK {
		fmt.Println("result: all stages completed without request or validation failures")
		return
	}

	fmt.Println("result: one or more stages reported failures")
	os.Exit(1)
}

func setupBenchmarkEnv(timeout time.Duration, seed int64, databaseURL, redisURL, redisPrefix string, legacyLocal bool) (*benchmarkEnv, error) {
	_ = os.Setenv("ALLOW_INSECURE_DEFAULTS", "true")
	_ = os.Setenv("RATE_LIMIT_PROXY_RPS", "20000")
	_ = os.Setenv("RATE_LIMIT_AUTH_RPS", "20000")
	_ = os.Setenv("DB_MAX_OPEN_CONNS", "64")
	_ = os.Setenv("DB_MAX_IDLE_CONNS", "32")

	mock := httptest.NewServer(http.HandlerFunc(mockResponsesHandler))

	tempDir, err := os.MkdirTemp("", "amp-loadtest-*")
	if err != nil {
		mock.Close()
		return nil, err
	}

	mode := "legacy-local"
	cfg := config.Load()
	if !legacyLocal {
		if databaseURL == "" || redisURL == "" {
			mock.Close()
			_ = os.RemoveAll(tempDir)
			return nil, fmt.Errorf("shared billing benchmark requires both -database-url and -redis-url")
		}
	}
	if databaseURL != "" && redisURL != "" && !legacyLocal {
		mode = "shared-billing"
		cfg.DBType = string(database.DBTypePostgres)
		cfg.DatabaseURL = databaseURL
		cfg.RedisURL = redisURL
		if redisPrefix == "" {
			redisPrefix = fmt.Sprintf("lt-%d", time.Now().UnixNano())
		}
		cfg.RedisPrefix = redisPrefix
	} else {
		dbPath := filepath.Join(tempDir, "loadtest.db")
		cfg.DBType = string(database.DBTypeSQLite)
		cfg.SQLitePath = dbPath
	}
	cfg.RateLimitProxyRPS = 20000
	cfg.RateLimitAuthRPS = 20000

	if err := database.InitWithOptions(cfg.DatabaseOptions()); err != nil {
		mock.Close()
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("init db: %w", err)
	}

	translator.DefaultRegistry()
	filters.RegisterFilters()
	billing.InitPriceStore()
	billing.InitCostCalculator()
	if mode != "shared-billing" {
		amp.UpdateRequestDetailConfig(amp.RequestDetailConfig{
			Enabled:        false,
			PersistEnabled: false,
			TTL:            time.Second,
			MaxEntries:     1,
			MaxMemoryBytes: 1024,
			BodyCapBytes:   1,
		})
	}
	if cfg.RedisURL != "" {
		if err := billingstate.Init(billingstate.Config{
			RedisURL:          cfg.RedisURL,
			Prefix:            cfg.RedisPrefix,
			ReservationTTL:    10 * time.Minute,
			ReconcileInterval: time.Minute,
			StreamBatchSize:   100,
		}); err != nil {
			database.Close()
			mock.Close()
			_ = os.RemoveAll(tempDir)
			return nil, fmt.Errorf("init billing state: %w", err)
		}
	}

	rawAPIKey, userID, err := seedProxyData(mock.URL, mode == "shared-billing")
	if err != nil {
		billingstate.Close()
		database.Close()
		mock.Close()
		_ = os.RemoveAll(tempDir)
		return nil, err
	}
	if runtime := billingstate.Get(); runtime != nil {
		if err := runtime.RefreshUserState(context.Background(), userID); err != nil {
			billingstate.Close()
			database.Close()
			mock.Close()
			_ = os.RemoveAll(tempDir)
			return nil, err
		}
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	amp.RegisterProxyRoutes(engine, amp.CreateDynamicReverseProxy())
	app := httptest.NewServer(engine)

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        4096,
			MaxIdleConnsPerHost: 4096,
			MaxConnsPerHost:     0,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true,
		},
	}

	cleanup := func() {
		app.Close()
		billingstate.Close()
		billing.StopPriceStore()
		mock.Close()
		database.Close()
		_ = os.RemoveAll(tempDir)
	}

	return &benchmarkEnv{
		apiKey:  rawAPIKey,
		appURL:  app.URL,
		mockURL: mock.URL,
		cleanup: cleanup,
		model:   "gpt-4.1-mini",
		mode:    mode,
		rand:    rand.New(rand.NewSource(seed)),
		client:  client,
	}, nil
}

func seedProxyData(mockURL string, billingEnabled bool) (string, string, error) {
	userRepo := repository.NewUserRepository()
	groupRepo := repository.NewGroupRepository()
	settingsRepo := repository.NewAmpSettingsRepository()
	apiKeyRepo := repository.NewAPIKeyRepository()
	channelRepo := repository.NewChannelRepository()
	planRepo := repository.NewSubscriptionPlanRepository()
	subRepo := repository.NewUserSubscriptionRepository()

	user := &model.User{
		Username:      fmt.Sprintf("loadtest-user-%d", time.Now().UnixNano()),
		PasswordHash:  "noop",
		BalanceMicros: 2_000_000_000_000,
	}
	if err := userRepo.Create(user); err != nil {
		return "", "", fmt.Errorf("create user: %w", err)
	}

	if existingChannels, err := channelRepo.List(); err == nil {
		for _, ch := range existingChannels {
			if strings.HasPrefix(ch.Name, "loadtest-openai") && ch.Enabled {
				_ = channelRepo.SetEnabled(ch.ID, false)
			}
		}
	}

	group := &model.Group{
		Name:           fmt.Sprintf("loadtest-group-%d", time.Now().UnixNano()),
		Description:    "benchmark lane",
		RateMultiplier: 1,
	}
	if err := groupRepo.Create(group); err != nil {
		return "", "", fmt.Errorf("create group: %w", err)
	}
	if !billingEnabled {
		group.RateMultiplier = 0
	}
	if err := groupRepo.Update(group); err != nil {
		return "", "", fmt.Errorf("update group multiplier: %w", err)
	}
	if err := userRepo.SetGroups(user.ID, []string{group.ID}); err != nil {
		return "", "", fmt.Errorf("assign group: %w", err)
	}

	if err := settingsRepo.Upsert(&model.AmpSettings{
		UserID:          user.ID,
		UpstreamURL:     mockURL,
		Enabled:         true,
		WebSearchMode:   model.WebSearchModeUpstream,
		NativeMode:      false,
		ShowBalanceInAd: false,
	}); err != nil {
		return "", "", fmt.Errorf("upsert settings: %w", err)
	}

	rawAPIKey := fmt.Sprintf("lt-benchmark-key-%d", time.Now().UnixNano())
	sum := sha256.Sum256([]byte(rawAPIKey))
	if err := apiKeyRepo.Create(&model.UserAPIKey{
		UserID:  user.ID,
		Name:    "loadtest-key",
		Prefix:  rawAPIKey[:8],
		KeyHash: hex.EncodeToString(sum[:]),
		APIKey:  rawAPIKey,
	}); err != nil {
		return "", "", fmt.Errorf("create api key: %w", err)
	}

	modelsJSON, _ := json.Marshal([]model.ChannelModel{{Name: "gpt-4.1-mini"}})
	if err := channelRepo.Create(&model.Channel{
		Type:        model.ChannelTypeOpenAI,
		Endpoint:    model.ChannelEndpointResponses,
		Name:        fmt.Sprintf("loadtest-openai-%d", time.Now().UnixNano()),
		BaseURL:     mockURL,
		APIKey:      "upstream-benchmark-key",
		Enabled:     true,
		Weight:      1,
		Priority:    1,
		ModelsJSON:  string(modelsJSON),
		HeadersJSON: "{}",
	}); err != nil {
		return "", "", fmt.Errorf("create channel: %w", err)
	}

	if billingEnabled {
		now := time.Now().UTC()
		plan := &model.SubscriptionPlan{
			Name:        fmt.Sprintf("loadtest-plan-%d", now.UnixNano()),
			Description: "benchmark quota plan",
			Enabled:     true,
		}
		limits := []model.SubscriptionPlanLimit{
			{LimitType: model.LimitTypeDaily, WindowMode: model.WindowModeSliding, LimitMicros: 5_000_000_000_000},
			{LimitType: model.LimitTypeRolling5h, WindowMode: model.WindowModeSliding, LimitMicros: 5_000_000_000_000},
			{LimitType: model.LimitTypeTotal, WindowMode: model.WindowModeFixed, LimitMicros: 5_000_000_000_000},
		}
		if err := planRepo.Create(plan, limits); err != nil {
			return "", "", fmt.Errorf("create subscription plan: %w", err)
		}
		sub := &model.UserSubscription{
			UserID:    user.ID,
			PlanID:    plan.ID,
			StartsAt:  now,
			Status:    model.SubscriptionStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := subRepo.Assign(sub); err != nil {
			return "", "", fmt.Errorf("assign subscription: %w", err)
		}
	}

	return rawAPIKey, user.ID, nil
}

func runStage(env *benchmarkEnv, rpm int, duration time.Duration) stageMetrics {
	if runtime := billingstate.Get(); runtime != nil {
		runtime.SnapshotMetrics(true)
	}

	totalRequests := int(math.Round(float64(rpm) * duration.Minutes()))
	if totalRequests < 1 {
		totalRequests = 1
	}

	stageStart := time.Now()
	interval := duration / time.Duration(totalRequests)
	if interval <= 0 {
		interval = time.Millisecond
	}

	latencies := make([]time.Duration, 0, totalRequests)
	var latMu sync.Mutex
	var success atomic.Int64
	var failures atomic.Int64
	var validationFailures atomic.Int64
	var errorSamples []string
	var errorMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 512)

	for i := 0; i < totalRequests; i++ {
		targetStart := stageStart.Add(time.Duration(i) * interval)
		if sleep := time.Until(targetStart); sleep > 0 {
			time.Sleep(sleep)
		}

		sc := chooseScenario(env.rand)
		body := buildRequestBody(env.model, sc.inputTokens, sc.clientStream)

		wg.Add(1)
		sem <- struct{}{}

		go func(sc scenario, body []byte) {
			defer wg.Done()
			defer func() { <-sem }()

			start := time.Now()
			err := issueRequest(env, sc, body)
			latency := time.Since(start)

			latMu.Lock()
			latencies = append(latencies, latency)
			latMu.Unlock()

			if err != nil {
				failures.Add(1)
				if strings.HasPrefix(err.Error(), "validate:") {
					validationFailures.Add(1)
				}
				errorMu.Lock()
				if len(errorSamples) < 5 {
					errorSamples = append(errorSamples, err.Error())
				}
				errorMu.Unlock()
				return
			}
			success.Add(1)
		}(sc, body)
	}

	wg.Wait()
	elapsed := time.Since(stageStart)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	var runtimeMetricsAfter billingstate.RuntimeMetricsSnapshot
	if runtime := billingstate.Get(); runtime != nil {
		runtimeMetricsAfter = runtime.SnapshotMetrics(true)
	}

	return stageMetrics{
		rpm:               rpm,
		total:             int64(totalRequests),
		success:           success.Load(),
		failures:          failures.Load(),
		elapsed:           elapsed,
		endToEndP50:       percentile(latencies, 0.50),
		endToEndP95:       percentile(latencies, 0.95),
		endToEndP99:       percentile(latencies, 0.99),
		admissionP50:      percentile(runtimeMetricsAfter.ReserveDurations, 0.50),
		admissionP95:      percentile(runtimeMetricsAfter.ReserveDurations, 0.95),
		admissionP99:      percentile(runtimeMetricsAfter.ReserveDurations, 0.99),
		settleP50:         percentile(runtimeMetricsAfter.SettleDurations, 0.50),
		settleP95:         percentile(runtimeMetricsAfter.SettleDurations, 0.95),
		settleP99:         percentile(runtimeMetricsAfter.SettleDurations, 0.99),
		projectP50:        percentile(runtimeMetricsAfter.ProjectDurations, 0.50),
		projectP95:        percentile(runtimeMetricsAfter.ProjectDurations, 0.95),
		projectP99:        percentile(runtimeMetricsAfter.ProjectDurations, 0.99),
		throughputRPS:     float64(success.Load()) / elapsed.Seconds(),
		errorRate:         float64(failures.Load()) / float64(totalRequests),
		validationFail:    validationFailures.Load(),
		admissionFailures: runtimeMetricsAfter.ReserveFailures,
		settleFailures:    runtimeMetricsAfter.SettleFailures,
		projectFailures:   runtimeMetricsAfter.ProjectFailures,
		reconcileFailures: runtimeMetricsAfter.ReconcileFailures,
		reconcileRepairs:  runtimeMetricsAfter.ReconcileRepairs,
		errorSamples:      append([]string(nil), errorSamples...),
	}
}

func issueRequest(env *benchmarkEnv, sc scenario, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, env.appURL+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+env.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lt-Scenario", sc.name)
	req.Header.Set("X-Lt-Input-Tokens", fmt.Sprintf("%d", sc.inputTokens))
	req.Header.Set("X-Lt-Output-Tokens", fmt.Sprintf("%d", sc.outputTokens))
	req.Header.Set("X-Lt-Cached-Tokens", fmt.Sprintf("%d", sc.cachedTokens))
	req.Header.Set("X-Lt-Ttfb-Ms", fmt.Sprintf("%d", sc.ttfbDelay.Milliseconds()))
	req.Header.Set("X-Lt-Chunk-Delay-Ms", fmt.Sprintf("%d", sc.chunkDelay.Milliseconds()))
	if sc.upstreamGzip {
		req.Header.Set("X-Lt-Upstream-Gzip", "true")
	}

	resp, err := env.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return readErr
	}

	if resp.StatusCode != sc.expectedHTTPCode {
		return fmt.Errorf("unexpected status=%d body=%s", resp.StatusCode, truncate(string(payload), 200))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	if err := validateUsage(resp.Header.Get("Content-Type"), payload, sc); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	return nil
}

func chooseScenario(r *rand.Rand) scenario {
	roll := r.Intn(100)
	inputTokens := sampleInputTokens(r)
	outputTokens := sampleOutputTokens(r)
	cachedTokens := sampleCachedTokens(r, inputTokens)
	var ttfb time.Duration
	var chunkDelay time.Duration
	switch band := r.Intn(100); {
	case band < 70:
		ttfb = time.Duration(300+r.Intn(500)) * time.Millisecond
		chunkDelay = time.Duration(30+r.Intn(40)) * time.Millisecond
	case band < 95:
		ttfb = time.Duration(800+r.Intn(1700)) * time.Millisecond
		chunkDelay = time.Duration(60+r.Intn(90)) * time.Millisecond
	default:
		ttfb = time.Duration(2500+r.Intn(3500)) * time.Millisecond
		chunkDelay = time.Duration(120+r.Intn(180)) * time.Millisecond
	}

	switch {
	case roll < 35:
		return scenario{
			name:             "json",
			clientStream:     false,
			upstreamGzip:     r.Intn(100) < 20,
			inputTokens:      inputTokens,
			outputTokens:     outputTokens,
			cachedTokens:     cachedTokens,
			ttfbDelay:        ttfb,
			chunkDelay:       chunkDelay,
			expectedHTTPCode: http.StatusOK,
		}
	case roll < 70:
		return scenario{
			name:             "sse",
			clientStream:     true,
			inputTokens:      inputTokens,
			outputTokens:     outputTokens,
			cachedTokens:     cachedTokens,
			ttfbDelay:        ttfb,
			chunkDelay:       chunkDelay,
			expectedHTTPCode: http.StatusOK,
		}
	default:
		return scenario{
			name:             "aggregate",
			clientStream:     false,
			inputTokens:      inputTokens,
			outputTokens:     outputTokens,
			cachedTokens:     cachedTokens,
			ttfbDelay:        ttfb,
			chunkDelay:       chunkDelay,
			expectedHTTPCode: http.StatusOK,
		}
	}
}

func sampleInputTokens(r *rand.Rand) int {
	switch roll := r.Intn(100); {
	case roll < 50:
		return 128 + r.Intn(256)
	case roll < 85:
		return 512 + r.Intn(1024)
	default:
		return 2048 + r.Intn(2048)
	}
}

func sampleOutputTokens(r *rand.Rand) int {
	switch roll := r.Intn(100); {
	case roll < 60:
		return 64 + r.Intn(128)
	case roll < 90:
		return 192 + r.Intn(256)
	default:
		return 512 + r.Intn(512)
	}
}

func sampleCachedTokens(r *rand.Rand, inputTokens int) int {
	switch roll := r.Intn(100); {
	case roll < 50:
		return 0
	case roll < 80:
		return inputTokens / 4
	default:
		return inputTokens / 2
	}
}

func buildRequestBody(modelName string, inputTokens int, stream bool) []byte {
	prompt := buildTokenText("prompt", inputTokens)
	payload := map[string]any{
		"model": modelName,
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
		"instructions": "Benchmark this /v1/responses path under realistic load.",
		"reasoning": map[string]any{
			"effort": "low",
		},
		"stream": stream,
	}
	body, _ := json.Marshal(payload)
	return body
}

func mockResponsesHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/responses" {
		http.NotFound(w, r)
		return
	}

	inputTokens := parseHeaderInt(r.Header.Get("X-Lt-Input-Tokens"), 256)
	outputTokens := parseHeaderInt(r.Header.Get("X-Lt-Output-Tokens"), 128)
	cachedTokens := parseHeaderInt(r.Header.Get("X-Lt-Cached-Tokens"), 0)
	ttfb := time.Duration(parseHeaderInt(r.Header.Get("X-Lt-Ttfb-Ms"), 3)) * time.Millisecond
	chunkDelay := time.Duration(parseHeaderInt(r.Header.Get("X-Lt-Chunk-Delay-Ms"), 1)) * time.Millisecond
	scenarioName := r.Header.Get("X-Lt-Scenario")
	modelName := "gpt-4.1-mini"

	var reqBody struct {
		Model string `json:"model"`
	}
	if body, err := io.ReadAll(r.Body); err == nil {
		_ = json.Unmarshal(body, &reqBody)
		if reqBody.Model != "" {
			modelName = reqBody.Model
		}
	}

	time.Sleep(ttfb)

	switch scenarioName {
	case "sse", "aggregate":
		streamResponses(w, modelName, inputTokens, outputTokens, cachedTokens, chunkDelay)
	default:
		writeJSONResponse(w, modelName, inputTokens, outputTokens, cachedTokens, strings.EqualFold(r.Header.Get("X-Lt-Upstream-Gzip"), "true"))
	}
}

func writeJSONResponse(w http.ResponseWriter, modelName string, inputTokens, outputTokens, cachedTokens int, gzipBody bool) {
	payload := buildResponseObject(modelName, inputTokens, outputTokens, cachedTokens)
	encoded, _ := json.Marshal(payload)

	if gzipBody {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(encoded)
		_ = zw.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buf.Bytes())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encoded)
}

func streamResponses(w http.ResponseWriter, modelName string, inputTokens, outputTokens, cachedTokens int, chunkDelay time.Duration) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	outputText := buildTokenText("answer", outputTokens)
	parts := []string{
		outputText[:len(outputText)/3],
		outputText[len(outputText)/3 : (2*len(outputText))/3],
		outputText[(2*len(outputText))/3:],
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	for _, part := range parts {
		frame := fmt.Sprintf("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"%s\"}\n\n", escapeJSONString(part))
		_, _ = io.WriteString(w, frame)
		flusher.Flush()
		time.Sleep(chunkDelay)
	}

	finalPayload, _ := json.Marshal(map[string]any{
		"type":     "response.completed",
		"response": buildResponseObject(modelName, inputTokens, outputTokens, cachedTokens),
	})
	_, _ = fmt.Fprintf(w, "event: response.completed\ndata: %s\n\n", string(finalPayload))
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func buildResponseObject(modelName string, inputTokens, outputTokens, cachedTokens int) map[string]any {
	outputText := buildTokenText("answer", outputTokens)
	return map[string]any{
		"id":         fmt.Sprintf("resp_%d", time.Now().UnixNano()),
		"object":     "response",
		"created_at": time.Now().Unix(),
		"model":      modelName,
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
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
			"total_tokens":  inputTokens + outputTokens,
			"input_tokens_details": map[string]any{
				"cached_tokens": cachedTokens,
			},
		},
	}
}

func validateUsage(contentType string, payload []byte, sc scenario) error {
	if strings.Contains(contentType, "text/event-stream") {
		final, err := extractFinalSSEPayload(payload)
		if err != nil {
			return err
		}
		return compareUsage(final, sc)
	}
	return compareUsage(payload, sc)
}

func compareUsage(payload []byte, sc scenario) error {
	var resp struct {
		Usage struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &resp); err != nil {
		return err
	}
	if resp.Usage.InputTokens != sc.inputTokens {
		return fmt.Errorf("input_tokens=%d want=%d", resp.Usage.InputTokens, sc.inputTokens)
	}
	if resp.Usage.OutputTokens != sc.outputTokens {
		return fmt.Errorf("output_tokens=%d want=%d", resp.Usage.OutputTokens, sc.outputTokens)
	}
	if resp.Usage.InputTokensDetails.CachedTokens != sc.cachedTokens {
		return fmt.Errorf("cached_tokens=%d want=%d", resp.Usage.InputTokensDetails.CachedTokens, sc.cachedTokens)
	}
	return nil
}

func extractFinalSSEPayload(payload []byte) ([]byte, error) {
	lines := strings.Split(string(payload), "\n")
	var eventName string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if data == "[DONE]" {
				continue
			}
			if eventName != "response.completed" {
				continue
			}
			var wrapper struct {
				Response json.RawMessage `json:"response"`
			}
			if err := json.Unmarshal([]byte(data), &wrapper); err != nil {
				return nil, err
			}
			if len(wrapper.Response) == 0 {
				return nil, fmt.Errorf("missing response payload")
			}
			return wrapper.Response, nil
		}
	}
	return nil, fmt.Errorf("missing response.completed frame")
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		return values[0]
	}
	if p >= 1 {
		return values[len(values)-1]
	}
	index := int(math.Ceil(float64(len(values))*p)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func parseHeaderInt(raw string, defaultValue int) int {
	if raw == "" {
		return defaultValue
	}
	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil || value < 0 {
		return defaultValue
	}
	return value
}

func buildTokenText(prefix string, tokens int) string {
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

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func escapeJSONString(value string) string {
	encoded, _ := json.Marshal(value)
	return strings.Trim(string(encoded), "\"")
}
