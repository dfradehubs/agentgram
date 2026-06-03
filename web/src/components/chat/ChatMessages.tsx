"use client";

import React from "react";
import { useT } from "@/lib/i18n";
import { getEntityColor } from "@/lib/agent-colors";
import { MarkdownMessage } from "./MarkdownMessage";
import { ToolCallBlock } from "./ToolCallBlock";
import { ChartBlock } from "./ChartBlock";
import { ThinkingBubbles } from "./ThinkingBubbles";
import { AttachmentDisplay } from "./AttachmentDisplay";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  BarChart2,
  Bot,
  Brain,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  Info,
  Loader2,
  RefreshCw,
  Server,
  Sparkles,
  Users,
} from "lucide-react";
import { useState, useCallback, useMemo } from "react";
import type { Agent, Attachment, ChartData, MCPServer, Message, TimelineItem, ToolCall } from "@/lib/types";
import { detectChartData } from "@/lib/chart-detection";
import { patchSessionCharts } from "@/lib/api";

// --- Per-block chart extraction banner ---
const MAX_CHART_DATA_LEN = 50_000;

function BlockChartBanner({
  toolCalls,
  messageContent,
  hasPersistedCharts,
  isStreamLoading,
  sessionId,
  agentId: blockAgentId,
  assistantOffset,
}: {
  toolCalls: ToolCall[];
  messageContent?: string;
  hasPersistedCharts: boolean;
  isStreamLoading: boolean;
  sessionId?: string;
  agentId?: string;
  assistantOffset: number;
}) {
  const [extractedCharts, setExtractedCharts] = useState<ChartData[]>([]);
  const [isExtracting, setIsExtracting] = useState(false);
  const [extractionDone, setExtractionDone] = useState(false);
  const [extractionError, setExtractionError] = useState<string | null>(null);

  const chartableResults = useMemo(() => {
    if (isStreamLoading) return [];
    const results: { toolName: string; result: string }[] = [];
    // Tool call results
    for (const tc of toolCalls) {
      if (tc.isComplete && tc.result && tc.result.length > 30 && tc.result.length <= MAX_CHART_DATA_LEN) {
        results.push({ toolName: tc.toolName, result: tc.result });
      }
    }
    // JSON blocks in assistant message content (```json ... ```)
    if (messageContent) {
      const jsonBlocks = messageContent.match(/```json\s*\n([\s\S]*?)```/g);
      if (jsonBlocks) {
        for (const block of jsonBlocks) {
          const inner = block.replace(/```json\s*\n/, "").replace(/```$/, "").trim();
          if (inner.length > 30 && inner.length <= MAX_CHART_DATA_LEN) {
            results.push({ toolName: "message_content", result: inner });
          }
        }
      }
    }
    return results;
  }, [toolCalls, messageContent, isStreamLoading]);

  const showBanner = !isStreamLoading && chartableResults.length > 0 && !extractionDone && !hasPersistedCharts;

  const handleExtract = useCallback(async () => {
    setIsExtracting(true);
    setExtractionError(null);
    const charts: ChartData[] = [];
    let lastError: string | null = null;
    for (const tr of chartableResults) {
      const heuristic = detectChartData(tr.result);
      if (heuristic) {
        charts.push(heuristic);
        continue;
      }
      try {
        const resp = await fetch("/api/chart/extract", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ data: tr.result }),
        });
        if (resp.status === 501) {
          lastError = "Chart extraction not configured";
          break;
        }
        if (!resp.ok) {
          lastError = "Error extracting chart";
          continue;
        }
        const body = await resp.json();
        if (body.chart) charts.push(body.chart);
      } catch {
        lastError = "Connection error while extracting chart";
      }
    }
    setExtractedCharts(charts);
    if (charts.length === 0 && lastError) {
      setExtractionError(lastError);
    } else if (charts.length === 0) {
      setExtractionError("No chartable data found");
    }
    if (charts.length > 0 && sessionId && blockAgentId) {
      patchSessionCharts(blockAgentId, sessionId, charts, assistantOffset).catch(() => {});
    }
    setIsExtracting(false);
    setExtractionDone(true);
  }, [chartableResults, sessionId, blockAgentId, assistantOffset]);

  if (!showBanner && extractedCharts.length === 0 && !extractionError) return null;

  return (
    <>
      {showBanner && (
        <div className="rounded-lg border bg-muted/30 p-3 flex items-center gap-3">
          <BarChart2 className="h-4 w-4 text-indigo-500 shrink-0" />
          <span className="text-sm text-muted-foreground flex-1">
            There is chartable data in the response.
          </span>
          <button
            onClick={handleExtract}
            disabled={isExtracting}
            className="flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50"
          >
            {isExtracting ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <Sparkles className="h-3 w-3" />
            )}
            Generate chart with AI
          </button>
        </div>
      )}
      {extractionError && (
        <div className="rounded-lg border border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-950/30 p-3 text-sm text-amber-700 dark:text-amber-400">
          {extractionError}
        </div>
      )}
      {extractedCharts.map((chart, i) => (
        <ChartBlock key={`extracted-chart-${i}`} chart={chart} />
      ))}
    </>
  );
}

