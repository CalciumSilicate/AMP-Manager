package router

import (
	"net/http"
	"os"
	"strings"

	"ampmanager/internal/amp"
	"ampmanager/internal/config"
	"ampmanager/internal/handler"
	"ampmanager/internal/middleware"
	"ampmanager/internal/service"
	"ampmanager/internal/web"

	"github.com/gin-gonic/gin"
)

type routeDeps struct {
	cfg                   *config.Config
	authLimiter           *middleware.RateLimiter
	proxyLimiter          *middleware.RateLimiter
	userHandler           *handler.UserHandler
	ampHandler            *handler.AmpHandler
	requestLogHandler     *handler.RequestLogHandler
	channelHandler        *handler.ChannelHandler
	modelHandler          *handler.ModelHandler
	modelMetadataHandler  *handler.ModelMetadataHandler
	systemHandler         *handler.SystemHandler
	billingHandler        *handler.BillingHandler
	groupHandler          *handler.GroupHandler
	subscriptionHandler   *handler.SubscriptionHandler
	billingSettingHandler *handler.BillingSettingHandler
	purchaseHandler       *handler.PurchaseHandler
	redeemHandler         *handler.RedeemHandler
	inviteHandler         *handler.InviteHandler
	couponHandler         *handler.CouponHandler
	announcementHandler   *handler.AnnouncementHandler
	statusMonitorHandler  *handler.StatusMonitorHandler
	statusMonitorService  *service.StatusMonitorService
	sessionHandler        *handler.SessionHandler
}

func Setup() *gin.Engine {
	cfg := config.Get()
	role := cfg.Role()
	r := newEngine(cfg, role)
	deps := newRouteDeps(cfg)

	if role.RunsPanel() {
		registerPanelRoutes(r, deps)
		web.RegisterStaticRoutes(r)
	}
	if role.RunsProxy() {
		registerProxyOwnedRoutes(r, deps)
		proxy := amp.CreateDynamicReverseProxy()
		amp.RegisterProxyRoutes(r, proxy)
	}

	return r
}

func newEngine(cfg *config.Config, role config.ServerRole) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	if os.Getenv("AMP_ENABLE_ACCESS_LOG") == "true" {
		r.Use(gin.Logger())
	}

	allowedOrigins := make([]string, 0)
	if cfg != nil && cfg.CORSAllowedOrigins != "" {
		for _, origin := range strings.Split(cfg.CORSAllowedOrigins, ",") {
			if trimmed := strings.TrimSpace(origin); trimmed != "" && trimmed != "*" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	}
	corsEnabled := len(allowedOrigins) > 0

	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if corsEnabled && origin != "" {
			allowed := false
			for _, candidate := range allowedOrigins {
				if candidate == origin {
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

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"ok":   true,
			"role": role,
		})
	})

	return r
}

func newRouteDeps(cfg *config.Config) *routeDeps {
	return &routeDeps{
		cfg:                   cfg,
		authLimiter:           middleware.NewRateLimiter(cfg.RateLimitAuthRPS, 10),
		proxyLimiter:          middleware.NewRateLimiter(cfg.RateLimitProxyRPS, 200),
		userHandler:           handler.NewUserHandler(),
		ampHandler:            handler.NewAmpHandler(),
		requestLogHandler:     handler.NewRequestLogHandler(),
		channelHandler:        handler.NewChannelHandler(),
		modelHandler:          handler.NewModelHandler(),
		modelMetadataHandler:  handler.NewModelMetadataHandler(),
		systemHandler:         handler.NewSystemHandler(),
		billingHandler:        handler.NewBillingHandler(),
		groupHandler:          handler.NewGroupHandler(),
		subscriptionHandler:   handler.NewSubscriptionHandler(),
		billingSettingHandler: handler.NewBillingSettingHandler(),
		purchaseHandler:       handler.NewPurchaseHandler(),
		redeemHandler:         handler.NewRedeemHandler(),
		inviteHandler:         handler.NewInviteHandler(),
		couponHandler:         handler.NewCouponHandler(),
		announcementHandler:   handler.NewAnnouncementHandler(),
		statusMonitorHandler:  handler.NewStatusMonitorHandler(),
		statusMonitorService:  service.NewStatusMonitorService(),
		sessionHandler:        handler.NewSessionHandler(),
	}
}
