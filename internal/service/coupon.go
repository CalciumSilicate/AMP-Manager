package service

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
)

const couponEnabledKey = "coupon_enabled"

var (
	ErrCouponDisabled         = errors.New("优惠码功能未开启")
	ErrCouponCodeNotFound     = errors.New("优惠码不存在")
	ErrCouponCodeInvalid      = errors.New("优惠码当前不可用")
	ErrCouponPerUserLimit     = errors.New("该优惠码已达到你的使用次数上限")
	ErrCouponTotalLimit       = errors.New("该优惠码已达到总使用次数上限")
	ErrCouponModeLocked       = errors.New("优惠活动创建后不能修改码类型")
	ErrCouponSharedCodeNeeded = errors.New("共享码活动必须填写共享码")
	ErrCouponBatchUnsupported = errors.New("共享码活动不支持批量生码")
)

type AppliedCoupon struct {
	Campaign *model.CouponCampaign
	Code     *model.CouponCode
	Quote    *model.PurchaseCouponQuote
}

type CouponService struct {
	repo       *repository.CouponRepository
	configRepo *repository.SystemConfigRepository
}

func NewCouponService() *CouponService {
	return &CouponService{
		repo:       repository.NewCouponRepository(),
		configRepo: repository.NewSystemConfigRepository(),
	}
}

func (s *CouponService) GetConfig() (model.CouponConfigResponse, error) {
	resp := model.CouponConfigResponse{}
	if value, err := s.configRepo.Get(couponEnabledKey); err != nil {
		return resp, err
	} else if value != "" {
		resp.Enabled = value == "true"
	}
	return resp, nil
}

func (s *CouponService) SetConfig(req model.CouponConfigRequest) (model.CouponConfigResponse, error) {
	if err := s.configRepo.Set(couponEnabledKey, boolToConfigString(req.Enabled)); err != nil {
		return model.CouponConfigResponse{}, err
	}
	return model.CouponConfigResponse{Enabled: req.Enabled}, nil
}

func (s *CouponService) ListCampaigns() ([]*model.CouponCampaign, error) {
	return s.repo.ListCampaigns()
}

