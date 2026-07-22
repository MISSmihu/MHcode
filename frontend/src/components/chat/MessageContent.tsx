import { For, Match, Switch, Show, createMemo, createSignal } from "solid-js";
import {
  AlertCircle,
  Check,
  ChevronDown,
  ChevronRight,
  Circle,
  Eye,
  ExternalLink,
  FileCode2,
  FolderOpen,
  GitBranch,
  Globe2,
  Image as ImageIcon,
  LoaderCircle,
  Monitor,
  Pencil,
  Search,
  TerminalSquare,
  Undo2,
  Users,
  Wrench,
} from "lucide-solid";
import type { MessagePart, WorkspaceFileView } from "../../types";
import { renderMarkdown, handleCodeCopyClick } from "../../lib/markdown";
import { parseWorkspaceFileRangeCandidate, parseWorkspacePathCandidate } from "../../lib/workspace-path";
import { inlineDiffStats } from "../../lib/inline-diff";
import { openWorkspaceFile, revealWorkspaceFile } from "../../services/workbench";
import { formatElapsedDuration } from "../../lib/duration";
import { InlineCodePreview } from "./InlineCodePreview";
import { InlineDiffPreview } from "./InlineDiffPreview";

// 操作流渲染：按片段类型分派。工具/diff 卡片默认折叠，用户按需展开（对标 ZCode/Codex）。
export function MessageContent(props: {
  parts: MessagePart[];
  inferFileArtifacts?: boolean;
  hideTeamRun?: boolean;
  hideFileChangesSummary?: boolean;
  undoingChanges?: boolean;
  onUndoChanges?: () => void | Promise<void>;
  onReviewChanges?: () => void | Promise<void>;
  onPreviewFile?: (path: string) => void | Promise<void>;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
  onOpenURL?: (url: string) => void | Promise<void>;
}) {
  const renderedParts = createMemo(() => withInferredFileArtifacts(props.parts, props.inferFileArtifacts !== false));
  const blocks = createMemo(() => groupRenderBlocks(renderedParts()));
  const fileChanges = createMemo(() => editedFileSummaries(renderedParts()));
  return (
    <div class="op-stream">
      <For each={blocks()}>
        {(block) => (
          <Switch>
            <Match when={block.kind === "text"}>
              <MarkdownBlock
                source={(block as TextRenderBlock).part.text}
                onOpenURL={props.onOpenURL}
                onOpenWorkspaceFile={props.onOpenWorkspaceFile}
              />
            </Match>
            <Match when={block.kind === "activity"}>
              <ActivityGroup
                parts={(block as ActivityRenderBlock).parts}
                onPreviewFile={props.onPreviewFile}
                onOpenWorkspaceFile={props.onOpenWorkspaceFile}
                onOpenURL={props.onOpenURL}
              />
            </Match>
            <Match when={block.kind === "team"}>
              <Show when={!props.hideTeamRun}>
                <TeamRun parts={(block as TeamRenderBlock).parts} />
              </Show>
            </Match>
          </Switch>
        )}
      </For>
      <Show when={!props.hideFileChangesSummary && fileChanges().length > 0}>
        <FileChangesSummary
          files={fileChanges()}
          undoing={props.undoingChanges}
          onUndo={props.onUndoChanges}
          onReview={props.onReviewChanges}
          onOpenFile={props.onOpenWorkspaceFile}
        />
      </Show>
    </div>
  );
}

type TextPart = Extract<MessagePart, { kind: "text" }>;
type ToolPart = Extract<MessagePart, { kind: "tool_call" }>;
type DiffPart = Extract<MessagePart, { kind: "diff" }>;
type FilePart = Extract<MessagePart, { kind: "file" }>;
export type TaskProgressPart = Extract<MessagePart, { kind: "task_progress" }>;
type WebSearchPart = Extract<MessagePart, { kind: "web_search_results" }>;
export type TeamPart = Extract<MessagePart, { kind: "team_role" }>;
type TextRenderBlock = { kind: "text"; part: TextPart };
type ActivityRenderBlock = { kind: "activity"; parts: MessagePart[] };
type TeamRenderBlock = { kind: "team"; parts: TeamPart[] };
type RenderBlock = TextRenderBlock | ActivityRenderBlock | TeamRenderBlock;
type ActivityCategory = "command" | "edit" | "read" | "directory" | "search" | "web" | "repository" | "image" | "browser" | "computer" | "open" | "file" | "tool";
type ActivityItem = { category: ActivityCategory; parts: MessagePart[] };

