export type ReasoningLevel = "low" | "medium" | "high" | "ultra";

export type ReasoningBudget = {
  maxToolCalls: number;
  contextPolicy: string;
  cachePolicy: string;
  planner: boolean;
};

export type ReasoningOption = {
  id: ReasoningLevel;
  label: string;
  description: string;
  budget: ReasoningBudget;
};

export type SkillIndexEntry = {
  name: string;
  version: number;
  trigger: string;
  summary: string;
  sha256: string;
  description: string;
};

export type ToolDescriptor = {
  name: string;
  inputSchemaHash: string;
  outputPolicy: string;
};

export type ServerSnapshot = {
  server: string;
  toolsHash: string;
  tools: ToolDescriptor[];
};

export type Model = {
  id: string;
  displayName: string;
  provider: string;
  contextWindowTokens: number;
  contextWindowSource?: string;
};

export type DeepSeekState = {
  configured: boolean;
  baseUrl: string;
  lastCheckStatus: "idle" | "ok" | "error" | string;
  lastCheckMessage: string;
  checkedAt?: string;
  models: Model[];
};

export type DeepSeekSessionState = {
  active: boolean;
  providerId: string;
  providerName: string;
  protocol: string;
  model: string;
  reasoning: ReasoningLevel | "";
  thinkingMode: string;
  reasoningEffort?: string;
  prefixHash: string;
  systemPromptHash: string;
  stablePromptTokens: number;
  messageCount: number;
  turnCount: number;
  startedAt?: string;
  resetReason: string;
  sessionCacheHitTokens: number;
  sessionCacheMissTokens: number;
  sessionCacheHitRate: number;
  appendOnlyPrefixStable: boolean;
  previousRequestMessageCount: number;
  commonPrefixMessageCount: number;
  contextWindowTokens?: number;
  contextWindowSource?: string;
  estimatedInputTokens?: number;
  inputBudgetTokens?: number;
  contextUsagePercent?: number;
  compressionCount?: number;
  compressedMessageCount?: number;
  lastCompressedAt?: string;
};

export type ContextSection = {
  name: string;
  content: string;
};

export type RequestContext = {
  stablePrefix: ContextSection[];
  volatileTail: ContextSection[];
  prefixHash: string;
};

export type UsageMetrics = {
  promptCacheHitTokens: number;
  promptCacheMissTokens: number;
  inputTokens: number;
  outputTokens: number;
  effectiveCost: number;
};

export type UsageLedgerState = {
  enabled: boolean;
  path?: string;
  sessionSamples: number;
  totalSamples: number;
  sessionInputTokens: number;
  sessionOutputTokens: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  sessionEffectiveCost: number;
  totalEffectiveCost: number;
  lastRecordedAt?: string;
  lastError?: string;
};

export type CacheHealth = {
  status: "pending" | "ok" | "watch" | "warming" | "cold" | "low" | string;
  message: string;
  hitRate: number;
  targetHitRate: number;
  hitTokens: number;
  missTokens: number;
  totalCacheTokens: number;
  missTokenBudget: number;
  requiredHitTokens: number;
  additionalHitTokensNeeded: number;
  shortPrompt: boolean;
  sampleCount: number;
  consecutiveBelowTarget: number;
  hitTokensIncreasing: boolean;
  missTokensStable: boolean;
  missTokensImproving: boolean;
};

export type RuntimeSettings = {
  sandboxMode: "read-only" | "workspace-write" | "danger-full-access" | string;
  filesystemAccess: "read-only" | "workspace-write" | "unrestricted" | string;
  networkAccess: boolean;
  shellAccess: boolean;
  approvalPolicy: "never" | "on-request" | "on-failure" | "untrusted" | string;
  workspaceRoot: string;
  extraWritableRoots: string[];
  maxCommandSeconds: number;
  maxCommandMemoryMb: number;
  maxCommandCpuPercent: number;
  maxCommandProcesses: number;
  allowDestructiveOps: boolean;
  toolResultPolicy: "summary-first" | "balanced" | "raw-local" | string;
  stablePrefixPolicy: "reuse-prefix" | "stable-prefix" | "strict-stable-prefix" | string;
  cacheTargetPercent: number;
  git: GitSettings;
  browser: BrowserSettings;
  computerControl: ComputerControlSettings;
  mcp: MCPSettings;
  model: ModelSettings;
  team: TeamSettings;
  workspace: WorkspaceSettings;
  memory: MemorySettings;
};

export type SandboxCapabilities = {
  platform: string;
  backend: string;
  processTree: boolean;
  resourceLimits: boolean;
  privilegeIsolation: boolean;
  filesystemIsolation: boolean;
  networkIsolation: boolean;
  summary: string;
};

