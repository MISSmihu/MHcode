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
	"net/url"
	"strings"
	"time"
)

const DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

type GeminiProvider struct {
	BaseURL       string
	APIKey        string
	ProviderID    string
	HTTPClient    *http.Client
	ExtraHeaders  string
	ExtraBodyJSON string
}

func (p GeminiProvider) Name() string {
	if strings.TrimSpace(p.ProviderID) != "" {
		return strings.TrimSpace(p.ProviderID)
	}
	return "gemini"
}

func (p GeminiProvider) ListModels(ctx context.Context) ([]Model, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, errors.New("Gemini API Key is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint("/models", true), nil)
	if err != nil {
		return nil, err
	}
	p.applyHeaders(req)
	if err := applyExtraHeaders(req, p.ExtraHeaders); err != nil {
		return nil, err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("Gemini models request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, geminiAPIError(resp)
	}

	var payload geminiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Gemini models: %w", err)
	}
	models := make([]Model, 0, len(payload.Models))
	for _, item := range payload.Models {
		if !geminiSupportsGenerateContent(item.SupportedGenerationMethods) {
			continue
		}
		id := strings.TrimPrefix(strings.TrimSpace(item.Name), "models/")
		if id == "" {
			continue
		}
		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" {
			displayName = id
		}
		models = append(models, Model{
			ID:                  id,
			DisplayName:         displayName,
			Provider:            p.Name(),
			ContextWindowTokens: item.InputTokenLimit,
		})
	}
	return models, nil
}

func (p GeminiProvider) Stream(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, errors.New("Gemini API Key is required")
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, errors.New("Gemini model is required")
	}
	system, contents := geminiContentsFromProtocol(request.Messages)
	if len(contents) == 0 {
		contents = []geminiContent{{Role: "user", Parts: []geminiPart{{Text: ""}}}}
	}
	body := geminiGenerateContentRequest{
		Contents: contents,
		GenerationConfig: geminiGenerationConfig{
			Temperature: request.Temperature,
		},
	}
	if strings.TrimSpace(system) != "" {
		body.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: system}},
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	encoded, err = mergeExtraJSONBody(encoded, p.ExtraBodyJSON, protectedBodyKeys("contents", "systemInstruction"))
	if err != nil {
		return nil, err
	}
	path := "/" + normalizeGeminiModelName(request.Model) + ":streamGenerateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(path, true)+"&alt=sse", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	p.applyHeaders(req)
	req.Header.Set("Accept", "text/event-stream")
	if err := applyExtraHeaders(req, p.ExtraHeaders); err != nil {
		return nil, err
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("Gemini chat request failed: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		return nil, geminiAPIError(resp)
	}

	events := make(chan StreamEvent, 16)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		parseGeminiStream(ctx, resp.Body, events)
	}()
	return events, nil
}

func (p GeminiProvider) endpoint(path string, includeKey bool) string {
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultGeminiBaseURL
	}
	endpoint := baseURL + path
	if includeKey {
		separator := "?"
		if strings.Contains(endpoint, "?") {
			separator = "&"
		}
		endpoint += separator + "key=" + url.QueryEscape(strings.TrimSpace(p.APIKey))
	}
	return endpoint
}

func (p GeminiProvider) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 45 * time.Second}
}

func (p GeminiProvider) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", strings.TrimSpace(p.APIKey))
}

func geminiContentsFromProtocol(messages []Message) (string, []geminiContent) {
	var systemParts []string
	contents := make([]geminiContent, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		switch message.Role {
		case "system":
			systemParts = append(systemParts, content)
		case "assistant":
			contents = append(contents, geminiContent{Role: "model", Parts: []geminiPart{{Text: content}}})
		default:
			contents = append(contents, geminiContent{Role: "user", Parts: []geminiPart{{Text: content}}})
		}
	}
	return strings.Join(systemParts, "\n\n"), contents
}

func parseGeminiStream(ctx context.Context, body io.Reader, events chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			events <- StreamEvent{Type: "error", Error: "读取 Gemini 流式响应被中断：请求上下文已取消或超时。"}
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
		var chunk geminiStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			events <- StreamEvent{Type: "error", Error: "无法解析 Gemini 流式响应: " + err.Error()}
			return
		}
		for _, candidate := range chunk.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					events <- StreamEvent{Type: "delta", Delta: part.Text}
				}
			}
		}
		if chunk.UsageMetadata != nil {
			events <- StreamEvent{Type: "usage", Usage: chunk.UsageMetadata.toTokenUsage()}
		}
	}
	if err := scanner.Err(); err != nil {
		events <- StreamEvent{Type: "error", Error: "读取 Gemini 流式响应失败: " + err.Error()}
		return
	}
	events <- StreamEvent{Type: "done"}
}

func geminiAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var envelope geminiErrorEnvelope
	_ = json.Unmarshal(body, &envelope)
	detail := strings.TrimSpace(envelope.Error.Message)
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if detail == "" {
		return fmt.Errorf("Gemini request failed (HTTP %d)", resp.StatusCode)
	}
	return fmt.Errorf("Gemini request failed: %s (HTTP %d)", detail, resp.StatusCode)
}

func normalizeGeminiModelName(model string) string {
	model = strings.TrimSpace(model)
	if strings.HasPrefix(model, "models/") {
		return model
	}
	return "models/" + model
}

func geminiSupportsGenerateContent(methods []string) bool {
	if len(methods) == 0 {
		return true
	}
	for _, method := range methods {
		if method == "generateContent" || method == "streamGenerateContent" {
			return true
		}
	}
	return false
}

type geminiModelsResponse struct {
	Models []struct {
		Name                       string   `json:"name"`
		DisplayName                string   `json:"displayName"`
		InputTokenLimit            int      `json:"inputTokenLimit"`
		OutputTokenLimit           int      `json:"outputTokenLimit"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
}

type geminiGenerateContentRequest struct {
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature float64 `json:"temperature,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiStreamChunk struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
}

type geminiUsageMetadata struct {
	PromptTokenCount        int64 `json:"promptTokenCount"`
	CandidatesTokenCount    int64 `json:"candidatesTokenCount"`
	TotalTokenCount         int64 `json:"totalTokenCount"`
	CachedContentTokenCount int64 `json:"cachedContentTokenCount"`
}

func (u geminiUsageMetadata) toTokenUsage() *TokenUsage {
	missTokens := u.PromptTokenCount - u.CachedContentTokenCount
	if missTokens < 0 {
		missTokens = 0
	}
	return &TokenUsage{
		PromptTokens:          u.PromptTokenCount,
		CompletionTokens:      u.CandidatesTokenCount,
		TotalTokens:           u.TotalTokenCount,
		PromptCacheHitTokens:  u.CachedContentTokenCount,
		PromptCacheMissTokens: missTokens,
	}
}

type geminiErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Status  string `json:"status"`
		Code    int    `json:"code"`
	} `json:"error"`
}
