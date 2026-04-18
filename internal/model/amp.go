package model

import "time"

// WebSearchMode constants
const (
	WebSearchModeUpstream    = "upstream"         // 上游代理（不做修改）
	WebSearchModeBuiltinFree = "builtin_free"     // 内置免费搜索（强制 isFreeTierRequest=true）
	WebSearchModeLocalDDG    = "local_duckduckgo" // 本地 DuckDuckGo 搜索
)

type AmpSettings struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	UpstreamURL          string    `json:"upstream_url"`
	UpstreamAPIKey       string    `json:"-"`
	ModelMappingsJSON    string    `json:"-"`
	Enabled              bool      `json:"enabled"`
	WebSearchMode        string    `json:"web_search_mode"` // upstream | builtin_free | local_duckduckgo
	NativeMode           bool      `json:"native_mode"`
	RouteMappingsEnabled bool      `json:"route_mappings_enabled"`
	ShowBalanceInAd      bool      `json:"show_balance_in_ad"`
	Socks5Proxy          string    `json:"socks5_proxy"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type ModelMapping struct {
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

type UserAPIKey struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	KeyHash    string     `json:"-"`
	APIKey     string     `json:"-"`
	Prefix     string     `json:"prefix"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	LastUsed   *time.Time `json:"last_used,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Request/Response 结构体

type AmpSettingsRequest struct {
	UpstreamURL          *string        `json:"upstreamUrl,omitempty"`
	UpstreamAPIKey       *string        `json:"upstreamApiKey,omitempty"`
	ModelMappings        []ModelMapping `json:"modelMappings,omitempty"`
	Enabled              *bool          `json:"enabled,omitempty"`
	WebSearchMode        *string        `json:"webSearchMode,omitempty"` // upstream | builtin_free | local_duckduckgo
	NativeMode           *bool          `json:"nativeMode,omitempty"`
	RouteMappingsEnabled *bool          `json:"routeMappingsEnabled,omitempty"`
	ShowBalanceInAd      *bool          `json:"showBalanceInAd,omitempty"`
	Socks5Proxy          *string        `json:"socks5Proxy,omitempty"`
}

type AmpSettingsResponse struct {
	UpstreamURL          string         `json:"upstreamUrl"`
	ModelMappings        []ModelMapping `json:"modelMappings"`
	Enabled              bool           `json:"enabled"`
	HasAPIKey            bool           `json:"apiKeySet"`
	WebSearchMode        string         `json:"webSearchMode"` // upstream | builtin_free | local_duckduckgo
	NativeMode           bool           `json:"nativeMode"`
	RouteMappingsEnabled bool           `json:"routeMappingsEnabled"`
	ShowBalanceInAd      bool           `json:"showBalanceInAd"`
	HasSocks5Proxy       bool           `json:"socks5ProxySet"`
	CreatedAt            time.Time      `json:"createdAt,omitempty"`
	UpdatedAt            time.Time      `json:"updatedAt,omitempty"`
}

type TestConnectionResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
}

type CreateAPIKeyRequest struct {
	Name      string     `json:"name" binding:"required,min=1,max=64"`
	CustomKey string     `json:"customKey,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type UpdateAPIKeyRequest struct {
	Name        string     `json:"name" binding:"required,min=1,max=64"`
	APIKey      string     `json:"apiKey,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	ClearExpiry bool       `json:"clearExpiry,omitempty"`
}

type UpdateAPIKeyStatusRequest struct {
	Disabled bool `json:"disabled"`
}

type CreateAPIKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	APIKey    string     `json:"apiKey"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	Message   string     `json:"message"`
}

type APIKeyRevealResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	APIKey    string     `json:"apiKey"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type CCSwitchUsageItem struct {
	PlanName       *string  `json:"planName,omitempty"`
	Extra          *string  `json:"extra,omitempty"`
	IsValid        *bool    `json:"isValid,omitempty"`
	InvalidMessage *string  `json:"invalidMessage,omitempty"`
	Total          *float64 `json:"total,omitempty"`
	Used           *float64 `json:"used,omitempty"`
	Remaining      *float64 `json:"remaining,omitempty"`
	Unit           *string  `json:"unit,omitempty"`
}

type APIKeyListItem struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	APIKey    string     `json:"apiKey,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	LastUsed  *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Status    string     `json:"status"`
	IsActive  bool       `json:"isActive"`
}

