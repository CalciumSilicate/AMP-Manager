package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/precision"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"
)

type manifestUser struct {
	UserID                string  `json:"user_id"`
	Username              string  `json:"username"`
	APIKey                string  `json:"api_key"`
	BalanceMicros         int64   `json:"balance_micros"`
	BalanceUsd            string  `json:"balance_usd"`
	ConcurrencyLimit      int     `json:"concurrency_limit"`
	SubscriptionPlanID    string  `json:"subscription_plan_id,omitempty"`
	SubscriptionExpiresAt *string `json:"subscription_expires_at,omitempty"`
}

type seedManifest struct {
	GeneratedAt       string         `json:"generated_at"`
	AppURL            string         `json:"app_url,omitempty"`
	UpstreamURL       string         `json:"upstream_url,omitempty"`
	UserPrefix        string         `json:"user_prefix"`
	UserCount         int            `json:"user_count"`
	DefaultPassword   string         `json:"default_password"`
	SupportedModels   []string       `json:"supported_models,omitempty"`
	SubscriptionShare float64        `json:"subscription_share"`
	Users             []manifestUser `json:"users"`
}

type assignedUserState struct {
	user             *model.User
	apiKey           string
	balance          int64
	concurrency      int
	planID           string
	subscriptionDays int
	expiresAt        *time.Time
}

