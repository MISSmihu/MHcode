package protocol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
