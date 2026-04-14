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

type benchmarkProfile struct {
	Name                string
	Description         string
	Mode                string
	DefaultStagesRPM    []int
	DefaultStageSeconds int
}

type runOptions struct {
	profileName        string
	label              string
	stageSeconds       int
	stageRPMs          []int
	timeout            time.Duration
	seed               int64
	databaseURL        string
	redisURL           string
	redisPrefix        string
	outputPath         string
	outputFormat       string
	runtimeKnobs       runtimeKnobs
	projectorRequested int
}

type runtimeKnobs struct {
	streamBatchSize    int
	reconcileBatchSize int
	expiryBatchSize    int
	projectorWorkers   int
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
	stageIndex        int
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

type reportMetadata struct {
	Profile                   string       `json:"profile"`
	Label                     string       `json:"label,omitempty"`
	Mode                      string       `json:"mode"`
	Description               string       `json:"description"`
	Model                     string       `json:"model"`
	StageSeconds              int          `json:"stage_seconds"`
	StageRPMs                 []int        `json:"stage_rpms"`
	Timeout                   string       `json:"timeout"`
	Seed                      int64        `json:"seed"`
	TargetURL                 string       `json:"target_url"`
	MockURL                   string       `json:"mock_url"`
	StartedAt                 string       `json:"started_at"`
	CompletedAt               string       `json:"completed_at"`
	OutputFormat              string       `json:"output_format,omitempty"`
	RuntimeKnobs              runtimeKnobs `json:"runtime_knobs"`
	ProjectorWorkersRequested int          `json:"projector_workers_requested,omitempty"`
	ProjectorWorkersApplied   bool         `json:"projector_workers_applied"`
	CompareFields             []string     `json:"compare_fields,omitempty"`
}

type reportSummary struct {
	Stages         int     `json:"stages"`
	TotalRequests  int64   `json:"total_requests"`
	Successes      int64   `json:"successes"`
	Failures       int64   `json:"failures"`
	ErrorRate      float64 `json:"error_rate"`
	AvgThroughput  float64 `json:"avg_throughput_rps"`
	WorstStageRPM  int     `json:"worst_stage_rpm"`
	WorstStageP95  string  `json:"worst_stage_e2e_p95"`
	ValidationFail int64   `json:"validation_failures"`
}

type reportStage struct {
	StageIndex        int      `json:"stage_index"`
	RPM               int      `json:"rpm"`
	Total             int64    `json:"total"`
	Success           int64    `json:"success"`
	Failures          int64    `json:"failures"`
	Elapsed           string   `json:"elapsed"`
	ThroughputRPS     float64  `json:"throughput_rps"`
	ErrorRate         float64  `json:"error_rate"`
	EndToEndP50       string   `json:"e2e_p50"`
	EndToEndP95       string   `json:"e2e_p95"`
	EndToEndP99       string   `json:"e2e_p99"`
	AdmissionP50      string   `json:"admission_p50"`
	AdmissionP95      string   `json:"admission_p95"`
	AdmissionP99      string   `json:"admission_p99"`
	SettleP50         string   `json:"settle_p50"`
	SettleP95         string   `json:"settle_p95"`
	SettleP99         string   `json:"settle_p99"`
	ProjectP50        string   `json:"project_p50"`
	ProjectP95        string   `json:"project_p95"`
	ProjectP99        string   `json:"project_p99"`
	ValidationFail    int64    `json:"validation_failures"`
	AdmissionFailures int64    `json:"admission_failures"`
	SettleFailures    int64    `json:"settle_failures"`
	ProjectFailures   int64    `json:"project_failures"`
	ReconcileFailures int64    `json:"reconcile_failures"`
	ReconcileRepairs  int64    `json:"reconcile_repairs"`
	ErrorSamples      []string `json:"error_samples,omitempty"`
}

type benchmarkReport struct {
	Metadata reportMetadata `json:"metadata"`
	Summary  reportSummary  `json:"summary"`
	Stages   []reportStage  `json:"stages"`
}

var benchmarkProfiles = map[string]benchmarkProfile{
	"benchmark-shared-billing": {
		Name:                "benchmark-shared-billing",
		Description:         "A/B benchmark profile backed by PostgreSQL + Redis shared billing state.",
		Mode:                "shared-billing",
		DefaultStagesRPM:    []int{100, 300, 1000, 3000, 6000, 10000},
		DefaultStageSeconds: 8,
	},
	"benchmark-legacy-local": {
		Name:                "benchmark-legacy-local",
		Description:         "Self-contained sqlite benchmark profile for local comparison baselines only.",
		Mode:                "legacy-local",
		DefaultStagesRPM:    []int{100, 300, 1000, 3000, 6000, 10000},
		DefaultStageSeconds: 8,
	},
	"smoke-shared-billing": {
		Name:                "smoke-shared-billing",
		Description:         "Shared billing smoke profile for quick verification before wider benchmark runs.",
		Mode:                "shared-billing",
		DefaultStagesRPM:    []int{100, 300},
		DefaultStageSeconds: 4,
	},
	"smoke-legacy-local": {
		Name:                "smoke-legacy-local",
		Description:         "Local smoke profile without external billing dependencies.",
		Mode:                "legacy-local",
		DefaultStagesRPM:    []int{100, 300},
		DefaultStageSeconds: 4,
	},
}

func main() {
	var (
		stageSeconds       int
		timeout            time.Duration
		seed               int64
		databaseURL        string
		redisURL           string
		redisPrefix        string
		legacyLocal        bool
		profileName        string
		stageList          string
		label              string
		outputPath         string
		outputFormat       string
		streamBatchSize    int
		reconcileBatchSize int
		expiryBatchSize    int
		projectorWorkers   int
	)

	flag.IntVar(&stageSeconds, "stage-seconds", 8, "duration of each RPM stage in seconds")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "per-request timeout")
	flag.Int64Var(&seed, "seed", 42, "random seed")
	flag.StringVar(&profileName, "profile", "benchmark-shared-billing", "benchmark profile: benchmark-shared-billing, benchmark-legacy-local, smoke-shared-billing, smoke-legacy-local")
	flag.StringVar(&stageList, "stages", "", "comma-separated RPM stage list; overrides profile defaults")
	flag.StringVar(&label, "label", "", "optional run label for A/B result comparison")
	flag.StringVar(&databaseURL, "database-url", os.Getenv("DATABASE_URL"), "postgres database url for shared billing benchmark")
	flag.StringVar(&redisURL, "redis-url", os.Getenv("REDIS_URL"), "redis url for shared billing benchmark")
	flag.StringVar(&redisPrefix, "redis-prefix", os.Getenv("REDIS_PREFIX"), "redis key prefix for shared billing benchmark")
	flag.StringVar(&outputPath, "output", "", "optional path to persist the benchmark report")
	flag.StringVar(&outputFormat, "output-format", "json", "report format for -output; supported: json")
	flag.IntVar(&streamBatchSize, "stream-batch-size", 100, "billing runtime stream batch size for shared billing mode")
	flag.IntVar(&reconcileBatchSize, "reconcile-batch-size", 0, "billing runtime reconcile batch size for shared billing mode; defaults to stream batch size when <= 0")
	flag.IntVar(&expiryBatchSize, "expiry-batch-size", 0, "billing runtime expiry batch size for shared billing mode; defaults to stream batch size when <= 0")
	flag.IntVar(&projectorWorkers, "projector-workers", 0, "billing runtime projector worker count for shared billing mode; defaults to runtime default when <= 0")
	flag.BoolVar(&legacyLocal, "legacy-local", false, "use self-contained sqlite smoke mode instead of real shared billing")
	flag.Parse()

	log.SetOutput(io.Discard)
	log.SetLevel(log.ErrorLevel)
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard

	options, profile, err := buildRunOptions(profileName, stageSeconds, stageList, timeout, seed, databaseURL, redisURL, redisPrefix, label, outputPath, outputFormat, legacyLocal, runtimeKnobs{
		streamBatchSize:    streamBatchSize,
		reconcileBatchSize: reconcileBatchSize,
		expiryBatchSize:    expiryBatchSize,
		projectorWorkers:   projectorWorkers,
	}, projectorWorkers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid options: %v\n", err)
		os.Exit(1)
	}

	startedAt := time.Now().UTC()
	env, err := setupBenchmarkEnv(profile.Mode, options.timeout, options.seed, options.databaseURL, options.redisURL, options.redisPrefix, options.runtimeKnobs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup failed: %v\n", err)
		os.Exit(1)
	}
	defer env.cleanup()

	fmt.Printf("loadtest profile=%s label=%s mode=%s target=%s mock=%s model=%s stage_duration=%ds\n",
		profile.Name,
		displayValue(options.label, "-"),
		env.mode,
		env.appURL,
		env.mockURL,
		env.model,
		options.stageSeconds,
	)
	if env.mode == "shared-billing" {
		fmt.Printf("profile_desc=%s\n", profile.Description)
		fmt.Printf("billing path enabled with Redis + PostgreSQL, metrics include admission/settle/projector\n")
	} else {
		fmt.Printf("profile_desc=%s\n", profile.Description)
		fmt.Printf("legacy-local mode: only for smoke verification, not representative of real billing throughput\n")
	}
	fmt.Printf("compare_fields=profile,label,mode,stage_rpm,e2e_p95,e2e_p99,throughput_rps,error_rate,admission_p95,settle_p95,project_p95\n")
	fmt.Printf("stage_plan=%s\n", joinRPMs(options.stageRPMs))
	fmt.Printf(
		"runtime_knobs=stream_batch_size:%d reconcile_batch_size:%d expiry_batch_size:%d projector_workers_requested:%s projector_workers_applied:%t\n",
		options.runtimeKnobs.streamBatchSize,
		options.runtimeKnobs.reconcileBatchSize,
		options.runtimeKnobs.expiryBatchSize,
		displayOptionalInt(options.projectorRequested),
		options.projectorRequested > 0,
	)
	printStageTableHeader()

	overallOK := true
	stageResults := make([]stageMetrics, 0, len(options.stageRPMs))

	for idx, rpm := range options.stageRPMs {
		metrics := runStage(env, rpm, time.Duration(options.stageSeconds)*time.Second)
		metrics.stageIndex = idx + 1
		stageResults = append(stageResults, metrics)
		printStageTableRow(profile.Name, options.label, metrics)
		for _, sample := range metrics.errorSamples {
			fmt.Printf("  error_sample: %s\n", sample)
		}
		if metrics.failures > 0 {
			overallOK = false
		}
	}

	completedAt := time.Now().UTC()
	report := buildBenchmarkReport(profile, options, env, startedAt, completedAt, stageResults)
	printSummary(report.Summary)
	if options.outputPath != "" {
		if err := writeReport(options.outputPath, options.outputFormat, report); err != nil {
			fmt.Fprintf(os.Stderr, "write report failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("report_written=%s format=%s\n", options.outputPath, options.outputFormat)
	}

	if overallOK {
		fmt.Println("result: all stages completed without request or validation failures")
		return
	}

	fmt.Println("result: one or more stages reported failures")
	os.Exit(1)
}

func buildRunOptions(profileName string, stageSeconds int, stageList string, timeout time.Duration, seed int64, databaseURL, redisURL, redisPrefix, label, outputPath, outputFormat string, legacyLocal bool, knobs runtimeKnobs, projectorWorkers int) (runOptions, benchmarkProfile, error) {
	profile, err := resolveProfile(profileName, legacyLocal)
	if err != nil {
		return runOptions{}, benchmarkProfile{}, err
	}
	if stageSeconds <= 0 {
		stageSeconds = profile.DefaultStageSeconds
	}
	stageRPMs, err := parseStages(stageList, profile.DefaultStagesRPM)
	if err != nil {
		return runOptions{}, benchmarkProfile{}, err
	}
	outputFormat = strings.ToLower(strings.TrimSpace(outputFormat))
	if outputPath != "" && outputFormat != "json" {
		return runOptions{}, benchmarkProfile{}, fmt.Errorf("unsupported output format %q", outputFormat)
	}
	knobs = normalizeRuntimeKnobs(knobs)
	if projectorWorkers < 0 {
		return runOptions{}, benchmarkProfile{}, fmt.Errorf("projector workers must be >= 0")
	}
	return runOptions{
		profileName:        profile.Name,
		label:              strings.TrimSpace(label),
		stageSeconds:       stageSeconds,
		stageRPMs:          stageRPMs,
		timeout:            timeout,
		seed:               seed,
		databaseURL:        databaseURL,
		redisURL:           redisURL,
		redisPrefix:        redisPrefix,
		outputPath:         strings.TrimSpace(outputPath),
		outputFormat:       outputFormat,
		runtimeKnobs:       knobs,
		projectorRequested: projectorWorkers,
	}, profile, nil
}

func normalizeRuntimeKnobs(knobs runtimeKnobs) runtimeKnobs {
	if knobs.streamBatchSize <= 0 {
		knobs.streamBatchSize = 100
	}
	if knobs.reconcileBatchSize <= 0 {
		knobs.reconcileBatchSize = knobs.streamBatchSize
	}
	if knobs.expiryBatchSize <= 0 {
		knobs.expiryBatchSize = knobs.streamBatchSize
	}
	if knobs.projectorWorkers <= 0 {
		knobs.projectorWorkers = 1
	}
	return knobs
}

func resolveProfile(profileName string, legacyLocal bool) (benchmarkProfile, error) {
	if legacyLocal && strings.TrimSpace(profileName) == "benchmark-shared-billing" {
		profileName = "benchmark-legacy-local"
	}
	switch strings.TrimSpace(profileName) {
	case "shared-billing":
		profileName = "benchmark-shared-billing"
	case "legacy-local":
		profileName = "benchmark-legacy-local"
	case "smoke":
		if legacyLocal {
			profileName = "smoke-legacy-local"
		} else {
			profileName = "smoke-shared-billing"
		}
	}
	profile, ok := benchmarkProfiles[profileName]
	if !ok {
		return benchmarkProfile{}, fmt.Errorf("unknown profile %q", profileName)
	}
	return profile, nil
}

func parseStages(raw string, defaults []int) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]int(nil), defaults...), nil
	}
	parts := strings.Split(raw, ",")
	stages := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var rpm int
		if _, err := fmt.Sscanf(part, "%d", &rpm); err != nil || rpm <= 0 {
			return nil, fmt.Errorf("invalid stage rpm %q", part)
		}
		stages = append(stages, rpm)
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("no valid stages configured")
	}
	return stages, nil
}

