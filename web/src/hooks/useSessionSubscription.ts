"use client";

import { useEffect, useRef, useCallback } from "react";
import { getSessionSubscribeUrl } from "@/lib/api";

interface UseSessionSubscriptionOptions {
  sessionId?: string;
  groupId?: string;
  enabled?: boolean;
  onEvent?: (event: Record<string, unknown>) => void;
}

/**
 * Hook that subscribes to real-time events for a group session via SSE.
 * Only activates when both sessionId and groupId are present and enabled is true.
 */
export function useSessionSubscription({
  sessionId,
  groupId,
  enabled = true,
  onEvent,
}: UseSessionSubscriptionOptions) {
  const eventSourceRef = useRef<EventSource | null>(null);
  const onEventRef = useRef(onEvent);
  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  const disconnect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
  }, []);

  useEffect(() => {
    if (!sessionId || !groupId || !enabled) {
      disconnect();
      return;
    }

    const url = getSessionSubscribeUrl(sessionId);
    const es = new EventSource(url, { withCredentials: true });
    eventSourceRef.current = es;

    es.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data);
        onEventRef.current?.(data);
      } catch {
        // Ignore non-JSON messages (keep-alive comments)
      }
    };

    es.onerror = () => {
      // EventSource auto-reconnects; nothing to do here
    };

    return () => {
      es.close();
      eventSourceRef.current = null;
    };
  }, [sessionId, groupId, enabled, disconnect]);

  return { disconnect };
}
