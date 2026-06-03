"use client";

import React, {
  createContext,
  useContext,
  useRef,
  useCallback,
  useSyncExternalStore,
  useEffect,
} from "react";
import { toast } from "sonner";
import { useSessionContext } from "./SessionContext";
import { useAgentContext } from "./AgentContext";
import { useMCPContext } from "./MCPContext";
import type { Message } from "@/lib/types";
import {
  requestNotificationPermission,
  isTabHidden,
  sendBrowserNotification,
} from "@/lib/notifications";
import { useT } from "@/lib/i18n";

type StreamType = "single" | "broadcast" | "conversation" | "mcp";

interface BackgroundStreamState {
  sessionId: string;
  sessionName: string;
  agentId: string;
  agentName: string;
  controller: AbortController;
  reader: ReadableStreamDefaultReader<Uint8Array>;
  eventBuffer: Uint8Array[];
  baseMessages: Message[]; // conversation history at transfer time (user + prior assistant messages)
  streamType: StreamType;
  isMultiAgent: boolean;
  completed: boolean;
  reclaimed: boolean; // signals background loop to stop without aborting fetch
}

interface BackgroundStreamContextType {
  transferStream: (
    state: Omit<BackgroundStreamState, "eventBuffer" | "completed" | "reclaimed">
  ) => void;
  reclaimStream: (
    sessionId: string
  ) => {
    reader: ReadableStreamDefaultReader<Uint8Array>;
    controller: AbortController;
    baseMessages: Message[];
  } | null;
  /** Get baseMessages for a non-reclaimable background stream (broadcast/conversation/mcp).
   *  Returns null if no active background stream exists for this session. */
  getBaseMessages: (sessionId: string) => Message[] | null;
  stopStream: (sessionId: string) => void;
  isSessionRunning: (sessionId: string) => boolean;
  runningSessionIds: string[];
}

const BackgroundStreamContext = createContext<
  BackgroundStreamContextType | undefined
>(undefined);

// External store for running session IDs to avoid re-renders on every Map mutation
function createSessionStore() {
  let ids: string[] = [];
  const listeners = new Set<() => void>();
  return {
    getSnapshot: () => ids,
    subscribe: (cb: () => void) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    update: (newIds: string[]) => {
      // Only notify if changed
      if (
        newIds.length !== ids.length ||
        newIds.some((id, i) => id !== ids[i])
      ) {
        ids = newIds;
        listeners.forEach((cb) => cb());
      }
    },
  };
}

