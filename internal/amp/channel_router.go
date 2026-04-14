package amp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"ampmanager/internal/billing"
	"ampmanager/internal/billingstate"
	"ampmanager/internal/model"
	"ampmanager/internal/opencc"
	"ampmanager/internal/service"
	"ampmanager/internal/translator"
	"ampmanager/internal/translator/filters"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// sharedChannelTransport 是共享的 Channel Proxy Transport，用于连接复用
var sharedChannelTransport = NewStreamingTransport()

// translationContextKey is used to store translation info in context
type translationContextKey struct{}

// responseWriterContextKey is used to store ResponseWriter in context for SSE keep-alive
type responseWriterContextKey struct{}

// traditionalChineseContextKey is used to flag that Traditional Chinese conversion is active
type traditionalChineseContextKey struct{}

// WithTraditionalChinese stores the traditional Chinese conversion flag in context
func WithTraditionalChinese(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, traditionalChineseContextKey{}, enabled)
}

// GetTraditionalChinese retrieves the traditional Chinese conversion flag from context
func GetTraditionalChinese(ctx context.Context) bool {
	if val := ctx.Value(traditionalChineseContextKey{}); val != nil {
		if enabled, ok := val.(bool); ok {
			return enabled
		}
	}
	return false
}

// WithResponseWriter stores ResponseWriter in context
func WithResponseWriter(ctx context.Context, w http.ResponseWriter) context.Context {
	return context.WithValue(ctx, responseWriterContextKey{}, w)
}

// GetResponseWriter retrieves ResponseWriter from context
func GetResponseWriter(ctx context.Context) http.ResponseWriter {
	if val := ctx.Value(responseWriterContextKey{}); val != nil {
		if w, ok := val.(http.ResponseWriter); ok {
			return w
		}
	}
	return nil
}

// TranslationInfo holds translation state for request/response conversion
type TranslationInfo struct {
	NeedsConversion     bool
	IncomingFormat      translator.Format
	OutgoingFormat      translator.Format
	OriginalRequestBody []byte
	ConvertedBody       []byte
	IsStreaming         bool
	Model               string // originalModel - for response rewriting
	UpstreamModel       string // mappedModel - for upstream URL path
	ResponseParam       *any
}

type channelConfigKey struct{}

type ChannelConfig struct {
	Channel *model.Channel
	Model   string
}

func WithChannelConfig(c *gin.Context, cfg *ChannelConfig) {
	c.Set("channel_config", cfg)
}

func GetChannelConfig(c *gin.Context) *ChannelConfig {
	if val, exists := c.Get("channel_config"); exists {
		if cfg, ok := val.(*ChannelConfig); ok {
			return cfg
		}
	}
	return nil
}

var channelService = service.NewChannelService()

// WithTranslationInfo stores translation info in context
func WithTranslationInfo(ctx context.Context, info *TranslationInfo) context.Context {
	return context.WithValue(ctx, translationContextKey{}, info)
}

// GetTranslationInfo retrieves translation info from context
func GetTranslationInfo(ctx context.Context) *TranslationInfo {
	if val := ctx.Value(translationContextKey{}); val != nil {
		if info, ok := val.(*TranslationInfo); ok {
			return info
		}
	}
	return nil
}

// detectIncomingFormat determines the request format based on the request path
func detectIncomingFormat(path string) translator.Format {
	switch {
	case strings.Contains(path, "/v1/chat/completions"):
		return translator.FormatOpenAIChat
	case strings.Contains(path, "/v1/responses"):
		return translator.FormatOpenAIResponses
	case strings.Contains(path, "/v1/messages"):
		return translator.FormatClaude
	case strings.Contains(path, "/v1beta/models/") || strings.Contains(path, "/v1beta1/publishers/google/models/"):
		return translator.FormatGemini
	default:
		return translator.FormatOpenAI // Default to OpenAI format
	}
}

// channelTypeToFormat converts channel type and endpoint to translator format
func channelTypeToFormat(channel *model.Channel) translator.Format {
	if channel == nil {
		return translator.FormatOpenAI
	}
	switch channel.Type {
	case model.ChannelTypeOpenAI:
		if channel.Endpoint == model.ChannelEndpointResponses {
			return translator.FormatOpenAIResponses
		}
		return translator.FormatOpenAIChat
	case model.ChannelTypeClaude:
		return translator.FormatClaude
	case model.ChannelTypeGemini:
		return translator.FormatGemini
	default:
		return translator.FormatOpenAI
	}
}

// needsFormatConversion checks if request/response format conversion is needed
func needsFormatConversion(incoming, outgoing translator.Format) bool {
	return incoming != outgoing
}

// getTargetEndpointPath returns the correct endpoint path for the target format
func getTargetEndpointPath(targetFormat translator.Format, channel *model.Channel) string {
	switch targetFormat {
	case translator.FormatOpenAI:
		if channel != nil && channel.Endpoint == model.ChannelEndpointResponses {
			return "/v1/responses"
		}
		return "/v1/chat/completions"
	case translator.FormatOpenAIChat:
		return "/v1/chat/completions"
	case translator.FormatOpenAIResponses:
		return "/v1/responses"
	case translator.FormatClaude:
		return "/v1/messages"
	case translator.FormatGemini:
		// Gemini paths are handled separately with model name in path
		return ""
	default:
		return "/v1/chat/completions"
	}
}

