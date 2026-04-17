package database

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MigrationProgress struct {
	Message  string
	Progress int
}

type MigrationParams struct {
	ClearTarget bool
	OnProgress  func(MigrationProgress)
	Source      Options
	Target      Options
	WithArchive bool
}

var migrationTables = []string{
	"groups",
	"users",
	"admin_management_keys",
	"user_amp_settings",
	"user_api_keys",
	"channels",
	"user_groups",
	"channel_groups",
	"channel_models",
	"model_metadata",
	"model_prices",
	"model_price_context_rules",
	"subscription_plans",
	"subscription_plan_limits",
	"user_subscriptions",
	"subscription_entitlements",
	"user_billing_settings",
	"billing_account_state",
	"subscription_window_state",
	"billing_reservations",
	"billing_projection_events",
	"request_logs",
	"global_request_metric_projections",
	"global_request_minute_metrics",
	"request_log_details",
	"announcements",
	"announcement_reads",
	"status_monitors",
	"status_monitor_results",
	"system_config",
	"purchase_products",
	"purchase_orders",
	"purchase_webhook_targets",
	"purchase_webhook_events",
	"purchase_order_payment_status_history",
	"purchase_manual_settlement_batches",
	"purchase_manual_settlement_batch_items",
	"redeem_campaigns",
	"redeem_code_batches",
	"redeem_codes",
	"redeem_user_counters",
	"redeem_redemptions",
	"billing_daily_reset_records",
	"billing_events",
}

func MigrateBetweenDatabases(params MigrationParams) error {
	sourceOptions, err := params.Source.Normalize()
	if err != nil {
		return err
	}
	targetOptions, err := params.Target.Normalize()
	if err != nil {
		return err
	}
	if sourceOptions.Type == targetOptions.Type {
		return fmt.Errorf("migration only supports switching between sqlite and postgres")
	}

	reportMigrationProgress(params.OnProgress, 5, "打开源数据库")
	sourceDB, err := OpenWithOptions(sourceOptions)
	if err != nil {
		return fmt.Errorf("打开源数据库失败: %w", err)
	}
	defer sourceDB.Close()

	reportMigrationProgress(params.OnProgress, 15, "初始化目标数据库结构")
	targetDB, err := prepareStandaloneDatabase(targetOptions)
	if err != nil {
		return fmt.Errorf("初始化目标数据库失败: %w", err)
	}
	defer targetDB.Close()

	if params.ClearTarget {
		reportMigrationProgress(params.OnProgress, 25, "清空目标数据库")
		if err := clearBusinessTablesOnDB(targetDB, targetOptions); err != nil {
			return fmt.Errorf("清空目标数据库失败: %w", err)
		}
	}

	totalSteps := len(migrationTables)
	if params.WithArchive {
		totalSteps++
	}
	baseProgress := 30
	progressSpan := 60

	for index, tableName := range migrationTables {
		progress := baseProgress + (index*progressSpan)/totalSteps
		reportMigrationProgress(params.OnProgress, progress, fmt.Sprintf("迁移表 %s", tableName))
		if err := copyTableBetween(sourceDB, targetDB, tableName, tableName); err != nil {
			return fmt.Errorf("迁移表 %s 失败: %w", tableName, err)
		}
	}

	if params.WithArchive {
		reportMigrationProgress(params.OnProgress, 92, "迁移请求详情归档")
		if err := migrateArchiveDataBetween(sourceOptions, targetOptions, sourceDB, targetDB, params.ClearTarget); err != nil {
			return fmt.Errorf("迁移归档数据失败: %w", err)
		}
	}

	reportMigrationProgress(params.OnProgress, 95, "数据复制完成")
	return nil
}

func reportMigrationProgress(callback func(MigrationProgress), progress int, message string) {
	if callback == nil {
		return
	}
	callback(MigrationProgress{Progress: progress, Message: message})
}

