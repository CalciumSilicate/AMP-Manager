package amp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type fakeChannelRepo struct {
	channels map[string]*model.Channel
	groups   map[string][]string
}

func (r *fakeChannelRepo) Create(channel *model.Channel) error { return nil }

func (r *fakeChannelRepo) GetByID(id string) (*model.Channel, error) {
	channel, ok := r.channels[id]
	if !ok {
		return nil, nil
	}
	clone := *channel
	return &clone, nil
}

func (r *fakeChannelRepo) List() ([]*model.Channel, error) {
	return r.listChannels(false), nil
}

func (r *fakeChannelRepo) ListEnabled() ([]*model.Channel, error) {
	return r.listChannels(true), nil
}

func (r *fakeChannelRepo) Update(channel *model.Channel) error { return nil }

func (r *fakeChannelRepo) Delete(id string) error { return nil }

func (r *fakeChannelRepo) SetEnabled(id string, enabled bool) error {
	if channel, ok := r.channels[id]; ok {
		channel.Enabled = enabled
	}
	return nil
}

func (r *fakeChannelRepo) SetGroups(id string, groupIDs []string) error {
	r.groups[id] = append([]string(nil), groupIDs...)
	return nil
}

func (r *fakeChannelRepo) GetGroupIDs(channelID string) ([]string, error) {
	groupIDs := r.groups[channelID]
	return append([]string(nil), groupIDs...), nil
}

func (r *fakeChannelRepo) GetGroupIDsByChannelIDs(channelIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(channelIDs))
	for _, channelID := range channelIDs {
		result[channelID] = append([]string(nil), r.groups[channelID]...)
	}
	return result, nil
}

func (r *fakeChannelRepo) listChannels(enabledOnly bool) []*model.Channel {
	ids := make([]string, 0, len(r.channels))
	for id := range r.channels {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	channels := make([]*model.Channel, 0, len(ids))
	for _, id := range ids {
		channel := r.channels[id]
		if enabledOnly && !channel.Enabled {
			continue
		}
		clone := *channel
		channels = append(channels, &clone)
	}
	return channels
}

func testChannel(id, name string) *model.Channel {
	modelsJSON, _ := json.Marshal([]model.ChannelModel{{Name: "gpt-4o"}})
	return &model.Channel{
		ID:         id,
		Name:       name,
		Type:       model.ChannelTypeOpenAI,
		Endpoint:   model.ChannelEndpointChatCompletions,
		BaseURL:    "https://example.com",
		Enabled:    true,
		Priority:   1,
		Weight:     1,
		ModelsJSON: string(modelsJSON),
	}
}

func TestApplyModelMappingMiddleware_BindsPreferredChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeChannelRepo{
		channels: map[string]*model.Channel{
			"preferred": testChannel("preferred", "Preferred"),
		},
		groups: map[string][]string{},
	}

	originalService := mappingChannelService
	mappingChannelService = service.NewChannelServiceWithRepo(repo)
	defer func() { mappingChannelService = originalService }()

	mappingsJSON, err := json.Marshal([]model.ModelMapping{{
		From:      "gpt-4.1",
		To:        "gpt-4o",
		ChannelID: "preferred",
	}})
	if err != nil {
		t.Fatalf("marshal mappings: %v", err)
	}

	var mappedModel string
	var preferredChannel string

	router := gin.New()
	router.Use(func(c *gin.Context) {
		cfg := &ProxyConfig{ModelMappingsJSON: string(mappingsJSON)}
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), cfg))
		c.Next()
	})
	router.Use(ApplyModelMappingMiddleware())
	router.POST("/", func(c *gin.Context) {
		body, readErr := io.ReadAll(c.Request.Body)
		if readErr != nil {
			t.Fatalf("read mapped body: %v", readErr)
		}
		var payload struct {
			Model string `json:"model"`
		}
		if unmarshalErr := json.Unmarshal(body, &payload); unmarshalErr != nil {
			t.Fatalf("unmarshal mapped body: %v", unmarshalErr)
		}
		mappedModel = payload.Model
		preferredChannel = GetPreferredChannelID(c)
		c.Status(204)
	})

	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"model":"gpt-4.1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if mappedModel != "gpt-4o" {
		t.Fatalf("expected mapped model gpt-4o, got %q", mappedModel)
	}
	if preferredChannel != "preferred" {
		t.Fatalf("expected preferred channel preferred, got %q", preferredChannel)
	}
}

