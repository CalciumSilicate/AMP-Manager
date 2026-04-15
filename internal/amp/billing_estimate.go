package amp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"ampmanager/internal/billing"
	"ampmanager/internal/billingstate"
	"ampmanager/internal/database"

	"github.com/gin-gonic/gin"
)

const defaultReservationMaxOutputTokens = 32768
const largeEstimateBodyThresholdBytes = 128 * 1024

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

type billingRequestKind string

const (
	billingRequestUnknown         billingRequestKind = ""
	billingRequestOpenAIChat      billingRequestKind = "openai_chat"
	billingRequestOpenAIResponses billingRequestKind = "openai_responses"
	billingRequestAnthropic       billingRequestKind = "anthropic"
	billingRequestGemini          billingRequestKind = "gemini"
)

func BillingEstimateMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsModelInvocation(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}
		if !shouldEstimateBilling(c.Request.Context()) {
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

		requestKind := detectBillingRequestKind(c.Request.URL.Path)
		maxOutputTokens := extractReservationMaxOutputTokens(payload.JSON, modelName)
		inputTokens, _ := estimateReservationInputTokensForPayload(payload, requestKind)

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

func shouldEstimateBilling(ctx context.Context) bool {
	if billingstate.Get() == nil {
		return false
	}

	cfg := GetProxyConfig(ctx)
	if cfg == nil {
		return false
	}

	return cfg.RateMultiplier != 0
}

func extractReservationMaxOutputTokens(payload map[string]interface{}, modelName string) int {
	explicit := extractPositiveInt(payload, "max_output_tokens", "max_completion_tokens", "max_tokens")
	metaLimit := lookupReservationMaxCompletionTokens(modelName)
	if explicit > 0 {
		if metaLimit > 0 && explicit > metaLimit {
			return metaLimit
		}
		return explicit
	}

	if metaLimit > 0 {
		return metaLimit
	}

	if strings.HasPrefix(strings.ToLower(modelName), "o") {
		return 65536
	}
	return defaultReservationMaxOutputTokens
}

func estimateReservationInputTokensForPayload(payload *RequestPayload, kind billingRequestKind) (int, bool) {
	if payload == nil {
		return 0, false
	}
	if len(payload.Body) >= largeEstimateBodyThresholdBytes {
		return approximateBytesTokenCount(payload.Body), true
	}
	return estimateReservationInputTokens(payload.JSON, kind), false
}

func estimateReservationInputTokens(payload map[string]interface{}, kind billingRequestKind) int {
	if payload != nil {
		var units estimateUnits
		switch kind {
		case billingRequestOpenAIResponses:
			estimateOpenAIResponsesPayload(&units, payload)
		case billingRequestAnthropic:
			estimateAnthropicPayload(&units, payload)
		case billingRequestGemini:
			estimateGeminiPayload(&units, payload)
		case billingRequestOpenAIChat:
			estimateOpenAIChatPayload(&units, payload)
		default:
			estimateGenericPayload(&units, payload)
		}
		if tokens := units.tokens(); tokens > 0 {
			return tokens
		}
	}

	return 0
}

func approximateBytesTokenCount(body []byte) int {
	return approximateTextTokenCount(string(body))
}

func detectBillingRequestKind(path string) billingRequestKind {
	normalizedPath := normalizeProviderPath(path)
	switch {
	case strings.Contains(normalizedPath, "/v1/responses"):
		return billingRequestOpenAIResponses
	case strings.Contains(normalizedPath, "/v1/messages"):
		return billingRequestAnthropic
	case strings.Contains(normalizedPath, ":generateContent"), strings.Contains(normalizedPath, ":streamGenerateContent"), strings.HasPrefix(normalizedPath, "/v1beta/models/"), strings.HasPrefix(normalizedPath, "/v1beta1/models/"), strings.HasPrefix(normalizedPath, "/v1beta1/publishers/google/models/"):
		return billingRequestGemini
	case strings.Contains(normalizedPath, "/v1/chat/completions"), strings.Contains(normalizedPath, "/v1/completions"):
		return billingRequestOpenAIChat
	default:
		return billingRequestUnknown
	}
}

func lookupReservationMaxCompletionTokens(modelName string) int {
	if modelName == "" {
		return 0
	}
	if database.GetDB() != nil {
		if meta := GetModelMetadata(modelName); meta != nil && meta.MaxCompletionTokens > 0 {
			return meta.MaxCompletionTokens
		}
	}
	if meta, ok := knownModelMetadata[modelName]; ok && meta.MaxCompletionTokens > 0 {
		return meta.MaxCompletionTokens
	}
	for pattern, meta := range knownModelMetadata {
		if matchPattern(pattern, modelName) && meta.MaxCompletionTokens > 0 {
			return meta.MaxCompletionTokens
		}
	}
	for knownModel, meta := range knownModelMetadata {
		if strings.Contains(knownModel, modelName) && meta.MaxCompletionTokens > 0 {
			return meta.MaxCompletionTokens
		}
	}
	return 0
}

func extractPositiveInt(payload map[string]interface{}, keys ...string) int {
	if payload == nil {
		return 0
	}
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			if typed > 0 {
				return int(typed)
			}
		case int:
			if typed > 0 {
				return typed
			}
		case int64:
			if typed > 0 {
				return int(typed)
			}
		case json.Number:
			if parsed, err := typed.Int64(); err == nil && parsed > 0 {
				return int(parsed)
			}
		}
	}
	return 0
}