// sanitizeURL removes sensitive query parameters from URL for safe logging
func sanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "[invalid-url]"
	}
	q := parsed.Query()
	sensitiveKeys := []string{"key", "api_key", "apikey", "token"}
	for _, k := range sensitiveKeys {
		if q.Has(k) {
			q.Set(k, "[REDACTED]")
		}
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func ChannelRouterMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		modelName := extractModelName(c)
		if modelName == "" {
			c.Next()
			return
		}

		var channel *model.Channel
		var err error
		proxyCfg := GetProxyConfig(c.Request.Context())
		preferredChannelID := GetPreferredChannelID(c)
		if preferredChannelID != "" {
			groupIDs := []string(nil)
			if proxyCfg != nil {
				groupIDs = proxyCfg.GroupIDs
			}
			channel, err = channelService.SelectSpecificChannelForModelWithGroups(preferredChannelID, modelName, groupIDs)
			if err != nil {
				log.Errorf("channel router: failed to select preferred channel %s: %v", preferredChannelID, err)
				c.Next()
				return
			}
			if channel != nil {
				log.Infof("channel router: routing model '%s' to preferred channel '%s' (%s)", modelName, channel.Name, channel.Type)
				WithChannelConfig(c, &ChannelConfig{
					Channel: channel,
					Model:   modelName,
				})
				c.Next()
				return
			}
			log.Warnf("channel router: preferred channel '%s' is unavailable for model '%s', falling back to automatic selection", preferredChannelID, modelName)
		}

		if proxyCfg != nil {
			channel, err = channelService.SelectChannelForModelWithGroups(modelName, proxyCfg.GroupIDs)
		} else {
			channel, err = channelService.SelectChannelForModel(modelName)
		}
		if err != nil {
			log.Errorf("channel router: failed to select channel: %v", err)
			c.Next()
			return
		}

		if channel == nil {
			c.Next()
			return
		}

		log.Infof("channel router: routing model '%s' to channel '%s' (%s)", modelName, channel.Name, channel.Type)
		WithChannelConfig(c, &ChannelConfig{
			Channel: channel,
			Model:   modelName,
		})

		c.Next()
	}
}

// extractModelFromPathPart extracts model name from path segment like "gemini-3-flash:generateContent"
func extractModelFromPathPart(modelPart string) string {
	if idx := strings.Index(modelPart, ":"); idx > 0 {
		return modelPart[:idx]
	}
	if idx := strings.Index(modelPart, "/"); idx > 0 {
		return modelPart[:idx]
	}
	return modelPart
}

func extractModelName(c *gin.Context) string {
	path := c.Request.URL.Path

	// Handle v1beta1/publishers/google/models/ path (used by Amp CLI sub-agents)
	if strings.Contains(path, "/v1beta1/publishers/google/models/") {
		parts := strings.Split(path, "/v1beta1/publishers/google/models/")
		if len(parts) > 1 {
			return extractModelFromPathPart(parts[1])
		}
	}

	// Handle v1beta/models/ path
	if strings.Contains(path, "/v1beta/models/") {
		parts := strings.Split(path, "/v1beta/models/")
		if len(parts) > 1 {
			return extractModelFromPathPart(parts[1])
		}
	}

	payload, err := EnsureRequestPayload(c)
	if err != nil || payload == nil || payload.JSON == nil {
		return ""
	}
	if modelName, ok := payload.JSON["model"].(string); ok {
		return modelName
	}
	return ""
}

