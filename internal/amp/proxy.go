package amp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"
)

type proxyConfigKey struct{}

type ProxyConfig struct {
	UserID            string
	APIKeyID          string
	UpstreamURL       string
	UpstreamAPIKey    string
	ModelMappingsJSON string
	Enabled           bool   // 启用 AMP 增强功能（模型映射、渠道路由等）
	WebSearchMode     string // upstream | builtin_free | local_duckduckgo
	NativeMode        bool
	ShowBalanceInAd   bool
	Socks5Proxy       string
	RateMultiplier    float64
	GroupIDs          []string
}

func WithProxyConfig(ctx context.Context, cfg *ProxyConfig) context.Context {
	return context.WithValue(ctx, proxyConfigKey{}, cfg)
}

func GetProxyConfig(ctx context.Context) *ProxyConfig {
	if val := ctx.Value(proxyConfigKey{}); val != nil {
		if cfg, ok := val.(*ProxyConfig); ok {
			return cfg
		}
	}
	return nil
}

// readCloser wraps a reader and forwards Close to a separate closer
type readCloser struct {
	r io.Reader
	c io.Closer
}

func (rc *readCloser) Read(p []byte) (int, error) { return rc.r.Read(p) }
func (rc *readCloser) Close() error               { return rc.c.Close() }

// 全局 RetryTransport 实例，支持动态配置更新
var globalRetryTransport *RetryTransport

// GetRetryTransport 获取全局 RetryTransport 实例
func GetRetryTransport() *RetryTransport {
	return globalRetryTransport
}

// TimeoutConfig 超时配置
type TimeoutConfig struct {
	IdleConnTimeout     time.Duration
	ReadIdleTimeout     time.Duration
	KeepAliveInterval   time.Duration
	DialTimeout         time.Duration
	TLSHandshakeTimeout time.Duration
}

var (
	globalTimeoutConfig *TimeoutConfig
	timeoutConfigMu     sync.RWMutex
)

// DefaultTimeoutConfig 默认超时配置
func DefaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		IdleConnTimeout:     300 * time.Second, // 5分钟，允许 AI 长时间思考
		ReadIdleTimeout:     300 * time.Second,
		KeepAliveInterval:   15 * time.Second,
		DialTimeout:         30 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
	}
}

// GetTimeoutConfig 获取当前超时配置
func GetTimeoutConfig() *TimeoutConfig {
	timeoutConfigMu.RLock()
	defer timeoutConfigMu.RUnlock()
	if globalTimeoutConfig == nil {
		return DefaultTimeoutConfig()
	}
	return globalTimeoutConfig
}

// UpdateTimeoutConfig 更新超时配置
func UpdateTimeoutConfig(idleConn, readIdle, keepAlive, dial, tlsHandshake time.Duration) {
	timeoutConfigMu.Lock()
	defer timeoutConfigMu.Unlock()
	globalTimeoutConfig = &TimeoutConfig{
		IdleConnTimeout:     idleConn,
		ReadIdleTimeout:     readIdle,
		KeepAliveInterval:   keepAlive,
		DialTimeout:         dial,
		TLSHandshakeTimeout: tlsHandshake,
	}

	// 同时更新连接健康检查配置
	UpdateConnectionHealthConfig(readIdle)

	// 更新 SSE 流配置
	UpdateStreamConfig(keepAlive)
}

// InitTimeoutConfig 初始化超时配置（从数据库加载）
func InitTimeoutConfig(configJSON string) {
	if configJSON == "" {
		return
	}

	var cfg struct {
		IdleConnTimeoutSec     int `json:"idleConnTimeoutSec"`
		ReadIdleTimeoutSec     int `json:"readIdleTimeoutSec"`
		KeepAliveIntervalSec   int `json:"keepAliveIntervalSec"`
		DialTimeoutSec         int `json:"dialTimeoutSec"`
		TLSHandshakeTimeoutSec int `json:"tlsHandshakeTimeoutSec"`
	}

	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return
	}

	UpdateTimeoutConfig(
		time.Duration(cfg.IdleConnTimeoutSec)*time.Second,
		time.Duration(cfg.ReadIdleTimeoutSec)*time.Second,
		time.Duration(cfg.KeepAliveIntervalSec)*time.Second,
		time.Duration(cfg.DialTimeoutSec)*time.Second,
		time.Duration(cfg.TLSHandshakeTimeoutSec)*time.Second,
	)
}

