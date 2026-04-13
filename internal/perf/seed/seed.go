package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"golang.org/x/crypto/bcrypt"
)

const requestDetailEnabledKey = "request_detail_enabled"

type Options struct {
	UserCount              int
	UserPrefix             string
	UserPassword           string
	AdminUsername          string
	AdminPassword          string
	BalanceMicros          int64
	UpstreamURL            string
	UpstreamAPIKey         string
	RequestDetailEnabled   bool
	PublicChatModel        string
	PublicResponsesModel   string
	UpstreamChatModel      string
	UpstreamResponsesModel string
	OutputPath             string
}

type ModelSet struct {
	Chat      string `json:"chat"`
	Responses string `json:"responses"`
}

type AdminManifest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type APIKeyManifest struct {
	Username string `json:"username"`
	APIKey   string `json:"api_key"`
}

type Manifest struct {
	GeneratedAt    time.Time        `json:"generated_at"`
	UserCount      int              `json:"user_count"`
	UpstreamURL    string           `json:"upstream_url"`
	PublicModels   ModelSet         `json:"public_models"`
	UpstreamModels ModelSet         `json:"upstream_models"`
	Admin          AdminManifest    `json:"admin"`
	APIKeys        []APIKeyManifest `json:"api_keys"`
}

type Result struct {
	UsersCreated    int
	ChannelsCreated int
	ManifestPath    string
}

type Seeder struct {
	options        Options
	userRepo       *repository.UserRepository
	settingsRepo   *repository.AmpSettingsRepository
	configRepo     *repository.SystemConfigRepository
	channelService *service.ChannelService
	ampService     *service.AmpService
}

func DefaultOptions() Options {
	return Options{
		UserCount:              5000,
		UserPrefix:             "perf-user",
		UserPassword:           "perf-user-password",
		AdminUsername:          "perf-admin",
		AdminPassword:          "perf-admin-password",
		BalanceMicros:          100_000_000,
		UpstreamURL:            "http://mock-upstream:18080",
		UpstreamAPIKey:         "perf-upstream-key",
		RequestDetailEnabled:   true,
		PublicChatModel:        "bench-chat",
		PublicResponsesModel:   "bench-responses",
		UpstreamChatModel:      "bench-chat-upstream",
		UpstreamResponsesModel: "bench-responses-upstream",
		OutputPath:             "",
	}
}

func New(options Options) *Seeder {
	return &Seeder{
		options:        sanitizeOptions(options),
		userRepo:       repository.NewUserRepository(),
		settingsRepo:   repository.NewAmpSettingsRepository(),
		configRepo:     repository.NewSystemConfigRepository(),
		channelService: service.NewChannelService(),
		ampService:     service.NewAmpService(),
	}
}

