package amp

import (
	"database/sql"
	"time"
)

type requestMetricProjection struct {
	RequestID    string
	MinuteBucket time.Time
	RequestCount int64
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	LatencyMs    int64
	TTFBMs       int64
}

func buildRequestMetricProjection(snapshot RequestTrace) *requestMetricProjection {
	if snapshot.RequestID == "" || snapshot.StartTime.IsZero() {
		return nil
	}

	inputTokens := int64(0)
	if snapshot.InputTokens != nil && *snapshot.InputTokens > 0 {
		inputTokens = int64(*snapshot.InputTokens)
	}

	outputTokens := int64(0)
	if snapshot.OutputTokens != nil && *snapshot.OutputTokens > 0 {
		outputTokens = int64(*snapshot.OutputTokens)
	}

	return &requestMetricProjection{
		RequestID:    snapshot.RequestID,
		MinuteBucket: snapshot.StartTime.UTC().Truncate(time.Minute),
		RequestCount: 1,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		LatencyMs:    snapshot.LatencyMs,
		TTFBMs:       valueOrZeroInt64(snapshot.TTFBMs),
	}
}

func valueOrZeroInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func queryRequestMetricProjectionTx(tx *sql.Tx, requestID string) (*requestMetricProjection, error) {
	if requestID == "" {
		return nil, nil
	}

	var projection requestMetricProjection
	err := tx.QueryRow(`
		SELECT request_id, minute_bucket, request_count, input_tokens, output_tokens, total_tokens, latency_ms, ttfb_ms
		FROM global_request_metric_projections
		WHERE request_id = ?
	`, requestID).Scan(
		&projection.RequestID,
		&projection.MinuteBucket,
		&projection.RequestCount,
		&projection.InputTokens,
		&projection.OutputTokens,
		&projection.TotalTokens,
		&projection.LatencyMs,
		&projection.TTFBMs,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &projection, nil
}

func upsertRequestMetricProjectionTx(tx *sql.Tx, projection *requestMetricProjection, now time.Time) error {
	if projection == nil {
		return nil
	}

	_, err := tx.Exec(`
		INSERT INTO global_request_metric_projections (
			request_id, minute_bucket, request_count, input_tokens, output_tokens, total_tokens, latency_ms, ttfb_ms, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(request_id) DO UPDATE SET
			minute_bucket = excluded.minute_bucket,
			request_count = excluded.request_count,
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			total_tokens = excluded.total_tokens,
			latency_ms = excluded.latency_ms,
			ttfb_ms = excluded.ttfb_ms,
			updated_at = excluded.updated_at
	`,
		projection.RequestID,
		projection.MinuteBucket.UTC(),
		projection.RequestCount,
		projection.InputTokens,
		projection.OutputTokens,
		projection.TotalTokens,
		projection.LatencyMs,
		projection.TTFBMs,
		now.UTC(),
	)
	return err
}

func applyMinuteMetricDeltaTx(tx *sql.Tx, minuteBucket time.Time, requestDelta, inputDelta, outputDelta, totalDelta int64, now time.Time) error {
	if minuteBucket.IsZero() {
		return nil
	}
	if requestDelta == 0 && inputDelta == 0 && outputDelta == 0 && totalDelta == 0 {
		return nil
	}

	_, err := tx.Exec(`
		INSERT INTO global_request_minute_metrics (
			minute_bucket, request_count_sum, input_tokens_sum, output_tokens_sum, total_tokens_sum, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(minute_bucket) DO UPDATE SET
			request_count_sum = global_request_minute_metrics.request_count_sum + excluded.request_count_sum,
			input_tokens_sum = global_request_minute_metrics.input_tokens_sum + excluded.input_tokens_sum,
			output_tokens_sum = global_request_minute_metrics.output_tokens_sum + excluded.output_tokens_sum,
			total_tokens_sum = global_request_minute_metrics.total_tokens_sum + excluded.total_tokens_sum,
			updated_at = excluded.updated_at
	`,
		minuteBucket.UTC(),
		requestDelta,
		inputDelta,
		outputDelta,
		totalDelta,
		now.UTC(),
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		DELETE FROM global_request_minute_metrics
		WHERE minute_bucket = ?
		  AND request_count_sum <= 0
		  AND input_tokens_sum <= 0
		  AND output_tokens_sum <= 0
		  AND total_tokens_sum <= 0
	`, minuteBucket.UTC())
	return err
}

func syncGlobalRequestMetricsTx(tx *sql.Tx, snapshot RequestTrace, now time.Time) error {
	newProjection := buildRequestMetricProjection(snapshot)
	if newProjection == nil {
		return nil
	}

	existingProjection, err := queryRequestMetricProjectionTx(tx, snapshot.RequestID)
	if err != nil {
		return err
	}

	if existingProjection != nil {
		if err := applyMinuteMetricDeltaTx(
			tx,
			existingProjection.MinuteBucket,
			-existingProjection.RequestCount,
			-existingProjection.InputTokens,
			-existingProjection.OutputTokens,
			-existingProjection.TotalTokens,
			now,
		); err != nil {
			return err
		}
	}

	if err := applyMinuteMetricDeltaTx(
		tx,
		newProjection.MinuteBucket,
		newProjection.RequestCount,
		newProjection.InputTokens,
		newProjection.OutputTokens,
		newProjection.TotalTokens,
		now,
	); err != nil {
		return err
	}

	return upsertRequestMetricProjectionTx(tx, newProjection, now)
}
