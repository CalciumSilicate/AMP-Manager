package amp

import (
	"testing"

	"ampmanager/internal/model"
	"ampmanager/internal/translator"

	"github.com/tidwall/gjson"
)

func TestInjectPromptCacheKeyForResponses(t *testing.T) {
	prevCfg := GetSessionStickyConfig()
	t.Cleanup(func() {
		UpdateSessionStickyConfig(prevCfg)
	})

	UpdateSessionStickyConfig(model.SessionStickyConfigResponse{
		Enabled:                       true,
		WindowMinutes:                 5,
		LogSearchMinChars:             4,
		InjectPromptCacheKeyResponses: true,
	})

	body := []byte(`{"model":"gpt-5.4","input":[]}`)
	updated, injected := injectPromptCacheKeyForResponses(
		body,
		translator.FormatOpenAIChat,
		translator.FormatOpenAIResponses,
		&RequestSession{SessionID: "sess-123"},
	)

	if !injected {
		t.Fatal("expected prompt_cache_key to be injected")
	}
	if got := gjson.GetBytes(updated, "prompt_cache_key").String(); got != "sess-123" {
		t.Fatalf("prompt_cache_key = %q, want sess-123", got)
	}
}

func TestInjectPromptCacheKeyForResponsesSkipsResponsesInput(t *testing.T) {
	prevCfg := GetSessionStickyConfig()
	t.Cleanup(func() {
		UpdateSessionStickyConfig(prevCfg)
	})

	UpdateSessionStickyConfig(model.SessionStickyConfigResponse{
		Enabled:                       true,
		WindowMinutes:                 5,
		LogSearchMinChars:             4,
		InjectPromptCacheKeyResponses: true,
	})

	body := []byte(`{"model":"gpt-5.4","input":[]}`)
	updated, injected := injectPromptCacheKeyForResponses(
		body,
		translator.FormatOpenAIResponses,
		translator.FormatOpenAIResponses,
		&RequestSession{SessionID: "sess-123"},
	)

	if injected {
		t.Fatal("expected direct responses requests to remain unchanged")
	}
	if string(updated) != string(body) {
		t.Fatalf("body changed unexpectedly: %s", updated)
	}
}

func TestInjectPromptCacheKeyForResponsesSkipsWhenDisabledOrAlreadyPresent(t *testing.T) {
	prevCfg := GetSessionStickyConfig()
	t.Cleanup(func() {
		UpdateSessionStickyConfig(prevCfg)
	})

	body := []byte(`{"model":"gpt-5.4","input":[]}`)

	UpdateSessionStickyConfig(model.SessionStickyConfigResponse{
		Enabled:                       true,
		WindowMinutes:                 5,
		LogSearchMinChars:             4,
		InjectPromptCacheKeyResponses: false,
	})
	if updated, injected := injectPromptCacheKeyForResponses(body, translator.FormatClaude, translator.FormatOpenAIResponses, &RequestSession{SessionID: "sess-123"}); injected || string(updated) != string(body) {
		t.Fatalf("expected disabled config to skip injection, injected=%v body=%s", injected, updated)
	}

	UpdateSessionStickyConfig(model.SessionStickyConfigResponse{
		Enabled:                       true,
		WindowMinutes:                 5,
		LogSearchMinChars:             4,
		InjectPromptCacheKeyResponses: true,
	})
	alreadySet := []byte(`{"model":"gpt-5.4","input":[],"prompt_cache_key":"existing"}`)
	updated, injected := injectPromptCacheKeyForResponses(alreadySet, translator.FormatClaude, translator.FormatOpenAIResponses, &RequestSession{SessionID: "sess-123"})
	if injected {
		t.Fatal("expected existing prompt_cache_key to be preserved")
	}
	if got := gjson.GetBytes(updated, "prompt_cache_key").String(); got != "existing" {
		t.Fatalf("prompt_cache_key = %q, want existing", got)
	}
}
