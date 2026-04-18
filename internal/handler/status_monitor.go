package handler

import (
	"context"
	"net/http"
	"strings"

	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type StatusMonitorHandler struct {
	service *service.StatusMonitorService
}

func NewStatusMonitorHandler() *StatusMonitorHandler {
	return &StatusMonitorHandler{
		service: service.NewStatusMonitorService(),
	}
}

func (h *StatusMonitorHandler) GetDashboard(c *gin.Context) {
	hasConfiguredMonitors, err := h.service.HasConfiguredMonitors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取状态监控面板失败"})
		return
	}
	if !hasConfiguredMonitors {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "状态监控未配置，请先在系统设置中创建至少一个状态监控项"})
		return
	}

	data, err := h.service.GetDashboard(c.DefaultQuery("period", "7d"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取状态监控面板失败"})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *StatusMonitorHandler) GetRuntimeConfig(c *gin.Context) {
	cfg, err := h.service.GetRuntimeConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取状态监控配置失败"})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *StatusMonitorHandler) UpdateRuntimeConfig(c *gin.Context) {
	var req model.StatusMonitorRuntimeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	cfg, err := h.service.UpdateRuntimeConfig(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "状态监控运行时配置已更新", "config": cfg})
}

func (h *StatusMonitorHandler) ListMonitors(c *gin.Context) {
	items, err := h.service.ListMonitors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取状态监控项失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *StatusMonitorHandler) CreateMonitor(c *gin.Context) {
	var req model.StatusMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	item, err := h.service.CreateMonitor(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *StatusMonitorHandler) UpdateMonitor(c *gin.Context) {
	var req model.StatusMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	item, err := h.service.UpdateMonitor(c.Param("id"), &req)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err == service.ErrStatusMonitorNotFound {
			statusCode = http.StatusNotFound
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *StatusMonitorHandler) DeleteMonitor(c *gin.Context) {
	err := h.service.DeleteMonitor(c.Param("id"))
	if err != nil {
		statusCode := http.StatusBadRequest
		if err == service.ErrStatusMonitorNotFound {
			statusCode = http.StatusNotFound
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "状态监控项已删除"})
}

func (h *StatusMonitorHandler) RunMonitor(c *gin.Context) {
	result, err := h.service.RunMonitor(context.Background(), c.Param("id"))
	if err != nil {
		statusCode := http.StatusBadRequest
		if err == service.ErrStatusMonitorNotFound {
			statusCode = http.StatusNotFound
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "状态监控探测完成",
		"result": gin.H{
			"status":         result.Status,
			"latencyMs":      result.LatencyMs,
			"ttfbMs":         result.TTFBMs,
			"httpStatusCode": result.HTTPStatusCode,
			"message":        result.Message,
			"endpointLabel":  result.EndpointLabel,
			"checkedAt":      result.CheckedAt,
		},
	})
}

func (h *StatusMonitorHandler) RunAll(c *gin.Context) {
	count, err := h.service.RunAllEnabledMonitors(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	message := "已完成状态监控全量探测"
	if count == 0 {
		message = "当前没有启用中的状态监控项"
	}
	c.JSON(http.StatusOK, gin.H{"message": message, "count": count})
}

func isStatusMonitorBadRequest(err error) bool {
	if err == nil {
		return false
	}
	return !strings.Contains(err.Error(), "内部")
}
