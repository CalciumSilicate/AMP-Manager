package opencc

import (
	"strings"
	"testing"
)

func TestSimplifiedToTraditional(t *testing.T) {
	result := SimplifiedToTraditional("简体中文转换测试")
	if result == "简体中文转换测试" {
		t.Fatal("S2T conversion did not change anything")
	}
	t.Logf("S2T: %s -> %s", "简体中文转换测试", result)
}

func TestTraditionalToSimplified(t *testing.T) {
	result := TraditionalToSimplified("簡體中文轉換測試")
	if result == "簡體中文轉換測試" {
		t.Fatal("T2S conversion did not change anything")
	}
	t.Logf("T2S: %s -> %s", "簡體中文轉換測試", result)
}

func TestRoundTrip(t *testing.T) {
	original := "你好世界，这是一个简体中文的测试"
	traditional := SimplifiedToTraditional(original)
	simplified := TraditionalToSimplified(traditional)
	t.Logf("Original:    %s", original)
	t.Logf("Traditional: %s", traditional)
	t.Logf("Simplified:  %s", simplified)
	if simplified != original {
		t.Logf("Note: round-trip not identical (expected for some characters)")
	}
}

func TestConvertClaudeRequestBodyS2T(t *testing.T) {
	body := []byte(`{"model":"claude-3","messages":[{"role":"user","content":"你好世界"}],"system":"这是系统提示"}`)
	result := ConvertClaudeRequestBodyS2T(body)
	resultStr := string(result)
	if resultStr == string(body) {
		t.Fatal("Request body S2T did not change anything")
	}
	t.Logf("Request S2T result: %s", resultStr)
}

func TestConvertClaudeSSEDataT2S_TextDelta(t *testing.T) {
	data := []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"這是繁體中文"}}`)
	result := ConvertClaudeSSEDataT2S(data)
	resultStr := string(result)
	if resultStr == string(data) {
		t.Fatal("SSE text_delta T2S did not change anything")
	}
	if strings.Contains(resultStr, "這是繁體中文") {
		t.Fatal("SSE text_delta still contains Traditional Chinese")
	}
	t.Logf("SSE text_delta T2S result: %s", resultStr)
}

func TestConvertClaudeSSEDataT2S_InputJsonDelta(t *testing.T) {
	data := []byte(`{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"\"關於架構設計，插件內註冊的工具\""}}`)
	result := ConvertClaudeSSEDataT2S(data)
	resultStr := string(result)
	if resultStr == string(data) {
		t.Fatal("SSE input_json_delta T2S did not change anything")
	}
	if strings.Contains(resultStr, "關於") || strings.Contains(resultStr, "註冊") {
		t.Fatal("SSE input_json_delta still contains Traditional Chinese")
	}
	t.Logf("SSE input_json_delta T2S result: %s", resultStr)
}

func TestConvertClaudeResponseBodyT2S_ToolUse(t *testing.T) {
	body := []byte(`{"content":[{"type":"tool_use","id":"toolu_123","name":"ask","input":{"prompt":"關於架構設計","title":"選擇方案","options":[{"label":"方案A","description":"純插件方案"}]}}]}`)
	result := ConvertClaudeResponseBodyT2S(body)
	resultStr := string(result)
	if strings.Contains(resultStr, "關於") {
		t.Fatal("Non-streaming tool_use still contains Traditional Chinese '關於'")
	}
	if strings.Contains(resultStr, "純插件") {
		t.Fatal("Non-streaming tool_use still contains Traditional Chinese '純插件'")
	}
	t.Logf("Non-streaming tool_use T2S result: %s", resultStr)
}

func TestConvertClaudeResponseBodyT2S_TextBlock(t *testing.T) {
	body := []byte(`{"content":[{"type":"text","text":"這是一個繁體中文的回應"}]}`)
	result := ConvertClaudeResponseBodyT2S(body)
	resultStr := string(result)
	if strings.Contains(resultStr, "這是") {
		t.Fatal("Non-streaming text still contains Traditional Chinese")
	}
	t.Logf("Non-streaming text T2S result: %s", resultStr)
}
