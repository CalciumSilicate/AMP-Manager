package repository

import (
	"errors"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

var ErrSubscriptionNotFound = errors.New("用户订阅不存在")

type UserSubscriptionRepositoryInterface interface {
	Assign(sub *model.UserSubscription) error
	GetActiveByUserID(userID string) (*model.UserSubscription, error)
	ListActiveByUserID(userID string) ([]*model.UserSubscription, error)
	ListByUserID(userID string) ([]*model.UserSubscription, error)
	UpdateStatus(id string, status model.SubscriptionStatus) error
	UpdateExpiry(id string, expiresAt time.Time) error
}

var _ UserSubscriptionRepositoryInterface = (*UserSubscriptionRepository)(nil)

type UserSubscriptionRepository struct{}

func NewUserSubscriptionRepository() *UserSubscriptionRepository {
	return &UserSubscriptionRepository{}
}

func (r *UserSubscriptionRepository) Assign(sub *model.UserSubscription) error {
	db := database.GetDB()
	sub.ID = uuid.New().String()
	sub.CreatedAt = time.Now().UTC()
	sub.UpdatedAt = sub.CreatedAt

	_, err := db.Exec(
		`INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.UserID, sub.PlanID, sub.StartsAt, sub.ExpiresAt, sub.Status, sub.CreatedAt, sub.UpdatedAt,
	)
	return err
}

func (r *UserSubscriptionRepository) GetActiveByUserID(userID string) (*model.UserSubscription, error) {
	subs, err := r.ListActiveByUserID(userID)
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, nil
	}
	return subs[0], nil
}

func (r *UserSubscriptionRepository) ListActiveByUserID(userID string) ([]*model.UserSubscription, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at
		 FROM user_subscriptions
		 WHERE user_id = ? AND status = 'active' AND (expires_at IS NULL OR expires_at > ?)
		 ORDER BY CASE WHEN expires_at IS NULL THEN 1 ELSE 0 END ASC, expires_at ASC, created_at ASC`,
		userID, time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []*model.UserSubscription
	for rows.Next() {
		sub := &model.UserSubscription{}
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.PlanID, &sub.StartsAt, &sub.ExpiresAt, &sub.Status, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (r *UserSubscriptionRepository) ListByUserID(userID string) ([]*model.UserSubscription, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at 
		 FROM user_subscriptions WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []*model.UserSubscription
	for rows.Next() {
		s := &model.UserSubscription{}
		if err := rows.Scan(&s.ID, &s.UserID, &s.PlanID, &s.StartsAt, &s.ExpiresAt, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

func (r *UserSubscriptionRepository) UpdateStatus(id string, status model.SubscriptionStatus) error {
	db := database.GetDB()
	result, err := db.Exec(
		`UPDATE user_subscriptions SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

func (r *UserSubscriptionRepository) UpdateExpiry(id string, expiresAt time.Time) error {
	db := database.GetDB()
	result, err := db.Exec(
		`UPDATE user_subscriptions SET expires_at = ?, updated_at = ? WHERE id = ?`,
		expiresAt, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}
