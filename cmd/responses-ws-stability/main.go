package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"nhooyr.io/websocket"
)

type config struct {
	ManagerBaseURL       string
	AdminUsername        string
	AdminPassword        string
	ResponsesBaseURL     string
	ResponsesAPIKey      string
	ResponsesModel       string
	ResponsesHeaders     map[string]string
	Prefix               string
	DownstreamCustomKey  string
	TopUpUSD             float64
	SingleTurnSamples    int
	ContinuationSessions int
	LongIdleSessions     int
	LongIdleSeconds      int
	ConcurrencyLevels    []int
	RequestTimeout       time.Duration
	LogFlushDelay        time.Duration
}

type authResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Token    string `json:"token"`
	IsAdmin  bool   `json:"isAdmin"`
	Message  string `json:"message"`
}

type ampSettings struct {
	UpstreamURL     string         `json:"upstreamUrl"`
	ModelMappings   []modelMapping `json:"modelMappings"`
	Enabled         bool           `json:"enabled"`
	NativeMode      bool           `json:"nativeMode"`
	WebSearchMode   string         `json:"webSearchMode"`
	ShowBalanceInAd bool           `json:"showBalanceInAd"`
}

type modelMapping struct {
	From                      string   `json:"from"`
	To                        string   `json:"to"`
	ChannelID                 string   `json:"channelId,omitempty"`
	Regex                     bool     `json:"regex"`
	ThinkingLevel             string   `json:"thinkingLevel,omitempty"`
	PseudoNonStream           bool     `json:"pseudoNonStream,omitempty"`
	AuditKeywords             []string `json:"auditKeywords,omitempty"`
	AmpOnly                   bool     `json:"ampOnly,omitempty"`
	FastMode                  bool     `json:"fastMode,omitempty"`
	CustomInstructions        string   `json:"customInstructions,omitempty"`
	CustomInstructionsEnabled bool     `json:"customInstructionsEnabled,omitempty"`
}

type createAPIKeyResponse struct {
	ID      string `json:"id"`
	APIKey  string `json:"apiKey"`
	Message string `json:"message"`
}

type channel struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Type                  string            `json:"type"`
	Endpoint              string            `json:"endpoint"`
	BaseURL               string            `json:"baseUrl"`
	Enabled               bool              `json:"enabled"`
	Weight                int               `json:"weight"`
	Priority              int               `json:"priority"`
	CodexWebsocketEnabled bool              `json:"codexWebsocketEnabled"`
	Headers               map[string]string `json:"headers"`
}

type channelListResponse struct {
	Channels []channel `json:"channels"`
}

type requestLogList struct {
	Items []requestLog `json:"items"`
	Total int          `json:"total"`
	Page  int          `json:"page"`
}

type requestLog struct {
	ID                      string `json:"id"`
	OriginalModel           string `json:"originalModel"`
	MappedModel             string `json:"mappedModel"`
	StatusCode              int    `json:"statusCode"`
	LatencyMs               int64  `json:"latencyMs"`
	TTFBMs                  *int64 `json:"ttfbMs"`
	DownstreamTransport     string `json:"downstreamTransport"`
	UpstreamTransport       string `json:"upstreamTransport"`
	TransportFallbackReason string `json:"transportFallbackReason"`
	ResponseText            string `json:"outputPreview"`
}

type scenarioConfig struct {
	Name             string
	ModelAlias       string
	ExpectedUpstream string
	SampleCount      int
	Continuation     bool
	LongIdleSeconds  int
	Concurrency      int
}

type sample struct {
	Scenario              string    `json:"scenario"`
	Model                 string    `json:"model"`
	StartedAt             time.Time `json:"startedAt"`
	FinishedAt            time.Time `json:"finishedAt"`
	ClientTTFBMs          int64     `json:"clientTtfbMs"`
	ClientLatencyMs       int64     `json:"clientLatencyMs"`
	ClientFrames          int       `json:"clientFrames"`
	ClientEventType       string    `json:"clientEventType"`
	ClientError           string    `json:"clientError,omitempty"`
	ContinuationSucceeded bool      `json:"continuationSucceeded,omitempty"`
	ResponseTexts         []string  `json:"responseTexts,omitempty"`
}