func main() {
	var dbType string
	var sqlitePath string
	var databaseURL string
	var count int
	var prefix string
	var password string
	var apiKeyName string
	var manifestPath string
	var appURL string
	var upstreamURL string
	var upstreamAPIKey string
	var supportedModelsCSV string
	var balanceOptionsCSV string
	var concurrencyOptionsCSV string
	var subscriptionDayOptionsCSV string
	var subscriptionPlanIDsCSV string
	var subscriptionShare float64
	var seed int64

	flag.StringVar(&dbType, "db-type", "sqlite", "database type: sqlite or postgres")
	flag.StringVar(&sqlitePath, "sqlite-path", "./data/data.db", "sqlite database path")
	flag.StringVar(&databaseURL, "database-url", "", "postgres database url")
	flag.IntVar(&count, "count", 6000, "number of users to ensure")
	flag.StringVar(&prefix, "user-prefix", "lt-user", "username prefix")
	flag.StringVar(&password, "password", "loadtest123", "default user password")
	flag.StringVar(&apiKeyName, "api-key-name", "loadtest", "api key name")
	flag.StringVar(&manifestPath, "manifest", "perf/runtime/seed-manifest.json", "output manifest path")
	flag.StringVar(&appURL, "app-url", "", "optional amp manager base url to store in manifest")
	flag.StringVar(&upstreamURL, "upstream-url", "", "optional upstream base url to store in manifest")
	flag.StringVar(&upstreamAPIKey, "upstream-api-key", "", "optional upstream api key written into user amp settings")
	flag.StringVar(&supportedModelsCSV, "supported-models", "gpt-5.4,gpt-5.3-codex,gpt-5.2,gpt-5.4-mini", "comma-separated models to store in manifest")
	flag.StringVar(&balanceOptionsCSV, "balance-usd-options", "0,3,8,20,50", "comma-separated usd balances to assign")
	flag.StringVar(&concurrencyOptionsCSV, "concurrency-options", "1,2,4,8,16,32", "comma-separated concurrency limits to assign")
	flag.StringVar(&subscriptionDayOptionsCSV, "subscription-day-options", "30,90,180", "comma-separated subscription durations in days")
	flag.StringVar(&subscriptionPlanIDsCSV, "subscription-plan-ids", "", "optional comma-separated subscription plan ids; defaults to all enabled plans")
	flag.Float64Var(&subscriptionShare, "subscription-share", 0.75, "fraction of users that receive a subscription")
	flag.Int64Var(&seed, "seed", time.Now().UnixNano(), "random seed")
	flag.Parse()

	if count <= 0 {
		log.Fatal("count must be > 0")
	}
	if len(strings.TrimSpace(prefix)) < 3 {
		log.Fatal("user-prefix must be at least 3 characters")
	}
	if len(password) < 6 {
		log.Fatal("password must be at least 6 characters")
	}
	if subscriptionShare < 0 || subscriptionShare > 1 {
		log.Fatal("subscription-share must be between 0 and 1")
	}

	options := database.Options{
		Type:        database.DBType(strings.ToLower(strings.TrimSpace(dbType))),
		SQLitePath:  sqlitePath,
		DatabaseURL: databaseURL,
	}
	if err := database.InitWithOptions(options); err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	defer database.Close()

	userSvc := service.NewUserService()
	ampSvc := service.NewAmpService()
	planSvc := service.NewSubscriptionPlanService()
	userRepo := repository.NewUserRepository()
	settingsRepo := repository.NewAmpSettingsRepository()
	channelRepo := repository.NewChannelRepository()

	balanceOptions, err := parseUSDOptions(balanceOptionsCSV)
	if err != nil {
		log.Fatalf("parse balance-usd-options failed: %v", err)
	}
	concurrencyOptions, err := parseIntOptions(concurrencyOptionsCSV, true)
	if err != nil {
		log.Fatalf("parse concurrency-options failed: %v", err)
	}
	subscriptionDayOptions, err := parseIntOptions(subscriptionDayOptionsCSV, true)
	if err != nil {
		log.Fatalf("parse subscription-day-options failed: %v", err)
	}
	supportedModels := parseStringList(supportedModelsCSV)
	if len(supportedModels) == 0 {
		log.Fatal("supported-models cannot be empty")
	}

	if strings.TrimSpace(upstreamURL) != "" {
		if err := ensureLoadtestResponsesChannel(channelRepo, strings.TrimSpace(upstreamURL), strings.TrimSpace(upstreamAPIKey), supportedModels); err != nil {
			log.Fatalf("ensure loadtest channel failed: %v", err)
		}
	}

	enabledPlans, err := planSvc.List()
	if err != nil {
		log.Fatalf("load subscription plans failed: %v", err)
	}
	planPool := resolvePlanPool(enabledPlans, parseStringList(subscriptionPlanIDsCSV))
	if subscriptionShare > 0 && len(planPool) == 0 {
		log.Printf("no enabled subscription plans resolved; seeding will continue with balance-only users")
		subscriptionShare = 0
	}

	rng := rand.New(rand.NewSource(seed))
	userStates := make([]*assignedUserState, 0, count)
	for i := 1; i <= count; i++ {
		username := fmt.Sprintf("%s-%05d", prefix, i)
		user, err := userRepo.GetByUsername(username)
		if err != nil {
			log.Fatalf("lookup user %s failed: %v", username, err)
		}
		if user == nil {
			user, err = userSvc.Register(&model.RegisterRequest{
				Username: username,
				Password: password,
			})
			if err != nil {
				log.Fatalf("register user %s failed: %v", username, err)
			}
		}

		if strings.TrimSpace(upstreamURL) != "" {
			if err := settingsRepo.Upsert(&model.AmpSettings{
				UserID:          user.ID,
				UpstreamURL:     strings.TrimSpace(upstreamURL),
				UpstreamAPIKey:  strings.TrimSpace(upstreamAPIKey),
				Enabled:         true,
				WebSearchMode:   model.WebSearchModeUpstream,
				NativeMode:      false,
				ShowBalanceInAd: false,
			}); err != nil {
				log.Fatalf("upsert amp settings for %s failed: %v", username, err)
			}
		}

		apiKeyValue, err := ensureUserAPIKey(ampSvc, user.ID, apiKeyName)
		if err != nil {
			log.Fatalf("ensure api key for %s failed: %v", username, err)
		}

		state := &assignedUserState{
			user:        user,
			apiKey:      apiKeyValue,
			balance:     balanceOptions[rng.Intn(len(balanceOptions))],
			concurrency: concurrencyOptions[rng.Intn(len(concurrencyOptions))],
		}
		if subscriptionShare > 0 && rng.Float64() < subscriptionShare {
			plan := planPool[rng.Intn(len(planPool))]
			days := subscriptionDayOptions[rng.Intn(len(subscriptionDayOptions))]
			expiresAt := time.Now().UTC().AddDate(0, 0, days)
			state.planID = plan.ID
			state.subscriptionDays = days
			state.expiresAt = &expiresAt
		}
		userStates = append(userStates, state)

		if i%500 == 0 || i == count {
			log.Printf("prepared %d/%d users", i, count)
		}
	}

	if err := applySeededState(userSvc, userStates); err != nil {
		log.Fatalf("apply seeded state failed: %v", err)
	}

	manifest := seedManifest{
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		AppURL:            strings.TrimSpace(appURL),
		UpstreamURL:       strings.TrimSpace(upstreamURL),
		UserPrefix:        prefix,
		UserCount:         len(userStates),
		DefaultPassword:   password,
		SupportedModels:   supportedModels,
		SubscriptionShare: subscriptionShare,
		Users:             buildManifestUsers(userStates),
	}
	if err := writeManifest(manifestPath, &manifest); err != nil {
		log.Fatalf("write manifest failed: %v", err)
	}

	log.Printf("seeded users=%d manifest=%s", len(userStates), manifestPath)
}

