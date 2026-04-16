package service

import (
	"errors"

	"ampmanager/internal/model"
	"ampmanager/internal/precision"
	"ampmanager/internal/repository"
)

var (
	ErrGroupNotFound   = errors.New("分组不存在")
	ErrGroupNameExists = errors.New("分组名称已存在")
)

type GroupService struct {
	repo repository.GroupRepositoryInterface
}

func NewGroupService() *GroupService {
	return &GroupService{
		repo: repository.NewGroupRepository(),
	}
}

func (s *GroupService) Create(req *model.GroupRequest) (*model.GroupResponse, error) {
	existing, err := s.repo.GetByName(req.Name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrGroupNameExists
	}

	group := &model.Group{
		Name:        req.Name,
		Description: req.Description,
	}
	rateMultiplierPPM, err := resolveGroupRateMultiplierPPM(req)
	if err != nil {
		return nil, err
	}
	group.RateMultiplierPPM = rateMultiplierPPM
	group.RateMultiplier = precision.MultiplierPPMToFloat64(rateMultiplierPPM)

	if err := s.repo.Create(group); err != nil {
		return nil, err
	}

	return s.toResponse(group)
}

func (s *GroupService) GetByID(id string) (*model.GroupResponse, error) {
	group, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrGroupNotFound
	}
	return s.toResponse(group)
}

func (s *GroupService) List() ([]*model.GroupResponse, error) {
	groups, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	responses := make([]*model.GroupResponse, len(groups))
	for i, g := range groups {
		resp, err := s.toResponse(g)
		if err != nil {
			return nil, err
		}
		responses[i] = resp
	}
	return responses, nil
}

func (s *GroupService) Update(id string, req *model.GroupRequest) (*model.GroupResponse, error) {
	group, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrGroupNotFound
	}

	if group.Name != req.Name {
		existing, err := s.repo.GetByName(req.Name)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, ErrGroupNameExists
		}
	}

	group.Name = req.Name
	group.Description = req.Description
	rateMultiplierPPM, err := resolveGroupRateMultiplierPPM(req)
	if err != nil {
		return nil, err
	}
	group.RateMultiplierPPM = rateMultiplierPPM
	group.RateMultiplier = precision.MultiplierPPMToFloat64(rateMultiplierPPM)

	if err := s.repo.Update(group); err != nil {
		return nil, err
	}

	return s.toResponse(group)
}

func (s *GroupService) Delete(id string) error {
	group, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if group == nil {
		return ErrGroupNotFound
	}
	return s.repo.Delete(id)
}

func (s *GroupService) toResponse(group *model.Group) (*model.GroupResponse, error) {
	userCount, err := s.repo.CountUsers(group.ID)
	if err != nil {
		return nil, err
	}
	channelCount, err := s.repo.CountChannels(group.ID)
	if err != nil {
		return nil, err
	}

	return &model.GroupResponse{
		ID:                group.ID,
		Name:              group.Name,
		Description:       group.Description,
		RateMultiplierPPM: group.RateMultiplierPPM,
		RateMultiplier:    precision.MultiplierPPMToFloat64(group.RateMultiplierPPM),
		UserCount:         userCount,
		ChannelCount:      channelCount,
		CreatedAt:         group.CreatedAt,
		UpdatedAt:         group.UpdatedAt,
	}, nil
}

func resolveGroupRateMultiplierPPM(req *model.GroupRequest) (int64, error) {
	if req == nil {
		return precision.DefaultMultiplierPPM, nil
	}
	if req.RateMultiplierPPM != nil && *req.RateMultiplierPPM > 0 {
		return *req.RateMultiplierPPM, nil
	}
	return precision.ParseMultiplierToPPM(req.RateMultiplier)
}
