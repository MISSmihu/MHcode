package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

type subagentProbeProvider struct {
	mu          sync.Mutex
	requests    []protocol.ChatRequest
	active      int
	maxActive   int
	delay       time.Duration
	block       bool
	started     chan struct{}
	startedOnce sync.Once
}

type subagentEndToEndProvider struct {
	mu                 sync.Mutex
	mainCalls          int
	childCalls         int
	childTools         []protocol.ToolDefinition
	sawDelegatedResult bool
}

type selectiveSubagentProvider struct {
	started   chan string
	cancelled chan string
	release   chan struct{}
}

func (p *selectiveSubagentProvider) Name() string { return "selective-subagent" }

func (p *selectiveSubagentProvider) ListModels(context.Context) ([]protocol.Model, error) {
	return []protocol.Model{{ID: "subagent-model"}}, nil
}

func (p *selectiveSubagentProvider) Stream(ctx context.Context, request protocol.ChatRequest) (<-chan protocol.StreamEvent, error) {
	taskID := request.Metadata["subagent_task_id"]
	events := make(chan protocol.StreamEvent, 2)
	go func() {
		defer close(events)
		p.started <- taskID
		select {
		case <-ctx.Done():
			p.cancelled <- taskID
		case <-p.release:
			events <- protocol.StreamEvent{Type: "delta", Delta: "worker completed"}
		}
	}()
	return events, nil
}

func (p *subagentProbeProvider) Name() string { return "subagent-probe" }

func (p *subagentProbeProvider) ListModels(context.Context) ([]protocol.Model, error) {
	return []protocol.Model{{ID: "subagent-model"}}, nil
}

func (p *subagentProbeProvider) Stream(ctx context.Context, request protocol.ChatRequest) (<-chan protocol.StreamEvent, error) {
	p.mu.Lock()
	p.requests = append(p.requests, request)
	p.active++
	if p.active > p.maxActive {
		p.maxActive = p.active
	}
	p.mu.Unlock()
	if p.started != nil {
		p.startedOnce.Do(func() { close(p.started) })
	}

	events := make(chan protocol.StreamEvent, 2)
	go func() {
		defer close(events)
		defer func() {
			p.mu.Lock()
			p.active--
			p.mu.Unlock()
		}()
		if p.block {
			<-ctx.Done()
			return
		}
		timer := time.NewTimer(p.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			events <- protocol.StreamEvent{Type: "delta", Delta: "已完成独立检查。"}
			events <- protocol.StreamEvent{Type: "usage", Usage: &protocol.TokenUsage{PromptTokens: 20, CompletionTokens: 5}}
		}
	}()
	return events, nil
}

func (p *subagentProbeProvider) snapshot() ([]protocol.ChatRequest, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]protocol.ChatRequest(nil), p.requests...), p.maxActive
}

func (p *subagentEndToEndProvider) Name() string { return "subagent-e2e" }

func (p *subagentEndToEndProvider) ListModels(context.Context) ([]protocol.Model, error) {
	return []protocol.Model{{ID: "subagent-model"}}, nil
}

func (p *subagentEndToEndProvider) Complete(context.Context, protocol.ChatRequest) (protocol.CompletionResult, error) {
	return protocol.CompletionResult{}, errors.New("unexpected non-streaming completion")
}

func (p *subagentEndToEndProvider) Stream(_ context.Context, request protocol.ChatRequest) (<-chan protocol.StreamEvent, error) {
	events := make(chan protocol.StreamEvent, 2)
	if request.Metadata["request_kind"] == "subagent" {
		p.mu.Lock()
		p.childCalls++
		p.childTools = append([]protocol.ToolDefinition(nil), request.Tools...)
		p.mu.Unlock()
		events <- protocol.StreamEvent{Type: "delta", Delta: "子代理确认了真实目录结构。"}
		close(events)
		return events, nil
	}

	p.mu.Lock()
	p.mainCalls++
	mainCall := p.mainCalls
	if len(request.Messages) > 0 && request.Messages[len(request.Messages)-1].Role == "tool" {
		p.sawDelegatedResult = strings.Contains(request.Messages[len(request.Messages)-1].Content, "子代理确认了真实目录结构")
	}
	p.mu.Unlock()
	if mainCall == 1 {
		events <- protocol.StreamEvent{Type: "tool_calls", ToolCalls: []protocol.ToolCall{{
			ID: "delegate-1", Type: "function", Function: protocol.ToolCallFunction{
				Name:      "delegate_task",
				Arguments: json.RawMessage(`{"tasks":[{"label":"结构检查","task":"检查当前项目结构并返回证据","agentType":"explore"}]}`),
			},
		}}}
	} else {
		events <- protocol.StreamEvent{Type: "delta", Delta: "主 Agent 已综合子代理结果。"}
	}
	close(events)
	return events, nil
}

