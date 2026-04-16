package amp

import (
	"net/http"
	"testing"

	"ampmanager/internal/model"
	"ampmanager/internal/translator"
)

func TestMatchErrorRuleMatchesBuiltInFake200(t *testing.T) {
	SetErrorRules([]model.ErrorRule{
		{
			ID:              "custom-fake-200",
			Name:            "fake 200",
			BuiltIn:         false,
			Enabled:         true,
			RequestType:     model.ErrorRuleRequestTypeResponses,
			UpstreamStatus:  "200",
			Pattern:         "An Error Occurred",
			MatchMode:       model.ErrorRuleMatchModeSubstring,
			OverrideStatus:  http.StatusBadGateway,
			OverrideMessage: "上游返回了错误内容",
		},
	})
	t.Cleanup(func() {
		SetErrorRules(nil)
	})

	match := MatchErrorRule(translator.FormatOpenAIResponses, 200, []byte("An Error Occurred while processing your request"))
	if match == nil {
		t.Fatal("expected match")
	}
	if match.Rule.OverrideStatus != http.StatusBadGateway {
		t.Fatalf("unexpected override status: %d", match.Rule.OverrideStatus)
	}
}

func TestBuildProtocolErrorResponseBodySupportsAnthropicAndGemini(t *testing.T) {
	anthropic := string(BuildProtocolErrorResponseBody(model.ErrorRuleRequestTypeAnthropic, 502, "上游异常"))
	if anthropic != `{"error":{"message":"上游异常","type":"server_error"},"type":"error"}` {
		t.Fatalf("unexpected anthropic payload: %s", anthropic)
	}

	gemini := string(BuildProtocolErrorResponseBody(model.ErrorRuleRequestTypeGemini, 503, "系统繁忙"))
	if gemini != `{"error":{"code":503,"message":"系统繁忙","status":"Service Unavailable"}}` {
		t.Fatalf("unexpected gemini payload: %s", gemini)
	}
}
