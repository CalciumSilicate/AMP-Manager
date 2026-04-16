package amp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

const defaultMaxRequestPayloadBytes int64 = 128 * 1024 * 1024

var errRequestBodyTooLarge = errors.New("request body too large")
var requestPayloadLimitBytes atomic.Int64

func init() {
	requestPayloadLimitBytes.Store(defaultMaxRequestPayloadBytes)
}

type requestPayloadKey struct{}

type RequestPayload struct {
	Body       []byte
	JSON       map[string]interface{}
	jsonParsed bool
}

func WithRequestPayload(ctx context.Context, payload *RequestPayload) context.Context {
	return context.WithValue(ctx, requestPayloadKey{}, payload)
}

func GetRequestPayload(ctx context.Context) *RequestPayload {
	if val := ctx.Value(requestPayloadKey{}); val != nil {
		if payload, ok := val.(*RequestPayload); ok {
			return payload
		}
	}
	return nil
}

func ensureRequestBody(c *gin.Context) (*RequestPayload, error) {
	if payload := GetRequestPayload(c.Request.Context()); payload != nil {
		return payload, nil
	}

	payload := &RequestPayload{}
	if c.Request.Body != nil {
		bodyBytes, err := readRequestBodyWithLimit(c.Request.Body, GetRequestPayloadLimitBytes())
		if err != nil {
			return nil, err
		}
		payload.Body = bodyBytes
		restoreRequestBody(c.Request, bodyBytes)
	}

	ctx := WithRequestPayload(c.Request.Context(), payload)
	c.Request = c.Request.WithContext(ctx)
	return payload, nil
}

func GetRequestPayloadLimitBytes() int64 {
	limit := requestPayloadLimitBytes.Load()
	if limit <= 0 {
		return defaultMaxRequestPayloadBytes
	}
	return limit
}

func UpdateRequestPayloadLimitBytes(limit int64) {
	if limit <= 0 {
		limit = defaultMaxRequestPayloadBytes
	}
	requestPayloadLimitBytes.Store(limit)
}

func EnsureRequestPayload(c *gin.Context) (*RequestPayload, error) {
	payload, err := ensureRequestBody(c)
	if err != nil {
		return nil, err
	}
	if payload.jsonParsed {
		return payload, nil
	}
	payload.jsonParsed = true
	if strings.Contains(c.GetHeader("Content-Type"), "application/json") && len(payload.Body) > 0 {
		var parsed map[string]interface{}
		if err := json.Unmarshal(payload.Body, &parsed); err == nil {
			payload.JSON = parsed
		}
	}
	return payload, nil
}

func RestoreRequestBody(req *http.Request, body []byte) {
	restoreRequestBody(req, body)
}

func restoreRequestBody(req *http.Request, body []byte) {
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.TransferEncoding = nil
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

func readRequestBodyWithLimit(body io.ReadCloser, limit int64) ([]byte, error) {
	defer body.Close()

	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errRequestBodyTooLarge
	}
	return data, nil
}

func isRequestBodyTooLarge(err error) bool {
	return errors.Is(err, errRequestBodyTooLarge)
}
