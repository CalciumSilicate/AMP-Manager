package amp

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"sync"
	"time"

	"ampmanager/internal/billing"
	"ampmanager/internal/model"
	"ampmanager/internal/realtime"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	log "github.com/sirupsen/logrus"
)

// LogEntryStatus 日志条目状态
type LogEntryStatus string

const (
	LogEntryStatusPending LogEntryStatus = "pending"
	LogEntryStatusSuccess LogEntryStatus = "success"
	LogEntryStatusError   LogEntryStatus = "error"
)

// LogEntry 日志条目，用于写入数据库
type LogEntry struct {
	ID                       string
	CreatedAt                time.Time
	UpdatedAt                *time.Time
	Status                   LogEntryStatus
	UserID                   string
	APIKeyID                 string
	OriginalModel            *string
	MappedModel              *string
	Provider                 *string
	ChannelID                *string
	Endpoint                 *string
	Method                   string
	Path                     string
	StatusCode               int
	LatencyMs                int64
	TTFBMs                   *int64
	IsStreaming              bool
	InputTokens              *int
	OutputTokens             *int
	CacheReadInputTokens     *int
	CacheCreationInputTokens *int
	ErrorType                *string
	RequestID                *string
	ThinkingLevel            *string
	DownstreamTransport      *string
	UpstreamTransport        *string
	TransportFallbackReason  *string
	// 成本相关
	CostMicros            *int64
	CostUsd               *string
	PricingModel          *string
	PricingRuleName       *string
	RateMultiplier        *float64
	ChannelRateMultiplier *float64
	GroupRateMultiplier   *float64
	SpecialRateMultiplier *float64
	SpecialRateReason     *string
}

// LogWriter 异步批量日志写入器
type LogWriter struct {
	db            *sql.DB
	entryChan     chan LogEntry
	batchSize     int
	flushInterval time.Duration
	wg            sync.WaitGroup
	stopChan      chan struct{}
	stopped       bool
	mu            sync.Mutex
}

