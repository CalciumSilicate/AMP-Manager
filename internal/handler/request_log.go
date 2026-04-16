package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ampmanager/internal/amp"
	"ampmanager/internal/middleware"
	"ampmanager/internal/model"
	"ampmanager/internal/realtime"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
	"nhooyr.io/websocket"
)

type RequestLogHandler struct {
	logService *service.RequestLogService
}

func parseStatusCodes(values []string) ([]int, error) {
	if len(values) == 0 {
		return nil, nil
	}

	seen := make(map[int]struct{}, len(values))
	statusCodes := make([]int, 0, len(values))
	for _, rawValue := range values {
		for _, part := range strings.Split(rawValue, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			statusCode, err := strconv.Atoi(trimmed)
			if err != nil {
				return nil, err
			}
			if _, exists := seen[statusCode]; exists {
				continue
			}
			seen[statusCode] = struct{}{}
			statusCodes = append(statusCodes, statusCode)
		}
	}
	return statusCodes, nil
}

func NewRequestLogHandler() *RequestLogHandler {
	return &RequestLogHandler{
		logService: service.NewRequestLogService(),
	}
}

// ListRequestLogs 获取请求日志列表
func (h *RequestLogHandler) ListRequestLogs(c *gin.Context) {
	userID := middleware.GetUserID(c)

	params := service.ListRequestLogsParams{
		UserID:   userID,
		Page:     1,
		PageSize: 20,
	}

	// 解析查询参数
	if page, err := strconv.Atoi(c.Query("page")); err == nil && page > 0 {
		params.Page = page
	}
	if pageSize, err := strconv.Atoi(c.Query("pageSize")); err == nil && pageSize > 0 {
		params.PageSize = pageSize
	}
	if apiKeyID := c.Query("apiKeyId"); apiKeyID != "" {
		params.APIKeyID = apiKeyID
	}
	if model := c.Query("model"); model != "" {
		params.Model = model
	}
	if rawStatuses := c.QueryArray("status"); len(rawStatuses) > 0 {
		statusCodes, err := parseStatusCodes(rawStatuses)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status 参数无效，应为整数"})
			return
		}
		params.StatusCodes = statusCodes
	}
	if isStreaming := c.Query("isStreaming"); isStreaming != "" {
		val := isStreaming == "true" || isStreaming == "1"
		params.IsStreaming = &val
	}
	if from := c.Query("from"); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from 时间格式错误，应为 RFC3339 格式"})
			return
		}
		params.From = &t
	}
	if to := c.Query("to"); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "to 时间格式错误，应为 RFC3339 格式"})
			return
		}
		params.To = &t
	}

	result, err := h.logService.List(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取日志失败"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetRequestLog 获取单条日志详情
func (h *RequestLogHandler) GetRequestLog(c *gin.Context) {
	userID := middleware.GetUserID(c)
	logID := c.Param("id")

	log, err := h.logService.GetByID(logID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取日志失败"})
		return
	}

	if log == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "日志不存在"})
		return
	}

	c.JSON(http.StatusOK, log)
}

// GetDistinctModels 获取当前用户使用过的模型列表
func (h *RequestLogHandler) GetDistinctModels(c *gin.Context) {
	userID := middleware.GetUserID(c)
	models, err := h.logService.GetDistinctModelsByUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"models": models})
}

// GetDistinctAPIKeys 获取当前用户使用过的 API Key 列表
func (h *RequestLogHandler) GetDistinctAPIKeys(c *gin.Context) {
	userID := middleware.GetUserID(c)
	keys, err := h.logService.GetDistinctAPIKeys(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Key 列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"keys": keys})
}

