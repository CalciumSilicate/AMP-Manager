package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	internaltranslator "ampmanager/internal/translator"
	"ampmanager/internal/util"
)

var (
	ErrChannelNotFound = errors.New("渠道不存在")
)

const defaultChannelRepoCacheKey = "default-channel-repo"

// modelsCache 缓存 ModelsJSON -> []model.ChannelModel 的解析结果
// key: ModelsJSON 字符串, value: *parsedModelsEntry
var modelsCache sync.Map
var compiledChannelModelRulesCache sync.Map
var headersCache sync.Map
var channelTranslatorCache sync.Map
var enabledChannelsSnapshot = &enabledChannelsCache{
	snapshots: make(map[string]enabledChannelsSnapshotEntry),
	cacheTTL:  2 * time.Second,
}

type parsedModelsEntry struct {
	models []model.ChannelModel
	valid  bool
}

type compiledChannelModelRule struct {
	name         string
	alias        string
	nameLower    string
	aliasLower   string
	wildcardName bool
}

type parsedHeadersEntry struct {
	headers map[string]string
	valid   bool
}

type parsedChannelTranslatorEntry struct {
	translator model.ChannelTranslator
	valid      bool
}

type enabledChannelsCache struct {
	mu        sync.RWMutex
	snapshots map[string]enabledChannelsSnapshotEntry
	cacheTTL  time.Duration
}

type enabledChannelsSnapshotEntry struct {
	channels        []*model.Channel
	channelGroupMap map[string][]string
	loadedAt        time.Time
}

// getParsedModels 从缓存获取或解析 ModelsJSON
func getParsedModels(modelsJSON string) ([]model.ChannelModel, bool) {
	if cached, ok := modelsCache.Load(modelsJSON); ok {
		entry := cached.(*parsedModelsEntry)
		return entry.models, entry.valid
	}

	var models []model.ChannelModel
	err := json.Unmarshal([]byte(modelsJSON), &models)
	entry := &parsedModelsEntry{
		models: models,
		valid:  err == nil,
	}
	modelsCache.Store(modelsJSON, entry)
	return models, entry.valid
}

func GetParsedChannelModels(modelsJSON string) ([]model.ChannelModel, bool) {
	return getParsedModels(modelsJSON)
}

func getCompiledChannelModelRules(modelsJSON string) ([]compiledChannelModelRule, bool) {
	if cached, ok := compiledChannelModelRulesCache.Load(modelsJSON); ok {
		return cached.([]compiledChannelModelRule), true
	}

	models, valid := getParsedModels(modelsJSON)
	if !valid {
		return nil, false
	}

	rules := make([]compiledChannelModelRule, 0, len(models))
	for _, modelRule := range models {
		rules = append(rules, compiledChannelModelRule{
			name:         modelRule.Name,
			alias:        modelRule.Alias,
			nameLower:    strings.ToLower(modelRule.Name),
			aliasLower:   strings.ToLower(modelRule.Alias),
			wildcardName: strings.Contains(strings.ToLower(modelRule.Name), "*"),
		})
	}
	compiledChannelModelRulesCache.Store(modelsJSON, rules)
	return rules, true
}

func MatchChannelModelRules(modelID string, modelsJSON string) bool {
	rules, valid := getCompiledChannelModelRules(modelsJSON)
	if !valid || len(rules) == 0 {
		return true
	}

	modelLower := strings.ToLower(modelID)
	for _, rule := range rules {
		if strings.EqualFold(rule.name, modelID) || (rule.alias != "" && strings.EqualFold(rule.alias, modelID)) {
			return true
		}
		if rule.wildcardName && wildcardMatch(rule.nameLower, modelLower) {
			return true
		}
	}
	return false
}

