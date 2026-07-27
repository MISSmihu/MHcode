import { describe, expect, test } from "bun:test";

import { appendLiveAssistantText, displayMessageParts, liveTaskStatus, updateLiveTimelineParts } from "../src/lib/timeline";
import type { ChatTaskEvent, MessagePart } from "../src/types";

describe("live task timeline", () => {
  test("keeps provider heartbeats out of the durable activity feed", () => {
    const parts: MessagePart[] = [{
      kind: "timeline_note",
      message: "正在分析任务",
      status: "running",
      startedAt: "2026-07-28T00:00:00Z",
    }];
    const heartbeat: ChatTaskEvent = {
      taskId: "task-1",
      type: "heartbeat",
      status: "waiting",
      message: "上游模型仍在处理（16s 未收到新数据）",
    };

    expect(updateLiveTimelineParts(parts, heartbeat)).toEqual(parts);
    expect(liveTaskStatus(heartbeat.message)).toBe("正在执行任务");
  });

  test("hides routine setup phases while keeping one compact live status", () => {
    const phases = ["正在准备上下文", "正在连接 Anthropic", "正在分析任务", "正在生成执行计划"];
    for (const message of phases) {
      const event: ChatTaskEvent = { taskId: "task-1", type: "status", status: "running", message };
      expect(updateLiveTimelineParts([], event)).toEqual([]);
      expect(liveTaskStatus(message)).toBe("正在执行任务");
    }
  });

  test("persists meaningful phases once", () => {
    const event: ChatTaskEvent = {
      taskId: "task-1",
      type: "status",
      status: "running",
      message: "连接中断，正在重试",
    };
    const first = updateLiveTimelineParts([], event);
    const duplicate = updateLiveTimelineParts(first, event);

    expect(first).toHaveLength(1);
    expect(duplicate).toEqual(first);
  });

  test("keeps streamed progress text in chronological order before an activity", () => {
    const parts: MessagePart[] = [{
      kind: "timeline_note",
      message: "正在分析任务",
      status: "running",
    }];
    const flushed = appendLiveAssistantText(parts, "我先核对目标机上的运行服务和实际配置。");

    expect(flushed).toEqual([
      parts[0],
      { kind: "text", text: "我先核对目标机上的运行服务和实际配置。" },
    ]);
    expect(appendLiveAssistantText(flushed, "我先核对目标机上的运行服务和实际配置。")).toEqual(flushed);
  });

  test("previews the current streamed text after existing activity parts", () => {
    const parts: MessagePart[] = [{ kind: "tool_call", name: "ssh", status: "ok" }];
    expect(displayMessageParts(parts, "已经找到实际配置文件。", true)).toEqual([
      parts[0],
      { kind: "text", text: "已经找到实际配置文件。" },
    ]);
    expect(displayMessageParts(parts, "最终答复", false)).toEqual(parts);
  });
});
