package service

import (
	"errors"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

var ErrAnnouncementNotFound = errors.New("公告不存在")

type AnnouncementService struct {
	repo     repository.AnnouncementRepositoryInterface
	userRepo repository.UserRepositoryInterface
}

func NewAnnouncementService() *AnnouncementService {
	return &AnnouncementService{
		repo:     repository.NewAnnouncementRepository(),
		userRepo: repository.NewUserRepository(),
	}
}

func (s *AnnouncementService) ListAdmin() ([]*model.AnnouncementResponse, error) {
	items, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}
	return s.toResponses(items, nil), nil
}

func (s *AnnouncementService) Create(req *model.AnnouncementRequest) (*model.AnnouncementResponse, error) {
	item := &model.Announcement{
		Title:    req.Title,
		Content:  req.Content,
		Audience: req.Audience,
		Pinned:   req.Pinned,
		Enabled:  req.Enabled,
	}
	if err := s.repo.Create(item); err != nil {
		return nil, err
	}
	return s.toResponse(item, nil), nil
}

func (s *AnnouncementService) Update(id string, req *model.AnnouncementRequest) (*model.AnnouncementResponse, error) {
	item, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrAnnouncementNotFound
	}

	item.Title = req.Title
	item.Content = req.Content
	item.Audience = req.Audience
	item.Pinned = req.Pinned
	item.Enabled = req.Enabled

	if err := s.repo.Update(item); err != nil {
		return nil, err
	}
	return s.toResponse(item, nil), nil
}

func (s *AnnouncementService) Delete(id string) error {
	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return ErrAnnouncementNotFound
	}
	return s.repo.Delete(id)
}

func (s *AnnouncementService) ListPublic() ([]*model.AnnouncementResponse, error) {
	items, err := s.repo.ListEnabledByAudiences([]model.AnnouncementAudience{model.AnnouncementAudiencePublic})
	if err != nil {
		return nil, err
	}
	return s.toResponses(items, nil), nil
}

func (s *AnnouncementService) ListForUser(userID string) ([]*model.AnnouncementResponse, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, repository.ErrUserNotFound
	}

	audiences := []model.AnnouncementAudience{
		model.AnnouncementAudiencePublic,
		model.AnnouncementAudienceAuthenticated,
	}
	audiences = append(audiences, model.AnnouncementAudienceNewUser)

	items, err := s.repo.ListEnabledByAudiences(audiences)
	if err != nil {
		return nil, err
	}
	filtered := make([]*model.Announcement, 0, len(items))
	for _, item := range items {
		if s.isAudienceApplicable(item, user.CreatedAt) {
			filtered = append(filtered, item)
		}
	}

	readMap, err := s.repo.GetReadMap(userID, collectAnnouncementIDs(filtered))
	if err != nil {
		return nil, err
	}
	return s.toResponses(filtered, readMap), nil
}

func (s *AnnouncementService) MarkRead(userID, announcementID string) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return repository.ErrUserNotFound
	}

	item, err := s.repo.GetByID(announcementID)
	if err != nil {
		return err
	}
	if item == nil {
		return ErrAnnouncementNotFound
	}

	if !item.Enabled {
		return ErrAnnouncementNotFound
	}

	if !s.isAudienceApplicable(item, user.CreatedAt) {
		return ErrAnnouncementNotFound
	}

	return s.repo.MarkRead(announcementID, userID)
}

func (s *AnnouncementService) isAudienceApplicable(item *model.Announcement, userCreatedAt time.Time) bool {
	switch item.Audience {
	case model.AnnouncementAudiencePublic, model.AnnouncementAudienceAuthenticated:
		return true
	case model.AnnouncementAudienceNewUser:
		return !userCreatedAt.Before(item.CreatedAt)
	default:
		return false
	}
}

func collectAnnouncementIDs(items []*model.Announcement) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func (s *AnnouncementService) toResponses(items []*model.Announcement, readMap map[string]time.Time) []*model.AnnouncementResponse {
	responses := make([]*model.AnnouncementResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, s.toResponse(item, readMap))
	}
	return responses
}

func (s *AnnouncementService) toResponse(item *model.Announcement, readMap map[string]time.Time) *model.AnnouncementResponse {
	resp := &model.AnnouncementResponse{
		ID:        item.ID,
		Title:     item.Title,
		Content:   item.Content,
		Audience:  item.Audience,
		Pinned:    item.Pinned,
		Enabled:   item.Enabled,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
	if readMap != nil {
		if readAt, ok := readMap[item.ID]; ok {
			resp.IsRead = true
			resp.ReadAt = &readAt
		}
	}
	return resp
}
