package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ampmanager/internal/billing"
	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

type RequestLogRepository struct{}

func NewRequestLogRepository() *RequestLogRepository {
	return &RequestLogRepository{}
}

func displayInputTokensInt64(inputTokens, cacheReadTokens int64) int64 {
	uncachedInputTokens := inputTokens - cacheReadTokens
	if uncachedInputTokens < 0 {
		return 0
	}
	return uncachedInputTokens
}

func displayInputTokens(inputTokens, cacheReadTokens *int) *int {
	if inputTokens == nil {
		return nil
	}

	cacheRead := 0
	if cacheReadTokens != nil {
		cacheRead = *cacheReadTokens
	}

	uncachedInputTokens := *inputTokens - cacheRead
	if uncachedInputTokens < 0 {
		uncachedInputTokens = 0
	}

	return &uncachedInputTokens
}

func normalizeRequestLogDisplay(log *model.RequestLog) {
	if log == nil {
		return
	}
	log.BillingStatus = normalizeBillingStatus(log.BillingStatus)
	log.InputTokens = displayInputTokens(log.InputTokens, log.CacheReadInputTokens)
}

func normalizeBillingStatus(status string) string {
	switch status {
	case "":
		return "none"
	case "under_reserved":
		return "overuse"
	default:
		return status
	}
}

func enrichRequestLogMetrics(log *model.RequestLog) {
	if log == nil {
		return
	}

	normalizeRequestLogDisplay(log)
	if log.CostMicros != nil {
		gap := *log.CostMicros - log.ChargedSubscriptionMicros - log.ChargedBalanceMicros
		if gap < 0 {
			gap = 0
		}
		log.BillingGapMicros = &gap
	}

	if !log.IsStreaming {
		log.TTFBMs = nil
		log.TPS = nil
		return
	}

	if log.TTFBMs == nil || log.OutputTokens == nil {
		return
	}

	streamDurationMs := log.LatencyMs - *log.TTFBMs
	if streamDurationMs <= 0 {
		return
	}

	tps := float64(*log.OutputTokens) / (float64(streamDurationMs) / 1000)
	log.TPS = &tps
}

func enrichRequestLogPricing(log *model.RequestLog) {
	if log == nil {
		return
	}

	store := billing.GetPriceStore()
	if store == nil {
		return
	}

	pricingModel := ""
	if log.PricingModel != nil && strings.TrimSpace(*log.PricingModel) != "" {
		pricingModel = strings.TrimSpace(*log.PricingModel)
	} else if log.MappedModel != nil && strings.TrimSpace(*log.MappedModel) != "" {
		pricingModel = strings.TrimSpace(*log.MappedModel)
	} else if log.OriginalModel != nil && strings.TrimSpace(*log.OriginalModel) != "" {
		pricingModel = strings.TrimSpace(*log.OriginalModel)
	}

	if pricingModel == "" {
		return
	}

	priceData, found := store.GetPrice(pricingModel)
	if !found {
		return
	}
	if log.PricingRuleName != nil && strings.TrimSpace(*log.PricingRuleName) != "" {
		if rule, ok := store.FindContextRuleByName(pricingModel, strings.TrimSpace(*log.PricingRuleName)); ok {
			priceData = billing.PriceData{
				InputMicrosPerMillion:         rule.InputMicrosPerMillion,
				OutputMicrosPerMillion:        rule.OutputMicrosPerMillion,
				CacheReadMicrosPerMillion:     rule.CacheReadMicrosPerMillion,
				CacheCreationMicrosPerMillion: rule.CacheCreationMicrosPerMillion,
				InputCostPerToken:             rule.InputCostPerToken,
				OutputCostPerToken:            rule.OutputCostPerToken,
				CacheReadInputPerToken:        rule.CacheReadInputPerToken,
				CacheCreationPerToken:         rule.CacheCreationPerToken,
			}
		}
	}

	log.InputMicrosPerMillion = &priceData.InputMicrosPerMillion
	log.OutputMicrosPerMillion = &priceData.OutputMicrosPerMillion
	log.CacheReadMicrosPerMillion = &priceData.CacheReadMicrosPerMillion
	log.CacheCreationMicrosPerMillion = &priceData.CacheCreationMicrosPerMillion
	log.InputCostPerToken = &priceData.InputCostPerToken
	log.OutputCostPerToken = &priceData.OutputCostPerToken
	log.CacheReadInputPerToken = &priceData.CacheReadInputPerToken
	log.CacheCreationInputPerToken = &priceData.CacheCreationPerToken
}

func parseRequestLogTranslator(raw string) *model.ChannelTranslator {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var translatorCfg model.ChannelTranslator
	if err := json.Unmarshal([]byte(raw), &translatorCfg); err != nil {
		return nil
	}
	return &translatorCfg
}

// ListParams 查询参数
type ListParams struct {
	UserID      string
	APIKeyID    string
	Model       string
	StatusCodes []int
	IsStreaming *bool
	From        *time.Time
	To          *time.Time
	Page        int
	PageSize    int
}

