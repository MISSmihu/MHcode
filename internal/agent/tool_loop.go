package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

// normalizeToolArgs 归一化模型返回的工具参数。
//
// OpenAI/DeepSeek function-calling 标准：tool_call 的 arguments 是「被 JSON 编码成字符串
// 的 JSON」，即 arguments 字段本身是一个字符串，如 "{\"path\":\".\"}"。
// 我们收到的 json.RawMessage 因此是带引号的字符串字面量，直接 Unmarshal 进 struct 会报
// "cannot unmarshal string into struct"。这里把这层字符串解开，还原为对象字节。
// 若本就是对象（个别 provider 直接给对象），原样返回。
func normalizeToolArgs(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return json.RawMessage("{}")
	}
	// 已是 JSON 对象/数组 → 原样返回。
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return raw
	}
	// 是被编码的字符串 → 先解出字符串内容（里面才是真正的 JSON）。
	if trimmed[0] == '"' {
		var inner string
		if err := json.Unmarshal(raw, &inner); err == nil {
			inner = strings.TrimSpace(inner)
			if inner == "" {
				return json.RawMessage("{}")
			}
			return json.RawMessage(inner)
		}
	}
	return raw
}

// runToolLoopTurn 在工具循环之外套上一层会话簿记（与流式路径的收尾逻辑对齐）：
// 构造注册表、跑循环、写回 session、更新计数与指标、组装 ChatResult。
func (s *Service) runToolLoopTurn(
	ctx context.Context,
	streamProvider protocol.Provider,
	caller protocol.ToolCaller,
	baseRequest protocol.ChatRequest,
	route chatRoute,
	prefixDiagnostic requestPrefixDiagnostic,
	requestMessages []protocol.Message,
	baseMessageCount int,
	sink ChatEventSink,
) (ChatResult, error) {
	sink = serializedChatEventSink(sink)
	execRequest := baseRequest
	planStarted := false
	reg := s.buildToolRegistryForContext(ctx)
	preflight := s.runDeploymentSSHPreflight(ctx, reg, baseRequest.Messages, sink)
	if preflight.Attempted {
		execRequest.Messages = appendDeploymentSSHPreflight(baseRequest.Messages, preflight)
	}

	// Plan 两段式：需用户显式开启 Plan 模式，且当前档位 Planner=true 时启用。
	// 默认关闭，避免每轮翻倍调用、破坏缓存经济性（符合真实工具的显式规划做法）。
	profile, _ := ReasoningProfileFor(s.reasoning)
	if s.planMode && !isGuidanceChatTurn(ctx) && profile.Budget.Planner && strings.TrimSpace(s.runtimeSettings.WorkspaceRoot) != "" {
		emitChatEvent(sink, ChatStreamEvent{Type: "status", Message: "正在生成执行计划", Model: route.ModelID})
		planRequest := baseRequest
		if preflight.Attempted {
			planRequest.Messages = appendDeploymentSSHPreflightSummary(baseRequest.Messages, preflight)
		}
		plan, _, planErr := s.runPlanPhase(ctx, caller, planRequest, route, sink)
		if planErr != nil {
			s.sessionMessages = s.sessionMessages[:baseMessageCount]
			s.markChatProviderStatus(route.Provider.ID, "error", planErr.Error())
			return ChatResult{State: s.workbenchStateLocked(), Model: route.ModelID, Parts: preflight.Result.Parts}, planErr
		}
		if plan != "" {
			planSteps := planStepsFromText(plan)
			if len(planSteps) > 0 {
				if planStateErr := s.startPlanState(planSteps); planStateErr != nil {
					s.sessionMessages = s.sessionMessages[:baseMessageCount]
					return ChatResult{State: s.workbenchStateLocked(), Model: route.ModelID, Parts: preflight.Result.Parts}, planStateErr
				}
				planStarted = true
			}
			approved, apprErr := s.requestPlanApproval(ctx, plan)
			if apprErr != nil {
				s.sessionMessages = s.sessionMessages[:baseMessageCount]
				apprErr = s.failStartedPlan(planStarted, apprErr)
				return ChatResult{State: s.workbenchStateLocked(), Model: route.ModelID, Parts: preflight.Result.Parts}, apprErr
			}
			if !approved {
				if planStarted {
					if planStateErr := s.finishPlanState("cancelled"); planStateErr != nil {
						return ChatResult{State: s.workbenchStateLocked(), Model: route.ModelID}, planStateErr
					}
				}
				// 用户否决计划：不执行，直接把计划作为回复返回。
				answer := "已生成执行计划，但你选择不执行。\n\n" + plan
				parts := mergeOutcomeParts(append([]tools.ResultPart{}, preflight.Result.Parts...), []tools.ResultPart{{Kind: tools.PartText, Text: plan}})
				s.sessionMessages = s.appendProtocolAssistantMessage(s.sessionMessages, answer, parts)
				s.sessionState.MessageCount = len(s.sessionMessages)
				s.recordAssistantAndCheckpoint(answer, route.ModelID, parts, chatTurnDurationMs(ctx))
				s.markChatProviderStatus(route.Provider.ID, "ok", "计划已生成，等待下一步。")
				return ChatResult{
					Content: answer,
					Model:   route.ModelID,
					Usage:   s.metrics,
					State:   s.workbenchStateLocked(),
					Parts:   parts,
				}, nil
			}
			if len(planSteps) > 0 {
				planSteps[0].Status = "in_progress"
				if planStateErr := s.updatePlanState(planSteps); planStateErr != nil {
					planStateErr = s.failStartedPlan(planStarted, planStateErr)
					return ChatResult{State: s.workbenchStateLocked(), Model: route.ModelID, Parts: preflight.Result.Parts}, planStateErr
				}
			}
			// 批准：把计划注入执行阶段的消息尾部，让模型据此执行。
			execRequest.Messages = append(append([]protocol.Message{}, execRequest.Messages...),
				protocol.Message{Role: "assistant", Content: "执行计划：\n" + plan},
				protocol.Message{Role: "user", Content: "计划已批准，请按计划执行。"})
		}
	}

	emitChatEvent(sink, ChatStreamEvent{Type: "status", Message: "正在分析任务", Model: route.ModelID})
	executionCtx := withSubagentExecutionScope(ctx, subagentExecutionScope{
		BaseRequest:  execRequest,
		PrimaryRoute: route,
	})
	usageObserver := func(usage *protocol.TokenUsage) {
		usageRoute := resolvedProviderRoute(streamProvider, route)
		s.recordLiveUsage(usage, usageRoute, sink)
	}
	outcome, err := s.runStreamingToolLoopWithState(executionCtx, streamProvider, reg, execRequest, sink, preflight, usageObserver)
	resolvedRoute := resolvedProviderRoute(streamProvider, route)
	if resolvedRoute.Provider.ID != route.Provider.ID {
		route = resolvedRoute
		s.adoptProviderRoute(route)
	}
	if err != nil {
		s.sessionMessages = s.sessionMessages[:baseMessageCount]
		s.markChatProviderStatus(route.Provider.ID, "error", err.Error())
		cancelled := chatTurnWasCancelled(ctx, err)
		if planStarted {
			if cancelled {
				if planErr := s.finishPlanState("cancelled"); planErr != nil {
					err = errors.Join(err, fmt.Errorf("cancel plan: %w", planErr))
				}
			} else {
				err = s.failStartedPlan(planStarted, err)
			}
		}
		terminalStatus := "failed"
		if cancelled {
			terminalStatus = "cancelled"
		}
		setOutcomeProgressStatus(&outcome, terminalStatus)
		emitOutcomeProgress(sink, outcome.Parts)

		partialContent := partialToolEvidenceContent(outcome)
		if partialContent == "" && !cancelled {
			partialContent = partialToolFailureContent(outcome)
		}
		if partialContent == "" && (strings.TrimSpace(outcome.Reasoning) != "" || hasMeaningfulResultParts(outcome.Parts)) {
			partialContent = retainedTurnContent(terminalStatus, "", outcome.Parts, outcome.Reasoning)
		}
		partialParts := appendTextPartIfMissing(outcome.Parts, partialContent)
		result := ChatResult{
			Content:   partialContent,
			Reasoning: outcome.Reasoning,
			Model:     route.ModelID,
			Usage:     s.metrics,
			State:     s.workbenchStateLocked(),
			Parts:     partialParts,
		}
		s.retainInterruptedTurn(&result, terminalStatus, requestMessages, baseMessageCount, prefixDiagnostic)
		return result, err
	}

	if planStarted {
		if planStateErr := s.finishPlanState("completed"); planStateErr != nil {
			return ChatResult{State: s.workbenchStateLocked(), Model: route.ModelID}, planStateErr
		}
	}

	answer := sanitizeModelContent(outcome.Content)
	s.sessionMessages = s.appendProtocolAssistantMessage(s.sessionMessages, answer, outcome.Parts)
	s.commitRequestPrefix(prefixDiagnostic, requestMessages)
	s.sessionState.MessageCount = len(s.sessionMessages)
	s.sessionState.TurnCount++
	// 文件快照已在每次工具写入后立即记录，这里只提交 assistant + checkpoint。
	s.recordAssistantAndCheckpoint(answer, route.ModelID, outcome.Parts, chatTurnDurationMs(ctx))
	s.markChatProviderStatus(route.Provider.ID, "ok", fmt.Sprintf("试聊成功，%s / %s 工具会话完成，产出 %d 个片段。", route.Provider.Name, route.ModelID, len(outcome.Parts)))

	return ChatResult{
		Content:   answer,
		Reasoning: outcome.Reasoning,
		Model:     route.ModelID,
		Usage:     s.metrics,
		State:     s.workbenchStateLocked(),
		Parts:     outcome.Parts,
	}, nil
}

func partialToolFailureContent(outcome toolLoopOutcome) string {
	if content := partialToolEvidenceContent(outcome); content != "" {
		return content
	}
	if hasMeaningfulResultParts(outcome.Parts) {
		return "工具结果已经保留，但本轮任务尚未形成可用结论。请重试，或根据执行记录继续调查。"
	}
	return ""
}

