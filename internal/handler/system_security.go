package handler

import (
	"errors"
	"net/http"

	"ampmanager/internal/invalidation"
	"ampmanager/internal/middleware"
	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *SystemHandler) GetManagementAPIKeyStatus(c *gin.Context) {
	status, err := service.NewAdminManagementKeyService().GetStatus(middleware.GetUserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取管理 API Key 状态失败"})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *SystemHandler) CreateManagementAPIKey(c *gin.Context) {
	resp, err := service.NewAdminManagementKeyService().Create(middleware.GetUserID(c))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrManagementAPIKeyAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建管理 API Key 失败"})
		}
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *SystemHandler) RevealManagementAPIKey(c *gin.Context) {
	var req model.ManagementAPIKeyPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	resp, err := service.NewAdminManagementKeyService().Reveal(middleware.GetUserID(c), req.CurrentPassword)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrManagementAPIKeyNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrManagementAPIKeyPasswordInvalid):
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrManagementAPIKeyPasswordRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查看管理 API Key 失败"})
		}
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SystemHandler) RotateManagementAPIKey(c *gin.Context) {
	var req model.ManagementAPIKeyPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	resp, err := service.NewAdminManagementKeyService().Rotate(middleware.GetUserID(c), req.CurrentPassword)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrManagementAPIKeyNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrManagementAPIKeyPasswordInvalid):
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrManagementAPIKeyPasswordRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "轮换管理 API Key 失败"})
		}
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SystemHandler) UpdateManagementAPIKeyEnabled(c *gin.Context) {
	var req model.ManagementAPIKeyEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	status, err := service.NewAdminManagementKeyService().SetEnabled(middleware.GetUserID(c), req.Enabled)
	if err != nil {
		if errors.Is(err, service.ErrManagementAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新管理 API Key 状态失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "管理 API Key 状态已更新", "status": status})
}

func (h *SystemHandler) GetUserPanelRateLimitConfig(c *gin.Context) {
	cfg, err := service.NewSystemConfigService().GetUserPanelRateLimitConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取普通用户面板限流配置失败"})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *SystemHandler) UpdateUserPanelRateLimitConfig(c *gin.Context) {
	var req model.UserPanelRateLimitConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	cfg, err := service.NewSystemConfigService().SetUserPanelRateLimitConfig(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存普通用户面板限流配置失败"})
		return
	}
	middleware.UpdateUserPanelRateLimitConfigRuntime(cfg)
	invalidation.Publish(invalidation.ChannelUserPanelRateLimitUpdated)
	c.JSON(http.StatusOK, gin.H{"message": "普通用户面板限流配置已更新", "config": cfg})
}
