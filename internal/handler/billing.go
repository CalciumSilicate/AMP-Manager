package handler

import (
	"net/http"

	"ampmanager/internal/billing"
	"ampmanager/internal/invalidation"
	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type BillingHandler struct{}

func NewBillingHandler() *BillingHandler {
	return &BillingHandler{}
}

// ListPrices 获取模型价格列表
func (h *BillingHandler) ListPrices(c *gin.Context) {
	store := billing.GetPriceStore()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "价格服务未初始化"})
		return
	}

	prices := store.ListPrices()

	// 转换为前端友好的格式
	result := make([]gin.H, 0, len(prices))
	for _, p := range prices {
		result = append(result, gin.H{
			"model":                  p.Model,
			"provider":               p.Provider,
			"source":                 p.Source,
			"inputCostPerToken":      p.PriceData.InputCostPerToken,
			"outputCostPerToken":     p.PriceData.OutputCostPerToken,
			"cacheReadInputPerToken": p.PriceData.CacheReadInputPerToken,
			"cacheCreationPerToken":  p.PriceData.CacheCreationPerToken,
			"updatedAt":              p.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items": result,
		"total": len(result),
	})
}

// GetPriceStats 获取价格服务状态
func (h *BillingHandler) GetPriceStats(c *gin.Context) {
	store := billing.GetPriceStore()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "价格服务未初始化"})
		return
	}

	count, source, fetchedAt := store.GetStats()
	c.JSON(http.StatusOK, gin.H{
		"modelCount": count,
		"source":     source,
		"fetchedAt":  fetchedAt,
	})
}

// RefreshPrices 手动刷新价格表
func (h *BillingHandler) RefreshPrices(c *gin.Context) {
	store := billing.GetPriceStore()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "价格服务未初始化"})
		return
	}

	if err := store.FetchFromLiteLLM(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "刷新价格表失败"})
		return
	}

	count, _, fetchedAt := store.GetStats()
	c.JSON(http.StatusOK, gin.H{
		"message":    "价格表刷新成功",
		"modelCount": count,
		"fetchedAt":  fetchedAt,
	})
}

func (h *BillingHandler) ListContextRules(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 model 参数"})
		return
	}

	rules, err := service.NewModelPriceContextRuleService().List(modelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rules})
}

func (h *BillingHandler) UpdateContextRules(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 model 参数"})
		return
	}

	var req model.UpdateModelPriceContextRulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	rules, err := service.NewModelPriceContextRuleService().Replace(modelName, req.Rules)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	invalidation.Publish(invalidation.ChannelPriceStoreUpdated)
	c.JSON(http.StatusOK, gin.H{"items": rules})
}
