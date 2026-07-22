package protocol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestOpenAIChatSerializesReasoningEffortForCompleteAndStream(t *testing.T) {
	var mu sync.Mutex
	var payloads []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		payloads = append(payloads, payload)
		mu.Unlock()
		if payload["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "ok"}}}})
	}))
	defer server.Close()

	provider := OpenAICompatibleProvider{BaseURL: server.URL, APIKey: "test"}
	request := ChatRequest{Model: "gpt-5.5", ReasoningEffort: "xhigh", Temperature: 0.2, Messages: []Message{{Role: "user", Content: "ping"}}}
	if _, err := provider.Complete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	events, err := provider.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(payloads) != 2 {
		t.Fatalf("payload count = %d, want 2", len(payloads))
	}
	for _, payload := range payloads {
		if payload["reasoning_effort"] != "xhigh" {
			t.Fatalf("reasoning_effort = %#v", payload["reasoning_effort"])
		}
		if _, present := payload["temperature"]; present {
			t.Fatalf("reasoning request unexpectedly sent temperature: %#v", payload)
		}
	}
}

func TestResponsesSerializesNestedReasoningForCompleteAndStream(t *testing.T) {
	var mu sync.Mutex
	var payloads []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		payloads = append(payloads, payload)
		mu.Unlock()
		response := `{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`
		if payload["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"))
			return
		}
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	provider := OpenAICompatibleProvider{BaseURL: server.URL, APIKey: "test", APIType: "responses"}
	request := ChatRequest{Model: "gpt-5.5", ReasoningEffort: "xhigh", Messages: []Message{{Role: "user", Content: "ping"}}}
	if _, err := provider.Complete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	events, err := provider.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(payloads) != 2 {
		t.Fatalf("payload count = %d, want 2", len(payloads))
	}
	for _, payload := range payloads {
		reasoning, ok := payload["reasoning"].(map[string]any)
		if !ok || reasoning["effort"] != "xhigh" {
			t.Fatalf("reasoning = %#v", payload["reasoning"])
		}
	}
}

func TestResponsesAutoDetectsGPT56ReasoningThroughRelay(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer server.Close()

	provider := OpenAICompatibleProvider{
		BaseURL:          server.URL,
		APIKey:           "test",
		APIType:          "responses",
		ReasoningProfile: "auto",
	}
	_, err := provider.Complete(context.Background(), ChatRequest{
		Model:          "gpt-5.6-sol",
		ReasoningLevel: "max",
		Messages:       []Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "max" {
		t.Fatalf("reasoning = %#v, want effort=max", payload["reasoning"])
	}
}

func TestAnthropicSerializesAdaptiveThinkingForCompleteAndStream(t *testing.T) {
	var mu sync.Mutex
	var payloads []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		payloads = append(payloads, payload)
		mu.Unlock()
		if payload["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
			return
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer server.Close()

	provider := AnthropicProvider{BaseURL: server.URL, APIKey: "test"}
	request := ChatRequest{Model: "claude-opus-4-6", ReasoningLevel: "ultra", Temperature: 0.2, Messages: []Message{{Role: "user", Content: "ping"}}}
	if _, err := provider.Complete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	events, err := provider.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(payloads) != 2 {
		t.Fatalf("payload count = %d, want 2", len(payloads))
	}
	for _, payload := range payloads {
		thinking, _ := payload["thinking"].(map[string]any)
		output, _ := payload["output_config"].(map[string]any)
		if thinking["type"] != "adaptive" || output["effort"] != "max" {
			t.Fatalf("thinking/output = %#v / %#v", thinking, output)
		}
		if _, present := payload["temperature"]; present {
			t.Fatalf("adaptive request unexpectedly sent temperature: %#v", payload)
		}
	}
}

func TestGeminiSerializesThinkingLevelForCompleteAndStream(t *testing.T) {
	var mu sync.Mutex
	var payloads []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		payloads = append(payloads, payload)
		mu.Unlock()
		response := `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`
		if r.URL.Query().Get("alt") == "sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: " + response + "\n\n"))
			return
		}
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	provider := GeminiProvider{BaseURL: server.URL, APIKey: "test"}
	request := ChatRequest{Model: "gemini-3.6-flash", ReasoningLevel: "medium", Messages: []Message{{Role: "user", Content: "ping"}}}
	if _, err := provider.Complete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	events, err := provider.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(payloads) != 2 {
		t.Fatalf("payload count = %d, want 2", len(payloads))
	}
	for _, payload := range payloads {
		generation, _ := payload["generationConfig"].(map[string]any)
		thinking, _ := generation["thinkingConfig"].(map[string]any)
		if thinking["thinkingLevel"] != "MEDIUM" {
			t.Fatalf("thinking config = %#v", thinking)
		}
	}
}

func TestNativeReasoningContinuationsRoundTripThroughToolMessages(t *testing.T) {
	t.Run("anthropic", func(t *testing.T) {
		result := anthropicCompletionFromBlocks([]anthropicContentBlock{
			{Type: "thinking", Thinking: "plan", Signature: "signed"},
			{Type: "tool_use", ID: "call-1", Name: "read_file", Input: json.RawMessage(`{"path":"README.md"}`)},
		})
		_, messages := anthropicMessagesFromProtocol([]Message{{Role: "assistant", ToolCalls: result.ToolCalls, Continuation: result.Continuation}})
		if len(messages) != 1 || len(messages[0].Content) != 2 || messages[0].Content[0].Signature != "signed" {
			t.Fatalf("anthropic continuation = %#v", messages)
		}
	})

	t.Run("gemini", func(t *testing.T) {
		result := geminiCompletionFromCandidates([]geminiCandidate{{Content: geminiContent{Role: "model", Parts: []geminiPart{
			{Text: "plan", Thought: true, ThoughtSignature: "signed"},
			{FunctionCall: &geminiFunctionCall{ID: "call-1", Name: "read_file", Args: map[string]any{"path": "README.md"}}, ThoughtSignature: "tool-signed"},
		}}}})
		_, contents := geminiContentsFromProtocol([]Message{{Role: "assistant", ToolCalls: result.ToolCalls, Continuation: result.Continuation}})
		if len(contents) != 1 || len(contents[0].Parts) != 2 || contents[0].Parts[1].ThoughtSignature != "tool-signed" {
			t.Fatalf("gemini continuation = %#v", contents)
		}
	})

	t.Run("responses", func(t *testing.T) {
		result := completionFromResponses(responsesResponse{Output: []responsesOutputItem{
			{Type: "reasoning", ID: "rs-1", EncryptedContent: "encrypted"},
			{Type: "function_call", CallID: "call-1", Name: "read_file", Arguments: `{"path":"README.md"}`},
		}})
		_, input := responsesInputFromProtocol([]Message{{Role: "assistant", ToolCalls: result.ToolCalls, Continuation: result.Continuation}})
		if len(input) != 2 || input[0].Type != "reasoning" || input[0].EncryptedContent != "encrypted" {
			t.Fatalf("responses continuation = %#v", input)
		}
	})
}
