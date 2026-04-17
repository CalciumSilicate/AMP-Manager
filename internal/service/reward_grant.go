package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"ampmanager/internal/billingstate"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
)

type BillingStateSyncAction struct {
	UserID             string
	RefreshUserState   bool
	BalanceDeltaMicros int64
}

type RewardGrantService struct {
	refreshUserState  func(ctx context.Context, userID string) error
	applyBalanceDelta func(ctx context.Context, userID string, deltaMicros int64) error
}

func NewRewardGrantService() *RewardGrantService {
	return &RewardGrantService{
		refreshUserState: func(ctx context.Context, userID string) error {
			if runtime := billingstate.Get(); runtime != nil {
				return runtime.RefreshUserState(ctx, userID)
			}
			return nil
		},
		applyBalanceDelta: func(ctx context.Context, userID string, deltaMicros int64) error {
			if runtime := billingstate.Get(); runtime != nil {
				return runtime.ApplyBalanceDelta(ctx, userID, deltaMicros)
			}
			return nil
		},
	}
}

func (s *RewardGrantService) SyncBillingState(ctx context.Context, action BillingStateSyncAction) error {
	userID := strings.TrimSpace(action.UserID)
	if userID == "" {
		return nil
	}
	if action.RefreshUserState {
		if s != nil && s.refreshUserState != nil {
			return s.refreshUserState(ctx, userID)
		}
		return nil
	}
	if action.BalanceDeltaMicros != 0 {
		if s != nil && s.applyBalanceDelta != nil {
			return s.applyBalanceDelta(ctx, userID, action.BalanceDeltaMicros)
		}
	}
	return nil
}

func (s *RewardGrantService) GrantSubscriptionTx(tx *sql.Tx, userID, planID string, durationDays int, now time.Time) (*model.UserSubscription, error) {
	if tx == nil {
		return nil, fmt.Errorf("grant subscription: nil tx")
	}
	if planID == "" || durationDays <= 0 {
		return nil, fmt.Errorf("grant subscription: invalid reward config")
	}

	activeSub, err := s.getActiveSubscriptionTx(tx, userID, now)
	if err != nil {
		return nil, err
	}

	if activeSub == nil {
		expiresAt := now.AddDate(0, 0, durationDays)
		sub := &model.UserSubscription{
			ID:        uuid.New().String(),
			UserID:    userID,
			PlanID:    planID,
			StartsAt:  now,
			ExpiresAt: &expiresAt,
			Status:    model.SubscriptionStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if _, err := tx.Exec(
			`INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			sub.ID,
			sub.UserID,
			sub.PlanID,
			sub.StartsAt,
			sub.ExpiresAt,
			sub.Status,
			sub.CreatedAt,
			sub.UpdatedAt,
		); err != nil {
			return nil, err
		}
		return sub, nil
	}

	if activeSub.PlanID != planID {
		return nil, ErrDifferentPlanActive
	}
	if activeSub.ExpiresAt == nil {
		return nil, ErrPermanentSubscription
	}

	base := now
	if activeSub.ExpiresAt != nil && activeSub.ExpiresAt.After(now) {
		base = activeSub.ExpiresAt.UTC()
	}
	expiresAt := base.AddDate(0, 0, durationDays)
	if _, err := tx.Exec(
		`UPDATE user_subscriptions SET expires_at = ?, updated_at = ? WHERE id = ?`,
		expiresAt,
		now,
		activeSub.ID,
	); err != nil {
		return nil, err
	}
	activeSub.ExpiresAt = &expiresAt
	activeSub.UpdatedAt = now
	return activeSub, nil
}

func (s *RewardGrantService) GrantBalanceTx(tx *sql.Tx, userID string, amountMicros int64, now time.Time) (int64, error) {
	if tx == nil {
		return 0, fmt.Errorf("grant balance: nil tx")
	}
	if amountMicros <= 0 {
		return s.queryBalanceTx(tx, userID)
	}

	result, err := tx.Exec(
		`UPDATE users SET balance_micros = balance_micros + ?, updated_at = ? WHERE id = ?`,
		amountMicros,
		now,
		userID,
	)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected == 0 {
		return 0, repository.ErrUserNotFound
	}

	if _, err := tx.Exec(
		`INSERT INTO billing_events (id, request_log_id, user_id, user_subscription_id, source, event_type, amount_micros, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(),
		nil,
		userID,
		nil,
		model.BillingSourceBalance,
		"adjustment",
		amountMicros,
		now,
	); err != nil {
		return 0, err
	}

	return s.queryBalanceTx(tx, userID)
}

func (s *RewardGrantService) getActiveSubscriptionTx(tx *sql.Tx, userID string, now time.Time) (*model.UserSubscription, error) {
	sub := &model.UserSubscription{}
	err := tx.QueryRow(
		`SELECT id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at
		   FROM user_subscriptions
		  WHERE user_id = ? AND status = 'active' AND (expires_at IS NULL OR expires_at > ?)`,
		userID,
		now.UTC(),
	).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.PlanID,
		&sub.StartsAt,
		&sub.ExpiresAt,
		&sub.Status,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

func (s *RewardGrantService) queryBalanceTx(tx *sql.Tx, userID string) (int64, error) {
	var balance int64
	err := tx.QueryRow(`SELECT balance_micros FROM users WHERE id = ?`, userID).Scan(&balance)
	if err == sql.ErrNoRows {
		return 0, repository.ErrUserNotFound
	}
	return balance, err
}
