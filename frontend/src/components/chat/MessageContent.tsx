import { For, Match, Switch, Show, createMemo, createSignal } from "solid-js";
import {
  AlertCircle,
  Check,
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
  Users,
  Wrench,
} from "lucide-solid";
import type { MessagePart } from "../../types";
import { renderMarkdown, handleCodeCopyClick } from "../../lib/markdown";
import { openWorkspaceFile, revealWorkspaceFile } from "../../services/workbench";

// 操作流渲染：按片段类型分派。工具/diff 卡片默认折叠，用户按需展开（对标 ZCode/Codex）。
export function MessageContent(props: {
  parts: MessagePart[];
  inferFileArtifacts?: boolean;
  onPreviewFile?: (path: string) => void | Promise<void>;
  onOpenURL?: (url: string) => void | Promise<void>;
}) {
  const renderedParts = createMemo(() => withInferredFileArtifacts(props.parts, props.inferFileArtifacts !== false));
  const blocks = createMemo(() => groupRenderBlocks(renderedParts()));
  return (
    <div class="op-stream">
      <For each={blocks()}>
        {(block) => (
          <Switch>
            <Match when={block.kind === "text"}>
              <MarkdownBlock source={(block as TextRenderBlock).part.text} onOpenURL={props.onOpenURL} />
            </Match>
            <Match when={block.kind === "activity"}>
              <ActivityGroup
                parts={(block as ActivityRenderBlock).parts}
                onPreviewFile={props.onPreviewFile}
                onOpenURL={props.onOpenURL}
              />
            </Match>
            <Match when={block.kind === "team"}>
              <TeamRun parts={(block as TeamRenderBlock).parts} />
            </Match>
          </Switch>
        )}
      </For>
    </div>
  );
}

type TextPart = Extract<MessagePart, { kind: "text" }>;
type ToolPart = Extract<MessagePart, { kind: "tool_call" }>;
type DiffPart = Extract<MessagePart, { kind: "diff" }>;
type FilePart = Extract<MessagePart, { kind: "file" }>;
export type TaskProgressPart = Extract<MessagePart, { kind: "task_progress" }>;
type WebSearchPart = Extract<MessagePart, { kind: "web_search_results" }>;
type TeamPart = Extract<MessagePart, { kind: "team_role" }>;
type TextRenderBlock = { kind: "text"; part: TextPart };
type ActivityRenderBlock = { kind: "activity"; parts: MessagePart[] };
type TeamRenderBlock = { kind: "team"; parts: TeamPart[] };
type RenderBlock = TextRenderBlock | ActivityRenderBlock | TeamRenderBlock;
type ActivityCategory = "command" | "edit" | "read" | "directory" | "search" | "web" | "repository" | "image" | "browser" | "computer" | "open" | "file" | "tool";
type ActivityItem = { category: ActivityCategory; parts: MessagePart[] };

function ActivityGroup(props: {
  parts: MessagePart[];
  onPreviewFile?: (path: string) => void | Promise<void>;
  onOpenURL?: (url: string) => void | Promise<void>;
}) {
  const items = createMemo(() => buildActivityItems(props.parts));
  const artifacts = createMemo(() => activityArtifacts(props.parts));
  return (
    <>
      <div class="op-activity-feed">
        <For each={items()}>{(item) => <ActivityRow item={item} onOpenURL={props.onOpenURL} />}</For>
      </div>
      <For each={artifacts()}>
        {(part) => <FileCard part={part} onPreviewFile={props.onPreviewFile} />}
      </For>
    </>
  );
}

