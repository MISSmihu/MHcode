package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/MISSmihu/MHcode/internal/eventlog"
	"github.com/MISSmihu/MHcode/internal/project"
	"github.com/MISSmihu/MHcode/internal/tools"
)

const taskRuntimeStreamPersistInterval = 750 * time.Millisecond

// TaskRuntimeState is a bounded, branch-anchored crash recovery view. The
// canonical completed conversation remains events.jsonl; this state only
// preserves work that had not reached a terminal event when MHcode exited.
type TaskRuntimeState struct {
	TaskID       string             `json:"taskId"`
	AnchorID     string             `json:"anchorId,omitempty"`
	StartedAt    string             `json:"startedAt"`
	UpdatedAt    string             `json:"updatedAt"`
	Status       string             `json:"status"`
	Message      string             `json:"message,omitempty"`
	Model        string             `json:"model,omitempty"`
	Content      string             `json:"content,omitempty"`
	Reasoning    string             `json:"reasoning,omitempty"`
	DurationMs   int64              `json:"durationMs,omitempty"`
	Parts        []tools.ResultPart `json:"parts,omitempty"`
	LastEvent    string             `json:"lastEvent,omitempty"`
	TurnSequence int                `json:"turnSequence,omitempty"`
}

func (s *Service) TaskRuntimeSnapshot() (TaskRuntimeState, bool) {
	if s == nil {
		return TaskRuntimeState{}, false
	}
	s.taskRuntimeMu.Lock()
	defer s.taskRuntimeMu.Unlock()
	if strings.TrimSpace(s.taskRuntime.TaskID) == "" {
		return TaskRuntimeState{}, false
	}
	// The event representation provides both a deep copy and the same secret
	// redaction used by the on-disk snapshot.
	return taskRuntimeStateFromEvent(taskRuntimeEventRecord(s.taskRuntime)), true
}

func (state TaskRuntimeState) Terminal() bool {
	switch strings.TrimSpace(state.Status) {
	case "completed", "failed", "cancelled", "interrupted":
		return true
	default:
		return false
	}
}

