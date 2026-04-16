package handler

import (
	"errors"
	"net/http"

	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type ChannelHandler struct {
	channelService *service.ChannelService
}

func NewChannelHandler() *ChannelHandler {
	return &ChannelHandler{
		channelService: service.NewChannelService(),
	}
}

func (h *ChannelHandler) List(c *gin.Context) {
	channels, err := h.channelService.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取渠道列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"channels": channels})
}

func (h *ChannelHandler) Get(c *gin.Context) {
	id := c.Param("id")

	channel, err := h.channelService.GetByID(id)
	if err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取渠道失败"})
		return
	}

	c.JSON(http.StatusOK, channel)
}

func (h *ChannelHandler) Create(c *gin.Context) {
	var req model.ChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	channel, err := h.channelService.Create(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建渠道失败"})
		return
	}

	c.JSON(http.StatusCreated, channel)
}

func (h *ChannelHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req model.ChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	channel, err := h.channelService.Update(id, &req)
	if err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新渠道失败"})
		return
	}

	c.JSON(http.StatusOK, channel)
}

func (h *ChannelHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	err := h.channelService.Delete(id)
	if err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除渠道失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "渠道已删除"})
}

func (h *ChannelHandler) SetEnabled(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	err := h.channelService.SetEnabled(id, req.Enabled)
	if err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新渠道状态失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "渠道状态已更新"})
}

func (h *ChannelHandler) TestConnection(c *gin.Context) {
	id := c.Param("id")

	var req model.TestChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.channelService.TestConnection(id, &req)
	if err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "测试连接失败"})
		return
	}

	c.JSON(http.StatusOK, result)
}
