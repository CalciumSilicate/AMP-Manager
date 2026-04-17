package service

import (
	"net/http"
	"strings"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

type SessionService struct {
	repo      *repository.RequestLogRepository
	configSvc *SystemConfigService
}

func NewSessionService() *SessionService {
	return &SessionService{
		repo:      repository.NewRequestLogRepository(),
		configSvc: NewSystemConfigService(),
	}
}

func (s *SessionService) List(page, pageSize int, query string, activeOnly bool) (*model.AdminSessionListResponse, error) {
	cfg, err := s.configSvc.GetSessionStickyConfig()
	if err != nil {
		return nil, err
	}

	items, total, err := s.repo.ListSessions(repository.SessionListParams{
		Page:          page,
		PageSize:      pageSize,
		Query:         query,
		ActiveOnly:    activeOnly,
		WindowMinutes: cfg.WindowMinutes,
	})
	if err != nil {
		return nil, err
	}

	return &model.AdminSessionListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *SessionService) Get(sessionID string) (*model.AdminSessionDetailResponse, int, string) {
	cfg, err := s.configSvc.GetSessionStickyConfig()
	if err != nil {
		return nil, http.StatusInternalServerError, "获取 Session 配置失败"
	}

	detail, err := s.repo.GetSessionDetail(strings.TrimSpace(sessionID), time.Duration(cfg.WindowMinutes)*time.Minute)
	if err != nil {
		return nil, http.StatusInternalServerError, "获取 Session 详情失败"
	}
	if detail == nil {
		return nil, http.StatusNotFound, "Session 不存在"
	}
	return detail, 0, ""
}

func (s *SessionService) GetLeaderboard(windowMinutes, limit int) (*model.AdminSessionLeaderboardResponse, error) {
	if windowMinutes <= 0 {
		windowMinutes = 5
	}
	if limit <= 0 {
		limit = 10
	}
	items, err := s.repo.GetSessionLeaderboard(time.Duration(windowMinutes)*time.Minute, limit)
	if err != nil {
		return nil, err
	}
	return &model.AdminSessionLeaderboardResponse{
		WindowMinutes: windowMinutes,
		Items:         items,
	}, nil
}
