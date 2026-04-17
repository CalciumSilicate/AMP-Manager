package errorruleutil

import (
	"encoding/json"
	"fmt"
)

const MaxOverrideResponseBytes = 10 * 1024

func ValidateOverrideResponse(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if len(raw) > MaxOverrideResponseBytes {
		return fmt.Errorf("覆写响应不能超过 10KB")
	}
	if !json.Valid(raw) {
		return fmt.Errorf("覆写响应必须是合法 JSON")
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("覆写响应解析失败")
	}
	if len(payload) == 0 {
		return fmt.Errorf("覆写响应必须是对象")
	}

	if payload["type"] == "error" {
		return validateClaudeOverride(payload)
	}

	errorValue, ok := payload["error"].(map[string]any)
	if !ok {
		return fmt.Errorf("覆写响应缺少 error 对象")
	}

	if _, ok := errorValue["code"].(float64); ok {
		return validateGeminiOverride(payload)
	}
	if _, ok := errorValue["type"].(string); ok {
		return validateOpenAIOverride(payload)
	}
	return fmt.Errorf("覆写响应格式无法识别")
}

func SanitizedOverrideResponse(raw json.RawMessage) json.RawMessage {
	if err := ValidateOverrideResponse(raw); err != nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func validateClaudeOverride(payload map[string]any) error {
	errorValue, ok := payload["error"].(map[string]any)
	if !ok {
		return fmt.Errorf("Claude 覆写响应缺少 error 对象")
	}
	if value, ok := errorValue["type"].(string); !ok || value == "" {
		return fmt.Errorf("Claude 覆写响应 error.type 缺失")
	}
	if _, ok := errorValue["message"].(string); !ok {
		return fmt.Errorf("Claude 覆写响应 error.message 必须是字符串")
	}
	if requestID, exists := payload["request_id"]; exists {
		if _, ok := requestID.(string); !ok {
			return fmt.Errorf("Claude 覆写响应 request_id 必须是字符串")
		}
	}
	return nil
}

func validateGeminiOverride(payload map[string]any) error {
	errorValue, ok := payload["error"].(map[string]any)
	if !ok {
		return fmt.Errorf("Gemini 覆写响应缺少 error 对象")
	}
	if _, ok := errorValue["code"].(float64); !ok {
		return fmt.Errorf("Gemini 覆写响应 error.code 必须是数字")
	}
	if _, ok := errorValue["message"].(string); !ok {
		return fmt.Errorf("Gemini 覆写响应 error.message 必须是字符串")
	}
	if value, ok := errorValue["status"].(string); !ok || value == "" {
		return fmt.Errorf("Gemini 覆写响应 error.status 缺失")
	}
	if details, exists := errorValue["details"]; exists {
		if _, ok := details.([]any); !ok {
			return fmt.Errorf("Gemini 覆写响应 error.details 必须是数组")
		}
	}
	return nil
}

func validateOpenAIOverride(payload map[string]any) error {
	errorValue, ok := payload["error"].(map[string]any)
	if !ok {
		return fmt.Errorf("OpenAI 覆写响应缺少 error 对象")
	}
	if value, ok := errorValue["type"].(string); !ok || value == "" {
		return fmt.Errorf("OpenAI 覆写响应 error.type 缺失")
	}
	if _, ok := errorValue["message"].(string); !ok {
		return fmt.Errorf("OpenAI 覆写响应 error.message 必须是字符串")
	}
	if param, exists := errorValue["param"]; exists && param != nil {
		if _, ok := param.(string); !ok {
			return fmt.Errorf("OpenAI 覆写响应 error.param 必须是字符串或 null")
		}
	}
	if code, exists := errorValue["code"]; exists && code != nil {
		if _, ok := code.(string); !ok {
			return fmt.Errorf("OpenAI 覆写响应 error.code 必须是字符串或 null")
		}
	}
	return nil
}
