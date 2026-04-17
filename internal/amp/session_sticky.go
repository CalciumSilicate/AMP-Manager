package amp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ampmanager/internal/config"
	"ampmanager/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

const (
	defaultSessionStickyWindowMinutes     = 5
	defaultSessionStickyLogSearchMinChars = 4
)

type requestSessionKey struct{}

type RequestSession struct {
	SessionID       string
	StickyChannelID string
	StickyProvider  string
}

type sessionStickyRuntime struct {
	mu     sync.RWMutex
	client *redis.Client
	prefix string

	healthy atomic.Bool
	config  atomic.Value
}

var globalSessionStickyRuntime = newSessionStickyRuntime()

func newSessionStickyRuntime() *sessionStickyRuntime {
	runtime := &sessionStickyRuntime{}
	runtime.config.Store(defaultSessionStickyConfig())
	return runtime
}

func defaultSessionStickyConfig() model.SessionStickyConfigResponse {
	return model.SessionStickyConfigResponse{
		Enabled:                       false,
		WindowMinutes:                 defaultSessionStickyWindowMinutes,
		LogSearchMinChars:             defaultSessionStickyLogSearchMinChars,
		InjectPromptCacheKeyResponses: false,
	}
}

func normalizeSessionStickyConfig(cfg model.SessionStickyConfigResponse) model.SessionStickyConfigResponse {
	if cfg.WindowMinutes <= 0 {
		cfg.WindowMinutes = defaultSessionStickyWindowMinutes
	}
	if cfg.LogSearchMinChars <= 0 {
		cfg.LogSearchMinChars = defaultSessionStickyLogSearchMinChars
	}
	return cfg
}

func InitSessionStickyRuntime(cfg *config.Config) {
	globalSessionStickyRuntime.mu.Lock()
	defer globalSessionStickyRuntime.mu.Unlock()

	if globalSessionStickyRuntime.client != nil {
		_ = globalSessionStickyRuntime.client.Close()
		globalSessionStickyRuntime.client = nil
	}
	globalSessionStickyRuntime.prefix = ""
	globalSessionStickyRuntime.healthy.Store(false)

	if cfg == nil || strings.TrimSpace(cfg.RedisURL) == "" {
		return
	}

	options, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Warnf("session sticky: invalid REDIS_URL, runtime disabled: %v", err)
		return
	}

	client := redis.NewClient(options)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Warnf("session sticky: redis ping failed, runtime disabled: %v", err)
		_ = client.Close()
		return
	}

	globalSessionStickyRuntime.client = client
	globalSessionStickyRuntime.prefix = strings.TrimSpace(cfg.RedisPrefix)
	if globalSessionStickyRuntime.prefix == "" {
		globalSessionStickyRuntime.prefix = "ampmanager"
	}
	globalSessionStickyRuntime.healthy.Store(true)
}

func StopSessionStickyRuntime() {
	globalSessionStickyRuntime.mu.Lock()
	defer globalSessionStickyRuntime.mu.Unlock()

	if globalSessionStickyRuntime.client != nil {
		_ = globalSessionStickyRuntime.client.Close()
		globalSessionStickyRuntime.client = nil
	}
	globalSessionStickyRuntime.healthy.Store(false)
}

func UpdateSessionStickyConfig(cfg model.SessionStickyConfigResponse) {
	cfg = normalizeSessionStickyConfig(cfg)
	globalSessionStickyRuntime.config.Store(cfg)
}

func GetSessionStickyConfig() model.SessionStickyConfigResponse {
	if cfg, ok := globalSessionStickyRuntime.config.Load().(model.SessionStickyConfigResponse); ok {
		return normalizeSessionStickyConfig(cfg)
	}
	return defaultSessionStickyConfig()
}

func SessionStickyRuntimeHealthy() bool {
	return globalSessionStickyRuntime.healthy.Load()
}

func SessionStickyRedisConfigured() bool {
	globalSessionStickyRuntime.mu.RLock()
	defer globalSessionStickyRuntime.mu.RUnlock()
	return globalSessionStickyRuntime.client != nil
}

func WithRequestSession(ctx context.Context, session *RequestSession) context.Context {
	return context.WithValue(ctx, requestSessionKey{}, session)
}

func GetRequestSession(ctx context.Context) *RequestSession {
	if val := ctx.Value(requestSessionKey{}); val != nil {
		if session, ok := val.(*RequestSession); ok {
			return session
		}
	}
	return nil
}

