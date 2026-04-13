package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

func parseSubscriptionExpiresAt(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("时间不能为空")
	}

	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), nil
	}

	for _, layout := range []string{
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
	} {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, errors.New("时间格式错误，应为 RFC3339 或 YYYY-MM-DDTHH:mm")
}

type SubscriptionHandler struct {
	planService *service.SubscriptionPlanService
	subService  *service.UserSubscriptionService
}

func NewSubscriptionHandler() *SubscriptionHandler {
	return &SubscriptionHandler{
		planService: service.NewSubscriptionPlanService(),
		subService:  service.NewUserSubscriptionService(),
	}
}

func (h *SubscriptionHandler) List(c *gin.Context) {
	plans, err := h.planService.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取套餐列表失败"})
		return
	}
	if plans == nil {
		plans = []*model.SubscriptionPlanResponse{}
	}
	c.JSON(http.StatusOK, gin.H{"plans": plans})
}

func (h *SubscriptionHandler) Create(c *gin.Context) {
	var req model.SubscriptionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	plan, err := h.planService.Create(&req)
	if err != nil {
		if errors.Is(err, service.ErrPlanNameRequired) ||
			errors.Is(err, service.ErrInvalidLimitType) ||
			errors.Is(err, service.ErrInvalidWindowMode) ||
			errors.Is(err, service.ErrDuplicateLimitType) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建套餐失败"})
		return
	}
	c.JSON(http.StatusCreated, plan)
}

func (h *SubscriptionHandler) Get(c *gin.Context) {
	id := c.Param("id")
	plan, err := h.planService.GetByID(id)
	if err != nil {
		if errors.Is(err, service.ErrPlanNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取套餐失败"})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func (h *SubscriptionHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req model.SubscriptionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	plan, err := h.planService.Update(id, &req)
	if err != nil {
		if errors.Is(err, service.ErrPlanNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrPlanNameRequired) ||
			errors.Is(err, service.ErrInvalidLimitType) ||
			errors.Is(err, service.ErrInvalidWindowMode) ||
			errors.Is(err, service.ErrDuplicateLimitType) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新套餐失败"})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func (h *SubscriptionHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	err := h.planService.Delete(id)
	if err != nil {
		if errors.Is(err, service.ErrPlanNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrPlanHasActiveSubs) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除套餐失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "套餐已删除"})
}

func (h *SubscriptionHandler) SetEnabled(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	err := h.planService.SetEnabled(id, req.Enabled)
	if err != nil {
		if errors.Is(err, service.ErrPlanNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新套餐状态失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "套餐状态已更新"})
}

func (h *SubscriptionHandler) AssignSubscription(c *gin.Context) {
	userID := c.Param("id")

	var req struct {
		PlanID    string  `json:"planId" binding:"required"`
		ExpiresAt *string `json:"expiresAt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && strings.TrimSpace(*req.ExpiresAt) != "" {
		parsed, err := parseSubscriptionExpiresAt(*req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
			return
		}
		expiresAt = &parsed
	}

	sub, err := h.subService.Assign(userID, &model.AssignSubscriptionRequest{
		PlanID:    req.PlanID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		if errors.Is(err, service.ErrPlanNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrPlanDisabled) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "分配订阅失败"})
		return
	}
	c.JSON(http.StatusCreated, sub)
}

func (h *SubscriptionHandler) CancelSubscription(c *gin.Context) {
	userID := c.Param("id")

	err := h.subService.Cancel(userID)
	if err != nil {
		if errors.Is(err, service.ErrNoActiveSubscription) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "取消订阅失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "订阅已取消"})
}

func (h *SubscriptionHandler) GetUserSubscription(c *gin.Context) {
	userID := c.Param("id")

	sub, err := h.subService.GetActive(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户订阅失败"})
		return
	}
	if sub == nil {
		c.JSON(http.StatusOK, gin.H{"subscription": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscription": sub})
}

func (h *SubscriptionHandler) UpdateSubscriptionExpiry(c *gin.Context) {
	userID := c.Param("id")

	var req struct {
		ExpiresAt string `json:"expiresAt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	expiresAt, err := parseSubscriptionExpiresAt(req.ExpiresAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	err = h.subService.UpdateExpiry(userID, expiresAt)
	if err != nil {
		if errors.Is(err, service.ErrNoActiveSubscription) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新订阅到期时间失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "订阅到期时间已更新"})
}
