package amp

import (
	"ampmanager/internal/database"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	_ "modernc.org/sqlite"
)

const (
	DefaultDetailTTL        = 5 * time.Minute
	DetailCleanupInterval   = 30 * time.Second
	DetailDBArchiveInterval = 1 * time.Hour
	DefaultArchiveDays      = 30
	ArchiveBatchSize        = 200             // 每批归档行数，避免大事务
	MaxBodySize             = 1 * 1024 * 1024 // 1MB max body size to store
	MaxDetailEntries        = 10000           // 内存中最多保存的条目数

	requestDetailArchiveKey = "request_detail_archive_days"
)

// RequestDetail stores request/response headers and bodies
type RequestDetail struct {
	RequestID              string
	CreatedAt              time.Time
	LastUpdatedAt          time.Time
	RequestHeaders         http.Header
	RequestBody            []byte
	TranslatedRequestBody    []byte      // 翻译后发送给上游的请求体
	TranslatedRequestHeaders http.Header // 翻译后发送给上游的请求头
	ResponseHeaders          http.Header
	ResponseBody           []byte
	TranslatedResponseBody []byte // 翻译后发送给客户端的响应体
	Persisted              bool
}

// RequestDetailStore stores request details in memory with TTL
type RequestDetailStore struct {
	mu               sync.RWMutex
	details          map[string]*RequestDetail
	db               *sql.DB
	archiveDB        *sql.DB
	hotTableName     string
	archiveTableName string
	ownsArchiveDB    bool
	ttl              time.Duration
	archiveDays      int
	lastArchiveAt    time.Time
	stopChan         chan struct{}
	wg               sync.WaitGroup
}

var (
	globalDetailStore *RequestDetailStore
	detailStoreOnce   sync.Once
	detailStoreMu     sync.Mutex
)

// InitRequestDetailStore initializes the global request detail store
func InitRequestDetailStore(db *sql.DB) {
	detailStoreOnce.Do(func() {
		globalDetailStore = NewRequestDetailStore(db, DefaultDetailTTL)
		log.Info("request detail store: initialized")
	})
}

// ReinitRequestDetailStore reinitializes the global request detail store (after db replacement)
func ReinitRequestDetailStore(db *sql.DB) {
	detailStoreMu.Lock()
	defer detailStoreMu.Unlock()
	if globalDetailStore != nil {
		globalDetailStore.Stop()
	}
	globalDetailStore = NewRequestDetailStore(db, DefaultDetailTTL)
	log.Info("request detail store: reinitialized")
}

// GetRequestDetailStore returns the global request detail store
func GetRequestDetailStore() *RequestDetailStore {
	return globalDetailStore
}

// StopRequestDetailStore stops the global request detail store
func StopRequestDetailStore() {
	if globalDetailStore != nil {
		globalDetailStore.Stop()
		log.Info("request detail store: stopped")
	}
}

// NewRequestDetailStore creates a new request detail store
func NewRequestDetailStore(db *sql.DB, ttl time.Duration) *RequestDetailStore {
	s := &RequestDetailStore{
		details:      make(map[string]*RequestDetail),
		db:           db,
		hotTableName: "request_log_details",
		ttl:          ttl,
		archiveDays:  DefaultArchiveDays,
		stopChan:     make(chan struct{}),
	}
	s.archiveDays = s.loadArchiveDays()
	s.archiveDB = s.openArchiveDB()
	log.Infof("request detail store: archive threshold set to %d days", s.archiveDays)
	s.wg.Add(1)
	go s.cleanupLoop()
	return s
}

