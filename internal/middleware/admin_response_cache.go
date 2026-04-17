package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type adminResponseCacheStore struct {
	mu      sync.RWMutex
	entries map[string]cachedUserResponse
}

var globalAdminResponseCache = &adminResponseCacheStore{
	entries: make(map[string]cachedUserResponse),
}

func AdminScopedResponseCache(ttl time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}

		adminUserID := GetUserID(c)
		if adminUserID == "" || !IsAdmin(c) || ttl <= 0 {
			c.Next()
			return
		}

		cacheKey := buildUserResponseCacheKey(adminUserID, c.Request.Method, c.FullPath(), c.Request.URL.RawQuery)
		if entry, ok := globalAdminResponseCache.get(cacheKey); ok {
			c.Header("Content-Type", entry.ContentType)
			c.Status(entry.Status)
			if c.Request.Method != http.MethodHead {
				_, _ = c.Writer.Write(entry.Body)
			}
			c.Abort()
			return
		}

		writer := &cachedResponseWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Next()

		if c.IsAborted() && writer.status == 0 {
			return
		}
		if writer.status != http.StatusOK || writer.buffer.Len() == 0 {
			return
		}

		contentType := strings.TrimSpace(c.Writer.Header().Get("Content-Type"))
		if !strings.HasPrefix(contentType, "application/json") {
			return
		}

		globalAdminResponseCache.set(cacheKey, cachedUserResponse{
			Status:      writer.status,
			Body:        append([]byte(nil), writer.buffer.Bytes()...),
			ContentType: contentType,
			ExpiresAt:   time.Now().Add(ttl),
		})
	}
}

func (s *adminResponseCacheStore) get(key string) (cachedUserResponse, bool) {
	s.mu.RLock()
	entry, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok {
		return cachedUserResponse{}, false
	}
	if time.Now().After(entry.ExpiresAt) {
		s.mu.Lock()
		delete(s.entries, key)
		s.mu.Unlock()
		return cachedUserResponse{}, false
	}
	return entry, true
}

func (s *adminResponseCacheStore) set(key string, entry cachedUserResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[key] = entry
}
