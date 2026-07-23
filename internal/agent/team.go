package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MISSmihu/MHcode/internal/cache"
	"github.com/MISSmihu/MHcode/internal/eventlog"
	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

const (
	TeamRolePlanner     = "planner"
	TeamRoleImplementer = "implementer"
	TeamRoleTester      = "tester"
	TeamRoleReviewer    = "reviewer"
	TeamRoleSynthesizer = "synthesizer"
)

var teamRoleOrder = []string{
	TeamRolePlanner,
	TeamRoleImplementer,
	TeamRoleTester,
	TeamRoleReviewer,
	TeamRoleSynthesizer,
}

type TeamState struct {
	Enabled     bool            `json:"enabled"`
	Active      bool            `json:"active"`
	RunID       string          `json:"runId,omitempty"`
	Status      string          `json:"status"`
	CurrentRole string          `json:"currentRole,omitempty"`
	Roles       []TeamRoleState `json:"roles"`
	StartedAt   string          `json:"startedAt,omitempty"`
	CompletedAt string          `json:"completedAt,omitempty"`
	Summary     string          `json:"summary,omitempty"`
}

type TeamRoleState struct {
	Role       string             `json:"role"`
	Label      string             `json:"label"`
	Enabled    bool               `json:"enabled"`
	Status     string             `json:"status"`
	ProviderID string             `json:"providerId,omitempty"`
	Model      string             `json:"model,omitempty"`
	Attempt    int                `json:"attempt"`
	Verdict    string             `json:"verdict,omitempty"`
	Summary    string             `json:"summary,omitempty"`
	Error      string             `json:"error,omitempty"`
	Usage      cache.UsageMetrics `json:"usage"`
	StartedAt  string             `json:"startedAt,omitempty"`
	FinishedAt string             `json:"finishedAt,omitempty"`
}

type TeamRoleEvent struct {
	RunID      string             `json:"runId"`
	Role       string             `json:"role"`
	Label      string             `json:"label"`
	Status     string             `json:"status"`
	ProviderID string             `json:"providerId,omitempty"`
	Model      string             `json:"model,omitempty"`
	Attempt    int                `json:"attempt"`
	Verdict    string             `json:"verdict,omitempty"`
	Summary    string             `json:"summary,omitempty"`
	Error      string             `json:"error,omitempty"`
	Usage      cache.UsageMetrics `json:"usage"`
}

type teamArtifact struct {
	role    string
	attempt int
	content string
	verdict string
	route   chatRoute
	parts   []tools.ResultPart
	usage   cache.UsageMetrics
}

const teamRunCheckpointVersion = 1

type teamRunCheckpoint struct {
	Version      int                      `json:"version"`
	Status       string                   `json:"status"`
	NextRole     string                   `json:"nextRole"`
	NextAttempt  int                      `json:"nextAttempt"`
	ReviewRound  int                      `json:"reviewRound"`
	PlanStarted  bool                     `json:"planStarted"`
	PlanApproved bool                     `json:"planApproved"`
	Team         TeamState                `json:"team"`
	Artifacts    []teamCheckpointArtifact `json:"artifacts"`
	Parts        []tools.ResultPart       `json:"parts"`
	Usage        cache.UsageMetrics       `json:"usage"`
	UpdatedAt    string                   `json:"updatedAt"`
}

type teamCheckpointArtifact struct {
	Role       string             `json:"role"`
	Attempt    int                `json:"attempt"`
	Content    string             `json:"content"`
	Verdict    string             `json:"verdict,omitempty"`
	ProviderID string             `json:"providerId,omitempty"`
	Model      string             `json:"model,omitempty"`
	Parts      []tools.ResultPart `json:"parts,omitempty"`
	Usage      cache.UsageMetrics `json:"usage"`
}

var errTeamRunPaused = errors.New("AI team run paused")

func isTeamRole(role string) bool {
	for _, candidate := range teamRoleOrder {
		if role == candidate {
			return true
		}
	}
	return false
}

func teamRoleLabel(role string) string {
	switch role {
	case TeamRolePlanner:
		return "规划"
	case TeamRoleImplementer:
		return "实现"
	case TeamRoleTester:
		return "测试"
	case TeamRoleReviewer:
		return "审阅"
	case TeamRoleSynthesizer:
		return "汇总"
	default:
		return role
	}
}

func cloneTeamState(state TeamState) TeamState {
	state.Roles = append([]TeamRoleState(nil), state.Roles...)
	return state
}

