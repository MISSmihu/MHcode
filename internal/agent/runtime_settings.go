package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type RuntimeSettings struct {
	SandboxMode         string                  `json:"sandboxMode"`
	FilesystemAccess    string                  `json:"filesystemAccess"`
	NetworkAccess       bool                    `json:"networkAccess"`
	ShellAccess         bool                    `json:"shellAccess"`
	ApprovalPolicy      string                  `json:"approvalPolicy"`
	WorkspaceRoot       string                  `json:"workspaceRoot"`
	ExtraWritableRoots  []string                `json:"extraWritableRoots"`
	MaxCommandSeconds   int                     `json:"maxCommandSeconds"`
	AllowDestructiveOps bool                    `json:"allowDestructiveOps"`
	ToolResultPolicy    string                  `json:"toolResultPolicy"`
	StablePrefixPolicy  string                  `json:"stablePrefixPolicy"`
	CacheTargetPercent  float64                 `json:"cacheTargetPercent"`
	Git                 GitSettings             `json:"git"`
	Browser             BrowserSettings         `json:"browser"`
	ComputerControl     ComputerControlSettings `json:"computerControl"`
	MCP                 MCPSettings             `json:"mcp"`
	Model               ModelSettings           `json:"model"`
	Workspace           WorkspaceSettings       `json:"workspace"`
}

type GitSettings struct {
	BranchPrefix            string `json:"branchPrefix"`
	MergeMethod             string `json:"mergeMethod"`
	ShowPullRequestIcon     bool   `json:"showPullRequestIcon"`
	ForcePushWithLease      bool   `json:"forcePushWithLease"`
	DraftPullRequests       bool   `json:"draftPullRequests"`
	AutoDeleteOldWorktrees  bool   `json:"autoDeleteOldWorktrees"`
	WorktreeCleanupLimit    int    `json:"worktreeCleanupLimit"`
	CommitInstructions      string `json:"commitInstructions"`
	PullRequestInstructions string `json:"pullRequestInstructions"`
}

type BrowserSettings struct {
	Enabled                    bool                    `json:"enabled"`
	DefaultLocalURLDestination string                  `json:"defaultLocalUrlDestination"`
	ClearDataPolicy            string                  `json:"clearDataPolicy"`
	ScreenshotAnnotations      string                  `json:"screenshotAnnotations"`
	PasswordManagerEnabled     bool                    `json:"passwordManagerEnabled"`
	AutofillContactEnabled     bool                    `json:"autofillContactEnabled"`
	SitePermissions            []BrowserSitePermission `json:"sitePermissions"`
	DeveloperCDPAccess         bool                    `json:"developerCdpAccess"`
}

type BrowserSitePermission struct {
	Origin     string `json:"origin"`
	Camera     string `json:"camera"`
	Microphone string `json:"microphone"`
	Clipboard  string `json:"clipboard"`
}

type ComputerControlSettings struct {
	AnyAppEnabled     bool     `json:"anyAppEnabled"`
	ChromeEnabled     bool     `json:"chromeEnabled"`
	AlwaysAllowedApps []string `json:"alwaysAllowedApps"`
}

type MCPSettings struct {
	Servers []MCPServerSetting `json:"servers"`
}

type MCPServerSetting struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Command            string     `json:"command"`
	Args               []string   `json:"args"`
	Env                []KeyValue `json:"env"`
	PassEnvironment    []string   `json:"passEnvironment"`
	WorkingDirectory   string     `json:"workingDirectory"`
	Enabled            bool       `json:"enabled"`
	ToolResultPolicy   string     `json:"toolResultPolicy"`
	SchemaSnapshotHash string     `json:"schemaSnapshotHash,omitempty"`
	LastSnapshotAt     string     `json:"lastSnapshotAt,omitempty"`
}

type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ModelSettings struct {
	SelectedProviderID string                 `json:"selectedProviderId"`
	SelectedModelID    string                 `json:"selectedModelId"`
	Providers          []ModelProviderSetting `json:"providers"`
}

