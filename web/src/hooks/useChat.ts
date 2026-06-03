"use client";

import { useState, useCallback, useRef, useEffect } from "react";
import type { Attachment, ChartData, Message, ToolCall, TimelineItem } from "@/lib/types";

function isValidChartData(data: unknown): data is ChartData {
  if (!data || typeof data !== "object") return false;
  const d = data as Record<string, unknown>;
  return (
    typeof d.chartType === "string" &&
    ["bar", "line", "pie", "area"].includes(d.chartType) &&
    Array.isArray(d.labels) &&
    Array.isArray(d.datasets) &&
    d.datasets.length > 0
  );
}
import { getChatEndpoint, getMCPChatEndpoint, getMultiMCPChatEndpoint, getRunStreamUrl } from "@/lib/api";
import { useBackgroundStreamContext } from "@/contexts/BackgroundStreamContext";
import { reportMetric } from "@/lib/telemetry";

import type {
  UseChatOptions,
  UseChatReturn,
  SSEStreamParams,
  AgentStreamParams,
  ContentSegment,
} from "./chat/types";
import { parseSSELines, createStreamDecoder } from "./chat/parse-sse";
import {
  buildTimelineFromMessages,
  finalizeSegments,
  buildFinalizedSegments,
  applyFinalMessage,
  attachToolCalls,
} from "./chat/message-builder";

// AgentError represents an error from the agent itself (RUN_ERROR).
// These should never be auto-retried — they must be shown to the user.
class AgentError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "AgentError";
  }
}

// GitHubAuthError represents a 403 github_token_required error.
// The UI should show a "Connect GitHub" button instead of a generic error.
export class GitHubAuthError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "GitHubAuthError";
  }
}

export class MCPOAuth2Error extends Error {
  constructor(message: string) {
    super(message);
    this.name = "MCPOAuth2Error";
  }
}

function isRetryableError(err: Error): boolean {
  if (err.name === "AbortError") return false;
  if (err instanceof AgentError) return false;
  if (err instanceof GitHubAuthError) return false;
  if (err instanceof MCPOAuth2Error) return false;
  const msg = err.message.toLowerCase();
  if (/status 4\d\d/.test(msg)) return false;
  if (msg.includes("unauthorized") || msg.includes("access denied")) return false;
  return true;
}

