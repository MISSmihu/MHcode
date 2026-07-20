package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MISSmihu/MHcode/internal/protocol"
)

type teamProviderScript struct {
	mu                  sync.Mutex
	calls               map[string]int
	readOnlyToolLeak    bool
	reviewerNeedsChange bool
	implementerErr      error
	blockRole           string
	blockStarted        chan struct{}
}

type scriptedTeamProvider struct {
	role   string
	script *teamProviderScript
}

func (p scriptedTeamProvider) Name() string { return "team-test-" + p.role }

func (p scriptedTeamProvider) ListModels(context.Context) ([]protocol.Model, error) {
	return []protocol.Model{{ID: "model-" + p.role}}, nil
}

func (p scriptedTeamProvider) Stream(ctx context.Context, request protocol.ChatRequest) (<-chan protocol.StreamEvent, error) {
	if p.script == nil {
		return nil, errors.New("missing team provider script")
	}
	p.script.mu.Lock()
	p.script.calls[p.role]++
	call := p.script.calls[p.role]
	if p.role == TeamRolePlanner || p.role == TeamRoleTester || p.role == TeamRoleReviewer {
		for _, definition := range request.Tools {
			switch definition.Function.Name {
			case "write_file", "apply_patch", "copy_file", "delete_file", "run_command", "terminal":
				p.script.readOnlyToolLeak = true
			}
		}
	}
	needsChange := p.script.reviewerNeedsChange
	implementerErr := p.script.implementerErr
	p.script.mu.Unlock()
	if p.role == p.script.blockRole {
		if p.script.blockStarted != nil {
			close(p.script.blockStarted)
		}
		events := make(chan protocol.StreamEvent)
		go func() {
			<-ctx.Done()
			close(events)
		}()
		return events, nil
	}
	if p.role == TeamRoleImplementer && implementerErr != nil {
		return nil, implementerErr
	}

	events := make(chan protocol.StreamEvent, 4)
	go func() {
		defer close(events)
		switch p.role {
		case TeamRolePlanner:
			events <- protocol.StreamEvent{Type: "delta", Delta: "1. 检查现有实现\n2. 完成修改\n3. 核验结果"}
		case TeamRoleImplementer:
			revision := requestContains(request, "正在进行审阅后的修订")
			if lastMessageRole(request) != "tool" {
				if revision {
					events <- protocol.StreamEvent{Type: "tool_calls", ToolCalls: []protocol.ToolCall{{
						ID: "patch-generated", Type: "function", Function: protocol.ToolCallFunction{
							Name: "apply_patch", Arguments: json.RawMessage(`{"path":"generated.txt","old_string":"first","new_string":"fixed"}`),
						},
					}}}
				} else {
					events <- protocol.StreamEvent{Type: "tool_calls", ToolCalls: []protocol.ToolCall{{
						ID: "write-generated", Type: "function", Function: protocol.ToolCallFunction{
							Name: "write_file", Arguments: json.RawMessage(`{"path":"generated.txt","content":"first\n"}`),
						},
					}}}
				}
				return
			}
			if revision {
				events <- protocol.StreamEvent{Type: "delta", Delta: "已按审阅反馈修订 generated.txt。"}
			} else {
				events <- protocol.StreamEvent{Type: "delta", Delta: "已创建 generated.txt 并完成实现。"}
			}
		case TeamRoleTester:
			events <- protocol.StreamEvent{Type: "delta", Delta: "VERDICT: APPROVED\n结构化写入结果可读取，未发现回归。"}
		case TeamRoleReviewer:
			if needsChange && call == 1 {
				events <- protocol.StreamEvent{Type: "delta", Delta: "VERDICT: CHANGES_REQUIRED\ngenerated.txt 应将 first 改为 fixed。"}
			} else {
				events <- protocol.StreamEvent{Type: "delta", Delta: "VERDICT: APPROVED\n实现与用户目标一致。"}
			}
		case TeamRoleSynthesizer:
			events <- protocol.StreamEvent{Type: "delta", Delta: "团队已完成实现、测试与审阅。"}
		}
		events <- protocol.StreamEvent{Type: "usage", Usage: &protocol.TokenUsage{PromptTokens: 100, CompletionTokens: 20}}
	}()
	return events, nil
}

