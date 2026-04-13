package amp

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// ConnectionHealthConfig 连接健康检查配置
type ConnectionHealthConfig struct {
	// 写入超时 - 检测客户端断开
	WriteTimeout time.Duration
	// 读取空闲超时 - 上游无数据时的最大等待时间
	ReadIdleTimeout time.Duration
	// 是否启用详细日志
	VerboseLogging bool
}

// DefaultConnectionHealthConfig 默认配置
func DefaultConnectionHealthConfig() *ConnectionHealthConfig {
	return &ConnectionHealthConfig{
		WriteTimeout:    30 * time.Second,
		ReadIdleTimeout: 300 * time.Second, // 5分钟，允许AI长时间思考
		VerboseLogging:  false,
	}
}

var (
	globalConnectionHealthConfig *ConnectionHealthConfig
	connectionHealthConfigMu     sync.RWMutex
)

// GetConnectionHealthConfig 获取当前连接健康检查配置
func GetConnectionHealthConfig() *ConnectionHealthConfig {
	connectionHealthConfigMu.RLock()
	defer connectionHealthConfigMu.RUnlock()
	if globalConnectionHealthConfig == nil {
		return DefaultConnectionHealthConfig()
	}
	return globalConnectionHealthConfig
}

// UpdateConnectionHealthConfig 更新连接健康检查配置
func UpdateConnectionHealthConfig(readIdleTimeout time.Duration) {
	connectionHealthConfigMu.Lock()
	defer connectionHealthConfigMu.Unlock()
	globalConnectionHealthConfig = &ConnectionHealthConfig{
		WriteTimeout:    30 * time.Second,
		ReadIdleTimeout: readIdleTimeout,
		VerboseLogging:  false,
	}
}

// HealthyStreamWrapper 健康流包装器
// 解决 "terminated" 问题的关键组件
type HealthyStreamWrapper struct {
	reader       io.ReadCloser
	trace        *RequestTrace
	cfg          *ConnectionHealthConfig
	lastReadTime time.Time
	mu           sync.Mutex
	closed       bool
	closeOnce    sync.Once
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewHealthyStreamWrapper 创建健康流包装器
func NewHealthyStreamWrapper(ctx context.Context, reader io.ReadCloser, trace *RequestTrace, cfg *ConnectionHealthConfig) *HealthyStreamWrapper {
	if cfg == nil {
		cfg = GetConnectionHealthConfig()
	}
	wrapCtx, cancel := context.WithCancel(ctx)
	return &HealthyStreamWrapper{
		reader:       reader,
		trace:        trace,
		cfg:          cfg,
		lastReadTime: time.Now(),
		ctx:          wrapCtx,
		cancel:       cancel,
	}
}

// deadlineReader 检测底层是否支持 SetReadDeadline
type deadlineReader interface {
	SetReadDeadline(t time.Time) error
}

// Read 带健康检查的读取（同步化，无数据竞争）
func (h *HealthyStreamWrapper) Read(p []byte) (int, error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	h.mu.Unlock()

	// 检查上下文是否已取消
	select {
	case <-h.ctx.Done():
		return 0, h.ctx.Err()
	default:
	}

	// 尝试设置读取超时（如果底层支持）
	if dr, ok := h.reader.(deadlineReader); ok {
		deadline := time.Now().Add(h.cfg.ReadIdleTimeout)
		_ = dr.SetReadDeadline(deadline)
		defer dr.SetReadDeadline(time.Time{}) // 清除 deadline
	}

	// 同步读取 - 无 goroutine，无数据竞争
	n, err := h.reader.Read(p)

	// 更新最后读取时间
	if n > 0 {
		h.mu.Lock()
		h.lastReadTime = time.Now()
		h.mu.Unlock()
		if h.trace != nil {
			h.trace.MarkFirstByte()
		}
	}

	// 处理超时错误
	if err != nil {
		if isTimeoutError(err) {
			h.logConnectionIssue("read idle timeout", err)
			h.forceClose()
			return n, &StreamTimeoutError{
				Duration: h.cfg.ReadIdleTimeout,
				Phase:    "read_idle",
			}
		}
		// 检查 context 取消
		select {
		case <-h.ctx.Done():
			return n, h.ctx.Err()
		default:
		}
	}

	return n, err
}

// isTimeoutError 检查是否为超时错误
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// Close 安全关闭
func (h *HealthyStreamWrapper) Close() error {
	var closeErr error
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.closed = true
		h.mu.Unlock()
		h.cancel()
		closeErr = h.reader.Close()

		// 更新 trace 状态
		if h.trace != nil {
			h.trace.SetResponse(200) // 正常关闭
		}
	})
	return closeErr
}

