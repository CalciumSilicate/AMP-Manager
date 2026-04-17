package amp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// FirstByteTimeoutError 首字节超时错误，实现 net.Error 接口
type FirstByteTimeoutError struct {
	Timeout_ time.Duration
}

func (e *FirstByteTimeoutError) Error() string {
	return "first byte timeout after " + e.Timeout_.String()
}

func (e *FirstByteTimeoutError) Timeout() bool {
	return true
}

func (e *FirstByteTimeoutError) Temporary() bool {
	return true
}

// RetryConfig 重试配置（可通过管理员界面配置）
type RetryConfig struct {
	Enabled           bool          `json:"enabled"`
	MaxAttempts       int           `json:"maxAttempts"`
	GateTimeout       time.Duration `json:"gateTimeout"`
	MaxBodyBytes      int64         `json:"maxBodyBytes"`
	BackoffBase       time.Duration `json:"backoffBase"`
	BackoffMax        time.Duration `json:"backoffMax"`
	RetryOn429        bool          `json:"retryOn429"`
	RetryOn5xx        bool          `json:"retryOn5xx"`
	RespectRetryAfter bool          `json:"respectRetryAfter"`
	RetryOnEmptyBody  bool          `json:"retryOnEmptyBody"`
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		Enabled:           true,
		MaxAttempts:       3,
		GateTimeout:       300 * time.Second, // 5分钟，与 ReadIdleTimeout 对齐，允许 AI 长时间思考
		MaxBodyBytes:      60 << 20,          // 60MB
		BackoffBase:       100 * time.Millisecond,
		BackoffMax:        2 * time.Second,
		RetryOn429:        true,
		RetryOn5xx:        true,
		RespectRetryAfter: true,
		RetryOnEmptyBody:  true,
	}
}

// RetryConfigFromDB 从数据库配置创建 RetryConfig
func RetryConfigFromDB(enabled bool, maxAttempts int, gateTimeoutMs, maxBodyBytes, backoffBaseMs, backoffMaxMs int64, retryOn429, retryOn5xx, respectRetryAfter, retryOnEmptyBody bool) *RetryConfig {
	return &RetryConfig{
		Enabled:           enabled,
		MaxAttempts:       maxAttempts,
		GateTimeout:       time.Duration(gateTimeoutMs) * time.Millisecond,
		MaxBodyBytes:      maxBodyBytes,
		BackoffBase:       time.Duration(backoffBaseMs) * time.Millisecond,
		BackoffMax:        time.Duration(backoffMaxMs) * time.Millisecond,
		RetryOn429:        retryOn429,
		RetryOn5xx:        retryOn5xx,
		RespectRetryAfter: respectRetryAfter,
		RetryOnEmptyBody:  retryOnEmptyBody,
	}
}

// RetryTransport 实现首包门控重试的 HTTP RoundTripper
type RetryTransport struct {
	Base http.RoundTripper
	cfg  *RetryConfig
	mu   sync.RWMutex
}

// NewRetryTransport 创建重试 Transport
func NewRetryTransport(base http.RoundTripper, cfg *RetryConfig) *RetryTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	if cfg == nil {
		cfg = DefaultRetryConfig()
	}
	return &RetryTransport{
		Base: base,
		cfg:  cfg,
	}
}

// UpdateConfig 动态更新配置（线程安全）
func (rt *RetryTransport) UpdateConfig(cfg *RetryConfig) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.cfg = cfg
}

// getConfig 获取当前配置（线程安全）
func (rt *RetryTransport) getConfig() *RetryConfig {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.cfg
}

// RetryExhaustedError 重试耗尽错误
type RetryExhaustedError struct {
	Attempts int
	LastErr  error
}

func (e *RetryExhaustedError) Error() string {
	return fmt.Sprintf("retry exhausted after %d attempts: %v", e.Attempts, e.LastErr)
}

func (e *RetryExhaustedError) Unwrap() error {
	return e.LastErr
}

