package service

import (
	"path/filepath"
	"slices"
	"testing"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

func TestListAvailableModelsForUserFiltersByChannelGroups(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ampmanager-test.db")
	if err := database.InitWithOptions(database.Options{
		Type:       database.DBTypeSQLite,
		SQLitePath: dbPath,
	}); err != nil {
		t.Fatalf("init database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.CloseAndRelease()
	})

	userRepo := repository.NewUserRepository()
	groupRepo := repository.NewGroupRepository()
	channelRepo := repository.NewChannelRepository()
	channelModelRepo := repository.NewChannelModelRepository()

	groupA := createTestGroup(t, groupRepo, "group-a")
	groupB := createTestGroup(t, groupRepo, "group-b")
	commonGroup := createTestGroup(t, groupRepo, "group-common")

	adminUser := createTestUser(t, userRepo, "admin", true)
	noGroupUser := createTestUser(t, userRepo, "user-no-group", false)
	groupAUser := createTestUser(t, userRepo, "user-group-a", false)

	if err := userRepo.SetGroups(adminUser.ID, []string{groupA.ID, groupB.ID, commonGroup.ID}); err != nil {
		t.Fatalf("set admin groups: %v", err)
	}
	if err := userRepo.SetGroups(groupAUser.ID, []string{groupA.ID, commonGroup.ID}); err != nil {
		t.Fatalf("set group-a user groups: %v", err)
	}

	publicChannel := createTestChannel(t, channelRepo, "public-channel")
	groupAChannel := createTestChannel(t, channelRepo, "group-a-channel")
	groupBChannel := createTestChannel(t, channelRepo, "group-b-channel")

	if err := channelRepo.SetGroups(publicChannel.ID, []string{commonGroup.ID}); err != nil {
		t.Fatalf("set public channel groups: %v", err)
	}
	if err := channelRepo.SetGroups(groupAChannel.ID, []string{groupA.ID}); err != nil {
		t.Fatalf("set channel groups: %v", err)
	}
	if err := channelRepo.SetGroups(groupBChannel.ID, []string{groupB.ID}); err != nil {
		t.Fatalf("set channel groups: %v", err)
	}

	if err := channelModelRepo.ReplaceModels(publicChannel.ID, []model.ChannelModel2{{ModelID: "public-model", DisplayName: "Public Model"}}); err != nil {
		t.Fatalf("insert public model: %v", err)
	}
	if err := channelModelRepo.ReplaceModels(groupAChannel.ID, []model.ChannelModel2{{ModelID: "group-a-model", DisplayName: "Group A Model"}}); err != nil {
		t.Fatalf("insert group-a model: %v", err)
	}
	if err := channelModelRepo.ReplaceModels(groupBChannel.ID, []model.ChannelModel2{{ModelID: "group-b-model", DisplayName: "Group B Model"}}); err != nil {
		t.Fatalf("insert group-b model: %v", err)
	}

	service := NewModelService()

	adminModels, err := service.ListAvailableModelsForUser(adminUser.ID, adminUser.IsAdmin)
	if err != nil {
		t.Fatalf("list admin models: %v", err)
	}
	assertModelIDs(t, adminModels, []string{"group-a-model", "group-b-model", "public-model"})

	noGroupModels, err := service.ListAvailableModelsForUser(noGroupUser.ID, noGroupUser.IsAdmin)
	if err != nil {
		t.Fatalf("list no-group models: %v", err)
	}
	assertModelIDs(t, noGroupModels, []string{})

	groupAModels, err := service.ListAvailableModelsForUser(groupAUser.ID, groupAUser.IsAdmin)
	if err != nil {
		t.Fatalf("list group-a models: %v", err)
	}
	assertModelIDs(t, groupAModels, []string{"group-a-model", "public-model"})
}

func TestListAvailableModelsForUserIncludesModelMetadata(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ampmanager-model-metadata.db")
	if err := database.InitWithOptions(database.Options{
		Type:       database.DBTypeSQLite,
		SQLitePath: dbPath,
	}); err != nil {
		t.Fatalf("init database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.CloseAndRelease()
	})

	userRepo := repository.NewUserRepository()
	groupRepo := repository.NewGroupRepository()
	channelRepo := repository.NewChannelRepository()
	channelModelRepo := repository.NewChannelModelRepository()
	modelMetadataRepo := repository.NewModelMetadataRepository()

	user := createTestUser(t, userRepo, "metadata-user", false)
	channel := createTestChannel(t, channelRepo, "metadata-channel")
	group := createTestGroup(t, groupRepo, "metadata-group")
	if err := userRepo.SetGroups(user.ID, []string{group.ID}); err != nil {
		t.Fatalf("set metadata user groups: %v", err)
	}
	if err := channelRepo.SetGroups(channel.ID, []string{group.ID}); err != nil {
		t.Fatalf("set metadata channel groups: %v", err)
	}

	if err := channelModelRepo.ReplaceModels(channel.ID, []model.ChannelModel2{
		{ModelID: "gpt-5.2", DisplayName: "GPT 5.2"},
		{ModelID: "claude-sonnet-4.5", DisplayName: "Claude Sonnet 4.5"},
	}); err != nil {
		t.Fatalf("insert channel models: %v", err)
	}

	if err := modelMetadataRepo.Create(&model.ModelMetadata{
		ModelPattern:        "claude-sonnet",
		DisplayName:         "Claude Sonnet",
		ContextLength:       222222,
		MaxCompletionTokens: 33333,
		Provider:            "anthropic",
	}); err != nil {
		t.Fatalf("create model metadata: %v", err)
	}

	service := NewModelService()
	models, err := service.ListAvailableModelsForUser(user.ID, user.IsAdmin)
	if err != nil {
		t.Fatalf("list models: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	byID := make(map[string]*model.AvailableModel, len(models))
	for _, availableModel := range models {
		byID[availableModel.ModelID] = availableModel
	}

	gpt := byID["gpt-5.2"]
	if gpt == nil || gpt.ContextLength == nil || *gpt.ContextLength != 400000 || gpt.MaxTokens == nil || *gpt.MaxTokens != 128000 {
		t.Fatalf("unexpected builtin metadata for gpt-5.2: %+v", gpt)
	}

	claude := byID["claude-sonnet-4.5"]
	if claude == nil || claude.ContextLength == nil || *claude.ContextLength != 222222 || claude.MaxTokens == nil || *claude.MaxTokens != 33333 {
		t.Fatalf("unexpected db metadata for claude-sonnet-4.5: %+v", claude)
	}
}

func createTestUser(t *testing.T, userRepo *repository.UserRepository, username string, isAdmin bool) *model.User {
	t.Helper()

	user := &model.User{
		Username:      username,
		PasswordHash:  "hash",
		IsAdmin:       isAdmin,
		BalanceMicros: 0,
	}
	if err := userRepo.Create(user); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return user
}

func createTestGroup(t *testing.T, groupRepo *repository.GroupRepository, name string) *model.Group {
	t.Helper()

	group := &model.Group{
		Name:           name,
		Description:    name,
		RateMultiplier: 1,
	}
	if err := groupRepo.Create(group); err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	return group
}

func createTestChannel(t *testing.T, channelRepo *repository.ChannelRepository, name string) *model.Channel {
	t.Helper()

	channel := &model.Channel{
		Type:           model.ChannelTypeOpenAI,
		Endpoint:       model.ChannelEndpointChatCompletions,
		Name:           name,
		BaseURL:        "https://example.com",
		APIKey:         "key",
		Enabled:        true,
		Weight:         1,
		Priority:       1,
		ModelWhitelist: false,
		ModelsJSON:     "[]",
		HeadersJSON:    "{}",
	}
	if err := channelRepo.Create(channel); err != nil {
		t.Fatalf("create channel %s: %v", name, err)
	}
	return channel
}

func assertModelIDs(t *testing.T, models []*model.AvailableModel, want []string) {
	t.Helper()

	got := make([]string, 0, len(models))
	for _, availableModel := range models {
		got = append(got, availableModel.ModelID)
	}

	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("model ids mismatch: got %v want %v", got, want)
	}
}
