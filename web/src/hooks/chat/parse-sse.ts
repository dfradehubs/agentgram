/**
 * Parsed AG-UI SSE event from the API stream.
 */
export interface AGUIEvent {
  type: string;
  threadId?: string;
  messageId?: string;
  runId?: string;
  delta?: string;
  message?: string;
  toolCallId?: string;
  toolName?: string;
  serverId?: string;
  result?: string;
  isThinking?: boolean;
  [key: string]: unknown;
}

/**
 * Parse SSE lines from a buffer chunk.
 * Returns parsed events and any remaining incomplete buffer.
 */
export function parseSSELines(buffer: string): { events: AGUIEvent[]; remaining: string } {
  const lines = buffer.split("\n");
  const remaining = lines.pop() || "";
  const events: AGUIEvent[] = [];

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || !trimmed.startsWith("data: ")) continue;

    try {
      const event = JSON.parse(trimmed.slice(6)) as AGUIEvent;
      events.push(event);
    } catch (parseErr) {
      if (parseErr instanceof SyntaxError) continue;
      throw parseErr;
    }
  }

  return { events, remaining };
}

/**
 * Read and decode chunks from a ReadableStream, yielding decoded text.
 * Handles the TextDecoder streaming mode.
 */
export function createStreamDecoder(): {
  decode: (chunk: Uint8Array) => string;
} {
  const decoder = new TextDecoder();
  return {
    decode: (chunk: Uint8Array) => decoder.decode(chunk, { stream: true }),
  };
}
