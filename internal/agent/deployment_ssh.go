package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

const deploymentSSHPreflightCallID = "host-ssh-preflight"

type deploymentSSHPreflight struct {
	Attempted    bool
	Succeeded    bool
	CredentialID string
	Call         protocol.ToolCall
	Result       tools.Result
	ToolMessage  protocol.Message
}

func (s *Service) runDeploymentSSHPreflight(
	ctx context.Context,
	reg *tools.Registry,
	messages []protocol.Message,
	sink ChatEventSink,
) deploymentSSHPreflight {
	credentialID, ok := s.deploymentSSHCredential(messages)
	if !ok || ctx.Err() != nil {
		return deploymentSSHPreflight{}
	}
	arguments, _ := json.Marshal(sshToolArguments{
		Action:       "test",
		CredentialID: scopedCredentialScheme + credentialID,
	})
	call := protocol.ToolCall{
		ID:   deploymentSSHPreflightCallID,
		Type: "function",
		Function: protocol.ToolCallFunction{
			Name:      "ssh",
			Arguments: arguments,
		},
	}
	input := toolInputForDisplay("ssh", arguments)
	emitChatEvent(sink, ChatStreamEvent{
		Type:       "tool",
		Message:    "正在验证 SSH 连接",
		ToolName:   "ssh",
		ToolCallID: call.ID,
		ToolInput:  input,
		Status:     "running",
	})
	toolCtx := tools.WithProgressSink(ctx, func(part tools.ResultPart) {
		part.Name = "ssh"
		part.ToolCallID = call.ID
		if part.Input == "" {
			part.Input = input
		}
		part.Status = "running"
		emitChatEvent(sink, ChatStreamEvent{
			Type:       "tool",
			Message:    "正在验证 SSH 连接",
			ToolName:   "ssh",
			ToolCallID: call.ID,
			ToolInput:  input,
			Status:     "running",
			Parts:      []tools.ResultPart{part},
		})
	})
	result, toolMessage := s.executeToolCall(toolCtx, reg, call)
	status := "completed"
	if result.IsError {
		status = "error"
	}
	emitChatEvent(sink, ChatStreamEvent{
		Type:       "tool",
		Message:    result.Summary,
		ToolName:   "ssh",
		ToolCallID: call.ID,
		ToolInput:  input,
		Status:     status,
		Parts:      result.Parts,
	})
	return deploymentSSHPreflight{
		Attempted:    true,
		Succeeded:    !result.IsError,
		CredentialID: credentialID,
		Call:         call,
		Result:       result,
		ToolMessage:  toolMessage,
	}
}

func appendDeploymentSSHPreflight(messages []protocol.Message, preflight deploymentSSHPreflight) []protocol.Message {
	result := append([]protocol.Message{}, messages...)
	if !preflight.Attempted {
		return result
	}
	result = append(result,
		protocol.Message{
			Role:         "assistant",
			ToolCalls:    []protocol.ToolCall{preflight.Call},
			InternalKind: "deployment-ssh-preflight",
		},
		preflight.ToolMessage,
	)
	result[len(result)-1].InternalKind = "deployment-ssh-preflight"
	return result
}

func appendDeploymentSSHPreflightSummary(messages []protocol.Message, preflight deploymentSSHPreflight) []protocol.Message {
	result := append([]protocol.Message{}, messages...)
	if !preflight.Attempted {
		return result
	}
	status := "failed"
	if preflight.Succeeded {
		status = "succeeded"
	}
	content := strings.TrimSpace(preflight.ToolMessage.Content)
	if content == "" {
		content = strings.TrimSpace(preflight.Result.Summary)
	}
	result = append(result, protocol.Message{
		Role:         "user",
		Content:      "[MHcode host SSH preflight " + status + "]\n" + content + "\nUse this completed host check when planning. Do not repeat ssh test.",
		InternalKind: "deployment-ssh-preflight",
	})
	return result
}

func (s *Service) deploymentSSHCredential(messages []protocol.Message) (string, bool) {
	requestIndex, request := latestUserMessage(messages)
	if requestIndex < 0 || !deploymentRequest(messages, requestIndex, request) {
		return "", false
	}

	current := validSSHCredentialIDs(request, s.resolveScopedSSHCredential)
	if len(current) == 1 {
		return current[0], true
	}
	if len(current) > 1 {
		return "", false
	}

	all := make([]string, 0, 2)
	seen := map[string]bool{}
	for _, message := range messages {
		for _, id := range validSSHCredentialIDs(message.Content, s.resolveScopedSSHCredential) {
			if !seen[id] {
				seen[id] = true
				all = append(all, id)
			}
		}
	}
	if len(all) != 1 {
		return "", false
	}
	return all[0], true
}

func validSSHCredentialIDs(value string, resolve func(string) (scopedSSHCredential, error)) []string {
	seen := map[string]bool{}
	ids := make([]string, 0, 2)
	for _, reference := range scopedCredentialReferencePattern.FindAllString(value, -1) {
		id := strings.TrimPrefix(reference, scopedCredentialScheme)
		if seen[id] {
			continue
		}
		if _, err := resolve(id); err != nil {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func deploymentRequest(messages []protocol.Message, requestIndex int, request string) bool {
	if hasDeploymentIntent(request) {
		return true
	}
	if !isContinuationRequest(request) {
		return false
	}
	for index, userMessages := requestIndex-1, 0; index >= 0 && userMessages < 3; index-- {
		if messages[index].InternalKind == contextSummaryKind && hasDeploymentIntent(messages[index].Content) {
			return true
		}
		if messages[index].Role != "user" || messages[index].InternalKind != "" {
			continue
		}
		userMessages++
		previous := strings.TrimSpace(messages[index].Content)
		if previous == "" {
			continue
		}
		if hasDeploymentIntent(previous) {
			return true
		}
		if !isContinuationRequest(previous) {
			return false
		}
	}
	return false
}

func hasDeploymentIntent(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"部署", "上线", "远程运维", "服务器运维", "连接服务器", "登录服务器", "搭建服务", "搭建网站",
		"deploy", "deployment", "remote operations", "remote server", "production server", "ssh into", "ssh to",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	remoteContext := strings.Contains(lower, "服务器") || strings.Contains(lower, "远程") || strings.Contains(lower, "主机") || strings.Contains(lower, "ssh")
	if !remoteContext {
		return false
	}
	for _, marker := range []string{
		"安装", "升级", "迁移", "发布", "配置", "修复", "排障", "调试", "重启", "启动", "停止", "更新",
		"install", "upgrade", "migrate", "release", "configure", "repair", "troubleshoot", "restart", "start", "stop", "update",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isContinuationRequest(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"继续", "接着", "往下做", "继续执行", "继续处理", "继续吧", "再试", "重试",
		"continue", "resume", "keep going", "carry on", "retry", "try again",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func latestUserMessage(messages []protocol.Message) (int, string) {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" && messages[index].InternalKind == "" {
			return index, strings.TrimSpace(messages[index].Content)
		}
	}
	return -1, ""
}
