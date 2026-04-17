package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
)

type fakePaymentGateway struct{}

func (fakePaymentGateway) CreateOrder(_ context.Context, _ *model.PurchaseOrder, _ *model.PurchaseProductResponse, _ string) (*PaymentCreateResult, error) {
	return nil, errors.New("should not be called")
}

func (fakePaymentGateway) QueryOrder(_ context.Context, _ string) (*PaymentQueryResult, error) {
	return nil, errors.New("should not be called")
}

func (fakePaymentGateway) DecodeNotification(_ context.Context, _ url.Values) (*PaymentNotification, error) {
	return nil, errors.New("should not be called")
}

func setupPurchaseServiceTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "purchase-service-test.sqlite")
	if err := database.Init(dbPath); err != nil {
		t.Fatalf("database.Init returned error: %v", err)
	}

	t.Cleanup(func() {
		if err := database.CloseAndRelease(); err != nil {
			t.Fatalf("database.CloseAndRelease returned error: %v", err)
		}
	})
}

func createPurchaseTestUser(t *testing.T, username string) *model.User {
	t.Helper()

	user := &model.User{
		Username:     username,
		PasswordHash: "hash",
	}
	if err := repository.NewUserRepository().Create(user); err != nil {
		t.Fatalf("create user returned error: %v", err)
	}
	return user
}

