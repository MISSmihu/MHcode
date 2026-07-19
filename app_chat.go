package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MISSmihu/MHcode/internal/agent"
	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type chatTaskRunner struct {
	mu     sync.Mutex
	active *chatTask
}

type chatTask struct {
	id                string
	startedAt         string
	cancel            context.CancelFunc
	acceptingGuidance bool
	guidance          []chatGuidance
}

type chatGuidance struct {
	id          string
	prompt      string
	attachments []agent.ChatAttachment
}

type ChatTaskState struct {
	TaskID    string `json:"taskId"`
	StartedAt string `json:"startedAt"`
}

type ChatTaskEvent struct {
	TaskID      string                         `json:"taskId"`
	Type        string                         `json:"type"`
	Delta       string                         `json:"delta,omitempty"`
	Message     string                         `json:"message,omitempty"`
	Model       string                         `json:"model,omitempty"`
	ToolName    string                         `json:"toolName,omitempty"`
	ToolCallID  string                         `json:"toolCallId,omitempty"`
	ToolInput   string                         `json:"toolInput,omitempty"`
	Status      string                         `json:"status,omitempty"`
	Usage       *protocol.TokenUsage           `json:"usage,omitempty"`
	Progress    *tools.ResultPart              `json:"progress,omitempty"`
	Parts       []tools.ResultPart             `json:"parts,omitempty"`
	Compression *agent.ContextCompressionEvent `json:"compression,omitempty"`
	Team        *agent.TeamRoleEvent           `json:"team,omitempty"`
	GuidanceID  string                         `json:"guidanceId,omitempty"`
	Guidance    string                         `json:"guidance,omitempty"`
	Attachments []agent.ChatAttachment         `json:"attachments,omitempty"`
	Result      *agent.ChatResult              `json:"result,omitempty"`
}

func (a *App) StartChatMessage(prompt string) (string, error) {
	return a.startChatMessage(prompt, nil)
}

func (a *App) StartChatMessageWithAttachments(prompt string, attachments []agent.ChatAttachment) (string, error) {
	return a.startChatMessage(prompt, attachments)
}

func (a *App) startChatMessage(prompt string, attachments []agent.ChatAttachment) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" && len(attachments) == 0 {
		return "", errors.New("消息内容不能为空")
	}

	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	task := &chatTask{
		id:                fmt.Sprintf("chat-%d", time.Now().UnixNano()),
		startedAt:         time.Now().Format(time.RFC3339Nano),
		cancel:            cancel,
		acceptingGuidance: true,
	}

	a.chat.mu.Lock()
	if a.chat.active != nil {
		a.chat.mu.Unlock()
		cancel()
		return "", errors.New("已有对话任务正在运行，请先停止当前任务")
	}
	a.chat.active = task
	a.chat.mu.Unlock()

	go a.runChatTask(ctx, task, prompt, attachments)
	return task.id, nil
}

func (a *App) StopChatMessage(taskID string) bool {
	return a.cancelActiveChatTask(strings.TrimSpace(taskID))
}

func (a *App) GuideChatMessage(taskID, guidanceID, prompt string) (bool, error) {
	return a.GuideChatMessageWithAttachments(taskID, guidanceID, prompt, nil)
}

func (a *App) GuideChatMessageWithAttachments(taskID, guidanceID, prompt string, attachments []agent.ChatAttachment) (bool, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" && len(attachments) == 0 {
		return false, errors.New("引导消息内容不能为空")
	}
	guidanceID = strings.TrimSpace(guidanceID)
	if guidanceID == "" {
		guidanceID = fmt.Sprintf("guidance-%d", time.Now().UnixNano())
	}

	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	task := a.chat.active
	if task == nil || task.id != strings.TrimSpace(taskID) || !task.acceptingGuidance {
		return false, nil
	}
	task.guidance = append(task.guidance, chatGuidance{
		id:          guidanceID,
		prompt:      prompt,
		attachments: append([]agent.ChatAttachment(nil), attachments...),
	})
	return true, nil
}

