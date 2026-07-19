package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MISSmihu/MHcode/internal/eventlog"
	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

type scriptedCompletion struct {
	result protocol.CompletionResult
	err    error
}

type scriptedToolCaller struct {
	steps []scriptedCompletion
	next  int
}

type failingBrowserTool struct{ calls *int }

type staticRepositoryTool struct{}

func (staticRepositoryTool) Name() string        { return "read_repository" }
func (staticRepositoryTool) Description() string { return "read a test repository" }
func (staticRepositoryTool) InputSchema() map[string]any {
	return map[string]any{"type": "object"}
}
func (staticRepositoryTool) Execute(context.Context, json.RawMessage) (tools.Result, error) {
	output := "Repository: MISSmihu/MHcode\nURL: https://github.com/MISSmihu/MHcode\nRef: main\nCommit: abc123\n\nREADME:\n# MHcode\n\nRepository tree:\nfrontend/\ninternal/"
	return tools.Result{
		Summary: "已读取 GitHub 仓库 MISSmihu/MHcode（main）",
		Parts: []tools.ResultPart{{
			Kind: tools.PartToolCall, Name: "read_repository", Status: "ok",
			Input: "https://github.com/MISSmihu/MHcode", Output: output,
		}},
	}, nil
}

func (t failingBrowserTool) Name() string        { return "browser" }
func (t failingBrowserTool) Description() string { return "test browser" }
func (t failingBrowserTool) InputSchema() map[string]any {
	return map[string]any{"type": "object"}
}
func (t failingBrowserTool) Execute(context.Context, json.RawMessage) (tools.Result, error) {
	(*t.calls)++
	summary := "browser open 执行失败: 内置浏览器启动失败。MHcode 已尝试备用浏览器和隔离配置目录。"
	return tools.Result{
		Summary: summary,
		IsError: true,
		Parts:   []tools.ResultPart{{Kind: tools.PartToolCall, Name: "browser", Status: "error", Output: summary}},
	}, nil
}

func (c *scriptedToolCaller) Complete(_ context.Context, _ protocol.ChatRequest) (protocol.CompletionResult, error) {
	if c.next >= len(c.steps) {
		return protocol.CompletionResult{}, errors.New("unexpected completion call")
	}
	step := c.steps[c.next]
	c.next++
	return step.result, step.err
}

func newNativeToolLoopService(t *testing.T) (*Service, string) {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "fixture.txt"), []byte("hello from fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	svc.runtimeSettings.WorkspaceRoot = workspace
	svc.runtimeSettings.FilesystemAccess = "workspace-write"
	svc.runtimeSettings.ApprovalPolicy = "never"
	return svc, workspace
}

func TestAnthropicStreamingToolLoopExecutesAndReturnsToolResult(t *testing.T) {
	svc, _ := newNativeToolLoopService(t)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch requestCount.Add(1) {
		case 1:
			if !strings.Contains(string(body), `"tools"`) {
				t.Errorf("first Anthropic request does not declare tools: %s", body)
			}
			_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_read\",\"name\":\"read_file\",\"input\":{\"path\":\"fixture.txt\"}}}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
		case 2:
			payload := string(body)
			for _, expected := range []string{`"type":"tool_result"`, `"tool_use_id":"toolu_read"`, "hello from fixture"} {
				if !strings.Contains(payload, expected) {
					t.Errorf("second Anthropic request missing %q: %s", expected, payload)
				}
			}
			_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"anthropic done\"}}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	provider := protocol.AnthropicProvider{BaseURL: server.URL, APIKey: "test-key"}
	outcome, err := svc.runStreamingToolLoop(context.Background(), provider, svc.buildToolRegistry(), protocol.ChatRequest{
		Model:    "claude-test",
		Messages: []protocol.Message{{Role: "user", Content: "read fixture.txt"}},
	}, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Content != "anthropic done" || requestCount.Load() != 2 {
		t.Fatalf("outcome = %#v, requests = %d", outcome, requestCount.Load())
	}
}

func TestGeminiStreamingToolLoopExecutesAndReturnsFunctionResponse(t *testing.T) {
	svc, _ := newNativeToolLoopService(t)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch requestCount.Add(1) {
		case 1:
			if !strings.Contains(string(body), `"functionDeclarations"`) {
				t.Errorf("first Gemini request does not declare tools: %s", body)
			}
			_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"functionCall\":{\"id\":\"gemini-read\",\"name\":\"read_file\",\"args\":{\"path\":\"fixture.txt\"}}}]}}]}\n\n"))
		case 2:
			payload := string(body)
			for _, expected := range []string{`"functionResponse"`, `"id":"gemini-read"`, "hello from fixture"} {
				if !strings.Contains(payload, expected) {
					t.Errorf("second Gemini request missing %q: %s", expected, payload)
				}
			}
			_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"gemini done\"}]}}]}\n\n"))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	provider := protocol.GeminiProvider{BaseURL: server.URL, APIKey: "test-key"}
	outcome, err := svc.runStreamingToolLoop(context.Background(), provider, svc.buildToolRegistry(), protocol.ChatRequest{
		Model:    "gemini-test",
		Messages: []protocol.Message{{Role: "user", Content: "read fixture.txt"}},
	}, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Content != "gemini done" || requestCount.Load() != 2 {
		t.Fatalf("outcome = %#v, requests = %d", outcome, requestCount.Load())
	}
}

