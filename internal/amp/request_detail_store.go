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
	DetailDBArchiveInterval = 1 * time.Hour
	DefaultArchiveDays      = 30
	ArchiveBatchSize        = 200

	requestDetailArchiveKey = "request_detail_archive_days"
)

// RequestDetail stores request/response headers and bodies.
type RequestDetail struct {
	RequestID                string
	CreatedAt                time.Time
	LastUpdatedAt            time.Time
	RequestHeaders           http.Header
	RequestBody              []byte
	TranslatedRequestBody    []byte
	TranslatedRequestHeaders http.Header
	ResponseHeaders          http.Header
	ResponseBody             []byte
	TranslatedResponseBody   []byte
	Persisted                bool
	MetadataOnly             bool
	Truncated                bool
	ApproxBytes              int64
}

// RequestDetailStore stores request details in memory with a hard budget.
type RequestDetailStore struct {
	mu               sync.RWMutex
	details          map[string]*RequestDetail
	currentBytes     int64
	db               *sql.DB
	archiveDB        *sql.DB
	hotTableName     string
	archiveTableName string
	ownsArchiveDB    bool
	ttl              time.Duration
	archiveDays      int
	lastArchiveAt    time.Time
	persistQueue     chan *RequestDetail
	stopChan         chan struct{}
	wg               sync.WaitGroup
}

var (
	globalDetailStore *RequestDetailStore
	detailStoreOnce   sync.Once
	detailStoreMu     sync.Mutex
)

// InitRequestDetailStore initializes the global request detail store.
func InitRequestDetailStore(db *sql.DB) {
	detailStoreOnce.Do(func() {
		globalDetailStore = NewRequestDetailStore(db, 0)
		log.Info("request detail store: initialized")
	})
}

// ReinitRequestDetailStore reinitializes the global request detail store (after db replacement).
func ReinitRequestDetailStore(db *sql.DB) {
	detailStoreMu.Lock()
	defer detailStoreMu.Unlock()
	if globalDetailStore != nil {
		globalDetailStore.Stop()
	}
	globalDetailStore = NewRequestDetailStore(db, 0)
	log.Info("request detail store: reinitialized")
}

// GetRequestDetailStore returns the global request detail store.
func GetRequestDetailStore() *RequestDetailStore {
	return globalDetailStore
}

// StopRequestDetailStore stops the global request detail store.
func StopRequestDetailStore() {
	if globalDetailStore != nil {
		globalDetailStore.Stop()
		log.Info("request detail store: stopped")
	}
}

// NewRequestDetailStore creates a new request detail store.
func NewRequestDetailStore(db *sql.DB, ttl time.Duration) *RequestDetailStore {
	cfg := GetRequestDetailConfig()
	if ttl > 0 {
		cfg.TTL = ttl
	}

	s := &RequestDetailStore{
		details:      make(map[string]*RequestDetail),
		db:           db,
		hotTableName: "request_log_details",
		ttl:          cfg.TTL,
		archiveDays:  DefaultArchiveDays,
		persistQueue: make(chan *RequestDetail, DefaultRequestDetailPersistQueueCap),
		stopChan:     make(chan struct{}),
	}
	s.archiveDays = s.loadArchiveDays()
	s.archiveDB = s.openArchiveDB()
	log.Infof("request detail store: archive threshold set to %d days", s.archiveDays)

	s.wg.Add(2)
	go s.cleanupLoop()
	go s.persistLoop()

	return s
}

func (s *RequestDetailStore) ApplyConfig(cfg RequestDetailConfig) {
	if s == nil {
		return
	}

	cfg = normalizeRequestDetailConfig(cfg)

	var snapshots []*RequestDetail

	s.mu.Lock()
	s.ttl = cfg.TTL
	if !cfg.Enabled {
		snapshots = s.collectAndClearLocked(cfg.PersistEnabled)
	} else {
		snapshots = s.enforceBudgetLocked(cfg, "")
	}
	s.mu.Unlock()

	s.enqueueSnapshots(cfg, snapshots)
}

