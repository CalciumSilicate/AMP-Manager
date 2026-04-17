package amp

import (
	"context"
	"sync"
	"time"

	"ampmanager/internal/service"
)

type requestTraceKey struct{}

// RequestTrace 记录请求的追踪信息，用于日志记录
// 使用指针存储在 context 中，可以在请求的不同阶段更新
type RequestTrace struct {
	mu sync.Mutex

	// 请求基本信息
	RequestID               string
	StartTime               time.Time
	UserID                  string
	APIKeyID                string
	Method                  string
	Path                    string
	OriginalModel           string
	MappedModel             string
	SessionID               string
	Provider                string
	ChannelID               string
	Endpoint                string
	RequestFormat           string
	UpstreamFormat          string
	IsStreaming             bool
	ThinkingLevel           string
	DownstreamTransport     string
	UpstreamTransport       string
	TransportFallbackReason string

	// 响应信息
	StatusCode int
	LatencyMs  int64
	TTFBMs     *int64

	// Token 使用量
	InputTokens              *int
	OutputTokens             *int
	CacheReadInputTokens     *int
	CacheCreationInputTokens *int

	// 成本信息
	CostMicros      *int64
	CostUsd         *string
	PricingModel    *string
	PricingRuleName *string

	// 计费结算结果
	BillingStatus             *string
	ChargedSubscriptionMicros *int64
	ChargedBalanceMicros      *int64

	// 倍率信息
	RateMultiplier        float64
	ChannelRateMultiplier float64
	GroupRateMultiplier   float64
	SpecialRateMultiplier float64
	SpecialRateReason     string

	// 错误信息
	ErrorType string

	// 响应文本（/v1/responses 聚合的助手文本）
	ResponseText string
}

// NewRequestTrace 创建新的请求追踪
func NewRequestTrace(requestID, userID, apiKeyID, method, path string) *RequestTrace {
	return &RequestTrace{
		RequestID:           requestID,
		StartTime:           time.Now().UTC(),
		UserID:              userID,
		APIKeyID:            apiKeyID,
		Method:              method,
		Path:                path,
		DownstreamTransport: "http",
	}
}

// WithRequestTrace 将 RequestTrace 存入 context
func WithRequestTrace(ctx context.Context, trace *RequestTrace) context.Context {
	return context.WithValue(ctx, requestTraceKey{}, trace)
}

// GetRequestTrace 从 context 获取 RequestTrace
func GetRequestTrace(ctx context.Context) *RequestTrace {
	if val := ctx.Value(requestTraceKey{}); val != nil {
		if trace, ok := val.(*RequestTrace); ok {
			return trace
		}
	}
	return nil
}

// SetModels 设置模型信息
func (t *RequestTrace) SetModels(original, mapped string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.OriginalModel = original
	t.MappedModel = mapped
}

func (t *RequestTrace) SetSessionID(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.SessionID = sessionID
}

// SetChannel 设置渠道信息
func (t *RequestTrace) SetChannel(channelID, provider, endpoint string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ChannelID = channelID
	t.Provider = provider
	t.Endpoint = endpoint
}

func (t *RequestTrace) SetFormatConversion(requestFormat, upstreamFormat string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.RequestFormat = requestFormat
	t.UpstreamFormat = upstreamFormat
}

// SetStreaming 设置是否流式
func (t *RequestTrace) SetStreaming(streaming bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.IsStreaming = streaming
}

// SetResponse 设置响应信息
func (t *RequestTrace) SetResponse(statusCode int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.StatusCode = statusCode
	t.LatencyMs = time.Since(t.StartTime).Milliseconds()
}

// MarkFirstByte 记录流式响应首字节时间
func (t *RequestTrace) MarkFirstByte() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.TTFBMs != nil {
		return
	}
	ttfbMs := time.Since(t.StartTime).Milliseconds()
	t.TTFBMs = &ttfbMs
}

// SetUsage 设置 token 使用量
func (t *RequestTrace) SetUsage(input, output, cacheRead, cacheCreation *int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if input != nil {
		t.InputTokens = input
	}
	if output != nil {
		t.OutputTokens = output
	}
	if cacheRead != nil {
		t.CacheReadInputTokens = cacheRead
	}
	if cacheCreation != nil {
		t.CacheCreationInputTokens = cacheCreation
	}
}

// UpdateOutputTokens 更新输出 token（流式时多次调用取最大值）
func (t *RequestTrace) UpdateOutputTokens(output int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.OutputTokens == nil || output > *t.OutputTokens {
		t.OutputTokens = &output
	}
}

// SetError 设置错误类型
func (t *RequestTrace) SetError(errorType string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ErrorType = errorType
}

// SetThinkingLevel 设置思维等级
func (t *RequestTrace) SetThinkingLevel(level string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ThinkingLevel = level
}

func (t *RequestTrace) SetDownstreamTransport(transport string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.DownstreamTransport = transport
}

