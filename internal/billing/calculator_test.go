package billing

import (
	"encoding/json"
	"testing"

	"ampmanager/internal/model"
)

func TestCalculateSubtractsCacheReadFromInputCost(t *testing.T) {
	store := &PriceStore{
		prices: map[string]ModelPrice{
			"test-model": {
				Model: "test-model",
				PriceData: PriceData{
					InputCostPerToken:      2.0 / 1_000_000,
					OutputCostPerToken:     5.0 / 1_000_000,
					CacheReadInputPerToken: 0.2 / 1_000_000,
					CacheCreationPerToken:  3.0 / 1_000_000,
				},
			},
		},
	}

	calculator := NewCostCalculator(store)
	result := calculator.Calculate("test-model", TokenUsage{
		InputTokens:              100,
		OutputTokens:             20,
		CacheReadInputTokens:     80,
		CacheCreationInputTokens: 10,
	})

	if !result.PriceFound {
		t.Fatalf("expected price to be found")
	}

	const wantMicros int64 = 186
	if result.CostMicros != wantMicros {
		t.Fatalf("unexpected cost micros: got %d want %d", result.CostMicros, wantMicros)
	}
	if result.CostUsd != "0.000186" {
		t.Fatalf("unexpected cost usd: got %s want %s", result.CostUsd, "0.000186")
	}
}

func TestCalculateClampsNegativeUncachedInputToZero(t *testing.T) {
	store := &PriceStore{
		prices: map[string]ModelPrice{
			"test-model": {
				Model: "test-model",
				PriceData: PriceData{
					InputCostPerToken:      2.0 / 1_000_000,
					OutputCostPerToken:     5.0 / 1_000_000,
					CacheReadInputPerToken: 0.2 / 1_000_000,
				},
			},
		},
	}

	calculator := NewCostCalculator(store)
	result := calculator.Calculate("test-model", TokenUsage{
		InputTokens:          10,
		OutputTokens:         1,
		CacheReadInputTokens: 20,
	})

	const wantMicros int64 = 9
	if result.CostMicros != wantMicros {
		t.Fatalf("unexpected cost micros: got %d want %d", result.CostMicros, wantMicros)
	}
}

func TestCalculateUsesAbove272kPricingWhenContextExceedsThreshold(t *testing.T) {
	store := &PriceStore{
		prices: map[string]ModelPrice{
			"gpt-5.4": {
				Model: "gpt-5.4",
				PriceData: PriceData{
					InputCostPerToken:               2.5 / 1_000_000,
					OutputCostPerToken:              15.0 / 1_000_000,
					CacheReadInputPerToken:          0.25 / 1_000_000,
					InputCostPerTokenAbove272k:      5.0 / 1_000_000,
					OutputCostPerTokenAbove272k:     22.5 / 1_000_000,
					CacheReadInputPerTokenAbove272k: 0.5 / 1_000_000,
				},
			},
		},
	}

	calculator := NewCostCalculator(store)
	result := calculator.Calculate("gpt-5.4", TokenUsage{
		InputTokens:          300000,
		OutputTokens:         1000,
		CacheReadInputTokens: 100000,
	})

	const wantMicros int64 = 1_072_500
	if result.CostMicros != wantMicros {
		t.Fatalf("unexpected long-context cost micros: got %d want %d", result.CostMicros, wantMicros)
	}
	if result.PricingRuleName != "272K 以上上下文" {
		t.Fatalf("unexpected long-context pricing rule name: %q", result.PricingRuleName)
	}
}

func TestCalculateKeepsContextRulePriorityOverBuiltInLongContextTier(t *testing.T) {
	store := &PriceStore{
		prices: map[string]ModelPrice{
			"gpt-5.4": {
				Model: "gpt-5.4",
				PriceData: PriceData{
					InputCostPerToken:               2.5 / 1_000_000,
					OutputCostPerToken:              15.0 / 1_000_000,
					CacheReadInputPerToken:          0.25 / 1_000_000,
					InputCostPerTokenAbove272k:      5.0 / 1_000_000,
					OutputCostPerTokenAbove272k:     22.5 / 1_000_000,
					CacheReadInputPerTokenAbove272k: 0.5 / 1_000_000,
				},
			},
		},
		contextRules: map[string][]model.ModelPriceContextRule{
			"gpt-5.4": {
				{
					Model:                         "gpt-5.4",
					RuleName:                      "自定义超长区间",
					MinTokens:                     272001,
					InputMicrosPerMillion:         6_000_000,
					OutputMicrosPerMillion:        30_000_000,
					CacheReadMicrosPerMillion:     600_000,
					CacheCreationMicrosPerMillion: 0,
				},
			},
		},
	}

	calculator := NewCostCalculator(store)
	result := calculator.Calculate("gpt-5.4", TokenUsage{
		InputTokens:          300000,
		OutputTokens:         1000,
		CacheReadInputTokens: 100000,
	})

	if result.PricingRuleName != "自定义超长区间" {
		t.Fatalf("expected custom context rule to win, got %q", result.PricingRuleName)
	}
	if result.CostMicros == 1_072_500 {
		t.Fatalf("expected built-in 272K tier not to override custom context rule")
	}
}

func TestLiteLLMPricingParsesAbove272kFields(t *testing.T) {
	raw := []byte(`{
		"litellm_provider": "openai",
		"mode": "chat",
		"input_cost_per_token": 0.0000025,
		"output_cost_per_token": 0.000015,
		"cache_read_input_token_cost": 0.00000025,
		"input_cost_per_token_above_272k_tokens": 0.000005,
		"output_cost_per_token_above_272k_tokens": 0.0000225,
		"cache_read_input_token_cost_above_272k_tokens": 0.0000005,
		"max_input_tokens": 1050000
	}`)

	var pricing LiteLLMPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if pricing.InputCostPerTokenAbove272k == nil || *pricing.InputCostPerTokenAbove272k != 0.000005 {
		t.Fatalf("unexpected input above272k: %#v", pricing.InputCostPerTokenAbove272k)
	}
	if pricing.OutputCostPerTokenAbove272k == nil || *pricing.OutputCostPerTokenAbove272k != 0.0000225 {
		t.Fatalf("unexpected output above272k: %#v", pricing.OutputCostPerTokenAbove272k)
	}
	if pricing.CacheReadInputTokenCostAbove272k == nil || *pricing.CacheReadInputTokenCostAbove272k != 0.0000005 {
		t.Fatalf("unexpected cache read above272k: %#v", pricing.CacheReadInputTokenCostAbove272k)
	}
}
