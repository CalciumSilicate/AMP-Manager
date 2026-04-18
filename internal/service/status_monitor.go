package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"ampmanager/internal/config"
	"ampmanager/internal/crypto"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	internaltranslator "ampmanager/internal/translator"

	log "github.com/sirupsen/logrus"
)

const (
	statusMonitorRuntimeConfigKey           = "status_monitor_runtime_config"
	defaultStatusMonitorPollIntervalSec     = 300
	defaultStatusMonitorDefaultTimeoutMs    = 45000
	defaultStatusMonitorRetentionDays       = 30
	defaultStatusMonitorDegradedThresholdMs = 10000
	statusMonitorWakeupInterval             = 30 * time.Second
	statusMonitorExecutionParallelism       = 4
)

var (
	ErrStatusMonitorNotFound     = errors.New("状态监控项不存在")
	globalStatusMonitorScheduler = &statusMonitorScheduler{}
)

type statusMonitorRuntimeConfigStored struct {
	Enabled              bool   `json:"enabled"`
	ServiceBaseURL       string `json:"serviceBaseUrl"`
	ServiceMonitorAPIKey string `json:"serviceMonitorApiKey"`
	PollIntervalSec      int    `json:"pollIntervalSec"`
	DefaultTimeoutMs     int64  `json:"defaultTimeoutMs"`
	RetentionDays        int    `json:"retentionDays"`
}

type statusMonitorScheduler struct {
	mu      sync.Mutex
	stopCh  chan struct{}
	wakeCh  chan struct{}
	running bool
}

type statusMonitorExecutionResult struct {
	status         model.StatusMonitorState
	latencyMs      int64
	ttfbMs         int64
	httpStatusCode int
	message        string
	endpointLabel  string
}

type StatusMonitorService struct {
	monitorRepo repository.StatusMonitorRepositoryInterface
	channelRepo repository.ChannelRepositoryInterface
	configRepo  *repository.SystemConfigRepository
}

func NewStatusMonitorService() *StatusMonitorService {
	return &StatusMonitorService{
		monitorRepo: repository.NewStatusMonitorRepository(),
		channelRepo: repository.NewChannelRepository(),
		configRepo:  repository.NewSystemConfigRepository(),
	}
}

func InitStatusMonitorScheduler() {
	globalStatusMonitorScheduler.Start()
}

func StopStatusMonitorScheduler() {
	globalStatusMonitorScheduler.Stop()
}

func NotifyStatusMonitorScheduler() {
	globalStatusMonitorScheduler.Wake()
}

func (s *statusMonitorScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}

	s.stopCh = make(chan struct{})
	s.wakeCh = make(chan struct{}, 1)
	s.running = true

	stopCh := s.stopCh
	wakeCh := s.wakeCh
	go s.loop(stopCh, wakeCh)
}

func (s *statusMonitorScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	close(s.stopCh)
	s.running = false
}

func (s *statusMonitorScheduler) Wake() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

func (s *statusMonitorScheduler) loop(stopCh <-chan struct{}, wakeCh <-chan struct{}) {
	service := NewStatusMonitorService()
	immediate := true

	for {
		cfg, err := service.getRuntimeConfigInternal()
		waitDuration := statusMonitorWakeupInterval
		if err != nil {
			log.Warnf("status monitor scheduler: load runtime config failed: %v", err)
		} else {
			if cfg.PollIntervalSec > 0 {
				waitDuration = time.Duration(cfg.PollIntervalSec) * time.Second
			}
			if cfg.Enabled || immediate {
				if cfg.Enabled {
					if _, runErr := service.RunAllEnabledMonitors(context.Background()); runErr != nil {
						log.Warnf("status monitor scheduler: run failed: %v", runErr)
					}
				}
			}
		}
		immediate = false

		timer := time.NewTimer(waitDuration)
		select {
		case <-stopCh:
			timer.Stop()
			return
		case <-wakeCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			immediate = true
		case <-timer.C:
		}
	}
}

func (s *StatusMonitorService) GetRuntimeConfig() (*model.StatusMonitorRuntimeConfigResponse, error) {
	cfg, err := s.getRuntimeConfigInternal()
	if err != nil {
		return nil, err
	}
	resp := s.toRuntimeConfigResponse(cfg)
	return &resp, nil
}

func (s *StatusMonitorService) UpdateRuntimeConfig(req model.StatusMonitorRuntimeConfigRequest) (*model.StatusMonitorRuntimeConfigResponse, error) {
	current, err := s.getRuntimeConfigInternal()
	if err != nil {
		return nil, err
	}

	next := current
	next.Enabled = req.Enabled
	next.ServiceBaseURL = strings.TrimRight(strings.TrimSpace(req.ServiceBaseURL), "/")
	next.PollIntervalSec = req.PollIntervalSec
	next.DefaultTimeoutMs = req.DefaultTimeoutMs
	next.RetentionDays = req.RetentionDays

	switch {
	case req.ClearServiceMonitorAPIKey:
		next.ServiceMonitorAPIKey = ""
	case req.RetainServiceMonitorAPIKey:
	case strings.TrimSpace(req.ServiceMonitorAPIKey) != "":
		next.ServiceMonitorAPIKey = strings.TrimSpace(req.ServiceMonitorAPIKey)
	default:
		next.ServiceMonitorAPIKey = ""
	}

	next = normalizeRuntimeConfig(next)
	if err := s.storeRuntimeConfig(next); err != nil {
		return nil, err
	}

	NotifyStatusMonitorScheduler()
	resp := s.toRuntimeConfigResponse(next)
	return &resp, nil
}

