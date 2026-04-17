package main

import (
	"log"
	"os"
	"time"

	"ampmanager/internal/amp"
	"ampmanager/internal/billing"
	"ampmanager/internal/billingstate"
	"ampmanager/internal/config"
	"ampmanager/internal/database"
	"ampmanager/internal/middleware"
	"ampmanager/internal/realtime"
	"ampmanager/internal/repository"
	"ampmanager/internal/router"
	"ampmanager/internal/service"
	"ampmanager/internal/translator"
	"ampmanager/internal/translator/filters"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	gin.SetMode(gin.ReleaseMode)

	// 显式初始化 translator registry 和 filters
	_ = translator.DefaultRegistry()
	filters.RegisterFilters()

	cfg := config.Load()

	if err := config.ValidateSecurityConfig(cfg); err != nil {
		log.Fatalf("Security check failed: %v", err)
	}

	if err := database.InitWithOptions(cfg.DatabaseOptions()); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer database.Close()

	sysConfigService := service.NewSystemConfigService()
	sysConfigService.ReloadBillingRuntimeFromSystemConfigBestEffort("server startup")
	middleware.LoadUserPanelRateLimitConfigBestEffort("server startup")
	defer billingstate.Close()

	// 初始化日志写入器
	amp.InitLogWriter(database.GetDB())
	defer amp.StopLogWriter()

	// 初始化请求详情存储器
	amp.InitRequestDetailStore(database.GetDB())
	defer amp.StopRequestDetailStore()

	// 初始化计费服务
	billing.InitPriceStore()
	defer billing.StopPriceStore()
	billing.InitCostCalculator()

	// 初始化 pending 请求清理器
	amp.InitPendingCleaner(database.GetDB())
	defer amp.StopPendingCleaner()

	service.InitStatusMonitorScheduler()
	defer service.StopStatusMonitorScheduler()

	// 初始化实时推送 hub
	logRepo := repository.NewRequestLogRepository()
	realtime.InitHub(func(id string) (interface{}, error) {
		return logRepo.GetByIDWithJoins(id)
	})

	userService := service.NewUserService()
	if err := userService.EnsureAdmin(); err != nil {
		log.Printf("警告: 管理员账户创建失败: %v", err)
	}

	r := router.Setup()

	// 加载重试配置
	if configJSON, err := sysConfigService.GetRetryConfigJSON(); err == nil && configJSON != "" {
		amp.InitRetryTransportConfig(configJSON)
	}

	// 加载超时配置
	if configJSON, err := sysConfigService.GetTimeoutConfigJSON(); err == nil && configJSON != "" {
		amp.InitTimeoutConfig(configJSON)
	}

	// 加载请求详情监控配置
	if requestDetailCfg, err := sysConfigService.GetRequestDetailConfig(); err == nil {
		amp.UpdateRequestDetailConfig(amp.RequestDetailConfig{
			Enabled:              requestDetailCfg.Enabled,
			TTL:                  time.Duration(requestDetailCfg.TTLSec) * time.Second,
			MaxEntries:           requestDetailCfg.MaxEntries,
			MaxMemoryBytes:       requestDetailCfg.MaxMemoryMB * 1024 * 1024,
			BodyCapBytes:         requestDetailCfg.BodyCapKB * 1024,
			PersistEnabled:       requestDetailCfg.PersistEnabled,
			HighRPMMode:          requestDetailCfg.HighRPMMode,
			HighRPMThreshold:     requestDetailCfg.HighRPMThreshold,
			HighRPMSamplePercent: requestDetailCfg.HighRPMSamplePercent,
		})
	}

	if payloadLimitCfg, err := sysConfigService.GetRequestPayloadLimit(); err == nil {
		amp.UpdateRequestPayloadLimitBytes(payloadLimitCfg.MaxBytes)
	}

	// 加载缓存 TTL 配置
	if cacheTTL, err := sysConfigService.GetCacheTTLOverride(); err == nil && cacheTTL != "" {
		filters.SetCacheTTLOverride(cacheTTL)
	}

	port := cfg.ServerPort
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	log.Printf("服务器启动在 http://0.0.0.0:%s", port)
	if err := r.Run("0.0.0.0:" + port); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
