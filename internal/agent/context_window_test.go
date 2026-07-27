package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

func TestCompressProtocolMessagesKeepsSystemAndRecentTail(t *testing.T) {
	messages := []protocol.Message{{Role: "system", Content: "stable-system"}}
	for index := 0; index < 12; index++ {
		messages = append(messages,
			protocol.Message{Role: "user", Content: strings.Repeat("old-user ", 250)},
			protocol.Message{Role: "assistant", Content: strings.Repeat("old-answer ", 250)},
		)
	}
	messages = append(messages, protocol.Message{Role: "user", Content: "current request must survive"})
	budget := contextBudget{InputLimitTokens: 5_000, TriggerTokens: 4_500, TargetTokens: 2_500}

	compressed, removed := compressProtocolMessages(messages, budget)
	if removed == 0 || len(compressed) >= len(messages) {
		t.Fatalf("removed = %d, message count %d -> %d", removed, len(messages), len(compressed))
	}
	if compressed[0].Role != "system" || compressed[0].Content != "stable-system" {
		t.Fatalf("stable system message changed: %#v", compressed[0])
	}
	if compressed[1].InternalKind != contextSummaryKind || !strings.Contains(compressed[1].Content, "compressed conversation memory") {
		t.Fatalf("summary message = %#v", compressed[1])
	}
	last := compressed[len(compressed)-1]
	if last.Content != "current request must survive" {
		t.Fatalf("current message = %#v", last)
	}
	if estimateProtocolMessagesTokens(compressed) > budget.InputLimitTokens {
		t.Fatalf("compressed estimate = %d, limit = %d", estimateProtocolMessagesTokens(compressed), budget.InputLimitTokens)
	}
}

func TestContextBudgetUsesManualModelWindow(t *testing.T) {
	route := chatRoute{
		Provider: ModelProviderSetting{ID: "custom", Protocol: "openai-compatible", Models: []ProviderModel{{
			ID:                  "manual-model",
			ContextWindowTokens: 96_000,
			ContextWindowSource: ContextWindowSourceManual,
		}}},
		ModelID: "manual-model",
	}
	budget := contextBudgetForRoute(route)
	if budget.WindowTokens != 96_000 || budget.WindowSource != ContextWindowSourceManual {
		t.Fatalf("budget = %#v", budget)
	}
	if budget.TargetTokens <= 0 || budget.TargetTokens >= budget.InputLimitTokens || budget.InputLimitTokens >= budget.WindowTokens {
		t.Fatalf("invalid reserves: %#v", budget)
	}
}

func TestFitToolLoopMessagesShrinksOldToolResults(t *testing.T) {
	messages := []protocol.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "task"},
		{Role: "assistant", ToolCalls: []protocol.ToolCall{{ID: "call-1", Function: protocol.ToolCallFunction{Name: "read_file"}}}},
		{Role: "tool", Name: "read_file", ToolCallID: "call-1", Content: strings.Repeat("large output ", 3_000)},
		{Role: "user", Content: "continue"},
	}
	fitted := fitToolLoopMessages(messages, 1_500)
	if len(fitted[3].Content) >= len(messages[3].Content) {
		t.Fatal("old tool output was not compacted")
	}
	if messages[3].Content == fitted[3].Content {
		t.Fatal("input messages were mutated")
	}
}

func TestCompressProtocolMessagesKeepsToolCallAndResultTogether(t *testing.T) {
	messages := []protocol.Message{{Role: "system", Content: "system"}}
	for index := 0; index < 8; index++ {
		callID := "call-" + string(rune('a'+index))
		messages = append(messages,
			protocol.Message{Role: "user", Content: strings.Repeat("request ", 200)},
			protocol.Message{Role: "assistant", ToolCalls: []protocol.ToolCall{{ID: callID, Function: protocol.ToolCallFunction{Name: "read_file"}}}},
			protocol.Message{Role: "tool", ToolCallID: callID, Name: "read_file", Content: strings.Repeat("result ", 400)},
		)
	}
	messages = append(messages, protocol.Message{Role: "user", Content: "latest request"})
	compressed, _ := compressProtocolMessages(messages, contextBudget{InputLimitTokens: 4_000, TargetTokens: 2_000})
	for index, message := range compressed {
		if message.Role != "assistant" || len(message.ToolCalls) == 0 {
			continue
		}
		if index+1 >= len(compressed) || compressed[index+1].Role != "tool" || compressed[index+1].ToolCallID != message.ToolCalls[0].ID {
			t.Fatalf("tool protocol pair was split at index %d: %#v", index, compressed)
		}
	}
}

