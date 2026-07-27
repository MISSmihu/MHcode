package agent

import (
	"strings"
	"time"

	"github.com/MISSmihu/MHcode/internal/tools"
)

const maxTurnTimelineNotes = 128

func (s *Service) resetTurnTimeline() {
	s.turnTimelineMu.Lock()
	s.turnTimelineParts = nil
	s.turnTimelineMu.Unlock()
}

func (s *Service) captureTurnTimeline(sink ChatEventSink) ChatEventSink {
	return func(event ChatStreamEvent) {
		s.captureTurnTimelineEvent(event)
		emitChatEvent(sink, event)
	}
}

func (s *Service) captureTurnTimelineEvent(event ChatStreamEvent) {
	s.turnTimelineMu.Lock()
	defer s.turnTimelineMu.Unlock()

	switch event.Type {
	case "status", "context_compression":
		message := strings.TrimSpace(event.Message)
		if message == "" || isRoutineTaskStatus(message) {
			return
		}
		status := strings.TrimSpace(event.Status)
		if event.Type == "context_compression" && event.Compression != nil {
			status = event.Compression.Status
		}
		if status == "" {
			status = "running"
		}
		message = redactSensitiveText(message)
		if last := lastTimelineNote(s.turnTimelineParts); last != nil && last.Message == message && last.Status == status {
			return
		}
		if timelineNoteCount(s.turnTimelineParts) >= maxTurnTimelineNotes {
			return
		}
		s.turnTimelineParts = append(s.turnTimelineParts, tools.ResultPart{
			Kind: tools.PartTimelineNote, Message: message, Status: status,
			StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	case "tool":
		state := TaskRuntimeState{Parts: s.turnTimelineParts}
		upsertTaskRuntimeToolPart(&state, event)
		state.Parts = mergeTaskRuntimeParts(state.Parts, redactTaskRuntimeParts(event.Parts))
		s.turnTimelineParts = state.Parts
	case "progress":
		if event.Progress != nil {
			s.turnTimelineParts = mergeTaskRuntimeParts(s.turnTimelineParts, redactTaskRuntimeParts([]tools.ResultPart{*event.Progress}))
		}
	case "team":
		if event.Team != nil {
			s.turnTimelineParts = mergeTaskRuntimeParts(s.turnTimelineParts, []tools.ResultPart{taskRuntimeTeamPart(*event.Team)})
		}
	case "subagent", "provider_notice":
		s.turnTimelineParts = mergeTaskRuntimeParts(s.turnTimelineParts, redactTaskRuntimeParts(event.Parts))
	}
}

func isRoutineTaskStatus(message string) bool {
	message = strings.TrimSpace(message)
	return message == "正在准备上下文" ||
		message == "正在分析任务" ||
		message == "正在生成执行计划" ||
		strings.HasPrefix(message, "正在连接 ") ||
		strings.HasPrefix(message, "上游模型仍在处理")
}

func (s *Service) mergeTurnTimelineParts(parts []tools.ResultPart, content, terminalStatus string) []tools.ResultPart {
	s.turnTimelineMu.Lock()
	timeline := redactTaskRuntimeParts(s.turnTimelineParts)
	s.turnTimelineMu.Unlock()
	if len(timeline) == 0 {
		return parts
	}

	for index := range timeline {
		if timeline[index].Kind != tools.PartTimelineNote {
			continue
		}
		if !taskRuntimePartTerminal(timeline[index].Status) {
			timeline[index].Status = terminalStatus
			timeline[index].CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	merged := mergeTaskRuntimeParts(timeline, redactTaskRuntimeParts(parts))
	return appendTextPartIfMissing(merged, sanitizeModelContent(content))
}

func lastTimelineNote(parts []tools.ResultPart) *tools.ResultPart {
	for index := len(parts) - 1; index >= 0; index-- {
		if parts[index].Kind == tools.PartTimelineNote {
			return &parts[index]
		}
	}
	return nil
}

func timelineNoteCount(parts []tools.ResultPart) int {
	count := 0
	for _, part := range parts {
		if part.Kind == tools.PartTimelineNote {
			count++
		}
	}
	return count
}
