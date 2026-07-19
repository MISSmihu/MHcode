package protocol

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesCompleteUsesNativeEndpointAndTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var request responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "gpt-test" || request.Instructions != "system rules" || len(request.Tools) != 1 {
			t.Fatalf("request = %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
  "output": [
    {"type":"message","content":[{"type":"output_text","text":"done"}]},
    {"type":"function_call","call_id":"call-1","name":"read_file","arguments":"{\"path\":\"a.txt\"}"}
  ],
  "usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,"input_tokens_details":{"cached_tokens":80}}
}`)
	}))
	defer server.Close()

	provider := OpenAICompatibleProvider{BaseURL: server.URL, APIKey: "test", APIType: "responses"}
	result, err := provider.Complete(context.Background(), ChatRequest{
		Model:    "gpt-test",
		Messages: []Message{{Role: "system", Content: "system rules"}, {Role: "user", Content: "work"}},
		Tools: []ToolDefinition{{Type: "function", Function: ToolDefinitionFunc{
			Name: "read_file", Parameters: map[string]any{"type": "object"},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "done" || len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != "call-1" {
		t.Fatalf("result = %#v", result)
	}
	if result.Usage == nil || result.Usage.PromptCacheHitTokens != 80 || result.Usage.PromptCacheMissTokens != 20 {
		t.Fatalf("usage = %#v", result.Usage)
	}
}

func TestResponsesStreamParsesTextToolsAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
		_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"output":[{"type":"function_call","call_id":"call-2","name":"search","arguments":"{\"query\":\"x\"}"}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":4}}}}`+"\n\n")
	}))
	defer server.Close()

	provider := OpenAICompatibleProvider{BaseURL: server.URL, APIKey: "test", APIType: "responses"}
	events, err := provider.Stream(context.Background(), ChatRequest{Model: "gpt-test", Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	var calls []ToolCall
	var usage *TokenUsage
	for event := range events {
		switch event.Type {
		case "delta":
			text.WriteString(event.Delta)
		case "tool_calls":
			calls = event.ToolCalls
		case "usage":
			usage = event.Usage
		}
	}
	if text.String() != "hello" || len(calls) != 1 || calls[0].ID != "call-2" {
		t.Fatalf("text=%q calls=%#v", text.String(), calls)
	}
	if usage == nil || usage.PromptCacheHitTokens != 4 {
		t.Fatalf("usage = %#v", usage)
	}
}
