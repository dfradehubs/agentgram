export type DebateIncompleteReason =
  | "all_agents_failed"
  | "moderator_error"
  | "timeout";

const REASONS = new Set<DebateIncompleteReason>([
  "all_agents_failed",
  "moderator_error",
  "timeout",
]);

export function debateIncompleteReason(event: unknown): DebateIncompleteReason | null {
  if (!event || typeof event !== "object") return null;
  const candidate = event as {
    type?: unknown;
    subType?: unknown;
    data?: { reason?: unknown };
  };
  if (candidate.type !== "CUSTOM" || candidate.subType !== "debate.incomplete") return null;
  const reason = candidate.data?.reason;
  return typeof reason === "string" && REASONS.has(reason as DebateIncompleteReason)
    ? (reason as DebateIncompleteReason)
    : null;
}

export function debateIncompleteMessage(reason: DebateIncompleteReason): string {
  switch (reason) {
    case "all_agents_failed":
      return "All selected agents failed to respond";
    case "timeout":
      return "The group debate timed out before finishing";
    case "moderator_error":
      return "The group debate ended because the moderator failed";
  }
}