func cloneTeamRunCheckpoint(checkpoint *teamRunCheckpoint) *teamRunCheckpoint {
	if checkpoint == nil {
		return nil
	}
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		clone := *checkpoint
		clone.Team = cloneTeamState(checkpoint.Team)
		clone.Artifacts = append([]teamCheckpointArtifact(nil), checkpoint.Artifacts...)
		clone.Parts = append([]tools.ResultPart(nil), checkpoint.Parts...)
		return &clone
	}
	var clone teamRunCheckpoint
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return nil
	}
	return &clone
}

func newTeamRunCheckpoint(settings TeamSettings) *teamRunCheckpoint {
	nextRole := TeamRoleImplementer
	if roleEnabled(settings, TeamRolePlanner) {
		nextRole = TeamRolePlanner
	}
	return &teamRunCheckpoint{
		Version:     teamRunCheckpointVersion,
		Status:      "running",
		NextRole:    nextRole,
		NextAttempt: 1,
		Artifacts:   []teamCheckpointArtifact{},
		Parts:       []tools.ResultPart{},
	}
}

func (s *Service) persistTeamRunCheckpoint(checkpoint *teamRunCheckpoint) error {
	if checkpoint == nil {
		s.teamResume = nil
		return nil
	}
	checkpoint.Version = teamRunCheckpointVersion
	checkpoint.Team = cloneTeamState(s.teamState)
	checkpoint.UpdatedAt = time.Now().Format(time.RFC3339Nano)
	if checkpoint.Status == "running" || checkpoint.Status == "paused" {
		s.teamResume = cloneTeamRunCheckpoint(checkpoint)
	} else {
		s.teamResume = nil
	}
	if s.eventStore == nil {
		return nil
	}
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("encode AI team checkpoint: %w", err)
	}
	hash, err := s.eventStore.WriteSnapshot(string(encoded))
	if err != nil {
		return fmt.Errorf("store AI team checkpoint: %w", err)
	}
	if _, err := s.eventStore.Append(eventlog.EventPayload{TeamCheckpointHash: hash}, eventlog.EventTeamCheckpoint); err != nil {
		return fmt.Errorf("append AI team checkpoint: %w", err)
	}
	return nil
}

func (s *Service) restoreTeamRunCheckpoint(hash string) *teamRunCheckpoint {
	if s.eventStore == nil || strings.TrimSpace(hash) == "" {
		return nil
	}
	encoded, err := s.eventStore.ReadSnapshot(hash)
	if err != nil {
		return nil
	}
	var checkpoint teamRunCheckpoint
	if err := json.Unmarshal([]byte(encoded), &checkpoint); err != nil || checkpoint.Version != teamRunCheckpointVersion {
		return nil
	}
	if checkpoint.NextAttempt < 1 {
		checkpoint.NextAttempt = 1
	}
	return &checkpoint
}

func teamArtifactsFromCheckpoint(checkpoint *teamRunCheckpoint) []teamArtifact {
	if checkpoint == nil {
		return nil
	}
	artifacts := make([]teamArtifact, 0, len(checkpoint.Artifacts))
	for _, item := range checkpoint.Artifacts {
		artifacts = append(artifacts, teamArtifact{
			role: item.Role, attempt: item.Attempt, content: item.Content, verdict: item.Verdict,
			route: chatRoute{Provider: ModelProviderSetting{ID: item.ProviderID}, ModelID: item.Model},
			parts: append([]tools.ResultPart(nil), item.Parts...), usage: item.Usage,
		})
	}
	return artifacts
}

func checkpointTeamArtifacts(artifacts []teamArtifact) []teamCheckpointArtifact {
	checkpoint := make([]teamCheckpointArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		checkpoint = append(checkpoint, teamCheckpointArtifact{
			Role: artifact.role, Attempt: artifact.attempt, Content: artifact.content, Verdict: artifact.verdict,
			ProviderID: artifact.route.Provider.ID, Model: artifact.route.ModelID,
			Parts: append([]tools.ResultPart(nil), artifact.parts...), Usage: artifact.usage,
		})
	}
	return checkpoint
}

func teamArtifactAt(artifacts []teamArtifact, role string, attempt int) (teamArtifact, bool) {
	for index := len(artifacts) - 1; index >= 0; index-- {
		if artifacts[index].role == role && artifacts[index].attempt == attempt {
			return artifacts[index], true
		}
	}
	return teamArtifact{}, false
}

