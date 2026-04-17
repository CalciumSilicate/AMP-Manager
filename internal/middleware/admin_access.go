package middleware

import (
	"net/http"
	"strings"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/service"

	"github.com/gin-gonic/gin"
)

func AdminAccessMiddleware() gin.HandlerFunc {
	userRepo := repository.NewUserRepository()
	keyService := service.NewAdminManagementKeyService()

	return func(c *gin.Context) {
		user, authMethod, newToken, ok := authenticateAdminAccess(c, userRepo, keyService)
		if !ok {
			return
		}
		if newToken != "" {
			c.Header("X-New-Token", newToken)
		}
		setAdminContext(c, user.ID, user.Username)
		c.Set("admin_auth_method", authMethod)
		c.Next()
	}
}

func AdminAccessFromQueryOrHeader(param string) gin.HandlerFunc {
	userRepo := repository.NewUserRepository()
	keyService := service.NewAdminManagementKeyService()

	return func(c *gin.Context) {
		queryToken := strings.TrimSpace(c.Query(param))
		if queryToken != "" {
			claims, err := validateJWTToken(queryToken)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
			user, err := userRepo.GetByID(claims.UserID)
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
			setAdminContext(c, user.ID, user.Username)
			c.Set("admin_auth_method", "jwt_query")
			c.Next()
			return
		}

		user, authMethod, newToken, ok := authenticateAdminAccess(c, userRepo, keyService)
		if !ok {
			return
		}
		if newToken != "" {
			c.Header("X-New-Token", newToken)
		}
		setAdminContext(c, user.ID, user.Username)
		c.Set("admin_auth_method", authMethod)
		c.Next()
	}
}

func authenticateAdminAccess(c *gin.Context, userRepo *repository.UserRepository, keyService *service.AdminManagementKeyService) (*model.User, string, string, bool) {
	if value := strings.TrimSpace(c.GetHeader("X-API-Key")); value != "" {
		return authenticateAdminKeyUser(c, userRepo, keyService, value, model.ManagementAPIKeyAuthMethodXAPIKey)
	}

	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer amsk-") {
		parts := strings.SplitN(authHeader, " ", 2)
		return authenticateAdminKeyUser(c, userRepo, keyService, strings.TrimSpace(parts[1]), model.ManagementAPIKeyAuthMethodBearer)
	}

	if authHeader != "" {
		if claims, newToken, err := authenticateJWTHeader(authHeader); err == nil {
			user, err := userRepo.GetByID(claims.UserID)
			if err != nil || user == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
				c.Abort()
				return nil, "", "", false
			}
			if !user.IsAdmin {
				c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
				c.Abort()
				return nil, "", "", false
			}
			return user, "jwt", newToken, true
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return nil, "", "", false
		}
	}

	c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
	c.Abort()
	return nil, "", "", false
}

func authenticateAdminKeyUser(c *gin.Context, userRepo *repository.UserRepository, keyService *service.AdminManagementKeyService, keyValue, authMethod string) (*model.User, string, string, bool) {
	record, err := keyService.Validate(keyValue, authMethod)
	if err != nil {
		status := http.StatusUnauthorized
		if err == service.ErrManagementAPIKeyDisabled {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		c.Abort()
		return nil, "", "", false
	}

	user, err := userRepo.GetByID(record.UserID)
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
		c.Abort()
		return nil, "", "", false
	}
	if !user.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
		c.Abort()
		return nil, "", "", false
	}
	return user, authMethod, "", true
}

func setAdminContext(c *gin.Context, userID, username string) {
	c.Set(ContextKeyUserID, userID)
	c.Set(ContextKeyUsername, username)
	c.Set(ContextKeyIsAdmin, true)
}