func partialToolEvidenceContent(outcome toolLoopOutcome) string {
	if content := sanitizeModelContent(outcome.Content); content != "" {
		return content
	}
	for _, part := range outcome.Parts {
		if isSuccessfulRepositoryRead(part) {
			return repositoryReadFallbackContent(part)
		}
	}
	return ""
}

func hasUsablePartialToolResult(parts []tools.ResultPart) bool {
	return hasSuccessfulRepositoryRead(parts) || hasSuccessfulWebpageRead(parts)
}

func hasSuccessfulRepositoryRead(parts []tools.ResultPart) bool {
	for _, part := range parts {
		if isSuccessfulRepositoryRead(part) {
			return true
		}
	}
	return false
}

func isSuccessfulRepositoryRead(part tools.ResultPart) bool {
	return part.Kind == tools.PartToolCall &&
		part.Name == "read_repository" &&
		part.Status != "error" &&
		strings.TrimSpace(part.Output) != ""
}

func hasSuccessfulWebpageRead(parts []tools.ResultPart) bool {
	for _, part := range parts {
		if isSuccessfulWebpageRead(part) {
			return true
		}
	}
	return false
}

func isSuccessfulWebpageRead(part tools.ResultPart) bool {
	return part.Kind == tools.PartToolCall &&
		part.Name == "read_webpage" &&
		part.Status != "error" &&
		strings.TrimSpace(part.Output) != ""
}

func hasWebSearchSources(parts []tools.ResultPart) bool {
	for _, part := range parts {
		if part.Kind == tools.PartWebSearch && len(part.Sources) > 0 {
			return true
		}
	}
	return false
}

func webSearchFallbackContent(part tools.ResultPart) string {
	var content strings.Builder
	content.WriteString("本轮未完成最终分析。以下仅为原始网络搜索记录，不能视为结论：")
	for index, source := range part.Sources {
		title := strings.TrimSpace(source.Title)
		if title == "" {
			title = strings.TrimSpace(source.URL)
		}
		fmt.Fprintf(&content, "\n\n%d. %s", index+1, title)
		if snippet := compactFallbackSnippet(source.Snippet, 220); snippet != "" {
			content.WriteString("\n   ")
			content.WriteString(snippet)
		}
		if source.URL != "" {
			content.WriteString("\n   ")
			content.WriteString(strings.TrimSpace(source.URL))
		}
	}
	return content.String()
}

func ensureWebSearchSourcesListed(content string, parts []tools.ResultPart) string {
	content = strings.TrimSpace(content)
	sources := make([]tools.SearchSource, 0)
	seen := map[string]bool{}
	for _, part := range parts {
		if part.Kind != tools.PartWebSearch {
			continue
		}
		for _, source := range part.Sources {
			url := strings.TrimSpace(source.URL)
			if url == "" || seen[url] {
				continue
			}
			seen[url] = true
			source.URL = url
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		return content
	}

	missing := make([]tools.SearchSource, 0, len(sources))
	for _, source := range sources {
		title := strings.TrimSpace(source.Title)
		hasURL := strings.Contains(content, source.URL)
		hasTitle := sourceTitleIsMentioned(content, title)
		// Search engines can return loosely related results. Only surface a URL
		// automatically when the model has actually named that source; the full
		// search record remains available in the activity stream for inspection.
		if hasTitle && !hasURL {
			missing = append(missing, source)
		}
	}
	if len(missing) == 0 {
		return content
	}

	var result strings.Builder
	if content != "" {
		result.WriteString(content)
		result.WriteString("\n\n")
	}
	if len(missing) == len(sources) {
		result.WriteString("来源链接：")
	} else {
		result.WriteString("补充来源链接：")
	}
	for _, source := range missing {
		title := strings.TrimSpace(source.Title)
		if title == "" {
			title = source.URL
		}
		result.WriteString("\n- ")
		result.WriteString(title)
		result.WriteString("\n  <")
		result.WriteString(strings.ReplaceAll(source.URL, ">", "%3E"))
		result.WriteString(">")
	}
	return result.String()
}

func sourceTitleIsMentioned(content, title string) bool {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) < 4 {
		return false
	}
	return strings.Contains(strings.ToLower(content), strings.ToLower(title))
}

func compactFallbackSnippet(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if maxRunes > 0 && len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return value
}

func repositoryReadFallbackContent(part tools.ResultPart) string {
	output := strings.TrimSpace(strings.ReplaceAll(part.Output, "\r\n", "\n"))
	if output == "" {
		return ""
	}

	metadata, readme, tree, entries, fileContent := splitRepositoryFallbackOutput(output)
	var content strings.Builder
	content.WriteString("GitHub 仓库读取已完成，但上游模型未能继续整理。以下内容直接来自仓库工具结果：")
	if repositoryURL := strings.TrimSpace(part.Input); repositoryURL != "" {
		content.WriteString("\n\n仓库地址：<")
		content.WriteString(strings.ReplaceAll(repositoryURL, ">", "%3E"))
		content.WriteString(">")
	}
	appendRepositoryFallbackSection(&content, "仓库信息", metadata, 1_200)
	appendRepositoryFallbackSection(&content, "README（截取）", readme, 3_200)
	appendRepositoryFallbackSection(&content, "目录树（截取）", tree, 2_400)
	appendRepositoryFallbackSection(&content, "目录内容（截取）", entries, 2_400)
	appendRepositoryFallbackSection(&content, "文件内容（截取）", fileContent, 4_000)
	content.WriteString("\n\n完整原始结果仍保留在上方“读取仓库”记录中，可展开查看或直接重试整理。")
	return content.String()
}

func webpageReadFallbackContent(part tools.ResultPart) string {
	output := compactMultilineFallback(strings.TrimSpace(strings.ReplaceAll(part.Output, "\r\n", "\n")), 7_000)
	if output == "" {
		return ""
	}
	return "已读取网页正文。以下内容直接来自网页读取结果，可展开上方记录查看完整内容：\n\n" + output
}

func splitRepositoryFallbackOutput(output string) (metadata, readme, tree, entries, fileContent string) {
	metadata = strings.TrimSpace(output)
	if before, after, ok := strings.Cut(metadata, "\nREADME:\n"); ok {
		metadata = strings.TrimSpace(before)
		readme = strings.TrimSpace(after)
		if beforeTree, afterTree, hasTree := strings.Cut(readme, "\nRepository tree:\n"); hasTree {
			readme = strings.TrimSpace(beforeTree)
			tree = strings.TrimSpace(afterTree)
		}
		return
	}
	if before, after, ok := strings.Cut(metadata, "\nRepository tree:\n"); ok {
		metadata = strings.TrimSpace(before)
		tree = strings.TrimSpace(after)
		return
	}
	if before, after, ok := strings.Cut(metadata, "\nEntries:\n"); ok {
		metadata = strings.TrimSpace(before)
		entries = strings.TrimSpace(after)
		return
	}
	if before, after, ok := strings.Cut(metadata, "\n\n"); ok {
		metadata = strings.TrimSpace(before)
		fileContent = strings.TrimSpace(after)
	}
	return
}

func appendRepositoryFallbackSection(content *strings.Builder, title, value string, maxRunes int) {
	value = compactMultilineFallback(value, maxRunes)
	if value == "" {
		return
	}
	content.WriteString("\n\n**")
	content.WriteString(title)
	content.WriteString("**\n\n")
	for _, line := range strings.Split(value, "\n") {
		content.WriteString("    ")
		content.WriteString(line)
		content.WriteByte('\n')
	}
}

func compactMultilineFallback(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if maxRunes > 0 && len(runes) > maxRunes {
		return strings.TrimSpace(string(runes[:maxRunes])) + "\n... [内容已截取]"
	}
	return value
}

func appendTextPartIfMissing(parts []tools.ResultPart, content string) []tools.ResultPart {
	content = strings.TrimSpace(content)
	if content == "" {
		return parts
	}
	for _, part := range parts {
		if part.Kind == tools.PartText && strings.TrimSpace(part.Text) == content {
			return parts
		}
	}
	return append(parts, tools.ResultPart{Kind: tools.PartText, Text: content})
}

// buildToolRegistry 依据当前 runtime settings 构造工具注册表。
// 沙盒策略从 RuntimeSettings 映射到 tools.SandboxPolicy，实现权限边界。
func (s *Service) buildToolRegistry() *tools.Registry {
	return s.buildToolRegistryForContext(context.Background())
}

func (s *Service) buildToolRegistryForContext(ctx context.Context) *tools.Registry {
	return s.buildMutableToolRegistryForContext(ctx, true, true)
}

// buildWorkerToolRegistry is used by delegated and fixed-team workers. It
// excludes coordinator tools so a worker cannot mutate the main plan or spawn
// another layer of agents.
func (s *Service) buildWorkerToolRegistry() *tools.Registry {
	return s.buildWorkerToolRegistryForContext(context.Background())
}

func (s *Service) buildMutableToolRegistry(includePlan, includeDelegation bool) *tools.Registry {
	return s.buildMutableToolRegistryForContext(context.Background(), includePlan, includeDelegation)
}

func (s *Service) buildWorkerToolRegistryForContext(ctx context.Context) *tools.Registry {
	return s.buildMutableToolRegistryForContext(ctx, false, false)
}

