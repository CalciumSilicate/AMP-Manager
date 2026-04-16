package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/precision"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type wholeNumber int64

func (n *wholeNumber) UnmarshalJSON(data []byte) error {
	var integerValue int64
	if err := json.Unmarshal(data, &integerValue); err == nil {
		*n = wholeNumber(integerValue)
		return nil
	}

	var floatValue float64
	if err := json.Unmarshal(data, &floatValue); err == nil {
		rounded := math.Round(floatValue)
		if math.Abs(floatValue-rounded) > 1e-9 {
			return fmt.Errorf("expected whole number, got %v", floatValue)
		}
		*n = wholeNumber(int64(rounded))
		return nil
	}

	return fmt.Errorf("invalid whole number: %s", string(data))
}

const (
	// LiteLLM 官方价格表 URL
	LiteLLMPriceURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
	// 刷新间隔
	PriceRefreshInterval = 6 * time.Hour
	// HTTP 超时
	PriceFetchTimeout = 30 * time.Second
)

// LiteLLMPricing LiteLLM 价格表条目结构
type LiteLLMPricing struct {
	LiteLLMProvider string `json:"litellm_provider"`
	Mode            string `json:"mode"`

	InputCostPerToken           *float64 `json:"input_cost_per_token,omitempty"`
	OutputCostPerToken          *float64 `json:"output_cost_per_token,omitempty"`
	InputCostPerTokenAbove272k  *float64 `json:"input_cost_per_token_above_272k_tokens,omitempty"`
	OutputCostPerTokenAbove272k *float64 `json:"output_cost_per_token_above_272k_tokens,omitempty"`

	CacheReadInputTokenCost          *float64 `json:"cache_read_input_token_cost,omitempty"`
	CacheCreationInputTokenCost      *float64 `json:"cache_creation_input_token_cost,omitempty"`
	CacheReadInputTokenCostAbove272k *float64 `json:"cache_read_input_token_cost_above_272k_tokens,omitempty"`
	SupportsPromptCaching            *bool    `json:"supports_prompt_caching,omitempty"`

	MaxInputTokens  *wholeNumber `json:"max_input_tokens,omitempty"`
	MaxOutputTokens *wholeNumber `json:"max_output_tokens,omitempty"`
}

var tierFieldPattern = regexp.MustCompile(`^(input_cost_per_token|output_cost_per_token|cache_read_input_token_cost|cache_creation_input_token_cost)_above_([0-9]+(?:\.[0-9]+)?[km]?)_tokens$`)

// PriceStore 管理模型价格表
type PriceStore struct {
	mu           sync.RWMutex
	prices       map[string]ModelPrice // model -> price
	contextRules map[string][]model.ModelPriceContextRule
	etag         string    // HTTP ETag 用于缓存协商
	fetchedAt    time.Time // 上次成功获取时间
	stopChan     chan struct{}
}

var (
	globalPriceStore *PriceStore
	priceStoreOnce   sync.Once
	stopOnce         sync.Once
)

// InitPriceStore 初始化全局价格存储
func InitPriceStore() {
	priceStoreOnce.Do(func() {
		globalPriceStore = &PriceStore{
			prices:       make(map[string]ModelPrice),
			contextRules: make(map[string][]model.ModelPriceContextRule),
			stopChan:     make(chan struct{}),
		}

		// 先从数据库加载（冷启动时使用缓存）
		if err := globalPriceStore.LoadFromDB(); err != nil {
			log.Warnf("billing: failed to load prices from DB: %v", err)
		}
		if err := globalPriceStore.LoadContextRulesFromDB(); err != nil {
			log.Warnf("billing: failed to load context rules from DB: %v", err)
		}

		// 如果数据库为空，初始化内置价格作为 seed
		if len(globalPriceStore.prices) == 0 {
			globalPriceStore.seedBuiltinPrices()
		}

		log.Infof("billing: price store initialized with %d models", len(globalPriceStore.prices))

		// 立即尝试从 LiteLLM 获取最新价格
		go func() {
			if err := globalPriceStore.FetchFromLiteLLM(context.Background()); err != nil {
				log.Warnf("billing: initial LiteLLM fetch failed: %v", err)
			}
		}()

		// 启动后台刷新
		go globalPriceStore.backgroundRefresh()
	})
}

