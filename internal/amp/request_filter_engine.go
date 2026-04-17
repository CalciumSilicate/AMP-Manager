package amp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"sort"
	"strings"
	"sync"

	"ampmanager/internal/database"
	"ampmanager/internal/invalidation"
	"ampmanager/internal/model"
	"ampmanager/internal/repository"

	log "github.com/sirupsen/logrus"
)

var requestFilterTransportHeaderBlacklist = map[string]struct{}{
	"content-length":    {},
	"connection":        {},
	"transfer-encoding": {},
}

type cachedRequestFilter struct {
	filter model.RequestFilter
}

type requestFilterEngine struct {
	mu                 sync.RWMutex
	globalGuard        []cachedRequestFilter
	boundGuard         []cachedRequestFilter
	globalFinal        []cachedRequestFilter
	boundFinal         []cachedRequestFilter
	initialized        bool
	loading            bool
	reloadDone         chan struct{}
	subscribeOnce      sync.Once
}

var globalRequestFilterEngine = &requestFilterEngine{}

func StartRequestFilterRuntime() {
	globalRequestFilterEngine.subscribe()
}

func PublishRequestFiltersUpdated() {
	invalidation.Publish(invalidation.ChannelRequestFiltersUpdated)
}

func ReloadRequestFilters() error {
	globalRequestFilterEngine.subscribe()
	return globalRequestFilterEngine.reload()
}

func ApplyGlobalGuardRequestFilters(headers http.Header, body []byte) (http.Header, []byte, error) {
	return globalRequestFilterEngine.apply(model.RequestFilterExecutionPhaseGuard, "", nil, true, headers, body)
}

func ApplyBoundGuardRequestFilters(channelID string, groupIDs []string, headers http.Header, body []byte) (http.Header, []byte, error) {
	return globalRequestFilterEngine.apply(model.RequestFilterExecutionPhaseGuard, channelID, groupIDs, false, headers, body)
}

func ApplyFinalRequestFilters(channelID string, groupIDs []string, headers http.Header, body []byte) (http.Header, []byte, error) {
	return globalRequestFilterEngine.apply(model.RequestFilterExecutionPhaseFinal, channelID, groupIDs, false, headers, body)
}

func (e *requestFilterEngine) subscribe() {
	e.subscribeOnce.Do(func() {
		invalidation.Subscribe(invalidation.ChannelRequestFiltersUpdated, func() {
			if err := e.reload(); err != nil {
				log.Warnf("request filters: reload failed: %v", err)
			}
		})
	})
}

func (e *requestFilterEngine) ensureInitialized() error {
	e.subscribe()
	e.mu.RLock()
	initialized := e.initialized
	e.mu.RUnlock()
	if initialized {
		return nil
	}
	return e.reload()
}

func (e *requestFilterEngine) reload() error {
	for {
		e.mu.Lock()
		if e.loading {
			done := e.reloadDone
			e.mu.Unlock()
			if done != nil {
				<-done
			}
			return nil
		}
			e.loading = true
			e.reloadDone = make(chan struct{})
			e.mu.Unlock()

			if database.GetDB() == nil {
				e.mu.Lock()
				e.globalGuard = nil
				e.boundGuard = nil
				e.globalFinal = nil
				e.boundFinal = nil
				e.initialized = true
				done := e.reloadDone
				e.loading = false
				e.reloadDone = nil
				e.mu.Unlock()
				if done != nil {
					close(done)
				}
				return nil
			}
		items, err := repository.NewRequestFilterRepository().ListActive()
		if err == nil {
			sort.SliceStable(items, func(i, j int) bool {
				if items[i].Priority == items[j].Priority {
					return items[i].ID < items[j].ID
				}
				return items[i].Priority < items[j].Priority
			})

			globalGuard := make([]cachedRequestFilter, 0)
			boundGuard := make([]cachedRequestFilter, 0)
			globalFinal := make([]cachedRequestFilter, 0)
			boundFinal := make([]cachedRequestFilter, 0)
			for _, item := range items {
				cached := cachedRequestFilter{filter: item}
				switch item.ExecutionPhase {
				case model.RequestFilterExecutionPhaseFinal:
					if item.BindingType == model.RequestFilterBindingTypeGlobal {
						globalFinal = append(globalFinal, cached)
					} else {
						boundFinal = append(boundFinal, cached)
					}
				default:
					if item.BindingType == model.RequestFilterBindingTypeGlobal {
						globalGuard = append(globalGuard, cached)
					} else {
						boundGuard = append(boundGuard, cached)
					}
				}
			}

			e.mu.Lock()
			e.globalGuard = globalGuard
			e.boundGuard = boundGuard
			e.globalFinal = globalFinal
			e.boundFinal = boundFinal
			e.initialized = true
			e.mu.Unlock()
		}

		e.mu.Lock()
		done := e.reloadDone
		e.loading = false
		e.reloadDone = nil
		e.mu.Unlock()
		if done != nil {
			close(done)
		}
		return err
	}
}

