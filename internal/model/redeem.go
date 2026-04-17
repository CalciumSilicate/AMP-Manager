package model

import "time"

type RedeemCodeMode string

const (
	RedeemCodeModeSingleUse RedeemCodeMode = "single_use"
	RedeemCodeModeShared    RedeemCodeMode = "shared"
)

type RedeemCodeStatus string

const (
	RedeemCodeStatusActive   RedeemCodeStatus = "active"
	RedeemCodeStatusDisabled RedeemCodeStatus = "disabled"
	RedeemCodeStatusConsumed RedeemCodeStatus = "consumed"
)

type RedeemRedemptionStatus string

const (
	RedeemRedemptionStatusSuccess  RedeemRedemptionStatus = "success"
	RedeemRedemptionStatusRejected RedeemRedemptionStatus = "rejected"
)

type RedeemCodeSourceType string

const (
	RedeemCodeSourceTypeCampaign       RedeemCodeSourceType = "campaign"
	RedeemCodeSourceTypeFree           RedeemCodeSourceType = "free"
	RedeemCodeSourceTypePurchaseOrder  RedeemCodeSourceType = "purchase_order"
	RedeemCodeSourceTypeLegacyDelivery RedeemCodeSourceType = "legacy_delivery"
)

type RedeemCampaign struct {
	ID                       string         `json:"id"`
	Name                     string         `json:"name"`
	Description              string         `json:"description"`
	CodeMode                 RedeemCodeMode `json:"codeMode"`
	SubscriptionPlanID       string         `json:"subscriptionPlanId"`
	SubscriptionDurationDays int            `json:"subscriptionDurationDays"`
	BalanceMicros            int64          `json:"balanceMicros"`
	TotalRedemptionsLimit    int            `json:"totalRedemptionsLimit"`
	RedeemedCount            int            `json:"redeemedCount"`
	PerUserLimit             int            `json:"perUserLimit"`
	StartsAt                 *time.Time     `json:"startsAt"`
	EndsAt                   *time.Time     `json:"endsAt"`
	Enabled                  bool           `json:"enabled"`
	CreatedAt                time.Time      `json:"createdAt"`
	UpdatedAt                time.Time      `json:"updatedAt"`
}

type RedeemCampaignRequest struct {
	Name                     string         `json:"name" binding:"required,min=1,max=64"`
	Description              string         `json:"description" binding:"max=160"`
	CodeMode                 RedeemCodeMode `json:"codeMode" binding:"required,oneof=single_use shared"`
	SharedCode               string         `json:"sharedCode" binding:"max=64"`
	SubscriptionPlanID       string         `json:"subscriptionPlanId"`
	SubscriptionDurationDays int            `json:"subscriptionDurationDays" binding:"min=0,max=3650"`
	BalanceMicros            int64          `json:"balanceMicros" binding:"min=0"`
	TotalRedemptionsLimit    int            `json:"totalRedemptionsLimit" binding:"min=0,max=1000000"`
	PerUserLimit             int            `json:"perUserLimit" binding:"min=1,max=10000"`
	StartsAt                 *time.Time     `json:"startsAt"`
	EndsAt                   *time.Time     `json:"endsAt"`
	Enabled                  bool           `json:"enabled"`
}

type RedeemCampaignResponse struct {
	ID                       string         `json:"id"`
	Name                     string         `json:"name"`
	Description              string         `json:"description"`
	CodeMode                 RedeemCodeMode `json:"codeMode"`
	SharedCode               string         `json:"sharedCode,omitempty"`
	SharedCodeMask           string         `json:"sharedCodeMask,omitempty"`
	SubscriptionPlanID       string         `json:"subscriptionPlanId"`
	SubscriptionPlanName     string         `json:"subscriptionPlanName"`
	SubscriptionDurationDays int            `json:"subscriptionDurationDays"`
	BalanceMicros            int64          `json:"balanceMicros"`
	TotalRedemptionsLimit    int            `json:"totalRedemptionsLimit"`
	RedeemedCount            int            `json:"redeemedCount"`
	PerUserLimit             int            `json:"perUserLimit"`
	StartsAt                 *time.Time     `json:"startsAt"`
	EndsAt                   *time.Time     `json:"endsAt"`
	Enabled                  bool           `json:"enabled"`
	CodeCount                int            `json:"codeCount"`
	BatchCount               int            `json:"batchCount"`
	CreatedAt                time.Time      `json:"createdAt"`
	UpdatedAt                time.Time      `json:"updatedAt"`
}