func isTeamResumePrompt(prompt string) bool {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	normalized = strings.Trim(normalized, "。！!？?，,.;； ")
	switch normalized {
	case "继续", "继续吧", "继续执行", "继续开发", "接着", "接着做", "接着开发", "resume", "continue", "go on":
		return true
	default:
		return false
	}
}

type teamResumeTurnKey struct{}

func withTeamResumeTurn(ctx context.Context) context.Context {
	return context.WithValue(ctx, teamResumeTurnKey{}, true)
}

func isTeamResumeTurn(ctx context.Context) bool {
	resume, _ := ctx.Value(teamResumeTurnKey{}).(bool)
	return resume
}

func (s *Service) recordTeamPauseMessage(content, model string, parts []tools.ResultPart, durations ...int64) {
	if s.eventStore == nil {
		return
	}
	durationMs := int64(0)
	if len(durations) > 0 && durations[0] > 0 {
		durationMs = durations[0]
	}
	_, _ = s.eventStore.Append(eventlog.EventPayload{
		Role: "assistant", Content: content, Model: model, DurationMs: durationMs, Parts: toEventParts(parts),
	}, eventlog.EventAssistantMessage)
}

func (s *Service) teamModeEnabled() bool {
	return s.runtimeSettings.Team.Enabled
}

