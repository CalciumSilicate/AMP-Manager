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
			ID:             "custom-fake-200",
			Name:           "fake 200",
			IsDefault:      false,
			IsEnabled:      true,
			RequestType:    model.ErrorRuleRequestTypeResponses,
			UpstreamStatus: "200",
			Pattern:        "An Error Occurred",
			MatchType:      model.ErrorRuleMatchTypeContains,
			OverrideStatusCode: func() *int {
				value := http.StatusBadGateway
				return &value
			}(),
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
	if match.Rule.OverrideStatusCode == nil || *match.Rule.OverrideStatusCode != http.StatusBadGateway {
		t.Fatalf("unexpected override status: %+v", match.Rule.OverrideStatusCode)
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
