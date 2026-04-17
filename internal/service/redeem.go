package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
)

var (
	ErrRedeemCampaignNotFound     = errors.New("兑换活动不存在")
	ErrRedeemCodeNotFound         = errors.New("兑换码不存在")
	ErrRedeemBatchNotFound        = errors.New("兑换批次不存在")
	ErrRedeemCampaignHasUsage     = errors.New("活动已生成兑换码或有兑换记录，不能删除")
	ErrRedeemRewardRequired       = errors.New("至少需要配置一种奖励")
	ErrRedeemRewardInvalid        = errors.New("订阅奖励配置无效")
	ErrRedeemSharedCodeRequired   = errors.New("共享活动码不能为空")
	ErrRedeemCodeModeImmutable    = errors.New("创建后不能修改兑换码模式")
	ErrRedeemSingleUseOnly        = errors.New("仅单次码活动支持批量生成")
	ErrRedeemCampaignDisabled     = errors.New("兑换活动已停用")
	ErrRedeemCodeDisabled         = errors.New("兑换码已停用")
	ErrRedeemCodeConsumed         = errors.New("兑换码已用尽")
	ErrRedeemCampaignNotStarted   = errors.New("兑换活动尚未开始")
	ErrRedeemCampaignEnded        = errors.New("兑换活动已结束")
	ErrRedeemPerUserLimitReached  = errors.New("已达到当前账号可兑换次数上限")
	ErrRedeemCampaignLimitReached = errors.New("活动可兑换次数已用尽")
	ErrRedeemSharedCodeLocked     = errors.New("共享活动码已有成功兑换记录，不能修改码值")
)

const redeemAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

type RedeemService struct {
	repo     repository.RedeemRepositoryInterface
	planRepo repository.SubscriptionPlanRepositoryInterface
	userRepo repository.UserRepositoryInterface
	subSvc   *UserSubscriptionService
	grantSvc *RewardGrantService
}

type redeemLookup struct {
	CodeID                   string
	CampaignID               string
	CampaignName             string
	CampaignDescription      string
	CodeMode                 model.RedeemCodeMode
	CodeValue                string
	CodeMask                 string
	CodeStatus               model.RedeemCodeStatus
	CodeMaxRedemptions       int
	CodeRedeemedCount        int
	SubscriptionPlanID       string
	SubscriptionPlanName     string
	SubscriptionDurationDays int
	BalanceMicros            int64
	TotalRedemptionsLimit    int
	CampaignRedeemedCount    int
	PerUserLimit             int
	StartsAt                 *time.Time
	EndsAt                   *time.Time
	CampaignEnabled          bool
}

func NewRedeemService() *RedeemService {
	return &RedeemService{
		repo:     repository.NewRedeemRepository(),
		planRepo: repository.NewSubscriptionPlanRepository(),
		userRepo: repository.NewUserRepository(),
		subSvc:   NewUserSubscriptionService(),
		grantSvc: NewRewardGrantService(),
	}
}

func (s *RedeemService) ListCampaigns() ([]*model.RedeemCampaignResponse, error) {
	return s.repo.ListCampaigns()
}