func (s *StatusMonitorService) ListMonitors() ([]*model.StatusMonitorResponse, error) {
	monitors, err := s.monitorRepo.ListAll()
	if err != nil {
		return nil, err
	}
	return s.toMonitorResponses(monitors)
}

func (s *StatusMonitorService) CreateMonitor(req *model.StatusMonitorRequest) (*model.StatusMonitorResponse, error) {
	monitor, err := s.buildMonitorForCreate(req)
	if err != nil {
		return nil, err
	}
	if err := s.monitorRepo.Create(monitor); err != nil {
		return nil, err
	}
	NotifyStatusMonitorScheduler()
	return s.toMonitorResponse(monitor)
}

func (s *StatusMonitorService) UpdateMonitor(id string, req *model.StatusMonitorRequest) (*model.StatusMonitorResponse, error) {
	current, err := s.monitorRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrStatusMonitorNotFound
	}

	next, err := s.buildMonitorForUpdate(current, req)
	if err != nil {
		return nil, err
	}
	if err := s.monitorRepo.Update(next); err != nil {
		return nil, err
	}
	NotifyStatusMonitorScheduler()
	return s.toMonitorResponse(next)
}

func (s *StatusMonitorService) DeleteMonitor(id string) error {
	item, err := s.monitorRepo.GetByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return ErrStatusMonitorNotFound
	}
	if err := s.monitorRepo.Delete(id); err != nil {
		return err
	}
	NotifyStatusMonitorScheduler()
	return nil
}

func (s *StatusMonitorService) RunMonitor(ctx context.Context, id string) (*model.StatusMonitorResult, error) {
	monitor, err := s.monitorRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if monitor == nil {
		return nil, ErrStatusMonitorNotFound
	}

	cfg, err := s.getRuntimeConfigInternal()
	if err != nil {
		return nil, err
	}
	result, err := s.executeMonitor(ctx, monitor, cfg)
	if err != nil {
		return nil, err
	}
	if cfg.RetentionDays > 0 {
		_ = s.monitorRepo.DeleteResultsOlderThan(time.Now().UTC().AddDate(0, 0, -cfg.RetentionDays))
	}
	return result, nil
}

func (s *StatusMonitorService) RunAllEnabledMonitors(ctx context.Context) (int, error) {
	cfg, err := s.getRuntimeConfigInternal()
	if err != nil {
		return 0, err
	}
	monitors, err := s.monitorRepo.ListEnabled()
	if err != nil {
		return 0, err
	}
	return len(monitors), s.runMonitors(ctx, monitors, cfg)
}

func (s *StatusMonitorService) HasConfiguredMonitors() (bool, error) {
	count, err := s.monitorRepo.CountAll()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *StatusMonitorService) GetDashboard(period string) (*model.StatusMonitorDashboardResponse, error) {
	monitors, err := s.monitorRepo.ListEnabled()
	if err != nil {
		return nil, err
	}

	cfg, err := s.getRuntimeConfigInternal()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	period = normalizeDashboardPeriod(period)
	periodDays := dashboardPeriodDays(period)
	lookbackDays := periodDays
	if lookbackDays < defaultStatusMonitorRetentionDays {
		lookbackDays = defaultStatusMonitorRetentionDays
	}
	results, err := s.monitorRepo.ListResultsSince(collectStatusMonitorIDs(monitors), now.AddDate(0, 0, -lookbackDays))
	if err != nil {
		return nil, err
	}

	resultsByMonitor := make(map[string][]*model.StatusMonitorResult, len(monitors))
	for _, result := range results {
		resultsByMonitor[result.MonitorID] = append(resultsByMonitor[result.MonitorID], result)
	}

	channelNames, err := s.channelNameMap(monitors)
	if err != nil {
		return nil, err
	}

	groupOrder := make([]string, 0)
	groupMap := make(map[string][]model.StatusMonitorDashboardItemResponse)
	summary := model.StatusMonitorSummaryCountsResponse{Total: len(monitors)}
	overallStatus := model.StatusMonitorStateUnknown
	var lastUpdated *time.Time
	periodCutoff := now.AddDate(0, 0, -periodDays)

	for _, monitor := range monitors {
		groupName := displayStatusMonitorGroupName(monitor.GroupName, monitor.TargetType)
		if _, exists := groupMap[groupName]; !exists {
			groupOrder = append(groupOrder, groupName)
		}

		monitorResults := resultsByMonitor[monitor.ID]
		latest := s.buildLatestDashboardResult(monitorResults)
		if latest.CheckedAt != nil {
			if lastUpdated == nil || latest.CheckedAt.After(*lastUpdated) {
				value := *latest.CheckedAt
				lastUpdated = &value
			}
		}

		switch latest.Status {
		case model.StatusMonitorStateOperational:
			summary.Operational++
		case model.StatusMonitorStateDegraded:
			summary.Degraded++
		case model.StatusMonitorStateError:
			summary.Error++
		case model.StatusMonitorStateFailed:
			summary.Failed++
		default:
			summary.Unknown++
		}
		if compareStatusSeverity(latest.Status, overallStatus) > 0 {
			overallStatus = latest.Status
		}

		periodResults := filterStatusMonitorResultsSince(monitorResults, periodCutoff)
		item := model.StatusMonitorDashboardItemResponse{
			ID:            monitor.ID,
			Name:          monitor.Name,
			TargetType:    monitor.TargetType,
			RequestFormat: monitor.RequestFormat,
			Model:         monitor.Model,
			ChannelName:   channelNames[monitor.ChannelID],
			EndpointLabel: s.resolveDashboardEndpointLabel(monitor, latest, cfg),
			Latest:        latest,
			Availability:  buildStatusMonitorAvailability(periodResults),
			History:       buildStatusMonitorHistory(periodResults, 60),
		}
		groupMap[groupName] = append(groupMap[groupName], item)
	}

	if overallStatus == model.StatusMonitorStateUnknown && len(monitors) == 0 {
		overallStatus = model.StatusMonitorStateUnknown
	}

	groups := make([]model.StatusMonitorDashboardGroupResponse, 0, len(groupOrder))
	for _, groupName := range groupOrder {
		groups = append(groups, model.StatusMonitorDashboardGroupResponse{
			GroupName: groupName,
			Items:     groupMap[groupName],
		})
	}

	return &model.StatusMonitorDashboardResponse{
		Period:          period,
		GeneratedAt:     now,
		LastUpdated:     lastUpdated,
		PollIntervalSec: cfg.PollIntervalSec,
		OverallStatus:   overallStatus,
		SummaryCounts:   summary,
		Groups:          groups,
	}, nil
}