// ChannelProxyHandler creates a handler using httputil.ReverseProxy for robust proxying
func ChannelProxyHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Security guard: ensure authentication was performed via proxy middleware
		if GetProxyConfig(c.Request.Context()) == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			return
		}

		channelCfg := GetChannelConfig(c)
		if channelCfg == nil || channelCfg.Channel == nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": "no channel available for this request",
			})
			return
		}

		channel := channelCfg.Channel

		// Use original model from context if mapping was applied, otherwise use channelCfg.Model
		// This ensures response rewriting uses the original requested model name
		originalModel := channelCfg.Model
		mappedModel := channelCfg.Model
		if IsModelMappingApplied(c) {
			if origModel := GetOriginalModel(c); origModel != "" {
				originalModel = origModel
				if m := GetMappedModel(c); m != "" {
					mappedModel = m
				}
				log.Debugf("channel proxy: using original model '%s' for response rewriting (mapped to '%s')", origModel, mappedModel)
			}
		}

		// Detect incoming and outgoing formats - format conversion is NOT supported
		incomingFormat := detectIncomingFormat(c.Request.URL.Path)
		outgoingFormat := channelTypeToFormat(channel)

		// Reject if formats don't match (no translation supported)
		if needsFormatConversion(incomingFormat, outgoingFormat) {
			log.Warnf("channel proxy: format mismatch - incoming %s, channel expects %s (format conversion not supported)", incomingFormat, outgoingFormat)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("format mismatch: request format is %s but channel expects %s. Format conversion is not supported, please use a channel with matching format.", incomingFormat, outgoingFormat),
			})
			return
		}

		// Read and process request body
		var originalRequestBody []byte
		var convertedBody []byte
		clientWantsStream := false
		isStreaming := false
		// Some clients send JSON bodies with chunked transfer encoding (Content-Length = -1).
		// We still need to buffer the body so /v1/responses SSE retry can replay it.
		if c.Request.Body != nil {
			requestPayload, err := EnsureRequestPayload(c)
			if err != nil {
				log.Errorf("channel proxy: failed to read request body: %v", err)
				c.JSON(http.StatusInternalServerError, NewStandardError(http.StatusInternalServerError, "failed to read request body"))
				return
			}
			bodyBytes := requestPayload.Body
			originalRequestBody = bodyBytes
			convertedBody = bodyBytes

			// Check if streaming
			if requestPayload.JSON != nil {
				if stream, ok := requestPayload.JSON["stream"].(bool); ok {
					clientWantsStream = stream
					isStreaming = stream
				}
			}

			// Apply outgoing format filters (e.g., Claude system string to array)
			filteredBody, filterErr := filters.ApplyFilters(outgoingFormat, bodyBytes)
			if filterErr != nil {
				log.Warnf("channel proxy: filter application failed: %v, using unfiltered body", filterErr)
				filteredBody = bodyBytes
			}
			convertedBody = filteredBody

			// CopilotAPI mode: save X-Amp-Thread-Id for x-session-id injection in Director.
			// Also normalize user message string content → array blocks so copilot-api's
			// __SUBAGENT_MARKER__ parser can detect them (it only checks Array.isArray).
			if channel.CopilotAPI {
				if ampThreadID := c.Request.Header.Get("X-Amp-Thread-Id"); ampThreadID != "" {
					c.Request = c.Request.WithContext(
						WithAmpSubagentInfo(c.Request.Context(), &AmpSubagentInfo{ThreadID: ampThreadID}),
					)
				}
				if newBody, normalized := NormalizeUserContentToArray(convertedBody); normalized {
					convertedBody = newBody
				}
			}

			if outgoingFormat == translator.FormatClaude && channel.SimulateCLI {
				// Apply Claude Code simulation filter (body restructuring)
				simFilter := &filters.ClaudeCodeSimulationFilter{}
				if newBody, changed, err := simFilter.Apply(convertedBody); err == nil && changed {
					convertedBody = newBody
				}

				if cfg := GetProxyConfig(c.Request.Context()); cfg != nil {
					if newBody, injected := ensureClaudeMetadataUserID(convertedBody, c.Request.Header.Get("User-Agent"), channel.APIKey); injected {
						convertedBody = newBody
					}
				}

				// Apply system prompt simulation: move original system to messages, replace with official Claude Code system prompt
				if channel.SimulateSystemPrompt {
					convertedBody = applyClaudeCodeSystemPrompt(convertedBody)
				}

				if newBody, toolMap, changed := PrefixClaudeToolNamesWithMap(convertedBody); changed {
					convertedBody = newBody
					if len(toolMap) > 0 {
						c.Request = c.Request.WithContext(WithClaudeToolNameMap(c.Request.Context(), toolMap))
					}
				}
			} else if outgoingFormat == translator.FormatClaude && channel.SimulateSystemPrompt {
				// SimulateCLI is off but SimulateSystemPrompt is independently enabled
				convertedBody = applyClaudeCodeSystemPrompt(convertedBody)
			}

			// Apply Traditional Chinese conversion (S2T) for Claude channels with the feature enabled
			if outgoingFormat == translator.FormatClaude && channel.TraditionalChinese {
				convertedBody = opencc.ConvertClaudeRequestBodyS2T(convertedBody)
				c.Request = c.Request.WithContext(WithTraditionalChinese(c.Request.Context(), true))
				log.Debugf("channel proxy: applied S2T traditional Chinese conversion to request body")
			}

			if !bytes.Equal(convertedBody, bodyBytes) {
				c.Request.Body = io.NopCloser(bytes.NewReader(convertedBody))
				c.Request.ContentLength = int64(len(convertedBody))
				c.Request.Header.Set("Content-Length", fmt.Sprintf("%d", len(convertedBody)))
			} else {
				c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}

			c.Request = c.Request.WithContext(WithStreamMode(c.Request.Context(), StreamMode{
				ClientWantsStream: clientWantsStream,
			}))
		}

		// Store request info in context for response processing
		var responseParam any
		translationInfo := &TranslationInfo{
			NeedsConversion:     false, // No conversion supported
			IncomingFormat:      incomingFormat,
			OutgoingFormat:      outgoingFormat,
			OriginalRequestBody: originalRequestBody,
			ConvertedBody:       convertedBody,
			IsStreaming:         isStreaming,
			Model:               originalModel,
			UpstreamModel:       channelCfg.Model,
			ResponseParam:       &responseParam,
		}
		c.Request = c.Request.WithContext(WithTranslationInfo(c.Request.Context(), translationInfo))

		targetURL, err := buildUpstreamURL(channel, c.Request)
		if err != nil {
			log.Errorf("channel proxy: failed to build upstream URL: %v", err)
			c.JSON(http.StatusInternalServerError, NewStandardError(http.StatusInternalServerError, "failed to build upstream URL"))
			return
		}

		parsed, err := url.Parse(targetURL)
		if err != nil {
			log.Errorf("channel proxy: failed to parse target URL: %v", err)
			c.JSON(http.StatusInternalServerError, NewStandardError(http.StatusInternalServerError, "invalid upstream URL"))
			return
		}

		// Get provider info for token extraction
		providerInfo := ProviderInfoFromChannel(channel)

		// Create RequestTrace for logging (only for model invocations)
		var trace *RequestTrace
		if IsModelInvocation(c.Request.Method, c.Request.URL.Path) {
			if cfg := GetProxyConfig(c.Request.Context()); cfg != nil {
				requestID := GetRequestID(c.Request.Context())
				if requestID == "" {
					requestID = uuid.New().String()
				}
				trace = NewRequestTrace(
					requestID,
					cfg.UserID,
					cfg.APIKeyID,
					c.Request.Method,
					c.Request.URL.Path,
				)
				// Set channel info
				trace.SetChannel(channel.ID, string(channel.Type), channel.BaseURL)
				trace.SetModels(originalModel, mappedModel)
				// Set thinking level if applied
				if thinkingLevel := GetThinkingLevel(c); thinkingLevel != "" {
					trace.SetThinkingLevel(thinkingLevel)
				}
				// Store trace in context
				c.Request = c.Request.WithContext(WithRequestTrace(c.Request.Context(), trace))

				// Write pending record to database immediately
				if writer := GetLogWriter(); writer != nil {
					writer.WritePendingFromTrace(trace)
				}

				// Capture request detail for logging (same as amp upstream proxy)
				if captureData := GetCaptureData(c.Request.Context()); captureData != nil {
					StoreRequestDetail(trace.RequestID, captureData.RequestHeaders, captureData.RequestBody)
				}

				// Store translated request body if different from original
				if len(convertedBody) > 0 && !bytes.Equal(convertedBody, originalRequestBody) {
					StoreTranslatedRequestBody(trace.RequestID, convertedBody)
				}

				log.Infof("channel proxy: model invocation %s %s -> %s (model: %s)", c.Request.Method, c.Request.URL.Path, sanitizeURL(targetURL), originalModel)
			}
		} else {
			log.Debugf("channel proxy: %s %s -> %s (model: %s)", c.Request.Method, c.Request.URL.Path, sanitizeURL(targetURL), originalModel)
		}

		proxy := &httputil.ReverseProxy{
			// 使用共享的流式 Transport，支持连接复用
			Transport: sharedChannelTransport,
			Director: func(req *http.Request) {
				req.URL.Scheme = parsed.Scheme
				req.URL.Host = parsed.Host
				req.URL.Path = parsed.Path
				req.URL.RawQuery = parsed.RawQuery
				req.Host = parsed.Host

				// Inject ProviderInfo into request context for token extraction
				*req = *req.WithContext(WithProviderInfo(req.Context(), providerInfo))

				// Inject ResponseWriter for SSE keep-alive support
				*req = *req.WithContext(WithResponseWriter(req.Context(), c.Writer))

				// Remove client auth headers (ReverseProxy handles hop-by-hop headers automatically)
				req.Header.Del("Authorization")
				req.Header.Del("X-Api-Key")
				req.Header.Del("x-api-key")
				req.Header.Del("X-Goog-Api-Key")
				req.Header.Del("x-goog-api-key")

				// Filter Anthropic-Beta header for local/channel handling paths
				filterAntropicBetaHeader(req)

				// Spoof User-Agent for OpenAI channels to mimic Codex CLI
				if channel.Type == model.ChannelTypeOpenAI {
					req.Header.Set("User-Agent", "codex_exec/0.98.0 (Mac OS 15.1.0; arm64) unknown")
					stripOpenAIUnsupportedFields(req)
				}

				// For OpenAI Chat, inject stream_options.include_usage=true for streaming requests
				if channel.Type == model.ChannelTypeOpenAI && channel.Endpoint != model.ChannelEndpointResponses {
					injectOpenAIStreamOptions(req)
				}

				simulateClaudeCLI := channel.SimulateCLI && channel.Type == model.ChannelTypeClaude
				simulateClaudeUA := channel.SimulateUA && channel.Type == model.ChannelTypeClaude

				// Apply header filtering (whitelist mode) when SimulateCLI is on,
				// with optional UA/X-Stainless spoofing controlled by SimulateUA.
				if simulateClaudeCLI {
					applyClaudeCLISimulation(req, simulateClaudeUA)
				}

				// Apply channel-specific authentication after any strict header rewrite.
				// Claude CLI simulation rebuilds the header set from scratch, so auth must be injected last.
				applyChannelAuth(channel, req)

				// In strict Claude CLI simulation mode we rebuild a fixed header set,
				// so custom channel headers must not be reintroduced afterwards.
				if !simulateClaudeCLI {
					// Apply custom headers from channel config
					var headersMap map[string]string
					if err := json.Unmarshal([]byte(channel.HeadersJSON), &headersMap); err == nil {
						for k, v := range headersMap {
							req.Header.Set(k, v)
						}
					}
				}

				// For Gemini, ensure no conflicting auth headers (but keep x-goog-api-key)
				if channel.Type == model.ChannelTypeGemini {
					req.Header.Del("Authorization")
					req.Header.Del("X-Api-Key")
					req.Header.Del("x-api-key")
				}

				// CopilotAPI mode: X-Amp-Thread-Id → x-session-id
				if ampInfo := GetAmpSubagentInfo(req.Context()); ampInfo != nil && ampInfo.ThreadID != "" {
					req.Header.Set("x-session-id", ampInfo.ThreadID)
				}

				// Capture translated request headers after all Director modifications
				if trace := GetRequestTrace(req.Context()); trace != nil {
					StoreTranslatedRequestHeaders(trace.RequestID, req.Header)
				}
			},
			FlushInterval: -1, // Flush immediately for SSE streaming support
			ModifyResponse: func(resp *http.Response) error {
				trace := GetRequestTrace(resp.Request.Context())
				transInfo := GetTranslationInfo(resp.Request.Context())
				isStreaming := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
				providerInfo, _ := GetProviderInfo(resp.Request.Context())

				// /v1/responses: retry on concurrency-limit / retryable errors.
				// This handles BOTH:
				//   a) HTTP 200 + SSE stream starting with event: error (handled by SSEConcurrencyRetryWrapper)
				//   b) HTTP 429/5xx with JSON/SSE error body (handled here directly)
				isResponsesPath := strings.Contains(resp.Request.URL.Path, "/v1/responses")
				if isResponsesPath {
					log.Debugf("sse-retry: /v1/responses detected, status=%d, streaming=%v, content-type=%s",
						resp.StatusCode, isStreaming, resp.Header.Get("Content-Type"))
					if ti := GetTranslationInfo(resp.Request.Context()); ti != nil && len(ti.ConvertedBody) > 0 {
						retryReq := resp.Request.Clone(resp.Request.Context())
						makeRetryRequest := func() (*http.Response, error) {
							clone := retryReq.Clone(retryReq.Context())
							clone.Body = io.NopCloser(bytes.NewReader(ti.ConvertedBody))
							clone.ContentLength = int64(len(ti.ConvertedBody))
							return sharedChannelTransport.RoundTrip(clone)
						}

						// Case (b): non-2xx status — peek body to check if retryable
						if resp.StatusCode == 429 || resp.StatusCode >= 500 {
							bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
							resp.Body.Close()
							if readErr == nil && isRetryableBytes(bodyBytes) {
								log.Warnf("sse-retry: HTTP %d with retryable error body, starting retries", resp.StatusCode)
								for attempt := 1; attempt <= sseConcurrencyRetryMax; attempt++ {
									wait := sseConcurrencyRetryBaseWait * time.Duration(attempt)
									log.Warnf("sse-retry: attempt %d/%d after %v", attempt, sseConcurrencyRetryMax, wait)
									time.Sleep(wait)
									retryResp, err := makeRetryRequest()
									if err != nil {
										log.Errorf("sse-retry: request failed: %v", err)
										continue
									}
									resp.StatusCode = retryResp.StatusCode
									resp.Status = retryResp.Status
									resp.Header = retryResp.Header
									resp.Body = retryResp.Body
									resp.ContentLength = retryResp.ContentLength
									resp.TransferEncoding = retryResp.TransferEncoding
									isStreaming = strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
									if resp.StatusCode >= 200 && resp.StatusCode < 300 {
										log.Infof("sse-retry: attempt %d succeeded with status %d", attempt, resp.StatusCode)
										break
									}
									// Still error? Read body and check if retryable for next iteration
									bodyBytes, readErr = io.ReadAll(io.LimitReader(resp.Body, 8*1024))
									resp.Body.Close()
									if readErr != nil || !isRetryableBytes(bodyBytes) {
										// Not retryable, reconstruct body and stop
										resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
										break
									}
								}
							} else {
								// Not retryable, reconstruct body so downstream can read it
								resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
							}
						}

						// Case (a): SSE stream — wrap with retry for in-stream errors
						if isStreaming {
							log.Debugf("sse-retry: wrapping SSE stream with concurrency retry wrapper")
							resp.Body = NewSSEConcurrencyRetryWrapper(resp.Body, func() (io.ReadCloser, error) {
								retryResp, err := makeRetryRequest()
								if err != nil {
									return nil, err
								}
								if retryResp.StatusCode < 200 || retryResp.StatusCode >= 300 {
									retryResp.Body.Close()
									return nil, fmt.Errorf("retry returned status %d", retryResp.StatusCode)
								}
								return retryResp.Body, nil
							})
						}
					} else {
						log.Debugf("sse-retry: no ConvertedBody available, cannot retry")
					}
				}

				// /v1/responses: if client requested non-stream but upstream responded with SSE,
				// aggregate the SSE into a single JSON response.
				if isStreaming && strings.Contains(resp.Request.URL.Path, "/v1/responses") {
					if mode, ok := GetStreamMode(resp.Request.Context()); ok && !mode.ClientWantsStream {
						jsonBody, assistantText, aggErr := aggregateOpenAIResponsesSSEToJSON(resp.Request.Context(), resp.Body)
						_ = resp.Body.Close()
						if aggErr != nil {
							return aggErr
						}
						// Store the extracted assistant text in the trace for logging
						if trace != nil && assistantText != "" {
							trace.SetResponseText(assistantText)
						}
						resp.Body = io.NopCloser(bytes.NewReader(jsonBody))
						resp.Header.Set("Content-Type", "application/json")
						resp.Header.Del("Content-Encoding")
						resp.Header.Del("Transfer-Encoding")
						resp.TransferEncoding = nil
						resp.ContentLength = int64(len(jsonBody))
						resp.Header.Set("Content-Length", strconv.Itoa(len(jsonBody)))

						isStreaming = false
					}
				}

				// Log non-2xx responses
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					log.Warnf("channel proxy: upstream returned status %d for %s", resp.StatusCode, sanitizeURL(targetURL))
					if trace != nil {
						trace.SetError("upstream_error")
						resp.Body = NewLoggingBodyWrapper(resp.Body, trace, resp.StatusCode, resp.Request.Context())
					}
					return nil
				}

				// For non-streaming responses, read the complete body upfront,
				// apply all transformations, then reset body with correct Content-Length
				if !isStreaming {
					if err := handleNonStreamingResponse(resp, trace, transInfo, originalModel, mappedModel); err != nil {
						return err
					}
					// Apply T2S Traditional Chinese conversion on the final non-streaming response body
					if GetTraditionalChinese(resp.Request.Context()) {
						bodyBytes, readErr := io.ReadAll(resp.Body)
						if readErr == nil && len(bodyBytes) > 0 {
							bodyBytes = opencc.ConvertClaudeResponseBodyT2S(bodyBytes)
							resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
							resp.ContentLength = int64(len(bodyBytes))
							resp.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
							log.Debugf("channel proxy: applied T2S traditional Chinese conversion to non-streaming response")
						}
					}
					return nil
				}

				resp.Body = NewSSETransformWrapper(resp.Body, func(b []byte) []byte {
					b = TransformResponseJSON(resp.Request.Context(), b, originalModel, mappedModel)
					// Apply T2S Traditional Chinese conversion on each SSE frame
					if GetTraditionalChinese(resp.Request.Context()) {
						b = opencc.ConvertClaudeSSEDataT2S(b)
					}
					return b
				})

				// Streaming response handling (existing logic)
				if trace != nil {
					resp.Body = WrapResponseBodyForTokenExtraction(resp.Body, isStreaming, trace, providerInfo)
					if IsRequestDetailCaptureEnabled(resp.Request.Context()) {
						resp.Body = NewResponseCaptureWrapper(resp.Body, trace.RequestID, resp.Header)
					}
					resp.Body = NewLoggingBodyWrapper(resp.Body, trace, resp.StatusCode, resp.Request.Context())
				}

				// Wrap SSE responses with keep-alive for long-running streams
				if rw := GetResponseWriter(resp.Request.Context()); rw != nil {
					// Check if pseudo-non-stream is enabled
					if GetPseudoNonStream(resp.Request.Context()) {
						var opts []PseudoNonStreamOption
						if kw := GetAuditKeywords(resp.Request.Context()); len(kw) > 0 {
							opts = append(opts, WithAuditKeywordsOption(kw))
						}
						// Build retry function using the request info available in ModifyResponse
						if transInfo := GetTranslationInfo(resp.Request.Context()); transInfo != nil && len(transInfo.ConvertedBody) > 0 {
							retryReq := resp.Request.Clone(resp.Request.Context())
							opts = append(opts, WithRetryFunc(func() (io.ReadCloser, error) {
								clone := retryReq.Clone(retryReq.Context())
								clone.Body = io.NopCloser(bytes.NewReader(transInfo.ConvertedBody))
								clone.ContentLength = int64(len(transInfo.ConvertedBody))
								retryResp, err := sharedChannelTransport.RoundTrip(clone)
								if err != nil {
									return nil, err
								}
								if retryResp.StatusCode < 200 || retryResp.StatusCode >= 300 {
									retryResp.Body.Close()
									return nil, fmt.Errorf("retry returned status %d", retryResp.StatusCode)
								}
								return retryResp.Body, nil
							}))
						}
						resp.Body = NewPseudoNonStreamBodyWrapper(resp.Body, rw, mappedModel, opts...)
						log.Infof("channel proxy: enabled pseudo-non-stream buffering for streaming response (model: %s)", mappedModel)
					} else if wrapper := NewSSEKeepAliveWrapper(resp.Body, rw, resp.Request.Context(), nil); wrapper != nil {
						resp.Body = wrapper
						log.Debugf("channel proxy: enabled SSE keep-alive for streaming response")
					}
				}

				return nil
			},
			ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
				log.Errorf("channel proxy: upstream request failed: %v", err)
				// Update error log (pending record was already written)
				if trace != nil {
					trace.SetError("upstream_request_failed")
					trace.SetResponse(http.StatusBadGateway)
					if writer := GetLogWriter(); writer != nil {
						writer.UpdateFromTrace(trace)
					}
				}
				// 使用清理后的错误消息，防止泄露敏感信息
				safeMsg := SanitizeError(err)
				WriteErrorResponse(rw, http.StatusBadGateway, "Upstream request failed: "+safeMsg)
			},
		}

		proxy.ServeHTTP(c.Writer, c.Request)
		c.Writer.Flush() // 确保非流式响应被发送给客户端
	}
}