type estimateUnits struct {
	ascii     int
	unicode   int
	cjk       int
	structure int
}

func (u *estimateUnits) addText(value string) {
	for _, r := range value {
		if unicode.IsSpace(r) {
			continue
		}
		switch {
		case r <= unicode.MaxASCII:
			u.ascii++
		case isCJKEstimateRune(r):
			u.cjk++
		default:
			u.unicode++
		}
	}
}

func (u *estimateUnits) addStructure(v any) {
	if v == nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	u.structure += approximateTextTokenCount(string(data))
}

func (u *estimateUnits) tokens() int {
	total := ceilDiv(u.ascii, 4) + ceilDiv(u.unicode, 2) + u.cjk + ceilDiv(u.structure, 2)
	if total < 0 {
		return 0
	}
	return total
}

func approximateTextTokenCount(value string) int {
	var units estimateUnits
	units.addText(value)
	return units.tokens()
}

func ceilDiv(value, divisor int) int {
	if value <= 0 || divisor <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}

func isCJKEstimateRune(r rune) bool {
	return unicode.In(r,
		unicode.Han,
		unicode.Hangul,
		unicode.Hiragana,
		unicode.Katakana,
		unicode.Bopomofo,
	)
}

func estimateOpenAIChatPayload(units *estimateUnits, payload map[string]interface{}) {
	addValueText(units, payload["prompt"])
	addMessageLikeCollection(units, payload["messages"])
	addTextBlocks(units, payload["tools"])
	addResponseFormatEstimate(units, payload["response_format"])
	if text, ok := payload["text"].(map[string]interface{}); ok {
		addResponseFormatEstimate(units, text["format"])
	}
}

func estimateOpenAIResponsesPayload(units *estimateUnits, payload map[string]interface{}) {
	addValueText(units, payload["instructions"])
	addValueText(units, payload["input"])
	addTextBlocks(units, payload["tools"])
	addResponseFormatEstimate(units, payload["response_format"])
	if text, ok := payload["text"].(map[string]interface{}); ok {
		addResponseFormatEstimate(units, text["format"])
	}
}

func estimateAnthropicPayload(units *estimateUnits, payload map[string]interface{}) {
	addValueText(units, payload["system"])
	addMessageLikeCollection(units, payload["messages"])
	addAnthropicToolsEstimate(units, payload["tools"])
}

func estimateGeminiPayload(units *estimateUnits, payload map[string]interface{}) {
	addValueText(units, payload["systemInstruction"])
	addValueText(units, payload["contents"])
	addGeminiToolsEstimate(units, payload["tools"])
	if cfg, ok := payload["generationConfig"].(map[string]interface{}); ok {
		addTextBlocks(units, cfg["responseSchema"])
		addValueText(units, cfg["systemInstruction"])
	}
}

func estimateGenericPayload(units *estimateUnits, payload map[string]interface{}) {
	addValueText(units, payload["system"])
	addValueText(units, payload["instructions"])
	addValueText(units, payload["input"])
	addValueText(units, payload["messages"])
	addTextBlocks(units, payload["tools"])
	addResponseFormatEstimate(units, payload["response_format"])
}

