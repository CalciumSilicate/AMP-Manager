package amp

import "context"

type streamModeKey struct{}

type StreamMode struct {
	ClientWantsStream bool
}

func WithStreamMode(ctx context.Context, m StreamMode) context.Context {
	return context.WithValue(ctx, streamModeKey{}, m)
}

func GetStreamMode(ctx context.Context) (StreamMode, bool) {
	val := ctx.Value(streamModeKey{})
	if val == nil {
		return StreamMode{}, false
	}
	m, ok := val.(StreamMode)
	return m, ok
}