type BootstrapResponse struct {
	HasSettings bool `json:"hasSettings"`
	HasAPIKey   bool `json:"hasApiKey"`
}

// RequestLogStatus 请求日志状态
type RequestLogStatus string

const (
	RequestLogStatusPending RequestLogStatus = "pending"
	RequestLogStatusSuccess RequestLogStatus = "success"
	RequestLogStatusError   RequestLogStatus = "error"
)

// RequestLog 请求日志记录
type RequestLog struct {
	ID                       string           `json:"id"`
	CreatedAt                string           `json:"createdAt"`
	UpdatedAt                *string          `json:"updatedAt,omitempty"`
	Status                   RequestLogStatus `json:"status"`
	UserID                   string           `json:"userId"`
	Username                 *string          `json:"username,omitempty"`
	APIKeyID                 string           `json:"apiKeyId"`
	APIKeyName               *string          `json:"apiKeyName,omitempty"`
	APIKeyPrefix             *string          `json:"apiKeyPrefix,omitempty"`
	OriginalModel            *string          `json:"originalModel,omitempty"`
	MappedModel              *string          `json:"mappedModel,omitempty"`
	SessionID                *string          `json:"sessionId,omitempty"`
	Provider                 *string          `json:"provider,omitempty"`
	ChannelID                *string          `json:"channelId,omitempty"`
	ChannelName              *string          `json:"channelName,omitempty"`
	Endpoint                 *string          `json:"endpoint,omitempty"`
	RequestFormat            *string          `json:"requestFormat,omitempty"`
	UpstreamFormat           *string          `json:"upstreamFormat,omitempty"`
	Method                   string           `json:"method"`
	Path                     string           `json:"path"`
	StatusCode               int              `json:"statusCode"`
	LatencyMs                int64            `json:"latencyMs"`
	TTFBMs                   *int64           `json:"ttfbMs,omitempty"`
	TPS                      *float64         `json:"tps,omitempty"`
	IsStreaming              bool             `json:"-"`
	InputTokens              *int             `json:"inputTokens,omitempty"`
	OutputTokens             *int             `json:"outputTokens,omitempty"`
	CacheReadInputTokens     *int             `json:"cacheReadInputTokens,omitempty"`
	CacheCreationInputTokens *int             `json:"cacheCreationInputTokens,omitempty"`
	ErrorType                *string          `json:"errorType,omitempty"`
	RequestID                *string          `json:"requestId,omitempty"`
	ThinkingLevel            *string          `json:"thinkingLevel,omitempty"` // 思维等级
	DownstreamTransport      *string          `json:"downstreamTransport,omitempty"`
	UpstreamTransport        *string          `json:"upstreamTransport,omitempty"`
	TransportFallbackReason  *string          `json:"transportFallbackReason,omitempty"`
	OutputPreview            *string          `json:"outputPreview,omitempty"` // 响应输出预览（前200字符）
	// 成本相关字段
	CostMicros                    *int64             `json:"costMicros,omitempty"`   // 成本（微美元，USD * 1e6）
	CostUsd                       *string            `json:"costUsd,omitempty"`      // 成本（USD，用于展示）
	PricingModel                  *string            `json:"pricingModel,omitempty"` // 计价模型名
	PricingRuleName               *string            `json:"pricingRuleName,omitempty"`
	InputMicrosPerMillion         *int64             `json:"inputMicrosPerMillion,omitempty"`
	OutputMicrosPerMillion        *int64             `json:"outputMicrosPerMillion,omitempty"`
	CacheReadMicrosPerMillion     *int64             `json:"cacheReadMicrosPerMillion,omitempty"`
	CacheCreationMicrosPerMillion *int64             `json:"cacheCreationMicrosPerMillion,omitempty"`
	InputCostPerToken             *float64           `json:"inputCostPerToken,omitempty"`
	OutputCostPerToken            *float64           `json:"outputCostPerToken,omitempty"`
	CacheReadInputPerToken        *float64           `json:"cacheReadInputPerToken,omitempty"`
	CacheCreationInputPerToken    *float64           `json:"cacheCreationInputPerToken,omitempty"`
	RateMultiplierPPM             *int64             `json:"rateMultiplierPpm,omitempty"`
	ChannelRateMultiplierPPM      *int64             `json:"channelRateMultiplierPpm,omitempty"`
	GroupRateMultiplierPPM        *int64             `json:"groupRateMultiplierPpm,omitempty"`
	SpecialRateMultiplierPPM      *int64             `json:"specialRateMultiplierPpm,omitempty"`
	RateMultiplier                *float64           `json:"rateMultiplier,omitempty"`
	ChannelRateMultiplier         *float64           `json:"channelRateMultiplier,omitempty"`
	GroupRateMultiplier           *float64           `json:"groupRateMultiplier,omitempty"`
	SpecialRateMultiplier         *float64           `json:"specialRateMultiplier,omitempty"`
	SpecialRateReason             *string            `json:"specialRateReason,omitempty"`
	ChannelTranslator             *ChannelTranslator `json:"channelTranslator,omitempty"`
	BillingStatus                 string             `json:"-"`
	ChargedSubscriptionMicros     int64              `json:"-"`
	ChargedBalanceMicros          int64              `json:"-"`
	BillingGapMicros              *int64             `json:"-"`
}