func (s *Seeder) Run() (*Result, error) {
	if err := s.ensureCleanNamespace(); err != nil {
		return nil, err
	}

	adminPasswordHash, err := bcrypt.GenerateFromPassword([]byte(s.options.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash admin password: %w", err)
	}
	userPasswordHash, err := bcrypt.GenerateFromPassword([]byte(s.options.UserPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash user password: %w", err)
	}

	if err := s.userRepo.Create(&model.User{
		Username:      s.options.AdminUsername,
		PasswordHash:  string(adminPasswordHash),
		IsAdmin:       true,
		BalanceMicros: s.options.BalanceMicros,
	}); err != nil {
		return nil, fmt.Errorf("create admin user: %w", err)
	}

	chatChannel, err := s.channelService.Create(&model.ChannelRequest{
		Type:     model.ChannelTypeOpenAI,
		Endpoint: model.ChannelEndpointChatCompletions,
		Name:     "perf-openai-chat",
		BaseURL:  s.options.UpstreamURL,
		APIKey:   s.options.UpstreamAPIKey,
		Enabled:  true,
		Weight:   1,
		Priority: 1,
		Models: []model.ChannelModel{
			{Name: s.options.UpstreamChatModel},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create chat channel: %w", err)
	}

	responsesChannel, err := s.channelService.Create(&model.ChannelRequest{
		Type:     model.ChannelTypeOpenAI,
		Endpoint: model.ChannelEndpointResponses,
		Name:     "perf-openai-responses",
		BaseURL:  s.options.UpstreamURL,
		APIKey:   s.options.UpstreamAPIKey,
		Enabled:  true,
		Weight:   1,
		Priority: 1,
		Models: []model.ChannelModel{
			{Name: s.options.UpstreamResponsesModel},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create responses channel: %w", err)
	}

	if err := s.configRepo.Set(requestDetailEnabledKey, boolString(s.options.RequestDetailEnabled)); err != nil {
		return nil, fmt.Errorf("set request detail config: %w", err)
	}

	manifest := Manifest{
		GeneratedAt: time.Now().UTC(),
		UserCount:   s.options.UserCount,
		UpstreamURL: s.options.UpstreamURL,
		PublicModels: ModelSet{
			Chat:      s.options.PublicChatModel,
			Responses: s.options.PublicResponsesModel,
		},
		UpstreamModels: ModelSet{
			Chat:      s.options.UpstreamChatModel,
			Responses: s.options.UpstreamResponsesModel,
		},
		Admin: AdminManifest{
			Username: s.options.AdminUsername,
			Password: s.options.AdminPassword,
		},
		APIKeys: make([]APIKeyManifest, 0, s.options.UserCount),
	}

	mappingsJSON, err := json.Marshal([]model.ModelMapping{
		{
			From:      s.options.PublicChatModel,
			To:        s.options.UpstreamChatModel,
			ChannelID: chatChannel.ID,
		},
		{
			From:      s.options.PublicResponsesModel,
			To:        s.options.UpstreamResponsesModel,
			ChannelID: responsesChannel.ID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal model mappings: %w", err)
	}

	for index := 1; index <= s.options.UserCount; index++ {
		username := fmt.Sprintf("%s-%05d", s.options.UserPrefix, index)
		user := &model.User{
			Username:      username,
			PasswordHash:  string(userPasswordHash),
			IsAdmin:       false,
			BalanceMicros: s.options.BalanceMicros,
		}
		if err := s.userRepo.Create(user); err != nil {
			return nil, fmt.Errorf("create user %s: %w", username, err)
		}

		if err := s.settingsRepo.Upsert(&model.AmpSettings{
			UserID:            user.ID,
			UpstreamURL:       s.options.UpstreamURL,
			UpstreamAPIKey:    s.options.UpstreamAPIKey,
			ModelMappingsJSON: string(mappingsJSON),
			Enabled:           true,
			WebSearchMode:     model.WebSearchModeUpstream,
			NativeMode:        false,
			ShowBalanceInAd:   false,
			Socks5Proxy:       "",
		}); err != nil {
			return nil, fmt.Errorf("upsert settings for %s: %w", username, err)
		}

		apiKey, err := s.ampService.CreateAPIKey(user.ID, &model.CreateAPIKeyRequest{Name: "perf-key"})
		if err != nil {
			return nil, fmt.Errorf("create api key for %s: %w", username, err)
		}

		manifest.APIKeys = append(manifest.APIKeys, APIKeyManifest{
			Username: username,
			APIKey:   apiKey.APIKey,
		})
	}

	if s.options.OutputPath != "" {
		if err := writeManifest(s.options.OutputPath, manifest); err != nil {
			return nil, err
		}
	}

	return &Result{
		UsersCreated:    s.options.UserCount,
		ChannelsCreated: 2,
		ManifestPath:    s.options.OutputPath,
	}, nil
}

func sanitizeOptions(options Options) Options {
	defaults := DefaultOptions()

	if options.UserCount < 1 {
		options.UserCount = defaults.UserCount
	}
	if options.UserPrefix == "" {
		options.UserPrefix = defaults.UserPrefix
	}
	if options.UserPassword == "" {
		options.UserPassword = defaults.UserPassword
	}
	if options.AdminUsername == "" {
		options.AdminUsername = defaults.AdminUsername
	}
	if options.AdminPassword == "" {
		options.AdminPassword = defaults.AdminPassword
	}
	if options.BalanceMicros <= 0 {
		options.BalanceMicros = defaults.BalanceMicros
	}
	if options.UpstreamURL == "" {
		options.UpstreamURL = defaults.UpstreamURL
	}
	if options.UpstreamAPIKey == "" {
		options.UpstreamAPIKey = defaults.UpstreamAPIKey
	}
	if options.PublicChatModel == "" {
		options.PublicChatModel = defaults.PublicChatModel
	}
	if options.PublicResponsesModel == "" {
		options.PublicResponsesModel = defaults.PublicResponsesModel
	}
	if options.UpstreamChatModel == "" {
		options.UpstreamChatModel = defaults.UpstreamChatModel
	}
	if options.UpstreamResponsesModel == "" {
		options.UpstreamResponsesModel = defaults.UpstreamResponsesModel
	}
	return options
}

func writeManifest(path string, manifest Manifest) error {
	directory := filepath.Dir(path)
	if directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create manifest directory: %w", err)
		}
	}

	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (s *Seeder) ensureCleanNamespace() error {
	existingAdmin, err := s.userRepo.ExistsByUsername(s.options.AdminUsername)
	if err != nil {
		return fmt.Errorf("check admin user existence: %w", err)
	}
	if existingAdmin {
		return fmt.Errorf("seed namespace already exists for admin user %q; reset the perf database before reseeding", s.options.AdminUsername)
	}

	firstPerfUser := fmt.Sprintf("%s-%05d", s.options.UserPrefix, 1)
	existingUser, err := s.userRepo.ExistsByUsername(firstPerfUser)
	if err != nil {
		return fmt.Errorf("check perf user existence: %w", err)
	}
	if existingUser {
		return fmt.Errorf("seed namespace already exists for user prefix %q; reset the perf database before reseeding", s.options.UserPrefix)
	}

	channels, err := s.channelService.List()
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}
	for _, channel := range channels {
		if strings.HasPrefix(channel.Name, "perf-openai-") {
			return fmt.Errorf("perf channels already exist; reset the perf database before reseeding")
		}
	}

	return nil
}
