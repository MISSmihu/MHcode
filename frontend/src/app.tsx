import {
  AlertTriangle,
  Archive,
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  BarChart3,
  Bot,
  Braces,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Command,
  Cpu,
  Database,
  FileText,
  Folder,
  Gauge,
  GitBranch,
  Globe2,
  Hash,
  HardDrive,
  Keyboard,
  KeyRound,
  LayoutList,
  ListFilter,
  LockKeyhole,
  MessageSquarePlus,
  Monitor,
  Moon,
  Network,
  Palette,
  Plug,
  Plus,
  RefreshCw,
  Save,
  Search,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  Sun,
  Terminal,
  Trash2,
  User,
  Wrench,
  X,
  Zap,
} from "lucide-solid";
import { For, Match, Show, Switch, createEffect, createMemo, createSignal, onCleanup, onMount } from "solid-js";
import type { JSX } from "solid-js";
import { ReasoningMenu } from "./components/ReasoningMenu";
import {
  clearDeepSeekAPIKey,
  getWorkbenchState,
  resetDeepSeekSession,
  refreshModelProviderModels,
  saveDeepSeekAPIKey,
  saveModelProviderAPIKey,
  sendDeepSeekMessage,
  setReasoningLevel,
  saveRuntimeSettings,
  testDeepSeekConnection,
  clearModelProviderAPIKey,
} from "./services/workbench";
import { defaultReasoningLevel, reasoningOptions as fallbackReasoningOptions } from "./state/reasoning";
import type {
  DeepSeekSessionState,
  MCPServerSetting,
  ModelProviderSetting,
  ReasoningLevel,
  RuntimeSettings,
  UsageMetrics,
  WorkbenchState,
} from "./types";

const fallbackProfile =
  fallbackReasoningOptions.find((option) => option.id === defaultReasoningLevel) ?? fallbackReasoningOptions[0];

type ChatMessage = {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  createdAt: string;
  model?: string;
  reasoning?: string;
  usage?: UsageMetrics;
  failed?: boolean;
};

type DrawerTab = "settings" | "cache" | "context" | "tools";
type SettingsCategory =
  | "general"
  | "appearance"
  | "config"
  | "profile"
  | "shortcuts"
  | "mcp"
  | "browser"
  | "computer"
  | "models"
  | "skills"
  | "commands"
  | "index"
  | "usage"
  | "environment"
  | "git"
  | "archive";
type ThemeMode = "dark" | "light";

const settingsGroups: Array<{
  title: string;
  items: Array<{ id: SettingsCategory; label: string; icon: JSX.Element }>;
}> = [
  {
    title: "个人",
    items: [
      { id: "general", label: "常规", icon: <Settings size={15} /> },
      { id: "appearance", label: "外观", icon: <Palette size={15} /> },
      { id: "config", label: "配置", icon: <SlidersHorizontal size={15} /> },
      { id: "profile", label: "个性化", icon: <User size={15} /> },
      { id: "shortcuts", label: "键盘快捷键", icon: <Keyboard size={15} /> },
    ],
  },
  {
    title: "集成",
    items: [
      { id: "mcp", label: "MCP 服务器", icon: <Plug size={15} /> },
      { id: "browser", label: "浏览器", icon: <Globe2 size={15} /> },
      { id: "computer", label: "电脑操控", icon: <Monitor size={15} /> },
    ],
  },
  {
    title: "编码",
    items: [
      { id: "models", label: "模型设置", icon: <Database size={15} /> },
      { id: "skills", label: "技能", icon: <Wrench size={15} /> },
      { id: "commands", label: "命令", icon: <Terminal size={15} /> },
      { id: "index", label: "索引库", icon: <Hash size={15} /> },
      { id: "usage", label: "使用统计", icon: <BarChart3 size={15} /> },
      { id: "git", label: "Git", icon: <GitBranch size={15} /> },
      { id: "environment", label: "环境", icon: <Folder size={15} /> },
    ],
  },
  {
    title: "已归档",
    items: [{ id: "archive", label: "已归档对话", icon: <Archive size={15} /> }],
  },
];

const sandboxOptions = [
  { value: "read-only", label: "只读", description: "只允许读取项目内容" },
  { value: "workspace-write", label: "工作区写入", description: "允许修改当前工作区" },
  { value: "danger-full-access", label: "全权限", description: "不限制文件系统边界" },
];

const filesystemOptions = [
  { value: "read-only", label: "只读" },
  { value: "workspace-write", label: "工作区" },
  { value: "unrestricted", label: "不限制" },
];

const approvalOptions = [
  { value: "on-request", label: "按需确认" },
  { value: "on-failure", label: "失败后确认" },
  { value: "untrusted", label: "不可信时确认" },
  { value: "never", label: "永不询问" },
];

const toolResultOptions = [
  { value: "summary-first", label: "摘要优先" },
  { value: "balanced", label: "平衡" },
  { value: "raw-local", label: "原文仅本地" },
];

const stablePrefixOptions = [
  { value: "reuse-prefix", label: "复用前缀" },
  { value: "stable-prefix", label: "稳定前缀" },
  { value: "strict-stable-prefix", label: "严格稳定" },
];

const providerProtocolOptions = [
  { value: "deepseek-official", label: "DeepSeek 官方" },
  { value: "openai-compatible", label: "OpenAI 兼容" },
  { value: "anthropic-compatible", label: "Anthropic 兼容" },
  { value: "gemini", label: "Gemini" },
  { value: "local", label: "本地兼容" },
];

const providerAPITypeOptions = [
  { value: "chat-completions", label: "Chat Completions" },
  { value: "responses", label: "Responses" },
  { value: "anthropic-messages", label: "Anthropic Messages" },
  { value: "gemini-generate-content", label: "Gemini Generate Content" },
];

type ProviderPreset = {
  id: string;
  name: string;
  protocol: ModelProviderSetting["protocol"];
  apiType: ModelProviderSetting["apiType"];
  baseUrl: string;
  balanceUrl?: string;
  contextWindowTokens?: number;
  note: string;
};

const providerPresets: ProviderPreset[] = [
  {
    id: "deepseek",
    name: "DeepSeek 官方",
    protocol: "deepseek-official",
    apiType: "chat-completions",
    baseUrl: "https://api.deepseek.com",
    contextWindowTokens: 64000,
    note: "官方通道，优先用于缓存命中观测。",
  },
  {
    id: "openai-compatible",
    name: "OpenAI 兼容",
    protocol: "openai-compatible",
    apiType: "chat-completions",
    baseUrl: "https://api.openai.com/v1",
    note: "标准 /v1/models 与 /chat/completions。",
  },
  {
    id: "anthropic-official",
    name: "Anthropic 官方",
    protocol: "anthropic-compatible",
    apiType: "anthropic-messages",
    baseUrl: "https://api.anthropic.com",
    note: "Claude Messages API 原生协议。",
  },
  {
    id: "google-gemini",
    name: "Google Gemini",
    protocol: "gemini",
    apiType: "gemini-generate-content",
    baseUrl: "https://generativelanguage.googleapis.com/v1beta",
    note: "Gemini generateContent 原生协议。",
  },
  {
    id: "glm-cn",
    name: "GLM CN API",
    protocol: "openai-compatible",
    apiType: "chat-completions",
    baseUrl: "https://open.bigmodel.cn/api/paas/v4",
    note: "智谱国内 OpenAI 兼容入口。",
  },
  {
    id: "zai-global",
    name: "Z.AI Global API",
    protocol: "openai-compatible",
    apiType: "chat-completions",
    baseUrl: "https://api.z.ai/api/paas/v4",
    note: "国际站 OpenAI 兼容入口。",
  },
  {
    id: "qwen-cn",
    name: "Qwen CN API",
    protocol: "openai-compatible",
    apiType: "chat-completions",
    baseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    note: "通义千问兼容模式。",
  },
  {
    id: "kimi",
    name: "Kimi",
    protocol: "openai-compatible",
    apiType: "chat-completions",
    baseUrl: "https://api.moonshot.cn/v1",
    note: "Moonshot / Kimi 兼容入口。",
  },
  {
    id: "minimax",
    name: "MiniMax",
    protocol: "openai-compatible",
    apiType: "chat-completions",
    baseUrl: "https://api.minimax.chat/v1",
    note: "MiniMax OpenAI 兼容入口。",
  },
  {
    id: "huggingface-router",
    name: "HuggingFace Router",
    protocol: "openai-compatible",
    apiType: "chat-completions",
    baseUrl: "https://router.huggingface.co/v1",
    note: "HuggingFace Inference Router。",
  },
  {
    id: "nvidia-nim",
    name: "NVIDIA NIM",
    protocol: "openai-compatible",
    apiType: "chat-completions",
    baseUrl: "https://integrate.api.nvidia.com/v1",
    note: "NIM OpenAI 兼容入口。",
  },
  {
    id: "ollama-local",
    name: "Ollama 本地",
    protocol: "local",
    apiType: "chat-completions",
    baseUrl: "http://127.0.0.1:11434/v1",
    note: "本地服务可无密钥拉取模型。",
  },
];

const sitePermissionOptions = [
  { value: "ask", label: "询问" },
  { value: "allow", label: "允许" },
  { value: "block", label: "阻止" },
];

const defaultSidebarWidth = 268;
const minSidebarWidth = 220;
const maxSidebarWidth = 420;
const sidebarWidthStorageKey = "mhcode:sidebar-width";
const themeStorageKey = "mhcode:theme";

