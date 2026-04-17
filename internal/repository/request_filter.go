package repository

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

type RequestFilterRepository struct{}

func NewRequestFilterRepository() *RequestFilterRepository {
	return &RequestFilterRepository{}
}

func (r *RequestFilterRepository) ListAll() ([]model.RequestFilter, error) {
	rows, err := database.GetDB().Query(`
		SELECT id, name, description, scope, action, match_type, target, replacement, priority, is_enabled,
		       binding_type, channel_ids_json, group_ids_json, rule_mode, execution_phase, operations_json, created_at, updated_at
		  FROM request_filters
		 ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequestFilters(rows)
}

func (r *RequestFilterRepository) ListActive() ([]model.RequestFilter, error) {
	rows, err := database.GetDB().Query(`
		SELECT id, name, description, scope, action, match_type, target, replacement, priority, is_enabled,
		       binding_type, channel_ids_json, group_ids_json, rule_mode, execution_phase, operations_json, created_at, updated_at
		  FROM request_filters
		 WHERE is_enabled = ?
		 ORDER BY priority ASC, id ASC`, 1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequestFilters(rows)
}

func (r *RequestFilterRepository) GetByID(id string) (*model.RequestFilter, error) {
	row := database.GetDB().QueryRow(`
		SELECT id, name, description, scope, action, match_type, target, replacement, priority, is_enabled,
		       binding_type, channel_ids_json, group_ids_json, rule_mode, execution_phase, operations_json, created_at, updated_at
		  FROM request_filters
		 WHERE id = ?`, id)
	return scanRequestFilterRow(row)
}

func (r *RequestFilterRepository) Create(filter *model.RequestFilter) error {
	if filter == nil {
		return nil
	}
	_, err := database.GetDB().Exec(`
		INSERT INTO request_filters (
			id, name, description, scope, action, match_type, target, replacement, priority, is_enabled,
			binding_type, channel_ids_json, group_ids_json, rule_mode, execution_phase, operations_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		filter.ID,
		filter.Name,
		filter.Description,
		filter.Scope,
		filter.Action,
		nullableMatchType(filter.MatchType),
		filter.Target,
		jsonRawString(filter.Replacement),
		filter.Priority,
		boolToInt(filter.IsEnabled),
		filter.BindingType,
		marshalStringSlice(filter.ChannelIDs),
		marshalStringSlice(filter.GroupIDs),
		filter.RuleMode,
		filter.ExecutionPhase,
		marshalOperations(filter.Operations),
		filter.CreatedAt.UTC(),
		filter.UpdatedAt.UTC(),
	)
	return err
}

func (r *RequestFilterRepository) Update(filter *model.RequestFilter) error {
	if filter == nil {
		return nil
	}
	_, err := database.GetDB().Exec(`
		UPDATE request_filters
		   SET name = ?, description = ?, scope = ?, action = ?, match_type = ?, target = ?, replacement = ?, priority = ?, is_enabled = ?,
		       binding_type = ?, channel_ids_json = ?, group_ids_json = ?, rule_mode = ?, execution_phase = ?, operations_json = ?, updated_at = ?
		 WHERE id = ?`,
		filter.Name,
		filter.Description,
		filter.Scope,
		filter.Action,
		nullableMatchType(filter.MatchType),
		filter.Target,
		jsonRawString(filter.Replacement),
		filter.Priority,
		boolToInt(filter.IsEnabled),
		filter.BindingType,
		marshalStringSlice(filter.ChannelIDs),
		marshalStringSlice(filter.GroupIDs),
		filter.RuleMode,
		filter.ExecutionPhase,
		marshalOperations(filter.Operations),
		filter.UpdatedAt.UTC(),
		filter.ID,
	)
	return err
}

func (r *RequestFilterRepository) Delete(id string) error {
	_, err := database.GetDB().Exec(`DELETE FROM request_filters WHERE id = ?`, id)
	return err
}

func scanRequestFilterRow(row *sql.Row) (*model.RequestFilter, error) {
	var (
		filter          model.RequestFilter
		enabledInt      int
		matchType       sql.NullString
		replacement     sql.NullString
		channelIDsJSON  sql.NullString
		groupIDsJSON    sql.NullString
		operationsJSON  sql.NullString
	)
	err := row.Scan(
		&filter.ID,
		&filter.Name,
		&filter.Description,
		&filter.Scope,
		&filter.Action,
		&matchType,
		&filter.Target,
		&replacement,
		&filter.Priority,
		&enabledInt,
		&filter.BindingType,
		&channelIDsJSON,
		&groupIDsJSON,
		&filter.RuleMode,
		&filter.ExecutionPhase,
		&operationsJSON,
		&filter.CreatedAt,
		&filter.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	filter.IsEnabled = enabledInt != 0
	filter.MatchType = parseRequestFilterMatchType(matchType)
	filter.Replacement = parseRawJSON(replacement)
	filter.ChannelIDs = parseStringSlice(channelIDsJSON)
	filter.GroupIDs = parseStringSlice(groupIDsJSON)
	filter.Operations = parseOperations(operationsJSON)
	return &filter, nil
}

func scanRequestFilters(rows *sql.Rows) ([]model.RequestFilter, error) {
	items := make([]model.RequestFilter, 0)
	for rows.Next() {
		var (
			filter          model.RequestFilter
			enabledInt      int
			matchType       sql.NullString
			replacement     sql.NullString
			channelIDsJSON  sql.NullString
			groupIDsJSON    sql.NullString
			operationsJSON  sql.NullString
		)
		if err := rows.Scan(
			&filter.ID,
			&filter.Name,
			&filter.Description,
			&filter.Scope,
			&filter.Action,
			&matchType,
			&filter.Target,
			&replacement,
			&filter.Priority,
			&enabledInt,
			&filter.BindingType,
			&channelIDsJSON,
			&groupIDsJSON,
			&filter.RuleMode,
			&filter.ExecutionPhase,
			&operationsJSON,
			&filter.CreatedAt,
			&filter.UpdatedAt,
		); err != nil {
			return nil, err
		}
		filter.IsEnabled = enabledInt != 0
		filter.MatchType = parseRequestFilterMatchType(matchType)
		filter.Replacement = parseRawJSON(replacement)
		filter.ChannelIDs = parseStringSlice(channelIDsJSON)
		filter.GroupIDs = parseStringSlice(groupIDsJSON)
		filter.Operations = parseOperations(operationsJSON)
		items = append(items, filter)
	}
	return items, rows.Err()
}

func nullableMatchType(value *model.RequestFilterMatchType) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func parseRequestFilterMatchType(value sql.NullString) *model.RequestFilterMatchType {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	matchType := model.RequestFilterMatchType(strings.TrimSpace(value.String))
	return &matchType
}

func marshalStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func parseStringSlice(value sql.NullString) []string {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(value.String), &items); err != nil {
		return nil
	}
	return items
}

func marshalOperations(operations []model.RequestFilterOperation) string {
	if len(operations) == 0 {
		return "[]"
	}
	data, err := json.Marshal(operations)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func parseOperations(value sql.NullString) []model.RequestFilterOperation {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	var operations []model.RequestFilterOperation
	if err := json.Unmarshal([]byte(value.String), &operations); err != nil {
		return nil
	}
	return operations
}

func jsonRawString(value json.RawMessage) string {
	if len(value) == 0 || !json.Valid(value) {
		return ""
	}
	return string(value)
}

func cloneNow(now time.Time) time.Time {
	return now.UTC()
}
