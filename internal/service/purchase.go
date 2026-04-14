package service

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"
)

var (
	ErrPurchaseDisabled         = errors.New("订阅购买功能未开启")
	ErrPaymentUnavailable       = errors.New("支付功能暂不可用")
	ErrPurchaseProductDisabled  = errors.New("该售卖商品已下架")
	ErrPurchaseOrderNotFound    = errors.New("订单不存在")
	ErrPurchaseProductHasOrders = errors.New("该商品已有订单，无法删除")
	ErrDifferentPlanActive      = errors.New("当前账号已有其他生效中的订阅，暂不支持切换购买")
	ErrPermanentSubscription    = errors.New("当前账号已有永久订阅，无法续费")
)

type purchasePaymentGateway interface {
	CreateOrder(ctx context.Context, order *model.PurchaseOrder, product *model.PurchaseProductResponse, username string) (*PaymentCreateResult, error)
	QueryOrder(ctx context.Context, orderNo string) (*PaymentQueryResult, error)
	DecodeNotification(ctx context.Context, values url.Values) (*PaymentNotification, error)
}

type PurchaseService struct {
	productRepo repository.PurchaseProductRepositoryInterface
	orderRepo   repository.PurchaseOrderRepositoryInterface
	planRepo    repository.SubscriptionPlanRepositoryInterface
	subRepo     repository.UserSubscriptionRepositoryInterface
	settingsSvc *PurchaseSettingsService
	payment     purchasePaymentGateway
}

