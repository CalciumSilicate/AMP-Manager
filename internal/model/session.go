package model

type SessionStickyPublicConfig struct {
	Enabled           bool `json:"enabled"`
	LogSearchMinChars int  `json:"logSearchMinChars"`
}

type SessionStickyConfigResponse struct {
	Enabled                       bool `json:"enabled"`
	WindowMinutes                 int  `json:"windowMinutes"`
	LogSearchMinChars             int  `json:"logSearchMinChars"`
	InjectPromptCacheKeyResponses bool `json:"injectPromptCacheKeyResponses"`
}

type SessionStickyConfigRequest struct {
	Enabled                       bool `json:"enabled"`
	WindowMinutes                 int  `json:"windowMinutes"`
	LogSearchMinChars             int  `json:"logSearchMinChars"`
	InjectPromptCacheKeyResponses bool `json:"injectPromptCacheKeyResponses"`
}

type SessionStickyRuntimeResponse struct {
	RedisConfigured    bool   `json:"redisConfigured"`
	RedisURLMasked     string `json:"redisUrlMasked,omitempty"`
	RedisPrefix        string `json:"redisPrefix,omitempty"`
	RuntimeEnabled     bool   `json:"runtimeEnabled"`
	RuntimeHealthy     bool   `json:"runtimeHealthy"`
	ActiveSessionCount int64  `json:"activeSessionCount"`
}

type AdminSessionListItem struct {
	ID             string `json:"id"`
	SessionID      string `json:"sessionId"`
	UserID         string `json:"userId,omitempty"`
	Username       string `json:"username,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Active         bool   `json:"active"`
	State          string `json:"state"`
	RequestCount   int64  `json:"requestCount"`
	FirstSeenAt    string `json:"firstSeenAt"`
	LastSeenAt     string `json:"lastSeenAt"`
	LastModel      string `json:"lastModel,omitempty"`
	LastStatusCode *int   `json:"lastStatusCode,omitempty"`
}

type AdminSessionListResponse struct {
	Items    []AdminSessionListItem `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
}

type AdminSessionDetailResponse struct {
	Session             AdminSessionListItem `json:"session"`
	Timeline            []RequestLog         `json:"timeline"`
	DistinctAPIKeyCount int64                `json:"distinctApiKeyCount"`
	DistinctModelCount  int64                `json:"distinctModelCount"`
	TotalInputTokens    int64                `json:"totalInputTokens"`
	TotalOutputTokens   int64                `json:"totalOutputTokens"`
	TotalCostUsd        string               `json:"totalCostUsd"`
}

type AdminSessionLeaderboardItem struct {
	UserID               string `json:"userId,omitempty"`
	Username             string `json:"username"`
	DistinctSessionCount int64  `json:"distinctSessionCount"`
	RequestCount         int64  `json:"requestCount"`
}

type AdminSessionLeaderboardResponse struct {
	WindowMinutes int                           `json:"windowMinutes"`
	Items         []AdminSessionLeaderboardItem `json:"items"`
}
