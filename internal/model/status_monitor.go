package model

import "time"

type StatusMonitorTargetType string

const (
	StatusMonitorTargetTypeServiceProxy  StatusMonitorTargetType = "service_proxy"
	StatusMonitorTargetTypeChannelDirect StatusMonitorTargetType = "channel_direct"
	StatusMonitorTargetTypeCustomHTTP    StatusMonitorTargetType = "custom_http"
)

type StatusMonitorState string

const (
	StatusMonitorStateOperational StatusMonitorState = "operational"
	StatusMonitorStateDegraded    StatusMonitorState = "degraded"
	StatusMonitorStateError       StatusMonitorState = "error"
	StatusMonitorStateFailed      StatusMonitorState = "failed"
	StatusMonitorStateUnknown     StatusMonitorState = "unknown"
)

type StatusMonitor struct {
	ID                      string                  `json:"id"`
	Name                    string                  `json:"name"`
	GroupName               string                  `json:"groupName"`
	TargetType              StatusMonitorTargetType `json:"targetType"`
	Enabled                 bool                    `json:"enabled"`
	SortOrder               int                     `json:"sortOrder"`
	TimeoutMs               int64                   `json:"timeoutMs"`
	DegradedThresholdMs     int64                   `json:"degradedThresholdMs"`
	RequestFormat           ChannelEndpoint         `json:"requestFormat"`
	Model                   string                  `json:"model"`
	ChannelID               string                  `json:"channelId"`
	URL                     string                  `json:"url"`
	Method                  string                  `json:"method"`
	HeadersJSON             string                  `json:"-"`
	BodyTemplate            string                  `json:"-"`
	ExpectedStatusCodesJSON string                  `json:"-"`
	ExpectedSubstring       string                  `json:"expectedSubstring"`
	CreatedAt               time.Time               `json:"createdAt"`
	UpdatedAt               time.Time               `json:"updatedAt"`
}

type StatusMonitorRequest struct {
	Name                string                  `json:"name"`
	GroupName           string                  `json:"groupName"`
	TargetType          StatusMonitorTargetType `json:"targetType"`
	Enabled             bool                    `json:"enabled"`
	SortOrder           int                     `json:"sortOrder"`
	TimeoutMs           int64                   `json:"timeoutMs"`
	DegradedThresholdMs int64                   `json:"degradedThresholdMs"`
	RequestFormat       ChannelEndpoint         `json:"requestFormat"`
	Model               string                  `json:"model"`
	ChannelID           string                  `json:"channelId"`
	URL                 string                  `json:"url"`
	Method              string                  `json:"method"`
	HeadersJSON         string                  `json:"headersJson,omitempty"`
	RetainHeadersJSON   bool                    `json:"retainHeadersJson,omitempty"`
	ClearHeadersJSON    bool                    `json:"clearHeadersJson,omitempty"`
	BodyTemplate        string                  `json:"bodyTemplate,omitempty"`
	RetainBodyTemplate  bool                    `json:"retainBodyTemplate,omitempty"`
	ClearBodyTemplate   bool                    `json:"clearBodyTemplate,omitempty"`
	ExpectedStatusCodes []int                   `json:"expectedStatusCodes"`
	ExpectedSubstring   string                  `json:"expectedSubstring"`
}

type StatusMonitorResponse struct {
	ID                  string                  `json:"id"`
	Name                string                  `json:"name"`
	GroupName           string                  `json:"groupName"`
	TargetType          StatusMonitorTargetType `json:"targetType"`
	Enabled             bool                    `json:"enabled"`
	SortOrder           int                     `json:"sortOrder"`
	TimeoutMs           int64                   `json:"timeoutMs"`
	DegradedThresholdMs int64                   `json:"degradedThresholdMs"`
	RequestFormat       ChannelEndpoint         `json:"requestFormat"`
	Model               string                  `json:"model"`
	ChannelID           string                  `json:"channelId"`
	ChannelName         string                  `json:"channelName"`
	URL                 string                  `json:"url"`
	Method              string                  `json:"method"`
	HeadersSet          bool                    `json:"headersSet"`
	HeadersMasked       string                  `json:"headersMasked,omitempty"`
	BodyTemplateSet     bool                    `json:"bodyTemplateSet"`
	BodyTemplateMasked  string                  `json:"bodyTemplateMasked,omitempty"`
	ExpectedStatusCodes []int                   `json:"expectedStatusCodes"`
	ExpectedSubstring   string                  `json:"expectedSubstring"`
	CreatedAt           time.Time               `json:"createdAt"`
	UpdatedAt           time.Time               `json:"updatedAt"`
}