// RoundTrip 实现 http.RoundTripper 接口
func (rt *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cfg := rt.getConfig()

	// 如果禁用重试或只允许1次，直接调用底层
	if !cfg.Enabled || cfg.MaxAttempts <= 1 {
		return rt.Base.RoundTrip(req)
	}

	// 缓存请求体以支持重放
	bodyBytes, canRetry, err := rt.cacheRequestBody(req, cfg.MaxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to cache request body: %w", err)
	}
	if !canRetry {
		// 请求体太大，无法重试
		log.Debug("retry: request body too large, skipping retry")
		return rt.Base.RoundTrip(req)
	}

	// 检查是否是模型调用请求，用于幂等性处理
	isModelCall := IsModelInvocation(req.Method, req.URL.Path)
	isStreaming := strings.Contains(req.Header.Get("Accept"), "text/event-stream") ||
		req.URL.Query().Get("stream") == "true"

	// 为模型调用请求添加幂等性 key（防止重试导致双重计费）
	var idempotencyKey string
	if isModelCall {
		if traceID := req.Header.Get("X-Request-ID"); traceID != "" {
			idempotencyKey = traceID
		} else {
			idempotencyKey = uuid.New().String()
		}
	}

	var lastErr error
	var lastResp *http.Response

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// 检查 context 是否已取消
		if err := req.Context().Err(); err != nil {
			return nil, err
		}

		// 为本次尝试准备请求
		attemptReq := rt.cloneRequest(req, bodyBytes)

		// 为模型调用请求设置幂等性 header
		if idempotencyKey != "" {
			attemptReq.Header.Set("Idempotency-Key", idempotencyKey)
		}

		// 发起请求
		resp, err := rt.Base.RoundTrip(attemptReq)

		if err != nil {
			lastErr = err

			// 流式请求在首字节超时后不应重试（上游可能已开始处理）
			if isStreaming && attempt > 1 {
				var fbTimeout *FirstByteTimeoutError
				if errors.As(err, &fbTimeout) {
					log.Warnf("retry: streaming request first-byte timeout, not retrying (upstream may have started processing)")
					return nil, err
				}
			}

			if rt.shouldRetryError(err) && attempt < cfg.MaxAttempts {
				rt.logRetryAttempt(req, attempt, cfg.MaxAttempts, err, nil)
				rt.backoff(req.Context(), attempt, cfg, nil)
				continue
			}
			return nil, err
		}

			// 检查是否需要根据状态码重试
			if rt.shouldRetryStatusCode(resp.StatusCode, cfg) && attempt < cfg.MaxAttempts {
				if matched, restoredResp := rt.skipRetryForMatchedErrorRule(req, resp); matched {
					resp = restoredResp
				} else {
					retryAfter := rt.parseRetryAfter(resp, cfg)
					if retryAfter == nil {
						// 尝试从响应体解析 retry delay
						retryAfter = rt.parseRetryDelayFromBody(resp, 64<<10) // 64KB limit
					}
					rt.logRetryAttempt(req, attempt, cfg.MaxAttempts, nil, resp)
					_ = resp.Body.Close()
					rt.backoff(req.Context(), attempt, cfg, retryAfter)
					continue
				}
			}

			// 检查是否因为空响应体需要重试（针对非流式 JSON 响应）
			if rt.shouldRetryEmptyBody(req, resp, cfg) && attempt < cfg.MaxAttempts {
				emptyBodyErr := fmt.Errorf("empty response body with status %d", resp.StatusCode)
				lastErr = emptyBodyErr
				rt.logRetryAttempt(req, attempt, cfg.MaxAttempts, emptyBodyErr, resp)
				_ = resp.Body.Close()
				rt.backoff(req.Context(), attempt, cfg, nil)
				continue
			}

		// 对流式响应进行首字节门控
		if rt.shouldGateResponse(resp) {
			// 流式请求在重试时不应再进行首字节门控（上游可能已开始处理）
			if isStreaming && attempt > 1 {
				log.Debugf("retry: skipping first-byte gate for streaming retry attempt %d", attempt)
			} else {
				firstByte, probeErr := rt.probeFirstByte(req.Context(), resp.Body, cfg.GateTimeout)
				if probeErr != nil {
					lastErr = probeErr
					_ = resp.Body.Close()
					// 流式请求首字节超时后不再重试
					if isStreaming {
						log.Warnf("retry: streaming request first-byte timeout, not retrying")
						return nil, probeErr
					}
					if attempt < cfg.MaxAttempts {
						rt.logRetryAttempt(req, attempt, cfg.MaxAttempts, probeErr, resp)
						rt.backoff(req.Context(), attempt, cfg, nil)
						continue
					}
					return nil, &RetryExhaustedError{Attempts: attempt, LastErr: probeErr}
				}

				// 成功读取首字节，回填到响应体
				resp.Body = &readCloser{
					r: io.MultiReader(bytes.NewReader(firstByte), resp.Body),
					c: resp.Body,
				}
			}
		}

		// 成功
		if attempt > 1 {
			log.Infof("retry: request succeeded after %d attempts: %s %s", attempt, req.Method, req.URL.Path)
		}
		return resp, nil
	}

	// 所有重试都失败
	if lastResp != nil {
		_ = lastResp.Body.Close()
	}
	return nil, &RetryExhaustedError{Attempts: cfg.MaxAttempts, LastErr: lastErr}
}

