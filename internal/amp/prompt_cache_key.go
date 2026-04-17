package amp

import (
	"strings"

	"ampmanager/internal/translator"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func injectPromptCacheKeyForResponses(body []byte, incomingFormat, outgoingFormat translator.Format, session *RequestSession) ([]byte, bool) {
	if !translator.Equivalent(outgoingFormat, translator.FormatOpenAIResponses) {
		return body, false
	}
	if translator.Equivalent(incomingFormat, translator.FormatOpenAIResponses) {
		return body, false
	}

	cfg := GetSessionStickyConfig()
	if !cfg.InjectPromptCacheKeyResponses {
		return body, false
	}

	if session == nil {
		return body, false
	}
	sessionID := strings.TrimSpace(session.SessionID)
	if sessionID == "" {
		return body, false
	}

	if existing := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()); existing != "" {
		return body, false
	}

	updated, err := sjson.SetBytes(body, "prompt_cache_key", sessionID)
	if err != nil {
		return body, false
	}
	return updated, true
}