func TestCompressProtocolMessagesKeepsRecentArtifactIndex(t *testing.T) {
	service := &Service{runtimeSettings: RuntimeSettings{WorkspaceRoot: t.TempDir()}}
	artifactPath := filepath.Join(service.runtimeSettings.WorkspaceRoot, "exports", "report.xlsx")
	messages := []protocol.Message{{Role: "system", Content: "stable-system"}}
	messages = append(messages,
		protocol.Message{Role: "user", Content: "create the report"},
		service.protocolAssistantMessage("report created", []tools.ResultPart{{
			Kind: tools.PartFile, Path: artifactPath, FileAction: "created", Created: true,
		}}),
	)
	for index := 0; index < 10; index++ {
		messages = append(messages,
			protocol.Message{Role: "user", Content: strings.Repeat("follow-up ", 180)},
			protocol.Message{Role: "assistant", Content: strings.Repeat("answer ", 180)},
		)
	}

	compressed, removed := compressProtocolMessages(messages, contextBudget{InputLimitTokens: 5_000, TargetTokens: 2_500})
	if removed == 0 {
		t.Fatal("expected old messages to be compressed")
	}
	foundArtifactIndex := false
	for _, message := range compressed {
		if message.InternalKind == contextArtifactKind {
			foundArtifactIndex = strings.Contains(message.Content, artifactPath)
		}
		if message.InternalKind == contextSummaryKind && strings.Contains(message.Content, localArtifactContextStart) {
			t.Fatalf("artifact context was duplicated into conversation summary: %q", message.Content)
		}
	}
	if !foundArtifactIndex {
		t.Fatalf("compressed messages lost artifact path %q: %#v", artifactPath, compressed)
	}
}

func TestCompressProtocolMessagesPreservesLatestExecutionCheckpoint(t *testing.T) {
	service := &Service{planState: PlanState{
		Revision: 2,
		Status:   "cancelled",
		Steps:    []tools.ProgressStep{{Title: "Retry with a different renderer", Status: "in_progress"}},
	}}
	exitCode := 1
	checkpointed := service.protocolAssistantContent("work interrupted", []tools.ResultPart{{
		Kind: tools.PartToolCall, Name: "run_command", Status: "error", ToolCallID: "render-2",
		Input: "render workbook", Stderr: "renderer crashed", ExitCode: &exitCode,
	}})

	messages := []protocol.Message{{Role: "system", Content: "stable-system"}}
	for index := 0; index < 10; index++ {
		messages = append(messages,
			protocol.Message{Role: "user", Content: strings.Repeat("old request ", 160)},
			protocol.Message{Role: "assistant", Content: strings.Repeat("old answer ", 160)},
		)
	}
	messages = append(messages,
		protocol.Message{Role: "user", Content: "resume source"},
		protocol.Message{Role: "assistant", Content: checkpointed},
		protocol.Message{Role: "user", Content: requestContextStart + "\n[execution_state]\nresume\n" + requestContextEnd, InternalKind: contextRequestKind},
		protocol.Message{Role: "user", Content: "continue"},
	)

	compressed, removed := compressProtocolMessages(messages, contextBudget{InputLimitTokens: 5_000, TargetTokens: 2_500})
	if removed == 0 {
		t.Fatal("expected compression to remove old history")
	}
	executionCount := 0
	for _, message := range compressed {
		if message.InternalKind == contextExecutionKind {
			executionCount++
			if !strings.Contains(message.Content, executionContextStart) || !strings.Contains(message.Content, "renderer crashed") || !strings.Contains(message.Content, executionContextEnd) {
				t.Fatalf("execution checkpoint was truncated or changed: %q", message.Content)
			}
		}
		if message.InternalKind == contextSummaryKind && strings.Contains(message.Content, executionContextStart) {
			t.Fatalf("execution checkpoint leaked into compressed prose summary: %q", message.Content)
		}
	}
	if executionCount != 1 {
		t.Fatalf("execution checkpoint count = %d, want 1: %#v", executionCount, compressed)
	}
	last := compressed[len(compressed)-1]
	if last.Role != "user" || last.Content != "continue" {
		t.Fatalf("latest user request was not preserved: %#v", last)
	}
}