func (s *Service) buildMutableToolRegistryForContext(ctx context.Context, includePlan, includeDelegation bool) *tools.Registry {
	policy := s.sandboxPolicyForContext(ctx)
	reg := tools.NewRegistry()
	if includePlan {
		reg.Add(tools.UpdatePlanTool{OnUpdate: s.updatePlanState})
	}
	reg.Add(tools.ReadFileTool{Policy: policy})
	reg.Add(tools.FileInfoTool{Policy: policy})
	reg.Add(tools.ListDirTool{Policy: policy})
	reg.Add(tools.SearchTool{Policy: policy})
	if s.config.ArtifactRenderer != nil {
		reg.Add(tools.RenderArtifactTool{Policy: policy, Controller: s})
		reg.Add(tools.InspectVisualTool{Policy: policy, Controller: s})
	}
	if s.runtimeSettings.NetworkAccess {
		reg.Add(tools.ReadRepositoryTool{Policy: policy})
		reg.Add(tools.ReadWebpageTool{Policy: policy, Browser: s.webpageBrowserRenderer()})
		reg.Add(tools.WebSearchTool{Policy: policy})
		reg.Add(tools.DownloadFileTool{Policy: policy})
		reg.Add(tools.GitRepositoryTool{Policy: policy})
	}
	if s.config.OpenFile != nil || s.config.PreviewFile != nil {
		reg.Add(tools.OpenFileTool{Policy: policy, Open: s.config.OpenFile, Preview: s.config.PreviewFile})
	}
	if s.config.Browser != nil && s.runtimeSettings.Browser.Enabled {
		reg.Add(tools.BrowserTool{Policy: policy, Controller: s.config.Browser})
	}
	computerSettings := s.runtimeSettings.ComputerControl
	if s.config.Computer != nil && (computerSettings.AnyAppEnabled || computerSettings.ChromeEnabled || len(computerSettings.AlwaysAllowedApps) > 0) {
		reg.Add(tools.ComputerTool{Policy: policy, Controller: s.config.Computer})
	}
	if includeDelegation {
		reg.Add(DelegateTaskTool{Service: s})
		reg.Add(AwaitSubagentsTool{Service: s})
	}
	reg.Add(tools.WriteFileTool{Policy: policy})
	reg.Add(tools.ApplyPatchTool{Policy: policy})
	reg.Add(tools.CopyFileTool{Policy: policy})
	reg.Add(tools.DeleteFileTool{Policy: policy})
	// run_command 仅在 ShellAccess 开启时注册（文件操作绝不经 shell）。
	if s.runtimeSettings.ShellAccess {
		reg.Add(tools.RunCommandTool{Policy: policy})
	}
	if s.runtimeSettings.NetworkAccess && s.runtimeSettings.ShellAccess {
		reg.Add(SSHCredentialTool{
			Policy:         policy,
			Resolve:        s.resolveScopedSSHCredential,
			CaptureSecret:  s.storeSecretResult,
			KnownHostsPath: s.scopedSSHKnownHostsPath(),
		})
	}
	if s.config.Git != nil {
		reg.Add(GitTool{Policy: policy, Controller: s.config.Git})
	}
	if s.config.Terminal != nil && s.runtimeSettings.ShellAccess {
		reg.Add(TerminalTool{Policy: policy, Controller: s.config.Terminal})
	}
	if s.mcpManager != nil {
		for _, remoteTool := range s.mcpManager.Tools() {
			reg.Add(remoteTool)
		}
	}
	if s.pluginManager != nil {
		for _, pluginTool := range s.pluginManager.Tools(s.runtimeSettings.Plugins, policy, false) {
			reg.Add(pluginTool)
		}
	}
	return reg
}

// buildReadOnlyRegistry 只含只读工具，供 Plan 阶段「先探索、不改动」使用。
func (s *Service) buildReadOnlyRegistry() *tools.Registry {
	return s.buildReadOnlyRegistryForContext(context.Background())
}

func (s *Service) buildReadOnlyRegistryForContext(ctx context.Context) *tools.Registry {
	policy := s.sandboxPolicyForContext(ctx)
	policy.SandboxMode = "read-only"
	policy.FilesystemAccess = "read-only"
	policy.ShellAccess = false
	policy.AllowDestructiveOps = false
	reg := tools.NewRegistry(
		tools.ReadFileTool{Policy: policy},
		tools.FileInfoTool{Policy: policy},
		tools.ListDirTool{Policy: policy},
		tools.SearchTool{Policy: policy},
	)
	if s.config.ArtifactRenderer != nil {
		reg.Add(tools.RenderArtifactTool{Policy: policy, Controller: s})
		reg.Add(tools.InspectVisualTool{Policy: policy, Controller: s})
	}
	if s.config.Git != nil {
		reg.Add(GitTool{Policy: policy, Controller: s.config.Git, ReadOnlyOnly: true})
	}
	if s.runtimeSettings.NetworkAccess {
		reg.Add(tools.ReadRepositoryTool{Policy: policy})
		reg.Add(tools.ReadWebpageTool{Policy: policy, Browser: s.webpageBrowserRenderer()})
		reg.Add(tools.WebSearchTool{Policy: policy})
	}
	if s.mcpManager != nil {
		for _, remoteTool := range s.mcpManager.Tools() {
			readOnlyTool, ok := remoteTool.(interface{ ReadOnly() bool })
			if ok && readOnlyTool.ReadOnly() {
				reg.Add(remoteTool)
			}
		}
	}
	if s.pluginManager != nil {
		for _, pluginTool := range s.pluginManager.Tools(s.runtimeSettings.Plugins, policy, true) {
			reg.Add(pluginTool)
		}
	}
	return reg
}

func (s *Service) webpageBrowserRenderer() tools.WebpageBrowserRenderer {
	if s.config.Browser == nil || !s.runtimeSettings.Browser.Enabled {
		return nil
	}
	renderer, _ := s.config.Browser.(tools.WebpageBrowserRenderer)
	return renderer
}

// toolDefinitions 把注册表的工具 schema 转成 provider 需要的 protocol.ToolDefinition。
func toolDefinitions(reg *tools.Registry) []protocol.ToolDefinition {
	schemas := reg.Schemas()
	defs := make([]protocol.ToolDefinition, 0, len(schemas))
	for _, sc := range schemas {
		defs = append(defs, protocol.ToolDefinition{
			Type: sc.Type,
			Function: protocol.ToolDefinitionFunc{
				Name:        sc.Function.Name,
				Description: sc.Function.Description,
				Parameters:  sc.Function.Parameters,
			},
		})
	}
	return defs
}

// toolLoopOutcome 汇总一次工具循环的最终结果。
type toolLoopOutcome struct {
	Content      string
	Reasoning    string
	Parts        []tools.ResultPart
	Changes      []tools.FileChange
	Usage        *protocol.TokenUsage
	UsageSamples []protocol.TokenUsage
}

// runToolLoop 执行「补全 → 若有 tool_calls 则执行并回喂 → 再补全」的循环，
// 直到模型给出最终文本、用户取消或循环保护判定没有继续执行价值。
// caller 为支持 function-calling 的 provider；baseMessages 为初始对话（含用户输入）。
func (s *Service) runToolLoop(
	ctx context.Context,
	caller protocol.ToolCaller,
	reg *tools.Registry,
	req protocol.ChatRequest,
) (toolLoopOutcome, error) {
	return s.runToolLoopWithCompletion(ctx, reg, req, caller.Complete, nil)
}

func (s *Service) runStreamingToolLoop(
	ctx context.Context,
	provider protocol.Provider,
	reg *tools.Registry,
	req protocol.ChatRequest,
	sink ChatEventSink,
) (toolLoopOutcome, error) {
	return s.runStreamingToolLoopWithState(ctx, provider, reg, req, sink, deploymentSSHPreflight{}, nil)
}

type usageObserver func(*protocol.TokenUsage)

func (s *Service) runStreamingToolLoopWithState(
	ctx context.Context,
	provider protocol.Provider,
	reg *tools.Registry,
	req protocol.ChatRequest,
	sink ChatEventSink,
	preflight deploymentSSHPreflight,
	observeUsage usageObserver,
) (toolLoopOutcome, error) {
	complete := func(ctx context.Context, request protocol.ChatRequest) (protocol.CompletionResult, error) {
		completion, err := collectProviderStream(ctx, provider, request, sink)
		if completion.Usage != nil && observeUsage != nil {
			usage := *completion.Usage
			observeUsage(&usage)
		}
		return completion, err
	}
	return s.runToolLoopWithCompletionState(ctx, reg, req, complete, sink, preflight)
}

type completionFunc func(context.Context, protocol.ChatRequest) (protocol.CompletionResult, error)

const postToolCompletionRetries = 2

const toolResultRecoveryKind = "tool-result-recovery"

var errEmptyToolResultSynthesis = errors.New("模型在工具执行后没有返回可用结论")

const (
	toolLoopCycleRepetitions = 3
	toolLoopMaxCyclePeriod   = 3
)

type completedWebSearch struct {
	query   string
	sources []tools.SearchSource
}

type toolLoopGuard struct {
	searches                   []completedWebSearch
	browserUnavailable         bool
	browserExplicitlyRequested bool
	externalBrowserSuppressed  bool
	remoteLookup               bool
	completedSSHCalls          map[string]bool
	lastPlanSignature          string
	planUpdates                int
	objectiveSatisfied         bool
	forceFinalResponse         bool
	maxPlanUpdates             int
	failureStrategy            failureStrategyState
	resolvedFailures           map[string]bool
	blockedFailures            map[string]int
	turnIndex                  int
	cycles                     toolLoopCycleGuard
}

type toolLoopCycleGuard struct {
	history []string
}

type toolLoopRoundRecord struct {
	Name      string
	Arguments json.RawMessage
	Result    tools.Result
}

type stableToolLoopRecord struct {
	Name        string                     `json:"name"`
	Arguments   string                     `json:"arguments"`
	Summary     string                     `json:"summary"`
	IsError     bool                       `json:"isError"`
	Parts       []tools.ResultPart         `json:"parts,omitempty"`
	Changes     []stableToolLoopChange     `json:"changes,omitempty"`
	Attachments []stableToolLoopAttachment `json:"attachments,omitempty"`
}

type stableToolLoopChange struct {
	Path            string `json:"path"`
	BeforeHash      string `json:"beforeHash"`
	AfterHash       string `json:"afterHash"`
	Existed         bool   `json:"existed"`
	Deleted         bool   `json:"deleted"`
	LineEnding      string `json:"lineEnding"`
	Encoding        string `json:"encoding"`
	HadBOM          bool   `json:"hadBom"`
	AfterLineEnding string `json:"afterLineEnding"`
	AfterEncoding   string `json:"afterEncoding"`
	AfterHadBOM     bool   `json:"afterHadBom"`
}

type stableToolLoopAttachment struct {
	Name     string `json:"name"`
	MIMEType string `json:"mimeType"`
	DataHash string `json:"dataHash"`
}