func TestWebSearchToolLoopFeedsSourcesIntoFinalCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel>
			<item><title>Ningbo warning</title><link>https://weather.example/warning</link><description>Typhoon warning details</description></item>
			<item><title>Forecast</title><link>https://weather.example/forecast</link><description>Latest forecast</description></item>
		</channel></rss>`))
	}))
	defer server.Close()

	svc := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	registry := tools.NewRegistry(tools.WebSearchTool{
		Policy:   tools.SandboxPolicy{NetworkAccess: true},
		Client:   server.Client(),
		Endpoint: server.URL,
	})
	completionCalls := 0
	complete := func(_ context.Context, request protocol.ChatRequest) (protocol.CompletionResult, error) {
		completionCalls++
		if completionCalls == 1 {
			return protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
				ID:   "search-1",
				Type: "function",
				Function: protocol.ToolCallFunction{
					Name:      "web_search",
					Arguments: json.RawMessage(`{"query":"宁波 台风 预警","max_results":2}`),
				},
			}}}, nil
		}
		last := request.Messages[len(request.Messages)-1]
		for _, expected := range []string{"Ningbo warning", "https://weather.example/warning", "Typhoon warning details"} {
			if !strings.Contains(last.Content, expected) {
				t.Errorf("tool feedback missing %q: %s", expected, last.Content)
			}
		}
		return protocol.CompletionResult{Content: "宁波预警信息已整理：[来源](https://weather.example/warning)"}, nil
	}

	outcome, err := svc.runToolLoopWithCompletion(context.Background(), registry, protocol.ChatRequest{
		Model:    "test-model",
		Messages: []protocol.Message{{Role: "user", Content: "搜索宁波台风预警"}},
	}, 4, complete, nil)
	if err != nil {
		t.Fatal(err)
	}
	if completionCalls != 2 || !strings.Contains(outcome.Content, "来源") {
		t.Fatalf("calls=%d outcome=%+v", completionCalls, outcome)
	}
	for _, expectedURL := range []string{"https://weather.example/warning", "https://weather.example/forecast"} {
		if !strings.Contains(outcome.Content, expectedURL) {
			t.Fatalf("final answer missing source %q: %s", expectedURL, outcome.Content)
		}
	}
	for _, expectedTitle := range []string{"Ningbo warning", "Forecast"} {
		if !strings.Contains(outcome.Content, expectedTitle) {
			t.Fatalf("final answer missing source title %q: %s", expectedTitle, outcome.Content)
		}
	}
	foundSources := false
	for _, part := range outcome.Parts {
		if part.Kind == tools.PartWebSearch && len(part.Sources) == 2 {
			foundSources = true
		}
	}
	if !foundSources {
		t.Fatalf("structured search sources missing: %+v", outcome.Parts)
	}
}

func TestToolLoopReusesSimilarWebSearchAndMergesSources(t *testing.T) {
	searchRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		searchRequests++
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel>
			<item><title>宁波天气预警</title><link>https://weather.example/warning</link><description>宁波市气象台发布预警。</description></item>
		</channel></rss>`))
	}))
	defer server.Close()

	svc := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	registry := tools.NewRegistry(tools.WebSearchTool{
		Policy: tools.SandboxPolicy{NetworkAccess: true}, Client: server.Client(), Endpoint: server.URL,
	})
	completionCalls := 0
	complete := func(_ context.Context, request protocol.ChatRequest) (protocol.CompletionResult, error) {
		completionCalls++
		switch completionCalls {
		case 1:
			return protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
				ID: "search-1", Type: "function", Function: protocol.ToolCallFunction{
					Name: "web_search", Arguments: json.RawMessage(`{"query":"宁波 今天 台风预警 2026年 宁波气象台"}`),
				},
			}}}, nil
		case 2:
			return protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
				ID: "search-2", Type: "function", Function: protocol.ToolCallFunction{
					Name: "web_search", Arguments: json.RawMessage(`{"query":"宁波市气象台 台风预警 2026年7月16日"}`),
				},
			}}}, nil
		default:
			if !strings.Contains(request.Messages[len(request.Messages)-1].Content, "复用已有 1 条来源") {
				t.Fatalf("duplicate search feedback = %q", request.Messages[len(request.Messages)-1].Content)
			}
			return protocol.CompletionResult{Content: "已根据来源整理。"}, nil
		}
	}

	outcome, err := svc.runToolLoopWithCompletion(context.Background(), registry, protocol.ChatRequest{
		Model: "test-model", Messages: []protocol.Message{{Role: "user", Content: "查询宁波台风预警"}},
	}, 6, complete, nil)
	if err != nil {
		t.Fatal(err)
	}
	if searchRequests != 1 || completionCalls != 3 {
		t.Fatalf("searchRequests=%d completionCalls=%d", searchRequests, completionCalls)
	}
	searchParts := 0
	for _, part := range outcome.Parts {
		if part.Kind == tools.PartToolCall && part.Name == "web_search" && part.Status != "error" {
			t.Fatalf("successful search tool card should be folded into sources: %+v", outcome.Parts)
		}
		if part.Kind == tools.PartWebSearch {
			searchParts++
			if len(part.Sources) != 1 || part.Sources[0].URL != "https://weather.example/warning" {
				t.Fatalf("search part = %+v", part)
			}
		}
	}
	if searchParts != 1 {
		t.Fatalf("search parts=%d outcome=%+v", searchParts, outcome.Parts)
	}
}