function App() {
  const [state, setState] = createSignal<WorkbenchState>();
  const [loading, setLoading] = createSignal(true);
  const [updatingReasoning, setUpdatingReasoning] = createSignal(false);
  const [savingKey, setSavingKey] = createSignal(false);
  const [testingDeepSeek, setTestingDeepSeek] = createSignal(false);
  const [clearingKey, setClearingKey] = createSignal(false);
  const [savingRuntime, setSavingRuntime] = createSignal(false);
  const [sendingMessage, setSendingMessage] = createSignal(false);
  const [resettingSession, setResettingSession] = createSignal(false);
  const [apiKeyDraft, setAPIKeyDraft] = createSignal("");
  const [providerKeyDrafts, setProviderKeyDrafts] = createSignal<Record<string, string>>({});
  const [savingProviderID, setSavingProviderID] = createSignal("");
  const [clearingProviderID, setClearingProviderID] = createSignal("");
  const [syncingProviderID, setSyncingProviderID] = createSignal("");
  const [promptDraft, setPromptDraft] = createSignal("");
  const [messages, setMessages] = createSignal<ChatMessage[]>([]);
  const [runtimeDraft, setRuntimeDraft] = createSignal<RuntimeSettings>();
  const [drawerOpen, setDrawerOpen] = createSignal(false);
  const [activeSettingsCategory, setActiveSettingsCategory] = createSignal<SettingsCategory>("general");
  const [error, setError] = createSignal("");
  const [sidebarWidth, setSidebarWidth] = createSignal(readStoredSidebarWidth());
  const [resizingSidebar, setResizingSidebar] = createSignal(false);
  const [themeMode, setThemeMode] = createSignal<ThemeMode>(readStoredThemeMode());
  let chatScrollRef: HTMLElement | undefined;
  let shellRef: HTMLElement | undefined;
  let pointerSidebarResizeActive = false;
  let mouseSidebarResizeActive = false;

  const profile = createMemo(() => state()?.reasoning ?? fallbackProfile);
  const options = createMemo(() => state()?.reasoningOptions ?? fallbackReasoningOptions);
  const usage = createMemo(() => state()?.usageMetrics ?? emptyUsageMetrics());
  const cacheTarget = createMemo(() => state()?.cacheTarget ?? 0.96);
  const cacheHitRate = createMemo(() => state()?.cacheHitRate ?? 0);
  const hasCacheTokens = createMemo(() => usage().promptCacheHitTokens + usage().promptCacheMissTokens > 0);
  const cacheHealth = createMemo(() => state()?.cacheHealth ?? fallbackCacheHealth());
  const snapshots = createMemo(() => state()?.mcpSnapshots ?? []);
  const skillsIndex = createMemo(() => state()?.skillsIndex ?? []);
  const contextPreview = createMemo(() => state()?.contextPreview);
  const runtimeSettings = createMemo(() => state()?.runtimeSettings ?? fallbackRuntimeSettings());
  const configFiles = createMemo(() => state()?.configFiles ?? fallbackConfigFiles());
  const activeRuntimeDraft = createMemo(() => runtimeDraft() ?? runtimeSettings());
  const hasProviderKeyDrafts = createMemo(() => Object.values(providerKeyDrafts()).some((apiKey) => apiKey.trim()));
  const diagnostics = createMemo(() => state()?.cacheDiagnostics ?? []);
  const deepSeek = createMemo(() => state()?.deepSeek ?? fallbackDeepSeekState());
  const deepSeekSession = createMemo(() => state()?.deepSeekSession ?? fallbackDeepSeekSession());
  const sessionHasCacheTokens = createMemo(
    () => deepSeekSession().sessionCacheHitTokens + deepSeekSession().sessionCacheMissTokens > 0,
  );
  const activeChatProvider = createMemo(() => selectedModelProvider(runtimeSettings()));
  const activeChatModel = createMemo(() => selectedModelName(runtimeSettings(), deepSeekSession().model));
  const activeProviderReady = createMemo(() => providerReadyForChat(activeChatProvider()));
  const activeProviderConnection = createMemo(() => providerConnectionSummary(activeChatProvider(), deepSeek()));
  const modelName = createMemo(() => {
    const provider = activeChatProvider();
    const model = deepSeekSession().model || activeChatModel();
    if (!provider) {
      return model || "选择模型";
    }
    return model ? `${provider.name} · ${model}` : provider.name;
  });
  const canSend = createMemo(() => promptDraft().trim().length > 0 && activeProviderReady() && Boolean(activeChatModel()) && !sendingMessage());

  const sidebarSessions = createMemo(() => [
    {
      title: messages()[messages().length - 1]?.content || "当前对话",
      meta: deepSeekSession().active ? `${formatInteger(deepSeekSession().turnCount)} 轮` : "今天",
      active: true,
      dot: true,
      onClick: () => undefined,
    },
    {
      title: "缓存命中率",
      meta: formatPercent(cacheHitRate(), hasCacheTokens()),
      active: activeSettingsCategory() === "usage" && drawerOpen(),
      dot: cacheHealth().status === "low",
      onClick: () => openDrawer("cache"),
    },
    {
      title: "# MHcode Agent 缓存优化",
      meta: "6天",
      active: false,
      dot: true,
      onClick: () => openDrawer("context"),
    },
    {
      title: "# DeepSeek 前缀稳定",
      meta: "17天",
      active: false,
      dot: false,
      onClick: () => openDrawer("cache"),
    },
    {
      title: "工具链与 Skills",
      meta: `${formatInteger(skillsIndex().length)} 项`,
      active: activeSettingsCategory() === "skills" && drawerOpen(),
      dot: false,
      onClick: () => openDrawer("tools"),
    },
  ]);

  onMount(() => {
    applyThemeMode(themeMode());
    applySidebarWidth(sidebarWidth(), shellRef);
    void refreshState();
  });

  createEffect(() => {
    applyThemeMode(themeMode(), shellRef);
  });

  createEffect(() => {
    applySidebarWidth(sidebarWidth(), shellRef);
  });

  createEffect(() => {
    messages().length;
    sendingMessage();
    queueMicrotask(() => {
      chatScrollRef?.scrollTo({ top: chatScrollRef.scrollHeight });
    });
  });

  const refreshState = async () => {
    setLoading(true);
    setError("");
    try {
      setState(await getWorkbenchState());
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const changeReasoning = async (level: ReasoningLevel) => {
    setUpdatingReasoning(true);
    setError("");
    try {
      setState(await setReasoningLevel(level));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setUpdatingReasoning(false);
    }
  };

  const saveKey = async () => {
    setSavingKey(true);
    setError("");
    try {
      setState(await saveDeepSeekAPIKey(apiKeyDraft()));
      setAPIKeyDraft("");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingKey(false);
    }
  };

  const testConnection = async () => {
    setTestingDeepSeek(true);
    setError("");
    try {
      setState(await testDeepSeekConnection());
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setTestingDeepSeek(false);
    }
  };

  const clearKey = async () => {
    setClearingKey(true);
    setError("");
    try {
      setState(await clearDeepSeekAPIKey());
      setAPIKeyDraft("");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setClearingKey(false);
    }
  };

  const setProviderKeyDraft = (providerID: string, value: string) => {
    setProviderKeyDrafts((current) => ({
      ...current,
      [providerID]: value,
    }));
  };

  const saveProviderKey = async (providerID: string) => {
    const apiKey = providerKeyDrafts()[providerID]?.trim() ?? "";
    if (!apiKey) {
      setError("请先填写 API Key。");
      return;
    }
    setSavingProviderID(providerID);
    setError("");
    try {
      if (runtimeDraft()) {
        setState(await saveRuntimeSettings(activeRuntimeDraft()));
        setRuntimeDraft(undefined);
      }
      setState(await saveModelProviderAPIKey(providerID, apiKey));
      setProviderKeyDraft(providerID, "");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingProviderID("");
    }
  };

  const clearProviderKey = async (providerID: string) => {
    setClearingProviderID(providerID);
    setError("");
    try {
      if (runtimeDraft()) {
        setState(await saveRuntimeSettings(activeRuntimeDraft()));
        setRuntimeDraft(undefined);
      }
      setState(await clearModelProviderAPIKey(providerID));
      setProviderKeyDraft(providerID, "");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setClearingProviderID("");
    }
  };

  const syncProviderModels = async (providerID: string) => {
    setSyncingProviderID(providerID);
    setError("");
    try {
      if (runtimeDraft()) {
        setState(await saveRuntimeSettings(activeRuntimeDraft()));
        setRuntimeDraft(undefined);
      }
      const apiKey = providerKeyDrafts()[providerID]?.trim() ?? "";
      if (apiKey) {
        setState(await saveModelProviderAPIKey(providerID, apiKey));
        setProviderKeyDraft(providerID, "");
      }
      setState(await refreshModelProviderModels(providerID));
      setRuntimeDraft(undefined);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSyncingProviderID("");
    }
  };

  const resetSession = async () => {
    setResettingSession(true);
    setError("");
    try {
      setState(await resetDeepSeekSession());
      setMessages([]);
      setPromptDraft("");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setResettingSession(false);
    }
  };

  const saveRuntime = async () => {
    setSavingRuntime(true);
    setError("");
    try {
      let next = await saveRuntimeSettings(activeRuntimeDraft());
      const pendingProviderKeys = Object.entries(providerKeyDrafts()).filter(([, apiKey]) => apiKey.trim());
      const savedProviderIDs: string[] = [];
      for (const [providerID, apiKey] of pendingProviderKeys) {
        next = await saveModelProviderAPIKey(providerID, apiKey.trim());
        savedProviderIDs.push(providerID);
      }
      setState(next);
      setRuntimeDraft(undefined);
      if (savedProviderIDs.length > 0) {
        setProviderKeyDrafts((current) => {
          const copy = { ...current };
          for (const providerID of savedProviderIDs) {
            delete copy[providerID];
          }
          return copy;
        });
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingRuntime(false);
    }
  };

  const selectModelRoute = async (providerID: string, modelID: string) => {
    const base = activeRuntimeDraft();
    const provider = base.model.providers.find((item) => item.id === providerID);
    if (!provider) {
      setError("未找到模型供应商。");
      return;
    }
    const nextSettings: RuntimeSettings = {
      ...base,
      model: {
        ...base.model,
        selectedProviderId: providerID,
        selectedModelId: modelID,
        providers: base.model.providers.map((item) =>
          item.id === providerID
            ? {
                ...item,
                enabled: true,
                defaultModelId: modelID || item.defaultModelId,
              }
            : item,
        ),
      },
    };
    setSavingRuntime(true);
    setError("");
    try {
      const next = await saveRuntimeSettings(nextSettings);
      setState(next);
      setRuntimeDraft(undefined);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingRuntime(false);
    }
  };

  const updateRuntimeDraft = (patch: Partial<RuntimeSettings>) => {
    setRuntimeDraft((current) => ({
      ...runtimeSettings(),
      ...current,
      ...patch,
    }));
  };

  const resetRuntimeDraft = () => {
    setRuntimeDraft(undefined);
    setProviderKeyDrafts({});
  };

  const sendMessage = async () => {
    const prompt = promptDraft().trim();
    if (!prompt || sendingMessage()) {
      return;
    }
    if (!activeProviderReady() || !activeChatModel()) {
      setError("请先在模型设置里完成当前供应商的密钥和模型配置。");
      openSettings("models");
      return;
    }

    setMessages((current) => [...current, createChatMessage("user", prompt)]);
    setPromptDraft("");
    setSendingMessage(true);
    setError("");
    try {
      const result = await sendDeepSeekMessage(prompt);
      setState(result.state);
      setMessages((current) => [
        ...current,
        {
          id: `assistant-${Date.now()}`,
          role: "assistant",
          content: result.content || result.reasoning || "本轮没有返回可展示内容。",
          reasoning: result.reasoning,
          model: result.model,
          usage: result.usage,
          createdAt: new Date().toISOString(),
        },
      ]);
    } catch (err) {
      const message = errorMessage(err);
      setError(message);
      setPromptDraft(prompt);
      setMessages((current) => [...current, { ...createChatMessage("system", message), failed: true }]);
    } finally {
      setSendingMessage(false);
    }
  };

  function openDrawer(tab: DrawerTab) {
    openSettings(categoryForDrawerTab(tab));
  }

  function openSettings(category: SettingsCategory) {
    setActiveSettingsCategory(category);
    setDrawerOpen(true);
  }

  const resizeSidebar = (clientX: number) => {
    const left = shellRef?.getBoundingClientRect().left ?? 0;
    const width = clamp(clientX - left, minSidebarWidth, maxSidebarWidth);
    setSidebarWidth(width);
    applySidebarWidth(width, shellRef);
  };

  const moveWindowPointerSidebarResize = (event: PointerEvent) => {
    if (!pointerSidebarResizeActive) {
      return;
    }
    resizeSidebar(event.clientX);
  };

  const stopWindowPointerSidebarResize = () => {
    if (!pointerSidebarResizeActive) {
      return;
    }
    pointerSidebarResizeActive = false;
    setResizingSidebar(false);
    persistSidebarWidth(sidebarWidth());
    window.removeEventListener("pointermove", moveWindowPointerSidebarResize);
    window.removeEventListener("pointerup", stopWindowPointerSidebarResize);
    window.removeEventListener("pointercancel", stopWindowPointerSidebarResize);
  };

  const startSidebarResize = (event: PointerEvent & { currentTarget: HTMLElement }) => {
    event.preventDefault();
    pointerSidebarResizeActive = true;
    setResizingSidebar(true);
    resizeSidebar(event.clientX);
    if (event.currentTarget.setPointerCapture) {
      event.currentTarget.setPointerCapture(event.pointerId);
    }
    window.addEventListener("pointermove", moveWindowPointerSidebarResize);
    window.addEventListener("pointerup", stopWindowPointerSidebarResize);
    window.addEventListener("pointercancel", stopWindowPointerSidebarResize);
  };

  const moveSidebarResize = (event: PointerEvent) => {
    moveWindowPointerSidebarResize(event);
  };

  const stopSidebarResize = (event: PointerEvent & { currentTarget: HTMLElement }) => {
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    stopWindowPointerSidebarResize();
  };

  const moveMouseSidebarResize = (event: MouseEvent) => {
    if (!mouseSidebarResizeActive) {
      return;
    }
    resizeSidebar(event.clientX);
  };

  const stopMouseSidebarResize = () => {
    if (!mouseSidebarResizeActive) {
      return;
    }
    mouseSidebarResizeActive = false;
    setResizingSidebar(false);
    persistSidebarWidth(sidebarWidth());
    window.removeEventListener("mousemove", moveMouseSidebarResize);
    window.removeEventListener("mouseup", stopMouseSidebarResize);
  };

  const startMouseSidebarResize = (event: MouseEvent) => {
    if (event.button !== 0 || resizingSidebar()) {
      return;
    }
    event.preventDefault();
    mouseSidebarResizeActive = true;
    setResizingSidebar(true);
    resizeSidebar(event.clientX);
    window.addEventListener("mousemove", moveMouseSidebarResize);
    window.addEventListener("mouseup", stopMouseSidebarResize);
  };

  onCleanup(() => {
    window.removeEventListener("pointermove", moveWindowPointerSidebarResize);
    window.removeEventListener("pointerup", stopWindowPointerSidebarResize);
    window.removeEventListener("pointercancel", stopWindowPointerSidebarResize);
    window.removeEventListener("mousemove", moveMouseSidebarResize);
    window.removeEventListener("mouseup", stopMouseSidebarResize);
  });

  const resetSidebarWidth = () => {
    setSidebarWidth(defaultSidebarWidth);
    applySidebarWidth(defaultSidebarWidth, shellRef);
    persistSidebarWidth(defaultSidebarWidth);
  };

  const toggleTheme = () => {
    const next = themeMode() === "dark" ? "light" : "dark";
    setThemeMode(next);
    applyThemeMode(next, shellRef);
    persistThemeMode(next);
  };

  const nudgeSidebarWidth = (direction: -1 | 1) => {
    const next = clamp(sidebarWidth() + direction * 16, minSidebarWidth, maxSidebarWidth);
    setSidebarWidth(next);
    applySidebarWidth(next, shellRef);
    persistSidebarWidth(next);
  };

  return (
    <main
      class="mh-shell"
      classList={{ resizing: resizingSidebar(), "theme-light": themeMode() === "light", "theme-dark": themeMode() === "dark" }}
      ref={shellRef}
      style={{ "--sidebar-width": `${sidebarWidth()}px` } as JSX.CSSProperties}
    >
      <aside class="mh-sidebar" aria-label="MHcode 导航">
        <div class="sidebar-top">
          <div class="brand-mark">M</div>
          <button class="ghost-icon" type="button" title="后退">
            <ArrowLeft size={16} />
          </button>
          <button class="ghost-icon" type="button" title="前进">
            <ArrowRight size={16} />
          </button>
          <button
            class="connection-pill"
            classList={{ ok: activeProviderConnection().ok, warn: !activeProviderConnection().ready }}
            type="button"
            onClick={() => openSettings("models")}
          >
            <RefreshCw size={13} classList={{ spinning: loading() || testingDeepSeek() }} />
            {activeProviderConnection().label}
          </button>
        </div>

        <div class="quick-menu">
          <button type="button" onClick={resetSession} disabled={resettingSession() || sendingMessage()}>
            <MessageSquarePlus size={16} />
            <span>新建任务</span>
            <kbd>Ctrl+N</kbd>
          </button>
          <button type="button" onClick={() => openSettings("index")}>
            <Search size={16} />
            <span>搜索</span>
            <kbd>Ctrl+K</kbd>
          </button>
          <button type="button" onClick={() => openSettings("skills")}>
            <Sparkles size={16} />
            <span>技能</span>
          </button>
        </div>

        <div class="sidebar-tabs">
          <button type="button" class="active">
            <Hash size={13} />
            分组
          </button>
          <button type="button">
            <Folder size={13} />
            项目
          </button>
          <div class="tab-icons" aria-hidden="true">
            <ListFilter size={15} />
            <LayoutList size={15} />
          </div>
        </div>

        <div class="project-row">
          <Folder size={15} />
          <span>MHcodeProject</span>
        </div>

        <div class="session-list">
          <For each={sidebarSessions()}>
            {(session) => (
              <button
                class="session-item"
                classList={{ active: session.active }}
                type="button"
                onClick={session.onClick}
                title={session.title}
              >
                <span class="session-dot" classList={{ empty: !session.dot }} />
                <span>{session.title}</span>
                <small>{session.meta}</small>
              </button>
            )}
          </For>
          <button class="more-sessions" type="button">
            显示更多
          </button>
        </div>

        <div class="sidebar-footer">
          <div class="avatar">A</div>
          <div>
            <strong>Administrator</strong>
            <small>{profile().label} · {profile().budget.cachePolicy}</small>
          </div>
          <button class="ghost-icon" type="button" title="设置" onClick={() => openDrawer("settings")}>
            <Settings size={16} />
          </button>
        </div>
      </aside>

      <div
        class="sidebar-resizer"
        role="separator"
        aria-label="调整侧栏宽度"
        aria-orientation="vertical"
        aria-valuemin={minSidebarWidth}
        aria-valuemax={maxSidebarWidth}
        aria-valuenow={Math.round(sidebarWidth())}
        tabIndex={0}
        title="拖拽调整宽度，双击恢复默认"
        onPointerDown={startSidebarResize}
        onPointerMove={moveSidebarResize}
        onPointerUp={stopSidebarResize}
        onPointerCancel={stopSidebarResize}
        onMouseDown={startMouseSidebarResize}
        onDblClick={resetSidebarWidth}
        onKeyDown={(event) => {
          if (event.key === "ArrowLeft") {
            event.preventDefault();
            nudgeSidebarWidth(-1);
          }
          if (event.key === "ArrowRight") {
            event.preventDefault();
            nudgeSidebarWidth(1);
          }
          if (event.key === "Home") {
            event.preventDefault();
            setSidebarWidth(minSidebarWidth);
            applySidebarWidth(minSidebarWidth, shellRef);
            persistSidebarWidth(minSidebarWidth);
          }
          if (event.key === "End") {
            event.preventDefault();
            setSidebarWidth(maxSidebarWidth);
            applySidebarWidth(maxSidebarWidth, shellRef);
            persistSidebarWidth(maxSidebarWidth);
          }
          if (event.key === "Enter") {
            event.preventDefault();
            resetSidebarWidth();
          }
        }}
      />

      <section class="chat-pane">
        <header class="chat-header">
          <div>
            <strong>新对话</strong>
            <span>{modelName()}</span>
          </div>
          <div class="header-actions">
            <button type="button" onClick={() => openDrawer("cache")}>
              <ShieldCheck size={15} />
              {formatPercent(cacheHitRate(), hasCacheTokens())}
            </button>
            <button
              class="theme-toggle"
              type="button"
              title={themeMode() === "dark" ? "切换浅色主题" : "切换暗色主题"}
              aria-pressed={themeMode() === "light"}
              onClick={toggleTheme}
            >
              <Show when={themeMode() === "dark"} fallback={<Moon size={15} />}>
                <Sun size={15} />
              </Show>
              <span>{themeMode() === "dark" ? "浅色" : "暗色"}</span>
            </button>
            <button type="button" title="设置" aria-label="设置" onClick={() => openDrawer("settings")}>
              <Settings size={15} />
            </button>
          </div>
        </header>

        <div class="error-slot">
          <Show when={error()}>
            <div class="error-strip" role="alert">
              <AlertTriangle size={16} />
              <span>{error()}</span>
            </div>
          </Show>
        </div>

        <section class="chat-scroll" classList={{ empty: messages().length === 0 }} ref={chatScrollRef} aria-live="polite">
          <For
            each={messages()}
            fallback={
              <div class="welcome-state">
                <div class="wire-logo" aria-hidden="true">
                  <span />
                  <span />
                  <span />
                </div>
                <h1>MHcode 准备好了</h1>
                <Show when={!deepSeek().configured}>
                  <button type="button" onClick={() => openDrawer("settings")}>
                    连接 DeepSeek
                  </button>
                </Show>
              </div>
            }
          >
            {(message) => (
              <article
                class="chat-message"
                classList={{
                  user: message.role === "user",
                  assistant: message.role === "assistant",
                  system: message.role === "system",
                  failed: message.failed,
                }}
              >
                <Show when={message.role !== "user"}>
                  <div class="message-avatar">{message.role === "assistant" ? "AI" : "!"}</div>
                </Show>
                <div class="message-bubble">
                  <div class="message-meta">
                    <strong>{messageTitle(message)}</strong>
                    <span>{formatClock(message.createdAt)}</span>
                  </div>
                  <p>{message.content}</p>
                  <Show when={message.reasoning}>
                    <details class="reasoning-block">
                      <summary>reasoning</summary>
                      <p>{message.reasoning}</p>
                    </details>
                  </Show>
                  <Show when={message.usage}>
                    {(messageUsage) => (
                      <div class="usage-line">
                        <span>hit {formatInteger(messageUsage().promptCacheHitTokens)}</span>
                        <span>miss {formatInteger(messageUsage().promptCacheMissTokens)}</span>
                        <span>in {formatInteger(messageUsage().inputTokens)}</span>
                        <span>out {formatInteger(messageUsage().outputTokens)}</span>
                      </div>
                    )}
                  </Show>
                </div>
              </article>
            )}
          </For>
          <Show when={sendingMessage()}>
            <article class="chat-message assistant">
              <div class="message-avatar">AI</div>
              <div class="message-bubble typing-card">
                <span />
                <span />
                <span />
              </div>
            </article>
          </Show>
        </section>

        <section class="composer-dock">
          <div class="composer-box">
            <div class="composer-project">
              <Folder size={14} />
              <button type="button" onClick={() => openSettings("index")}>
                MHcodeProject
              </button>
              <button type="button" class="hash-chip" onClick={() => openSettings("index")}>
                <Hash size={13} />
                {shortHash(contextPreview()?.prefixHash)}
              </button>
            </div>
            <textarea
              value={promptDraft()}
              onInput={(event) => setPromptDraft(event.currentTarget.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  void sendMessage();
                }
              }}
              rows={2}
              spellcheck={false}
              placeholder="向 MHcode 提问，或描述要修改的代码"
            />
            <div class="composer-toolbar">
              <div>
                <button type="button" title="添加上下文" onClick={() => openSettings("index")}>
                  <Plus size={17} />
                </button>
                <button type="button" onClick={() => openSettings("environment")}>
                  <ShieldCheck size={15} />
                  完全访问
                </button>
              </div>
              <div>
                <ModelRouteMenu
                  settings={runtimeSettings()}
                  saving={savingRuntime()}
                  onManage={() => openSettings("models")}
                  onSelect={(providerID, modelID) => void selectModelRoute(providerID, modelID)}
                />
                <ReasoningMenu
                  value={profile().id}
                  options={options()}
                  running={updatingReasoning()}
                  onChange={changeReasoning}
                />
                <button class="send-button" type="button" disabled={!canSend()} onClick={() => void sendMessage()} title="发送">
                  <ArrowUp size={17} />
                </button>
              </div>
            </div>
          </div>
        </section>
      </section>

      <Show when={drawerOpen()}>
        <aside class="settings-screen" aria-label="MHcode 设置">
          <div class="drawer-head">
            <button class="settings-back" type="button" onClick={() => setDrawerOpen(false)}>
              <ArrowLeft size={16} />
              <span>返回工作区</span>
            </button>
            <strong>MHcode 设置</strong>
            <button class="ghost-icon" type="button" title="关闭设置" onClick={() => setDrawerOpen(false)}>
              <X size={16} />
            </button>
          </div>
          <SettingsCenter
            activeCategory={activeSettingsCategory()}
            apiKeyDraft={apiKeyDraft()}
            cacheHealth={cacheHealth()}
            cacheHitRate={cacheHitRate()}
            cacheTarget={cacheTarget()}
            clearingKey={clearingKey()}
            clearKey={clearKey}
            contextPreview={contextPreview()}
            deepSeek={deepSeek()}
            deepSeekSession={deepSeekSession()}
            diagnostics={diagnostics()}
            hasCacheTokens={hasCacheTokens()}
            models={deepSeek().models}
            nudgeSidebarWidth={nudgeSidebarWidth}
            profile={profile()}
            providerKeyDrafts={providerKeyDrafts()}
            reasoningOptions={options()}
            runtimeDraft={activeRuntimeDraft()}
            runtimeDirty={Boolean(runtimeDraft()) || hasProviderKeyDrafts()}
            configFiles={configFiles()}
            clearProviderKey={clearProviderKey}
            saveKey={saveKey}
            saveProviderKey={saveProviderKey}
            saveRuntime={saveRuntime}
            clearingProviderID={clearingProviderID()}
            savingProviderID={savingProviderID()}
            savingKey={savingKey()}
            savingRuntime={savingRuntime()}
            selectCategory={setActiveSettingsCategory}
            sessionHasCacheTokens={sessionHasCacheTokens()}
            setAPIKeyDraft={setAPIKeyDraft}
            sidebarWidth={sidebarWidth()}
            skills={skillsIndex()}
            snapshots={snapshots()}
            testConnection={testConnection}
            testingDeepSeek={testingDeepSeek()}
            setProviderKeyDraft={setProviderKeyDraft}
            syncProviderModels={syncProviderModels}
            syncingProviderID={syncingProviderID()}
            themeMode={themeMode()}
            toggleTheme={toggleTheme}
            updateReasoning={changeReasoning}
            updateRuntimeDraft={updateRuntimeDraft}
            updatingReasoning={updatingReasoning()}
            resetRuntimeDraft={resetRuntimeDraft}
            resetSidebarWidth={resetSidebarWidth}
            usage={usage()}
          />
        </aside>
      </Show>
    </main>
  );
}

type SettingsCenterProps = {
  activeCategory: SettingsCategory;
  apiKeyDraft: string;
  cacheHealth: WorkbenchState["cacheHealth"];
  cacheHitRate: number;
  cacheTarget: number;
  configFiles: WorkbenchState["configFiles"];
  clearingKey: boolean;
  clearingProviderID: string;
  clearKey: () => void;
  clearProviderKey: (providerID: string) => Promise<void> | void;
  contextPreview: WorkbenchState["contextPreview"] | undefined;
  deepSeek: WorkbenchState["deepSeek"];
  deepSeekSession: DeepSeekSessionState;
  diagnostics: string[];
  hasCacheTokens: boolean;
  models: WorkbenchState["deepSeek"]["models"];
  nudgeSidebarWidth: (direction: -1 | 1) => void;
  profile: WorkbenchState["reasoning"];
  providerKeyDrafts: Record<string, string>;
  reasoningOptions: WorkbenchState["reasoningOptions"];
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveKey: () => void;
  saveRuntime: () => void;
  savingKey: boolean;
  savingRuntime: boolean;
  selectCategory: (category: SettingsCategory) => void;
  sessionHasCacheTokens: boolean;
  setAPIKeyDraft: (value: string) => void;
  sidebarWidth: number;
  skills: WorkbenchState["skillsIndex"];
  snapshots: WorkbenchState["mcpSnapshots"];
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  updateReasoning: (level: ReasoningLevel) => void;
  updatingReasoning: boolean;
  resetRuntimeDraft: () => void;
  resetSidebarWidth: () => void;
  saveProviderKey: (providerID: string) => Promise<void> | void;
  savingProviderID: string;
  setProviderKeyDraft: (providerID: string, value: string) => void;
  syncProviderModels: (providerID: string) => Promise<void> | void;
  syncingProviderID: string;
  testConnection: () => void;
  testingDeepSeek: boolean;
  themeMode: ThemeMode;
  toggleTheme: () => void;
  usage: UsageMetrics;
};

function SettingsCenter(props: SettingsCenterProps) {
  const activeItem = createMemo(() => findSettingsItem(props.activeCategory));

  return (
    <div class="settings-center">
      <nav class="settings-nav" aria-label="设置分类">
        <For each={settingsGroups}>
          {(group) => (
            <div class="settings-nav-group">
              <p>{group.title}</p>
              <For each={group.items}>
                {(item) => (
                  <button
                    type="button"
                    classList={{ active: props.activeCategory === item.id }}
                    aria-current={props.activeCategory === item.id ? "page" : undefined}
                    onClick={() => props.selectCategory(item.id)}
                  >
                    {item.icon}
                    <span>{item.label}</span>
                  </button>
                )}
              </For>
            </div>
          )}
        </For>
      </nav>

      <section class="settings-content" aria-label={activeItem().label}>
        <div class="settings-page-head">
          <span>{settingsGroupTitle(props.activeCategory)}</span>
          <h1>{activeItem().label}</h1>
          <p>{settingsCategoryDescription(props.activeCategory)}</p>
        </div>
        <Switch>
          <Match when={props.activeCategory === "general"}>
            <GeneralSettingsPanel
              apiKeyDraft={props.apiKeyDraft}
              clearingKey={props.clearingKey}
              clearKey={props.clearKey}
              deepSeek={props.deepSeek}
              deepSeekSession={props.deepSeekSession}
              saveKey={props.saveKey}
              savingKey={props.savingKey}
              setAPIKeyDraft={props.setAPIKeyDraft}
              testConnection={props.testConnection}
              testingDeepSeek={props.testingDeepSeek}
            />
          </Match>
          <Match when={props.activeCategory === "appearance"}>
            <AppearanceSettingsPanel
              nudgeSidebarWidth={props.nudgeSidebarWidth}
              resetSidebarWidth={props.resetSidebarWidth}
              sidebarWidth={props.sidebarWidth}
              themeMode={props.themeMode}
              toggleTheme={props.toggleTheme}
            />
          </Match>
          <Match when={props.activeCategory === "config"}>
            <ConfigSettingsPanel
              configFiles={props.configFiles}
              resetRuntimeDraft={props.resetRuntimeDraft}
              runtimeDirty={props.runtimeDirty}
              runtimeDraft={props.runtimeDraft}
              saveRuntime={props.saveRuntime}
              savingRuntime={props.savingRuntime}
              updateRuntimeDraft={props.updateRuntimeDraft}
            />
          </Match>
          <Match when={props.activeCategory === "profile"}>
            <ProfileSettingsPanel
              deepSeek={props.deepSeek}
              profile={props.profile}
              runtimeDraft={props.runtimeDraft}
              themeMode={props.themeMode}
            />
          </Match>
          <Match when={props.activeCategory === "shortcuts"}>
            <ShortcutSettingsPanel />
          </Match>
          <Match when={props.activeCategory === "mcp"}>
            <McpSettingsPanel
              resetRuntimeDraft={props.resetRuntimeDraft}
              runtimeDirty={props.runtimeDirty}
              runtimeDraft={props.runtimeDraft}
              saveRuntime={props.saveRuntime}
              savingRuntime={props.savingRuntime}
              snapshots={props.snapshots}
              updateRuntimeDraft={props.updateRuntimeDraft}
            />
          </Match>
          <Match when={props.activeCategory === "browser"}>
            <BrowserSettingsPanel
              resetRuntimeDraft={props.resetRuntimeDraft}
              runtimeDirty={props.runtimeDirty}
              runtimeDraft={props.runtimeDraft}
              saveRuntime={props.saveRuntime}
              savingRuntime={props.savingRuntime}
              updateRuntimeDraft={props.updateRuntimeDraft}
            />
          </Match>
          <Match when={props.activeCategory === "computer"}>
            <ComputerControlSettingsPanel
              resetRuntimeDraft={props.resetRuntimeDraft}
              runtimeDirty={props.runtimeDirty}
              runtimeDraft={props.runtimeDraft}
              saveRuntime={props.saveRuntime}
              savingRuntime={props.savingRuntime}
              updateRuntimeDraft={props.updateRuntimeDraft}
            />
          </Match>
          <Match when={props.activeCategory === "models"}>
            <ModelSettingsPanel
              clearProviderKey={props.clearProviderKey}
              clearingProviderID={props.clearingProviderID}
              profile={props.profile}
              providerKeyDrafts={props.providerKeyDrafts}
              reasoningOptions={props.reasoningOptions}
              resetRuntimeDraft={props.resetRuntimeDraft}
              runtimeDirty={props.runtimeDirty}
              runtimeDraft={props.runtimeDraft}
              saveProviderKey={props.saveProviderKey}
              saveRuntime={props.saveRuntime}
              savingProviderID={props.savingProviderID}
              savingRuntime={props.savingRuntime}
              setProviderKeyDraft={props.setProviderKeyDraft}
              syncProviderModels={props.syncProviderModels}
              syncingProviderID={props.syncingProviderID}
              updateRuntimeDraft={props.updateRuntimeDraft}
              updateReasoning={props.updateReasoning}
              updatingReasoning={props.updatingReasoning}
            />
          </Match>
          <Match when={props.activeCategory === "skills"}>
            <SkillsSettingsPanel skills={props.skills} snapshots={props.snapshots} />
          </Match>
          <Match when={props.activeCategory === "commands"}>
            <CommandSettingsPanel runtimeDraft={props.runtimeDraft} skills={props.skills} snapshots={props.snapshots} />
          </Match>
          <Match when={props.activeCategory === "index"}>
            <ContextPanel contextPreview={props.contextPreview} />
          </Match>
          <Match when={props.activeCategory === "usage"}>
            <CachePanel
              cacheHealth={props.cacheHealth}
              cacheHitRate={props.cacheHitRate}
              cacheTarget={props.cacheTarget}
              diagnostics={props.diagnostics}
              hasCacheTokens={props.hasCacheTokens}
              session={props.deepSeekSession}
              sessionHasCacheTokens={props.sessionHasCacheTokens}
              usage={props.usage}
            />
          </Match>
          <Match when={props.activeCategory === "environment"}>
            <EnvironmentSettingsPanel
              resetRuntimeDraft={props.resetRuntimeDraft}
              runtimeDirty={props.runtimeDirty}
              runtimeDraft={props.runtimeDraft}
              saveRuntime={props.saveRuntime}
              savingRuntime={props.savingRuntime}
              updateRuntimeDraft={props.updateRuntimeDraft}
            />
          </Match>
          <Match when={props.activeCategory === "git"}>
            <GitSettingsPanel
              resetRuntimeDraft={props.resetRuntimeDraft}
              runtimeDirty={props.runtimeDirty}
              runtimeDraft={props.runtimeDraft}
              saveRuntime={props.saveRuntime}
              savingRuntime={props.savingRuntime}
              updateRuntimeDraft={props.updateRuntimeDraft}
            />
          </Match>
          <Match when={props.activeCategory === "archive"}>
            <ArchiveSettingsPanel />
          </Match>
        </Switch>
      </section>
    </div>
  );
}

function GeneralSettingsPanel(props: {
  apiKeyDraft: string;
  clearingKey: boolean;
  clearKey: () => void;
  deepSeek: WorkbenchState["deepSeek"];
  deepSeekSession: DeepSeekSessionState;
  saveKey: () => void;
  savingKey: boolean;
  setAPIKeyDraft: (value: string) => void;
  testConnection: () => void;
  testingDeepSeek: boolean;
}) {
  return (
    <div class="settings-page-body">
      <PanelSection icon={<KeyRound size={16} />} title="DeepSeek">
        <div class="connection-card">
          <StatusPill
            icon={props.deepSeek.lastCheckStatus === "ok" ? <CheckCircle2 size={14} /> : <AlertTriangle size={14} />}
            label={statusLabel(props.deepSeek.lastCheckStatus)}
            tone={props.deepSeek.lastCheckStatus === "ok" ? "good" : props.deepSeek.lastCheckStatus === "error" ? "bad" : "watch"}
          />
          <strong>{props.deepSeek.configured ? "API Key 已保存" : "API Key 未配置"}</strong>
          <p>{props.deepSeek.lastCheckMessage}</p>
          <code>{props.deepSeek.baseUrl}</code>
        </div>
        <div class="key-row">
          <input
            type="password"
            autocomplete="off"
            spellcheck={false}
            placeholder="sk-..."
            value={props.apiKeyDraft}
            onInput={(event) => props.setAPIKeyDraft(event.currentTarget.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && props.apiKeyDraft.trim()) {
                props.saveKey();
              }
            }}
          />
          <IconButton title="保存 API Key" disabled={!props.apiKeyDraft.trim() || props.savingKey} onClick={props.saveKey}>
            <Save size={15} />
          </IconButton>
          <IconButton title="测试连接" disabled={!props.deepSeek.configured || props.testingDeepSeek} onClick={props.testConnection}>
            <RefreshCw size={15} classList={{ spinning: props.testingDeepSeek }} />
          </IconButton>
          <IconButton title="清除 API Key" danger disabled={!props.deepSeek.configured || props.clearingKey} onClick={props.clearKey}>
            <Trash2 size={15} />
          </IconButton>
        </div>
      </PanelSection>

      <PanelSection icon={<Gauge size={16} />} title="会话">
        <MetricGrid
          items={[
            ["状态", props.deepSeekSession.active ? "运行中" : "待初始化"],
            ["轮次", formatInteger(props.deepSeekSession.turnCount)],
            ["消息", formatInteger(props.deepSeekSession.messageCount)],
            ["前缀", prefixStatusLabel(props.deepSeekSession)],
            ["模型", props.deepSeekSession.model || "下一轮选择"],
            ["思考", thinkingStatusLabel(props.deepSeekSession)],
          ]}
        />
      </PanelSection>
    </div>
  );
}

function AppearanceSettingsPanel(props: {
  nudgeSidebarWidth: (direction: -1 | 1) => void;
  resetSidebarWidth: () => void;
  sidebarWidth: number;
  themeMode: ThemeMode;
  toggleTheme: () => void;
}) {
  return (
    <div class="settings-page-body">
      <PanelSection icon={<Palette size={16} />} title="主题">
        <div class="choice-grid two">
          <button
            type="button"
            classList={{ active: props.themeMode === "dark" }}
            onClick={() => props.themeMode === "light" && props.toggleTheme()}
          >
            <Moon size={15} />
            <span>暗色</span>
          </button>
          <button
            type="button"
            classList={{ active: props.themeMode === "light" }}
            onClick={() => props.themeMode === "dark" && props.toggleTheme()}
          >
            <Sun size={15} />
            <span>浅色</span>
          </button>
        </div>
      </PanelSection>

      <PanelSection icon={<LayoutList size={16} />} title="侧栏">
        <SettingField label="左侧栏宽度" value={`${Math.round(props.sidebarWidth)}px`}>
          <div class="inline-control-row">
            <IconButton title="缩小侧栏" onClick={() => props.nudgeSidebarWidth(-1)}>
              <ArrowLeft size={15} />
            </IconButton>
            <IconButton title="放大侧栏" onClick={() => props.nudgeSidebarWidth(1)}>
              <ArrowRight size={15} />
            </IconButton>
            <button type="button" onClick={props.resetSidebarWidth}>
              恢复默认
            </button>
          </div>
        </SettingField>
      </PanelSection>
    </div>
  );
}

function ConfigSettingsPanel(props: {
  configFiles: WorkbenchState["configFiles"];
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveRuntime: () => void;
  savingRuntime: boolean;
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  resetRuntimeDraft: () => void;
}) {
  return (
    <div class="settings-page-body">
      <SettingsSection title="配置文件">
        <SettingsCard>
          <SettingsRow
            title="运行配置"
            description="沙盒、权限、MCP、浏览器、环境和模型路由都会写入这个 JSON 文件"
            control={<code class="settings-path-value">{props.configFiles.runtimeSettingsPath || "未设置"}</code>}
          />
          <SettingsRow
            title="模型供应商配置"
            description={`多自定义供应商、协议、API 地址、模型列表和上下文窗口保存在 model.providers；当前 ${props.runtimeDraft.model.providers.length} 个供应商`}
            control={<code class="settings-path-value">{props.configFiles.modelProvidersPath || "未设置"}</code>}
          />
          <SettingsRow
            title="密钥存储"
            description="API Key 不写入 JSON 配置文件，只保存配置状态和本地密钥索引"
            control={<code class="settings-path-value">{props.configFiles.secretsStore || "本地 vault"}</code>}
          />
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title="运行策略">
        <div class="settings-toolbar-row">
          <SelectControl value="user" options={[{ value: "user", label: "用户配置" }]} onChange={() => undefined} />
        </div>
        <SettingsCard>
          <SettingsRow
            title="批准策略"
            description="选择 MHcode 何时请求批准"
            control={
              <SelectControl
                value={props.runtimeDraft.approvalPolicy}
                options={approvalOptions}
                onChange={(value) => props.updateRuntimeDraft({ approvalPolicy: value })}
              />
            }
          />
          <SettingsRow
            title="沙盒设置"
            description="选择 MHcode 的命令执行权限"
            control={
              <SelectControl
                value={props.runtimeDraft.sandboxMode}
                options={sandboxOptions}
                onChange={(value) =>
                  props.updateRuntimeDraft({
                    sandboxMode: value,
                    filesystemAccess: value === "read-only" ? "read-only" : props.runtimeDraft.filesystemAccess,
                  })
                }
              />
            }
          />
          <SettingsRow
            title="文件系统权限"
            description="控制读取、写入和跨工作区访问范围"
            control={
              <SelectControl
                value={props.runtimeDraft.filesystemAccess}
                options={filesystemOptions}
                onChange={(value) => props.updateRuntimeDraft({ filesystemAccess: value })}
              />
            }
          />
          <SettingsRow
            title="工具结果"
            description="长输出优先摘要，原文保留在本地引用中"
            control={
              <SelectControl
                value={props.runtimeDraft.toolResultPolicy}
                options={toolResultOptions}
                onChange={(value) => props.updateRuntimeDraft({ toolResultPolicy: value })}
              />
            }
          />
          <SettingsRow
            title="稳定前缀"
            description="控制 Skills、MCP schema 和项目摘要的缓存稳定策略"
            control={
              <SelectControl
                value={props.runtimeDraft.stablePrefixPolicy}
                options={stablePrefixOptions}
                onChange={(value) => props.updateRuntimeDraft({ stablePrefixPolicy: value })}
              />
            }
          />
          <SettingsRow
            title="缓存目标"
            description="低于目标时会在使用统计中提示前缀诊断"
            control={
              <input
                class="settings-input numeric"
                type="number"
                min="0"
                max="100"
                step="0.1"
                value={props.runtimeDraft.cacheTargetPercent}
                onInput={(event) => props.updateRuntimeDraft({ cacheTargetPercent: Number(event.currentTarget.value) })}
              />
            }
          />
        </SettingsCard>
        <RuntimeSaveActions
          dirty={props.runtimeDirty}
          reset={props.resetRuntimeDraft}
          save={props.saveRuntime}
          saving={props.savingRuntime}
        />
      </SettingsSection>

      <SettingsSection title="工作空间依赖项">
        <SettingsCard>
          <SettingsRow title="当前版本" description="如果工具调用失败，请运行诊断或重新安装" control={<span class="settings-muted-value">未安装</span>} />
          <SettingsRow
            title="MHcode 依赖项"
            description="允许 MHcode 安装并提供随附的 Node.js 和 Python 工具"
            control={
              <SwitchControl
                checked={props.runtimeDraft.workspace.dependenciesEnabled}
                onChange={(value) =>
                  props.updateRuntimeDraft({
                    workspace: { ...props.runtimeDraft.workspace, dependenciesEnabled: value },
                  })
                }
              />
            }
          />
          <SettingsRow
            title="诊断 MHcode 工作空间中的问题"
            description="检查当前捆绑包并记录诊断日志"
            control={
              <button class="settings-soft-button" type="button">
                <Search size={14} />
                诊断
              </button>
            }
          />
          <SettingsRow
            title="重置并安装工作空间"
            description="删除本地捆绑包，重新下载后再重新加载工具"
            danger
            control={
              <button class="settings-danger-button" type="button">
                <RefreshCw size={14} />
                重新安装
              </button>
            }
          />
        </SettingsCard>
      </SettingsSection>
    </div>
  );
}

function EnvironmentSettingsPanel(props: {
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveRuntime: () => void;
  savingRuntime: boolean;
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  resetRuntimeDraft: () => void;
}) {
  return (
    <div class="settings-page-body">
      <SettingsSection
        title="选择项目"
        action={
          <button
            class="settings-soft-button"
            type="button"
            onClick={() => props.updateRuntimeDraft({ extraWritableRoots: [...props.runtimeDraft.extraWritableRoots, ""] })}
          >
            添加项目
          </button>
        }
      >
        <div class="settings-project-list">
          <div class="settings-project-row">
            <Folder size={16} />
            <strong>{baseNameFromPath(props.runtimeDraft.workspaceRoot) || "MHcode"}</strong>
            <span>{compactPath(parentPath(props.runtimeDraft.workspaceRoot))}</span>
          </div>
          <For each={props.runtimeDraft.extraWritableRoots}>
            {(root, index) => (
              <div class="settings-project-row">
                <Folder size={16} />
                <strong>{baseNameFromPath(root)}</strong>
                <span>{compactPath(parentPath(root))}</span>
                <button
                  class="settings-square-button"
                  type="button"
                  title="移除项目"
                  onClick={() =>
                    props.updateRuntimeDraft({
                      extraWritableRoots: props.runtimeDraft.extraWritableRoots.filter((_, itemIndex) => itemIndex !== index()),
                    })
                  }
                >
                  <X size={16} />
                </button>
              </div>
            )}
          </For>
        </div>
      </SettingsSection>

      <SettingsSection title="路径">
        <SettingsCard>
          <SettingsRow
            title="工作区根目录"
            description={compactPath(props.runtimeDraft.workspaceRoot)}
            control={
              <input
                class="settings-input row-control"
                value={props.runtimeDraft.workspaceRoot}
                spellcheck={false}
                onInput={(event) => props.updateRuntimeDraft({ workspaceRoot: event.currentTarget.value })}
              />
            }
          />
          <SettingsRow
            title="额外可写目录"
            description={`${props.runtimeDraft.extraWritableRoots.filter(Boolean).length} 项`}
            control={
              <textarea
                class="settings-textarea row-control"
                rows={4}
                spellcheck={false}
                value={props.runtimeDraft.extraWritableRoots.join("\n")}
                placeholder="每行一个绝对路径"
                onInput={(event) =>
                  props.updateRuntimeDraft({
                    extraWritableRoots: event.currentTarget.value.split(/\r?\n/),
                  })
                }
              />
            }
          />
        </SettingsCard>
        <RuntimeSaveActions
          dirty={props.runtimeDirty}
          reset={props.resetRuntimeDraft}
          save={props.saveRuntime}
          saving={props.savingRuntime}
        />
      </SettingsSection>
    </div>
  );
}

function GitSettingsPanel(props: {
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveRuntime: () => void;
  savingRuntime: boolean;
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  resetRuntimeDraft: () => void;
}) {
  const updateGit = (patch: Partial<RuntimeSettings["git"]>) => {
    props.updateRuntimeDraft({ git: { ...props.runtimeDraft.git, ...patch } });
  };

  return (
    <div class="settings-page-body">
      <SettingsSection>
        <SettingsCard>
          <SettingsRow
            title="分支前缀"
            description="在 MHcode 中创建新分支时使用的前缀"
            control={
              <input
                class="settings-input row-control"
                value={props.runtimeDraft.git.branchPrefix}
                onInput={(event) => updateGit({ branchPrefix: event.currentTarget.value })}
              />
            }
          />
          <SettingsRow
            title="拉取请求合并方法"
            description="选择 MHcode 合并拉取请求的方法"
            control={
              <div class="settings-mini-segment">
                <button type="button" classList={{ active: props.runtimeDraft.git.mergeMethod === "merge" }} onClick={() => updateGit({ mergeMethod: "merge" })}>
                  合并
                </button>
                <button type="button" classList={{ active: props.runtimeDraft.git.mergeMethod === "squash" }} onClick={() => updateGit({ mergeMethod: "squash" })}>
                  压缩
                </button>
                <button type="button" classList={{ active: props.runtimeDraft.git.mergeMethod === "rebase" }} onClick={() => updateGit({ mergeMethod: "rebase" })}>
                  变基
                </button>
              </div>
            }
          />
          <SettingsRow
            title="在侧边栏显示 PR 图标"
            description="在侧边栏的对话行中显示 PR 状态图标"
            control={<SwitchControl checked={props.runtimeDraft.git.showPullRequestIcon} onChange={(value) => updateGit({ showPullRequestIcon: value })} />}
          />
          <SettingsRow
            title="始终强制推送"
            description="从 MHcode 推送时使用 --force-with-lease 参数"
            control={<SwitchControl checked={props.runtimeDraft.git.forcePushWithLease} onChange={(value) => updateGit({ forcePushWithLease: value })} />}
          />
          <SettingsRow
            title="创建草稿拉取请求"
            description="从 MHcode 创建 PR 时默认使用草稿拉取请求"
            control={<SwitchControl checked={props.runtimeDraft.git.draftPullRequests} onChange={(value) => updateGit({ draftPullRequests: value })} />}
          />
          <SettingsRow
            title="自动删除旧工作树"
            description="推荐大多数用户启用。仅当你需要手动管理旧工作树和磁盘使用空间时，再关闭此功能。"
            control={<SwitchControl checked={props.runtimeDraft.git.autoDeleteOldWorktrees} onChange={(value) => updateGit({ autoDeleteOldWorktrees: value })} />}
          />
          <SettingsRow
            title="自动删除限制"
            description="自动清理较旧工作树前保留的 MHcode 工作树数量。"
            control={
              <input
                class="settings-input numeric"
                type="number"
                min="1"
                max="99"
                value={props.runtimeDraft.git.worktreeCleanupLimit}
                onInput={(event) => updateGit({ worktreeCleanupLimit: Number(event.currentTarget.value) })}
              />
            }
          />
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title="提交指令"
        action={
          <button class="settings-soft-button" type="button" disabled={!props.runtimeDirty || props.savingRuntime} onClick={props.saveRuntime}>
            保存
          </button>
        }
      >
        <p class="settings-section-note">已添加到提交信息生成提示中</p>
        <textarea
          class="settings-large-textarea"
          value={props.runtimeDraft.git.commitInstructions}
          placeholder="添加提交消息指引..."
          onInput={(event) => updateGit({ commitInstructions: event.currentTarget.value })}
        />
      </SettingsSection>

      <SettingsSection
        title="拉取请求指令"
        action={
          <button class="settings-soft-button" type="button" disabled={!props.runtimeDirty || props.savingRuntime} onClick={props.saveRuntime}>
            保存
          </button>
        }
      >
        <p class="settings-section-note">已添加到 PR 标题/描述生成提示中</p>
        <textarea
          class="settings-large-textarea"
          value={props.runtimeDraft.git.pullRequestInstructions}
          placeholder="添加拉取请求指引..."
          onInput={(event) => updateGit({ pullRequestInstructions: event.currentTarget.value })}
        />
      </SettingsSection>

      <SettingsSection title="工作区路径">
        <SettingsCard>
          <SettingsRow
            title="工作区根目录"
            description={compactPath(props.runtimeDraft.workspaceRoot)}
            control={
              <input
                class="settings-input row-control"
                value={props.runtimeDraft.workspaceRoot}
                spellcheck={false}
                onInput={(event) => props.updateRuntimeDraft({ workspaceRoot: event.currentTarget.value })}
              />
            }
          />
          <SettingsRow
            title="额外可写目录"
            description={`${props.runtimeDraft.extraWritableRoots.length} 项`}
            control={
              <textarea
                class="settings-textarea row-control"
                rows={3}
                spellcheck={false}
                value={props.runtimeDraft.extraWritableRoots.join("\n")}
                placeholder="每行一个绝对路径"
                onInput={(event) =>
                  props.updateRuntimeDraft({
                    extraWritableRoots: event.currentTarget.value
                      .split(/\r?\n/)
                      .map((item) => item.trim())
                      .filter(Boolean),
                  })
                }
              />
            }
          />
        </SettingsCard>
        <RuntimeSaveActions
          dirty={props.runtimeDirty}
          reset={props.resetRuntimeDraft}
          save={props.saveRuntime}
          saving={props.savingRuntime}
        />
      </SettingsSection>
    </div>
  );
}

function ModelSettingsPanel(props: {
  clearProviderKey: (providerID: string) => Promise<void> | void;
  clearingProviderID: string;
  profile: WorkbenchState["reasoning"];
  providerKeyDrafts: Record<string, string>;
  reasoningOptions: WorkbenchState["reasoningOptions"];
  resetRuntimeDraft: () => void;
  runtimeDirty: boolean;
  runtimeDraft: RuntimeSettings;
  saveProviderKey: (providerID: string) => Promise<void> | void;
  saveRuntime: () => void;
  savingProviderID: string;
  savingRuntime: boolean;
  setProviderKeyDraft: (providerID: string, value: string) => void;
  syncProviderModels: (providerID: string) => Promise<void> | void;
  syncingProviderID: string;
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  updateReasoning: (level: ReasoningLevel) => void;
  updatingReasoning: boolean;
}) {
  const [entryMode, setEntryMode] = createSignal<"preset" | "custom">("preset");
  const [activeProviderID, setActiveProviderID] = createSignal(
    props.runtimeDraft.model.selectedProviderId || props.runtimeDraft.model.providers[0]?.id || "",
  );
  const updateModelSettings = (patch: Partial<RuntimeSettings["model"]>) => {
    props.updateRuntimeDraft({ model: { ...props.runtimeDraft.model, ...patch } });
  };
  const activeProvider = createMemo(
    () =>
      props.runtimeDraft.model.providers.find((provider) => provider.id === activeProviderID()) ??
      props.runtimeDraft.model.providers[0],
  );

  createEffect(() => {
    if (!props.runtimeDraft.model.providers.some((provider) => provider.id === activeProviderID())) {
      setActiveProviderID(props.runtimeDraft.model.selectedProviderId || props.runtimeDraft.model.providers[0]?.id || "");
    }
  });

  const updateProvider = (providerID: string, patch: Partial<ModelProviderSetting>) => {
    updateModelSettings({
      providers: props.runtimeDraft.model.providers.map((provider) =>
        provider.id === providerID ? { ...provider, ...patch } : provider,
      ),
    });
  };
  const selectProvider = (providerID: string) => {
    setActiveProviderID(providerID);
    setEntryMode("custom");
  };
  const upsertPresetProvider = (preset: ProviderPreset) => {
    const existing = props.runtimeDraft.model.providers.find((provider) => provider.id === preset.id);
    const provider = providerFromPreset(preset, existing);
    const providers = existing
      ? props.runtimeDraft.model.providers.map((item) => (item.id === preset.id ? provider : item))
      : [...props.runtimeDraft.model.providers, provider];
    updateModelSettings({
      providers,
      selectedProviderId: provider.id,
      selectedModelId: provider.defaultModelId || provider.models[0]?.id || "",
    });
    setActiveProviderID(provider.id);
    setEntryMode("custom");
  };
  const addProvider = () => {
    const provider = createEmptyProvider(props.runtimeDraft.model.providers);
    updateModelSettings({
      providers: [...props.runtimeDraft.model.providers, provider],
      selectedProviderId: provider.id,
      selectedModelId: "",
    });
    setActiveProviderID(provider.id);
    setEntryMode("custom");
  };
  const removeProvider = (providerID: string) => {
    const providers = props.runtimeDraft.model.providers.filter((provider) => provider.id !== providerID);
    updateModelSettings({
      providers,
      selectedProviderId:
        props.runtimeDraft.model.selectedProviderId === providerID ? providers[0]?.id ?? "" : props.runtimeDraft.model.selectedProviderId,
      selectedModelId:
        props.runtimeDraft.model.selectedProviderId === providerID ? providers[0]?.defaultModelId ?? "" : props.runtimeDraft.model.selectedModelId,
    });
  };
  const changeProviderProtocol = (provider: ModelProviderSetting, protocol: string) => {
    updateProvider(provider.id, {
      protocol,
      apiType: defaultAPITypeForProtocol(protocol),
      baseUrl: provider.baseUrl || providerBaseURLPlaceholder(protocol),
      supportsModelFetch: supportsModelFetchForProtocol(protocol),
    });
  };
  const addProviderModel = (provider: ModelProviderSetting) => {
    const nextIndex = provider.models.length + 1;
    const modelID = `model-${nextIndex}`;
    updateProvider(provider.id, {
      models: [
        ...provider.models,
        {
          id: modelID,
          displayName: modelID,
          provider: provider.id,
          contextWindowTokens: provider.contextWindowTokens,
        },
      ],
      defaultModelId: provider.defaultModelId || modelID,
    });
  };
  const updateProviderModel = (provider: ModelProviderSetting, modelIndex: number, patch: Partial<ModelProviderSetting["models"][number]>) => {
    updateProvider(provider.id, {
      models: provider.models.map((model, index) => (index === modelIndex ? { ...model, ...patch } : model)),
    });
  };
  const removeProviderModel = (provider: ModelProviderSetting, modelIndex: number) => {
    const removed = provider.models[modelIndex];
    const models = provider.models.filter((_, index) => index !== modelIndex);
    updateProvider(provider.id, {
      models,
      defaultModelId: provider.defaultModelId === removed?.id ? models[0]?.id ?? "" : provider.defaultModelId,
    });
  };
  const setCurrentRoute = (provider: ModelProviderSetting) => {
    updateModelSettings({
      selectedProviderId: provider.id,
      selectedModelId: provider.defaultModelId || provider.models[0]?.id || "",
    });
  };

  return (
    <div class="settings-page-body">
      <PanelSection icon={<Cpu size={16} />} title="推理强度">
        <div class="settings-inline-card">
          <div>
            <strong>{props.profile.label}</strong>
            <span>{props.profile.description}</span>
          </div>
          <ReasoningMenu
            value={props.profile.id}
            options={props.reasoningOptions}
            running={props.updatingReasoning}
            onChange={props.updateReasoning}
          />
        </div>
        <MetricGrid
          items={[
            ["工具预算", `${formatInteger(props.profile.budget.maxToolCalls)} 次`],
            ["上下文策略", props.profile.budget.contextPolicy],
            ["缓存策略", props.profile.budget.cachePolicy],
            ["规划器", props.profile.budget.planner ? "开启" : "关闭"],
          ]}
        />
      </PanelSection>

      <SettingsSection
        title="供应商接入"
        action={
          <button class="settings-soft-button" type="button" onClick={addProvider}>
            <Plus size={15} />
            添加模型服务
          </button>
        }
      >
        <div class="provider-onboarding">
          <div class="settings-mini-segment">
            <button type="button" classList={{ active: entryMode() === "preset" }} onClick={() => setEntryMode("preset")}>
              推荐预设
            </button>
            <button type="button" classList={{ active: entryMode() === "custom" }} onClick={() => setEntryMode("custom")}>
              自定义供应商
            </button>
          </div>

          <Show when={entryMode() === "preset"}>
            <div class="provider-preset-grid">
              <For each={providerPresets}>
                {(preset) => {
                  const configured = createMemo(() =>
                    props.runtimeDraft.model.providers.some((provider) => provider.id === preset.id),
                  );
                  return (
                    <button
                      class="provider-preset-card"
                      classList={{ active: configured() }}
                      type="button"
                      onClick={() => upsertPresetProvider(preset)}
                    >
                      <strong>{preset.name}</strong>
                      <span>{runtimeLabel(providerProtocolOptions, preset.protocol)}</span>
                      <small>{preset.note}</small>
                    </button>
                  );
                }}
              </For>
            </div>
          </Show>

          <Show when={entryMode() === "custom" ? activeProvider() : undefined}>
            {(provider) => (
              <div class="provider-editor">
                <div class="provider-editor-head">
                  <div>
                    <strong>{provider().name || "自定义供应商"}</strong>
                    <span>{provider().id}</span>
                  </div>
                  <div class="settings-row-actions">
                    <StatusPill
                      icon={provider().lastSyncStatus === "ok" ? <CheckCircle2 size={13} /> : <AlertTriangle size={13} />}
                      label={provider().lastSyncStatus === "ok" ? "连接正常" : provider().lastSyncStatus === "error" ? "连接异常" : "待测试"}
                      tone={provider().lastSyncStatus === "ok" ? "good" : provider().lastSyncStatus === "error" ? "bad" : "neutral"}
                    />
                    <SwitchControl checked={provider().enabled} onChange={(value) => updateProvider(provider().id, { enabled: value })} />
                  </div>
                </div>
                <SettingsCard>
                  <SettingsRow
                    title="自定义供应商名称"
                    description="设置页、模型菜单和路由列表中展示的名称"
                    control={
                      <input
                        class="settings-input row-control"
                        value={provider().name}
                        onInput={(event) => updateProvider(provider().id, { name: event.currentTarget.value })}
                      />
                    }
                  />
                  <SettingsRow
                    title="接入协议"
                    description="OpenAI 兼容协议支持自动获取上游模型"
                    control={
                      <SelectControl
                        value={provider().protocol}
                        options={providerProtocolOptions}
                        onChange={(value) => changeProviderProtocol(provider(), value)}
                      />
                    }
                  />
                  <SettingsRow
                    title="API 类型"
                    description="后续路由会按 API 类型选择请求格式"
                    control={
                      <SelectControl
                        value={provider().apiType}
                        options={providerAPITypeOptions}
                        onChange={(value) => updateProvider(provider().id, { apiType: value })}
                      />
                    }
                  />
                  <SettingsRow
                    title="API 地址"
                    description={providerBaseURLHint(provider().protocol)}
                    control={
                      <input
                        class="settings-input row-control"
                        value={provider().baseUrl}
                        spellcheck={false}
                        placeholder={providerBaseURLPlaceholder(provider().protocol)}
                        onInput={(event) => updateProvider(provider().id, { baseUrl: event.currentTarget.value })}
                      />
                    }
                  />
                  <SettingsRow
                    title="额外请求头（可选）"
                    description="用于 OpenRouter 等要求来源标识的兼容转发。一行一个 Header: value；API Key 仍建议通过上方密钥保存。"
                    control={
                      <textarea
                        class="settings-textarea row-control provider-compat-textarea"
                        value={provider().extraHeaders ?? ""}
                        spellcheck={false}
                        placeholder={"HTTP-Referer: https://example.com\nX-Title: MHcode"}
                        onInput={(event) => updateProvider(provider().id, { extraHeaders: event.currentTarget.value })}
                      />
                    }
                  />
                  <SettingsRow
                    title="额外请求体（可选）"
                    description="合并到聊天请求体顶层 JSON；model、messages、stream 等核心字段仍由 MHcode 控制。"
                    control={
                      <textarea
                        class="settings-textarea row-control provider-compat-textarea"
                        value={provider().extraBodyJson ?? ""}
                        spellcheck={false}
                        placeholder={'{\n  "enable_thinking": true\n}'}
                        onInput={(event) => updateProvider(provider().id, { extraBodyJson: event.currentTarget.value })}
                      />
                    }
                  />
                  <SettingsRow
                    title="密钥"
                    description={provider().apiKeyConfigured ? "密钥已保存到该供应商的本地凭据项，不写入 JSON 明文" : "输入后点保存设置或保存密钥；本地兼容服务可留空"}
                    control={
                      <div class="settings-row-stack">
                        <input
                          class="settings-input row-control"
                          type="password"
                          value={props.providerKeyDrafts[provider().id] ?? ""}
                          placeholder={provider().apiKeyConfigured ? "输入新 Key 可覆盖" : "sk-..."}
                          onInput={(event) => props.setProviderKeyDraft(provider().id, event.currentTarget.value)}
                        />
                        <div class="settings-row-actions">
                          <button
                            class="settings-soft-button"
                            type="button"
                            disabled={props.savingProviderID === provider().id}
                            onClick={() => void props.saveProviderKey(provider().id)}
                          >
                            <Save size={14} />
                            {props.savingProviderID === provider().id ? "保存中" : "保存密钥"}
                          </button>
                          <button
                            class="settings-soft-button"
                            type="button"
                            disabled={!provider().apiKeyConfigured || props.clearingProviderID === provider().id}
                            onClick={() => void props.clearProviderKey(provider().id)}
                          >
                            清除密钥
                          </button>
                        </div>
                      </div>
                    }
                  />
                  <SettingsRow
                    title="测试并获取模型"
                    description={provider().lastSyncMessage || "会使用上方 API 地址与密钥确认连接，并用返回结果填充模型列表。"}
                    control={
                      <button
                        class="settings-soft-button"
                        type="button"
                        disabled={!provider().supportsModelFetch || props.syncingProviderID === provider().id}
                        onClick={() => void props.syncProviderModels(provider().id)}
                      >
                        <RefreshCw size={14} classList={{ spinning: props.syncingProviderID === provider().id }} />
                        {props.syncingProviderID === provider().id ? "获取中" : "测试并获取"}
                      </button>
                    }
                  />
                  <SettingsRow
                    title="余额查询 URL（可选）"
                    description="仅保存链接；后续可接入供应商余额查询视图"
                    control={
                      <input
                        class="settings-input row-control"
                        value={provider().balanceUrl}
                        spellcheck={false}
                        placeholder="https://..."
                        onInput={(event) => updateProvider(provider().id, { balanceUrl: event.currentTarget.value })}
                      />
                    }
                  />
                  <SettingsRow
                    title="上下文窗口"
                    description="该模型服务在上下文中保留的最大 token 数；填 0 表示使用模型服务默认值"
                    control={
                      <input
                        class="settings-input row-control numeric"
                        type="number"
                        min="0"
                        step="1"
                        value={provider().contextWindowTokens}
                        onInput={(event) =>
                          updateProvider(provider().id, { contextWindowTokens: Math.max(0, Number(event.currentTarget.value) || 0) })
                        }
                      />
                    }
                  />
                  <SettingsRow
                    title="默认模型"
                    description="该提供商被选中时优先使用的模型"
                    control={
                      <SelectControl
                        value={provider().defaultModelId}
                        options={[
                          { value: "", label: "自动选择" },
                          ...provider().models.map((model) => ({ value: model.id, label: model.displayName || model.id })),
                        ]}
                        onChange={(value) => updateProvider(provider().id, { defaultModelId: value })}
                      />
                    }
                  />
                  <SettingsRow
                    title="当前路由"
                    description={props.runtimeDraft.model.selectedProviderId === provider().id ? "当前会话会优先使用该供应商" : "保存后后续会话可使用该供应商"}
                    control={
                      <div class="settings-row-actions">
                        <button class="settings-soft-button" type="button" onClick={() => setCurrentRoute(provider())}>
                          设为当前
                        </button>
                        <Show when={provider().id !== "deepseek"}>
                          <IconButton title="删除供应商" danger onClick={() => removeProvider(provider().id)}>
                            <Trash2 size={14} />
                          </IconButton>
                        </Show>
                      </div>
                    }
                  />
                </SettingsCard>

                <div class="provider-model-editor">
                  <div class="provider-model-editor-head">
                    <div>
                      <strong>模型列表</strong>
                      <span>{provider().models.length} 个模型 · 上下文 {formatTokenWindow(provider().contextWindowTokens)}</span>
                    </div>
                    <div class="settings-row-actions">
                      <button class="settings-soft-button" type="button" onClick={() => addProviderModel(provider())}>
                        <Plus size={14} />
                        添加模型
                      </button>
                      <button
                        class="settings-soft-button"
                        type="button"
                        disabled={!provider().supportsModelFetch || props.syncingProviderID === provider().id}
                        onClick={() => void props.syncProviderModels(provider().id)}
                      >
                        <RefreshCw size={14} classList={{ spinning: props.syncingProviderID === provider().id }} />
                        从上游获取
                      </button>
                    </div>
                  </div>
                  <For each={provider().models} fallback={<p class="empty-line">暂无模型，可手动添加或从上游获取。</p>}>
                    {(model, index) => (
                      <div class="provider-model-row">
                        <input
                          class="settings-input"
                          value={model.id}
                          spellcheck={false}
                          placeholder="模型 ID"
                          onInput={(event) =>
                            updateProviderModel(provider(), index(), {
                              id: event.currentTarget.value,
                              displayName: model.displayName || event.currentTarget.value,
                            })
                          }
                        />
                        <input
                          class="settings-input"
                          value={model.displayName}
                          placeholder="显示名称"
                          onInput={(event) => updateProviderModel(provider(), index(), { displayName: event.currentTarget.value })}
                        />
                        <input
                          class="settings-input numeric"
                          type="number"
                          min="0"
                          step="1"
                          value={model.contextWindowTokens}
                          title="模型上下文窗口，0 表示服务默认"
                          onInput={(event) =>
                            updateProviderModel(provider(), index(), {
                              contextWindowTokens: Math.max(0, Number(event.currentTarget.value) || 0),
                            })
                          }
                        />
                        <IconButton title="删除模型" danger onClick={() => removeProviderModel(provider(), index())}>
                          <Trash2 size={14} />
                        </IconButton>
                      </div>
                    )}
                  </For>
                </div>
              </div>
            )}
          </Show>
        </div>
        <RuntimeSaveActions
          dirty={props.runtimeDirty}
          reset={props.resetRuntimeDraft}
          save={props.saveRuntime}
          saving={props.savingRuntime}
        />
      </SettingsSection>

      <SettingsSection title="已配置供应商">
        <div class="provider-grid">
          <For each={props.runtimeDraft.model.providers}>
            {(provider) => (
              <button
                class="provider-summary-card"
                classList={{ active: activeProviderID() === provider.id }}
                type="button"
                onClick={() => selectProvider(provider.id)}
              >
                <div class="provider-card-head">
                  <div>
                    <strong>{provider.name}</strong>
                    <span>
                      {runtimeLabel(providerProtocolOptions, provider.protocol)} · {runtimeLabel(providerAPITypeOptions, provider.apiType)}
                    </span>
                  </div>
                  <StatusPill
                    icon={provider.lastSyncStatus === "ok" ? <CheckCircle2 size={13} /> : <Database size={13} />}
                    label={`${provider.models.length} 模型`}
                    tone={provider.lastSyncStatus === "ok" ? "good" : provider.lastSyncStatus === "error" ? "bad" : "neutral"}
                  />
                </div>
                <div class="provider-status-line">
                  <StatusPill
                    icon={<KeyRound size={13} />}
                    label={provider.apiKeyConfigured ? "Key 已保存" : "未保存 Key"}
                    tone={provider.apiKeyConfigured || provider.protocol === "local" ? "good" : "watch"}
                  />
                  <StatusPill icon={<Hash size={13} />} label={`上下文 ${formatTokenWindow(provider.contextWindowTokens)}`} tone="neutral" />
                  <StatusPill
                    icon={<CheckCircle2 size={13} />}
                    label={props.runtimeDraft.model.selectedProviderId === provider.id ? "当前路由" : provider.enabled ? "已启用" : "未启用"}
                    tone={props.runtimeDraft.model.selectedProviderId === provider.id || provider.enabled ? "good" : "neutral"}
                  />
                </div>
              </button>
            )}
          </For>
        </div>
      </SettingsSection>

      <PanelSection icon={<Database size={16} />} title="当前模型列表">
        <div class="item-list">
          <For
            each={props.runtimeDraft.model.providers.flatMap((provider) =>
              provider.models.map((model) => ({ ...model, providerName: provider.name })),
            )}
            fallback={<p class="empty-line">暂无模型，先点击“获取模型”。</p>}
          >
            {(model) => (
              <div class="model-row">
                <strong>{model.displayName || model.id}</strong>
                <code>
                  {model.providerName} · {model.id} · 上下文 {formatTokenWindow(model.contextWindowTokens)}
                </code>
              </div>
            )}
          </For>
        </div>
      </PanelSection>
    </div>
  );
}

function SkillsSettingsPanel(props: {
  skills: WorkbenchState["skillsIndex"];
  snapshots: WorkbenchState["mcpSnapshots"];
}) {
  return (
    <div class="settings-page-body">
      <PanelSection icon={<Sparkles size={16} />} title="Skills">
        <div class="item-list">
          <For each={props.skills} fallback={<p class="empty-line">未发现 Skill</p>}>
            {(skill) => (
              <div class="resource-row">
                <strong>{skill.name}</strong>
                <span>{skill.summary}</span>
                <code>{shortHash(skill.sha256)}</code>
              </div>
            )}
          </For>
        </div>
      </PanelSection>
      <PanelSection icon={<FileText size={16} />} title="加载链路">
        <div class="route-stack">
          <RouteStep icon={<Sparkles size={15} />} title="Skills index" detail={`${formatInteger(props.skills.length)} loaded`} />
          <RouteStep icon={<Network size={15} />} title="MCP schema" detail={`${formatInteger(props.snapshots.length)} snapshot`} />
          <RouteStep icon={<FileText size={15} />} title="Summary-first results" detail="raw output stays local" />
        </div>
      </PanelSection>
    </div>
  );
}

function McpSettingsPanel(props: {
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveRuntime: () => void;
  savingRuntime: boolean;
  snapshots: WorkbenchState["mcpSnapshots"];
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  resetRuntimeDraft: () => void;
}) {
  const updateServers = (servers: MCPServerSetting[]) => {
    props.updateRuntimeDraft({ mcp: { ...props.runtimeDraft.mcp, servers } });
  };
  const updateServer = (id: string, patch: Partial<MCPServerSetting>) => {
    updateServers(props.runtimeDraft.mcp.servers.map((server) => (server.id === id ? { ...server, ...patch } : server)));
  };
  const addServer = () => {
    const index = props.runtimeDraft.mcp.servers.length + 1;
    updateServers([
      ...props.runtimeDraft.mcp.servers,
      {
        id: `mcp-${index}`,
        name: `MCP ${index}`,
        command: "",
        args: [],
        env: [],
        passEnvironment: [],
        workingDirectory: "",
        enabled: false,
        toolResultPolicy: "summary-first",
      },
    ]);
  };
  const removeServer = (id: string) => {
    updateServers(props.runtimeDraft.mcp.servers.filter((server) => server.id !== id));
  };

  return (
    <div class="settings-page-body">
      <SettingsSection
        title="服务器"
        action={
          <button class="settings-soft-button" type="button" onClick={addServer}>
            <Plus size={15} />
            添加服务器
          </button>
        }
      >
        <SettingsCard>
          <For each={props.runtimeDraft.mcp.servers} fallback={<SettingsRow title="暂无服务器" description="添加一个 MCP 服务器后即可配置命令和环境变量" />}>
            {(server) => (
              <SettingsRow
                title={server.name || server.id}
                description={`${server.command || "未设置命令"} · ${server.enabled ? "已启用" : "已停用"}`}
                control={
                  <div class="settings-row-actions">
                    <IconButton title="删除服务器" danger onClick={() => removeServer(server.id)}>
                      <Trash2 size={14} />
                    </IconButton>
                    <SwitchControl checked={server.enabled} onChange={(value) => updateServer(server.id, { enabled: value })} />
                  </div>
                }
              />
            )}
          </For>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title="服务器详情">
        <For each={props.runtimeDraft.mcp.servers} fallback={<p class="settings-empty-box">尚无可编辑的服务器配置</p>}>
          {(server) => (
            <SettingsCard>
              <SettingsRow
                title="名称"
                description={server.id}
                control={
                  <input
                    class="settings-input row-control"
                    value={server.name}
                    onInput={(event) => updateServer(server.id, { name: event.currentTarget.value })}
                  />
                }
              />
              <SettingsRow
                title="启动命令"
                description="例如 npx、node、python 或绝对路径"
                control={
                  <input
                    class="settings-input row-control"
                    value={server.command}
                    spellcheck={false}
                    onInput={(event) => updateServer(server.id, { command: event.currentTarget.value })}
                  />
                }
              />
              <SettingsRow
                title="参数"
                description="每行一个参数，顺序会保持稳定"
                control={
                  <textarea
                    class="settings-textarea row-control"
                    rows={3}
                    spellcheck={false}
                    value={server.args.join("\n")}
                    onInput={(event) => updateServer(server.id, { args: event.currentTarget.value.split(/\r?\n/) })}
                  />
                }
              />
              <SettingsRow
                title="工作目录"
                description="留空时使用当前工作区"
                control={
                  <input
                    class="settings-input row-control"
                    value={server.workingDirectory}
                    spellcheck={false}
                    onInput={(event) => updateServer(server.id, { workingDirectory: event.currentTarget.value })}
                  />
                }
              />
              <SettingsRow
                title="环境变量"
                description="每行 KEY=VALUE"
                control={
                  <textarea
                    class="settings-textarea row-control"
                    rows={3}
                    spellcheck={false}
                    value={server.env.map((item) => `${item.key}=${item.value}`).join("\n")}
                    onInput={(event) => updateServer(server.id, { env: parseEnvLines(event.currentTarget.value) })}
                  />
                }
              />
              <SettingsRow
                title="透传环境变量"
                description="每行一个变量名"
                control={
                  <textarea
                    class="settings-textarea row-control"
                    rows={3}
                    spellcheck={false}
                    value={server.passEnvironment.join("\n")}
                    onInput={(event) => updateServer(server.id, { passEnvironment: event.currentTarget.value.split(/\r?\n/) })}
                  />
                }
              />
              <SettingsRow
                title="工具结果策略"
                description="控制 MCP 工具输出如何进入上下文"
                control={
                  <SelectControl
                    value={server.toolResultPolicy}
                    options={toolResultOptions}
                    onChange={(value) => updateServer(server.id, { toolResultPolicy: value })}
                  />
                }
              />
            </SettingsCard>
          )}
        </For>
        <RuntimeSaveActions
          dirty={props.runtimeDirty}
          reset={props.resetRuntimeDraft}
          save={props.saveRuntime}
          saving={props.savingRuntime}
        />
      </SettingsSection>

      <SettingsSection title="当前快照">
        <div class="item-list">
          <For each={props.snapshots} fallback={<p class="settings-empty-box">尚未记录 MCP schema 快照</p>}>
            {(snapshot) => (
              <div class="resource-row">
                <strong>{snapshot.server}</strong>
                <span>{snapshot.tools.map((tool) => tool.name).join(", ") || "暂无工具"}</span>
                <code>{shortHash(snapshot.toolsHash)}</code>
              </div>
            )}
          </For>
        </div>
      </SettingsSection>
    </div>
  );
}

function BrowserSettingsPanel(props: {
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveRuntime: () => void;
  savingRuntime: boolean;
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  resetRuntimeDraft: () => void;
}) {
  const updateBrowser = (patch: Partial<RuntimeSettings["browser"]>) => {
    props.updateRuntimeDraft({ browser: { ...props.runtimeDraft.browser, ...patch } });
  };

  return (
    <div class="settings-page-body">
      <SettingsSection>
        <SettingsCard>
          <SettingsRow
            icon={<Globe2 size={28} />}
            title="浏览器"
            description="允许 MHcode 控制内置浏览器"
            control={<SwitchControl checked={props.runtimeDraft.browser.enabled} onChange={(value) => updateBrowser({ enabled: value })} />}
          />
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title="General">
        <SettingsCard>
          <SettingsRow
            title="Default local URL open destination"
            description="Where localhost and loopback URLs open by default"
            control={
              <SelectControl
                value={props.runtimeDraft.browser.defaultLocalUrlDestination}
                options={[
                  { value: "mhcode", label: "MHcode 内置浏览器" },
                  { value: "system", label: "系统默认浏览器" },
                  { value: "ask", label: "每次询问" },
                ]}
                onChange={(value) => updateBrowser({ defaultLocalUrlDestination: value })}
              />
            }
          />
          <SettingsRow
            title="浏览数据"
            description="清除应用内浏览器中的网站数据和缓存"
            control={
              <SelectControl
                value={props.runtimeDraft.browser.clearDataPolicy}
                options={[
                  { value: "ask", label: "清理前询问" },
                  { value: "session", label: "关闭会话时清理" },
                  { value: "all", label: "立即清理全部" },
                  { value: "never", label: "不自动清理" },
                ]}
                onChange={(value) => updateBrowser({ clearDataPolicy: value })}
              />
            }
          />
          <SettingsRow
            title="批注截图"
            description="截图可帮助 MHcode 更好地理解并处理评论，但会增加套餐用量"
            control={
              <SelectControl
                value={props.runtimeDraft.browser.screenshotAnnotations}
                options={[
                  { value: "always", label: "始终包含" },
                  { value: "ask", label: "需要时询问" },
                  { value: "never", label: "从不包含" },
                ]}
                onChange={(value) => updateBrowser({ screenshotAnnotations: value })}
              />
            }
          />
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title="Autofill and passwords">
        <SettingsCard>
          <SettingsRow
            title="密码管理器"
            description="允许内置浏览器保存和填充密码"
            control={<SwitchControl checked={props.runtimeDraft.browser.passwordManagerEnabled} onChange={(value) => updateBrowser({ passwordManagerEnabled: value })} />}
          />
          <SettingsRow
            title="联系信息"
            description="允许保存地址、电话号码和电子邮件地址"
            control={<SwitchControl checked={props.runtimeDraft.browser.autofillContactEnabled} onChange={(value) => updateBrowser({ autofillContactEnabled: value })} />}
          />
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title="权限">
        <SettingsCard>
          <SettingsRow
            title="网站设置"
            description="在 MHcode 的浏览器中控制摄像头、麦克风和剪贴板权限"
            control={<span class="settings-muted-value">{props.runtimeDraft.browser.sitePermissions.length} 个站点</span>}
          />
          <SettingsRow
            title="审批"
            description="选择是否让 MHcode 在打开网站前先请求批准"
            control={
              <SelectControl
                value={props.runtimeDraft.approvalPolicy}
                options={approvalOptions}
                onChange={(value) => props.updateRuntimeDraft({ approvalPolicy: value })}
              />
            }
          />
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title="网站权限"
        action={
          <button
            class="settings-soft-button"
            type="button"
            onClick={() =>
              updateBrowser({
                sitePermissions: [
                  ...props.runtimeDraft.browser.sitePermissions,
                  { origin: "https://example.com", camera: "ask", microphone: "ask", clipboard: "ask" },
                ],
              })
            }
          >
            <Plus size={15} />
            添加
          </button>
        }
      >
        <SettingsCard>
          <For each={props.runtimeDraft.browser.sitePermissions} fallback={<SettingsRow title="尚无网站专属权限" description="添加站点后可以单独控制摄像头、麦克风和剪贴板" />}>
            {(permission, index) => (
              <SettingsRow
                title={permission.origin || "新站点"}
                description={`摄像头 ${permissionLabel(permission.camera)} · 麦克风 ${permissionLabel(permission.microphone)} · 剪贴板 ${permissionLabel(permission.clipboard)}`}
                control={
                  <div class="settings-row-stack">
                    <input
                      class="settings-input row-control"
                      value={permission.origin}
                      spellcheck={false}
                      onInput={(event) => {
                        const sitePermissions = [...props.runtimeDraft.browser.sitePermissions];
                        sitePermissions[index()] = { ...permission, origin: event.currentTarget.value };
                        updateBrowser({ sitePermissions });
                      }}
                    />
                    <div class="settings-row-actions">
                      <SelectControl
                        value={permission.camera}
                        options={sitePermissionOptions}
                        onChange={(value) => {
                          const sitePermissions = [...props.runtimeDraft.browser.sitePermissions];
                          sitePermissions[index()] = { ...permission, camera: value };
                          updateBrowser({ sitePermissions });
                        }}
                      />
                      <SelectControl
                        value={permission.microphone}
                        options={sitePermissionOptions}
                        onChange={(value) => {
                          const sitePermissions = [...props.runtimeDraft.browser.sitePermissions];
                          sitePermissions[index()] = { ...permission, microphone: value };
                          updateBrowser({ sitePermissions });
                        }}
                      />
                      <SelectControl
                        value={permission.clipboard}
                        options={sitePermissionOptions}
                        onChange={(value) => {
                          const sitePermissions = [...props.runtimeDraft.browser.sitePermissions];
                          sitePermissions[index()] = { ...permission, clipboard: value };
                          updateBrowser({ sitePermissions });
                        }}
                      />
                      <IconButton
                        title="移除站点权限"
                        danger
                        onClick={() =>
                          updateBrowser({
                            sitePermissions: props.runtimeDraft.browser.sitePermissions.filter((_, itemIndex) => itemIndex !== index()),
                          })
                        }
                      >
                        <Trash2 size={14} />
                      </IconButton>
                    </div>
                  </div>
                }
              />
            )}
          </For>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title="开发者模式">
        <div class="settings-risk-card">
          <strong>风险升高</strong>
          <SettingsRow
            title="启用完整 CDP 访问权限"
            description="允许 MHcode 在已连接的 Browser Use 会话中使用完整的 Chrome 开发者工具协议访问权限。完整的 CDP 访问权限会让 MHcode 检查和控制敏感的浏览器内部功能，可能使你的数据面临风险。"
            control={<SwitchControl checked={props.runtimeDraft.browser.developerCdpAccess} onChange={(value) => updateBrowser({ developerCdpAccess: value })} />}
          />
        </div>
        <RuntimeSaveActions
          dirty={props.runtimeDirty}
          reset={props.resetRuntimeDraft}
          save={props.saveRuntime}
          saving={props.savingRuntime}
        />
      </SettingsSection>
    </div>
  );
}

function ComputerControlSettingsPanel(props: {
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveRuntime: () => void;
  savingRuntime: boolean;
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  resetRuntimeDraft: () => void;
}) {
  const updateComputerControl = (patch: Partial<RuntimeSettings["computerControl"]>) => {
    props.updateRuntimeDraft({ computerControl: { ...props.runtimeDraft.computerControl, ...patch } });
  };

  return (
    <div class="settings-page-body">
      <SettingsSection title="控制">
        <SettingsCard>
          <SettingsRow
            icon={<Monitor size={30} />}
            title="任意应用"
            description="允许 MHcode 控制您电脑上的应用"
            control={
              <SwitchControl
                checked={props.runtimeDraft.computerControl.anyAppEnabled}
                onChange={(value) => updateComputerControl({ anyAppEnabled: value })}
              />
            }
          />
          <SettingsRow
            icon={<Globe2 size={30} />}
            title="Google Chrome"
            description="浏览器扩展程序未连接"
            control={
              <SwitchControl
                checked={props.runtimeDraft.computerControl.chromeEnabled}
                onChange={(value) => updateComputerControl({ chromeEnabled: value })}
              />
            }
          />
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title="始终允许的应用">
        <SettingsCard>
          <SettingsRow
            title="应用列表"
            description="每行一个进程名或应用名"
            control={
              <textarea
                class="settings-textarea row-control"
                rows={4}
                spellcheck={false}
                value={props.runtimeDraft.computerControl.alwaysAllowedApps.join("\n")}
                placeholder="Code.exe&#10;chrome.exe"
                onInput={(event) =>
                  updateComputerControl({
                    alwaysAllowedApps: event.currentTarget.value.split(/\r?\n/),
                  })
                }
              />
            }
          />
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title="命令执行">
        <SettingsCard>
          <SettingsRow
            title="允许命令执行"
            description="允许 MHcode 调用本地命令和脚本"
            control={<SwitchControl checked={props.runtimeDraft.shellAccess} onChange={(value) => props.updateRuntimeDraft({ shellAccess: value })} />}
          />
          <SettingsRow
            title="允许破坏性操作"
            description="启用后允许删除、覆盖或移动本地文件等高风险操作"
            danger
            control={<SwitchControl checked={props.runtimeDraft.allowDestructiveOps} onChange={(value) => props.updateRuntimeDraft({ allowDestructiveOps: value })} />}
          />
          <SettingsRow
            title="命令超时"
            description="单个命令允许运行的最长时间"
            control={
              <input
                class="settings-input numeric"
                type="number"
                min="5"
                max="3600"
                value={props.runtimeDraft.maxCommandSeconds}
                onInput={(event) => props.updateRuntimeDraft({ maxCommandSeconds: Number(event.currentTarget.value) })}
              />
            }
          />
        </SettingsCard>
        <RuntimeSaveActions
          dirty={props.runtimeDirty}
          reset={props.resetRuntimeDraft}
          save={props.saveRuntime}
          saving={props.savingRuntime}
        />
      </SettingsSection>
    </div>
  );
}

function CommandSettingsPanel(props: {
  runtimeDraft: RuntimeSettings;
  skills: WorkbenchState["skillsIndex"];
  snapshots: WorkbenchState["mcpSnapshots"];
}) {
  return (
    <div class="settings-page-body">
      <PanelSection icon={<Command size={16} />} title="命令">
        <MetricGrid
          items={[
            ["Shell", props.runtimeDraft.shellAccess ? "允许" : "关闭"],
            ["网络", props.runtimeDraft.networkAccess ? "允许" : "关闭"],
            ["命令超时", `${props.runtimeDraft.maxCommandSeconds}s`],
            ["审批", runtimeLabel(approvalOptions, props.runtimeDraft.approvalPolicy)],
          ]}
        />
      </PanelSection>
      <PanelSection icon={<FileText size={16} />} title="执行链路">
        <div class="route-stack">
          <RouteStep icon={<Bot size={15} />} title="DeepSeek official" detail="primary route" />
          <RouteStep icon={<Sparkles size={15} />} title="Skills index" detail={`${formatInteger(props.skills.length)} loaded`} />
          <RouteStep icon={<Network size={15} />} title="MCP schema" detail={`${formatInteger(props.snapshots.length)} snapshot`} />
          <RouteStep icon={<FileText size={15} />} title="Summary-first results" detail="raw output stays local" />
        </div>
      </PanelSection>
    </div>
  );
}

function ProfileSettingsPanel(props: {
  deepSeek: WorkbenchState["deepSeek"];
  profile: WorkbenchState["reasoning"];
  runtimeDraft: RuntimeSettings;
  themeMode: ThemeMode;
}) {
  return (
    <div class="settings-page-body">
      <PanelSection icon={<User size={16} />} title="个性化">
        <MetricGrid
          items={[
            ["账户", "Administrator"],
            ["主题", props.themeMode === "dark" ? "暗色" : "浅色"],
            ["推理", props.profile.label],
            ["连接", props.deepSeek.configured ? statusLabel(props.deepSeek.lastCheckStatus) : "未连接"],
            ["工作区", compactPath(props.runtimeDraft.workspaceRoot)],
            ["缓存目标", `${props.runtimeDraft.cacheTargetPercent.toFixed(1)}%`],
          ]}
        />
      </PanelSection>
    </div>
  );
}

function ShortcutSettingsPanel() {
  return (
    <div class="settings-page-body">
      <PanelSection icon={<Keyboard size={16} />} title="键盘快捷键">
        <div class="item-list">
          <div class="shortcut-row">
            <span>新建任务</span>
            <kbd>Ctrl+N</kbd>
          </div>
          <div class="shortcut-row">
            <span>搜索</span>
            <kbd>Ctrl+K</kbd>
          </div>
          <div class="shortcut-row">
            <span>发送消息</span>
            <kbd>Enter</kbd>
          </div>
          <div class="shortcut-row">
            <span>换行</span>
            <kbd>Shift+Enter</kbd>
          </div>
        </div>
      </PanelSection>
    </div>
  );
}

function ArchiveSettingsPanel() {
  return (
    <div class="settings-page-body">
      <PanelSection icon={<Archive size={16} />} title="已归档对话">
        <p class="empty-line">暂无已归档对话</p>
      </PanelSection>
    </div>
  );
}

function RuntimeSaveActions(props: {
  dirty: boolean;
  reset: () => void;
  save: () => void;
  saving: boolean;
}) {
  return (
    <div class="settings-actions">
      <button type="button" onClick={props.reset} disabled={!props.dirty || props.saving}>
        重置
      </button>
      <button class="primary" type="button" onClick={props.save} disabled={!props.dirty || props.saving}>
        {props.saving ? "保存中" : "保存设置"}
      </button>
    </div>
  );
}

function ModelRouteMenu(props: {
  onManage: () => void;
  onSelect: (providerID: string, modelID: string) => void;
  saving: boolean;
  settings: RuntimeSettings;
}) {
  const [open, setOpen] = createSignal(false);
  const [hoverProviderID, setHoverProviderID] = createSignal("");
  let menuRef: HTMLDivElement | undefined;

  const selectedProvider = createMemo(() => selectedModelProvider(props.settings));
  const selectedModel = createMemo(() => selectedModelName(props.settings));
  const activeProviderID = createMemo(() => hoverProviderID() || selectedProvider()?.id || props.settings.model.providers[0]?.id || "");
  const activeProvider = createMemo(
    () => props.settings.model.providers.find((provider) => provider.id === activeProviderID()) ?? selectedProvider(),
  );
  const currentLabel = createMemo(() => {
    const provider = selectedProvider();
    const model = selectedModel();
    if (!provider) {
      return "选择模型";
    }
    return model ? `${provider.name} · ${shortModelName(model)}` : provider.name;
  });

  const close = () => {
    setOpen(false);
    setHoverProviderID("");
  };
  const selectModel = (providerID: string, modelID: string) => {
    close();
    props.onSelect(providerID, modelID);
  };

  onMount(() => {
    const handlePointerDown = (event: PointerEvent) => {
      if (!open()) {
        return;
      }
      const target = event.target;
      if (target instanceof Node && menuRef?.contains(target)) {
        return;
      }
      close();
    };
    window.addEventListener("pointerdown", handlePointerDown);
    onCleanup(() => window.removeEventListener("pointerdown", handlePointerDown));
  });

  return (
    <div class="model-route-menu" ref={menuRef}>
      <button
        class="model-route-trigger"
        type="button"
        aria-haspopup="menu"
        aria-expanded={open()}
        title="选择模型"
        disabled={props.saving}
        onClick={() => setOpen((value) => !value)}
      >
        <Cpu size={15} />
        <span>{currentLabel()}</span>
        <ChevronDown size={14} aria-hidden="true" />
      </button>
      <Show when={open()}>
        <div class="model-route-popover" role="menu">
          <div class="model-provider-column">
            <For each={props.settings.model.providers}>
              {(provider) => (
                <button
                  class="model-provider-option"
                  classList={{ active: activeProviderID() === provider.id }}
                  type="button"
                  onClick={() => setHoverProviderID(provider.id)}
                  onPointerEnter={() => setHoverProviderID(provider.id)}
                >
                  <span>
                    <strong>{provider.name}</strong>
                    <small>{runtimeLabel(providerProtocolOptions, provider.protocol)}</small>
                  </span>
                  <Show when={selectedProvider()?.id === provider.id}>
                    <Check size={14} aria-label="当前供应商" />
                  </Show>
                  <ChevronRight size={14} aria-hidden="true" />
                </button>
              )}
            </For>
          </div>
          <div class="model-list-column">
            <Show when={activeProvider()} keyed>
              {(provider) => (
                <>
                  <div class="model-list-head">
                    <strong>{provider.name}</strong>
                    <span>{providerReadyForChat(provider) ? `${modelOptionsForProvider(provider).length} 个模型` : "未配置密钥"}</span>
                  </div>
                  <div class="model-list-scroll">
                    <For each={modelOptionsForProvider(provider)} fallback={<p class="model-list-empty">暂无模型，先获取或手动添加。</p>}>
                      {(model) => (
                        <button
                          class="model-option"
                          classList={{ selected: selectedProvider()?.id === provider.id && selectedModel() === model.id }}
                          type="button"
                          onClick={() => selectModel(provider.id, model.id)}
                        >
                          <span>
                            <strong>{model.displayName || model.id}</strong>
                            <small>
                              {model.id}
                              <Show when={model.contextWindowTokens}> · {formatTokenWindow(model.contextWindowTokens)}</Show>
                            </small>
                          </span>
                          <Show when={selectedProvider()?.id === provider.id && selectedModel() === model.id}>
                            <Check size={15} aria-label="已选中" />
                          </Show>
                        </button>
                      )}
                    </For>
                  </div>
                </>
              )}
            </Show>
          </div>
          <button
            class="model-manage-button"
            type="button"
            onClick={() => {
              close();
              props.onManage();
            }}
          >
            管理模型
          </button>
        </div>
      </Show>
    </div>
  );
}

function SettingsSection(props: { action?: JSX.Element; children: JSX.Element; title?: string }) {
  return (
    <section class="settings-form-section">
      <Show when={props.title || props.action}>
        <div class="settings-form-title">
          <Show when={props.title}>
            <h2>{props.title}</h2>
          </Show>
          <Show when={props.action}>
            <div>{props.action}</div>
          </Show>
        </div>
      </Show>
      {props.children}
    </section>
  );
}

function SettingsCard(props: { children: JSX.Element }) {
  return <div class="settings-card">{props.children}</div>;
}

function SettingsRow(props: {
  control?: JSX.Element;
  danger?: boolean;
  description?: string;
  icon?: JSX.Element;
  title: string;
}) {
  return (
    <div class="settings-row" classList={{ danger: props.danger, "has-icon": Boolean(props.icon) }}>
      <Show when={props.icon}>
        <div class="settings-row-icon">{props.icon}</div>
      </Show>
      <div class="settings-row-copy">
        <strong>{props.title}</strong>
        <Show when={props.description}>
          <span>{props.description}</span>
        </Show>
      </div>
      <Show when={props.control}>
        <div class="settings-row-control">{props.control}</div>
      </Show>
    </div>
  );
}

function SwitchControl(props: { checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label class="settings-switch">
      <input type="checkbox" checked={props.checked} onChange={(event) => props.onChange(event.currentTarget.checked)} />
      <span aria-hidden="true">
        <span />
      </span>
    </label>
  );
}

function CachePanel(props: {
  cacheHealth: WorkbenchState["cacheHealth"];
  cacheHitRate: number;
  cacheTarget: number;
  diagnostics: string[];
  hasCacheTokens: boolean;
  session: DeepSeekSessionState;
  sessionHasCacheTokens: boolean;
  usage: UsageMetrics;
}) {
  return (
    <div class="settings-page-body">
      <PanelSection icon={<ShieldCheck size={16} />} title="命中率">
        <div class="cache-readout">
          <strong>{formatPercent(props.cacheHitRate, props.hasCacheTokens)}</strong>
          <span>{cacheStatusLabel(props.cacheHealth.status)}</span>
          <small>
            目标 {formatPercent(props.cacheTarget, true)} · hit {formatInteger(props.usage.promptCacheHitTokens)} / miss{" "}
            {formatInteger(props.usage.promptCacheMissTokens)}
          </small>
        </div>
        <MetricGrid
          items={[
            ["会话命中", formatPercent(props.session.sessionCacheHitRate, props.sessionHasCacheTokens)],
            ["会话 hit/miss", `${formatInteger(props.session.sessionCacheHitTokens)} / ${formatInteger(props.session.sessionCacheMissTokens)}`],
            ["miss 预算", formatInteger(props.cacheHealth.missTokenBudget)],
            ["达标 hit", formatInteger(props.cacheHealth.requiredHitTokens)],
            ["还差 hit", formatInteger(props.cacheHealth.additionalHitTokensNeeded)],
            ["稳定前缀", `${formatInteger(props.session.stablePromptTokens)} tok`],
            ["系统 hash", shortHash(props.session.systemPromptHash)],
            ["样本", `${props.cacheHealth.shortPrompt ? "短样本" : "常规"} · ${formatInteger(props.cacheHealth.sampleCount)} 轮`],
          ]}
        />
      </PanelSection>
      <PanelSection icon={<AlertTriangle size={16} />} title="诊断">
        <div class="diagnostic-stack">
          <For each={props.diagnostics} fallback={<p class="empty-line">暂无诊断</p>}>
            {(item) => <p>{item}</p>}
          </For>
        </div>
      </PanelSection>
    </div>
  );
}

function ContextPanel(props: { contextPreview: WorkbenchState["contextPreview"] | undefined }) {
  return (
    <div class="settings-page-body">
      <PanelSection icon={<Hash size={16} />} title="前缀">
        <code class="hash-box">{shortHash(props.contextPreview?.prefixHash)}</code>
      </PanelSection>
      <PanelSection icon={<Braces size={16} />} title="稳定区">
        <ContextList sections={props.contextPreview?.stablePrefix ?? []} />
      </PanelSection>
      <PanelSection icon={<Zap size={16} />} title="易变尾部">
        <ContextList sections={props.contextPreview?.volatileTail ?? []} />
      </PanelSection>
    </div>
  );
}

function PanelSection(props: { icon: JSX.Element; title: string; children: JSX.Element }) {
  return (
    <section class="panel-section">
      <div class="panel-title">
        {props.icon}
        <h2>{props.title}</h2>
      </div>
      {props.children}
    </section>
  );
}

function MetricGrid(props: { items: Array<[string, string]> }) {
  return (
    <div class="metric-grid">
      <For each={props.items}>
        {([label, value]) => (
          <div>
            <span>{label}</span>
            <strong>{value}</strong>
          </div>
        )}
      </For>
    </div>
  );
}

function ContextList(props: { sections: Array<{ name: string; content: string }> }) {
  return (
    <div class="context-stack">
      <For each={props.sections} fallback={<p class="empty-line">暂无内容</p>}>
        {(section) => (
          <div class="context-row">
            <strong>{sectionLabel(section.name)}</strong>
            <span>{section.content || "空"}</span>
          </div>
        )}
      </For>
    </div>
  );
}

function RouteStep(props: { icon: JSX.Element; title: string; detail: string }) {
  return (
    <div class="route-step">
      {props.icon}
      <strong>{props.title}</strong>
      <span>{props.detail}</span>
    </div>
  );
}

function IconButton(props: {
  children: JSX.Element;
  danger?: boolean;
  disabled?: boolean;
  onClick: () => void;
  title: string;
}) {
  return (
    <button
      class="icon-button"
      classList={{ danger: props.danger }}
      type="button"
      title={props.title}
      disabled={props.disabled}
      onClick={props.onClick}
    >
      {props.children}
    </button>
  );
}

function StatusPill(props: { icon: JSX.Element; label: string; tone: "good" | "watch" | "bad" | "neutral" }) {
  return (
    <span class={`status-pill ${props.tone}`}>
      {props.icon}
      {props.label}
    </span>
  );
}

function SettingField(props: { label: string; value: string; children: JSX.Element }) {
  return (
    <div class="setting-field">
      <div class="setting-field-head">
        <span>{props.label}</span>
        <strong>{props.value}</strong>
      </div>
      {props.children}
    </div>
  );
}

function SegmentedControl(props: {
  value: string;
  options: Array<{ value: string; label: string; description?: string }>;
  onChange: (value: string) => void;
}) {
  return (
    <div class="segmented-control">
      <For each={props.options}>
        {(option) => (
          <button
            type="button"
            classList={{ active: props.value === option.value, danger: option.value === "danger-full-access" }}
            title={option.description}
            onClick={() => props.onChange(option.value)}
          >
            {option.label}
          </button>
        )}
      </For>
    </div>
  );
}

function SelectControl(props: {
  value: string;
  options: Array<{ value: string; label: string }>;
  onChange: (value: string) => void;
}) {
  return (
    <select class="settings-select" value={props.value} onChange={(event) => props.onChange(event.currentTarget.value)}>
      <For each={props.options}>
        {(option) => <option value={option.value}>{option.label}</option>}
      </For>
    </select>
  );
}

function ToggleRow(props: {
  checked: boolean;
  danger?: boolean;
  icon: JSX.Element;
  label: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label class="toggle-row" classList={{ danger: props.danger }}>
      <span class="toggle-label">
        {props.icon}
        {props.label}
      </span>
      <input type="checkbox" checked={props.checked} onChange={(event) => props.onChange(event.currentTarget.checked)} />
      <span class="switch-track" aria-hidden="true">
        <span />
      </span>
    </label>
  );
}

function createChatMessage(role: ChatMessage["role"], content: string): ChatMessage {
  return {
    id: `${role}-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    role,
    content,
    createdAt: new Date().toISOString(),
  };
}

function emptyUsageMetrics(): UsageMetrics {
  return {
    promptCacheHitTokens: 0,
    promptCacheMissTokens: 0,
    inputTokens: 0,
    outputTokens: 0,
    effectiveCost: 0,
  };
}

function fallbackDeepSeekState() {
  return {
    configured: false,
    baseUrl: "https://api.deepseek.com",
    lastCheckStatus: "idle",
    lastCheckMessage: "等待保存 DeepSeek API Key。",
    models: [],
  };
}

function fallbackDeepSeekSession(): DeepSeekSessionState {
  return {
    active: false,
    providerId: "",
    providerName: "",
    protocol: "",
    model: "",
    reasoning: defaultReasoningLevel,
    thinkingMode: "disabled",
    reasoningEffort: "",
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
  };
}

function fallbackCacheHealth(): WorkbenchState["cacheHealth"] {
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

function fallbackRuntimeSettings(): RuntimeSettings {
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
          extraHeaders: "",
          extraBodyJson: "",
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
          extraHeaders: "",
          extraBodyJson: "",
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
          extraHeaders: "",
          extraBodyJson: "",
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

function fallbackConfigFiles(): WorkbenchState["configFiles"] {
  return {
    runtimeSettingsPath: "C:\\Users\\Administrator\\AppData\\Roaming\\MHcode\\runtime-settings.json",
    modelProvidersPath: "C:\\Users\\Administrator\\AppData\\Roaming\\MHcode\\runtime-settings.json",
    secretsStore: "系统凭据管理器 / 本地 vault",
  };
}

function categoryForDrawerTab(tab: DrawerTab): SettingsCategory {
  switch (tab) {
    case "settings":
      return "general";
    case "cache":
      return "usage";
    case "context":
      return "index";
    case "tools":
      return "skills";
  }
}

function findSettingsItem(category: SettingsCategory) {
  for (const group of settingsGroups) {
    const item = group.items.find((candidate) => candidate.id === category);
    if (item) {
      return item;
    }
  }
  return settingsGroups[0].items[0];
}

function settingsGroupTitle(category: SettingsCategory) {
  return settingsGroups.find((group) => group.items.some((item) => item.id === category))?.title ?? "设置";
}

function settingsCategoryDescription(category: SettingsCategory) {
  switch (category) {
    case "config":
      return "配置审批策略和沙盒设置。";
    case "mcp":
      return "连接外部工具和数据源。";
    case "browser":
      return "管理 MHcode 的浏览器。可在电脑使用设置中配置 Google Chrome。";
    case "computer":
      return "管理 MHcode 如何使用您电脑上的其他应用程序。";
    case "git":
      return "设置分支、拉取请求和工作树清理策略。";
    case "environment":
      return "本地环境用于指示 MHcode 如何为项目设置工作树。";
    case "general":
      return "管理 DeepSeek 连接和当前会话。";
    case "appearance":
      return "调整主题和工作区布局。";
    case "models":
      return "选择模型和推理强度。";
    case "skills":
      return "查看已加载的 Skills 和执行链路。";
    case "commands":
      return "查看本地命令权限和工具路由。";
    case "index":
      return "查看稳定前缀和易变上下文。";
    case "usage":
      return "查看缓存命中率、tokens 和诊断信息。";
    case "profile":
      return "查看当前用户和个性化摘要。";
    case "shortcuts":
      return "查看常用键盘快捷键。";
    case "archive":
      return "查看已归档的对话。";
  }
}

function baseNameFromPath(path: string) {
  return path.replace(/[\\/]+$/, "").split(/[\\/]/).filter(Boolean).pop() ?? path;
}

function parentPath(path: string) {
  const parts = path.replace(/[\\/]+$/, "").split(/[\\/]/);
  if (parts.length <= 1) {
    return path;
  }
  return parts.slice(0, -1).join("\\");
}

function prefixStatusLabel(session: DeepSeekSessionState) {
  if (!session.active || session.previousRequestMessageCount === 0) {
    return "首轮";
  }
  if (session.appendOnlyPrefixStable) {
    return `稳定 ${session.commonPrefixMessageCount}/${session.previousRequestMessageCount}`;
  }
  return `变动 ${session.commonPrefixMessageCount}/${session.previousRequestMessageCount}`;
}

function thinkingStatusLabel(session: DeepSeekSessionState) {
  if (session.thinkingMode === "enabled") {
    return session.reasoningEffort ? `开启 ${session.reasoningEffort}` : "开启";
  }
  if (session.thinkingMode === "disabled") {
    return "关闭";
  }
  return "下一轮选择";
}

function messageTitle(message: ChatMessage) {
  if (message.role === "user") {
    return "你";
  }
  if (message.role === "assistant") {
    return message.model || "DeepSeek";
  }
  return "系统";
}

function formatClock(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
}

function formatPercent(value: number, available: boolean) {
  if (!available) {
    return "待采集";
  }
  return `${(value * 100).toFixed(1)}%`;
}

function formatInteger(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function shortHash(value?: string) {
  if (!value) {
    return "sha256:pending";
  }
  if (value.length <= 22) {
    return value;
  }
  return `${value.slice(0, 13)}...${value.slice(-8)}`;
}

function sectionLabel(name: string) {
  const labels: Record<string, string> = {
    product_identity: "产品身份",
    system_rules: "Agent 规则",
    reasoning: "当前推理",
    skills_index: "Skills 索引",
    mcp_schema_snapshot: "MCP schema",
    project_summary: "项目摘要",
    routing_policy: "路由策略",
    user_input: "用户输入",
    recent_diff: "最近 diff",
    tool_results: "工具结果",
    output_requirements: "输出要求",
  };
  return labels[name] ?? name;
}

function errorMessage(err: unknown) {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

function statusLabel(status: string) {
  switch (status) {
    case "ok":
      return "已连接";
    case "error":
      return "连接异常";
    default:
      return "待检测";
  }
}

function cacheStatusLabel(status: string) {
  switch (status) {
    case "ok":
      return "已达标";
    case "watch":
      return "观察中";
    case "warming":
      return "DS 预热中";
    case "cold":
      return "冷启动";
    case "low":
      return "需优化";
    default:
      return "待采集";
  }
}

function runtimeLabel(options: Array<{ value: string; label: string }>, value: string) {
  return options.find((option) => option.value === value)?.label ?? value;
}

function selectedModelProvider(settings: RuntimeSettings) {
  return (
    settings.model.providers.find((provider) => provider.id === settings.model.selectedProviderId) ??
    settings.model.providers.find((provider) => provider.enabled) ??
    settings.model.providers[0]
  );
}

function selectedModelName(settings: RuntimeSettings, activeSessionModel?: string) {
  if (activeSessionModel) {
    return activeSessionModel;
  }
  const provider = selectedModelProvider(settings);
  if (!provider) {
    return "";
  }
  if (settings.model.selectedProviderId === provider.id && settings.model.selectedModelId) {
    return settings.model.selectedModelId;
  }
  if (provider.defaultModelId) {
    return provider.defaultModelId;
  }
  if (provider.models[0]?.id) {
    return provider.models[0].id;
  }
  if (provider.id === "deepseek") {
    return "deepseek-v4-flash";
  }
  return "";
}

function modelOptionsForProvider(provider: ModelProviderSetting) {
  if (provider.models.length > 0) {
    return provider.models;
  }
  if (provider.id === "deepseek") {
    return [
      { id: "deepseek-v4-flash", displayName: "DeepSeek V4 Flash", provider: provider.id, contextWindowTokens: provider.contextWindowTokens },
      { id: "deepseek-v4-pro", displayName: "DeepSeek V4 Pro", provider: provider.id, contextWindowTokens: provider.contextWindowTokens },
    ];
  }
  return [];
}

function shortModelName(modelID: string) {
  if (modelID.length <= 28) {
    return modelID;
  }
  return `${modelID.slice(0, 14)}...${modelID.slice(-10)}`;
}

function providerReadyForChat(provider: ModelProviderSetting | undefined) {
  if (!provider) {
    return false;
  }
  return provider.apiKeyConfigured || provider.protocol === "local" || isLocalProviderURL(provider.baseUrl);
}

function providerConnectionSummary(provider: ModelProviderSetting | undefined, deepSeek: WorkbenchState["deepSeek"]) {
  if (!provider) {
    return { label: "未连接", ok: false, ready: false };
  }
  const ready = providerReadyForChat(provider);
  if (provider.id === "deepseek") {
    return {
      label: deepSeek.configured ? statusLabel(deepSeek.lastCheckStatus) : "未连接",
      ok: deepSeek.lastCheckStatus === "ok",
      ready: deepSeek.configured,
    };
  }
  if (!ready) {
    return { label: "未连接", ok: false, ready: false };
  }
  return {
    label: provider.lastSyncStatus === "ok" ? "已连接" : provider.protocol === "local" ? "本地" : "已配置",
    ok: provider.lastSyncStatus === "ok" || provider.protocol === "local",
    ready: true,
  };
}

function isLocalProviderURL(baseUrl: string) {
  const value = baseUrl.toLowerCase();
  return value.includes("localhost") || value.includes("127.0.0.1") || value.includes("[::1]") || value.includes("0.0.0.0");
}

function permissionLabel(value: string) {
  return runtimeLabel(sitePermissionOptions, value);
}

function providerBaseURLHint(protocol: string) {
  switch (protocol) {
    case "deepseek-official":
      return "DeepSeek 官方默认 https://api.deepseek.com";
    case "openai-compatible":
      return "填写兼容 /v1 接口的根地址，例如 https://api.openai.com/v1";
    case "local":
      return "填写本机兼容 /v1 地址，例如 http://127.0.0.1:11434/v1";
    case "anthropic-compatible":
      return "填写 Anthropic API 根地址，例如 https://api.anthropic.com";
    case "gemini":
      return "填写 Gemini API 根地址，例如 https://generativelanguage.googleapis.com/v1beta";
    default:
      return "填写上游模型服务的根地址。";
  }
}

function providerBaseURLPlaceholder(protocol: string) {
  switch (protocol) {
    case "deepseek-official":
      return "https://api.deepseek.com";
    case "local":
      return "http://127.0.0.1:11434/v1";
    case "anthropic-compatible":
      return "https://api.anthropic.com";
    case "gemini":
      return "https://generativelanguage.googleapis.com/v1beta";
    default:
      return "https://api.openai.com/v1";
  }
}

function providerFromPreset(preset: ProviderPreset, existing?: ModelProviderSetting): ModelProviderSetting {
  return {
    id: preset.id,
    name: preset.name,
    protocol: preset.protocol,
    apiType: preset.apiType,
    baseUrl: preset.baseUrl,
    balanceUrl: preset.balanceUrl ?? existing?.balanceUrl ?? "",
    extraHeaders: existing?.extraHeaders ?? "",
    extraBodyJson: existing?.extraBodyJson ?? "",
    enabled: existing?.enabled ?? true,
    apiKeyConfigured: existing?.apiKeyConfigured ?? false,
    defaultModelId: existing?.defaultModelId ?? "",
    contextWindowTokens: preset.contextWindowTokens ?? existing?.contextWindowTokens ?? 0,
    models: existing?.models ?? [],
    lastSyncStatus: existing?.lastSyncStatus ?? "idle",
    lastSyncMessage: existing?.lastSyncMessage ?? "填写密钥后可测试并获取模型。",
    checkedAt: existing?.checkedAt,
    supportsModelFetch: supportsModelFetchForProtocol(preset.protocol),
  };
}

function createEmptyProvider(providers: ModelProviderSetting[]): ModelProviderSetting {
  const nextIndex = providers.length + 1;
  const id = uniqueProviderID(providers, "custom-provider");
  return {
    id,
    name: `自定义供应商 ${nextIndex}`,
    protocol: "openai-compatible",
    apiType: "chat-completions",
    baseUrl: "",
    balanceUrl: "",
    extraHeaders: "",
    extraBodyJson: "",
    enabled: true,
    apiKeyConfigured: false,
    defaultModelId: "",
    contextWindowTokens: 0,
    models: [],
    lastSyncStatus: "idle",
    lastSyncMessage: "填写 API 地址与密钥后可测试并获取模型。",
    supportsModelFetch: true,
  };
}

function uniqueProviderID(providers: ModelProviderSetting[], prefix: string) {
  const ids = new Set(providers.map((provider) => provider.id));
  let index = providers.length + 1;
  let id = `${prefix}-${index}`;
  while (ids.has(id)) {
    index += 1;
    id = `${prefix}-${index}`;
  }
  return id;
}

function defaultAPITypeForProtocol(protocol: string) {
  if (protocol === "anthropic-compatible" || protocol === "anthropic") {
    return "anthropic-messages";
  }
  if (protocol === "gemini") {
    return "gemini-generate-content";
  }
  return "chat-completions";
}

function supportsModelFetchForProtocol(protocol: string) {
  return (
    protocol === "deepseek-official" ||
    protocol === "openai-compatible" ||
    protocol === "anthropic" ||
    protocol === "anthropic-compatible" ||
    protocol === "gemini" ||
    protocol === "local"
  );
}

function formatTokenWindow(value: number) {
  if (!value || value <= 0) {
    return "默认";
  }
  return `${formatInteger(value)} tokens`;
}

function parseEnvLines(value: string) {
  return value
    .split(/\r?\n/)
    .map((line) => {
      const separatorIndex = line.indexOf("=");
      if (separatorIndex < 0) {
        return { key: line.trim(), value: "" };
      }
      return {
        key: line.slice(0, separatorIndex).trim(),
        value: line.slice(separatorIndex + 1).trim(),
      };
    })
    .filter((item) => item.key);
}

function compactPath(value: string) {
  if (!value) {
    return "未设置";
  }
  const normalized = value.replaceAll("/", "\\");
  const parts = normalized.split("\\").filter(Boolean);
  if (parts.length <= 3) {
    return normalized;
  }
  return `${parts[0]}\\...\\${parts.slice(-2).join("\\")}`;
}

function readStoredSidebarWidth() {
  const stored = Number(readLocalStorage(sidebarWidthStorageKey));
  if (!Number.isFinite(stored)) {
    return defaultSidebarWidth;
  }
  return clamp(stored, minSidebarWidth, maxSidebarWidth);
}

function persistSidebarWidth(width: number) {
  writeLocalStorage(sidebarWidthStorageKey, String(Math.round(clamp(width, minSidebarWidth, maxSidebarWidth))));
}

function readStoredThemeMode(): ThemeMode {
  const stored = readLocalStorage(themeStorageKey);
  return stored === "light" ? "light" : "dark";
}

function persistThemeMode(mode: ThemeMode) {
  writeLocalStorage(themeStorageKey, mode);
}

function applyThemeMode(mode: ThemeMode, shell?: HTMLElement) {
  document.documentElement.dataset.theme = mode;
  document.body.dataset.theme = mode;
  shell?.classList.toggle("theme-light", mode === "light");
  shell?.classList.toggle("theme-dark", mode === "dark");
  forceStyleFlush(shell);
}

function applySidebarWidth(width: number, shell?: HTMLElement) {
  const next = `${Math.round(clamp(width, minSidebarWidth, maxSidebarWidth))}px`;
  document.documentElement.style.setProperty("--sidebar-width", next);
  shell?.style.setProperty("--sidebar-width", next);
  forceStyleFlush(shell);
}

function forceStyleFlush(element?: HTMLElement) {
  // WebView2 can occasionally defer repaint on first launch; reading layout flushes pending style changes.
  void document.documentElement.offsetWidth;
  if (element) {
    void element.offsetWidth;
  }
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function readLocalStorage(key: string) {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeLocalStorage(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Storage can be unavailable in a locked-down WebView; keep the in-memory UI state.
  }
}

export default App;
