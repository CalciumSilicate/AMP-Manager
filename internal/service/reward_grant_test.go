package service

import (
	"context"
	"errors"
	"testing"
)

func TestRewardGrantServiceSyncBillingStatePrefersRefreshUserState(t *testing.T) {
	svc := NewRewardGrantService()

	var refreshCalls []string
	var balanceCalls []struct {
		userID string
		delta  int64
	}
	svc.refreshUserState = func(_ context.Context, userID string) error {
		refreshCalls = append(refreshCalls, userID)
		return nil
	}
	svc.applyBalanceDelta = func(_ context.Context, userID string, deltaMicros int64) error {
		balanceCalls = append(balanceCalls, struct {
			userID string
			delta  int64
		}{userID: userID, delta: deltaMicros})
		return nil
	}

	err := svc.SyncBillingState(context.Background(), BillingStateSyncAction{
		UserID:             "user-refresh",
		RefreshUserState:   true,
		BalanceDeltaMicros: 123,
	})
	if err != nil {
		t.Fatalf("SyncBillingState returned error: %v", err)
	}
	if len(refreshCalls) != 1 || refreshCalls[0] != "user-refresh" {
		t.Fatalf("unexpected refresh calls: %+v", refreshCalls)
	}
	if len(balanceCalls) != 0 {
		t.Fatalf("expected no balance delta calls, got %+v", balanceCalls)
	}
}

func TestRewardGrantServiceSyncBillingStateAppliesBalanceDelta(t *testing.T) {
	svc := NewRewardGrantService()

	var gotUserID string
	var gotDelta int64
	svc.refreshUserState = func(_ context.Context, userID string) error {
		t.Fatalf("unexpected refresh call for user %s", userID)
		return nil
	}
	svc.applyBalanceDelta = func(_ context.Context, userID string, deltaMicros int64) error {
		gotUserID = userID
		gotDelta = deltaMicros
		return nil
	}

	err := svc.SyncBillingState(context.Background(), BillingStateSyncAction{
		UserID:             "user-balance",
		BalanceDeltaMicros: 456,
	})
	if err != nil {
		t.Fatalf("SyncBillingState returned error: %v", err)
	}
	if gotUserID != "user-balance" || gotDelta != 456 {
		t.Fatalf("unexpected balance delta call: user=%s delta=%d", gotUserID, gotDelta)
	}
}

func TestRewardGrantServiceSyncBillingStateReturnsHookError(t *testing.T) {
	svc := NewRewardGrantService()
	wantErr := errors.New("refresh failed")
	svc.refreshUserState = func(_ context.Context, userID string) error {
		if userID != "user-error" {
			t.Fatalf("unexpected user id: %s", userID)
		}
		return wantErr
	}

	err := svc.SyncBillingState(context.Background(), BillingStateSyncAction{
		UserID:           "user-error",
		RefreshUserState: true,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}