func clearBusinessTablesOnDB(targetDB *sql.DB, targetOptions Options) error {
	allTables := append([]string{}, migrationTables...)
	if targetOptions.Type == DBTypePostgres {
		allTables = append(allTables, "request_log_details_archive")
	}

	if targetOptions.Type == DBTypePostgres {
		truncateSQL := "TRUNCATE TABLE " + strings.Join(allTables, ", ") + " CASCADE"
		_, err := targetDB.Exec(truncateSQL)
		return err
	}

	if _, err := targetDB.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	defer targetDB.Exec("PRAGMA foreign_keys = ON")

	for index := len(allTables) - 1; index >= 0; index-- {
		if _, err := targetDB.Exec("DELETE FROM " + allTables[index]); err != nil {
			return err
		}
	}

	archivePath := sqliteArchivePath(targetOptions.SQLitePath)
	if _, err := os.Stat(archivePath); err == nil {
		_ = os.Remove(archivePath)
	}

	return nil
}

func copyTableBetween(sourceDB, targetDB *sql.DB, sourceTable, targetTable string) error {
	sourceColumns, err := tableColumnsOnDB(sourceDB, sourceTable)
	if err != nil {
		return err
	}
	targetColumns, err := tableColumnsOnDB(targetDB, targetTable)
	if err != nil {
		return err
	}
	columns := intersectColumns(sourceColumns, targetColumns)
	if len(columns) == 0 {
		return nil
	}

	selectSQL := fmt.Sprintf("SELECT %s FROM %s", strings.Join(columns, ", "), sourceTable)
	rows, err := sourceDB.Query(selectSQL)
	if err != nil {
		return err
	}
	defer rows.Close()

	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", targetTable, strings.Join(columns, ", "), placeholderListForType(len(columns), detectDBTypeFromHandle(targetDB)))
	transaction, err := targetDB.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()

	statement, err := transaction.Prepare(insertSQL)
	if err != nil {
		return err
	}
	defer statement.Close()

	values := make([]any, len(columns))
	scanTargets := make([]any, len(columns))
	for index := range values {
		scanTargets[index] = &values[index]
	}

	for rows.Next() {
		if err := rows.Scan(scanTargets...); err != nil {
			return err
		}
		if _, err := statement.Exec(normalizeMigrationRow(values)...); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return transaction.Commit()
}

func tableColumnsOnDB(db *sql.DB, tableName string) ([]string, error) {
	rows, err := db.Query("SELECT * FROM " + tableName + " LIMIT 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rows.Columns()
}

func tableExistsOnDB(db *sql.DB, tableName string) (bool, error) {
	_, err := tableColumnsOnDB(db, tableName)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "does not exist") || strings.Contains(message, "no such table") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func intersectColumns(sourceColumns, targetColumns []string) []string {
	targetSet := make(map[string]struct{}, len(targetColumns))
	for _, column := range targetColumns {
		targetSet[column] = struct{}{}
	}

	commonColumns := make([]string, 0, len(sourceColumns))
	for _, column := range sourceColumns {
		if _, exists := targetSet[column]; exists {
			commonColumns = append(commonColumns, column)
		}
	}
	return commonColumns
}

func normalizeMigrationRow(values []any) []any {
	normalized := make([]any, len(values))
	for index, value := range values {
		switch typed := value.(type) {
		case []byte:
			normalized[index] = sanitizeTextForPostgres(typed)
		case string:
			normalized[index] = sanitizeTextForPostgres([]byte(typed))
		default:
			normalized[index] = typed
		}
	}
	return normalized
}

func sanitizeTextForPostgres(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	cleaned := bytes.ToValidUTF8(raw, []byte("\uFFFD"))
	cleaned = bytes.ReplaceAll(cleaned, []byte{0}, []byte{})
	return string(cleaned)
}

func migrateArchiveDataBetween(sourceOptions, targetOptions Options, sourceMainDB, targetMainDB *sql.DB, clearTarget bool) error {
	sourceArchiveDB, sourceArchiveTable, closeSourceArchive, err := openArchiveSourceOnDB(sourceOptions, sourceMainDB)
	if err != nil {
		return err
	}
	if closeSourceArchive != nil {
		defer closeSourceArchive()
	}
	if sourceArchiveDB == nil {
		return nil
	}

	targetArchiveDB, targetArchiveTable, closeTargetArchive, err := openArchiveTargetOnDB(targetOptions, targetMainDB, clearTarget)
	if err != nil {
		return err
	}
	if closeTargetArchive != nil {
		defer closeTargetArchive()
	}

	return copyTableBetween(sourceArchiveDB, targetArchiveDB, sourceArchiveTable, targetArchiveTable)
}

func openArchiveSourceOnDB(options Options, mainDB *sql.DB) (*sql.DB, string, func() error, error) {
	if options.Type == DBTypePostgres {
		exists, err := tableExistsOnDB(mainDB, "request_log_details_archive")
		if err != nil {
			return nil, "", nil, err
		}
		if !exists {
			return nil, "", nil, nil
		}
		return mainDB, "request_log_details_archive", nil, nil
	}

	archivePath := sqliteArchivePath(options.SQLitePath)
	if _, err := os.Stat(archivePath); err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil, nil
		}
		return nil, "", nil, err
	}

	dsn := archivePath + "?_pragma=foreign_keys(OFF)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	archiveDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, "", nil, err
	}
	if err := archiveDB.Ping(); err != nil {
		archiveDB.Close()
		return nil, "", nil, err
	}
	return archiveDB, "request_log_details", archiveDB.Close, nil
}

