package protocol

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

type responsesRequest struct {
	Model        string               `json:"model"`
	Instructions string               `json:"instructions,omitempty"`
	Input        []responsesInputItem `json:"input"`
	Stream       bool                 `json:"stream"`
	Tools        []responsesTool      `json:"tools,omitempty"`
}

type responsesInputItem struct {
	Type      string             `json:"type,omitempty"`
	Role      string             `json:"role,omitempty"`
	Content   []responsesContent `json:"content,omitempty"`
	CallID    string             `json:"call_id,omitempty"`
	Name      string             `json:"name,omitempty"`
	Arguments string             `json:"arguments,omitempty"`
	Output    string             `json:"output,omitempty"`
}

type responsesContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type responsesResponse struct {
	Output []responsesOutputItem `json:"output"`
	Usage  *responsesUsage       `json:"usage,omitempty"`
	Error  *responsesError       `json:"error,omitempty"`
}

type responsesOutputItem struct {
	Type      string             `json:"type"`
	ID        string             `json:"id,omitempty"`
	CallID    string             `json:"call_id,omitempty"`
	Name      string             `json:"name,omitempty"`
	Arguments string             `json:"arguments,omitempty"`
	Content   []responsesContent `json:"content,omitempty"`
}

type responsesUsage struct {
	InputTokens        int64 `json:"input_tokens"`
	OutputTokens       int64 `json:"output_tokens"`
	TotalTokens        int64 `json:"total_tokens"`
	InputTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

type responsesError struct {
	Message string `json:"message"`
}

type responsesStreamEnvelope struct {
	Type     string              `json:"type"`
	Delta    string              `json:"delta,omitempty"`
	Item     responsesOutputItem `json:"item,omitempty"`
	Response responsesResponse   `json:"response,omitempty"`
	Error    *responsesError     `json:"error,omitempty"`
}

func (p OpenAICompatibleProvider) streamResponses(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error) {
	if err := p.validateResponsesRequest(request); err != nil {
		return nil, err
	}
	instructions, input := responsesInputFromProtocol(request.Messages)
	body := responsesRequest{
		Model:        request.Model,
		Instructions: instructions,
		Input:        input,
		Stream:       true,
		Tools:        responsesToolsFromProtocol(request.Tools),
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	encoded, err = mergeExtraJSONBody(encoded, p.ExtraBodyJSON, protectedBodyKeys("model", "instructions", "input", "stream", "tools"))
	if err != nil {
		return nil, err
	}
	resp, err := p.doRequestWithRetry(ctx, http.MethodPost, "/responses", encoded, "text/event-stream")
	if err != nil {
		return nil, fmt.Errorf("OpenAI Responses request failed: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer resp.Body.Close()
		return nil, openAICompatibleAPIError(resp)
	}
	events := make(chan StreamEvent, 16)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		parseResponsesStream(ctx, resp.Body, events)
	}()
	return events, nil
}

func (p OpenAICompatibleProvider) completeResponses(ctx context.Context, request ChatRequest) (CompletionResult, error) {
	if err := p.validateResponsesRequest(request); err != nil {
		return CompletionResult{}, err
	}
	instructions, input := responsesInputFromProtocol(request.Messages)
	body := responsesRequest{
		Model:        request.Model,
		Instructions: instructions,
		Input:        input,
		Stream:       false,
		Tools:        responsesToolsFromProtocol(request.Tools),
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return CompletionResult{}, err
	}
	encoded, err = mergeExtraJSONBody(encoded, p.ExtraBodyJSON, protectedBodyKeys("model", "instructions", "input", "stream", "tools"))
	if err != nil {
		return CompletionResult{}, err
	}
	resp, err := p.doRequestWithRetry(ctx, http.MethodPost, "/responses", encoded, "application/json")
	if err != nil {
		return CompletionResult{}, fmt.Errorf("OpenAI Responses request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return CompletionResult{}, openAICompatibleAPIError(resp)
	}
	var payload responsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return CompletionResult{}, fmt.Errorf("decode OpenAI Responses completion: %w", err)
	}
	if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
		return CompletionResult{}, errors.New(payload.Error.Message)
	}
	return completionFromResponses(payload), nil
}

func (p OpenAICompatibleProvider) validateResponsesRequest(request ChatRequest) error {
	if strings.TrimSpace(p.BaseURL) == "" {
		return errors.New("OpenAI-compatible base url is required")
	}
	if strings.TrimSpace(p.APIKey) == "" && !p.AllowNoAuth {
		return errors.New("OpenAI-compatible API Key is required")
	}
	if strings.TrimSpace(request.Model) == "" {
		return errors.New("OpenAI Responses model is required")
	}
	return nil
}

func responsesInputFromProtocol(messages []Message) (string, []responsesInputItem) {
	system := make([]string, 0, 2)
	input := make([]responsesInputItem, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "system":
			if text := strings.TrimSpace(message.Content); text != "" {
				system = append(system, text)
			}
		case "tool":
			input = append(input, responsesInputItem{
				Type:   "function_call_output",
				CallID: message.ToolCallID,
				Output: message.Content,
			})
			if len(message.Attachments) > 0 {
				content := []responsesContent{{Type: "input_text", Text: "Visual output from tool call " + message.ToolCallID}}
				for _, attachment := range message.Attachments {
					content = append(content, responsesContent{Type: "input_image", ImageURL: "data:" + attachment.MIMEType + ";base64," + attachment.Data})
				}
				input = append(input, responsesInputItem{Type: "message", Role: "user", Content: content})
			}
		default:
			contentType := "input_text"
			if message.Role == "assistant" {
				contentType = "output_text"
			}
			content := make([]responsesContent, 0, len(message.Attachments)+1)
			if message.Content != "" {
				content = append(content, responsesContent{Type: contentType, Text: message.Content})
			}
			for _, attachment := range message.Attachments {
				content = append(content, responsesContent{
					Type:     "input_image",
					ImageURL: "data:" + attachment.MIMEType + ";base64," + attachment.Data,
				})
			}
			if len(content) > 0 {
				input = append(input, responsesInputItem{Type: "message", Role: message.Role, Content: content})
			}
			for _, call := range message.ToolCalls {
				arguments := strings.TrimSpace(string(call.Function.Arguments))
				if arguments == "" {
					arguments = "{}"
				}
				input = append(input, responsesInputItem{
					Type:      "function_call",
					CallID:    call.ID,
					Name:      call.Function.Name,
					Arguments: arguments,
				})
			}
		}
	}
	return strings.Join(system, "\n\n"), input
}