// openArchiveDB opens (or creates) the archive database next to the main DB.
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

// Store keeps backward compatibility for tests and any cold path callers.
func (s *RequestDetailStore) Store(detail *RequestDetail) {
	if detail == nil || detail.RequestID == "" {
		return
	}

	cfg := GetRequestDetailConfig()
	if !cfg.Enabled {
		return
	}

	var snapshots []*RequestDetail
	now := time.Now().UTC()

	s.mu.Lock()
	copied := copyDetail(detail)
	copied.CreatedAt = now
	copied.LastUpdatedAt = now
	applyDetailBodyCap(copied, cfg.BodyCapBytes)
	oldSize := int64(0)
	if existing := s.details[detail.RequestID]; existing != nil {
		oldSize = existing.ApproxBytes
	}
	copied.ApproxBytes = estimateDetailBytes(copied)
	s.details[detail.RequestID] = copied
	s.currentBytes += copied.ApproxBytes - oldSize
	snapshots = s.enforceBudgetLocked(cfg, copied.RequestID)
	s.mu.Unlock()

	s.enqueueSnapshots(cfg, snapshots)
}

func (s *RequestDetailStore) UpdateRequestData(requestID string, headers http.Header, body []byte) {
	s.mutateDetail(requestID, func(detail *RequestDetail, cfg RequestDetailConfig) {
		detail.RequestHeaders = cloneHeaders(headers)
		detail.RequestBody = cloneBodyWithCap(body, cfg.BodyCapBytes)
		detail.MetadataOnly = false
		detail.Truncated = len(body) > cfg.BodyCapBytes
	}, true)
}

func (s *RequestDetailStore) UpdateTranslatedRequestBody(requestID string, body []byte) {
	if len(body) == 0 {
		return
	}

	s.mutateDetail(requestID, func(detail *RequestDetail, cfg RequestDetailConfig) {
		detail.TranslatedRequestBody = cloneBodyWithCap(body, cfg.BodyCapBytes)
		detail.MetadataOnly = false
		detail.Truncated = detail.Truncated || len(body) > cfg.BodyCapBytes
	}, false)
}

func (s *RequestDetailStore) UpdateTranslatedRequestHeaders(requestID string, headers http.Header) {
	if len(headers) == 0 {
		return
	}

	s.mutateDetail(requestID, func(detail *RequestDetail, _ RequestDetailConfig) {
		detail.TranslatedRequestHeaders = cloneHeaders(headers)
	}, false)
}

func (s *RequestDetailStore) UpdateResponseData(requestID string, headers http.Header, body []byte) {
	s.mutateDetail(requestID, func(detail *RequestDetail, cfg RequestDetailConfig) {
		detail.ResponseHeaders = cloneHeaders(headers)
		detail.ResponseBody = cloneBodyWithCap(body, cfg.BodyCapBytes)
		detail.MetadataOnly = false
		detail.Truncated = detail.Truncated || len(body) > cfg.BodyCapBytes
	}, false)
}

func (s *RequestDetailStore) AppendTranslatedResponse(requestID string, data []byte) {
	if len(data) == 0 {
		return
	}

	s.mutateDetail(requestID, func(detail *RequestDetail, cfg RequestDetailConfig) {
		if detail.MetadataOnly {
			return
		}
		currentLen := len(detail.TranslatedResponseBody)
		if currentLen >= cfg.BodyCapBytes {
			detail.Truncated = true
			return
		}

		remaining := cfg.BodyCapBytes - currentLen
		if len(data) > remaining {
			data = data[:remaining]
			detail.Truncated = true
		}

		detail.TranslatedResponseBody = append(detail.TranslatedResponseBody, data...)
	}, false)
}

func (s *RequestDetailStore) UpdateTranslatedResponseBody(requestID string, body []byte) {
	if len(body) == 0 {
		return
	}

	s.mutateDetail(requestID, func(detail *RequestDetail, cfg RequestDetailConfig) {
		detail.TranslatedResponseBody = cloneBodyWithCap(body, cfg.BodyCapBytes)
		detail.MetadataOnly = false
		detail.Truncated = detail.Truncated || len(body) > cfg.BodyCapBytes
	}, false)
}