func (t *RequestTrace) SetUpstreamTransport(transport string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.UpstreamTransport = transport
}

func (t *RequestTrace) SetTransportFallbackReason(reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TransportFallbackReason = reason
}

// SetResponseText 设置响应文本
func (t *RequestTrace) SetResponseText(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ResponseText = text
}

// copyIntPtr 深拷贝 *int 指针
func copyIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// SetCost 设置成本信息
func (t *RequestTrace) SetCost(costMicros int64, costUsd, pricingModel, pricingRuleName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.CostMicros = &costMicros
	t.CostUsd = &costUsd
	t.PricingModel = &pricingModel
	if pricingRuleName != "" {
		t.PricingRuleName = &pricingRuleName
	} else {
		t.PricingRuleName = nil
	}
}

func (t *RequestTrace) SetPricingBreakdown(groupMultiplier, channelMultiplier, specialMultiplier float64, specialReason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.GroupRateMultiplier = groupMultiplier
	t.ChannelRateMultiplier = channelMultiplier
	t.SpecialRateMultiplier = specialMultiplier
	t.SpecialRateReason = specialReason
	t.RateMultiplier = groupMultiplier * channelMultiplier * specialMultiplier
}

// SetBillingResult 设置计费结算结果，供最终 request_logs 单次落库使用。
func (t *RequestTrace) SetBillingResult(result *service.RequestBillingResult) {
	if result == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	status := result.Status
	chargedSub := result.ChargedSubscriptionMicros
	chargedBal := result.ChargedBalanceMicros
	t.BillingStatus = &status
	t.ChargedSubscriptionMicros = &chargedSub
	t.ChargedBalanceMicros = &chargedBal
}

func (t *RequestTrace) BillingResult() *service.RequestBillingResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.BillingStatus == nil && t.ChargedSubscriptionMicros == nil && t.ChargedBalanceMicros == nil {
		return nil
	}

	result := &service.RequestBillingResult{}
	if t.BillingStatus != nil {
		result.Status = *t.BillingStatus
	}
	if t.ChargedSubscriptionMicros != nil {
		result.ChargedSubscriptionMicros = *t.ChargedSubscriptionMicros
	}
	if t.ChargedBalanceMicros != nil {
		result.ChargedBalanceMicros = *t.ChargedBalanceMicros
	}
	return result
}

// Clone 获取当前状态的快照
func (t *RequestTrace) Clone() RequestTrace {
	t.mu.Lock()
	defer t.mu.Unlock()
	return RequestTrace{
		RequestID:                 t.RequestID,
		StartTime:                 t.StartTime,
		UserID:                    t.UserID,
		APIKeyID:                  t.APIKeyID,
		Method:                    t.Method,
		Path:                      t.Path,
		OriginalModel:             t.OriginalModel,
		MappedModel:               t.MappedModel,
		Provider:                  t.Provider,
		ChannelID:                 t.ChannelID,
		Endpoint:                  t.Endpoint,
		RequestFormat:             t.RequestFormat,
		UpstreamFormat:            t.UpstreamFormat,
		IsStreaming:               t.IsStreaming,
		ThinkingLevel:             t.ThinkingLevel,
		DownstreamTransport:       t.DownstreamTransport,
		UpstreamTransport:         t.UpstreamTransport,
		TransportFallbackReason:   t.TransportFallbackReason,
		StatusCode:                t.StatusCode,
		LatencyMs:                 t.LatencyMs,
		TTFBMs:                    copyInt64Ptr(t.TTFBMs),
		InputTokens:               copyIntPtr(t.InputTokens),
		OutputTokens:              copyIntPtr(t.OutputTokens),
		CacheReadInputTokens:      copyIntPtr(t.CacheReadInputTokens),
		CacheCreationInputTokens:  copyIntPtr(t.CacheCreationInputTokens),
		CostMicros:                copyInt64Ptr(t.CostMicros),
		CostUsd:                   copyStringPtr(t.CostUsd),
		PricingModel:              copyStringPtr(t.PricingModel),
		PricingRuleName:           copyStringPtr(t.PricingRuleName),
		BillingStatus:             copyStringPtr(t.BillingStatus),
		ChargedSubscriptionMicros: copyInt64Ptr(t.ChargedSubscriptionMicros),
		ChargedBalanceMicros:      copyInt64Ptr(t.ChargedBalanceMicros),
		RateMultiplier:            t.RateMultiplier,
		ChannelRateMultiplier:     t.ChannelRateMultiplier,
		GroupRateMultiplier:       t.GroupRateMultiplier,
		SpecialRateMultiplier:     t.SpecialRateMultiplier,
		SpecialRateReason:         t.SpecialRateReason,
		ErrorType:                 t.ErrorType,
		ResponseText:              t.ResponseText,
	}
}

// copyInt64Ptr 深拷贝 *int64 指针
func copyInt64Ptr(p *int64) *int64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// copyStringPtr 深拷贝 *string 指针
func copyStringPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