func ensureUserAPIKey(ampSvc *service.AmpService, userID, apiKeyName string) (string, error) {
	keys, err := ampSvc.ListAPIKeysForAdmin(userID)
	if err != nil {
		return "", err
	}
	for _, key := range keys {
		if strings.TrimSpace(key.APIKey) != "" && key.Status == "active" {
			return key.APIKey, nil
		}
	}

	created, err := ampSvc.CreateAPIKey(userID, &model.CreateAPIKeyRequest{Name: apiKeyName})
	if err != nil {
		return "", err
	}
	return created.APIKey, nil
}

func applySeededState(userSvc *service.UserService, userStates []*assignedUserState) error {
	balanceGroups := make(map[int64][]string)
	concurrencyGroups := make(map[int][]string)
	type subscriptionGroupKey struct {
		planID string
		days   int
	}
	subscriptionGroups := make(map[subscriptionGroupKey][]string)
	cancelIDs := make([]string, 0)

	for _, state := range userStates {
		balanceGroups[state.balance] = append(balanceGroups[state.balance], state.user.ID)
		concurrencyGroups[state.concurrency] = append(concurrencyGroups[state.concurrency], state.user.ID)
		if state.planID == "" || state.expiresAt == nil {
			cancelIDs = append(cancelIDs, state.user.ID)
			continue
		}
		groupKey := subscriptionGroupKey{planID: state.planID, days: state.subscriptionDays}
		subscriptionGroups[groupKey] = append(subscriptionGroups[groupKey], state.user.ID)
	}

	for amountMicros, userIDs := range balanceGroups {
		if _, err := userSvc.ApplyBatchUpdate(&model.UserBatchApplyRequest{
			TargetMode:      model.UserBatchTargetSelected,
			SelectedUserIDs: userIDs,
			Changes: model.UserBatchChangeSet{
				Balance: &model.BalanceBatchChange{
					Mode:         model.BalanceBatchChangeSet,
					AmountMicros: amountMicros,
				},
			},
		}); err != nil {
			return err
		}
	}

	for concurrencyLimit, userIDs := range concurrencyGroups {
		if _, err := userSvc.ApplyBatchUpdate(&model.UserBatchApplyRequest{
			TargetMode:      model.UserBatchTargetSelected,
			SelectedUserIDs: userIDs,
			Changes: model.UserBatchChangeSet{
				ConcurrencyLimit: &model.UpdateUserConcurrencyLimitRequest{
					ConcurrencyLimit: concurrencyLimit,
				},
			},
		}); err != nil {
			return err
		}
	}

	if len(cancelIDs) > 0 {
		if _, err := userSvc.ApplyBatchUpdate(&model.UserBatchApplyRequest{
			TargetMode:      model.UserBatchTargetSelected,
			SelectedUserIDs: cancelIDs,
			Changes: model.UserBatchChangeSet{
				Subscription: &model.SubscriptionBatchChange{
					PlanMode: model.SubscriptionBatchPlanCancel,
				},
			},
		}); err != nil {
			return err
		}
	}

	for key, userIDs := range subscriptionGroups {
		expiresAt := time.Now().UTC().AddDate(0, 0, key.days)
		if _, err := userSvc.ApplyBatchUpdate(&model.UserBatchApplyRequest{
			TargetMode:      model.UserBatchTargetSelected,
			SelectedUserIDs: userIDs,
			Changes: model.UserBatchChangeSet{
				Subscription: &model.SubscriptionBatchChange{
					PlanMode:   model.SubscriptionBatchPlanAssign,
					PlanID:     key.planID,
					ExpiryMode: model.SubscriptionBatchExpirySet,
					ExpiresAt:  &expiresAt,
				},
			},
		}); err != nil {
			return err
		}
	}

	return nil
}

