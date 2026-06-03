"use client";

import { useState, useEffect, useCallback } from "react";
import type { Session } from "@/lib/types";
import { getMCPSessions, renameMCPSession as apiRenameMCPSession, deleteMCPSession as apiDeleteMCPSession } from "@/lib/api";
import { useMCPContext } from "@/contexts/MCPContext";
import { useBackgroundStreamContext } from "@/contexts/BackgroundStreamContext";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ChevronDown, ChevronUp, Loader2, MoreHorizontal, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useT } from "@/lib/i18n";

const VISIBLE_LIMIT = 10;

interface MCPSessionListProps {
  serverId: string;
}

export function MCPSessionList({ serverId }: MCPSessionListProps) {
  const {
    currentMCPSession,
    selectMCPSession,
    sessionVersion,
  } = useMCPContext();
  const { isSessionRunning, stopStream } = useBackgroundStreamContext();

  const t = useT();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [showAll, setShowAll] = useState(false);

  const refreshSessions = useCallback(async () => {
    try {
      const fetched = await getMCPSessions(serverId);
      fetched.sort((a, b) => b.last_activity - a.last_activity);
      setSessions(fetched);
    } catch {
      setSessions([]);
    }
  }, [serverId]);

  // Fetch sessions on mount, when serverId changes, or when sessionVersion bumps (new session created)
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- fetches sessions from server
    refreshSessions();
  }, [refreshSessions, sessionVersion]);

  const handleDelete = useCallback(async (sessionId: string) => {
    stopStream(sessionId);
    await apiDeleteMCPSession(serverId, sessionId);
    if (currentMCPSession?.session_id === sessionId) {
      selectMCPSession(null);
    }
    await refreshSessions();
    toast.success(t("sessions.deleted"));
  }, [serverId, currentMCPSession, selectMCPSession, stopStream, refreshSessions, t]);

  const handleRename = useCallback(async (sessionId: string, name: string) => {
    await apiRenameMCPSession(serverId, sessionId, name);
    await refreshSessions();
  }, [serverId, refreshSessions]);

  const hasMore = sessions.length > VISIBLE_LIMIT;
  const visibleSessions = showAll ? sessions : sessions.slice(0, VISIBLE_LIMIT);

  return (
    <div className="space-y-0.5">
      <button
        className="flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
        onClick={() => selectMCPSession(null)}
      >
        <Plus className="h-3.5 w-3.5" />
        {t("agents.newSession")}
      </button>

      {visibleSessions.map((session) => (
        <MCPSessionItem
          key={session.session_id}
          session={session}
          isSelected={currentMCPSession?.session_id === session.session_id}
          isRunning={isSessionRunning(session.session_id)}
          onSelect={() => selectMCPSession(session.session_id)}
          onDelete={() => handleDelete(session.session_id)}
          onRename={(name) => handleRename(session.session_id, name)}
        />
      ))}

      {hasMore && (
        <button
          onClick={() => setShowAll(!showAll)}
          className="flex w-full items-center gap-1 px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:text-foreground"
        >
          {showAll ? (
            <>
              <ChevronUp className="h-3 w-3" />
              {t("mcp.showLess")}
            </>
          ) : (
            <>
              <ChevronDown className="h-3 w-3" />
              {t("mcp.moreSessions", { count: sessions.length - VISIBLE_LIMIT })}
            </>
          )}
        </button>
      )}

      {sessions.length === 0 && (
        <p className="px-2 py-1 text-xs text-muted-foreground">
          {t("mcp.noConversations")}
        </p>
      )}
    </div>
  );
}

function MCPSessionItem({
  session,
  isSelected,
  isRunning,
  onSelect,
  onDelete,
  onRename,
}: {
  session: Session;
  isSelected: boolean;
  isRunning: boolean;
  onSelect: () => void;
  onDelete: () => void;
  onRename: (name: string) => void;
}) {
  const t = useT();
  const [isEditing, setIsEditing] = useState(false);
  const [editName, setEditName] = useState(session.session_name);

  const handleRename = async () => {
    if (editName.trim() && editName !== session.session_name) {
      try {
        await onRename(editName.trim());
      } catch {
        setEditName(session.session_name);
      }
    }
    setIsEditing(false);
  };

  const formatRelativeDate = (ts: number) => {
    const d = new Date(ts * 1000);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    if (diffMins < 1) return "now";
    if (diffMins < 60) return `${diffMins}m`;
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours}h`;
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 7) return `${diffDays}d`;
    return d.toLocaleDateString("en", { day: "numeric", month: "short" });
  };

  return (
    <div
      className={`group flex items-center rounded-md px-2 py-1.5 ${
        isSelected ? "bg-accent" : "hover:bg-accent/50"
      }`}
    >
      {isEditing ? (
        <input
          type="text"
          value={editName}
          onChange={(e) => setEditName(e.target.value)}
          onBlur={handleRename}
          onKeyDown={(e) => {
            if (e.key === "Enter") handleRename();
            if (e.key === "Escape") {
              setEditName(session.session_name);
              setIsEditing(false);
            }
          }}
          aria-label={t("sessions.nameLabel")}
          className="flex-1 rounded border border-ring bg-background px-1.5 py-0.5 text-xs"
          autoFocus
        />
      ) : (
        <button
          onClick={onSelect}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
        >
          <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${
            isRunning ? "bg-purple-400 animate-pulse" : "bg-emerald-500"
          }`} />
          <span className="truncate text-xs text-foreground flex-1">
            {session.session_name}
          </span>
          {isRunning && (
            <Loader2 className="h-3 w-3 animate-spin text-primary shrink-0" />
          )}
          <span className="text-[10px] text-muted-foreground shrink-0">
            {formatRelativeDate(session.last_activity)}
          </span>
        </button>
      )}

      {!isEditing && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button aria-label={t("sessions.options")} className="ml-1 shrink-0 rounded p-0.5 opacity-0 hover:bg-accent group-hover:opacity-100">
              <MoreHorizontal className="h-3.5 w-3.5 text-muted-foreground" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-32">
            <DropdownMenuItem onClick={() => setIsEditing(true)}>
              <Pencil className="mr-2 h-3.5 w-3.5" />
              {t("common.rename")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onDelete()}
              className="text-destructive focus:text-destructive"
            >
              <Trash2 className="mr-2 h-3.5 w-3.5" />
              {t("common.delete")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </div>
  );
}
