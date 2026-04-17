package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"ampmanager/internal/billingstate"
	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
)

var (
	ErrPlanDisabled         = errors.New("该套餐已禁用")
	ErrNoActiveSubscription = errors.New("用户没有活跃订阅")
)

type UserSubscriptionService struct {
	subRepo     repository.UserSubscriptionRepositoryInterface
	planRepo    repository.SubscriptionPlanRepositoryInterface
	entRepo     repository.SubscriptionEntitlementRepositoryInterface
	runtimeRepo *repository.SubscriptionRuntimeRepository
}

func NewUserSubscriptionService() *UserSubscriptionService {
	return &UserSubscriptionService{
		subRepo:     repository.NewUserSubscriptionRepository(),
		planRepo:    repository.NewSubscriptionPlanRepository(),
		entRepo:     repository.NewSubscriptionEntitlementRepository(),
		runtimeRepo: repository.NewSubscriptionRuntimeRepository(),
	}
}

func NewUserSubscriptionServiceWithRepo(
	subRepo repository.UserSubscriptionRepositoryInterface,
	planRepo repository.SubscriptionPlanRepositoryInterface,
) *UserSubscriptionService {
	return &UserSubscriptionService{
		subRepo:     subRepo,
		planRepo:    planRepo,
		entRepo:     repository.NewSubscriptionEntitlementRepository(),
		runtimeRepo: repository.NewSubscriptionRuntimeRepository(),
	}
}

