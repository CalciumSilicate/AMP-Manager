package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ampmanager/internal/middleware"
	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type RedeemHandler struct {
	redeemService *service.RedeemService
}

func NewRedeemHandler() *RedeemHandler {
	return &RedeemHandler{
		redeemService: service.NewRedeemService(),
	}
}

func (h *RedeemHandler) Redeem(c *gin.Context) {
	userID := middleware.GetUserID(c)
	username := middleware.GetUsername(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var req model.RedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	result, err := h.redeemService.Redeem(c.Request.Context(), userID, username, req.Code)
	if err != nil {
		h.writeRedeemError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *RedeemHandler) ListMyRecords(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	records, err := h.redeemService.ListUserRedemptions(userID, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取兑换记录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": records})
}

func (h *RedeemHandler) ListCampaigns(c *gin.Context) {
	items, err := h.redeemService.ListCampaigns()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取兑换活动失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *RedeemHandler) CreateCampaign(c *gin.Context) {
	var req model.RedeemCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	item, err := h.redeemService.CreateCampaign(&req)
	if err != nil {
		h.writeAdminRedeemError(c, err, "创建兑换活动失败")
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *RedeemHandler) UpdateCampaign(c *gin.Context) {
	var req model.RedeemCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	item, err := h.redeemService.UpdateCampaign(c.Param("id"), &req)
	if err != nil {
		h.writeAdminRedeemError(c, err, "更新兑换活动失败")
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *RedeemHandler) DeleteCampaign(c *gin.Context) {
	if err := h.redeemService.DeleteCampaign(c.Param("id")); err != nil {
		h.writeAdminRedeemError(c, err, "删除兑换活动失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "兑换活动已删除"})
}

func (h *RedeemHandler) CreateBatch(c *gin.Context) {
	var req model.RedeemCodeBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	batch, codes, err := h.redeemService.CreateBatch(c.Param("id"), &req)
	if err != nil {
		h.writeAdminRedeemError(c, err, "生成兑换批次失败")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"batch": batch, "codes": codes})
}

func (h *RedeemHandler) ListBatches(c *gin.Context) {
	items, err := h.redeemService.ListBatches(strings.TrimSpace(c.Query("campaignId")))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取批次列表失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *RedeemHandler) ExportBatch(c *gin.Context) {
	content, err := h.redeemService.ExportBatchCSV(c.Param("id"))
	if err != nil {
		h.writeAdminRedeemError(c, err, "导出批次失败")
		return
	}

	filename := fmt.Sprintf("redeem-batch-%s.csv", c.Param("id"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", content)
}

func (h *RedeemHandler) ListCodes(c *gin.Context) {
	limit := 200
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	items, err := h.redeemService.ListCodes(
		strings.TrimSpace(c.Query("campaignId")),
		strings.TrimSpace(c.Query("batchId")),
		strings.TrimSpace(c.Query("status")),
		strings.TrimSpace(c.Query("keyword")),
		limit,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取兑换码失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *RedeemHandler) SetCodeEnabled(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.redeemService.SetCodeEnabled(c.Param("id"), req.Enabled); err != nil {
		h.writeAdminRedeemError(c, err, "更新兑换码状态失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "兑换码状态已更新"})
}

func (h *RedeemHandler) ListRedemptions(c *gin.Context) {
	limit := 200
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	items, err := h.redeemService.ListRedemptions(
		strings.TrimSpace(c.Query("campaignId")),
		strings.TrimSpace(c.Query("status")),
		strings.TrimSpace(c.Query("username")),
		limit,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取兑换记录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *RedeemHandler) writeAdminRedeemError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrRedeemCampaignNotFound),
		errors.Is(err, service.ErrRedeemCodeNotFound),
		errors.Is(err, service.ErrRedeemBatchNotFound),
		errors.Is(err, service.ErrPlanNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrRedeemCampaignHasUsage),
		errors.Is(err, service.ErrRedeemSingleUseOnly),
		errors.Is(err, service.ErrRedeemSharedCodeLocked),
		errors.Is(err, service.ErrRedeemCodeConsumed):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrRedeemRewardRequired),
		errors.Is(err, service.ErrRedeemRewardInvalid),
		errors.Is(err, service.ErrRedeemSharedCodeRequired),
		errors.Is(err, service.ErrRedeemCodeModeImmutable):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}

func (h *RedeemHandler) writeRedeemError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrRedeemCodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrRedeemCampaignDisabled),
		errors.Is(err, service.ErrRedeemCodeDisabled),
		errors.Is(err, service.ErrRedeemCodeConsumed),
		errors.Is(err, service.ErrRedeemCampaignNotStarted),
		errors.Is(err, service.ErrRedeemCampaignEnded),
		errors.Is(err, service.ErrRedeemPerUserLimitReached),
		errors.Is(err, service.ErrRedeemCampaignLimitReached),
		errors.Is(err, service.ErrDifferentPlanActive),
		errors.Is(err, service.ErrPermanentSubscription):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "兑换失败"})
	}
}