func TestTeamModeRunsRolesWithReadOnlyReviewers(t *testing.T) {
	svc, workspace, script := newTeamTestService(t, false)
	var events []ChatStreamEvent
	result, err := svc.SendChatMessageWithEvents(context.Background(), "创建一个验证文件", func(event ChatStreamEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "generated.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(data), "\r\n", "\n") != "first\n" {
		t.Fatalf("generated content = %q", data)
	}
	if result.Content != "团队已完成实现、测试与审阅。" || result.State.Team.Status != "completed" || result.State.Team.Active {
		t.Fatalf("team result = %#v", result)
	}
	if script.readOnlyToolLeak {
		t.Fatal("planner/tester/reviewer received a mutating tool")
	}
	for _, role := range teamRoleOrder {
		if script.calls[role] == 0 {
			t.Fatalf("role %s was not called: %#v", role, script.calls)
		}
	}
	teamParts := 0
	for _, part := range result.Parts {
		if part.Kind == "team_role" {
			teamParts++
		}
	}
	if teamParts != 5 {
		t.Fatalf("team parts = %d, parts=%#v", teamParts, result.Parts)
	}
	persistedTeamPart := false
	for _, message := range svc.GetSessionMessages() {
		for _, part := range message.Parts {
			if part.Kind == "team_role" && part.Role == TeamRoleReviewer && part.Model == "model-reviewer" && part.Verdict == "approved" {
				persistedTeamPart = true
			}
		}
	}
	if !persistedTeamPart {
		t.Fatal("team role metadata was not restored from the event log")
	}
	teamEvents := 0
	for _, event := range events {
		if event.Type == "team" && event.Team != nil {
			teamEvents++
		}
	}
	if teamEvents < 10 {
		t.Fatalf("team events = %d", teamEvents)
	}
}

func TestTeamModeRevisesAfterReviewerFeedback(t *testing.T) {
	svc, workspace, script := newTeamTestService(t, true)
	result, err := svc.SendChatMessage(context.Background(), "创建并审阅验证文件")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "generated.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(data), "\r\n", "\n") != "fixed\n" {
		t.Fatalf("revision content = %q", data)
	}
	if script.calls[TeamRoleReviewer] != 2 || script.calls[TeamRoleTester] != 2 {
		t.Fatalf("verification calls = %#v", script.calls)
	}
	implementerParts := 0
	for _, part := range result.Parts {
		if part.Kind == "team_role" && part.Role == TeamRoleImplementer {
			implementerParts++
		}
	}
	if implementerParts != 2 {
		t.Fatalf("implementer attempts = %d", implementerParts)
	}
}

func TestTeamPlanRejectionDoesNotRunImplementer(t *testing.T) {
	svc, workspace, script := newTeamTestService(t, false)
	svc.planMode = true
	svc.SetApprovalNotify(func(request ApprovalRequest) {
		go func() { _ = svc.RespondApproval(request.ID, request.Tool, false, "once") }()
	})
	result, err := svc.SendChatMessage(context.Background(), "先规划再创建文件")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "选择暂不执行") || result.State.Team.Status != "cancelled" {
		t.Fatalf("rejected result = %#v", result)
	}
	if result.State.PlanState.Status != "cancelled" {
		t.Fatalf("rejected plan state = %#v", result.State.PlanState)
	}
	if script.calls[TeamRoleImplementer] != 0 {
		t.Fatalf("implementer calls = %d", script.calls[TeamRoleImplementer])
	}
	if _, err := os.Stat(filepath.Join(workspace, "generated.txt")); !os.IsNotExist(err) {
		t.Fatalf("plan rejection wrote a file: %v", err)
	}
}