// List 查询请求日志列表
func (r *RequestLogRepository) List(params ListParams) ([]model.RequestLog, int64, error) {
	db := database.GetDB()

	// 构建 WHERE 条件（使用 r. 前缀避免 JOIN 时的歧义）
	conditions := []string{"1=1"}
	args := []interface{}{}

	if params.UserID != "" {
		conditions = append(conditions, "r.user_id = ?")
		args = append(args, params.UserID)
	}
	if params.APIKeyID != "" {
		conditions = append(conditions, "r.api_key_id = ?")
		args = append(args, params.APIKeyID)
	}
	if params.Model != "" {
		conditions = append(conditions, "(r.original_model = ? OR r.mapped_model = ?)")
		args = append(args, params.Model, params.Model)
	}
	if len(params.StatusCodes) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(params.StatusCodes)), ",")
		conditions = append(conditions, "r.status_code IN ("+placeholders+")")
		for _, statusCode := range params.StatusCodes {
			args = append(args, statusCode)
		}
	}
	if params.IsStreaming != nil {
		val := 0
		if *params.IsStreaming {
			val = 1
		}
		conditions = append(conditions, "r.is_streaming = ?")
		args = append(args, val)
	}
	if params.From != nil {
		conditions = append(conditions, "r.created_at >= ?")
		args = append(args, params.From.UTC())
	}
	if params.To != nil {
		conditions = append(conditions, "r.created_at <= ?")
		args = append(args, params.To.UTC())
	}

	whereClause := strings.Join(conditions, " AND ")

	// 查询总数
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM request_logs r WHERE %s", whereClause)
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 分页
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	offset := (params.Page - 1) * params.PageSize
	detailJoin := "LEFT JOIN request_log_details d ON r.id = d.request_id"
	outputPreviewExpr := "COALESCE(SUBSTR(r.response_text, 1, 200), SUBSTR(d.response_body, 1, 200))"
	if database.IsPostgres() {
		detailJoin += "\n                LEFT JOIN request_log_details_archive da ON r.id = da.request_id"
		outputPreviewExpr = "COALESCE(SUBSTR(r.response_text, 1, 200), SUBSTR(d.response_body, 1, 200), SUBSTR(da.response_body, 1, 200))"
	}

	// 查询数据
	query := fmt.Sprintf(`
		SELECT r.id, r.created_at, r.updated_at, r.status, r.user_id, u.username, r.api_key_id, k.name as api_key_name, k.prefix as api_key_prefix, r.original_model, r.mapped_model,
		       r.provider, r.channel_id, c.name as channel_name, r.endpoint, r.request_format, r.upstream_format, r.method, r.path, r.status_code, r.latency_ms, r.ttfb_ms,
		       r.is_streaming, r.input_tokens, r.output_tokens, r.cache_read_input_tokens,
		       r.cache_creation_input_tokens, r.error_type, r.request_id, r.cost_micros, r.cost_usd, r.pricing_model, r.pricing_rule_name, r.thinking_level,
		       r.rate_multiplier, r.channel_rate_multiplier, r.group_rate_multiplier, r.special_rate_multiplier, r.special_rate_reason, c.translator_json,
		       r.downstream_transport, r.upstream_transport, r.transport_fallback_reason,
		       r.charged_subscription_micros, r.charged_balance_micros, r.billing_status,
		       %s as output_preview
		FROM request_logs r
                LEFT JOIN users u ON r.user_id = u.id
		LEFT JOIN user_api_keys k ON r.api_key_id = k.id
		%s
		LEFT JOIN channels c ON r.channel_id = c.id
		WHERE %s
		ORDER BY r.created_at DESC
		LIMIT ? OFFSET ?
	`, outputPreviewExpr, detailJoin, whereClause)

	args = append(args, params.PageSize, offset)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []model.RequestLog
	for rows.Next() {
		var log model.RequestLog
		var createdAt time.Time
		var updatedAt sql.NullTime
		var status sql.NullString
		var isStreaming int
		var username, apiKeyName, apiKeyPrefix sql.NullString
		var originalModel, mappedModel, provider, channelID, channelName, endpoint, requestFormat, upstreamFormat, errorType, requestID, costUsd, pricingModel, pricingRuleName, thinkingLevel, downstreamTransport, upstreamTransport, transportFallbackReason, outputPreview, billingStatus, specialRateReason, translatorJSON sql.NullString
		var rateMultiplier, channelRateMultiplier, groupRateMultiplier, specialRateMultiplier sql.NullFloat64
		var inputTokens, outputTokens, cacheRead, cacheCreation, costMicros, ttfbMs, chargedSubscriptionMicros, chargedBalanceMicros sql.NullInt64

		err := rows.Scan(
			&log.ID, &createdAt, &updatedAt, &status, &log.UserID, &username, &log.APIKeyID, &apiKeyName, &apiKeyPrefix,
			&originalModel, &mappedModel, &provider, &channelID, &channelName, &endpoint, &requestFormat, &upstreamFormat,
			&log.Method, &log.Path, &log.StatusCode, &log.LatencyMs, &ttfbMs,
			&isStreaming, &inputTokens, &outputTokens, &cacheRead, &cacheCreation,
			&errorType, &requestID, &costMicros, &costUsd, &pricingModel, &pricingRuleName, &thinkingLevel,
			&rateMultiplier, &channelRateMultiplier, &groupRateMultiplier, &specialRateMultiplier, &specialRateReason, &translatorJSON,
			&downstreamTransport, &upstreamTransport, &transportFallbackReason,
			&chargedSubscriptionMicros, &chargedBalanceMicros, &billingStatus,
			&outputPreview,
		)
		if err != nil {
			return nil, 0, err
		}

		log.CreatedAt = createdAt.Format(time.RFC3339)
		log.IsStreaming = isStreaming == 1

		if username.Valid {
			log.Username = &username.String
		}
		if apiKeyName.Valid {
			log.APIKeyName = &apiKeyName.String
		}
		if apiKeyPrefix.Valid {
			log.APIKeyPrefix = &apiKeyPrefix.String
		}
		if updatedAt.Valid {
			formatted := updatedAt.Time.Format(time.RFC3339)
			log.UpdatedAt = &formatted
		}
		if ttfbMs.Valid {
			log.TTFBMs = &ttfbMs.Int64
		}
		if status.Valid {
			log.Status = model.RequestLogStatus(status.String)
		} else {
			log.Status = model.RequestLogStatusSuccess // 默认为 success（兼容旧数据）
		}

		if originalModel.Valid {
			log.OriginalModel = &originalModel.String
		}
		if mappedModel.Valid {
			log.MappedModel = &mappedModel.String
		}
		if provider.Valid {
			log.Provider = &provider.String
		}
		if channelID.Valid {
			log.ChannelID = &channelID.String
		}
		if channelName.Valid {
			log.ChannelName = &channelName.String
		}
		if endpoint.Valid {
			log.Endpoint = &endpoint.String
		}
		if requestFormat.Valid {
			log.RequestFormat = &requestFormat.String
		}
		if upstreamFormat.Valid {
			log.UpstreamFormat = &upstreamFormat.String
		}
		if errorType.Valid {
			log.ErrorType = &errorType.String
		}
		if requestID.Valid {
			log.RequestID = &requestID.String
		}
		if inputTokens.Valid {
			v := int(inputTokens.Int64)
			log.InputTokens = &v
		}
		if outputTokens.Valid {
			v := int(outputTokens.Int64)
			log.OutputTokens = &v
		}
		if cacheRead.Valid {
			v := int(cacheRead.Int64)
			log.CacheReadInputTokens = &v
		}
		if cacheCreation.Valid {
			v := int(cacheCreation.Int64)
			log.CacheCreationInputTokens = &v
		}
		if costMicros.Valid {
			log.CostMicros = &costMicros.Int64
		}
		if chargedSubscriptionMicros.Valid {
			log.ChargedSubscriptionMicros = chargedSubscriptionMicros.Int64
		}
		if chargedBalanceMicros.Valid {
			log.ChargedBalanceMicros = chargedBalanceMicros.Int64
		}
		if costUsd.Valid {
			log.CostUsd = &costUsd.String
		}
		if pricingModel.Valid {
			log.PricingModel = &pricingModel.String
		}
		if pricingRuleName.Valid {
			log.PricingRuleName = &pricingRuleName.String
		}
		if rateMultiplier.Valid {
			log.RateMultiplier = &rateMultiplier.Float64
		}
		if channelRateMultiplier.Valid {
			log.ChannelRateMultiplier = &channelRateMultiplier.Float64
		}
		if groupRateMultiplier.Valid {
			log.GroupRateMultiplier = &groupRateMultiplier.Float64
		}
		if specialRateMultiplier.Valid {
			log.SpecialRateMultiplier = &specialRateMultiplier.Float64
		}
		if specialRateReason.Valid {
			log.SpecialRateReason = &specialRateReason.String
		}
		if translatorJSON.Valid {
			log.ChannelTranslator = parseRequestLogTranslator(translatorJSON.String)
		}
		if billingStatus.Valid {
			log.BillingStatus = billingStatus.String
		}
		if thinkingLevel.Valid {
			log.ThinkingLevel = &thinkingLevel.String
		}
		if downstreamTransport.Valid {
			log.DownstreamTransport = &downstreamTransport.String
		}
		if upstreamTransport.Valid {
			log.UpstreamTransport = &upstreamTransport.String
		}
		if transportFallbackReason.Valid {
			log.TransportFallbackReason = &transportFallbackReason.String
		}
		if outputPreview.Valid {
			log.OutputPreview = &outputPreview.String
		}
		enrichRequestLogMetrics(&log)
		enrichRequestLogPricing(&log)

		logs = append(logs, log)
	}

	return logs, total, rows.Err()
}

