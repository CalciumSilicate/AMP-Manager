package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"ampmanager/internal/database"

	"golang.org/x/sync/singleflight"
)

const (
	dashboardScopeGlobal            = "global"
	dashboardAggregateStateKey      = "default"
	dashboardAggregateSchemaVersion = "dashboard-v2"
)

var dashboardAggregateEnsureGroup singleflight.Group

type DashboardProjectionInput struct {
	RequestID                string
	UserID                   string
	StartTime                time.Time
	MappedModel              string
	OriginalModel            string
	StatusCode               int
	LatencyMs                int64
	TTFBMs                   int64
	InputTokens              int64
	OutputTokens             int64
	CacheReadInputTokens     int64
	CacheCreationInputTokens int64
	CostMicros               int64
}

type dashboardAggregateState struct {
	StateKey      string
	SchemaVersion string
	LocationName  string
	Status        string
	LastRequestAt sql.NullTime
	LastRequestID string
	LastBackfillAt sql.NullTime
	ErrorMessage  string
}

type dashboardProjectionSnapshot struct {
	RequestID                string
	UserID                   string
	MinuteBucket             time.Time
	CompletedMinuteBucket    time.Time
	DayBucket                string
	EffectiveModel           string
	ProviderGroup            string
	RequestCount             int64
	InputTokens              int64
	OutputTokens             int64
	TotalTokens              int64
	CostMicros               int64
	ErrorCount               int64
	CacheReadInputTokens     int64
	CacheCreationInputTokens int64
	LatencyMs                int64
	TTFBMs                   int64
}

type dashboardDayAggregateRow struct {
	DayBucket    string
	RequestCount int64
	CostMicros   int64
}

type dashboardModelAggregateRow struct {
	Model        string
	RequestCount int64
	CostMicros   int64
}

type dashboardProviderAggregateRow struct {
	Provider                 string
	TotalInputTokens         int64
	CacheReadTokens          int64
	CacheCreationTokens      int64
	RequestCount             int64
}

func dashboardScopeID(scopeType, userID string) string {
	if scopeType == dashboardScopeGlobal {
		return dashboardScopeGlobal
	}
	return userID
}

func dashboardEffectiveModel(mappedModel, originalModel string) string {
	modelName := strings.TrimSpace(mappedModel)
	if modelName != "" {
		return modelName
	}
	modelName = strings.TrimSpace(originalModel)
	if modelName != "" {
		return modelName
	}
	return "unknown"
}

func dashboardProviderGroup(modelName string) string {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.HasPrefix(normalized, "claude"):
		return "Claude"
	case strings.HasPrefix(normalized, "gpt"),
		strings.HasPrefix(normalized, "o1"),
		strings.HasPrefix(normalized, "o3"),
		strings.HasPrefix(normalized, "o4"),
		strings.HasPrefix(normalized, "chatgpt"):
		return "OpenAI"
	case strings.HasPrefix(normalized, "gemini"):
		return "Gemini"
	default:
		return "Other"
	}
}

func dashboardDayBucket(ts time.Time, location *time.Location) string {
	location = normalizeDashboardLocation(location)
	return ts.In(location).Format("2006-01-02")
}

func dashboardCompletedMinuteBucket(startTime time.Time, latencyMs int64) time.Time {
	if latencyMs <= 0 {
		return startTime.UTC().Truncate(time.Minute)
	}
	return startTime.UTC().Add(time.Duration(latencyMs) * time.Millisecond).Truncate(time.Minute)
}

func buildDashboardProjectionSnapshot(input DashboardProjectionInput, location *time.Location) dashboardProjectionSnapshot {
	effectiveModel := dashboardEffectiveModel(input.MappedModel, input.OriginalModel)
	errorCount := int64(0)
	if input.StatusCode >= 400 {
		errorCount = 1
	}
	return dashboardProjectionSnapshot{
		RequestID:                input.RequestID,
		UserID:                   input.UserID,
		MinuteBucket:             input.StartTime.UTC().Truncate(time.Minute),
		CompletedMinuteBucket:    dashboardCompletedMinuteBucket(input.StartTime.UTC(), input.LatencyMs),
		DayBucket:                dashboardDayBucket(input.StartTime.UTC(), location),
		EffectiveModel:           effectiveModel,
		ProviderGroup:            dashboardProviderGroup(effectiveModel),
		RequestCount:             1,
		InputTokens:              input.InputTokens,
		OutputTokens:             input.OutputTokens,
		TotalTokens:              input.InputTokens + input.OutputTokens,
		CostMicros:               input.CostMicros,
		ErrorCount:               errorCount,
		CacheReadInputTokens:     input.CacheReadInputTokens,
		CacheCreationInputTokens: input.CacheCreationInputTokens,
		LatencyMs:                input.LatencyMs,
		TTFBMs:                   input.TTFBMs,
	}
}

func dashboardProjectionRowExists(err error) bool {
	return err == nil
}

