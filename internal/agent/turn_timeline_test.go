package agent

import (
	"testing"

	"github.com/MISSmihu/MHcode/internal/tools"
)

func TestTurnTimelinePersistsProgressAroundToolActivity(t *testing.T) {
	service := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	service.resetTurnTimeline()
	sink := service.captureTurnTimeline(nil)
	sink(ChatStreamEvent{Type: "status", Message: "正在分析任务", Status: "running"})
	sink(ChatStreamEvent{
		Type: "tool", ToolName: "run_command", ToolCallID: "call-1", ToolInput: "go test ./...", Status: "running",
	})
	sink(ChatStreamEvent{
		Type: "tool", ToolName: "run_command", ToolCallID: "call-1", ToolInput: "go test ./...", Status: "completed",
		Message: "测试通过", Parts: []tools.ResultPart{{
			Kind: tools.PartToolCall, Name: "run_command", ToolCallID: "call-1", Status: "ok",
			Input: "go test ./...", Output: "ok",
		}},
	})
	sink(ChatStreamEvent{Type: "status", Message: "正在整理验证结果", Status: "running"})
	service.recordAssistantAndCheckpoint("全部验证通过。", "test-model", nil, 1_250)

	history := service.GetSessionMessages()
	if len(history) != 1 {
		t.Fatalf("history = %#v", history)
	}
	parts := history[0].Parts
	if len(parts) != 4 {
		t.Fatalf("timeline parts = %#v", parts)
	}
	if parts[0].Kind != tools.PartTimelineNote || parts[0].Message != "正在分析任务" || parts[0].Status != "completed" {
		t.Fatalf("first progress note = %#v", parts[0])
	}
	if parts[1].Kind != tools.PartToolCall || parts[1].Status != "ok" || parts[1].Output != "ok" {
		t.Fatalf("tool activity = %#v", parts[1])
	}
	if parts[2].Kind != tools.PartTimelineNote || parts[2].Message != "正在整理验证结果" {
		t.Fatalf("second progress note = %#v", parts[2])
	}
	if parts[3].Kind != tools.PartText || parts[3].Text != "全部验证通过。" {
		t.Fatalf("final answer = %#v", parts[3])
	}
}

func TestTurnTimelineSettlesProgressOnFailure(t *testing.T) {
	service := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	service.resetTurnTimeline()
	service.captureTurnTimelineEvent(ChatStreamEvent{Type: "status", Message: "连接中断，正在重试", Status: "retrying"})
	if err := service.recordTurnTerminal("failed", "模型服务仍不可用", "test-model", nil, 500); err != nil {
		t.Fatal(err)
	}
	history := service.GetSessionMessages()
	if len(history) != 1 || history[0].Status != "failed" || len(history[0].Parts) != 2 {
		t.Fatalf("failed timeline = %#v", history)
	}
	if history[0].Parts[0].Kind != tools.PartTimelineNote || history[0].Parts[0].Status != "failed" {
		t.Fatalf("failed progress note = %#v", history[0].Parts[0])
	}
}

func TestTurnTimelineDoesNotPersistProviderHeartbeats(t *testing.T) {
	service := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	service.resetTurnTimeline()
	service.captureTurnTimelineEvent(ChatStreamEvent{
		Type: "heartbeat", Message: "upstream still processing", Status: "waiting",
	})
	if len(service.turnTimelineParts) != 0 {
		t.Fatalf("provider heartbeat leaked into durable timeline: %#v", service.turnTimelineParts)
	}
}
