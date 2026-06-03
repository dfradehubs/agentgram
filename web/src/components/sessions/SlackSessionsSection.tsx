"use client";

import { useCallback, useEffect, useState } from "react";
import { useUser } from "@/hooks/useUser";
import { useSessions } from "@/hooks/useSessions";
import { getSlackSessions } from "@/lib/api";
import type { Session } from "@/lib/types";
import { SessionList } from "./SessionList";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { ChevronRight } from "lucide-react";

const SlackIcon = () => (
  <svg className="h-3 w-3 shrink-0" viewBox="0 0 24 24" fill="currentColor">
    <path d="M5.042 15.165a2.528 2.528 0 0 1-2.52 2.523A2.528 2.528 0 0 1 0 15.165a2.527 2.527 0 0 1 2.522-2.52h2.52v2.52zm1.271 0a2.527 2.527 0 0 1 2.521-2.52 2.527 2.527 0 0 1 2.521 2.52v6.313A2.528 2.528 0 0 1 8.834 24a2.528 2.528 0 0 1-2.521-2.522v-6.313zM8.834 5.042a2.528 2.528 0 0 1-2.521-2.52A2.528 2.528 0 0 1 8.834 0a2.528 2.528 0 0 1 2.521 2.522v2.52H8.834zm0 1.271a2.528 2.528 0 0 1 2.521 2.521 2.528 2.528 0 0 1-2.521 2.521H2.522A2.528 2.528 0 0 1 0 8.834a2.528 2.528 0 0 1 2.522-2.521h6.312zm10.122 2.521a2.528 2.528 0 0 1 2.522-2.521A2.528 2.528 0 0 1 24 8.834a2.528 2.528 0 0 1-2.522 2.521h-2.522V8.834zm-1.268 0a2.528 2.528 0 0 1-2.523 2.521 2.527 2.527 0 0 1-2.52-2.521V2.522A2.527 2.527 0 0 1 15.165 0a2.528 2.528 0 0 1 2.523 2.522v6.312zm-2.523 10.122a2.528 2.528 0 0 1 2.523 2.522A2.528 2.528 0 0 1 15.165 24a2.527 2.527 0 0 1-2.52-2.522v-2.522h2.52zm0-1.268a2.527 2.527 0 0 1-2.52-2.523 2.526 2.526 0 0 1 2.52-2.52h6.313A2.527 2.527 0 0 1 24 15.165a2.528 2.528 0 0 1-2.522 2.523h-6.313z" />
  </svg>
);

export function SlackSessionsSection() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [isExpanded, setIsExpanded] = useState(false);
  const { user } = useUser();
  const { currentSession } = useSessions();

  const isActive = currentSession?.source === "slack";

  const fetchSessions = useCallback(async () => {
    try {
      const data = await getSlackSessions();
      setSessions(data);
    } catch {
      // Silent fail
    }
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- fetches sessions from server on mount, then polls every 15s
    fetchSessions();
    const interval = setInterval(fetchSessions, 15_000);
    return () => clearInterval(interval);
  }, [fetchSessions]);

  if (sessions.length === 0) return null;

  const email = user?.email;
  const mySessions = sessions.filter((s) => s.user_id === email);
  const otherSessions = sessions.filter((s) => s.user_id !== email);

  return (
    <>
      <div className="mx-2.5 my-2 border-t" />
      <Collapsible open={isActive || isExpanded} onOpenChange={setIsExpanded}>
        <CollapsibleTrigger asChild>
          <button className="flex w-full items-center gap-1.5 px-2.5 py-1.5 text-left">
            <SlackIcon />
            <span className="flex-1 text-xs font-medium uppercase tracking-wider text-muted-foreground">
              Slack
            </span>
            <ChevronRight
              className={`h-3 w-3 text-muted-foreground transition-transform ${
                isActive || isExpanded ? "rotate-90" : ""
              }`}
            />
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          {mySessions.length > 0 && (
            <>
              <span className="block px-2 pt-1.5 pb-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                Mis conversaciones
              </span>
              <SessionList sessions={mySessions} readOnly />
            </>
          )}
          {otherSessions.length > 0 && (
            <>
              <span className="block px-2 pt-2 pb-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                Otras conversaciones
              </span>
              <SessionList sessions={otherSessions} readOnly />
            </>
          )}
        </CollapsibleContent>
      </Collapsible>
    </>
  );
}
