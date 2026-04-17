package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
)

type SessionListParams struct {
	Page          int
	PageSize      int
	Query         string
	ActiveOnly    bool
	WindowMinutes int
}

func (r *RequestLogRepository) CountActiveSessions(window time.Duration) (int64, error) {
	db := database.GetDB()
	var total int64
	err := db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT session_id
			FROM request_logs
			WHERE session_id IS NOT NULL AND session_id <> '' AND created_at >= ?
			GROUP BY session_id
		) sessions
	`, time.Now().UTC().Add(-window)).Scan(&total)
	return total, err
}

func (r *RequestLogRepository) ListSessions(params SessionListParams) ([]model.AdminSessionListItem, int64, error) {
	db := database.GetDB()
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	if params.WindowMinutes <= 0 {
		params.WindowMinutes = 5
	}

	queryFilter := ""
	args := make([]interface{}, 0, 8)
	if trimmed := strings.TrimSpace(params.Query); trimmed != "" {
		pattern := "%" + strings.ToLower(trimmed) + "%"
		queryFilter = ` AND (
			LOWER(COALESCE(r.session_id, '')) LIKE ?
			OR LOWER(COALESCE(r.user_id, '')) LIKE ?
			OR EXISTS (
				SELECT 1 FROM users ux
				WHERE ux.id = r.user_id AND LOWER(COALESCE(ux.username, '')) LIKE ?
			)
		)`
		args = append(args, pattern, pattern, pattern)
	}

	activeCutoff := time.Now().UTC().Add(-time.Duration(params.WindowMinutes) * time.Minute)
	havingClause := ""
	if params.ActiveOnly {
		havingClause = " HAVING MAX(r.created_at) >= ?"
		args = append(args, activeCutoff)
	}

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM (
			SELECT r.session_id
			FROM request_logs r
			WHERE r.session_id IS NOT NULL AND r.session_id <> ''%s
			GROUP BY r.session_id%s
		) sessions
	`, queryFilter, havingClause)
	var total int64
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PageSize
	listQuery := fmt.Sprintf(`
		SELECT
			r.session_id,
			COALESCE((
				SELECT r2.user_id
				FROM request_logs r2
				WHERE r2.session_id = r.session_id
				ORDER BY r2.created_at DESC
				LIMIT 1
			), '') AS user_id,
			COALESCE((
				SELECT u2.username
				FROM request_logs r2
				LEFT JOIN users u2 ON u2.id = r2.user_id
				WHERE r2.session_id = r.session_id
				ORDER BY r2.created_at DESC
				LIMIT 1
			), '') AS username,
			COALESCE((
				SELECT COALESCE(NULLIF(r2.provider, ''), '')
				FROM request_logs r2
				WHERE r2.session_id = r.session_id
				ORDER BY r2.created_at DESC
				LIMIT 1
			), '') AS provider,
			COUNT(*) AS request_count,
			MIN(r.created_at) AS first_seen_at,
			MAX(r.created_at) AS last_seen_at,
			COALESCE((
				SELECT COALESCE(NULLIF(r2.mapped_model, ''), NULLIF(r2.original_model, ''), '')
				FROM request_logs r2
				WHERE r2.session_id = r.session_id
				ORDER BY r2.created_at DESC
				LIMIT 1
			), '') AS last_model,
			COALESCE((
				SELECT r2.status_code
				FROM request_logs r2
				WHERE r2.session_id = r.session_id
				ORDER BY r2.created_at DESC
				LIMIT 1
			), 0) AS last_status_code
		FROM request_logs r
		WHERE r.session_id IS NOT NULL AND r.session_id <> ''%s
		GROUP BY r.session_id%s
		ORDER BY CASE WHEN MAX(r.created_at) >= ? THEN 1 ELSE 0 END DESC, MAX(r.created_at) DESC
		LIMIT ? OFFSET ?
	`, queryFilter, havingClause)

	listArgs := append([]interface{}{}, args...)
	listArgs = append(listArgs, activeCutoff, params.PageSize, offset)

	rows, err := db.Query(listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]model.AdminSessionListItem, 0, params.PageSize)
	for rows.Next() {
		var item model.AdminSessionListItem
		var firstSeenAt, lastSeenAt time.Time
		var lastStatusCode int
		if err := rows.Scan(
			&item.SessionID,
			&item.UserID,
			&item.Username,
			&item.Provider,
			&item.RequestCount,
			&firstSeenAt,
			&lastSeenAt,
			&item.LastModel,
			&lastStatusCode,
		); err != nil {
			return nil, 0, err
		}
		item.ID = item.SessionID
		item.FirstSeenAt = firstSeenAt.Format(time.RFC3339)
		item.LastSeenAt = lastSeenAt.Format(time.RFC3339)
		item.Active = !lastSeenAt.Before(activeCutoff)
		if item.Active {
			item.State = "active"
		} else {
			item.State = "idle"
		}
		item.LastStatusCode = &lastStatusCode
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *RequestLogRepository) GetSessionDetail(sessionID string, window time.Duration) (*model.AdminSessionDetailResponse, error) {
	db := database.GetDB()
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}

	var (
		item               model.AdminSessionListItem
		firstSeenAt        time.Time
		lastSeenAt         time.Time
		lastStatusCode     int
		distinctAPIKeyCount int64
		distinctModelCount int64
		totalInputTokens   int64
		totalOutputTokens  int64
		totalCostMicros    int64
	)

	err := db.QueryRow(`
		SELECT
			r.session_id,
			COALESCE((
				SELECT r2.user_id
				FROM request_logs r2
				WHERE r2.session_id = r.session_id
				ORDER BY r2.created_at DESC
				LIMIT 1
			), '') AS user_id,
			COALESCE((
				SELECT u2.username
				FROM request_logs r2
				LEFT JOIN users u2 ON u2.id = r2.user_id
				WHERE r2.session_id = r.session_id
				ORDER BY r2.created_at DESC
				LIMIT 1
			), '') AS username,
			COALESCE((
				SELECT COALESCE(NULLIF(r2.provider, ''), '')
				FROM request_logs r2
				WHERE r2.session_id = r.session_id
				ORDER BY r2.created_at DESC
				LIMIT 1
			), '') AS provider,
			COUNT(*) AS request_count,
			MIN(r.created_at) AS first_seen_at,
			MAX(r.created_at) AS last_seen_at,
			COALESCE((
				SELECT COALESCE(NULLIF(r2.mapped_model, ''), NULLIF(r2.original_model, ''), '')
				FROM request_logs r2
				WHERE r2.session_id = r.session_id
				ORDER BY r2.created_at DESC
				LIMIT 1
			), '') AS last_model,
			COALESCE((
				SELECT r2.status_code
				FROM request_logs r2
				WHERE r2.session_id = r.session_id
				ORDER BY r2.created_at DESC
				LIMIT 1
			), 0) AS last_status_code,
			COUNT(DISTINCT r.api_key_id) AS distinct_api_key_count,
			COUNT(DISTINCT COALESCE(NULLIF(r.mapped_model, ''), NULLIF(r.original_model, ''))) AS distinct_model_count,
			COALESCE(SUM(r.input_tokens), 0) AS total_input_tokens,
			COALESCE(SUM(r.output_tokens), 0) AS total_output_tokens,
			COALESCE(SUM(r.cost_micros), 0) AS total_cost_micros
		FROM request_logs r
		WHERE r.session_id = ?
		GROUP BY r.session_id
	`, sessionID).Scan(
		&item.SessionID,
		&item.UserID,
		&item.Username,
		&item.Provider,
		&item.RequestCount,
		&firstSeenAt,
		&lastSeenAt,
		&item.LastModel,
		&lastStatusCode,
		&distinctAPIKeyCount,
		&distinctModelCount,
		&totalInputTokens,
		&totalOutputTokens,
		&totalCostMicros,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	item.ID = item.SessionID
	item.FirstSeenAt = firstSeenAt.Format(time.RFC3339)
	item.LastSeenAt = lastSeenAt.Format(time.RFC3339)
	item.Active = !lastSeenAt.Before(time.Now().UTC().Add(-window))
	if item.Active {
		item.State = "active"
	} else {
		item.State = "idle"
	}
	item.LastStatusCode = &lastStatusCode

	timeline, err := r.listSessionTimeline(sessionID, 100)
	if err != nil {
		return nil, err
	}

	return &model.AdminSessionDetailResponse{
		Session:             item,
		Timeline:            timeline,
		DistinctAPIKeyCount: distinctAPIKeyCount,
		DistinctModelCount:  distinctModelCount,
		TotalInputTokens:    totalInputTokens,
		TotalOutputTokens:   totalOutputTokens,
		TotalCostUsd:        fmt.Sprintf("%.6f", float64(totalCostMicros)/1_000_000),
	}, nil
}

