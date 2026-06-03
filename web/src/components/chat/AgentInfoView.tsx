"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type { Agent, MultiAgentGroup, MCPServer, MCPTool, Session } from "@/lib/types";
import { getMCPSessions, getMCPTools, getMCPOAuth2LoginURL } from "@/lib/api";
import { useMCPContext } from "@/contexts/MCPContext";
import { useAgents } from "@/hooks/useAgents";
import { useUser } from "@/hooks/useUser";
import { useT } from "@/lib/i18n";
import { getEntityColor } from "@/lib/agent-colors";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { SessionItem } from "@/components/sessions/SessionItem";
import { Bot, ChevronDown, ChevronUp, Circle, KeyRound, Loader2, MessageSquarePlus, Server, Users, Wrench } from "lucide-react";

const VISIBLE_LIMIT = 10;

interface AgentInfoViewProps {
  mode: "agent" | "group" | "mcp";
  agent?: Agent | null;
  group?: MultiAgentGroup | null;
  mcpServer?: MCPServer | null;
  sessions: Session[];
  onNewConversation: () => void;
}

function CollapsibleSessionList({ label, items, readOnly }: { label?: string; items: Session[]; readOnly?: boolean }) {
  const [showAll, setShowAll] = useState(false);
  const t = useT();

  if (items.length === 0) return null;

  const hasMore = items.length > VISIBLE_LIMIT;
  const visible = showAll ? items : items.slice(0, VISIBLE_LIMIT);
  const hiddenCount = items.length - VISIBLE_LIMIT;

  return (
    <div className="w-full text-left">
      {label && (
        <span className="block px-1 pb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          {label}
        </span>
      )}
      <div className="space-y-0.5">
        {visible.map((session) => (
          <SessionItem key={session.session_id} session={session} readOnly={readOnly} />
        ))}
      </div>
      {hasMore && (
        <button
          onClick={() => setShowAll(!showAll)}
          className="flex w-full items-center gap-1 px-2 py-1.5 text-[11px] text-muted-foreground transition-colors hover:text-foreground"
        >
          {showAll ? (
            <>
              <ChevronUp className="h-3 w-3" />
              {t("sidebar.showLess")}
            </>
          ) : (
            <>
              <ChevronDown className="h-3 w-3" />
              {t("sidebar.showMore", { count: String(hiddenCount) })}
            </>
          )}
        </button>
      )}
    </div>
  );
}