func EnsureRequestSession(c *gin.Context) *RequestSession {
	if c == nil || c.Request == nil {
		return nil
	}
	if session := GetRequestSession(c.Request.Context()); session != nil {
		return session
	}

	apiKeyID := ""
	if proxyCfg := GetProxyConfig(c.Request.Context()); proxyCfg != nil {
		apiKeyID = proxyCfg.APIKeyID
	}
	sessionID := resolveSessionIDFromHeadersAndBody(c.Request.Context(), c.Request.Header, requestBodyBytes(c), "", apiKeyID, c.ClientIP())
	session := &RequestSession{SessionID: sessionID}
	if binding, ok := lookupStickyBinding(c.Request.Context(), sessionID); ok {
		session.StickyChannelID, session.StickyProvider = parseStickyBinding(binding)
	}
	c.Request = c.Request.WithContext(WithRequestSession(c.Request.Context(), session))
	return session
}

func BuildRequestSession(ctx context.Context, headers http.Header, body []byte, fallback string) *RequestSession {
	apiKeyID := ""
	if proxyCfg := GetProxyConfig(ctx); proxyCfg != nil {
		apiKeyID = proxyCfg.APIKeyID
	}
	sessionID := resolveSessionIDFromHeadersAndBody(ctx, headers, body, fallback, apiKeyID, clientIPFromHeaders(headers, ""))
	session := &RequestSession{SessionID: sessionID}
	if binding, ok := lookupStickyBinding(ctx, sessionID); ok {
		session.StickyChannelID, session.StickyProvider = parseStickyBinding(binding)
	}
	return session
}

func BuildWebsocketSessionFallback() string {
	return "ws-" + uuid.New().String()
}

func NormalizeStickyProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "claude", "anthropic":
		return "anthropic"
	case "openai", "openai_chat", "openai_responses":
		return "openai"
	case "gemini", "google":
		return "gemini"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func StickyProviderFromChannel(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	switch channel.Type {
	case model.ChannelTypeClaude:
		return "anthropic"
	case model.ChannelTypeOpenAI:
		return "openai"
	case model.ChannelTypeGemini:
		return "gemini"
	default:
		return NormalizeStickyProvider(string(channel.Type))
	}
}

func StickyProviderMatchesChannel(provider string, channel *model.Channel) bool {
	provider = NormalizeStickyProvider(provider)
	if provider == "" {
		return true
	}
	return StickyProviderFromChannel(channel) == provider
}

func parseStickyBinding(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	switch strings.ToLower(raw) {
	case "claude", "anthropic":
		return "", "anthropic"
	case "openai", "openai_chat", "openai_responses":
		return "", "openai"
	case "gemini", "google":
		return "", "gemini"
	default:
		return raw, ""
	}
}

func lookupStickyBinding(ctx context.Context, sessionID string) (string, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", false
	}

	cfg := GetSessionStickyConfig()
	if !cfg.Enabled {
		return "", false
	}

	client := sessionStickyClient()
	if client == nil {
		return "", false
	}

	binding, err := client.Get(runtimeContext(ctx), sessionStickyBindingKey(sessionID)).Result()
	if err != nil {
		if err != redis.Nil {
			markSessionStickyUnhealthy(err)
		}
		return "", false
	}

	binding = strings.TrimSpace(binding)
	return binding, binding != ""
}

func LookupStickyProvider(ctx context.Context, sessionID string) (string, bool) {
	binding, ok := lookupStickyBinding(ctx, sessionID)
	if !ok {
		return "", false
	}

	_, provider := parseStickyBinding(binding)
	if provider == "" {
		return "", false
	}
	return provider, true
}

func GetSessionStickyBinding(ctx context.Context, sessionID string) (string, time.Duration, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", 0, false
	}

	client := sessionStickyClient()
	if client == nil {
		return "", 0, false
	}

	ctx = runtimeContext(ctx)
	key := sessionStickyBindingKey(sessionID)
	binding, err := client.Get(ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			markSessionStickyUnhealthy(err)
		}
		return "", 0, false
	}
	ttl, ttlErr := client.TTL(ctx, key).Result()
	if ttlErr != nil && ttlErr != redis.Nil {
		markSessionStickyUnhealthy(ttlErr)
		ttl = 0
	}
	channelID, provider := parseStickyBinding(binding)
	if channelID != "" {
		return channelID, ttl, true
	}
	return provider, ttl, provider != ""
}

