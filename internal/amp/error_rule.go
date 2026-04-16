package amp

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"ampmanager/internal/database"
	"ampmanager/internal/model"
	"ampmanager/internal/service"
	"ampmanager/internal/translator"
)

type ErrorRuleMatch struct {
	Rule           model.ErrorRule
	RequestType    model.ErrorRuleRequestType
	OriginalStatus int
	MatchedText    string
}

var errorRulesCache sync.RWMutex
var cachedErrorRules []model.ErrorRule

func loadErrorRules() []model.ErrorRule {
	errorRulesCache.RLock()
	if len(cachedErrorRules) > 0 {
		rules := append([]model.ErrorRule(nil), cachedErrorRules...)
		errorRulesCache.RUnlock()
		return rules
	}
	errorRulesCache.RUnlock()

	if database.GetDB() == nil {
		return nil
	}

	rules, err := service.NewSystemConfigService().GetErrorRules()
	if err != nil {
		rules = nil
	}
	SetErrorRules(rules)
	return append([]model.ErrorRule(nil), cachedErrorRules...)
}

func SetErrorRules(rules []model.ErrorRule) {
	errorRulesCache.Lock()
	defer errorRulesCache.Unlock()
	if len(rules) == 0 {
		cachedErrorRules = nil
		return
	}
	cachedErrorRules = append([]model.ErrorRule(nil), rules...)
}

func requestTypeFromFormat(format translator.Format) model.ErrorRuleRequestType {
	switch {
	case translator.Equivalent(format, translator.FormatOpenAIResponses):
		return model.ErrorRuleRequestTypeResponses
	case translator.Equivalent(format, translator.FormatClaude):
		return model.ErrorRuleRequestTypeAnthropic
	case translator.Equivalent(format, translator.FormatGemini):
		return model.ErrorRuleRequestTypeGemini
	default:
		return model.ErrorRuleRequestTypeChatCompletions
	}
}

func MatchErrorRule(format translator.Format, upstreamStatus int, body []byte) *ErrorRuleMatch {
	requestType := requestTypeFromFormat(format)
	text := strings.TrimSpace(string(body))
	for _, rule := range loadErrorRules() {
		if !rule.Enabled || rule.RequestType != requestType {
			continue
		}
		if !matchesStatus(rule.UpstreamStatus, upstreamStatus) {
			continue
		}
		if !matchesText(rule, text) {
			continue
		}
		return &ErrorRuleMatch{
			Rule:           rule,
			RequestType:    requestType,
			OriginalStatus: upstreamStatus,
			MatchedText:    text,
		}
	}
	return nil
}

func matchesStatus(pattern string, status int) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	if strings.Contains(pattern, "-") {
		parts := strings.SplitN(pattern, "-", 2)
		if len(parts) != 2 {
			return false
		}
		start := strings.TrimSpace(parts[0])
		end := strings.TrimSpace(parts[1])
		low, err := strconv.Atoi(start)
		if err != nil {
			return false
		}
		high, err := strconv.Atoi(end)
		if err != nil {
			return false
		}
		return status >= low && status <= high
	}
	exact, err := strconv.Atoi(pattern)
	if err != nil {
		return false
	}
	return status == exact
}

func matchesText(rule model.ErrorRule, text string) bool {
	switch rule.MatchMode {
	case model.ErrorRuleMatchModeRegex:
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return false
		}
		return re.MatchString(text)
	default:
		return strings.Contains(strings.ToLower(text), strings.ToLower(rule.Pattern))
	}
}

func BuildProtocolErrorResponseBody(requestType model.ErrorRuleRequestType, status int, message string) []byte {
	switch requestType {
	case model.ErrorRuleRequestTypeAnthropic:
		payload, err := json.Marshal(map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    MapHTTPStatusToErrorType(status),
				"message": message,
			},
		})
		if err == nil {
			return payload
		}
	case model.ErrorRuleRequestTypeGemini:
		payload, err := json.Marshal(map[string]any{
			"error": map[string]any{
				"code":    status,
				"message": message,
				"status":  http.StatusText(status),
			},
		})
		if err == nil {
			return payload
		}
	}
	return BuildErrorResponseBody(status, message)
}
