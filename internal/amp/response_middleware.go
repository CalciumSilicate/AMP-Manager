package amp

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	log "github.com/sirupsen/logrus"
)

// ResponseContext 响应处理上下文
type ResponseContext struct {
	Ctx        context.Context
	Trace      *RequestTrace
	Provider   ProviderInfo
	Headers    http.Header
	StatusCode int
	RequestID  string
}

// StreamingMiddleware 流式响应中间件接口
type StreamingMiddleware interface {
	WrapReader(reader io.ReadCloser, ctx *ResponseContext) io.ReadCloser
}

// NonStreamingMiddleware 非流式响应中间件接口
type NonStreamingMiddleware interface {
	ProcessBody(body []byte, ctx *ResponseContext) ([]byte, error)
}

// StreamingPipeline 流式响应处理管道
type StreamingPipeline struct {
	middlewares []StreamingMiddleware
}

// NewStreamingPipeline 创建流式响应管道
func NewStreamingPipeline(middlewares ...StreamingMiddleware) *StreamingPipeline {
	return &StreamingPipeline{middlewares: middlewares}
}

// ProcessStreamingResponse 处理流式响应
func (p *StreamingPipeline) ProcessStreamingResponse(resp *http.Response, ctx *ResponseContext) error {
	if ctx.Trace != nil {
		ctx.Trace.SetStreaming(true)
	}

	reader := resp.Body
	for _, mw := range p.middlewares {
		reader = mw.WrapReader(reader, ctx)
	}
	resp.Body = reader
	return nil
}

// NonStreamingPipeline 非流式响应处理管道
type NonStreamingPipeline struct {
	decompressor *GzipDecompressor
	middlewares  []NonStreamingMiddleware
}

// NewNonStreamingPipeline 创建非流式响应管道
func NewNonStreamingPipeline(decompressor *GzipDecompressor, middlewares ...NonStreamingMiddleware) *NonStreamingPipeline {
	return &NonStreamingPipeline{
		decompressor: decompressor,
		middlewares:  middlewares,
	}
}

// ProcessNonStreamingResponse 处理非流式响应
func (p *NonStreamingPipeline) ProcessNonStreamingResponse(resp *http.Response, ctx *ResponseContext) error {
	originalBody := resp.Body
	contentEncoding := resp.Header.Get("Content-Encoding")

	// 读取完整响应体
	const maxResponseSize = 10 * 1024 * 1024
	compressedData, err := io.ReadAll(io.LimitReader(originalBody, maxResponseSize+1))
	_ = originalBody.Close()
	if err != nil {
		log.Warnf("amp proxy: failed to read response body: %v", err)
		resp.Body = io.NopCloser(bytes.NewReader(compressedData))
		if ctx.Trace != nil {
			resp.Body = NewLoggingBodyWrapper(resp.Body, ctx.Trace, resp.StatusCode, ctx.Ctx)
		}
		return nil
	}

	// 检测响应是否过大
	if len(compressedData) > maxResponseSize {
		log.Warnf("amp proxy: response too large (%d bytes), skipping token extraction", len(compressedData))
		resp.Body = io.NopCloser(bytes.NewReader(compressedData[:maxResponseSize]))
		resp.ContentLength = int64(maxResponseSize)
		resp.Header.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		if ctx.Trace != nil {
			resp.Body = NewLoggingBodyWrapper(resp.Body, ctx.Trace, resp.StatusCode, ctx.Ctx)
		}
		return nil
	}

	// 解压响应体
	bodyData := p.decompressor.Decompress(compressedData, contentEncoding, resp.Header)

	// 应用中间件链
	for _, mw := range p.middlewares {
		bodyData, err = mw.ProcessBody(bodyData, ctx)
		if err != nil {
			log.Warnf("amp proxy: middleware error: %v", err)
		}
	}

	// 设置响应体
	resp.Body = io.NopCloser(bytes.NewReader(bodyData))
	resp.ContentLength = int64(len(bodyData))
	resp.Header.Del("Content-Length")
	resp.Header.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))

	// 添加日志包装
	if ctx.Trace != nil {
		resp.Body = NewLoggingBodyWrapper(resp.Body, ctx.Trace, resp.StatusCode, ctx.Ctx)
	}

	return nil
}

// ========== 流式中间件实现 ==========

// HealthCheckMiddleware 健康检查中间件
type HealthCheckMiddleware struct{}

func (m *HealthCheckMiddleware) WrapReader(reader io.ReadCloser, ctx *ResponseContext) io.ReadCloser {
	if ctx.Trace == nil {
		return reader
	}
	return NewHealthyStreamWrapper(
		ctx.Ctx,
		reader,
		ctx.Trace,
		DefaultConnectionHealthConfig(),
	)
}

// TokenExtractionMiddleware token 提取中间件
type TokenExtractionMiddleware struct{}

