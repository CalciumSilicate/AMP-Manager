package service

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ampmanager/internal/billing"
	"ampmanager/internal/model"
	"ampmanager/internal/precision"

	"github.com/google/uuid"
)

type ModelPriceContextRuleService struct{}

func NewModelPriceContextRuleService() *ModelPriceContextRuleService {
	return &ModelPriceContextRuleService{}
}

func (s *ModelPriceContextRuleService) List(modelName string) ([]model.ModelPriceContextRule, error) {
	store := billing.GetPriceStore()
	if store == nil {
		return nil, fmt.Errorf("价格服务未初始化")
	}
	return store.ListContextRules(strings.TrimSpace(modelName)), nil
}

func (s *ModelPriceContextRuleService) Replace(modelName string, reqs []model.ModelPriceContextRuleRequest) ([]model.ModelPriceContextRule, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, fmt.Errorf("model 不能为空")
	}

	next := make([]model.ModelPriceContextRule, 0, len(reqs))
	now := time.Now().UTC()
	for idx, req := range reqs {
		ruleName := strings.TrimSpace(req.RuleName)
		if ruleName == "" {
			return nil, fmt.Errorf("规则名称不能为空")
		}
		if req.MinTokens < 0 {
			return nil, fmt.Errorf("最小 token 不能小于 0")
		}
		if req.MaxTokens != nil && *req.MaxTokens <= req.MinTokens {
			return nil, fmt.Errorf("规则 %q 的最大 token 必须大于最小 token", ruleName)
		}
		inputMicrosPerMillion, err := resolveRuleMicrosPerMillion(req.InputMicrosPerMillion, req.InputCostPerToken)
		if err != nil {
			return nil, fmt.Errorf("规则 %q 输入价格无效: %w", ruleName, err)
		}
		outputMicrosPerMillion, err := resolveRuleMicrosPerMillion(req.OutputMicrosPerMillion, req.OutputCostPerToken)
		if err != nil {
			return nil, fmt.Errorf("规则 %q 输出价格无效: %w", ruleName, err)
		}
		cacheReadMicrosPerMillion, err := resolveRuleMicrosPerMillion(req.CacheReadMicrosPerMillion, req.CacheReadInputPerToken)
		if err != nil {
			return nil, fmt.Errorf("规则 %q 缓存读价格无效: %w", ruleName, err)
		}
		cacheCreationMicrosPerMillion, err := resolveRuleMicrosPerMillion(req.CacheCreationMicrosPerMillion, req.CacheCreationPerToken)
		if err != nil {
			return nil, fmt.Errorf("规则 %q 缓存写价格无效: %w", ruleName, err)
		}

		next = append(next, model.ModelPriceContextRule{
			ID:                            uuid.New().String(),
			Model:                         modelName,
			RuleName:                      ruleName,
			MinTokens:                     req.MinTokens,
			MaxTokens:                     req.MaxTokens,
			InputMicrosPerMillion:         inputMicrosPerMillion,
			OutputMicrosPerMillion:        outputMicrosPerMillion,
			CacheReadMicrosPerMillion:     cacheReadMicrosPerMillion,
			CacheCreationMicrosPerMillion: cacheCreationMicrosPerMillion,
			InputCostPerToken:             precision.MicrosPerMillionToCostPerToken(inputMicrosPerMillion),
			OutputCostPerToken:            precision.MicrosPerMillionToCostPerToken(outputMicrosPerMillion),
			CacheReadInputPerToken:        precision.MicrosPerMillionToCostPerToken(cacheReadMicrosPerMillion),
			CacheCreationPerToken:         precision.MicrosPerMillionToCostPerToken(cacheCreationMicrosPerMillion),
			SortOrder:                     idx,
			CreatedAt:                     now,
			UpdatedAt:                     now,
		})
	}

	sort.Slice(next, func(i, j int) bool {
		if next[i].MinTokens == next[j].MinTokens {
			return next[i].SortOrder < next[j].SortOrder
		}
		return next[i].MinTokens < next[j].MinTokens
	})

	prevMax := int64(math.MinInt64)
	hasOpenEnded := false
	for _, rule := range next {
		if hasOpenEnded {
			return nil, fmt.Errorf("存在开放区间后仍有后续规则，规则 %q 与前序区间重叠", rule.RuleName)
		}
		if rule.MinTokens < prevMax {
			return nil, fmt.Errorf("规则 %q 与前序区间重叠", rule.RuleName)
		}
		if rule.MaxTokens == nil {
			hasOpenEnded = true
			prevMax = math.MaxInt64
			continue
		}
		prevMax = *rule.MaxTokens
	}

	store := billing.GetPriceStore()
	if store == nil {
		return nil, fmt.Errorf("价格服务未初始化")
	}
	if err := store.ReplaceContextRules(modelName, next); err != nil {
		return nil, err
	}
	return next, nil
}

func resolveRuleMicrosPerMillion(ppmValue *int64, legacy precision.DecimalString) (int64, error) {
	if ppmValue != nil {
		if *ppmValue < 0 {
			return 0, fmt.Errorf("value must be non-negative")
		}
		return *ppmValue, nil
	}
	return precision.ParseUSDPerMillionToMicros(legacy)
}
