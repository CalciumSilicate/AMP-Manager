package amp

import (
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
