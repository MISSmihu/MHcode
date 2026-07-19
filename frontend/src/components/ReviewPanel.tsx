import {
  Check,
  FileDiff,
  ListFilter,
  Minus,
  Pilcrow,
  Plus,
  RefreshCw,
  Search,
  WrapText,
  X,
} from "lucide-solid";
import { For, Show, createEffect, createMemo, createSignal } from "solid-js";
import {
  getGitReviewDiff,
  getGitStatus,
  stageGitPaths,
  unstageGitPaths,
} from "../services/workbench";
import { countDiffChanges, decorateWordDifferences, parseUnifiedDiff } from "../lib/review-diff";
import type { GitDiff, GitFileStatus, GitStatus } from "../types";

type ReviewPanelProps = {
  open: boolean;
  workspaceRoot: string;
  readOnly: boolean;
  onClose: () => void;
};

export function ReviewPanel(props: ReviewPanelProps) {
  const [status, setStatus] = createSignal<GitStatus>();
  const [diff, setDiff] = createSignal<GitDiff>();
  const [selectedPath, setSelectedPath] = createSignal("");
  const [stagedDiff, setStagedDiff] = createSignal(false);
  const [filter, setFilter] = createSignal("");
  const [ignoreWhitespace, setIgnoreWhitespace] = createSignal(false);
  const [wordDiff, setWordDiff] = createSignal(true);
  const [wrapLines, setWrapLines] = createSignal(false);
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");
  let workspace = "";
  let diffRequest = 0;

  const selectedFile = createMemo(() => status()?.files.find((file) => file.path === selectedPath()));
  const filteredFiles = createMemo(() => {
    const query = filter().trim().toLowerCase();
    const files = status()?.files ?? [];
    return query ? files.filter((file) => file.path.toLowerCase().includes(query)) : files;
  });
  const parsedDiff = createMemo(() => {
    const rows = parseUnifiedDiff(diff()?.patch ?? "");
    return wordDiff() ? decorateWordDifferences(rows) : rows;
  });
  const diffStats = createMemo(() => countDiffChanges(diff()?.patch ?? ""));

  createEffect(() => {
    const root = props.workspaceRoot;
    if (!props.open) return;
    if (workspace !== root) {
      workspace = root;
      setSelectedPath("");
      setDiff(undefined);
      setFilter("");
    }
    void refresh();
  });

  async function refresh() {
    if (!props.workspaceRoot.trim()) {
      setStatus(undefined);
      setDiff(undefined);
      return;
    }
    setBusy(true);
    setError("");
    try {
      const next = await getGitStatus();
      setStatus(next);
      const current = next.files.find((file) => file.path === selectedPath());
      const file = current ?? next.files[0];
      if (file) {
        await selectFile(file, preferredDiffSide(file));
      } else {
        setSelectedPath("");
        setDiff(undefined);
      }
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusy(false);
    }
  }

  async function selectFile(file: GitFileStatus, staged = preferredDiffSide(file), whitespace = ignoreWhitespace()) {
    setSelectedPath(file.path);
    setStagedDiff(staged);
    setError("");
    const request = ++diffRequest;
    try {
      const next = await getGitReviewDiff(file.path, staged, whitespace);
      if (request === diffRequest) setDiff(next);
    } catch (cause) {
      if (request === diffRequest) {
        setDiff(undefined);
        setError(errorText(cause));
      }
    }
  }

  async function mutate(operation: () => Promise<GitStatus>) {
    setBusy(true);
    setError("");
    try {
      const next = await operation();
      setStatus(next);
      const file = next.files.find((item) => item.path === selectedPath());
      if (file) {
        await selectFile(file, stagedDiff() && file.staged ? true : preferredDiffSide(file));
      } else {
        setSelectedPath("");
        setDiff(undefined);
      }
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setBusy(false);
    }
  }

  async function toggleWhitespace() {
    const next = !ignoreWhitespace();
    setIgnoreWhitespace(next);
    const file = selectedFile();
    if (file) await selectFile(file, stagedDiff(), next);
  }

  async function toggleSelectedStage() {
    const file = selectedFile();
    if (!file || props.readOnly) return;
    if (stagedDiff()) {
      await mutate(() => unstageGitPaths([file.path]));
    } else {
      await mutate(() => stageGitPaths([file.path]));
    }
  }

  return (
    <Show when={props.open}>
      <aside class="review-panel" aria-label="代码审阅">
        <header class="review-head">
          <div class="review-title">
            <FileDiff size={16} />
            <strong>审阅</strong>
            <Show when={status()?.branch}><span>{status()?.branch}</span></Show>
            <Show when={selectedPath()}>
              <b class="review-additions">+{diffStats().additions}</b>
              <b class="review-deletions">-{diffStats().deletions}</b>
            </Show>
          </div>
          <div class="review-actions">
            <button
              type="button"
              classList={{ active: ignoreWhitespace() }}
              title="隐藏仅空白字符差异"
              aria-pressed={ignoreWhitespace()}
              disabled={busy()}
              onClick={() => void toggleWhitespace()}
            >
              <Pilcrow size={14} />
            </button>
            <button type="button" classList={{ active: wordDiff() }} title="字词差异" aria-pressed={wordDiff()} onClick={() => setWordDiff((value) => !value)}>
              <ListFilter size={14} />
            </button>
            <button type="button" classList={{ active: wrapLines() }} title="自动换行" aria-pressed={wrapLines()} onClick={() => setWrapLines((value) => !value)}>
              <WrapText size={14} />
            </button>
            <button type="button" title="刷新审阅" disabled={busy()} onClick={() => void refresh()}>
              <RefreshCw size={14} classList={{ spinning: busy() }} />
            </button>
            <button type="button" title="关闭审阅" aria-label="关闭审阅" onClick={props.onClose}><X size={15} /></button>
          </div>
        </header>

        <Show when={props.workspaceRoot.trim()} fallback={<div class="review-empty">请先选择项目工作区</div>}>
          <Show when={status()?.available} fallback={<div class="review-empty">当前工作区不是 Git 仓库</div>}>
            <div class="review-layout">
              <section class="review-diff-surface">
                <div class="review-diff-toolbar">
                  <div>
                    <strong title={selectedPath()}>{selectedPath() || "选择文件查看差异"}</strong>
                    <Show when={diff()?.truncated}><span>Diff 已截断</span></Show>
                  </div>
                  <div class="review-diff-controls">
                    <Show when={selectedFile()?.staged && (selectedFile()?.modified || selectedFile()?.untracked)}>
                      <div class="review-side-toggle">
                        <button type="button" classList={{ active: !stagedDiff() }} onClick={() => void selectFile(selectedFile()!, false)}>工作区</button>
                        <button type="button" classList={{ active: stagedDiff() }} onClick={() => void selectFile(selectedFile()!, true)}>暂存区</button>
                      </div>
                    </Show>
                    <Show when={selectedFile() && !props.readOnly}>
                      <button class="review-stage-selected" type="button" disabled={busy()} onClick={() => void toggleSelectedStage()}>
                        <Show when={stagedDiff()} fallback={<><Plus size={13} />暂存</>}><Minus size={13} />取消暂存</Show>
                      </button>
                    </Show>
                  </div>
                </div>

                <div class="review-diff-scroll" classList={{ wrap: wrapLines() }}>
                  <For each={parsedDiff()} fallback={<div class="review-diff-empty">{selectedPath() ? "没有可显示的差异" : "从右侧选择文件"}</div>}>
                    {(line) => (
                      <div class={`review-diff-line ${line.kind}`}>
                        <span class="review-line-number">{line.oldLine ?? ""}</span>
                        <span class="review-line-number">{line.newLine ?? ""}</span>
                        <span class="review-line-marker">{line.marker}</span>
                        <code>
                          <For each={line.segments ?? [{ text: line.text, changed: false }]}>
                            {(segment) => <span classList={{ changed: segment.changed }}>{segment.text}</span>}
                          </For>
                        </code>
                      </div>
                    )}
                  </For>
                </div>
              </section>

              <aside class="review-file-rail">
                <div class="review-file-summary">
                  <div>
                    <strong>更改的文件</strong>
                    <span>{status()?.files.length ?? 0}</span>
                  </div>
                  <div>
                    <button type="button" title="全部取消暂存" disabled={busy() || props.readOnly || !(status()?.stagedCount)} onClick={() => void mutate(() => unstageGitPaths([]))}><Minus size={13} /></button>
                    <button type="button" title="全部暂存" disabled={busy() || props.readOnly || status()?.clean} onClick={() => void mutate(() => stageGitPaths([]))}><Plus size={13} /></button>
                  </div>
                </div>
                <label class="review-file-filter">
                  <Search size={13} />
                  <input value={filter()} placeholder="筛选文件" spellcheck={false} onInput={(event) => setFilter(event.currentTarget.value)} />
                </label>
                <div class="review-file-list">
                  <For each={filteredFiles()} fallback={<div class="review-clean"><Check size={15} />{status()?.clean ? "工作区干净" : "没有匹配文件"}</div>}>
                    {(file) => (
                      <div class="review-file-row" classList={{ active: file.path === selectedPath(), conflict: file.conflicted }}>
                        <button type="button" class="review-file-select" title={file.path} onClick={() => void selectFile(file)}>
                          <span class={`review-file-code ${file.untracked ? "untracked" : file.conflicted ? "conflict" : file.staged ? "staged" : "modified"}`}>
                            {file.untracked ? "U" : file.conflicted ? "!" : file.staged && !file.modified ? "A" : "M"}
                          </span>
                          <span>
                            <strong>{fileName(file.path)}</strong>
                            <small>{fileDirectory(file.path)}</small>
                          </span>
                        </button>
                        <Show when={!props.readOnly}>
                          <button
                            type="button"
                            class="review-file-stage"
                            title={file.staged && !file.modified ? "取消暂存" : "暂存"}
                            onClick={() => void mutate(() => file.staged && !file.modified ? unstageGitPaths([file.path]) : stageGitPaths([file.path]))}
                          >
                            <Show when={file.staged && !file.modified} fallback={<Plus size={12} />}><Minus size={12} /></Show>
                          </button>
                        </Show>
                      </div>
                    )}
                  </For>
                </div>
              </aside>
            </div>
          </Show>
        </Show>
        <Show when={error()}><div class="review-error" role="alert">{error()}</div></Show>
      </aside>
    </Show>
  );
}

function preferredDiffSide(file: GitFileStatus): boolean {
  return file.staged && !file.modified && !file.untracked;
}

function fileName(path: string): string {
  const parts = path.replace(/\\/g, "/").split("/");
  return parts.pop() || path;
}

function fileDirectory(path: string): string {
  const normalized = path.replace(/\\/g, "/");
  const index = normalized.lastIndexOf("/");
  return index >= 0 ? normalized.slice(0, index) : "项目根目录";
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
