import { describe, expect, test } from "bun:test";

import { hasUsablePartialResult } from "../src/lib/chat-results";

describe("chat UI regressions", () => {
  test("keeps a one-line user message compact", async () => {
    const css = await Bun.file(new URL("../src/styles/chat.css", import.meta.url)).text();
    expect(css).toContain(".op-user-bubble .md-body p { margin: 0; }");
    expect(css).toContain(".op-user-bubble .op-stream { gap: 0;");
  });

  test("does not draw a line inside the rich composer", async () => {
    const css = (await Bun.file(new URL("../src/styles/polish.css", import.meta.url)).text()).replaceAll("\r\n", "\n");
    const ruleStart = css.indexOf(".composer-text-editor,\n.composer-tail-editor");
    expect(ruleStart).toBeGreaterThanOrEqual(0);
    const editorRule = css.slice(ruleStart);
    expect(editorRule).toContain("border-bottom: 0;");
    expect(editorRule).toContain("box-shadow: none;");
    expect(editorRule).toContain("text-decoration: none;");
  });

  test("keeps a URL-only composer aligned from the left", async () => {
    const [app, css] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles.css", import.meta.url)).text(),
    ]);
    expect(app).toContain('"starts-with-link": composerLinks().length > 0 && promptDraft().trim().length === 0');
    expect(css).toContain(".composer-rich-input.starts-with-link .composer-text-editor:empty");
    expect(css).toContain("flex: 0 0 0;");
  });

  test("keeps the link remove action inside the composer chip", async () => {
    const css = await Bun.file(new URL("../src/styles.css", import.meta.url)).text();
    const ruleStart = css.indexOf(".composer-link-remove {\n  position: static;");
    expect(ruleStart).toBeGreaterThanOrEqual(0);
    const rule = css.slice(ruleStart, css.indexOf("}", ruleStart) + 1);
    expect(rule).toContain("position: static;");
    expect(rule).toContain("width: 18px;");
    expect(rule).not.toContain("top: -7px;");
    expect(rule).not.toContain("right: -7px;");
  });

  test("supports composer undo and redo across rich input regions", async () => {
    const app = await Bun.file(new URL("../src/app.tsx", import.meta.url)).text();
    expect(app).toContain("handleComposerHistoryShortcut(event)");
    expect(app).toContain("undoComposerInput");
    expect(app).toContain("redoComposerInput");
    expect(app).toContain('title="撤销输入 (Ctrl+Z)"');
  });

  test("renders generated files as collapsed artifacts with HTML open actions", async () => {
    const messageContent = await Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text();
    expect(messageContent).toContain('<details class="op-file-artifact"');
    expect(messageContent).toContain('runFromSummary(event, "view")');
    expect(messageContent).toContain("在右侧查看文件");
    expect(messageContent).toContain("在内置浏览器中预览");
  });

  test("opens workspace files in a non-Git right-side file panel", async () => {
    const [app, messageContent, reviewPanel] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/ReviewPanel.tsx", import.meta.url)).text(),
    ]);
    expect(app).toContain("onOpenWorkspaceFile={handleOpenWorkspaceFile}");
    expect(app).toContain("fileRequest={workspaceFileRequest()}");
    expect(messageContent).toContain("<EditedFilesList");
    expect(messageContent).toContain('props.item.category === "edit" ? true : undefined');
    expect(messageContent).toContain("data-workspace-path");
    expect(reviewPanel).toContain("readWorkspaceFile(target)");
    expect(reviewPanel).toContain("文件会在这里打开，不需要 Git 仓库。");
    expect(reviewPanel).not.toContain("当前工作区不是 Git 仓库");
  });

  test("uses a read-only CodeMirror viewer and compact read_file links", async () => {
    const [messageContent, reviewPanel, codeViewer] = await Promise.all([
      Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/ReviewPanel.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/CodeViewer.tsx", import.meta.url)).text(),
    ]);
    expect(reviewPanel).toContain("<CodeViewer");
    expect(reviewPanel).not.toContain('class="review-code-line"');
    expect(codeViewer).toContain("basicSetup");
    expect(codeViewer).toContain("LanguageDescription.matchFilename");
    expect(codeViewer).toContain("EditorState.readOnly.of(true)");
    expect(codeViewer).toContain("EditorView.scrollIntoView");
    expect(messageContent).toContain('class="op-read-file"');
    expect(messageContent).toContain('props.onOpenWorkspaceFile?.(reference.path, "file", reference.startLine)');
  });

  test("preserves the embedded browser while switching to a file tab", async () => {
    const [app, host, css] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/SidePanelHost.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles.css", import.meta.url)).text(),
    ]);
    const openFileStart = app.indexOf("const handleOpenWorkspaceFile");
    const openFileEnd = app.indexOf("const handleOpenBrowser", openFileStart);
    const openFileHandler = app.slice(openFileStart, openFileEnd);
    expect(openFileHandler).toContain('setSidePanelView("files")');
    expect(openFileHandler).not.toContain("setBrowserPreview(undefined)");
    expect(app).toContain("<SidePanelHost");
    expect(host).toContain('type SidePanelView = "browser" | "files"');
    expect(host).toContain('suspended={props.browserSuspended || activeView() !== "browser"}');
    expect(host).toContain('onClick={() => props.onSelectView("browser")}');
    expect(host).toContain('onClick={() => props.onSelectView("files")}');
    expect(css).toContain(".side-panel-page.inactive");
    expect(css).toContain("visibility: hidden;");
  });

  test("keeps task progress above the composer instead of inside messages", async () => {
    const [app, messageContent, css] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles.css", import.meta.url)).text(),
    ]);
    expect(messageContent).not.toContain('<Match when={block.kind === "progress"}>');
    expect(messageContent).toContain('if (part.kind === "task_progress") {');
    expect(app).toContain('<section class="task-progress-dock"');
    expect(app).toContain('<TaskProgress part={progress()} />');
    expect(css).toContain(".task-progress-dock > .op-task-progress");
    expect(css).toContain("width: min(760px, 100%);");
    expect(css).toContain("justify-self: center;");
  });

  test("keeps the jump-to-bottom action centered above the plan and composer", async () => {
    const [app, css] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles.css", import.meta.url)).text(),
    ]);
    expect(app).toContain('<div class="chat-jump-bottom-dock">');
    expect(app.indexOf('class="chat-jump-bottom-dock"')).toBeLessThan(app.indexOf('<section class="task-progress-dock"'));

    const dockStart = css.indexOf(".chat-jump-bottom-dock {");
    const dockRule = css.slice(dockStart, css.indexOf("}", dockStart) + 1);
    expect(dockRule).toContain("height: 0;");
    expect(dockRule).toContain("pointer-events: none;");

    const buttonStart = css.indexOf(".chat-jump-bottom {");
    const buttonRule = css.slice(buttonStart, css.indexOf("}", buttonStart) + 1);
    expect(buttonRule).toContain("bottom: 12px;");
    expect(buttonRule).toContain("left: 50%;");
    expect(buttonRule).toContain("transform: translateX(-50%);");
    expect(buttonRule).not.toContain("right: max(");
  });

  test("allows independent background tasks for different sessions", async () => {
    const app = await Bun.file(new URL("../src/app.tsx", import.meta.url)).text();
    expect(app).toContain("startChatMessageForSession(sessionID, prompt, attachments)");
    expect(app).toContain("const isSessionBusy = (sessionID: string)");
    expect(app).toContain("if (isSessionBusy(sessionID))");
    expect(app).toContain("const backgroundTaskCount = createMemo");
    expect(app).toContain("const activeTasks = await getActiveChatTasks()");
    expect(app).not.toContain("if ((!prompt && attachments.length === 0) || sendingMessage())");
  });

  test("treats successful repository reads as usable partial results", () => {
    expect(hasUsablePartialResult([{
      kind: "tool_call",
      name: "read_repository",
      status: "ok",
      input: "https://github.com/MISSmihu/MHcode",
      output: "Repository: MISSmihu/MHcode\nCommit: abc123",
    }])).toBe(true);
    expect(hasUsablePartialResult([{
      kind: "tool_call",
      name: "read_repository",
      status: "error",
      output: "request failed",
    }])).toBe(false);
  });

  test("treats successful webpage reads as usable partial results", () => {
    expect(hasUsablePartialResult([{
      kind: "tool_call",
      name: "read_webpage",
      status: "ok",
      input: "https://example.com",
      output: "Title: Example\n\nPage content:\nActual page text",
    }])).toBe(true);
    expect(hasUsablePartialResult([{
      kind: "tool_call",
      name: "read_webpage",
      status: "error",
      output: "HTTP 403",
    }])).toBe(false);
  });
});
