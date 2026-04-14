package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"ampmanager/internal/model"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestSummarizeBillingRuntimeMetric(t *testing.T) {
	summary := summarizeBillingRuntimeMetric([]time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
	}, 3)

	if summary.Samples != 5 {
		t.Fatalf("Samples = %d, want 5", summary.Samples)
	}
	if summary.P95Ms != 500 {
		t.Fatalf("P95Ms = %d, want 500", summary.P95Ms)
	}
	if summary.P99Ms != 500 {
		t.Fatalf("P99Ms = %d, want 500", summary.P99Ms)
	}
	if summary.Failures != 3 {
		t.Fatalf("Failures = %d, want 3", summary.Failures)
	}
}

func TestSummarizeBillingRuntimeMetricEmpty(t *testing.T) {
	summary := summarizeBillingRuntimeMetric(nil, 0)

	if summary.Samples != 0 || summary.P95Ms != 0 || summary.P99Ms != 0 || summary.Failures != 0 {
		t.Fatalf("unexpected zero summary: %+v", summary)
	}
}

func TestLoadBillingRuntimeProjectorSummaryReturnsZeroWhenRuntimeDisabled(t *testing.T) {
	summary, err := loadBillingRuntimeProjectorSummary(context.Background(), model.BillingRuntimeConfigRequest{})
	if err != nil {
		t.Fatalf("loadBillingRuntimeProjectorSummary returned error: %v", err)
	}
	if summary != (billingRuntimeProjectorSummary{}) {
		t.Fatalf("summary = %+v, want zero", summary)
	}
}

func TestLoadBillingRuntimeProjectorSummaryReturnsZeroWhenGroupMissing(t *testing.T) {
	mr := miniredis.RunT(t)

	summary, err := loadBillingRuntimeProjectorSummary(context.Background(), model.BillingRuntimeConfigRequest{
		RedisURL:              "redis://" + mr.Addr(),
		RedisPrefix:           "team-a",
		ProjectorClaimIdleSec: 30,
	})
	if err != nil {
		t.Fatalf("loadBillingRuntimeProjectorSummary returned error: %v", err)
	}
	if summary != (billingRuntimeProjectorSummary{}) {
		t.Fatalf("summary = %+v, want zero", summary)
	}
}

func TestLoadBillingRuntimeProjectorSummarySummarizesConsumersAndPending(t *testing.T) {
	mr := miniredis.RunT(t)
	now := time.Unix(1_700_000_000, 0)
	mr.SetTime(now)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	streamKey := billingRuntimeStreamKey("team-a")
	if err := client.XGroupCreateMkStream(context.Background(), streamKey, billingProjectorGroup, "0").Err(); err != nil {
		t.Fatalf("XGroupCreateMkStream returned error: %v", err)
	}

	busyID := addBillingRuntimeEventForTest(t, client, streamKey, "busy")
	staleID := addBillingRuntimeEventForTest(t, client, streamKey, "stale")
	freshID := addBillingRuntimeEventForTest(t, client, streamKey, "fresh")

	if err := readPendingForTest(client, streamKey, "busy-consumer"); err != nil {
		t.Fatalf("readPendingForTest busy returned error: %v", err)
	}
	if err := readPendingForTest(client, streamKey, "stale-consumer"); err != nil {
		t.Fatalf("readPendingForTest stale returned error: %v", err)
	}
	if _, err := client.XAck(context.Background(), streamKey, billingProjectorGroup, staleID).Result(); err != nil {
		t.Fatalf("XAck stale returned error: %v", err)
	}

	mr.SetTime(now.Add(45 * time.Second))

	if err := readPendingForTest(client, streamKey, "fresh-consumer"); err != nil {
		t.Fatalf("readPendingForTest fresh returned error: %v", err)
	}
	if _, err := client.XAck(context.Background(), streamKey, billingProjectorGroup, freshID).Result(); err != nil {
		t.Fatalf("XAck fresh returned error: %v", err)
	}

	summary, err := loadBillingRuntimeProjectorSummary(context.Background(), model.BillingRuntimeConfigRequest{
		RedisURL:              "redis://" + mr.Addr(),
		RedisPrefix:           "team-a",
		ProjectorClaimIdleSec: 30,
	})
	if err != nil {
		t.Fatalf("loadBillingRuntimeProjectorSummary returned error: %v", err)
	}

	if summary.ConsumerCount != 3 {
		t.Fatalf("ConsumerCount = %d, want 3", summary.ConsumerCount)
	}
	if summary.PendingEntries != 1 {
		t.Fatalf("PendingEntries = %d, want 1", summary.PendingEntries)
	}
	if summary.OldestPendingMs < 45_000 {
		t.Fatalf("OldestPendingMs = %d, want >= 45000", summary.OldestPendingMs)
	}

	pending, err := client.XPendingExt(context.Background(), &redis.XPendingExtArgs{
		Stream: streamKey,
		Group:  billingProjectorGroup,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if err != nil {
		t.Fatalf("XPendingExt returned error: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != busyID {
		t.Fatalf("pending = %+v, want only busy entry", pending)
	}
}

func TestSummarizeBillingRuntimeProjectorConsumers(t *testing.T) {
	active, stale := summarizeBillingRuntimeProjectorConsumers([]redis.XInfoConsumer{
		{Name: "busy-consumer", Pending: 1, Idle: 2 * time.Minute},
		{Name: "fresh-consumer", Pending: 0, Idle: 10 * time.Second},
		{Name: "unknown-idle-consumer", Pending: 0, Idle: -1 * time.Millisecond},
		{Name: "stale-consumer", Pending: 0, Idle: 31 * time.Second},
	}, 30*time.Second)

	if active != 3 {
		t.Fatalf("active = %d, want 3", active)
	}
	if stale != 1 {
		t.Fatalf("stale = %d, want 1", stale)
	}
}

func addBillingRuntimeEventForTest(t *testing.T, client *redis.Client, streamKey string, label string) string {
	t.Helper()

	id, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]any{
			"event_type": label,
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd returned error: %v", err)
	}
	return id
}

func readPendingForTest(client *redis.Client, streamKey string, consumer string) error {
	streams, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    billingProjectorGroup,
		Consumer: consumer,
		Streams:  []string{streamKey, ">"},
		Count:    1,
	}).Result()
	if err != nil {
		return err
	}
	if len(streams) != 1 || len(streams[0].Messages) != 1 {
		return errors.New("expected one pending message")
	}
	return nil
}