function ActivityGroup(props: {
  parts: MessagePart[];
  onPreviewFile?: (path: string) => void | Promise<void>;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
  onOpenURL?: (url: string) => void | Promise<void>;
}) {
  const items = createMemo(() => buildActivityItems(props.parts));
  const artifacts = createMemo(() => activityArtifacts(props.parts));
  return (
    <>
      <div class="op-activity-feed">
        <For each={items()}>
          {(item) => (
            <ActivityRow
              item={item}
              onOpenURL={props.onOpenURL}
              onOpenWorkspaceFile={props.onOpenWorkspaceFile}
            />
          )}
        </For>
      </div>
      <For each={artifacts()}>
        {(part) => (
          <FileCard
            part={part}
            onPreviewFile={props.onPreviewFile}
            onOpenWorkspaceFile={props.onOpenWorkspaceFile}
          />
        )}
      </For>
    </>
  );
}

function ActivityRow(props: {
  item: ActivityItem;
  onOpenURL?: (url: string) => void | Promise<void>;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
}) {
  const status = () => activityStatus(props.item);
  return (
    <details
      class="op-activity-item"
      classList={{ [status()]: true }}
    >
      <summary title={activityTitle(props.item)}>
        <span class="op-activity-icon"><ActivityIcon category={props.item.category} /></span>
        <span class="op-activity-label">{activityLabel(props.item)}</span>
        <Show when={status() === "running"}>
          <span class="op-activity-spinner" aria-label="执行中" />
        </Show>
        <Show when={status() === "error"}>
          <AlertCircle class="op-activity-error" size={14} aria-label="执行失败" />
        </Show>
        <ChevronRight class="op-activity-chevron" size={14} aria-hidden="true" />
      </summary>
      <div class="op-activity-body">
        <For each={props.item.parts}>
          {(part) => (
            <Switch>
              <Match when={part.kind === "tool_call"}>
                <ToolDetail
                  part={part as ToolPart}
                  hideOutput={props.item.parts.some((item) => item.kind === "web_search_results")}
                  onOpenWorkspaceFile={props.onOpenWorkspaceFile}
                />
              </Match>
              <Match when={part.kind === "diff"}>
                <DiffDetail part={part as DiffPart} onOpenWorkspaceFile={props.onOpenWorkspaceFile} />
              </Match>
              <Match when={part.kind === "web_search_results"}>
                <WebSearchResults part={part as WebSearchPart} onOpenURL={props.onOpenURL} />
              </Match>
            </Switch>
          )}
        </For>
      </div>
    </details>
  );
}

function ActivityIcon(props: { category: ActivityCategory }) {
  return (
    <Switch fallback={<Wrench size={14} />}>
      <Match when={props.category === "command"}><TerminalSquare size={14} /></Match>
      <Match when={props.category === "edit"}><Pencil size={14} /></Match>
      <Match when={props.category === "read" || props.category === "open"}><Eye size={14} /></Match>
      <Match when={props.category === "directory" || props.category === "file"}><FolderOpen size={14} /></Match>
      <Match when={props.category === "search" || props.category === "web"}><Search size={14} /></Match>
      <Match when={props.category === "repository"}><GitBranch size={14} /></Match>
      <Match when={props.category === "image"}><ImageIcon size={14} /></Match>
      <Match when={props.category === "browser"}><Globe2 size={14} /></Match>
      <Match when={props.category === "computer"}><Monitor size={14} /></Match>
    </Switch>
  );
}

function ToolDetail(props: {
  part: ToolPart;
  hideOutput?: boolean;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
}) {
  if (props.part.name === "run_command") {
    return <ShellToolDetail part={props.part} />;
  }
  const readReference = createMemo(() => props.part.name === "read_file"
    ? parseWorkspaceFileRangeCandidate(props.part.input ?? "")
    : undefined);
  const hasCodePreview = () => props.part.status !== "error" && Boolean(props.part.output) && !props.hideOutput;
  return (
    <div class="op-activity-detail">
      <div class="op-activity-detail-head">
        <code>{props.part.name}</code>
        <span>{toolStatusLabel(props.part.status ?? "ok")}{props.part.durationMs !== undefined ? ` · ${formatElapsedDuration(props.part.durationMs)}` : ""}</span>
      </div>
      <Show when={readReference()} fallback={
        <>
          <Show when={props.part.input}><pre>{props.part.input}</pre></Show>
          <Show when={props.part.output && !props.hideOutput}><pre>{props.part.output}</pre></Show>
        </>
      }>
        <Show when={!hasCodePreview()}>
          <button
            type="button"
            class="op-read-file"
            disabled={!props.onOpenWorkspaceFile}
            title={`在右侧查看 ${readReference()?.path}`}
            onClick={() => {
              const reference = readReference();
              if (reference) void props.onOpenWorkspaceFile?.(reference.path, "file", reference.startLine);
            }}
          >
            <FileCode2 size={15} aria-hidden="true" />
            <span class="op-read-file-main">
              <strong>{baseName(readReference()?.path ?? "")}</strong>
              <small>{readReference()?.path}</small>
            </span>
            <Show when={readRangeLabel(readReference())}>
              <span class="op-read-file-range">{readRangeLabel(readReference())}</span>
            </Show>
            <ChevronRight size={13} aria-hidden="true" />
          </button>
        </Show>
        <Show when={hasCodePreview()}>
          <InlineCodePreview
            path={readReference()?.path ?? ""}
            content={props.part.output ?? ""}
            startLine={readReference()?.startLine}
            onOpen={props.onOpenWorkspaceFile ? () => {
              const reference = readReference();
              if (reference) void props.onOpenWorkspaceFile?.(reference.path, "file", reference.startLine);
            } : undefined}
          />
        </Show>
        <Show when={props.part.status === "error" && props.part.output && !props.hideOutput}>
          <pre>{props.part.output}</pre>
        </Show>
      </Show>
    </div>
  );
}