func (s *Service) runTeamTurnLegacy(
	ctx context.Context,
	baseRequest protocol.ChatRequest,
	maxToolCalls int,
	primary chatRoute,
	prefixDiagnostic requestPrefixDiagnostic,
	requestMessages []protocol.Message,
	baseMessageCount int,
	sink ChatEventSink,
) (ChatResult, error) {
	settings := s.runtimeSettings.Team
	s.startTeamRun(settings, sink)
	parts := make([]tools.ResultPart, 0, 24)
	artifacts := make([]teamArtifact, 0, 8)
	aggregate := cache.UsageMetrics{}
	planStarted := false
	cancelResult := func() (ChatResult, error) {
		cancelErr := ctx.Err()
		if cancelErr == nil {
			cancelErr = context.Canceled
		}
		s.finishTeamRun("cancelled", "用户已停止团队任务", sink)
		return ChatResult{State: s.workbenchStateLocked(), Model: primary.ModelID, Parts: parts}, cancelErr
	}

	run := func(role string, attempt int, prior []teamArtifact) (teamArtifact, error) {
		if err := ctx.Err(); err != nil {
			return teamArtifact{role: role}, err
		}
		roleSettings := teamRoleSettings(settings, role)
		artifact, err := s.runTeamRole(ctx, roleSettings, attempt, primary, baseRequest, maxToolCalls, prior, sink)
		if artifact.route.Provider.ID != "" {
			aggregate = addUsageMetrics(aggregate, artifact.usage)
		}
		parts = append(parts, teamOperationalParts(artifact.parts)...)
		parts = append(parts, teamArtifactPart(artifact, attempt, err))
		if err == nil {
			artifacts = append(artifacts, artifact)
		}
		return artifact, err
	}

	var plan teamArtifact
	if roleEnabled(settings, TeamRolePlanner) {
		var err error
		plan, err = run(TeamRolePlanner, 1, nil)
		if err != nil {
			if ctx.Err() != nil {
				return cancelResult()
			}
			emitChatEvent(sink, ChatStreamEvent{Type: "status", Message: "规划角色暂不可用，团队将直接进入实现", Model: primary.ModelID})
		}
	}

	if s.planMode && strings.TrimSpace(plan.content) != "" {
		steps := planStepsFromText(plan.content)
		if len(steps) > 0 {
			if err := s.startPlanState(steps); err != nil {
				s.finishTeamRun("failed", "计划状态保存失败", sink)
				return ChatResult{State: s.workbenchStateLocked(), Model: primary.ModelID}, err
			}
			planStarted = true
		}
		approved, err := s.requestPlanApproval(ctx, plan.content)
		if err != nil {
			if ctx.Err() != nil {
				return cancelResult()
			}
			err = s.failStartedPlan(planStarted, err)
			s.finishTeamRun("failed", err.Error(), sink)
			return ChatResult{State: s.workbenchStateLocked(), Model: primary.ModelID}, err
		}
		if !approved {
			if planStarted {
				if err := s.finishPlanState("cancelled"); err != nil {
					s.finishTeamRun("failed", err.Error(), sink)
					return ChatResult{State: s.workbenchStateLocked(), Model: primary.ModelID}, err
				}
			}
			answer := "团队已完成规划，但你选择暂不执行。\n\n" + plan.content
			parts = append(parts, tools.ResultPart{Kind: tools.PartText, Text: answer})
			s.finishTeamRun("cancelled", "计划未获批准", sink)
			s.metrics = aggregate
			s.sessionMessages = append(s.sessionMessages, protocol.Message{Role: "assistant", Content: answer})
			s.commitRequestPrefix(prefixDiagnostic, requestMessages)
			s.sessionState.MessageCount = len(s.sessionMessages)
			s.sessionState.TurnCount++
			s.recordAssistantAndCheckpoint(answer, plan.route.ModelID, parts, chatTurnDurationMs(ctx))
			return ChatResult{Content: answer, Model: plan.route.ModelID, Usage: aggregate, State: s.workbenchStateLocked(), Parts: parts}, nil
		}
		if len(steps) > 0 {
			steps[0].Status = "in_progress"
			if err := s.updatePlanState(steps); err != nil {
				err = s.failStartedPlan(planStarted, err)
				s.finishTeamRun("failed", err.Error(), sink)
				return ChatResult{State: s.workbenchStateLocked(), Model: primary.ModelID}, err
			}
		}
	}

	implementation, err := run(TeamRoleImplementer, 1, artifacts)
	if err != nil {
		if ctx.Err() != nil {
			return cancelResult()
		}
		err = s.failStartedPlan(planStarted, err)
		s.finishTeamRun("failed", err.Error(), sink)
		s.sessionMessages = s.sessionMessages[:baseMessageCount]
		return ChatResult{State: s.workbenchStateLocked(), Model: primary.ModelID, Parts: parts}, err
	}

	latestChecks := make(map[string]teamArtifact, 2)
	for _, role := range []string{TeamRoleTester, TeamRoleReviewer} {
		if !roleEnabled(settings, role) {
			continue
		}
		artifact, roleErr := run(role, 1, artifacts)
		if ctx.Err() != nil {
			return cancelResult()
		}
		if roleErr == nil {
			latestChecks[role] = artifact
		}
	}

	for round := 1; round <= settings.MaxReviewRounds && teamNeedsRevision(latestChecks); round++ {
		if ctx.Err() != nil {
			return cancelResult()
		}
		feedback := reviewArtifacts(latestChecks)
		revisionContext := append(append([]teamArtifact(nil), artifacts...), teamArtifact{
			role: TeamRoleReviewer, content: "需要修订：\n" + feedback, verdict: "changes_required",
		})
		implementation, err = run(TeamRoleImplementer, round+1, revisionContext)
		if err != nil {
			if ctx.Err() != nil {
				return cancelResult()
			}
			err = s.failStartedPlan(planStarted, err)
			s.finishTeamRun("failed", err.Error(), sink)
			s.sessionMessages = s.sessionMessages[:baseMessageCount]
			return ChatResult{State: s.workbenchStateLocked(), Model: primary.ModelID, Parts: parts}, err
		}
		latestChecks = make(map[string]teamArtifact, 2)
		for _, role := range []string{TeamRoleTester, TeamRoleReviewer} {
			if !roleEnabled(settings, role) {
				continue
			}
			artifact, roleErr := run(role, round+1, artifacts)
			if ctx.Err() != nil {
				return cancelResult()
			}
			if roleErr == nil {
				latestChecks[role] = artifact
			}
		}
	}

	var final teamArtifact
	if roleEnabled(settings, TeamRoleSynthesizer) {
		final, err = run(TeamRoleSynthesizer, 1, artifacts)
	}
	if ctx.Err() != nil {
		return cancelResult()
	}
	answer := strings.TrimSpace(final.content)
	if err != nil || answer == "" {
		answer = fallbackTeamAnswer(implementation, latestChecks)
	}
	answer = sanitizeModelContent(answer)
	parts = append(parts, tools.ResultPart{Kind: tools.PartText, Text: answer})
	s.metrics = aggregate
	s.sessionMessages = append(s.sessionMessages, protocol.Message{Role: "assistant", Content: answer})
	s.commitRequestPrefix(prefixDiagnostic, requestMessages)
	s.sessionState.MessageCount = len(s.sessionMessages)
	s.sessionState.TurnCount++
	model := primary.ModelID
	if final.route.ModelID != "" {
		model = final.route.ModelID
	}
	if planStarted {
		if err := s.finishPlanState("completed"); err != nil {
			s.finishTeamRun("failed", err.Error(), sink)
			return ChatResult{State: s.workbenchStateLocked(), Model: primary.ModelID, Parts: parts}, err
		}
	}
	s.recordAssistantAndCheckpoint(answer, model, parts, chatTurnDurationMs(ctx))
	s.markChatProviderStatus(primary.Provider.ID, "ok", fmt.Sprintf("AI 团队任务完成，共执行 %d 个角色回合。", len(artifacts)))
	s.finishTeamRun("completed", answer, sink)
	return ChatResult{Content: answer, Model: model, Usage: aggregate, State: s.workbenchStateLocked(), Parts: parts}, nil
}

