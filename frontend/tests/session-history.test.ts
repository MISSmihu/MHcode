import { describe, expect, test } from "bun:test";

import { reconcileSessionMessages } from "../src/lib/session-history";
import type { ChatMessage } from "../src/ui-types";

const current: ChatMessage[] = [
  {
    id: "local-user",
    role: "user",
    content: "keep this turn",
    createdAt: "2026-07-19T00:00:00.000Z",
  },
];

describe("session history reconciliation", () => {
  test("keeps optimistic messages when a completion sync is temporarily empty", () => {
    expect(reconcileSessionMessages(current, [], true)).toBe(current);
  });

  test("clears messages when an explicit session change has no history", () => {
    expect(reconcileSessionMessages(current, [], false)).toEqual([]);
  });

  test("replaces optimistic messages with persisted history", () => {
    expect(reconcileSessionMessages(current, [
      {
        id: "event-1",
        role: "user",
        content: "persisted turn",
        createdAt: "2026-07-19T01:00:00.000Z",
      },
      {
        id: "event-2",
        role: "unexpected-role",
        content: "persisted reply",
        createdAt: "2026-07-19T01:00:01.000Z",
      },
    ], true)).toMatchObject([
      { id: "event-1", eventId: "event-1", role: "user", content: "persisted turn" },
      { id: "event-2", eventId: "event-2", role: "assistant", content: "persisted reply" },
    ]);
  });
});
