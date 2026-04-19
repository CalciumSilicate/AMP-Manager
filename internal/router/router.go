package router

import (
	"os"
	"strings"
	"time"

	"ampmanager/internal/amp"
	"ampmanager/internal/config"
	"ampmanager/internal/handler"
	"ampmanager/internal/middleware"
	"ampmanager/internal/service"
	"ampmanager/internal/web"

	"github.com/gin-gonic/gin"
)

func Setup() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	if os.Getenv("AMP_ENABLE_ACCESS_LOG") == "true" {
		r.Use(gin.Logger())
	}

	cfg := config.Get()

	// 解析 CORS 配置
	allowedOrigins := make([]string, 0)
	if cfg.CORSAllowedOrigins != "" {
		for _, o := range strings.Split(cfg.CORSAllowedOrigins, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" && trimmed != "*" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	}
	corsEnabled := len(allowedOrigins) > 0

	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// 只有配置了具体的允许源时才启用 CORS
		if corsEnabled && origin != "" {
			allowed := false
			for _, o := range allowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Api-Key")
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Vary", "Origin")
			}
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	authLimiter := middleware.NewRateLimiter(cfg.RateLimitAuthRPS, 10)
	proxyLimiter := middleware.NewRateLimiter(cfg.RateLimitProxyRPS, 200)

	userHandler := handler.NewUserHandler()
	ampHandler := handler.NewAmpHandler()
	requestLogHandler := handler.NewRequestLogHandler()
	channelHandler := handler.NewChannelHandler()
	modelHandler := handler.NewModelHandler()
	modelMetadataHandler := handler.NewModelMetadataHandler()
	systemHandler := handler.NewSystemHandler()
	billingHandler := handler.NewBillingHandler()
	groupHandler := handler.NewGroupHandler()
	subscriptionHandler := handler.NewSubscriptionHandler()
	billingSettingHandler := handler.NewBillingSettingHandler()
	purchaseHandler := handler.NewPurchaseHandler()
	redeemHandler := handler.NewRedeemHandler()
	inviteHandler := handler.NewInviteHandler()
	couponHandler := handler.NewCouponHandler()
	announcementHandler := handler.NewAnnouncementHandler()
	statusMonitorHandler := handler.NewStatusMonitorHandler()
	statusMonitorService := service.NewStatusMonitorService()
	sessionHandler := handler.NewSessionHandler()

	api := r.Group("/api")
	{
		api.GET("/usage", amp.APIKeyAuthMiddleware(), proxyLimiter.RateLimitByAPIKey(), ampHandler.GetAPIUsage)

		// Local management auth (using /manage/auth to avoid conflict with proxy /api/auth/*)
		manageAuth := api.Group("/manage/auth")
		manageAuth.Use(authLimiter.RateLimitByIP())
		{
			manageAuth.POST("/register", userHandler.Register)
			manageAuth.POST("/login", userHandler.Login)
		}

		public := api.Group("/public")
		{
			public.GET("/site-config", systemHandler.GetPublicSiteConfig)
			public.GET("/announcements", announcementHandler.ListPublic)
			public.POST("/purchase/alipay/notify", purchaseHandler.AlipayNotify)
		}

		me := api.Group("/me")
		me.Use(middleware.JWTAuthMiddleware())
		me.Use(middleware.CredentialBootstrapMiddleware())
		me.Use(middleware.InvalidateUserResponseCachesOnWrite())
		me.Use(middleware.UserPanelRateLimit())
		{
			me.GET("/bootstrap/state", userHandler.GetCredentialBootstrapState)
			me.POST("/bootstrap/credentials", userHandler.CompleteBootstrapCredentials)
			me.PUT("/password", userHandler.ChangePassword)
			me.PUT("/username", userHandler.ChangeUsername)
			me.GET("/balance", userHandler.GetMyBalance)
			me.GET("/dashboard", middleware.UserScopedResponseCache(func(*gin.Context) time.Duration {
				return 30 * time.Second
			}), requestLogHandler.GetDashboard)
			me.GET("/billing/state", middleware.UserScopedResponseCache(func(*gin.Context) time.Duration {
				return 5 * time.Second
			}), billingSettingHandler.GetBillingState)
			me.PUT("/billing/priority", billingSettingHandler.UpdateBillingPriority)
			me.POST("/billing/daily-reset", billingSettingHandler.ResetDailyBilling)
			me.GET("/subscription", billingSettingHandler.GetMySubscription)
			me.GET("/announcements", announcementHandler.ListForMe)
			me.POST("/announcements/:id/read", announcementHandler.MarkRead)
			me.GET("/status/dashboard", middleware.UserScopedResponseCache(func(*gin.Context) time.Duration {
				cfg, err := statusMonitorService.GetRuntimeConfig()
				if err != nil || cfg.PollIntervalSec <= 0 {
					return 15 * time.Second
				}
				ttl := time.Duration(cfg.PollIntervalSec) * time.Second
				if ttl > 15*time.Second {
					return 15 * time.Second
				}
				return ttl
			}), statusMonitorHandler.GetDashboard)

			purchase := me.Group("/purchase")
			{
				purchase.GET("/products", purchaseHandler.GetCatalog)
				purchase.GET("/orders", purchaseHandler.ListMyOrders)
				purchase.POST("/quote", purchaseHandler.QuoteOrder)
				purchase.POST("/orders", purchaseHandler.CreateOrder)
				purchase.POST("/balance-topup/orders", purchaseHandler.CreateBalanceTopupOrder)
				purchase.GET("/orders/:orderNo", purchaseHandler.GetMyOrder)
				purchase.POST("/orders/:orderNo/refresh", purchaseHandler.RefreshMyOrder)
			}

			invite := me.Group("/invite")
			{
				invite.GET("/summary", inviteHandler.GetMySummary)
				invite.GET("/rewards", inviteHandler.ListMyRewardEvents)
			}

			redeem := me.Group("/redeem")
			{
				redeem.POST("", redeemHandler.Redeem)
				redeem.GET("/records", redeemHandler.ListMyRecords)
			}

			ampGroup := me.Group("/amp")
			{
				ampGroup.GET("/settings", ampHandler.GetSettings)
				ampGroup.PUT("/settings", ampHandler.UpdateSettings)
				ampGroup.POST("/settings/test", ampHandler.TestConnection)

				ampGroup.GET("/api-keys", ampHandler.ListAPIKeys)
				ampGroup.POST("/api-keys", ampHandler.CreateAPIKey)
				ampGroup.GET("/api-keys/:id", ampHandler.GetAPIKey)
				ampGroup.PATCH("/api-keys/:id", ampHandler.UpdateAPIKey)
				ampGroup.PATCH("/api-keys/:id/status", ampHandler.UpdateAPIKeyStatus)
				ampGroup.DELETE("/api-keys/:id", ampHandler.DeleteAPIKey)

				ampGroup.GET("/bootstrap", ampHandler.GetBootstrap)

				// 请求日志
				ampGroup.GET("/request-logs", requestLogHandler.ListRequestLogs)
				ampGroup.GET("/request-logs/models", requestLogHandler.GetDistinctModels)
				ampGroup.GET("/request-logs/keys", requestLogHandler.GetDistinctAPIKeys)
				ampGroup.GET("/request-logs/:id", requestLogHandler.GetRequestLog)
				ampGroup.GET("/request-logs/:id/detail", requestLogHandler.GetRequestLogDetail)
				ampGroup.GET("/usage/summary", middleware.UserScopedResponseCache(func(*gin.Context) time.Duration {
					return 10 * time.Second
				}), requestLogHandler.GetUsageSummary)
			}
		}

		models := api.Group("/models")
		models.Use(middleware.JWTAuthMiddleware())
		models.Use(middleware.UserPanelRateLimit())
		{
			models.GET("", middleware.UserScopedResponseCache(func(*gin.Context) time.Duration {
				return 10 * time.Second
			}), modelHandler.ListAvailableModels)
		}

		admin := api.Group("/admin")
		admin.Use(middleware.AdminAccessMiddleware())
		{
			channels := admin.Group("/channels")
			{
				channels.GET("", channelHandler.List)
				channels.POST("", channelHandler.Create)
				channels.GET("/:id", channelHandler.Get)
				channels.PUT("/:id", channelHandler.Update)
				channels.DELETE("/:id", channelHandler.Delete)
				channels.PATCH("/:id/enabled", channelHandler.SetEnabled)
				channels.POST("/:id/test", channelHandler.TestConnection)
				channels.POST("/:id/fetch-models", modelHandler.FetchChannelModels)
				channels.GET("/:id/models", modelHandler.GetChannelModels)
			}

			adminModels := admin.Group("/models")
			{
				adminModels.POST("/fetch-all", modelHandler.FetchAllModels)
			}

			modelMetadata := admin.Group("/model-metadata")
			{
				modelMetadata.GET("", modelMetadataHandler.List)
				modelMetadata.GET("/:id", modelMetadataHandler.Get)
				modelMetadata.POST("", modelMetadataHandler.Create)
				modelMetadata.PUT("/:id", modelMetadataHandler.Update)
				modelMetadata.DELETE("/:id", modelMetadataHandler.Delete)
			}

			system := admin.Group("/system")
			{
				security := system.Group("/security")
				{
					managementKey := security.Group("/management-key")
					{
						managementKey.GET("/status", systemHandler.GetManagementAPIKeyStatus)
						managementKey.POST("/create", systemHandler.CreateManagementAPIKey)
						managementKey.POST("/reveal", systemHandler.RevealManagementAPIKey)
						managementKey.POST("/rotate", systemHandler.RotateManagementAPIKey)
						managementKey.PATCH("/enabled", systemHandler.UpdateManagementAPIKeyEnabled)
					}
					security.GET("/user-panel-rate-limit", systemHandler.GetUserPanelRateLimitConfig)
					security.PUT("/user-panel-rate-limit", systemHandler.UpdateUserPanelRateLimitConfig)
				}

				system.POST("/database/upload", systemHandler.UploadDatabase)
				system.GET("/database/download", systemHandler.DownloadDatabase)
				system.GET("/database/backups", systemHandler.ListBackups)
				system.POST("/database/restore", systemHandler.RestoreBackup)
				system.DELETE("/database/backups/:filename", systemHandler.DeleteBackup)
				system.GET("/database-info", systemHandler.GetDatabaseInfo)
				system.POST("/database/migrate", systemHandler.StartDatabaseMigration)
				system.GET("/database/migrate/:taskID", systemHandler.GetDatabaseMigrationTask)

				// 重试配置
				system.GET("/retry-config", systemHandler.GetRetryConfig)
				system.PUT("/retry-config", systemHandler.UpdateRetryConfig)
				system.GET("/request-payload-limit", systemHandler.GetRequestPayloadLimit)
				system.PUT("/request-payload-limit", systemHandler.UpdateRequestPayloadLimit)
				system.GET("/billing-daily-reset-config", systemHandler.GetBillingDailyResetConfig)
				system.PUT("/billing-daily-reset-config", systemHandler.UpdateBillingDailyResetConfig)
				system.GET("/error-rules", systemHandler.ListErrorRulesV2)
				system.POST("/error-rules", systemHandler.CreateErrorRule)
				system.POST("/error-rules/test", systemHandler.TestErrorRule)
				system.POST("/error-rules/refresh", systemHandler.RefreshErrorRuleRuntime)
				system.GET("/error-rules/cache-stats", systemHandler.GetErrorRuleCacheStats)
				system.PATCH("/error-rules/:id", systemHandler.UpdateErrorRule)
				system.DELETE("/error-rules/:id", systemHandler.DeleteErrorRule)

				system.GET("/request-filters", systemHandler.ListRequestFilters)
				system.POST("/request-filters", systemHandler.CreateRequestFilter)
				system.POST("/request-filters/refresh", systemHandler.RefreshRequestFilters)
				system.GET("/request-filters/bindings", systemHandler.GetRequestFilterBindings)
				system.PATCH("/request-filters/:id", systemHandler.UpdateRequestFilter)
				system.DELETE("/request-filters/:id", systemHandler.DeleteRequestFilter)

				// 请求详情监控配置
				system.GET("/request-detail-enabled", systemHandler.GetRequestDetailEnabled)
				system.PUT("/request-detail-enabled", systemHandler.UpdateRequestDetailEnabled)
				system.GET("/request-detail-config", systemHandler.GetRequestDetailConfig)
				system.PUT("/request-detail-config", systemHandler.UpdateRequestDetailConfig)

				// 超时配置
				system.GET("/timeout-config", systemHandler.GetTimeoutConfig)
				system.PUT("/timeout-config", systemHandler.UpdateTimeoutConfig)

				// 缓存 TTL 配置
				system.GET("/cache-ttl", systemHandler.GetCacheTTLConfig)
				system.PUT("/cache-ttl", systemHandler.UpdateCacheTTLConfig)

				// Session 粘滞配置
				system.GET("/session-sticky-config", systemHandler.GetSessionStickyConfig)
				system.PUT("/session-sticky-config", systemHandler.UpdateSessionStickyConfig)
				system.GET("/session-sticky-runtime", systemHandler.GetSessionStickyRuntime)

				// Redis 计费运行时配置
				system.GET("/billing-runtime-config", systemHandler.GetBillingRuntimeConfig)
				system.GET("/billing-runtime-stats", systemHandler.GetBillingRuntimeStats)
				system.PUT("/billing-runtime-config", systemHandler.UpdateBillingRuntimeConfig)

				// 站点配置
				system.GET("/site-config", systemHandler.GetSiteConfig)
				system.PUT("/site-config", systemHandler.UpdateSiteConfig)
				system.GET("/status-monitor-runtime", statusMonitorHandler.GetRuntimeConfig)
				system.PUT("/status-monitor-runtime", statusMonitorHandler.UpdateRuntimeConfig)
				system.GET("/status-monitors", statusMonitorHandler.ListMonitors)
				system.POST("/status-monitors", statusMonitorHandler.CreateMonitor)
				system.PUT("/status-monitors/:id", statusMonitorHandler.UpdateMonitor)
				system.DELETE("/status-monitors/:id", statusMonitorHandler.DeleteMonitor)
				system.POST("/status-monitors/:id/run", statusMonitorHandler.RunMonitor)
				system.POST("/status-monitors/run-all", statusMonitorHandler.RunAll)
			}

			users := admin.Group("/users")
			{
				users.GET("/paged", userHandler.ListUsersPaged)
				users.GET("", userHandler.ListUsers)
				users.POST("/batch-preview", userHandler.PreviewBatchUpdate)
				users.POST("/batch-apply", userHandler.ApplyBatchUpdate)
				users.PATCH("/:id/admin", userHandler.SetAdmin)
				users.PATCH("/:id/concurrency", userHandler.SetConcurrencyLimit)
				users.PATCH("/:id/group", userHandler.SetGroup)
				users.GET("/:id/api-keys", ampHandler.AdminListUserAPIKeys)
				users.PATCH("/:id/api-keys/:keyId", ampHandler.AdminUpdateUserAPIKey)
				users.PATCH("/:id/api-keys/:keyId/status", ampHandler.AdminUpdateUserAPIKeyStatus)
				users.DELETE("/:id/api-keys/:keyId", ampHandler.AdminDeleteUserAPIKey)
				users.POST("/:id/reset-password", userHandler.ResetPassword)
				users.POST("/:id/topup", userHandler.TopUp)
				users.DELETE("/:id", userHandler.DeleteUser)
				users.GET("/:id/subscription", subscriptionHandler.GetUserSubscription)
				users.POST("/:id/subscription", subscriptionHandler.AssignSubscription)
				users.PATCH("/:id/subscription", subscriptionHandler.UpdateSubscriptionExpiry)
				users.DELETE("/:id/subscription", subscriptionHandler.CancelSubscription)
			}

			groups := admin.Group("/groups")
			{
				groups.GET("", groupHandler.List)
				groups.POST("", groupHandler.Create)
				groups.GET("/:id", groupHandler.Get)
				groups.PUT("/:id", groupHandler.Update)
				groups.DELETE("/:id", groupHandler.Delete)
			}

			subscriptions := admin.Group("/subscriptions")
			{
				plans := subscriptions.Group("/plans")
				{
					plans.GET("", subscriptionHandler.List)
					plans.POST("", subscriptionHandler.Create)
					plans.GET("/:id", subscriptionHandler.Get)
					plans.PUT("/:id", subscriptionHandler.Update)
					plans.DELETE("/:id", subscriptionHandler.Delete)
					plans.PATCH("/:id/enabled", subscriptionHandler.SetEnabled)
				}
			}

			announcements := admin.Group("/announcements")
			{
				announcements.GET("", announcementHandler.ListAdmin)
				announcements.POST("", announcementHandler.Create)
				announcements.PUT("/:id", announcementHandler.Update)
				announcements.DELETE("/:id", announcementHandler.Delete)
			}

			// 管理员日志和使用统计
			admin.GET("/request-logs", requestLogHandler.AdminListRequestLogs)
			admin.GET("/request-logs/models", requestLogHandler.AdminGetDistinctModels)
			admin.GET("/request-logs/keys", requestLogHandler.AdminGetDistinctAPIKeys)
			admin.GET("/request-logs/:id/detail", requestLogHandler.AdminGetRequestLogDetail)
			admin.GET("/sessions", sessionHandler.ListSessions)
			admin.GET("/sessions/leaderboard", sessionHandler.GetLeaderboard)
			admin.GET("/sessions/:id", sessionHandler.GetSession)
			admin.GET("/usage/summary", requestLogHandler.AdminGetUsageSummary)
			admin.GET("/dashboard/summary", middleware.AdminScopedResponseCache(30*time.Second), requestLogHandler.GetAdminDashboardSummary)
			admin.GET("/dashboard/trends", middleware.AdminScopedResponseCache(15*time.Second), requestLogHandler.GetAdminDashboardTrends)
			admin.GET("/dashboard/cache-hit", middleware.AdminScopedResponseCache(60*time.Second), requestLogHandler.GetAdminDashboardCacheHit)
			admin.GET("/dashboard", middleware.AdminScopedResponseCache(15*time.Second), requestLogHandler.GetAdminDashboard)

			// 价格表管理
			prices := admin.Group("/prices")
			{
				prices.GET("", billingHandler.ListPrices)
				prices.GET("/stats", billingHandler.GetPriceStats)
				prices.POST("/refresh", billingHandler.RefreshPrices)
				prices.GET("/context-rules", billingHandler.ListContextRules)
				prices.PUT("/context-rules", billingHandler.UpdateContextRules)
			}

			purchase := admin.Group("/purchase")
			{
				purchase.GET("/settings", purchaseHandler.GetSettings)
				purchase.PUT("/settings", purchaseHandler.UpdateSettings)
				purchase.GET("/webhooks", purchaseHandler.ListWebhookTargets)
				purchase.POST("/webhooks", purchaseHandler.CreateWebhookTarget)
				purchase.PUT("/webhooks/:id", purchaseHandler.UpdateWebhookTarget)
				purchase.PATCH("/webhooks/:id/enabled", purchaseHandler.SetWebhookTargetEnabled)
				purchase.DELETE("/webhooks/:id", purchaseHandler.DeleteWebhookTarget)
				purchase.POST("/webhooks/:id/test", purchaseHandler.TestWebhookTarget)

				purchase.GET("/products", purchaseHandler.ListProductsAdmin)
				purchase.POST("/products", purchaseHandler.CreateProduct)
				purchase.PUT("/products/:id", purchaseHandler.UpdateProduct)
				purchase.DELETE("/products/:id", purchaseHandler.DeleteProduct)
				purchase.PATCH("/products/:id/enabled", purchaseHandler.SetProductEnabled)

				purchase.GET("/orders", purchaseHandler.ListOrdersAdmin)
				purchase.POST("/orders/:orderNo/refresh", purchaseHandler.RefreshOrderAdmin)
				purchase.POST("/orders/:orderNo/payment-status", purchaseHandler.UpdateOrderPaymentStatus)
				purchase.GET("/orders/:orderNo/payment-status-history", purchaseHandler.ListOrderPaymentStatusHistory)
				purchase.POST("/orders/:orderNo/manual-settlement", purchaseHandler.CreateSingleManualSettlement)
				purchase.POST("/manual-settlements/preview", purchaseHandler.PreviewBatchManualSettlement)
				purchase.POST("/manual-settlements/confirm", purchaseHandler.CreateBatchManualSettlement)
			}

			redeem := admin.Group("/redeem")
			{
				redeem.GET("/campaigns", redeemHandler.ListCampaigns)
				redeem.POST("/campaigns", redeemHandler.CreateCampaign)
				redeem.PUT("/campaigns/:id", redeemHandler.UpdateCampaign)
				redeem.DELETE("/campaigns/:id", redeemHandler.DeleteCampaign)
				redeem.GET("/batches", redeemHandler.ListBatches)
				redeem.POST("/campaigns/:id/batches", redeemHandler.CreateBatch)
				redeem.GET("/batches/:id/export", redeemHandler.ExportBatch)
				redeem.POST("/codes", redeemHandler.CreateCode)
				redeem.GET("/codes", redeemHandler.ListCodes)
				redeem.PATCH("/codes/:id/enabled", redeemHandler.SetCodeEnabled)
				redeem.GET("/redemptions", redeemHandler.ListRedemptions)
			}

			invite := admin.Group("/invite")
			{
				invite.GET("/config", inviteHandler.GetConfig)
				invite.PUT("/config", inviteHandler.UpdateConfig)
				invite.GET("/stats", inviteHandler.GetStats)
				invite.GET("/relations", inviteHandler.ListRelations)
				invite.GET("/reward-events", inviteHandler.ListRewardEvents)
			}

			coupons := admin.Group("/coupons")
			{
				coupons.GET("/config", couponHandler.GetConfig)
				coupons.PUT("/config", couponHandler.UpdateConfig)
				coupons.GET("/campaigns", couponHandler.ListCampaigns)
				coupons.POST("/campaigns", couponHandler.CreateCampaign)
				coupons.PUT("/campaigns/:id", couponHandler.UpdateCampaign)
				coupons.DELETE("/campaigns/:id", couponHandler.DeleteCampaign)
				coupons.POST("/campaigns/:id/batches", couponHandler.CreateBatch)
				coupons.GET("/codes", couponHandler.ListCodes)
				coupons.PATCH("/codes/:id/enabled", couponHandler.SetCodeEnabled)
				coupons.GET("/usages", couponHandler.ListUsages)
			}
		}
	}

	// WebSocket 实时日志推送（使用 query 参数认证）
	api.GET("/admin/request-logs/ws",
		middleware.AdminAccessFromQueryOrHeader("token"),
		requestLogHandler.AdminRequestLogsWS,
	)

	proxy := amp.CreateDynamicReverseProxy()
	amp.RegisterProxyRoutes(r, proxy)

	// Serve embedded frontend static files
	web.RegisterStaticRoutes(r)

	return r
}