export function AgentInfoView({ mode, agent, group, mcpServer, sessions, onNewConversation }: AgentInfoViewProps) {
  const { agents } = useAgents();
  const { user } = useUser();
  const t = useT();

  const { mySessions, otherSessions } = useMemo(() => {
    const email = user?.email;
    if (!email) return { mySessions: sessions, otherSessions: [] };
    return {
      mySessions: sessions.filter((s) => s.user_id === email),
      otherSessions: sessions.filter((s) => s.user_id !== email),
    };
  }, [sessions, user?.email]);

  if (mode === "agent" && agent) {
    const color = getEntityColor(agent.id);
    return (
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
        <div className="mx-auto flex w-full max-w-md flex-col items-center gap-6 p-8 text-center">
          <div
            className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl text-2xl font-bold text-white"
            style={{ background: `linear-gradient(135deg, ${color.avatarFrom}, ${color.avatarTo})` }}
          >
            {agent.name.charAt(0).toUpperCase()}
          </div>
          <div>
            <h2 className="text-xl font-semibold">{agent.name}</h2>
            {agent.description && (
              <p className="mt-1.5 text-sm text-muted-foreground">{agent.description}</p>
            )}
          </div>
          <div className="flex flex-wrap items-center justify-center gap-2">
            {agent.category && (
              <Badge variant="secondary" className="text-xs">
                {agent.category}
              </Badge>
            )}
            <Badge variant="outline" className="text-xs">
              {agent.protocol.toUpperCase()}
            </Badge>
            <Badge variant="outline" className="gap-1 text-xs">
              <Circle
                className={`h-1.5 w-1.5 fill-current ${
                  agent.status === "healthy" ? "text-emerald-500" : "text-muted-foreground"
                }`}
              />
              {agent.status === "healthy" ? t("info.healthy") : agent.status}
            </Badge>
          </div>
          <Button onClick={onNewConversation} className="gap-2">
            <MessageSquarePlus className="h-4 w-4" />
            {t("info.newConversation")}
          </Button>
          {sessions.length > 0 && (
            <div className="w-full border-t pt-4">
              <CollapsibleSessionList items={sessions} />
            </div>
          )}
        </div>
      </div>
    );
  }

  if (mode === "group" && group) {
    const groupAgents = group.agentIds
      .map((id) => agents.find((a) => a.id === id))
      .filter(Boolean) as Agent[];

    return (
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
        <div className="mx-auto flex w-full max-w-md flex-col items-center gap-6 p-8 text-center">
          <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-violet-500/20 to-fuchsia-500/20">
            <Users className="h-8 w-8 text-violet-600 dark:text-violet-400" />
          </div>
          <div>
            <h2 className="text-xl font-semibold">{group.name}</h2>
          </div>
          <div className="flex flex-col gap-2 w-full">
            {groupAgents.map((a) => {
              const c = getEntityColor(a.id);
              return (
                <div
                  key={a.id}
                  className="flex items-center gap-2.5 rounded-lg border px-3 py-2"
                >
                  <div
                    className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-white"
                    style={{ background: `linear-gradient(135deg, ${c.avatarFrom}, ${c.avatarTo})` }}
                  >
                    <Bot className="h-3 w-3" />
                  </div>
                  <span className="text-sm font-medium">{a.name}</span>
                  <Badge variant="outline" className="ml-auto text-[10px]">
                    {a.protocol.toUpperCase()}
                  </Badge>
                  <Circle
                    className={`h-2 w-2 shrink-0 fill-current ${
                      a.status === "healthy" ? "text-emerald-500" : "text-muted-foreground"
                    }`}
                  />
                </div>
              );
            })}
          </div>
          <Button onClick={onNewConversation} className="gap-2">
            <MessageSquarePlus className="h-4 w-4" />
            {t("info.newConversation")}
          </Button>
          <div className="w-full border-t pt-4 space-y-3">
            <div className="w-full text-left">
              <span className="block px-1 pb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                {t("sessions.myConversations")}
              </span>
              {mySessions.length > 0 ? (
                <CollapsibleSessionList items={mySessions} />
              ) : (
                <p className="px-2 py-1 text-[11px] text-muted-foreground">{t("sessions.noConversations")}</p>
              )}
            </div>
            <div className="w-full text-left">
              <span className="block px-1 pb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                {t("sessions.otherConversations")}
              </span>
              {otherSessions.length > 0 ? (
                <CollapsibleSessionList items={otherSessions} readOnly />
              ) : (
                <p className="px-2 py-1 text-[11px] text-muted-foreground">{t("sessions.noConversations")}</p>
              )}
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (mode === "mcp" && mcpServer) {
    return <MCPInfoView mcpServer={mcpServer} onNewConversation={onNewConversation} />;
  }

  return null;
}

function MCPInfoView({ mcpServer, onNewConversation }: { mcpServer: MCPServer; onNewConversation: () => void }) {
  const t = useT();
  const { user } = useUser();
  const { selectMCPSession, sessionVersion } = useMCPContext();
  const color = getEntityColor(mcpServer.id);

  // Fetch sessions for this MCP server
  const [mcpSessions, setMcpSessions] = useState<Session[]>([]);
  const refreshMcpSessions = useCallback(async () => {
    try {
      const fetched = await getMCPSessions(mcpServer.id);
      fetched.sort((a, b) => b.last_activity - a.last_activity);
      setMcpSessions(fetched);
    } catch {
      setMcpSessions([]);
    }
  }, [mcpServer.id]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- fetches sessions from server
    refreshMcpSessions();
  }, [refreshMcpSessions, sessionVersion]);

  // Tools: expandable section
  const [toolsExpanded, setToolsExpanded] = useState(false);
  const [tools, setTools] = useState<MCPTool[]>([]);
  const [isLoadingTools, setIsLoadingTools] = useState(false);
  const [toolsLoaded, setToolsLoaded] = useState(false);

  const handleToggleTools = () => {
    const next = !toolsExpanded;
    setToolsExpanded(next);
    if (next && !toolsLoaded) {
      setIsLoadingTools(true);
      getMCPTools(mcpServer.id)
        .then((result) => { setTools(result); setToolsLoaded(true); })
        .catch(() => setTools([]))
        .finally(() => setIsLoadingTools(false));
    }
  };

  // Reset tools state when server changes
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reset local state on server change
    setToolsExpanded(false);
    setToolsLoaded(false);
    setTools([]);
  }, [mcpServer.id]);

  // Split sessions: mine vs others
  const { mySessions, otherSessions } = useMemo(() => {
    const email = user?.email;
    if (!email) return { mySessions: mcpSessions, otherSessions: [] };
    return {
      mySessions: mcpSessions.filter((s) => s.user_id === email),
      otherSessions: mcpSessions.filter((s) => s.user_id !== email),
    };
  }, [mcpSessions, user?.email]);

  const formatRelativeDate = (ts: number) => {
    const d = new Date(ts * 1000);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    if (diffMins < 1) return t("time.now");
    if (diffMins < 60) return `${diffMins}m`;
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours}h`;
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 7) return `${diffDays}d`;
    return d.toLocaleDateString(t("pdf.dateLocale"), { day: "numeric", month: "short" });
  };

  const renderSessionList = (items: Session[]) => {
    if (items.length === 0) return <p className="px-2 py-1 text-[11px] text-muted-foreground">{t("sessions.noConversations")}</p>;
    return (
      <div className="space-y-0.5">
        {items.slice(0, VISIBLE_LIMIT).map((session) => (
          <button
            key={session.session_id}
            onClick={() => selectMCPSession(session.session_id)}
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-all hover:bg-accent/50 active:scale-[0.98]"
          >
            <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-emerald-500" />
            <span className="flex-1 truncate text-xs">{session.session_name}</span>
            <span className="text-[10px] text-muted-foreground shrink-0">
              {formatRelativeDate(session.last_activity)}
            </span>
          </button>
        ))}
        {items.length > VISIBLE_LIMIT && (
          <p className="px-2 py-1 text-[11px] text-muted-foreground">
            {t("time.more", { count: String(items.length - VISIBLE_LIMIT) })}
          </p>
        )}
      </div>
    );
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <div className="mx-auto flex w-full max-w-md flex-col items-center gap-6 p-8 text-center">
        <div
          className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl text-white"
          style={{ background: `linear-gradient(135deg, ${color.avatarFrom}, ${color.avatarTo})` }}
        >
          <Server className="h-8 w-8" />
        </div>
        <div>
          <h2 className="text-xl font-semibold">{mcpServer.name}</h2>
          {mcpServer.description && (
            <p className="mt-1.5 text-sm text-muted-foreground">{mcpServer.description}</p>
          )}
        </div>
        <div className="flex flex-wrap items-center justify-center gap-2">
          <Badge variant="outline" className="text-xs">
            {mcpServer.transport.toUpperCase()}
          </Badge>
          <Badge variant="outline" className="gap-1 text-xs">
            <Circle
              className={`h-1.5 w-1.5 fill-current ${
                mcpServer.status === "connected" ? "text-emerald-500" : "text-red-500"
              }`}
            />
            {mcpServer.status === "connected" ? t("info.connected") : t("info.disconnected")}
          </Badge>
          <button onClick={handleToggleTools}>
            <Badge variant="secondary" className="gap-1 text-xs cursor-pointer hover:bg-secondary/80 transition-colors">
              <Wrench className="h-3 w-3" />
              {mcpServer.tool_count} {t("info.tools")}
              {toolsExpanded ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
            </Badge>
          </button>
        </div>

        {/* Expandable tools section */}
        {toolsExpanded && (
          <div className="w-full text-left">
            {isLoadingTools ? (
              <div className="flex items-center justify-center py-4">
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              </div>
            ) : tools.length === 0 ? (
              <p className="py-2 text-center text-xs text-muted-foreground">{t("mcp.noTools")}</p>
            ) : (
              <div className="space-y-1.5">
                {tools.map((tool) => (
                  <div key={tool.name} className="rounded-lg border bg-muted/30 px-3 py-2">
                    <div className="flex items-center gap-2">
                      <Wrench className="h-3 w-3 shrink-0 text-muted-foreground" />
                      <span className="text-xs font-medium">{tool.name}</span>
                    </div>
                    {tool.description && (
                      <p className="mt-0.5 text-[11px] text-muted-foreground">{tool.description}</p>
                    )}
                    {tool.inputSchema &&
                      typeof tool.inputSchema.properties === "object" &&
                      tool.inputSchema.properties != null && (
                      <div className="mt-1 flex flex-wrap gap-1">
                        {Object.keys(tool.inputSchema.properties as Record<string, unknown>).map((param) => (
                          <Badge key={param} variant="secondary" className="text-[10px]">
                            {param}
                          </Badge>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {mcpServer.auth_type === "oauth2" && !mcpServer.oauth2_connected ? (
          <Button
            onClick={() => {
              const popup = window.open(
                getMCPOAuth2LoginURL(mcpServer.id, window.location.href),
                "_blank",
                "width=600,height=700"
              );
              const check = setInterval(() => {
                if (popup && popup.closed) {
                  clearInterval(check);
                  window.location.reload();
                }
              }, 500);
            }}
            className="gap-2"
          >
            <KeyRound className="h-4 w-4" />
            Conectar con OAuth2
          </Button>
        ) : (
          <Button onClick={onNewConversation} disabled={mcpServer.status !== "connected"} className="gap-2">
            <MessageSquarePlus className="h-4 w-4" />
            {t("info.newConversation")}
          </Button>
        )}

        {/* Sessions section */}
        {mcpSessions.length > 0 && (
          <div className="w-full border-t pt-4 space-y-3">
            <div className="w-full text-left">
              <span className="block px-1 pb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                {t("sessions.myConversations")}
              </span>
              {renderSessionList(mySessions)}
            </div>
            {otherSessions.length > 0 && (
              <div className="w-full text-left">
                <span className="block px-1 pb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                  {t("sessions.otherConversations")}
                </span>
                {renderSessionList(otherSessions)}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
