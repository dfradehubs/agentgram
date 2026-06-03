"use client";

import React, { createContext, useContext, useState, useCallback, useEffect } from "react";
import type { MCPServer, Session } from "@/lib/types";
import {
  getMCPServers,
  getMCPSession,
  getMultiMCPSessions,
  getMultiMCPSession,
  deleteMultiMCPSession as apiDeleteMultiMCPSession,
  renameMultiMCPSession as apiRenameMultiMCPSession,
} from "@/lib/api";

interface MCPContextType {
  // Servers
  mcpServers: MCPServer[];
  currentMCPServer: MCPServer | null;
  isLoading: boolean;
  error: string | null;
  selectMCPServer: (serverId: string | null) => void;
  refreshServers: () => Promise<void>;

  // MCP Sessions (per server)
  currentMCPSession: Session | null;
  selectMCPSession: (sessionId: string | null) => void;
  refreshMCPSessions: () => Promise<void>;
  /** Incremented when sessions change — used by MCPSessionList to trigger refresh */
  sessionVersion: number;

  // Multi-MCP
  selectedMCPServerIds: string[];
  multiMCPSessions: Session[];
  currentMultiMCPSession: Session | null;
  isMCPMulti: boolean;
  selectMultiMCP: (serverIds: string[]) => void;
  selectMultiMCPSession: (sessionId: string | null) => void;
  refreshMultiMCPSessions: () => Promise<void>;
  deleteMultiMCPSession: (sessionId: string) => Promise<void>;
  renameMultiMCPSession: (sessionId: string, name: string) => Promise<void>;
}

const MCPContext = createContext<MCPContextType | undefined>(undefined);

