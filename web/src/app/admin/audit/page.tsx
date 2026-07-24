"use client";

import { useEffect, useState, useCallback, Fragment } from "react";
import type { AuditEvent } from "@/lib/types";
import { getAuditEvents, getAdminSettings } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ChevronRight, ChevronLeft, ChevronsLeft, ChevronsRight, RefreshCw } from "lucide-react";
import { AdminTableSkeleton } from "@/components/admin/AdminTableSkeleton";

const BASE_RANGES: { label: string; minutes: number }[] = [
  { label: "Last 30 minutes", minutes: 30 },
  { label: "Last hour", minutes: 60 },
  { label: "Last 6 hours", minutes: 360 },
  { label: "Last 12 hours", minutes: 720 },
  { label: "Last 24 hours", minutes: 1440 },
  { label: "Last 7 days", minutes: 10080 },
  { label: "Last 14 days", minutes: 20160 },
  { label: "Last 30 days", minutes: 43200 },
  { label: "Last 60 days", minutes: 86400 },
  { label: "Last 90 days", minutes: 129600 },
  { label: "Last 180 days", minutes: 259200 },
  { label: "Last 365 days", minutes: 525600 },
];

// buildRanges caps the selectable range to the audit retention window (events
// older than retention are deleted, so offering a longer range is pointless),
// and always ends with an option covering exactly the retention window.
function buildRanges(retentionDays: number): { label: string; minutes: number }[] {
  const maxMin = Math.max(1, retentionDays) * 1440;
  const opts = BASE_RANGES.filter((r) => r.minutes <= maxMin);
  if (!opts.some((r) => r.minutes === maxMin)) {
    opts.push({ label: `Last ${retentionDays} days`, minutes: maxMin });
  }
  return opts;
}

const CATEGORIES = ["", "agent", "mcp", "skill", "group"];
const MAX_OPTIONS = [50, 100, 250, 500];

function fmtDate(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleString(undefined, { day: "2-digit", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });
}

// pageItems builds the page numbers to show, inserting "…" (ellipsis) where the
// sequence skips: always first + last, plus a window around the current page.
function pageItems(current: number, totalPages: number): (number | "…")[] {
  if (totalPages <= 7) return Array.from({ length: totalPages }, (_, i) => i + 1);
  const items: (number | "…")[] = [1];
  const start = Math.max(2, current - 1);
  const end = Math.min(totalPages - 1, current + 1);
  if (start > 2) items.push("…");
  for (let p = start; p <= end; p++) items.push(p);
  if (end < totalPages - 1) items.push("…");
  items.push(totalPages);
  return items;
}