func wildcardMatch(pattern, text string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(text, strings.Trim(pattern, "*"))
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(text, strings.TrimPrefix(pattern, "*"))
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(text, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == text
}

func getParsedHeaders(headersJSON string) (map[string]string, bool) {
	if cached, ok := headersCache.Load(headersJSON); ok {
		entry := cached.(*parsedHeadersEntry)
		return entry.headers, entry.valid
	}

	var headers map[string]string
	err := json.Unmarshal([]byte(headersJSON), &headers)
	entry := &parsedHeadersEntry{
		headers: headers,
		valid:   err == nil,
	}
	headersCache.Store(headersJSON, entry)
	return headers, entry.valid
}

func getParsedChannelTranslator(translatorJSON string) (model.ChannelTranslator, bool) {
	if strings.TrimSpace(translatorJSON) == "" {
		return model.ChannelTranslator{}, true
	}
	if cached, ok := channelTranslatorCache.Load(translatorJSON); ok {
		entry := cached.(*parsedChannelTranslatorEntry)
		return entry.translator, entry.valid
	}

	var translatorConfig model.ChannelTranslator
	err := json.Unmarshal([]byte(translatorJSON), &translatorConfig)
	entry := &parsedChannelTranslatorEntry{
		translator: translatorConfig,
		valid:      err == nil,
	}
	channelTranslatorCache.Store(translatorJSON, entry)
	return translatorConfig, entry.valid
}

func marshalChannelTranslator(translatorConfig model.ChannelTranslator) string {
	if translatorConfig == (model.ChannelTranslator{}) {
		return "{}"
	}
	translatorJSON, err := json.Marshal(translatorConfig)
	if err != nil {
		return "{}"
	}
	return string(translatorJSON)
}

func cloneChannels(channels []*model.Channel) []*model.Channel {
	if len(channels) == 0 {
		return nil
	}
	cloned := make([]*model.Channel, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		copyChannel := *channel
		cloned = append(cloned, &copyChannel)
	}
	return cloned
}

func invalidateEnabledChannelsCache() {
	enabledChannelsSnapshot.mu.Lock()
	defer enabledChannelsSnapshot.mu.Unlock()
	enabledChannelsSnapshot.snapshots = make(map[string]enabledChannelsSnapshotEntry)
}

func cacheKeyForChannelRepo(repo repository.ChannelRepositoryInterface) string {
	if _, ok := repo.(*repository.ChannelRepository); ok {
		return defaultChannelRepoCacheKey
	}
	value := reflect.ValueOf(repo)
	if value.IsValid() && value.Kind() == reflect.Pointer {
		return fmt.Sprintf("%T:%x", repo, value.Pointer())
	}
	return fmt.Sprintf("%T", repo)
}

type ChannelService struct {
	repo      repository.ChannelRepositoryInterface
	groupRepo repository.GroupRepositoryInterface
	rrCounter sync.Map // map[string]*atomic.Uint64
}

// NewChannelServiceWithRepo 使用指定的 repository 创建 ChannelService（用于依赖注入）
func NewChannelServiceWithRepo(repo repository.ChannelRepositoryInterface) *ChannelService {
	return &ChannelService{
		repo:      repo,
		groupRepo: repository.NewGroupRepository(),
	}
}

// NewChannelService 使用默认 repository 创建 ChannelService（便利方法）
func NewChannelService() *ChannelService {
	return NewChannelServiceWithRepo(repository.NewChannelRepository())
}

// getRRCounter 获取或创建指定 key 的原子计数器
func (s *ChannelService) getRRCounter(key string) *atomic.Uint64 {
	if v, ok := s.rrCounter.Load(key); ok {
		return v.(*atomic.Uint64)
	}
	counter := &atomic.Uint64{}
	actual, _ := s.rrCounter.LoadOrStore(key, counter)
	return actual.(*atomic.Uint64)
}

func (s *ChannelService) Create(req *model.ChannelRequest) (*model.ChannelResponse, error) {
	modelsJSON, _ := json.Marshal(req.Models)
	if req.Models == nil {
		modelsJSON = []byte("[]")
	}
	headersJSON, _ := json.Marshal(req.Headers)
	if req.Headers == nil {
		headersJSON = []byte("{}")
	}
	translatorJSON := marshalChannelTranslator(req.Translator)

	weight := req.Weight
	if weight < 1 {
		weight = 1
	}
	priority := req.Priority
	if priority < 1 {
		priority = 100
	}
	rateMultiplier := req.RateMultiplier
	if rateMultiplier <= 0 {
		rateMultiplier = 1
	}

	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint = s.defaultEndpointForType(req.Type)
	}

	channel := &model.Channel{
		Type:                  req.Type,
		Endpoint:              endpoint,
		Name:                  req.Name,
		BaseURL:               strings.TrimSuffix(req.BaseURL, "/"),
		APIKey:                req.APIKey,
		Enabled:               req.Enabled,
		Weight:                weight,
		Priority:              priority,
		RateMultiplier:        rateMultiplier,
		ModelWhitelist:        req.ModelWhitelist,
		SimulateCLI:           req.SimulateCLI,
		SimulateUA:            req.SimulateUA,
		SimulateSystemPrompt:  req.SimulateSystemPrompt,
		TraditionalChinese:    req.TraditionalChinese,
		CopilotAPI:            req.CopilotAPI,
		CodexWebsocketEnabled: req.CodexWebsocketEnabled,
		ModelsJSON:            string(modelsJSON),
		HeadersJSON:           string(headersJSON),
		TranslatorJSON:        translatorJSON,
	}

	if err := s.repo.Create(channel); err != nil {
		return nil, err
	}
	invalidateEnabledChannelsCache()

	if len(req.GroupIDs) > 0 {
		_ = s.repo.SetGroups(channel.ID, req.GroupIDs)
	}

	return s.toResponse(channel), nil
}

