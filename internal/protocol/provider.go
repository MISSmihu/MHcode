package protocol

import "context"

type Model struct {
	ID                  string `json:"id"`
	DisplayName         string `json:"displayName"`
	Provider            string `json:"provider"`
	ContextWindowTokens int    `json:"contextWindowTokens"`
}

type ChatRequest struct {
	Model           string            `json:"model"`
	Messages        []Message         `json:"messages"`
	Temperature     float64           `json:"temperature"`
	ThinkingMode    string            `json:"thinkingMode,omitempty"`
	ReasoningEffort string            `json:"reasoningEffort,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TokenUsage struct {
	PromptTokens          int64 `json:"promptTokens"`
	CompletionTokens      int64 `json:"completionTokens"`
	TotalTokens           int64 `json:"totalTokens"`
	PromptCacheHitTokens  int64 `json:"promptCacheHitTokens"`
	PromptCacheMissTokens int64 `json:"promptCacheMissTokens"`
}

type StreamEvent struct {
	Type  string      `json:"type"`
	Delta string      `json:"delta,omitempty"`
	Error string      `json:"error,omitempty"`
	Usage *TokenUsage `json:"usage,omitempty"`
}

type Provider interface {
	Name() string
	ListModels(ctx context.Context) ([]Model, error)
	Stream(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error)
}
