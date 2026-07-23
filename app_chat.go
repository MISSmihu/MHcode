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
	mu sync.Mutex
	// active is retained as a compatibility pointer for older callers. The
	// maps are authoritative once the application starts normally.
	active         *chatTask
	tasks          map[string]*chatTask
	bySession      map[string]string
	meta           map[string]chatTaskMeta
	approvalOwners map[string]*chatTask
}

type chatTaskMeta struct {
	projectID string
	sessionID string
}

type chatTask struct {
	id                string
	projectID         string
	sessionID         string
	service           *agent.Service
	startedAt         string
	cancel            context.CancelFunc
	acceptingGuidance bool
	guidance          []chatGuidance
	done              chan struct{}
	doneOnce          sync.Once
	terminalOnce      sync.Once
}

const chatTaskStopGracePeriod = 2 * time.Second

type chatGuidance struct {
	id          string
	prompt      string
	attachments []agent.ChatAttachment
}

type ChatTaskState struct {
	TaskID    string `json:"taskId"`
	StartedAt string `json:"startedAt"`
	ProjectID string `json:"projectId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

type ChatTaskEvent struct {
	TaskID      string                         `json:"taskId"`
	ProjectID   string                         `json:"projectId,omitempty"`
	SessionID   string                         `json:"sessionId,omitempty"`
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
	Forced      bool                           `json:"forced,omitempty"`
}

func (a *App) StartChatMessage(prompt string) (string, error) {
	return a.startChatMessageForProjectSession("", "", prompt, nil)
}

func (a *App) StartChatMessageWithAttachments(prompt string, attachments []agent.ChatAttachment) (string, error) {
	return a.startChatMessageForProjectSession("", "", prompt, attachments)
}

func (a *App) StartChatMessageForSession(sessionID, prompt string) (string, error) {
	return a.startChatMessageForProjectSession("", sessionID, prompt, nil)
}

func (a *App) StartChatMessageForSessionWithAttachments(sessionID, prompt string, attachments []agent.ChatAttachment) (string, error) {
	return a.startChatMessageForProjectSession("", sessionID, prompt, attachments)
}

func (a *App) StartChatMessageForProjectSession(projectID, sessionID, prompt string) (string, error) {
	return a.startChatMessageForProjectSession(projectID, sessionID, prompt, nil)
}

func (a *App) StartChatMessageForProjectSessionWithAttachments(projectID, sessionID, prompt string, attachments []agent.ChatAttachment) (string, error) {
	return a.startChatMessageForProjectSession(projectID, sessionID, prompt, attachments)
}

func (a *App) startChatMessageForProjectSession(projectID, sessionID, prompt string, attachments []agent.ChatAttachment) (string, error) {
	return a.startChatMessageForProjectSessionRoute(projectID, sessionID, prompt, attachments, "", "")
}

func (a *App) startChatMessageForProjectSessionRoute(projectID, sessionID, prompt string, attachments []agent.ChatAttachment, providerID, modelID string) (string, error) {
	return a.startChatMessageForProjectSessionRouteRegistered(projectID, sessionID, prompt, attachments, providerID, modelID, nil)
}

func (a *App) startChatMessageForProjectSessionRouteRegistered(projectID, sessionID, prompt string, attachments []agent.ChatAttachment, providerID, modelID string, registered func(string)) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" && len(attachments) == 0 {
		return "", errors.New("消息内容不能为空")
	}

	if a.service == nil {
		return "", errors.New("Agent 服务尚未初始化")
	}
	activeProjectID, activeSessionID := a.service.ActiveSessionIDs()
	if strings.TrimSpace(sessionID) == "" {
		sessionID = activeSessionID
	}
	if strings.TrimSpace(projectID) == "" && sessionID == activeSessionID {
		projectID = activeProjectID
	}
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("当前没有可用会话")
	}
	runtime, runtimeErr := a.service.NewProjectSessionRuntime(projectID, sessionID)
	if runtimeErr != nil {
		return "", runtimeErr
	}
	if err := runtime.SetEphemeralModelRoute(providerID, modelID); err != nil {
		return "", err
	}
	// The requested session is authoritative. This matters when the user
	// switches to a session owned by another project while another task runs.
	if runtimeProjectID, runtimeSessionID := runtime.ActiveSessionIDs(); runtimeProjectID != "" {
		projectID = runtimeProjectID
		if runtimeSessionID != "" {
			sessionID = runtimeSessionID
		}
	}

	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	task := &chatTask{
		id:                fmt.Sprintf("chat-%d", time.Now().UnixNano()),
		projectID:         projectID,
		sessionID:         sessionID,
		service:           runtime,
		startedAt:         time.Now().Format(time.RFC3339Nano),
		cancel:            cancel,
		acceptingGuidance: true,
		done:              make(chan struct{}),
	}

	a.chat.mu.Lock()
	if a.chat.tasks == nil {
		a.chat.tasks = make(map[string]*chatTask)
	}
	if a.chat.bySession == nil {
		a.chat.bySession = make(map[string]string)
	}
	if a.chat.meta == nil {
		a.chat.meta = make(map[string]chatTaskMeta)
	}
	if a.chat.approvalOwners == nil {
		a.chat.approvalOwners = make(map[string]*chatTask)
	}
	sessionKey := chatSessionKey(projectID, sessionID)
	if a.chat.bySession[sessionKey] != "" {
		a.chat.mu.Unlock()
		cancel()
		return "", errors.New("当前会话已有任务正在运行，请切换会话或将消息加入队列")
	}
	a.chat.tasks[task.id] = task
	a.chat.bySession[sessionKey] = task.id
	a.chat.active = task
	a.chat.mu.Unlock()
	runtime.SetApprovalNotify(func(request agent.ApprovalRequest) {
		a.chat.mu.Lock()
		if a.chat.approvalOwners == nil {
			a.chat.approvalOwners = make(map[string]*chatTask)
		}
		a.chat.approvalOwners[request.ID] = task
		a.chat.mu.Unlock()
		if a.ctx != nil {
			wruntime.EventsEmit(a.ctx, "approval:request", request)
		}
	})
	if registered != nil {
		registered(task.id)
	}

	go a.runChatTask(ctx, task, prompt, attachments)
	return task.id, nil
}

func (a *App) StopChatMessage(taskID string) bool {
	task := a.cancelActiveChatTask(strings.TrimSpace(taskID))
	if task == nil {
		return false
	}
	go a.enforceChatTaskStop(task, chatTaskStopGracePeriod)
	return true
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
	task := a.chat.tasks[strings.TrimSpace(taskID)]
	if task == nil && a.chat.active != nil && a.chat.active.id == strings.TrimSpace(taskID) {
		task = a.chat.active
	}
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
	task := a.chat.active
	if task == nil {
		for _, candidate := range a.chat.tasks {
			task = candidate
			break
		}
	}
	if task == nil {
		return nil
	}
	return &ChatTaskState{TaskID: task.id, StartedAt: task.startedAt, ProjectID: task.projectID, SessionID: task.sessionID}
}

func (a *App) GetActiveChatTasks() []*ChatTaskState {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	states := make([]*ChatTaskState, 0, len(a.chat.tasks))
	for _, task := range a.chat.tasks {
		states = append(states, &ChatTaskState{TaskID: task.id, StartedAt: task.startedAt, ProjectID: task.projectID, SessionID: task.sessionID})
	}
	if len(states) == 0 && a.chat.active != nil {
		states = append(states, &ChatTaskState{TaskID: a.chat.active.id, StartedAt: a.chat.active.startedAt, ProjectID: a.chat.active.projectID, SessionID: a.chat.active.sessionID})
	}
	return states
}

func (a *App) requireIdleChat(action string) error {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	if len(a.chat.tasks) > 0 || a.chat.active != nil {
		return fmt.Errorf("对话任务正在运行，停止生成后才能%s", action)
	}
	return nil
}

func (a *App) requireSessionIdleChat(sessionID string) error {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	sessionID = strings.TrimSpace(sessionID)
	for _, task := range a.chat.tasks {
		if task != nil && task.sessionID == sessionID {
			return fmt.Errorf("该会话已有对话任务正在运行")
		}
	}
	return nil
}

func (a *App) requireProjectSessionIdleChat(projectID, sessionID string) error {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)
	if taskID := a.chat.bySession[chatSessionKey(projectID, sessionID)]; taskID != "" {
		return fmt.Errorf("该会话已有对话任务正在运行")
	}
	for _, task := range a.chat.tasks {
		if task != nil && task.projectID == projectID && task.sessionID == sessionID {
			return fmt.Errorf("该会话已有对话任务正在运行")
		}
	}
	return nil
}

func chatSessionKey(projectID, sessionID string) string {
	return strings.TrimSpace(projectID) + "\x00" + strings.TrimSpace(sessionID)
}

func (a *App) runChatTask(ctx context.Context, task *chatTask, prompt string, attachments []agent.ChatAttachment) {
	defer task.markDone()
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
			result, err = task.service.SendChatGuidanceWithAttachmentsAndEvents(ctx, currentPrompt, currentAttachments, emitProgress)
		} else {
			result, err = task.service.SendChatMessageWithAttachmentsAndEvents(ctx, currentPrompt, currentAttachments, emitProgress)
		}
		if err != nil || ctx.Err() != nil {
			break
		}

		guidance, ok := a.takeNextChatGuidance(task)
		if !ok {
			a.completeChatTask(task, ChatTaskEvent{TaskID: task.id, Type: "completed", Model: result.Model, Result: &result})
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

	wasCancelled := chatTaskWasCancelled(ctx, err)

	if err != nil {
		if wasCancelled {
			a.completeChatTask(task, ChatTaskEvent{
				TaskID: task.id, Type: "cancelled", Message: "已停止生成",
				Model: result.Model, Result: &result,
			})
			return
		}
		a.completeChatTask(task, ChatTaskEvent{TaskID: task.id, Type: "failed", Message: err.Error(), Model: result.Model, Result: &result})
		return
	}
}

func (task *chatTask) markDone() {
	if task == nil || task.done == nil {
		return
	}
	task.doneOnce.Do(func() { close(task.done) })
}

func (a *App) completeChatTask(task *chatTask, event ChatTaskEvent) {
	if task == nil {
		return
	}
	task.terminalOnce.Do(func() {
		a.finishChatTask(task)
		if task.cancel != nil {
			task.cancel()
		}
		a.reloadCompletedChatTask(task)
		a.emitChatTaskEvent(event)
	})
}

func (a *App) enforceChatTaskStop(task *chatTask, grace time.Duration) {
	if task == nil || task.done == nil {
		return
	}
	if grace <= 0 {
		grace = chatTaskStopGracePeriod
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-task.done:
		return
	case <-timer.C:
		a.completeChatTask(task, ChatTaskEvent{
			TaskID:  task.id,
			Type:    "cancelled",
			Message: "任务未在取消期限内退出，已强制结束并转入后台清理。",
			Forced:  true,
		})
	}
}

func (a *App) reloadCompletedChatTask(task *chatTask) {
	if a.service == nil || task == nil {
		return
	}
	_, _, _ = a.service.ReloadProjectSessionIfActive(task.projectID, task.sessionID)
}

func (a *App) takeNextChatGuidance(task *chatTask) (chatGuidance, bool) {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	tracked := a.chat.tasks[task.id]
	if tracked == nil && a.chat.active == task {
		tracked = task
	}
	if tracked != task || !task.acceptingGuidance {
		removeChatTaskLocked(&a.chat, task)
		return chatGuidance{}, false
	}
	if len(task.guidance) == 0 {
		task.acceptingGuidance = false
		removeChatTaskLocked(&a.chat, task)
		return chatGuidance{}, false
	}
	guidance := task.guidance[0]
	task.guidance = task.guidance[1:]
	return guidance, true
}

func (a *App) finishChatTask(task *chatTask) {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	task.acceptingGuidance = false
	removeChatTaskLocked(&a.chat, task)
}

func chatTaskWasCancelled(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
}

func (a *App) cancelActiveChatTask(taskID string) *chatTask {
	a.chat.mu.Lock()
	task := a.chat.tasks[strings.TrimSpace(taskID)]
	if task == nil && a.chat.active != nil && (taskID == "" || a.chat.active.id == taskID) {
		task = a.chat.active
	}
	if task == nil || (taskID != "" && task.id != taskID) {
		a.chat.mu.Unlock()
		return nil
	}
	task.acceptingGuidance = false
	a.chat.mu.Unlock()
	if task.cancel != nil {
		task.cancel()
	}
	return task
}

func (a *App) cancelAllChatTasks() {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	seen := make(map[string]bool)
	for _, task := range a.chat.tasks {
		if task == nil || seen[task.id] {
			continue
		}
		seen[task.id] = true
		task.acceptingGuidance = false
		task.cancel()
	}
	if a.chat.active != nil && !seen[a.chat.active.id] {
		a.chat.active.acceptingGuidance = false
		a.chat.active.cancel()
	}
}

func removeChatTaskLocked(runner *chatTaskRunner, task *chatTask) {
	if task == nil {
		return
	}
	if runner.meta == nil {
		runner.meta = make(map[string]chatTaskMeta)
	}
	runner.meta[task.id] = chatTaskMeta{projectID: task.projectID, sessionID: task.sessionID}
	if runner.tasks != nil {
		delete(runner.tasks, task.id)
	}
	key := chatSessionKey(task.projectID, task.sessionID)
	if runner.bySession != nil && runner.bySession[key] == task.id {
		delete(runner.bySession, key)
	}
	for requestID, owner := range runner.approvalOwners {
		if owner == task {
			delete(runner.approvalOwners, requestID)
		}
	}
	if runner.active == task {
		runner.active = nil
		for _, candidate := range runner.tasks {
			runner.active = candidate
			break
		}
	}
}

func (a *App) approvalService(requestID string) *agent.Service {
	a.chat.mu.Lock()
	defer a.chat.mu.Unlock()
	task := a.chat.approvalOwners[strings.TrimSpace(requestID)]
	if task == nil {
		return nil
	}
	delete(a.chat.approvalOwners, strings.TrimSpace(requestID))
	return task.service
}

func (a *App) emitChatTaskEvent(event ChatTaskEvent) {
	if event.TaskID != "" {
		a.chat.mu.Lock()
		if task := a.chat.tasks[event.TaskID]; task != nil {
			if event.ProjectID == "" {
				event.ProjectID = task.projectID
			}
			if event.SessionID == "" {
				event.SessionID = task.sessionID
			}
		} else if meta, ok := a.chat.meta[event.TaskID]; ok {
			if event.ProjectID == "" {
				event.ProjectID = meta.projectID
			}
			if event.SessionID == "" {
				event.SessionID = meta.sessionID
			}
		}
		a.chat.mu.Unlock()
	}
	if a.automations != nil {
		switch event.Type {
		case "completed":
			a.automations.CompleteChatTask(event.TaskID, "completed", "Agent 已完成自动化任务")
		case "failed":
			a.automations.CompleteChatTask(event.TaskID, "failed", event.Message)
		case "cancelled":
			a.automations.CompleteChatTask(event.TaskID, "cancelled", event.Message)
		}
	}
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "chat:task", event)
	}
}
