package repository

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

type ErrorRuleRepository struct{}

func NewErrorRuleRepository() *ErrorRuleRepository {
	return &ErrorRuleRepository{}
}

func (r *ErrorRuleRepository) ListAll() ([]model.ErrorRule, error) {
	rows, err := database.GetDB().Query(`
		SELECT id, name, description, request_type, upstream_status, pattern, match_type, category, priority,
		       is_enabled, is_default, override_status_code, override_message, override_response, created_at, updated_at
		  FROM error_rules
		 ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanErrorRules(rows)
}

func (r *ErrorRuleRepository) ListActive() ([]model.ErrorRule, error) {
	rows, err := database.GetDB().Query(`
		SELECT id, name, description, request_type, upstream_status, pattern, match_type, category, priority,
		       is_enabled, is_default, override_status_code, override_message, override_response, created_at, updated_at
		  FROM error_rules
		 WHERE is_enabled = ?
		 ORDER BY priority ASC, id ASC`, 1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanErrorRules(rows)
}

func (r *ErrorRuleRepository) GetByID(id string) (*model.ErrorRule, error) {
	row := database.GetDB().QueryRow(`
		SELECT id, name, description, request_type, upstream_status, pattern, match_type, category, priority,
		       is_enabled, is_default, override_status_code, override_message, override_response, created_at, updated_at
		  FROM error_rules
		 WHERE id = ?`, id)
	return scanErrorRuleRow(row)
}

func (r *ErrorRuleRepository) GetByPattern(pattern string) (*model.ErrorRule, error) {
	row := database.GetDB().QueryRow(`
		SELECT id, name, description, request_type, upstream_status, pattern, match_type, category, priority,
		       is_enabled, is_default, override_status_code, override_message, override_response, created_at, updated_at
		  FROM error_rules
		 WHERE pattern = ?`, pattern)
	return scanErrorRuleRow(row)
}

func (r *ErrorRuleRepository) Create(rule *model.ErrorRule) error {
	if rule == nil {
		return nil
	}
	_, err := database.GetDB().Exec(`
		INSERT INTO error_rules (
			id, name, description, request_type, upstream_status, pattern, match_type, category, priority,
			is_enabled, is_default, override_status_code, override_message, override_response, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID,
		rule.Name,
		rule.Description,
		rule.RequestType,
		rule.UpstreamStatus,
		rule.Pattern,
		rule.MatchType,
		rule.Category,
		rule.Priority,
		boolToInt(rule.IsEnabled),
		boolToInt(rule.IsDefault),
		rule.OverrideStatusCode,
		rule.OverrideMessage,
		jsonString(rule.OverrideResponse),
		rule.CreatedAt.UTC(),
		rule.UpdatedAt.UTC(),
	)
	return err
}

func (r *ErrorRuleRepository) Update(rule *model.ErrorRule) error {
	if rule == nil {
		return nil
	}
	_, err := database.GetDB().Exec(`
		UPDATE error_rules
		   SET name = ?, description = ?, request_type = ?, upstream_status = ?, pattern = ?, match_type = ?, category = ?, priority = ?,
		       is_enabled = ?, is_default = ?, override_status_code = ?, override_message = ?, override_response = ?, updated_at = ?
		 WHERE id = ?`,
		rule.Name,
		rule.Description,
		rule.RequestType,
		rule.UpstreamStatus,
		rule.Pattern,
		rule.MatchType,
		rule.Category,
		rule.Priority,
		boolToInt(rule.IsEnabled),
		boolToInt(rule.IsDefault),
		rule.OverrideStatusCode,
		rule.OverrideMessage,
		jsonString(rule.OverrideResponse),
		rule.UpdatedAt.UTC(),
		rule.ID,
	)
	return err
}

func (r *ErrorRuleRepository) Delete(id string) error {
	_, err := database.GetDB().Exec(`DELETE FROM error_rules WHERE id = ?`, id)
	return err
}

func (r *ErrorRuleRepository) DeleteDefaultNotInPatterns(patterns []string) error {
	if len(patterns) == 0 {
		_, err := database.GetDB().Exec(`DELETE FROM error_rules WHERE is_default = ?`, 1)
		return err
	}

	placeholders := make([]string, 0, len(patterns))
	args := make([]any, 0, len(patterns)+1)
	args = append(args, 1)
	for _, pattern := range patterns {
		placeholders = append(placeholders, "?")
		args = append(args, pattern)
	}

	query := `DELETE FROM error_rules WHERE is_default = ? AND pattern NOT IN (` + strings.Join(placeholders, ",") + `)`
	_, err := database.GetDB().Exec(query, args...)
	return err
}

func scanErrorRuleRow(row *sql.Row) (*model.ErrorRule, error) {
	var (
		rule               model.ErrorRule
		enabledInt         int
		defaultInt         int
		overrideStatusCode sql.NullInt64
		overrideResponse   sql.NullString
	)
	err := row.Scan(
		&rule.ID,
		&rule.Name,
		&rule.Description,
		&rule.RequestType,
		&rule.UpstreamStatus,
		&rule.Pattern,
		&rule.MatchType,
		&rule.Category,
		&rule.Priority,
		&enabledInt,
		&defaultInt,
		&overrideStatusCode,
		&rule.OverrideMessage,
		&overrideResponse,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rule.IsEnabled = enabledInt != 0
	rule.IsDefault = defaultInt != 0
	if overrideStatusCode.Valid {
		value := int(overrideStatusCode.Int64)
		rule.OverrideStatusCode = &value
	}
	rule.OverrideResponse = parseRawJSON(overrideResponse)
	return &rule, nil
}

func scanErrorRules(rows *sql.Rows) ([]model.ErrorRule, error) {
	items := make([]model.ErrorRule, 0)
	for rows.Next() {
		var (
			rule               model.ErrorRule
			enabledInt         int
			defaultInt         int
			overrideStatusCode sql.NullInt64
			overrideResponse   sql.NullString
		)
		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Description,
			&rule.RequestType,
			&rule.UpstreamStatus,
			&rule.Pattern,
			&rule.MatchType,
			&rule.Category,
			&rule.Priority,
			&enabledInt,
			&defaultInt,
			&overrideStatusCode,
			&rule.OverrideMessage,
			&overrideResponse,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rule.IsEnabled = enabledInt != 0
		rule.IsDefault = defaultInt != 0
		if overrideStatusCode.Valid {
			value := int(overrideStatusCode.Int64)
			rule.OverrideStatusCode = &value
		}
		rule.OverrideResponse = parseRawJSON(overrideResponse)
		items = append(items, rule)
	}
	return items, rows.Err()
}

func parseRawJSON(value sql.NullString) json.RawMessage {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value.String)
	if !json.Valid([]byte(trimmed)) {
		return nil
	}
	return json.RawMessage(trimmed)
}

func jsonString(value json.RawMessage) string {
	if len(value) == 0 || !json.Valid(value) {
		return ""
	}
	return string(value)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func cloneTime(value time.Time) time.Time {
	return value.UTC()
}