func (s *RequestDetailStore) mutateDetail(requestID string, fn func(detail *RequestDetail, cfg RequestDetailConfig), allowCreate bool) {
	if s == nil || requestID == "" {
		return
	}

	cfg := GetRequestDetailConfig()
	if !cfg.Enabled {
		return
	}

	var snapshots []*RequestDetail

	s.mu.Lock()
	detail := s.details[requestID]
	if detail == nil {
		if !allowCreate {
			s.mu.Unlock()
			return
		}
		now := time.Now().UTC()
		detail = &RequestDetail{
			RequestID:     requestID,
			CreatedAt:     now,
			LastUpdatedAt: now,
		}
		s.details[requestID] = detail
	}

	oldSize := detail.ApproxBytes
	fn(detail, cfg)
	detail.LastUpdatedAt = time.Now().UTC()
	detail.ApproxBytes = estimateDetailBytes(detail)
	s.currentBytes += detail.ApproxBytes - oldSize
	snapshots = s.enforceBudgetLocked(cfg, requestID)
	s.mu.Unlock()

	s.enqueueSnapshots(cfg, snapshots)
}

// Get retrieves request detail by ID (from memory first, then hot DB, then archive DB).
func (s *RequestDetailStore) Get(requestID string) *RequestDetail {
	s.mu.RLock()
	detail, exists := s.details[requestID]
	if exists {
		copied := copyDetail(detail)
		s.mu.RUnlock()
		return copied
	}
	s.mu.RUnlock()

	if d := s.getFromDB(s.db, s.hotTableName, requestID); d != nil {
		return d
	}
	if s.archiveDB != nil {
		return s.getFromDB(s.archiveDB, s.archiveTableName, requestID)
	}
	return nil
}

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
	detail.MetadataOnly = len(detail.RequestBody) == 0 &&
		len(detail.TranslatedRequestBody) == 0 &&
		len(detail.ResponseBody) == 0 &&
		len(detail.TranslatedResponseBody) == 0
	detail.ApproxBytes = estimateDetailBytes(&detail)

	return &detail
}

func (s *RequestDetailStore) persistLoop() {
	defer s.wg.Done()

	for {
		select {
		case snapshot := <-s.persistQueue:
			if snapshot != nil {
				_ = s.persistToDB(snapshot)
			}
		case <-s.stopChan:
			for {
				select {
				case snapshot := <-s.persistQueue:
					if snapshot != nil {
						_ = s.persistToDB(snapshot)
					}
				default:
					return
				}
			}
		}
	}
}

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
	_, err := s.db.Exec(
		query,
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

	return nil
}

func (s *RequestDetailStore) cleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(DefaultRequestDetailCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopChan:
			return
		}
	}
}

func (s *RequestDetailStore) cleanup() {
	cfg := GetRequestDetailConfig()
	var snapshots []*RequestDetail
	now := time.Now().UTC()

	s.mu.Lock()
	snapshots = s.removeExpiredLocked(now, cfg.PersistEnabled, "")
	s.mu.Unlock()

	s.enqueueSnapshots(cfg, snapshots)
	s.archiveOldDetails(now)
}

func (s *RequestDetailStore) removeExpiredLocked(now time.Time, persistEnabled bool, protectedID string) []*RequestDetail {
	if s.ttl <= 0 {
		return nil
	}

	var snapshots []*RequestDetail
	for id, detail := range s.details {
		if id == protectedID {
			continue
		}
		if now.Sub(detail.LastUpdatedAt) <= s.ttl {
			continue
		}
		snapshot := s.evictDetailLocked(id, persistEnabled)
		if snapshot != nil {
			snapshots = append(snapshots, snapshot)
		}
	}

	return snapshots
}