type scenarioResult struct {
	Config              scenarioConfig `json:"config"`
	Samples             []sample       `json:"samples"`
	RequestLogs         []requestLog   `json:"requestLogs"`
	ClientSuccesses     int            `json:"clientSuccesses"`
	ClientFailures      int            `json:"clientFailures"`
	LogCount            int            `json:"logCount"`
	MissingLogs         int            `json:"missingLogs"`
	FallbackCount       int            `json:"fallbackCount"`
	P50TTFBMs           int64          `json:"p50TtfbMs"`
	P95TTFBMs           int64          `json:"p95TtfbMs"`
	P50LatencyMs        int64          `json:"p50LatencyMs"`
	P95LatencyMs        int64          `json:"p95LatencyMs"`
	UnexpectedTransport int            `json:"unexpectedTransport"`
}

type runSummary struct {
	StartedAt        time.Time        `json:"startedAt"`
	ManagerBaseURL   string           `json:"managerBaseUrl"`
	ResponsesBaseURL string           `json:"responsesBaseUrl"`
	ResponsesModel   string           `json:"responsesModel"`
	Prefix           string           `json:"prefix"`
	Scenarios        []scenarioResult `json:"scenarios"`
	Passed           bool             `json:"passed"`
	OutputDir        string           `json:"outputDir"`
}

type client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fail(err)
	}

	runStartedAt := time.Now().UTC()
	outputDir := filepath.Join(".tmp", "ws-stability", runStartedAt.Format("20060102-150405"))
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fail(err)
	}

	cl := &client{
		baseURL: strings.TrimRight(cfg.ManagerBaseURL, "/"),
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}

	auth, err := cl.login(cfg.AdminUsername, cfg.AdminPassword)
	if err != nil {
		fail(err)
	}
	cl.token = auth.Token

	if err := cl.topUp(auth.ID, cfg.TopUpUSD); err != nil {
		fail(err)
	}
	if _, err := cl.ensureAPIKey(cfg.DownstreamCustomKey); err != nil {
		fail(err)
	}

	httpChannel, wsChannel, err := cl.ensureChannels(cfg)
	if err != nil {
		fail(err)
	}
	if err := cl.ensureAmpSettings(cfg, httpChannel.ID, wsChannel.ID); err != nil {
		fail(err)
	}

	scenarios := buildScenarios(cfg)
	results := make([]scenarioResult, 0, len(scenarios))
	allSamplesPath := filepath.Join(outputDir, "samples.jsonl")
	samplesFile, err := os.Create(allSamplesPath)
	if err != nil {
		fail(err)
	}
	defer samplesFile.Close()

	passed := true
	for _, scenario := range scenarios {
		result, err := runScenario(cfg, cl, scenario)
		if err != nil {
			fail(err)
		}
		for _, sample := range result.Samples {
			line, _ := json.Marshal(sample)
			_, _ = samplesFile.Write(append(line, '\n'))
		}
		if result.ClientFailures > 0 || result.UnexpectedTransport > 0 || result.MissingLogs > 0 {
			passed = false
		}
		results = append(results, result)
	}

	summary := runSummary{
		StartedAt:        runStartedAt,
		ManagerBaseURL:   cfg.ManagerBaseURL,
		ResponsesBaseURL: cfg.ResponsesBaseURL,
		ResponsesModel:   cfg.ResponsesModel,
		Prefix:           cfg.Prefix,
		Scenarios:        results,
		Passed:           passed,
		OutputDir:        outputDir,
	}

	writeJSON(filepath.Join(outputDir, "summary.json"), summary)
	writeText(filepath.Join(outputDir, "report.md"), renderReport(summary))

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(summary)

	if !passed {
		os.Exit(1)
	}
}