// openArchiveDB opens (or creates) the archive SQLite database next to the main DB.
func (s *RequestDetailStore) openArchiveDB() *sql.DB {
	if s.db == nil {
		return nil
	}
	if database.IsPostgres() {
		s.archiveTableName = "request_log_details_archive"
		s.ownsArchiveDB = false
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS request_log_details_archive (
				request_id TEXT PRIMARY KEY,
				request_headers TEXT,
				request_body TEXT,
				translated_request_body TEXT,
				translated_request_headers TEXT,
				response_headers TEXT,
				response_body TEXT,
				translated_response_body TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
			)
		`)
		if err != nil {
			log.Warnf("request detail store: failed to ensure postgres archive table: %v", err)
			return nil
		}
		_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_request_log_details_archive_created ON request_log_details_archive(created_at DESC)`)
		if err != nil {
			log.Warnf("request detail store: failed to ensure postgres archive index: %v", err)
			return nil
		}
		return s.db
	}

	// 使用 database.GetPath() 获取主库路径，归档库放在同级目录
	mainPath := getMainDBPath()
	if mainPath == "" {
		return nil
	}
	s.archiveTableName = "request_log_details"

	archivePath := filepath.Join(filepath.Dir(mainPath), "data_details_archive.db")
	dsn := archivePath + "?_pragma=foreign_keys(OFF)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	adb, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Warnf("request detail store: failed to open archive db: %v", err)
		return nil
	}
	if err = adb.Ping(); err != nil {
		adb.Close()
		log.Warnf("request detail store: archive db ping failed: %v", err)
		return nil
	}

	adb.SetMaxOpenConns(3)
	adb.SetMaxIdleConns(1)
	adb.SetConnMaxLifetime(time.Hour)

	// 建表
	_, err = adb.Exec(`
		CREATE TABLE IF NOT EXISTS request_log_details (
			request_id TEXT PRIMARY KEY,
			request_headers TEXT,
			request_body TEXT,
			translated_request_body TEXT,
			translated_request_headers TEXT,
			response_headers TEXT,
			response_body TEXT,
				translated_response_body TEXT,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_archive_details_created ON request_log_details(created_at DESC);
		`)
	if err != nil {
		log.Warnf("request detail store: failed to create archive table: %v", err)
		adb.Close()
		return nil
	}
	_, _ = adb.Exec(`ALTER TABLE request_log_details ADD COLUMN translated_request_body TEXT`)
	_, _ = adb.Exec(`ALTER TABLE request_log_details ADD COLUMN translated_response_body TEXT`)
	_, _ = adb.Exec(`ALTER TABLE request_log_details ADD COLUMN translated_request_headers TEXT`)

	s.ownsArchiveDB = true
	log.Info("request detail store: archive db ready")
	return adb
}

// getMainDBPath returns the main database file path using the database package.
func getMainDBPath() string {
	// 延迟导入避免循环依赖：通过 database.GetPath() 获取
	// 这里直接使用 database 包的函数
	return getDBPathFunc()
}

// getDBPathFunc is set during init to avoid import cycles.
// It will be overridden by the init function in request_detail_store_init.go
var getDBPathFunc = func() string { return "" }

func (s *RequestDetailStore) loadArchiveDays() int {
	if s.db == nil {
		return DefaultArchiveDays
	}

	var value string
	err := s.db.QueryRow(`SELECT value FROM system_config WHERE key = ?`, requestDetailArchiveKey).Scan(&value)
	if err == sql.ErrNoRows {
		return DefaultArchiveDays
	}
	if err != nil {
		log.Warnf("request detail store: failed to load archive days from db: %v", err)
		return DefaultArchiveDays
	}

	days, convErr := strconv.Atoi(strings.TrimSpace(value))
	if convErr != nil || days < 1 || days > 3650 {
		log.Warnf("request detail store: invalid archive days '%s', fallback to %d", value, DefaultArchiveDays)
		return DefaultArchiveDays
	}

	return days
}

// Store stores request detail in memory
func (s *RequestDetailStore) Store(detail *RequestDetail) {
	if detail == nil || detail.RequestID == "" {
		return
	}
	detail.CreatedAt = time.Now().UTC()
	detail.LastUpdatedAt = time.Now().UTC()

	s.mu.Lock()
	if len(s.details) >= MaxDetailEntries {
		s.evictOldestLocked()
	}
	s.details[detail.RequestID] = detail
	s.mu.Unlock()

	log.Debugf("request detail store: stored detail for %s", detail.RequestID)
}

// evictOldestLocked 驱逐最老的条目（必须在持锁状态下调用）
func (s *RequestDetailStore) evictOldestLocked() {
	var oldestID string
	var oldestTime time.Time
	first := true

	for id, detail := range s.details {
		if first || detail.LastUpdatedAt.Before(oldestTime) {
			oldestID = id
			oldestTime = detail.LastUpdatedAt
			first = false
		}
	}

	if oldestID != "" {
		delete(s.details, oldestID)
		log.Debugf("request detail store: evicted oldest entry %s due to max entries limit", oldestID)
	}
}

