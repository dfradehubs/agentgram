"use client";

import { useState } from "react";
import { useAgents } from "@/hooks/useAgents";
import { useSessions } from "@/hooks/useSessions";
import { useMCPContext } from "@/contexts/MCPContext";
import { useAgentContext } from "@/contexts/AgentContext";
import { getEntityColor } from "@/lib/agent-colors";
import { useT } from "@/lib/i18n";
import { AgentItem } from "./AgentItem";
import { AgentItemSkeleton } from "./AgentItemSkeleton";
import { GroupItem } from "../sessions/GroupItem";
import { SlackSessionsSection } from "../sessions/SlackSessionsSection";
import { MultiMCPItem } from "../mcp/MultiMCPItem";
import { MCPSessionList } from "../mcp/MCPSessionList";
import { CreateMultiMCPDialog } from "../mcp/CreateMultiMCPDialog";
import { CreateMultiAgentDialog } from "../sessions/CreateMultiAgentDialog";
import { Badge } from "@/components/ui/badge";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { ChevronRight, Plus, RefreshCw, Server, Users } from "lucide-react";
import { reconnectMCPServer } from "@/lib/api";
import { toast } from "sonner";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

export function AgentList() {
  const { agents, isLoading, error } = useAgents();
  const { clearCurrentSession, multiAgentGroups } = useSessions();
  const {
    mcpServers,
    currentMCPServer,
    selectMCPServer,
    multiMCPSessions,
    refreshServers,
  } = useMCPContext();
  const { selectAgent } = useAgentContext();
  const [showMultiMCPDialog, setShowMultiMCPDialog] = useState(false);
  const [showMultiAgentDialog, setShowMultiAgentDialog] = useState(false);
  const [isMCPExpanded, setIsMCPExpanded] = useState(false);
  const [isMultiMCPExpanded, setIsMultiMCPExpanded] = useState(false);
  const [expandedMCPServer, setExpandedMCPServer] = useState<string | null>(null);
  const [reconnectingMCP, setReconnectingMCP] = useState<string | null>(null);
  const t = useT();

  if (isLoading) {
    return (
      <div className="space-y-0.5 p-1" aria-busy="true" role="status" aria-label={t("a11y.loading")}>
        {[1, 2, 3, 4].map((i) => (
          <AgentItemSkeleton key={i} />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-3" role="alert" aria-live="assertive">
        <p className="text-xs text-destructive">{error}</p>
      </div>
    );
  }

  if (agents.length === 0 && mcpServers.length === 0) {
    return (
      <div className="p-3">
        <p className="text-xs text-muted-foreground">{t("agents.noAgents")}</p>
      </div>
    );
  }

  const handleMCPServerClick = (serverId: string) => {
    // Reset wantsNewChat + clear agent session state so the MCP info view shows
    clearCurrentSession();
    selectAgent("");
    selectMCPServer(serverId);
    setExpandedMCPServer((prev) => (prev === serverId ? null : serverId));
  };

  return (
    <div className="p-1" role="listbox" aria-label={t("sidebar.agentList")}>
      {agents.map((agent) => (
        <AgentItem key={agent.id} agent={agent} />
      ))}

      {/* Slack Sessions (independent concept — not a group) */}
      <SlackSessionsSection />

      {/* Multi-agent groups */}
      {agents.length >= 2 && (
        <>
          <div className="mx-2.5 my-2 border-t" />
          <div className="flex items-center justify-between px-2.5 py-1">
            <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              {t("multiAgent.groups")}
            </span>
            <button
              onClick={() => setShowMultiAgentDialog(true)}
              className="rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground"
              aria-label={t("multiAgent.newGroup")}
            >
              <Plus className="h-3.5 w-3.5" />
            </button>
          </div>
          {multiAgentGroups.map((group) => (
            <GroupItem key={group.id} group={group} />
          ))}
          {multiAgentGroups.length === 0 && (
            <button
              className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm transition-all active:scale-[0.98] hover:bg-accent/50"
              onClick={() => setShowMultiAgentDialog(true)}
            >
              <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-gradient-to-br from-violet-500/20 to-fuchsia-500/20">
                <Users className="h-3.5 w-3.5 text-violet-600 dark:text-violet-400" />
              </div>
              <span className="flex-1 truncate text-sm text-muted-foreground">{t("multiAgent.newGroup")}</span>
              <Plus className="h-3.5 w-3.5 text-muted-foreground" />
            </button>
          )}
        </>
      )}

      {mcpServers.length > 0 && (
        <>
          <div className="mx-2.5 my-2 border-t" />
          <Collapsible open={isMCPExpanded} onOpenChange={setIsMCPExpanded}>
            <CollapsibleTrigger asChild>
              <button className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-left">
                <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  MCP
                </span>
                <ChevronRight
                  className={`ml-auto h-3 w-3 text-muted-foreground transition-transform ${
                    isMCPExpanded ? "rotate-90" : ""
                  }`}
                />
              </button>
            </CollapsibleTrigger>
            <CollapsibleContent>
              {mcpServers.map((server) => (
                <Collapsible
                  key={server.id}
                  open={expandedMCPServer === server.id}
                  onOpenChange={(open) =>
                    setExpandedMCPServer(open ? server.id : null)
                  }
                >
                  <CollapsibleTrigger asChild>
                    <button
                      onClick={() => handleMCPServerClick(server.id)}
                      className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm transition-all active:scale-[0.98] hover:bg-accent/50 ${
                        currentMCPServer?.id === server.id
                          ? "bg-accent text-accent-foreground"
                          : "text-foreground"
                      }`}
                    >
                      <div className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-md ${getEntityColor(server.id).iconBg}`}>
                        <Server className={`h-3.5 w-3.5 ${getEntityColor(server.id).iconFg}`} />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5">
                          <span
                            className={`h-1.5 w-1.5 shrink-0 rounded-full ${
                              server.status === "connected"
                                ? "bg-green-500"
                                : server.status === "error"
                                  ? "bg-red-500"
                                  : "bg-gray-400"
                            }`}
                          />
                          <span className="truncate text-sm font-medium">{server.name}</span>
                        </div>
                        <div className="flex items-center gap-1">
                          {server.status === "error" && server.status_error && (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span className="block truncate text-[10px] text-destructive">
                                  {server.status_error}
                                </span>
                              </TooltipTrigger>
                              <TooltipContent side="right" className="max-w-xs">
                                {server.status_error}
                              </TooltipContent>
                            </Tooltip>
                          )}
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <button
                                onClick={async (e) => {
                                  e.stopPropagation();
                                  setReconnectingMCP(server.id);
                                  try {
                                    await reconnectMCPServer(server.id);
                                    await refreshServers();
                                    toast.success(t("mcp.reconnected", { name: server.name }));
                                  } catch {
                                    toast.error(t("mcp.reconnectError"));
                                  } finally {
                                    setReconnectingMCP(null);
                                  }
                                }}
                                disabled={reconnectingMCP === server.id}
                                className="shrink-0 rounded p-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                              >
                                <RefreshCw className={`h-3 w-3 ${reconnectingMCP === server.id ? "animate-spin" : ""}`} />
                              </button>
                            </TooltipTrigger>
                            <TooltipContent side="right">
                              Reconectar (detectar tools nuevas)
                            </TooltipContent>
                          </Tooltip>
                        </div>
                        {server.status !== "error" && server.description && (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span className="block truncate text-xs text-muted-foreground">
                                {server.description}
                              </span>
                            </TooltipTrigger>
                            <TooltipContent side="right" className="max-w-xs">
                              {server.description}
                            </TooltipContent>
                          </Tooltip>
                        )}
                      </div>
                      <ChevronRight
                        className={`h-3 w-3 shrink-0 text-muted-foreground transition-transform ${
                          expandedMCPServer === server.id ? "rotate-90" : ""
                        }`}
                      />
                    </button>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <div className="ml-3 mt-0.5 border-l pl-3">
                      <MCPSessionList serverId={server.id} />
                    </div>
                  </CollapsibleContent>
                </Collapsible>
              ))}

              {/* Multi-MCP section */}
              {mcpServers.length >= 2 && (
                <Collapsible open={isMultiMCPExpanded} onOpenChange={setIsMultiMCPExpanded}>
                  <CollapsibleTrigger asChild>
                    <button className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm text-foreground transition-all active:scale-[0.98] hover:bg-accent/50">
                      <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md rainbow-pill">
                        <Users className="h-3.5 w-3.5 text-white" />
                      </div>
                      <span className="flex-1 truncate text-sm font-medium">Multi-MCP</span>
                      {multiMCPSessions.length > 0 && (
                        <Badge variant="secondary" className="h-5 px-1.5 text-[10px]">
                          {multiMCPSessions.length}
                        </Badge>
                      )}
                      <ChevronRight
                        className={`h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform ${
                          isMultiMCPExpanded ? "rotate-90" : ""
                        }`}
                      />
                    </button>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <div className="ml-3 mt-0.5 border-l pl-3">
                      <button
                        className="flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-xs text-muted-foreground transition-all active:scale-[0.98] hover:bg-accent/50 hover:text-foreground"
                        onClick={() => setShowMultiMCPDialog(true)}
                      >
                        <Plus className="h-3.5 w-3.5" />
                        {t("agents.newSession")}
                      </button>

                      {multiMCPSessions.map((session) => (
                        <MultiMCPItem
                          key={session.session_id}
                          session={session}
                        />
                      ))}
                    </div>
                  </CollapsibleContent>
                </Collapsible>
              )}
            </CollapsibleContent>
          </Collapsible>
        </>
      )}

      <CreateMultiMCPDialog
        isOpen={showMultiMCPDialog}
        onClose={() => setShowMultiMCPDialog(false)}
      />

      <CreateMultiAgentDialog
        isOpen={showMultiAgentDialog}
        onClose={() => setShowMultiAgentDialog(false)}
      />
    </div>
  );
}