func TestToolLoopDoesNotRestartUnavailableBrowser(t *testing.T) {
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	svc.runtimeSettings.ApprovalPolicy = "never"
	browserCalls := 0
	registry := tools.NewRegistry(failingBrowserTool{calls: &browserCalls})
	completionCalls := 0
	complete := func(_ context.Context, request protocol.ChatRequest) (protocol.CompletionResult, error) {
		completionCalls++
		if completionCalls <= 2 {
			return protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
				ID: fmt.Sprintf("browser-%d", completionCalls), Type: "function", Function: protocol.ToolCallFunction{
					Name: "browser", Arguments: json.RawMessage(fmt.Sprintf(`{"action":"open","url":"https://example.com/%d"}`, completionCalls)),
				},
			}}}, nil
		}
		if !strings.Contains(request.Messages[len(request.Messages)-1].Content, "已跳过重复启动") {
			t.Fatalf("browser short-circuit feedback = %q", request.Messages[len(request.Messages)-1].Content)
		}
		return protocol.CompletionResult{Content: "浏览器不可用，已保留现有来源。"}, nil
	}
	_, err := svc.runToolLoopWithCompletion(context.Background(), registry, protocol.ChatRequest{
		Model: "test-model", Messages: []protocol.Message{{Role: "user", Content: "打开网页"}},
	}, 4, complete, nil)
	if err != nil {
		t.Fatal(err)
	}
	if browserCalls != 1 {
		t.Fatalf("browser executed %d times, want 1", browserCalls)
	}
}

