package main

import (
	"encoding/json"
	"testing"
)

func TestRuntimeKnobsMarshalJSON(t *testing.T) {
	encoded, err := json.Marshal(runtimeKnobs{
		streamBatchSize:    100,
		reconcileBatchSize: 120,
		expiryBatchSize:    140,
		projectorWorkers:   4,
		projectorClaimIdle: 45,
	})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload["streamBatchSize"] != float64(100) {
		t.Fatalf("streamBatchSize = %v, want 100", payload["streamBatchSize"])
	}
	if payload["reconcileBatchSize"] != float64(120) {
		t.Fatalf("reconcileBatchSize = %v, want 120", payload["reconcileBatchSize"])
	}
	if payload["expiryBatchSize"] != float64(140) {
		t.Fatalf("expiryBatchSize = %v, want 140", payload["expiryBatchSize"])
	}
	if payload["projectorWorkers"] != float64(4) {
		t.Fatalf("projectorWorkers = %v, want 4", payload["projectorWorkers"])
	}
	if payload["projectorClaimIdleSec"] != float64(45) {
		t.Fatalf("projectorClaimIdleSec = %v, want 45", payload["projectorClaimIdleSec"])
	}
}