func NewPurchaseService() *PurchaseService {
	return &PurchaseService{
		productRepo: repository.NewPurchaseProductRepository(),
		orderRepo:   repository.NewPurchaseOrderRepository(),
		planRepo:    repository.NewSubscriptionPlanRepository(),
		subRepo:     repository.NewUserSubscriptionRepository(),
		settingsSvc: NewPurchaseSettingsService(),
		payment:     NewAlipayService(),
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
		planRepo:    planRepo,
		subRepo:     subRepo,
		settingsSvc: settingsSvc,
		payment:     payment,
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
		responses = append(responses, s.toProductResponse(product, plan.Name))
	}

	return &model.PurchaseCatalogResponse{
		PurchaseEnabled:     settings.PurchaseEnabled,
		DebugAutoPaid:       settings.DebugAutoPaid,
		PaymentConfigured:   s.settingsSvc.CanCreateOrders(settings),
		RenewalRule:         "同套餐续期，不同套餐不可购买",
		CurrentSubscription: currentSubscription,
		Products:            responses,
	}, nil
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
		SubscriptionPlanID: req.SubscriptionPlanID,
		DurationDays:       req.DurationDays,
		PriceCNYCent:       req.PriceCNYCent,
		IsRecommended:      req.IsRecommended,
		SortOrder:          req.SortOrder,
		Enabled:            req.Enabled,
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
		SubscriptionPlanID: req.SubscriptionPlanID,
		DurationDays:       req.DurationDays,
		PriceCNYCent:       req.PriceCNYCent,
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

func (s *PurchaseService) CreateOrder(ctx context.Context, userID, username, productID string) (*model.PurchaseOrderResponse, error) {
	settings, err := s.settingsSvc.Get()
	if err != nil {
		return nil, err
	}
	if !settings.PurchaseEnabled {
		return nil, ErrPurchaseDisabled
	}
	if !s.settingsSvc.CanCreateOrders(settings) {
		return nil, ErrPaymentUnavailable
	}

	product, plan, productResp, err := s.loadPurchasableProduct(productID)
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
	if activeSubscription != nil {
		if activeSubscription.PlanID != product.SubscriptionPlanID {
			return nil, ErrDifferentPlanActive
		}
		if activeSubscription.ExpiresAt == nil {
			return nil, ErrPermanentSubscription
		}
	}

	now := time.Now().UTC()
	order := &model.PurchaseOrder{
		ID:                 uuid.New().String(),
		OrderNo:            newPurchaseOrderNo(now),
		UserID:             userID,
		ProductID:          product.ID,
		SubscriptionPlanID: product.SubscriptionPlanID,
		DurationDays:       product.DurationDays,
		AmountCNYCent:      product.PriceCNYCent,
		PaymentChannel:     model.PaymentChannelAlipay,
		PaymentStatus:      model.PurchasePaymentStatusPending,
		FulfillmentStatus:  model.PurchaseFulfillmentStatusPending,
		FailureReason:      "",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.orderRepo.Create(order); err != nil {
		return nil, err
	}

	if settings.DebugAutoPaid {
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
	order, err := s.orderRepo.GetDetailByOrderNoForUser(orderNo, userID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrPurchaseOrderNotFound
	}
	return s.refreshOrder(ctx, order)
}

func (s *PurchaseService) RefreshOrderAdmin(ctx context.Context, orderNo string) (*model.PurchaseOrderResponse, error) {
	order, err := s.orderRepo.GetDetailByOrderNo(orderNo)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrPurchaseOrderNotFound
	}
	return s.refreshOrder(ctx, order)
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

	if err := s.fulfillOrderTx(tx, order, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
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

func (s *PurchaseService) fulfillOrderTx(tx *sql.Tx, order *model.PurchaseOrder, now time.Time) error {
	if order.FulfillmentStatus == model.PurchaseFulfillmentStatusFulfilled {
		return nil
	}

	activeSub, err := s.getActiveSubscriptionTx(tx, order.UserID)
	if err != nil {
		return err
	}

	var failureReason string
	if activeSub != nil {
		if activeSub.PlanID != order.SubscriptionPlanID {
			failureReason = ErrDifferentPlanActive.Error()
		} else if activeSub.ExpiresAt == nil {
			failureReason = ErrPermanentSubscription.Error()
		}
	}

	if failureReason != "" {
		_, err := tx.Exec(
			`UPDATE purchase_orders SET fulfillment_status = ?, failure_reason = ?, updated_at = ? WHERE order_no = ?`,
			model.PurchaseFulfillmentStatusFailed,
			failureReason,
			now,
			order.OrderNo,
		)
		return err
	}

	if activeSub == nil {
		expiresAt := now.AddDate(0, 0, order.DurationDays)
		_, err := tx.Exec(
			`INSERT INTO user_subscriptions (id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.New().String(),
			order.UserID,
			order.SubscriptionPlanID,
			now,
			expiresAt,
			model.SubscriptionStatusActive,
			now,
			now,
		)
		if err != nil {
			return err
		}
	} else {
		base := now
		if activeSub.ExpiresAt != nil && activeSub.ExpiresAt.After(now) {
			base = activeSub.ExpiresAt.UTC()
		}
		expiresAt := base.AddDate(0, 0, order.DurationDays)
		if _, err := tx.Exec(
			`UPDATE user_subscriptions SET expires_at = ?, updated_at = ? WHERE id = ?`,
			expiresAt,
			now,
			activeSub.ID,
		); err != nil {
			return err
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
	return err
}

func (s *PurchaseService) getActiveSubscriptionTx(tx *sql.Tx, userID string) (*model.UserSubscription, error) {
	sub := &model.UserSubscription{}
	err := tx.QueryRow(
		`SELECT id, user_id, plan_id, starts_at, expires_at, status, created_at, updated_at
		   FROM user_subscriptions
		  WHERE user_id = ? AND status = 'active' AND (expires_at IS NULL OR expires_at > ?)`,
		userID,
		time.Now().UTC(),
	).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.PlanID,
		&sub.StartsAt,
		&sub.ExpiresAt,
		&sub.Status,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

func (s *PurchaseService) getOrderByOrderNoTx(tx *sql.Tx, orderNo string) (*model.PurchaseOrder, error) {
	order := &model.PurchaseOrder{}
	err := tx.QueryRow(
		`SELECT id, order_no, user_id, product_id, subscription_plan_id, duration_days, amount_cny_cent, payment_channel, payment_status,
		        fulfillment_status, alipay_trade_no, alipay_qr_code, alipay_qr_url, expires_at, paid_at, fulfilled_at, failure_reason,
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
		&order.AmountCNYCent,
		&order.PaymentChannel,
		&order.PaymentStatus,
		&order.FulfillmentStatus,
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
	_, err := db.Exec(
		`UPDATE purchase_orders SET payment_status = ?, failure_reason = ?, updated_at = ? WHERE order_no = ?`,
		model.PurchasePaymentStatusFailed,
		strings.TrimSpace(reason),
		time.Now().UTC(),
		orderNo,
	)
	return err
}

func (s *PurchaseService) markOrderExpired(orderNo string) error {
	db := database.GetDB()
	_, err := db.Exec(
		`UPDATE purchase_orders
		    SET payment_status = CASE WHEN payment_status = ? THEN ? ELSE payment_status END,
		        updated_at = ?
		  WHERE order_no = ?`,
		model.PurchasePaymentStatusPending,
		model.PurchasePaymentStatusExpired,
		time.Now().UTC(),
		orderNo,
	)
	return err
}

func (s *PurchaseService) markOrderClosed(orderNo, tradeNo string, expired bool) error {
	db := database.GetDB()
	status := model.PurchasePaymentStatusClosed
	if expired {
		status = model.PurchasePaymentStatusExpired
	}
	_, err := db.Exec(
		`UPDATE purchase_orders
		    SET payment_status = ?, alipay_trade_no = CASE WHEN ? <> '' THEN ? ELSE alipay_trade_no END, updated_at = ?
		  WHERE order_no = ? AND payment_status = ?`,
		status,
		tradeNo,
		tradeNo,
		time.Now().UTC(),
		orderNo,
		model.PurchasePaymentStatusPending,
	)
	return err
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
	}, nil
}

func (s *PurchaseService) toProductResponse(product *model.PurchaseProduct, planName string) *model.PurchaseProductResponse {
	if product == nil {
		return nil
	}
	return &model.PurchaseProductResponse{
		ID:                   product.ID,
		Name:                 product.Name,
		Summary:              product.Summary,
		SubscriptionPlanID:   product.SubscriptionPlanID,
		SubscriptionPlanName: planName,
		DurationDays:         product.DurationDays,
		PriceCNYCent:         product.PriceCNYCent,
		IsRecommended:        product.IsRecommended,
		SortOrder:            product.SortOrder,
		Enabled:              product.Enabled,
		CreatedAt:            product.CreatedAt,
		UpdatedAt:            product.UpdatedAt,
	}
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
