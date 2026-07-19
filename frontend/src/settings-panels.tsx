// 设置中心与各设置面板、共享控件、抽屉侧栏面板（从 app.tsx 抽离）。
import { For, Index, Match, Show, Switch, createEffect, createMemo, createSignal, onCleanup, onMount } from "solid-js";
import type { JSX } from "solid-js";
import {
  AlertTriangle, Archive, ArrowLeft, ArrowRight, ArrowUp, BarChart3, Bot, Braces,
  Check, CheckCircle2, ChevronDown, ChevronLeft, ChevronRight, Command, Cpu, Database, FileText,
  Folder, Gauge, GitBranch, Globe2, Hash, HardDrive, Keyboard, KeyRound, LayoutList,
  ListFilter, LockKeyhole, MessageSquarePlus, Monitor, Moon, Network, Palette, Plug,
  Plus, RefreshCw, Save, Search, Settings, ShieldCheck, SlidersHorizontal, Sparkles,
  Sun, Terminal, Trash2, User, Wrench, X, Zap,
  Users,
} from "lucide-solid";
import { ReasoningMenu } from "./components/ReasoningMenu";
import type {
  DeepSeekSessionState,
  MCPServerSetting,
  ModelProviderSetting,
  ReasoningLevel,
  RuntimeSettings,
  TeamRole,
  UsageMetrics,
  WorkbenchState,
} from "./types";
import type { SettingsCategory, ThemeMode, ProviderPreset } from "./ui-types";
import {
  sandboxOptions, filesystemOptions, approvalOptions, toolResultOptions,
  stablePrefixOptions, providerProtocolOptions, providerAPITypeOptions,
  providerPresets, sitePermissionOptions, settingsGroups,
} from "./constants";
import {
  formatPercent, formatInteger, formatCost, shortHash, runtimeLabel, sectionLabel,
  cacheStatusLabel, statusLabel, settingsGroupTitle, settingsCategoryDescription,
  findSettingsItem, baseNameFromPath, parentPath, compactPath, formatTokenWindow,
  selectedModelProvider, selectedModelName, modelOptionsForProvider,
  shortModelName, permissionLabel, providerBaseURLHint, providerBaseURLPlaceholder,
  providerFromPreset, createEmptyProvider, defaultAPITypeForProtocol,
  supportsModelFetchForProtocol, parseEnvLines, parseHeaderLines, prefixStatusLabel, thinkingStatusLabel,
} from "./format";
import { inferModelContextWindow, contextWindowSourceLabel } from "./model-context";
import { browserClearData, deleteBrowserCredential, saveBrowserCredential } from "./services/workbench";

export type SettingsCenterProps = {
  activeCategory: SettingsCategory;
  apiKeyDraft: string;
  cacheHealth: WorkbenchState["cacheHealth"];
  cacheHitRate: number;
  cacheTarget: number;
  configFiles: WorkbenchState["configFiles"];
  sandboxCapabilities: WorkbenchState["sandboxCapabilities"];
  clearingKey: boolean;
  clearingProviderID: string;
  clearKey: () => void;
  clearProviderKey: (providerID: string) => Promise<void> | void;
  deleteProvider: (providerID: string) => Promise<void> | void;
  deletingProviderID: string;
  contextPreview: WorkbenchState["contextPreview"] | undefined;
  deepSeek: WorkbenchState["deepSeek"];
  deepSeekSession: DeepSeekSessionState;
  diagnostics: string[];
  hasCacheTokens: boolean;
  mcpServers: WorkbenchState["mcpServers"];
  models: WorkbenchState["deepSeek"]["models"];
  projectMemory: WorkbenchState["projectMemory"];
  nudgeSidebarWidth: (direction: -1 | 1) => void;
  profile: WorkbenchState["reasoning"];
  refreshMCPServer: (serverID: string) => Promise<void> | void;
  refreshingMCPID: string;
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
  usageLedger?: WorkbenchState["usageLedger"];
};

