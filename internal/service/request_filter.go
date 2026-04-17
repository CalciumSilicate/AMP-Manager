package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ampmanager/internal/invalidation"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
)

const maxRequestFilterOperations = 50

type RequestFilterService struct {
	repo *repository.RequestFilterRepository
}

func NewRequestFilterService() *RequestFilterService {
	return &RequestFilterService{repo: repository.NewRequestFilterRepository()}
}

func (s *RequestFilterService) List() ([]model.RequestFilter, error) {
	return s.repo.ListAll()
}

func (s *RequestFilterService) Get(id string) (*model.RequestFilter, error) {
	return s.repo.GetByID(id)
}

func (s *RequestFilterService) Create(req model.RequestFilterCreateRequest) (*model.RequestFilter, error) {
	filter, err := buildRequestFilter("", req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(filter); err != nil {
		return nil, err
	}
	invalidation.Publish(invalidation.ChannelRequestFiltersUpdated)
	return s.repo.GetByID(filter.ID)
}

func (s *RequestFilterService) Update(id string, req model.RequestFilterUpdateRequest) (*model.RequestFilter, error) {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("过滤器不存在")
	}
	filter, err := buildRequestFilter(id, req)
	if err != nil {
		return nil, err
	}
	filter.CreatedAt = existing.CreatedAt
	filter.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(filter); err != nil {
		return nil, err
	}
	invalidation.Publish(invalidation.ChannelRequestFiltersUpdated)
	return s.repo.GetByID(id)
}

func (s *RequestFilterService) Delete(id string) error {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("过滤器不存在")
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	invalidation.Publish(invalidation.ChannelRequestFiltersUpdated)
	return nil
}

