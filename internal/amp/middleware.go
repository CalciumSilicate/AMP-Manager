package amp

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/precision"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const (
	debugMaxBodySize       = 1024
	debugMaxGzipDecompress = 10 * 1024
)

var (
	debugInternalAPIEnabled = os.Getenv("AMP_DEBUG_INTERNAL_API") == "true"
	sensitiveHeaders        = map[string]bool{
		"Authorization": true,
		"X-Api-Key":     true,
		"Cookie":        true,
		"Set-Cookie":    true,
	}
)

// Constants for free tier request interception
const (
	webSearchQuery             = "webSearch2"
	extractWebPageContentQuery = "extractWebPageContent"
	mcpToolPrefix              = "mcp__tools__mcp-"
)

// adEndpoints are ad-related query parameters that should be blocked
var adEndpoints = []string{
	"recordAdImpressionEnd",
	"recordAdImpressionStart",
	"getCurrentAd",
}

func detectLocalToolQuery(rawQuery string) (string, bool) {
	if rawQuery == "" {
		return "", false
	}
	parts := strings.Split(rawQuery, "&")
	for _, part := range parts {
		key := part
		if idx := strings.IndexByte(part, '='); idx >= 0 {
			key = part[:idx]
		}
		switch key {
		case webSearchQuery, mcpToolPrefix + transformToolName(webSearchQuery):
			return webSearchQuery, true
		case extractWebPageContentQuery, mcpToolPrefix + transformToolName(extractWebPageContentQuery):
			return extractWebPageContentQuery, true
		}
	}
	return "", false
}

// detectAdEndpoint checks if the query string contains an ad-related endpoint
// and returns its name for per-endpoint response generation
func detectAdEndpoint(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	parts := strings.Split(rawQuery, "&")
	for _, part := range parts {
		key := part
		if idx := strings.IndexByte(part, '='); idx >= 0 {
			key = part[:idx]
		}
		for _, ad := range adEndpoints {
			if key == ad {
				return ad
			}
		}
	}
	return ""
}

// isAdRequest checks if the query string contains ad-related endpoints
func isAdRequest(rawQuery string) bool {
	return detectAdEndpoint(rawQuery) != ""
}

// buildAdResponse returns a realistic response for each ad endpoint
// so the client behaves as if ampcode.com actually handled the request
func buildAdResponse(endpoint string) gin.H {
	switch endpoint {
	case "getCurrentAd":
		return gin.H{
			"ok":     true,
			"result": nil,
		}
	case "recordAdImpressionStart":
		return gin.H{
			"ok":              true,
			"result":          gin.H{},
			"creditsConsumed": "0",
		}
	case "recordAdImpressionEnd":
		return gin.H{
			"ok":              true,
			"result":          gin.H{},
			"creditsConsumed": "0",
		}
	default:
		return gin.H{"ok": true}
	}
}

