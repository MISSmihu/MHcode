import type { SessionMessage } from "../types";
import type { ChatMessage } from "../ui-types";

export function reconcileSessionMessages(
  current: ChatMessage[],
  history: SessionMessage[],
  preserveCurrentOnEmpty: boolean,
  fallbackTimestamp = Date.now(),
): ChatMessage[] {
  if (history.length === 0) {
    return preserveCurrentOnEmpty ? current : [];
  }

  return history.map((message, index) => ({
    id: message.id || `history-${index}-${fallbackTimestamp}`,
    eventId: message.id || undefined,
    role: normalizeHistoryRole(message.role),
    content: message.content,
    model: message.model,
    parts: message.parts,
    attachments: message.attachments,
    createdAt: message.createdAt || new Date(fallbackTimestamp).toISOString(),
  }));
}

function normalizeHistoryRole(role: string): ChatMessage["role"] {
  return role === "user" || role === "assistant" || role === "system" ? role : "assistant";
}