// cacheRequestBody 缓存请求体以支持重放
func (rt *RetryTransport) cacheRequestBody(req *http.Request, maxBytes int64) ([]byte, bool, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, true, nil
	}

	// 如果已有 GetBody，使用它
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, false, err
		}
		data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
		_ = body.Close()
		if err != nil {
			return nil, false, err
		}
		if int64(len(data)) > maxBytes {
			return nil, false, nil // 太大，不重试
		}
		return data, true, nil
	}

	// 读取并缓存请求体
	data, err := io.ReadAll(io.LimitReader(req.Body, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	_ = req.Body.Close()

	if int64(len(data)) > maxBytes {
		// 太大，重建 body 并返回不可重试
		req.Body = io.NopCloser(bytes.NewReader(data))
		return nil, false, nil
	}

	// 设置 GetBody 以支持重放
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	req.Body = io.NopCloser(bytes.NewReader(data))
	req.ContentLength = int64(len(data))

	return data, true, nil
}

// cloneRequest 克隆请求并设置新的请求体
func (rt *RetryTransport) cloneRequest(req *http.Request, bodyBytes []byte) *http.Request {
	clone := req.Clone(req.Context())
	if bodyBytes == nil {
		clone.Body = http.NoBody
		clone.ContentLength = 0
	} else {
		clone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		clone.ContentLength = int64(len(bodyBytes))
	}
	return clone
}

func (rt *RetryTransport) skipRetryForMatchedErrorRule(req *http.Request, resp *http.Response) (bool, *http.Response) {
	if resp == nil || resp.Body == nil || req == nil {
		return false, resp
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return false, resp
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	if matched := MatchErrorRule(detectIncomingFormat(req.URL.Path), resp.StatusCode, bodyBytes); matched != nil {
		log.Debugf("retry: skip retry because error rule matched %s", matched.Rule.Name)
		return true, resp
	}
	return false, resp
}

// shouldRetryError 判断网络错误是否应该重试
func (rt *RetryTransport) shouldRetryError(err error) bool {
	if err == nil {
		return false
	}

	// EOF 或 unexpected EOF
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// 网络错误
	var netErr net.Error
	if errors.As(err, &netErr) {
		// 超时或临时错误可重试
		if netErr.Timeout() {
			return true
		}
	}

	// 系统调用错误
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		// 连接重置、连接被拒绝等
		if errors.Is(opErr.Err, syscall.ECONNRESET) ||
			errors.Is(opErr.Err, syscall.ECONNREFUSED) ||
			errors.Is(opErr.Err, syscall.ETIMEDOUT) ||
			errors.Is(opErr.Err, syscall.EPIPE) {
			return true
		}
	}

	// 检查错误字符串（兜底）
	errStr := err.Error()
	retryableStrings := []string{
		"connection reset",
		"connection refused",
		"i/o timeout",
		"no such host",
		"temporary failure",
		"EOF",
		"broken pipe",
	}
	for _, s := range retryableStrings {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(s)) {
			return true
		}
	}

	return false
}