type StatusMonitorRuntimeConfig struct {
	Enabled              bool   `json:"enabled"`
	ServiceBaseURL       string `json:"serviceBaseUrl"`
	ServiceMonitorAPIKey string `json:"-"`
	PollIntervalSec      int    `json:"pollIntervalSec"`
	DefaultTimeoutMs     int64  `json:"defaultTimeoutMs"`
	RetentionDays        int    `json:"retentionDays"`
}

type StatusMonitorRuntimeConfigRequest struct {
	Enabled                    bool   `json:"enabled"`
	ServiceBaseURL             string `json:"serviceBaseUrl"`
	ServiceMonitorAPIKey       string `json:"serviceMonitorApiKey,omitempty"`
	RetainServiceMonitorAPIKey bool   `json:"retainServiceMonitorApiKey,omitempty"`
	ClearServiceMonitorAPIKey  bool   `json:"clearServiceMonitorApiKey,omitempty"`
	PollIntervalSec            int    `json:"pollIntervalSec"`
	DefaultTimeoutMs           int64  `json:"defaultTimeoutMs"`
	RetentionDays              int    `json:"retentionDays"`
}

type StatusMonitorRuntimeConfigResponse struct {
	Enabled                    bool   `json:"enabled"`
	ServiceBaseURL             string `json:"serviceBaseUrl"`
	ServiceMonitorAPIKeySet    bool   `json:"serviceMonitorApiKeySet"`
	ServiceMonitorAPIKeyMasked string `json:"serviceMonitorApiKeyMasked,omitempty"`
	PollIntervalSec            int    `json:"pollIntervalSec"`
	DefaultTimeoutMs           int64  `json:"defaultTimeoutMs"`
	RetentionDays              int    `json:"retentionDays"`
}

type StatusMonitorResult struct {
	ID             string             `json:"id"`
	MonitorID      string             `json:"monitorId"`
	Status         StatusMonitorState `json:"status"`
	LatencyMs      int64              `json:"latencyMs"`
	TTFBMs         int64              `json:"ttfbMs"`
	HTTPStatusCode int                `json:"httpStatusCode"`
	Message        string             `json:"message"`
	EndpointLabel  string             `json:"endpointLabel"`
	CheckedAt      time.Time          `json:"checkedAt"`
	CreatedAt      time.Time          `json:"createdAt"`
}

type StatusMonitorLatestResultResponse struct {
	Status         StatusMonitorState `json:"status"`
	LatencyMs      int64              `json:"latencyMs"`
	TTFBMs         int64              `json:"ttfbMs"`
	HTTPStatusCode int                `json:"httpStatusCode"`
	Message        string             `json:"message"`
	CheckedAt      *time.Time         `json:"checkedAt,omitempty"`
}

type StatusMonitorAvailabilityResponse struct {
	TotalChecks      int     `json:"totalChecks"`
	OperationalCount int     `json:"operationalCount"`
	AvailabilityPct  float64 `json:"availabilityPct"`
}

type StatusMonitorHistoryPointResponse struct {
	Status    StatusMonitorState `json:"status"`
	TTFBMs    int64              `json:"ttfbMs"`
	CheckedAt time.Time          `json:"checkedAt"`
}

type StatusMonitorDashboardItemResponse struct {
	ID            string                              `json:"id"`
	Name          string                              `json:"name"`
	TargetType    StatusMonitorTargetType             `json:"targetType"`
	RequestFormat ChannelEndpoint                     `json:"requestFormat"`
	Model         string                              `json:"model"`
	ChannelName   string                              `json:"channelName,omitempty"`
	EndpointLabel string                              `json:"endpointLabel"`
	Latest        StatusMonitorLatestResultResponse   `json:"latest"`
	Availability  StatusMonitorAvailabilityResponse   `json:"availability"`
	History       []StatusMonitorHistoryPointResponse `json:"history"`
}

type StatusMonitorDashboardGroupResponse struct {
	GroupName string                               `json:"groupName"`
	Items     []StatusMonitorDashboardItemResponse `json:"items"`
}

type StatusMonitorSummaryCountsResponse struct {
	Total       int `json:"total"`
	Operational int `json:"operational"`
	Degraded    int `json:"degraded"`
	Error       int `json:"error"`
	Failed      int `json:"failed"`
	Unknown     int `json:"unknown"`
}

type StatusMonitorDashboardResponse struct {
	Period          string                                `json:"period"`
	GeneratedAt     time.Time                             `json:"generatedAt"`
	LastUpdated     *time.Time                            `json:"lastUpdated,omitempty"`
	PollIntervalSec int                                   `json:"pollIntervalSec"`
	OverallStatus   StatusMonitorState                    `json:"overallStatus"`
	SummaryCounts   StatusMonitorSummaryCountsResponse    `json:"summaryCounts"`
	Groups          []StatusMonitorDashboardGroupResponse `json:"groups"`
}
