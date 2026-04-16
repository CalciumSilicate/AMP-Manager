package amp

import (
	"net/http/httputil"

	"ampmanager/internal/config"
	"ampmanager/internal/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterProxyRoutes(engine *gin.Engine, proxy *httputil.ReverseProxy) {
	cfg := config.Get()
	proxyLimiter := middleware.NewRateLimiter(cfg.RateLimitProxyRPS, 200)

	proxyHandler := ProxyHandler(proxy)
	channelHandler := ChannelProxyHandler()
	modelsHandler := ModelsHandler()

	// Register amp proxy routes at root level
	registerAmpProxyAPI(engine, proxyHandler, channelHandler, modelsHandler, proxyLimiter)

	// Register management routes (user, auth, threads, etc.) - proxied to ampcode.com
	registerManagementRoutes(engine, proxyHandler, proxyLimiter)
}

// ThreadRedirectHandler redirects /threads/T-xxx to official Amp threads
func ThreadRedirectHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		threadID := c.Param("threadID")
		officialURL := "https://ampcode.com/threads/" + threadID
		c.Redirect(302, officialURL)
	}
}

func createRoutingHandler(upstreamHandler, channelHandler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsNativeMode(c) {
			upstreamHandler(c)
			return
		}
		channelCfg := GetChannelConfig(c)
		if channelCfg != nil && channelCfg.Channel != nil {
			channelHandler(c)
			return
		}
		upstreamHandler(c)
	}
}

// createProviderHandler routes provider requests, with special handling for /models endpoints
func createProviderHandler(upstreamHandler, channelHandler, modelsHandler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsNativeMode(c) {
			upstreamHandler(c)
			return
		}

		path := c.Param("path")

		// Check if this is a models request - return local metadata
		if c.Request.Method == "GET" && isModelsEndpoint(path) {
			modelsHandler(c)
			return
		}

		// Otherwise use normal routing
		channelCfg := GetChannelConfig(c)
		if channelCfg != nil && channelCfg.Channel != nil {
			channelHandler(c)
			return
		}
		upstreamHandler(c)
	}
}

// isModelsEndpoint checks if path is a models list endpoint
func isModelsEndpoint(path string) bool {
	// /models, /v1/models, /v1beta/models
	return path == "/models" || path == "/v1/models" || path == "/v1beta/models"
}

// registerManagementRoutes registers Amp management proxy routes
// These routes proxy through to ampcode.com for OAuth, user management, threads, etc.
func registerManagementRoutes(engine *gin.Engine, proxyHandler gin.HandlerFunc, rateLimiter *middleware.RateLimiter) {
	// Management routes under /api/* - proxied to ampcode.com
	api := engine.Group("/api")
	api.Use(APIKeyAuthMiddleware())
	api.Use(rateLimiter.RateLimitByAPIKey())

	// User and auth management
	api.Any("/user", proxyHandler)
	api.Any("/user/*path", proxyHandler)
	api.Any("/auth", proxyHandler)
	api.Any("/auth/*path", proxyHandler)

	// Metadata and telemetry (ads blocked locally)
	api.Any("/meta", proxyHandler)
	api.Any("/meta/*path", proxyHandler)
	api.Any("/ads", BalanceAdMiddleware())
	api.Any("/telemetry", proxyHandler)
	api.Any("/telemetry/*path", proxyHandler)

	// Thread management
	api.Any("/threads", proxyHandler)
	api.Any("/threads/*path", proxyHandler)

	// OpenTelemetry and tab
	api.Any("/otel", proxyHandler)
	api.Any("/otel/*path", proxyHandler)
	api.Any("/tab", proxyHandler)
	api.Any("/tab/*path", proxyHandler)

	// Root-level routes that AMP CLI expects without /api prefix
	// These are public pages (threads, docs, etc.) that don't require API key auth
	publicRoutes := engine.Group("/")
	publicRoutes.Use(PublicProxyMiddleware())

	// Thread pages redirect to official ampcode.com for browser access
	publicRoutes.GET("/threads/:threadID", ThreadRedirectHandler())
	publicRoutes.GET("/threads", proxyHandler)
	publicRoutes.GET("/docs", proxyHandler)
	publicRoutes.GET("/docs/*path", proxyHandler)
	publicRoutes.GET("/settings", proxyHandler)
	publicRoutes.GET("/settings/*path", proxyHandler)
	publicRoutes.GET("/threads.rss", proxyHandler)
	publicRoutes.GET("/news.rss", proxyHandler)

	// Root-level auth routes for CLI login flow
	publicRoutes.Any("/auth", proxyHandler)
	publicRoutes.Any("/auth/*path", proxyHandler)
}

