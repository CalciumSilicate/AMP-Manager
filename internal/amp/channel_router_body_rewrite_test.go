package amp

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/tidwall/gjson"
)

func TestStripOpenAIUnsupportedFieldsBytes(t *testing.T) {
	input := []byte(`{"model":"gpt-4.1","max_output_tokens":128,"stream_options":{"include_usage":true},"stream":true}`)
	output, modified := stripOpenAIUnsupportedFieldsBytes(input)
	if !modified {
		t.Fatal("expected body to be modified")
	}
	if gjson.GetBytes(output, "max_output_tokens").Exists() {
		t.Fatal("expected max_output_tokens to be removed")
	}
	if gjson.GetBytes(output, "stream_options").Exists() {
		t.Fatal("expected stream_options to be removed")
	}
	if !gjson.GetBytes(output, "stream").Bool() {
		t.Fatal("expected unrelated fields to stay intact")
	}
}

func TestInjectOpenAIStreamOptionsBytes(t *testing.T) {
	input := []byte(`{"model":"gpt-4.1","stream":true}`)
	output, modified := injectOpenAIStreamOptionsBytes(input)
	if !modified {
		t.Fatal("expected stream_options to be injected")
	}
	if !gjson.GetBytes(output, "stream_options.include_usage").Bool() {
		t.Fatal("expected include_usage=true")
	}
}

func TestInjectOpenAIStreamOptionsBytesSkipsNonStreaming(t *testing.T) {
	input := []byte(`{"model":"gpt-4.1","stream":false}`)
	output, modified := injectOpenAIStreamOptionsBytes(input)
	if modified {
		t.Fatal("expected non-streaming body to remain unchanged")
	}
	if string(output) != string(input) {
		t.Fatal("expected body bytes to stay identical")
	}
}

func TestRewriteOpenAIRequestBodyCombinesStripAndInject(t *testing.T) {
	req := newJSONRequestForRewriteTest(`{"model":"gpt-4.1","stream":true,"max_output_tokens":128}`)
	rewriteOpenAIRequestBody(req, true)
	body := readRequestBodyForRewriteTest(t, req)

	if gjson.GetBytes(body, "max_output_tokens").Exists() {
		t.Fatal("expected max_output_tokens to be removed")
	}
	if !gjson.GetBytes(body, "stream_options.include_usage").Bool() {
		t.Fatal("expected include_usage=true to be injected")
	}
}

func newJSONRequestForRewriteTest(raw string) *http.Request {
	req, _ := http.NewRequest("POST", "http://example.test", bytes.NewBufferString(raw))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(raw))
	return req
}

func readRequestBodyForRewriteTest(t *testing.T, req *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("io.ReadAll returned error: %v", err)
	}
	return body
}
