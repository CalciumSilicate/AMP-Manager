package handler

import (
	"testing"
	"time"
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