export function MCPProvider({ children }: { children: React.ReactNode }) {
  const [mcpServers, setMCPServers] = useState<MCPServer[]>([]);
  const [currentMCPServer, setCurrentMCPServer] = useState<MCPServer | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Sessions
  const [currentMCPSession, setCurrentMCPSession] = useState<Session | null>(null);
  const [sessionVersion, setSessionVersion] = useState(0);

  // Multi-MCP
  const [selectedMCPServerIds, setSelectedMCPServerIds] = useState<string[]>([]);
  const [multiMCPSessions, setMultiMCPSessions] = useState<Session[]>([]);
  const [currentMultiMCPSession, setCurrentMultiMCPSession] = useState<Session | null>(null);

  const isMCPMulti = selectedMCPServerIds.length > 1;

  const refreshServers = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const servers = await getMCPServers();
      servers.sort((a, b) => a.name.localeCompare(b.name));
      setMCPServers(servers);
      // Keep currentMCPServer in sync with fresh data
      setCurrentMCPServer((prev) => {
        if (!prev) return null;
        return servers.find((s) => s.id === prev.id) || prev;
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load MCP servers");
    } finally {
      setIsLoading(false);
    }
  }, []);

  const selectMCPServer = useCallback(
    (serverId: string | null) => {
      if (serverId === null) {
        setCurrentMCPServer(null);
        setCurrentMCPSession(null);
        setSelectedMCPServerIds([]);
        setCurrentMultiMCPSession(null);
        return;
      }
      const server = mcpServers.find((s) => s.id === serverId) || null;
      setCurrentMCPServer(server);
      setCurrentMCPSession(null);
      setSelectedMCPServerIds([]);
      setCurrentMultiMCPSession(null);
    },
    [mcpServers]
  );

  // Signal MCPSessionList components to refresh their local sessions
  const refreshMCPSessions = useCallback(async () => {
    setSessionVersion((v) => v + 1);
  }, []);

  const selectMCPSession = useCallback(
    async (sessionId: string | null) => {
      if (!sessionId || !currentMCPServer) {
        setCurrentMCPSession(null);
        return;
      }
      // Clear immediately to avoid stale data while fetching
      setCurrentMCPSession(null);
      try {
        const session = await getMCPSession(currentMCPServer.id, sessionId);
        setCurrentMCPSession(session);
      } catch {
        setCurrentMCPSession(null);
      }
    },
    [currentMCPServer]
  );

  // Multi-MCP
  const selectMultiMCP = useCallback(
    (serverIds: string[]) => {
      setSelectedMCPServerIds(serverIds);
      setCurrentMCPServer(null);
      setCurrentMCPSession(null);
      setCurrentMultiMCPSession(null);
    },
    []
  );

  const refreshMultiMCPSessions = useCallback(async () => {
    try {
      const sessions = await getMultiMCPSessions();
      setMultiMCPSessions(sessions);
    } catch {
      setMultiMCPSessions([]);
    }
  }, []);

  const selectMultiMCPSession = useCallback(
    async (sessionId: string | null) => {
      if (!sessionId) {
        setCurrentMultiMCPSession(null);
        return;
      }
      // Clear immediately to avoid stale data
      setCurrentMultiMCPSession(null);
      setCurrentMCPServer(null);
      setCurrentMCPSession(null);
      try {
        const session = await getMultiMCPSession(sessionId);
        // Set server IDs so isMCPMulti becomes true and ChatArea renders MCPChat
        if (session?.agent_ids?.length) {
          setSelectedMCPServerIds(session.agent_ids);
        }
        setCurrentMultiMCPSession(session);
      } catch {
        setCurrentMultiMCPSession(null);
      }
    },
    []
  );

  const deleteMultiMCPSession = useCallback(
    async (sessionId: string) => {
      await apiDeleteMultiMCPSession(sessionId);
      if (currentMultiMCPSession?.session_id === sessionId) {
        setCurrentMultiMCPSession(null);
      }
      await refreshMultiMCPSessions();
    },
    [currentMultiMCPSession, refreshMultiMCPSessions]
  );

  const renameMultiMCPSession = useCallback(
    async (sessionId: string, name: string) => {
      await apiRenameMultiMCPSession(sessionId, name);
      await refreshMultiMCPSessions();
    },
    [refreshMultiMCPSessions]
  );

  // Auto-fetch servers on mount
  useEffect(() => {
    refreshServers();
  }, [refreshServers]);

  // Silent polling every 30s to keep status indicators up-to-date.
  // Only update currentMCPServer reference if something actually changed
  // to avoid triggering downstream effects.
  useEffect(() => {
    const serverChanged = (a: MCPServer, b: MCPServer) =>
      a.status !== b.status || a.name !== b.name || a.description !== b.description || a.status_error !== b.status_error;

    const interval = setInterval(async () => {
      try {
        const freshServers = await getMCPServers();
        freshServers.sort((a, b) => a.name.localeCompare(b.name));
        setMCPServers(freshServers);
        setCurrentMCPServer((prev) => {
          if (!prev) return null;
          const updated = freshServers.find((s) => s.id === prev.id);
          if (!updated) return prev;
          return serverChanged(prev, updated) ? updated : prev;
        });
      } catch {
        // Silent fail — don't update error state for background polls
      }
    }, 30_000);
    return () => clearInterval(interval);
  }, []);

  // No need to refresh sessions on server change — MCPSessionList manages its own sessions per-server

  // Refresh multi-MCP sessions on mount
  useEffect(() => {
    refreshMultiMCPSessions();
  }, [refreshMultiMCPSessions]);

  return (
    <MCPContext.Provider
      value={{
        mcpServers,
        currentMCPServer,
        isLoading,
        error,
        selectMCPServer,
        refreshServers,
        currentMCPSession,
        selectMCPSession,
        refreshMCPSessions,
        sessionVersion,
        selectedMCPServerIds,
        multiMCPSessions,
        currentMultiMCPSession,
        isMCPMulti,
        selectMultiMCP,
        selectMultiMCPSession,
        refreshMultiMCPSessions,
        deleteMultiMCPSession,
        renameMultiMCPSession,
      }}
    >
      {children}
    </MCPContext.Provider>
  );
}

export function useMCPContext() {
  const context = useContext(MCPContext);
  if (context === undefined) {
    throw new Error("useMCPContext must be used within an MCPProvider");
  }
  return context;
}