// GetUsageSummary 获取用量统计
// userID 为 nil 或空字符串时查询所有用户
func (r *RequestLogRepository) GetUsageSummary(userID *string, from, to *time.Time, groupBy string, modelFilter string) ([]model.UsageSummary, error) {
	db := database.GetDB()

	var groupColumn string
	switch groupBy {
	case "day":
		groupColumn = database.DayBucketExpr("created_at")
	case "model":
		groupColumn = "COALESCE(mapped_model, original_model, 'unknown')"
	case "apiKey":
		groupColumn = "api_key_id"
	case "user":
		groupColumn = "user_id"
	default:
		groupColumn = database.DayBucketExpr("created_at")
	}

	conditions := []string{"1=1"}
	args := []interface{}{}

	if userID != nil && *userID != "" {
		conditions = append(conditions, "user_id = ?")
		args = append(args, *userID)
	}

	if modelFilter != "" {
		conditions = append(conditions, "(mapped_model = ? OR original_model = ?)")
		args = append(args, modelFilter, modelFilter)
	}

	if from != nil {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, from.UTC())
	}
	if to != nil {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, to.UTC())
	}

	whereClause := strings.Join(conditions, " AND ")

	query := fmt.Sprintf(`
			SELECT 
				%s as group_key,
				COALESCE(SUM(CASE
					WHEN COALESCE(input_tokens, 0) > COALESCE(cache_read_input_tokens, 0)
						THEN COALESCE(input_tokens, 0) - COALESCE(cache_read_input_tokens, 0)
					ELSE 0
				END), 0) as input_tokens_sum,
				COALESCE(SUM(output_tokens), 0) as output_tokens_sum,
				COALESCE(SUM(cache_read_input_tokens), 0) as cache_read_sum,
			COALESCE(SUM(cache_creation_input_tokens), 0) as cache_creation_sum,
			COUNT(*) as request_count,
			SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as error_count,
			COALESCE(SUM(cost_micros), 0) as cost_micros_sum
		FROM request_logs
		WHERE %s
		GROUP BY %s
		ORDER BY %s DESC
		LIMIT 100
	`, groupColumn, whereClause, groupColumn, groupColumn)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []model.UsageSummary
	for rows.Next() {
		var s model.UsageSummary
		err := rows.Scan(
			&s.GroupKey,
			&s.InputTokensSum,
			&s.OutputTokensSum,
			&s.CacheReadInputTokensSum,
			&s.CacheCreationInputTokensSum,
			&s.RequestCount,
			&s.ErrorCount,
			&s.CostMicrosSum,
		)
		if err != nil {
			return nil, err
		}
		// 转换为 USD 字符串
		s.CostUsdSum = fmt.Sprintf("%.6f", float64(s.CostMicrosSum)/1_000_000)
		summaries = append(summaries, s)
	}

	return summaries, rows.Err()
}