// AdBlockMiddleware intercepts ad-related requests and returns realistic fake responses
func AdBlockMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		endpoint := detectAdEndpoint(c.Request.URL.RawQuery)
		if endpoint != "" {
			log.Debugf("amp: blocked ad request: path=%s, endpoint=%s", c.Request.URL.Path, endpoint)
			c.JSON(http.StatusOK, buildAdResponse(endpoint))
			c.Abort()
			return
		}

		if c.Request.URL.Path == "/api/ads" || strings.HasPrefix(c.Request.URL.Path, "/api/ads/") {
			log.Debugf("amp: blocked ads endpoint: path=%s", c.Request.URL.Path)
			c.JSON(http.StatusOK, gin.H{
				"ok":     true,
				"result": nil,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// BalanceAdMiddleware intercepts ad requests and optionally injects balance info
func BalanceAdMiddleware() gin.HandlerFunc {
	userRepo := repository.NewUserRepository()
	return func(c *gin.Context) {
		endpoint := detectAdEndpoint(c.Request.URL.RawQuery)
		if endpoint != "" {
			cfg := GetProxyConfig(c.Request.Context())
			if cfg != nil && cfg.ShowBalanceInAd && endpoint == "getCurrentAd" {
				balance, err := userRepo.GetBalance(cfg.UserID)
				if err == nil {
					balanceUsd := fmt.Sprintf("$%.2f", float64(balance)/1e6)
					c.JSON(http.StatusOK, gin.H{
						"ok": true,
						"result": gin.H{
							"id":           "balance-info",
							"type":         "text",
							"title":        "💰 账户余额",
							"body":         fmt.Sprintf("当前余额: %s", balanceUsd),
							"impressionId": "balance-" + cfg.UserID,
							"ctaText":      "",
							"ctaUrl":       "",
						},
					})
					c.Abort()
					return
				}
			}
			log.Debugf("amp: blocked ad request: path=%s, endpoint=%s", c.Request.URL.Path, endpoint)
			c.JSON(http.StatusOK, buildAdResponse(endpoint))
			c.Abort()
			return
		}

		if c.Request.URL.Path == "/api/ads" || strings.HasPrefix(c.Request.URL.Path, "/api/ads/") {
			log.Debugf("amp: blocked ads endpoint: path=%s", c.Request.URL.Path)
			c.JSON(http.StatusOK, gin.H{
				"ok":     true,
				"result": nil,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// Pre-compiled regex for matching isFreeTierRequest field
var isFreeTierRequestRegex = regexp.MustCompile(`"isFreeTierRequest"\s*:\s*false`)

var (
	apiKeyRepo   = repository.NewAPIKeyRepository()
	settingsRepo = repository.NewAmpSettingsRepository()
	systemCfgSvc = service.NewSystemConfigService()
	userRepo     = repository.NewUserRepository()
)

var groupRepo = repository.NewGroupRepository()

var userConcurrencyLimiter = newActiveRequestLimiter()

func APIKeyAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := extractAPIKey(c)
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, NewStandardError(http.StatusUnauthorized, "missing api key"))
			return
		}

		keyHash := hashAPIKey(apiKey)

		apiKeyRecord, err := apiKeyRepo.GetByKeyHash(keyHash)
		if err != nil {
			log.Errorf("amp api key auth: db error: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, NewStandardError(http.StatusInternalServerError, "internal server error"))
			return
		}

		if apiKeyRecord == nil {
			log.Warnf("amp api key auth: invalid key (prefix: %s...)", maskAPIKey(apiKey))
			c.AbortWithStatusJSON(http.StatusUnauthorized, NewStandardError(http.StatusUnauthorized, "invalid api key"))
			return
		}

		if apiKeyRecord.RevokedAt != nil {
			log.Warnf("amp api key auth: revoked key used (id: %s)", apiKeyRecord.ID)
			c.AbortWithStatusJSON(http.StatusUnauthorized, NewStandardError(http.StatusUnauthorized, "api key revoked"))
			return
		}

		if apiKeyRecord.ExpiresAt != nil && time.Now().After(*apiKeyRecord.ExpiresAt) {
			log.Warnf("amp api key auth: expired key used (id: %s)", apiKeyRecord.ID)
			c.AbortWithStatusJSON(http.StatusUnauthorized, NewStandardError(http.StatusUnauthorized, "api key expired"))
			return
		}

		ampProxySettingsPolicy, err := systemCfgSvc.GetAmpProxySettingsPolicy()
		if err != nil {
			log.Errorf("amp api key auth: failed to load system config: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, NewStandardError(http.StatusInternalServerError, "internal server error"))
			return
		}

		user, err := userRepo.GetByID(apiKeyRecord.UserID)
		if err != nil {
			log.Errorf("amp api key auth: failed to load user %s: %v", apiKeyRecord.UserID, err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, NewStandardError(http.StatusInternalServerError, "internal server error"))
			return
		}
		userIsAdmin := user != nil && user.IsAdmin

		proxyCfg := &ProxyConfig{
			UserID:            apiKeyRecord.UserID,
			APIKeyID:          apiKeyRecord.ID,
			UpstreamURL:       "https://ampcode.com",
			UpstreamAPIKey:    "",
			ModelMappingsJSON: "",
			Enabled:           false,
			WebSearchMode:     "upstream",
			NativeMode:        false,
			ShowBalanceInAd:   false,
			Socks5Proxy:       "",
		}

		if service.CanAccessAmpSettingsForPolicy(ampProxySettingsPolicy, userIsAdmin) {
			settings, err := settingsRepo.GetByUserID(apiKeyRecord.UserID)
			if err != nil {
				log.Errorf("amp api key auth: failed to load settings: %v", err)
				c.AbortWithStatusJSON(http.StatusInternalServerError, NewStandardError(http.StatusInternalServerError, "internal server error"))
				return
			}

			if settings != nil {
				if settings.UpstreamURL != "" {
					proxyCfg.UpstreamURL = settings.UpstreamURL
				}
				proxyCfg.UpstreamAPIKey = settings.UpstreamAPIKey
				proxyCfg.ModelMappingsJSON = settings.ModelMappingsJSON
				proxyCfg.Enabled = settings.Enabled
				proxyCfg.WebSearchMode = settings.WebSearchMode
				proxyCfg.NativeMode = settings.NativeMode
				proxyCfg.ShowBalanceInAd = settings.ShowBalanceInAd
				proxyCfg.Socks5Proxy = settings.Socks5Proxy
			}
		}

		rateMultiplierPPM, groupIDs, err := groupRepo.GetMinRateMultiplierByUserID(apiKeyRecord.UserID)
		if err != nil {
			log.Warnf("amp api key auth: failed to get rate multiplier for user %s: %v", apiKeyRecord.UserID, err)
		}
		rateMultiplier := precision.MultiplierPPMToFloat64(rateMultiplierPPM)
		proxyCfg.RateMultiplier = rateMultiplier
		proxyCfg.GroupRateMultiplier = rateMultiplier
		proxyCfg.GroupIDs = groupIDs

		if user != nil && user.ConcurrencyLimit > 0 {
			if !userConcurrencyLimiter.TryAcquire(apiKeyRecord.UserID, user.ConcurrencyLimit) {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, NewStandardError(http.StatusTooManyRequests, "user concurrency limit exceeded"))
				return
			}
			defer userConcurrencyLimiter.Release(apiKeyRecord.UserID)
		}

		ctx := WithProxyConfig(c.Request.Context(), proxyCfg)
		c.Request = c.Request.WithContext(ctx)

		go func() {
			if err := apiKeyRepo.UpdateLastUsedThrottled(apiKeyRecord.ID, DefaultLastUsedUpdateInterval); err != nil {
				log.Warnf("amp api key auth: failed to update last_used_at: %v", err)
			}
		}()

		c.Next()
	}
}

func extractAPIKey(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		if strings.HasPrefix(authHeader, "Bearer ") {
			return strings.TrimPrefix(authHeader, "Bearer ")
		}
		return authHeader
	}

	xApiKey := c.GetHeader("X-Api-Key")
	if xApiKey != "" {
		return xApiKey
	}

	// Support Google Gemini SDK which sends API key via x-goog-api-key header
	if googKey := c.GetHeader("x-goog-api-key"); googKey != "" {
		return googKey
	}

	// Support Google Gemini SDK which sends API key as query parameter
	if qKey := c.Query("key"); qKey != "" {
		return qKey
	}

	return ""
}

func hashAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

type activeRequestLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newActiveRequestLimiter() *activeRequestLimiter {
	return &activeRequestLimiter{
		counts: make(map[string]int),
	}
}

func (l *activeRequestLimiter) TryAcquire(userID string, limit int) bool {
	if userID == "" || limit <= 0 {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	current := l.counts[userID]
	if current >= limit {
		return false
	}
	l.counts[userID] = current + 1
	return true
}

func (l *activeRequestLimiter) Release(userID string) {
	if userID == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	current := l.counts[userID]
	if current <= 1 {
		delete(l.counts, userID)
		return
	}
	l.counts[userID] = current - 1
}

func (l *activeRequestLimiter) Current(userID string) int {
	if userID == "" {
		return 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	return l.counts[userID]
}

func GetUserCurrentConcurrency(userID string) int {
	return userConcurrencyLimiter.Current(userID)
}

func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 4 {
		return strings.Repeat("*", len(apiKey))
	}
	return apiKey[:4] + "***"
}

// DebugInternalAPIMiddleware logs request/response details for webSearch2 and extractWebPageContent
// Disabled by default, enable with AMP_DEBUG_INTERNAL_API=true environment variable
func DebugInternalAPIMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !debugInternalAPIEnabled {
			c.Next()
			return
		}

		tool, ok := detectLocalToolQuery(c.Request.URL.RawQuery)
		if !ok {
			c.Next()
			return
		}

		// Log request headers (with sensitive header masking)
		log.Infof("=== DEBUG %s REQUEST ===", tool)
		log.Infof("URL: %s", c.Request.URL.String())
		log.Infof("Method: %s", c.Request.Method)
		log.Infof("--- Request Headers ---")
		for k, v := range c.Request.Header {
			if sensitiveHeaders[k] {
				log.Infof("  %s: [REDACTED]", k)
			} else {
				log.Infof("  %s: %v", k, v)
			}
		}

		// Read and log request body (limited to debugMaxBodySize)
		if c.Request.Body != nil {
			bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, debugMaxBodySize+1))
			if err == nil {
				log.Infof("--- Request Body ---")
				if len(bodyBytes) > debugMaxBodySize {
					log.Infof("%s... [truncated]", string(bodyBytes[:debugMaxBodySize]))
				} else {
					log.Infof("%s", string(bodyBytes))
				}
				// Restore body for downstream handlers - need to read remaining if truncated
				if len(bodyBytes) > debugMaxBodySize {
					remaining, _ := io.ReadAll(c.Request.Body)
					c.Request.Body = io.NopCloser(bytes.NewBuffer(append(bodyBytes, remaining...)))
				} else {
					c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				}
			}
		}

		// Create response writer wrapper to capture response
		rw := &responseLogWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = rw

		c.Next()

		// Log response
		log.Infof("--- Response Status ---")
		log.Infof("Status: %d", rw.Status())
		log.Infof("--- Response Headers ---")
		for k, v := range rw.Header() {
			if sensitiveHeaders[k] {
				log.Infof("  %s: [REDACTED]", k)
			} else {
				log.Infof("  %s: %v", k, v)
			}
		}
		log.Infof("--- Response Body ---")
		// Decompress gzip if needed (with size limit)
		respBody := rw.body.Bytes()
		if rw.Header().Get("Content-Encoding") == "gzip" && len(respBody) > 0 {
			gr, err := gzip.NewReader(bytes.NewReader(respBody))
			if err == nil {
				decompressed, err := io.ReadAll(io.LimitReader(gr, debugMaxGzipDecompress))
				gr.Close()
				if err == nil {
					respBody = decompressed
				}
			}
		}
		// Limit response body logging
		if len(respBody) > debugMaxBodySize {
			log.Infof("%s... [truncated]", string(respBody[:debugMaxBodySize]))
		} else {
			log.Infof("%s", string(respBody))
		}
		log.Infof("=== END DEBUG %s ===", tool)
	}
}

type responseLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// BillingCheckMiddleware checks if user has sufficient balance/subscription before model invocations.
// Blocks requests with 403 if balance and subscription quota are both exhausted.
func BillingCheckMiddleware() gin.HandlerFunc {
	billingSvc := service.NewBillingService()
	return func(c *gin.Context) {
		// Only check for model invocation requests (the ones that cost money)
		if !IsModelInvocation(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}

		cfg := GetProxyConfig(c.Request.Context())
		if cfg == nil {
			c.Next()
			return
		}

		channelCfg := GetChannelConfig(c)
		breakdown := computePricingBreakdown(c.Request.Context(), nilIfNilChannelConfig(channelCfg), nil)
		if breakdown.TotalMultiplier == 0 {
			c.Next()
			return
		}

		requestID := GetRequestID(c.Request.Context())
		if requestID == "" {
			requestID = uuid.New().String()
			c.Request = c.Request.WithContext(WithRequestID(c.Request.Context(), requestID))
		}

		estimate := GetBillingEstimate(c.Request.Context())
		canStart, err := billingSvc.ReserveRequest(service.AdmissionRequest{
			RequestID:           requestID,
			UserID:              cfg.UserID,
			PricingModel:        safeEstimateModel(estimate),
			EstimatedCostMicros: safeEstimateCost(estimate),
		})
		if err != nil {
			log.Errorf("billing check: failed for user %s: %v", cfg.UserID, err)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, NewStandardError(http.StatusServiceUnavailable, "billing state unavailable"))
			return
		}

		if !canStart {
			log.Warnf("billing check: insufficient funds for user %s", cfg.UserID)
			c.AbortWithStatusJSON(http.StatusForbidden, NewStandardError(http.StatusForbidden, "余额和订阅额度均不足，请充值后再使用"))
			return
		}

		c.Next()
	}
}