func (s *RequestDetailStore) enforceBudgetLocked(cfg RequestDetailConfig, protectedID string) []*RequestDetail {
	var snapshots []*RequestDetail
	now := time.Now().UTC()

	snapshots = append(snapshots, s.removeExpiredLocked(now, cfg.PersistEnabled, protectedID)...)

	for len(s.details) > cfg.MaxEntries || s.currentBytes > cfg.MaxMemoryBytes {
		oldestID := s.oldestDetailIDLocked(protectedID)
		if oldestID == "" {
			break
		}
		snapshot := s.evictDetailLocked(oldestID, cfg.PersistEnabled)
		if snapshot != nil {
			snapshots = append(snapshots, snapshot)
		}
	}

	if (len(s.details) > cfg.MaxEntries || s.currentBytes > cfg.MaxMemoryBytes) && protectedID != "" {
		if detail := s.details[protectedID]; detail != nil && !detail.MetadataOnly {
			oldSize := detail.ApproxBytes
			detail.RequestBody = nil
			detail.TranslatedRequestBody = nil
			detail.ResponseBody = nil
			detail.TranslatedResponseBody = nil
			detail.MetadataOnly = true
			detail.Truncated = true
			detail.ApproxBytes = estimateDetailBytes(detail)
			s.currentBytes += detail.ApproxBytes - oldSize
		}
	}

	for len(s.details) > cfg.MaxEntries || s.currentBytes > cfg.MaxMemoryBytes {
		oldestID := s.oldestDetailIDLocked("")
		if oldestID == "" {
			break
		}
		snapshot := s.evictDetailLocked(oldestID, cfg.PersistEnabled)
		if snapshot != nil {
			snapshots = append(snapshots, snapshot)
		}
	}

	if s.currentBytes < 0 {
		s.currentBytes = 0
	}

	return snapshots
}

func (s *RequestDetailStore) collectAndClearLocked(persistEnabled bool) []*RequestDetail {
	snapshots := make([]*RequestDetail, 0, len(s.details))
	for id := range s.details {
		snapshot := s.evictDetailLocked(id, persistEnabled)
		if snapshot != nil {
			snapshots = append(snapshots, snapshot)
		}
	}
	s.currentBytes = 0
	return snapshots
}

func (s *RequestDetailStore) oldestDetailIDLocked(excludeID string) string {
	var oldestID string
	var oldestTime time.Time
	first := true

	for id, detail := range s.details {
		if id == excludeID {
			continue
		}
		if first || detail.LastUpdatedAt.Before(oldestTime) {
			oldestID = id
			oldestTime = detail.LastUpdatedAt
			first = false
		}
	}

	return oldestID
}

func (s *RequestDetailStore) evictDetailLocked(requestID string, persistEnabled bool) *RequestDetail {
	detail := s.details[requestID]
	if detail == nil {
		return nil
	}

	var snapshot *RequestDetail
	if persistEnabled {
		snapshot = copyDetail(detail)
	}

	s.currentBytes -= detail.ApproxBytes
	delete(s.details, requestID)

	return snapshot
}

func (s *RequestDetailStore) enqueueSnapshots(cfg RequestDetailConfig, snapshots []*RequestDetail) {
	if !cfg.PersistEnabled || len(snapshots) == 0 {
		return
	}

	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		select {
		case s.persistQueue <- snapshot:
		default:
			select {
			case <-s.persistQueue:
				log.Warn("request detail store: persist queue full, dropping oldest queued detail")
			default:
			}
			select {
			case s.persistQueue <- snapshot:
			default:
				log.Warn("request detail store: persist queue still full, dropping detail snapshot")
			}
		}
	}
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

	if configuredDays := s.loadArchiveDays(); configuredDays != s.archiveDays {
		s.archiveDays = configuredDays
		log.Infof("request detail store: archive threshold changed to %d days", s.archiveDays)
	}

	s.lastArchiveAt = now
	cutoff := now.AddDate(0, 0, -s.archiveDays).UTC()

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

	var ids []string
	for _, r := range batch {
		var exists int
		verifySQL := fmt.Sprintf(`SELECT 1 FROM %s WHERE request_id = ?`, s.archiveTableName)
		if err := s.archiveDB.QueryRow(verifySQL, r.requestID).Scan(&exists); err != nil {
			log.Warnf("request detail store: archive verify failed for %s, skipping delete: %v", r.requestID, err)
			return
		}
		ids = append(ids, r.requestID)
	}

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