func newAwaitSubagentsToolCall() protocol.ToolCall {
	return protocol.ToolCall{
		ID:   fmt.Sprintf("await-subagents-%d", time.Now().UnixNano()),
		Type: "function",
		Function: protocol.ToolCallFunction{
			Name:      "await_subagents",
			Arguments: json.RawMessage(`{}`),
		},
	}
}

const (
	maxLookupPlanUpdates = 4
)

func (g *toolLoopGuard) before(call protocol.ToolCall) (tools.Result, protocol.Message, bool, bool) {
	name := call.Function.Name
	input := toolInputForDisplay(name, call.Function.Arguments)
	if name == "update_plan" {
		signature := normalizedToolArguments(call.Function.Arguments)
		if signature != "" && signature == g.lastPlanSignature {
			summary := "计划内容与状态没有变化，已跳过重复更新。"
			return tools.Result{Summary: summary}, protocol.Message{Role: "tool", ToolCallID: call.ID, Name: name, Content: summary}, true, true
		}
		if g.maxPlanUpdates > 0 && g.planUpdates >= g.maxPlanUpdates {
			summary := "计划更新过于频繁，已保留最近一次有效状态；请继续执行或直接总结。"
			g.forceFinalResponse = true
			return tools.Result{Summary: summary}, protocol.Message{Role: "tool", ToolCallID: call.ID, Name: name, Content: summary}, true, true
		}
	}
	if result, message, blocked := g.beforeEquivalentFailure(call); blocked {
		return result, message, true, false
	}
	if name == "ssh" {
		signature := sshToolCallSignature(call.Function.Arguments)
		if signature != "" && g.completedSSHCalls[signature] && (g.remoteLookup || strings.HasPrefix(signature, "test\x00")) {
			summary := "相同的 SSH 操作本轮已经成功执行，已跳过重复调用；请复用先前结果继续任务。"
			if g.remoteLookup {
				g.forceFinalResponse = true
			}
			return tools.Result{Summary: summary}, protocol.Message{Role: "tool", ToolCallID: call.ID, Name: name, Content: summary}, true, true
		}
	}
	if name == "browser" && !g.browserExplicitlyRequested {
		rawArgs := normalizeToolArgs(call.Function.Arguments)
		if g.externalBrowserSuppressed || browserNavigationNeedsApproval(rawArgs) {
			g.externalBrowserSuppressed = true
			sourceCount := 0
			for _, search := range g.searches {
				sourceCount += len(search.sources)
			}
			summary := "网页未打开：用户没有要求浏览器导航。若需检索公开信息，请使用 web_search，并直接整理回答、列出来源标题和完整 URL。"
			if sourceCount > 0 {
				summary = fmt.Sprintf("网页未打开：用户没有要求浏览器导航。本轮已获得 %d 条搜索来源，请直接基于来源整理完整回答，并逐条列出标题和完整 URL；不要再次调用 browser。", sourceCount)
			}
			return tools.Result{Summary: summary}, protocol.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       name,
				Content:    summary,
			}, true, true
		}
	}
	if name == "browser" && g.browserUnavailable {
		summary := "内置浏览器本轮仍不可用，已跳过重复启动。请使用已有搜索来源完成回答。"
		result := tools.Result{
			Summary: summary,
			IsError: true,
			Parts: []tools.ResultPart{{
				Kind: tools.PartToolCall, Name: name, Status: "error", Input: input, Output: summary,
			}},
		}
		return result, protocol.Message{Role: "tool", ToolCallID: call.ID, Name: name, Content: summary}, true, false
	}
	if name != "web_search" || strings.TrimSpace(input) == "" {
		return tools.Result{}, protocol.Message{}, false, false
	}
	for _, search := range g.searches {
		if !searchQueriesSimilar(input, search.query) {
			continue
		}
		summary := fmt.Sprintf("本轮已完成相近搜索 %q，复用已有 %d 条来源，未重复联网。", search.query, len(search.sources))
		result := tools.Result{
			Summary: summary,
			Parts: []tools.ResultPart{
				{Kind: tools.PartToolCall, Name: name, Status: "ok", Input: input, Output: summary},
				{Kind: tools.PartWebSearch, Query: search.query, Sources: append([]tools.SearchSource(nil), search.sources...)},
			},
		}
		return result, protocol.Message{Role: "tool", ToolCallID: call.ID, Name: name, Content: summary}, true, false
	}
	return tools.Result{}, protocol.Message{}, false, false
}

func (g *toolLoopGuard) after(call protocol.ToolCall, result tools.Result, message *protocol.Message) {
	if result.IsError {
		record, diagnosis := g.failureStrategy.observeFailure(call, result, g.turnIndex)
		if record.StrategyKey != "" {
			feedback := failureDiagnosticContent("change_strategy_before_retry", record, diagnosis)
			message.Content = strings.TrimSpace(message.Content + "\n\nMHcode structured failure diagnosis:\n" + feedback)
			if record.Retryable && record.Attempts >= equivalentFailureRepetitions {
				g.forceFinalResponse = true
			}
		}
	} else {
		g.failureStrategy.observeSuccess(call, result, g.resolvedFailures)
	}
	switch call.Function.Name {
	case "update_plan":
		if result.IsError {
			return
		}
		g.lastPlanSignature = normalizedToolArguments(call.Function.Arguments)
		g.planUpdates++
	case "ssh":
		if result.IsError {
			return
		}
		if g.completedSSHCalls == nil {
			g.completedSSHCalls = make(map[string]bool)
		}
		if signature := sshToolCallSignature(call.Function.Arguments); signature != "" {
			g.completedSSHCalls[signature] = true
		}
		for _, part := range result.Parts {
			if part.Kind == tools.PartSecretResult && part.SecretID != "" {
				g.objectiveSatisfied = true
				g.forceFinalResponse = true
				break
			}
		}
	case "web_search":
		if result.IsError {
			return
		}
		sources := searchSourcesFromParts(result.Parts)
		if len(sources) == 0 {
			return
		}
		g.searches = append(g.searches, completedWebSearch{
			query:   toolInputForDisplay("web_search", call.Function.Arguments),
			sources: sources,
		})
	case "browser":
		if !result.IsError || !browserEngineUnavailable(result.Summary) {
			return
		}
		g.browserUnavailable = true
		message.Content = strings.TrimSpace(message.Content + "\n本轮不要再次调用 browser；如已有搜索来源，请直接基于来源完成回答。")
	}
}

const equivalentFailureRepetitions = 3

func equivalentToolArgumentShape(raw json.RawMessage) string {
	var value any
	if err := json.Unmarshal(normalizeToolArgs(raw), &value); err != nil {
		return normalizeEquivalentText(string(raw))
	}
	value = normalizeEquivalentValue(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return normalizeEquivalentText(string(raw))
	}
	return string(encoded)
}

func normalizeEquivalentValue(value any) any {
	switch typed := value.(type) {
	case string:
		return normalizeEquivalentText(typed)
	case []any:
		items := make([]any, len(typed))
		for index, item := range typed {
			items[index] = normalizeEquivalentValue(item)
		}
		return items
	case map[string]any:
		items := make(map[string]any, len(typed))
		for key, item := range typed {
			items[key] = normalizeEquivalentValue(item)
		}
		return items
	default:
		return value
	}
}

func normalizeEquivalentText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("\"", "", "'", "", "`", "", "\\", "/").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func normalizedToolArguments(raw json.RawMessage) string {
	var value any
	if err := json.Unmarshal(normalizeToolArgs(raw), &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(string(raw))
	}
	return string(encoded)
}

func (g *toolLoopCycleGuard) observe(records []toolLoopRoundRecord) (int, bool) {
	fingerprint := toolLoopRoundFingerprint(records)
	if fingerprint == "" {
		return 0, false
	}
	g.history = append(g.history, fingerprint)
	maxHistory := toolLoopCycleRepetitions * toolLoopMaxCyclePeriod
	if len(g.history) > maxHistory {
		g.history = append([]string(nil), g.history[len(g.history)-maxHistory:]...)
	}
	for period := 1; period <= toolLoopMaxCyclePeriod; period++ {
		needed := period * toolLoopCycleRepetitions
		if len(g.history) < needed {
			continue
		}
		window := g.history[len(g.history)-needed:]
		repeated := true
		for index := period; index < len(window); index++ {
			if window[index] != window[index%period] {
				repeated = false
				break
			}
		}
		if repeated {
			return period, true
		}
	}
	return 0, false
}

func toolLoopRoundFingerprint(records []toolLoopRoundRecord) string {
	if len(records) == 0 {
		return ""
	}
	stable := make([]stableToolLoopRecord, 0, len(records))
	for _, record := range records {
		parts := append([]tools.ResultPart(nil), record.Result.Parts...)
		for index := range parts {
			parts[index].ToolCallID = ""
			parts[index].StartedAt = ""
			parts[index].CompletedAt = ""
			parts[index].DurationMs = 0
			parts[index].RequestID = ""
			parts[index].TaskID = ""
			parts[index].SecretID = ""
			parts[index].Activities = append([]tools.SubagentActivity(nil), parts[index].Activities...)
			for activityIndex := range parts[index].Activities {
				parts[index].Activities[activityIndex].ID = ""
				parts[index].Activities[activityIndex].StartedAt = ""
				parts[index].Activities[activityIndex].CompletedAt = ""
				parts[index].Activities[activityIndex].DurationMs = 0
			}
		}
		changes := make([]stableToolLoopChange, 0, len(record.Result.Changes))
		for _, change := range record.Result.Changes {
			changes = append(changes, stableToolLoopChange{
				Path:            change.Path,
				BeforeHash:      toolLoopContentHash(change.Before),
				AfterHash:       toolLoopContentHash(change.After),
				Existed:         change.Existed,
				Deleted:         change.Deleted,
				LineEnding:      change.LineEnding,
				Encoding:        change.Encoding,
				HadBOM:          change.HadBOM,
				AfterLineEnding: change.AfterLineEnding,
				AfterEncoding:   change.AfterEncoding,
				AfterHadBOM:     change.AfterHadBOM,
			})
		}
		attachments := make([]stableToolLoopAttachment, 0, len(record.Result.Attachments))
		for _, attachment := range record.Result.Attachments {
			attachments = append(attachments, stableToolLoopAttachment{
				Name:     attachment.Name,
				MIMEType: attachment.MIMEType,
				DataHash: toolLoopContentHash(attachment.Data),
			})
		}
		stable = append(stable, stableToolLoopRecord{
			Name:        record.Name,
			Arguments:   normalizedToolArguments(record.Arguments),
			Summary:     strings.TrimSpace(record.Result.Summary),
			IsError:     record.Result.IsError,
			Parts:       parts,
			Changes:     changes,
			Attachments: attachments,
		})
	}
	encoded, err := json.Marshal(stable)
	if err != nil {
		return ""
	}
	return toolLoopContentHash(string(encoded))
}

func toolLoopContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum[:])
}

func toolLoopCycleFeedback(period int) string {
	if period <= 1 {
		return "安全熔断：相同的工具调用和结果已经连续重复，任务没有产生新进展。请停止调用工具，基于已有结果说明当前状态、已完成内容和建议的下一步。"
	}
	return fmt.Sprintf("安全熔断：检测到重复的 %d 步工具调用周期，结果没有产生新进展。请停止调用工具，基于已有结果说明当前状态、已完成内容和建议的下一步。", period)
}

func appendToolLoopFeedback(messages []protocol.Message, feedback string) {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role != "tool" {
			continue
		}
		messages[index].Content = strings.TrimSpace(messages[index].Content + "\n\n" + feedback)
		return
	}
}

func sshToolCallSignature(raw json.RawMessage) string {
	var args sshToolArguments
	if err := json.Unmarshal(normalizeToolArgs(raw), &args); err != nil {
		return ""
	}
	action := strings.ToLower(strings.TrimSpace(args.Action))
	credentialID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(args.CredentialID), scopedCredentialScheme))
	switch action {
	case "test":
		return action + "\x00" + credentialID
	case "run", "capture_secret":
		command := strings.ToLower(strings.Join(strings.Fields(args.Command), " "))
		if command == "" {
			return ""
		}
		return action + "\x00" + credentialID + "\x00" + command
	default:
		return ""
	}
}

func searchSourcesFromParts(parts []tools.ResultPart) []tools.SearchSource {
	for _, part := range parts {
		if part.Kind == tools.PartWebSearch && len(part.Sources) > 0 {
			return append([]tools.SearchSource(nil), part.Sources...)
		}
	}
	return nil
}

func browserEngineUnavailable(summary string) bool {
	return strings.Contains(summary, "内置浏览器启动失败") || strings.Contains(summary, "启动浏览器引擎失败")
}

func browserUseExplicitlyRequested(messages []protocol.Message) bool {
	request := ""
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			request = strings.ToLower(strings.TrimSpace(messages[index].Content))
			break
		}
	}
	if request == "" {
		return false
	}
	for _, phrase := range []string{
		"不要打开", "不用打开", "无需打开", "别打开", "不要使用浏览器", "不用浏览器",
		"do not open", "don't open", "dont open", "without opening", "do not use the browser",
	} {
		if strings.Contains(request, phrase) {
			return false
		}
	}
	for _, phrase := range []string{
		"打开网页", "打开网站", "打开链接", "访问网页", "访问网站", "使用浏览器", "用浏览器", "内置浏览器",
		"浏览这个网页", "浏览这个网站", "读取网页", "分析网页", "操作网页", "点击网页", "登录网站", "登录网页",
		"官网下载", "从官网下载", "下载官网", "下载网站", "下载页", "下载页面",
		"open the webpage", "open the page", "open the website", "open this link", "use the browser", "in-app browser",
		"visit the website", "browse the website", "read the webpage", "click on the page", "log in to the website",
		"download from the official website", "download from the vendor website", "download page",
	} {
		if strings.Contains(request, phrase) {
			return true
		}
	}
	hasAddress := strings.Contains(request, "http://") || strings.Contains(request, "https://") || strings.Contains(request, "www.")
	if !hasAddress {
		return false
	}
	for _, action := range []string{"打开", "访问", "浏览", "读取", "分析", "登录", "open", "visit", "browse", "read", "inspect", "log in"} {
		if strings.Contains(request, action) {
			return true
		}
	}
	return false
}

func searchQueriesSimilar(left, right string) bool {
	leftSite, leftTerms, leftCompact := searchQuerySignature(left)
	rightSite, rightTerms, rightCompact := searchQuerySignature(right)
	if leftSite != rightSite && (leftSite != "" || rightSite != "") {
		return false
	}
	if leftCompact != "" && leftCompact == rightCompact {
		return true
	}
	if len(leftTerms) == 0 || len(rightTerms) == 0 {
		return false
	}
	intersection := 0
	for term := range leftTerms {
		if rightTerms[term] {
			intersection++
		}
	}
	minimum := len(leftTerms)
	if len(rightTerms) < minimum {
		minimum = len(rightTerms)
	}
	return intersection >= 2 && float64(intersection)/float64(minimum) >= 0.72
}

func searchQuerySignature(query string) (string, map[string]bool, string) {
	query = strings.ToLower(strings.TrimSpace(query))
	fields := strings.Fields(query)
	kept := make([]string, 0, len(fields))
	site := ""
	for _, field := range fields {
		if strings.HasPrefix(field, "site:") {
			site = strings.Trim(strings.TrimPrefix(field, "site:"), "\"'()[]{}，。；;")
			continue
		}
		kept = append(kept, field)
	}
	query = strings.Join(kept, " ")
	for _, phrase := range []string{"今天", "今日", "当前", "最新", "现在", "实时", "today", "latest", "current", "now"} {
		query = strings.ReplaceAll(query, phrase, " ")
	}
	terms := map[string]bool{}
	segments := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var compact strings.Builder
	for _, segment := range segments {
		runes := []rune(segment)
		containsDigit := false
		containsHan := false
		for _, r := range runes {
			containsDigit = containsDigit || unicode.IsDigit(r)
			containsHan = containsHan || unicode.Is(unicode.Han, r)
			if !unicode.IsDigit(r) {
				compact.WriteRune(r)
			}
		}
		if containsDigit {
			continue
		}
		if containsHan {
			if len(runes) == 1 {
				terms[string(runes)] = true
				continue
			}
			for index := 0; index < len(runes)-1; index++ {
				terms[string(runes[index:index+2])] = true
			}
			continue
		}
		term := string(runes)
		if len(term) > 4 && strings.HasSuffix(term, "s") {
			term = strings.TrimSuffix(term, "s")
		}
		if term != "" {
			terms[term] = true
		}
	}
	return site, terms, compact.String()
}

func (s *Service) runToolLoopWithCompletion(
	ctx context.Context,
	reg *tools.Registry,
	req protocol.ChatRequest,
	complete completionFunc,
	sink ChatEventSink,
) (toolLoopOutcome, error) {
	return s.runToolLoopWithCompletionState(ctx, reg, req, complete, sink, deploymentSSHPreflight{})
}