// NewStreamingTransport 创建针对 AI 流式请求优化的 HTTP Transport
// 解决 60 秒左右连接中断的问题
func NewStreamingTransport() *http.Transport {
	cfg := GetTimeoutConfig()
	return &http.Transport{
		// 连接设置
		DialContext: (&net.Dialer{
			Timeout:   cfg.DialTimeout,
			KeepAlive: 30 * time.Second, // TCP Keep-Alive 间隔
		}).DialContext,

		// TLS 设置
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		TLSHandshakeTimeout: cfg.TLSHandshakeTimeout,

		// 连接池设置
		MaxIdleConns:        2048,
		MaxIdleConnsPerHost: 1024,
		MaxConnsPerHost:     0, // 无限制

		// 关键：延长空闲连接超时，避免连接过早关闭
		IdleConnTimeout: cfg.IdleConnTimeout,

		// 关键：不设置 ResponseHeaderTimeout，允许 AI 模型长时间思考
		// ResponseHeaderTimeout: 0 表示无超时
		ResponseHeaderTimeout: 0,

		// 关键：不设置 ExpectContinueTimeout，避免 100-continue 超时
		ExpectContinueTimeout: 0,

		// 禁用压缩以支持流式响应（gzip 缓冲会导致 SSE 流看起来无数据，触发 idle timeout）
		DisableCompression: true,

		// 保持连接复用
		DisableKeepAlives: false,

		// 强制尝试 HTTP/2
		ForceAttemptHTTP2: true,
	}
}

// InitRetryTransportConfig 初始化重试配置（从数据库加载）
// 需要在 CreateDynamicReverseProxy 之后调用
func InitRetryTransportConfig(configJSON string) {
	if globalRetryTransport == nil || configJSON == "" {
		return
	}

	var cfg struct {
		Enabled           bool  `json:"enabled"`
		MaxAttempts       int   `json:"maxAttempts"`
		GateTimeoutMs     int64 `json:"gateTimeoutMs"`
		MaxBodyBytes      int64 `json:"maxBodyBytes"`
		BackoffBaseMs     int64 `json:"backoffBaseMs"`
		BackoffMaxMs      int64 `json:"backoffMaxMs"`
		RetryOn429        bool  `json:"retryOn429"`
		RetryOn5xx        bool  `json:"retryOn5xx"`
		RespectRetryAfter bool  `json:"respectRetryAfter"`
		RetryOnEmptyBody  bool  `json:"retryOnEmptyBody"`
	}

	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		log.Warnf("retry: 解析配置失败，使用默认配置: %v", err)
		return
	}

	globalRetryTransport.UpdateConfig(&RetryConfig{
		Enabled:           cfg.Enabled,
		MaxAttempts:       cfg.MaxAttempts,
		GateTimeout:       time.Duration(cfg.GateTimeoutMs) * time.Millisecond,
		MaxBodyBytes:      cfg.MaxBodyBytes,
		BackoffBase:       time.Duration(cfg.BackoffBaseMs) * time.Millisecond,
		BackoffMax:        time.Duration(cfg.BackoffMaxMs) * time.Millisecond,
		RetryOn429:        cfg.RetryOn429,
		RetryOn5xx:        cfg.RetryOn5xx,
		RespectRetryAfter: cfg.RespectRetryAfter,
		RetryOnEmptyBody:  cfg.RetryOnEmptyBody,
	})

	log.WithFields(log.Fields{
		"enabled":     cfg.Enabled,
		"maxAttempts": cfg.MaxAttempts,
	}).Info("retry: 已加载保存的重试配置")
}

// socks5TransportCache caches per-proxy transports
var socks5TransportCache sync.Map

// getSocks5Transport returns a cached or new transport for the given SOCKS5 proxy URL
func getSocks5Transport(proxyURL string) (*http.Transport, error) {
	if cached, ok := socks5TransportCache.Load(proxyURL); ok {
		return cached.(*http.Transport), nil
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid socks5 proxy URL: %w", err)
	}

	var auth *proxy.Auth
	if parsed.User != nil {
		password, _ := parsed.User.Password()
		auth = &proxy.Auth{
			User:     parsed.User.Username(),
			Password: password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", parsed.Host, auth, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create socks5 dialer: %w", err)
	}

	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("socks5 dialer does not support context")
	}

	cfg := GetTimeoutConfig()
	transport := &http.Transport{
		DialContext:           contextDialer.DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		MaxIdleConns:          2048,
		MaxIdleConnsPerHost:   1024,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		ResponseHeaderTimeout: 0,
		ExpectContinueTimeout: 0,
		DisableCompression:    true,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     false,
	}

	socks5TransportCache.Store(proxyURL, transport)
	log.Infof("socks5: created new transport for proxy %s", maskProxyURL(proxyURL))
	return transport, nil
}