func (s *Service) runTeamRole(
	ctx context.Context,
	settings TeamRoleSetting,
	attempt int,
	primary chatRoute,
	baseRequest protocol.ChatRequest,
	maxToolCalls int,
	artifacts []teamArtifact,
	sink ChatEventSink,
) (teamArtifact, error) {
	role := settings.Role
	route, err := s.resolveTeamRoleRoute(settings, primary)
	if err != nil {
		s.failTeamRole(role, attempt, err, sink)
		return teamArtifact{role: role}, err
	}
	provider, err := s.chatProviderForRoute(route)
	if err != nil {
		s.failTeamRole(role, attempt, err, sink)
		return teamArtifact{role: role, route: route}, err
	}
	s.startTeamRole(role, attempt, route, sink)

	request := teamRoleRequest(baseRequest, role, attempt, route, artifacts)
	roleSink := teamRoleToolSink(sink)
	var outcome toolLoopOutcome
	if role == TeamRoleSynthesizer {
		completion, completionErr := collectProviderStream(ctx, provider, request, sink)
		if completionErr != nil {
			err = completionErr
		} else {
			outcome = toolLoopOutcome{Content: completion.Content, Reasoning: completion.Reasoning, Usage: completion.Usage, Parts: providerNoticeParts(completion.Notices)}
		}
	} else {
		registry := s.buildReadOnlyRegistry()
		budget := teamToolBudget(role, maxToolCalls)
		if role == TeamRoleImplementer {
			registry = s.buildToolRegistry()
		}
		outcome, err = s.runStreamingToolLoop(ctx, provider, registry, request, budget, roleSink)
	}

	artifact := teamArtifact{role: role, attempt: attempt, content: strings.TrimSpace(outcome.Content), route: route, parts: outcome.Parts}
	if outcome.Usage != nil {
		artifact.usage = usageMetricsFor(route.Provider, outcome.Usage)
		s.recordUsageMetrics(artifact.usage, route)
	}
	artifact.verdict = teamVerdict(role, artifact.content)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			s.cancelTeamRole(role, attempt, route, artifact, sink)
			if ctx.Err() != nil {
				return artifact, ctx.Err()
			}
			return artifact, err
		}
		s.failTeamRoleWithArtifact(role, attempt, route, artifact, err, sink)
		return artifact, err
	}
	s.completeTeamRole(role, attempt, route, artifact, sink)
	return artifact, nil
}

func (s *Service) resolveTeamRoleRoute(settings TeamRoleSetting, primary chatRoute) (chatRoute, error) {
	if strings.TrimSpace(settings.ProviderID) == "" {
		route := primary
		if strings.TrimSpace(settings.ModelID) != "" {
			route.ModelID = strings.TrimSpace(settings.ModelID)
		}
		return route, nil
	}
	runtime := s.stateRuntimeSettings()
	provider, _, ok := findModelProvider(runtime.Model.Providers, settings.ProviderID)
	if !ok || !provider.Enabled {
		return chatRoute{}, fmt.Errorf("团队角色 %s 的供应商不可用：%s", teamRoleLabel(settings.Role), settings.ProviderID)
	}
	route, err := s.chatRouteForProvider(runtime, provider)
	if err != nil {
		return chatRoute{}, err
	}
	if strings.TrimSpace(settings.ModelID) != "" {
		route.ModelID = strings.TrimSpace(settings.ModelID)
	}
	return route, nil
}

