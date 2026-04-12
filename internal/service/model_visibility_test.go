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

	adminUser := createTestUser(t, userRepo, "admin", true)
	noGroupUser := createTestUser(t, userRepo, "user-no-group", false)
	groupAUser := createTestUser(t, userRepo, "user-group-a", false)

	if err := userRepo.SetGroups(groupAUser.ID, []string{groupA.ID}); err != nil {
		t.Fatalf("set user groups: %v", err)
	}

	publicChannel := createTestChannel(t, channelRepo, "public-channel")
	groupAChannel := createTestChannel(t, channelRepo, "group-a-channel")
	groupBChannel := createTestChannel(t, channelRepo, "group-b-channel")

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
	assertModelIDs(t, noGroupModels, []string{"public-model"})

	groupAModels, err := service.ListAvailableModelsForUser(groupAUser.ID, groupAUser.IsAdmin)
	if err != nil {
		t.Fatalf("list group-a models: %v", err)
	}
	assertModelIDs(t, groupAModels, []string{"group-a-model", "public-model"})
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
