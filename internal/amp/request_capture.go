package amp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// multiReaderCloser wraps MultiReader with Close capability
type multiReaderCloser struct {
	io.Reader
	io.Closer
}

type captureDataKey struct{}

// CaptureData holds captured request data
type CaptureData struct {
	RequestHeaders http.Header
	RequestBody    []byte
}

// WithCaptureData stores capture data in context
func WithCaptureData(ctx context.Context, data *CaptureData) context.Context {
	return context.WithValue(ctx, captureDataKey{}, data)
}

// GetCaptureData retrieves capture data from context
func GetCaptureData(ctx context.Context) *CaptureData {
	if val := ctx.Value(captureDataKey{}); val != nil {
		if data, ok := val.(*CaptureData); ok {
			return data
		}
	}
	return nil
}

// RequestCaptureMiddleware captures request headers and body for detail logging
func RequestCaptureMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsRequestDetailEnabled() {
			c.Next()
			return
		}

		// Only capture for model invocation requests
		if !IsModelInvocation(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}

		// Capture request headers (sanitize sensitive headers)
		requestHeaders := sanitizeHeaders(c.Request.Header)

		// Capture request body
		var requestBody []byte
		if c.Request.Body != nil {
			bodyCap := GetRequestDetailConfig().BodyCapBytes
			bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, int64(bodyCap)+1))
			if err == nil {
				if len(bodyBytes) > bodyCap {
					requestBody = bodyBytes[:bodyCap]
				} else {
					requestBody = bodyBytes
				}
				// Restore the body using MultiReader: captured prefix + remaining original body
				c.Request.Body = &multiReaderCloser{
					Reader: io.MultiReader(bytes.NewReader(bodyBytes), c.Request.Body),
					Closer: c.Request.Body,
				}
			}
		}

		// Store capture data in context
		captureData := &CaptureData{
			RequestHeaders: requestHeaders,
			RequestBody:    requestBody,
		}
		ctx := WithCaptureData(c.Request.Context(), captureData)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// StoreRequestDetail stores captured request detail after trace is created
func StoreRequestDetail(requestID string, headers http.Header, body []byte) {
	if !IsRequestDetailEnabled() {
		return
	}

	store := GetRequestDetailStore()
	if store == nil {
		return
	}

	store.UpdateRequestData(requestID, headers, body)
	log.Debugf("request capture: stored request data for %s (headers: %d, body: %d bytes)",
		requestID, len(headers), len(body))
}

// StoreTranslatedRequestBody stores the translated request body
func StoreTranslatedRequestBody(requestID string, body []byte) {
	if !IsRequestDetailEnabled() {
		return
	}

	store := GetRequestDetailStore()
	if store == nil {
		return
	}

	store.UpdateTranslatedRequestBody(requestID, body)
	log.Debugf("request capture: stored translated request body for %s (%d bytes)",
		requestID, len(body))
}

// StoreTranslatedRequestHeaders stores the translated request headers
func StoreTranslatedRequestHeaders(requestID string, headers http.Header) {
	if !IsRequestDetailEnabled() {
		return
	}

	store := GetRequestDetailStore()
	if store == nil {
		return
	}

	store.UpdateTranslatedRequestHeaders(requestID, headers)
	log.Debugf("request capture: stored translated request headers for %s (%d headers)",
		requestID, len(headers))
}

// StoreResponseDetail stores response headers and body
func StoreResponseDetail(requestID string, headers http.Header, body []byte) {
	if !IsRequestDetailEnabled() {
		return
	}

	store := GetRequestDetailStore()
	if store == nil {
		return
	}

	store.UpdateResponseData(requestID, headers, body)
	log.Debugf("request capture: stored response data for %s (headers: %d, body: %d bytes)",
		requestID, len(headers), len(body))
}

// sanitizeHeaders removes sensitive headers for storage
func sanitizeHeaders(headers http.Header) http.Header {
	sanitized := headers.Clone()

	// Explicitly listed sensitive headers
	exactSensitiveKeys := []string{
		"Authorization",
		"Proxy-Authorization",
		"X-Api-Key",
		"Cookie",
		"Set-Cookie",
		"X-Auth-Token",
		"X-Access-Token",
		"X-Refresh-Token",
		"X-Amz-Security-Token",
		"X-Amz-Credential",
		"X-Goog-Api-Key",
	}

	for _, key := range exactSensitiveKeys {
		if sanitized.Get(key) != "" {
			sanitized.Set(key, "[REDACTED]")
		}
	}

	// Fuzzy match headers containing sensitive keywords
	sensitivePatterns := []string{"secret", "token", "key", "password", "credential", "auth"}
	for key := range sanitized {
		keyLower := strings.ToLower(key)
		for _, pattern := range sensitivePatterns {
			if strings.Contains(keyLower, pattern) && sanitized.Get(key) != "[REDACTED]" {
				sanitized.Set(key, "[REDACTED]")
				break
			}
		}
	}

	return sanitized
}

// ResponseCaptureWrapper wraps response body to capture data for storage
type ResponseCaptureWrapper struct {
	io.ReadCloser
	requestID string
	headers   http.Header
	buffer    *bytes.Buffer
	maxSize   int
}

// NewResponseCaptureWrapper creates a new response capture wrapper
func NewResponseCaptureWrapper(body io.ReadCloser, requestID string, headers http.Header) *ResponseCaptureWrapper {
	bodyCap := GetRequestDetailConfig().BodyCapBytes
	return &ResponseCaptureWrapper{
		ReadCloser: body,
		requestID:  requestID,
		headers:    sanitizeHeaders(headers),
		buffer:     &bytes.Buffer{},
		maxSize:    bodyCap,
	}
}

func (w *ResponseCaptureWrapper) Read(p []byte) (int, error) {
	n, err := w.ReadCloser.Read(p)
	if n > 0 && w.buffer.Len() < w.maxSize {
		remaining := w.maxSize - w.buffer.Len()
		if n <= remaining {
			w.buffer.Write(p[:n])
		} else {
			w.buffer.Write(p[:remaining])
		}
	}
	return n, err
}

func (w *ResponseCaptureWrapper) Close() error {
	// Store response detail before closing
	if w.requestID != "" {
		StoreResponseDetail(w.requestID, w.headers, w.buffer.Bytes())
	}
	return w.ReadCloser.Close()
}