func teamRoleRequest(base protocol.ChatRequest, role string, attempt int, route chatRoute, artifacts []teamArtifact) protocol.ChatRequest {
	request := base
	request.Model = route.ModelID
	request.Metadata = make(map[string]string, len(base.Metadata)+3)
	for key, value := range base.Metadata {
		request.Metadata[key] = value
	}
	request.Metadata["request_kind"] = "team_role"
	request.Metadata["team_role"] = role
	request.Metadata["team_attempt"] = fmt.Sprintf("%d", attempt)
	if threadID := strings.TrimSuffix(strings.TrimSpace(base.ThreadID), ":"); threadID != "" {
		request.ThreadID = threadID + ":team:" + role
	}
	if turnID := strings.TrimSuffix(strings.TrimSpace(base.TurnID), ":"); turnID != "" {
		request.TurnID = turnID + ":" + role + ":" + fmt.Sprintf("%d", attempt)
	}
	request.Messages = append([]protocol.Message(nil), base.Messages...)
	request.Messages = append(request.Messages, protocol.Message{Role: "user", Content: teamRoleInstruction(role, attempt, artifacts)})
	request.Tools = nil
	return request
}

func teamRoleInstruction(role string, attempt int, artifacts []teamArtifact) string {
	handoff := formatTeamHandoff(artifacts)
	var instruction string
	switch role {
	case TeamRolePlanner:
		instruction = "你是 AI 团队的规划员。只读检查真实工作区，识别风险与验证方式；不要修改文件。最终只输出 3-8 步 Markdown 有序计划。"
	case TeamRoleImplementer:
		if attempt > 1 {
			instruction = "你是 AI 团队的实现者，正在进行审阅后的修订。使用结构化工具修复反馈中的真实问题，并运行必要验证；不要只解释。完成后简要列出改动和测试。"
		} else {
			instruction = "你是 AI 团队的实现者。依据用户任务与规划使用结构化工具完成真实代码修改，并运行必要验证；不要只给建议。完成后简要列出改动和测试。"
		}
	case TeamRoleTester:
		instruction = "你是 AI 团队的测试核验员。你只有只读工具；检查真实工作区、实现报告和已有验证证据，寻找未覆盖场景与回归。首行必须是 VERDICT: APPROVED 或 VERDICT: CHANGES_REQUIRED，随后给出具体证据。不要修改文件。"
	case TeamRoleReviewer:
		instruction = "你是 AI 团队的代码审阅员。你只有只读工具；核对用户目标、真实 diff、正确性、安全边界和测试缺口。首行必须是 VERDICT: APPROVED 或 VERDICT: CHANGES_REQUIRED，随后按严重程度列出发现。不要修改文件。"
	case TeamRoleSynthesizer:
		instruction = "你是 AI 团队的汇总员。基于各角色交接结果，向用户给出最终答复：先说明完成内容，再说明验证；如仍有风险要明确指出。不要提及内部提示，不要虚构测试或改动。"
	}
	if handoff == "" {
		return instruction
	}
	return instruction + "\n\n团队交接记录：\n" + handoff
}

func formatTeamHandoff(artifacts []teamArtifact) string {
	var out strings.Builder
	for _, artifact := range artifacts {
		content := compactTeamText(artifact.content, 5000)
		if content == "" {
			continue
		}
		fmt.Fprintf(&out, "\n[%s / %s]\n%s\n", artifact.role, artifact.route.ModelID, content)
	}
	return strings.TrimSpace(out.String())
}

func teamToolBudget(role string, maximum int) int {
	if maximum < 1 {
		maximum = 1
	}
	switch role {
	case TeamRoleImplementer:
		return maximum
	case TeamRolePlanner:
		if maximum/2 < 3 {
			return 3
		}
		return maximum / 2
	default:
		if maximum/4 < 3 {
			return 3
		}
		return maximum / 4
	}
}

func teamRoleToolSink(sink ChatEventSink) ChatEventSink {
	if sink == nil {
		return nil
	}
	return func(event ChatStreamEvent) {
		switch event.Type {
		case "tool", "progress", "context_compression":
			emitChatEvent(sink, event)
		}
	}
}

func teamOperationalParts(parts []tools.ResultPart) []tools.ResultPart {
	filtered := make([]tools.ResultPart, 0, len(parts))
	for _, part := range parts {
		if part.Kind != tools.PartText && part.Kind != tools.PartTeamRole {
			filtered = append(filtered, part)
		}
	}
	return filtered
}

func teamArtifactPart(artifact teamArtifact, attempt int, err error) tools.ResultPart {
	status := "completed"
	summary := compactTeamText(artifact.content, 1800)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			status = "cancelled"
			summary = "已取消"
		} else {
			status = "error"
			summary = err.Error()
		}
	}
	return tools.ResultPart{
		Kind:       tools.PartTeamRole,
		Role:       artifact.role,
		RoleLabel:  teamRoleLabel(artifact.role),
		ProviderID: artifact.route.Provider.ID,
		Model:      artifact.route.ModelID,
		Status:     status,
		Summary:    summary,
		Verdict:    artifact.verdict,
		Attempt:    attempt,
	}
}