function ShellToolDetail(props: { part: ToolPart }) {
  const duration = () => props.part.durationMs !== undefined ? formatElapsedDuration(props.part.durationMs) : "";
  return (
    <div class="op-activity-detail op-shell-detail">
      <div class="op-activity-detail-head">
        <span class="op-shell-title"><TerminalSquare size={13} /><code>Shell</code></span>
        <span class="op-shell-meta">
          <Show when={props.part.exitCode !== undefined}>
            <em classList={{ error: props.part.exitCode !== 0 }}>exit {props.part.exitCode}</em>
          </Show>
          <Show when={duration()}><em>{duration()}</em></Show>
        </span>
      </div>
      <div class="op-shell-command"><span aria-hidden="true">$</span><code>{props.part.input || "(empty command)"}</code></div>
      <Show when={props.part.workingDirectory}>
        <div class="op-shell-workdir" title={props.part.workingDirectory}>cwd <code>{props.part.workingDirectory}</code></div>
      </Show>
      <pre class="op-shell-output">{props.part.output || "(no output)"}</pre>
    </div>
  );
}

function DiffDetail(props: {
  part: DiffPart;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
}) {
  return (
    <InlineDiffPreview
      path={props.part.path}
      patch={props.part.patch}
      additions={props.part.additions}
      deletions={props.part.deletions}
      onOpen={props.onOpenWorkspaceFile
        ? () => void props.onOpenWorkspaceFile?.(props.part.path, "changes")
        : undefined}
    />
  );
}

type EditedFileSummary = {
  path: string;
  additions: number;
  deletions: number;
};

function FileChangesSummary(props: {
  files: EditedFileSummary[];
  undoing?: boolean;
  onUndo?: () => void | Promise<void>;
  onReview?: () => void | Promise<void>;
  onOpenFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
}) {
  const [expanded, setExpanded] = createSignal(false);
  const visibleFiles = createMemo(() => expanded() ? props.files : props.files.slice(0, 3));
  const hiddenCount = createMemo(() => Math.max(0, props.files.length - 3));
  const additions = createMemo(() => props.files.reduce((total, file) => total + file.additions, 0));
  const deletions = createMemo(() => props.files.reduce((total, file) => total + file.deletions, 0));
  return (
    <section class="op-file-changes" aria-label={`已编辑 ${props.files.length} 个文件`}>
      <header class="op-file-changes-head">
        <span class="op-file-changes-icon" aria-hidden="true"><FileCode2 size={15} /></span>
        <span class="op-file-changes-title">
          <strong>已编辑 {props.files.length} 个文件</strong>
          <small class="op-diff-stat">
            <Show when={additions() > 0}><em class="add">+{additions()}</em></Show>
            <Show when={deletions() > 0}><em class="del">-{deletions()}</em></Show>
          </small>
        </span>
        <span class="op-file-changes-actions">
          <Show when={props.onUndo}>
            <button type="button" disabled={props.undoing} title="撤销本轮文件修改" onClick={() => void props.onUndo?.()}>
              <Undo2 size={13} /><span>{props.undoing ? "撤销中" : "撤销"}</span>
            </button>
          </Show>
          <Show when={props.onReview}>
            <button type="button" title="在右侧审阅修改" onClick={() => void props.onReview?.()}>
              <Eye size={13} /><span>审阅</span>
            </button>
          </Show>
        </span>
      </header>
      <div class="op-edited-files">
        <For each={visibleFiles()}>
          {(file) => (
            <button
              type="button"
              class="op-edited-file"
              disabled={!props.onOpenFile}
              title={`在右侧查看 ${file.path}`}
              onClick={() => void props.onOpenFile?.(file.path, "changes")}
            >
              <span>{file.path}</span>
              <span class="op-diff-stat">
                <Show when={file.additions > 0}><em class="add">+{file.additions}</em></Show>
                <Show when={file.deletions > 0}><em class="del">-{file.deletions}</em></Show>
              </span>
              <ChevronRight size={13} aria-hidden="true" />
            </button>
          )}
        </For>
      </div>
      <Show when={hiddenCount() > 0}>
        <button
          class="op-file-changes-more"
          type="button"
          aria-expanded={expanded()}
          onClick={() => setExpanded((value) => !value)}
        >
          <span>{expanded() ? "收起文件" : `再显示 ${hiddenCount()} 个文件`}</span>
          <ChevronDown size={13} aria-hidden="true" />
        </button>
      </Show>
    </section>
  );
}