// --- Reasoning section: wraps intermediate texts + tool calls ---
function ReasoningSection({
  items,
  startIdx,
  expanded,
  isStreaming,
}: {
  items: TimelineItem[];
  startIdx: number;
  expanded: boolean;
  isStreaming: boolean;
}) {
  const [open, setOpen] = useState(false);

  // Build a summary from the last intermediate text message
  const lastTextItem = [...items].reverse().find(
    (it) => it.type === "message" && it.message.role === "assistant" && !it.message.isThinking
  );
  const summary = lastTextItem && lastTextItem.type === "message"
    ? lastTextItem.message.content.trim().split("\n").filter(Boolean).pop()?.slice(0, 120) || "Agent reasoning"
    : "Agent reasoning";

  const toolCount = items.filter(it => it.type === "tool_group").reduce(
    (acc, it) => acc + (it.type === "tool_group" ? it.toolCalls.length : 0), 0
  );

  const renderContent = () => (
    <div className="space-y-2">
      {items.map((item, bIdx) => {
        const globalIdx = startIdx + bIdx;
        if (item.type === "tool_group") {
          return (
            <div key={`tg-${globalIdx}`} className="space-y-2">
              {item.toolCalls.map((tc) => (
                <ToolCallBlock key={tc.toolCallId} toolCall={tc} />
              ))}
            </div>
          );
        }
        if (item.type === "chart") {
          return <ChartBlock key={`chart-${globalIdx}`} chart={item.chart} agentId={item.agentId} />;
        }
        const msg = item.message;
        if (msg.isThinking || (!msg.content?.trim())) return null;
        return (
          <div key={globalIdx} className="text-xs italic text-muted-foreground whitespace-pre-wrap">
            {msg.content}
          </div>
        );
      })}
    </div>
  );

  const headerIcon = isStreaming
    ? <Loader2 className="h-3 w-3 shrink-0 animate-spin" />
    : <Brain className="h-3 w-3 shrink-0" />;

  if (expanded) {
    return (
      <div className="mb-2 rounded-lg border border-border bg-muted/50 px-3 py-2">
        <div className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          {headerIcon}
          <span>Reasoning</span>
          {toolCount > 0 && (
            <span className="text-[10px] opacity-70">· {toolCount} tool{toolCount !== 1 ? "s" : ""}</span>
          )}
        </div>
        <div className="max-h-80 overflow-y-auto">
          {renderContent()}
        </div>
      </div>
    );
  }

  return (
    <div className="mb-2">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
      >
        {isStreaming ? (
          <Loader2 className="h-3 w-3 shrink-0 animate-spin" />
        ) : open ? (
          <ChevronDown className="h-3 w-3 shrink-0" />
        ) : (
          <ChevronRight className="h-3 w-3 shrink-0" />
        )}
        <Brain className="h-3 w-3 shrink-0" />
        <span className="truncate italic">{summary}</span>
        {toolCount > 0 && (
          <span className="text-[10px] opacity-70">· {toolCount} tool{toolCount !== 1 ? "s" : ""}</span>
        )}
      </button>
      {open && (
        <div className="mt-1 rounded-lg border border-border bg-muted/50 px-3 py-2 max-h-80 overflow-y-auto">
          {renderContent()}
        </div>
      )}
    </div>
  );
}