func (m *TokenExtractionMiddleware) WrapReader(reader io.ReadCloser, ctx *ResponseContext) io.ReadCloser {
	if ctx.Trace == nil {
		return reader
	}
	return NewSSETokenExtractor(reader, ctx.Trace, ctx.Provider)
}

// ResponseCaptureMiddleware 响应捕获中间件
type ResponseCaptureMiddleware struct{}

func (m *ResponseCaptureMiddleware) WrapReader(reader io.ReadCloser, ctx *ResponseContext) io.ReadCloser {
	if ctx.RequestID == "" || !IsRequestDetailCaptureEnabled(ctx.Ctx) {
		return reader
	}
	return NewResponseCaptureWrapper(reader, ctx.RequestID, ctx.Headers)
}

// LoggingMiddleware 日志中间件
type LoggingMiddleware struct{}

func (m *LoggingMiddleware) WrapReader(reader io.ReadCloser, ctx *ResponseContext) io.ReadCloser {
	if ctx.Trace == nil {
		return reader
	}
	return NewLoggingBodyWrapper(reader, ctx.Trace, ctx.StatusCode, ctx.Ctx)
}

// ========== 非流式中间件实现 ==========

// TokenUsageMiddleware token 使用量提取中间件
type TokenUsageMiddleware struct{}

func (m *TokenUsageMiddleware) ProcessBody(body []byte, ctx *ResponseContext) ([]byte, error) {
	if ctx.Trace != nil && len(body) > 0 {
		if usage := ExtractTokenUsage(body, ctx.Provider); usage != nil {
			ctx.Trace.SetUsage(usage.InputTokens, usage.OutputTokens, usage.CacheReadInputTokens, usage.CacheCreationInputTokens)
			log.Debugf("amp proxy: extracted non-streaming token usage - input=%v, output=%v",
				ptrToInt(usage.InputTokens), ptrToInt(usage.OutputTokens))
		}
	}
	return body, nil
}

// ResponseStorageMiddleware 响应存储中间件
type ResponseStorageMiddleware struct{}

func (m *ResponseStorageMiddleware) ProcessBody(body []byte, ctx *ResponseContext) ([]byte, error) {
	if ctx.RequestID != "" && len(body) > 0 && IsRequestDetailCaptureEnabled(ctx.Ctx) {
		StoreResponseDetail(ctx.RequestID, sanitizeHeaders(ctx.Headers), body)
	}
	return body, nil
}

// ToolNamePrefixStripMiddleware strips mcp_ tool name prefix from Anthropic responses.
// This mirrors amp_processor.rs strip_mcp_name_prefix_bytes behavior.
type ToolNamePrefixStripMiddleware struct{}

func (m *ToolNamePrefixStripMiddleware) ProcessBody(body []byte, ctx *ResponseContext) ([]byte, error) {
	if ctx.Provider.Provider != ProviderAnthropic {
		return body, nil
	}
	toolMap, ok := GetClaudeToolNameMap(ctx.Ctx)
	if !ok || len(toolMap) == 0 {
		return body, nil
	}
	if unprefixed, changed := UnprefixClaudeToolNamesWithMap(body, toolMap); changed {
		return unprefixed, nil
	}
	return body, nil
}

// ========== Gzip 解压器 ==========

// GzipDecompressor gzip 解压器
type GzipDecompressor struct {
	maxDecompressedSize int64
}

// NewGzipDecompressor 创建解压器
func NewGzipDecompressor() *GzipDecompressor {
	return &GzipDecompressor{
		maxDecompressedSize: 50 * 1024 * 1024, // 50MB
	}
}

// Decompress 解压数据，支持 gzip/br/zstd/deflate
func (d *GzipDecompressor) Decompress(data []byte, contentEncoding string, header http.Header) []byte {
	switch strings.ToLower(contentEncoding) {
	case "gzip":
		return d.decompressGzip(data, header, true)
	case "br":
		return d.decompressBrotli(data, header)
	case "zstd":
		return d.decompressZstd(data, header)
	case "deflate":
		return d.decompressDeflate(data, header)
	case "":
		// 检测隐式 gzip
		if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
			return d.decompressGzip(data, header, false)
		}
		return data
	default:
		log.Warnf("amp proxy: unsupported Content-Encoding: %s, passing through raw data", contentEncoding)
		return data
	}
}

func (d *GzipDecompressor) decompressGzip(data []byte, header http.Header, explicitGzip bool) []byte {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		log.Warnf("amp proxy: failed to create gzip reader: %v", err)
		return data
	}
	defer gzipReader.Close()

	decompressed, err := io.ReadAll(io.LimitReader(gzipReader, d.maxDecompressedSize+1))
	if err != nil {
		log.Warnf("amp proxy: failed to decompress gzip: %v", err)
		return data
	}

	if int64(len(decompressed)) > d.maxDecompressedSize {
		log.Warnf("amp proxy: decompressed response too large (%d bytes), skipping token extraction", len(decompressed))
		return data
	}

	if explicitGzip {
		header.Del("Content-Encoding")
	}
	log.Debugf("amp proxy: decompressed gzip response (%d -> %d bytes)", len(data), len(decompressed))
	return decompressed
}

