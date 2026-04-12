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

type ModelHandler struct {
	modelService   *service.ModelService
	channelService *service.ChannelService
	userRepo       *repository.UserRepository
}

func NewModelHandler() *ModelHandler {
	return &ModelHandler{
		modelService:   service.NewModelService(),
		channelService: service.NewChannelService(),
		userRepo:       repository.NewUserRepository(),
	}
}

func (h *ModelHandler) ListAvailableModels(c *gin.Context) {
	userID := middleware.GetUserID(c)
	user, err := h.userRepo.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户信息失败"})
		return
	}
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
		return
	}

	models, err := h.modelService.ListAvailableModelsForUser(user.ID, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型列表失败"})
		return
	}

	if models == nil {
		models = []*model.AvailableModel{}
	}

	c.JSON(http.StatusOK, gin.H{"models": models})
}

func (h *ModelHandler) FetchChannelModels(c *gin.Context) {
	channelID := c.Param("id")

	count, err := h.modelService.FetchAndSaveModels(channelID)
	if err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "模型获取成功",
		"count":   count,
	})
}

func (h *ModelHandler) GetChannelModels(c *gin.Context) {
	channelID := c.Param("id")

	models, err := h.modelService.GetModelsByChannelID(channelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型失败"})
		return
	}

	if models == nil {
		models = []*model.ChannelModel2{}
	}

	c.JSON(http.StatusOK, gin.H{"models": models})
}

func (h *ModelHandler) FetchAllModels(c *gin.Context) {
	results, err := h.modelService.FetchAllChannelsModels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "模型获取完成",
		"results": results,
	})
}