export type GitSettings = {
  branchPrefix: string;
  mergeMethod: "merge" | "squash" | "rebase" | string;
  showPullRequestIcon: boolean;
  forcePushWithLease: boolean;
  draftPullRequests: boolean;
  autoDeleteOldWorktrees: boolean;
  worktreeCleanupLimit: number;
  commitInstructions: string;
  pullRequestInstructions: string;
};

export type MemorySettings = {
  enabled: boolean;
  maxSessions: number;
  maxCharacters: number;
  includeArchived: boolean;
};

export type GitFileStatus = {
  path: string;
  originalPath?: string;
  indexStatus: string;
  worktreeStatus: string;
  staged: boolean;
  modified: boolean;
  untracked: boolean;
  conflicted: boolean;
};

export type GitBranchState = {
  name: string;
  upstream?: string;
  current: boolean;
};

export type GitStatus = {
  available: boolean;
  repositoryRoot?: string;
  branch?: string;
  upstream?: string;
  commit?: string;
  ahead: number;
  behind: number;
  clean: boolean;
  detached: boolean;
  stagedCount: number;
  modifiedCount: number;
  untrackedCount: number;
  conflictCount: number;
  files: GitFileStatus[];
  branches: GitBranchState[];
};

export type GitDiff = {
  path?: string;
  staged: boolean;
  patch: string;
  truncated: boolean;
};

export type TerminalSessionState = {
  id: string;
  shell: string;
  workdir: string;
  running: boolean;
  startedAt: string;
  exitCode: number;
  error?: string;
  output: string;
  sandboxed: boolean;
  sandboxBackend?: string;
  privilegeRestricted: boolean;
};

export type BrowserSettings = {
  enabled: boolean;
  defaultLocalUrlDestination: "mhcode" | "system" | "ask" | "browser" | string;
  clearDataPolicy: "ask" | "session" | "all" | "never" | string;
  screenshotAnnotations: "always" | "ask" | "never" | string;
  passwordManagerEnabled: boolean;
  autofillContactEnabled: boolean;
  autofillProfile: BrowserAutofillProfile;
  credentials: BrowserCredential[];
  sitePermissions: BrowserSitePermission[];
  developerCdpAccess: boolean;
};

export type BrowserCredential = {
  id: string;
  origin: string;
  username: string;
  passwordConfigured: boolean;
};

export type BrowserAutofillProfile = {
  fullName: string;
  email: string;
  phone: string;
  organization: string;
  streetAddress: string;
  city: string;
  region: string;
  postalCode: string;
  country: string;
};

export type BrowserSitePermission = {
  origin: string;
  camera: "ask" | "allow" | "block" | string;
  microphone: "ask" | "allow" | "block" | string;
  clipboard: "ask" | "allow" | "block" | string;
};

export type ComputerControlSettings = {
  anyAppEnabled: boolean;
  chromeEnabled: boolean;
  alwaysAllowedApps: string[];
};

export type MCPSettings = {
  servers: MCPServerSetting[];
};

export type MCPServerSetting = {
  id: string;
  name: string;
  transport: "builtin" | "stdio" | "streamable-http" | "sse" | string;
  command: string;
  args: string[];
  env: KeyValue[];
  passEnvironment: string[];
  workingDirectory: string;
  url: string;
  headers: KeyValue[];
  enabled: boolean;
  toolResultPolicy: "summary-first" | "balanced" | "raw-local" | string;
  schemaSnapshotHash?: string;
  lastSnapshotAt?: string;
};

export type MCPServerStatus = {
  id: string;
  name: string;
  transport: string;
  state: "idle" | "disabled" | "ready" | "error" | string;
  message: string;
  toolCount: number;
  protocolVersion?: string;
  serverVersion?: string;
  checkedAt?: string;
};

export type KeyValue = {
  key: string;
  value: string;
};

export type ModelSettings = {
  selectedProviderId: string;
  selectedModelId: string;
  providers: ModelProviderSetting[];
};

export type TeamSettings = {
  enabled: boolean;
  maxReviewRounds: number;
  roles: TeamRoleSetting[];
};

export type TeamRole = "planner" | "implementer" | "tester" | "reviewer" | "synthesizer";

export type TeamRoleSetting = {
  role: TeamRole;
  enabled: boolean;
  providerId: string;
  modelId: string;
};

export type ModelProviderSetting = {
  id: string;
  name: string;
  protocol: "deepseek-official" | "openai-compatible" | "anthropic" | "anthropic-compatible" | "gemini" | "local" | string;
  apiType: "chat-completions" | "responses" | "anthropic-messages" | "gemini-generate-content" | string;
  baseUrl: string;
  balanceUrl: string;
  extraHeaders: string;
  extraBodyJson: string;
  enabled: boolean;
  apiKeyConfigured: boolean;
  defaultModelId: string;
  contextWindowTokens: number;
  inputPricePerMillion?: number;
  outputPricePerMillion?: number;
  cacheHitPricePerMillion?: number;
  cacheMissPricePerMillion?: number;
  models: Model[];
  lastSyncStatus: "idle" | "ok" | "error" | string;
  lastSyncMessage: string;
  checkedAt?: string;
  supportsModelFetch: boolean;
};

