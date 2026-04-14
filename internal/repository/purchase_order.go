package repository

import (
	"database/sql"
	"strings"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

type PurchaseOrderRepositoryInterface interface {
	Create(order *model.PurchaseOrder) error
	GetByOrderNo(orderNo string) (*model.PurchaseOrder, error)
	GetDetailByOrderNo(orderNo string) (*model.PurchaseOrderResponse, error)
	GetDetailByOrderNoForUser(orderNo, userID string) (*model.PurchaseOrderResponse, error)
	ListByUser(userID string, limit int) ([]*model.PurchaseOrderResponse, int64, error)
	ListAdmin(filters model.PurchaseOrderFilters) ([]*model.PurchaseOrderResponse, int64, error)
	CountByProductID(productID string) (int64, error)
}

var _ PurchaseOrderRepositoryInterface = (*PurchaseOrderRepository)(nil)

type PurchaseOrderRepository struct{}

func NewPurchaseOrderRepository() *PurchaseOrderRepository {
	return &PurchaseOrderRepository{}
}

func (r *PurchaseOrderRepository) Create(order *model.PurchaseOrder) error {
	db := database.GetDB()
	_, err := db.Exec(
		`INSERT INTO purchase_orders
		 (id, order_no, user_id, product_id, subscription_plan_id, duration_days, amount_cny_cent, payment_channel, payment_status, fulfillment_status,
		  alipay_trade_no, alipay_qr_code, alipay_qr_url, expires_at, paid_at, fulfilled_at, failure_reason, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		order.ID,
		order.OrderNo,
		order.UserID,
		order.ProductID,
		order.SubscriptionPlanID,
		order.DurationDays,
		order.AmountCNYCent,
		order.PaymentChannel,
		order.PaymentStatus,
		order.FulfillmentStatus,
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
			`SELECT id, order_no, user_id, product_id, subscription_plan_id, duration_days, amount_cny_cent, payment_channel, payment_status,
			        fulfillment_status, alipay_trade_no, alipay_qr_code, alipay_qr_url, expires_at, paid_at, fulfilled_at, failure_reason,
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
	conditions := []string{}
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

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

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
		limit = 100
	}
	queryArgs := append([]interface{}{}, args...)
	queryArgs = append(queryArgs, limit)
	rows, err := db.Query(
		r.detailSelectSQL()+where+` ORDER BY o.created_at DESC LIMIT ?`,
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

func (r *PurchaseOrderRepository) CountByProductID(productID string) (int64, error) {
	db := database.GetDB()
	var count int64
	err := db.QueryRow(`SELECT COUNT(*) FROM purchase_orders WHERE product_id = ?`, productID).Scan(&count)
	return count, err
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

func (r *PurchaseOrderRepository) detailSelectSQL() string {
	return `SELECT o.id, o.order_no, o.user_id, u.username, o.product_id, p.name, p.summary, o.subscription_plan_id, sp.name,
	               o.duration_days, o.amount_cny_cent, o.payment_channel, o.payment_status, o.fulfillment_status,
	               o.alipay_trade_no, o.alipay_qr_code, o.alipay_qr_url, o.expires_at, o.paid_at, o.fulfilled_at,
	               o.failure_reason, o.created_at, o.updated_at
	          FROM purchase_orders o
	          INNER JOIN users u ON u.id = o.user_id
	          INNER JOIN purchase_products p ON p.id = o.product_id
	          INNER JOIN subscription_plans sp ON sp.id = o.subscription_plan_id`
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
		&order.AmountCNYCent,
		&order.PaymentChannel,
		&order.PaymentStatus,
		&order.FulfillmentStatus,
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
			&order.AmountCNYCent,
			&order.PaymentChannel,
			&order.PaymentStatus,
			&order.FulfillmentStatus,
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
