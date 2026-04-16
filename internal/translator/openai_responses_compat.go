package translator

import (
	"context"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type openAIResponsesToChatState struct {
	ResponseID                string
	CreatedAt                 int64
	Model                     string
	FunctionCallIndex         int
	HasReceivedArgumentsDelta bool
	HasToolCallAnnounced      bool
}

type chainedResponseParams struct {
	First  any
	Second any
}

func registerSupplementalTransforms(registry *Registry) {
	registry.Register(
		FormatOpenAIChat,
		FormatOpenAIResponses,
		convertOpenAIChatRequestToResponses,
		ResponseTransform{
			Stream:    convertOpenAIResponsesStreamToChat,
			NonStream: convertOpenAIResponsesNonStreamToChat,
		},
	)
	registry.Register(
		FormatClaude,
		FormatOpenAIResponses,
		func(modelName string, rawJSON []byte, stream bool) ([]byte, error) {
			return convertChainedRequestToResponses(registry, FormatClaude, modelName, rawJSON, stream)
		},
		ResponseTransform{
			Stream: func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) ([]string, error) {
				return chainResponsesStream(registry, ctx, model, originalRequestRawJSON, requestRawJSON, rawJSON, param, FormatClaude)
			},
			NonStream: func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) (string, error) {
				return chainResponsesNonStream(registry, ctx, model, originalRequestRawJSON, requestRawJSON, rawJSON, param, FormatClaude)
			},
		},
	)
	registry.Register(
		FormatGemini,
		FormatOpenAIResponses,
		func(modelName string, rawJSON []byte, stream bool) ([]byte, error) {
			return convertChainedRequestToResponses(registry, FormatGemini, modelName, rawJSON, stream)
		},
		ResponseTransform{
			Stream: func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) ([]string, error) {
				return chainResponsesStream(registry, ctx, model, originalRequestRawJSON, requestRawJSON, rawJSON, param, FormatGemini)
			},
			NonStream: func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) (string, error) {
				return chainResponsesNonStream(registry, ctx, model, originalRequestRawJSON, requestRawJSON, rawJSON, param, FormatGemini)
			},
		},
	)
}

func convertChainedRequestToResponses(registry *Registry, sourceFormat Format, modelName string, rawJSON []byte, stream bool) ([]byte, error) {
	chatRequest, err := registry.TranslateRequest(sourceFormat, FormatOpenAIChat, modelName, rawJSON, stream)
	if err != nil {
		return nil, err
	}
	return registry.TranslateRequest(FormatOpenAIChat, FormatOpenAIResponses, modelName, chatRequest, stream)
}

func chainResponsesStream(
	registry *Registry,
	ctx context.Context,
	model string,
	originalRequestRawJSON, requestRawJSON, rawJSON []byte,
	param *any,
	targetFormat Format,
) ([]string, error) {
	stageParams := getChainedResponseParams(param)
	chatRequest, err := registry.TranslateRequest(targetFormat, FormatOpenAIChat, model, originalRequestRawJSON, true)
	if err != nil {
		return nil, err
	}

	chatPayloads, err := registry.TranslateStream(
		ctx,
		FormatOpenAIChat,
		FormatOpenAIResponses,
		model,
		chatRequest,
		requestRawJSON,
		rawJSON,
		&stageParams.First,
	)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(chatPayloads))
	for _, chatPayload := range chatPayloads {
		targetPayloads, targetErr := registry.TranslateStream(
			ctx,
			targetFormat,
			FormatOpenAIChat,
			model,
			originalRequestRawJSON,
			chatRequest,
			[]byte(chatPayload),
			&stageParams.Second,
		)
		if targetErr != nil {
			return nil, targetErr
		}
		out = append(out, targetPayloads...)
	}
	*param = stageParams
	return out, nil
}

