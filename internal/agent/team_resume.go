package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

const (
	teamStagePlanApproval = "plan_approval"
	teamStageFinalize     = "finalize"
)

// runTeamTurn persists every role transition. A stopped or crashed run can
// therefore resume at the unfinished role without replaying completed work.
func (s *Service) runTeamTurn(
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
	checkpoint := cloneTeamRunCheckpoint(s.teamResume)
	resuming := isTeamResumeTurn(ctx) && checkpoint != nil
	if !resuming {
		checkpoint = newTeamRunCheckpoint(settings)
		s.startTeamRun(settings, sink)
	} else {
		s.resumeTeamRun(checkpoint, settings, sink)
	}

	artifacts := teamArtifactsFromCheckpoint(checkpoint)
	parts := append([]tools.ResultPart(nil), checkpoint.Parts...)
	aggregate := checkpoint.Usage
	planStarted := checkpoint.PlanStarted
	planApproved := checkpoint.PlanApproved
	reviewRound := checkpoint.ReviewRound
	if checkpoint.NextAttempt < 1 {
		checkpoint.NextAttempt = 1
	}
	if strings.TrimSpace(checkpoint.NextRole) == "" {
		checkpoint.NextRole = teamStageFinalize
	}
	if resuming {
		parts = removeTeamRolePart(parts, checkpoint.NextRole, checkpoint.NextAttempt)
	}

	syncCheckpoint := func(status, nextRole string, nextAttempt int) error {
		if nextAttempt < 1 {
			nextAttempt = 1
		}
		checkpoint.Status = status
		checkpoint.NextRole = nextRole
		checkpoint.NextAttempt = nextAttempt
		checkpoint.ReviewRound = reviewRound
		checkpoint.PlanStarted = planStarted
		checkpoint.PlanApproved = planApproved
		checkpoint.Artifacts = checkpointTeamArtifacts(artifacts)
		checkpoint.Parts = append([]tools.ResultPart(nil), parts...)
		checkpoint.Usage = aggregate
		return s.persistTeamRunCheckpoint(checkpoint)
	}

	failRun := func(cause error) (ChatResult, error) {
		cause = s.failStartedPlan(planStarted, cause)
		s.finishTeamRun("failed", cause.Error(), sink)
		if checkpointErr := syncCheckpoint("failed", checkpoint.NextRole, checkpoint.NextAttempt); checkpointErr != nil {
			cause = errors.Join(cause, checkpointErr)
		}
		if baseMessageCount >= 0 && baseMessageCount <= len(s.sessionMessages) {
			s.sessionMessages = s.sessionMessages[:baseMessageCount]
		}
		return ChatResult{State: s.workbenchStateLocked(), Model: primary.ModelID, Parts: parts}, cause
	}

	pauseRun := func(stage string, attempt int) (ChatResult, error) {
		model := primary.ModelID
		if isTeamRole(stage) {
			state := s.teamRoleState(stage)
			state.Status = "paused"
			state.Attempt = attempt
			state.Error = ""
			state.FinishedAt = time.Now().Format(time.RFC3339Nano)
			s.setTeamRoleState(state)
			parts = markTeamRolePartPaused(parts, stage, attempt)
			if state.Model != "" {
				model = state.Model
			}
			emitChatEvent(sink, ChatStreamEvent{
				Type: "team", Message: fmt.Sprintf("%s角色已暂停", state.Label), Model: model,
				Team: teamEvent(s.teamState.RunID, state),
			})
		}
		s.teamState.Active = false
		s.teamState.Status = "paused"
		s.teamState.CurrentRole = stage
		s.teamState.CompletedAt = time.Now().Format(time.RFC3339Nano)
		s.teamState.Summary = fmt.Sprintf("已暂停，将从%s继续", teamStageLabel(stage))

		checkpointErr := syncCheckpoint("paused", stage, attempt)
		answer := fmt.Sprintf(
			"AI 团队已暂停。已保留 %d 个完成的角色回合和当前文件修改；发送“继续”将从%s继续。",
			len(artifacts), teamStageLabel(stage),
		)
		resultParts := append(append([]tools.ResultPart(nil), parts...), tools.ResultPart{Kind: tools.PartText, Text: answer})
		s.metrics = aggregate
		s.sessionMessages = append(s.sessionMessages, protocol.Message{Role: "assistant", Content: answer})
		s.commitRequestPrefix(prefixDiagnostic, requestMessages)
		s.sessionState.MessageCount = len(s.sessionMessages)
		s.recordTeamPauseMessage(answer, model, resultParts, chatTurnDurationMs(ctx))

		cancelErr := ctx.Err()
		if cancelErr == nil {
			cancelErr = context.Canceled
		}
		pauseErr := errors.Join(errTeamRunPaused, cancelErr)
		if checkpointErr != nil {
			pauseErr = errors.Join(pauseErr, checkpointErr)
		}
		return ChatResult{
			Content: answer, Model: model, Usage: aggregate,
			State: s.workbenchStateLocked(), Parts: resultParts,
		}, pauseErr
	}

	runRole := func(role string, attempt int, prior []teamArtifact) (teamArtifact, error) {
		if err := ctx.Err(); err != nil {
			return teamArtifact{role: role, attempt: attempt}, err
		}
		parts = removeTeamRolePart(parts, role, attempt)
		roleSettings := teamRoleSettings(settings, role)
		artifact, roleErr := s.runTeamRole(ctx, roleSettings, attempt, primary, baseRequest, maxToolCalls, prior, sink)
		if artifact.route.Provider.ID != "" {
			aggregate = addUsageMetrics(aggregate, artifact.usage)
		}
		parts = append(parts, teamOperationalParts(artifact.parts)...)
		parts = append(parts, teamArtifactPart(artifact, attempt, roleErr))
		if roleErr == nil {
			artifacts = append(artifacts, artifact)
		}
		return artifact, roleErr
	}

	if err := syncCheckpoint("running", checkpoint.NextRole, checkpoint.NextAttempt); err != nil {
		return failRun(err)
	}

	for {
		stage := checkpoint.NextRole
		attempt := checkpoint.NextAttempt
		if err := ctx.Err(); err != nil {
			return pauseRun(stage, attempt)
		}

		switch stage {
		case TeamRolePlanner:
			plan, roleErr := runRole(TeamRolePlanner, 1, nil)
			if roleErr != nil {
				if ctx.Err() != nil || errors.Is(roleErr, context.Canceled) {
					return pauseRun(TeamRolePlanner, 1)
				}
				emitChatEvent(sink, ChatStreamEvent{Type: "status", Message: "规划角色暂不可用，团队将直接进入实现", Model: primary.ModelID})
				checkpoint.NextRole = TeamRoleImplementer
				checkpoint.NextAttempt = 1
				if err := syncCheckpoint("running", checkpoint.NextRole, checkpoint.NextAttempt); err != nil {
					return failRun(err)
				}
				continue
			}

			if s.planMode && strings.TrimSpace(plan.content) != "" {
				steps := planStepsFromText(plan.content)
				if len(steps) > 0 {
					if err := s.startPlanState(steps); err != nil {
						return failRun(err)
					}
					planStarted = true
				}
				checkpoint.NextRole = teamStagePlanApproval
			} else {
				checkpoint.NextRole = TeamRoleImplementer
			}
			checkpoint.NextAttempt = 1
			if err := syncCheckpoint("running", checkpoint.NextRole, checkpoint.NextAttempt); err != nil {
				return failRun(err)
			}

		case teamStagePlanApproval:
			plan, ok := teamArtifactAt(artifacts, TeamRolePlanner, 1)
			if !ok || !s.planMode || strings.TrimSpace(plan.content) == "" {
				checkpoint.NextRole = TeamRoleImplementer
				checkpoint.NextAttempt = 1
				if err := syncCheckpoint("running", checkpoint.NextRole, checkpoint.NextAttempt); err != nil {
					return failRun(err)
				}
				continue
			}

			if !planApproved {
				approved, approvalErr := s.requestPlanApproval(ctx, plan.content)
				if approvalErr != nil {
					if ctx.Err() != nil || errors.Is(approvalErr, context.Canceled) {
						return pauseRun(teamStagePlanApproval, 1)
					}
					return failRun(approvalErr)
				}
				if !approved {
					if planStarted {
						if err := s.finishPlanState("cancelled"); err != nil {
							return failRun(err)
						}
					}
					answer := "团队已完成规划，但你选择暂不执行。\n\n" + plan.content
					parts = append(parts, tools.ResultPart{Kind: tools.PartText, Text: answer})
					s.metrics = aggregate
					s.finishTeamRun("cancelled", "计划未获批准", sink)
					if err := syncCheckpoint("cancelled", teamStageFinalize, 1); err != nil {
						return failRun(err)
					}
					s.sessionMessages = append(s.sessionMessages, protocol.Message{Role: "assistant", Content: answer})
					s.commitRequestPrefix(prefixDiagnostic, requestMessages)
					s.sessionState.MessageCount = len(s.sessionMessages)
					s.sessionState.TurnCount++
					s.recordAssistantAndCheckpoint(answer, plan.route.ModelID, parts, chatTurnDurationMs(ctx))
					return ChatResult{Content: answer, Model: plan.route.ModelID, Usage: aggregate, State: s.workbenchStateLocked(), Parts: parts}, nil
				}
				planApproved = true
			}

			if planStarted && len(s.planState.Steps) > 0 {
				steps := append([]tools.ProgressStep(nil), s.planState.Steps...)
				if steps[0].Status == "pending" {
					steps[0].Status = "in_progress"
					if err := s.updatePlanState(steps); err != nil {
						return failRun(err)
					}
				}
			}
			checkpoint.NextRole = TeamRoleImplementer
			checkpoint.NextAttempt = 1
			if err := syncCheckpoint("running", checkpoint.NextRole, checkpoint.NextAttempt); err != nil {
				return failRun(err)
			}

		case TeamRoleImplementer:
			prior := append([]teamArtifact(nil), artifacts...)
			if attempt > 1 {
				feedback := reviewArtifacts(checksForAttempt(artifacts, attempt-1))
				if feedback != "" {
					prior = append(prior, teamArtifact{
						role: TeamRoleReviewer, attempt: attempt - 1,
						content: "需要修订：\n" + feedback, verdict: "changes_required",
					})
				}
			}
			_, roleErr := runRole(TeamRoleImplementer, attempt, prior)
			if roleErr != nil {
				if ctx.Err() != nil || errors.Is(roleErr, context.Canceled) {
					return pauseRun(TeamRoleImplementer, attempt)
				}
				return failRun(roleErr)
			}
			checkpoint.NextRole = firstTeamCheckRole(settings)
			if checkpoint.NextRole == "" {
				checkpoint.NextRole = teamStageAfterChecks(settings)
			}
			checkpoint.NextAttempt = attempt
			if err := syncCheckpoint("running", checkpoint.NextRole, checkpoint.NextAttempt); err != nil {
				return failRun(err)
			}

		case TeamRoleTester, TeamRoleReviewer:
			if !roleEnabled(settings, stage) {
				checkpoint.NextRole = nextTeamCheckRole(settings, stage)
				if checkpoint.NextRole == "" {
					checkpoint.NextRole = teamStageAfterChecks(settings)
				}
				if err := syncCheckpoint("running", checkpoint.NextRole, attempt); err != nil {
					return failRun(err)
				}
				continue
			}

			_, roleErr := runRole(stage, attempt, artifacts)
			if roleErr != nil && (ctx.Err() != nil || errors.Is(roleErr, context.Canceled)) {
				return pauseRun(stage, attempt)
			}
			nextRole := nextTeamCheckRole(settings, stage)
			if nextRole == "" {
				checks := checksForAttempt(artifacts, attempt)
				if teamNeedsRevision(checks) && attempt-1 < settings.MaxReviewRounds {
					reviewRound = attempt
					nextRole = TeamRoleImplementer
					attempt++
				} else {
					nextRole = teamStageAfterChecks(settings)
				}
			}
			checkpoint.NextRole = nextRole
			checkpoint.NextAttempt = attempt
			if err := syncCheckpoint("running", checkpoint.NextRole, checkpoint.NextAttempt); err != nil {
				return failRun(err)
			}

		case TeamRoleSynthesizer:
			if !roleEnabled(settings, TeamRoleSynthesizer) {
				checkpoint.NextRole = teamStageFinalize
				checkpoint.NextAttempt = 1
				if err := syncCheckpoint("running", checkpoint.NextRole, checkpoint.NextAttempt); err != nil {
					return failRun(err)
				}
				continue
			}
			_, roleErr := runRole(TeamRoleSynthesizer, 1, artifacts)
			if roleErr != nil && (ctx.Err() != nil || errors.Is(roleErr, context.Canceled)) {
				return pauseRun(TeamRoleSynthesizer, 1)
			}
			checkpoint.NextRole = teamStageFinalize
			checkpoint.NextAttempt = 1
			if err := syncCheckpoint("running", checkpoint.NextRole, checkpoint.NextAttempt); err != nil {
				return failRun(err)
			}

		case teamStageFinalize:
			implementation, _ := latestTeamArtifact(artifacts, TeamRoleImplementer)
			checks := checksForAttempt(artifacts, implementation.attempt)
			final, hasFinal := latestTeamArtifact(artifacts, TeamRoleSynthesizer)
			answer := ""
			if hasFinal {
				answer = strings.TrimSpace(final.content)
			}
			if answer == "" {
				answer = fallbackTeamAnswer(implementation, checks)
			}
			answer = sanitizeModelContent(answer)
			parts = append(parts, tools.ResultPart{Kind: tools.PartText, Text: answer})
			s.metrics = aggregate
			model := primary.ModelID
			if hasFinal && final.route.ModelID != "" {
				model = final.route.ModelID
			}
			if planStarted {
				if err := s.finishPlanState("completed"); err != nil {
					return failRun(err)
				}
			}
			s.finishTeamRun("completed", answer, sink)
			if err := syncCheckpoint("completed", teamStageFinalize, 1); err != nil {
				return failRun(err)
			}
			s.sessionMessages = append(s.sessionMessages, protocol.Message{Role: "assistant", Content: answer})
			s.commitRequestPrefix(prefixDiagnostic, requestMessages)
			s.sessionState.MessageCount = len(s.sessionMessages)
			s.sessionState.TurnCount++
			s.recordAssistantAndCheckpoint(answer, model, parts, chatTurnDurationMs(ctx))
			s.markChatProviderStatus(primary.Provider.ID, "ok", fmt.Sprintf("AI 团队任务完成，共执行 %d 个角色回合。", len(artifacts)))
			return ChatResult{Content: answer, Model: model, Usage: aggregate, State: s.workbenchStateLocked(), Parts: parts}, nil

		default:
			return failRun(fmt.Errorf("unknown AI team checkpoint stage %q", stage))
		}
	}
}

