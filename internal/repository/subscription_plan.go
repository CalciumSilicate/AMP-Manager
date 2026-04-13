package repository

import (
	"database/sql"
	"errors"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

var ErrPlanNotFound = errors.New("订阅套餐不存在")

type SubscriptionPlanRepositoryInterface interface {
	Create(plan *model.SubscriptionPlan, limits []model.SubscriptionPlanLimit) error
	GetByID(id string) (*model.SubscriptionPlan, []model.SubscriptionPlanLimit, error)
	List() ([]*model.SubscriptionPlan, map[string][]model.SubscriptionPlanLimit, error)
	Update(id string, plan *model.SubscriptionPlan, limits []model.SubscriptionPlanLimit) error
	Delete(id string) error
	SetEnabled(id string, enabled bool) error
}

var _ SubscriptionPlanRepositoryInterface = (*SubscriptionPlanRepository)(nil)

type SubscriptionPlanRepository struct{}

func NewSubscriptionPlanRepository() *SubscriptionPlanRepository {
	return &SubscriptionPlanRepository{}
}

func nullableFixedResetMinute(limit model.SubscriptionPlanLimit) (interface{}, error) {
	if limit.FixedResetTime == nil {
		return nil, nil
	}

	minute, err := model.ParseFixedResetTime(*limit.FixedResetTime)
	if err != nil {
		return nil, err
	}

	return minute, nil
}

func (r *SubscriptionPlanRepository) Create(plan *model.SubscriptionPlan, limits []model.SubscriptionPlanLimit) error {
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	plan.ID = uuid.New().String()
	plan.CreatedAt = time.Now().UTC()
	plan.UpdatedAt = plan.CreatedAt

	_, err = tx.Exec(
		`INSERT INTO subscription_plans (id, name, description, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		plan.ID, plan.Name, plan.Description, plan.Enabled, plan.CreatedAt, plan.UpdatedAt,
	)
	if err != nil {
		return err
	}

	for i := range limits {
		limits[i].ID = uuid.New().String()
		limits[i].PlanID = plan.ID
		limits[i].CreatedAt = plan.CreatedAt
		limits[i].UpdatedAt = plan.CreatedAt
		fixedResetMinute, err := nullableFixedResetMinute(limits[i])
		if err != nil {
			return err
		}
		_, err = tx.Exec(
			`INSERT INTO subscription_plan_limits (id, plan_id, limit_type, window_mode, limit_micros, fixed_reset_minute, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			limits[i].ID, limits[i].PlanID, limits[i].LimitType, limits[i].WindowMode, limits[i].LimitMicros, fixedResetMinute, limits[i].CreatedAt, limits[i].UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *SubscriptionPlanRepository) GetByID(id string) (*model.SubscriptionPlan, []model.SubscriptionPlanLimit, error) {
	db := database.GetDB()
	plan := &model.SubscriptionPlan{}
	err := db.QueryRow(
		`SELECT id, name, description, enabled, created_at, updated_at FROM subscription_plans WHERE id = ?`, id,
	).Scan(&plan.ID, &plan.Name, &plan.Description, &plan.Enabled, &plan.CreatedAt, &plan.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	limits, err := r.getLimitsByPlanID(db, plan.ID)
	if err != nil {
		return nil, nil, err
	}

	return plan, limits, nil
}

func (r *SubscriptionPlanRepository) List() ([]*model.SubscriptionPlan, map[string][]model.SubscriptionPlanLimit, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, name, description, enabled, created_at, updated_at FROM subscription_plans ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var plans []*model.SubscriptionPlan
	for rows.Next() {
		p := &model.SubscriptionPlan{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, nil, err
		}
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	limitsMap := make(map[string][]model.SubscriptionPlanLimit)
	for _, p := range plans {
		limits, err := r.getLimitsByPlanID(db, p.ID)
		if err != nil {
			return nil, nil, err
		}
		limitsMap[p.ID] = limits
	}

	return plans, limitsMap, nil
}

func (r *SubscriptionPlanRepository) Update(id string, plan *model.SubscriptionPlan, limits []model.SubscriptionPlanLimit) error {
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	result, err := tx.Exec(
		`UPDATE subscription_plans SET name = ?, description = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		plan.Name, plan.Description, plan.Enabled, now, id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPlanNotFound
	}

	_, err = tx.Exec(`DELETE FROM subscription_plan_limits WHERE plan_id = ?`, id)
	if err != nil {
		return err
	}

	for i := range limits {
		limits[i].ID = uuid.New().String()
		limits[i].PlanID = id
		limits[i].CreatedAt = now
		limits[i].UpdatedAt = now
		fixedResetMinute, err := nullableFixedResetMinute(limits[i])
		if err != nil {
			return err
		}
		_, err = tx.Exec(
			`INSERT INTO subscription_plan_limits (id, plan_id, limit_type, window_mode, limit_micros, fixed_reset_minute, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			limits[i].ID, limits[i].PlanID, limits[i].LimitType, limits[i].WindowMode, limits[i].LimitMicros, fixedResetMinute, limits[i].CreatedAt, limits[i].UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *SubscriptionPlanRepository) Delete(id string) error {
	db := database.GetDB()
	_, err := db.Exec(`DELETE FROM subscription_plans WHERE id = ?`, id)
	return err
}

func (r *SubscriptionPlanRepository) SetEnabled(id string, enabled bool) error {
	db := database.GetDB()
	result, err := db.Exec(
		`UPDATE subscription_plans SET enabled = ?, updated_at = ? WHERE id = ?`,
		enabled, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPlanNotFound
	}
	return nil
}

func (r *SubscriptionPlanRepository) getLimitsByPlanID(db *sql.DB, planID string) ([]model.SubscriptionPlanLimit, error) {
	rows, err := db.Query(
		`SELECT id, plan_id, limit_type, window_mode, limit_micros, fixed_reset_minute, created_at, updated_at FROM subscription_plan_limits WHERE plan_id = ? ORDER BY limit_type`,
		planID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var limits []model.SubscriptionPlanLimit
	for rows.Next() {
		l := model.SubscriptionPlanLimit{}
		var fixedResetMinute sql.NullInt64
		if err := rows.Scan(&l.ID, &l.PlanID, &l.LimitType, &l.WindowMode, &l.LimitMicros, &fixedResetMinute, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		if fixedResetMinute.Valid {
			minutes := int(fixedResetMinute.Int64)
			l.FixedResetTime = model.FormatFixedResetTime(&minutes)
		}
		limits = append(limits, l)
	}
	return limits, rows.Err()
}
