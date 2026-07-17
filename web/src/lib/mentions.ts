const MENTION_ESCAPE_RE = /[.*+?^${}()|[\]\\]/g;

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
 * roster override and broadcast to the whole group). Kebab-case agent IDs don't
 * collide on case in practice; if two ever did, both would match — visible, not
 * a silent widening.
 *
 * Shared by the fresh-send and retry paths so both parse mentions identically.
 */
export function extractMentions(text: string, roster: string[]): string[] {
  const found: string[] = [];
  for (const id of roster) {
    const esc = id.replace(MENTION_ESCAPE_RE, "\\$&");
    const re = new RegExp(`(^|\\s)@${esc}(?![\\w-])(?!\\.\\w)`, "i");
    if (re.test(text)) found.push(id);
  }
  return found;
}
