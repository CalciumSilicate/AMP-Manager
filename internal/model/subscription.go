package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type BillingSource string

const (
	BillingSourceSubscription BillingSource = "subscription"
	BillingSourceBalance      BillingSource = "balance"
)

type WindowMode string

const (
	WindowModeFixed   WindowMode = "fixed"
	WindowModeSliding WindowMode = "sliding"
)

type LimitType string

const (
	LimitTypeDaily     LimitType = "daily"
	LimitTypeWeekly    LimitType = "weekly"
	LimitTypeMonthly   LimitType = "monthly"
	LimitTypeRolling5h LimitType = "rolling_5h"
	LimitTypeTotal     LimitType = "total"
)

type SubscriptionStatus string

const (
	SubscriptionStatusActive    SubscriptionStatus = "active"
	SubscriptionStatusPaused    SubscriptionStatus = "paused"
	SubscriptionStatusExpired   SubscriptionStatus = "expired"
	SubscriptionStatusCancelled SubscriptionStatus = "cancelled"
)

type SubscriptionTimelinePhaseType string

const (
	SubscriptionTimelinePhaseTypeBoost              SubscriptionTimelinePhaseType = "boost"
	SubscriptionTimelinePhaseTypeRestore            SubscriptionTimelinePhaseType = "restore"
	SubscriptionTimelinePhaseTypeConvertedExtension SubscriptionTimelinePhaseType = "converted_extension"
	SubscriptionTimelinePhaseTypeOverwrite          SubscriptionTimelinePhaseType = "overwrite"
	SubscriptionTimelinePhaseTypeExtendDuration     SubscriptionTimelinePhaseType = "extend_duration"
)

type SubscriptionTimelinePhaseStatus string

const (
	SubscriptionTimelinePhaseStatusScheduled  SubscriptionTimelinePhaseStatus = "scheduled"
	SubscriptionTimelinePhaseStatusActive     SubscriptionTimelinePhaseStatus = "active"
	SubscriptionTimelinePhaseStatusCompleted  SubscriptionTimelinePhaseStatus = "completed"
	SubscriptionTimelinePhaseStatusSuperseded SubscriptionTimelinePhaseStatus = "superseded"
	SubscriptionTimelinePhaseStatusCancelled  SubscriptionTimelinePhaseStatus = "cancelled"
)

type SubscriptionPlan struct {
	ID                            string    `json:"id"`
	Name                          string    `json:"name"`
	Description                   string    `json:"description"`
	Enabled                       bool      `json:"enabled"`
	UpgradeRank                   int       `json:"upgradeRank"`
	UpgradeValuationCnyCentPerDay int64     `json:"upgradeValuationCnyCentPerDay"`
	LegacySource                  string    `json:"legacySource,omitempty"`
	LegacyRefID                   string    `json:"legacyRefId,omitempty"`
	CreatedAt                     time.Time `json:"createdAt"`
	UpdatedAt                     time.Time `json:"updatedAt"`
}

