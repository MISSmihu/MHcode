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
  allowDestructiveOps: boolean;
  toolResultPolicy: "summary-first" | "balanced" | "raw-local" | string;
  stablePrefixPolicy: "reuse-prefix" | "stable-prefix" | "strict-stable-prefix" | string;
  cacheTargetPercent: number;
  git: GitSettings;
  browser: BrowserSettings;
  computerControl: ComputerControlSettings;
  mcp: MCPSettings;
  model: ModelSettings;
  workspace: WorkspaceSettings;
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

export type BrowserSettings = {
  enabled: boolean;
  defaultLocalUrlDestination: "mhcode" | "system" | "ask" | "browser" | string;
  clearDataPolicy: "ask" | "session" | "all" | "never" | string;
  screenshotAnnotations: "always" | "ask" | "never" | string;
  passwordManagerEnabled: boolean;
  autofillContactEnabled: boolean;
  sitePermissions: BrowserSitePermission[];
  developerCdpAccess: boolean;
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
  command: string;
  args: string[];
  env: KeyValue[];
  passEnvironment: string[];
  workingDirectory: string;
  enabled: boolean;
  toolResultPolicy: "summary-first" | "balanced" | "raw-local" | string;
  schemaSnapshotHash?: string;
  lastSnapshotAt?: string;
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
  contextPreview: RequestContext;
  cacheDiagnostics: string[];
  runtimeSettings: RuntimeSettings;
  configFiles: ConfigFilesState;
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
};
