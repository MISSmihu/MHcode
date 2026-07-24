import { For, Match, Switch, Show, createMemo, createSignal, onCleanup } from "solid-js";
import {
  AlertCircle,
  Check,
	Copy,
  ChevronDown,
  ChevronRight,
  Circle,
  Eye,
	EyeOff,
  ExternalLink,
  FileCode2,
  FolderOpen,
  GitBranch,
  Globe2,
  Image as ImageIcon,
	KeyRound,
  LoaderCircle,
  Monitor,
  Pencil,
  Route,
  Search,
  ShieldAlert,
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
import { writeClipboardText } from "../../lib/clipboard";
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
	onRevealSecret?: (secretID: string) => Promise<string>;
	isDisclosureOpen?: (key: string) => boolean;
	onDisclosureChange?: (key: string, open: boolean) => void;
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
				isDisclosureOpen={props.isDisclosureOpen}
				onDisclosureChange={props.onDisclosureChange}
              />
            </Match>
            <Match when={block.kind === "team"}>
              <Show when={!props.hideTeamRun}>
                <TeamRun parts={(block as TeamRenderBlock).parts} />
              </Show>
            </Match>
            <Match when={block.kind === "subagents"}>
              <SubagentRun parts={(block as SubagentRenderBlock).parts} />
            </Match>
            <Match when={block.kind === "provider"}>
              <ProviderNotice part={(block as ProviderRenderBlock).part} />
            </Match>
			<Match when={block.kind === "secret"}>
			  <SecretResultCard
				part={(block as SecretRenderBlock).part}
				onReveal={props.onRevealSecret}
			  />
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
export type SubagentPart = Extract<MessagePart, { kind: "subagent" }>;
type ProviderNoticePart = Extract<MessagePart, { kind: "provider_notice" }>;
type SecretResultPart = Extract<MessagePart, { kind: "secret_result" }>;
type TextRenderBlock = { kind: "text"; part: TextPart };
type ActivityRenderBlock = { kind: "activity"; parts: MessagePart[] };
type TeamRenderBlock = { kind: "team"; parts: TeamPart[] };
type SubagentRenderBlock = { kind: "subagents"; parts: SubagentPart[] };
type ProviderRenderBlock = { kind: "provider"; part: ProviderNoticePart };
type SecretRenderBlock = { kind: "secret"; part: SecretResultPart };
type RenderBlock = TextRenderBlock | ActivityRenderBlock | TeamRenderBlock | SubagentRenderBlock | ProviderRenderBlock | SecretRenderBlock;
type ActivityCategory = "command" | "edit" | "read" | "directory" | "search" | "web" | "repository" | "image" | "browser" | "computer" | "open" | "file" | "tool";
type ActivityItem = { category: ActivityCategory; parts: MessagePart[] };
type DisclosureProps = {
	isDisclosureOpen?: (key: string) => boolean;
	onDisclosureChange?: (key: string, open: boolean) => void;
};

function ActivityGroup(props: {
  parts: MessagePart[];
  onPreviewFile?: (path: string) => void | Promise<void>;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
  onOpenURL?: (url: string) => void | Promise<void>;
} & DisclosureProps) {
  const items = createMemo(() => buildActivityItems(props.parts));
  const artifacts = createMemo(() => activityArtifacts(props.parts));
  return (
    <>
      <div class="op-activity-feed">
        <For each={items()}>
          {(item, index) => (
            <ActivityRow
              item={item}
			  disclosureKey={activityDisclosureKey(item, index())}
              onOpenURL={props.onOpenURL}
              onOpenWorkspaceFile={props.onOpenWorkspaceFile}
			  isDisclosureOpen={props.isDisclosureOpen}
			  onDisclosureChange={props.onDisclosureChange}
            />
          )}
        </For>
      </div>
      <For each={artifacts()}>
        {(part, index) => (
          <FileCard
            part={part}
			disclosureKey={`file:${index()}:${part.path}`}
            onPreviewFile={props.onPreviewFile}
            onOpenWorkspaceFile={props.onOpenWorkspaceFile}
			isDisclosureOpen={props.isDisclosureOpen}
			onDisclosureChange={props.onDisclosureChange}
          />
        )}
      </For>
    </>
  );
}

function ActivityRow(props: {
  item: ActivityItem;
	disclosureKey: string;
  onOpenURL?: (url: string) => void | Promise<void>;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
} & DisclosureProps) {
  const status = () => activityStatus(props.item);
  return (
    <details
      class="op-activity-item"
      classList={{ [status()]: true }}
	  open={props.isDisclosureOpen?.(props.disclosureKey)}
	  onToggle={(event) => props.onDisclosureChange?.(props.disclosureKey, event.currentTarget.open)}
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
        <Show when={props.item.category === "command"} fallback={
          <For each={props.item.parts}>
            {(part, index) => (
              <Switch>
                <Match when={part.kind === "tool_call"}>
                  <ToolDetail
                    part={part as ToolPart}
					disclosureKey={`tool:${index()}:${partDisclosureIdentity(part)}`}
                    hideOutput={props.item.parts.some((item) => item.kind === "web_search_results")}
                    onOpenWorkspaceFile={props.onOpenWorkspaceFile}
					isDisclosureOpen={props.isDisclosureOpen}
					onDisclosureChange={props.onDisclosureChange}
                  />
                </Match>
                <Match when={part.kind === "diff"}>
				  <DiffDetail
					part={part as DiffPart}
					disclosureKey={`diff:${index()}:${partDisclosureIdentity(part)}`}
					onOpenWorkspaceFile={props.onOpenWorkspaceFile}
					isDisclosureOpen={props.isDisclosureOpen}
					onDisclosureChange={props.onDisclosureChange}
				  />
                </Match>
                <Match when={part.kind === "web_search_results"}>
                  <WebSearchResults part={part as WebSearchPart} onOpenURL={props.onOpenURL} />
                </Match>
              </Switch>
            )}
          </For>
        }>
		  <CommandActivityList
			parts={props.item.parts}
			isDisclosureOpen={props.isDisclosureOpen}
			onDisclosureChange={props.onDisclosureChange}
		  />
        </Show>
      </div>
    </details>
  );
}

function CommandActivityList(props: { parts: MessagePart[] } & DisclosureProps) {
  const commands = createMemo(() => props.parts.filter((part): part is ToolPart => part.kind === "tool_call"));
  return (
    <div class="op-command-list">
      <For each={commands()}>
        {(part, index) => {
		  const disclosureKey = () => `command:${index()}:${partDisclosureIdentity(part)}`;
		  return (
          <details
			class="op-command-entry"
			classList={{ [part.status ?? "ok"]: true }}
			open={props.isDisclosureOpen?.(disclosureKey())}
			onToggle={(event) => props.onDisclosureChange?.(disclosureKey(), event.currentTarget.open)}
		  >
            <summary title={commandDisplayInput(part)}>
              <span class="op-command-state" aria-hidden="true">
                <Show when={part.status === "running"} fallback={
                  <Show when={part.status === "error"} fallback={<Check size={12} />}>
                    <AlertCircle size={12} />
                  </Show>
                }>
                  <span class="op-activity-spinner" />
                </Show>
              </span>
              <code>{commandDisplayInput(part)}</code>
              <span class="op-command-meta">{commandPartStatus(part)}</span>
              <ChevronRight size={13} aria-hidden="true" />
            </summary>
            <ShellToolDetail part={part} />
          </details>
		  );
		}}
      </For>
    </div>
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
	disclosureKey: string;
  hideOutput?: boolean;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
} & DisclosureProps) {
  if (props.part.name === "run_command" || props.part.name === "terminal" || props.part.name === "ssh") {
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
			expanded={props.isDisclosureOpen?.(`${props.disclosureKey}:code`)}
			onExpandedChange={(open) => props.onDisclosureChange?.(`${props.disclosureKey}:code`, open)}
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
	const [copied, setCopied] = createSignal<"command" | "output" | "">("");
	const stdout = () => props.part.stdout ?? (props.part.stderr ? "" : props.part.output ?? "");
	const stderr = () => props.part.stderr ?? "";
	const combinedOutput = () => [stdout(), stderr()].filter(Boolean).join("\n") || props.part.output || "";
	const copy = async (kind: "command" | "output", value: string) => {
		if (!value) return;
		await navigator.clipboard.writeText(value);
		setCopied(kind);
		window.setTimeout(() => setCopied(""), 1_200);
	};
  return (
    <div class="op-activity-detail op-shell-detail">
      <div class="op-activity-detail-head">
        <span class="op-shell-title"><TerminalSquare size={13} /><code>{props.part.name === "terminal" ? "Terminal" : props.part.name === "ssh" ? "SSH" : "Shell"}</code></span>
        <span class="op-shell-meta">
		  <Show when={props.part.status === "running"}>
			<em class="running"><LivePartElapsed startedAt={props.part.startedAt} /></em>
		  </Show>
          <Show when={props.part.exitCode !== undefined}>
            <em classList={{ error: props.part.exitCode !== 0 }}>exit {props.part.exitCode}</em>
          </Show>
          <Show when={duration()}><em>{duration()}</em></Show>
        </span>
      </div>
	  <div class="op-shell-command">
		<span aria-hidden="true">$</span><code>{commandDisplayInput(props.part)}</code>
		<button type="button" title="复制命令" aria-label="复制命令" onClick={() => void copy("command", commandDisplayInput(props.part))}>
		  <Show when={copied() === "command"} fallback={<Copy size={12} />}><Check size={12} /></Show>
		</button>
	  </div>
      <Show when={props.part.workingDirectory}>
        <div class="op-shell-workdir" title={props.part.workingDirectory}>cwd <code>{props.part.workingDirectory}</code></div>
      </Show>
	  <div class="op-shell-output-head">
		<span>输出</span>
		<button type="button" title="复制输出" aria-label="复制输出" onClick={() => void copy("output", combinedOutput())}>
		  <Show when={copied() === "output"} fallback={<Copy size={12} />}><Check size={12} /></Show>
		</button>
	  </div>
	  <Show when={stdout()} fallback={<Show when={!stderr()}><pre class="op-shell-output muted">{props.part.status === "running" ? "等待命令输出…" : "(无输出)"}</pre></Show>}>
		<pre class="op-shell-output stdout">{stdout()}</pre>
	  </Show>
	  <Show when={stderr()}>
		<div class="op-shell-output-label error">stderr</div>
		<pre class="op-shell-output stderr">{stderr()}</pre>
	  </Show>
    </div>
  );
}

function commandDisplayInput(part: ToolPart): string {
  const input = part.input?.trim() ?? "";
  if (part.name !== "terminal" || !input.startsWith("{")) {
    return input || (part.name === "terminal" ? "终端操作" : "(空命令)");
  }
  try {
    const args = JSON.parse(input) as { action?: string; command?: string; session_id?: string };
    if (args.command?.trim()) return args.command.trim();
    const session = args.session_id?.trim() ? ` ${args.session_id.trim()}` : "";
    switch (args.action?.trim().toLowerCase()) {
      case "start": return "启动持久终端";
      case "state": return `读取终端状态${session}`;
      case "stop": return `停止持久终端${session}`;
      default: return `${args.action?.trim() || "终端操作"}${session}`;
    }
  } catch {
    return input;
  }
}

function commandPartStatus(part: ToolPart): string {
  const duration = part.durationMs !== undefined ? ` · ${formatElapsedDuration(part.durationMs)}` : "";
  if (part.status === "running") return "正在运行";
  if (part.status === "error") return `失败${duration}`;
  if (part.exitCode !== undefined && part.exitCode !== 0) return `exit ${part.exitCode}${duration}`;
  return `已运行${duration}`;
}

function LivePartElapsed(props: { startedAt?: string }) {
	const startedAt = new Date(props.startedAt ?? "").getTime();
	const readElapsed = () => Math.max(0, Date.now() - (Number.isFinite(startedAt) ? startedAt : Date.now()));
	const [elapsed, setElapsed] = createSignal(readElapsed());
	const timer = window.setInterval(() => setElapsed(readElapsed()), 1_000);
	onCleanup(() => window.clearInterval(timer));
	return <>{formatElapsedDuration(elapsed())}</>;
}

function DiffDetail(props: {
  part: DiffPart;
	disclosureKey: string;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
} & DisclosureProps) {
  return (
    <InlineDiffPreview
      path={props.part.path}
      patch={props.part.patch}
      additions={props.part.additions}
      deletions={props.part.deletions}
	  expanded={props.isDisclosureOpen?.(props.disclosureKey)}
	  onExpandedChange={(open) => props.onDisclosureChange?.(props.disclosureKey, open)}
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
  const subagentParts = parts.filter((part): part is SubagentPart => part.kind === "subagent");
  let teamAdded = false;
  let subagentsAdded = false;
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
    if (part.kind === "subagent") {
      if (!subagentsAdded) {
        blocks.push({ kind: "subagents", parts: subagentParts });
        subagentsAdded = true;
      }
      continue;
    }
    if (part.kind === "tool_call" && part.name === "delegate_task" && subagentParts.length > 0) {
      continue;
    }
    if (part.kind === "provider_notice") {
      blocks.push({ kind: "provider", part });
      continue;
    }
	if (part.kind === "secret_result") {
	  blocks.push({ kind: "secret", part });
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
  if (part.kind === "subagent") return "tool";
  if (part.kind === "provider_notice") return "tool";
	if (part.kind === "secret_result") return "tool";
  switch (part.name) {
    case "run_command":
    case "terminal":
    case "ssh": return "command";
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

function activityDisclosureKey(item: ActivityItem, index: number): string {
	return `activity:${index}:${item.category}:${partDisclosureIdentity(item.parts[0])}`;
}

function partDisclosureIdentity(part: MessagePart | undefined): string {
	if (!part) return "empty";
	switch (part.kind) {
		case "tool_call":
			return part.toolCallId || `${part.name}:${part.input ?? ""}`;
		case "diff":
			return `diff:${part.path}`;
		case "file":
			return `file:${part.path}`;
		case "web_search_results":
			return `web:${part.query}`;
		case "team_role":
			return `team:${part.role}:${part.attempt ?? 1}`;
		case "subagent":
			return `subagent:${part.taskId}`;
		case "provider_notice":
			return `provider:${part.noticeKind}:${part.requestId ?? ""}`;
		case "secret_result":
			return `secret:${part.secretId}`;
		case "task_progress":
			return "progress";
		case "text":
			return "text";
	}
}

function ProviderNotice(props: { part: ProviderNoticePart }) {
  const title = () => {
    switch (props.part.noticeKind) {
      case "model_reroute": return "服务端调整了本轮模型";
      case "safety_buffering": return "服务端安全检查";
      case "model_verification": return "供应商建议验证访问资格";
      case "moderation": return "供应商内容安全元数据";
      case "policy_error": return "请求被供应商安全策略阻止";
      case "provider_error": return "供应商请求失败";
      default: return "供应商运行通知";
    }
  };
  const summary = () => {
    if (props.part.noticeKind === "model_reroute") {
      return `${props.part.requestedModel || "请求模型"} → ${props.part.effectiveModel || "实际模型"}`;
    }
    if (props.part.noticeKind === "safety_buffering") {
      return props.part.retryModel ? `必要时由 ${props.part.retryModel} 继续处理` : "本轮正在由上游安全路由处理";
    }
    if (props.part.noticeKind === "model_verification") {
      return (props.part.verifications ?? []).join("、") || "上游返回了账户验证建议";
    }
    if (props.part.noticeKind === "moderation") {
      return props.part.metadataKeys?.length ? `已读取 ${props.part.metadataKeys.length} 项元数据` : "已读取本轮元数据";
    }
    if (props.part.noticeKind === "policy_error") {
      return props.part.message || "上游拒绝了本轮请求";
    }
    if (props.part.noticeKind === "provider_error") {
      return props.part.message || "上游服务未完成本轮请求";
    }
    return props.part.message || "已收到供应商运行信息";
  };
  const details = () => [
    ...(props.part.useCases?.length ? [`用途：${props.part.useCases.join("、")}`] : []),
    ...(props.part.reasons?.length ? [`原因：${props.part.reasons.join("、")}`] : []),
    ...(props.part.errorCode ? [`错误代码：${props.part.errorCode}`] : []),
    ...(props.part.httpStatus ? [`HTTP：${props.part.httpStatus}`] : []),
    ...(props.part.retryable === false ? ["该错误不可自动重试"] : []),
    ...(props.part.requestId ? [`请求 ID：${props.part.requestId}`] : []),
  ];
  return (
    <section class="op-provider-notice" classList={{
      warning: props.part.severity === "warning",
      error: props.part.severity === "error",
    }} aria-label={title()}>
      <span class="op-provider-notice-icon" aria-hidden="true">
        <Show when={props.part.noticeKind === "model_reroute"} fallback={<ShieldAlert size={15} />}>
          <Route size={15} />
        </Show>
      </span>
      <span class="op-provider-notice-main">
        <strong>{title()}</strong>
        <small>{summary()}</small>
        <Show when={details().length > 0}>
          <span class="op-provider-notice-meta">{details().join(" · ")}</span>
        </Show>
      </span>
    </section>
  );
}

export function SubagentRun(props: { parts: SubagentPart[] }) {
  const completed = createMemo(() => props.parts.filter((part) => part.status === "completed").length);
  const running = createMemo(() => props.parts.filter((part) => part.status === "running").length);
  return (
    <section class="op-subagent-run" aria-label="动态子代理执行记录">
      <header class="op-subagent-head">
        <span class="op-subagent-mark" aria-hidden="true"><Route size={14} /></span>
        <span class="op-subagent-title">
          <strong>子代理</strong>
          <small>{running() > 0 ? `${running()} 个正在工作` : `${completed()}/${props.parts.length} 已完成`}</small>
        </span>
      </header>
      <div class="op-subagent-list">
        <For each={props.parts}>
          {(part) => (
            <article class="op-subagent-item" classList={{ [part.status || "pending"]: true }}>
              <span class="op-subagent-status" aria-hidden="true"><SubagentStatus status={part.status} /></span>
              <span class="op-subagent-main">
                <span class="op-subagent-name">
                  <strong>{part.label || "子任务"}</strong>
                  <small>{subagentTypeLabel(part.agentType)}</small>
                </span>
                <small class="op-subagent-meta">
                  {part.model || "跟随当前模型"}
                  {part.durationMs !== undefined ? ` · ${formatElapsedDuration(part.durationMs)}` : ""}
                  {part.changedFiles ? ` · ${part.changedFiles} 个文件` : ""}
                </small>
                <Show when={part.currentAction && part.status !== "completed"}>
                  <span class="op-subagent-action">{part.currentAction}</span>
                </Show>
              </span>
              <Show when={(part.steps?.length ?? 0) > 0 || part.summary}>
                <details class="op-subagent-details">
                  <summary>查看工作记录 <ChevronRight size={12} /></summary>
                  <Show when={(part.steps?.length ?? 0) > 0}>
                    <ol class="op-subagent-steps">
                      <For each={part.steps ?? []}>
                        {(step) => <li classList={{ [step.status]: true }}>{step.title}</li>}
                      </For>
                    </ol>
                  </Show>
                  <Show when={part.summary}>
                    <p>{part.summary}</p>
                  </Show>
                </details>
              </Show>
            </article>
          )}
        </For>
      </div>
    </section>
  );
}

function SubagentStatus(props: { status?: string }) {
  return (
    <Show when={props.status === "completed"} fallback={
      <Show when={props.status === "error" || props.status === "cancelled"} fallback={
        <Show when={props.status === "running"} fallback={<Circle size={11} />}>
          <span class="op-subagent-spinner" />
        </Show>
      }>
        <AlertCircle size={12} />
      </Show>
    }>
      <Check size={12} />
    </Show>
  );
}

function subagentTypeLabel(agentType: string): string {
  switch (agentType) {
    case "explore": return "探索";
    case "review": return "审阅";
    case "implement": return "实现";
    default: return agentType || "任务";
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
	const running = activityStatus(item) === "running";
  switch (item.category) {
    case "command":
	  return running ? (tools.length > 1 ? `正在运行 ${tools.length} 个命令` : "正在运行命令") : tools.length > 1 ? `运行了 ${tools.length} 个命令` : "运行了命令";
    case "edit":
	  return running ? "正在编辑文件" : files.length > 1 ? `编辑了 ${files.length} 个文件` : files[0] ? `编辑了 ${baseName(files[0])}` : "编辑了文件";
    case "read":
	  return running ? (input ? `正在读取 ${baseName(input)}` : "正在读取文件") : tools.length > 1 ? `读取了 ${tools.length} 个文件` : input ? `读取了 ${baseName(input)}` : "读取了文件";
    case "directory":
	  return running ? (input ? `正在查看目录 ${input}` : "正在查看目录") : input ? `查看了目录 ${input}` : "查看了目录";
    case "search":
	  return running ? (input ? `正在搜索代码“${compactLabel(input)}”` : "正在搜索代码") : input ? `搜索了代码“${compactLabel(input)}”` : "搜索了代码";
    case "web":
      if (tools.some((part) => part.name === "read_webpage")) {
		return running ? (input ? `正在读取网页 ${compactLabel(input)}` : "正在读取网页正文") : input ? `读取了网页 ${compactLabel(input)}` : "读取了网页正文";
      }
	  return running ? (input ? `正在搜索网络“${compactLabel(input)}”` : "正在搜索网络") : input ? `搜索了网络“${compactLabel(input)}”` : "搜索了网络";
    case "repository":
	  return running ? (input ? `正在读取仓库 ${compactLabel(input)}` : "正在读取代码仓库") : input ? `读取了仓库 ${compactLabel(input)}` : "读取了代码仓库";
    case "image": {
      const count = Math.max(1, files.filter(isImagePath).length);
      return `查看了 ${count} 张图像`;
    }
    case "browser":
	  return running ? "正在使用内置浏览器" : input.startsWith("http") ? "打开了网页" : "使用了内置浏览器";
    case "computer":
	  return running ? "正在操作其他窗口" : input ? `操作了窗口（${compactLabel(input)}）` : "操作了其他窗口";
    case "open":
      return input ? `打开了 ${baseName(input)}` : "打开了文件";
    case "file":
      return files.length > 1 ? `生成了 ${files.length} 个文件` : files[0] ? `生成了 ${baseName(files[0])}` : "生成了文件";
    default:
	  return running ? `正在运行 ${friendlyToolName(tools[0]?.name)}` : tools.length > 1 ? `运行了 ${tools.length} 个工具` : `运行了 ${friendlyToolName(tools[0]?.name)}`;
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

function SecretResultCard(props: {
  part: SecretResultPart;
  onReveal?: (secretID: string) => Promise<string>;
}) {
  const [value, setValue] = createSignal("");
  const [revealed, setRevealed] = createSignal(false);
  const [busy, setBusy] = createSignal<"reveal" | "copy" | "">("");
  const [copied, setCopied] = createSignal(false);
  const [error, setError] = createSignal("");
  let hideTimer: number | undefined;

  const clearHideTimer = () => {
    if (hideTimer !== undefined) window.clearTimeout(hideTimer);
    hideTimer = undefined;
  };
  const hide = () => {
    clearHideTimer();
    setRevealed(false);
    setValue("");
  };
  const loadValue = async () => {
    if (!props.onReveal) throw new Error("当前视图无法读取该敏感结果。");
    return props.onReveal(props.part.secretId);
  };
  const toggleReveal = async () => {
    if (revealed()) {
      hide();
      return;
    }
    setBusy("reveal");
    setError("");
    try {
      const nextValue = await loadValue();
      setValue(nextValue);
      setRevealed(true);
      clearHideTimer();
      hideTimer = window.setTimeout(hide, 60_000);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy("");
    }
  };
  const copyValue = async () => {
    setBusy("copy");
    setError("");
    try {
      const nextValue = value() || await loadValue();
      await writeClipboardText(nextValue);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1_400);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy("");
    }
  };

  onCleanup(clearHideTimer);

  return (
    <section class="op-secret-result" classList={{ error: props.part.status === "error" }} aria-label="敏感结果">
      <div class="op-secret-result-icon" aria-hidden="true"><KeyRound size={16} /></div>
      <div class="op-secret-result-main">
        <strong>{props.part.secretLabel || "远程敏感结果"}</strong>
        <Show when={props.part.secretSource}><small>{props.part.secretSource}</small></Show>
        <Show
          when={revealed()}
          fallback={<span class="op-secret-result-mask">内容已安全保存，查看时才从本机凭据库读取</span>}
        >
          <code class="op-secret-result-value">{value()}</code>
        </Show>
        <Show when={error()}><span class="op-secret-result-error">{error()}</span></Show>
      </div>
      <div class="op-secret-result-actions">
        <button
          type="button"
          disabled={Boolean(busy()) || !props.onReveal}
          title={revealed() ? "隐藏敏感结果" : "查看敏感结果"}
          onClick={() => void toggleReveal()}
        >
          <Show when={busy() === "reveal"} fallback={revealed() ? <EyeOff size={14} /> : <Eye size={14} />}>
            <LoaderCircle class="spinning" size={14} />
          </Show>
          {revealed() ? "隐藏" : "查看"}
        </button>
        <button
          type="button"
          disabled={Boolean(busy()) || !props.onReveal}
          title="复制敏感结果"
          onClick={() => void copyValue()}
        >
          <Show when={busy() === "copy"} fallback={copied() ? <Check size={14} /> : <Copy size={14} />}>
            <LoaderCircle class="spinning" size={14} />
          </Show>
          {copied() ? "已复制" : "复制"}
        </button>
      </div>
    </section>
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
	disclosureKey: string;
  onPreviewFile?: (path: string) => void | Promise<void>;
  onOpenWorkspaceFile?: (path: string, view?: WorkspaceFileView, line?: number) => void | Promise<void>;
} & DisclosureProps) {
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
	<details
	  class="op-file-artifact"
	  open={error() ? true : props.isDisclosureOpen?.(props.disclosureKey)}
	  onToggle={(event) => props.onDisclosureChange?.(props.disclosureKey, event.currentTarget.open)}
	>
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
