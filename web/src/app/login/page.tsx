"use client";

import { useEffect, useState } from "react";
import { getAuthSession, getAuthProviders } from "@/lib/api";
import type { AuthProvider } from "@/lib/api";
import { AgentgramLogo } from "@/components/icons/AgentgramLogo";

const PROVIDER_LABELS: Record<string, string> = {
  oidc: "Sign in with SSO",
  basic: "Sign in",
};

export default function LoginPage() {
  const [providers, setProviders] = useState<AuthProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [basicUsername, setBasicUsername] = useState("");
  const [basicPassword, setBasicPassword] = useState("");
  const [basicError, setBasicError] = useState<string | null>(null);
  const [basicLoading, setBasicLoading] = useState(false);

  useEffect(() => {
    // Check if already authenticated
    getAuthSession()
      .then((data) => {
        if (data.authenticated) {
          window.location.href = "/";
          return;
        }
      })
      .catch(() => {});

    // Fetch available providers
    getAuthProviders()
      .then((p) => {
        setProviders(p);

        // Auto-redirect if only one non-basic provider
        const nonBasic = p.filter((prov) => prov.type !== "basic");
        if (nonBasic.length === 1 && p.length === 1) {
          window.location.href = nonBasic[0].login_url;
          return;
        }

        setLoading(false);
      })
      .catch(() => {
        // If providers fail to load, just show empty state
        setLoading(false);
      });
  }, []);

  const handleBasicLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setBasicError(null);
    setBasicLoading(true);
    try {
      const res = await fetch("/auth/basic/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: basicUsername, password: basicPassword }),
        credentials: "include",
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        setBasicError(data.error || "Invalid credentials");
        return;
      }
      window.location.href = "/";
    } catch {
      setBasicError("Connection error");
    } finally {
      setBasicLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-zinc-50 dark:bg-zinc-950">
        <div className="text-zinc-500">Loading...</div>
      </div>
    );
  }

  const nonBasicProviders = providers.filter((p) => p.type !== "basic");
  const basicProvider = providers.find((p) => p.type === "basic");

  return (
    <div className="flex min-h-screen items-center justify-center bg-zinc-50 dark:bg-zinc-950">
      <div className="w-full max-w-sm space-y-8 rounded-xl border bg-white p-8 shadow-lg dark:border-zinc-800 dark:bg-zinc-900">
        {/* Logo + Title */}
        <div className="flex flex-col items-center gap-3">
          <AgentgramLogo className="h-10 w-10 text-zinc-900 dark:text-zinc-100" />
          <h1 className="text-2xl font-bold text-zinc-900 dark:text-zinc-100">Agentgram</h1>
        </div>

        {/* Provider Buttons */}
        <div className="space-y-3">
          {nonBasicProviders.map((provider) => (
            <a
              key={provider.type}
              href={provider.login_url}
              className="flex w-full items-center justify-center gap-2 rounded-lg border bg-zinc-50 px-4 py-2.5 text-sm font-medium text-zinc-900 transition-colors hover:bg-zinc-100 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-100 dark:hover:bg-zinc-700"
            >
              {PROVIDER_LABELS[provider.type] || `Sign in with ${provider.name}`}
            </a>
          ))}
        </div>

        {/* Basic Auth Form */}
        {basicProvider && (
          <>
            {nonBasicProviders.length > 0 && (
              <div className="relative">
                <div className="absolute inset-0 flex items-center">
                  <div className="w-full border-t border-zinc-200 dark:border-zinc-700" />
                </div>
                <div className="relative flex justify-center text-xs">
                  <span className="bg-white px-2 text-zinc-500 dark:bg-zinc-900">or</span>
                </div>
              </div>
            )}
            <form onSubmit={handleBasicLogin} className="space-y-3">
              <input
                type="text"
                placeholder="Username"
                value={basicUsername}
                onChange={(e) => setBasicUsername(e.target.value)}
                className="w-full rounded-lg border bg-transparent px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-zinc-400 dark:border-zinc-700"
                required
                autoComplete="username"
              />
              <input
                type="password"
                placeholder="Password"
                value={basicPassword}
                onChange={(e) => setBasicPassword(e.target.value)}
                className="w-full rounded-lg border bg-transparent px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-zinc-400 dark:border-zinc-700"
                required
                autoComplete="current-password"
              />
              {basicError && (
                <p className="text-sm text-red-500">{basicError}</p>
              )}
              <button
                type="submit"
                disabled={basicLoading}
                className="w-full rounded-lg bg-zinc-900 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-zinc-800 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
              >
                {basicLoading ? "Signing in..." : "Sign in"}
              </button>
            </form>
          </>
        )}

        {providers.length === 0 && (
          <p className="text-center text-sm text-zinc-500">
            No authentication providers configured
          </p>
        )}
      </div>
    </div>
  );
}
