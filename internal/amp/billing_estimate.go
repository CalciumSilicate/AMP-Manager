package amp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"ampmanager/internal/billing"
	"ampmanager/internal/repository"

	"github.com/gin-gonic/gin"
)

const defaultReservationMaxOutputTokens = 32768

type billingEstimateKey struct{}

type BillingEstimate struct {
	PricingModel          string
	EstimatedInputTokens  int
	EstimatedOutputTokens int
	EstimatedCostMicros   int64
}

func WithBillingEstimate(ctx context.Context, estimate *BillingEstimate) context.Context {
	return context.WithValue(ctx, billingEstimateKey{}, estimate)
}

func GetBillingEstimate(ctx context.Context) *BillingEstimate {
	if val := ctx.Value(billingEstimateKey{}); val != nil {
		if estimate, ok := val.(*BillingEstimate); ok {
			return estimate
		}
	}
	return nil
}

func BillingEstimateMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsModelInvocation(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}

		payload, err := EnsureRequestPayload(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, NewStandardError(http.StatusInternalServerError, "failed to read request body"))
			return
		}

		modelName := ""
		if mapped := GetMappedModel(c); mapped != "" {
			modelName = mapped
		} else if original := GetOriginalModel(c); original != "" {
			modelName = original
		} else if payload.JSON != nil {
			if model, ok := payload.JSON["model"].(string); ok {
				modelName = model
			}
		}
		if modelName == "" {
			modelName = extractModelName(c)
		}

		maxOutputTokens := extractReservationMaxOutputTokens(payload.JSON, modelName)
		inputTokens := len(payload.Body)
		if inputTokens < 0 {
			inputTokens = 0
		}

		estimate := &BillingEstimate{
			PricingModel:          modelName,
			EstimatedInputTokens:  inputTokens,
			EstimatedOutputTokens: maxOutputTokens,
		}

		if calc := billing.GetCostCalculator(); calc != nil && modelName != "" {
			cost := calc.CalculateFromPointers(
				modelName,
				intPtr(inputTokens),
				intPtr(maxOutputTokens),
				nil,
				nil,
			)
			if cost.PriceFound {
				estimate.EstimatedCostMicros = cost.CostMicros
				if cfg := GetProxyConfig(c.Request.Context()); cfg != nil {
					if cfg.RateMultiplier == 0 {
						estimate.EstimatedCostMicros = 0
					} else if cfg.RateMultiplier > 0 {
						estimate.EstimatedCostMicros = int64(float64(cost.CostMicros) * cfg.RateMultiplier)
					}
				}
			}
		}

		ctx := WithBillingEstimate(c.Request.Context(), estimate)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func extractReservationMaxOutputTokens(payload map[string]interface{}, modelName string) int {
	if payload != nil {
		for _, key := range []string{"max_output_tokens", "max_completion_tokens", "max_tokens"} {
			if value, ok := payload[key]; ok {
				switch typed := value.(type) {
				case float64:
					if typed > 0 {
						return int(typed)
					}
				case int:
					if typed > 0 {
						return typed
					}
				case json.Number:
					if parsed, err := typed.Int64(); err == nil && parsed > 0 {
						return int(parsed)
					}
				}
			}
		}
	}

	if modelName != "" {
		if meta, err := repository.NewModelMetadataRepository().FindMatchingModel(modelName); err == nil && meta != nil && meta.MaxCompletionTokens > 0 {
			return meta.MaxCompletionTokens
		}
	}

	if strings.HasPrefix(strings.ToLower(modelName), "o") {
		return 65536
	}
	return defaultReservationMaxOutputTokens
}
