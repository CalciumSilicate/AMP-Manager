package repository

import (
	"database/sql"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

type CouponRepository struct{}

func NewCouponRepository() *CouponRepository {
	return &CouponRepository{}
}

func (r *CouponRepository) CreateCampaign(campaign *model.CouponCampaign) error {
	db := database.GetDB()
	return r.createCampaignExec(db, campaign)
}

func (r *CouponRepository) CreateCampaignTx(tx *sql.Tx, campaign *model.CouponCampaign) error {
	return r.createCampaignExec(tx, campaign)
}

func (r *CouponRepository) createCampaignExec(exec interface {
	Exec(query string, args ...any) (sql.Result, error)
}, campaign *model.CouponCampaign) error {
	_, err := exec.Exec(
		`INSERT INTO coupon_campaigns (
			id, name, description, code_mode, shared_code, discount_type, fixed_discount_cny_cent,
			percent_off_bps, max_discount_cny_cent, total_usage_limit, used_count, per_user_limit,
			starts_at, ends_at, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		campaign.ID,
		campaign.Name,
		campaign.Description,
		campaign.CodeMode,
		campaign.SharedCode,
		campaign.DiscountType,
		campaign.FixedDiscountCNYCent,
		campaign.PercentOffBPS,
		campaign.MaxDiscountCNYCent,
		campaign.TotalUsageLimit,
		campaign.UsedCount,
		campaign.PerUserLimit,
		campaign.StartsAt,
		campaign.EndsAt,
		campaign.Enabled,
		campaign.CreatedAt,
		campaign.UpdatedAt,
	)
	return err
}

func (r *CouponRepository) UpdateCampaign(campaign *model.CouponCampaign) error {
	db := database.GetDB()
	return r.updateCampaignExec(db, campaign)
}

func (r *CouponRepository) UpdateCampaignTx(tx *sql.Tx, campaign *model.CouponCampaign) error {
	return r.updateCampaignExec(tx, campaign)
}

func (r *CouponRepository) updateCampaignExec(exec interface {
	Exec(query string, args ...any) (sql.Result, error)
}, campaign *model.CouponCampaign) error {
	_, err := exec.Exec(
		`UPDATE coupon_campaigns
		    SET name = ?, description = ?, code_mode = ?, shared_code = ?, discount_type = ?,
		        fixed_discount_cny_cent = ?, percent_off_bps = ?, max_discount_cny_cent = ?,
		        total_usage_limit = ?, per_user_limit = ?, starts_at = ?, ends_at = ?, enabled = ?, updated_at = ?
		  WHERE id = ?`,
		campaign.Name,
		campaign.Description,
		campaign.CodeMode,
		campaign.SharedCode,
		campaign.DiscountType,
		campaign.FixedDiscountCNYCent,
		campaign.PercentOffBPS,
		campaign.MaxDiscountCNYCent,
		campaign.TotalUsageLimit,
		campaign.PerUserLimit,
		campaign.StartsAt,
		campaign.EndsAt,
		campaign.Enabled,
		campaign.UpdatedAt,
		campaign.ID,
	)
	return err
}

func (r *CouponRepository) DeleteCampaign(id string) error {
	db := database.GetDB()
	_, err := db.Exec(`DELETE FROM coupon_campaigns WHERE id = ?`, id)
	return err
}

func (r *CouponRepository) GetCampaignByID(id string) (*model.CouponCampaign, error) {
	db := database.GetDB()
	return r.scanCampaignRow(db.QueryRow(
		`SELECT id, name, description, code_mode, shared_code, discount_type, fixed_discount_cny_cent,
		        percent_off_bps, max_discount_cny_cent, total_usage_limit, used_count, per_user_limit,
		        starts_at, ends_at, enabled, created_at, updated_at
		   FROM coupon_campaigns WHERE id = ?`,
		id,
	))
}

func (r *CouponRepository) ListCampaigns() ([]*model.CouponCampaign, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, name, description, code_mode, shared_code, discount_type, fixed_discount_cny_cent,
		        percent_off_bps, max_discount_cny_cent, total_usage_limit, used_count, per_user_limit,
		        starts_at, ends_at, enabled, created_at, updated_at
		   FROM coupon_campaigns
		  ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.CouponCampaign, 0)
	for rows.Next() {
		item := &model.CouponCampaign{}
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&item.CodeMode,
			&item.SharedCode,
			&item.DiscountType,
			&item.FixedDiscountCNYCent,
			&item.PercentOffBPS,
			&item.MaxDiscountCNYCent,
			&item.TotalUsageLimit,
			&item.UsedCount,
			&item.PerUserLimit,
			&item.StartsAt,
			&item.EndsAt,
			&item.Enabled,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *CouponRepository) CreateBatchTx(tx *sql.Tx, batch *model.CouponBatch) error {
	_, err := tx.Exec(
		`INSERT INTO coupon_code_batches (id, campaign_id, name, prefix, code_count, code_length, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		batch.ID,
		batch.CampaignID,
		batch.Name,
		batch.Prefix,
		batch.CodeCount,
		batch.CodeLength,
		batch.CreatedAt,
		batch.UpdatedAt,
	)
	return err
}

func (r *CouponRepository) CreateCodeTx(tx *sql.Tx, code *model.CouponCode) error {
	_, err := tx.Exec(
		`INSERT INTO coupon_codes (
			id, campaign_id, batch_id, code_mode, code_value, code_hash, code_mask, status,
			max_usages, used_count, last_used_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		code.ID,
		code.CampaignID,
		nullIfEmpty(code.BatchID),
		code.CodeMode,
		code.CodeValue,
		code.CodeHash,
		code.CodeMask,
		code.Status,
		code.MaxUsages,
		code.UsedCount,
		code.LastUsedAt,
		code.CreatedAt,
		code.UpdatedAt,
	)
	return err
}

func (r *CouponRepository) GetSharedCodeByCampaignTx(tx *sql.Tx, campaignID string) (*model.CouponCode, error) {
	item := &model.CouponCode{}
	var batchID sql.NullString
	err := tx.QueryRow(
		`SELECT id, campaign_id, batch_id, code_mode, code_value, code_hash, code_mask, status, max_usages, used_count, last_used_at, created_at, updated_at
		   FROM coupon_codes
		  WHERE campaign_id = ? AND code_mode = ?
		  ORDER BY created_at ASC
		  LIMIT 1`,
		campaignID, model.CouponCodeModeShared,
	).Scan(
		&item.ID,
		&item.CampaignID,
		&batchID,
		&item.CodeMode,
		&item.CodeValue,
		&item.CodeHash,
		&item.CodeMask,
		&item.Status,
		&item.MaxUsages,
		&item.UsedCount,
		&item.LastUsedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if batchID.Valid {
		item.BatchID = batchID.String
	}
	return item, err
}

func (r *CouponRepository) UpdateSharedCodeTx(tx *sql.Tx, code *model.CouponCode) error {
	_, err := tx.Exec(
		`UPDATE coupon_codes
		    SET code_value = ?, code_hash = ?, code_mask = ?, status = ?, max_usages = ?, updated_at = ?
		  WHERE id = ?`,
		code.CodeValue,
		code.CodeHash,
		code.CodeMask,
		code.Status,
		code.MaxUsages,
		code.UpdatedAt,
		code.ID,
	)
	return err
}

func (r *CouponRepository) GetCodeWithCampaignTx(tx *sql.Tx, codeValue string) (*model.CouponCode, *model.CouponCampaign, error) {
	code := &model.CouponCode{}
	campaign := &model.CouponCampaign{}
	var batchID sql.NullString
	var batchName sql.NullString
	err := tx.QueryRow(
		`SELECT cc.id, cc.campaign_id, c.name, cc.batch_id, COALESCE(b.name, ''), cc.code_mode, cc.code_value, cc.code_mask, c.discount_type,
		        c.fixed_discount_cny_cent, c.percent_off_bps, c.max_discount_cny_cent, cc.status, cc.max_usages, cc.used_count,
		        c.per_user_limit, c.starts_at, c.ends_at, cc.created_at, cc.updated_at,
		        c.id, c.name, c.description, c.code_mode, c.shared_code, c.discount_type, c.fixed_discount_cny_cent,
		        c.percent_off_bps, c.max_discount_cny_cent, c.total_usage_limit, c.used_count, c.per_user_limit,
		        c.starts_at, c.ends_at, c.enabled, c.created_at, c.updated_at
		   FROM coupon_codes cc
		   INNER JOIN coupon_campaigns c ON c.id = cc.campaign_id
		   LEFT JOIN coupon_code_batches b ON b.id = cc.batch_id
		  WHERE cc.code_value = ?`,
		strings.TrimSpace(codeValue),
	).Scan(
		&code.ID,
		&code.CampaignID,
		&code.CampaignName,
		&batchID,
		&batchName,
		&code.CodeMode,
		&code.CodeValue,
		&code.CodeMask,
		&code.DiscountType,
		&code.FixedDiscountCNYCent,
		&code.PercentOffBPS,
		&code.MaxDiscountCNYCent,
		&code.Status,
		&code.MaxUsages,
		&code.UsedCount,
		&code.PerUserLimit,
		&code.StartsAt,
		&code.EndsAt,
		&code.CreatedAt,
		&code.UpdatedAt,
		&campaign.ID,
		&campaign.Name,
		&campaign.Description,
		&campaign.CodeMode,
		&campaign.SharedCode,
		&campaign.DiscountType,
		&campaign.FixedDiscountCNYCent,
		&campaign.PercentOffBPS,
		&campaign.MaxDiscountCNYCent,
		&campaign.TotalUsageLimit,
		&campaign.UsedCount,
		&campaign.PerUserLimit,
		&campaign.StartsAt,
		&campaign.EndsAt,
		&campaign.Enabled,
		&campaign.CreatedAt,
		&campaign.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if batchID.Valid {
		code.BatchID = batchID.String
	}
	if batchName.Valid {
		code.BatchName = batchName.String
	}
	return code, campaign, nil
}

func (r *CouponRepository) CountAppliedUsageByUserTx(tx *sql.Tx, userID, campaignID string) (int, error) {
	var count int
	err := tx.QueryRow(
		`SELECT COUNT(*) FROM coupon_usages WHERE user_id = ? AND campaign_id = ? AND status = ?`,
		userID, campaignID, model.CouponUsageStatusApplied,
	).Scan(&count)
	return count, err
}

func (r *CouponRepository) CreateUsageTx(tx *sql.Tx, usage *model.CouponUsage) error {
	_, err := tx.Exec(
		`INSERT INTO coupon_usages (
			id, campaign_id, code_id, user_id, order_id, order_no, status, discount_cny_cent, final_amount_cny_cent, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		usage.ID,
		usage.CampaignID,
		usage.CodeID,
		usage.UserID,
		usage.OrderID,
		usage.OrderNo,
		usage.Status,
		usage.DiscountCNYCent,
		usage.FinalAmountCNYCent,
		usage.CreatedAt,
		usage.UpdatedAt,
	)
	return err
}

func (r *CouponRepository) GetUsageByOrderIDTx(tx *sql.Tx, orderID string) (*model.CouponUsage, error) {
	item := &model.CouponUsage{}
	err := tx.QueryRow(
		`SELECT id, campaign_id, code_id, user_id, order_id, order_no, status, discount_cny_cent, final_amount_cny_cent, created_at, updated_at
		   FROM coupon_usages
		  WHERE order_id = ?`,
		orderID,
	).Scan(
		&item.ID,
		&item.CampaignID,
		&item.CodeID,
		&item.UserID,
		&item.OrderID,
		&item.OrderNo,
		&item.Status,
		&item.DiscountCNYCent,
		&item.FinalAmountCNYCent,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (r *CouponRepository) MarkUsageReversedTx(tx *sql.Tx, usageID string, now time.Time) error {
	_, err := tx.Exec(
		`UPDATE coupon_usages SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		model.CouponUsageStatusReversed, now, usageID, model.CouponUsageStatusApplied,
	)
	return err
}

func (r *CouponRepository) ApplyUsageCountersTx(tx *sql.Tx, campaignID, codeID string, delta int, status model.CouponCodeStatus, now time.Time) error {
	_, err := tx.Exec(
		`UPDATE coupon_campaigns
		    SET used_count = CASE WHEN used_count + ? < 0 THEN 0 ELSE used_count + ? END,
		        updated_at = ?
		  WHERE id = ?`,
		delta, delta, now, campaignID,
	)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`UPDATE coupon_codes
		    SET used_count = CASE WHEN used_count + ? < 0 THEN 0 ELSE used_count + ? END,
		        status = ?,
		        last_used_at = CASE WHEN ? > 0 THEN ? ELSE last_used_at END,
		        updated_at = ?
		  WHERE id = ?`,
		delta, delta, status, delta, now, now, codeID,
	)
	return err
}

func (r *CouponRepository) ListCodes(campaignID, status, keyword string, limit int) ([]*model.CouponCode, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 200
	}
	builder := strings.Builder{}
	builder.WriteString(
		`SELECT cc.id, cc.campaign_id, c.name, cc.batch_id, COALESCE(b.name, ''), cc.code_mode, cc.code_value, cc.code_mask,
		        c.discount_type, c.fixed_discount_cny_cent, c.percent_off_bps, c.max_discount_cny_cent, cc.status,
		        cc.max_usages, cc.used_count, c.per_user_limit, c.starts_at, c.ends_at, cc.created_at, cc.updated_at
		   FROM coupon_codes cc
		   INNER JOIN coupon_campaigns c ON c.id = cc.campaign_id
		   LEFT JOIN coupon_code_batches b ON b.id = cc.batch_id
		  WHERE 1 = 1`,
	)
	args := make([]any, 0, 4)
	if trimmed := strings.TrimSpace(campaignID); trimmed != "" {
		builder.WriteString(` AND cc.campaign_id = ?`)
		args = append(args, trimmed)
	}
	if trimmed := strings.TrimSpace(status); trimmed != "" {
		builder.WriteString(` AND cc.status = ?`)
		args = append(args, trimmed)
	}
	if trimmed := strings.TrimSpace(keyword); trimmed != "" {
		builder.WriteString(` AND (cc.code_value LIKE ? OR cc.code_mask LIKE ? OR c.name LIKE ?)`)
		search := "%" + trimmed + "%"
		args = append(args, search, search, search)
	}
	builder.WriteString(` ORDER BY cc.created_at DESC LIMIT ?`)
	args = append(args, limit)

	rows, err := db.Query(builder.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.CouponCode, 0)
	for rows.Next() {
		item := &model.CouponCode{}
		var batchID sql.NullString
		var batchName sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.CampaignID,
			&item.CampaignName,
			&batchID,
			&batchName,
			&item.CodeMode,
			&item.CodeValue,
			&item.CodeMask,
			&item.DiscountType,
			&item.FixedDiscountCNYCent,
			&item.PercentOffBPS,
			&item.MaxDiscountCNYCent,
			&item.Status,
			&item.MaxUsages,
			&item.UsedCount,
			&item.PerUserLimit,
			&item.StartsAt,
			&item.EndsAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if batchID.Valid {
			item.BatchID = batchID.String
		}
		if batchName.Valid {
			item.BatchName = batchName.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *CouponRepository) SetCodeStatus(id string, status model.CouponCodeStatus) error {
	db := database.GetDB()
	_, err := db.Exec(`UPDATE coupon_codes SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().UTC(), id)
	return err
}

func (r *CouponRepository) ListUsages(limit int) ([]*model.CouponUsageResponse, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.Query(
		`SELECT u.id, u.campaign_id, COALESCE(c.name, ''), u.code_id, COALESCE(code.code_mask, ''),
		        u.user_id, COALESCE(users.username, ''), u.order_id, u.order_no, u.status,
		        u.discount_cny_cent, u.final_amount_cny_cent, u.created_at, u.updated_at
		   FROM coupon_usages u
		   LEFT JOIN coupon_campaigns c ON c.id = u.campaign_id
		   LEFT JOIN coupon_codes code ON code.id = u.code_id
		   LEFT JOIN users ON users.id = u.user_id
		  ORDER BY u.created_at DESC
		  LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.CouponUsageResponse, 0)
	for rows.Next() {
		item := &model.CouponUsageResponse{}
		if err := rows.Scan(
			&item.ID,
			&item.CampaignID,
			&item.CampaignName,
			&item.CodeID,
			&item.CodeMask,
			&item.UserID,
			&item.Username,
			&item.OrderID,
			&item.OrderNo,
			&item.Status,
			&item.DiscountCNYCent,
			&item.FinalAmountCNYCent,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *CouponRepository) scanCampaignRow(row *sql.Row) (*model.CouponCampaign, error) {
	item := &model.CouponCampaign{}
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.CodeMode,
		&item.SharedCode,
		&item.DiscountType,
		&item.FixedDiscountCNYCent,
		&item.PercentOffBPS,
		&item.MaxDiscountCNYCent,
		&item.TotalUsageLimit,
		&item.UsedCount,
		&item.PerUserLimit,
		&item.StartsAt,
		&item.EndsAt,
		&item.Enabled,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