interface ChatMessagesProps {
  // Mode flags
  isMCP: boolean;
  isMCPMulti: boolean;
  isMultiAgent: boolean;
  // Thinking toggle
  showThinking: boolean;
  // Session info (for chart persistence)
  sessionId?: string;
  agentId?: string;
  // Data
  timeline: TimelineItem[];
  messages: Message[];
  isLoading: boolean;
  activeStreamAgentIds: string[];
  // Helpers
  currentAgent: Agent | null;
  currentMCPServer: MCPServer | null;
  mcpServers: MCPServer[];
  multiMCPServerNames: string[];
  selectedTargetAgentIds: string[];
  selectedModelId: string;
  getAgentName: (agentId: string) => string;
  // User info
  user: { email?: string } | null;
  currentUserName: string;
  // Copy
  copiedMessageIdx: number | null;
  onCopyMessage: (idx: number, text: string) => void;
  // Retry
  onRetry: (targetMessage?: Message, targetAgentIds?: string[]) => void;
  // Width
  widthCls: string;
  // Scroll refs
  scrollContainerRef: React.Ref<HTMLDivElement>;
  messagesEndRef: React.RefObject<HTMLDivElement | null>;
  // Pagination
  hasMoreMessages?: boolean;
  isLoadingMore?: boolean;
  onLoadOlder?: () => void;
}

