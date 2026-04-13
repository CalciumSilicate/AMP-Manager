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

type SubscriptionPlan struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
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
	ID        string             `json:"id"`
	UserID    string             `json:"userId"`
	PlanID    string             `json:"planId"`
	StartsAt  time.Time          `json:"startsAt"`
	ExpiresAt *time.Time         `json:"expiresAt"`
	Status    SubscriptionStatus `json:"status"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
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

// --- Request / Response DTOs ---

type SubscriptionPlanRequest struct {
	Name        string             `json:"name" binding:"required,min=1,max=64"`
	Description string             `json:"description" binding:"max=256"`
	Enabled     bool               `json:"enabled"`
	Limits      []PlanLimitRequest `json:"limits"`
}

type PlanLimitRequest struct {
	LimitType      LimitType  `json:"limitType" binding:"required"`
	WindowMode     WindowMode `json:"windowMode" binding:"required"`
	LimitMicros    int64      `json:"limitMicros" binding:"required,min=0"`
	FixedResetTime *string    `json:"fixedResetTime,omitempty"`
}

type SubscriptionPlanResponse struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Enabled     bool                    `json:"enabled"`
	Limits      []SubscriptionPlanLimit `json:"limits"`
	CreatedAt   time.Time               `json:"createdAt"`
	UpdatedAt   time.Time               `json:"updatedAt"`
}

type AssignSubscriptionRequest struct {
	PlanID    string     `json:"planId" binding:"required"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

type UserSubscriptionResponse struct {
	ID        string                  `json:"id"`
	UserID    string                  `json:"userId"`
	PlanID    string                  `json:"planId"`
	PlanName  string                  `json:"planName"`
	StartsAt  time.Time               `json:"startsAt"`
	ExpiresAt *time.Time              `json:"expiresAt"`
	Status    SubscriptionStatus      `json:"status"`
	Limits    []SubscriptionPlanLimit `json:"limits"`
	CreatedAt time.Time               `json:"createdAt"`
	UpdatedAt time.Time               `json:"updatedAt"`
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
	BalanceMicros   int64                     `json:"balanceMicros"`
	BalanceUsd      string                    `json:"balanceUsd"`
	Subscription    *UserSubscriptionResponse `json:"subscription"`
	Windows         []WindowRemaining         `json:"windows"`
	PrimarySource   BillingSource             `json:"primarySource"`
	SecondarySource BillingSource             `json:"secondarySource"`
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
