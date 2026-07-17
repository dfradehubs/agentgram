"use client";

import { useSessionContext } from "@/contexts/SessionContext";

export function useSessions() {
  const {
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
    newGroupConversation,
    multiAgentGroups,
    activeGroupId,
    selectGroup,
    markSessionActive,
  } = useSessionContext();

  return {
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
    newGroupConversation,
    multiAgentGroups,
    activeGroupId,
    selectGroup,
    markSessionActive,
  };
}