func buildUpstreamURL(channel *model.Channel, req *http.Request) (string, error) {
	parsed, err := url.Parse(channel.BaseURL)
	if err != nil {
		return "", err
	}

	upstreamPath := getEndpointPath(channel, req)

	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + upstreamPath
	parsed.RawQuery = req.URL.RawQuery

	if channel.Type == model.ChannelTypeGemini {
		q := parsed.Query()
		q.Set("key", channel.APIKey)
		// For streaming requests, add alt=sse to get SSE format responses
		if strings.Contains(upstreamPath, "streamGenerateContent") {
			q.Set("alt", "sse")
		}
		parsed.RawQuery = q.Encode()
	}

	if channel.Type == model.ChannelTypeClaude {
		q := parsed.Query()
		if q.Get("beta") != "true" {
			q.Set("beta", "true")
			parsed.RawQuery = q.Encode()
		}
	}

	return parsed.String(), nil
}

func getEndpointPath(channel *model.Channel, req *http.Request) string {
	originalPath := req.URL.Path

	// Check if we need format conversion (OpenAI request -> Gemini channel)
	transInfo := GetTranslationInfo(req.Context())

	switch channel.Type {
	case model.ChannelTypeOpenAI:
		if channel.Endpoint == model.ChannelEndpointResponses {
			return "/v1/responses"
		}
		return "/v1/chat/completions"

	case model.ChannelTypeClaude:
		return "/v1/messages"

	case model.ChannelTypeGemini:
		// When format conversion is needed (e.g., OpenAI request -> Gemini channel),
		// we need to construct the correct Gemini path with model name
		if transInfo != nil && transInfo.NeedsConversion {
			incomingFormat := transInfo.IncomingFormat
			// OpenAI/Claude format incoming, need to build Gemini path
			if incomingFormat == translator.FormatOpenAI || incomingFormat == translator.FormatClaude {
				modelName := transInfo.UpstreamModel // 使用 UpstreamModel
				if modelName != "" {
					// 移除可能存在的 models/ 前缀
					modelName = strings.TrimPrefix(modelName, "models/")
					// Gemini uses :streamGenerateContent for streaming, :generateContent for non-streaming
					action := "generateContent"
					if transInfo.IsStreaming {
						action = "streamGenerateContent"
					}
					return fmt.Sprintf("/v1beta/models/%s:%s", modelName, action)
				}
			}
		}

		// Convert v1beta1/publishers/google/models/X:action to v1beta/models/X:action
		if strings.Contains(originalPath, "/v1beta1/publishers/google/models/") {
			parts := strings.Split(originalPath, "/v1beta1/publishers/google/models/")
			if len(parts) > 1 {
				return "/v1beta/models/" + parts[1]
			}
		}
		if strings.Contains(originalPath, "/v1beta/models/") {
			return originalPath
		}
		return originalPath
	}

	return originalPath
}

