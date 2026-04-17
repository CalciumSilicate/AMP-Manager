package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	ContextKeyUserID             = "user_id"
	ContextKeyUsername           = "username"
	ContextKeyIsAdmin            = "is_admin"
	ContextKeyMustChangePassword = "must_change_password"
	ContextKeyMustChangeUsername = "must_change_username"

	// tokenRefreshThreshold 当 Token 签发超过此时间后，自动刷新（滑动过期）
	tokenRefreshThreshold = 1 * time.Hour
)

func JWTAuthMiddleware() gin.HandlerFunc {
	userRepo := repository.NewUserRepository()

	return func(c *gin.Context) {
		claims, newToken, err := authenticateJWTHeader(c.GetHeader("Authorization"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyUsername, claims.Username)
		c.Set(ContextKeyIsAdmin, loadCachedUserAdminStatus(claims.UserID))
		if user, userErr := userRepo.GetByID(claims.UserID); userErr == nil && user != nil {
			c.Set(ContextKeyMustChangePassword, user.MustChangePassword)
			c.Set(ContextKeyMustChangeUsername, user.MustChangeUsername)
		}
		if newToken != "" {
			c.Header("X-New-Token", newToken)
		}

		c.Next()
	}
}

func CredentialBootstrapMiddleware() gin.HandlerFunc {
	userRepo := repository.NewUserRepository()

	return func(c *gin.Context) {
		userID := GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
			c.Abort()
			return
		}

		user, err := userRepo.GetByID(userID)
		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			c.Abort()
			return
		}

		c.Set(ContextKeyMustChangePassword, user.MustChangePassword)
		c.Set(ContextKeyMustChangeUsername, user.MustChangeUsername)
		if !user.MustChangePassword && !user.MustChangeUsername {
			c.Next()
			return
		}

		switch c.FullPath() {
		case "/api/me/password", "/api/me/username", "/api/me/bootstrap/state", "/api/me/bootstrap/credentials":
			c.Next()
			return
		default:
			c.JSON(http.StatusPreconditionRequired, gin.H{
				"error":              "请先完成首次改密和改名",
				"mustChangePassword": user.MustChangePassword,
				"mustChangeUsername": user.MustChangeUsername,
			})
			c.Abort()
			return
		}
	}
}

func AdminMiddleware() gin.HandlerFunc {
	userRepo := repository.NewUserRepository()

	return func(c *gin.Context) {
		if IsAdmin(c) {
			c.Next()
			return
		}

		userID := GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
			c.Abort()
			return
		}

		user, err := userRepo.GetByID(userID)
		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			c.Abort()
			return
		}

		if !user.IsAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}

		c.Set(ContextKeyUsername, user.Username)
		c.Set(ContextKeyIsAdmin, true)
		c.Next()
	}
}

// JWTAuthFromQuery 从 query 参数中提取 JWT 进行认证（用于 WebSocket）
func JWTAuthFromQuery(param string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.Query(param)
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少认证参数"})
			c.Abort()
			return
		}

		claims, err := validateJWTToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyUsername, claims.Username)
		c.Set(ContextKeyIsAdmin, loadCachedUserAdminStatus(claims.UserID))
		c.Next()
	}
}

func GetUserID(c *gin.Context) string {
	userID, _ := c.Get(ContextKeyUserID)
	if id, ok := userID.(string); ok {
		return id
	}
	return ""
}

func GetUsername(c *gin.Context) string {
	username, _ := c.Get(ContextKeyUsername)
	if name, ok := username.(string); ok {
		return name
	}
	return ""
}

func IsAdmin(c *gin.Context) bool {
	v, exists := c.Get(ContextKeyIsAdmin)
	if !exists {
		return false
	}
	isAdmin, ok := v.(bool)
	return ok && isAdmin
}

func authenticateJWTHeader(authHeader string) (*service.JWTClaims, string, error) {
	if authHeader == "" {
		return nil, "", errors.New("缺少 Authorization 头")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return nil, "", errors.New("Authorization 格式错误")
	}

	claims, err := validateJWTToken(parts[1])
	if err != nil {
		return nil, "", err
	}

	newToken := ""
	if claims.IssuedAt != nil && time.Since(claims.IssuedAt.Time) > tokenRefreshThreshold {
		if refreshedToken, refreshErr := service.NewJWTService().GenerateToken(claims.UserID, claims.Username); refreshErr == nil {
			newToken = refreshedToken
		}
	}

	return claims, newToken, nil
}

func validateJWTToken(tokenString string) (*service.JWTClaims, error) {
	claims, err := service.NewJWTService().ValidateToken(tokenString)
	if err != nil {
		if err == service.ErrExpiredToken {
			return nil, errors.New("Token 已过期")
		}
		return nil, errors.New("Token 验证失败")
	}
	return claims, nil
}