func createPurchaseTestPlan(t *testing.T, name string) *model.SubscriptionPlanResponse {
	t.Helper()

	plan, err := NewSubscriptionPlanService().Create(&model.SubscriptionPlanRequest{
		Name:        name,
		Description: name,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("Create plan returned error: %v", err)
	}
	return plan
}

func createPurchaseTestPlanWithUpgrade(t *testing.T, name string, rank int, valuationPerDay int64) *model.SubscriptionPlanResponse {
	t.Helper()

	plan, err := NewSubscriptionPlanService().Create(&model.SubscriptionPlanRequest{
		Name:                          name,
		Description:                   name,
		Enabled:                       true,
		UpgradeRank:                   rank,
		UpgradeValuationCnyCentPerDay: valuationPerDay,
	})
	if err != nil {
		t.Fatalf("Create plan returned error: %v", err)
	}
	return plan
}

func createPurchaseTestProduct(t *testing.T, planID, name string, days int, cents int64) *model.PurchaseProduct {
	t.Helper()

	product := &model.PurchaseProduct{
		Name:               name,
		Summary:            "test",
		SubscriptionPlanID: planID,
		DurationDays:       days,
		PriceCNYCent:       cents,
		IsRecommended:      false,
		SortOrder:          0,
		Enabled:            true,
	}
	if err := repository.NewPurchaseProductRepository().Create(product); err != nil {
		t.Fatalf("Create product returned error: %v", err)
	}
	return product
}

func createPendingPurchaseTestOrder(t *testing.T, userID string, product *model.PurchaseProduct, kind model.PurchaseOrderKind, balanceTopupMicros int64) *model.PurchaseOrder {
	t.Helper()

	now := time.Now().UTC()
	order := &model.PurchaseOrder{
		ID:                 uuid.New().String(),
		OrderNo:            "PO-" + uuid.New().String(),
		UserID:             userID,
		ProductID:          product.ID,
		SubscriptionPlanID: product.SubscriptionPlanID,
		DurationDays:       product.DurationDays,
		AmountCNYCent:      product.PriceCNYCent,
		OrderKind:          kind,
		DeliveryMode:       model.PurchaseDeliveryModeAccount,
		BalanceTopupMicros: balanceTopupMicros,
		PaymentChannel:     model.PaymentChannelAlipay,
		PaymentStatus:      model.PurchasePaymentStatusPending,
		FulfillmentStatus:  model.PurchaseFulfillmentStatusPending,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := repository.NewPurchaseOrderRepository().Create(order); err != nil {
		t.Fatalf("Create order returned error: %v", err)
	}
	return order
}

func enableDebugPurchase(t *testing.T, settingsSvc *PurchaseSettingsService) {
	t.Helper()

	if _, err := settingsSvc.Update(&model.PurchaseSettingsRequest{
		PurchaseEnabled:   true,
		DebugAutoPaid:     true,
		AlipayEnvironment: model.AlipayEnvironmentSandbox,
	}); err != nil {
		t.Fatalf("Update settings returned error: %v", err)
	}
}

func TestPurchaseServiceCreateOrderDebugAutoPaidCreatesSubscription(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	settingsSvc := NewPurchaseSettingsService()
	enableDebugPurchase(t, settingsSvc)

	user := createPurchaseTestUser(t, "buyer-debug")
	plan := createPurchaseTestPlan(t, "Debug Plan")
	product := createPurchaseTestProduct(t, plan.ID, "月付", 30, 990)

	svc := NewPurchaseServiceWithDeps(
		repository.NewPurchaseProductRepository(),
		repository.NewPurchaseOrderRepository(),
		repository.NewSubscriptionPlanRepository(),
		repository.NewUserSubscriptionRepository(),
		settingsSvc,
		fakePaymentGateway{},
	)

	order, err := svc.CreateOrder(context.Background(), user.ID, user.Username, product.ID, model.PurchaseDeliveryModeAccount)
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if order.PaymentStatus != model.PurchasePaymentStatusPaid {
		t.Fatalf("unexpected payment status: %s", order.PaymentStatus)
	}
	if order.FulfillmentStatus != model.PurchaseFulfillmentStatusFulfilled {
		t.Fatalf("unexpected fulfillment status: %s", order.FulfillmentStatus)
	}

	sub, err := repository.NewUserSubscriptionRepository().GetActiveByUserID(user.ID)
	if err != nil {
		t.Fatalf("GetActiveByUserID returned error: %v", err)
	}
	if sub == nil {
		t.Fatal("expected active subscription")
	}
	if sub.PlanID != plan.ID {
		t.Fatalf("unexpected plan id: got %s want %s", sub.PlanID, plan.ID)
	}
	if sub.ExpiresAt == nil {
		t.Fatal("expected expires_at to be set")
	}
}

func TestPurchaseServiceCreateOrderExtendsSamePlan(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	settingsSvc := NewPurchaseSettingsService()
	enableDebugPurchase(t, settingsSvc)

	user := createPurchaseTestUser(t, "buyer-renew")
	plan := createPurchaseTestPlan(t, "Renew Plan")
	product := createPurchaseTestProduct(t, plan.ID, "续费 30 天", 30, 1990)

	initialExpiry := time.Now().UTC().AddDate(0, 0, 10).Truncate(time.Second)
	if err := repository.NewUserSubscriptionRepository().Assign(&model.UserSubscription{
		UserID:    user.ID,
		PlanID:    plan.ID,
		StartsAt:  time.Now().UTC().AddDate(0, 0, -20),
		ExpiresAt: &initialExpiry,
		Status:    model.SubscriptionStatusActive,
	}); err != nil {
		t.Fatalf("Assign subscription returned error: %v", err)
	}

	svc := NewPurchaseServiceWithDeps(
		repository.NewPurchaseProductRepository(),
		repository.NewPurchaseOrderRepository(),
		repository.NewSubscriptionPlanRepository(),
		repository.NewUserSubscriptionRepository(),
		settingsSvc,
		fakePaymentGateway{},
	)

	if _, err := svc.CreateOrder(context.Background(), user.ID, user.Username, product.ID, model.PurchaseDeliveryModeAccount); err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	sub, err := repository.NewUserSubscriptionRepository().GetActiveByUserID(user.ID)
	if err != nil {
		t.Fatalf("GetActiveByUserID returned error: %v", err)
	}
	if sub == nil || sub.ExpiresAt == nil {
		t.Fatal("expected active subscription with expiry")
	}

	wantExpiry := initialExpiry.AddDate(0, 0, 30)
	if !sub.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("unexpected expiry: got %s want %s", sub.ExpiresAt.UTC(), wantExpiry.UTC())
	}
}

func TestPurchaseServiceRejectsSameRankDifferentPlan(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	settingsSvc := NewPurchaseSettingsService()
	enableDebugPurchase(t, settingsSvc)

	user := createPurchaseTestUser(t, "buyer-conflict")
	activePlan := createPurchaseTestPlan(t, "Active Plan")
	targetPlan := createPurchaseTestPlan(t, "Target Plan")
	product := createPurchaseTestProduct(t, targetPlan.ID, "目标商品", 30, 1990)

	activeExpiry := time.Now().UTC().AddDate(0, 0, 5)
	if err := repository.NewUserSubscriptionRepository().Assign(&model.UserSubscription{
		UserID:    user.ID,
		PlanID:    activePlan.ID,
		StartsAt:  time.Now().UTC().AddDate(0, 0, -25),
		ExpiresAt: &activeExpiry,
		Status:    model.SubscriptionStatusActive,
	}); err != nil {
		t.Fatalf("Assign subscription returned error: %v", err)
	}

	svc := NewPurchaseServiceWithDeps(
		repository.NewPurchaseProductRepository(),
		repository.NewPurchaseOrderRepository(),
		repository.NewSubscriptionPlanRepository(),
		repository.NewUserSubscriptionRepository(),
		settingsSvc,
		fakePaymentGateway{},
	)

	_, err := svc.CreateOrder(context.Background(), user.ID, user.Username, product.ID, model.PurchaseDeliveryModeAccount)
	if !errors.Is(err, ErrSameRankPlanSwitch) {
		t.Fatalf("expected ErrSameRankPlanSwitch, got %v", err)
	}
}

func TestPurchaseServiceApplySuccessfulPaymentRefreshesRuntimeAfterSubscriptionGrant(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	settingsSvc := NewPurchaseSettingsService()
	enableDebugPurchase(t, settingsSvc)

	user := createPurchaseTestUser(t, "buyer-refresh")
	plan := createPurchaseTestPlan(t, "Refresh Plan")
	product := createPurchaseTestProduct(t, plan.ID, "订阅商品", 30, 990)
	order := createPendingPurchaseTestOrder(t, user.ID, product, model.PurchaseOrderKindSubscription, 0)

	svc := NewPurchaseServiceWithDeps(
		repository.NewPurchaseProductRepository(),
		repository.NewPurchaseOrderRepository(),
		repository.NewSubscriptionPlanRepository(),
		repository.NewUserSubscriptionRepository(),
		settingsSvc,
		fakePaymentGateway{},
	)
	svc.grantSvc = NewRewardGrantService()

	var refreshCalls []string
	svc.grantSvc.refreshUserState = func(_ context.Context, userID string) error {
		refreshCalls = append(refreshCalls, userID)
		return nil
	}

	resp, err := svc.applySuccessfulPayment(order.OrderNo, "trade-refresh", nil)
	if err != nil {
		t.Fatalf("applySuccessfulPayment returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected purchase order response")
	}
	if len(refreshCalls) != 1 || refreshCalls[0] != user.ID {
		t.Fatalf("unexpected refresh calls: %+v", refreshCalls)
	}
}

func TestPurchaseServiceApplySuccessfulPaymentReturnsErrorWhenRefreshFailsAfterCommit(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	settingsSvc := NewPurchaseSettingsService()
	enableDebugPurchase(t, settingsSvc)

	user := createPurchaseTestUser(t, "buyer-refresh-fail")
	plan := createPurchaseTestPlan(t, "Fail Refresh Plan")
	product := createPurchaseTestProduct(t, plan.ID, "订阅商品", 30, 990)
	order := createPendingPurchaseTestOrder(t, user.ID, product, model.PurchaseOrderKindSubscription, 0)

	svc := NewPurchaseServiceWithDeps(
		repository.NewPurchaseProductRepository(),
		repository.NewPurchaseOrderRepository(),
		repository.NewSubscriptionPlanRepository(),
		repository.NewUserSubscriptionRepository(),
		settingsSvc,
		fakePaymentGateway{},
	)
	svc.grantSvc = NewRewardGrantService()

	wantErr := errors.New("refresh failed after commit")
	svc.grantSvc.refreshUserState = func(_ context.Context, userID string) error {
		if userID != user.ID {
			t.Fatalf("unexpected user id: %s", userID)
		}
		return wantErr
	}

	_, err := svc.applySuccessfulPayment(order.OrderNo, "trade-refresh-fail", nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}

	sub, getErr := repository.NewUserSubscriptionRepository().GetActiveByUserID(user.ID)
	if getErr != nil {
		t.Fatalf("GetActiveByUserID returned error: %v", getErr)
	}
	if sub == nil {
		t.Fatal("expected subscription to be committed before refresh failure")
	}
}

func TestPurchaseServiceApplySuccessfulPaymentBalanceDeltaOnlyOnce(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	settingsSvc := NewPurchaseSettingsService()
	enableDebugPurchase(t, settingsSvc)

	user := createPurchaseTestUser(t, "buyer-balance-once")
	plan := createPurchaseTestPlan(t, "Balance Topup Plan")
	product := createPurchaseTestProduct(t, plan.ID, "余额充值商品", 1, 500)
	order := createPendingPurchaseTestOrder(t, user.ID, product, model.PurchaseOrderKindBalanceTopup, 2_000_000)

	svc := NewPurchaseServiceWithDeps(
		repository.NewPurchaseProductRepository(),
		repository.NewPurchaseOrderRepository(),
		repository.NewSubscriptionPlanRepository(),
		repository.NewUserSubscriptionRepository(),
		settingsSvc,
		fakePaymentGateway{},
	)
	svc.grantSvc = NewRewardGrantService()

	var deltaCalls []int64
	svc.grantSvc.applyBalanceDelta = func(_ context.Context, userID string, deltaMicros int64) error {
		if userID != user.ID {
			t.Fatalf("unexpected user id: %s", userID)
		}
		deltaCalls = append(deltaCalls, deltaMicros)
		return nil
	}

	if _, err := svc.applySuccessfulPayment(order.OrderNo, "trade-balance-1", nil); err != nil {
		t.Fatalf("first applySuccessfulPayment returned error: %v", err)
	}
	if _, err := svc.applySuccessfulPayment(order.OrderNo, "trade-balance-2", nil); err != nil {
		t.Fatalf("second applySuccessfulPayment returned error: %v", err)
	}

	if len(deltaCalls) != 1 || deltaCalls[0] != 2_000_000 {
		t.Fatalf("unexpected balance delta calls: %+v", deltaCalls)
	}

	balanceMicros, err := repository.NewUserRepository().GetBalance(user.ID)
	if err != nil {
		t.Fatalf("GetBalance returned error: %v", err)
	}
	if balanceMicros != 2_000_000 {
		t.Fatalf("unexpected balance after repeated payment apply: got %d want %d", balanceMicros, int64(2_000_000))
	}
}

func TestPurchaseServiceRedeemCodeDeliveryIgnoresCurrentAccountPlanConflict(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	settingsSvc := NewPurchaseSettingsService()
	enableDebugPurchase(t, settingsSvc)

	user := createPurchaseTestUser(t, "buyer-redeem-code")
	activePlan := createPurchaseTestPlanWithUpgrade(t, "Active Plan", 10, 300)
	targetPlan := createPurchaseTestPlanWithUpgrade(t, "Gift Plan", 20, 500)
	product := createPurchaseTestProduct(t, targetPlan.ID, "升级礼品", 30, 1990)

	activeExpiry := time.Now().UTC().AddDate(0, 0, 10)
	if err := repository.NewUserSubscriptionRepository().Assign(&model.UserSubscription{
		UserID:    user.ID,
		PlanID:    activePlan.ID,
		StartsAt:  time.Now().UTC().AddDate(0, 0, -20),
		ExpiresAt: &activeExpiry,
		Status:    model.SubscriptionStatusActive,
	}); err != nil {
		t.Fatalf("Assign subscription returned error: %v", err)
	}

	svc := NewPurchaseServiceWithDeps(
		repository.NewPurchaseProductRepository(),
		repository.NewPurchaseOrderRepository(),
		repository.NewSubscriptionPlanRepository(),
		repository.NewUserSubscriptionRepository(),
		settingsSvc,
		fakePaymentGateway{},
	)

	order, err := svc.CreateOrder(context.Background(), user.ID, user.Username, product.ID, model.PurchaseDeliveryModeRedeemCode)
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if order.DeliveryMode != model.PurchaseDeliveryModeRedeemCode {
		t.Fatalf("delivery mode = %s, want redeem_code", order.DeliveryMode)
	}
	if order.GeneratedRedeemCode == "" {
		t.Fatal("expected generated redeem code")
	}

	sub, err := repository.NewUserSubscriptionRepository().GetActiveByUserID(user.ID)
	if err != nil {
		t.Fatalf("GetActiveByUserID returned error: %v", err)
	}
	if sub == nil || sub.PlanID != activePlan.ID {
		t.Fatalf("active subscription plan = %v, want %s", sub, activePlan.ID)
	}
}

func TestPurchaseServiceUpgradeToHigherRankConsumesOldEntitlements(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	settingsSvc := NewPurchaseSettingsService()
	enableDebugPurchase(t, settingsSvc)

	user := createPurchaseTestUser(t, "buyer-upgrade")
	lowPlan := createPurchaseTestPlanWithUpgrade(t, "Starter", 10, 100)
	highPlan := createPurchaseTestPlanWithUpgrade(t, "Pro", 20, 200)
	product := createPurchaseTestProduct(t, highPlan.ID, "Pro 30 天", 30, 3000)

	initialExpiry := time.Now().UTC().AddDate(0, 0, 10).Truncate(time.Second)
	if err := repository.NewUserSubscriptionRepository().Assign(&model.UserSubscription{
		UserID:    user.ID,
		PlanID:    lowPlan.ID,
		StartsAt:  time.Now().UTC().AddDate(0, 0, -20),
		ExpiresAt: &initialExpiry,
		Status:    model.SubscriptionStatusActive,
	}); err != nil {
		t.Fatalf("Assign subscription returned error: %v", err)
	}

	svc := NewPurchaseServiceWithDeps(
		repository.NewPurchaseProductRepository(),
		repository.NewPurchaseOrderRepository(),
		repository.NewSubscriptionPlanRepository(),
		repository.NewUserSubscriptionRepository(),
		settingsSvc,
		fakePaymentGateway{},
	)

	order, err := svc.CreateOrder(context.Background(), user.ID, user.Username, product.ID, model.PurchaseDeliveryModeAccount)
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if order.UpgradeSourcePlanID != lowPlan.ID {
		t.Fatalf("upgrade source plan = %s, want %s", order.UpgradeSourcePlanID, lowPlan.ID)
	}
	if order.UpgradeCreditCnyCent <= 0 {
		t.Fatalf("expected positive upgrade credit, got %d", order.UpgradeCreditCnyCent)
	}

	sub, err := repository.NewUserSubscriptionRepository().GetActiveByUserID(user.ID)
	if err != nil {
		t.Fatalf("GetActiveByUserID returned error: %v", err)
	}
	if sub == nil || sub.PlanID != highPlan.ID {
		t.Fatalf("upgraded plan = %v, want %s", sub, highPlan.ID)
	}
	if sub.ExpiresAt == nil || !sub.ExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("expected upgraded subscription expiry in the future, got %v", sub.ExpiresAt)
	}
}