func (e *requestFilterEngine) apply(
	phase model.RequestFilterExecutionPhase,
	channelID string,
	groupIDs []string,
	globalOnly bool,
	headers http.Header,
	body []byte,
) (http.Header, []byte, error) {
	if err := e.ensureInitialized(); err != nil {
		return headers, body, err
	}

	e.mu.RLock()
	var first, second []cachedRequestFilter
	switch phase {
	case model.RequestFilterExecutionPhaseFinal:
		first = append([]cachedRequestFilter(nil), e.globalFinal...)
		if !globalOnly {
			second = append([]cachedRequestFilter(nil), e.boundFinal...)
		}
	default:
		first = append([]cachedRequestFilter(nil), e.globalGuard...)
		if !globalOnly {
			second = append([]cachedRequestFilter(nil), e.boundGuard...)
		}
	}
	e.mu.RUnlock()

	workingHeaders := cloneHeader(headers)
	workingBody := append([]byte(nil), body...)
	if len(workingBody) == 0 {
		workingBody = []byte("{}")
	}

	var err error
	for _, entry := range append(first, second...) {
		if !filterApplies(entry.filter, channelID, groupIDs, globalOnly) {
			continue
		}
		workingHeaders, workingBody, err = applyRequestFilter(entry.filter, workingHeaders, workingBody)
		if err != nil {
			return headers, body, err
		}
	}

	if phase == model.RequestFilterExecutionPhaseFinal {
		for key := range requestFilterTransportHeaderBlacklist {
			workingHeaders.Del(key)
		}
	}
	return workingHeaders, workingBody, nil
}

func filterApplies(filter model.RequestFilter, channelID string, groupIDs []string, globalOnly bool) bool {
	switch filter.BindingType {
	case model.RequestFilterBindingTypeGlobal:
		return true
	case model.RequestFilterBindingTypeChannels:
		if globalOnly {
			return false
		}
		for _, id := range filter.ChannelIDs {
			if id == channelID {
				return true
			}
		}
	case model.RequestFilterBindingTypeGroups:
		if globalOnly {
			return false
		}
		for _, allowed := range filter.GroupIDs {
			for _, groupID := range groupIDs {
				if allowed == groupID {
					return true
				}
			}
		}
	}
	return false
}

func applyRequestFilter(filter model.RequestFilter, headers http.Header, body []byte) (http.Header, []byte, error) {
	if filter.RuleMode == model.RequestFilterRuleModeAdvanced {
		return applyAdvancedRequestFilter(filter, headers, body)
	}
	return applySimpleRequestFilter(filter, headers, body)
}

