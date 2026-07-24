package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

const (
	maxDelegatedTasks        = 3
	maxSubagentTimelineSteps = 10
	maxSubagentActivities    = 120
	maxSubagentOutputRunes   = 96_000
)

const (
	subagentExplore   = "explore"
	subagentReview    = "review"
	subagentImplement = "implement"
)

var subagentSequence atomic.Uint64

type subagentExecutionScope struct {
	BaseRequest  protocol.ChatRequest
	PrimaryRoute chatRoute
}

type subagentExecutionScopeKey struct{}

func withSubagentExecutionScope(ctx context.Context, scope subagentExecutionScope) context.Context {
	return context.WithValue(ctx, subagentExecutionScopeKey{}, scope)
}

func subagentExecutionScopeFrom(ctx context.Context) (subagentExecutionScope, bool) {
	scope, ok := ctx.Value(subagentExecutionScopeKey{}).(subagentExecutionScope)
	return scope, ok
}

func (s *Service) registerSubagent(parent context.Context, part tools.ResultPart) (context.Context, *subagentControl) {
	ctx, cancel := context.WithCancel(parent)
	control := &subagentControl{
		taskID: part.TaskID,
		cancel: cancel,
		done:   make(chan struct{}),
		latest: cloneSubagentPart(part),
	}
	s.subagentMu.Lock()
	if s.subagents == nil {
		s.subagents = make(map[string]*subagentControl)
	}
	s.subagents[part.TaskID] = control
	s.subagentMu.Unlock()
	return ctx, control
}

// CancelSubagent stops one delegated worker without cancelling its siblings or
// the coordinating parent turn.
func (s *Service) CancelSubagent(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}
	s.subagentMu.Lock()
	control := s.subagents[taskID]
	s.subagentMu.Unlock()
	if control == nil || control.cancel == nil {
		return false
	}
	control.cancel()
	return true
}

type delegateTaskArguments struct {
	Tasks []delegateTaskSpec `json:"tasks"`
}

type delegateTaskSpec struct {
	Label      string `json:"label"`
	Task       string `json:"task"`
	AgentType  string `json:"agentType"`
	ProviderID string `json:"providerId,omitempty"`
	Model      string `json:"model,omitempty"`
}

type delegatedTaskResult struct {
	part      tools.ResultPart
	artifacts []tools.ResultPart
	usage     *protocol.TokenUsage
	route     chatRoute
}

type subagentControl struct {
	mu        sync.RWMutex
	taskID    string
	cancel    context.CancelFunc
	done      chan struct{}
	doneOnce  sync.Once
	latest    tools.ResultPart
	result    delegatedTaskResult
	finished  bool
	collected bool
}

func (c *subagentControl) update(part tools.ResultPart) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.latest = cloneSubagentPart(part)
	c.mu.Unlock()
}

func (c *subagentControl) finish(result delegatedTaskResult) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.result = cloneDelegatedTaskResult(result)
	c.latest = cloneSubagentPart(result.part)
	c.finished = true
	c.mu.Unlock()
	c.doneOnce.Do(func() { close(c.done) })
}

