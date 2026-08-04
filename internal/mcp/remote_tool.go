package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"path"
	"strings"

	"github.com/MISSmihu/MHcode/internal/tools"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type RemoteTool struct {
	manager           *Manager
	serverID          string
	remoteName        string
	name              string
	description       string
	schema            map[string]any
	readOnly          bool
	workspaceRoot     string
	injectProjectPath bool
}

func (t *RemoteTool) Name() string { return t.name }

func (t *RemoteTool) Description() string {
	if t.description == "" {
		return fmt.Sprintf("MCP tool %s from server %s", t.remoteName, t.serverID)
	}
	return fmt.Sprintf("[%s] %s", t.serverID, t.description)
}

func (t *RemoteTool) InputSchema() map[string]any { return t.schema }

func (t *RemoteTool) ReadOnly() bool { return t.readOnly }

func (t *RemoteTool) Execute(ctx context.Context, rawArgs json.RawMessage) (tools.Result, error) {
	if t.manager == nil {
		return tools.Result{}, errors.New("MCP manager 不可用")
	}
	arguments, effectiveArgs, workingDirectory, err := t.argumentsForCall(rawArgs)
	if err != nil {
		return tools.Result{}, err
	}
	return t.manager.callTool(ctx, t.serverID, t.remoteName, t.name, arguments, effectiveArgs, workingDirectory)
}

func (t *RemoteTool) argumentsForCall(rawArgs json.RawMessage) (map[string]any, json.RawMessage, string, error) {
	var arguments map[string]any
	if len(rawArgs) == 0 {
		arguments = map[string]any{}
	} else if err := json.Unmarshal(rawArgs, &arguments); err != nil {
		return nil, nil, "", fmt.Errorf("MCP 工具参数不是有效 JSON 对象: %w", err)
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	if t.injectProjectPath && t.workspaceRoot != "" {
		if _, exists := arguments["projectPath"]; !exists {
			arguments["projectPath"] = t.workspaceRoot
		}
	}
	effectiveArgs, err := json.Marshal(arguments)
	if err != nil {
		return nil, nil, "", fmt.Errorf("编码 MCP 工具参数失败: %w", err)
	}
	workingDirectory := ""
	if t.injectProjectPath {
		workingDirectory, _ = arguments["projectPath"].(string)
		workingDirectory = strings.TrimSpace(workingDirectory)
	}
	return arguments, effectiveArgs, workingDirectory, nil
}

func (m *Manager) callTool(ctx context.Context, serverID, remoteName, displayName string, arguments map[string]any, rawArgs json.RawMessage, workingDirectory string) (tools.Result, error) {
	return m.callToolWithDisplay(ctx, serverID, remoteName, displayName, arguments, rawArgs, workingDirectory, "")
}

func (m *Manager) callToolWithDisplay(ctx context.Context, serverID, remoteName, displayName string, arguments map[string]any, rawArgs json.RawMessage, workingDirectory, displayInput string) (tools.Result, error) {
	m.mu.RLock()
	server := m.servers[serverID]
	if server == nil || server.session == nil || server.status.State != "ready" {
		m.mu.RUnlock()
		return tools.Result{}, fmt.Errorf("MCP 服务器 %s 未连接，请刷新服务器后重试", serverID)
	}
	session := server.session
	resultPolicy := server.config.ToolResultPolicy
	m.mu.RUnlock()
	input := strings.TrimSpace(displayInput)
	if input == "" {
		input = remoteToolInputForDisplay(remoteName, arguments)
	}
	if input == "" {
		input = truncateText(string(rawArgs), 1200)
	}
	tools.EmitProgress(ctx, tools.ResultPart{
		Kind:             tools.PartToolCall,
		Name:             displayName,
		Status:           "waiting",
		Input:            input,
		Output:           fmt.Sprintf("正在等待 MCP 服务器 %s 返回结果", serverID),
		WorkingDirectory: workingDirectory,
	})
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: remoteName, Arguments: arguments})
	if err != nil {
		m.mu.Lock()
		if m.servers[serverID] == server {
			server.status.Message = truncateText(err.Error(), 1200)
		}
		m.mu.Unlock()
		tools.EmitProgress(ctx, tools.ResultPart{
			Kind:             tools.PartToolCall,
			Name:             displayName,
			Status:           "error",
			Input:            input,
			Output:           truncateText(err.Error(), 1200),
			WorkingDirectory: workingDirectory,
		})
		return tools.Result{}, err
	}
	summary := summarizeCallToolResult(result, resultPolicy)
	attachments, attachmentWarnings := callToolResultAttachments(result)
	if len(attachmentWarnings) > 0 {
		summary = strings.TrimSpace(summary + "\n" + strings.Join(attachmentWarnings, "\n"))
	}
	status := "ok"
	if result.IsError {
		status = "error"
	}
	return tools.Result{
		Summary: summary,
		IsError: result.IsError,
		Parts: []tools.ResultPart{{
			Kind:             tools.PartToolCall,
			Name:             displayName,
			Status:           status,
			Input:            input,
			Output:           summary,
			WorkingDirectory: workingDirectory,
		}},
		Attachments: attachments,
	}, nil
}