func (s *Service) resumeTeamRun(checkpoint *teamRunCheckpoint, settings TeamSettings, sink ChatEventSink) {
	if len(checkpoint.Team.Roles) == 0 {
		s.startTeamRun(settings, sink)
	} else {
		s.teamState = cloneTeamState(checkpoint.Team)
	}
	if s.teamState.RunID == "" {
		s.teamState.RunID = fmt.Sprintf("team-%d", time.Now().UnixNano())
	}
	s.teamState.Enabled = true
	s.teamState.Active = true
	s.teamState.Status = "running"
	s.teamState.CompletedAt = ""
	s.teamState.Summary = ""
	s.teamState.CurrentRole = checkpoint.NextRole
	if isTeamRole(checkpoint.NextRole) {
		state := s.teamRoleState(checkpoint.NextRole)
		if state.Status == "paused" || state.Status == "cancelled" || state.Status == "running" {
			state.Status = "pending"
			state.Error = ""
			state.FinishedAt = ""
			s.setTeamRoleState(state)
		}
	}
	emitChatEvent(sink, ChatStreamEvent{
		Type:    "status",
		Message: fmt.Sprintf("AI 团队已恢复，正在从%s继续", teamStageLabel(checkpoint.NextRole)),
	})
}

func firstTeamCheckRole(settings TeamSettings) string {
	return nextTeamCheckRole(settings, "")
}

