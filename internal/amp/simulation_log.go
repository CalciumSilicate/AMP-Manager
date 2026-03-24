package amp

import (
	"net/http"
	"sort"
	"strings"

	"github.com/tidwall/gjson"

	log "github.com/sirupsen/logrus"
)

// logSimulationBody logs a summary of the converted request body at Debug level.
// It extracts key fields (model, thinking, system block count, message count, tool count,
// top-level keys) without dumping the full payload.
func logSimulationBody(prefix string, body []byte) {
	if !gjson.ValidBytes(body) {
		log.Debugf("%s: (invalid JSON, len=%d)", prefix, len(body))
		return
	}

	model := gjson.GetBytes(body, "model").String()
	thinking := gjson.GetBytes(body, "thinking.type").String()
	systemCount := gjson.GetBytes(body, "system.#").Int()
	msgCount := gjson.GetBytes(body, "messages.#").Int()
	toolCount := gjson.GetBytes(body, "tools.#").Int()
	hasCtxMgmt := gjson.GetBytes(body, "context_management").Exists()
	hasOutputCfg := gjson.GetBytes(body, "output_config").Exists()
	hasMetadata := gjson.GetBytes(body, "metadata").Exists()
	stream := gjson.GetBytes(body, "stream").Bool()

	// Count <system-reminder> blocks in first user message
	reminderCount := 0
	firstContent := gjson.GetBytes(body, "messages.0.content")
	if firstContent.IsArray() {
		firstContent.ForEach(func(_, v gjson.Result) bool {
			if strings.Contains(v.Get("text").String(), "<system-reminder>") {
				reminderCount++
			}
			return true
		})
	}

	// Collect top-level keys
	var keys []string
	gjson.ParseBytes(body).ForEach(func(key, _ gjson.Result) bool {
		keys = append(keys, key.String())
		return true
	})

	log.Debugf("%s: model=%s thinking=%s stream=%v system=%d msgs=%d tools=%d reminders=%d ctx_mgmt=%v output_cfg=%v metadata=%v keys=%v len=%d",
		prefix, model, thinking, stream,
		systemCount, msgCount, toolCount, reminderCount,
		hasCtxMgmt, hasOutputCfg, hasMetadata,
		keys, len(body))
}

// logSimulationHeaders logs the final outgoing request headers at Debug level.
// Sensitive values (Authorization, X-Api-Key) are masked.
func logSimulationHeaders(prefix string, headers http.Header) {
	var parts []string
	for key, vals := range headers {
		val := strings.Join(vals, ", ")
		// Mask sensitive headers
		keyLower := strings.ToLower(key)
		if keyLower == "authorization" || keyLower == "x-api-key" || keyLower == "x-goog-api-key" {
			if len(val) > 10 {
				val = val[:6] + "..." + val[len(val)-4:]
			} else {
				val = "***"
			}
		}
		parts = append(parts, key+": "+val)
	}
	sort.Strings(parts)
	log.Debugf("%s: [%s]", prefix, strings.Join(parts, " | "))
}