func teamVerdict(role, content string) string {
	if role != TeamRoleTester && role != TeamRoleReviewer {
		return ""
	}
	upper := strings.ToUpper(compactTeamText(content, 600))
	if strings.Contains(upper, "CHANGES_REQUIRED") || strings.Contains(content, "需要修改") || strings.Contains(content, "必须修复") {
		return "changes_required"
	}
	if strings.Contains(upper, "APPROVED") || strings.Contains(content, "通过") {
		return "approved"
	}
	return "unknown"
}

func teamNeedsRevision(checks map[string]teamArtifact) bool {
	for _, artifact := range checks {
		if artifact.verdict == "changes_required" {
			return true
		}
	}
	return false
}

func reviewArtifacts(checks map[string]teamArtifact) string {
	var out strings.Builder
	for _, role := range []string{TeamRoleTester, TeamRoleReviewer} {
		artifact, ok := checks[role]
		if !ok || artifact.verdict != "changes_required" {
			continue
		}
		fmt.Fprintf(&out, "[%s]\n%s\n", teamRoleLabel(role), compactTeamText(artifact.content, 4000))
	}
	return strings.TrimSpace(out.String())
}

func fallbackTeamAnswer(implementation teamArtifact, checks map[string]teamArtifact) string {
	var out strings.Builder
	out.WriteString("AI 团队已完成本轮任务。")
	if summary := compactTeamText(implementation.content, 1800); summary != "" {
		out.WriteString("\n\n实现结果：\n")
		out.WriteString(summary)
	}
	for _, role := range []string{TeamRoleTester, TeamRoleReviewer} {
		artifact, ok := checks[role]
		if !ok || strings.TrimSpace(artifact.content) == "" {
			continue
		}
		fmt.Fprintf(&out, "\n\n%s结果：\n%s", teamRoleLabel(role), compactTeamText(artifact.content, 1200))
	}
	return out.String()
}

func compactTeamText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func addUsageMetrics(left, right cache.UsageMetrics) cache.UsageMetrics {
	left.PromptCacheHitTokens += right.PromptCacheHitTokens
	left.PromptCacheMissTokens += right.PromptCacheMissTokens
	left.InputTokens += right.InputTokens
	left.OutputTokens += right.OutputTokens
	left.EffectiveCost += right.EffectiveCost
	return left
}

func teamRoleSettings(settings TeamSettings, role string) TeamRoleSetting {
	for _, item := range settings.Roles {
		if item.Role == role {
			return item
		}
	}
	return TeamRoleSetting{Role: role, Enabled: role == TeamRoleImplementer || role == TeamRoleSynthesizer}
}

func roleEnabled(settings TeamSettings, role string) bool {
	return teamRoleSettings(settings, role).Enabled
}

func (s *Service) startTeamRun(settings TeamSettings, sink ChatEventSink) {
	roles := make([]TeamRoleState, 0, len(teamRoleOrder))
	for _, role := range teamRoleOrder {
		configured := teamRoleSettings(settings, role)
		status := "pending"
		if !configured.Enabled {
			status = "skipped"
		}
		roles = append(roles, TeamRoleState{
			Role: role, Label: teamRoleLabel(role), Enabled: configured.Enabled, Status: status,
			ProviderID: configured.ProviderID, Model: configured.ModelID,
		})
	}
	s.teamState = TeamState{
		Enabled: true, Active: true, RunID: fmt.Sprintf("team-%d", time.Now().UnixNano()),
		Status: "running", Roles: roles, StartedAt: time.Now().Format(time.RFC3339Nano),
	}
	emitChatEvent(sink, ChatStreamEvent{Type: "status", Message: "AI 团队正在分配角色"})
}

func (s *Service) startTeamRole(role string, attempt int, route chatRoute, sink ChatEventSink) {
	now := time.Now().Format(time.RFC3339Nano)
	state := s.teamRoleState(role)
	state.Status = "running"
	state.ProviderID = route.Provider.ID
	state.Model = route.ModelID
	state.Attempt = attempt
	state.Verdict = ""
	state.Summary = ""
	state.Error = ""
	state.StartedAt = now
	state.FinishedAt = ""
	s.setTeamRoleState(state)
	s.teamState.CurrentRole = role
	emitChatEvent(sink, ChatStreamEvent{Type: "team", Message: fmt.Sprintf("%s角色正在工作", state.Label), Model: route.ModelID, Team: teamEvent(s.teamState.RunID, state)})
}

