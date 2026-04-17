package model

import (
	"encoding/json"
	"time"
)

type ErrorRuleRequestType string

const (
	ErrorRuleRequestTypeResponses       ErrorRuleRequestType = "responses"
	ErrorRuleRequestTypeChatCompletions ErrorRuleRequestType = "chat_completions"
	ErrorRuleRequestTypeGemini          ErrorRuleRequestType = "gemini"
	ErrorRuleRequestTypeAnthropic       ErrorRuleRequestType = "messages"
)

type ErrorRuleMatchType string

const (
	ErrorRuleMatchTypeContains ErrorRuleMatchType = "contains"
	ErrorRuleMatchTypeExact    ErrorRuleMatchType = "exact"
	ErrorRuleMatchTypeRegex    ErrorRuleMatchType = "regex"
)

type ErrorRule struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	Description        string               `json:"description"`
	RequestType        ErrorRuleRequestType `json:"requestType"`
	UpstreamStatus     string               `json:"upstreamStatus"`
	Pattern            string               `json:"pattern"`
	MatchType          ErrorRuleMatchType   `json:"matchType"`
	Category           string               `json:"category"`
	Priority           int                  `json:"priority"`
	IsEnabled          bool                 `json:"isEnabled"`
	IsDefault          bool                 `json:"isDefault"`
	OverrideStatusCode *int                 `json:"overrideStatusCode,omitempty"`
	OverrideMessage    string               `json:"overrideMessage"`
	OverrideResponse   json.RawMessage      `json:"overrideResponse,omitempty"`
	CreatedAt          time.Time            `json:"createdAt"`
	UpdatedAt          time.Time            `json:"updatedAt"`
}

type ErrorRuleListResponse struct {
	Rules []ErrorRule `json:"rules"`
}

type ErrorRuleCreateRequest struct {
	Name               string               `json:"name"`
	Description        string               `json:"description"`
	RequestType        ErrorRuleRequestType `json:"requestType"`
	UpstreamStatus     string               `json:"upstreamStatus"`
	Pattern            string               `json:"pattern"`
	MatchType          ErrorRuleMatchType   `json:"matchType"`
	Category           string               `json:"category"`
	Priority           int                  `json:"priority"`
	IsEnabled          bool                 `json:"isEnabled"`
	OverrideStatusCode *int                 `json:"overrideStatusCode,omitempty"`
	OverrideMessage    string               `json:"overrideMessage"`
	OverrideResponse   json.RawMessage      `json:"overrideResponse,omitempty"`
}

type ErrorRuleUpdateRequest struct {
	Name               string               `json:"name"`
	Description        string               `json:"description"`
	UpstreamStatus     string               `json:"upstreamStatus"`
	Pattern            string               `json:"pattern"`
	MatchType          ErrorRuleMatchType   `json:"matchType"`
	Category           string               `json:"category"`
	Priority           int                  `json:"priority"`
	IsEnabled          bool                 `json:"isEnabled"`
	OverrideStatusCode *int                 `json:"overrideStatusCode,omitempty"`
	OverrideMessage    string               `json:"overrideMessage"`
	OverrideResponse   json.RawMessage      `json:"overrideResponse,omitempty"`
}

type ErrorRuleTestRequest struct {
	RequestType    ErrorRuleRequestType `json:"requestType"`
	UpstreamStatus int                  `json:"upstreamStatus"`
	Body           string               `json:"body"`
}

type ErrorRuleTestResponse struct {
	Matched      bool            `json:"matched"`
	Rule         *ErrorRule      `json:"rule,omitempty"`
	StatusCode   int             `json:"statusCode"`
	ResponseBody json.RawMessage `json:"responseBody,omitempty"`
}

type ErrorRuleCacheStats struct {
	LoadedAt        *time.Time `json:"loadedAt,omitempty"`
	ContainsCount   int        `json:"containsCount"`
	ExactCount      int        `json:"exactCount"`
	RegexCount      int        `json:"regexCount"`
	TotalCount      int        `json:"totalCount"`
	Reloading       bool       `json:"reloading"`
	LastReloadError string     `json:"lastReloadError,omitempty"`
}
