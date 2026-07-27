package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/MISSmihu/MHcode/internal/protocol"
)

const contextSummaryKind = "context-summary"

type contextBudget struct {
	WindowTokens     int
	WindowSource     string
	OutputReserve    int
	ToolReserve      int
	SafetyReserve    int
	InputLimitTokens int
	TriggerTokens    int
	TargetTokens     int
}

type contextCompressionResult struct {
	Budget          contextBudget
	Compressed      bool
	BeforeTokens    int
	AfterTokens     int
	RemovedMessages int
}

func (s *Service) contextCompressionPreview(route chatRoute) (contextBudget, int, bool) {
	profile, _ := ReasoningProfileFor(s.reasoning)
	budget := contextBudgetForRoutePolicy(route, profile.Budget.ContextPolicy)
	estimated := estimateProtocolMessagesTokens(s.sessionMessages)
	return budget, estimated, estimated > budget.TriggerTokens
}

func (s *Service) prepareSessionContextWithEvents(route chatRoute, sink ChatEventSink) (contextCompressionResult, error) {
	budget, before, needed := s.contextCompressionPreview(route)
	if needed {
		emitChatEvent(sink, ChatStreamEvent{
			Type:    "context_compression",
			Message: "正在自动压缩上下文",
			Model:   route.ModelID,
			Compression: &ContextCompressionEvent{
				Status:       "running",
				BeforeTokens: before,
				TargetTokens: budget.TargetTokens,
			},
		})
	}
	result, err := s.prepareSessionContext(route)
	if err != nil {
		if needed {
			emitChatEvent(sink, ChatStreamEvent{
				Type:    "context_compression",
				Message: "自动压缩上下文失败",
				Model:   route.ModelID,
				Compression: &ContextCompressionEvent{
					Status:       "error",
					BeforeTokens: before,
					TargetTokens: budget.TargetTokens,
				},
			})
		}
		return result, err
	}
	if needed {
		message := "上下文已整理，无需移除历史消息"
		if result.Compressed {
			message = fmt.Sprintf("上下文已自动压缩：%d → %d tokens", result.BeforeTokens, result.AfterTokens)
		}
		emitChatEvent(sink, ChatStreamEvent{
			Type:    "context_compression",
			Message: message,
			Model:   route.ModelID,
			Compression: &ContextCompressionEvent{
				Status:          "completed",
				BeforeTokens:    result.BeforeTokens,
				AfterTokens:     result.AfterTokens,
				RemovedMessages: result.RemovedMessages,
				TargetTokens:    result.Budget.TargetTokens,
			},
		})
	}
	return result, nil
}

func (s *Service) prepareSessionContext(route chatRoute) (contextCompressionResult, error) {
	profile, _ := ReasoningProfileFor(s.reasoning)
	budget := contextBudgetForRoutePolicy(route, profile.Budget.ContextPolicy)
	result := contextCompressionResult{Budget: budget, BeforeTokens: estimateProtocolMessagesTokens(s.sessionMessages)}
	s.updateContextTelemetry(budget, result.BeforeTokens)
	if result.BeforeTokens <= budget.TriggerTokens {
		result.AfterTokens = result.BeforeTokens
		return result, nil
	}

	compressed, removed := compressProtocolMessages(s.sessionMessages, budget)
	result.AfterTokens = estimateProtocolMessagesTokens(compressed)
	if result.AfterTokens > budget.InputLimitTokens {
		return result, fmt.Errorf(
			"当前请求约 %d tokens，超过模型 %s 的可用输入预算 %d tokens；请缩短本轮输入或选择更大上下文模型",
			result.AfterTokens,
			route.ModelID,
			budget.InputLimitTokens,
		)
	}
	if removed == 0 && protocolMessagesEqualSlice(compressed, s.sessionMessages) {
		result.AfterTokens = result.BeforeTokens
		return result, nil
	}

	s.sessionMessages = compressed
	result.Compressed = true
	result.RemovedMessages = removed
	s.sessionState.CompressionCount++
	s.sessionState.CompressedMessageCount += removed
	s.sessionState.LastCompressedAt = time.Now().Format(time.RFC3339)
	s.updateContextTelemetry(budget, result.AfterTokens)
	return result, nil
}

