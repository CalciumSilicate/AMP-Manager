package handler

import (
	"net/http"

	"ampmanager/internal/amp"
	"ampmanager/internal/invalidation"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	"github.com/gin-gonic/gin"
)

type ModelMetadataHandler struct {
	repo *repository.ModelMetadataRepository
}

func NewModelMetadataHandler() *ModelMetadataHandler {
	return &ModelMetadataHandler{
		repo: repository.NewModelMetadataRepository(),
	}
}

func (h *ModelMetadataHandler) List(c *gin.Context) {
	list, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型元数据列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"metadata": list})
}

func (h *ModelMetadataHandler) Get(c *gin.Context) {
	id := c.Param("id")

	meta, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型元数据失败"})
		return
	}
	if meta == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "模型元数据不存在"})
		return
	}

	c.JSON(http.StatusOK, meta)
}

func (h *ModelMetadataHandler) Create(c *gin.Context) {
	var req model.ModelMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	existing, _ := h.repo.GetByPattern(req.ModelPattern)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "模型模式已存在"})
		return
	}

	meta := &model.ModelMetadata{
		ModelPattern:        req.ModelPattern,
		DisplayName:         req.DisplayName,
		ContextLength:       req.ContextLength,
		MaxCompletionTokens: req.MaxCompletionTokens,
		Provider:            req.Provider,
	}

	if err := h.repo.Create(meta); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建模型元数据失败"})
		return
	}
	amp.InvalidateModelMetadataCache()
	invalidation.Publish(invalidation.ChannelModelMetadataUpdated)

	c.JSON(http.StatusCreated, meta)
}

func (h *ModelMetadataHandler) Update(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型元数据失败"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "模型元数据不存在"})
		return
	}

	var req model.ModelMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	if req.ModelPattern != existing.ModelPattern {
		duplicate, _ := h.repo.GetByPattern(req.ModelPattern)
		if duplicate != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "模型模式已存在"})
			return
		}
	}

	existing.ModelPattern = req.ModelPattern
	existing.DisplayName = req.DisplayName
	existing.ContextLength = req.ContextLength
	existing.MaxCompletionTokens = req.MaxCompletionTokens
	existing.Provider = req.Provider

	if err := h.repo.Update(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新模型元数据失败"})
		return
	}
	amp.InvalidateModelMetadataCache()
	invalidation.Publish(invalidation.ChannelModelMetadataUpdated)

	c.JSON(http.StatusOK, existing)
}

func (h *ModelMetadataHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取模型元数据失败"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "模型元数据不存在"})
		return
	}

	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除模型元数据失败"})
		return
	}
	amp.InvalidateModelMetadataCache()
	invalidation.Publish(invalidation.ChannelModelMetadataUpdated)

	c.JSON(http.StatusOK, gin.H{"message": "模型元数据已删除"})
}