function editedFileSummaries(parts: MessagePart[]): EditedFileSummary[] {
  const files = new Map<string, EditedFileSummary>();
  for (const part of parts) {
    if (part.kind !== "diff" && (part.kind !== "file" || !isChangedFilePart(part))) continue;
    const current = files.get(part.path) ?? { path: part.path, additions: 0, deletions: 0 };
    if (part.kind === "diff") {
      const calculated = inlineDiffStats(part.patch);
      current.additions += part.additions ?? calculated.additions;
      current.deletions += part.deletions ?? calculated.deletions;
    }
    files.set(part.path, current);
  }
  return [...files.values()];
}

function isChangedFilePart(part: FilePart): boolean {
  return part.created === true || part.fileAction === "created" || part.fileAction === "modified";
}

function groupRenderBlocks(parts: MessagePart[]): RenderBlock[] {
  const blocks: RenderBlock[] = [];
  const teamParts = parts.filter((part): part is TeamPart => part.kind === "team_role");
  let teamAdded = false;
  for (const part of parts) {
    if (part.kind === "text") {
      blocks.push({ kind: "text", part });
      continue;
    }
    if (part.kind === "task_progress") {
      // 任务清单固定显示在输入框上方，避免把执行状态混进助手正文。
      continue;
    }
    if (part.kind === "team_role") {
      if (!teamAdded) {
        blocks.push({ kind: "team", parts: teamParts });
        teamAdded = true;
      }
      continue;
    }
    const previous = blocks.at(-1);
    if (previous?.kind === "activity") {
      previous.parts.push(part);
    } else {
      blocks.push({ kind: "activity", parts: [part] });
    }
  }
  return blocks;
}

function buildActivityItems(parts: MessagePart[]): ActivityItem[] {
  const raw: ActivityItem[] = [];
  for (let index = 0; index < parts.length; index++) {
    const part = parts[index];
    if (part.kind === "text") {
      continue;
    }
    const itemParts: MessagePart[] = [part];
    let category = activityCategory(part);
    const next = parts[index + 1];
    if (next?.kind === "web_search_results" && part.kind === "tool_call" && part.name === "web_search") {
      itemParts.push(next);
      index++;
    } else if (next?.kind === "file" && (
      part.kind === "diff" && next.path === part.path ||
      part.kind === "tool_call"
    )) {
      itemParts.push(next);
      index++;
      if (part.kind === "tool_call" && part.name === "browser" && isImagePath(next.path)) {
        category = "image";
      }
    }
    const previous = raw.at(-1);
    if (previous && previous.category === category && aggregateActivity(category)) {
      previous.parts.push(...itemParts);
    } else {
      raw.push({ category, parts: itemParts });
    }
  }
  return raw;
}

function activityCategory(part: Exclude<MessagePart, TextPart>): ActivityCategory {
  if (part.kind === "diff") return "edit";
  if (part.kind === "file") return isImagePath(part.path) ? "image" : "file";
  if (part.kind === "web_search_results") return "web";
  if (part.kind === "task_progress") return "tool";
  if (part.kind === "team_role") return "tool";
  switch (part.name) {
    case "run_command": return "command";
    case "read_file": return "read";
    case "file_info": return "read";
    case "list_dir": return "directory";
    case "search": return "search";
    case "web_search": return "web";
    case "read_webpage": return "web";
    case "read_repository": return "repository";
    case "browser": return "browser";
    case "computer": return "computer";
    case "open_file": return "open";
    case "write_file":
    case "apply_patch":
    case "copy_file":
    case "delete_file": return "edit";
    default: return "tool";
  }
}