// RequestLogListResponse 请求日志列表响应
type RequestLogListResponse struct {
	Items    []RequestLog `json:"items"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
}

// UsageSummary 用量统计
type UsageSummary struct {
	GroupKey                    string `json:"groupKey"`
	InputTokensSum              int64  `json:"inputTokensSum"`
	OutputTokensSum             int64  `json:"outputTokensSum"`
	CacheReadInputTokensSum     int64  `json:"cacheReadInputTokensSum"`
	CacheCreationInputTokensSum int64  `json:"cacheCreationInputTokensSum"`
	RequestCount                int64  `json:"requestCount"`
	ErrorCount                  int64  `json:"errorCount"`
	CostMicrosSum               int64  `json:"costMicrosSum"` // 总成本（微美元）
	CostUsdSum                  string `json:"costUsdSum"`    // 总成本（USD 展示）
}

// UsageSummaryResponse 用量统计响应
type UsageSummaryResponse struct {
	Items []UsageSummary `json:"items"`
}

// RequestLogDetail 请求日志详情（包含请求/响应头和体）
type RequestLogDetail struct {
	RequestID                 string            `json:"requestId"`
	RequestHeaders            map[string]string `json:"requestHeaders"`
	RequestBody               string            `json:"requestBody"`
	TranslatedRequestBody     string            `json:"translatedRequestBody,omitempty"`    // 翻译后发送给上游的请求体
	TranslatedRequestHeaders  map[string]string `json:"translatedRequestHeaders,omitempty"` // 翻译后发送给上游的请求头
	ResponseHeaders           map[string]string `json:"responseHeaders"`
	ResponseBody              string            `json:"responseBody"`
	TranslatedResponseBody    string            `json:"translatedResponseBody,omitempty"` // 翻译后发送给客户端的响应
	CreatedAt                 time.Time         `json:"createdAt"`
	BillingStatus             string            `json:"billingStatus"`
	ChargedSubscriptionMicros int64             `json:"chargedSubscriptionMicros"`
	ChargedBalanceMicros      int64             `json:"chargedBalanceMicros"`
	BillingGapMicros          *int64            `json:"billingGapMicros,omitempty"`
	CostMicros                *int64            `json:"costMicros,omitempty"`
	CostUsd                   *string           `json:"costUsd,omitempty"`
}
