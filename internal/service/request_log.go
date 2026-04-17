package service

import (
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

type RequestLogService struct {
	repo *repository.RequestLogRepository
}

func NewRequestLogService() *RequestLogService {
	return &RequestLogService{
		repo: repository.NewRequestLogRepository(),
	}
}

// ListParams 列表查询参数
type ListRequestLogsParams struct {
	UserID         string
	APIKeyID       string
	SessionID      string
	Model          string
	Channel        string
	RequestFormat  string
	UpstreamFormat string
	StatusCodes    []int
	IsStreaming    *bool
	From           *time.Time
	To             *time.Time
	Page           int
	PageSize       int
}

// List 查询请求日志列表
func (s *RequestLogService) List(params ListRequestLogsParams) (*model.RequestLogListResponse, error) {
	repoParams := repository.ListParams{
		UserID:         params.UserID,
		APIKeyID:       params.APIKeyID,
		SessionID:      params.SessionID,
		Model:          params.Model,
		Channel:        params.Channel,
		RequestFormat:  params.RequestFormat,
		UpstreamFormat: params.UpstreamFormat,
		StatusCodes:    params.StatusCodes,
		IsStreaming:    params.IsStreaming,
		From:           params.From,
		To:             params.To,
		Page:           params.Page,
		PageSize:       params.PageSize,
	}

	logs, total, err := s.repo.List(repoParams)
	if err != nil {
		return nil, err
	}

	return &model.RequestLogListResponse{
		Items:    logs,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

// GetUsageSummary 获取用量统计（用户自身）
func (s *RequestLogService) GetUsageSummary(userID string, from, to *time.Time, groupBy string, modelFilter string) (*model.UsageSummaryResponse, error) {
	summaries, err := s.repo.GetUsageSummary(&userID, from, to, groupBy, modelFilter)
	if err != nil {
		return nil, err
	}

	return &model.UsageSummaryResponse{
		Items: summaries,
	}, nil
}

// GetUsageSummaryAdmin 获取用量统计（管理员，查看所有用户）
func (s *RequestLogService) GetUsageSummaryAdmin(userID *string, from, to *time.Time, groupBy string, modelFilter string) (*model.UsageSummaryResponse, error) {
	summaries, err := s.repo.GetUsageSummary(userID, from, to, groupBy, modelFilter)
	if err != nil {
		return nil, err
	}

	return &model.UsageSummaryResponse{
		Items: summaries,
	}, nil
}

// ListAdmin 管理员查询请求日志列表（可选按用户过滤）
func (s *RequestLogService) ListAdmin(params ListRequestLogsParams) (*model.RequestLogListResponse, error) {
	repoParams := repository.ListParams{
		UserID:         params.UserID, // UserID 可为空
		APIKeyID:       params.APIKeyID,
		SessionID:      params.SessionID,
		Model:          params.Model,
		Channel:        params.Channel,
		RequestFormat:  params.RequestFormat,
		UpstreamFormat: params.UpstreamFormat,
		StatusCodes:    params.StatusCodes,
		IsStreaming:    params.IsStreaming,
		From:           params.From,
		To:             params.To,
		Page:           params.Page,
		PageSize:       params.PageSize,
	}

	logs, total, err := s.repo.List(repoParams)
	if err != nil {
		return nil, err
	}

	return &model.RequestLogListResponse{
		Items:    logs,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

// GetDistinctModels 获取使用过的模型列表（管理员用）
func (s *RequestLogService) GetDistinctModels() ([]string, error) {
	return s.repo.GetDistinctModels()
}

// GetDistinctModelsByUser 获取指定用户使用过的模型列表
func (s *RequestLogService) GetDistinctModelsByUser(userID string) ([]string, error) {
	return s.repo.GetDistinctModelsByUser(userID)
}

// GetDistinctAPIKeys 获取使用过的 API Key 列表
func (s *RequestLogService) GetDistinctAPIKeys(userID string) ([]repository.DistinctAPIKey, error) {
	return s.repo.GetDistinctAPIKeys(userID)
}

// GetDashboardStats 获取仪表盘统计数据
func (s *RequestLogService) GetDashboardStats(userID string) (today, week, month repository.DashboardPeriodStats, topModels []repository.DashboardTopModel, dailyTrend []repository.DashboardDailyTrend, err error) {
	return s.repo.GetDashboardStats(userID)
}

// GetCacheHitRateByProvider 按提供商获取缓存命中率
func (s *RequestLogService) GetCacheHitRateByProvider(userID string) ([]repository.DashboardCacheHitRate, error) {
	return s.repo.GetCacheHitRateByProvider(userID)
}

// GetAdminDashboardStats 获取管理员仪表盘统计数据
func (s *RequestLogService) GetAdminDashboardStats(windowKey string) (today, week, month repository.DashboardPeriodStats, topModels []repository.DashboardTopModel, dailyTrend []repository.DashboardDailyTrend, throughputTrend []repository.DashboardThroughputPoint, ttfbTrend []repository.DashboardTimingPoint, durationTrend []repository.DashboardTimingPoint, err error) {
	return s.repo.GetAdminDashboardStats(windowKey)
}

// GetAdminCacheHitRateByProvider 管理员全局缓存命中率
func (s *RequestLogService) GetAdminCacheHitRateByProvider() ([]repository.DashboardCacheHitRate, error) {
	return s.repo.GetAdminCacheHitRateByProvider()
}

// GetByID 获取单条日志
func (s *RequestLogService) GetByID(id, userID string) (*model.RequestLog, error) {
	log, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 验证所有权
	if log != nil && log.UserID != userID {
		return nil, nil
	}

	return log, nil
}

func (s *RequestLogService) GetByIDAdmin(id string) (*model.RequestLog, error) {
	return s.repo.GetByID(id)
}