func (s *UserSubscriptionService) Assign(userID string, req *model.AssignSubscriptionRequest) (*model.UserSubscriptionResponse, error) {
	plan, limits, err := s.planRepo.GetByID(req.PlanID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound
	}
	if !plan.Enabled {
		return nil, ErrPlanDisabled
	}

	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("assign subscription: begin tx: %w", err)
	}
	defer tx.Rollback()

	var existingID string
	now := time.Now().UTC()
	err = tx.QueryRow(
		`SELECT id FROM user_subscriptions WHERE user_id = ? AND status = 'active' AND (expires_at IS NULL OR expires_at > ?)`,
		userID, now,
	).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if existingID != "" {
		if _, err := tx.Exec(
			`UPDATE user_subscriptions SET status = ?, updated_at = ? WHERE id = ?`,
			model.SubscriptionStatusCancelled, now, existingID,
		); err != nil {
			return nil, err
		}
		if err := s.entRepo.CancelActiveByUserTx(tx, userID, now); err != nil {
			return nil, err
		}
	}

	sub := &model.UserSubscription{
		ID:        uuid.New().String(),
		UserID:    userID,
		PlanID:    req.PlanID,
		StartsAt:  now,
		ExpiresAt: req.ExpiresAt,
		Status:    model.SubscriptionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if _, err := tx.Exec(
		`INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.UserID, sub.PlanID, sub.StartsAt, sub.ExpiresAt, sub.Status, sub.CreatedAt, sub.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := s.entRepo.CreateTx(tx, &model.SubscriptionEntitlement{
		UserID:                 userID,
		PlanID:                 req.PlanID,
		SourceType:             model.SubscriptionEntitlementSourceAdminAssign,
		SourceRefID:            sub.ID,
		ValuationCnyCentPerDay: plan.UpgradeValuationCnyCentPerDay,
		StartsAt:               sub.StartsAt,
		ExpiresAt:              sub.ExpiresAt,
		Status:                 model.SubscriptionEntitlementStatusActive,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("assign subscription: commit: %w", err)
	}

	if runtime := billingstate.Get(); runtime != nil {
		if err := runtime.RefreshUserState(context.Background(), userID); err != nil {
			return nil, err
		}
	}

	return &model.UserSubscriptionResponse{
		ID:              sub.ID,
		UserID:          sub.UserID,
		PlanID:          sub.PlanID,
		PlanName:        plan.Name,
		PlanUpgradeRank: plan.UpgradeRank,
		StartsAt:        sub.StartsAt,
		ExpiresAt:       sub.ExpiresAt,
		Status:          sub.Status,
		Limits:          limits,
		CreatedAt:       sub.CreatedAt,
		UpdatedAt:       sub.UpdatedAt,
	}, nil
}

func (s *UserSubscriptionService) GetActive(userID string) (*model.UserSubscriptionResponse, error) {
	sub, err := s.subRepo.GetActiveByUserID(userID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, nil
	}

	plan, limits, err := s.planRepo.GetByID(sub.PlanID)
	if err != nil {
		return nil, err
	}

	planName := ""
	if plan != nil {
		planName = plan.Name
	}

	resp := &model.UserSubscriptionResponse{
		ID:       sub.ID,
		UserID:   sub.UserID,
		PlanID:   sub.PlanID,
		PlanName: planName,
		PlanUpgradeRank: func() int {
			if plan != nil {
				return plan.UpgradeRank
			}
			return 0
		}(),
		StartsAt:  sub.StartsAt,
		ExpiresAt: sub.ExpiresAt,
		Status:    sub.Status,
		Limits:    limits,
		CreatedAt: sub.CreatedAt,
		UpdatedAt: sub.UpdatedAt,
	}
	if s.runtimeRepo != nil {
		if effective, timeline, _, err := s.runtimeRepo.ResolveEffectiveLimitsBySubscription(sub, limits, time.Now().UTC()); err == nil {
			resp.EffectiveLimits = effective
			resp.Timeline = timeline
			resp.FinalExpiresAt = sub.ExpiresAt
		}
	}
	return resp, nil
}

func (s *UserSubscriptionService) Cancel(userID string) error {
	sub, err := s.subRepo.GetActiveByUserID(userID)
	if err != nil {
		return err
	}
	if sub == nil {
		return ErrNoActiveSubscription
	}
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`UPDATE user_subscriptions SET status = ?, updated_at = ? WHERE id = ?`,
		model.SubscriptionStatusCancelled,
		time.Now().UTC(),
		sub.ID,
	); err != nil {
		return err
	}
	if err := s.entRepo.CancelActiveByUserTx(tx, userID, time.Now().UTC()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if runtime := billingstate.Get(); runtime != nil {
		return runtime.RefreshUserState(context.Background(), userID)
	}
	return nil
}

func (s *UserSubscriptionService) UpdateExpiry(userID string, expiresAt time.Time) error {
	sub, err := s.subRepo.GetActiveByUserID(userID)
	if err != nil {
		return err
	}
	if sub == nil {
		return ErrNoActiveSubscription
	}
	plan, _, err := s.planRepo.GetByID(sub.PlanID)
	if err != nil {
		return err
	}
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`UPDATE user_subscriptions SET expires_at = ?, updated_at = ? WHERE id = ?`,
		expiresAt,
		time.Now().UTC(),
		sub.ID,
	); err != nil {
		return err
	}
	if err := s.entRepo.CancelActiveByUserTx(tx, userID, time.Now().UTC()); err != nil {
		return err
	}
	if err := s.entRepo.CreateTx(tx, &model.SubscriptionEntitlement{
		UserID:      userID,
		PlanID:      sub.PlanID,
		SourceType:  model.SubscriptionEntitlementSourceAdminAdjust,
		SourceRefID: sub.ID,
		ValuationCnyCentPerDay: func() int64 {
			if plan != nil {
				return plan.UpgradeValuationCnyCentPerDay
			}
			return 0
		}(),
		StartsAt:  sub.StartsAt,
		ExpiresAt: &expiresAt,
		Status:    model.SubscriptionEntitlementStatusActive,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if runtime := billingstate.Get(); runtime != nil {
		return runtime.RefreshUserState(context.Background(), userID)
	}
	return nil
}

func (s *UserSubscriptionService) ListByUserID(userID string) ([]*model.UserSubscriptionResponse, error) {
	subs, err := s.subRepo.ListByUserID(userID)
	if err != nil {
		return nil, err
	}

	result := make([]*model.UserSubscriptionResponse, len(subs))
	for i, sub := range subs {
		plan, limits, err := s.planRepo.GetByID(sub.PlanID)
		if err != nil {
			return nil, err
		}
		planName := ""
		if plan != nil {
			planName = plan.Name
		}
		result[i] = &model.UserSubscriptionResponse{
			ID:       sub.ID,
			UserID:   sub.UserID,
			PlanID:   sub.PlanID,
			PlanName: planName,
			PlanUpgradeRank: func() int {
				if plan != nil {
					return plan.UpgradeRank
				}
				return 0
			}(),
			StartsAt:  sub.StartsAt,
			ExpiresAt: sub.ExpiresAt,
			Status:    sub.Status,
			Limits:    limits,
			CreatedAt: sub.CreatedAt,
			UpdatedAt: sub.UpdatedAt,
		}
	}
	return result, nil
}
