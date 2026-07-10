package protocol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s, want /models", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Fatalf("key = %q", got)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Fatalf("x-goog-api-key = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{
					"name":                       "models/gemini-2.5-flash",
					"displayName":                "Gemini 2.5 Flash",
					"inputTokenLimit":            1048576,
					"supportedGenerationMethods": []string{"generateContent"},
				},
				{
					"name":                       "models/embedding-001",
					"displayName":                "Embedding",
					"supportedGenerationMethods": []string{"embedContent"},
				},
			},
		})
	}))
	defer server.Close()

	provider := GeminiProvider{BaseURL: server.URL, APIKey: "test-key", ProviderID: "gemini"}
	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1: %#v", len(models), models)
	}
	if models[0].ID != "gemini-2.5-flash" || models[0].ContextWindowTokens != 1048576 {
		t.Fatalf("model = %#v", models[0])
	}
}

func TestGeminiStreamParsesDeltaAndUsage(t *testing.T) {
	var payload geminiGenerateContentRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-2.5-flash:streamGenerateContent" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("alt") != "sse" {
			t.Fatalf("alt = %q, want sse", r.URL.Query().Get("alt"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hi\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":2,\"totalTokenCount\":12,\"cachedContentTokenCount\":7}}\n\n"))
	}))
	defer server.Close()

	provider := GeminiProvider{BaseURL: server.URL, APIKey: "test-key"}
	events, err := provider.Stream(context.Background(), ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "previous"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}
	if payload.SystemInstruction == nil || payload.SystemInstruction.Parts[0].Text != "system" {
		t.Fatalf("system instruction = %#v", payload.SystemInstruction)
	}
	if len(payload.Contents) != 2 || payload.Contents[1].Role != "model" {
		t.Fatalf("contents = %#v", payload.Contents)
	}
	if len(got) != 3 {
		t.Fatalf("events = %#v, want delta/usage/done", got)
	}
	if got[0].Type != "delta" || got[0].Delta != "hi" {
		t.Fatalf("delta = %#v", got[0])
	}
	if got[1].Type != "usage" || got[1].Usage.PromptCacheHitTokens != 7 || got[1].Usage.PromptCacheMissTokens != 3 {
		t.Fatalf("usage = %#v", got[1])
	}
	if got[2].Type != "done" {
		t.Fatalf("last = %#v", got[2])
	}
}

func TestGeminiErrorIncludesMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer server.Close()

	provider := GeminiProvider{BaseURL: server.URL, APIKey: "test-key"}
	_, err := provider.ListModels(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("error = %v, want bad request", err)
	}
}