func TestToolLoopSuppressesUnrequestedExternalBrowserAfterSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel>
			<item><title>Ningbo warning</title><link>https://weather.example/warning</link><description>Typhoon warning details</description></item>
		</channel></rss>`))
	}))
	defer server.Close()

	svc := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	svc.runtimeSettings.ApprovalPolicy = "never"
	browserCalls := 0
	registry := tools.NewRegistry(
		tools.WebSearchTool{Policy: tools.SandboxPolicy{NetworkAccess: true}, Client: server.Client(), Endpoint: server.URL},
		failingBrowserTool{calls: &browserCalls},
	)
	completionCalls := 0
	var events []ChatStreamEvent
	complete := func(_ context.Context, request protocol.ChatRequest) (protocol.CompletionResult, error) {
		completionCalls++
		switch completionCalls {
		case 1:
			return protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
				ID: "search-1", Type: "function", Function: protocol.ToolCallFunction{
					Name: "web_search", Arguments: json.RawMessage(`{"query":"宁波 台风预警","max_results":1}`),
				},
			}}}, nil
		case 2:
			return protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
				ID: "browser-1", Type: "function", Function: protocol.ToolCallFunction{
					Name: "browser", Arguments: json.RawMessage(`{"action":"open","url":"https://weather.example/warning"}`),
				},
			}}}, nil
		default:
			feedback := request.Messages[len(request.Messages)-1].Content
			for _, expected := range []string{"网页未打开", "直接基于来源整理完整回答", "完整 URL"} {
				if !strings.Contains(feedback, expected) {
					t.Fatalf("suppression feedback missing %q: %s", expected, feedback)
				}
			}
			return protocol.CompletionResult{Content: "宁波预警信息已整理。"}, nil
		}
	}

	outcome, err := svc.runToolLoopWithCompletion(context.Background(), registry, protocol.ChatRequest{
		Model: "test-model", Messages: []protocol.Message{{Role: "user", Content: "搜索宁波台风预警并附来源链接"}},
	}, 5, complete, func(event ChatStreamEvent) { events = append(events, event) })
	if err != nil {
		t.Fatal(err)
	}
	if browserCalls != 0 {
		t.Fatalf("browser executed %d times, want 0", browserCalls)
	}
	for _, event := range events {
		if event.Type == "tool" && event.ToolName == "browser" {
			t.Fatalf("suppressed browser call leaked into activity events: %+v", events)
		}
	}
	for _, expected := range []string{"Ningbo warning", "https://weather.example/warning"} {
		if !strings.Contains(outcome.Content, expected) {
			t.Fatalf("final answer missing visible source detail %q: %s", expected, outcome.Content)
		}
	}
	for _, part := range outcome.Parts {
		if part.Kind == tools.PartToolCall && part.Name == "browser" {
			t.Fatalf("suppressed browser call leaked into result parts: %+v", outcome.Parts)
		}
	}
}

func TestBrowserUseExplicitlyRequested(t *testing.T) {
	if browserUseExplicitlyRequested([]protocol.Message{{Role: "user", Content: "搜索宁波天气并附来源"}}) {
		t.Fatal("ordinary search should not opt into browser navigation")
	}
	if !browserUseExplicitlyRequested([]protocol.Message{{Role: "user", Content: "请打开 https://example.com 查看网页"}}) {
		t.Fatal("explicit URL opening should opt into browser navigation")
	}
	if browserUseExplicitlyRequested([]protocol.Message{{Role: "user", Content: "不要打开网页，只列出链接"}}) {
		t.Fatal("negated browser request should remain disabled")
	}
}

func TestSearchQueriesSimilarKeepsSiteRefinementsDistinct(t *testing.T) {
	if !searchQueriesSimilar(
		"宁波 今天 台风预警 2026年 宁波气象台",
		"宁波市气象台 台风预警 2026年7月16日",
	) {
		t.Fatal("general refinements should be considered similar")
	}
	if searchQueriesSimilar("宁波台风预警", "site:nmc.cn 宁波台风预警") {
		t.Fatal("site-specific refinement should remain distinct")
	}
	if searchQueriesSimilar("site:nmc.cn 宁波台风预警", "site:qx121.com 宁波台风预警") {
		t.Fatal("different site filters should remain distinct")
	}
}

func TestWebSearchToolLoopRetriesFinalSynthesisWithoutRepeatingSearch(t *testing.T) {
	searchRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		searchRequests++
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel>
			<item><title>Repository</title><link>https://github.com/example/project</link><description>Source repository</description></item>
		</channel></rss>`))
	}))
	defer server.Close()

	svc := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	registry := tools.NewRegistry(tools.WebSearchTool{
		Policy: tools.SandboxPolicy{NetworkAccess: true}, Client: server.Client(), Endpoint: server.URL,
	})
	completionCalls := 0
	var events []ChatStreamEvent
	complete := func(_ context.Context, request protocol.ChatRequest) (protocol.CompletionResult, error) {
		completionCalls++
		switch completionCalls {
		case 1:
			return protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
				ID: "search-retry", Type: "function", Function: protocol.ToolCallFunction{
					Name: "web_search", Arguments: json.RawMessage(`{"query":"example project","max_results":1}`),
				},
			}}}, nil
		case 2:
			if !strings.Contains(request.Messages[len(request.Messages)-1].Content, "https://github.com/example/project") {
				t.Fatal("retry request lost the completed search result")
			}
			return protocol.CompletionResult{}, io.ErrUnexpectedEOF
		default:
			return protocol.CompletionResult{Content: "search result synthesized"}, nil
		}
	}

	outcome, err := svc.runToolLoopWithCompletion(context.Background(), registry, protocol.ChatRequest{
		Model: "test-model", Messages: []protocol.Message{{Role: "user", Content: "inspect example project"}},
	}, 4, complete, func(event ChatStreamEvent) { events = append(events, event) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outcome.Content, "search result synthesized") || !strings.Contains(outcome.Content, "https://github.com/example/project") || completionCalls != 3 || searchRequests != 1 {
		t.Fatalf("content=%q completionCalls=%d searchRequests=%d", outcome.Content, completionCalls, searchRequests)
	}
	foundRetryStatus := false
	for _, event := range events {
		if event.Type == "status" && strings.Contains(event.Message, "继续整理工具结果") {
			foundRetryStatus = true
			break
		}
	}
	if !foundRetryStatus {
		t.Fatalf("retry status event missing: %+v", events)
	}
}

