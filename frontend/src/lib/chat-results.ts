import type { MessagePart } from "../types";

export function hasUsablePartialResult(parts: MessagePart[] | undefined): boolean {
  return Boolean(parts?.some((part) => (
    part.kind === "web_search_results" && part.sources.length > 0
  ) || (
    part.kind === "tool_call" &&
    (part.name === "read_repository" || part.name === "read_webpage") &&
    part.status !== "error" &&
    Boolean(part.output?.trim())
  )));
}
