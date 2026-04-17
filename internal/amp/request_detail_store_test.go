package amp

import (
	"net/http"
	"testing"
	"time"
)

func resetRequestDetailRPMCounterForTest() {
	requestDetailRPMState = newRequestDetailRPMCounter()
}

func TestShouldCaptureRequestDetailHighRPMOffMode(t *testing.T) {
	prevCfg := GetRequestDetailConfig()
	prevRPMState := requestDetailRPMState
	t.Cleanup(func() {
		requestDetailRPMState = prevRPMState
		UpdateRequestDetailConfig(prevCfg)
	})

	resetRequestDetailRPMCounterForTest()
	UpdateRequestDetailConfig(RequestDetailConfig{
		Enabled:              true,
		TTL:                  10 * time.Minute,
		MaxEntries:           10,
		MaxMemoryBytes:       64 * 1024 * 1024,
		BodyCapBytes:         64,
		PersistEnabled:       false,
		HighRPMMode:          RequestDetailModeOff,
		HighRPMThreshold:     1,
		HighRPMSamplePercent: 100,
	})

	if ShouldCaptureRequestDetail("req-off") {
		t.Fatalf("expected capture to be disabled once RPM threshold is reached in off mode")
	}
}

func TestShouldCaptureRequestDetailHighRPMSampleMode(t *testing.T) {
	prevCfg := GetRequestDetailConfig()
	prevRPMState := requestDetailRPMState
	t.Cleanup(func() {
		requestDetailRPMState = prevRPMState
		UpdateRequestDetailConfig(prevCfg)
	})

	resetRequestDetailRPMCounterForTest()
	UpdateRequestDetailConfig(RequestDetailConfig{
		Enabled:              true,
		TTL:                  10 * time.Minute,
		MaxEntries:           10,
		MaxMemoryBytes:       64 * 1024 * 1024,
		BodyCapBytes:         64,
		PersistEnabled:       false,
		HighRPMMode:          RequestDetailModeSample,
		HighRPMThreshold:     1,
		HighRPMSamplePercent: 100,
	})

	if !ShouldCaptureRequestDetail("req-sample") {
		t.Fatalf("expected sample mode to retain request detail when sample percent is 100")
	}
}

func TestRequestDetailRPMCounterRollsWindowForward(t *testing.T) {
	counter := newRequestDetailRPMCounter()
	start := time.Unix(1000, 0)

	for i := 0; i < 60; i++ {
		if got, want := counter.Record(start.Add(time.Duration(i)*time.Second)), i+1; got != want {
			t.Fatalf("counter at second %d = %d, want %d", i, got, want)
		}
	}

	if got, want := counter.Record(start.Add(60*time.Second)), 60; got != want {
		t.Fatalf("counter after rolling window = %d, want %d", got, want)
	}
}

func TestRequestDetailStoreEnforcesMaxEntries(t *testing.T) {
	prevCfg := GetRequestDetailConfig()
	t.Cleanup(func() {
		UpdateRequestDetailConfig(prevCfg)
	})

	UpdateRequestDetailConfig(RequestDetailConfig{
		Enabled:        true,
		TTL:            10 * time.Minute,
		MaxEntries:     2,
		MaxMemoryBytes: 64 * 1024 * 1024,
		BodyCapBytes:   64,
		PersistEnabled: false,
	})

	store := NewRequestDetailStore(nil, 0)
	t.Cleanup(store.Stop)

	store.UpdateRequestData("req-1", http.Header{"X-Test": []string{"1"}}, []byte("one"))
	time.Sleep(5 * time.Millisecond)
	store.UpdateRequestData("req-2", http.Header{"X-Test": []string{"2"}}, []byte("two"))
	time.Sleep(5 * time.Millisecond)
	store.UpdateRequestData("req-3", http.Header{"X-Test": []string{"3"}}, []byte("three"))

	store.mu.RLock()
	defer store.mu.RUnlock()

	if got := len(store.details); got != 2 {
		t.Fatalf("expected 2 details after eviction, got %d", got)
	}
	if _, exists := store.details["req-1"]; exists {
		t.Fatalf("expected oldest request to be evicted")
	}
}

