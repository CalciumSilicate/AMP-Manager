package repository

import (
	"database/sql"
	"strings"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

type RedeemRepositoryInterface interface {
	GetCampaign(id string) (*model.RedeemCampaign, error)
	GetCampaignResponse(id string) (*model.RedeemCampaignResponse, error)
	GetCode(id string) (*model.RedeemCode, error)
	GetSharedCodeByCampaign(campaignID string) (*model.RedeemCode, error)
	ListCampaigns() ([]*model.RedeemCampaignResponse, error)
	ListBatches(campaignID string) ([]*model.RedeemCodeBatchResponse, error)
	ListCodes(campaignID, batchID, status, keyword string, limit int) ([]*model.RedeemCodeResponse, error)
	ListRedemptions(campaignID, status, username string, limit int) ([]*model.RedeemRedemptionResponse, error)
	ListRedemptionsByUser(userID string, limit int) ([]*model.RedeemRedemptionResponse, error)
	SetCodeStatus(id string, status model.RedeemCodeStatus) error
	GetBatchCodes(batchID string) ([]*model.RedeemCode, error)
	CountCampaignAssets(campaignID string) (int, int, error)
	CountCodeRedemptions(codeID string) (int, error)
}

var _ RedeemRepositoryInterface = (*RedeemRepository)(nil)

type RedeemRepository struct{}

func NewRedeemRepository() *RedeemRepository {
	return &RedeemRepository{}
}

func (r *RedeemRepository) GetCampaign(id string) (*model.RedeemCampaign, error) {
	db := database.GetDB()
	item := &model.RedeemCampaign{}
	err := db.QueryRow(
		`SELECT id, name, description, code_mode, subscription_plan_id, subscription_duration_days, balance_micros,
		        total_redemptions_limit, redeemed_count, per_user_limit, starts_at, ends_at, enabled, created_at, updated_at
		   FROM redeem_campaigns
		  WHERE id = ?`,
		id,
	).Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.CodeMode,
		&item.SubscriptionPlanID,
		&item.SubscriptionDurationDays,
		&item.BalanceMicros,
		&item.TotalRedemptionsLimit,
		&item.RedeemedCount,
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

func (r *RedeemRepository) GetCampaignResponse(id string) (*model.RedeemCampaignResponse, error) {
	db := database.GetDB()
	item := &model.RedeemCampaignResponse{}
	err := db.QueryRow(
		`SELECT c.id, c.name, c.description, c.code_mode,
		        COALESCE(sc.code_value, ''), COALESCE(sc.code_mask, ''),
		        c.subscription_plan_id, COALESCE(sp.name, ''), c.subscription_duration_days, c.balance_micros,
		        c.total_redemptions_limit, c.redeemed_count, c.per_user_limit, c.starts_at, c.ends_at, c.enabled,
		        COALESCE(code_stats.code_count, 0), COALESCE(batch_stats.batch_count, 0),
		        c.created_at, c.updated_at
		   FROM redeem_campaigns c
		   LEFT JOIN subscription_plans sp ON sp.id = c.subscription_plan_id
		   LEFT JOIN (
		        SELECT campaign_id, MIN(code_value) AS code_value, MIN(code_mask) AS code_mask
		          FROM redeem_codes
		         WHERE batch_id IS NULL
		         GROUP BY campaign_id
		   ) sc ON sc.campaign_id = c.id
		   LEFT JOIN (
		        SELECT campaign_id, COUNT(*) AS code_count
		          FROM redeem_codes
		         GROUP BY campaign_id
		   ) code_stats ON code_stats.campaign_id = c.id
		   LEFT JOIN (
		        SELECT campaign_id, COUNT(*) AS batch_count
		          FROM redeem_code_batches
		         GROUP BY campaign_id
		   ) batch_stats ON batch_stats.campaign_id = c.id
		  WHERE c.id = ?`,
		id,
	).Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.CodeMode,
		&item.SharedCode,
		&item.SharedCodeMask,
		&item.SubscriptionPlanID,
		&item.SubscriptionPlanName,
		&item.SubscriptionDurationDays,
		&item.BalanceMicros,
		&item.TotalRedemptionsLimit,
		&item.RedeemedCount,
		&item.PerUserLimit,
		&item.StartsAt,
		&item.EndsAt,
		&item.Enabled,
		&item.CodeCount,
		&item.BatchCount,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (r *RedeemRepository) GetCode(id string) (*model.RedeemCode, error) {
	db := database.GetDB()
	item := &model.RedeemCode{}
	var batchID sql.NullString
	err := db.QueryRow(
		`SELECT id, campaign_id, batch_id, code_value, code_hash, code_mask, status, max_redemptions, redeemed_count, last_redeemed_at, created_at, updated_at
		   FROM redeem_codes
		  WHERE id = ?`,
		id,
	).Scan(
		&item.ID,
		&item.CampaignID,
		&batchID,
		&item.CodeValue,
		&item.CodeHash,
		&item.CodeMask,
		&item.Status,
		&item.MaxRedemptions,
		&item.RedeemedCount,
		&item.LastRedeemedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if batchID.Valid {
		item.BatchID = &batchID.String
	}
	return item, nil
}

func (r *RedeemRepository) GetSharedCodeByCampaign(campaignID string) (*model.RedeemCode, error) {
	db := database.GetDB()
	item := &model.RedeemCode{}
	var batchID sql.NullString
	err := db.QueryRow(
		`SELECT id, campaign_id, batch_id, code_value, code_hash, code_mask, status, max_redemptions, redeemed_count, last_redeemed_at, created_at, updated_at
		   FROM redeem_codes
		  WHERE campaign_id = ? AND batch_id IS NULL
		  ORDER BY created_at ASC
		  LIMIT 1`,
		campaignID,
	).Scan(
		&item.ID,
		&item.CampaignID,
		&batchID,
		&item.CodeValue,
		&item.CodeHash,
		&item.CodeMask,
		&item.Status,
		&item.MaxRedemptions,
		&item.RedeemedCount,
		&item.LastRedeemedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if batchID.Valid {
		item.BatchID = &batchID.String
	}
	return item, nil
}

func (r *RedeemRepository) ListCampaigns() ([]*model.RedeemCampaignResponse, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT c.id, c.name, c.description, c.code_mode,
		        COALESCE(sc.code_value, ''), COALESCE(sc.code_mask, ''),
		        c.subscription_plan_id, COALESCE(sp.name, ''), c.subscription_duration_days, c.balance_micros,
		        c.total_redemptions_limit, c.redeemed_count, c.per_user_limit, c.starts_at, c.ends_at, c.enabled,
		        COALESCE(code_stats.code_count, 0), COALESCE(batch_stats.batch_count, 0),
		        c.created_at, c.updated_at
		   FROM redeem_campaigns c
		   LEFT JOIN subscription_plans sp ON sp.id = c.subscription_plan_id
		   LEFT JOIN (
		        SELECT campaign_id, MIN(code_value) AS code_value, MIN(code_mask) AS code_mask
		          FROM redeem_codes
		         WHERE batch_id IS NULL
		         GROUP BY campaign_id
		   ) sc ON sc.campaign_id = c.id
		   LEFT JOIN (
		        SELECT campaign_id, COUNT(*) AS code_count
		          FROM redeem_codes
		         GROUP BY campaign_id
		   ) code_stats ON code_stats.campaign_id = c.id
		   LEFT JOIN (
		        SELECT campaign_id, COUNT(*) AS batch_count
		          FROM redeem_code_batches
		         GROUP BY campaign_id
		   ) batch_stats ON batch_stats.campaign_id = c.id
		  ORDER BY c.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.RedeemCampaignResponse, 0)
	for rows.Next() {
		item := &model.RedeemCampaignResponse{}
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&item.CodeMode,
			&item.SharedCode,
			&item.SharedCodeMask,
			&item.SubscriptionPlanID,
			&item.SubscriptionPlanName,
			&item.SubscriptionDurationDays,
			&item.BalanceMicros,
			&item.TotalRedemptionsLimit,
			&item.RedeemedCount,
			&item.PerUserLimit,
			&item.StartsAt,
			&item.EndsAt,
			&item.Enabled,
			&item.CodeCount,
			&item.BatchCount,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *RedeemRepository) ListBatches(campaignID string) ([]*model.RedeemCodeBatchResponse, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT b.id, b.campaign_id, COALESCE(c.name, ''), b.name, b.prefix, b.code_count, b.code_length, b.created_at, b.updated_at
		   FROM redeem_code_batches b
		   INNER JOIN redeem_campaigns c ON c.id = b.campaign_id
		  WHERE (? = '' OR b.campaign_id = ?)
		  ORDER BY b.created_at DESC`,
		campaignID,
		campaignID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.RedeemCodeBatchResponse, 0)
	for rows.Next() {
		item := &model.RedeemCodeBatchResponse{}
		if err := rows.Scan(
			&item.ID,
			&item.CampaignID,
			&item.CampaignName,
			&item.Name,
			&item.Prefix,
			&item.CodeCount,
			&item.CodeLength,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *RedeemRepository) ListCodes(campaignID, batchID, status, keyword string, limit int) ([]*model.RedeemCodeResponse, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 200
	}
	keyword = strings.TrimSpace(keyword)
	search := ""
	if keyword != "" {
		search = "%" + keyword + "%"
	}

	rows, err := db.Query(
		`SELECT rc.id, rc.campaign_id, COALESCE(c.name, ''), rc.batch_id, COALESCE(b.name, ''), c.code_mode,
		        rc.code_value, rc.code_mask, rc.status, rc.max_redemptions, rc.redeemed_count, rc.last_redeemed_at, rc.created_at, rc.updated_at
		   FROM redeem_codes rc
		   INNER JOIN redeem_campaigns c ON c.id = rc.campaign_id
		   LEFT JOIN redeem_code_batches b ON b.id = rc.batch_id
		  WHERE (? = '' OR rc.campaign_id = ?)
		    AND (? = '' OR COALESCE(rc.batch_id, '') = ?)
		    AND (? = '' OR rc.status = ?)
		    AND (? = '' OR rc.code_value LIKE ? OR rc.code_mask LIKE ? OR c.name LIKE ?)
		  ORDER BY rc.created_at DESC
		  LIMIT ?`,
		campaignID,
		campaignID,
		batchID,
		batchID,
		status,
		status,
		search,
		search,
		search,
		search,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.RedeemCodeResponse, 0)
	for rows.Next() {
		item := &model.RedeemCodeResponse{}
		var batchName sql.NullString
		var batchIDValue sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.CampaignID,
			&item.CampaignName,
			&batchIDValue,
			&batchName,
			&item.CodeMode,
			&item.CodeValue,
			&item.CodeMask,
			&item.Status,
			&item.MaxRedemptions,
			&item.RedeemedCount,
			&item.LastRedeemedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if batchIDValue.Valid {
			item.BatchID = &batchIDValue.String
		}
		if batchName.Valid {
			item.BatchName = batchName.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *RedeemRepository) ListRedemptions(campaignID, status, username string, limit int) ([]*model.RedeemRedemptionResponse, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 200
	}
	searchUser := ""
	if trimmed := strings.TrimSpace(username); trimmed != "" {
		searchUser = "%" + trimmed + "%"
	}

	rows, err := db.Query(
		`SELECT rr.id, rr.campaign_id, COALESCE(c.name, ''), rr.code_id, rr.user_id, rr.username, rr.code_mask,
		        rr.subscription_plan_id, COALESCE(sp.name, ''), rr.subscription_duration_days, rr.balance_micros,
		        rr.status, rr.failure_reason, rr.granted_subscription_id, rr.granted_expires_at, rr.balance_after_micros, rr.created_at
		   FROM redeem_redemptions rr
		   LEFT JOIN redeem_campaigns c ON c.id = rr.campaign_id
		   LEFT JOIN subscription_plans sp ON sp.id = rr.subscription_plan_id
		  WHERE (? = '' OR COALESCE(rr.campaign_id, '') = ?)
		    AND (? = '' OR rr.status = ?)
		    AND (? = '' OR rr.username LIKE ?)
		  ORDER BY rr.created_at DESC
		  LIMIT ?`,
		campaignID,
		campaignID,
		status,
		status,
		searchUser,
		searchUser,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.RedeemRedemptionResponse, 0)
	for rows.Next() {
		item := &model.RedeemRedemptionResponse{}
		var campaignIDValue sql.NullString
		var codeIDValue sql.NullString
		if err := rows.Scan(
			&item.ID,
			&campaignIDValue,
			&item.CampaignName,
			&codeIDValue,
			&item.UserID,
			&item.Username,
			&item.CodeMask,
			&item.SubscriptionPlanID,
			&item.SubscriptionPlanName,
			&item.SubscriptionDurationDays,
			&item.BalanceMicros,
			&item.Status,
			&item.FailureReason,
			&item.GrantedSubscriptionID,
			&item.GrantedExpiresAt,
			&item.BalanceAfterMicros,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if campaignIDValue.Valid {
			item.CampaignID = &campaignIDValue.String
		}
		if codeIDValue.Valid {
			item.CodeID = &codeIDValue.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *RedeemRepository) ListRedemptionsByUser(userID string, limit int) ([]*model.RedeemRedemptionResponse, error) {
	return r.ListRedemptionsForUserWithLimit(userID, limit)
}

func (r *RedeemRepository) ListRedemptionsForUserWithLimit(userID string, limit int) ([]*model.RedeemRedemptionResponse, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(
		`SELECT rr.id, rr.campaign_id, COALESCE(c.name, ''), rr.code_id, rr.user_id, rr.username, rr.code_mask,
		        rr.subscription_plan_id, COALESCE(sp.name, ''), rr.subscription_duration_days, rr.balance_micros,
		        rr.status, rr.failure_reason, rr.granted_subscription_id, rr.granted_expires_at, rr.balance_after_micros, rr.created_at
		   FROM redeem_redemptions rr
		   LEFT JOIN redeem_campaigns c ON c.id = rr.campaign_id
		   LEFT JOIN subscription_plans sp ON sp.id = rr.subscription_plan_id
		  WHERE rr.user_id = ?
		  ORDER BY rr.created_at DESC
		  LIMIT ?`,
		userID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.RedeemRedemptionResponse, 0)
	for rows.Next() {
		item := &model.RedeemRedemptionResponse{}
		var campaignIDValue sql.NullString
		var codeIDValue sql.NullString
		if err := rows.Scan(
			&item.ID,
			&campaignIDValue,
			&item.CampaignName,
			&codeIDValue,
			&item.UserID,
			&item.Username,
			&item.CodeMask,
			&item.SubscriptionPlanID,
			&item.SubscriptionPlanName,
			&item.SubscriptionDurationDays,
			&item.BalanceMicros,
			&item.Status,
			&item.FailureReason,
			&item.GrantedSubscriptionID,
			&item.GrantedExpiresAt,
			&item.BalanceAfterMicros,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if campaignIDValue.Valid {
			item.CampaignID = &campaignIDValue.String
		}
		if codeIDValue.Valid {
			item.CodeID = &codeIDValue.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *RedeemRepository) SetCodeStatus(id string, status model.RedeemCodeStatus) error {
	db := database.GetDB()
	_, err := db.Exec(
		`UPDATE redeem_codes SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status,
		id,
	)
	return err
}

func (r *RedeemRepository) GetBatchCodes(batchID string) ([]*model.RedeemCode, error) {
	db := database.GetDB()
	rows, err := db.Query(
		`SELECT id, campaign_id, batch_id, code_value, code_hash, code_mask, status, max_redemptions, redeemed_count, last_redeemed_at, created_at, updated_at
		   FROM redeem_codes
		  WHERE batch_id = ?
		  ORDER BY created_at ASC`,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*model.RedeemCode, 0)
	for rows.Next() {
		item := &model.RedeemCode{}
		var batchIDValue sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.CampaignID,
			&batchIDValue,
			&item.CodeValue,
			&item.CodeHash,
			&item.CodeMask,
			&item.Status,
			&item.MaxRedemptions,
			&item.RedeemedCount,
			&item.LastRedeemedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if batchIDValue.Valid {
			item.BatchID = &batchIDValue.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *RedeemRepository) CountCampaignAssets(campaignID string) (int, int, error) {
	db := database.GetDB()
	var codeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM redeem_codes WHERE campaign_id = ?`, campaignID).Scan(&codeCount); err != nil {
		return 0, 0, err
	}
	var redemptionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM redeem_redemptions WHERE COALESCE(campaign_id, '') = ?`, campaignID).Scan(&redemptionCount); err != nil {
		return 0, 0, err
	}
	return codeCount, redemptionCount, nil
}

func (r *RedeemRepository) CountCodeRedemptions(codeID string) (int, error) {
	db := database.GetDB()
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM redeem_redemptions WHERE COALESCE(code_id, '') = ? AND status = 'success'`,
		codeID,
	).Scan(&count)
	return count, err
}