func chainResponsesNonStream(
	registry *Registry,
	ctx context.Context,
	model string,
	originalRequestRawJSON, requestRawJSON, rawJSON []byte,
	param *any,
	targetFormat Format,
) (string, error) {
	stageParams := getChainedResponseParams(param)
	chatRequest, err := registry.TranslateRequest(targetFormat, FormatOpenAIChat, model, originalRequestRawJSON, false)
	if err != nil {
		return "", err
	}
	chatPayload, err := registry.TranslateNonStream(
		ctx,
		FormatOpenAIChat,
		FormatOpenAIResponses,
		model,
		chatRequest,
		requestRawJSON,
		rawJSON,
		&stageParams.First,
	)
	if err != nil {
		return "", err
	}
	targetPayload, err := registry.TranslateNonStream(
		ctx,
		targetFormat,
		FormatOpenAIChat,
		model,
		originalRequestRawJSON,
		chatRequest,
		[]byte(chatPayload),
		&stageParams.Second,
	)
	if err != nil {
		return "", err
	}
	*param = stageParams
	return targetPayload, nil
}

func getChainedResponseParams(param *any) *chainedResponseParams {
	if param == nil {
		return &chainedResponseParams{}
	}
	if existing, ok := (*param).(*chainedResponseParams); ok && existing != nil {
		return existing
	}
	stageParams := &chainedResponseParams{}
	*param = stageParams
	return stageParams
}

