package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

// ChatStreamEvent is the provider-independent progress contract used by the
// desktop task runner. Stable event names keep the UI independent of each API.
type ChatStreamEvent struct {
	Type        string                   `json:"type"`
	Delta       string                   `json:"delta,omitempty"`
	Message     string                   `json:"message,omitempty"`
	Model       string                   `json:"model,omitempty"`
	ToolName    string                   `json:"toolName,omitempty"`
	ToolCallID  string                   `json:"toolCallId,omitempty"`
	ToolInput   string                   `json:"toolInput,omitempty"`
	Status      string                   `json:"status,omitempty"`
	Usage       *protocol.TokenUsage     `json:"usage,omitempty"`
	Progress    *tools.ResultPart        `json:"progress,omitempty"`
	Parts       []tools.ResultPart       `json:"parts,omitempty"`
	Compression *ContextCompressionEvent `json:"compression,omitempty"`
	Team        *TeamRoleEvent           `json:"team,omitempty"`
}

// ContextCompressionEvent exposes automatic context compaction as a first-class
// task phase. The UI can show progress without parsing localized status text.
type ContextCompressionEvent struct {
	Status          string `json:"status"` // running | completed | error
	BeforeTokens    int    `json:"beforeTokens"`
	AfterTokens     int    `json:"afterTokens,omitempty"`
	RemovedMessages int    `json:"removedMessages,omitempty"`
	TargetTokens    int    `json:"targetTokens"`
}

type ChatEventSink func(ChatStreamEvent)

func emitChatEvent(sink ChatEventSink, event ChatStreamEvent) {
	if sink != nil {
		sink(event)
	}
}

func collectProviderStream(
	ctx context.Context,
	provider protocol.Provider,
	request protocol.ChatRequest,
	sink ChatEventSink,
) (protocol.CompletionResult, error) {
	events, err := provider.Stream(ctx, request)
	if err != nil {
		return protocol.CompletionResult{}, err
	}

	var content strings.Builder
	var reasoning strings.Builder
	result := protocol.CompletionResult{}
	for event := range events {
		switch event.Type {
		case "delta":
			content.WriteString(event.Delta)
			emitChatEvent(sink, ChatStreamEvent{Type: "delta", Delta: event.Delta})
		case "reasoning":
			reasoning.WriteString(event.Delta)
			emitChatEvent(sink, ChatStreamEvent{Type: "reasoning", Delta: event.Delta})
		case "tool_calls":
			result.ToolCalls = append(result.ToolCalls, event.ToolCalls...)
		case "usage":
			result.Usage = event.Usage
			emitChatEvent(sink, ChatStreamEvent{Type: "usage", Usage: event.Usage})
		case "error":
			if ctxErr := ctx.Err(); ctxErr != nil {
				return protocol.CompletionResult{}, ctxErr
			}
			return protocol.CompletionResult{}, errors.New(event.Error)
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return protocol.CompletionResult{}, ctxErr
	}
	result.Content = content.String()
	result.Reasoning = reasoning.String()
	return result, nil
}