func (c *subagentControl) snapshot() (tools.ResultPart, delegatedTaskResult, bool, bool) {
	if c == nil {
		return tools.ResultPart{}, delegatedTaskResult{}, false, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneSubagentPart(c.latest), cloneDelegatedTaskResult(c.result), c.finished, c.collected
}

func (c *subagentControl) collect() (delegatedTaskResult, bool, bool) {
	if c == nil {
		return delegatedTaskResult{}, false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.finished {
		return delegatedTaskResult{}, false, false
	}
	newlyCollected := !c.collected
	c.collected = true
	return cloneDelegatedTaskResult(c.result), true, newlyCollected
}

func (c *subagentControl) stateFlags() (finished, collected bool) {
	if c == nil {
		return false, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.finished, c.collected
}

func cloneDelegatedTaskResult(result delegatedTaskResult) delegatedTaskResult {
	result.part = cloneSubagentPart(result.part)
	result.artifacts = append([]tools.ResultPart(nil), result.artifacts...)
	if result.usage != nil {
		usage := *result.usage
		result.usage = &usage
	}
	return result
}

// DelegateTaskTool starts independent workers in the background and returns
// immediately. Worker registries deliberately omit coordinator tools, so
// delegation cannot recurse.
type DelegateTaskTool struct {
	Service *Service
}

func (DelegateTaskTool) Name() string { return "delegate_task" }

func (DelegateTaskTool) Description() string {
	return "将 1-3 个彼此独立的子任务同时放到后台执行并立即返回任务 ID。explore/review 仅可读取；implement 可修改工作区并继续遵守当前沙箱与审批规则。启动后主 Agent 应继续自己的独立工作，临近最终综合时再调用 await_subagents。多个 implement 以及主 Agent 的写入范围必须互不重叠。"
}

func (DelegateTaskTool) InputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"tasks": map[string]any{
				"type":        "array",
				"minItems":    1,
				"maxItems":    maxDelegatedTasks,
				"description": "会同时启动的彼此独立子任务；implement 的文件范围不得重叠，需要前后依赖的工作应分多次调用。",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"label": map[string]any{
							"type":        "string",
							"description": "用于界面展示的简短任务名。",
						},
						"task": map[string]any{
							"type":        "string",
							"description": "完整、可独立执行的目标与交付要求。",
						},
						"agentType": map[string]any{
							"type":        "string",
							"enum":        []string{subagentExplore, subagentReview, subagentImplement},
							"description": "explore=只读探索，review=只读审阅，implement=允许修改。",
						},
						"providerId": map[string]any{
							"type":        "string",
							"description": "可选；留空时跟随主 Agent 的供应商。",
						},
						"model": map[string]any{
							"type":        "string",
							"description": "可选；留空时跟随所选供应商的当前模型。",
						},
					},
					"required": []string{"label", "task", "agentType"},
				},
			},
		},
		"required": []string{"tasks"},
	}
}

func (t DelegateTaskTool) Execute(ctx context.Context, rawArgs json.RawMessage) (tools.Result, error) {
	if t.Service == nil {
		return delegatedTaskError("子代理执行器未初始化"), nil
	}
	scope, ok := subagentExecutionScopeFrom(ctx)
	if !ok {
		return delegatedTaskError("当前调用缺少主 Agent 执行上下文"), nil
	}

	var args delegateTaskArguments
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return delegatedTaskError("子代理参数无效: " + err.Error()), nil
	}
	specs, err := normalizeDelegatedTaskSpecs(args.Tasks)
	if err != nil {
		return delegatedTaskError(err.Error()), nil
	}
	if active := t.Service.activeSubagentCount(); active+len(specs) > maxDelegatedTasks {
		return delegatedTaskError(fmt.Sprintf("当前已有 %d 个子代理运行中，并发扇出最多为 %d；请先继续主任务或收集已有结果。", active, maxDelegatedTasks)), nil
	}

	parts := make([]tools.ResultPart, len(specs))
	workerContexts := make([]context.Context, len(specs))
	controls := make([]*subagentControl, len(specs))
	callID := fmt.Sprintf("subagent-%d-%d", time.Now().UnixNano(), subagentSequence.Add(1))
	var emitMu sync.Mutex
	emitProgress := func(part tools.ResultPart) {
		emitMu.Lock()
		tools.EmitProgress(ctx, cloneSubagentPart(part))
		emitMu.Unlock()
	}

	for index, spec := range specs {
		parts[index] = tools.ResultPart{
			Kind:      tools.PartSubagent,
			TaskID:    fmt.Sprintf("%s-%d", callID, index+1),
			AgentType: spec.AgentType,
			Label:     spec.Label,
			Status:    "pending",
			Summary:   "等待调度",
		}
	}
	if ctx.Err() != nil {
		for index := range parts {
			parts[index].Status = "cancelled"
			parts[index].Summary = "任务在开始前已停止"
			parts[index].CurrentAction = "已停止"
			emitProgress(parts[index])
		}
		return tools.Result{
			Summary: "父任务已停止，子代理未启动。",
			Parts:   parts,
			IsError: true,
		}, nil
	}
	for index := range parts {
		workerContexts[index], controls[index] = t.Service.registerSubagent(ctx, parts[index])
		emitProgress(parts[index])
	}

	for index, spec := range specs {
		go func(index int, spec delegateTaskSpec) {
			control := controls[index]
			emit := func(part tools.ResultPart) {
				part = cloneSubagentPart(part)
				control.update(part)
				emitProgress(part)
			}
			result := t.Service.runDelegatedTask(workerContexts[index], scope, spec, parts[index], emit)
			control.finish(result)
		}(index, spec)
	}
	return tools.Result{
		Summary: delegatedTaskStartSummary(parts),
		Parts:   parts,
	}, nil
}

