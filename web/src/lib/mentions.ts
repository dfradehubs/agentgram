const MENTION_ESCAPE_RE = /[.*+?^${}()|[\]\\]/g;

/**
 * Returns the roster agent IDs @mentioned in the text. Each roster ID is
 * matched literally (so IDs containing "." like "logs.prod" work) as a whole
 * @token: preceded by start/whitespace and terminated so that:
 *  - trailing punctuation is fine — "@logs-agent." matches "logs-agent";
 *  - it isn't a prefix of a longer ID — "@logs.prod" does NOT match "logs".
 * The terminator forbids a following ID char ([\w-]) and a following ".<word>"
 * (which would be a longer dotted ID, not sentence punctuation).
 * Case-SENSITIVE: agent IDs may differ only by case.
 *
 * Shared by the fresh-send and retry paths so both parse mentions identically.
 */
export function extractMentions(text: string, roster: string[]): string[] {
  const found: string[] = [];
  for (const id of roster) {
    const esc = id.replace(MENTION_ESCAPE_RE, "\\$&");
    const re = new RegExp(`(^|\\s)@${esc}(?![\\w-])(?!\\.\\w)`);
    if (re.test(text)) found.push(id);
  }
  return found;
}
