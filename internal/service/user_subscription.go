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
	subRepo  repository.UserSubscriptionRepositoryInterface
	planRepo repository.SubscriptionPlanRepositoryInterface
}

func NewUserSubscriptionService() *UserSubscriptionService {
	return &UserSubscriptionService{
		subRepo:  repository.NewUserSubscriptionRepository(),
		planRepo: repository.NewSubscriptionPlanRepository(),
	}
}

func NewUserSubscriptionServiceWithRepo(
	subRepo repository.UserSubscriptionRepositoryInterface,
	planRepo repository.SubscriptionPlanRepositoryInterface,
) *UserSubscriptionService {
	return &UserSubscriptionService{
		subRepo:  subRepo,
		planRepo: planRepo,
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

	var (
		existingID     string
		existingPlanID string
		existingExpiry *time.Time
	)
	now := time.Now().UTC()
	err = tx.QueryRow(
		`SELECT id, plan_id, expires_at FROM user_subscriptions WHERE user_id = ? AND status = 'active' AND (expires_at IS NULL OR expires_at > ?)`,
		userID, now,
	).Scan(&existingID, &existingPlanID, &existingExpiry)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if existingID != "" {
		if existingExpiry == nil {
			return nil, ErrPermanentSubscription
		}
		if existingPlanID != req.PlanID {
			return nil, ErrDifferentPlanActive
		}
		if _, err := tx.Exec(
			`UPDATE user_subscriptions SET expires_at = ?, updated_at = ? WHERE id = ?`,
			req.ExpiresAt, now, existingID,
		); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("assign subscription: commit: %w", err)
		}
		sub, err := s.subRepo.GetActiveByUserID(userID)
		if err != nil {
			return nil, err
		}
		return buildUserSubscriptionResponse(sub, plan.Name, limits), nil
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

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("assign subscription: commit: %w", err)
	}

	if runtime := billingstate.Get(); runtime != nil {
		if err := runtime.RefreshUserState(context.Background(), userID); err != nil {
			return nil, err
		}
	}

	return buildUserSubscriptionResponse(sub, plan.Name, limits), nil
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

	return buildUserSubscriptionResponse(sub, planName, limits), nil
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
		result[i] = buildUserSubscriptionResponse(sub, planName, limits)
	}
	return result, nil
}

func buildUserSubscriptionResponse(sub *model.UserSubscription, planName string, limits []model.SubscriptionPlanLimit) *model.UserSubscriptionResponse {
	if sub == nil {
		return nil
	}

	return &model.UserSubscriptionResponse{
		ID:        sub.ID,
		UserID:    sub.UserID,
		PlanID:    sub.PlanID,
		PlanName:  planName,
		StartsAt:  sub.StartsAt,
		ExpiresAt: sub.ExpiresAt,
		Status:    sub.Status,
		Limits:    limits,
		CreatedAt: sub.CreatedAt,
		UpdatedAt: sub.UpdatedAt,
	}
}
