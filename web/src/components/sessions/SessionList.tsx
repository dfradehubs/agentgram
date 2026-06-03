"use client";

import { useState } from "react";
import type { Session } from "@/lib/types";
import { SessionItem } from "./SessionItem";
import { NewSessionButton } from "./NewSessionButton";
import { useSessions } from "@/hooks/useSessions";
import { useT } from "@/lib/i18n";
import { ChevronDown, ChevronUp } from "lucide-react";

const VISIBLE_LIMIT = 10;

interface SessionListProps {
  sessions: Session[];
  onNewSession?: () => void;
  readOnly?: boolean;
}

export function SessionList({ sessions, onNewSession, readOnly }: SessionListProps) {
  const { currentSession } = useSessions();
  const t = useT();
  const [showAll, setShowAll] = useState(false);

  const hasMore = sessions.length > VISIBLE_LIMIT;

  // Always include the selected session in the visible slice
  let visibleSessions: Session[];
  if (showAll || !hasMore) {
    visibleSessions = sessions;
  } else {
    const top = sessions.slice(0, VISIBLE_LIMIT);
    const selectedInTop = !currentSession || top.some((s) => s.session_id === currentSession.session_id);
    if (selectedInTop) {
      visibleSessions = top;
    } else {
      // Selected session is beyond the limit — include it at the end
      const selected = sessions.find((s) => s.session_id === currentSession.session_id);
      visibleSessions = selected ? [...top, selected] : top;
    }
  }

  return (
    <div className="space-y-0.5" role="list" aria-label={t("sessions.nameLabel")}>
      {!readOnly && <NewSessionButton onNewSession={onNewSession} />}
      {visibleSessions.map((session) => (
        <SessionItem key={session.session_id} session={session} readOnly={readOnly} />
      ))}
      {hasMore && (
        <button
          onClick={() => setShowAll(!showAll)}
          className="flex w-full items-center gap-1 px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:text-foreground"
          aria-expanded={showAll}
        >
          {showAll ? (
            <>
              <ChevronUp className="h-3 w-3" />
              {t("sidebar.showLess")}
            </>
          ) : (
            <>
              <ChevronDown className="h-3 w-3" />
              {t("sidebar.showMore", { count: String(sessions.length - VISIBLE_LIMIT) })}
            </>
          )}
        </button>
      )}
      {sessions.length === 0 && (
        <p className="px-2 py-1 text-[11px] text-muted-foreground">
          {t("sessions.noConversations")}
        </p>
      )}
    </div>
  );
}
