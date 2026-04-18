package service

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/precision"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"
)

var (
	ErrPurchaseDisabled           = errors.New("订阅购买功能未开启")
	ErrPaymentUnavailable         = errors.New("支付功能暂不可用")
	ErrPurchaseProductDisabled    = errors.New("该售卖商品已下架")
	ErrPurchaseOrderNotFound      = errors.New("订单不存在")
	ErrPurchaseProductHasOrders   = errors.New("该商品已有订单，无法删除")
	ErrDifferentPlanActive        = errors.New("当前账号已有其他生效中的订阅，暂不支持切换购买")
	ErrPermanentSubscription      = errors.New("当前账号已有永久订阅，无法续费")
	ErrBalanceTopupUnavailable    = errors.New("余额充值未开启")
	ErrInvalidBalanceTopupAmount  = errors.New("充值金额无效")
	ErrPendingSubscriptionOrder   = errors.New("当前已有待支付的订阅相关订单，请先处理后再下单")
	ErrDowngradeNotAllowed        = errors.New("当前仅允许购买更高级套餐升级，不允许降级")
	ErrSameRankPlanSwitch         = errors.New("同级别不同套餐不支持直接切换，请选择兑换码交付或联系管理员")
	ErrUpgradeConflict            = errors.New("升级报价已失效，请重新下单")
	ErrUpgradeValuationMissing    = errors.New("套餐升级估值未配置，暂不支持升级")
	ErrBoostRequiresSubscription  = errors.New("当前没有可加额的有效订阅")
	ErrInvalidSubscriptionMode    = errors.New("订阅购买模式无效")
	ErrTargetSubscriptionRequired = errors.New("续期模式需要指定目标订阅")
	ErrTargetSubscriptionInvalid  = errors.New("目标订阅无效")
)

const (
	balanceTopupPlanID    = "system-balance-topup-plan"
	balanceTopupProductID = "system-balance-topup-product"
)

func normalizePurchaseSubscriptionMode(mode model.PurchaseSubscriptionMode) model.PurchaseSubscriptionMode {
	if mode == "" {
		return model.PurchaseSubscriptionModeNew
	}
	return mode
}

func (s *PurchaseService) resolvePurchaseMode(userID string, product *model.PurchaseProduct, deliveryMode model.PurchaseDeliveryMode, mode model.PurchaseSubscriptionMode, targetSubscriptionID string) (model.PurchaseSubscriptionMode, string, error) {
	mode = normalizePurchaseSubscriptionMode(mode)
	if deliveryMode != model.PurchaseDeliveryModeAccount {
		return model.PurchaseSubscriptionModeNew, "", nil
	}
	if mode == model.PurchaseSubscriptionModeRenew {
		targetSubscriptionID = strings.TrimSpace(targetSubscriptionID)
		if targetSubscriptionID == "" {
			return "", "", ErrTargetSubscriptionRequired
		}
		activeSubs, err := s.subRepo.ListActiveByUserID(userID)
		if err != nil {
			return "", "", err
		}
		for _, sub := range activeSubs {
			if sub.ID == targetSubscriptionID && sub.PlanID == product.SubscriptionPlanID {
				return model.PurchaseSubscriptionModeRenew, sub.ID, nil
			}
		}
		return "", "", ErrTargetSubscriptionInvalid
	}
	if mode != model.PurchaseSubscriptionModeNew {
		return "", "", ErrInvalidSubscriptionMode
	}
	return model.PurchaseSubscriptionModeNew, "", nil
}

func (s *PurchaseService) findActiveSubscriptionByIDAndPlan(userID, subscriptionID, planID string) (*model.UserSubscription, error) {
	activeSubs, err := s.subRepo.ListActiveByUserID(userID)
	if err != nil {
		return nil, err
	}
	for _, sub := range activeSubs {
		if sub.ID == subscriptionID && sub.PlanID == planID {
			return sub, nil
		}
	}
	return nil, nil
}

type purchasePaymentGateway interface {
	CreateOrder(ctx context.Context, order *model.PurchaseOrder, product *model.PurchaseProductResponse, username string) (*PaymentCreateResult, error)
	QueryOrder(ctx context.Context, orderNo string) (*PaymentQueryResult, error)
	DecodeNotification(ctx context.Context, values url.Values) (*PaymentNotification, error)
}

type PurchaseService struct {
	productRepo repository.PurchaseProductRepositoryInterface
	orderRepo   repository.PurchaseOrderRepositoryInterface
	adminRepo   *repository.PurchaseAdminRepository
	planRepo    repository.SubscriptionPlanRepositoryInterface
	subRepo     repository.UserSubscriptionRepositoryInterface
	settingsSvc *PurchaseSettingsService
	payment     purchasePaymentGateway
	grantSvc    *RewardGrantService
	couponSvc   *CouponService
	inviteSvc   *InviteService
	runtimeRepo *repository.SubscriptionRuntimeRepository
}

type purchaseUpgradeQuote struct {
	SourcePlanID        string
	SourceExpiresAt     *time.Time
	CreditCNYCent       int64
	LockedTargetSeconds int64
	PayableCNYCent      int64
	StateToken          string
}

func NewPurchaseService() *PurchaseService {
	return &PurchaseService{
		productRepo: repository.NewPurchaseProductRepository(),
		orderRepo:   repository.NewPurchaseOrderRepository(),
		adminRepo:   repository.NewPurchaseAdminRepository(),
		planRepo:    repository.NewSubscriptionPlanRepository(),
		subRepo:     repository.NewUserSubscriptionRepository(),
		settingsSvc: NewPurchaseSettingsService(),
		payment:     NewAlipayService(),
		grantSvc:    NewRewardGrantService(),
		couponSvc:   NewCouponService(),
		inviteSvc:   NewInviteService(),
		runtimeRepo: repository.NewSubscriptionRuntimeRepository(),
	}
}