// shouldRetryStatusCode 判断状态码是否应该重试
func (rt *RetryTransport) shouldRetryStatusCode(statusCode int, cfg *RetryConfig) bool {
	if statusCode == 429 && cfg.RetryOn429 {
		return true
	}
	if statusCode >= 500 && statusCode < 600 && cfg.RetryOn5xx {
		// 500, 502, 503, 504 可重试
		return statusCode == 500 || statusCode == 502 || statusCode == 503 || statusCode == 504
	}
	return false
}

// shouldGateResponse 判断是否需要对响应进行首字节门控
func (rt *RetryTransport) shouldGateResponse(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return strings.Contains(contentType, "text/event-stream")
}

// shouldRetryEmptyBody 判断是否因为空响应体而需要重试
// 只对预期应有 body 的 2xx 响应生效，排除 204/205 等合法空响应
func (rt *RetryTransport) shouldRetryEmptyBody(req *http.Request, resp *http.Response, cfg *RetryConfig) bool {
	if !cfg.RetryOnEmptyBody {
		return false
	}

	// 只处理 2xx 成功响应
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	// 204 No Content 和 205 Reset Content 按规范就是空的
	if resp.StatusCode == 204 || resp.StatusCode == 205 {
		return false
	}

	// HEAD 请求不应该有 body
	if req.Method == http.MethodHead {
		return false
	}

	// 检查是否是 JSON 类型响应（AI API 通常返回 JSON）
	contentType := resp.Header.Get("Content-Type")
	isJSON := strings.Contains(contentType, "application/json")

	// 只对 JSON 响应检查空 body
	if !isJSON {
		return false
	}

	// 检查明确的空响应
	// 1. Body 为 NoBody
	if resp.Body == http.NoBody {
		return true
	}

	// 2. ContentLength 明确为 0
	if resp.ContentLength == 0 {
		return true
	}

	return false
}

// probeFirstByte 探测响应的第一个字节
// 修复：使用带取消的上下文确保 goroutine 不泄漏
func (rt *RetryTransport) probeFirstByte(ctx context.Context, body io.ReadCloser, timeout time.Duration) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)

	// 创建可取消的上下文，用于通知 goroutine 退出
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	go func() {
		buf := make([]byte, 1)
		n, err := body.Read(buf)
		select {
		case ch <- result{data: buf[:n], err: err}:
		case <-probeCtx.Done():
			// 上下文已取消，goroutine 退出
			// 这里不需要发送结果，主 goroutine 已经返回
		}
	}()

	select {
	case res := <-ch:
		if len(res.data) > 0 {
			return res.data, nil
		}
		if res.err == nil {
			res.err = io.EOF
		}
		return nil, res.err
	case <-probeCtx.Done():
		// 超时或上下文取消
		// 主动关闭 body，确保 goroutine 中的 Read 能返回
		body.Close()
		
		if ctx.Err() != nil {
			// 父上下文取消
			return nil, ctx.Err()
		}
		// 超时
		return nil, &FirstByteTimeoutError{Timeout_: timeout}
	}
}

