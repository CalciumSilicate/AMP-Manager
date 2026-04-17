package service

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
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
	ErrPurchaseDisabled          = errors.New("订阅购买功能未开启")
	ErrPaymentUnavailable        = errors.New("支付功能暂不可用")
	ErrPurchaseProductDisabled   = errors.New("该售卖商品已下架")
	ErrPurchaseOrderNotFound     = errors.New("订单不存在")
	ErrPurchaseProductHasOrders  = errors.New("该商品已有订单，无法删除")
	ErrDifferentPlanActive       = errors.New("当前账号已有其他生效中的订阅，暂不支持切换购买")
	ErrPermanentSubscription     = errors.New("当前账号已有永久订阅，无法续费")
	ErrBalanceTopupUnavailable   = errors.New("余额充值未开启")
	ErrInvalidBalanceTopupAmount = errors.New("充值金额无效")
	ErrPendingSubscriptionOrder  = errors.New("当前已有待支付的订阅相关订单，请先处理后再下单")
	ErrDowngradeNotAllowed       = errors.New("当前仅允许购买更高级套餐升级，不允许降级")
	ErrSameRankPlanSwitch        = errors.New("同级别不同套餐不支持直接切换，请选择兑换码交付或联系管理员")
	ErrUpgradeConflict           = errors.New("升级报价已失效，请重新下单")
	ErrUpgradeValuationMissing   = errors.New("套餐升级估值未配置，暂不支持升级")
	ErrBoostRequiresSubscription = errors.New("当前没有可加额的有效订阅")
)

