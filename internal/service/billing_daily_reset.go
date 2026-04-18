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

var ErrBillingDailyResetInvalidState = errors.New("invalid billing daily reset state")

type billingDailyResetRuleError struct {
	message string
}

func (e *billingDailyResetRuleError) Error() string {
	return e.message
}

func (e *billingDailyResetRuleError) Is(target error) bool {
	return target == ErrBillingDailyResetInvalidState
}

type billingDailyResetEvaluation struct {
	config       model.BillingDailyResetConfigResponse
	state        model.BillingDailyResetState
	window       *model.WindowRemaining
	subscription *model.UserSubscriptionResponse
	now          time.Time
}

func newBillingDailyResetRuleError(message string) error {
	return &billingDailyResetRuleError{message: message}
}

func (s *BillingService) buildBillingDailyResetState(
	userID string,
	subscription *model.UserSubscriptionResponse,
	windows []model.WindowRemaining,
) (model.BillingDailyResetState, error) {
	evaluation, err := s.evaluateBillingDailyReset(userID, subscription, windows)
	if err != nil {
		return model.BillingDailyResetState{}, err
	}
	return evaluation.state, nil
}

func (s *BillingService) ResetDailyBilling(userID string) (*model.BillingStateResponse, error) {
	subscription, err := s.subSvc.GetActive(userID)
	if err != nil {
		return nil, err
	}

	_, windows, err := s.quotaSvc.GetSubscriptionRemaining(userID)
	if err != nil {
		return nil, err
	}

	evaluation, err := s.evaluateBillingDailyReset(userID, subscription, windows)
	if err != nil {
		return nil, err
	}
	if !evaluation.state.Allowed || evaluation.window == nil || evaluation.subscription == nil {
		return nil, newBillingDailyResetRuleError(evaluation.state.Message)
	}

	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	sub, err := s.queryActiveSubscription(tx, userID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, newBillingDailyResetRuleError("当前没有可重置的活跃订阅")
	}
	if sub.ExpiresAt == nil {
		return nil, newBillingDailyResetRuleError("当前订阅未设置到期时间，无法重置")
	}

	if err := s.revalidateBillingDailyReset(tx, userID, sub, evaluation); err != nil {
		return nil, err
	}

	usedMicrosBeforeReset, err := queryBillingUsageInWindowTx(
		tx,
		sub.ID,
		model.LimitTypeDaily,
		model.WindowModeFixed,
		evaluation.window.WindowStart,
		evaluation.window.WindowEnd,
	)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	expiresAtBefore := sub.ExpiresAt.UTC()
	expiresAtAfter := expiresAtBefore.AddDate(0, 0, -1)

	if _, err := tx.Exec(
		`INSERT INTO billing_daily_reset_records (
			id, user_id, user_subscription_id, window_start, window_end,
			used_micros_before_reset, expires_at_before, expires_at_after, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(),
		userID,
		sub.ID,
		evaluation.window.WindowStart.UTC(),
		evaluation.window.WindowEnd.UTC(),
		usedMicrosBeforeReset,
		expiresAtBefore,
		expiresAtAfter,
		now,
	); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(
		`UPDATE user_subscriptions SET expires_at = ?, updated_at = ? WHERE id = ?`,
		expiresAtAfter,
		now,
		sub.ID,
	); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(
		`UPDATE subscription_window_state
		 SET used_micros = 0,
		     remaining_micros = CASE WHEN limit_micros > reserved_micros THEN limit_micros - reserved_micros ELSE 0 END,
		     revision = revision + 1,
		     updated_at = ?
		 WHERE user_subscription_id = ?
		   AND limit_type = ?
		   AND window_mode = ?
		   AND window_start = ?
		   AND window_end = ?`,
		now,
		sub.ID,
		model.LimitTypeDaily,
		model.WindowModeFixed,
		evaluation.window.WindowStart.UTC(),
		evaluation.window.WindowEnd.UTC(),
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if runtime := billingstate.Get(); runtime != nil {
		if err := runtime.RefreshUserState(context.Background(), userID); err != nil {
			return nil, err
		}
	}
	state, err := s.GetBillingState(userID)
	if err != nil {
		return nil, err
	}
	if state.Subscription != nil {
		state.Subscription.ExpiresAt = &expiresAtAfter
	}
	for idx := range state.Windows {
		window := &state.Windows[idx]
		if window.LimitType == model.LimitTypeDaily &&
			window.WindowMode == model.WindowModeFixed &&
			window.WindowStart.Equal(evaluation.window.WindowStart) &&
			window.WindowEnd.Equal(evaluation.window.WindowEnd) {
			window.UsedMicros = 0
			window.LeftMicros = window.LimitMicros
		}
	}
	state.DailyReset.Allowed = false
	state.DailyReset.CurrentUsagePercent = 0
	state.DailyReset.Message = fmt.Sprintf("今日用量需高于 %d%% 才可重置", evaluation.config.UsageThresholdPercent)
	location, err := s.quotaSvc.getSiteLocation()
	if err == nil {
		dayStart, dayEnd := getDayBounds(now, location)
		if usedToday, countErr := repository.NewBillingDailyResetRepository().CountByUserBetween(userID, dayStart, dayEnd); countErr == nil {
			state.DailyReset.UsedToday = usedToday
			if usedToday >= evaluation.config.DailyLimit {
				state.DailyReset.Message = fmt.Sprintf("今日最多可重置 %d 次", evaluation.config.DailyLimit)
			}
		}
	}
	return state, nil
}

func (s *BillingService) evaluateBillingDailyReset(
	userID string,
	subscription *model.UserSubscriptionResponse,
	windows []model.WindowRemaining,
) (*billingDailyResetEvaluation, error) {
	config, err := NewSystemConfigService().GetBillingDailyResetConfig()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	location, err := s.quotaSvc.getSiteLocation()
	if err != nil {
		return nil, err
	}
	dayStart, dayEnd := getDayBounds(now, location)
	usedToday, err := repository.NewBillingDailyResetRepository().CountByUserBetween(userID, dayStart, dayEnd)
	if err != nil {
		return nil, err
	}

	state := model.BillingDailyResetState{
		Allowed:               false,
		Supported:             false,
		UsedToday:             usedToday,
		DailyLimit:            config.DailyLimit,
		UsageThresholdPercent: config.UsageThresholdPercent,
		MinRemainingDays:      config.MinRemainingDays,
	}

	evaluation := &billingDailyResetEvaluation{
		config:       config,
		state:        state,
		subscription: subscription,
		now:          now,
	}

	if !config.Enabled {
		evaluation.state.Message = "系统未启用今日计费重置"
		return evaluation, nil
	}

	if subscription == nil {
		evaluation.state.Message = "当前没有可重置的活跃订阅"
		return evaluation, nil
	}
	if subscription.ExpiresAt == nil {
		evaluation.state.Message = "当前订阅未设置到期时间，无法重置"
		return evaluation, nil
	}

	window := selectBillingDailyResetWindow(windows)
	if window == nil {
		evaluation.state.Message = "当前订阅没有固定日额度"
		return evaluation, nil
	}

	evaluation.window = window
	evaluation.state.Supported = true
	if window.LimitMicros > 0 {
		evaluation.state.CurrentUsagePercent = (float64(window.UsedMicros) / float64(window.LimitMicros)) * 100
	}

	if config.DailyLimit == 0 {
		evaluation.state.Message = "当前配置不允许重置今日计费"
		return evaluation, nil
	}

	if subscription.ExpiresAt.UTC().Sub(now) <= time.Duration(config.MinRemainingDays)*24*time.Hour {
		evaluation.state.Message = fmt.Sprintf("剩余时长需超过 %d 天", config.MinRemainingDays)
		return evaluation, nil
	}

	if evaluation.state.CurrentUsagePercent <= float64(config.UsageThresholdPercent) {
		evaluation.state.Message = fmt.Sprintf("今日用量需高于 %d%% 才可重置", config.UsageThresholdPercent)
		return evaluation, nil
	}

	if usedToday >= config.DailyLimit {
		evaluation.state.Message = fmt.Sprintf("今日最多可重置 %d 次", config.DailyLimit)
		return evaluation, nil
	}

	evaluation.state.Allowed = true
	evaluation.state.Message = "可重置今日计费"
	return evaluation, nil
}

func (s *BillingService) revalidateBillingDailyReset(
	tx *sql.Tx,
	userID string,
	sub *model.UserSubscription,
	evaluation *billingDailyResetEvaluation,
) error {
	if evaluation.window == nil {
		return newBillingDailyResetRuleError("当前订阅没有固定日额度")
	}
	if !evaluation.config.Enabled {
		return newBillingDailyResetRuleError("系统未启用今日计费重置")
	}
	if sub.ExpiresAt == nil {
		return newBillingDailyResetRuleError("当前订阅未设置到期时间，无法重置")
	}
	if evaluation.config.DailyLimit == 0 {
		return newBillingDailyResetRuleError("当前配置不允许重置今日计费")
	}
	if sub.ExpiresAt.UTC().Sub(time.Now().UTC()) <= time.Duration(evaluation.config.MinRemainingDays)*24*time.Hour {
		return newBillingDailyResetRuleError(fmt.Sprintf("剩余时长需超过 %d 天", evaluation.config.MinRemainingDays))
	}

	countToday, err := queryBillingDailyResetCountTx(tx, userID, evaluation.now, s.quotaSvc)
	if err != nil {
		return err
	}
	if countToday >= evaluation.config.DailyLimit {
		return newBillingDailyResetRuleError(fmt.Sprintf("今日最多可重置 %d 次", evaluation.config.DailyLimit))
	}

	currentUsageMicros, err := queryBillingUsageInWindowTx(
		tx,
		sub.ID,
		model.LimitTypeDaily,
		model.WindowModeFixed,
		evaluation.window.WindowStart,
		evaluation.window.WindowEnd,
	)
	if err != nil {
		return err
	}
	currentUsagePercent := 0.0
	if evaluation.window.LimitMicros > 0 {
		currentUsagePercent = (float64(currentUsageMicros) / float64(evaluation.window.LimitMicros)) * 100
	}
	if currentUsagePercent <= float64(evaluation.config.UsageThresholdPercent) {
		return newBillingDailyResetRuleError(fmt.Sprintf("今日用量需高于 %d%% 才可重置", evaluation.config.UsageThresholdPercent))
	}

	return nil
}

func selectBillingDailyResetWindow(windows []model.WindowRemaining) *model.WindowRemaining {
	var selected *model.WindowRemaining
	highestUsage := -1.0

	for i := range windows {
		window := windows[i]
		if window.LimitType != model.LimitTypeDaily || window.WindowMode != model.WindowModeFixed {
			continue
		}

		usagePercent := 0.0
		if window.LimitMicros > 0 {
			usagePercent = (float64(window.UsedMicros) / float64(window.LimitMicros)) * 100
		}
		if selected == nil || usagePercent > highestUsage {
			candidate := window
			selected = &candidate
			highestUsage = usagePercent
		}
	}

	return selected
}

func getDayBounds(now time.Time, location *time.Location) (time.Time, time.Time) {
	if location == nil {
		location = time.UTC
	}
	localNow := now.In(location)
	startLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	endLocal := startLocal.AddDate(0, 0, 1)
	return startLocal.UTC(), endLocal.UTC()
}

func queryBillingDailyResetCountTx(tx *sql.Tx, userID string, now time.Time, quotaSvc *QuotaService) (int, error) {
	location, err := quotaSvc.getSiteLocation()
	if err != nil {
		return 0, err
	}
	dayStart, dayEnd := getDayBounds(now, location)
	var count int
	err = tx.QueryRow(
		`SELECT COUNT(*) FROM billing_daily_reset_records WHERE user_id = ? AND created_at >= ? AND created_at < ?`,
		userID,
		dayStart,
		dayEnd,
	).Scan(&count)
	return count, err
}

func queryBillingUsageInWindowTx(
	tx *sql.Tx,
	userSubscriptionID string,
	limitType model.LimitType,
	windowMode model.WindowMode,
	windowStart time.Time,
	windowEnd time.Time,
) (int64, error) {
	effectiveStart := windowStart.UTC()
	if limitType == model.LimitTypeDaily && windowMode == model.WindowModeFixed {
		var latestResetAt sql.NullTime
		err := tx.QueryRow(
			`SELECT created_at
			 FROM billing_daily_reset_records
			 WHERE user_subscription_id = ? AND created_at >= ? AND created_at < ?
			 ORDER BY created_at DESC
			 LIMIT 1`,
			userSubscriptionID,
			windowStart.UTC(),
			windowEnd.UTC(),
		).Scan(&latestResetAt)
		if err != nil && err != sql.ErrNoRows {
			return 0, err
		}
		if latestResetAt.Valid && latestResetAt.Time.After(effectiveStart) {
			effectiveStart = latestResetAt.Time.UTC()
		}
	}

	var chargeSum, refundSum sql.NullInt64
	err := tx.QueryRow(
		`SELECT
			COALESCE(SUM(CASE WHEN event_type = 'charge' THEN amount_micros ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN event_type = 'refund' THEN amount_micros ELSE 0 END), 0)
		 FROM billing_events
		 WHERE user_subscription_id = ? AND source = 'subscription' AND created_at >= ? AND created_at < ?`,
		userSubscriptionID,
		effectiveStart,
		windowEnd.UTC(),
	).Scan(&chargeSum, &refundSum)
	if err != nil {
		return 0, err
	}
	return chargeSum.Int64 - refundSum.Int64, nil
}