func (s *CouponService) CreateCampaign(req *model.CouponCampaignRequest) (*model.CouponCampaign, error) {
	if err := validateCouponCampaignRequest(req); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	campaign := &model.CouponCampaign{
		ID:                   uuid.NewString(),
		Name:                 strings.TrimSpace(req.Name),
		Description:          strings.TrimSpace(req.Description),
		CodeMode:             req.CodeMode,
		SharedCode:           normalizeCouponCode(req.SharedCode),
		DiscountType:         req.DiscountType,
		FixedDiscountCNYCent: maxInt64(req.FixedDiscountCNYCent, 0),
		PercentOffBPS:        maxInt(req.PercentOffBPS, 0),
		MaxDiscountCNYCent:   maxInt64(req.MaxDiscountCNYCent, 0),
		TotalUsageLimit:      maxInt(req.TotalUsageLimit, 0),
		PerUserLimit:         maxInt(req.PerUserLimit, 1),
		StartsAt:             req.StartsAt,
		EndsAt:               req.EndsAt,
		Enabled:              req.Enabled,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := s.repo.CreateCampaignTx(tx, campaign); err != nil {
		return nil, err
	}
	if campaign.CodeMode == model.CouponCodeModeShared {
		code := sharedCouponCodeModel(campaign, now)
		if err := s.repo.CreateCodeTx(tx, code); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return campaign, nil
}

func (s *CouponService) UpdateCampaign(id string, req *model.CouponCampaignRequest) (*model.CouponCampaign, error) {
	if err := validateCouponCampaignRequest(req); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetCampaignByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrCouponCodeNotFound
	}
	if existing.CodeMode != req.CodeMode {
		return nil, ErrCouponModeLocked
	}
	existing.Name = strings.TrimSpace(req.Name)
	existing.Description = strings.TrimSpace(req.Description)
	existing.SharedCode = normalizeCouponCode(req.SharedCode)
	existing.DiscountType = req.DiscountType
	existing.FixedDiscountCNYCent = maxInt64(req.FixedDiscountCNYCent, 0)
	existing.PercentOffBPS = maxInt(req.PercentOffBPS, 0)
	existing.MaxDiscountCNYCent = maxInt64(req.MaxDiscountCNYCent, 0)
	existing.TotalUsageLimit = maxInt(req.TotalUsageLimit, 0)
	existing.PerUserLimit = maxInt(req.PerUserLimit, 1)
	existing.StartsAt = req.StartsAt
	existing.EndsAt = req.EndsAt
	existing.Enabled = req.Enabled
	existing.UpdatedAt = time.Now().UTC()

	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := s.repo.UpdateCampaignTx(tx, existing); err != nil {
		return nil, err
	}
	if existing.CodeMode == model.CouponCodeModeShared {
		code, err := s.repo.GetSharedCodeByCampaignTx(tx, existing.ID)
		if err != nil {
			return nil, err
		}
		if code == nil {
			code = sharedCouponCodeModel(existing, existing.UpdatedAt)
			if err := s.repo.CreateCodeTx(tx, code); err != nil {
				return nil, err
			}
		} else {
			code.CodeValue = existing.SharedCode
			code.CodeHash = hashRedeemCode(existing.SharedCode)
			code.CodeMask = maskRedeemCode(existing.SharedCode)
			code.Status = couponStatusFromUsage(code.UsedCount, code.MaxUsages)
			code.UpdatedAt = existing.UpdatedAt
			if err := s.repo.UpdateSharedCodeTx(tx, code); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *CouponService) DeleteCampaign(id string) error {
	return s.repo.DeleteCampaign(id)
}

func (s *CouponService) CreateBatch(campaignID string, req *model.CouponBatchRequest) (*model.CouponBatch, []string, error) {
	campaign, err := s.repo.GetCampaignByID(campaignID)
	if err != nil {
		return nil, nil, err
	}
	if campaign == nil {
		return nil, nil, ErrCouponCodeNotFound
	}
	if campaign.CodeMode != model.CouponCodeModeSingleUse {
		return nil, nil, ErrCouponBatchUnsupported
	}
	now := time.Now().UTC()
	batch := &model.CouponBatch{
		ID:         uuid.NewString(),
		CampaignID: campaignID,
		Name:       strings.TrimSpace(req.Name),
		Prefix:     strings.TrimSpace(strings.ToUpper(req.Prefix)),
		CodeCount:  req.CodeCount,
		CodeLength: req.CodeLength,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	db := database.GetDB()
	tx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	if err := s.repo.CreateBatchTx(tx, batch); err != nil {
		return nil, nil, err
	}
	generated := make([]string, 0, req.CodeCount)
	seen := make(map[string]struct{}, req.CodeCount)
	for len(generated) < req.CodeCount {
		value, err := s.generateUniqueCodeTx(tx, batch.Prefix, batch.CodeLength, seen)
		if err != nil {
			return nil, nil, err
		}
		code := &model.CouponCode{
			ID:         uuid.NewString(),
			CampaignID: campaignID,
			BatchID:    batch.ID,
			CodeMode:   model.CouponCodeModeSingleUse,
			CodeValue:  value,
			CodeHash:   hashRedeemCode(value),
			CodeMask:   maskRedeemCode(value),
			Status:     model.CouponCodeStatusActive,
			MaxUsages:  1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := s.repo.CreateCodeTx(tx, code); err != nil {
			return nil, nil, err
		}
		generated = append(generated, value)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return batch, generated, nil
}

func (s *CouponService) ListCodes(campaignID, status, keyword string, limit int) ([]*model.CouponCode, error) {
	return s.repo.ListCodes(campaignID, status, keyword, limit)
}

func (s *CouponService) SetCodeEnabled(id string, enabled bool) error {
	status := model.CouponCodeStatusDisabled
	if enabled {
		status = model.CouponCodeStatusActive
	}
	return s.repo.SetCodeStatus(id, status)
}

func (s *CouponService) ListUsages(limit int) ([]*model.CouponUsageResponse, error) {
	return s.repo.ListUsages(limit)
}

func (s *CouponService) EvaluateCodeTx(tx *sql.Tx, userID, rawCode string, originalAmount int64, now time.Time) (*AppliedCoupon, error) {
	codeValue := normalizeCouponCode(rawCode)
	if codeValue == "" {
		return nil, nil
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, ErrCouponDisabled
	}
	if originalAmount <= 0 {
		return nil, ErrCouponCodeInvalid
	}
	code, campaign, err := s.repo.GetCodeWithCampaignTx(tx, codeValue)
	if err != nil {
		return nil, err
	}
	if code == nil || campaign == nil {
		return nil, ErrCouponCodeNotFound
	}
	if !campaign.Enabled || code.Status != model.CouponCodeStatusActive {
		return nil, ErrCouponCodeInvalid
	}
	if campaign.StartsAt != nil && now.Before(campaign.StartsAt.UTC()) {
		return nil, ErrCouponCodeInvalid
	}
	if campaign.EndsAt != nil && now.After(campaign.EndsAt.UTC()) {
		return nil, ErrCouponCodeInvalid
	}
	if campaign.TotalUsageLimit > 0 && campaign.UsedCount >= campaign.TotalUsageLimit {
		return nil, ErrCouponTotalLimit
	}
	if code.MaxUsages > 0 && code.UsedCount >= code.MaxUsages {
		return nil, ErrCouponCodeInvalid
	}
	perUserCount, err := s.repo.CountAppliedUsageByUserTx(tx, userID, campaign.ID)
	if err != nil {
		return nil, err
	}
	if campaign.PerUserLimit > 0 && perUserCount >= campaign.PerUserLimit {
		return nil, ErrCouponPerUserLimit
	}
	discount := calculateCouponDiscount(campaign, originalAmount)
	if discount <= 0 {
		return nil, ErrCouponCodeInvalid
	}
	return &AppliedCoupon{
		Campaign: campaign,
		Code:     code,
		Quote: &model.PurchaseCouponQuote{
			CampaignID:           campaign.ID,
			CampaignName:         campaign.Name,
			CodeID:               code.ID,
			CodeValue:            code.CodeValue,
			CodeMask:             code.CodeMask,
			DiscountType:         campaign.DiscountType,
			FixedDiscountCNYCent: campaign.FixedDiscountCNYCent,
			PercentOffBPS:        campaign.PercentOffBPS,
			MaxDiscountCNYCent:   campaign.MaxDiscountCNYCent,
			DiscountCNYCent:      discount,
		},
	}, nil
}

func (s *CouponService) ApplyToOrderTx(tx *sql.Tx, userID string, order *model.PurchaseOrder, rawCode string, originalAmount int64, now time.Time) (*model.PurchaseCouponQuote, error) {
	applied, err := s.EvaluateCodeTx(tx, userID, rawCode, originalAmount, now)
	if err != nil {
		return nil, err
	}
	if applied == nil {
		order.OriginalAmountCNYCent = originalAmount
		order.DiscountCNYCent = 0
		return nil, nil
	}
	s.ApplyQuoteToOrder(order, originalAmount, applied)
	if err := s.CommitUsageTx(tx, userID, order, applied, now); err != nil {
		return nil, err
	}
	return applied.Quote, nil
}

func (s *CouponService) ApplyQuoteToOrder(order *model.PurchaseOrder, originalAmount int64, applied *AppliedCoupon) {
	if order == nil {
		return
	}
	order.OriginalAmountCNYCent = originalAmount
	order.DiscountCNYCent = 0
	order.AmountCNYCent = originalAmount
	if applied == nil || applied.Quote == nil || applied.Campaign == nil || applied.Code == nil {
		return
	}
	finalAmount := originalAmount - applied.Quote.DiscountCNYCent
	if finalAmount < 0 {
		finalAmount = 0
	}
	order.DiscountCNYCent = applied.Quote.DiscountCNYCent
	order.AmountCNYCent = finalAmount
	order.CouponCampaignID = applied.Campaign.ID
	order.CouponCampaignName = applied.Campaign.Name
	order.CouponCodeID = applied.Code.ID
	order.CouponCodeValue = applied.Code.CodeValue
	order.CouponDiscountType = applied.Campaign.DiscountType
	order.CouponPercentOffBPS = applied.Campaign.PercentOffBPS
	order.CouponFixedDiscountCNYCent = applied.Campaign.FixedDiscountCNYCent
	order.CouponMaxDiscountCNYCent = applied.Campaign.MaxDiscountCNYCent
}

func (s *CouponService) CommitUsageTx(tx *sql.Tx, userID string, order *model.PurchaseOrder, applied *AppliedCoupon, now time.Time) error {
	if tx == nil || order == nil || applied == nil || applied.Campaign == nil || applied.Code == nil || applied.Quote == nil {
		return nil
	}
	usage := &model.CouponUsage{
		ID:                 uuid.NewString(),
		CampaignID:         applied.Campaign.ID,
		CodeID:             applied.Code.ID,
		UserID:             userID,
		OrderID:            order.ID,
		OrderNo:            order.OrderNo,
		Status:             model.CouponUsageStatusApplied,
		DiscountCNYCent:    applied.Quote.DiscountCNYCent,
		FinalAmountCNYCent: order.AmountCNYCent,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.repo.CreateUsageTx(tx, usage); err != nil {
		return err
	}
	return s.repo.ApplyUsageCountersTx(tx, applied.Campaign.ID, applied.Code.ID, 1, couponStatusFromUsage(applied.Code.UsedCount+1, applied.Code.MaxUsages), now)
}

func (s *CouponService) ReverseUsageForOrderTx(tx *sql.Tx, order *model.PurchaseOrder, now time.Time) error {
	if tx == nil || order == nil {
		return nil
	}
	usage, err := s.repo.GetUsageByOrderIDTx(tx, order.ID)
	if err != nil {
		return err
	}
	if usage == nil || usage.Status != model.CouponUsageStatusApplied {
		return nil
	}
	if err := s.repo.MarkUsageReversedTx(tx, usage.ID, now); err != nil {
		return err
	}
	return s.repo.ApplyUsageCountersTx(tx, usage.CampaignID, usage.CodeID, -1, model.CouponCodeStatusActive, now)
}

func (s *CouponService) generateUniqueCodeTx(tx *sql.Tx, prefix string, randomLength int, seen map[string]struct{}) (string, error) {
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
		if err := tx.QueryRow(`SELECT COUNT(*) FROM coupon_codes WHERE code_hash = ?`, hashRedeemCode(value)).Scan(&count); err != nil {
			return "", err
		}
		if count > 0 {
			continue
		}
		seen[value] = struct{}{}
		return value, nil
	}
	return "", fmt.Errorf("生成优惠码失败，请重试")
}

func calculateCouponDiscount(campaign *model.CouponCampaign, originalAmount int64) int64 {
	if campaign == nil || originalAmount <= 0 {
		return 0
	}
	switch campaign.DiscountType {
	case model.CouponDiscountTypeFixed:
		if campaign.FixedDiscountCNYCent <= 0 {
			return 0
		}
		if campaign.FixedDiscountCNYCent > originalAmount {
			return originalAmount
		}
		return campaign.FixedDiscountCNYCent
	case model.CouponDiscountTypePercentage:
		if campaign.PercentOffBPS <= 0 {
			return 0
		}
		discount := int64(math.Round(float64(originalAmount) * float64(campaign.PercentOffBPS) / 10000))
		if campaign.MaxDiscountCNYCent > 0 && discount > campaign.MaxDiscountCNYCent {
			discount = campaign.MaxDiscountCNYCent
		}
		if discount > originalAmount {
			discount = originalAmount
		}
		return discount
	default:
		return 0
	}
}

func validateCouponCampaignRequest(req *model.CouponCampaignRequest) error {
	if req == nil {
		return ErrCouponCodeInvalid
	}
	if req.CodeMode == model.CouponCodeModeShared && normalizeCouponCode(req.SharedCode) == "" {
		return ErrCouponSharedCodeNeeded
	}
	if req.DiscountType == model.CouponDiscountTypeFixed && req.FixedDiscountCNYCent <= 0 {
		return ErrCouponCodeInvalid
	}
	if req.DiscountType == model.CouponDiscountTypePercentage && req.PercentOffBPS <= 0 {
		return ErrCouponCodeInvalid
	}
	if req.PerUserLimit <= 0 {
		req.PerUserLimit = 1
	}
	return nil
}

func sharedCouponCodeModel(campaign *model.CouponCampaign, now time.Time) *model.CouponCode {
	return &model.CouponCode{
		ID:         uuid.NewString(),
		CampaignID: campaign.ID,
		CodeMode:   model.CouponCodeModeShared,
		CodeValue:  campaign.SharedCode,
		CodeHash:   hashRedeemCode(campaign.SharedCode),
		CodeMask:   maskRedeemCode(campaign.SharedCode),
		Status:     model.CouponCodeStatusActive,
		MaxUsages:  0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func couponStatusFromUsage(usedCount, maxUsages int) model.CouponCodeStatus {
	if maxUsages > 0 && usedCount >= maxUsages {
		return model.CouponCodeStatusConsumed
	}
	return model.CouponCodeStatusActive
}

func normalizeCouponCode(value string) string {
	return strings.TrimSpace(strings.ToUpper(value))
}

func maxInt(value, lower int) int {
	if value < lower {
		return lower
	}
	return value
}