// Socks5AwareTransport wraps RetryTransport and uses SOCKS5 proxy when configured
type Socks5AwareTransport struct {
	Base *RetryTransport
}

func (t *Socks5AwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cfg := GetProxyConfig(req.Context())
	if cfg == nil || cfg.Socks5Proxy == "" {
		return t.Base.RoundTrip(req)
	}

	socks5Transport, err := getSocks5Transport(cfg.Socks5Proxy)
	if err != nil {
		log.Errorf("socks5: failed to create transport: %v, falling back to direct", err)
		return t.Base.RoundTrip(req)
	}

	retryTransport := NewRetryTransport(socks5Transport, t.Base.getConfig())
	return retryTransport.RoundTrip(req)
}

// maskProxyURL masks password in proxy URL for logging
func maskProxyURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "***"
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword(parsed.User.Username(), "***")
	}
	return parsed.String()
}

// CreateDynamicReverseProxy creates a reverse proxy for ampcode.com upstream
// Following CLIProxyAPI pattern: does NOT filter Anthropic-Beta headers
// Users going through ampcode.com are paying for the service and should get all features
func CreateDynamicReverseProxy() *httputil.ReverseProxy {
	// 使用针对 AI 流式请求优化的 Transport，解决 60 秒超时问题
	streamingTransport := NewStreamingTransport()

	// 初始化全局 RetryTransport
	globalRetryTransport = NewRetryTransport(streamingTransport, DefaultRetryConfig())

	proxy := &httputil.ReverseProxy{
		// 使用 SOCKS5 感知的 Transport，自动根据用户配置选择直连或走代理
		Transport: &Socks5AwareTransport{Base: globalRetryTransport},
		// FlushInterval 设为 -1 确保流式响应（SSE）立即刷新到客户端
		// 避免缓冲导致 "request ended without sending any chunks" 错误
		FlushInterval: -1,
		Director: func(req *http.Request) {
			cfg := GetProxyConfig(req.Context())
			if cfg == nil {
				log.Warn("amp proxy: no config in context")
				return
			}

			parsed, err := url.Parse(cfg.UpstreamURL)
			if err != nil {
				log.Errorf("amp proxy: invalid upstream url: %v", err)
				return
			}

			req.URL.Scheme = parsed.Scheme
			req.URL.Host = parsed.Host
			req.Host = parsed.Host

			log.Debugf("amp proxy: %s %s -> %s%s", req.Method, req.URL.Path, req.URL.Host, req.URL.Path)

			// In native mode, skip all request processing (tracing, model mapping, etc.)
			if cfg.NativeMode {
				req.Header.Del("Authorization")
				req.Header.Del("X-Api-Key")
				req.Header.Del("Transfer-Encoding")
				req.TransferEncoding = nil
				if cfg.UpstreamAPIKey != "" {
					req.Header.Set("X-Api-Key", cfg.UpstreamAPIKey)
					req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.UpstreamAPIKey))
				}
				return
			}

			// Only create trace for model invocation requests
			if IsModelInvocation(req.Method, req.URL.Path) {
				requestID := GetRequestID(req.Context())
				if requestID == "" {
					requestID = uuid.New().String()
				}
				trace := NewRequestTrace(
					requestID,
					cfg.UserID,
					cfg.APIKeyID,
					req.Method,
					req.URL.Path,
				)
				trace.SetUpstreamTransport("http")
				// Set provider info (amp upstream defaults to Anthropic)
				trace.SetChannel("", string(ProviderAnthropic), cfg.UpstreamURL)
				// Get model info from context if available
				if modelInfo := GetModelInfo(req.Context()); modelInfo != nil {
					trace.SetModels(modelInfo.OriginalModel, modelInfo.MappedModel)
				}
				if thinkingLevel := GetThinkingLevelFromContext(req.Context()); thinkingLevel != "" {
					trace.SetThinkingLevel(thinkingLevel)
				}
				// Store trace in context
				ctx := WithRequestTrace(req.Context(), trace)
				// Store ProviderInfo in context for token extraction
				ctx = WithProviderInfo(ctx, ProviderInfo{Provider: ProviderAnthropic})
				*req = *req.WithContext(ctx)

				// Write pending record to database immediately
				if writer := GetLogWriter(); writer != nil {
					writer.WritePendingFromTrace(trace)
				}

				// Capture request detail for logging
				if captureData := GetCaptureData(req.Context()); captureData != nil {
					StoreRequestDetail(trace.RequestID, captureData.RequestHeaders, captureData.RequestBody)
				}

				log.Infof("amp proxy: model invocation %s %s -> %s", req.Method, req.URL.Path, req.URL.Host)
			}

			// Remove client auth headers and hop-by-hop headers
			req.Header.Del("Authorization")
			req.Header.Del("X-Api-Key")
			req.Header.Del("Transfer-Encoding")
			req.TransferEncoding = nil

			// NOTE: Following CLIProxyAPI pattern - we do NOT filter Anthropic-Beta headers here
			// Users going through ampcode.com proxy are paying for the service and should get all features
			// including 1M context window (context-1m-2025-08-07)

			// Set upstream API key
			if cfg.UpstreamAPIKey != "" {
				req.Header.Set("X-Api-Key", cfg.UpstreamAPIKey)
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.UpstreamAPIKey))
			}
		},
		ModifyResponse: modifyResponse,
		ErrorHandler:   errorHandler,
	}
	return proxy
}

