package amp

import (
	"context"
	"sync"
	"time"
)

type requestTraceKey struct{}

// RequestTrace 记录请求的追踪信息，用于日志记录
// 使用指针存储在 context 中，可以在请求的不同阶段更新
type RequestTrace struct {
	mu sync.Mutex

	// 请求基本信息
	RequestID     string
	StartTime     time.Time
	UserID        string
	APIKeyID      string
	Method        string
	Path          string
	OriginalModel string
	MappedModel   string
	Provider      string
	ChannelID     string
	Endpoint      string
	IsStreaming   bool
	ThinkingLevel string

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
	CostMicros   *int64
	CostUsd      *string
	PricingModel *string

	// 倍率信息
	RateMultiplier float64

	// 错误信息
	ErrorType string

	// 响应文本（/v1/responses 聚合的助手文本）
	ResponseText string
}

// NewRequestTrace 创建新的请求追踪
func NewRequestTrace(requestID, userID, apiKeyID, method, path string) *RequestTrace {
	return &RequestTrace{
		RequestID: requestID,
		StartTime: time.Now().UTC(),
		UserID:    userID,
		APIKeyID:  apiKeyID,
		Method:    method,
		Path:      path,
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

// SetChannel 设置渠道信息
func (t *RequestTrace) SetChannel(channelID, provider, endpoint string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ChannelID = channelID
	t.Provider = provider
	t.Endpoint = endpoint
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
func (t *RequestTrace) SetCost(costMicros int64, costUsd, pricingModel string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.CostMicros = &costMicros
	t.CostUsd = &costUsd
	t.PricingModel = &pricingModel
}

// Clone 获取当前状态的快照
func (t *RequestTrace) Clone() RequestTrace {
	t.mu.Lock()
	defer t.mu.Unlock()
	return RequestTrace{
		RequestID:                t.RequestID,
		StartTime:                t.StartTime,
		UserID:                   t.UserID,
		APIKeyID:                 t.APIKeyID,
		Method:                   t.Method,
		Path:                     t.Path,
		OriginalModel:            t.OriginalModel,
		MappedModel:              t.MappedModel,
		Provider:                 t.Provider,
		ChannelID:                t.ChannelID,
		Endpoint:                 t.Endpoint,
		IsStreaming:              t.IsStreaming,
		ThinkingLevel:            t.ThinkingLevel,
		StatusCode:               t.StatusCode,
		LatencyMs:                t.LatencyMs,
		TTFBMs:                   copyInt64Ptr(t.TTFBMs),
		InputTokens:              copyIntPtr(t.InputTokens),
		OutputTokens:             copyIntPtr(t.OutputTokens),
		CacheReadInputTokens:     copyIntPtr(t.CacheReadInputTokens),
		CacheCreationInputTokens: copyIntPtr(t.CacheCreationInputTokens),
		CostMicros:               copyInt64Ptr(t.CostMicros),
		CostUsd:                  copyStringPtr(t.CostUsd),
		PricingModel:             copyStringPtr(t.PricingModel),
		RateMultiplier:           t.RateMultiplier,
		ErrorType:                t.ErrorType,
		ResponseText:             t.ResponseText,
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