// UpdateRequestData updates the request headers and body
func (s *RequestDetailStore) UpdateRequestData(requestID string, headers http.Header, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	detail, exists := s.details[requestID]
	if !exists {
		now := time.Now().UTC()
		detail = &RequestDetail{
			RequestID:     requestID,
			CreatedAt:     now,
			LastUpdatedAt: now,
		}
		s.details[requestID] = detail
	}
	detail.LastUpdatedAt = time.Now().UTC()
	detail.RequestHeaders = headers.Clone()
	if len(body) <= MaxBodySize {
		detail.RequestBody = make([]byte, len(body))
		copy(detail.RequestBody, body)
	} else {
		detail.RequestBody = make([]byte, MaxBodySize)
		copy(detail.RequestBody, body[:MaxBodySize])
	}
}

// UpdateTranslatedRequestBody stores the translated request body
func (s *RequestDetailStore) UpdateTranslatedRequestBody(requestID string, body []byte) {
	if len(body) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	detail, exists := s.details[requestID]
	if !exists {
		now := time.Now().UTC()
		detail = &RequestDetail{
			RequestID:     requestID,
			CreatedAt:     now,
			LastUpdatedAt: now,
		}
		s.details[requestID] = detail
	}
	detail.LastUpdatedAt = time.Now().UTC()
	if len(body) <= MaxBodySize {
		detail.TranslatedRequestBody = make([]byte, len(body))
		copy(detail.TranslatedRequestBody, body)
	} else {
		detail.TranslatedRequestBody = make([]byte, MaxBodySize)
		copy(detail.TranslatedRequestBody, body[:MaxBodySize])
	}
}

// UpdateTranslatedRequestHeaders stores the translated request headers
func (s *RequestDetailStore) UpdateTranslatedRequestHeaders(requestID string, headers http.Header) {
	if len(headers) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	detail, exists := s.details[requestID]
	if !exists {
		now := time.Now().UTC()
		detail = &RequestDetail{
			RequestID:     requestID,
			CreatedAt:     now,
			LastUpdatedAt: now,
		}
		s.details[requestID] = detail
	}
	detail.LastUpdatedAt = time.Now().UTC()
	detail.TranslatedRequestHeaders = headers.Clone()
}

// UpdateResponseData updates the response headers and body
func (s *RequestDetailStore) UpdateResponseData(requestID string, headers http.Header, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	detail, exists := s.details[requestID]
	if !exists {
		now := time.Now().UTC()
		detail = &RequestDetail{
			RequestID:     requestID,
			CreatedAt:     now,
			LastUpdatedAt: now,
		}
		s.details[requestID] = detail
	}
	detail.LastUpdatedAt = time.Now().UTC()
	detail.ResponseHeaders = headers.Clone()
	if len(body) <= MaxBodySize {
		detail.ResponseBody = make([]byte, len(body))
		copy(detail.ResponseBody, body)
	} else {
		detail.ResponseBody = make([]byte, MaxBodySize)
		copy(detail.ResponseBody, body[:MaxBodySize])
	}
}

// AppendTranslatedResponse appends translated response data for debugging
func (s *RequestDetailStore) AppendTranslatedResponse(requestID string, data []byte) {
	if len(data) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	detail, exists := s.details[requestID]
	if !exists {
		return
	}

	// 限制翻译响应大小
	currentLen := len(detail.TranslatedResponseBody)
	if currentLen >= MaxBodySize {
		return
	}

	remaining := MaxBodySize - currentLen
	if len(data) > remaining {
		data = data[:remaining]
	}

	detail.TranslatedResponseBody = append(detail.TranslatedResponseBody, data...)
	detail.LastUpdatedAt = time.Now().UTC()
}

// copyDetail creates a deep copy of a RequestDetail
func copyDetail(detail *RequestDetail) *RequestDetail {
	copied := &RequestDetail{
		RequestID:              detail.RequestID,
		CreatedAt:              detail.CreatedAt,
		LastUpdatedAt:          detail.LastUpdatedAt,
		RequestHeaders:         nil,
		RequestBody:            make([]byte, len(detail.RequestBody)),
		TranslatedRequestBody:  make([]byte, len(detail.TranslatedRequestBody)),
		ResponseHeaders:        nil,
		ResponseBody:           make([]byte, len(detail.ResponseBody)),
		TranslatedResponseBody: make([]byte, len(detail.TranslatedResponseBody)),
		Persisted:              detail.Persisted,
	}
	copy(copied.RequestBody, detail.RequestBody)
	copy(copied.TranslatedRequestBody, detail.TranslatedRequestBody)
	copy(copied.ResponseBody, detail.ResponseBody)
	copy(copied.TranslatedResponseBody, detail.TranslatedResponseBody)
	if detail.RequestHeaders != nil {
		copied.RequestHeaders = detail.RequestHeaders.Clone()
	}
	if detail.TranslatedRequestHeaders != nil {
		copied.TranslatedRequestHeaders = detail.TranslatedRequestHeaders.Clone()
	}
	if detail.ResponseHeaders != nil {
		copied.ResponseHeaders = detail.ResponseHeaders.Clone()
	}
	return copied
}

