package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"ampmanager/internal/config"
	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	userPanelSectionOverviewStatus   = "overviewStatus"
	userPanelSectionAmpSettings      = "ampSettings"
	userPanelSectionAPIKeys          = "apiKeys"
	userPanelSectionRequestLogsUsage = "requestLogsUsage"
	userPanelSectionModels           = "models"
	userPanelSectionAccountPurchase  = "accountPurchase"
)

var userPanelRateLimitReserveScript = redis.NewScript(`
local burst = tonumber(ARGV[1])
local rps = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local max_wait = tonumber(ARGV[4])

if burst == nil or burst < 1 or rps == nil or rps <= 0 then
  return {1, 0}
end

local refill_per_ms = rps / 1000.0
local state = redis.call('HMGET', KEYS[1], 'tokens', 'updated_at')
local tokens = tonumber(state[1])
local updated_at = tonumber(state[2])

if tokens == nil then
  tokens = burst
end
if updated_at == nil then
  updated_at = now
end

if now > updated_at then
  tokens = math.min(burst, tokens + ((now - updated_at) * refill_per_ms))
  updated_at = now
end

local wait_ms = 0
if tokens >= 1 then
  tokens = tokens - 1
else
  wait_ms = math.ceil(((1 - tokens) / rps) * 1000)
  if wait_ms > max_wait then
    redis.call('HMSET', KEYS[1], 'tokens', tokens, 'updated_at', updated_at)
    redis.call('PEXPIRE', KEYS[1], math.max(max_wait + 60000, 60000))
    return {0, wait_ms}
  end
  tokens = tokens - 1
end

redis.call('HMSET', KEYS[1], 'tokens', tokens, 'updated_at', updated_at)
local ttl_ms = math.max(math.ceil(((burst - tokens) / rps) * 1000) + 60000, 60000)
redis.call('PEXPIRE', KEYS[1], ttl_ms)
return {1, wait_ms}
`)

type userPanelRateLimitRuntime struct {
	clientOnce sync.Once
	configOnce sync.Once
	client     *redis.Client
	clientErr  error

	configMu sync.RWMutex
	config   model.UserPanelRateLimitConfig
}

var globalUserPanelRateLimitRuntime = &userPanelRateLimitRuntime{
	config: serviceDefaultUserPanelRateLimitConfig(),
}

func UserPanelRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserID(c)
		if userID == "" || IsAdmin(c) {
			c.Next()
			return
		}

		section, ok := classifyUserPanelRateLimitSection(c.FullPath(), c.Request.URL.Path)
		if !ok {
			c.Next()
			return
		}

		cfg := globalUserPanelRateLimitRuntime.getConfig()
		if !cfg.Enabled {
			c.Next()
			return
		}

		sectionCfg := userPanelRateLimitSectionConfig(cfg, section)
		allowed, waitMs, err := globalUserPanelRateLimitRuntime.reserve(c.Request.Context(), userID, section, sectionCfg)
		if err != nil {
			log.Printf("user panel rate limit: reserve failed for %s/%s: %v", userID, section, err)
			c.Next()
			return
		}
		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":          "访问过快，请稍后再试",
				"section":        section,
				"retry_after_ms": waitMs,
				"waited_ms":      0,
			})
			c.Abort()
			return
		}
		if waitMs > 0 {
			timer := time.NewTimer(time.Duration(waitMs) * time.Millisecond)
			select {
			case <-c.Request.Context().Done():
				timer.Stop()
				c.Abort()
				return
			case <-timer.C:
			}
		}

		c.Next()
	}
}

func LoadUserPanelRateLimitConfigBestEffort(reason string) {
	cfg, err := service.NewSystemConfigService().GetUserPanelRateLimitConfig()
	if err != nil {
		log.Printf("user panel rate limit: load config failed (%s): %v", reason, err)
		globalUserPanelRateLimitRuntime.setConfig(serviceDefaultUserPanelRateLimitConfig())
		return
	}
	globalUserPanelRateLimitRuntime.setConfig(cfg)
}

func UpdateUserPanelRateLimitConfigRuntime(cfg model.UserPanelRateLimitConfig) {
	globalUserPanelRateLimitRuntime.setConfig(cfg)
}

func (r *userPanelRateLimitRuntime) getConfig() model.UserPanelRateLimitConfig {
	r.configOnce.Do(func() {
		LoadUserPanelRateLimitConfigBestEffort("lazy load")
	})
	r.configMu.RLock()
	defer r.configMu.RUnlock()
	return r.config
}

func (r *userPanelRateLimitRuntime) setConfig(cfg model.UserPanelRateLimitConfig) {
	r.configMu.Lock()
	defer r.configMu.Unlock()
	r.config = cfg
}

