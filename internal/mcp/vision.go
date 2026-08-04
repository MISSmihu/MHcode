package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/MISSmihu/MHcode/internal/tools"
)

var ErrNoVisionToolConfigured = errors.New("未配置可用的 MCP 视觉工具")

type VisionRequest struct {
	Name     string
	MIMEType string
	Data     string
	Prompt   string
}

type VisionResponse struct {
	ServerID string
	ToolName string
	Summary  string
	Result   tools.Result
}

type visionCandidate struct {
	serverID  string
	toolName  string
	display   string
	transport string
	config    VisionToolConfig
}

func (m *Manager) AnalyzeImage(ctx context.Context, request VisionRequest) (VisionResponse, error) {
	if m == nil {
		return VisionResponse{}, ErrNoVisionToolConfigured
	}
	request.Name = strings.TrimSpace(request.Name)
	request.MIMEType = strings.ToLower(strings.TrimSpace(request.MIMEType))
	request.Data = strings.TrimSpace(request.Data)
	request.Prompt = strings.TrimSpace(request.Prompt)
	if !strings.HasPrefix(request.MIMEType, "image/") || request.Data == "" {
		return VisionResponse{}, errors.New("MCP 视觉请求缺少有效图片")
	}

	m.mu.RLock()
	candidates := make([]visionCandidate, 0)
	for _, serverID := range m.order {
		server := m.servers[serverID]
		if server == nil || server.status.State != "ready" || server.session == nil || !server.config.Vision.Enabled {
			continue
		}
		toolName := strings.TrimSpace(server.config.Vision.ToolName)
		if toolName == "" || server.tools[toolName] == nil {
			continue
		}
		candidates = append(candidates, visionCandidate{
			serverID: serverID, toolName: toolName, display: namespacedToolName(serverID, toolName),
			transport: server.config.Transport, config: server.config.Vision,
		})
	}
	m.mu.RUnlock()
	if len(candidates) == 0 {
		return VisionResponse{}, ErrNoVisionToolConfigured
	}

	failures := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if isRemoteTransport(candidate.transport) && !candidate.config.AllowRemoteImages {
			failures = append(failures, fmt.Sprintf("%s 未允许上传图片到远程 MCP", candidate.serverID))
			continue
		}
		arguments := map[string]any{}
		imageValue := request.Data
		if candidate.config.InputMode != "base64" {
			imageValue = "data:" + request.MIMEType + ";base64," + request.Data
		}
		arguments[candidate.config.ImageArgument] = imageValue
		if candidate.config.PromptArgument != "" {
			arguments[candidate.config.PromptArgument] = request.Prompt
		}
		if candidate.config.MIMETypeArgument != "" {
			arguments[candidate.config.MIMETypeArgument] = request.MIMEType
		}
		if candidate.config.FileNameArgument != "" {
			arguments[candidate.config.FileNameArgument] = request.Name
		}
		rawArguments, err := json.Marshal(arguments)
		if err != nil {
			failures = append(failures, candidate.serverID+": "+err.Error())
			continue
		}
		displayInput := request.Name
		if displayInput == "" {
			displayInput = request.MIMEType
		} else {
			displayInput += " · " + request.MIMEType
		}
		result, err := m.callToolWithDisplay(
			ctx, candidate.serverID, candidate.toolName, candidate.display,
			arguments, rawArguments, "", displayInput,
		)
		if err != nil {
			failures = append(failures, candidate.serverID+": "+err.Error())
			continue
		}
		if result.IsError {
			failure := strings.TrimSpace(result.Summary)
			emitVisionCandidateFailure(ctx, candidate, displayInput, failure, result)
			failures = append(failures, candidate.serverID+": "+failure)
			continue
		}
		summary := strings.TrimSpace(result.Summary)
		if !hasUsableVisionText(summary) {
			failure := "视觉工具没有返回文字或结构化分析"
			emitVisionCandidateFailure(ctx, candidate, displayInput, failure, result)
			failures = append(failures, candidate.serverID+": "+failure)
			continue
		}
		return VisionResponse{
			ServerID: candidate.serverID, ToolName: candidate.display, Summary: truncateText(summary, 24*1024), Result: result,
		}, nil
	}
	if len(failures) == 0 {
		return VisionResponse{}, ErrNoVisionToolConfigured
	}
	return VisionResponse{}, fmt.Errorf("MCP 视觉分析失败: %s", strings.Join(failures, "; "))
}

func emitVisionCandidateFailure(ctx context.Context, candidate visionCandidate, input, message string, result tools.Result) {
	parts := result.Parts
	if len(parts) == 0 {
		parts = []tools.ResultPart{{Kind: tools.PartToolCall}}
	}
	for _, part := range parts {
		if strings.TrimSpace(part.Name) == "" {
			part.Name = candidate.display
		}
		if strings.TrimSpace(part.Input) == "" {
			part.Input = input
		}
		part.Status = "error"
		part.Output = message
		tools.EmitProgress(ctx, part)
	}
}

func (m *Manager) IsVisionTool(displayName string) bool {
	if m == nil {
		return false
	}
	displayName = strings.TrimSpace(displayName)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, serverID := range m.order {
		server := m.servers[serverID]
		if server == nil || !server.config.Vision.Enabled {
			continue
		}
		if namespacedToolName(serverID, server.config.Vision.ToolName) == displayName {
			return true
		}
	}
	return false
}

func isRemoteTransport(transport string) bool {
	return transport == TransportStreamableHTTP || transport == TransportSSE
}

func hasUsableVisionText(summary string) bool {
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "image:") || strings.HasPrefix(line, "image resource:") || strings.HasPrefix(line, "audio:") || strings.HasPrefix(line, "resource:") {
			continue
		}
		return true
	}
	return false
}