func buildManifestUsers(userStates []*assignedUserState) []manifestUser {
	items := make([]manifestUser, 0, len(userStates))
	for _, state := range userStates {
		var expiresAt *string
		if state.expiresAt != nil {
			value := state.expiresAt.UTC().Format(time.RFC3339)
			expiresAt = &value
		}
		items = append(items, manifestUser{
			UserID:                state.user.ID,
			Username:              state.user.Username,
			APIKey:                state.apiKey,
			BalanceMicros:         state.balance,
			BalanceUsd:            formatMicrosUSD(state.balance),
			ConcurrencyLimit:      state.concurrency,
			SubscriptionPlanID:    state.planID,
			SubscriptionExpiresAt: expiresAt,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Username < items[j].Username
	})
	return items
}

func resolvePlanPool(plans []*model.SubscriptionPlanResponse, requestedPlanIDs []string) []*model.SubscriptionPlanResponse {
	if len(requestedPlanIDs) == 0 {
		pool := make([]*model.SubscriptionPlanResponse, 0)
		for _, plan := range plans {
			if plan.Enabled {
				pool = append(pool, plan)
			}
		}
		return pool
	}

	requested := make(map[string]struct{}, len(requestedPlanIDs))
	for _, planID := range requestedPlanIDs {
		if planID = strings.TrimSpace(planID); planID != "" {
			requested[planID] = struct{}{}
		}
	}
	pool := make([]*model.SubscriptionPlanResponse, 0, len(requested))
	for _, plan := range plans {
		if _, ok := requested[plan.ID]; ok {
			pool = append(pool, plan)
		}
	}
	return pool
}

func parseUSDOptions(value string) ([]int64, error) {
	parts := parseStringList(value)
	if len(parts) == 0 {
		return nil, fmt.Errorf("no usd options provided")
	}
	values := make([]int64, 0, len(parts))
	for _, part := range parts {
		micros, err := precision.ParseUSDToMicros(precision.DecimalString(part))
		if err != nil {
			return nil, err
		}
		values = append(values, micros)
	}
	return values, nil
}

func parseIntOptions(value string, positiveOnly bool) ([]int, error) {
	parts := parseStringList(value)
	if len(parts) == 0 {
		return nil, fmt.Errorf("no int options provided")
	}
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		if positiveOnly && parsed <= 0 {
			return nil, fmt.Errorf("value %d must be > 0", parsed)
		}
		values = append(values, parsed)
	}
	return values, nil
}

func parseStringList(value string) []string {
	rawParts := strings.Split(value, ",")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func formatMicrosUSD(value int64) string {
	return fmt.Sprintf("%.6f", float64(value)/1_000_000)
}

func writeManifest(path string, manifest *seedManifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func ensureLoadtestResponsesChannel(channelRepo *repository.ChannelRepository, upstreamURL, upstreamAPIKey string, supportedModels []string) error {
	models := make([]model.ChannelModel, 0, len(supportedModels))
	for _, modelName := range supportedModels {
		models = append(models, model.ChannelModel{Name: modelName})
	}
	modelsJSON, err := json.Marshal(models)
	if err != nil {
		return err
	}

	translatorJSON, err := json.Marshal(model.ChannelTranslator{})
	if err != nil {
		return err
	}

	headersJSON, err := json.Marshal(map[string]string{})
	if err != nil {
		return err
	}

	const channelName = "lt-fake-responses"
	channels, err := channelRepo.List()
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if channel.Name != channelName {
			continue
		}
		channel.Type = model.ChannelTypeOpenAI
		channel.Endpoint = model.ChannelEndpointResponses
		channel.BaseURL = upstreamURL
		channel.APIKey = upstreamAPIKey
		channel.Enabled = true
		channel.Weight = 1
		channel.Priority = 1
		channel.RateMultiplier = 1
		channel.ModelWhitelist = false
		channel.CodexWebsocketEnabled = true
		channel.ModelsJSON = string(modelsJSON)
		channel.HeadersJSON = string(headersJSON)
		channel.TranslatorJSON = string(translatorJSON)
		return channelRepo.Update(channel)
	}

	return channelRepo.Create(&model.Channel{
		Type:                  model.ChannelTypeOpenAI,
		Endpoint:              model.ChannelEndpointResponses,
		Name:                  channelName,
		BaseURL:               upstreamURL,
		APIKey:                upstreamAPIKey,
		Enabled:               true,
		Weight:                1,
		Priority:              1,
		RateMultiplier:        1,
		CodexWebsocketEnabled: true,
		ModelsJSON:            string(modelsJSON),
		HeadersJSON:           string(headersJSON),
		TranslatorJSON:        string(translatorJSON),
	})
}
