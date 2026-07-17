const MENTION_ESCAPE_RE = /[.*+?^${}()|[\]\\]/g;

/**
 * Returns the roster agent IDs @mentioned in the text. Each roster ID is
 * matched literally (so IDs containing "." like "logs.prod" work) as a whole
 * @token: preceded by start/whitespace and followed by a non-ID char or end,
 * so trailing punctuation ("@logs-agent.") still matches. Case-insensitive.
 *
 * Shared by the fresh-send and retry paths so both parse mentions identically.
 */
export function extractMentions(text: string, roster: string[]): string[] {
  const found: string[] = [];
  for (const id of roster) {
    const esc = id.replace(MENTION_ESCAPE_RE, "\\$&");
    const re = new RegExp(`(^|\\s)@${esc}(?![\\w.-])`, "i");
    if (re.test(text)) found.push(id);
  }
  return found;
}
