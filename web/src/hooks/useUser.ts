"use client";

import { useUserContext } from "@/contexts/UserContext";

export function useUser() {
  const { user, isLoading, isAdmin, displayName, logout, disconnectGitHub } = useUserContext();

  return {
    user,
    isLoading,
    isAdmin,
    displayName,
    logout,
    disconnectGitHub,
  };
}
