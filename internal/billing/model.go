package billing

import "time"

// ModelPrice 模型价格记录
type ModelPrice struct {
	ID        string    `json:"id"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider,omitempty"`
	PriceData PriceData `json:"priceData"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PriceData 价格数据（遵循 LiteLLM 格式）
// 整数列单位:
// - *MicrosPerMillion: USD micros per 1M tokens
// 兼容字段单位:
// - *CostPerToken: USD per token
type PriceData struct {
	InputMicrosPerMillion         int64   `json:"input_micros_per_million"`
	OutputMicrosPerMillion        int64   `json:"output_micros_per_million"`
	CacheReadMicrosPerMillion     int64   `json:"cache_read_micros_per_million,omitempty"`
	CacheCreationMicrosPerMillion int64   `json:"cache_creation_micros_per_million,omitempty"`
	InputCostPerToken             float64 `json:"input_cost_per_token,omitempty"`
	OutputCostPerToken            float64 `json:"output_cost_per_token,omitempty"`
	CacheReadInputPerToken        float64 `json:"cache_read_input_token_cost,omitempty"`
	CacheCreationPerToken         float64 `json:"cache_creation_input_token_cost,omitempty"`
}

// TokenUsage 统一的 token 使用量结构
type TokenUsage struct {
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int
}

// CostResult 成本计算结果
type CostResult struct {
	CostMicros      int64  // 微美元 (USD * 1e6)
	CostUsd         string // USD 字符串（用于展示）
	PricingModel    string // 使用的计价模型名
	PricingRuleName string
	PriceFound      bool // 是否找到价格
}
