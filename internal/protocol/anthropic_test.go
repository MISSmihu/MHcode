package protocol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicMessagesFromProtocolSerializesImageAttachments(t *testing.T) {
	system, messages := anthropicMessagesFromProtocol([]Message{{
		Role:    "user",
		Content: "分析图片",
		Attachments: []Attachment{{
			Name: "screen.webp", MIMEType: "image/webp", Data: "aGVsbG8=",
		}},
	}})
	if system != "" || len(messages) != 1 || len(messages[0].Content) != 2 {
		t.Fatalf("system=%q messages=%#v", system, messages)
	}
	image := messages[0].Content[0]
	if image.Type != "image" || image.Source == nil || image.Source.Type != "base64" || image.Source.MediaType != "image/webp" || image.Source.Data != "aGVsbG8=" {
		t.Fatalf("image block = %#v", image)
	}
	if messages[0].Content[1].Type != "text" || messages[0].Content[1].Text != "分析图片" {
		t.Fatalf("text block = %#v", messages[0].Content[1])
	}
}

func TestAnthropicListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Fatalf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got == "" {
			t.Fatal("anthropic-version header should be set")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "claude-3-5-sonnet-latest", "display_name": "Claude 3.5 Sonnet"},
			},
		})
	}))
	defer server.Close()

	provider := AnthropicProvider{
		BaseURL:    server.URL,
		APIKey:     "test-key",
		ProviderID: "anthropic-compatible",
	}
	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "claude-3-5-sonnet-latest" || models[0].Provider != "anthropic-compatible" {
		t.Fatalf("models = %#v", models)
	}
}

func TestAnthropicStreamParsesDeltaReasoningAndUsage(t *testing.T) {
	var payload anthropicMessagesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %s, want /v1/messages", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":12,\"cache_read_input_tokens\":8,\"cache_creation_input_tokens\":4}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"think\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider := AnthropicProvider{BaseURL: server.URL, APIKey: "test-key"}
	events, err := provider.Stream(context.Background(), ChatRequest{
		Model: "claude-3-5-sonnet-latest",
		Messages: []Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "hi"},
		},
		Temperature: 0.2,
	})
	if err != nil {
		t.Fatal(err)
	}

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}
	if payload.System != "system" || len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(got) != 5 {
		t.Fatalf("got %d events, want 5: %#v", len(got), got)
	}
	if got[0].Type != "usage" || got[0].Usage.PromptCacheHitTokens != 8 || got[0].Usage.PromptCacheMissTokens != 4 {
		t.Fatalf("usage event = %#v", got[0])
	}
	if got[1].Type != "reasoning" || got[1].Delta != "think" {
		t.Fatalf("reasoning event = %#v", got[1])
	}
	if got[2].Type != "delta" || got[2].Delta != "hello" {
		t.Fatalf("delta event = %#v", got[2])
	}
	if got[3].Type != "usage" || got[3].Usage.CompletionTokens != 3 {
		t.Fatalf("completion usage = %#v", got[3])
	}
	if got[4].Type != "done" {
		t.Fatalf("last event = %#v", got[4])
	}
}

func TestAnthropicMessagesJoinStablePromptAndCompressedMemory(t *testing.T) {
	system, messages := anthropicMessagesFromProtocol([]Message{
		{Role: "system", Content: "stable prompt"},
		{Role: "system", Content: "compressed memory"},
		{Role: "user", Content: "continue"},
	})
	if system != "stable prompt\n\ncompressed memory" {
		t.Fatalf("system = %q", system)
	}
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestAnthropicErrorIncludesMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()

	provider := AnthropicProvider{BaseURL: server.URL, APIKey: "bad-key"}
	_, err := provider.ListModels(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("error = %v, want bad key", err)
	}
}

func TestAnthropicCompleteUsesNativeToolBlocks(t *testing.T) {
	var payload anthropicMessagesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "checking"},
				{"type": "tool_use", "id": "toolu_2", "name": "read_file", "input": map[string]any{"path": "README.md"}},
			},
			"usage": map[string]any{"input_tokens": 20, "output_tokens": 4},
		})
	}))
	defer server.Close()

	provider := AnthropicProvider{BaseURL: server.URL, APIKey: "test-key"}
	result, err := provider.Complete(context.Background(), ChatRequest{
		Model: "claude-sonnet-4",
		Messages: []Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "inspect"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "toolu_1", Type: "function", Function: ToolCallFunction{Name: "list_dir", Arguments: json.RawMessage(`{"path":"."}`)}}}},
			{Role: "tool", Name: "list_dir", ToolCallID: "toolu_1", Content: "README.md"},
		},
		Tools: []ToolDefinition{{Type: "function", Function: ToolDefinitionFunc{
			Name: "read_file", Description: "Read a file", Parameters: map[string]any{"type": "object"},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Name != "read_file" {
		t.Fatalf("tools = %#v", payload.Tools)
	}
	if len(payload.Messages) != 3 || payload.Messages[1].Content[0].Type != "tool_use" || payload.Messages[2].Content[0].Type != "tool_result" {
		t.Fatalf("messages = %#v", payload.Messages)
	}
	if result.Content != "checking" || len(result.ToolCalls) != 1 || result.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("result = %#v", result)
	}
	if string(result.ToolCalls[0].Function.Arguments) != `{"path":"README.md"}` || result.Usage == nil || result.Usage.PromptTokens != 20 {
		t.Fatalf("result details = %#v", result)
	}
}

func TestAnthropicStreamReassemblesToolUseJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"read_file\",\"input\":{}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"README.md\\\"}\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider := AnthropicProvider{BaseURL: server.URL, APIKey: "test-key"}
	events, err := provider.Stream(context.Background(), ChatRequest{Model: "claude-sonnet-4", Messages: []Message{{Role: "user", Content: "read"}}})
	if err != nil {
		t.Fatal(err)
	}
	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}
	if len(got) != 2 || got[0].Type != "tool_calls" || got[1].Type != "done" {
		t.Fatalf("events = %#v", got)
	}
	if len(got[0].ToolCalls) != 1 || string(got[0].ToolCalls[0].Function.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("tool call = %#v", got[0].ToolCalls)
	}
}
