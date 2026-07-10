import { defaultReasoningLevel, reasoningOptions } from "../state/reasoning";
import type { ChatResult, ReasoningLevel, RuntimeSettings, ServerSnapshot, SkillIndexEntry, WorkbenchState } from "../types";

type WailsAppBinding = {
  GetWorkbenchState: () => Promise<WorkbenchState>;
  SetReasoningLevel: (level: ReasoningLevel) => Promise<WorkbenchState>;
  SaveDeepSeekAPIKey: (apiKey: string) => Promise<WorkbenchState>;
  ClearDeepSeekAPIKey: () => Promise<WorkbenchState>;
  TestDeepSeekConnection: () => Promise<WorkbenchState>;
  SendDeepSeekMessage: (prompt: string) => Promise<ChatResult>;
  ResetDeepSeekSession: () => Promise<WorkbenchState>;
  SaveRuntimeSettings: (settings: RuntimeSettings) => Promise<WorkbenchState>;
  SaveModelProviderAPIKey: (providerID: string, apiKey: string) => Promise<WorkbenchState>;
  ClearModelProviderAPIKey: (providerID: string) => Promise<WorkbenchState>;
  RefreshModelProviderModels: (providerID: string) => Promise<WorkbenchState>;
};

type WailsWindow = Window & {
  go?: {
    main?: {
      App?: WailsAppBinding;
    };
  };
};

const fallbackSkillsIndex: SkillIndexEntry[] = [
  {
    name: "mhcode-agent-core",
    version: 1,
    trigger: "推理强度、缓存命中、MCP、工具调用、tokens 成本",
    summary: "统一管理推理强度、Skills、MCP、工具调用、缓存命中和成本控制",
    sha256: "sha256:local-preview",
    description: "普通浏览器预览模式下的本地工作台状态。",
  },
];

const fallbackSnapshots: ServerSnapshot[] = [
  {
    server: "filesystem",
    toolsHash: "sha256:local-preview",
    tools: [
      {
        name: "read_file",
        inputSchemaHash: "sha256:path-schema",
        outputPolicy: "summary-first",
      },
      {
        name: "list_directory",
        inputSchemaHash: "sha256:path-schema",
        outputPolicy: "summary-first",
      },
    ],
  },
];

let fallbackState = createFallbackState(defaultReasoningLevel);

export async function getWorkbenchState(): Promise<WorkbenchState> {
  const binding = wailsBinding();
  if (binding) {
    return binding.GetWorkbenchState();
  }
  return cloneState(fallbackState);
}

export async function setReasoningLevel(level: ReasoningLevel): Promise<WorkbenchState> {
  const binding = wailsBinding();
  if (binding) {
    return binding.SetReasoningLevel(level);
  }
  fallbackState = {
    ...createFallbackState(level),
    deepSeek: fallbackState.deepSeek,
  };
  return cloneState(fallbackState);
}

export async function saveDeepSeekAPIKey(apiKey: string): Promise<WorkbenchState> {
  const binding = wailsBinding();
  if (binding) {
    return binding.SaveDeepSeekAPIKey(apiKey);
  }
  fallbackState = {
    ...fallbackState,
    deepSeek: {
      ...fallbackState.deepSeek,
      configured: true,
      lastCheckStatus: "idle",
      lastCheckMessage: "DeepSeek API Key 已保存，等待连接测试。",
      checkedAt: undefined,
      models: [],
    },
  };
  return cloneState(fallbackState);
}

export async function clearDeepSeekAPIKey(): Promise<WorkbenchState> {
  const binding = wailsBinding();
  if (binding) {
    return binding.ClearDeepSeekAPIKey();
  }
  fallbackState = {
    ...fallbackState,
    deepSeek: {
      configured: false,
      baseUrl: "https://api.deepseek.com",
      lastCheckStatus: "idle",
      lastCheckMessage: "DeepSeek API Key 已清除。",
      models: [],
    },
  };
  return cloneState(fallbackState);
}