func TestTeamPlanApprovalCompletesPlanAndImplementation(t *testing.T) {
	svc, workspace, script := newTeamTestService(t, false)
	svc.planMode = true
	svc.SetApprovalNotify(func(request ApprovalRequest) {
		go func() { _ = svc.RespondApproval(request.ID, request.Tool, true, "once") }()
	})
	result, err := svc.SendChatMessage(context.Background(), "按团队计划创建文件")
	if err != nil {
		t.Fatal(err)
	}
	if result.State.PlanState.Status != "completed" || len(result.State.PlanState.Steps) != 3 {
		t.Fatalf("completed plan state = %#v", result.State.PlanState)
	}
	for _, step := range result.State.PlanState.Steps {
		if step.Status != "completed" {
			t.Fatalf("unfinished team plan step = %#v", step)
		}
	}
	if script.calls[TeamRoleImplementer] == 0 {
		t.Fatal("approved team plan did not run implementer")
	}
	if _, err := os.Stat(filepath.Join(workspace, "generated.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestTeamModeLowReasoningRejectsBeforeAnyRoleRuns(t *testing.T) {
	svc, workspace, script := newTeamTestService(t, false)
	svc.reasoning = ReasoningLow
	_, err := svc.SendChatMessage(context.Background(), "低推理下不应运行团队")
	if !errors.Is(err, errTeamModeRequiresPlanner) {
		t.Fatalf("team error = %v", err)
	}
	if len(script.calls) != 0 {
		t.Fatalf("team roles ran before rejection: %#v", script.calls)
	}
	if _, err := os.Stat(filepath.Join(workspace, "generated.txt")); !os.IsNotExist(err) {
		t.Fatalf("rejected team mode wrote a file: %v", err)
	}
}

func TestTeamModeCancellationStopsBeforeNextRole(t *testing.T) {
	svc, _, script := newTeamTestService(t, false)
	started := make(chan struct{})
	script.blockRole = TeamRolePlanner
	script.blockStarted = started
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := svc.SendChatMessage(ctx, "取消团队任务")
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("planner did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("team cancellation error = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("team cancellation did not return promptly")
	}
	script.mu.Lock()
	implementerCalls := script.calls[TeamRoleImplementer]
	script.mu.Unlock()
	if implementerCalls != 0 {
		t.Fatalf("implementer started after cancellation: %d calls", implementerCalls)
	}
	if state := svc.WorkbenchState().Team; state.Status != "cancelled" || state.Active {
		t.Fatalf("team state after cancellation = %#v", state)
	}
}

func TestTeamPlanMarksPlanAndTeamFailedWhenImplementerFails(t *testing.T) {
	svc, workspace, script := newTeamTestService(t, false)
	svc.planMode = true
	script.implementerErr = errors.New("implementer unavailable")
	svc.SetApprovalNotify(func(request ApprovalRequest) {
		go func() { _ = svc.RespondApproval(request.ID, request.Tool, true, "once") }()
	})

	_, err := svc.SendChatMessage(context.Background(), "按团队计划执行但模拟失败")
	if err == nil || !strings.Contains(err.Error(), "implementer unavailable") {
		t.Fatalf("team execution error = %v", err)
	}
	state := svc.WorkbenchState()
	if state.PlanState.Status != "failed" || state.Team.Status != "failed" {
		t.Fatalf("failed team execution left non-terminal state: plan=%#v team=%#v", state.PlanState, state.Team)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "generated.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("failed implementer unexpectedly wrote a file: %v", statErr)
	}
}

func TestGuidanceTurnDoesNotRestartTeamOrchestration(t *testing.T) {
	svc, _, script := newTeamTestService(t, false)
	result, err := svc.SendChatGuidanceWithAttachmentsAndEvents(context.Background(), "调整当前实现", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "model-implementer" {
		t.Fatalf("guidance model = %q", result.Model)
	}
	if script.calls[TeamRoleImplementer] == 0 {
		t.Fatal("guidance did not run the current implementation model")
	}
	for _, role := range []string{TeamRolePlanner, TeamRoleTester, TeamRoleReviewer, TeamRoleSynthesizer} {
		if script.calls[role] != 0 {
			t.Fatalf("guidance restarted team role %s: %#v", role, script.calls)
		}
	}
}

func newTeamTestService(t *testing.T, reviewerNeedsChange bool) (*Service, string, *teamProviderScript) {
	t.Helper()
	workspace := t.TempDir()
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	svc.reasoning = ReasoningUltra
	svc.runtimeSettings.WorkspaceRoot = workspace
	svc.runtimeSettings.FilesystemAccess = "workspace-write"
	svc.runtimeSettings.SandboxMode = "workspace-write"
	svc.runtimeSettings.ApprovalPolicy = "never"
	models := make([]ProviderModel, 0, len(teamRoleOrder))
	roles := make([]TeamRoleSetting, 0, len(teamRoleOrder))
	for _, role := range teamRoleOrder {
		models = append(models, ProviderModel{ID: "model-" + role, DisplayName: role, Provider: "team-local", ContextWindowTokens: 128000})
		roles = append(roles, TeamRoleSetting{Role: role, Enabled: true, ProviderID: "team-local", ModelID: "model-" + role})
	}
	svc.runtimeSettings.Model = ModelSettings{
		SelectedProviderID: "team-local",
		SelectedModelID:    "model-implementer",
		Providers: []ModelProviderSetting{{
			ID: "team-local", Name: "Team Local", Protocol: "local", APIType: "chat-completions",
			BaseURL: "http://127.0.0.1:11434/v1", Enabled: true, DefaultModelID: "model-implementer", Models: models,
		}},
	}
	svc.runtimeSettings.Team = TeamSettings{Enabled: true, MaxReviewRounds: 1, Roles: roles}
	script := &teamProviderScript{calls: map[string]int{}, reviewerNeedsChange: reviewerNeedsChange}
	svc.providerFactory = func(route chatRoute) (protocol.Provider, error) {
		role := strings.TrimPrefix(route.ModelID, "model-")
		if !isTeamRole(role) {
			return nil, errors.New("unexpected team model: " + route.ModelID)
		}
		return scriptedTeamProvider{role: role, script: script}, nil
	}
	return svc, workspace, script
}

func requestContains(request protocol.ChatRequest, needle string) bool {
	for _, message := range request.Messages {
		if strings.Contains(message.Content, needle) {
			return true
		}
	}
	return false
}

func lastMessageRole(request protocol.ChatRequest) string {
	if len(request.Messages) == 0 {
		return ""
	}
	return request.Messages[len(request.Messages)-1].Role
}