func buildRequestFilter(id string, req model.RequestFilterCreateRequest) (*model.RequestFilter, error) {
	now := time.Now().UTC()
	if id == "" {
		id = uuid.NewString()
	}
	filter := &model.RequestFilter{
		ID:             id,
		Name:           strings.TrimSpace(req.Name),
		Description:    strings.TrimSpace(req.Description),
		Scope:          req.Scope,
		Action:         req.Action,
		MatchType:      req.MatchType,
		Target:         strings.TrimSpace(req.Target),
		Replacement:    cloneJSON(req.Replacement),
		Priority:       req.Priority,
		IsEnabled:      req.IsEnabled,
		BindingType:    req.BindingType,
		ChannelIDs:     cloneStrings(req.ChannelIDs),
		GroupIDs:       cloneStrings(req.GroupIDs),
		RuleMode:       req.RuleMode,
		ExecutionPhase: req.ExecutionPhase,
		Operations:     cloneOperations(req.Operations),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := validateRequestFilter(filter); err != nil {
		return nil, err
	}
	return filter, nil
}

func validateRequestFilter(filter *model.RequestFilter) error {
	if filter == nil {
		return fmt.Errorf("过滤器不能为空")
	}
	if filter.Name == "" {
		return fmt.Errorf("名称不能为空")
	}
	switch filter.BindingType {
	case model.RequestFilterBindingTypeGlobal:
		if len(filter.ChannelIDs) > 0 || len(filter.GroupIDs) > 0 {
			return fmt.Errorf("全局过滤器不能绑定渠道或分组")
		}
	case model.RequestFilterBindingTypeChannels:
		if len(filter.ChannelIDs) == 0 {
			return fmt.Errorf("渠道过滤器至少选择一个渠道")
		}
		if len(filter.GroupIDs) > 0 {
			return fmt.Errorf("渠道过滤器不能同时绑定分组")
		}
	case model.RequestFilterBindingTypeGroups:
		if len(filter.GroupIDs) == 0 {
			return fmt.Errorf("分组过滤器至少选择一个分组")
		}
		if len(filter.ChannelIDs) > 0 {
			return fmt.Errorf("分组过滤器不能同时绑定渠道")
		}
	default:
		return fmt.Errorf("绑定类型无效")
	}

	switch filter.RuleMode {
	case model.RequestFilterRuleModeSimple:
		if err := validateSimpleRequestFilter(filter); err != nil {
			return err
		}
	case model.RequestFilterRuleModeAdvanced:
		if filter.ExecutionPhase != model.RequestFilterExecutionPhaseFinal {
			return fmt.Errorf("advanced 模式只允许 final 阶段")
		}
		if len(filter.Operations) == 0 {
			return fmt.Errorf("advanced 模式至少包含一个 operation")
		}
		if len(filter.Operations) > maxRequestFilterOperations {
			return fmt.Errorf("operations 不能超过 50 条")
		}
		for _, operation := range filter.Operations {
			if err := validateRequestFilterOperation(operation); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("规则模式无效")
	}

	switch filter.ExecutionPhase {
	case model.RequestFilterExecutionPhaseGuard, model.RequestFilterExecutionPhaseFinal:
	default:
		return fmt.Errorf("执行阶段无效")
	}
	return nil
}

func validateSimpleRequestFilter(filter *model.RequestFilter) error {
	switch filter.Scope {
	case model.RequestFilterScopeHeader:
		switch filter.Action {
		case model.RequestFilterActionRemove, model.RequestFilterActionSet:
		default:
			return fmt.Errorf("header 仅支持 set/remove")
		}
	case model.RequestFilterScopeBody:
		switch filter.Action {
		case model.RequestFilterActionJSONPath, model.RequestFilterActionTextReplace:
		default:
			return fmt.Errorf("body 仅支持 json_path/text_replace")
		}
	default:
		return fmt.Errorf("scope 无效")
	}

	if filter.Target == "" {
		return fmt.Errorf("target 不能为空")
	}
	if filter.Action == model.RequestFilterActionTextReplace {
		if filter.MatchType == nil {
			defaultType := model.RequestFilterMatchTypeContains
			filter.MatchType = &defaultType
		}
		switch *filter.MatchType {
		case model.RequestFilterMatchTypeContains, model.RequestFilterMatchTypeExact, model.RequestFilterMatchTypeRegex:
		default:
			return fmt.Errorf("matchType 无效")
		}
	}
	return nil
}

func validateRequestFilterOperation(operation model.RequestFilterOperation) error {
	if strings.TrimSpace(operation.Type) == "" {
		return fmt.Errorf("operation.type 不能为空")
	}
	switch operation.Type {
	case "set", "remove", "merge", "insert":
	default:
		return fmt.Errorf("operation.type 无效")
	}
	switch operation.Scope {
	case model.RequestFilterScopeHeader, model.RequestFilterScopeBody:
	default:
		return fmt.Errorf("operation.scope 无效")
	}
	if operation.Type != "merge" && strings.TrimSpace(operation.Path) == "" {
		return fmt.Errorf("operation.path 不能为空")
	}
	if len(operation.Value) > 0 && !json.Valid(operation.Value) {
		return fmt.Errorf("operation.value 必须是合法 JSON")
	}
	if operation.Matcher != nil && len(operation.Matcher.Value) > 0 && !json.Valid(operation.Matcher.Value) {
		return fmt.Errorf("operation.matcher.value 必须是合法 JSON")
	}
	if operation.Anchor != nil && len(operation.Anchor.Value) > 0 && !json.Valid(operation.Anchor.Value) {
		return fmt.Errorf("operation.anchor.value 必须是合法 JSON")
	}
	return nil
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneOperations(values []model.RequestFilterOperation) []model.RequestFilterOperation {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]model.RequestFilterOperation, 0, len(values))
	for _, item := range values {
		next := item
		next.Value = cloneJSON(item.Value)
		if item.Matcher != nil {
			matcher := *item.Matcher
			matcher.Value = cloneJSON(item.Matcher.Value)
			next.Matcher = &matcher
		}
		if item.Anchor != nil {
			anchor := *item.Anchor
			anchor.Value = cloneJSON(item.Anchor.Value)
			next.Anchor = &anchor
		}
		if item.Dedupe != nil {
			dedupe := *item.Dedupe
			dedupe.ByFields = cloneStrings(item.Dedupe.ByFields)
			next.Dedupe = &dedupe
		}
		cloned = append(cloned, next)
	}
	return cloned
}