export async function testDeepSeekConnection(): Promise<WorkbenchState> {
  const binding = wailsBinding();
  if (binding) {
    return binding.TestDeepSeekConnection();
  }
  fallbackState = {
    ...fallbackState,
    deepSeek: {
      ...fallbackState.deepSeek,
      configured: true,
      lastCheckStatus: "ok",
      lastCheckMessage: "预览模式连接模拟成功，发现 2 个模型。",
      checkedAt: new Date().toISOString(),
      models: [
        { id: "deepseek-v4-flash", displayName: "DeepSeek V4 Flash", provider: "deepseek", contextWindowTokens: 64000 },
        { id: "deepseek-v4-pro", displayName: "DeepSeek V4 Pro", provider: "deepseek", contextWindowTokens: 64000 },
      ],
    },
  };
  return cloneState(fallbackState);
}

export async function sendDeepSeekMessage(prompt: string): Promise<ChatResult> {
  const binding = wailsBinding();
  if (binding) {
    return binding.SendDeepSeekMessage(prompt);
  }
  const usage = {
    promptCacheHitTokens: 96,
    promptCacheMissTokens: 4,
    inputTokens: 100,
    outputTokens: 12,
    effectiveCost: 0,
  };
  fallbackState = {
    ...fallbackState,
    deepSeek: {
      ...fallbackState.deepSeek,
      configured: true,
      lastCheckStatus: "ok",
      lastCheckMessage: `预览模式试聊成功，${fallbackState.deepSeek.models[0]?.id ?? "deepseek-v4-flash"} 流式通道正常。`,
      checkedAt: new Date().toISOString(),
    },
    deepSeekSession: {
      active: true,
      model: fallbackState.deepSeek.models[0]?.id ?? "deepseek-v4-flash",
      reasoning: fallbackState.reasoning.id,
      thinkingMode: thinkingModeForReasoning(fallbackState.reasoning.id),
      reasoningEffort: reasoningEffortForReasoning(fallbackState.reasoning.id),
      prefixHash: fallbackState.contextPreview.prefixHash,
      systemPromptHash: "sha256:local-preview-system",
      stablePromptTokens: 900,
      messageCount: (fallbackState.deepSeekSession.messageCount || 1) + 2,
      turnCount: fallbackState.deepSeekSession.turnCount + 1,
      sessionCacheHitTokens: fallbackState.deepSeekSession.sessionCacheHitTokens + usage.promptCacheHitTokens,
      sessionCacheMissTokens: fallbackState.deepSeekSession.sessionCacheMissTokens + usage.promptCacheMissTokens,
      sessionCacheHitRate: 0.96,
      appendOnlyPrefixStable: true,
      previousRequestMessageCount: fallbackState.deepSeekSession.messageCount,
      commonPrefixMessageCount: fallbackState.deepSeekSession.messageCount,
      startedAt: fallbackState.deepSeekSession.startedAt ?? new Date().toISOString(),
      resetReason: "预览模式会话已初始化。",
    },
    usageMetrics: usage,
    cacheHitRate: 0.96,
    cacheHealth: {
      status: "ok",
      message: "缓存命中率达到 96% 目标。",
      hitRate: 0.96,
      targetHitRate: 0.96,
      hitTokens: usage.promptCacheHitTokens,
      missTokens: usage.promptCacheMissTokens,
      totalCacheTokens: usage.promptCacheHitTokens + usage.promptCacheMissTokens,
      missTokenBudget: 4,
      requiredHitTokens: 96,
      additionalHitTokensNeeded: 0,
      shortPrompt: true,
      sampleCount: 1,
      consecutiveBelowTarget: 0,
      hitTokensIncreasing: false,
      missTokensStable: false,
      missTokensImproving: false,
    },
    cacheDiagnostics: ["缓存命中率达到 96% 目标。"],
  };
  return cloneChatResult({
    content: "预览模式返回：DeepSeek 试聊链路已接入，桌面环境会调用真实流式接口。",
    model: fallbackState.deepSeek.models[0]?.id ?? "deepseek-v4-flash",
    usage,
    state: fallbackState,
  });
}