export type WorkspaceSettings = {
  configured: boolean;
  dependenciesEnabled: boolean;
  lastDiagnosticAt?: string;
  lastDiagnosticNote?: string;
};

export type WorkbenchState = {
  reasoning: ReasoningOption;
  reasoningOptions: ReasoningOption[];
  cacheTarget: number;
  usageMetrics: UsageMetrics;
  cacheHitRate: number;
  cacheHealth: CacheHealth;
  deepSeek: DeepSeekState;
  deepSeekSession: DeepSeekSessionState;
  skillsIndex: SkillIndexEntry[];
  mcpSnapshots: ServerSnapshot[];
  mcpServers: MCPServerStatus[];
  contextPreview: RequestContext;
  cacheDiagnostics: string[];
  runtimeSettings: RuntimeSettings;
  sandboxCapabilities: SandboxCapabilities;
  configFiles: ConfigFilesState;
  planMode?: boolean;
  planState?: PlanState;
  team: TeamState;
  projectMemory: ProjectMemoryState;
  usageLedger?: UsageLedgerState;
};

export type PlanState = {
  revision: number;
  status: "running" | "completed" | string;
  steps: Array<{ title: string; status: "pending" | "in_progress" | "completed" | string }>;
  updatedAt?: string;
};

export type TeamState = {
  enabled: boolean;
  active: boolean;
  runId?: string;
  status: "idle" | "running" | "completed" | "failed" | "cancelled" | string;
  currentRole?: TeamRole | string;
  roles: TeamRoleState[];
  startedAt?: string;
  completedAt?: string;
  summary?: string;
};

export type TeamRoleState = {
  role: TeamRole;
  label: string;
  enabled: boolean;
  status: "pending" | "running" | "completed" | "error" | "skipped" | string;
  providerId?: string;
  model?: string;
  attempt: number;
  verdict?: "approved" | "changes_required" | "unknown" | string;
  summary?: string;
  error?: string;
  usage: UsageMetrics;
  startedAt?: string;
  finishedAt?: string;
};

export type ProjectMemoryState = {
  enabled: boolean;
  projectId?: string;
  projectName?: string;
  sessionCount: number;
  turnCount: number;
  updatedAt?: string;
  snapshotHash?: string;
  summary: string;
};

export type ConfigFilesState = {
  runtimeSettingsPath: string;
  modelProvidersPath: string;
  secretsStore: string;
};

export type ChatResult = {
  content: string;
  reasoning?: string;
  model: string;
  usage: UsageMetrics;
  state: WorkbenchState;
  parts?: MessagePart[];
};

export type ChatTaskState = {
  taskId: string;
  startedAt: string;
};

export type ChatAttachment = {
  name: string;
  mimeType: string;
  data: string;
};

export type ChatTaskEvent = {
  taskId: string;
  type: "started" | "status" | "context_compression" | "delta" | "reasoning" | "usage" | "tool" | "completed" | "failed" | "cancelled" | string;
  delta?: string;
  message?: string;
  model?: string;
  toolName?: string;
  toolCallId?: string;
  toolInput?: string;
  status?: "running" | "completed" | "error" | string;
  usage?: {
    promptTokens: number;
    completionTokens: number;
    totalTokens: number;
    promptCacheHitTokens: number;
    promptCacheMissTokens: number;
  };
  progress?: Extract<MessagePart, { kind: "task_progress" }>;
  parts?: MessagePart[];
  compression?: {
    status: "running" | "completed" | "error" | string;
    beforeTokens: number;
    afterTokens?: number;
    removedMessages?: number;
    targetTokens: number;
  };
  team?: {
    runId: string;
    role: TeamRole;
    label: string;
    status: string;
    providerId?: string;
    model?: string;
    attempt: number;
    verdict?: string;
    summary?: string;
    error?: string;
    usage: UsageMetrics;
  };
  guidanceId?: string;
  guidance?: string;
  attachments?: ChatAttachment[];
  result?: ChatResult;
};

export type BrowserPreview = {
  path: string;
  name: string;
  url: string;
  ask?: boolean;
  tabId?: string;
  managed?: boolean;
};

export type BrowserTab = {
  id: string;
  title: string;
  url: string;
  loading: boolean;
  canGoBack: boolean;
  canGoForward: boolean;
  error?: string;
  viewportWidth: number;
  viewportHeight: number;
  dialog?: BrowserDialog;
};