func TestPurchaseServiceApplySuccessfulPaymentQueuesAndDeliversWebhook(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	user := createPurchaseTestUser(t, "buyer-webhook")
	plan := createPurchaseTestPlan(t, "Webhook Plan")
	product := createPurchaseTestProduct(t, plan.ID, "Webhook 商品", 30, 990)
	order := createPendingPurchaseTestOrder(t, user.ID, product, model.PurchaseOrderKindSubscription, 0)

	var receivedBody string
	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		receivedHeader = r.Header.Get("X-Order-No")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	adminRepo := repository.NewPurchaseAdminRepository()
	if err := adminRepo.CreateWebhookTarget(&model.PurchaseWebhookTarget{
		Name:            "primary",
		TargetURL:       server.URL,
		BodyTemplate:    `{"orderNo":"{{ orderNo }}","amount":"{{ amountCny }}"}`,
		HeadersTemplate: `{"X-Order-No":"{{ orderNo }}"}`,
		Enabled:         true,
	}); err != nil {
		t.Fatalf("CreateWebhookTarget returned error: %v", err)
	}

	svc := NewPurchaseService()
	if _, err := svc.applySuccessfulPayment(order.OrderNo, "trade-webhook", nil); err != nil {
		t.Fatalf("applySuccessfulPayment returned error: %v", err)
	}
	if err := svc.ProcessPendingWebhookEvents(context.Background(), 1); err != nil {
		t.Fatalf("ProcessPendingWebhookEvents returned error: %v", err)
	}

	if !strings.Contains(receivedBody, order.OrderNo) {
		t.Fatalf("expected webhook body to contain order no, got %q", receivedBody)
	}
	if receivedHeader != order.OrderNo {
		t.Fatalf("expected webhook header %q, got %q", order.OrderNo, receivedHeader)
	}
}

