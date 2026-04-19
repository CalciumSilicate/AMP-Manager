package repository

import (
	"database/sql"
	"strings"
	"sync"
	"time"

	"ampmanager/internal/database"

	log "github.com/sirupsen/logrus"
)

const (
	dashboardAggregateJobTargetMinute    = "minute"
	dashboardAggregateJobTargetDay       = "day"
	dashboardAggregateJobTargetModelDay  = "model_day"
	dashboardAggregateJobTargetProvider  = "provider_day"
	defaultDashboardAggregateBatchSize   = 512
	defaultDashboardAggregateFlushPeriod = 500 * time.Millisecond
)

type dashboardAggregateJob struct {
	ID                             int64
	TargetType                     string
	ScopeType                      string
	ScopeID                        string
	MinuteBucket                   *time.Time
	DayBucket                      string
	Model                          string
	Provider                       string
	RequestCountDelta              int64
	InputTokensDelta               int64
	OutputTokensDelta              int64
	TotalTokensDelta               int64
	CostMicrosDelta                int64
	ErrorCountDelta                int64
	CacheReadInputTokensDelta      int64
	CacheCreationInputTokensDelta  int64
	LatencySumMsDelta              int64
	LatencySampleCountDelta        int64
	TTFBSumMsDelta                 int64
	TTFBSampleCountDelta           int64
	ConcurrencyDelta               int64
}

type dashboardMinuteMetricDelta struct {
	RequestCount         int64
	InputTokens          int64
	OutputTokens         int64
	TotalTokens          int64
	CostMicros           int64
	ErrorCount           int64
	CacheReadInputTokens int64
	CacheCreationTokens  int64
	LatencySumMs         int64
	LatencySampleCount   int64
	TTFBSumMs            int64
	TTFBSampleCount      int64
	ConcurrencyDelta     int64
}

type dashboardDayMetricDelta struct {
	RequestCount int64
	CostMicros   int64
}

type dashboardProviderMetricDelta struct {
	RequestCount         int64
	TotalInputTokens     int64
	CacheReadInputTokens int64
	CacheCreationTokens  int64
}

type dashboardAggregateWorker struct {
	batchSize     int
	flushInterval time.Duration
	stopChan      chan struct{}
	wakeChan      chan struct{}
	wg            sync.WaitGroup
}

var (
	dashboardAggregateWorkerMu sync.Mutex
	globalDashboardAggregateWorker *dashboardAggregateWorker
)

func StartDashboardAggregateWorker() {
	dashboardAggregateWorkerMu.Lock()
	defer dashboardAggregateWorkerMu.Unlock()

	if globalDashboardAggregateWorker != nil || database.GetDB() == nil {
		return
	}

	worker := &dashboardAggregateWorker{
		batchSize:     defaultDashboardAggregateBatchSize,
		flushInterval: defaultDashboardAggregateFlushPeriod,
		stopChan:      make(chan struct{}),
		wakeChan:      make(chan struct{}, 1),
	}
	worker.wg.Add(1)
	go worker.run()
	globalDashboardAggregateWorker = worker
}

func StopDashboardAggregateWorker() {
	dashboardAggregateWorkerMu.Lock()
	worker := globalDashboardAggregateWorker
	globalDashboardAggregateWorker = nil
	dashboardAggregateWorkerMu.Unlock()

	if worker == nil {
		return
	}
	close(worker.stopChan)
	worker.wg.Wait()
}

func RestartDashboardAggregateWorker() {
	StopDashboardAggregateWorker()
	StartDashboardAggregateWorker()
}

func NotifyDashboardAggregateWorker() {
	dashboardAggregateWorkerMu.Lock()
	worker := globalDashboardAggregateWorker
	dashboardAggregateWorkerMu.Unlock()
	if worker == nil {
		return
	}
	select {
	case worker.wakeChan <- struct{}{}:
	default:
	}
}

func (w *dashboardAggregateWorker) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-w.wakeChan:
		case <-w.stopChan:
			w.processAllAvailable()
			return
		}

		if err := w.processAllAvailable(); err != nil {
			log.Warnf("dashboard aggregate worker: process failed: %v", err)
		}
	}
}

func (w *dashboardAggregateWorker) processAllAvailable() error {
	for {
		count, err := processDashboardAggregateJobBatch(w.batchSize)
		if err != nil {
			return err
		}
		if count < w.batchSize {
			return nil
		}
	}
}

