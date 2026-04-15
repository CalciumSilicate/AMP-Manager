package service

import (
	"context"
	"errors"

	"ampmanager/internal/billingstate"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

var (
	ErrInvalidBillingSource = errors.New("无效的计费来源，必须为 subscription 或 balance")
)

type BillingSettingService struct {
	repo repository.BillingSettingRepositoryInterface
}

func NewBillingSettingService() *BillingSettingService {
	return &BillingSettingService{
		repo: repository.NewBillingSettingRepository(),
	}
}

func NewBillingSettingServiceWithRepo(repo repository.BillingSettingRepositoryInterface) *BillingSettingService {
	return &BillingSettingService{repo: repo}
}

func (s *BillingSettingService) Get(userID string) (*model.UserBillingSetting, error) {
	return s.repo.GetByUserID(userID)
}

func (s *BillingSettingService) Update(userID string, req *model.UpdateBillingPriorityRequest) (*model.UserBillingSetting, error) {
	if req.PrimarySource != model.BillingSourceSubscription && req.PrimarySource != model.BillingSourceBalance {
		return nil, ErrInvalidBillingSource
	}

	secondary := model.BillingSourceBalance
	if req.PrimarySource == model.BillingSourceBalance {
		secondary = model.BillingSourceSubscription
	}

	setting := &model.UserBillingSetting{
		UserID:          userID,
		PrimarySource:   req.PrimarySource,
		SecondarySource: secondary,
	}

	if err := s.repo.Upsert(setting); err != nil {
		return nil, err
	}

	if runtime := billingstate.Get(); runtime != nil {
		if err := runtime.RefreshUserState(context.Background(), userID); err != nil {
			return nil, err
		}
	}

	return s.repo.GetByUserID(userID)
}
