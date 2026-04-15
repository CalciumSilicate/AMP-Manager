package amp

import (
	"path/filepath"
	"testing"

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
	trace.SetModels("gpt-4.1-mini", "gpt-4.1-mini")
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
}
