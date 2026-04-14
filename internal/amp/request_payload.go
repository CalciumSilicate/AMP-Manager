package amp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxRequestPayloadBytes = 10 * 1024 * 1024

type requestPayloadKey struct{}

type RequestPayload struct {
	Body []byte
	JSON map[string]interface{}
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

func EnsureRequestPayload(c *gin.Context) (*RequestPayload, error) {
	if payload := GetRequestPayload(c.Request.Context()); payload != nil {
		return payload, nil
	}

	payload := &RequestPayload{}
	if c.Request.Body != nil {
		bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, maxRequestPayloadBytes))
		if err != nil {
			return nil, err
		}
		payload.Body = bodyBytes
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		c.Request.ContentLength = int64(len(bodyBytes))
		c.Request.TransferEncoding = nil

		if strings.Contains(c.GetHeader("Content-Type"), "application/json") && len(bodyBytes) > 0 {
			var parsed map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &parsed); err == nil {
				payload.JSON = parsed
			}
		}
	}

	ctx := WithRequestPayload(c.Request.Context(), payload)
	c.Request = c.Request.WithContext(ctx)
	return payload, nil
}

func RestoreRequestBody(req *http.Request, body []byte) {
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.TransferEncoding = nil
}