// GetUsageSummary 获取用量统计
func (h *RequestLogHandler) GetUsageSummary(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var from, to *time.Time
	if fromStr := c.Query("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from 时间格式错误，应为 RFC3339 格式"})
			return
		}
		from = &t
	}
	if toStr := c.Query("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "to 时间格式错误，应为 RFC3339 格式"})
			return
		}
		to = &t
	}

	groupBy := c.DefaultQuery("groupBy", "day")
	allowedGroupBy := map[string]bool{"day": true, "model": true, "apiKey": true}
	if !allowedGroupBy[groupBy] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "groupBy 参数无效，允许值: day, model, apiKey"})
		return
	}

	modelFilter := c.Query("model")

	result, err := h.logService.GetUsageSummary(userID, from, to, groupBy, modelFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取统计失败"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// AdminListRequestLogs 管理员获取所有请求日志列表
func (h *RequestLogHandler) AdminListRequestLogs(c *gin.Context) {
	params := service.ListRequestLogsParams{
		Page:     1,
		PageSize: 20,
	}

	// 解析查询参数
	if page, err := strconv.Atoi(c.Query("page")); err == nil && page > 0 {
		params.Page = page
	}
	if pageSize, err := strconv.Atoi(c.Query("pageSize")); err == nil && pageSize > 0 {
		params.PageSize = pageSize
	}
	// 可选：按用户过滤
	if userID := c.Query("userId"); userID != "" {
		params.UserID = userID
	}
	if apiKeyID := c.Query("apiKeyId"); apiKeyID != "" {
		params.APIKeyID = apiKeyID
	}
	if model := c.Query("model"); model != "" {
		params.Model = model
	}
	if rawStatuses := c.QueryArray("status"); len(rawStatuses) > 0 {
		statusCodes, err := parseStatusCodes(rawStatuses)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status 参数无效，应为整数"})
			return
		}
		params.StatusCodes = statusCodes
	}
	if isStreaming := c.Query("isStreaming"); isStreaming != "" {
		val := isStreaming == "true" || isStreaming == "1"
		params.IsStreaming = &val
	}
	if from := c.Query("from"); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from 时间格式错误，应为 RFC3339 格式"})
			return
		}
		params.From = &t
	}
	if to := c.Query("to"); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "to 时间格式错误，应为 RFC3339 格式"})
			return
		}
		params.To = &t
	}

	result, err := h.logService.ListAdmin(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取日志失败"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// AdminGetDistinctModels 管理员获取使用过的模型列表
func (h *RequestLogHandler) AdminGetDistinctModels(c *gin.Context) {
	models, err := h.logService.GetDistinctModels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"models": models})
}

// AdminGetDistinctAPIKeys 管理员获取使用过的 API Key 列表
func (h *RequestLogHandler) AdminGetDistinctAPIKeys(c *gin.Context) {
	userID := c.Query("userId")
	keys, err := h.logService.GetDistinctAPIKeys(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Key 列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"keys": keys})
}

// AdminGetUsageSummary 管理员获取全局用量统计
func (h *RequestLogHandler) AdminGetUsageSummary(c *gin.Context) {
	var from, to *time.Time
	if fromStr := c.Query("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from 时间格式错误，应为 RFC3339 格式"})
			return
		}
		from = &t
	}
	if toStr := c.Query("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "to 时间格式错误，应为 RFC3339 格式"})
			return
		}
		to = &t
	}

	groupBy := c.DefaultQuery("groupBy", "day")
	allowedGroupBy := map[string]bool{"day": true, "model": true, "apiKey": true, "user": true}
	if !allowedGroupBy[groupBy] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "groupBy 参数无效，允许值: day, model, apiKey, user"})
		return
	}

	// 可选：按用户过滤
	var userID *string
	if uid := c.Query("userId"); uid != "" {
		userID = &uid
	}

	modelFilter := c.Query("model")

	result, err := h.logService.GetUsageSummaryAdmin(userID, from, to, groupBy, modelFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取统计失败"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// AdminGetRequestLogDetail 管理员获取请求日志详情（含请求/响应头和体）
func (h *RequestLogHandler) AdminGetRequestLogDetail(c *gin.Context) {
	logID := c.Param("id")
	if logID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "日志 ID 不能为空"})
		return
	}

	store := amp.GetRequestDetailStore()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "详情存储未初始化"})
		return
	}

	detail := store.Get(logID)
	if detail == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "日志详情不存在或已过期"})
		return
	}
	logEntry, err := h.logService.GetByIDAdmin(logID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取日志计费信息失败"})
		return
	}
	if logEntry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "日志不存在"})
		return
	}

	// Convert headers to map[string]string for JSON response
	requestHeaders := make(map[string]string)
	for k, v := range detail.RequestHeaders {
		if len(v) > 0 {
			requestHeaders[k] = v[0]
		}
	}

	responseHeaders := make(map[string]string)
	for k, v := range detail.ResponseHeaders {
		if len(v) > 0 {
			responseHeaders[k] = v[0]
		}
	}

	translatedRequestHeaders := make(map[string]string)
	for k, v := range detail.TranslatedRequestHeaders {
		if len(v) > 0 {
			translatedRequestHeaders[k] = v[0]
		}
	}

	result := &model.RequestLogDetail{
		RequestID:                 detail.RequestID,
		RequestHeaders:            requestHeaders,
		RequestBody:               string(detail.RequestBody),
		TranslatedRequestBody:     string(detail.TranslatedRequestBody),
		TranslatedRequestHeaders:  translatedRequestHeaders,
		ResponseHeaders:           responseHeaders,
		ResponseBody:              string(detail.ResponseBody),
		TranslatedResponseBody:    string(detail.TranslatedResponseBody),
		CreatedAt:                 detail.CreatedAt,
		BillingStatus:             logEntry.BillingStatus,
		ChargedSubscriptionMicros: logEntry.ChargedSubscriptionMicros,
		ChargedBalanceMicros:      logEntry.ChargedBalanceMicros,
		BillingGapMicros:          logEntry.BillingGapMicros,
		CostMicros:                logEntry.CostMicros,
		CostUsd:                   logEntry.CostUsd,
	}

	c.JSON(http.StatusOK, result)
}

