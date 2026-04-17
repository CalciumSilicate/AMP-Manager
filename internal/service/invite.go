package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/google/uuid"
)

const (
	inviteEnabledKey         = "invite_enabled"
	inviteInviterRewardKey   = "invite_inviter_reward_micros"
	inviteInviteeRewardKey   = "invite_invitee_reward_micros"
	inviteMinFirstPaidCNYKey = "invite_min_first_paid_cny_cent"
	inviteCodeLength         = 10
)

var (
	ErrInviteDisabled         = errors.New("邀请码功能未开启")
	ErrInviteCodeInvalid      = errors.New("邀请码无效")
	ErrInviteAlreadyBound     = errors.New("该账号已绑定邀请关系")
	ErrInviteSelfNotAllowed   = errors.New("不能使用自己的邀请码")
	ErrInviteConfigIncomplete = errors.New("邀请奖励配置未完成")
)

type InviteService struct {
	userRepo   *repository.UserRepository
	repo       *repository.InviteRepository
	configRepo *repository.SystemConfigRepository
	grantSvc   *RewardGrantService
}

func NewInviteService() *InviteService {
	return &InviteService{
		userRepo:   repository.NewUserRepository(),
		repo:       repository.NewInviteRepository(),
		configRepo: repository.NewSystemConfigRepository(),
		grantSvc:   NewRewardGrantService(),
	}
}

func (s *InviteService) GetConfig() (model.InviteConfigResponse, error) {
	resp := model.InviteConfigResponse{}
	if value, err := s.configRepo.Get(inviteEnabledKey); err != nil {
		return resp, err
	} else if value != "" {
		resp.Enabled = value == "true"
	}
	if value, err := s.configRepo.Get(inviteInviterRewardKey); err != nil {
		return resp, err
	} else if value != "" {
		fmt.Sscan(value, &resp.InviterRewardMicros)
	}
	if value, err := s.configRepo.Get(inviteInviteeRewardKey); err != nil {
		return resp, err
	} else if value != "" {
		fmt.Sscan(value, &resp.InviteeRewardMicros)
	}
	if value, err := s.configRepo.Get(inviteMinFirstPaidCNYKey); err != nil {
		return resp, err
	} else if value != "" {
		fmt.Sscan(value, &resp.MinFirstPaidCNYCent)
	}
	resp.ConfigComplete = resp.InviterRewardMicros > 0 && resp.InviteeRewardMicros > 0
	return resp, nil
}

func (s *InviteService) SetConfig(req model.InviteConfigRequest) (model.InviteConfigResponse, error) {
	resp := model.InviteConfigResponse{
		Enabled:             req.Enabled,
		InviterRewardMicros: maxInt64(req.InviterRewardMicros, 0),
		InviteeRewardMicros: maxInt64(req.InviteeRewardMicros, 0),
		MinFirstPaidCNYCent: maxInt64(req.MinFirstPaidCNYCent, 0),
	}
	resp.ConfigComplete = resp.InviterRewardMicros > 0 && resp.InviteeRewardMicros > 0
	if resp.Enabled && !resp.ConfigComplete {
		return model.InviteConfigResponse{}, ErrInviteConfigIncomplete
	}
	if err := s.configRepo.Set(inviteEnabledKey, boolToConfigString(resp.Enabled)); err != nil {
		return model.InviteConfigResponse{}, err
	}
	if err := s.configRepo.Set(inviteInviterRewardKey, formatInt64(resp.InviterRewardMicros)); err != nil {
		return model.InviteConfigResponse{}, err
	}
	if err := s.configRepo.Set(inviteInviteeRewardKey, formatInt64(resp.InviteeRewardMicros)); err != nil {
		return model.InviteConfigResponse{}, err
	}
	if err := s.configRepo.Set(inviteMinFirstPaidCNYKey, formatInt64(resp.MinFirstPaidCNYCent)); err != nil {
		return model.InviteConfigResponse{}, err
	}
	return resp, nil
}

func (s *InviteService) IsPublicEnabled() bool {
	cfg, err := s.GetConfig()
	return err == nil && cfg.Enabled && cfg.ConfigComplete
}

func (s *InviteService) EnsureUserInviteCode(userID string) (string, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", repository.ErrUserNotFound
	}
	if strings.TrimSpace(user.InviteCode) != "" {
		return user.InviteCode, nil
	}
	return s.assignUniqueInviteCode(userID)
}

