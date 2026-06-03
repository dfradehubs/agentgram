"use client";

import React, { createContext, useContext, useState, useEffect, useCallback } from "react";
import type { User } from "@/lib/types";
import { getAuthSession, getMe, logout as apiLogout, disconnectGitHub as apiDisconnectGitHub, ApiError } from "@/lib/api";

interface UserContextType {
  user: User | null;
  isLoading: boolean;
  isAdmin: boolean;
  displayName: string;
  logout: () => Promise<void>;
  disconnectGitHub: () => Promise<void>;
}

const UserContext = createContext<UserContextType | undefined>(undefined);

export function UserProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [authError, setAuthError] = useState<string | null>(null);

  const checkAuth = useCallback(() => {
    setIsLoading(true);
    setAuthError(null);
    getAuthSession()
      .then(async (data) => {
        if (data.authenticated) {
          // Fetch /api/me for admin status
          let isAdmin = false;
          try {
            const meData = await getMe();
            isAdmin = (meData as { is_admin?: boolean }).is_admin || false;
          } catch (err) {
            if (err instanceof ApiError && err.status === 401) {
              // Token expired between session check and API call — treat as unauthenticated
              setUser(null);
              setIsLoading(false);
              const path = window.location.pathname;
              if (path !== "/login" && !path.startsWith("/auth/")) {
                window.location.href = "/login";
              }
              return;
            }
            // Other errors: admin status defaults to false
          }
          setUser({
            email: data.email || "",
            name: data.name || "",
            groups: data.groups,
            isAdmin,
            githubConnected: data.github_connected,
            githubUsername: data.github_username,
          });
        } else {
          setUser(null);
          const path = window.location.pathname;
          if (path !== "/login" && !path.startsWith("/auth/")) {
            window.location.href = "/login";
          }
        }
      })
      .catch((err) => {
        setUser(null);
        setAuthError(err instanceof Error ? err.message : "Connection error");
      })
      .finally(() => setIsLoading(false));
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- fetch initial auth state from server on mount
    checkAuth();
  }, [checkAuth]);

  const logout = useCallback(async () => {
    const result = await apiLogout();
    setUser(null);
    if (result.logout_url) {
      window.location.href = result.logout_url;
    } else {
      window.location.href = "/login";
    }
  }, []);

  const disconnectGitHub = useCallback(async () => {
    await apiDisconnectGitHub();
    setUser((prev) => prev ? { ...prev, githubConnected: false, githubUsername: undefined } : null);
  }, []);

  const displayName =
    user?.name
      ? user.name
      : user?.email && user.email !== "anonymous@localhost"
        ? user.email.split("@")[0]
        : "Anonymous";

  // Don't render children until auth is confirmed - prevents flash of content
  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-zinc-50 dark:bg-zinc-950">
        <div className="text-zinc-500">Loading...</div>
      </div>
    );
  }

  // Auth error - show retry button instead of blank screen
  if (authError && !user) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-zinc-50 dark:bg-zinc-950">
        <div className="text-red-500">{authError}</div>
        <button
          onClick={checkAuth}
          className="rounded-md bg-zinc-800 px-4 py-2 text-sm text-white hover:bg-zinc-700 dark:bg-zinc-200 dark:text-zinc-900 dark:hover:bg-zinc-300"
        >
          Retry
        </button>
      </div>
    );
  }

  // Not authenticated and not redirecting yet - show nothing
  if (!user && window.location.pathname !== "/login" && !window.location.pathname.startsWith("/auth/")) {
    return null;
  }

  const isAdmin = user?.isAdmin || false;

  return (
    <UserContext.Provider value={{ user, isLoading, isAdmin, displayName, logout, disconnectGitHub }}>
      {children}
    </UserContext.Provider>
  );
}

export function useUserContext() {
  const context = useContext(UserContext);
  if (context === undefined) {
    throw new Error("useUserContext must be used within a UserProvider");
  }
  return context;
}