func responsesToolsFromProtocol(definitions []ToolDefinition) []responsesTool {
	tools := make([]responsesTool, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, responsesTool{
			Type:        "function",
			Name:        definition.Function.Name,
			Description: definition.Function.Description,
			Parameters:  definition.Function.Parameters,
		})
	}
	return tools
}

func parseResponsesStream(ctx context.Context, body io.Reader, events chan<- StreamEvent) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	toolCalls := make(map[string]ToolCall)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			events <- StreamEvent{Type: "error", Error: ctx.Err().Error()}
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
			emitResponsesToolCalls(events, toolCalls)
			events <- StreamEvent{Type: "done"}
			return
		}
		var envelope responsesStreamEnvelope
		if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
			events <- StreamEvent{Type: "error", Error: "decode OpenAI Responses stream: " + err.Error()}
			return
		}
		switch envelope.Type {
		case "response.output_text.delta":
			if envelope.Delta != "" {
				events <- StreamEvent{Type: "delta", Delta: envelope.Delta}
			}
		case "response.output_item.done":
			if call, ok := toolCallFromResponsesItem(envelope.Item); ok {
				toolCalls[call.ID] = call
			}
		case "response.completed":
			completion := completionFromResponses(envelope.Response)
			for _, call := range completion.ToolCalls {
				toolCalls[call.ID] = call
			}
			emitResponsesToolCalls(events, toolCalls)
			if completion.Usage != nil {
				events <- StreamEvent{Type: "usage", Usage: completion.Usage}
			}
			events <- StreamEvent{Type: "finish", FinishReason: "stop"}
			events <- StreamEvent{Type: "done"}
			return
		case "response.failed", "error":
			message := "OpenAI Responses request failed"
			if envelope.Error != nil && strings.TrimSpace(envelope.Error.Message) != "" {
				message = envelope.Error.Message
			}
			events <- StreamEvent{Type: "error", Error: message}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		events <- StreamEvent{Type: "error", Error: "read OpenAI Responses stream: " + err.Error()}
		return
	}
	emitResponsesToolCalls(events, toolCalls)
	events <- StreamEvent{Type: "done"}
}

func completionFromResponses(response responsesResponse) CompletionResult {
	result := CompletionResult{}
	var text strings.Builder
	for _, item := range response.Output {
		if call, ok := toolCallFromResponsesItem(item); ok {
			result.ToolCalls = append(result.ToolCalls, call)
			continue
		}
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" || content.Type == "text" {
				text.WriteString(content.Text)
			}
		}
	}
	result.Content = text.String()
	if response.Usage != nil {
		cached := response.Usage.InputTokensDetails.CachedTokens
		miss := response.Usage.InputTokens - cached
		if miss < 0 {
			miss = 0
		}
		result.Usage = &TokenUsage{
			PromptTokens:          response.Usage.InputTokens,
			CompletionTokens:      response.Usage.OutputTokens,
			TotalTokens:           response.Usage.TotalTokens,
			PromptCacheHitTokens:  cached,
			PromptCacheMissTokens: miss,
		}
	}
	return result
}

func toolCallFromResponsesItem(item responsesOutputItem) (ToolCall, bool) {
	if item.Type != "function_call" || strings.TrimSpace(item.Name) == "" {
		return ToolCall{}, false
	}
	callID := strings.TrimSpace(item.CallID)
	if callID == "" {
		callID = strings.TrimSpace(item.ID)
	}
	arguments := strings.TrimSpace(item.Arguments)
	if arguments == "" || !json.Valid([]byte(arguments)) {
		arguments = "{}"
	}
	return ToolCall{
		ID:   callID,
		Type: "function",
		Function: ToolCallFunction{
			Name:      item.Name,
			Arguments: json.RawMessage(arguments),
		},
	}, true
}

func emitResponsesToolCalls(events chan<- StreamEvent, calls map[string]ToolCall) {
	if len(calls) == 0 {
		return
	}
	ordered := make([]ToolCall, 0, len(calls))
	ids := make([]string, 0, len(calls))
	for id := range calls {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		ordered = append(ordered, calls[id])
	}
	events <- StreamEvent{Type: "tool_calls", ToolCalls: ordered}
}
