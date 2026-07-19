package main

import (
	"context"
	"errors"
	"testing"

	"github.com/MISSmihu/MHcode/internal/agent"
)

func TestChatTaskCancellationClassification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	if chatTaskWasCancelled(ctx, errors.New("upstream failed")) {
		t.Fatal("ordinary provider errors must not be reported as user cancellation")
	}
	cancel()
	if !chatTaskWasCancelled(ctx, context.Canceled) {
		t.Fatal("cancelled task must be reported as cancellation")
	}
}

func TestChatGuidanceQueuesInOrderAndClosesAtomically(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	task := &chatTask{id: "task-1", cancel: cancel, acceptingGuidance: true}
	app := &App{}
	app.chat.active = task

	accepted, err := app.GuideChatMessage("task-1", "guide-1", "先检查测试")
	if err != nil || !accepted {
		t.Fatalf("first guidance accepted=%v err=%v", accepted, err)
	}
	accepted, err = app.GuideChatMessageWithAttachments("task-1", "guide-2", "再看截图", []agent.ChatAttachment{{Name: "screen.png", MIMEType: "image/png", Data: "eA=="}})
	if err != nil || !accepted {
		t.Fatalf("second guidance accepted=%v err=%v", accepted, err)
	}

	first, ok := app.takeNextChatGuidance(task)
	if !ok || first.id != "guide-1" || first.prompt != "先检查测试" {
		t.Fatalf("first guidance = %#v ok=%v", first, ok)
	}
	second, ok := app.takeNextChatGuidance(task)
	if !ok || second.id != "guide-2" || len(second.attachments) != 1 {
		t.Fatalf("second guidance = %#v ok=%v", second, ok)
	}
	if _, ok := app.takeNextChatGuidance(task); ok {
		t.Fatal("empty guidance queue should finish the task")
	}
	if app.GetActiveChatTask() != nil {
		t.Fatal("task remained active after its guidance queue closed")
	}
	accepted, err = app.GuideChatMessage("task-1", "late", "太晚了")
	if err != nil || accepted {
		t.Fatalf("late guidance accepted=%v err=%v", accepted, err)
	}
}

func TestChatGuidanceRejectsWrongTaskAndEmptyMessage(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	app := &App{}
	app.chat.active = &chatTask{id: "task-1", cancel: cancel, acceptingGuidance: true}

	if accepted, err := app.GuideChatMessage("other", "guide", "内容"); err != nil || accepted {
		t.Fatalf("wrong task accepted=%v err=%v", accepted, err)
	}
	if accepted, err := app.GuideChatMessage("task-1", "guide", "   "); err == nil || accepted {
		t.Fatalf("empty guidance accepted=%v err=%v", accepted, err)
	}
}
