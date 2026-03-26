package opencc

import (
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

func TestConvertClaudeSSEDataT2S(t *testing.T) {
	data := []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"這是繁體中文"}}`)
	result := ConvertClaudeSSEDataT2S(data)
	resultStr := string(result)
	if resultStr == string(data) {
		t.Fatal("SSE T2S did not change anything")
	}
	t.Logf("SSE T2S result: %s", resultStr)
}