export default function AdminAuditPage() {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [expanded, setExpanded] = useState<string | null>(null);

  const [rangeMinutes, setRangeMinutes] = useState(1440);
  const [user, setUser] = useState("");
  const [category, setCategory] = useState("");
  const [session, setSession] = useState("");
  const [maxResults, setMaxResults] = useState(50);
  const [offset, setOffset] = useState(0);
  const [retentionDays, setRetentionDays] = useState(30);

  // Load the audit retention so the range selector can't exceed it.
  useEffect(() => {
    getAdminSettings()
      .then((settings) => {
        const r = settings.find((s) => s.key === "audit_retention_days");
        if (r) setRetentionDays(Number(r.value || r.default) || 30);
      })
      .catch(() => {
        /* keep default */
      });
  }, []);

  const ranges = buildRanges(retentionDays);

  const fetchEvents = useCallback(async () => {
    setLoading(true);
    try {
      const from = new Date(Date.now() - rangeMinutes * 60 * 1000).toISOString();
      const data = await getAuditEvents({
        from,
        user,
        resource_type: category,
        session,
        limit: String(maxResults),
        offset: String(offset),
      });
      setEvents(data.events);
      setTotal(data.total);
    } catch {
      setEvents([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [rangeMinutes, user, category, session, maxResults, offset]);

  useEffect(() => {
    fetchEvents();
  }, [fetchEvents]);

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Audit</h1>
        <Button variant="outline" size="sm" onClick={() => fetchEvents()}>
          <RefreshCw className="mr-1 h-4 w-4" />
          Refresh
        </Button>
      </div>

      {/* Filters */}
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">Range</label>
          <select
            className="rounded-md border bg-background px-3 py-1.5 text-sm"
            value={rangeMinutes}
            onChange={(e) => { setOffset(0); setRangeMinutes(Number(e.target.value)); }}
          >
            {ranges.map((r) => <option key={r.minutes} value={r.minutes}>{r.label}</option>)}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">Member</label>
          <input
            className="rounded-md border bg-background px-3 py-1.5 text-sm"
            placeholder="email"
            value={user}
            onChange={(e) => { setOffset(0); setUser(e.target.value); }}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">Category</label>
          <select
            className="rounded-md border bg-background px-3 py-1.5 text-sm"
            value={category}
            onChange={(e) => { setOffset(0); setCategory(e.target.value); }}
          >
            {CATEGORIES.map((c) => <option key={c} value={c}>{c === "" ? "All categories" : c}</option>)}
          </select>
        </div>
        <div className="flex-1">
          <label className="mb-1 block text-xs font-medium text-muted-foreground">Session</label>
          <input
            className="w-full rounded-md border bg-background px-3 py-1.5 text-sm"
            placeholder="Search by session ID"
            value={session}
            onChange={(e) => { setOffset(0); setSession(e.target.value); }}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">Per page</label>
          <select
            className="rounded-md border bg-background px-3 py-1.5 text-sm"
            value={maxResults}
            onChange={(e) => { setOffset(0); setMaxResults(Number(e.target.value)); }}
          >
            {MAX_OPTIONS.map((n) => <option key={n} value={n}>{n}</option>)}
          </select>
        </div>
      </div>

      {loading ? (
        <AdminTableSkeleton columns={5} rows={6} />
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="px-4 py-3 text-left font-medium">Date</th>
                <th className="px-4 py-3 text-left font-medium">Action</th>
                <th className="px-4 py-3 text-left font-medium">Member</th>
                <th className="px-4 py-3 text-left font-medium">Session</th>
                <th className="px-4 py-3 text-left font-medium">Category</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {events.map((e) => {
                const isOpen = expanded === e.id;
                return (
                  <Fragment key={e.id}>
                    <tr
                      className="cursor-pointer hover:bg-muted/30"
                      onClick={() => setExpanded(isOpen ? null : e.id)}
                    >
                      <td className="whitespace-nowrap px-4 py-3 text-xs text-muted-foreground">{fmtDate(e.created_at)}</td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5">
                          <ChevronRight className={`h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform ${isOpen ? "rotate-90" : ""}`} />
                          <span className="font-medium">{e.resource_name || e.resource_id}</span>
                          {e.status === "error" && (
                            <span className="rounded bg-destructive/15 px-1.5 py-0.5 text-[10px] font-medium text-destructive">error</span>
                          )}
                        </div>
                        <div className="ml-5 text-xs text-muted-foreground">
                          {e.action} · {e.duration_ms}ms · {e.source}
                          {e.client ? ` · ${e.client.length > 40 ? e.client.slice(0, 40) + "…" : e.client}` : ""}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-xs">{e.user_email}</td>
                      <td className="px-4 py-3 text-xs">
                        {e.session_id ? (
                          <button
                            className="font-mono text-muted-foreground hover:text-foreground hover:underline"
                            title="Filter by this session"
                            onClick={(ev) => { ev.stopPropagation(); setOffset(0); setSession(e.session_id!); }}
                          >
                            {e.session_id.slice(0, 8)}…
                          </button>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span className="rounded bg-muted px-2 py-0.5 text-xs capitalize">{e.resource_type}</span>
                      </td>
                    </tr>
                    {isOpen && (
                      <tr className="bg-muted/20">
                        <td colSpan={5} className="px-6 py-4">
                          <div className="space-y-3 text-xs">
                            <div className="flex flex-wrap gap-x-4 gap-y-1 text-muted-foreground">
                              {e.client && <span><span className="font-semibold">Client:</span> {e.client}</span>}
                              {e.session_id && <span><span className="font-semibold">Session:</span> <span className="font-mono">{e.session_id}</span></span>}
                              {e.llm_model && <span><span className="font-semibold">Model:</span> {e.llm_model}</span>}
                              {e.token_usage && e.token_usage.total > 0 && (
                                <span><span className="font-semibold">Tokens:</span> {e.token_usage.input}/{e.token_usage.output} ({e.token_usage.total})</span>
                              )}
                            </div>
                            {e.tool_calls && e.tool_calls.length > 0 && (
                              <div>
                                <div className="mb-1 font-semibold">Tool calls</div>
                                <div className="space-y-1">
                                  {e.tool_calls.map((tc, i) => (
                                    <details key={i} className="rounded bg-background p-2">
                                      <summary className="cursor-pointer font-mono">{tc.name}</summary>
                                      {tc.arguments && <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap text-[11px] text-muted-foreground">args: {tc.arguments}</pre>}
                                      {tc.result && <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap text-[11px] text-muted-foreground">result: {tc.result}</pre>}
                                    </details>
                                  ))}
                                </div>
                              </div>
                            )}
                            {e.prompt && (
                              <div>
                                <div className="mb-1 font-semibold">Prompt</div>
                                <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded bg-background p-3">{e.prompt}</pre>
                              </div>
                            )}
                            {e.response && (
                              <div>
                                <div className="mb-1 font-semibold">Response</div>
                                <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded bg-background p-3">{e.response}</pre>
                              </div>
                            )}
                            {e.error_msg && (
                              <div>
                                <div className="mb-1 font-semibold text-destructive">Error</div>
                                <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded bg-background p-3 text-destructive">{e.error_msg}</pre>
                              </div>
                            )}
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
          {events.length === 0 && (
            <div className="px-4 py-8 text-center text-muted-foreground">No audit events for this filter</div>
          )}
        </div>
      )}

      {/* Pagination */}
      {(() => {
        const totalPages = Math.max(1, Math.ceil(total / maxResults));
        const page = Math.floor(offset / maxResults) + 1;
        if (totalPages <= 1) return null;
        const goTo = (p: number) => setOffset((Math.min(Math.max(1, p), totalPages) - 1) * maxResults);
        const navBtn = "rounded px-2 py-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:pointer-events-none disabled:opacity-40";
        return (
          <div className="mt-4 flex flex-wrap items-center justify-center gap-1 text-sm">
            <button className={`${navBtn} flex items-center gap-0.5`} disabled={page === 1} onClick={() => goTo(1)}>
              <ChevronsLeft className="h-3.5 w-3.5" /> First
            </button>
            <button className={navBtn} disabled={page === 1} onClick={() => goTo(page - 1)} aria-label="Previous page">
              <ChevronLeft className="h-4 w-4" />
            </button>
            {pageItems(page, totalPages).map((it, i) =>
              it === "…" ? (
                <span key={`e${i}`} className="px-1.5 text-muted-foreground">…</span>
              ) : (
                <button
                  key={it}
                  onClick={() => goTo(it)}
                  className={`min-w-[2rem] rounded px-2 py-1 transition-colors ${
                    it === page ? "bg-primary/10 font-medium text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                  }`}
                >
                  {it}
                </button>
              )
            )}
            <button className={navBtn} disabled={page === totalPages} onClick={() => goTo(page + 1)} aria-label="Next page">
              <ChevronRight className="h-4 w-4" />
            </button>
            <button className={`${navBtn} flex items-center gap-0.5`} disabled={page === totalPages} onClick={() => goTo(totalPages)}>
              Last <ChevronsRight className="h-3.5 w-3.5" />
            </button>
          </div>
        );
      })()}
    </div>
  );
}