func applySimpleRequestFilter(filter model.RequestFilter, headers http.Header, body []byte) (http.Header, []byte, error) {
	switch filter.Scope {
	case model.RequestFilterScopeHeader:
		switch filter.Action {
		case model.RequestFilterActionRemove:
			headers.Del(filter.Target)
		case model.RequestFilterActionSet:
			headers.Set(filter.Target, jsonRawToString(filter.Replacement))
		}
		return headers, body, nil
	case model.RequestFilterScopeBody:
		payload, err := decodeBodyObject(body)
		if err != nil {
			return headers, body, err
		}
		switch filter.Action {
		case model.RequestFilterActionJSONPath:
			value, err := rawJSONToAny(filter.Replacement)
			if err != nil {
				return headers, body, err
			}
			setValueByPath(payload, filter.Target, value)
		case model.RequestFilterActionTextReplace:
			matchType := model.RequestFilterMatchTypeContains
			if filter.MatchType != nil {
				matchType = *filter.MatchType
			}
			replaceValue := jsonRawToString(filter.Replacement)
			payload = replaceTextInAny(payload, filter.Target, replaceValue, matchType).(map[string]any)
		}
		nextBody, err := json.Marshal(payload)
		return headers, nextBody, err
	default:
		return headers, body, nil
	}
}

func applyAdvancedRequestFilter(filter model.RequestFilter, headers http.Header, body []byte) (http.Header, []byte, error) {
	payload, err := decodeBodyObject(body)
	if err != nil {
		return headers, body, err
	}
	for _, op := range filter.Operations {
		switch op.Type {
		case "set":
			value, err := rawJSONToAny(op.Value)
			if err != nil {
				return headers, body, err
			}
			if op.Scope == model.RequestFilterScopeHeader {
				headerValue := anyToString(value)
				if op.WriteMode == model.RequestFilterWriteModeIfMissing {
					if headers.Get(op.Path) == "" {
						headers.Set(op.Path, headerValue)
					}
				} else {
					headers.Set(op.Path, headerValue)
				}
				continue
			}
			if op.WriteMode == model.RequestFilterWriteModeIfMissing {
				if existing, ok := getValueByPath(payload, op.Path); ok && existing != nil {
					continue
				}
			}
			setValueByPath(payload, op.Path, value)
		case "remove":
			if op.Scope == model.RequestFilterScopeHeader {
				headers.Del(op.Path)
				continue
			}
			if op.Matcher == nil {
				deleteByPath(payload, op.Path)
				continue
			}
			removeByMatcher(payload, op.Path, *op.Matcher)
		case "merge":
			value, err := rawJSONToMap(op.Value)
			if err != nil {
				return headers, body, err
			}
			if op.Path == "" {
				deepMerge(payload, value)
				continue
			}
			current, _ := getValueByPath(payload, op.Path)
			currentMap, _ := current.(map[string]any)
			if currentMap == nil {
				currentMap = map[string]any{}
			}
			deepMerge(currentMap, value)
			setValueByPath(payload, op.Path, currentMap)
		case "insert":
			value, err := rawJSONToAny(op.Value)
			if err != nil {
				return headers, body, err
			}
			if err := insertByOperation(payload, op, value); err != nil {
				return headers, body, err
			}
		}
	}
	nextBody, err := json.Marshal(payload)
	return headers, nextBody, err
}