// Get retrieves request detail by ID (from memory first, then hot DB, then archive DB)
func (s *RequestDetailStore) Get(requestID string) *RequestDetail {
	s.mu.RLock()
	detail, exists := s.details[requestID]
	if exists {
		copied := copyDetail(detail)
		s.mu.RUnlock()
		return copied
	}
	s.mu.RUnlock()

	// 先查热库
	if d := s.getFromDB(s.db, s.hotTableName, requestID); d != nil {
		return d
	}
	// 再查归档库
	if s.archiveDB != nil {
		return s.getFromDB(s.archiveDB, s.archiveTableName, requestID)
	}
	return nil
}

// getFromDB retrieves request detail from a given database connection
func (s *RequestDetailStore) getFromDB(db *sql.DB, tableName, requestID string) *RequestDetail {
	if db == nil {
		return nil
	}

	var detail RequestDetail
	var requestHeaders, requestBody, translatedRequestBody, translatedRequestHeaders, responseHeaders, responseBody, translatedResponseBody sql.NullString

	query := fmt.Sprintf(`
		SELECT request_id, request_headers, request_body, translated_request_body, translated_request_headers, response_headers, response_body, translated_response_body, created_at
		FROM %s
		WHERE request_id = ?
	`, tableName)
	err := db.QueryRow(query, requestID).Scan(
		&detail.RequestID,
		&requestHeaders,
		&requestBody,
		&translatedRequestBody,
		&translatedRequestHeaders,
		&responseHeaders,
		&responseBody,
		&translatedResponseBody,
		&detail.CreatedAt,
	)

	if err != nil {
		if err != sql.ErrNoRows {
			log.Errorf("request detail store: failed to query from db: %v", err)
		}
		return nil
	}

	if requestHeaders.Valid {
		detail.RequestHeaders = parseHeadersJSON(requestHeaders.String)
	}
	if translatedRequestHeaders.Valid {
		detail.TranslatedRequestHeaders = parseHeadersJSON(translatedRequestHeaders.String)
	}
	if responseHeaders.Valid {
		detail.ResponseHeaders = parseHeadersJSON(responseHeaders.String)
	}
	if requestBody.Valid {
		detail.RequestBody = []byte(requestBody.String)
	}
	if translatedRequestBody.Valid {
		detail.TranslatedRequestBody = []byte(translatedRequestBody.String)
	}
	if responseBody.Valid {
		detail.ResponseBody = []byte(responseBody.String)
	}
	if translatedResponseBody.Valid {
		detail.TranslatedResponseBody = []byte(translatedResponseBody.String)
	}
	detail.LastUpdatedAt = detail.CreatedAt
	detail.Persisted = true

	return &detail
}

// persistToDB persists request detail to database
func (s *RequestDetailStore) persistToDB(detail *RequestDetail) error {
	if s.db == nil || detail == nil {
		return nil
	}

	requestHeadersJSON := headersToJSON(detail.RequestHeaders)
	translatedRequestHeadersJSON := headersToJSON(detail.TranslatedRequestHeaders)
	responseHeadersJSON := headersToJSON(detail.ResponseHeaders)
	requestBody := sanitizeBodyForStorage(detail.RequestBody)
	translatedRequestBody := sanitizeBodyForStorage(detail.TranslatedRequestBody)
	responseBody := sanitizeBodyForStorage(detail.ResponseBody)
	translatedResponseBody := sanitizeBodyForStorage(detail.TranslatedResponseBody)

	query := fmt.Sprintf(`
		INSERT INTO %s
		(request_id, request_headers, request_body, translated_request_body, translated_request_headers, response_headers, response_body, translated_response_body, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (request_id) DO UPDATE SET
			request_headers = excluded.request_headers,
			request_body = excluded.request_body,
			translated_request_body = excluded.translated_request_body,
			translated_request_headers = excluded.translated_request_headers,
			response_headers = excluded.response_headers,
			response_body = excluded.response_body,
			translated_response_body = excluded.translated_response_body,
			created_at = excluded.created_at
	`, s.hotTableName)
	_, err := s.db.Exec(query,
		detail.RequestID,
		requestHeadersJSON,
		requestBody,
		translatedRequestBody,
		translatedRequestHeadersJSON,
		responseHeadersJSON,
		responseBody,
		translatedResponseBody,
		detail.CreatedAt.UTC(),
	)

	if err != nil {
		log.Errorf("request detail store: failed to persist to db: %v", err)
		return err
	}

	log.Debugf("request detail store: persisted detail for %s", detail.RequestID)
	return nil
}