func setupBenchmarkEnv(mode string, timeout time.Duration, seed int64, databaseURL, redisURL, redisPrefix string, knobs runtimeKnobs) (*benchmarkEnv, error) {
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

	cfg := config.Load()
	if mode == "shared-billing" {
		if databaseURL == "" || redisURL == "" {
			mock.Close()
			_ = os.RemoveAll(tempDir)
			return nil, fmt.Errorf("shared billing benchmark requires both -database-url and -redis-url")
		}
	}
	if mode == "shared-billing" {
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
			RedisURL:           cfg.RedisURL,
			Prefix:             cfg.RedisPrefix,
			ReservationTTL:     10 * time.Minute,
			ReconcileInterval:  time.Minute,
			StreamBatchSize:    int64(knobs.streamBatchSize),
			ReconcileBatchSize: int64(knobs.reconcileBatchSize),
			ExpiryBatchSize:    int64(knobs.expiryBatchSize),
			ProjectorWorkers:   knobs.projectorWorkers,
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

func buildBenchmarkReport(profile benchmarkProfile, options runOptions, env *benchmarkEnv, startedAt, completedAt time.Time, stages []stageMetrics) benchmarkReport {
	reportStages := make([]reportStage, 0, len(stages))
	var totalRequests int64
	var successes int64
	var failures int64
	var validationFailures int64
	var throughputSum float64
	worstStageRPM := 0
	worstStageP95 := time.Duration(0)

	for _, stage := range stages {
		totalRequests += stage.total
		successes += stage.success
		failures += stage.failures
		validationFailures += stage.validationFail
		throughputSum += stage.throughputRPS
		if stage.endToEndP95 > worstStageP95 {
			worstStageP95 = stage.endToEndP95
			worstStageRPM = stage.rpm
		}
		reportStages = append(reportStages, reportStage{
			StageIndex:        stage.stageIndex,
			RPM:               stage.rpm,
			Total:             stage.total,
			Success:           stage.success,
			Failures:          stage.failures,
			Elapsed:           stage.elapsed.Round(time.Millisecond).String(),
			ThroughputRPS:     roundFloat(stage.throughputRPS, 2),
			ErrorRate:         roundFloat(stage.errorRate, 6),
			EndToEndP50:       roundDuration(stage.endToEndP50),
			EndToEndP95:       roundDuration(stage.endToEndP95),
			EndToEndP99:       roundDuration(stage.endToEndP99),
			AdmissionP50:      roundDuration(stage.admissionP50),
			AdmissionP95:      roundDuration(stage.admissionP95),
			AdmissionP99:      roundDuration(stage.admissionP99),
			SettleP50:         roundDuration(stage.settleP50),
			SettleP95:         roundDuration(stage.settleP95),
			SettleP99:         roundDuration(stage.settleP99),
			ProjectP50:        roundDuration(stage.projectP50),
			ProjectP95:        roundDuration(stage.projectP95),
			ProjectP99:        roundDuration(stage.projectP99),
			ValidationFail:    stage.validationFail,
			AdmissionFailures: stage.admissionFailures,
			SettleFailures:    stage.settleFailures,
			ProjectFailures:   stage.projectFailures,
			ReconcileFailures: stage.reconcileFailures,
			ReconcileRepairs:  stage.reconcileRepairs,
			ErrorSamples:      append([]string(nil), stage.errorSamples...),
		})
	}

	avgThroughput := 0.0
	if len(stages) > 0 {
		avgThroughput = throughputSum / float64(len(stages))
	}

	errorRate := 0.0
	if totalRequests > 0 {
		errorRate = float64(failures) / float64(totalRequests)
	}

	return benchmarkReport{
		Metadata: reportMetadata{
			Profile:                   profile.Name,
			Label:                     options.label,
			Mode:                      env.mode,
			Description:               profile.Description,
			Model:                     env.model,
			StageSeconds:              options.stageSeconds,
			StageRPMs:                 append([]int(nil), options.stageRPMs...),
			Timeout:                   options.timeout.String(),
			Seed:                      options.seed,
			TargetURL:                 env.appURL,
			MockURL:                   env.mockURL,
			StartedAt:                 startedAt.Format(time.RFC3339),
			CompletedAt:               completedAt.Format(time.RFC3339),
			OutputFormat:              options.outputFormat,
			RuntimeKnobs:              options.runtimeKnobs,
			ProjectorWorkersRequested: options.projectorRequested,
			ProjectorWorkersApplied:   options.projectorRequested > 0,
			CompareFields:             []string{"profile", "label", "mode", "stage_rpm", "e2e_p95", "e2e_p99", "throughput_rps", "error_rate", "admission_p95", "settle_p95", "project_p95"},
		},
		Summary: reportSummary{
			Stages:         len(stages),
			TotalRequests:  totalRequests,
			Successes:      successes,
			Failures:       failures,
			ErrorRate:      roundFloat(errorRate, 6),
			AvgThroughput:  roundFloat(avgThroughput, 2),
			WorstStageRPM:  worstStageRPM,
			WorstStageP95:  roundDuration(worstStageP95),
			ValidationFail: validationFailures,
		},
		Stages: reportStages,
	}
}

func writeReport(path, format string, report benchmarkReport) error {
	if format != "json" {
		return fmt.Errorf("unsupported output format %q", format)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(path, encoded, 0o644)
}

func printStageTableHeader() {
	fmt.Printf("%-26s %-16s %3s %6s %6s %6s %7s %9s %9s %9s %9s %9s %9s %6s %6s %6s %6s %6s %6s\n",
		"profile", "label", "#", "rpm", "ok", "err", "err%", "rps", "e2e_p95", "e2e_p99", "adm_p95", "set_p95", "prj_p95", "val", "resv", "stl", "prj", "recon", "fix")
}

func printStageTableRow(profileName, label string, metrics stageMetrics) {
	fmt.Printf("%-26s %-16s %3d %6d %6d %6d %6.2f %9.1f %9s %9s %9s %9s %9s %6d %6d %6d %6d %6d %6d\n",
		profileName,
		displayValue(label, "-"),
		metrics.stageIndex,
		metrics.rpm,
		metrics.success,
		metrics.failures,
		metrics.errorRate*100,
		metrics.throughputRPS,
		roundDuration(metrics.endToEndP95),
		roundDuration(metrics.endToEndP99),
		roundDuration(metrics.admissionP95),
		roundDuration(metrics.settleP95),
		roundDuration(metrics.projectP95),
		metrics.validationFail,
		metrics.admissionFailures,
		metrics.settleFailures,
		metrics.projectFailures,
		metrics.reconcileFailures,
		metrics.reconcileRepairs,
	)
}

func printSummary(summary reportSummary) {
	fmt.Printf("summary stages=%d total=%d ok=%d err=%d err_rate=%.2f%% avg_throughput=%.1f rps worst_stage_rpm=%d worst_e2e_p95=%s validation_err=%d\n",
		summary.Stages,
		summary.TotalRequests,
		summary.Successes,
		summary.Failures,
		summary.ErrorRate*100,
		summary.AvgThroughput,
		summary.WorstStageRPM,
		summary.WorstStageP95,
		summary.ValidationFail,
	)
}

func joinRPMs(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	return strings.Join(parts, ",")
}

func displayValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func displayOptionalInt(value int) string {
	if value <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d", value)
}

func roundDuration(value time.Duration) string {
	return value.Round(time.Millisecond).String()
}

func roundFloat(value float64, places int) float64 {
	if places < 0 {
		return value
	}
	factor := math.Pow(10, float64(places))
	return math.Round(value*factor) / factor
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
