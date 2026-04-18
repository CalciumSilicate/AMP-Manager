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

type GroupRepositoryInterface interface {
	Create(group *model.Group) error
	GetByID(id string) (*model.Group, error)
	GetByIDs(ids []string) (map[string]*model.Group, error)
	GetByName(name string) (*model.Group, error)
	List() ([]*model.Group, error)
	Update(group *model.Group) error
	Delete(id string) error
	CountUsers(groupID string) (int, error)
	CountChannels(groupID string) (int, error)
	GetMinRateMultiplierByUserID(userID string) (int64, []string, error)
}

var _ GroupRepositoryInterface = (*GroupRepository)(nil)

type GroupRepository struct{}

func NewGroupRepository() *GroupRepository {
	return &GroupRepository{}
}

func (r *GroupRepository) Create(group *model.Group) error {
	db := database.GetDB()
	group.ID = uuid.New().String()
	now := time.Now().UTC()
	group.CreatedAt = now
	group.UpdatedAt = now
	if group.RateMultiplierPPM <= 0 {
		group.RateMultiplierPPM = precision.DefaultMultiplierPPM
	}
	group.RateMultiplier = precision.MultiplierPPMToFloat64(group.RateMultiplierPPM)

	_, err := db.Exec(
		`INSERT INTO groups (id, name, description, rate_multiplier, rate_multiplier_ppm, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		group.ID, group.Name, group.Description, group.RateMultiplier, group.RateMultiplierPPM, group.CreatedAt, group.UpdatedAt,
	)
	return err
}

func (r *GroupRepository) GetByID(id string) (*model.Group, error) {
	db := database.GetDB()
	group := &model.Group{}
	var rateMultiplier sql.NullFloat64
	var rateMultiplierPPM sql.NullInt64
	err := db.QueryRow(
		`SELECT id, name, description, rate_multiplier, rate_multiplier_ppm, created_at, updated_at FROM groups WHERE id = ?`, id,
	).Scan(&group.ID, &group.Name, &group.Description, &rateMultiplier, &rateMultiplierPPM, &group.CreatedAt, &group.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err == nil {
		group.RateMultiplierPPM = deriveMultiplierPPM(rateMultiplierPPM, rateMultiplier)
		group.RateMultiplier = precision.MultiplierPPMToFloat64(group.RateMultiplierPPM)
	}
	return group, err
}

func (r *GroupRepository) GetByIDs(ids []string) (map[string]*model.Group, error) {
	result := make(map[string]*model.Group)
	if len(ids) == 0 {
		return result, nil
	}

	db := database.GetDB()
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	query := `SELECT id, name, description, rate_multiplier, rate_multiplier_ppm, created_at, updated_at FROM groups WHERE id IN (` + placeholders + `)`

	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		group := &model.Group{}
		var rateMultiplier sql.NullFloat64
		var rateMultiplierPPM sql.NullInt64
		if err := rows.Scan(&group.ID, &group.Name, &group.Description, &rateMultiplier, &rateMultiplierPPM, &group.CreatedAt, &group.UpdatedAt); err != nil {
			return nil, err
		}
		group.RateMultiplierPPM = deriveMultiplierPPM(rateMultiplierPPM, rateMultiplier)
		group.RateMultiplier = precision.MultiplierPPMToFloat64(group.RateMultiplierPPM)
		result[group.ID] = group
	}
	return result, rows.Err()
}

func (r *GroupRepository) GetByName(name string) (*model.Group, error) {
	db := database.GetDB()
	group := &model.Group{}
	var rateMultiplier sql.NullFloat64
	var rateMultiplierPPM sql.NullInt64
	err := db.QueryRow(
		`SELECT id, name, description, rate_multiplier, rate_multiplier_ppm, created_at, updated_at FROM groups WHERE name = ?`, name,
	).Scan(&group.ID, &group.Name, &group.Description, &rateMultiplier, &rateMultiplierPPM, &group.CreatedAt, &group.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err == nil {
		group.RateMultiplierPPM = deriveMultiplierPPM(rateMultiplierPPM, rateMultiplier)
		group.RateMultiplier = precision.MultiplierPPMToFloat64(group.RateMultiplierPPM)
	}
	return group, err
}

func (r *GroupRepository) List() ([]*model.Group, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, name, description, rate_multiplier, rate_multiplier_ppm, created_at, updated_at FROM groups ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*model.Group
	for rows.Next() {
		group := &model.Group{}
		var rateMultiplier sql.NullFloat64
		var rateMultiplierPPM sql.NullInt64
		if err := rows.Scan(&group.ID, &group.Name, &group.Description, &rateMultiplier, &rateMultiplierPPM, &group.CreatedAt, &group.UpdatedAt); err != nil {
			return nil, err
		}
		group.RateMultiplierPPM = deriveMultiplierPPM(rateMultiplierPPM, rateMultiplier)
		group.RateMultiplier = precision.MultiplierPPMToFloat64(group.RateMultiplierPPM)
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (r *GroupRepository) Update(group *model.Group) error {
	db := database.GetDB()
	group.UpdatedAt = time.Now().UTC()
	if group.RateMultiplierPPM <= 0 {
		group.RateMultiplierPPM = precision.DefaultMultiplierPPM
	}
	group.RateMultiplier = precision.MultiplierPPMToFloat64(group.RateMultiplierPPM)
	_, err := db.Exec(
		`UPDATE groups SET name = ?, description = ?, rate_multiplier = ?, rate_multiplier_ppm = ?, updated_at = ? WHERE id = ?`,
		group.Name, group.Description, group.RateMultiplier, group.RateMultiplierPPM, group.UpdatedAt, group.ID,
	)
	return err
}

func (r *GroupRepository) Delete(id string) error {
	db := database.GetDB()
	_, _ = db.Exec(`DELETE FROM user_groups WHERE group_id = ?`, id)
	_, _ = db.Exec(`DELETE FROM channel_groups WHERE group_id = ?`, id)
	_, _ = db.Exec(`DELETE FROM channel_subscription_groups WHERE group_id = ?`, id)
	_, _ = db.Exec(`DELETE FROM channel_usage_groups WHERE group_id = ?`, id)
	_, err := db.Exec(`DELETE FROM groups WHERE id = ?`, id)
	return err
}

func (r *GroupRepository) CountUsers(groupID string) (int, error) {
	db := database.GetDB()
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM user_groups WHERE group_id = ?`, groupID).Scan(&count)
	return count, err
}

