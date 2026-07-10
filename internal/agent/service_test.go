package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/MISSmihu/MHcode/internal/protocol"
)

func TestServiceDeepSeekKeyLifecycle(t *testing.T) {
	service := NewService(ServiceConfig{SkillsDir: t.TempDir()})

	state, err := service.SaveDeepSeekAPIKey(" sk-test ")
	if err != nil {
		t.Fatal(err)
	}
	if !state.DeepSeek.Configured {
		t.Fatal("deepseek should be configured after saving key")
	}
	if state.DeepSeek.Models != nil {
		t.Fatalf("models = %#v, want nil before connection test", state.DeepSeek.Models)
	}

	state, err = service.ClearDeepSeekAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if state.DeepSeek.Configured {
		t.Fatal("deepseek should not be configured after clearing key")
	}
}

func TestServiceDeepSeekConnectionWithoutKey(t *testing.T) {
	service := NewService(ServiceConfig{SkillsDir: t.TempDir()})

	state, err := service.TestDeepSeekConnection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.DeepSeek.LastCheckStatus != "error" {
		t.Fatalf("status = %q, want error", state.DeepSeek.LastCheckStatus)
	}
}

func TestServiceRuntimeSettingsPersist(t *testing.T) {
	settingsPath := t.TempDir() + "/runtime-settings.json"
	service := NewService(ServiceConfig{SkillsDir: t.TempDir(), SettingsPath: settingsPath})

	settings := DefaultRuntimeSettings()
	settings.SandboxMode = "read-only"
	settings.FilesystemAccess = "read-only"
	settings.NetworkAccess = false
	settings.ShellAccess = false
	settings.ApprovalPolicy = "never"
	settings.MaxCommandSeconds = 30
	settings.CacheTargetPercent = 97.5
	settings.Git.BranchPrefix = "codex/"
	settings.Git.MergeMethod = "squash"
	settings.Browser.Enabled = false
	settings.Browser.DefaultLocalURLDestination = "system"
	settings.ComputerControl.AlwaysAllowedApps = []string{"Code.exe", "chrome.exe"}
	settings.MCP.Servers = []MCPServerSetting{{
		ID:               "node-repl",
		Name:             "Node REPL",
		Command:          "node",
		Args:             []string{"server.js"},
		Enabled:          true,
		ToolResultPolicy: "balanced",
	}}
	settings.Model.SelectedProviderID = "openai-compatible"
	settings.Model.Providers = []ModelProviderSetting{{
		ID:                 "openai-compatible",
		Name:               "OpenAI 兼容",
		Protocol:           "openai-compatible",
		BaseURL:            "https://example.test/v1",
		Enabled:            true,
		LastSyncStatus:     "idle",
		SupportsModelFetch: true,
	}}

	state, err := service.SaveRuntimeSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if state.RuntimeSettings.SandboxMode != "read-only" || state.RuntimeSettings.NetworkAccess {
		t.Fatalf("runtime settings not reflected in state: %#v", state.RuntimeSettings)
	}
	if state.CacheTarget != 0.975 {
		t.Fatalf("cache target = %f, want 0.975", state.CacheTarget)
	}
	if state.RuntimeSettings.Git.BranchPrefix != "codex/" || state.RuntimeSettings.Git.MergeMethod != "squash" {
		t.Fatalf("git settings not reflected in state: %#v", state.RuntimeSettings.Git)
	}
	if state.RuntimeSettings.Browser.Enabled {
		t.Fatalf("browser setting not reflected in state: %#v", state.RuntimeSettings.Browser)
	}
	if len(state.RuntimeSettings.MCP.Servers) != 1 || state.RuntimeSettings.MCP.Servers[0].Command != "node" {
		t.Fatalf("mcp settings not reflected in state: %#v", state.RuntimeSettings.MCP)
	}

	reloaded := NewService(ServiceConfig{SkillsDir: t.TempDir(), SettingsPath: settingsPath})
	reloadedState := reloaded.WorkbenchState()
	if reloadedState.RuntimeSettings.SandboxMode != "read-only" || reloadedState.RuntimeSettings.ShellAccess {
		t.Fatalf("runtime settings not persisted: %#v", reloadedState.RuntimeSettings)
	}
	if reloadedState.RuntimeSettings.Git.BranchPrefix != "codex/" || reloadedState.RuntimeSettings.Browser.DefaultLocalURLDestination != "system" {
		t.Fatalf("nested runtime settings not persisted: %#v", reloadedState.RuntimeSettings)
	}
	if reloadedState.RuntimeSettings.Model.SelectedProviderID != "openai-compatible" {
		t.Fatalf("model settings not persisted: %#v", reloadedState.RuntimeSettings.Model)
	}
}