func enqueueDashboardAggregateJobTx(tx *sql.Tx, job dashboardAggregateJob) error {
	var minuteBucket any
	if job.MinuteBucket != nil {
		minuteBucket = job.MinuteBucket.UTC()
	}

	_, err := tx.Exec(`
		INSERT INTO dashboard_aggregate_jobs (
			target_type, scope_type, scope_id, minute_bucket, day_bucket, model, provider,
			request_count_delta, input_tokens_delta, output_tokens_delta, total_tokens_delta,
			cost_micros_delta, error_count_delta, cache_read_input_tokens_delta, cache_creation_input_tokens_delta,
			latency_sum_ms_delta, latency_sample_count_delta, ttfb_sum_ms_delta, ttfb_sample_count_delta,
			concurrency_delta, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		job.TargetType,
		job.ScopeType,
		job.ScopeID,
		minuteBucket,
		job.DayBucket,
		job.Model,
		job.Provider,
		job.RequestCountDelta,
		job.InputTokensDelta,
		job.OutputTokensDelta,
		job.TotalTokensDelta,
		job.CostMicrosDelta,
		job.ErrorCountDelta,
		job.CacheReadInputTokensDelta,
		job.CacheCreationInputTokensDelta,
		job.LatencySumMsDelta,
		job.LatencySampleCountDelta,
		job.TTFBSumMsDelta,
		job.TTFBSampleCountDelta,
		job.ConcurrencyDelta,
		time.Now().UTC(),
	)
	return err
}

func enqueueDashboardAggregateDeltaTx(tx *sql.Tx, snapshot dashboardProjectionSnapshot, sign int64) error {
	if sign == 0 {
		return nil
	}

	scopeTypes := []string{dashboardScopeGlobal}
	if strings.TrimSpace(snapshot.UserID) != "" {
		scopeTypes = append(scopeTypes, "user")
	}

	for _, scopeType := range scopeTypes {
		scopeID := dashboardScopeID(scopeType, snapshot.UserID)
		minuteBucket := snapshot.MinuteBucket.UTC()
		primaryMinuteJob := dashboardAggregateJob{
			TargetType:                    dashboardAggregateJobTargetMinute,
			ScopeType:                     scopeType,
			ScopeID:                       scopeID,
			MinuteBucket:                  &minuteBucket,
			RequestCountDelta:             snapshot.RequestCount * sign,
			InputTokensDelta:              snapshot.InputTokens * sign,
			OutputTokensDelta:             snapshot.OutputTokens * sign,
			TotalTokensDelta:              snapshot.TotalTokens * sign,
			CostMicrosDelta:               snapshot.CostMicros * sign,
			ErrorCountDelta:               snapshot.ErrorCount * sign,
			CacheReadInputTokensDelta:     snapshot.CacheReadInputTokens * sign,
			CacheCreationInputTokensDelta: snapshot.CacheCreationInputTokens * sign,
		}
		if snapshot.LatencyMs > 0 {
			primaryMinuteJob.LatencySumMsDelta = snapshot.LatencyMs * sign
			primaryMinuteJob.LatencySampleCountDelta = sign
			primaryMinuteJob.ConcurrencyDelta = sign
		}
		if snapshot.TTFBMs > 0 {
			primaryMinuteJob.TTFBSumMsDelta = snapshot.TTFBMs * sign
			primaryMinuteJob.TTFBSampleCountDelta = sign
		}
		if err := enqueueDashboardAggregateJobTx(tx, primaryMinuteJob); err != nil {
			return err
		}

		if snapshot.LatencyMs > 0 {
			endMinuteBucket := snapshot.CompletedMinuteBucket.Add(time.Minute).UTC()
			if err := enqueueDashboardAggregateJobTx(tx, dashboardAggregateJob{
				TargetType:       dashboardAggregateJobTargetMinute,
				ScopeType:        scopeType,
				ScopeID:          scopeID,
				MinuteBucket:     &endMinuteBucket,
				ConcurrencyDelta: -sign,
			}); err != nil {
				return err
			}
		}

		if err := enqueueDashboardAggregateJobTx(tx, dashboardAggregateJob{
			TargetType:        dashboardAggregateJobTargetDay,
			ScopeType:         scopeType,
			ScopeID:           scopeID,
			DayBucket:         snapshot.DayBucket,
			RequestCountDelta: snapshot.RequestCount * sign,
			CostMicrosDelta:   snapshot.CostMicros * sign,
		}); err != nil {
			return err
		}

		if err := enqueueDashboardAggregateJobTx(tx, dashboardAggregateJob{
			TargetType:        dashboardAggregateJobTargetModelDay,
			ScopeType:         scopeType,
			ScopeID:           scopeID,
			DayBucket:         snapshot.DayBucket,
			Model:             snapshot.EffectiveModel,
			RequestCountDelta: snapshot.RequestCount * sign,
			CostMicrosDelta:   snapshot.CostMicros * sign,
		}); err != nil {
			return err
		}

		if err := enqueueDashboardAggregateJobTx(tx, dashboardAggregateJob{
			TargetType:                    dashboardAggregateJobTargetProvider,
			ScopeType:                     scopeType,
			ScopeID:                       scopeID,
			DayBucket:                     snapshot.DayBucket,
			Provider:                      snapshot.ProviderGroup,
			RequestCountDelta:             snapshot.RequestCount * sign,
			InputTokensDelta:              snapshot.InputTokens * sign,
			CacheReadInputTokensDelta:     snapshot.CacheReadInputTokens * sign,
			CacheCreationInputTokensDelta: snapshot.CacheCreationInputTokens * sign,
		}); err != nil {
			return err
		}
	}

	return nil
}

func processDashboardAggregateJobBatch(batchSize int) (int, error) {
	db := database.GetDB()
	if db == nil {
		return 0, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT id, target_type, scope_type, scope_id, minute_bucket, day_bucket, model, provider,
		       request_count_delta, input_tokens_delta, output_tokens_delta, total_tokens_delta,
		       cost_micros_delta, error_count_delta, cache_read_input_tokens_delta, cache_creation_input_tokens_delta,
		       latency_sum_ms_delta, latency_sample_count_delta, ttfb_sum_ms_delta, ttfb_sample_count_delta,
		       concurrency_delta
		FROM dashboard_aggregate_jobs
		ORDER BY id ASC
		LIMIT ?
	`, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	jobs := make([]dashboardAggregateJob, 0, batchSize)
	for rows.Next() {
		var job dashboardAggregateJob
		var minuteBucket sql.NullTime
		if err := rows.Scan(
			&job.ID,
			&job.TargetType,
			&job.ScopeType,
			&job.ScopeID,
			&minuteBucket,
			&job.DayBucket,
			&job.Model,
			&job.Provider,
			&job.RequestCountDelta,
			&job.InputTokensDelta,
			&job.OutputTokensDelta,
			&job.TotalTokensDelta,
			&job.CostMicrosDelta,
			&job.ErrorCountDelta,
			&job.CacheReadInputTokensDelta,
			&job.CacheCreationInputTokensDelta,
			&job.LatencySumMsDelta,
			&job.LatencySampleCountDelta,
			&job.TTFBSumMsDelta,
			&job.TTFBSampleCountDelta,
			&job.ConcurrencyDelta,
		); err != nil {
			return 0, err
		}
		if minuteBucket.Valid {
			value := minuteBucket.Time.UTC()
			job.MinuteBucket = &value
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}

	if err := applyDashboardAggregateJobBatchTx(tx, jobs); err != nil {
		return 0, err
	}
	if err := deleteDashboardAggregateJobsTx(tx, jobs); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return len(jobs), nil
}

func applyDashboardAggregateJobBatchTx(tx *sql.Tx, jobs []dashboardAggregateJob) error {
	type minuteKey struct {
		scopeType string
		scopeID   string
		minute    time.Time
	}
	type dayKey struct {
		scopeType string
		scopeID   string
		dayBucket string
	}
	type modelKey struct {
		scopeType string
		scopeID   string
		dayBucket string
		model     string
	}
	type providerKey struct {
		scopeType string
		scopeID   string
		dayBucket string
		provider  string
	}

	minuteDeltas := make(map[minuteKey]dashboardMinuteMetricDelta)
	dayDeltas := make(map[dayKey]dashboardDayMetricDelta)
	modelDeltas := make(map[modelKey]dashboardDayMetricDelta)
	providerDeltas := make(map[providerKey]dashboardProviderMetricDelta)

	for _, job := range jobs {
		switch job.TargetType {
		case dashboardAggregateJobTargetMinute:
			if job.MinuteBucket == nil {
				continue
			}
			key := minuteKey{scopeType: job.ScopeType, scopeID: job.ScopeID, minute: job.MinuteBucket.UTC()}
			delta := minuteDeltas[key]
			delta.RequestCount += job.RequestCountDelta
			delta.InputTokens += job.InputTokensDelta
			delta.OutputTokens += job.OutputTokensDelta
			delta.TotalTokens += job.TotalTokensDelta
			delta.CostMicros += job.CostMicrosDelta
			delta.ErrorCount += job.ErrorCountDelta
			delta.CacheReadInputTokens += job.CacheReadInputTokensDelta
			delta.CacheCreationTokens += job.CacheCreationInputTokensDelta
			delta.LatencySumMs += job.LatencySumMsDelta
			delta.LatencySampleCount += job.LatencySampleCountDelta
			delta.TTFBSumMs += job.TTFBSumMsDelta
			delta.TTFBSampleCount += job.TTFBSampleCountDelta
			delta.ConcurrencyDelta += job.ConcurrencyDelta
			minuteDeltas[key] = delta
		case dashboardAggregateJobTargetDay:
			key := dayKey{scopeType: job.ScopeType, scopeID: job.ScopeID, dayBucket: job.DayBucket}
			delta := dayDeltas[key]
			delta.RequestCount += job.RequestCountDelta
			delta.CostMicros += job.CostMicrosDelta
			dayDeltas[key] = delta
		case dashboardAggregateJobTargetModelDay:
			key := modelKey{scopeType: job.ScopeType, scopeID: job.ScopeID, dayBucket: job.DayBucket, model: job.Model}
			delta := modelDeltas[key]
			delta.RequestCount += job.RequestCountDelta
			delta.CostMicros += job.CostMicrosDelta
			modelDeltas[key] = delta
		case dashboardAggregateJobTargetProvider:
			key := providerKey{scopeType: job.ScopeType, scopeID: job.ScopeID, dayBucket: job.DayBucket, provider: job.Provider}
			delta := providerDeltas[key]
			delta.RequestCount += job.RequestCountDelta
			delta.TotalInputTokens += job.InputTokensDelta
			delta.CacheReadInputTokens += job.CacheReadInputTokensDelta
			delta.CacheCreationTokens += job.CacheCreationInputTokensDelta
			providerDeltas[key] = delta
		}
	}

	now := time.Now().UTC()
	for key, delta := range minuteDeltas {
		if err := applyMinuteMetricDeltaTx(tx, key.scopeType, key.scopeID, key.minute, delta, now); err != nil {
			return err
		}
	}
	for key, delta := range dayDeltas {
		if err := applyDayMetricDeltaTx(tx, key.scopeType, key.scopeID, key.dayBucket, delta, now); err != nil {
			return err
		}
	}
	for key, delta := range modelDeltas {
		if err := applyModelMetricDeltaTx(tx, key.scopeType, key.scopeID, key.dayBucket, key.model, delta, now); err != nil {
			return err
		}
	}
	for key, delta := range providerDeltas {
		if err := applyProviderMetricDeltaTx(tx, key.scopeType, key.scopeID, key.dayBucket, key.provider, delta, now); err != nil {
			return err
		}
	}

	return nil
}

func applyMinuteMetricDeltaTx(tx *sql.Tx, scopeType, scopeID string, minute time.Time, delta dashboardMinuteMetricDelta, now time.Time) error {
	_, err := tx.Exec(`
		INSERT INTO dashboard_minute_metrics (
			scope_type, scope_id, minute_bucket, request_count_sum, input_tokens_sum, output_tokens_sum, total_tokens_sum,
			cost_micros_sum, error_count_sum, cache_read_input_tokens_sum, cache_creation_input_tokens_sum,
			latency_sum_ms, latency_sample_count, ttfb_sum_ms, ttfb_sample_count, concurrency_delta_sum, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			concurrency_delta_sum = dashboard_minute_metrics.concurrency_delta_sum + excluded.concurrency_delta_sum,
			updated_at = excluded.updated_at
	`,
		scopeType,
		scopeID,
		minute.UTC(),
		delta.RequestCount,
		delta.InputTokens,
		delta.OutputTokens,
		delta.TotalTokens,
		delta.CostMicros,
		delta.ErrorCount,
		delta.CacheReadInputTokens,
		delta.CacheCreationTokens,
		delta.LatencySumMs,
		delta.LatencySampleCount,
		delta.TTFBSumMs,
		delta.TTFBSampleCount,
		delta.ConcurrencyDelta,
		now,
	)
	return err
}

func applyDayMetricDeltaTx(tx *sql.Tx, scopeType, scopeID, dayBucket string, delta dashboardDayMetricDelta, now time.Time) error {
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
		delta.RequestCount,
		delta.CostMicros,
		now,
	)
	return err
}

func applyModelMetricDeltaTx(tx *sql.Tx, scopeType, scopeID, dayBucket, model string, delta dashboardDayMetricDelta, now time.Time) error {
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
		model,
		delta.RequestCount,
		delta.CostMicros,
		now,
	)
	return err
}

func applyProviderMetricDeltaTx(tx *sql.Tx, scopeType, scopeID, dayBucket, provider string, delta dashboardProviderMetricDelta, now time.Time) error {
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
		provider,
		delta.RequestCount,
		delta.TotalInputTokens,
		delta.CacheReadInputTokens,
		delta.CacheCreationTokens,
		now,
	)
	return err
}

func deleteDashboardAggregateJobsTx(tx *sql.Tx, jobs []dashboardAggregateJob) error {
	if len(jobs) == 0 {
		return nil
	}
	ids := make([]any, len(jobs))
	for i, job := range jobs {
		ids[i] = job.ID
	}
	query := `DELETE FROM dashboard_aggregate_jobs WHERE id IN (` + database.PlaceholderList(len(ids)) + `)`
	_, err := tx.Exec(database.Rebind(query), ids...)
	return err
}
