package repository

import (
	"database/sql"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

type SubscriptionRuntimeRepository struct{}

func NewSubscriptionRuntimeRepository() *SubscriptionRuntimeRepository {
	return &SubscriptionRuntimeRepository{}
}

func (r *SubscriptionRuntimeRepository) CreateRechargeHistoryTx(tx *sql.Tx, item *model.SubscriptionRechargeHistory) error {
	if tx == nil {
		return sql.ErrTxDone
	}
	now := time.Now().UTC()
	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	_, err := tx.Exec(
		`INSERT INTO subscription_recharge_history (
			id, user_id, user_subscription_id, plan_id, mode, status, source_type, source_ref_id,
			source_daily_limit_micros, target_daily_limit_before_micros, peak_daily_limit_micros,
			target_expires_at_before, target_expires_at_after, preview_json, confirmed_at, applied_at,
			legacy_source, legacy_ref_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.UserID, item.UserSubscriptionID, item.PlanID, item.Mode, item.Status, item.SourceType, item.SourceRefID,
		item.SourceDailyLimitMicros, item.TargetDailyLimitBeforeMicros, item.PeakDailyLimitMicros,
		item.TargetExpiresAtBefore, item.TargetExpiresAtAfter, item.PreviewJSON, item.ConfirmedAt, item.AppliedAt,
		item.LegacySource, item.LegacyRefID, item.CreatedAt, item.UpdatedAt,
	)
	return err
}

func (r *SubscriptionRuntimeRepository) CreateTimelinePhaseTx(tx *sql.Tx, phase *model.SubscriptionTimelinePhase) error {
	if tx == nil {
		return sql.ErrTxDone
	}
	now := time.Now().UTC()
	if phase.ID == "" {
		phase.ID = uuid.New().String()
	}
	if phase.CreatedAt.IsZero() {
		phase.CreatedAt = now
	}
	phase.UpdatedAt = now

	var fixedResetMinute any
	if phase.FixedResetTime != nil {
		minute, err := model.ParseFixedResetTime(*phase.FixedResetTime)
		if err != nil {
			return err
		}
		fixedResetMinute = minute
	}

	_, err := tx.Exec(
		`INSERT INTO subscription_timeline_phases (
			id, user_id, user_subscription_id, plan_id, phase_type, status, source_type, source_ref_id,
			daily_limit_micros, weekly_limit_micros, monthly_limit_micros, rolling_5h_limit_micros, total_limit_micros,
			fixed_reset_minute, starts_at, ends_at, final_expires_at, preview_json, applied_at,
			legacy_source, legacy_ref_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		phase.ID, phase.UserID, phase.UserSubscriptionID, phase.PlanID, phase.PhaseType, phase.Status, phase.SourceType, phase.SourceRefID,
		phase.DailyLimitMicros, phase.WeeklyLimitMicros, phase.MonthlyLimitMicros, phase.Rolling5hLimitMicros, phase.TotalLimitMicros,
		fixedResetMinute, phase.StartsAt, phase.EndsAt, phase.FinalExpiresAt, phase.PreviewJSON, phase.AppliedAt,
		phase.LegacySource, phase.LegacyRefID, phase.CreatedAt, phase.UpdatedAt,
	)
	return err
}

func (r *SubscriptionRuntimeRepository) SupersedeFutureTimelinePhasesTx(tx *sql.Tx, userSubscriptionID string, from time.Time) error {
	if tx == nil {
		return sql.ErrTxDone
	}
	_, err := tx.Exec(
		`UPDATE subscription_timeline_phases
		    SET status = ?, updated_at = ?
		  WHERE user_subscription_id = ?
		    AND status IN (?, ?)
		    AND ends_at >= ?`,
		model.SubscriptionTimelinePhaseStatusSuperseded,
		time.Now().UTC(),
		userSubscriptionID,
		model.SubscriptionTimelinePhaseStatusScheduled,
		model.SubscriptionTimelinePhaseStatusActive,
		from.UTC(),
	)
	return err
}

func (r *SubscriptionRuntimeRepository) ListTimelinePhasesBySubscriptionID(userSubscriptionID string) ([]*model.SubscriptionTimelinePhase, error) {
	return r.listTimelinePhases(userSubscriptionID, nil)
}

func (r *SubscriptionRuntimeRepository) ListActiveTimelinePhasesBySubscriptionID(userSubscriptionID string, now time.Time) ([]*model.SubscriptionTimelinePhase, error) {
	return r.listTimelinePhases(userSubscriptionID, &now)
}

func (r *SubscriptionRuntimeRepository) listTimelinePhases(userSubscriptionID string, activeAt *time.Time) ([]*model.SubscriptionTimelinePhase, error) {
	query := `SELECT id, user_id, user_subscription_id, plan_id, phase_type, status, source_type, source_ref_id,
	                 daily_limit_micros, weekly_limit_micros, monthly_limit_micros, rolling_5h_limit_micros, total_limit_micros,
	                 fixed_reset_minute, starts_at, ends_at, final_expires_at, preview_json, applied_at,
	                 legacy_source, legacy_ref_id, created_at, updated_at
	            FROM subscription_timeline_phases
	           WHERE user_subscription_id = ?`
	args := []any{userSubscriptionID}
	if activeAt != nil {
		query += ` AND status IN (?, ?) AND starts_at <= ? AND ends_at > ?`
		args = append(args, model.SubscriptionTimelinePhaseStatusScheduled, model.SubscriptionTimelinePhaseStatusActive, activeAt.UTC(), activeAt.UTC())
	} else {
		query += ` AND status <> ?`
		args = append(args, model.SubscriptionTimelinePhaseStatusCancelled)
	}
	query += ` ORDER BY starts_at ASC, created_at ASC`

	rows, err := database.GetDB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.SubscriptionTimelinePhase
	for rows.Next() {
		item, err := scanTimelinePhase(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SubscriptionRuntimeRepository) ResolveEffectiveLimitsBySubscription(
	sub *model.UserSubscription,
	baseLimits []model.SubscriptionPlanLimit,
	now time.Time,
) ([]model.SubscriptionPlanLimit, []*model.SubscriptionTimelinePhase, *model.SubscriptionTimelinePhase, error) {
	if sub == nil {
		return nil, nil, nil, nil
	}

	phases, err := r.ListTimelinePhasesBySubscriptionID(sub.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	var active *model.SubscriptionTimelinePhase
	for _, phase := range phases {
		if phase == nil {
			continue
		}
		if phase.Status == model.SubscriptionTimelinePhaseStatusSuperseded || phase.Status == model.SubscriptionTimelinePhaseStatusCancelled {
			continue
		}
		if !phase.StartsAt.After(now.UTC()) && phase.EndsAt.After(now.UTC()) {
			candidate := phase
			active = candidate
		}
	}

	resolved := append([]model.SubscriptionPlanLimit(nil), baseLimits...)
	if active == nil {
		return resolved, phases, nil, nil
	}
	resolved = overlayPlanLimits(resolved, active)
	return resolved, phases, active, nil
}

func overlayPlanLimits(base []model.SubscriptionPlanLimit, phase *model.SubscriptionTimelinePhase) []model.SubscriptionPlanLimit {
	if phase == nil {
		return base
	}

	type overlay struct {
		limitType      model.LimitType
		limitMicros    *int64
		fixedResetTime *string
	}
	overlays := []overlay{
		{limitType: model.LimitTypeDaily, limitMicros: phase.DailyLimitMicros, fixedResetTime: phase.FixedResetTime},
		{limitType: model.LimitTypeWeekly, limitMicros: phase.WeeklyLimitMicros},
		{limitType: model.LimitTypeMonthly, limitMicros: phase.MonthlyLimitMicros},
		{limitType: model.LimitTypeRolling5h, limitMicros: phase.Rolling5hLimitMicros},
		{limitType: model.LimitTypeTotal, limitMicros: phase.TotalLimitMicros},
	}

	result := append([]model.SubscriptionPlanLimit(nil), base...)
	for _, item := range overlays {
		if item.limitMicros == nil {
			continue
		}
		applied := false
		for idx := range result {
			if result[idx].LimitType != item.limitType {
				continue
			}
			result[idx].LimitMicros = *item.limitMicros
			if item.limitType == model.LimitTypeDaily && item.fixedResetTime != nil {
				result[idx].FixedResetTime = item.fixedResetTime
			}
			applied = true
			break
		}
		if applied {
			continue
		}
		windowMode := model.WindowModeFixed
		if item.limitType == model.LimitTypeRolling5h {
			windowMode = model.WindowModeSliding
		}
		result = append(result, model.SubscriptionPlanLimit{
			ID:             "",
			PlanID:         phase.PlanID,
			LimitType:      item.limitType,
			WindowMode:     windowMode,
			LimitMicros:    *item.limitMicros,
			FixedResetTime: item.fixedResetTime,
		})
	}
	return result
}

func scanTimelinePhase(scanner interface {
	Scan(dest ...any) error
}) (*model.SubscriptionTimelinePhase, error) {
	item := &model.SubscriptionTimelinePhase{}
	var daily, weekly, monthly, rolling5h, total sql.NullInt64
	var fixedResetMinute sql.NullInt64
	var finalExpiresAt, appliedAt sql.NullTime
	err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.UserSubscriptionID,
		&item.PlanID,
		&item.PhaseType,
		&item.Status,
		&item.SourceType,
		&item.SourceRefID,
		&daily,
		&weekly,
		&monthly,
		&rolling5h,
		&total,
		&fixedResetMinute,
		&item.StartsAt,
		&item.EndsAt,
		&finalExpiresAt,
		&item.PreviewJSON,
		&appliedAt,
		&item.LegacySource,
		&item.LegacyRefID,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if daily.Valid {
		value := daily.Int64
		item.DailyLimitMicros = &value
	}
	if weekly.Valid {
		value := weekly.Int64
		item.WeeklyLimitMicros = &value
	}
	if monthly.Valid {
		value := monthly.Int64
		item.MonthlyLimitMicros = &value
	}
	if rolling5h.Valid {
		value := rolling5h.Int64
		item.Rolling5hLimitMicros = &value
	}
	if total.Valid {
		value := total.Int64
		item.TotalLimitMicros = &value
	}
	if fixedResetMinute.Valid {
		minute := int(fixedResetMinute.Int64)
		item.FixedResetTime = model.FormatFixedResetTime(&minute)
	}
	if finalExpiresAt.Valid {
		value := finalExpiresAt.Time.UTC()
		item.FinalExpiresAt = &value
	}
	if appliedAt.Valid {
		value := appliedAt.Time.UTC()
		item.AppliedAt = &value
	}
	return item, nil
}