// cleanupLoop periodically cleans up expired entries and persists them to database
func (s *RequestDetailStore) cleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(DetailCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopChan:
			s.persistAll()
			return
		}
	}
}

// cleanup removes expired entries and persists them to database
func (s *RequestDetailStore) cleanup() {
	now := time.Now().UTC()
	var expiredIDs []string
	var snapshots []*RequestDetail

	s.mu.RLock()
	for id, detail := range s.details {
		if now.Sub(detail.LastUpdatedAt) > s.ttl {
			expiredIDs = append(expiredIDs, id)
			if !detail.Persisted {
				snapshots = append(snapshots, copyDetail(detail))
			}
		}
	}
	s.mu.RUnlock()

	persistedIDs := make(map[string]bool)
	for _, snapshot := range snapshots {
		if err := s.persistToDB(snapshot); err == nil {
			persistedIDs[snapshot.RequestID] = true
		}
	}

	if len(expiredIDs) > 0 {
		s.mu.Lock()
		for _, id := range expiredIDs {
			detail, exists := s.details[id]
			if exists && (detail.Persisted || persistedIDs[id]) {
				delete(s.details, id)
			}
		}
		s.mu.Unlock()
	}

	if len(persistedIDs) > 0 {
		log.Debugf("request detail store: cleaned up %d expired entries", len(persistedIDs))
	}

	s.archiveOldDetails(now)
}

