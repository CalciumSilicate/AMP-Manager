package opencc

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertClaudeRequestBodyS2T converts Simplified Chinese text fields in a Claude
// /v1/messages request body to Traditional Chinese. It targets user-visible
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
					blockType := block.Get("type").Str
					blockPrefix := prefix + "." + ci.String()
					switch blockType {
					case "text":
						t := block.Get("text").Str
						if t != "" {
							result, _ = sjson.SetBytes(result, blockPrefix+".text", SimplifiedToTraditional(t))
						}
					case "tool_result":
						// Convert text content inside tool_result blocks
						trContent := block.Get("content")
						if trContent.Type == gjson.String && trContent.Str != "" {
							result, _ = sjson.SetBytes(result, blockPrefix+".content", SimplifiedToTraditional(trContent.Str))
						} else if trContent.IsArray() {
							trContent.ForEach(func(tri, trBlock gjson.Result) bool {
								if trBlock.Get("type").Str == "text" {
									t := trBlock.Get("text").Str
									if t != "" {
										result, _ = sjson.SetBytes(result, blockPrefix+".content."+tri.String()+".text", SimplifiedToTraditional(t))
									}
								}
								return true
							})
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
// /v1/messages response body to Simplified Chinese. Handles text blocks, tool_use
// input, and any nested string values.
func ConvertClaudeResponseBodyT2S(body []byte) []byte {
	if len(body) == 0 || getT2S() == nil {
		return body
	}
	result := body

	content := gjson.GetBytes(result, "content")
	if content.Exists() && content.IsArray() {
		content.ForEach(func(ci, block gjson.Result) bool {
			blockPrefix := "content." + ci.String()
			blockType := block.Get("type").Str
			switch blockType {
			case "text":
				t := block.Get("text").Str
				if t != "" {
					result, _ = sjson.SetBytes(result, blockPrefix+".text", TraditionalToSimplified(t))
				}
			case "thinking":
				t := block.Get("thinking").Str
				if t != "" {
					result, _ = sjson.SetBytes(result, blockPrefix+".thinking", TraditionalToSimplified(t))
				}
			case "tool_use":
				// Recursively convert all string values in tool_use input
				input := block.Get("input")
				if input.Exists() {
					result = convertAllStringsT2S(result, blockPrefix+".input", input)
				}
			}
			return true
		})
	}

	return result
}

// convertAllStringsT2S recursively converts all string values in a gjson.Result from T2S.
func convertAllStringsT2S(body []byte, path string, node gjson.Result) []byte {
	switch {
	case node.Type == gjson.String:
		if node.Str != "" {
			body, _ = sjson.SetBytes(body, path, TraditionalToSimplified(node.Str))
		}
	case node.IsObject():
		node.ForEach(func(key, value gjson.Result) bool {
			body = convertAllStringsT2S(body, path+"."+key.Str, value)
			return true
		})
	case node.IsArray():
		node.ForEach(func(idx, value gjson.Result) bool {
			body = convertAllStringsT2S(body, path+"."+idx.String(), value)
			return true
		})
	}
	return body
}

// ConvertClaudeSSEDataT2S converts Traditional Chinese text in a single SSE JSON
// data payload to Simplified Chinese. Handles all event types: text_delta,
// thinking_delta, input_json_delta, and content_block_start.
func ConvertClaudeSSEDataT2S(data []byte) []byte {
	if len(data) == 0 || getT2S() == nil {
		return data
	}

	typ := gjson.GetBytes(data, "type").Str

	switch typ {
	case "content_block_delta":
		deltaType := gjson.GetBytes(data, "delta.type").Str
		switch deltaType {
		case "text_delta":
			t := gjson.GetBytes(data, "delta.text").Str
			if t != "" {
				if result, err := sjson.SetBytes(data, "delta.text", TraditionalToSimplified(t)); err == nil {
					return result
				}
			}
		case "thinking_delta":
			t := gjson.GetBytes(data, "delta.thinking").Str
			if t != "" {
				if result, err := sjson.SetBytes(data, "delta.thinking", TraditionalToSimplified(t)); err == nil {
					return result
				}
			}
		case "input_json_delta":
			// Convert Chinese text within partial JSON fragments for tool_use input.
			// OpenCC only converts Chinese characters and leaves JSON syntax/ASCII unchanged,
			// so this is safe even for partial JSON.
			t := gjson.GetBytes(data, "delta.partial_json").Str
			if t != "" {
				converted := TraditionalToSimplified(t)
				if converted != t {
					if result, err := sjson.SetBytes(data, "delta.partial_json", converted); err == nil {
						return result
					}
				}
			}
		}

	case "content_block_start":
		blockType := gjson.GetBytes(data, "content_block.type").Str
		switch blockType {
		case "text":
			t := gjson.GetBytes(data, "content_block.text").Str
			if t != "" {
				if result, err := sjson.SetBytes(data, "content_block.text", TraditionalToSimplified(t)); err == nil {
					return result
				}
			}
		case "thinking":
			t := gjson.GetBytes(data, "content_block.thinking").Str
			if t != "" {
				if result, err := sjson.SetBytes(data, "content_block.thinking", TraditionalToSimplified(t)); err == nil {
					return result
				}
			}
		}
	}

	return data
}
