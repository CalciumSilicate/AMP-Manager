package billing

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// 预编译正则表达式，避免热路径重复编译
var (
	dateRe8Digit = regexp.MustCompile(`(\d{8})`)                 // YYYYMMDD 格式
	dateReDash   = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`) // YYYY-MM-DD 格式
)

// CostCalculator 成本计算器
type CostCalculator struct {
	store *PriceStore
}

// NewCostCalculator 创建成本计算器
func NewCostCalculator(store *PriceStore) *CostCalculator {
	return &CostCalculator{store: store}
}

// Calculate 计算请求成本
// pricingModel: 计价模型名（可以是 originalModel 或 mappedModel）
// usage: token 使用量
func (c *CostCalculator) Calculate(pricingModel string, usage TokenUsage) CostResult {
	result := CostResult{
		PricingModel: pricingModel,
	}

	if pricingModel == "" {
		return result
	}

	// 查找价格
	modelPrice, found := c.store.GetModelPrice(pricingModel)
	if !found {
		// 尝试模糊匹配（移除版本后缀）
		modelPrice, found = c.tryFuzzyMatch(pricingModel)
		if !found {
			log.Debugf("billing: price not found for model %s", pricingModel)
			return result
		}
	}

	result.PriceFound = true
	result.PricingModel = modelPrice.Model

	// 防御性处理：负数 token 归零
	inputTokens := usage.InputTokens
	if inputTokens < 0 {
		inputTokens = 0
	}
	outputTokens := usage.OutputTokens
	if outputTokens < 0 {
		outputTokens = 0
	}
	cacheReadTokens := usage.CacheReadInputTokens
	if cacheReadTokens < 0 {
		cacheReadTokens = 0
	}
	cacheCreationTokens := usage.CacheCreationInputTokens
	if cacheCreationTokens < 0 {
		cacheCreationTokens = 0
	}

	// 使用微美元整数累计，避免浮点误差
	uncachedInputTokens := inputTokens - cacheReadTokens
	if uncachedInputTokens < 0 {
		uncachedInputTokens = 0
	}

	priceData := modelPrice.PriceData
	totalTokens := int64(inputTokens + outputTokens)
	appliedCustomRule := false
	if rule, ok := c.store.MatchContextRule(modelPrice.Model, totalTokens); ok {
		priceData = PriceData{
			InputMicrosPerMillion:         rule.InputMicrosPerMillion,
			OutputMicrosPerMillion:        rule.OutputMicrosPerMillion,
			CacheReadMicrosPerMillion:     rule.CacheReadMicrosPerMillion,
			CacheCreationMicrosPerMillion: rule.CacheCreationMicrosPerMillion,
			InputCostPerToken:             rule.InputCostPerToken,
			OutputCostPerToken:            rule.OutputCostPerToken,
			CacheReadInputPerToken:        rule.CacheReadInputPerToken,
			CacheCreationPerToken:         rule.CacheCreationPerToken,
		}
		result.PricingRuleName = rule.RuleName
		appliedCustomRule = true
	}
	if !appliedCustomRule {
		var tierName string
		priceData, tierName = ApplyPriceDataForUsage(priceData, inputTokens)
		if tierName != "" {
			result.PricingRuleName = tierName
		}
	}

	inputMicros := int64(math.Round(float64(int64(uncachedInputTokens)*priceData.InputMicrosPerMillion) / 1_000_000.0))
	outputMicros := int64(math.Round(float64(int64(outputTokens)*priceData.OutputMicrosPerMillion) / 1_000_000.0))
	cacheReadMicros := int64(math.Round(float64(int64(cacheReadTokens)*priceData.CacheReadMicrosPerMillion) / 1_000_000.0))
	cacheCreateMicros := int64(math.Round(float64(int64(cacheCreationTokens)*priceData.CacheCreationMicrosPerMillion) / 1_000_000.0))

	totalMicros := inputMicros + outputMicros + cacheReadMicros + cacheCreateMicros
	result.CostMicros = totalMicros

	// 从 CostMicros 反推 CostUsd（保留 6 位小数）
	result.CostUsd = fmt.Sprintf("%.6f", float64(totalMicros)/1e6)

	log.Debugf("billing: calculated cost for %s - input=%d, uncached_input=%d, output=%d, cache_read=%d, cache_creation=%d -> $%s",
		pricingModel, inputTokens, uncachedInputTokens, outputTokens,
		cacheReadTokens, cacheCreationTokens, result.CostUsd)

	return result
}

func ApplyPriceDataForUsage(priceData PriceData, inputTokens int) (PriceData, string) {
	if len(priceData.Tiers) == 0 {
		return priceData, ""
	}

	var matched *PriceTier
	for index := range priceData.Tiers {
		tier := &priceData.Tiers[index]
		if int64(inputTokens) > tier.ThresholdTokens {
			matched = tier
		}
	}
	if matched == nil {
		return priceData, ""
	}
	if matched.InputMicrosPerMillion > 0 {
		priceData.InputMicrosPerMillion = matched.InputMicrosPerMillion
		priceData.InputCostPerToken = float64(matched.InputMicrosPerMillion) / 1_000_000_000_000
	}
	if matched.OutputMicrosPerMillion > 0 {
		priceData.OutputMicrosPerMillion = matched.OutputMicrosPerMillion
		priceData.OutputCostPerToken = float64(matched.OutputMicrosPerMillion) / 1_000_000_000_000
	}
	if matched.CacheReadMicrosPerMillion > 0 {
		priceData.CacheReadMicrosPerMillion = matched.CacheReadMicrosPerMillion
		priceData.CacheReadInputPerToken = float64(matched.CacheReadMicrosPerMillion) / 1_000_000_000_000
	}
	if matched.CacheCreationMicrosPerMillion > 0 {
		priceData.CacheCreationMicrosPerMillion = matched.CacheCreationMicrosPerMillion
		priceData.CacheCreationPerToken = float64(matched.CacheCreationMicrosPerMillion) / 1_000_000_000_000
	}
	return priceData, formatTierPricingRuleName(matched.ThresholdTokens)
}

func formatTierPricingRuleName(threshold int64) string {
	switch {
	case threshold%1_000_000 == 0:
		return fmt.Sprintf("%dM 以上上下文", threshold/1_000_000)
	case threshold > 1_000_000:
		return fmt.Sprintf("%.1fM 以上上下文", float64(threshold)/1_000_000)
	case threshold%1_000 == 0:
		return fmt.Sprintf("%dK 以上上下文", threshold/1_000)
	case threshold > 1_000:
		return fmt.Sprintf("%.1fK 以上上下文", float64(threshold)/1_000)
	default:
		return fmt.Sprintf("%d 以上上下文", threshold)
	}
}

// tryFuzzyMatch 尝试模糊匹配模型名
func (c *CostCalculator) tryFuzzyMatch(model string) (ModelPrice, bool) {
	// 常见的模型名变体匹配规则
	// 例如: claude-sonnet-4-20250514 -> claude-sonnet-4-20250514
	// 例如: claude-3-5-sonnet-latest -> claude-3-5-sonnet-20241022

	var candidates []ModelPrice

	// 尝试移除 "-latest" 后缀
	if strings.HasSuffix(model, "-latest") {
		baseModel := strings.TrimSuffix(model, "-latest")
		for _, price := range c.store.ListPrices() {
			if strings.HasPrefix(price.Model, baseModel) {
				candidates = append(candidates, price)
			}
		}
	}

	// 尝试匹配模型系列
	if len(candidates) == 0 {
		querySeries := extractModelSeries(model)
		for _, price := range c.store.ListPrices() {
			if querySeries != "" && extractModelSeries(price.Model) == querySeries {
				candidates = append(candidates, price)
			}
		}
	}

	if len(candidates) == 0 {
		return ModelPrice{}, false
	}

	// 多个候选时按版本日期选择最新
	if len(candidates) > 1 {
		sort.Slice(candidates, func(i, j int) bool {
			dateI := extractVersionDate(candidates[i].Model)
			dateJ := extractVersionDate(candidates[j].Model)
			return dateI > dateJ // 降序，最新在前
		})
	}

	return candidates[0], true
}

// extractModelSeries 提取模型系列名
func extractModelSeries(model string) string {
	// 匹配常见的日期格式: 20240229, 20241022 等（YYYYMMDD）
	// 以及分段日期格式: 2024-02-29, 2024-10-22 等（YYYY-MM-DD）
	parts := strings.Split(model, "-")
	var series []string
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		// 跳过纯数字的日期部分（YYYYMMDD 格式）
		if len(p) == 8 && isNumeric(p) {
			continue
		}
		// 检测 YYYY-MM-DD 分段日期（4位年-2位月-2位日）
		if len(p) == 4 && isNumeric(p) && i+2 < len(parts) {
			if len(parts[i+1]) == 2 && isNumeric(parts[i+1]) &&
				len(parts[i+2]) == 2 && isNumeric(parts[i+2]) {
				// 跳过 YYYY, MM, DD 三个部分
				i += 2
				continue
			}
		}
		series = append(series, p)
	}
	return strings.Join(series, "-")
}

// extractVersionDate 从模型名提取版本日期（返回 YYYYMMDD 格式整数）
func extractVersionDate(model string) int {
	// 匹配 YYYYMMDD 格式
	if m := dateRe8Digit.FindString(model); m != "" {
		if v, err := strconv.Atoi(m); err == nil {
			return v
		}
	}
	// 匹配 YYYY-MM-DD 格式
	if m := dateReDash.FindStringSubmatch(model); len(m) == 4 {
		dateStr := m[1] + m[2] + m[3]
		if v, err := strconv.Atoi(dateStr); err == nil {
			return v
		}
	}
	return 0
}

// isNumeric 检查字符串是否为纯数字
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// CalculateFromPointers 从指针类型计算成本（便于与现有代码集成）
func (c *CostCalculator) CalculateFromPointers(pricingModel string, inputTokens, outputTokens, cacheRead, cacheCreation *int) CostResult {
	usage := TokenUsage{}
	if inputTokens != nil {
		usage.InputTokens = *inputTokens
	}
	if outputTokens != nil {
		usage.OutputTokens = *outputTokens
	}
	if cacheRead != nil {
		usage.CacheReadInputTokens = *cacheRead
	}
	if cacheCreation != nil {
		usage.CacheCreationInputTokens = *cacheCreation
	}
	return c.Calculate(pricingModel, usage)
}

// 全局计算器实例
var globalCalculator *CostCalculator

// InitCostCalculator 初始化全局成本计算器
func InitCostCalculator() {
	globalCalculator = NewCostCalculator(GetPriceStore())
}

// GetCostCalculator 获取全局成本计算器
func GetCostCalculator() *CostCalculator {
	return globalCalculator
}