// archiveOldDetails moves old rows from hot DB to archive DB (two-phase: copy then delete).
// Data is NEVER lost — worst case is duplication (row exists in both), never deletion without copy.
func (s *RequestDetailStore) archiveOldDetails(now time.Time) {
	if s.db == nil || s.archiveDB == nil || s.archiveDays <= 0 {
		return
	}
	if !s.lastArchiveAt.IsZero() && now.Sub(s.lastArchiveAt) < DetailDBArchiveInterval {
		return
	}

	// 每次真正执行归档前重新读取配置，支持运行时动态更新
	if configuredDays := s.loadArchiveDays(); configuredDays != s.archiveDays {
		s.archiveDays = configuredDays
		log.Infof("request detail store: archive threshold changed to %d days", s.archiveDays)
	}

	s.lastArchiveAt = now
	cutoff := now.AddDate(0, 0, -s.archiveDays).UTC()

	// 查找需要归档的行（分批处理）
	query := fmt.Sprintf(`SELECT request_id, request_headers, request_body, translated_request_body, translated_request_headers, response_headers, response_body, translated_response_body, created_at
		 FROM %s WHERE created_at < ? ORDER BY created_at LIMIT ?`, s.hotTableName)
	rows, err := s.db.Query(query, cutoff, ArchiveBatchSize)
	if err != nil {
		log.Warnf("request detail store: archive query failed: %v", err)
		return
	}
	defer rows.Close()

	type row struct {
		requestID                string
		requestHeaders           sql.NullString
		requestBody              sql.NullString
		translatedRequestBody    sql.NullString
		translatedRequestHeaders sql.NullString
		responseHeaders          sql.NullString
		responseBody             sql.NullString
		translatedResponseBody   sql.NullString
		createdAt                time.Time
	}
	var batch []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.requestID, &r.requestHeaders, &r.requestBody, &r.translatedRequestBody, &r.translatedRequestHeaders, &r.responseHeaders, &r.responseBody, &r.translatedResponseBody, &r.createdAt); err != nil {
			log.Warnf("request detail store: archive scan failed: %v", err)
			return
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		log.Warnf("request detail store: archive rows error: %v", err)
		return
	}

	if len(batch) == 0 {
		return
	}

	// Phase 1: 复制到归档库（INSERT OR IGNORE 保证幂等）
	ctx := context.Background()
	archiveTx, err := s.archiveDB.BeginTx(ctx, nil)
	if err != nil {
		log.Warnf("request detail store: archive begin tx failed: %v", err)
		return
	}

	archiveInsertSQL := fmt.Sprintf(`INSERT INTO %s
		(request_id, request_headers, request_body, translated_request_body, translated_request_headers, response_headers, response_body, translated_response_body, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (request_id) DO NOTHING`, s.archiveTableName)
	stmt, err := archiveTx.Prepare(archiveInsertSQL)
	if err != nil {
		archiveTx.Rollback()
		log.Warnf("request detail store: archive prepare failed: %v", err)
		return
	}
	defer stmt.Close()

	for _, r := range batch {
		_, err := stmt.Exec(r.requestID, r.requestHeaders, r.requestBody, r.translatedRequestBody, r.translatedRequestHeaders, r.responseHeaders, r.responseBody, r.translatedResponseBody, r.createdAt)
		if err != nil {
			archiveTx.Rollback()
			log.Warnf("request detail store: archive insert failed: %v", err)
			return
		}
	}

	if err := archiveTx.Commit(); err != nil {
		log.Warnf("request detail store: archive commit failed: %v", err)
		return
	}

	// Phase 2: 验证归档成功后，才从热库删除
	var ids []string
	for _, r := range batch {
		// 逐条验证归档库中确实存在
		var exists int
		verifySQL := fmt.Sprintf(`SELECT 1 FROM %s WHERE request_id = ?`, s.archiveTableName)
		if err := s.archiveDB.QueryRow(verifySQL, r.requestID).Scan(&exists); err != nil {
			log.Warnf("request detail store: archive verify failed for %s, skipping delete: %v", r.requestID, err)
			return // 任何一条验证失败就停止删除，保证安全
		}
		ids = append(ids, r.requestID)
	}

	// 从热库批量删除已验证归档的行
	hotTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		log.Warnf("request detail store: hot delete begin tx failed: %v", err)
		return
	}

	deleteSQL := fmt.Sprintf(`DELETE FROM %s WHERE request_id = ?`, s.hotTableName)
	delStmt, err := hotTx.Prepare(deleteSQL)
	if err != nil {
		hotTx.Rollback()
		log.Warnf("request detail store: hot delete prepare failed: %v", err)
		return
	}
	defer delStmt.Close()

	for _, id := range ids {
		if _, err := delStmt.Exec(id); err != nil {
			hotTx.Rollback()
			log.Warnf("request detail store: hot delete failed: %v", err)
			return
		}
	}

	if err := hotTx.Commit(); err != nil {
		log.Warnf("request detail store: hot delete commit failed: %v", err)
		return
	}

	log.Infof("request detail store: archived %d rows older than %d days", len(ids), s.archiveDays)
}

// persistAll persists all entries to database (called on shutdown)
func (s *RequestDetailStore) persistAll() {
	s.mu.RLock()
	snapshots := make([]*RequestDetail, 0, len(s.details))
	for _, detail := range s.details {
		if !detail.Persisted {
			snapshots = append(snapshots, copyDetail(detail))
		}
	}
	s.mu.RUnlock()

	count := 0
	for _, snapshot := range snapshots {
		if err := s.persistToDB(snapshot); err == nil {
			count++
		}
	}

	if count > 0 {
		log.Infof("request detail store: persisted %d entries on shutdown", count)
	}
}

// Stop stops the cleanup loop
func (s *RequestDetailStore) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	if s.ownsArchiveDB && s.archiveDB != nil {
		s.archiveDB.Close()
	}
}

// Helper functions for JSON serialization of headers
func headersToJSON(headers http.Header) string {
	if headers == nil {
		return "{}"
	}
	data, err := json.Marshal(headers)
	if err != nil {
		log.Warnf("request detail store: failed to marshal headers: %v", err)
		return "{}"
	}
	return string(data)
}

func parseHeadersJSON(jsonStr string) http.Header {
	headers := make(http.Header)
	if jsonStr == "" || jsonStr == "{}" {
		return headers
	}

	var data map[string][]string
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		log.Warnf("request detail store: failed to parse headers JSON: %v", err)
		return headers
	}

	for k, v := range data {
		headers[k] = v
	}
	return headers
}

func sanitizeBodyForStorage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	cleaned := bytes.ToValidUTF8(body, []byte("\uFFFD"))
	cleaned = bytes.ReplaceAll(cleaned, []byte{0}, []byte{})
	return string(cleaned)
}
