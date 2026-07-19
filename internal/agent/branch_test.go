package agent

import (
	"path/filepath"
	"testing"

	"github.com/MISSmihu/MHcode/internal/eventlog"
	"github.com/MISSmihu/MHcode/internal/tools"
)

// TestSwitchBranchMovesFilesAcrossLines 验证分支切换时文件正确在两条线间切换。
func TestSwitchBranchMovesFilesAcrossLines(t *testing.T) {
	workspace := t.TempDir()
	sessions := t.TempDir()
	target := filepath.Join(workspace, "note.txt")

	svc := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: sessions})
	svc.runtimeSettings.WorkspaceRoot = workspace
	svc.runtimeSettings.FilesystemAccess = "workspace-write"
	svc.sessionMessages = nil

	// 基线 v1。
	if err := tools.WriteFileTextAtomic(target, "v1\n", tools.FileText{LineEnding: tools.LineEndingLF}); err != nil {
		t.Fatal(err)
	}

	// 第 1 轮：v1 → v2。
	svc.recordUserEvent("改成 v2")
	svc.recordFileSnapshot(tools.FileChange{Path: "note.txt", Before: "v1\n", After: "v2\n", Existed: true, LineEnding: "lf"})
	_ = tools.WriteFileTextAtomic(target, "v2\n", tools.FileText{LineEnding: tools.LineEndingLF})
	svc.sessionState.TurnCount = 1
	svc.recordAssistantAndCheckpoint("已改 v2", "m", nil)
	cp1 := svc.ListCheckpoints()[0].ID

	// 第 2 轮（分支 A）：v2 → v3a。
	svc.recordUserEvent("改成 v3a")
	svc.recordFileSnapshot(tools.FileChange{Path: "note.txt", Before: "v2\n", After: "v3a\n", Existed: true, LineEnding: "lf"})
	_ = tools.WriteFileTextAtomic(target, "v3a\n", tools.FileText{LineEnding: tools.LineEndingLF})
	svc.sessionState.TurnCount = 2
	svc.recordAssistantAndCheckpoint("已改 v3a", "m", nil)
	branchALeaf := svc.eventStore.Head()

	// 回退到 cp1（文件回到 v2），再分叉出分支 B：v2 → v3b。
	if _, err := svc.RewindToCheckpoint(cp1); err != nil {
		t.Fatal(err)
	}
	if got, _ := tools.ReadFileText(target); got.Content != "v2\n" {
		t.Fatalf("回退后应为 v2, got %q", got.Content)
	}
	svc.recordUserEvent("改成 v3b")
	svc.recordFileSnapshot(tools.FileChange{Path: "note.txt", Before: "v2\n", After: "v3b\n", Existed: true, LineEnding: "lf"})
	_ = tools.WriteFileTextAtomic(target, "v3b\n", tools.FileText{LineEnding: tools.LineEndingLF})
	svc.sessionState.TurnCount = 2
	svc.recordAssistantAndCheckpoint("已改 v3b", "m", nil)

	// 现在应有两条分支。
	branches := svc.ListBranches()
	if len(branches) != 2 {
		t.Fatalf("分支数 = %d, want 2", len(branches))
	}

	// 当前在分支 B（v3b）。切换到分支 A → 文件应变回 v3a。
	if got, _ := tools.ReadFileText(target); got.Content != "v3b\n" {
		t.Fatalf("当前应为 v3b, got %q", got.Content)
	}
	if _, err := svc.SwitchBranch(branchALeaf); err != nil {
		t.Fatal(err)
	}
	if got, _ := tools.ReadFileText(target); got.Content != "v3a\n" {
		t.Fatalf("切换到分支 A 后应为 v3a, got %q", got.Content)
	}
}

func TestForkFromUserMessageRestoresPriorTurn(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "note.txt")
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	svc.runtimeSettings.WorkspaceRoot = workspace
	svc.runtimeSettings.FilesystemAccess = "workspace-write"
	if err := tools.WriteFileTextAtomic(target, "v1\n", tools.FileText{LineEnding: tools.LineEndingLF}); err != nil {
		t.Fatal(err)
	}

	svc.recordUserEvent("first")
	if err := svc.recordFileSnapshot(tools.FileChange{Path: "note.txt", Before: "v1\n", After: "v2\n", Existed: true, LineEnding: "lf"}); err != nil {
		t.Fatal(err)
	}
	_ = tools.WriteFileTextAtomic(target, "v2\n", tools.FileText{LineEnding: tools.LineEndingLF})
	svc.sessionState.TurnCount = 1
	svc.recordAssistantAndCheckpoint("first reply", "m", nil)

	svc.recordUserEvent("second")
	if err := svc.recordFileSnapshot(tools.FileChange{Path: "note.txt", Before: "v2\n", After: "v3\n", Existed: true, LineEnding: "lf"}); err != nil {
		t.Fatal(err)
	}
	_ = tools.WriteFileTextAtomic(target, "v3\n", tools.FileText{LineEnding: tools.LineEndingLF})
	svc.sessionState.TurnCount = 2
	svc.recordAssistantAndCheckpoint("second reply", "m", nil)

	history := svc.GetSessionMessages()
	if len(history) != 4 || history[2].ID == "" {
		t.Fatalf("历史消息事件 ID 未正确暴露: %#v", history)
	}
	if _, err := svc.ForkFromMessage(history[2].ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := tools.ReadFileText(target); got.Content != "v2\n" {
		t.Fatalf("从第二条用户消息分叉后文件 = %q, want v2", got.Content)
	}
	forkedHistory := svc.GetSessionMessages()
	if len(forkedHistory) != 2 || forkedHistory[0].Content != "first" || forkedHistory[1].Content != "first reply" {
		t.Fatalf("分叉后历史不正确: %#v", forkedHistory)
	}
	head, ok := svc.eventStore.Event(svc.eventStore.Head())
	if !ok || head.Type != eventlog.EventBranchMarker {
		t.Fatalf("分叉后 head 应为 branch marker, got %#v", head)
	}
	if branches := svc.ListBranches(); len(branches) != 2 {
		t.Fatalf("分叉数 = %d, want 2", len(branches))
	}
}

func TestForkFromAssistantKeepsReplyCheckpoint(t *testing.T) {
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	svc.recordUserEvent("first")
	svc.sessionState.TurnCount = 1
	svc.recordAssistantAndCheckpoint("first reply", "m", nil)
	svc.recordUserEvent("second")
	svc.sessionState.TurnCount = 2
	svc.recordAssistantAndCheckpoint("second reply", "m", nil)

	history := svc.GetSessionMessages()
	if _, err := svc.ForkFromMessage(history[1].ID); err != nil {
		t.Fatal(err)
	}
	forkedHistory := svc.GetSessionMessages()
	if len(forkedHistory) != 2 || forkedHistory[1].Content != "first reply" {
		t.Fatalf("助手消息分叉应保留该回复: %#v", forkedHistory)
	}
	if svc.sessionState.TurnCount != 1 {
		t.Fatalf("分叉后轮数 = %d, want 1", svc.sessionState.TurnCount)
	}
}
