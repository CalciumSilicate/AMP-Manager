package repository

import (
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

type ChannelModelRepository struct{}

func NewChannelModelRepository() *ChannelModelRepository {
	return &ChannelModelRepository{}
}

func (r *ChannelModelRepository) ReplaceModels(channelID string, models []model.ChannelModel2) error {
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM channel_models WHERE channel_id = ?`, channelID)
	if err != nil {
		tx.Rollback()
		return err
	}

	for _, m := range models {
		m.ID = uuid.New().String()
		m.CreatedAt = time.Now().UTC()
		_, err = tx.Exec(
			`INSERT INTO channel_models (id, channel_id, model_id, display_name, created_at) VALUES (?, ?, ?, ?, ?)`,
			m.ID, channelID, m.ModelID, m.DisplayName, m.CreatedAt,
		)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (r *ChannelModelRepository) GetByChannelID(channelID string) ([]*model.ChannelModel2, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, channel_id, model_id, display_name, created_at FROM channel_models WHERE channel_id = ? ORDER BY model_id`,
		channelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []*model.ChannelModel2
	for rows.Next() {
		m := &model.ChannelModel2{}
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.ModelID, &m.DisplayName, &m.CreatedAt); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

func (r *ChannelModelRepository) ListAllWithChannel() ([]*model.AvailableModel, error) {
	db := database.GetDB()
	rows, err := db.Query(`
		SELECT cm.model_id, cm.display_name, c.id, c.type, c.name, c.model_whitelist, c.models_json
		FROM channel_models cm
		JOIN channels c ON cm.channel_id = c.id
		WHERE c.enabled = 1
		ORDER BY c.type, cm.model_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []*model.AvailableModel
	for rows.Next() {
		m := &model.AvailableModel{}
		if err := rows.Scan(&m.ModelID, &m.DisplayName, &m.ChannelID, &m.ChannelType, &m.ChannelName, &m.ModelWhitelist, &m.ModelsJSON); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

func (r *ChannelModelRepository) DeleteByChannelID(channelID string) error {
	db := database.GetDB()
	_, err := db.Exec(`DELETE FROM channel_models WHERE channel_id = ?`, channelID)
	return err
}
