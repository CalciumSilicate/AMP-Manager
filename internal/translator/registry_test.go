package translator

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestTranslateRequestUsesCLIProxyAPISDK(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-4.1",
		"input":[
			{
				"role":"user",
				"content":[{"type":"input_text","text":"hello"}]
			}
		],
		"stream":false
	}`)

	translated, err := TranslateRequest(FormatOpenAIResponses, FormatOpenAIChat, "gpt-4.1", raw, false)
	if err != nil {
		t.Fatalf("TranslateRequest returned error: %v", err)
	}
	if !gjson.GetBytes(translated, "messages.0").Exists() {
		t.Fatalf("expected chat-completions messages in translated payload, got %s", string(translated))
	}
	if got := gjson.GetBytes(translated, "messages.0.role").String(); got != "user" {
		t.Fatalf("messages.0.role = %q, want user", got)
	}
}
