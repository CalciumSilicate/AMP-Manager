package service

import (
	"context"
	"database/sql"
	"fmt"
	"math"
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
	entitlementRepo   repository.SubscriptionEntitlementRepositoryInterface
	planRepo          repository.SubscriptionPlanRepositoryInterface
	runtimeRepo       *repository.SubscriptionRuntimeRepository
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
		entitlementRepo: repository.NewSubscriptionEntitlementRepository(),
		planRepo:        repository.NewSubscriptionPlanRepository(),
		runtimeRepo:     repository.NewSubscriptionRuntimeRepository(),
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
	return s.GrantSubscriptionTxWithSource(tx, userID, planID, durationDays, model.SubscriptionEntitlementSourcePurchase, "", now)
}

func (s *RewardGrantService) GrantSubscriptionTxWithSource(
	tx *sql.Tx,
	userID, planID string,
	durationDays int,
	sourceType model.SubscriptionEntitlementSourceType,
	sourceRefID string,
	now time.Time,
) (*model.UserSubscription, error) {
	if tx == nil {
		return nil, fmt.Errorf("grant subscription: nil tx")
	}
	if planID == "" || durationDays <= 0 {
		return nil, fmt.Errorf("grant subscription: invalid reward config")
	}
	plan, _, err := s.planRepo.GetByID(planID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound
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
		if err := s.entitlementRepo.CreateTx(tx, &model.SubscriptionEntitlement{
			UserID:                 userID,
			PlanID:                 planID,
			SourceType:             sourceType,
			SourceRefID:            strings.TrimSpace(sourceRefID),
			ValuationCnyCentPerDay: plan.UpgradeValuationCnyCentPerDay,
			StartsAt:               now,
			ExpiresAt:              sub.ExpiresAt,
			Status:                 model.SubscriptionEntitlementStatusActive,
		}); err != nil {
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
	if err := s.entitlementRepo.CreateTx(tx, &model.SubscriptionEntitlement{
		UserID:                 userID,
		PlanID:                 planID,
		SourceType:             sourceType,
		SourceRefID:            strings.TrimSpace(sourceRefID),
		ValuationCnyCentPerDay: plan.UpgradeValuationCnyCentPerDay,
		StartsAt:               base,
		ExpiresAt:              &expiresAt,
		Status:                 model.SubscriptionEntitlementStatusActive,
	}); err != nil {
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

func (s *RewardGrantService) OverwriteSubscriptionTx(
	tx *sql.Tx,
	userID, planID string,
	durationDays int,
	sourceType model.SubscriptionEntitlementSourceType,
	sourceRefID string,
	now time.Time,
) (*model.UserSubscription, error) {
	if tx == nil {
		return nil, fmt.Errorf("overwrite subscription: nil tx")
	}
	if planID == "" || durationDays <= 0 {
		return nil, fmt.Errorf("overwrite subscription: invalid reward config")
	}
	plan, _, err := s.planRepo.GetByID(planID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound
	}

	activeSub, err := s.getActiveSubscriptionTx(tx, userID, now)
	if err != nil {
		return nil, err
	}
	expiresAt := now.AddDate(0, 0, durationDays)
	if activeSub == nil {
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
			sub.ID, sub.UserID, sub.PlanID, sub.StartsAt, sub.ExpiresAt, sub.Status, sub.CreatedAt, sub.UpdatedAt,
		); err != nil {
			return nil, err
		}
		activeSub = sub
	} else {
		if err := s.entitlementRepo.CancelActiveByUserTx(tx, userID, now); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(
			`UPDATE user_subscriptions
			    SET plan_id = ?, starts_at = ?, expires_at = ?, updated_at = ?
			  WHERE id = ?`,
			planID, now, expiresAt, now, activeSub.ID,
		); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`DELETE FROM subscription_window_state WHERE user_subscription_id = ?`, activeSub.ID); err != nil {
			return nil, err
		}
		if s.runtimeRepo != nil {
			if err := s.runtimeRepo.SupersedeFutureTimelinePhasesTx(tx, activeSub.ID, now); err != nil {
				return nil, err
			}
		}
		activeSub.PlanID = planID
		activeSub.StartsAt = now
		activeSub.ExpiresAt = &expiresAt
		activeSub.UpdatedAt = now
	}

	if err := s.entitlementRepo.CreateTx(tx, &model.SubscriptionEntitlement{
		UserID:                 userID,
		PlanID:                 planID,
		SourceType:             sourceType,
		SourceRefID:            strings.TrimSpace(sourceRefID),
		ValuationCnyCentPerDay: plan.UpgradeValuationCnyCentPerDay,
		StartsAt:               now,
		ExpiresAt:              &expiresAt,
		Status:                 model.SubscriptionEntitlementStatusActive,
	}); err != nil {
		return nil, err
	}
	return activeSub, nil
}

func (s *RewardGrantService) CreateBoostTimelineTx(
	tx *sql.Tx,
	userID, planID string,
	durationDays int,
	sourceType model.SubscriptionEntitlementSourceType,
	sourceRefID string,
	now time.Time,
) (*model.UserSubscription, []*model.SubscriptionTimelinePhase, error) {
	if tx == nil {
		return nil, nil, fmt.Errorf("boost subscription: nil tx")
	}
	if planID == "" || durationDays <= 0 {
		return nil, nil, fmt.Errorf("boost subscription: invalid reward config")
	}

	sourcePlan, sourceLimits, err := s.planRepo.GetByID(planID)
	if err != nil {
		return nil, nil, err
	}
	if sourcePlan == nil {
		return nil, nil, ErrPlanNotFound
	}
	sourceDaily := findPlanLimit(sourceLimits, model.LimitTypeDaily)
	if sourceDaily == nil || sourceDaily.LimitMicros <= 0 {
		return nil, nil, fmt.Errorf("加额商品缺少日额度配置")
	}

	activeSub, err := s.getActiveSubscriptionTx(tx, userID, now)
	if err != nil {
		return nil, nil, err
	}
	if activeSub == nil || activeSub.ExpiresAt == nil {
		return nil, nil, ErrDifferentPlanActive
	}

	_, targetLimits, err := s.planRepo.GetByID(activeSub.PlanID)
	if err != nil {
		return nil, nil, err
	}
	if s.runtimeRepo != nil {
		if resolved, _, _, resolveErr := s.runtimeRepo.ResolveEffectiveLimitsBySubscription(activeSub, targetLimits, now); resolveErr != nil {
			return nil, nil, resolveErr
		} else if len(resolved) > 0 {
			targetLimits = resolved
		}
	}
	targetDaily := findPlanLimit(targetLimits, model.LimitTypeDaily)
	if targetDaily == nil || targetDaily.LimitMicros <= 0 {
		return nil, nil, fmt.Errorf("当前订阅缺少可加额的日额度配置")
	}

	if s.runtimeRepo != nil {
		if err := s.runtimeRepo.SupersedeFutureTimelinePhasesTx(tx, activeSub.ID, now); err != nil {
			return nil, nil, err
		}
	}

	boostedDaily := targetDaily.LimitMicros + sourceDaily.LimitMicros
	sourceDirectEnd := now.AddDate(0, 0, durationDays)
	currentExpiry := activeSub.ExpiresAt.UTC()
	overlapEnd := sourceDirectEnd
	if currentExpiry.Before(overlapEnd) {
		overlapEnd = currentExpiry
	}

	phases := make([]*model.SubscriptionTimelinePhase, 0, 3)
	if overlapEnd.After(now) {
		phases = append(phases, &model.SubscriptionTimelinePhase{
			UserID:             userID,
			UserSubscriptionID: activeSub.ID,
			PlanID:             activeSub.PlanID,
			PhaseType:          model.SubscriptionTimelinePhaseTypeBoost,
			Status:             phaseStatusFromWindow(now, now, overlapEnd),
			SourceType:         string(sourceType),
			SourceRefID:        strings.TrimSpace(sourceRefID),
			DailyLimitMicros:   int64Ptr(boostedDaily),
			FixedResetTime:     targetDaily.FixedResetTime,
			StartsAt:           now,
			EndsAt:             overlapEnd,
		})
	}

	finalExpiresAt := currentExpiry
	if sourceDirectEnd.After(currentExpiry) {
		remainingSeconds := sourceDirectEnd.Sub(currentExpiry).Seconds()
		convertedSeconds := int64(math.Round(remainingSeconds * float64(sourceDaily.LimitMicros) / float64(boostedDaily)))
		if convertedSeconds > 0 {
			convertedEnd := currentExpiry.Add(time.Duration(convertedSeconds) * time.Second)
			finalExpiresAt = convertedEnd
			phases = append(phases, &model.SubscriptionTimelinePhase{
				UserID:             userID,
				UserSubscriptionID: activeSub.ID,
				PlanID:             activeSub.PlanID,
				PhaseType:          model.SubscriptionTimelinePhaseTypeConvertedExtension,
				Status:             phaseStatusFromWindow(now, currentExpiry, convertedEnd),
				SourceType:         string(sourceType),
				SourceRefID:        strings.TrimSpace(sourceRefID),
				DailyLimitMicros:   int64Ptr(boostedDaily),
				FixedResetTime:     targetDaily.FixedResetTime,
				StartsAt:           currentExpiry,
				EndsAt:             convertedEnd,
				FinalExpiresAt:     &convertedEnd,
			})
		}
	} else if currentExpiry.After(sourceDirectEnd) {
		phases = append(phases, &model.SubscriptionTimelinePhase{
			UserID:             userID,
			UserSubscriptionID: activeSub.ID,
			PlanID:             activeSub.PlanID,
			PhaseType:          model.SubscriptionTimelinePhaseTypeRestore,
			Status:             phaseStatusFromWindow(now, sourceDirectEnd, currentExpiry),
			SourceType:         string(sourceType),
			SourceRefID:        strings.TrimSpace(sourceRefID),
			DailyLimitMicros:   int64Ptr(targetDaily.LimitMicros),
			FixedResetTime:     targetDaily.FixedResetTime,
			StartsAt:           sourceDirectEnd,
			EndsAt:             currentExpiry,
			FinalExpiresAt:     &currentExpiry,
		})
	}

	if !finalExpiresAt.Equal(currentExpiry) {
		if _, err := tx.Exec(`UPDATE user_subscriptions SET expires_at = ?, updated_at = ? WHERE id = ?`, finalExpiresAt, now, activeSub.ID); err != nil {
			return nil, nil, err
		}
		activeSub.ExpiresAt = &finalExpiresAt
		activeSub.UpdatedAt = now
	}

	for _, phase := range phases {
		if phase.FinalExpiresAt == nil && activeSub.ExpiresAt != nil {
			phase.FinalExpiresAt = activeSub.ExpiresAt
		}
		if s.runtimeRepo != nil {
			if err := s.runtimeRepo.CreateTimelinePhaseTx(tx, phase); err != nil {
				return nil, nil, err
			}
		}
	}

	return activeSub, phases, nil
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

func findPlanLimit(limits []model.SubscriptionPlanLimit, limitType model.LimitType) *model.SubscriptionPlanLimit {
	for idx := range limits {
		if limits[idx].LimitType == limitType {
			return &limits[idx]
		}
	}
	return nil
}

func phaseStatusFromWindow(now, start, end time.Time) model.SubscriptionTimelinePhaseStatus {
	if !start.After(now) && end.After(now) {
		return model.SubscriptionTimelinePhaseStatusActive
	}
	if end.Before(now) || end.Equal(now) {
		return model.SubscriptionTimelinePhaseStatusCompleted
	}
	return model.SubscriptionTimelinePhaseStatusScheduled
}

func int64Ptr(value int64) *int64 {
	return &value
}
