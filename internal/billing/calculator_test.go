package billing

import "testing"

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
