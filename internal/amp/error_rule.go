package amp

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ampmanager/internal/database"
	"ampmanager/internal/errorruleutil"
	"ampmanager/internal/invalidation"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"
	"ampmanager/internal/translator"

	log "github.com/sirupsen/logrus"
)

type ErrorRuleMatch struct {
	Rule           model.ErrorRule
	RequestType    model.ErrorRuleRequestType
	OriginalStatus int
	MatchedText    string
}

type compiledErrorRule struct {
	rule      model.ErrorRule
	pattern   string
	exactText string
	regex     *regexp.Regexp
}

type errorRuleDetector struct {
	mu                    sync.RWMutex
	containsRules         []compiledErrorRule
	exactRules            map[string]compiledErrorRule
	regexRules            []compiledErrorRule
	loadedAt              *time.Time
	lastReloadError       string
	loading               bool
	dbLoadedSuccessfully  bool
	initialized           bool
	reloadRequested       bool
	reloadDone            chan struct{}
	subscribeOnce         sync.Once
}

var globalErrorRuleDetector = &errorRuleDetector{
	exactRules: make(map[string]compiledErrorRule),
}

func StartErrorRuleRuntime() {
	globalErrorRuleDetector.subscribe()
}

func PublishErrorRulesUpdated() {
	invalidation.Publish(invalidation.ChannelErrorRulesUpdated)
}

func SetErrorRules(rules []model.ErrorRule) {
	globalErrorRuleDetector.subscribe()
	globalErrorRuleDetector.mu.Lock()
	defer globalErrorRuleDetector.mu.Unlock()
	globalErrorRuleDetector.containsRules, globalErrorRuleDetector.exactRules, globalErrorRuleDetector.regexRules = compileErrorRules(rules)
	now := time.Now().UTC()
	globalErrorRuleDetector.loadedAt = &now
	globalErrorRuleDetector.initialized = true
	globalErrorRuleDetector.dbLoadedSuccessfully = true
	globalErrorRuleDetector.lastReloadError = ""
}

func ReloadErrorRules() error {
	globalErrorRuleDetector.subscribe()
	return globalErrorRuleDetector.reload()
}

func ErrorRuleCacheStats() model.ErrorRuleCacheStats {
	return globalErrorRuleDetector.stats()
}

func MatchErrorRule(format translator.Format, upstreamStatus int, body []byte) *ErrorRuleMatch {
	return MatchErrorRuleByRequestType(requestTypeFromFormat(format), upstreamStatus, body)
}

func MatchErrorRuleByRequestType(requestType model.ErrorRuleRequestType, upstreamStatus int, body []byte) *ErrorRuleMatch {
	if err := globalErrorRuleDetector.ensureInitialized(); err != nil {
		log.Warnf("error rules: ensure initialized failed: %v", err)
	}

	text := strings.TrimSpace(string(body))

	globalErrorRuleDetector.mu.RLock()
	containsRules := append([]compiledErrorRule(nil), globalErrorRuleDetector.containsRules...)
	regexRules := append([]compiledErrorRule(nil), globalErrorRuleDetector.regexRules...)
	exactRules := make(map[string]compiledErrorRule, len(globalErrorRuleDetector.exactRules))
	for key, value := range globalErrorRuleDetector.exactRules {
		exactRules[key] = value
	}
	globalErrorRuleDetector.mu.RUnlock()

	for _, entry := range containsRules {
		if entry.rule.RequestType != requestType || !matchesStatus(entry.rule.UpstreamStatus, upstreamStatus) {
			continue
		}
		if strings.Contains(strings.ToLower(text), entry.pattern) {
			return &ErrorRuleMatch{Rule: entry.rule, RequestType: requestType, OriginalStatus: upstreamStatus, MatchedText: text}
		}
	}

	if entry, ok := exactRules[buildExactRuleKey(requestType, strings.ToLower(text))]; ok {
		if matchesStatus(entry.rule.UpstreamStatus, upstreamStatus) {
			return &ErrorRuleMatch{Rule: entry.rule, RequestType: requestType, OriginalStatus: upstreamStatus, MatchedText: text}
		}
	}

	for _, entry := range regexRules {
		if entry.rule.RequestType != requestType || !matchesStatus(entry.rule.UpstreamStatus, upstreamStatus) {
			continue
		}
		if entry.regex != nil && entry.regex.MatchString(text) {
			return &ErrorRuleMatch{Rule: entry.rule, RequestType: requestType, OriginalStatus: upstreamStatus, MatchedText: text}
		}
	}

	return nil
}

func BuildProtocolErrorResponseBody(requestType model.ErrorRuleRequestType, status int, message string) []byte {
	return BuildProtocolErrorResponseBodyWithOverride(requestType, status, message, nil)
}