func loadDashboardProjectionSnapshotTx(tx *sql.Tx, requestID string) (*dashboardProjectionSnapshot, error) {
	var snapshot dashboardProjectionSnapshot
	err := tx.QueryRow(`
		SELECT request_id, user_id, minute_bucket, completed_minute_bucket, day_bucket, effective_model, provider_group,
		       request_count, input_tokens, output_tokens, total_tokens, cost_micros, error_count,
		       cache_read_input_tokens, cache_creation_input_tokens, latency_ms, ttfb_ms
		FROM global_request_metric_projections
		WHERE request_id = ?
	`, requestID).Scan(
		&snapshot.RequestID,
		&snapshot.UserID,
		&snapshot.MinuteBucket,
		&snapshot.CompletedMinuteBucket,
		&snapshot.DayBucket,
		&snapshot.EffectiveModel,
		&snapshot.ProviderGroup,
		&snapshot.RequestCount,
		&snapshot.InputTokens,
		&snapshot.OutputTokens,
		&snapshot.TotalTokens,
		&snapshot.CostMicros,
		&snapshot.ErrorCount,
		&snapshot.CacheReadInputTokens,
		&snapshot.CacheCreationInputTokens,
		&snapshot.LatencyMs,
		&snapshot.TTFBMs,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func upsertDashboardAggregateStateTx(tx *sql.Tx, state dashboardAggregateState) error {
	_, err := tx.Exec(`
		INSERT INTO dashboard_aggregate_state (
			state_key, schema_version, location_name, status, last_request_at, last_request_id, last_backfill_at, error_message, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(state_key) DO UPDATE SET
			schema_version = excluded.schema_version,
			location_name = excluded.location_name,
			status = excluded.status,
			last_request_at = excluded.last_request_at,
			last_request_id = excluded.last_request_id,
			last_backfill_at = excluded.last_backfill_at,
			error_message = excluded.error_message,
			updated_at = excluded.updated_at
	`,
		state.StateKey,
		state.SchemaVersion,
		state.LocationName,
		state.Status,
		nullTimeValue(state.LastRequestAt),
		state.LastRequestID,
		nullTimeValue(state.LastBackfillAt),
		state.ErrorMessage,
		time.Now().UTC(),
	)
	return err
}

func nullTimeValue(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func upsertMinuteMetricDeltaTx(tx *sql.Tx, scopeType, scopeID string, minute time.Time, snapshot dashboardProjectionSnapshot, sign int64) error {
	if sign == 0 {
		return nil
	}
	latencySum := int64(0)
	latencySamples := int64(0)
	if snapshot.LatencyMs > 0 {
		latencySum = snapshot.LatencyMs * sign
		latencySamples = sign
	}
	ttfbSum := int64(0)
	ttfbSamples := int64(0)
	if snapshot.TTFBMs > 0 {
		ttfbSum = snapshot.TTFBMs * sign
		ttfbSamples = sign
	}
	_, err := tx.Exec(`
		INSERT INTO dashboard_minute_metrics (
			scope_type, scope_id, minute_bucket, request_count_sum, input_tokens_sum, output_tokens_sum, total_tokens_sum,
			cost_micros_sum, error_count_sum, cache_read_input_tokens_sum, cache_creation_input_tokens_sum,
			latency_sum_ms, latency_sample_count, ttfb_sum_ms, ttfb_sample_count, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, minute_bucket) DO UPDATE SET
			request_count_sum = dashboard_minute_metrics.request_count_sum + excluded.request_count_sum,
			input_tokens_sum = dashboard_minute_metrics.input_tokens_sum + excluded.input_tokens_sum,
			output_tokens_sum = dashboard_minute_metrics.output_tokens_sum + excluded.output_tokens_sum,
			total_tokens_sum = dashboard_minute_metrics.total_tokens_sum + excluded.total_tokens_sum,
			cost_micros_sum = dashboard_minute_metrics.cost_micros_sum + excluded.cost_micros_sum,
			error_count_sum = dashboard_minute_metrics.error_count_sum + excluded.error_count_sum,
			cache_read_input_tokens_sum = dashboard_minute_metrics.cache_read_input_tokens_sum + excluded.cache_read_input_tokens_sum,
			cache_creation_input_tokens_sum = dashboard_minute_metrics.cache_creation_input_tokens_sum + excluded.cache_creation_input_tokens_sum,
			latency_sum_ms = dashboard_minute_metrics.latency_sum_ms + excluded.latency_sum_ms,
			latency_sample_count = dashboard_minute_metrics.latency_sample_count + excluded.latency_sample_count,
			ttfb_sum_ms = dashboard_minute_metrics.ttfb_sum_ms + excluded.ttfb_sum_ms,
			ttfb_sample_count = dashboard_minute_metrics.ttfb_sample_count + excluded.ttfb_sample_count,
			updated_at = excluded.updated_at
	`,
		scopeType,
		scopeID,
		minute.UTC(),
		snapshot.RequestCount*sign,
		snapshot.InputTokens*sign,
		snapshot.OutputTokens*sign,
		snapshot.TotalTokens*sign,
		snapshot.CostMicros*sign,
		snapshot.ErrorCount*sign,
		snapshot.CacheReadInputTokens*sign,
		snapshot.CacheCreationInputTokens*sign,
		latencySum,
		latencySamples,
		ttfbSum,
		ttfbSamples,
		time.Now().UTC(),
	)
	return err
}

func upsertConcurrencyDeltaTx(tx *sql.Tx, scopeType, scopeID string, minute time.Time, delta int64) error {
	if delta == 0 {
		return nil
	}
	_, err := tx.Exec(`
		INSERT INTO dashboard_minute_metrics (
			scope_type, scope_id, minute_bucket, concurrency_delta_sum, updated_at
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, minute_bucket) DO UPDATE SET
			concurrency_delta_sum = dashboard_minute_metrics.concurrency_delta_sum + excluded.concurrency_delta_sum,
			updated_at = excluded.updated_at
	`,
		scopeType,
		scopeID,
		minute.UTC(),
		delta,
		time.Now().UTC(),
	)
	return err
}

func upsertDayMetricDeltaTx(tx *sql.Tx, scopeType, scopeID, dayBucket string, snapshot dashboardProjectionSnapshot, sign int64) error {
	if sign == 0 {
		return nil
	}
	_, err := tx.Exec(`
		INSERT INTO dashboard_day_metrics (
			scope_type, scope_id, day_bucket, request_count_sum, cost_micros_sum, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, day_bucket) DO UPDATE SET
			request_count_sum = dashboard_day_metrics.request_count_sum + excluded.request_count_sum,
			cost_micros_sum = dashboard_day_metrics.cost_micros_sum + excluded.cost_micros_sum,
			updated_at = excluded.updated_at
	`,
		scopeType,
		scopeID,
		dayBucket,
		snapshot.RequestCount*sign,
		snapshot.CostMicros*sign,
		time.Now().UTC(),
	)
	return err
}

func upsertModelDayMetricDeltaTx(tx *sql.Tx, scopeType, scopeID, dayBucket string, snapshot dashboardProjectionSnapshot, sign int64) error {
	if sign == 0 {
		return nil
	}
	_, err := tx.Exec(`
		INSERT INTO dashboard_model_day_metrics (
			scope_type, scope_id, day_bucket, model, request_count_sum, cost_micros_sum, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, day_bucket, model) DO UPDATE SET
			request_count_sum = dashboard_model_day_metrics.request_count_sum + excluded.request_count_sum,
			cost_micros_sum = dashboard_model_day_metrics.cost_micros_sum + excluded.cost_micros_sum,
			updated_at = excluded.updated_at
	`,
		scopeType,
		scopeID,
		dayBucket,
		snapshot.EffectiveModel,
		snapshot.RequestCount*sign,
		snapshot.CostMicros*sign,
		time.Now().UTC(),
	)
	return err
}

func upsertProviderDayMetricDeltaTx(tx *sql.Tx, scopeType, scopeID, dayBucket string, snapshot dashboardProjectionSnapshot, sign int64) error {
	if sign == 0 {
		return nil
	}
	_, err := tx.Exec(`
		INSERT INTO dashboard_provider_day_metrics (
			scope_type, scope_id, day_bucket, provider, request_count_sum, total_input_tokens_sum,
			cache_read_input_tokens_sum, cache_creation_input_tokens_sum, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope_type, scope_id, day_bucket, provider) DO UPDATE SET
			request_count_sum = dashboard_provider_day_metrics.request_count_sum + excluded.request_count_sum,
			total_input_tokens_sum = dashboard_provider_day_metrics.total_input_tokens_sum + excluded.total_input_tokens_sum,
			cache_read_input_tokens_sum = dashboard_provider_day_metrics.cache_read_input_tokens_sum + excluded.cache_read_input_tokens_sum,
			cache_creation_input_tokens_sum = dashboard_provider_day_metrics.cache_creation_input_tokens_sum + excluded.cache_creation_input_tokens_sum,
			updated_at = excluded.updated_at
	`,
		scopeType,
		scopeID,
		dayBucket,
		snapshot.ProviderGroup,
		snapshot.RequestCount*sign,
		snapshot.InputTokens*sign,
		snapshot.CacheReadInputTokens*sign,
		snapshot.CacheCreationInputTokens*sign,
		time.Now().UTC(),
	)
	return err
}

func applyDashboardAggregateDeltaTx(tx *sql.Tx, snapshot dashboardProjectionSnapshot, sign int64) error {
	scopeTypes := []string{dashboardScopeGlobal}
	if strings.TrimSpace(snapshot.UserID) != "" {
		scopeTypes = append(scopeTypes, "user")
	}
	for _, scopeType := range scopeTypes {
		scopeID := dashboardScopeID(scopeType, snapshot.UserID)
		if err := upsertMinuteMetricDeltaTx(tx, scopeType, scopeID, snapshot.MinuteBucket, snapshot, sign); err != nil {
			return err
		}
		if snapshot.LatencyMs > 0 {
			if err := upsertConcurrencyDeltaTx(tx, scopeType, scopeID, snapshot.MinuteBucket, sign); err != nil {
				return err
			}
			if err := upsertConcurrencyDeltaTx(tx, scopeType, scopeID, snapshot.CompletedMinuteBucket.Add(time.Minute), -sign); err != nil {
				return err
			}
		}
		if err := upsertDayMetricDeltaTx(tx, scopeType, scopeID, snapshot.DayBucket, snapshot, sign); err != nil {
			return err
		}
		if err := upsertModelDayMetricDeltaTx(tx, scopeType, scopeID, snapshot.DayBucket, snapshot, sign); err != nil {
			return err
		}
		if err := upsertProviderDayMetricDeltaTx(tx, scopeType, scopeID, snapshot.DayBucket, snapshot, sign); err != nil {
			return err
		}
	}
	return nil
}

func upsertDashboardProjectionSnapshotTx(tx *sql.Tx, snapshot dashboardProjectionSnapshot) error {
	_, err := tx.Exec(`
		INSERT INTO global_request_metric_projections (
			request_id, user_id, minute_bucket, completed_minute_bucket, day_bucket, effective_model, provider_group,
			request_count, input_tokens, output_tokens, total_tokens, cost_micros, error_count,
			cache_read_input_tokens, cache_creation_input_tokens, latency_ms, ttfb_ms, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(request_id) DO UPDATE SET
			user_id = excluded.user_id,
			minute_bucket = excluded.minute_bucket,
			completed_minute_bucket = excluded.completed_minute_bucket,
			day_bucket = excluded.day_bucket,
			effective_model = excluded.effective_model,
			provider_group = excluded.provider_group,
			request_count = excluded.request_count,
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			total_tokens = excluded.total_tokens,
			cost_micros = excluded.cost_micros,
			error_count = excluded.error_count,
			cache_read_input_tokens = excluded.cache_read_input_tokens,
			cache_creation_input_tokens = excluded.cache_creation_input_tokens,
			latency_ms = excluded.latency_ms,
			ttfb_ms = excluded.ttfb_ms,
			updated_at = excluded.updated_at
	`,
		snapshot.RequestID,
		snapshot.UserID,
		snapshot.MinuteBucket.UTC(),
		snapshot.CompletedMinuteBucket.UTC(),
		snapshot.DayBucket,
		snapshot.EffectiveModel,
		snapshot.ProviderGroup,
		snapshot.RequestCount,
		snapshot.InputTokens,
		snapshot.OutputTokens,
		snapshot.TotalTokens,
		snapshot.CostMicros,
		snapshot.ErrorCount,
		snapshot.CacheReadInputTokens,
		snapshot.CacheCreationInputTokens,
		snapshot.LatencyMs,
		snapshot.TTFBMs,
		time.Now().UTC(),
	)
	return err
}

func (r *RequestLogRepository) SyncDashboardProjectionTx(tx *sql.Tx, input DashboardProjectionInput, location *time.Location) error {
	location = normalizeDashboardLocation(location)
	oldSnapshot, err := loadDashboardProjectionSnapshotTx(tx, input.RequestID)
	if err != nil {
		return err
	}
	newSnapshot := buildDashboardProjectionSnapshot(input, location)
	if err := upsertDashboardProjectionSnapshotTx(tx, newSnapshot); err != nil {
		return err
	}
	if oldSnapshot != nil {
		if err := enqueueDashboardAggregateDeltaTx(tx, *oldSnapshot, -1); err != nil {
			return err
		}
	}
	if err := enqueueDashboardAggregateDeltaTx(tx, newSnapshot, 1); err != nil {
		return err
	}
	return nil
}

func minuteBucketSQLExpr(column string) string {
	if database.IsPostgres() {
		return fmt.Sprintf("DATE_TRUNC('minute', %s)", column)
	}
	return fmt.Sprintf("STRFTIME('%%Y-%%m-%%d %%H:%%M:00', SUBSTR(%s, 1, 19))", column)
}

func completedMinuteBucketSQLExpr(createdColumn, latencyColumn string) string {
	if database.IsPostgres() {
		return fmt.Sprintf("DATE_TRUNC('minute', %s + (GREATEST(COALESCE(%s, 0), 0) * INTERVAL '1 millisecond'))", createdColumn, latencyColumn)
	}
	return fmt.Sprintf(
		"STRFTIME('%%Y-%%m-%%d %%H:%%M:00', JULIANDAY(SUBSTR(%s, 1, 19)) + ((CASE WHEN COALESCE(%s, 0) > 0 THEN COALESCE(%s, 0) ELSE 0 END) / 86400000.0))",
		createdColumn,
		latencyColumn,
		latencyColumn,
	)
}

func minutePlusOneSQLExpr(column string) string {
	if database.IsPostgres() {
		return fmt.Sprintf("(%s + INTERVAL '1 minute')", column)
	}
	return fmt.Sprintf("DATETIME(%s, '+1 minute')", column)
}

func dashboardEffectiveModelSQLExpr() string {
	return "COALESCE(NULLIF(TRIM(mapped_model), ''), NULLIF(TRIM(original_model), ''), 'unknown')"
}

func dashboardProviderGroupSQLExpr(modelExpr string) string {
	return fmt.Sprintf(`CASE
		WHEN LOWER(%s) LIKE 'claude%%' THEN 'Claude'
		WHEN LOWER(%s) LIKE 'gpt%%'
		  OR LOWER(%s) LIKE 'o1%%'
		  OR LOWER(%s) LIKE 'o3%%'
		  OR LOWER(%s) LIKE 'o4%%'
		  OR LOWER(%s) LIKE 'chatgpt%%' THEN 'OpenAI'
		WHEN LOWER(%s) LIKE 'gemini%%' THEN 'Gemini'
		ELSE 'Other'
	END`, modelExpr, modelExpr, modelExpr, modelExpr, modelExpr, modelExpr, modelExpr)
}

func loadDashboardAggregateState() (*dashboardAggregateState, error) {
	var state dashboardAggregateState
	err := database.GetDB().QueryRow(`
		SELECT state_key, schema_version, location_name, status, last_request_at, last_request_id, last_backfill_at, error_message
		FROM dashboard_aggregate_state
		WHERE state_key = ?
	`, dashboardAggregateStateKey).Scan(
		&state.StateKey,
		&state.SchemaVersion,
		&state.LocationName,
		&state.Status,
		&state.LastRequestAt,
		&state.LastRequestID,
		&state.LastBackfillAt,
		&state.ErrorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *RequestLogRepository) EnsureDashboardAggregatesReady(location *time.Location) error {
	if database.GetDB() == nil {
		return nil
	}
	location = normalizeDashboardLocation(location)
	key := dashboardAggregateSchemaVersion + "|" + location.String()
	_, err, _ := dashboardAggregateEnsureGroup.Do(key, func() (any, error) {
		state, err := loadDashboardAggregateState()
		if err != nil {
			return nil, err
		}
		if state != nil && state.SchemaVersion == dashboardAggregateSchemaVersion && state.LocationName == location.String() && state.Status == "ready" {
			return nil, nil
		}
		return nil, r.rebuildDashboardAggregates(location)
	})
	return err
}

func (r *RequestLogRepository) rebuildDashboardAggregates(location *time.Location) error {
	location = normalizeDashboardLocation(location)
	tx, err := database.GetDB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rebuildStartedAt := time.Now().UTC()
	if err := upsertDashboardAggregateStateTx(tx, dashboardAggregateState{
		StateKey:      dashboardAggregateStateKey,
		SchemaVersion: dashboardAggregateSchemaVersion,
		LocationName:  location.String(),
		Status:        "rebuilding",
	}); err != nil {
		return err
	}

	clearStatements := []string{
		`DELETE FROM dashboard_aggregate_jobs`,
		`DELETE FROM dashboard_timing_histograms`,
		`DELETE FROM dashboard_provider_day_metrics`,
		`DELETE FROM dashboard_model_day_metrics`,
		`DELETE FROM dashboard_day_metrics`,
		`DELETE FROM dashboard_minute_metrics`,
		`DELETE FROM global_request_metric_projections`,
	}
	for _, statement := range clearStatements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}

	if err := rebuildDashboardProjectionFactsTx(tx, location, rebuildStartedAt); err != nil {
		return err
	}
	if err := rebuildDashboardMinuteMetricsTx(tx, rebuildStartedAt); err != nil {
		return err
	}
	if err := rebuildDashboardDayMetricsTx(tx, rebuildStartedAt); err != nil {
		return err
	}
	if err := rebuildDashboardModelDayMetricsTx(tx, rebuildStartedAt); err != nil {
		return err
	}
	if err := rebuildDashboardProviderDayMetricsTx(tx, rebuildStartedAt); err != nil {
		return err
	}
	if err := rebuildDashboardTimingHistogramsTx(tx, rebuildStartedAt); err != nil {
		return err
	}

	state := dashboardAggregateState{
		StateKey:      dashboardAggregateStateKey,
		SchemaVersion: dashboardAggregateSchemaVersion,
		LocationName:  location.String(),
		Status:        "ready",
		LastBackfillAt: sql.NullTime{Time: rebuildStartedAt, Valid: true},
	}

	row := tx.QueryRow(`
		SELECT created_at, id
		FROM request_logs
		WHERE status <> 'pending'
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`)
	var lastRequestAt time.Time
	var lastRequestID string
	if err := row.Scan(&lastRequestAt, &lastRequestID); err == nil {
		state.LastRequestAt = sql.NullTime{Time: lastRequestAt.UTC(), Valid: true}
		state.LastRequestID = lastRequestID
	} else if err != sql.ErrNoRows {
		return err
	}

	if err := upsertDashboardAggregateStateTx(tx, state); err != nil {
		return err
	}

	return tx.Commit()
}

func rebuildDashboardProjectionFactsTx(tx *sql.Tx, location *time.Location, now time.Time) error {
	modelExpr := dashboardEffectiveModelSQLExpr()
	query := fmt.Sprintf(`
		INSERT INTO global_request_metric_projections (
			request_id, user_id, minute_bucket, completed_minute_bucket, day_bucket, effective_model, provider_group,
			request_count, input_tokens, output_tokens, total_tokens, cost_micros, error_count,
			cache_read_input_tokens, cache_creation_input_tokens, latency_ms, ttfb_ms, updated_at
		)
		SELECT
			id,
			user_id,
			%s,
			%s,
			%s,
			%s,
			%s,
			1,
			COALESCE(input_tokens, 0),
			COALESCE(output_tokens, 0),
			COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0),
			COALESCE(cost_micros, 0),
			CASE WHEN status_code >= 400 THEN 1 ELSE 0 END,
			COALESCE(cache_read_input_tokens, 0),
			COALESCE(cache_creation_input_tokens, 0),
			COALESCE(latency_ms, 0),
			COALESCE(ttfb_ms, 0),
			?
		FROM request_logs
		WHERE status <> 'pending'
	`,
		minuteBucketSQLExpr("created_at"),
		completedMinuteBucketSQLExpr("created_at", "latency_ms"),
		database.DayBucketExprInLocation("created_at", location),
		modelExpr,
		dashboardProviderGroupSQLExpr(modelExpr),
	)
	_, err := tx.Exec(query, now.UTC())
	return err
}

func rebuildDashboardMinuteMetricsTx(tx *sql.Tx, now time.Time) error {
	queries := []string{
		`
			INSERT INTO dashboard_minute_metrics (
				scope_type, scope_id, minute_bucket, request_count_sum, input_tokens_sum, output_tokens_sum, total_tokens_sum,
				cost_micros_sum, error_count_sum, cache_read_input_tokens_sum, cache_creation_input_tokens_sum,
				latency_sum_ms, latency_sample_count, ttfb_sum_ms, ttfb_sample_count, updated_at
			)
			SELECT
				'global',
				'global',
				minute_bucket,
				COALESCE(SUM(request_count), 0),
				COALESCE(SUM(input_tokens), 0),
				COALESCE(SUM(output_tokens), 0),
				COALESCE(SUM(total_tokens), 0),
				COALESCE(SUM(cost_micros), 0),
				COALESCE(SUM(error_count), 0),
				COALESCE(SUM(cache_read_input_tokens), 0),
				COALESCE(SUM(cache_creation_input_tokens), 0),
				COALESCE(SUM(CASE WHEN latency_ms > 0 THEN latency_ms ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN latency_ms > 0 THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN ttfb_ms > 0 THEN ttfb_ms ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN ttfb_ms > 0 THEN 1 ELSE 0 END), 0),
				?
			FROM global_request_metric_projections
			GROUP BY minute_bucket
		`,
		`
			INSERT INTO dashboard_minute_metrics (
				scope_type, scope_id, minute_bucket, request_count_sum, input_tokens_sum, output_tokens_sum, total_tokens_sum,
				cost_micros_sum, error_count_sum, cache_read_input_tokens_sum, cache_creation_input_tokens_sum,
				latency_sum_ms, latency_sample_count, ttfb_sum_ms, ttfb_sample_count, updated_at
			)
			SELECT
				'user',
				user_id,
				minute_bucket,
				COALESCE(SUM(request_count), 0),
				COALESCE(SUM(input_tokens), 0),
				COALESCE(SUM(output_tokens), 0),
				COALESCE(SUM(total_tokens), 0),
				COALESCE(SUM(cost_micros), 0),
				COALESCE(SUM(error_count), 0),
				COALESCE(SUM(cache_read_input_tokens), 0),
				COALESCE(SUM(cache_creation_input_tokens), 0),
				COALESCE(SUM(CASE WHEN latency_ms > 0 THEN latency_ms ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN latency_ms > 0 THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN ttfb_ms > 0 THEN ttfb_ms ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN ttfb_ms > 0 THEN 1 ELSE 0 END), 0),
				?
			FROM global_request_metric_projections
			GROUP BY user_id, minute_bucket
		`,
		fmt.Sprintf(`
			INSERT INTO dashboard_minute_metrics (
				scope_type, scope_id, minute_bucket, concurrency_delta_sum, updated_at
			)
			SELECT
				'global',
				'global',
				minute_bucket,
				COALESCE(SUM(delta), 0),
				?
			FROM (
				SELECT minute_bucket, 1 AS delta
				FROM global_request_metric_projections
				WHERE latency_ms > 0
				UNION ALL
				SELECT %s AS minute_bucket, -1 AS delta
				FROM global_request_metric_projections
				WHERE latency_ms > 0
			) deltas
			GROUP BY minute_bucket
			ON CONFLICT(scope_type, scope_id, minute_bucket) DO UPDATE SET
				concurrency_delta_sum = dashboard_minute_metrics.concurrency_delta_sum + excluded.concurrency_delta_sum,
				updated_at = excluded.updated_at
		`, minutePlusOneSQLExpr("completed_minute_bucket")),
		fmt.Sprintf(`
			INSERT INTO dashboard_minute_metrics (
				scope_type, scope_id, minute_bucket, concurrency_delta_sum, updated_at
			)
			SELECT
				'user',
				user_id,
				minute_bucket,
				COALESCE(SUM(delta), 0),
				?
			FROM (
				SELECT user_id, minute_bucket, 1 AS delta
				FROM global_request_metric_projections
				WHERE latency_ms > 0
				UNION ALL
				SELECT user_id, %s AS minute_bucket, -1 AS delta
				FROM global_request_metric_projections
				WHERE latency_ms > 0
			) deltas
			GROUP BY user_id, minute_bucket
			ON CONFLICT(scope_type, scope_id, minute_bucket) DO UPDATE SET
				concurrency_delta_sum = dashboard_minute_metrics.concurrency_delta_sum + excluded.concurrency_delta_sum,
				updated_at = excluded.updated_at
		`, minutePlusOneSQLExpr("completed_minute_bucket")),
	}
	for _, query := range queries {
		if _, err := tx.Exec(query, now.UTC()); err != nil {
			return err
		}
	}
	return nil
}

func rebuildDashboardDayMetricsTx(tx *sql.Tx, now time.Time) error {
	queries := []string{
		`
			INSERT INTO dashboard_day_metrics (
				scope_type, scope_id, day_bucket, request_count_sum, cost_micros_sum, updated_at
			)
			SELECT 'global', 'global', day_bucket, COALESCE(SUM(request_count), 0), COALESCE(SUM(cost_micros), 0), ?
			FROM global_request_metric_projections
			GROUP BY day_bucket
		`,
		`
			INSERT INTO dashboard_day_metrics (
				scope_type, scope_id, day_bucket, request_count_sum, cost_micros_sum, updated_at
			)
			SELECT 'user', user_id, day_bucket, COALESCE(SUM(request_count), 0), COALESCE(SUM(cost_micros), 0), ?
			FROM global_request_metric_projections
			GROUP BY user_id, day_bucket
		`,
	}
	for _, query := range queries {
		if _, err := tx.Exec(query, now.UTC()); err != nil {
			return err
		}
	}
	return nil
}

func rebuildDashboardModelDayMetricsTx(tx *sql.Tx, now time.Time) error {
	queries := []string{
		`
			INSERT INTO dashboard_model_day_metrics (
				scope_type, scope_id, day_bucket, model, request_count_sum, cost_micros_sum, updated_at
			)
			SELECT 'global', 'global', day_bucket, effective_model, COALESCE(SUM(request_count), 0), COALESCE(SUM(cost_micros), 0), ?
			FROM global_request_metric_projections
			GROUP BY day_bucket, effective_model
		`,
		`
			INSERT INTO dashboard_model_day_metrics (
				scope_type, scope_id, day_bucket, model, request_count_sum, cost_micros_sum, updated_at
			)
			SELECT 'user', user_id, day_bucket, effective_model, COALESCE(SUM(request_count), 0), COALESCE(SUM(cost_micros), 0), ?
			FROM global_request_metric_projections
			GROUP BY user_id, day_bucket, effective_model
		`,
	}
	for _, query := range queries {
		if _, err := tx.Exec(query, now.UTC()); err != nil {
			return err
		}
	}
	return nil
}

func rebuildDashboardProviderDayMetricsTx(tx *sql.Tx, now time.Time) error {
	queries := []string{
		`
			INSERT INTO dashboard_provider_day_metrics (
				scope_type, scope_id, day_bucket, provider, request_count_sum, total_input_tokens_sum,
				cache_read_input_tokens_sum, cache_creation_input_tokens_sum, updated_at
			)
			SELECT
				'global',
				'global',
				day_bucket,
				provider_group,
				COALESCE(SUM(request_count), 0),
				COALESCE(SUM(input_tokens), 0),
				COALESCE(SUM(cache_read_input_tokens), 0),
				COALESCE(SUM(cache_creation_input_tokens), 0),
				?
			FROM global_request_metric_projections
			GROUP BY day_bucket, provider_group
		`,
		`
			INSERT INTO dashboard_provider_day_metrics (
				scope_type, scope_id, day_bucket, provider, request_count_sum, total_input_tokens_sum,
				cache_read_input_tokens_sum, cache_creation_input_tokens_sum, updated_at
			)
			SELECT
				'user',
				user_id,
				day_bucket,
				provider_group,
				COALESCE(SUM(request_count), 0),
				COALESCE(SUM(input_tokens), 0),
				COALESCE(SUM(cache_read_input_tokens), 0),
				COALESCE(SUM(cache_creation_input_tokens), 0),
				?
			FROM global_request_metric_projections
			GROUP BY user_id, day_bucket, provider_group
		`,
	}
	for _, query := range queries {
		if _, err := tx.Exec(query, now.UTC()); err != nil {
			return err
		}
	}
	return nil
}

func rebuildDashboardTimingHistogramsTx(tx *sql.Tx, now time.Time) error {
	rows, err := tx.Query(`
		SELECT minute_bucket, latency_ms, ttfb_ms
		FROM global_request_metric_projections
		WHERE latency_ms > 0 OR ttfb_ms > 0
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type histogramKey struct {
		minute     time.Time
		metricName string
		bucket     int64
	}
	counts := make(map[histogramKey]int64)
	for rows.Next() {
		var minute time.Time
		var latencyMs int64
		var ttfbMs int64
		if err := rows.Scan(&minute, &latencyMs, &ttfbMs); err != nil {
			return err
		}
		minute = minute.UTC()
		if latencyMs > 0 {
			key := histogramKey{
				minute:     minute,
				metricName: dashboardTimingMetricDuration,
				bucket:     dashboardTimingHistogramBucketUpper(latencyMs),
			}
			counts[key]++
		}
		if ttfbMs > 0 {
			key := histogramKey{
				minute:     minute,
				metricName: dashboardTimingMetricTTFB,
				bucket:     dashboardTimingHistogramBucketUpper(ttfbMs),
			}
			counts[key]++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for key, count := range counts {
		if _, err := tx.Exec(`
			INSERT INTO dashboard_timing_histograms (
				scope_type, scope_id, minute_bucket, metric_name, bucket_upper_ms, sample_count, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`,
			dashboardScopeGlobal,
			dashboardScopeGlobal,
			key.minute,
			key.metricName,
			key.bucket,
			count,
			now.UTC(),
		); err != nil {
			return err
		}
	}
	return nil
}

func dashboardScopePair(userID *string) (string, string) {
	if userID == nil {
		return dashboardScopeGlobal, dashboardScopeGlobal
	}
	return "user", *userID
}

func loadDashboardPeriodStatsFromAggregates(scopeType, scopeID string, from time.Time) (DashboardPeriodStats, error) {
	var row dashboardPeriodStats
	query := `
		SELECT
			COALESCE(SUM(request_count_sum), 0),
			COALESCE(SUM(input_tokens_sum), 0),
			COALESCE(SUM(cache_read_input_tokens_sum), 0),
			COALESCE(SUM(output_tokens_sum), 0),
			COALESCE(SUM(cost_micros_sum), 0),
			COALESCE(SUM(error_count_sum), 0)
		FROM dashboard_minute_metrics
		WHERE scope_type = ? AND scope_id = ? AND minute_bucket >= ?
	`
	args := []any{scopeType, scopeID, from.UTC()}
	if !database.IsPostgres() {
		query = `
			SELECT
				COALESCE(SUM(request_count_sum), 0),
				COALESCE(SUM(input_tokens_sum), 0),
				COALESCE(SUM(cache_read_input_tokens_sum), 0),
				COALESCE(SUM(output_tokens_sum), 0),
				COALESCE(SUM(cost_micros_sum), 0),
				COALESCE(SUM(error_count_sum), 0)
			FROM dashboard_minute_metrics
			WHERE scope_type = ? AND scope_id = ? AND SUBSTR(minute_bucket, 1, 19) >= ?
		`
		args[2] = from.UTC().Format("2006-01-02 15:04:05")
	}
	err := database.GetDB().QueryRow(query, args...).Scan(
		&row.RequestCount,
		&row.InputTokensSum,
		&row.CacheReadInputTokens,
		&row.OutputTokensSum,
		&row.CostMicrosSum,
		&row.ErrorCount,
	)
	if err != nil {
		return DashboardPeriodStats{}, err
	}
	return DashboardPeriodStats{
		RequestCount:   row.RequestCount,
		InputTokensSum: displayInputTokensInt64(row.InputTokensSum, row.CacheReadInputTokens),
		OutputTokensSum: row.OutputTokensSum,
		CostMicrosSum:  row.CostMicrosSum,
		ErrorCount:     row.ErrorCount,
	}, nil
}

type dashboardPeriodStats struct {
	RequestCount        int64
	InputTokensSum      int64
	CacheReadInputTokens int64
	OutputTokensSum     int64
	CostMicrosSum       int64
	ErrorCount          int64
}

func loadDashboardTopModelsFromAggregates(scopeType, scopeID, dayFrom string, limit int) ([]DashboardTopModel, error) {
	rows, err := database.GetDB().Query(`
		SELECT model, COALESCE(SUM(request_count_sum), 0) AS cnt, COALESCE(SUM(cost_micros_sum), 0) AS cost
		FROM dashboard_model_day_metrics
		WHERE scope_type = ? AND scope_id = ? AND day_bucket >= ?
		GROUP BY model
		ORDER BY cnt DESC
		LIMIT ?
	`, scopeType, scopeID, dayFrom, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]DashboardTopModel, 0, limit)
	for rows.Next() {
		var row dashboardModelAggregateRow
		if err := rows.Scan(&row.Model, &row.RequestCount, &row.CostMicros); err != nil {
			return nil, err
		}
		result = append(result, DashboardTopModel{
			Model:        row.Model,
			RequestCount: row.RequestCount,
			CostMicros:   row.CostMicros,
		})
	}
	return result, rows.Err()
}

func loadDashboardDailyTrendFromAggregates(scopeType, scopeID, dayFrom string) ([]DashboardDailyTrend, error) {
	rows, err := database.GetDB().Query(`
		SELECT day_bucket, COALESCE(SUM(cost_micros_sum), 0) AS cost, COALESCE(SUM(request_count_sum), 0) AS cnt
		FROM dashboard_day_metrics
		WHERE scope_type = ? AND scope_id = ? AND day_bucket >= ?
		GROUP BY day_bucket
		ORDER BY day_bucket ASC
	`, scopeType, scopeID, dayFrom)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]DashboardDailyTrend, 0, 14)
	for rows.Next() {
		var row dashboardDayAggregateRow
		if err := rows.Scan(&row.DayBucket, &row.CostMicros, &row.RequestCount); err != nil {
			return nil, err
		}
		result = append(result, DashboardDailyTrend{
			Date:       row.DayBucket,
			CostMicros: row.CostMicros,
			Requests:   row.RequestCount,
		})
	}
	return result, rows.Err()
}

func loadDashboardCacheHitRatesFromAggregates(scopeType, scopeID, dayFrom string) ([]DashboardCacheHitRate, error) {
	rows, err := database.GetDB().Query(`
		SELECT provider, COALESCE(SUM(total_input_tokens_sum), 0), COALESCE(SUM(cache_read_input_tokens_sum), 0),
		       COALESCE(SUM(cache_creation_input_tokens_sum), 0), COALESCE(SUM(request_count_sum), 0)
		FROM dashboard_provider_day_metrics
		WHERE scope_type = ? AND scope_id = ? AND day_bucket >= ? AND provider IN ('Claude', 'OpenAI', 'Gemini')
		GROUP BY provider
		ORDER BY COALESCE(SUM(request_count_sum), 0) DESC
	`, scopeType, scopeID, dayFrom)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DashboardCacheHitRate
	for rows.Next() {
		var row dashboardProviderAggregateRow
		if err := rows.Scan(&row.Provider, &row.TotalInputTokens, &row.CacheReadTokens, &row.CacheCreationTokens, &row.RequestCount); err != nil {
			return nil, err
		}
		item := DashboardCacheHitRate{
			Provider:            row.Provider,
			TotalInputTokens:    row.TotalInputTokens,
			CacheReadTokens:     row.CacheReadTokens,
			CacheCreationTokens: row.CacheCreationTokens,
			RequestCount:        row.RequestCount,
		}
		if item.TotalInputTokens > 0 {
			item.HitRate = float64(item.CacheReadTokens) / float64(item.TotalInputTokens) * 100
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func loadDashboardTimingHistogramRows(scopeType, scopeID, metricName string, start, end time.Time) (map[time.Time]map[int64]int64, error) {
	query := `
		SELECT minute_bucket, bucket_upper_ms, sample_count
		FROM dashboard_timing_histograms
		WHERE scope_type = ? AND scope_id = ? AND metric_name = ? AND minute_bucket >= ? AND minute_bucket <= ?
		ORDER BY minute_bucket ASC, bucket_upper_ms ASC
	`
	args := []any{scopeType, scopeID, metricName, start.UTC(), end.UTC()}
	if !database.IsPostgres() {
		query = `
			SELECT minute_bucket, bucket_upper_ms, sample_count
			FROM dashboard_timing_histograms
			WHERE scope_type = ? AND scope_id = ? AND metric_name = ?
			  AND SUBSTR(minute_bucket, 1, 19) >= ? AND SUBSTR(minute_bucket, 1, 19) <= ?
			ORDER BY minute_bucket ASC, bucket_upper_ms ASC
		`
		args[3] = start.UTC().Format("2006-01-02 15:04:05")
		args[4] = end.UTC().Format("2006-01-02 15:04:05")
	}

	rows, err := database.GetDB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[time.Time]map[int64]int64)
	for rows.Next() {
		var minute time.Time
		var upper int64
		var count int64
		if err := rows.Scan(&minute, &upper, &count); err != nil {
			return nil, err
		}
		minute = minute.UTC()
		buckets := result[minute]
		if buckets == nil {
			buckets = make(map[int64]int64)
			result[minute] = buckets
		}
		buckets[upper] += count
	}
	return result, rows.Err()
}

func loadDashboardMinuteMetrics(scopeType, scopeID string, start, end time.Time) ([]dashboardMinuteMetricRow, error) {
	query := `
		SELECT minute_bucket, request_count_sum, input_tokens_sum, output_tokens_sum, total_tokens_sum,
		       concurrency_delta_sum, latency_sum_ms, latency_sample_count, ttfb_sum_ms, ttfb_sample_count
		FROM dashboard_minute_metrics
		WHERE scope_type = ? AND scope_id = ? AND minute_bucket >= ? AND minute_bucket <= ?
		ORDER BY minute_bucket ASC
	`
	args := []any{scopeType, scopeID, start.UTC(), end.UTC()}
	if !database.IsPostgres() {
		query = `
			SELECT minute_bucket, request_count_sum, input_tokens_sum, output_tokens_sum, total_tokens_sum,
			       concurrency_delta_sum, latency_sum_ms, latency_sample_count, ttfb_sum_ms, ttfb_sample_count
			FROM dashboard_minute_metrics
			WHERE scope_type = ? AND scope_id = ? AND SUBSTR(minute_bucket, 1, 19) >= ? AND SUBSTR(minute_bucket, 1, 19) <= ?
			ORDER BY minute_bucket ASC
		`
		args[2] = start.UTC().Format("2006-01-02 15:04:05")
		args[3] = end.UTC().Format("2006-01-02 15:04:05")
	}
	rows, err := database.GetDB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []dashboardMinuteMetricRow
	for rows.Next() {
		var row dashboardMinuteMetricRow
		if err := rows.Scan(
			&row.MinuteBucket,
			&row.RequestCountSum,
			&row.InputTokensSum,
			&row.OutputTokensSum,
			&row.TotalTokensSum,
			&row.ConcurrencyDeltaSum,
			&row.LatencySumMs,
			&row.LatencySampleCount,
			&row.TTFBSumMs,
			&row.TTFBSampleCount,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func loadConcurrencyByMinuteFromAggregates(scopeType, scopeID string, start, end time.Time) (map[string]int64, error) {
	var baseline int64
	baselineQuery := `
		SELECT COALESCE(SUM(concurrency_delta_sum), 0)
		FROM dashboard_minute_metrics
		WHERE scope_type = ? AND scope_id = ? AND minute_bucket < ?
	`
	baselineArgs := []any{scopeType, scopeID, start.UTC()}
	if !database.IsPostgres() {
		baselineQuery = `
			SELECT COALESCE(SUM(concurrency_delta_sum), 0)
			FROM dashboard_minute_metrics
			WHERE scope_type = ? AND scope_id = ? AND SUBSTR(minute_bucket, 1, 19) < ?
		`
		baselineArgs[2] = start.UTC().Format("2006-01-02 15:04:05")
	}
	if err := database.GetDB().QueryRow(baselineQuery, baselineArgs...).Scan(&baseline); err != nil {
		return nil, err
	}

	rangeQuery := `
		SELECT minute_bucket, concurrency_delta_sum
		FROM dashboard_minute_metrics
		WHERE scope_type = ? AND scope_id = ? AND minute_bucket >= ? AND minute_bucket <= ?
		ORDER BY minute_bucket ASC
	`
	rangeArgs := []any{scopeType, scopeID, start.UTC(), end.UTC()}
	if !database.IsPostgres() {
		rangeQuery = `
			SELECT minute_bucket, concurrency_delta_sum
			FROM dashboard_minute_metrics
			WHERE scope_type = ? AND scope_id = ? AND SUBSTR(minute_bucket, 1, 19) >= ? AND SUBSTR(minute_bucket, 1, 19) <= ?
			ORDER BY minute_bucket ASC
		`
		rangeArgs[2] = start.UTC().Format("2006-01-02 15:04:05")
		rangeArgs[3] = end.UTC().Format("2006-01-02 15:04:05")
	}
	rows, err := database.GetDB().Query(rangeQuery, rangeArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deltas := make(map[string]int64)
	for rows.Next() {
		var minute time.Time
		var delta int64
		if err := rows.Scan(&minute, &delta); err != nil {
			return nil, err
		}
		deltas[minute.UTC().Format(time.RFC3339)] = delta
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make(map[string]int64, int(end.Sub(start)/time.Minute)+1)
	current := baseline
	for minute := start.UTC(); !minute.After(end.UTC()); minute = minute.Add(time.Minute) {
		current += deltas[minute.Format(time.RFC3339)]
		result[minute.Format(time.RFC3339)] = current
	}
	return result, nil
}

func (r *RequestLogRepository) getAdminThroughputTrendFromAggregates(windowKey string) ([]DashboardThroughputPoint, error) {
	start, end := adminThroughputWindowBounds(time.Now().UTC(), windowKey)
	rows, err := loadDashboardMinuteMetrics(dashboardScopeGlobal, dashboardScopeGlobal, start, end)
	if err != nil {
		return nil, err
	}
	concurrencyByMinute, err := loadConcurrencyByMinuteFromAggregates(dashboardScopeGlobal, dashboardScopeGlobal, start, end)
	if err != nil {
		return nil, err
	}
	return buildDashboardThroughputPoints(fillDashboardMinuteMetrics(start, end, rows), concurrencyByMinute), nil
}

func (r *RequestLogRepository) getAdminTimingTrendFromAggregates(windowKey string) (ttfbTrend []DashboardTimingPoint, durationTrend []DashboardTimingPoint, err error) {
	startMinute, endMinute := adminThroughputWindowBounds(time.Now().UTC(), windowKey)
	rows, err := loadDashboardMinuteMetrics(dashboardScopeGlobal, dashboardScopeGlobal, startMinute, endMinute)
	if err != nil {
		return nil, nil, err
	}
	ttfbHistogram, err := loadDashboardTimingHistogramRows(dashboardScopeGlobal, dashboardScopeGlobal, dashboardTimingMetricTTFB, startMinute, endMinute)
	if err != nil {
		return nil, nil, err
	}
	durationHistogram, err := loadDashboardTimingHistogramRows(dashboardScopeGlobal, dashboardScopeGlobal, dashboardTimingMetricDuration, startMinute, endMinute)
	if err != nil {
		return nil, nil, err
	}
	return buildDashboardTimingPointsFromAggregates(startMinute, endMinute, rows, ttfbHistogram, dashboardTimingMetricTTFB),
		buildDashboardTimingPointsFromAggregates(startMinute, endMinute, rows, durationHistogram, dashboardTimingMetricDuration),
		nil
}

func (r *RequestLogRepository) GetAdminDashboardSummary(location *time.Location) (today, week, month DashboardPeriodStats, topModels []DashboardTopModel, dailyTrend []DashboardDailyTrend, err error) {
	location = normalizeDashboardLocation(location)
	if err = r.EnsureDashboardAggregatesReady(location); err != nil {
		return
	}

	_, todayStartUTC, weekStartUTC, monthStartUTC, trendStartLocal, _ := dashboardWindowStarts(time.Now().UTC(), location)
	monthStartDay := dashboardDayBucket(monthStartUTC, location)
	trendStartDay := trendStartLocal.In(location).Format("2006-01-02")

	today, err = loadDashboardPeriodStatsFromAggregates(dashboardScopeGlobal, dashboardScopeGlobal, todayStartUTC)
	if err != nil {
		return
	}
	week, err = loadDashboardPeriodStatsFromAggregates(dashboardScopeGlobal, dashboardScopeGlobal, weekStartUTC)
	if err != nil {
		return
	}
	month, err = loadDashboardPeriodStatsFromAggregates(dashboardScopeGlobal, dashboardScopeGlobal, monthStartUTC)
	if err != nil {
		return
	}
	topModels, err = loadDashboardTopModelsFromAggregates(dashboardScopeGlobal, dashboardScopeGlobal, monthStartDay, 10)
	if err != nil {
		return
	}
	dailyTrend, err = loadDashboardDailyTrendFromAggregates(dashboardScopeGlobal, dashboardScopeGlobal, trendStartDay)
	if err != nil {
		return
	}
	dailyTrend = fillDashboardDailyTrendInLocation(trendStartLocal, 14, location, dailyTrend)
	return
}

func (r *RequestLogRepository) GetAdminDashboardTrends(windowKey string, location *time.Location) (throughputTrend []DashboardThroughputPoint, ttfbTrend []DashboardTimingPoint, durationTrend []DashboardTimingPoint, err error) {
	location = normalizeDashboardLocation(location)
	if err = r.EnsureDashboardAggregatesReady(location); err != nil {
		return
	}
	throughputTrend, err = r.getAdminThroughputTrendFromAggregates(windowKey)
	if err != nil {
		return
	}
	ttfbTrend, durationTrend, err = r.getAdminTimingTrendFromAggregates(windowKey)
	return
}
