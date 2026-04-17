package handler

import (
	"net/http"

	"ampmanager/internal/amp"
	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *SystemHandler) ListRequestFilters(c *gin.Context) {
	items, err := service.NewRequestFilterService().List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取请求过滤失败"})
		return
	}
	c.JSON(http.StatusOK, model.RequestFilterListResponse{Filters: items})
}

func (h *SystemHandler) CreateRequestFilter(c *gin.Context) {
	var req model.RequestFilterCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	item, err := service.NewRequestFilterService().Create(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"filter": item})
}

func (h *SystemHandler) UpdateRequestFilter(c *gin.Context) {
	var req model.RequestFilterUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	item, err := service.NewRequestFilterService().Update(c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"filter": item})
}

func (h *SystemHandler) DeleteRequestFilter(c *gin.Context) {
	if err := service.NewRequestFilterService().Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "请求过滤已删除"})
}

func (h *SystemHandler) RefreshRequestFilters(c *gin.Context) {
	if err := amp.ReloadRequestFilters(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "请求过滤缓存已刷新"})
}

func (h *SystemHandler) GetRequestFilterBindings(c *gin.Context) {
	channels, err := service.NewChannelService().List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取渠道失败"})
		return
	}
	groups, err := service.NewGroupService().List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取分组失败"})
		return
	}
	resp := model.RequestFilterBindingsResponse{
		Channels: make([]model.RequestFilterBindingOption, 0, len(channels)),
		Groups:   make([]model.RequestFilterBindingOption, 0, len(groups)),
	}
	for _, item := range channels {
		resp.Channels = append(resp.Channels, model.RequestFilterBindingOption{ID: item.ID, Name: item.Name})
	}
	for _, item := range groups {
		resp.Groups = append(resp.Groups, model.RequestFilterBindingOption{ID: item.ID, Name: item.Name})
	}
	c.JSON(http.StatusOK, resp)
}
