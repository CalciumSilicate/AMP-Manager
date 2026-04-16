package handler

import (
	"errors"
	"net/http"

	"ampmanager/internal/middleware"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type AnnouncementHandler struct {
	service *service.AnnouncementService
}

func NewAnnouncementHandler() *AnnouncementHandler {
	return &AnnouncementHandler{
		service: service.NewAnnouncementService(),
	}
}

func (h *AnnouncementHandler) ListPublic(c *gin.Context) {
	items, err := h.service.ListPublic()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取公告失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"announcements": items})
}

func (h *AnnouncementHandler) ListForMe(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	items, err := h.service.ListForUser(userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取公告失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"announcements": items})
}

func (h *AnnouncementHandler) MarkRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	if err := h.service.MarkRead(userID, c.Param("id")); err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			return
		}
		if errors.Is(err, service.ErrAnnouncementNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "标记已读失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "公告已标记为已读"})
}

func (h *AnnouncementHandler) ListAdmin(c *gin.Context) {
	items, err := h.service.ListAdmin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取公告失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"announcements": items})
}

func (h *AnnouncementHandler) Create(c *gin.Context) {
	var req model.AnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	item, err := h.service.Create(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建公告失败"})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *AnnouncementHandler) Update(c *gin.Context) {
	var req model.AnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	item, err := h.service.Update(c.Param("id"), &req)
	if err != nil {
		if errors.Is(err, service.ErrAnnouncementNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新公告失败"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *AnnouncementHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Param("id")); err != nil {
		if errors.Is(err, service.ErrAnnouncementNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除公告失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "公告已删除"})
}
