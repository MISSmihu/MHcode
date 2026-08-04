package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MISSmihu/MHcode/internal/tools"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type addInput struct {
	A int `json:"a" jsonschema:"first number"`
	B int `json:"b" jsonschema:"second number"`
}

type visionInput struct {
	Image    string `json:"image"`
	Prompt   string `json:"prompt"`
	MIMEType string `json:"mimeType"`
	FileName string `json:"fileName"`
}

func TestManagerStreamableHTTPDiscoverAndExecute(t *testing.T) {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "1.2.3"}, nil)
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "add",
		Description: "Add two integers",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *sdkmcp.CallToolRequest, input addInput) (*sdkmcp.CallToolResult, any, error) {
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: "sum=" + strconv.Itoa(input.A+input.B)},
		}}, map[string]int{"sum": input.A + input.B}, nil
	})

	var sawAuth atomic.Bool
	handler := sdkmcp.NewStreamableHTTPHandler(func(request *http.Request) *sdkmcp.Server {
		if request.Header.Get("Authorization") == "Bearer integration-test" {
			sawAuth.Store(true)
		}
		return server
	}, &sdkmcp.StreamableHTTPOptions{JSONResponse: true})
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	manager := NewManager()
	defer manager.Close()
	config := ServerConfig{
		ID:               "test-server",
		Name:             "Test Server",
		Transport:        TransportStreamableHTTP,
		URL:              httpServer.URL,
		Headers:          []KeyValue{{Key: "Authorization", Value: "Bearer integration-test"}},
		Enabled:          true,
		AllowNetwork:     true,
		ToolResultPolicy: "summary-first",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	statuses := manager.Configure(ctx, []ServerConfig{config})
	if len(statuses) != 1 || statuses[0].State != "ready" {
		t.Fatalf("statuses = %#v, want one ready server", statuses)
	}
	if statuses[0].ToolCount != 1 || statuses[0].ServerVersion != "1.2.3" {
		t.Fatalf("status metadata = %#v", statuses[0])
	}
	if !sawAuth.Load() {
		t.Fatal("configured HTTP Authorization header was not sent")
	}

	available := manager.Tools()
	if len(available) != 1 || available[0].Name() != "mcp__test-server__add" {
		t.Fatalf("tools = %#v", available)
	}
	remote, ok := available[0].(*RemoteTool)
	if !ok || !remote.ReadOnly() {
		t.Fatalf("tool = %#v, want read-only RemoteTool", available[0])
	}
	progress := make([]tools.ResultPart, 0, 1)
	callCtx := tools.WithProgressSink(ctx, func(part tools.ResultPart) {
		progress = append(progress, part)
	})
	result, err := available[0].Execute(callCtx, json.RawMessage(`{"a":2,"b":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Summary, "sum=5") || !strings.Contains(result.Summary, `"sum":5`) {
		t.Fatalf("result = %#v", result)
	}
	if len(progress) != 1 || progress[0].Status != "waiting" || !strings.Contains(progress[0].Output, "test-server") {
		t.Fatalf("MCP progress = %#v", progress)
	}

	snapshots := manager.Snapshots()
	if len(snapshots) != 1 || len(snapshots[0].Tools) != 1 || snapshots[0].Tools[0].Name != "mcp__test-server__add" {
		t.Fatalf("snapshots = %#v", snapshots)
	}
}

func TestManagerAnalyzeImageUsesConfiguredWireFormat(t *testing.T) {
	for _, test := range []struct {
		name      string
		inputMode string
		wantImage func(string) string
	}{
		{name: "data URL", inputMode: "data-url", wantImage: func(data string) string { return "data:image/png;base64," + data }},
		{name: "base64", inputMode: "base64", wantImage: func(data string) string { return data }},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := &atomic.Int32{}
			received := make(chan visionInput, 1)
			manager := newVisionTestManager(t, true, test.inputMode, calls, received)
			imageData := base64.StdEncoding.EncodeToString([]byte("image-bytes"))
			progress := make([]tools.ResultPart, 0, 1)
			ctx := tools.WithProgressSink(context.Background(), func(part tools.ResultPart) {
				progress = append(progress, part)
			})

			response, err := manager.AnalyzeImage(ctx, VisionRequest{
				Name: "screen.png", MIMEType: "image/png", Data: imageData, Prompt: "找出页面错误",
			})
			if err != nil {
				t.Fatal(err)
			}
			if response.ToolName != "mcp__vision__inspect_image" || !strings.Contains(response.Summary, "按钮被遮挡") {
				t.Fatalf("response = %#v", response)
			}
			if calls.Load() != 1 {
				t.Fatalf("vision calls = %d, want 1", calls.Load())
			}
			input := <-received
			if input.Image != test.wantImage(imageData) || input.MIMEType != "image/png" || input.FileName != "screen.png" {
				t.Fatalf("vision input = %#v", input)
			}
			if !strings.Contains(input.Prompt, "找出页面错误") {
				t.Fatalf("vision prompt = %q", input.Prompt)
			}
			if len(progress) != 1 || progress[0].Input != "screen.png · image/png" || strings.Contains(progress[0].Input, imageData) {
				t.Fatalf("vision progress leaked image data: %#v", progress)
			}
		})
	}
}

func TestManagerAnalyzeImageRequiresRemoteUploadPermission(t *testing.T) {
	calls := &atomic.Int32{}
	manager := newVisionTestManager(t, false, "data-url", calls, nil)
	_, err := manager.AnalyzeImage(context.Background(), VisionRequest{
		Name: "private.png", MIMEType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("private")),
	})
	if err == nil || !strings.Contains(err.Error(), "未允许上传图片到远程 MCP") {
		t.Fatalf("permission error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("remote vision tool was called %d times without permission", calls.Load())
	}
}

func TestCallToolResultAttachmentsPreservesImagesWithoutLeakingBlob(t *testing.T) {
	pngBytes := []byte{0x89, 'P', 'N', 'G'}
	webpBytes := []byte("RIFF-webp")
	result := &sdkmcp.CallToolResult{Content: []sdkmcp.Content{
		&sdkmcp.TextContent{Text: "视觉分析完成"},
		&sdkmcp.ImageContent{MIMEType: "image/png", Data: pngBytes},
		&sdkmcp.EmbeddedResource{Resource: &sdkmcp.ResourceContents{
			URI: "file:///tmp/chart.webp", MIMEType: "image/webp", Blob: webpBytes,
		}},
	}}
	attachments, warnings := callToolResultAttachments(result)
	if len(warnings) != 0 || len(attachments) != 2 {
		t.Fatalf("attachments = %#v, warnings = %#v", attachments, warnings)
	}
	if attachments[0].Name != "mcp-image-1.png" || attachments[0].Data != base64.StdEncoding.EncodeToString(pngBytes) {
		t.Fatalf("image attachment = %#v", attachments[0])
	}
	if attachments[1].Name != "chart.webp" || attachments[1].Data != base64.StdEncoding.EncodeToString(webpBytes) {
		t.Fatalf("resource attachment = %#v", attachments[1])
	}
	summary := summarizeCallToolResult(result, "summary-first")
	if !strings.Contains(summary, "视觉分析完成") || !strings.Contains(summary, "image resource:") {
		t.Fatalf("summary = %q", summary)
	}
	for _, attachment := range attachments {
		if strings.Contains(summary, attachment.Data) {
			t.Fatalf("summary leaked image payload: %q", summary)
		}
	}
}

func TestRemoteToolInputForDisplayHidesImagePayload(t *testing.T) {
	input := remoteToolInputForDisplay("inspect_image", map[string]any{
		"image": "data:image/png;base64,aGVsbG8=",
	})
	if input != "包含图片或二进制数据（已隐藏）" {
		t.Fatalf("display input = %q", input)
	}
}

func TestHasUsableVisionTextRejectsMediaOnlySummaries(t *testing.T) {
	for _, summary := range []string{
		"image: image/png (128 bytes)",
		"image resource: file:///tmp/capture.png (image/png, 128 bytes)",
		"audio: audio/wav (256 bytes)",
		"resource: file:///tmp/result.bin",
	} {
		if hasUsableVisionText(summary) {
			t.Fatalf("media-only summary was accepted: %q", summary)
		}
	}
	if !hasUsableVisionText("OCR: 保存按钮被遮挡") {
		t.Fatal("textual visual analysis was rejected")
	}
}

func newVisionTestManager(t *testing.T, allowRemote bool, inputMode string, calls *atomic.Int32, received chan<- visionInput) *Manager {
	t.Helper()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "vision-test", Version: "1.0.0"}, nil)
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "inspect_image", Description: "Analyze an image",
	}, func(_ context.Context, _ *sdkmcp.CallToolRequest, input visionInput) (*sdkmcp.CallToolResult, any, error) {
		calls.Add(1)
		if received != nil {
			received <- input
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: "发现按钮被遮挡"},
		}}, nil, nil
	})

	handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return server }, &sdkmcp.StreamableHTTPOptions{JSONResponse: true})
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	manager := NewManager()
	t.Cleanup(manager.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	statuses := manager.Configure(ctx, []ServerConfig{{
		ID: "vision", Name: "Vision", Transport: TransportStreamableHTTP, URL: httpServer.URL,
		Enabled: true, AllowNetwork: true, ToolResultPolicy: "summary-first",
		Vision: VisionToolConfig{
			Enabled: true, ToolName: "inspect_image", ImageArgument: "image", PromptArgument: "prompt",
			MIMETypeArgument: "mimeType", FileNameArgument: "fileName", InputMode: inputMode,
			AllowRemoteImages: allowRemote,
		},
	}})
	if len(statuses) != 1 || statuses[0].State != "ready" {
		t.Fatalf("vision MCP statuses = %#v", statuses)
	}
	return manager
}

func TestToolsForWorkspaceInjectsCodeGraphProjectPathPerRuntime(t *testing.T) {
	config := ServerConfig{
		ID:        "codegraph",
		Name:      "CodeGraph",
		Transport: TransportStdio,
		Command:   `C:\Users\tester\AppData\Roaming\npm\codegraph.cmd`,
		Args:      []string{"serve", "--mcp"},
		Enabled:   true,
	}
	manager := managerWithRemoteTool(config, &sdkmcp.Tool{
		Name:        "codegraph_explore",
		Description: "Explore code relationships",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":       map[string]any{"type": "string"},
				"projectPath": map[string]any{"type": "string"},
			},
		},
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	})

	firstRoot := filepath.Join(t.TempDir(), "first")
	secondRoot := filepath.Join(t.TempDir(), "second")
	first := onlyRemoteTool(t, manager.ToolsForWorkspace(firstRoot))
	second := onlyRemoteTool(t, manager.ToolsForWorkspace(secondRoot))

	firstArgs, firstRaw, firstWorkingDirectory, err := first.argumentsForCall(json.RawMessage(`{"query":"session runtime"}`))
	if err != nil {
		t.Fatal(err)
	}
	secondArgs, _, secondWorkingDirectory, err := second.argumentsForCall(json.RawMessage(`{"query":"background runtime"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := firstArgs["projectPath"], canonicalWorkspaceRoot(firstRoot); got != want {
		t.Fatalf("first projectPath = %#v, want %q", got, want)
	}
	if got, want := secondArgs["projectPath"], canonicalWorkspaceRoot(secondRoot); got != want {
		t.Fatalf("second projectPath = %#v, want %q", got, want)
	}
	if firstWorkingDirectory != canonicalWorkspaceRoot(firstRoot) || secondWorkingDirectory != canonicalWorkspaceRoot(secondRoot) {
		t.Fatalf("working directories = %q, %q", firstWorkingDirectory, secondWorkingDirectory)
	}
	if !strings.Contains(string(firstRaw), `"projectPath"`) {
		t.Fatalf("effective args = %s, want injected projectPath", firstRaw)
	}
	if !first.ReadOnly() || !second.ReadOnly() {
		t.Fatal("CodeGraph tool should retain its read-only annotation")
	}
}

func TestCodeGraphWorkspaceInjectionPreservesExplicitProjectPath(t *testing.T) {
	tool := &RemoteTool{workspaceRoot: canonicalWorkspaceRoot(t.TempDir()), injectProjectPath: true}
	explicit := filepath.Join(t.TempDir(), "explicit")
	arguments, _, workingDirectory, err := tool.argumentsForCall(json.RawMessage(fmt.Sprintf(`{"query":"impact","projectPath":%q}`, explicit)))
	if err != nil {
		t.Fatal(err)
	}
	if got := arguments["projectPath"]; got != explicit {
		t.Fatalf("projectPath = %#v, want explicit %q", got, explicit)
	}
	if workingDirectory != explicit {
		t.Fatalf("working directory = %q, want %q", workingDirectory, explicit)
	}
}

func TestWorkspacePathIsNotInjectedIntoOtherMCPServers(t *testing.T) {
	configs := []ServerConfig{
		{ID: "other", Transport: TransportStdio, Command: "other-mcp", Args: []string{"serve", "--mcp"}, Enabled: true},
		{ID: "remote-codegraph", Transport: TransportStreamableHTTP, Command: "codegraph", Args: []string{"serve", "--mcp"}, Enabled: true},
	}
	for _, config := range configs {
		t.Run(config.ID, func(t *testing.T) {
			manager := managerWithRemoteTool(config, &sdkmcp.Tool{
				Name: "query",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{
					"query":       map[string]any{"type": "string"},
					"projectPath": map[string]any{"type": "string"},
				}},
			})
			remote := onlyRemoteTool(t, manager.ToolsForWorkspace(t.TempDir()))
			arguments, _, workingDirectory, err := remote.argumentsForCall(json.RawMessage(`{"query":"hello"}`))
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := arguments["projectPath"]; exists {
				t.Fatalf("non-local CodeGraph MCP received projectPath: %#v", arguments)
			}
			if workingDirectory != "" {
				t.Fatalf("working directory = %q, want empty", workingDirectory)
			}
		})
	}
}

func TestCodeGraphToolWithoutProjectPathSchemaIsNotModified(t *testing.T) {
	config := ServerConfig{ID: "codegraph", Transport: TransportStdio, Command: "codegraph", Args: []string{"serve", "--mcp"}, Enabled: true}
	manager := managerWithRemoteTool(config, &sdkmcp.Tool{
		Name:        "legacy_tool",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
	})
	remote := onlyRemoteTool(t, manager.ToolsForWorkspace(t.TempDir()))
	arguments, _, _, err := remote.argumentsForCall(json.RawMessage(`{"query":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := arguments["projectPath"]; exists {
		t.Fatalf("tool without projectPath schema was modified: %#v", arguments)
	}
}

func managerWithRemoteTool(config ServerConfig, remote *sdkmcp.Tool) *Manager {
	return &Manager{
		servers: map[string]*managedServer{
			config.ID: {
				config:  config,
				session: &sdkmcp.ClientSession{},
				tools:   map[string]*sdkmcp.Tool{remote.Name: remote},
				status:  ServerStatus{State: "ready"},
			},
		},
		order: []string{config.ID},
	}
}

func onlyRemoteTool(t *testing.T, available []tools.Tool) *RemoteTool {
	t.Helper()
	if len(available) != 1 {
		t.Fatalf("tools = %#v, want one", available)
	}
	remote, ok := available[0].(*RemoteTool)
	if !ok {
		t.Fatalf("tool = %T, want *RemoteTool", available[0])
	}
	return remote
}
