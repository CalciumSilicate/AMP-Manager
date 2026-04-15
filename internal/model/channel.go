package model

import "time"

type ChannelType string

const (
	ChannelTypeGemini ChannelType = "gemini"
	ChannelTypeClaude ChannelType = "claude"
	ChannelTypeOpenAI ChannelType = "openai"
)

type ChannelEndpoint string

const (
	ChannelEndpointChatCompletions ChannelEndpoint = "chat_completions"
	ChannelEndpointResponses       ChannelEndpoint = "responses"
	ChannelEndpointMessages        ChannelEndpoint = "messages"
	ChannelEndpointGenerateContent ChannelEndpoint = "generate_content"
)

type ChannelTranslator struct {
	Compatible bool `json:"compatible"`
	Responses  bool `json:"responses"`
	Messages   bool `json:"messages"`
	Gemini     bool `json:"gemini"`
}

type Channel struct {
	ID                    string          `json:"id"`
	Type                  ChannelType     `json:"type"`
	Endpoint              ChannelEndpoint `json:"endpoint"`
	Name                  string          `json:"name"`
	BaseURL               string          `json:"baseUrl"`
	APIKey                string          `json:"-"`
	Enabled               bool            `json:"enabled"`
	Weight                int             `json:"weight"`
	Priority              int             `json:"priority"`
	ModelWhitelist        bool            `json:"modelWhitelist"`
	SimulateCLI           bool            `json:"simulateCli"`
	SimulateUA            bool            `json:"simulateUa"`
	SimulateSystemPrompt  bool            `json:"simulateSystemPrompt"`
	TraditionalChinese    bool            `json:"traditionalChinese"`
	CopilotAPI            bool            `json:"copilotApi"`
	CodexWebsocketEnabled bool            `json:"codexWebsocketEnabled"`
	ModelsJSON            string          `json:"-"`
	HeadersJSON           string          `json:"-"`
	TranslatorJSON        string          `json:"-"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
}

type ChannelModel struct {
	Name  string `json:"name"`
	Alias string `json:"alias,omitempty"`
}

type ChannelRequest struct {
	Type                  ChannelType       `json:"type" binding:"required,oneof=gemini claude openai"`
	Endpoint              ChannelEndpoint   `json:"endpoint"`
	Name                  string            `json:"name" binding:"required,min=1,max=64"`
	BaseURL               string            `json:"baseUrl" binding:"required,url"`
	APIKey                string            `json:"apiKey,omitempty"`
	Enabled               bool              `json:"enabled"`
	Weight                int               `json:"weight"`
	Priority              int               `json:"priority"`
	ModelWhitelist        bool              `json:"modelWhitelist"`
	SimulateCLI           bool              `json:"simulateCli"`
	SimulateUA            bool              `json:"simulateUa"`
	SimulateSystemPrompt  bool              `json:"simulateSystemPrompt"`
	TraditionalChinese    bool              `json:"traditionalChinese"`
	CopilotAPI            bool              `json:"copilotApi"`
	CodexWebsocketEnabled bool              `json:"codexWebsocketEnabled"`
	GroupIDs              []string          `json:"groupIds"`
	Models                []ChannelModel    `json:"models,omitempty"`
	Headers               map[string]string `json:"headers,omitempty"`
	Translator            ChannelTranslator `json:"translator"`
}

type ChannelResponse struct {
	ID                    string            `json:"id"`
	Type                  ChannelType       `json:"type"`
	Endpoint              ChannelEndpoint   `json:"endpoint"`
	Name                  string            `json:"name"`
	BaseURL               string            `json:"baseUrl"`
	APIKeySet             bool              `json:"apiKeySet"`
	Enabled               bool              `json:"enabled"`
	Weight                int               `json:"weight"`
	Priority              int               `json:"priority"`
	ModelWhitelist        bool              `json:"modelWhitelist"`
	SimulateCLI           bool              `json:"simulateCli"`
	SimulateUA            bool              `json:"simulateUa"`
	SimulateSystemPrompt  bool              `json:"simulateSystemPrompt"`
	TraditionalChinese    bool              `json:"traditionalChinese"`
	CopilotAPI            bool              `json:"copilotApi"`
	CodexWebsocketEnabled bool              `json:"codexWebsocketEnabled"`
	GroupIDs              []string          `json:"groupIds"`
	GroupNames            []string          `json:"groupNames"`
	Models                []ChannelModel    `json:"models"`
	Headers               map[string]string `json:"headers"`
	Translator            ChannelTranslator `json:"translator"`
	CreatedAt             time.Time         `json:"createdAt"`
	UpdatedAt             time.Time         `json:"updatedAt"`
}

type TestChannelResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
}

type ChannelModel2 struct {
	ID          string    `json:"id"`
	ChannelID   string    `json:"channelId"`
	ModelID     string    `json:"modelId"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type AvailableModel struct {
	ModelID        string      `json:"modelId"`
	DisplayName    string      `json:"displayName"`
	ChannelID      string      `json:"channelId"`
	ChannelType    ChannelType `json:"channelType"`
	ChannelName    string      `json:"channelName"`
	ModelWhitelist bool        `json:"-"`
	ModelsJSON     string      `json:"-"`
}
