package amp

import (
	"encoding/json"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// UsageParser 提供商 token 使用量解析接口
type UsageParser interface {
	// ConsumeSSE 解析 SSE 事件数据
	// eventName: SSE event: 行的值（可能为空）
	// data: data: 行后的 JSON bytes
	// 返回: usage（可能为 nil）, 是否为最终事件, 是否成功解析
	ConsumeSSE(eventName string, data []byte) (usage *TokenUsage, final bool, ok bool)

	// ParseResponse 解析非流式响应体
	ParseResponse(body []byte) (usage *TokenUsage, ok bool)
}

// NewUsageParser 根据 ProviderInfo 创建对应的解析器
func NewUsageParser(info ProviderInfo) UsageParser {
	switch info.Provider {
	case ProviderAnthropic:
		return &anthropicParser{}
	case ProviderOpenAIChat:
		return &openAIChatParser{}
	case ProviderOpenAIResponses:
		return &openAIResponsesParser{}
	case ProviderGemini:
		return &geminiParser{}
	default:
		return &anthropicParser{}
	}
}

// intPtr 辅助函数，创建 int 指针
func intPtr(v int) *int {
	return &v
}

// ========== Anthropic Parser ==========

type anthropicParser struct {
	cur TokenUsage
}

type anthropicSSEEvent struct {
	Type    string `json:"type"`
	Message *struct {
		Usage *TokenUsage `json:"usage,omitempty"`
	} `json:"message,omitempty"`
	Usage *TokenUsage `json:"usage,omitempty"`
}

func (p *anthropicParser) ConsumeSSE(eventName string, data []byte) (*TokenUsage, bool, bool) {
	var ev anthropicSSEEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return nil, false, false
	}

	switch ev.Type {
	case "message_start":
		// 重置状态，防止旧值泄漏
		p.cur = TokenUsage{}
		if ev.Message != nil && ev.Message.Usage != nil {
			u := ev.Message.Usage
			p.cur.InputTokens = u.InputTokens
			p.cur.CacheReadInputTokens = u.CacheReadInputTokens
			p.cur.CacheCreationInputTokens = u.CacheCreationInputTokens
			log.Debugf("usage parser [anthropic]: message_start - input=%v, cache_read=%v, cache_creation=%v",
				ptrToInt(u.InputTokens), ptrToInt(u.CacheReadInputTokens), ptrToInt(u.CacheCreationInputTokens))
		}
		// 不在 message_start 时返回 usage，等待 message_delta 获取最终值
		return nil, false, false
	case "message_delta":
		if ev.Usage != nil {
			if ev.Usage.InputTokens != nil {
				p.cur.InputTokens = ev.Usage.InputTokens
			}
			if ev.Usage.OutputTokens != nil {
				p.cur.OutputTokens = ev.Usage.OutputTokens
			}
			if ev.Usage.CacheReadInputTokens != nil {
				p.cur.CacheReadInputTokens = ev.Usage.CacheReadInputTokens
			}
			if ev.Usage.CacheCreationInputTokens != nil {
				p.cur.CacheCreationInputTokens = ev.Usage.CacheCreationInputTokens
			}
			log.Debugf("usage parser [anthropic]: message_delta - input=%v, output=%v, cache_read=%v, cache_creation=%v",
				ptrToInt(ev.Usage.InputTokens), ptrToInt(ev.Usage.OutputTokens), ptrToInt(ev.Usage.CacheReadInputTokens), ptrToInt(ev.Usage.CacheCreationInputTokens))
			// 返回副本而非指针，标记为 final
			usage := p.cur
			return &usage, true, true
		}
	}
	return nil, false, false
}

