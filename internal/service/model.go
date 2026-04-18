package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	log "github.com/sirupsen/logrus"
)

type ModelService struct {
	channelRepo      *repository.ChannelRepository
	channelModelRepo *repository.ChannelModelRepository
	modelMetaRepo    *repository.ModelMetadataRepository
	userRepo         *repository.UserRepository
}

func NewModelService() *ModelService {
	return &ModelService{
		channelRepo:      repository.NewChannelRepository(),
		channelModelRepo: repository.NewChannelModelRepository(),
		modelMetaRepo:    repository.NewModelMetadataRepository(),
		userRepo:         repository.NewUserRepository(),
	}
}

func (s *ModelService) FetchAndSaveModels(channelID string) (int, error) {
	channel, err := s.channelRepo.GetByID(channelID)
	if err != nil {
		return 0, err
	}
	if channel == nil {
		return 0, fmt.Errorf("渠道不存在")
	}

	models, err := s.fetchModelsFromProvider(channel)
	if err != nil {
		return 0, err
	}
	if len(models) == 0 {
		return 0, fmt.Errorf("未获取到任何模型")
	}

	channelModels := make([]model.ChannelModel2, len(models))
	for i, m := range models {
		channelModels[i] = model.ChannelModel2{
			ChannelID:   channelID,
			ModelID:     m.ID,
			DisplayName: m.DisplayName,
		}
	}

	if err := s.channelModelRepo.ReplaceModels(channelID, channelModels); err != nil {
		return 0, err
	}

	return len(channelModels), nil
}

type fetchedModel struct {
	ID          string
	DisplayName string
}

func (s *ModelService) fetchModelsFromProvider(channel *model.Channel) ([]fetchedModel, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	var (
		url string
		req *http.Request
		err error
	)

	switch channel.Type {
	case model.ChannelTypeOpenAI:
		url = strings.TrimSuffix(channel.BaseURL, "/") + "/v1/models"
		req, err = http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+channel.APIKey)

	case model.ChannelTypeClaude:
		url = strings.TrimSuffix(channel.BaseURL, "/") + "/v1/models"
		req, err = http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", channel.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")

	case model.ChannelTypeGemini:
		url = strings.TrimSuffix(channel.BaseURL, "/") + "/v1beta/models?key=" + channel.APIKey
		req, err = http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-goog-api-key", channel.APIKey)

	default:
		return nil, fmt.Errorf("不支持的渠道类型: %s", channel.Type)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("请求失败 HTTP %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return s.parseModelsResponse(channel.Type, bodyBytes)
}