func (s *InviteService) AssignInviteCodeTx(tx *sql.Tx, userID string) (string, error) {
	for attempts := 0; attempts < 256; attempts++ {
		value, err := randomRedeemString(inviteCodeLength)
		if err != nil {
			return "", err
		}
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE invite_code = ?`, value).Scan(&count); err != nil {
			return "", err
		}
		if count > 0 {
			continue
		}
		if err := s.userRepo.SetInviteCodeTx(tx, userID, value); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				continue
			}
			return "", err
		}
		return value, nil
	}
	return "", fmt.Errorf("生成邀请码失败，请重试")
}

func (s *InviteService) BindInviteDuringRegistrationTx(tx *sql.Tx, inviteeUserID, rawInviteCode string, now time.Time) error {
	code := strings.TrimSpace(strings.ToUpper(rawInviteCode))
	if code == "" {
		return nil
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return ErrInviteDisabled
	}
	if !cfg.ConfigComplete {
		return ErrInviteConfigIncomplete
	}
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM invite_relationships WHERE invitee_user_id = ?`, inviteeUserID).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return ErrInviteAlreadyBound
	}
	var inviterID string
	if err := tx.QueryRow(`SELECT id FROM users WHERE invite_code = ?`, code).Scan(&inviterID); err != nil {
		if err == sql.ErrNoRows {
			return ErrInviteCodeInvalid
		}
		return err
	}
	if inviterID == inviteeUserID {
		return ErrInviteSelfNotAllowed
	}
	relation := &model.InviteRelationship{
		ID:            uuid.NewString(),
		InviterUserID: inviterID,
		InviterCode:   code,
		InviteeUserID: inviteeUserID,
		Status:        model.InviteRelationshipStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return s.repo.CreateRelationshipTx(tx, relation)
}