func (p *anthropicParser) ParseResponse(body []byte) (*TokenUsage, bool) {
	var resp struct {
		Usage *TokenUsage `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Usage == nil {
		return nil, false
	}
	return resp.Usage, true
}

// ========== OpenAI Chat Completions Parser ==========

type openAIChatParser struct{}

type openAIChatUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails *struct {
		CachedTokens *int `json:"cached_tokens,omitempty"`
	} `json:"prompt_tokens_details,omitempty"`
}

func (p *openAIChatParser) ConsumeSSE(eventName string, data []byte) (*TokenUsage, bool, bool) {
	usage, ok := parseOpenAIChatUsageBytes(data)
	if !ok {
		return nil, false, false
	}

	log.Debugf("usage parser [openai_chat]: usage chunk - input=%d, output=%d, cache_read=%v",
		ptrToInt(usage.InputTokens), ptrToInt(usage.OutputTokens), ptrToInt(usage.CacheReadInputTokens))

	return usage, true, true
}

func (p *openAIChatParser) ParseResponse(body []byte) (*TokenUsage, bool) {
	return parseOpenAIChatUsageBytes(body)
}

// ========== OpenAI Responses API Parser ==========

type openAIResponsesParser struct{}

type openAIResponsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails *struct {
		CachedTokens *int `json:"cached_tokens,omitempty"`
	} `json:"input_tokens_details,omitempty"`
}

func (p *openAIResponsesParser) ConsumeSSE(eventName string, data []byte) (*TokenUsage, bool, bool) {
	eventType := gjson.GetBytes(data, "type").String()
	isCompleted := eventName == "response.completed" || eventType == "response.completed"
	if !isCompleted {
		return nil, false, false
	}

	usage, ok := parseOpenAIResponsesUsageBytes(data)
	if !ok {
		usage, ok = parseOpenAIResponsesUsageBytes([]byte(gjson.GetBytes(data, "response").Raw))
		if !ok {
			return nil, false, false
		}
	}

	log.Debugf("usage parser [openai_responses]: response.completed - input=%d, output=%d, cache_read=%v",
		ptrToInt(usage.InputTokens), ptrToInt(usage.OutputTokens), ptrToInt(usage.CacheReadInputTokens))

	return usage, true, true
}

func (p *openAIResponsesParser) ParseResponse(body []byte) (*TokenUsage, bool) {
	return parseOpenAIResponsesUsageBytes(body)
}

// ========== Gemini Parser ==========

type geminiParser struct{}

type geminiUsageMetadata struct {
	PromptTokenCount        int  `json:"promptTokenCount"`
	CandidatesTokenCount    int  `json:"candidatesTokenCount"`
	TotalTokenCount         int  `json:"totalTokenCount"`
	CachedContentTokenCount *int `json:"cachedContentTokenCount,omitempty"`
}

func (p *geminiParser) ConsumeSSE(eventName string, data []byte) (*TokenUsage, bool, bool) {
	usage, ok := parseGeminiUsageMetadataBytes(data)
	if !ok {
		return nil, false, false
	}

	isFinal := false
	gjson.GetBytes(data, "candidates").ForEach(func(_, value gjson.Result) bool {
		if value.Get("finishReason").String() != "" {
			isFinal = true
		}
		return true
	})

	log.Debugf("usage parser [gemini]: usageMetadata - input=%d, output=%d, cache_read=%v, final=%v",
		ptrToInt(usage.InputTokens), ptrToInt(usage.OutputTokens), ptrToInt(usage.CacheReadInputTokens), isFinal)

	return usage, isFinal, true
}

func (p *geminiParser) ParseResponse(body []byte) (*TokenUsage, bool) {
	return parseGeminiUsageMetadataBytes(body)
}

func parseOpenAIChatUsageBytes(body []byte) (*TokenUsage, bool) {
	root := gjson.GetBytes(body, "usage")
	if !root.Exists() {
		return nil, false
	}
	prompt := int(root.Get("prompt_tokens").Int())
	completion := int(root.Get("completion_tokens").Int())
	usage := &TokenUsage{
		InputTokens:  intPtr(prompt),
		OutputTokens: intPtr(completion),
	}
	if cached := root.Get("prompt_tokens_details.cached_tokens"); cached.Exists() {
		value := int(cached.Int())
		usage.CacheReadInputTokens = &value
	}
	return usage, true
}

func parseOpenAIResponsesUsageBytes(body []byte) (*TokenUsage, bool) {
	root := gjson.GetBytes(body, "usage")
	if !root.Exists() {
		return nil, false
	}
	input := int(root.Get("input_tokens").Int())
	output := int(root.Get("output_tokens").Int())
	usage := &TokenUsage{
		InputTokens:  intPtr(input),
		OutputTokens: intPtr(output),
	}
	if cached := root.Get("input_tokens_details.cached_tokens"); cached.Exists() {
		value := int(cached.Int())
		usage.CacheReadInputTokens = &value
	}
	return usage, true
}

func parseGeminiUsageMetadataBytes(body []byte) (*TokenUsage, bool) {
	root := gjson.GetBytes(body, "usageMetadata")
	if !root.Exists() {
		return nil, false
	}
	prompt := int(root.Get("promptTokenCount").Int())
	candidates := int(root.Get("candidatesTokenCount").Int())
	usage := &TokenUsage{
		InputTokens:  intPtr(prompt),
		OutputTokens: intPtr(candidates),
	}
	if cached := root.Get("cachedContentTokenCount"); cached.Exists() {
		value := int(cached.Int())
		usage.CacheReadInputTokens = &value
	}
	return usage, true
}
