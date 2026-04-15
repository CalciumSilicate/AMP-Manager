package translator

import (
	"context"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	_ "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator/builtin"
)

// Registry wraps the CLIProxyAPI SDK registry so the rest of AMP Manager can keep
// using its existing translator package surface.
type Registry struct {
	inner *sdktranslator.Registry
}

// NewRegistry returns a wrapper around the SDK default registry.
func NewRegistry() *Registry {
	registry := &Registry{inner: sdktranslator.Default()}
	registerSupplementalTransforms(registry)
	return registry
}

// Register attaches custom transforms to the shared registry.
func (r *Registry) Register(from, to Format, request RequestTransform, response ResponseTransform) {
	if r == nil {
		return
	}

	var requestAdapter sdktranslator.RequestTransform
	if request != nil {
		requestAdapter = func(model string, rawJSON []byte, stream bool) []byte {
			out, err := request(model, rawJSON, stream)
			if err != nil {
				return rawJSON
			}
			return out
		}
	}

	var responseAdapter sdktranslator.ResponseTransform
	if response.Stream != nil {
		responseAdapter.Stream = func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
			chunks, err := response.Stream(ctx, model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
			if err != nil {
				return [][]byte{rawJSON}
			}
			out := make([][]byte, 0, len(chunks))
			for _, chunk := range chunks {
				out = append(out, []byte(chunk))
			}
			return out
		}
	}
	if response.NonStream != nil {
		responseAdapter.NonStream = func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
			out, err := response.NonStream(ctx, model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
			if err != nil {
				return rawJSON
			}
			return []byte(out)
		}
	}
	if response.TokenCount != nil {
		responseAdapter.TokenCount = func(ctx context.Context, count int64) []byte {
			return []byte(response.TokenCount(ctx, count))
		}
	}

	r.inner.Register(canonicalSDKFormat(from), canonicalSDKFormat(to), requestAdapter, responseAdapter)
}

// TranslateRequest converts a request from client format to upstream format.
func (r *Registry) TranslateRequest(from, to Format, model string, rawJSON []byte, stream bool) ([]byte, error) {
	if r == nil {
		return rawJSON, nil
	}
	return r.inner.TranslateRequest(canonicalSDKFormat(from), canonicalSDKFormat(to), model, rawJSON, stream), nil
}

// HasResponseTransformer indicates whether a response translator exists for the request pair.
func (r *Registry) HasResponseTransformer(from, to Format) bool {
	if r == nil {
		return false
	}
	return r.inner.HasResponseTransformer(canonicalSDKFormat(from), canonicalSDKFormat(to))
}

// TranslateStream converts a streaming upstream response back to the client format.
func (r *Registry) TranslateStream(ctx context.Context, from, to Format, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) ([]string, error) {
	if r == nil {
		return []string{string(rawJSON)}, nil
	}
	chunks := r.inner.TranslateStream(ctx, canonicalSDKFormat(to), canonicalSDKFormat(from), model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, string(chunk))
	}
	return out, nil
}

// TranslateNonStream converts a non-streaming upstream response back to the client format.
func (r *Registry) TranslateNonStream(ctx context.Context, from, to Format, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) (string, error) {
	if r == nil {
		return string(rawJSON), nil
	}
	out := r.inner.TranslateNonStream(ctx, canonicalSDKFormat(to), canonicalSDKFormat(from), model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
	return string(out), nil
}

// TranslateTokenCount converts token-count payloads back to the client format.
func (r *Registry) TranslateTokenCount(ctx context.Context, from, to Format, count int64, rawJSON []byte) string {
	if r == nil {
		return string(rawJSON)
	}
	return string(r.inner.TranslateTokenCount(ctx, canonicalSDKFormat(to), canonicalSDKFormat(from), count, rawJSON))
}

var defaultRegistry = NewRegistry()

// DefaultRegistry returns the shared registry wrapper.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// Default exposes the package-level registry for shared use.
func Default() *Registry {
	return DefaultRegistry()
}

// Register attaches transforms to the default registry.
func Register(from, to Format, request RequestTransform, response ResponseTransform) {
	DefaultRegistry().Register(from, to, request, response)
}

// TranslateRequest is a helper on the default registry.
func TranslateRequest(from, to Format, model string, rawJSON []byte, stream bool) ([]byte, error) {
	return DefaultRegistry().TranslateRequest(from, to, model, rawJSON, stream)
}

// HasResponseTransformer inspects the default registry.
func HasResponseTransformer(from, to Format) bool {
	return DefaultRegistry().HasResponseTransformer(from, to)
}

// TranslateStream is a helper on the default registry.
func TranslateStream(ctx context.Context, from, to Format, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) ([]string, error) {
	return DefaultRegistry().TranslateStream(ctx, from, to, model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}

// TranslateNonStream is a helper on the default registry.
func TranslateNonStream(ctx context.Context, from, to Format, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) (string, error) {
	return DefaultRegistry().TranslateNonStream(ctx, from, to, model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}

// TranslateTokenCount is a helper on the default registry.
func TranslateTokenCount(ctx context.Context, from, to Format, count int64, rawJSON []byte) string {
	return DefaultRegistry().TranslateTokenCount(ctx, from, to, count, rawJSON)
}