type ModelProviderSetting struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Protocol            string          `json:"protocol"`
	APIType             string          `json:"apiType"`
	BaseURL             string          `json:"baseUrl"`
	BalanceURL          string          `json:"balanceUrl"`
	ExtraHeaders        string          `json:"extraHeaders"`
	ExtraBodyJSON       string          `json:"extraBodyJson"`
	Enabled             bool            `json:"enabled"`
	APIKeyConfigured    bool            `json:"apiKeyConfigured"`
	DefaultModelID      string          `json:"defaultModelId"`
	ContextWindowTokens int             `json:"contextWindowTokens"`
	Models              []ProviderModel `json:"models"`
	LastSyncStatus      string          `json:"lastSyncStatus"`
	LastSyncMessage     string          `json:"lastSyncMessage"`
	CheckedAt           string          `json:"checkedAt,omitempty"`
	SupportsModelFetch  bool            `json:"supportsModelFetch"`
}

type ProviderModel struct {
	ID                  string `json:"id"`
	DisplayName         string `json:"displayName"`
	Provider            string `json:"provider"`
	ContextWindowTokens int    `json:"contextWindowTokens"`
}

type WorkspaceSettings struct {
	Configured          bool   `json:"configured"`
	DependenciesEnabled bool   `json:"dependenciesEnabled"`
	LastDiagnosticAt    string `json:"lastDiagnosticAt,omitempty"`
	LastDiagnosticNote  string `json:"lastDiagnosticNote,omitempty"`
}

func DefaultRuntimeSettings() RuntimeSettings {
	workspaceRoot, err := os.Getwd()
	if err != nil || strings.TrimSpace(workspaceRoot) == "" {
		workspaceRoot = "."
	}
	return RuntimeSettings{
		SandboxMode:         "workspace-write",
		FilesystemAccess:    "workspace-write",
		NetworkAccess:       true,
		ShellAccess:         true,
		ApprovalPolicy:      "on-request",
		WorkspaceRoot:       workspaceRoot,
		ExtraWritableRoots:  []string{},
		MaxCommandSeconds:   120,
		AllowDestructiveOps: false,
		ToolResultPolicy:    "summary-first",
		StablePrefixPolicy:  "strict-stable-prefix",
		CacheTargetPercent:  96,
		Git: GitSettings{
			BranchPrefix:           "mhcode/",
			MergeMethod:            "merge",
			ShowPullRequestIcon:    true,
			ForcePushWithLease:     false,
			DraftPullRequests:      true,
			AutoDeleteOldWorktrees: true,
			WorktreeCleanupLimit:   15,
		},
		Browser: BrowserSettings{
			Enabled:                    true,
			DefaultLocalURLDestination: "mhcode",
			ClearDataPolicy:            "ask",
			ScreenshotAnnotations:      "always",
			PasswordManagerEnabled:     false,
			AutofillContactEnabled:     false,
			SitePermissions:            []BrowserSitePermission{},
			DeveloperCDPAccess:         false,
		},
		ComputerControl: ComputerControlSettings{
			AnyAppEnabled:     false,
			ChromeEnabled:     false,
			AlwaysAllowedApps: []string{},
		},
		MCP: MCPSettings{
			Servers: []MCPServerSetting{
				{
					ID:               "filesystem",
					Name:             "filesystem",
					Command:          "builtin:filesystem",
					Args:             []string{},
					Env:              []KeyValue{},
					PassEnvironment:  []string{},
					Enabled:          true,
					ToolResultPolicy: "summary-first",
				},
			},
		},
		Model: ModelSettings{
			SelectedProviderID: "deepseek",
			SelectedModelID:    "",
			Providers: []ModelProviderSetting{
				{
					ID:                 "deepseek",
					Name:               "DeepSeek 官方",
					Protocol:           "deepseek-official",
					APIType:            "chat-completions",
					BaseURL:            "https://api.deepseek.com",
					Enabled:            true,
					LastSyncStatus:     "idle",
					LastSyncMessage:    "等待保存 API Key 后刷新模型。",
					SupportsModelFetch: true,
				},
				{
					ID:                 "openai-compatible",
					Name:               "OpenAI 兼容",
					Protocol:           "openai-compatible",
					APIType:            "chat-completions",
					BaseURL:            "https://api.openai.com/v1",
					Enabled:            false,
					LastSyncStatus:     "idle",
					LastSyncMessage:    "填写 Base URL 与 API Key 后可自动获取模型。",
					SupportsModelFetch: true,
				},
				{
					ID:                 "local-openai",
					Name:               "本地 OpenAI 兼容",
					Protocol:           "openai-compatible",
					APIType:            "chat-completions",
					BaseURL:            "http://127.0.0.1:11434/v1",
					Enabled:            false,
					LastSyncStatus:     "idle",
					LastSyncMessage:    "适用于 Ollama、LM Studio 等本地兼容服务。",
					SupportsModelFetch: true,
				},
			},
		},
		Workspace: WorkspaceSettings{
			Configured:          true,
			DependenciesEnabled: true,
		},
	}
}

