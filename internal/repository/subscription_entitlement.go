package repository

import (
	"database/sql"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

type SubscriptionEntitlementRepositoryInterface interface {
	CreateTx(tx *sql.Tx, item *model.SubscriptionEntitlement) error
	ListActiveByUserID(userID string, now time.Time) ([]*model.SubscriptionEntitlement, error)
	ListActiveByUserIDTx(tx *sql.Tx, userID string, now time.Time) ([]*model.SubscriptionEntitlement, error)
	CountByUserTx(tx *sql.Tx, userID string) (int, error)
	CancelActiveByUserTx(tx *sql.Tx, userID string, now time.Time) error
	ConsumeActiveByUserPlanTx(tx *sql.Tx, userID, planID, orderNo string, now time.Time) error
}

type SubscriptionEntitlementRepository struct{}

func NewSubscriptionEntitlementRepository() *SubscriptionEntitlementRepository {
	return &SubscriptionEntitlementRepository{}
}

func (r *SubscriptionEntitlementRepository) CreateTx(tx *sql.Tx, item *model.SubscriptionEntitlement) error {
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
		`INSERT INTO subscription_entitlements (
			id, user_id, plan_id, source_type, source_ref_id, valuation_cny_cent_per_day,
			starts_at, expires_at, status, consumed_by_purchase_order_no, consumed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID,
		item.UserID,
		item.PlanID,
		item.SourceType,
		item.SourceRefID,
		item.ValuationCnyCentPerDay,
		item.StartsAt,
		item.ExpiresAt,
		item.Status,
		item.ConsumedByPurchaseOrderNo,
		item.ConsumedAt,
		item.CreatedAt,
		item.UpdatedAt,
	)
	return err
}

func (r *SubscriptionEntitlementRepository) ListActiveByUserID(userID string, now time.Time) ([]*model.SubscriptionEntitlement, error) {
	return r.listActiveByUserID(database.GetDB().Query, userID, now)
}

func (r *SubscriptionEntitlementRepository) ListActiveByUserIDTx(tx *sql.Tx, userID string, now time.Time) ([]*model.SubscriptionEntitlement, error) {
	return r.listActiveByUserID(tx.Query, userID, now)
}

func (r *SubscriptionEntitlementRepository) listActiveByUserID(
	query func(string, ...any) (*sql.Rows, error),
	userID string,
	now time.Time,
) ([]*model.SubscriptionEntitlement, error) {
	rows, err := query(
		`SELECT id, user_id, plan_id, source_type, source_ref_id, valuation_cny_cent_per_day,
		        starts_at, expires_at, status, consumed_by_purchase_order_no, consumed_at, created_at, updated_at
		   FROM subscription_entitlements
		  WHERE user_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)
		  ORDER BY starts_at ASC, created_at ASC`,
		userID,
		model.SubscriptionEntitlementStatusActive,
		now.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.SubscriptionEntitlement, 0)
	for rows.Next() {
		item := &model.SubscriptionEntitlement{}
		var consumedAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.PlanID,
			&item.SourceType,
			&item.SourceRefID,
			&item.ValuationCnyCentPerDay,
			&item.StartsAt,
			&item.ExpiresAt,
			&item.Status,
			&item.ConsumedByPurchaseOrderNo,
			&consumedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if consumedAt.Valid {
			item.ConsumedAt = &consumedAt.Time
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SubscriptionEntitlementRepository) CountByUserTx(tx *sql.Tx, userID string) (int, error) {
	var count int
	err := tx.QueryRow(`SELECT COUNT(*) FROM subscription_entitlements WHERE user_id = ?`, userID).Scan(&count)
	return count, err
}

func (r *SubscriptionEntitlementRepository) CancelActiveByUserTx(tx *sql.Tx, userID string, now time.Time) error {
	_, err := tx.Exec(
		`UPDATE subscription_entitlements
		    SET status = ?, consumed_by_purchase_order_no = '', consumed_at = ?, updated_at = ?
		  WHERE user_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)`,
		model.SubscriptionEntitlementStatusCancelled,
		now,
		now,
		userID,
		model.SubscriptionEntitlementStatusActive,
		now,
	)
	return err
}

func (r *SubscriptionEntitlementRepository) ConsumeActiveByUserPlanTx(tx *sql.Tx, userID, planID, orderNo string, now time.Time) error {
	_, err := tx.Exec(
		`UPDATE subscription_entitlements
		    SET status = ?, consumed_by_purchase_order_no = ?, consumed_at = ?, updated_at = ?
		  WHERE user_id = ? AND plan_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)`,
		model.SubscriptionEntitlementStatusConsumed,
		orderNo,
		now,
		now,
		userID,
		planID,
		model.SubscriptionEntitlementStatusActive,
		now,
	)
	return err
}