func TestWebSearchFailureFallsBackToSourceSummaries(t *testing.T) {
	outcome := toolLoopOutcome{Parts: []tools.ResultPart{{
		Kind:  tools.PartWebSearch,
		Query: "宁波 台风预警",
		Sources: []tools.SearchSource{
			{Title: "宁波天气预警", URL: "https://weather.example/warning", Snippet: "宁波市气象台发布最新预警。"},
			{Title: "实时台风消息", URL: "https://weather.example/typhoon", Snippet: "台风路径与防御信息。"},
		},
	}}}
	if !hasWebSearchSources(outcome.Parts) {
		t.Fatal("search sources should make the partial result usable")
	}
	content := partialToolFailureContent(outcome)
	for _, expected := range []string{"网络搜索已完成", "请通过链接核对关键细节", "宁波天气预警", "宁波市气象台发布最新预警", "https://weather.example/warning"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("fallback missing %q: %s", expected, content)
		}
	}
	emptyCompletion := emptyToolCompletionContent(outcome.Parts)
	if emptyCompletion != content {
		t.Fatalf("empty completion fallback differs from provider failure fallback:\nempty: %s\nerror: %s", emptyCompletion, content)
	}
}

func TestWebSearchFallbackIncludesEverySource(t *testing.T) {
	sources := make([]tools.SearchSource, 0, 7)
	for index := 1; index <= 7; index++ {
		sources = append(sources, tools.SearchSource{
			Title: fmt.Sprintf("Source %d", index),
			URL:   fmt.Sprintf("https://example.com/source-%d", index),
		})
	}
	content := webSearchFallbackContent(tools.ResultPart{Kind: tools.PartWebSearch, Sources: sources})
	if !strings.Contains(content, "Source 7") || !strings.Contains(content, "https://example.com/source-7") {
		t.Fatalf("fallback omitted later sources: %s", content)
	}
}

