"use client";

import { useState, useMemo, useCallback, Suspense } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Filter, Search, X } from "lucide-react";
import {
  getMetricsOverview,
  getMetricsTimeline,
  getMetricsTopResources,
  getMetricsOverviewUsers,
  getMetricsOverviewErrors,
  getMetricsOverviewErrorEvents,
  getResourceMetrics,
  getResourceTimeline,
  getResourceUsers,
  getResourceErrors,
  getResourceErrorEvents,
  getUserMetrics,
  getUserTimeline,
  getUserTopResources,
} from "@/lib/api";
import type { MetricsResourceType } from "@/lib/api";
import type { GlobalStats, ResourceStats, UserDetailStats, ResourceRanking, ErrorEvent } from "@/lib/types";
import { useMetrics } from "@/hooks/useMetrics";
import { getEntityColor } from "@/lib/agent-colors";
import { StatsCards } from "@/components/admin/observability/StatsCards";
import { TimelineChart, type Range, type ResourceTimeline } from "@/components/admin/observability/TimelineChart";
import { LatencyChart } from "@/components/admin/observability/LatencyChart";
import { ResourceTable } from "@/components/admin/observability/ResourceTable";
import { RequestsBarChart } from "@/components/admin/observability/RequestsBarChart";
import { ErrorsTable } from "@/components/admin/observability/ErrorsTable";
import { UsersTable } from "@/components/admin/observability/UsersTable";
import { ResourceSelector, type ResourceSelection } from "@/components/admin/observability/ResourceSelector";
import { Skeleton } from "@/components/ui/skeleton";

type StatusFilter = "all" | "error" | "ok";

const RANGE_OPTIONS: { value: Range; label: string }[] = [
  { value: "1h", label: "1h" },
  { value: "4h", label: "4h" },
  { value: "12h", label: "12h" },
  { value: "24h", label: "24h" },
  { value: "7d", label: "7d" },
  { value: "30d", label: "30d" },
  { value: "90d", label: "90d" },
];

const STATUS_OPTIONS: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "error", label: "Errors only" },
  { value: "ok", label: "Success only" },
];

function rangeToISO(range: Range): { from: string; to: string } {
  const to = new Date();
  const from = new Date();
  switch (range) {
    case "1h": from.setHours(from.getHours() - 1); break;
    case "4h": from.setHours(from.getHours() - 4); break;
    case "12h": from.setHours(from.getHours() - 12); break;
    case "24h": from.setHours(from.getHours() - 24); break;
    case "7d": from.setDate(from.getDate() - 7); break;
    case "30d": from.setDate(from.getDate() - 30); break;
    case "90d": from.setDate(from.getDate() - 90); break;
  }
  return { from: from.toISOString(), to: to.toISOString() };
}

function rangeToInterval(range: Range): string {
  switch (range) {
    case "1h": return "5m";
    case "4h": return "15m";
    case "12h": return "30m";
    case "24h": return "1h";
    case "7d": return "1d";
    case "30d": return "1d";
    case "90d": return "1d";
  }
}

function dbTypeToApiType(type: string): MetricsResourceType {
  switch (type) {
    case "agent": return "agents";
    case "mcp": return "mcp";
    default: return "agents";
  }
}

function encodeSelections(sel: ResourceSelection[]): string {
  return sel.map((s) => `${s.type}:${s.id}`).join(",");
}

function decodeSelections(param: string | null): ResourceSelection[] {
  if (!param) return [];
  return param.split(",").filter(Boolean).map((s) => {
    const [type, ...idParts] = s.split(":");
    return { type: type as ResourceSelection["type"], id: idParts.join(":"), name: "" };
  });
}

function buildURL(resource: ResourceSelection[], user: string | null): string {
  const parts: string[] = [];
  const encoded = encodeSelections(resource);
  if (encoded) parts.push(`r=${encoded}`);
  if (user) parts.push(`u=${encodeURIComponent(user)}`);
  return parts.length > 0 ? `/admin/observability?${parts.join("&")}` : "/admin/observability";
}

