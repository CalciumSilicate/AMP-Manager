package handler

import (
	"net/http"

	"ampmanager/internal/amp"
	"ampmanager/internal/model"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *SystemHandler) ListErrorRulesV2(c *gin.Context) {
	rules, err := service.NewErrorRuleService().List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取错误规则失败"})
		return
	}
	c.JSON(http.StatusOK, model.ErrorRuleListResponse{Rules: rules})
}

func (h *SystemHandler) CreateErrorRule(c *gin.Context) {
	var req model.ErrorRuleCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	rule, err := service.NewErrorRuleService().Create(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"rule": rule})
}

func (h *SystemHandler) UpdateErrorRule(c *gin.Context) {
	var req model.ErrorRuleUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	rule, err := service.NewErrorRuleService().Update(c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

func (h *SystemHandler) DeleteErrorRule(c *gin.Context) {
	if err := service.NewErrorRuleService().Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "错误规则已删除"})
}

func (h *SystemHandler) RefreshErrorRuleRuntime(c *gin.Context) {
	if err := service.NewErrorRuleService().RefreshRuntime(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":   "错误规则缓存已刷新",
		"cacheStats": amp.ErrorRuleCacheStats(),
	})
}

func (h *SystemHandler) GetErrorRuleCacheStats(c *gin.Context) {
	c.JSON(http.StatusOK, amp.ErrorRuleCacheStats())
}

func (h *SystemHandler) TestErrorRule(c *gin.Context) {
	var req model.ErrorRuleTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	match := amp.MatchErrorRuleByRequestType(req.RequestType, req.UpstreamStatus, []byte(req.Body))
	if match == nil {
		c.JSON(http.StatusOK, model.ErrorRuleTestResponse{
			Matched:    false,
			StatusCode: req.UpstreamStatus,
		})
		return
	}
	statusCode := req.UpstreamStatus
	if match.Rule.OverrideStatusCode != nil {
		statusCode = *match.Rule.OverrideStatusCode
	}
	responseBody := amp.BuildProtocolErrorResponseBodyWithOverride(
		match.RequestType,
		statusCode,
		match.Rule.OverrideMessage,
		match.Rule.OverrideResponse,
	)
	c.JSON(http.StatusOK, model.ErrorRuleTestResponse{
		Matched:      true,
		Rule:         &match.Rule,
		StatusCode:   statusCode,
		ResponseBody: responseBody,
	})
}