// StopPriceStore 停止价格存储的后台任务
func StopPriceStore() {
	stopOnce.Do(func() {
		if globalPriceStore != nil && globalPriceStore.stopChan != nil {
			close(globalPriceStore.stopChan)
		}
	})
}

// GetPriceStore 获取全局价格存储
func GetPriceStore() *PriceStore {
	return globalPriceStore
}

// backgroundRefresh 后台定时刷新价格表
func (s *PriceStore) backgroundRefresh() {
	ticker := time.NewTicker(PriceRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), PriceFetchTimeout)
			if err := s.FetchFromLiteLLM(ctx); err != nil {
				log.Warnf("billing: background LiteLLM fetch failed: %v", err)
			}
			cancel()
		}
	}
}

// FetchFromLiteLLM 从 LiteLLM 获取最新价格表
func (s *PriceStore) FetchFromLiteLLM(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", LiteLLMPriceURL, nil)
	if err != nil {
		return err
	}

	// 使用 ETag 进行缓存协商
	s.mu.RLock()
	if s.etag != "" {
		req.Header.Set("If-None-Match", s.etag)
	}
	s.mu.RUnlock()

	client := &http.Client{Timeout: PriceFetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 304 Not Modified - 使用缓存
	if resp.StatusCode == http.StatusNotModified {
		s.mu.Lock()
		s.fetchedAt = time.Now()
		s.mu.Unlock()
		log.Debug("billing: LiteLLM prices not modified (304)")
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return &httpError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// 解析 JSON
	var rawPrices map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawPrices); err != nil {
		return err
	}

	// 转换为内部格式
	newPrices := make(map[string]ModelPrice)
	for model, raw := range rawPrices {
		// 跳过 sample_spec
		if model == "sample_spec" {
			continue
		}

		var lp LiteLLMPricing
		if err := json.Unmarshal(raw, &lp); err != nil {
			log.Warnf("billing: failed to unmarshal price for model %s: %v", model, err)
			continue
		}

		// 只处理有价格的条目
		if lp.InputCostPerToken == nil && lp.OutputCostPerToken == nil {
			continue
		}

		mp := ModelPrice{
			ID:       uuid.New().String(),
			Model:    model,
			Provider: lp.LiteLLMProvider,
			Source:   "litellm",
			PriceData: normalizePriceData(PriceData{
				InputCostPerToken:               ptrFloat64(lp.InputCostPerToken),
				OutputCostPerToken:              ptrFloat64(lp.OutputCostPerToken),
				CacheReadInputPerToken:          ptrFloat64(lp.CacheReadInputTokenCost),
				CacheCreationPerToken:           ptrFloat64(lp.CacheCreationInputTokenCost),
				InputCostPerTokenAbove272k:      ptrFloat64(lp.InputCostPerTokenAbove272k),
				OutputCostPerTokenAbove272k:     ptrFloat64(lp.OutputCostPerTokenAbove272k),
				CacheReadInputPerTokenAbove272k: ptrFloat64(lp.CacheReadInputTokenCostAbove272k),
				Tiers:                           extractLiteLLMPriceTiers(raw),
			}),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		newPrices[model] = mp
	}

	// 合并更新内存缓存（保留 source=manual 的条目）
	s.mu.Lock()
	for model, mp := range newPrices {
		existing, exists := s.prices[model]
		if exists && existing.Source == "manual" {
			// 保留手动设置的价格
			continue
		}
		s.prices[model] = mp
	}
	s.etag = resp.Header.Get("ETag")
	s.fetchedAt = time.Now()
	s.mu.Unlock()

	// 异步保存到数据库（只保存非 manual 的）
	go s.saveBatchToDB(newPrices)

	log.Infof("billing: fetched %d model prices from LiteLLM", len(newPrices))
	return nil
}

// ptrFloat64 安全获取 float64 指针的值
func ptrFloat64(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

type httpError struct {
	StatusCode int
}

func (e *httpError) Error() string {
	return "HTTP error: " + http.StatusText(e.StatusCode)
}

// GetPrice 获取模型价格
func (s *PriceStore) GetPrice(model string) (PriceData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if p, ok := s.prices[model]; ok {
		return normalizePriceData(p.PriceData), true
	}
	return PriceData{}, false
}

func (s *PriceStore) GetModelPrice(model string) (ModelPrice, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	price, ok := s.prices[model]
	if ok {
		price.PriceData = normalizePriceData(price.PriceData)
	}
	return price, ok
}

// SetPrice 设置模型价格
func (s *PriceStore) SetPrice(model, provider string, data PriceData, source string) error {
	now := time.Now()
	mp := ModelPrice{
		ID:        uuid.New().String(),
		Model:     model,
		Provider:  provider,
		PriceData: normalizePriceData(data),
		Source:    source,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// 锁内只更新内存 map
	s.mu.Lock()
	s.prices[model] = mp
	s.mu.Unlock()

	// 解锁后再写 DB
	return s.saveToDB(mp)
}

// LoadFromDB 从数据库加载价格表
func (s *PriceStore) LoadFromDB() error {
	db := database.GetDB()
	if db == nil {
		return nil
	}

	rows, err := db.Query(`SELECT id, model, provider, price_data, source, created_at, updated_at FROM model_prices`)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	for rows.Next() {
		var mp ModelPrice
		var priceDataJSON string
		var createdAt, updatedAt time.Time
		var provider sql.NullString

		if err := rows.Scan(&mp.ID, &mp.Model, &provider, &priceDataJSON, &mp.Source, &createdAt, &updatedAt); err != nil {
			log.Warnf("billing: failed to scan price row: %v", err)
			continue
		}

		if provider.Valid {
			mp.Provider = provider.String
		}
		mp.CreatedAt = createdAt
		mp.UpdatedAt = updatedAt

		if err := json.Unmarshal([]byte(priceDataJSON), &mp.PriceData); err != nil {
			log.Warnf("billing: failed to parse price data for %s: %v", mp.Model, err)
			continue
		}
		mp.PriceData = normalizePriceData(mp.PriceData)

		s.prices[mp.Model] = mp
	}

	return rows.Err()
}

func (s *PriceStore) LoadContextRulesFromDB() error {
	db := database.GetDB()
	if db == nil {
		return nil
	}

	rows, err := db.Query(`
		SELECT id, model, rule_name, min_tokens, max_tokens, input_micros_per_million, output_micros_per_million,
		       cache_read_micros_per_million, cache_creation_micros_per_million, input_cost_per_token, output_cost_per_token,
		       cache_read_input_per_token, cache_creation_per_token, sort_order, created_at, updated_at
		FROM model_price_context_rules
		ORDER BY model ASC, sort_order ASC, created_at ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	next := make(map[string][]model.ModelPriceContextRule)
	for rows.Next() {
		var rule model.ModelPriceContextRule
		var maxTokens sql.NullInt64
		if err := rows.Scan(
			&rule.ID,
			&rule.Model,
			&rule.RuleName,
			&rule.MinTokens,
			&maxTokens,
			&rule.InputMicrosPerMillion,
			&rule.OutputMicrosPerMillion,
			&rule.CacheReadMicrosPerMillion,
			&rule.CacheCreationMicrosPerMillion,
			&rule.InputCostPerToken,
			&rule.OutputCostPerToken,
			&rule.CacheReadInputPerToken,
			&rule.CacheCreationPerToken,
			&rule.SortOrder,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return err
		}
		if maxTokens.Valid {
			rule.MaxTokens = &maxTokens.Int64
		}
		syncModelPriceContextRule(&rule)
		next[rule.Model] = append(next[rule.Model], rule)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	s.contextRules = next
	s.mu.Unlock()
	return nil
}

// saveToDB 保存单个价格记录到数据库
func (s *PriceStore) saveToDB(mp ModelPrice) error {
	db := database.GetDB()
	if db == nil {
		return nil
	}

	priceDataJSON, err := json.Marshal(mp.PriceData)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO model_prices (id, model, provider, price_data, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(model) DO UPDATE SET
			provider = excluded.provider,
			price_data = excluded.price_data,
			source = excluded.source,
			updated_at = excluded.updated_at
	`, mp.ID, mp.Model, mp.Provider, string(priceDataJSON), mp.Source, mp.CreatedAt, mp.UpdatedAt)

	return err
}

// saveBatchToDB 批量保存价格到数据库
func (s *PriceStore) saveBatchToDB(prices map[string]ModelPrice) {
	db := database.GetDB()
	if db == nil {
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Warnf("billing: failed to begin transaction: %v", err)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT INTO model_prices (id, model, provider, price_data, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(model) DO UPDATE SET
			provider = excluded.provider,
			price_data = excluded.price_data,
			source = excluded.source,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		tx.Rollback()
		log.Warnf("billing: failed to prepare statement: %v", err)
		return
	}
	defer stmt.Close()

	for _, mp := range prices {
		mp.PriceData = normalizePriceData(mp.PriceData)
		priceDataJSON, err := json.Marshal(mp.PriceData)
		if err != nil {
			log.Warnf("billing: failed to marshal price data for %s: %v", mp.Model, err)
			continue
		}
		if _, err := stmt.Exec(mp.ID, mp.Model, mp.Provider, string(priceDataJSON), mp.Source, mp.CreatedAt, mp.UpdatedAt); err != nil {
			log.Warnf("billing: failed to save price for %s: %v", mp.Model, err)
		}
	}

	if err := tx.Commit(); err != nil {
		tx.Rollback()
		log.Warnf("billing: failed to commit transaction: %v", err)
		return
	}

	log.Debugf("billing: saved %d prices to database", len(prices))
}

func (s *PriceStore) ListContextRules(modelName string) []model.ModelPriceContextRule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rules := s.contextRules[modelName]
	if len(rules) == 0 {
		return []model.ModelPriceContextRule{}
	}
	cloned := make([]model.ModelPriceContextRule, len(rules))
	copy(cloned, rules)
	return cloned
}

func (s *PriceStore) ReplaceContextRules(modelName string, rules []model.ModelPriceContextRule) error {
	db := database.GetDB()
	if db == nil {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM model_price_context_rules WHERE model = ?`, modelName); err != nil {
		return err
	}

	for _, rule := range rules {
		syncModelPriceContextRule(&rule)
		if _, err := tx.Exec(`
			INSERT INTO model_price_context_rules (
				id, model, rule_name, min_tokens, max_tokens, input_micros_per_million, output_micros_per_million,
				cache_read_micros_per_million, cache_creation_micros_per_million, input_cost_per_token, output_cost_per_token,
				cache_read_input_per_token, cache_creation_per_token, sort_order, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			rule.ID,
			rule.Model,
			rule.RuleName,
			rule.MinTokens,
			rule.MaxTokens,
			rule.InputMicrosPerMillion,
			rule.OutputMicrosPerMillion,
			rule.CacheReadMicrosPerMillion,
			rule.CacheCreationMicrosPerMillion,
			rule.InputCostPerToken,
			rule.OutputCostPerToken,
			rule.CacheReadInputPerToken,
			rule.CacheCreationPerToken,
			rule.SortOrder,
			rule.CreatedAt,
			rule.UpdatedAt,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	next := make([]model.ModelPriceContextRule, len(rules))
	copy(next, rules)
	s.mu.Lock()
	if len(next) == 0 {
		delete(s.contextRules, modelName)
	} else {
		s.contextRules[modelName] = next
	}
	s.mu.Unlock()
	return nil
}

func normalizePriceData(data PriceData) PriceData {
	if data.InputMicrosPerMillion <= 0 && data.InputCostPerToken > 0 {
		data.InputMicrosPerMillion = precision.CostPerTokenToMicrosPerMillion(data.InputCostPerToken)
	}
	if data.OutputMicrosPerMillion <= 0 && data.OutputCostPerToken > 0 {
		data.OutputMicrosPerMillion = precision.CostPerTokenToMicrosPerMillion(data.OutputCostPerToken)
	}
	if data.CacheReadMicrosPerMillion <= 0 && data.CacheReadInputPerToken > 0 {
		data.CacheReadMicrosPerMillion = precision.CostPerTokenToMicrosPerMillion(data.CacheReadInputPerToken)
	}
	if data.CacheCreationMicrosPerMillion <= 0 && data.CacheCreationPerToken > 0 {
		data.CacheCreationMicrosPerMillion = precision.CostPerTokenToMicrosPerMillion(data.CacheCreationPerToken)
	}
	if data.InputMicrosPerMillionAbove272k <= 0 && data.InputCostPerTokenAbove272k > 0 {
		data.InputMicrosPerMillionAbove272k = precision.CostPerTokenToMicrosPerMillion(data.InputCostPerTokenAbove272k)
	}
	if data.OutputMicrosPerMillionAbove272k <= 0 && data.OutputCostPerTokenAbove272k > 0 {
		data.OutputMicrosPerMillionAbove272k = precision.CostPerTokenToMicrosPerMillion(data.OutputCostPerTokenAbove272k)
	}
	if data.CacheReadMicrosPerMillionAbove272k <= 0 && data.CacheReadInputPerTokenAbove272k > 0 {
		data.CacheReadMicrosPerMillionAbove272k = precision.CostPerTokenToMicrosPerMillion(data.CacheReadInputPerTokenAbove272k)
	}
	if data.CacheCreationMicrosPerMillion > 0 && data.CacheCreationPerToken == 0 {
		data.CacheCreationPerToken = precision.MicrosPerMillionToCostPerToken(data.CacheCreationMicrosPerMillion)
	}
	if data.CacheCreationPerToken > 0 && data.CacheCreationMicrosPerMillion == 0 {
		data.CacheCreationMicrosPerMillion = precision.CostPerTokenToMicrosPerMillion(data.CacheCreationPerToken)
	}

	data.InputCostPerToken = precision.MicrosPerMillionToCostPerToken(data.InputMicrosPerMillion)
	data.OutputCostPerToken = precision.MicrosPerMillionToCostPerToken(data.OutputMicrosPerMillion)
	data.CacheReadInputPerToken = precision.MicrosPerMillionToCostPerToken(data.CacheReadMicrosPerMillion)
	data.CacheCreationPerToken = precision.MicrosPerMillionToCostPerToken(data.CacheCreationMicrosPerMillion)
	data.InputCostPerTokenAbove272k = precision.MicrosPerMillionToCostPerToken(data.InputMicrosPerMillionAbove272k)
	data.OutputCostPerTokenAbove272k = precision.MicrosPerMillionToCostPerToken(data.OutputMicrosPerMillionAbove272k)
	data.CacheReadInputPerTokenAbove272k = precision.MicrosPerMillionToCostPerToken(data.CacheReadMicrosPerMillionAbove272k)

	if len(data.Tiers) == 0 {
		data = appendLegacyAbove272kTier(data)
	}
	if len(data.Tiers) > 0 {
		sortedTiers := make([]PriceTier, 0, len(data.Tiers))
		for _, tier := range data.Tiers {
			if tier.ThresholdTokens <= 0 {
				continue
			}
			if tier.InputMicrosPerMillion < 0 || tier.OutputMicrosPerMillion < 0 || tier.CacheReadMicrosPerMillion < 0 || tier.CacheCreationMicrosPerMillion < 0 {
				continue
			}
			sortedTiers = append(sortedTiers, tier)
		}
		sort.Slice(sortedTiers, func(i, j int) bool {
			return sortedTiers[i].ThresholdTokens < sortedTiers[j].ThresholdTokens
		})
		data.Tiers = sortedTiers
	}
	return data
}

func appendLegacyAbove272kTier(data PriceData) PriceData {
	if data.InputMicrosPerMillionAbove272k <= 0 && data.OutputMicrosPerMillionAbove272k <= 0 && data.CacheReadMicrosPerMillionAbove272k <= 0 {
		return data
	}
	threshold, err := parseTokenThresholdValue("272k")
	if err != nil || threshold <= 0 {
		return data
	}
	data.Tiers = append(data.Tiers, PriceTier{
		ThresholdTokens:           threshold,
		InputMicrosPerMillion:     data.InputMicrosPerMillionAbove272k,
		OutputMicrosPerMillion:    data.OutputMicrosPerMillionAbove272k,
		CacheReadMicrosPerMillion: data.CacheReadMicrosPerMillionAbove272k,
	})
	return data
}

func extractLiteLLMPriceTiers(raw json.RawMessage) []PriceTier {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}

	tiers := make(map[int64]*PriceTier)
	for key, rawValue := range fields {
		matches := tierFieldPattern.FindStringSubmatch(key)
		if len(matches) != 3 {
			continue
		}
		var value *float64
		if err := json.Unmarshal(rawValue, &value); err != nil || value == nil {
			continue
		}
		threshold, err := parseTokenThresholdValue(matches[2])
		if err != nil || threshold <= 0 {
			continue
		}
		tier, ok := tiers[threshold]
		if !ok {
			tier = &PriceTier{ThresholdTokens: threshold}
			tiers[threshold] = tier
		}
		micros := precision.CostPerTokenToMicrosPerMillion(*value)
		switch matches[1] {
		case "input_cost_per_token":
			tier.InputMicrosPerMillion = micros
		case "output_cost_per_token":
			tier.OutputMicrosPerMillion = micros
		case "cache_read_input_token_cost":
			tier.CacheReadMicrosPerMillion = micros
		case "cache_creation_input_token_cost":
			tier.CacheCreationMicrosPerMillion = micros
		}
	}

	if len(tiers) == 0 {
		return nil
	}
	out := make([]PriceTier, 0, len(tiers))
	for _, tier := range tiers {
		out = append(out, *tier)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ThresholdTokens < out[j].ThresholdTokens
	})
	return out
}

func parseTokenThresholdValue(raw string) (int64, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, ",", "")
	multiplier := 1.0
	switch {
	case strings.HasSuffix(value, "k"):
		multiplier = 1_000
		value = strings.TrimSuffix(value, "k")
	case strings.HasSuffix(value, "m"):
		multiplier = 1_000_000
		value = strings.TrimSuffix(value, "m")
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	return int64(math.Round(parsed * multiplier)), nil
}

func syncModelPriceContextRule(rule *model.ModelPriceContextRule) {
	if rule == nil {
		return
	}
	if rule.InputMicrosPerMillion <= 0 && rule.InputCostPerToken > 0 {
		rule.InputMicrosPerMillion = precision.CostPerTokenToMicrosPerMillion(rule.InputCostPerToken)
	}
	if rule.OutputMicrosPerMillion <= 0 && rule.OutputCostPerToken > 0 {
		rule.OutputMicrosPerMillion = precision.CostPerTokenToMicrosPerMillion(rule.OutputCostPerToken)
	}
	if rule.CacheReadMicrosPerMillion <= 0 && rule.CacheReadInputPerToken > 0 {
		rule.CacheReadMicrosPerMillion = precision.CostPerTokenToMicrosPerMillion(rule.CacheReadInputPerToken)
	}
	if rule.CacheCreationMicrosPerMillion <= 0 && rule.CacheCreationPerToken > 0 {
		rule.CacheCreationMicrosPerMillion = precision.CostPerTokenToMicrosPerMillion(rule.CacheCreationPerToken)
	}
	rule.InputCostPerToken = precision.MicrosPerMillionToCostPerToken(rule.InputMicrosPerMillion)
	rule.OutputCostPerToken = precision.MicrosPerMillionToCostPerToken(rule.OutputMicrosPerMillion)
	rule.CacheReadInputPerToken = precision.MicrosPerMillionToCostPerToken(rule.CacheReadMicrosPerMillion)
	rule.CacheCreationPerToken = precision.MicrosPerMillionToCostPerToken(rule.CacheCreationMicrosPerMillion)
}

func (s *PriceStore) MatchContextRule(modelName string, totalTokens int64) (*model.ModelPriceContextRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rule := range s.contextRules[modelName] {
		if totalTokens < rule.MinTokens {
			continue
		}
		if rule.MaxTokens != nil && totalTokens >= *rule.MaxTokens {
			continue
		}
		matched := rule
		return &matched, true
	}
	return nil, false
}

func (s *PriceStore) FindContextRuleByName(modelName, ruleName string) (*model.ModelPriceContextRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rule := range s.contextRules[modelName] {
		if rule.RuleName == ruleName {
			matched := rule
			return &matched, true
		}
	}
	return nil, false
}

// seedBuiltinPrices 初始化内置价格表（作为 fallback）
func (s *PriceStore) seedBuiltinPrices() {
	builtins := []struct {
		model    string
		provider string
		data     PriceData
	}{
		{
			model:    "gpt-4.1-mini",
			provider: "openai",
			data: PriceData{
				InputCostPerToken:      0.40 / 1_000_000,
				OutputCostPerToken:     1.60 / 1_000_000,
				CacheReadInputPerToken: 0.10 / 1_000_000,
				CacheCreationPerToken:  0.40 / 1_000_000,
			},
		},
		{
			model:    "gpt-4o-mini",
			provider: "openai",
			data: PriceData{
				InputCostPerToken:      0.15 / 1_000_000,
				OutputCostPerToken:     0.60 / 1_000_000,
				CacheReadInputPerToken: 0.075 / 1_000_000,
				CacheCreationPerToken:  0.15 / 1_000_000,
			},
		},
		// Anthropic Claude 4 系列 (2025 年价格)
		{
			model:    "claude-sonnet-4-20250514",
			provider: "anthropic",
			data: PriceData{
				InputCostPerToken:      3.0 / 1_000_000,  // $3/1M
				OutputCostPerToken:     15.0 / 1_000_000, // $15/1M
				CacheReadInputPerToken: 0.30 / 1_000_000, // $0.30/1M
				CacheCreationPerToken:  3.75 / 1_000_000, // $3.75/1M
			},
		},
		{
			model:    "claude-opus-4-20250514",
			provider: "anthropic",
			data: PriceData{
				InputCostPerToken:      15.0 / 1_000_000,
				OutputCostPerToken:     75.0 / 1_000_000,
				CacheReadInputPerToken: 1.50 / 1_000_000,
				CacheCreationPerToken:  18.75 / 1_000_000,
			},
		},
		// Claude 3.5 系列
		{
			model:    "claude-3-5-sonnet-20241022",
			provider: "anthropic",
			data: PriceData{
				InputCostPerToken:      3.0 / 1_000_000,
				OutputCostPerToken:     15.0 / 1_000_000,
				CacheReadInputPerToken: 0.30 / 1_000_000,
				CacheCreationPerToken:  3.75 / 1_000_000,
			},
		},
		{
			model:    "claude-3-5-haiku-20241022",
			provider: "anthropic",
			data: PriceData{
				InputCostPerToken:      0.80 / 1_000_000,
				OutputCostPerToken:     4.0 / 1_000_000,
				CacheReadInputPerToken: 0.08 / 1_000_000,
				CacheCreationPerToken:  1.0 / 1_000_000,
			},
		},
	}

	for _, b := range builtins {
		now := time.Now()
		mp := ModelPrice{
			ID:        uuid.New().String(),
			Model:     b.model,
			Provider:  b.provider,
			PriceData: b.data,
			Source:    "builtin",
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.prices[b.model] = mp
		_ = s.saveToDB(mp)
	}

	log.Infof("billing: seeded %d builtin model prices", len(builtins))
}

// ListPrices 列出所有价格
func (s *PriceStore) ListPrices() []ModelPrice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ModelPrice, 0, len(s.prices))
	for _, p := range s.prices {
		result = append(result, p)
	}
	return result
}

// GetStats 获取价格存储统计信息
func (s *PriceStore) GetStats() (count int, source string, fetchedAt time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.prices), "litellm", s.fetchedAt
}