func TestServiceSendDeepSeekMessageUpdatesUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":12,\"total_tokens\":112,\"prompt_cache_hit_tokens\":96,\"prompt_cache_miss_tokens\":4}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	service := NewService(ServiceConfig{
		SkillsDir:       t.TempDir(),
		DeepSeekBaseURL: server.URL,
	})
	if _, err := service.SaveDeepSeekAPIKey("sk-test"); err != nil {
		t.Fatal(err)
	}

	result, err := service.SendDeepSeekMessage(context.Background(), "ping")
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "hello" {
		t.Fatalf("content = %q, want hello", result.Content)
	}
	if result.State.UsageMetrics.PromptCacheHitTokens != 96 {
		t.Fatalf("hit tokens = %d, want 96", result.State.UsageMetrics.PromptCacheHitTokens)
	}
	if result.State.CacheHitRate != 0.96 {
		t.Fatalf("cache hit rate = %f, want 0.96", result.State.CacheHitRate)
	}
	if result.State.DeepSeekSession.SessionCacheHitTokens != 96 || result.State.DeepSeekSession.SessionCacheMissTokens != 4 {
		t.Fatalf("session cache = hit %d / miss %d, want 96 / 4", result.State.DeepSeekSession.SessionCacheHitTokens, result.State.DeepSeekSession.SessionCacheMissTokens)
	}
	if result.State.DeepSeekSession.SessionCacheHitRate != 0.96 {
		t.Fatalf("session cache hit rate = %f, want 0.96", result.State.DeepSeekSession.SessionCacheHitRate)
	}
	if result.State.DeepSeek.LastCheckStatus != "ok" {
		t.Fatalf("deepseek status = %q, want ok", result.State.DeepSeek.LastCheckStatus)
	}
	if !strings.Contains(result.State.DeepSeek.LastCheckMessage, "试聊成功") {
		t.Fatalf("deepseek message = %q, want chat success notice", result.State.DeepSeek.LastCheckMessage)
	}
}

func TestServiceRefreshOpenAICompatibleProviderModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "upstream-a"},
				{"id": "upstream-b"},
			},
		})
	}))
	defer server.Close()

	service := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	settings := service.WorkbenchState().RuntimeSettings
	for index, provider := range settings.Model.Providers {
		if provider.ID == "openai-compatible" {
			settings.Model.Providers[index].BaseURL = server.URL
			settings.Model.Providers[index].Enabled = true
		}
	}
	if _, err := service.SaveRuntimeSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveModelProviderAPIKey("openai-compatible", "sk-test"); err != nil {
		t.Fatal(err)
	}

	state, err := service.RefreshModelProviderModels(context.Background(), "openai-compatible")
	if err != nil {
		t.Fatal(err)
	}
	provider, _, ok := findModelProvider(state.RuntimeSettings.Model.Providers, "openai-compatible")
	if !ok {
		t.Fatal("openai-compatible provider missing")
	}
	if provider.LastSyncStatus != "ok" {
		t.Fatalf("sync status = %q, want ok: %s", provider.LastSyncStatus, provider.LastSyncMessage)
	}
	if len(provider.Models) != 2 || provider.DefaultModelID != "upstream-a" {
		t.Fatalf("provider models = %#v, default = %q", provider.Models, provider.DefaultModelID)
	}
	if !provider.APIKeyConfigured {
		t.Fatal("provider should report saved API key")
	}
}

