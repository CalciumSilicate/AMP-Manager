package amp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const maxResponsesSSEAggregateBytes = 50 * 1024 * 1024 // 50MB

// aggregateOpenAIResponsesSSEToJSON consumes an OpenAI Responses SSE stream and returns a single non-stream
// Responses JSON body (object: "response") and extracts the assistant text for logging.
//
// It prefers the embedded "response" object from response.completed/response.done events.
func aggregateOpenAIResponsesSSEToJSON(ctx context.Context, r io.Reader) ([]byte, string, error) {
	var buf bytes.Buffer
	var sseBuffer bytes.Buffer
	var totalRead int64
	var finalResponseRaw string
	outputItems := make(map[int]string)

	for {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		default:
		}

		tmp := make([]byte, 4096)
		n, err := r.Read(tmp)
		if n > 0 {
			totalRead += int64(n)
			if totalRead > maxResponsesSSEAggregateBytes {
				return nil, "", fmt.Errorf("responses sse aggregate: exceeded max bytes (%d)", maxResponsesSSEAggregateBytes)
			}
			sseBuffer.Write(tmp[:n])
		}

		for {
			data := sseBuffer.Bytes()
			idx, delimLen := findSSEDelimiter(data)
			if idx == -1 {
				break
			}

			event := make([]byte, idx+delimLen)
			copy(event, data[:idx+delimLen])
			sseBuffer.Reset()
			sseBuffer.Write(data[idx+delimLen:])

			eventName, payload, done := parseSSEEvent(event)
			if done {
				// Stop consuming once [DONE] is received.
				goto FINISH
			}
			if len(payload) == 0 {
				continue
			}

			switch firstNonEmptyString(eventName, gjson.GetBytes(payload, "type").String()) {
			case "response.output_item.done":
				if item := gjson.GetBytes(payload, "item"); item.Exists() && item.IsObject() {
					outputIndex := int(gjson.GetBytes(payload, "output_index").Int())
					outputItems[outputIndex] = item.Raw
				}
			}

			// Keep the latest full response snapshot if present.
			// Common shape: {"type":"...","response":{...}} (including response.completed).
			if resp := gjson.GetBytes(payload, "response"); resp.Exists() && resp.IsObject() {
				finalResponseRaw = resp.Raw
				continue
			}

			// Some providers may send the response object directly.
			if gjson.GetBytes(payload, "object").String() == "response" {
				finalResponseRaw = string(bytes.TrimSpace(payload))
				continue
			}

			// Keep a copy of raw events for debugging if we fail to find a final response.
			buf.Write(payload)
			buf.WriteByte('\n')
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}
	}

FINISH:
	if strings.TrimSpace(finalResponseRaw) == "" {
		return nil, "", fmt.Errorf("responses sse aggregate: missing final response.completed event")
	}
	if rebuilt, changed := rebuildResponsesOutput(finalResponseRaw, outputItems); changed {
		finalResponseRaw = rebuilt
	}

	// Extract assistant text from the response for logging
	assistantText := extractAssistantText(finalResponseRaw)

	return []byte(finalResponseRaw), assistantText, nil
}

func rebuildResponsesOutput(responseJSON string, outputItems map[int]string) (string, bool) {
	if len(outputItems) == 0 {
		return responseJSON, false
	}
	if output := gjson.Get(responseJSON, "output"); output.Exists() && output.IsArray() && len(output.Array()) > 0 {
		return responseJSON, false
	}

	indices := make([]int, 0, len(outputItems))
	for idx := range outputItems {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	rebuilt, err := sjson.SetRaw(responseJSON, "output", "[]")
	if err != nil {
		return responseJSON, false
	}
	for _, idx := range indices {
		itemJSON := strings.TrimSpace(outputItems[idx])
		if itemJSON == "" {
			continue
		}
		next, appendErr := sjson.SetRaw(rebuilt, "output.-1", itemJSON)
		if appendErr != nil {
			return responseJSON, false
		}
		rebuilt = next
	}

	return rebuilt, true
}

// extractAssistantText extracts the assistant's text from a response object
func extractAssistantText(responseJSON string) string {
	// Try to extract from output array: response.output[?].content.text
	outputs := gjson.Get(responseJSON, "output")
	if outputs.Exists() && outputs.IsArray() {
		var texts []string
		outputs.ForEach(func(_, value gjson.Result) bool {
			if value.Get("type").String() == "message" {
				content := value.Get("content")
				if content.IsArray() {
					content.ForEach(func(_, item gjson.Result) bool {
						partType := item.Get("type").String()
						if partType == "text" || partType == "output_text" {
							if text := item.Get("text").String(); text != "" {
								texts = append(texts, text)
							}
						}
						return true
					})
				}
			}
			return true
		})
		if len(texts) > 0 {
			return strings.Join(texts, "\n")
		}
	}

	// Fallback: try other common paths
	if text := gjson.Get(responseJSON, "text").String(); text != "" {
		return text
	}
	if text := gjson.Get(responseJSON, "content.text").String(); text != "" {
		return text
	}

	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseSSEEvent(event []byte) (eventName string, payload []byte, done bool) {
	// SSE event is a sequence of lines terminated by a blank line.
	lines := bytes.Split(event, []byte("\n"))
	var dataLines [][]byte
	for _, line := range lines {
		line = bytes.TrimRight(line, "\r")
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if bytes.HasPrefix(trimmed, []byte("event:")) {
			eventName = strings.TrimSpace(string(bytes.TrimPrefix(trimmed, []byte("event:"))))
			continue
		}
		if bytes.HasPrefix(trimmed, []byte("data:")) {
			data := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
			if bytes.Equal(data, []byte("[DONE]")) {
				return eventName, nil, true
			}
			if len(data) > 0 {
				dataLines = append(dataLines, data)
			}
		}
	}
	if len(dataLines) == 0 {
		return eventName, nil, false
	}
	// SSE spec: multiple data: lines are joined with \n.
	payload = bytes.Join(dataLines, []byte("\n"))
	return eventName, payload, false
}
