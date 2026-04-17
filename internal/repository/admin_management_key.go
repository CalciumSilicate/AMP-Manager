package repository

import (
	"database/sql"
	"sync"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

type adminManagementKeyUsageUpdate struct {
	At         time.Time
	AuthMethod string
}

var adminManagementKeyLastUsedCache sync.Map

type AdminManagementKeyRepository struct{}

func NewAdminManagementKeyRepository() *AdminManagementKeyRepository {
	return &AdminManagementKeyRepository{}
}

func (r *AdminManagementKeyRepository) GetByUserID(userID string) (*model.AdminManagementKey, error) {
	db := database.GetDB()
	item := &model.AdminManagementKey{}
	var previousValidUntil, rotatedAt, lastUsedAt sql.NullTime
	err := db.QueryRow(
		`SELECT user_id, key_hash, key_ciphertext, key_prefix,
		        previous_key_hash, previous_key_ciphertext, previous_key_prefix, previous_valid_until,
		        enabled, created_at, rotated_at, last_used_at, last_auth_method, updated_at
		   FROM admin_management_keys
		  WHERE user_id = ?`,
		userID,
	).Scan(
		&item.UserID,
		&item.KeyHash,
		&item.KeyCiphertext,
		&item.KeyPrefix,
		&item.PreviousKeyHash,
		&item.PreviousKeyCiphertext,
		&item.PreviousKeyPrefix,
		&previousValidUntil,
		&item.Enabled,
		&item.CreatedAt,
		&rotatedAt,
		&lastUsedAt,
		&item.LastAuthMethod,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if previousValidUntil.Valid {
		item.PreviousValidUntil = &previousValidUntil.Time
	}
	if rotatedAt.Valid {
		item.RotatedAt = &rotatedAt.Time
	}
	if lastUsedAt.Valid {
		item.LastUsedAt = &lastUsedAt.Time
	}
	return item, nil
}

func (r *AdminManagementKeyRepository) GetByAnyHash(hash string) (*model.AdminManagementKey, error) {
	db := database.GetDB()
	item := &model.AdminManagementKey{}
	var previousValidUntil, rotatedAt, lastUsedAt sql.NullTime
	err := db.QueryRow(
		`SELECT user_id, key_hash, key_ciphertext, key_prefix,
		        previous_key_hash, previous_key_ciphertext, previous_key_prefix, previous_valid_until,
		        enabled, created_at, rotated_at, last_used_at, last_auth_method, updated_at
		   FROM admin_management_keys
		  WHERE key_hash = ? OR (previous_key_hash = ? AND previous_valid_until > ?)
		  LIMIT 1`,
		hash,
		hash,
		time.Now().UTC(),
	).Scan(
		&item.UserID,
		&item.KeyHash,
		&item.KeyCiphertext,
		&item.KeyPrefix,
		&item.PreviousKeyHash,
		&item.PreviousKeyCiphertext,
		&item.PreviousKeyPrefix,
		&previousValidUntil,
		&item.Enabled,
		&item.CreatedAt,
		&rotatedAt,
		&lastUsedAt,
		&item.LastAuthMethod,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if previousValidUntil.Valid {
		item.PreviousValidUntil = &previousValidUntil.Time
	}
	if rotatedAt.Valid {
		item.RotatedAt = &rotatedAt.Time
	}
	if lastUsedAt.Valid {
		item.LastUsedAt = &lastUsedAt.Time
	}
	return item, nil
}

func (r *AdminManagementKeyRepository) Upsert(item *model.AdminManagementKey) error {
	db := database.GetDB()
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	_, err := db.Exec(
		`INSERT INTO admin_management_keys (
		     user_id, key_hash, key_ciphertext, key_prefix,
		     previous_key_hash, previous_key_ciphertext, previous_key_prefix, previous_valid_until,
		     enabled, created_at, rotated_at, last_used_at, last_auth_method, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		     key_hash = excluded.key_hash,
		     key_ciphertext = excluded.key_ciphertext,
		     key_prefix = excluded.key_prefix,
		     previous_key_hash = excluded.previous_key_hash,
		     previous_key_ciphertext = excluded.previous_key_ciphertext,
		     previous_key_prefix = excluded.previous_key_prefix,
		     previous_valid_until = excluded.previous_valid_until,
		     enabled = excluded.enabled,
		     created_at = excluded.created_at,
		     rotated_at = excluded.rotated_at,
		     last_used_at = excluded.last_used_at,
		     last_auth_method = excluded.last_auth_method,
		     updated_at = excluded.updated_at`,
		item.UserID,
		item.KeyHash,
		item.KeyCiphertext,
		item.KeyPrefix,
		item.PreviousKeyHash,
		item.PreviousKeyCiphertext,
		item.PreviousKeyPrefix,
		item.PreviousValidUntil,
		item.Enabled,
		item.CreatedAt,
		item.RotatedAt,
		item.LastUsedAt,
		item.LastAuthMethod,
		item.UpdatedAt,
	)
	return err
}

func (r *AdminManagementKeyRepository) UpdateUsage(userID, authMethod string, usedAt time.Time) error {
	db := database.GetDB()
	_, err := db.Exec(
		`UPDATE admin_management_keys
		    SET last_used_at = ?, last_auth_method = ?, updated_at = ?
		  WHERE user_id = ?`,
		usedAt,
		authMethod,
		usedAt,
		userID,
	)
	return err
}

func (r *AdminManagementKeyRepository) UpdateUsageThrottled(userID, authMethod string, minInterval time.Duration) error {
	if userID == "" {
		return nil
	}

	now := time.Now().UTC()
	if minInterval > 0 {
		if cached, ok := adminManagementKeyLastUsedCache.Load(userID); ok {
			if last, ok := cached.(adminManagementKeyUsageUpdate); ok && now.Sub(last.At) < minInterval && last.AuthMethod == authMethod {
				return nil
			}
		}
		adminManagementKeyLastUsedCache.Store(userID, adminManagementKeyUsageUpdate{
			At:         now,
			AuthMethod: authMethod,
		})
	}

	if err := r.UpdateUsage(userID, authMethod, now); err != nil {
		adminManagementKeyLastUsedCache.Delete(userID)
		return err
	}
	return nil
}