// NewLogWriter 创建日志写入器
func NewLogWriter(db *sql.DB, bufferSize, batchSize int, flushInterval time.Duration) *LogWriter {
	w := &LogWriter{
		db:            db,
		entryChan:     make(chan LogEntry, bufferSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stopChan:      make(chan struct{}),
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// Write 异步写入日志（非阻塞）
func (w *LogWriter) Write(entry LogEntry) bool {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return false
	}
	w.mu.Unlock()

	select {
	case w.entryChan <- entry:
		return true
	default:
		log.Warn("log writer: queue full, dropping entry")
		return false
	}
}

// WritePendingFromTrace 同步写入 pending 状态的日志记录
// 使用 trace.RequestID 作为数据库 ID，以便后续 UPDATE
func (w *LogWriter) WritePendingFromTrace(trace *RequestTrace) bool {
	if trace == nil || trace.RequestID == "" {
		return false
	}

	snapshot := trace.Clone()

	// 构建可选字段
	var originalModel, mappedModel, sessionID, provider, channelID, endpoint, requestFormat, upstreamFormat *string
	if snapshot.OriginalModel != "" {
		originalModel = &snapshot.OriginalModel
	}
	if snapshot.MappedModel != "" {
		mappedModel = &snapshot.MappedModel
	}
	if snapshot.SessionID != "" {
		sessionID = &snapshot.SessionID
	}
	if snapshot.Provider != "" {
		provider = &snapshot.Provider
	}
	if snapshot.ChannelID != "" {
		channelID = &snapshot.ChannelID
	}
	if snapshot.Endpoint != "" {
		endpoint = &snapshot.Endpoint
	}
	if snapshot.RequestFormat != "" {
		requestFormat = &snapshot.RequestFormat
	}
	if snapshot.UpstreamFormat != "" {
		upstreamFormat = &snapshot.UpstreamFormat
	}

	// 同步写入数据库（pending 记录需要立即可见）
	_, err := w.db.Exec(`
		INSERT INTO request_logs (
			id, created_at, status, user_id, api_key_id, original_model, mapped_model,
			session_id, provider, channel_id, endpoint, request_format, upstream_format, method, path, status_code, latency_ms, ttfb_ms, is_streaming, downstream_transport, upstream_transport
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		snapshot.RequestID, // 使用 RequestID 作为数据库 ID
		snapshot.StartTime.UTC(),
		LogEntryStatusPending,
		snapshot.UserID,
		snapshot.APIKeyID,
		originalModel,
		mappedModel,
		sessionID,
		provider,
		channelID,
		endpoint,
		requestFormat,
		upstreamFormat,
		snapshot.Method,
		snapshot.Path,
		0, // pending 时 status_code 为 0
		0, // pending 时 latency_ms 为 0
		nil,
		0, // pending 时 is_streaming 为 0
		stringPtrIfNonEmpty(snapshot.DownstreamTransport),
		stringPtrIfNonEmpty(snapshot.UpstreamTransport),
	)

	if err != nil {
		log.Errorf("log writer: failed to insert pending entry: %v", err)
		return false
	}

	log.Debugf("log writer: inserted pending request %s", snapshot.RequestID)
	return true
}

// UpdateFromTrace 更新已存在的 pending 记录为完成状态
func (w *LogWriter) UpdateFromTrace(trace *RequestTrace) bool {
	if trace == nil || trace.RequestID == "" {
		return false
	}
	w.FinalizeAPIKeyCircuitBreaker(trace)

	snapshot := trace.Clone()

	// 确定最终状态
	status := LogEntryStatusSuccess
	if snapshot.ErrorType != "" || snapshot.StatusCode >= 400 {
		status = LogEntryStatusError
	}

	now := time.Now().UTC()
	isStreaming := 0
	if snapshot.IsStreaming {
		isStreaming = 1
	}

	// 构建可选字段
	var originalModel, mappedModel, sessionID, provider, channelID, endpoint, requestFormat, upstreamFormat, errorType, pricingModel, pricingRuleName, costUsd, billingStatus, specialRateReason *string
	var chargedSubscriptionMicros, chargedBalanceMicros *int64
	if snapshot.OriginalModel != "" {
		originalModel = &snapshot.OriginalModel
	}
	if snapshot.MappedModel != "" {
		mappedModel = &snapshot.MappedModel
	}
	if snapshot.SessionID != "" {
		sessionID = &snapshot.SessionID
	}
	if snapshot.Provider != "" {
		provider = &snapshot.Provider
	}
	if snapshot.ChannelID != "" {
		channelID = &snapshot.ChannelID
	}
	if snapshot.Endpoint != "" {
		endpoint = &snapshot.Endpoint
	}
	if snapshot.RequestFormat != "" {
		requestFormat = &snapshot.RequestFormat
	}
	if snapshot.UpstreamFormat != "" {
		upstreamFormat = &snapshot.UpstreamFormat
	}
	if snapshot.ErrorType != "" {
		errorType = &snapshot.ErrorType
	}
	if snapshot.PricingModel != nil {
		pricingModel = snapshot.PricingModel
	}
	if snapshot.PricingRuleName != nil {
		pricingRuleName = snapshot.PricingRuleName
	}
	if snapshot.CostUsd != nil {
		costUsd = snapshot.CostUsd
	}
	if snapshot.BillingStatus != nil {
		billingStatus = snapshot.BillingStatus
	}
	if snapshot.ChargedSubscriptionMicros != nil {
		chargedSubscriptionMicros = snapshot.ChargedSubscriptionMicros
	}
	if snapshot.ChargedBalanceMicros != nil {
		chargedBalanceMicros = snapshot.ChargedBalanceMicros
	}

	var thinkingLevel *string
	if snapshot.ThinkingLevel != "" {
		thinkingLevel = &snapshot.ThinkingLevel
	}
	var downstreamTransport, upstreamTransport, transportFallbackReason *string
	if snapshot.DownstreamTransport != "" {
		downstreamTransport = &snapshot.DownstreamTransport
	}
	if snapshot.UpstreamTransport != "" {
		upstreamTransport = &snapshot.UpstreamTransport
	}
	if snapshot.TransportFallbackReason != "" {
		transportFallbackReason = &snapshot.TransportFallbackReason
	}
	if snapshot.SpecialRateReason != "" {
		specialRateReason = &snapshot.SpecialRateReason
	}

	var rateMultiplier *float64
	if snapshot.RateMultiplier != 0 {
		rm := snapshot.RateMultiplier
		rateMultiplier = &rm
	}
	var channelRateMultiplier *float64
	if snapshot.ChannelRateMultiplier != 0 {
		rm := snapshot.ChannelRateMultiplier
		channelRateMultiplier = &rm
	}
	var groupRateMultiplier *float64
	if snapshot.GroupRateMultiplier != 0 {
		rm := snapshot.GroupRateMultiplier
		groupRateMultiplier = &rm
	}
	var specialRateMultiplier *float64
	if snapshot.SpecialRateMultiplier != 0 {
		rm := snapshot.SpecialRateMultiplier
		specialRateMultiplier = &rm
	}

	tx, err := w.db.Begin()
	if err != nil {
		log.Errorf("log writer: failed to begin transaction for %s: %v", snapshot.RequestID, err)
		return false
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE request_logs SET
			updated_at = ?,
			status = ?,
			original_model = COALESCE(?, original_model),
			mapped_model = COALESCE(?, mapped_model),
			session_id = COALESCE(?, session_id),
			provider = COALESCE(?, provider),
			channel_id = COALESCE(?, channel_id),
			endpoint = COALESCE(?, endpoint),
			request_format = COALESCE(?, request_format),
			upstream_format = COALESCE(?, upstream_format),
			status_code = ?,
			latency_ms = ?,
			ttfb_ms = ?,
			is_streaming = ?,
			input_tokens = ?,
			output_tokens = ?,
			cache_read_input_tokens = ?,
			cache_creation_input_tokens = ?,
			error_type = ?,
			cost_micros = ?,
			cost_usd = ?,
			pricing_model = ?,
			pricing_rule_name = COALESCE(?, pricing_rule_name),
			charged_subscription_micros = COALESCE(?, charged_subscription_micros),
			charged_balance_micros = COALESCE(?, charged_balance_micros),
			billing_status = COALESCE(?, billing_status),
			thinking_level = COALESCE(?, thinking_level),
			downstream_transport = COALESCE(?, downstream_transport),
			upstream_transport = COALESCE(?, upstream_transport),
			transport_fallback_reason = COALESCE(?, transport_fallback_reason),
			rate_multiplier = COALESCE(?, rate_multiplier),
			channel_rate_multiplier = COALESCE(?, channel_rate_multiplier),
			group_rate_multiplier = COALESCE(?, group_rate_multiplier),
			special_rate_multiplier = COALESCE(?, special_rate_multiplier),
			special_rate_reason = COALESCE(?, special_rate_reason),
			response_text = COALESCE(?, response_text)
		WHERE id = ?
	`,
		now,
		status,
		originalModel,
		mappedModel,
		sessionID,
		provider,
		channelID,
		endpoint,
		requestFormat,
		upstreamFormat,
		snapshot.StatusCode,
		snapshot.LatencyMs,
		snapshot.TTFBMs,
		isStreaming,
		snapshot.InputTokens,
		snapshot.OutputTokens,
		snapshot.CacheReadInputTokens,
		snapshot.CacheCreationInputTokens,
		errorType,
		snapshot.CostMicros,
		costUsd,
		pricingModel,
		pricingRuleName,
		chargedSubscriptionMicros,
		chargedBalanceMicros,
		billingStatus,
		thinkingLevel,
		downstreamTransport,
		upstreamTransport,
		transportFallbackReason,
		rateMultiplier,
		channelRateMultiplier,
		groupRateMultiplier,
		specialRateMultiplier,
		specialRateReason,
		stringPtrIfNonEmpty(snapshot.ResponseText),
		snapshot.RequestID,
	)
	if err != nil {
		log.Errorf("log writer: failed to update entry %s: %v", snapshot.RequestID, err)
		return false
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Warnf("log writer: pending record not found for %s, inserting new", snapshot.RequestID)
		if err := w.insertCompleteTx(tx, snapshot, status, now); err != nil {
			log.Errorf("log writer: failed to insert fallback entry %s: %v", snapshot.RequestID, err)
			return false
		}
	}

	if err := syncGlobalRequestMetricsTx(tx, snapshot, now); err != nil {
		log.Errorf("log writer: failed to sync minute metrics for %s: %v", snapshot.RequestID, err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Errorf("log writer: failed to commit entry %s: %v", snapshot.RequestID, err)
		return false
	}

	log.Debugf("log writer: updated request %s to status %s", snapshot.RequestID, status)
	realtime.NotifyLogCompleted(snapshot.RequestID)
	return true
}

func (w *LogWriter) FinalizeAPIKeyCircuitBreaker(trace *RequestTrace) {
	if trace == nil || trace.APIKeyID == "" {
		return
	}
	if !trace.CompleteAPIKeyCircuitBreakerOutcome() {
		return
	}

	repo := repository.NewAPIKeyRepository()
	key, err := repo.GetByID(trace.APIKeyID)
	if err != nil || key == nil {
		if err != nil {
			log.Warnf("log writer: failed to load api key circuit breaker state for %s: %v", trace.APIKeyID, err)
		}
		return
	}

	service.WithAPIKeyCircuitBreakerLock(trace.APIKeyID, func() {
		now := time.Now().UTC()
		if model.ApplyAPIKeyCircuitBreakerOutcome(key, trace.APIKeyCircuitBreakerOutcome(), now) {
			if err := repo.UpdateCircuitBreakerState(key); err != nil {
				log.Warnf("log writer: failed to persist api key circuit breaker state for %s: %v", trace.APIKeyID, err)
			}
		}
	})
}

// insertComplete 直接插入完整记录（fallback 用于 pending 记录丢失的情况）
func (w *LogWriter) insertComplete(trace *RequestTrace) bool {
	snapshot := trace.Clone()

	status := LogEntryStatusSuccess
	if snapshot.ErrorType != "" || snapshot.StatusCode >= 400 {
		status = LogEntryStatusError
	}

	now := time.Now().UTC()

	tx, err := w.db.Begin()
	if err != nil {
		log.Errorf("log writer: failed to begin insert transaction: %v", err)
		return false
	}
	defer tx.Rollback()

	if err := w.insertCompleteTx(tx, snapshot, status, now); err != nil {
		log.Errorf("log writer: failed to insert complete entry: %v", err)
		return false
	}
	if err := syncGlobalRequestMetricsTx(tx, snapshot, now); err != nil {
		log.Errorf("log writer: failed to sync minute metrics for %s: %v", snapshot.RequestID, err)
		return false
	}
	if err := tx.Commit(); err != nil {
		log.Errorf("log writer: failed to commit inserted entry: %v", err)
		return false
	}
	realtime.NotifyLogCompleted(snapshot.RequestID)
	return true
}

func (w *LogWriter) insertCompleteTx(tx *sql.Tx, snapshot RequestTrace, status LogEntryStatus, now time.Time) error {
	isStreaming := 0
	if snapshot.IsStreaming {
		isStreaming = 1
	}

	var originalModel, mappedModel, sessionID, provider, channelID, endpoint, requestFormat, upstreamFormat, errorType, pricingModel, pricingRuleName, costUsd, billingStatus, specialRateReason *string
	var chargedSubscriptionMicros, chargedBalanceMicros *int64
	if snapshot.OriginalModel != "" {
		originalModel = &snapshot.OriginalModel
	}
	if snapshot.MappedModel != "" {
		mappedModel = &snapshot.MappedModel
	}
	if snapshot.SessionID != "" {
		sessionID = &snapshot.SessionID
	}
	if snapshot.Provider != "" {
		provider = &snapshot.Provider
	}
	if snapshot.ChannelID != "" {
		channelID = &snapshot.ChannelID
	}
	if snapshot.Endpoint != "" {
		endpoint = &snapshot.Endpoint
	}
	if snapshot.RequestFormat != "" {
		requestFormat = &snapshot.RequestFormat
	}
	if snapshot.UpstreamFormat != "" {
		upstreamFormat = &snapshot.UpstreamFormat
	}
	if snapshot.ErrorType != "" {
		errorType = &snapshot.ErrorType
	}
	if snapshot.PricingModel != nil {
		pricingModel = snapshot.PricingModel
	}
	if snapshot.PricingRuleName != nil {
		pricingRuleName = snapshot.PricingRuleName
	}
	if snapshot.CostUsd != nil {
		costUsd = snapshot.CostUsd
	}
	if snapshot.BillingStatus != nil {
		billingStatus = snapshot.BillingStatus
	}
	if snapshot.ChargedSubscriptionMicros != nil {
		chargedSubscriptionMicros = snapshot.ChargedSubscriptionMicros
	}
	if snapshot.ChargedBalanceMicros != nil {
		chargedBalanceMicros = snapshot.ChargedBalanceMicros
	}
	var thinkingLevel *string
	if snapshot.ThinkingLevel != "" {
		thinkingLevel = &snapshot.ThinkingLevel
	}
	var downstreamTransport, upstreamTransport, transportFallbackReason *string
	if snapshot.DownstreamTransport != "" {
		downstreamTransport = &snapshot.DownstreamTransport
	}
	if snapshot.UpstreamTransport != "" {
		upstreamTransport = &snapshot.UpstreamTransport
	}
	if snapshot.TransportFallbackReason != "" {
		transportFallbackReason = &snapshot.TransportFallbackReason
	}
	if snapshot.SpecialRateReason != "" {
		specialRateReason = &snapshot.SpecialRateReason
	}
	var rateMultiplier *float64
	if snapshot.RateMultiplier != 0 {
		rm := snapshot.RateMultiplier
		rateMultiplier = &rm
	}
	var channelRateMultiplier *float64
	if snapshot.ChannelRateMultiplier != 0 {
		rm := snapshot.ChannelRateMultiplier
		channelRateMultiplier = &rm
	}
	var groupRateMultiplier *float64
	if snapshot.GroupRateMultiplier != 0 {
		rm := snapshot.GroupRateMultiplier
		groupRateMultiplier = &rm
	}
	var specialRateMultiplier *float64
	if snapshot.SpecialRateMultiplier != 0 {
		rm := snapshot.SpecialRateMultiplier
		specialRateMultiplier = &rm
	}

	_, err := tx.Exec(`
		INSERT INTO request_logs (
			id, created_at, updated_at, status, user_id, api_key_id, original_model, mapped_model,
			session_id, provider, channel_id, endpoint, request_format, upstream_format, method, path, status_code, latency_ms, ttfb_ms,
			is_streaming, input_tokens, output_tokens, cache_read_input_tokens,
			cache_creation_input_tokens, error_type, cost_micros, cost_usd, pricing_model, pricing_rule_name,
			charged_subscription_micros, charged_balance_micros, billing_status, thinking_level,
			downstream_transport, upstream_transport, transport_fallback_reason, rate_multiplier,
			channel_rate_multiplier, group_rate_multiplier, special_rate_multiplier, special_rate_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
		snapshot.RequestID,
		snapshot.StartTime.UTC(),
		now,
		status,
		snapshot.UserID,
		snapshot.APIKeyID,
		originalModel,
		mappedModel,
		sessionID,
		provider,
		channelID,
		endpoint,
		requestFormat,
		upstreamFormat,
		snapshot.Method,
		snapshot.Path,
		snapshot.StatusCode,
		snapshot.LatencyMs,
		snapshot.TTFBMs,
		isStreaming,
		snapshot.InputTokens,
		snapshot.OutputTokens,
		snapshot.CacheReadInputTokens,
		snapshot.CacheCreationInputTokens,
		errorType,
		snapshot.CostMicros,
		costUsd,
		pricingModel,
		pricingRuleName,
		chargedSubscriptionMicros,
		chargedBalanceMicros,
		billingStatus,
		thinkingLevel,
		downstreamTransport,
		upstreamTransport,
		transportFallbackReason,
		rateMultiplier,
		channelRateMultiplier,
		groupRateMultiplier,
		specialRateMultiplier,
		specialRateReason,
	)
	return err
}

// WriteFromTrace 直接写入完整日志记录（用于非 pending 工作流，如非模型调用请求）
func (w *LogWriter) WriteFromTrace(trace *RequestTrace) bool {
	if trace == nil || trace.RequestID == "" {
		return false
	}
	return w.insertComplete(trace)
}

// Stop 停止写入器并刷新剩余日志
func (w *LogWriter) Stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true
	w.mu.Unlock()

	close(w.stopChan)
	w.wg.Wait()
}

// run 后台运行的写入循环
func (w *LogWriter) run() {
	defer w.wg.Done()

	batch := make([]LogEntry, 0, w.batchSize)
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case entry := <-w.entryChan:
			batch = append(batch, entry)
			if len(batch) >= w.batchSize {
				w.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.flush(batch)
				batch = batch[:0]
			}
		case <-w.stopChan:
			// 处理剩余的日志
			close(w.entryChan)
			for entry := range w.entryChan {
				batch = append(batch, entry)
			}
			if len(batch) > 0 {
				w.flush(batch)
			}
			return
		}
	}
}

// flush 批量写入数据库
func (w *LogWriter) flush(entries []LogEntry) {
	if len(entries) == 0 {
		return
	}

	tx, err := w.db.Begin()
	if err != nil {
		log.Errorf("log writer: failed to begin transaction: %v", err)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT INTO request_logs (
			id, created_at, updated_at, status, user_id, api_key_id, original_model, mapped_model,
			provider, channel_id, endpoint, method, path, status_code, latency_ms, ttfb_ms,
			is_streaming, input_tokens, output_tokens, cache_read_input_tokens,
			cache_creation_input_tokens, error_type
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		log.Errorf("log writer: failed to prepare statement: %v", err)
		tx.Rollback()
		return
	}
	defer stmt.Close()

	for _, e := range entries {
		isStreaming := 0
		if e.IsStreaming {
			isStreaming = 1
		}

		_, err := stmt.Exec(
			e.ID, e.CreatedAt.UTC(), e.CreatedAt.UTC(), LogEntryStatusSuccess, e.UserID, e.APIKeyID, e.OriginalModel, e.MappedModel,
			e.Provider, e.ChannelID, e.Endpoint, e.Method, e.Path, e.StatusCode, e.LatencyMs,
			e.TTFBMs,
			isStreaming, e.InputTokens, e.OutputTokens, e.CacheReadInputTokens,
			e.CacheCreationInputTokens, e.ErrorType,
		)
		if err != nil {
			log.Errorf("log writer: failed to insert entry: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Errorf("log writer: failed to commit transaction: %v", err)
		tx.Rollback()
		return
	}

	log.Debugf("log writer: flushed %d entries", len(entries))
}

// 全局日志写入器实例
var (
	globalLogWriter *LogWriter
	logWriterOnce   sync.Once
	logWriterMu     sync.Mutex
)

// InitLogWriter 初始化全局日志写入器
func InitLogWriter(db *sql.DB) {
	logWriterOnce.Do(func() {
		globalLogWriter = NewLogWriter(db, 10000, 100, 200*time.Millisecond)
		log.Info("log writer: initialized")
	})
}

// ReinitLogWriter 重新初始化全局日志写入器（数据库替换后调用）
func ReinitLogWriter(db *sql.DB) {
	logWriterMu.Lock()
	defer logWriterMu.Unlock()
	if globalLogWriter != nil {
		globalLogWriter.Stop()
	}
	globalLogWriter = NewLogWriter(db, 10000, 100, 200*time.Millisecond)
	log.Info("log writer: reinitialized")
}

// GetLogWriter 获取全局日志写入器
func GetLogWriter() *LogWriter {
	return globalLogWriter
}

// StopLogWriter 停止全局日志写入器
func StopLogWriter() {
	if globalLogWriter != nil {
		globalLogWriter.Stop()
		log.Info("log writer: stopped")
	}
}

// LoggingBodyWrapper 包装响应体，在 Close 时写入日志
type LoggingBodyWrapper struct {
	io.ReadCloser
	trace      *RequestTrace
	statusCode int
	once       sync.Once
	ctx        context.Context
}

// NewLoggingBodyWrapper 创建日志包装器
func NewLoggingBodyWrapper(body io.ReadCloser, trace *RequestTrace, statusCode int, ctx context.Context) *LoggingBodyWrapper {
	return &LoggingBodyWrapper{
		ReadCloser: body,
		trace:      trace,
		statusCode: statusCode,
		ctx:        ctx,
	}
}

// Close 关闭并更新日志记录
func (w *LoggingBodyWrapper) Close() error {
	err := w.ReadCloser.Close()
	w.once.Do(func() {
		if w.trace != nil {
			w.trace.SetResponse(w.statusCode)

			// 计算成本（在设置 usage 之后）
			if calc := billing.GetCostCalculator(); calc != nil {
				// 使用 MappedModel 作为计价模型（如果没有则使用 OriginalModel）
				pricingModel := w.trace.MappedModel
				if pricingModel == "" {
					pricingModel = w.trace.OriginalModel
				}
				if pricingModel != "" {
					costResult := calc.CalculateFromPointers(
						pricingModel,
						w.trace.InputTokens,
						w.trace.OutputTokens,
						w.trace.CacheReadInputTokens,
						w.trace.CacheCreationInputTokens,
					)
					if costResult.PriceFound {
						multiplier := 1.0
						var proxyCfg *ProxyConfig
						if w.ctx != nil {
							proxyCfg = GetProxyConfig(w.ctx)
						}
						multiplier = traceMultiplier(w.trace)

						if multiplier == 0 {
							w.trace.SetCost(costResult.CostMicros, costResult.CostUsd, costResult.PricingModel, costResult.PricingRuleName)
						} else {
							adjustedCostMicros := int64(float64(costResult.CostMicros) * multiplier)
							adjustedCostUsd := fmt.Sprintf("%.6f", float64(adjustedCostMicros)/1e6)
							w.trace.SetCost(adjustedCostMicros, adjustedCostUsd, costResult.PricingModel, costResult.PricingRuleName)

							if proxyCfg != nil && adjustedCostMicros > 0 {
								billingSvc := service.NewBillingService()
								result, err := billingSvc.SettleRequestCostResultWithSource(w.trace.RequestID, proxyCfg.UserID, adjustedCostMicros, proxyCfg.ForcedBillingSource)
								if err != nil {
									log.Warnf("log writer: failed to settle cost for user %s: %v", proxyCfg.UserID, err)
								} else {
									w.trace.SetBillingResult(result)
								}
							}
						}
					}
				}
			}

			if writer := GetLogWriter(); writer != nil {
				if ok := writer.UpdateFromTrace(w.trace); !ok {
					if billingResult := w.trace.BillingResult(); billingResult != nil {
						billingSvc := service.NewBillingService()
						if err := billingSvc.ApplyBillingResult(w.trace.RequestID, billingResult); err != nil {
							log.Warnf("log writer: failed to apply billing fallback for request %s: %v", w.trace.RequestID, err)
						}
					}
				}
			}

			FinalizeSessionSticky(w.ctx, w.trace)
		}
	})
	return err
}

func stringPtrIfNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