func decodeBodyObject(body []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func rawJSONToAny(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func rawJSONToMap(raw json.RawMessage) (map[string]any, error) {
	value, err := rawJSONToAny(raw)
	if err != nil {
		return nil, err
	}
	mapped, ok := value.(map[string]any)
	if !ok {
		return nil, nil
	}
	return mapped, nil
}

func jsonRawToString(raw json.RawMessage) string {
	value, err := rawJSONToAny(raw)
	if err != nil {
		return string(raw)
	}
	return anyToString(value)
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		data, _ := json.Marshal(typed)
		return string(data)
	}
}

func cloneHeader(headers http.Header) http.Header {
	cloned := make(http.Header, len(headers))
	for key, values := range headers {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func parsePath(path string) []any {
	re := regexp.MustCompile(`([^.\[\]]+)|\[(\d+)\]`)
	matches := re.FindAllStringSubmatch(path, -1)
	parts := make([]any, 0, len(matches))
	for _, match := range matches {
		if match[1] != "" {
			parts = append(parts, match[1])
			continue
		}
		if match[2] != "" {
			parts = append(parts, atoiSafe(match[2]))
		}
	}
	return parts
}

func atoiSafe(value string) int {
	number, _ := strconv.Atoi(value)
	return number
}

func setValueByPath(root map[string]any, path string, value any) {
	keys := parsePath(path)
	if len(keys) == 0 {
		return
	}
	current := any(root)
	for index, key := range keys {
		last := index == len(keys)-1
		switch typedKey := key.(type) {
		case string:
			node, ok := current.(map[string]any)
			if !ok {
				return
			}
			if last {
				node[typedKey] = value
				return
			}
			next, exists := node[typedKey]
			if !exists || next == nil {
				switch keys[index+1].(type) {
				case int:
					next = []any{}
				default:
					next = map[string]any{}
				}
				node[typedKey] = next
			}
			current = node[typedKey]
		case int:
			node, ok := current.([]any)
			if !ok {
				return
			}
			for len(node) <= typedKey {
				node = append(node, nil)
			}
			if last {
				node[typedKey] = value
				return
			}
			if node[typedKey] == nil {
				switch keys[index+1].(type) {
				case int:
					node[typedKey] = []any{}
				default:
					node[typedKey] = map[string]any{}
				}
			}
			current = node[typedKey]
		}
	}
}

func getValueByPath(root map[string]any, path string) (any, bool) {
	keys := parsePath(path)
	if len(keys) == 0 {
		return nil, false
	}
	current := any(root)
	for _, key := range keys {
		switch typedKey := key.(type) {
		case string:
			node, ok := current.(map[string]any)
			if !ok {
				return nil, false
			}
			current, ok = node[typedKey]
			if !ok {
				return nil, false
			}
		case int:
			node, ok := current.([]any)
			if !ok || typedKey < 0 || typedKey >= len(node) {
				return nil, false
			}
			current = node[typedKey]
		}
	}
	return current, true
}

func deleteByPath(root map[string]any, path string) {
	keys := parsePath(path)
	if len(keys) == 0 {
		return
	}
	current := any(root)
	for index, key := range keys[:len(keys)-1] {
		switch typedKey := key.(type) {
		case string:
			node, ok := current.(map[string]any)
			if !ok {
				return
			}
			current = node[typedKey]
		case int:
			node, ok := current.([]any)
			if !ok || typedKey < 0 || typedKey >= len(node) {
				return
			}
			current = node[typedKey]
		}
		if current == nil && index < len(keys)-1 {
			return
		}
	}
	last := keys[len(keys)-1]
	switch typedKey := last.(type) {
	case string:
		node, ok := current.(map[string]any)
		if ok {
			delete(node, typedKey)
		}
	case int:
		node, ok := current.([]any)
		if ok && typedKey >= 0 && typedKey < len(node) {
			reduced := append(node[:typedKey], node[typedKey+1:]...)
			if len(keys) == 1 {
				return
			}
			setValueByPath(root, strings.TrimSuffix(path, "["+strconv.Itoa(typedKey)+"]"), reduced)
		}
	}
}

func deepMerge(target map[string]any, source map[string]any) {
	for key, value := range source {
		if value == nil {
			delete(target, key)
			continue
		}
		if targetMap, ok := target[key].(map[string]any); ok {
			if sourceMap, ok := value.(map[string]any); ok {
				deepMerge(targetMap, sourceMap)
				target[key] = targetMap
				continue
			}
		}
		target[key] = value
	}
}

func replaceTextInAny(value any, target, replacement string, matchType model.RequestFilterMatchType) any {
	switch typed := value.(type) {
	case string:
		return replaceText(typed, target, replacement, matchType)
	case []any:
		next := make([]any, 0, len(typed))
		for _, item := range typed {
			next = append(next, replaceTextInAny(item, target, replacement, matchType))
		}
		return next
	case map[string]any:
		next := make(map[string]any, len(typed))
		for key, item := range typed {
			next[key] = replaceTextInAny(item, target, replacement, matchType)
		}
		return next
	default:
		return value
	}
}

func replaceText(input, target, replacement string, matchType model.RequestFilterMatchType) string {
	switch matchType {
	case model.RequestFilterMatchTypeExact:
		if input == target {
			return replacement
		}
		return input
	case model.RequestFilterMatchTypeRegex:
		re, err := regexp.Compile(target)
		if err != nil {
			return input
		}
		return re.ReplaceAllString(input, replacement)
	default:
		if target == "" {
			return input
		}
		return strings.ReplaceAll(input, target, replacement)
	}
}

func matchElement(value any, matcher model.RequestFilterMatcher) bool {
	fieldValue := value
	if matcher.Field != "" {
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		var exists bool
		fieldValue, exists = getValueByPath(object, matcher.Field)
		if !exists {
			return false
		}
	}
	expected, err := rawJSONToAny(matcher.Value)
	if err != nil {
		return false
	}
	matchType := matcher.MatchType
	if matchType == "" {
		matchType = model.RequestFilterMatchTypeExact
	}
	switch matchType {
	case model.RequestFilterMatchTypeContains:
		return strings.Contains(anyToString(fieldValue), anyToString(expected))
	case model.RequestFilterMatchTypeRegex:
		re, err := regexp.Compile(anyToString(expected))
		if err != nil {
			return false
		}
		return re.MatchString(anyToString(fieldValue))
	default:
		return reflect.DeepEqual(fieldValue, expected)
	}
}

func removeByMatcher(root map[string]any, path string, matcher model.RequestFilterMatcher) {
	value, ok := getValueByPath(root, path)
	if !ok {
		return
	}
	items, ok := value.([]any)
	if !ok {
		return
	}
	filtered := make([]any, 0, len(items))
	for _, item := range items {
		if matchElement(item, matcher) {
			continue
		}
		filtered = append(filtered, item)
	}
	setValueByPath(root, path, filtered)
}

func insertByOperation(root map[string]any, operation model.RequestFilterOperation, value any) error {
	current, ok := getValueByPath(root, operation.Path)
	if !ok {
		if operation.OnAnchorMissing == model.RequestFilterOnAnchorMissingPrepend || operation.OnAnchorMissing == model.RequestFilterOnAnchorMissingAppend {
			setValueByPath(root, operation.Path, []any{value})
		}
		return nil
	}
	items, ok := current.([]any)
	if !ok {
		return nil
	}

	insertAt := len(items)
	switch operation.Position {
	case model.RequestFilterInsertPositionStart:
		insertAt = 0
	case model.RequestFilterInsertPositionEnd:
		insertAt = len(items)
	case model.RequestFilterInsertPositionBefore, model.RequestFilterInsertPositionAfter:
		if operation.Anchor == nil {
			return nil
		}
		foundIndex := -1
		for index, item := range items {
			if matchElement(item, *operation.Anchor) {
				foundIndex = index
				break
			}
		}
		if foundIndex == -1 {
			switch operation.OnAnchorMissing {
			case model.RequestFilterOnAnchorMissingPrepend:
				insertAt = 0
			case model.RequestFilterOnAnchorMissingAppend:
				insertAt = len(items)
			default:
				return nil
			}
		} else {
			insertAt = foundIndex
			if operation.Position == model.RequestFilterInsertPositionAfter {
				insertAt = foundIndex + 1
			}
		}
	}

	if operation.Dedupe != nil && len(operation.Dedupe.ByFields) > 0 {
		if dedupeCandidate, ok := value.(map[string]any); ok {
			for _, item := range items {
				mapped, ok := item.(map[string]any)
				if !ok {
					continue
				}
				matched := true
				for _, field := range operation.Dedupe.ByFields {
					if !reflect.DeepEqual(mapped[field], dedupeCandidate[field]) {
						matched = false
						break
					}
				}
				if matched {
					return nil
				}
			}
		}
	}

	items = append(items, nil)
	copy(items[insertAt+1:], items[insertAt:])
	items[insertAt] = value
	setValueByPath(root, operation.Path, items)
	return nil
}
