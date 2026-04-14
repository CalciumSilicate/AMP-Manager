package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"ampmanager/internal/billingstate"
	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

var (
	ErrInsufficientFunds = errors.New("余额和订阅额度均不足")
)

type BillingService struct {
	settingRepo repository.BillingSettingRepositoryInterface
	subRepo     repository.UserSubscriptionRepositoryInterface
	planRepo    repository.SubscriptionPlanRepositoryInterface
	eventRepo   repository.BillingEventRepositoryInterface
	userRepo    repository.UserRepositoryInterface
	quotaSvc    *QuotaService
	subSvc      *UserSubscriptionService
}

type RequestBillingResult struct {
	Status                    string
	ChargedSubscriptionMicros int64
	ChargedBalanceMicros      int64
}

type AdmissionRequest struct {
	RequestID           string
	UserID              string
	PricingModel        string
	EstimatedCostMicros int64
}

func NewBillingService() *BillingService {
	return &BillingService{
		settingRepo: repository.NewBillingSettingRepository(),
		subRepo:     repository.NewUserSubscriptionRepository(),
		planRepo:    repository.NewSubscriptionPlanRepository(),
		eventRepo:   repository.NewBillingEventRepository(),
		userRepo:    repository.NewUserRepository(),
		quotaSvc:    NewQuotaService(),
		subSvc:      NewUserSubscriptionService(),
	}
}

func NewBillingServiceWithRepo(
	settingRepo repository.BillingSettingRepositoryInterface,
	subRepo repository.UserSubscriptionRepositoryInterface,
	planRepo repository.SubscriptionPlanRepositoryInterface,
	eventRepo repository.BillingEventRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
) *BillingService {
	return &BillingService{
		settingRepo: settingRepo,
		subRepo:     subRepo,
		planRepo:    planRepo,
		eventRepo:   eventRepo,
		userRepo:    userRepo,
		quotaSvc:    NewQuotaService(),
		subSvc:      NewUserSubscriptionService(),
	}
}

func (s *BillingService) CanStartRequest(userID string) (bool, error) {
	return s.canStartRequestLegacy(userID)
}

func (s *BillingService) canStartRequestLegacy(userID string) (bool, error) {
	setting, err := s.settingRepo.GetByUserID(userID)
	if err != nil {
		return false, err
	}

	sub, err := s.subRepo.GetActiveByUserID(userID)
	if err != nil {
		return false, err
	}

	var subscriptionRemaining int64
	if sub != nil {
		_, limits, err := s.planRepo.GetByID(sub.PlanID)
		if err != nil {
			return false, err
		}
		if len(limits) > 0 {
			subscriptionRemaining = s.calcSubscriptionRemaining(sub, limits)
		}
	}

	balance, err := s.userRepo.GetBalance(userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			balance = 0
		} else {
			return false, err
		}
	}

	sources := []model.BillingSource{setting.PrimarySource, setting.SecondarySource}
	for _, src := range sources {
		switch src {
		case model.BillingSourceSubscription:
			if subscriptionRemaining > 0 {
				return true, nil
			}
		case model.BillingSourceBalance:
			if balance > 0 {
				return true, nil
			}
		}
	}

	return false, nil
}