func TestPurchaseServiceUpdateOrderPaymentStatusAdminCreatesHistory(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	user := createPurchaseTestUser(t, "buyer-admin-status")
	plan := createPurchaseTestPlan(t, "Admin Status Plan")
	product := createPurchaseTestProduct(t, plan.ID, "Admin Status 商品", 30, 990)
	order := createPendingPurchaseTestOrder(t, user.ID, product, model.PurchaseOrderKindSubscription, 0)

	svc := NewPurchaseService()
	if _, err := svc.UpdateOrderPaymentStatusAdmin(order.OrderNo, &model.PurchaseOrderPaymentStatusUpdateRequest{
		PaymentStatus: model.PurchasePaymentStatusRefunded,
		Note:          "manual mark",
	}, "admin"); err != nil {
		t.Fatalf("UpdateOrderPaymentStatusAdmin returned error: %v", err)
	}

	updated, err := svc.orderRepo.GetDetailByOrderNo(order.OrderNo)
	if err != nil {
		t.Fatalf("GetDetailByOrderNo returned error: %v", err)
	}
	if updated == nil || updated.PaymentStatus != model.PurchasePaymentStatusRefunded {
		t.Fatalf("unexpected payment status: %+v", updated)
	}

	history, err := svc.ListOrderPaymentStatusHistory(order.OrderNo)
	if err != nil {
		t.Fatalf("ListOrderPaymentStatusHistory returned error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history count = %d, want 1", len(history))
	}
	if history[0].Note != "manual mark" {
		t.Fatalf("history note = %q", history[0].Note)
	}
}