// persistAll persists all entries to database (called on shutdown).
func (s *RequestDetailStore) persistAll() {
	cfg := GetRequestDetailConfig()
	if !cfg.PersistEnabled || s.db == nil {
		return
	}

	s.mu.RLock()
	snapshots := make([]*RequestDetail, 0, len(s.details))
	for _, detail := range s.details {
		snapshots = append(snapshots, copyDetail(detail))
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

// Stop stops the cleanup loop.
func (s *RequestDetailStore) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	s.persistAll()
	if s.ownsArchiveDB && s.archiveDB != nil {
		_ = s.archiveDB.Close()
	}
}

func copyDetail(detail *RequestDetail) *RequestDetail {
	copied := &RequestDetail{
		RequestID:              detail.RequestID,
		CreatedAt:              detail.CreatedAt,
		LastUpdatedAt:          detail.LastUpdatedAt,
		RequestBody:            append([]byte(nil), detail.RequestBody...),
		TranslatedRequestBody:  append([]byte(nil), detail.TranslatedRequestBody...),
		ResponseBody:           append([]byte(nil), detail.ResponseBody...),
		TranslatedResponseBody: append([]byte(nil), detail.TranslatedResponseBody...),
		Persisted:              detail.Persisted,
		MetadataOnly:           detail.MetadataOnly,
		Truncated:              detail.Truncated,
		ApproxBytes:            detail.ApproxBytes,
	}
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

func applyDetailBodyCap(detail *RequestDetail, bodyCap int) {
	detail.RequestBody = cloneBodyWithCap(detail.RequestBody, bodyCap)
	detail.TranslatedRequestBody = cloneBodyWithCap(detail.TranslatedRequestBody, bodyCap)
	detail.ResponseBody = cloneBodyWithCap(detail.ResponseBody, bodyCap)
	if len(detail.TranslatedResponseBody) > bodyCap {
		detail.TranslatedResponseBody = append([]byte(nil), detail.TranslatedResponseBody[:bodyCap]...)
		detail.Truncated = true
	}
	if len(detail.RequestBody) == 0 &&
		len(detail.TranslatedRequestBody) == 0 &&
		len(detail.ResponseBody) == 0 &&
		len(detail.TranslatedResponseBody) == 0 {
		detail.MetadataOnly = true
	}
}

func cloneBodyWithCap(body []byte, bodyCap int) []byte {
	if len(body) == 0 || bodyCap <= 0 {
		return nil
	}
	if len(body) > bodyCap {
		body = body[:bodyCap]
	}
	return append([]byte(nil), body...)
}

func cloneHeaders(headers http.Header) http.Header {
	if headers == nil {
		return nil
	}
	return headers.Clone()
}

func estimateHeaderBytes(headers http.Header) int64 {
	var size int64
	for key, values := range headers {
		size += int64(len(key))
		for _, value := range values {
			size += int64(len(value))
		}
	}
	return size
}

func estimateDetailBytes(detail *RequestDetail) int64 {
	if detail == nil {
		return 0
	}

	size := int64(len(detail.RequestID) + 256)
	size += int64(len(detail.RequestBody))
	size += int64(len(detail.TranslatedRequestBody))
	size += int64(len(detail.ResponseBody))
	size += int64(len(detail.TranslatedResponseBody))
	size += estimateHeaderBytes(detail.RequestHeaders)
	size += estimateHeaderBytes(detail.TranslatedRequestHeaders)
	size += estimateHeaderBytes(detail.ResponseHeaders)

	return size
}

// Helper functions for JSON serialization of headers.
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