func (s *Service) updateContextTelemetry(budget contextBudget, estimatedTokens int) {
	s.sessionState.ContextWindowTokens = budget.WindowTokens
	s.sessionState.ContextWindowSource = budget.WindowSource
	s.sessionState.EstimatedInputTokens = estimatedTokens
	s.sessionState.InputBudgetTokens = budget.InputLimitTokens
	if budget.InputLimitTokens > 0 {
		s.sessionState.ContextUsagePercent = float64(estimatedTokens) / float64(budget.InputLimitTokens) * 100
	} else {
		s.sessionState.ContextUsagePercent = 0
	}
}

func contextBudgetForRoute(route chatRoute) contextBudget {
	return contextBudgetForRoutePolicy(route, "full-relevant")
}

func contextBudgetForRoutePolicy(route chatRoute, policy string) contextBudget {
	resolved := resolveProviderModelContexts(route.Provider, []protocol.Model{{ID: route.ModelID, Provider: route.Provider.ID}})
	windowTokens := safeDefaultContextWindowTokens
	windowSource := ContextWindowSourceFallback
	if len(resolved) > 0 && resolved[0].ContextWindowTokens > 0 {
		windowTokens = resolved[0].ContextWindowTokens
		windowSource = resolved[0].ContextWindowSource
	}

	outputReserve := clampContextReserve(windowTokens/8, 2_048, 16_384)
	toolReserve := clampContextReserve(windowTokens/12, 1_024, 8_192)
	safetyReserve := clampContextReserve(windowTokens/20, 512, 4_096)
	inputLimit := windowTokens - outputReserve - toolReserve - safetyReserve
	if inputLimit < windowTokens/2 {
		inputLimit = windowTokens / 2
	}
	triggerRatio, targetRatio := 90, 65
	switch policy {
	case "minimal":
		triggerRatio, targetRatio = 72, 48
	case "task-summary":
		triggerRatio, targetRatio = 80, 56
	case "expanded":
		triggerRatio, targetRatio = 87, 64
	case "full-relevant":
		triggerRatio, targetRatio = 92, 72
	}
	return contextBudget{
		WindowTokens:     windowTokens,
		WindowSource:     windowSource,
		OutputReserve:    outputReserve,
		ToolReserve:      toolReserve,
		SafetyReserve:    safetyReserve,
		InputLimitTokens: inputLimit,
		TriggerTokens:    inputLimit * triggerRatio / 100,
		TargetTokens:     inputLimit * targetRatio / 100,
	}
}

