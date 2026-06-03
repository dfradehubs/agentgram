"use client";

import { memo, useState, useEffect, useRef, useCallback } from "react";
import type { Session } from "@/lib/types";
import { useSessions } from "@/hooks/useSessions";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Loader2, MoreHorizontal, Pencil, Share2, Trash2, Undo2 } from "lucide-react";
import { toast } from "sonner";
import { useT } from "@/lib/i18n";
import { useBackgroundStreamContext } from "@/contexts/BackgroundStreamContext";
import { useReadState } from "@/hooks/useReadState";
import { shareSession } from "@/lib/api";
import { useAgentContext } from "@/contexts/AgentContext";

const UNDO_TIMEOUT_MS = 5000;

interface SessionItemProps {
  session: Session;
  readOnly?: boolean;
}

function formatRelativeDate(ts: number): string {
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
}

export const SessionItem = memo(function SessionItem({ session, readOnly }: SessionItemProps) {
  const { currentSession, selectSession, renameSession, deleteSession } =
    useSessions();
  const { currentAgent } = useAgentContext();
  const { isSessionRunning, stopStream } = useBackgroundStreamContext();
  const { getUnreadCount, markAsRead } = useReadState();
  const t = useT();
  const unreadCount = getUnreadCount(session.session_id, session.message_count);
  const [isEditing, setIsEditing] = useState(false);
  const [editName, setEditName] = useState(session.session_name);
  const [pendingDelete, setPendingDelete] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const isSelected = currentSession?.session_id === session.session_id;
  const running = isSessionRunning(session.session_id);

  // Auto-mark as read when this session is selected (including when message_count updates via polling)
  useEffect(() => {
    if (isSelected) {
      markAsRead(session.session_id, session.message_count);
    }
  }, [isSelected, session.session_id, session.message_count, markAsRead]);

  // Clean up timer on unmount
  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  const handleClick = () => {
    if (!isEditing) {
      markAsRead(session.session_id, session.message_count);
      // For Slack sessions, pass the first agent ID so the session can load without a pre-selected agent
      const slackAgentId = session.source === "slack" && session.agent_ids?.length
        ? session.agent_ids[0]
        : undefined;
      selectSession(session.session_id, slackAgentId);
    }
  };

  const handleRename = async () => {
    if (editName.trim() && editName !== session.session_name) {
      try {
        await renameSession(session.session_id, editName.trim());
      } catch {
        setEditName(session.session_name);
      }
    }
    setIsEditing(false);
  };

  const commitDelete = useCallback(async () => {
    try {
      stopStream(session.session_id);
      await deleteSession(session.session_id);
    } catch (err) {
      console.error("Failed to delete session:", err);
      toast.error(t("sessions.deleteError"));
      setPendingDelete(false);
    }
  }, [session.session_id, deleteSession, stopStream, t]);

  const handleShare = async () => {
    if (!currentAgent) return;
    try {
      const resp = await shareSession(currentAgent.id, session.session_id);
      const url = `${window.location.origin}${resp.url}`;
      await navigator.clipboard.writeText(url);
      toast.success(t("sessions.shareSuccess"));
    } catch {
      toast.error(t("sessions.shareError"));
    }
  };

  const handleDelete = () => {
    setPendingDelete(true);
    timerRef.current = setTimeout(() => {
      timerRef.current = null;
      commitDelete();
    }, UNDO_TIMEOUT_MS);
  };

  const handleUndo = () => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    setPendingDelete(false);
  };

  // Undo bar view
  if (pendingDelete) {
    return (
      <div
        role="listitem"
        className="group relative flex items-center rounded-md px-2 py-1.5 overflow-hidden bg-destructive/10"
      >
        {/* Shrinking progress bar */}
        <div
          className="absolute bottom-0 left-0 h-0.5 bg-destructive/40"
          style={{
            animation: `shrink-bar ${UNDO_TIMEOUT_MS}ms linear forwards`,
          }}
        />
        <span className="flex-1 truncate text-xs text-muted-foreground">
          {t("sessions.deleted")}
        </span>
        <button
          onClick={handleUndo}
          className="ml-2 flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-xs font-medium text-primary transition-colors hover:bg-accent"
        >
          <Undo2 className="h-3 w-3" />
          {t("sessions.undo")}
        </button>
      </div>
    );
  }

  return (
    <div
      role="listitem"
      className={`group flex items-center rounded-md px-2 py-1.5 transition-all active:scale-[0.98] ${
        isSelected
          ? "bg-accent"
          : "hover:bg-accent/50"
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
          onClick={handleClick}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
        >
          <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${
            running ? "bg-purple-400 animate-pulse" : "bg-emerald-500"
          }`} />
          <span className="truncate text-xs text-foreground flex-1">
            {session.source === "slack" && (
              <span className="mr-1 inline-block rounded bg-purple-500/20 px-1 py-0 text-[9px] font-medium text-purple-400 align-middle">Slack</span>
            )}
            {session.session_name}
          </span>
          {running && (
            <Loader2 className="h-3 w-3 animate-spin text-primary shrink-0" />
          )}
          {!running && !isSelected && unreadCount > 0 && (
            <span className="flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-medium text-primary-foreground shrink-0">
              {unreadCount > 99 ? "99+" : unreadCount}
            </span>
          )}
          <span className="text-[10px] text-muted-foreground shrink-0">
            {formatRelativeDate(session.last_activity)}
          </span>
        </button>
      )}

      {!isEditing && !readOnly && (
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
            <DropdownMenuItem onClick={handleShare}>
              <Share2 className="mr-2 h-3.5 w-3.5" />
              {t("sessions.share")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={handleDelete}
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
});