type awaitSubagentsArguments struct {
	TaskIDs []string `json:"taskIds,omitempty"`
	Wait    *bool    `json:"wait,omitempty"`
}

// AwaitSubagentsTool collects background worker results near the synthesis
// phase. Keeping it separate from delegate_task lets the primary Agent make
// progress while children are still running.
type AwaitSubagentsTool struct {
	Service *Service
}

func (AwaitSubagentsTool) Name() string { return "await_subagents" }

func (AwaitSubagentsTool) Description() string {
	return "查询或等待后台子代理并收集最终结果。默认等待当前轮次尚未收集的全部子代理；wait=false 只查询当前状态。应先完成主 Agent 可独立推进的工作，再在最终综合前调用。"
}

func (AwaitSubagentsTool) InputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"taskIds": map[string]any{
				"type":        "array",
				"description": "可选；指定要收集的子代理任务 ID。留空时处理当前轮次所有尚未收集的任务。",
				"items":       map[string]any{"type": "string"},
			},
			"wait": map[string]any{
				"type":        "boolean",
				"description": "true=等待任务结束（默认）；false=仅返回当前状态。",
				"default":     true,
			},
		},
	}
}

func (t AwaitSubagentsTool) Execute(ctx context.Context, rawArgs json.RawMessage) (tools.Result, error) {
	if t.Service == nil {
		return delegatedTaskError("子代理执行器未初始化"), nil
	}
	var args awaitSubagentsArguments
	if len(strings.TrimSpace(string(rawArgs))) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return delegatedTaskError("收集子代理参数无效: " + err.Error()), nil
		}
	}
	taskIDs := normalizeSubagentTaskIDs(args.TaskIDs)
	wait := true
	if args.Wait != nil {
		wait = *args.Wait
	}
	controls, missing := t.Service.selectSubagents(taskIDs, len(taskIDs) == 0)
	if len(controls) == 0 {
		message := "当前没有尚未收集的子代理。"
		if len(missing) > 0 {
			message = "未找到子代理任务: " + strings.Join(missing, ", ")
		}
		return delegatedTaskError(message), nil
	}

	results := make([]delegatedTaskResult, 0, len(controls))
	parts := make([]tools.ResultPart, 0, len(controls)*2)
	running := 0
	for _, control := range controls {
		if wait {
			select {
			case <-control.done:
			case <-ctx.Done():
			}
		}
		latest, _, finished, _ := control.snapshot()
		if finished {
			result, _, newlyCollected := control.collect()
			results = append(results, result)
			parts = append(parts, result.part)
			if newlyCollected {
				parts = append(parts, result.artifacts...)
				t.Service.recordDelegatedTaskUsage(result)
			}
			continue
		}
		running++
		results = append(results, delegatedTaskResult{part: latest})
		parts = append(parts, latest)
	}

	summary := delegatedTaskSummary(results)
	if running > 0 {
		summary = fmt.Sprintf("仍有 %d 个子代理运行中。\n\n%s", running, summary)
	}
	if len(missing) > 0 {
		summary += "\n\n未找到任务: " + strings.Join(missing, ", ")
	}
	allFinishedWithoutSuccess := running == 0
	if allFinishedWithoutSuccess {
		for _, result := range results {
			if result.part.Status == "completed" {
				allFinishedWithoutSuccess = false
				break
			}
		}
	}
	return tools.Result{
		Summary: clipContextText(summary, 16_000),
		Parts:   parts,
		IsError: allFinishedWithoutSuccess,
	}, nil
}