func nilIfNilChannelConfig(cfg *ChannelConfig) *model.Channel {
	if cfg == nil {
		return nil
	}
	return cfg.Channel
}

func safeEstimateModel(estimate *BillingEstimate) string {
	if estimate == nil {
		return ""
	}
	return estimate.PricingModel
}

func safeEstimateCost(estimate *BillingEstimate) int64 {
	if estimate == nil {
		return 0
	}
	return estimate.EstimatedCostMicros
}

// ForceFreeTierMiddleware forces webSearch2 and extractWebPageContent requests to use free tier
// Deprecated: Use WebSearchStrategyMiddleware instead
func ForceFreeTierMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tool, ok := detectLocalToolQuery(c.Request.URL.RawQuery)
		if !ok {
			c.Next()
			return
		}

		if c.Request.Body == nil {
			c.Next()
			return
		}

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Warnf("amp: could not read request body for %s, proxying as-is: %v", tool, err)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			c.Next()
			return
		}

		if isFreeTierRequestRegex.Match(bodyBytes) {
			modifiedBody := isFreeTierRequestRegex.ReplaceAll(bodyBytes, []byte(`"isFreeTierRequest":true`))
			c.Request.ContentLength = int64(len(modifiedBody))
			c.Request.Header.Set("Content-Length", strconv.Itoa(len(modifiedBody)))
			c.Request.Body = io.NopCloser(bytes.NewBuffer(modifiedBody))
			log.Debugf("amp: %s request modified to use free tier", tool)
		} else {
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		c.Next()
	}
}