func TestApplyModelMappingMiddleware_LargeBodyAvoidsEagerJSONParseForSimpleRewrite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeChannelRepo{
		channels: map[string]*model.Channel{
			"preferred": testChannel("preferred", "Preferred"),
		},
		groups: map[string][]string{},
	}

	originalService := mappingChannelService
	mappingChannelService = service.NewChannelServiceWithRepo(repo)
	defer func() { mappingChannelService = originalService }()

	mappingsJSON, err := json.Marshal([]model.ModelMapping{{
		From: "gpt-4.1",
		To:   "gpt-4o",
	}})
	if err != nil {
		t.Fatalf("marshal mappings: %v", err)
	}

	largeText := strings.Repeat("hello world ", largeEstimateBodyThresholdBytes/12+32)
	body := fmt.Sprintf(`{"model":"gpt-4.1","input":[{"role":"user","content":"%s"}]}`, largeText)

	var payload *RequestPayload
	var mappedModel string

	router := gin.New()
	router.Use(func(c *gin.Context) {
		cfg := &ProxyConfig{ModelMappingsJSON: string(mappingsJSON)}
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), cfg))
		c.Next()
	})
	router.Use(ApplyModelMappingMiddleware())
	router.POST("/", func(c *gin.Context) {
		payload = GetRequestPayload(c.Request.Context())
		raw, readErr := io.ReadAll(c.Request.Body)
		if readErr != nil {
			t.Fatalf("read mapped body: %v", readErr)
		}
		var decoded struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("unmarshal mapped body: %v", err)
		}
		mappedModel = decoded.Model
		c.Status(204)
	})

	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if mappedModel != "gpt-4o" {
		t.Fatalf("expected mapped model gpt-4o, got %q", mappedModel)
	}
	if payload == nil {
		t.Fatal("expected request payload to be cached")
	}
	if payload.JSON != nil || payload.jsonParsed {
		t.Fatal("expected large-body simple rewrite to avoid eager JSON parsing")
	}
}

func TestChannelRouterMiddleware_UsesPreferredChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeChannelRepo{
		channels: map[string]*model.Channel{
			"auto":      testChannel("auto", "Auto"),
			"preferred": testChannel("preferred", "Preferred"),
		},
		groups: map[string][]string{},
	}

	originalService := channelService
	channelService = service.NewChannelServiceWithRepo(repo)
	defer func() { channelService = originalService }()

	selectedChannelID := ""

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), &ProxyConfig{}))
		c.Set(PreferredChannelContextKey, "preferred")
		c.Next()
	})
	router.Use(ChannelRouterMiddleware())
	router.POST("/", func(c *gin.Context) {
		cfg := GetChannelConfig(c)
		if cfg == nil || cfg.Channel == nil {
			t.Fatalf("expected channel config to be set")
		}
		selectedChannelID = cfg.Channel.ID
		c.Status(204)
	})

	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"model":"gpt-4o"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if selectedChannelID != "preferred" {
		t.Fatalf("expected preferred channel to be selected, got %q", selectedChannelID)
	}
}

func TestChannelRouterMiddleware_FallsBackWhenPreferredChannelIsInaccessible(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeChannelRepo{
		channels: map[string]*model.Channel{
			"public":    testChannel("public", "Public"),
			"preferred": testChannel("preferred", "Preferred"),
		},
		groups: map[string][]string{
			"preferred": {"vip"},
		},
	}

	originalService := channelService
	channelService = service.NewChannelServiceWithRepo(repo)
	defer func() { channelService = originalService }()

	selectedChannelID := ""

	router := gin.New()
	router.Use(func(c *gin.Context) {
		cfg := &ProxyConfig{GroupIDs: nil}
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), cfg))
		c.Set(PreferredChannelContextKey, "preferred")
		c.Next()
	})
	router.Use(ChannelRouterMiddleware())
	router.POST("/", func(c *gin.Context) {
		cfg := GetChannelConfig(c)
		if cfg == nil || cfg.Channel == nil {
			t.Fatalf("expected fallback channel config to be set")
		}
		selectedChannelID = cfg.Channel.ID
		c.Status(204)
	})

	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"model":"gpt-4o"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 204 {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if selectedChannelID != "public" {
		t.Fatalf("expected fallback public channel, got %q", selectedChannelID)
	}
}

func TestChannelRouterMiddleware_ReturnsServiceUnavailableWhenNoChannelSupportsModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeChannelRepo{
		channels: map[string]*model.Channel{
			"openai": testChannel("openai", "OpenAI"),
		},
		groups: map[string][]string{},
	}

	originalService := channelService
	channelService = service.NewChannelServiceWithRepo(repo)
	defer func() { channelService = originalService }()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), &ProxyConfig{}))
		c.Next()
	})
	router.Use(ChannelRouterMiddleware())
	router.POST("/", func(c *gin.Context) {
		t.Fatal("expected middleware to abort before handler")
	})

	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"model":"gpt-5.4"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), noAvailableChannelMessage) {
		t.Fatalf("expected response body to contain %q, got %s", noAvailableChannelMessage, rec.Body.String())
	}
}

func TestChannelRouterMiddleware_ReturnsServiceUnavailableWhenUserCannotAccessAnyChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeChannelRepo{
		channels: map[string]*model.Channel{
			"vip-only": testChannel("vip-only", "VIP"),
		},
		groups: map[string][]string{
			"vip-only": {"vip"},
		},
	}

	originalService := channelService
	channelService = service.NewChannelServiceWithRepo(repo)
	defer func() { channelService = originalService }()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithProxyConfig(c.Request.Context(), &ProxyConfig{GroupIDs: []string{"basic"}}))
		c.Next()
	})
	router.Use(ChannelRouterMiddleware())
	router.POST("/", func(c *gin.Context) {
		t.Fatal("expected middleware to abort before handler")
	})

	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"model":"gpt-4o"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), noAvailableChannelMessage) {
		t.Fatalf("expected response body to contain %q, got %s", noAvailableChannelMessage, rec.Body.String())
	}
}
