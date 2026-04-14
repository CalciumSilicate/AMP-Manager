package handler

import (
	"errors"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"

	"ampmanager/internal/middleware"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type PurchaseHandler struct {
	purchaseService *service.PurchaseService
}

func NewPurchaseHandler() *PurchaseHandler {
	return &PurchaseHandler{
		purchaseService: service.NewPurchaseService(),
	}
}

func (h *PurchaseHandler) GetCatalog(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	catalog, err := h.purchaseService.GetCatalog(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取售卖商品失败"})
		return
	}
	c.JSON(http.StatusOK, catalog)
}

func (h *PurchaseHandler) CreateOrder(c *gin.Context) {
	userID := middleware.GetUserID(c)
	username := middleware.GetUsername(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var req model.CreatePurchaseOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	order, err := h.purchaseService.CreateOrder(c.Request.Context(), userID, username, req.ProductID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPurchaseDisabled),
			errors.Is(err, service.ErrPaymentUnavailable),
			errors.Is(err, service.ErrPurchaseProductDisabled),
			errors.Is(err, service.ErrDifferentPlanActive),
			errors.Is(err, service.ErrPermanentSubscription):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case errors.Is(err, repository.ErrPurchaseProductNotFound),
			errors.Is(err, service.ErrPlanNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, order)
}

func (h *PurchaseHandler) ListMyOrders(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	orders, err := h.purchaseService.ListOrdersForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取订单列表失败"})
		return
	}
	c.JSON(http.StatusOK, orders)
}

func (h *PurchaseHandler) GetMyOrder(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	order, err := h.purchaseService.GetOrderForUser(userID, c.Param("orderNo"))
	if err != nil {
		if errors.Is(err, service.ErrPurchaseOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取订单失败"})
		return
	}
	c.JSON(http.StatusOK, order)
}

func (h *PurchaseHandler) RefreshMyOrder(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	order, err := h.purchaseService.RefreshOrderForUser(c.Request.Context(), userID, c.Param("orderNo"))
	if err != nil {
		if errors.Is(err, service.ErrPurchaseOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, order)
}

func (h *PurchaseHandler) GetSettings(c *gin.Context) {
	settings, err := h.purchaseService.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取支付配置失败"})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *PurchaseHandler) UpdateSettings(c *gin.Context) {
	var req model.PurchaseSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	if req.AlipayEnvironment == "" {
		req.AlipayEnvironment = model.AlipayEnvironmentSandbox
	}
	if notifyURL := strings.TrimSpace(req.AlipayNotifyURL); notifyURL != "" {
		if _, err := neturl.ParseRequestURI(notifyURL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "支付宝回调地址无效"})
			return
		}
	}

	settings, err := h.purchaseService.UpdateSettings(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存支付配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "支付配置已更新", "settings": settings})
}

func (h *PurchaseHandler) ListProductsAdmin(c *gin.Context) {
	products, err := h.purchaseService.ListProductsAdmin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取售卖商品失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"products": products})
}

func (h *PurchaseHandler) CreateProduct(c *gin.Context) {
	var req model.PurchaseProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	product, err := h.purchaseService.CreateProduct(&req)
	if err != nil {
		if errors.Is(err, service.ErrPlanNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建售卖商品失败"})
		return
	}
	c.JSON(http.StatusCreated, product)
}

func (h *PurchaseHandler) UpdateProduct(c *gin.Context) {
	var req model.PurchaseProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误", "details": err.Error()})
		return
	}

	product, err := h.purchaseService.UpdateProduct(c.Param("id"), &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPlanNotFound), errors.Is(err, repository.ErrPurchaseProductNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新售卖商品失败"})
		}
		return
	}
	c.JSON(http.StatusOK, product)
}

func (h *PurchaseHandler) DeleteProduct(c *gin.Context) {
	err := h.purchaseService.DeleteProduct(c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrPurchaseProductNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrPurchaseProductHasOrders):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "删除售卖商品失败"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "售卖商品已删除"})
}

func (h *PurchaseHandler) SetProductEnabled(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.purchaseService.SetProductEnabled(c.Param("id"), req.Enabled); err != nil {
		if errors.Is(err, repository.ErrPurchaseProductNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新售卖商品状态失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "售卖商品状态已更新"})
}

func (h *PurchaseHandler) ListOrdersAdmin(c *gin.Context) {
	filters := model.PurchaseOrderFilters{
		PaymentStatus:     model.PurchasePaymentStatus(strings.TrimSpace(c.Query("paymentStatus"))),
		FulfillmentStatus: model.PurchaseFulfillmentStatus(strings.TrimSpace(c.Query("fulfillmentStatus"))),
		Username:          strings.TrimSpace(c.Query("username")),
		ProductID:         strings.TrimSpace(c.Query("productId")),
		Limit:             100,
	}
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 && parsed <= 500 {
			filters.Limit = parsed
		}
	}

	orders, err := h.purchaseService.ListOrdersAdmin(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取订单列表失败"})
		return
	}
	c.JSON(http.StatusOK, orders)
}

func (h *PurchaseHandler) RefreshOrderAdmin(c *gin.Context) {
	order, err := h.purchaseService.RefreshOrderAdmin(c.Request.Context(), c.Param("orderNo"))
	if err != nil {
		if errors.Is(err, service.ErrPurchaseOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, order)
}

func (h *PurchaseHandler) AlipayNotify(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "解析支付宝回调失败"})
		return
	}

	if _, err := h.purchaseService.HandleNotification(c.Request.Context(), c.Request.PostForm); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.String(http.StatusOK, "success")
}