func (r *GroupRepository) CountChannels(groupID string) (int, error) {
	db := database.GetDB()
	var count int
	err := db.QueryRow(`
		SELECT COUNT(DISTINCT channel_id)
		FROM (
			SELECT channel_id FROM channel_groups WHERE group_id = ?
			UNION ALL
			SELECT channel_id FROM channel_subscription_groups WHERE group_id = ?
			UNION ALL
			SELECT channel_id FROM channel_usage_groups WHERE group_id = ?
		)
	`, groupID, groupID, groupID).Scan(&count)
	return count, err
}

func (r *GroupRepository) GetMinRateMultiplierByUserID(userID string) (int64, []string, error) {
	db := database.GetDB()
	rows, err := db.Query(`
		SELECT g.id, g.rate_multiplier_ppm, g.rate_multiplier
		FROM groups g
		INNER JOIN user_groups ug ON g.id = ug.group_id
		WHERE ug.user_id = ?
	`, userID)
	if err != nil {
		return precision.DefaultMultiplierPPM, nil, err
	}
	defer rows.Close()

	var groupIDs []string
	minMultiplier := int64(-1)
	for rows.Next() {
		var gid string
		var mult sql.NullFloat64
		var multPPM sql.NullInt64
		if err := rows.Scan(&gid, &multPPM, &mult); err != nil {
			return precision.DefaultMultiplierPPM, nil, err
		}
		groupIDs = append(groupIDs, gid)
		ppm := deriveMultiplierPPM(multPPM, mult)
		if minMultiplier < 0 || ppm < minMultiplier {
			minMultiplier = ppm
		}
	}
	if err := rows.Err(); err != nil {
		return precision.DefaultMultiplierPPM, nil, err
	}

	if len(groupIDs) == 0 {
		return precision.DefaultMultiplierPPM, nil, nil
	}
	return minMultiplier, groupIDs, nil
}

func deriveMultiplierPPM(ppm sql.NullInt64, legacy sql.NullFloat64) int64 {
	if ppm.Valid && ppm.Int64 > 0 {
		return ppm.Int64
	}
	if legacy.Valid {
		return precision.FloatMultiplierToPPM(legacy.Float64)
	}
	return precision.DefaultMultiplierPPM
}
