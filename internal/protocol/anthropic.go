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

const DefaultAnthropicBaseURL = "https://api.anthropic.com"

const anthropicVersion = "2023-06-01"

type AnthropicProvider struct {
	BaseURL       string
	APIKey        string
	ProviderID    string
	HTTPClient    *http.Client
	ExtraHeaders  string
	ExtraBodyJSON string
}

func (p AnthropicProvider) Name() string {
	if strings.TrimSpace(p.ProviderID) != "" {
		return strings.TrimSpace(p.ProviderID)
	}
	return "anthropic"
}

func (p AnthropicProvider) ListModels(ctx context.Context) ([]Model, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, errors.New("Anthropic API Key is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint("/v1/models"), nil)
	if err != nil {
		return nil, err
	}
	p.applyHeaders(req, "")
	if err := applyExtraHeaders(req, p.ExtraHeaders); err != nil {
		return nil, err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("Anthropic models request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, anthropicAPIError(resp)
	}

	var payload anthropicModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Anthropic models: %w", err)
	}
	models := make([]Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" {
			displayName = id
		}
		models = append(models, Model{
			ID:          id,
			DisplayName: displayName,
			Provider:    p.Name(),
		})
	}
	return models, nil
}

func (p AnthropicProvider) Stream(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, errors.New("Anthropic API Key is required")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, errors.New("Anthropic model is required")
	}
	system, messages := anthropicMessagesFromProtocol(request.Messages)
	if len(messages) == 0 {
		messages = []anthropicMessage{{Role: "user", Content: []anthropicContentBlock{{Type: "text", Text: ""}}}}
	}
	body := anthropicMessagesRequest{
		Model:       request.Model,
		MaxTokens:   4096,
		System:      system,
		Messages:    messages,
		Stream:      true,
		Temperature: request.Temperature,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	encoded, err = mergeExtraJSONBody(encoded, p.ExtraBodyJSON, protectedBodyKeys("model", "messages", "system", "stream"))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/v1/messages"), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	p.applyHeaders(req, "text/event-stream")
	if err := applyExtraHeaders(req, p.ExtraHeaders); err != nil {
		return nil, err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("Anthropic chat request failed: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		return nil, anthropicAPIError(resp)
	}

	events := make(chan StreamEvent, 16)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		parseAnthropicStream(ctx, resp.Body, events)
	}()
	return events, nil
}

func (p AnthropicProvider) endpoint(path string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAnthropicBaseURL
	}
	if strings.HasSuffix(baseURL, "/v1") && strings.HasPrefix(path, "/v1/") {
		return baseURL + strings.TrimPrefix(path, "/v1")
	}
	return baseURL + path
}

func (p AnthropicProvider) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 45 * time.Second}
}

func (p AnthropicProvider) applyHeaders(req *http.Request, accept string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", strings.TrimSpace(p.APIKey))
	req.Header.Set("anthropic-version", anthropicVersion)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
}

func anthropicMessagesFromProtocol(messages []Message) (string, []anthropicMessage) {
	var systemParts []string
	converted := make([]anthropicMessage, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		switch message.Role {
		case "system":
			systemParts = append(systemParts, content)
		case "assistant":
			converted = append(converted, anthropicMessage{
				Role:    "assistant",
				Content: []anthropicContentBlock{{Type: "text", Text: content}},
			})
		default:
			converted = append(converted, anthropicMessage{
				Role:    "user",
				Content: []anthropicContentBlock{{Type: "text", Text: content}},
			})
		}
	}
	return strings.Join(systemParts, "\n\n"), converted
}

func parseAnthropicStream(ctx context.Context, body io.Reader, events chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			events <- StreamEvent{Type: "error", Error: "读取 Anthropic 流式响应被中断：请求上下文已取消或超时。"}
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var chunk anthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			events <- StreamEvent{Type: "error", Error: "无法解析 Anthropic 流式响应: " + err.Error()}
			return
		}
		switch chunk.Type {
		case "content_block_delta":
			if chunk.Delta.Text != "" {
				events <- StreamEvent{Type: "delta", Delta: chunk.Delta.Text}
			}
			if chunk.Delta.Thinking != "" {
				events <- StreamEvent{Type: "reasoning", Delta: chunk.Delta.Thinking}
			}
		case "message_start":
			if chunk.Message != nil && chunk.Message.Usage != nil {
				events <- StreamEvent{Type: "usage", Usage: chunk.Message.Usage.toTokenUsage()}
			}
		case "message_delta":
			if chunk.Usage != nil {
				events <- StreamEvent{Type: "usage", Usage: chunk.Usage.toTokenUsage()}
			}
		case "error":
			if chunk.Error.Message != "" {
				events <- StreamEvent{Type: "error", Error: chunk.Error.Message}
				return
			}
		case "message_stop":
			events <- StreamEvent{Type: "done"}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		events <- StreamEvent{Type: "error", Error: "读取 Anthropic 流式响应失败: " + err.Error()}
		return
	}
	events <- StreamEvent{Type: "done"}
}

func anthropicAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var envelope anthropicErrorEnvelope
	_ = json.Unmarshal(body, &envelope)
	detail := strings.TrimSpace(envelope.Error.Message)
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if detail == "" {
		return fmt.Errorf("Anthropic request failed (HTTP %d)", resp.StatusCode)
	}
	return fmt.Errorf("Anthropic request failed: %s (HTTP %d)", detail, resp.StatusCode)
}

type anthropicModelsResponse struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

type anthropicMessagesRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Stream      bool               `json:"stream"`
	Temperature float64            `json:"temperature,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"delta"`
	Message *struct {
		Usage *anthropicUsage `json:"usage"`
	} `json:"message"`
	Usage *anthropicUsage `json:"usage"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type anthropicUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

func (u anthropicUsage) toTokenUsage() *TokenUsage {
	return &TokenUsage{
		PromptTokens:          u.InputTokens,
		CompletionTokens:      u.OutputTokens,
		TotalTokens:           u.InputTokens + u.OutputTokens,
		PromptCacheHitTokens:  u.CacheReadInputTokens,
		PromptCacheMissTokens: u.CacheCreationInputTokens,
	}
}

type anthropicErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}
