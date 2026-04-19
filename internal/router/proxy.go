package router

import (
	"time"

	"ampmanager/internal/amp"
	"ampmanager/internal/middleware"

	"github.com/gin-gonic/gin"
)

func registerProxyOwnedRoutes(r *gin.Engine, deps *routeDeps) {
	api := r.Group("/api")
	api.GET("/usage", amp.APIKeyAuthMiddleware(), deps.proxyLimiter.RateLimitByAPIKey(), deps.ampHandler.GetAPIUsage)

	me := api.Group("/me")
	me.Use(middleware.JWTAuthMiddleware())
	me.Use(middleware.CredentialBootstrapMiddleware())
	me.Use(middleware.InvalidateUserResponseCachesOnWrite())
	me.Use(middleware.UserPanelRateLimit())
	{
		me.GET("/dashboard", middleware.UserScopedResponseCache(func(*gin.Context) time.Duration {
			return 30 * time.Second
		}), deps.requestLogHandler.GetDashboard)

		ampGroup := me.Group("/amp")
		{
			ampGroup.GET("/request-logs/:id/detail", deps.requestLogHandler.GetRequestLogDetail)
		}
	}

	admin := api.Group("/admin")
	admin.Use(middleware.AdminAccessMiddleware())
	{
		channels := admin.Group("/channels")
		{
			channels.GET("", deps.channelHandler.List)
			channels.POST("", deps.channelHandler.Create)
			channels.GET("/:id", deps.channelHandler.Get)
			channels.PUT("/:id", deps.channelHandler.Update)
			channels.DELETE("/:id", deps.channelHandler.Delete)
			channels.PATCH("/:id/enabled", deps.channelHandler.SetEnabled)
			channels.POST("/:id/test", deps.channelHandler.TestConnection)
			channels.POST("/:id/fetch-models", deps.modelHandler.FetchChannelModels)
			channels.GET("/:id/models", deps.modelHandler.GetChannelModels)
		}

		adminModels := admin.Group("/models")
		{
			adminModels.POST("/fetch-all", deps.modelHandler.FetchAllModels)
		}

		modelMetadata := admin.Group("/model-metadata")
		{
			modelMetadata.GET("", deps.modelMetadataHandler.List)
			modelMetadata.GET("/:id", deps.modelMetadataHandler.Get)
			modelMetadata.POST("", deps.modelMetadataHandler.Create)
			modelMetadata.PUT("/:id", deps.modelMetadataHandler.Update)
			modelMetadata.DELETE("/:id", deps.modelMetadataHandler.Delete)
		}

		system := admin.Group("/system")
		registerProxyAdminSystemRoutes(system, deps)

		admin.GET("/request-logs", deps.requestLogHandler.AdminListRequestLogs)
		admin.GET("/request-logs/models", deps.requestLogHandler.AdminGetDistinctModels)
		admin.GET("/request-logs/keys", deps.requestLogHandler.AdminGetDistinctAPIKeys)
		admin.GET("/request-logs/:id/detail", deps.requestLogHandler.AdminGetRequestLogDetail)
		admin.GET("/sessions", deps.sessionHandler.ListSessions)
		admin.GET("/sessions/leaderboard", deps.sessionHandler.GetLeaderboard)
		admin.GET("/sessions/:id", deps.sessionHandler.GetSession)
		admin.GET("/usage/summary", deps.requestLogHandler.AdminGetUsageSummary)
		admin.GET("/dashboard/summary", middleware.AdminScopedResponseCache(30*time.Second), deps.requestLogHandler.GetAdminDashboardSummary)
		admin.GET("/dashboard/trends", middleware.AdminScopedResponseCache(15*time.Second), deps.requestLogHandler.GetAdminDashboardTrends)
		admin.GET("/dashboard/cache-hit", middleware.AdminScopedResponseCache(60*time.Second), deps.requestLogHandler.GetAdminDashboardCacheHit)
		admin.GET("/dashboard", middleware.AdminScopedResponseCache(15*time.Second), deps.requestLogHandler.GetAdminDashboard)

		prices := admin.Group("/prices")
		{
			prices.GET("", deps.billingHandler.ListPrices)
			prices.GET("/stats", deps.billingHandler.GetPriceStats)
			prices.POST("/refresh", deps.billingHandler.RefreshPrices)
			prices.GET("/context-rules", deps.billingHandler.ListContextRules)
			prices.PUT("/context-rules", deps.billingHandler.UpdateContextRules)
		}
	}

	api.GET("/admin/request-logs/ws",
		middleware.AdminAccessFromQueryOrHeader("token"),
		deps.requestLogHandler.AdminRequestLogsWS,
	)
}

