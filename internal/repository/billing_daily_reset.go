package repository

import (
	"database/sql"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

type BillingDailyResetRepositoryInterface interface {
	Create(record *model.BillingDailyResetRecord) error
	GetLatestForWindow(userSubscriptionID string, windowStart, windowEnd time.Time) (*model.BillingDailyResetRecord, error)
	CountByUserBetween(userID string, start, end time.Time) (int, error)
}

var _ BillingDailyResetRepositoryInterface = (*BillingDailyResetRepository)(nil)

type BillingDailyResetRepository struct{}

func NewBillingDailyResetRepository() *BillingDailyResetRepository {
	return &BillingDailyResetRepository{}
}

func (r *BillingDailyResetRepository) Create(record *model.BillingDailyResetRecord) error {
	db := database.GetDB()
	record.ID = uuid.New().String()
	record.CreatedAt = time.Now().UTC()

	_, err := db.Exec(
		`INSERT INTO billing_daily_reset_records (
			id, user_id, user_subscription_id, window_start, window_end,
			used_micros_before_reset, expires_at_before, expires_at_after, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.UserID,
		record.UserSubscriptionID,
		record.WindowStart,
		record.WindowEnd,
		record.UsedMicrosBeforeReset,
		record.ExpiresAtBefore,
		record.ExpiresAtAfter,
		record.CreatedAt,
	)
	return err
}

func (r *BillingDailyResetRepository) GetLatestForWindow(userSubscriptionID string, windowStart, windowEnd time.Time) (*model.BillingDailyResetRecord, error) {
	db := database.GetDB()
	record := &model.BillingDailyResetRecord{}
	err := db.QueryRow(
		`SELECT id, user_id, user_subscription_id, window_start, window_end,
		        used_micros_before_reset, expires_at_before, expires_at_after, created_at
		 FROM billing_daily_reset_records
		 WHERE user_subscription_id = ? AND created_at >= ? AND created_at < ?
		 ORDER BY created_at DESC
		 LIMIT 1`,
		userSubscriptionID,
		windowStart.UTC(),
		windowEnd.UTC(),
	).Scan(
		&record.ID,
		&record.UserID,
		&record.UserSubscriptionID,
		&record.WindowStart,
		&record.WindowEnd,
		&record.UsedMicrosBeforeReset,
		&record.ExpiresAtBefore,
		&record.ExpiresAtAfter,
		&record.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (r *BillingDailyResetRepository) CountByUserBetween(userID string, start, end time.Time) (int, error) {
	db := database.GetDB()
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*)
		 FROM billing_daily_reset_records
		 WHERE user_id = ? AND created_at >= ? AND created_at < ?`,
		userID,
		start.UTC(),
		end.UTC(),
	).Scan(&count)
	return count, err
}
