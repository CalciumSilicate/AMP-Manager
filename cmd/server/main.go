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
	"ampmanager/internal/invalidation"
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
	defer billingstate.Close()

	role := cfg.Role()
	sysConfigService := service.NewSystemConfigService()
	middleware.LoadUserPanelRateLimitConfigBestEffort("server startup")
	service.RefreshStatsLocationCache()

	startRuntimeSubscriptions(role, sysConfigService)

	if role.RunsProxy() {
		if err := service.NewErrorRuleService().SyncDefaultErrorRules(); err != nil {
			log.Printf("warning: error rule sync failed at startup: %v", err)
		}
		amp.StartErrorRuleRuntime()
		if err := amp.ReloadErrorRules(); err != nil {
			log.Printf("warning: error rule runtime reload failed at startup: %v", err)
		}
		amp.StartRequestFilterRuntime()
		if err := amp.ReloadRequestFilters(); err != nil {
			log.Printf("warning: request filter runtime reload failed at startup: %v", err)
		}
		sysConfigService.ReloadBillingRuntimeFromSystemConfigBestEffort("server startup")
		amp.InitSessionStickyRuntime(cfg)
		defer amp.StopSessionStickyRuntime()
		amp.InitLogWriter(database.GetDB())
		defer amp.StopLogWriter()
		amp.InitRequestDetailStore(database.GetDB())
		defer amp.StopRequestDetailStore()
		billing.InitPriceStore()
		defer billing.StopPriceStore()
		billing.InitCostCalculator()
		amp.InitPendingCleaner(database.GetDB())
		defer amp.StopPendingCleaner()

		logRepo := repository.NewRequestLogRepository()
		realtime.InitHub(func(id string) (interface{}, error) {
			return logRepo.GetByIDWithJoins(id)
		})

		if err := service.NewRequestLogService().EnsureDashboardAggregatesReady(); err != nil {
			log.Printf("warning: dashboard aggregates bootstrap failed: %v", err)
		}
		repository.StartDashboardAggregateWorker()
		defer repository.StopDashboardAggregateWorker()
	}

	if role.RunsPanel() {
		service.InitStatusMonitorScheduler()
		defer service.StopStatusMonitorScheduler()
		service.StartPurchaseWebhookWorker()
		defer service.StopPurchaseWebhookWorker()
	}

	if err := service.NewUserService().EnsureAdmin(); err != nil {
		log.Printf("警告: 管理员账户创建失败: %v", err)
	}

	r := router.Setup()

	if role.RunsProxy() {
		reloadProxyRuntimeConfig(sysConfigService)
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

func startRuntimeSubscriptions(role config.ServerRole, sysConfigService *service.SystemConfigService) {
	if role.RunsProxy() {
		invalidation.Subscribe(invalidation.ChannelRetryConfigUpdated, func() {
			reloadRetryConfig(sysConfigService)
		})
		invalidation.Subscribe(invalidation.ChannelRequestPayloadLimitUpdated, func() {
			reloadRequestPayloadLimit(sysConfigService)
		})
		invalidation.Subscribe(invalidation.ChannelRequestDetailConfigUpdated, func() {
			reloadRequestDetailConfig(sysConfigService)
		})
		invalidation.Subscribe(invalidation.ChannelTimeoutConfigUpdated, func() {
			reloadTimeoutConfig(sysConfigService)
		})
		invalidation.Subscribe(invalidation.ChannelSessionStickyConfigUpdated, func() {
			reloadSessionStickyConfig(sysConfigService)
		})
		invalidation.Subscribe(invalidation.ChannelBillingRuntimeUpdated, func() {
			sysConfigService.ReloadBillingRuntimeFromSystemConfigBestEffort("pubsub reload")
		})
		invalidation.Subscribe(invalidation.ChannelModelMetadataUpdated, func() {
			amp.InvalidateModelMetadataCache()
		})
		invalidation.Subscribe(invalidation.ChannelPriceStoreUpdated, func() {
			reloadPriceStore()
		})
	}

	if role.RunsPanel() || role.RunsProxy() {
		invalidation.Subscribe(invalidation.ChannelSiteConfigUpdated, func() {
			service.RefreshStatsLocationCache()
		})
		invalidation.Subscribe(invalidation.ChannelChannelsUpdated, func() {
			service.InvalidateEnabledChannelsCache()
		})
	}

	if role.RunsPanel() {
		invalidation.Subscribe(invalidation.ChannelUserPanelRateLimitUpdated, func() {
			middleware.LoadUserPanelRateLimitConfigBestEffort("pubsub reload")
		})
	}
}

func reloadProxyRuntimeConfig(sysConfigService *service.SystemConfigService) {
	reloadRetryConfig(sysConfigService)
	reloadTimeoutConfig(sysConfigService)
	reloadRequestDetailConfig(sysConfigService)
	reloadRequestPayloadLimit(sysConfigService)
	reloadSessionStickyConfig(sysConfigService)
	if cacheTTL, err := sysConfigService.GetCacheTTLOverride(); err == nil && cacheTTL != "" {
		filters.SetCacheTTLOverride(cacheTTL)
	}
}

func reloadRetryConfig(sysConfigService *service.SystemConfigService) {
	if configJSON, err := sysConfigService.GetRetryConfigJSON(); err == nil && configJSON != "" {
		amp.InitRetryTransportConfig(configJSON)
	}
}

func reloadTimeoutConfig(sysConfigService *service.SystemConfigService) {
	if configJSON, err := sysConfigService.GetTimeoutConfigJSON(); err == nil && configJSON != "" {
		amp.InitTimeoutConfig(configJSON)
	}
}

func reloadRequestDetailConfig(sysConfigService *service.SystemConfigService) {
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
}

func reloadRequestPayloadLimit(sysConfigService *service.SystemConfigService) {
	if payloadLimitCfg, err := sysConfigService.GetRequestPayloadLimit(); err == nil {
		amp.UpdateRequestPayloadLimitBytes(payloadLimitCfg.MaxBytes)
	}
}

func reloadSessionStickyConfig(sysConfigService *service.SystemConfigService) {
	if sessionStickyCfg, err := sysConfigService.GetSessionStickyConfig(); err == nil {
		amp.UpdateSessionStickyConfig(sessionStickyCfg)
	}
}

func reloadPriceStore() {
	if store := billing.GetPriceStore(); store != nil {
		if err := store.LoadFromDB(); err != nil {
			log.Printf("warning: price store reload failed: %v", err)
		}
		if err := store.LoadContextRulesFromDB(); err != nil {
			log.Printf("warning: price context rule reload failed: %v", err)
		}
	}
}
