package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

var (
	ErrChannelNotFound = errors.New("渠道不存在")
)

const defaultChannelRepoCacheKey = "default-channel-repo"

// modelsCache 缓存 ModelsJSON -> []model.ChannelModel 的解析结果
// key: ModelsJSON 字符串, value: *parsedModelsEntry
var modelsCache sync.Map
var enabledChannelsSnapshot = &enabledChannelsCache{
	snapshots: make(map[string]enabledChannelsSnapshotEntry),
	cacheTTL: 2 * time.Second,
}

type parsedModelsEntry struct {
	models []model.ChannelModel
	valid  bool
}

type enabledChannelsCache struct {
	mu       sync.RWMutex
	snapshots map[string]enabledChannelsSnapshotEntry
	cacheTTL time.Duration
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

	weight := req.Weight
	if weight < 1 {
		weight = 1
	}
	priority := req.Priority
	if priority < 1 {
		priority = 100
	}

	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint = s.defaultEndpointForType(req.Type)
	}

	channel := &model.Channel{
		Type:                 req.Type,
		Endpoint:             endpoint,
		Name:                 req.Name,
		BaseURL:              strings.TrimSuffix(req.BaseURL, "/"),
		APIKey:               req.APIKey,
		Enabled:              req.Enabled,
		Weight:               weight,
		Priority:             priority,
		ModelWhitelist:       req.ModelWhitelist,
		SimulateCLI:          req.SimulateCLI,
		SimulateUA:           req.SimulateUA,
		SimulateSystemPrompt: req.SimulateSystemPrompt,
		TraditionalChinese:   req.TraditionalChinese,
		CopilotAPI:           req.CopilotAPI,
		ModelsJSON:           string(modelsJSON),
		HeadersJSON:          string(headersJSON),
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

	weight := req.Weight
	if weight < 1 {
		weight = 1
	}
	priority := req.Priority
	if priority < 1 {
		priority = 100
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
	existing.ModelWhitelist = req.ModelWhitelist
	existing.SimulateCLI = req.SimulateCLI
	existing.SimulateUA = req.SimulateUA
	existing.SimulateSystemPrompt = req.SimulateSystemPrompt
	existing.TraditionalChinese = req.TraditionalChinese
	existing.CopilotAPI = req.CopilotAPI
	existing.ModelsJSON = string(modelsJSON)
	existing.HeadersJSON = string(headersJSON)

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

func (s *ChannelService) TestConnection(id string) (*model.TestChannelResponse, error) {
	channel, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, ErrChannelNotFound
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var testURL string

	switch channel.Type {
	case model.ChannelTypeOpenAI:
		testURL = channel.BaseURL + "/v1/models"
	case model.ChannelTypeClaude:
		testURL = channel.BaseURL + "/v1/models"
	case model.ChannelTypeGemini:
		testURL = channel.BaseURL + "/v1beta/models"
	default:
		testURL = channel.BaseURL
	}

	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return &model.TestChannelResponse{
			Success: false,
			Message: fmt.Sprintf("创建请求失败: %v", err),
		}, nil
	}

	switch channel.Type {
	case model.ChannelTypeOpenAI:
		req.Header.Set("Authorization", "Bearer "+channel.APIKey)
	case model.ChannelTypeClaude:
		req.Header.Set("x-api-key", channel.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case model.ChannelTypeGemini:
		q := req.URL.Query()
		q.Set("key", channel.APIKey)
		req.URL.RawQuery = q.Encode()
		req.Header.Set("x-goog-api-key", channel.APIKey)
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return &model.TestChannelResponse{
			Success:   false,
			Message:   fmt.Sprintf("连接失败: %v", err),
			LatencyMs: latency,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &model.TestChannelResponse{
			Success:   true,
			Message:   fmt.Sprintf("连接成功 (HTTP %d)", resp.StatusCode),
			LatencyMs: latency,
		}, nil
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return &model.TestChannelResponse{
			Success:   false,
			Message:   fmt.Sprintf("认证失败 (HTTP %d)", resp.StatusCode),
			LatencyMs: latency,
		}, nil
	}

	return &model.TestChannelResponse{
		Success:   false,
		Message:   fmt.Sprintf("请求失败: HTTP %d", resp.StatusCode),
		LatencyMs: latency,
	}, nil
}

func (s *ChannelService) SelectChannelForModel(modelName string) (*model.Channel, error) {
	channels, err := s.listEnabledChannels()
	if err != nil {
		return nil, err
	}

	var candidates []*model.Channel
	for _, ch := range channels {
		if s.channelMatchesModel(ch, modelName) {
			candidates = append(candidates, ch)
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	minPriority := candidates[0].Priority
	for _, c := range candidates {
		if c.Priority < minPriority {
			minPriority = c.Priority
		}
	}

	var priorityCandidates []*model.Channel
	for _, c := range candidates {
		if c.Priority == minPriority {
			priorityCandidates = append(priorityCandidates, c)
		}
	}

	// 按 ID 排序确保稳定顺序
	sort.Slice(priorityCandidates, func(i, j int) bool {
		return priorityCandidates[i].ID < priorityCandidates[j].ID
	})

	// 使用原子计数器实现线程安全的 round-robin
	counter := s.getRRCounter(modelName)
	idx := int(counter.Add(1) - 1)
	selected := priorityCandidates[idx%len(priorityCandidates)]

	return selected, nil
}

// SelectSpecificChannelForModelWithGroups validates and returns a user-selected channel.
// The channel must be enabled, match the requested model, and be accessible to the user's groups.
func (s *ChannelService) SelectSpecificChannelForModelWithGroups(channelID, modelName string, groupIDs []string) (*model.Channel, error) {
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
	if !s.channelMatchesModel(channel, modelName) {
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
	channels, channelGroupMap, err := s.listEnabledChannelsWithGroups()
	if err != nil {
		return nil, err
	}

	// Collect IDs of model-matching channels for batch group lookup
	var matchingChannels []*model.Channel
	for _, ch := range channels {
		if s.channelMatchesModel(ch, modelName) {
			matchingChannels = append(matchingChannels, ch)
		}
	}

	if len(matchingChannels) == 0 {
		return nil, nil
	}

	userGroupIDSet := toStringSet(groupIDs)

	// Filter by group access
	var candidates []*model.Channel
	for _, ch := range matchingChannels {
		chGroupIDs := channelGroupMap[ch.ID]
		if channelAccessibleWithSet(chGroupIDs, userGroupIDSet) {
			candidates = append(candidates, ch)
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	minPriority := candidates[0].Priority
	for _, c := range candidates {
		if c.Priority < minPriority {
			minPriority = c.Priority
		}
	}

	var priorityCandidates []*model.Channel
	for _, c := range candidates {
		if c.Priority == minPriority {
			priorityCandidates = append(priorityCandidates, c)
		}
	}

	sort.Slice(priorityCandidates, func(i, j int) bool {
		return priorityCandidates[i].ID < priorityCandidates[j].ID
	})

	counter := s.getRRCounter(modelName)
	idx := int(counter.Add(1) - 1)
	selected := priorityCandidates[idx%len(priorityCandidates)]

	return selected, nil
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

func (s *ChannelService) channelMatchesModel(channel *model.Channel, modelName string) bool {
	models, valid := getParsedModels(channel.ModelsJSON)
	if !valid {
		return false
	}

	if len(models) == 0 {
		return s.defaultModelMatch(channel.Type, modelName)
	}

	modelLower := strings.ToLower(modelName)
	for _, m := range models {
		if strings.EqualFold(m.Name, modelName) || strings.EqualFold(m.Alias, modelName) {
			return true
		}
		nameLower := strings.ToLower(m.Name)
		if strings.Contains(nameLower, "*") {
			if s.wildcardMatch(nameLower, modelLower) {
				return true
			}
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
	var models []model.ChannelModel
	_ = json.Unmarshal([]byte(channel.ModelsJSON), &models)
	if models == nil {
		models = []model.ChannelModel{}
	}

	var headers map[string]string
	_ = json.Unmarshal([]byte(channel.HeadersJSON), &headers)
	if headers == nil {
		headers = map[string]string{}
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
		ID:                   channel.ID,
		Type:                 channel.Type,
		Endpoint:             channel.Endpoint,
		Name:                 channel.Name,
		BaseURL:              channel.BaseURL,
		APIKeySet:            channel.APIKey != "",
		Enabled:              channel.Enabled,
		Weight:               channel.Weight,
		Priority:             channel.Priority,
		ModelWhitelist:       channel.ModelWhitelist,
		SimulateCLI:          channel.SimulateCLI,
		SimulateUA:           channel.SimulateUA,
		SimulateSystemPrompt: channel.SimulateSystemPrompt,
		TraditionalChinese:   channel.TraditionalChinese,
		CopilotAPI:           channel.CopilotAPI,
		GroupIDs:             groupIDs,
		GroupNames:           groupNames,
		Models:               models,
		Headers:              headers,
		CreatedAt:            channel.CreatedAt,
		UpdatedAt:            channel.UpdatedAt,
	}
}
