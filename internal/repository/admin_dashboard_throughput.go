package repository

import (
	"math"
	"sort"
	"time"

	"ampmanager/internal/database"
)

const (
	adminThroughputWindowMinutes = 24 * 60
	throughputRollingWindowSize  = 5
)

var adminThroughputWindowKeyMinutes = map[string]int{
	"1h":  60,
	"3h":  180,
	"6h":  360,
	"12h": 720,
	"24h": 24 * 60,
	"3d":  3 * 24 * 60,
}

type DashboardThroughputPoint struct {
	Minute        string
	QPS1m         float64
	RPM5m         float64
	TPM5m         float64
	Concurrency1m int64
}

type DashboardTimingPoint struct {
	Bucket    string
	AvgMs     float64
	P50Ms     float64
	P90Ms     float64
	P99Ms     float64
	SampleCnt int64
}

type dashboardMinuteMetricRow struct {
	MinuteBucket    time.Time
	RequestCountSum int64
	InputTokensSum  int64
	OutputTokensSum int64
	TotalTokensSum  int64
	ConcurrencyDeltaSum int64
	LatencySumMs        int64
	LatencySampleCount  int64
	TTFBSumMs           int64
	TTFBSampleCount     int64
}

type dashboardTimingProjectionRow struct {
	MinuteBucket time.Time
	LatencyMs    int64
	TTFBMs       int64
}

func adminThroughputWindowBounds(now time.Time, windowKey string) (time.Time, time.Time) {
	windowMinutes, ok := adminThroughputWindowKeyMinutes[windowKey]
	if !ok || windowMinutes <= 0 {
		windowMinutes = adminThroughputWindowMinutes
	}
	end := now.UTC().Truncate(time.Minute)
	start := end.Add(-time.Duration(windowMinutes-1) * time.Minute)
	return start, end
}

func fillDashboardMinuteMetrics(start, end time.Time, rows []dashboardMinuteMetricRow) []dashboardMinuteMetricRow {
	if end.Before(start) {
		return nil
	}

	byMinute := make(map[string]dashboardMinuteMetricRow, len(rows))
	for _, row := range rows {
		key := row.MinuteBucket.UTC().Format(time.RFC3339)
		byMinute[key] = row
	}

	totalMinutes := int(end.Sub(start)/time.Minute) + 1
	filled := make([]dashboardMinuteMetricRow, 0, totalMinutes)
	for minute := start.UTC(); !minute.After(end.UTC()); minute = minute.Add(time.Minute) {
		key := minute.Format(time.RFC3339)
		if row, ok := byMinute[key]; ok {
			filled = append(filled, row)
			continue
		}
		filled = append(filled, dashboardMinuteMetricRow{MinuteBucket: minute})
	}
	return filled
}

func buildDashboardThroughputPoints(rows []dashboardMinuteMetricRow, concurrencyByMinute map[string]int64) []DashboardThroughputPoint {
	points := make([]DashboardThroughputPoint, 0, len(rows))

	var requestWindowSum int64
	var tokenWindowSum int64
	for index, row := range rows {
		requestWindowSum += row.RequestCountSum
		tokenWindowSum += row.TotalTokensSum
		if index >= throughputRollingWindowSize {
			requestWindowSum -= rows[index-throughputRollingWindowSize].RequestCountSum
			tokenWindowSum -= rows[index-throughputRollingWindowSize].TotalTokensSum
		}

		points = append(points, DashboardThroughputPoint{
			Minute:        row.MinuteBucket.UTC().Format(time.RFC3339),
			QPS1m:         float64(row.RequestCountSum) / 60.0,
			RPM5m:         float64(requestWindowSum) / float64(throughputRollingWindowSize),
			TPM5m:         float64(tokenWindowSum) / float64(throughputRollingWindowSize),
			Concurrency1m: concurrencyByMinute[row.MinuteBucket.UTC().Format(time.RFC3339)],
		})
	}

	return points
}

func timingBucketStart(minute time.Time) time.Time {
	return minute.UTC().Truncate(5 * time.Minute)
}

