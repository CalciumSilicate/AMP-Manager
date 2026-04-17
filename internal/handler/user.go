package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ampmanager/internal/middleware"
	"ampmanager/internal/model"
	"ampmanager/internal/precision"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler() *UserHandler {
	return &UserHandler{
		userService: service.NewUserService(),
	}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	user, err := h.userService.Register(&req)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "注册失败"

		if errors.Is(err, service.ErrUsernameExists) {
			status = http.StatusConflict
			msg = err.Error()
		} else if errors.Is(err, service.ErrInviteDisabled) ||
			errors.Is(err, service.ErrInviteCodeInvalid) ||
			errors.Is(err, service.ErrInviteAlreadyBound) ||
			errors.Is(err, service.ErrInviteSelfNotAllowed) ||
			errors.Is(err, service.ErrInviteConfigIncomplete) {
			status = http.StatusBadRequest
			msg = err.Error()
		}

		c.JSON(status, gin.H{"error": msg})
		return
	}

	// 注册成功后自动生成 token
	jwtService := service.NewJWTService()
	token, err := jwtService.GenerateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusCreated, model.AuthResponse{
			ID:       user.ID,
			Username: user.Username,
			Message:  "注册成功，请登录",
		})
		return
	}

	c.JSON(http.StatusCreated, model.AuthResponse{
		ID:                 user.ID,
		Username:           user.Username,
		Token:              token,
		IsAdmin:            user.IsAdmin,
		MustChangePassword: user.MustChangePassword,
		MustChangeUsername: user.MustChangeUsername,
		Message:            "注册成功",
	})
}

func (h *UserHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	user, token, err := h.userService.Login(&req)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "登录失败"

		if errors.Is(err, service.ErrInvalidCredentials) {
			status = http.StatusUnauthorized
			msg = err.Error()
		}

		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, model.AuthResponse{
		ID:                 user.ID,
		Username:           user.Username,
		Token:              token,
		IsAdmin:            user.IsAdmin,
		MustChangePassword: user.MustChangePassword,
		MustChangeUsername: user.MustChangeUsername,
		Message:            "登录成功",
	})
}

func (h *UserHandler) GetCredentialBootstrapState(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	state, err := h.userService.GetCredentialBootstrapState(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *UserHandler) CompleteBootstrapCredentials(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var req model.CompleteBootstrapCredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	user, token, err := h.userService.CompleteBootstrapCredentials(userID, &req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrUsernameExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, model.AuthResponse{
		ID:                 user.ID,
		Username:           user.Username,
		Token:              token,
		IsAdmin:            user.IsAdmin,
		MustChangePassword: user.MustChangePassword,
		MustChangeUsername: user.MustChangeUsername,
		Message:            "首次登录信息已更新",
	})
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var req model.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.userService.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
}

func (h *UserHandler) ChangeUsername(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var req model.ChangeUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.userService.ChangeUsername(userID, req.NewUsername); err != nil {
		if errors.Is(err, service.ErrUsernameExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "用户名修改成功"})
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.userService.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户列表失败"})
		return
	}

	c.JSON(http.StatusOK, users)
}

func (h *UserHandler) ListUsersPaged(c *gin.Context) {
	page := 1
	pageSize := 20
	keyword := strings.TrimSpace(c.Query("keyword"))

	if value := c.Query("page"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "page 参数无效"})
			return
		}
		page = parsed
	}

	if value := c.Query("pageSize"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pageSize 参数无效"})
			return
		}
		pageSize = parsed
	}

	result, err := h.userService.ListUsersPaged(page, pageSize, keyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户列表失败"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *UserHandler) SetAdmin(c *gin.Context) {
	userID := c.Param("id")

	var req model.SetAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.userService.SetAdmin(userID, req.IsAdmin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "设置权限失败"})
		return
	}
	middleware.InvalidateUserRoleCache(userID)

	c.JSON(http.StatusOK, gin.H{"message": "权限设置成功"})
}

func (h *UserHandler) SetConcurrencyLimit(c *gin.Context) {
	userID := c.Param("id")

	var req model.UpdateUserConcurrencyLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.userService.SetConcurrencyLimit(userID, req.ConcurrencyLimit); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "设置并发限制失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "并发限制已更新"})
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")
	currentUserID := middleware.GetUserID(c)

	if userID == currentUserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能删除自己"})
		return
	}

	if err := h.userService.DeleteUser(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除用户失败"})
		return
	}
	middleware.InvalidateUserRoleCache(userID)

	c.JSON(http.StatusOK, gin.H{"message": "用户已删除"})
}

func (h *UserHandler) PreviewBatchUpdate(c *gin.Context) {
	var req model.UserBatchPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.userService.PreviewBatchUpdate(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *UserHandler) ApplyBatchUpdate(c *gin.Context) {
	var req model.UserBatchApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	result, err := h.userService.ApplyBatchUpdate(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	userID := c.Param("id")

	var req model.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if err := h.userService.ResetPassword(userID, req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置密码失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码已重置"})
}

func (h *UserHandler) SetGroup(c *gin.Context) {
	userID := c.Param("id")
	var req model.SetGroupsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if err := h.userService.SetGroups(userID, req.GroupIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "设置分组失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "分组设置成功"})
}

func (h *UserHandler) TopUp(c *gin.Context) {
	userID := c.Param("id")

	var req model.TopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	amountMicros, err := precision.ParseUSDToMicros(req.AmountUsd)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "充值金额格式错误"})
		return
	}
	if amountMicros <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "充值金额必须大于0"})
		return
	}

	if err := h.userService.TopUp(userID, amountMicros); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "充值失败"})
		return
	}

	balance, _ := h.userService.GetBalance(userID)
	c.JSON(http.StatusOK, gin.H{
		"message":       "充值成功",
		"balanceMicros": balance,
		"balanceUsd":    fmt.Sprintf("%.6f", float64(balance)/1e6),
	})
}

func (h *UserHandler) GetMyBalance(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	balance, err := h.userService.GetBalance(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取余额失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"balanceMicros": balance,
		"balanceUsd":    fmt.Sprintf("%.6f", float64(balance)/1e6),
	})
}
