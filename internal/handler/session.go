package handler

import (
	"net/http"
	"strconv"
	"strings"

	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

type SessionHandler struct {
	service *service.SessionService
}

func NewSessionHandler() *SessionHandler {
	return &SessionHandler{
		service: service.NewSessionService(),
	}
}

func (h *SessionHandler) ListSessions(c *gin.Context) {
	page := 1
	pageSize := 20
	if parsed, err := strconv.Atoi(c.Query("page")); err == nil && parsed > 0 {
		page = parsed
	}
	if parsed, err := strconv.Atoi(c.Query("pageSize")); err == nil && parsed > 0 {
		pageSize = parsed
	}

	resp, err := h.service.List(page, pageSize, strings.TrimSpace(c.Query("query")), c.Query("activeOnly") == "true")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Session 列表失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SessionHandler) GetSession(c *gin.Context) {
	resp, statusCode, errMessage := h.service.Get(c.Param("id"))
	if errMessage != "" {
		c.JSON(statusCode, gin.H{"error": errMessage})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SessionHandler) GetLeaderboard(c *gin.Context) {
	windowMinutes := 5
	limit := 10
	if parsed, err := strconv.Atoi(c.Query("windowMinutes")); err == nil && parsed > 0 {
		windowMinutes = parsed
	}
	if parsed, err := strconv.Atoi(c.Query("limit")); err == nil && parsed > 0 {
		limit = parsed
	}

	resp, err := h.service.GetLeaderboard(windowMinutes, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Session 排行失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}