const (
	balanceTopupPlanID    = "system-balance-topup-plan"
	balanceTopupProductID = "system-balance-topup-product"
)

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

	products, err := s.productRepo.List(false)
	if err != nil {
		return nil, err
	}
	plans, err := s.listPlanMap()
	if err != nil {
		return nil, err
	}

	currentSubscription, err := s.getCurrentSubscriptionResponse(userID)
	if err != nil {
		return nil, err
	}

	responses := make([]*model.PurchaseProductResponse, 0, len(products))
	for _, product := range products {
		plan := plans[product.SubscriptionPlanID]
		if plan == nil || !plan.Enabled {
			continue
		}
		responses = append(responses, s.toProductResponse(product, plan.Name, plan.UpgradeRank))
	}

	return &model.PurchaseCatalogResponse{
		PurchaseEnabled:                settings.PurchaseEnabled,
		DebugAutoPaid:                  settings.DebugAutoPaid,
		PaymentConfigured:              s.settingsSvc.CanCreateOrders(settings),
		RenewalRule:                    "显式区分续期、加额、换档与余额充值；礼品码路径不改当前账号",
		CurrentSubscription:            currentSubscription,
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
		return s.quoteSubscriptionOrder(ctx, userID, req.ProductID, req.DeliveryMode, req.CouponCode)
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
		planRank := 0
		if plan := plans[product.SubscriptionPlanID]; plan != nil {
			planRank = plan.UpgradeRank
		}
		responses = append(responses, s.toProductResponse(product, planName, planRank))
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
	if err := s.productRepo.Create(product); err != nil {
		return nil, err
	}
	return s.toProductResponse(product, plan.Name, plan.UpgradeRank), nil
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
	return s.toProductResponse(updated, plan.Name, plan.UpgradeRank), nil
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

	_, planLimits, err := s.planRepo.GetByID(product.SubscriptionPlanID)
	if err != nil {
		return nil, err
	}

	activeSubscription, err := s.subRepo.GetActiveByUserID(userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var upgradeQuote *purchaseUpgradeQuote
	if deliveryMode == model.PurchaseDeliveryModeAccount && product.ProductKind == model.PurchaseProductKindBoostQuota && (activeSubscription == nil || activeSubscription.ExpiresAt == nil) {
		return nil, ErrBoostRequiresSubscription
	}
	if activeSubscription != nil && deliveryMode == model.PurchaseDeliveryModeAccount {
		if product.ProductKind == model.PurchaseProductKindOverwrite {
			upgradeQuote = nil
		} else if product.ProductKind == model.PurchaseProductKindExtendDuration {
			if activeSubscription.PlanID != product.SubscriptionPlanID {
				return nil, ErrDifferentPlanActive
			}
		} else if product.ProductKind == model.PurchaseProductKindBoostQuota {
			upgradeQuote = nil
		} else {
			currentPlan, _, err := s.planRepo.GetByID(activeSubscription.PlanID)
			if err != nil {
				return nil, err
			}
			if currentPlan == nil {
				return nil, ErrPlanNotFound
			}
			if activeSubscription.ExpiresAt == nil && deliveryMode == model.PurchaseDeliveryModeAccount {
				return nil, ErrPermanentSubscription
			}

			switch {
			case plan.UpgradeRank < currentPlan.UpgradeRank:
				return nil, ErrDowngradeNotAllowed
			case plan.UpgradeRank == currentPlan.UpgradeRank && activeSubscription.PlanID != product.SubscriptionPlanID:
				return nil, ErrSameRankPlanSwitch
			case plan.UpgradeRank > currentPlan.UpgradeRank:
				upgradeQuote, err = s.buildUpgradeQuote(ctx, userID, activeSubscription, currentPlan, plan, product, now)
				if err != nil {
					return nil, err
				}
			case activeSubscription.PlanID != product.SubscriptionPlanID:
				return nil, ErrDifferentPlanActive
			}
		}
	}

	actionSnapshotJSON, err := s.buildActionSnapshotJSON(product, planLimits)
	if err != nil {
		return nil, err
	}

	originalAmountCNYCent := product.PriceCNYCent
	upgradeSourcePlanID := ""
	var upgradeSourceExpiresAt *time.Time
	upgradeCreditCNYCent := int64(0)
	upgradeLockedTargetSeconds := int64(0)
	upgradeStateToken := ""
	if upgradeQuote != nil {
		originalAmountCNYCent = upgradeQuote.PayableCNYCent
		upgradeSourcePlanID = upgradeQuote.SourcePlanID
		upgradeSourceExpiresAt = upgradeQuote.SourceExpiresAt
		upgradeCreditCNYCent = upgradeQuote.CreditCNYCent
		upgradeLockedTargetSeconds = upgradeQuote.LockedTargetSeconds
		upgradeStateToken = upgradeQuote.StateToken
	}

	order := &model.PurchaseOrder{
		ID:                      uuid.New().String(),
		OrderNo:                 newPurchaseOrderNo(now),
		UserID:                  userID,
		ProductID:               product.ID,
		SubscriptionPlanID:      product.SubscriptionPlanID,
		DurationDays:            product.DurationDays,
		OriginalAmountCNYCent:   originalAmountCNYCent,
		AmountCNYCent:           originalAmountCNYCent,
		OrderKind:               model.PurchaseOrderKindSubscription,
		DeliveryMode:            deliveryMode,
		PaymentChannel:          model.PaymentChannelAlipay,
		PaymentStatus:           model.PurchasePaymentStatusPending,
		FulfillmentStatus:       model.PurchaseFulfillmentStatusPending,
		UpgradeSourcePlanID:     upgradeSourcePlanID,
		UpgradeSourceExpiresAt:  upgradeSourceExpiresAt,
		UpgradeCreditCNYCent:    upgradeCreditCNYCent,
		UpgradeLockedTargetSecs: upgradeLockedTargetSeconds,
		UpgradeStateToken:       upgradeStateToken,
		ActionSnapshotJSON:      actionSnapshotJSON,
		FailureReason:           "",
		CreatedAt:               now,
		UpdatedAt:               now,
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
		ID:                          balanceTopupProductID,
		Name:                        "余额充值",
		Summary:                     fmt.Sprintf("$%.2f", float64(balanceTopupMicros)/1e6),
		SubscriptionPlanID:          balanceTopupPlanID,
		SubscriptionPlanName:        "余额充值",
		SubscriptionPlanUpgradeRank: 0,
		DurationDays:                1,
		PriceCNYCent:                order.AmountCNYCent,
		GroupName:                   "",
		GroupSort:                   0,
		Enabled:                     false,
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
	activeSubscription, err := s.subRepo.GetActiveByUserID(userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var upgradeQuote *purchaseUpgradeQuote
	if deliveryMode == model.PurchaseDeliveryModeAccount && product.ProductKind == model.PurchaseProductKindBoostQuota && (activeSubscription == nil || activeSubscription.ExpiresAt == nil) {
		return nil, ErrBoostRequiresSubscription
	}
	if activeSubscription != nil && deliveryMode == model.PurchaseDeliveryModeAccount && product.ProductKind == model.PurchaseProductKindSubscription {
		currentPlan, _, err := s.planRepo.GetByID(activeSubscription.PlanID)
		if err != nil {
			return nil, err
		}
		if currentPlan == nil {
			return nil, ErrPlanNotFound
		}
		if activeSubscription.ExpiresAt == nil {
			return nil, ErrPermanentSubscription
		}
		switch {
		case plan.UpgradeRank < currentPlan.UpgradeRank:
			return nil, ErrDowngradeNotAllowed
		case plan.UpgradeRank == currentPlan.UpgradeRank && activeSubscription.PlanID != product.SubscriptionPlanID:
			return nil, ErrSameRankPlanSwitch
		case plan.UpgradeRank > currentPlan.UpgradeRank:
			upgradeQuote, err = s.buildUpgradeQuote(ctx, userID, activeSubscription, currentPlan, plan, product, now)
			if err != nil {
				return nil, err
			}
		case activeSubscription.PlanID != product.SubscriptionPlanID:
			return nil, ErrDifferentPlanActive
		}
	}
	originalAmount := product.PriceCNYCent
	upgradeCredit := int64(0)
	if upgradeQuote != nil {
		originalAmount = upgradeQuote.PayableCNYCent
		upgradeCredit = upgradeQuote.CreditCNYCent
	}
	resp := &model.PurchaseQuoteResponse{
		Kind:                  model.PurchaseOrderKindSubscription,
		ProductID:             product.ID,
		ProductName:           product.Name,
		OrderKindLabel:        "订阅购买",
		DeliveryMode:          deliveryMode,
		OriginalAmountCNYCent: originalAmount,
		UpgradeCreditCNYCent:  upgradeCredit,
		FinalAmountCNYCent:    originalAmount,
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
	applied, err := s.couponSvc.EvaluateCodeTx(tx, userID, couponCode, originalAmount, now)
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

func (s *PurchaseService) buildUpgradeQuote(
	_ context.Context,
	userID string,
	activeSub *model.UserSubscription,
	currentPlan *model.SubscriptionPlan,
	targetPlan *model.SubscriptionPlan,
	product *model.PurchaseProduct,
	now time.Time,
) (*purchaseUpgradeQuote, error) {
	if activeSub == nil || currentPlan == nil || targetPlan == nil || product == nil {
		return nil, nil
	}
	if targetPlan.UpgradeValuationCnyCentPerDay <= 0 {
		return nil, ErrUpgradeValuationMissing
	}

	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := s.ensureLegacyEntitlementsTx(tx, userID, activeSub, currentPlan, now); err != nil {
		return nil, err
	}

	entitlements, err := s.grantSvc.entitlementRepo.ListActiveByUserIDTx(tx, userID, now)
	if err != nil {
		return nil, err
	}

	currentPlanEntitlements := make([]*model.SubscriptionEntitlement, 0, len(entitlements))
	for _, item := range entitlements {
		if item == nil || item.PlanID != activeSub.PlanID {
			continue
		}
		currentPlanEntitlements = append(currentPlanEntitlements, item)
	}

	creditCNYCent := int64(0)
	for _, item := range currentPlanEntitlements {
		creditCNYCent += remainingEntitlementValueCNYCent(item, now)
	}

	productTargetSeconds := int64(product.DurationDays) * 24 * 60 * 60
	lockedTargetSeconds := productTargetSeconds
	if creditCNYCent > 0 {
		convertedSeconds := int64(math.Round(float64(creditCNYCent) * 86400 / float64(targetPlan.UpgradeValuationCnyCentPerDay)))
		if convertedSeconds > lockedTargetSeconds {
			lockedTargetSeconds = convertedSeconds
		}
	}

	payableCNYCent := product.PriceCNYCent - creditCNYCent
	if payableCNYCent < 0 {
		payableCNYCent = 0
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &purchaseUpgradeQuote{
		SourcePlanID:        activeSub.PlanID,
		SourceExpiresAt:     activeSub.ExpiresAt,
		CreditCNYCent:       creditCNYCent,
		LockedTargetSeconds: lockedTargetSeconds,
		PayableCNYCent:      payableCNYCent,
		StateToken:          buildUpgradeStateToken(activeSub, currentPlanEntitlements),
	}, nil
}

func (s *PurchaseService) ensureLegacyEntitlementsTx(
	tx *sql.Tx,
	userID string,
	activeSub *model.UserSubscription,
	currentPlan *model.SubscriptionPlan,
	now time.Time,
) error {
	if tx == nil || activeSub == nil || currentPlan == nil || activeSub.ExpiresAt == nil || !activeSub.ExpiresAt.After(now) {
		return nil
	}
	count, err := s.grantSvc.entitlementRepo.CountByUserTx(tx, userID)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return s.grantSvc.entitlementRepo.CreateTx(tx, &model.SubscriptionEntitlement{
		UserID:                 userID,
		PlanID:                 activeSub.PlanID,
		SourceType:             model.SubscriptionEntitlementSourceLegacySnapshot,
		SourceRefID:            activeSub.ID,
		ValuationCnyCentPerDay: currentPlan.UpgradeValuationCnyCentPerDay,
		StartsAt:               now,
		ExpiresAt:              activeSub.ExpiresAt,
		Status:                 model.SubscriptionEntitlementStatusActive,
	})
}

func buildUpgradeStateToken(activeSub *model.UserSubscription, entitlements []*model.SubscriptionEntitlement) string {
	var builder strings.Builder
	if activeSub != nil {
		builder.WriteString(activeSub.ID)
		builder.WriteByte('|')
		builder.WriteString(activeSub.PlanID)
		builder.WriteByte('|')
		if activeSub.ExpiresAt != nil {
			builder.WriteString(activeSub.ExpiresAt.UTC().Format(time.RFC3339Nano))
		}
	}
	builder.WriteString("::")
	for _, item := range entitlements {
		if item == nil {
			continue
		}
		builder.WriteString(item.ID)
		builder.WriteByte('|')
		builder.WriteString(item.PlanID)
		builder.WriteByte('|')
		builder.WriteString(string(item.Status))
		builder.WriteByte('|')
		builder.WriteString(item.UpdatedAt.UTC().Format(time.RFC3339Nano))
		builder.WriteByte('|')
		if item.ExpiresAt != nil {
			builder.WriteString(item.ExpiresAt.UTC().Format(time.RFC3339Nano))
		}
		builder.WriteByte(',')
	}
	return builder.String()
}

func remainingEntitlementValueCNYCent(item *model.SubscriptionEntitlement, now time.Time) int64 {
	if item == nil || item.ExpiresAt == nil || !item.ExpiresAt.After(now) || item.ValuationCnyCentPerDay <= 0 {
		return 0
	}
	remainingSeconds := item.ExpiresAt.Sub(now).Seconds()
	if remainingSeconds <= 0 {
		return 0
	}
	return int64(math.Round(remainingSeconds * float64(item.ValuationCnyCentPerDay) / 86400))
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
	action, err := decodePurchaseActionSnapshot(order.ActionSnapshotJSON)
	if err != nil {
		return BillingStateSyncAction{}, err
	}
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
		case action != nil && action.ProductKind == model.PurchaseProductKindOverwrite:
			if _, err := s.grantSvc.OverwriteSubscriptionTx(tx, order.UserID, order.SubscriptionPlanID, order.DurationDays, model.SubscriptionEntitlementSourcePurchase, order.OrderNo, now); err != nil {
				return BillingStateSyncAction{}, err
			}
			syncAction = BillingStateSyncAction{UserID: order.UserID, RefreshUserState: true}
		case action != nil && action.ProductKind == model.PurchaseProductKindBoostQuota:
			sub, phases, err := s.grantSvc.CreateBoostTimelineTx(tx, order.UserID, order.SubscriptionPlanID, order.DurationDays, model.SubscriptionEntitlementSourcePurchase, order.OrderNo, now)
			if err != nil {
				return BillingStateSyncAction{}, err
			}
			if err := s.recordRechargeHistoryTx(tx, order, sub, phases, now); err != nil {
				return BillingStateSyncAction{}, err
			}
			syncAction = BillingStateSyncAction{UserID: order.UserID, RefreshUserState: true}
		case order.UpgradeStateToken != "" && order.UpgradeLockedTargetSecs > 0:
			if err := s.applyLockedUpgradeTx(tx, order, now); err != nil {
				if errors.Is(err, ErrUpgradeConflict) || errors.Is(err, ErrPermanentSubscription) {
					_, updateErr := tx.Exec(
						`UPDATE purchase_orders SET fulfillment_status = ?, failure_reason = ?, updated_at = ? WHERE order_no = ?`,
						model.PurchaseFulfillmentStatusFailed,
						err.Error(),
						now,
						order.OrderNo,
					)
					return BillingStateSyncAction{}, updateErr
				}
				return BillingStateSyncAction{}, err
			}
			syncAction = BillingStateSyncAction{
				UserID:           order.UserID,
				RefreshUserState: true,
			}
		default:
			if _, err := s.grantSvc.GrantSubscriptionTxWithSource(
				tx,
				order.UserID,
				order.SubscriptionPlanID,
				order.DurationDays,
				model.SubscriptionEntitlementSourcePurchase,
				order.OrderNo,
				now,
			); err != nil {
				if errors.Is(err, ErrDifferentPlanActive) || errors.Is(err, ErrPermanentSubscription) {
					_, updateErr := tx.Exec(
						`UPDATE purchase_orders SET fulfillment_status = ?, failure_reason = ?, updated_at = ? WHERE order_no = ?`,
						model.PurchaseFulfillmentStatusFailed,
						err.Error(),
						now,
						order.OrderNo,
					)
					return BillingStateSyncAction{}, updateErr
				}
				return BillingStateSyncAction{}, err
			}
			syncAction = BillingStateSyncAction{
				UserID:           order.UserID,
				RefreshUserState: true,
			}
		}
	}

	_, err = tx.Exec(
		`UPDATE purchase_orders
		    SET fulfillment_status = ?, fulfilled_at = COALESCE(fulfilled_at, ?), failure_reason = '', updated_at = ?
		  WHERE order_no = ?`,
		model.PurchaseFulfillmentStatusFulfilled,
		now,
		now,
		order.OrderNo,
	)
	if err != nil {
		return BillingStateSyncAction{}, err
	}
	return syncAction, nil
}

func (s *PurchaseService) applyLockedUpgradeTx(tx *sql.Tx, order *model.PurchaseOrder, now time.Time) error {
	activeSub, err := s.grantSvc.getActiveSubscriptionTx(tx, order.UserID, now)
	if err != nil {
		return err
	}
	if activeSub == nil || activeSub.ExpiresAt == nil || activeSub.PlanID != order.UpgradeSourcePlanID {
		return ErrUpgradeConflict
	}

	currentPlan, _, err := s.planRepo.GetByID(activeSub.PlanID)
	if err != nil {
		return err
	}
	if currentPlan == nil {
		return ErrPlanNotFound
	}
	if err := s.ensureLegacyEntitlementsTx(tx, order.UserID, activeSub, currentPlan, now); err != nil {
		return err
	}

	entitlements, err := s.grantSvc.entitlementRepo.ListActiveByUserIDTx(tx, order.UserID, now)
	if err != nil {
		return err
	}
	currentPlanEntitlements := make([]*model.SubscriptionEntitlement, 0, len(entitlements))
	for _, item := range entitlements {
		if item == nil || item.PlanID != activeSub.PlanID {
			continue
		}
		currentPlanEntitlements = append(currentPlanEntitlements, item)
	}
	if buildUpgradeStateToken(activeSub, currentPlanEntitlements) != order.UpgradeStateToken {
		return ErrUpgradeConflict
	}

	targetPlan, _, err := s.planRepo.GetByID(order.SubscriptionPlanID)
	if err != nil {
		return err
	}
	if targetPlan == nil {
		return ErrPlanNotFound
	}

	if err := s.grantSvc.entitlementRepo.ConsumeActiveByUserPlanTx(tx, order.UserID, activeSub.PlanID, order.OrderNo, now); err != nil {
		return err
	}

	newExpiry := now.Add(time.Duration(order.UpgradeLockedTargetSecs) * time.Second)
	if _, err := tx.Exec(
		`UPDATE user_subscriptions SET plan_id = ?, starts_at = ?, expires_at = ?, updated_at = ? WHERE id = ?`,
		order.SubscriptionPlanID,
		now,
		newExpiry,
		now,
		activeSub.ID,
	); err != nil {
		return err
	}

	return s.grantSvc.entitlementRepo.CreateTx(tx, &model.SubscriptionEntitlement{
		UserID:                 order.UserID,
		PlanID:                 order.SubscriptionPlanID,
		SourceType:             model.SubscriptionEntitlementSourceUpgrade,
		SourceRefID:            order.OrderNo,
		ValuationCnyCentPerDay: targetPlan.UpgradeValuationCnyCentPerDay,
		StartsAt:               now,
		ExpiresAt:              &newExpiry,
		Status:                 model.SubscriptionEntitlementStatusActive,
	})
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
		order.ActionSnapshotJSON,
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
		`SELECT id, order_no, user_id, product_id, subscription_plan_id, duration_days, original_amount_cny_cent, discount_cny_cent, amount_cny_cent, order_kind, delivery_mode, balance_topup_micros, payment_channel, payment_status,
		        fulfillment_status, generated_redeem_code_id, upgrade_source_plan_id, upgrade_source_expires_at, upgrade_credit_cny_cent, upgrade_locked_target_seconds, upgrade_state_token,
		        coupon_campaign_id, coupon_campaign_name, coupon_code_id, coupon_code_value, coupon_discount_type, coupon_percent_off_bps, coupon_fixed_discount_cny_cent, coupon_max_discount_cny_cent,
		        manual_settlement_done, alipay_trade_no, alipay_qr_code, alipay_qr_url, expires_at, paid_at, fulfilled_at, failure_reason,
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
		&order.PaymentChannel,
		&order.PaymentStatus,
		&order.FulfillmentStatus,
		&order.GeneratedRedeemCodeID,
		&order.UpgradeSourcePlanID,
		&order.UpgradeSourceExpiresAt,
		&order.UpgradeCreditCNYCent,
		&order.UpgradeLockedTargetSecs,
		&order.UpgradeStateToken,
		&order.CouponCampaignID,
		&order.CouponCampaignName,
		&order.CouponCodeID,
		&order.CouponCodeValue,
		&order.CouponDiscountType,
		&order.CouponPercentOffBPS,
		&order.CouponFixedDiscountCNYCent,
		&order.CouponMaxDiscountCNYCent,
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

	return product, plan, s.toProductResponse(product, plan.Name, plan.UpgradeRank), nil
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
			ID:                            plan.ID,
			Name:                          plan.Name,
			Description:                   plan.Description,
			Enabled:                       plan.Enabled,
			UpgradeRank:                   plan.UpgradeRank,
			UpgradeValuationCnyCentPerDay: plan.UpgradeValuationCnyCentPerDay,
			CreatedAt:                     plan.CreatedAt,
			UpdatedAt:                     plan.UpdatedAt,
		}
	}
	return result, nil
}

func (s *PurchaseService) getCurrentSubscriptionResponse(userID string) (*model.UserSubscriptionResponse, error) {
	sub, err := s.subRepo.GetActiveByUserID(userID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, nil
	}
	plan, limits, err := s.planRepo.GetByID(sub.PlanID)
	if err != nil {
		return nil, err
	}
	planName := ""
	if plan != nil {
		planName = plan.Name
	}
	return &model.UserSubscriptionResponse{
		ID:       sub.ID,
		UserID:   sub.UserID,
		PlanID:   sub.PlanID,
		PlanName: planName,
		PlanUpgradeRank: func() int {
			if plan != nil {
				return plan.UpgradeRank
			}
			return 0
		}(),
		StartsAt:  sub.StartsAt,
		ExpiresAt: sub.ExpiresAt,
		Status:    sub.Status,
		Limits:    limits,
		CreatedAt: sub.CreatedAt,
		UpdatedAt: sub.UpdatedAt,
	}, nil
}

func (s *PurchaseService) toProductResponse(product *model.PurchaseProduct, planName string, planRank int) *model.PurchaseProductResponse {
	if product == nil {
		return nil
	}
	return &model.PurchaseProductResponse{
		ID:                          product.ID,
		Name:                        product.Name,
		Summary:                     product.Summary,
		ProductKind:                 product.ProductKind,
		SubscriptionPlanID:          product.SubscriptionPlanID,
		SubscriptionPlanName:        planName,
		SubscriptionPlanUpgradeRank: planRank,
		DurationDays:                product.DurationDays,
		PriceCNYCent:                product.PriceCNYCent,
		ActionSnapshotJSON:          product.ActionSnapshotJSON,
		LegacySource:                product.LegacySource,
		LegacyRefID:                 product.LegacyRefID,
		GroupName:                   product.GroupName,
		GroupSort:                   product.GroupSort,
		IsRecommended:               product.IsRecommended,
		SortOrder:                   product.SortOrder,
		Enabled:                     product.Enabled,
		CreatedAt:                   product.CreatedAt,
		UpdatedAt:                   product.UpdatedAt,
	}
}

func (s *PurchaseService) buildActionSnapshotJSON(product *model.PurchaseProduct, limits []model.SubscriptionPlanLimit) (string, error) {
	if strings.TrimSpace(product.ActionSnapshotJSON) != "" {
		return product.ActionSnapshotJSON, nil
	}
	snapshot := &model.PurchaseActionSnapshot{
		ProductKind:  product.ProductKind,
		PlanID:       product.SubscriptionPlanID,
		DurationDays: product.DurationDays,
	}
	for _, limit := range limits {
		switch limit.LimitType {
		case model.LimitTypeDaily:
			snapshot.SourceDailyLimitMicros = limit.LimitMicros
			snapshot.FixedResetTime = limit.FixedResetTime
		case model.LimitTypeWeekly:
			snapshot.SourceWeeklyLimitMicros = limit.LimitMicros
		case model.LimitTypeMonthly:
			snapshot.SourceMonthlyLimitMicros = limit.LimitMicros
		case model.LimitTypeRolling5h:
			snapshot.SourceRolling5hMicros = limit.LimitMicros
		case model.LimitTypeTotal:
			snapshot.SourceTotalLimitMicros = limit.LimitMicros
		}
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodePurchaseActionSnapshot(raw string) (*model.PurchaseActionSnapshot, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var snapshot model.PurchaseActionSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (s *PurchaseService) recordRechargeHistoryTx(
	tx *sql.Tx,
	order *model.PurchaseOrder,
	sub *model.UserSubscription,
	phases []*model.SubscriptionTimelinePhase,
	now time.Time,
) error {
	if tx == nil || order == nil || sub == nil || s.runtimeRepo == nil {
		return nil
	}
	var targetDailyBefore *int64
	var peakDaily *int64
	if action, err := decodePurchaseActionSnapshot(order.ActionSnapshotJSON); err == nil && action != nil {
		if action.SourceDailyLimitMicros > 0 {
			value := action.SourceDailyLimitMicros
			targetDailyBefore = &value
		}
	}
	if len(phases) > 0 && phases[0] != nil && phases[0].DailyLimitMicros != nil {
		value := *phases[0].DailyLimitMicros
		peakDaily = &value
	}
	return s.runtimeRepo.CreateRechargeHistoryTx(tx, &model.SubscriptionRechargeHistory{
		UserID:                       order.UserID,
		UserSubscriptionID:           sub.ID,
		PlanID:                       order.SubscriptionPlanID,
		Mode:                         string(model.PurchaseProductKindBoostQuota),
		Status:                       "completed",
		SourceType:                   "purchase_order",
		SourceRefID:                  order.OrderNo,
		TargetDailyLimitBeforeMicros: targetDailyBefore,
		PeakDailyLimitMicros:         peakDaily,
		TargetExpiresAtBefore:        order.UpgradeSourceExpiresAt,
		TargetExpiresAtAfter:         sub.ExpiresAt,
		PreviewJSON:                  order.TimelinePreviewJSON,
		ConfirmedAt:                  &now,
		AppliedAt:                    &now,
	})
}

func (s *PurchaseService) ensureBalanceTopupPlaceholders() error {
	db := database.GetDB()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO subscription_plans (id, name, description, enabled, upgrade_rank, upgrade_valuation_cny_cent_per_day, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO NOTHING`,
		balanceTopupPlanID,
		"余额充值",
		"系统余额充值占位套餐",
		false,
		0,
		0,
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