export function SettingsCenter(props: SettingsCenterProps) {
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
                    {item.icon()}
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
              sandboxCapabilities={props.sandboxCapabilities}
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
              statuses={props.mcpServers}
              refreshMCPServer={props.refreshMCPServer}
              refreshingMCPID={props.refreshingMCPID}
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
              deleteProvider={props.deleteProvider}
              deletingProviderID={props.deletingProviderID}
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
          <Match when={props.activeCategory === "team"}>
            <TeamSettingsPanel
              resetRuntimeDraft={props.resetRuntimeDraft}
              runtimeDirty={props.runtimeDirty}
              runtimeDraft={props.runtimeDraft}
              saveRuntime={props.saveRuntime}
              savingRuntime={props.savingRuntime}
              updateRuntimeDraft={props.updateRuntimeDraft}
            />
          </Match>
          <Match when={props.activeCategory === "skills"}>
            <SkillsSettingsPanel skills={props.skills} snapshots={props.snapshots} />
          </Match>
          <Match when={props.activeCategory === "commands"}>
            <CommandSettingsPanel runtimeDraft={props.runtimeDraft} skills={props.skills} snapshots={props.snapshots} />
          </Match>
          <Match when={props.activeCategory === "memory"}>
            <MemorySettingsPanel
              memory={props.projectMemory}
              resetRuntimeDraft={props.resetRuntimeDraft}
              runtimeDirty={props.runtimeDirty}
              runtimeDraft={props.runtimeDraft}
              saveRuntime={props.saveRuntime}
              savingRuntime={props.savingRuntime}
              updateRuntimeDraft={props.updateRuntimeDraft}
            />
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
              usageLedger={props.usageLedger}
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

export function GeneralSettingsPanel(props: {
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
      <PanelSection icon={<Database size={16} />} title="上下文管理">
        <MetricGrid
          items={[
            ["模型窗口", props.deepSeekSession.contextWindowTokens ? formatTokenWindow(props.deepSeekSession.contextWindowTokens) : "下一轮识别"],
            ["可用输入", props.deepSeekSession.inputBudgetTokens ? formatTokenWindow(props.deepSeekSession.inputBudgetTokens) : "待计算"],
            ["估算占用", props.deepSeekSession.inputBudgetTokens ? `${(props.deepSeekSession.contextUsagePercent ?? 0).toFixed(1)}%` : "待计算"],
            ["长度来源", contextWindowSourceLabel(props.deepSeekSession.contextWindowSource)],
            ["压缩次数", `${formatInteger(props.deepSeekSession.compressionCount ?? 0)} 次`],
            ["已压缩消息", formatInteger(props.deepSeekSession.compressedMessageCount ?? 0)],
          ]}
        />
      </PanelSection>
    </div>
  );
}

export function AppearanceSettingsPanel(props: {
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

export function ConfigSettingsPanel(props: {
  configFiles: WorkbenchState["configFiles"];
  sandboxCapabilities: WorkbenchState["sandboxCapabilities"];
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
            title="系统级进程隔离"
            description={
              props.sandboxCapabilities.processTree
                ? "Windows Job Object 已接管 Agent 命令和终端进程树；停止、超时或退出时会回收全部子进程。"
                : props.sandboxCapabilities.summary
            }
            control={
              <span class="settings-runtime-status" classList={{ active: props.sandboxCapabilities.processTree }}>
                <ShieldCheck size={14} />
                {props.sandboxCapabilities.processTree ? "进程隔离已启用" : "进程隔离不可用"}
              </span>
            }
          />
          <SettingsRow
            title="管理员权限隔离"
            description="工作区与只读模式使用 Windows 有限用户令牌，禁用管理员组并降为 Medium 完整性；全权限模式保留当前用户令牌。目录白名单与断网仍由工具策略守卫控制。"
            control={
              <span
                class="settings-runtime-status"
                classList={{ active: props.sandboxCapabilities.privilegeIsolation && props.runtimeDraft.sandboxMode !== "danger-full-access" }}
              >
                <LockKeyhole size={14} />
                {props.runtimeDraft.sandboxMode === "danger-full-access" ? "完整用户令牌" : "受限令牌"}
              </span>
            }
          />
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
            title="进程树内存上限"
            description="单个 Agent 命令或终端会话可使用的最大总内存（MB）"
            control={
              <input
                class="settings-input numeric"
                type="number"
                min="256"
                max="65536"
                step="256"
                value={props.runtimeDraft.maxCommandMemoryMb}
                onInput={(event) => props.updateRuntimeDraft({ maxCommandMemoryMb: Number(event.currentTarget.value) })}
              />
            }
          />
          <SettingsRow
            title="CPU 上限"
            description="低于 100% 时由 Windows Job Object 对进程树应用硬上限"
            control={
              <input
                class="settings-input numeric"
                type="number"
                min="10"
                max="100"
                step="5"
                value={props.runtimeDraft.maxCommandCpuPercent}
                onInput={(event) => props.updateRuntimeDraft({ maxCommandCpuPercent: Number(event.currentTarget.value) })}
              />
            }
          />
          <SettingsRow
            title="最大活动进程数"
            description="限制单个命令或终端进程树同时存在的进程数量"
            control={
              <input
                class="settings-input numeric"
                type="number"
                min="4"
                max="1024"
                value={props.runtimeDraft.maxCommandProcesses}
                onInput={(event) => props.updateRuntimeDraft({ maxCommandProcesses: Number(event.currentTarget.value) })}
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
              <button class="settings-soft-button" type="button" disabled title="即将支持">
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
              <button class="settings-danger-button" type="button" disabled title="即将支持">
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

export function EnvironmentSettingsPanel(props: {
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

export function GitSettingsPanel(props: {
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

export function ModelSettingsPanel(props: {
  clearProviderKey: (providerID: string) => Promise<void> | void;
  clearingProviderID: string;
  deleteProvider: (providerID: string) => Promise<void> | void;
  deletingProviderID: string;
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
  const [entryMode, setEntryMode] = createSignal<"preset" | "custom">("custom");
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
    const context = inferModelContextWindow(modelID, provider.protocol, provider.contextWindowTokens);
    updateProvider(provider.id, {
      models: [
        ...provider.models,
        {
          id: modelID,
          displayName: modelID,
          provider: provider.id,
          contextWindowTokens: context.tokens,
          contextWindowSource: context.source,
        },
      ],
      defaultModelId: provider.defaultModelId || modelID,
    });
  };
  const updateProviderModel = (provider: ModelProviderSetting, modelIndex: number, patch: Partial<ModelProviderSetting["models"][number]>) => {
    const previous = provider.models[modelIndex];
    const models = provider.models.map((model, index) => (index === modelIndex ? { ...model, ...patch } : model));
    const nextID = typeof patch.id === "string" ? patch.id : previous?.id;
    updateModelSettings({
      providers: props.runtimeDraft.model.providers.map((item) =>
        item.id === provider.id
          ? { ...item, models, defaultModelId: item.defaultModelId === previous?.id ? nextID ?? "" : item.defaultModelId }
          : item,
      ),
      selectedModelId:
        props.runtimeDraft.model.selectedProviderId === provider.id && props.runtimeDraft.model.selectedModelId === previous?.id
          ? nextID ?? ""
          : props.runtimeDraft.model.selectedModelId,
    });
  };
  const removeProviderModel = (provider: ModelProviderSetting, modelIndex: number) => {
    const removed = provider.models[modelIndex];
    const models = provider.models.filter((_, index) => index !== modelIndex);
    const defaultModelId = provider.defaultModelId === removed?.id ? models[0]?.id ?? "" : provider.defaultModelId;
    updateModelSettings({
      providers: props.runtimeDraft.model.providers.map((item) =>
        item.id === provider.id ? { ...item, models, defaultModelId } : item,
      ),
      selectedModelId:
        props.runtimeDraft.model.selectedProviderId === provider.id && props.runtimeDraft.model.selectedModelId === removed?.id
          ? defaultModelId
          : props.runtimeDraft.model.selectedModelId,
    });
  };
  const setCurrentRoute = (provider: ModelProviderSetting, modelID?: string) => {
    const selectedModelID = modelID ?? (provider.defaultModelId || provider.models[0]?.id || "");
    updateModelSettings({
      selectedProviderId: provider.id,
      selectedModelId: selectedModelID,
      providers: props.runtimeDraft.model.providers.map((item) =>
        item.id === provider.id ? { ...item, enabled: true, defaultModelId: selectedModelID || item.defaultModelId } : item,
      ),
    });
  };

  return (
    <div class="settings-page-body model-settings-page">
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
        class="model-editor-section"
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
                  const configured = () =>
                    props.runtimeDraft.model.providers.some((provider) => provider.id === preset.id);
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
                    <IconButton
                      title="删除供应商"
                      danger
                      disabled={props.deletingProviderID === provider().id}
                      onClick={() => void props.deleteProvider(provider().id)}
                    >
                      <Trash2 size={14} />
                    </IconButton>
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
                    title="Token 价格（每百万）"
                    description="可选。用于估算输入、输出和缓存读写成本；留空或填 0 表示不估算。"
                    control={
                      <div class="provider-pricing-grid">
                        <label>
                          <span>输入</span>
                          <input
                            class="settings-input numeric"
                            type="number"
                            min="0"
                            step="0.000001"
                            value={provider().inputPricePerMillion ?? 0}
                            onInput={(event) => updateProvider(provider().id, { inputPricePerMillion: Math.max(0, Number(event.currentTarget.value) || 0) })}
                          />
                        </label>
                        <label>
                          <span>输出</span>
                          <input
                            class="settings-input numeric"
                            type="number"
                            min="0"
                            step="0.000001"
                            value={provider().outputPricePerMillion ?? 0}
                            onInput={(event) => updateProvider(provider().id, { outputPricePerMillion: Math.max(0, Number(event.currentTarget.value) || 0) })}
                          />
                        </label>
                        <label>
                          <span>缓存命中</span>
                          <input
                            class="settings-input numeric"
                            type="number"
                            min="0"
                            step="0.000001"
                            value={provider().cacheHitPricePerMillion ?? 0}
                            onInput={(event) => updateProvider(provider().id, { cacheHitPricePerMillion: Math.max(0, Number(event.currentTarget.value) || 0) })}
                          />
                        </label>
                        <label>
                          <span>缓存未命中</span>
                          <input
                            class="settings-input numeric"
                            type="number"
                            min="0"
                            step="0.000001"
                            value={provider().cacheMissPricePerMillion ?? 0}
                            onInput={(event) => updateProvider(provider().id, { cacheMissPricePerMillion: Math.max(0, Number(event.currentTarget.value) || 0) })}
                          />
                        </label>
                      </div>
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
                  <div class="provider-model-columns" aria-hidden="true">
                    <span>当前</span>
                    <span>模型 ID</span>
                    <span>显示名称</span>
                    <span>上下文 / 来源</span>
                    <span />
                  </div>
                  <Index each={provider().models} fallback={<p class="empty-line">暂无模型，可手动添加或从上游获取。</p>}>
                    {(model, index) => (
                      <div
                        class="provider-model-row"
                        classList={{ selected: props.runtimeDraft.model.selectedProviderId === provider().id && props.runtimeDraft.model.selectedModelId === model().id }}
                      >
                        <IconButton title="设为当前模型" onClick={() => setCurrentRoute(provider(), model().id)}>
                          <Show
                            when={props.runtimeDraft.model.selectedProviderId === provider().id && props.runtimeDraft.model.selectedModelId === model().id}
                            fallback={<span class="model-route-dot" />}
                          >
                            <Check size={14} />
                          </Show>
                        </IconButton>
                        <input
                          class="settings-input"
                          value={model().id}
                          spellcheck={false}
                          placeholder="模型 ID"
                          onInput={(event) => {
                            const id = event.currentTarget.value;
                            const context = inferModelContextWindow(id, provider().protocol, provider().contextWindowTokens);
                            const displayName = !model().displayName || model().displayName === model().id
                              ? id
                              : model().displayName;
                            updateProviderModel(provider(), index, {
                              id,
                              displayName,
                              ...(model().contextWindowSource === "manual"
                                ? {}
                                : { contextWindowTokens: context.tokens, contextWindowSource: context.source }),
                            });
                          }}
                        />
                        <input
                          class="settings-input"
                          value={model().displayName}
                          placeholder="显示名称"
                          onInput={(event) => updateProviderModel(provider(), index, { displayName: event.currentTarget.value })}
                        />
                        <div class="model-context-cell">
                          <input
                            class="settings-input numeric"
                            type="number"
                            min="0"
                            step="1"
                            value={model().contextWindowTokens}
                            title="模型上下文窗口；输入 0 会重新自动识别"
                            onInput={(event) => {
                              const tokens = Math.max(0, Number(event.currentTarget.value) || 0);
                              if (tokens > 0) {
                                updateProviderModel(provider(), index, {
                                  contextWindowTokens: tokens,
                                  contextWindowSource: "manual",
                                });
                                return;
                              }
                              const context = inferModelContextWindow(model().id, provider().protocol, provider().contextWindowTokens);
                              updateProviderModel(provider(), index, {
                                contextWindowTokens: context.tokens,
                                contextWindowSource: context.source,
                              });
                            }}
                          />
                          <small>{contextWindowSourceLabel(model().contextWindowSource)}</small>
                        </div>
                        <IconButton title={`删除模型 ${model().displayName || model().id}`} danger onClick={() => removeProviderModel(provider(), index)}>
                          <Trash2 size={14} />
                        </IconButton>
                      </div>
                    )}
                  </Index>
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

      <SettingsSection class="model-provider-section" title="已配置供应商">
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
                  <span
                    class="provider-route-state"
                    classList={{
                      current: props.runtimeDraft.model.selectedProviderId === provider.id,
                      disabled: !provider.enabled,
                      error: provider.lastSyncStatus === "error",
                    }}
                  >
                    <i />
                    {props.runtimeDraft.model.selectedProviderId === provider.id ? "当前" : provider.enabled ? "启用" : "停用"}
                  </span>
                </div>
                <div class="provider-summary-meta">
                  <span><Database size={12} />{provider.models.length} 个模型</span>
                  <span><KeyRound size={12} />{provider.apiKeyConfigured ? "密钥已保存" : provider.protocol === "local" ? "无需密钥" : "未保存密钥"}</span>
                  <span><Hash size={12} />{formatTokenWindow(provider.contextWindowTokens)}</span>
                </div>
              </button>
            )}
          </For>
        </div>
      </SettingsSection>

    </div>
  );
}

const teamRoleMeta: Record<TeamRole, { label: string; description: string; locked: boolean }> = {
  planner: { label: "规划员", description: "只读调研并拆解执行步骤", locked: false },
  implementer: { label: "实现者", description: "唯一拥有完整写入工具的角色", locked: true },
  tester: { label: "测试核验", description: "只读检查验证证据和回归风险", locked: false },
  reviewer: { label: "代码审阅", description: "只读审查 diff、边界与测试缺口", locked: false },
  synthesizer: { label: "结果汇总", description: "整合团队结论并生成最终答复", locked: true },
};

export function TeamSettingsPanel(props: {
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveRuntime: () => void;
  savingRuntime: boolean;
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  resetRuntimeDraft: () => void;
}) {
  const updateTeam = (patch: Partial<RuntimeSettings["team"]>) => {
    props.updateRuntimeDraft({ team: { ...props.runtimeDraft.team, ...patch } });
  };
  const updateRole = (role: TeamRole, patch: Partial<RuntimeSettings["team"]["roles"][number]>) => {
    updateTeam({
      roles: props.runtimeDraft.team.roles.map((item) => item.role === role ? { ...item, ...patch } : item),
    });
  };
  const providerForRole = (providerID: string) => props.runtimeDraft.model.providers.find((provider) => provider.id === providerID);

  return (
    <div class="settings-page-body">
      <SettingsSection title="团队编排">
        <SettingsRow
          icon={<Users size={16} />}
          title="启用 AI 团队模式"
          description="高或超高推理下依次完成规划、实现、核验、审阅和结果汇总"
          control={<SwitchControl checked={props.runtimeDraft.team.enabled} onChange={(enabled) => updateTeam({ enabled })} />}
        />
        <SettingsRow
          icon={<RefreshCw size={16} />}
          title="审阅返工轮数"
          description="测试或审阅要求修改时，重新交给实现者的最大次数"
          control={
            <SelectControl
              value={String(props.runtimeDraft.team.maxReviewRounds)}
              options={[
                { value: "0", label: "不自动返工" },
                { value: "1", label: "最多 1 轮" },
                { value: "2", label: "最多 2 轮" },
              ]}
              onChange={(value) => updateTeam({ maxReviewRounds: Number(value) })}
            />
          }
        />
      </SettingsSection>

      <SettingsSection title="角色与模型">
        <div class="team-role-list">
          <For each={props.runtimeDraft.team.roles}>
            {(role) => {
              const meta = () => teamRoleMeta[role.role];
              const provider = () => providerForRole(role.providerId);
              const models = () => provider() ? modelOptionsForProvider(provider()!) : [];
              return (
                <div class="team-role-row" classList={{ disabled: !role.enabled }}>
                  <span class="team-role-icon" aria-hidden="true"><Bot size={16} /></span>
                  <span class="team-role-copy">
                    <strong>{meta().label}</strong>
                    <small>{meta().description}</small>
                  </span>
                  <div class="team-role-route">
                    <select
                      class="settings-select"
                      aria-label={`${meta().label}供应商`}
                      value={role.providerId}
                      disabled={!role.enabled}
                      onChange={(event) => updateRole(role.role, { providerId: event.currentTarget.value, modelId: "" })}
                    >
                      <option value="">跟随当前供应商</option>
                      <For each={props.runtimeDraft.model.providers.filter((item) => item.enabled)}>
                        {(item) => <option value={item.id}>{item.name}</option>}
                      </For>
                    </select>
                    <select
                      class="settings-select"
                      aria-label={`${meta().label}模型`}
                      value={role.modelId}
                      disabled={!role.enabled || !role.providerId || models().length === 0}
                      onChange={(event) => updateRole(role.role, { modelId: event.currentTarget.value })}
                    >
                      <option value="">{role.providerId ? "使用供应商默认模型" : "跟随当前模型"}</option>
                      <For each={models()}>
                        {(model) => <option value={model.id}>{model.displayName || model.id}</option>}
                      </For>
                    </select>
                  </div>
                  <Show
                    when={!meta().locked}
                    fallback={<span class="team-role-required" title="团队核心角色始终启用"><LockKeyhole size={13} />必需</span>}
                  >
                    <SwitchControl checked={role.enabled} onChange={(enabled) => updateRole(role.role, { enabled })} />
                  </Show>
                </div>
              );
            }}
          </For>
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

export function SkillsSettingsPanel(props: {
  skills: WorkbenchState["skillsIndex"];
  snapshots: WorkbenchState["mcpSnapshots"];
}) {
  const builtin = createMemo(() => props.snapshots.find((snapshot) => snapshot.server === "builtin"));
  return (
    <div class="settings-page-body">
      <PanelSection icon={<Wrench size={16} />} title={`内置 Agent 工具 · ${formatInteger(builtin()?.tools.length ?? 0)}`}>
        <div class="agent-tool-list">
          <For each={builtin()?.tools ?? []} fallback={<p class="empty-line">当前运行时未暴露内置工具</p>}>
            {(tool) => (
              <div class="agent-tool-row">
                <span class="agent-tool-icon" aria-hidden="true"><Braces size={15} /></span>
                <span class="agent-tool-main">
                  <code>{tool.name}</code>
                  <small>{builtinToolDescription(tool.name)}</small>
                </span>
                <span class="agent-tool-meta">
                  <strong>可调用</strong>
                  <small>{tool.outputPolicy} · {shortHash(tool.inputSchemaHash)}</small>
                </span>
              </div>
            )}
          </For>
        </div>
      </PanelSection>
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
          <RouteStep icon={<Wrench size={15} />} title="Built-in tools" detail={`${formatInteger(builtin()?.tools.length ?? 0)} callable`} />
          <RouteStep icon={<Network size={15} />} title="MCP schema" detail={`${formatInteger(props.snapshots.filter((snapshot) => snapshot.server !== "builtin").length)} external snapshot`} />
          <RouteStep icon={<FileText size={15} />} title="Summary-first results" detail="raw output stays local" />
        </div>
      </PanelSection>
    </div>
  );
}

function builtinToolDescription(name: string): string {
  switch (name) {
    case "update_plan": return "维护 Agent 执行计划和步骤状态";
    case "read_file": return "按行读取文本，返回编码、行尾和 sha256";
    case "file_info": return "读取文件或目录元数据，不加载完整内容";
    case "list_dir": return "按深度列出工作区目录树";
    case "search": return "文本或正则搜索，支持文件过滤";
    case "write_file": return "原子创建或覆盖文本文件";
    case "apply_patch": return "执行单个或批量精确文本替换";
    case "copy_file": return "保留编码与行尾复制文本文件";
    case "delete_file": return "删除文本文件并记录 rewind 快照";
    case "open_file": return "使用内置预览或系统应用打开文件";
    case "read_repository": return "读取真实 Git 仓库树和源码";
    case "web_search": return "联网搜索并保留来源链接";
    case "browser": return "操作 MHcode 内置浏览器";
    case "computer": return "操作获准的其他桌面窗口";
    case "run_command": return "仅运行构建、测试、编译器和程序命令";
    case "git": return "结构化执行状态、diff、暂存、提交和分支操作";
    case "terminal": return "管理 UTF-8 持久终端会话";
    default: return "由 MHcode 运行时提供的模型工具";
  }
}

const mcpTransportOptions = [
  { value: "builtin", label: "MHcode 内置" },
  { value: "stdio", label: "本地进程（stdio）" },
  { value: "streamable-http", label: "Streamable HTTP" },
  { value: "sse", label: "旧版 SSE" },
];

function mcpStatusLabel(state?: string) {
  switch (state) {
    case "ready": return "运行中";
    case "error": return "连接失败";
    case "disabled": return "已停用";
    default: return "等待连接";
  }
}

function mcpStatusTone(state?: string): "good" | "bad" | "watch" | "neutral" {
  switch (state) {
    case "ready": return "good";
    case "error": return "bad";
    case "idle": return "watch";
    default: return "neutral";
  }
}

export function McpSettingsPanel(props: {
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveRuntime: () => void;
  savingRuntime: boolean;
  snapshots: WorkbenchState["mcpSnapshots"];
  statuses: WorkbenchState["mcpServers"];
  refreshMCPServer: (serverID: string) => Promise<void> | void;
  refreshingMCPID: string;
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  resetRuntimeDraft: () => void;
}) {
  const [activeServerID, setActiveServerID] = createSignal(props.runtimeDraft.mcp.servers[0]?.id || "");
  const updateServers = (servers: MCPServerSetting[]) => {
    props.updateRuntimeDraft({ mcp: { ...props.runtimeDraft.mcp, servers } });
  };
  const updateServer = (id: string, patch: Partial<MCPServerSetting>) => {
    updateServers(props.runtimeDraft.mcp.servers.map((server) => (server.id === id ? { ...server, ...patch } : server)));
  };
  const activeServer = createMemo(
    () => props.runtimeDraft.mcp.servers.find((server) => server.id === activeServerID()) ?? props.runtimeDraft.mcp.servers[0],
  );
  const activeStatus = createMemo(() => props.statuses.find((status) => status.id === activeServer()?.id));
  const statusFor = (id: string) => props.statuses.find((status) => status.id === id);

  createEffect(() => {
    if (!props.runtimeDraft.mcp.servers.some((server) => server.id === activeServerID())) {
      setActiveServerID(props.runtimeDraft.mcp.servers[0]?.id || "");
    }
  });

  const addServer = () => {
    let index = props.runtimeDraft.mcp.servers.length + 1;
    while (props.runtimeDraft.mcp.servers.some((server) => server.id === `mcp-${index}`)) {
      index += 1;
    }
    const server: MCPServerSetting = {
      id: `mcp-${index}`,
      name: `MCP ${index}`,
      transport: "stdio",
      command: "",
      args: [],
      env: [],
      passEnvironment: [],
      workingDirectory: "",
      url: "",
      headers: [],
      enabled: false,
      toolResultPolicy: "summary-first",
    };
    updateServers([...props.runtimeDraft.mcp.servers, server]);
    setActiveServerID(server.id);
  };
  const removeServer = (server: MCPServerSetting) => {
    if (server.transport === "builtin" || !window.confirm(`删除 MCP 服务器“${server.name || server.id}”？`)) {
      return;
    }
    updateServers(props.runtimeDraft.mcp.servers.filter((item) => item.id !== server.id));
  };
  const changeTransport = (server: MCPServerSetting, transport: string) => {
    updateServer(server.id, {
      transport,
      command: transport === "builtin" ? server.command : server.command.startsWith("builtin:") ? "" : server.command,
    });
  };
  const connectionSummary = (server: MCPServerSetting) => {
    if (server.transport === "builtin") return "由 MHcode 运行时提供";
    if (server.transport === "stdio") return server.command || "尚未设置启动命令";
    return server.url || "尚未设置服务器 URL";
  };

  return (
    <div class="settings-page-body mcp-settings-page">
      <SettingsSection
        class="mcp-server-list-section"
        title="MCP 服务器"
        action={
          <IconButton title="添加 MCP 服务器" onClick={addServer}>
            <Plus size={15} />
          </IconButton>
        }
      >
        <div class="mcp-server-list">
          <For each={props.runtimeDraft.mcp.servers} fallback={<p class="settings-empty-box">尚未添加 MCP 服务器</p>}>
            {(server) => {
              const status = () => statusFor(server.id);
              return (
                <button
                  class="mcp-server-option"
                  classList={{ active: activeServerID() === server.id }}
                  type="button"
                  onClick={() => setActiveServerID(server.id)}
                >
                  <span class="mcp-status-dot" classList={{ [status()?.state || "idle"]: true }} />
                  <span class="mcp-server-copy">
                    <strong>{server.name || server.id}</strong>
                    <small>{runtimeLabel(mcpTransportOptions, server.transport)}</small>
                  </span>
                  <Show when={status()?.toolCount}>
                    <span class="mcp-tool-count">{status()?.toolCount}</span>
                  </Show>
                </button>
              );
            }}
          </For>
        </div>
      </SettingsSection>

      <SettingsSection class="mcp-server-editor-section" title="连接配置">
        <Show when={activeServer()} fallback={<p class="settings-empty-box">添加服务器后在这里配置连接</p>}>
          {(server) => (
            <div class="mcp-server-editor">
              <div class="mcp-editor-head">
                <div>
                  <strong>{server().name || server().id}</strong>
                  <span>{connectionSummary(server())}</span>
                </div>
                <div class="settings-row-actions">
                  <StatusPill
                    icon={activeStatus()?.state === "ready" ? <CheckCircle2 size={13} /> : <Network size={13} />}
                    label={mcpStatusLabel(activeStatus()?.state)}
                    tone={mcpStatusTone(activeStatus()?.state)}
                  />
                  <SwitchControl checked={server().enabled} onChange={(value) => updateServer(server().id, { enabled: value })} />
                  <IconButton
                    title="保存配置并重新连接"
                    disabled={props.refreshingMCPID === server().id}
                    onClick={() => void props.refreshMCPServer(server().id)}
                  >
                    <RefreshCw size={14} classList={{ spinning: props.refreshingMCPID === server().id }} />
                  </IconButton>
                  <Show when={server().transport !== "builtin"}>
                    <IconButton title="删除 MCP 服务器" danger onClick={() => removeServer(server())}>
                      <Trash2 size={14} />
                    </IconButton>
                  </Show>
                </div>
              </div>

              <div class="mcp-runtime-status" classList={{ error: activeStatus()?.state === "error" }}>
                <span>{activeStatus()?.message || "保存配置后可连接并读取工具列表"}</span>
                <Show when={activeStatus()?.protocolVersion || activeStatus()?.serverVersion}>
                  <code>{[activeStatus()?.protocolVersion, activeStatus()?.serverVersion].filter(Boolean).join(" · ")}</code>
                </Show>
              </div>

              <SettingsCard>
                <SettingsRow
                  title="服务器名称"
                  description={`稳定标识：${server().id}`}
                  control={
                    <input
                      class="settings-input row-control"
                      value={server().name}
                      onInput={(event) => updateServer(server().id, { name: event.currentTarget.value })}
                    />
                  }
                />
                <SettingsRow
                  title="连接方式"
                  description="本地进程使用 stdio，远程服务优先使用 Streamable HTTP"
                  control={
                    <select
                      class="settings-select"
                      value={server().transport}
                      disabled={server().transport === "builtin"}
                      onChange={(event) => changeTransport(server(), event.currentTarget.value)}
                    >
                      <For each={mcpTransportOptions}>
                        {(option) => <option value={option.value}>{option.label}</option>}
                      </For>
                    </select>
                  }
                />

                <Show when={server().transport === "stdio"}>
                  <SettingsRow
                    title="启动命令"
                    description="例如 npx、node、python 或可执行文件绝对路径"
                    control={
                      <input
                        class="settings-input row-control"
                        value={server().command}
                        spellcheck={false}
                        placeholder="npx"
                        onInput={(event) => updateServer(server().id, { command: event.currentTarget.value })}
                      />
                    }
                  />
                  <SettingsRow
                    title="启动参数"
                    description="每行一个参数，顺序会原样保留"
                    control={
                      <textarea
                        class="settings-textarea row-control"
                        rows={3}
                        spellcheck={false}
                        value={server().args.join("\n")}
                        onInput={(event) => updateServer(server().id, { args: event.currentTarget.value.split(/\r?\n/) })}
                      />
                    }
                  />
                  <SettingsRow
                    title="工作目录"
                    description="留空时使用当前工作区"
                    control={
                      <input
                        class="settings-input row-control"
                        value={server().workingDirectory}
                        spellcheck={false}
                        onInput={(event) => updateServer(server().id, { workingDirectory: event.currentTarget.value })}
                      />
                    }
                  />
                  <SettingsRow
                    title="环境变量"
                    description="每行 KEY=VALUE；只传给该 MCP 子进程"
                    control={
                      <textarea
                        class="settings-textarea row-control"
                        rows={3}
                        spellcheck={false}
                        value={server().env.map((item) => `${item.key}=${item.value}`).join("\n")}
                        onInput={(event) => updateServer(server().id, { env: parseEnvLines(event.currentTarget.value) })}
                      />
                    }
                  />
                  <SettingsRow
                    title="透传系统变量"
                    description="每行一个变量名；PATH 等基础变量会自动保留"
                    control={
                      <textarea
                        class="settings-textarea row-control"
                        rows={3}
                        spellcheck={false}
                        value={server().passEnvironment.join("\n")}
                        onInput={(event) => updateServer(server().id, { passEnvironment: event.currentTarget.value.split(/\r?\n/) })}
                      />
                    }
                  />
                </Show>

                <Show when={server().transport === "streamable-http" || server().transport === "sse"}>
                  <SettingsRow
                    title="服务器 URL"
                    description={server().transport === "sse" ? "旧版 SSE 事件端点" : "MCP Streamable HTTP 端点"}
                    control={
                      <input
                        class="settings-input row-control"
                        value={server().url}
                        spellcheck={false}
                        placeholder="https://example.com/mcp"
                        onInput={(event) => updateServer(server().id, { url: event.currentTarget.value })}
                      />
                    }
                  />
                  <SettingsRow
                    title="请求头"
                    description="每行 Header: value；可配置 Bearer Token 等认证信息"
                    control={
                      <textarea
                        class="settings-textarea row-control"
                        rows={3}
                        spellcheck={false}
                        value={server().headers.map((item) => `${item.key}: ${item.value}`).join("\n")}
                        onInput={(event) => updateServer(server().id, { headers: parseHeaderLines(event.currentTarget.value) })}
                      />
                    }
                  />
                  <Show when={!props.runtimeDraft.networkAccess}>
                    <SettingsRow
                      danger
                      title="网络访问已关闭"
                      description="远程 MCP 需要先在环境设置中开启网络访问"
                    />
                  </Show>
                </Show>

                <Show when={server().transport === "builtin"}>
                  <SettingsRow title="内置运行时" description="该服务随 MHcode 启动，不需要额外命令或网络连接" />
                </Show>

                <SettingsRow
                  title="工具结果策略"
                  description="控制远程工具结果进入模型上下文时的最大保留量"
                  control={
                    <SelectControl
                      value={server().toolResultPolicy}
                      options={toolResultOptions}
                      onChange={(value) => updateServer(server().id, { toolResultPolicy: value })}
                    />
                  }
                />
              </SettingsCard>
            </div>
          )}
        </Show>
        <RuntimeSaveActions
          dirty={props.runtimeDirty}
          reset={props.resetRuntimeDraft}
          save={props.saveRuntime}
          saving={props.savingRuntime}
        />
      </SettingsSection>

      <SettingsSection class="mcp-snapshot-section" title="工具快照">
        <div class="item-list">
          <For each={props.snapshots} fallback={<p class="settings-empty-box">连接服务器后会在这里记录稳定 schema 快照</p>}>
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

export function BrowserSettingsPanel(props: {
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveRuntime: () => void;
  savingRuntime: boolean;
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  resetRuntimeDraft: () => void;
}) {
	const [credentialOrigin, setCredentialOrigin] = createSignal("");
	const [credentialUsername, setCredentialUsername] = createSignal("");
	const [credentialPassword, setCredentialPassword] = createSignal("");
	const [credentialBusy, setCredentialBusy] = createSignal("");
	const [credentialError, setCredentialError] = createSignal("");
	const [clearingBrowserData, setClearingBrowserData] = createSignal(false);
	const [browserDataMessage, setBrowserDataMessage] = createSignal("");
  const updateBrowser = (patch: Partial<RuntimeSettings["browser"]>) => {
    props.updateRuntimeDraft({ browser: { ...props.runtimeDraft.browser, ...patch } });
  };
  const updateAutofill = (patch: Partial<RuntimeSettings["browser"]["autofillProfile"]>) => {
    updateBrowser({ autofillProfile: { ...props.runtimeDraft.browser.autofillProfile, ...patch } });
  };
	const saveCredential = async () => {
		setCredentialBusy("save");
		setCredentialError("");
		try {
			const next = await saveBrowserCredential("", credentialOrigin(), credentialUsername(), credentialPassword());
			props.updateRuntimeDraft({ browser: next.runtimeSettings.browser });
			setCredentialOrigin("");
			setCredentialUsername("");
			setCredentialPassword("");
		} catch (error) {
			setCredentialError(error instanceof Error ? error.message : String(error));
		} finally {
			setCredentialBusy("");
		}
	};
	const removeCredential = async (credentialID: string) => {
		setCredentialBusy(credentialID);
		setCredentialError("");
		try {
			const next = await deleteBrowserCredential(credentialID);
			props.updateRuntimeDraft({ browser: next.runtimeSettings.browser });
		} catch (error) {
			setCredentialError(error instanceof Error ? error.message : String(error));
		} finally {
			setCredentialBusy("");
		}
	};
	const clearBrowserData = async () => {
		if (props.runtimeDraft.browser.clearDataPolicy === "ask" && !window.confirm("清除内置浏览器的 Cookie、缓存和登录状态？")) return;
		setClearingBrowserData(true);
		setBrowserDataMessage("");
		try {
			await browserClearData();
			setBrowserDataMessage("浏览数据已清除");
		} catch (error) {
			setBrowserDataMessage(error instanceof Error ? error.message : String(error));
		} finally {
			setClearingBrowserData(false);
		}
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
			  <div class="settings-row-actions">
				<SelectControl
				  value={props.runtimeDraft.browser.clearDataPolicy}
				  options={[
					{ value: "ask", label: "清理前询问" },
					{ value: "session", label: "关闭会话时清理" },
					{ value: "all", label: "保存设置时清理" },
					{ value: "never", label: "不自动清理" },
				  ]}
				  onChange={(value) => updateBrowser({ clearDataPolicy: value })}
				/>
				<button class="settings-soft-button" type="button" disabled={clearingBrowserData()} onClick={() => void clearBrowserData()}>
				  <Show when={!clearingBrowserData()} fallback={<RefreshCw class="spinning" size={14} />}><Trash2 size={14} /></Show>立即清除
				</button>
				<Show when={browserDataMessage()}><span class="settings-muted-value">{browserDataMessage()}</span></Show>
			  </div>
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
          <Show when={props.runtimeDraft.browser.passwordManagerEnabled}>
			<div class="browser-credential-manager">
				<For each={props.runtimeDraft.browser.credentials}>
					{(credential) => (
						<div class="browser-credential-row">
							<KeyRound size={15} />
							<div><strong>{credential.origin}</strong><span>{credential.username}</span></div>
							<button type="button" title="删除网站凭据" aria-label={`删除 ${credential.origin} 的凭据`} disabled={Boolean(credentialBusy())} onClick={() => void removeCredential(credential.id)}>
								<Show when={credentialBusy() !== credential.id} fallback={<RefreshCw class="spinning" size={14} />}><Trash2 size={14} /></Show>
							</button>
						</div>
					)}
				</For>
				<form class="browser-credential-form" onSubmit={(event) => { event.preventDefault(); void saveCredential(); }}>
					<label><span>网站来源</span><input value={credentialOrigin()} placeholder="https://example.com" spellcheck={false} onInput={(event) => setCredentialOrigin(event.currentTarget.value)} /></label>
					<label><span>用户名</span><input value={credentialUsername()} autocomplete="username" onInput={(event) => setCredentialUsername(event.currentTarget.value)} /></label>
					<label><span>密码</span><input type="password" value={credentialPassword()} autocomplete="new-password" onInput={(event) => setCredentialPassword(event.currentTarget.value)} /></label>
					<button type="submit" disabled={credentialBusy() === "save" || !credentialOrigin().trim() || !credentialUsername().trim() || !credentialPassword()}>
						<Show when={credentialBusy() !== "save"} fallback={<RefreshCw class="spinning" size={14} />}><Save size={14} /></Show>保存凭据
					</button>
				</form>
				<Show when={credentialError()}><p class="browser-credential-error">{credentialError()}</p></Show>
			</div>
		  </Show>
          <SettingsRow
            title="联系信息"
            description="允许保存地址、电话号码和电子邮件地址"
            control={<SwitchControl checked={props.runtimeDraft.browser.autofillContactEnabled} onChange={(value) => updateBrowser({ autofillContactEnabled: value })} />}
          />
          <Show when={props.runtimeDraft.browser.autofillContactEnabled}>
            <div class="browser-autofill-form">
              <label><span>姓名</span><input value={props.runtimeDraft.browser.autofillProfile.fullName} onInput={(event) => updateAutofill({ fullName: event.currentTarget.value })} /></label>
              <label><span>电子邮件</span><input type="email" value={props.runtimeDraft.browser.autofillProfile.email} onInput={(event) => updateAutofill({ email: event.currentTarget.value })} /></label>
              <label><span>电话</span><input value={props.runtimeDraft.browser.autofillProfile.phone} onInput={(event) => updateAutofill({ phone: event.currentTarget.value })} /></label>
              <label><span>组织</span><input value={props.runtimeDraft.browser.autofillProfile.organization} onInput={(event) => updateAutofill({ organization: event.currentTarget.value })} /></label>
              <label class="wide"><span>街道地址</span><input value={props.runtimeDraft.browser.autofillProfile.streetAddress} onInput={(event) => updateAutofill({ streetAddress: event.currentTarget.value })} /></label>
              <label><span>城市</span><input value={props.runtimeDraft.browser.autofillProfile.city} onInput={(event) => updateAutofill({ city: event.currentTarget.value })} /></label>
              <label><span>省/州</span><input value={props.runtimeDraft.browser.autofillProfile.region} onInput={(event) => updateAutofill({ region: event.currentTarget.value })} /></label>
              <label><span>邮政编码</span><input value={props.runtimeDraft.browser.autofillProfile.postalCode} onInput={(event) => updateAutofill({ postalCode: event.currentTarget.value })} /></label>
              <label><span>国家或地区</span><input value={props.runtimeDraft.browser.autofillProfile.country} onInput={(event) => updateAutofill({ country: event.currentTarget.value })} /></label>
            </div>
          </Show>
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

export function ComputerControlSettingsPanel(props: {
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
            description="允许 MHcode 列出、截图和操控其他 Windows 应用窗口"
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
            description="允许通过 Windows 窗口通道操控 Google Chrome"
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

export function CommandSettingsPanel(props: {
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
            ["内存上限", `${props.runtimeDraft.maxCommandMemoryMb} MB`],
            ["进程上限", `${props.runtimeDraft.maxCommandProcesses}`],
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

export function MemorySettingsPanel(props: {
  memory: WorkbenchState["projectMemory"];
  runtimeDraft: RuntimeSettings;
  runtimeDirty: boolean;
  saveRuntime: () => void;
  savingRuntime: boolean;
  updateRuntimeDraft: (patch: Partial<RuntimeSettings>) => void;
  resetRuntimeDraft: () => void;
}) {
  const updateMemory = (patch: Partial<RuntimeSettings["memory"]>) => {
    props.updateRuntimeDraft({ memory: { ...props.runtimeDraft.memory, ...patch } });
  };

  return (
    <div class="settings-page-body">
      <SettingsSection title="项目记忆">
        <SettingsCard>
          <SettingsRow
            title="跨会话记忆"
            description="项目级"
            control={<SwitchControl checked={props.runtimeDraft.memory.enabled} onChange={(value) => updateMemory({ enabled: value })} />}
          />
          <SettingsRow
            title="历史会话"
            description={`${props.runtimeDraft.memory.maxSessions} 个`}
            control={
              <input
                class="settings-input numeric"
                type="number"
                min="1"
                max="100"
                value={props.runtimeDraft.memory.maxSessions}
                onInput={(event) => updateMemory({ maxSessions: Number(event.currentTarget.value) })}
              />
            }
          />
          <SettingsRow
            title="字符预算"
            description={`${formatInteger(props.runtimeDraft.memory.maxCharacters)} 字符`}
            control={
              <input
                class="settings-input numeric"
                type="number"
                min="1000"
                max="20000"
                step="500"
                value={props.runtimeDraft.memory.maxCharacters}
                onInput={(event) => updateMemory({ maxCharacters: Number(event.currentTarget.value) })}
              />
            }
          />
          <SettingsRow
            title="包含归档会话"
            description="归档"
            control={<SwitchControl checked={props.runtimeDraft.memory.includeArchived} onChange={(value) => updateMemory({ includeArchived: value })} />}
          />
        </SettingsCard>
        <RuntimeSaveActions
          dirty={props.runtimeDirty}
          reset={props.resetRuntimeDraft}
          save={props.saveRuntime}
          saving={props.savingRuntime}
        />
      </SettingsSection>

      <SettingsSection title="当前快照">
        <MetricGrid
          items={[
            ["项目", props.memory.projectName || "当前工作区"],
            ["会话", formatInteger(props.memory.sessionCount)],
            ["历史轮次", formatInteger(props.memory.turnCount)],
            ["快照", props.memory.snapshotHash || "未生成"],
          ]}
        />
        <pre class="memory-preview">{props.memory.summary || "暂无跨会话记忆"}</pre>
      </SettingsSection>
    </div>
  );
}

export function ProfileSettingsPanel(props: {
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
            ["账户", "本地用户"],
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

export function ShortcutSettingsPanel() {
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

export function ArchiveSettingsPanel() {
  return (
    <div class="settings-page-body">
      <PanelSection icon={<Archive size={16} />} title="已归档对话">
        <p class="empty-line">暂无已归档对话</p>
      </PanelSection>
    </div>
  );
}

export function RuntimeSaveActions(props: {
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

export function ModelRouteMenu(props: {
  onManage: () => void;
  onSelect: (providerID: string, modelID: string) => void;
  saving: boolean;
  settings: RuntimeSettings;
}) {
  const [open, setOpen] = createSignal(false);
  const [activeProviderID, setActiveProviderID] = createSignal("");
  const [compactMenu, setCompactMenu] = createSignal(false);
  const [menuPopoverLeft, setMenuPopoverLeft] = createSignal(0);
  const [compactMenuWidth, setCompactMenuWidth] = createSignal(430);
  const [submenuSide, setSubmenuSide] = createSignal<"left" | "right">("right");
  const [submenuWidth, setSubmenuWidth] = createSignal(230);
  let menuRef: HTMLDivElement | undefined;
  let triggerRef: HTMLButtonElement | undefined;

  const selectedProvider = createMemo(() => selectedModelProvider(props.settings));
  const selectedModel = createMemo(() => selectedModelName(props.settings));
  const providers = createMemo(() => {
    const withModels = props.settings.model.providers.filter((provider) => modelOptionsForProvider(provider).length > 0);
    const enabled = withModels.filter((provider) => provider.enabled);
    return enabled.length > 0 ? enabled : withModels;
  });
  const activeProvider = createMemo(() => {
    const available = providers();
    return (
      available.find((provider) => provider.id === activeProviderID()) ??
      available.find((provider) => provider.id === selectedProvider()?.id) ??
      available[0]
    );
  });
  const selectedModelSetting = createMemo(() => {
    const provider = selectedProvider();
    return provider ? modelOptionsForProvider(provider).find((model) => model.id === selectedModel()) : undefined;
  });
  const currentLabel = createMemo(() => {
    const provider = selectedProvider();
    if (!provider) {
      return "选择模型";
    }
    const model = selectedModelSetting();
    return model?.displayName || shortModelName(selectedModel()) || provider.name;
  });
  const updateMenuLayout = () => {
    if (!menuRef) {
      return;
    }
    const pane = menuRef.closest(".chat-pane") as HTMLElement | null;
    const paneRect = pane?.getBoundingClientRect() ?? {
      left: 0,
      right: window.innerWidth,
      width: window.innerWidth,
    };
    const margin = 14;
    const gap = 6;
    const popoverWidth = 192;
    const preferredSubmenuWidth = 230;
    const minimumSubmenuWidth = 170;
    const menuRect = menuRef.getBoundingClientRect();
    const minPopoverLeft = paneRect.left + margin;
    const maxPopoverLeft = Math.max(minPopoverLeft, paneRect.right - margin - popoverWidth);
    const popoverLeft = Math.min(Math.max(menuRect.left, minPopoverLeft), maxPopoverLeft);
    const leftSpace = Math.max(0, popoverLeft - gap - minPopoverLeft);
    const rightSpace = Math.max(
      0,
      paneRect.right - margin - (popoverLeft + popoverWidth + gap),
    );

    let side: "left" | "right";
    if (rightSpace >= preferredSubmenuWidth) {
      side = "right";
    } else if (leftSpace >= preferredSubmenuWidth) {
      side = "left";
    } else {
      side = rightSpace >= leftSpace ? "right" : "left";
    }
    const availableSubmenuWidth = side === "right" ? rightSpace : leftSpace;
    if (availableSubmenuWidth >= minimumSubmenuWidth) {
      setCompactMenu(false);
      setMenuPopoverLeft(Math.round(popoverLeft - menuRect.left));
      setSubmenuSide(side);
      setSubmenuWidth(Math.floor(Math.min(preferredSubmenuWidth, availableSubmenuWidth)));
      return;
    }

    setCompactMenu(true);
    setSubmenuSide("right");
    const width = Math.min(430, Math.max(180, paneRect.width - margin * 2));
    const minLeft = paneRect.left + margin;
    const maxLeft = Math.max(minLeft, paneRect.right - margin - width);
    const viewportLeft = Math.min(Math.max(menuRect.left, minLeft), maxLeft);
    setCompactMenuWidth(Math.floor(width));
    setMenuPopoverLeft(Math.round(viewportLeft - menuRect.left));
  };

  const close = () => {
    setOpen(false);
  };
  const toggle = () => {
    const next = !open();
    setOpen(next);
    if (next) {
      const available = providers();
      const initial = available.find((provider) => provider.id === selectedProvider()?.id) ?? available[0];
      setActiveProviderID(initial?.id ?? "");
      queueMicrotask(updateMenuLayout);
    }
  };
  const selectModel = (providerID: string, modelID: string) => {
    close();
    props.onSelect(providerID, modelID);
  };
  const focusProvider = (current: HTMLButtonElement, offset: number) => {
    const options = Array.from(menuRef?.querySelectorAll<HTMLButtonElement>(".model-provider-option") ?? []);
    if (options.length === 0) {
      return;
    }
    const currentIndex = options.indexOf(current);
    const nextIndex = (Math.max(currentIndex, 0) + offset + options.length) % options.length;
    const next = options[nextIndex];
    setActiveProviderID(next.dataset.providerId ?? "");
    next.focus();
  };
  const focusModel = (current: HTMLButtonElement, offset: number) => {
    const options = Array.from(
      current.closest(".model-route-submenu")?.querySelectorAll<HTMLButtonElement>(".model-option") ?? [],
    );
    if (options.length === 0) {
      return;
    }
    const currentIndex = options.indexOf(current);
    options[(Math.max(currentIndex, 0) + offset + options.length) % options.length].focus();
  };
  const focusActiveProvider = () => {
    const providerID = activeProvider()?.id;
    const option = Array.from(menuRef?.querySelectorAll<HTMLButtonElement>(".model-provider-option") ?? [])
      .find((item) => item.dataset.providerId === providerID);
    option?.focus();
  };

  onMount(() => {
    const pane = menuRef?.closest(".chat-pane");
    const resizeObserver = new ResizeObserver(updateMenuLayout);
    if (pane) {
      resizeObserver.observe(pane);
    }
    updateMenuLayout();
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
    window.addEventListener("resize", updateMenuLayout);
    onCleanup(() => {
      resizeObserver.disconnect();
      window.removeEventListener("pointerdown", handlePointerDown);
      window.removeEventListener("resize", updateMenuLayout);
    });
  });

  return (
    <div
      class="model-route-menu"
      classList={{ compact: compactMenu(), "submenu-left": submenuSide() === "left" }}
      ref={menuRef}
      style={`--model-menu-popover-left: ${menuPopoverLeft()}px; --model-menu-compact-width: ${compactMenuWidth()}px; --model-menu-submenu-width: ${submenuWidth()}px;`}
    >
      <button
        ref={triggerRef}
        class="model-route-trigger"
        type="button"
        aria-haspopup="menu"
        aria-expanded={open()}
        title="选择模型"
        disabled={props.saving}
        onClick={toggle}
      >
        <Cpu size={15} />
        <span class="model-route-current">{currentLabel()}</span>
        <ChevronDown size={14} aria-hidden="true" />
      </button>
      <Show when={open()}>
        <div class="model-route-popover" aria-label="选择模型">
          <div class="model-provider-column" role="menu" aria-label="模型供应商">
            <For each={providers()} fallback={<p class="model-list-empty">还没有可用的模型。</p>}>
              {(provider) => {
                const active = () => activeProvider()?.id === provider.id;
                const selected = () => selectedProvider()?.id === provider.id;
                return (
                  <button
                    class="model-provider-option"
                    classList={{ active: active() }}
                    type="button"
                    role="menuitem"
                    aria-haspopup="menu"
                    aria-expanded={active()}
                    data-provider-id={provider.id}
                    onFocus={() => setActiveProviderID(provider.id)}
                    onPointerEnter={() => setActiveProviderID(provider.id)}
                    onClick={() => setActiveProviderID(provider.id)}
                    onKeyDown={(event) => {
                      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                        event.preventDefault();
                        focusProvider(event.currentTarget, event.key === "ArrowDown" ? 1 : -1);
                      } else if (
                        event.key === (compactMenu() || submenuSide() === "right" ? "ArrowRight" : "ArrowLeft")
                      ) {
                        event.preventDefault();
                        menuRef?.querySelector<HTMLButtonElement>(".model-route-submenu .model-option")?.focus();
                      } else if (event.key === "Escape") {
                        close();
                        triggerRef?.focus();
                      }
                    }}
                  >
                    <span class="model-provider-name">{provider.name}</span>
                    <span class="model-provider-indicators">
                      <Show when={selected()}><Check size={14} aria-label="当前供应商" /></Show>
                      <Show
                        when={!compactMenu() && submenuSide() === "left"}
                        fallback={<ChevronRight size={14} aria-hidden="true" />}
                      >
                        <ChevronLeft size={14} aria-hidden="true" />
                      </Show>
                    </span>
                  </button>
                );
              }}
            </For>
          </div>
          <Show when={activeProvider()} keyed>
            {(provider) => (
              <div class="model-route-submenu">
                <div class="model-list-scroll" role="menu" aria-label={`${provider.name} 模型`}>
                  <For each={modelOptionsForProvider(provider)} fallback={<p class="model-list-empty">没有可用模型。</p>}>
                    {(model) => {
                      const selected = () => selectedProvider()?.id === provider.id && selectedModel() === model.id;
                      return (
                        <button
                          class="model-option"
                          classList={{ selected: selected() }}
                          type="button"
                          role="menuitemradio"
                          aria-checked={selected()}
                          title={model.displayName || model.id}
                          onClick={() => selectModel(provider.id, model.id)}
                          onKeyDown={(event) => {
                            if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                              event.preventDefault();
                              focusModel(event.currentTarget, event.key === "ArrowDown" ? 1 : -1);
                            } else if (
                              event.key === (compactMenu() || submenuSide() === "right" ? "ArrowLeft" : "ArrowRight")
                            ) {
                              event.preventDefault();
                              focusActiveProvider();
                            } else if (event.key === "Escape") {
                              close();
                              triggerRef?.focus();
                            }
                          }}
                        >
                          <span>{model.displayName || model.id}</span>
                          <Show when={selected()}><Check size={14} aria-label="已选中" /></Show>
                        </button>
                      );
                    }}
                  </For>
                </div>
              </div>
            )}
          </Show>
          <button
            class="model-manage-button"
            type="button"
            onClick={() => {
              close();
              props.onManage();
            }}
          >
            <SlidersHorizontal size={14} />
            管理模型
          </button>
        </div>
      </Show>
    </div>
  );
}

export function SettingsSection(props: { action?: JSX.Element; children: JSX.Element; class?: string; title?: string }) {
  return (
    <section class={`settings-form-section ${props.class ?? ""}`}>
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

export function SettingsCard(props: { children: JSX.Element }) {
  return <div class="settings-card">{props.children}</div>;
}

export function SettingsRow(props: {
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

export function SwitchControl(props: { checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label class="settings-switch">
      <input type="checkbox" checked={props.checked} onChange={(event) => props.onChange(event.currentTarget.checked)} />
      <span aria-hidden="true">
        <span />
      </span>
    </label>
  );
}

export function CachePanel(props: {
  cacheHealth: WorkbenchState["cacheHealth"];
  cacheHitRate: number;
  cacheTarget: number;
  diagnostics: string[];
  hasCacheTokens: boolean;
  session: DeepSeekSessionState;
  sessionHasCacheTokens: boolean;
  usage: UsageMetrics;
  usageLedger?: WorkbenchState["usageLedger"];
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
      <PanelSection icon={<Database size={16} />} title="用量账本">
        <MetricGrid
          items={[
            ["本会话请求", formatInteger(props.usageLedger?.sessionSamples ?? 0)],
            ["累计请求", formatInteger(props.usageLedger?.totalSamples ?? 0)],
            ["本会话 Token", formatInteger((props.usageLedger?.sessionInputTokens ?? 0) + (props.usageLedger?.sessionOutputTokens ?? 0))],
            ["累计 Token", formatInteger((props.usageLedger?.totalInputTokens ?? 0) + (props.usageLedger?.totalOutputTokens ?? 0))],
            ["本会话成本", formatCost(props.usageLedger?.sessionEffectiveCost ?? 0)],
            ["累计成本", formatCost(props.usageLedger?.totalEffectiveCost ?? 0)],
          ]}
        />
        <Show when={props.usageLedger?.lastError}>
          <p class="empty-line">用量账本暂不可用：{props.usageLedger?.lastError}</p>
        </Show>
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

export function ContextPanel(props: { contextPreview: WorkbenchState["contextPreview"] | undefined }) {
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

export function PanelSection(props: { icon: JSX.Element; title: string; children: JSX.Element }) {
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

export function MetricGrid(props: { items: Array<[string, string]> }) {
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

export function ContextList(props: { sections: Array<{ name: string; content: string }> }) {
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

export function RouteStep(props: { icon: JSX.Element; title: string; detail: string }) {
  return (
    <div class="route-step">
      {props.icon}
      <strong>{props.title}</strong>
      <span>{props.detail}</span>
    </div>
  );
}

export function IconButton(props: {
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

export function StatusPill(props: { icon: JSX.Element; label: string; tone: "good" | "watch" | "bad" | "neutral" }) {
  return (
    <span class={`status-pill ${props.tone}`}>
      {props.icon}
      {props.label}
    </span>
  );
}

export function SettingField(props: { label: string; value: string; children: JSX.Element }) {
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

export function SegmentedControl(props: {
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

export function SelectControl(props: {
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

export function ToggleRow(props: {
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