func applyChannelAuth(channel *model.Channel, req *http.Request) {
	switch channel.Type {
	case model.ChannelTypeOpenAI:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", channel.APIKey))
	case model.ChannelTypeClaude:
		req.Header.Set("x-api-key", channel.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		ensureRequiredAnthropicBetas(req)
	case model.ChannelTypeGemini:
		req.Header.Set("x-goog-api-key", channel.APIKey)
	}
}

// applyClaudeCLISimulation 注入 Claude Code CLI headers。
// 严格白名单模式：除少数协议必需头外，不透传客户端、代理或自定义 headers。
// spoofUA 为 true 时额外注入 User-Agent 和 X-Stainless SDK 指纹。
func applyClaudeCLISimulation(req *http.Request, spoofUA bool) {
	if req == nil {
		return
	}

	contentType := req.Header.Get("Content-Type")
	contentEncoding := req.Header.Get("Content-Encoding")
	contentLength := req.Header.Get("Content-Length")

	req.Header = make(http.Header)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if contentEncoding != "" {
		req.Header.Set("Content-Encoding", contentEncoding)
	}
	if contentLength != "" {
		req.Header.Set("Content-Length", contentLength)
	}

	// User-Agent — 仅在 spoofUA 时伪装为 Claude Code CLI
	if spoofUA {
		req.Header.Set("User-Agent", "claude-cli/2.1.81 (external, cli)")
	}

	// Anthropic 专用 headers
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")
	req.Header.Set("X-App", "cli")
	req.Header.Set("Accept-Language", "*")
	req.Header.Set("Sec-Fetch-Mode", "cors")

	// Anthropic-Beta: 合并已有 betas + 所需 betas
	requiredBetas := []string{
		"claude-code-20250219",
		"interleaved-thinking-2025-05-14",
		"context-management-2025-06-27",
		"prompt-caching-scope-2026-01-05",
		"effort-2025-11-24",
	}
	req.Header.Set("Anthropic-Beta", strings.Join(requiredBetas, ","))

	// X-Stainless SDK 指纹 — 仅在 spoofUA 时注入
	if spoofUA {
		req.Header.Set("X-Stainless-Retry-Count", "0")
		req.Header.Set("X-Stainless-Runtime-Version", "v22.17.0")
		req.Header.Set("X-Stainless-Package-Version", "0.74.0")
		req.Header.Set("X-Stainless-Runtime", "node")
		req.Header.Set("X-Stainless-Lang", "js")
		req.Header.Set("X-Stainless-Arch", mapStainlessArch())
		req.Header.Set("X-Stainless-Os", mapStainlessOS())
		req.Header.Set("X-Stainless-Timeout", "600")
	}

	// 连接和内容 headers
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Accept-Encoding", "gzip, br")
	req.Header.Set("Accept", "application/json")
}

// mapStainlessOS maps runtime.GOOS to Stainless SDK OS names.
func mapStainlessOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "MacOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	case "freebsd":
		return "FreeBSD"
	default:
		return "Other::" + runtime.GOOS
	}
}

