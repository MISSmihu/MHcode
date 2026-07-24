package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

type deploymentPreflightProvider struct {
	requests []protocol.ChatRequest
}

func (p *deploymentPreflightProvider) Name() string { return "deployment-preflight" }

func (p *deploymentPreflightProvider) ListModels(context.Context) ([]protocol.Model, error) {
	return []protocol.Model{{ID: "deployment-test"}}, nil
}

func (p *deploymentPreflightProvider) Complete(context.Context, protocol.ChatRequest) (protocol.CompletionResult, error) {
	return protocol.CompletionResult{}, errors.New("unexpected non-streaming completion")
}

func (p *deploymentPreflightProvider) Stream(_ context.Context, request protocol.ChatRequest) (<-chan protocol.StreamEvent, error) {
	p.requests = append(p.requests, request)
	events := make(chan protocol.StreamEvent, 2)
	events <- protocol.StreamEvent{Type: "delta", Delta: "部署准备完成"}
	close(events)
	return events, nil
}

func TestDeploymentTurnRunsSSHPreflightBeforeModel(t *testing.T) {
	const password = "deployment-test-password"
	server := startSSHTestServer(t, password)
	defer server.Close()

	service, reference := newDeploymentSSHService(t, server.Address(), password)
	messages := []protocol.Message{
		{Role: "system", Content: "test system"},
		{Role: "user", Content: "请把当前项目部署到服务器 " + reference},
	}
	service.sessionMessages = append([]protocol.Message{}, messages...)
	provider := &deploymentPreflightProvider{}
	events := make([]ChatStreamEvent, 0, 4)
	result, err := service.runToolLoopTurn(
		context.Background(),
		provider,
		provider,
		protocol.ChatRequest{Model: "deployment-test", Messages: messages},
		8,
		chatRoute{Provider: ModelProviderSetting{ID: "test", Name: "Test", Protocol: "openai-compatible"}, ModelID: "deployment-test"},
		requestPrefixDiagnostic{},
		messages,
		1,
		func(event ChatStreamEvent) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "部署准备完成" || len(provider.requests) != 1 {
		t.Fatalf("result=%#v requests=%d", result, len(provider.requests))
	}
	request := provider.requests[0]
	if !hasPreflightToolExchange(request.Messages, "ok") {
		t.Fatalf("model request did not contain successful SSH preflight: %#v", request.Messages)
	}
	if !hasSSHPreflightEvent(events, "completed") {
		t.Fatalf("SSH preflight was not visible in timeline events: %#v", events)
	}
	encoded, _ := json.Marshal(struct {
		Request protocol.ChatRequest
		Events  []ChatStreamEvent
		Result  ChatResult
	}{request, events, result})
	if strings.Contains(string(encoded), password) {
		t.Fatalf("deployment preflight leaked SSH password: %s", encoded)
	}
}

func TestDeploymentSSHCredentialRestoresForContinueOnly(t *testing.T) {
	service, reference := newDeploymentSSHService(t, "127.0.0.1:22", "continue-password")
	history := []protocol.Message{
		{Role: "system", Content: "test"},
		{Role: "user", Content: "把网站部署到服务器 " + reference},
		{Role: "assistant", Content: "部署已开始"},
		{Role: "user", Content: "继续"},
	}
	credentialID, ok := service.deploymentSSHCredential(history)
	if !ok || scopedCredentialScheme+credentialID != reference {
		t.Fatalf("continue did not restore deployment credential: id=%q ok=%v", credentialID, ok)
	}
	history = append(history, protocol.Message{Role: "assistant", Content: "连接中断"}, protocol.Message{Role: "user", Content: "继续吧"})
	credentialID, ok = service.deploymentSSHCredential(history)
	if !ok || scopedCredentialScheme+credentialID != reference {
		t.Fatalf("repeated continue did not restore deployment credential: id=%q ok=%v", credentialID, ok)
	}
	compressed := []protocol.Message{
		{Role: "system", Content: "test " + reference},
		{Role: "system", InternalKind: contextSummaryKind, Content: "User requested deployment to the authorized server."},
		{Role: "user", Content: "继续"},
	}
	credentialID, ok = service.deploymentSSHCredential(compressed)
	if !ok || scopedCredentialScheme+credentialID != reference {
		t.Fatalf("compressed deployment context did not restore credential: id=%q ok=%v", credentialID, ok)
	}
	history[len(history)-1].Content = "解释一下这段代码"
	if _, ok := service.deploymentSSHCredential(history); ok {
		t.Fatal("non-deployment follow-up unexpectedly restored SSH credential")
	}
}

func TestDeploymentSSHPreflightRequiresIntentAndCredential(t *testing.T) {
	service, reference := newDeploymentSSHService(t, "127.0.0.1:22", "intent-password")
	cases := []struct {
		name     string
		messages []protocol.Message
	}{
		{name: "no credential", messages: []protocol.Message{{Role: "user", Content: "请部署当前项目"}}},
		{name: "no deployment intent", messages: []protocol.Message{{Role: "user", Content: "解释当前项目 " + reference}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, ok := service.deploymentSSHCredential(testCase.messages); ok {
				t.Fatalf("unexpected SSH preflight for %#v", testCase.messages)
			}
		})
	}
}

func TestDeploymentSSHPreflightFailureAndApprovalRejectionReachModel(t *testing.T) {
	const password = "correct-password"
	server := startSSHTestServer(t, password)
	defer server.Close()

	t.Run("authentication failure", func(t *testing.T) {
		service, reference := newDeploymentSSHService(t, server.Address(), "wrong-password")
		messages := []protocol.Message{{Role: "user", Content: "部署网站到服务器 " + reference}}
		preflight := service.runDeploymentSSHPreflight(context.Background(), service.buildToolRegistry(), messages, nil)
		if !preflight.Attempted || preflight.Succeeded || !preflight.Result.IsError {
			t.Fatalf("preflight = %#v", preflight)
		}
		injected := appendDeploymentSSHPreflight(messages, preflight)
		if !hasPreflightToolExchange(injected, "error") {
			t.Fatalf("failed preflight was not injected: %#v", injected)
		}
	})

	t.Run("approval rejected", func(t *testing.T) {
		service, reference := newDeploymentSSHService(t, server.Address(), password)
		service.runtimeSettings.ApprovalPolicy = "on-request"
		service.SetApprovalNotify(func(request ApprovalRequest) {
			go func() { _ = service.RespondApproval(request.ID, request.Tool, false, "once") }()
		})
		messages := []protocol.Message{{Role: "user", Content: "部署网站到服务器 " + reference}}
		preflight := service.runDeploymentSSHPreflight(context.Background(), service.buildToolRegistry(), messages, nil)
		if !preflight.Attempted || preflight.Succeeded || !strings.Contains(preflight.ToolMessage.Content, "用户拒绝") {
			t.Fatalf("preflight = %#v", preflight)
		}
	})
}

func TestToolLoopGuardDeduplicatesOnlySuccessfulSSHTestForDeployment(t *testing.T) {
	testCall := protocol.ToolCall{
		ID: "ssh-test",
		Function: protocol.ToolCallFunction{
			Name:      "ssh",
			Arguments: json.RawMessage(`{"action":"test","credential_id":"ssh-test"}`),
		},
	}
	runCall := protocol.ToolCall{
		ID: "ssh-run",
		Function: protocol.ToolCallFunction{
			Name:      "ssh",
			Arguments: json.RawMessage(`{"action":"run","credential_id":"ssh-test","command":"systemctl status app"}`),
		},
	}
	guard := toolLoopGuard{completedSSHCalls: map[string]bool{}}
	guard.after(testCall, successfulSSHGuardResult(), &protocol.Message{})
	if _, _, guarded, hidden := guard.before(testCall); !guarded || !hidden || guard.forceFinalResponse {
		t.Fatalf("successful SSH test was not deduplicated correctly: guarded=%v hidden=%v forceFinal=%v", guarded, hidden, guard.forceFinalResponse)
	}
	guard.after(runCall, successfulSSHGuardResult(), &protocol.Message{})
	if _, _, guarded, _ := guard.before(runCall); guarded {
		t.Fatal("deployment command was incorrectly deduplicated")
	}
}

func TestCredentialLookupPlanUpdateLimitIsFour(t *testing.T) {
	guard := toolLoopGuard{remoteLookup: true, maxPlanUpdates: maxLookupPlanUpdates}
	for index := 0; index < maxLookupPlanUpdates; index++ {
		call := protocol.ToolCall{
			ID: "plan-" + strconv.Itoa(index),
			Function: protocol.ToolCallFunction{
				Name:      "update_plan",
				Arguments: json.RawMessage(`{"steps":[{"title":"step ` + strconv.Itoa(index) + `","status":"in_progress"}]}`),
			},
		}
		if _, _, guarded, _ := guard.before(call); guarded {
			t.Fatalf("plan update %d was rejected too early", index)
		}
		guard.after(call, successfulSSHGuardResult(), &protocol.Message{})
	}
	overLimit := protocol.ToolCall{
		ID: "plan-over-limit",
		Function: protocol.ToolCallFunction{
			Name:      "update_plan",
			Arguments: json.RawMessage(`{"steps":[{"title":"extra","status":"in_progress"}]}`),
		},
	}
	if _, _, guarded, hidden := guard.before(overLimit); !guarded || !hidden || !guard.forceFinalResponse {
		t.Fatalf("fifth lookup plan update was not stopped: guarded=%v hidden=%v forceFinal=%v", guarded, hidden, guard.forceFinalResponse)
	}
}

func newDeploymentSSHService(t *testing.T, address, password string) (*Service, string) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		SkillsDir:    t.TempDir(),
		SettingsPath: filepath.Join(t.TempDir(), "runtime-settings.json"),
	})
	service.projectID = "project-deployment"
	service.sessionID = "session-deployment"
	settings := DefaultRuntimeSettings()
	settings.WorkspaceRoot = t.TempDir()
	settings.FilesystemAccess = "workspace-write"
	settings.SandboxMode = "workspace-write"
	settings.NetworkAccess = true
	settings.ShellAccess = true
	settings.ApprovalPolicy = "never"
	service.runtimeSettings = settings.Normalized()
	credential := scopedSSHCredential{
		ID:        scopedSSHCredentialID(service.projectID, service.sessionID, host, port, "root"),
		Kind:      "ssh_password",
		Host:      host,
		Port:      port,
		Username:  "root",
		Password:  password,
		ProjectID: service.projectID,
		SessionID: service.sessionID,
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.secretVault.Set(scopedCredentialServiceName, credential.ID, string(encoded)); err != nil {
		t.Fatal(err)
	}
	return service, scopedCredentialScheme + credential.ID
}

func hasPreflightToolExchange(messages []protocol.Message, expectedStatus string) bool {
	for index := 0; index+1 < len(messages); index++ {
		assistant := messages[index]
		toolMessage := messages[index+1]
		if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != deploymentSSHPreflightCallID {
			continue
		}
		if toolMessage.Role != "tool" || toolMessage.ToolCallID != deploymentSSHPreflightCallID || toolMessage.Name != "ssh" {
			return false
		}
		if expectedStatus == "error" {
			return strings.Contains(strings.ToLower(toolMessage.Content), "error") || strings.Contains(toolMessage.Content, "失败") || strings.Contains(toolMessage.Content, "拒绝") || strings.Contains(toolMessage.Content, "unable") || strings.Contains(toolMessage.Content, "handshake")
		}
		return strings.Contains(toolMessage.Content, "exit code 0")
	}
	return false
}

func hasSSHPreflightEvent(events []ChatStreamEvent, status string) bool {
	for _, event := range events {
		if event.ToolName == "ssh" && event.ToolCallID == deploymentSSHPreflightCallID && event.Status == status {
			return true
		}
	}
	return false
}

func successfulSSHGuardResult() tools.Result {
	return tools.Result{Summary: "ok"}
}