// modifyResponse handles gzip decompression and token extraction
// Does NOT attempt to rewrite context_length as that's not where Amp gets the value
func modifyResponse(resp *http.Response) error {
	// In native mode, pass through response without any processing
	cfg := GetProxyConfig(resp.Request.Context())
	if cfg != nil && cfg.NativeMode {
		return nil
	}

	trace := GetRequestTrace(resp.Request.Context())

	// 获取 provider 信息（amp upstream 默认为 Anthropic）
	info, ok := GetProviderInfo(resp.Request.Context())
	if !ok {
		info = ProviderInfo{Provider: ProviderAnthropic}
	}

	// 构建响应上下文
	rctx := &ResponseContext{
		Ctx:        resp.Request.Context(),
		Trace:      trace,
		Provider:   info,
		Headers:    resp.Header,
		StatusCode: resp.StatusCode,
	}
	if trace != nil {
		rctx.RequestID = trace.RequestID
	}

	// 根据响应类型选择处理管道
	if isStreamingResponse(resp) {
		// Claude/Anthropic: unprefix only names we prefixed on the way out
		if rctx.Provider.Provider == ProviderAnthropic {
			if toolMap, ok := GetClaudeToolNameMap(resp.Request.Context()); ok && len(toolMap) > 0 {
				resp.Body = NewSSETransformWrapper(resp.Body, func(b []byte) []byte {
					out, _ := UnprefixClaudeToolNamesWithMap(b, toolMap)
					return out
				})
			}
		}

		pipeline := NewStreamingPipelineWithContext(resp.Request.Context())
		if err := pipeline.ProcessStreamingResponse(resp, rctx); err != nil {
			return err
		}

		// Wrap SSE responses with keep-alive for long-running streams
		if rw := GetResponseWriter(resp.Request.Context()); rw != nil {
			// Check if pseudo-non-stream is enabled
			if GetPseudoNonStream(resp.Request.Context()) {
				auditModelName := ""
				if mi := GetModelInfo(resp.Request.Context()); mi != nil {
					auditModelName = mi.MappedModel
				}
				var opts []PseudoNonStreamOption
				if kw := GetAuditKeywords(resp.Request.Context()); len(kw) > 0 {
					opts = append(opts, WithAuditKeywordsOption(kw))
				}
				// Build retry function for amp upstream proxy
				cfg := GetProxyConfig(resp.Request.Context())
				if cfg != nil {
					retryReq := resp.Request.Clone(resp.Request.Context())
					opts = append(opts, WithRetryFunc(func() (io.ReadCloser, error) {
						clone := retryReq.Clone(retryReq.Context())
						if clone.GetBody != nil {
							body, err := clone.GetBody()
							if err == nil {
								clone.Body = body
							}
						}
						rt := GetRetryTransport()
						var transport http.RoundTripper
						if rt != nil {
							transport = rt.Base
						} else {
							transport = http.DefaultTransport
						}
						retryResp, err := transport.RoundTrip(clone)
						if err != nil {
							return nil, err
						}
						if retryResp.StatusCode < 200 || retryResp.StatusCode >= 300 {
							retryResp.Body.Close()
							return nil, fmt.Errorf("retry returned status %d", retryResp.StatusCode)
						}
						return retryResp.Body, nil
					}))
				}
				resp.Body = NewPseudoNonStreamBodyWrapper(resp.Body, rw, auditModelName, opts...)
				log.Infof("amp proxy: enabled pseudo-non-stream buffering for streaming response (model: %s)", auditModelName)
			} else if wrapper := NewSSEKeepAliveWrapper(resp.Body, rw, resp.Request.Context(), nil); wrapper != nil {
				resp.Body = wrapper
				log.Debugf("amp proxy: enabled SSE keep-alive for streaming response")
			}
		}
		return nil
	}

	// 处理非 2xx 响应
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return handleErrorResponse(resp, rctx)
	}

	// 处理非流式响应
	return NewDefaultNonStreamingPipeline().ProcessNonStreamingResponse(resp, rctx)
}