func (s *ModelService) parseModelsResponse(channelType model.ChannelType, body []byte) ([]fetchedModel, error) {
	switch channelType {
	case model.ChannelTypeOpenAI:
		var resp struct {
			Data []struct {
				ID      string `json:"id"`
				Object  string `json:"object"`
				Created int64  `json:"created"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}
		models := make([]fetchedModel, len(resp.Data))
		for i, m := range resp.Data {
			models[i] = fetchedModel{ID: m.ID, DisplayName: m.ID}
		}
		return models, nil

	case model.ChannelTypeClaude:
		var resp struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
				Type        string `json:"type"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}
		models := make([]fetchedModel, len(resp.Data))
		for i, m := range resp.Data {
			displayName := m.DisplayName
			if displayName == "" {
				displayName = m.ID
			}
			models[i] = fetchedModel{ID: m.ID, DisplayName: displayName}
		}
		return models, nil

	case model.ChannelTypeGemini:
		var resp struct {
			Models []struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
			} `json:"models"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}
		models := make([]fetchedModel, len(resp.Models))
		for i, m := range resp.Models {
			modelID := strings.TrimPrefix(m.Name, "models/")
			displayName := m.DisplayName
			if displayName == "" {
				displayName = modelID
			}
			models[i] = fetchedModel{ID: modelID, DisplayName: displayName}
		}
		return models, nil

	default:
		return nil, fmt.Errorf("不支持的渠道类型")
	}
}
func (s *ModelService) GetModelsByChannelID(channelID string) ([]*model.ChannelModel2, error) {
	return s.channelModelRepo.GetByChannelID(channelID)
}

func (s *ModelService) ListAllAvailableModels() ([]*model.AvailableModel, error) {
	all, err := s.channelModelRepo.ListAllWithChannel()
	if err != nil {
		return nil, err
	}

	var result []*model.AvailableModel
	for _, m := range all {
		if m.ModelWhitelist && !modelMatchesChannelRules(m.ModelID, m.ModelsJSON) {
			continue
		}
		result = append(result, m)
	}

	s.enrichAvailableModels(result)
	return result, nil
}

func (s *ModelService) ListAvailableModelsForUser(userID string, isAdmin bool) ([]*model.AvailableModel, error) {
	all, err := s.ListAllAvailableModels()
	if err != nil {
		return all, err
	}

	userGroupIDs, err := s.userRepo.GetGroupIDs(userID)
	if err != nil {
		return nil, err
	}

	channelIDs := make([]string, 0, len(all))
	seenChannelIDs := make(map[string]struct{}, len(all))
	for _, availableModel := range all {
		if _, ok := seenChannelIDs[availableModel.ChannelID]; ok {
			continue
		}
		seenChannelIDs[availableModel.ChannelID] = struct{}{}
		channelIDs = append(channelIDs, availableModel.ChannelID)
	}

	channelGroupMap, err := s.channelRepo.GetGroupBindingsByChannelIDs(channelIDs)
	if err != nil {
		return nil, err
	}

	userGroupSet := toStringSet(userGroupIDs)
	result := make([]*model.AvailableModel, 0, len(all))
	for _, availableModel := range all {
		if evaluateChannelGroupAccess(channelGroupMap[availableModel.ChannelID], userGroupSet).Allowed {
			result = append(result, availableModel)
		}
	}

	return result, nil
}

// modelMatchesChannelRules checks if a model ID matches the channel's model rules (supports * wildcard)
func modelMatchesChannelRules(modelID string, modelsJSON string) bool {
	return MatchChannelModelRules(modelID, modelsJSON)
}

func (s *ModelService) FetchAllChannelsModels() (map[string]int, error) {
	channels, err := s.channelRepo.ListEnabled()
	if err != nil {
		return nil, err
	}

	results := make(map[string]int)
	for _, ch := range channels {
		count, err := s.FetchAndSaveModels(ch.ID)
		if err != nil {
			log.Warnf("获取渠道 %s 模型失败: %v", ch.Name, err)
			results[ch.Name] = -1
		} else {
			results[ch.Name] = count
		}
	}

	return results, nil
}

var builtinAvailableModelMetadata = map[string]availableModelMetadata{
	"claude-4":      {contextLength: 200000, maxCompletionTokens: 64000},
	"claude-3":      {contextLength: 200000, maxCompletionTokens: 8192},
	"claude-sonnet": {contextLength: 200000, maxCompletionTokens: 64000},
	"claude-opus":   {contextLength: 200000, maxCompletionTokens: 64000},
	"claude-haiku":  {contextLength: 200000, maxCompletionTokens: 64000},
	"gpt-5":         {contextLength: 400000, maxCompletionTokens: 128000},
	"gpt-5.1":       {contextLength: 400000, maxCompletionTokens: 128000},
	"gpt-5.2":       {contextLength: 400000, maxCompletionTokens: 128000},
	"gpt-5-codex":   {contextLength: 400000, maxCompletionTokens: 128000},
	"gpt-4":         {contextLength: 128000, maxCompletionTokens: 16384},
	"gpt-4o":        {contextLength: 128000, maxCompletionTokens: 16384},
	"gpt-4.1":       {contextLength: 1047576, maxCompletionTokens: 32768},
	"gemini-2.5":    {contextLength: 1048576, maxCompletionTokens: 65536},
	"gemini-3":      {contextLength: 1048576, maxCompletionTokens: 65536},
	"deepseek-v3":   {contextLength: 128000, maxCompletionTokens: 8192},
	"deepseek-r1":   {contextLength: 128000, maxCompletionTokens: 8192},
	"qwen3":         {contextLength: 32768, maxCompletionTokens: 8192},
	"qwen3-coder":   {contextLength: 32768, maxCompletionTokens: 8192},
}

type availableModelMetadata struct {
	contextLength       int
	maxCompletionTokens int
}

func (s *ModelService) enrichAvailableModels(models []*model.AvailableModel) {
	if len(models) == 0 {
		return
	}

	dbMetadata, err := s.modelMetaRepo.List()
	if err != nil {
		dbMetadata = nil
	}

	for _, availableModel := range models {
		if availableModel == nil {
			continue
		}

		metadata := resolveAvailableModelMetadata(availableModel.ModelID, dbMetadata)
		if metadata == nil {
			continue
		}

		contextLength := metadata.contextLength
		maxCompletionTokens := metadata.maxCompletionTokens
		availableModel.ContextLength = &contextLength
		availableModel.MaxTokens = &maxCompletionTokens
	}
}

func resolveAvailableModelMetadata(modelID string, dbMetadata []*model.ModelMetadata) *availableModelMetadata {
	if strings.TrimSpace(modelID) == "" {
		return nil
	}

	if metadata := findDatabaseModelMetadata(modelID, dbMetadata); metadata != nil {
		return metadata
	}

	if metadata := findBuiltinModelMetadata(modelID); metadata != nil {
		return metadata
	}

	return nil
}

func findDatabaseModelMetadata(modelID string, dbMetadata []*model.ModelMetadata) *availableModelMetadata {
	if len(dbMetadata) == 0 {
		return nil
	}

	ordered := slices.Clone(dbMetadata)
	slices.SortFunc(ordered, func(left, right *model.ModelMetadata) int {
		return len(strings.TrimSpace(right.ModelPattern)) - len(strings.TrimSpace(left.ModelPattern))
	})

	for _, item := range ordered {
		if item == nil {
			continue
		}
		pattern := strings.TrimSpace(item.ModelPattern)
		if pattern == "" {
			continue
		}
		if modelID == pattern || strings.HasPrefix(modelID, pattern) {
			return &availableModelMetadata{
				contextLength:       item.ContextLength,
				maxCompletionTokens: item.MaxCompletionTokens,
			}
		}
	}

	return nil
}

func findBuiltinModelMetadata(modelID string) *availableModelMetadata {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	if normalized == "" {
		return nil
	}

	if exact, ok := builtinAvailableModelMetadata[normalized]; ok {
		return &exact
	}

	for prefix, metadata := range builtinAvailableModelMetadata {
		if strings.HasPrefix(normalized, prefix) {
			return &metadata
		}
	}

	for knownModel, metadata := range builtinAvailableModelMetadata {
		if strings.Contains(knownModel, normalized) {
			return &metadata
		}
	}

	return nil
}