func loadConfig() (*config, error) {
	headers := map[string]string{}
	if raw := strings.TrimSpace(os.Getenv("RESPONSES_HEADERS_JSON")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &headers); err != nil {
			return nil, fmt.Errorf("invalid RESPONSES_HEADERS_JSON: %w", err)
		}
	}

	levels, err := parseIntList(getEnvDefault("STABILITY_CONCURRENCY_LEVELS", "1,5,20"))
	if err != nil {
		return nil, fmt.Errorf("invalid STABILITY_CONCURRENCY_LEVELS: %w", err)
	}

	topUpUSD, err := strconv.ParseFloat(getEnvDefault("STABILITY_TOPUP_USD", "5"), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid STABILITY_TOPUP_USD: %w", err)
	}

	cfg := &config{
		ManagerBaseURL:       getEnvDefault("AMP_MANAGER_BASE_URL", "http://127.0.0.1:16823"),
		AdminUsername:        os.Getenv("AMP_ADMIN_USERNAME"),
		AdminPassword:        os.Getenv("AMP_ADMIN_PASSWORD"),
		ResponsesBaseURL:     os.Getenv("RESPONSES_BASE_URL"),
		ResponsesAPIKey:      os.Getenv("RESPONSES_API_KEY"),
		ResponsesModel:       os.Getenv("RESPONSES_MODEL"),
		ResponsesHeaders:     headers,
		Prefix:               getEnvDefault("STABILITY_PREFIX", "responses-ws-stability"),
		DownstreamCustomKey:  getEnvDefault("STABILITY_CUSTOM_DOWNSTREAM_KEY", "ResponsesWSStabilityKey123456"),
		TopUpUSD:             topUpUSD,
		SingleTurnSamples:    mustParseInt("STABILITY_SINGLE_TURN_SAMPLES", "20"),
		ContinuationSessions: mustParseInt("STABILITY_CONTINUATION_SESSIONS", "10"),
		LongIdleSessions:     mustParseInt("STABILITY_LONG_IDLE_SESSIONS", "5"),
		LongIdleSeconds:      mustParseInt("STABILITY_LONG_IDLE_SECONDS", "30"),
		ConcurrencyLevels:    levels,
		RequestTimeout:       90 * time.Second,
		LogFlushDelay:        600 * time.Millisecond,
	}

	for key, value := range map[string]string{
		"AMP_ADMIN_USERNAME": cfg.AdminUsername,
		"AMP_ADMIN_PASSWORD": cfg.AdminPassword,
		"RESPONSES_BASE_URL": cfg.ResponsesBaseURL,
		"RESPONSES_API_KEY":  cfg.ResponsesAPIKey,
		"RESPONSES_MODEL":    cfg.ResponsesModel,
	} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("missing required environment variable %s", key)
		}
	}

	return cfg, nil
}

func buildScenarios(cfg *config) []scenarioConfig {
	scenarios := []scenarioConfig{
		{
			Name:             "single-turn-http-baseline",
			ModelAlias:       cfg.Prefix + "-http",
			ExpectedUpstream: "http",
			SampleCount:      cfg.SingleTurnSamples,
		},
		{
			Name:             "single-turn-ws",
			ModelAlias:       cfg.Prefix + "-ws",
			ExpectedUpstream: "websocket",
			SampleCount:      cfg.SingleTurnSamples,
		},
		{
			Name:             "continuation-ws",
			ModelAlias:       cfg.Prefix + "-ws",
			ExpectedUpstream: "websocket",
			SampleCount:      cfg.ContinuationSessions,
			Continuation:     true,
		},
		{
			Name:             "long-idle-ws",
			ModelAlias:       cfg.Prefix + "-ws",
			ExpectedUpstream: "websocket",
			SampleCount:      cfg.LongIdleSessions,
			Continuation:     true,
			LongIdleSeconds:  cfg.LongIdleSeconds,
		},
	}
	for _, level := range cfg.ConcurrencyLevels {
		if level <= 0 {
			continue
		}
		scenarios = append(scenarios, scenarioConfig{
			Name:             fmt.Sprintf("concurrency-ws-%d", level),
			ModelAlias:       cfg.Prefix + "-ws",
			ExpectedUpstream: "websocket",
			SampleCount:      level,
			Concurrency:      level,
		})
	}
	return scenarios
}

