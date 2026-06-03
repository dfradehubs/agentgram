"use client";

import React, { useCallback, useEffect, useRef, useState } from "react";
import { useAgents } from "@/hooks/useAgents";
import { useUser } from "@/hooks/useUser";
import { useSessions } from "@/hooks/useSessions";
import { useChat } from "@/hooks/useChat";
import { useSessionSubscription } from "@/hooks/useSessionSubscription";
import { useMCPContext } from "@/contexts/MCPContext";
import { useConfig } from "@/contexts/ConfigContext";
import { usePreferencesContext } from "@/contexts/PreferencesContext";
import { useAgentContext } from "@/contexts/AgentContext";
import { useT } from "@/lib/i18n";
import { EmptyState } from "./EmptyState";
import { AgentInfoView } from "./AgentInfoView";
import { ChatHeader } from "./ChatHeader";
import { ChatMessages } from "./ChatMessages";
import { ChatInput } from "./ChatInput";
import { MCPToolsPanel } from "../mcp/MCPToolsPanel";
import { Button } from "@/components/ui/button";
import type { Attachment } from "@/lib/types";
import { reconnectMCPServer, getSession as fetchSession, shareSession, getMCPOAuth2LoginURL, ApiError } from "@/lib/api";
import { toast } from "sonner";
import {
  AlertTriangle,
  Github,
  KeyRound,
  Loader2,
  RefreshCw,
  Server,
} from "lucide-react";

const chatWidthClass = {
  normal: "max-w-3xl",
  wide: "max-w-5xl",
  full: "max-w-full px-4",
} as const;

const MAX_FILE_SIZE = 10 * 1024 * 1024; // 10MB