func normalizeSubagentTaskIDs(input []string) []string {
	result := make([]string, 0, len(input))
	seen := make(map[string]bool, len(input))
	for _, taskID := range input {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" || seen[taskID] {
			continue
		}
		seen[taskID] = true
		result = append(result, taskID)
	}
	return result
}

func (s *Service) selectSubagents(taskIDs []string, uncollectedOnly bool) ([]*subagentControl, []string) {
	s.subagentMu.Lock()
	controlsByID := make(map[string]*subagentControl, len(s.subagents))
	for taskID, control := range s.subagents {
		controlsByID[taskID] = control
	}
	s.subagentMu.Unlock()

	missing := make([]string, 0)
	controls := make([]*subagentControl, 0, len(controlsByID))
	if len(taskIDs) > 0 {
		for _, taskID := range taskIDs {
			control := controlsByID[taskID]
			if control == nil {
				missing = append(missing, taskID)
				continue
			}
			controls = append(controls, control)
		}
		return controls, missing
	}

	for _, control := range controlsByID {
		if uncollectedOnly {
			_, collected := control.stateFlags()
			if collected {
				continue
			}
		}
		controls = append(controls, control)
	}
	sort.Slice(controls, func(left, right int) bool {
		return controls[left].taskID < controls[right].taskID
	})
	return controls, nil
}

func (s *Service) activeSubagentCount() int {
	controls, _ := s.selectSubagents(nil, false)
	active := 0
	for _, control := range controls {
		finished, _ := control.stateFlags()
		if !finished {
			active++
		}
	}
	return active
}

func (s *Service) hasUncollectedSubagents() bool {
	controls, _ := s.selectSubagents(nil, true)
	return len(controls) > 0
}

// finishSubagentTurn is the final lifecycle barrier. On cancellation every
// worker is stopped first; in all cases the method joins workers before the
// caller can commit or roll back the parent turn.
func (s *Service) finishSubagentTurn(cancelWorkers bool) []tools.ResultPart {
	controls, _ := s.selectSubagents(nil, false)
	if cancelWorkers {
		for _, control := range controls {
			if control.cancel != nil {
				control.cancel()
			}
		}
	}
	for _, control := range controls {
		<-control.done
	}

	parts := make([]tools.ResultPart, 0, len(controls)*2)
	for _, control := range controls {
		result, finished, newlyCollected := control.collect()
		if !finished {
			continue
		}
		parts = append(parts, result.part)
		if newlyCollected {
			parts = append(parts, result.artifacts...)
			s.recordDelegatedTaskUsage(result)
		}
	}

	s.subagentMu.Lock()
	for _, control := range controls {
		if s.subagents[control.taskID] == control {
			delete(s.subagents, control.taskID)
		}
	}
	s.subagentMu.Unlock()
	return parts
}

func (s *Service) recordDelegatedTaskUsage(result delegatedTaskResult) {
	if result.usage == nil || result.route.Provider.ID == "" {
		return
	}
	s.recordUsageMetrics(usageMetricsFor(result.route.Provider, result.usage), result.route)
}

func normalizeDelegatedTaskSpecs(input []delegateTaskSpec) ([]delegateTaskSpec, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("至少需要一个子任务")
	}
	if len(input) > maxDelegatedTasks {
		return nil, fmt.Errorf("一次最多委派 %d 个子任务", maxDelegatedTasks)
	}
	output := make([]delegateTaskSpec, len(input))
	for index, spec := range input {
		spec.Label = strings.TrimSpace(spec.Label)
		spec.Task = strings.TrimSpace(spec.Task)
		spec.AgentType = strings.ToLower(strings.TrimSpace(spec.AgentType))
		spec.ProviderID = strings.TrimSpace(spec.ProviderID)
		spec.Model = strings.TrimSpace(spec.Model)
		if spec.Label == "" {
			return nil, fmt.Errorf("第 %d 个子任务缺少 label", index+1)
		}
		if spec.Task == "" {
			return nil, fmt.Errorf("第 %d 个子任务缺少 task", index+1)
		}
		switch spec.AgentType {
		case subagentExplore, subagentReview, subagentImplement:
		default:
			return nil, fmt.Errorf("第 %d 个子任务的 agentType 无效: %s", index+1, spec.AgentType)
		}
		spec.Label = clipContextText(spec.Label, 80)
		spec.Task = clipContextText(spec.Task, 8_000)
		output[index] = spec
	}
	return output, nil
}

