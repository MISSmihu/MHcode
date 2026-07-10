package protocol

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatibleProvider struct {
	BaseURL       string
	APIKey        string
	ProviderID    string
	DisplayName   string
	HTTPClient    *http.Client
	AllowNoAuth   bool
	ExtraHeaders  string
	ExtraBodyJSON string
}

func (p OpenAICompatibleProvider) Name() string {
	if strings.TrimSpace(p.ProviderID) != "" {
		return strings.TrimSpace(p.ProviderID)
	}
	return "openai-compatible"
}

func (p OpenAICompatibleProvider) ListModels(ctx context.Context) ([]Model, error) {
	if strings.TrimSpace(p.BaseURL) == "" {
		return nil, errors.New("OpenAI-compatible base url is required")
	}
	if strings.TrimSpace(p.APIKey) == "" && !p.AllowNoAuth {
		return nil, errors.New("OpenAI-compatible API Key is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint("/models"), nil)
	if err != nil {
		return nil, err
	}
	p.applyHeaders(req, "")
	if err := applyExtraHeaders(req, p.ExtraHeaders); err != nil {
		return nil, err
	}
	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI-compatible models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, openAICompatibleAPIError(resp)
	}

	var payload openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode OpenAI-compatible models: %w", err)
	}

	models := make([]Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		model := openAICompatibleModelFromPayload(item, p.Name())
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		models = append(models, model)
	}
	return models, nil
}

func (p OpenAICompatibleProvider) Stream(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error) {
	if strings.TrimSpace(p.BaseURL) == "" {
		return nil, errors.New("OpenAI-compatible base url is required")
	}
	if strings.TrimSpace(p.APIKey) == "" && !p.AllowNoAuth {
		return nil, errors.New("OpenAI-compatible API Key is required")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, errors.New("OpenAI-compatible model is required")
	}

	body := openAIChatRequest{
		Model:         request.Model,
		Messages:      request.Messages,
		Temperature:   request.Temperature,
		Stream:        true,
		StreamOptions: openAIStreamOptions{IncludeUsage: true},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	encoded, err = mergeExtraJSONBody(encoded, p.ExtraBodyJSON, protectedBodyKeys("model", "messages", "stream", "stream_options"))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/chat/completions"), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	p.applyHeaders(req, "text/event-stream")
	if err := applyExtraHeaders(req, p.ExtraHeaders); err != nil {
		return nil, err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI-compatible chat request failed: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		return nil, openAICompatibleAPIError(resp)
	}

	events := make(chan StreamEvent, 16)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		parseOpenAICompatibleStream(ctx, resp.Body, events)
	}()
	return events, nil
}

func (p OpenAICompatibleProvider) endpoint(path string) string {
	return strings.TrimRight(strings.TrimSpace(p.BaseURL), "/") + path
}

func (p OpenAICompatibleProvider) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 45 * time.Second}
}

func (p OpenAICompatibleProvider) applyHeaders(req *http.Request, accept string) {
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(p.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.APIKey))
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
}

func parseOpenAICompatibleStream(ctx context.Context, body io.Reader, events chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			events <- StreamEvent{Type: "error", Error: "读取 OpenAI-compatible 流式响应被中断：请求上下文已取消或超时。"}
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			events <- StreamEvent{Type: "done"}
			return
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			events <- StreamEvent{Type: "error", Error: "无法解析 OpenAI-compatible 流式响应: " + err.Error()}
			return
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				events <- StreamEvent{Type: "delta", Delta: choice.Delta.Content}
			}
		}
		if chunk.Usage != nil {
			events <- StreamEvent{Type: "usage", Usage: chunk.Usage.toTokenUsage()}
		}
	}
	if err := scanner.Err(); err != nil {
		events <- StreamEvent{Type: "error", Error: "读取 OpenAI-compatible 流式响应失败: " + err.Error()}
		return
	}
	events <- StreamEvent{Type: "done"}
}

func openAICompatibleAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var envelope openAIErrorEnvelope
	_ = json.Unmarshal(body, &envelope)

	detail := strings.TrimSpace(envelope.Error.Message)
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if detail == "" {
		return fmt.Errorf("OpenAI-compatible request failed (HTTP %d)", resp.StatusCode)
	}
	return fmt.Errorf("OpenAI-compatible request failed: %s (HTTP %d)", detail, resp.StatusCode)
}

func openAICompatibleDisplayName(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return id
	}
	return id
}

type openAIModelsResponse struct {
	Data []json.RawMessage `json:"data"`
}

func openAICompatibleModelFromPayload(payload json.RawMessage, providerID string) Model {
	var item map[string]any
	if err := json.Unmarshal(payload, &item); err != nil {
		return Model{}
	}
	id, _ := item["id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return Model{}
	}
	return Model{
		ID:                  id,
		DisplayName:         openAICompatibleDisplayName(id),
		Provider:            providerID,
		ContextWindowTokens: openAICompatibleContextWindow(item),
	}
}

func openAICompatibleContextWindow(item map[string]any) int {
	for _, key := range []string{
		"contextWindowTokens",
		"context_window_tokens",
		"context_length",
		"context_window",
		"max_context_length",
		"max_context_window",
		"max_model_len",
		"max_input_tokens",
		"n_ctx",
		"max_tokens",
	} {
		if value := positiveIntFromAny(item[key]); value > 0 {
			return value
		}
	}
	for _, key := range []string{"metadata", "details", "top_provider"} {
		nested, ok := item[key].(map[string]any)
		if !ok {
			continue
		}
		if value := openAICompatibleContextWindow(nested); value > 0 {
			return value
		}
	}
	return 0
}

func positiveIntFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case int:
		if typed > 0 {
			return typed
		}
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			return int(parsed)
		}
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

type openAIChatRequest struct {
	Model         string              `json:"model"`
	Messages      []Message           `json:"messages"`
	Temperature   float64             `json:"temperature,omitempty"`
	Stream        bool                `json:"stream"`
	StreamOptions openAIStreamOptions `json:"stream_options,omitempty"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage"`
}

type openAIUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

func (u openAIUsage) toTokenUsage() *TokenUsage {
	return &TokenUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}

type openAIErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}