func TestServiceMapsReasoningToDeepSeekThinking(t *testing.T) {
	var mu sync.Mutex
	type requestPayload struct {
		Model    string `json:"model"`
		Thinking *struct {
			Type string `json:"type"`
		} `json:"thinking"`
		ReasoningEffort string `json:"reasoning_effort"`
	}
	var payloads []requestPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var payload requestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		payloads = append(payloads, payload)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	service := NewService(ServiceConfig{
		SkillsDir:       t.TempDir(),
		DeepSeekBaseURL: server.URL,
	})
	if _, err := service.SaveDeepSeekAPIKey("sk-test"); err != nil {
		t.Fatal(err)
	}
	ultra, err := service.SendDeepSeekMessage(context.Background(), "ultra")
	if err != nil {
		t.Fatal(err)
	}
	if ultra.State.DeepSeekSession.ThinkingMode != "enabled" || ultra.State.DeepSeekSession.ReasoningEffort != "max" {
		t.Fatalf("ultra session thinking = %#v, want enabled/max", ultra.State.DeepSeekSession)
	}

	if _, err := service.SetReasoningLevel(ReasoningMedium); err != nil {
		t.Fatal(err)
	}
	medium, err := service.SendDeepSeekMessage(context.Background(), "medium")
	if err != nil {
		t.Fatal(err)
	}
	if medium.State.DeepSeekSession.ThinkingMode != "disabled" || medium.State.DeepSeekSession.ReasoningEffort != "" {
		t.Fatalf("medium session thinking = %#v, want disabled", medium.State.DeepSeekSession)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(payloads) != 2 {
		t.Fatalf("payload count = %d, want 2", len(payloads))
	}
	if payloads[0].Model != "deepseek-v4-pro" || payloads[0].Thinking == nil || payloads[0].Thinking.Type != "enabled" || payloads[0].ReasoningEffort != "max" {
		t.Fatalf("ultra payload = %#v, want v4-pro enabled/max", payloads[0])
	}
	if payloads[1].Model != "deepseek-v4-flash" || payloads[1].Thinking == nil || payloads[1].Thinking.Type != "disabled" || payloads[1].ReasoningEffort != "" {
		t.Fatalf("medium payload = %#v, want v4-flash disabled", payloads[1])
	}
}

func TestServiceSendDeepSeekMessageReusesAppendOnlySession(t *testing.T) {
	var mu sync.Mutex
	var requests [][]protocol.Message
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Messages []protocol.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mu.Lock()
		callCount++
		reply := fmt.Sprintf("answer-%d", callCount)
		requests = append(requests, cloneProtocolMessages(payload.Messages))
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", reply)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	service := NewService(ServiceConfig{
		SkillsDir:       t.TempDir(),
		DeepSeekBaseURL: server.URL,
	})
	if _, err := service.SaveDeepSeekAPIKey("sk-test"); err != nil {
		t.Fatal(err)
	}

	first, err := service.SendDeepSeekMessage(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	if first.Content != "answer-1" {
		t.Fatalf("first content = %q, want answer-1", first.Content)
	}
	if !first.State.DeepSeekSession.AppendOnlyPrefixStable || first.State.DeepSeekSession.PreviousRequestMessageCount != 0 {
		t.Fatalf("first prefix diagnostic = %#v, want stable first request with no previous request", first.State.DeepSeekSession)
	}
	second, err := service.SendDeepSeekMessage(context.Background(), "second")
	if err != nil {
		t.Fatal(err)
	}
	if second.Content != "answer-2" {
		t.Fatalf("second content = %q, want answer-2", second.Content)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("captured requests = %d, want 2", len(requests))
	}
	if len(requests[0]) != 2 {
		t.Fatalf("first request message count = %d, want 2", len(requests[0]))
	}
	assertMessage(t, requests[0], 0, "system", "")
	assertMessage(t, requests[0], 1, "user", "first")
	if !strings.Contains(requests[0][0].Content, "cache_prefix_hash=sha256:") {
		t.Fatalf("system prompt should include stable prefix hash: %q", requests[0][0].Content)
	}

	if len(requests[1]) != 4 {
		t.Fatalf("second request message count = %d, want 4", len(requests[1]))
	}
	assertMessage(t, requests[1], 0, "system", requests[0][0].Content)
	assertMessage(t, requests[1], 1, "user", "first")
	assertMessage(t, requests[1], 2, "assistant", "answer-1")
	assertMessage(t, requests[1], 3, "user", "second")
	if second.State.DeepSeekSession.TurnCount != 2 {
		t.Fatalf("turn count = %d, want 2", second.State.DeepSeekSession.TurnCount)
	}
	if second.State.DeepSeekSession.MessageCount != 5 {
		t.Fatalf("session message count = %d, want 5", second.State.DeepSeekSession.MessageCount)
	}
	if !second.State.DeepSeekSession.AppendOnlyPrefixStable {
		t.Fatalf("append-only prefix stable = false, want true")
	}
	if second.State.DeepSeekSession.PreviousRequestMessageCount != len(requests[0]) {
		t.Fatalf("previous request messages = %d, want %d", second.State.DeepSeekSession.PreviousRequestMessageCount, len(requests[0]))
	}
	if second.State.DeepSeekSession.CommonPrefixMessageCount != len(requests[0]) {
		t.Fatalf("common prefix messages = %d, want %d", second.State.DeepSeekSession.CommonPrefixMessageCount, len(requests[0]))
	}
}