func percentileFloat64(samples []float64, percentile float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	index := int(math.Ceil(float64(len(sorted))*percentile)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func buildDashboardTimingPoints(start, end time.Time, valuesByBucket map[time.Time][]float64) []DashboardTimingPoint {
	points := make([]DashboardTimingPoint, 0, int(end.Sub(start)/(5*time.Minute))+1)
	for bucket := start.UTC(); !bucket.After(end.UTC()); bucket = bucket.Add(5 * time.Minute) {
		samples := valuesByBucket[bucket]
		if len(samples) == 0 {
			points = append(points, DashboardTimingPoint{Bucket: bucket.Format(time.RFC3339)})
			continue
		}
		sum := 0.0
		for _, value := range samples {
			sum += value
		}
		points = append(points, DashboardTimingPoint{
			Bucket:    bucket.Format(time.RFC3339),
			AvgMs:     sum / float64(len(samples)),
			P50Ms:     percentileFloat64(samples, 0.50),
			P90Ms:     percentileFloat64(samples, 0.90),
			P99Ms:     percentileFloat64(samples, 0.99),
			SampleCnt: int64(len(samples)),
		})
	}
	return points
}

func (r *RequestLogRepository) GetAdminThroughputTrend(windowKey string) ([]DashboardThroughputPoint, error) {
	start, end := adminThroughputWindowBounds(time.Now().UTC(), windowKey)
	if err := r.ensureAdminProjectionWindow(start, end); err != nil {
		return nil, err
	}

	rows, err := database.GetDB().Query(`
		SELECT minute_bucket,
		       COALESCE(SUM(request_count), 0) AS request_count_sum,
		       COALESCE(SUM(input_tokens), 0) AS input_tokens_sum,
		       COALESCE(SUM(output_tokens), 0) AS output_tokens_sum,
		       COALESCE(SUM(total_tokens), 0) AS total_tokens_sum
		FROM global_request_metric_projections
		WHERE minute_bucket >= ? AND minute_bucket <= ?
		GROUP BY minute_bucket
		ORDER BY minute_bucket ASC
	`, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metricRows []dashboardMinuteMetricRow
	for rows.Next() {
		var row dashboardMinuteMetricRow
		if err := rows.Scan(
			&row.MinuteBucket,
			&row.RequestCountSum,
			&row.InputTokensSum,
			&row.OutputTokensSum,
			&row.TotalTokensSum,
		); err != nil {
			return nil, err
		}
		metricRows = append(metricRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	filled := fillDashboardMinuteMetrics(start, end, metricRows)
	concurrencyByMinute, err := r.getAdminConcurrencyByMinute(start, end)
	if err != nil {
		return nil, err
	}
	return buildDashboardThroughputPoints(filled, concurrencyByMinute), nil
}

func (r *RequestLogRepository) GetAdminTimingTrend(windowKey string) (ttfbTrend []DashboardTimingPoint, durationTrend []DashboardTimingPoint, err error) {
	startMinute, endMinute := adminThroughputWindowBounds(time.Now().UTC(), windowKey)
	if err = r.ensureAdminProjectionWindow(startMinute, endMinute); err != nil {
		return nil, nil, err
	}

	rows, err := database.GetDB().Query(`
		SELECT minute_bucket, latency_ms, ttfb_ms
		FROM global_request_metric_projections
		WHERE minute_bucket >= ? AND minute_bucket <= ?
		ORDER BY minute_bucket ASC
	`, startMinute.UTC(), endMinute.UTC())
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	ttfbByBucket := make(map[time.Time][]float64)
	durationByBucket := make(map[time.Time][]float64)
	for rows.Next() {
		var row dashboardTimingProjectionRow
		if err = rows.Scan(&row.MinuteBucket, &row.LatencyMs, &row.TTFBMs); err != nil {
			return nil, nil, err
		}
		bucket := timingBucketStart(row.MinuteBucket)
		if row.TTFBMs > 0 {
			ttfbByBucket[bucket] = append(ttfbByBucket[bucket], float64(row.TTFBMs))
		}
		if row.LatencyMs > 0 {
			durationByBucket[bucket] = append(durationByBucket[bucket], float64(row.LatencyMs))
		}
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	startBucket := timingBucketStart(startMinute)
	endBucket := timingBucketStart(endMinute)
	return buildDashboardTimingPoints(startBucket, endBucket, ttfbByBucket), buildDashboardTimingPoints(startBucket, endBucket, durationByBucket), nil
}

func (r *RequestLogRepository) getAdminConcurrencyByMinute(start, end time.Time) (map[string]int64, error) {
	rows, err := database.GetDB().Query(`
		SELECT created_at, latency_ms
		FROM request_logs
		WHERE created_at >= ? AND created_at <= ? AND status <> 'pending' AND latency_ms > 0
		ORDER BY created_at ASC
	`, start.Add(-24*time.Hour).UTC(), end.Add(time.Minute).UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	totalMinutes := int(end.Sub(start)/time.Minute) + 1
	diff := make([]int64, totalMinutes+1)

	for rows.Next() {
		var createdAt time.Time
		var latencyMs int64
		if err := rows.Scan(&createdAt, &latencyMs); err != nil {
			return nil, err
		}
		startTime := createdAt.UTC()
		endTime := startTime.Add(time.Duration(latencyMs) * time.Millisecond)
		if endTime.Before(start) || startTime.After(end.Add(time.Minute)) {
			continue
		}

		startBucket := startTime.Truncate(time.Minute)
		endBucket := endTime.Truncate(time.Minute)
		if startBucket.Before(start) {
			startBucket = start
		}
		if endBucket.After(end) {
			endBucket = end
		}

		startIdx := int(startBucket.Sub(start) / time.Minute)
		endIdx := int(endBucket.Sub(start) / time.Minute)
		if startIdx < 0 {
			startIdx = 0
		}
		if endIdx >= totalMinutes {
			endIdx = totalMinutes - 1
		}
		diff[startIdx]++
		if endIdx+1 < len(diff) {
			diff[endIdx+1]--
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make(map[string]int64, totalMinutes)
	current := int64(0)
	for idx := 0; idx < totalMinutes; idx++ {
		current += diff[idx]
		minute := start.Add(time.Duration(idx) * time.Minute).UTC().Format(time.RFC3339)
		result[minute] = current
	}
	return result, nil
}

func (r *RequestLogRepository) ensureAdminProjectionWindow(start, end time.Time) error {
	var count int64
	err := database.GetDB().QueryRow(`
		SELECT COUNT(*)
		FROM global_request_metric_projections
		WHERE minute_bucket >= ? AND minute_bucket <= ?
	`, start.UTC(), end.UTC()).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return r.rebuildAdminProjectionWindow(start, end)
}

func (r *RequestLogRepository) rebuildAdminProjectionWindow(start, end time.Time) error {
	tx, err := database.GetDB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		DELETE FROM global_request_metric_projections
		WHERE minute_bucket >= ? AND minute_bucket <= ?
	`, start.UTC(), end.UTC()); err != nil {
		return err
	}

	rows, err := tx.Query(`
		SELECT id, created_at, COALESCE(input_tokens, 0), COALESCE(output_tokens, 0), COALESCE(latency_ms, 0), COALESCE(ttfb_ms, 0)
		FROM request_logs
		WHERE created_at >= ? AND created_at < ? AND status <> 'pending'
		ORDER BY created_at ASC
	`, start.UTC(), end.UTC().Add(time.Minute))
	if err != nil {
		return err
	}
	defer rows.Close()

	now := time.Now().UTC()
	for rows.Next() {
		var requestID string
		var createdAt time.Time
		var inputTokens int64
		var outputTokens int64
		var latencyMs int64
		var ttfbMs int64
		if err := rows.Scan(&requestID, &createdAt, &inputTokens, &outputTokens, &latencyMs, &ttfbMs); err != nil {
			return err
		}

		if _, err := tx.Exec(`
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
			requestID,
			createdAt.UTC().Truncate(time.Minute),
			1,
			inputTokens,
			outputTokens,
			inputTokens+outputTokens,
			latencyMs,
			ttfbMs,
			now,
		); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return tx.Commit()
}