func (s *ChannelService) defaultEndpointForType(channelType model.ChannelType) model.ChannelEndpoint {
	switch channelType {
	case model.ChannelTypeOpenAI:
		return model.ChannelEndpointChatCompletions
	case model.ChannelTypeClaude:
		return model.ChannelEndpointMessages
	case model.ChannelTypeGemini:
		return model.ChannelEndpointGenerateContent
	default:
		return model.ChannelEndpointChatCompletions
	}
}

func (s *ChannelService) GetByID(id string) (*model.ChannelResponse, error) {
	channel, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, ErrChannelNotFound
	}
	return s.toResponse(channel), nil
}

func (s *ChannelService) List() ([]*model.ChannelResponse, error) {
	channels, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	return s.toResponsesBatch(channels)
}

func (s *ChannelService) Update(id string, req *model.ChannelRequest) (*model.ChannelResponse, error) {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrChannelNotFound
	}

	modelsJSON, _ := json.Marshal(req.Models)
	if req.Models == nil {
		modelsJSON = []byte("[]")
	}
	headersJSON, _ := json.Marshal(req.Headers)
	if req.Headers == nil {
		headersJSON = []byte("{}")
	}
	translatorJSON := marshalChannelTranslator(req.Translator)

	weight := req.Weight
	if weight < 1 {
		weight = 1
	}
	priority := req.Priority
	if priority < 1 {
		priority = 100
	}
	rateMultiplier := req.RateMultiplier
	if rateMultiplier <= 0 {
		rateMultiplier = 1
	}

	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint = s.defaultEndpointForType(req.Type)
	}

	existing.Type = req.Type
	existing.Endpoint = endpoint
	existing.Name = req.Name
	existing.BaseURL = strings.TrimSuffix(req.BaseURL, "/")
	existing.Enabled = req.Enabled
	existing.Weight = weight
	existing.Priority = priority
	existing.RateMultiplier = rateMultiplier
	existing.ModelWhitelist = req.ModelWhitelist
	existing.SimulateCLI = req.SimulateCLI
	existing.SimulateUA = req.SimulateUA
	existing.SimulateSystemPrompt = req.SimulateSystemPrompt
	existing.TraditionalChinese = req.TraditionalChinese
	existing.CopilotAPI = req.CopilotAPI
	existing.CodexWebsocketEnabled = req.CodexWebsocketEnabled
	existing.ModelsJSON = string(modelsJSON)
	existing.HeadersJSON = string(headersJSON)
	existing.TranslatorJSON = translatorJSON

	if req.APIKey != "" {
		existing.APIKey = req.APIKey
	}

	if err := s.repo.Update(existing); err != nil {
		return nil, err
	}
	invalidateEnabledChannelsCache()

	_ = s.repo.SetGroups(id, req.GroupIDs)

	return s.toResponse(existing), nil
}

func (s *ChannelService) Delete(id string) error {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrChannelNotFound
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	invalidateEnabledChannelsCache()
	return nil
}

func (s *ChannelService) SetEnabled(id string, enabled bool) error {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrChannelNotFound
	}
	if err := s.repo.SetEnabled(id, enabled); err != nil {
		return err
	}
	invalidateEnabledChannelsCache()
	return nil
}