func NewPurchaseServiceWithDeps(
	productRepo repository.PurchaseProductRepositoryInterface,
	orderRepo repository.PurchaseOrderRepositoryInterface,
	planRepo repository.SubscriptionPlanRepositoryInterface,
	subRepo repository.UserSubscriptionRepositoryInterface,
	settingsSvc *PurchaseSettingsService,
	payment purchasePaymentGateway,
) *PurchaseService {
	if settingsSvc == nil {
		settingsSvc = NewPurchaseSettingsService()
	}
	if payment == nil {
		payment = NewAlipayService()
	}
	return &PurchaseService{
		productRepo: productRepo,
		orderRepo:   orderRepo,
		adminRepo:   repository.NewPurchaseAdminRepository(),
		planRepo:    planRepo,
		subRepo:     subRepo,
		settingsSvc: settingsSvc,
		payment:     payment,
		grantSvc:    NewRewardGrantService(),
		couponSvc:   NewCouponService(),
		inviteSvc:   NewInviteService(),
		runtimeRepo: repository.NewSubscriptionRuntimeRepository(),
	}
}

func (s *PurchaseService) GetSettings() (*model.PurchaseSettingsResponse, error) {
	settings, err := s.settingsSvc.Get()
	if err != nil {
		return nil, err
	}
	return s.settingsSvc.ToResponse(settings), nil
}

func (s *PurchaseService) UpdateSettings(req *model.PurchaseSettingsRequest) (*model.PurchaseSettingsResponse, error) {
	return s.settingsSvc.Update(req)
}

func (s *PurchaseService) GetCatalog(userID string) (*model.PurchaseCatalogResponse, error) {
	settings, err := s.settingsSvc.Get()
	if err != nil {
		return nil, err
	}
	couponConfig, err := s.couponSvc.GetConfig()
	if err != nil {
		return nil, err
	}

	products, err := s.productRepo.List(false)
	if err != nil {
		return nil, err
	}
	plans, err := s.listPlanMap()
	if err != nil {
		return nil, err
	}

	subscriptions, err := s.getActiveSubscriptionResponses(userID)
	if err != nil {
		return nil, err
	}
	var currentSubscription *model.UserSubscriptionResponse
	if len(subscriptions) > 0 {
		currentSubscription = subscriptions[0]
	}

	responses := make([]*model.PurchaseProductResponse, 0, len(products))
	for _, product := range products {
		plan := plans[product.SubscriptionPlanID]
		if plan == nil || !plan.Enabled {
			continue
		}
		if product.ProductKind != model.PurchaseProductKindSubscription && product.ProductKind != model.PurchaseProductKindBalanceTopup {
			continue
		}
		responses = append(responses, s.toProductResponse(product, plan.Name))
	}

	return &model.PurchaseCatalogResponse{
		PurchaseEnabled:                settings.PurchaseEnabled,
		CouponEnabled:                  couponConfig.Enabled,
		DebugAutoPaid:                  settings.DebugAutoPaid,
		PaymentConfigured:              s.settingsSvc.CanCreateOrders(settings),
		RenewalRule:                    "同套餐购买顺延到期，不同套餐需先取消当前订阅",
		CurrentSubscription:            currentSubscription,
		Subscriptions:                  subscriptions,
		Products:                       responses,
		BalanceTopupEnabled:            s.settingsSvc.CanCreateBalanceTopup(settings),
		BalanceTopupPriceCnyPerUsd:     float64(settings.BalanceTopupPriceCnyCentPerUSD) / 100,
		BalanceTopupPriceCnyCentPerUsd: settings.BalanceTopupPriceCnyCentPerUSD,
	}, nil
}

func (s *PurchaseService) QuoteOrder(ctx context.Context, userID string, req *model.PurchaseQuoteRequest) (*model.PurchaseQuoteResponse, error) {
	if req == nil {
		return nil, errors.New("请求参数错误")
	}
	switch req.Kind {
	case model.PurchaseOrderKindBalanceTopup:
		return s.quoteBalanceTopupOrder(ctx, userID, req.AmountUsd, req.CouponCode)
	default:
		return s.quoteSubscriptionOrderWithOptions(ctx, userID, req.ProductID, req.DeliveryMode, req.SubscriptionMode, req.TargetSubscriptionID, req.CouponCode)
	}
}

func (s *PurchaseService) ListProductsAdmin() ([]*model.PurchaseProductResponse, error) {
	products, err := s.productRepo.List(true)
	if err != nil {
		return nil, err
	}
	plans, err := s.listPlanMap()
	if err != nil {
		return nil, err
	}

	responses := make([]*model.PurchaseProductResponse, 0, len(products))
	for _, product := range products {
		planName := ""
		if plan := plans[product.SubscriptionPlanID]; plan != nil {
			planName = plan.Name
		}
		responses = append(responses, s.toProductResponse(product, planName))
	}
	return responses, nil
}

func (s *PurchaseService) CreateProduct(req *model.PurchaseProductRequest) (*model.PurchaseProductResponse, error) {
	plan, _, err := s.planRepo.GetByID(req.SubscriptionPlanID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound
	}

	product := &model.PurchaseProduct{
		Name:               strings.TrimSpace(req.Name),
		Summary:            strings.TrimSpace(req.Summary),
		ProductKind:        req.ProductKind,
		SubscriptionPlanID: req.SubscriptionPlanID,
		DurationDays:       req.DurationDays,
		PriceCNYCent:       req.PriceCNYCent,
		ActionSnapshotJSON: strings.TrimSpace(req.ActionSnapshotJSON),
		GroupName:          strings.TrimSpace(req.GroupName),
		GroupSort:          req.GroupSort,
		IsRecommended:      req.IsRecommended,
		SortOrder:          req.SortOrder,
		Enabled:            req.Enabled,
	}
	if product.ProductKind == "" {
		product.ProductKind = model.PurchaseProductKindSubscription
	}
	if product.ProductKind != model.PurchaseProductKindSubscription && product.ProductKind != model.PurchaseProductKindBalanceTopup {
		return nil, errors.New("仅支持基础订阅和余额充值商品")
	}
	if err := s.productRepo.Create(product); err != nil {
		return nil, err
	}
	return s.toProductResponse(product, plan.Name), nil
}

