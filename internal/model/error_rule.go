package model

type ErrorRuleRequestType string

const (
	ErrorRuleRequestTypeResponses       ErrorRuleRequestType = "responses"
	ErrorRuleRequestTypeChatCompletions ErrorRuleRequestType = "chat_completions"
	ErrorRuleRequestTypeGemini          ErrorRuleRequestType = "gemini"
	ErrorRuleRequestTypeAnthropic       ErrorRuleRequestType = "messages"
)

type ErrorRuleMatchMode string

const (
	ErrorRuleMatchModeSubstring ErrorRuleMatchMode = "substring"
	ErrorRuleMatchModeRegex     ErrorRuleMatchMode = "regex"
)

type ErrorRule struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	BuiltIn         bool                 `json:"builtIn"`
	Enabled         bool                 `json:"enabled"`
	RequestType     ErrorRuleRequestType `json:"requestType"`
	UpstreamStatus  string               `json:"upstreamStatus"`
	Pattern         string               `json:"pattern"`
	MatchMode       ErrorRuleMatchMode   `json:"matchMode"`
	OverrideStatus  int                  `json:"overrideStatus"`
	OverrideMessage string               `json:"overrideMessage"`
}

type ErrorRuleListResponse struct {
	Rules []ErrorRule `json:"rules"`
}

type ErrorRuleListRequest struct {
	Rules []ErrorRule `json:"rules"`
}
