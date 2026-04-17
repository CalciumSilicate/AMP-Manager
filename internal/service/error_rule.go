package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ampmanager/internal/errorruleutil"
	"ampmanager/internal/invalidation"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
)

type ErrorRuleService struct {
	repo *repository.ErrorRuleRepository
}

func NewErrorRuleService() *ErrorRuleService {
	return &ErrorRuleService{repo: repository.NewErrorRuleRepository()}
}

func (s *ErrorRuleService) SyncDefaultErrorRules() error {
	existing, err := s.repo.ListAll()
	if err != nil {
		return err
	}

	byPattern := make(map[string]model.ErrorRule, len(existing))
	for _, item := range existing {
		if !item.IsDefault {
			continue
		}
		byPattern[defaultRuleKey(item.RequestType, item.Pattern)] = item
	}

	seenKeys := make(map[string]struct{}, len(defaultErrorRules()))
	now := time.Now().UTC()
	for _, defaultRule := range defaultErrorRules() {
		key := defaultRuleKey(defaultRule.RequestType, defaultRule.Pattern)
		seenKeys[key] = struct{}{}
		existingRule, ok := byPattern[key]
		if !ok {
			item := defaultRule
			item.ID = uuid.NewString()
			item.CreatedAt = now
			item.UpdatedAt = now
			if err := s.repo.Create(&item); err != nil {
				return err
			}
			continue
		}
		defaultRule.ID = existingRule.ID
		defaultRule.CreatedAt = existingRule.CreatedAt
		defaultRule.UpdatedAt = now
		if err := s.repo.Update(&defaultRule); err != nil {
			return err
		}
	}

	for _, item := range existing {
		if !item.IsDefault {
			continue
		}
		if _, ok := seenKeys[defaultRuleKey(item.RequestType, item.Pattern)]; ok {
			continue
		}
		if err := s.repo.Delete(item.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *ErrorRuleService) List() ([]model.ErrorRule, error) {
	if err := s.SyncDefaultErrorRules(); err != nil {
		return nil, err
	}
	return s.repo.ListAll()
}

func (s *ErrorRuleService) Create(req model.ErrorRuleCreateRequest) (*model.ErrorRule, error) {
	rule, err := s.buildRuleFromCreate(req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(rule); err != nil {
		return nil, err
	}
	invalidation.Publish(invalidation.ChannelErrorRulesUpdated)
	return s.repo.GetByID(rule.ID)
}

func (s *ErrorRuleService) Update(id string, req model.ErrorRuleUpdateRequest) (*model.ErrorRule, error) {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("规则不存在")
	}

	updated, err := s.buildRuleFromUpdate(existing, req)
	if err != nil {
		return nil, err
	}
	if existing.IsDefault {
		updated.IsDefault = false
	}
	if err := s.repo.Update(updated); err != nil {
		return nil, err
	}
	invalidation.Publish(invalidation.ChannelErrorRulesUpdated)
	return s.repo.GetByID(id)
}

func (s *ErrorRuleService) Delete(id string) error {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("规则不存在")
	}
	if existing.IsDefault {
		return fmt.Errorf("默认规则不能删除")
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	invalidation.Publish(invalidation.ChannelErrorRulesUpdated)
	return nil
}

func (s *ErrorRuleService) RefreshRuntime() error {
	if err := s.SyncDefaultErrorRules(); err != nil {
		return err
	}
	invalidation.Publish(invalidation.ChannelErrorRulesUpdated)
	return nil
}

func (s *ErrorRuleService) buildRuleFromCreate(req model.ErrorRuleCreateRequest) (*model.ErrorRule, error) {
	now := time.Now().UTC()
	rule := &model.ErrorRule{
		ID:                 uuid.NewString(),
		Name:               strings.TrimSpace(req.Name),
		Description:        strings.TrimSpace(req.Description),
		RequestType:        req.RequestType,
		UpstreamStatus:     normalizeErrorRuleStatusPattern(req.UpstreamStatus),
		Pattern:            strings.TrimSpace(req.Pattern),
		MatchType:          req.MatchType,
		Category:           strings.TrimSpace(req.Category),
		Priority:           req.Priority,
		IsEnabled:          req.IsEnabled,
		IsDefault:          false,
		OverrideStatusCode: req.OverrideStatusCode,
		OverrideMessage:    strings.TrimSpace(req.OverrideMessage),
		OverrideResponse:   errorruleutil.SanitizedOverrideResponse(req.OverrideResponse),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := validateErrorRule(rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *ErrorRuleService) buildRuleFromUpdate(existing *model.ErrorRule, req model.ErrorRuleUpdateRequest) (*model.ErrorRule, error) {
	rule := *existing
	rule.Name = strings.TrimSpace(req.Name)
	rule.Description = strings.TrimSpace(req.Description)
	rule.UpstreamStatus = normalizeErrorRuleStatusPattern(req.UpstreamStatus)
	rule.Pattern = strings.TrimSpace(req.Pattern)
	rule.MatchType = req.MatchType
	rule.Category = strings.TrimSpace(req.Category)
	rule.Priority = req.Priority
	rule.IsEnabled = req.IsEnabled
	rule.OverrideStatusCode = req.OverrideStatusCode
	rule.OverrideMessage = strings.TrimSpace(req.OverrideMessage)
	rule.OverrideResponse = errorruleutil.SanitizedOverrideResponse(req.OverrideResponse)
	rule.UpdatedAt = time.Now().UTC()
	if err := validateErrorRule(&rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

func validateErrorRule(rule *model.ErrorRule) error {
	if rule == nil {
		return fmt.Errorf("规则不能为空")
	}
	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("规则名称不能为空")
	}
	switch rule.RequestType {
	case model.ErrorRuleRequestTypeResponses,
		model.ErrorRuleRequestTypeChatCompletions,
		model.ErrorRuleRequestTypeGemini,
		model.ErrorRuleRequestTypeAnthropic:
	default:
		return fmt.Errorf("请求类型无效")
	}
	switch rule.MatchType {
	case model.ErrorRuleMatchTypeContains, model.ErrorRuleMatchTypeExact, model.ErrorRuleMatchTypeRegex:
	default:
		return fmt.Errorf("匹配类型无效")
	}
	if strings.TrimSpace(rule.Pattern) == "" {
		return fmt.Errorf("匹配内容不能为空")
	}
	if strings.TrimSpace(rule.Category) == "" {
		return fmt.Errorf("分类不能为空")
	}
	if rule.OverrideStatusCode != nil {
		if *rule.OverrideStatusCode < 400 || *rule.OverrideStatusCode > 599 {
			return fmt.Errorf("覆盖状态码必须在 400-599")
		}
	}
	if len(rule.OverrideResponse) > 0 {
		if err := errorruleutil.ValidateOverrideResponse(rule.OverrideResponse); err != nil {
			return err
		}
	}
	if rule.MatchType == model.ErrorRuleMatchTypeRegex {
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("正则表达式无效")
		}
	}
	return validateErrorStatusPattern(rule.UpstreamStatus)
}

func validateErrorStatusPattern(pattern string) error {
	pattern = normalizeErrorRuleStatusPattern(pattern)
	if pattern == "*" {
		return nil
	}
	if strings.Contains(pattern, "-") {
		parts := strings.SplitN(pattern, "-", 2)
		if len(parts) != 2 {
			return fmt.Errorf("上游状态码范围无效")
		}
		if !statusTokenOK(parts[0]) || !statusTokenOK(parts[1]) {
			return fmt.Errorf("上游状态码范围无效")
		}
		return nil
	}
	if !statusTokenOK(pattern) {
		return fmt.Errorf("上游状态码无效")
	}
	return nil
}

func statusTokenOK(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 3 {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func normalizeErrorRuleStatusPattern(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "*"
	}
	return value
}

func defaultErrorRules() []model.ErrorRule {
	return []model.ErrorRule{
		{
			Name:            "OpenAI 风格错误字符串（Responses）",
			Description:     "OpenAI Responses 200 body 内含通用错误文本",
			RequestType:     model.ErrorRuleRequestTypeResponses,
			UpstreamStatus:  "200",
			Pattern:         "An Error Occurred",
			MatchType:       model.ErrorRuleMatchTypeContains,
			Category:        "model_error",
			Priority:        100,
			IsEnabled:       true,
			IsDefault:       true,
			OverrideMessage: "上游返回了错误内容",
			OverrideResponse: mustJSON(map[string]any{
				"error": map[string]any{
					"type":    "server_error",
					"message": "上游返回了错误内容",
					"code":    "upstream_error",
				},
			}),
		},
		{
			Name:            "OpenAI 风格错误字符串（Chat）",
			Description:     "Chat Completions 200 body 内含通用错误文本",
			RequestType:     model.ErrorRuleRequestTypeChatCompletions,
			UpstreamStatus:  "200",
			Pattern:         "An Error Occurred",
			MatchType:       model.ErrorRuleMatchTypeContains,
			Category:        "model_error",
			Priority:        100,
			IsEnabled:       true,
			IsDefault:       true,
			OverrideMessage: "上游返回了错误内容",
			OverrideResponse: mustJSON(map[string]any{
				"error": map[string]any{
					"type":    "server_error",
					"message": "上游返回了错误内容",
					"code":    "upstream_error",
				},
			}),
		},
		{
			Name:            "200 但 body 为 error 对象（Responses）",
			Description:     "Responses fake-200 error object",
			RequestType:     model.ErrorRuleRequestTypeResponses,
			UpstreamStatus:  "200",
			Pattern:         `(?is)^\s*\{.*"error"\s*:\s*(\{|\").*`,
			MatchType:       model.ErrorRuleMatchTypeRegex,
			Category:        "model_error",
			Priority:        90,
			IsEnabled:       true,
			IsDefault:       true,
			OverrideMessage: "上游返回了错误对象",
			OverrideResponse: mustJSON(map[string]any{
				"error": map[string]any{
					"type":    "server_error",
					"message": "上游返回了错误对象",
					"code":    "upstream_error",
				},
			}),
		},
		{
			Name:            "200 但 body 为 error 对象（Chat）",
			Description:     "Chat fake-200 error object",
			RequestType:     model.ErrorRuleRequestTypeChatCompletions,
			UpstreamStatus:  "200",
			Pattern:         `(?is)^\s*\{.*"error"\s*:\s*(\{|\").*`,
			MatchType:       model.ErrorRuleMatchTypeRegex,
			Category:        "model_error",
			Priority:        90,
			IsEnabled:       true,
			IsDefault:       true,
			OverrideMessage: "上游返回了错误对象",
			OverrideResponse: mustJSON(map[string]any{
				"error": map[string]any{
					"type":    "server_error",
					"message": "上游返回了错误对象",
					"code":    "upstream_error",
				},
			}),
		},
		{
			Name:            "200 但 body 为 error 对象（Gemini）",
			Description:     "Gemini fake-200 error object",
			RequestType:     model.ErrorRuleRequestTypeGemini,
			UpstreamStatus:  "200",
			Pattern:         `(?is)^\s*\{.*"error"\s*:\s*(\{|\").*`,
			MatchType:       model.ErrorRuleMatchTypeRegex,
			Category:        "model_error",
			Priority:        90,
			IsEnabled:       true,
			IsDefault:       true,
			OverrideMessage: "上游返回了错误对象",
			OverrideResponse: mustJSON(map[string]any{
				"error": map[string]any{
					"code":    502,
					"message": "上游返回了错误对象",
					"status":  http.StatusText(http.StatusBadGateway),
				},
			}),
		},
		{
			Name:            "200 但 body 为 error 对象（Anthropic）",
			Description:     "Anthropic fake-200 error object",
			RequestType:     model.ErrorRuleRequestTypeAnthropic,
			UpstreamStatus:  "200",
			Pattern:         `(?is)^\s*\{.*"error"\s*:\s*(\{|\").*`,
			MatchType:       model.ErrorRuleMatchTypeRegex,
			Category:        "model_error",
			Priority:        90,
			IsEnabled:       true,
			IsDefault:       true,
			OverrideMessage: "上游返回了错误对象",
			OverrideResponse: mustJSON(map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "server_error",
					"message": "上游返回了错误对象",
				},
			}),
		},
		{
			Name:            "Prompt 过长",
			Description:     "Prompt token limit exceeded",
			RequestType:     model.ErrorRuleRequestTypeAnthropic,
			UpstreamStatus:  "400-599",
			Pattern:         `prompt is too long.*(tokens.*maximum|maximum.*tokens)`,
			MatchType:       model.ErrorRuleMatchTypeRegex,
			Category:        "prompt_limit",
			Priority:        80,
			IsEnabled:       true,
			IsDefault:       true,
			OverrideMessage: "输入内容过长，请减少 Prompt 后重试",
			OverrideResponse: mustJSON(map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "invalid_request_error",
					"message": "输入内容过长，请减少 Prompt 后重试",
				},
			}),
		},
		{
			Name:            "Input 过长",
			Description:     "Input content length exceeds provider limit",
			RequestType:     model.ErrorRuleRequestTypeAnthropic,
			UpstreamStatus:  "400-599",
			Pattern:         `Input is too long`,
			MatchType:       model.ErrorRuleMatchTypeContains,
			Category:        "input_limit",
			Priority:        75,
			IsEnabled:       true,
			IsDefault:       true,
			OverrideMessage: "输入内容超过供应商限制，请减少输入长度后重试",
			OverrideResponse: mustJSON(map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "invalid_request_error",
					"message": "输入内容超过供应商限制，请减少输入长度后重试",
				},
			}),
		},
	}
}

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func defaultRuleKey(requestType model.ErrorRuleRequestType, pattern string) string {
	return string(requestType) + "\x00" + pattern
}