func addMessageLikeCollection(units *estimateUnits, value any) {
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			addValueText(units, item)
		}
	default:
		addValueText(units, value)
	}
}

func addResponseFormatEstimate(units *estimateUnits, value any) {
	obj, ok := value.(map[string]interface{})
	if !ok {
		addTextBlocks(units, value)
		return
	}
	addValueText(units, obj["type"])
	addValueText(units, obj["description"])
	addValueText(units, obj["name"])
	addTextBlocks(units, obj["schema"])
	addTextBlocks(units, obj["json_schema"])
	if jsonSchema, ok := obj["json_schema"].(map[string]interface{}); ok {
		addValueText(units, jsonSchema["name"])
		addValueText(units, jsonSchema["description"])
		addTextBlocks(units, jsonSchema["schema"])
		addValueText(units, jsonSchema["strict"])
	}
}

func addAnthropicToolsEstimate(units *estimateUnits, value any) {
	list, ok := value.([]interface{})
	if !ok {
		addTextBlocks(units, value)
		return
	}
	for _, item := range list {
		obj, ok := item.(map[string]interface{})
		if !ok {
			addTextBlocks(units, item)
			continue
		}
		addValueText(units, obj["name"])
		addValueText(units, obj["description"])
		addTextBlocks(units, obj["input_schema"])
	}
}

func addGeminiToolsEstimate(units *estimateUnits, value any) {
	list, ok := value.([]interface{})
	if !ok {
		addTextBlocks(units, value)
		return
	}
	for _, item := range list {
		obj, ok := item.(map[string]interface{})
		if !ok {
			addTextBlocks(units, item)
			continue
		}
		addTextBlocks(units, obj["functionDeclarations"])
		addTextBlocks(units, obj["googleSearch"])
		addTextBlocks(units, obj["retrieval"])
	}
}

func addTextBlocks(units *estimateUnits, value any) {
	if value == nil {
		return
	}
	units.addStructure(value)
}

func addValueText(units *estimateUnits, value any) {
	switch typed := value.(type) {
	case nil:
		return
	case string:
		units.addText(typed)
	case json.Number:
		units.addText(typed.String())
	case bool:
		if typed {
			units.addText("true")
		} else {
			units.addText("false")
		}
	case float64:
		units.addText(strconvFormatFloat(typed))
	case []interface{}:
		for _, item := range typed {
			addValueText(units, item)
		}
	case map[string]interface{}:
		for key, nested := range typed {
			switch key {
			case "text", "input_text", "output_text", "content", "instructions", "system", "name", "description", "arguments", "input", "query", "title", "subtitle", "prompt", "role", "type", "mimeType", "responseMimeType":
				addValueText(units, nested)
			case "schema", "parameters", "input_schema", "response_format", "responseSchema", "json_schema", "function", "tool_calls", "toolCall", "toolResult", "functionCall", "functionResponse":
				addTextBlocks(units, nested)
			case "parts", "messages", "contents", "input_texts", "input_image", "output":
				addValueText(units, nested)
			default:
				switch nestedTyped := nested.(type) {
				case string:
					if shouldIncludeLooseTextField(key) {
						units.addText(nestedTyped)
					}
				case []interface{}:
					if shouldDescendLooseField(key) {
						addValueText(units, nested)
					}
				case map[string]interface{}:
					if shouldDescendLooseField(key) {
						addValueText(units, nestedTyped)
					}
				}
			}
		}
	default:
		units.addStructure(typed)
	}
}

func shouldIncludeLooseTextField(key string) bool {
	switch strings.ToLower(key) {
	case "id", "model", "stream", "temperature", "top_p", "presence_penalty", "frequency_penalty", "seed", "user":
		return false
	default:
		return strings.Contains(strings.ToLower(key), "text") ||
			strings.Contains(strings.ToLower(key), "content") ||
			strings.Contains(strings.ToLower(key), "instruction") ||
			strings.Contains(strings.ToLower(key), "schema") ||
			strings.Contains(strings.ToLower(key), "prompt")
	}
}

func shouldDescendLooseField(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "message") ||
		strings.Contains(lower, "content") ||
		strings.Contains(lower, "input") ||
		strings.Contains(lower, "tool") ||
		strings.Contains(lower, "schema") ||
		strings.Contains(lower, "part")
}

func strconvFormatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