export async function resetDeepSeekSession(): Promise<WorkbenchState> {
  const binding = wailsBinding();
  if (binding) {
    return binding.ResetDeepSeekSession();
  }
  fallbackState = {
    ...fallbackState,
    usageMetrics: {
      promptCacheHitTokens: 0,
      promptCacheMissTokens: 0,
      inputTokens: 0,
      outputTokens: 0,
      effectiveCost: 0,
    },
    cacheHitRate: 0,
    cacheHealth: pendingCacheHealth(),
    deepSeekSession: {
      active: false,
      model: "",
      reasoning: fallbackState.reasoning.id,
      thinkingMode: thinkingModeForReasoning(fallbackState.reasoning.id),
      reasoningEffort: reasoningEffortForReasoning(fallbackState.reasoning.id),
      prefixHash: "",
      systemPromptHash: "",
      stablePromptTokens: 0,
      messageCount: 0,
      turnCount: 0,
      sessionCacheHitTokens: 0,
      sessionCacheMissTokens: 0,
      sessionCacheHitRate: 0,
      appendOnlyPrefixStable: true,
      previousRequestMessageCount: 0,
      commonPrefixMessageCount: 0,
      resetReason: "用户手动开启新会话。",
    },
    cacheDiagnostics: ["等待首轮模型请求记录缓存命中数据。"],
  };
  return cloneState(fallbackState);
}

export async function saveRuntimeSettings(settings: RuntimeSettings): Promise<WorkbenchState> {
  const binding = wailsBinding();
  if (binding) {
    return binding.SaveRuntimeSettings(settings);
  }
  fallbackState = {
    ...fallbackState,
    runtimeSettings: normalizeRuntimeSettings(settings),
    cacheTarget: normalizeRuntimeSettings(settings).cacheTargetPercent / 100,
  };
  return cloneState(fallbackState);
}

export async function saveModelProviderAPIKey(providerID: string, apiKey: string): Promise<WorkbenchState> {
  const binding = wailsBinding();
  if (binding) {
    return binding.SaveModelProviderAPIKey(providerID, apiKey);
  }
  fallbackState = updateFallbackProvider(providerID, (provider) => ({
    ...provider,
    apiKeyConfigured: Boolean(apiKey.trim()),
    lastSyncStatus: "idle",
    lastSyncMessage: apiKey.trim() ? "API Key 已保存，等待刷新模型。" : "API Key 不能为空。",
    checkedAt: undefined,
  }));
  return cloneState(fallbackState);
}

export async function clearModelProviderAPIKey(providerID: string): Promise<WorkbenchState> {
  const binding = wailsBinding();
  if (binding) {
    return binding.ClearModelProviderAPIKey(providerID);
  }
  fallbackState = updateFallbackProvider(providerID, (provider) => ({
    ...provider,
    apiKeyConfigured: false,
    models: [],
    defaultModelId: "",
    lastSyncStatus: "idle",
    lastSyncMessage: "API Key 已清除。",
    checkedAt: undefined,
  }));
  return cloneState(fallbackState);
}

export async function refreshModelProviderModels(providerID: string): Promise<WorkbenchState> {
  const binding = wailsBinding();
  if (binding) {
    return binding.RefreshModelProviderModels(providerID);
  }
  const now = new Date().toISOString();
  const fallbackModels = providerID === "deepseek"
    ? [
        { id: "deepseek-v4-flash", displayName: "DeepSeek V4 Flash", provider: "deepseek", contextWindowTokens: 64000 },
        { id: "deepseek-v4-pro", displayName: "DeepSeek V4 Pro", provider: "deepseek", contextWindowTokens: 64000 },
      ]
    : [
        { id: "upstream-chat", displayName: "upstream-chat", provider: providerID, contextWindowTokens: 128000 },
        { id: "upstream-reasoner", displayName: "upstream-reasoner", provider: providerID, contextWindowTokens: 128000 },
      ];
  fallbackState = updateFallbackProvider(providerID, (provider) => ({
    ...provider,
    models: fallbackModels,
    defaultModelId: provider.defaultModelId || fallbackModels[0]?.id || "",
    lastSyncStatus: "ok",
    lastSyncMessage: `预览模式连接成功，发现 ${fallbackModels.length} 个模型。`,
    checkedAt: now,
  }));
  const runtimeSettings = fallbackState.runtimeSettings;
  if (providerID === "deepseek") {
    fallbackState = {
      ...fallbackState,
      deepSeek: {
        ...fallbackState.deepSeek,
        configured: true,
        models: fallbackModels,
        lastCheckStatus: "ok",
        lastCheckMessage: `预览模式连接成功，发现 ${fallbackModels.length} 个模型。`,
        checkedAt: now,
      },
      runtimeSettings,
    };
  }
  return cloneState(fallbackState);
}

function wailsBinding(): WailsAppBinding | undefined {
  return (window as WailsWindow).go?.main?.App;
}