func (s *RedeemService) CreateCampaign(req *model.RedeemCampaignRequest) (*model.RedeemCampaignResponse, error) {
	if err := s.validateCampaignRequest(req); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	campaignID := uuid.New().String()
	if _, err := tx.Exec(
		`INSERT INTO redeem_campaigns (
			id, name, description, code_mode, subscription_plan_id, subscription_duration_days, balance_micros,
			total_redemptions_limit, redeemed_count, per_user_limit, starts_at, ends_at, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
		campaignID,
		strings.TrimSpace(req.Name),
		strings.TrimSpace(req.Description),
		req.CodeMode,
		strings.TrimSpace(req.SubscriptionPlanID),
		req.SubscriptionDurationDays,
		req.BalanceMicros,
		req.TotalRedemptionsLimit,
		req.PerUserLimit,
		req.StartsAt,
		req.EndsAt,
		req.Enabled,
		now,
		now,
	); err != nil {
		return nil, err
	}

	if req.CodeMode == model.RedeemCodeModeShared {
		if err := s.insertSharedCodeTx(tx, campaignID, req.SharedCode, req.TotalRedemptionsLimit, req.Enabled, now); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.repo.GetCampaignResponse(campaignID)
}

func (s *RedeemService) UpdateCampaign(id string, req *model.RedeemCampaignRequest) (*model.RedeemCampaignResponse, error) {
	existing, err := s.repo.GetCampaign(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrRedeemCampaignNotFound
	}
	if existing.CodeMode != req.CodeMode {
		return nil, ErrRedeemCodeModeImmutable
	}
	if err := s.validateCampaignRequest(req); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE redeem_campaigns
		    SET name = ?, description = ?, subscription_plan_id = ?, subscription_duration_days = ?, balance_micros = ?,
		        total_redemptions_limit = ?, per_user_limit = ?, starts_at = ?, ends_at = ?, enabled = ?, updated_at = ?
		  WHERE id = ?`,
		strings.TrimSpace(req.Name),
		strings.TrimSpace(req.Description),
		strings.TrimSpace(req.SubscriptionPlanID),
		req.SubscriptionDurationDays,
		req.BalanceMicros,
		req.TotalRedemptionsLimit,
		req.PerUserLimit,
		req.StartsAt,
		req.EndsAt,
		req.Enabled,
		now,
		id,
	); err != nil {
		return nil, err
	}

	if existing.CodeMode == model.RedeemCodeModeShared {
		if err := s.upsertSharedCodeTx(tx, id, req.SharedCode, req.TotalRedemptionsLimit, req.Enabled, now); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.repo.GetCampaignResponse(id)
}

func (s *RedeemService) DeleteCampaign(id string) error {
	existing, err := s.repo.GetCampaign(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrRedeemCampaignNotFound
	}

	codeCount, redemptionCount, err := s.repo.CountCampaignAssets(id)
	if err != nil {
		return err
	}
	if codeCount > 0 || redemptionCount > 0 {
		return ErrRedeemCampaignHasUsage
	}

	_, err = database.GetDB().Exec(`DELETE FROM redeem_campaigns WHERE id = ?`, id)
	return err
}

func (s *RedeemService) CreateBatch(campaignID string, req *model.RedeemCodeBatchRequest) (*model.RedeemCodeBatchResponse, []string, error) {
	campaign, err := s.repo.GetCampaign(campaignID)
	if err != nil {
		return nil, nil, err
	}
	if campaign == nil {
		return nil, nil, ErrRedeemCampaignNotFound
	}
	if campaign.CodeMode != model.RedeemCodeModeSingleUse {
		return nil, nil, ErrRedeemSingleUseOnly
	}

	prefix := sanitizeRedeemSegment(req.Prefix)
	name := strings.TrimSpace(req.Name)
	now := time.Now().UTC()
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	batchID := uuid.New().String()
	if _, err := tx.Exec(
		`INSERT INTO redeem_code_batches (id, campaign_id, name, prefix, code_count, code_length, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		batchID,
		campaignID,
		name,
		prefix,
		req.CodeCount,
		req.CodeLength,
		now,
		now,
	); err != nil {
		return nil, nil, err
	}

	generated := make([]string, 0, req.CodeCount)
	seen := make(map[string]struct{}, req.CodeCount)
	codeStatus := model.RedeemCodeStatusActive
	if !campaign.Enabled {
		codeStatus = model.RedeemCodeStatusDisabled
	}
	for len(generated) < req.CodeCount {
		value, err := s.generateUniqueCodeTx(tx, prefix, req.CodeLength, seen)
		if err != nil {
			return nil, nil, err
		}
		generated = append(generated, value)
		if _, err := tx.Exec(
			`INSERT INTO redeem_codes (
				id, campaign_id, batch_id, code_value, code_hash, code_mask, status, max_redemptions, redeemed_count, last_redeemed_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, NULL, ?, ?)`,
			uuid.New().String(),
			campaignID,
			batchID,
			value,
			hashRedeemCode(value),
			maskRedeemCode(value),
			codeStatus,
			1,
			now,
			now,
		); err != nil {
			return nil, nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	batches, err := s.repo.ListBatches(campaignID)
	if err != nil {
		return nil, nil, err
	}
	for _, batch := range batches {
		if batch.ID == batchID {
			return batch, generated, nil
		}
	}
	return nil, generated, ErrRedeemBatchNotFound
}

func (s *RedeemService) ListBatches(campaignID string) ([]*model.RedeemCodeBatchResponse, error) {
	return s.repo.ListBatches(campaignID)
}

func (s *RedeemService) ExportBatchCSV(batchID string) ([]byte, error) {
	codes, err := s.repo.GetBatchCodes(batchID)
	if err != nil {
		return nil, err
	}
	if len(codes) == 0 {
		return nil, ErrRedeemBatchNotFound
	}

	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	if err := writer.Write([]string{"code", "mask", "status", "redeemedCount", "createdAt"}); err != nil {
		return nil, err
	}
	for _, code := range codes {
		if err := writer.Write([]string{
			code.CodeValue,
			code.CodeMask,
			string(code.Status),
			fmt.Sprintf("%d", code.RedeemedCount),
			code.CreatedAt.Format(time.RFC3339),
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return []byte(builder.String()), nil
}

func (s *RedeemService) ListCodes(campaignID, batchID, status, keyword string, limit int) ([]*model.RedeemCodeResponse, error) {
	return s.repo.ListCodes(campaignID, batchID, status, keyword, limit)
}

func (s *RedeemService) SetCodeEnabled(codeID string, enabled bool) error {
	code, err := s.repo.GetCode(codeID)
	if err != nil {
		return err
	}
	if code == nil {
		return ErrRedeemCodeNotFound
	}
	if enabled && code.MaxRedemptions > 0 && code.RedeemedCount >= code.MaxRedemptions {
		return ErrRedeemCodeConsumed
	}
	status := model.RedeemCodeStatusDisabled
	if enabled {
		status = model.RedeemCodeStatusActive
	}
	return s.repo.SetCodeStatus(codeID, status)
}

func (s *RedeemService) ListRedemptions(campaignID, status, username string, limit int) ([]*model.RedeemRedemptionResponse, error) {
	return s.repo.ListRedemptions(campaignID, status, username, limit)
}

func (s *RedeemService) ListUserRedemptions(userID string, limit int) ([]*model.RedeemRedemptionResponse, error) {
	return s.repo.ListRedemptionsByUser(userID, limit)
}

func (s *RedeemService) Redeem(_ context.Context, userID, username, rawCode string) (*model.RedeemResultResponse, error) {
	codeInput := normalizeRedeemCode(rawCode)
	now := time.Now().UTC()
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	lookup, err := s.loadLookupByCodeTx(tx, codeInput)
	if err != nil {
		return nil, err
	}
	if lookup == nil {
		if err := s.insertRedemptionTx(tx, &model.RedeemRedemption{
			ID:            uuid.New().String(),
			UserID:        userID,
			Username:      username,
			CodeInput:     codeInput,
			CodeMask:      maskRedeemCode(codeInput),
			Status:        model.RedeemRedemptionStatusRejected,
			FailureReason: ErrRedeemCodeNotFound.Error(),
			CreatedAt:     now,
		}); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, ErrRedeemCodeNotFound
	}

	if err := s.validateRedeemLookup(now, lookup); err != nil {
		if err := s.insertRejectedLookupTx(tx, lookup, userID, username, codeInput, err.Error(), now); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, err
	}

	activeSub, err := s.grantSvc.getActiveSubscriptionTx(tx, userID, now)
	if err != nil {
		return nil, err
	}
	if activeSub != nil && lookup.SubscriptionDurationDays > 0 {
		if activeSub.PlanID != lookup.SubscriptionPlanID {
			if err := s.insertRejectedLookupTx(tx, lookup, userID, username, codeInput, ErrDifferentPlanActive.Error(), now); err != nil {
				return nil, err
			}
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return nil, ErrDifferentPlanActive
		}
		if activeSub.ExpiresAt == nil {
			if err := s.insertRejectedLookupTx(tx, lookup, userID, username, codeInput, ErrPermanentSubscription.Error(), now); err != nil {
				return nil, err
			}
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return nil, ErrPermanentSubscription
		}
	}

	var grantedSub *model.UserSubscription
	if lookup.SubscriptionDurationDays > 0 {
		grantedSub, err = s.grantSvc.GrantSubscriptionTx(tx, userID, lookup.SubscriptionPlanID, lookup.SubscriptionDurationDays, now)
		if err != nil {
			return nil, err
		}
	}

	balanceAfterMicros, err := s.grantSvc.GrantBalanceTx(tx, userID, lookup.BalanceMicros, now)
	if err != nil {
		return nil, err
	}

	if err := s.reserveUserRedemptionTx(tx, lookup.CodeID, userID, lookup.PerUserLimit, now); err != nil {
		if err := s.insertRejectedLookupTx(tx, lookup, userID, username, codeInput, err.Error(), now); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, err
	}
	if err := s.reserveCampaignRedemptionTx(tx, lookup.CampaignID, now); err != nil {
		if err := s.insertRejectedLookupTx(tx, lookup, userID, username, codeInput, err.Error(), now); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, err
	}
	if err := s.reserveCodeRedemptionTx(tx, lookup.CodeID, now); err != nil {
		if err := s.insertRejectedLookupTx(tx, lookup, userID, username, codeInput, err.Error(), now); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, err
	}

	campaignID := lookup.CampaignID
	codeID := lookup.CodeID
	redemption := &model.RedeemRedemption{
		ID:                       uuid.New().String(),
		CampaignID:               &campaignID,
		CodeID:                   &codeID,
		UserID:                   userID,
		Username:                 username,
		CodeInput:                codeInput,
		CodeMask:                 lookup.CodeMask,
		SubscriptionPlanID:       lookup.SubscriptionPlanID,
		SubscriptionDurationDays: lookup.SubscriptionDurationDays,
		BalanceMicros:            lookup.BalanceMicros,
		Status:                   model.RedeemRedemptionStatusSuccess,
		GrantedSubscriptionID:    "",
		BalanceAfterMicros:       balanceAfterMicros,
		CreatedAt:                now,
	}
	if grantedSub != nil {
		redemption.GrantedSubscriptionID = grantedSub.ID
		redemption.GrantedExpiresAt = grantedSub.ExpiresAt
	}
	if err := s.insertRedemptionTx(tx, redemption); err != nil {
		return nil, err
	}

	syncAction := BillingStateSyncAction{}
	if grantedSub != nil {
		syncAction = BillingStateSyncAction{
			UserID:           userID,
			RefreshUserState: true,
		}
	} else if lookup.BalanceMicros > 0 {
		syncAction = BillingStateSyncAction{
			UserID:             userID,
			BalanceDeltaMicros: lookup.BalanceMicros,
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if err := s.grantSvc.SyncBillingState(context.Background(), syncAction); err != nil {
		return nil, err
	}

	currentSubscription, err := s.subSvc.GetActive(userID)
	if err != nil {
		return nil, err
	}
	if balanceAfterMicros == 0 {
		balanceAfterMicros, err = s.userRepo.GetBalance(userID)
		if err != nil {
			return nil, err
		}
	}
	campaignResponse, err := s.repo.GetCampaignResponse(lookup.CampaignID)
	if err != nil {
		return nil, err
	}
	responseRedemption := &model.RedeemRedemptionResponse{
		ID:                       redemption.ID,
		CampaignID:               redemption.CampaignID,
		CampaignName:             lookup.CampaignName,
		CodeID:                   redemption.CodeID,
		UserID:                   redemption.UserID,
		Username:                 redemption.Username,
		CodeMask:                 redemption.CodeMask,
		SubscriptionPlanID:       redemption.SubscriptionPlanID,
		SubscriptionPlanName:     lookup.SubscriptionPlanName,
		SubscriptionDurationDays: redemption.SubscriptionDurationDays,
		BalanceMicros:            redemption.BalanceMicros,
		Status:                   redemption.Status,
		FailureReason:            redemption.FailureReason,
		GrantedSubscriptionID:    redemption.GrantedSubscriptionID,
		GrantedExpiresAt:         redemption.GrantedExpiresAt,
		BalanceAfterMicros:       redemption.BalanceAfterMicros,
		CreatedAt:                redemption.CreatedAt,
	}

	return &model.RedeemResultResponse{
		Message:             "兑换成功",
		Campaign:            campaignResponse,
		Redemption:          responseRedemption,
		CurrentSubscription: currentSubscription,
		BalanceMicros:       balanceAfterMicros,
		BalanceUsd:          formatBalanceMicros(balanceAfterMicros),
	}, nil
}

func (s *RedeemService) validateCampaignRequest(req *model.RedeemCampaignRequest) error {
	if req == nil {
		return ErrRedeemCampaignNotFound
	}
	if strings.TrimSpace(req.Name) == "" {
		return ErrRedeemCampaignNotFound
	}
	if req.SubscriptionDurationDays > 0 && strings.TrimSpace(req.SubscriptionPlanID) == "" {
		return ErrRedeemRewardInvalid
	}
	if strings.TrimSpace(req.SubscriptionPlanID) != "" && req.SubscriptionDurationDays <= 0 {
		return ErrRedeemRewardInvalid
	}
	if req.BalanceMicros < 0 {
		return ErrRedeemRewardInvalid
	}
	if req.SubscriptionDurationDays <= 0 && req.BalanceMicros <= 0 {
		return ErrRedeemRewardRequired
	}
	if req.StartsAt != nil && req.EndsAt != nil && req.StartsAt.After(*req.EndsAt) {
		return ErrRedeemRewardInvalid
	}
	if req.CodeMode == model.RedeemCodeModeShared && normalizeRedeemCode(req.SharedCode) == "" {
		return ErrRedeemSharedCodeRequired
	}
	if strings.TrimSpace(req.SubscriptionPlanID) != "" {
		plan, _, err := s.planRepo.GetByID(strings.TrimSpace(req.SubscriptionPlanID))
		if err != nil {
			return err
		}
		if plan == nil {
			return ErrPlanNotFound
		}
	}
	return nil
}

func (s *RedeemService) insertSharedCodeTx(tx *sql.Tx, campaignID, rawCode string, totalLimit int, enabled bool, now time.Time) error {
	value := normalizeRedeemCode(rawCode)
	status := model.RedeemCodeStatusDisabled
	if enabled {
		status = model.RedeemCodeStatusActive
	}
	_, err := tx.Exec(
		`INSERT INTO redeem_codes (
			id, campaign_id, batch_id, code_value, code_hash, code_mask, status, max_redemptions, redeemed_count, last_redeemed_at, created_at, updated_at
		) VALUES (?, ?, NULL, ?, ?, ?, ?, ?, 0, NULL, ?, ?)`,
		uuid.New().String(),
		campaignID,
		value,
		hashRedeemCode(value),
		maskRedeemCode(value),
		status,
		totalLimit,
		now,
		now,
	)
	return err
}

func (s *RedeemService) upsertSharedCodeTx(tx *sql.Tx, campaignID, rawCode string, totalLimit int, enabled bool, now time.Time) error {
	item, err := s.loadSharedCodeTx(tx, campaignID)
	if err != nil {
		return err
	}
	status := model.RedeemCodeStatusDisabled
	if enabled {
		status = model.RedeemCodeStatusActive
	}
	value := normalizeRedeemCode(rawCode)
	if item == nil {
		return s.insertSharedCodeTx(tx, campaignID, value, totalLimit, enabled, now)
	}
	if value != item.CodeValue {
		var count int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM redeem_redemptions WHERE COALESCE(code_id, '') = ? AND status = 'success'`,
			item.ID,
		).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return ErrRedeemSharedCodeLocked
		}
	}
	_, err = tx.Exec(
		`UPDATE redeem_codes
		    SET code_value = ?, code_hash = ?, code_mask = ?, status = ?, max_redemptions = ?, updated_at = ?
		  WHERE id = ?`,
		value,
		hashRedeemCode(value),
		maskRedeemCode(value),
		status,
		totalLimit,
		now,
		item.ID,
	)
	return err
}