export function TeamRun(props: { parts: TeamPart[]; docked?: boolean }) {
  const completed = createMemo(() => props.parts.filter((part) => part.status === "completed").length);
  const current = createMemo(() =>
    props.parts.find((part) => part.status === "running") ??
    props.parts.find((part) => part.status === "error") ??
    props.parts.find((part) => part.status !== "completed") ??
    props.parts[props.parts.length - 1],
  );
  const progress = createMemo(() => {
    if (props.parts.length === 0) return 0;
    const runningWeight = props.parts.some((part) => part.status === "running") ? 0.45 : 0;
    return Math.min(100, Math.round(((completed() + runningWeight) / props.parts.length) * 100));
  });

  if (props.docked) {
    return (
      <details class="op-team-run docked" aria-label="AI 团队执行状态">
        <summary class="op-team-dock-summary">
          <span class="op-team-mark"><Users size={15} /></span>
          <span class="op-team-dock-copy">
            <strong>AI 团队</strong>
            <small>{teamCurrentStatus(current(), completed(), props.parts.length)}</small>
          </span>
          <span class="op-team-count">{completed()}/{props.parts.length}</span>
          <ChevronDown class="op-team-disclosure" size={14} />
          <span class="op-team-progress-track" aria-hidden="true">
            <span style={{ width: `${progress()}%` }} />
          </span>
          <span class="op-team-stage-track" aria-label="团队阶段">
            <For each={props.parts}>
              {(part) => (
                <span
                  class="op-team-stage"
                  classList={{ [part.status || "pending"]: true }}
                  title={`${part.roleLabel || part.role} · ${part.model || "跟随当前模型"}`}
                >
                  <TeamRoleStatus status={part.status} />
                  <span>{part.roleLabel || part.role}</span>
                </span>
              )}
            </For>
          </span>
        </summary>
        <div class="op-team-dock-details">
          <TeamRoleRows parts={props.parts} />
        </div>
      </details>
    );
  }

  return (
    <section class="op-team-run" aria-label="AI 团队执行记录">
      <div class="op-team-head"><Users size={14} /><strong>AI 团队</strong></div>
      <TeamRoleRows parts={props.parts} />
    </section>
  );
}

function TeamRoleRows(props: { parts: TeamPart[] }) {
  return (
    <div class="op-team-roles">
      <For each={props.parts}>
        {(part) => (
          <div class="op-team-role" classList={{ [part.status || "pending"]: true }}>
            <span class="op-team-role-status" aria-hidden="true"><TeamRoleStatus status={part.status} /></span>
            <span class="op-team-role-main">
              <strong>{part.roleLabel || part.role}</strong>
              <small>{part.model || "跟随当前模型"}{(part.attempt ?? 1) > 1 ? ` · 第 ${part.attempt} 轮` : ""}</small>
            </span>
            <Show when={part.verdict}>
              <span class="op-team-verdict" classList={{ approved: part.verdict === "approved", changes: part.verdict === "changes_required" }}>
                {part.verdict === "approved" ? "通过" : part.verdict === "changes_required" ? "需修改" : "已检查"}
              </span>
            </Show>
            <Show when={part.summary}>
              <details class="op-team-summary">
                <summary>查看结果 <ChevronRight size={12} /></summary>
                <p>{part.summary}</p>
              </details>
            </Show>
          </div>
        )}
      </For>
    </div>
  );
}

function TeamRoleStatus(props: { status?: string }) {
  return (
    <Show when={props.status === "completed"} fallback={
      <Show when={props.status === "error"} fallback={
        <Show when={props.status === "running"} fallback={<Circle size={11} />}>
          <span class="op-team-spinner" />
        </Show>
      }>
        <AlertCircle size={12} />
      </Show>
    }>
      <Check size={12} />
    </Show>
  );
}

function teamCurrentStatus(part: TeamPart | undefined, completed: number, total: number) {
  if (total > 0 && completed === total) return "全部阶段已完成";
  if (!part) return "正在准备任务";
  const label = part.roleLabel || part.role || "团队角色";
  if (part.status === "error") return `${label}需要处理`;
  if (part.status === "running") return `${label}正在工作`;
  return `等待${label}`;
}

function aggregateActivity(category: ActivityCategory): boolean {
  return category === "command" || category === "edit" || category === "read" || category === "image" || category === "search";
}

function activityStatus(item: ActivityItem): "running" | "ok" | "error" {
  const tools = item.parts.filter((part): part is ToolPart => part.kind === "tool_call");
  if (tools.some((part) => part.status === "error")) return "error";
  if (tools.some((part) => part.status === "running")) return "running";
  return "ok";
}

