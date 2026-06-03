"use client";

import { useAgentContext } from "@/contexts/AgentContext";

export function useAgents() {
  const { agents, currentAgent, isLoading, error, selectAgent, refreshAgents } =
    useAgentContext();

  return {
    agents,
    currentAgent,
    isLoading,
    error,
    selectAgent,
    refreshAgents,
  };
}
