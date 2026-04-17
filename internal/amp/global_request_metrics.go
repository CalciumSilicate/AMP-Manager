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

func syncGlobalRequestMetricsTx(tx *sql.Tx, snapshot RequestTrace, now time.Time) error {
	projection := buildRequestMetricProjection(snapshot)
	if projection == nil {
		return nil
	}

	return upsertRequestMetricProjectionTx(tx, projection, now)
}