// forceClose 强制关闭（错误场景）
func (h *HealthyStreamWrapper) forceClose() {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.closed = true
		h.mu.Unlock()
		h.cancel()
		_ = h.reader.Close()

		// 更新 trace 状态为错误
		if h.trace != nil {
			h.trace.SetError("stream_timeout")
		}
	})
}

func (h *HealthyStreamWrapper) logConnectionIssue(msg string, err error) {
	if h.cfg.VerboseLogging {
		h.mu.Lock()
		lastRead := h.lastReadTime
		h.mu.Unlock()

		fields := log.Fields{
			"lastRead": lastRead,
			"elapsed":  time.Since(lastRead),
		}
		if err != nil {
			fields["error"] = err.Error()
		}
		log.WithFields(fields).Warnf("stream health: %s", msg)
	}
}

// StreamTimeoutError 流超时错误
type StreamTimeoutError struct {
	Duration time.Duration
	Phase    string
}

func (e *StreamTimeoutError) Error() string {
	return "stream " + e.Phase + " timeout after " + e.Duration.String()
}

func (e *StreamTimeoutError) Timeout() bool   { return true }
func (e *StreamTimeoutError) Temporary() bool { return true }

// IsClientDisconnect 判断错误是否是客户端主动断开
// 修复: 只对明确的断开场景返回 true，不再将所有 net.Error 都判为客户端断开
func IsClientDisconnect(err error) bool {
	if err == nil {
		return false
	}

	// 使用统一的错误分类器
	class := ClassifyError(err, "")
	return class == ErrorClassClientClosed
}

// LogStreamError 记录流错误（带分类）
func LogStreamError(err error, requestID string) {
	if err == nil {
		return
	}

	category := ClassifyStreamError(err)
	fields := log.Fields{
		"requestID": requestID,
		"category":  category,
	}

	switch category {
	case "client_disconnect":
		log.WithFields(fields).Debug("stream: client disconnected")
	case "upstream_timeout":
		log.WithFields(fields).Warn("stream: upstream timeout")
	case "network_error":
		log.WithFields(fields).Warn("stream: network error: " + err.Error())
	default:
		log.WithFields(fields).Error("stream: unexpected error: " + err.Error())
	}
}

// ClassifyStreamError 分类流错误
// 使用统一的错误分类器，返回兼容的字符串分类
func ClassifyStreamError(err error) string {
	if err == nil {
		return "none"
	}

	// 使用统一的错误分类器
	class := ClassifyError(err, "read")

	// 映射到旧的字符串分类（保持向后兼容）
	switch class {
	case ErrorClassTimeout:
		return "upstream_timeout"
	case ErrorClassClientClosed:
		return "client_disconnect"
	case ErrorClassServerCancel:
		return "server_cancel"
	case ErrorClassBackpressure:
		return "backpressure"
	case ErrorClassUpstreamReset:
		return "upstream_closed"
	case ErrorClassProtocol:
		return "protocol_error"
	default:
		// 对于未知错误，进行细化检查
		var netErr net.Error
		if errors.As(err, &netErr) {
			return "network_error"
		}
		return "unknown"
	}
}