func BuildProtocolErrorResponseBodyWithOverride(requestType model.ErrorRuleRequestType, status int, message string, override json.RawMessage) []byte {
	if sanitized := errorruleutil.SanitizedOverrideResponse(override); len(sanitized) > 0 {
		return sanitized
	}

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

func (d *errorRuleDetector) subscribe() {
	d.subscribeOnce.Do(func() {
		invalidation.Subscribe(invalidation.ChannelErrorRulesUpdated, func() {
			if err := d.reload(); err != nil {
				log.Warnf("error rules: reload after invalidation failed: %v", err)
			}
		})
	})
}

func (d *errorRuleDetector) ensureInitialized() error {
	d.subscribe()

	d.mu.RLock()
	initialized := d.initialized && d.dbLoadedSuccessfully
	d.mu.RUnlock()
	if initialized {
		return nil
	}
	return d.reload()
}

func (d *errorRuleDetector) reload() error {
	for {
		d.mu.Lock()
		if d.loading {
			d.reloadRequested = true
			done := d.reloadDone
			d.mu.Unlock()
			if done != nil {
				<-done
			}
			continue
		}
		d.loading = true
		d.reloadDone = make(chan struct{})
		d.mu.Unlock()

		err := d.reloadOnce()

		d.mu.Lock()
		done := d.reloadDone
		d.loading = false
		d.reloadDone = nil
		shouldReloadAgain := d.reloadRequested
		d.reloadRequested = false
		if done != nil {
			close(done)
		}
		d.mu.Unlock()

		if err != nil {
			return err
		}
		if !shouldReloadAgain {
			return nil
		}
	}
}

func (d *errorRuleDetector) reloadOnce() error {
	if database.GetDB() == nil {
		d.mu.Lock()
		d.containsRules = nil
		d.exactRules = make(map[string]compiledErrorRule)
		d.regexRules = nil
		d.initialized = true
		d.dbLoadedSuccessfully = true
		d.lastReloadError = ""
		d.loadedAt = nil
		d.mu.Unlock()
		return nil
	}
	rules, err := repository.NewErrorRuleRepository().ListActive()
	if err != nil {
		d.mu.Lock()
		d.lastReloadError = err.Error()
		d.mu.Unlock()
		return err
	}

	containsRules, exactRules, regexRules := compileErrorRules(rules)

	now := time.Now().UTC()
	d.mu.Lock()
	d.containsRules = containsRules
	d.exactRules = exactRules
	d.regexRules = regexRules
	d.loadedAt = &now
	d.lastReloadError = ""
	d.dbLoadedSuccessfully = true
	d.initialized = true
	d.mu.Unlock()
	return nil
}

func compileErrorRules(rules []model.ErrorRule) ([]compiledErrorRule, map[string]compiledErrorRule, []compiledErrorRule) {
	containsRules := make([]compiledErrorRule, 0)
	exactRules := make(map[string]compiledErrorRule)
	regexRules := make([]compiledErrorRule, 0)
	for _, rule := range rules {
		entry := compiledErrorRule{rule: rule, pattern: strings.ToLower(rule.Pattern)}
		switch rule.MatchType {
		case model.ErrorRuleMatchTypeContains:
			containsRules = append(containsRules, entry)
		case model.ErrorRuleMatchTypeExact:
			entry.exactText = strings.ToLower(rule.Pattern)
			exactRules[buildExactRuleKey(rule.RequestType, entry.exactText)] = entry
		case model.ErrorRuleMatchTypeRegex:
			re, compileErr := regexp.Compile(rule.Pattern)
			if compileErr != nil {
				log.Warnf("error rules: skip invalid regex %s: %v", rule.Pattern, compileErr)
				continue
			}
			entry.regex = re
			regexRules = append(regexRules, entry)
		}
	}
	return containsRules, exactRules, regexRules
}

func (d *errorRuleDetector) stats() model.ErrorRuleCacheStats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	stats := model.ErrorRuleCacheStats{
		ContainsCount:   len(d.containsRules),
		ExactCount:      len(d.exactRules),
		RegexCount:      len(d.regexRules),
		TotalCount:      len(d.containsRules) + len(d.exactRules) + len(d.regexRules),
		Reloading:       d.loading,
		LastReloadError: d.lastReloadError,
	}
	if d.loadedAt != nil {
		copyValue := *d.loadedAt
		stats.LoadedAt = &copyValue
	}
	return stats
}

func buildExactRuleKey(requestType model.ErrorRuleRequestType, value string) string {
	return string(requestType) + "\x00" + value
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
		low, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return false
		}
		high, err := strconv.Atoi(strings.TrimSpace(parts[1]))
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
