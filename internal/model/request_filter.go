package model

import (
	"encoding/json"
	"time"
)

type RequestFilterScope string

const (
	RequestFilterScopeHeader RequestFilterScope = "header"
	RequestFilterScopeBody   RequestFilterScope = "body"
)

type RequestFilterAction string

const (
	RequestFilterActionRemove      RequestFilterAction = "remove"
	RequestFilterActionSet         RequestFilterAction = "set"
	RequestFilterActionJSONPath    RequestFilterAction = "json_path"
	RequestFilterActionTextReplace RequestFilterAction = "text_replace"
)

type RequestFilterMatchType string

const (
	RequestFilterMatchTypeRegex    RequestFilterMatchType = "regex"
	RequestFilterMatchTypeContains RequestFilterMatchType = "contains"
	RequestFilterMatchTypeExact    RequestFilterMatchType = "exact"
)

type RequestFilterBindingType string

const (
	RequestFilterBindingTypeGlobal   RequestFilterBindingType = "global"
	RequestFilterBindingTypeChannels RequestFilterBindingType = "channels"
	RequestFilterBindingTypeGroups   RequestFilterBindingType = "groups"
)

type RequestFilterRuleMode string

const (
	RequestFilterRuleModeSimple   RequestFilterRuleMode = "simple"
	RequestFilterRuleModeAdvanced RequestFilterRuleMode = "advanced"
)

type RequestFilterExecutionPhase string

const (
	RequestFilterExecutionPhaseGuard RequestFilterExecutionPhase = "guard"
	RequestFilterExecutionPhaseFinal RequestFilterExecutionPhase = "final"
)

type RequestFilterWriteMode string

const (
	RequestFilterWriteModeOverwrite RequestFilterWriteMode = "overwrite"
	RequestFilterWriteModeIfMissing RequestFilterWriteMode = "if_missing"
)

type RequestFilterInsertPosition string

const (
	RequestFilterInsertPositionStart  RequestFilterInsertPosition = "start"
	RequestFilterInsertPositionEnd    RequestFilterInsertPosition = "end"
	RequestFilterInsertPositionBefore RequestFilterInsertPosition = "before"
	RequestFilterInsertPositionAfter  RequestFilterInsertPosition = "after"
)

type RequestFilterOnAnchorMissing string

const (
	RequestFilterOnAnchorMissingSkip   RequestFilterOnAnchorMissing = "skip"
	RequestFilterOnAnchorMissingAppend RequestFilterOnAnchorMissing = "append"
	RequestFilterOnAnchorMissingPrepend RequestFilterOnAnchorMissing = "prepend"
)

type RequestFilterMatcher struct {
	Field     string                  `json:"field"`
	MatchType RequestFilterMatchType  `json:"matchType"`
	Value     json.RawMessage         `json:"value"`
}

type RequestFilterDedupe struct {
	ByFields []string `json:"byFields"`
}

type RequestFilterOperation struct {
	Type            string                        `json:"type"`
	Scope           RequestFilterScope            `json:"scope"`
	Path            string                        `json:"path"`
	Value           json.RawMessage               `json:"value,omitempty"`
	WriteMode       RequestFilterWriteMode        `json:"writeMode,omitempty"`
	Matcher         *RequestFilterMatcher         `json:"matcher,omitempty"`
	Position        RequestFilterInsertPosition   `json:"position,omitempty"`
	Anchor          *RequestFilterMatcher         `json:"anchor,omitempty"`
	OnAnchorMissing RequestFilterOnAnchorMissing  `json:"onAnchorMissing,omitempty"`
	Dedupe          *RequestFilterDedupe          `json:"dedupe,omitempty"`
}

type RequestFilter struct {
	ID             string                      `json:"id"`
	Name           string                      `json:"name"`
	Description    string                      `json:"description"`
	Scope          RequestFilterScope          `json:"scope"`
	Action         RequestFilterAction         `json:"action"`
	MatchType      *RequestFilterMatchType     `json:"matchType,omitempty"`
	Target         string                      `json:"target"`
	Replacement    json.RawMessage             `json:"replacement,omitempty"`
	Priority       int                         `json:"priority"`
	IsEnabled      bool                        `json:"isEnabled"`
	BindingType    RequestFilterBindingType    `json:"bindingType"`
	ChannelIDs     []string                    `json:"channelIds"`
	GroupIDs       []string                    `json:"groupIds"`
	RuleMode       RequestFilterRuleMode       `json:"ruleMode"`
	ExecutionPhase RequestFilterExecutionPhase `json:"executionPhase"`
	Operations     []RequestFilterOperation    `json:"operations"`
	CreatedAt      time.Time                   `json:"createdAt"`
	UpdatedAt      time.Time                   `json:"updatedAt"`
}

type RequestFilterListResponse struct {
	Filters []RequestFilter `json:"filters"`
}

type RequestFilterBindingsResponse struct {
	Channels []RequestFilterBindingOption `json:"channels"`
	Groups   []RequestFilterBindingOption `json:"groups"`
}

type RequestFilterBindingOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RequestFilterCreateRequest struct {
	Name           string                      `json:"name"`
	Description    string                      `json:"description"`
	Scope          RequestFilterScope          `json:"scope"`
	Action         RequestFilterAction         `json:"action"`
	MatchType      *RequestFilterMatchType     `json:"matchType,omitempty"`
	Target         string                      `json:"target"`
	Replacement    json.RawMessage             `json:"replacement,omitempty"`
	Priority       int                         `json:"priority"`
	IsEnabled      bool                        `json:"isEnabled"`
	BindingType    RequestFilterBindingType    `json:"bindingType"`
	ChannelIDs     []string                    `json:"channelIds"`
	GroupIDs       []string                    `json:"groupIds"`
	RuleMode       RequestFilterRuleMode       `json:"ruleMode"`
	ExecutionPhase RequestFilterExecutionPhase `json:"executionPhase"`
	Operations     []RequestFilterOperation    `json:"operations"`
}

type RequestFilterUpdateRequest = RequestFilterCreateRequest