func runScenario(cfg *config, cl *client, scenario scenarioConfig) (scenarioResult, error) {
	startedAt := time.Now().UTC()
	result := scenarioResult{Config: scenario}

	switch {
	case scenario.Concurrency > 0:
		samples := make([]sample, scenario.Concurrency)
		var wg sync.WaitGroup
		var mu sync.Mutex
		var firstErr error
		wg.Add(scenario.Concurrency)
		for i := 0; i < scenario.Concurrency; i++ {
			go func(idx int) {
				defer wg.Done()
				s, err := cl.runSingleTurnSample(cfg.DownstreamCustomKey, scenario.ModelAlias, scenario.Name)
				if err != nil {
					s.ClientError = err.Error()
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
				samples[idx] = s
			}(i)
		}
		wg.Wait()
		_ = firstErr
		result.Samples = samples
	case scenario.Continuation:
		for i := 0; i < scenario.SampleCount; i++ {
			s, err := cl.runContinuationSample(cfg.DownstreamCustomKey, scenario.ModelAlias, scenario.Name, scenario.LongIdleSeconds)
			if err != nil {
				s.ClientError = err.Error()
			}
			result.Samples = append(result.Samples, s)
		}
	default:
		for i := 0; i < scenario.SampleCount; i++ {
			s, err := cl.runSingleTurnSample(cfg.DownstreamCustomKey, scenario.ModelAlias, scenario.Name)
			if err != nil {
				s.ClientError = err.Error()
			}
			result.Samples = append(result.Samples, s)
		}
	}

	for _, s := range result.Samples {
		if s.ClientError == "" {
			result.ClientSuccesses++
		} else {
			result.ClientFailures++
		}
	}

	time.Sleep(cfg.LogFlushDelay)
	logs, err := cl.fetchAllAdminRequestLogs(scenario.ModelAlias, startedAt.Add(-2*time.Second))
	if err != nil {
		return result, err
	}
	result.RequestLogs = logs
	result.LogCount = len(logs)
	if result.LogCount < result.ClientSuccesses {
		result.MissingLogs = result.ClientSuccesses - result.LogCount
	}

	var ttfbValues, latencyValues []int64
	for _, logItem := range logs {
		if logItem.UpstreamTransport != scenario.ExpectedUpstream {
			result.UnexpectedTransport++
		}
		if strings.TrimSpace(logItem.TransportFallbackReason) != "" {
			result.FallbackCount++
		}
		if logItem.TTFBMs != nil {
			ttfbValues = append(ttfbValues, *logItem.TTFBMs)
		}
		latencyValues = append(latencyValues, logItem.LatencyMs)
	}
	result.P50TTFBMs = percentile(ttfbValues, 50)
	result.P95TTFBMs = percentile(ttfbValues, 95)
	result.P50LatencyMs = percentile(latencyValues, 50)
	result.P95LatencyMs = percentile(latencyValues, 95)

	return result, nil
}

func (c *client) login(username, password string) (*authResponse, error) {
	var out authResponse
	if err := c.doJSON(http.MethodPost, "/api/manage/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) topUp(userID string, amountUSD float64) error {
	return c.doJSON(http.MethodPost, "/api/admin/users/"+url.PathEscape(userID)+"/topup", map[string]float64{
		"amountUsd": amountUSD,
	}, c.token, nil)
}

func (c *client) ensureAPIKey(customKey string) (*createAPIKeyResponse, error) {
	var out createAPIKeyResponse
	err := c.doJSON(http.MethodPost, "/api/me/amp/api-keys", map[string]string{
		"name":      "responses-ws-stability",
		"customKey": customKey,
	}, c.token, &out)
	if err == nil {
		return &out, nil
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		return &createAPIKeyResponse{APIKey: customKey}, nil
	}
	return nil, err
}

func (c *client) ensureChannels(cfg *config) (*channel, *channel, error) {
	list, err := c.listChannels()
	if err != nil {
		return nil, nil, err
	}

	httpName := cfg.Prefix + "-http"
	wsName := cfg.Prefix + "-ws"
	httpPayload := map[string]any{
		"type":                  "openai",
		"endpoint":              "responses",
		"name":                  httpName,
		"baseUrl":               cfg.ResponsesBaseURL,
		"apiKey":                cfg.ResponsesAPIKey,
		"enabled":               true,
		"weight":                1,
		"priority":              100,
		"groupIds":              []string{},
		"models":                []map[string]string{{"name": cfg.ResponsesModel, "alias": ""}},
		"modelWhitelist":        false,
		"codexWebsocketEnabled": false,
		"headers":               cfg.ResponsesHeaders,
	}
	wsPayload := map[string]any{
		"type":                  "openai",
		"endpoint":              "responses",
		"name":                  wsName,
		"baseUrl":               cfg.ResponsesBaseURL,
		"apiKey":                cfg.ResponsesAPIKey,
		"enabled":               true,
		"weight":                1,
		"priority":              100,
		"groupIds":              []string{},
		"models":                []map[string]string{{"name": cfg.ResponsesModel, "alias": ""}},
		"modelWhitelist":        false,
		"codexWebsocketEnabled": true,
		"headers":               cfg.ResponsesHeaders,
	}

	httpChannel, err := c.upsertChannel(findChannelByName(list, httpName), httpPayload)
	if err != nil {
		return nil, nil, err
	}
	wsChannel, err := c.upsertChannel(findChannelByName(list, wsName), wsPayload)
	if err != nil {
		return nil, nil, err
	}
	return httpChannel, wsChannel, nil
}

func (c *client) ensureAmpSettings(cfg *config, httpChannelID, wsChannelID string) error {
	var current ampSettings
	if err := c.doJSON(http.MethodGet, "/api/me/amp/settings", nil, c.token, &current); err != nil {
		return err
	}

	filtered := make([]modelMapping, 0, len(current.ModelMappings)+2)
	httpAlias := cfg.Prefix + "-http"
	wsAlias := cfg.Prefix + "-ws"
	for _, mapping := range current.ModelMappings {
		if mapping.From == httpAlias || mapping.From == wsAlias {
			continue
		}
		filtered = append(filtered, mapping)
	}
	filtered = append(filtered,
		modelMapping{From: httpAlias, To: cfg.ResponsesModel, ChannelID: httpChannelID, Regex: false},
		modelMapping{From: wsAlias, To: cfg.ResponsesModel, ChannelID: wsChannelID, Regex: false},
	)

	upstreamURL := current.UpstreamURL
	if strings.TrimSpace(upstreamURL) == "" {
		upstreamURL = cfg.ResponsesBaseURL
	}
	webSearchMode := current.WebSearchMode
	if strings.TrimSpace(webSearchMode) == "" {
		webSearchMode = "upstream"
	}

	payload := map[string]any{
		"upstreamUrl":     upstreamURL,
		"enabled":         true,
		"nativeMode":      false,
		"webSearchMode":   webSearchMode,
		"showBalanceInAd": current.ShowBalanceInAd,
		"modelMappings":   filtered,
	}
	return c.doJSON(http.MethodPut, "/api/me/amp/settings", payload, c.token, nil)
}

func (c *client) runSingleTurnSample(downstreamKey, modelAlias, scenario string) (sample, error) {
	return c.runWebsocketConversation(downstreamKey, modelAlias, scenario, false, 0)
}

func (c *client) runContinuationSample(downstreamKey, modelAlias, scenario string, idleSeconds int) (sample, error) {
	return c.runWebsocketConversation(downstreamKey, modelAlias, scenario, true, idleSeconds)
}

func (c *client) runWebsocketConversation(downstreamKey, modelAlias, scenario string, continuation bool, idleSeconds int) (sample, error) {
	startedAt := time.Now().UTC()
	s := sample{
		Scenario:  scenario,
		Model:     modelAlias,
		StartedAt: startedAt,
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.httpClient.Timeout)
	defer cancel()

	wsURL, err := websocketURL(c.baseURL + "/v1/responses")
	if err != nil {
		return s, err
	}
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + downstreamKey},
		},
	})
	if err != nil {
		return s, err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	firstStart := time.Now()
	req := []byte(`{"type":"response.create","model":"` + modelAlias + `","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"stability probe"}]}]}`)
	if err := conn.Write(ctx, websocket.MessageText, req); err != nil {
		return s, err
	}

	firstResp, firstTTFB, err := readUntilCompleted(ctx, conn, firstStart)
	if err != nil {
		return s, err
	}
	s.ClientTTFBMs = firstTTFB.Milliseconds()
	s.ClientFrames++
	s.ResponseTexts = append(s.ResponseTexts, responseTextFromPayload(firstResp))

	if continuation {
		if idleSeconds > 0 {
			time.Sleep(time.Duration(idleSeconds) * time.Second)
		}
		responseID := gjson.GetBytes(firstResp, "response.id").String()
		appendReq := []byte(`{"type":"response.append","previous_response_id":"` + responseID + `","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"continuation probe"}]}]}`)
		secondStart := time.Now()
		if err := conn.Write(ctx, websocket.MessageText, appendReq); err != nil {
			return s, err
		}
		secondResp, _, err := readUntilCompleted(ctx, conn, secondStart)
		if err != nil {
			return s, err
		}
		s.ClientFrames += 2
		s.ContinuationSucceeded = true
		s.ResponseTexts = append(s.ResponseTexts, responseTextFromPayload(secondResp))
		s.ClientEventType = gjson.GetBytes(secondResp, "type").String()
	} else {
		s.ClientEventType = gjson.GetBytes(firstResp, "type").String()
	}

	s.ClientLatencyMs = time.Since(firstStart).Milliseconds()
	s.FinishedAt = time.Now().UTC()
	return s, nil
}

