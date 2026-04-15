// Package translator provides types and functions for converting requests and responses between schemas.
package translator

import "context"

// RequestTransform converts a request payload from a source schema to a target schema.
type RequestTransform func(model string, rawJSON []byte, stream bool) ([]byte, error)

// ResponseStreamTransform converts a streaming response from an upstream schema back to the client schema.
type ResponseStreamTransform func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) ([]string, error)

// ResponseNonStreamTransform converts a non-streaming response from an upstream schema back to the client schema.
type ResponseNonStreamTransform func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) (string, error)

// ResponseTokenCountTransform transforms a token count from a source format to a target format.
type ResponseTokenCountTransform func(ctx context.Context, count int64) string

// ResponseTransform groups the functions for transforming streaming and non-streaming responses.
type ResponseTransform struct {
	Stream     ResponseStreamTransform
	NonStream  ResponseNonStreamTransform
	TokenCount ResponseTokenCountTransform
}
