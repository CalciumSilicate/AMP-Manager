package amp

import "testing"

func TestOpenAIChatParserParseResponse(t *testing.T) {
	parser := &openAIChatParser{}
	usage, ok := parser.ParseResponse([]byte(`{"usage":{"prompt_tokens":12,"completion_tokens":34,"prompt_tokens_details":{"cached_tokens":5}}}`))
	if !ok {
		t.Fatal("expected parse success")
	}
	if ptrToInt(usage.InputTokens) != 12 || ptrToInt(usage.OutputTokens) != 34 || ptrToInt(usage.CacheReadInputTokens) != 5 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestOpenAIResponsesParserConsumeSSE(t *testing.T) {
	parser := &openAIResponsesParser{}
	usage, final, ok := parser.ConsumeSSE("response.completed", []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":21,"output_tokens":8,"input_tokens_details":{"cached_tokens":3}}}}`))
	if !ok || !final {
		t.Fatalf("expected final parse success, got ok=%v final=%v", ok, final)
	}
	if ptrToInt(usage.InputTokens) != 21 || ptrToInt(usage.OutputTokens) != 8 || ptrToInt(usage.CacheReadInputTokens) != 3 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestGeminiParserParseResponse(t *testing.T) {
	parser := &geminiParser{}
	usage, ok := parser.ParseResponse([]byte(`{"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"cachedContentTokenCount":2}}`))
	if !ok {
		t.Fatal("expected parse success")
	}
	if ptrToInt(usage.InputTokens) != 11 || ptrToInt(usage.OutputTokens) != 7 || ptrToInt(usage.CacheReadInputTokens) != 2 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}