func FinalizeSessionSticky(ctx context.Context, trace *RequestTrace) {
	if trace == nil {
		return
	}
	snapshot := trace.Clone()
	if snapshot.SessionID == "" || snapshot.ChannelID == "" {
		return
	}

	if snapshot.ErrorType == "" && snapshot.StatusCode >= 200 && snapshot.StatusCode < 300 {
		bindStickyChannel(ctx, snapshot.SessionID, snapshot.ChannelID)
		return
	}

	clearStickyProvider(ctx, snapshot.SessionID)
}

func bindStickyChannel(ctx context.Context, sessionID string, channelID string) {
	cfg := GetSessionStickyConfig()
	if !cfg.Enabled {
		return
	}
	client := sessionStickyClient()
	if client == nil {
		return
	}

	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return
	}

	if err := client.Set(runtimeContext(ctx), sessionStickyBindingKey(sessionID), channelID, time.Duration(cfg.WindowMinutes)*time.Minute).Err(); err != nil {
		markSessionStickyUnhealthy(err)
	}
}

func clearStickyProvider(ctx context.Context, sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	client := sessionStickyClient()
	if client == nil {
		return
	}
	if err := client.Del(runtimeContext(ctx), sessionStickyBindingKey(sessionID)).Err(); err != nil && err != redis.Nil {
		markSessionStickyUnhealthy(err)
	}
}

func requestBodyBytes(c *gin.Context) []byte {
	payload, err := ensureRequestBody(c)
	if err != nil || payload == nil {
		return nil
	}
	return payload.Body
}

func resolveSessionIDFromHeadersAndBody(ctx context.Context, headers http.Header, body []byte, fallback string, apiKeyID string, clientIP string) string {
	if sessionID := firstNonEmptyHeader(headers, "Session_id", "session_id"); sessionID != "" {
		return sessionID
	}
	if sessionID := firstNonEmptyHeader(headers, "X-Session-Id", "x-session-id"); sessionID != "" {
		return sessionID
	}
	if sessionID := firstNonEmptyHeader(headers, "X-Amp-Thread-Id", "x-amp-thread-id"); sessionID != "" {
		return sessionID
	}

	if sessionID := extractSessionIDFromBody(body); sessionID != "" {
		return sessionID
	}

	seedHash := requestContentSeedHash(body)
	if sessionID := lookupOrCreateFingerprintSessionID(ctx, apiKeyID, clientIP, firstNonEmptyHeader(headers, "User-Agent", "user-agent"), seedHash); sessionID != "" {
		return sessionID
	}
	if sessionID := lookupOrCreateContentSessionID(ctx, seedHash); sessionID != "" {
		return sessionID
	}

	return strings.TrimSpace(fallback)
}

func extractSessionIDFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	for _, path := range []string{
		"session_id",
		"sessionId",
		"metadata.session_id",
		"metadata.sessionId",
		"previous_response_id",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}

	userIDPayload := strings.TrimSpace(gjson.GetBytes(body, "metadata.user_id").String())
	if userIDPayload != "" && gjson.Valid(userIDPayload) {
		for _, path := range []string{"session_id", "sessionId"} {
			if value := strings.TrimSpace(gjson.Get(userIDPayload, path).String()); value != "" {
				return value
			}
		}
	}

	return ""
}

