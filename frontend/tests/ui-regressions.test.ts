import { describe, expect, test } from "bun:test";

import { hasMeaningfulTurnOutput, hasUsablePartialResult } from "../src/lib/chat-results";

describe("chat UI regressions", () => {
  test("streams, deduplicates, and renders provider safety notices", async () => {
    const [app, messageContent, css, types] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles/chat.css", import.meta.url)).text(),
      Bun.file(new URL("../src/types.ts", import.meta.url)).text(),
    ]);
    expect(app.match(/case "provider_notice"/g)?.length).toBe(2);
    expect(app).toContain("providerNoticeIdentity(part)");
    expect(messageContent).toContain("<ProviderNotice");
    expect(messageContent).toContain("服务端调整了本轮模型");
    expect(css).toContain(".op-provider-notice {");
    expect(css).toContain("color-mix(in srgb, var(--accent)");
    expect(types).toContain('kind: "provider_notice"');
  });

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

	test("keeps URL chips on a separate row below one native editor", async () => {
    const [app, css] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles.css", import.meta.url)).text(),
    ]);
	expect(app).toContain('<textarea');
	expect(app).toContain('class="composer-link-row"');
	expect(app).not.toContain("composerTailEditorRef");
	expect(css).toContain(".composer-link-row {");
	expect(css).toContain(".composer-text-editor::placeholder");
  });

  test("keeps the link remove action inside the composer chip", async () => {
    const css = (await Bun.file(new URL("../src/styles.css", import.meta.url)).text()).replace(/\r\n?/g, "\n");
    const ruleStart = css.indexOf(".composer-link-remove {\n  position: static;");
    expect(ruleStart).toBeGreaterThanOrEqual(0);
    const rule = css.slice(ruleStart, css.indexOf("}", ruleStart) + 1);
    expect(rule).toContain("position: static;");
    expect(rule).toContain("width: 18px;");
    expect(rule).not.toContain("top: -7px;");
    expect(rule).not.toContain("right: -7px;");
  });

	test("uses one native composer while retaining toolbar undo and redo", async () => {
    const app = await Bun.file(new URL("../src/app.tsx", import.meta.url)).text();
	expect(app).toContain("ref={composerEditorRef}");
	expect(app).toContain("event.currentTarget.value");
	expect(app).not.toContain("composerTailEditorRef");
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
    expect(messageContent).toContain("<FileChangesSummary");
    expect(messageContent).toContain('onOpenFile={props.onOpenWorkspaceFile}');
    expect(messageContent).toContain("data-workspace-path");
    expect(reviewPanel).toContain("readWorkspaceFile(target)");
    expect(reviewPanel).toContain("文件会在这里打开，不需要 Git 仓库。");
    expect(reviewPanel).not.toContain("当前工作区不是 Git 仓库");
  });

  test("uses a read-only CodeMirror viewer and compact read_file links", async () => {
    const [messageContent, reviewPanel, codeViewer, inlineCode, css, polish] = await Promise.all([
      Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/ReviewPanel.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/CodeViewer.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/chat/InlineCodePreview.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles/chat.css", import.meta.url)).text(),
      Bun.file(new URL("../src/styles/polish.css", import.meta.url)).text(),
    ]);
    expect(reviewPanel).toContain("<CodeViewer");
    expect(reviewPanel).not.toContain('class="review-code-line"');
    expect(codeViewer).toContain("basicSetup");
    expect(codeViewer).toContain("LanguageDescription.matchFilename");
    expect(codeViewer).toContain("EditorState.readOnly.of(true)");
    expect(codeViewer).toContain("EditorView.scrollIntoView");
    expect(messageContent).toContain('class="op-read-file"');
    expect(messageContent).toContain('props.onOpenWorkspaceFile?.(reference.path, "file", reference.startLine)');
    expect(messageContent).toContain("<InlineCodePreview");
    expect(inlineCode).toContain("parseInlineCode(props.content, props.startLine)");
    expect(inlineCode).toContain("highlightCodeBlock(code(), codeLanguageForPath(props.path))");
    expect(inlineCode).toContain('class="op-inline-code-gutter"');
    expect(inlineCode).toContain('title="在右侧代码查看器中打开"');
    expect(css).toContain(".op-inline-code-scroll");
    expect(css).toContain("contain: inline-size;");
    expect(css).toContain("scrollbar-gutter: stable;");
    expect(polish).toContain("overflow-x: hidden;");
  });

  test("shows edit diffs directly in the conversation activity", async () => {
    const [messageContent, inlineDiff] = await Promise.all([
      Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/chat/InlineDiffPreview.tsx", import.meta.url)).text(),
    ]);
    expect(messageContent).toContain('<Match when={part.kind === "diff"}>');
    expect(messageContent).not.toContain('part.kind === "diff" && props.item.category !== "edit"');
    expect(messageContent).toContain("<InlineDiffPreview");
    expect(inlineDiff).toContain("parseInlineDiff(props.patch)");
    expect(inlineDiff).toContain('title={copied() ? "已复制" : "复制修改"}');
  });

  test("shows a compact file-change summary at the end of a completed response", async () => {
    const [app, messageContent, css] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles/chat.css", import.meta.url)).text(),
    ]);
    expect(messageContent.indexOf("<FileChangesSummary")).toBeGreaterThan(messageContent.indexOf("<For each={blocks()}"));
    expect(messageContent).toContain("已编辑 {props.files.length} 个文件");
    expect(messageContent).toContain("再显示 ${hiddenCount()} 个文件");
    expect(messageContent).toContain('title="撤销本轮文件修改"');
    expect(messageContent).toContain('title="在右侧审阅修改"');
    expect(app).toContain("const handleUndoMessageChanges = async");
    expect(app).toContain("setState(await forkFromMessage(sourceMessage.eventId, activeProjectID(), activeSessionID()))");
    expect(app).toContain("hideFileChangesSummary={message.streaming}");
    expect(css).toContain(".op-file-changes-head");
  });

  test("collapses tool output, markdown code, read_file code, and diffs by default", async () => {
    const [messageContent, markdown, inlineCode, inlineDiff] = await Promise.all([
      Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/lib/markdown.ts", import.meta.url)).text(),
      Bun.file(new URL("../src/components/chat/InlineCodePreview.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/chat/InlineDiffPreview.tsx", import.meta.url)).text(),
    ]);
    expect(messageContent).not.toContain('open={status() === "error"');
    expect(markdown).toContain('`<details class="code-block"');
    expect(inlineCode).toContain("const [expanded, setExpanded] = createSignal(false)");
    expect(inlineDiff).toContain("const [expanded, setExpanded] = createSignal(false)");
    expect(inlineDiff).toContain('innerHTML={highlightCode(row.content || " ", language())');
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

  test("keeps plan and AI team status in separate centered panels above the composer", async () => {
    const [app, messageContent, reasoningMenu, css] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/ReasoningMenu.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles.css", import.meta.url)).text(),
    ]);
    expect(messageContent).not.toContain('<Match when={block.kind === "progress"}>');
    expect(messageContent).toContain('if (part.kind === "task_progress") {');
    expect(app).toContain('class="execution-status-dock"');
    expect(app).toContain('classList={{ combined: Boolean(activeTaskProgress()) && activeTeamParts().length > 0 }}');
    expect(app).toContain('<TeamRun parts={activeTeamParts()} docked />');
    expect(messageContent).toContain("hideTeamRun?: boolean;");
    expect(css).toContain(".execution-status-dock.combined");
    expect(css).toContain("grid-template-columns: minmax(260px, 420px) minmax(340px, 520px);");
    expect(messageContent).toContain('<details class="op-team-run docked"');
    expect(messageContent).toContain('class="op-team-stage-track"');
    expect(reasoningMenu).toContain("<Portal>");
  });

  test("keeps overlays out of clipped layout and hides side panels behind settings", async () => {
    const [app, reasoningMenu, css] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/ReasoningMenu.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles.css", import.meta.url)).text(),
    ]);
    expect(reasoningMenu).toContain('triggerRef.getBoundingClientRect()');
    expect(reasoningMenu).toContain('data-placement={position().placement}');
    expect(reasoningMenu).toContain('document.addEventListener("scroll", reposition, true)');
    expect(css).toContain("position: fixed;");
    expect(app).toContain('<Show when={!drawerOpen() && (Boolean(browserPreview()) || reviewOpen())}>');
    expect(app).toContain('"side-panel-open": !drawerOpen() && (Boolean(browserPreview()) || reviewOpen())');
  });

  test("keeps the jump-to-bottom action centered above the plan and composer", async () => {
    const [app, css] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles.css", import.meta.url)).text(),
    ]);
    expect(app).toContain('<div class="chat-jump-bottom-dock">');
    expect(app.indexOf('class="chat-jump-bottom-dock"')).toBeLessThan(app.indexOf('class="execution-status-dock"'));

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

  test("uses a centered theme-aware app dialog instead of native confirmations", async () => {
    const [app, settings, dialog, polish] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/settings-panels.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/components/ConfirmDialog.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles/polish.css", import.meta.url)).text(),
    ]);
    expect(`${app}\n${settings}`).not.toContain("window.confirm");
    expect(app).toContain("<ConfirmDialog request={confirmation()} onResolve={resolveConfirmation} />");
    expect(settings).toContain("confirmAction: (request: ConfirmationRequest) => Promise<boolean>");
    expect(dialog).toContain("<Portal>");
    expect(dialog).toContain('class="app-confirm-overlay"');

    const overlayStart = polish.indexOf(".app-confirm-overlay {");
    const overlayRule = polish.slice(overlayStart, polish.indexOf("}", overlayStart) + 1);
    expect(overlayRule).toContain("position: fixed;");
    expect(overlayRule).toContain("inset: 0;");
    expect(overlayRule).toContain("place-items: center;");
    expect(polish).toContain(':root[data-theme="light"] .app-confirm-overlay');
    expect(polish).toContain("background: var(--bg-card);");
  });

  test("provides real updater and persistent Agent automation settings", async () => {
    const [panels, services, constants, types, css] = await Promise.all([
      Bun.file(new URL("../src/settings-panels.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/services/workbench.ts", import.meta.url)).text(),
      Bun.file(new URL("../src/constants.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/types.ts", import.meta.url)).text(),
      Bun.file(new URL("../src/styles.css", import.meta.url)).text(),
    ]);
    expect(constants).toContain('{ id: "automation", label: "自动化任务"');
    expect(constants).toContain('{ id: "about", label: "关于 MHcode"');
    expect(panels).toContain("<AutomationSettingsPanel");
    expect(panels).toContain("runAutomationTaskNow");
    expect(panels).toContain("stopAutomationTask");
    expect(panels).toContain("saveAutomationTask");
    expect(panels).toContain("<AboutSettingsPanel");
    expect(panels).toContain("checkForUpdates");
    expect(panels).toContain("installUpdate");
    expect(services).toContain('runtime.EventsOn("automation:state"');
    expect(services).toContain('runtime.EventsOn("update:state"');
    expect(types).toContain("export type AutomationTask");
    expect(types).toContain("export type UpdateState");
    expect(css).toContain(".automation-settings-page");
    expect(css).toContain(".about-download-progress");
  });

  test("switches to the backend replacement after deleting the exact active conversation", async () => {
    const app = await Bun.file(new URL("../src/app.tsx", import.meta.url)).text();
    const deleteStart = app.indexOf("const handleDeleteSession");
    const deleteEnd = app.indexOf("const beginEditMessage", deleteStart);
    const handler = app.slice(deleteStart, deleteEnd);
    expect(handler).toContain("projectID === currentProjectID && session.id === currentSessionID");
    expect(handler).not.toContain("session.isActive");
    expect(handler).toContain("reloadAfterSessionChange(nextState, false)");
    expect(handler).toContain("nextState.activeProjectId,");
    expect(handler).toContain("nextState.activeSessionId,");
    expect(handler).toContain("delete next[sessionKey]");
  });

  test("provides project-scoped conversation actions from the context menu", async () => {
    const [app, services] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/services/workbench.ts", import.meta.url)).text(),
    ]);
    expect(app).toContain("onContextMenu={(event) => openSessionMenu(event, project, session)}");
    expect(app).toContain('class="project-action-menu session-action-menu"');
    expect(app).toContain("重命名");
    expect(app).toContain("打开项目目录");
    expect(app).toContain("复制工作目录");
    expect(app).toContain("复制会话 ID");
    expect(app).toContain("永久删除");
    expect(app).toContain("renameSession(dialog.project.id, dialog.session.id, title)");
    expect(services).toContain("binding.RenameProjectSession(projectID, sessionID, title)");
    expect(services).toContain("binding.ArchiveProjectSession(projectID, sessionID, archived)");
    expect(services).toContain("binding.DeleteProjectSession(projectID, sessionID)");
  });

  test("renders a workspace-focused empty view without a provider-specific link", async () => {
    const [app, css] = await Promise.all([
      Bun.file(new URL("../src/app.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles.css", import.meta.url)).text(),
    ]);
    expect(app).not.toContain("连接 DeepSeek");
    expect(app).toContain('class="welcome-brand"');
    expect(app).toContain('class="welcome-prompt-grid"');
    expect(app).toContain("primeWelcomePrompt");
    expect(app).toContain("梳理当前项目");
    expect(app).toContain("检查当前修改");
    expect(css).toContain(".welcome-prompt-grid");
    expect(css).toContain("grid-template-columns: repeat(2, minmax(0, 1fr));");
  });

  test("allows independent background tasks for different sessions", async () => {
    const app = await Bun.file(new URL("../src/app.tsx", import.meta.url)).text();
    expect(app).toContain("startChatMessageForSession(projectID, sessionID, prompt, attachments)");
    expect(app).toContain("const isSessionBusy = (projectID: string, sessionID: string)");
    expect(app).toContain("if (isSessionBusy(projectID, sessionID))");
    expect(app).toContain("sessionIdentityKey(projectID, sessionID)");
    expect(app).toContain("const backgroundTaskCount = createMemo");
    expect(app).toContain("const activeTasks = await getActiveChatTasks()");
    expect(app).not.toContain("if ((!prompt && attachments.length === 0) || sendingMessage())");
  });

  test("restores conversation UI by project and session identity", async () => {
    const app = await Bun.file(new URL("../src/app.tsx", import.meta.url)).text();
    expect(app).toContain("const [sessionViewStates, setSessionViewStates]");
    expect(app).toContain("rememberCurrentSessionView();");
    expect(app).toContain("restoreSessionView(nextState.activeProjectId, nextState.activeSessionId)");
    expect(app).toContain("reduceBackgroundTaskEvent(eventProjectID, eventSessionID, event)");
    expect(app).toContain("browserPreview: browserPreview()");
    expect(app).toContain("workspaceFileRequest: workspaceFileRequest()");
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

  test("only restores the composer when a stopped turn produced no meaningful output", async () => {
    const app = await Bun.file(new URL("../src/app.tsx", import.meta.url)).text();
    expect(app).toContain('case "cancelled": {');
    expect(app).toContain("hasMeaningfulTurnOutput(result, eventTask?.assistantMessage)");
    expect(app).toMatch(/if \(!retainedTurn\) \{\s*rollbackOptimisticTurn\(eventProjectID, eventSessionID\);/);
    expect(app).toMatch(/if \(accepted\) \{[\s\S]{0,220}Do not erase the turn here\.[\s\S]{0,80}return;/);
    expect(app).toMatch(/case "cancelled": \{[\s\S]{0,1600}cancelled: true/);
  });

  test("recognizes partial text, reasoning, and terminal activity as retained output", () => {
    expect(hasMeaningfulTurnOutput({ content: "partial", turnCommitted: true })).toBe(true);
    expect(hasMeaningfulTurnOutput({ content: "", turnCommitted: false }, { content: "", reasoning: "started", parts: [] })).toBe(false);
    expect(hasMeaningfulTurnOutput({ content: "", turnCommitted: false }, {
      content: "",
      parts: [{ kind: "tool_call", name: "terminal", status: "ok", input: "Get-ChildItem" }],
    })).toBe(false);
    expect(hasMeaningfulTurnOutput({ content: "", turnCommitted: false }, {
      content: "",
      parts: [{ kind: "provider_notice", noticeKind: "policy_error", message: "blocked" }],
    })).toBe(false);
  });

  test("groups shell, terminal, and SSH actions into nested command rows", async () => {
    const [messageContent, css] = await Promise.all([
      Bun.file(new URL("../src/components/chat/MessageContent.tsx", import.meta.url)).text(),
      Bun.file(new URL("../src/styles/chat.css", import.meta.url)).text(),
    ]);
    expect(messageContent).toContain('<CommandActivityList parts={props.item.parts} />');
    expect(messageContent).toContain('class="op-command-entry"');
    expect(messageContent).toContain('case "ssh": return "command";');
    expect(messageContent).toContain('props.part.name === "run_command" || props.part.name === "terminal" || props.part.name === "ssh"');
    expect(css).toContain(".op-command-list {");
    expect(css).toContain(".op-command-entry > summary {");
    expect(css).toContain(".op-command-entry[open] > summary > svg {");
  });
});
