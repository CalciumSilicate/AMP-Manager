package model

import "time"

type UserBatchTargetMode string

const (
	UserBatchTargetSelected UserBatchTargetMode = "selected"
	UserBatchTargetFiltered UserBatchTargetMode = "filtered"
)

type BalanceBatchChangeMode string

const (
	BalanceBatchChangeAdd      BalanceBatchChangeMode = "add"
	BalanceBatchChangeSet      BalanceBatchChangeMode = "set"
	BalanceBatchChangeSubtract BalanceBatchChangeMode = "subtract"
)

type GroupBatchChangeMode string

const (
	GroupBatchChangeAdd    GroupBatchChangeMode = "add"
	GroupBatchChangeSet    GroupBatchChangeMode = "set"
	GroupBatchChangeRemove GroupBatchChangeMode = "remove"
)

type SubscriptionBatchPlanMode string

const (
	SubscriptionBatchPlanKeep   SubscriptionBatchPlanMode = "keep"
	SubscriptionBatchPlanAssign SubscriptionBatchPlanMode = "assign"
	SubscriptionBatchPlanCancel SubscriptionBatchPlanMode = "cancel"
)

type SubscriptionBatchExpiryMode string

const (
	SubscriptionBatchExpiryKeep       SubscriptionBatchExpiryMode = "keep"
	SubscriptionBatchExpirySet        SubscriptionBatchExpiryMode = "set"
	SubscriptionBatchExpiryExtendDays SubscriptionBatchExpiryMode = "extend_days"
	SubscriptionBatchExpiryShortenDays SubscriptionBatchExpiryMode = "shorten_days"
)

type UserBatchFilter struct {
	Keyword                  string               `json:"keyword"`
	IsAdmin                  *bool                `json:"isAdmin,omitempty"`
	GroupIDs                 []string             `json:"groupIds,omitempty"`
	SubscriptionStatuses     []SubscriptionStatus `json:"subscriptionStatuses,omitempty"`
	PlanIDs                  []string             `json:"planIds,omitempty"`
	BalanceMinMicros         *int64               `json:"balanceMinMicros,omitempty"`
	BalanceMaxMicros         *int64               `json:"balanceMaxMicros,omitempty"`
	SubscriptionExpiresAfter *time.Time           `json:"subscriptionExpiresAfter,omitempty"`
	SubscriptionExpiresBefore *time.Time          `json:"subscriptionExpiresBefore,omitempty"`
	LimitType                *LimitType           `json:"limitType,omitempty"`
	LimitMinMicros           *int64               `json:"limitMinMicros,omitempty"`
	LimitMaxMicros           *int64               `json:"limitMaxMicros,omitempty"`
}

type BalanceBatchChange struct {
	Mode         BalanceBatchChangeMode `json:"mode"`
	AmountMicros int64                  `json:"amountMicros"`
}

type GroupBatchChange struct {
	Mode     GroupBatchChangeMode `json:"mode"`
	GroupIDs []string             `json:"groupIds,omitempty"`
}

type SubscriptionBatchChange struct {
	PlanMode   SubscriptionBatchPlanMode   `json:"planMode"`
	PlanID     string                      `json:"planId,omitempty"`
	ExpiryMode SubscriptionBatchExpiryMode `json:"expiryMode"`
	ExpiresAt  *time.Time                  `json:"expiresAt,omitempty"`
	Days       int                         `json:"days,omitempty"`
}

type UserBatchChangeSet struct {
	Balance          *BalanceBatchChange             `json:"balance,omitempty"`
	Groups           *GroupBatchChange               `json:"groups,omitempty"`
	Subscription     *SubscriptionBatchChange        `json:"subscription,omitempty"`
	ConcurrencyLimit *UpdateUserConcurrencyLimitRequest `json:"concurrency,omitempty"`
}

type UserBatchPreviewRequest struct {
	TargetMode      UserBatchTargetMode `json:"targetMode"`
	SelectedUserIDs []string            `json:"selectedUserIds,omitempty"`
	Filters         *UserBatchFilter    `json:"filters,omitempty"`
}

type UserBatchApplyRequest struct {
	TargetMode      UserBatchTargetMode `json:"targetMode"`
	SelectedUserIDs []string            `json:"selectedUserIds,omitempty"`
	Filters         *UserBatchFilter    `json:"filters,omitempty"`
	Changes         UserBatchChangeSet  `json:"changes"`
}

type UserBatchPreviewItem struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	IsAdmin   bool   `json:"isAdmin"`
	BalanceUsd string `json:"balanceUsd"`
}

type UserBatchPreviewResponse struct {
	Count int                    `json:"count"`
	Items []UserBatchPreviewItem `json:"items"`
}

type UserBatchApplyResponse struct {
	MatchedCount int `json:"matchedCount"`
	AppliedCount int `json:"appliedCount"`
}
