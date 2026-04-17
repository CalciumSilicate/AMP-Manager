package middleware

import (
	"sync"
	"time"

	"ampmanager/internal/repository"
)

const userRoleCacheTTL = 30 * time.Second

type userRoleCacheEntry struct {
	IsAdmin   bool
	ExpiresAt time.Time
}

var userRoleCache sync.Map

func loadCachedUserAdminStatus(userID string) bool {
	if userID == "" {
		return false
	}

	now := time.Now().UTC()
	if cached, ok := userRoleCache.Load(userID); ok {
		if entry, ok := cached.(userRoleCacheEntry); ok && now.Before(entry.ExpiresAt) {
			return entry.IsAdmin
		}
	}

	user, err := repository.NewUserRepository().GetByID(userID)
	if err != nil || user == nil {
		userRoleCache.Delete(userID)
		return false
	}

	userRoleCache.Store(userID, userRoleCacheEntry{
		IsAdmin:   user.IsAdmin,
		ExpiresAt: now.Add(userRoleCacheTTL),
	})
	return user.IsAdmin
}

func InvalidateUserRoleCache(userID string) {
	if userID == "" {
		return
	}
	userRoleCache.Delete(userID)
}