func TestRepositoryReadFailureFallsBackToReadableRepositoryContent(t *testing.T) {
	part := tools.ResultPart{
		Kind:   tools.PartToolCall,
		Name:   "read_repository",
		Status: "ok",
		Input:  "https://github.com/MISSmihu/MHcode",
		Output: strings.Join([]string{
			"Repository: MISSmihu/MHcode",
			"URL: https://github.com/MISSmihu/MHcode",
			"Ref: main",
			"Commit: abc123",
			"Language: Go",
			"",
			"README:",
			"# MHcode",
			"A coding agent workbench.",
			"",
			"Repository tree:",
			"frontend/",
			"internal/",
			"go.mod (123 bytes)",
		}, "\n"),
	}
	outcome := toolLoopOutcome{Parts: []tools.ResultPart{part}}
	if !hasUsablePartialToolResult(outcome.Parts) || !hasSuccessfulRepositoryRead(outcome.Parts) {
		t.Fatal("a successful repository read should remain usable when final synthesis fails")
	}
	content := partialToolFailureContent(outcome)
	for _, expected := range []string{
		"GitHub 仓库读取已完成",
		"https://github.com/MISSmihu/MHcode",
		"Commit: abc123",
		"README（截取）",
		"A coding agent workbench.",
		"目录树（截取）",
		"frontend/",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("repository fallback missing %q: %s", expected, content)
		}
	}
	if empty := emptyToolCompletionContent(outcome.Parts); empty != content {
		t.Fatalf("empty completion fallback differs from provider failure fallback:\nempty: %s\nerror: %s", empty, content)
	}
}

func TestRepositoryReadFallbackTruncatesLargeSectionsIndependently(t *testing.T) {
	content := repositoryReadFallbackContent(tools.ResultPart{
		Kind:   tools.PartToolCall,
		Name:   "read_repository",
		Status: "ok",
		Input:  "https://github.com/example/large",
		Output: "Repository: example/large\nRef: main\nCommit: deadbeef\n\nREADME:\n" +
			strings.Repeat("R", 4_000) + "\nRepository tree:\ncmd/\ninternal/\nREADME.md",
	})
	if !strings.Contains(content, "内容已截取") || !strings.Contains(content, "internal/") {
		t.Fatalf("large README should not hide the repository tree: %s", content)
	}
}

func TestRepositoryReadToolLoopKeepsResultAfterFinalSynthesisEOF(t *testing.T) {
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	registry := tools.NewRegistry(staticRepositoryTool{})
	completionCalls := 0
	complete := func(_ context.Context, request protocol.ChatRequest) (protocol.CompletionResult, error) {
		completionCalls++
		if completionCalls == 1 {
			return protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
				ID: "repo-1", Type: "function", Function: protocol.ToolCallFunction{
					Name: "read_repository", Arguments: json.RawMessage(`{"url":"https://github.com/MISSmihu/MHcode"}`),
				},
			}}}, nil
		}
		if !strings.Contains(request.Messages[len(request.Messages)-1].Content, "Commit: abc123") {
			t.Fatalf("repository result was not preserved for synthesis: %s", request.Messages[len(request.Messages)-1].Content)
		}
		return protocol.CompletionResult{}, io.ErrUnexpectedEOF
	}

	outcome, err := svc.runToolLoopWithCompletion(context.Background(), registry, protocol.ChatRequest{
		Model: "test-model", Messages: []protocol.Message{{Role: "user", Content: "读取 MHcode 仓库"}},
	}, 4, complete, nil)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want unexpected EOF", err)
	}
	if completionCalls != 2+postToolCompletionRetries {
		t.Fatalf("completion calls = %d, want %d", completionCalls, 2+postToolCompletionRetries)
	}
	content := partialToolFailureContent(outcome)
	if !hasUsablePartialToolResult(outcome.Parts) || !strings.Contains(content, "Commit: abc123") || !strings.Contains(content, "frontend/") {
		t.Fatalf("repository partial result was lost: outcome=%+v content=%s", outcome, content)
	}
}

