package agent

import (
	"fmt"
	"strings"

	"github.com/MISSmihu/MHcode/internal/protocol"
	"github.com/MISSmihu/MHcode/internal/tools"
)

const (
	executionContextStart = "[MHcode execution checkpoint]"
	executionContextEnd   = "[/MHcode execution checkpoint]"
	contextExecutionKind  = "execution-context"
)

func (s *Service) formatExecutionCheckpoint(parts []tools.ResultPart) string {
	lines := make([]string, 0, 24)
	if plan := formatPlanCheckpoint(s.planState); plan != "" {
		lines = append(lines, plan)
	}
	if team := formatTeamCheckpoint(s.teamResume); team != "" {
		lines = append(lines, team)
	}

	toolParts := 0
	for _, part := range parts {
		switch part.Kind {
		case tools.PartToolCall:
			if toolParts >= 16 {
				continue
			}
			toolParts++
			lines = append(lines, formatToolCheckpoint(part))
		case tools.PartProgress:
			if progress := formatProgressCheckpoint(part); progress != "" {
				lines = append(lines, progress)
			}
		case tools.PartSubagent:
			lines = append(lines, fmt.Sprintf(
				"subagent task_id=%s type=%s status=%s label=%q action=%q",
				checkpointValue(part.TaskID), checkpointValue(part.AgentType), checkpointValue(part.Status),
				checkpointText(part.Label, 300), checkpointText(part.CurrentAction, 500),
			))
		case tools.PartTeamRole:
			lines = append(lines, fmt.Sprintf(
				"team_role role=%s attempt=%d status=%s verdict=%s summary=%q",
				checkpointValue(part.Role), part.Attempt, checkpointValue(part.Status), checkpointValue(part.Verdict),
				checkpointText(part.Summary, 800),
			))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	content := strings.Join([]string{
		executionContextStart,
		"Branch-local operational facts from completed or interrupted work. Continue from these facts, reuse successful results, and do not repeat a failed strategy without a substantive change.",
		strings.Join(lines, "\n"),
		executionContextEnd,
	}, "\n")
	return clipDelimitedContext(content, executionContextStart, executionContextEnd, 8_000)
}

func formatPlanCheckpoint(plan PlanState) string {
	if len(plan.Steps) == 0 {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "plan revision=%d status=%s", plan.Revision, checkpointValue(plan.Status))
	for index, step := range plan.Steps {
		fmt.Fprintf(&out, "\n  step=%d status=%s title=%q", index+1, checkpointValue(step.Status), checkpointText(step.Title, 500))
	}
	return out.String()
}

func formatTeamCheckpoint(checkpoint *teamRunCheckpoint) string {
	if checkpoint == nil {
		return ""
	}
	return fmt.Sprintf(
		"team run_id=%s status=%s next_role=%s next_attempt=%d review_round=%d completed_roles=%d",
		checkpointValue(checkpoint.Team.RunID), checkpointValue(checkpoint.Status), checkpointValue(checkpoint.NextRole),
		checkpoint.NextAttempt, checkpoint.ReviewRound, len(checkpoint.Artifacts),
	)
}

func formatToolCheckpoint(part tools.ResultPart) string {
	var out strings.Builder
	fmt.Fprintf(&out, "tool name=%s status=%s call_id=%s", checkpointValue(part.Name), checkpointValue(part.Status), checkpointValue(part.ToolCallID))
	if part.WorkingDirectory != "" {
		fmt.Fprintf(&out, " cwd=%q", checkpointText(part.WorkingDirectory, 800))
	}
	if part.ExitCode != nil {
		fmt.Fprintf(&out, " exit_code=%d", *part.ExitCode)
	}
	if part.DurationMs > 0 {
		fmt.Fprintf(&out, " duration_ms=%d", part.DurationMs)
	}
	appendCheckpointField(&out, "input", part.Input, 2_000)
	appendCheckpointField(&out, "stdout", part.Stdout, 1_500)
	appendCheckpointField(&out, "stderr", part.Stderr, 2_000)
	if strings.TrimSpace(part.Stdout) == "" && strings.TrimSpace(part.Stderr) == "" {
		appendCheckpointField(&out, "output", part.Output, 1_500)
	}
	return out.String()
}

func formatProgressCheckpoint(part tools.ResultPart) string {
	if len(part.Steps) == 0 && strings.TrimSpace(part.TaskStatus) == "" {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "task status=%s changed_files=%d", checkpointValue(part.TaskStatus), part.ChangedFiles)
	for index, step := range part.Steps {
		fmt.Fprintf(&out, "\n  step=%d status=%s title=%q", index+1, checkpointValue(step.Status), checkpointText(step.Title, 500))
	}
	return out.String()
}

func appendCheckpointField(out *strings.Builder, name, value string, maxRunes int) {
	value = checkpointText(value, maxRunes)
	if value == "" {
		return
	}
	fmt.Fprintf(out, "\n  %s=%q", name, value)
}

func checkpointText(value string, maxRunes int) string {
	value = redactSensitiveText(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return clipContextText(value, maxRunes)
}

func checkpointValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func latestExecutionCheckpoint(messages []protocol.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if checkpoint := extractExecutionCheckpoint(messages[index].Content); checkpoint != "" {
			return checkpoint
		}
	}
	return ""
}

func extractExecutionCheckpoint(content string) string {
	start := strings.LastIndex(content, executionContextStart)
	if start < 0 {
		return ""
	}
	relativeEnd := strings.Index(content[start+len(executionContextStart):], executionContextEnd)
	if relativeEnd < 0 {
		return ""
	}
	end := start + len(executionContextStart) + relativeEnd + len(executionContextEnd)
	return strings.TrimSpace(content[start:end])
}

func stripExecutionCheckpoint(content string) string {
	for {
		start := strings.Index(content, executionContextStart)
		if start < 0 {
			return strings.TrimSpace(content)
		}
		relativeEnd := strings.Index(content[start+len(executionContextStart):], executionContextEnd)
		if relativeEnd < 0 {
			return strings.TrimSpace(content[:start])
		}
		end := start + len(executionContextStart) + relativeEnd + len(executionContextEnd)
		content = strings.TrimSpace(content[:start]) + "\n" + strings.TrimSpace(content[end:])
	}
}

func (s *Service) continuationExecutionContext(userInput string) string {
	continuing := isContinuationRequest(userInput)
	resumablePlan := len(s.planState.Steps) > 0 && s.planState.Status != "completed"
	resumableTeam := s.teamResume != nil && (s.teamResume.Status == "paused" || s.teamResume.Status == "running")
	if !continuing && !resumablePlan && !resumableTeam {
		return ""
	}
	if checkpoint := latestExecutionCheckpoint(s.sessionMessages); checkpoint != "" {
		return checkpoint
	}
	return s.formatExecutionCheckpoint(nil)
}