func (settings RuntimeSettings) Normalized() RuntimeSettings {
	defaults := DefaultRuntimeSettings()

	settings.SandboxMode = normalizeChoice(settings.SandboxMode, defaults.SandboxMode, map[string]bool{
		"read-only":          true,
		"workspace-write":    true,
		"danger-full-access": true,
	})
	settings.FilesystemAccess = normalizeChoice(settings.FilesystemAccess, defaults.FilesystemAccess, map[string]bool{
		"read-only":       true,
		"workspace-write": true,
		"unrestricted":    true,
	})
	settings.ApprovalPolicy = normalizeChoice(settings.ApprovalPolicy, defaults.ApprovalPolicy, map[string]bool{
		"never":      true,
		"on-request": true,
		"on-failure": true,
		"untrusted":  true,
	})
	settings.ToolResultPolicy = normalizeChoice(settings.ToolResultPolicy, defaults.ToolResultPolicy, map[string]bool{
		"summary-first": true,
		"balanced":      true,
		"raw-local":     true,
	})
	settings.StablePrefixPolicy = normalizeChoice(settings.StablePrefixPolicy, defaults.StablePrefixPolicy, map[string]bool{
		"reuse-prefix":         true,
		"stable-prefix":        true,
		"strict-stable-prefix": true,
	})

	settings.WorkspaceRoot = strings.TrimSpace(settings.WorkspaceRoot)
	if settings.WorkspaceRoot == "" {
		settings.WorkspaceRoot = defaults.WorkspaceRoot
	}
	settings.ExtraWritableRoots = cleanStringList(settings.ExtraWritableRoots)
	if settings.MaxCommandSeconds < 5 {
		settings.MaxCommandSeconds = 5
	}
	if settings.MaxCommandSeconds > 3600 {
		settings.MaxCommandSeconds = 3600
	}
	if settings.CacheTargetPercent < 0 {
		settings.CacheTargetPercent = 0
	}
	if settings.CacheTargetPercent > 100 {
		settings.CacheTargetPercent = 100
	}
	settings.Git = normalizeGitSettings(settings.Git, defaults.Git)
	settings.Browser = normalizeBrowserSettings(settings.Browser, defaults.Browser)
	settings.ComputerControl = normalizeComputerControlSettings(settings.ComputerControl, defaults.ComputerControl)
	settings.MCP = normalizeMCPSettings(settings.MCP, defaults.MCP)
	settings.Model = normalizeModelSettings(settings.Model, defaults.Model)
	settings.Workspace = normalizeWorkspaceSettings(settings.Workspace, defaults.Workspace)
	return settings
}

