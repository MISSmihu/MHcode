package agent

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/MISSmihu/MHcode/internal/eventlog"
	"github.com/MISSmihu/MHcode/internal/tools"
)

func newTaskRuntimeTestService(t *testing.T, base string) *Service {
	t.Helper()
	return NewService(ServiceConfig{
		SkillsDir:    t.TempDir(),
		SessionsDir:  filepath.Join(base, "sessions"),
		ProjectsPath: filepath.Join(base, "projects.json"),
	})
}

func TestTaskRuntimePersistsLiveToolAndTerminalState(t *testing.T) {
	service := newTaskRuntimeTestService(t, t.TempDir())
	if err := service.StartTaskRuntime("task-live", "2026-07-27T01:02:03Z"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordTaskStreamEvent("task-live", ChatStreamEvent{Type: "delta", Delta: "partial answer"}); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordTaskStreamEvent("task-live", ChatStreamEvent{
		Type: "tool", ToolName: "run_command", ToolCallID: "call-1", ToolInput: "go test ./...", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	record, ok, err := service.eventStore.ReadTaskRuntime()
	if err != nil || !ok {
		t.Fatalf("runtime ok=%v err=%v", ok, err)
	}
	if record.Content != "partial answer" || record.Status != "running" || len(record.Parts) != 1 {
		t.Fatalf("live runtime = %#v", record)
	}
	if err := service.FinishTaskRuntime("task-live", "cancelled", "已停止", ChatResult{}); err != nil {
		t.Fatal(err)
	}
	record, ok, err = service.eventStore.ReadTaskRuntime()
	if err != nil || !ok || record.Status != "cancelled" {
		t.Fatalf("terminal runtime = %#v ok=%v err=%v", record, ok, err)
	}
	if record.Parts[0].Status != "error" || record.Parts[0].Output == "" {
		t.Fatalf("running tool was not settled: %#v", record.Parts[0])
	}
}

func TestTaskRuntimeHeartbeatUpdatesStatusWithoutTimelineNoise(t *testing.T) {
	service := newTaskRuntimeTestService(t, t.TempDir())
	if err := service.StartTaskRuntime("task-heartbeat", "2026-07-27T01:02:03Z"); err != nil {
		t.Fatal(err)
	}
	service.taskRuntimeLastWrite = time.Time{}
	if err := service.RecordTaskStreamEvent("task-heartbeat", ChatStreamEvent{
		Type: "heartbeat", Message: "upstream still processing", Status: "waiting",
	}); err != nil {
		t.Fatal(err)
	}
	record, ok, err := service.eventStore.ReadTaskRuntime()
	if err != nil || !ok {
		t.Fatalf("runtime ok=%v err=%v", ok, err)
	}
	if record.Status != "waiting" || record.Message != "upstream still processing" || len(record.Parts) != 0 {
		t.Fatalf("heartbeat runtime = %#v", record)
	}
}

func TestServiceStartupRecoversInterruptedTaskOnce(t *testing.T) {
	base := t.TempDir()
	service := newTaskRuntimeTestService(t, base)
	if err := service.StartTaskRuntime("task-crashed", "2026-07-27T01:02:03Z"); err != nil {
		t.Fatal(err)
	}
	service.recordUserEvent("继续完成报表")
	if _, err := service.eventStore.Append(eventlog.EventPayload{
		PlanStatus: "running",
		PlanSteps:  []eventlog.MessageProgressStep{{Title: "生成报表", Status: "in_progress"}},
	}, eventlog.EventPlanUpdate); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordTaskStreamEvent("task-crashed", ChatStreamEvent{Type: "delta", Delta: "已经生成基础文件。"}); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordTaskStreamEvent("task-crashed", ChatStreamEvent{
		Type: "tool", ToolName: "render_artifact", ToolCallID: "render-1", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}

	restarted := newTaskRuntimeTestService(t, base)
	history := restarted.GetSessionMessages()
	if len(history) != 2 {
		t.Fatalf("recovered history = %#v", history)
	}
	terminal := history[1]
	if terminal.Status != "interrupted" || terminal.Content != "已经生成基础文件。" {
		t.Fatalf("recovered terminal = %#v", terminal)
	}
	if len(terminal.Parts) != 2 || terminal.Parts[0].Status != "error" || terminal.Parts[1].Kind != tools.PartText {
		t.Fatalf("recovered tool parts = %#v", terminal.Parts)
	}
	if restarted.planState.Status != "interrupted" || len(restarted.planState.Steps) != 1 {
		t.Fatalf("recovered plan = %#v", restarted.planState)
	}
	record, ok, err := restarted.eventStore.ReadTaskRuntime()
	if err != nil || !ok || record.Status != "interrupted" {
		t.Fatalf("recovered runtime = %#v ok=%v err=%v", record, ok, err)
	}

	restartedAgain := newTaskRuntimeTestService(t, base)
	if got := restartedAgain.GetSessionMessages(); len(got) != 2 {
		t.Fatalf("repeated startup duplicated interrupted turn: %#v", got)
	}
}

func TestStartupDoesNotDuplicateAlreadyCommittedTask(t *testing.T) {
	base := t.TempDir()
	service := newTaskRuntimeTestService(t, base)
	if err := service.StartTaskRuntime("task-committed", "2026-07-27T01:02:03Z"); err != nil {
		t.Fatal(err)
	}
	service.recordUserEvent("完成任务")
	if err := service.RecordTaskStreamEvent("task-committed", ChatStreamEvent{Type: "status", Message: "正在收尾"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.eventStore.Append(eventlog.EventPayload{
		Role: "assistant", Content: "任务完成", Parts: []eventlog.MessagePart{{Kind: string(tools.PartText), Text: "任务完成"}},
	}, eventlog.EventAssistantMessage); err != nil {
		t.Fatal(err)
	}
	if _, err := service.eventStore.Append(eventlog.EventPayload{Label: "任务完成", TurnIndex: 1}, eventlog.EventCheckpoint); err != nil {
		t.Fatal(err)
	}

	restarted := newTaskRuntimeTestService(t, base)
	history := restarted.GetSessionMessages()
	if len(history) != 2 || history[1].Content != "任务完成" || history[1].Status != "" {
		t.Fatalf("committed task was recovered as interrupted: %#v", history)
	}
	record, ok, err := restarted.eventStore.ReadTaskRuntime()
	if err != nil || !ok || record.Status != "completed" {
		t.Fatalf("committed runtime = %#v ok=%v err=%v", record, ok, err)
	}
}
