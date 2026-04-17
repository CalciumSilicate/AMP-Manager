package billingstate

import (
	"testing"
	"time"

	"ampmanager/internal/model"
)

func TestGetWindowBoundsHonorsSiteTimeZoneAndFixedResetTime(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("time.LoadLocation returned error: %v", err)
	}

	resetTime := "09:30"
	now := time.Date(2026, 4, 17, 0, 30, 0, 0, time.UTC) // 08:30 local, before reset time
	start, end, err := getWindowBounds(
		model.LimitTypeDaily,
		model.WindowModeFixed,
		&resetTime,
		now,
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		location,
	)
	if err != nil {
		t.Fatalf("getWindowBounds returned error: %v", err)
	}

	wantStart := time.Date(2026, 4, 16, 1, 30, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 4, 17, 1, 30, 0, 0, time.UTC)
	if !start.Equal(wantStart) {
		t.Fatalf("start = %v, want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Fatalf("end = %v, want %v", end, wantEnd)
	}
}
