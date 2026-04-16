package router

import (
	"os"
	"strings"

	"ampmanager/internal/amp"
	"ampmanager/internal/config"
	"ampmanager/internal/handler"
	"ampmanager/internal/middleware"
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

	api := r.Group("/api")
	{
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
			public.POST("/purchase/alipay/notify", purchaseHandler.AlipayNotify)
		}

		me := api.Group("/me")
		me.Use(middleware.JWTAuthMiddleware())
		{
			me.PUT("/password", userHandler.ChangePassword)
			me.PUT("/username", userHandler.ChangeUsername)
			me.GET("/balance", userHandler.GetMyBalance)
			me.GET("/dashboard", requestLogHandler.GetDashboard)
			me.GET("/billing/state", billingSettingHandler.GetBillingState)
			me.PUT("/billing/priority", billingSettingHandler.UpdateBillingPriority)
			me.GET("/subscription", billingSettingHandler.GetMySubscription)

				purchase := me.Group("/purchase")
				{
					purchase.GET("/products", purchaseHandler.GetCatalog)
					purchase.GET("/orders", purchaseHandler.ListMyOrders)
					purchase.POST("/orders", purchaseHandler.CreateOrder)
					purchase.POST("/balance-topup/orders", purchaseHandler.CreateBalanceTopupOrder)
					purchase.GET("/orders/:orderNo", purchaseHandler.GetMyOrder)
					purchase.POST("/orders/:orderNo/refresh", purchaseHandler.RefreshMyOrder)
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
				ampGroup.DELETE("/api-keys/:id", ampHandler.DeleteAPIKey)

				ampGroup.GET("/bootstrap", ampHandler.GetBootstrap)

				// 请求日志
				ampGroup.GET("/request-logs", requestLogHandler.ListRequestLogs)
				ampGroup.GET("/request-logs/models", requestLogHandler.GetDistinctModels)
				ampGroup.GET("/request-logs/keys", requestLogHandler.GetDistinctAPIKeys)
				ampGroup.GET("/request-logs/:id", requestLogHandler.GetRequestLog)
				ampGroup.GET("/usage/summary", requestLogHandler.GetUsageSummary)
			}
		}

		models := api.Group("/models")
		models.Use(middleware.JWTAuthMiddleware())
		{
			models.GET("", modelHandler.ListAvailableModels)
		}

		admin := api.Group("/admin")
		admin.Use(middleware.JWTAuthMiddleware())
		admin.Use(middleware.AdminMiddleware())
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

				// Redis 计费运行时配置
				system.GET("/billing-runtime-config", systemHandler.GetBillingRuntimeConfig)
				system.GET("/billing-runtime-stats", systemHandler.GetBillingRuntimeStats)
				system.PUT("/billing-runtime-config", systemHandler.UpdateBillingRuntimeConfig)

				// 站点配置
				system.GET("/site-config", systemHandler.GetSiteConfig)
				system.PUT("/site-config", systemHandler.UpdateSiteConfig)
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
			admin.GET("/usage/summary", requestLogHandler.AdminGetUsageSummary)
			admin.GET("/dashboard", requestLogHandler.GetAdminDashboard)

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

				purchase.GET("/products", purchaseHandler.ListProductsAdmin)
				purchase.POST("/products", purchaseHandler.CreateProduct)
				purchase.PUT("/products/:id", purchaseHandler.UpdateProduct)
				purchase.DELETE("/products/:id", purchaseHandler.DeleteProduct)
				purchase.PATCH("/products/:id/enabled", purchaseHandler.SetProductEnabled)

				purchase.GET("/orders", purchaseHandler.ListOrdersAdmin)
				purchase.POST("/orders/:orderNo/refresh", purchaseHandler.RefreshOrderAdmin)
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
				redeem.GET("/codes", redeemHandler.ListCodes)
				redeem.PATCH("/codes/:id/enabled", redeemHandler.SetCodeEnabled)
				redeem.GET("/redemptions", redeemHandler.ListRedemptions)
			}
		}
	}

	// WebSocket 实时日志推送（使用 query 参数认证）
	api.GET("/admin/request-logs/ws",
		middleware.JWTAuthFromQuery("token"),
		middleware.AdminMiddleware(),
		requestLogHandler.AdminRequestLogsWS,
	)

	proxy := amp.CreateDynamicReverseProxy()
	amp.RegisterProxyRoutes(r, proxy)

	// Serve embedded frontend static files
	web.RegisterStaticRoutes(r)

	return r
}