func (s *RedeemService) generateUniqueCodeTx(tx *sql.Tx, prefix string, randomLength int, seen map[string]struct{}) (string, error) {
	for attempts := 0; attempts < 256; attempts++ {
		suffix, err := randomRedeemString(randomLength)
		if err != nil {
			return "", err
		}
		value := suffix
		if prefix != "" {
			value = prefix + "-" + suffix
		}
		if _, ok := seen[value]; ok {
			continue
		}
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM redeem_codes WHERE code_hash = ?`, hashRedeemCode(value)).Scan(&count); err != nil {
			return "", err
		}
		if count > 0 {
			continue
		}
		seen[value] = struct{}{}
		return value, nil
	}
	return "", fmt.Errorf("生成兑换码失败，请重试")
}

func (s *RedeemService) loadLookupByCodeTx(tx *sql.Tx, codeValue string) (*redeemLookup, error) {
	item := &redeemLookup{}
	err := tx.QueryRow(
		`SELECT rc.id, c.id, c.name, c.description, c.code_mode, rc.code_value, rc.code_mask, rc.status, rc.max_redemptions, rc.redeemed_count,
		        c.subscription_plan_id, COALESCE(sp.name, ''), c.subscription_duration_days, c.balance_micros,
		        c.total_redemptions_limit, c.redeemed_count, c.per_user_limit, c.starts_at, c.ends_at, c.enabled
		   FROM redeem_codes rc
		   INNER JOIN redeem_campaigns c ON c.id = rc.campaign_id
		   LEFT JOIN subscription_plans sp ON sp.id = c.subscription_plan_id
		  WHERE rc.code_value = ?`,
		codeValue,
	).Scan(
		&item.CodeID,
		&item.CampaignID,
		&item.CampaignName,
		&item.CampaignDescription,
		&item.CodeMode,
		&item.CodeValue,
		&item.CodeMask,
		&item.CodeStatus,
		&item.CodeMaxRedemptions,
		&item.CodeRedeemedCount,
		&item.SubscriptionPlanID,
		&item.SubscriptionPlanName,
		&item.SubscriptionDurationDays,
		&item.BalanceMicros,
		&item.TotalRedemptionsLimit,
		&item.CampaignRedeemedCount,
		&item.PerUserLimit,
		&item.StartsAt,
		&item.EndsAt,
		&item.CampaignEnabled,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *RedeemService) validateRedeemLookup(now time.Time, item *redeemLookup) error {
	if item == nil {
		return ErrRedeemCodeNotFound
	}
	if !item.CampaignEnabled {
		return ErrRedeemCampaignDisabled
	}
	if item.StartsAt != nil && now.Before(item.StartsAt.UTC()) {
		return ErrRedeemCampaignNotStarted
	}
	if item.EndsAt != nil && now.After(item.EndsAt.UTC()) {
		return ErrRedeemCampaignEnded
	}
	if item.CodeStatus == model.RedeemCodeStatusDisabled {
		return ErrRedeemCodeDisabled
	}
	if item.CodeStatus == model.RedeemCodeStatusConsumed || (item.CodeMaxRedemptions > 0 && item.CodeRedeemedCount >= item.CodeMaxRedemptions) {
		return ErrRedeemCodeConsumed
	}
	if item.TotalRedemptionsLimit > 0 && item.CampaignRedeemedCount >= item.TotalRedemptionsLimit {
		return ErrRedeemCampaignLimitReached
	}
	if item.SubscriptionDurationDays <= 0 && item.BalanceMicros <= 0 {
		return ErrRedeemRewardRequired
	}
	if item.SubscriptionDurationDays > 0 && item.SubscriptionPlanID == "" {
		return ErrRedeemRewardInvalid
	}
	return nil
}

func (s *RedeemService) reserveUserRedemptionTx(tx *sql.Tx, codeID, userID string, perUserLimit int, now time.Time) error {
	if perUserLimit <= 0 {
		return nil
	}
	result, err := tx.Exec(
		`INSERT INTO redeem_user_counters (code_id, user_id, redeemed_count, updated_at)
		 VALUES (?, ?, 1, ?)
		 ON CONFLICT(code_id, user_id) DO UPDATE SET
		    redeemed_count = redeem_user_counters.redeemed_count + 1,
		    updated_at = excluded.updated_at
		 WHERE redeem_user_counters.redeemed_count < ?`,
		codeID,
		userID,
		now,
		perUserLimit,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRedeemPerUserLimitReached
	}
	return nil
}

func (s *RedeemService) reserveCampaignRedemptionTx(tx *sql.Tx, campaignID string, now time.Time) error {
	result, err := tx.Exec(
		`UPDATE redeem_campaigns
		    SET redeemed_count = redeemed_count + 1, updated_at = ?
		  WHERE id = ? AND enabled = 1 AND (total_redemptions_limit <= 0 OR redeemed_count < total_redemptions_limit)`,
		now,
		campaignID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRedeemCampaignLimitReached
	}
	return nil
}

func (s *RedeemService) reserveCodeRedemptionTx(tx *sql.Tx, codeID string, now time.Time) error {
	result, err := tx.Exec(
		`UPDATE redeem_codes
		    SET redeemed_count = redeemed_count + 1,
		        last_redeemed_at = ?,
		        updated_at = ?,
		        status = CASE
		            WHEN max_redemptions > 0 AND redeemed_count + 1 >= max_redemptions THEN ?
		            ELSE status
		        END
		  WHERE id = ? AND status = ? AND (max_redemptions <= 0 OR redeemed_count < max_redemptions)`,
		now,
		now,
		model.RedeemCodeStatusConsumed,
		codeID,
		model.RedeemCodeStatusActive,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRedeemCodeConsumed
	}
	return nil
}

func (s *RedeemService) insertRejectedLookupTx(tx *sql.Tx, lookup *redeemLookup, userID, username, codeInput, failureReason string, now time.Time) error {
	campaignID := lookup.CampaignID
	codeID := lookup.CodeID
	return s.insertRedemptionTx(tx, &model.RedeemRedemption{
		ID:                       uuid.New().String(),
		CampaignID:               &campaignID,
		CodeID:                   &codeID,
		UserID:                   userID,
		Username:                 username,
		CodeInput:                codeInput,
		CodeMask:                 lookup.CodeMask,
		SubscriptionPlanID:       lookup.SubscriptionPlanID,
		SubscriptionDurationDays: lookup.SubscriptionDurationDays,
		BalanceMicros:            lookup.BalanceMicros,
		Status:                   model.RedeemRedemptionStatusRejected,
		FailureReason:            failureReason,
		CreatedAt:                now,
	})
}

func (s *RedeemService) insertRedemptionTx(tx *sql.Tx, item *model.RedeemRedemption) error {
	if item == nil {
		return fmt.Errorf("insert redemption: nil item")
	}
	_, err := tx.Exec(
		`INSERT INTO redeem_redemptions (
			id, campaign_id, code_id, user_id, username, code_input, code_mask,
			subscription_plan_id, subscription_duration_days, balance_micros, status, failure_reason,
			granted_subscription_id, granted_expires_at, balance_after_micros, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID,
		item.CampaignID,
		item.CodeID,
		item.UserID,
		item.Username,
		item.CodeInput,
		item.CodeMask,
		item.SubscriptionPlanID,
		item.SubscriptionDurationDays,
		item.BalanceMicros,
		item.Status,
		item.FailureReason,
		item.GrantedSubscriptionID,
		item.GrantedExpiresAt,
		item.BalanceAfterMicros,
		item.CreatedAt,
	)
	return err
}

func (s *RedeemService) loadSharedCodeTx(tx *sql.Tx, campaignID string) (*model.RedeemCode, error) {
	item := &model.RedeemCode{}
	err := tx.QueryRow(
		`SELECT id, campaign_id, batch_id, code_value, code_hash, code_mask, status, max_redemptions, redeemed_count, last_redeemed_at, created_at, updated_at
		   FROM redeem_codes
		  WHERE campaign_id = ? AND batch_id IS NULL
		  ORDER BY created_at ASC
		  LIMIT 1`,
		campaignID,
	).Scan(
		&item.ID,
		&item.CampaignID,
		new(sql.NullString),
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
	return item, err
}

func normalizeRedeemCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func sanitizeRedeemSegment(value string) string {
	normalized := normalizeRedeemCode(value)
	var builder strings.Builder
	for _, char := range normalized {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func maskRedeemCode(value string) string {
	normalized := normalizeRedeemCode(value)
	if normalized == "" {
		return ""
	}
	if len(normalized) <= 2 {
		return strings.Repeat("*", len(normalized))
	}
	if len(normalized) <= 6 {
		return normalized[:1] + strings.Repeat("*", len(normalized)-2) + normalized[len(normalized)-1:]
	}
	if len(normalized) <= 8 {
		return normalized[:2] + strings.Repeat("*", len(normalized)-4) + normalized[len(normalized)-2:]
	}
	return normalized[:4] + strings.Repeat("*", len(normalized)-8) + normalized[len(normalized)-4:]
}

func hashRedeemCode(value string) string {
	sum := sha256.Sum256([]byte(normalizeRedeemCode(value)))
	return hex.EncodeToString(sum[:])
}

func randomRedeemString(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("invalid redeem length")
	}
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	var builder strings.Builder
	builder.Grow(length)
	for _, b := range bytes {
		builder.WriteByte(redeemAlphabet[int(b)%len(redeemAlphabet)])
	}
	return builder.String(), nil
}

func formatBalanceMicros(amountMicros int64) string {
	return fmt.Sprintf("%.6f", float64(amountMicros)/1e6)
}