function ActivityRow(props: { item: ActivityItem; onOpenURL?: (url: string) => void | Promise<void> }) {
  const status = () => activityStatus(props.item);
  return (
    <details class="op-activity-item" classList={{ [status()]: true }} open={status() === "error"}>
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
                />
              </Match>
              <Match when={part.kind === "diff"}>
                <DiffDetail part={part as DiffPart} />
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

function ToolDetail(props: { part: ToolPart; hideOutput?: boolean }) {
  return (
    <div class="op-activity-detail">
      <div class="op-activity-detail-head">
        <code>{props.part.name}</code>
        <span>{toolStatusLabel(props.part.status ?? "ok")}</span>
      </div>
      <Show when={props.part.input}><pre>{props.part.input}</pre></Show>
      <Show when={props.part.output && !props.hideOutput}><pre>{props.part.output}</pre></Show>
    </div>
  );
}

function DiffDetail(props: { part: DiffPart }) {
  const lines = () => props.part.patch.split("\n");
  return (
    <div class="op-activity-diff">
      <div class="op-activity-detail-head">
        <code>{props.part.path}</code>
        <span class="op-diff-stat">
          <Show when={props.part.additions}><em class="add">+{props.part.additions}</em></Show>
          <Show when={props.part.deletions}><em class="del">-{props.part.deletions}</em></Show>
        </span>
      </div>
      <pre class="op-diff-body">
        <For each={lines()}>{(line) => <DiffLine line={line} />}</For>
      </pre>
    </div>
  );
}

function DiffLine(props: { line: string }) {
  const add = () => props.line.startsWith("+") && !props.line.startsWith("+++");
  const del = () => props.line.startsWith("-") && !props.line.startsWith("---");
  const meta = () => props.line.startsWith("@@") || props.line.startsWith("diff ") || props.line.startsWith("index ") || props.line.startsWith("--- ") || props.line.startsWith("+++ ");
  return <span class="op-line" classList={{ add: add(), del: del(), meta: meta() }}>{props.line || " "}</span>;
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

function TeamRun(props: { parts: TeamPart[] }) {
  return (
    <section class="op-team-run" aria-label="AI 团队执行记录">
      <div class="op-team-head"><Users size={14} /><strong>AI 团队</strong></div>
      <div class="op-team-roles">
        <For each={props.parts}>
          {(part) => (
            <div class="op-team-role" classList={{ [part.status || "pending"]: true }}>
              <span class="op-team-role-status" aria-hidden="true">
                <Show when={part.status === "completed"} fallback={
                  <Show when={part.status === "error"} fallback={
                    <Show when={part.status === "running"} fallback={<Circle size={12} />}>
                      <span class="op-team-spinner" />
                    </Show>
                  }>
                    <AlertCircle size={13} />
                  </Show>
                }>
                  <Check size={13} />
                </Show>
              </span>
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
    </section>
  );
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
    if (part.fileAction !== "created" && part.fileAction !== "modified" && part.created !== true) continue;
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

function uniqueStrings(values: string[]): string[] {
  return [...new Set(values.filter(Boolean))];
}

function isImagePath(path: string): boolean {
  return ["png", "jpg", "jpeg", "webp", "gif", "bmp"].includes(extensionOf(path));
}

function FileCard(props: {
  part: Extract<MessagePart, { kind: "file" }>;
  onPreviewFile?: (path: string) => void | Promise<void>;
}) {
  const [busyAction, setBusyAction] = createSignal<"preview" | "open" | "reveal" | "">("");
  const [error, setError] = createSignal("");
  const fileName = () => baseName(props.part.path);
  const isHTML = () => extensionOf(props.part.path) === "html" || extensionOf(props.part.path) === "htm";

  const run = async (action: "preview" | "open" | "reveal") => {
    setBusyAction(action);
    setError("");
    try {
      if (action === "preview") {
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

  return (
    <div class="op-file" title={props.part.path}>
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
          onClick={() => void run(isHTML() ? "preview" : "open")}
          title={isHTML() ? "在 MHcode 内置浏览器中预览" : "使用系统默认应用打开"}
        >
          <Show when={busyAction() === (isHTML() ? "preview" : "open")} fallback={isHTML() ? <Globe2 size={14} /> : <ExternalLink size={14} />}>
            <LoaderCircle class="spinning" size={14} />
          </Show>
          {isHTML() ? "预览" : "打开"}
        </button>
        <Show when={isHTML()}>
          <button class="op-file-system" type="button" disabled={Boolean(busyAction())} onClick={() => void run("open")} title="使用系统浏览器打开" aria-label="使用系统浏览器打开">
            <Show when={busyAction() === "open"} fallback={<ExternalLink size={15} />}>
              <LoaderCircle class="spinning" size={15} />
            </Show>
          </button>
        </Show>
        <button class="op-file-reveal" type="button" disabled={Boolean(busyAction())} onClick={() => void run("reveal")} title="在文件夹中显示" aria-label="在文件夹中显示">
          <Show when={busyAction() === "reveal"} fallback={<FolderOpen size={15} />}>
            <LoaderCircle class="spinning" size={15} />
          </Show>
        </button>
      </span>
      <Show when={error()}>
        <span class="op-file-error">{error()}</span>
      </Show>
    </div>
  );
}

function MarkdownBlock(props: { source: string; onOpenURL?: (url: string) => void | Promise<void> }) {
  const handleClick = (event: MouseEvent) => {
    handleCodeCopyClick(event);
    const target = event.target as HTMLElement | null;
    const anchor = target?.closest<HTMLAnchorElement>("a[href]");
    if (!anchor || !props.onOpenURL) return;
    event.preventDefault();
    event.stopPropagation();
    void props.onOpenURL(anchor.href);
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