function activityLabel(item: ActivityItem): string {
  const tools = item.parts.filter((part): part is ToolPart => part.kind === "tool_call");
  const files = uniqueStrings(item.parts.flatMap((part) => part.kind === "diff" || part.kind === "file" ? [part.path] : []));
  const input = tools.find((part) => part.input)?.input ?? "";
  switch (item.category) {
    case "command":
      return tools.length > 1 ? `运行了 ${tools.length} 个命令` : "运行了命令";
    case "edit":
      return files.length > 1 ? `编辑了 ${files.length} 个文件` : files[0] ? `编辑了 ${baseName(files[0])}` : "编辑了文件";
    case "read":
      return tools.length > 1 ? `读取了 ${tools.length} 个文件` : input ? `读取了 ${baseName(input)}` : "读取了文件";
    case "directory":
      return input ? `查看了目录 ${input}` : "查看了目录";
    case "search":
      return input ? `搜索了代码“${compactLabel(input)}”` : "搜索了代码";
    case "web":
      if (tools.some((part) => part.name === "read_webpage")) {
        return input ? `读取了网页 ${compactLabel(input)}` : "读取了网页正文";
      }
      return input ? `搜索了网络“${compactLabel(input)}”` : "搜索了网络";
    case "repository":
      return input ? `读取了仓库 ${compactLabel(input)}` : "读取了代码仓库";
    case "image": {
      const count = Math.max(1, files.filter(isImagePath).length);
      return `查看了 ${count} 张图像`;
    }
    case "browser":
      return input.startsWith("http") ? "打开了网页" : "使用了内置浏览器";
    case "computer":
      return input ? `操作了窗口（${compactLabel(input)}）` : "操作了其他窗口";
    case "open":
      return input ? `打开了 ${baseName(input)}` : "打开了文件";
    case "file":
      return files.length > 1 ? `生成了 ${files.length} 个文件` : files[0] ? `生成了 ${baseName(files[0])}` : "生成了文件";
    default:
      return tools.length > 1 ? `运行了 ${tools.length} 个工具` : `运行了 ${friendlyToolName(tools[0]?.name)}`;
  }
}

function activityTitle(item: ActivityItem): string {
  const details = item.parts.flatMap((part) => {
    if (part.kind === "tool_call") return [part.input || part.name];
    if (part.kind === "diff" || part.kind === "file") return [part.path];
    if (part.kind === "web_search_results") return [part.query];
    return [];
  });
  return uniqueStrings(details).join("\n");
}

export function TaskProgress(props: { part: TaskProgressPart }) {
  const completed = () => props.part.steps.filter((step) => step.status === "completed").length;
  const activeIndex = () => props.part.steps.findIndex((step) => step.status === "in_progress");
  const currentStep = () => activeIndex() >= 0 ? activeIndex() + 1 : completed();
  const running = () => props.part.taskStatus === "running" || !props.part.taskStatus;
  const fileStats = () => {
    const files = props.part.changedFiles ?? 0;
    const additions = props.part.additions ?? 0;
    const deletions = props.part.deletions ?? 0;
    if (files === 0 && additions === 0 && deletions === 0) return "";
    return `${files} 个文件已更改`;
  };

  return (
    <section class="op-task-progress" aria-label="任务进度">
      <div class="op-task-steps">
        <For each={props.part.steps}>
          {(step) => (
            <div class="op-task-step" classList={{ [step.status]: true }}>
              <span class="op-task-step-icon" aria-hidden="true">
                <Show
                  when={step.status === "completed"}
                  fallback={
                    <Show when={step.status === "in_progress" && running()} fallback={<Circle size={12} />}>
                      <span class="op-task-step-spinner" />
                    </Show>
                  }
                >
                  <Check size={12} />
                </Show>
              </span>
              <span>{step.title}</span>
            </div>
          )}
        </For>
      </div>
      <div
        class="op-task-footer"
        classList={{ failed: props.part.taskStatus === "failed", cancelled: props.part.taskStatus === "cancelled" }}
      >
        <span class="op-task-footer-state" aria-hidden="true">
          <Show
            when={running()}
            fallback={props.part.taskStatus === "completed" ? <Check size={12} /> : <AlertCircle size={12} />}
          >
            <span class="op-task-footer-spinner" />
          </Show>
        </span>
        <span>第 {currentStep()} / {props.part.steps.length} 步</span>
        <Show when={fileStats()}>
          <span class="op-task-separator">·</span>
          <span>{fileStats()}</span>
        </Show>
        <Show when={(props.part.additions ?? 0) > 0}>
          <strong class="op-task-add">+{props.part.additions}</strong>
        </Show>
        <Show when={(props.part.deletions ?? 0) > 0}>
          <strong class="op-task-del">-{props.part.deletions}</strong>
        </Show>
      </div>
    </section>
  );
}

