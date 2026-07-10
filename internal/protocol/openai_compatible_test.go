package protocol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompatibleListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4.1", "context_length": 1047576},
				{"id": "router/custom-model", "metadata": map[string]any{"max_context_length": 128000}},
			},
		})
	}))
	defer server.Close()

	provider := OpenAICompatibleProvider{
		BaseURL:    server.URL,
		APIKey:     "test-key",
		ProviderID: "openai-compatible",
	}
	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if models[0].ID != "gpt-4.1" || models[0].Provider != "openai-compatible" {
		t.Fatalf("unexpected first model: %#v", models[0])
	}
	if models[0].ContextWindowTokens != 1047576 || models[1].ContextWindowTokens != 128000 {
		t.Fatalf("context windows = %d / %d, want 1047576 / 128000", models[0].ContextWindowTokens, models[1].ContextWindowTokens)
	}
}

func TestOpenAICompatibleListModelsAllowsLocalNoAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("authorization = %q, want empty", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "local-model"}},
		})
	}))
	defer server.Close()

	provider := OpenAICompatibleProvider{
		BaseURL:     server.URL,
		ProviderID:  "local-openai",
		AllowNoAuth: true,
	}
	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "local-model" {
		t.Fatalf("models = %#v, want local-model", models)
	}
}

func TestOpenAICompatibleStreamUsesCompatibilityOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("X-Title"); got != "MHcode" {
			t.Fatalf("X-Title = %q, want MHcode", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["model"] != "router/model" {
			t.Fatalf("model = %v, want protected router/model", payload["model"])
		}
		if payload["enable_thinking"] != true {
			t.Fatalf("enable_thinking = %v, want true", payload["enable_thinking"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := OpenAICompatibleProvider{
		BaseURL:       server.URL,
		APIKey:        "test-key",
		ProviderID:    "router",
		ExtraHeaders:  "X-Title: MHcode",
		ExtraBodyJSON: `{"enable_thinking":true,"model":"blocked"}`,
	}
	events, err := provider.Stream(context.Background(), ChatRequest{
		Model:    "router/model",
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for event := range events {
		if event.Type == "delta" {
			got += event.Delta
		}
	}
	if got != "ok" {
		t.Fatalf("stream content = %q, want ok", got)
	}
}