func TestToolLoopEmitsAndPersistsTaskProgressWithDiffStats(t *testing.T) {
	workspace := t.TempDir()
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	svc.runtimeSettings.WorkspaceRoot = workspace
	svc.runtimeSettings.FilesystemAccess = "workspace-write"
	svc.runtimeSettings.ApprovalPolicy = "never"
	registry := tools.NewRegistry(
		tools.UpdatePlanTool{},
		tools.WriteFileTool{Policy: svc.sandboxPolicy()},
	)
	steps := []scriptedCompletion{
		{result: protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
			ID: "plan-1", Type: "function", Function: protocol.ToolCallFunction{
				Name:      "update_plan",
				Arguments: json.RawMessage(`{"steps":[{"title":"Implement","status":"in_progress"},{"title":"Verify","status":"pending"}]}`),
			},
		}}}},
		{result: protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
			ID: "write-1", Type: "function", Function: protocol.ToolCallFunction{
				Name:      "write_file",
				Arguments: json.RawMessage(`{"path":"result.txt","content":"done\n"}`),
			},
		}}}},
		{result: protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
			ID: "plan-2", Type: "function", Function: protocol.ToolCallFunction{
				Name:      "update_plan",
				Arguments: json.RawMessage(`{"steps":[{"title":"Implement","status":"completed"},{"title":"Verify","status":"completed"}]}`),
			},
		}}}},
		{result: protocol.CompletionResult{Content: "done"}},
	}
	caller := &scriptedToolCaller{steps: steps}
	var progressEvents []ChatStreamEvent
	outcome, err := svc.runToolLoopWithCompletion(context.Background(), registry, protocol.ChatRequest{
		Model:    "test-model",
		Messages: []protocol.Message{{Role: "user", Content: "implement"}},
	}, 6, caller.Complete, func(event ChatStreamEvent) {
		if event.Type == "progress" {
			progressEvents = append(progressEvents, event)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	var progress *tools.ResultPart
	for index := range outcome.Parts {
		if outcome.Parts[index].Kind == tools.PartProgress {
			progress = &outcome.Parts[index]
		}
	}
	if progress == nil || progress.TaskStatus != "completed" || progress.ChangedFiles != 1 || progress.Additions != 1 || progress.Deletions != 0 {
		t.Fatalf("progress=%+v parts=%+v", progress, outcome.Parts)
	}
	if len(progressEvents) < 3 || progressEvents[len(progressEvents)-1].Progress.TaskStatus != "completed" {
		t.Fatalf("progress events=%+v", progressEvents)
	}
}

func TestToolLoopPersistsSnapshotBeforeFinalCompletion(t *testing.T) {
	workspace := t.TempDir()
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: t.TempDir()})
	svc.runtimeSettings.WorkspaceRoot = workspace
	svc.runtimeSettings.FilesystemAccess = "workspace-write"
	svc.runtimeSettings.ApprovalPolicy = "never"

	svc.recordUserEvent("baseline")
	svc.sessionState.TurnCount = 1
	svc.recordAssistantAndCheckpoint("baseline", "test-model", nil)
	checkpoints := svc.eventStore.Checkpoints()
	if len(checkpoints) != 1 {
		t.Fatalf("checkpoints = %d, want 1", len(checkpoints))
	}
	baselineID := checkpoints[0].ID
	svc.recordUserEvent("create page")

	args, err := json.Marshal(map[string]string{
		"path":    "generated.html",
		"content": "<h1>ok</h1>\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	caller := &scriptedToolCaller{steps: []scriptedCompletion{
		{result: protocol.CompletionResult{ToolCalls: []protocol.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: protocol.ToolCallFunction{
				Name:      "write_file",
				Arguments: args,
			},
		}}}},
		{err: errors.New("provider connection dropped")},
	}}

	_, loopErr := svc.runToolLoop(context.Background(), caller, svc.buildToolRegistry(), protocol.ChatRequest{
		Model:    "test-model",
		Messages: []protocol.Message{{Role: "user", Content: "create page"}},
	}, 4)
	if loopErr == nil {
		t.Fatal("最终补全失败应返回错误")
	}
	generated := filepath.Join(workspace, "generated.html")
	if _, err := os.Stat(generated); err != nil {
		t.Fatalf("工具已成功写入的文件应保留: %v", err)
	}

	foundSnapshot := false
	for _, ev := range svc.eventStore.Events() {
		if ev.Type == eventlog.EventFileSnapshot && ev.Payload.Path == "generated.html" {
			foundSnapshot = true
			break
		}
	}
	if !foundSnapshot {
		t.Fatal("模型最终补全失败前，文件快照事件必须已经持久化")
	}

	if _, err := svc.RewindToCheckpoint(baselineID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(generated); !os.IsNotExist(err) {
		t.Fatalf("rewind 应删除失败轮次新建的文件，stat err=%v", err)
	}
}

