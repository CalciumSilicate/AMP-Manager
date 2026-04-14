package amp

import "context"

type requestIDContextKey struct{}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func GetRequestID(ctx context.Context) string {
	if val := ctx.Value(requestIDContextKey{}); val != nil {
		if requestID, ok := val.(string); ok {
			return requestID
		}
	}
	return ""
}