func (s *BillingService) ReserveRequest(req AdmissionRequest) (bool, error) {
	if runtime := billingstate.Get(); runtime != nil {
		if err := runtime.ReserveRequest(context.Background(), req.RequestID, req.UserID, req.EstimatedCostMicros); err != nil {
			if errors.Is(err, billingstate.ErrInsufficientBudget) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	return s.canStartRequestLegacy(req.UserID)
}

func (s *BillingService) calcSubscriptionRemaining(sub *model.UserSubscription, limits []model.SubscriptionPlanLimit) int64 {
	now := time.Now().UTC()
	minRemaining := int64(math.MaxInt64)

	for _, limit := range limits {
		start, end, err := GetWindowBounds(limit.LimitType, limit.WindowMode, now, sub.StartsAt)
		if err != nil {
			continue
		}
		used, err := s.eventRepo.GetUsageInWindow(sub.ID, start, end)
		if err != nil {
			continue
		}
		left := limit.LimitMicros - used
		if left < 0 {
			left = 0
		}
		if left < minRemaining {
			minRemaining = left
		}
	}

	if minRemaining == math.MaxInt64 {
		return 0
	}
	return minRemaining
}

func (s *BillingService) SettleRequestCost(requestLogID, userID string, costMicros int64) error {
	result, err := s.SettleRequestCostResult(requestLogID, userID, costMicros)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return s.ApplyBillingResult(requestLogID, result)
}

func (s *BillingService) ApplyBillingResult(requestLogID string, result *RequestBillingResult) error {
	if result == nil {
		return nil
	}
	return s.markBillingStatus(requestLogID, result.Status, result.ChargedSubscriptionMicros, result.ChargedBalanceMicros)
}

func (s *BillingService) SettleRequestCostResult(requestLogID, userID string, costMicros int64) (*RequestBillingResult, error) {
	if runtime := billingstate.Get(); runtime != nil {
		result, err := runtime.SettleRequest(context.Background(), requestLogID, userID, costMicros)
		if err != nil {
			if errors.Is(err, billingstate.ErrReservationNotFound) {
				return s.settleRequestCostLegacy(requestLogID, userID, costMicros)
			}
			return nil, err
		}

		status := "free"
		if result != nil {
			status = result.Status
			if status == "" {
				if costMicros == 0 {
					status = "free"
				} else {
					status = "settled"
				}
			}
		}
		chargedSub := int64(0)
		chargedBal := int64(0)
		if result != nil {
			chargedSub = result.ChargedSubscriptionMicros
			chargedBal = result.ChargedBalanceMicros
		}
		return &RequestBillingResult{
			Status:                    status,
			ChargedSubscriptionMicros: chargedSub,
			ChargedBalanceMicros:      chargedBal,
		}, nil
	}
	return s.settleRequestCostLegacy(requestLogID, userID, costMicros)
}

func (s *BillingService) settleRequestCostLegacy(requestLogID, userID string, costMicros int64) (*RequestBillingResult, error) {
	if costMicros < 0 {
		return nil, fmt.Errorf("billing: invalid negative cost %d", costMicros)
	}
	if costMicros == 0 {
		return &RequestBillingResult{Status: "free"}, nil
	}

	db := database.GetDB()

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("billing: begin tx: %w", err)
	}
	defer tx.Rollback()

	setting, err := s.queryBillingSetting(tx, userID)
	if err != nil {
		return nil, fmt.Errorf("billing: query setting: %w", err)
	}

	sub, err := s.queryActiveSubscription(tx, userID)
	if err != nil {
		return nil, fmt.Errorf("billing: query subscription: %w", err)
	}

	var subscriptionRemaining int64
	if sub != nil {
		subscriptionRemaining, err = s.calcSubscriptionRemainingTx(tx, sub)
		if err != nil {
			return nil, fmt.Errorf("billing: calc subscription remaining: %w", err)
		}
	}

	balance, err := s.queryBalance(tx, userID)
	if err != nil {
		return nil, fmt.Errorf("billing: query balance: %w", err)
	}

	var chargedSubscription, chargedBalance int64
	remaining := costMicros

	sources := []model.BillingSource{setting.PrimarySource, setting.SecondarySource}
	for _, src := range sources {
		if remaining <= 0 {
			break
		}
		switch src {
		case model.BillingSourceSubscription:
			if sub != nil && subscriptionRemaining > 0 {
				charge := remaining
				if charge > subscriptionRemaining {
					charge = subscriptionRemaining
				}
				chargedSubscription += charge
				remaining -= charge
				subscriptionRemaining -= charge
			}
		case model.BillingSourceBalance:
			if balance > 0 {
				charge := remaining
				if charge > balance {
					charge = balance
				}
				chargedBalance += charge
				remaining -= charge
				balance -= charge
			}
		}
	}

	// If remaining > 0, funds were insufficient. Do NOT force-charge — this would cause negative balance.
	// The overuse amount is recorded in billing_status but not charged.

	now := time.Now().UTC()

	if chargedSubscription > 0 && sub != nil {
		if err := s.insertBillingEvent(tx, requestLogID, userID, &sub.ID, model.BillingSourceSubscription, "charge", chargedSubscription, now); err != nil {
			return nil, fmt.Errorf("billing: insert subscription event: %w", err)
		}
	}

	if chargedBalance > 0 {
		if err := s.insertBillingEvent(tx, requestLogID, userID, nil, model.BillingSourceBalance, "charge", chargedBalance, now); err != nil {
			return nil, fmt.Errorf("billing: insert balance event: %w", err)
		}
		if _, err := tx.Exec(
			`UPDATE users SET balance_micros = CASE WHEN balance_micros >= ? THEN balance_micros - ? ELSE 0 END, updated_at = ? WHERE id = ?`,
			chargedBalance, chargedBalance, now, userID,
		); err != nil {
			return nil, fmt.Errorf("billing: deduct balance: %w", err)
		}
	}

	billingStatus := "settled"
	if costMicros == 0 {
		billingStatus = "free"
	}
	if remaining > 0 {
		billingStatus = "overuse"
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("billing: commit: %w", err)
	}

	log.Debugf("billing: settled request %s user %s cost=%d sub=%d bal=%d status=%s",
		requestLogID, userID, costMicros, chargedSubscription, chargedBalance, billingStatus)
	return &RequestBillingResult{
		Status:                    billingStatus,
		ChargedSubscriptionMicros: chargedSubscription,
		ChargedBalanceMicros:      chargedBalance,
	}, nil
}

func (s *BillingService) GetBillingState(userID string) (*model.BillingStateResponse, error) {
	setting, err := s.settingRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	balance, err := s.userRepo.GetBalance(userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			balance = 0
		} else {
			return nil, err
		}
	}

	subSvc := s.subSvc
	subResp, err := subSvc.GetActive(userID)
	if err != nil {
		return nil, err
	}

	quotaSvc := s.quotaSvc
	_, windows, err := quotaSvc.GetSubscriptionRemaining(userID)
	if err != nil {
		return nil, err
	}

	return &model.BillingStateResponse{
		BalanceMicros:   balance,
		BalanceUsd:      fmt.Sprintf("%.6f", float64(balance)/1e6),
		Subscription:    subResp,
		Windows:         windows,
		PrimarySource:   setting.PrimarySource,
		SecondarySource: setting.SecondarySource,
	}, nil
}

