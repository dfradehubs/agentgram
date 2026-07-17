const MENTION_ESCAPE_RE = /[.*+?^${}()|[\]\\]/g;

export type MentionResolutionError = "ambiguous" | "unrecognized";

export interface MentionResolution {
  agentIds: string[];
  error?: MentionResolutionError;
}

export interface MentionOption {
  id: string;
  label: string;
  description?: string;
}

export interface ActiveMention {
  start: number;
  end: number;
  query: string;
}

export interface AppliedMention {
  value: string;
  caret: number;
}

const MENTION_ID_CHAR_RE = /[\w.-]/;

// Finds the @token being edited at the caret. Mentions must start at the
// beginning of the message or after whitespace, so email addresses never open
// the autocomplete. The returned range covers the complete token, including
// any characters after the caret, so selecting a suggestion replaces it once.
export function findActiveMention(text: string, caret: number): ActiveMention | null {
  const safeCaret = Math.max(0, Math.min(caret, text.length));
  const prefix = text.slice(0, safeCaret);
  const match = prefix.match(/(?:^|\s)@([\w.-]*)$/);
  if (!match) return null;

  const query = match[1];
  const start = safeCaret - query.length - 1;
  let end = safeCaret;
  while (end < text.length && MENTION_ID_CHAR_RE.test(text[end])) end++;
  return { start, end, query };
}

export function filterMentionOptions(options: MentionOption[], query: string): MentionOption[] {
  const normalized = query.toLocaleLowerCase();
  if (!normalized) return options;
  return options.filter((option) =>
    option.id.toLocaleLowerCase().includes(normalized)
      || option.label.toLocaleLowerCase().includes(normalized)
  );
}

export function applyMention(text: string, mention: ActiveMention, agentId: string): AppliedMention {
  const suffix = text.slice(mention.end);
  const needsSpace = suffix.length === 0 || /^[\w@]/.test(suffix);
  const replacement = `@${agentId}${needsSpace ? " " : ""}`;
  return {
    value: text.slice(0, mention.start) + replacement + suffix,
    caret: mention.start + replacement.length,
  };
}

/**
 * Returns the roster agent IDs @mentioned in the text. Each roster ID is
 * matched literally (so IDs containing "." like "logs.prod" work) as a whole
 * @token: preceded by start/whitespace and terminated so that:
 *  - trailing punctuation is fine — "@logs-agent." matches "logs-agent";
 *  - it isn't a prefix of a longer ID — "@logs.prod" does NOT match "logs".
 * The terminator forbids a following ID char ([\w-]) and a following ".<word>"
 * (which would be a longer dotted ID, not sentence punctuation).
 *
 * Case-INSENSITIVE: a case typo like "@Logs-Agent" still resolves to
 * "logs-agent" rather than silently matching nothing (which would drop the
 * roster override and broadcast to the whole group). If multiple roster IDs
 * match the same token, resolveMentions rejects the ambiguous selection.
 *
 * Shared by the fresh-send and retry paths so both parse mentions identically.
 */
export function extractMentions(text: string, roster: string[]): string[] {
  return resolveMentions(text, roster).agentIds;
}

/**
 * Resolves every explicit @token independently. An unknown or ambiguous token
 * is an error so callers never turn a typo/case collision into a whole-roster
 * request. No @tokens means the moderator may choose freely.
 */
export function resolveMentions(text: string, roster: string[]): MentionResolution {
  const tokens = [...text.matchAll(/(?:^|\s)@([^\s]+)/g)].map((match) => match[1]);
  if (tokens.length === 0) return { agentIds: [] };

  const agentIds: string[] = [];
  for (const token of tokens) {
    const mention = `@${token}`;
    const matches = roster.filter((id) => {
      const esc = id.replace(MENTION_ESCAPE_RE, "\\$&");
      return new RegExp(`^@${esc}(?![\\w-])(?!\\.\\w)`, "i").test(mention);
    });
    if (matches.length === 0) return { agentIds: [], error: "unrecognized" };
    if (matches.length > 1) return { agentIds: [], error: "ambiguous" };
    if (!agentIds.includes(matches[0])) agentIds.push(matches[0]);
  }
  return { agentIds };
}
