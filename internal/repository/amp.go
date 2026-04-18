package repository

import (
	"database/sql"
	"sync"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

var apiKeyLastUsedCache sync.Map

type AmpSettingsRepository struct{}

func NewAmpSettingsRepository() *AmpSettingsRepository {
	return &AmpSettingsRepository{}
}

func (r *AmpSettingsRepository) GetByUserID(userID string) (*model.AmpSettings, error) {
	db := database.GetDB()
	settings := &model.AmpSettings{}

	var webSearchMode sql.NullString
	err := db.QueryRow(
		`SELECT id, user_id, upstream_url, upstream_api_key, model_mappings_json, 
		        enabled, web_search_mode, native_mode, route_mappings_enabled, show_balance_in_ad, socks5_proxy, created_at, updated_at 
		 FROM user_amp_settings WHERE user_id = ?`,
		userID,
	).Scan(
		&settings.ID, &settings.UserID, &settings.UpstreamURL, &settings.UpstreamAPIKey,
		&settings.ModelMappingsJSON, &settings.Enabled,
		&webSearchMode, &settings.NativeMode, &settings.RouteMappingsEnabled, &settings.ShowBalanceInAd, &settings.Socks5Proxy, &settings.CreatedAt, &settings.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if webSearchMode.Valid {
		settings.WebSearchMode = webSearchMode.String
	} else {
		settings.WebSearchMode = model.WebSearchModeUpstream
	}
	return settings, nil
}

func (r *AmpSettingsRepository) Upsert(settings *model.AmpSettings) error {
	db := database.GetDB()
	now := time.Now().UTC()
	settings.UpdatedAt = now

	existing, err := r.GetByUserID(settings.UserID)
	if err != nil {
		return err
	}

	if existing == nil {
		settings.ID = uuid.New().String()
		settings.CreatedAt = now
		_, err = db.Exec(
			`INSERT INTO user_amp_settings 
			 (id, user_id, upstream_url, upstream_api_key, model_mappings_json, 
			  enabled, web_search_mode, native_mode, route_mappings_enabled, show_balance_in_ad, socks5_proxy, created_at, updated_at) 
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			settings.ID, settings.UserID, settings.UpstreamURL, settings.UpstreamAPIKey,
			settings.ModelMappingsJSON, settings.Enabled,
			settings.WebSearchMode, settings.NativeMode, settings.RouteMappingsEnabled, settings.ShowBalanceInAd, settings.Socks5Proxy, settings.CreatedAt, settings.UpdatedAt,
		)
	} else {
		settings.ID = existing.ID
		settings.CreatedAt = existing.CreatedAt
		_, err = db.Exec(
			`UPDATE user_amp_settings 
			 SET upstream_url = ?, upstream_api_key = ?, model_mappings_json = ?, 
			     enabled = ?, web_search_mode = ?, native_mode = ?, route_mappings_enabled = ?, show_balance_in_ad = ?, socks5_proxy = ?, updated_at = ? 
			 WHERE user_id = ?`,
			settings.UpstreamURL, settings.UpstreamAPIKey, settings.ModelMappingsJSON,
			settings.Enabled, settings.WebSearchMode,
			settings.NativeMode, settings.RouteMappingsEnabled, settings.ShowBalanceInAd, settings.Socks5Proxy, settings.UpdatedAt, settings.UserID,
		)
	}
	return err
}

type APIKeyRepository struct{}

func NewAPIKeyRepository() *APIKeyRepository {
	return &APIKeyRepository{}
}

func (r *APIKeyRepository) Create(apiKey *model.UserAPIKey) error {
	db := database.GetDB()
	apiKey.ID = uuid.New().String()
	apiKey.CreatedAt = time.Now().UTC()

	_, err := db.Exec(
		`INSERT INTO user_api_keys (id, user_id, name, prefix, key_hash, api_key, expires_at, created_at) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		apiKey.ID, apiKey.UserID, apiKey.Name, apiKey.Prefix, apiKey.KeyHash, apiKey.APIKey, apiKey.ExpiresAt, apiKey.CreatedAt,
	)
	return err
}

func (r *APIKeyRepository) ListByUserID(userID string) ([]*model.UserAPIKey, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, user_id, name, prefix, key_hash, created_at, revoked_at, last_used_at, expires_at
		 FROM user_api_keys WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*model.UserAPIKey
	for rows.Next() {
		key := &model.UserAPIKey{}
		var revokedAt, lastUsed, expiresAt sql.NullTime
		err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.KeyHash, &key.CreatedAt, &revokedAt, &lastUsed, &expiresAt)
		if err != nil {
			return nil, err
		}
		if revokedAt.Valid {
			key.RevokedAt = &revokedAt.Time
		}
		if lastUsed.Valid {
			key.LastUsed = &lastUsed.Time
		}
		if expiresAt.Valid {
			key.ExpiresAt = &expiresAt.Time
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (r *APIKeyRepository) GetByID(id string) (*model.UserAPIKey, error) {
	db := database.GetDB()
	key := &model.UserAPIKey{}
	var revokedAt, lastUsed, expiresAt sql.NullTime
	err := db.QueryRow(
		`SELECT id, user_id, name, prefix, key_hash, api_key, created_at, revoked_at, last_used_at, expires_at 
		 FROM user_api_keys WHERE id = ?`,
		id,
	).Scan(&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.KeyHash, &key.APIKey, &key.CreatedAt, &revokedAt, &lastUsed, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if revokedAt.Valid {
		key.RevokedAt = &revokedAt.Time
	}
	if lastUsed.Valid {
		key.LastUsed = &lastUsed.Time
	}
	if expiresAt.Valid {
		key.ExpiresAt = &expiresAt.Time
	}
	return key, nil
}

func (r *APIKeyRepository) Delete(id string) error {
	db := database.GetDB()
	_, err := db.Exec(`DELETE FROM user_api_keys WHERE id = ?`, id)
	return err
}

func (r *APIKeyRepository) UpdateEditableFields(id, name string, expiresAt *time.Time) error {
	db := database.GetDB()
	_, err := db.Exec(
		`UPDATE user_api_keys SET name = ?, expires_at = ? WHERE id = ?`,
		name, expiresAt, id,
	)
	return err
}

func (r *APIKeyRepository) UpdateKeyFields(id, name, prefix, keyHash, apiKey string, expiresAt *time.Time) error {
	db := database.GetDB()
	_, err := db.Exec(
		`UPDATE user_api_keys
		 SET name = ?, prefix = ?, key_hash = ?, api_key = ?, expires_at = ?
		 WHERE id = ?`,
		name, prefix, keyHash, apiKey, expiresAt, id,
	)
	return err
}

func (r *APIKeyRepository) SetRevoked(id string, revoked bool) error {
	db := database.GetDB()
	if revoked {
		_, err := db.Exec(`UPDATE user_api_keys SET revoked_at = ? WHERE id = ?`, time.Now().UTC(), id)
		return err
	}

	_, err := db.Exec(`UPDATE user_api_keys SET revoked_at = NULL WHERE id = ?`, id)
	return err
}

func (r *APIKeyRepository) GetByKeyHash(keyHash string) (*model.UserAPIKey, error) {
	db := database.GetDB()
	key := &model.UserAPIKey{}
	var revokedAt, lastUsed, expiresAt sql.NullTime
	err := db.QueryRow(
		`SELECT id, user_id, name, prefix, key_hash, created_at, revoked_at, last_used_at, expires_at 
		 FROM user_api_keys WHERE key_hash = ?`,
		keyHash,
	).Scan(&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.KeyHash, &key.CreatedAt, &revokedAt, &lastUsed, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if revokedAt.Valid {
		key.RevokedAt = &revokedAt.Time
	}
	if lastUsed.Valid {
		key.LastUsed = &lastUsed.Time
	}
	if expiresAt.Valid {
		key.ExpiresAt = &expiresAt.Time
	}
	return key, nil
}

func (r *APIKeyRepository) UpdateLastUsed(id string) error {
	db := database.GetDB()
	now := time.Now().UTC()
	_, err := db.Exec(`UPDATE user_api_keys SET last_used_at = ? WHERE id = ?`, now, id)
	return err
}

func (r *APIKeyRepository) UpdateLastUsedThrottled(id string, minInterval time.Duration) error {
	if id == "" {
		return nil
	}

	now := time.Now().UTC()
	if minInterval > 0 {
		if cached, ok := apiKeyLastUsedCache.Load(id); ok {
			if lastUpdated, ok := cached.(time.Time); ok && now.Sub(lastUpdated) < minInterval {
				return nil
			}
		}
		apiKeyLastUsedCache.Store(id, now)
	}

	if err := r.UpdateLastUsed(id); err != nil {
		apiKeyLastUsedCache.Delete(id)
		return err
	}

	return nil
}

func (r *APIKeyRepository) HasActiveByUserID(userID string) (bool, error) {
	db := database.GetDB()
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM user_api_keys
		 WHERE user_id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)`,
		userID,
		time.Now().UTC(),
	).Scan(&count)
	return count > 0, err
}