export function Chat() {
  const { agents, currentAgent } = useAgents();
  const { user, displayName } = useUser();
  const { focusKey } = useAgentContext();
  const { sessions, currentSession, sessionResetKey, refreshSessions, pendingMultiAgentIds, activeGroupId, multiAgentGroups, createNewSession, wantsNewChat, createMultiAgentSession, selectGroup, markSessionActive, hasMoreMessages, isLoadingMore, loadOlderMessages } = useSessions();
  const {
    currentMCPServer,
    currentMCPSession,
    currentMultiMCPSession,
    selectedMCPServerIds,
    isMCPMulti,
    mcpServers,
    refreshMCPSessions,
    refreshMultiMCPSessions,
    refreshServers,
  } = useMCPContext();
  const config = useConfig();
  const { preferences, updatePreference } = usePreferencesContext();
  const t = useT();
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const isNearBottomRef = useRef(true);
  const autoScrollRef = useRef(true);

  // Mode detection
  const isMCP = !!(currentMCPServer || isMCPMulti);
  const activeMCPSession = isMCPMulti ? currentMultiMCPSession : currentMCPSession;

  // Multi-agent detection: prefer group's agentIds, then session, then pending
  const activeGroup = activeGroupId ? multiAgentGroups.find(g => g.id === activeGroupId) : null;
  const multiAgentIds = activeGroup && activeGroup.agentIds.length > 0
    ? activeGroup.agentIds
    : currentSession?.is_multi_agent || currentSession?.source === "slack"
      ? (currentSession?.agent_ids || [])
      : pendingMultiAgentIds;
  const isSlackSession = currentSession?.source === "slack";
  const isMultiAgent = !isMCP && (multiAgentIds.length >= 2 || isSlackSession);
  const [selectedTargetAgentIds, setSelectedTargetAgentIds] = useState<string[]>([]);

  // Auto-select all agents in multi-agent mode
  useEffect(() => {
    if (isMultiAgent && multiAgentIds.length > 0) {
      setSelectedTargetAgentIds((prev) => {
        // Keep current selection if all selected agents are still valid
        const valid = prev.filter((id) => multiAgentIds.includes(id));
        return valid.length > 0 ? valid : [multiAgentIds[0]];
      });
    }
  }, [isMultiAgent, multiAgentIds]);

  const toggleTargetAgent = useCallback((agentId: string) => {
    setSelectedTargetAgentIds((prev) => {
      if (prev.includes(agentId)) {
        // Don't allow deselecting the last one
        if (prev.length <= 1) return prev;
        return prev.filter((id) => id !== agentId);
      }
      return [...prev, agentId];
    });
  }, []);

  // MCP model selection
  const defaultModel = config.available_models.find((m) => m.default) || config.available_models[0];
  const [selectedModelId, setSelectedModelId] = useState(defaultModel?.id || "");
  useEffect(() => {
    if (defaultModel && !selectedModelId) {
      setSelectedModelId(defaultModel.id);
    }
  }, [defaultModel, selectedModelId]);

  // MCP server IDs
  const mcpServerIds = isMCPMulti
    ? selectedMCPServerIds
    : currentMCPServer
      ? [currentMCPServer.id]
      : [];

  // State
  const [pendingAttachments, setPendingAttachments] = useState<Attachment[]>([]);
  const [copied, setCopied] = useState(false);
  const [copiedMessageIdx, setCopiedMessageIdx] = useState<number | null>(null);
  const [exporting, setExporting] = useState(false);
  const [showToolsPanel, setShowToolsPanel] = useState(false);
  const [isMCPReconnecting, setIsMCPReconnecting] = useState(false);
  const [isSharing, setIsSharing] = useState(false);

  // Determine chat params based on mode
  const chatAgentId = isMCP ? (mcpServerIds[0] || "") : (currentAgent?.id || "");
  const chatAgentName = isMCP ? (currentMCPServer?.name || "") : (currentAgent?.name || "");
  const chatSessionName = isMCP
    ? (activeMCPSession?.session_name || "")
    : (currentSession?.session_name || "");
  const chatSessionId = isMCP ? activeMCPSession?.session_id : currentSession?.session_id;
  const chatInitialMessages = isMCP
    ? (activeMCPSession?.messages || [])
    : (currentSession?.messages || []);

  const onSessionCreated = useCallback((sessionId?: string, sessionName?: string) => {
    if (isMCP) {
      if (isMCPMulti) {
        refreshMultiMCPSessions();
      } else {
        refreshMCPSessions();
      }
    } else {
      if (sessionId) markSessionActive(sessionId, sessionName);
      refreshSessions();
    }
  }, [isMCP, isMCPMulti, refreshMCPSessions, refreshMultiMCPSessions, refreshSessions, markSessionActive]);

  const {
    messages,
    timeline,
    input,
    setInput,
    sendMessage,
    sendMultiple,
    activeStreamAgentIds,
    isLoading,
    error,
    errorType,
    stop,
    retry,
    replaceMessages,
    prependMessages,
    isReconnecting: isChatReconnecting,
  } = useChat({
    agentId: chatAgentId,
    agentName: chatAgentName,
    sessionName: chatSessionName,
    sessionId: chatSessionId,
    sessionResetKey: isMCP ? 0 : sessionResetKey,
    initialMessages: chatInitialMessages,
    onSessionCreated,
    mcpConfig: isMCP && mcpServerIds.length > 0
      ? { serverIds: mcpServerIds, modelId: selectedModelId }
      : undefined,
    groupId: activeGroupId || undefined,
    userName: displayName,
  });

  // Listen for GitHub OAuth popup completion → auto-retry
  useEffect(() => {
    const handler = (e: MessageEvent) => {
      if (e.data?.type === "github_connected") {
        retry(undefined, isMultiAgent && selectedTargetAgentIds.length > 0 ? selectedTargetAgentIds : undefined);
      }
    };
    window.addEventListener("message", handler);
    return () => window.removeEventListener("message", handler);
  }, [retry, isMultiAgent, selectedTargetAgentIds]);

  // Real-time subscription for group sessions: reload messages + sessions on RUN_FINISHED
  useSessionSubscription({
    sessionId: chatSessionId,
    groupId: activeGroupId || undefined,
    enabled: !!activeGroupId && !!chatSessionId && !isLoading,
    onEvent: useCallback((event: Record<string, unknown>) => {
      if (event.type === "RUN_FINISHED") {
        refreshSessions();
        // Reload messages from server so other users' messages appear immediately
        if (chatSessionId && currentAgent?.id) {
          fetchSession(currentAgent.id, chatSessionId)
            .then((session) => {
              if (session.messages) {
                replaceMessages(session.messages);
              }
            })
            .catch(() => {});
        }
      }
    }, [refreshSessions, chatSessionId, currentAgent?.id, replaceMessages]),
  });

  // Scroll tracking — disable auto-scroll when user scrolls up manually,
  // re-enable only when user scrolls back to the bottom on their own.
  const lastScrollTopRef = useRef(0);
  const userScrolledUpRef = useRef(false);
  const prevScrollHeightRef = useRef(0);
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;
    const handleScroll = () => {
      const { scrollTop, scrollHeight, clientHeight } = container;
      const nearBottom = scrollHeight - scrollTop - clientHeight < 120;
      isNearBottomRef.current = nearBottom;

      // Detect intentional upward scroll by user
      if (scrollTop < lastScrollTopRef.current - 10) {
        userScrolledUpRef.current = true;
        autoScrollRef.current = false;
      }
      // Re-enable auto-scroll only when user scrolls back near bottom
      if (nearBottom && userScrolledUpRef.current) {
        userScrolledUpRef.current = false;
        autoScrollRef.current = true;
      }
      // If user never scrolled up, keep auto-scroll synced to position
      if (!userScrolledUpRef.current) {
        autoScrollRef.current = nearBottom;
      }
      lastScrollTopRef.current = scrollTop;
    };
    container.addEventListener("scroll", handleScroll, { passive: true });
    return () => container.removeEventListener("scroll", handleScroll);
  }, []);

  // Handle loading older messages when scrolling near the top
  const handleLoadOlder = useCallback(async () => {
    if (!hasMoreMessages || isLoadingMore) return;
    const container = scrollContainerRef.current;
    if (container) {
      prevScrollHeightRef.current = container.scrollHeight;
    }
    const olderMessages = await loadOlderMessages();
    if (olderMessages.length > 0) {
      prependMessages(olderMessages);
    }
  }, [hasMoreMessages, isLoadingMore, loadOlderMessages, prependMessages]);

  // Trigger load-more when user scrolls near the top
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;
    const handleScrollForLoadMore = () => {
      if (container.scrollTop < 100) {
        handleLoadOlder();
      }
    };
    container.addEventListener("scroll", handleScrollForLoadMore, { passive: true });
    return () => container.removeEventListener("scroll", handleScrollForLoadMore);
  }, [handleLoadOlder]);

  useEffect(() => {
    if (autoScrollRef.current) {
      messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [timeline]);

  // Preserve scroll position after older messages are prepended
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container || prevScrollHeightRef.current === 0) return;
    const newScrollHeight = container.scrollHeight;
    const heightDiff = newScrollHeight - prevScrollHeightRef.current;
    if (heightDiff > 0) {
      container.scrollTop += heightDiff;
    }
    prevScrollHeightRef.current = 0;
  }, [messages]);

  // Auto-retry incomplete conversations (page reload during stream)
  useEffect(() => {
    if (!chatSessionId || isLoading || isChatReconnecting) return;
    if (messages.length === 0) return;

    const lastMsg = messages[messages.length - 1];
    if (lastMsg.role === "user") {
      const timer = setTimeout(() => {
        retry(lastMsg);
      }, 500);
      return () => clearTimeout(timer);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only on session load
  }, [chatSessionId]);

  // Focus input on agent/session change
  useEffect(() => {
    const timer = setTimeout(() => inputRef.current?.focus({ preventScroll: true }), 50);
    return () => clearTimeout(timer);
  }, [currentAgent, currentSession, focusKey, currentMCPServer, selectedMCPServerIds]);

  // Helpers
  const getAgentName = useCallback((agentId: string) => {
    const agent = agents.find((a) => a.id === agentId);
    return agent?.name || agentId;
  }, [agents]);

  const entityName = isMCPMulti
    ? "Multi-MCP"
    : currentMCPServer?.name || currentAgent?.name || t("common.agent");

  const copyConversation = useCallback(() => {
    if (messages.length === 0) return;
    const text = messages
      .filter((m) => m.role !== "system")
      .map((m) => {
        const name = m.role === "user"
          ? t("common.user")
          : entityName;
        return `${name}: ${m.content}`;
      })
      .join("\n\n");
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messages, entityName]);

  const handleExportPDF = useCallback(async () => {
    if (messages.length === 0 || !scrollContainerRef.current) return;
    setExporting(true);
    try {
      const { exportChatToPDF } = await import("@/lib/export-pdf");
      const sessionName = isMCP
        ? activeMCPSession?.session_name
        : currentSession?.session_name;
      await exportChatToPDF(
        scrollContainerRef.current,
        entityName,
        sessionName,
      );
    } catch (err) {
      console.error("PDF export failed:", err);
    } finally {
      setExporting(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messages, entityName, currentSession, activeMCPSession, isMCP]);

  const copyMessage = useCallback((idx: number, text: string) => {
    navigator.clipboard.writeText(text).then(() => {
      setCopiedMessageIdx(idx);
      setTimeout(() => setCopiedMessageIdx(null), 2000);
    });
  }, []);

  const handleSend = useCallback(() => {
    autoScrollRef.current = true;
    userScrolledUpRef.current = false;
    const atts = pendingAttachments.length > 0 ? pendingAttachments : undefined;
    if (isMultiAgent && selectedTargetAgentIds.length > 0) {
      // Parallel send to all selected agents with context propagation
      sendMultiple(selectedTargetAgentIds, true, atts);
    } else {
      sendMessage(undefined, undefined, atts);
    }
    setPendingAttachments([]);
    if (inputRef.current) {
      inputRef.current.style.height = "auto";
    }
  }, [pendingAttachments, isMultiAgent, selectedTargetAgentIds, sendMultiple, sendMessage]);

  const handleFileSelect = useCallback((files: FileList | null) => {
    if (!files) return;
    Array.from(files).forEach((file) => {
      if (file.size > MAX_FILE_SIZE) return;
      const reader = new FileReader();
      reader.onload = () => {
        const result = reader.result as string;
        const base64 = result.split(",")[1];
        setPendingAttachments((prev) => [
          ...prev,
          {
            filename: file.name,
            content_type: file.type || "application/octet-stream",
            data: base64,
          },
        ]);
      };
      reader.readAsDataURL(file);
    });
  }, []);

  const handleRemoveAttachment = useCallback((i: number) => {
    setPendingAttachments((prev) => prev.filter((_, idx) => idx !== i));
  }, []);

  const handleReconnect = async () => {
    if (!currentMCPServer) return;
    setIsMCPReconnecting(true);
    try {
      await reconnectMCPServer(currentMCPServer.id);
      await refreshServers();
      toast.success(t("mcp.reconnected", { name: currentMCPServer.name }));
    } catch (err: unknown) {
      const apiErr = err as { status?: number; message?: string };
      if (apiErr.status === 403 && apiErr.message === "oauth2_consent_required") {
        window.open(
          getMCPOAuth2LoginURL(currentMCPServer.id, window.location.href),
          "_blank",
          "width=600,height=700"
        );
        const handler = (e: MessageEvent) => {
          if (e.data?.type === "mcp-oauth-complete") {
            window.removeEventListener("message", handler);
            handleReconnect();
          }
        };
        window.addEventListener("message", handler);
      } else {
        toast.error(t("mcp.reconnectError"));
      }
    } finally {
      setIsMCPReconnecting(false);
    }
  };

  const handleShare = useCallback(async () => {
    if (!currentAgent || !chatSessionId || isMCP) return;
    setIsSharing(true);
    try {
      const resp = await shareSession(currentAgent.id, chatSessionId);
      const url = `${window.location.origin}/shared/${resp.token}`;
      await navigator.clipboard.writeText(url);
      toast.success(t("sessions.shareSuccess"));
    } catch {
      toast.error(t("sessions.shareError"));
    } finally {
      setIsSharing(false);
    }
  }, [currentAgent, chatSessionId, isMCP, t]);

  // Callbacks for "New conversation" from the info view — sets wantsNewChat to bypass info view
  // Must be declared before any early returns to maintain consistent hook order
  const handleNewAgentConversation = useCallback(() => {
    createNewSession(); // sets wantsNewChat=true, clears session
  }, [createNewSession]);

  const handleNewGroupConversation = useCallback(() => {
    if (!activeGroupId || !activeGroup) return;
    createMultiAgentSession(activeGroup.agentIds); // sets wantsNewChat=true
    selectGroup(activeGroupId); // re-selects group context
  }, [createMultiAgentSession, selectGroup, activeGroupId, activeGroup]);

  // Empty state: no agent and not in MCP mode
  if (!isMCP && !currentAgent) {
    return <EmptyState />;
  }

  // MCP empty state: no servers selected
  if (isMCP && mcpServerIds.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <div className="text-center">
          <Server className="mx-auto mb-4 h-12 w-12 text-muted-foreground/30" />
          <p className="text-sm text-muted-foreground">{t("mcp.selectServer")}</p>
        </div>
      </div>
    );
  }

  // Info view: show agent/group/MCP details when no active session (unless user clicked "New conversation")
  if (!wantsNewChat) {
    if (currentAgent && !currentSession && !isMCP && !activeGroupId) {
      return <AgentInfoView mode="agent" agent={currentAgent} sessions={sessions} onNewConversation={handleNewAgentConversation} />;
    }
    if (activeGroupId && !currentSession && !isMCP) {
      return <AgentInfoView mode="group" group={activeGroup} sessions={sessions} onNewConversation={handleNewGroupConversation} />;
    }
    if (isMCP && !activeMCPSession && !isMCPMulti && currentMCPServer) {
      return <AgentInfoView mode="mcp" mcpServer={currentMCPServer} sessions={[]} onNewConversation={handleNewAgentConversation} />;
    }
  }

  const githubRequired = !isMCP && !user?.githubConnected && !!currentAgent?.require_github_token;
  const widthCls = chatWidthClass[preferences.chatWidth] || chatWidthClass.wide;

  const isInputDisabled = isMCP
    ? (isMCPMulti
      ? !selectedMCPServerIds.some((id) => mcpServers.find((s) => s.id === id)?.status === "connected")
      : currentMCPServer?.status !== "connected")
    : !!githubRequired;

  // Multi-MCP display info
  const multiServers = isMCPMulti
    ? selectedMCPServerIds.map((id) => mcpServers.find((s) => s.id === id)).filter(Boolean)
    : [];

  const multiMCPServerNames = multiServers.map((s) => s!.name);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <ChatHeader
        isMCP={isMCP}
        isMCPMulti={isMCPMulti}
        isMultiAgent={isMultiAgent}
        currentAgent={currentAgent}
        currentSessionName={currentSession?.session_name}
        multiAgentIds={multiAgentIds}
        activeGroupId={activeGroupId}
        multiAgentGroups={multiAgentGroups}
        getAgentName={getAgentName}
        currentMCPServer={currentMCPServer}
        multiServers={multiServers}
        selectedModelId={selectedModelId}
        onModelChange={setSelectedModelId}
        availableModels={config.available_models}
        onShowToolsPanel={() => setShowToolsPanel(true)}
        showThinking={preferences.showThinking}
        onToggleThinking={() => updatePreference("showThinking", !preferences.showThinking)}
        hasMessages={messages.length > 0}
        onExportPDF={handleExportPDF}
        exporting={exporting}
        onCopyConversation={copyConversation}
        copied={copied}
        onShare={!isMCP && currentAgent && chatSessionId ? handleShare : undefined}
        sharing={isSharing}
      />

      {/* GitHub connection required banner (agent mode only) */}
      {githubRequired && (
        <div role="alert" className="flex items-center gap-3 border-b border-yellow-300/30 bg-yellow-50 dark:bg-yellow-950/20 px-6 py-2.5">
          <AlertTriangle className="h-4 w-4 shrink-0 text-yellow-600" />
          <p className="text-sm text-yellow-800 dark:text-yellow-200">
            {t("chat.githubRequired")}
          </p>
          <Button
            variant="outline"
            size="sm"
            onClick={() => window.location.href = "/auth/github/login"}
            className="h-7 shrink-0 gap-1.5 border-yellow-400 text-xs text-yellow-800 hover:bg-yellow-100 dark:border-yellow-600 dark:text-yellow-200"
          >
            <Github className="h-3 w-3" />
            {t("chat.connectGithub")}
          </Button>
        </div>
      )}

      {/* MCP connection error banner */}
      {isMCP && !isMCPMulti && currentMCPServer?.status === "error" && (
        <div role="alert" className="flex items-center gap-2 border-b border-destructive/20 bg-destructive/5 px-6 py-2">
          <p className="flex-1 text-xs text-destructive">
            {t("mcp.connectionError", { error: currentMCPServer.status_error || "" })}
          </p>
          <Button
            variant="outline"
            size="sm"
            onClick={handleReconnect}
            disabled={isMCPReconnecting}
            className="h-7 text-xs"
          >
            {isMCPReconnecting ? (
              <Loader2 className="mr-1 h-3 w-3 animate-spin" />
            ) : (
              <RefreshCw className="mr-1 h-3 w-3" />
            )}
            {t("common.retry")}
          </Button>
        </div>
      )}

      {/* MCP OAuth2 connect banner */}
      {isMCP && !isMCPMulti && currentMCPServer?.auth_type === "oauth2" && !currentMCPServer?.oauth2_connected && (
        <div role="alert" className="flex items-center gap-2 border-b border-amber-500/20 bg-amber-500/5 px-6 py-2">
          <p className="flex-1 text-xs text-amber-700 dark:text-amber-400">
            This server requires OAuth2 authorization to connect
          </p>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              window.open(
                getMCPOAuth2LoginURL(currentMCPServer.id, window.location.href),
                "_blank",
                "width=600,height=700"
              );
              const handler = (e: MessageEvent) => {
                if (e.data?.type === "mcp-oauth-complete") {
                  window.removeEventListener("message", handler);
                  refreshServers();
                }
              };
              window.addEventListener("message", handler);
            }}
            className="h-7 shrink-0 gap-1.5 text-xs"
          >
            <KeyRound className="h-3 w-3" />
            Conectar
          </Button>
        </div>
      )}

      {/* Messages area */}
      <ChatMessages
        isMCP={isMCP}
        isMCPMulti={isMCPMulti}
        isMultiAgent={isMultiAgent}
        showThinking={preferences.showThinking}
        sessionId={chatSessionId}
        agentId={chatAgentId}
        timeline={timeline}
        messages={messages}
        isLoading={isLoading}
        activeStreamAgentIds={activeStreamAgentIds}
        currentAgent={currentAgent}
        currentMCPServer={currentMCPServer}
        mcpServers={mcpServers}
        multiMCPServerNames={multiMCPServerNames}
        selectedTargetAgentIds={selectedTargetAgentIds}
        selectedModelId={selectedModelId}
        getAgentName={getAgentName}
        user={user}
        currentUserName={displayName}
        copiedMessageIdx={copiedMessageIdx}
        onCopyMessage={copyMessage}
        onRetry={retry}
        widthCls={widthCls}
        scrollContainerRef={scrollContainerRef}
        messagesEndRef={messagesEndRef}
        hasMoreMessages={hasMoreMessages}
        isLoadingMore={isLoadingMore}
        onLoadOlder={handleLoadOlder}
      />

      {/* Reconnecting banner */}
      {isChatReconnecting && (
        <div role="status" aria-live="polite" className="flex items-center justify-center gap-2 border-t border-yellow-300/30 bg-yellow-50 dark:bg-yellow-950/20 px-6 py-2">
          <Loader2 className="h-3 w-3 animate-spin text-yellow-600 dark:text-yellow-400" />
          <p className="text-xs text-yellow-800 dark:text-yellow-200">{t("chat.reconnecting")}</p>
        </div>
      )}

      {/* Error banner with retry */}
      {error && (
        <div role="alert" aria-live="assertive" className="flex items-center justify-center gap-3 border-t border-destructive/20 bg-destructive/5 px-6 py-2">
          <p className="text-sm text-destructive">{error}</p>
          {errorType === "github_auth" ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => window.open("/auth/github/login", "_blank", "width=600,height=700")}
              className="h-7 shrink-0 gap-1.5 border-destructive/30 text-xs text-destructive hover:bg-destructive/10"
            >
              <Github className="h-3 w-3" />
              {t("chat.connectGithub")}
            </Button>
          ) : errorType === "mcp_oauth2" ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                const serverId = currentMCPServer?.id || selectedMCPServerIds[0];
                if (serverId) {
                  const popup = window.open(
                    getMCPOAuth2LoginURL(serverId, window.location.href),
                    "_blank",
                    "width=600,height=700"
                  );
                  const handler = (e: MessageEvent) => {
                    if (e.data?.type === "mcp-oauth-complete") {
                      window.removeEventListener("message", handler);
                      retry();
                    }
                  };
                  window.addEventListener("message", handler);
                }
              }}
              className="h-7 shrink-0 gap-1.5 border-destructive/30 text-xs text-destructive hover:bg-destructive/10"
            >
              <KeyRound className="h-3 w-3" />
              Conectar
            </Button>
          ) : (
            <Button
              variant="outline"
              size="sm"
              onClick={() => retry(undefined, isMultiAgent && selectedTargetAgentIds.length > 0 ? selectedTargetAgentIds : undefined)}
              className="h-7 gap-1.5 border-destructive/30 text-xs text-destructive hover:bg-destructive/10"
            >
              <RefreshCw className="h-3 w-3" />
              {t("common.retry")}
            </Button>
          )}
        </div>
      )}

      {/* Input area */}
      <ChatInput
        isMCP={isMCP}
        isMultiAgent={isMultiAgent}
        isInputDisabled={isInputDisabled}
        isLoading={isLoading}
        input={input}
        setInput={setInput}
        pendingAttachments={pendingAttachments}
        onRemoveAttachment={handleRemoveAttachment}
        onFileSelect={handleFileSelect}
        multiAgentIds={multiAgentIds}
        selectedTargetAgentIds={selectedTargetAgentIds}
        onToggleTargetAgent={toggleTargetAgent}
        onSend={handleSend}
        onStop={stop}
        widthCls={widthCls}
        inputRef={inputRef}
        fileInputRef={fileInputRef}
      />

      {/* MCP Tools Panel Dialog */}
      {isMCP && <MCPToolsPanel isOpen={showToolsPanel} onClose={() => setShowToolsPanel(false)} />}
    </div>
  );
}