func openArchiveTargetOnDB(options Options, mainDB *sql.DB, clearTarget bool) (*sql.DB, string, func() error, error) {
	if options.Type == DBTypePostgres {
		if clearTarget {
			if _, err := mainDB.Exec("DELETE FROM request_log_details_archive"); err != nil {
				return nil, "", nil, err
			}
		}
		return mainDB, "request_log_details_archive", nil, nil
	}

	archivePath := sqliteArchivePath(options.SQLitePath)
	dir := filepath.Dir(archivePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, "", nil, err
		}
	}

	dsn := archivePath + "?_pragma=foreign_keys(OFF)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	archiveDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, "", nil, err
	}
	if err := archiveDB.Ping(); err != nil {
		archiveDB.Close()
		return nil, "", nil, err
	}

	if _, err := archiveDB.Exec(`
		CREATE TABLE IF NOT EXISTS request_log_details (
			request_id TEXT PRIMARY KEY,
			request_headers TEXT,
			request_body TEXT,
			translated_request_body TEXT,
			response_headers TEXT,
			response_body TEXT,
			translated_response_body TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_archive_details_created ON request_log_details(created_at DESC);
	`); err != nil {
		archiveDB.Close()
		return nil, "", nil, err
	}

	if clearTarget {
		if _, err := archiveDB.Exec("DELETE FROM request_log_details"); err != nil {
			archiveDB.Close()
			return nil, "", nil, err
		}
	}

	return archiveDB, "request_log_details", archiveDB.Close, nil
}

func sqliteArchivePath(sqlitePath string) string {
	return filepath.Join(filepath.Dir(sqlitePath), "data_details_archive.db")
}

func placeholderListForType(count int, currentType DBType) string {
	placeholders := make([]string, count)
	for index := 0; index < count; index++ {
		if currentType == DBTypePostgres {
			placeholders[index] = fmt.Sprintf("$%d", index+1)
		} else {
			placeholders[index] = "?"
		}
	}
	return strings.Join(placeholders, ",")
}

func detectDBTypeFromHandle(db *sql.DB) DBType {
	if err := db.Ping(); err != nil {
		return DBTypeSQLite
	}
	rows, err := db.Query("SELECT version()")
	if err == nil {
		rows.Close()
		return DBTypePostgres
	}
	return DBTypeSQLite
}