// handleErrorResponse 处理错误响应
func handleErrorResponse(resp *http.Response, ctx *ResponseContext) error {
	if ctx.Trace != nil {
		ctx.Trace.SetError("upstream_error")
		streamBody := io.ReadCloser(resp.Body)
		if ctx.RequestID != "" && IsRequestDetailCaptureEnabled(ctx.Ctx) {
			streamBody = NewResponseCaptureWrapper(streamBody, ctx.RequestID, resp.Header)
		}
		resp.Body = NewLoggingBodyWrapper(streamBody, ctx.Trace, resp.StatusCode, nil)
	}
	return nil
}

func isStreamingResponse(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return strings.Contains(contentType, "text/event-stream")
}

func errorHandler(rw http.ResponseWriter, req *http.Request, err error) {
	log.Errorf("amp upstream proxy error for %s %s: %v", req.Method, req.URL.Path, err)
	// Update error log (pending record was already written in Director)
	var requestID string
	if trace := GetRequestTrace(req.Context()); trace != nil {
		trace.SetError("upstream_request_failed")
		trace.SetResponse(http.StatusBadGateway)
		requestID = trace.RequestID
		if writer := GetLogWriter(); writer != nil {
			writer.UpdateFromTrace(trace)
		}
	}
	// 使用清理后的错误消息，防止泄露敏感信息
	safeMsg := SanitizeError(err)
	body := BuildErrorResponseBody(http.StatusBadGateway, "Failed to reach Amp upstream: "+safeMsg)
	if IsRequestDetailCaptureEnabled(req.Context()) && requestID != "" {
		StoreErrorResponseDetail(requestID, http.StatusBadGateway, body)
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusBadGateway)
	_, _ = rw.Write(body)
}

func ProxyHandler(proxy *httputil.ReverseProxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		if GetProxyConfig(c.Request.Context()) == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: missing proxy configuration"})
			return
		}
		// Inject ResponseWriter into context for SSE keep-alive support
		ctx := WithResponseWriter(c.Request.Context(), c.Writer)
		c.Request = c.Request.WithContext(ctx)
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

// filterAntropicBetaHeader removes the context-1m-2025-08-07 beta feature
// This should ONLY be called for local/channel handling paths, NOT for ampcode.com proxy
func filterAntropicBetaHeader(req *http.Request) {
	betaHeader := req.Header.Get("Anthropic-Beta")
	if betaHeader == "" {
		return
	}

	filtered := filterBetaFeatures(betaHeader, "context-1m-2025-08-07")
	if filtered != "" {
		req.Header.Set("Anthropic-Beta", filtered)
		log.Debugf("channel proxy: filtered Anthropic-Beta header: %s -> %s", betaHeader, filtered)
	} else {
		req.Header.Del("Anthropic-Beta")
		log.Debugf("channel proxy: removed Anthropic-Beta header (was: %s)", betaHeader)
	}
}

// filterBetaFeatures removes a specific feature from comma-separated beta features list
func filterBetaFeatures(header, featureToRemove string) string {
	features := strings.Split(header, ",")
	filtered := make([]string, 0, len(features))

	for _, feature := range features {
		trimmed := strings.TrimSpace(feature)
		if trimmed != "" && trimmed != featureToRemove {
			filtered = append(filtered, trimmed)
		}
	}

	return strings.Join(filtered, ",")
}
