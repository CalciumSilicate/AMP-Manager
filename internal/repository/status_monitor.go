package repository

import (
	"database/sql"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

type StatusMonitorRepositoryInterface interface {
	Create(monitor *model.StatusMonitor) error
	Update(monitor *model.StatusMonitor) error
	Delete(id string) error
	GetByID(id string) (*model.StatusMonitor, error)
	ListAll() ([]*model.StatusMonitor, error)
	ListEnabled() ([]*model.StatusMonitor, error)
	CreateResult(result *model.StatusMonitorResult) error
	ListResultsSince(monitorIDs []string, since time.Time) ([]*model.StatusMonitorResult, error)
	DeleteResultsOlderThan(cutoff time.Time) error
}

var _ StatusMonitorRepositoryInterface = (*StatusMonitorRepository)(nil)

type StatusMonitorRepository struct{}

func NewStatusMonitorRepository() *StatusMonitorRepository {
	return &StatusMonitorRepository{}
}

func (r *StatusMonitorRepository) Create(monitor *model.StatusMonitor) error {
	db := database.GetDB()
	now := time.Now().UTC()
	monitor.ID = uuid.New().String()
	monitor.CreatedAt = now
	monitor.UpdatedAt = now

	_, err := db.Exec(
		`INSERT INTO status_monitors (
			id, name, group_name, target_type, enabled, sort_order, timeout_ms, degraded_threshold_ms,
			request_format, model, channel_id, url, method, headers_json, body_template,
			expected_status_codes_json, expected_substring, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		monitor.ID,
		monitor.Name,
		monitor.GroupName,
		monitor.TargetType,
		monitor.Enabled,
		monitor.SortOrder,
		monitor.TimeoutMs,
		monitor.DegradedThresholdMs,
		monitor.RequestFormat,
		monitor.Model,
		monitor.ChannelID,
		monitor.URL,
		monitor.Method,
		monitor.HeadersJSON,
		monitor.BodyTemplate,
		monitor.ExpectedStatusCodesJSON,
		monitor.ExpectedSubstring,
		monitor.CreatedAt,
		monitor.UpdatedAt,
	)
	return err
}

func (r *StatusMonitorRepository) Update(monitor *model.StatusMonitor) error {
	db := database.GetDB()
	monitor.UpdatedAt = time.Now().UTC()

	_, err := db.Exec(
		`UPDATE status_monitors
		 SET name = ?, group_name = ?, target_type = ?, enabled = ?, sort_order = ?, timeout_ms = ?,
		     degraded_threshold_ms = ?, request_format = ?, model = ?, channel_id = ?, url = ?, method = ?,
		     headers_json = ?, body_template = ?, expected_status_codes_json = ?, expected_substring = ?, updated_at = ?
		 WHERE id = ?`,
		monitor.Name,
		monitor.GroupName,
		monitor.TargetType,
		monitor.Enabled,
		monitor.SortOrder,
		monitor.TimeoutMs,
		monitor.DegradedThresholdMs,
		monitor.RequestFormat,
		monitor.Model,
		monitor.ChannelID,
		monitor.URL,
		monitor.Method,
		monitor.HeadersJSON,
		monitor.BodyTemplate,
		monitor.ExpectedStatusCodesJSON,
		monitor.ExpectedSubstring,
		monitor.UpdatedAt,
		monitor.ID,
	)
	return err
}

func (r *StatusMonitorRepository) Delete(id string) error {
	db := database.GetDB()
	_, err := db.Exec(`DELETE FROM status_monitors WHERE id = ?`, id)
	return err
}

func (r *StatusMonitorRepository) GetByID(id string) (*model.StatusMonitor, error) {
	db := database.GetDB()
	item := &model.StatusMonitor{}
	err := db.QueryRow(
		`SELECT id, name, group_name, target_type, enabled, sort_order, timeout_ms, degraded_threshold_ms,
		        request_format, model, channel_id, url, method, headers_json, body_template,
		        expected_status_codes_json, expected_substring, created_at, updated_at
		   FROM status_monitors
		  WHERE id = ?`,
		id,
	).Scan(
		&item.ID,
		&item.Name,
		&item.GroupName,
		&item.TargetType,
		&item.Enabled,
		&item.SortOrder,
		&item.TimeoutMs,
		&item.DegradedThresholdMs,
		&item.RequestFormat,
		&item.Model,
		&item.ChannelID,
		&item.URL,
		&item.Method,
		&item.HeadersJSON,
		&item.BodyTemplate,
		&item.ExpectedStatusCodesJSON,
		&item.ExpectedSubstring,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (r *StatusMonitorRepository) ListAll() ([]*model.StatusMonitor, error) {
	return r.listByQuery(
		`SELECT id, name, group_name, target_type, enabled, sort_order, timeout_ms, degraded_threshold_ms,
		        request_format, model, channel_id, url, method, headers_json, body_template,
		        expected_status_codes_json, expected_substring, created_at, updated_at
		   FROM status_monitors
		  ORDER BY sort_order ASC, created_at DESC`,
	)
}

func (r *StatusMonitorRepository) ListEnabled() ([]*model.StatusMonitor, error) {
	return r.listByQuery(
		`SELECT id, name, group_name, target_type, enabled, sort_order, timeout_ms, degraded_threshold_ms,
		        request_format, model, channel_id, url, method, headers_json, body_template,
		        expected_status_codes_json, expected_substring, created_at, updated_at
		   FROM status_monitors
		  WHERE enabled = 1
		  ORDER BY sort_order ASC, created_at DESC`,
	)
}

func (r *StatusMonitorRepository) CreateResult(result *model.StatusMonitorResult) error {
	db := database.GetDB()
	now := time.Now().UTC()
	result.ID = uuid.New().String()
	result.CreatedAt = now
	if result.CheckedAt.IsZero() {
		result.CheckedAt = now
	}

	_, err := db.Exec(
		`INSERT INTO status_monitor_results (
			id, monitor_id, status, latency_ms, ttfb_ms, http_status_code, message, endpoint_label, checked_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.ID,
		result.MonitorID,
		result.Status,
		result.LatencyMs,
		result.TTFBMs,
		result.HTTPStatusCode,
		result.Message,
		result.EndpointLabel,
		result.CheckedAt,
		result.CreatedAt,
	)
	return err
}

func (r *StatusMonitorRepository) ListResultsSince(monitorIDs []string, since time.Time) ([]*model.StatusMonitorResult, error) {
	if len(monitorIDs) == 0 {
		return []*model.StatusMonitorResult{}, nil
	}

	db := database.GetDB()
	placeholders := strings.TrimRight(strings.Repeat("?,", len(monitorIDs)), ",")
	args := make([]any, 0, len(monitorIDs)+1)
	for _, id := range monitorIDs {
		args = append(args, id)
	}
	args = append(args, since)

	rows, err := db.Query(
		`SELECT id, monitor_id, status, latency_ms, ttfb_ms, http_status_code, message, endpoint_label, checked_at, created_at
		   FROM status_monitor_results
		  WHERE monitor_id IN (`+placeholders+`) AND checked_at >= ?
		  ORDER BY checked_at DESC, created_at DESC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.StatusMonitorResult, 0)
	for rows.Next() {
		item := &model.StatusMonitorResult{}
		if err := rows.Scan(
			&item.ID,
			&item.MonitorID,
			&item.Status,
			&item.LatencyMs,
			&item.TTFBMs,
			&item.HTTPStatusCode,
			&item.Message,
			&item.EndpointLabel,
			&item.CheckedAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *StatusMonitorRepository) DeleteResultsOlderThan(cutoff time.Time) error {
	db := database.GetDB()
	_, err := db.Exec(`DELETE FROM status_monitor_results WHERE checked_at < ?`, cutoff)
	return err
}

func (r *StatusMonitorRepository) listByQuery(query string, args ...any) ([]*model.StatusMonitor, error) {
	db := database.GetDB()
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.StatusMonitor, 0)
	for rows.Next() {
		item := &model.StatusMonitor{}
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.GroupName,
			&item.TargetType,
			&item.Enabled,
			&item.SortOrder,
			&item.TimeoutMs,
			&item.DegradedThresholdMs,
			&item.RequestFormat,
			&item.Model,
			&item.ChannelID,
			&item.URL,
			&item.Method,
			&item.HeadersJSON,
			&item.BodyTemplate,
			&item.ExpectedStatusCodesJSON,
			&item.ExpectedSubstring,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