// WebSearchStrategyMiddleware 根据用户设置选择网页搜索策略
// 统一处理 webSearch2 和 extractWebPageContent 请求
func WebSearchStrategyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tool, ok := detectLocalToolQuery(c.Request.URL.RawQuery)
		if !ok {
			c.Next()
			return
		}

		cfg := GetProxyConfig(c.Request.Context())
		if cfg == nil {
			c.Next()
			return
		}

		switch cfg.WebSearchMode {
		case "local_duckduckgo":
			handleLocalWebSearch(c, tool)
		case "builtin_free":
			handleBuiltinFreeSearch(c, tool)
		default:
			c.Next()
		}
	}
}

// handleLocalWebSearch 处理本地搜索（DuckDuckGo）
func handleLocalWebSearch(c *gin.Context, query string) {
	if query == extractWebPageContentQuery {
		handleExtractWebPage(c)
		return
	}

	if query == webSearchQuery {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Errorf("web_search: failed to read request body: %v", err)
			c.Next()
			return
		}

		var req WebSearchRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			log.Errorf("web_search: failed to parse request: %v", err)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			c.Next()
			return
		}

		log.Infof("web_search: handling locally - queries: %v, maxResults: %d", req.Params.SearchQueries, req.Params.MaxResults)

		results, err := performDuckDuckGoSearch(req.Params.SearchQueries, req.Params.MaxResults)
		if err != nil {
			log.Errorf("web_search: search failed: %v", err)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			c.Next()
			return
		}

		resp := WebSearchResponse{
			OK:              true,
			CreditsConsumed: "0",
		}
		resp.Result.Results = results
		resp.Result.Provider = "local-duckduckgo"
		resp.Result.ShowParallelAttribution = false

		log.Infof("web_search: returning %d results locally", len(results))
		c.JSON(200, resp)
		c.Abort()
	}
}