func (s *StatusMonitorService) buildMonitorForCreate(req *model.StatusMonitorRequest) (*model.StatusMonitor, error) {
	headersJSON, bodyTemplate, err := s.resolveSensitiveFields("", "", req, false)
	if err != nil {
		return nil, err
	}
	return s.buildNormalizedMonitor(&model.StatusMonitor{
		HeadersJSON:  headersJSON,
		BodyTemplate: bodyTemplate,
	}, req)
}

func (s *StatusMonitorService) buildMonitorForUpdate(current *model.StatusMonitor, req *model.StatusMonitorRequest) (*model.StatusMonitor, error) {
	headersJSON, bodyTemplate, err := s.resolveSensitiveFields(current.HeadersJSON, current.BodyTemplate, req, true)
	if err != nil {
		return nil, err
	}
	base := *current
	base.HeadersJSON = headersJSON
	base.BodyTemplate = bodyTemplate
	return s.buildNormalizedMonitor(&base, req)
}

func (s *StatusMonitorService) buildNormalizedMonitor(base *model.StatusMonitor, req *model.StatusMonitorRequest) (*model.StatusMonitor, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("监控项名称不能为空")
	}
	if len(name) > 96 {
		return nil, errors.New("监控项名称长度不能超过 96 个字符")
	}
	groupName := strings.TrimSpace(req.GroupName)
	if len(groupName) > 64 {
		return nil, errors.New("分组名称长度不能超过 64 个字符")
	}

	targetType := req.TargetType
	switch targetType {
	case model.StatusMonitorTargetTypeServiceProxy, model.StatusMonitorTargetTypeChannelDirect, model.StatusMonitorTargetTypeCustomHTTP:
	default:
		return nil, errors.New("监控目标类型无效")
	}

	requestFormat := req.RequestFormat
	switch requestFormat {
	case "", model.ChannelEndpointChatCompletions, model.ChannelEndpointResponses, model.ChannelEndpointMessages, model.ChannelEndpointGenerateContent:
	default:
		return nil, errors.New("请求制式无效")
	}
	if requestFormat == "" {
		requestFormat = model.ChannelEndpointResponses
	}

	timeoutMs := req.TimeoutMs
	if timeoutMs < 0 {
		timeoutMs = 0
	}
	degradedThresholdMs := req.DegradedThresholdMs
	if degradedThresholdMs <= 0 {
		degradedThresholdMs = defaultStatusMonitorDegradedThresholdMs
	}

	expectedStatusCodes, err := normalizeExpectedStatusCodes(req.ExpectedStatusCodes)
	if err != nil {
		return nil, err
	}
	expectedStatusCodesJSON, err := json.Marshal(expectedStatusCodes)
	if err != nil {
		return nil, err
	}

	base.Name = name
	base.GroupName = groupName
	base.TargetType = targetType
	base.Enabled = req.Enabled
	base.SortOrder = req.SortOrder
	base.TimeoutMs = timeoutMs
	base.DegradedThresholdMs = degradedThresholdMs
	base.RequestFormat = requestFormat
	base.Model = strings.TrimSpace(req.Model)
	base.ChannelID = strings.TrimSpace(req.ChannelID)
	base.URL = strings.TrimSpace(req.URL)
	base.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	base.ExpectedStatusCodesJSON = string(expectedStatusCodesJSON)
	base.ExpectedSubstring = strings.TrimSpace(req.ExpectedSubstring)

	switch targetType {
	case model.StatusMonitorTargetTypeServiceProxy:
		if base.Model == "" {
			return nil, errors.New("服务层监控必须指定模型")
		}
		base.ChannelID = ""
		base.URL = ""
		base.Method = ""
		base.HeadersJSON = ""
		base.BodyTemplate = ""
		base.ExpectedSubstring = ""
		base.ExpectedStatusCodesJSON = "[]"
	case model.StatusMonitorTargetTypeChannelDirect:
		if base.ChannelID == "" {
			return nil, errors.New("上游渠道监控必须指定渠道")
		}
		if base.Model == "" {
			return nil, errors.New("上游渠道监控必须指定模型")
		}
		base.URL = ""
		base.Method = ""
		base.HeadersJSON = ""
		base.BodyTemplate = ""
		base.ExpectedSubstring = ""
		base.ExpectedStatusCodesJSON = "[]"
	case model.StatusMonitorTargetTypeCustomHTTP:
		if base.URL == "" {
			return nil, errors.New("自定义 HTTP 监控必须指定 URL")
		}
		parsedURL, err := url.Parse(base.URL)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return nil, errors.New("自定义 HTTP URL 无效")
		}
		if base.Method == "" {
			base.Method = http.MethodGet
		}
		if base.Method != http.MethodGet && base.Method != http.MethodPost {
			return nil, errors.New("自定义 HTTP 仅支持 GET 或 POST")
		}
		if base.HeadersJSON != "" {
			var headers map[string]string
			if err := json.Unmarshal([]byte(decryptStatusMonitorSecret(base.HeadersJSON)), &headers); err != nil {
				return nil, errors.New("头信息必须是 JSON 对象")
			}
		}
		base.ChannelID = ""
		base.Model = ""
	}

	return base, nil
}