func (s *Service) runDelegatedTask(
	ctx context.Context,
	scope subagentExecutionScope,
	spec delegateTaskSpec,
	part tools.ResultPart,
	emit func(tools.ResultPart),
) delegatedTaskResult {
	startedAt := time.Now()
	part.StartedAt = startedAt.Format(time.RFC3339Nano)
	part.Status = "running"
	part.Summary = ""
	part.CurrentAction = "正在准备独立上下文"
	part.Steps = []tools.ProgressStep{{Title: "准备独立上下文", Status: "in_progress"}}
	emit(part)

	finish := func(status, summary, action string, outcome toolLoopOutcome, route chatRoute) delegatedTaskResult {
		completedAt := time.Now()
		part.Status = status
		part.Summary = clipContextText(strings.TrimSpace(summary), 8_000)
		part.CurrentAction = action
		if content := sanitizeModelContent(outcome.Content); content != "" {
			part.SubagentOutput = boundedSubagentText("", content)
		}
		if reasoning := strings.TrimSpace(outcome.Reasoning); reasoning != "" {
			part.SubagentReasoning = boundedSubagentText("", reasoning)
		}
		if part.SubagentOutput == "" && strings.TrimSpace(summary) != "" {
			part.SubagentOutput = boundedSubagentText("", summary)
		}
		part.CompletedAt = completedAt.Format(time.RFC3339Nano)
		part.DurationMs = completedAt.Sub(startedAt).Milliseconds()
		if part.DurationMs < 1 {
			part.DurationMs = 1
		}
		for index := range part.Steps {
			if part.Steps[index].Status == "in_progress" {
				if status == "completed" {
					part.Steps[index].Status = "completed"
				} else {
					part.Steps[index].Status = status
				}
			}
		}
		files, additions, deletions := tools.SummarizeFileChanges(outcome.Changes)
		part.ChangedFiles = files
		part.Additions = additions
		part.Deletions = deletions
		emit(part)
		return delegatedTaskResult{
			part:      part,
			artifacts: delegatedTaskArtifacts(outcome.Parts),
			usage:     outcome.Usage,
			route:     route,
		}
	}

	if err := ctx.Err(); err != nil {
		return finish("cancelled", "任务在开始前已停止", "已停止", toolLoopOutcome{}, chatRoute{})
	}
	route, err := s.resolveSubagentRoute(spec, scope.PrimaryRoute)
	if err != nil {
		return finish("error", err.Error(), "路由失败", toolLoopOutcome{}, chatRoute{})
	}
	part.ProviderID = route.Provider.ID
	part.Model = route.ModelID
	provider, err := s.chatProviderForRoute(route)
	if err != nil {
		return finish("error", err.Error(), "模型连接失败", toolLoopOutcome{}, route)
	}

	part.Steps[0].Status = "completed"
	part.Steps = append(part.Steps, tools.ProgressStep{Title: "分析任务", Status: "in_progress"})
	part.CurrentAction = "正在分析任务"
	emit(part)

	request := subagentRequest(scope.BaseRequest, spec, part.TaskID, route)
	registry := s.buildReadOnlyRegistry()
	if spec.AgentType == subagentImplement {
		registry = s.buildWorkerToolRegistry()
	}
	stepIndex := make(map[string]int)
	activityIndex := make(map[string]int)
	lastStreamEmit := time.Time{}
	emitStream := func(force bool) {
		if !force && time.Since(lastStreamEmit) < 75*time.Millisecond {
			return
		}
		lastStreamEmit = time.Now()
		emit(part)
	}
	childSink := func(event ChatStreamEvent) {
		switch event.Type {
		case "delta":
			part.SubagentOutput = boundedSubagentText(part.SubagentOutput, event.Delta)
			emitStream(false)
		case "reasoning":
			part.SubagentReasoning = boundedSubagentText(part.SubagentReasoning, event.Delta)
			emitStream(false)
		case "tool":
			key := strings.TrimSpace(event.ToolCallID)
			if key == "" {
				key = event.ToolName + "\x00" + event.ToolInput
			}
			index, exists := stepIndex[key]
			if !exists && len(part.Steps) < maxSubagentTimelineSteps {
				title := subagentToolStepTitle(event.ToolName, event.ToolInput)
				part.Steps = append(part.Steps, tools.ProgressStep{Title: title, Status: "in_progress"})
				index = len(part.Steps) - 1
				stepIndex[key] = index
			}
			if exists || index > 0 {
				switch event.Status {
				case "error":
					part.Steps[index].Status = "error"
				case "completed", "ok":
					part.Steps[index].Status = "completed"
				default:
					part.Steps[index].Status = "in_progress"
				}
			}
			if message := strings.TrimSpace(event.Message); message != "" {
				part.CurrentAction = clipContextText(message, 240)
			} else {
				part.CurrentAction = subagentToolStepTitle(event.ToolName, event.ToolInput)
			}
			upsertSubagentActivity(&part, activityIndex, event)
			emit(part)
		case "status", "context_compression":
			if message := strings.TrimSpace(event.Message); message != "" {
				part.CurrentAction = clipContextText(message, 240)
				emit(part)
			}
		case "provider_notice":
			appendSubagentProviderActivity(&part, event)
			emit(part)
		}
	}

	outcome, runErr := s.runStreamingToolLoop(ctx, provider, registry, request, childSink)
	emitStream(true)
	content := sanitizeModelContent(outcome.Content)
	if content == "" {
		content = partialToolFailureContent(outcome)
	}
	if ctx.Err() != nil {
		if content == "" {
			content = "子任务已停止"
		}
		return finish("cancelled", content, "已停止", outcome, route)
	}
	if runErr != nil {
		if content == "" {
			content = redactSensitiveText(runErr.Error())
		}
		return finish("error", content, "执行失败", outcome, route)
	}
	if content == "" {
		content = "子任务已完成，未返回额外说明。"
	}
	return finish("completed", content, "已完成", outcome, route)
}