func (s *PurchaseService) UpdateProduct(id string, req *model.PurchaseProductRequest) (*model.PurchaseProductResponse, error) {
	plan, _, err := s.planRepo.GetByID(req.SubscriptionPlanID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound
	}
	product := &model.PurchaseProduct{
		Name:               strings.TrimSpace(req.Name),
		Summary:            strings.TrimSpace(req.Summary),
		ProductKind:        req.ProductKind,
		SubscriptionPlanID: req.SubscriptionPlanID,
		DurationDays:       req.DurationDays,
		PriceCNYCent:       req.PriceCNYCent,
		ActionSnapshotJSON: strings.TrimSpace(req.ActionSnapshotJSON),
		GroupName:          strings.TrimSpace(req.GroupName),
		GroupSort:          req.GroupSort,
		IsRecommended:      req.IsRecommended,
		SortOrder:          req.SortOrder,
		Enabled:            req.Enabled,
	}
	if product.ProductKind == "" {
		product.ProductKind = model.PurchaseProductKindSubscription
	}
	if product.ProductKind != model.PurchaseProductKindSubscription && product.ProductKind != model.PurchaseProductKindBalanceTopup {
		return nil, errors.New("仅支持基础订阅和余额充值商品")
	}
	if err := s.productRepo.Update(id, product); err != nil {
		return nil, err
	}
	updated, err := s.productRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, repository.ErrPurchaseProductNotFound
	}
	return s.toProductResponse(updated, plan.Name), nil
}

func (s *PurchaseService) DeleteProduct(id string) error {
	count, err := s.orderRepo.CountByProductID(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrPurchaseProductHasOrders
	}
	return s.productRepo.Delete(id)
}

func (s *PurchaseService) SetProductEnabled(id string, enabled bool) error {
	return s.productRepo.SetEnabled(id, enabled)
}

func (s *PurchaseService) CreateOrder(ctx context.Context, userID, username, productID string, deliveryMode model.PurchaseDeliveryMode, couponCode ...string) (*model.PurchaseOrderResponse, error) {
	return s.CreateOrderWithOptions(ctx, userID, username, productID, deliveryMode, model.PurchaseSubscriptionModeNew, "", couponCode...)
}