func TestClipDelimitedContextKeepsMarkers(t *testing.T) {
	content := requestContextStart + "\n" + strings.Repeat("private context ", 500) + "\n" + requestContextEnd
	clipped := clipDelimitedContext(content, requestContextStart, requestContextEnd, 400)
	if !strings.Contains(clipped, requestContextStart) || !strings.Contains(clipped, requestContextEnd) {
		t.Fatalf("clipped context lost delimiters: %q", clipped)
	}
	if len([]rune(clipped)) > 430 {
		t.Fatalf("clipped context is unexpectedly large: %d runes", len([]rune(clipped)))
	}
}

func TestFitToolLoopMessagesClipsLastLargeToolResult(t *testing.T) {
	messages := []protocol.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "task"},
		{Role: "assistant", ToolCalls: []protocol.ToolCall{{ID: "call-last", Function: protocol.ToolCallFunction{Name: "run_command"}}}},
		{Role: "tool", ToolCallID: "call-last", Name: "run_command", Content: strings.Repeat("very large output ", 10_000)},
	}
	fitted := fitToolLoopMessages(messages, 1_000)
	if len(fitted[3].Content) >= len(messages[3].Content) {
		t.Fatal("latest tool output was not compacted")
	}
	if fitted[2].ToolCalls[0].ID != fitted[3].ToolCallID {
		t.Fatal("latest tool pair was not preserved")
	}
}

func TestEstimatePromptTokensAccountsForCJKText(t *testing.T) {
	if got := estimatePromptTokens(strings.Repeat("中", 100)); got < 100 {
		t.Fatalf("CJK estimate = %d, want at least one token per rune", got)
	}
	if got := estimatePromptTokens(strings.Repeat("a", 300)); got != 100 {
		t.Fatalf("ASCII estimate = %d, want 100", got)
	}
}

func TestPrepareSessionContextEmitsAutomaticCompressionEvents(t *testing.T) {
	service := &Service{reasoning: ReasoningUltra}
	service.sessionMessages = []protocol.Message{{Role: "system", Content: "stable"}}
	for index := 0; index < 12; index++ {
		service.sessionMessages = append(service.sessionMessages,
			protocol.Message{Role: "user", Content: strings.Repeat("request ", 300)},
			protocol.Message{Role: "assistant", Content: strings.Repeat("answer ", 300)},
		)
	}
	route := chatRoute{
		Provider: ModelProviderSetting{ID: "test", Models: []ProviderModel{{
			ID: "small", ContextWindowTokens: 8_000, ContextWindowSource: ContextWindowSourceManual,
		}}},
		ModelID: "small",
	}
	var events []ChatStreamEvent
	result, err := service.prepareSessionContextWithEvents(route, func(event ChatStreamEvent) { events = append(events, event) })
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compressed || len(events) != 2 {
		t.Fatalf("result=%#v events=%#v", result, events)
	}
	if events[0].Type != "context_compression" || events[0].Compression == nil || events[0].Compression.Status != "running" {
		t.Fatalf("running event = %#v", events[0])
	}
	if events[1].Compression == nil || events[1].Compression.Status != "completed" || events[1].Compression.AfterTokens >= events[1].Compression.BeforeTokens {
		t.Fatalf("completed event = %#v", events[1])
	}
}