// handleBuiltinFreeSearch 强制使用内置免费搜索（修改 isFreeTierRequest）
func handleBuiltinFreeSearch(c *gin.Context, query string) {
	if c.Request.Body == nil {
		c.Next()
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Warnf("amp: could not read request body for %s, proxying as-is: %v", query, err)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		c.Next()
		return
	}

	// 使用 JSON 解析修改（比正则更可靠）
	modifiedBody, modified := modifyFreeTierRequest(bodyBytes)
	if modified {
		c.Request.ContentLength = int64(len(modifiedBody))
		c.Request.Header.Set("Content-Length", strconv.Itoa(len(modifiedBody)))
		c.Request.Body = io.NopCloser(bytes.NewBuffer(modifiedBody))
		log.Debugf("amp: %s request modified to use builtin free tier", query)
	} else {
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	c.Next()
}

// modifyFreeTierRequest 使用 JSON 解析修改 isFreeTierRequest 为 true
func modifyFreeTierRequest(bodyBytes []byte) ([]byte, bool) {
	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return bodyBytes, false
	}

	params, ok := data["params"].(map[string]interface{})
	if !ok {
		return bodyBytes, false
	}

	if val, exists := params["isFreeTierRequest"]; exists {
		if boolVal, ok := val.(bool); ok && !boolVal {
			params["isFreeTierRequest"] = true
			modifiedBytes, err := json.Marshal(data)
			if err != nil {
				return bodyBytes, false
			}
			return modifiedBytes, true
		}
	}

	return bodyBytes, false
}

