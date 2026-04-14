package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ampmanager/internal/amp"
	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"
	"ampmanager/internal/translator/filters"

	"github.com/gin-gonic/gin"
)

// backupFilenamePattern 备份文件名正则：data.db.backup.YYYYMMDDHHmmss (14位时间戳)
var backupFilenamePattern = regexp.MustCompile(`^data\.db\.backup\.\d{14}$`)

const retryConfigKey = "retry_config"
const timeoutConfigKey = "timeout_config"
const cacheTTLConfigKey = "cache_ttl_override"

type SystemHandler struct {
	configRepo *repository.SystemConfigRepository
}

func ensureSQLiteDatabaseFiles(c *gin.Context) bool {
	if database.SupportsFileBackups() {
		return true
	}

	c.JSON(http.StatusBadRequest, gin.H{
		"error": "当前使用的是 PostgreSQL，文件级数据库备份/恢复不可用，请改用 dbtool 迁移命令",
	})
	return false
}

func NewSystemHandler() *SystemHandler {
	return &SystemHandler{
		configRepo: repository.NewSystemConfigRepository(),
	}
}

func (h *SystemHandler) GetPublicSiteConfig(c *gin.Context) {
	cfg, err := service.NewSystemConfigService().GetSiteConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取站点配置失败"})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

func (h *SystemHandler) GetSiteConfig(c *gin.Context) {
	cfg, err := service.NewSystemConfigService().GetSiteConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取站点配置失败"})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

func (h *SystemHandler) UpdateSiteConfig(c *gin.Context) {
	var req model.SiteConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	if len(strings.TrimSpace(req.SiteName)) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "网站名称长度不能超过 64 个字符"})
		return
	}

	cfg, err := service.NewSystemConfigService().SetSiteConfig(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存站点配置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "站点配置已更新", "config": cfg})
}

func maskDatabaseURL(raw string) string {
	if raw == "" {
		return ""
	}

	re := regexp.MustCompile(`://([^:/?#]+):([^@/?#]+)@`)
	return re.ReplaceAllString(raw, `://$1:******@`)
}

func (h *SystemHandler) GetDatabaseInfo(c *gin.Context) {
	options := database.GetOptions()
	sqlitePath := options.SQLitePath
	if sqlitePath == "" {
		sqlitePath = "./data/data.db"
	}
	databaseURL := options.DatabaseURL

	archiveMode := "独立 SQLite 归档库"
	if database.IsPostgres() {
		archiveMode = "同库归档表 request_log_details_archive"
	}

	c.JSON(http.StatusOK, gin.H{
		"currentType":         database.GetType(),
		"supportsFileBackups": database.SupportsFileBackups(),
		"sqlitePath":          sqlitePath,
		"databaseURL":         databaseURL,
		"databaseURLMasked":   maskDatabaseURL(databaseURL),
		"archiveMode":         archiveMode,
	})
}

