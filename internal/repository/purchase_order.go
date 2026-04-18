package repository

import (
	"database/sql"
	"strings"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

type PurchaseOrderRepositoryInterface interface {
	Create(order *model.PurchaseOrder) error
	CreateTx(tx *sql.Tx, order *model.PurchaseOrder) error
	GetByOrderNo(orderNo string) (*model.PurchaseOrder, error)
	GetDetailByOrderNo(orderNo string) (*model.PurchaseOrderResponse, error)
	GetDetailByOrderNoForUser(orderNo, userID string) (*model.PurchaseOrderResponse, error)
	ListByUser(userID string, limit int) ([]*model.PurchaseOrderResponse, int64, error)
	ListAdmin(filters model.PurchaseOrderFilters) ([]*model.PurchaseOrderResponse, int64, error)
	ListAdminOrderNos(filters model.PurchaseOrderFilters) ([]string, error)
	SumAdminAmount(filters model.PurchaseOrderFilters) (int64, error)
	CountByProductID(productID string) (int64, error)
	CountPendingSubscriptionOrdersByUser(userID string) (int64, error)
}

var _ PurchaseOrderRepositoryInterface = (*PurchaseOrderRepository)(nil)

type PurchaseOrderRepository struct{}

func NewPurchaseOrderRepository() *PurchaseOrderRepository {
	return &PurchaseOrderRepository{}
}

func (r *PurchaseOrderRepository) Create(order *model.PurchaseOrder) error {
	db := database.GetDB()
	return r.createWithExec(db, order)
}

func (r *PurchaseOrderRepository) CreateTx(tx *sql.Tx, order *model.PurchaseOrder) error {
	return r.createWithExec(tx, order)
}

func (r *PurchaseOrderRepository) createWithExec(exec interface {
	Exec(query string, args ...any) (sql.Result, error)
}, order *model.PurchaseOrder) error {
	_, err := exec.Exec(
		`INSERT INTO purchase_orders
		 (id, order_no, user_id, product_id, subscription_plan_id, duration_days, original_amount_cny_cent, discount_cny_cent, amount_cny_cent, order_kind, delivery_mode, balance_topup_micros, payment_channel, payment_status, fulfillment_status,
		  coupon_campaign_id, coupon_campaign_name, coupon_code_id, coupon_code_value, coupon_discount_type, coupon_percent_off_bps, coupon_fixed_discount_cny_cent, coupon_max_discount_cny_cent,
		  generated_redeem_code_id, legacy_source, legacy_ref_id,
		  manual_settlement_done, alipay_trade_no, alipay_qr_code, alipay_qr_url, expires_at, paid_at, fulfilled_at, failure_reason, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		order.ID,
		order.OrderNo,
		order.UserID,
		order.ProductID,
		order.SubscriptionPlanID,
		order.DurationDays,
		order.OriginalAmountCNYCent,
		order.DiscountCNYCent,
		order.AmountCNYCent,
		order.OrderKind,
		order.DeliveryMode,
		order.BalanceTopupMicros,
		order.PaymentChannel,
		order.PaymentStatus,
		order.FulfillmentStatus,
		order.CouponCampaignID,
		order.CouponCampaignName,
		order.CouponCodeID,
		order.CouponCodeValue,
		order.CouponDiscountType,
		order.CouponPercentOffBPS,
		order.CouponFixedDiscountCNYCent,
		order.CouponMaxDiscountCNYCent,
		order.GeneratedRedeemCodeID,
		order.LegacySource,
		order.LegacyRefID,
		order.ManualSettlementDone,
		order.AlipayTradeNo,
		order.AlipayQRCode,
		order.AlipayQRURL,
		order.ExpiresAt,
		order.PaidAt,
		order.FulfilledAt,
		order.FailureReason,
		order.CreatedAt,
		order.UpdatedAt,
	)
	return err
}

func (r *PurchaseOrderRepository) GetByOrderNo(orderNo string) (*model.PurchaseOrder, error) {
	db := database.GetDB()
	return r.getOrderByQuery(
		db.QueryRow(
			`SELECT id, order_no, user_id, product_id, subscription_plan_id, duration_days, original_amount_cny_cent, discount_cny_cent, amount_cny_cent, order_kind, delivery_mode, balance_topup_micros, payment_channel, payment_status,
			        fulfillment_status,
			        coupon_campaign_id, coupon_campaign_name, coupon_code_id, coupon_code_value, coupon_discount_type, coupon_percent_off_bps, coupon_fixed_discount_cny_cent, coupon_max_discount_cny_cent,
			        generated_redeem_code_id, legacy_source, legacy_ref_id,
			        manual_settlement_done, alipay_trade_no, alipay_qr_code, alipay_qr_url, expires_at, paid_at, fulfilled_at, failure_reason,
			        created_at, updated_at
			   FROM purchase_orders
			  WHERE order_no = ?`,
			orderNo,
		),
	)
}

func (r *PurchaseOrderRepository) GetDetailByOrderNo(orderNo string) (*model.PurchaseOrderResponse, error) {
	db := database.GetDB()
	return r.getOrderDetailByQuery(
		db.QueryRow(
			r.detailSelectSQL()+` WHERE o.order_no = ?`,
			orderNo,
		),
	)
}

func (r *PurchaseOrderRepository) GetDetailByOrderNoForUser(orderNo, userID string) (*model.PurchaseOrderResponse, error) {
	db := database.GetDB()
	return r.getOrderDetailByQuery(
		db.QueryRow(
			r.detailSelectSQL()+` WHERE o.order_no = ? AND o.user_id = ?`,
			orderNo,
			userID,
		),
	)
}

func (r *PurchaseOrderRepository) ListByUser(userID string, limit int) ([]*model.PurchaseOrderResponse, int64, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 20
	}

	var total int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM purchase_orders WHERE user_id = ?`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(
		r.detailSelectSQL()+` WHERE o.user_id = ? ORDER BY o.created_at DESC LIMIT ?`,
		userID,
		limit,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items, err := r.scanOrderDetails(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *PurchaseOrderRepository) ListAdmin(filters model.PurchaseOrderFilters) ([]*model.PurchaseOrderResponse, int64, error) {
	db := database.GetDB()
	where, args := buildPurchaseOrderAdminWhere(filters)

	var total int64
	if err := db.QueryRow(
		`SELECT COUNT(*)
		   FROM purchase_orders o
		   INNER JOIN users u ON u.id = o.user_id`+where,
		args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := filters.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}
	queryArgs := append([]interface{}{}, args...)
	queryArgs = append(queryArgs, limit, offset)
	rows, err := db.Query(
		r.detailSelectSQL()+where+` ORDER BY o.created_at DESC LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items, err := r.scanOrderDetails(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *PurchaseOrderRepository) ListAdminOrderNos(filters model.PurchaseOrderFilters) ([]string, error) {
	db := database.GetDB()
	where, args := buildPurchaseOrderAdminWhere(filters)

	rows, err := db.Query(
		`SELECT o.order_no
		   FROM purchase_orders o
		   INNER JOIN users u ON u.id = o.user_id`+where+` ORDER BY COALESCE(o.paid_at, o.created_at) ASC, o.order_no ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orderNos := make([]string, 0)
	for rows.Next() {
		var orderNo string
		if err := rows.Scan(&orderNo); err != nil {
			return nil, err
		}
		orderNos = append(orderNos, orderNo)
	}
	return orderNos, rows.Err()
}

func (r *PurchaseOrderRepository) SumAdminAmount(filters model.PurchaseOrderFilters) (int64, error) {
	db := database.GetDB()
	where, args := buildPurchaseOrderAdminWhere(filters)

	var totalAmount int64
	err := db.QueryRow(
		`SELECT COALESCE(SUM(o.amount_cny_cent), 0)
		   FROM purchase_orders o
		   INNER JOIN users u ON u.id = o.user_id`+where,
		args...,
	).Scan(&totalAmount)
	return totalAmount, err
}

func (r *PurchaseOrderRepository) CountByProductID(productID string) (int64, error) {
	db := database.GetDB()
	var count int64
	err := db.QueryRow(`SELECT COUNT(*) FROM purchase_orders WHERE product_id = ?`, productID).Scan(&count)
	return count, err
}

func (r *PurchaseOrderRepository) CountPendingSubscriptionOrdersByUser(userID string) (int64, error) {
	db := database.GetDB()
	var count int64
	err := db.QueryRow(
		`SELECT COUNT(*)
		   FROM purchase_orders
		  WHERE user_id = ? AND order_kind = ? AND payment_status = ?`,
		userID,
		model.PurchaseOrderKindSubscription,
		model.PurchasePaymentStatusPending,
	).Scan(&count)
	return count, err
}

func buildPurchaseOrderAdminWhere(filters model.PurchaseOrderFilters) (string, []interface{}) {
	conditions := make([]string, 0)
	args := make([]interface{}, 0)

	if filters.PaymentStatus != "" {
		conditions = append(conditions, "o.payment_status = ?")
		args = append(args, filters.PaymentStatus)
	}
	if filters.FulfillmentStatus != "" {
		conditions = append(conditions, "o.fulfillment_status = ?")
		args = append(args, filters.FulfillmentStatus)
	}
	if productID := strings.TrimSpace(filters.ProductID); productID != "" {
		conditions = append(conditions, "o.product_id = ?")
		args = append(args, productID)
	}
	if username := strings.TrimSpace(filters.Username); username != "" {
		conditions = append(conditions, "u.username LIKE ?")
		args = append(args, "%"+username+"%")
	}
	if filters.PaidFrom != nil {
		conditions = append(conditions, "o.paid_at >= ?")
		args = append(args, filters.PaidFrom.UTC())
	}
	if filters.PaidTo != nil {
		conditions = append(conditions, "o.paid_at <= ?")
		args = append(args, filters.PaidTo.UTC())
	}
	if filters.ManualSettlementDone != nil {
		conditions = append(conditions, "o.manual_settlement_done = ?")
		args = append(args, *filters.ManualSettlementDone)
	}
	if filters.EligibleForManualSettlement {
		conditions = append(conditions, "o.payment_status IN (?, ?)")
		args = append(args, model.PurchasePaymentStatusPaid, model.PurchasePaymentStatusRefunded)
	}

	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func (r *PurchaseOrderRepository) getOrderByQuery(row *sql.Row) (*model.PurchaseOrder, error) {
	order := &model.PurchaseOrder{}
	err := row.Scan(
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
		&order.CouponCampaignID,
		&order.CouponCampaignName,
		&order.CouponCodeID,
		&order.CouponCodeValue,
		&order.CouponDiscountType,
		&order.CouponPercentOffBPS,
		&order.CouponFixedDiscountCNYCent,
		&order.CouponMaxDiscountCNYCent,
		&order.GeneratedRedeemCodeID,
		&order.LegacySource,
		&order.LegacyRefID,
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

func (r *PurchaseOrderRepository) detailSelectSQL() string {
	return `SELECT o.id, o.order_no, o.user_id, u.username, o.product_id, p.name, p.summary, o.subscription_plan_id, sp.name,
	               o.duration_days, o.original_amount_cny_cent, o.discount_cny_cent, o.amount_cny_cent, o.order_kind, o.delivery_mode, o.balance_topup_micros, o.payment_channel, o.payment_status, o.fulfillment_status,
	               o.coupon_campaign_id, o.coupon_campaign_name, o.coupon_code_id, o.coupon_code_value, o.coupon_discount_type, o.coupon_percent_off_bps, o.coupon_fixed_discount_cny_cent, o.coupon_max_discount_cny_cent,
	               o.legacy_source, o.legacy_ref_id,
	               o.generated_redeem_code_id, COALESCE(generated_code.code_value, ''), COALESCE(generated_code.code_mask, ''), COALESCE(generated_code.status, ''), generated_code.last_redeemed_at,
	               o.manual_settlement_done, o.alipay_trade_no, o.alipay_qr_code, o.alipay_qr_url, o.expires_at, o.paid_at, o.fulfilled_at,
	               o.failure_reason, o.created_at, o.updated_at
	          FROM purchase_orders o
	          INNER JOIN users u ON u.id = o.user_id
	          INNER JOIN purchase_products p ON p.id = o.product_id
	          INNER JOIN subscription_plans sp ON sp.id = o.subscription_plan_id
	          LEFT JOIN redeem_codes generated_code ON generated_code.id = NULLIF(o.generated_redeem_code_id, '')`
}

func (r *PurchaseOrderRepository) getOrderDetailByQuery(row *sql.Row) (*model.PurchaseOrderResponse, error) {
	order := &model.PurchaseOrderResponse{}
	err := row.Scan(
		&order.ID,
		&order.OrderNo,
		&order.UserID,
		&order.Username,
		&order.ProductID,
		&order.ProductName,
		&order.ProductSummary,
		&order.SubscriptionPlanID,
		&order.SubscriptionPlanName,
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
		&order.CouponCampaignID,
		&order.CouponCampaignName,
		&order.CouponCodeID,
		&order.CouponCodeValue,
		&order.CouponDiscountType,
		&order.CouponPercentOffBPS,
		&order.CouponFixedDiscountCNYCent,
		&order.CouponMaxDiscountCNYCent,
		&order.LegacySource,
		&order.LegacyRefID,
		&order.GeneratedRedeemCodeID,
		&order.GeneratedRedeemCode,
		&order.GeneratedRedeemCodeMask,
		&order.GeneratedRedeemCodeStatus,
		&order.GeneratedRedeemedAt,
		&order.ManualSettlementDone,
		&order.AlipayTradeNo,
		&order.PaymentQRCode,
		&order.PaymentQRURL,
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

func (r *PurchaseOrderRepository) scanOrderDetails(rows *sql.Rows) ([]*model.PurchaseOrderResponse, error) {
	items := make([]*model.PurchaseOrderResponse, 0)
	for rows.Next() {
		order := &model.PurchaseOrderResponse{}
		if err := rows.Scan(
			&order.ID,
			&order.OrderNo,
			&order.UserID,
			&order.Username,
			&order.ProductID,
			&order.ProductName,
			&order.ProductSummary,
			&order.SubscriptionPlanID,
			&order.SubscriptionPlanName,
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
			&order.CouponCampaignID,
			&order.CouponCampaignName,
			&order.CouponCodeID,
			&order.CouponCodeValue,
			&order.CouponDiscountType,
			&order.CouponPercentOffBPS,
			&order.CouponFixedDiscountCNYCent,
			&order.CouponMaxDiscountCNYCent,
			&order.LegacySource,
			&order.LegacyRefID,
			&order.GeneratedRedeemCodeID,
			&order.GeneratedRedeemCode,
			&order.GeneratedRedeemCodeMask,
			&order.GeneratedRedeemCodeStatus,
			&order.GeneratedRedeemedAt,
			&order.ManualSettlementDone,
			&order.AlipayTradeNo,
			&order.PaymentQRCode,
			&order.PaymentQRURL,
			&order.ExpiresAt,
			&order.PaidAt,
			&order.FulfilledAt,
			&order.FailureReason,
			&order.CreatedAt,
			&order.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, order)
	}
	return items, rows.Err()
}