// parseRetryAfter 解析 Retry-After 头
func (rt *RetryTransport) parseRetryAfter(resp *http.Response, cfg *RetryConfig) *time.Duration {
	if !cfg.RespectRetryAfter {
		return nil
	}
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return nil
	}

	// 尝试解析为秒数
	var seconds int
	if _, err := fmt.Sscanf(retryAfter, "%d", &seconds); err == nil {
		d := time.Duration(seconds) * time.Second
		return &d
	}

	// 尝试解析为 HTTP 日期
	if t, err := time.Parse(time.RFC1123, retryAfter); err == nil {
		d := time.Until(t)
		if d > 0 {
			return &d
		}
	}

	return nil
}

// parseRetryDelayFromBody 从响应体解析 retry delay
// 支持 Google API 格式: error.details[].retryDelay 或 error.details[].metadata.quotaResetDelay
func (rt *RetryTransport) parseRetryDelayFromBody(resp *http.Response, limit int64) *time.Duration {
	if resp.Body == nil || resp.Body == http.NoBody {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil || len(body) == 0 {
		return nil
	}

	// 重置 body 以便后续读取
	resp.Body = io.NopCloser(bytes.NewReader(body))

	// 使用 gjson 解析 error.details 数组
	details := gjson.GetBytes(body, "error.details")
	if !details.Exists() || !details.IsArray() {
		return nil
	}

	for _, detail := range details.Array() {
		typeVal := detail.Get("@type").String()

		// 尝试 RetryInfo.retryDelay
		if typeVal == "type.googleapis.com/google.rpc.RetryInfo" {
			retryDelay := detail.Get("retryDelay").String()
			if retryDelay != "" {
				if d, err := time.ParseDuration(retryDelay); err == nil && d > 0 {
					return &d
				}
			}
		}

		// 尝试 ErrorInfo.metadata.quotaResetDelay
		if typeVal == "type.googleapis.com/google.rpc.ErrorInfo" {
			quotaResetDelay := detail.Get("metadata.quotaResetDelay").String()
			if quotaResetDelay != "" {
				if d, err := time.ParseDuration(quotaResetDelay); err == nil && d > 0 {
					return &d
				}
			}
		}
	}

	return nil
}

// backoff 执行退避等待
func (rt *RetryTransport) backoff(ctx context.Context, attempt int, cfg *RetryConfig, retryAfter *time.Duration) {
	var delay time.Duration

	if retryAfter != nil {
		delay = *retryAfter
	} else {
		// 指数退避 + 抖动
		delay = cfg.BackoffBase * (1 << (attempt - 1))
		if delay > cfg.BackoffMax {
			delay = cfg.BackoffMax
		}
		// 添加 ±25% 的抖动
		jitter := time.Duration(rand.Float64()*float64(delay)*0.5) - delay/4
		delay += jitter
	}

	log.Debugf("retry: backing off for %v before attempt %d", delay, attempt+1)

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// logRetryAttempt 记录重试日志
func (rt *RetryTransport) logRetryAttempt(req *http.Request, attempt, maxAttempts int, err error, resp *http.Response) {
	fields := log.Fields{
		"method":      req.Method,
		"path":        req.URL.Path,
		"attempt":     attempt,
		"maxAttempts": maxAttempts,
	}

	if err != nil {
		fields["error"] = err.Error()
		fields["errorClass"] = classifyError(err)
	}

	if resp != nil {
		fields["statusCode"] = resp.StatusCode
	}

	log.WithFields(fields).Warnf("retry: attempt %d/%d failed, will retry", attempt, maxAttempts)
}

// classifyError 将错误分类为可观测的类型
func classifyError(err error) string {
	if err == nil {
		return "none"
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "eof"
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "timeout"
		}
		return "network"
	}

	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}

	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "connection reset") {
		return "reset"
	}
	if strings.Contains(errStr, "connection refused") {
		return "refused"
	}
	if strings.Contains(errStr, "dns") || strings.Contains(errStr, "no such host") {
		return "dns"
	}
	if strings.Contains(errStr, "tls") || strings.Contains(errStr, "certificate") {
		return "tls"
	}

	return "unknown"
}