func TestServiceResetDeepSeekSessionClearsSessionAndMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":12,\"total_tokens\":112,\"prompt_cache_hit_tokens\":80,\"prompt_cache_miss_tokens\":20}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	service := NewService(ServiceConfig{
		SkillsDir:       t.TempDir(),
		DeepSeekBaseURL: server.URL,
	})
	if _, err := service.SaveDeepSeekAPIKey("sk-test"); err != nil {
		t.Fatal(err)
	}
	result, err := service.SendDeepSeekMessage(context.Background(), "ping")
	if err != nil {
		t.Fatal(err)
	}
	if !result.State.DeepSeekSession.Active || result.State.DeepSeekSession.TurnCount != 1 {
		t.Fatalf("session state after send = %#v, want active one-turn session", result.State.DeepSeekSession)
	}
	if result.State.UsageMetrics.PromptCacheHitTokens != 80 {
		t.Fatalf("hit tokens = %d, want 80", result.State.UsageMetrics.PromptCacheHitTokens)
	}

	state, err := service.ResetDeepSeekSession()
	if err != nil {
		t.Fatal(err)
	}
	if state.DeepSeekSession.Active {
		t.Fatal("session should be inactive after reset")
	}
	if state.DeepSeekSession.MessageCount != 0 || state.DeepSeekSession.TurnCount != 0 {
		t.Fatalf("session counts after reset = messages %d turns %d, want 0/0", state.DeepSeekSession.MessageCount, state.DeepSeekSession.TurnCount)
	}
	if state.UsageMetrics.PromptCacheHitTokens != 0 || state.UsageMetrics.PromptCacheMissTokens != 0 {
		t.Fatalf("usage after reset = %#v, want zero cache tokens", state.UsageMetrics)
	}
	if state.CacheHitRate != 0 {
		t.Fatalf("cache hit rate after reset = %f, want 0", state.CacheHitRate)
	}
	if state.DeepSeekSession.SessionCacheHitTokens != 0 || state.DeepSeekSession.SessionCacheMissTokens != 0 || state.DeepSeekSession.SessionCacheHitRate != 0 {
		t.Fatalf("session cache after reset = %#v, want zero cache totals", state.DeepSeekSession)
	}
	if len(service.metricsHistory) != 0 {
		t.Fatalf("metrics history length after reset = %d, want 0", len(service.metricsHistory))
	}
}

func TestServiceSanitizesInternalPrefixLeak(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"stable_prefix: product_identity secret\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	service := NewService(ServiceConfig{
		SkillsDir:       t.TempDir(),
		DeepSeekBaseURL: server.URL,
	})
	if _, err := service.SaveDeepSeekAPIKey("sk-test"); err != nil {
		t.Fatal(err)
	}

	result, err := service.SendDeepSeekMessage(context.Background(), "ping")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Content, "stable_prefix") {
		t.Fatalf("content leaked internal prefix: %q", result.Content)
	}
	if !strings.Contains(result.Content, "已拦截") {
		t.Fatalf("content = %q, want interception notice", result.Content)
	}
}

func TestFormatStablePromptMarksInternalsAsHidden(t *testing.T) {
	ctx := NewContextBuilder().Build(StableContext{
		ProductIdentity: "MHcode",
		SystemRules:     []string{"rule"},
		Reasoning:       ReasoningProfile{ID: ReasoningUltra, Budget: ReasoningBudget{CachePolicy: "strict-stable-prefix"}},
		ProjectSummary:  "summary",
		RoutingPolicy:   "route",
	}, VolatileContext{UserInput: "hidden user input"})

	prompt := formatStablePrompt(ctx)
	if !strings.Contains(prompt, "Never quote") {
		t.Fatalf("prompt should forbid quoting internals: %q", prompt)
	}
	if strings.Contains(prompt, "hidden user input") {
		t.Fatalf("stable prompt should not include volatile user input: %q", prompt)
	}
	if strings.Contains(prompt, "skills_index") || strings.Contains(prompt, "mcp_schema_snapshot") {
		t.Fatalf("stable prompt should not expand long internal sections: %q", prompt)
	}
	if !strings.Contains(prompt, "cache_prefix_hash=sha256:") {
		t.Fatalf("stable prompt should keep prefix hash for observability: %q", prompt)
	}
}

func assertMessage(t *testing.T, messages []protocol.Message, index int, role string, content string) {
	t.Helper()
	if index >= len(messages) {
		t.Fatalf("message index %d out of range for %d messages", index, len(messages))
	}
	if messages[index].Role != role {
		t.Fatalf("message %d role = %q, want %q", index, messages[index].Role, role)
	}
	if content != "" && messages[index].Content != content {
		t.Fatalf("message %d content = %q, want %q", index, messages[index].Content, content)
	}
}
