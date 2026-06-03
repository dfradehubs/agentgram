"use client";

import React, {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
} from "react";
import type { Session, Message, MultiAgentGroup } from "@/lib/types";
import {
  getSessions,
  getSession,
  getSessionPaginated,
  getGroups,
  createGroup as apiCreateGroup,
  updateGroup as apiUpdateGroup,
  deleteGroup as apiDeleteGroup,
  getGroupSessions,
  addGroupSession as apiAddGroupSession,
  renameSession as apiRenameSession,
  deleteSession as apiDeleteSession,
} from "@/lib/api";
import { useAgentContext } from "./AgentContext";
import { useMCPContext } from "./MCPContext";

interface SessionContextType {
  sessions: Session[];
  currentSession: Session | null;
  sessionResetKey: number;
  isLoading: boolean;
  error: string | null;
  selectSession: (sessionId: string, overrideAgentId?: string) => Promise<void>;
  reloadCurrentSession: () => Promise<void>;
  createNewSession: () => void;
  clearCurrentSession: () => void;
  wantsNewChat: boolean;
  renameSession: (sessionId: string, newName: string) => Promise<void>;
  deleteSession: (sessionId: string) => Promise<void>;
  refreshSessions: () => Promise<void>;
  getInitialMessages: () => Message[];
  // Pagination
  hasMoreMessages: boolean;
  isLoadingMore: boolean;
  loadOlderMessages: () => Promise<Message[]>;
  // Multi-agent group chat
  pendingMultiAgentIds: string[];
  createMultiAgentSession: (agentIds: string[]) => void;
  clearMultiAgentSession: () => void;
  // Persistent groups (backed by API)
  multiAgentGroups: MultiAgentGroup[];
  activeGroupId: string | null;
  addMultiAgentGroup: (name: string, agentIds: string[], allowedUsers?: string[], allowedGroups?: string[]) => Promise<MultiAgentGroup>;
  updateMultiAgentGroup: (groupId: string, data: { name?: string; agentIds?: string[]; allowed_users?: string[]; allowed_groups?: string[] }) => Promise<void>;
  removeMultiAgentGroup: (id: string) => Promise<void>;
  selectGroup: (id: string) => void;
  addSessionToGroup: (groupId: string, sessionId: string) => void;
  markSessionActive: (sessionId: string, sessionName?: string) => void;
}

const SessionContext = createContext<SessionContextType | undefined>(undefined);