// RequestLoggingMiddleware 请求日志记录中间件
// 在请求开始时创建 RequestTrace，请求结束后写入日志
// 注意：模型调用请求由 pending 工作流处理，此中间件跳过
func RequestLoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取代理配置（由 APIKeyAuthMiddleware 设置）
		cfg := GetProxyConfig(c.Request.Context())
		if cfg == nil {
			c.Next()
			return
		}

		// 模型调用请求由 pending 工作流处理，跳过记录
		if IsModelInvocation(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}

		// 创建请求追踪
		requestID := uuid.New().String()
		trace := NewRequestTrace(
			requestID,
			cfg.UserID,
			cfg.APIKeyID,
			c.Request.Method,
			c.Request.URL.Path,
		)

		// 将 trace 存入 context
		ctx := WithRequestTrace(c.Request.Context(), trace)
		c.Request = c.Request.WithContext(ctx)

		// 执行后续处理
		c.Next()

		// 设置响应状态
		trace.SetResponse(c.Writer.Status())

		// 从 model mapping 上下文获取模型信息
		if original := GetOriginalModel(c); original != "" {
			if mapped := GetMappedModel(c); mapped != "" {
				trace.SetModels(original, mapped)
			} else {
				trace.SetModels(original, original)
			}
		}

		// 异步写入日志
		if writer := GetLogWriter(); writer != nil {
			writer.WriteFromTrace(trace)
		}
	}
}

// IsNativeMode checks if native mode is enabled for the current request
func IsNativeMode(c *gin.Context) bool {
	cfg := GetProxyConfig(c.Request.Context())
	return cfg != nil && cfg.NativeMode
}

// IsProxyDisabled checks if AMP proxy enhancements are disabled (pure passthrough except model mapping)
func IsProxyDisabled(c *gin.Context) bool {
	cfg := GetProxyConfig(c.Request.Context())
	return cfg != nil && !cfg.Enabled
}

// NativeModeSkipMiddleware wraps a middleware and skips it when native mode is enabled
func NativeModeSkipMiddleware(inner gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsNativeMode(c) {
			c.Next()
			return
		}
		inner(c)
	}
}

// ProxyDisabledSkipMiddleware wraps a middleware and skips it when AMP proxy is disabled or native mode is enabled
func ProxyDisabledSkipMiddleware(inner gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsNativeMode(c) || IsProxyDisabled(c) {
			c.Next()
			return
		}
		inner(c)
	}
}

// PublicProxyMiddleware sets default ProxyConfig for public routes (threads, docs, etc.)
// These routes don't require API key authentication and proxy directly to ampcode.com
func PublicProxyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Set default ProxyConfig for public routes
		proxyCfg := &ProxyConfig{
			UserID:         "public",
			APIKeyID:       "public",
			UpstreamURL:    "https://ampcode.com",
			UpstreamAPIKey: "", // No API key needed for public pages
		}
		ctx := WithProxyConfig(c.Request.Context(), proxyCfg)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