function WebSearchResults(props: {
  part: WebSearchPart;
  onOpenURL?: (url: string) => void | Promise<void>;
}) {
  const openSource = (url: string) => {
    if (props.onOpenURL) {
      void props.onOpenURL(url);
      return;
    }
    window.open(url, "_blank", "noopener,noreferrer");
  };

  return (
    <div class="op-search-results">
      <For each={props.part.sources}>
        {(source) => (
          <button type="button" class="op-search-source" onClick={() => openSource(source.url)} title={source.url}>
            <span class="op-search-source-main">
              <strong>{source.title}</strong>
              <small>{source.url}</small>
            </span>
            <Show when={source.snippet}>
              <span class="op-search-snippet">{source.snippet}</span>
            </Show>
            <ExternalLink size={13} aria-hidden="true" />
          </button>
        )}
      </For>
    </div>
  );
}

function activityArtifacts(parts: MessagePart[]): FilePart[] {
  const byPath = new Map<string, FilePart>();
  for (const part of parts) {
    if (part.kind !== "file") continue;
    if (part.fileAction !== "created" && part.created !== true) continue;
    byPath.set(part.path, part);
  }
  return [...byPath.values()];
}

function friendlyToolName(name = "工具"): string {
  return name.replaceAll("_", " ");
}

function compactLabel(value: string): string {
  const text = value.trim().replaceAll(/\s+/g, " ");
  return text.length > 30 ? `${text.slice(0, 30)}…` : text;
}

function readRangeLabel(reference: ReturnType<typeof parseWorkspaceFileRangeCandidate>): string {
  if (!reference?.startLine) return "";
  return reference.endLine && reference.endLine !== reference.startLine
    ? `${reference.startLine}-${reference.endLine} 行`
    : `第 ${reference.startLine} 行`;
}

function uniqueStrings(values: string[]): string[] {
  return [...new Set(values.filter(Boolean))];
}

function isImagePath(path: string): boolean {
  return ["png", "jpg", "jpeg", "webp", "gif", "bmp"].includes(extensionOf(path));
}

