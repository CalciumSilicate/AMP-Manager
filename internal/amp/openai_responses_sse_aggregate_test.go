package amp

import (
	"context"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestAggregateOpenAIResponsesSSEToJSON(t *testing.T) {
	input := strings.Join([]string{
		"event: response.created\n",
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":123,\"model\":\"gpt-4.1\",\"status\":\"in_progress\"}}\n\n",
		"event: response.output_text.delta\n",
		"data: {\"type\":\"response.output_text.delta\",\"response_id\":\"resp_1\",\"output_index\":0,\"delta\":\"hello\"}\n\n",
		"event: response.completed\n",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":123,\"model\":\"gpt-4.1\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n",
		"data: [DONE]\n\n",
	}, "")

	out, assistantText, err := aggregateOpenAIResponsesSSEToJSON(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	root := gjson.ParseBytes(out)
	if root.Get("object").String() != "response" {
		t.Fatalf("expected object=response, got %q", root.Get("object").String())
	}
	if root.Get("id").String() != "resp_1" {
		t.Fatalf("expected id=resp_1, got %q", root.Get("id").String())
	}
	if root.Get("status").String() != "completed" {
		t.Fatalf("expected status=completed, got %q", root.Get("status").String())
	}
	if root.Get("output.0.content.0.text").String() != "hello" {
		t.Fatalf("expected output text=hello, got %q", root.Get("output.0.content.0.text").String())
	}
	if root.Get("usage.input_tokens").Int() != 1 {
		t.Fatalf("expected usage.input_tokens=1")
	}
	if assistantText != "hello" {
		t.Fatalf("expected assistant text=hello, got %q", assistantText)
	}
}

func TestAggregateOpenAIResponsesSSEToJSON_DirectResponseObject(t *testing.T) {
	input := "data: {\"object\":\"response\",\"id\":\"resp_2\"}\n\n" +
		"data: [DONE]\n\n"

	out, _, err := aggregateOpenAIResponsesSSEToJSON(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	root := gjson.ParseBytes(out)
	if root.Get("id").String() != "resp_2" {
		t.Fatalf("expected id=resp_2, got %q", root.Get("id").String())
	}
}

func TestAggregateOpenAIResponsesSSEToJSON_RebuildsOutputFromOutputItemDone(t *testing.T) {
	input := strings.Join([]string{
		"event: response.created\n",
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_3\",\"object\":\"response\",\"created_at\":123,\"model\":\"gpt-4.1\",\"status\":\"in_progress\",\"output\":[]}}\n\n",
		"event: response.output_item.done\n",
		"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"HELLO\"}]}}\n\n",
		"event: response.completed\n",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_3\",\"object\":\"response\",\"created_at\":123,\"model\":\"gpt-4.1\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n",
	}, "")

	out, assistantText, err := aggregateOpenAIResponsesSSEToJSON(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	root := gjson.ParseBytes(out)
	if got := root.Get("output.0.content.0.text").String(); got != "HELLO" {
		t.Fatalf("expected rebuilt output text=HELLO, got %q", got)
	}
	if assistantText != "HELLO" {
		t.Fatalf("expected assistant text=HELLO, got %q", assistantText)
	}
}