func TestPurchaseServiceSingleManualSettlementRejectsDuplicate(t *testing.T) {
	setupPurchaseServiceTestDB(t)

	user := createPurchaseTestUser(t, "buyer-manual-settlement")
	plan := createPurchaseTestPlan(t, "Settlement Plan")
	product := createPurchaseTestProduct(t, plan.ID, "Settlement 商品", 30, 990)
	order := createPendingPurchaseTestOrder(t, user.ID, product, model.PurchaseOrderKindSubscription, 0)

	svc := NewPurchaseService()
	if _, err := svc.UpdateOrderPaymentStatusAdmin(order.OrderNo, &model.PurchaseOrderPaymentStatusUpdateRequest{
		PaymentStatus: model.PurchasePaymentStatusPaid,
		Note:          "paid",
	}, "admin"); err != nil {
		t.Fatalf("UpdateOrderPaymentStatusAdmin returned error: %v", err)
	}

	if _, err := svc.CreateSingleManualSettlement(order.OrderNo, "batch-1", "admin"); err != nil {
		t.Fatalf("CreateSingleManualSettlement returned error: %v", err)
	}
	if _, err := svc.CreateSingleManualSettlement(order.OrderNo, "batch-2", "admin"); !errors.Is(err, ErrPurchaseManualSettlementDup) {
		t.Fatalf("expected ErrPurchaseManualSettlementDup, got %v", err)
	}
}