func (s *StatusMonitorService) resolveSensitiveFields(currentHeadersJSON, currentBodyTemplate string, req *model.StatusMonitorRequest, isUpdate bool) (string, string, error) {
	headersJSON := currentHeadersJSON
	bodyTemplate := currentBodyTemplate

	switch {
	case req.ClearHeadersJSON:
		headersJSON = ""
	case isUpdate && req.RetainHeadersJSON:
	case strings.TrimSpace(req.HeadersJSON) != "":
		encrypted, err := encryptStatusMonitorSecret(strings.TrimSpace(req.HeadersJSON))
		if err != nil {
			return "", "", err
		}
		headersJSON = encrypted
	case !isUpdate:
		headersJSON = ""
	default:
		headersJSON = ""
	}

	switch {
	case req.ClearBodyTemplate:
		bodyTemplate = ""
	case isUpdate && req.RetainBodyTemplate:
	case strings.TrimSpace(req.BodyTemplate) != "":
		encrypted, err := encryptStatusMonitorSecret(req.BodyTemplate)
		if err != nil {
			return "", "", err
		}
		bodyTemplate = encrypted
	case !isUpdate:
		bodyTemplate = ""
	default:
		bodyTemplate = ""
	}

	return headersJSON, bodyTemplate, nil
}

