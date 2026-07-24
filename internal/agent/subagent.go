package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

const (
	maxDelegatedTasks          = 3
	maxDelegatedToolCallBudget = 24
	minDelegatedToolCallBudget = 6
	maxSubagentTimelineSteps   = 10
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
	MaxToolCalls int
}

type subagentExecutionScopeKey struct{}

func withSubagentExecutionScope(ctx context.Context, scope subagentExecutionScope) context.Context {
	return context.WithValue(ctx, subagentExecutionScopeKey{}, scope)
}

func subagentExecutionScopeFrom(ctx context.Context) (subagentExecutionScope, bool) {
	scope, ok := ctx.Value(subagentExecutionScopeKey{}).(subagentExecutionScope)
	return scope, ok
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

// DelegateTaskTool lets the primary Agent create bounded, independent workers.
// Worker registries deliberately omit this tool, so delegation cannot recurse.
type DelegateTaskTool struct {
	Service *Service
}

func (DelegateTaskTool) Name() string { return "delegate_task" }

func (DelegateTaskTool) Description() string {
	return "将 1-3 个彼此独立的子任务委派给动态子代理。explore/review 仅可读取并可并发；implement 可修改工作区但会串行执行并继续遵守当前沙箱与审批规则。仅在并行探索、独立审阅或隔离实现能明显推进任务时使用。"
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
				"description": "彼此独立的子任务；需要前后依赖的工作应分多次调用。",
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

	budgets := allocateSubagentToolBudgets(scope.MaxToolCalls, specs)
	results := make([]delegatedTaskResult, len(specs))
	parts := make([]tools.ResultPart, len(specs))
	callID := fmt.Sprintf("subagent-%d-%d", time.Now().UnixNano(), subagentSequence.Add(1))
	var emitMu sync.Mutex
	emit := func(part tools.ResultPart) {
		part.Steps = append([]tools.ProgressStep(nil), part.Steps...)
		emitMu.Lock()
		tools.EmitProgress(ctx, part)
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
		emit(parts[index])
	}

	// Read-only workers can safely share the same workspace snapshot. They are
	// completed before writable workers start, so reviews never race file edits.
	var readers sync.WaitGroup
	for index, spec := range specs {
		if spec.AgentType == subagentImplement {
			continue
		}
		readers.Add(1)
		go func(index int, spec delegateTaskSpec) {
			defer readers.Done()
			results[index] = t.Service.runDelegatedTask(ctx, scope, spec, parts[index], budgets[index], emit)
		}(index, spec)
	}
	readers.Wait()

	for index, spec := range specs {
		if spec.AgentType != subagentImplement {
			continue
		}
		results[index] = t.Service.runDelegatedTask(ctx, scope, spec, parts[index], budgets[index], emit)
	}

	resultParts := make([]tools.ResultPart, 0, len(results)*2)
	completed := 0
	for _, result := range results {
		resultParts = append(resultParts, result.part)
		resultParts = append(resultParts, result.artifacts...)
		if result.part.Status == "completed" {
			completed++
		}
		if result.usage != nil && result.route.Provider.ID != "" {
			t.Service.recordUsageMetrics(usageMetricsFor(result.route.Provider, result.usage), result.route)
		}
	}

	summary := delegatedTaskSummary(results)
	if completed == 0 {
		resultParts = append([]tools.ResultPart{{
			Kind: tools.PartToolCall, Name: "delegate_task", Status: "error", Output: summary,
		}}, resultParts...)
	}
	return tools.Result{
		Summary: summary,
		Parts:   resultParts,
		IsError: completed == 0,
	}, nil
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

func allocateSubagentToolBudgets(parentBudget int, specs []delegateTaskSpec) []int {
	if len(specs) == 0 {
		return nil
	}
	total := parentBudget
	if total < minDelegatedToolCallBudget {
		total = minDelegatedToolCallBudget
	}
	if total > maxDelegatedToolCallBudget {
		total = maxDelegatedToolCallBudget
	}
	weights := make([]int, len(specs))
	weightTotal := 0
	for index, spec := range specs {
		weights[index] = 1
		if spec.AgentType == subagentImplement {
			weights[index] = 2
		}
		weightTotal += weights[index]
	}
	budgets := make([]int, len(specs))
	remaining := total
	remainingWeight := weightTotal
	for index, weight := range weights {
		budget := remaining * weight / remainingWeight
		if budget < 1 {
			budget = 1
		}
		budgets[index] = budget
		remaining -= budget
		remainingWeight -= weight
	}
	return budgets
}

func (s *Service) runDelegatedTask(
	ctx context.Context,
	scope subagentExecutionScope,
	spec delegateTaskSpec,
	part tools.ResultPart,
	toolBudget int,
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
	childSink := func(event ChatStreamEvent) {
		switch event.Type {
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
			emit(part)
		case "status", "context_compression":
			if message := strings.TrimSpace(event.Message); message != "" {
				part.CurrentAction = clipContextText(message, 240)
				emit(part)
			}
		}
	}

	outcome, runErr := s.runStreamingToolLoop(ctx, provider, registry, request, toolBudget, childSink)
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
		modeRule = "使用结构化工具完成真实修改和必要验证；所有写入、命令、网络与审批继续遵守主 Agent 的沙箱策略。"
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