func (r *userPanelRateLimitRuntime) reserve(ctx context.Context, userID, section string, sectionCfg model.UserPanelRateLimitSectionConfig) (bool, int, error) {
	client, prefix, err := r.redisClient()
	if err != nil {
		if err == redis.Nil {
			return true, 0, nil
		}
		return true, 0, err
	}

	key := prefix + ":user_panel_rl:" + userID + ":" + section
	values, err := userPanelRateLimitReserveScript.Run(ctx, client, []string{key},
		sectionCfg.Burst,
		sectionCfg.RPS,
		time.Now().UnixMilli(),
		sectionCfg.MaxWaitMs,
	).Result()
	if err != nil {
		return true, 0, err
	}

	result, ok := values.([]interface{})
	if !ok || len(result) != 2 {
		return true, 0, nil
	}

	allowed := toInt64(result[0]) == 1
	waitMs := int(toInt64(result[1]))
	return allowed, waitMs, nil
}

func (r *userPanelRateLimitRuntime) redisClient() (*redis.Client, string, error) {
	r.clientOnce.Do(func() {
		cfg := config.Get()
		if cfg == nil || strings.TrimSpace(cfg.RedisURL) == "" {
			r.clientErr = redis.Nil
			return
		}
		options, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			r.clientErr = err
			return
		}
		r.client = redis.NewClient(options)
	})

	prefix := "ampmanager"
	if cfg := config.Get(); cfg != nil && strings.TrimSpace(cfg.RedisPrefix) != "" {
		prefix = strings.TrimSpace(cfg.RedisPrefix)
	}

	if r.clientErr != nil {
		return nil, prefix, r.clientErr
	}
	if r.client == nil {
		return nil, prefix, redis.Nil
	}
	return r.client, prefix, nil
}

func classifyUserPanelRateLimitSection(fullPath, rawPath string) (string, bool) {
	path := fullPath
	if path == "" {
		path = rawPath
	}

	switch {
	case path == "/api/me/dashboard",
		path == "/api/me/status/dashboard",
		strings.HasPrefix(path, "/api/me/announcements"):
		return userPanelSectionOverviewStatus, true
	case strings.HasPrefix(path, "/api/me/amp/settings"):
		return userPanelSectionAmpSettings, true
	case strings.HasPrefix(path, "/api/me/amp/api-keys"),
		path == "/api/me/amp/bootstrap":
		return userPanelSectionAPIKeys, true
	case strings.HasPrefix(path, "/api/me/amp/request-logs"),
		path == "/api/me/amp/usage/summary":
		return userPanelSectionRequestLogsUsage, true
	case path == "/api/models":
		return userPanelSectionModels, true
	case path == "/api/me/balance",
		strings.HasPrefix(path, "/api/me/billing/"),
		path == "/api/me/subscription",
		path == "/api/me/password",
		path == "/api/me/username",
		strings.HasPrefix(path, "/api/me/purchase/"),
		strings.HasPrefix(path, "/api/me/redeem"):
		return userPanelSectionAccountPurchase, true
	default:
		return "", false
	}
}

func userPanelRateLimitSectionConfig(cfg model.UserPanelRateLimitConfig, section string) model.UserPanelRateLimitSectionConfig {
	switch section {
	case userPanelSectionOverviewStatus:
		return cfg.Sections.OverviewStatus
	case userPanelSectionAmpSettings:
		return cfg.Sections.AmpSettings
	case userPanelSectionAPIKeys:
		return cfg.Sections.APIKeys
	case userPanelSectionRequestLogsUsage:
		return cfg.Sections.RequestLogsUsage
	case userPanelSectionModels:
		return cfg.Sections.Models
	default:
		return cfg.Sections.AccountPurchase
	}
}

func serviceDefaultUserPanelRateLimitConfig() model.UserPanelRateLimitConfig {
	return model.UserPanelRateLimitConfig{
		Enabled:  true,
		Backend:  "redis",
		FailOpen: true,
		Sections: model.UserPanelRateLimitSections{
			OverviewStatus:   model.UserPanelRateLimitSectionConfig{RPS: 1, Burst: 3, MaxWaitMs: 4500},
			AmpSettings:      model.UserPanelRateLimitSectionConfig{RPS: 0.5, Burst: 2, MaxWaitMs: 9000},
			APIKeys:          model.UserPanelRateLimitSectionConfig{RPS: 0.5, Burst: 2, MaxWaitMs: 9000},
			RequestLogsUsage: model.UserPanelRateLimitSectionConfig{RPS: 0.2, Burst: 1, MaxWaitMs: 15000},
			Models:           model.UserPanelRateLimitSectionConfig{RPS: 1, Burst: 2, MaxWaitMs: 3000},
			AccountPurchase:  model.UserPanelRateLimitSectionConfig{RPS: 0.5, Burst: 2, MaxWaitMs: 12000},
		},
	}
}

func toInt64(value interface{}) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}
