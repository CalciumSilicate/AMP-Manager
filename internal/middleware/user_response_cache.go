package middleware

import (
	"bytes"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type cachedUserResponse struct {
	Status      int
	Body        []byte
	ContentType string
	ExpiresAt   time.Time
}

type userResponseCacheStore struct {
	mu      sync.RWMutex
	entries map[string]cachedUserResponse
	byUser  map[string]map[string]struct{}
	inflight map[string]chan struct{}
}

var globalUserResponseCache = &userResponseCacheStore{
	entries: make(map[string]cachedUserResponse),
	byUser:  make(map[string]map[string]struct{}),
	inflight: make(map[string]chan struct{}),
}

type cachedResponseWriter struct {
	gin.ResponseWriter
	buffer bytes.Buffer
	status int
}

func (w *cachedResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *cachedResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.buffer.Write(data)
	return w.ResponseWriter.Write(data)
}

func UserScopedResponseCache(ttlResolver func(*gin.Context) time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}

		userID := GetUserID(c)
		if userID == "" {
			c.Next()
			return
		}

		ttl := ttlResolver(c)
		if ttl <= 0 {
			c.Next()
			return
		}

		cacheKey := buildUserResponseCacheKey(userID, c.Request.Method, c.FullPath(), c.Request.URL.RawQuery)
		for {
			if entry, ok := globalUserResponseCache.get(cacheKey); ok {
				c.Header("Content-Type", entry.ContentType)
				c.Status(entry.Status)
				if c.Request.Method != http.MethodHead {
					_, _ = c.Writer.Write(entry.Body)
				}
				c.Abort()
				return
			}
			if ch, waiting := globalUserResponseCache.beginFill(cacheKey); waiting {
				<-ch
				continue
			} else {
				defer globalUserResponseCache.finishFill(cacheKey)
				break
			}
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

		globalUserResponseCache.set(userID, cacheKey, cachedUserResponse{
			Status:      writer.status,
			Body:        append([]byte(nil), writer.buffer.Bytes()...),
			ContentType: contentType,
			ExpiresAt:   time.Now().Add(ttl),
		})
	}
}

func InvalidateUserResponseCachesOnWrite() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			return
		}

		if c.Writer.Status() >= http.StatusBadRequest {
			return
		}

		userID := GetUserID(c)
		if userID == "" {
			return
		}
		globalUserResponseCache.invalidateUser(userID)
	}
}

func buildUserResponseCacheKey(userID, method, fullPath, rawQuery string) string {
	if fullPath == "" {
		fullPath = rawQuery
	}
	return userID + "|" + method + "|" + fullPath + "|" + rawQuery
}

func (s *userResponseCacheStore) get(key string) (cachedUserResponse, bool) {
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

func (s *userResponseCacheStore) set(userID, key string, entry cachedUserResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[key] = entry
	if _, ok := s.byUser[userID]; !ok {
		s.byUser[userID] = make(map[string]struct{})
	}
	s.byUser[userID][key] = struct{}{}
}

func (s *userResponseCacheStore) beginFill(key string) (chan struct{}, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.inflight[key]; ok {
		return existing, true
	}
	ch := make(chan struct{})
	s.inflight[key] = ch
	return ch, false
}

func (s *userResponseCacheStore) finishFill(key string) {
	s.mu.Lock()
	ch, ok := s.inflight[key]
	if ok {
		delete(s.inflight, key)
	}
	s.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (s *userResponseCacheStore) invalidateUser(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := s.byUser[userID]
	for key := range keys {
		delete(s.entries, key)
	}
	delete(s.byUser, userID)
}
