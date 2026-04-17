package repository

import (
	"database/sql"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

type BillingEventRepositoryInterface interface {
	Create(event *model.BillingEvent) error
	GetUsageInWindow(userSubscriptionID string, start, end time.Time) (int64, error)
	GetUsageInWindowForLimit(userSubscriptionID string, limitType model.LimitType, windowMode model.WindowMode, start, end time.Time) (int64, error)
	ListByUserID(userID string, limit, offset int) ([]*model.BillingEvent, error)
	ListByRequestLogID(requestLogID string) ([]*model.BillingEvent, error)
}

var _ BillingEventRepositoryInterface = (*BillingEventRepository)(nil)

type BillingEventRepository struct{}

func NewBillingEventRepository() *BillingEventRepository {
	return &BillingEventRepository{}
}

func (r *BillingEventRepository) Create(event *model.BillingEvent) error {
	db := database.GetDB()
	event.ID = uuid.New().String()
	event.CreatedAt = time.Now().UTC()

	_, err := db.Exec(
		`INSERT INTO billing_events (id, request_log_id, user_id, user_subscription_id, source, event_type, amount_micros, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.RequestLogID, event.UserID, event.UserSubscriptionID, event.Source, event.EventType, event.AmountMicros, event.CreatedAt,
	)
	return err
}

func (r *BillingEventRepository) GetUsageInWindow(userSubscriptionID string, start, end time.Time) (int64, error) {
	return r.getUsageInWindow(userSubscriptionID, start, end)
}

func (r *BillingEventRepository) GetUsageInWindowForLimit(userSubscriptionID string, limitType model.LimitType, windowMode model.WindowMode, start, end time.Time) (int64, error) {
	effectiveStart := start
	if limitType == model.LimitTypeDaily && windowMode == model.WindowModeFixed {
		resetRecord, err := NewBillingDailyResetRepository().GetLatestForWindow(userSubscriptionID, start, end)
		if err != nil {
			return 0, err
		}
		if resetRecord != nil && resetRecord.CreatedAt.After(effectiveStart) {
			effectiveStart = resetRecord.CreatedAt.UTC()
		}
	}
	return r.getUsageInWindow(userSubscriptionID, effectiveStart, end)
}

func (r *BillingEventRepository) getUsageInWindow(userSubscriptionID string, start, end time.Time) (int64, error) {
	db := database.GetDB()
	var chargeSum, refundSum sql.NullInt64

	err := db.QueryRow(
		`SELECT 
			COALESCE(SUM(CASE WHEN event_type = 'charge' THEN amount_micros ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN event_type = 'refund' THEN amount_micros ELSE 0 END), 0)
		 FROM billing_events 
		 WHERE user_subscription_id = ? AND source = 'subscription' AND created_at >= ? AND created_at < ?`,
		userSubscriptionID, start, end,
	).Scan(&chargeSum, &refundSum)
	if err != nil {
		return 0, err
	}

	return chargeSum.Int64 - refundSum.Int64, nil
}

func (r *BillingEventRepository) ListByUserID(userID string, limit, offset int) ([]*model.BillingEvent, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, request_log_id, user_id, user_subscription_id, source, event_type, amount_micros, created_at 
		 FROM billing_events WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*model.BillingEvent
	for rows.Next() {
		e := &model.BillingEvent{}
		if err := rows.Scan(&e.ID, &e.RequestLogID, &e.UserID, &e.UserSubscriptionID, &e.Source, &e.EventType, &e.AmountMicros, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *BillingEventRepository) ListByRequestLogID(requestLogID string) ([]*model.BillingEvent, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, request_log_id, user_id, user_subscription_id, source, event_type, amount_micros, created_at 
		 FROM billing_events WHERE request_log_id = ? ORDER BY created_at DESC`,
		requestLogID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*model.BillingEvent
	for rows.Next() {
		e := &model.BillingEvent{}
		if err := rows.Scan(&e.ID, &e.RequestLogID, &e.UserID, &e.UserSubscriptionID, &e.Source, &e.EventType, &e.AmountMicros, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