func newSubagentToolTest(t *testing.T, provider protocol.Provider) (*Service, context.Context) {
	t.Helper()
	service := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	service.runtimeSettings.WorkspaceRoot = t.TempDir()
	service.runtimeSettings.FilesystemAccess = "workspace-write"
	service.runtimeSettings.ApprovalPolicy = "never"
	service.providerFactory = func(chatRoute) (protocol.Provider, error) { return provider, nil }
	route := chatRoute{
		Provider: ModelProviderSetting{ID: "probe", Name: "Probe", Protocol: "local", Enabled: true},
		ModelID:  "subagent-model",
	}
	scope := subagentExecutionScope{
		BaseRequest: protocol.ChatRequest{
			Model:      route.ModelID,
			Messages:   []protocol.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "inspect the project"}},
			Metadata:   map[string]string{"task_kind": "chat"},
			SessionID:  "session-test",
			ThreadID:   "thread-test",
			TurnID:     "turn-test",
			ToolChoice: "auto",
		},
		PrimaryRoute: route,
	}
	return service, withSubagentExecutionScope(context.Background(), scope)
}

func TestDelegateTaskRunsReadOnlyWorkersConcurrentlyWithoutCoordinatorTools(t *testing.T) {
	provider := &subagentProbeProvider{delay: 80 * time.Millisecond}
	service, ctx := newSubagentToolTest(t, provider)
	args := json.RawMessage(`{"tasks":[
		{"label":"结构探索","task":"检查目录结构","agentType":"explore"},
		{"label":"风险审阅","task":"检查潜在回归","agentType":"review"}
	]}`)
	result, err := (DelegateTaskTool{Service: service}).Execute(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("delegate result = %#v", result)
	}
	requests, maxActive := provider.snapshot()
	if maxActive < 2 {
		t.Fatalf("read-only workers did not overlap, max active = %d", maxActive)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	for _, request := range requests {
		if request.Metadata["request_kind"] != "subagent" || request.Metadata["subagent_task_id"] == "" {
			t.Fatalf("missing subagent metadata: %#v", request.Metadata)
		}
		assertSubagentToolSet(t, request.Tools, false)
	}
	assertSubagentParts(t, result.Parts, 2, "completed")
}

func TestMainAgentDelegatesAndSynthesizesSubagentResult(t *testing.T) {
	provider := &subagentEndToEndProvider{}
	service := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	service.reasoning = ReasoningHigh
	service.runtimeSettings.WorkspaceRoot = t.TempDir()
	service.runtimeSettings.FilesystemAccess = "workspace-write"
	service.runtimeSettings.SandboxMode = "workspace-write"
	service.runtimeSettings.ApprovalPolicy = "never"
	service.runtimeSettings.Model = ModelSettings{
		SelectedProviderID: "probe",
		SelectedModelID:    "subagent-model",
		Providers: []ModelProviderSetting{{
			ID: "probe", Name: "Probe", Protocol: "local", APIType: "chat-completions",
			BaseURL: "http://127.0.0.1:11434/v1", Enabled: true, DefaultModelID: "subagent-model",
			Models: []ProviderModel{{ID: "subagent-model", DisplayName: "Subagent", Provider: "probe", ContextWindowTokens: 128000}},
		}},
	}
	service.providerFactory = func(chatRoute) (protocol.Provider, error) { return provider, nil }
	var liveStatuses []string
	var liveOutput string
	result, err := service.SendChatMessageWithEvents(context.Background(), "请并行检查项目结构", func(event ChatStreamEvent) {
		for _, part := range event.Parts {
			if part.Kind == tools.PartSubagent {
				liveStatuses = append(liveStatuses, part.Status)
				if part.SubagentOutput != "" {
					liveOutput = part.SubagentOutput
				}
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "主 Agent 已综合子代理结果。" {
		t.Fatalf("result content = %q", result.Content)
	}
	provider.mu.Lock()
	mainCalls, childCalls := provider.mainCalls, provider.childCalls
	childTools := append([]protocol.ToolDefinition(nil), provider.childTools...)
	sawDelegatedResult := provider.sawDelegatedResult
	provider.mu.Unlock()
	if mainCalls != 2 || childCalls != 1 || !sawDelegatedResult {
		t.Fatalf("provider calls main=%d child=%d saw result=%v", mainCalls, childCalls, sawDelegatedResult)
	}
	assertSubagentToolSet(t, childTools, false)
	assertSubagentParts(t, result.Parts, 1, "completed")
	if !containsString(liveStatuses, "running") || !containsString(liveStatuses, "completed") {
		t.Fatalf("live subagent statuses = %#v", liveStatuses)
	}
	if !strings.Contains(liveOutput, "子代理确认了真实目录结构") {
		t.Fatalf("live subagent output = %q", liveOutput)
	}
}

func TestDelegateTaskRunsImplementWorkersConcurrentlyAndAllowsWriteTools(t *testing.T) {
	provider := &subagentProbeProvider{delay: 45 * time.Millisecond}
	service, ctx := newSubagentToolTest(t, provider)
	args := json.RawMessage(`{"tasks":[
		{"label":"实现一","task":"完成第一项修改","agentType":"implement"},
		{"label":"实现二","task":"完成第二项修改","agentType":"implement"}
	]}`)
	result, err := (DelegateTaskTool{Service: service}).Execute(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	requests, maxActive := provider.snapshot()
	if maxActive < 2 {
		t.Fatalf("implement workers did not overlap, max active = %d", maxActive)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	for _, request := range requests {
		assertSubagentToolSet(t, request.Tools, true)
	}
	assertSubagentParts(t, result.Parts, 2, "completed")
}

func TestDelegateTaskCancellationPersistsCancelledWorkerState(t *testing.T) {
	provider := &subagentProbeProvider{block: true, started: make(chan struct{})}
	service, baseCtx := newSubagentToolTest(t, provider)
	ctx, cancel := context.WithCancel(baseCtx)
	done := make(chan tools.Result, 1)
	go func() {
		result, _ := (DelegateTaskTool{Service: service}).Execute(ctx, json.RawMessage(`{"tasks":[{"label":"阻塞检查","task":"等待取消","agentType":"explore"}]}`))
		done <- result
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("subagent provider did not start")
	}
	cancel()
	select {
	case result := <-done:
		if !result.IsError {
			t.Fatalf("cancelled result should be marked as error: %#v", result)
		}
		assertSubagentParts(t, result.Parts, 1, "cancelled")
	case <-time.After(time.Second):
		t.Fatal("delegate task did not stop promptly")
	}
}

func TestCancelSubagentStopsOnlySelectedWorker(t *testing.T) {
	provider := &selectiveSubagentProvider{
		started: make(chan string, 2), cancelled: make(chan string, 1), release: make(chan struct{}),
	}
	service, baseCtx := newSubagentToolTest(t, provider)
	var mu sync.Mutex
	var taskIDs []string
	ctx := tools.WithProgressSink(baseCtx, func(part tools.ResultPart) {
		if part.Kind != tools.PartSubagent || part.Status != "pending" {
			return
		}
		mu.Lock()
		taskIDs = append(taskIDs, part.TaskID)
		mu.Unlock()
	})
	done := make(chan tools.Result, 1)
	go func() {
		result, _ := (DelegateTaskTool{Service: service}).Execute(ctx, json.RawMessage(`{"tasks":[
			{"label":"first","task":"wait","agentType":"explore"},
			{"label":"second","task":"wait","agentType":"review"}
		]}`))
		done <- result
	}()

	started := map[string]bool{}
	for len(started) < 2 {
		select {
		case taskID := <-provider.started:
			started[taskID] = true
		case <-time.After(time.Second):
			t.Fatal("workers did not start concurrently")
		}
	}
	mu.Lock()
	if len(taskIDs) != 2 {
		mu.Unlock()
		t.Fatalf("pending task IDs = %#v", taskIDs)
	}
	selected := taskIDs[0]
	mu.Unlock()
	if !service.CancelSubagent(selected) {
		t.Fatal("selected subagent was not cancelled")
	}
	select {
	case cancelled := <-provider.cancelled:
		if cancelled != selected {
			t.Fatalf("cancelled worker = %q, want %q", cancelled, selected)
		}
	case <-time.After(time.Second):
		t.Fatal("selected worker did not stop promptly")
	}
	close(provider.release)

	select {
	case result := <-done:
		statuses := map[string]string{}
		for _, part := range result.Parts {
			if part.Kind == tools.PartSubagent {
				statuses[part.TaskID] = part.Status
			}
		}
		if statuses[selected] != "cancelled" {
			t.Fatalf("selected status = %q", statuses[selected])
		}
		completed := 0
		for taskID, status := range statuses {
			if taskID != selected && status == "completed" {
				completed++
			}
		}
		if completed != 1 {
			t.Fatalf("sibling status map = %#v", statuses)
		}
	case <-time.After(time.Second):
		t.Fatal("delegate task did not finish after sibling completed")
	}
}

func TestSubagentPartRoundTripsThroughEventLog(t *testing.T) {
	source := []tools.ResultPart{{
		Kind: tools.PartSubagent, TaskID: "subagent-1", AgentType: "review", Label: "审阅",
		Status: "completed", ProviderID: "provider", Model: "model", Summary: "没有发现回归。",
		CurrentAction: "已完成", Steps: []tools.ProgressStep{{Title: "检查测试", Status: "completed"}},
		SubagentOutput: "审阅输出", SubagentReasoning: "检查路径",
		Activities:   []tools.SubagentActivity{{ID: "tool-1", Kind: "tool", Title: "读取文件", Status: "completed", Output: "ok"}},
		ChangedFiles: 2, Additions: 4, Deletions: 1, DurationMs: 25,
	}}
	restored := fromEventParts(toEventParts(source))
	if len(restored) != 1 {
		t.Fatalf("restored parts = %#v", restored)
	}
	part := restored[0]
	if part.Kind != tools.PartSubagent || part.TaskID != source[0].TaskID || part.AgentType != "review" || part.CurrentAction != "已完成" {
		t.Fatalf("restored subagent part = %#v", part)
	}
	if len(part.Steps) != 1 || part.Steps[0].Title != "检查测试" || part.ChangedFiles != 2 {
		t.Fatalf("restored subagent details = %#v", part)
	}
	if part.SubagentOutput != "审阅输出" || part.SubagentReasoning != "检查路径" || len(part.Activities) != 1 || part.Activities[0].Output != "ok" {
		t.Fatalf("restored subagent transcript = %#v", part)
	}
}

func TestCancelledSubagentPartSurvivesToolErrorWrapping(t *testing.T) {
	provider := &subagentProbeProvider{delay: time.Millisecond}
	service, baseCtx := newSubagentToolTest(t, provider)
	ctx, cancel := context.WithCancel(baseCtx)
	cancel()
	call := protocol.ToolCall{ID: "delegate-cancelled", Type: "function", Function: protocol.ToolCallFunction{
		Name:      "delegate_task",
		Arguments: json.RawMessage(`{"tasks":[{"label":"取消测试","task":"不应开始","agentType":"explore"}]}`),
	}}
	result, _ := service.executeToolCall(ctx, service.buildToolRegistry(), call)
	assertSubagentParts(t, result.Parts, 1, "cancelled")
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func assertSubagentToolSet(t *testing.T, definitions []protocol.ToolDefinition, writable bool) {
	t.Helper()
	names := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		names[definition.Function.Name] = true
	}
	if names["delegate_task"] {
		t.Fatal("worker tool registry contains delegate_task")
	}
	if names["update_plan"] {
		t.Fatal("worker tool registry contains main-plan coordinator")
	}
	if !names["read_file"] || !names["search"] {
		t.Fatalf("worker is missing read tools: %#v", names)
	}
	for _, name := range []string{"write_file", "apply_patch", "copy_file", "delete_file", "run_command", "terminal"} {
		if !writable && names[name] {
			t.Fatalf("read-only worker contains mutating tool %s", name)
		}
	}
	if writable && (!names["write_file"] || !names["apply_patch"]) {
		t.Fatalf("implement worker is missing write tools: %#v", names)
	}
}

func assertSubagentParts(t *testing.T, parts []tools.ResultPart, expected int, status string) {
	t.Helper()
	count := 0
	ids := make(map[string]bool)
	for _, part := range parts {
		if part.Kind != tools.PartSubagent {
			continue
		}
		count++
		if part.Status != status {
			t.Fatalf("subagent status = %q, want %q: %#v", part.Status, status, part)
		}
		if part.TaskID == "" || ids[part.TaskID] {
			t.Fatalf("invalid subagent task id: %#v", part)
		}
		ids[part.TaskID] = true
	}
	if count != expected {
		t.Fatalf("subagent part count = %d, want %d: %#v", count, expected, parts)
	}
}