func (h *SystemHandler) StartDatabaseMigration(c *gin.Context) {
	var req struct {
		ClearTarget       bool   `json:"clearTarget"`
		TargetDatabaseURL string `json:"targetDatabaseUrl"`
		TargetSQLitePath  string `json:"targetSqlitePath"`
		TargetType        string `json:"targetType"`
		WithArchive       bool   `json:"withArchive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	sourceOptions := database.GetOptions()
	targetOptions := database.Options{
		Type:        database.DBType(req.TargetType),
		DatabaseURL: strings.TrimSpace(req.TargetDatabaseURL),
		SQLitePath:  strings.TrimSpace(req.TargetSQLitePath),
	}
	normalizedTarget, err := targetOptions.Normalize()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if sourceOptions.Type == normalizedTarget.Type {
		c.JSON(http.StatusBadRequest, gin.H{"error": "当前只支持 SQLite 与 PostgreSQL 之间互相切换"})
		return
	}

	task, err := globalDatabaseTaskManager.createTask("database_migration", sourceOptions.Type, normalizedTarget.Type)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	go runDatabaseMigrationTask(task.ID, sourceOptions, normalizedTarget, req.ClearTarget, req.WithArchive)
	c.JSON(http.StatusAccepted, task)
}

func (h *SystemHandler) GetDatabaseMigrationTask(c *gin.Context) {
	taskID := c.Param("taskID")
	task, exists := globalDatabaseTaskManager.getTask(taskID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	c.JSON(http.StatusOK, task)
}

// GetRetryConfig 获取重试配置
func (h *SystemHandler) GetRetryConfig(c *gin.Context) {
	value, err := h.configRepo.Get(retryConfigKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	// 如果没有配置，返回默认值
	if value == "" {
		defaultCfg := amp.DefaultRetryConfig()
		c.JSON(http.StatusOK, model.RetryConfigResponse{
			Enabled:           defaultCfg.Enabled,
			MaxAttempts:       defaultCfg.MaxAttempts,
			GateTimeoutMs:     defaultCfg.GateTimeout.Milliseconds(),
			MaxBodyBytes:      defaultCfg.MaxBodyBytes,
			BackoffBaseMs:     defaultCfg.BackoffBase.Milliseconds(),
			BackoffMaxMs:      defaultCfg.BackoffMax.Milliseconds(),
			RetryOn429:        defaultCfg.RetryOn429,
			RetryOn5xx:        defaultCfg.RetryOn5xx,
			RespectRetryAfter: defaultCfg.RespectRetryAfter,
			RetryOnEmptyBody:  defaultCfg.RetryOnEmptyBody,
		})
		return
	}

	var resp model.RetryConfigResponse
	if err := json.Unmarshal([]byte(value), &resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解析配置失败"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// UpdateRetryConfig 更新重试配置
func (h *SystemHandler) UpdateRetryConfig(c *gin.Context) {
	var req model.RetryConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	// 验证参数（移除上限，仅保留必要的下限和溢出保护）
	if req.MaxAttempts < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "maxAttempts 必须 >= 1"})
		return
	}
	if req.GateTimeoutMs < 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateTimeoutMs 必须 >= 1000"})
		return
	}

	const maxDuration = time.Duration(1<<63 - 1)
	maxMs := int64(maxDuration / time.Millisecond)
	if req.GateTimeoutMs > maxMs || req.BackoffBaseMs > maxMs || req.BackoffMaxMs > maxMs {
		c.JSON(http.StatusBadRequest, gin.H{"error": "重试时间参数过大，超出可表示范围"})
		return
	}

	// 保存到数据库
	resp := model.RetryConfigResponse{
		Enabled:           req.Enabled,
		MaxAttempts:       req.MaxAttempts,
		GateTimeoutMs:     req.GateTimeoutMs,
		MaxBodyBytes:      req.MaxBodyBytes,
		BackoffBaseMs:     req.BackoffBaseMs,
		BackoffMaxMs:      req.BackoffMaxMs,
		RetryOn429:        req.RetryOn429,
		RetryOn5xx:        req.RetryOn5xx,
		RespectRetryAfter: req.RespectRetryAfter,
		RetryOnEmptyBody:  req.RetryOnEmptyBody,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化配置失败"})
		return
	}

	if err := h.configRepo.Set(retryConfigKey, string(data)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败"})
		return
	}

	// 更新运行时配置
	rt := amp.GetRetryTransport()
	if rt != nil {
		rt.UpdateConfig(&amp.RetryConfig{
			Enabled:           req.Enabled,
			MaxAttempts:       req.MaxAttempts,
			GateTimeout:       time.Duration(req.GateTimeoutMs) * time.Millisecond,
			MaxBodyBytes:      req.MaxBodyBytes,
			BackoffBase:       time.Duration(req.BackoffBaseMs) * time.Millisecond,
			BackoffMax:        time.Duration(req.BackoffMaxMs) * time.Millisecond,
			RetryOn429:        req.RetryOn429,
			RetryOn5xx:        req.RetryOn5xx,
			RespectRetryAfter: req.RespectRetryAfter,
			RetryOnEmptyBody:  req.RetryOnEmptyBody,
		})
	}

	c.JSON(http.StatusOK, gin.H{"message": "配置已更新", "config": resp})
}

func (h *SystemHandler) GetBillingRuntimeConfig(c *gin.Context) {
	resp, err := service.NewSystemConfigService().GetBillingRuntimeConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SystemHandler) UpdateBillingRuntimeConfig(c *gin.Context) {
	var req model.BillingRuntimeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	req.RedisURL = strings.TrimSpace(req.RedisURL)
	req.RedisPrefix = strings.TrimSpace(req.RedisPrefix)
	if req.RedisPrefix == "" {
		req.RedisPrefix = "ampmanager"
	}
	if req.ReservationTTLSec < 60 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reservationTtlSec 必须 >= 60"})
		return
	}
	if req.ReconcileIntervalSec < 10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reconcileIntervalSec 必须 >= 10"})
		return
	}
	if req.StreamBatchSize < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "streamBatchSize 必须 >= 1"})
		return
	}
	if req.ReconcileBatchSize <= 0 {
		req.ReconcileBatchSize = req.StreamBatchSize
	}
	if req.ExpiryBatchSize <= 0 {
		req.ExpiryBatchSize = req.StreamBatchSize
	}
	if req.ReconcileBatchSize < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reconcileBatchSize 必须 >= 1"})
		return
	}
	if req.ExpiryBatchSize < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expiryBatchSize 必须 >= 1"})
		return
	}
	if req.ProjectorWorkers == 0 {
		req.ProjectorWorkers = 1
	}
	if req.ProjectorWorkers < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "projectorWorkers 必须 >= 1"})
		return
	}

	cfgService := service.NewSystemConfigService()
	if err := cfgService.ApplyAndStoreBillingRuntimeConfig(req); err != nil {
		if service.IsBillingRuntimePersistError(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败，仍使用旧运行时: " + err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Redis 配置应用失败，配置未保存且仍使用旧运行时: " + err.Error()})
		return
	}

	resp, err := cfgService.GetBillingRuntimeConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "配置已更新", "config": resp})
}

func (h *SystemHandler) UploadDatabase(c *gin.Context) {
	if database.IsPostgres() {
		h.uploadPostgresDump(c)
		return
	}

	if !ensureSQLiteDatabaseFiles(c) {
		return
	}

	file, err := c.FormFile("database")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择数据库文件"})
		return
	}

	if filepath.Ext(file.Filename) != ".db" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只支持 .db 文件"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取上传文件"})
		return
	}
	defer src.Close()

	dbPath := "./data/data.db"
	walPath := dbPath + "-wal"
	shmPath := dbPath + "-shm"
	backupPath := "./data/data.db.backup." + time.Now().Format("20060102150405")

	// 关闭数据库连接，释放文件句柄（Windows 必须先关闭才能操作文件）
	if err := database.CloseAndRelease(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "关闭数据库连接失败: " + err.Error()})
		return
	}

	// 后续操作如果失败，必须重新打开数据库
	reopenOnError := func() {
		if err := database.Reopen(); err != nil {
			// 记录严重错误，但此时无法通过日志系统记录（因为依赖数据库）
			_ = err
		}
	}

	// 备份当前数据库
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Rename(dbPath, backupPath); err != nil {
			reopenOnError()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "备份现有数据库失败: " + err.Error()})
			return
		}
	}

	// 删除 WAL 和 SHM 文件
	os.Remove(walPath)
	os.Remove(shmPath)

	dst, err := os.Create(dbPath)
	if err != nil {
		os.Rename(backupPath, dbPath)
		reopenOnError()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建数据库文件失败"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(dbPath)
		os.Rename(backupPath, dbPath)
		reopenOnError()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存数据库文件失败"})
		return
	}
	dst.Close()

	// 重新打开新数据库
	if err := database.Reopen(); err != nil {
		// 回滚：恢复备份
		os.Remove(dbPath)
		os.Rename(backupPath, dbPath)
		database.Reopen()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "新数据库打开失败，已回滚: " + err.Error()})
		return
	}

	// 重新初始化依赖数据库和系统配置的运行时组件
	if err := reinitDatabaseBackedRuntimeServices(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "运行时服务重载失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "数据库上传并切换成功",
		"backupFile": filepath.Base(backupPath),
	})
}

func (h *SystemHandler) uploadPostgresDump(c *gin.Context) {
	file, err := c.FormFile("database")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择 PostgreSQL dump 文件"})
		return
	}

	if ext := strings.ToLower(filepath.Ext(file.Filename)); ext != ".sql" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "PostgreSQL 仅支持上传 .sql dump 文件"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取上传文件"})
		return
	}
	defer src.Close()

	dumpContent, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 dump 文件失败"})
		return
	}

	currentOptions := database.GetOptions()
	if err := database.CloseAndRelease(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "关闭数据库连接失败: " + err.Error()})
		return
	}

	reopenOnError := func() {
		_ = database.InitWithOptions(currentOptions)
		_ = reinitDatabaseBackedRuntimeServices()
	}

	if err := database.RestorePostgresDatabase(context.Background(), currentOptions, bytes.ToValidUTF8(dumpContent, []byte(""))); err != nil {
		reopenOnError()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "导入 PostgreSQL dump 失败: " + err.Error()})
		return
	}

	if err := database.InitWithOptions(currentOptions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重新连接 PostgreSQL 失败: " + err.Error()})
		return
	}
	if err := reinitDatabaseBackedRuntimeServices(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "运行时服务重载失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "PostgreSQL dump 导入成功"})
}

func (h *SystemHandler) DownloadDatabase(c *gin.Context) {
	if database.IsPostgres() {
		h.downloadPostgresDump(c)
		return
	}

	if !ensureSQLiteDatabaseFiles(c) {
		return
	}

	dbPath := "./data/data.db"

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "数据库文件不存在"})
		return
	}

	// 执行 checkpoint 确保所有 WAL 数据写入主数据库文件
	db := database.GetDB()
	if db != nil && database.IsSQLite() {
		_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	}

	filename := "ampmanager_" + time.Now().Format("20060102150405") + ".db"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/octet-stream")
	c.File(dbPath)
}

func (h *SystemHandler) downloadPostgresDump(c *gin.Context) {
	dumpContent, err := database.DumpPostgresDatabase(context.Background(), database.GetOptions())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "导出 PostgreSQL dump 失败: " + err.Error()})
		return
	}

	filename := "ampmanager_" + time.Now().Format("20060102150405") + ".sql"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "application/sql", dumpContent)
}

func (h *SystemHandler) ListBackups(c *gin.Context) {
	if !ensureSQLiteDatabaseFiles(c) {
		return
	}

	files, err := filepath.Glob("./data/data.db.backup.*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取备份列表失败"})
		return
	}

	backups := make([]gin.H, 0)
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		backups = append(backups, gin.H{
			"filename": filepath.Base(f),
			"size":     info.Size(),
			"modTime":  info.ModTime(),
		})
	}

	c.JSON(http.StatusOK, backups)
}

func (h *SystemHandler) RestoreBackup(c *gin.Context) {
	if !ensureSQLiteDatabaseFiles(c) {
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	// 严格校验文件名格式，防止路径穿越
	if !backupFilenamePattern.MatchString(req.Filename) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的备份文件名"})
		return
	}

	backupPath := filepath.Clean(filepath.Join("./data", req.Filename))
	// 二次验证路径仍在 data 目录内
	absBackupPath, _ := filepath.Abs(backupPath)
	absDataDir, _ := filepath.Abs("./data")
	if !strings.HasPrefix(absBackupPath, absDataDir+string(filepath.Separator)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的备份路径"})
		return
	}

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "备份文件不存在"})
		return
	}

	dbPath := "./data/data.db"
	walPath := dbPath + "-wal"
	shmPath := dbPath + "-shm"
	currentBackup := "./data/data.db.backup." + time.Now().Format("20060102150405")

	// 关闭数据库连接，释放文件句柄
	if err := database.CloseAndRelease(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "关闭数据库连接失败: " + err.Error()})
		return
	}

	reopenOnError := func() {
		_ = database.Reopen()
	}

	// 备份当前数据库
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Rename(dbPath, currentBackup); err != nil {
			reopenOnError()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "备份当前数据库失败"})
			return
		}
	}

	// 删除 WAL 和 SHM 文件（关键！避免旧的 WAL 覆盖还原的数据）
	os.Remove(walPath)
	os.Remove(shmPath)

	srcFile, err := os.Open(backupPath)
	if err != nil {
		os.Rename(currentBackup, dbPath)
		reopenOnError()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取备份文件失败"})
		return
	}
	defer srcFile.Close()

	dst, err := os.Create(dbPath)
	if err != nil {
		os.Rename(currentBackup, dbPath)
		reopenOnError()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建数据库文件失败"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, srcFile); err != nil {
		dst.Close()
		os.Remove(dbPath)
		os.Rename(currentBackup, dbPath)
		reopenOnError()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "恢复数据库失败"})
		return
	}
	dst.Close()

	// 重新打开新数据库
	if err := database.Reopen(); err != nil {
		os.Remove(dbPath)
		os.Rename(currentBackup, dbPath)
		database.Reopen()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "恢复后数据库打开失败，已回滚: " + err.Error()})
		return
	}

	// 重新初始化依赖数据库和系统配置的运行时组件
	if err := reinitDatabaseBackedRuntimeServices(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "运行时服务重载失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "数据库恢复并切换成功"})
}

func (h *SystemHandler) DeleteBackup(c *gin.Context) {
	if !ensureSQLiteDatabaseFiles(c) {
		return
	}

	filename := c.Param("filename")

	// 严格校验文件名格式，防止路径穿越
	if !backupFilenamePattern.MatchString(filename) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的备份文件名"})
		return
	}

	backupPath := filepath.Clean(filepath.Join("./data", filename))
	// 二次验证路径仍在 data 目录内
	absBackupPath, _ := filepath.Abs(backupPath)
	absDataDir, _ := filepath.Abs("./data")
	if !strings.HasPrefix(absBackupPath, absDataDir+string(filepath.Separator)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的备份路径"})
		return
	}

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "备份文件不存在"})
		return
	}

	if err := os.Remove(backupPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除备份失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "备份已删除"})
}

// GetRequestDetailEnabled 获取请求详情监控状态
func (h *SystemHandler) GetRequestDetailEnabled(c *gin.Context) {
	cfg, err := service.NewSystemConfigService().GetRequestDetailConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": cfg.Enabled})
}

// UpdateRequestDetailEnabled 更新请求详情监控状态
func (h *SystemHandler) UpdateRequestDetailEnabled(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	cfgService := service.NewSystemConfigService()
	if err := cfgService.SetRequestDetailEnabled(req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败"})
		return
	}
	amp.SetRequestDetailEnabled(req.Enabled)

	c.JSON(http.StatusOK, gin.H{"message": "配置已更新", "enabled": req.Enabled})
}

// GetRequestDetailConfig 获取请求详情完整配置
func (h *SystemHandler) GetRequestDetailConfig(c *gin.Context) {
	cfg, err := service.NewSystemConfigService().GetRequestDetailConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// UpdateRequestDetailConfig 更新请求详情完整配置
func (h *SystemHandler) UpdateRequestDetailConfig(c *gin.Context) {
	var req model.RequestDetailConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	if req.TTLSec < 30 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ttlSec 必须 >= 30"})
		return
	}
	if req.MaxEntries < 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "maxEntries 必须 >= 50"})
		return
	}
	if req.MaxMemoryMB < 16 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "maxMemoryMB 必须 >= 16"})
		return
	}
	if req.BodyCapKB < 4 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bodyCapKB 必须 >= 4"})
		return
	}
	switch req.HighRPMMode {
	case amp.RequestDetailModeFull, amp.RequestDetailModeOff, amp.RequestDetailModeSample:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "highRpmMode 必须是 full/off/sample"})
		return
	}
	if req.HighRPMThreshold < 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "highRpmThreshold 必须 >= 100"})
		return
	}
	if req.HighRPMSamplePercent < 1 || req.HighRPMSamplePercent > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "highRpmSamplePercent 必须在 1-100"})
		return
	}

	resp := model.RequestDetailConfigResponse{
		Enabled:              req.Enabled,
		TTLSec:               req.TTLSec,
		MaxEntries:           req.MaxEntries,
		MaxMemoryMB:          req.MaxMemoryMB,
		BodyCapKB:            req.BodyCapKB,
		PersistEnabled:       req.PersistEnabled,
		HighRPMMode:          req.HighRPMMode,
		HighRPMThreshold:     req.HighRPMThreshold,
		HighRPMSamplePercent: req.HighRPMSamplePercent,
	}

	if err := service.NewSystemConfigService().SetRequestDetailConfig(resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败"})
		return
	}
	amp.UpdateRequestDetailConfig(amp.RequestDetailConfig{
		Enabled:              resp.Enabled,
		TTL:                  time.Duration(resp.TTLSec) * time.Second,
		MaxEntries:           resp.MaxEntries,
		MaxMemoryBytes:       resp.MaxMemoryMB * 1024 * 1024,
		BodyCapBytes:         resp.BodyCapKB * 1024,
		PersistEnabled:       resp.PersistEnabled,
		HighRPMMode:          resp.HighRPMMode,
		HighRPMThreshold:     resp.HighRPMThreshold,
		HighRPMSamplePercent: resp.HighRPMSamplePercent,
	})

	c.JSON(http.StatusOK, gin.H{"message": "配置已更新", "config": resp})
}

// GetTimeoutConfig 获取超时配置
func (h *SystemHandler) GetTimeoutConfig(c *gin.Context) {
	value, err := h.configRepo.Get(timeoutConfigKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	// 如果没有配置，返回默认值
	if value == "" {
		c.JSON(http.StatusOK, model.TimeoutConfigResponse{
			IdleConnTimeoutSec:     300,
			ReadIdleTimeoutSec:     300,
			KeepAliveIntervalSec:   15,
			DialTimeoutSec:         30,
			TLSHandshakeTimeoutSec: 15,
		})
		return
	}

	var resp model.TimeoutConfigResponse
	if err := json.Unmarshal([]byte(value), &resp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解析配置失败"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// UpdateTimeoutConfig 更新超时配置
func (h *SystemHandler) UpdateTimeoutConfig(c *gin.Context) {
	var req model.TimeoutConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	// 验证参数下限（单位：秒），移除上限（仅保留溢出保护）
	if req.IdleConnTimeoutSec < 30 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "idleConnTimeoutSec 必须 >= 30"})
		return
	}
	if req.ReadIdleTimeoutSec < 60 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "readIdleTimeoutSec 必须 >= 60"})
		return
	}
	if req.KeepAliveIntervalSec < 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keepAliveIntervalSec 必须 >= 5"})
		return
	}
	if req.DialTimeoutSec < 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dialTimeoutSec 必须 >= 5"})
		return
	}
	if req.TLSHandshakeTimeoutSec < 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tlsHandshakeTimeoutSec 必须 >= 5"})
		return
	}

	const maxDuration = time.Duration(1<<63 - 1)
	maxSec := int64(maxDuration / time.Second)
	if int64(req.IdleConnTimeoutSec) > maxSec || int64(req.ReadIdleTimeoutSec) > maxSec ||
		int64(req.KeepAliveIntervalSec) > maxSec || int64(req.DialTimeoutSec) > maxSec ||
		int64(req.TLSHandshakeTimeoutSec) > maxSec {
		c.JSON(http.StatusBadRequest, gin.H{"error": "超时参数过大，超出可表示范围"})
		return
	}

	// 保存到数据库
	resp := model.TimeoutConfigResponse{
		IdleConnTimeoutSec:     req.IdleConnTimeoutSec,
		ReadIdleTimeoutSec:     req.ReadIdleTimeoutSec,
		KeepAliveIntervalSec:   req.KeepAliveIntervalSec,
		DialTimeoutSec:         req.DialTimeoutSec,
		TLSHandshakeTimeoutSec: req.TLSHandshakeTimeoutSec,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化配置失败"})
		return
	}

	if err := h.configRepo.Set(timeoutConfigKey, string(data)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败"})
		return
	}

	// 更新运行时配置
	amp.UpdateTimeoutConfig(
		time.Duration(req.IdleConnTimeoutSec)*time.Second,
		time.Duration(req.ReadIdleTimeoutSec)*time.Second,
		time.Duration(req.KeepAliveIntervalSec)*time.Second,
		time.Duration(req.DialTimeoutSec)*time.Second,
		time.Duration(req.TLSHandshakeTimeoutSec)*time.Second,
	)

	c.JSON(http.StatusOK, gin.H{"message": "配置已更新", "config": resp})
}

// GetCacheTTLConfig 获取缓存 TTL 配置
func (h *SystemHandler) GetCacheTTLConfig(c *gin.Context) {
	value, _ := h.configRepo.Get(cacheTTLConfigKey)
	if value == "" {
		value = "1h"
	}
	c.JSON(http.StatusOK, gin.H{"cacheTTL": value})
}

// UpdateCacheTTLConfig 更新缓存 TTL 配置
func (h *SystemHandler) UpdateCacheTTLConfig(c *gin.Context) {
	var req struct {
		CacheTTL string `json:"cacheTTL"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	switch req.CacheTTL {
	case "1h", "5m", "":
		// valid
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "cacheTTL 只支持 \"1h\"、\"5m\" 或 \"\"（不覆盖）"})
		return
	}

	if err := h.configRepo.Set(cacheTTLConfigKey, req.CacheTTL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败"})
		return
	}

	filters.SetCacheTTLOverride(req.CacheTTL)

	c.JSON(http.StatusOK, gin.H{"message": "配置已更新", "cacheTTL": req.CacheTTL})
}
