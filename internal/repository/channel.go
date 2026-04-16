package repository

import (
	"database/sql"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

type ChannelRepositoryInterface interface {
	Create(channel *model.Channel) error
	GetByID(id string) (*model.Channel, error)
	List() ([]*model.Channel, error)
	ListEnabled() ([]*model.Channel, error)
	Update(channel *model.Channel) error
	Delete(id string) error
	SetEnabled(id string, enabled bool) error
	SetGroups(id string, groupIDs []string) error
	GetGroupIDs(channelID string) ([]string, error)
	GetGroupIDsByChannelIDs(channelIDs []string) (map[string][]string, error)
}

var _ ChannelRepositoryInterface = (*ChannelRepository)(nil)

type ChannelRepository struct{}

func NewChannelRepository() *ChannelRepository {
	return &ChannelRepository{}
}

func (r *ChannelRepository) Create(channel *model.Channel) error {
	db := database.GetDB()
	channel.ID = uuid.New().String()
	now := time.Now().UTC()
	channel.CreatedAt = now
	channel.UpdatedAt = now

	_, err := db.Exec(
		`INSERT INTO channels (id, type, endpoint, name, base_url, api_key, enabled, weight, priority, rate_multiplier, model_whitelist, simulate_cli, simulate_ua, simulate_system_prompt, traditional_chinese, copilot_api, codex_websocket_enabled, models_json, headers_json, translator_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		channel.ID, channel.Type, channel.Endpoint, channel.Name, channel.BaseURL, channel.APIKey,
		channel.Enabled, channel.Weight, channel.Priority, channel.RateMultiplier, channel.ModelWhitelist, channel.SimulateCLI, channel.SimulateUA, channel.SimulateSystemPrompt, channel.TraditionalChinese, channel.CopilotAPI, channel.CodexWebsocketEnabled, channel.ModelsJSON, channel.HeadersJSON, channel.TranslatorJSON,
		channel.CreatedAt, channel.UpdatedAt,
	)
	return err
}

func (r *ChannelRepository) GetByID(id string) (*model.Channel, error) {
	db := database.GetDB()
	channel := &model.Channel{}

	err := db.QueryRow(
		`SELECT id, type, endpoint, name, base_url, api_key, enabled, weight, priority, rate_multiplier, model_whitelist, simulate_cli, simulate_ua, simulate_system_prompt, traditional_chinese, copilot_api, codex_websocket_enabled, models_json, headers_json, translator_json, created_at, updated_at
		 FROM channels WHERE id = ?`,
		id,
	).Scan(
		&channel.ID, &channel.Type, &channel.Endpoint, &channel.Name, &channel.BaseURL, &channel.APIKey,
		&channel.Enabled, &channel.Weight, &channel.Priority, &channel.RateMultiplier, &channel.ModelWhitelist, &channel.SimulateCLI, &channel.SimulateUA, &channel.SimulateSystemPrompt, &channel.TraditionalChinese, &channel.CopilotAPI, &channel.CodexWebsocketEnabled, &channel.ModelsJSON, &channel.HeadersJSON, &channel.TranslatorJSON,
		&channel.CreatedAt, &channel.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return channel, nil
}

func (r *ChannelRepository) List() ([]*model.Channel, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, type, endpoint, name, base_url, api_key, enabled, weight, priority, rate_multiplier, model_whitelist, simulate_cli, simulate_ua, simulate_system_prompt, traditional_chinese, copilot_api, codex_websocket_enabled, models_json, headers_json, translator_json, created_at, updated_at
		 FROM channels ORDER BY priority ASC, created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*model.Channel
	for rows.Next() {
		channel := &model.Channel{}
		err := rows.Scan(
			&channel.ID, &channel.Type, &channel.Endpoint, &channel.Name, &channel.BaseURL, &channel.APIKey,
			&channel.Enabled, &channel.Weight, &channel.Priority, &channel.RateMultiplier, &channel.ModelWhitelist, &channel.SimulateCLI, &channel.SimulateUA, &channel.SimulateSystemPrompt, &channel.TraditionalChinese, &channel.CopilotAPI, &channel.CodexWebsocketEnabled, &channel.ModelsJSON, &channel.HeadersJSON, &channel.TranslatorJSON,
			&channel.CreatedAt, &channel.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (r *ChannelRepository) ListEnabled() ([]*model.Channel, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, type, endpoint, name, base_url, api_key, enabled, weight, priority, rate_multiplier, model_whitelist, simulate_cli, simulate_ua, simulate_system_prompt, traditional_chinese, copilot_api, codex_websocket_enabled, models_json, headers_json, translator_json, created_at, updated_at
		 FROM channels WHERE enabled = 1 ORDER BY priority ASC, weight DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*model.Channel
	for rows.Next() {
		channel := &model.Channel{}
		err := rows.Scan(
			&channel.ID, &channel.Type, &channel.Endpoint, &channel.Name, &channel.BaseURL, &channel.APIKey,
			&channel.Enabled, &channel.Weight, &channel.Priority, &channel.RateMultiplier, &channel.ModelWhitelist, &channel.SimulateCLI, &channel.SimulateUA, &channel.SimulateSystemPrompt, &channel.TraditionalChinese, &channel.CopilotAPI, &channel.CodexWebsocketEnabled, &channel.ModelsJSON, &channel.HeadersJSON, &channel.TranslatorJSON,
			&channel.CreatedAt, &channel.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (r *ChannelRepository) Update(channel *model.Channel) error {
	db := database.GetDB()
	channel.UpdatedAt = time.Now().UTC()

	_, err := db.Exec(
		`UPDATE channels SET type = ?, endpoint = ?, name = ?, base_url = ?, api_key = ?, enabled = ?, weight = ?, priority = ?, rate_multiplier = ?, model_whitelist = ?, simulate_cli = ?, simulate_ua = ?, simulate_system_prompt = ?, traditional_chinese = ?, copilot_api = ?, codex_websocket_enabled = ?, models_json = ?, headers_json = ?, translator_json = ?, updated_at = ?
		 WHERE id = ?`,
		channel.Type, channel.Endpoint, channel.Name, channel.BaseURL, channel.APIKey, channel.Enabled, channel.Weight, channel.Priority, channel.RateMultiplier, channel.ModelWhitelist, channel.SimulateCLI, channel.SimulateUA, channel.SimulateSystemPrompt, channel.TraditionalChinese, channel.CopilotAPI, channel.CodexWebsocketEnabled, channel.ModelsJSON, channel.HeadersJSON, channel.TranslatorJSON, channel.UpdatedAt,
		channel.ID,
	)
	return err
}

func (r *ChannelRepository) Delete(id string) error {
	db := database.GetDB()
	_, err := db.Exec(`DELETE FROM channels WHERE id = ?`, id)
	return err
}

func (r *ChannelRepository) SetEnabled(id string, enabled bool) error {
	db := database.GetDB()
	_, err := db.Exec(`UPDATE channels SET enabled = ?, updated_at = ? WHERE id = ?`, enabled, time.Now().UTC(), id)
	return err
}

func (r *ChannelRepository) SetGroups(id string, groupIDs []string) error {
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM channel_groups WHERE channel_id = ?`, id)
	if err != nil {
		return err
	}

	for _, gid := range groupIDs {
		if gid == "" {
			continue
		}
		_, err = tx.Exec(`INSERT INTO channel_groups (channel_id, group_id) VALUES (?, ?)`, id, gid)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *ChannelRepository) GetGroupIDs(channelID string) ([]string, error) {
	db := database.GetDB()
	rows, err := db.Query(`SELECT group_id FROM channel_groups WHERE channel_id = ?`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *ChannelRepository) GetGroupIDsByChannelIDs(channelIDs []string) (map[string][]string, error) {
	result := make(map[string][]string)
	if len(channelIDs) == 0 {
		return result, nil
	}

	db := database.GetDB()
	placeholders := strings.TrimRight(strings.Repeat("?,", len(channelIDs)), ",")
	query := `SELECT channel_id, group_id FROM channel_groups WHERE channel_id IN (` + placeholders + `)`

	args := make([]interface{}, len(channelIDs))
	for i, id := range channelIDs {
		args[i] = id
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var channelID, groupID string
		if err := rows.Scan(&channelID, &groupID); err != nil {
			return nil, err
		}
		result[channelID] = append(result[channelID], groupID)
	}
	return result, rows.Err()
}
