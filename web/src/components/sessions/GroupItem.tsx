"use client";

import { useState, useCallback, useMemo, useEffect } from "react";
import type { MultiAgentGroup, Session } from "@/lib/types";
import { useAgents } from "@/hooks/useAgents";
import { useSessions } from "@/hooks/useSessions";
import { useAgentContext } from "@/contexts/AgentContext";
import { useUser } from "@/hooks/useUser";
import { getEntityColor } from "@/lib/agent-colors";
import { useT } from "@/lib/i18n";
import { getGroupSessions } from "@/lib/api";
import { SessionList } from "./SessionList";
import { EditGroupDialog } from "./EditGroupDialog";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Bot, ChevronRight, Pencil, Trash2, Users } from "lucide-react";
import { useReadState } from "@/hooks/useReadState";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface GroupItemProps {
  group: MultiAgentGroup;
}

export function GroupItem({ group }: GroupItemProps) {
  const { agents } = useAgents();
  const { sessions, activeGroupId, selectGroup, removeMultiAgentGroup, createMultiAgentSession } = useSessions();
  const { selectAgent } = useAgentContext();
  const { user } = useUser();
  const { getTotalUnread } = useReadState();
  const t = useT();
  const [isExpanded, setIsExpanded] = useState(false);
  const [editOpen, setEditOpen] = useState(false);

  const isSelected = activeGroupId === group.id;

  // Poll group sessions independently when NOT selected (for unread badge)
  const [polledSessions, setPolledSessions] = useState<Session[]>([]);

  useEffect(() => {
    if (isSelected) return;
    // Initial fetch + poll every 15s
    let cancelled = false;
    const fetchSessions = () => {
      getGroupSessions(group.id)
        .then((s) => { if (!cancelled) setPolledSessions(s); })
        .catch(() => {});
    };
    fetchSessions();
    const interval = setInterval(fetchSessions, 15_000);
    return () => { cancelled = true; clearInterval(interval); };
  }, [group.id, isSelected]);

  // When selected, use SessionContext sessions; otherwise use polled sessions
  const groupSessions = isSelected ? sessions : polledSessions;

  const { mySessions, otherSessions } = useMemo(() => {
    const email = user?.email;
    if (!email) return { mySessions: groupSessions, otherSessions: [] };
    return {
      mySessions: groupSessions.filter((s) => s.user_id === email),
      otherSessions: groupSessions.filter((s) => s.user_id !== email),
    };
  }, [groupSessions, user?.email]);

  const handleClick = () => {
    if (isSelected) {
      setIsExpanded(!isExpanded);
    } else {
      selectAgent(group.agentIds[0]);
      selectGroup(group.id);
      setIsExpanded(true);
    }
  };

  // New session within group: reset chat but keep group selected
  const handleNewGroupSession = useCallback(() => {
    createMultiAgentSession(group.agentIds);
    selectGroup(group.id);
  }, [createMultiAgentSession, selectGroup, group.agentIds, group.id]);

  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await removeMultiAgentGroup(group.id);
    } catch {
      // Error handled silently
    }
  };

  const handleEdit = (e: React.MouseEvent) => {
    e.stopPropagation();
    setEditOpen(true);
  };

  const totalUnread = getTotalUnread(groupSessions);

  const groupAgentNames = group.agentIds
    .map((id) => agents.find((a) => a.id === id)?.name || id)
    .join(", ");

  return (
    <>
      <Collapsible open={isSelected && isExpanded} className="mb-0.5">
        <CollapsibleTrigger asChild>
          <button
            onClick={handleClick}
            className={`group/item flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm transition-all active:scale-[0.98] ${
              isSelected
                ? "bg-accent text-accent-foreground"
                : "text-foreground hover:bg-accent/50"
            }`}
          >
            <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-gradient-to-br from-violet-500/20 to-fuchsia-500/20">
              <Users className="h-3.5 w-3.5 text-violet-600 dark:text-violet-400" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5">
                <span className="truncate text-sm font-medium leading-tight">
                  {group.name}
                </span>
                {totalUnread > 0 && (
                  <span className="flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-medium text-primary-foreground shrink-0">
                    {totalUnread > 99 ? "99+" : totalUnread}
                  </span>
                )}
              </div>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="block truncate text-xs leading-tight text-muted-foreground">
                    {groupAgentNames}
                  </span>
                </TooltipTrigger>
                <TooltipContent side="right" className="max-w-xs">
                  {groupAgentNames}
                </TooltipContent>
              </Tooltip>
            </div>
            <div className="flex items-center gap-1">
              {group.agentIds.slice(0, 3).map((id) => {
                const color = getEntityColor(id);
                return (
                  <div
                    key={id}
                    className="flex h-4 w-4 items-center justify-center rounded-full text-white"
                    style={{ background: `linear-gradient(135deg, ${color.avatarFrom}, ${color.avatarTo})` }}
                  >
                    <Bot className="h-2 w-2" />
                  </div>
                );
              })}
              {group.agentIds.length > 3 && (
                <span className="text-[10px] text-muted-foreground">+{group.agentIds.length - 3}</span>
              )}
            </div>
            <button
              onClick={handleEdit}
              className="h-5 w-5 shrink-0 items-center justify-center rounded text-muted-foreground opacity-0 transition-opacity hover:text-foreground group-hover/item:opacity-100 hidden group-hover/item:flex"
              aria-label={t("a11y.editGroup")}
            >
              <Pencil className="h-3 w-3" />
            </button>
            <button
              onClick={handleDelete}
              className="h-5 w-5 shrink-0 items-center justify-center rounded text-muted-foreground opacity-0 transition-opacity hover:text-destructive group-hover/item:opacity-100 hidden group-hover/item:flex"
              aria-label={t("a11y.deleteGroup")}
            >
              <Trash2 className="h-3 w-3" />
            </button>
            <ChevronRight
              className={`h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform ${
                isSelected && isExpanded ? "rotate-90" : ""
              }`}
            />
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="ml-3 mt-0.5 border-l pl-3">
            <span className="block px-2 pt-1.5 pb-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
              Mis conversaciones
            </span>
            <SessionList sessions={mySessions} onNewSession={handleNewGroupSession} />
            <span className="block px-2 pt-2 pb-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
              Otras conversaciones
            </span>
            <SessionList sessions={otherSessions} readOnly />
          </div>
        </CollapsibleContent>
      </Collapsible>
      <EditGroupDialog
        isOpen={editOpen}
        onClose={() => setEditOpen(false)}
        group={group}
      />
    </>
  );
}
