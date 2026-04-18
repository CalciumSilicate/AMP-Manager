package repository

import (
	"database/sql"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/precision"

	"github.com/google/uuid"
)

const channelSelectColumns = `id, type, endpoint, name, base_url, api_key, enabled, split_groups_by_source, weight, priority, rate_multiplier, rate_multiplier_ppm, model_whitelist, simulate_cli, simulate_ua, simulate_system_prompt, traditional_chinese, copilot_api, codex_websocket_enabled, models_json, headers_json, translator_json, created_at, updated_at`

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
	SetGroupBinding(id string, binding *model.ChannelGroupBinding) error
	GetGroupBinding(channelID string) (*model.ChannelGroupBinding, error)
	GetGroupBindingsByChannelIDs(channelIDs []string) (map[string]*model.ChannelGroupBinding, error)
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
		`INSERT INTO channels (id, type, endpoint, name, base_url, api_key, enabled, split_groups_by_source, weight, priority, rate_multiplier, rate_multiplier_ppm, model_whitelist, simulate_cli, simulate_ua, simulate_system_prompt, traditional_chinese, copilot_api, codex_websocket_enabled, models_json, headers_json, translator_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		channel.ID, channel.Type, channel.Endpoint, channel.Name, channel.BaseURL, channel.APIKey,
		channel.Enabled, channel.SplitGroupsBySource, channel.Weight, channel.Priority, channel.RateMultiplier, channel.RateMultiplierPPM, channel.ModelWhitelist, channel.SimulateCLI, channel.SimulateUA, channel.SimulateSystemPrompt, channel.TraditionalChinese, channel.CopilotAPI, channel.CodexWebsocketEnabled, channel.ModelsJSON, channel.HeadersJSON, channel.TranslatorJSON,
		channel.CreatedAt, channel.UpdatedAt,
	)
	return err
}

func scanChannel(scanner interface {
	Scan(dest ...any) error
}) (*model.Channel, error) {
	channel := &model.Channel{}
	var rateMultiplier sql.NullFloat64
	var rateMultiplierPPM sql.NullInt64

	err := scanner.Scan(
		&channel.ID, &channel.Type, &channel.Endpoint, &channel.Name, &channel.BaseURL, &channel.APIKey,
		&channel.Enabled, &channel.SplitGroupsBySource, &channel.Weight, &channel.Priority, &rateMultiplier, &rateMultiplierPPM, &channel.ModelWhitelist, &channel.SimulateCLI, &channel.SimulateUA, &channel.SimulateSystemPrompt, &channel.TraditionalChinese, &channel.CopilotAPI, &channel.CodexWebsocketEnabled, &channel.ModelsJSON, &channel.HeadersJSON, &channel.TranslatorJSON,
		&channel.CreatedAt, &channel.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	channel.RateMultiplierPPM = deriveChannelMultiplierPPM(rateMultiplierPPM, rateMultiplier)
	channel.RateMultiplier = precision.MultiplierPPMToFloat64(channel.RateMultiplierPPM)
	return channel, nil
}

func (r *ChannelRepository) GetByID(id string) (*model.Channel, error) {
	db := database.GetDB()
	channel, err := scanChannel(db.QueryRow(`SELECT `+channelSelectColumns+` FROM channels WHERE id = ?`, id))
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
	rows, err := db.Query(`SELECT ` + channelSelectColumns + ` FROM channels ORDER BY priority ASC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*model.Channel
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (r *ChannelRepository) ListEnabled() ([]*model.Channel, error) {
	db := database.GetDB()
	rows, err := db.Query(`SELECT ` + channelSelectColumns + ` FROM channels WHERE enabled = 1 ORDER BY priority ASC, weight DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*model.Channel
	for rows.Next() {
		channel, err := scanChannel(rows)
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
		`UPDATE channels SET type = ?, endpoint = ?, name = ?, base_url = ?, api_key = ?, enabled = ?, split_groups_by_source = ?, weight = ?, priority = ?, rate_multiplier = ?, rate_multiplier_ppm = ?, model_whitelist = ?, simulate_cli = ?, simulate_ua = ?, simulate_system_prompt = ?, traditional_chinese = ?, copilot_api = ?, codex_websocket_enabled = ?, models_json = ?, headers_json = ?, translator_json = ?, updated_at = ?
		 WHERE id = ?`,
		channel.Type, channel.Endpoint, channel.Name, channel.BaseURL, channel.APIKey, channel.Enabled, channel.SplitGroupsBySource, channel.Weight, channel.Priority, channel.RateMultiplier, channel.RateMultiplierPPM, channel.ModelWhitelist, channel.SimulateCLI, channel.SimulateUA, channel.SimulateSystemPrompt, channel.TraditionalChinese, channel.CopilotAPI, channel.CodexWebsocketEnabled, channel.ModelsJSON, channel.HeadersJSON, channel.TranslatorJSON, channel.UpdatedAt,
		channel.ID,
	)
	return err
}

func deriveChannelMultiplierPPM(ppm sql.NullInt64, legacy sql.NullFloat64) int64 {
	if ppm.Valid && ppm.Int64 > 0 {
		return ppm.Int64
	}
	if legacy.Valid {
		return precision.FloatMultiplierToPPM(legacy.Float64)
	}
	return precision.DefaultMultiplierPPM
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

func setChannelGroupRows(tx *sql.Tx, tableName, channelID string, groupIDs []string) error {
	if _, err := tx.Exec(`DELETE FROM `+tableName+` WHERE channel_id = ?`, channelID); err != nil {
		return err
	}
	for _, gid := range groupIDs {
		if gid == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO `+tableName+` (channel_id, group_id) VALUES (?, ?)`, channelID, gid); err != nil {
			return err
		}
	}
	return nil
}