func (s *Service) runToolLoopWithCompletionState(
	ctx context.Context,
	reg *tools.Registry,
	req protocol.ChatRequest,
	complete completionFunc,
	sink ChatEventSink,
	preflight deploymentSSHPreflight,
) (toolLoopOutcome, error) {
	outcome := toolLoopOutcome{Parts: append([]tools.ResultPart{}, preflight.Result.Parts...)}
	messages := append([]protocol.Message{}, req.Messages...)
	toolDefs := toolDefinitions(reg)
	toolsDisabled := false // 自定义模型不支持 function-calling 时降级为纯对话
	emptyCompletionStage := 0
	synthesisOnly := false
	visualVerificationRequested := false
	guard := toolLoopGuard{
		browserExplicitlyRequested: browserUseExplicitlyRequested(req.Messages),
		remoteLookup:               remoteLookupTask(req.Messages),
		completedSSHCalls:          make(map[string]bool),
		failureStrategy:            s.failureStrategySnapshot(),
		resolvedFailures:           make(map[string]bool),
		blockedFailures:            make(map[string]int),
		turnIndex:                  s.sessionState.TurnCount + 1,
	}
	defer func() {
		s.mergeFailureStrategyState(guard.failureStrategy, guard.resolvedFailures)
	}()
	if guard.remoteLookup {
		guard.maxPlanUpdates = maxLookupPlanUpdates
	}
	if preflight.Succeeded && preflight.CredentialID != "" {
		guard.completedSSHCalls["test\x00"+preflight.CredentialID] = true
	}

	executed := 0
	for {
		if err := ctx.Err(); err != nil {
			setOutcomeProgressStatus(&outcome, "cancelled")
			emitOutcomeProgress(sink, outcome.Parts)
			return outcome, err
		}
		stepReq := req
		stepReq.Messages = fitToolLoopMessages(messages, req.TargetInputTokens)
		if !toolsDisabled {
			stepReq.Tools = toolDefs
		} else {
			stepReq.Tools = nil
			stepReq.ToolChoice = "none"
		}

		completion, err := complete(ctx, stepReq)
		if err != nil && executed > 0 && isRetryablePostToolCompletionError(ctx, err) {
			for retry := 1; retry <= postToolCompletionRetries; retry++ {
				emitChatEvent(sink, ChatStreamEvent{
					Type:    "status",
					Message: fmt.Sprintf("模型连接中断，正在继续整理工具结果（%d/%d）", retry, postToolCompletionRetries),
					Status:  "retrying",
				})
				if waitErr := waitForPostToolRetry(ctx, retry); waitErr != nil {
					err = waitErr
					break
				}
				completion, err = complete(ctx, stepReq)
				if err == nil || !isRetryablePostToolCompletionError(ctx, err) {
					break
				}
			}
		}
		outcome.Parts = mergeOutcomeParts(outcome.Parts, providerNoticeParts(completion.Notices))
		if completion.Usage != nil {
			usage := *completion.Usage
			outcome.Usage = &usage
			outcome.UsageSamples = append(outcome.UsageSamples, usage)
		}
		if strings.TrimSpace(completion.Reasoning) != "" {
			outcome.Reasoning = completion.Reasoning
		}
		if strings.TrimSpace(completion.Content) != "" {
			outcome.Content = completion.Content
		}
		if err != nil {
			// Only retry without tools when the provider explicitly rejects the
			// function-calling fields. Transport, timeout, policy, authentication,
			// and unrelated request errors must retain their real failure semantics.
			if !toolsDisabled && executed == 0 && len(toolDefs) > 0 && isToolCompatibilityError(err) {
				emitChatEvent(sink, ChatStreamEvent{
					Type:    "status",
					Message: "当前模型端点明确不支持工具调用，已降级为普通对话。",
					Status:  "retrying",
				})
				toolsDisabled = true
				continue
			}
			setOutcomeProgressStatus(&outcome, "failed")
			emitOutcomeProgress(sink, outcome.Parts)
			return outcome, err
		}

		// 无工具调用 → 最终答案。
		if toolsDisabled && len(completion.ToolCalls) > 0 {
			if synthesisOnly {
				if content := strings.TrimSpace(completion.Content); content != "" {
					outcome.Content = s.appendVisualVerificationDisclosure(ensureWebSearchSourcesListed(content, outcome.Parts))
					outcome.Parts = append(outcome.Parts, tools.ResultPart{Kind: tools.PartText, Text: outcome.Content})
					setOutcomeProgressStatus(&outcome, "completed")
					emitOutcomeProgress(sink, outcome.Parts)
					return outcome, nil
				}
				setOutcomeProgressStatus(&outcome, "failed")
				emitOutcomeProgress(sink, outcome.Parts)
				return outcome, errEmptyToolResultSynthesis
			}
			summary := "上游模型在工具已禁用后仍请求调用工具，MHcode 已停止本轮工具循环并保留全部已完成结果。"
			if _, canCollectSubagents := reg.Get("await_subagents"); canCollectSubagents && s.hasUncollectedSubagents() {
				call := newAwaitSubagentsToolCall()
				result, _ := s.executeToolCall(ctx, reg, call)
				outcome.Parts = mergeOutcomeParts(outcome.Parts, result.Parts)
				if strings.TrimSpace(result.Summary) != "" {
					summary += "\n\n" + result.Summary
				}
				if err := ctx.Err(); err != nil {
					setOutcomeProgressStatus(&outcome, "cancelled")
					emitOutcomeProgress(sink, outcome.Parts)
					return outcome, err
				}
			}
			emitChatEvent(sink, ChatStreamEvent{Type: "status", Message: summary, Status: "completed"})
			outcome.Content = strings.TrimSpace(completion.Content)
			if outcome.Content == "" {
				outcome.Content = summary
			} else {
				outcome.Content += "\n\n" + summary
			}
			outcome.Content = s.appendVisualVerificationDisclosure(outcome.Content)
			outcome.Parts = append(outcome.Parts, tools.ResultPart{Kind: tools.PartText, Text: outcome.Content})
			setOutcomeProgressStatus(&outcome, "completed")
			emitOutcomeProgress(sink, outcome.Parts)
			return outcome, nil
		}

		if len(completion.ToolCalls) == 0 {
			_, canCollectSubagents := reg.Get("await_subagents")
			if canCollectSubagents && s.hasUncollectedSubagents() {
				call := newAwaitSubagentsToolCall()
				messages = append(messages, protocol.Message{
					Role:             "assistant",
					Content:          completion.Content,
					ReasoningContent: completion.Reasoning,
					ToolCalls:        []protocol.ToolCall{call},
				})
				executed++
				emitChatEvent(sink, ChatStreamEvent{
					Type:       "tool",
					Message:    "正在等待后台子代理完成",
					ToolName:   call.Function.Name,
					ToolCallID: call.ID,
					Status:     "waiting",
				})
				result, toolMessage := s.executeToolCall(ctx, reg, call)
				toolStatus := "completed"
				if result.IsError {
					toolStatus = "error"
				}
				emitChatEvent(sink, ChatStreamEvent{
					Type:       "tool",
					Message:    result.Summary,
					ToolName:   call.Function.Name,
					ToolCallID: call.ID,
					Status:     toolStatus,
					Parts:      result.Parts,
				})
				outcome.Parts = mergeOutcomeParts(outcome.Parts, result.Parts)
				messages = append(messages, toolMessage)
				if err := ctx.Err(); err != nil {
					setOutcomeProgressStatus(&outcome, "cancelled")
					emitOutcomeProgress(sink, outcome.Parts)
					return outcome, err
				}
				continue
			}
			pendingVisual := s.pendingVisualArtifactsForCurrentTurn()
			if len(pendingVisual) > 0 && !visualVerificationRequested && !toolsDisabled {
				visualVerificationRequested = true
				messages = append(messages, visualVerificationRecoveryMessage(pendingVisual))
				emitChatEvent(sink, ChatStreamEvent{
					Type:    "status",
					Message: "发现尚未完成视觉验收的产物，正在请求渲染与检查",
					Status:  "running",
				})
				continue
			}
			content := strings.TrimSpace(completion.Content)
			if content == "" {
				hasToolEvidence := executed > 0 || hasMeaningfulResultParts(outcome.Parts)
				if hasToolEvidence && emptyCompletionStage == 0 && !toolsDisabled {
					emptyCompletionStage = 1
					messages = append(messages, toolResultRecoveryMessage(false))
					emitChatEvent(sink, ChatStreamEvent{
						Type:    "status",
						Message: "模型尚未形成结论，正在继续核验工具结果",
						Status:  "retrying",
					})
					continue
				}
				if hasToolEvidence && emptyCompletionStage < 2 {
					emptyCompletionStage = 2
					synthesisOnly = true
					toolsDisabled = true
					messages = append(messages, toolResultRecoveryMessage(true))
					emitChatEvent(sink, ChatStreamEvent{
						Type:    "status",
						Message: "调查结果已保留，正在要求模型生成最终结论",
						Status:  "retrying",
					})
					continue
				}
				setOutcomeProgressStatus(&outcome, "failed")
				emitOutcomeProgress(sink, outcome.Parts)
				return outcome, errEmptyToolResultSynthesis
			}
			outcome.Content = s.appendVisualVerificationDisclosure(ensureWebSearchSourcesListed(content, outcome.Parts))
			if strings.TrimSpace(outcome.Content) != "" {
				outcome.Parts = append(outcome.Parts, tools.ResultPart{
					Kind: tools.PartText,
					Text: outcome.Content,
				})
			}
			setOutcomeProgressStatus(&outcome, "completed")
			emitOutcomeProgress(sink, outcome.Parts)
			return outcome, nil
		}

		// 记录 assistant 的工具调用轮（回喂需要）。
		messages = append(messages, protocol.Message{
			Role:             "assistant",
			Content:          completion.Content,
			ReasoningContent: completion.Reasoning,
			ToolCalls:        completion.ToolCalls,
			Continuation:     completion.Continuation,
		})
		if strings.TrimSpace(completion.Content) != "" {
			outcome.Parts = append(outcome.Parts, tools.ResultPart{Kind: tools.PartText, Text: completion.Content})
		}

		// 逐个执行工具调用。
		roundRecords := make([]toolLoopRoundRecord, 0, len(completion.ToolCalls))
		for _, call := range completion.ToolCalls {
			if err := ctx.Err(); err != nil {
				setOutcomeProgressStatus(&outcome, "cancelled")
				emitOutcomeProgress(sink, outcome.Parts)
				return outcome, err
			}
			isProgressUpdate := call.Function.Name == "update_plan"
			toolInput := toolInputForDisplay(call.Function.Name, call.Function.Arguments)
			result, toolMsg, guarded, hidden := guard.before(call)
			if !isProgressUpdate && !hidden {
				executed++
			}
			if !isProgressUpdate && !hidden {
				emitChatEvent(sink, ChatStreamEvent{
					Type:       "tool",
					Message:    fmt.Sprintf("正在运行 %s", call.Function.Name),
					ToolName:   call.Function.Name,
					ToolCallID: call.ID,
					ToolInput:  toolInput,
					Status:     "running",
				})
			}

			if !guarded {
				toolCtx := tools.WithProgressSink(ctx, func(part tools.ResultPart) {
					if part.Kind == tools.PartSubagent {
						message := strings.TrimSpace(part.CurrentAction)
						if message == "" {
							message = strings.TrimSpace(part.Summary)
						}
						if message == "" {
							message = "子代理正在工作"
						}
						emitChatEvent(sink, ChatStreamEvent{
							Type:    "subagent",
							Message: message,
							Status:  part.Status,
							Parts:   []tools.ResultPart{part},
						})
						return
					}
					if part.Name == "" {
						part.Name = call.Function.Name
					}
					if part.Input == "" {
						part.Input = toolInput
					}
					if part.ToolCallID == "" {
						part.ToolCallID = call.ID
					}
					if part.Status == "" {
						part.Status = "running"
					}
					eventStatus := strings.TrimSpace(part.Status)
					if eventStatus == "" {
						eventStatus = "running"
					}
					emitChatEvent(sink, ChatStreamEvent{
						Type:       "tool",
						Message:    toolProgressMessage(call.Function.Name, eventStatus, part.Output),
						ToolName:   call.Function.Name,
						ToolCallID: call.ID,
						ToolInput:  toolInput,
						Status:     eventStatus,
						Parts:      []tools.ResultPart{part},
					})
				})
				result, toolMsg = s.executeToolCall(toolCtx, reg, call)
				if err := ctx.Err(); err != nil {
					if !hidden {
						outcome.Parts = mergeOutcomeParts(outcome.Parts, result.Parts)
						outcome.Changes = append(outcome.Changes, result.Changes...)
						updateOutcomeProgressStats(&outcome)
					}
					setOutcomeProgressStatus(&outcome, "cancelled")
					emitOutcomeProgress(sink, outcome.Parts)
					return outcome, err
				}
				guard.after(call, result, &toolMsg)
			}
			toolStatus := "completed"
			if result.IsError {
				toolStatus = "error"
			}
			if (!isProgressUpdate || result.IsError) && !hidden {
				emitChatEvent(sink, ChatStreamEvent{
					Type:       "tool",
					Message:    result.Summary,
					ToolName:   call.Function.Name,
					ToolCallID: call.ID,
					ToolInput:  toolInput,
					Status:     toolStatus,
					Parts:      result.Parts,
				})
			}
			if !hidden {
				outcome.Parts = mergeOutcomeParts(outcome.Parts, result.Parts)
				outcome.Changes = append(outcome.Changes, result.Changes...)
				updateOutcomeProgressStats(&outcome)
				emitOutcomeProgress(sink, outcome.Parts)
			}
			// 把工具结果回喂给模型。
			messages = append(messages, toolMsg)
			roundRecords = append(roundRecords, toolLoopRoundRecord{
				Name:      call.Function.Name,
				Arguments: append(json.RawMessage(nil), call.Function.Arguments...),
				Result:    result,
			})
			if guard.forceFinalResponse {
				toolsDisabled = true
				break
			}
		}
		if period, stalled := guard.cycles.observe(roundRecords); stalled {
			feedback := toolLoopCycleFeedback(period)
			appendToolLoopFeedback(messages, feedback)
			toolsDisabled = true
			emitChatEvent(sink, ChatStreamEvent{
				Type:    "status",
				Message: feedback,
				Status:  "running",
			})
		}
	}
}

