import {
  AlertTriangle,
  Archive,
  ArrowDown,
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
  ClipboardList,
  Command,
  Copy,
  Cpu,
  Database,
  FileDiff,
  FileText,
  Folder,
  FolderOpen,
  Gauge,
  GitBranch,
  GitFork,
  Globe2,
  HardDrive,
  History,
  ImagePlus,
  Keyboard,
  KeyRound,
  LayoutList,
  ListPlus,
  ListFilter,
  ListCollapse,
  LockKeyhole,
  MessageSquarePlus,
  Monitor,
  Moon,
  Network,
  Palette,
  Pencil,
  Pin,
  Plug,
  Plus,
  RefreshCw,
  Save,
  Search,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  Square,
  Sun,
  Terminal,
  Trash2,
  User,
  Users,
  Wrench,
  X,
  Zap,
  ExternalLink,
  Ellipsis,
  Undo2,
  Redo2,
} from "lucide-solid";
import { For, Match, Show, Switch, createEffect, createMemo, createSignal, onCleanup, onMount } from "solid-js";
import type { JSX } from "solid-js";
import { Portal } from "solid-js/web";
import { ReasoningMenu } from "./components/ReasoningMenu";
import { BrowserPreviewPanel } from "./components/BrowserPreviewPanel";
import { MessageContent, TaskProgress, textToParts } from "./components/chat/MessageContent";
import type { TaskProgressPart } from "./components/chat/MessageContent";
import { TimelinePanel } from "./components/TimelinePanel";
import { ApprovalModal } from "./components/ApprovalModal";
import { ImagePreviewModal } from "./components/ImagePreviewModal";
import { WorkspaceToolsPanel } from "./components/WorkspaceToolsPanel";
import { ReviewPanel } from "./components/ReviewPanel";
import {
  clearDeepSeekAPIKey,
  getWorkbenchState,
  resetDeepSeekSession,
  refreshModelProviderModels,
  refreshMCPServer,
  saveDeepSeekAPIKey,
  saveModelProviderAPIKey,
  startChatMessageForSession,
  getActiveChatTasks,
  guideChatMessage,
  stopChatMessage,
  onChatTaskEvent,
  onMCPState,
  setReasoningLevel,
  saveRuntimeSettings,
  testDeepSeekConnection,
  clearModelProviderAPIKey,
  deleteModelProvider,
  listCheckpoints,
  rewindToCheckpoint,
  listBranches,
  switchBranch,
  forkFromMessage,
  onApprovalRequest,
  respondApproval,
  setPlanMode,
  listProjects,
  listSessions,
  getProjectTree,
  createProject,
  switchProject,
  setProjectPinned,
  renameProject,
  archiveProjectTasks,
  removeProject,
  openProjectInFileManager,
  createPermanentWorktree,
  newSession,
  switchSession,
  archiveSession,
  deleteSession,
  selectDirectory,
  selectWorktreeParentDirectory,
  getSessionMessages,
  getSessionMessagesForSession,
  onBrowserPreviewOpen,
  onBrowserPreviewClose,
  openWorkspaceFile,
  previewWorkspaceFile,
  openBrowserURL,
  openURLInSystemBrowser,
} from "./services/workbench";
import { defaultReasoningLevel, reasoningOptions as fallbackReasoningOptions } from "./state/reasoning";
import type {
  ApprovalRequest,
  BranchInfo,
  ChatAttachment,
  BrowserPreview,
  ChatTaskEvent,
  CheckpointInfo,
  ProjectInfo,
  ProjectNode,
  SessionInfo,
  DeepSeekSessionState,
  MCPServerSetting,
  MessagePart,
  ModelProviderSetting,
  ReasoningLevel,
  RuntimeSettings,
  UsageMetrics,
  WorkbenchState,
} from "./types";

const fallbackProfile =
  fallbackReasoningOptions.find((option) => option.id === defaultReasoningLevel) ?? fallbackReasoningOptions[0];
import { SettingsCenter, ModelRouteMenu } from "./settings-panels";
import type { ChatMessage, DrawerTab, ViewSnapshot, SidebarSession, SettingsCategory, ThemeMode } from "./ui-types";
import {
  defaultSidebarWidth, minSidebarWidth, maxSidebarWidth, defaultBrowserPanelWidth,
  minBrowserPanelWidth, minChatPaneWidth,
} from "./constants";
import {
  formatPercent, formatInteger, shortHash, categoryForDrawerTab, baseNameFromPath,
  selectedModelProvider, selectedModelName, providerReadyForChat, providerConnectionSummary,
  createChatMessage, emptyUsageMetrics, fallbackDeepSeekState, fallbackDeepSeekSession,
  fallbackCacheHealth, fallbackRuntimeSettings, fallbackConfigFiles, readStoredSidebarWidth,
  persistSidebarWidth, readStoredBrowserPanelWidth, persistBrowserPanelWidth,
  readStoredThemeMode, persistThemeMode, applyThemeMode,
  applySidebarWidth, clamp, messageTitle, formatClock,
} from "./format";
import { errorMessage } from "./lib/errors";
import { reconcileSessionMessages } from "./lib/session-history";
import { clearGuidanceMessages, dequeueMessage, enqueueMessage, prioritizeMessage, removeMessage, takeMessageForEditing } from "./lib/message-queue";
import type { ComposerLink, QueuedComposerMessage } from "./lib/message-queue";
import {
  composerSnapshotsEqual,
  emptyComposerHistory,
  recordComposerSnapshot,
  redoComposerSnapshot,
  undoComposerSnapshot,
} from "./lib/composer-history";
import type { ComposerHistory, ComposerSnapshot } from "./lib/composer-history";
import { hasUsablePartialResult } from "./lib/chat-results";
import type { UIAppearancePreferences } from "./ui-appearance";
import {
  applyUIAppearance,
  defaultUIAppearance,
  normalizeUIAppearance,
  persistUIAppearance,
  readStoredUIAppearance,
  resolveEffectiveUIScale,
} from "./ui-appearance";

type ProjectMenuState = {
  project: ProjectNode;
  left: number;
  top: number;
};

type ProjectDialogState = {
  kind: "rename" | "worktree" | "archive" | "remove";
  project: ProjectNode;
  name: string;
  branch: string;
  destinationParent: string;
  destinationName: string;
};

type SessionTaskRuntime = {
  taskID: string;
  sessionID: string;
  messageID: string;
  prompt: string;
  tail: string;
  attachments: ChatAttachment[];
  links: ComposerLink[];
  startedAt: string;
};

function parentDirectory(path: string): string {
  const clean = path.trim().replace(/[\\/]+$/, "");
  const index = Math.max(clean.lastIndexOf("\\"), clean.lastIndexOf("/"));
  if (index < 0) return "";
  if (index === 0) return clean.slice(0, 1);
  if (index === 2 && clean[1] === ":") return clean.slice(0, 3);
  return clean.slice(0, index);
}

function joinNativePath(parent: string, child: string): string {
  const cleanParent = parent.trim().replace(/[\\/]+$/, "");
  if (!cleanParent) return child;
  const separator = cleanParent.includes("\\") ? "\\" : "/";
  return `${cleanParent}${separator}${child}`;
}