func (s *Service) resolveSubagentRoute(spec delegateTaskSpec, primary chatRoute) (chatRoute, error) {
	if spec.ProviderID == "" {
		route := primary
		if spec.Model != "" {
			route.ModelID = spec.Model
		}
		return route, nil
	}
	runtime := s.stateRuntimeSettings()
	provider, _, ok := findModelProvider(runtime.Model.Providers, spec.ProviderID)
	if !ok || !provider.Enabled {
		return chatRoute{}, fmt.Errorf("子代理供应商不可用: %s", spec.ProviderID)
	}
	route, err := s.chatRouteForProvider(runtime, provider)
	if err != nil {
		return chatRoute{}, err
	}
	if spec.Model != "" {
		route.ModelID = spec.Model
	}
	return route, nil
}

func subagentRequest(base protocol.ChatRequest, spec delegateTaskSpec, taskID string, route chatRoute) protocol.ChatRequest {
	request := base
	request.Model = route.ModelID
	request.Metadata = make(map[string]string, len(base.Metadata)+4)
	for key, value := range base.Metadata {
		request.Metadata[key] = value
	}
	request.Metadata["request_kind"] = "subagent"
	request.Metadata["subagent_task_id"] = taskID
	request.Metadata["subagent_type"] = spec.AgentType
	request.Metadata["parent_turn_id"] = base.TurnID
	if value := strings.TrimSuffix(strings.TrimSpace(base.SessionID), ":"); value != "" {
		request.SessionID = value + ":subagent:" + taskID
	}
	if value := strings.TrimSuffix(strings.TrimSpace(base.ThreadID), ":"); value != "" {
		request.ThreadID = value + ":subagent:" + taskID
	}
	if value := strings.TrimSuffix(strings.TrimSpace(base.TurnID), ":"); value != "" {
		request.TurnID = value + ":subagent:" + taskID
	}
	request.ResponsesContext.RequestKind = "subagent"
	request.ResponsesContext.ThreadSource = "agent"
	if request.ResponsesContext.WindowID != "" {
		request.ResponsesContext.WindowID += ":subagent:" + taskID
	}
	request.Messages = cloneProtocolMessages(base.Messages)
	request.Messages = append(request.Messages, protocol.Message{
		Role:    "user",
		Content: subagentInstruction(spec),
	})
	request.Tools = nil
	request.ToolChoice = "auto"
	request.ParallelToolCalls = false
	return request
}