export const ChatMessages = React.memo(function ChatMessages({
  isMCP,
  isMCPMulti,
  isMultiAgent,
  showThinking,
  sessionId,
  agentId,
  timeline,
  messages,
  isLoading,
  activeStreamAgentIds,
  currentAgent,
  currentMCPServer,
  mcpServers,
  multiMCPServerNames,
  selectedTargetAgentIds,
  selectedModelId,
  getAgentName,
  user,
  currentUserName,
  copiedMessageIdx,
  onCopyMessage,
  onRetry,
  widthCls,
  scrollContainerRef,
  messagesEndRef,
  hasMoreMessages,
  isLoadingMore,
  onLoadOlder,
}: ChatMessagesProps) {
  const t = useT();

  const getAgentColor = (aid: string) => getEntityColor(aid);

  // --- Empty timeline ---
  const renderEmptyTimeline = () => {
    if (isMultiAgent) {
      return (
        <div className="flex h-full items-center justify-center p-8">
          <div className="text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
              <Users className="h-6 w-6 text-muted-foreground" />
            </div>
            <p className="text-sm text-muted-foreground">
              {t("chat.emptyMultiAgent")}
            </p>
          </div>
        </div>
      );
    }
    if (isMCP) {
      return (
        <div className="flex h-full items-center justify-center p-8">
          <div className="text-center">
            <Server className="mx-auto mb-4 h-12 w-12 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">
              {isMCPMulti
                ? t("mcp.chatWithMultiTools", { names: multiMCPServerNames.join(", ") })
                : t("mcp.chatWithTools", { name: currentMCPServer!.name })}
            </p>
          </div>
        </div>
      );
    }
    return (
      <div className="flex h-full items-center justify-center p-8">
        <div className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
            <Bot className="h-6 w-6 text-muted-foreground" />
          </div>
          <p className="text-sm text-muted-foreground">
            {t("chat.emptyConversation", { name: currentAgent!.name })}
          </p>
        </div>
      </div>
    );
  };

  // --- User message ---
  const renderUserMessage = (idx: number, msg: { role: string; content: string; attachments?: Attachment[]; user_name?: string; user_email?: string; is_admin?: boolean; broadcast_agent_ids?: string[] }) => {
    const isOwnMessage = msg.user_email
      ? msg.user_email === user?.email
      : !msg.user_name || msg.user_name === currentUserName;

    const targetBadges = isMultiAgent && msg.broadcast_agent_ids && msg.broadcast_agent_ids.length > 0 ? (
      <div className={`flex items-center gap-1 ${isOwnMessage ? "justify-end" : ""}`}>
        <span className="text-[10px] text-muted-foreground">&rarr;</span>
        {msg.broadcast_agent_ids.map((aid) => {
          const c = getAgentColor(aid);
          return (
            <Tooltip key={aid}>
              <TooltipTrigger asChild>
                <div
                  className="flex h-4 w-4 items-center justify-center rounded-full text-white cursor-default"
                  style={{ background: `linear-gradient(135deg, ${c.avatarFrom}, ${c.avatarTo})` }}
                >
                  <Bot className="h-2.5 w-2.5" />
                </div>
              </TooltipTrigger>
              <TooltipContent side="top" className="text-xs">
                {getAgentName(aid)}
              </TooltipContent>
            </Tooltip>
          );
        })}
      </div>
    ) : null;

    if (!isOwnMessage) {
      const otherColor = getEntityColor(msg.user_name || "unknown");
      const initial = (msg.user_name || "?")[0].toUpperCase();
      return (
        <div key={idx} className="group/block flex items-start gap-2.5 max-w-[85%]">
          <div
            className="w-6 h-6 rounded-full flex items-center justify-center text-white text-xs font-bold shrink-0 mt-0.5"
            style={{ background: `linear-gradient(135deg, ${otherColor.avatarFrom}, ${otherColor.avatarTo})` }}
          >
            {initial}
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium" style={{ color: otherColor.avatarFrom }}>
                {msg.user_name}{msg.user_email && ` (${msg.user_email})`}
                {msg.is_admin && (
                  <span className="ml-1.5 inline-flex items-center rounded-full bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-semibold text-amber-600 dark:text-amber-400">
                    Admin
                  </span>
                )}
              </span>
              {targetBadges}
            </div>
            <div className="mt-0.5 rounded-2xl rounded-tl-sm bg-accent px-3 py-2">
              <div className="whitespace-pre-wrap text-sm leading-relaxed">{msg.content}</div>
              {msg.attachments && <AttachmentDisplay attachments={msg.attachments} />}
            </div>
          </div>
        </div>
      );
    }

    return (
      <div key={idx} className="group/block flex justify-end">
        <div className="max-w-[85%]">
          {targetBadges && <div className="mb-0.5">{targetBadges}</div>}
          <div className="rounded-2xl rounded-tr-sm bg-emerald-600/20 dark:bg-emerald-500/15 px-3 py-2">
            <div className="whitespace-pre-wrap text-sm leading-relaxed">{msg.content}</div>
            {msg.attachments && <AttachmentDisplay attachments={msg.attachments} />}
          </div>
          <div className="mt-0.5 flex justify-end items-center gap-2 opacity-0 transition-opacity group-hover/block:opacity-100">
            <button
              onClick={() => onCopyMessage(idx, msg.content)}
              className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
              title={t("chat.copyMessage")}
            >
              {copiedMessageIdx === idx ? (
                <Check className="h-3 w-3 text-emerald-500" />
              ) : (
                <Copy className="h-3 w-3" />
              )}
            </button>
            <button
              onClick={() => {
                if (isMCP) {
                  onRetry(undefined);
                } else {
                  const targetMsg = messages.find((m) => m.role === "user" && m.content === msg.content);
                  if (targetMsg) {
                    const retryTargets = isMultiAgent && selectedTargetAgentIds.length > 0
                      ? selectedTargetAgentIds
                      : undefined;
                    onRetry(targetMsg, retryTargets);
                  }
                }
              }}
              disabled={isLoading}
              className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground disabled:pointer-events-none"
              title={t("chat.regenerateFrom")}
            >
              <RefreshCw className="h-3 w-3" />
            </button>
          </div>
        </div>
      </div>
    );
  };

  // --- Agent timeline ---
  const renderAgentTimeline = () => {
    const isAgentItem = (it: TimelineItem) =>
      it.type === "tool_group" ||
      it.type === "chart" ||
      (it.type === "message" && it.message.role === "assistant");

    type AgentBlock = { agentId: string; startIdx: number; items: TimelineItem[]; stableKey: string };

    // --- Pass 1: collect all agent blocks to compute assistant offsets ---
    const allBlocks: { block: AgentBlock; timelineIdx: number }[] = [];
    const nonBlockItems: { idx: number; item: TimelineItem }[] = [];
    {
      let idx = 0;
      while (idx < timeline.length) {
        const item = timeline[idx];
        if (isAgentItem(item)) {
          const blockAgentId = (item.type === "tool_group" ? item.agentId : item.type === "chart" ? item.agentId : item.message.agent_id) || currentAgent?.id || "";
          // Generate stable key from first item content (survives timeline rebuilds)
          const firstKey = item.type === "tool_group"
            ? item.toolCalls[0]?.toolCallId || `tg-${idx}`
            : item.type === "chart"
            ? `chart-${idx}`
            : item.message.content?.slice(0, 32) || `msg-${idx}`;
          const block: AgentBlock = { agentId: blockAgentId, startIdx: idx, items: [item], stableKey: `${blockAgentId}-${firstKey}` };
          while (idx + 1 < timeline.length) {
            const next = timeline[idx + 1];
            if (!isAgentItem(next)) break;
            const nextAgentId = (next.type === "tool_group" ? next.agentId : next.type === "chart" ? next.agentId : next.message.agent_id) || currentAgent?.id || "";
            if (nextAgentId !== blockAgentId) break;
            block.items.push(next);
            idx++;
          }
          allBlocks.push({ block, timelineIdx: block.startIdx });
        } else {
          nonBlockItems.push({ idx, item });
        }
        idx++;
      }
    }

    // Total assistant blocks → compute offset from end for each
    const totalAssistantBlocks = allBlocks.length;

    // --- Pass 2: render in timeline order ---
    const rendered: React.ReactNode[] = [];
    let blockPointer = 0;
    let nonBlockPointer = 0;

    const renderBlock = (block: AgentBlock, assistantOffset: number) => {
      const color = getAgentColor(block.agentId);

      // Collect all tool calls and message content in this block for chart extraction
      const blockToolCalls: ToolCall[] = [];
      let blockMessageContent = "";
      for (const it of block.items) {
        if (it.type === "tool_group") {
          blockToolCalls.push(...it.toolCalls);
        } else if (it.type === "message" && it.message.role === "assistant" && !it.message.isThinking && it.message.content) {
          blockMessageContent += it.message.content + "\n";
        }
      }
      const blockHasPersistedCharts = block.items.some((it) => it.type === "chart");
      const hasChartableContent = blockToolCalls.length > 0 || blockMessageContent.includes("```json");

      rendered.push(
        <div key={`ab-${block.stableKey}`} data-testid="assistant-message" className="space-y-1.5">
          <div className="flex items-center gap-2">
            <div
              className="w-6 h-6 rounded-full flex items-center justify-center text-white shrink-0"
              style={{ background: `linear-gradient(135deg, ${color.avatarFrom}, ${color.avatarTo})` }}
            >
              <Bot className="w-3 h-3" />
            </div>
            <span className="text-sm font-medium">
              {getAgentName(block.agentId)}
            </span>
          </div>
          <div className="group/block ml-8 space-y-3">
            {(() => {
              // Find the last non-thinking text message index → that's the final response
              let lastTextIdx = -1;
              for (let i = block.items.length - 1; i >= 0; i--) {
                const it = block.items[i];
                if (it.type === "message" && it.message.role === "assistant" && !it.message.isThinking) {
                  lastTextIdx = i;
                  break;
                }
              }

              // Check if there's any tool_group in the block (reasoning exists)
              const hasAnyTool = block.items.some(it => it.type === "tool_group");
              // Check if there are tools AFTER the last text message
              // If so, there's no "final response" yet — everything is reasoning
              const hasToolAfterLastText = lastTextIdx >= 0
                && block.items.slice(lastTextIdx + 1).some(it => it.type === "tool_group");

              // During streaming, check if the block is the last in the timeline
              // (i.e. the agent is actively producing this block)
              const blockEndIdx = block.startIdx + block.items.length - 1;
              const isActivelyStreaming = isLoading && blockEndIdx === timeline.length - 1;

              // During streaming, detect if the last text starts with a markdown header
              // (e.g. "# Title") — strong signal it's the final structured response after tools
              const lastTextContent = lastTextIdx >= 0
                ? (block.items[lastTextIdx] as Extract<TimelineItem, { type: "message" }>).message.content || ""
                : "";
              const looksLikeFinalResponse = /^#{1,3}\s/.test(lastTextContent.trimStart());
              const streamingSplitOut = isActivelyStreaming && hasAnyTool && lastTextIdx > 0
                && !hasToolAfterLastText && looksLikeFinalResponse;

              // Split: reasoning items vs final response
              let reasoningItems: TimelineItem[];
              let finalItems: TimelineItem[];
              if (isActivelyStreaming && !streamingSplitOut) {
                reasoningItems = block.items;
                finalItems = [];
              } else if (hasToolAfterLastText) {
                reasoningItems = block.items;
                finalItems = [];
              } else if (hasAnyTool && lastTextIdx > 0) {
                reasoningItems = block.items.slice(0, lastTextIdx);
                finalItems = block.items.slice(lastTextIdx);
              } else {
                reasoningItems = [];
                finalItems = block.items;
              }

              // Collect intermediate text contents so we can strip them from the final message
              const intermediateTexts = reasoningItems
                .filter((it): it is Extract<TimelineItem, { type: "message" }> =>
                  it.type === "message" && it.message.role === "assistant" && !it.message.isThinking && !!it.message.content?.trim())
                .map(it => it.message.content.trim());

              const nodes: React.ReactNode[] = [];

              // Render reasoning section (intermediate texts + tool calls)
              if (reasoningItems.length > 0) {
                nodes.push(
                  <ReasoningSection
                    key={`reasoning-${block.stableKey}`}
                    items={reasoningItems}
                    startIdx={block.startIdx}
                    expanded={showThinking}
                    isStreaming={isActivelyStreaming}
                  />
                );
              }

              // Render final items (last text message, or all items if no reasoning split)
              for (let bIdx = 0; bIdx < finalItems.length; bIdx++) {
                const blockItem = finalItems[bIdx];
                const originalIdx = reasoningItems.length + bIdx;
                const globalIdx = block.startIdx + originalIdx;

                if (blockItem.type === "tool_group") {
                  nodes.push(
                    <div key={`tg-${globalIdx}`} className="space-y-2">
                      {blockItem.toolCalls.map((tc) => (
                        <ToolCallBlock key={tc.toolCallId} toolCall={tc} />
                      ))}
                    </div>
                  );
                  continue;
                }

                if (blockItem.type === "chart") {
                  nodes.push(
                    <ChartBlock key={`chart-${globalIdx}`} chart={blockItem.chart} agentId={blockItem.agentId} />
                  );
                  continue;
                }

                const msg = blockItem.message;
                const isLastGlobal = globalIdx === timeline.length - 1;

                if (msg.isThinking) {
                  nodes.push(
                    <div key={globalIdx}>
                      <ThinkingBubbles steps={[msg]} isStreaming={isLoading && isLastGlobal} expanded={showThinking} />
                    </div>
                  );
                  continue;
                }

                // Strip intermediate texts from final message content to avoid duplication
                let displayContent = msg.content;
                if (intermediateTexts.length > 0) {
                  let remaining = displayContent;
                  for (const prefix of intermediateTexts) {
                    remaining = remaining.replace(prefix, "");
                  }
                  displayContent = remaining.trim();
                }

                nodes.push(
                  <div key={globalIdx} className="prose-sm text-sm leading-relaxed">
                    <MarkdownMessage content={displayContent} />
                    {isLoading && isLastGlobal && (
                      <span className="ml-0.5 inline-block h-4 w-0.5 animate-pulse bg-foreground/60" />
                    )}
                  </div>
                );
              }

              return nodes;
            })()}
            {(() => {
              const fullText = block.items
                .filter((it): it is Extract<typeof it, { type: "message" }> =>
                  it.type === "message" && it.message.role === "assistant" && !it.message.isThinking && !!it.message.content)
                .map((it) => it.message.content)
                .join("\n\n");
              if (!fullText) return null;
              return (
                <button
                  onClick={() => onCopyMessage(block.startIdx, fullText)}
                  className="mt-1 flex items-center gap-1 text-xs text-muted-foreground opacity-0 transition-opacity hover:text-foreground group-hover/block:opacity-100"
                  title={t("chat.copyMessage")}
                >
                  {copiedMessageIdx === block.startIdx ? (
                    <Check className="h-3 w-3 text-emerald-500" />
                  ) : (
                    <Copy className="h-3 w-3" />
                  )}
                </button>
              );
            })()}
            {/* Per-block chart extraction banner */}
            {hasChartableContent && (
              <BlockChartBanner
                key={`chart-banner-${block.stableKey}`}
                toolCalls={blockToolCalls}
                messageContent={blockMessageContent || undefined}
                hasPersistedCharts={blockHasPersistedCharts}
                isStreamLoading={isLoading}
                sessionId={sessionId}
                agentId={agentId}
                assistantOffset={assistantOffset}
              />
            )}
          </div>
        </div>
      );
    };

    // Merge blocks and non-block items in timeline order
    while (blockPointer < allBlocks.length || nonBlockPointer < nonBlockItems.length) {
      const nextBlockIdx = blockPointer < allBlocks.length ? allBlocks[blockPointer].timelineIdx : Infinity;
      const nextNonBlockIdx = nonBlockPointer < nonBlockItems.length ? nonBlockItems[nonBlockPointer].idx : Infinity;

      if (nextBlockIdx <= nextNonBlockIdx) {
        const { block } = allBlocks[blockPointer];
        const assistantOffset = totalAssistantBlocks - 1 - blockPointer;
        renderBlock(block, assistantOffset);
        blockPointer++;
      } else {
        const { idx, item } = nonBlockItems[nonBlockPointer];
        if (item.type === "message") {
          const msg = item.message;
          if (msg.role === "system") {
            rendered.push(
              <div key={idx} className="group">
                <div className="flex items-center justify-center gap-1.5 py-1">
                  <Info className="h-3 w-3 text-muted-foreground/80" />
                  <span className="text-xs text-muted-foreground/80">{msg.content}</span>
                </div>
              </div>
            );
          } else if (msg.role === "user") {
            rendered.push(renderUserMessage(idx, msg));
          }
        }
        nonBlockPointer++;
      }
    }

    return rendered;
  };

  // --- MCP timeline ---
  const renderMCPTimeline = () => {
    const mcpColor = getEntityColor(currentMCPServer?.id || "mcp");

    // Group consecutive non-user items into blocks so they share a single
    // avatar/header instead of rendering separate bubbles after tool calls.
    type MCPBlock = { startIdx: number; items: { idx: number; item: TimelineItem }[] };
    const blocks: (MCPBlock | { idx: number; item: TimelineItem })[] = [];
    let currentBlock: MCPBlock | null = null;

    for (let i = 0; i < timeline.length; i++) {
      const item = timeline[i];
      const isUser = item.type === "message" && item.message.role === "user";
      if (isUser) {
        if (currentBlock) { blocks.push(currentBlock); currentBlock = null; }
        blocks.push({ idx: i, item });
      } else {
        if (!currentBlock) currentBlock = { startIdx: i, items: [] };
        currentBlock.items.push({ idx: i, item });
      }
    }
    if (currentBlock) blocks.push(currentBlock);

    return blocks.map((entry) => {
      // Single user message
      if ("item" in entry) {
        const { idx, item } = entry;
        if (item.type === "message" && item.message.role === "user") {
          return renderUserMessage(idx, item.message);
        }
        return null;
      }

      // Assistant block: one header, multiple content items
      const block = entry;
      return (
        <div key={`mcp-block-${block.startIdx}`} className="space-y-1.5">
          <div className="flex items-center gap-2">
            <div
              className="w-6 h-6 rounded-full flex items-center justify-center text-white shrink-0"
              style={{ background: `linear-gradient(135deg, ${mcpColor.avatarFrom}, ${mcpColor.avatarTo})` }}
            >
              <Server className="w-3 h-3" />
            </div>
            <span className="text-sm font-medium">{selectedModelId || "LLM"}</span>
          </div>
          <div className="group/block ml-8 space-y-3">
            {block.items.map(({ idx: i, item }) => {
              if (item.type === "message") {
                const msg = item.message;
                const isLastAssistant = isLoading && i === timeline.length - 1 && msg.role === "assistant";
                return (
                  <div key={i}>
                    <div className="prose-sm text-sm leading-relaxed">
                      <MarkdownMessage content={msg.content} />
                      {isLastAssistant && (
                        <span className="ml-0.5 inline-block h-4 w-0.5 animate-pulse bg-foreground/60" />
                      )}
                    </div>
                    {msg.attachments && <AttachmentDisplay attachments={msg.attachments} />}
                  </div>
                );
              }
              if (item.type === "chart") {
                return (
                  <div key={i}>
                    <ChartBlock chart={item.chart} agentId={item.agentId} />
                  </div>
                );
              }
              return (
                <div key={i} className="space-y-2">
                  {item.toolCalls.map((tc) => (
                    <ToolCallBlock
                      key={tc.toolCallId}
                      toolCall={tc}
                      serverLabel={isMCPMulti && tc.serverId ? (mcpServers.find((s) => s.id === tc.serverId)?.name || tc.serverId) : undefined}
                    />
                  ))}
                </div>
              );
            })}
          </div>
        </div>
      );
    });
  };

  // --- Loading indicator ---
  const renderLoadingIndicator = () => {
    const lastTimelineItem = timeline[timeline.length - 1];
    const lastIsAssistantOrTool = lastTimelineItem?.type === "tool_group" ||
      (lastTimelineItem?.type === "message" && lastTimelineItem.message.role === "assistant");

    if (isMultiAgent && activeStreamAgentIds.length > 0) {
      let lastUserIdx = -1;
      for (let i = timeline.length - 1; i >= 0; i--) {
        const item = timeline[i];
        if (item.type === "message" && item.message.role === "user") {
          lastUserIdx = i;
          break;
        }
      }
      const agentsWithContent = new Set<string>();
      for (let i = lastUserIdx + 1; i < timeline.length; i++) {
        const item = timeline[i];
        if (item.type === "message" && item.message.role === "assistant" && item.message.agent_id) {
          agentsWithContent.add(item.message.agent_id);
        } else if (item.type === "tool_group" && item.agentId) {
          agentsWithContent.add(item.agentId);
        } else if (item.type === "chart" && item.agentId) {
          agentsWithContent.add(item.agentId);
        }
      }
      const pendingAgents = activeStreamAgentIds.filter((id) => !agentsWithContent.has(id));
      if (pendingAgents.length === 0) return null;
      return (
        <div className="space-y-3">
          {pendingAgents.map((aid) => {
            const c = getAgentColor(aid);
            return (
              <div key={aid} className="space-y-1.5">
                <div className="flex items-center gap-2">
                  <div
                    className="w-6 h-6 rounded-full flex items-center justify-center text-white shrink-0"
                    style={{ background: `linear-gradient(135deg, ${c.avatarFrom}, ${c.avatarTo})` }}
                  >
                    <Bot className="w-3 h-3" />
                  </div>
                  <span className="text-sm font-medium">{getAgentName(aid)}</span>
                </div>
                <div className="ml-8 flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  {t("common.thinking")}
                </div>
              </div>
            );
          })}
        </div>
      );
    }

    if (lastIsAssistantOrTool) return null;

    if (isMCP) {
      const mcpColor = getEntityColor(currentMCPServer?.id || "mcp");
      return (
        <div className="space-y-1.5">
          <div className="flex items-center gap-2">
            <div
              className="w-6 h-6 rounded-full flex items-center justify-center text-white shrink-0"
              style={{ background: `linear-gradient(135deg, ${mcpColor.avatarFrom}, ${mcpColor.avatarTo})` }}
            >
              <Server className="w-3 h-3" />
            </div>
            <span className="text-sm font-medium">{selectedModelId || "LLM"}</span>
          </div>
          <div className="ml-8 flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            {t("common.thinking")}
          </div>
        </div>
      );
    }

    const singleAgentId = activeStreamAgentIds[0] || currentAgent?.id;
    if (!singleAgentId) return null;
    const c = getAgentColor(singleAgentId);
    return (
      <div className="space-y-1.5">
        <div className="flex items-center gap-2">
          <div
            className="w-6 h-6 rounded-full flex items-center justify-center text-white shrink-0"
            style={{ background: `linear-gradient(135deg, ${c.avatarFrom}, ${c.avatarTo})` }}
          >
            <Bot className="w-3 h-3" />
          </div>
          <span className="text-sm font-medium">
            {getAgentName(singleAgentId)}
          </span>
        </div>
        <div className="ml-8 flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          {t("common.thinking")}
        </div>
      </div>
    );
  };

  // --- Main timeline render ---
  const renderTimeline = () => {
    if (isMCP) {
      return renderMCPTimeline();
    }
    return renderAgentTimeline();
  };

  return (
    <div ref={scrollContainerRef} className="min-h-0 flex-1 overflow-y-auto">
      {timeline.length === 0 ? (
        renderEmptyTimeline()
      ) : (
        <div data-pdf-content className={`mx-auto ${widthCls} px-4 py-6`}>
          <div className="space-y-4">
            {(isLoadingMore || hasMoreMessages) && (
              <div className="flex justify-center py-2">
                {isLoadingMore ? (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    {t("chat.loadingOlder")}
                  </div>
                ) : (
                  <button
                    onClick={onLoadOlder}
                    className="text-xs text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {t("chat.loadOlder")}
                  </button>
                )}
              </div>
            )}
            {renderTimeline()}
            {isLoading && renderLoadingIndicator()}
            <div ref={messagesEndRef} />
          </div>
        </div>
      )}
    </div>
  );
});