func (settings RuntimeSettings) Validate() error {
	settings = settings.Normalized()
	if settings.SandboxMode == "read-only" && settings.FilesystemAccess != "read-only" {
		return errors.New("只读沙箱下文件权限必须为只读")
	}
	if settings.SandboxMode == "danger-full-access" && settings.ApprovalPolicy == "never" && settings.AllowDestructiveOps {
		return errors.New("危险全权限 + 永不审批时不能开启破坏性操作")
	}
	return nil
}

func loadRuntimeSettings(path string) RuntimeSettings {
	settings := DefaultRuntimeSettings()
	if strings.TrimSpace(path) == "" {
		return settings
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return settings
	}
	var stored RuntimeSettings
	if err := json.Unmarshal(data, &stored); err != nil {
		return settings
	}
	return stored.Normalized()
}

func saveRuntimeSettings(path string, settings RuntimeSettings) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings.Normalized(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func normalizeChoice(value string, fallback string, allowed map[string]bool) string {
	value = strings.TrimSpace(value)
	if allowed[value] {
		return value
	}
	return fallback
}

func cleanStringList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func normalizeGitSettings(settings GitSettings, defaults GitSettings) GitSettings {
	if settings == (GitSettings{}) {
		return defaults
	}
	settings.BranchPrefix = strings.TrimSpace(settings.BranchPrefix)
	if settings.BranchPrefix == "" {
		settings.BranchPrefix = defaults.BranchPrefix
	}
	settings.MergeMethod = normalizeChoice(settings.MergeMethod, defaults.MergeMethod, map[string]bool{
		"merge":  true,
		"squash": true,
		"rebase": true,
	})
	if settings.WorktreeCleanupLimit < 1 {
		settings.WorktreeCleanupLimit = defaults.WorktreeCleanupLimit
	}
	if settings.WorktreeCleanupLimit > 99 {
		settings.WorktreeCleanupLimit = 99
	}
	settings.CommitInstructions = strings.TrimSpace(settings.CommitInstructions)
	settings.PullRequestInstructions = strings.TrimSpace(settings.PullRequestInstructions)
	return settings
}

func normalizeBrowserSettings(settings BrowserSettings, defaults BrowserSettings) BrowserSettings {
	if isZeroBrowserSettings(settings) {
		return defaults
	}
	settings.DefaultLocalURLDestination = normalizeChoice(settings.DefaultLocalURLDestination, defaults.DefaultLocalURLDestination, map[string]bool{
		"mhcode":  true,
		"system":  true,
		"ask":     true,
		"browser": true,
	})
	settings.ClearDataPolicy = normalizeChoice(settings.ClearDataPolicy, defaults.ClearDataPolicy, map[string]bool{
		"ask":     true,
		"session": true,
		"all":     true,
		"never":   true,
	})
	settings.ScreenshotAnnotations = normalizeChoice(settings.ScreenshotAnnotations, defaults.ScreenshotAnnotations, map[string]bool{
		"always": true,
		"ask":    true,
		"never":  true,
	})
	cleaned := make([]BrowserSitePermission, 0, len(settings.SitePermissions))
	seen := map[string]bool{}
	for _, permission := range settings.SitePermissions {
		permission.Origin = strings.TrimSpace(permission.Origin)
		if permission.Origin == "" || seen[permission.Origin] {
			continue
		}
		seen[permission.Origin] = true
		permission.Camera = normalizeChoice(permission.Camera, "ask", permissionChoices())
		permission.Microphone = normalizeChoice(permission.Microphone, "ask", permissionChoices())
		permission.Clipboard = normalizeChoice(permission.Clipboard, "ask", permissionChoices())
		cleaned = append(cleaned, permission)
	}
	settings.SitePermissions = cleaned
	return settings
}

func permissionChoices() map[string]bool {
	return map[string]bool{
		"ask":   true,
		"allow": true,
		"block": true,
	}
}

func normalizeComputerControlSettings(settings ComputerControlSettings, defaults ComputerControlSettings) ComputerControlSettings {
	settings.AlwaysAllowedApps = cleanStringList(settings.AlwaysAllowedApps)
	if settings.AlwaysAllowedApps == nil {
		settings.AlwaysAllowedApps = defaults.AlwaysAllowedApps
	}
	return settings
}

func normalizeMCPSettings(settings MCPSettings, defaults MCPSettings) MCPSettings {
	if len(settings.Servers) == 0 {
		settings.Servers = defaults.Servers
	}
	cleaned := make([]MCPServerSetting, 0, len(settings.Servers))
	seen := map[string]bool{}
	for _, server := range settings.Servers {
		server.ID = stableID(server.ID, server.Name, server.Command)
		if server.ID == "" || seen[server.ID] {
			continue
		}
		seen[server.ID] = true
		server.Name = strings.TrimSpace(server.Name)
		if server.Name == "" {
			server.Name = server.ID
		}
		server.Command = strings.TrimSpace(server.Command)
		server.WorkingDirectory = strings.TrimSpace(server.WorkingDirectory)
		server.Args = cleanStringList(server.Args)
		server.PassEnvironment = cleanStringList(server.PassEnvironment)
		server.Env = cleanKeyValues(server.Env)
		server.ToolResultPolicy = normalizeChoice(server.ToolResultPolicy, defaults.Servers[0].ToolResultPolicy, map[string]bool{
			"summary-first": true,
			"balanced":      true,
			"raw-local":     true,
		})
		cleaned = append(cleaned, server)
	}
	settings.Servers = cleaned
	return settings
}

func normalizeModelSettings(settings ModelSettings, defaults ModelSettings) ModelSettings {
	settings.SelectedProviderID = strings.TrimSpace(settings.SelectedProviderID)
	settings.SelectedModelID = strings.TrimSpace(settings.SelectedModelID)
	if len(settings.Providers) == 0 {
		settings.Providers = defaults.Providers
	}
	cleaned := make([]ModelProviderSetting, 0, len(settings.Providers))
	seen := map[string]bool{}
	for _, provider := range settings.Providers {
		provider.ID = stableID(provider.ID, provider.Name, provider.Protocol)
		if provider.ID == "" || seen[provider.ID] {
			continue
		}
		seen[provider.ID] = true
		provider.Name = strings.TrimSpace(provider.Name)
		if provider.Name == "" {
			provider.Name = provider.ID
		}
		provider.Protocol = normalizeChoice(provider.Protocol, "openai-compatible", map[string]bool{
			"deepseek-official":    true,
			"openai-compatible":    true,
			"anthropic":            true,
			"anthropic-compatible": true,
			"gemini":               true,
			"local":                true,
		})
		provider.APIType = normalizeAPIType(provider.APIType, provider.Protocol)
		provider.BaseURL = strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
		if provider.BaseURL == "" {
			provider.BaseURL = defaultBaseURLForProtocol(provider.Protocol)
		}
		provider.BalanceURL = strings.TrimSpace(provider.BalanceURL)
		provider.ExtraHeaders = strings.TrimSpace(provider.ExtraHeaders)
		provider.ExtraBodyJSON = strings.TrimSpace(provider.ExtraBodyJSON)
		provider.DefaultModelID = strings.TrimSpace(provider.DefaultModelID)
		if provider.ContextWindowTokens < 0 {
			provider.ContextWindowTokens = 0
		}
		provider.LastSyncStatus = normalizeChoice(provider.LastSyncStatus, "idle", map[string]bool{
			"idle":  true,
			"ok":    true,
			"error": true,
		})
		provider.LastSyncMessage = strings.TrimSpace(provider.LastSyncMessage)
		provider.Models = normalizeProviderModels(provider.Models, provider.ID)
		provider.SupportsModelFetch = supportsModelFetch(provider.Protocol)
		cleaned = append(cleaned, provider)
	}
	settings.Providers = ensureDefaultProviders(cleaned, defaults.Providers)
	if settings.SelectedProviderID == "" || !hasProvider(settings.Providers, settings.SelectedProviderID) {
		settings.SelectedProviderID = settings.Providers[0].ID
	}
	return settings
}

func normalizeWorkspaceSettings(settings WorkspaceSettings, defaults WorkspaceSettings) WorkspaceSettings {
	if !settings.Configured {
		return defaults
	}
	settings.Configured = true
	settings.LastDiagnosticAt = strings.TrimSpace(settings.LastDiagnosticAt)
	settings.LastDiagnosticNote = strings.TrimSpace(settings.LastDiagnosticNote)
	return settings
}

func cleanKeyValues(values []KeyValue) []KeyValue {
	cleaned := make([]KeyValue, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value.Key = strings.TrimSpace(value.Key)
		if value.Key == "" || seen[value.Key] {
			continue
		}
		seen[value.Key] = true
		value.Value = strings.TrimSpace(value.Value)
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func stableID(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultBaseURLForProtocol(protocol string) string {
	switch protocol {
	case "deepseek-official":
		return "https://api.deepseek.com"
	case "openai-compatible":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com"
	case "anthropic-compatible":
		return "https://api.anthropic.com"
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta"
	case "local":
		return "http://127.0.0.1:11434/v1"
	default:
		return ""
	}
}

func normalizeAPIType(apiType string, protocol string) string {
	return normalizeChoice(apiType, defaultAPITypeForProtocol(protocol), map[string]bool{
		"chat-completions":        true,
		"responses":               true,
		"anthropic-messages":      true,
		"gemini-generate-content": true,
	})
}

func defaultAPITypeForProtocol(protocol string) string {
	switch protocol {
	case "anthropic", "anthropic-compatible":
		return "anthropic-messages"
	case "gemini":
		return "gemini-generate-content"
	default:
		return "chat-completions"
	}
}

func supportsModelFetch(protocol string) bool {
	return protocol == "deepseek-official" ||
		protocol == "openai-compatible" ||
		protocol == "anthropic" ||
		protocol == "anthropic-compatible" ||
		protocol == "gemini" ||
		protocol == "local"
}

func normalizeProviderModels(models []ProviderModel, providerID string) []ProviderModel {
	cleaned := make([]ProviderModel, 0, len(models))
	seen := map[string]bool{}
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" || seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		model.DisplayName = strings.TrimSpace(model.DisplayName)
		if model.DisplayName == "" {
			model.DisplayName = model.ID
		}
		model.Provider = strings.TrimSpace(model.Provider)
		if model.Provider == "" {
			model.Provider = providerID
		}
		if model.ContextWindowTokens < 0 {
			model.ContextWindowTokens = 0
		}
		cleaned = append(cleaned, model)
	}
	return cleaned
}

func ensureDefaultProviders(providers []ModelProviderSetting, defaults []ModelProviderSetting) []ModelProviderSetting {
	seen := map[string]bool{}
	for _, provider := range providers {
		seen[provider.ID] = true
	}
	for _, provider := range defaults {
		if seen[provider.ID] {
			continue
		}
		providers = append(providers, provider)
	}
	return providers
}

func hasProvider(providers []ModelProviderSetting, id string) bool {
	for _, provider := range providers {
		if provider.ID == id {
			return true
		}
	}
	return false
}

func isZeroBrowserSettings(settings BrowserSettings) bool {
	return !settings.Enabled &&
		settings.DefaultLocalURLDestination == "" &&
		settings.ClearDataPolicy == "" &&
		settings.ScreenshotAnnotations == "" &&
		!settings.PasswordManagerEnabled &&
		!settings.AutofillContactEnabled &&
		len(settings.SitePermissions) == 0 &&
		!settings.DeveloperCDPAccess
}