func subagentInstruction(spec delegateTaskSpec) string {
	modeRule := "只读探索真实工作区并返回有文件路径和行号的证据；禁止修改文件或运行 Shell。"
	switch spec.AgentType {
	case subagentReview:
		modeRule = "只读审阅真实工作区，优先报告正确性、回归、安全边界与测试缺口；禁止修改文件或运行 Shell。"
	case subagentImplement:
		modeRule = "使用结构化工具完成真实修改和必要验证；只修改子任务明确分配的文件范围，避免触碰主 Agent 或兄弟子代理负责的文件；所有写入、命令、网络与审批继续遵守主 Agent 的沙箱策略。"
	}
	return strings.Join([]string{
		"你是由 MHcode 主 Agent 动态创建的独立子代理。",
		"不要尝试创建或委派新的子代理；你的工具集中也不会提供 delegate_task。",
		modeRule,
		"只处理下面的独立目标，不要重新执行整个用户任务。完成后给主 Agent 一份简洁、可核验的结果摘要。",
		"",
		"子任务：" + spec.Label,
		spec.Task,
	}, "\n")
}

func subagentToolStepTitle(name, input string) string {
	label := strings.TrimSpace(name)
	if label == "" {
		label = "工具"
	}
	switch label {
	case "read_file":
		label = "读取文件"
	case "list_dir":
		label = "查看目录"
	case "search":
		label = "搜索代码"
	case "read_repository":
		label = "读取仓库"
	case "read_webpage":
		label = "读取网页"
	case "web_search":
		label = "搜索网络"
	case "write_file":
		label = "写入文件"
	case "apply_patch":
		label = "修改文件"
	case "run_command":
		label = "运行命令"
	case "git":
		label = "检查 Git"
	case "terminal":
		label = "操作终端"
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return label
	}
	return clipContextText(label+" · "+input, 180)
}

func delegatedTaskArtifacts(parts []tools.ResultPart) []tools.ResultPart {
	artifacts := make([]tools.ResultPart, 0)
	seen := make(map[string]bool)
	for _, part := range parts {
		if part.Kind != tools.PartDiff && part.Kind != tools.PartFile {
			continue
		}
		key := string(part.Kind) + "\x00" + part.Path + "\x00" + part.Patch
		if seen[key] {
			continue
		}
		seen[key] = true
		artifacts = append(artifacts, part)
	}
	return artifacts
}

func delegatedTaskStartSummary(parts []tools.ResultPart) string {
	var out strings.Builder
	fmt.Fprintf(&out, "已在后台启动 %d 个子代理。主 Agent 应继续处理可独立推进且文件范围不重叠的工作，并在最终综合前调用 await_subagents 收集结果。", len(parts))
	for _, part := range parts {
		label := strings.TrimSpace(part.Label)
		if label == "" {
			label = "子任务"
		}
		fmt.Fprintf(&out, "\n- %s: %s (%s)", part.TaskID, label, part.AgentType)
	}
	return clipContextText(out.String(), 4_000)
}

