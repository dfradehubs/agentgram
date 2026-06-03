"use client";

import React, { createContext, useContext, useState, useCallback, useEffect, useRef } from "react";
import type { Session } from "@/lib/types";
import { getReadState, markSessionRead, migrateReadState, getReadStateSubscribeUrl } from "@/lib/api";

const STORAGE_KEY = "agentgram-read-counts";
const POLL_INTERVAL = 60_000; // 60s (fallback)
const MAX_RECONNECT_DELAY = 30_000; // 30s max backoff

interface ReadStateContextType {
  getUnreadCount: (sessionId: string, currentMessageCount: number) => number;
  markAsRead: (sessionId: string, messageCount: number) => void;
  getTotalUnread: (sessions: Session[]) => number;
}

const ReadStateContext = createContext<ReadStateContextType | undefined>(undefined);

export function ReadStateProvider({ children }: { children: React.ReactNode }) {
  const [counts, setCounts] = useState<Record<string, number>>({});
  const initialized = useRef(false);
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const reconnectAttemptRef = useRef(0);
  const usingSSERef = useRef(false);

  // Load from server on mount + migrate localStorage if needed
  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;

    async function init() {
      try {
        const serverState = await getReadState();
        setCounts(serverState);

        // Migrate localStorage data if present
        const raw = localStorage.getItem(STORAGE_KEY);
        if (raw) {
          const localState: Record<string, number> = JSON.parse(raw);
          if (Object.keys(localState).length > 0) {
            // Merge: keep the higher count from either source
            const merged = { ...serverState };
            for (const [sid, count] of Object.entries(localState)) {
              if (count > (merged[sid] ?? 0)) {
                merged[sid] = count;
              }
            }
            await migrateReadState(merged);
            setCounts(merged);
          }
          localStorage.removeItem(STORAGE_KEY);
        }
      } catch {
        // Fallback: try localStorage if server is unreachable
        try {
          const raw = localStorage.getItem(STORAGE_KEY);
          if (raw) setCounts(JSON.parse(raw));
        } catch {
          // ignore
        }
      }
    }

    init();
  }, []);

  // SSE connection with polling fallback
  useEffect(() => {
    let pollIntervalId: NodeJS.Timeout | null = null;

    const connectSSE = () => {
      // Clean up any existing connection
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }

      const url = getReadStateSubscribeUrl();
      const es = new EventSource(url, { withCredentials: true });
      eventSourceRef.current = es;

      es.onopen = () => {
        usingSSERef.current = true;
        reconnectAttemptRef.current = 0;
        // Stop polling when SSE is connected
        stopPolling();
      };

      es.onmessage = (ev) => {
        try {
          const data = JSON.parse(ev.data);
          if (data.type === "read_state_update" && data.session_id) {
            setCounts((prev) => {
              if (prev[data.session_id] === data.count) return prev;
              return { ...prev, [data.session_id]: data.count };
            });
          }
        } catch {
          // Ignore non-JSON messages (keep-alive comments)
        }
      };

      es.onerror = () => {
        // Connection lost - close and schedule reconnect
        es.close();
        eventSourceRef.current = null;
        usingSSERef.current = false;

        // Start polling as fallback while disconnected
        startPolling();

        // Schedule reconnection with exponential backoff
        const attempt = reconnectAttemptRef.current;
        const delay = Math.min(1000 * Math.pow(2, attempt), MAX_RECONNECT_DELAY);
        reconnectAttemptRef.current = attempt + 1;

        reconnectTimeoutRef.current = setTimeout(() => {
          reconnectTimeoutRef.current = null;
          connectSSE();
        }, delay);
      };
    };

    const poll = async () => {
      try {
        const serverState = await getReadState();
        setCounts(serverState);
      } catch {
        // ignore polling errors
      }
    };

    const startPolling = () => {
      if (pollIntervalId) return;
      pollIntervalId = setInterval(poll, POLL_INTERVAL);
    };

    const stopPolling = () => {
      if (pollIntervalId) {
        clearInterval(pollIntervalId);
        pollIntervalId = null;
      }
    };

    const handleVisibility = () => {
      if (document.hidden) {
        // Tab hidden: disconnect SSE to save resources, stop polling
        if (eventSourceRef.current) {
          eventSourceRef.current.close();
          eventSourceRef.current = null;
        }
        if (reconnectTimeoutRef.current) {
          clearTimeout(reconnectTimeoutRef.current);
          reconnectTimeoutRef.current = null;
        }
        stopPolling();
      } else {
        // Tab visible: fetch latest state, reconnect SSE
        poll();
        reconnectAttemptRef.current = 0;
        connectSSE();
      }
    };

    // Start SSE connection
    connectSSE();
    document.addEventListener("visibilitychange", handleVisibility);

    return () => {
      document.removeEventListener("visibilitychange", handleVisibility);
      stopPolling();
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }
    };
  }, []);

  const getUnreadCount = useCallback((sessionId: string, currentMessageCount: number): number => {
    const stored = counts[sessionId] ?? 0;
    return Math.max(0, currentMessageCount - stored);
  }, [counts]);

  const markAsRead = useCallback((sessionId: string, messageCount: number) => {
    setCounts((prev) => {
      if (prev[sessionId] === messageCount) return prev;
      const next = { ...prev, [sessionId]: messageCount };
      // Fire-and-forget server update
      markSessionRead(sessionId, messageCount).catch(() => {});
      return next;
    });
  }, []);

  const getTotalUnread = useCallback((sessions: Session[]): number => {
    let total = 0;
    for (const s of sessions) {
      const stored = counts[s.session_id] ?? 0;
      total += Math.max(0, s.message_count - stored);
    }
    return total;
  }, [counts]);

  return (
    <ReadStateContext.Provider value={{ getUnreadCount, markAsRead, getTotalUnread }}>
      {children}
    </ReadStateContext.Provider>
  );
}

export function useReadState() {
  const context = useContext(ReadStateContext);
  if (context === undefined) {
    throw new Error("useReadState must be used within a ReadStateProvider");
  }
  return context;
}