func (a *App) GetActiveChatTask() *ChatTaskState {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	if a.chat.active == nil {
		return nil
	}
	return &ChatTaskState{TaskID: a.chat.active.id, StartedAt: a.chat.active.startedAt}
}

func (a *App) requireIdleChat(action string) error {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	if a.chat.active != nil {
		return fmt.Errorf("对话任务正在运行，停止生成后才能%s", action)
	}
	return nil
}

func (a *App) runChatTask(ctx context.Context, task *chatTask, prompt string, attachments []agent.ChatAttachment) {
	a.emitChatTaskEvent(ChatTaskEvent{TaskID: task.id, Type: "started", Message: "正在准备上下文"})
	emitProgress := func(progress agent.ChatStreamEvent) {
		a.emitChatTaskEvent(ChatTaskEvent{
			TaskID:      task.id,
			Type:        progress.Type,
			Delta:       progress.Delta,
			Message:     progress.Message,
			Model:       progress.Model,
			ToolName:    progress.ToolName,
			ToolCallID:  progress.ToolCallID,
			ToolInput:   progress.ToolInput,
			Status:      progress.Status,
			Usage:       progress.Usage,
			Progress:    progress.Progress,
			Parts:       progress.Parts,
			Compression: progress.Compression,
			Team:        progress.Team,
		})
	}

	currentPrompt := prompt
	currentAttachments := attachments
	guidanceTurn := false
	var result agent.ChatResult
	var err error
	for {
		if guidanceTurn {
			result, err = a.service.SendChatGuidanceWithAttachmentsAndEvents(ctx, currentPrompt, currentAttachments, emitProgress)
		} else {
			result, err = a.service.SendChatMessageWithAttachmentsAndEvents(ctx, currentPrompt, currentAttachments, emitProgress)
		}
		if err != nil || ctx.Err() != nil {
			break
		}

		guidance, ok := a.takeNextChatGuidance(task)
		if !ok {
			task.cancel()
			a.emitChatTaskEvent(ChatTaskEvent{TaskID: task.id, Type: "completed", Model: result.Model, Result: &result})
			return
		}

		completedResult := result
		a.emitChatTaskEvent(ChatTaskEvent{
			TaskID:      task.id,
			Type:        "guidance",
			Message:     "已收到引导，正在调整当前任务",
			GuidanceID:  guidance.id,
			Guidance:    guidance.prompt,
			Attachments: append([]agent.ChatAttachment(nil), guidance.attachments...),
			Result:      &completedResult,
		})
		currentPrompt = guidance.prompt
		currentAttachments = guidance.attachments
		guidanceTurn = true
	}

	a.finishChatTask(task)
	wasCancelled := chatTaskWasCancelled(ctx, err)
	task.cancel()

	if err != nil {
		if wasCancelled {
			a.emitChatTaskEvent(ChatTaskEvent{TaskID: task.id, Type: "cancelled", Message: "已停止生成"})
			return
		}
		a.emitChatTaskEvent(ChatTaskEvent{TaskID: task.id, Type: "failed", Message: err.Error(), Model: result.Model, Result: &result})
		return
	}
}

func (a *App) takeNextChatGuidance(task *chatTask) (chatGuidance, bool) {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	if a.chat.active != task || !task.acceptingGuidance {
		if a.chat.active == task {
			a.chat.active = nil
		}
		return chatGuidance{}, false
	}
	if len(task.guidance) == 0 {
		task.acceptingGuidance = false
		a.chat.active = nil
		return chatGuidance{}, false
	}
	guidance := task.guidance[0]
	task.guidance = task.guidance[1:]
	return guidance, true
}

func (a *App) finishChatTask(task *chatTask) {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	if a.chat.active == task {
		task.acceptingGuidance = false
		a.chat.active = nil
	}
}

func chatTaskWasCancelled(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
}

func (a *App) cancelActiveChatTask(taskID string) bool {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	if a.chat.active == nil || (taskID != "" && a.chat.active.id != taskID) {
		return false
	}
	a.chat.active.acceptingGuidance = false
	a.chat.active.cancel()
	return true
}

func (a *App) emitChatTaskEvent(event ChatTaskEvent) {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "chat:task", event)
	}
}
