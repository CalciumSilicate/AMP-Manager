package repository

import (
	"database/sql"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

type AnnouncementRepositoryInterface interface {
	Create(announcement *model.Announcement) error
	Update(announcement *model.Announcement) error
	Delete(id string) error
	GetByID(id string) (*model.Announcement, error)
	ListAll() ([]*model.Announcement, error)
	ListEnabledByAudiences(audiences []model.AnnouncementAudience) ([]*model.Announcement, error)
	GetReadMap(userID string, announcementIDs []string) (map[string]time.Time, error)
	MarkRead(announcementID, userID string) error
}

var _ AnnouncementRepositoryInterface = (*AnnouncementRepository)(nil)

type AnnouncementRepository struct{}

func NewAnnouncementRepository() *AnnouncementRepository {
	return &AnnouncementRepository{}
}

func (r *AnnouncementRepository) Create(announcement *model.Announcement) error {
	db := database.GetDB()
	now := time.Now().UTC()
	announcement.ID = uuid.New().String()
	announcement.CreatedAt = now
	announcement.UpdatedAt = now

	_, err := db.Exec(
		`INSERT INTO announcements (id, title, content, audience, pinned, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		announcement.ID,
		announcement.Title,
		announcement.Content,
		announcement.Audience,
		announcement.Pinned,
		announcement.Enabled,
		announcement.CreatedAt,
		announcement.UpdatedAt,
	)
	return err
}

func (r *AnnouncementRepository) Update(announcement *model.Announcement) error {
	db := database.GetDB()
	announcement.UpdatedAt = time.Now().UTC()
	_, err := db.Exec(
		`UPDATE announcements
		 SET title = ?, content = ?, audience = ?, pinned = ?, enabled = ?, updated_at = ?
		 WHERE id = ?`,
		announcement.Title,
		announcement.Content,
		announcement.Audience,
		announcement.Pinned,
		announcement.Enabled,
		announcement.UpdatedAt,
		announcement.ID,
	)
	return err
}

func (r *AnnouncementRepository) Delete(id string) error {
	db := database.GetDB()
	_, err := db.Exec(`DELETE FROM announcements WHERE id = ?`, id)
	return err
}

func (r *AnnouncementRepository) GetByID(id string) (*model.Announcement, error) {
	db := database.GetDB()
	announcement := &model.Announcement{}
	err := db.QueryRow(
		`SELECT id, title, content, audience, pinned, enabled, created_at, updated_at
		 FROM announcements
		 WHERE id = ?`,
		id,
	).Scan(
		&announcement.ID,
		&announcement.Title,
		&announcement.Content,
		&announcement.Audience,
		&announcement.Pinned,
		&announcement.Enabled,
		&announcement.CreatedAt,
		&announcement.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return announcement, err
}

func (r *AnnouncementRepository) ListAll() ([]*model.Announcement, error) {
	return r.listByQuery(
		`SELECT id, title, content, audience, pinned, enabled, created_at, updated_at
		 FROM announcements
		 ORDER BY pinned DESC, created_at DESC`,
	)
}

func (r *AnnouncementRepository) ListEnabledByAudiences(audiences []model.AnnouncementAudience) ([]*model.Announcement, error) {
	if len(audiences) == 0 {
		return []*model.Announcement{}, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(audiences)), ",")
	args := make([]any, 0, len(audiences))
	for _, audience := range audiences {
		args = append(args, audience)
	}

	query := `SELECT id, title, content, audience, pinned, enabled, created_at, updated_at
		FROM announcements
		WHERE enabled = 1 AND audience IN (` + placeholders + `)
		ORDER BY pinned DESC, created_at DESC`

	return r.listByQuery(query, args...)
}

func (r *AnnouncementRepository) GetReadMap(userID string, announcementIDs []string) (map[string]time.Time, error) {
	readMap := make(map[string]time.Time)
	if userID == "" || len(announcementIDs) == 0 {
		return readMap, nil
	}

	db := database.GetDB()
	placeholders := strings.TrimRight(strings.Repeat("?,", len(announcementIDs)), ",")
	args := make([]any, 0, len(announcementIDs)+1)
	args = append(args, userID)
	for _, id := range announcementIDs {
		args = append(args, id)
	}

	rows, err := db.Query(
		`SELECT announcement_id, read_at
		 FROM announcement_reads
		 WHERE user_id = ? AND announcement_id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var announcementID string
		var readAt time.Time
		if err := rows.Scan(&announcementID, &readAt); err != nil {
			return nil, err
		}
		readMap[announcementID] = readAt
	}
	return readMap, rows.Err()
}

func (r *AnnouncementRepository) MarkRead(announcementID, userID string) error {
	db := database.GetDB()
	now := time.Now().UTC()
	_, err := db.Exec(
		`INSERT INTO announcement_reads (announcement_id, user_id, read_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(announcement_id, user_id) DO UPDATE SET read_at = excluded.read_at`,
		announcementID,
		userID,
		now,
	)
	return err
}

func (r *AnnouncementRepository) listByQuery(query string, args ...any) ([]*model.Announcement, error) {
	db := database.GetDB()
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.Announcement, 0)
	for rows.Next() {
		item := &model.Announcement{}
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.Content,
			&item.Audience,
			&item.Pinned,
			&item.Enabled,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