func toolProgressMessage(name, status, output string) string {
	if message := strings.TrimSpace(output); message != "" && (status == "waiting" || status == "retrying") {
		return message
	}
	switch status {
	case "waiting":
		return fmt.Sprintf("正在等待 %s", name)
	case "retrying":
		return fmt.Sprintf("正在重试 %s", name)
	default:
		return fmt.Sprintf("正在运行 %s", name)
	}
}

func remoteLookupTask(messages []protocol.Message) bool {
	request := latestUserRequest(messages)
	if request == "" || !strings.Contains(request, scopedCredentialScheme) {
		return false
	}
	lower := strings.ToLower(request)
	for _, marker := range []string{
		"部署", "安装", "升级", "迁移", "修复", "排障", "调试", "配置服务", "发布",
		"deploy", "install", "upgrade", "migrate", "repair", "debug", "troubleshoot", "configure", "release",
	} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	for _, marker := range []string{
		"读取", "查找", "找出", "获取", "查看", "确认", "账号", "账户", "密码", "口令", "密钥", "令牌",
		"read", "find", "locate", "get", "inspect", "account", "credential", "password", "secret", "token",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func latestUserRequest(messages []protocol.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			return strings.TrimSpace(messages[index].Content)
		}
	}
	return ""
}

func toolResultRecoveryMessage(final bool) protocol.Message {
	instruction := strings.Join([]string{
		"[MHcode private tool-result recovery]",
		"The previous completion returned no user-facing answer after tool execution. The task is not complete.",
		"Continue investigating with the available tools. Treat web_search snippets only as discovery: for named software, open and read the official website, official documentation, or real source repository before drawing conclusions.",
		"For questions about the user's computer or current configuration, inspect the authorized local state or explicitly request the missing permission. Never substitute a raw search-result list for the requested diagnosis.",
		"[/MHcode private tool-result recovery]",
	}, "\n")
	if final {
		instruction = strings.Join([]string{
			"[MHcode private tool-result recovery]",
			"Produce the final user-facing answer now from the verified tool evidence already in the conversation.",
			"If the evidence is insufficient, say exactly what remains unverified and which permission or user action is needed. Include only relevant source links actually used. Never present raw search snippets as a completed answer.",
			"[/MHcode private tool-result recovery]",
		}, "\n")
	}
	return protocol.Message{Role: "user", Content: instruction, InternalKind: toolResultRecoveryKind}
}

func isToolCompatibilityError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if info, ok := protocol.ProviderErrorDetails(err); ok {
		// Authentication, throttling, policy, and transient server failures are
		// never evidence that the endpoint lacks function calling.
		if info.HTTPStatus == 401 || info.HTTPStatus == 403 || info.HTTPStatus == 408 ||
			info.HTTPStatus == 409 || info.HTTPStatus == 429 || info.HTTPStatus >= 500 {
			return false
		}
		code := strings.ToLower(strings.TrimSpace(info.Code + " " + info.Type))
		mentionsCapability := strings.Contains(code, "tool") || strings.Contains(code, "function_call") || strings.Contains(code, "function-call")
		rejectsCapability := strings.Contains(code, "unsupported") || strings.Contains(code, "not_supported") ||
			strings.Contains(code, "unknown_parameter") || strings.Contains(code, "unrecognized_parameter")
		if mentionsCapability && rejectsCapability {
			return true
		}
		return toolCompatibilityMessage(info.Message)
	}
	return toolCompatibilityMessage(err.Error())
}

func toolCompatibilityMessage(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	if message == "" {
		return false
	}
	for _, marker := range []string{
		"does not support tool completions",
		"does not support tool calls",
		"doesn't support tool calls",
		"tool calls are not supported",
		"tool calling is not supported",
		"tool use is not supported",
		"function calling is not supported",
		"function calls are not supported",
		"unsupported parameter: tools",
		"unsupported parameter 'tools'",
		"unsupported parameter \"tools\"",
		"unknown parameter: tools",
		"unknown parameter: 'tools'",
		"unknown parameter: \"tools\"",
		"unrecognized request argument supplied: tools",
		"unknown field: tools",
		"unknown field 'tools'",
		"unknown field \"tools\"",
		"'tools' is not allowed",
		"\"tools\" is not allowed",
		"unsupported tool_choice",
		"unknown parameter: tool_choice",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func isRetryablePostToolCompletionError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"eof", "timeout", "timed out", "connection reset", "connection refused",
		"server closed", "temporarily unavailable", "bad gateway", "service unavailable",
		"gateway timeout", "deadline", "超时", "中断", "http 502", "http 503", "http 504",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func waitForPostToolRetry(ctx context.Context, retry int) error {
	timer := time.NewTimer(time.Duration(retry) * 350 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func mergeOutcomeParts(existing, incoming []tools.ResultPart) []tools.ResultPart {
	for _, part := range incoming {
		if part.Kind == tools.PartToolCall && part.Name == "web_search" && part.Status != "error" {
			continue
		}
		if part.Kind == tools.PartWebSearch {
			replaced := false
			for index := range existing {
				if existing[index].Kind != tools.PartWebSearch {
					continue
				}
				existing[index] = mergeWebSearchParts(existing[index], part)
				replaced = true
				break
			}
			if !replaced {
				existing = append(existing, part)
			}
			continue
		}
		if part.Kind == tools.PartProviderNotice {
			identity := providerResultPartIdentity(part)
			duplicate := false
			for _, current := range existing {
				if current.Kind == tools.PartProviderNotice && providerResultPartIdentity(current) == identity {
					duplicate = true
					break
				}
			}
			if !duplicate {
				existing = append(existing, part)
			}
			continue
		}
		if part.Kind == tools.PartSubagent && strings.TrimSpace(part.TaskID) != "" {
			replaced := false
			for index := range existing {
				if existing[index].Kind != tools.PartSubagent || existing[index].TaskID != part.TaskID {
					continue
				}
				existing[index] = mergeSubagentParts(existing[index], part)
				replaced = true
				break
			}
			if !replaced {
				existing = append(existing, part)
			}
			continue
		}
		if part.Kind != tools.PartProgress {
			existing = append(existing, part)
			continue
		}
		replaced := false
		for index := range existing {
			if existing[index].Kind == tools.PartProgress {
				existing[index] = part
				replaced = true
				break
			}
		}
		if !replaced {
			existing = append(existing, part)
		}
	}
	return existing
}

func mergeSubagentParts(existing, incoming tools.ResultPart) tools.ResultPart {
	if subagentStatusIsTerminal(existing.Status) && !subagentStatusIsTerminal(incoming.Status) {
		return existing
	}
	return incoming
}

func subagentStatusIsTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "error", "cancelled":
		return true
	default:
		return false
	}
}

func providerResultPartIdentity(part tools.ResultPart) string {
	return strings.Join([]string{
		part.NoticeKind,
		part.RequestedModel,
		part.EffectiveModel,
		part.RetryModel,
		strings.Join(part.UseCases, ","),
		strings.Join(part.Reasons, ","),
		strings.Join(part.Verifications, ","),
		strings.Join(part.MetadataKeys, ","),
		part.ErrorCode,
	}, "\x00")
}

func mergeWebSearchParts(existing, incoming tools.ResultPart) tools.ResultPart {
	if existing.Query == "" {
		existing.Query = incoming.Query
	} else if incoming.Query != "" && existing.Query != incoming.Query && !strings.Contains(existing.Query, "含补充搜索") {
		existing.Query += "（含补充搜索）"
	}
	byURL := make(map[string]int, len(existing.Sources)+len(incoming.Sources))
	for index, source := range existing.Sources {
		byURL[strings.TrimSpace(source.URL)] = index
	}
	for _, source := range incoming.Sources {
		key := strings.TrimSpace(source.URL)
		if key == "" {
			continue
		}
		if index, ok := byURL[key]; ok {
			if existing.Sources[index].Title == "" {
				existing.Sources[index].Title = source.Title
			}
			if existing.Sources[index].Snippet == "" {
				existing.Sources[index].Snippet = source.Snippet
			}
			continue
		}
		if len(existing.Sources) >= 16 {
			break
		}
		byURL[key] = len(existing.Sources)
		existing.Sources = append(existing.Sources, source)
	}
	return existing
}

func updateOutcomeProgressStats(outcome *toolLoopOutcome) {
	if outcome == nil {
		return
	}
	files, additions, deletions := tools.SummarizeFileChanges(outcome.Changes)
	for index := range outcome.Parts {
		if outcome.Parts[index].Kind != tools.PartProgress {
			continue
		}
		outcome.Parts[index].ChangedFiles = files
		outcome.Parts[index].Additions = additions
		outcome.Parts[index].Deletions = deletions
		return
	}
}

func setOutcomeProgressStatus(outcome *toolLoopOutcome, status string) {
	if outcome == nil {
		return
	}
	updateOutcomeProgressStats(outcome)
	for index := range outcome.Parts {
		if outcome.Parts[index].Kind == tools.PartProgress {
			if status == "completed" {
				for _, step := range outcome.Parts[index].Steps {
					if step.Status != "completed" {
						status = "running"
						break
					}
				}
			}
			outcome.Parts[index].TaskStatus = status
			return
		}
	}
}

func emitOutcomeProgress(sink ChatEventSink, parts []tools.ResultPart) {
	for index := range parts {
		if parts[index].Kind != tools.PartProgress {
			continue
		}
		progress := parts[index]
		emitChatEvent(sink, ChatStreamEvent{Type: "progress", Progress: &progress})
		return
	}
}

// executeToolCall 执行单个工具调用，返回结构化结果与回喂给模型的 tool 消息。
func (s *Service) executeToolCall(ctx context.Context, reg *tools.Registry, call protocol.ToolCall) (result tools.Result, message protocol.Message) {
	name := call.Function.Name
	normalizedArgs := normalizeToolArgs(call.Function.Arguments)
	startedAt := time.Now()
	defer func() {
		result = ensureToolExecutionMetadata(result, name, call.ID, normalizedArgs, startedAt, time.Now())
	}()
	tool, ok := reg.Get(name)
	if !ok {
		summary := fmt.Sprintf("未知工具: %s", name)
		return tools.Result{
				Summary: summary,
				IsError: true,
				Parts:   []tools.ResultPart{{Kind: tools.PartToolCall, Name: name, Status: "error", Output: summary}},
			}, protocol.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       name,
				Content:    summary,
			}
	}
	timeout := time.Duration(s.runtimeSettings.ToolTimeoutSeconds) * time.Second
	result, err := s.runToolWithWatchdog(ctx, tool, name, normalizedArgs, timeout)
	if err != nil {
		summary := fmt.Sprintf("工具 %s 执行出错: %v", name, err)
		return tools.Result{
				Summary: summary,
				IsError: true,
				Parts:   []tools.ResultPart{{Kind: tools.PartToolCall, Name: name, Status: "error", Output: summary}},
			}, protocol.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       name,
				Content:    summary,
			}
	}
	result = applyToolResultPolicy(result, s.runtimeSettings.ToolResultPolicy)

	result = ensureToolErrorPart(result, name, normalizedArgs)
	if !result.IsError {
		for _, change := range result.Changes {
			if snapshotErr := s.recordFileSnapshot(change); snapshotErr != nil {
				rollbackErr := tools.RestoreFile(s.sandboxPolicy(), change.Path, change.Before, change.Existed, change.LineEnding, change.Encoding, change.HadBOM)
				summary := fmt.Sprintf("工具 %s 已写入文件，但记录回退快照失败: %v", name, snapshotErr)
				if rollbackErr != nil {
					summary += fmt.Sprintf("；自动恢复也失败: %v", rollbackErr)
				} else {
					summary += "；已自动恢复写入前状态"
				}
				result = tools.Result{Summary: summary, IsError: true}
				result = ensureToolErrorPart(result, name, normalizedArgs)
				break
			}
			s.turnChanges = append(s.turnChanges, change)
		}
	}
	var artifactRecords []ArtifactRecord
	if !result.IsError {
		// Attach the call ID before registration so the durable record can be
		// associated with the exact tool invocation even if the turn is later
		// interrupted before an assistant message is produced.
		result = ensureToolExecutionMetadata(result, name, call.ID, normalizedArgs, startedAt, time.Now())
		var artifactErr error
		artifactRecords, artifactErr = s.recordToolArtifacts(name, call.ID, result)
		if artifactErr != nil {
			result.IsError = true
			result.Summary = strings.TrimSpace(result.Summary + "\n" + artifactErr.Error())
			for index := range result.Parts {
				if result.Parts[index].Kind != tools.PartToolCall {
					continue
				}
				result.Parts[index].Status = "error"
				result.Parts[index].Output = result.Summary
			}
		}
	}
	// Build model feedback only after snapshot recording and any automatic
	// rollback, so the model never receives a stale success message.
	feedback := result.Summary
	if feedback == "" {
		feedback = "（无输出）"
	}
	for _, part := range result.Parts {
		if part.Kind == tools.PartToolCall && part.Output != "" && strings.TrimSpace(part.Output) != strings.TrimSpace(feedback) {
			feedback = feedback + "\n" + part.Output
			break
		}
	}
	if context := formatLocalArtifactContext(artifactReferencesFromRecords(artifactRecords), 4_000); context != "" {
		feedback += "\n\n" + context
	}
	return result, protocol.Message{
		Role:        "tool",
		ToolCallID:  call.ID,
		Name:        name,
		Content:     feedback,
		Attachments: protocolToolAttachments(result.Attachments),
	}
}

