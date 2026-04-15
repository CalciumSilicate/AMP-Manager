package amp

import (
	"path/filepath"
	"testing"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/service"
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
	trace.SetCost(130, "0.000130", "gpt-4.1-mini")
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
	var requestCountSum, inputTokensSum, outputTokensSum, totalTokensSum int64
	if err := database.GetDB().QueryRow(`
		SELECT request_count_sum, input_tokens_sum, output_tokens_sum, total_tokens_sum
		FROM global_request_minute_metrics
		WHERE minute_bucket = ?
	`, minuteBucket).Scan(&requestCountSum, &inputTokensSum, &outputTokensSum, &totalTokensSum); err != nil {
		t.Fatalf("query global_request_minute_metrics returned error: %v", err)
	}
	if requestCountSum != 1 || inputTokensSum != 12 || outputTokensSum != 8 || totalTokensSum != 20 {
		t.Fatalf("unexpected minute metrics = req:%d input:%d output:%d total:%d", requestCountSum, inputTokensSum, outputTokensSum, totalTokensSum)
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
		FROM global_request_minute_metrics
		WHERE minute_bucket = ?
	`, firstMinute).Scan(&count); err != nil {
		t.Fatalf("query first minute returned error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected first minute aggregate to be removed, got count %d", count)
	}

	var requestCountSum, totalTokensSum int64
	if err := database.GetDB().QueryRow(`
		SELECT request_count_sum, total_tokens_sum
		FROM global_request_minute_metrics
		WHERE minute_bucket = ?
	`, secondMinute).Scan(&requestCountSum, &totalTokensSum); err != nil {
		t.Fatalf("query second minute returned error: %v", err)
	}
	if requestCountSum != 1 || totalTokensSum != 17 {
		t.Fatalf("unexpected second minute metrics = req:%d total:%d", requestCountSum, totalTokensSum)
	}
}