export type BrowserDialog = {
  type: string;
  message: string;
  defaultValue?: string;
};

export type BrowserDownload = {
  id: string;
  url: string;
  filename: string;
  path?: string;
  state: string;
  receivedBytes: number;
  totalBytes: number;
  startedAt: string;
};

export type BrowserState = {
  available: boolean;
  running: boolean;
  engine: string;
  renderMode: "native" | "stream" | string;
  activeTabId: string;
  tabs: BrowserTab[];
  downloads: BrowserDownload[];
  lastError?: string;
  cdpEnabled: boolean;
};

export type BrowserElement = {
  index: number;
  selector: string;
  tag: string;
  role?: string;
  name?: string;
  text?: string;
  type?: string;
  placeholder?: string;
  href?: string;
  x: number;
  y: number;
  width: number;
  height: number;
};

export type BrowserFrame = {
  tab: BrowserTab;
  imageDataUrl: string;
  width: number;
  height: number;
  elements?: BrowserElement[];
  capturedAt: string;
};

export type BrowserSnapshot = {
  title: string;
  url: string;
  text: string;
  elements: BrowserElement[];
  capturedAt: string;
};

export type BrowserConsoleEntry = {
  level: string;
  message: string;
  timestamp: string;
};

export type BrowserNetworkEntry = {
  method: string;
  url: string;
  status: number;
  type: string;
  failed: boolean;
  error?: string;
  timestamp: string;
};

export type BrowserInspector = {
  snapshot: BrowserSnapshot;
  console: BrowserConsoleEntry[];
  network: BrowserNetworkEntry[];
};

// 前向兼容的消息片段模型：当前后端仅产出纯文本（走 text 片段），
// 但渲染管线已按类型分派，后续接入工具执行 / 文件编辑时可直接产出
// tool_call / diff 片段，无需改动前端渲染架构。
export type MessagePart =
  | { kind: "text"; text: string }
  | { kind: "diff"; path: string; patch: string; additions?: number; deletions?: number }
  | { kind: "file"; path: string; lineCount?: number; created?: boolean; fileAction?: "created" | "modified" | "available" | string }
  | { kind: "tool_call"; name: string; status?: "running" | "ok" | "error"; input?: string; output?: string; toolCallId?: string }
  | {
      kind: "task_progress";
      steps: Array<{ title: string; status: "pending" | "in_progress" | "completed" | string }>;
      taskStatus?: "running" | "completed" | "failed" | "cancelled" | string;
      changedFiles?: number;
      additions?: number;
      deletions?: number;
    }
  | {
      kind: "web_search_results";
      query: string;
      sources: Array<{ title: string; url: string; snippet?: string }>;
    }
  | {
      kind: "team_role";
      role: TeamRole;
      roleLabel: string;
      providerId?: string;
      model?: string;
      status?: "running" | "completed" | "error" | string;
      summary?: string;
      verdict?: "approved" | "changes_required" | "unknown" | string;
      attempt?: number;
    };

// 事件日志的可回退检查点（对应后端 agent.CheckpointInfo）。
export type CheckpointInfo = {
  id: string;
  label: string;
  turnIndex: number;
  timestamp: string;
  preview: string;
};

// 对话线（分支）摘要（对应后端 agent.BranchInfo）。
export type BranchInfo = {
  leafId: string;
  label: string;
  turnCount: number;
  timestamp: string;
  isCurrent: boolean;
};

// 待审批请求（对应后端 agent.ApprovalRequest），用于代码审核弹窗。
export type ApprovalRequest = {
  id: string;
  tool: string;
  kind: "file" | "command" | string;
  path?: string;
  command?: string;
  url?: string;
  summary: string;
  parts?: MessagePart[];
};

// 项目摘要（对应后端 agent.ProjectInfo）。
export type ProjectInfo = {
  id: string;
  name: string;
  workspaceRoot: string;
  pinned: boolean;
  isActive: boolean;
  sessionCount: number;
};

// 会话摘要（对应后端 agent.SessionInfo）。
export type SessionInfo = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  archived: boolean;
  isActive: boolean;
};

// 会话历史消息（对应后端 agent.SessionMessage），用于恢复对话。
export type SessionMessage = {
  id: string;
  role: "user" | "assistant" | "system" | string;
  content: string;
  model?: string;
  createdAt: string;
  parts?: MessagePart[];
  attachments?: ChatAttachment[];
};

// 侧边栏树的项目节点（对应后端 agent.ProjectNode），含它的会话。
export type ProjectNode = {
  id: string;
  name: string;
  workspaceRoot: string;
  pinned: boolean;
  isActive: boolean;
  sessions: SessionInfo[];
};