export function useChat({
  agentId,
  agentName = "",
  sessionName = "",
  sessionId,
  sessionResetKey = 0,
  initialMessages = [],
  onSessionCreated,
  chatEndpoint: chatEndpointOverride,
  mcpConfig,
  groupId,
  userName,
}: UseChatOptions): UseChatReturn {
  const [messages, setMessages] = useState<Message[]>(initialMessages);
  // Latest messages, readable inside callbacks without adding a dependency.
  const messagesStateRef = useRef(messages);
  messagesStateRef.current = messages;
  const [timeline, setTimeline] = useState<TimelineItem[]>(
    buildTimelineFromMessages(initialMessages)
  );
  const [input, setInput] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [errorType, setErrorType] = useState<"github_auth" | "mcp_oauth2" | null>(null);
  const [activeStreamAgentIds, setActiveStreamAgentIds] = useState<string[]>([]);
  const [isReconnecting, setIsReconnecting] = useState(false);
  const retryCountRef = useRef(0);
  const abortRef = useRef<AbortController | null>(null);
  const readerRef = useRef<ReadableStreamDefaultReader<Uint8Array> | null>(null);
  // Track active session ID across messages (set immediately on RUN_STARTED)
  const activeSessionIdRef = useRef<string | undefined>(sessionId);
  // Generation counter to invalidate stale streaming callbacks after agent/session switch
  const requestGenRef = useRef(0);
  // Sync isLoading to a ref so cleanup effect can read the latest value
  const isLoadingRef = useRef(false);
  isLoadingRef.current = isLoading;
  // Capture the agent/session that started the current stream (for transfer to background)
  const streamAgentIdRef = useRef(agentId);
  const streamAgentNameRef = useRef(agentName);
  const streamSessionNameRef = useRef(sessionName);
  // Capture current messages at stream-start time for background transfer
  const streamMessagesRef = useRef<Message[]>([]);
  // Parallel stream controllers for stop
  const parallelControllersRef = useRef<Map<string, AbortController>>(new Map());

  const { transferStream, reclaimStream, getBaseMessages, stopStream } = useBackgroundStreamContext();
  const mcpServerIdsKey = mcpConfig?.serverIds.join(",") ?? "";

  // Extracted SSE stream processing for single-agent chat.
  // Called from both sendMessage and resume logic.
  const processSSEStream = useCallback((params: SSEStreamParams) => {
    const { reader, gen, allMessages, baseTimeline, effectiveAgentId, suppressSessionCreated } = params;

    const streamDecoder = createStreamDecoder();
    let buffer = "";
    let currentContent = "";
    let currentIsThinking = false;
    let hasOpenMessage = false;
    const segments: ContentSegment[] = [];
    let streamItems: TimelineItem[] = [];
    const streamStartMs = Date.now();
    let ttfbReported = false;

    const isStale = () => requestGenRef.current !== gen;
    let rafId: number | null = null;
    let pendingUpdate = false;
    const scheduleUpdate = () => {
      if (isStale()) return;
      pendingUpdate = true;
      if (rafId === null) {
        rafId = requestAnimationFrame(() => {
          rafId = null;
          if (pendingUpdate && !isStale()) {
            pendingUpdate = false;
            setTimeline([...baseTimeline, ...streamItems]);
          }
        });
      }
    };
    const flushUpdate = () => {
      if (rafId !== null) cancelAnimationFrame(rafId);
      rafId = null;
      if (!isStale()) setTimeline([...baseTimeline, ...streamItems]);
    };

    const processLoop = async () => {
      let runFinished = false;

      while (true) {
        if (isStale()) break;
        const { done, value } = await reader.read();
        if (done || isStale()) break;

        buffer += streamDecoder.decode(value);
        const { events, remaining } = parseSSELines(buffer);
        buffer = remaining;

        for (const event of events) {
          if (isStale()) break;

          switch (event.type) {
            case "RUN_STARTED":
              if (event.threadId) {
                activeSessionIdRef.current = event.threadId;
                if (!suppressSessionCreated) {
                  onSessionCreated?.(event.threadId, event.sessionName as string | undefined);
                }
              }
              break;

            case "TEXT_MESSAGE_START":
              currentContent = "";
              currentIsThinking = event.isThinking === true;
              hasOpenMessage = true;
              break;

            case "TEXT_MESSAGE_CONTENT": {
              if (!ttfbReported && !currentIsThinking) {
                ttfbReported = true;
                reportMetric({ name: "ttfb", labels: { agent_id: effectiveAgentId }, value: (Date.now() - streamStartMs) / 1000 });
              }
              currentContent += event.delta;
              const latestThinking = [...segments].reverse().find(s => s.isThinking);

              setMessages([
                ...allMessages,
                ...(latestThinking && !currentIsThinking ? [{
                  role: "assistant" as const,
                  content: latestThinking.content,
                  isThinking: true,
                  agent_id: effectiveAgentId,
                }] : []),
                {
                  role: "assistant" as const,
                  content: currentContent,
                  isThinking: currentIsThinking,
                  agent_id: effectiveAgentId,
                },
              ]);

              const msgItem: TimelineItem = {
                type: "message",
                message: {
                  role: "assistant",
                  content: currentContent,
                  isThinking: currentIsThinking,
                  agent_id: effectiveAgentId,
                },
              };
              const lastStream = streamItems[streamItems.length - 1];
              if (lastStream?.type === "message" && lastStream.message.role === "assistant") {
                streamItems = [...streamItems.slice(0, -1), msgItem];
              } else {
                streamItems = [...streamItems, msgItem];
              }
              scheduleUpdate();
              break;
            }

            case "TEXT_MESSAGE_END":
              if (currentContent.trim()) {
                segments.push({ content: currentContent, isThinking: currentIsThinking });
              }
              hasOpenMessage = false;
              break;

            case "TOOL_CALL_START": {
              const newTc: ToolCall = {
                toolCallId: event.toolCallId!,
                toolName: event.toolName!,
                args: "",
                isComplete: false,
                ...(event.serverId ? { serverId: event.serverId } : {}),
              };
              const lastItem = streamItems[streamItems.length - 1];
              if (lastItem?.type === "tool_group") {
                streamItems = [...streamItems.slice(0, -1), { type: "tool_group", toolCalls: [...lastItem.toolCalls, newTc], agentId: effectiveAgentId }];
              } else {
                streamItems = [...streamItems, { type: "tool_group", toolCalls: [newTc], agentId: effectiveAgentId }];
              }
              flushUpdate();
              break;
            }

            case "TOOL_CALL_ARGS": {
              streamItems = streamItems.map((item) => {
                if (item.type === "tool_group") {
                  return {
                    ...item,
                    toolCalls: item.toolCalls.map((tc) =>
                      tc.toolCallId === event.toolCallId ? { ...tc, args: tc.args + event.delta } : tc
                    ),
                  };
                }
                return item;
              });
              scheduleUpdate();
              break;
            }

            case "TOOL_CALL_END": {
              streamItems = streamItems.map((item) => {
                if (item.type === "tool_group") {
                  return {
                    ...item,
                    toolCalls: item.toolCalls.map((tc) =>
                      tc.toolCallId === event.toolCallId ? { ...tc, result: event.result, isComplete: true } : tc
                    ),
                  };
                }
                return item;
              });
              flushUpdate();
              break;
            }

            case "CUSTOM": {
              if (event.subType === "CHART" && isValidChartData(event.data)) {
                streamItems = [...streamItems, {
                  type: "chart" as const,
                  chart: event.data,
                  agentId: effectiveAgentId,
                }];
                flushUpdate();
              }
              break;
            }

            case "RUN_FINISHED": {
              runFinished = true;
              retryCountRef.current = 0;
              const finalized = buildFinalizedSegments(segments, hasOpenMessage, currentContent, currentIsThinking);
              const finalContent = finalizeSegments(finalized);

              if (finalContent) {
                const finalMsg: Message = attachToolCalls(
                  { role: "assistant" as const, content: finalContent, agent_id: effectiveAgentId },
                  streamItems,
                );
                setMessages([...allMessages, finalMsg]);
                streamItems = applyFinalMessage(streamItems, finalMsg);
                flushUpdate();
              }
              break;
            }

            case "RUN_ERROR":
              throw new AgentError(event.message || "Agent error");
          }
        }
      }

      // Fallback if stream ended without RUN_FINISHED
      const finalized = buildFinalizedSegments(segments, hasOpenMessage, currentContent, currentIsThinking);
      if (finalized.length > 0) {
        const finalContent = finalizeSegments(finalized);
        const finalMsg: Message = attachToolCalls(
          { role: "assistant" as const, content: finalContent, agent_id: effectiveAgentId },
          streamItems,
        );
        setMessages([...allMessages, finalMsg]);
        streamItems = applyFinalMessage(streamItems, finalMsg);
        flushUpdate();
      }

      // Stream ended without RUN_FINISHED and not stale — interrupted
      if (!runFinished && !isStale()) {
        throw new Error("Stream interrupted");
      }
    };

    return processLoop();
  }, [onSessionCreated]);

  // Reset messages when agent or session changes — transfer active stream to background
  useEffect(() => {
    // Skip reset when sessionId is being set to what the current stream already uses
    // (e.g. markSessionActive sets currentSession after RUN_STARTED provides threadId)
    if (sessionId && sessionId === activeSessionIdRef.current && isLoadingRef.current) {
      return;
    }

    if (
      abortRef.current &&
      readerRef.current &&
      activeSessionIdRef.current &&
      isLoadingRef.current &&
      // Only MCP streams are kept alive in the browser via the background stream.
      // Single-agent runs are buffered server-side (Redis stream) and resumed on
      // return via reconnectToRun, which replays the whole run cleanly — reusing
      // the partial background reader here corrupted the message (showed only the
      // tail and mislabelled it as "thinking").
      mcpConfig
    ) {
      transferStream({
        sessionId: activeSessionIdRef.current,
        sessionName: streamSessionNameRef.current,
        agentId: streamAgentIdRef.current,
        agentName: streamAgentNameRef.current,
        controller: abortRef.current,
        reader: readerRef.current,
        baseMessages: streamMessagesRef.current,
        streamType: "mcp",
        isMultiAgent: mcpConfig.serverIds.length > 1,
      });
      reportMetric({ name: "background_transfer", labels: { agent_id: streamAgentIdRef.current }, value: 1 });
    } else {
      // Abort the fetch; the server keeps the run alive (drain mode) and buffers
      // it, so returning to the session reconnects live via reconnectToRun.
      abortRef.current?.abort();
    }
    // Also abort any parallel streams
    for (const controller of parallelControllersRef.current.values()) {
      controller.abort();
    }
    parallelControllersRef.current.clear();
    abortRef.current = null;
    readerRef.current = null;
    requestGenRef.current++;
    activeSessionIdRef.current = sessionId;
    setMessages(initialMessages);
    setTimeline(buildTimelineFromMessages(initialMessages));
    setInput("");
    setIsLoading(false);
    setActiveStreamAgentIds([]);
    setError(null);
    setErrorType(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally only resets on agent/session change
  }, [agentId, sessionId, sessionResetKey, mcpServerIdsKey]);

  // Resume a background stream when switching back to a session with an active stream
  useEffect(() => {
    if (!sessionId) return;
    const reclaimed = reclaimStream(sessionId);
    if (reclaimed) {
      abortRef.current = reclaimed.controller;
      readerRef.current = reclaimed.reader;
      const gen = ++requestGenRef.current;
      setIsLoading(true);

      // Use baseMessages from the background state (captured at transfer time)
      const baseMessages = reclaimed.baseMessages;
      const baseTimeline = buildTimelineFromMessages(baseMessages);
      setMessages(baseMessages);
      setTimeline(baseTimeline);

      processSSEStream({
        reader: reclaimed.reader,
        gen,
        allMessages: baseMessages,
        baseTimeline,
        effectiveAgentId: agentId,
      })
        .catch((err) => {
          if (err.name === "AbortError") return;
          setError(err.message);
        })
        .finally(() => {
          abortRef.current = null;
          readerRef.current = null;
          setIsLoading(false);
        });
      return;
    }

    // Non-reclaimable streams: show baseMessages captured at transfer time
    const bgMessages = getBaseMessages(sessionId);
    if (bgMessages && bgMessages.length > 0) {
      setMessages(bgMessages);
      setTimeline(buildTimelineFromMessages(bgMessages));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- resume only on session switch
  }, [sessionId, sessionResetKey]);

  const stop = useCallback(() => {
    // Stop single stream
    abortRef.current?.abort();
    abortRef.current = null;
    readerRef.current = null;
    // Stop parallel streams
    for (const controller of parallelControllersRef.current.values()) {
      controller.abort();
    }
    parallelControllersRef.current.clear();
    setIsLoading(false);
    setActiveStreamAgentIds([]);
    if (activeSessionIdRef.current) {
      stopStream(activeSessionIdRef.current);
    }
  }, [stopStream]);

  // Single-agent send (also used for MCP)
  const sendMessage = useCallback((targetAgentId?: string, sendContext?: boolean, attachments?: Attachment[], textOverride?: string, baseMessagesOverride?: Message[]) => {
    const text = (textOverride || input).trim();
    if (!text || isLoading) return;

    const baseMessages = baseMessagesOverride || messages;
    const effectiveAgentId = targetAgentId || agentId;

    const userMessage: Message = {
      role: "user",
      content: text,
      ...(userName ? { user_name: userName } : {}),
      ...(attachments && attachments.length > 0 ? { attachments } : {}),
    };
    const allMessages = [...baseMessages, userMessage];
    setMessages(allMessages);

    // Build base timeline from allMessages (preserves tool groups from history)
    const baseTimeline: TimelineItem[] = buildTimelineFromMessages(allMessages);
    setTimeline(baseTimeline);

    if (!textOverride) setInput(""); // Don't clear input on retry
    setError(null);
    setErrorType(null);
    setIsLoading(true);
    setActiveStreamAgentIds([effectiveAgentId]);

    retryCountRef.current = 0;

    const controller = new AbortController();
    abortRef.current = controller;
    const thisGen = ++requestGenRef.current;
    streamAgentIdRef.current = effectiveAgentId;
    streamAgentNameRef.current = agentName;
    streamSessionNameRef.current = sessionName;
    streamMessagesRef.current = allMessages;

    // Determine endpoint: MCP mode uses MCP endpoints, otherwise agent endpoint
    let endpoint: string;
    if (mcpConfig) {
      endpoint = mcpConfig.serverIds.length > 1
        ? getMultiMCPChatEndpoint()
        : getMCPChatEndpoint(mcpConfig.serverIds[0]);
    } else {
      endpoint = chatEndpointOverride || getChatEndpoint(effectiveAgentId);
    }

    const bodyObj: Record<string, unknown> = {
      messages: allMessages.map((m) => ({
        role: m.role,
        content: m.content,
        ...(m.agent_id ? { agent_id: m.agent_id } : {}),
        ...(m.broadcast_agent_ids && m.broadcast_agent_ids.length > 0 ? { broadcast_agent_ids: m.broadcast_agent_ids } : {}),
        ...(m.attachments && m.attachments.length > 0 ? { attachments: m.attachments } : {}),
      })),
      ...(activeSessionIdRef.current ? { session_id: activeSessionIdRef.current } : {}),
    };

    // MCP-specific fields
    if (mcpConfig) {
      bodyObj.model_id = mcpConfig.modelId;
      if (mcpConfig.serverIds.length > 1) {
        bodyObj.server_ids = mcpConfig.serverIds;
      }
    }

    // Multi-agent context propagation
    if (sendContext) bodyObj.send_context = true;

    // Group ID for collaborative sessions
    if (groupId) bodyObj.group_id = groupId;

    fetch(endpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Accept-Language": document.documentElement.lang || "es",
      },
      body: JSON.stringify(bodyObj),
      signal: controller.signal,
    })
      .then(async (response) => {
        // Discard if a newer request has been initiated (agent/session switched)
        if (requestGenRef.current !== thisGen) return;

        if (response.status === 401) {
          window.location.href = "/login";
          return;
        }
        if (!response.ok) {
          const errData = await response.json().catch(() => ({}));
          if (errData.error === "github_token_required") {
            throw new GitHubAuthError(errData.message || "GitHub authentication required");
          }
          if (errData.error === "oauth2_consent_required") {
            throw new MCPOAuth2Error(errData.message || "OAuth2 authorization required");
          }
          throw new Error(errData.error || `Error ${response.status}`);
        }

        const reader = response.body?.getReader();
        if (!reader) throw new Error("No response stream");

        readerRef.current = reader;

        return processSSEStream({
          reader,
          gen: thisGen,
          allMessages,
          baseTimeline,
          effectiveAgentId,
        });
      })
      .catch((err) => {
        if (err.name === "AbortError") return;
        if (requestGenRef.current !== thisGen) return;

        if (isRetryableError(err) && retryCountRef.current < 3) {
          retryCountRef.current++;
          const delay = Math.pow(2, retryCountRef.current - 1) * 1000; // 1s, 2s, 4s
          setIsReconnecting(true);
          setError(null);
          reportMetric({ name: "sse_reconnect", labels: { agent_id: effectiveAgentId, attempt: String(retryCountRef.current) }, value: 1 });

          setTimeout(() => {
            if (requestGenRef.current !== thisGen) {
              setIsReconnecting(false);
              return;
            }
            setIsReconnecting(false);
            retry();
          }, delay);
          return;
        }

        retryCountRef.current = 0;
        setIsReconnecting(false);
        setError(err.message);
        setErrorType(err instanceof GitHubAuthError ? "github_auth" : err instanceof MCPOAuth2Error ? "mcp_oauth2" : null);
        reportMetric({ name: "sse_error", labels: { agent_id: effectiveAgentId, error_type: "stream" }, value: 1 });
      })
      .finally(() => {
        if (requestGenRef.current !== thisGen) return;
        // Don't reset loading state if a reconnection retry is pending
        if (retryCountRef.current > 0) return;
        abortRef.current = null;
        readerRef.current = null;
        setIsLoading(false);
        setActiveStreamAgentIds([]);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- agentName, chatEndpointOverride, sessionName are intentionally excluded to avoid re-creating callback on display-only changes
  }, [input, messages, isLoading, agentId, sessionId, onSessionCreated, processSSEStream]);

  // Parallel multi-agent send: fires N streams simultaneously
  const sendMultiple = useCallback((targetAgentIds: string[], sendContext?: boolean, attachments?: Attachment[], textOverride?: string, baseMessagesOverride?: Message[]) => {
    const text = (textOverride || input).trim();
    if (!text || isLoading || targetAgentIds.length === 0) return;

    const baseMessages = baseMessagesOverride || messages;

    const userMessage: Message = {
      role: "user",
      content: text,
      ...(userName ? { user_name: userName } : {}),
      ...(targetAgentIds.length > 0 ? { broadcast_agent_ids: [...targetAgentIds] } : {}),
      ...(attachments && attachments.length > 0 ? { attachments } : {}),
    };
    const allMessages = [...baseMessages, userMessage];
    setMessages(allMessages);

    const baseTimeline: TimelineItem[] = buildTimelineFromMessages(allMessages);
    setTimeline(baseTimeline);
    if (!textOverride) setInput(""); // Don't clear input on retry
    setError(null);
    setErrorType(null);
    setIsLoading(true);
    setActiveStreamAgentIds([...targetAgentIds]);

    const thisGen = ++requestGenRef.current;
    streamAgentIdRef.current = targetAgentIds[0];
    streamAgentNameRef.current = agentName;
    streamSessionNameRef.current = sessionName;
    streamMessagesRef.current = allMessages;

    // Per-agent retry tracking
    const agentRetryMap = new Map<string, number>();
    const retryingAgents = new Set<string>();

    // Per-agent stream items for parallel timeline merging
    const perAgentItems = new Map<string, TimelineItem[]>();
    const perAgentFinalMsgs = new Map<string, Message>();
    // Track order in which agents start producing content (first TEXT_MESSAGE_START)
    const respondedOrder: string[] = [];
    let pendingCount = targetAgentIds.length;

    const rebuildTimeline = () => {
      if (requestGenRef.current !== thisGen) return;
      const merged: TimelineItem[] = [...baseTimeline];
      // Agents that have responded come first (in order of first response)
      for (const aid of respondedOrder) {
        const items = perAgentItems.get(aid);
        if (items) merged.push(...items);
      }
      // Then agents that haven't responded yet (preserve original order)
      for (const aid of targetAgentIds) {
        if (!respondedOrder.includes(aid)) {
          const items = perAgentItems.get(aid);
          if (items) merged.push(...items);
        }
      }
      setTimeline(merged);
    };

    const rebuildFinalMessages = () => {
      if (requestGenRef.current !== thisGen) return;
      const finalMsgs: Message[] = [...allMessages];
      // Use respondedOrder for consistent ordering, then remaining agents
      for (const aid of respondedOrder) {
        const msg = perAgentFinalMsgs.get(aid);
        if (msg) finalMsgs.push(msg);
      }
      for (const aid of targetAgentIds) {
        if (!respondedOrder.includes(aid)) {
          const msg = perAgentFinalMsgs.get(aid);
          if (msg) finalMsgs.push(msg);
        }
      }
      setMessages(finalMsgs);
    };

    const mappedMessages = allMessages.map((m) => ({
      role: m.role,
      content: m.content,
      ...(m.agent_id ? { agent_id: m.agent_id } : {}),
      ...(m.broadcast_agent_ids && m.broadcast_agent_ids.length > 0 ? { broadcast_agent_ids: m.broadcast_agent_ids } : {}),
      ...(m.attachments && m.attachments.length > 0 ? { attachments: m.attachments } : {}),
    }));

    // Launch a stream for one agent
    const launchAgent = (targetId: string, onRunStarted?: () => void) => {
      const controller = new AbortController();
      parallelControllersRef.current.set(targetId, controller);

      const endpoint = getChatEndpoint(targetId);
      const bodyObj: Record<string, unknown> = {
        messages: mappedMessages,
        ...(activeSessionIdRef.current ? { session_id: activeSessionIdRef.current } : {}),
      };
      if (sendContext) bodyObj.send_context = true;
      if (groupId) bodyObj.group_id = groupId;

      fetch(endpoint, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Accept-Language": document.documentElement.lang || "es",
        },
        body: JSON.stringify(bodyObj),
        signal: controller.signal,
      })
        .then(async (response) => {
          if (requestGenRef.current !== thisGen) return;
          if (response.status === 401) { window.location.href = "/login"; return; }
          if (!response.ok) {
            const errData = await response.json().catch(() => ({}));
            if (errData.error === "github_token_required") {
              throw new GitHubAuthError(errData.message || "GitHub authentication required");
            }
            if (errData.error === "oauth2_consent_required") {
              throw new MCPOAuth2Error(errData.message || "OAuth2 authorization required");
            }
            throw new Error(errData.error || `Error ${response.status}`);
          }
          const reader = response.body?.getReader();
          if (!reader) throw new Error("No response stream");

          await processAgentStream({
            reader,
            gen: thisGen,
            targetAgentId: targetId,
            perAgentItems,
            perAgentFinalMsgs,
            respondedOrder,
            rebuildTimeline,
            onRunStarted,
          });
        })
        .catch((err) => {
          if (err.name === "AbortError") return;
          if (requestGenRef.current !== thisGen) return;

          const agentRetries = agentRetryMap.get(targetId) ?? 0;
          if (isRetryableError(err) && agentRetries < 3) {
            agentRetryMap.set(targetId, agentRetries + 1);
            retryingAgents.add(targetId);
            const delay = Math.pow(2, agentRetries) * 1000;
            setIsReconnecting(true);
            reportMetric({ name: "sse_reconnect", labels: { agent_id: targetId, attempt: String(agentRetries + 1) }, value: 1 });

            setTimeout(() => {
              if (requestGenRef.current !== thisGen) {
                setIsReconnecting(false);
                return;
              }
              setIsReconnecting(false);
              pendingCount++;
              launchAgent(targetId);
            }, delay);
            return;
          }

          setError(err.message);
          setErrorType(err instanceof GitHubAuthError ? "github_auth" : err instanceof MCPOAuth2Error ? "mcp_oauth2" : null);
          reportMetric({ name: "sse_error", labels: { agent_id: targetId, error_type: "stream" }, value: 1 });
        })
        .finally(() => {
          parallelControllersRef.current.delete(targetId);
          pendingCount--;
          // Don't remove from active streams if a retry is pending
          if (retryingAgents.has(targetId)) {
            retryingAgents.delete(targetId);
          } else {
            setActiveStreamAgentIds((prev) => prev.filter((id) => id !== targetId));
          }
          if (pendingCount <= 0) {
            setIsLoading(false);
            setIsReconnecting(false);
            rebuildFinalMessages();
          }
        });
    };

    if (activeSessionIdRef.current || targetAgentIds.length === 1) {
      // Session exists or single agent: fire all in parallel
      for (const targetId of targetAgentIds) {
        launchAgent(targetId);
      }
    } else {
      // No session yet: first agent establishes session, rest wait
      let resolveSessionReady: () => void;
      const sessionReady = new Promise<void>((r) => { resolveSessionReady = r; });

      launchAgent(targetAgentIds[0], () => resolveSessionReady());

      sessionReady.then(() => {
        if (requestGenRef.current !== thisGen) return;
        for (const targetId of targetAgentIds.slice(1)) {
          launchAgent(targetId);
        }
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [input, messages, isLoading, agentId, sessionId, onSessionCreated]);

  // Process a single agent's SSE stream for parallel multi-agent mode.
  // Each stream maintains its own items and merges via rebuildTimeline.
  const processAgentStream = useCallback(async (params: AgentStreamParams) => {
    const { reader, gen, targetAgentId, perAgentItems, perAgentFinalMsgs, respondedOrder, rebuildTimeline } = params;

    const streamDecoder = createStreamDecoder();
    let buffer = "";
    let currentContent = "";
    let currentIsThinking = false;
    let hasOpenMessage = false;
    const segments: ContentSegment[] = [];
    let streamItems: TimelineItem[] = [];
    let runFinished = false;

    const isStale = () => requestGenRef.current !== gen;
    let rafId: number | null = null;
    let pendingUpdate = false;

    const scheduleUpdate = () => {
      if (isStale()) return;
      pendingUpdate = true;
      if (rafId === null) {
        rafId = requestAnimationFrame(() => {
          rafId = null;
          if (pendingUpdate && !isStale()) {
            pendingUpdate = false;
            perAgentItems.set(targetAgentId, [...streamItems]);
            rebuildTimeline();
          }
        });
      }
    };
    const flushUpdate = () => {
      if (rafId !== null) cancelAnimationFrame(rafId);
      rafId = null;
      if (!isStale()) {
        perAgentItems.set(targetAgentId, [...streamItems]);
        rebuildTimeline();
      }
    };

    while (true) {
      if (isStale()) break;
      const { done, value } = await reader.read();
      if (done || isStale()) break;

      buffer += streamDecoder.decode(value);
      const { events, remaining } = parseSSELines(buffer);
      buffer = remaining;

      for (const event of events) {
        if (isStale()) break;

        switch (event.type) {
          case "RUN_STARTED":
            if (event.threadId) {
              activeSessionIdRef.current = event.threadId;
              onSessionCreated?.(event.threadId, event.sessionName as string | undefined);
            }
            // Always signal run started (even without threadId) so parallel agents can proceed
            params.onRunStarted?.();
            break;

          case "TEXT_MESSAGE_START":
            currentContent = "";
            currentIsThinking = event.isThinking === true;
            hasOpenMessage = true;
            // Track response order: first TEXT_MESSAGE_START (thinking or not) registers the agent
            if (!respondedOrder.includes(targetAgentId)) {
              respondedOrder.push(targetAgentId);
            }
            break;

          case "TEXT_MESSAGE_CONTENT": {
            currentContent += event.delta;
            const msgItem: TimelineItem = {
              type: "message",
              message: {
                role: "assistant",
                content: currentContent,
                isThinking: currentIsThinking,
                agent_id: targetAgentId,
              },
            };
            const lastStream = streamItems[streamItems.length - 1];
            if (lastStream?.type === "message" && lastStream.message.role === "assistant") {
              streamItems = [...streamItems.slice(0, -1), msgItem];
            } else {
              streamItems = [...streamItems, msgItem];
            }
            scheduleUpdate();
            break;
          }

          case "TEXT_MESSAGE_END":
            if (currentContent.trim()) {
              segments.push({ content: currentContent, isThinking: currentIsThinking });
            }
            hasOpenMessage = false;
            break;

          case "TOOL_CALL_START": {
            // Tool calls also count as "responded" for ordering
            if (!respondedOrder.includes(targetAgentId)) {
              respondedOrder.push(targetAgentId);
            }
            const newTc: ToolCall = {
              toolCallId: event.toolCallId!,
              toolName: event.toolName!,
              args: "",
              isComplete: false,
              ...(event.serverId ? { serverId: event.serverId } : {}),
            };
            const lastItem = streamItems[streamItems.length - 1];
            if (lastItem?.type === "tool_group") {
              streamItems = [...streamItems.slice(0, -1), { type: "tool_group", toolCalls: [...lastItem.toolCalls, newTc], agentId: targetAgentId }];
            } else {
              streamItems = [...streamItems, { type: "tool_group", toolCalls: [newTc], agentId: targetAgentId }];
            }
            flushUpdate();
            break;
          }

          case "TOOL_CALL_ARGS": {
            streamItems = streamItems.map((item) => {
              if (item.type === "tool_group") {
                return {
                  ...item,
                  toolCalls: item.toolCalls.map((tc) =>
                    tc.toolCallId === event.toolCallId ? { ...tc, args: tc.args + event.delta } : tc
                  ),
                };
              }
              return item;
            });
            scheduleUpdate();
            break;
          }

          case "TOOL_CALL_END": {
            streamItems = streamItems.map((item) => {
              if (item.type === "tool_group") {
                return {
                  ...item,
                  toolCalls: item.toolCalls.map((tc) =>
                    tc.toolCallId === event.toolCallId ? { ...tc, result: event.result, isComplete: true } : tc
                  ),
                };
              }
              return item;
            });
            flushUpdate();
            break;
          }

          case "CUSTOM": {
            if (event.subType === "CHART" && event.data) {
              streamItems = [...streamItems, {
                type: "chart" as const,
                chart: event.data as ChartData,
                agentId: targetAgentId,
              }];
              flushUpdate();
            }
            break;
          }

          case "RUN_FINISHED": {
            runFinished = true;
            const finalized = buildFinalizedSegments(segments, hasOpenMessage, currentContent, currentIsThinking);
            const finalContent = finalizeSegments(finalized);

            if (finalContent) {
              const finalMsg: Message = attachToolCalls(
                { role: "assistant", content: finalContent, agent_id: targetAgentId },
                streamItems,
              );
              perAgentFinalMsgs.set(targetAgentId, finalMsg);
              streamItems = applyFinalMessage(streamItems, finalMsg);
              flushUpdate();
            }
            break;
          }

          case "RUN_ERROR":
            throw new Error(event.message || "Agent error");
        }
      }
    }

    // Fallback only if stream ended without RUN_FINISHED
    if (!runFinished) {
      const finalized = buildFinalizedSegments(segments, hasOpenMessage, currentContent, currentIsThinking);
      if (finalized.length > 0) {
        const finalContent = finalizeSegments(finalized);
        const finalMsg: Message = attachToolCalls(
          { role: "assistant", content: finalContent, agent_id: targetAgentId },
          streamItems,
        );
        perAgentFinalMsgs.set(targetAgentId, finalMsg);
        streamItems = applyFinalMessage(streamItems, finalMsg);
        flushUpdate();
      }
    }
  }, [onSessionCreated]);

  const retry = useCallback((targetMessage?: Message, targetAgentIds?: string[]) => {
    if (isLoading) return;

    // Find target user message: explicit or last one
    const targetMsg = targetMessage || [...messages].reverse().find((m) => m.role === "user");
    if (!targetMsg || targetMsg.role !== "user") return;
    const targetIdx = messages.lastIndexOf(targetMsg);
    if (targetIdx === -1) return;

    // Also remove the system message right before (context status)
    let startIdx = targetIdx;
    if (startIdx > 0 && messages[startIdx - 1]?.role === "system") {
      startIdx--;
    }
    const messagesBeforeRetry = messages.slice(0, startIdx);
    setError(null);

    // Multi-agent retry: use sendMultiple with the provided or original target agents
    const retryTargets = targetAgentIds || targetMsg.broadcast_agent_ids;
    if (retryTargets && retryTargets.length > 0) {
      sendMultiple(retryTargets, true, targetMsg.attachments, targetMsg.content, messagesBeforeRetry);
      return;
    }

    sendMessage(targetMsg.agent_id, undefined, targetMsg.attachments, targetMsg.content, messagesBeforeRetry);
  }, [messages, isLoading, sendMessage, sendMultiple]);

  // Allow external code to replace messages (e.g. when reloading session from server)
  const replaceMessages = useCallback((newMessages: Message[]) => {
    if (isLoading) return; // Don't replace while streaming
    setMessages(newMessages);
    setTimeline(buildTimelineFromMessages(newMessages));
  }, [isLoading]);

  // Reconnect to an in-flight run after a page reload: opens the server's
  // replay+live SSE stream and feeds it through the same render pipeline as a
  // normal chat. Returns false if there's nothing to reconnect to (204), so the
  // caller can fall back to polling / loading the persisted session.
  const reconnectToRun = useCallback(async (reconnectAgentId: string, reconnectSessionId: string): Promise<boolean> => {
    if (isLoadingRef.current) return false;

    const controller = new AbortController();
    let response: Response;
    try {
      response = await fetch(getRunStreamUrl(reconnectAgentId, reconnectSessionId), {
        method: "GET",
        signal: controller.signal,
      });
    } catch {
      return false;
    }
    // 204 = nothing buffered and no active run → fall back.
    if (response.status === 204 || !response.ok || !response.body) {
      return false;
    }

    const reader = response.body.getReader();
    const gen = ++requestGenRef.current;
    abortRef.current = controller;
    readerRef.current = reader;
    setIsLoading(true);
    setActiveStreamAgentIds([reconnectAgentId]);

    const baseMessages = messagesStateRef.current; // ends in the user message
    const baseTimeline = buildTimelineFromMessages(baseMessages);

    try {
      await processSSEStream({
        reader,
        gen,
        allMessages: baseMessages,
        baseTimeline,
        effectiveAgentId: reconnectAgentId,
        suppressSessionCreated: true,
      });
      return true;
    } catch (err) {
      if ((err as Error).name === "AbortError") return true;
      return false; // caller falls back to polling / GET session
    } finally {
      if (requestGenRef.current === gen) {
        abortRef.current = null;
        readerRef.current = null;
        setIsLoading(false);
      }
    }
  }, [processSSEStream]);

  // Prepend older messages (for pagination / load-more)
  const prependMessages = useCallback((olderMessages: Message[]) => {
    if (isLoading) return; // Don't modify while streaming
    setMessages((prev) => {
      const combined = [...olderMessages, ...prev];
      setTimeline(buildTimelineFromMessages(combined));
      return combined;
    });
  }, [isLoading]);

  return { messages, timeline, input, setInput, sendMessage, sendMultiple, activeStreamAgentIds, isLoading, error, errorType, stop, retry, replaceMessages, prependMessages, isReconnecting, reconnectToRun };
}