function ObservabilityContent() {
  const searchParams = useSearchParams();
  const router = useRouter();

  const [initialSelection] = useState(() => decodeSelections(searchParams.get("r")));
  const [initialUser] = useState(() => searchParams.get("u") || null);

  const [range, setRange] = useState<Range>("24h");
  const [selected, setSelected] = useState<ResourceSelection[]>(initialSelection);
  const [selectedUser, setSelectedUser] = useState<string | null>(initialUser);
  const [userSearch, setUserSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const { from, to } = useMemo(() => rangeToISO(range), [range]);
  const interval = rangeToInterval(range);

  const selectedKey = useMemo(
    () => selected.map((s) => `${s.type}:${s.id}`).sort().join(",") || "all",
    [selected],
  );

  const isSingle = selected.length === 1;
  const isMulti = selected.length > 1;
  const isGlobal = selected.length === 0;
  const hasUser = selectedUser !== null;

  const handleFilterChange = useCallback((sel: ResourceSelection[]) => {
    setSelected(sel);
    router.replace(buildURL(sel, selectedUser));
  }, [router, selectedUser]);

  const handleUserClick = useCallback((email: string) => {
    const newUser = selectedUser === email ? null : email;
    setSelectedUser(newUser);
    router.replace(buildURL(selected, newUser));
  }, [router, selected, selectedUser]);

  const clearUser = useCallback(() => {
    setSelectedUser(null);
    setUserSearch("");
    router.replace(buildURL(selected, null));
  }, [router, selected]);

  const clearAllFilters = useCallback(() => {
    setSelected([]);
    setSelectedUser(null);
    setUserSearch("");
    setStatusFilter("all");
    router.replace("/admin/observability");
  }, [router]);

  const hasActiveFilters = selected.length > 0 || selectedUser !== null || statusFilter !== "all";

  // Resource filter for user+resource combined queries
  const userResourceFilter = useMemo(() => {
    if (!hasUser || !isSingle) return undefined;
    const r = selected[0];
    return { type: dbTypeToApiType(r.type), id: r.id };
  }, [hasUser, isSingle, selected]);

  // --- Data fetching ---

  const overview = useMetrics<GlobalStats | ResourceStats | UserDetailStats>(
    async (): Promise<GlobalStats | ResourceStats | UserDetailStats> => {
      if (hasUser) {
        return getUserMetrics(
          selectedUser!,
          from, to,
          userResourceFilter?.type,
          userResourceFilter?.id,
        );
      }
      if (isSingle) {
        const r = selected[0];
        return getResourceMetrics(dbTypeToApiType(r.type), r.id, from, to);
      }
      return getMetricsOverview(from, to);
    },
    `overview:${range}:${hasUser ? `user:${selectedUser}` : ""}:${isSingle ? selectedKey : "global"}`,
  );

  type TimelineResultType =
    | { mode: "global"; data: { timestamp: string; requests: number; errors: number; avg_duration_ms?: number; avg_ttfb?: number | null }[] }
    | { mode: "single"; resourceId: string; data: { timestamp: string; requests: number; errors: number; avg_duration_ms?: number; avg_ttfb?: number | null }[] }
    | { mode: "multi"; resources: ResourceTimeline[] };

  const timelineResult = useMetrics<TimelineResultType>(
    async () => {
      if (hasUser) {
        const data = await getUserTimeline(
          selectedUser!,
          from, to, interval,
          userResourceFilter?.type,
          userResourceFilter?.id,
        );
        return { mode: "global", data };
      }
      if (isGlobal) {
        const data = await getMetricsTimeline(from, to, interval);
        return { mode: "global", data };
      }
      if (isSingle) {
        const r = selected[0];
        const data = await getResourceTimeline(dbTypeToApiType(r.type), r.id, from, to, interval);
        return { mode: "single", resourceId: r.id, data };
      }
      const results = await Promise.all(
        selected.map(async (r) => ({
          id: r.id,
          name: r.name || r.id,
          color: getEntityColor(r.id).avatarFrom,
          data: await getResourceTimeline(dbTypeToApiType(r.type), r.id, from, to, interval),
        })),
      );
      return { mode: "multi", resources: results };
    },
    `timeline:${range}:${hasUser ? `user:${selectedUser}` : ""}:${selectedKey}`,
  );

  const top = useMetrics(
    () => {
      if (hasUser) return getUserTopResources(selectedUser!, from, to, 20);
      return getMetricsTopResources(from, to, 20);
    },
    `top:${range}:${hasUser ? `user:${selectedUser}` : "global"}`,
  );

  const users = useMetrics(
    () => {
      if (hasUser) return Promise.resolve([]);
      if (isSingle) {
        const r = selected[0];
        return getResourceUsers(dbTypeToApiType(r.type), r.id, from, to);
      }
      return getMetricsOverviewUsers(from, to, 20);
    },
    `users:${range}:${hasUser ? "none" : isSingle ? selectedKey : "global"}`,
  );

  // Errors: now works for both single resource AND global view
  const errors = useMetrics(
    () => {
      if (isSingle) {
        const r = selected[0];
        return getResourceErrors(dbTypeToApiType(r.type), r.id, from, to);
      }
      return getMetricsOverviewErrors(from, to);
    },
    `errors:${range}:${isSingle ? selectedKey : "global"}`,
  );

  // Error events (detailed) — also works globally
  const errorEvents = useMetrics<ErrorEvent[]>(
    () => {
      if (isSingle) {
        const r = selected[0];
        return getResourceErrorEvents(dbTypeToApiType(r.type), r.id, from, to, 50);
      }
      return getMetricsOverviewErrorEvents(from, to, 50);
    },
    `error-events:${range}:${isSingle ? selectedKey : "global"}`,
  );

  // --- Derived data ---

  const stats = overview.data;

  const aggregatedStats = useMemo(() => {
    if (!isMulti || !top.data || hasUser) return null;
    const selectedIds = new Set(selected.map((s) => `${s.type}:${s.id}`));
    const filtered = top.data.filter((r: ResourceRanking) => selectedIds.has(`${r.resource_type}:${r.resource_id}`));
    if (filtered.length === 0) return null;
    const totalRequests = filtered.reduce((sum: number, r: ResourceRanking) => sum + r.requests, 0);
    const weightedErrorRate = totalRequests > 0
      ? filtered.reduce((sum: number, r: ResourceRanking) => sum + r.error_rate * r.requests, 0) / totalRequests
      : 0;
    const weightedLatency = totalRequests > 0
      ? filtered.reduce((sum: number, r: ResourceRanking) => sum + r.avg_duration_ms * r.requests, 0) / totalRequests
      : 0;
    return { total_requests: totalRequests, error_rate: weightedErrorRate, avg_duration_ms: weightedLatency };
  }, [isMulti, selected, top.data, hasUser]);

  // displayStats is a union of all possible stats shapes; use Record to allow safe property access
  const displayStats = (hasUser ? stats : (isMulti ? aggregatedStats : stats)) as Record<string, unknown> | null;

  const cards: { label: string; value: string | number; trend?: "up" | "neutral"; subValue?: string }[] = displayStats
    ? [
        { label: "Total Requests", value: (displayStats.total_requests as number)?.toLocaleString() ?? "0" },
        { label: "Error Rate", value: `${((displayStats.error_rate as number) ?? 0).toFixed(1)}%`, trend: ((displayStats.error_rate as number) ?? 0) > 5 ? "up" as const : "neutral" as const },
        { label: isMulti && !hasUser ? "Avg latency" : "P95 latency", value: `${Math.round(((isMulti && !hasUser ? displayStats.avg_duration_ms : displayStats.p95_duration_ms) as number) ?? 0)}ms` },
        ...(!isMulti && displayStats.unique_users != null ? [{ label: "Active users", value: displayStats.unique_users as number }] : []),
        ...(hasUser && displayStats.active_agents != null ? [{ label: "Agents used", value: displayStats.active_agents as number }] : []),
        ...(isMulti && !hasUser ? [{ label: "Resources", value: selected.length }] : []),
        // Token usage card
        ...((displayStats.token_usage as { total?: number; input?: number; output?: number } | undefined)?.total ? [{
          label: "Tokens consumidos",
          value: (displayStats.token_usage as { total: number }).total.toLocaleString(),
          subValue: `In: ${(displayStats.token_usage as { input?: number }).input?.toLocaleString() ?? "0"} / Out: ${(displayStats.token_usage as { output?: number }).output?.toLocaleString() ?? "0"}`,
        }] : []),
      ]
    : [];

  const filteredTop = useMemo(() => {
    const data = top.data ?? [];
    if (isGlobal || hasUser) return data;
    const selectedIds = new Set(selected.map((s) => `${s.type}:${s.id}`));
    return data.filter((r: ResourceRanking) => selectedIds.has(`${r.resource_type}:${r.resource_id}`));
  }, [top.data, selected, isGlobal, hasUser]);

  // Filter users by search text
  const filteredUsers = useMemo(() => {
    const data = users.data ?? [];
    if (!userSearch) return data;
    const q = userSearch.toLowerCase();
    return data.filter((u) => u.user_email.toLowerCase().includes(q));
  }, [users.data, userSearch]);

  const tlData = timelineResult.data;
  const singleColor = isSingle ? getEntityColor(selected[0].id).avatarFrom : undefined;

  // Show errors section when status filter is "error" or always in the errors+users grid
  const showErrorsSection = statusFilter !== "ok";
  const showUsersSection = statusFilter !== "error" && !hasUser;

  return (
    <div className="space-y-6">
      {/* Sticky filter bar */}
      <div className="sticky top-0 z-10 -mx-6 border-b bg-background/95 backdrop-blur px-6 py-3">
        <div className="flex flex-col gap-3">
          <div className="flex items-center gap-2">
            <Filter className="h-4 w-4 text-muted-foreground shrink-0" />
            <h1 className="text-lg font-bold shrink-0">Observability</h1>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {/* Resource selector */}
            <ResourceSelector
              onFilterChange={handleFilterChange}
              initialSelection={initialSelection}
            />

            {/* User search */}
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <input
                type="text"
                placeholder="Search user..."
                value={selectedUser ?? userSearch}
                onChange={(e) => {
                  if (selectedUser) {
                    clearUser();
                  }
                  setUserSearch(e.target.value);
                }}
                className="h-8 w-48 rounded-md border bg-background pl-8 pr-8 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
              />
              {(selectedUser || userSearch) && (
                <button
                  onClick={() => { clearUser(); setUserSearch(""); }}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              )}
            </div>

            {/* Status filter */}
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as StatusFilter)}
              className="h-8 rounded-md border bg-background px-3 text-sm"
            >
              {STATUS_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>

            {/* Time range */}
            <select
              value={range}
              onChange={(e) => setRange(e.target.value as Range)}
              className="h-8 rounded-md border bg-background px-3 text-sm"
            >
              {RANGE_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>

            {/* Clear all */}
            {hasActiveFilters && (
              <button
                onClick={clearAllFilters}
                className="flex items-center gap-1 rounded-md border border-dashed px-2.5 py-1 text-xs text-muted-foreground hover:bg-muted transition-colors"
              >
                <X className="h-3 w-3" />
                Clear filters
              </button>
            )}
          </div>

          {/* Active filter chips */}
          {(selectedUser || statusFilter !== "all") && (
            <div className="flex flex-wrap items-center gap-1.5">
              {selectedUser && (
                <span className="inline-flex items-center gap-1.5 rounded-full bg-primary/10 px-2.5 py-0.5 text-xs font-medium text-primary">
                  User: {selectedUser}
                  <button onClick={clearUser} className="rounded-full p-0.5 hover:bg-primary/20">
                    <X className="h-3 w-3" />
                  </button>
                </span>
              )}
              {statusFilter !== "all" && (
                <span className="inline-flex items-center gap-1.5 rounded-full bg-primary/10 px-2.5 py-0.5 text-xs font-medium text-primary">
                  Status: {STATUS_OPTIONS.find((o) => o.value === statusFilter)?.label}
                  <button onClick={() => setStatusFilter("all")} className="rounded-full p-0.5 hover:bg-primary/20">
                    <X className="h-3 w-3" />
                  </button>
                </span>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Only show full loading on first load */}
      {overview.loading && !overview.data && !aggregatedStats ? (
        <div className="space-y-6" aria-busy="true">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="rounded-lg border p-4 space-y-2">
                <Skeleton className="h-3 w-20" />
                <Skeleton className="h-7 w-16" />
              </div>
            ))}
          </div>
          <div className="grid gap-6 lg:grid-cols-2">
            <Skeleton className="h-48 rounded-lg" />
            <Skeleton className="h-48 rounded-lg" />
          </div>
        </div>
      ) : overview.error && !overview.data ? (
        <div className="text-red-500">Error: {overview.error}</div>
      ) : (
        <>
          {cards.length > 0 && <StatsCards cards={cards} />}

          {/* Timeline + Latency grid */}
          {statusFilter !== "error" && (
            <div className="grid gap-6 lg:grid-cols-2">
              <div className="rounded-lg border p-4">
                <h3 className="mb-4 font-semibold">Activity</h3>
                {tlData?.mode === "multi" ? (
                  <TimelineChart multiData={tlData.resources} range={range} />
                ) : (
                  <TimelineChart data={tlData?.data ?? []} range={range} color={singleColor} />
                )}
              </div>
              <div className="rounded-lg border p-4">
                <h3 className="mb-4 font-semibold">Average latency</h3>
                {tlData?.mode === "multi" ? (
                  <LatencyChart multiData={tlData.resources} range={range} />
                ) : (
                  <LatencyChart data={tlData?.data ?? []} range={range} color={singleColor} />
                )}
              </div>
            </div>
          )}

          {/* Requests by resource */}
          {!isSingle && filteredTop.length > 1 && statusFilter !== "error" && (
            <div className="rounded-lg border p-4">
              <h3 className="mb-4 font-semibold">Requests by resource</h3>
              <RequestsBarChart data={filteredTop} />
            </div>
          )}

          {/* Errors + Users */}
          <div className="grid gap-6 lg:grid-cols-2">
            {showErrorsSection && (
              <div className={!showUsersSection ? "lg:col-span-2" : ""}>
                <h3 className="mb-3 font-semibold">Recent errors</h3>
                <ErrorsTable errors={errors.data ?? []} errorEvents={errorEvents.data ?? []} />
              </div>
            )}
            {showUsersSection && (
              <div className={!showErrorsSection ? "lg:col-span-2" : ""}>
                <h3 className="mb-3 font-semibold">Users</h3>
                {userSearch && !selectedUser && (
                  <p className="mb-2 text-xs text-muted-foreground">
                    Showing results for &quot;{userSearch}&quot;
                  </p>
                )}
                <UsersTable
                  users={filteredUsers}
                  onUserClick={handleUserClick}
                  selectedUser={selectedUser}
                />
              </div>
            )}
          </div>

          {/* Top resources */}
          {statusFilter !== "error" && (
            <ResourceTable title={hasUser ? `Resources for ${selectedUser}` : "Top Resources"} resources={filteredTop} basePath="/admin/observability" />
          )}
        </>
      )}
    </div>
  );
}

export default function ObservabilityPage() {
  return (
    <Suspense fallback={
      <div className="space-y-6 p-6" aria-busy="true">
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="rounded-lg border p-4 space-y-2">
              <Skeleton className="h-3 w-20" />
              <Skeleton className="h-7 w-16" />
            </div>
          ))}
        </div>
        <div className="grid gap-6 lg:grid-cols-2">
          <Skeleton className="h-48 rounded-lg" />
          <Skeleton className="h-48 rounded-lg" />
        </div>
      </div>
    }>
      <ObservabilityContent />
    </Suspense>
  );
}