func TestExecuteToolCallRendersValidationFailureAsToolCard(t *testing.T) {
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	svc.runtimeSettings.WorkspaceRoot = t.TempDir()
	svc.runtimeSettings.FilesystemAccess = "workspace-write"

	result, message := svc.executeToolCall(context.Background(), svc.buildToolRegistry(), protocol.ToolCall{
		ID:   "call-bad-path",
		Type: "function",
		Function: protocol.ToolCallFunction{
			Name:      "list_dir",
			Arguments: json.RawMessage(`{"path":"~"}`),
		},
	})
	if !result.IsError || len(result.Parts) != 1 {
		t.Fatalf("invalid path result = %+v", result)
	}
	part := result.Parts[0]
	if part.Kind != "tool_call" || part.Name != "list_dir" || part.Status != "error" || part.Input != "~" {
		t.Fatalf("failure card = %+v", part)
	}
	if message.Content != result.Summary {
		t.Fatalf("tool feedback should contain one summary, got %q", message.Content)
	}
}

func TestToolRegistryIncludesHostOpenFile(t *testing.T) {
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir(), OpenFile: func(string) error { return nil }})
	svc.runtimeSettings.WorkspaceRoot = t.TempDir()
	if _, ok := svc.buildToolRegistry().Get("open_file"); !ok {
		t.Fatal("desktop service must expose open_file to the model")
	}
}

func TestToolRegistryExposesStructuredWorkspaceToolsToModels(t *testing.T) {
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	svc.runtimeSettings.WorkspaceRoot = t.TempDir()
	svc.runtimeSettings.FilesystemAccess = "workspace-write"
	registry := svc.buildToolRegistry()
	for _, name := range []string{
		"read_file", "file_info", "list_dir", "search",
		"write_file", "apply_patch", "copy_file", "delete_file",
	} {
		if _, ok := registry.Get(name); !ok {
			t.Fatalf("model tool registry is missing %s", name)
		}
	}
	snapshot := svc.builtinToolSnapshot()
	for _, name := range []string{"read_file", "apply_patch", "copy_file", "delete_file"} {
		found := false
		for _, descriptor := range snapshot.Tools {
			if descriptor.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("builtin schema snapshot is missing %s: %#v", name, snapshot.Tools)
		}
	}
}

func TestToolRegistryIncludesNetworkToolsOnlyWithNetworkAccess(t *testing.T) {
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	svc.runtimeSettings.WorkspaceRoot = t.TempDir()
	svc.runtimeSettings.NetworkAccess = false
	for _, name := range []string{"read_repository", "web_search"} {
		if _, ok := svc.buildToolRegistry().Get(name); ok {
			t.Fatalf("network-disabled registry must not expose %s", name)
		}
	}
	svc.runtimeSettings.NetworkAccess = true
	for _, name := range []string{"read_repository", "web_search"} {
		if _, ok := svc.buildToolRegistry().Get(name); !ok {
			t.Fatalf("network-enabled registry must expose %s", name)
		}
	}
}