func (s *BillingService) queryBillingSetting(tx *sql.Tx, userID string) (*model.UserBillingSetting, error) {
	setting := &model.UserBillingSetting{}
	err := tx.QueryRow(
		`SELECT user_id, primary_source, secondary_source FROM user_billing_settings WHERE user_id = ?`,
		userID,
	).Scan(&setting.UserID, &setting.PrimarySource, &setting.SecondarySource)
	if err == sql.ErrNoRows {
		return &model.UserBillingSetting{
			UserID:          userID,
			PrimarySource:   model.BillingSourceSubscription,
			SecondarySource: model.BillingSourceBalance,
		}, nil
	}
	return setting, err
}

func (s *BillingService) queryActiveSubscription(tx *sql.Tx, userID string) (*model.UserSubscription, error) {
	sub := &model.UserSubscription{}
	now := time.Now().UTC()
	err := tx.QueryRow(
		`SELECT id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at 
		 FROM user_subscriptions 
		 WHERE user_id = ? AND status = 'active' AND (expires_at IS NULL OR expires_at > ?)`,
		userID, now,
	).Scan(&sub.ID, &sub.UserID, &sub.PlanID, &sub.StartsAt, &sub.ExpiresAt, &sub.Status, &sub.CreatedAt, &sub.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

func (s *BillingService) queryBalance(tx *sql.Tx, userID string) (int64, error) {
	var balance int64
	err := tx.QueryRow(`SELECT balance_micros FROM users WHERE id = ?`, userID).Scan(&balance)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return balance, err
}

func (s *BillingService) calcSubscriptionRemainingTx(tx *sql.Tx, sub *model.UserSubscription) (int64, error) {
	rows, err := tx.Query(
		`SELECT id, plan_id, limit_type, window_mode, limit_micros, created_at, updated_at 
		 FROM subscription_plan_limits WHERE plan_id = ? ORDER BY limit_type`,
		sub.PlanID,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var limits []model.SubscriptionPlanLimit
	for rows.Next() {
		l := model.SubscriptionPlanLimit{}
		if err := rows.Scan(&l.ID, &l.PlanID, &l.LimitType, &l.WindowMode, &l.LimitMicros, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return 0, err
		}
		limits = append(limits, l)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(limits) == 0 {
		return 0, nil
	}

	now := time.Now().UTC()
	minRemaining := int64(math.MaxInt64)

	for _, limit := range limits {
		start, end, err := GetWindowBounds(limit.LimitType, limit.WindowMode, now, sub.StartsAt)
		if err != nil {
			return 0, err
		}

		var chargeSum, refundSum sql.NullInt64
		err = tx.QueryRow(
			`SELECT 
				COALESCE(SUM(CASE WHEN event_type = 'charge' THEN amount_micros ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN event_type = 'refund' THEN amount_micros ELSE 0 END), 0)
			 FROM billing_events 
			 WHERE user_subscription_id = ? AND source = 'subscription' AND created_at >= ? AND created_at < ?`,
			sub.ID, start, end,
		).Scan(&chargeSum, &refundSum)
		if err != nil {
			return 0, err
		}

		used := chargeSum.Int64 - refundSum.Int64
		left := limit.LimitMicros - used
		if left < 0 {
			left = 0
		}
		if left < minRemaining {
			minRemaining = left
		}
	}

	if minRemaining == math.MaxInt64 {
		return 0, nil
	}
	return minRemaining, nil
}

func (s *BillingService) insertBillingEvent(tx *sql.Tx, requestLogID, userID string, userSubscriptionID *string, source model.BillingSource, eventType string, amount int64, now time.Time) error {
	id := uuid.New().String()
	_, err := tx.Exec(
		`INSERT INTO billing_events (id, request_log_id, user_id, user_subscription_id, source, event_type, amount_micros, created_at) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, requestLogID, userID, userSubscriptionID, source, eventType, amount, now,
	)
	return err
}

func (s *BillingService) markBillingStatus(requestLogID, status string, subMicros, balMicros int64) error {
	db := database.GetDB()
	return updateRequestLogBilling(db, requestLogID, status, subMicros, balMicros)
}

func updateRequestLogBilling(exec interface {
	Exec(query string, args ...any) (sql.Result, error)
}, requestLogID, status string, subMicros, balMicros int64) error {
	_, err := exec.Exec(
		`UPDATE request_logs
		 SET charged_subscription_micros = ?, charged_balance_micros = ?, billing_status = ?
		 WHERE id = ?
		   AND (
		     COALESCE(charged_subscription_micros, -1) <> ?
		     OR COALESCE(charged_balance_micros, -1) <> ?
		     OR COALESCE(billing_status, '') <> ?
		   )`,
		subMicros, balMicros, status, requestLogID,
		subMicros, balMicros, status,
	)
	return err
}
