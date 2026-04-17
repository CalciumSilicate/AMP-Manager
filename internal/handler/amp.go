package handler

import (
	"errors"
	"net/http"

	ampproxy "ampmanager/internal/amp"
	"ampmanager/internal/middleware"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type AmpHandler struct {
	ampService *service.AmpService
}

func NewAmpHandler() *AmpHandler {
	return &AmpHandler{
		ampService: service.NewAmpService(),
	}
}

func (h *AmpHandler) GetSettings(c *gin.Context) {
	canAccessRouteSettings, canAccessUpstreamSettings, err := loadAmpSettingsPermissions(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}
	if !canAccessRouteSettings && !canAccessUpstreamSettings {
		c.JSON(http.StatusForbidden, gin.H{"error": "当前无权访问 Amp 设置"})
		return
	}

	userID := middleware.GetUserID(c)

	settings, err := h.ampService.GetSettings(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	c.JSON(http.StatusOK, sanitizeAmpSettingsResponse(settings, canAccessRouteSettings, canAccessUpstreamSettings))
}

func (h *AmpHandler) UpdateSettings(c *gin.Context) {
	canAccessRouteSettings, canAccessUpstreamSettings, err := loadAmpSettingsPermissions(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}
	if !canAccessRouteSettings && !canAccessUpstreamSettings {
		c.JSON(http.StatusForbidden, gin.H{"error": "当前无权修改 Amp 设置"})
		return
	}

	userID := middleware.GetUserID(c)

	var req model.AmpSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}
	if containsRouteSettingsUpdate(&req) && !canAccessRouteSettings {
		c.JSON(http.StatusForbidden, gin.H{"error": "当前无权修改路由设置"})
		return
	}
	if containsUpstreamSettingsUpdate(&req) && !canAccessUpstreamSettings {
		c.JSON(http.StatusForbidden, gin.H{"error": "当前无权修改 Amp 设置"})
		return
	}

	settings, err := h.ampService.UpdateSettings(userID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新配置失败"})
		return
	}

	c.JSON(http.StatusOK, sanitizeAmpSettingsResponse(settings, canAccessRouteSettings, canAccessUpstreamSettings))
}

func (h *AmpHandler) TestConnection(c *gin.Context) {
	_, canAccessUpstreamSettings, err := loadAmpSettingsPermissions(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}
	if !canAccessUpstreamSettings {
		c.JSON(http.StatusForbidden, gin.H{"error": "当前无权测试 Amp 设置"})
		return
	}

	userID := middleware.GetUserID(c)

	result, err := h.ampService.TestConnection(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "测试连接失败"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func loadIsAdmin(c *gin.Context) (bool, error) {
	if middleware.IsAdmin(c) {
		return true, nil
	}

	user, err := repository.NewUserRepository().GetByID(middleware.GetUserID(c))
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, errors.New("user not found")
	}
	return user.IsAdmin, nil
}

func loadAmpSettingsPermissions(c *gin.Context) (bool, bool, error) {
	isAdmin, err := loadIsAdmin(c)
	if err != nil {
		return false, false, err
	}
	cfgSvc := service.NewSystemConfigService()
	canAccessRouteSettings, err := cfgSvc.CanAccessAmpSettings(isAdmin)
	if err != nil {
		return false, false, err
	}
	canAccessUpstreamSettings, err := cfgSvc.CanAccessAmpUpstreamSettings(isAdmin)
	if err != nil {
		return false, false, err
	}
	return canAccessRouteSettings, canAccessUpstreamSettings, nil
}

func containsRouteSettingsUpdate(req *model.AmpSettingsRequest) bool {
	return req != nil && (req.ModelMappings != nil || req.RouteMappingsEnabled != nil)
}

func containsUpstreamSettingsUpdate(req *model.AmpSettingsRequest) bool {
	return req != nil && (req.UpstreamURL != nil ||
		req.UpstreamAPIKey != nil ||
		req.Enabled != nil ||
		req.WebSearchMode != nil ||
		req.NativeMode != nil ||
		req.ShowBalanceInAd != nil ||
		req.Socks5Proxy != nil)
}

func sanitizeAmpSettingsResponse(resp *model.AmpSettingsResponse, canAccessRouteSettings, canAccessUpstreamSettings bool) *model.AmpSettingsResponse {
	if resp == nil {
		return nil
	}
	sanitized := *resp
	if !canAccessRouteSettings {
		sanitized.ModelMappings = []model.ModelMapping{}
		sanitized.RouteMappingsEnabled = false
	}
	if !canAccessUpstreamSettings {
		sanitized.UpstreamURL = ""
		sanitized.Enabled = false
		sanitized.HasAPIKey = false
		sanitized.WebSearchMode = model.WebSearchModeUpstream
		sanitized.NativeMode = false
		sanitized.ShowBalanceInAd = false
		sanitized.HasSocks5Proxy = false
	}
	return &sanitized
}

func (h *AmpHandler) ListAPIKeys(c *gin.Context) {
	userID := middleware.GetUserID(c)

	keys, err := h.ampService.ListAPIKeys(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 API Key 列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"apiKeys": keys})
}

func (h *AmpHandler) CreateAPIKey(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req model.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	key, err := h.ampService.CreateAPIKey(userID, &req)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "创建 API Key 失败"

		if errors.Is(err, service.ErrInvalidAPIKeyFormat) {
			status = http.StatusBadRequest
			msg = err.Error()
		} else if errors.Is(err, service.ErrDuplicateAPIKey) {
			status = http.StatusConflict
			msg = err.Error()
		}

		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusCreated, key)
}