// registerAmpProxyAPI registers amp proxy routes at root /api/* level
// This is needed because amp CLI ignores URL path and sends requests directly to /api/*
func registerAmpProxyAPI(engine *gin.Engine, proxyHandler, channelHandler, modelsHandler gin.HandlerFunc, rateLimiter *middleware.RateLimiter) {
	api := engine.Group("/api")
	api.Use(APIKeyAuthMiddleware())
	api.Use(rateLimiter.RateLimitByAPIKey())
	api.Use(NativeModeSkipMiddleware(ApplyModelMappingMiddleware()))
	api.Use(NativeModeSkipMiddleware(ChannelRouterMiddleware()))
	api.Use(NativeModeSkipMiddleware(BillingEstimateMiddleware()))
	api.Use(NativeModeSkipMiddleware(BillingCheckMiddleware()))
	api.Use(NativeModeSkipMiddleware(RequestCaptureMiddleware()))

	api.Any("/internal", ProxyDisabledSkipMiddleware(BalanceAdMiddleware()), ProxyDisabledSkipMiddleware(DebugInternalAPIMiddleware()), ProxyDisabledSkipMiddleware(WebSearchStrategyMiddleware()), proxyHandler)
	api.Any("/internal/*path", ProxyDisabledSkipMiddleware(BalanceAdMiddleware()), ProxyDisabledSkipMiddleware(DebugInternalAPIMiddleware()), ProxyDisabledSkipMiddleware(WebSearchStrategyMiddleware()), proxyHandler)

	api.Any("/provider/:provider/*path", createProviderHandler(proxyHandler, channelHandler, modelsHandler))

	// Root level v1/v1beta routes for OpenAI/Anthropic/Gemini compatible endpoints
	v1 := engine.Group("/v1")
	v1.Use(APIKeyAuthMiddleware())
	v1.Use(rateLimiter.RateLimitByAPIKey())
	v1.Use(NativeModeSkipMiddleware(ApplyModelMappingMiddleware()))
	v1.Use(NativeModeSkipMiddleware(ChannelRouterMiddleware()))
	v1.Use(NativeModeSkipMiddleware(BillingEstimateMiddleware()))
	v1.Use(NativeModeSkipMiddleware(BillingCheckMiddleware()))
	v1.Use(NativeModeSkipMiddleware(RequestCaptureMiddleware()))

	v1.POST("/chat/completions", createRoutingHandler(proxyHandler, channelHandler))
	v1.POST("/completions", createRoutingHandler(proxyHandler, channelHandler))
	v1.POST("/messages", createRoutingHandler(proxyHandler, channelHandler))
	v1.GET("/responses", ResponsesWebsocketProxyHandler())
	v1.POST("/responses", createRoutingHandler(proxyHandler, channelHandler))

	v1beta := engine.Group("/v1beta")
	v1beta.Use(APIKeyAuthMiddleware())
	v1beta.Use(rateLimiter.RateLimitByAPIKey())
	v1beta.Use(NativeModeSkipMiddleware(ApplyModelMappingMiddleware()))
	v1beta.Use(NativeModeSkipMiddleware(ChannelRouterMiddleware()))
	v1beta.Use(NativeModeSkipMiddleware(BillingEstimateMiddleware()))
	v1beta.Use(NativeModeSkipMiddleware(BillingCheckMiddleware()))
	v1beta.Use(NativeModeSkipMiddleware(RequestCaptureMiddleware()))

	v1beta.POST("/models/*action", createRoutingHandler(proxyHandler, channelHandler))
	v1beta.GET("/models/*action", proxyHandler)

	// Models listing endpoints - no auth required
	engine.GET("/v1beta/models", createGeminiModelsHandler())
	engine.GET("/v1/models", createOpenAIModelsHandler())
}
