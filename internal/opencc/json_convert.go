package opencc

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertClaudeRequestBodyS2T converts Simplified Chinese text fields in a Claude
// /v1/messages request body to Traditional Chinese. It targets only user-visible
// text: messages[].content (string or text blocks), and system prompt text.
func ConvertClaudeRequestBodyS2T(body []byte) []byte {
	if len(body) == 0 || getS2T() == nil {
		return body
	}
	var err error
	result := body

	// Convert system prompt
	sys := gjson.GetBytes(result, "system")
	if sys.Exists() {
		if sys.Type == gjson.String {
			result, err = sjson.SetBytes(result, "system", SimplifiedToTraditional(sys.Str))
			if err != nil {
				return body
			}
		} else if sys.IsArray() {
			sys.ForEach(func(key, value gjson.Result) bool {
				if value.Get("type").Str == "text" {
					path := "system." + key.String() + ".text"
					t := value.Get("text").Str
					if t != "" {
						result, _ = sjson.SetBytes(result, path, SimplifiedToTraditional(t))
					}
				}
				return true
			})
		}
	}

	// Convert messages[].content
	messages := gjson.GetBytes(result, "messages")
	if messages.Exists() && messages.IsArray() {
		messages.ForEach(func(mi, msg gjson.Result) bool {
			content := msg.Get("content")
			if !content.Exists() {
				return true
			}
			prefix := "messages." + mi.String() + ".content"
			if content.Type == gjson.String {
				result, _ = sjson.SetBytes(result, prefix, SimplifiedToTraditional(content.Str))
			} else if content.IsArray() {
				content.ForEach(func(ci, block gjson.Result) bool {
					if block.Get("type").Str == "text" {
						t := block.Get("text").Str
						if t != "" {
							result, _ = sjson.SetBytes(result, prefix+"."+ci.String()+".text", SimplifiedToTraditional(t))
						}
					}
					return true
				})
			}
			return true
		})
	}

	return result
}

// ConvertClaudeResponseBodyT2S converts Traditional Chinese text fields in a Claude
// /v1/messages response body to Simplified Chinese.
func ConvertClaudeResponseBodyT2S(body []byte) []byte {
	if len(body) == 0 || getT2S() == nil {
		return body
	}
	result := body

	// Convert content[].text in response
	content := gjson.GetBytes(result, "content")
	if content.Exists() && content.IsArray() {
		content.ForEach(func(ci, block gjson.Result) bool {
			if block.Get("type").Str == "text" {
				t := block.Get("text").Str
				if t != "" {
					result, _ = sjson.SetBytes(result, "content."+ci.String()+".text", TraditionalToSimplified(t))
				}
			}
			return true
		})
	}

	return result
}

// ConvertClaudeSSEDataT2S converts Traditional Chinese text in a single SSE JSON
// data payload (content_block_delta or message_delta events) to Simplified Chinese.
func ConvertClaudeSSEDataT2S(data []byte) []byte {
	if len(data) == 0 || getT2S() == nil {
		return data
	}

	typ := gjson.GetBytes(data, "type").Str

	switch typ {
	case "content_block_delta":
		// delta.text for text_delta
		deltaType := gjson.GetBytes(data, "delta.type").Str
		if deltaType == "text_delta" {
			t := gjson.GetBytes(data, "delta.text").Str
			if t != "" {
				result, err := sjson.SetBytes(data, "delta.text", TraditionalToSimplified(t))
				if err == nil {
					return result
				}
			}
		} else if deltaType == "thinking_delta" {
			t := gjson.GetBytes(data, "delta.thinking").Str
			if t != "" {
				result, err := sjson.SetBytes(data, "delta.thinking", TraditionalToSimplified(t))
				if err == nil {
					return result
				}
			}
		}
	case "content_block_start":
		// content_block.text for text blocks
		t := gjson.GetBytes(data, "content_block.text").Str
		if t != "" {
			result, err := sjson.SetBytes(data, "content_block.text", TraditionalToSimplified(t))
			if err == nil {
				return result
			}
		}
	case "message_delta":
		// stop_reason is metadata, not text to convert
	}

	return data
}
