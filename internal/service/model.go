package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	log "github.com/sirupsen/logrus"
)

type ModelService struct {
	channelRepo      *repository.ChannelRepository
	channelModelRepo *repository.ChannelModelRepository
}

func NewModelService() *ModelService {
	return &ModelService{
		channelRepo:      repository.NewChannelRepository(),
		channelModelRepo: repository.NewChannelModelRepository(),
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
	return result, nil
}

// modelMatchesChannelRules checks if a model ID matches the channel's model rules (supports * wildcard)
func modelMatchesChannelRules(modelID string, modelsJSON string) bool {
	var rules []model.ChannelModel
	if err := json.Unmarshal([]byte(modelsJSON), &rules); err != nil || len(rules) == 0 {
		return true
	}

	modelLower := strings.ToLower(modelID)
	for _, r := range rules {
		if strings.EqualFold(r.Name, modelID) || strings.EqualFold(r.Alias, modelID) {
			return true
		}
		nameLower := strings.ToLower(r.Name)
		if strings.Contains(nameLower, "*") {
			if simpleWildcardMatch(nameLower, modelLower) {
				return true
			}
		}
	}
	return false
}

func simpleWildcardMatch(pattern, text string) bool {
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