function worktreePathSegment(value: string): string {
  const leaf = value.trim().split(/[\\/]/).filter(Boolean).pop() || "worktree";
  return leaf.replace(/[<>:"/\\|?*\x00-\x1f]+/g, "-").replace(/[. ]+$/g, "").slice(0, 64) || "worktree";
}

function App() {
  const storedBrowserPanelWidth = readStoredBrowserPanelWidth();
  const storedUIAppearance = readStoredUIAppearance();
  const [state, setState] = createSignal<WorkbenchState>();
  const [loading, setLoading] = createSignal(true);
  const [updatingReasoning, setUpdatingReasoning] = createSignal(false);
  const [pendingReasoningLevel, setPendingReasoningLevel] = createSignal<ReasoningLevel | undefined>();
  const [savingKey, setSavingKey] = createSignal(false);
  const [testingDeepSeek, setTestingDeepSeek] = createSignal(false);
  const [clearingKey, setClearingKey] = createSignal(false);
  const [savingRuntime, setSavingRuntime] = createSignal(false);
  const [sendingMessage, setSendingMessage] = createSignal(false);
  const [activeChatTaskID, setActiveChatTaskID] = createSignal("");
  const [streamingMessageID, setStreamingMessageID] = createSignal("");
  const [activeTaskProgress, setActiveTaskProgress] = createSignal<TaskProgressPart>();
  const [submittedPrompt, setSubmittedPrompt] = createSignal("");
  const [submittedTail, setSubmittedTail] = createSignal("");
  const [submittedAttachments, setSubmittedAttachments] = createSignal<ChatAttachment[]>([]);
  const [submittedLinks, setSubmittedLinks] = createSignal<ComposerLink[]>([]);
  const [queuedMessages, setQueuedMessages] = createSignal<QueuedComposerMessage[]>([]);
  const [queuedMessagesBySession, setQueuedMessagesBySession] = createSignal<Record<string, QueuedComposerMessage[]>>({});
  const [resettingSession, setResettingSession] = createSignal(false);
  const [apiKeyDraft, setAPIKeyDraft] = createSignal("");
  const [providerKeyDrafts, setProviderKeyDrafts] = createSignal<Record<string, string>>({});
  const [savingProviderID, setSavingProviderID] = createSignal("");
  const [clearingProviderID, setClearingProviderID] = createSignal("");
  const [syncingProviderID, setSyncingProviderID] = createSignal("");
  const [deletingProviderID, setDeletingProviderID] = createSignal("");
  const [refreshingMCPID, setRefreshingMCPID] = createSignal("");
  const [promptDraft, setPromptDraft] = createSignal("");
  const [composerTailDraft, setComposerTailDraftSignal] = createSignal("");
  const [composerAttachments, setComposerAttachments] = createSignal<ChatAttachment[]>([]);
  const [composerLinks, setComposerLinks] = createSignal<ComposerLink[]>([]);
  const [composerUndoDepth, setComposerUndoDepth] = createSignal(0);
  const [composerRedoDepth, setComposerRedoDepth] = createSignal(0);
  const [addingImages, setAddingImages] = createSignal(false);
  const [pendingLinkURL, setPendingLinkURL] = createSignal("");
  const [linkOpenBusy, setLinkOpenBusy] = createSignal<"internal" | "external" | "">("");
  const [previewAttachment, setPreviewAttachment] = createSignal<ChatAttachment>();
  const [copiedMessageID, setCopiedMessageID] = createSignal("");
  const [messages, setMessages] = createSignal<ChatMessage[]>([]);
  const [chatNearBottom, setChatNearBottom] = createSignal(true);
  const [runtimeDraft, setRuntimeDraft] = createSignal<RuntimeSettings>();
  const [drawerOpen, setDrawerOpen] = createSignal(false);
  const [activeSettingsCategory, setActiveSettingsCategory] = createSignal<SettingsCategory>("general");
  const [error, setError] = createSignal("");
  const [sidebarWidth, setSidebarWidth] = createSignal(readStoredSidebarWidth());
  const [resizingSidebar, setResizingSidebar] = createSignal(false);
  const [browserPanelWidth, setBrowserPanelWidth] = createSignal(storedBrowserPanelWidth ?? defaultBrowserPanelWidth);
  const [resizingBrowserPanel, setResizingBrowserPanel] = createSignal(false);
  const [themeMode, setThemeMode] = createSignal<ThemeMode>(readStoredThemeMode());
  const [uiAppearance, setUIAppearance] = createSignal<UIAppearancePreferences>(storedUIAppearance);
  const [effectiveUIScale, setEffectiveUIScale] = createSignal(resolveEffectiveUIScale(storedUIAppearance));
  // 侧边栏交互状态（此前为写死的假 UI，现改为真实信号驱动）
  const [sidebarTab, setSidebarTab] = createSignal<"groups" | "projects">("groups");
  const [showAllSessions, setShowAllSessions] = createSignal(false);
  // 多项目 / 多会话状态。
  const [projects, setProjects] = createSignal<ProjectInfo[]>([]);
  const [sessions, setSessions] = createSignal<SessionInfo[]>([]);
  const [projectTree, setProjectTree] = createSignal<ProjectNode[]>([]);
  const [sessionTaskRuntimes, setSessionTaskRuntimes] = createSignal<Record<string, SessionTaskRuntime>>({});
  const [selectedSessionID, setSelectedSessionID] = createSignal("");
  const [sessionSort, setSessionSort] = createSignal<"recent" | "name">("recent");
  const [showArchived, setShowArchived] = createSignal(false);
  const [switchingSession, setSwitchingSession] = createSignal(false);
  const [deletingSessionID, setDeletingSessionID] = createSignal("");
  const [projectMenu, setProjectMenu] = createSignal<ProjectMenuState>();
  const [projectDialog, setProjectDialog] = createSignal<ProjectDialogState>();
  const [projectActionBusy, setProjectActionBusy] = createSignal(false);
  // 视图历史栈：记录“抽屉是否打开 + 当前设置分类”，供前进/后退按钮导航。
  const [viewHistory, setViewHistory] = createSignal<ViewSnapshot[]>([{ drawer: false, category: "general" }]);
  const [viewCursor, setViewCursor] = createSignal(0);
  // Rewind 时间线状态。
  const [timelineOpen, setTimelineOpen] = createSignal(false);
  const [checkpoints, setCheckpoints] = createSignal<CheckpointInfo[]>([]);
  const [branches, setBranches] = createSignal<BranchInfo[]>([]);
  const [rewinding, setRewinding] = createSignal(false);
  const [forkingMessageID, setForkingMessageID] = createSignal("");
  const [editingMessageID, setEditingMessageID] = createSignal("");
  const [editMessageDraft, setEditMessageDraft] = createSignal("");
  const [editMessageAttachments, setEditMessageAttachments] = createSignal<ChatAttachment[]>([]);
  const [editingMessageBusy, setEditingMessageBusy] = createSignal(false);
  // 审批弹窗：待决请求队列（可能连续多次），逐个处理。
  const [approvalQueue, setApprovalQueue] = createSignal<ApprovalRequest[]>([]);
  const [approvalBusy, setApprovalBusy] = createSignal(false);
  const [browserPreview, setBrowserPreview] = createSignal<BrowserPreview>();
  const [workspaceToolsOpen, setWorkspaceToolsOpen] = createSignal(false);
  const [reviewOpen, setReviewOpen] = createSignal(false);
  let chatScrollRef: HTMLElement | undefined;
  let shellRef: HTMLElement | undefined;
  let workbenchRef: HTMLDivElement | undefined;
  let composerEditorRef: HTMLDivElement | undefined;
  let composerTailEditorRef: HTMLDivElement | undefined;
  let composerImageInputRef: HTMLInputElement | undefined;
  let projectActionMenuRef: HTMLDivElement | undefined;
  let pointerSidebarResizeActive = false;
  let mouseSidebarResizeActive = false;
  let pointerBrowserResizeActive = false;
  let mouseBrowserResizeActive = false;
  let browserPanelWidthInitialized = storedBrowserPanelWidth !== undefined;
  let composerHistory: ComposerHistory = emptyComposerHistory();

  const syncUIAppearance = () => {
    const next = applyUIAppearance(uiAppearance(), shellRef);
    setEffectiveUIScale((current) => current === next ? current : next);
  };

  const updateUIAppearance = (patch: Partial<UIAppearancePreferences>) => {
    const next = normalizeUIAppearance({ ...uiAppearance(), ...patch });
    setUIAppearance(next);
    persistUIAppearance(next);
  };

  const resetUIAppearance = () => {
    const next = { ...defaultUIAppearance };
    setUIAppearance(next);
    persistUIAppearance(next);
  };

  const profile = createMemo(() => state()?.reasoning ?? fallbackProfile);
  const options = createMemo(() => state()?.reasoningOptions ?? fallbackReasoningOptions);
  const usage = createMemo(() => state()?.usageMetrics ?? emptyUsageMetrics());
  const cacheTarget = createMemo(() => state()?.cacheTarget ?? 0.96);
  const cacheHitRate = createMemo(() => state()?.cacheHitRate ?? 0);
  const hasCacheTokens = createMemo(() => usage().promptCacheHitTokens + usage().promptCacheMissTokens > 0);
  const cacheHealth = createMemo(() => state()?.cacheHealth ?? fallbackCacheHealth());
  const snapshots = createMemo(() => state()?.mcpSnapshots ?? []);
  const builtinToolCount = createMemo(() => snapshots().find((snapshot) => snapshot.server === "builtin")?.tools.length ?? 0);
  const mcpServers = createMemo(() => state()?.mcpServers ?? []);
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
  const activeChatModel = createMemo(() => selectedModelName(runtimeSettings()));
  const activeProviderReady = createMemo(() => providerReadyForChat(activeChatProvider()));
  const activeProviderConnection = createMemo(() => providerConnectionSummary(activeChatProvider(), deepSeek()));
  // 项目名绑定真实工作区根路径（缺失时回退占位，不再写死 MHcodeProject）。
  const workspaceName = createMemo(() => {
    const root = runtimeSettings().workspaceRoot?.trim();
    return root ? baseNameFromPath(root) : "未选择项目";
  });
  const activeSessionID = createMemo(() => {
    if (selectedSessionID()) return selectedSessionID();
    const activeProject = projectTree().find((project) => project.isActive);
    return activeProject?.sessions.find((session) => session.isActive)?.id
      ?? sessions().find((session) => session.isActive)?.id
      ?? "";
  });
  const activeSessionTask = createMemo(() => sessionTaskRuntimes()[activeSessionID()]);
  const isSessionBusy = (sessionID: string) => Boolean(sessionID && sessionTaskRuntimes()[sessionID]);
  const currentSessionBusy = createMemo(() => isSessionBusy(activeSessionID()));
  const anySessionBusy = createMemo(() => Object.keys(sessionTaskRuntimes()).length > 0);
  const backgroundTaskCount = createMemo(
    () => Object.keys(sessionTaskRuntimes()).filter((sessionID) => sessionID !== activeSessionID()).length,
  );
  const activeSessionTitle = createMemo(() => {
    const sessionID = activeSessionID();
    for (const project of projectTree()) {
      const session = project.sessions.find((candidate) => candidate.id === sessionID);
      if (session) return session.title || "新对话";
    }
    return sessions().find((session) => session.id === sessionID)?.title || "新对话";
  });

  const rememberCurrentSessionQueue = (sessionID = activeSessionID()) => {
    if (!sessionID) return;
    setQueuedMessagesBySession((current) => ({ ...current, [sessionID]: [...queuedMessages()] }));
  };

  const restoreSessionQueue = (sessionID: string) => {
    setQueuedMessages(queuedMessagesBySession()[sessionID] ?? []);
  };
  const planMode = createMemo(() => state()?.planMode ?? false);
  const teamMode = createMemo(() => runtimeSettings().team.enabled);
  // 完全访问 = 审批策略为 never（命令/文件修改不再逐次弹框确认）。
  const fullAccess = createMemo(() => runtimeSettings().approvalPolicy === "never");
  const modelName = createMemo(() => {
    const provider = activeChatProvider();
    const model = deepSeekSession().model || activeChatModel();
    if (!provider) {
      return model || "选择模型";
    }
    return model ? `${provider.name} · ${model}` : provider.name;
  });
  const canSend = createMemo(() => (promptDraft().trim().length > 0 || composerTailDraft().trim().length > 0 || composerLinks().length > 0 || composerAttachments().length > 0) && activeProviderReady() && Boolean(activeChatModel()));

  createEffect(() => {
    const value = promptDraft();
    const editor = composerEditorRef;
    if (!editor || document.activeElement === editor || composerEditorText(editor) === value) return;
    editor.textContent = value;
  });

  createEffect(() => {
    const value = composerTailDraft();
    const editor = composerTailEditorRef;
    if (!editor || document.activeElement === editor || composerEditorText(editor) === value) return;
    editor.textContent = value;
  });

  const handleBrowserPreviewRequest = async (preview: BrowserPreview) => {
    try {
      if (preview.ask) {
        const useEmbeddedBrowser = window.confirm(`在 MHcode 内置浏览器中打开 ${preview.name}？\n\n选择“取消”将使用系统浏览器。`);
        if (!useEmbeddedBrowser) {
          await openWorkspaceFile(preview.path);
          return;
        }
      }
      setReviewOpen(false);
      setBrowserPreview({ ...preview, ask: false });
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const handlePreviewFile = async (path: string) => {
    setError("");
    try {
      setReviewOpen(false);
      setBrowserPreview(await previewWorkspaceFile(path));
    } catch (err) {
      setError(errorMessage(err));
      throw err;
    }
  };

  const handleOpenBrowser = async () => {
    setError("");
    try {
      setReviewOpen(false);
      const browserState = await openBrowserURL("about:blank");
      setBrowserPreview({
        path: "",
        name: "浏览器",
        url: "about:blank",
        tabId: browserState.activeTabId,
        managed: true,
      });
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const openLinkInternally = async (url: string) => {
    setError("");
    setReviewOpen(false);
    const browserState = await openBrowserURL(url);
    let name = "网页";
    try {
      name = new URL(url).hostname || name;
    } catch {
      // Browser service performs the authoritative URL validation.
    }
    setBrowserPreview({
      path: "",
      name,
      url,
      tabId: browserState.activeTabId,
      managed: true,
    });
  };

  const toggleReviewPanel = () => {
    const next = !reviewOpen();
    setReviewOpen(next);
    if (next) {
      setBrowserPreview(undefined);
      setWorkspaceToolsOpen(false);
      queueMicrotask(constrainBrowserPanelWidth);
    }
  };

  const requestOpenURL = (url: string) => {
    const target = url.trim();
    if (!/^https?:\/\//i.test(target)) {
      setError("只能打开 HTTP 或 HTTPS 链接。");
      return;
    }
    setPendingLinkURL(target);
  };

  const openPendingLink = async (destination: "internal" | "external") => {
    const url = pendingLinkURL();
    if (!url || linkOpenBusy()) return;
    setLinkOpenBusy(destination);
    setError("");
    try {
      if (destination === "internal") {
        await openLinkInternally(url);
      } else {
        await openURLInSystemBrowser(url);
      }
      setPendingLinkURL("");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLinkOpenBusy("");
    }
  };

  const copyMessage = async (message: ChatMessage) => {
    const text = message.content.trim() || (message.parts ?? [])
      .filter((part): part is Extract<MessagePart, { kind: "text" }> => part.kind === "text")
      .map((part) => part.text)
      .join("\n\n")
      .trim();
    if (!text) return;
    try {
      await writeClipboardText(text);
      setCopiedMessageID(message.id);
      window.setTimeout(() => setCopiedMessageID((current) => current === message.id ? "" : current), 1400);
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  // 真实会话列表。当前后端仅支持单会话，故此处只反映“当前对话”；
  // 多会话历史待后端事件流存储落地后再填充（不再摆放写死的演示条目）。
  const sidebarSessions = createMemo(() => {
    const list: SidebarSession[] = [];
    if (messages().length > 0) {
      list.push({
        title: messages()[messages().length - 1]?.content || "当前对话",
        meta: deepSeekSession().active ? `${formatInteger(deepSeekSession().turnCount)} 轮` : "进行中",
        active: !drawerOpen(),
        dot: true,
        onClick: () => setDrawerOpen(false),
      });
    }
    return list;
  });

  // 侧边栏“快捷入口”：真实指标驱动，指向设置抽屉的对应分类。
  const sidebarShortcuts = createMemo<SidebarSession[]>(() => [
    {
      title: "缓存命中率",
      meta: formatPercent(cacheHitRate(), hasCacheTokens()),
      active: activeSettingsCategory() === "usage" && drawerOpen(),
      dot: cacheHealth().status === "low",
      onClick: () => openDrawer("cache"),
    },
    {
      title: "工具链与 Skills",
      meta: `${formatInteger(builtinToolCount())} 工具 · ${formatInteger(skillsIndex().length)} Skills`,
      active: activeSettingsCategory() === "skills" && drawerOpen(),
      dot: false,
      onClick: () => openDrawer("tools"),
    },
  ]);

  // 折叠时最多显示 4 条会话，其余由“显示更多”展开。
  const visibleSessions = createMemo(() =>
    showAllSessions() ? sidebarSessions() : sidebarSessions().slice(0, 4),
  );
  const hasMoreSessions = createMemo(() => sidebarSessions().length > 4);

  const updateStreamingMessage = (update: (message: ChatMessage) => ChatMessage) => {
    const messageID = streamingMessageID();
    if (!messageID) {
      return;
    }
    setMessages((current) => current.map((message) => (message.id === messageID ? update(message) : message)));
  };

  const settleActiveTaskProgress = (taskStatus: "completed" | "failed" | "cancelled") => {
    setActiveTaskProgress((current) => current ? { ...current, taskStatus } : current);
  };

  const finishChatTask = (finishedSessionID = activeSessionID(), finishedTaskID = "") => {
    const visibleSessionID = activeSessionID();
    const visibleTaskID = activeChatTaskID();
    const pendingReasoning = pendingReasoningLevel();
    if (!finishedSessionID) return undefined;
    let removed = false;
    setSessionTaskRuntimes((current) => {
      const task = current[finishedSessionID];
      if (!task || (finishedTaskID && task.taskID && task.taskID !== finishedTaskID)) return current;
      const next = { ...current };
      delete next[finishedSessionID];
      removed = true;
      return next;
    });
    const belongsToVisibleTask = visibleSessionID === finishedSessionID
      && (!finishedTaskID || !visibleTaskID || visibleTaskID === finishedTaskID);
    if (removed && belongsToVisibleTask) {
      setSendingMessage(false);
      setActiveChatTaskID("");
      setStreamingMessageID("");
      setSubmittedPrompt("");
      setSubmittedTail("");
      setSubmittedAttachments([]);
      setSubmittedLinks([]);
      setPendingReasoningLevel(undefined);
      return pendingReasoning && pendingReasoning !== profile().id ? pendingReasoning : undefined;
    }
    return undefined;
  };

  const applyPendingReasoning = async (pending?: ReasoningLevel) => {
    if (pending) await changeReasoning(pending);
  };

  const updateChatScrollState = () => {
    const element = chatScrollRef;
    if (!element) {
      return;
    }
    const remaining = element.scrollHeight - element.scrollTop - element.clientHeight;
    setChatNearBottom(remaining <= 72);
  };

  const scrollChatToBottom = (behavior: ScrollBehavior = "smooth") => {
    const element = chatScrollRef;
    if (!element) {
      return;
    }
    element.scrollTo({ top: element.scrollHeight, behavior });
    if (behavior === "auto") {
      setChatNearBottom(true);
    }
  };

  const handleChatTaskEvent = (event: ChatTaskEvent) => {
    const eventSessionID = event.sessionId?.trim() || activeSessionID();
    const currentTaskID = activeChatTaskID();
    const currentSessionID = activeSessionID();
    const eventTask = eventSessionID ? sessionTaskRuntimes()[eventSessionID] : undefined;
    const isCurrentSession = !eventSessionID || !currentSessionID || eventSessionID === currentSessionID;
    const matchesSessionTask = !eventTask || !eventTask.taskID || eventTask.taskID === event.taskId;
    if (!isCurrentSession || !matchesSessionTask || (currentTaskID && event.taskId !== currentTaskID)) {
      if (event.type === "completed" || event.type === "failed" || event.type === "cancelled") {
        if (eventSessionID && matchesSessionTask) finishChatTask(eventSessionID, event.taskId);
        void refreshProjectsAndSessions();
      }
      return;
    }
    if (currentTaskID && event.taskId !== currentTaskID) {
      return;
    }
    if (!eventTask && !sendingMessage()) {
      return;
    }
    if (!currentTaskID) {
      setActiveChatTaskID(event.taskId);
    }

    switch (event.type) {
      case "started":
      case "status":
        updateStreamingMessage((message) => ({ ...message, status: event.message || "正在思考", statusKind: undefined, compressionStatus: undefined }));
        break;
      case "context_compression":
        updateStreamingMessage((message) => ({
          ...message,
          status: event.message || (event.compression?.status === "error" ? "自动压缩上下文失败" : "正在自动压缩上下文"),
          statusKind: "compression",
          compressionStatus: event.compression?.status === "completed"
            ? "completed"
            : event.compression?.status === "error"
              ? "error"
              : "running",
        }));
        break;
      case "delta":
        updateStreamingMessage((message) => ({
          ...message,
          content: message.content + (event.delta ?? ""),
          model: event.model || message.model,
          status: "正在生成",
          statusKind: undefined,
          compressionStatus: undefined,
        }));
        break;
      case "reasoning":
        updateStreamingMessage((message) => ({
          ...message,
          reasoning: (message.reasoning ?? "") + (event.delta ?? ""),
          status: "正在推理",
          statusKind: undefined,
          compressionStatus: undefined,
        }));
        break;
      case "tool":
        updateStreamingMessage((message) => ({
          ...message,
          parts: mergeLiveToolResultParts(updateLiveToolParts(message.parts, event), event.parts),
          status: event.status === "running"
            ? `正在运行 ${event.toolName || "工具"}`
            : event.status === "error"
              ? `${event.toolName || "工具"} 执行失败`
              : `${event.toolName || "工具"} 已完成`,
          statusKind: undefined,
          compressionStatus: undefined,
        }));
        break;
      case "progress":
        if (event.progress) {
          setActiveTaskProgress(cloneTaskProgress(event.progress));
        }
        updateStreamingMessage((message) => ({
          ...message,
          parts: updateLiveProgressPart(message.parts, event.progress),
          status: "正在执行任务",
          statusKind: undefined,
          compressionStatus: undefined,
        }));
        break;
      case "team":
        updateStreamingMessage((message) => ({
          ...message,
          parts: updateLiveTeamPart(message.parts, event),
          model: event.model || message.model,
          status: event.message || `${event.team?.label || "团队角色"}正在工作`,
          statusKind: undefined,
          compressionStatus: undefined,
        }));
        break;
      case "guidance": {
        const previousResult = event.result;
        const previousProgress = findTaskProgress(previousResult?.parts);
        if (previousProgress) {
          setActiveTaskProgress(cloneTaskProgress(previousProgress));
        }
        if (previousResult) {
          setState(previousResult.state);
          updateStreamingMessage((message) => ({
            ...message,
            content: previousResult.content || message.content || previousResult.reasoning || "本轮没有返回可展示内容。",
            reasoning: previousResult.reasoning,
            model: previousResult.model,
            usage: previousResult.usage,
            parts: previousResult.parts,
            streaming: false,
            status: undefined,
          }));
        }
        if (event.guidanceId) {
          setQueuedMessages((current) => removeMessage(current, event.guidanceId!));
        }
        const guidance = event.guidance?.trim() ?? "";
        const guidanceAttachments = event.attachments?.map((attachment) => ({ ...attachment })) ?? [];
        const assistantMessageID = `assistant-guidance-${Date.now()}`;
        setMessages((current) => [
          ...current,
          { ...createChatMessage("user", guidance), attachments: guidanceAttachments },
          {
            id: assistantMessageID,
            role: "assistant",
            content: "",
            createdAt: new Date().toISOString(),
            model: activeChatModel(),
            streaming: true,
            status: event.message || "正在应用引导",
          },
        ]);
        setStreamingMessageID(assistantMessageID);
        setSubmittedPrompt(guidance);
        setSubmittedTail("");
        setSubmittedAttachments(guidanceAttachments);
        setSubmittedLinks([]);
        break;
      }
      case "completed": {
        const result = event.result;
        const resultProgress = findTaskProgress(result?.parts);
        if (resultProgress) {
          setActiveTaskProgress({ ...cloneTaskProgress(resultProgress), taskStatus: "completed" });
        } else {
          settleActiveTaskProgress("completed");
        }
        if (result) {
          setState(result.state);
          updateStreamingMessage((message) => ({
            ...message,
            content: result.content || message.content || result.reasoning || "本轮没有返回可展示内容。",
            reasoning: result.reasoning,
            model: result.model,
            usage: result.usage,
            parts: result.parts,
            streaming: false,
            status: undefined,
          }));
        } else {
          updateStreamingMessage((message) => ({ ...message, streaming: false, status: undefined }));
        }
        const pendingReasoning = finishChatTask(eventSessionID, event.taskId);
        void (async () => {
          if (eventSessionID === activeSessionID()) {
            await restoreSessionMessages(true, eventSessionID);
            await applyPendingReasoning(pendingReasoning);
            await startNextQueuedMessage();
          }
          void refreshProjectsAndSessions();
          if (eventSessionID === activeSessionID()) void refreshCheckpoints();
        })();
        break;
      }
      case "cancelled":
        setQueuedMessages(clearGuidanceMessages);
        setComposerDraft(submittedPrompt());
        setComposerTail(submittedTail());
        setComposerAttachments(submittedAttachments());
        setComposerLinks(submittedLinks());
        resetComposerHistory();
        updateStreamingMessage((message) => ({
          ...message,
          parts: settleLiveProgress(message.parts, "cancelled"),
          streaming: false,
          cancelled: true,
          status: undefined,
        }));
        settleActiveTaskProgress("cancelled");
        void applyPendingReasoning(finishChatTask(eventSessionID, event.taskId));
        break;
      case "failed": {
        setQueuedMessages(clearGuidanceMessages);
        const message = chatFailureMessage(event.message || "模型请求失败。");
        const result = event.result;
        const partialToolCompleted = hasUsablePartialResult(result?.parts);
        if (result) {
          setState(result.state);
        }
        setError(partialToolCompleted ? "" : message);
        if (!partialToolCompleted) {
          setComposerDraft(submittedPrompt());
          setComposerTail(submittedTail());
          setComposerAttachments(submittedAttachments());
          setComposerLinks(submittedLinks());
          resetComposerHistory();
        }
        updateStreamingMessage((current) => ({
          ...current,
          content: partialToolCompleted
            ? result?.content || current.content || message
            : current.content || result?.content || message,
          reasoning: result?.reasoning || current.reasoning,
          model: result?.model || current.model,
          usage: result?.usage || current.usage,
          parts: settleLiveProgress(
            result?.parts?.length ? result.parts : current.parts,
            partialToolCompleted ? "completed" : "failed",
          ),
          failed: !partialToolCompleted,
          streaming: false,
          status: undefined,
        }));
        const resultProgress = findTaskProgress(result?.parts);
        if (resultProgress) {
          setActiveTaskProgress({
            ...cloneTaskProgress(resultProgress),
            taskStatus: partialToolCompleted ? "completed" : "failed",
          });
        } else {
          settleActiveTaskProgress(partialToolCompleted ? "completed" : "failed");
        }
        void applyPendingReasoning(finishChatTask(eventSessionID, event.taskId));
        break;
      }
    }
  };

  onMount(() => {
    applyThemeMode(themeMode());
    applySidebarWidth(sidebarWidth(), shellRef);
    syncUIAppearance();
    void (async () => {
      await recoverActiveChatTasks();
      await refreshState();
      await refreshProjectsAndSessions();
      await restoreSessionMessages();
      activateSessionTask(activeSessionID());
    })();
    // 订阅后端审批请求，入队等待用户处理。
    const unsubscribe = onApprovalRequest((req) => {
      setApprovalQueue((queue) => [...queue, req]);
    });
    const unsubscribeBrowserOpen = onBrowserPreviewOpen((preview) => {
      void handleBrowserPreviewRequest(preview);
    });
    const unsubscribeBrowserClose = onBrowserPreviewClose(() => setBrowserPreview(undefined));
    const unsubscribeChatTask = onChatTaskEvent(handleChatTaskEvent);
    const unsubscribeMCPState = onMCPState(setState);
    const closeProjectMenuOnOutsidePress = (event: PointerEvent) => {
      if (!projectMenu()) return;
      const target = event.target;
      if (target instanceof Node && projectActionMenuRef?.contains(target)) return;
      if (target instanceof Element && target.closest("[data-project-menu-trigger]")) return;
      setProjectMenu(undefined);
    };
    const closeProjectUIOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (projectDialog() && !projectActionBusy()) {
        setProjectDialog(undefined);
        return;
      }
      setProjectMenu(undefined);
    };
    window.addEventListener("pointerdown", closeProjectMenuOnOutsidePress);
    window.addEventListener("keydown", closeProjectUIOnEscape);
    onCleanup(unsubscribe);
    onCleanup(unsubscribeBrowserOpen);
    onCleanup(unsubscribeBrowserClose);
    onCleanup(unsubscribeChatTask);
    onCleanup(unsubscribeMCPState);
    onCleanup(() => window.removeEventListener("pointerdown", closeProjectMenuOnOutsidePress));
    onCleanup(() => window.removeEventListener("keydown", closeProjectUIOnEscape));
  });

  // 当前正在处理的审批请求（队首）。
  const activeApproval = createMemo(() => approvalQueue()[0]);
	const browserSurfaceSuspended = createMemo(() => Boolean(
		drawerOpen()
		|| timelineOpen()
		|| activeApproval()
		|| pendingLinkURL()
		|| previewAttachment()
	));

  // 用户对审批弹窗的决定。
  const decideApproval = async (approved: boolean, scope: "once" | "session") => {
    const req = activeApproval();
    if (!req) {
      return;
    }
    setApprovalBusy(true);
    try {
      await respondApproval(req.id, req.tool, approved, scope);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setApprovalBusy(false);
      setApprovalQueue((queue) => queue.slice(1));
    }
  };

  createEffect(() => {
    applyThemeMode(themeMode(), shellRef);
  });

  createEffect(() => {
    applySidebarWidth(sidebarWidth(), shellRef);
  });

  createEffect(() => {
    uiAppearance();
    syncUIAppearance();
  });

  createEffect(() => {
    messages();
    currentSessionBusy();
    const shouldFollow = chatNearBottom();
    queueMicrotask(() => {
      if (shouldFollow) {
        scrollChatToBottom("auto");
      } else {
        updateChatScrollState();
      }
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
    if (currentSessionBusy()) {
      setPendingReasoningLevel(level);
      setError("");
      return;
    }
    setPendingReasoningLevel(undefined);
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

  const removeModelProvider = async (providerID: string) => {
    const provider = activeRuntimeDraft().model.providers.find((item) => item.id === providerID);
    if (!provider || !window.confirm(`删除模型供应商“${provider.name}”？\n\n对应的本地 API Key 也会被清除。`)) {
      return;
    }
    setDeletingProviderID(providerID);
    setError("");
    try {
      if (runtimeDraft()) {
        setState(await saveRuntimeSettings(activeRuntimeDraft()));
        setRuntimeDraft(undefined);
      }
      setState(await deleteModelProvider(providerID));
      setProviderKeyDrafts((current) => {
        const next = { ...current };
        delete next[providerID];
        return next;
      });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setDeletingProviderID("");
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

  const refreshMCPRuntime = async (serverID: string) => {
    setRefreshingMCPID(serverID);
    setError("");
    try {
      if (runtimeDraft()) {
        setState(await saveRuntimeSettings(activeRuntimeDraft()));
        setRuntimeDraft(undefined);
      }
      setState(await refreshMCPServer(serverID));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setRefreshingMCPID("");
    }
  };

  const resetSession = async () => {
    setResettingSession(true);
    setError("");
    try {
      setState(await resetDeepSeekSession());
      setMessages([]);
      setActiveTaskProgress(undefined);
      setQueuedMessages([]);
      setComposerDraft("");
      setComposerTail("");
      setComposerAttachments([]);
      setComposerLinks([]);
      resetComposerHistory();
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

  const setComposerDraft = (value: string) => {
    setPromptDraft(value);
    if (composerEditorRef && composerEditorText(composerEditorRef) !== value) {
      composerEditorRef.textContent = value;
    }
  };

  const setComposerTail = (value: string) => {
    setComposerTailDraftSignal(value);
    if (composerTailEditorRef && composerEditorText(composerTailEditorRef) !== value) {
      composerTailEditorRef.textContent = value;
    }
  };

  const currentComposerSnapshot = (): ComposerSnapshot => ({
    draft: promptDraft(),
    tail: composerTailDraft(),
    attachments: composerAttachments().map((attachment) => ({ ...attachment })),
    links: composerLinks().map((link) => ({ ...link })),
  });

  const syncComposerHistoryDepth = () => {
    setComposerUndoDepth(composerHistory.past.length);
    setComposerRedoDepth(composerHistory.future.length);
  };

  const resetComposerHistory = () => {
    composerHistory = emptyComposerHistory();
    syncComposerHistoryDepth();
  };

  const applyComposerSnapshot = (snapshot: ComposerSnapshot) => {
    setComposerDraft(snapshot.draft);
    setComposerTail(snapshot.tail);
    setComposerAttachments(snapshot.attachments.map((attachment) => ({ ...attachment })));
    setComposerLinks(snapshot.links.map((link) => ({ ...link })));
  };

  const commitComposerSnapshot = (snapshot: ComposerSnapshot) => {
    const current = currentComposerSnapshot();
    if (composerSnapshotsEqual(current, snapshot)) return;
    composerHistory = recordComposerSnapshot(composerHistory, current);
    syncComposerHistoryDepth();
    applyComposerSnapshot(snapshot);
  };

  const focusComposerAfterHistoryMove = () => {
    queueMicrotask(() => {
      const editor = composerLinks().length > 0 ? composerTailEditorRef : composerEditorRef;
      editor?.focus();
      if (editor) placeCaretAtEnd(editor);
    });
  };

  const undoComposerInput = () => {
    const move = undoComposerSnapshot(composerHistory, currentComposerSnapshot());
    composerHistory = move.history;
    syncComposerHistoryDepth();
    if (!move.snapshot) return;
    applyComposerSnapshot(move.snapshot);
    focusComposerAfterHistoryMove();
  };

  const redoComposerInput = () => {
    const move = redoComposerSnapshot(composerHistory, currentComposerSnapshot());
    composerHistory = move.history;
    syncComposerHistoryDepth();
    if (!move.snapshot) return;
    applyComposerSnapshot(move.snapshot);
    focusComposerAfterHistoryMove();
  };

  const handleComposerHistoryShortcut = (event: KeyboardEvent): boolean => {
    if (event.isComposing || event.altKey || (!event.ctrlKey && !event.metaKey)) return false;
    const key = event.key.toLowerCase();
    if (key === "z") {
      event.preventDefault();
      event.shiftKey ? redoComposerInput() : undoComposerInput();
      return true;
    }
    if (key === "y") {
      event.preventDefault();
      redoComposerInput();
      return true;
    }
    return false;
  };

  const focusComposerEnd = () => {
    const editor = composerLinks().length > 0 ? composerTailEditorRef : composerEditorRef;
    editor?.focus();
    if (editor) placeCaretAtEnd(editor);
  };

  const addComposerLinks = (links: ComposerLink[]) => {
    if (links.length === 0) return;
    const current = currentComposerSnapshot();
    commitComposerSnapshot({ ...current, links: mergeComposerLinks(current.links, links) });
  };

  const removeComposerLink = (url: string) => {
    const current = currentComposerSnapshot();
    const links = current.links.filter((link) => link.url !== url);
    let draft = current.draft;
    let tail = current.tail;
    if (links.length === 0 && tail.trim()) {
      draft = [draft.trim(), tail.trim()].filter(Boolean).join(" ");
      tail = "";
    }
    commitComposerSnapshot({ ...current, draft, tail, links });
    if (links.length > 0) return;
    queueMicrotask(() => {
      composerEditorRef?.focus();
      if (composerEditorRef) placeCaretAtEnd(composerEditorRef);
    });
  };

  const absorbComposerURLs = (value: string, target: "prefix" | "tail" = "prefix") => {
    const links = extractComposerLinks(value);
    if (links.length === 0) return false;
    addComposerLinks(links);
    const text = removeComposerURLs(value, links);
    if (target === "tail") {
      setComposerTail(text);
    } else {
      setComposerDraft(text);
    }
    queueMicrotask(() => {
      const editor = composerTailEditorRef ?? composerEditorRef;
      editor?.focus();
      if (editor) placeCaretAtEnd(editor);
    });
    return true;
  };

  const addComposerImages = async (files: File[]) => {
    if (files.length === 0 || addingImages()) return;
    const available = 4 - composerAttachments().length;
    if (available <= 0) {
      setError("一次最多添加 4 张图片。");
      return;
    }
    setAddingImages(true);
    setError("");
    try {
      const selected = files.slice(0, available);
      const next = await Promise.all(selected.map((file, index) => readChatImage(file, composerAttachments().length + index)));
      const combined = [...composerAttachments(), ...next];
      const totalBytes = combined.reduce((sum, attachment) => sum + approximateBase64Bytes(attachment.data), 0);
      if (totalBytes > 12 * 1024 * 1024) {
        throw new Error("图片总大小不能超过 12 MB。");
      }
      commitComposerSnapshot({ ...currentComposerSnapshot(), attachments: combined });
      if (files.length > available) setError("一次最多添加 4 张图片，其余图片未加入。");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setAddingImages(false);
      if (composerImageInputRef) composerImageInputRef.value = "";
    }
  };

  const handleComposerPaste = (event: ClipboardEvent, target: "prefix" | "tail") => {
    const files = Array.from(event.clipboardData?.items ?? [])
      .filter((item) => item.kind === "file" && item.type.startsWith("image/"))
      .map((item) => item.getAsFile())
      .filter((file): file is File => Boolean(file));
    const pastedText = event.clipboardData?.getData("text/plain") ?? "";
    const links = extractComposerLinks(pastedText);
    if (files.length === 0 && links.length === 0) return;
    event.preventDefault();
    if (files.length > 0) void addComposerImages(files);
    if (links.length > 0) addComposerLinks(links);
    const remainingText = links.length > 0 ? removeComposerURLs(pastedText, links) : pastedText;
    const editor = target === "tail" ? composerTailEditorRef : composerEditorRef;
    if (remainingText) insertTextAtSelection(editor, remainingText);
    if (target === "tail") {
      if (editor) setComposerTail(composerEditorText(editor));
    } else if (editor) {
      setComposerDraft(composerEditorText(editor));
    }
    if (links.length > 0) queueMicrotask(focusComposerEnd);
  };

  const sendPrompt = async (rawPrompt: string, attachmentOverride?: ChatAttachment[], linkOverride?: ComposerLink[], tailOverride?: string) => {
    const draft = rawPrompt.trim();
    const tail = (tailOverride ?? composerTailDraft()).trim();
    const attachments = (attachmentOverride ?? composerAttachments()).map((attachment) => ({ ...attachment }));
    const links = (linkOverride ?? composerLinks()).map((link) => ({ ...link }));
    const prompt = composeComposerPrompt(draft, links, tail);
    const sessionID = activeSessionID();
    if (!prompt && attachments.length === 0) {
      return;
    }
    if (!sessionID) {
      setError("当前没有可用会话，请先新建或选择一个会话");
      return;
    }
    if (isSessionBusy(sessionID)) {
      return;
    }
    if (!activeProviderReady() || !activeChatModel()) {
      setError("请先在模型设置里完成当前供应商的密钥和模型配置。");
      openSettings("models");
      return;
    }

    const assistantMessageID = `assistant-stream-${Date.now()}`;
    setActiveTaskProgress(undefined);
    setChatNearBottom(true);
    setMessages((current) => [
      ...current,
      { ...createChatMessage("user", prompt), attachments },
      {
        id: assistantMessageID,
        role: "assistant",
        content: "",
        createdAt: new Date().toISOString(),
        model: activeChatModel(),
        streaming: true,
        status: "正在准备上下文",
      },
    ]);
    setComposerDraft("");
    setComposerTail("");
    setComposerAttachments([]);
    setComposerLinks([]);
    resetComposerHistory();
    setSubmittedPrompt(draft);
    setSubmittedTail(tail);
    setSubmittedAttachments(attachments);
    setSubmittedLinks(links);
    setStreamingMessageID(assistantMessageID);
    setSendingMessage(true);
    setSessionTaskRuntimes((current) => ({
      ...current,
      [sessionID]: {
        taskID: "",
        sessionID,
        messageID: assistantMessageID,
        prompt: draft,
        tail,
        attachments,
        links,
        startedAt: new Date().toISOString(),
      },
    }));
    setError("");
    let taskID = "";
    try {
      taskID = await startChatMessageForSession(sessionID, prompt, attachments);
      setSessionTaskRuntimes((current) => {
        const task = current[sessionID];
        if (!task) return current;
        return { ...current, [sessionID]: { ...task, taskID } };
      });
      if (sessionID === activeSessionID()) {
        setActiveChatTaskID(taskID);
      }
    } catch (err) {
      const message = errorMessage(err);
      if (sessionID === activeSessionID()) {
        setError(message);
        setComposerDraft(draft);
        setComposerTail(tail);
        setComposerAttachments(attachments);
        setComposerLinks(links);
        resetComposerHistory();
        updateStreamingMessage((current) => ({ ...current, content: message, failed: true, streaming: false, status: undefined }));
      }
      void applyPendingReasoning(finishChatTask(sessionID, taskID));
    }
  };

  const enqueueCurrentMessage = (guidance = false) => {
    const draft = promptDraft().trim();
    const tail = composerTailDraft().trim();
    const attachments = composerAttachments().map((attachment) => ({ ...attachment }));
    const links = composerLinks().map((link) => ({ ...link }));
    const prompt = composeComposerPrompt(draft, links, tail);
    if (!prompt && attachments.length === 0) return;
    const queued: QueuedComposerMessage = {
      id: `queued-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      draft,
      tail,
      attachments,
      links,
      createdAt: new Date().toISOString(),
      guidance,
    };
    setQueuedMessages((current) => enqueueMessage(current, queued));
    setComposerDraft("");
    setComposerTail("");
    setComposerAttachments([]);
    setComposerLinks([]);
    resetComposerHistory();
    setError("");
    queueMicrotask(() => composerEditorRef?.focus());
  };

  const startNextQueuedMessage = async () => {
    if (currentSessionBusy()) return;
    const dequeued = dequeueMessage(queuedMessages());
    if (!dequeued.message) return;
    setQueuedMessages(dequeued.queue);
    await sendPrompt(dequeued.message.draft, dequeued.message.attachments, dequeued.message.links, dequeued.message.tail);
  };

  const guideQueuedMessage = async (id: string) => {
    const queued = queuedMessages().find((message) => message.id === id);
    if (!queued) return;
    const pendingGuidance = queuedMessages().find((message) => message.guidance && message.id !== id);
    if (pendingGuidance) {
      setError("已有一条引导等待当前 Agent 应用，请稍后再引导下一条。");
      return;
    }
    setQueuedMessages((current) => prioritizeMessage(current, id));
    const taskID = activeChatTaskID();
    if (!currentSessionBusy() || !taskID) {
      setQueuedMessages(clearGuidanceMessages);
      return;
    }
    try {
      const accepted = await guideChatMessage(
        taskID,
        queued.id,
        composeComposerPrompt(queued.draft, queued.links, queued.tail),
        queued.attachments,
      );
      if (!accepted) {
        setQueuedMessages(clearGuidanceMessages);
      }
    } catch (err) {
      setQueuedMessages(clearGuidanceMessages);
      setError(errorMessage(err));
    }
  };

  const editQueuedMessage = (id: string) => {
    const editing = takeMessageForEditing(queuedMessages(), id);
    if (!editing.message) return;
    setQueuedMessages(editing.queue);
    setComposerDraft(editing.message.draft);
    setComposerTail(editing.message.tail);
    setComposerAttachments(editing.message.attachments);
    setComposerLinks(editing.message.links);
    resetComposerHistory();
    queueMicrotask(focusComposerEnd);
  };

  const sendMessage = async () => {
    if (currentSessionBusy()) {
      enqueueCurrentMessage();
      return;
    }
    await sendPrompt(promptDraft());
  };

  const stopMessage = async () => {
    const sessionID = activeSessionID();
    const taskID = activeChatTaskID();
    if (!sessionID || !taskID) {
      return;
    }
    updateStreamingMessage((message) => ({ ...message, status: "正在停止" }));
    try {
      const accepted = await stopChatMessage(taskID);
      if (accepted) return;

      const activeTasks = await getActiveChatTasks();
      const stillRunning = activeTasks.some((task) => task.taskId === taskID || task.sessionId === sessionID);
      if (stillRunning) {
        setError("停止请求未被当前任务接受，请重试。");
        return;
      }
      updateStreamingMessage((message) => ({
        ...message,
        parts: settleLiveProgress(message.parts, "cancelled"),
        streaming: false,
        cancelled: true,
        status: undefined,
      }));
      settleActiveTaskProgress("cancelled");
      const pendingReasoning = finishChatTask(sessionID, taskID);
      await restoreSessionMessages(true, sessionID);
      await applyPendingReasoning(pendingReasoning);
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  // 拉取项目与会话列表（含 Codex 式项目树）。
  const refreshProjectsAndSessions = async () => {
    try {
      const [projs, sess, tree] = await Promise.all([listProjects(), listSessions(), getProjectTree()]);
      setProjects(projs);
      setSessions(sess);
      setProjectTree(tree);
      const activeProject = tree.find((project) => project.isActive);
      const activeSession = activeProject?.sessions.find((session) => session.isActive);
      if (activeSession) setSelectedSessionID(activeSession.id);
    } catch {
      // 辅助功能失败不打断主流程。
    }
  };

  // 相对时间：5小时 / 2天 / 刚刚（Codex 式）。
  const relativeTime = (iso: string): string => {
    const t = new Date(iso).getTime();
    if (Number.isNaN(t)) return "";
    const diff = Date.now() - t;
    const min = Math.floor(diff / 60000);
    if (min < 1) return "刚刚";
    if (min < 60) return `${min}分钟`;
    const hr = Math.floor(min / 60);
    if (hr < 24) return `${hr}小时`;
    const day = Math.floor(hr / 24);
    if (day < 30) return `${day}天`;
    const mon = Math.floor(day / 30);
    if (mon < 12) return `${mon}个月`;
    return `${Math.floor(mon / 12)}年`;
  };

  // 从后端事件日志恢复当前会话的历史消息到前端（修复关闭打开对话消失）。
  const restoreSessionMessages = async (preserveCurrentOnEmpty = false, sessionID = activeSessionID()) => {
    try {
      const history = await getSessionMessagesForSession(sessionID);
      setMessages((current) => reconcileSessionMessages(current, history, preserveCurrentOnEmpty));
    } catch {
      // 恢复失败不阻断使用。
    }
  };

  // 当前活动会话内容需在切换后重载：从后端事件日志恢复消息。
  const activateSessionTask = (sessionID: string) => {
    const task = sessionTaskRuntimes()[sessionID];
    if (!task) {
      setSendingMessage(false);
      setActiveChatTaskID("");
      setStreamingMessageID("");
      return;
    }
    setSendingMessage(true);
    setActiveChatTaskID(task.taskID);
    setStreamingMessageID(task.messageID);
    setSubmittedPrompt(task.prompt);
    setSubmittedTail(task.tail);
    setSubmittedAttachments(task.attachments);
    setSubmittedLinks(task.links);
    setMessages((current) => {
      if (current.some((message) => message.id === task.messageID)) return current;
      const composed = composeComposerPrompt(task.prompt, task.links, task.tail);
      const hasUser = current.some((message) => message.role === "user" && message.content === composed);
      return [
        ...current,
        ...(!composed || hasUser ? [] : [{ ...createChatMessage("user", composed), attachments: task.attachments }]),
        {
          id: task.messageID,
          role: "assistant" as const,
          content: "",
          createdAt: task.startedAt,
          model: activeChatModel(),
          streaming: true,
          status: "后台任务正在运行",
        },
      ];
    });
  };

  const recoverActiveChatTasks = async () => {
    try {
      const activeTasks = await getActiveChatTasks();
      if (activeTasks.length === 0) return;
      const recovered: Record<string, SessionTaskRuntime> = {};
      for (const task of activeTasks) {
        const sessionID = task.sessionId?.trim();
        if (!sessionID) continue;
        recovered[sessionID] = {
          taskID: task.taskId,
          sessionID,
          messageID: "assistant-recovered-" + task.taskId,
          prompt: "",
          tail: "",
          attachments: [],
          links: [],
          startedAt: task.startedAt || new Date().toISOString(),
        };
      }
      setSessionTaskRuntimes((current) => ({ ...recovered, ...current }));
    } catch {
      // A failed recovery must not block opening the workspace.
    }
  };

  const reloadAfterSessionChange = (nextState: WorkbenchState, rememberQueue = true) => {
    if (rememberQueue) rememberCurrentSessionQueue();
    setState(nextState);
    setActiveTaskProgress(undefined);
    setQueuedMessages([]);
    setComposerDraft("");
    setComposerTail("");
    setComposerAttachments([]);
    setComposerLinks([]);
    resetComposerHistory();
    setChatNearBottom(true);
    if (rememberQueue) void restoreSessionMessages();
    void refreshProjectsAndSessions();
    void refreshCheckpoints();
  };

  const handleNewSession = async () => {
    try {
      reloadAfterSessionChange(await newSession());
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const handleSwitchSession = async (sessionID: string) => {
    setSwitchingSession(true);
    try {
      rememberCurrentSessionQueue(activeSessionID());
      const nextState = await switchSession(sessionID);
      setSelectedSessionID(sessionID);
      reloadAfterSessionChange(nextState, false);
      await restoreSessionMessages(false, sessionID);
      restoreSessionQueue(sessionID);
      activateSessionTask(sessionID);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSwitchingSession(false);
    }
  };

  const handleSwitchProject = async (projectID: string) => {
    try {
      reloadAfterSessionChange(await switchProject(projectID));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const handleAddProject = async () => {
    try {
      const dir = await selectDirectory();
      if (!dir) {
        return;
      }
      const name = baseNameFromPath(dir);
      reloadAfterSessionChange(await createProject(name, dir));
      setSidebarTab("projects");
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const defaultWorktreeBranch = () => {
    const now = new Date();
    const stamp = [now.getFullYear(), String(now.getMonth() + 1).padStart(2, "0"), String(now.getDate()).padStart(2, "0")].join("")
      + "-" + [String(now.getHours()).padStart(2, "0"), String(now.getMinutes()).padStart(2, "0"), String(now.getSeconds()).padStart(2, "0")].join("");
    const prefix = runtimeSettings().git.branchPrefix.trim().replace(/\/+$/, "");
    const leaf = `worktree-${stamp}`;
    return prefix ? `${prefix}/${leaf}` : leaf;
  };

  const defaultWorktreeLocation = (project: ProjectNode) => {
    const sourceRoot = project.workspaceRoot.trim();
    const parent = parentDirectory(sourceRoot);
    const sourceName = worktreePathSegment(baseNameFromPath(sourceRoot) || project.name);
    return { parent, name: `${sourceName}-worktree` };
  };

  const openProjectMenu = (event: MouseEvent, project: ProjectNode) => {
    event.preventDefault();
    event.stopPropagation();
    if (projectActionBusy()) return;
    if (projectMenu()?.project.id === project.id) {
      setProjectMenu(undefined);
      return;
    }
    const rect = event.currentTarget instanceof HTMLElement
      ? event.currentTarget.getBoundingClientRect()
      : undefined;
    if (!rect) return;
    const width = 208;
    const height = 246;
    const margin = 8;
    const left = Math.min(Math.max(margin, rect.right - width), Math.max(margin, window.innerWidth - width - margin));
    const top = rect.bottom + height + margin <= window.innerHeight
      ? rect.bottom + 4
      : Math.max(margin, rect.top - height - 4);
    setProjectMenu({ project, left: Math.round(left), top: Math.round(top) });
  };

  const openProjectDialog = (kind: ProjectDialogState["kind"], project: ProjectNode) => {
    const branch = kind === "worktree" ? defaultWorktreeBranch() : "";
    const worktreeLocation = defaultWorktreeLocation(project);
    setProjectMenu(undefined);
    setError("");
    setProjectDialog({
      kind,
      project,
      name: kind === "rename" ? project.name : "",
      branch,
      destinationParent: kind === "worktree" ? worktreeLocation.parent : "",
      destinationName: kind === "worktree" ? worktreeLocation.name : "",
    });
  };

  const handleSetProjectPinned = async (project: ProjectNode) => {
    setProjectMenu(undefined);
    setProjectActionBusy(true);
    setError("");
    try {
      setState(await setProjectPinned(project.id, !project.pinned));
      await refreshProjectsAndSessions();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setProjectActionBusy(false);
    }
  };

  const handleOpenProjectInFileManager = async (project: ProjectNode) => {
    setProjectMenu(undefined);
    setProjectActionBusy(true);
    setError("");
    try {
      await openProjectInFileManager(project.id);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setProjectActionBusy(false);
    }
  };

  const handleChooseWorktreeParent = async () => {
    const dialog = projectDialog();
    if (!dialog || dialog.kind !== "worktree" || projectActionBusy()) return;
    try {
      const parent = await selectWorktreeParentDirectory();
      if (!parent) return;
      setProjectDialog((current) => current && current.kind === "worktree"
        ? { ...current, destinationParent: parent }
        : current);
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const submitProjectDialog = async () => {
    const dialog = projectDialog();
    if (!dialog || projectActionBusy()) return;
    if (dialog.kind === "rename" && !dialog.name.trim()) {
      setError("项目名称不能为空");
      return;
    }
    if (dialog.kind === "worktree" && (!dialog.branch.trim() || !dialog.destinationParent.trim() || !dialog.destinationName.trim())) {
      setError("分支名和工作树目录不能为空");
      return;
    }
    setProjectActionBusy(true);
    setError("");
    try {
      let nextState: WorkbenchState;
      switch (dialog.kind) {
        case "rename":
          nextState = await renameProject(dialog.project.id, dialog.name.trim());
          setProjectDialog(undefined);
          setState(nextState);
          await refreshProjectsAndSessions();
          break;
        case "archive":
          nextState = await archiveProjectTasks(dialog.project.id);
          setProjectDialog(undefined);
          if (dialog.project.isActive) {
            reloadAfterSessionChange(nextState);
          } else {
            setState(nextState);
            await refreshProjectsAndSessions();
          }
          break;
        case "remove":
          nextState = await removeProject(dialog.project.id);
          setProjectDialog(undefined);
          if (dialog.project.isActive) {
            reloadAfterSessionChange(nextState);
          } else {
            setState(nextState);
            await refreshProjectsAndSessions();
          }
          break;
        case "worktree":
          nextState = await createPermanentWorktree(
            dialog.project.id,
            dialog.branch.trim(),
            joinNativePath(dialog.destinationParent, dialog.destinationName.trim()),
          );
          setProjectDialog(undefined);
          reloadAfterSessionChange(nextState);
          break;
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setProjectActionBusy(false);
    }
  };

  const handleArchiveSession = async (sessionID: string, archived: boolean) => {
    try {
      setState(await archiveSession(sessionID, archived));
      await refreshProjectsAndSessions();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const handleDeleteSession = async (session: SessionInfo) => {
    if (isSessionBusy(session.id) || deletingSessionID()) {
      return;
    }
    const title = session.title || "新对话";
    if (!window.confirm(`永久删除对话“${title}”？\n\n对话记录与事件快照会一并删除，此操作无法撤销。`)) {
      return;
    }
    setDeletingSessionID(session.id);
    setError("");
    try {
      const nextState = await deleteSession(session.id);
      if (session.isActive) {
        reloadAfterSessionChange(nextState);
      } else {
        setState(nextState);
        await refreshProjectsAndSessions();
      }
    } catch (err) {
      setError(errorMessage(err));
      await refreshProjectsAndSessions();
      try {
        setState(await getWorkbenchState());
      } catch {
        // 保留原始删除错误。
      }
      await restoreSessionMessages();
    } finally {
      setDeletingSessionID("");
    }
  };

  const beginEditMessage = (message: ChatMessage) => {
    if (!message.eventId || currentSessionBusy()) {
      return;
    }
    setEditingMessageID(message.id);
    setEditMessageDraft(message.content);
    setEditMessageAttachments(message.attachments ?? []);
    queueMicrotask(() => {
      const editor = document.querySelector<HTMLTextAreaElement>(".op-message-editor textarea");
      editor?.focus();
      editor?.setSelectionRange(editor.value.length, editor.value.length);
      editor?.closest(".op-message-editor")?.scrollIntoView({ block: "end" });
    });
  };

  const cancelEditMessage = () => {
    if (editingMessageBusy()) {
      return;
    }
    setEditingMessageID("");
    setEditMessageDraft("");
    setEditMessageAttachments([]);
  };

  const handleEditResend = async (message: ChatMessage) => {
    const prompt = editMessageDraft().trim();
    const attachments = editMessageAttachments();
    if (!message.eventId || (!prompt && attachments.length === 0) || currentSessionBusy() || editingMessageBusy()) {
      return;
    }
    setEditingMessageBusy(true);
    setError("");
    try {
      setState(await forkFromMessage(message.eventId));
      await restoreSessionMessages();
      await refreshCheckpoints();
      await refreshProjectsAndSessions();
      setEditingMessageID("");
      setEditMessageDraft("");
      setEditMessageAttachments([]);
      setComposerDraft(prompt);
      setComposerTail("");
      await sendPrompt(prompt, attachments, [], "");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setEditingMessageBusy(false);
    }
  };

  const handleForkMessage = async (message: ChatMessage) => {
    if (!message.eventId || currentSessionBusy() || forkingMessageID()) {
      return;
    }
    setForkingMessageID(message.id);
    setError("");
    try {
      setState(await forkFromMessage(message.eventId));
      await restoreSessionMessages();
      await refreshCheckpoints();
      await refreshProjectsAndSessions();
      setTimelineOpen(false);
      setEditingMessageID("");
      setEditMessageDraft("");
      const forkDraft = message.role === "user" ? splitComposerPrompt(message.content) : { text: "", tail: "", links: [] };
      setComposerDraft(forkDraft.text);
      setComposerTail(forkDraft.tail);
      setComposerLinks(forkDraft.links);
      setComposerAttachments(message.role === "user" ? (message.attachments ?? []) : []);
      resetComposerHistory();
      queueMicrotask(focusComposerEnd);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setForkingMessageID("");
    }
  };

  // 会话列表：按排序与归档筛选。
  const displayedSessions = createMemo(() => {
    let list = sessions().filter((s) => showArchived() || !s.archived);
    if (sessionSort() === "name") {
      list = [...list].sort((a, b) => a.title.localeCompare(b.title));
    } else {
      list = [...list].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
    }
    return list;
  });

  // 拉取当前会话的可回退检查点与分支。
  const refreshCheckpoints = async () => {
    try {
      const [cps, brs] = await Promise.all([listCheckpoints(), listBranches()]);
      setCheckpoints(cps);
      setBranches(brs);
    } catch {
      // 时间线是辅助功能，失败不打断主流程。
    }
  };

  const openTimeline = async () => {
    await refreshCheckpoints();
    setTimelineOpen(true);
  };

  // 开关 Plan 两段式。
  const togglePlanMode = async () => {
    try {
      setState(await setPlanMode(!planMode()));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const toggleTeamMode = async () => {
    if (!teamMode() && !profile().budget.planner) {
      setError("AI 团队模式需要高或超高推理强度。");
      return;
    }
    try {
      const current = runtimeSettings();
      setState(await saveRuntimeSettings({
        ...current,
        team: { ...current.team, enabled: !teamMode() },
      }));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  // 切换完全访问：在 never（不弹框）与 on-request（每次确认）之间切换审批策略。
  const toggleFullAccess = async () => {
    try {
      const nextPolicy = fullAccess() ? "on-request" : "never";
      const nextSettings = { ...runtimeSettings(), approvalPolicy: nextPolicy };
      setState(await saveRuntimeSettings(nextSettings));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  // 回退到某个检查点：后端会同时还原对话与文件，随后重建前端消息列表。
  const handleRewind = async (checkpointID: string) => {
    setRewinding(true);
    try {
      const nextState = await rewindToCheckpoint(checkpointID);
      setState(nextState);
      await restoreSessionMessages();
      await refreshCheckpoints();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setRewinding(false);
    }
  };

  // 切换分支：文件与对话在后端一起切换，再从事件日志恢复该分支消息。
  const handleSwitchBranch = async (leafID: string) => {
    setRewinding(true);
    try {
      const nextState = await switchBranch(leafID);
      setState(nextState);
      await restoreSessionMessages();
      await refreshCheckpoints();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setRewinding(false);
    }
  };

  function openDrawer(tab: DrawerTab) {
    openSettings(categoryForDrawerTab(tab));
  }

  function openSettings(category: SettingsCategory) {
    setActiveSettingsCategory(category);
    setDrawerOpen(true);
    pushView({ drawer: true, category });
  }

  function closeDrawer() {
    setDrawerOpen(false);
    pushView({ drawer: false, category: activeSettingsCategory() });
  }

  // 把一个视图快照压入历史（截断游标之后的“前进”记录，与浏览器一致）。
  function pushView(snapshot: ViewSnapshot) {
    const current = viewHistory()[viewCursor()];
    if (current && current.drawer === snapshot.drawer && current.category === snapshot.category) {
      return;
    }
    const trimmed = viewHistory().slice(0, viewCursor() + 1);
    trimmed.push(snapshot);
    setViewHistory(trimmed);
    setViewCursor(trimmed.length - 1);
  }

  // 应用某个历史快照（后退/前进时调用，不再写入历史）。
  function applyView(snapshot: ViewSnapshot) {
    setActiveSettingsCategory(snapshot.category);
    setDrawerOpen(snapshot.drawer);
  }

  const canGoBack = createMemo(() => viewCursor() > 0);
  const canGoForward = createMemo(() => viewCursor() < viewHistory().length - 1);

  function navigateBack() {
    if (!canGoBack()) {
      return;
    }
    const next = viewCursor() - 1;
    setViewCursor(next);
    applyView(viewHistory()[next]);
  }

  function navigateForward() {
    if (!canGoForward()) {
      return;
    }
    const next = viewCursor() + 1;
    setViewCursor(next);
    applyView(viewHistory()[next]);
  }

  const resizeSidebar = (clientX: number) => {
    const left = shellRef?.getBoundingClientRect().left ?? 0;
    const width = clamp((clientX - left) / Math.max(effectiveUIScale(), 0.01), minSidebarWidth, maxSidebarWidth);
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

  const browserPanelLimits = () => {
    const scale = Math.max(effectiveUIScale(), 0.01);
    const available = (workbenchRef?.getBoundingClientRect().width ?? window.innerWidth) / scale;
    const overlay = window.innerWidth / scale <= 1080;
    const min = Math.min(minBrowserPanelWidth, available);
    const max = overlay
      ? available
      : Math.max(min, available - minChatPaneWidth - 8);
    return { min, max };
  };

  const defaultBrowserWidth = () => {
    const scale = Math.max(effectiveUIScale(), 0.01);
    const available = (workbenchRef?.getBoundingClientRect().width ?? window.innerWidth) / scale;
    const desired = window.innerWidth / scale <= 1080 ? defaultBrowserPanelWidth : available * 0.48;
    const limits = browserPanelLimits();
    return clamp(desired, limits.min, limits.max);
  };

  const constrainBrowserPanelWidth = () => {
    const limits = browserPanelLimits();
    const desired = browserPanelWidthInitialized ? browserPanelWidth() : defaultBrowserWidth();
    browserPanelWidthInitialized = true;
    const next = clamp(desired, limits.min, limits.max);
    if (next !== browserPanelWidth()) {
      setBrowserPanelWidth(next);
    }
  };

  const resizeBrowserPanel = (clientX: number) => {
    const rect = workbenchRef?.getBoundingClientRect();
    if (!rect) {
      return;
    }
    const limits = browserPanelLimits();
    setBrowserPanelWidth(clamp((rect.right - clientX) / Math.max(effectiveUIScale(), 0.01), limits.min, limits.max));
  };

  const moveWindowPointerBrowserResize = (event: PointerEvent) => {
    if (pointerBrowserResizeActive) {
      resizeBrowserPanel(event.clientX);
    }
  };

  const stopWindowPointerBrowserResize = () => {
    if (!pointerBrowserResizeActive) {
      return;
    }
    pointerBrowserResizeActive = false;
    setResizingBrowserPanel(false);
    persistBrowserPanelWidth(browserPanelWidth());
    window.removeEventListener("pointermove", moveWindowPointerBrowserResize);
    window.removeEventListener("pointerup", stopWindowPointerBrowserResize);
    window.removeEventListener("pointercancel", stopWindowPointerBrowserResize);
  };

  const startBrowserResize = (event: PointerEvent & { currentTarget: HTMLElement }) => {
    event.preventDefault();
    pointerBrowserResizeActive = true;
    setResizingBrowserPanel(true);
    resizeBrowserPanel(event.clientX);
    event.currentTarget.setPointerCapture?.(event.pointerId);
    window.addEventListener("pointermove", moveWindowPointerBrowserResize);
    window.addEventListener("pointerup", stopWindowPointerBrowserResize);
    window.addEventListener("pointercancel", stopWindowPointerBrowserResize);
  };

  const stopBrowserResize = (event: PointerEvent & { currentTarget: HTMLElement }) => {
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    stopWindowPointerBrowserResize();
  };

  const moveMouseBrowserResize = (event: MouseEvent) => {
    if (mouseBrowserResizeActive) {
      resizeBrowserPanel(event.clientX);
    }
  };

  const stopMouseBrowserResize = () => {
    if (!mouseBrowserResizeActive) {
      return;
    }
    mouseBrowserResizeActive = false;
    setResizingBrowserPanel(false);
    persistBrowserPanelWidth(browserPanelWidth());
    window.removeEventListener("mousemove", moveMouseBrowserResize);
    window.removeEventListener("mouseup", stopMouseBrowserResize);
  };

  const startMouseBrowserResize = (event: MouseEvent) => {
    if (event.button !== 0 || resizingBrowserPanel()) {
      return;
    }
    event.preventDefault();
    mouseBrowserResizeActive = true;
    setResizingBrowserPanel(true);
    resizeBrowserPanel(event.clientX);
    window.addEventListener("mousemove", moveMouseBrowserResize);
    window.addEventListener("mouseup", stopMouseBrowserResize);
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
    window.removeEventListener("pointermove", moveWindowPointerBrowserResize);
    window.removeEventListener("pointerup", stopWindowPointerBrowserResize);
    window.removeEventListener("pointercancel", stopWindowPointerBrowserResize);
    window.removeEventListener("mousemove", moveMouseBrowserResize);
    window.removeEventListener("mouseup", stopMouseBrowserResize);
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

  const setAndPersistBrowserPanelWidth = (width: number) => {
    const limits = browserPanelLimits();
    const next = clamp(width, limits.min, limits.max);
    setBrowserPanelWidth(next);
    persistBrowserPanelWidth(next);
  };

  const resetBrowserPanelWidth = () => {
    setAndPersistBrowserPanelWidth(defaultBrowserWidth());
  };

  const nudgeBrowserPanelWidth = (direction: -1 | 1) => {
    setAndPersistBrowserPanelWidth(browserPanelWidth() + direction * 16);
  };

  onMount(() => {
    const observer = new ResizeObserver(constrainBrowserPanelWidth);
    if (workbenchRef) {
      observer.observe(workbenchRef);
    }
    constrainBrowserPanelWidth();
    window.addEventListener("resize", constrainBrowserPanelWidth);
    window.addEventListener("resize", syncUIAppearance);
    window.visualViewport?.addEventListener("resize", syncUIAppearance);
    onCleanup(() => {
      observer.disconnect();
      window.removeEventListener("resize", constrainBrowserPanelWidth);
      window.removeEventListener("resize", syncUIAppearance);
      window.visualViewport?.removeEventListener("resize", syncUIAppearance);
    });
  });

  createEffect(() => {
    if (browserPreview() || reviewOpen()) {
      queueMicrotask(constrainBrowserPanelWidth);
    }
  });

  return (
    <main
      class="mh-shell"
      classList={{
        resizing: resizingSidebar(),
        "resizing-browser": resizingBrowserPanel(),
        "theme-light": themeMode() === "light",
        "theme-dark": themeMode() === "dark",
      }}
      ref={shellRef}
      style={{ "--sidebar-width": `${sidebarWidth()}px` } as JSX.CSSProperties}
    >
      <aside class="mh-sidebar" aria-label="MHcode 导航">
        <div class="sidebar-top">
          <div class="brand-mark">M</div>
          <button
            class="ghost-icon"
            type="button"
            title="后退"
            disabled={!canGoBack()}
            onClick={navigateBack}
          >
            <ArrowLeft size={16} />
          </button>
          <button
            class="ghost-icon"
            type="button"
            title="前进"
            disabled={!canGoForward()}
            onClick={navigateForward}
          >
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
          <button type="button" onClick={() => void handleNewSession()} disabled={resettingSession()}>
            <MessageSquarePlus size={16} />
            <span>新建任务</span>
          </button>
          <button type="button" onClick={() => openSettings("index")}>
            <Search size={16} />
            <span>搜索</span>
          </button>
          <button type="button" onClick={() => openSettings("skills")}>
            <Sparkles size={16} />
            <span>技能</span>
          </button>
        </div>

        <div class="proj-tree-head">
          <span>项目</span>
          <div class="proj-tree-tools">
            <button
              type="button"
              class="proj-tool-btn"
              classList={{ active: showArchived() }}
              title={showArchived() ? "隐藏已归档会话" : "显示已归档会话"}
              onClick={() => setShowArchived((v) => !v)}
            >
              <Archive size={14} />
            </button>
            <button type="button" class="proj-tool-btn" title="添加项目" onClick={() => void handleAddProject()}>
              <Plus size={15} />
            </button>
          </div>
        </div>

        <div class="proj-tree" onScroll={() => setProjectMenu(undefined)}>
          <Show
            when={projectTree().length > 0}
            fallback={<div class="sidebar-empty"><span>暂无项目</span><small>点右上角 + 添加项目</small></div>}
          >
            <For each={projectTree()}>
              {(project) => (
                <div class="proj-node">
                  <div class="proj-row-wrap">
                    <button
                      class="proj-row"
                      classList={{ active: project.isActive }}
                      type="button"
                      disabled={projectActionBusy()}
                      onClick={() => void handleSwitchProject(project.id)}
                      title={project.workspaceRoot}
                    >
                      <Folder size={14} />
                      <span class="proj-name">{project.name}</span>
                      <Show when={project.pinned}><Pin class="proj-pin" size={11} /></Show>
                    </button>
                    <button
                      class="proj-menu-trigger"
                      classList={{ active: projectMenu()?.project.id === project.id }}
                      type="button"
                      data-project-menu-trigger
                      title="项目操作"
                      aria-label={`${project.name} 项目操作`}
                      aria-haspopup="menu"
                      aria-expanded={projectMenu()?.project.id === project.id}
                      disabled={projectActionBusy()}
                      onClick={(event) => openProjectMenu(event, project)}
                    >
                      <Ellipsis size={15} />
                    </button>
                  </div>
                  <For each={project.sessions.filter((s) => showArchived() || !s.archived)}>
                    {(session) => (
                      <div class="sess-row-wrap" classList={{ archived: session.archived }}>
                        <button
                          class="sess-row"
                          classList={{ active: session.isActive }}
                          type="button"
                          disabled={switchingSession() || deletingSessionID() === session.id}
                          onClick={() => void handleSwitchSession(session.id)}
                          title={session.title}
                        >
                          <span class="sess-title">{session.title || "新对话"}</span>
                          <Show when={sessionTaskRuntimes()[session.id]}>
                            <span class="sess-running-dot" title="后台生成中" aria-label="后台生成中" />
                          </Show>
                          <span class="sess-time">{relativeTime(session.updatedAt)}</span>
                        </button>
                        <div class="sess-actions">
                          <button
                            class="sess-action"
                            type="button"
                            title={session.archived ? "取消归档" : "归档"}
                            aria-label={session.archived ? "取消归档" : "归档"}
                            disabled={isSessionBusy(session.id) || Boolean(deletingSessionID())}
                            onClick={() => void handleArchiveSession(session.id, !session.archived)}
                          >
                            <Archive size={12} />
                          </button>
                          <button
                            class="sess-action danger"
                            type="button"
                            title="永久删除对话"
                            aria-label="永久删除对话"
                            disabled={isSessionBusy(session.id) || Boolean(deletingSessionID())}
                            onClick={() => void handleDeleteSession(session)}
                          >
                            <Trash2 size={12} />
                          </button>
                        </div>
                      </div>
                    )}
                  </For>
                </div>
              )}
            </For>
          </Show>
        </div>

        <Show when={projectMenu()}>
          {(menu) => (
            <Portal>
              <div
                ref={projectActionMenuRef}
                class="project-action-menu"
                role="menu"
                aria-label={`${menu().project.name} 项目操作`}
                style={{ left: `${menu().left}px`, top: `${menu().top}px` }}
              >
                <button type="button" role="menuitem" disabled={projectActionBusy() || anySessionBusy()} onClick={() => void handleSetProjectPinned(menu().project)}>
                  <Pin size={15} />
                  <span>{menu().project.pinned ? "取消置顶" : "置顶项目"}</span>
                </button>
                <button type="button" role="menuitem" disabled={projectActionBusy()} onClick={() => void handleOpenProjectInFileManager(menu().project)}>
                  <FolderOpen size={15} />
                  <span>在资源管理器中打开</span>
                </button>
                <button
                  type="button"
                  role="menuitem"
                  disabled={projectActionBusy() || anySessionBusy() || runtimeSettings().filesystemAccess === "read-only" || runtimeSettings().sandboxMode === "read-only"}
                  title={runtimeSettings().filesystemAccess === "read-only" || runtimeSettings().sandboxMode === "read-only" ? "只读模式下不可创建工作树" : ""}
                  onClick={() => openProjectDialog("worktree", menu().project)}
                >
                  <GitBranch size={15} />
                  <span>创建永久工作树</span>
                </button>
                <button type="button" role="menuitem" disabled={projectActionBusy() || anySessionBusy()} onClick={() => openProjectDialog("rename", menu().project)}>
                  <Pencil size={15} />
                  <span>重命名项目</span>
                </button>
                <button type="button" role="menuitem" disabled={projectActionBusy() || anySessionBusy()} onClick={() => openProjectDialog("archive", menu().project)}>
                  <Archive size={15} />
                  <span>归档任务</span>
                </button>
                <button
                  class="danger"
                  type="button"
                  role="menuitem"
                  disabled={projectActionBusy() || anySessionBusy()}
                  onClick={() => openProjectDialog("remove", menu().project)}
                >
                  <X size={15} />
                  <span>移除</span>
                </button>
              </div>
            </Portal>
          )}
        </Show>

        <Show when={projectDialog()}>
          {(dialog) => (
            <Portal>
              <div
                class="project-action-overlay"
                role="presentation"
                onPointerDown={(event) => {
                  if (event.currentTarget === event.target && !projectActionBusy()) setProjectDialog(undefined);
                }}
              >
                <form
                  class="project-action-dialog"
                  aria-label={dialog().kind === "rename" ? "重命名项目" : dialog().kind === "worktree" ? "创建永久工作树" : dialog().kind === "archive" ? "归档项目任务" : "移除项目"}
                  onSubmit={(event) => {
                    event.preventDefault();
                    void submitProjectDialog();
                  }}
                >
                  <header>
                    <div class="project-action-title-icon">
                      <Show when={dialog().kind === "rename"}><Pencil size={17} /></Show>
                      <Show when={dialog().kind === "worktree"}><GitBranch size={17} /></Show>
                      <Show when={dialog().kind === "archive"}><Archive size={17} /></Show>
                      <Show when={dialog().kind === "remove"}><X size={17} /></Show>
                    </div>
                    <div>
                      <strong>{dialog().kind === "rename" ? "重命名项目" : dialog().kind === "worktree" ? "创建永久工作树" : dialog().kind === "archive" ? "归档任务" : "移除项目"}</strong>
                      <span>{dialog().project.name}</span>
                    </div>
                    <button type="button" title="关闭" aria-label="关闭" disabled={projectActionBusy()} onClick={() => setProjectDialog(undefined)}><X size={15} /></button>
                  </header>

                  <div class="project-action-body">
                    <Show when={dialog().kind === "rename"}>
                      <label class="project-action-field">
                        <span>项目名称</span>
                        <input
                          value={dialog().name}
                          maxlength={200}
                          disabled={projectActionBusy()}
                          onInput={(event) => setProjectDialog((current) => current ? { ...current, name: event.currentTarget.value } : current)}
                        />
                      </label>
                      <p class="project-action-note">工作区位置保持不变。</p>
                    </Show>

                    <Show when={dialog().kind === "worktree"}>
                      <div class="project-action-source"><Folder size={14} /><span>{dialog().project.workspaceRoot}</span></div>
                      <label class="project-action-field">
                        <span>新分支</span>
                        <input
                          value={dialog().branch}
                          spellcheck={false}
                          disabled={projectActionBusy()}
                          onInput={(event) => {
                            const branch = event.currentTarget.value;
                            setProjectDialog((current) => current && current.kind === "worktree"
                              ? { ...current, branch }
                              : current);
                          }}
                        />
                      </label>
                      <label class="project-action-field">
                        <span>父目录</span>
                        <div class="project-action-path-input">
                          <div class="project-action-source"><Folder size={14} /><span>{dialog().destinationParent}</span></div>
                          <button type="button" title="选择父目录" aria-label="选择父目录" disabled={projectActionBusy()} onClick={() => void handleChooseWorktreeParent()}><FolderOpen size={15} /></button>
                        </div>
                      </label>
                      <label class="project-action-field">
                        <span>工作树名称</span>
                        <input
                          value={dialog().destinationName}
                          spellcheck={false}
                          disabled={projectActionBusy()}
                          onInput={(event) => setProjectDialog((current) => current && current.kind === "worktree" ? { ...current, destinationName: event.currentTarget.value } : current)}
                        />
                      </label>
                    </Show>

                    <Show when={dialog().kind === "archive"}>
                      <div class="project-action-confirm">
                        <Archive size={18} />
                        <div>
                          <strong>归档 {dialog().project.sessions.filter((session) => !session.archived).length} 个任务</strong>
                          <span>任务记录会保留，可通过侧栏的归档筛选重新查看。</span>
                        </div>
                      </div>
                    </Show>

                    <Show when={dialog().kind === "remove"}>
                      <div class="project-action-confirm danger">
                        <AlertTriangle size={18} />
                        <div>
                          <strong>从 MHcode 中移除此项目</strong>
                          <span>{projectTree().length <= 1 ? "会清理本地任务记录，并切换到 MHcodeProject 临时工作区；不会删除源码。" : "会清理本地任务记录，但不会删除工作区源码。"}</span>
                        </div>
                      </div>
                      <div class="project-action-source"><Folder size={14} /><span>{dialog().project.workspaceRoot}</span></div>
                    </Show>
                  </div>

                  <footer>
                    <button class="secondary" type="button" disabled={projectActionBusy()} onClick={() => setProjectDialog(undefined)}>取消</button>
                    <button
                      classList={{ danger: dialog().kind === "remove" }}
                      type="submit"
                      disabled={projectActionBusy() || (dialog().kind === "rename" && !dialog().name.trim()) || (dialog().kind === "worktree" && (!dialog().branch.trim() || !dialog().destinationParent.trim() || !dialog().destinationName.trim()))}
                    >
                      {projectActionBusy() ? "处理中" : dialog().kind === "rename" ? "保存" : dialog().kind === "worktree" ? "创建" : dialog().kind === "archive" ? "归档" : "移除"}
                    </button>
                  </footer>
                </form>
              </div>
            </Portal>
          )}
        </Show>

        <div class="sidebar-footer">
          <div class="avatar">M</div>
          <div>
            <strong>本地用户</strong>
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

      <div
        class="workbench-main"
        classList={{ "side-panel-open": Boolean(browserPreview()) || reviewOpen(), "browser-open": Boolean(browserPreview()), "review-open": reviewOpen(), "resizing-browser": resizingBrowserPanel() }}
        ref={workbenchRef}
        style={{ "--browser-panel-width": `${Math.round(browserPanelWidth())}px` } as JSX.CSSProperties}
      >
      <section class="chat-pane" classList={{ "workspace-tools-open": workspaceToolsOpen() }}>
        <header class="chat-header">
          <div>
            <strong>{activeSessionTitle()}</strong>
            <span>{modelName()}</span>
          </div>
          <div class="header-actions">
            <button type="button" onClick={() => openDrawer("cache")}>
              <ShieldCheck size={15} />
              {formatPercent(cacheHitRate(), hasCacheTokens())}
            </button>
            <Show when={backgroundTaskCount() > 0}>
              <span class="background-task-indicator" role="status" title={String(backgroundTaskCount()) + " 个会话正在后台生成"}>
                <Bot size={14} />
                {backgroundTaskCount()} 后台
              </span>
            </Show>
            <button type="button" title="回退时间线" aria-label="回退时间线" onClick={() => void openTimeline()}>
              <History size={15} />
            </button>
            <button
              type="button"
              classList={{ active: workspaceToolsOpen() }}
              title="Git 与终端"
              aria-label="Git 与终端"
              onClick={() => setWorkspaceToolsOpen((open) => !open)}
            >
              <Terminal size={15} />
            </button>
            <button
              type="button"
              classList={{ active: reviewOpen() }}
              title="代码审阅"
              aria-label="代码审阅"
              onClick={toggleReviewPanel}
            >
              <FileDiff size={15} />
            </button>
            <button type="button" title="内置浏览器" aria-label="内置浏览器" onClick={() => void handleOpenBrowser()}>
              <Globe2 size={15} />
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
              <button type="button" title="关闭错误提示" aria-label="关闭错误提示" onClick={() => setError("")}>
                <X size={14} />
              </button>
            </div>
          </Show>
        </div>

        <section
          class="chat-scroll"
          classList={{ empty: messages().length === 0 }}
          ref={chatScrollRef}
          aria-live="polite"
          onScroll={updateChatScrollState}
        >
          <For
            each={messages()}
            fallback={
              <div class="welcome-state">
                <h1>开始新任务</h1>
                <Show when={!deepSeek().configured}>
                  <button type="button" onClick={() => openDrawer("settings")}>
                    连接 DeepSeek
                  </button>
                </Show>
              </div>
            }
          >
            {(message) => (
              <Show
                when={message.role === "user"}
                fallback={
                  <article class="op-msg assistant" classList={{ system: message.role === "system", failed: message.failed }}>
                    <MessageContent
                      parts={message.parts && message.parts.length > 0 ? message.parts : textToParts(message.content)}
                      onPreviewFile={handlePreviewFile}
                      onOpenURL={requestOpenURL}
                    />
                    <Show when={(message.streaming && !message.parts?.some((part) => part.kind === "tool_call")) || message.cancelled}>
                      <div class="op-stream-state" classList={{ cancelled: message.cancelled }}>
                            <Show when={message.streaming}>
                          <Show when={message.statusKind === "compression"} fallback={<span class="op-thinking-spinner" />}>
                            <ListCollapse
                              class="op-compression-icon"
                              classList={{ completed: message.compressionStatus === "completed", error: message.compressionStatus === "error" }}
                              size={14}
                              aria-hidden="true"
                            />
                          </Show>
                        </Show>
                        <span>{message.status || (message.cancelled ? "已停止" : "正在生成")}</span>
                      </div>
                    </Show>
                    <Show when={message.role === "assistant" && !message.streaming && (message.content || message.parts?.length)}>
                      <div class="op-message-actions assistant-actions">
                        <button type="button" title="复制回复" aria-label="复制回复" onClick={() => void copyMessage(message)}>
                          <Show when={copiedMessageID() === message.id} fallback={<Copy size={14} />}><Check size={14} /></Show>
                        </button>
                        <Show when={message.eventId}>
                          <button
                            type="button"
                            title="从这条回复分叉"
                            aria-label="从这条回复分叉"
                            disabled={currentSessionBusy() || Boolean(forkingMessageID())}
                            onClick={() => void handleForkMessage(message)}
                          >
                            <GitFork size={14} />
                          </button>
                        </Show>
                      </div>
                    </Show>
                  </article>
                }
              >
                <article class="op-msg user">
                  <div class="op-user-stack">
                    <Show
                      when={editingMessageID() === message.id}
                      fallback={
                        <div class="op-user-bubble">
                          <Show when={message.attachments?.length}>
                            <div class="op-user-images">
                              <For each={message.attachments}>
                                {(attachment) => (
                                  <img
                                    class="previewable-image"
                                    src={chatAttachmentURL(attachment)}
                                    alt={attachment.name}
                                    title="双击查看原图"
                                    tabIndex={0}
                                    draggable={false}
                                    onDblClick={() => setPreviewAttachment(attachment)}
                                    onKeyDown={(event) => {
                                      if (event.key === "Enter" || event.key === " ") {
                                        event.preventDefault();
                                        setPreviewAttachment(attachment);
                                      }
                                    }}
                                  />
                                )}
                              </For>
                            </div>
                          </Show>
                          <Show when={message.content}>
                            <MessageContent
                              parts={textToParts(message.content)}
                              inferFileArtifacts={false}
                              onOpenURL={requestOpenURL}
                            />
                          </Show>
                        </div>
                      }
                    >
                      <div class="op-message-editor">
                        <Show when={editMessageAttachments().length > 0}>
                          <div class="composer-attachments compact">
                            <For each={editMessageAttachments()}>
                              {(attachment, index) => (
                                <figure>
                                  <img
                                    class="previewable-image"
                                    src={chatAttachmentURL(attachment)}
                                    alt={attachment.name}
                                    title="双击查看原图"
                                    tabIndex={0}
                                    draggable={false}
                                    onDblClick={() => setPreviewAttachment(attachment)}
                                    onKeyDown={(event) => {
                                      if (event.key === "Enter" || event.key === " ") {
                                        event.preventDefault();
                                        setPreviewAttachment(attachment);
                                      }
                                    }}
                                  />
                                  <button type="button" title="移除图片" aria-label={`移除 ${attachment.name}`} onClick={() => setEditMessageAttachments((current) => current.filter((_, itemIndex) => itemIndex !== index()))}><X size={12} /></button>
                                </figure>
                              )}
                            </For>
                          </div>
                        </Show>
                        <textarea
                          value={editMessageDraft()}
                          onInput={(event) => setEditMessageDraft(event.currentTarget.value)}
                          onKeyDown={(event) => {
                            if (event.key === "Escape") {
                              event.preventDefault();
                              cancelEditMessage();
                            }
                            if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
                              event.preventDefault();
                              void handleEditResend(message);
                            }
                          }}
                          rows={Math.min(8, Math.max(2, editMessageDraft().split("\n").length))}
                          spellcheck={false}
                          disabled={editingMessageBusy()}
                        />
                        <div class="op-editor-actions">
                          <button
                            type="button"
                            title="取消编辑"
                            aria-label="取消编辑"
                            disabled={editingMessageBusy()}
                            onClick={cancelEditMessage}
                          >
                            <X size={14} />
                          </button>
                          <button
                            type="button"
                            class="primary"
                            title="发送修改"
                            aria-label="发送修改"
                            disabled={(!editMessageDraft().trim() && editMessageAttachments().length === 0) || editingMessageBusy()}
                            onClick={() => void handleEditResend(message)}
                          >
                            <ArrowUp size={14} />
                          </button>
                        </div>
                      </div>
                    </Show>
                    <Show when={editingMessageID() !== message.id}>
                      <div class="op-message-actions user-actions">
                        <button type="button" title="复制消息" aria-label="复制消息" onClick={() => void copyMessage(message)}>
                          <Show when={copiedMessageID() === message.id} fallback={<Copy size={14} />}><Check size={14} /></Show>
                        </button>
                        <Show when={message.eventId}>
                          <button
                            type="button"
                            title="编辑并重新发送"
                            aria-label="编辑并重新发送"
                            disabled={currentSessionBusy() || Boolean(forkingMessageID())}
                            onClick={() => beginEditMessage(message)}
                          >
                            <Pencil size={14} />
                          </button>
                          <button
                            type="button"
                            title="从这条消息分叉"
                            aria-label="从这条消息分叉"
                            disabled={currentSessionBusy() || Boolean(forkingMessageID())}
                            onClick={() => void handleForkMessage(message)}
                          >
                            <GitFork size={14} />
                          </button>
                        </Show>
                      </div>
                    </Show>
                  </div>
                </article>
              </Show>
            )}
          </For>
        </section>

        <Show when={!chatNearBottom() && messages().length > 0}>
          <button
            class="chat-jump-bottom"
            type="button"
            title="跳到最新消息"
            aria-label="跳到最新消息"
            onClick={() => scrollChatToBottom()}
          >
            <ArrowDown size={16} />
          </button>
        </Show>

        <Show when={activeTaskProgress()}>
          {(progress) => (
            <section class="task-progress-dock" aria-label="当前任务进度" aria-live="polite">
              <TaskProgress part={progress()} />
            </section>
          )}
        </Show>

        <section class="composer-dock">
          <div class="composer-box">
            <div class="composer-project">
              <Folder size={14} />
              <button type="button" onClick={() => openSettings("index")}>
                {workspaceName()}
              </button>
            </div>
            <Show when={composerAttachments().length > 0}>
              <div class="composer-attachments">
                <For each={composerAttachments()}>
                  {(attachment, index) => (
                    <figure>
                      <img
                        class="previewable-image"
                        src={chatAttachmentURL(attachment)}
                        alt={attachment.name}
                        title="双击查看原图"
                        tabIndex={0}
                        draggable={false}
                        onDblClick={() => setPreviewAttachment(attachment)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter" || event.key === " ") {
                            event.preventDefault();
                            setPreviewAttachment(attachment);
                          }
                        }}
                      />
                      <figcaption>{attachment.name}</figcaption>
                      <button
                        type="button"
                        title="移除图片"
                        aria-label={`移除 ${attachment.name}`}
                        onClick={() => commitComposerSnapshot({
                          ...currentComposerSnapshot(),
                          attachments: composerAttachments().filter((_, itemIndex) => itemIndex !== index()),
                        })}
                      >
                        <X size={12} />
                      </button>
                    </figure>
                  )}
                </For>
              </div>
            </Show>
            <Show when={queuedMessages().length > 0}>
              <div class="composer-queue" aria-label={`已排队 ${queuedMessages().length} 条消息`}>
                <For each={queuedMessages()}>
                  {(queued, index) => (
                    <div class="composer-queue-item" classList={{ guidance: queued.guidance }}>
                      <span class="composer-queue-index" title={queued.guidance ? "下一条引导" : `队列第 ${index() + 1} 条`}>
                        <ListPlus size={13} />
                        {index() + 1}
                      </span>
                      <button class="composer-queue-copy" type="button" disabled={queued.guidance} title={queued.guidance ? "等待当前 Agent 应用" : "编辑这条排队消息"} onClick={() => editQueuedMessage(queued.id)}>
                        {composeComposerPrompt(queued.draft, queued.links, queued.tail) || `${queued.attachments.length} 张图片`}
                      </button>
                      <button
                        class="composer-queue-guide"
                        type="button"
                        disabled={index() === 0 && queued.guidance}
                        title={currentSessionBusy() ? "在当前步骤结束后引导正在执行的任务" : "设为队列下一条"}
                        onClick={() => void guideQueuedMessage(queued.id)}
                      >
                        <ArrowUp size={13} />
                        {queued.guidance ? "等待" : "引导"}
                      </button>
                      <button type="button" disabled={queued.guidance} title="编辑排队消息" aria-label="编辑排队消息" onClick={() => editQueuedMessage(queued.id)}><Pencil size={13} /></button>
                      <button type="button" disabled={queued.guidance} title="移除排队消息" aria-label="移除排队消息" onClick={() => setQueuedMessages((current) => removeMessage(current, queued.id))}><Trash2 size={13} /></button>
                    </div>
                  )}
                </For>
              </div>
            </Show>
            <div
              class="composer-rich-input"
              classList={{ "starts-with-link": composerLinks().length > 0 && promptDraft().trim().length === 0 }}
              onClick={(event) => {
              if (event.currentTarget === event.target) focusComposerEnd();
              }}
            >
              <div
                ref={composerEditorRef}
                class="composer-text-editor"
                role="textbox"
                aria-label="向 MHcode 提问，或描述要修改的代码"
                aria-multiline="true"
                data-placeholder="向 MHcode 提问，或描述要修改的代码"
                contentEditable={true}
                spellcheck={false}
                onInput={(event) => {
                  const value = composerEditorText(event.currentTarget);
                  commitComposerSnapshot({ ...currentComposerSnapshot(), draft: value });
                  if (/\s$/.test(value)) absorbComposerURLs(value, "prefix");
                }}
                onBlur={(event) => absorbComposerURLs(composerEditorText(event.currentTarget), "prefix")}
                onPaste={(event) => handleComposerPaste(event, "prefix")}
                onKeyDown={(event) => {
                  if (handleComposerHistoryShortcut(event)) return;
                  if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
                    event.preventDefault();
                    void sendMessage();
                  }
                }}
              />
              <For each={composerLinks()}>
                {(link) => (
                  <span class="composer-link-chip" title={link.url}>
                    <Globe2 class="composer-link-icon" size={12} aria-hidden="true" />
                    <button type="button" class="composer-link-target" aria-label={`打开 ${link.domain}`} onClick={() => requestOpenURL(link.url)}>{link.url}</button>
                    <button type="button" class="composer-link-remove" title="移除链接" aria-label={`移除 ${link.domain}`} onClick={() => removeComposerLink(link.url)}><X size={11} /></button>
                  </span>
                )}
              </For>
              <Show when={composerLinks().length > 0 || composerTailDraft().length > 0}>
                <div
                  ref={composerTailEditorRef}
                  class="composer-tail-editor"
                  role="textbox"
                  aria-label="在链接后继续输入"
                  aria-multiline="true"
                  contentEditable={true}
                  spellcheck={false}
                  onInput={(event) => {
                    const value = composerEditorText(event.currentTarget);
                    commitComposerSnapshot({ ...currentComposerSnapshot(), tail: value });
                    if (/\s$/.test(value)) absorbComposerURLs(value, "tail");
                  }}
                  onBlur={(event) => absorbComposerURLs(composerEditorText(event.currentTarget), "tail")}
                  onPaste={(event) => handleComposerPaste(event, "tail")}
                  onKeyDown={(event) => {
                    if (handleComposerHistoryShortcut(event)) return;
                    if (event.key === "Backspace" && !composerEditorText(event.currentTarget) && composerLinks().length > 0) {
                      event.preventDefault();
                      removeComposerLink(composerLinks()[composerLinks().length - 1].url);
                      return;
                    }
                    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
                      event.preventDefault();
                      void sendMessage();
                    }
                  }}
                />
              </Show>
            </div>
            <div class="composer-toolbar">
              <div>
                <input
                  ref={composerImageInputRef}
                  class="composer-image-input"
                  type="file"
                  accept="image/png,image/jpeg,image/webp,image/gif"
                  multiple
                  onChange={(event) => void addComposerImages(Array.from(event.currentTarget.files ?? []))}
                />
                <button type="button" title="添加图片" aria-label="添加图片" disabled={addingImages() || composerAttachments().length >= 4} onClick={() => composerImageInputRef?.click()}>
                  <ImagePlus size={17} />
                </button>
                <button type="button" title="添加上下文" onClick={() => openSettings("index")}>
                  <Plus size={17} />
                </button>
                <button
                  type="button"
                  title="撤销输入 (Ctrl+Z)"
                  aria-label="撤销输入"
                  disabled={composerUndoDepth() === 0}
                  onClick={undoComposerInput}
                >
                  <Undo2 size={16} />
                </button>
                <button
                  type="button"
                  title="重做输入 (Ctrl+Y)"
                  aria-label="重做输入"
                  disabled={composerRedoDepth() === 0}
                  onClick={redoComposerInput}
                >
                  <Redo2 size={16} />
                </button>
                <button
                  type="button"
                  class="access-toggle"
                  classList={{ active: fullAccess() }}
                  disabled={currentSessionBusy()}
                  title={fullAccess() ? "完全访问已开启：命令/文件修改不再逐次确认" : "点击开启完全访问（不再弹审批框）"}
                  onClick={() => void toggleFullAccess()}
                >
                  <ShieldCheck size={15} />
                  {fullAccess() ? "完全访问：开" : "完全访问"}
                </button>
                <button
                  type="button"
                  class="plan-toggle"
                  classList={{ active: planMode() }}
                  disabled={currentSessionBusy()}
                  title="Plan 模式：先规划后执行（需高/超高推理强度）"
                  onClick={() => void togglePlanMode()}
                >
                  <ClipboardList size={15} />
                  {planMode() ? "计划模式：开" : "计划模式"}
                </button>
                <button
                  type="button"
                  class="team-toggle"
                  classList={{ active: teamMode() }}
                  disabled={currentSessionBusy()}
                  title="AI 团队：规划、实现、测试、审阅和汇总使用可独立配置的模型"
                  onClick={() => void toggleTeamMode()}
                >
                  <Users size={15} />
                  {teamMode() ? "团队：开" : "AI 团队"}
                </button>
              </div>
              <div>
                <ModelRouteMenu
                  settings={runtimeSettings()}
                  saving={savingRuntime() || currentSessionBusy()}
                  onManage={() => openSettings("models")}
                  onSelect={(providerID, modelID) => void selectModelRoute(providerID, modelID)}
                />
                <ReasoningMenu
                  value={pendingReasoningLevel() ?? profile().id}
                  options={options()}
                  running={updatingReasoning() || (currentSessionBusy() && pendingReasoningLevel() !== undefined)}
                  onChange={changeReasoning}
                />
                <Show when={currentSessionBusy()}>
                  <button
                    class="queue-send-button"
                    type="button"
                    disabled={!canSend()}
                    onClick={() => enqueueCurrentMessage()}
                    title="将当前消息加入队列"
                    aria-label="将当前消息加入队列"
                  >
                    <ListPlus size={16} />
                  </button>
                </Show>
                <button
                  class="send-button"
                  classList={{ stop: currentSessionBusy() }}
                  type="button"
                  disabled={currentSessionBusy() ? !activeChatTaskID() : !canSend()}
                  onClick={() => void (currentSessionBusy() ? stopMessage() : sendMessage())}
                  title={currentSessionBusy() ? "停止生成" : "发送"}
                >
                  <Show when={currentSessionBusy()} fallback={<ArrowUp size={17} />}>
                    <Square size={13} fill="currentColor" />
                  </Show>
                </button>
              </div>
            </div>
          </div>
        </section>

        <WorkspaceToolsPanel
          open={workspaceToolsOpen()}
          workspaceRoot={runtimeSettings().workspaceRoot}
          shellAccess={runtimeSettings().shellAccess}
          readOnly={runtimeSettings().sandboxMode === "read-only" || runtimeSettings().filesystemAccess === "read-only"}
          onClose={() => setWorkspaceToolsOpen(false)}
        />
      </section>

      <Show when={browserPreview()}>
        {(preview) => (
          <>
            <div
              class="browser-panel-resizer"
				classList={{ "browser-surface-suspended": browserSurfaceSuspended() }}
              role="separator"
              aria-label="调整浏览器面板宽度"
              aria-orientation="vertical"
              aria-valuemin={Math.round(browserPanelLimits().min)}
              aria-valuemax={Math.round(browserPanelLimits().max)}
              aria-valuenow={Math.round(browserPanelWidth())}
              tabIndex={0}
              title="拖拽调整浏览器宽度，双击恢复默认"
              onPointerDown={startBrowserResize}
              onPointerMove={moveWindowPointerBrowserResize}
              onPointerUp={stopBrowserResize}
              onPointerCancel={stopBrowserResize}
              onMouseDown={startMouseBrowserResize}
              onDblClick={resetBrowserPanelWidth}
              onKeyDown={(event) => {
                if (event.key === "ArrowLeft") {
                  event.preventDefault();
                  nudgeBrowserPanelWidth(1);
                }
                if (event.key === "ArrowRight") {
                  event.preventDefault();
                  nudgeBrowserPanelWidth(-1);
                }
                if (event.key === "Home") {
                  event.preventDefault();
                  setAndPersistBrowserPanelWidth(browserPanelLimits().min);
                }
                if (event.key === "End") {
                  event.preventDefault();
                  setAndPersistBrowserPanelWidth(browserPanelLimits().max);
                }
                if (event.key === "Enter") {
                  event.preventDefault();
                  resetBrowserPanelWidth();
                }
              }}
            />
            <BrowserPreviewPanel
              preview={preview()}
              annotationPolicy={runtimeDraft()?.browser.screenshotAnnotations ?? "never"}
              credentials={runtimeDraft()?.browser.credentials ?? []}
				suspended={browserSurfaceSuspended()}
              onClose={() => setBrowserPreview(undefined)}
            />
          </>
        )}
      </Show>
      <Show when={reviewOpen()}>
        <>
          <div
            class="browser-panel-resizer"
            role="separator"
            aria-label="调整审阅面板宽度"
            aria-orientation="vertical"
            aria-valuemin={Math.round(browserPanelLimits().min)}
            aria-valuemax={Math.round(browserPanelLimits().max)}
            aria-valuenow={Math.round(browserPanelWidth())}
            tabIndex={0}
            title="拖拽调整审阅宽度，双击恢复默认"
            onPointerDown={startBrowserResize}
            onPointerMove={moveWindowPointerBrowserResize}
            onPointerUp={stopBrowserResize}
            onPointerCancel={stopBrowserResize}
            onMouseDown={startMouseBrowserResize}
            onDblClick={resetBrowserPanelWidth}
            onKeyDown={(event) => {
              if (event.key === "ArrowLeft") {
                event.preventDefault();
                nudgeBrowserPanelWidth(1);
              }
              if (event.key === "ArrowRight") {
                event.preventDefault();
                nudgeBrowserPanelWidth(-1);
              }
              if (event.key === "Home") {
                event.preventDefault();
                setAndPersistBrowserPanelWidth(browserPanelLimits().min);
              }
              if (event.key === "End") {
                event.preventDefault();
                setAndPersistBrowserPanelWidth(browserPanelLimits().max);
              }
              if (event.key === "Enter") {
                event.preventDefault();
                resetBrowserPanelWidth();
              }
            }}
          />
          <ReviewPanel
            open={reviewOpen()}
            workspaceRoot={runtimeSettings().workspaceRoot}
            readOnly={runtimeSettings().sandboxMode === "read-only" || runtimeSettings().filesystemAccess === "read-only"}
            onClose={() => setReviewOpen(false)}
          />
        </>
      </Show>
      </div>

      <Show when={drawerOpen()}>
        <aside class="settings-screen" aria-label="MHcode 设置">
          <div class="drawer-head">
            <button class="settings-back" type="button" onClick={closeDrawer}>
              <ArrowLeft size={16} />
              <span>返回工作区</span>
            </button>
            <strong>MHcode 设置</strong>
            <button class="ghost-icon" type="button" title="关闭设置" onClick={closeDrawer}>
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
            mcpServers={mcpServers()}
            models={deepSeek().models}
            nudgeSidebarWidth={nudgeSidebarWidth}
            profile={profile()}
            projectMemory={state()?.projectMemory ?? {
              enabled: true,
              projectName: "MHcode",
              sessionCount: 0,
              turnCount: 0,
              summary: "Project: MHcode",
            }}
            uiAppearance={uiAppearance()}
            effectiveUIScale={effectiveUIScale()}
            updateUIAppearance={updateUIAppearance}
            resetUIAppearance={resetUIAppearance}
            refreshMCPServer={refreshMCPRuntime}
            refreshingMCPID={refreshingMCPID()}
            providerKeyDrafts={providerKeyDrafts()}
            reasoningOptions={options()}
            runtimeDraft={activeRuntimeDraft()}
            sandboxCapabilities={state()?.sandboxCapabilities ?? {
              platform: "browser",
              backend: "preview-only",
              processTree: false,
              resourceLimits: false,
              privilegeIsolation: false,
              filesystemIsolation: false,
              networkIsolation: false,
              summary: "桌面运行时连接后显示系统沙箱能力。",
            }}
            runtimeDirty={Boolean(runtimeDraft()) || hasProviderKeyDrafts()}
            configFiles={configFiles()}
            clearProviderKey={clearProviderKey}
            deleteProvider={removeModelProvider}
            saveKey={saveKey}
            saveProviderKey={saveProviderKey}
            saveRuntime={saveRuntime}
            clearingProviderID={clearingProviderID()}
            deletingProviderID={deletingProviderID()}
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
            usageLedger={state()?.usageLedger}
          />
        </aside>
      </Show>

      <Show when={timelineOpen()}>
        <TimelinePanel
          checkpoints={checkpoints()}
          branches={branches()}
          onRewind={handleRewind}
          onSwitchBranch={handleSwitchBranch}
          onClose={() => setTimelineOpen(false)}
          busy={rewinding()}
        />
      </Show>

      <Show when={activeApproval()}>
        {(req) => <ApprovalModal request={req()} busy={approvalBusy()} onDecide={decideApproval} />}
      </Show>

      <Show when={pendingLinkURL()}>
        <div class="link-open-overlay" role="presentation" onPointerDown={(event) => {
          if (event.currentTarget === event.target && !linkOpenBusy()) setPendingLinkURL("");
        }}>
          <section class="link-open-dialog" role="dialog" aria-modal="true" aria-label="打开链接">
            <div class="link-open-head">
              <Globe2 size={16} />
              <strong>打开链接</strong>
              <button type="button" title="取消" aria-label="取消" disabled={Boolean(linkOpenBusy())} onClick={() => setPendingLinkURL("")}><X size={14} /></button>
            </div>
            <code>{pendingLinkURL()}</code>
            <div class="link-open-actions">
              <button type="button" disabled={Boolean(linkOpenBusy())} onClick={() => void openPendingLink("internal")}>
                <Globe2 size={15} />内置浏览器
              </button>
              <button type="button" disabled={Boolean(linkOpenBusy())} onClick={() => void openPendingLink("external")}>
                <ExternalLink size={15} />系统浏览器
              </button>
            </div>
          </section>
        </div>
      </Show>

      <Show when={previewAttachment()}>
        {(attachment) => <ImagePreviewModal attachment={attachment()} onClose={() => setPreviewAttachment(undefined)} />}
      </Show>
    </main>
  );
}

async function writeClipboardText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();
  const copied = document.execCommand("copy");
  textarea.remove();
  if (!copied) throw new Error("复制失败，请检查剪贴板权限。");
}

function chatAttachmentURL(attachment: ChatAttachment): string {
  return `data:${attachment.mimeType};base64,${attachment.data}`;
}

function approximateBase64Bytes(data: string): number {
  const padding = data.endsWith("==") ? 2 : data.endsWith("=") ? 1 : 0;
  return Math.max(0, Math.floor((data.length * 3) / 4) - padding);
}

async function readChatImage(file: File, index: number): Promise<ChatAttachment> {
  const mimeType = file.type.toLowerCase() === "image/jpg" ? "image/jpeg" : file.type.toLowerCase();
  if (!["image/png", "image/jpeg", "image/webp", "image/gif"].includes(mimeType)) {
    throw new Error(`不支持图片格式 ${file.type || file.name}，请使用 PNG、JPEG、WebP 或 GIF。`);
  }
  if (file.size === 0) throw new Error(`${file.name || "图片"} 为空。`);
  if (file.size > 6 * 1024 * 1024) throw new Error(`${file.name || "图片"} 超过 6 MB。`);
  const dataURL = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error ?? new Error("读取图片失败。"));
    reader.readAsDataURL(file);
  });
  const comma = dataURL.indexOf(",");
  if (comma < 0 || !dataURL.slice(comma + 1)) throw new Error("读取图片失败。");
  const extension = mimeType === "image/jpeg" ? "jpg" : mimeType.slice("image/".length);
  return {
    name: file.name || `clipboard-${Date.now()}-${index + 1}.${extension}`,
    mimeType,
    data: dataURL.slice(comma + 1),
  };
}

function extractComposerLinks(value: string): ComposerLink[] {
  const matches = value.match(/https?:\/\/[^\s<>"'`]+/gi) ?? [];
  const links: ComposerLink[] = [];
  const seen = new Set<string>();
  for (const match of matches) {
    const raw = match.replace(/[.,;:!?，。；：！？)\]}>》】）]+$/g, "");
    if (!raw || seen.has(raw)) continue;
    try {
      const parsed = new URL(raw);
      if (parsed.protocol !== "http:" && parsed.protocol !== "https:") continue;
      seen.add(raw);
      const domain = parsed.hostname.replace(/^www\./, "").toLowerCase();
      const segments = parsed.pathname.split("/").filter(Boolean);
      const label = domain === "github.com" && segments.length >= 2
        ? `${segments[0]}/${segments[1]}`
        : `${decodeURIComponent(parsed.pathname || "/")}${parsed.search}`;
      links.push({ url: raw, domain, label });
      if (links.length >= 3) break;
    } catch {
      // 输入中的半成品 URL 保持普通文本，完成后再显示预览。
    }
  }
  return links;
}

function removeComposerURLs(value: string, links: ComposerLink[]): string {
  let result = value;
  for (const link of links) {
    result = result.replace(`\`${link.url}\``, "").replace(link.url, "");
  }
  return result
    .replace(/[ \t]{2,}/g, " ")
    .replace(/ +\n/g, "\n")
    .trim();
}

function mergeComposerLinks(current: ComposerLink[], incoming: ComposerLink[]): ComposerLink[] {
  const merged = [...current];
  const seen = new Set(current.map((link) => link.url));
  for (const link of incoming) {
    if (seen.has(link.url)) continue;
    seen.add(link.url);
    merged.push(link);
    if (merged.length >= 3) break;
  }
  return merged;
}

function composeComposerPrompt(text: string, links: ComposerLink[], tail = ""): string {
  return [text.trim(), ...links.map((link) => link.url), tail.trim()].filter(Boolean).join(" ");
}

function splitComposerPrompt(value: string): { text: string; tail: string; links: ComposerLink[] } {
  const links = extractComposerLinks(value);
  return { text: removeComposerURLs(value, links), tail: "", links };
}

function composerEditorText(editor: HTMLDivElement): string {
  return editor.innerText.replace(/\r/g, "").replace(/\n$/, "");
}

function placeCaretAtEnd(element: HTMLElement): void {
  const selection = window.getSelection();
  if (!selection) return;
  const range = document.createRange();
  range.selectNodeContents(element);
  range.collapse(false);
  selection.removeAllRanges();
  selection.addRange(range);
}

function insertTextAtSelection(editor: HTMLDivElement | undefined, text: string): void {
  if (!editor || !text) return;
  editor.focus();
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || !editor.contains(selection.anchorNode)) {
    editor.append(document.createTextNode(text));
    placeCaretAtEnd(editor);
    return;
  }
  const range = selection.getRangeAt(0);
  range.deleteContents();
  const node = document.createTextNode(text);
  range.insertNode(node);
  range.setStartAfter(node);
  range.collapse(true);
  selection.removeAllRanges();
  selection.addRange(range);
}

function chatFailureMessage(message: string): string {
  const normalized = message.trim();
  if (/\b502\b|bad gateway/i.test(normalized)) {
    return "模型服务暂时不可用（HTTP 502）。MHcode 已自动重试，请稍后重试或切换模型供应商。";
  }
  if (/\b429\b|rate.?limit|too many requests/i.test(normalized)) {
    return "模型服务请求过于频繁（HTTP 429）。请稍后重试或切换模型供应商。";
  }
  if (/context deadline exceeded|timed? out|\bEOF\b/i.test(normalized)) {
    return "模型服务连接超时。请检查网络后重试，或切换模型供应商。";
  }
  return normalized || "模型请求失败。";
}

function updateLiveToolParts(parts: MessagePart[] | undefined, event: ChatTaskEvent): MessagePart[] {
  const name = event.toolName?.trim();
  if (!name) {
    return parts ?? [];
  }
  const toolCallId = name === "web_search" ? "live-web_search" : event.toolCallId || `live-${name}`;
  const status: "running" | "ok" | "error" = event.status === "error"
    ? "error"
    : event.status === "running"
      ? "running"
      : "ok";
  const next = [...(parts ?? [])];
  const index = next.findIndex(
    (part) => part.kind === "tool_call" && part.toolCallId === toolCallId,
  );
  const livePart: MessagePart = {
    kind: "tool_call",
    name,
    status,
    toolCallId,
    input: event.toolInput,
    output: status === "running" ? undefined : event.message,
  };
  if (index >= 0) {
    next[index] = { ...next[index], ...livePart } as MessagePart;
  } else {
    next.push(livePart);
  }
  return next;
}

function findTaskProgress(parts: MessagePart[] | undefined): TaskProgressPart | undefined {
  return parts?.find((part): part is TaskProgressPart => part.kind === "task_progress");
}

function cloneTaskProgress(progress: TaskProgressPart): TaskProgressPart {
  return {
    ...progress,
    steps: progress.steps.map((step) => ({ ...step })),
  };
}

function updateLiveProgressPart(
  parts: MessagePart[] | undefined,
  progress: Extract<MessagePart, { kind: "task_progress" }> | undefined,
): MessagePart[] {
  if (!progress) {
    return parts ?? [];
  }
  const next = [...(parts ?? [])];
  const index = next.findIndex((part) => part.kind === "task_progress");
  if (index >= 0) {
    next[index] = progress;
  } else {
    next.push(progress);
  }
  return next;
}

function updateLiveTeamPart(parts: MessagePart[] | undefined, event: ChatTaskEvent): MessagePart[] {
  const team = event.team;
  if (!team) {
    return parts ?? [];
  }
  const next = [...(parts ?? [])];
  const attempt = team.attempt || 1;
  const index = next.findIndex(
    (part) => part.kind === "team_role" && part.role === team.role && (part.attempt || 1) === attempt,
  );
  const rolePart: MessagePart = {
    kind: "team_role",
    role: team.role,
    roleLabel: team.label,
    providerId: team.providerId,
    model: team.model,
    status: team.status,
    summary: team.error || team.summary,
    verdict: team.verdict,
    attempt,
  };
  if (index >= 0) {
    next[index] = rolePart;
  } else {
    next.push(rolePart);
  }
  return next;
}

function mergeLiveToolResultParts(
  current: MessagePart[],
  resultParts: MessagePart[] | undefined,
): MessagePart[] {
  if (!resultParts?.length) {
    return current;
  }
  const next = [...current];
  for (const part of resultParts) {
    if (part.kind === "tool_call" || part.kind === "task_progress") {
      continue;
    }
    if (part.kind === "web_search_results") {
      const index = next.findIndex((item) => item.kind === "web_search_results");
      if (index >= 0) {
        next[index] = mergeWebSearchMessageParts(next[index] as Extract<MessagePart, { kind: "web_search_results" }>, part);
      } else {
        next.push(part);
      }
      continue;
    }
    next.push(part);
  }
  return next;
}

function mergeWebSearchMessageParts(
  current: Extract<MessagePart, { kind: "web_search_results" }>,
  incoming: Extract<MessagePart, { kind: "web_search_results" }>,
): Extract<MessagePart, { kind: "web_search_results" }> {
  const sources = [...current.sources];
  const seen = new Set(sources.map((source) => source.url.trim()));
  for (const source of incoming.sources) {
    const key = source.url.trim();
    if (!key || seen.has(key)) continue;
    if (sources.length >= 16) break;
    seen.add(key);
    sources.push(source);
  }
  const query = current.query && incoming.query && current.query !== incoming.query && !current.query.includes("含补充搜索")
    ? `${current.query}（含补充搜索）`
    : current.query || incoming.query;
  return { ...current, query, sources };
}

function settleLiveProgress(
  parts: MessagePart[] | undefined,
  taskStatus: "completed" | "failed" | "cancelled",
): MessagePart[] | undefined {
  if (!parts) {
    return parts;
  }
  return parts.map((part) => part.kind === "task_progress" ? { ...part, taskStatus } : part);
}

export default App;
