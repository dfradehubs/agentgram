"use client";

import { memo, useState } from "react";
import type { Agent } from "@/lib/types";
import { getEntityColor } from "@/lib/agent-colors";
import { useAgents } from "@/hooks/useAgents";
import { useSessions } from "@/hooks/useSessions";
import { useMCPContext } from "@/contexts/MCPContext";
import { SessionList } from "../sessions/SessionList";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { ChevronRight } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface AgentItemProps {
  agent: Agent;
}

export const AgentItem = memo(function AgentItem({ agent }: AgentItemProps) {
  const { currentAgent, selectAgent } = useAgents();
  const { sessions, currentSession, clearCurrentSession } = useSessions();
  const { selectMCPServer } = useMCPContext();
  const [isExpanded, setIsExpanded] = useState(false);

  // Don't highlight individual agent when a multi-agent session is active
  const isSelected = currentAgent?.id === agent.id && !currentSession?.is_multi_agent;
  const agentSessions = isSelected ? sessions : [];

  const handleClick = () => {
    if (isSelected) {
      setIsExpanded(!isExpanded);
    } else {
      selectMCPServer(null); // Deselect MCP server
      selectAgent(agent.id);
      clearCurrentSession(); // clears session, pendingMultiAgentIds, bumps resetKey
      setIsExpanded(true);
    }
  };

  return (
    <Collapsible open={isSelected && isExpanded} className="mb-0.5" role="option" aria-selected={isSelected}>
      <CollapsibleTrigger asChild>
        <button
          onClick={handleClick}
          aria-expanded={isSelected && isExpanded}
          className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm transition-all active:scale-[0.98] ${
            isSelected
              ? "bg-accent text-accent-foreground"
              : "text-foreground hover:bg-accent/50"
          }`}
        >
          <div
            className="relative flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-xs font-medium text-white"
            style={{ background: `linear-gradient(135deg, ${getEntityColor(agent.id).avatarFrom}, ${getEntityColor(agent.id).avatarTo})` }}
            aria-hidden="true"
          >
            {agent.name.charAt(0).toUpperCase()}
            <span
              className={`absolute -bottom-0.5 -right-0.5 h-2 w-2 rounded-full border-[1.5px] border-background ${
                agent.status === "healthy"
                  ? "bg-emerald-500"
                  : "bg-muted-foreground/50"
              }`}
              aria-label={agent.status === "healthy" ? "Online" : "Offline"}
            />
          </div>
          <div className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium leading-tight">
              {agent.name}
            </span>
            {agent.description && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="block truncate text-xs leading-tight text-muted-foreground">
                    {agent.description}
                  </span>
                </TooltipTrigger>
                <TooltipContent side="right" className="max-w-xs">
                  {agent.description}
                </TooltipContent>
              </Tooltip>
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
          <SessionList sessions={agentSessions} />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
});