func (s *Service) StartTaskRuntime(taskID, startedAt string) error {
	if s == nil || s.eventStore == nil {
		return nil
	}
	s.taskRuntimeMu.Lock()
	defer s.taskRuntimeMu.Unlock()
	startedAt = strings.TrimSpace(startedAt)
	if startedAt == "" {
		startedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	s.taskRuntime = TaskRuntimeState{
		TaskID:       strings.TrimSpace(taskID),
		AnchorID:     s.eventStore.Head(),
		StartedAt:    startedAt,
		UpdatedAt:    startedAt,
		Status:       "running",
		Message:      "正在准备上下文",
		LastEvent:    "started",
		TurnSequence: 1,
	}
	s.taskRuntimeLastWrite = time.Time{}
	s.taskRuntimeLastHash = ""
	return s.persistTaskRuntimeLocked(true)
}

// RecordTaskStreamEvent folds provider-independent events into one durable
// snapshot. Token deltas are throttled; tool terminal states and phase changes
// are flushed immediately.
func (s *Service) RecordTaskStreamEvent(taskID string, event ChatStreamEvent) error {
	if s == nil || s.eventStore == nil {
		return nil
	}
	s.taskRuntimeMu.Lock()
	defer s.taskRuntimeMu.Unlock()
	if s.taskRuntime.TaskID == "" {
		s.taskRuntime = TaskRuntimeState{
			TaskID: strings.TrimSpace(taskID), AnchorID: s.eventStore.Head(),
			StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Status: "running", TurnSequence: 1,
		}
	}
	if s.taskRuntime.TaskID != strings.TrimSpace(taskID) || s.taskRuntime.Terminal() {
		return nil
	}
	applyTaskStreamEvent(&s.taskRuntime, event)
	force := taskRuntimeEventRequiresFlush(event)
	return s.persistTaskRuntimeLocked(force)
}

func (s *Service) StartGuidedTaskRuntime(taskID, message, model string) error {
	if s == nil || s.eventStore == nil {
		return nil
	}
	s.taskRuntimeMu.Lock()
	defer s.taskRuntimeMu.Unlock()
	if s.taskRuntime.TaskID != strings.TrimSpace(taskID) {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sequence := s.taskRuntime.TurnSequence + 1
	s.taskRuntime = TaskRuntimeState{
		TaskID:       strings.TrimSpace(taskID),
		AnchorID:     s.eventStore.Head(),
		StartedAt:    now,
		UpdatedAt:    now,
		Status:       "running",
		Message:      redactSensitiveText(message),
		Model:        strings.TrimSpace(model),
		LastEvent:    "guidance",
		TurnSequence: sequence,
	}
	s.taskRuntimeLastWrite = time.Time{}
	s.taskRuntimeLastHash = ""
	return s.persistTaskRuntimeLocked(true)
}

func (s *Service) FinishTaskRuntime(taskID, status, message string, result ChatResult) error {
	if s == nil || s.eventStore == nil {
		return nil
	}
	s.taskRuntimeMu.Lock()
	defer s.taskRuntimeMu.Unlock()
	if s.taskRuntime.TaskID == "" {
		s.taskRuntime = TaskRuntimeState{
			TaskID: strings.TrimSpace(taskID), AnchorID: s.eventStore.Head(),
			StartedAt: time.Now().UTC().Format(time.RFC3339Nano), TurnSequence: 1,
		}
	}
	if s.taskRuntime.TaskID != strings.TrimSpace(taskID) || s.taskRuntime.Terminal() {
		return nil
	}
	s.taskRuntime.Status = normalizeTaskRuntimeStatus(status)
	s.taskRuntime.LastEvent = s.taskRuntime.Status
	s.taskRuntime.Message = redactSensitiveText(message)
	if strings.TrimSpace(result.Model) != "" {
		s.taskRuntime.Model = result.Model
	}
	if result.Content != "" {
		s.taskRuntime.Content = redactSensitiveText(result.Content)
	}
	if result.Reasoning != "" {
		s.taskRuntime.Reasoning = redactSensitiveText(result.Reasoning)
	}
	if len(result.Parts) > 0 {
		s.taskRuntime.Parts = redactTaskRuntimeParts(result.Parts)
	}
	if result.DurationMs > 0 {
		s.taskRuntime.DurationMs = result.DurationMs
	}
	settleTaskRuntimeParts(&s.taskRuntime, s.taskRuntime.Status)
	return s.persistTaskRuntimeLocked(true)
}

func applyTaskStreamEvent(state *TaskRuntimeState, event ChatStreamEvent) {
	if state == nil {
		return
	}
	state.LastEvent = strings.TrimSpace(event.Type)
	if message := strings.TrimSpace(event.Message); message != "" {
		state.Message = redactSensitiveText(message)
	}
	if model := strings.TrimSpace(event.Model); model != "" {
		state.Model = model
	}
	if status := taskRuntimeStatusFromStreamEvent(event); status != "" {
		state.Status = status
	}
	switch event.Type {
	case "status", "context_compression":
		appendTaskRuntimeTimelineNote(state, event)
	case "delta":
		state.Content += event.Delta
	case "reasoning":
		state.Reasoning += event.Delta
	case "tool":
		upsertTaskRuntimeToolPart(state, event)
		state.Parts = mergeTaskRuntimeParts(state.Parts, redactTaskRuntimeParts(event.Parts))
	case "progress":
		if event.Progress != nil {
			state.Parts = mergeTaskRuntimeParts(state.Parts, redactTaskRuntimeParts([]tools.ResultPart{*event.Progress}))
		}
	case "team":
		if event.Team != nil {
			state.Parts = mergeTaskRuntimeParts(state.Parts, []tools.ResultPart{taskRuntimeTeamPart(*event.Team)})
		}
	case "subagent", "provider_notice":
		state.Parts = mergeTaskRuntimeParts(state.Parts, redactTaskRuntimeParts(event.Parts))
	}
}

func appendTaskRuntimeTimelineNote(state *TaskRuntimeState, event ChatStreamEvent) {
	if state == nil {
		return
	}
	message := strings.TrimSpace(event.Message)
	if message == "" {
		return
	}
	status := strings.TrimSpace(event.Status)
	if event.Type == "context_compression" && event.Compression != nil {
		status = event.Compression.Status
	}
	if status == "" {
		status = "running"
	}
	for index := len(state.Parts) - 1; index >= 0; index-- {
		part := state.Parts[index]
		if part.Kind != tools.PartTimelineNote {
			continue
		}
		if part.Message == message && part.Status == status {
			return
		}
		break
	}
	state.Parts = append(state.Parts, tools.ResultPart{
		Kind: tools.PartTimelineNote, Message: redactSensitiveText(message), Status: status,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func taskRuntimeStatusFromStreamEvent(event ChatStreamEvent) string {
	switch event.Type {
	case "status", "started", "heartbeat":
		if status := strings.TrimSpace(event.Status); status != "" {
			return status
		}
		return "running"
	case "tool":
		switch event.Status {
		case "waiting", "retrying":
			return event.Status
		default:
			return "running"
		}
	case "delta", "reasoning", "provider_notice", "context_compression", "progress", "team", "subagent":
		return "running"
	default:
		return ""
	}
}

func taskRuntimeEventRequiresFlush(event ChatStreamEvent) bool {
	switch event.Type {
	case "status", "context_compression", "provider_notice", "progress", "team":
		return true
	case "tool":
		return event.Status != "running" || len(event.Parts) == 0
	default:
		return false
	}
}

func (s *Service) persistTaskRuntimeLocked(force bool) error {
	if s.eventStore == nil || strings.TrimSpace(s.taskRuntime.TaskID) == "" {
		return nil
	}
	now := time.Now().UTC()
	if !force && !s.taskRuntimeLastWrite.IsZero() && now.Sub(s.taskRuntimeLastWrite) < taskRuntimeStreamPersistInterval {
		return nil
	}
	s.taskRuntime.UpdatedAt = now.Format(time.RFC3339Nano)
	if started, err := time.Parse(time.RFC3339Nano, s.taskRuntime.StartedAt); err == nil {
		elapsed := max(now.Sub(started).Milliseconds(), 1)
		if !s.taskRuntime.Terminal() || s.taskRuntime.DurationMs <= 0 {
			s.taskRuntime.DurationMs = elapsed
		}
	}
	record := taskRuntimeEventRecord(s.taskRuntime)
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(encoded)
	hash := hex.EncodeToString(sum[:])
	if hash == s.taskRuntimeLastHash {
		return nil
	}
	if err := s.eventStore.WriteTaskRuntime(record); err != nil {
		return fmt.Errorf("保存任务运行快照失败: %w", err)
	}
	s.taskRuntimeLastWrite = now
	s.taskRuntimeLastHash = hash
	return nil
}

func taskRuntimeEventRecord(state TaskRuntimeState) eventlog.TaskRuntimeRecord {
	return eventlog.TaskRuntimeRecord{
		Version: 1, TaskID: state.TaskID, AnchorID: state.AnchorID,
		StartedAt: state.StartedAt, UpdatedAt: state.UpdatedAt, Status: state.Status,
		Message: redactSensitiveText(state.Message), Model: state.Model, Content: redactSensitiveText(state.Content),
		Reasoning: redactSensitiveText(state.Reasoning), DurationMs: state.DurationMs,
		Parts: toEventParts(state.Parts), LastEvent: state.LastEvent, TurnSequence: state.TurnSequence,
	}
}

func taskRuntimeStateFromEvent(record eventlog.TaskRuntimeRecord) TaskRuntimeState {
	return TaskRuntimeState{
		TaskID: record.TaskID, AnchorID: record.AnchorID, StartedAt: record.StartedAt,
		UpdatedAt: record.UpdatedAt, Status: record.Status, Message: record.Message,
		Model: record.Model, Content: record.Content, Reasoning: record.Reasoning,
		DurationMs: record.DurationMs, Parts: fromEventParts(record.Parts),
		LastEvent: record.LastEvent, TurnSequence: record.TurnSequence,
	}
}

func upsertTaskRuntimeToolPart(state *TaskRuntimeState, event ChatStreamEvent) {
	name := strings.TrimSpace(event.ToolName)
	if state == nil || name == "" {
		return
	}
	id := strings.TrimSpace(event.ToolCallID)
	if id == "" {
		id = "live-" + name
	}
	status := "ok"
	switch event.Status {
	case "running", "waiting", "retrying":
		status = event.Status
	case "error", "failed":
		status = "error"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	part := tools.ResultPart{
		Kind: tools.PartToolCall, Name: name, Status: status,
		Input: redactSensitiveText(event.ToolInput), ToolCallID: id,
	}
	if status == "running" || status == "waiting" || status == "retrying" {
		part.StartedAt = now
	} else {
		part.Output = redactSensitiveText(event.Message)
		part.CompletedAt = now
	}
	for index := range state.Parts {
		current := state.Parts[index]
		if current.Kind != tools.PartToolCall || current.ToolCallID != id {
			continue
		}
		if current.StartedAt != "" {
			part.StartedAt = current.StartedAt
		}
		if part.Input == "" {
			part.Input = current.Input
		}
		if part.Output == "" {
			part.Output = current.Output
		}
		state.Parts[index] = mergeTaskRuntimeToolPart(current, part)
		return
	}
	state.Parts = append(state.Parts, part)
}

func mergeTaskRuntimeToolPart(current, incoming tools.ResultPart) tools.ResultPart {
	if taskRuntimePartTerminal(current.Status) && !taskRuntimePartTerminal(incoming.Status) {
		return current
	}
	if incoming.Name == "" {
		incoming.Name = current.Name
	}
	if incoming.ToolCallID == "" {
		incoming.ToolCallID = current.ToolCallID
	}
	if incoming.Input == "" {
		incoming.Input = current.Input
	}
	if incoming.Output == "" {
		incoming.Output = current.Output
	}
	if incoming.Stdout == "" {
		incoming.Stdout = current.Stdout
	}
	if incoming.Stderr == "" {
		incoming.Stderr = current.Stderr
	}
	if incoming.WorkingDirectory == "" {
		incoming.WorkingDirectory = current.WorkingDirectory
	}
	if incoming.ExitCode == nil {
		incoming.ExitCode = current.ExitCode
	}
	if incoming.StartedAt == "" {
		incoming.StartedAt = current.StartedAt
	}
	if incoming.CompletedAt == "" {
		incoming.CompletedAt = current.CompletedAt
	}
	if incoming.DurationMs == 0 {
		incoming.DurationMs = current.DurationMs
	}
	return incoming
}

func mergeTaskRuntimeParts(existing, incoming []tools.ResultPart) []tools.ResultPart {
	for _, part := range incoming {
		replaced := false
		switch part.Kind {
		case tools.PartToolCall:
			for index := range existing {
				if existing[index].Kind != tools.PartToolCall {
					continue
				}
				if part.ToolCallID != "" && existing[index].ToolCallID == part.ToolCallID ||
					part.ToolCallID == "" && existing[index].Name == part.Name && existing[index].Input == part.Input {
					existing[index] = mergeTaskRuntimeToolPart(existing[index], part)
					replaced = true
					break
				}
			}
		case tools.PartProgress:
			for index := range existing {
				if existing[index].Kind == tools.PartProgress {
					existing[index] = part
					replaced = true
					break
				}
			}
		case tools.PartTeamRole:
			for index := range existing {
				if existing[index].Kind == tools.PartTeamRole && existing[index].Role == part.Role && existing[index].Attempt == part.Attempt {
					existing[index] = part
					replaced = true
					break
				}
			}
		case tools.PartSubagent:
			for index := range existing {
				if existing[index].Kind == tools.PartSubagent && existing[index].TaskID == part.TaskID {
					existing[index] = mergeSubagentParts(existing[index], part)
					replaced = true
					break
				}
			}
		case tools.PartWebSearch:
			for index := range existing {
				if existing[index].Kind == tools.PartWebSearch {
					existing[index] = mergeWebSearchParts(existing[index], part)
					replaced = true
					break
				}
			}
		case tools.PartProviderNotice:
			identity := providerResultPartIdentity(part)
			for _, current := range existing {
				if current.Kind == tools.PartProviderNotice && providerResultPartIdentity(current) == identity {
					replaced = true
					break
				}
			}
		}
		if !replaced {
			existing = append(existing, part)
		}
	}
	return existing
}

func taskRuntimeTeamPart(event TeamRoleEvent) tools.ResultPart {
	return tools.ResultPart{
		Kind: tools.PartTeamRole, Role: event.Role, RoleLabel: event.Label,
		ProviderID: event.ProviderID, Model: event.Model, Status: event.Status,
		Summary: firstNonEmpty(event.Error, event.Summary), Verdict: event.Verdict, Attempt: event.Attempt,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func taskRuntimePartTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case "ok", "completed", "error", "failed", "cancelled", "interrupted":
		return true
	default:
		return false
	}
}

func normalizeTaskRuntimeStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "cancelled", "interrupted":
		return strings.TrimSpace(status)
	default:
		return "failed"
	}
}

func settleTaskRuntimeParts(state *TaskRuntimeState, status string) {
	if state == nil {
		return
	}
	for index := range state.Parts {
		part := &state.Parts[index]
		switch part.Kind {
		case tools.PartToolCall:
			if !taskRuntimePartTerminal(part.Status) {
				part.Status = "error"
				if part.Output == "" {
					part.Output = taskRuntimeTerminalMessage(status)
				}
				part.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
			}
		case tools.PartProgress:
			if part.TaskStatus == "" || part.TaskStatus == "running" {
				part.TaskStatus = status
			}
		case tools.PartSubagent, tools.PartTeamRole:
			if !taskRuntimePartTerminal(part.Status) {
				part.Status = status
			}
		case tools.PartTimelineNote:
			if !taskRuntimePartTerminal(part.Status) {
				part.Status = status
				part.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
			}
		}
	}
}

func taskRuntimeTerminalMessage(status string) string {
	switch status {
	case "cancelled":
		return "任务已停止"
	case "interrupted":
		return "应用退出时该操作仍在运行"
	case "failed":
		return "任务失败前该操作未完成"
	default:
		return "操作已结束"
	}
}

func redactTaskRuntimeParts(parts []tools.ResultPart) []tools.ResultPart {
	if len(parts) == 0 {
		return nil
	}
	encoded, err := json.Marshal(parts)
	if err != nil {
		return nil
	}
	var cloned []tools.ResultPart
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil
	}
	for index := range cloned {
		part := &cloned[index]
		part.Text = redactSensitiveText(part.Text)
		part.Patch = redactSensitiveText(part.Patch)
		part.Input = redactSensitiveText(part.Input)
		part.Output = redactSensitiveText(part.Output)
		part.Stdout = redactSensitiveText(part.Stdout)
		part.Stderr = redactSensitiveText(part.Stderr)
		part.Summary = redactSensitiveText(part.Summary)
		part.CurrentAction = redactSensitiveText(part.CurrentAction)
		part.SubagentOutput = redactSensitiveText(part.SubagentOutput)
		part.SubagentReasoning = redactSensitiveText(part.SubagentReasoning)
		part.Message = redactSensitiveText(part.Message)
		for activityIndex := range part.Activities {
			part.Activities[activityIndex].Input = redactSensitiveText(part.Activities[activityIndex].Input)
			part.Activities[activityIndex].Output = redactSensitiveText(part.Activities[activityIndex].Output)
		}
		for sourceIndex := range part.Sources {
			part.Sources[sourceIndex].Snippet = redactSensitiveText(part.Sources[sourceIndex].Snippet)
		}
	}
	return cloned
}

// RecoverInterruptedTaskRuntimes converts stale non-terminal snapshots into a
// durable interrupted turn. It is safe to call repeatedly: the runtime file is
// marked terminal after the first recovery.
func (s *Service) RecoverInterruptedTaskRuntimes() (int, error) {
	if s == nil || s.projects == nil || strings.TrimSpace(s.config.SessionsDir) == "" {
		return 0, nil
	}
	manifest := s.projects.Snapshot()
	projects := append(append([]projectRuntimeTarget(nil), projectRuntimeTargets(manifest.Projects)...), projectRuntimeTargets(manifest.DetachedProjects)...)
	recovered := 0
	var recoveryErr error
	for _, target := range projects {
		store := s.eventStore
		if store == nil || target.projectID != s.projectID || target.sessionID != s.sessionID {
			var err error
			store, err = eventlog.Open(filepath.Join(s.config.SessionsDir, target.projectID, target.sessionID))
			if err != nil {
				recoveryErr = errors.Join(recoveryErr, err)
				continue
			}
		}
		changed, err := recoverInterruptedTaskRuntime(store)
		if err != nil {
			recoveryErr = errors.Join(recoveryErr, err)
			continue
		}
		if changed {
			recovered++
			_ = s.projects.TouchSession(target.projectID, target.sessionID)
		}
	}
	return recovered, recoveryErr
}

type projectRuntimeTarget struct {
	projectID string
	sessionID string
}

func projectRuntimeTargets(projects []project.Project) []projectRuntimeTarget {
	targets := make([]projectRuntimeTarget, 0)
	for _, item := range projects {
		for _, session := range item.Sessions {
			targets = append(targets, projectRuntimeTarget{projectID: item.ID, sessionID: session.ID})
		}
	}
	return targets
}

func recoverInterruptedTaskRuntime(store *eventlog.Store) (bool, error) {
	record, ok, err := store.ReadTaskRuntime()
	if err != nil || !ok || record.Terminal() {
		return false, err
	}
	if record.AnchorID != "" && !store.IsOnCurrentChain(record.AnchorID) {
		return false, nil
	}
	events := store.Events()
	updatedAt, _ := time.Parse(time.RFC3339Nano, record.UpdatedAt)
	if status, completed := taskRuntimeCompletionAfter(events, updatedAt); completed {
		record.Status = status
		record.LastEvent = status
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return true, store.WriteTaskRuntime(record)
	}

	state := taskRuntimeStateFromEvent(record)
	state.Status = "interrupted"
	state.LastEvent = "interrupted"
	state.Message = "上次任务因应用退出而中断"
	settleTaskRuntimeParts(&state, "interrupted")
	if latestPlan, ok := runningPlanAfterAnchor(events, record.AnchorID); ok {
		latestPlan.PlanStatus = "interrupted"
		if _, err := store.Append(latestPlan, eventlog.EventPlanUpdate); err != nil {
			return false, err
		}
	}
	if hasUserMessageAfterAnchor(events, record.AnchorID) {
		content := strings.TrimSpace(state.Content)
		if content == "" {
			content = "任务因应用意外退出而中断，已保留执行记录。发送“继续”可从当前进度恢复。"
		}
		terminalParts := appendTextPartIfMissing(state.Parts, content)
		if _, err := store.Append(eventlog.EventPayload{
			Role: "assistant", Content: content, Model: state.Model,
			DurationMs: state.DurationMs, Parts: toEventParts(terminalParts), Status: "interrupted",
		}, eventlog.EventTurnTerminal); err != nil {
			return false, err
		}
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return true, store.WriteTaskRuntime(taskRuntimeEventRecord(state))
}

func taskRuntimeCompletionAfter(events []eventlog.Event, updatedAt time.Time) (string, bool) {
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if !updatedAt.IsZero() && !event.TS.After(updatedAt) {
			break
		}
		switch event.Type {
		case eventlog.EventTurnTerminal:
			return normalizeTaskRuntimeStatus(event.Payload.Status), true
		case eventlog.EventCheckpoint, eventlog.EventAssistantMessage:
			return "completed", true
		}
	}
	return "", false
}

func hasUserMessageAfterAnchor(events []eventlog.Event, anchorID string) bool {
	afterAnchor := anchorID == ""
	for _, event := range events {
		if !afterAnchor {
			if event.ID == anchorID {
				afterAnchor = true
			}
			continue
		}
		if event.Type == eventlog.EventUserMessage {
			return true
		}
	}
	return false
}

func runningPlanAfterAnchor(events []eventlog.Event, anchorID string) (eventlog.EventPayload, bool) {
	afterAnchor := anchorID == ""
	var latest eventlog.EventPayload
	found := false
	for _, event := range events {
		if !afterAnchor {
			if event.ID == anchorID {
				afterAnchor = true
			}
			continue
		}
		if event.Type == eventlog.EventPlanUpdate {
			latest = event.Payload
			found = true
		}
	}
	return latest, found && latest.PlanStatus == "running"
}