func (s *InviteService) GetSummary(userID string) (*model.InviteSummaryResponse, error) {
	inviteCode, err := s.EnsureUserInviteCode(userID)
	if err != nil {
		return nil, err
	}
	summary, err := s.repo.GetSummary(userID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	summary.Enabled = cfg.Enabled
	summary.ConfigComplete = cfg.ConfigComplete
	summary.InviteCode = inviteCode
	return summary, nil
}

func (s *InviteService) ListMyRewardEvents(userID string) ([]*model.InviteRewardEventResponse, error) {
	return s.repo.ListRewardEventsByUser(userID, 100)
}

func (s *InviteService) GetStats() (*model.InviteStatsResponse, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	stats, err := s.repo.GetStats()
	if err != nil {
		return nil, err
	}
	stats.Enabled = cfg.Enabled
	stats.ConfigComplete = cfg.ConfigComplete
	return stats, nil
}

func (s *InviteService) ListRelations() ([]*model.InviteRelationshipAdminResponse, error) {
	return s.repo.ListRelations(200)
}

func (s *InviteService) ListRewardEvents() ([]*model.InviteRewardEventResponse, error) {
	return s.repo.ListRewardEvents(200)
}

func (s *InviteService) HandlePaidOrderTx(tx *sql.Tx, order *model.PurchaseOrder, now time.Time) ([]BillingStateSyncAction, error) {
	if tx == nil || order == nil {
		return nil, nil
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || !cfg.ConfigComplete {
		return nil, nil
	}
	if order.AmountCNYCent <= 0 || order.AmountCNYCent < cfg.MinFirstPaidCNYCent {
		return nil, nil
	}
	relation, err := s.repo.GetRelationshipByInviteeUserIDTx(tx, order.UserID)
	if err != nil {
		return nil, err
	}
	if relation == nil || relation.Status == model.InviteRelationshipStatusRewarded {
		return nil, nil
	}

	actions := make([]BillingStateSyncAction, 0, 2)
	if cfg.InviterRewardMicros > 0 {
		if _, err := s.grantSvc.AdjustBalanceTx(tx, relation.InviterUserID, cfg.InviterRewardMicros, now); err != nil {
			return nil, err
		}
		if err := s.repo.CreateRewardEventTx(tx, &model.InviteRewardEvent{
			ID:                     uuid.NewString(),
			RelationID:             relation.ID,
			BeneficiaryUserID:      relation.InviterUserID,
			BeneficiaryRole:        model.InviteRewardBeneficiaryInviter,
			OrderID:                order.ID,
			OrderNo:                order.OrderNo,
			Status:                 model.InviteRewardEventStatusGranted,
			AmountMicros:           cfg.InviterRewardMicros,
			OrderPaidAmountCNYCent: order.AmountCNYCent,
			CreatedAt:              now,
		}); err != nil {
			return nil, err
		}
		actions = append(actions, BillingStateSyncAction{UserID: relation.InviterUserID, BalanceDeltaMicros: cfg.InviterRewardMicros})
	}
	if cfg.InviteeRewardMicros > 0 {
		if _, err := s.grantSvc.AdjustBalanceTx(tx, relation.InviteeUserID, cfg.InviteeRewardMicros, now); err != nil {
			return nil, err
		}
		if err := s.repo.CreateRewardEventTx(tx, &model.InviteRewardEvent{
			ID:                     uuid.NewString(),
			RelationID:             relation.ID,
			BeneficiaryUserID:      relation.InviteeUserID,
			BeneficiaryRole:        model.InviteRewardBeneficiaryInvitee,
			OrderID:                order.ID,
			OrderNo:                order.OrderNo,
			Status:                 model.InviteRewardEventStatusGranted,
			AmountMicros:           cfg.InviteeRewardMicros,
			OrderPaidAmountCNYCent: order.AmountCNYCent,
			CreatedAt:              now,
		}); err != nil {
			return nil, err
		}
		actions = append(actions, BillingStateSyncAction{UserID: relation.InviteeUserID, BalanceDeltaMicros: cfg.InviteeRewardMicros})
	}
	if err := s.repo.MarkRewardedTx(tx, relation.ID, order, now); err != nil {
		return nil, err
	}
	return actions, nil
}

func (s *InviteService) HandleRefundedOrderTx(tx *sql.Tx, order *model.PurchaseOrder, now time.Time) ([]BillingStateSyncAction, error) {
	if tx == nil || order == nil {
		return nil, nil
	}
	relation, err := s.repo.GetRelationshipByFirstPaidOrderNoTx(tx, order.OrderNo)
	if err != nil {
		return nil, err
	}
	if relation == nil {
		return nil, nil
	}
	grantedEvents, err := s.repo.ListRewardEventsByRelationOrderTx(tx, relation.ID, order.OrderNo, model.InviteRewardEventStatusGranted)
	if err != nil {
		return nil, err
	}
	if len(grantedEvents) == 0 {
		return nil, nil
	}
	actions := make([]BillingStateSyncAction, 0, len(grantedEvents))
	for _, event := range grantedEvents {
		reversed, err := s.repo.HasRewardEventTx(tx, relation.ID, event.BeneficiaryRole, model.InviteRewardEventStatusReversed, order.OrderNo)
		if err != nil {
			return nil, err
		}
		if reversed {
			continue
		}
		if _, err := s.grantSvc.AdjustBalanceTx(tx, event.BeneficiaryUserID, -event.AmountMicros, now); err != nil {
			return nil, err
		}
		if err := s.repo.CreateRewardEventTx(tx, &model.InviteRewardEvent{
			ID:                     uuid.NewString(),
			RelationID:             relation.ID,
			BeneficiaryUserID:      event.BeneficiaryUserID,
			BeneficiaryRole:        event.BeneficiaryRole,
			OrderID:                order.ID,
			OrderNo:                order.OrderNo,
			Status:                 model.InviteRewardEventStatusReversed,
			AmountMicros:           event.AmountMicros,
			OrderPaidAmountCNYCent: event.OrderPaidAmountCNYCent,
			CreatedAt:              now,
		}); err != nil {
			return nil, err
		}
		actions = append(actions, BillingStateSyncAction{UserID: event.BeneficiaryUserID, BalanceDeltaMicros: -event.AmountMicros})
	}
	if len(actions) == 0 {
		return nil, nil
	}
	if err := s.repo.ResetAfterRefundTx(tx, relation.ID, now); err != nil {
		return nil, err
	}
	return actions, nil
}

func (s *InviteService) assignUniqueInviteCode(userID string) (string, error) {
	for attempts := 0; attempts < 256; attempts++ {
		value, err := randomRedeemString(inviteCodeLength)
		if err != nil {
			return "", err
		}
		if err := s.userRepo.SetInviteCode(userID, value); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				continue
			}
			return "", err
		}
		return value, nil
	}
	return "", fmt.Errorf("生成邀请码失败，请重试")
}

func maxInt64(value, lower int64) int64 {
	if value < lower {
		return lower
	}
	return value
}

func (s *InviteService) SyncBillingStates(ctx context.Context, actions []BillingStateSyncAction) error {
	return SyncBillingStateActions(ctx, s.grantSvc, actions)
}
