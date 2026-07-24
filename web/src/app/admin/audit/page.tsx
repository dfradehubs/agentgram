"use client";

import { useEffect, useState, useCallback, Fragment } from "react";
import type { AuditEvent } from "@/lib/types";
import { getAuditEvents } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ChevronRight, RefreshCw } from "lucide-react";
import { AdminTableSkeleton } from "@/components/admin/AdminTableSkeleton";

const RANGES: { label: string; minutes: number }[] = [
  { label: "Last 30 minutes", minutes: 30 },
  { label: "Last hour", minutes: 60 },
  { label: "Last 6 hours", minutes: 360 },
  { label: "Last 12 hours", minutes: 720 },
  { label: "Last 24 hours", minutes: 1440 },
  { label: "Last 7 days", minutes: 10080 },
];

const CATEGORIES = ["", "agent", "mcp", "skill", "group"];
const MAX_OPTIONS = [50, 100, 250, 500];

function fmtDate(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleString(undefined, { day: "2-digit", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });
}

export default function AdminAuditPage() {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [expanded, setExpanded] = useState<string | null>(null);

  const [rangeMinutes, setRangeMinutes] = useState(1440);
  const [user, setUser] = useState("");
  const [category, setCategory] = useState("");
  const [maxResults, setMaxResults] = useState(50);
  const [offset, setOffset] = useState(0);

  const fetchEvents = useCallback(async () => {
    setLoading(true);
    try {
      const from = new Date(Date.now() - rangeMinutes * 60 * 1000).toISOString();
      const data = await getAuditEvents({
        from,
        user,
        resource_type: category,
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
  }, [rangeMinutes, user, category, maxResults, offset]);

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
            {RANGES.map((r) => <option key={r.minutes} value={r.minutes}>{r.label}</option>)}
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
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">Max</label>
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
        <AdminTableSkeleton columns={4} rows={6} />
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="px-4 py-3 text-left font-medium">Date</th>
                <th className="px-4 py-3 text-left font-medium">Action</th>
                <th className="px-4 py-3 text-left font-medium">Member</th>
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
                      <td className="px-4 py-3">
                        <span className="rounded bg-muted px-2 py-0.5 text-xs capitalize">{e.resource_type}</span>
                      </td>
                    </tr>
                    {isOpen && (
                      <tr className="bg-muted/20">
                        <td colSpan={4} className="px-6 py-4">
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
      {total > maxResults && (
        <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
          <span>{offset + 1}–{Math.min(offset + maxResults, total)} of {total}</span>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - maxResults))}>Previous</Button>
            <Button variant="outline" size="sm" disabled={offset + maxResults >= total} onClick={() => setOffset(offset + maxResults)}>Next</Button>
          </div>
        </div>
      )}
    </div>
  );
}
