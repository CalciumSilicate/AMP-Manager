package service

import (
	"testing"
	"time"

	"ampmanager/internal/model"
)

func TestGetWindowBoundsDailyFixedResetUsesSiteTimezone(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("time.LoadLocation returned error: %v", err)
	}

	reset := "09:30"
	limit := model.SubscriptionPlanLimit{
		LimitType:      model.LimitTypeDaily,
		WindowMode:     model.WindowModeFixed,
		FixedResetTime: &reset,
	}

	now := time.Date(2026, 4, 14, 0, 30, 0, 0, time.UTC) // 08:30 in Asia/Shanghai
	subscriptionStartsAt := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	start, end, err := GetWindowBounds(limit, now, subscriptionStartsAt, location)
	if err != nil {
		t.Fatalf("GetWindowBounds returned error: %v", err)
	}

	wantStart := time.Date(2026, 4, 13, 1, 30, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 4, 14, 1, 30, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("unexpected daily fixed bounds: start=%s end=%s wantStart=%s wantEnd=%s", start, end, wantStart, wantEnd)
	}
}

func TestGetWindowBoundsWeeklyAndMonthlyFixedUseSiteTimezoneMidnight(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("time.LoadLocation returned error: %v", err)
	}

	now := time.Date(2026, 4, 15, 4, 0, 0, 0, time.UTC) // 12:00 in Asia/Shanghai
	subscriptionStartsAt := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	weeklyStart, weeklyEnd, err := GetWindowBounds(model.SubscriptionPlanLimit{
		LimitType:  model.LimitTypeWeekly,
		WindowMode: model.WindowModeFixed,
	}, now, subscriptionStartsAt, location)
	if err != nil {
		t.Fatalf("GetWindowBounds weekly returned error: %v", err)
	}

	wantWeeklyStart := time.Date(2026, 4, 12, 16, 0, 0, 0, time.UTC) // Monday 00:00 Asia/Shanghai
	wantWeeklyEnd := time.Date(2026, 4, 19, 16, 0, 0, 0, time.UTC)
	if !weeklyStart.Equal(wantWeeklyStart) || !weeklyEnd.Equal(wantWeeklyEnd) {
		t.Fatalf("unexpected weekly bounds: start=%s end=%s wantStart=%s wantEnd=%s", weeklyStart, weeklyEnd, wantWeeklyStart, wantWeeklyEnd)
	}

	monthlyStart, monthlyEnd, err := GetWindowBounds(model.SubscriptionPlanLimit{
		LimitType:  model.LimitTypeMonthly,
		WindowMode: model.WindowModeFixed,
	}, now, subscriptionStartsAt, location)
	if err != nil {
		t.Fatalf("GetWindowBounds monthly returned error: %v", err)
	}

	wantMonthlyStart := time.Date(2026, 3, 31, 16, 0, 0, 0, time.UTC) // April 1st 00:00 Asia/Shanghai
	wantMonthlyEnd := time.Date(2026, 4, 30, 16, 0, 0, 0, time.UTC)
	if !monthlyStart.Equal(wantMonthlyStart) || !monthlyEnd.Equal(wantMonthlyEnd) {
		t.Fatalf("unexpected monthly bounds: start=%s end=%s wantStart=%s wantEnd=%s", monthlyStart, monthlyEnd, wantMonthlyStart, wantMonthlyEnd)
	}
}