// mapStainlessArch maps runtime.GOARCH to Stainless SDK architecture names.
func mapStainlessArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return "other::" + runtime.GOARCH
	}
}

// stripOpenAIUnsupportedFields 从 OpenAI 请求体中移除不支持的字段（max_output_tokens、stream_options）
func stripOpenAIUnsupportedFields(req *http.Request) {
	if req.Body == nil || req.ContentLength == 0 {
		return
	}
	contentType := req.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, 10*1024*1024))
	if err != nil {
		return
	}
	req.Body.Close()

	var payload map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
		return
	}

	modified := false
	if _, exists := payload["max_output_tokens"]; exists {
		delete(payload, "max_output_tokens")
		modified = true
	}
	if _, exists := payload["stream_options"]; exists {
		delete(payload, "stream_options")
		modified = true
	}

	if !modified {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
		return
	}

	newBody, err := json.Marshal(payload)
	if err != nil {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(newBody))
	req.ContentLength = int64(len(newBody))
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(newBody)))
}

// injectOpenAIStreamOptions 为 OpenAI Chat 流式请求注入 stream_options.include_usage=true
// 这是获取 streaming 响应中 usage 数据的必要条件
func injectOpenAIStreamOptions(req *http.Request) {
	if req.Body == nil || req.ContentLength == 0 {
		return
	}

	contentType := req.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, 10*1024*1024))
	if err != nil {
		return
	}
	req.Body.Close()

	var payload map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
		return
	}

	// 检查是否为流式请求
	stream, ok := payload["stream"].(bool)
	if !ok || !stream {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
		return
	}

	// 检查 stream_options 是否已存在
	streamOptions, ok := payload["stream_options"].(map[string]interface{})
	if !ok {
		streamOptions = make(map[string]interface{})
		payload["stream_options"] = streamOptions
	}

	// 设置 include_usage = true
	if _, exists := streamOptions["include_usage"]; !exists {
		streamOptions["include_usage"] = true
		log.Debugf("channel proxy: injected stream_options.include_usage=true for OpenAI streaming")
	}

	newBody, err := json.Marshal(payload)
	if err != nil {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
		return
	}

	req.Body = io.NopCloser(bytes.NewReader(newBody))
	req.ContentLength = int64(len(newBody))
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(newBody)))
}