type SubscriptionPlanLimit struct {
	ID             string     `json:"id"`
	PlanID         string     `json:"planId"`
	LimitType      LimitType  `json:"limitType"`
	WindowMode     WindowMode `json:"windowMode"`
	LimitMicros    int64      `json:"limitMicros"`
	FixedResetTime *string    `json:"fixedResetTime,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type UserSubscription struct {
	ID              string             `json:"id"`
	UserID          string             `json:"userId"`
	PlanID          string             `json:"planId"`
	PlanUpgradeRank int                `json:"planUpgradeRank"`
	StartsAt        time.Time          `json:"startsAt"`
	ExpiresAt       *time.Time         `json:"expiresAt"`
	Status          SubscriptionStatus `json:"status"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

type SubscriptionTimelinePhase struct {
	ID                   string                          `json:"id"`
	UserID               string                          `json:"userId"`
	UserSubscriptionID   string                          `json:"userSubscriptionId"`
	PlanID               string                          `json:"planId"`
	PhaseType            SubscriptionTimelinePhaseType   `json:"phaseType"`
	Status               SubscriptionTimelinePhaseStatus `json:"status"`
	SourceType           string                          `json:"sourceType"`
	SourceRefID          string                          `json:"sourceRefId"`
	DailyLimitMicros     *int64                          `json:"dailyLimitMicros,omitempty"`
	WeeklyLimitMicros    *int64                          `json:"weeklyLimitMicros,omitempty"`
	MonthlyLimitMicros   *int64                          `json:"monthlyLimitMicros,omitempty"`
	Rolling5hLimitMicros *int64                          `json:"rolling5hLimitMicros,omitempty"`
	TotalLimitMicros     *int64                          `json:"totalLimitMicros,omitempty"`
	FixedResetTime       *string                         `json:"fixedResetTime,omitempty"`
	StartsAt             time.Time                       `json:"startsAt"`
	EndsAt               time.Time                       `json:"endsAt"`
	FinalExpiresAt       *time.Time                      `json:"finalExpiresAt,omitempty"`
	PreviewJSON          string                          `json:"previewJson,omitempty"`
	AppliedAt            *time.Time                      `json:"appliedAt,omitempty"`
	LegacySource         string                          `json:"legacySource,omitempty"`
	LegacyRefID          string                          `json:"legacyRefId,omitempty"`
	CreatedAt            time.Time                       `json:"createdAt"`
	UpdatedAt            time.Time                       `json:"updatedAt"`
}

type SubscriptionRechargeHistory struct {
	ID                           string     `json:"id"`
	UserID                       string     `json:"userId"`
	UserSubscriptionID           string     `json:"userSubscriptionId"`
	PlanID                       string     `json:"planId"`
	Mode                         string     `json:"mode"`
	Status                       string     `json:"status"`
	SourceType                   string     `json:"sourceType"`
	SourceRefID                  string     `json:"sourceRefId"`
	SourceDailyLimitMicros       *int64     `json:"sourceDailyLimitMicros,omitempty"`
	TargetDailyLimitBeforeMicros *int64     `json:"targetDailyLimitBeforeMicros,omitempty"`
	PeakDailyLimitMicros         *int64     `json:"peakDailyLimitMicros,omitempty"`
	TargetExpiresAtBefore        *time.Time `json:"targetExpiresAtBefore,omitempty"`
	TargetExpiresAtAfter         *time.Time `json:"targetExpiresAtAfter,omitempty"`
	PreviewJSON                  string     `json:"previewJson,omitempty"`
	ConfirmedAt                  *time.Time `json:"confirmedAt,omitempty"`
	AppliedAt                    *time.Time `json:"appliedAt,omitempty"`
	LegacySource                 string     `json:"legacySource,omitempty"`
	LegacyRefID                  string     `json:"legacyRefId,omitempty"`
	CreatedAt                    time.Time  `json:"createdAt"`
	UpdatedAt                    time.Time  `json:"updatedAt"`
}

type SubscriptionEntitlementSourceType string

const (
	SubscriptionEntitlementSourceLegacySnapshot SubscriptionEntitlementSourceType = "legacy_snapshot"
	SubscriptionEntitlementSourcePurchase       SubscriptionEntitlementSourceType = "purchase"
	SubscriptionEntitlementSourceRedeem         SubscriptionEntitlementSourceType = "redeem"
	SubscriptionEntitlementSourceAdminAssign    SubscriptionEntitlementSourceType = "admin_assign"
	SubscriptionEntitlementSourceAdminAdjust    SubscriptionEntitlementSourceType = "admin_adjust"
	SubscriptionEntitlementSourceUpgrade        SubscriptionEntitlementSourceType = "upgrade"
)

type SubscriptionEntitlementStatus string

const (
	SubscriptionEntitlementStatusActive    SubscriptionEntitlementStatus = "active"
	SubscriptionEntitlementStatusConsumed  SubscriptionEntitlementStatus = "consumed"
	SubscriptionEntitlementStatusCancelled SubscriptionEntitlementStatus = "cancelled"
)

type SubscriptionEntitlement struct {
	ID                        string                            `json:"id"`
	UserID                    string                            `json:"userId"`
	PlanID                    string                            `json:"planId"`
	SourceType                SubscriptionEntitlementSourceType `json:"sourceType"`
	SourceRefID               string                            `json:"sourceRefId"`
	ValuationCnyCentPerDay    int64                             `json:"valuationCnyCentPerDay"`
	StartsAt                  time.Time                         `json:"startsAt"`
	ExpiresAt                 *time.Time                        `json:"expiresAt"`
	Status                    SubscriptionEntitlementStatus     `json:"status"`
	ConsumedByPurchaseOrderNo string                            `json:"consumedByPurchaseOrderNo"`
	ConsumedAt                *time.Time                        `json:"consumedAt"`
	CreatedAt                 time.Time                         `json:"createdAt"`
	UpdatedAt                 time.Time                         `json:"updatedAt"`
}

type UserBillingSetting struct {
	UserID          string        `json:"userId"`
	PrimarySource   BillingSource `json:"primarySource"`
	SecondarySource BillingSource `json:"secondarySource"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

type BillingEvent struct {
	ID                 string        `json:"id"`
	RequestLogID       *string       `json:"requestLogId"`
	UserID             string        `json:"userId"`
	UserSubscriptionID *string       `json:"userSubscriptionId"`
	Source             BillingSource `json:"source"`
	EventType          string        `json:"eventType"`
	AmountMicros       int64         `json:"amountMicros"`
	CreatedAt          time.Time     `json:"createdAt"`
}

type BillingDailyResetRecord struct {
	ID                    string    `json:"id"`
	UserID                string    `json:"userId"`
	UserSubscriptionID    string    `json:"userSubscriptionId"`
	WindowStart           time.Time `json:"windowStart"`
	WindowEnd             time.Time `json:"windowEnd"`
	UsedMicrosBeforeReset int64     `json:"usedMicrosBeforeReset"`
	ExpiresAtBefore       time.Time `json:"expiresAtBefore"`
	ExpiresAtAfter        time.Time `json:"expiresAtAfter"`
	CreatedAt             time.Time `json:"createdAt"`
}

type BillingDailyResetState struct {
	Supported             bool    `json:"supported"`
	Allowed               bool    `json:"allowed"`
	Message               string  `json:"message"`
	UsedToday             int     `json:"usedToday"`
	DailyLimit            int     `json:"dailyLimit"`
	CurrentUsagePercent   float64 `json:"currentUsagePercent"`
	UsageThresholdPercent int     `json:"usageThresholdPercent"`
	MinRemainingDays      int     `json:"minRemainingDays"`
}

// --- Request / Response DTOs ---

type SubscriptionPlanRequest struct {
	Name                          string             `json:"name" binding:"required,min=1,max=64"`
	Description                   string             `json:"description" binding:"max=256"`
	Enabled                       bool               `json:"enabled"`
	UpgradeRank                   int                `json:"upgradeRank" binding:"min=0,max=1000000"`
	UpgradeValuationCnyCentPerDay int64              `json:"upgradeValuationCnyCentPerDay" binding:"min=0"`
	Limits                        []PlanLimitRequest `json:"limits"`
}

type PlanLimitRequest struct {
	LimitType      LimitType  `json:"limitType" binding:"required"`
	WindowMode     WindowMode `json:"windowMode" binding:"required"`
	LimitMicros    int64      `json:"limitMicros" binding:"required,min=0"`
	FixedResetTime *string    `json:"fixedResetTime,omitempty"`
}

type SubscriptionPlanResponse struct {
	ID                            string                  `json:"id"`
	Name                          string                  `json:"name"`
	Description                   string                  `json:"description"`
	Enabled                       bool                    `json:"enabled"`
	UpgradeRank                   int                     `json:"upgradeRank"`
	UpgradeValuationCnyCentPerDay int64                   `json:"upgradeValuationCnyCentPerDay"`
	Limits                        []SubscriptionPlanLimit `json:"limits"`
	CreatedAt                     time.Time               `json:"createdAt"`
	UpdatedAt                     time.Time               `json:"updatedAt"`
}

type AssignSubscriptionRequest struct {
	PlanID    string     `json:"planId" binding:"required"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

type UserSubscriptionResponse struct {
	ID              string                       `json:"id"`
	UserID          string                       `json:"userId"`
	PlanID          string                       `json:"planId"`
	PlanName        string                       `json:"planName"`
	PlanUpgradeRank int                          `json:"planUpgradeRank"`
	StartsAt        time.Time                    `json:"startsAt"`
	ExpiresAt       *time.Time                   `json:"expiresAt"`
	Status          SubscriptionStatus           `json:"status"`
	Limits          []SubscriptionPlanLimit      `json:"limits"`
	EffectiveLimits []SubscriptionPlanLimit      `json:"effectiveLimits,omitempty"`
	Timeline        []*SubscriptionTimelinePhase `json:"timeline,omitempty"`
	FinalExpiresAt  *time.Time                   `json:"finalExpiresAt,omitempty"`
	CreatedAt       time.Time                    `json:"createdAt"`
	UpdatedAt       time.Time                    `json:"updatedAt"`
}

type WindowRemaining struct {
	LimitType   LimitType  `json:"limitType"`
	WindowMode  WindowMode `json:"windowMode"`
	LimitMicros int64      `json:"limitMicros"`
	UsedMicros  int64      `json:"usedMicros"`
	LeftMicros  int64      `json:"leftMicros"`
	WindowStart time.Time  `json:"windowStart"`
	WindowEnd   time.Time  `json:"windowEnd"`
}

type BillingStateResponse struct {
	BalanceMicros   int64                        `json:"balanceMicros"`
	BalanceUsd      string                       `json:"balanceUsd"`
	Subscription    *UserSubscriptionResponse    `json:"subscription"`
	Windows         []WindowRemaining            `json:"windows"`
	DailyReset      BillingDailyResetState       `json:"dailyReset"`
	PrimarySource   BillingSource                `json:"primarySource"`
	SecondarySource BillingSource                `json:"secondarySource"`
	Timeline        []*SubscriptionTimelinePhase `json:"timeline,omitempty"`
	FinalExpiresAt  *time.Time                   `json:"finalExpiresAt,omitempty"`
}

type UpdateBillingPriorityRequest struct {
	PrimarySource BillingSource `json:"primarySource" binding:"required"`
}

func ParseFixedResetTime(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("重置时间不能为空")
	}

	parts := strings.Split(trimmed, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("重置时间格式错误，应为 HH:mm")
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("重置时间格式错误，应为 HH:mm")
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("重置时间格式错误，应为 HH:mm")
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("重置时间格式错误，应为 HH:mm")
	}

	return hour*60 + minute, nil
}

func FormatFixedResetTime(minutes *int) *string {
	if minutes == nil {
		return nil
	}
	if *minutes < 0 || *minutes >= 24*60 {
		return nil
	}

	formatted := fmt.Sprintf("%02d:%02d", *minutes/60, *minutes%60)
	return &formatted
}