func readUntilCompleted(ctx context.Context, conn *websocket.Conn, started time.Time) ([]byte, time.Duration, error) {
	var first time.Duration
	var lastPayload []byte
	for {
		_, payload, err := conn.Read(ctx)
		if err != nil {
			return nil, first, err
		}
		if first == 0 {
			first = time.Since(started)
		}
		lastPayload = payload
		eventType := gjson.GetBytes(payload, "type").String()
		if eventType == "error" {
			message := strings.TrimSpace(gjson.GetBytes(payload, "error.message").String())
			if message == "" {
				message = "websocket error"
			}
			return payload, first, errors.New(message)
		}
		if eventType == "response.completed" {
			return lastPayload, first, nil
		}
	}
}

func responseTextFromPayload(payload []byte) string {
	if text := strings.TrimSpace(gjson.GetBytes(payload, "response.output.0.content.0.text").String()); text != "" {
		return text
	}
	return strings.TrimSpace(gjson.GetBytes(payload, "error.message").String())
}

func (c *client) fetchAllAdminRequestLogs(model string, from time.Time) ([]requestLog, error) {
	var all []requestLog
	page := 1
	for {
		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("pageSize", "100")
		query.Set("model", model)
		query.Set("from", from.Format(time.RFC3339))

		var out requestLogList
		if err := c.doJSON(http.MethodGet, "/api/admin/request-logs?"+query.Encode(), nil, c.token, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Items...)
		if len(all) >= out.Total || len(out.Items) == 0 {
			break
		}
		page++
	}
	return all, nil
}

