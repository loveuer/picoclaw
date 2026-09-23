package openai_responses_common

import (
	"strings"
	"testing"
)

func TestParseResponseStream_CompletesWithToolCall(t *testing.T) {
	stream := strings.NewReader("event: response.created\n" +
		"data: {\"type\":\"response.created\"}\n\n" +
		"event: response.output_item.done\n" +
		"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"lookup\",\"arguments\":\"{\\\"query\\\":\\\"hi\\\"}\"}}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":4,\"total_tokens\":7}}}\n\n")
	response, err := ParseResponseStream(stream)
	if err != nil {
		t.Fatalf("ParseResponseStream() error = %v", err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Name != "lookup" || response.ToolCalls[0].ID != "call_1" {
		t.Fatalf("ToolCalls = %#v", response.ToolCalls)
	}
	if response.Usage == nil || response.Usage.TotalTokens != 7 {
		t.Fatalf("Usage = %#v", response.Usage)
	}
}

func TestParseResponseStream_RejectsMissingFinalResponse(t *testing.T) {
	_, err := ParseResponseStream(strings.NewReader("event: response.created\ndata: {\"type\":\"response.created\"}\n\n"))
	if err == nil || !strings.Contains(err.Error(), "without a final response") {
		t.Fatalf("error = %v, want missing final response", err)
	}
}

func TestParseResponseStream_UsesDeltasWhenFinalOutputIsEmpty(t *testing.T) {
	stream := strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello \"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n")
	response, err := ParseResponseStream(stream)
	if err != nil {
		t.Fatalf("ParseResponseStream() error = %v", err)
	}
	if response.Content != "hello world" {
		t.Fatalf("Content = %q, want hello world", response.Content)
	}
}