func TestRequestDetailStoreEnforcesMaxMemory(t *testing.T) {
	prevCfg := GetRequestDetailConfig()
	t.Cleanup(func() {
		UpdateRequestDetailConfig(prevCfg)
	})

	UpdateRequestDetailConfig(RequestDetailConfig{
		Enabled:        true,
		TTL:            10 * time.Minute,
		MaxEntries:     10,
		MaxMemoryBytes: 360,
		BodyCapBytes:   64,
		PersistEnabled: false,
	})

	store := NewRequestDetailStore(nil, 0)
	t.Cleanup(store.Stop)

	store.UpdateRequestData("req-a", nil, []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	time.Sleep(5 * time.Millisecond)
	store.UpdateRequestData("req-b", nil, []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))

	store.mu.RLock()
	defer store.mu.RUnlock()

	if got := len(store.details); got != 1 {
		t.Fatalf("expected memory budget to keep only one detail, got %d", got)
	}
	if _, exists := store.details["req-a"]; exists {
		t.Fatalf("expected oldest request to be evicted by memory budget")
	}
	if store.currentBytes > GetRequestDetailConfig().MaxMemoryBytes {
		t.Fatalf("expected currentBytes <= budget, got %d > %d", store.currentBytes, GetRequestDetailConfig().MaxMemoryBytes)
	}
}

func TestRequestDetailStoreApplyConfigDisabledClearsMemory(t *testing.T) {
	prevCfg := GetRequestDetailConfig()
	t.Cleanup(func() {
		UpdateRequestDetailConfig(prevCfg)
	})

	UpdateRequestDetailConfig(RequestDetailConfig{
		Enabled:        true,
		TTL:            10 * time.Minute,
		MaxEntries:     10,
		MaxMemoryBytes: 64 * 1024 * 1024,
		BodyCapBytes:   64,
		PersistEnabled: false,
	})

	store := NewRequestDetailStore(nil, 0)
	t.Cleanup(store.Stop)

	store.UpdateRequestData("req-1", nil, []byte("payload"))

	store.ApplyConfig(RequestDetailConfig{
		Enabled:        false,
		TTL:            10 * time.Minute,
		MaxEntries:     10,
		MaxMemoryBytes: 64 * 1024 * 1024,
		BodyCapBytes:   64,
		PersistEnabled: false,
	})

	store.mu.RLock()
	defer store.mu.RUnlock()

	if len(store.details) != 0 {
		t.Fatalf("expected all in-memory details to be cleared when disabled")
	}
	if store.currentBytes != 0 {
		t.Fatalf("expected currentBytes to reset, got %d", store.currentBytes)
	}
}

func TestRequestDetailStoreResponseUpdateDoesNotCreateMissingDetail(t *testing.T) {
	prevCfg := GetRequestDetailConfig()
	t.Cleanup(func() {
		UpdateRequestDetailConfig(prevCfg)
	})

	UpdateRequestDetailConfig(RequestDetailConfig{
		Enabled:              true,
		TTL:                  10 * time.Minute,
		MaxEntries:           10,
		MaxMemoryBytes:       64 * 1024 * 1024,
		BodyCapBytes:         64,
		PersistEnabled:       false,
		HighRPMMode:          RequestDetailModeFull,
		HighRPMThreshold:     DefaultHighRPMThreshold,
		HighRPMSamplePercent: DefaultHighRPMSamplePercent,
	})

	store := NewRequestDetailStore(nil, 0)
	t.Cleanup(store.Stop)

	store.UpdateResponseData("req-missing", http.Header{"X-Test": []string{"1"}}, []byte("body"))
	store.UpdateTranslatedRequestBody("req-missing", []byte("translated"))
	store.UpdateTranslatedRequestHeaders("req-missing", http.Header{"X-Trace": []string{"1"}})
	store.UpdateTranslatedResponseBody("req-missing", []byte("translated-response"))

	store.mu.RLock()
	defer store.mu.RUnlock()

	if len(store.details) != 0 {
		t.Fatalf("expected response-only updates to skip creating missing request detail entries")
	}
}
