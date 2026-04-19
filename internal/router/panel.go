package router

import (
	"time"

	"ampmanager/internal/config"
	"ampmanager/internal/middleware"

	"github.com/gin-gonic/gin"
)

func registerPanelRoutes(r *gin.Engine, deps *routeDeps) {
	api := r.Group("/api")

	manageAuth := api.Group("/manage/auth")
	manageAuth.Use(deps.authLimiter.RateLimitByIP())
	{
		manageAuth.POST("/register", deps.userHandler.Register)
		manageAuth.POST("/login", deps.userHandler.Login)
	}

	public := api.Group("/public")
	{
		public.GET("/site-config", deps.systemHandler.GetPublicSiteConfig)
		public.GET("/announcements", deps.announcementHandler.ListPublic)
		public.POST("/purchase/alipay/notify", deps.purchaseHandler.AlipayNotify)
	}

	me := api.Group("/me")
	me.Use(middleware.JWTAuthMiddleware())
	me.Use(middleware.CredentialBootstrapMiddleware())
	me.Use(middleware.InvalidateUserResponseCachesOnWrite())
	me.Use(middleware.UserPanelRateLimit())
	{
		me.GET("/bootstrap/state", deps.userHandler.GetCredentialBootstrapState)
		me.POST("/bootstrap/credentials", deps.userHandler.CompleteBootstrapCredentials)
		me.PUT("/password", deps.userHandler.ChangePassword)
		me.PUT("/username", deps.userHandler.ChangeUsername)
		me.GET("/balance", deps.userHandler.GetMyBalance)
		me.GET("/billing/state", middleware.UserScopedResponseCache(func(*gin.Context) time.Duration {
			return 5 * time.Second
		}), deps.billingSettingHandler.GetBillingState)
		me.PUT("/billing/priority", deps.billingSettingHandler.UpdateBillingPriority)
		me.POST("/billing/daily-reset", deps.billingSettingHandler.ResetDailyBilling)
		me.GET("/subscription", deps.billingSettingHandler.GetMySubscription)
		me.GET("/announcements", deps.announcementHandler.ListForMe)
		me.POST("/announcements/:id/read", deps.announcementHandler.MarkRead)
		me.GET("/status/dashboard", middleware.UserScopedResponseCache(func(*gin.Context) time.Duration {
			cfg, err := deps.statusMonitorService.GetRuntimeConfig()
			if err != nil || cfg.PollIntervalSec <= 0 {
				return 15 * time.Second
			}
			ttl := time.Duration(cfg.PollIntervalSec) * time.Second
			if ttl > 15*time.Second {
				return 15 * time.Second
			}
			return ttl
		}), deps.statusMonitorHandler.GetDashboard)

		purchase := me.Group("/purchase")
		{
			purchase.GET("/products", deps.purchaseHandler.GetCatalog)
			purchase.GET("/orders", deps.purchaseHandler.ListMyOrders)
			purchase.POST("/quote", deps.purchaseHandler.QuoteOrder)
			purchase.POST("/orders", deps.purchaseHandler.CreateOrder)
			purchase.POST("/balance-topup/orders", deps.purchaseHandler.CreateBalanceTopupOrder)
			purchase.GET("/orders/:orderNo", deps.purchaseHandler.GetMyOrder)
			purchase.POST("/orders/:orderNo/refresh", deps.purchaseHandler.RefreshMyOrder)
		}

		invite := me.Group("/invite")
		{
			invite.GET("/summary", deps.inviteHandler.GetMySummary)
			invite.GET("/rewards", deps.inviteHandler.ListMyRewardEvents)
		}

		redeem := me.Group("/redeem")
		{
			redeem.POST("", deps.redeemHandler.Redeem)
			redeem.GET("/records", deps.redeemHandler.ListMyRecords)
		}

		ampGroup := me.Group("/amp")
		{
			ampGroup.GET("/settings", deps.ampHandler.GetSettings)
			ampGroup.PUT("/settings", deps.ampHandler.UpdateSettings)
			ampGroup.POST("/settings/test", deps.ampHandler.TestConnection)

			ampGroup.GET("/api-keys", deps.ampHandler.ListAPIKeys)
			ampGroup.POST("/api-keys", deps.ampHandler.CreateAPIKey)
			ampGroup.GET("/api-keys/:id", deps.ampHandler.GetAPIKey)
			ampGroup.PATCH("/api-keys/:id", deps.ampHandler.UpdateAPIKey)
			ampGroup.PATCH("/api-keys/:id/status", deps.ampHandler.UpdateAPIKeyStatus)
			ampGroup.DELETE("/api-keys/:id", deps.ampHandler.DeleteAPIKey)

			ampGroup.GET("/bootstrap", deps.ampHandler.GetBootstrap)
			ampGroup.GET("/request-logs", deps.requestLogHandler.ListRequestLogs)
			ampGroup.GET("/request-logs/models", deps.requestLogHandler.GetDistinctModels)
			ampGroup.GET("/request-logs/keys", deps.requestLogHandler.GetDistinctAPIKeys)
			ampGroup.GET("/request-logs/:id", deps.requestLogHandler.GetRequestLog)
			ampGroup.GET("/usage/summary", middleware.UserScopedResponseCache(func(*gin.Context) time.Duration {
				return 10 * time.Second
			}), deps.requestLogHandler.GetUsageSummary)
		}
	}

	models := api.Group("/models")
	models.Use(middleware.JWTAuthMiddleware())
	models.Use(middleware.UserPanelRateLimit())
	{
		models.GET("", middleware.UserScopedResponseCache(func(*gin.Context) time.Duration {
			return 10 * time.Second
		}), deps.modelHandler.ListAvailableModels)
	}

	admin := api.Group("/admin")
	admin.Use(middleware.AdminAccessMiddleware())
	{
		system := admin.Group("/system")
		registerPanelAdminSystemRoutes(system, deps)

		users := admin.Group("/users")
		{
			users.GET("/paged", deps.userHandler.ListUsersPaged)
			users.GET("", deps.userHandler.ListUsers)
			users.POST("/batch-preview", deps.userHandler.PreviewBatchUpdate)
			users.POST("/batch-apply", deps.userHandler.ApplyBatchUpdate)
			users.PATCH("/:id/admin", deps.userHandler.SetAdmin)
			users.PATCH("/:id/concurrency", deps.userHandler.SetConcurrencyLimit)
			users.PATCH("/:id/group", deps.userHandler.SetGroup)
			users.GET("/:id/api-keys", deps.ampHandler.AdminListUserAPIKeys)
			users.PATCH("/:id/api-keys/:keyId", deps.ampHandler.AdminUpdateUserAPIKey)
			users.PATCH("/:id/api-keys/:keyId/status", deps.ampHandler.AdminUpdateUserAPIKeyStatus)
			users.DELETE("/:id/api-keys/:keyId", deps.ampHandler.AdminDeleteUserAPIKey)
			users.POST("/:id/reset-password", deps.userHandler.ResetPassword)
			users.POST("/:id/topup", deps.userHandler.TopUp)
			users.DELETE("/:id", deps.userHandler.DeleteUser)
			users.GET("/:id/subscription", deps.subscriptionHandler.GetUserSubscription)
			users.POST("/:id/subscription", deps.subscriptionHandler.AssignSubscription)
			users.PATCH("/:id/subscription", deps.subscriptionHandler.UpdateSubscriptionExpiry)
			users.DELETE("/:id/subscription", deps.subscriptionHandler.CancelSubscription)
		}

		groups := admin.Group("/groups")
		{
			groups.GET("", deps.groupHandler.List)
			groups.POST("", deps.groupHandler.Create)
			groups.GET("/:id", deps.groupHandler.Get)
			groups.PUT("/:id", deps.groupHandler.Update)
			groups.DELETE("/:id", deps.groupHandler.Delete)
		}

		subscriptions := admin.Group("/subscriptions")
		{
			plans := subscriptions.Group("/plans")
			{
				plans.GET("", deps.subscriptionHandler.List)
				plans.POST("", deps.subscriptionHandler.Create)
				plans.GET("/:id", deps.subscriptionHandler.Get)
				plans.PUT("/:id", deps.subscriptionHandler.Update)
				plans.DELETE("/:id", deps.subscriptionHandler.Delete)
				plans.PATCH("/:id/enabled", deps.subscriptionHandler.SetEnabled)
			}
		}

		announcements := admin.Group("/announcements")
		{
			announcements.GET("", deps.announcementHandler.ListAdmin)
			announcements.POST("", deps.announcementHandler.Create)
			announcements.PUT("/:id", deps.announcementHandler.Update)
			announcements.DELETE("/:id", deps.announcementHandler.Delete)
		}

		purchase := admin.Group("/purchase")
		{
			purchase.GET("/settings", deps.purchaseHandler.GetSettings)
			purchase.PUT("/settings", deps.purchaseHandler.UpdateSettings)
			purchase.GET("/webhooks", deps.purchaseHandler.ListWebhookTargets)
			purchase.POST("/webhooks", deps.purchaseHandler.CreateWebhookTarget)
			purchase.PUT("/webhooks/:id", deps.purchaseHandler.UpdateWebhookTarget)
			purchase.PATCH("/webhooks/:id/enabled", deps.purchaseHandler.SetWebhookTargetEnabled)
			purchase.DELETE("/webhooks/:id", deps.purchaseHandler.DeleteWebhookTarget)
			purchase.POST("/webhooks/:id/test", deps.purchaseHandler.TestWebhookTarget)
			purchase.GET("/products", deps.purchaseHandler.ListProductsAdmin)
			purchase.POST("/products", deps.purchaseHandler.CreateProduct)
			purchase.PUT("/products/:id", deps.purchaseHandler.UpdateProduct)
			purchase.DELETE("/products/:id", deps.purchaseHandler.DeleteProduct)
			purchase.PATCH("/products/:id/enabled", deps.purchaseHandler.SetProductEnabled)
			purchase.GET("/orders", deps.purchaseHandler.ListOrdersAdmin)
			purchase.POST("/orders/:orderNo/refresh", deps.purchaseHandler.RefreshOrderAdmin)
			purchase.POST("/orders/:orderNo/payment-status", deps.purchaseHandler.UpdateOrderPaymentStatus)
			purchase.GET("/orders/:orderNo/payment-status-history", deps.purchaseHandler.ListOrderPaymentStatusHistory)
			purchase.POST("/orders/:orderNo/manual-settlement", deps.purchaseHandler.CreateSingleManualSettlement)
			purchase.POST("/manual-settlements/preview", deps.purchaseHandler.PreviewBatchManualSettlement)
			purchase.POST("/manual-settlements/confirm", deps.purchaseHandler.CreateBatchManualSettlement)
		}

		redeem := admin.Group("/redeem")
		{
			redeem.GET("/campaigns", deps.redeemHandler.ListCampaigns)
			redeem.POST("/campaigns", deps.redeemHandler.CreateCampaign)
			redeem.PUT("/campaigns/:id", deps.redeemHandler.UpdateCampaign)
			redeem.DELETE("/campaigns/:id", deps.redeemHandler.DeleteCampaign)
			redeem.GET("/batches", deps.redeemHandler.ListBatches)
			redeem.POST("/campaigns/:id/batches", deps.redeemHandler.CreateBatch)
			redeem.GET("/batches/:id/export", deps.redeemHandler.ExportBatch)
			redeem.POST("/codes", deps.redeemHandler.CreateCode)
			redeem.GET("/codes", deps.redeemHandler.ListCodes)
			redeem.PATCH("/codes/:id/enabled", deps.redeemHandler.SetCodeEnabled)
			redeem.GET("/redemptions", deps.redeemHandler.ListRedemptions)
		}

		invite := admin.Group("/invite")
		{
			invite.GET("/config", deps.inviteHandler.GetConfig)
			invite.PUT("/config", deps.inviteHandler.UpdateConfig)
			invite.GET("/stats", deps.inviteHandler.GetStats)
			invite.GET("/relations", deps.inviteHandler.ListRelations)
			invite.GET("/reward-events", deps.inviteHandler.ListRewardEvents)
		}

		coupons := admin.Group("/coupons")
		{
			coupons.GET("/config", deps.couponHandler.GetConfig)
			coupons.PUT("/config", deps.couponHandler.UpdateConfig)
			coupons.GET("/campaigns", deps.couponHandler.ListCampaigns)
			coupons.POST("/campaigns", deps.couponHandler.CreateCampaign)
			coupons.PUT("/campaigns/:id", deps.couponHandler.UpdateCampaign)
			coupons.DELETE("/campaigns/:id", deps.couponHandler.DeleteCampaign)
			coupons.POST("/campaigns/:id/batches", deps.couponHandler.CreateBatch)
			coupons.GET("/codes", deps.couponHandler.ListCodes)
			coupons.PATCH("/codes/:id/enabled", deps.couponHandler.SetCodeEnabled)
			coupons.GET("/usages", deps.couponHandler.ListUsages)
		}

	}
}

