package amp

import (
	"context"
	"strings"

	"github.com/tidwall/gjson"
)

func GetThinkingLevelFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if val := ctx.Value(thinkingLevelKey{}); val != nil {
		if level, ok := val.(string); ok {
			return normalizeThinkingLevel(level)
		}
	}
	if payload := GetRequestPayload(ctx); payload != nil {
		if level := extractThinkingLevelFromBody(payload.Body); level != "" {
			return level
		}
	}
	if captureData := GetCaptureData(ctx); captureData != nil {
		if level := extractThinkingLevelFromBody(captureData.RequestBody); level != "" {
			return level
		}
	}
	return ""
}

func extractThinkingLevelFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if level := normalizeThinkingLevel(gjson.GetBytes(body, "reasoning.effort").String()); level != "" {
		return level
	}
	if level := normalizeThinkingLevel(gjson.GetBytes(body, "reasoning_effort").String()); level != "" {
		return level
	}
	return ""
}

func normalizeThinkingLevel(level string) string {
	return strings.ToLower(strings.TrimSpace(level))
}
