import { describe, expect, test } from "bun:test";

import { hasUsablePartialResult } from "../src/lib/chat-results";

describe("chat UI regressions", () => {
  test("keeps a one-line user message compact", async () => {
    const css = await Bun.file(new URL("../src/styles/chat.css", import.meta.url)).text();
    expect(css).toContain(".op-user-bubble .md-body p { margin: 0; }");
    expect(css).toContain(".op-user-bubble .op-stream { gap: 0;");
  });

  test("does not draw a line inside the rich composer", async () => {
    const css = await Bun.file(new URL("../src/styles/polish.css", import.meta.url)).text();
    const editorRule = css.slice(css.indexOf(".composer-text-editor,\n.composer-tail-editor"));
    expect(editorRule).toContain("border-bottom: 0;");
    expect(editorRule).toContain("box-shadow: none;");
    expect(editorRule).toContain("text-decoration: none;");
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
});