func registerPanelAdminSystemRoutes(system *gin.RouterGroup, deps *routeDeps) {
	security := system.Group("/security")
	{
		managementKey := security.Group("/management-key")
		{
			managementKey.GET("/status", deps.systemHandler.GetManagementAPIKeyStatus)
			managementKey.POST("/create", deps.systemHandler.CreateManagementAPIKey)
			managementKey.POST("/reveal", deps.systemHandler.RevealManagementAPIKey)
			managementKey.POST("/rotate", deps.systemHandler.RotateManagementAPIKey)
			managementKey.PATCH("/enabled", deps.systemHandler.UpdateManagementAPIKeyEnabled)
		}
		security.GET("/user-panel-rate-limit", deps.systemHandler.GetUserPanelRateLimitConfig)
		security.PUT("/user-panel-rate-limit", deps.systemHandler.UpdateUserPanelRateLimitConfig)
	}

	system.GET("/database-info", deps.systemHandler.GetDatabaseInfo)
	if deps.cfg.Role() == config.ServerRoleAll {
		system.POST("/database/upload", deps.systemHandler.UploadDatabase)
		system.GET("/database/download", deps.systemHandler.DownloadDatabase)
		system.GET("/database/backups", deps.systemHandler.ListBackups)
		system.POST("/database/restore", deps.systemHandler.RestoreBackup)
		system.DELETE("/database/backups/:filename", deps.systemHandler.DeleteBackup)
		system.POST("/database/migrate", deps.systemHandler.StartDatabaseMigration)
		system.GET("/database/migrate/:taskID", deps.systemHandler.GetDatabaseMigrationTask)
	}

	system.GET("/billing-daily-reset-config", deps.systemHandler.GetBillingDailyResetConfig)
	system.PUT("/billing-daily-reset-config", deps.systemHandler.UpdateBillingDailyResetConfig)

	system.GET("/status-monitor-runtime", deps.statusMonitorHandler.GetRuntimeConfig)
	system.PUT("/status-monitor-runtime", deps.statusMonitorHandler.UpdateRuntimeConfig)
	system.GET("/status-monitors", deps.statusMonitorHandler.ListMonitors)
	system.POST("/status-monitors", deps.statusMonitorHandler.CreateMonitor)
	system.PUT("/status-monitors/:id", deps.statusMonitorHandler.UpdateMonitor)
	system.DELETE("/status-monitors/:id", deps.statusMonitorHandler.DeleteMonitor)
	system.POST("/status-monitors/:id/run", deps.statusMonitorHandler.RunMonitor)
	system.POST("/status-monitors/run-all", deps.statusMonitorHandler.RunAll)
}
