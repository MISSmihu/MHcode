import type { ChatTaskEvent, MessagePart } from "../types";

export function appendLiveAssistantText(
  parts: MessagePart[] | undefined,
  content: string | undefined,
): MessagePart[] {
  const text = content?.trim();
  if (!text) return parts ?? [];

  const next = [...(parts ?? [])];
  const last = next.at(-1);
  if (last?.kind === "text" && last.text.trim() === text) return next;
  next.push({ kind: "text", text });
  return next;
}

export function displayMessageParts(
  parts: MessagePart[] | undefined,
  content: string,
  streaming: boolean | undefined,
): MessagePart[] {
  if (!parts?.length) return [{ kind: "text", text: content }];
  return streaming ? appendLiveAssistantText(parts, content) : parts;
}

export function updateLiveTimelineParts(
  parts: MessagePart[] | undefined,
  event: ChatTaskEvent,
): MessagePart[] {
  if (event.type === "heartbeat" || event.type === "started") return parts ?? [];

  const message = event.message?.trim();
  if (!message || isRoutineTaskStatus(message)) return parts ?? [];
  const status = event.type === "context_compression"
    ? event.compression?.status || event.status || "running"
    : event.status || "running";
  const next = [...(parts ?? [])];
  for (let index = next.length - 1; index >= 0; index--) {
    const part = next[index];
    if (part.kind !== "timeline_note") continue;
    if (part.message === message && part.status === status) return next;
    break;
  }
  next.push({
    kind: "timeline_note",
    message,
    status,
    startedAt: new Date().toISOString(),
  });
  return next;
}

export function isRoutineTaskStatus(message: string | undefined): boolean {
  const normalized = message?.trim() ?? "";
  return normalized === "正在准备上下文"
    || normalized === "正在分析任务"
    || normalized === "正在生成执行计划"
    || normalized.startsWith("正在连接 ")
    || normalized.startsWith("上游模型仍在处理");
}

export function liveTaskStatus(message: string | undefined): string {
  return isRoutineTaskStatus(message) ? "正在执行任务" : message?.trim() || "正在执行任务";
}
