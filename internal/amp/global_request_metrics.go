package amp

import (
	"database/sql"
	"time"

	"ampmanager/internal/repository"
)

func valueOrZeroInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func syncGlobalRequestMetricsTx(tx *sql.Tx, snapshot RequestTrace, location *time.Location) error {
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

	cacheReadInputTokens := valueOrZeroInt64(int64PtrFromInt(snapshot.CacheReadInputTokens))
	cacheCreationInputTokens := valueOrZeroInt64(int64PtrFromInt(snapshot.CacheCreationInputTokens))
	costMicros := valueOrZeroInt64(snapshot.CostMicros)

	return repository.NewRequestLogRepository().SyncDashboardProjectionTx(tx, repository.DashboardProjectionInput{
		RequestID:                snapshot.RequestID,
		UserID:                   snapshot.UserID,
		StartTime:                snapshot.StartTime.UTC(),
		MappedModel:              snapshot.MappedModel,
		OriginalModel:            snapshot.OriginalModel,
		StatusCode:               snapshot.StatusCode,
		LatencyMs:                snapshot.LatencyMs,
		TTFBMs:                   valueOrZeroInt64(snapshot.TTFBMs),
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheReadInputTokens:     cacheReadInputTokens,
		CacheCreationInputTokens: cacheCreationInputTokens,
		CostMicros:               costMicros,
	}, location)
}

func int64PtrFromInt(value *int) *int64 {
	if value == nil {
		return nil
	}
	converted := int64(*value)
	return &converted
}