func (h *AmpHandler) DeleteAPIKey(c *gin.Context) {
	userID := middleware.GetUserID(c)
	keyID := c.Param("id")

	err := h.ampService.DeleteAPIKey(userID, keyID)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "删除 API Key 失败"

		if errors.Is(err, service.ErrAPIKeyNotFound) {
			status = http.StatusNotFound
			msg = err.Error()
		} else if errors.Is(err, service.ErrNotOwner) {
			status = http.StatusForbidden
			msg = err.Error()
		}

		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API Key 已删除"})
}

func (h *AmpHandler) UpdateAPIKey(c *gin.Context) {
	userID := middleware.GetUserID(c)
	keyID := c.Param("id")

	var req model.UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	key, err := h.ampService.UpdateAPIKey(userID, keyID, &req)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "更新 API Key 失败"

		if errors.Is(err, service.ErrAPIKeyNotFound) {
			status = http.StatusNotFound
			msg = err.Error()
		} else if errors.Is(err, service.ErrNotOwner) {
			status = http.StatusForbidden
			msg = err.Error()
		}

		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, key)
}

func (h *AmpHandler) GetAPIKey(c *gin.Context) {
	userID := middleware.GetUserID(c)
	keyID := c.Param("id")

	key, err := h.ampService.GetAPIKey(userID, keyID)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "获取 API Key 失败"

		if errors.Is(err, service.ErrAPIKeyNotFound) {
			status = http.StatusNotFound
			msg = err.Error()
		} else if errors.Is(err, service.ErrNotOwner) {
			status = http.StatusForbidden
			msg = err.Error()
		} else if errors.Is(err, service.ErrAPIKeyNotRetrievable) {
			status = http.StatusGone
			msg = err.Error()
		}

		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, key)
}

func (h *AmpHandler) UpdateAPIKeyStatus(c *gin.Context) {
	userID := middleware.GetUserID(c)
	keyID := c.Param("id")

	var req model.UpdateAPIKeyStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	key, err := h.ampService.SetAPIKeyDisabled(userID, keyID, req.Disabled)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "更新 API Key 状态失败"

		if errors.Is(err, service.ErrAPIKeyNotFound) {
			status = http.StatusNotFound
			msg = err.Error()
		} else if errors.Is(err, service.ErrNotOwner) {
			status = http.StatusForbidden
			msg = err.Error()
		}

		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, key)
}

func (h *AmpHandler) AdminListUserAPIKeys(c *gin.Context) {
	userID := c.Param("id")

	keys, err := h.ampService.ListAPIKeysForAdmin(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 API Key 列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"apiKeys": keys})
}

func (h *AmpHandler) AdminUpdateUserAPIKey(c *gin.Context) {
	userID := c.Param("id")
	keyID := c.Param("keyId")

	var req model.UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	key, err := h.ampService.UpdateAPIKeyForAdmin(userID, keyID, &req)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "更新 API Key 失败"
		if errors.Is(err, service.ErrAPIKeyNotFound) {
			status = http.StatusNotFound
			msg = err.Error()
		} else if errors.Is(err, service.ErrNotOwner) {
			status = http.StatusBadRequest
			msg = "API Key 不属于当前用户"
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, key)
}

func (h *AmpHandler) AdminDeleteUserAPIKey(c *gin.Context) {
	userID := c.Param("id")
	keyID := c.Param("keyId")

	if err := h.ampService.DeleteAPIKeyForAdmin(userID, keyID); err != nil {
		status := http.StatusInternalServerError
		msg := "删除 API Key 失败"
		if errors.Is(err, service.ErrAPIKeyNotFound) {
			status = http.StatusNotFound
			msg = err.Error()
		} else if errors.Is(err, service.ErrNotOwner) {
			status = http.StatusBadRequest
			msg = "API Key 不属于当前用户"
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API Key 已删除"})
}

func (h *AmpHandler) AdminUpdateUserAPIKeyStatus(c *gin.Context) {
	userID := c.Param("id")
	keyID := c.Param("keyId")

	var req model.UpdateAPIKeyStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	key, err := h.ampService.SetAPIKeyDisabledForAdmin(userID, keyID, req.Disabled)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "更新 API Key 状态失败"
		if errors.Is(err, service.ErrAPIKeyNotFound) {
			status = http.StatusNotFound
			msg = err.Error()
		} else if errors.Is(err, service.ErrNotOwner) {
			status = http.StatusBadRequest
			msg = "API Key 不属于当前用户"
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, key)
}

func (h *AmpHandler) GetBootstrap(c *gin.Context) {
	userID := middleware.GetUserID(c)

	bootstrap, err := h.ampService.GetBootstrap(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取引导信息失败"})
		return
	}

	c.JSON(http.StatusOK, bootstrap)
}

func (h *AmpHandler) GetAPIUsage(c *gin.Context) {
	proxyCfg := ampproxy.GetProxyConfig(c.Request.Context())
	if proxyCfg == nil || proxyCfg.UserID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	items, err := h.ampService.GetClientUsage(proxyCfg.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用量失败"})
		return
	}

	c.JSON(http.StatusOK, items)
}
