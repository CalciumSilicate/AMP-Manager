package service

import (
	"context"
	"testing"

	"ampmanager/internal/model"
)

func TestRedeemServiceRedeemSubscriptionAndBalanceRefreshesRuntimeOnly(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	user := createPurchaseTestUser(t, "redeem-user")
	plan := createPurchaseTestPlan(t, "Redeem Plan")

	svc := NewRedeemService()
	svc.grantSvc = NewRewardGrantService()

	var refreshCalls []string
	var balanceCalls []int64
	svc.grantSvc.refreshUserState = func(_ context.Context, userID string) error {
		refreshCalls = append(refreshCalls, userID)
		return nil
	}
	svc.grantSvc.applyBalanceDelta = func(_ context.Context, userID string, deltaMicros int64) error {
		if userID != user.ID {
			t.Fatalf("unexpected user id for balance delta: %s", userID)
		}
		balanceCalls = append(balanceCalls, deltaMicros)
		return nil
	}

	campaign, err := svc.CreateCampaign(&model.RedeemCampaignRequest{
		Name:                     "订阅加余额活动",
		Description:              "test",
		CodeMode:                 model.RedeemCodeModeSingleUse,
		SubscriptionPlanID:       plan.ID,
		SubscriptionDurationDays: 30,
		BalanceMicros:            1_500_000,
		PerUserLimit:             1,
		Enabled:                  true,
	})
	if err != nil {
		t.Fatalf("CreateCampaign returned error: %v", err)
	}

	_, codes, err := svc.CreateBatch(campaign.ID, &model.RedeemCodeBatchRequest{
		Name:       "首批",
		CodeCount:  1,
		CodeLength: 8,
	})
	if err != nil {
		t.Fatalf("CreateBatch returned error: %v", err)
	}
	if len(codes) != 1 {
		t.Fatalf("expected one redeem code, got %d", len(codes))
	}

	result, err := svc.Redeem(context.Background(), user.ID, user.Username, codes[0])
	if err != nil {
		t.Fatalf("Redeem returned error: %v", err)
	}
	if result == nil || result.Redemption == nil {
		t.Fatal("expected redeem result")
	}

	if len(refreshCalls) != 1 || refreshCalls[0] != user.ID {
		t.Fatalf("unexpected refresh calls: %+v", refreshCalls)
	}
	if len(balanceCalls) != 0 {
		t.Fatalf("expected no balance delta calls, got %+v", balanceCalls)
	}
}
