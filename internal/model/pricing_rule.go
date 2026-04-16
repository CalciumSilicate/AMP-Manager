package model

import "time"

type ModelPriceContextRule struct {
	ID                       string     `json:"id"`
	Model                    string     `json:"model"`
	RuleName                 string     `json:"ruleName"`
	MinTokens                int64      `json:"minTokens"`
	MaxTokens                *int64     `json:"maxTokens,omitempty"`
	InputCostPerToken        float64    `json:"inputCostPerToken"`
	OutputCostPerToken       float64    `json:"outputCostPerToken"`
	CacheReadInputPerToken   float64    `json:"cacheReadInputPerToken"`
	CacheCreationPerToken    float64    `json:"cacheCreationPerToken"`
	SortOrder                int        `json:"sortOrder"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

type ModelPriceContextRuleRequest struct {
	RuleName               string `json:"ruleName" binding:"required,min=1,max=64"`
	MinTokens              int64  `json:"minTokens" binding:"min=0"`
	MaxTokens              *int64 `json:"maxTokens,omitempty"`
	InputCostPerToken      float64 `json:"inputCostPerToken"`
	OutputCostPerToken     float64 `json:"outputCostPerToken"`
	CacheReadInputPerToken float64 `json:"cacheReadInputPerToken"`
	CacheCreationPerToken  float64 `json:"cacheCreationPerToken"`
	SortOrder              int    `json:"sortOrder"`
}

type UpdateModelPriceContextRulesRequest struct {
	Rules []ModelPriceContextRuleRequest `json:"rules"`
}