// findSSEDelimiter finds the earliest SSE delimiter (\n\n or \r\n\r\n) in data
func findSSEDelimiter(data []byte) (idx int, delimLen int) {
	lfIdx := bytes.Index(data, []byte("\n\n"))
	crlfIdx := bytes.Index(data, []byte("\r\n\r\n"))

	if lfIdx == -1 && crlfIdx == -1 {
		return -1, 0
	}
	if lfIdx == -1 {
		return crlfIdx, 4
	}
	if crlfIdx == -1 {
		return lfIdx, 2
	}
	if lfIdx < crlfIdx {
		return lfIdx, 2
	}
	return crlfIdx, 4
}

// MaxNonStreamingResponseSize is the maximum size for non-streaming response body (10MB)
const MaxNonStreamingResponseSize = 10 * 1024 * 1024

// handleNonStreamingResponse reads the complete upstream response, applies transformations,
// and resets resp.Body with correct Content-Length to avoid JSON truncation issues
func handleNonStreamingResponse(resp *http.Response, trace *RequestTrace, transInfo *TranslationInfo, originalModel, mappedModel string) error {
	// Read complete upstream body with size limit
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxNonStreamingResponseSize))
	resp.Body.Close()
	if err != nil {
		log.Errorf("channel proxy: failed to read non-streaming response: %v", err)
		resp.Body = io.NopCloser(bytes.NewReader([]byte(`{"error":"failed to read upstream response"}`)))
		resp.ContentLength = -1
		resp.Header.Del("Content-Length")
		return nil
	}

	// Decompress if needed (supports gzip/br/zstd/deflate)
	contentEncoding := resp.Header.Get("Content-Encoding")
	body = NewGzipDecompressor().Decompress(body, contentEncoding, resp.Header)

	// Extract token usage for logging
	if trace != nil {
		info, _ := GetProviderInfo(resp.Request.Context())
		extractTokenUsageFromBody(body, trace, &info)
	}

	// Apply response-side transformations without touching user-visible free-form text.
	body = TransformResponseJSON(resp.Request.Context(), body, originalModel, mappedModel)

	// Capture response for logging
	if trace != nil && IsRequestDetailCaptureEnabled(resp.Request.Context()) {
		StoreResponseDetail(trace.RequestID, sanitizeHeaders(resp.Header), body)
	}

	// Reset body with correct Content-Length
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Del("Transfer-Encoding")
	resp.TransferEncoding = nil
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))

	// Log completion with cost calculation
	if trace != nil {
		trace.SetResponse(resp.StatusCode)

		if calc := billing.GetCostCalculator(); calc != nil {
			pricingModel := trace.MappedModel
			if pricingModel == "" {
				pricingModel = trace.OriginalModel
			}
			if pricingModel != "" {
				costResult := calc.CalculateFromPointers(
					pricingModel,
					trace.InputTokens,
					trace.OutputTokens,
					trace.CacheReadInputTokens,
					trace.CacheCreationInputTokens,
				)
				if costResult.PriceFound {
					proxyCfg := GetProxyConfig(resp.Request.Context())
					multiplier := 1.0
					if proxyCfg != nil {
						multiplier = proxyCfg.RateMultiplier
						trace.RateMultiplier = multiplier
					}

					if multiplier == 0 {
						trace.SetCost(costResult.CostMicros, costResult.CostUsd, costResult.PricingModel)
					} else {
						adjustedCostMicros := int64(float64(costResult.CostMicros) * multiplier)
						adjustedCostUsd := fmt.Sprintf("%.6f", float64(adjustedCostMicros)/1e6)
						trace.SetCost(adjustedCostMicros, adjustedCostUsd, costResult.PricingModel)

						if proxyCfg != nil && adjustedCostMicros > 0 {
							billingSvc := service.NewBillingService()
							if billingstate.Get() != nil {
								result, err := billingSvc.SettleRequestCostResult(trace.RequestID, proxyCfg.UserID, adjustedCostMicros)
								if err != nil {
									log.Warnf("channel router: failed to settle cost for user %s: %v", proxyCfg.UserID, err)
								} else {
									trace.SetBillingResult(result)
								}
							} else {
								if err := billingSvc.SettleRequestCost(trace.RequestID, proxyCfg.UserID, adjustedCostMicros); err != nil {
									log.Warnf("channel router: failed to settle cost for user %s: %v", proxyCfg.UserID, err)
								}
							}
						}
					}
				}
			}
		}

		if writer := GetLogWriter(); writer != nil {
			writer.UpdateFromTrace(trace)
		}
	}

	return nil
}