function createFallbackState(level: ReasoningLevel): WorkbenchState {
  const reasoning = reasoningOptions.find((option) => option.id === level) ?? reasoningOptions[0];
  return {
    reasoning,
    reasoningOptions,
    cacheTarget: 0.96,
    usageMetrics: {
      promptCacheHitTokens: 0,
      promptCacheMissTokens: 0,
      inputTokens: 0,
      outputTokens: 0,
      effectiveCost: 0,
    },
    cacheHitRate: 0,
    cacheHealth: pendingCacheHealth(),
    deepSeek: {
      configured: false,
      baseUrl: "https://api.deepseek.com",
      lastCheckStatus: "idle",
      lastCheckMessage: "等待保存 DeepSeek API Key。",
      models: [],
    },
    deepSeekSession: {
      active: false,
      model: "",
      reasoning: reasoning.id,
      thinkingMode: thinkingModeForReasoning(reasoning.id),
      reasoningEffort: reasoningEffortForReasoning(reasoning.id),
      prefixHash: "",
      systemPromptHash: "",
      stablePromptTokens: 0,
      messageCount: 0,
      turnCount: 0,
      sessionCacheHitTokens: 0,
      sessionCacheMissTokens: 0,
      sessionCacheHitRate: 0,
      appendOnlyPrefixStable: true,
      previousRequestMessageCount: 0,
      commonPrefixMessageCount: 0,
      resetReason: "等待首轮会话初始化。",
    },
    skillsIndex: fallbackSkillsIndex,
    mcpSnapshots: fallbackSnapshots,
    contextPreview: {
      stablePrefix: [
        { name: "product_identity", content: "MHcode 是面向开发者的 AI 协议交换台。" },
        { name: "system_rules", content: "稳定前缀保持顺序、文本和 schema 哈希可复现。" },
        { name: "reasoning", content: `${reasoning.id}:${reasoning.budget.cachePolicy}` },
        { name: "skills_index", content: "skill: mhcode-agent-core" },
        { name: "mcp_schema_snapshot", content: "filesystem tools summary-first" },
        { name: "project_summary", content: "Go 核心引擎 + Wails v2 + SolidJS 前端。" },
        { name: "routing_policy", content: "DeepSeek official first, OpenAI-compatible later." },
      ],
      volatileTail: [
        { name: "user_input", content: "用户本轮输入会进入易变尾部。" },
        { name: "recent_diff", content: "" },
        { name: "tool_results", content: "[]" },
        { name: "output_requirements", content: "输出结构化摘要。" },
      ],
      prefixHash: "sha256:local-preview",
    },
    cacheDiagnostics: ["等待首轮模型请求记录缓存命中数据。"],
    runtimeSettings: defaultRuntimeSettings(),
  };
}

function cloneState(state: WorkbenchState): WorkbenchState {
  return JSON.parse(JSON.stringify(state)) as WorkbenchState;
}

function cloneChatResult(result: ChatResult): ChatResult {
  return JSON.parse(JSON.stringify(result)) as ChatResult;
}

function pendingCacheHealth() {
  return {
    status: "pending",
    message: "等待首轮模型请求记录缓存命中数据。",
    hitRate: 0,
    targetHitRate: 0.96,
    hitTokens: 0,
    missTokens: 0,
    totalCacheTokens: 0,
    missTokenBudget: 0,
    requiredHitTokens: 0,
    additionalHitTokensNeeded: 0,
    shortPrompt: false,
    sampleCount: 0,
    consecutiveBelowTarget: 0,
    hitTokensIncreasing: false,
    missTokensStable: false,
    missTokensImproving: false,
  };
}