func convertOpenAIChatRequestToResponses(modelName string, rawJSON []byte, stream bool) ([]byte, error) {
	root := gjson.ParseBytes(rawJSON)
	out := []byte(`{"model":"","input":[],"stream":false}`)
	var err error

	if modelName == "" {
		modelName = root.Get("model").String()
	}
	out, _ = sjson.SetBytes(out, "model", modelName)
	out, _ = sjson.SetBytes(out, "stream", stream)

	if maxTokens := root.Get("max_tokens"); maxTokens.Exists() {
		out, _ = sjson.SetBytes(out, "max_output_tokens", maxTokens.Int())
	}
	if parallelToolCalls := root.Get("parallel_tool_calls"); parallelToolCalls.Exists() {
		out, _ = sjson.SetBytes(out, "parallel_tool_calls", parallelToolCalls.Bool())
	}
	if temperature := root.Get("temperature"); temperature.Exists() {
		out, _ = sjson.SetBytes(out, "temperature", temperature.Value())
	}
	if topP := root.Get("top_p"); topP.Exists() {
		out, _ = sjson.SetBytes(out, "top_p", topP.Value())
	}
	if store := root.Get("store"); store.Exists() {
		out, _ = sjson.SetBytes(out, "store", store.Value())
	}
	if metadata := root.Get("metadata"); metadata.Exists() {
		out, _ = sjson.SetRawBytes(out, "metadata", []byte(metadata.Raw))
	}
	if user := root.Get("user"); user.Exists() {
		out, _ = sjson.SetBytes(out, "user", user.Value())
	}
	if toolChoice := root.Get("tool_choice"); toolChoice.Exists() {
		out, _ = sjson.SetRawBytes(out, "tool_choice", []byte(toolChoice.Raw))
	}

	reasoningEffort := strings.TrimSpace(root.Get("reasoning_effort").String())
	if reasoningEffort == "" {
		reasoningEffort = strings.TrimSpace(root.Get("reasoning.effort").String())
	}
	if reasoningEffort != "" {
		out, _ = sjson.SetBytes(out, "reasoning.effort", reasoningEffort)
	}

	if tools := root.Get("tools"); tools.Exists() && tools.IsArray() {
		for _, tool := range tools.Array() {
			toolType := strings.TrimSpace(tool.Get("type").String())
			switch toolType {
			case "function", "":
				fn := tool.Get("function")
				if !fn.Exists() {
					fn = tool
				}
				responseTool := []byte(`{"type":"function","name":"","description":"","parameters":{}}`)
				if name := fn.Get("name"); name.Exists() {
					responseTool, _ = sjson.SetBytes(responseTool, "name", name.String())
				}
				if description := fn.Get("description"); description.Exists() {
					responseTool, _ = sjson.SetBytes(responseTool, "description", description.String())
				}
				if parameters := fn.Get("parameters"); parameters.Exists() {
					responseTool, _ = sjson.SetRawBytes(responseTool, "parameters", []byte(parameters.Raw))
				}
				out, err = sjson.SetRawBytes(out, "tools.-1", responseTool)
				if err != nil {
					return nil, err
				}
			default:
				out, err = sjson.SetRawBytes(out, "tools.-1", []byte(tool.Raw))
				if err != nil {
					return nil, err
				}
			}
		}
	}

	var instructions []string
	if messages := root.Get("messages"); messages.Exists() && messages.IsArray() {
		for _, message := range messages.Array() {
			role := strings.TrimSpace(message.Get("role").String())
			content := message.Get("content")
			switch role {
			case "system", "developer":
				if text := openAIContentToText(content); text != "" {
					instructions = append(instructions, text)
				}
			case "assistant":
				if toolCalls := message.Get("tool_calls"); toolCalls.Exists() && toolCalls.IsArray() {
					for _, toolCall := range toolCalls.Array() {
						item := []byte(`{"type":"function_call","call_id":"","name":"","arguments":""}`)
						if callID := toolCall.Get("id"); callID.Exists() {
							item, _ = sjson.SetBytes(item, "call_id", callID.String())
						}
						fn := toolCall.Get("function")
						if name := fn.Get("name"); name.Exists() {
							item, _ = sjson.SetBytes(item, "name", name.String())
						}
						if arguments := fn.Get("arguments"); arguments.Exists() {
							item, _ = sjson.SetBytes(item, "arguments", arguments.String())
						}
						out, err = sjson.SetRawBytes(out, "input.-1", item)
						if err != nil {
							return nil, err
						}
					}
				}
				if textItem := openAIMessageToResponsesMessage("assistant", content); len(textItem) > 0 {
					out, err = sjson.SetRawBytes(out, "input.-1", textItem)
					if err != nil {
						return nil, err
					}
				}
			case "tool":
				item := []byte(`{"type":"function_call_output","call_id":"","output":""}`)
				if callID := message.Get("tool_call_id"); callID.Exists() {
					item, _ = sjson.SetBytes(item, "call_id", callID.String())
				}
				item, _ = sjson.SetBytes(item, "output", openAIContentToText(content))
				out, err = sjson.SetRawBytes(out, "input.-1", item)
				if err != nil {
					return nil, err
				}
			default:
				if msg := openAIMessageToResponsesMessage("user", content); len(msg) > 0 {
					out, err = sjson.SetRawBytes(out, "input.-1", msg)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}

	if len(instructions) > 0 {
		out, _ = sjson.SetBytes(out, "instructions", strings.Join(instructions, "\n\n"))
	}

	return out, nil
}

func openAIMessageToResponsesMessage(role string, content gjson.Result) []byte {
	item := []byte(`{"type":"message","role":"","content":[]}`)
	item, _ = sjson.SetBytes(item, "role", role)

	switch {
	case content.IsArray():
		for _, part := range content.Array() {
			partType := strings.TrimSpace(part.Get("type").String())
			switch partType {
			case "image_url":
				imageURL := part.Get("image_url.url").String()
				if imageURL == "" {
					imageURL = part.Get("image_url").String()
				}
				contentPart := []byte(`{"type":"input_image","image_url":""}`)
				contentPart, _ = sjson.SetBytes(contentPart, "image_url", imageURL)
				item, _ = sjson.SetRawBytes(item, "content.-1", contentPart)
			default:
				text := part.Get("text").String()
				if text == "" && partType == "" && part.Type == gjson.String {
					text = part.String()
				}
				if text == "" {
					continue
				}
				contentPart := []byte(`{"type":"input_text","text":""}`)
				if role == "assistant" {
					contentPart = []byte(`{"type":"output_text","text":""}`)
				}
				contentPart, _ = sjson.SetBytes(contentPart, "text", text)
				item, _ = sjson.SetRawBytes(item, "content.-1", contentPart)
			}
		}
	case content.Type == gjson.String:
		part := []byte(`{"type":"input_text","text":""}`)
		if role == "assistant" {
			part = []byte(`{"type":"output_text","text":""}`)
		}
		part, _ = sjson.SetBytes(part, "text", content.String())
		item, _ = sjson.SetRawBytes(item, "content.-1", part)
	default:
		return nil
	}

	return item
}

func openAIContentToText(content gjson.Result) string {
	switch {
	case content.Type == gjson.String:
		return content.String()
	case content.IsArray():
		var parts []string
		for _, item := range content.Array() {
			if text := strings.TrimSpace(item.Get("text").String()); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func convertOpenAIResponsesStreamToChat(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) ([]string, error) {
	if *param == nil {
		*param = &openAIResponsesToChatState{
			FunctionCallIndex: -1,
		}
	}
	state := (*param).(*openAIResponsesToChatState)

	root := gjson.ParseBytes(rawJSON)
	eventType := root.Get("type").String()
	if eventType == "" {
		return nil, nil
	}

	switch eventType {
	case "response.created":
		state.ResponseID = root.Get("response.id").String()
		state.CreatedAt = root.Get("response.created_at").Int()
		state.Model = root.Get("response.model").String()
		return nil, nil
	case "response.output_text.delta":
		return []string{string(buildOpenAIChatChunk(state, func(chunk []byte) []byte {
			chunk, _ = sjson.SetBytes(chunk, "choices.0.delta.role", "assistant")
			chunk, _ = sjson.SetBytes(chunk, "choices.0.delta.content", root.Get("delta").String())
			return chunk
		}))}, nil
	case "response.reasoning_summary_text.delta":
		return []string{string(buildOpenAIChatChunk(state, func(chunk []byte) []byte {
			chunk, _ = sjson.SetBytes(chunk, "choices.0.delta.role", "assistant")
			chunk, _ = sjson.SetBytes(chunk, "choices.0.delta.reasoning_content", root.Get("delta").String())
			return chunk
		}))}, nil
	case "response.reasoning_summary_text.done":
		return []string{string(buildOpenAIChatChunk(state, func(chunk []byte) []byte {
			chunk, _ = sjson.SetBytes(chunk, "choices.0.delta.role", "assistant")
			chunk, _ = sjson.SetBytes(chunk, "choices.0.delta.reasoning_content", "\n\n")
			return chunk
		}))}, nil
	case "response.output_item.added":
		item := root.Get("item")
		if item.Get("type").String() != "function_call" {
			return nil, nil
		}
		state.FunctionCallIndex++
		state.HasReceivedArgumentsDelta = false
		state.HasToolCallAnnounced = true
		return []string{string(buildOpenAIChatChunk(state, func(chunk []byte) []byte {
			toolCall := []byte(`{"index":0,"id":"","type":"function","function":{"name":"","arguments":""}}`)
			toolCall, _ = sjson.SetBytes(toolCall, "index", state.FunctionCallIndex)
			toolCall, _ = sjson.SetBytes(toolCall, "id", item.Get("call_id").String())
			toolCall, _ = sjson.SetBytes(toolCall, "function.name", item.Get("name").String())
			chunk, _ = sjson.SetBytes(chunk, "choices.0.delta.role", "assistant")
			chunk, _ = sjson.SetRawBytes(chunk, "choices.0.delta.tool_calls", []byte(`[]`))
			chunk, _ = sjson.SetRawBytes(chunk, "choices.0.delta.tool_calls.-1", toolCall)
			return chunk
		}))}, nil
	case "response.function_call_arguments.delta":
		state.HasReceivedArgumentsDelta = true
		return []string{string(buildOpenAIChatChunk(state, func(chunk []byte) []byte {
			toolCall := []byte(`{"index":0,"function":{"arguments":""}}`)
			toolCall, _ = sjson.SetBytes(toolCall, "index", state.FunctionCallIndex)
			toolCall, _ = sjson.SetBytes(toolCall, "function.arguments", root.Get("delta").String())
			chunk, _ = sjson.SetRawBytes(chunk, "choices.0.delta.tool_calls", []byte(`[]`))
			chunk, _ = sjson.SetRawBytes(chunk, "choices.0.delta.tool_calls.-1", toolCall)
			return chunk
		}))}, nil
	case "response.function_call_arguments.done":
		if state.HasReceivedArgumentsDelta {
			return nil, nil
		}
		return []string{string(buildOpenAIChatChunk(state, func(chunk []byte) []byte {
			toolCall := []byte(`{"index":0,"function":{"arguments":""}}`)
			toolCall, _ = sjson.SetBytes(toolCall, "index", state.FunctionCallIndex)
			toolCall, _ = sjson.SetBytes(toolCall, "function.arguments", root.Get("arguments").String())
			chunk, _ = sjson.SetRawBytes(chunk, "choices.0.delta.tool_calls", []byte(`[]`))
			chunk, _ = sjson.SetRawBytes(chunk, "choices.0.delta.tool_calls.-1", toolCall)
			return chunk
		}))}, nil
	case "response.output_item.done":
		item := root.Get("item")
		if item.Get("type").String() != "function_call" {
			return nil, nil
		}
		if state.HasToolCallAnnounced {
			state.HasToolCallAnnounced = false
			return nil, nil
		}
		state.FunctionCallIndex++
		return []string{string(buildOpenAIChatChunk(state, func(chunk []byte) []byte {
			toolCall := []byte(`{"index":0,"id":"","type":"function","function":{"name":"","arguments":""}}`)
			toolCall, _ = sjson.SetBytes(toolCall, "index", state.FunctionCallIndex)
			toolCall, _ = sjson.SetBytes(toolCall, "id", item.Get("call_id").String())
			toolCall, _ = sjson.SetBytes(toolCall, "function.name", item.Get("name").String())
			toolCall, _ = sjson.SetBytes(toolCall, "function.arguments", item.Get("arguments").String())
			chunk, _ = sjson.SetBytes(chunk, "choices.0.delta.role", "assistant")
			chunk, _ = sjson.SetRawBytes(chunk, "choices.0.delta.tool_calls", []byte(`[]`))
			chunk, _ = sjson.SetRawBytes(chunk, "choices.0.delta.tool_calls.-1", toolCall)
			return chunk
		}))}, nil
	case "response.completed":
		return []string{string(buildOpenAIChatCompletedChunk(state, root))}, nil
	default:
		return nil, nil
	}
}

func buildOpenAIChatChunk(state *openAIResponsesToChatState, mutate func([]byte) []byte) []byte {
	chunk := []byte(`{"id":"","object":"chat.completion.chunk","created":0,"model":"","choices":[{"index":0,"delta":{},"finish_reason":null}]}`)
	chunk, _ = sjson.SetBytes(chunk, "id", state.ResponseID)
	chunk, _ = sjson.SetBytes(chunk, "created", state.CreatedAt)
	chunk, _ = sjson.SetBytes(chunk, "model", state.Model)
	if mutate != nil {
		chunk = mutate(chunk)
	}
	return chunk
}

func buildOpenAIChatCompletedChunk(state *openAIResponsesToChatState, root gjson.Result) []byte {
	chunk := buildOpenAIChatChunk(state, nil)
	finishReason := "stop"
	if output := root.Get("response.output"); output.Exists() && output.IsArray() {
		for _, item := range output.Array() {
			if item.Get("type").String() == "function_call" {
				finishReason = "tool_calls"
				break
			}
		}
	}
	chunk, _ = sjson.SetBytes(chunk, "choices.0.finish_reason", finishReason)
	if usage := root.Get("response.usage"); usage.Exists() {
		if inputTokens := usage.Get("input_tokens"); inputTokens.Exists() {
			chunk, _ = sjson.SetBytes(chunk, "usage.prompt_tokens", inputTokens.Int())
		}
		if outputTokens := usage.Get("output_tokens"); outputTokens.Exists() {
			chunk, _ = sjson.SetBytes(chunk, "usage.completion_tokens", outputTokens.Int())
		}
		if totalTokens := usage.Get("total_tokens"); totalTokens.Exists() {
			chunk, _ = sjson.SetBytes(chunk, "usage.total_tokens", totalTokens.Int())
		}
		if cachedTokens := usage.Get("input_tokens_details.cached_tokens"); cachedTokens.Exists() {
			chunk, _ = sjson.SetBytes(chunk, "usage.prompt_tokens_details.cached_tokens", cachedTokens.Int())
		}
		if reasoningTokens := usage.Get("output_tokens_details.reasoning_tokens"); reasoningTokens.Exists() {
			chunk, _ = sjson.SetBytes(chunk, "usage.completion_tokens_details.reasoning_tokens", reasoningTokens.Int())
		}
	}
	return chunk
}

func convertOpenAIResponsesNonStreamToChat(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) (string, error) {
	root := gjson.ParseBytes(rawJSON)
	response := root
	if root.Get("type").String() == "response.completed" {
		response = root.Get("response")
	}
	if response.Get("object").String() != "response" && !response.Get("output").Exists() {
		return string(rawJSON), nil
	}

	out := []byte(`{"id":"","object":"chat.completion","created":0,"model":"","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":null,"tool_calls":null},"finish_reason":"stop"}]}`)
	if id := response.Get("id"); id.Exists() {
		out, _ = sjson.SetBytes(out, "id", id.String())
	}
	if model := response.Get("model"); model.Exists() {
		out, _ = sjson.SetBytes(out, "model", model.String())
	}
	if createdAt := response.Get("created_at"); createdAt.Exists() {
		out, _ = sjson.SetBytes(out, "created", createdAt.Int())
	} else {
		out, _ = sjson.SetBytes(out, "created", time.Now().Unix())
	}

	finishReason := "stop"
	var toolCalls [][]byte
	var contentText string
	var reasoningText string

	if output := response.Get("output"); output.Exists() && output.IsArray() {
		for _, item := range output.Array() {
			switch item.Get("type").String() {
			case "reasoning":
				for _, summary := range item.Get("summary").Array() {
					if summary.Get("type").String() == "summary_text" {
						reasoningText = summary.Get("text").String()
						break
					}
				}
			case "message":
				for _, contentItem := range item.Get("content").Array() {
					if contentItem.Get("type").String() == "output_text" {
						contentText = contentItem.Get("text").String()
						break
					}
				}
			case "function_call":
				toolCall := []byte(`{"id":"","type":"function","function":{"name":"","arguments":""}}`)
				if callID := item.Get("call_id"); callID.Exists() {
					toolCall, _ = sjson.SetBytes(toolCall, "id", callID.String())
				}
				if name := item.Get("name"); name.Exists() {
					toolCall, _ = sjson.SetBytes(toolCall, "function.name", name.String())
				}
				if arguments := item.Get("arguments"); arguments.Exists() {
					toolCall, _ = sjson.SetBytes(toolCall, "function.arguments", arguments.String())
				}
				toolCalls = append(toolCalls, toolCall)
			}
		}
	}

	if contentText != "" {
		out, _ = sjson.SetBytes(out, "choices.0.message.role", "assistant")
		out, _ = sjson.SetBytes(out, "choices.0.message.content", contentText)
	}
	if reasoningText != "" {
		out, _ = sjson.SetBytes(out, "choices.0.message.role", "assistant")
		out, _ = sjson.SetBytes(out, "choices.0.message.reasoning_content", reasoningText)
	}
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
		out, _ = sjson.SetBytes(out, "choices.0.message.role", "assistant")
		out, _ = sjson.SetRawBytes(out, "choices.0.message.tool_calls", []byte(`[]`))
		for _, toolCall := range toolCalls {
			out, _ = sjson.SetRawBytes(out, "choices.0.message.tool_calls.-1", toolCall)
		}
	}
	out, _ = sjson.SetBytes(out, "choices.0.finish_reason", finishReason)

	if usage := response.Get("usage"); usage.Exists() {
		if inputTokens := usage.Get("input_tokens"); inputTokens.Exists() {
			out, _ = sjson.SetBytes(out, "usage.prompt_tokens", inputTokens.Int())
		}
		if outputTokens := usage.Get("output_tokens"); outputTokens.Exists() {
			out, _ = sjson.SetBytes(out, "usage.completion_tokens", outputTokens.Int())
		}
		if totalTokens := usage.Get("total_tokens"); totalTokens.Exists() {
			out, _ = sjson.SetBytes(out, "usage.total_tokens", totalTokens.Int())
		}
		if cachedTokens := usage.Get("input_tokens_details.cached_tokens"); cachedTokens.Exists() {
			out, _ = sjson.SetBytes(out, "usage.prompt_tokens_details.cached_tokens", cachedTokens.Int())
		}
		if reasoningTokens := usage.Get("output_tokens_details.reasoning_tokens"); reasoningTokens.Exists() {
			out, _ = sjson.SetBytes(out, "usage.completion_tokens_details.reasoning_tokens", reasoningTokens.Int())
		}
	}

	return string(out), nil
}