func (d *GzipDecompressor) decompressBrotli(data []byte, header http.Header) []byte {
	reader := brotli.NewReader(bytes.NewReader(data))
	decompressed, err := io.ReadAll(io.LimitReader(reader, d.maxDecompressedSize+1))
	if err != nil {
		log.Warnf("amp proxy: failed to decompress brotli: %v", err)
		return data
	}
	if int64(len(decompressed)) > d.maxDecompressedSize {
		log.Warnf("amp proxy: brotli decompressed response too large (%d bytes)", len(decompressed))
		return data
	}
	header.Del("Content-Encoding")
	log.Debugf("amp proxy: decompressed brotli response (%d -> %d bytes)", len(data), len(decompressed))
	return decompressed
}

func (d *GzipDecompressor) decompressZstd(data []byte, header http.Header) []byte {
	decoder, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		log.Warnf("amp proxy: failed to create zstd reader: %v", err)
		return data
	}
	defer decoder.Close()
	decompressed, err := io.ReadAll(io.LimitReader(decoder, d.maxDecompressedSize+1))
	if err != nil {
		log.Warnf("amp proxy: failed to decompress zstd: %v", err)
		return data
	}
	if int64(len(decompressed)) > d.maxDecompressedSize {
		log.Warnf("amp proxy: zstd decompressed response too large (%d bytes)", len(decompressed))
		return data
	}
	header.Del("Content-Encoding")
	log.Debugf("amp proxy: decompressed zstd response (%d -> %d bytes)", len(data), len(decompressed))
	return decompressed
}

func (d *GzipDecompressor) decompressDeflate(data []byte, header http.Header) []byte {
	reader := flate.NewReader(bytes.NewReader(data))
	defer reader.Close()
	decompressed, err := io.ReadAll(io.LimitReader(reader, d.maxDecompressedSize+1))
	if err != nil {
		log.Warnf("amp proxy: failed to decompress deflate: %v", err)
		return data
	}
	if int64(len(decompressed)) > d.maxDecompressedSize {
		log.Warnf("amp proxy: deflate decompressed response too large (%d bytes)", len(decompressed))
		return data
	}
	header.Del("Content-Encoding")
	log.Debugf("amp proxy: decompressed deflate response (%d -> %d bytes)", len(data), len(decompressed))
	return decompressed
}

// ========== 默认管道构建器 ==========

// StreamingPipelineWithContext 带上下文的流式管道
type StreamingPipelineWithContext struct {
	ctx context.Context
}

// NewStreamingPipelineWithContext 创建带上下文的流式管道
func NewStreamingPipelineWithContext(ctx context.Context) *StreamingPipelineWithContext {
	return &StreamingPipelineWithContext{ctx: ctx}
}

// ProcessStreamingResponse 处理流式响应
func (p *StreamingPipelineWithContext) ProcessStreamingResponse(resp *http.Response, ctx *ResponseContext) error {
	if ctx.Trace == nil {
		return nil
	}

	ctx.Trace.SetStreaming(true)

	// 1. 健康检查包装器（最内层）
	healthWrapper := NewHealthyStreamWrapper(
		p.ctx,
		resp.Body,
		ctx.Trace,
		DefaultConnectionHealthConfig(),
	)
	// 2. Token 提取器
	tokenExtractor := NewSSETokenExtractor(healthWrapper, ctx.Trace, ctx.Provider)
	streamBody := io.ReadCloser(tokenExtractor)
	if ctx.RequestID != "" && IsRequestDetailCaptureEnabled(ctx.Ctx) {
		streamBody = NewResponseCaptureWrapper(tokenExtractor, ctx.RequestID, ctx.Headers)
	}
	// 4. 日志包装器（最外层）
	resp.Body = NewLoggingBodyWrapper(streamBody, ctx.Trace, resp.StatusCode, ctx.Ctx)

	return nil
}

// NewDefaultStreamingPipeline 创建默认流式处理管道（无 context 版本）
// 注意：推荐使用 NewStreamingPipelineWithContext
func NewDefaultStreamingPipeline() *StreamingPipeline {
	return NewStreamingPipeline(
		&HealthCheckMiddleware{},
		&TokenExtractionMiddleware{},
		&ResponseCaptureMiddleware{},
		&LoggingMiddleware{},
	)
}

// NewDefaultNonStreamingPipeline 创建默认非流式处理管道
func NewDefaultNonStreamingPipeline() *NonStreamingPipeline {
	return NewNonStreamingPipeline(
		NewGzipDecompressor(),
		&TokenUsageMiddleware{},
		&ToolNamePrefixStripMiddleware{},
		&ResponseStorageMiddleware{},
	)
}