// GetDistinctModels 获取使用过的模型列表
func (r *RequestLogRepository) GetDistinctModels() ([]string, error) {
	db := database.GetDB()

	query := `
		SELECT DISTINCT COALESCE(mapped_model, original_model) as model
		FROM request_logs
		WHERE mapped_model IS NOT NULL OR original_model IS NOT NULL
		ORDER BY model
		LIMIT 100
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		models = append(models, m)
	}

	return models, rows.Err()
}

// GetDistinctModelsByUser 获取指定用户使用过的模型列表
func (r *RequestLogRepository) GetDistinctModelsByUser(userID string) ([]string, error) {
	db := database.GetDB()

	query := `
		SELECT DISTINCT COALESCE(mapped_model, original_model) as model
		FROM request_logs
		WHERE user_id = ? AND (mapped_model IS NOT NULL OR original_model IS NOT NULL)
		ORDER BY model
		LIMIT 100
	`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		models = append(models, m)
	}

	return models, rows.Err()
}

// DistinctAPIKey 去重 API Key 信息
type DistinctAPIKey struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

// GetDistinctAPIKeys 获取在日志中使用过的 API Key 列表
func (r *RequestLogRepository) GetDistinctAPIKeys(userID string) ([]DistinctAPIKey, error) {
	db := database.GetDB()

	var rows *sql.Rows
	var err error

	if userID != "" {
		rows, err = db.Query(`
			SELECT DISTINCT k.id, k.name, k.prefix
			FROM request_logs r
			INNER JOIN user_api_keys k ON r.api_key_id = k.id
			WHERE r.user_id = ?
			ORDER BY k.name
			LIMIT 200
		`, userID)
	} else {
		rows, err = db.Query(`
			SELECT DISTINCT k.id, k.name, k.prefix
			FROM request_logs r
			INNER JOIN user_api_keys k ON r.api_key_id = k.id
			ORDER BY k.name
			LIMIT 200
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []DistinctAPIKey
	for rows.Next() {
		var k DistinctAPIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// DashboardPeriodStats 仪表盘时段统计
type DashboardPeriodStats struct {
	RequestCount    int64
	InputTokensSum  int64
	OutputTokensSum int64
	CostMicrosSum   int64
	ErrorCount      int64
}

// DashboardTopModel 仪表盘热门模型
type DashboardTopModel struct {
	Model        string
	RequestCount int64
	CostMicros   int64
}

// DashboardDailyTrend 仪表盘每日趋势
type DashboardDailyTrend struct {
	Date       string
	CostMicros int64
	Requests   int64
}

func fillDashboardDailyTrend(start time.Time, days int, trends []DashboardDailyTrend) []DashboardDailyTrend {
	byDate := make(map[string]DashboardDailyTrend, len(trends))
	for _, trend := range trends {
		byDate[trend.Date] = trend
	}

	filled := make([]DashboardDailyTrend, 0, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		if trend, ok := byDate[key]; ok {
			filled = append(filled, trend)
			continue
		}
		filled = append(filled, DashboardDailyTrend{
			Date:       key,
			CostMicros: 0,
			Requests:   0,
		})
	}

	return filled
}

// DashboardCacheHitRate 按提供商分类的缓存命中率
type DashboardCacheHitRate struct {
	Provider            string
	TotalInputTokens    int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	RequestCount        int64
	HitRate             float64
}

// GetDashboardStats 获取仪表盘统计数据
func (r *RequestLogRepository) GetDashboardStats(userID string) (today, week, month DashboardPeriodStats, topModels []DashboardTopModel, dailyTrend []DashboardDailyTrend, err error) {
	db := database.GetDB()
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	weekStart := todayStart.AddDate(0, 0, -7)
	monthStart := todayStart.AddDate(0, 0, -30)
	trendStart := todayStart.AddDate(0, 0, -13)

	queryPeriod := func(from time.Time) (DashboardPeriodStats, error) {
		var s DashboardPeriodStats
		err := db.QueryRow(`
				SELECT COUNT(*),
				       COALESCE(SUM(CASE
				           WHEN COALESCE(input_tokens, 0) > COALESCE(cache_read_input_tokens, 0)
				               THEN COALESCE(input_tokens, 0) - COALESCE(cache_read_input_tokens, 0)
				           ELSE 0
				       END), 0),
				       COALESCE(SUM(output_tokens), 0),
				       COALESCE(SUM(cost_micros), 0),
			       COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END), 0)
			FROM request_logs WHERE user_id = ? AND created_at >= ?
		`, userID, from.UTC()).Scan(&s.RequestCount, &s.InputTokensSum, &s.OutputTokensSum, &s.CostMicrosSum, &s.ErrorCount)
		return s, err
	}

	today, err = queryPeriod(todayStart)
	if err != nil {
		return
	}
	week, err = queryPeriod(weekStart)
	if err != nil {
		return
	}
	month, err = queryPeriod(monthStart)
	if err != nil {
		return
	}

	rows, err := db.Query(`
		SELECT COALESCE(mapped_model, original_model, 'unknown') as model,
		       COUNT(*) as cnt,
		       COALESCE(SUM(cost_micros), 0) as cost
		FROM request_logs
		WHERE user_id = ? AND created_at >= ?
		GROUP BY model
		ORDER BY cnt DESC
		LIMIT 5
	`, userID, monthStart.UTC())
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var m DashboardTopModel
		if err = rows.Scan(&m.Model, &m.RequestCount, &m.CostMicros); err != nil {
			return
		}
		topModels = append(topModels, m)
	}
	if err = rows.Err(); err != nil {
		return
	}

	rows2, err := db.Query(fmt.Sprintf(`
		SELECT %s as day,
		       COALESCE(SUM(cost_micros), 0) as cost,
		       COUNT(*) as cnt
		FROM request_logs
		WHERE user_id = ? AND created_at >= ?
		GROUP BY day
		ORDER BY day ASC
	`, database.DayBucketExpr("created_at")), userID, trendStart.UTC())
	if err != nil {
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var d DashboardDailyTrend
		if err = rows2.Scan(&d.Date, &d.CostMicros, &d.Requests); err != nil {
			return
		}
		dailyTrend = append(dailyTrend, d)
	}
	err = rows2.Err()
	if err == nil {
		dailyTrend = fillDashboardDailyTrend(trendStart, 14, dailyTrend)
	}
	return
}

// GetCacheHitRateByProvider 按提供商分类获取缓存命中率（30天）
func (r *RequestLogRepository) GetCacheHitRateByProvider(userID string) ([]DashboardCacheHitRate, error) {
	db := database.GetDB()
	monthStart := time.Now().UTC().AddDate(0, 0, -30)

	providerExpr := `CASE
						WHEN LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'claude%%' THEN 'Claude'
						WHEN LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'gpt%%'
						  OR LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'o1%%'
						  OR LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'o3%%'
						  OR LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'o4%%'
						  OR LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'chatgpt%%' THEN 'OpenAI'
						WHEN LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'gemini%%' THEN 'Gemini'
						ELSE 'Other'
				END`

	query := `
		SELECT provider, total_input, cache_read, cache_creation, req_count FROM (
			SELECT
				` + providerExpr + ` as provider,
				COALESCE(SUM(input_tokens), 0) as total_input,
				COALESCE(SUM(cache_read_input_tokens), 0) as cache_read,
				COALESCE(SUM(cache_creation_input_tokens), 0) as cache_creation,
				COUNT(*) as req_count
			FROM request_logs
			WHERE user_id = ? AND created_at >= ?
			GROUP BY ` + providerExpr + `
		) WHERE provider IN ('Claude', 'OpenAI', 'Gemini')
		ORDER BY req_count DESC
	`

	rows, err := db.Query(query, userID, monthStart.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DashboardCacheHitRate
	for rows.Next() {
		var r DashboardCacheHitRate
		if err := rows.Scan(&r.Provider, &r.TotalInputTokens, &r.CacheReadTokens, &r.CacheCreationTokens, &r.RequestCount); err != nil {
			return nil, err
		}
		if r.TotalInputTokens > 0 {
			r.HitRate = float64(r.CacheReadTokens) / float64(r.TotalInputTokens) * 100
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetAdminDashboardStats 获取管理员仪表盘统计数据（全局，不按用户过滤）
func (r *RequestLogRepository) GetAdminDashboardStats(windowKey string) (today, week, month DashboardPeriodStats, topModels []DashboardTopModel, dailyTrend []DashboardDailyTrend, throughputTrend []DashboardThroughputPoint, ttfbTrend []DashboardTimingPoint, durationTrend []DashboardTimingPoint, err error) {
	db := database.GetDB()
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	weekStart := todayStart.AddDate(0, 0, -7)
	monthStart := todayStart.AddDate(0, 0, -30)
	trendStart := todayStart.AddDate(0, 0, -13)

	queryPeriod := func(from time.Time) (DashboardPeriodStats, error) {
		var s DashboardPeriodStats
		err := db.QueryRow(`
				SELECT COUNT(*),
				       COALESCE(SUM(CASE
				           WHEN COALESCE(input_tokens, 0) > COALESCE(cache_read_input_tokens, 0)
				               THEN COALESCE(input_tokens, 0) - COALESCE(cache_read_input_tokens, 0)
				           ELSE 0
				       END), 0),
				       COALESCE(SUM(output_tokens), 0),
				       COALESCE(SUM(cost_micros), 0),
			       COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END), 0)
			FROM request_logs WHERE created_at >= ?
		`, from.UTC()).Scan(&s.RequestCount, &s.InputTokensSum, &s.OutputTokensSum, &s.CostMicrosSum, &s.ErrorCount)
		return s, err
	}

	today, err = queryPeriod(todayStart)
	if err != nil {
		return
	}
	week, err = queryPeriod(weekStart)
	if err != nil {
		return
	}
	month, err = queryPeriod(monthStart)
	if err != nil {
		return
	}

	rows, err := db.Query(`
		SELECT COALESCE(mapped_model, original_model, 'unknown') as model,
		       COUNT(*) as cnt,
		       COALESCE(SUM(cost_micros), 0) as cost
		FROM request_logs
		WHERE created_at >= ?
		GROUP BY model
		ORDER BY cnt DESC
		LIMIT 10
	`, monthStart.UTC())
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var m DashboardTopModel
		if err = rows.Scan(&m.Model, &m.RequestCount, &m.CostMicros); err != nil {
			return
		}
		topModels = append(topModels, m)
	}
	if err = rows.Err(); err != nil {
		return
	}

	rows2, err := db.Query(fmt.Sprintf(`
		SELECT %s as day,
		       COALESCE(SUM(cost_micros), 0) as cost,
		       COUNT(*) as cnt
		FROM request_logs
		WHERE created_at >= ?
		GROUP BY day
		ORDER BY day ASC
	`, database.DayBucketExpr("created_at")), trendStart.UTC())
	if err != nil {
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var d DashboardDailyTrend
		if err = rows2.Scan(&d.Date, &d.CostMicros, &d.Requests); err != nil {
			return
		}
		dailyTrend = append(dailyTrend, d)
	}
	err = rows2.Err()
	if err == nil {
		dailyTrend = fillDashboardDailyTrend(trendStart, 14, dailyTrend)
	}
	if err != nil {
		return
	}

	throughputTrend, err = r.GetAdminThroughputTrend(windowKey)
	if err != nil {
		return
	}
	ttfbTrend, durationTrend, err = r.GetAdminTimingTrend(windowKey)
	return
}

// GetAdminCacheHitRateByProvider 管理员全局缓存命中率（30天）
func (r *RequestLogRepository) GetAdminCacheHitRateByProvider() ([]DashboardCacheHitRate, error) {
	db := database.GetDB()
	monthStart := time.Now().UTC().AddDate(0, 0, -30)

	providerExpr := `CASE
						WHEN LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'claude%%' THEN 'Claude'
						WHEN LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'gpt%%'
						  OR LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'o1%%'
						  OR LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'o3%%'
						  OR LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'o4%%'
						  OR LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'chatgpt%%' THEN 'OpenAI'
						WHEN LOWER(COALESCE(mapped_model, original_model, '')) LIKE 'gemini%%' THEN 'Gemini'
						ELSE 'Other'
				END`

	query := `
		SELECT provider, total_input, cache_read, cache_creation, req_count FROM (
			SELECT
				` + providerExpr + ` as provider,
				COALESCE(SUM(input_tokens), 0) as total_input,
				COALESCE(SUM(cache_read_input_tokens), 0) as cache_read,
				COALESCE(SUM(cache_creation_input_tokens), 0) as cache_creation,
				COUNT(*) as req_count
			FROM request_logs
			WHERE created_at >= ?
			GROUP BY ` + providerExpr + `
		) WHERE provider IN ('Claude', 'OpenAI', 'Gemini')
		ORDER BY req_count DESC
	`

	rows, err := db.Query(query, monthStart.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DashboardCacheHitRate
	for rows.Next() {
		var r DashboardCacheHitRate
		if err := rows.Scan(&r.Provider, &r.TotalInputTokens, &r.CacheReadTokens, &r.CacheCreationTokens, &r.RequestCount); err != nil {
			return nil, err
		}
		if r.TotalInputTokens > 0 {
			r.HitRate = float64(r.CacheReadTokens) / float64(r.TotalInputTokens) * 100
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetByID 获取单条日志
func (r *RequestLogRepository) GetByID(id string) (*model.RequestLog, error) {
	db := database.GetDB()

	var log model.RequestLog
	var createdAt time.Time
	var updatedAt sql.NullTime
	var status sql.NullString
	var isStreaming int
	var originalModel, mappedModel, provider, channelID, channelName, endpoint, requestFormat, upstreamFormat, errorType, requestID, costUsd, pricingModel, pricingRuleName, thinkingLevel, downstreamTransport, upstreamTransport, transportFallbackReason, billingStatus, specialRateReason, translatorJSON sql.NullString
	var rateMultiplier, channelRateMultiplier, groupRateMultiplier, specialRateMultiplier sql.NullFloat64
	var inputTokens, outputTokens, cacheRead, cacheCreation, costMicros, ttfbMs, chargedSubscriptionMicros, chargedBalanceMicros sql.NullInt64

	err := db.QueryRow(`
		SELECT r.id, r.created_at, r.updated_at, r.status, r.user_id, r.api_key_id, r.original_model, r.mapped_model,
		       r.provider, r.channel_id, c.name as channel_name, r.endpoint, r.request_format, r.upstream_format, r.method, r.path, r.status_code, r.latency_ms, r.ttfb_ms,
		       r.is_streaming, r.input_tokens, r.output_tokens, r.cache_read_input_tokens,
		       r.cache_creation_input_tokens, r.error_type, r.request_id, r.cost_micros, r.cost_usd, r.pricing_model, r.pricing_rule_name, r.thinking_level,
		       r.rate_multiplier, r.channel_rate_multiplier, r.group_rate_multiplier, r.special_rate_multiplier, r.special_rate_reason, c.translator_json,
		       r.downstream_transport, r.upstream_transport, r.transport_fallback_reason,
		       r.charged_subscription_micros, r.charged_balance_micros, r.billing_status
		FROM request_logs r
		LEFT JOIN channels c ON r.channel_id = c.id
		WHERE r.id = ?
	`, id).Scan(
		&log.ID, &createdAt, &updatedAt, &status, &log.UserID, &log.APIKeyID,
		&originalModel, &mappedModel, &provider, &channelID, &channelName, &endpoint, &requestFormat, &upstreamFormat,
		&log.Method, &log.Path, &log.StatusCode, &log.LatencyMs, &ttfbMs,
		&isStreaming, &inputTokens, &outputTokens, &cacheRead, &cacheCreation,
		&errorType, &requestID, &costMicros, &costUsd, &pricingModel, &pricingRuleName, &thinkingLevel,
		&rateMultiplier, &channelRateMultiplier, &groupRateMultiplier, &specialRateMultiplier, &specialRateReason, &translatorJSON,
		&downstreamTransport, &upstreamTransport, &transportFallbackReason,
		&chargedSubscriptionMicros, &chargedBalanceMicros, &billingStatus,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	log.CreatedAt = createdAt.Format(time.RFC3339)
	log.IsStreaming = isStreaming == 1

	if updatedAt.Valid {
		formatted := updatedAt.Time.Format(time.RFC3339)
		log.UpdatedAt = &formatted
	}
	if ttfbMs.Valid {
		log.TTFBMs = &ttfbMs.Int64
	}
	if status.Valid {
		log.Status = model.RequestLogStatus(status.String)
	} else {
		log.Status = model.RequestLogStatusSuccess
	}

	if originalModel.Valid {
		log.OriginalModel = &originalModel.String
	}
	if mappedModel.Valid {
		log.MappedModel = &mappedModel.String
	}
	if provider.Valid {
		log.Provider = &provider.String
	}
	if channelID.Valid {
		log.ChannelID = &channelID.String
	}
	if channelName.Valid {
		log.ChannelName = &channelName.String
	}
	if endpoint.Valid {
		log.Endpoint = &endpoint.String
	}
	if requestFormat.Valid {
		log.RequestFormat = &requestFormat.String
	}
	if upstreamFormat.Valid {
		log.UpstreamFormat = &upstreamFormat.String
	}
	if errorType.Valid {
		log.ErrorType = &errorType.String
	}
	if requestID.Valid {
		log.RequestID = &requestID.String
	}
	if inputTokens.Valid {
		v := int(inputTokens.Int64)
		log.InputTokens = &v
	}
	if outputTokens.Valid {
		v := int(outputTokens.Int64)
		log.OutputTokens = &v
	}
	if cacheRead.Valid {
		v := int(cacheRead.Int64)
		log.CacheReadInputTokens = &v
	}
	if cacheCreation.Valid {
		v := int(cacheCreation.Int64)
		log.CacheCreationInputTokens = &v
	}
	if costMicros.Valid {
		log.CostMicros = &costMicros.Int64
	}
	if chargedSubscriptionMicros.Valid {
		log.ChargedSubscriptionMicros = chargedSubscriptionMicros.Int64
	}
	if chargedBalanceMicros.Valid {
		log.ChargedBalanceMicros = chargedBalanceMicros.Int64
	}
	if costUsd.Valid {
		log.CostUsd = &costUsd.String
	}
	if pricingModel.Valid {
		log.PricingModel = &pricingModel.String
	}
	if pricingRuleName.Valid {
		log.PricingRuleName = &pricingRuleName.String
	}
	if rateMultiplier.Valid {
		log.RateMultiplier = &rateMultiplier.Float64
	}
	if channelRateMultiplier.Valid {
		log.ChannelRateMultiplier = &channelRateMultiplier.Float64
	}
	if groupRateMultiplier.Valid {
		log.GroupRateMultiplier = &groupRateMultiplier.Float64
	}
	if specialRateMultiplier.Valid {
		log.SpecialRateMultiplier = &specialRateMultiplier.Float64
	}
	if specialRateReason.Valid {
		log.SpecialRateReason = &specialRateReason.String
	}
	if translatorJSON.Valid {
		log.ChannelTranslator = parseRequestLogTranslator(translatorJSON.String)
	}
	if billingStatus.Valid {
		log.BillingStatus = billingStatus.String
	}
	if thinkingLevel.Valid {
		log.ThinkingLevel = &thinkingLevel.String
	}
	if downstreamTransport.Valid {
		log.DownstreamTransport = &downstreamTransport.String
	}
	if upstreamTransport.Valid {
		log.UpstreamTransport = &upstreamTransport.String
	}
	if transportFallbackReason.Valid {
		log.TransportFallbackReason = &transportFallbackReason.String
	}
	enrichRequestLogMetrics(&log)
	enrichRequestLogPricing(&log)

	return &log, nil
}

// GetByIDWithJoins 获取单条日志（含用户名、Key信息、渠道名，用于广播）
func (r *RequestLogRepository) GetByIDWithJoins(id string) (*model.RequestLog, error) {
	db := database.GetDB()

	var l model.RequestLog
	var createdAt time.Time
	var updatedAt sql.NullTime
	var status sql.NullString
	var isStreaming int
	var username, apiKeyName, apiKeyPrefix sql.NullString
	var originalModel, mappedModel, provider, channelID, channelName, endpoint, requestFormat, upstreamFormat, errorType, requestID, costUsd, pricingModel, pricingRuleName, thinkingLevel, downstreamTransport, upstreamTransport, transportFallbackReason, billingStatus, specialRateReason, translatorJSON sql.NullString
	var rateMultiplier, channelRateMultiplier, groupRateMultiplier, specialRateMultiplier sql.NullFloat64
	var inputTokens, outputTokens, cacheRead, cacheCreation, costMicros, ttfbMs, chargedSubscriptionMicros, chargedBalanceMicros sql.NullInt64

	err := db.QueryRow(`
		SELECT r.id, r.created_at, r.updated_at, r.status, r.user_id, u.username, r.api_key_id, k.name, k.prefix,
		       r.original_model, r.mapped_model, r.provider, r.channel_id, c.name, r.endpoint, r.request_format, r.upstream_format,
		       r.method, r.path, r.status_code, r.latency_ms, r.ttfb_ms,
		       r.is_streaming, r.input_tokens, r.output_tokens, r.cache_read_input_tokens,
		       r.cache_creation_input_tokens, r.error_type, r.request_id, r.cost_micros, r.cost_usd, r.pricing_model, r.pricing_rule_name, r.thinking_level,
		       r.rate_multiplier, r.channel_rate_multiplier, r.group_rate_multiplier, r.special_rate_multiplier, r.special_rate_reason, c.translator_json,
		       r.downstream_transport, r.upstream_transport, r.transport_fallback_reason,
		       r.charged_subscription_micros, r.charged_balance_micros, r.billing_status
		FROM request_logs r
		LEFT JOIN users u ON r.user_id = u.id
		LEFT JOIN user_api_keys k ON r.api_key_id = k.id
		LEFT JOIN channels c ON r.channel_id = c.id
		WHERE r.id = ?
	`, id).Scan(
		&l.ID, &createdAt, &updatedAt, &status, &l.UserID, &username, &l.APIKeyID, &apiKeyName, &apiKeyPrefix,
		&originalModel, &mappedModel, &provider, &channelID, &channelName, &endpoint, &requestFormat, &upstreamFormat,
		&l.Method, &l.Path, &l.StatusCode, &l.LatencyMs, &ttfbMs,
		&isStreaming, &inputTokens, &outputTokens, &cacheRead, &cacheCreation,
		&errorType, &requestID, &costMicros, &costUsd, &pricingModel, &pricingRuleName, &thinkingLevel,
		&rateMultiplier, &channelRateMultiplier, &groupRateMultiplier, &specialRateMultiplier, &specialRateReason, &translatorJSON,
		&downstreamTransport, &upstreamTransport, &transportFallbackReason,
		&chargedSubscriptionMicros, &chargedBalanceMicros, &billingStatus,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	l.CreatedAt = createdAt.Format(time.RFC3339)
	l.IsStreaming = isStreaming == 1

	if username.Valid {
		l.Username = &username.String
	}
	if apiKeyName.Valid {
		l.APIKeyName = &apiKeyName.String
	}
	if apiKeyPrefix.Valid {
		l.APIKeyPrefix = &apiKeyPrefix.String
	}
	if updatedAt.Valid {
		f := updatedAt.Time.Format(time.RFC3339)
		l.UpdatedAt = &f
	}
	if ttfbMs.Valid {
		l.TTFBMs = &ttfbMs.Int64
	}
	if status.Valid {
		l.Status = model.RequestLogStatus(status.String)
	} else {
		l.Status = model.RequestLogStatusSuccess
	}
	if originalModel.Valid {
		l.OriginalModel = &originalModel.String
	}
	if mappedModel.Valid {
		l.MappedModel = &mappedModel.String
	}
	if provider.Valid {
		l.Provider = &provider.String
	}
	if channelID.Valid {
		l.ChannelID = &channelID.String
	}
	if channelName.Valid {
		l.ChannelName = &channelName.String
	}
	if endpoint.Valid {
		l.Endpoint = &endpoint.String
	}
	if requestFormat.Valid {
		l.RequestFormat = &requestFormat.String
	}
	if upstreamFormat.Valid {
		l.UpstreamFormat = &upstreamFormat.String
	}
	if errorType.Valid {
		l.ErrorType = &errorType.String
	}
	if requestID.Valid {
		l.RequestID = &requestID.String
	}
	if inputTokens.Valid {
		v := int(inputTokens.Int64)
		l.InputTokens = &v
	}
	if outputTokens.Valid {
		v := int(outputTokens.Int64)
		l.OutputTokens = &v
	}
	if cacheRead.Valid {
		v := int(cacheRead.Int64)
		l.CacheReadInputTokens = &v
	}
	if cacheCreation.Valid {
		v := int(cacheCreation.Int64)
		l.CacheCreationInputTokens = &v
	}
	if costMicros.Valid {
		l.CostMicros = &costMicros.Int64
	}
	if chargedSubscriptionMicros.Valid {
		l.ChargedSubscriptionMicros = chargedSubscriptionMicros.Int64
	}
	if chargedBalanceMicros.Valid {
		l.ChargedBalanceMicros = chargedBalanceMicros.Int64
	}
	if costUsd.Valid {
		l.CostUsd = &costUsd.String
	}
	if pricingModel.Valid {
		l.PricingModel = &pricingModel.String
	}
	if pricingRuleName.Valid {
		l.PricingRuleName = &pricingRuleName.String
	}
	if rateMultiplier.Valid {
		l.RateMultiplier = &rateMultiplier.Float64
	}
	if channelRateMultiplier.Valid {
		l.ChannelRateMultiplier = &channelRateMultiplier.Float64
	}
	if groupRateMultiplier.Valid {
		l.GroupRateMultiplier = &groupRateMultiplier.Float64
	}
	if specialRateMultiplier.Valid {
		l.SpecialRateMultiplier = &specialRateMultiplier.Float64
	}
	if specialRateReason.Valid {
		l.SpecialRateReason = &specialRateReason.String
	}
	if translatorJSON.Valid {
		l.ChannelTranslator = parseRequestLogTranslator(translatorJSON.String)
	}
	if billingStatus.Valid {
		l.BillingStatus = billingStatus.String
	}
	if thinkingLevel.Valid {
		l.ThinkingLevel = &thinkingLevel.String
	}
	if downstreamTransport.Valid {
		l.DownstreamTransport = &downstreamTransport.String
	}
	if upstreamTransport.Valid {
		l.UpstreamTransport = &upstreamTransport.String
	}
	if transportFallbackReason.Valid {
		l.TransportFallbackReason = &transportFallbackReason.String
	}
	enrichRequestLogMetrics(&l)
	enrichRequestLogPricing(&l)

	return &l, nil
}