func (r *ChannelRepository) SetGroups(id string, groupIDs []string) error {
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := setChannelGroupRows(tx, "channel_groups", id, groupIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE channels SET split_groups_by_source = 0, updated_at = ? WHERE id = ?`, time.Now().UTC(), id); err != nil {
		return err
	}
	if err := setChannelGroupRows(tx, "channel_subscription_groups", id, groupIDs); err != nil {
		return err
	}
	if err := setChannelGroupRows(tx, "channel_usage_groups", id, groupIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func loadChannelGroupRows(db *sql.DB, tableName string, channelIDs []string) (map[string][]string, error) {
	result := make(map[string][]string)
	if len(channelIDs) == 0 {
		return result, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(channelIDs)), ",")
	query := `SELECT channel_id, group_id FROM ` + tableName + ` WHERE channel_id IN (` + placeholders + `)`
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
	return loadChannelGroupRows(database.GetDB(), "channel_groups", channelIDs)
}

func (r *ChannelRepository) SetGroupBinding(id string, binding *model.ChannelGroupBinding) error {
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if binding == nil {
		binding = &model.ChannelGroupBinding{}
	}

	if _, err := tx.Exec(`UPDATE channels SET split_groups_by_source = ?, updated_at = ? WHERE id = ?`, binding.SplitBySource, time.Now().UTC(), id); err != nil {
		return err
	}
	if err := setChannelGroupRows(tx, "channel_groups", id, binding.SharedGroupIDs); err != nil {
		return err
	}
	if err := setChannelGroupRows(tx, "channel_subscription_groups", id, binding.SubscriptionGroupIDs); err != nil {
		return err
	}
	if err := setChannelGroupRows(tx, "channel_usage_groups", id, binding.UsageGroupIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ChannelRepository) GetGroupBinding(channelID string) (*model.ChannelGroupBinding, error) {
	bindings, err := r.GetGroupBindingsByChannelIDs([]string{channelID})
	if err != nil {
		return nil, err
	}
	if binding, ok := bindings[channelID]; ok {
		return binding, nil
	}
	return &model.ChannelGroupBinding{}, nil
}

func (r *ChannelRepository) GetGroupBindingsByChannelIDs(channelIDs []string) (map[string]*model.ChannelGroupBinding, error) {
	result := make(map[string]*model.ChannelGroupBinding, len(channelIDs))
	if len(channelIDs) == 0 {
		return result, nil
	}

	db := database.GetDB()
	placeholders := strings.TrimRight(strings.Repeat("?,", len(channelIDs)), ",")
	args := make([]interface{}, len(channelIDs))
	for i, id := range channelIDs {
		args[i] = id
		result[id] = &model.ChannelGroupBinding{}
	}

	rows, err := db.Query(`SELECT id, split_groups_by_source FROM channels WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var channelID string
		var splitBySource bool
		if err := rows.Scan(&channelID, &splitBySource); err != nil {
			rows.Close()
			return nil, err
		}
		result[channelID].SplitBySource = splitBySource
	}
	rows.Close()

	sharedMap, err := loadChannelGroupRows(db, "channel_groups", channelIDs)
	if err != nil {
		return nil, err
	}
	subscriptionMap, err := loadChannelGroupRows(db, "channel_subscription_groups", channelIDs)
	if err != nil {
		return nil, err
	}
	usageMap, err := loadChannelGroupRows(db, "channel_usage_groups", channelIDs)
	if err != nil {
		return nil, err
	}

	for _, channelID := range channelIDs {
		binding := result[channelID]
		binding.SharedGroupIDs = append([]string(nil), sharedMap[channelID]...)
		binding.SubscriptionGroupIDs = append([]string(nil), subscriptionMap[channelID]...)
		binding.UsageGroupIDs = append([]string(nil), usageMap[channelID]...)
	}
	return result, nil
}