func (r *RequestLogRepository) listSessionTimeline(sessionID string, limit int) ([]model.RequestLog, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 100
	}

	rows, err := db.Query(`
		SELECT r.id, r.created_at, r.updated_at, r.status, r.user_id, u.username, r.api_key_id, k.name, k.prefix,
		       r.original_model, r.mapped_model, r.session_id, r.provider, r.channel_id, c.name, r.endpoint, r.request_format, r.upstream_format,
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
		WHERE r.session_id = ?
		ORDER BY r.created_at DESC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]model.RequestLog, 0, limit)
	for rows.Next() {
		var logEntry model.RequestLog
		var createdAt time.Time
		var updatedAt sql.NullTime
		var status sql.NullString
		var isStreaming int
		var username, apiKeyName, apiKeyPrefix sql.NullString
		var originalModel, mappedModel, rawSessionID, provider, channelID, channelName, endpoint, requestFormat, upstreamFormat, errorType, requestID, costUsd, pricingModel, pricingRuleName, thinkingLevel, downstreamTransport, upstreamTransport, transportFallbackReason, billingStatus, specialRateReason, translatorJSON sql.NullString
		var rateMultiplier, channelRateMultiplier, groupRateMultiplier, specialRateMultiplier sql.NullFloat64
		var inputTokens, outputTokens, cacheRead, cacheCreation, costMicros, ttfbMs, chargedSubscriptionMicros, chargedBalanceMicros sql.NullInt64

		if err := rows.Scan(
			&logEntry.ID, &createdAt, &updatedAt, &status, &logEntry.UserID, &username, &logEntry.APIKeyID, &apiKeyName, &apiKeyPrefix,
			&originalModel, &mappedModel, &rawSessionID, &provider, &channelID, &channelName, &endpoint, &requestFormat, &upstreamFormat,
			&logEntry.Method, &logEntry.Path, &logEntry.StatusCode, &logEntry.LatencyMs, &ttfbMs,
			&isStreaming, &inputTokens, &outputTokens, &cacheRead, &cacheCreation,
			&errorType, &requestID, &costMicros, &costUsd, &pricingModel, &pricingRuleName, &thinkingLevel,
			&rateMultiplier, &channelRateMultiplier, &groupRateMultiplier, &specialRateMultiplier, &specialRateReason, &translatorJSON,
			&downstreamTransport, &upstreamTransport, &transportFallbackReason,
			&chargedSubscriptionMicros, &chargedBalanceMicros, &billingStatus,
		); err != nil {
			return nil, err
		}

		logEntry.CreatedAt = createdAt.Format(time.RFC3339)
		logEntry.IsStreaming = isStreaming == 1
		if username.Valid {
			logEntry.Username = &username.String
		}
		if apiKeyName.Valid {
			logEntry.APIKeyName = &apiKeyName.String
		}
		if apiKeyPrefix.Valid {
			logEntry.APIKeyPrefix = &apiKeyPrefix.String
		}
		if updatedAt.Valid {
			formatted := updatedAt.Time.Format(time.RFC3339)
			logEntry.UpdatedAt = &formatted
		}
		if status.Valid {
			logEntry.Status = model.RequestLogStatus(status.String)
		} else {
			logEntry.Status = model.RequestLogStatusSuccess
		}
		if ttfbMs.Valid {
			logEntry.TTFBMs = &ttfbMs.Int64
		}
		if originalModel.Valid {
			logEntry.OriginalModel = &originalModel.String
		}
		if mappedModel.Valid {
			logEntry.MappedModel = &mappedModel.String
		}
		if rawSessionID.Valid {
			logEntry.SessionID = &rawSessionID.String
		}
		if provider.Valid {
			logEntry.Provider = &provider.String
		}
		if channelID.Valid {
			logEntry.ChannelID = &channelID.String
		}
		if channelName.Valid {
			logEntry.ChannelName = &channelName.String
		}
		if endpoint.Valid {
			logEntry.Endpoint = &endpoint.String
		}
		if requestFormat.Valid {
			logEntry.RequestFormat = &requestFormat.String
		}
		if upstreamFormat.Valid {
			logEntry.UpstreamFormat = &upstreamFormat.String
		}
		if errorType.Valid {
			logEntry.ErrorType = &errorType.String
		}
		if requestID.Valid {
			logEntry.RequestID = &requestID.String
		}
		if inputTokens.Valid {
			v := int(inputTokens.Int64)
			logEntry.InputTokens = &v
		}
		if outputTokens.Valid {
			v := int(outputTokens.Int64)
			logEntry.OutputTokens = &v
		}
		if cacheRead.Valid {
			v := int(cacheRead.Int64)
			logEntry.CacheReadInputTokens = &v
		}
		if cacheCreation.Valid {
			v := int(cacheCreation.Int64)
			logEntry.CacheCreationInputTokens = &v
		}
		if costMicros.Valid {
			logEntry.CostMicros = &costMicros.Int64
		}
		if costUsd.Valid {
			logEntry.CostUsd = &costUsd.String
		}
		if pricingModel.Valid {
			logEntry.PricingModel = &pricingModel.String
		}
		if pricingRuleName.Valid {
			logEntry.PricingRuleName = &pricingRuleName.String
		}
		if thinkingLevel.Valid {
			logEntry.ThinkingLevel = &thinkingLevel.String
		}
		if rateMultiplier.Valid {
			logEntry.RateMultiplier = &rateMultiplier.Float64
		}
		if channelRateMultiplier.Valid {
			logEntry.ChannelRateMultiplier = &channelRateMultiplier.Float64
		}
		if groupRateMultiplier.Valid {
			logEntry.GroupRateMultiplier = &groupRateMultiplier.Float64
		}
		if specialRateMultiplier.Valid {
			logEntry.SpecialRateMultiplier = &specialRateMultiplier.Float64
		}
		if specialRateReason.Valid {
			logEntry.SpecialRateReason = &specialRateReason.String
		}
		if translatorJSON.Valid {
			logEntry.ChannelTranslator = parseRequestLogTranslator(translatorJSON.String)
		}
		if downstreamTransport.Valid {
			logEntry.DownstreamTransport = &downstreamTransport.String
		}
		if upstreamTransport.Valid {
			logEntry.UpstreamTransport = &upstreamTransport.String
		}
		if transportFallbackReason.Valid {
			logEntry.TransportFallbackReason = &transportFallbackReason.String
		}
		if chargedSubscriptionMicros.Valid {
			logEntry.ChargedSubscriptionMicros = chargedSubscriptionMicros.Int64
		}
		if chargedBalanceMicros.Valid {
			logEntry.ChargedBalanceMicros = chargedBalanceMicros.Int64
		}
		if billingStatus.Valid {
			logEntry.BillingStatus = billingStatus.String
		}
		enrichRequestLogPricing(&logEntry)
		enrichRequestLogMetrics(&logEntry)
		logs = append(logs, logEntry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return logs, nil
}

func (r *RequestLogRepository) GetSessionLeaderboard(window time.Duration, limit int) ([]model.AdminSessionLeaderboardItem, error) {
	db := database.GetDB()
	if limit <= 0 {
		limit = 10
	}

	rows, err := db.Query(`
		SELECT r.user_id, COALESCE(u.username, ''), COUNT(DISTINCT r.session_id) AS distinct_session_count, COUNT(*) AS request_count
		FROM request_logs r
		LEFT JOIN users u ON u.id = r.user_id
		WHERE r.session_id IS NOT NULL AND r.session_id <> '' AND r.created_at >= ?
		GROUP BY r.user_id, u.username
		ORDER BY distinct_session_count DESC, request_count DESC, MAX(r.created_at) DESC
		LIMIT ?
	`, time.Now().UTC().Add(-window), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.AdminSessionLeaderboardItem, 0, limit)
	for rows.Next() {
		var item model.AdminSessionLeaderboardItem
		if err := rows.Scan(&item.UserID, &item.Username, &item.DistinctSessionCount, &item.RequestCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}
