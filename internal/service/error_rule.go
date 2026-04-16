package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"ampmanager/internal/model"
)

const errorRulesConfigKey = "error_rules"

func defaultErrorRules() []model.ErrorRule {
	return []model.ErrorRule{
		{
			ID:              "builtin-openai-an-error-occurred-responses",
			Name:            "OpenAI 风格 An Error Occurred（Responses）",
			BuiltIn:         true,
			Enabled:         true,
			RequestType:     model.ErrorRuleRequestTypeResponses,
			UpstreamStatus:  "200",
			Pattern:         "An Error Occurred",
			MatchMode:       model.ErrorRuleMatchModeSubstring,
			OverrideStatus:  http.StatusBadGateway,
			OverrideMessage: "上游返回了错误内容",
		},
		{
			ID:              "builtin-openai-an-error-occurred-chat",
			Name:            "OpenAI 风格 An Error Occurred（Chat Completions）",
			BuiltIn:         true,
			Enabled:         true,
			RequestType:     model.ErrorRuleRequestTypeChatCompletions,
			UpstreamStatus:  "200",
			Pattern:         "An Error Occurred",
			MatchMode:       model.ErrorRuleMatchModeSubstring,
			OverrideStatus:  http.StatusBadGateway,
			OverrideMessage: "上游返回了错误内容",
		},
		{
			ID:              "builtin-fake-200-error-object-responses",
			Name:            "200 但 body 为 error 对象（Responses）",
			BuiltIn:         true,
			Enabled:         true,
			RequestType:     model.ErrorRuleRequestTypeResponses,
			UpstreamStatus:  "200",
			Pattern:         `(?is)^\s*\{.*"error"\s*:\s*(\{|\").*`,
			MatchMode:       model.ErrorRuleMatchModeRegex,
			OverrideStatus:  http.StatusBadGateway,
			OverrideMessage: "上游返回了错误对象",
		},
		{
			ID:              "builtin-fake-200-error-object-chat",
			Name:            "200 但 body 为 error 对象（Chat Completions）",
			BuiltIn:         true,
			Enabled:         true,
			RequestType:     model.ErrorRuleRequestTypeChatCompletions,
			UpstreamStatus:  "200",
			Pattern:         `(?is)^\s*\{.*"error"\s*:\s*(\{|\").*`,
			MatchMode:       model.ErrorRuleMatchModeRegex,
			OverrideStatus:  http.StatusBadGateway,
			OverrideMessage: "上游返回了错误对象",
		},
		{
			ID:              "builtin-fake-200-error-object-gemini",
			Name:            "200 但 body 为 error 对象（Gemini）",
			BuiltIn:         true,
			Enabled:         true,
			RequestType:     model.ErrorRuleRequestTypeGemini,
			UpstreamStatus:  "200",
			Pattern:         `(?is)^\s*\{.*"error"\s*:\s*(\{|\").*`,
			MatchMode:       model.ErrorRuleMatchModeRegex,
			OverrideStatus:  http.StatusBadGateway,
			OverrideMessage: "上游返回了错误对象",
		},
		{
			ID:              "builtin-fake-200-error-object-anthropic",
			Name:            "200 但 body 为 error 对象（Anthropic Messages）",
			BuiltIn:         true,
			Enabled:         true,
			RequestType:     model.ErrorRuleRequestTypeAnthropic,
			UpstreamStatus:  "200",
			Pattern:         `(?is)^\s*\{.*"error"\s*:\s*(\{|\").*`,
			MatchMode:       model.ErrorRuleMatchModeRegex,
			OverrideStatus:  http.StatusBadGateway,
			OverrideMessage: "上游返回了错误对象",
		},
	}
}

func normalizeErrorRule(rule model.ErrorRule) (model.ErrorRule, error) {
	rule.ID = strings.TrimSpace(rule.ID)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Pattern = strings.TrimSpace(rule.Pattern)
	rule.UpstreamStatus = normalizeErrorRuleStatusPattern(rule.UpstreamStatus)
	rule.OverrideMessage = strings.TrimSpace(rule.OverrideMessage)

	switch rule.RequestType {
	case model.ErrorRuleRequestTypeResponses,
		model.ErrorRuleRequestTypeChatCompletions,
		model.ErrorRuleRequestTypeGemini,
		model.ErrorRuleRequestTypeAnthropic:
	default:
		return model.ErrorRule{}, fmt.Errorf("请求类型无效")
	}

	switch rule.MatchMode {
	case model.ErrorRuleMatchModeSubstring, model.ErrorRuleMatchModeRegex:
	default:
		return model.ErrorRule{}, fmt.Errorf("匹配模式无效")
	}

	if rule.ID == "" {
		return model.ErrorRule{}, fmt.Errorf("规则 ID 不能为空")
	}
	if rule.Name == "" {
		return model.ErrorRule{}, fmt.Errorf("规则名称不能为空")
	}
	if rule.Pattern == "" {
		return model.ErrorRule{}, fmt.Errorf("匹配内容不能为空")
	}
	if rule.OverrideStatus < 100 || rule.OverrideStatus > 599 {
		return model.ErrorRule{}, fmt.Errorf("覆盖状态码无效")
	}
	if rule.UpstreamStatus == "" {
		rule.UpstreamStatus = "*"
	}
	if rule.OverrideMessage == "" {
		rule.OverrideMessage = http.StatusText(rule.OverrideStatus)
	}
	if rule.MatchMode == model.ErrorRuleMatchModeRegex {
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return model.ErrorRule{}, fmt.Errorf("正则表达式无效")
		}
	}

	return rule, nil
}

func normalizeErrorRuleStatusPattern(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "*"
	}
	return value
}

func mergeErrorRules(stored []model.ErrorRule) ([]model.ErrorRule, error) {
	defaults := defaultErrorRules()
	merged := make([]model.ErrorRule, 0, len(defaults)+len(stored))
	overrides := make(map[string]model.ErrorRule, len(stored))

	for _, rule := range stored {
		normalized, err := normalizeErrorRule(rule)
		if err != nil {
			return nil, err
		}
		overrides[normalized.ID] = normalized
	}

	for _, builtIn := range defaults {
		if override, ok := overrides[builtIn.ID]; ok {
			override.BuiltIn = true
			merged = append(merged, override)
			delete(overrides, builtIn.ID)
			continue
		}
		merged = append(merged, builtIn)
	}

	for _, rule := range stored {
		override, ok := overrides[strings.TrimSpace(rule.ID)]
		if !ok {
			continue
		}
		override.BuiltIn = false
		merged = append(merged, override)
		delete(overrides, override.ID)
	}

	return merged, nil
}

func (s *SystemConfigService) GetErrorRules() ([]model.ErrorRule, error) {
	value, err := s.repo.Get(errorRulesConfigKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(value) == "" {
		return defaultErrorRules(), nil
	}

	var stored []model.ErrorRule
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return nil, err
	}
	return mergeErrorRules(stored)
}

func (s *SystemConfigService) SetErrorRules(rules []model.ErrorRule) ([]model.ErrorRule, error) {
	normalized := make([]model.ErrorRule, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		next, err := normalizeErrorRule(rule)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[next.ID]; exists {
			return nil, fmt.Errorf("规则 ID 重复")
		}
		seen[next.ID] = struct{}{}
		normalized = append(normalized, next)
	}

	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Set(errorRulesConfigKey, string(data)); err != nil {
		return nil, err
	}
	return mergeErrorRules(normalized)
}
