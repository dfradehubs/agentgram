import type { ChartData, Message, ToolCall, TimelineItem } from "@/lib/types";
import type { ContentSegment } from "./types";

/**
 * Build a timeline from session messages, reconstructing tool_group items
 * from messages that have tool_calls/tool_results fields.
 * If content_parts is present, uses it to interleave text segments with tool groups.
 */
export function buildTimelineFromMessages(msgs: Message[]): TimelineItem[] {
  const items: TimelineItem[] = [];
  for (const msg of msgs) {
    if (msg.role === "assistant" && msg.tool_calls?.length) {
      // Build ToolCall objects from stored data
      const allToolCalls: ToolCall[] = msg.tool_calls.map((tc, i) => {
        const result = msg.tool_results?.[i];
        return {
          toolCallId: tc.id || `${tc.name}-${i}`,
          toolName: tc.name,
          args: JSON.stringify(tc.args, null, 2),
          result: result ? JSON.stringify(result.response, null, 2) : undefined,
          isComplete: true,
        };
      });

      if (msg.content_parts?.length) {
        // Use content_parts for proper interleaving
        let currentToolGroup: ToolCall[] = [];
        for (const part of msg.content_parts) {
          if (part.type === "text" && part.text) {
            // Flush pending tool group before text
            if (currentToolGroup.length > 0) {
              items.push({ type: "tool_group", toolCalls: currentToolGroup, agentId: msg.agent_id });
              currentToolGroup = [];
            }
            items.push({
              type: "message",
              message: { role: "assistant", content: part.text, agent_id: msg.agent_id },
            });
          } else if (part.type === "tool_use") {
            const tc = allToolCalls[part.tool_index ?? 0];
            if (tc) currentToolGroup.push(tc);
          } else if (part.type === "chart" && part.chart) {
            // Flush pending tool group before chart
            if (currentToolGroup.length > 0) {
              items.push({ type: "tool_group", toolCalls: currentToolGroup, agentId: msg.agent_id });
              currentToolGroup = [];
            }
            items.push({ type: "chart", chart: part.chart, agentId: msg.agent_id });
          }
        }
        // Flush remaining tool group
        if (currentToolGroup.length > 0) {
          items.push({ type: "tool_group", toolCalls: currentToolGroup, agentId: msg.agent_id });
        }
      } else {
        // Legacy fallback: tool_group before final text
        items.push({ type: "tool_group", toolCalls: allToolCalls, agentId: msg.agent_id });
        items.push({ type: "message", message: msg });
      }
    } else {
      items.push({ type: "message", message: msg });
    }
  }
  return items;
}

/**
 * Extract completed tool calls from stream items and attach them to a Message.
 * This ensures tool_calls/tool_results persist in the messages array so that
 * buildTimelineFromMessages can reconstruct tool_group items on subsequent renders.
 */
export function attachToolCalls(msg: Message, streamItems: TimelineItem[]): Message {
  const toolGroups = streamItems.filter(
    (i): i is Extract<TimelineItem, { type: "tool_group" }> => i.type === "tool_group"
  );
  if (toolGroups.length === 0) return msg;

  const allCalls: ToolCall[] = toolGroups.flatMap((g) => g.toolCalls);
  if (allCalls.length === 0) return msg;

  const storedCalls = allCalls.map((tc) => ({
    id: tc.toolCallId,
    name: tc.toolName,
    args: safeParseJSON(tc.args),
  }));

  const storedResults = allCalls.map((tc) => ({
    id: tc.toolCallId,
    name: tc.toolName,
    response: tc.result ? safeParseJSON(tc.result) : {},
  }));

  return { ...msg, tool_calls: storedCalls, tool_results: storedResults };
}

function safeParseJSON(s: string): Record<string, unknown> {
  try { return JSON.parse(s); } catch { return { text: s }; }
}

/**
 * Finalize content segments into a single content string.
 * Prefers regular (non-thinking) content; falls back to the last segment's content.
 */
export function finalizeSegments(segments: ContentSegment[]): string {
  const regularContent = segments
    .filter((s) => !s.isThinking)
    .map((s) => s.content)
    .join("");
  const fallbackContent =
    segments.length > 0 ? segments[segments.length - 1].content : "";
  return regularContent || fallbackContent;
}

/**
 * Build the finalized segment list, including the currently open message if needed.
 */
export function buildFinalizedSegments(
  segments: ContentSegment[],
  hasOpenMessage: boolean,
  currentContent: string,
  currentIsThinking: boolean,
): ContentSegment[] {
  return hasOpenMessage && currentContent.trim()
    ? [...segments, { content: currentContent, isThinking: currentIsThinking }]
    : segments;
}

/**
 * Update stream items with a final message, replacing the last assistant message
 * if it exists, and removing thinking items.
 *
 * When tool_groups are present (interleaved text+tools), builds content_parts on the
 * finalMsg so that buildTimelineFromMessages can reconstruct the interleaving on reload.
 * The stream items are collapsed into a single message item (the finalMsg) because the
 * timeline will be rebuilt from messages on next render/reload.
 */
export function applyFinalMessage(
  streamItems: TimelineItem[],
  finalMsg: Message,
): TimelineItem[] {
  const hasTools = streamItems.some((i) => i.type === "tool_group");

  const hasCharts = streamItems.some((i) => i.type === "chart");

  if ((hasTools && finalMsg.tool_calls?.length) || hasCharts) {
    // Build content_parts from the stream items for proper interleaving on reload
    const contentParts: { type: "text" | "tool_use" | "chart"; text?: string; tool_index?: number; chart?: ChartData }[] = [];
    let toolIndex = 0;
    for (const item of streamItems) {
      if (item.type === "message" && item.message.role === "assistant" && !item.message.isThinking && item.message.content?.trim()) {
        contentParts.push({ type: "text", text: item.message.content });
      } else if (item.type === "tool_group") {
        for (let i = 0; i < item.toolCalls.length; i++) {
          contentParts.push({ type: "tool_use", tool_index: toolIndex++ });
        }
      } else if (item.type === "chart") {
        contentParts.push({ type: "chart", chart: item.chart });
      }
    }
    finalMsg = { ...finalMsg, content_parts: contentParts };
  }

  const lastItem = streamItems[streamItems.length - 1];
  let updated: TimelineItem[];
  if (lastItem?.type === "message" && lastItem.message.role === "assistant" && !lastItem.message.isThinking) {
    updated = [...streamItems.slice(0, -1), { type: "message" as const, message: finalMsg }];
  } else {
    updated = [...streamItems, { type: "message" as const, message: finalMsg }];
  }
  return updated.filter(
    (i) => !(i.type === "message" && i.message.isThinking)
  );
}