func (s *Service) completeTeamRole(role string, attempt int, route chatRoute, artifact teamArtifact, sink ChatEventSink) {
	state := s.teamRoleState(role)
	state.Status = "completed"
	state.ProviderID = route.Provider.ID
	state.Model = route.ModelID
	state.Attempt = attempt
	state.Verdict = artifact.verdict
	state.Summary = compactTeamText(artifact.content, 1200)
	state.Error = ""
	state.Usage = artifact.usage
	state.FinishedAt = time.Now().Format(time.RFC3339Nano)
	s.setTeamRoleState(state)
	emitChatEvent(sink, ChatStreamEvent{Type: "team", Message: fmt.Sprintf("%s角色已完成", state.Label), Model: route.ModelID, Team: teamEvent(s.teamState.RunID, state)})
}

func (s *Service) cancelTeamRole(role string, attempt int, route chatRoute, artifact teamArtifact, sink ChatEventSink) {
	state := s.teamRoleState(role)
	state.Status = "cancelled"
	state.ProviderID = route.Provider.ID
	state.Model = route.ModelID
	state.Attempt = attempt
	state.Verdict = artifact.verdict
	state.Summary = compactTeamText(artifact.content, 1200)
	state.Error = ""
	state.Usage = artifact.usage
	state.FinishedAt = time.Now().Format(time.RFC3339Nano)
	s.setTeamRoleState(state)
	emitChatEvent(sink, ChatStreamEvent{Type: "team", Message: fmt.Sprintf("%s角色已取消", state.Label), Model: route.ModelID, Team: teamEvent(s.teamState.RunID, state)})
}

func (s *Service) failTeamRole(role string, attempt int, err error, sink ChatEventSink) {
	s.failTeamRoleWithArtifact(role, attempt, chatRoute{}, teamArtifact{role: role}, err, sink)
}

func (s *Service) failTeamRoleWithArtifact(role string, attempt int, route chatRoute, artifact teamArtifact, err error, sink ChatEventSink) {
	state := s.teamRoleState(role)
	state.Status = "error"
	state.Attempt = attempt
	state.ProviderID = route.Provider.ID
	state.Model = route.ModelID
	state.Verdict = artifact.verdict
	state.Summary = compactTeamText(artifact.content, 1200)
	state.Error = err.Error()
	state.Usage = artifact.usage
	state.FinishedAt = time.Now().Format(time.RFC3339Nano)
	s.setTeamRoleState(state)
	emitChatEvent(sink, ChatStreamEvent{Type: "team", Message: fmt.Sprintf("%s角色失败", state.Label), Model: route.ModelID, Team: teamEvent(s.teamState.RunID, state)})
}

func (s *Service) finishTeamRun(status, summary string, sink ChatEventSink) {
	s.teamState.Active = false
	s.teamState.Status = status
	s.teamState.CurrentRole = ""
	s.teamState.CompletedAt = time.Now().Format(time.RFC3339Nano)
	s.teamState.Summary = compactTeamText(summary, 1600)
	emitChatEvent(sink, ChatStreamEvent{Type: "team", Message: "AI 团队任务" + teamRunStatusLabel(status)})
}

func (s *Service) teamRoleState(role string) TeamRoleState {
	for _, state := range s.teamState.Roles {
		if state.Role == role {
			return state
		}
	}
	return TeamRoleState{Role: role, Label: teamRoleLabel(role), Enabled: true}
}

func (s *Service) setTeamRoleState(next TeamRoleState) {
	for index := range s.teamState.Roles {
		if s.teamState.Roles[index].Role == next.Role {
			s.teamState.Roles[index] = next
			return
		}
	}
	s.teamState.Roles = append(s.teamState.Roles, next)
}

func teamEvent(runID string, state TeamRoleState) *TeamRoleEvent {
	return &TeamRoleEvent{
		RunID: runID, Role: state.Role, Label: state.Label, Status: state.Status,
		ProviderID: state.ProviderID, Model: state.Model, Attempt: state.Attempt,
		Verdict: state.Verdict, Summary: state.Summary, Error: state.Error, Usage: state.Usage,
	}
}

func teamRunStatusLabel(status string) string {
	switch status {
	case "completed":
		return "已完成"
	case "cancelled":
		return "已取消"
	default:
		return "失败"
	}
}

var errTeamModeRequiresPlanner = errors.New("AI 团队模式需要高或超高推理强度")