function defaultRuntimeSettings(): RuntimeSettings {
  return {
    sandboxMode: "workspace-write",
    filesystemAccess: "workspace-write",
    networkAccess: true,
    shellAccess: true,
    approvalPolicy: "on-request",
    workspaceRoot: "C:\\Users\\Administrator\\Desktop\\MHcode",
    extraWritableRoots: [],
    maxCommandSeconds: 120,
    allowDestructiveOps: false,
    toolResultPolicy: "summary-first",
    stablePrefixPolicy: "strict-stable-prefix",
    cacheTargetPercent: 96,
    git: {
      branchPrefix: "mhcode/",
      mergeMethod: "merge",
      showPullRequestIcon: true,
      forcePushWithLease: false,
      draftPullRequests: true,
      autoDeleteOldWorktrees: true,
      worktreeCleanupLimit: 15,
      commitInstructions: "",
      pullRequestInstructions: "",
    },
    browser: {
      enabled: true,
      defaultLocalUrlDestination: "mhcode",
      clearDataPolicy: "ask",
      screenshotAnnotations: "always",
      passwordManagerEnabled: false,
      autofillContactEnabled: false,
      sitePermissions: [],
      developerCdpAccess: false,
    },
    computerControl: {
      anyAppEnabled: false,
      chromeEnabled: false,
      alwaysAllowedApps: [],
    },
    mcp: {
      servers: [
        {
          id: "filesystem",
          name: "filesystem",
          command: "builtin:filesystem",
          args: [],
          env: [],
          passEnvironment: [],
          workingDirectory: "",
          enabled: true,
          toolResultPolicy: "summary-first",
        },
      ],
    },
    model: {
      selectedProviderId: "deepseek",
      selectedModelId: "",
      providers: [
        {
          id: "deepseek",
          name: "DeepSeek 官方",
          protocol: "deepseek-official",
          apiType: "chat-completions",
          baseUrl: "https://api.deepseek.com",
          balanceUrl: "",
          enabled: true,
          apiKeyConfigured: false,
          defaultModelId: "",
          contextWindowTokens: 64000,
          models: [],
          lastSyncStatus: "idle",
          lastSyncMessage: "等待保存 API Key 后刷新模型。",
          supportsModelFetch: true,
        },
        {
          id: "openai-compatible",
          name: "OpenAI 兼容",
          protocol: "openai-compatible",
          apiType: "chat-completions",
          baseUrl: "https://api.openai.com/v1",
          balanceUrl: "",
          enabled: false,
          apiKeyConfigured: false,
          defaultModelId: "",
          contextWindowTokens: 0,
          models: [],
          lastSyncStatus: "idle",
          lastSyncMessage: "填写 Base URL 与 API Key 后可自动获取模型。",
          supportsModelFetch: true,
        },
        {
          id: "local-openai",
          name: "本地 OpenAI 兼容",
          protocol: "openai-compatible",
          apiType: "chat-completions",
          baseUrl: "http://127.0.0.1:11434/v1",
          balanceUrl: "",
          enabled: false,
          apiKeyConfigured: false,
          defaultModelId: "",
          contextWindowTokens: 0,
          models: [],
          lastSyncStatus: "idle",
          lastSyncMessage: "适用于 Ollama、LM Studio 等本地兼容服务。",
          supportsModelFetch: true,
        },
      ],
    },
    workspace: {
      configured: true,
      dependenciesEnabled: true,
    },
  };
}