export function BackgroundStreamProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const streamsRef = useRef(new Map<string, BackgroundStreamState>());
  const store = React.useMemo(() => createSessionStore(), []);
  const {
    selectSession,
  } = useSessionContext();
  const { selectAgent } = useAgentContext();
  const { selectMCPSession, selectMultiMCPSession, selectMCPServer } = useMCPContext();
  const t = useT();

  // Request notification permission on mount
  useEffect(() => {
    requestNotificationPermission();
  }, []);

  const syncStore = useCallback(() => {
    const ids = Array.from(streamsRef.current.keys());
    store.update(ids);
  }, [store]);

  const processBackgroundStream = useCallback(
    async (state: BackgroundStreamState) => {
      const { reader, sessionId } = state;
      try {
        const decoder = new TextDecoder();
        let textBuffer = "";

        while (true) {
          // If reclaimed, useChat took ownership of the reader — exit silently
          if (state.reclaimed) return;

          const { done, value } = await reader.read();
          if (done) break;
          if (state.reclaimed) return;

          // Store raw bytes for reclaim
          state.eventBuffer.push(value);

          // Minimal parse to detect completion
          textBuffer += decoder.decode(value, { stream: true });
          const lines = textBuffer.split("\n");
          textBuffer = lines.pop() || "";

          for (const line of lines) {
            const trimmed = line.trim();
            if (!trimmed || !trimmed.startsWith("data: ")) continue;
            // Check for RUN_FINISHED or RUN_ERROR
            if (
              trimmed.includes('"RUN_FINISHED"') ||
              trimmed.includes('"RUN_ERROR"')
            ) {
              state.completed = true;
              const isError = trimmed.includes('"RUN_ERROR"');

              // Notify
              const label = state.sessionName || state.agentName;
              const message = isError
                ? t("notifications.error", { name: label })
                : t("notifications.completed", { name: label });

              const navigateToSession = () => {
                if (state.streamType === "mcp") {
                  if (state.isMultiAgent) {
                    selectMultiMCPSession(sessionId);
                  } else {
                    selectMCPServer(state.agentId);
                    selectMCPSession(sessionId);
                  }
                } else {
                  selectAgent(state.agentId);
                  selectSession(sessionId);
                }
              };

              toast(message, {
                action: {
                  label: t("notifications.view"),
                  onClick: navigateToSession,
                },
              });

              if (isTabHidden()) {
                sendBrowserNotification(
                  "Agentgram",
                  message,
                  navigateToSession
                );
              }

              // Cleanup
              streamsRef.current.delete(sessionId);
              syncStore();
              return;
            }
          }
        }

        // Stream ended without RUN_FINISHED (connection dropped)
        state.completed = true;
        streamsRef.current.delete(sessionId);
        syncStore();
      } catch (err) {
        // AbortError is expected when stopStream is called
        if (err instanceof DOMException && err.name === "AbortError") return;

        const label = state.sessionName || state.agentName;
        toast.error(t("notifications.error", { name: label }));

        streamsRef.current.delete(sessionId);
        syncStore();
      }
    },
    [
      t,
      selectSession,
      selectAgent,
      selectMCPSession,
      selectMultiMCPSession,
      selectMCPServer,
      syncStore,
    ]
  );

  const transferStream = useCallback(
    (
      incoming: Omit<BackgroundStreamState, "eventBuffer" | "completed" | "reclaimed">
    ) => {
      const state: BackgroundStreamState = {
        ...incoming,
        eventBuffer: [],
        completed: false,
        reclaimed: false,
      };
      streamsRef.current.set(incoming.sessionId, state);
      syncStore();
      processBackgroundStream(state);
    },
    [syncStore, processBackgroundStream]
  );

  const reclaimStream = useCallback(
    (
      sessionId: string
    ): {
      reader: ReadableStreamDefaultReader<Uint8Array>;
      controller: AbortController;
      baseMessages: Message[];
    } | null => {
      const state = streamsRef.current.get(sessionId);
      if (!state || state.completed) {
        // If completed, just clean up
        if (state) {
          streamsRef.current.delete(sessionId);
          syncStore();
        }
        return null;
      }

      // Only reclaim single-agent streams (they use processSSEStream).
      // Broadcast, conversation and MCP streams stay in background for monitoring
      // (spinner + notification). User sees API data when they open the session.
      if (state.streamType !== "single") {
        return null;
      }

      // Signal background loop to stop reading — we're taking ownership
      state.reclaimed = true;

      // Build composite stream: buffered events + live reader
      const bufferedChunks = [...state.eventBuffer];
      const liveReader = state.reader;

      const compositeStream = new ReadableStream<Uint8Array>({
        async start(controller) {
          // Emit buffered chunks
          for (const chunk of bufferedChunks) {
            controller.enqueue(chunk);
          }
          // Then pipe live reader
          try {
            while (true) {
              const { done, value } = await liveReader.read();
              if (done) {
                controller.close();
                break;
              }
              controller.enqueue(value);
            }
          } catch (err) {
            controller.error(err);
          }
        },
        cancel() {
          liveReader.cancel();
        },
      });

      const result = {
        reader: compositeStream.getReader(),
        controller: state.controller,
        baseMessages: state.baseMessages,
      };

      // Remove from background (useChat takes ownership)
      streamsRef.current.delete(sessionId);
      syncStore();

      return result;
    },
    [syncStore]
  );

  const getBaseMessages = useCallback((sessionId: string): Message[] | null => {
    const state = streamsRef.current.get(sessionId);
    if (!state || state.completed) return null;
    return state.baseMessages;
  }, []);

  const stopStream = useCallback(
    (sessionId: string) => {
      const state = streamsRef.current.get(sessionId);
      if (state) {
        state.controller.abort();
        streamsRef.current.delete(sessionId);
        syncStore();
      }
    },
    [syncStore]
  );

  const isSessionRunning = useCallback((sessionId: string): boolean => {
    const state = streamsRef.current.get(sessionId);
    return !!state && !state.completed;
  }, []);

  const runningSessionIds = useSyncExternalStore(
    store.subscribe,
    store.getSnapshot,
    store.getSnapshot
  );

  return (
    <BackgroundStreamContext.Provider
      value={{
        transferStream,
        reclaimStream,
        getBaseMessages,
        stopStream,
        isSessionRunning,
        runningSessionIds,
      }}
    >
      {children}
    </BackgroundStreamContext.Provider>
  );
}

export function useBackgroundStreamContext() {
  const context = useContext(BackgroundStreamContext);
  if (context === undefined) {
    throw new Error(
      "useBackgroundStreamContext must be used within a BackgroundStreamProvider"
    );
  }
  return context;
}
