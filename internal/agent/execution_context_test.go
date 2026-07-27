package agent

import (
	"strings"
	"testing"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

func TestProtocolAssistantContentAddsPrivateExecutionCheckpoint(t *testing.T) {
	exitCode := 1
	service := &Service{planState: PlanState{
		Revision: 3,
		Status:   "cancelled",
		Steps: []tools.ProgressStep{
			{Title: "Inspect the workbook", Status: "completed"},
			{Title: "Render and verify", Status: "in_progress"},
		},
	}}
	content := service.protocolAssistantContent("visible answer", []tools.ResultPart{{
		Kind:             tools.PartToolCall,
		Name:             "run_command",
		Status:           "error",
		ToolCallID:       "call-1",
		Input:            `python -c "print('verify')"`,
		WorkingDirectory: `C:\workspace`,
		Stderr:           "SyntaxError: unterminated string literal",
		ExitCode:         &exitCode,
		DurationMs:       120,
	}})

	for _, expected := range []string{
		executionContextStart,
		"plan revision=3 status=cancelled",
		"Render and verify",
		"tool name=run_command status=error",
		"exit_code=1",
		"SyntaxError: unterminated string literal",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("execution checkpoint is missing %q: %q", expected, content)
		}
	}
	if visible := stripPrivateAssistantContext(content); visible != "visible answer" {
		t.Fatalf("visible assistant content = %q", visible)
	}
}

func TestContinueInjectsLatestExecutionCheckpoint(t *testing.T) {
	service := &Service{planState: PlanState{
		Revision: 1,
		Status:   "cancelled",
		Steps:    []tools.ProgressStep{{Title: "Run verification", Status: "in_progress"}},
	}}
	checkpointed := service.protocolAssistantContent("stopped", []tools.ResultPart{{
		Kind: tools.PartToolCall, Name: "read_file", Status: "ok", Input: `{"path":"report.xlsx"}`, Output: "read complete",
	}})
	service.sessionMessages = []protocol.Message{{Role: "assistant", Content: checkpointed}}

	preview := service.contextPreviewForInput("continue")
	executionState := ""
	for _, section := range preview.VolatileTail {
		if section.Name == "execution_state" {
			executionState = section.Content
			break
		}
	}
	if !strings.Contains(executionState, "read_file") || !strings.Contains(executionState, "Run verification") {
		t.Fatalf("continue context did not restore execution checkpoint: %q", executionState)
	}
	privateContext := formatPrivateTurnContext(preview)
	if !strings.Contains(privateContext, "[execution_state]") || !strings.Contains(privateContext, "read_file") {
		t.Fatalf("execution state was not sent in the private turn context: %q", privateContext)
	}
}

func TestUnrelatedTurnDoesNotInjectCompletedExecutionCheckpoint(t *testing.T) {
	service := &Service{planState: PlanState{
		Revision: 1,
		Status:   "completed",
		Steps:    []tools.ProgressStep{{Title: "Done", Status: "completed"}},
	}}
	service.sessionMessages = []protocol.Message{{Role: "assistant", Content: service.protocolAssistantContent("done", nil)}}

	preview := service.contextPreviewForInput("explain another file")
	for _, section := range preview.VolatileTail {
		if section.Name == "execution_state" && strings.TrimSpace(section.Content) != "" {
			t.Fatalf("completed checkpoint leaked into unrelated turn: %q", section.Content)
		}
	}
}
