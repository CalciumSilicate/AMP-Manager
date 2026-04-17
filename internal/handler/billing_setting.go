package handler

import (
	"errors"
	"net/http"

	"ampmanager/internal/middleware"
	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type BillingSettingHandler struct {
	billingService *service.BillingService
	settingService *service.BillingSettingService
	subService     *service.UserSubscriptionService
}

func NewBillingSettingHandler() *BillingSettingHandler {
	return &BillingSettingHandler{
		billingService: service.NewBillingService(),
		settingService: service.NewBillingSettingService(),
		subService:     service.NewUserSubscriptionService(),
	}
}

func (h *BillingSettingHandler) GetBillingState(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	state, err := h.billingService.GetBillingState(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取计费状态失败"})
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *BillingSettingHandler) UpdateBillingPriority(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var req model.UpdateBillingPriorityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	setting, err := h.settingService.Update(userID, &req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidBillingSource) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新计费优先级失败"})
		return
	}
	c.JSON(http.StatusOK, setting)
}

func (h *BillingSettingHandler) GetMySubscription(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	sub, err := h.subService.GetActive(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取订阅信息失败"})
		return
	}
	if sub == nil {
		c.JSON(http.StatusOK, gin.H{"subscription": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscription": sub})
}

func (h *BillingSettingHandler) ResetDailyBilling(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	state, err := h.billingService.ResetDailyBilling(userID)
	if err != nil {
		if errors.Is(err, service.ErrBillingDailyResetInvalidState) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置今日计费失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "今日计费已重置",
		"state":   state,
	})
}
