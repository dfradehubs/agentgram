"use client";

import React, {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
} from "react";
import type { Agent } from "@/lib/types";
import { getAgents } from "@/lib/api";

interface AgentContextType {
  agents: Agent[];
  currentAgent: Agent | null;
  isLoading: boolean;
  error: string | null;
  focusKey: number;
  selectAgent: (agentId: string) => void;
  refreshAgents: () => Promise<void>;
}

const AgentContext = createContext<AgentContextType | undefined>(undefined);

export function AgentProvider({ children }: { children: React.ReactNode }) {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [currentAgent, setCurrentAgent] = useState<Agent | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [focusKey, setFocusKey] = useState(0);

  const refreshAgents = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const fetchedAgents = await getAgents();
      fetchedAgents.sort((a, b) => a.name.localeCompare(b.name));
      setAgents(fetchedAgents);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load agents");
    } finally {
      setIsLoading(false);
    }
  }, []);

  const selectAgent = useCallback(
    (agentId: string) => {
      const agent = agents.find((a) => a.id === agentId) || null;
      setCurrentAgent(agent);
      setFocusKey((k) => k + 1);
      try { sessionStorage.setItem("agentgram-current-agent", agentId); } catch {}
    },
    [agents]
  );

  // Load agents on mount
  useEffect(() => {
    refreshAgents();
  }, [refreshAgents]);

  // Restore last selected agent after agents load
  useEffect(() => {
    if (agents.length === 0 || currentAgent) return;
    try {
      const savedId = sessionStorage.getItem("agentgram-current-agent");
      if (savedId) {
        const agent = agents.find((a) => a.id === savedId);
        if (agent) setCurrentAgent(agent);
      }
    } catch {}
    // eslint-disable-next-line react-hooks/exhaustive-deps -- currentAgent excluded to only restore on initial load
  }, [agents]);

  // Silent polling every 30s to keep status indicators up-to-date.
  // Only update currentAgent reference if something actually changed
  // (status, name, etc.) to avoid triggering downstream effects that
  // reset the current session.
  // Polling pauses when the browser tab is not visible.
  useEffect(() => {
    let intervalId: NodeJS.Timeout | null = null;

    const agentChanged = (a: Agent, b: Agent) =>
      a.status !== b.status || a.name !== b.name || a.description !== b.description;

    const poll = async () => {
      try {
        const freshAgents = await getAgents();
        freshAgents.sort((a, b) => a.name.localeCompare(b.name));
        setAgents(freshAgents);
        setCurrentAgent((prev) => {
          if (!prev) return null;
          const updated = freshAgents.find((a) => a.id === prev.id);
          if (!updated) return prev;
          return agentChanged(prev, updated) ? updated : prev;
        });
      } catch {
        // Silent fail — don't update error state for background polls
      }
    };

    const startPolling = () => {
      if (intervalId) return;
      intervalId = setInterval(poll, 30_000);
    };

    const stopPolling = () => {
      if (intervalId) {
        clearInterval(intervalId);
        intervalId = null;
      }
    };

    const handleVisibility = () => {
      if (document.hidden) {
        stopPolling();
      } else {
        poll();
        startPolling();
      }
    };

    startPolling();
    document.addEventListener("visibilitychange", handleVisibility);

    return () => {
      stopPolling();
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, []);

  return (
    <AgentContext.Provider
      value={{
        agents,
        currentAgent,
        isLoading,
        error,
        focusKey,
        selectAgent,
        refreshAgents,
      }}
    >
      {children}
    </AgentContext.Provider>
  );
}

export function useAgentContext() {
  const context = useContext(AgentContext);
  if (context === undefined) {
    throw new Error("useAgentContext must be used within an AgentProvider");
  }
  return context;
}