func registerProxyAdminSystemRoutes(system *gin.RouterGroup, deps *routeDeps) {
	system.GET("/retry-config", deps.systemHandler.GetRetryConfig)
	system.PUT("/retry-config", deps.systemHandler.UpdateRetryConfig)
	system.GET("/request-payload-limit", deps.systemHandler.GetRequestPayloadLimit)
	system.PUT("/request-payload-limit", deps.systemHandler.UpdateRequestPayloadLimit)
	system.GET("/error-rules", deps.systemHandler.ListErrorRulesV2)
	system.POST("/error-rules", deps.systemHandler.CreateErrorRule)
	system.POST("/error-rules/test", deps.systemHandler.TestErrorRule)
	system.POST("/error-rules/refresh", deps.systemHandler.RefreshErrorRuleRuntime)
	system.GET("/error-rules/cache-stats", deps.systemHandler.GetErrorRuleCacheStats)
	system.PATCH("/error-rules/:id", deps.systemHandler.UpdateErrorRule)
	system.DELETE("/error-rules/:id", deps.systemHandler.DeleteErrorRule)
	system.GET("/request-filters", deps.systemHandler.ListRequestFilters)
	system.POST("/request-filters", deps.systemHandler.CreateRequestFilter)
	system.POST("/request-filters/refresh", deps.systemHandler.RefreshRequestFilters)
	system.GET("/request-filters/bindings", deps.systemHandler.GetRequestFilterBindings)
	system.PATCH("/request-filters/:id", deps.systemHandler.UpdateRequestFilter)
	system.DELETE("/request-filters/:id", deps.systemHandler.DeleteRequestFilter)
	system.GET("/request-detail-enabled", deps.systemHandler.GetRequestDetailEnabled)
	system.PUT("/request-detail-enabled", deps.systemHandler.UpdateRequestDetailEnabled)
	system.GET("/request-detail-config", deps.systemHandler.GetRequestDetailConfig)
	system.PUT("/request-detail-config", deps.systemHandler.UpdateRequestDetailConfig)
	system.GET("/timeout-config", deps.systemHandler.GetTimeoutConfig)
	system.PUT("/timeout-config", deps.systemHandler.UpdateTimeoutConfig)
	system.GET("/cache-ttl", deps.systemHandler.GetCacheTTLConfig)
	system.PUT("/cache-ttl", deps.systemHandler.UpdateCacheTTLConfig)
	system.GET("/session-sticky-config", deps.systemHandler.GetSessionStickyConfig)
	system.PUT("/session-sticky-config", deps.systemHandler.UpdateSessionStickyConfig)
	system.GET("/session-sticky-runtime", deps.systemHandler.GetSessionStickyRuntime)
	system.GET("/billing-runtime-config", deps.systemHandler.GetBillingRuntimeConfig)
	system.GET("/billing-runtime-stats", deps.systemHandler.GetBillingRuntimeStats)
	system.PUT("/billing-runtime-config", deps.systemHandler.UpdateBillingRuntimeConfig)
	system.GET("/site-config", deps.systemHandler.GetSiteConfig)
	system.PUT("/site-config", deps.systemHandler.UpdateSiteConfig)
}
