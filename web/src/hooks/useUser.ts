"use client";

import { useUserContext } from "@/contexts/UserContext";

export function useUser() {
  const { user, isLoading, isAdmin, role, displayName, logout, disconnectGitHub } = useUserContext();

  return {
    user,
    isLoading,
    isAdmin,
    role,
    displayName,
    logout,
    disconnectGitHub,
  };
}