func clampContextReserve(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func compressProtocolMessages(messages []protocol.Message, budget contextBudget) ([]protocol.Message, int) {
	if len(messages) <= 1 {
		return cloneProtocolMessages(messages), 0
	}

	result := make([]protocol.Message, 0, len(messages))
	result = append(result, messages[0])
	start := 1
	existingSummary := ""
	existingArtifacts := ""
	existingFailures := ""
	existingExecution := ""
	for start < len(messages) {
		switch messages[start].InternalKind {
		case contextSummaryKind:
			existingSummary = messages[start].Content
		case contextArtifactKind:
			existingArtifacts = messages[start].Content
		case contextFailureStrategyKind:
			existingFailures = messages[start].Content
		case contextExecutionKind:
			existingExecution = messages[start].Content
		default:
			goto internalMessagesRead
		}
		start++
	}

internalMessagesRead:
	body := cloneProtocolMessages(messages[start:])
	filteredBody := make([]protocol.Message, 0, len(body))
	removedFailureContexts := 0
	removedExecutionContexts := 0
	latestRequestContext := -1
	for index := range body {
		if body[index].InternalKind == contextRequestKind {
			latestRequestContext = index
		}
	}
	removedRequestContexts := 0
	for index, message := range body {
		if message.InternalKind == contextFailureStrategyKind {
			existingFailures = message.Content
			removedFailureContexts++
			continue
		}
		if message.InternalKind == contextRequestKind && index != latestRequestContext {
			removedRequestContexts++
			continue
		}
		if message.InternalKind == contextExecutionKind {
			existingExecution = message.Content
			removedExecutionContexts++
			continue
		}
		if message.Role == "assistant" {
			if checkpoint := extractExecutionCheckpoint(message.Content); checkpoint != "" {
				existingExecution = checkpoint
				message.Content = stripExecutionCheckpoint(message.Content)
			}
		}
		filteredBody = append(filteredBody, message)
	}
	body = filteredBody
	if len(body) == 0 {
		if existingSummary != "" {
			result = append(result, protocol.Message{Role: "system", Content: existingSummary, InternalKind: contextSummaryKind})
		}
		if references := recentLocalArtifactReferences(existingArtifacts); len(references) > 0 {
			result = append(result, protocol.Message{Role: "system", Content: formatLocalArtifactContext(references, artifactContextRuneBudget(budget)), InternalKind: contextArtifactKind})
		}
		if strings.TrimSpace(existingFailures) != "" {
			result = append(result, protocol.Message{Role: "system", Content: existingFailures, InternalKind: contextFailureStrategyKind})
		}
		if strings.TrimSpace(existingExecution) != "" {
			result = append(result, protocol.Message{Role: "user", Content: existingExecution, InternalKind: contextExecutionKind})
		}
		return result, removedFailureContexts + removedRequestContexts + removedExecutionContexts
	}
	groups := groupProtocolMessages(body)

	keepCount := len(groups)
	if keepCount > 6 {
		keepCount = 6
	}
	tailStart := len(groups) - keepCount
	for tailStart < len(groups)-2 {
		candidate := append([]protocol.Message{messages[0]}, flattenProtocolMessageGroups(groups[tailStart:])...)
		if estimateProtocolMessagesTokens(candidate) <= budget.TargetTokens*3/4 {
			break
		}
		tailStart++
	}
	if tailStart == 0 && len(groups) > 2 {
		tailStart = len(groups) - 2
	}

	oldMessages := flattenProtocolMessageGroups(groups[:tailStart])
	recentMessages := flattenProtocolMessageGroups(groups[tailStart:])
	removed := len(oldMessages) + removedFailureContexts + removedRequestContexts + removedExecutionContexts
	if existingSummary != "" || len(oldMessages) > 0 {
		summaryTokens := budget.TargetTokens / 4
		if summaryTokens < 512 {
			summaryTokens = 512
		}
		if summaryTokens > 8_192 {
			summaryTokens = 8_192
		}
		result = append(result, protocol.Message{
			Role:         "system",
			Content:      buildContextSummary(existingSummary, oldMessages, summaryTokens),
			InternalKind: contextSummaryKind,
		})
	}
	artifactContents := make([]string, 0, len(body)+1)
	artifactContents = append(artifactContents, existingArtifacts)
	for _, message := range body {
		artifactContents = append(artifactContents, message.Content)
	}
	if references := recentLocalArtifactReferences(artifactContents...); len(references) > 0 {
		result = append(result, protocol.Message{
			Role:         "system",
			Content:      formatLocalArtifactContext(references, artifactContextRuneBudget(budget)),
			InternalKind: contextArtifactKind,
		})
	}
	if strings.TrimSpace(existingFailures) != "" {
		result = append(result, protocol.Message{
			Role:         "system",
			Content:      existingFailures,
			InternalKind: contextFailureStrategyKind,
		})
	}
	if strings.TrimSpace(existingExecution) != "" {
		result = append(result, protocol.Message{
			Role:         "user",
			Content:      existingExecution,
			InternalKind: contextExecutionKind,
		})
	}
	result = append(result, recentMessages...)

	clipProtocolMessagesToBudget(result, budget.TargetTokens)
	return result, removed
}

func artifactContextRuneBudget(budget contextBudget) int {
	limit := budget.TargetTokens * 3 / 5
	if limit < 750 {
		return 750
	}
	if limit > 6_000 {
		return 6_000
	}
	return limit
}

func groupProtocolMessages(messages []protocol.Message) [][]protocol.Message {
	groups := make([][]protocol.Message, 0, len(messages))
	for index := 0; index < len(messages); {
		message := messages[index]
		group := []protocol.Message{message}
		index++
		if message.Role == "assistant" && len(message.ToolCalls) > 0 {
			callIDs := make(map[string]bool, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				callIDs[call.ID] = true
			}
			for index < len(messages) && messages[index].Role == "tool" && callIDs[messages[index].ToolCallID] {
				group = append(group, messages[index])
				index++
			}
		}
		groups = append(groups, group)
	}
	return groups
}

func flattenProtocolMessageGroups(groups [][]protocol.Message) []protocol.Message {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	messages := make([]protocol.Message, 0, total)
	for _, group := range groups {
		messages = append(messages, group...)
	}
	return messages
}

func clipProtocolMessagesToBudget(messages []protocol.Message, targetTokens int) {
	for _, maxRunes := range []int{1_500, 700, 300} {
		if estimateProtocolMessagesTokens(messages) <= targetTokens {
			return
		}
		for index := 1; index < len(messages); index++ {
			message := &messages[index]
			if message.InternalKind == contextRequestKind {
				message.Content = clipDelimitedContext(message.Content, requestContextStart, requestContextEnd, maxRunes)
				continue
			}
			if message.InternalKind == contextExecutionKind {
				message.Content = clipDelimitedContext(message.Content, executionContextStart, executionContextEnd, maxRunes)
				continue
			}
			if message.InternalKind == contextSummaryKind || message.Role == "tool" || (message.Role == "assistant" && len(message.ToolCalls) == 0) {
				message.Content = clipContextText(message.Content, maxRunes)
			}
		}
	}
}

func buildContextSummary(existing string, messages []protocol.Message, maxTokens int) string {
	maxRunes := maxTokens * 3
	if maxRunes < 1_500 {
		maxRunes = 1_500
	}
	var lines []string
	lines = append(lines, "[MHcode compressed conversation memory]")
	if strings.TrimSpace(existing) != "" {
		memory := strings.TrimPrefix(stripPrivateAssistantContext(existing), "[MHcode compressed conversation memory]")
		lines = append(lines, "Previous memory: "+compactContextLine(memory, maxRunes/3))
	}
	for _, message := range messages {
		if message.InternalKind == contextRequestKind || message.InternalKind == contextExecutionKind {
			continue
		}
		role := message.Role
		if role == "tool" && message.Name != "" {
			role = "tool:" + message.Name
		}
		line := role + ": " + compactContextLine(stripPrivateAssistantContext(message.Content), 1_200)
		if len(message.Attachments) > 0 {
			names := make([]string, 0, len(message.Attachments))
			for _, attachment := range message.Attachments {
				names = append(names, attachment.Name)
			}
			line += " [images: " + strings.Join(names, ", ") + "]"
		}
		if len(message.ToolCalls) > 0 {
			names := make([]string, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				names = append(names, call.Function.Name)
			}
			line += " [tools: " + strings.Join(names, ", ") + "]"
		}
		lines = append(lines, line)
	}
	return clipContextText(strings.Join(lines, "\n"), maxRunes)
}

func compactContextLine(value string, maxRunes int) string {
	return clipContextText(strings.Join(strings.Fields(value), " "), maxRunes)
}

func clipContextText(value string, maxRunes int) string {
	runes := []rune(value)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return value
	}
	head := maxRunes * 2 / 3
	tail := maxRunes - head
	return string(runes[:head]) + "\n... [context compressed] ...\n" + string(runes[len(runes)-tail:])
}