func (s *PurchaseService) CreateOrderWithOptions(ctx context.Context, userID, username, productID string, deliveryMode model.PurchaseDeliveryMode, subscriptionMode model.PurchaseSubscriptionMode, targetSubscriptionID string, couponCode ...string) (*model.PurchaseOrderResponse, error) {
	requestCouponCode := ""
	if len(couponCode) > 0 {
		requestCouponCode = couponCode[0]
	}
	settings, err := s.settingsSvc.Get()
	if err != nil {
		return nil, err
	}
	if !settings.PurchaseEnabled {
		return nil, ErrPurchaseDisabled
	}
	if deliveryMode == "" {
		deliveryMode = model.PurchaseDeliveryModeAccount
	}

	product, plan, productResp, err := s.loadPurchasableProduct(productID)
	if err != nil {
		return nil, err
	}
	if !product.Enabled || plan == nil || !plan.Enabled {
		return nil, ErrPurchaseProductDisabled
	}

	if count, err := s.orderRepo.CountPendingSubscriptionOrdersByUser(userID); err != nil {
		return nil, err
	} else if count > 0 {
		return nil, ErrPendingSubscriptionOrder
	}

	resolvedMode, resolvedTargetSubscriptionID, err := s.resolvePurchaseMode(userID, product, deliveryMode, subscriptionMode, targetSubscriptionID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()

	order := &model.PurchaseOrder{
		ID:                    uuid.New().String(),
		OrderNo:               newPurchaseOrderNo(now),
		UserID:                userID,
		ProductID:             product.ID,
		SubscriptionPlanID:    product.SubscriptionPlanID,
		DurationDays:          product.DurationDays,
		OriginalAmountCNYCent: product.PriceCNYCent,
		AmountCNYCent:         product.PriceCNYCent,
		OrderKind:             model.PurchaseOrderKindSubscription,
		DeliveryMode:          deliveryMode,
		SubscriptionMode:      resolvedMode,
		TargetSubscriptionID:  resolvedTargetSubscriptionID,
		PaymentChannel:        model.PaymentChannelAlipay,
		PaymentStatus:         model.PurchasePaymentStatusPending,
		FulfillmentStatus:     model.PurchaseFulfillmentStatusPending,
		FailureReason:         "",
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	appliedCoupon, err := s.couponSvc.EvaluateCodeTx(tx, userID, requestCouponCode, product.PriceCNYCent, now)
	if err != nil {
		return nil, err
	}
	s.couponSvc.ApplyQuoteToOrder(order, product.PriceCNYCent, appliedCoupon)
	if !settings.DebugAutoPaid && order.AmountCNYCent > 0 && !s.settingsSvc.CanCreateOrders(settings) {
		return nil, ErrPaymentUnavailable
	}
	if err := s.orderRepo.CreateTx(tx, order); err != nil {
		return nil, err
	}
	if err := s.couponSvc.CommitUsageTx(tx, userID, order, appliedCoupon, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if settings.DebugAutoPaid || order.AmountCNYCent == 0 {
		return s.applySuccessfulPayment(order.OrderNo, "debug-"+order.OrderNo, &now)
	}

	paymentResult, err := s.payment.CreateOrder(ctx, order, productResp, username)
	if err != nil {
		_ = s.markOrderFailed(order.OrderNo, err.Error())
		return nil, err
	}

	if err := s.updateOrderPrecreateData(order.OrderNo, paymentResult); err != nil {
		return nil, err
	}
	return s.GetOrderForUser(userID, order.OrderNo)
}

func (s *PurchaseService) CreateBalanceTopupOrder(ctx context.Context, userID, username string, amountUsd precision.DecimalString, couponCode ...string) (*model.PurchaseOrderResponse, error) {
	requestCouponCode := ""
	if len(couponCode) > 0 {
		requestCouponCode = couponCode[0]
	}
	settings, err := s.settingsSvc.Get()
	if err != nil {
		return nil, err
	}
	if !s.settingsSvc.CanCreateBalanceTopup(settings) {
		return nil, ErrBalanceTopupUnavailable
	}

	balanceTopupMicros, err := precision.ParseUSDToMicros(amountUsd)
	if err != nil {
		return nil, ErrInvalidBalanceTopupAmount
	}
	if balanceTopupMicros <= 0 {
		return nil, ErrInvalidBalanceTopupAmount
	}
	originalAmountCNYCent := int64(math.Round((float64(balanceTopupMicros) / 1_000_000) * float64(settings.BalanceTopupPriceCnyCentPerUSD)))
	if originalAmountCNYCent <= 0 {
		return nil, ErrInvalidBalanceTopupAmount
	}

	if err := s.ensureBalanceTopupPlaceholders(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	order := &model.PurchaseOrder{
		ID:                    uuid.New().String(),
		OrderNo:               newPurchaseOrderNo(now),
		UserID:                userID,
		ProductID:             balanceTopupProductID,
		SubscriptionPlanID:    balanceTopupPlanID,
		DurationDays:          1,
		OriginalAmountCNYCent: originalAmountCNYCent,
		AmountCNYCent:         originalAmountCNYCent,
		OrderKind:             model.PurchaseOrderKindBalanceTopup,
		DeliveryMode:          model.PurchaseDeliveryModeAccount,
		BalanceTopupMicros:    balanceTopupMicros,
		PaymentChannel:        model.PaymentChannelAlipay,
		PaymentStatus:         model.PurchasePaymentStatusPending,
		FulfillmentStatus:     model.PurchaseFulfillmentStatusPending,
		FailureReason:         "",
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	appliedCoupon, err := s.couponSvc.EvaluateCodeTx(tx, userID, requestCouponCode, originalAmountCNYCent, now)
	if err != nil {
		return nil, err
	}
	s.couponSvc.ApplyQuoteToOrder(order, originalAmountCNYCent, appliedCoupon)
	if !settings.DebugAutoPaid && order.AmountCNYCent > 0 && !s.settingsSvc.CanCreateOrders(settings) {
		return nil, ErrPaymentUnavailable
	}
	if err := s.orderRepo.CreateTx(tx, order); err != nil {
		return nil, err
	}
	if err := s.couponSvc.CommitUsageTx(tx, userID, order, appliedCoupon, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	productResp := &model.PurchaseProductResponse{
		ID:                   balanceTopupProductID,
		Name:                 "余额充值",
		Summary:              fmt.Sprintf("$%.2f", float64(balanceTopupMicros)/1e6),
		SubscriptionPlanID:   balanceTopupPlanID,
		SubscriptionPlanName: "余额充值",
		DurationDays:         1,
		PriceCNYCent:         order.AmountCNYCent,
		GroupName:            "",
		GroupSort:            0,
		Enabled:              false,
	}

	if settings.DebugAutoPaid || order.AmountCNYCent == 0 {
		return s.applySuccessfulPayment(order.OrderNo, "debug-"+order.OrderNo, &now)
	}

	paymentResult, err := s.payment.CreateOrder(ctx, order, productResp, username)
	if err != nil {
		_ = s.markOrderFailed(order.OrderNo, err.Error())
		return nil, err
	}
	if err := s.updateOrderPrecreateData(order.OrderNo, paymentResult); err != nil {
		return nil, err
	}
	return s.GetOrderForUser(userID, order.OrderNo)
}

func (s *PurchaseService) quoteSubscriptionOrder(ctx context.Context, userID, productID string, deliveryMode model.PurchaseDeliveryMode, couponCode string) (*model.PurchaseQuoteResponse, error) {
	return s.quoteSubscriptionOrderWithOptions(ctx, userID, productID, deliveryMode, model.PurchaseSubscriptionModeNew, "", couponCode)
}

func (s *PurchaseService) quoteSubscriptionOrderWithOptions(ctx context.Context, userID, productID string, deliveryMode model.PurchaseDeliveryMode, subscriptionMode model.PurchaseSubscriptionMode, targetSubscriptionID string, couponCode string) (*model.PurchaseQuoteResponse, error) {
	if deliveryMode == "" {
		deliveryMode = model.PurchaseDeliveryModeAccount
	}
	product, plan, _, err := s.loadPurchasableProduct(productID)
	if err != nil {
		return nil, err
	}
	if !product.Enabled || plan == nil || !plan.Enabled {
		return nil, ErrPurchaseProductDisabled
	}
	resolvedMode, resolvedTargetSubscriptionID, err := s.resolvePurchaseMode(userID, product, deliveryMode, subscriptionMode, targetSubscriptionID)
	if err != nil {
		return nil, err
	}
	resp := &model.PurchaseQuoteResponse{
		Kind:                  model.PurchaseOrderKindSubscription,
		ProductID:             product.ID,
		ProductName:           product.Name,
		OrderKindLabel:        "订阅购买",
		DeliveryMode:          deliveryMode,
		SubscriptionMode:      resolvedMode,
		TargetSubscriptionID:  resolvedTargetSubscriptionID,
		OriginalAmountCNYCent: product.PriceCNYCent,
		FinalAmountCNYCent:    product.PriceCNYCent,
	}
	if strings.TrimSpace(couponCode) == "" {
		return resp, nil
	}
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	applied, err := s.couponSvc.EvaluateCodeTx(tx, userID, couponCode, product.PriceCNYCent, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if applied != nil {
		resp.Coupon = applied.Quote
		resp.DiscountCNYCent = applied.Quote.DiscountCNYCent
		resp.FinalAmountCNYCent = maxInt64(product.PriceCNYCent-applied.Quote.DiscountCNYCent, 0)
	}
	return resp, nil
}

func (s *PurchaseService) quoteBalanceTopupOrder(_ context.Context, userID string, amountUsd string, couponCode string) (*model.PurchaseQuoteResponse, error) {
	settings, err := s.settingsSvc.Get()
	if err != nil {
		return nil, err
	}
	if !s.settingsSvc.CanCreateBalanceTopup(settings) {
		return nil, ErrBalanceTopupUnavailable
	}
	balanceTopupMicros, err := precision.ParseUSDToMicros(precision.DecimalString(amountUsd))
	if err != nil || balanceTopupMicros <= 0 {
		return nil, ErrInvalidBalanceTopupAmount
	}
	originalAmount := int64(math.Round((float64(balanceTopupMicros) / 1_000_000) * float64(settings.BalanceTopupPriceCnyCentPerUSD)))
	if originalAmount <= 0 {
		return nil, ErrInvalidBalanceTopupAmount
	}
	resp := &model.PurchaseQuoteResponse{
		Kind:                  model.PurchaseOrderKindBalanceTopup,
		ProductID:             balanceTopupProductID,
		ProductName:           "余额充值",
		OrderKindLabel:        "余额充值",
		DeliveryMode:          model.PurchaseDeliveryModeAccount,
		OriginalAmountCNYCent: originalAmount,
		FinalAmountCNYCent:    originalAmount,
		BalanceTopupMicros:    balanceTopupMicros,
	}
	if strings.TrimSpace(couponCode) == "" {
		return resp, nil
	}
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	applied, err := s.couponSvc.EvaluateCodeTx(tx, userID, couponCode, originalAmount, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if applied != nil {
		resp.Coupon = applied.Quote
		resp.DiscountCNYCent = applied.Quote.DiscountCNYCent
		resp.FinalAmountCNYCent = maxInt64(originalAmount-applied.Quote.DiscountCNYCent, 0)
	}
	return resp, nil
}

func (s *PurchaseService) GetOrderForUser(userID, orderNo string) (*model.PurchaseOrderResponse, error) {
	order, err := s.orderRepo.GetDetailByOrderNoForUser(orderNo, userID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrPurchaseOrderNotFound
	}
	if err := s.normalizeOrderState(order); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *PurchaseService) ListOrdersForUser(userID string) (*model.PurchaseOrderListResponse, error) {
	items, total, err := s.orderRepo.ListByUser(userID, 20)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := s.normalizeOrderState(item); err != nil {
			return nil, err
		}
	}
	return &model.PurchaseOrderListResponse{Items: items, Total: total}, nil
}

func (s *PurchaseService) ListOrdersAdmin(filters model.PurchaseOrderFilters) (*model.PurchaseOrderListResponse, error) {
	items, total, err := s.orderRepo.ListAdmin(filters)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := s.normalizeOrderState(item); err != nil {
			return nil, err
		}
	}
	return &model.PurchaseOrderListResponse{Items: items, Total: total}, nil
}

func (s *PurchaseService) RefreshOrderForUser(ctx context.Context, userID, orderNo string) (*model.PurchaseOrderResponse, error) {
	return s.retryRefreshOrder(func() (*model.PurchaseOrderResponse, error) {
		order, err := s.orderRepo.GetDetailByOrderNoForUser(orderNo, userID)
		if err != nil {
			return nil, err
		}
		if order == nil {
			return nil, ErrPurchaseOrderNotFound
		}
		return s.refreshOrder(ctx, order)
	})
}

func (s *PurchaseService) RefreshOrderAdmin(ctx context.Context, orderNo string) (*model.PurchaseOrderResponse, error) {
	return s.retryRefreshOrder(func() (*model.PurchaseOrderResponse, error) {
		order, err := s.orderRepo.GetDetailByOrderNo(orderNo)
		if err != nil {
			return nil, err
		}
		if order == nil {
			return nil, ErrPurchaseOrderNotFound
		}
		return s.refreshOrder(ctx, order)
	})
}

func (s *PurchaseService) HandleNotification(ctx context.Context, values url.Values) (*model.PurchaseOrderResponse, error) {
	notification, err := s.payment.DecodeNotification(ctx, values)
	if err != nil {
		return nil, err
	}
	if notification == nil || strings.TrimSpace(notification.OrderNo) == "" {
		return nil, ErrPurchaseOrderNotFound
	}

	switch notification.PaymentStatus {
	case model.PurchasePaymentStatusPaid:
		return s.applySuccessfulPayment(notification.OrderNo, notification.TradeNo, notification.PaidAt)
	case model.PurchasePaymentStatusClosed:
		if err := s.markOrderClosed(notification.OrderNo, notification.TradeNo, true); err != nil {
			return nil, err
		}
		return s.orderRepo.GetDetailByOrderNo(notification.OrderNo)
	default:
		return s.orderRepo.GetDetailByOrderNo(notification.OrderNo)
	}
}

func (s *PurchaseService) refreshOrder(ctx context.Context, order *model.PurchaseOrderResponse) (*model.PurchaseOrderResponse, error) {
	if order == nil {
		return nil, ErrPurchaseOrderNotFound
	}
	if order.PaymentStatus != model.PurchasePaymentStatusPending {
		if err := s.normalizeOrderState(order); err != nil {
			return nil, err
		}
		return order, nil
	}

	if order.ExpiresAt != nil && time.Now().UTC().After(order.ExpiresAt.UTC()) {
		if err := s.markOrderExpired(order.OrderNo); err != nil {
			return nil, err
		}
		return s.reloadOrderForRole(order)
	}

	queryResult, err := s.payment.QueryOrder(ctx, order.OrderNo)
	if err != nil {
		return nil, err
	}

	switch queryResult.PaymentStatus {
	case model.PurchasePaymentStatusPaid:
		return s.applySuccessfulPayment(order.OrderNo, queryResult.TradeNo, queryResult.PaidAt)
	case model.PurchasePaymentStatusClosed:
		if err := s.markOrderClosed(order.OrderNo, queryResult.TradeNo, false); err != nil {
			return nil, err
		}
		return s.reloadOrderForRole(order)
	default:
		if err := s.normalizeOrderState(order); err != nil {
			return nil, err
		}
		return order, nil
	}
}

func (s *PurchaseService) retryRefreshOrder(fn func() (*model.PurchaseOrderResponse, error)) (*model.PurchaseOrderResponse, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		order, err := fn()
		if err == nil {
			return order, nil
		}
		if !isTransientPurchaseConnectionError(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func isTransientPurchaseConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrConnDone) {
		return true
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "driver: bad connection") ||
		strings.Contains(message, "bad connection") ||
		strings.Contains(message, "connection is already closed")
}

func (s *PurchaseService) normalizeOrderState(order *model.PurchaseOrderResponse) error {
	if order == nil {
		return nil
	}

	if order.PaymentStatus == model.PurchasePaymentStatusPending && order.ExpiresAt != nil && time.Now().UTC().After(order.ExpiresAt.UTC()) {
		if err := s.markOrderExpired(order.OrderNo); err != nil {
			return err
		}
		reloaded, err := s.reloadOrderForRole(order)
		if err != nil {
			return err
		}
		*order = *reloaded
	}

	order.CanRefresh = order.PaymentStatus == model.PurchasePaymentStatusPending
	order.PaymentQRImageDataURL = buildPurchaseQRCodeDataURL(order.PaymentQRCode)
	return nil
}

func (s *PurchaseService) reloadOrderForRole(order *model.PurchaseOrderResponse) (*model.PurchaseOrderResponse, error) {
	if order == nil {
		return nil, ErrPurchaseOrderNotFound
	}
	if order.Username != "" {
		reloaded, err := s.orderRepo.GetDetailByOrderNo(order.OrderNo)
		if err != nil {
			return nil, err
		}
		if reloaded == nil {
			return nil, ErrPurchaseOrderNotFound
		}
		reloaded.CanRefresh = reloaded.PaymentStatus == model.PurchasePaymentStatusPending
		reloaded.PaymentQRImageDataURL = buildPurchaseQRCodeDataURL(reloaded.PaymentQRCode)
		return reloaded, nil
	}
	reloaded, err := s.orderRepo.GetDetailByOrderNoForUser(order.OrderNo, order.UserID)
	if err != nil {
		return nil, err
	}
	if reloaded == nil {
		return nil, ErrPurchaseOrderNotFound
	}
	reloaded.CanRefresh = reloaded.PaymentStatus == model.PurchasePaymentStatusPending
	reloaded.PaymentQRImageDataURL = buildPurchaseQRCodeDataURL(reloaded.PaymentQRCode)
	return reloaded, nil
}

func (s *PurchaseService) applySuccessfulPayment(orderNo, tradeNo string, paidAt *time.Time) (*model.PurchaseOrderResponse, error) {
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	if paidAt == nil {
		paidAt = &now
	}

	result, err := tx.Exec(
		`UPDATE purchase_orders
		    SET payment_status = ?, alipay_trade_no = CASE WHEN ? <> '' THEN ? ELSE alipay_trade_no END,
		        paid_at = COALESCE(paid_at, ?), failure_reason = '', updated_at = ?
		  WHERE order_no = ? AND payment_status = ?`,
		model.PurchasePaymentStatusPaid,
		tradeNo,
		tradeNo,
		paidAt.UTC(),
		now,
		orderNo,
		model.PurchasePaymentStatusPending,
	)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	order, err := s.getOrderByOrderNoTx(tx, orderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrPurchaseOrderNotFound
	}

	if affected == 0 && order.PaymentStatus != model.PurchasePaymentStatusPaid {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.orderRepo.GetDetailByOrderNo(orderNo)
	}

	if strings.TrimSpace(tradeNo) != "" {
		order.AlipayTradeNo = tradeNo
	}
	order.PaymentStatus = model.PurchasePaymentStatusPaid
	order.PaidAt = paidAt

	syncAction, err := s.fulfillOrderTx(tx, order, now)
	if err != nil {
		return nil, err
	}
	inviteActions, err := s.inviteSvc.HandlePaidOrderTx(tx, order, now)
	if err != nil {
		return nil, err
	}
	if err := s.adminRepo.QueueWebhookEventsTx(tx, order.ID, order.OrderNo); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	allSyncActions := append([]BillingStateSyncAction{syncAction}, inviteActions...)
	if err := SyncBillingStateActions(context.Background(), s.grantSvc, allSyncActions); err != nil {
		return nil, err
	}

	response, err := s.orderRepo.GetDetailByOrderNo(orderNo)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, ErrPurchaseOrderNotFound
	}
	return response, s.normalizeOrderState(response)
}

func (s *PurchaseService) fulfillOrderTx(tx *sql.Tx, order *model.PurchaseOrder, now time.Time) (BillingStateSyncAction, error) {
	if order.FulfillmentStatus == model.PurchaseFulfillmentStatusFulfilled {
		if order.OrderKind == model.PurchaseOrderKindSubscription && order.DeliveryMode == model.PurchaseDeliveryModeAccount {
			return BillingStateSyncAction{
				UserID:           order.UserID,
				RefreshUserState: true,
			}, nil
		}
		return BillingStateSyncAction{}, nil
	}

	syncAction := BillingStateSyncAction{}
	switch order.OrderKind {
	case model.PurchaseOrderKindBalanceTopup:
		if _, err := s.grantSvc.GrantBalanceTx(tx, order.UserID, order.BalanceTopupMicros, now); err != nil {
			return BillingStateSyncAction{}, err
		}
		syncAction = BillingStateSyncAction{
			UserID:             order.UserID,
			BalanceDeltaMicros: order.BalanceTopupMicros,
		}
	default:
		switch {
		case order.DeliveryMode == model.PurchaseDeliveryModeRedeemCode:
			if err := s.createGeneratedRedeemCodeTx(tx, order, now); err != nil {
				return BillingStateSyncAction{}, err
			}
		default:
			switch normalizePurchaseSubscriptionMode(order.SubscriptionMode) {
			case model.PurchaseSubscriptionModeRenew:
				targetSub, err := s.findActiveSubscriptionByIDAndPlan(order.UserID, order.TargetSubscriptionID, order.SubscriptionPlanID)
				if err != nil {
					return BillingStateSyncAction{}, err
				}
				if targetSub == nil {
					_, updateErr := tx.Exec(
						`UPDATE purchase_orders SET fulfillment_status = ?, failure_reason = ?, updated_at = ? WHERE order_no = ?`,
						model.PurchaseFulfillmentStatusFailed,
						ErrTargetSubscriptionInvalid.Error(),
						now,
						order.OrderNo,
					)
					return BillingStateSyncAction{}, updateErr
				}
				if targetSub.ExpiresAt == nil {
					_, updateErr := tx.Exec(
						`UPDATE purchase_orders SET fulfillment_status = ?, failure_reason = ?, updated_at = ? WHERE order_no = ?`,
						model.PurchaseFulfillmentStatusFailed,
						ErrPermanentSubscription.Error(),
						now,
						order.OrderNo,
					)
					return BillingStateSyncAction{}, updateErr
				}
				base := now
				if targetSub.ExpiresAt.After(now) {
					base = targetSub.ExpiresAt.UTC()
				}
				finalExpiresAt := base.AddDate(0, 0, order.DurationDays)
				if _, err := tx.Exec(`UPDATE user_subscriptions SET expires_at = ?, updated_at = ? WHERE id = ?`, finalExpiresAt, now, targetSub.ID); err != nil {
					return BillingStateSyncAction{}, err
				}
			default:
				sub := &model.UserSubscription{
					ID:        uuid.New().String(),
					UserID:    order.UserID,
					PlanID:    order.SubscriptionPlanID,
					StartsAt:  now,
					Status:    model.SubscriptionStatusActive,
					CreatedAt: now,
					UpdatedAt: now,
				}
				expiresAt := now.AddDate(0, 0, order.DurationDays)
				sub.ExpiresAt = &expiresAt
				if _, err := tx.Exec(
					`INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
					sub.ID, sub.UserID, sub.PlanID, sub.StartsAt, sub.ExpiresAt, sub.Status, sub.CreatedAt, sub.UpdatedAt,
				); err != nil {
					return BillingStateSyncAction{}, err
				}
			}
			syncAction = BillingStateSyncAction{
				UserID:           order.UserID,
				RefreshUserState: true,
			}
		}
	}

	if _, execErr := tx.Exec(
		`UPDATE purchase_orders
		    SET fulfillment_status = ?, fulfilled_at = COALESCE(fulfilled_at, ?), failure_reason = '', updated_at = ?
		  WHERE order_no = ?`,
		model.PurchaseFulfillmentStatusFulfilled,
		now,
		now,
		order.OrderNo,
	); execErr != nil {
		return BillingStateSyncAction{}, execErr
	}
	return syncAction, nil
}

func (s *PurchaseService) createGeneratedRedeemCodeTx(tx *sql.Tx, order *model.PurchaseOrder, now time.Time) error {
	codeValue, err := s.generatePurchaseRedeemCodeTx(tx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO redeem_codes (
			id, campaign_id, batch_id, source_type, source_ref_id, code_value, code_hash, code_mask,
			subscription_plan_id, subscription_duration_days, balance_micros, reward_snapshot_json, per_user_limit, starts_at, ends_at,
			status, max_redemptions, redeemed_count, last_redeemed_at, created_at, updated_at
		) VALUES (?, NULL, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, NULL, ?, ?)`,
		uuid.New().String(),
		model.RedeemCodeSourceTypePurchaseOrder,
		order.OrderNo,
		codeValue,
		hashRedeemCode(codeValue),
		maskRedeemCode(codeValue),
		order.SubscriptionPlanID,
		order.DurationDays,
		0,
		"",
		1,
		nil,
		nil,
		model.RedeemCodeStatusActive,
		1,
		now,
		now,
	)
	if err != nil {
		return err
	}
	if err := tx.QueryRow(
		`SELECT id FROM redeem_codes WHERE source_type = ? AND source_ref_id = ? ORDER BY created_at DESC LIMIT 1`,
		model.RedeemCodeSourceTypePurchaseOrder,
		order.OrderNo,
	).Scan(&order.GeneratedRedeemCodeID); err != nil {
		return err
	}
	_, err = tx.Exec(
		`UPDATE purchase_orders SET generated_redeem_code_id = ?, updated_at = ? WHERE order_no = ?`,
		order.GeneratedRedeemCodeID,
		now,
		order.OrderNo,
	)
	return err
}

func (s *PurchaseService) generatePurchaseRedeemCodeTx(tx *sql.Tx) (string, error) {
	seen := map[string]struct{}{}
	for attempts := 0; attempts < 256; attempts++ {
		randomPart, err := randomRedeemString(10)
		if err != nil {
			return "", err
		}
		value := "BUY-" + randomPart
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM redeem_codes WHERE code_hash = ?`, hashRedeemCode(value)).Scan(&count); err != nil {
			return "", err
		}
		if count == 0 {
			return value, nil
		}
	}
	return "", fmt.Errorf("生成购买兑换码失败")
}

func (s *PurchaseService) getOrderByOrderNoTx(tx *sql.Tx, orderNo string) (*model.PurchaseOrder, error) {
	order := &model.PurchaseOrder{}
	err := tx.QueryRow(
		`SELECT id, order_no, user_id, product_id, subscription_plan_id, duration_days, original_amount_cny_cent, discount_cny_cent, amount_cny_cent, order_kind, delivery_mode, balance_topup_micros, subscription_mode, target_subscription_id, payment_channel, payment_status,
		        fulfillment_status,
		        coupon_campaign_id, coupon_campaign_name, coupon_code_id, coupon_code_value, coupon_discount_type, coupon_percent_off_bps, coupon_fixed_discount_cny_cent, coupon_max_discount_cny_cent,
		        generated_redeem_code_id, manual_settlement_done, alipay_trade_no, alipay_qr_code, alipay_qr_url, expires_at, paid_at, fulfilled_at, failure_reason,
		        created_at, updated_at
		   FROM purchase_orders
		  WHERE order_no = ?`,
		orderNo,
	).Scan(
		&order.ID,
		&order.OrderNo,
		&order.UserID,
		&order.ProductID,
		&order.SubscriptionPlanID,
		&order.DurationDays,
		&order.OriginalAmountCNYCent,
		&order.DiscountCNYCent,
		&order.AmountCNYCent,
		&order.OrderKind,
		&order.DeliveryMode,
		&order.BalanceTopupMicros,
		&order.SubscriptionMode,
		&order.TargetSubscriptionID,
		&order.PaymentChannel,
		&order.PaymentStatus,
		&order.FulfillmentStatus,
		&order.CouponCampaignID,
		&order.CouponCampaignName,
		&order.CouponCodeID,
		&order.CouponCodeValue,
		&order.CouponDiscountType,
		&order.CouponPercentOffBPS,
		&order.CouponFixedDiscountCNYCent,
		&order.CouponMaxDiscountCNYCent,
		&order.GeneratedRedeemCodeID,
		&order.ManualSettlementDone,
		&order.AlipayTradeNo,
		&order.AlipayQRCode,
		&order.AlipayQRURL,
		&order.ExpiresAt,
		&order.PaidAt,
		&order.FulfilledAt,
		&order.FailureReason,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return order, err
}

func (s *PurchaseService) updateOrderPrecreateData(orderNo string, paymentResult *PaymentCreateResult) error {
	if paymentResult == nil {
		return nil
	}
	db := database.GetDB()
	_, err := db.Exec(
		`UPDATE purchase_orders
		    SET alipay_qr_code = ?, alipay_qr_url = ?, expires_at = ?, updated_at = ?
		  WHERE order_no = ?`,
		paymentResult.QRCode,
		paymentResult.QRURL,
		paymentResult.ExpiresAt.UTC(),
		time.Now().UTC(),
		orderNo,
	)
	return err
}

func (s *PurchaseService) markOrderFailed(orderNo, reason string) error {
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.Exec(
		`UPDATE purchase_orders SET payment_status = ?, failure_reason = ?, updated_at = ? WHERE order_no = ?`,
		model.PurchasePaymentStatusFailed,
		strings.TrimSpace(reason),
		now,
		orderNo,
	); err != nil {
		return err
	}
	order, err := s.getOrderByOrderNoTx(tx, orderNo)
	if err != nil {
		return err
	}
	if err := s.couponSvc.ReverseUsageForOrderTx(tx, order, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PurchaseService) markOrderExpired(orderNo string) error {
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	result, err := tx.Exec(
		`UPDATE purchase_orders
		    SET payment_status = CASE WHEN payment_status = ? THEN ? ELSE payment_status END,
		        updated_at = ?
		  WHERE order_no = ?`,
		model.PurchasePaymentStatusPending,
		model.PurchasePaymentStatusExpired,
		now,
		orderNo,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		order, err := s.getOrderByOrderNoTx(tx, orderNo)
		if err != nil {
			return err
		}
		if err := s.couponSvc.ReverseUsageForOrderTx(tx, order, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *PurchaseService) markOrderClosed(orderNo, tradeNo string, expired bool) error {
	db := database.GetDB()
	status := model.PurchasePaymentStatusClosed
	if expired {
		status = model.PurchasePaymentStatusExpired
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	result, err := tx.Exec(
		`UPDATE purchase_orders
		    SET payment_status = ?, alipay_trade_no = CASE WHEN ? <> '' THEN ? ELSE alipay_trade_no END, updated_at = ?
		  WHERE order_no = ? AND payment_status = ?`,
		status,
		tradeNo,
		tradeNo,
		now,
		orderNo,
		model.PurchasePaymentStatusPending,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		order, err := s.getOrderByOrderNoTx(tx, orderNo)
		if err != nil {
			return err
		}
		if err := s.couponSvc.ReverseUsageForOrderTx(tx, order, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *PurchaseService) loadPurchasableProduct(productID string) (*model.PurchaseProduct, *model.SubscriptionPlan, *model.PurchaseProductResponse, error) {
	product, err := s.productRepo.GetByID(productID)
	if err != nil {
		return nil, nil, nil, err
	}
	if product == nil {
		return nil, nil, nil, repository.ErrPurchaseProductNotFound
	}

	plan, _, err := s.planRepo.GetByID(product.SubscriptionPlanID)
	if err != nil {
		return nil, nil, nil, err
	}
	if plan == nil {
		return nil, nil, nil, ErrPlanNotFound
	}

	return product, plan, s.toProductResponse(product, plan.Name), nil
}

func (s *PurchaseService) listPlanMap() (map[string]*model.SubscriptionPlan, error) {
	plans, limitsMap, err := s.planRepo.List()
	if err != nil {
		return nil, err
	}
	_ = limitsMap
	result := make(map[string]*model.SubscriptionPlan, len(plans))
	for _, plan := range plans {
		if plan == nil {
			continue
		}
		result[plan.ID] = &model.SubscriptionPlan{
			ID:          plan.ID,
			Name:        plan.Name,
			Description: plan.Description,
			Enabled:     plan.Enabled,
			CreatedAt:   plan.CreatedAt,
			UpdatedAt:   plan.UpdatedAt,
		}
	}
	return result, nil
}

func (s *PurchaseService) getActiveSubscriptionResponses(userID string) ([]*model.UserSubscriptionResponse, error) {
	subs, err := s.subRepo.ListActiveByUserID(userID)
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return []*model.UserSubscriptionResponse{}, nil
	}
	result := make([]*model.UserSubscriptionResponse, len(subs))
	for i, sub := range subs {
		plan, limits, err := s.planRepo.GetByID(sub.PlanID)
		if err != nil {
			return nil, err
		}
		planName := ""
		if plan != nil {
			planName = plan.Name
		}
		result[i] = &model.UserSubscriptionResponse{
			ID:        sub.ID,
			UserID:    sub.UserID,
			PlanID:    sub.PlanID,
			PlanName:  planName,
			StartsAt:  sub.StartsAt,
			ExpiresAt: sub.ExpiresAt,
			Status:    sub.Status,
			Limits:    limits,
			CreatedAt: sub.CreatedAt,
			UpdatedAt: sub.UpdatedAt,
		}
	}
	return result, nil
}

func (s *PurchaseService) toProductResponse(product *model.PurchaseProduct, planName string) *model.PurchaseProductResponse {
	if product == nil {
		return nil
	}
	return &model.PurchaseProductResponse{
		ID:                   product.ID,
		Name:                 product.Name,
		Summary:              product.Summary,
		ProductKind:          product.ProductKind,
		SubscriptionPlanID:   product.SubscriptionPlanID,
		SubscriptionPlanName: planName,
		DurationDays:         product.DurationDays,
		PriceCNYCent:         product.PriceCNYCent,
		LegacySource:         product.LegacySource,
		LegacyRefID:          product.LegacyRefID,
		GroupName:            product.GroupName,
		GroupSort:            product.GroupSort,
		IsRecommended:        product.IsRecommended,
		SortOrder:            product.SortOrder,
		Enabled:              product.Enabled,
		CreatedAt:            product.CreatedAt,
		UpdatedAt:            product.UpdatedAt,
	}
}

func (s *PurchaseService) ensureBalanceTopupPlaceholders() error {
	db := database.GetDB()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO subscription_plans (id, name, description, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO NOTHING`,
		balanceTopupPlanID,
		"余额充值",
		"系统余额充值占位套餐",
		false,
		now,
		now,
	); err != nil {
		return err
	}

	if _, err := db.Exec(
		`INSERT INTO purchase_products (id, name, summary, product_kind, subscription_plan_id, duration_days, price_cny_cent, action_snapshot_json, legacy_source, legacy_ref_id, group_name, group_sort, is_recommended, sort_order, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO NOTHING`,
		balanceTopupProductID,
		"余额充值",
		"系统余额充值占位商品",
		model.PurchaseProductKindBalanceTopup,
		balanceTopupPlanID,
		1,
		1,
		"",
		"",
		"",
		"",
		0,
		false,
		0,
		false,
		now,
		now,
	); err != nil {
		return err
	}

	return nil
}

func newPurchaseOrderNo(now time.Time) string {
	token := strings.ToUpper(strings.ReplaceAll(uuid.New().String(), "-", ""))
	if len(token) > 10 {
		token = token[:10]
	}
	return fmt.Sprintf("PO%s%s", now.UTC().Format("20060102150405"), token)
}

func buildPurchaseQRCodeDataURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	data, err := qrcode.Encode(value, qrcode.Medium, 256)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}