func toolNeedsExclusiveWorkspaceAccess(name string, tool tools.Tool) bool {
	if _, ok := tool.(tools.MutatingTool); ok {
		return true
	}
	if strings.HasPrefix(name, "plugin__") {
		if readOnly, ok := tool.(interface{ ReadOnly() bool }); ok {
			return !readOnly.ReadOnly()
		}
		return true
	}
	switch name {
	case "run_command", "git", "git_repository", "terminal":
		return true
	}
	return strings.HasPrefix(name, "mcp__")
}

func ensureToolExecutionMetadata(result tools.Result, name, toolCallID string, rawArgs json.RawMessage, startedAt, completedAt time.Time) tools.Result {
	status := "ok"
	if result.IsError {
		status = "error"
	}
	input := toolInputForDisplay(name, rawArgs)
	durationMs := completedAt.Sub(startedAt).Milliseconds()
	if durationMs < 1 {
		durationMs = 1
	}
	found := false
	for index := range result.Parts {
		part := &result.Parts[index]
		if part.ToolCallID == "" {
			part.ToolCallID = toolCallID
		}
		if part.Kind != tools.PartToolCall {
			continue
		}
		found = true
		if part.Name == "" {
			part.Name = name
		}
		if part.Status == "" {
			part.Status = status
		}
		if part.Input == "" {
			part.Input = input
		}
		if part.StartedAt == "" {
			part.StartedAt = startedAt.Format(time.RFC3339Nano)
		}
		if part.CompletedAt == "" {
			part.CompletedAt = completedAt.Format(time.RFC3339Nano)
		}
		if part.DurationMs <= 0 {
			part.DurationMs = durationMs
		}
	}
	if found {
		return result
	}
	result.Parts = append([]tools.ResultPart{{
		Kind:        tools.PartToolCall,
		Name:        name,
		ToolCallID:  toolCallID,
		Status:      status,
		Input:       input,
		Output:      result.Summary,
		StartedAt:   startedAt.Format(time.RFC3339Nano),
		CompletedAt: completedAt.Format(time.RFC3339Nano),
		DurationMs:  durationMs,
	}}, result.Parts...)
	return result
}

func protocolToolAttachments(attachments []tools.Attachment) []protocol.Attachment {
	converted := make([]protocol.Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		converted = append(converted, protocol.Attachment{Name: attachment.Name, MIMEType: attachment.MIMEType, Data: attachment.Data})
	}
	return converted
}

func applyToolResultPolicy(result tools.Result, policy string) tools.Result {
	summaryLimit, detailLimit := 4_000, 8_000
	switch policy {
	case "balanced":
		summaryLimit, detailLimit = 12_000, 24_000
	case "raw-local":
		summaryLimit, detailLimit = 32_000, 64_000
	}
	result.Summary = clipContextText(result.Summary, summaryLimit)
	for index := range result.Parts {
		part := &result.Parts[index]
		part.Input = clipContextText(part.Input, detailLimit/2)
		part.Output = clipContextText(part.Output, detailLimit)
		part.Stdout = clipContextText(part.Stdout, detailLimit)
		part.Stderr = clipContextText(part.Stderr, detailLimit)
		part.Summary = clipContextText(part.Summary, detailLimit)
		part.CurrentAction = clipContextText(part.CurrentAction, detailLimit/2)
		if part.Kind == tools.PartText {
			part.Text = clipContextText(part.Text, detailLimit)
		}
		for sourceIndex := range part.Sources {
			part.Sources[sourceIndex].Snippet = clipContextText(part.Sources[sourceIndex].Snippet, 800)
		}
	}
	return result
}

func ensureToolErrorPart(result tools.Result, name string, rawArgs json.RawMessage) tools.Result {
	if !result.IsError {
		return result
	}
	for _, part := range result.Parts {
		if part.Kind == tools.PartToolCall {
			return result
		}
	}
	preserved := make([]tools.ResultPart, 0, len(result.Parts)+1)
	preserved = append(preserved, tools.ResultPart{
		Kind:   tools.PartToolCall,
		Name:   name,
		Status: "error",
		Input:  toolInputForDisplay(name, rawArgs),
		Output: result.Summary,
	})
	for _, part := range result.Parts {
		if part.Kind == tools.PartSubagent {
			preserved = append(preserved, part)
		}
	}
	result.Parts = preserved
	return result
}

func toolInputForDisplay(name string, rawArgs json.RawMessage) string {
	var args map[string]any
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return ""
	}
	key := "path"
	switch name {
	case "search", "web_search":
		key = "query"
	case "read_repository", "read_webpage":
		key = "url"
	case "run_command":
		return tools.RunCommandInputForDisplay(rawArgs)
	case "download_file":
		return tools.DownloadFileInputForDisplay(rawArgs)
	case "git_repository":
		return tools.GitRepositoryInputForDisplay(rawArgs)
	case "terminal":
		command, _ := args["command"].(string)
		action, _ := args["action"].(string)
		sessionID, _ := args["session_id"].(string)
		return terminalActionDisplay(action, sessionID, command)
	case "ssh":
		return sshToolInputForDisplay(rawArgs)
	case "copy_file":
		source, _ := args["source"].(string)
		destination, _ := args["destination"].(string)
		return strings.TrimSpace(source + " -> " + destination)
	case "browser":
		if value, _ := args["url"].(string); strings.TrimSpace(value) != "" {
			return tools.SafeDownloadURLForDisplay(value)
		}
		key = "action"
	case "computer":
		action, _ := args["action"].(string)
		windowID, _ := args["window_id"].(string)
		return strings.TrimSpace(action + " " + windowID)
	}
	value, _ := args[key].(string)
	return value
}