function normalizeRuntimeSettings(settings: RuntimeSettings): RuntimeSettings {
  const defaults = defaultRuntimeSettings();
  const merged = {
    ...defaults,
    ...settings,
    git: { ...defaults.git, ...settings.git },
    browser: {
      ...defaults.browser,
      ...settings.browser,
      sitePermissions: Array.isArray(settings.browser?.sitePermissions) ? settings.browser.sitePermissions : [],
    },
    computerControl: {
      ...defaults.computerControl,
      ...settings.computerControl,
      alwaysAllowedApps: Array.isArray(settings.computerControl?.alwaysAllowedApps)
        ? settings.computerControl.alwaysAllowedApps
        : [],
    },
    mcp: {
      ...defaults.mcp,
      ...settings.mcp,
      servers: Array.isArray(settings.mcp?.servers) && settings.mcp.servers.length > 0 ? settings.mcp.servers : defaults.mcp.servers,
    },
    model: {
      ...defaults.model,
      ...settings.model,
      providers: mergeProviders(defaults.model.providers, settings.model?.providers ?? []),
    },
    workspace: { ...defaults.workspace, ...settings.workspace },
  };
  return {
    ...merged,
    extraWritableRoots: Array.isArray(merged.extraWritableRoots)
      ? merged.extraWritableRoots.map((item) => item.trim()).filter(Boolean)
      : [],
    maxCommandSeconds: clampNumber(Number(merged.maxCommandSeconds), 5, 3600),
    cacheTargetPercent: clampNumber(Number(merged.cacheTargetPercent), 0, 100),
    git: {
      ...merged.git,
      worktreeCleanupLimit: clampNumber(Number(merged.git.worktreeCleanupLimit), 1, 99),
    },
    browser: {
      ...merged.browser,
      sitePermissions: merged.browser.sitePermissions
        .map((item) => ({
          origin: item.origin.trim(),
          camera: item.camera || "ask",
          microphone: item.microphone || "ask",
          clipboard: item.clipboard || "ask",
        }))
        .filter((item) => item.origin),
    },
    computerControl: {
      ...merged.computerControl,
      alwaysAllowedApps: merged.computerControl.alwaysAllowedApps.map((item) => item.trim()).filter(Boolean),
    },
    mcp: {
      ...merged.mcp,
      servers: merged.mcp.servers.map((server) => ({
        ...server,
        id: server.id.trim() || server.name.trim() || server.command.trim(),
        name: server.name.trim() || server.id.trim() || "MCP Server",
        command: server.command.trim(),
        args: Array.isArray(server.args) ? server.args.map((item) => item.trim()).filter(Boolean) : [],
        env: Array.isArray(server.env)
          ? server.env.map((item) => ({ key: item.key.trim(), value: item.value.trim() })).filter((item) => item.key)
          : [],
        passEnvironment: Array.isArray(server.passEnvironment) ? server.passEnvironment.map((item) => item.trim()).filter(Boolean) : [],
        workingDirectory: server.workingDirectory?.trim() ?? "",
        toolResultPolicy: server.toolResultPolicy || "summary-first",
      })),
    },
    model: {
      ...merged.model,
      selectedProviderId: merged.model.selectedProviderId || merged.model.providers[0]?.id || "deepseek",
      providers: merged.model.providers.map((provider) => ({
        ...provider,
        id: (provider.id ?? "").trim(),
        name: (provider.name ?? "").trim() || provider.id,
        protocol: provider.protocol || "openai-compatible",
        apiType: provider.apiType || defaultAPITypeForProtocol(provider.protocol),
        baseUrl: (provider.baseUrl ?? "").trim(),
        balanceUrl: provider.balanceUrl?.trim() ?? "",
        contextWindowTokens: normalizeTokenWindow(provider.contextWindowTokens),
        models: Array.isArray(provider.models)
          ? provider.models.map((model) => ({
              ...model,
              id: (model.id ?? "").trim(),
              displayName: model.displayName?.trim() || model.id,
              provider: model.provider?.trim() || provider.id,
              contextWindowTokens: normalizeTokenWindow(model.contextWindowTokens),
            }))
          : [],
        supportsModelFetch: supportsProviderModelFetch(provider.protocol),
      })),
    },
  };
}

function mergeProviders(defaults: RuntimeSettings["model"]["providers"], providers: RuntimeSettings["model"]["providers"]) {
  const merged = [...providers];
  const ids = new Set(merged.map((provider) => provider.id));
  for (const provider of defaults) {
    if (!ids.has(provider.id)) {
      merged.push(provider);
    }
  }
  return merged;
}

function updateFallbackProvider(
  providerID: string,
  updater: (provider: RuntimeSettings["model"]["providers"][number]) => RuntimeSettings["model"]["providers"][number],
) {
  const runtimeSettings = normalizeRuntimeSettings(fallbackState.runtimeSettings);
  const providers = runtimeSettings.model.providers.map((provider) => (provider.id === providerID ? updater(provider) : provider));
  return {
    ...fallbackState,
    runtimeSettings: normalizeRuntimeSettings({
      ...runtimeSettings,
      model: {
        ...runtimeSettings.model,
        providers,
      },
    }),
  };
}

function clampNumber(value: number, min: number, max: number) {
  if (!Number.isFinite(value)) {
    return min;
  }
  return Math.min(Math.max(value, min), max);
}

function normalizeTokenWindow(value: number) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed < 0) {
    return 0;
  }
  return Math.floor(parsed);
}

function defaultAPITypeForProtocol(protocol: string) {
  return protocol === "anthropic" || protocol === "anthropic-compatible" ? "anthropic-messages" : "chat-completions";
}

function supportsProviderModelFetch(protocol: string) {
  return protocol === "deepseek-official" || protocol === "openai-compatible" || protocol === "local";
}

function thinkingModeForReasoning(level: ReasoningLevel) {
  return level === "high" || level === "ultra" ? "enabled" : "disabled";
}

function reasoningEffortForReasoning(level: ReasoningLevel) {
  if (level === "ultra") {
    return "max";
  }
  if (level === "high") {
    return "high";
  }
  return "";
}