function FileCard(props: {
  part: Extract<MessagePart, { kind: "file" }>;
  onPreviewFile?: (path: string) => void | Promise<void>;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
}) {
  const [busyAction, setBusyAction] = createSignal<"view" | "preview" | "open" | "reveal" | "">("");
  const [error, setError] = createSignal("");
  const fileName = () => baseName(props.part.path);
  const isHTML = () => extensionOf(props.part.path) === "html" || extensionOf(props.part.path) === "htm";

  const run = async (action: "view" | "preview" | "open" | "reveal") => {
    setBusyAction(action);
    setError("");
    try {
      if (action === "view") {
        if (!props.onOpenWorkspaceFile) {
          throw new Error("当前视图无法打开右侧文件面板。");
        }
        await props.onOpenWorkspaceFile(props.part.path, "file");
      } else if (action === "preview") {
        if (!props.onPreviewFile) {
          throw new Error("当前视图无法打开内置预览。");
        }
        await props.onPreviewFile(props.part.path);
      } else if (action === "open") {
        await openWorkspaceFile(props.part.path);
      } else {
        await revealWorkspaceFile(props.part.path);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusyAction("");
    }
  };

  const runFromSummary = (event: MouseEvent, action: "view" | "preview" | "open" | "reveal") => {
    event.preventDefault();
    event.stopPropagation();
    void run(action);
  };

  return (
    <details class="op-file-artifact" open={error() ? true : undefined}>
      <summary class="op-file" title={props.part.path}>
        <span class="op-file-icon" aria-hidden="true">
          <Show when={isHTML()} fallback={<FileCode2 size={18} />}>
            <Globe2 size={18} />
          </Show>
        </span>
        <span class="op-file-info">
          <strong>{fileName()}</strong>
          <small>{fileMeta(props.part)}</small>
        </span>
        <span class="op-file-actions">
          <button
            type="button"
            disabled={Boolean(busyAction())}
            onClick={(event) => runFromSummary(event, "view")}
            title="在右侧查看文件"
          >
            <Show when={busyAction() === "view"} fallback={<Eye size={14} />}>
              <LoaderCircle class="spinning" size={14} />
            </Show>
            查看
          </button>
          <Show when={isHTML()}>
            <button class="op-file-system" type="button" disabled={Boolean(busyAction())} onClick={(event) => runFromSummary(event, "preview")} title="在内置浏览器中预览">
              <Show when={busyAction() === "preview"} fallback={<Globe2 size={14} />}>
                <LoaderCircle class="spinning" size={14} />
              </Show>
              预览
            </button>
          </Show>
          <button class="op-file-system" type="button" disabled={Boolean(busyAction())} onClick={(event) => runFromSummary(event, "open")} title="使用系统应用打开" aria-label="使用系统应用打开">
            <Show when={busyAction() === "open"} fallback={<ExternalLink size={14} />}>
              <LoaderCircle class="spinning" size={14} />
            </Show>
          </button>
          <button class="op-file-reveal" type="button" disabled={Boolean(busyAction())} onClick={(event) => runFromSummary(event, "reveal")} title="在文件夹中显示" aria-label="在文件夹中显示">
            <Show when={busyAction() === "reveal"} fallback={<FolderOpen size={15} />}>
              <LoaderCircle class="spinning" size={15} />
            </Show>
          </button>
        </span>
        <ChevronRight class="op-file-chevron" size={15} aria-hidden="true" />
      </summary>
      <div class="op-file-detail">
        <code title={props.part.path}>{props.part.path}</code>
        <Show when={error()}>
          <span class="op-file-error">{error()}</span>
        </Show>
      </div>
    </details>
  );
}

function MarkdownBlock(props: {
  source: string;
  onOpenURL?: (url: string) => void | Promise<void>;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
}) {
  const handleClick = (event: MouseEvent) => {
    handleCodeCopyClick(event);
    const target = event.target as HTMLElement | null;
    const workspaceLink = target?.closest<HTMLElement>("[data-workspace-path]");
    if (workspaceLink) {
      event.preventDefault();
      event.stopPropagation();
      const path = workspaceLink.dataset.workspacePath ?? "";
      const line = Number(workspaceLink.dataset.workspaceLine ?? "") || undefined;
      if (path && props.onOpenWorkspaceFile) void props.onOpenWorkspaceFile(path, "file", line);
      return;
    }
    const anchor = target?.closest<HTMLAnchorElement>("a[href]");
    if (!anchor) return;
    const href = anchor.getAttribute("href") ?? "";
    if (/^https?:\/\//i.test(href)) {
      if (!props.onOpenURL) return;
      event.preventDefault();
      event.stopPropagation();
      void props.onOpenURL(href);
      return;
    }
    const candidate = parseWorkspacePathCandidate(href);
    if (!candidate || !props.onOpenWorkspaceFile) return;
    event.preventDefault();
    event.stopPropagation();
    void props.onOpenWorkspaceFile(candidate.path, "file", candidate.line);
  };
  return (
    <div class="md-body" onClick={handleClick} innerHTML={renderMarkdown(props.source)} />
  );
}

function toolStatusLabel(status: "running" | "ok" | "error"): string {
  if (status === "running") return "执行中…";
  if (status === "error") return "失败";
  return "完成";
}

function baseName(path: string): string {
  const normalized = path.replaceAll("\\", "/");
  return normalized.split("/").filter(Boolean).at(-1) || path;
}

function extensionOf(path: string): string {
  const name = baseName(path);
  const dot = name.lastIndexOf(".");
  return dot > 0 ? name.slice(dot + 1).toLowerCase() : "";
}

function fileMeta(part: Extract<MessagePart, { kind: "file" }>): string {
  const extension = extensionOf(part.path);
  const kind = extension ? `${extension.toUpperCase()} 文件` : "文件";
  const action = part.fileAction === "created" || part.created === true
    ? "已创建"
    : part.fileAction === "modified" || part.created === false
      ? "已修改"
      : "可打开";
  return part.lineCount ? `${kind} · ${part.lineCount} 行 · ${action}` : `${kind} · ${action}`;
}

function withInferredFileArtifacts(parts: MessagePart[], enabled: boolean): MessagePart[] {
  if (!enabled) {
    return parts;
  }
  const explicitFiles = new Set(parts.filter((part) => part.kind === "file").map((part) => part.path));
  if (!parts.some((part) => part.kind === "diff" && !explicitFiles.has(part.path))) {
    return parts;
  }
  const rendered: MessagePart[] = [];
  for (const part of parts) {
    rendered.push(part);
    if (part.kind === "diff" && !explicitFiles.has(part.path)) {
      rendered.push({ kind: "file", path: part.path, fileAction: "modified" });
    }
  }
  return rendered;
}

export function textToParts(content: string): MessagePart[] {
  return [{ kind: "text", text: content }];
}
