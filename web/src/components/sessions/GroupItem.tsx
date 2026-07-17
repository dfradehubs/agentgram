"use client";

import { useState, useCallback, useEffect } from "react";
import type { MultiAgentGroup, Session } from "@/lib/types";
import { useAgents } from "@/hooks/useAgents";
import { useSessions } from "@/hooks/useSessions";
import { useAgentContext } from "@/contexts/AgentContext";
import { getEntityColor } from "@/lib/agent-colors";
import { getGroupSessions } from "@/lib/api";
import { SessionList } from "./SessionList";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Bot, ChevronRight, Users } from "lucide-react";
import { useReadState } from "@/hooks/useReadState";
import { useUser } from "@/hooks/useUser";
import { selectGroupAnchorAgent } from "@/lib/groups";
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
  const { sessions, activeGroupId, selectGroup, newGroupConversation } = useSessions();
  const { selectAgent } = useAgentContext();
  const { getTotalUnread } = useReadState();
  const { user } = useUser();
  const [isExpanded, setIsExpanded] = useState(false);

  const isSelected = activeGroupId === group.id;

  // Poll group sessions independently when NOT selected (for unread badge)
  const [polledSessions, setPolledSessions] = useState<Session[]>([]);

  useEffect(() => {
    if (isSelected) return;
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

  // Group sessions are personal — the API already returns only the user's own.
  const groupSessions = isSelected ? sessions : polledSessions;

  const handleClick = () => {
    if (isSelected) {
      setIsExpanded(!isExpanded);
    } else {
      const anchorAgent = selectGroupAnchorAgent(group.agentIds, agents, !!user?.githubConnected);
      if (anchorAgent) selectAgent(anchorAgent);
      selectGroup(group.id);
      setIsExpanded(true);
    }
  };

  // New conversation within the group: reset chat but keep the group selected
  const handleNewGroupSession = useCallback(() => {
    newGroupConversation();
    selectGroup(group.id);
  }, [newGroupConversation, selectGroup, group.id]);

  const totalUnread = getTotalUnread(groupSessions);

  const groupAgentNames = group.agentIds
    .map((id) => agents.find((a) => a.id === id)?.name || id)
    .join(", ");

  return (
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
          <ChevronRight
            className={`h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform ${
              isSelected && isExpanded ? "rotate-90" : ""
            }`}
          />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="ml-3 mt-0.5 border-l pl-3">
          <SessionList sessions={groupSessions} onNewSession={handleNewGroupSession} />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