func delegatedTaskSummary(results []delegatedTaskResult) string {
	var out strings.Builder
	completed := 0
	for _, result := range results {
		if result.part.Status == "completed" {
			completed++
		}
	}
	fmt.Fprintf(&out, "子代理完成 %d/%d 个任务。", completed, len(results))
	for _, result := range results {
		label := strings.TrimSpace(result.part.Label)
		if label == "" {
			label = result.part.TaskID
		}
		fmt.Fprintf(&out, "\n\n[%s · %s · %s]", label, result.part.AgentType, result.part.Status)
		if summary := strings.TrimSpace(result.part.Summary); summary != "" {
			out.WriteString("\n")
			out.WriteString(clipContextText(summary, 6_000))
		}
	}
	return clipContextText(out.String(), 16_000)
}

func delegatedTaskError(message string) tools.Result {
	return tools.Result{Summary: message, IsError: true}
}

func cloneSubagentPart(part tools.ResultPart) tools.ResultPart {
	part.Steps = append([]tools.ProgressStep(nil), part.Steps...)
	part.Activities = append([]tools.SubagentActivity(nil), part.Activities...)
	return part
}

func boundedSubagentText(current, addition string) string {
	if addition == "" {
		return current
	}
	runes := []rune(current + addition)
	if len(runes) <= maxSubagentOutputRunes {
		return string(runes)
	}
	return "[较早的输出已省略]\n" + string(runes[len(runes)-maxSubagentOutputRunes:])
}

func upsertSubagentActivity(part *tools.ResultPart, indexes map[string]int, event ChatStreamEvent) {
	if part == nil {
		return
	}
	key := strings.TrimSpace(event.ToolCallID)
	if key == "" {
		key = event.ToolName + "\x00" + event.ToolInput
	}
	index, exists := indexes[key]
	if !exists {
		if len(part.Activities) >= maxSubagentActivities {
			return
		}
		index = len(part.Activities)
		indexes[key] = index
		part.Activities = append(part.Activities, tools.SubagentActivity{
			ID: key, Kind: "tool", Title: subagentToolStepTitle(event.ToolName, event.ToolInput),
			Status: "running", Input: strings.TrimSpace(event.ToolInput), StartedAt: time.Now().Format(time.RFC3339Nano),
		})
	}
	activity := &part.Activities[index]
	if event.Status != "" {
		activity.Status = event.Status
	}
	if output := subagentActivityOutput(event); output != "" {
		activity.Output = boundedSubagentText(activity.Output, output)
	}
	if event.Status == "completed" || event.Status == "ok" || event.Status == "error" {
		completed := time.Now()
		activity.CompletedAt = completed.Format(time.RFC3339Nano)
		if started, err := time.Parse(time.RFC3339Nano, activity.StartedAt); err == nil {
			activity.DurationMs = completed.Sub(started).Milliseconds()
			if activity.DurationMs < 1 {
				activity.DurationMs = 1
			}
		}
	}
}

func subagentActivityOutput(event ChatStreamEvent) string {
	var output strings.Builder
	for _, part := range event.Parts {
		for _, value := range []string{part.Output, part.Stdout, part.Stderr} {
			value = strings.TrimSpace(value)
			if value == "" || strings.Contains(output.String(), value) {
				continue
			}
			if output.Len() > 0 {
				output.WriteString("\n")
			}
			output.WriteString(value)
		}
	}
	if output.Len() == 0 && event.Status != "running" {
		output.WriteString(strings.TrimSpace(event.Message))
	}
	return output.String()
}

func appendSubagentProviderActivity(part *tools.ResultPart, event ChatStreamEvent) {
	if part == nil || len(part.Activities) >= maxSubagentActivities {
		return
	}
	message := strings.TrimSpace(event.Message)
	if message == "" {
		return
	}
	part.Activities = append(part.Activities, tools.SubagentActivity{
		ID: fmt.Sprintf("provider-%d", time.Now().UnixNano()), Kind: "provider", Title: "模型运行信息",
		Status: "completed", Output: message, StartedAt: time.Now().Format(time.RFC3339Nano),
	})
}