func (c *client) listChannels() ([]channel, error) {
	var out channelListResponse
	if err := c.doJSON(http.MethodGet, "/api/admin/channels", nil, c.token, &out); err != nil {
		return nil, err
	}
	return out.Channels, nil
}

func findChannelByName(channels []channel, name string) *channel {
	for _, ch := range channels {
		if ch.Name == name {
			copyCh := ch
			return &copyCh
		}
	}
	return nil
}

func (c *client) upsertChannel(existing *channel, payload map[string]any) (*channel, error) {
	var out channel
	method := http.MethodPost
	path := "/api/admin/channels"
	if existing != nil {
		method = http.MethodPut
		path = "/api/admin/channels/" + url.PathEscape(existing.ID)
		payload["apiKey"] = ""
	}
	if err := c.doJSON(method, path, payload, c.token, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) doJSON(method, path string, payload any, token string, out any) error {
	fullURL := c.baseURL + path
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(gjson.GetBytes(respBody, "error").String())
		if message == "" {
			message = strings.TrimSpace(string(respBody))
		}
		return &apiError{StatusCode: resp.StatusCode, Message: message}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

type apiError struct {
	StatusCode int
	Message    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("api %d: %s", e.StatusCode, e.Message)
}

func renderReport(summary runSummary) string {
	var b strings.Builder
	b.WriteString("# Responses WebSocket Stability Report\n\n")
	b.WriteString(fmt.Sprintf("- Started: `%s`\n", summary.StartedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- AMP Manager: `%s`\n", summary.ManagerBaseURL))
	b.WriteString(fmt.Sprintf("- Responses Base URL: `%s`\n", summary.ResponsesBaseURL))
	b.WriteString(fmt.Sprintf("- Model: `%s`\n", summary.ResponsesModel))
	b.WriteString(fmt.Sprintf("- Prefix: `%s`\n", summary.Prefix))
	b.WriteString(fmt.Sprintf("- Passed: `%t`\n\n", summary.Passed))

	b.WriteString("| Scenario | Expected U | Client OK | Client Fail | Logs | Missing Logs | Fallback | Unexpected U | p50 TTFB | p95 TTFB | p50 Latency | p95 Latency |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, scenario := range summary.Scenarios {
		b.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d | %d | %d | %d | %dms | %dms | %dms | %dms |\n",
			scenario.Config.Name,
			scenario.Config.ExpectedUpstream,
			scenario.ClientSuccesses,
			scenario.ClientFailures,
			scenario.LogCount,
			scenario.MissingLogs,
			scenario.FallbackCount,
			scenario.UnexpectedTransport,
			scenario.P50TTFBMs,
			scenario.P95TTFBMs,
			scenario.P50LatencyMs,
			scenario.P95LatencyMs,
		))
	}

	b.WriteString("\n## Notes\n\n")
	b.WriteString("- `Client Fail` counts downstream WebSocket failures seen by the harness.\n")
	b.WriteString("- `Missing Logs` means the request-log surface did not capture every client-visible success.\n")
	b.WriteString("- `Fallback` counts request logs where `transportFallbackReason` was non-empty.\n")
	b.WriteString("- `Unexpected U` counts request logs whose `upstreamTransport` did not match the scenario expectation.\n")
	return b.String()
}

func writeJSON(path string, value any) {
	data, _ := json.MarshalIndent(value, "", "  ")
	_ = os.WriteFile(path, data, 0o644)
}

func writeText(path, text string) {
	_ = os.WriteFile(path, []byte(text), 0o644)
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	copied := append([]int64(nil), values...)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })
	if len(copied) == 1 {
		return copied[0]
	}
	pos := (p / 100) * float64(len(copied)-1)
	idx := int(math.Ceil(pos))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(copied) {
		idx = len(copied) - 1
	}
	return copied[idx]
}

func websocketURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	}
	return parsed.String(), nil
}

func getEnvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func mustParseInt(key, fallback string) int {
	value, err := strconv.Atoi(getEnvDefault(key, fallback))
	if err != nil {
		fail(fmt.Errorf("invalid %s: %w", key, err))
	}
	return value
}

func parseIntList(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, errors.New("empty list")
	}
	return values, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