func estimateProtocolMessagesTokens(messages []protocol.Message) int {
	total := 0
	for _, message := range messages {
		total += 4 + estimatePromptTokens(message.Role) + estimatePromptTokens(message.Content)
		total += len(message.Attachments) * 1_024
		if len(message.ToolCalls) > 0 {
			encoded, _ := json.Marshal(message.ToolCalls)
			total += estimatePromptTokens(string(encoded))
		}
		if message.ToolCallID != "" {
			total += estimatePromptTokens(message.ToolCallID) + estimatePromptTokens(message.Name)
		}
	}
	return total
}

func fitToolLoopMessages(messages []protocol.Message, targetTokens int) []protocol.Message {
	fitted := cloneProtocolMessages(messages)
	if targetTokens <= 0 || estimateProtocolMessagesTokens(fitted) <= targetTokens {
		return fitted
	}
	clipProtocolMessagesToBudget(fitted, targetTokens)
	return fitted
}

func protocolMessagesEqualSlice(left, right []protocol.Message) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Role != right[index].Role || left[index].Content != right[index].Content || left[index].InternalKind != right[index].InternalKind || !protocolAttachmentsEqual(left[index].Attachments, right[index].Attachments) {
			return false
		}
	}
	return true
}

func protocolAttachmentsEqual(left, right []protocol.Attachment) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