func remoteToolInputForDisplay(remoteName string, arguments map[string]any) string {
	name := strings.ToLower(strings.TrimSpace(remoteName))
	keys := []string{"query", "symbol", "file", "path", "url", "action", "name"}
	if name == "codegraph_node" {
		keys = []string{"symbol", "file"}
	} else if name == "codegraph_files" {
		keys = []string{"path", "pattern"}
	} else if name == "codegraph_status" {
		return "检查代码索引状态"
	}
	for _, key := range keys {
		if value, ok := arguments[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	for _, value := range arguments {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if strings.HasPrefix(strings.ToLower(text), "data:image/") || looksLikeLargeBase64(text) {
			return "包含图片或二进制数据（已隐藏）"
		}
	}
	return ""
}

const maxMCPImageAttachmentBytes = 8 * 1024 * 1024

func callToolResultAttachments(result *sdkmcp.CallToolResult) ([]tools.Attachment, []string) {
	if result == nil {
		return nil, nil
	}
	attachments := make([]tools.Attachment, 0, len(result.Content))
	warnings := make([]string, 0)
	imageIndex := 0
	for _, content := range result.Content {
		var name, mimeType string
		var data []byte
		switch item := content.(type) {
		case *sdkmcp.ImageContent:
			if item == nil {
				continue
			}
			imageIndex++
			mimeType = strings.ToLower(strings.TrimSpace(item.MIMEType))
			data = item.Data
			name = generatedMCPImageName(imageIndex, mimeType)
		case *sdkmcp.EmbeddedResource:
			if item == nil || item.Resource == nil {
				continue
			}
			mimeType = strings.ToLower(strings.TrimSpace(item.Resource.MIMEType))
			if !strings.HasPrefix(mimeType, "image/") || len(item.Resource.Blob) == 0 {
				continue
			}
			imageIndex++
			data = item.Resource.Blob
			name = mcpResourceName(item.Resource.URI, imageIndex, mimeType)
		default:
			continue
		}
		if !strings.HasPrefix(mimeType, "image/") {
			warnings = append(warnings, fmt.Sprintf("MCP 返回了未声明为图片的内容 %q，已忽略", mimeType))
			continue
		}
		if len(data) == 0 {
			warnings = append(warnings, fmt.Sprintf("MCP 返回的图片 %s 为空，已忽略", name))
			continue
		}
		if len(data) > maxMCPImageAttachmentBytes {
			warnings = append(warnings, fmt.Sprintf("MCP 返回的图片 %s 超过 8 MiB，已忽略", name))
			continue
		}
		attachments = append(attachments, tools.Attachment{
			Name: name, MIMEType: mimeType, Data: base64.StdEncoding.EncodeToString(data),
		})
	}
	return attachments, warnings
}

func generatedMCPImageName(index int, mimeType string) string {
	extension := ".png"
	if extensions, err := mime.ExtensionsByType(mimeType); err == nil && len(extensions) > 0 {
		extension = extensions[0]
	}
	return fmt.Sprintf("mcp-image-%d%s", index, extension)
}

func mcpResourceName(rawURI string, index int, mimeType string) string {
	if parsed, err := url.Parse(strings.TrimSpace(rawURI)); err == nil {
		if base := strings.TrimSpace(path.Base(parsed.Path)); base != "" && base != "." && base != "/" {
			return base
		}
	}
	return generatedMCPImageName(index, mimeType)
}

func looksLikeLargeBase64(value string) bool {
	if len(value) < 512 {
		return false
	}
	for _, char := range value[:512] {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '+' || char == '/' || char == '=' || char == '\r' || char == '\n' {
			continue
		}
		return false
	}
	return true
}

func summarizeCallToolResult(result *sdkmcp.CallToolResult, policy string) string {
	if result == nil {
		return "MCP 工具未返回内容"
	}
	parts := make([]string, 0, len(result.Content)+1)
	for _, content := range result.Content {
		switch item := content.(type) {
		case *sdkmcp.TextContent:
			if text := strings.TrimSpace(item.Text); text != "" {
				parts = append(parts, text)
			}
		case *sdkmcp.ResourceLink:
			parts = append(parts, fmt.Sprintf("resource: %s", item.URI))
		case *sdkmcp.EmbeddedResource:
			if item == nil || item.Resource == nil {
				continue
			}
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.Resource.MIMEType)), "image/") && len(item.Resource.Blob) > 0 {
				parts = append(parts, fmt.Sprintf("image resource: %s (%s, %d bytes)", item.Resource.URI, item.Resource.MIMEType, len(item.Resource.Blob)))
				continue
			}
			if text := strings.TrimSpace(item.Resource.Text); text != "" {
				parts = append(parts, text)
			} else {
				parts = append(parts, fmt.Sprintf("resource: %s", item.Resource.URI))
			}
		case *sdkmcp.ImageContent:
			parts = append(parts, fmt.Sprintf("image: %s (%d bytes)", item.MIMEType, len(item.Data)))
		case *sdkmcp.AudioContent:
			parts = append(parts, fmt.Sprintf("audio: %s (%d bytes)", item.MIMEType, len(item.Data)))
		default:
			if encoded, err := json.Marshal(item); err == nil {
				parts = append(parts, string(encoded))
			}
		}
	}
	if result.StructuredContent != nil {
		if encoded, err := json.Marshal(result.StructuredContent); err == nil {
			structured := string(encoded)
			if len(parts) == 0 || !strings.Contains(strings.Join(parts, "\n"), structured) {
				parts = append(parts, structured)
			}
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "MCP 工具执行完成，无文本输出")
	}
	limit := 12 * 1024
	switch policy {
	case "balanced":
		limit = 32 * 1024
	case "raw-local":
		limit = 64 * 1024
	}
	return truncateText(strings.Join(parts, "\n"), limit)
}

func truncateText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + fmt.Sprintf("\n... [truncated %d bytes]", len(value)-limit)
}