// extractTokenUsageFromBody extracts token usage from response body for logging
func extractTokenUsageFromBody(body []byte, trace *RequestTrace, info *ProviderInfo) {
	if len(body) == 0 {
		return
	}

	if info != nil {
		usage := ExtractTokenUsage(body, *info)
		if usage != nil {
			trace.SetUsage(usage.InputTokens, usage.OutputTokens, usage.CacheReadInputTokens, usage.CacheCreationInputTokens)
			log.Debugf("channel proxy: extracted tokens from non-streaming response: input=%v, output=%v, cache_read=%v, cache_creation=%v",
				ptrToInt(usage.InputTokens), ptrToInt(usage.OutputTokens),
				ptrToInt(usage.CacheReadInputTokens), ptrToInt(usage.CacheCreationInputTokens))
			return
		}
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}

	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		return
	}

	var inputTokens, outputTokens int

	if v, ok := usage["input_tokens"].(float64); ok {
		inputTokens = int(v)
	} else if v, ok := usage["prompt_tokens"].(float64); ok {
		inputTokens = int(v)
	}

	if v, ok := usage["output_tokens"].(float64); ok {
		outputTokens = int(v)
	} else if v, ok := usage["completion_tokens"].(float64); ok {
		outputTokens = int(v)
	}

	if inputTokens > 0 || outputTokens > 0 {
		trace.SetUsage(&inputTokens, &outputTokens, nil, nil)
		log.Debugf("channel proxy: extracted tokens from non-streaming response: input=%d, output=%d", inputTokens, outputTokens)
	}
}

// captureResponseForLogging captures the first part of response for debug logging
func captureResponseForLogging(requestID string, header http.Header, body []byte) {
	if log.GetLevel() < log.DebugLevel {
		return
	}

	maxCapture := 2048
	if len(body) > maxCapture {
		log.Debugf("[%s] response (first %d bytes): %s...", requestID, maxCapture, string(body[:maxCapture]))
	} else {
		log.Debugf("[%s] response: %s", requestID, string(body))
	}
}