func firstNonEmptyHeader(headers http.Header, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func clientIPFromHeaders(headers http.Header, fallback string) string {
	for _, candidate := range []string{
		firstNonEmptyHeader(headers, "X-Forwarded-For", "x-forwarded-for"),
		firstNonEmptyHeader(headers, "X-Real-Ip", "x-real-ip"),
		fallback,
	} {
		if candidate == "" {
			continue
		}
		first := strings.TrimSpace(strings.Split(candidate, ",")[0])
		if first == "" {
			continue
		}
		if host, _, err := net.SplitHostPort(first); err == nil {
			return host
		}
		return first
	}
	return ""
}

func requestContentSeedHash(body []byte) string {
	parts := extractFirstMessageTexts(body)
	if len(parts) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func extractFirstMessageTexts(body []byte) []string {
	if len(body) == 0 {
		return nil
	}

	extract := func(items []gjson.Result) []string {
		parts := make([]string, 0, 3)
		for _, item := range items {
			if len(parts) >= 3 {
				break
			}
			content := item.Get("content")
			switch {
			case content.Type == gjson.String:
				text := strings.TrimSpace(content.String())
				if text != "" {
					parts = append(parts, text)
				}
			case content.IsArray():
				var builder strings.Builder
				for _, block := range content.Array() {
					blockType := strings.ToLower(strings.TrimSpace(block.Get("type").String()))
					if blockType == "" || blockType == "text" || blockType == "input_text" {
						text := strings.TrimSpace(block.Get("text").String())
						if text != "" {
							builder.WriteString(text)
						}
					}
				}
				if builder.Len() > 0 {
					parts = append(parts, builder.String())
				}
			}
		}
		return parts
	}

	if messages := gjson.GetBytes(body, "messages"); messages.IsArray() {
		if parts := extract(messages.Array()); len(parts) > 0 {
			return parts
		}
	}
	if inputs := gjson.GetBytes(body, "input"); inputs.IsArray() {
		if parts := extract(inputs.Array()); len(parts) > 0 {
			return parts
		}
	}
	return nil
}

func lookupOrCreateFingerprintSessionID(ctx context.Context, apiKeyID, clientIP, userAgent, contentHash string) string {
	client := sessionStickyClient()
	if client == nil {
		return ""
	}
	if strings.TrimSpace(apiKeyID) == "" && strings.TrimSpace(clientIP) == "" && strings.TrimSpace(userAgent) == "" && strings.TrimSpace(contentHash) == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(apiKeyID),
		strings.TrimSpace(clientIP),
		strings.TrimSpace(userAgent),
		strings.TrimSpace(contentHash),
	}, "|")))
	hash := hex.EncodeToString(sum[:])
	return lookupOrCreateMappedSessionID(ctx, sessionStickyFingerprintKey(hash), "sess-"+hash[:16])
}

func lookupOrCreateContentSessionID(ctx context.Context, contentHash string) string {
	if strings.TrimSpace(contentHash) == "" {
		return ""
	}
	if sessionStickyClient() == nil {
		return ""
	}
	return lookupOrCreateMappedSessionID(ctx, sessionStickyContentKey(contentHash), "sess-"+contentHash[:16])
}

func lookupOrCreateMappedSessionID(ctx context.Context, key string, fallbackID string) string {
	client := sessionStickyClient()
	if client == nil {
		return ""
	}

	ctx = runtimeContext(ctx)
	if existing, err := client.Get(ctx, key).Result(); err == nil && strings.TrimSpace(existing) != "" {
		return strings.TrimSpace(existing)
	} else if err != nil && err != redis.Nil {
		markSessionStickyUnhealthy(err)
		return ""
	}

	sessionID := strings.TrimSpace(fallbackID)
	if sessionID == "" {
		sessionID = "sess-" + uuid.NewString()
	}
	ttl := time.Duration(GetSessionStickyConfig().WindowMinutes) * time.Minute
	if err := client.SetNX(ctx, key, sessionID, ttl).Err(); err != nil {
		markSessionStickyUnhealthy(err)
		return ""
	}
	if existing, err := client.Get(ctx, key).Result(); err == nil && strings.TrimSpace(existing) != "" {
		return strings.TrimSpace(existing)
	}
	return sessionID
}

func sessionStickyBindingKey(sessionID string) string {
	return fmt.Sprintf("%s:session-sticky:v1:binding:%s", globalSessionStickyRuntime.prefix, strings.TrimSpace(sessionID))
}

func sessionStickyFingerprintKey(hash string) string {
	return fmt.Sprintf("%s:session-sticky:v1:fingerprint:%s", globalSessionStickyRuntime.prefix, strings.TrimSpace(hash))
}

func sessionStickyContentKey(hash string) string {
	return fmt.Sprintf("%s:session-sticky:v1:content:%s", globalSessionStickyRuntime.prefix, strings.TrimSpace(hash))
}

func sessionStickyClient() *redis.Client {
	globalSessionStickyRuntime.mu.RLock()
	defer globalSessionStickyRuntime.mu.RUnlock()
	return globalSessionStickyRuntime.client
}

func runtimeContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func markSessionStickyUnhealthy(err error) {
	globalSessionStickyRuntime.healthy.Store(false)
	if err != nil {
		log.Warnf("session sticky: redis unavailable, fail-open fallback: %v", err)
	}
}
