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