func (s *ChannelService) TestConnection(id string, req *model.TestChannelRequest) (*model.TestChannelResponse, error) {
	channel, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, ErrChannelNotFound
	}

	testPayload, err := buildChannelTestPayload(req)
	if err != nil {
		return &model.TestChannelResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	incomingFormat := channelEndpointToFormat(req.Format)
	outgoingFormat := channelNativeFormat(channel)
	if !s.channelSupportsRequestFormat(channel, incomingFormat, true) {
		return &model.TestChannelResponse{
			Success: false,
			Message: "当前渠道不支持该接口制式",
		}, nil
	}

	upstreamPayload := testPayload
	if !internaltranslator.Equivalent(incomingFormat, outgoingFormat) {
		translated, err := internaltranslator.TranslateRequest(incomingFormat, outgoingFormat, strings.TrimSpace(req.Model), testPayload, true)
		if err != nil {
			return &model.TestChannelResponse{
				Success: false,
				Message: fmt.Sprintf("转换测试请求失败: %v", err),
			}, nil
		}
		upstreamPayload = translated
	}

	targetURL, err := buildChannelTestURL(channel, strings.TrimSpace(req.Model))
	if err != nil {
		return &model.TestChannelResponse{
			Success: false,
			Message: fmt.Sprintf("构造测试地址失败: %v", err),
		}, nil
	}

	httpReq, err := http.NewRequest("POST", targetURL, bytes.NewReader(upstreamPayload))
	if err != nil {
		return &model.TestChannelResponse{
			Success: false,
			Message: fmt.Sprintf("创建请求失败: %v", err),
		}, nil
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	switch channel.Type {
	case model.ChannelTypeOpenAI:
		httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
	case model.ChannelTypeClaude:
		httpReq.Header.Set("x-api-key", channel.APIKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
		httpReq.Header.Set("Accept", "text/event-stream")
	case model.ChannelTypeGemini:
		httpReq.Header.Set("x-goog-api-key", channel.APIKey)
	}

	if headers, ok := getParsedHeaders(channel.HeadersJSON); ok {
		for key, value := range headers {
			if strings.TrimSpace(key) != "" {
				httpReq.Header.Set(key, value)
			}
		}
	}

	client := &http.Client{Timeout: 45 * time.Second}
	start := time.Now()
	resp, err := client.Do(httpReq)
	ttfb := time.Since(start).Milliseconds()

	if err != nil {
		return &model.TestChannelResponse{
			Success:   false,
			Message:   fmt.Sprintf("连接失败: %v", err),
			LatencyMs: ttfb,
			TTFBMs:    ttfb,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &model.TestChannelResponse{
			Success:    false,
			Message:    buildChannelTestFailureMessage(resp.StatusCode, string(snippet)),
			LatencyMs:  ttfb,
			TTFBMs:     ttfb,
			StatusCode: resp.StatusCode,
		}, nil
	}

	firstByte := make([]byte, 1)
	if _, err := resp.Body.Read(firstByte); err != nil {
		if errors.Is(err, io.EOF) {
			return &model.TestChannelResponse{
				Success:    false,
				Message:    "连接成功，但未收到首包",
				LatencyMs:  ttfb,
				TTFBMs:     ttfb,
				StatusCode: resp.StatusCode,
			}, nil
		}
		return &model.TestChannelResponse{
			Success:    false,
			Message:    fmt.Sprintf("读取首包失败: %v", err),
			LatencyMs:  ttfb,
			TTFBMs:     ttfb,
			StatusCode: resp.StatusCode,
		}, nil
	}

	return &model.TestChannelResponse{
		Success:    true,
		Message:    fmt.Sprintf("已收到首包并断开 (HTTP %d)", resp.StatusCode),
		LatencyMs:  ttfb,
		TTFBMs:     ttfb,
		StatusCode: resp.StatusCode,
	}, nil
}

func channelEndpointToFormat(endpoint model.ChannelEndpoint) internaltranslator.Format {
	switch endpoint {
	case model.ChannelEndpointResponses:
		return internaltranslator.FormatOpenAIResponses
	case model.ChannelEndpointMessages:
		return internaltranslator.FormatClaude
	case model.ChannelEndpointGenerateContent:
		return internaltranslator.FormatGemini
	default:
		return internaltranslator.FormatOpenAIChat
	}
}

func buildChannelTestPayload(req *model.TestChannelRequest) ([]byte, error) {
	modelName := strings.TrimSpace(req.Model)
	prompt := strings.TrimSpace(req.Prompt)
	instructions := strings.TrimSpace(req.Instructions)
	if modelName == "" || prompt == "" || instructions == "" {
		return nil, errors.New("模型、提示词和 instructions 不能为空")
	}

	switch req.Format {
	case model.ChannelEndpointResponses:
		payload := map[string]any{
			"model":        modelName,
			"stream":       true,
			"instructions": instructions,
			"input": []map[string]any{{
				"role": "user",
				"content": []map[string]any{{
					"type": "input_text",
					"text": prompt,
				}},
			}},
		}
		if effort := strings.TrimSpace(req.ThinkingEffort); effort != "" {
			payload["reasoning"] = map[string]any{"effort": effort}
		}
		return json.Marshal(payload)
	case model.ChannelEndpointMessages:
		payload := map[string]any{
			"model":       modelName,
			"stream":      true,
			"system":      instructions,
			"max_tokens":  64,
			"messages":    []map[string]any{{"role": "user", "content": prompt}},
			"instructions": instructions,
		}
		if effort := strings.TrimSpace(req.ThinkingEffort); effort != "" {
			if budget, ok := util.ThinkingEffortToBudget(modelName, effort); ok {
				payload["thinking"] = map[string]any{
					"type":          "enabled",
					"budget_tokens": budget,
				}
			}
		}
		return json.Marshal(payload)
	case model.ChannelEndpointGenerateContent:
		payload := map[string]any{
			"contents": []map[string]any{{
				"role": "user",
				"parts": []map[string]any{{"text": prompt}},
			}},
			"system_instruction": map[string]any{
				"parts": []map[string]any{{"text": instructions}},
			},
			"instructions": instructions,
		}
		return json.Marshal(payload)
	default:
		payload := map[string]any{
			"model":  modelName,
			"stream": true,
			"messages": []map[string]any{
				{"role": "system", "content": instructions},
				{"role": "user", "content": prompt},
			},
			"instructions": instructions,
		}
		if effort := strings.TrimSpace(req.ThinkingEffort); effort != "" {
			payload["reasoning_effort"] = effort
		}
		return json.Marshal(payload)
	}
}

func buildChannelTestURL(channel *model.Channel, modelName string) (string, error) {
	baseURL := strings.TrimSuffix(channel.BaseURL, "/")
	switch channel.Type {
	case model.ChannelTypeClaude:
		return baseURL + "/v1/messages?beta=true", nil
	case model.ChannelTypeGemini:
		action := "streamGenerateContent"
		trimmedModel := strings.TrimPrefix(strings.TrimSpace(modelName), "models/")
		if trimmedModel == "" {
			return "", errors.New("Gemini 测试需要模型名称")
		}
		return fmt.Sprintf("%s/v1beta/models/%s:%s?alt=sse&key=%s", baseURL, trimmedModel, action, channel.APIKey), nil
	case model.ChannelTypeOpenAI:
		if channel.Endpoint == model.ChannelEndpointResponses {
			return baseURL + "/v1/responses", nil
		}
		return baseURL + "/v1/chat/completions", nil
	default:
		return baseURL, nil
	}
}

func buildChannelTestFailureMessage(statusCode int, body string) string {
	snippet := strings.TrimSpace(body)
	if snippet == "" {
		return fmt.Sprintf("请求失败: HTTP %d", statusCode)
	}
	if len(snippet) > 160 {
		snippet = snippet[:160]
	}
	return fmt.Sprintf("请求失败: HTTP %d - %s", statusCode, snippet)
}

func (s *ChannelService) SelectChannelForModel(modelName string) (*model.Channel, error) {
	return s.SelectChannelForModelAndFormat(modelName, "", true)
}

func (s *ChannelService) SelectChannelForModelAndFormat(modelName string, incomingFormat internaltranslator.Format, allowTranslation bool) (*model.Channel, error) {
	channels, err := s.listEnabledChannels()
	if err != nil {
		return nil, err
	}

	var candidates []*model.Channel
	for _, ch := range channels {
		if s.channelMatchesModel(ch, modelName) && s.channelSupportsRequestFormat(ch, incomingFormat, allowTranslation) {
			candidates = append(candidates, ch)
		}
	}

	return s.selectCandidate(modelName, candidates), nil
}

// SelectSpecificChannelForModelWithGroups validates and returns a user-selected channel.
// The channel must be enabled, match the requested model, satisfy the format policy,
// and be accessible to the user's groups.
func (s *ChannelService) SelectSpecificChannelForModelWithGroups(channelID, modelName string, groupIDs []string) (*model.Channel, error) {
	return s.SelectSpecificChannelForModelWithGroupsAndFormat(channelID, modelName, groupIDs, "", true)
}

func (s *ChannelService) SelectSpecificChannelForModelWithGroupsAndFormat(channelID, modelName string, groupIDs []string, incomingFormat internaltranslator.Format, allowTranslation bool) (*model.Channel, error) {
	if channelID == "" {
		return nil, nil
	}

	channels, channelGroupMap, err := s.listEnabledChannelsWithGroups()
	if err != nil {
		return nil, err
	}

	var channel *model.Channel
	for _, candidate := range channels {
		if candidate != nil && candidate.ID == channelID {
			channel = candidate
			break
		}
	}
	if channel == nil {
		return nil, nil
	}
	if !s.channelMatchesModel(channel, modelName) || !s.channelSupportsRequestFormat(channel, incomingFormat, allowTranslation) {
		return nil, nil
	}

	channelGroupIDs := channelGroupMap[channelID]
	if !channelAccessibleForGroups(channelGroupIDs, groupIDs) {
		return nil, nil
	}

	return channel, nil
}

// SelectChannelForModelWithGroups 根据分组过滤选择渠道
// 无分组用户: 只能使用未关联分组的渠道
// 有分组用户: 可以使用其分组渠道 + 未关联分组的渠道
func (s *ChannelService) SelectChannelForModelWithGroups(modelName string, groupIDs []string) (*model.Channel, error) {
	return s.SelectChannelForModelWithGroupsAndFormat(modelName, groupIDs, "", true)
}

func (s *ChannelService) SelectChannelForModelWithGroupsAndFormat(modelName string, groupIDs []string, incomingFormat internaltranslator.Format, allowTranslation bool) (*model.Channel, error) {
	channels, channelGroupMap, err := s.listEnabledChannelsWithGroups()
	if err != nil {
		return nil, err
	}

	userGroupIDSet := toStringSet(groupIDs)
	var candidates []*model.Channel
	for _, ch := range channels {
		if !s.channelMatchesModel(ch, modelName) || !s.channelSupportsRequestFormat(ch, incomingFormat, allowTranslation) {
			continue
		}
		if channelAccessibleWithSet(channelGroupMap[ch.ID], userGroupIDSet) {
			candidates = append(candidates, ch)
		}
	}

	return s.selectCandidate(modelName, candidates), nil
}

func (s *ChannelService) selectCandidate(modelName string, candidates []*model.Channel) *model.Channel {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	minPriority := candidates[0].Priority
	for _, candidate := range candidates {
		if candidate.Priority < minPriority {
			minPriority = candidate.Priority
		}
	}

	priorityCandidates := make([]*model.Channel, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Priority == minPriority {
			priorityCandidates = append(priorityCandidates, candidate)
		}
	}

	sort.Slice(priorityCandidates, func(i, j int) bool {
		return priorityCandidates[i].ID < priorityCandidates[j].ID
	})

	counter := s.getRRCounter(modelName)
	idx := int(counter.Add(1) - 1)
	return priorityCandidates[idx%len(priorityCandidates)]
}

func (s *ChannelService) listEnabledChannels() ([]*model.Channel, error) {
	cacheKey := cacheKeyForChannelRepo(s.repo)
	enabledChannelsSnapshot.mu.RLock()
	if snapshot, ok := enabledChannelsSnapshot.snapshots[cacheKey]; ok && time.Since(snapshot.loadedAt) < enabledChannelsSnapshot.cacheTTL && len(snapshot.channels) > 0 {
		channels := cloneChannels(snapshot.channels)
		enabledChannelsSnapshot.mu.RUnlock()
		return channels, nil
	}
	enabledChannelsSnapshot.mu.RUnlock()

	channels, err := s.repo.ListEnabled()
	if err != nil {
		return nil, err
	}

	cloned := cloneChannels(channels)
	enabledChannelsSnapshot.mu.Lock()
	enabledChannelsSnapshot.snapshots[cacheKey] = enabledChannelsSnapshotEntry{
		channels:        cloneChannels(channels),
		channelGroupMap: nil,
		loadedAt:        time.Now(),
	}
	enabledChannelsSnapshot.mu.Unlock()
	return cloned, nil
}

func cloneChannelGroupMap(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(source))
	for key, values := range source {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func (s *ChannelService) listEnabledChannelsWithGroups() ([]*model.Channel, map[string][]string, error) {
	cacheKey := cacheKeyForChannelRepo(s.repo)
	enabledChannelsSnapshot.mu.RLock()
	if snapshot, ok := enabledChannelsSnapshot.snapshots[cacheKey]; ok && time.Since(snapshot.loadedAt) < enabledChannelsSnapshot.cacheTTL && len(snapshot.channels) > 0 && snapshot.channelGroupMap != nil {
		channels := cloneChannels(snapshot.channels)
		groupMap := cloneChannelGroupMap(snapshot.channelGroupMap)
		enabledChannelsSnapshot.mu.RUnlock()
		return channels, groupMap, nil
	}
	enabledChannelsSnapshot.mu.RUnlock()

	channels, err := s.repo.ListEnabled()
	if err != nil {
		return nil, nil, err
	}

	channelIDs := make([]string, 0, len(channels))
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		channelIDs = append(channelIDs, ch.ID)
	}

	channelGroupMap, batchErr := s.repo.GetGroupIDsByChannelIDs(channelIDs)
	fallbackToSingleLookup := batchErr != nil
	if fallbackToSingleLookup {
		channelGroupMap = make(map[string][]string, len(channelIDs))
		for _, channelID := range channelIDs {
			gids, err := s.repo.GetGroupIDs(channelID)
			if err != nil {
				continue
			}
			channelGroupMap[channelID] = gids
		}
	}

	clonedChannels := cloneChannels(channels)
	clonedGroups := cloneChannelGroupMap(channelGroupMap)
	enabledChannelsSnapshot.mu.Lock()
	enabledChannelsSnapshot.snapshots[cacheKey] = enabledChannelsSnapshotEntry{
		channels:        cloneChannels(channels),
		channelGroupMap: cloneChannelGroupMap(channelGroupMap),
		loadedAt:        time.Now(),
	}
	enabledChannelsSnapshot.mu.Unlock()
	return clonedChannels, clonedGroups, nil
}
func toStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func hasAnyInSet(set map[string]struct{}, values []string) bool {
	for _, value := range values {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}

func channelAccessibleForGroups(channelGroupIDs, userGroupIDs []string) bool {
	return channelAccessibleWithSet(channelGroupIDs, toStringSet(userGroupIDs))
}

func channelAccessibleWithSet(channelGroupIDs []string, userGroupIDSet map[string]struct{}) bool {
	if len(channelGroupIDs) == 0 {
		return true
	}
	return len(userGroupIDSet) > 0 && hasAnyInSet(userGroupIDSet, channelGroupIDs)
}

func channelNativeFormat(channel *model.Channel) internaltranslator.Format {
	if channel == nil {
		return internaltranslator.FormatOpenAIChat
	}
	switch channel.Type {
	case model.ChannelTypeOpenAI:
		if channel.Endpoint == model.ChannelEndpointResponses {
			return internaltranslator.FormatOpenAIResponses
		}
		return internaltranslator.FormatOpenAIChat
	case model.ChannelTypeClaude:
		return internaltranslator.FormatClaude
	case model.ChannelTypeGemini:
		return internaltranslator.FormatGemini
	default:
		return internaltranslator.FormatOpenAIChat
	}
}

func translatorConfigAllowsFormat(cfg model.ChannelTranslator, incomingFormat internaltranslator.Format) bool {
	switch {
	case internaltranslator.Equivalent(incomingFormat, internaltranslator.FormatOpenAIChat):
		return cfg.Compatible
	case internaltranslator.Equivalent(incomingFormat, internaltranslator.FormatOpenAIResponses):
		return cfg.Responses
	case internaltranslator.Equivalent(incomingFormat, internaltranslator.FormatClaude):
		return cfg.Messages
	case internaltranslator.Equivalent(incomingFormat, internaltranslator.FormatGemini):
		return cfg.Gemini
	default:
		return false
	}
}

func (s *ChannelService) channelSupportsRequestFormat(channel *model.Channel, incomingFormat internaltranslator.Format, allowTranslation bool) bool {
	if strings.TrimSpace(incomingFormat.String()) == "" {
		return true
	}

	nativeFormat := channelNativeFormat(channel)
	if internaltranslator.Equivalent(incomingFormat, nativeFormat) {
		return true
	}
	if !allowTranslation {
		return false
	}

	translatorConfig, valid := getParsedChannelTranslator(channel.TranslatorJSON)
	if !valid || !translatorConfigAllowsFormat(translatorConfig, incomingFormat) {
		return false
	}
	return internaltranslator.SupportsTranslation(incomingFormat, nativeFormat)
}

func (s *ChannelService) channelMatchesModel(channel *model.Channel, modelName string) bool {
	rules, valid := getCompiledChannelModelRules(channel.ModelsJSON)
	if !valid {
		return false
	}

	if len(rules) == 0 {
		return s.defaultModelMatch(channel.Type, modelName)
	}

	modelLower := strings.ToLower(modelName)
	for _, rule := range rules {
		if strings.EqualFold(rule.name, modelName) || (rule.alias != "" && strings.EqualFold(rule.alias, modelName)) {
			return true
		}
		if rule.wildcardName && s.wildcardMatch(rule.nameLower, modelLower) {
			return true
		}
	}

	return false
}

func (s *ChannelService) defaultModelMatch(channelType model.ChannelType, modelName string) bool {
	modelLower := strings.ToLower(modelName)
	switch channelType {
	case model.ChannelTypeGemini:
		return strings.HasPrefix(modelLower, "gemini")
	case model.ChannelTypeClaude:
		return strings.HasPrefix(modelLower, "claude")
	case model.ChannelTypeOpenAI:
		return strings.HasPrefix(modelLower, "gpt") || strings.HasPrefix(modelLower, "o1") || strings.HasPrefix(modelLower, "o3") || strings.HasPrefix(modelLower, "o4")
	}
	return false
}

func (s *ChannelService) wildcardMatch(pattern, text string) bool {
	return wildcardMatch(pattern, text)
}

func (s *ChannelService) GetChannelInternal(id string) (*model.Channel, error) {
	return s.repo.GetByID(id)
}

func (s *ChannelService) toResponse(channel *model.Channel) *model.ChannelResponse {
	groupIDs := []string{}
	if gids, err := s.repo.GetGroupIDs(channel.ID); err == nil {
		groupIDs = gids
	}

	groupMap := make(map[string]*model.Group)
	if len(groupIDs) > 0 {
		if groups, err := s.groupRepo.GetByIDs(groupIDs); err == nil {
			groupMap = groups
		}
	}

	return s.buildResponse(channel, groupIDs, groupMap)
}

func (s *ChannelService) toResponsesBatch(channels []*model.Channel) ([]*model.ChannelResponse, error) {
	if len(channels) == 0 {
		return []*model.ChannelResponse{}, nil
	}

	channelIDs := make([]string, len(channels))
	for i, ch := range channels {
		channelIDs[i] = ch.ID
	}

	channelGroupMap, err := s.repo.GetGroupIDsByChannelIDs(channelIDs)
	if err != nil {
		channelGroupMap = make(map[string][]string, len(channels))
		for _, ch := range channels {
			if gids, singleLookupErr := s.repo.GetGroupIDs(ch.ID); singleLookupErr == nil {
				channelGroupMap[ch.ID] = gids
			}
		}
	}

	groupIDSet := make(map[string]struct{})
	for _, gids := range channelGroupMap {
		for _, gid := range gids {
			groupIDSet[gid] = struct{}{}
		}
	}
	uniqueGroupIDs := make([]string, 0, len(groupIDSet))
	for gid := range groupIDSet {
		uniqueGroupIDs = append(uniqueGroupIDs, gid)
	}

	groupMap := make(map[string]*model.Group)
	if len(uniqueGroupIDs) > 0 {
		groupMap, err = s.groupRepo.GetByIDs(uniqueGroupIDs)
		if err != nil {
			groupMap = make(map[string]*model.Group)
		}
	}

	responses := make([]*model.ChannelResponse, len(channels))
	for i, ch := range channels {
		responses[i] = s.buildResponse(ch, channelGroupMap[ch.ID], groupMap)
	}
	return responses, nil
}

func (s *ChannelService) buildResponse(channel *model.Channel, gids []string, groupMap map[string]*model.Group) *model.ChannelResponse {
	models, validModels := getParsedModels(channel.ModelsJSON)
	if !validModels || models == nil {
		models = []model.ChannelModel{}
	}
	models = append([]model.ChannelModel(nil), models...)

	headers, validHeaders := getParsedHeaders(channel.HeadersJSON)
	if !validHeaders || headers == nil {
		headers = map[string]string{}
	}
	clonedHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		clonedHeaders[k] = v
	}

	translatorConfig, validTranslator := getParsedChannelTranslator(channel.TranslatorJSON)
	if !validTranslator {
		translatorConfig = model.ChannelTranslator{}
	}

	groupIDs := []string{}
	groupNames := []string{}
	if len(gids) > 0 {
		groupIDs = gids
		for _, gid := range gids {
			if g, ok := groupMap[gid]; ok && g != nil {
				groupNames = append(groupNames, g.Name)
			}
		}
	}

	return &model.ChannelResponse{
		ID:                    channel.ID,
		Type:                  channel.Type,
		Endpoint:              channel.Endpoint,
		Name:                  channel.Name,
		BaseURL:               channel.BaseURL,
		APIKeySet:             channel.APIKey != "",
		Enabled:               channel.Enabled,
		Weight:                channel.Weight,
		Priority:              channel.Priority,
		RateMultiplier:        channel.RateMultiplier,
		ModelWhitelist:        channel.ModelWhitelist,
		SimulateCLI:           channel.SimulateCLI,
		SimulateUA:            channel.SimulateUA,
		SimulateSystemPrompt:  channel.SimulateSystemPrompt,
		TraditionalChinese:    channel.TraditionalChinese,
		CopilotAPI:            channel.CopilotAPI,
		CodexWebsocketEnabled: channel.CodexWebsocketEnabled,
		GroupIDs:              groupIDs,
		GroupNames:            groupNames,
		Models:                models,
		Headers:               clonedHeaders,
		Translator:            translatorConfig,
		CreatedAt:             channel.CreatedAt,
		UpdatedAt:             channel.UpdatedAt,
	}
}