type RedeemCodeBatch struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"campaignId"`
	Name       string    `json:"name"`
	Prefix     string    `json:"prefix"`
	CodeCount  int       `json:"codeCount"`
	CodeLength int       `json:"codeLength"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type RedeemCodeBatchRequest struct {
	Name       string `json:"name" binding:"required,min=1,max=64"`
	Prefix     string `json:"prefix" binding:"max=16"`
	CodeCount  int    `json:"codeCount" binding:"required,min=1,max=5000"`
	CodeLength int    `json:"codeLength" binding:"required,min=6,max=32"`
}

type RedeemCodeBatchResponse struct {
	ID           string    `json:"id"`
	CampaignID   string    `json:"campaignId"`
	CampaignName string    `json:"campaignName"`
	Name         string    `json:"name"`
	Prefix       string    `json:"prefix"`
	CodeCount    int       `json:"codeCount"`
	CodeLength   int       `json:"codeLength"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type RedeemCode struct {
	ID                       string               `json:"id"`
	CampaignID               *string              `json:"campaignId"`
	BatchID                  *string              `json:"batchId"`
	SourceType               RedeemCodeSourceType `json:"sourceType"`
	SourceRefID              string               `json:"sourceRefId"`
	CodeValue                string               `json:"codeValue"`
	CodeHash                 string               `json:"-"`
	CodeMask                 string               `json:"codeMask"`
	SubscriptionPlanID       *string              `json:"subscriptionPlanId"`
	SubscriptionDurationDays int                  `json:"subscriptionDurationDays"`
	BalanceMicros            int64                `json:"balanceMicros"`
	RewardSnapshotJSON       string               `json:"rewardSnapshotJson,omitempty"`
	LegacySource             string               `json:"legacySource,omitempty"`
	LegacyRefID              string               `json:"legacyRefId,omitempty"`
	PerUserLimit             int                  `json:"perUserLimit"`
	StartsAt                 *time.Time           `json:"startsAt"`
	EndsAt                   *time.Time           `json:"endsAt"`
	Status                   RedeemCodeStatus     `json:"status"`
	MaxRedemptions           int                  `json:"maxRedemptions"`
	RedeemedCount            int                  `json:"redeemedCount"`
	LastRedeemedAt           *time.Time           `json:"lastRedeemedAt"`
	CreatedAt                time.Time            `json:"createdAt"`
	UpdatedAt                time.Time            `json:"updatedAt"`
}

type RedeemCodeResponse struct {
	ID                       string               `json:"id"`
	CampaignID               *string              `json:"campaignId"`
	CampaignName             string               `json:"campaignName"`
	BatchID                  *string              `json:"batchId"`
	BatchName                string               `json:"batchName,omitempty"`
	SourceType               RedeemCodeSourceType `json:"sourceType"`
	SourceRefID              string               `json:"sourceRefId"`
	CodeMode                 RedeemCodeMode       `json:"codeMode"`
	CodeValue                string               `json:"codeValue"`
	CodeMask                 string               `json:"codeMask"`
	SubscriptionPlanID       string               `json:"subscriptionPlanId"`
	SubscriptionPlanName     string               `json:"subscriptionPlanName"`
	SubscriptionDurationDays int                  `json:"subscriptionDurationDays"`
	BalanceMicros            int64                `json:"balanceMicros"`
	RewardSnapshotJSON       string               `json:"rewardSnapshotJson,omitempty"`
	LegacySource             string               `json:"legacySource,omitempty"`
	LegacyRefID              string               `json:"legacyRefId,omitempty"`
	PerUserLimit             int                  `json:"perUserLimit"`
	StartsAt                 *time.Time           `json:"startsAt"`
	EndsAt                   *time.Time           `json:"endsAt"`
	Status                   RedeemCodeStatus     `json:"status"`
	MaxRedemptions           int                  `json:"maxRedemptions"`
	RedeemedCount            int                  `json:"redeemedCount"`
	LastRedeemedAt           *time.Time           `json:"lastRedeemedAt"`
	CreatedAt                time.Time            `json:"createdAt"`
	UpdatedAt                time.Time            `json:"updatedAt"`
}

type ManualRedeemCodeRequest struct {
	CampaignID               string     `json:"campaignId"`
	CodeValue                string     `json:"codeValue" binding:"required,min=3,max=128"`
	SubscriptionPlanID       string     `json:"subscriptionPlanId"`
	SubscriptionDurationDays int        `json:"subscriptionDurationDays" binding:"min=0,max=3650"`
	BalanceMicros            int64      `json:"balanceMicros" binding:"min=0"`
	PerUserLimit             int        `json:"perUserLimit" binding:"min=1,max=10000"`
	MaxRedemptions           int        `json:"maxRedemptions" binding:"min=1,max=1000000"`
	StartsAt                 *time.Time `json:"startsAt"`
	EndsAt                   *time.Time `json:"endsAt"`
	Enabled                  bool       `json:"enabled"`
}

type RedeemRedemption struct {
	ID                       string                 `json:"id"`
	CampaignID               *string                `json:"campaignId"`
	CodeID                   *string                `json:"codeId"`
	UserID                   string                 `json:"userId"`
	Username                 string                 `json:"username"`
	CodeInput                string                 `json:"codeInput"`
	CodeMask                 string                 `json:"codeMask"`
	SubscriptionPlanID       string                 `json:"subscriptionPlanId"`
	SubscriptionDurationDays int                    `json:"subscriptionDurationDays"`
	BalanceMicros            int64                  `json:"balanceMicros"`
	RewardSnapshotJSON       string                 `json:"rewardSnapshotJson,omitempty"`
	Status                   RedeemRedemptionStatus `json:"status"`
	FailureReason            string                 `json:"failureReason"`
	GrantedSubscriptionID    string                 `json:"grantedSubscriptionId"`
	GrantedExpiresAt         *time.Time             `json:"grantedExpiresAt"`
	BalanceAfterMicros       int64                  `json:"balanceAfterMicros"`
	CreatedAt                time.Time              `json:"createdAt"`
}

type RedeemRedemptionResponse struct {
	ID                       string                 `json:"id"`
	CampaignID               *string                `json:"campaignId"`
	CampaignName             string                 `json:"campaignName"`
	CodeID                   *string                `json:"codeId"`
	UserID                   string                 `json:"userId"`
	Username                 string                 `json:"username"`
	CodeMask                 string                 `json:"codeMask"`
	SubscriptionPlanID       string                 `json:"subscriptionPlanId"`
	SubscriptionPlanName     string                 `json:"subscriptionPlanName"`
	SubscriptionDurationDays int                    `json:"subscriptionDurationDays"`
	BalanceMicros            int64                  `json:"balanceMicros"`
	RewardSnapshotJSON       string                 `json:"rewardSnapshotJson,omitempty"`
	Status                   RedeemRedemptionStatus `json:"status"`
	FailureReason            string                 `json:"failureReason"`
	GrantedSubscriptionID    string                 `json:"grantedSubscriptionId"`
	GrantedExpiresAt         *time.Time             `json:"grantedExpiresAt"`
	BalanceAfterMicros       int64                  `json:"balanceAfterMicros"`
	CreatedAt                time.Time              `json:"createdAt"`
}

type RedeemRequest struct {
	Code string `json:"code" binding:"required,min=3,max=128"`
}

type RedeemResultResponse struct {
	Message             string                    `json:"message"`
	Campaign            *RedeemCampaignResponse   `json:"campaign"`
	Redemption          *RedeemRedemptionResponse `json:"redemption"`
	CurrentSubscription *UserSubscriptionResponse `json:"currentSubscription,omitempty"`
	BalanceMicros       int64                     `json:"balanceMicros"`
	BalanceUsd          string                    `json:"balanceUsd"`
}
