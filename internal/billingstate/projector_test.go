package billingstate

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ampmanager/internal/database"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestBuildProjectorConsumerNamesReturnsUniqueStableNames(t *testing.T) {
	names := buildProjectorConsumerNames("host-a", 1234, "instance-a", 3)
	want := []string{
		"host-a-1234-instance-a-projector-1",
		"host-a-1234-instance-a-projector-2",
		"host-a-1234-instance-a-projector-3",
	}

	if len(names) != len(want) {
		t.Fatalf("consumer names length = %d, want %d", len(names), len(want))
	}
	for idx := range want {
		if names[idx] != want[idx] {
			t.Fatalf("consumer name[%d] = %q, want %q", idx, names[idx], want[idx])
		}
	}
}

func TestBuildStartsConfiguredProjectorWorkers(t *testing.T) {
	mr := miniredis.RunT(t)
	rt, err := Build(Config{
		RedisURL:         fmt.Sprintf("redis://%s/0", mr.Addr()),
		Prefix:           "amp",
		StreamBatchSize:  2,
		ProjectorWorkers: 3,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if rt == nil {
		t.Fatal("Build returned nil runtime")
	}
	defer rt.Close()

	if got := len(rt.projectorConsumers); got != 3 {
		t.Fatalf("projector consumer count = %d, want 3", got)
	}
	seen := make(map[string]struct{}, len(rt.projectorConsumers))
	for _, name := range rt.projectorConsumers {
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate projector consumer name %q", name)
		}
		seen[name] = struct{}{}
	}
}

func TestProjectMessagesFallsBackToPerMessageAck(t *testing.T) {
	setupBillingStateTestDB(t)

	now := time.Now().UTC()
	db := database.GetDB()
	mustExecBillingState(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 20)`, "user-1")
	mustExecBillingState(t, db, `INSERT INTO billing_account_state (user_id, primary_source, secondary_source, balance_micros, revision, created_at, updated_at) VALUES (?, 'subscription', 'balance', 20, 0, ?, ?)`, "user-1", now, now)

	rt := &Runtime{}
	messages := []redis.XMessage{
		{
			ID: "1-0",
			Values: map[string]any{
				"event_type":                   "reserve",
				"request_id":                   "req-good",
				"user_id":                      "user-1",
				"user_subscription_id":         "",
				"status":                       "reserved",
				"estimated_cost_micros":        "5",
				"reserved_subscription_micros": "0",
				"reserved_balance_micros":      "5",
				"window_refs_json":             "[]",
				"expires_at_unix":              now.Add(5 * time.Minute).Unix(),
			},
		},
		{
			ID: "2-0",
			Values: map[string]any{
				"event_type":                   "reserve",
				"request_id":                   "req-bad",
				"user_id":                      "user-1",
				"user_subscription_id":         "",
				"status":                       "reserved",
				"estimated_cost_micros":        "3",
				"reserved_subscription_micros": "0",
				"reserved_balance_micros":      "3",
				"window_refs_json":             "{bad-json",
				"expires_at_unix":              now.Add(5 * time.Minute).Unix(),
			},
		},
	}

	acked := rt.projectMessages(context.Background(), messages)
	if len(acked) != 1 || acked[0] != "1-0" {
		t.Fatalf("acked = %v, want [1-0]", acked)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_projection_events`).Scan(&count); err != nil {
		t.Fatalf("query billing_projection_events returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("projection event count = %d, want 1", count)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_reservations WHERE request_id = ?`, "req-good").Scan(&count); err != nil {
		t.Fatalf("query good reservation returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("good reservation count = %d, want 1", count)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_reservations WHERE request_id = ?`, "req-bad").Scan(&count); err != nil {
		t.Fatalf("query bad reservation returned error: %v", err)
	}
	if count != 0 {
		t.Fatalf("bad reservation count = %d, want 0", count)
	}

	var balance int64
	if err := db.QueryRow(`SELECT balance_micros FROM billing_account_state WHERE user_id = ?`, "user-1").Scan(&balance); err != nil {
		t.Fatalf("query billing_account_state returned error: %v", err)
	}
	if balance != 15 {
		t.Fatalf("balance_micros = %d, want 15", balance)
	}
}

func TestReclaimPendingEntriesProjectsAndAcksClaimedMessages(t *testing.T) {
	setupBillingStateTestDB(t)

	now := time.Now().UTC()
	db := database.GetDB()
	mustExecBillingState(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 20)`, "user-1")
	mustExecBillingState(t, db, `INSERT INTO billing_account_state (user_id, primary_source, secondary_source, balance_micros, revision, created_at, updated_at) VALUES (?, 'subscription', 'balance', 20, 0, ?, ?)`, "user-1", now, now)

	mr := miniredis.RunT(t)
	mr.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	rt := &Runtime{
		client: client,
		cfg: Config{
			Prefix:          "amp",
			StreamBatchSize: 4,
		},
	}
	if err := rt.ensureConsumerGroup(context.Background()); err != nil {
		t.Fatalf("ensureConsumerGroup returned error: %v", err)
	}

	msgID, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: rt.streamKey(),
		Values: map[string]any{
			"event_type":                   "reserve",
			"request_id":                   "req-claim",
			"user_id":                      "user-1",
			"user_subscription_id":         "",
			"status":                       "reserved",
			"estimated_cost_micros":        "5",
			"reserved_subscription_micros": "0",
			"reserved_balance_micros":      "5",
			"window_refs_json":             "[]",
			"expires_at_unix":              now.Add(5 * time.Minute).Unix(),
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd returned error: %v", err)
	}

	streams, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    defaultProjectorGroup,
		Consumer: "stale-consumer",
		Streams:  []string{rt.streamKey(), ">"},
		Count:    1,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup returned error: %v", err)
	}
	if len(streams) != 1 || len(streams[0].Messages) != 1 || streams[0].Messages[0].ID != msgID {
		t.Fatalf("unexpected initial read result: %+v", streams)
	}

	mr.SetTime(now.Add(defaultProjectorClaimIdle + time.Second))

	count, err := rt.reclaimPendingEntries(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("reclaimPendingEntries returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("reclaimed count = %d, want 1", count)
	}

	var reservationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_reservations WHERE request_id = ?`, "req-claim").Scan(&reservationCount); err != nil {
		t.Fatalf("query reservation returned error: %v", err)
	}
	if reservationCount != 1 {
		t.Fatalf("reservation count = %d, want 1", reservationCount)
	}

	pending, err := client.XPending(context.Background(), rt.streamKey(), defaultProjectorGroup).Result()
	if err != nil {
		t.Fatalf("XPending returned error: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("pending count = %d, want 0", pending.Count)
	}

	metrics := rt.SnapshotMetrics(false)
	if got := len(metrics.ReclaimDurations); got != 1 {
		t.Fatalf("reclaim duration samples = %d, want 1", got)
	}
	if metrics.ReclaimFailures != 0 {
		t.Fatalf("reclaim failures = %d, want 0", metrics.ReclaimFailures)
	}
	if metrics.ReclaimClaimed != 1 {
		t.Fatalf("reclaim claimed = %d, want 1", metrics.ReclaimClaimed)
	}
}

func TestReclaimPendingEntriesAfterCrashProcessesStalePendingAcrossConsumers(t *testing.T) {
	setupBillingStateTestDB(t)

	now := time.Now().UTC()
	db := database.GetDB()
	mustExecBillingState(t, db, `INSERT INTO users (id, username, password_hash, is_admin, balance_micros) VALUES (?, 'alice', 'x', 0, 20)`, "user-1")
	mustExecBillingState(t, db, `INSERT INTO billing_account_state (user_id, primary_source, secondary_source, balance_micros, revision, created_at, updated_at) VALUES (?, 'subscription', 'balance', 20, 0, ?, ?)`, "user-1", now, now)

	mr := miniredis.RunT(t)
	mr.SetTime(now)
	crashClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	restartClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = restartClient.Close() })

	rtBeforeCrash := &Runtime{
		client: crashClient,
		cfg: Config{
			Prefix:          "amp",
			StreamBatchSize: 8,
		},
		projectorConsumers: []string{"worker-a", "worker-b"},
		stopCh:             make(chan struct{}),
	}
	if err := rtBeforeCrash.ensureConsumerGroup(context.Background()); err != nil {
		t.Fatalf("ensureConsumerGroup returned error: %v", err)
	}

	for _, requestID := range []string{"req-crash-1", "req-crash-2"} {
		if _, err := crashClient.XAdd(context.Background(), &redis.XAddArgs{
			Stream: rtBeforeCrash.streamKey(),
			Values: map[string]any{
				"event_type":                   "reserve",
				"request_id":                   requestID,
				"user_id":                      "user-1",
				"user_subscription_id":         "",
				"status":                       "reserved",
				"estimated_cost_micros":        "5",
				"reserved_subscription_micros": "0",
				"reserved_balance_micros":      "5",
				"window_refs_json":             "[]",
				"expires_at_unix":              now.Add(5 * time.Minute).Unix(),
			},
		}).Result(); err != nil {
			t.Fatalf("XAdd returned error: %v", err)
		}
	}

	for _, consumerName := range []string{"worker-a", "worker-b"} {
		streams, err := crashClient.XReadGroup(context.Background(), &redis.XReadGroupArgs{
			Group:    defaultProjectorGroup,
			Consumer: consumerName,
			Streams:  []string{rtBeforeCrash.streamKey(), ">"},
			Count:    1,
		}).Result()
		if err != nil {
			t.Fatalf("XReadGroup returned error: %v", err)
		}
		if len(streams) != 1 || len(streams[0].Messages) != 1 {
			t.Fatalf("unexpected initial read result for %s: %+v", consumerName, streams)
		}
	}

	rtBeforeCrash.Close()
	mr.SetTime(now.Add(defaultProjectorClaimIdle + time.Second))

	rtAfterRestart := &Runtime{
		client: restartClient,
		cfg: Config{
			Prefix:          "amp",
			StreamBatchSize: 8,
		},
	}

	reclaimed, err := rtAfterRestart.reclaimPendingEntries(context.Background(), "worker-restart")
	if err != nil {
		t.Fatalf("reclaimPendingEntries returned error: %v", err)
	}
	if reclaimed != 2 {
		t.Fatalf("reclaimed count = %d, want 2", reclaimed)
	}

	for _, requestID := range []string{"req-crash-1", "req-crash-2"} {
		var reservationCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM billing_reservations WHERE request_id = ?`, requestID).Scan(&reservationCount); err != nil {
			t.Fatalf("query reservation returned error: %v", err)
		}
		if reservationCount != 1 {
			t.Fatalf("reservation count for %s = %d, want 1", requestID, reservationCount)
		}
	}

	pending, err := restartClient.XPending(context.Background(), rtAfterRestart.streamKey(), defaultProjectorGroup).Result()
	if err != nil {
		t.Fatalf("XPending returned error: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("pending count = %d, want 0", pending.Count)
	}

	metrics := rtAfterRestart.SnapshotMetrics(false)
	if got := len(metrics.ReclaimDurations); got != 1 {
		t.Fatalf("reclaim duration samples = %d, want 1", got)
	}
	if metrics.ReclaimFailures != 0 {
		t.Fatalf("reclaim failures = %d, want 0", metrics.ReclaimFailures)
	}
	if metrics.ReclaimClaimed != 2 {
		t.Fatalf("reclaim claimed = %d, want 2", metrics.ReclaimClaimed)
	}
}

func TestRuntimeCloseDeletesIdleProjectorConsumer(t *testing.T) {
	mr := miniredis.RunT(t)
	runtimeClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	inspectClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = inspectClient.Close() })

	rt := &Runtime{
		client:             runtimeClient,
		cfg:                Config{Prefix: "amp"},
		projectorConsumers: []string{"worker-1"},
		stopCh:             make(chan struct{}),
	}
	if err := rt.ensureConsumerGroup(context.Background()); err != nil {
		t.Fatalf("ensureConsumerGroup returned error: %v", err)
	}
	if _, err := runtimeClient.XGroupCreateConsumer(context.Background(), rt.streamKey(), defaultProjectorGroup, "worker-1").Result(); err != nil {
		t.Fatalf("XGroupCreateConsumer returned error: %v", err)
	}

	rt.Close()

	consumers, err := inspectClient.XInfoConsumers(context.Background(), rt.streamKey(), defaultProjectorGroup).Result()
	if err != nil {
		t.Fatalf("XInfoConsumers returned error: %v", err)
	}
	if len(consumers) != 0 {
		t.Fatalf("consumer count after Close = %d, want 0", len(consumers))
	}
}

func TestRuntimeCloseSkipsProjectorConsumerCleanupWithPending(t *testing.T) {
	mr := miniredis.RunT(t)
	runtimeClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	inspectClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = inspectClient.Close() })

	rt := &Runtime{
		client:             runtimeClient,
		cfg:                Config{Prefix: "amp"},
		projectorConsumers: []string{"worker-1"},
		stopCh:             make(chan struct{}),
	}
	if err := rt.ensureConsumerGroup(context.Background()); err != nil {
		t.Fatalf("ensureConsumerGroup returned error: %v", err)
	}
	if _, err := runtimeClient.XAdd(context.Background(), &redis.XAddArgs{
		Stream: rt.streamKey(),
		Values: map[string]any{"event_type": "reserve", "request_id": "req-1"},
	}).Result(); err != nil {
		t.Fatalf("XAdd returned error: %v", err)
	}
	if _, err := runtimeClient.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    defaultProjectorGroup,
		Consumer: "worker-1",
		Streams:  []string{rt.streamKey(), ">"},
		Count:    1,
	}).Result(); err != nil {
		t.Fatalf("XReadGroup returned error: %v", err)
	}

	rt.Close()

	consumers, err := inspectClient.XInfoConsumers(context.Background(), rt.streamKey(), defaultProjectorGroup).Result()
	if err != nil {
		t.Fatalf("XInfoConsumers returned error: %v", err)
	}
	if len(consumers) != 1 {
		t.Fatalf("consumer count after Close = %d, want 1", len(consumers))
	}
	if consumers[0].Name != "worker-1" {
		t.Fatalf("consumer name = %q, want worker-1", consumers[0].Name)
	}
	if consumers[0].Pending != 1 {
		t.Fatalf("pending count = %d, want 1", consumers[0].Pending)
	}
}
