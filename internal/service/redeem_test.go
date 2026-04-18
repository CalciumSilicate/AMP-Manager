package service

import (
	"context"
	"database/sql"
	"testing"

	"ampmanager/internal/database"
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

func TestRedeemServiceCampaignBalanceOnlyKeepsSubscriptionPlanNullable(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	user := createPurchaseTestUser(t, "redeem-balance-only")
	svc := NewRedeemService()

	campaign, err := svc.CreateCampaign(&model.RedeemCampaignRequest{
		Name:                  "纯余额活动",
		Description:           "balance only",
		CodeMode:              model.RedeemCodeModeSingleUse,
		BalanceMicros:         2_500_000,
		TotalRedemptionsLimit: 10,
		PerUserLimit:          1,
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("CreateCampaign returned error: %v", err)
	}
	if campaign.SubscriptionPlanID != "" {
		t.Fatalf("expected empty subscription plan id in campaign response, got %q", campaign.SubscriptionPlanID)
	}

	var storedCampaignPlanID sql.NullString
	if err := database.GetDB().QueryRow(`SELECT subscription_plan_id FROM redeem_campaigns WHERE id = ?`, campaign.ID).Scan(&storedCampaignPlanID); err != nil {
		t.Fatalf("query campaign subscription plan returned error: %v", err)
	}
	if storedCampaignPlanID.Valid {
		t.Fatalf("expected nullable subscription_plan_id for campaign, got %q", storedCampaignPlanID.String)
	}

	updatedCampaign, err := svc.UpdateCampaign(campaign.ID, &model.RedeemCampaignRequest{
		Name:                  "纯余额活动-更新",
		Description:           "balance only updated",
		CodeMode:              model.RedeemCodeModeSingleUse,
		BalanceMicros:         3_000_000,
		TotalRedemptionsLimit: 20,
		PerUserLimit:          2,
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("UpdateCampaign returned error: %v", err)
	}
	if updatedCampaign.SubscriptionPlanID != "" {
		t.Fatalf("expected empty subscription plan id in updated campaign response, got %q", updatedCampaign.SubscriptionPlanID)
	}

	codes, generated, err := svc.CreateBatch(campaign.ID, &model.RedeemCodeBatchRequest{
		Name:       "余额批次",
		CodeCount:  1,
		CodeLength: 8,
	})
	if err != nil {
		t.Fatalf("CreateBatch returned error: %v", err)
	}
	if codes == nil || len(generated) != 1 {
		t.Fatalf("expected one generated code, got batch=%v codes=%d", codes != nil, len(generated))
	}

	codeList, err := svc.ListCodes(campaign.ID, "", "", generated[0], 10)
	if err != nil {
		t.Fatalf("ListCodes returned error: %v", err)
	}
	if len(codeList) != 1 {
		t.Fatalf("expected one listed code, got %d", len(codeList))
	}
	if codeList[0].SubscriptionPlanID != "" {
		t.Fatalf("expected empty subscription plan id in listed code, got %q", codeList[0].SubscriptionPlanID)
	}

	result, err := svc.Redeem(context.Background(), user.ID, user.Username, generated[0])
	if err != nil {
		t.Fatalf("Redeem returned error: %v", err)
	}
	if result.Redemption == nil {
		t.Fatal("expected redemption result")
	}
	if result.Redemption.SubscriptionPlanID != "" {
		t.Fatalf("expected empty subscription plan id in redeem result, got %q", result.Redemption.SubscriptionPlanID)
	}

	var storedRedemptionPlanID sql.NullString
	if err := database.GetDB().QueryRow(`SELECT subscription_plan_id FROM redeem_redemptions WHERE id = ?`, result.Redemption.ID).Scan(&storedRedemptionPlanID); err != nil {
		t.Fatalf("query redemption subscription plan returned error: %v", err)
	}
	if storedRedemptionPlanID.Valid {
		t.Fatalf("expected nullable subscription_plan_id for redemption, got %q", storedRedemptionPlanID.String)
	}

	redemptions, err := svc.ListRedemptions(campaign.ID, "", user.Username, 10)
	if err != nil {
		t.Fatalf("ListRedemptions returned error: %v", err)
	}
	if len(redemptions) != 1 {
		t.Fatalf("expected one redemption entry, got %d", len(redemptions))
	}
	if redemptions[0].SubscriptionPlanID != "" {
		t.Fatalf("expected empty subscription plan id in listed redemption, got %q", redemptions[0].SubscriptionPlanID)
	}
}

func TestRedeemServiceManualBalanceOnlyCodeKeepsSubscriptionPlanNullable(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	user := createPurchaseTestUser(t, "redeem-free-balance")
	svc := NewRedeemService()

	code, err := svc.CreateManualCode(&model.ManualRedeemCodeRequest{
		CodeValue:      "FREE-BALANCE-001",
		BalanceMicros:  1_250_000,
		PerUserLimit:   1,
		MaxRedemptions: 1,
		Enabled:        true,
	})
	if err != nil {
		t.Fatalf("CreateManualCode returned error: %v", err)
	}
	if code.SubscriptionPlanID != "" {
		t.Fatalf("expected empty subscription plan id in manual code response, got %q", code.SubscriptionPlanID)
	}

	var storedCodePlanID sql.NullString
	if err := database.GetDB().QueryRow(`SELECT subscription_plan_id FROM redeem_codes WHERE id = ?`, code.ID).Scan(&storedCodePlanID); err != nil {
		t.Fatalf("query code subscription plan returned error: %v", err)
	}
	if storedCodePlanID.Valid {
		t.Fatalf("expected nullable subscription_plan_id for manual code, got %q", storedCodePlanID.String)
	}

	result, err := svc.Redeem(context.Background(), user.ID, user.Username, code.CodeValue)
	if err != nil {
		t.Fatalf("Redeem returned error: %v", err)
	}
	if result.Redemption == nil {
		t.Fatal("expected redemption result")
	}
	if result.Redemption.SubscriptionPlanID != "" {
		t.Fatalf("expected empty subscription plan id in manual code redemption, got %q", result.Redemption.SubscriptionPlanID)
	}

	var storedRedemptionPlanID sql.NullString
	if err := database.GetDB().QueryRow(`SELECT subscription_plan_id FROM redeem_redemptions WHERE id = ?`, result.Redemption.ID).Scan(&storedRedemptionPlanID); err != nil {
		t.Fatalf("query manual redemption subscription plan returned error: %v", err)
	}
	if storedRedemptionPlanID.Valid {
		t.Fatalf("expected nullable subscription_plan_id for manual redemption, got %q", storedRedemptionPlanID.String)
	}

	codeList, err := svc.ListCodes("", "", "", code.CodeValue, 10)
	if err != nil {
		t.Fatalf("ListCodes returned error: %v", err)
	}
	if len(codeList) != 1 {
		t.Fatalf("expected one listed manual code, got %d", len(codeList))
	}
	if codeList[0].SubscriptionPlanID != "" {
		t.Fatalf("expected empty subscription plan id in listed manual code, got %q", codeList[0].SubscriptionPlanID)
	}

	userRedemptions, err := svc.ListUserRedemptions(user.ID, 10)
	if err != nil {
		t.Fatalf("ListUserRedemptions returned error: %v", err)
	}
	if len(userRedemptions) != 1 {
		t.Fatalf("expected one user redemption entry, got %d", len(userRedemptions))
	}
	if userRedemptions[0].SubscriptionPlanID != "" {
		t.Fatalf("expected empty subscription plan id in user redemption list, got %q", userRedemptions[0].SubscriptionPlanID)
	}
}
