package amp

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ampmanager/internal/config"
	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/alicebob/miniredis/v2"
)

func TestLogWriterUpdateFromTraceWritesBillingResult(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "log-writer-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})

	writer := &LogWriter{db: database.GetDB()}
	trace := NewRequestTrace("req-1", "user-1", "key-1", "POST", "/v1/responses")
	trace.StartTime = time.Date(2026, time.April, 16, 1, 2, 25, 0, time.UTC)
	trace.SetModels("gpt-4.1-mini", "gpt-4.1-mini")
	inputTokens := 12
	outputTokens := 8
	trace.SetUsage(&inputTokens, &outputTokens, nil, nil)
	trace.SetResponse(200)
	trace.SetCost(130, "0.000130", "gpt-4.1-mini", "")
	trace.SetBillingResult(&service.RequestBillingResult{
		Status:                    "overuse",
		ChargedSubscriptionMicros: 100,
		ChargedBalanceMicros:      10,
	})

	if ok := writer.WritePendingFromTrace(trace); !ok {
		t.Fatal("WritePendingFromTrace returned false")
	}
	if ok := writer.UpdateFromTrace(trace); !ok {
		t.Fatal("UpdateFromTrace returned false")
	}

	var chargedSub, chargedBal int64
	var status string
	if err := database.GetDB().QueryRow(`SELECT charged_subscription_micros, charged_balance_micros, billing_status FROM request_logs WHERE id = ?`, "req-1").Scan(&chargedSub, &chargedBal, &status); err != nil {
		t.Fatalf("query request_logs returned error: %v", err)
	}
	if chargedSub != 100 || chargedBal != 10 || status != "overuse" {
		t.Fatalf("request log billing = sub:%d bal:%d status:%s", chargedSub, chargedBal, status)
	}

	minuteBucket := trace.StartTime.UTC().Truncate(time.Minute)
	var requestCount, inputTokensSum, outputTokensSum, totalTokensSum int64
	if err := database.GetDB().QueryRow(`
		SELECT request_count, input_tokens, output_tokens, total_tokens
		FROM global_request_metric_projections
		WHERE request_id = ?
	`, "req-1").Scan(&requestCount, &inputTokensSum, &outputTokensSum, &totalTokensSum); err != nil {
		t.Fatalf("query global_request_metric_projections returned error: %v", err)
	}
	if requestCount != 1 || inputTokensSum != 12 || outputTokensSum != 8 || totalTokensSum != 20 {
		t.Fatalf("unexpected projection = req:%d input:%d output:%d total:%d", requestCount, inputTokensSum, outputTokensSum, totalTokensSum)
	}

	var projectedMinute time.Time
	if err := database.GetDB().QueryRow(`
		SELECT minute_bucket
		FROM global_request_metric_projections
		WHERE request_id = ?
	`, "req-1").Scan(&projectedMinute); err != nil {
		t.Fatalf("query projection minute returned error: %v", err)
	}
	if !projectedMinute.Equal(minuteBucket) {
		t.Fatalf("projection minute = %s, want %s", projectedMinute.Format(time.RFC3339), minuteBucket.Format(time.RFC3339))
	}

	var minuteMetricCount int64
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM global_request_minute_metrics`).Scan(&minuteMetricCount); err != nil {
		t.Fatalf("query global_request_minute_metrics count returned error: %v", err)
	}
	if minuteMetricCount != 0 {
		t.Fatalf("expected no minute metric writes, got %d rows", minuteMetricCount)
	}
}

func TestLogWriterUpdateFromTraceReplacesProjectedMinuteMetrics(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "log-writer-reproject-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})

	writer := &LogWriter{db: database.GetDB()}
	trace := NewRequestTrace("req-2", "user-1", "key-1", "POST", "/v1/responses")
	trace.StartTime = time.Date(2026, time.April, 16, 1, 2, 25, 0, time.UTC)
	trace.SetModels("gpt-4.1-mini", "gpt-4.1-mini")
	initialInput := 10
	initialOutput := 5
	trace.SetUsage(&initialInput, &initialOutput, nil, nil)
	trace.SetResponse(200)

	if ok := writer.WritePendingFromTrace(trace); !ok {
		t.Fatal("WritePendingFromTrace returned false")
	}
	if ok := writer.UpdateFromTrace(trace); !ok {
		t.Fatal("first UpdateFromTrace returned false")
	}

	trace.StartTime = trace.StartTime.Add(time.Minute)
	updatedOutput := 7
	trace.SetUsage(&initialInput, &updatedOutput, nil, nil)
	if ok := writer.UpdateFromTrace(trace); !ok {
		t.Fatal("second UpdateFromTrace returned false")
	}

	firstMinute := time.Date(2026, time.April, 16, 1, 2, 0, 0, time.UTC)
	secondMinute := firstMinute.Add(time.Minute)

	var count int64
	if err := database.GetDB().QueryRow(`
		SELECT COUNT(*)
		FROM global_request_metric_projections
		WHERE request_id = ? AND minute_bucket = ?
	`, "req-2", firstMinute).Scan(&count); err != nil {
		t.Fatalf("query first projection minute returned error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected projection to move off first minute, got count %d", count)
	}

	var requestCount, totalTokensSum int64
	if err := database.GetDB().QueryRow(`
		SELECT request_count, total_tokens
		FROM global_request_metric_projections
		WHERE request_id = ? AND minute_bucket = ?
	`, "req-2", secondMinute).Scan(&requestCount, &totalTokensSum); err != nil {
		t.Fatalf("query second projection minute returned error: %v", err)
	}
	if requestCount != 1 || totalTokensSum != 17 {
		t.Fatalf("unexpected second projection = req:%d total:%d", requestCount, totalTokensSum)
	}

	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM global_request_minute_metrics`).Scan(&count); err != nil {
		t.Fatalf("query minute metric count returned error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no minute metric writes, got count %d", count)
	}
}

func TestLoggingBodyWrapperCloseFinalizesSessionSticky(t *testing.T) {
	mr := miniredis.RunT(t)
	StopSessionStickyRuntime()
	InitSessionStickyRuntime(&config.Config{
		RedisURL:    fmt.Sprintf("redis://%s/0", mr.Addr()),
		RedisPrefix: "log-writer-test",
	})
	UpdateSessionStickyConfig(model.SessionStickyConfigResponse{
		Enabled:           true,
		WindowMinutes:     5,
		LogSearchMinChars: 4,
	})
	t.Cleanup(func() {
		StopSessionStickyRuntime()
	})

	trace := NewRequestTrace("req-sticky", "user-1", "key-1", "POST", "/v1/responses")
	trace.SetSessionID("sess-sticky")
	trace.SetChannel("channel-sticky", "openai", string(model.ChannelEndpointResponses))

	wrapper := NewLoggingBodyWrapper(io.NopCloser(strings.NewReader("ok")), trace, 200, context.Background())
	if err := wrapper.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	binding, _, ok := GetSessionStickyBinding(context.Background(), "sess-sticky")
	if !ok {
		t.Fatal("expected sticky binding to be stored")
	}
	if binding != "channel-sticky" {
		t.Fatalf("binding = %q, want channel-sticky", binding)
	}
}