func nextTeamCheckRole(settings TeamSettings, current string) string {
	roles := []string{TeamRoleTester, TeamRoleReviewer}
	start := 0
	if current != "" {
		start = len(roles)
		for index, role := range roles {
			if role == current {
				start = index + 1
				break
			}
		}
	}
	for _, role := range roles[start:] {
		if roleEnabled(settings, role) {
			return role
		}
	}
	return ""
}

func teamStageAfterChecks(settings TeamSettings) string {
	if roleEnabled(settings, TeamRoleSynthesizer) {
		return TeamRoleSynthesizer
	}
	return teamStageFinalize
}

func checksForAttempt(artifacts []teamArtifact, attempt int) map[string]teamArtifact {
	checks := make(map[string]teamArtifact, 2)
	for _, role := range []string{TeamRoleTester, TeamRoleReviewer} {
		if artifact, ok := teamArtifactAt(artifacts, role, attempt); ok {
			checks[role] = artifact
		}
	}
	return checks
}

func latestTeamArtifact(artifacts []teamArtifact, role string) (teamArtifact, bool) {
	for index := len(artifacts) - 1; index >= 0; index-- {
		if artifacts[index].role == role {
			return artifacts[index], true
		}
	}
	return teamArtifact{}, false
}

func removeTeamRolePart(parts []tools.ResultPart, role string, attempt int) []tools.ResultPart {
	filtered := make([]tools.ResultPart, 0, len(parts))
	for _, part := range parts {
		partAttempt := part.Attempt
		if partAttempt < 1 {
			partAttempt = 1
		}
		if part.Kind == tools.PartTeamRole && part.Role == role && partAttempt == attempt {
			continue
		}
		filtered = append(filtered, part)
	}
	return filtered
}

func markTeamRolePartPaused(parts []tools.ResultPart, role string, attempt int) []tools.ResultPart {
	marked := append([]tools.ResultPart(nil), parts...)
	for index := len(marked) - 1; index >= 0; index-- {
		partAttempt := marked[index].Attempt
		if partAttempt < 1 {
			partAttempt = 1
		}
		if marked[index].Kind == tools.PartTeamRole && marked[index].Role == role && partAttempt == attempt {
			marked[index].Status = "paused"
			marked[index].Summary = "已暂停，继续后将从此角色恢复"
			return marked
		}
	}
	return marked
}

func teamStageLabel(stage string) string {
	switch stage {
	case teamStagePlanApproval:
		return "计划确认"
	case teamStageFinalize:
		return "结果整理"
	default:
		return teamRoleLabel(stage)
	}
}