func (s *StatusMonitorService) runMonitors(ctx context.Context, monitors []*model.StatusMonitor, cfg model.StatusMonitorRuntimeConfig) error {
	if len(monitors) == 0 {
		return nil
	}

	sem := make(chan struct{}, statusMonitorExecutionParallelism)
	errCh := make(chan error, len(monitors))
	var wg sync.WaitGroup

	for _, monitor := range monitors {
		current := monitor
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if _, err := s.executeMonitor(ctx, current, cfg); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	cleanupErr := error(nil)

	if cfg.RetentionDays > 0 {
		cleanupErr = s.monitorRepo.DeleteResultsOlderThan(time.Now().UTC().AddDate(0, 0, -cfg.RetentionDays))
	}
	close(errCh)

	var messages []string
	for err := range errCh {
		messages = append(messages, err.Error())
	}
	if cleanupErr != nil {
		messages = append(messages, cleanupErr.Error())
	}
	if len(messages) == 0 {
		return nil
	}
	sort.Strings(messages)
	return errors.New(strings.Join(messages, "; "))
}

func (s *StatusMonitorService) executeMonitor(parent context.Context, monitor *model.StatusMonitor, cfg model.StatusMonitorRuntimeConfig) (*model.StatusMonitorResult, error) {
	timeoutMs := monitor.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = cfg.DefaultTimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = defaultStatusMonitorDefaultTimeoutMs
	}

	ctx, cancel := context.WithTimeout(parent, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	var exec statusMonitorExecutionResult
	var err error
	switch monitor.TargetType {
	case model.StatusMonitorTargetTypeServiceProxy:
		exec, err = s.executeServiceProxyMonitor(ctx, monitor, cfg)
	case model.StatusMonitorTargetTypeChannelDirect:
		exec, err = s.executeChannelDirectMonitor(ctx, monitor)
	case model.StatusMonitorTargetTypeCustomHTTP:
		exec, err = s.executeCustomHTTPMonitor(ctx, monitor)
	default:
		err = errors.New("未知监控目标类型")
	}
	if err != nil {
		exec = statusMonitorExecutionResult{
			status:        model.StatusMonitorStateFailed,
			message:       err.Error(),
			endpointLabel: s.resolveMonitorEndpointLabel(monitor, cfg),
		}
	}

	if exec.status == model.StatusMonitorStateOperational && monitor.DegradedThresholdMs > 0 && exec.latencyMs > monitor.DegradedThresholdMs {
		exec.status = model.StatusMonitorStateDegraded
		if strings.TrimSpace(exec.message) == "" {
			exec.message = fmt.Sprintf("响应成功但耗时 %dms", exec.latencyMs)
		}
	}

	result := &model.StatusMonitorResult{
		MonitorID:      monitor.ID,
		Status:         exec.status,
		LatencyMs:      exec.latencyMs,
		TTFBMs:         exec.ttfbMs,
		HTTPStatusCode: exec.httpStatusCode,
		Message:        truncateStatusMonitorMessage(exec.message, 240),
		EndpointLabel:  exec.endpointLabel,
		CheckedAt:      time.Now().UTC(),
	}
	if err := s.monitorRepo.CreateResult(result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *StatusMonitorService) executeServiceProxyMonitor(ctx context.Context, monitor *model.StatusMonitor, cfg model.StatusMonitorRuntimeConfig) (statusMonitorExecutionResult, error) {
	if strings.TrimSpace(cfg.ServiceBaseURL) == "" {
		return statusMonitorExecutionResult{}, errors.New("未配置服务层探测地址")
	}
	if strings.TrimSpace(cfg.ServiceMonitorAPIKey) == "" {
		return statusMonitorExecutionResult{}, errors.New("未配置服务层探测 API Key")
	}

	testRequest := &model.TestChannelRequest{
		Format:         monitor.RequestFormat,
		Model:          monitor.Model,
		Prompt:         "输出 ok。",
		Instructions:   "请仅输出 ok。",
		ThinkingEffort: "low",
	}
	payload, err := buildChannelTestPayload(testRequest)
	if err != nil {
		return statusMonitorExecutionResult{}, err
	}

	targetURL, err := buildServiceProxyTestURL(cfg.ServiceBaseURL, monitor.RequestFormat, monitor.Model)
	if err != nil {
		return statusMonitorExecutionResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
	if err != nil {
		return statusMonitorExecutionResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+cfg.ServiceMonitorAPIKey)
	req.Header.Set("X-Api-Key", cfg.ServiceMonitorAPIKey)
	req.Header.Set("x-goog-api-key", cfg.ServiceMonitorAPIKey)

	return performStreamingStatusMonitorRequest(req, targetURL)
}

func (s *StatusMonitorService) executeChannelDirectMonitor(ctx context.Context, monitor *model.StatusMonitor) (statusMonitorExecutionResult, error) {
	channel, err := s.channelRepo.GetByID(monitor.ChannelID)
	if err != nil {
		return statusMonitorExecutionResult{}, err
	}
	if channel == nil {
		return statusMonitorExecutionResult{}, errors.New("关联渠道不存在")
	}

	testRequest := &model.TestChannelRequest{
		Format:         monitor.RequestFormat,
		Model:          monitor.Model,
		Prompt:         "输出 ok。",
		Instructions:   "请仅输出 ok。",
		ThinkingEffort: "low",
	}
	testPayload, err := buildChannelTestPayload(testRequest)
	if err != nil {
		return statusMonitorExecutionResult{}, err
	}

	channelService := NewChannelServiceWithRepo(s.channelRepo)
	incomingFormat := channelEndpointToFormat(monitor.RequestFormat)
	outgoingFormat := channelNativeFormat(channel)
	upstreamPayload := testPayload
	if !channelService.channelSupportsRequestFormat(channel, incomingFormat, true) {
		return statusMonitorExecutionResult{}, errors.New("当前渠道不支持该接口制式")
	}
	if !internaltranslator.Equivalent(incomingFormat, outgoingFormat) {
		translated, err := internaltranslator.TranslateRequest(incomingFormat, outgoingFormat, monitor.Model, testPayload, true)
		if err != nil {
			return statusMonitorExecutionResult{}, fmt.Errorf("转换测试请求失败: %w", err)
		}
		upstreamPayload = translated
	}

	targetURL, err := buildChannelTestURL(channel, monitor.Model)
	if err != nil {
		return statusMonitorExecutionResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(upstreamPayload))
	if err != nil {
		return statusMonitorExecutionResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	switch channel.Type {
	case model.ChannelTypeOpenAI:
		req.Header.Set("Authorization", "Bearer "+channel.APIKey)
	case model.ChannelTypeClaude:
		req.Header.Set("x-api-key", channel.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case model.ChannelTypeGemini:
		req.Header.Set("x-goog-api-key", channel.APIKey)
	}

	if headers, ok := getParsedHeaders(channel.HeadersJSON); ok {
		for key, value := range headers {
			if strings.TrimSpace(key) != "" {
				req.Header.Set(key, value)
			}
		}
	}

	return performStreamingStatusMonitorRequest(req, targetURL)
}

func (s *StatusMonitorService) executeCustomHTTPMonitor(ctx context.Context, monitor *model.StatusMonitor) (statusMonitorExecutionResult, error) {
	var body io.Reader
	bodyTemplate := decryptStatusMonitorSecret(monitor.BodyTemplate)
	if monitor.Method == http.MethodPost && strings.TrimSpace(bodyTemplate) != "" {
		body = strings.NewReader(bodyTemplate)
	}

	req, err := http.NewRequestWithContext(ctx, monitor.Method, monitor.URL, body)
	if err != nil {
		return statusMonitorExecutionResult{}, err
	}

	if monitor.Method == http.MethodPost && req.Header.Get("Content-Type") == "" && strings.TrimSpace(bodyTemplate) != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	if strings.TrimSpace(monitor.HeadersJSON) != "" {
		headers := map[string]string{}
		if err := json.Unmarshal([]byte(decryptStatusMonitorSecret(monitor.HeadersJSON)), &headers); err != nil {
			return statusMonitorExecutionResult{}, errors.New("头信息不是合法 JSON")
		}
		for key, value := range headers {
			if strings.TrimSpace(key) != "" {
				req.Header.Set(key, value)
			}
		}
	}

	expectedStatusCodes, err := parseExpectedStatusCodesJSON(monitor.ExpectedStatusCodesJSON)
	if err != nil {
		return statusMonitorExecutionResult{}, err
	}
	return performBufferedStatusMonitorRequest(req, monitor.URL, expectedStatusCodes, monitor.ExpectedSubstring)
}

func performStreamingStatusMonitorRequest(req *http.Request, endpointLabel string) (statusMonitorExecutionResult, error) {
	client := &http.Client{}
	start := time.Now()
	resp, err := client.Do(req)
	ttfb := time.Since(start).Milliseconds()
	if err != nil {
		return statusMonitorExecutionResult{
			status:        model.StatusMonitorStateFailed,
			latencyMs:     ttfb,
			ttfbMs:        ttfb,
			message:       fmt.Sprintf("连接失败: %v", err),
			endpointLabel: endpointLabel,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return statusMonitorExecutionResult{
			status:         model.StatusMonitorStateError,
			latencyMs:      ttfb,
			ttfbMs:         ttfb,
			httpStatusCode: resp.StatusCode,
			message:        buildChannelTestFailureMessage(resp.StatusCode, string(snippet)),
			endpointLabel:  endpointLabel,
		}, nil
	}

	firstByte := make([]byte, 1)
	if _, err := resp.Body.Read(firstByte); err != nil {
		if errors.Is(err, io.EOF) {
			return statusMonitorExecutionResult{
				status:         model.StatusMonitorStateFailed,
				latencyMs:      ttfb,
				ttfbMs:         ttfb,
				httpStatusCode: resp.StatusCode,
				message:        "连接成功，但未收到首包",
				endpointLabel:  endpointLabel,
			}, nil
		}
		return statusMonitorExecutionResult{
			status:         model.StatusMonitorStateFailed,
			latencyMs:      ttfb,
			ttfbMs:         ttfb,
			httpStatusCode: resp.StatusCode,
			message:        fmt.Sprintf("读取首包失败: %v", err),
			endpointLabel:  endpointLabel,
		}, nil
	}

	return statusMonitorExecutionResult{
		status:         model.StatusMonitorStateOperational,
		latencyMs:      time.Since(start).Milliseconds(),
		ttfbMs:         ttfb,
		httpStatusCode: resp.StatusCode,
		message:        fmt.Sprintf("已收到首包并断开 (HTTP %d)", resp.StatusCode),
		endpointLabel:  endpointLabel,
	}, nil
}

func performBufferedStatusMonitorRequest(req *http.Request, endpointLabel string, expectedStatusCodes []int, expectedSubstring string) (statusMonitorExecutionResult, error) {
	client := &http.Client{}
	start := time.Now()
	resp, err := client.Do(req)
	ttfb := time.Since(start).Milliseconds()
	if err != nil {
		return statusMonitorExecutionResult{
			status:        model.StatusMonitorStateFailed,
			latencyMs:     ttfb,
			ttfbMs:        ttfb,
			message:       fmt.Sprintf("连接失败: %v", err),
			endpointLabel: endpointLabel,
		}, nil
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	latency := time.Since(start).Milliseconds()
	if readErr != nil {
		return statusMonitorExecutionResult{
			status:         model.StatusMonitorStateFailed,
			latencyMs:      latency,
			ttfbMs:         ttfb,
			httpStatusCode: resp.StatusCode,
			message:        fmt.Sprintf("读取响应失败: %v", readErr),
			endpointLabel:  endpointLabel,
		}, nil
	}

	if !containsInt(expectedStatusCodes, resp.StatusCode) {
		return statusMonitorExecutionResult{
			status:         model.StatusMonitorStateError,
			latencyMs:      latency,
			ttfbMs:         ttfb,
			httpStatusCode: resp.StatusCode,
			message:        buildChannelTestFailureMessage(resp.StatusCode, string(bodyBytes)),
			endpointLabel:  endpointLabel,
		}, nil
	}

	if strings.TrimSpace(expectedSubstring) != "" && !strings.Contains(string(bodyBytes), expectedSubstring) {
		return statusMonitorExecutionResult{
			status:         model.StatusMonitorStateError,
			latencyMs:      latency,
			ttfbMs:         ttfb,
			httpStatusCode: resp.StatusCode,
			message:        "响应成功，但未匹配到预期文本",
			endpointLabel:  endpointLabel,
		}, nil
	}

	return statusMonitorExecutionResult{
		status:         model.StatusMonitorStateOperational,
		latencyMs:      latency,
		ttfbMs:         ttfb,
		httpStatusCode: resp.StatusCode,
		message:        fmt.Sprintf("请求成功 (HTTP %d)", resp.StatusCode),
		endpointLabel:  endpointLabel,
	}, nil
}

func buildServiceProxyTestURL(baseURL string, endpoint model.ChannelEndpoint, modelName string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch endpoint {
	case model.ChannelEndpointMessages:
		return baseURL + "/v1/messages", nil
	case model.ChannelEndpointGenerateContent:
		trimmedModel := strings.TrimPrefix(strings.TrimSpace(modelName), "models/")
		if trimmedModel == "" {
			return "", errors.New("Gemini 探测需要模型名称")
		}
		return fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent", baseURL, trimmedModel), nil
	case model.ChannelEndpointChatCompletions:
		return baseURL + "/v1/chat/completions", nil
	default:
		return baseURL + "/v1/responses", nil
	}
}

func (s *StatusMonitorService) toRuntimeConfigResponse(cfg model.StatusMonitorRuntimeConfig) model.StatusMonitorRuntimeConfigResponse {
	resp := model.StatusMonitorRuntimeConfigResponse{
		Enabled:                 cfg.Enabled,
		ServiceBaseURL:          cfg.ServiceBaseURL,
		PollIntervalSec:         cfg.PollIntervalSec,
		DefaultTimeoutMs:        cfg.DefaultTimeoutMs,
		RetentionDays:           cfg.RetentionDays,
		ServiceMonitorAPIKeySet: strings.TrimSpace(cfg.ServiceMonitorAPIKey) != "",
	}
	if resp.ServiceMonitorAPIKeySet {
		resp.ServiceMonitorAPIKeyMasked = maskSecretValue(cfg.ServiceMonitorAPIKey)
	}
	return resp
}

func (s *StatusMonitorService) toMonitorResponses(monitors []*model.StatusMonitor) ([]*model.StatusMonitorResponse, error) {
	responses := make([]*model.StatusMonitorResponse, 0, len(monitors))
	for _, monitor := range monitors {
		resp, err := s.toMonitorResponse(monitor)
		if err != nil {
			return nil, err
		}
		responses = append(responses, resp)
	}
	return responses, nil
}

func (s *StatusMonitorService) toMonitorResponse(monitor *model.StatusMonitor) (*model.StatusMonitorResponse, error) {
	if monitor == nil {
		return nil, nil
	}
	expectedStatusCodes, err := parseExpectedStatusCodesJSON(monitor.ExpectedStatusCodesJSON)
	if err != nil {
		return nil, err
	}

	resp := &model.StatusMonitorResponse{
		ID:                  monitor.ID,
		Name:                monitor.Name,
		GroupName:           monitor.GroupName,
		TargetType:          monitor.TargetType,
		Enabled:             monitor.Enabled,
		SortOrder:           monitor.SortOrder,
		TimeoutMs:           monitor.TimeoutMs,
		DegradedThresholdMs: monitor.DegradedThresholdMs,
		RequestFormat:       monitor.RequestFormat,
		Model:               monitor.Model,
		ChannelID:           monitor.ChannelID,
		URL:                 monitor.URL,
		Method:              monitor.Method,
		ExpectedStatusCodes: expectedStatusCodes,
		ExpectedSubstring:   monitor.ExpectedSubstring,
		CreatedAt:           monitor.CreatedAt,
		UpdatedAt:           monitor.UpdatedAt,
		HeadersSet:          strings.TrimSpace(monitor.HeadersJSON) != "",
		BodyTemplateSet:     strings.TrimSpace(monitor.BodyTemplate) != "",
	}
	if resp.HeadersSet {
		resp.HeadersMasked = "已配置敏感头信息"
	}
	if resp.BodyTemplateSet {
		resp.BodyTemplateMasked = "已配置请求体模板"
	}
	if strings.TrimSpace(monitor.ChannelID) != "" {
		channel, err := s.channelRepo.GetByID(monitor.ChannelID)
		if err != nil {
			return nil, err
		}
		if channel != nil {
			resp.ChannelName = channel.Name
		}
	}
	return resp, nil
}

func (s *StatusMonitorService) getRuntimeConfigInternal() (model.StatusMonitorRuntimeConfig, error) {
	cfg := model.StatusMonitorRuntimeConfig{}
	value, err := s.configRepo.Get(statusMonitorRuntimeConfigKey)
	if err != nil {
		return cfg, err
	}
	if strings.TrimSpace(value) == "" {
		return normalizeRuntimeConfig(cfg), nil
	}

	var stored statusMonitorRuntimeConfigStored
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return cfg, err
	}

	cfg = model.StatusMonitorRuntimeConfig{
		Enabled:              stored.Enabled,
		ServiceBaseURL:       stored.ServiceBaseURL,
		ServiceMonitorAPIKey: decryptStatusMonitorSecret(stored.ServiceMonitorAPIKey),
		PollIntervalSec:      stored.PollIntervalSec,
		DefaultTimeoutMs:     stored.DefaultTimeoutMs,
		RetentionDays:        stored.RetentionDays,
	}
	return normalizeRuntimeConfig(cfg), nil
}

func (s *StatusMonitorService) storeRuntimeConfig(cfg model.StatusMonitorRuntimeConfig) error {
	encryptedKey, err := encryptStatusMonitorSecret(cfg.ServiceMonitorAPIKey)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(statusMonitorRuntimeConfigStored{
		Enabled:              cfg.Enabled,
		ServiceBaseURL:       cfg.ServiceBaseURL,
		ServiceMonitorAPIKey: encryptedKey,
		PollIntervalSec:      cfg.PollIntervalSec,
		DefaultTimeoutMs:     cfg.DefaultTimeoutMs,
		RetentionDays:        cfg.RetentionDays,
	})
	if err != nil {
		return err
	}
	return s.configRepo.Set(statusMonitorRuntimeConfigKey, string(payload))
}

func normalizeRuntimeConfig(cfg model.StatusMonitorRuntimeConfig) model.StatusMonitorRuntimeConfig {
	cfg.ServiceBaseURL = strings.TrimRight(strings.TrimSpace(cfg.ServiceBaseURL), "/")
	if cfg.PollIntervalSec <= 0 {
		cfg.PollIntervalSec = defaultStatusMonitorPollIntervalSec
	}
	if cfg.DefaultTimeoutMs <= 0 {
		cfg.DefaultTimeoutMs = defaultStatusMonitorDefaultTimeoutMs
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = defaultStatusMonitorRetentionDays
	}
	return cfg
}

func parseExpectedStatusCodesJSON(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return []int{http.StatusOK}, nil
	}
	var codes []int
	if err := json.Unmarshal([]byte(raw), &codes); err != nil {
		return nil, err
	}
	return normalizeExpectedStatusCodes(codes)
}

func normalizeExpectedStatusCodes(values []int) ([]int, error) {
	if len(values) == 0 {
		return []int{http.StatusOK}, nil
	}
	unique := make(map[int]struct{}, len(values))
	codes := make([]int, 0, len(values))
	for _, value := range values {
		if value < 100 || value > 599 {
			return nil, errors.New("预期状态码必须位于 100-599 之间")
		}
		if _, exists := unique[value]; exists {
			continue
		}
		unique[value] = struct{}{}
		codes = append(codes, value)
	}
	sort.Ints(codes)
	return codes, nil
}

func encryptStatusMonitorSecret(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	key := config.Get().GetEncryptionKey()
	if key == nil {
		return value, nil
	}
	return crypto.Encrypt([]byte(value), key)
}

func decryptStatusMonitorSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	key := config.Get().GetEncryptionKey()
	if key == nil {
		return value
	}
	decrypted, err := crypto.Decrypt(value, key)
	if err != nil {
		return value
	}
	return string(decrypted)
}

func maskSecretValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return value[:2] + strings.Repeat("*", statusMonitorMinInt(len(value)-4, 8)) + value[len(value)-2:]
}

func statusMonitorMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func collectStatusMonitorIDs(monitors []*model.StatusMonitor) []string {
	ids := make([]string, 0, len(monitors))
	for _, monitor := range monitors {
		ids = append(ids, monitor.ID)
	}
	return ids
}

func normalizeDashboardPeriod(period string) string {
	switch period {
	case "15d", "30d":
		return period
	default:
		return "7d"
	}
}

func dashboardPeriodDays(period string) int {
	switch period {
	case "15d":
		return 15
	case "30d":
		return 30
	default:
		return 7
	}
}

func filterStatusMonitorResultsSince(results []*model.StatusMonitorResult, cutoff time.Time) []*model.StatusMonitorResult {
	filtered := make([]*model.StatusMonitorResult, 0, len(results))
	for _, result := range results {
		if result.CheckedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered
}

func buildStatusMonitorAvailability(results []*model.StatusMonitorResult) model.StatusMonitorAvailabilityResponse {
	availability := model.StatusMonitorAvailabilityResponse{TotalChecks: len(results)}
	for _, result := range results {
		if result.Status == model.StatusMonitorStateOperational {
			availability.OperationalCount++
		}
	}
	if availability.TotalChecks > 0 {
		availability.AvailabilityPct = float64(availability.OperationalCount) * 100 / float64(availability.TotalChecks)
	}
	return availability
}

func buildStatusMonitorHistory(results []*model.StatusMonitorResult, limit int) []model.StatusMonitorHistoryPointResponse {
	if len(results) == 0 {
		return []model.StatusMonitorHistoryPointResponse{}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	history := make([]model.StatusMonitorHistoryPointResponse, 0, len(results))
	for _, result := range results {
		history = append(history, model.StatusMonitorHistoryPointResponse{
			Status:    result.Status,
			TTFBMs:    result.TTFBMs,
			CheckedAt: result.CheckedAt,
		})
	}
	return history
}

func (s *StatusMonitorService) buildLatestDashboardResult(results []*model.StatusMonitorResult) model.StatusMonitorLatestResultResponse {
	if len(results) == 0 {
		return model.StatusMonitorLatestResultResponse{
			Status:  model.StatusMonitorStateUnknown,
			Message: "暂无探测记录",
		}
	}
	latest := results[0]
	checkedAt := latest.CheckedAt
	return model.StatusMonitorLatestResultResponse{
		Status:         latest.Status,
		LatencyMs:      latest.LatencyMs,
		TTFBMs:         latest.TTFBMs,
		HTTPStatusCode: latest.HTTPStatusCode,
		Message:        latest.Message,
		CheckedAt:      &checkedAt,
	}
}

func displayStatusMonitorGroupName(groupName string, targetType model.StatusMonitorTargetType) string {
	if strings.TrimSpace(groupName) != "" {
		return strings.TrimSpace(groupName)
	}
	switch targetType {
	case model.StatusMonitorTargetTypeServiceProxy:
		return "服务层"
	case model.StatusMonitorTargetTypeChannelDirect:
		return "上游层"
	default:
		return "自定义"
	}
}

func compareStatusSeverity(a, b model.StatusMonitorState) int {
	return statusSeverity(a) - statusSeverity(b)
}

func statusSeverity(state model.StatusMonitorState) int {
	switch state {
	case model.StatusMonitorStateFailed:
		return 5
	case model.StatusMonitorStateError:
		return 4
	case model.StatusMonitorStateDegraded:
		return 3
	case model.StatusMonitorStateOperational:
		return 2
	default:
		return 1
	}
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func truncateStatusMonitorMessage(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "…"
}

func (s *StatusMonitorService) channelNameMap(monitors []*model.StatusMonitor) (map[string]string, error) {
	result := make(map[string]string)
	needed := make(map[string]struct{})
	for _, monitor := range monitors {
		if strings.TrimSpace(monitor.ChannelID) != "" {
			needed[monitor.ChannelID] = struct{}{}
		}
	}
	for channelID := range needed {
		channel, err := s.channelRepo.GetByID(channelID)
		if err != nil {
			return nil, err
		}
		if channel != nil {
			result[channelID] = channel.Name
		}
	}
	return result, nil
}

func (s *StatusMonitorService) resolveDashboardEndpointLabel(monitor *model.StatusMonitor, latest model.StatusMonitorLatestResultResponse, cfg model.StatusMonitorRuntimeConfig) string {
	_ = latest
	return s.resolveMonitorEndpointLabel(monitor, cfg)
}

func (s *StatusMonitorService) resolveMonitorEndpointLabel(monitor *model.StatusMonitor, cfg model.StatusMonitorRuntimeConfig) string {
	switch monitor.TargetType {
	case model.StatusMonitorTargetTypeServiceProxy:
		label, err := buildServiceProxyTestURL(cfg.ServiceBaseURL, monitor.RequestFormat, monitor.Model)
		if err == nil {
			return label
		}
		return cfg.ServiceBaseURL
	case model.StatusMonitorTargetTypeChannelDirect:
		channel, err := s.channelRepo.GetByID(monitor.ChannelID)
		if err == nil && channel != nil {
			label, err := buildChannelTestURL(channel, monitor.Model)
			if err == nil {
				return label
			}
			return channel.BaseURL
		}
		return ""
	default:
		return monitor.URL
	}
}
