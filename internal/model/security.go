package model

import "time"

const (
	ManagementAPIKeyAuthMethodBearer  = "bearer"
	ManagementAPIKeyAuthMethodXAPIKey = "x-api-key"
)

type AdminManagementKey struct {
	UserID                string
	KeyHash               string
	KeyCiphertext         string
	KeyPrefix             string
	PreviousKeyHash       string
	PreviousKeyCiphertext string
	PreviousKeyPrefix     string
	PreviousValidUntil    *time.Time
	Enabled               bool
	CreatedAt             time.Time
	RotatedAt             *time.Time
	LastUsedAt            *time.Time
	LastAuthMethod        string
	UpdatedAt             time.Time
}

type ManagementAPIKeyStatus struct {
	KeyExists       bool       `json:"keyExists"`
	Enabled         bool       `json:"enabled"`
	Prefix          string     `json:"prefix"`
	CreatedAt       *time.Time `json:"createdAt,omitempty"`
	RotatedAt       *time.Time `json:"rotatedAt,omitempty"`
	LastUsedAt      *time.Time `json:"lastUsedAt,omitempty"`
	LastAuthMethod  string     `json:"lastAuthMethod,omitempty"`
	GraceUntil      *time.Time `json:"graceUntil,omitempty"`
	EncryptionReady bool       `json:"encryptionReady"`
}

type ManagementAPIKeyRevealResponse struct {
	APIKey     string     `json:"apiKey"`
	Prefix     string     `json:"prefix"`
	GraceUntil *time.Time `json:"graceUntil,omitempty"`
}

type ManagementAPIKeyPasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
}

type ManagementAPIKeyEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type UserPanelRateLimitSectionConfig struct {
	RPS       float64 `json:"rps"`
	Burst     int     `json:"burst"`
	MaxWaitMs int     `json:"maxWaitMs"`
}

type UserPanelRateLimitSections struct {
	OverviewStatus   UserPanelRateLimitSectionConfig `json:"overviewStatus"`
	AmpSettings      UserPanelRateLimitSectionConfig `json:"ampSettings"`
	APIKeys          UserPanelRateLimitSectionConfig `json:"apiKeys"`
	RequestLogsUsage UserPanelRateLimitSectionConfig `json:"requestLogsUsage"`
	Models           UserPanelRateLimitSectionConfig `json:"models"`
	AccountPurchase  UserPanelRateLimitSectionConfig `json:"accountPurchase"`
}

type UserPanelRateLimitConfig struct {
	Enabled  bool                       `json:"enabled"`
	Backend  string                     `json:"backend"`
	FailOpen bool                       `json:"failOpen"`
	Sections UserPanelRateLimitSections `json:"sections"`
}

type UserPanelRateLimitConfigRequest struct {
	Enabled  bool                       `json:"enabled"`
	Sections UserPanelRateLimitSections `json:"sections"`
}
