package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/MISSmihu/MHcode/internal/eventlog"
	"github.com/MISSmihu/MHcode/internal/project"
	"github.com/MISSmihu/MHcode/internal/protocol"
)

// ActiveSessionIDs returns the manifest pointers without changing them.
// Chat task runners use it only to bind a new task to the current conversation.
func (s *Service) ActiveSessionIDs() (projectID, sessionID string) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	if s.projectID != "" && s.sessionID != "" {
		return s.projectID, s.sessionID
	}
	if s.projects == nil {
		return s.projectID, s.sessionID
	}
	return s.projects.ActiveIDs()
}

// NewSessionRuntime creates an isolated Agent state machine for one session.
// Provider configuration, MCP, credentials, and host bridges are shared, while
// conversation history, event log, compression counters, plan/team state,
// approvals, and usage samples are private to the returned runtime.
//
// The application-level active session is deliberately not changed. This is
// what allows a user to switch conversations while another task is streaming.
func (s *Service) NewSessionRuntime(sessionID string) (*Service, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		_, sessionID = s.ActiveSessionIDs()
	}

	s.stateMu.RLock()
	projects := s.projects
	config := s.config
	settings := cloneRuntimeSettings(s.runtimeSettings)
	reasoning := s.reasoning
	deepSeekState := cloneDeepSeekState(s.deepSeekState)
	secretVault := s.secretVault
	builder := s.builder
	mcpManager := s.mcpManager
	usageStore := s.usageStore
	usageLedger := s.usageLedger
	planMode := s.planMode
	providerFactory := s.providerFactory
	baseMessages := cloneProtocolMessages(s.sessionMessages)
	baseSessionState := s.sessionState
	teamState := cloneTeamState(s.teamState)
	snapshot := cloneWorkbenchState(s.stateSnapshot)
	var notify func(ApprovalRequest)
	if s.approvals != nil {
		notify = s.approvals.notification()
	}
	s.stateMu.RUnlock()

	if projects == nil || strings.TrimSpace(config.SessionsDir) == "" {
		// In-memory/test services have no persistent session directory. They can
		// still get an isolated copy, but there is no event log to reopen.
		runtime := &Service{
			config:          config,
			reasoning:       reasoning,
			deepSeekState:   deepSeekState,
			runtimeSettings: settings,
			settingsPath:    config.SettingsPath,
			secretVault:     secretVault,
			builder:         builder,
			mcpManager:      mcpManager,
			usageStore:      usageStore,
			usageLedger:     usageLedger,
			planMode:        planMode,
			providerFactory: providerFactory,
			approvals:       newApprovalBroker(),
			sessionID:       sessionID,
			sessionMessages: baseMessages,
			sessionState:    baseSessionState,
		}
		runtime.approvals.SetNotify(notify)
		runtime.teamState = teamState
		runtime.stateSnapshot = snapshot
		return runtime, nil
	}

	projectID, _, ok := projects.FindSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}
	workspace, ok := projects.Project(projectID)
	if !ok {
		return nil, fmt.Errorf("项目不存在: %s", projectID)
	}
	if strings.TrimSpace(workspace.WorkspaceRoot) != "" {
		settings.WorkspaceRoot = workspace.WorkspaceRoot
		settings.ExtraWritableRoots = append([]string(nil), workspace.ExtraWritableRoots...)
	}

	runtime := &Service{
		config:          config,
		reasoning:       reasoning,
		deepSeekState:   deepSeekState,
		runtimeSettings: settings,
		settingsPath:    config.SettingsPath,
		secretVault:     secretVault,
		builder:         builder,
		mcpManager:      mcpManager,
		usageStore:      usageStore,
		usageLedger:     usageLedger,
		planMode:        planMode,
		providerFactory: providerFactory,
		projects:        projects,
		approvals:       newApprovalBroker(),
		sessionID:       sessionID,
		projectID:       projectID,
		teamState:       TeamState{Enabled: settings.Team.Enabled, Status: "idle", Roles: []TeamRoleState{}},
	}
	runtime.approvals.SetNotify(notify)

	store, err := eventlog.Open(filepath.Join(config.SessionsDir, projectID, sessionID))
	if err != nil {
		return nil, fmt.Errorf("打开会话事件日志失败: %w", err)
	}
	runtime.eventStore = store
	runtime.rebuildSessionFromEvents()
	runtime.restoreUsageMetrics()
	runtime.refreshProjectMemory()
	runtime.storeWorkbenchSnapshot(runtime.workbenchStateLocked())
	return runtime, nil
}

func cloneRuntimeSettings(settings RuntimeSettings) RuntimeSettings {
	encoded, err := json.Marshal(settings)
	if err != nil {
		return settings
	}
	var cloned RuntimeSettings
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return settings
	}
	return cloned.Normalized()
}

func cloneDeepSeekState(state DeepSeekState) DeepSeekState {
	state.Models = append([]protocol.Model(nil), state.Models...)
	return state
}

// sessionProject is intentionally small and detached; it is useful to callers
// that need to validate a task target without changing the active session.
func (s *Service) sessionProject(sessionID string) (projectID string, session project.Session, err error) {
	s.stateMu.RLock()
	projects := s.projects
	s.stateMu.RUnlock()
	if projects == nil {
		return "", project.Session{}, fmt.Errorf("会话运行时不可用")
	}
	projectID, session, ok := projects.FindSession(sessionID)
	if !ok {
		return "", project.Session{}, fmt.Errorf("会话不存在: %s", sessionID)
	}
	return projectID, session, nil
}