const MESSAGES_PAGE_SIZE = 50;

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const { currentAgent, selectAgent } = useAgentContext();
  const { selectMCPServer } = useMCPContext();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [currentSession, setCurrentSession] = useState<Session | null>(null);
  const [sessionResetKey, setSessionResetKey] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pendingMultiAgentIds, setPendingMultiAgentIds] = useState<string[]>([]);
  const [activeGroupId, setActiveGroupId] = useState<string | null>(null);
  const [wantsNewChat, setWantsNewChat] = useState(false);

  // Pagination state
  const [hasMoreMessages, setHasMoreMessages] = useState(false);
  const [nextCursor, setNextCursor] = useState<number | undefined>(undefined);
  const [isLoadingMore, setIsLoadingMore] = useState(false);

  // Persistent groups from API
  const [multiAgentGroups, setMultiAgentGroups] = useState<MultiAgentGroup[]>([]);

  // Fetch groups from API on mount
  useEffect(() => {
    getGroups()
      .then(setMultiAgentGroups)
      .catch(() => {});
  }, []);

  const refreshSessions = useCallback(async () => {
    // If a group is active, load sessions from the group API
    if (activeGroupId) {
      setIsLoading(true);
      setError(null);
      try {
        const fetchedSessions = await getGroupSessions(activeGroupId);
        fetchedSessions.sort((a, b) => b.last_activity - a.last_activity);
        setSessions(fetchedSessions);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load sessions");
      } finally {
        setIsLoading(false);
      }
      return;
    }

    if (!currentAgent) {
      setSessions([]);
      return;
    }

    setIsLoading(true);
    setError(null);
    try {
      const fetchedSessions = await getSessions(currentAgent.id);
      // Sort by last activity (most recent first)
      fetchedSessions.sort((a, b) => b.last_activity - a.last_activity);
      setSessions(fetchedSessions);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load sessions");
    } finally {
      setIsLoading(false);
    }
  }, [currentAgent, activeGroupId]);

  const selectSession = useCallback(
    async (sessionId: string, overrideAgentId?: string) => {
      const agentId = overrideAgentId || currentAgent?.id;
      if (!agentId) return;

      // If overriding agent, select it in the agent context
      if (overrideAgentId && overrideAgentId !== currentAgent?.id) {
        selectAgent(overrideAgentId);
      }

      // Clear MCP state so ChatArea switches to agent chat
      selectMCPServer(null);

      // Clear group state for Slack sessions (they're not groups)
      setActiveGroupId(null);

      // Clear immediately to avoid stale data while fetching
      setCurrentSession(null);
      setWantsNewChat(false);
      setHasMoreMessages(false);
      setNextCursor(undefined);
      setIsLoading(true);
      setError(null);
      try {
        const resp = await getSessionPaginated(agentId, sessionId, MESSAGES_PAGE_SIZE);
        const session: Session = {
          ...resp.session,
          messages: resp.messages,
        };
        setCurrentSession(session);
        setHasMoreMessages(resp.has_more);
        setNextCursor(resp.next_cursor);
        try { sessionStorage.setItem("agentgram-current-session", sessionId); } catch {}
      } catch (err) {
        setError(
          err instanceof Error ? err.message : "Failed to load session"
        );
      } finally {
        setIsLoading(false);
      }
    },
    [currentAgent, selectAgent, selectMCPServer]
  );

  // Silently reload the current session's messages (no loading flash)
  const reloadCurrentSession = useCallback(async () => {
    if (!currentAgent || !currentSession) return;
    try {
      const session = await getSession(currentAgent.id, currentSession.session_id);
      setCurrentSession(session);
    } catch {
      // Silent fail — next poll or manual refresh will catch up
    }
  }, [currentAgent, currentSession]);

  // Load older messages (prepend to current session).
  // Returns the older messages so the caller can also update the chat hook.
  const loadOlderMessages = useCallback(async (): Promise<Message[]> => {
    if (!currentAgent || !currentSession || !hasMoreMessages || isLoadingMore) return [];
    setIsLoadingMore(true);
    try {
      const resp = await getSessionPaginated(
        currentAgent.id,
        currentSession.session_id,
        MESSAGES_PAGE_SIZE,
        nextCursor
      );
      setCurrentSession((prev) => {
        if (!prev) return prev;
        return {
          ...prev,
          messages: [...resp.messages, ...(prev.messages || [])],
        };
      });
      setHasMoreMessages(resp.has_more);
      setNextCursor(resp.next_cursor);
      return resp.messages;
    } catch {
      // Silent fail — user can retry by scrolling up again
      return [];
    } finally {
      setIsLoadingMore(false);
    }
  }, [currentAgent, currentSession, hasMoreMessages, isLoadingMore, nextCursor]);

  const createNewSession = useCallback(() => {
    setCurrentSession(null);
    setPendingMultiAgentIds([]);
    setActiveGroupId(null);
    setWantsNewChat(true);
    setHasMoreMessages(false);
    setNextCursor(undefined);
    setSessionResetKey((k) => k + 1);
    try { sessionStorage.removeItem("agentgram-current-session"); } catch {}
  }, []);

  const clearCurrentSession = useCallback(() => {
    setCurrentSession(null);
    setPendingMultiAgentIds([]);
    setActiveGroupId(null);
    setWantsNewChat(false);
    setHasMoreMessages(false);
    setNextCursor(undefined);
    setSessions([]); // Clear stale sessions immediately (will be refreshed by agent change effect)
    setSessionResetKey((k) => k + 1);
    try { sessionStorage.removeItem("agentgram-current-session"); } catch {}
  }, []);

  const createMultiAgentSession = useCallback((agentIds: string[]) => {
    setPendingMultiAgentIds(agentIds);
    setCurrentSession(null);
    setWantsNewChat(true);
    setSessionResetKey((k) => k + 1);
    // Clear MCP state
    selectMCPServer(null);
    try { sessionStorage.removeItem("agentgram-current-session"); } catch {}
  }, [selectMCPServer]);

  const clearMultiAgentSession = useCallback(() => {
    setPendingMultiAgentIds([]);
  }, []);

  const addMultiAgentGroup = useCallback(async (name: string, agentIds: string[], allowedUsers?: string[], allowedGroups?: string[]): Promise<MultiAgentGroup> => {
    const groupName = name || agentIds.map((id) => id.split("-")[0]).join(" + ");
    const group = await apiCreateGroup(groupName, agentIds, allowedUsers, allowedGroups);
    setMultiAgentGroups((prev) => [...prev, group]);
    return group;
  }, []);

  const updateMultiAgentGroup = useCallback(async (groupId: string, data: { name?: string; agentIds?: string[]; allowed_users?: string[]; allowed_groups?: string[] }) => {
    const updated = await apiUpdateGroup(groupId, data);
    setMultiAgentGroups((prev) => prev.map((g) => g.id === groupId ? updated : g));
  }, []);

  const addSessionToGroup = useCallback((groupId: string, sessionId: string) => {
    // Persist to API (fire-and-forget)
    apiAddGroupSession(groupId, sessionId).catch(() => {});
  }, []);

  const removeMultiAgentGroup = useCallback(async (id: string) => {
    await apiDeleteGroup(id);
    setMultiAgentGroups((prev) => prev.filter((g) => g.id !== id));
    if (activeGroupId === id) {
      setActiveGroupId(null);
      setPendingMultiAgentIds([]);
    }
  }, [activeGroupId]);

  const selectGroup = useCallback((id: string) => {
    const group = multiAgentGroups.find((g) => g.id === id);
    if (!group) return;
    setActiveGroupId(id);
    setPendingMultiAgentIds(group.agentIds);
    setCurrentSession(null);
    setSessionResetKey((k) => k + 1);
    selectMCPServer(null);
    try { sessionStorage.removeItem("agentgram-current-session"); } catch {}
  }, [multiAgentGroups, selectMCPServer]);

  // Lightweight session activation: creates a stub currentSession without API fetch.
  // Used when RUN_STARTED provides a threadId (and optionally sessionName) so the sidebar highlights the new session.
  const markSessionActive = useCallback((sessionId: string, sessionName?: string) => {
    setCurrentSession((prev) => {
      if (prev) return prev; // Don't overwrite an existing session
      return {
        session_id: sessionId,
        session_name: sessionName || "New session",
        user_id: "",
        app_name: "",
        is_multi_agent: pendingMultiAgentIds.length >= 2,
        created_at: Date.now() / 1000,
        last_activity: Date.now() / 1000,
        message_count: 0,
        messages: [],
      };
    });
    setWantsNewChat(false);
    try { sessionStorage.setItem("agentgram-current-session", sessionId); } catch {}
  }, [pendingMultiAgentIds]);

  const renameSession = useCallback(
    async (sessionId: string, newName: string) => {
      if (!currentAgent) return;

      try {
        await apiRenameSession(currentAgent.id, sessionId, newName);
        await refreshSessions();
        // Update current session if it's the one being renamed
        if (currentSession?.session_id === sessionId) {
          setCurrentSession((prev) =>
            prev ? { ...prev, session_name: newName } : null
          );
        }
      } catch (err) {
        throw err;
      }
    },
    [currentAgent, currentSession, refreshSessions]
  );

  const deleteSession = useCallback(
    async (sessionId: string) => {
      if (!currentAgent) return;

      try {
        await apiDeleteSession(currentAgent.id, sessionId);
        // Clear current session if it's the one being deleted
        if (currentSession?.session_id === sessionId) {
          setCurrentSession(null);
        }
        await refreshSessions();
      } catch (err) {
        throw err;
      }
    },
    [currentAgent, currentSession, refreshSessions]
  );

  const getInitialMessages = useCallback((): Message[] => {
    if (!currentSession?.messages) return [];
    return currentSession.messages;
  }, [currentSession]);

  // Restore saved session on mount
  const [restored, setRestored] = useState(false);

  // Track agent ID to only clear session when the actual agent changes,
  // not when the same agent's status/name updates during polling.
  const prevAgentIdRef = useRef<string | null>(null);

  // Reset sessions when agent changes
  useEffect(() => {
    const agentId = currentAgent?.id ?? null;
    const agentIdChanged = agentId !== prevAgentIdRef.current;
    prevAgentIdRef.current = agentId;

    if (agentIdChanged) {
      setCurrentSession(null);
      setWantsNewChat(false);
    }
    if (currentAgent) {
      refreshSessions();
    } else {
      setSessions([]);
    }
  }, [currentAgent, refreshSessions]);

  // Refresh sessions when activeGroupId changes + poll every 10s for group sessions
  useEffect(() => {
    if (!activeGroupId) return;
    refreshSessions();
    const interval = setInterval(refreshSessions, 15_000);
    return () => clearInterval(interval);
  }, [activeGroupId, refreshSessions]);

  // Handle pending-select from shared session clone redirect
  useEffect(() => {
    if (restored) return;
    try {
      const raw = sessionStorage.getItem("agentgram-pending-select");
      if (!raw) return;
      sessionStorage.removeItem("agentgram-pending-select");
      const { agentId, sessionId } = JSON.parse(raw) as { agentId: string; sessionId: string };
      if (agentId && sessionId) {
        selectAgent(agentId);
        // Store the session ID so the normal restore flow picks it up
        sessionStorage.setItem("agentgram-current-session", sessionId);
      }
    } catch {
      // Ignore parse errors
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- only on mount
  }, []);

  // After sessions load, restore the saved session (once)
  useEffect(() => {
    if (restored || !currentAgent) return;
    try {
      const savedId = sessionStorage.getItem("agentgram-current-session");
      if (!savedId) { setRestored(true); return; }
      if (sessions.length > 0 || !isLoading) {
        selectSession(savedId);
      } else {
        return; // Wait for sessions to load
      }
      setRestored(true);
    } catch {
      setRestored(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- selectSession excluded to avoid restore loops
  }, [currentAgent, sessions, isLoading, restored]);

  return (
    <SessionContext.Provider
      value={{
        sessions,
        currentSession,
        sessionResetKey,
        isLoading,
        error,
        selectSession,
        reloadCurrentSession,
        createNewSession,
        clearCurrentSession,
        wantsNewChat,
        renameSession,
        deleteSession,
        refreshSessions,
        getInitialMessages,
        hasMoreMessages,
        isLoadingMore,
        loadOlderMessages,
        pendingMultiAgentIds,
        createMultiAgentSession,
        clearMultiAgentSession,
        multiAgentGroups,
        activeGroupId,
        addMultiAgentGroup,
        updateMultiAgentGroup,
        removeMultiAgentGroup,
        selectGroup,
        addSessionToGroup,
        markSessionActive,
      }}
    >
      {children}
    </SessionContext.Provider>
  );
}

export function useSessionContext() {
  const context = useContext(SessionContext);
  if (context === undefined) {
    throw new Error("useSessionContext must be used within a SessionProvider");
  }
  return context;
}
