package repository

import (
	"database/sql"
	"errors"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"

	"github.com/google/uuid"
)

var ErrPurchaseProductNotFound = errors.New("售卖商品不存在")

type PurchaseProductRepositoryInterface interface {
	Create(product *model.PurchaseProduct) error
	GetByID(id string) (*model.PurchaseProduct, error)
	List(includeDisabled bool) ([]*model.PurchaseProduct, error)
	Update(id string, product *model.PurchaseProduct) error
	Delete(id string) error
	SetEnabled(id string, enabled bool) error
}

var _ PurchaseProductRepositoryInterface = (*PurchaseProductRepository)(nil)

type PurchaseProductRepository struct{}

func NewPurchaseProductRepository() *PurchaseProductRepository {
	return &PurchaseProductRepository{}
}

func (r *PurchaseProductRepository) Create(product *model.PurchaseProduct) error {
	db := database.GetDB()
	now := time.Now().UTC()
	product.ID = uuid.New().String()
	product.CreatedAt = now
	product.UpdatedAt = now

	_, err := db.Exec(
		`INSERT INTO purchase_products
		 (id, name, summary, subscription_plan_id, duration_days, price_cny_cent, is_recommended, sort_order, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		product.ID,
		product.Name,
		product.Summary,
		product.SubscriptionPlanID,
		product.DurationDays,
		product.PriceCNYCent,
		product.IsRecommended,
		product.SortOrder,
		product.Enabled,
		product.CreatedAt,
		product.UpdatedAt,
	)
	return err
}

func (r *PurchaseProductRepository) GetByID(id string) (*model.PurchaseProduct, error) {
	db := database.GetDB()
	product := &model.PurchaseProduct{}
	err := db.QueryRow(
		`SELECT id, name, summary, subscription_plan_id, duration_days, price_cny_cent, is_recommended, sort_order, enabled, created_at, updated_at
		 FROM purchase_products WHERE id = ?`,
		id,
	).Scan(
		&product.ID,
		&product.Name,
		&product.Summary,
		&product.SubscriptionPlanID,
		&product.DurationDays,
		&product.PriceCNYCent,
		&product.IsRecommended,
		&product.SortOrder,
		&product.Enabled,
		&product.CreatedAt,
		&product.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return product, err
}

func (r *PurchaseProductRepository) List(includeDisabled bool) ([]*model.PurchaseProduct, error) {
	db := database.GetDB()

	query := `SELECT id, name, summary, subscription_plan_id, duration_days, price_cny_cent, is_recommended, sort_order, enabled, created_at, updated_at
		FROM purchase_products`
	args := make([]interface{}, 0)
	if !includeDisabled {
		query += ` WHERE enabled = ?`
		args = append(args, true)
	}
	query += ` ORDER BY is_recommended DESC, sort_order ASC, created_at DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []*model.PurchaseProduct
	for rows.Next() {
		product := &model.PurchaseProduct{}
		if err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Summary,
			&product.SubscriptionPlanID,
			&product.DurationDays,
			&product.PriceCNYCent,
			&product.IsRecommended,
			&product.SortOrder,
			&product.Enabled,
			&product.CreatedAt,
			&product.UpdatedAt,
		); err != nil {
			return nil, err
		}
		products = append(products, product)
	}

	return products, rows.Err()
}

func (r *PurchaseProductRepository) Update(id string, product *model.PurchaseProduct) error {
	db := database.GetDB()
	now := time.Now().UTC()
	result, err := db.Exec(
		`UPDATE purchase_products
		 SET name = ?, summary = ?, subscription_plan_id = ?, duration_days = ?, price_cny_cent = ?, is_recommended = ?, sort_order = ?, enabled = ?, updated_at = ?
		 WHERE id = ?`,
		product.Name,
		product.Summary,
		product.SubscriptionPlanID,
		product.DurationDays,
		product.PriceCNYCent,
		product.IsRecommended,
		product.SortOrder,
		product.Enabled,
		now,
		id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPurchaseProductNotFound
	}
	return nil
}

func (r *PurchaseProductRepository) Delete(id string) error {
	db := database.GetDB()
	result, err := db.Exec(`DELETE FROM purchase_products WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPurchaseProductNotFound
	}
	return nil
}

func (r *PurchaseProductRepository) SetEnabled(id string, enabled bool) error {
	db := database.GetDB()
	result, err := db.Exec(
		`UPDATE purchase_products SET enabled = ?, updated_at = ? WHERE id = ?`,
		enabled,
		time.Now().UTC(),
		id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrPurchaseProductNotFound
	}
	return nil
}