// GetDashboard 获取用户仪表盘数据
func (h *RequestLogHandler) GetDashboard(c *gin.Context) {
	userID := middleware.GetUserID(c)

	userService := service.NewUserService()
	balance, err := userService.GetBalance(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取余额失败"})
		return
	}

	today, week, month, topModels, dailyTrend, err := h.logService.GetDashboardStats(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取统计数据失败"})
		return
	}

	formatPeriod := func(s repository.DashboardPeriodStats) gin.H {
		return gin.H{
			"requestCount":    s.RequestCount,
			"inputTokensSum":  s.InputTokensSum,
			"outputTokensSum": s.OutputTokensSum,
			"costMicros":      s.CostMicrosSum,
			"costUsd":         fmt.Sprintf("%.6f", float64(s.CostMicrosSum)/1e6),
			"errorCount":      s.ErrorCount,
		}
	}

	topModelsList := make([]gin.H, 0, len(topModels))
	for _, m := range topModels {
		topModelsList = append(topModelsList, gin.H{
			"model":        m.Model,
			"requestCount": m.RequestCount,
			"costMicros":   m.CostMicros,
			"costUsd":      fmt.Sprintf("%.6f", float64(m.CostMicros)/1e6),
		})
	}

	trendList := make([]gin.H, 0, len(dailyTrend))
	for _, d := range dailyTrend {
		trendList = append(trendList, gin.H{
			"date":       d.Date,
			"costMicros": d.CostMicros,
			"costUsd":    fmt.Sprintf("%.6f", float64(d.CostMicros)/1e6),
			"requests":   d.Requests,
		})
	}

	cacheHitRates, err := h.logService.GetCacheHitRateByProvider(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取缓存命中率失败"})
		return
	}

	cacheHitRateList := make([]gin.H, 0, len(cacheHitRates))
	for _, r := range cacheHitRates {
		cacheHitRateList = append(cacheHitRateList, gin.H{
			"provider":            r.Provider,
			"totalInputTokens":    r.TotalInputTokens,
			"cacheReadTokens":     r.CacheReadTokens,
			"cacheCreationTokens": r.CacheCreationTokens,
			"requestCount":        r.RequestCount,
			"hitRate":             fmt.Sprintf("%.1f", r.HitRate),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"balance": gin.H{
			"balanceMicros": balance,
			"balanceUsd":    fmt.Sprintf("%.6f", float64(balance)/1e6),
		},
		"today":         formatPeriod(today),
		"week":          formatPeriod(week),
		"month":         formatPeriod(month),
		"topModels":     topModelsList,
		"dailyTrend":    trendList,
		"cacheHitRates": cacheHitRateList,
	})
}

// GetAdminDashboard 获取管理员仪表盘数据（全局汇总）
func (h *RequestLogHandler) GetAdminDashboard(c *gin.Context) {
	userService := service.NewUserService()
	totalBalance, userCount, err := userService.GetTotalBalanceAndUserCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取余额失败"})
		return
	}

	today, week, month, topModels, dailyTrend, throughputTrend, ttfbTrend, durationTrend, err := h.logService.GetAdminDashboardStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取统计数据失败"})
		return
	}

	formatPeriod := func(s repository.DashboardPeriodStats) gin.H {
		return gin.H{
			"requestCount":    s.RequestCount,
			"inputTokensSum":  s.InputTokensSum,
			"outputTokensSum": s.OutputTokensSum,
			"costMicros":      s.CostMicrosSum,
			"costUsd":         fmt.Sprintf("%.6f", float64(s.CostMicrosSum)/1e6),
			"errorCount":      s.ErrorCount,
		}
	}

	topModelsList := make([]gin.H, 0, len(topModels))
	for _, m := range topModels {
		topModelsList = append(topModelsList, gin.H{
			"model":        m.Model,
			"requestCount": m.RequestCount,
			"costMicros":   m.CostMicros,
			"costUsd":      fmt.Sprintf("%.6f", float64(m.CostMicros)/1e6),
		})
	}

	trendList := make([]gin.H, 0, len(dailyTrend))
	for _, d := range dailyTrend {
		trendList = append(trendList, gin.H{
			"date":       d.Date,
			"costMicros": d.CostMicros,
			"costUsd":    fmt.Sprintf("%.6f", float64(d.CostMicros)/1e6),
			"requests":   d.Requests,
		})
	}

	throughputList := make([]gin.H, 0, len(throughputTrend))
	for _, point := range throughputTrend {
		throughputList = append(throughputList, gin.H{
			"minute": point.Minute,
			"qps1m":  point.QPS1m,
			"rpm5m":  point.RPM5m,
			"tpm5m":  point.TPM5m,
		})
	}

	formatTimingTrend := func(points []repository.DashboardTimingPoint) []gin.H {
		items := make([]gin.H, 0, len(points))
		for _, point := range points {
			items = append(items, gin.H{
				"bucket":    point.Bucket,
				"avgMs":     point.AvgMs,
				"p50Ms":     point.P50Ms,
				"p90Ms":     point.P90Ms,
				"p99Ms":     point.P99Ms,
				"sampleCnt": point.SampleCnt,
			})
		}
		return items
	}

	cacheHitRates, err := h.logService.GetAdminCacheHitRateByProvider()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取缓存命中率失败"})
		return
	}

	cacheHitRateList := make([]gin.H, 0, len(cacheHitRates))
	for _, r := range cacheHitRates {
		cacheHitRateList = append(cacheHitRateList, gin.H{
			"provider":            r.Provider,
			"totalInputTokens":    r.TotalInputTokens,
			"cacheReadTokens":     r.CacheReadTokens,
			"cacheCreationTokens": r.CacheCreationTokens,
			"requestCount":        r.RequestCount,
			"hitRate":             fmt.Sprintf("%.1f", r.HitRate),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"balance": gin.H{
			"totalBalanceMicros": totalBalance,
			"totalBalanceUsd":    fmt.Sprintf("%.6f", float64(totalBalance)/1e6),
			"userCount":          userCount,
		},
			"today":           formatPeriod(today),
			"week":            formatPeriod(week),
			"month":           formatPeriod(month),
			"topModels":       topModelsList,
			"dailyTrend":      trendList,
			"throughputTrend": throughputList,
			"ttfbTrend":       formatTimingTrend(ttfbTrend),
			"durationTrend":   formatTimingTrend(durationTrend),
			"cacheHitRates":   cacheHitRateList,
		})
}

// AdminRequestLogsWS WebSocket 实时日志推送
func (h *RequestLogHandler) AdminRequestLogsWS(c *gin.Context) {
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusInternalError, "")

	hub := realtime.GetHub()
	if hub == nil {
		conn.Close(websocket.StatusInternalError, "hub not initialized")
		return
	}

	client := realtime.NewClient(conn)
	hub.Register(client)
	defer hub.Unregister(client)

	ctx := c.Request.Context()
	go client.WriteLoop(ctx)
	client.ReadLoop(ctx)
}
