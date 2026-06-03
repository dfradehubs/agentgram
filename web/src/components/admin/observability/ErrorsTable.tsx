"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import type { ErrorEvent } from "@/lib/types";

interface ErrorRow {
  error_type: string;
  count: number;
  last_seen: string;
  last_msg?: string;
}

interface ErrorsTableProps {
  errors: ErrorRow[];
  errorEvents?: ErrorEvent[];
}

export function ErrorsTable({ errors, errorEvents }: ErrorsTableProps) {
  const [expandedType, setExpandedType] = useState<string | null>(null);

  if (!errors || errors.length === 0) {
    return <p className="text-sm text-muted-foreground">No errors in the period</p>;
  }

  return (
    <div className="overflow-x-auto rounded-lg border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b text-left text-muted-foreground">
            <th className="w-8 px-2 py-2" />
            <th className="px-4 py-2 font-medium">Type</th>
            <th className="px-4 py-2 font-medium text-right">Count</th>
            <th className="px-4 py-2 font-medium">Last</th>
            <th className="px-4 py-2 font-medium">Message</th>
          </tr>
        </thead>
        <tbody>
          {errors.map((row) => {
            const isExpanded = expandedType === row.error_type;
            const relatedEvents = errorEvents?.filter((e) => e.error_type === row.error_type) ?? [];

            return (
              <ErrorTypeRow
                key={row.error_type}
                row={row}
                isExpanded={isExpanded}
                relatedEvents={relatedEvents}
                hasEvents={!!errorEvents && errorEvents.length > 0}
                onToggle={() => setExpandedType(isExpanded ? null : row.error_type)}
              />
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function ErrorTypeRow({
  row,
  isExpanded,
  relatedEvents,
  hasEvents,
  onToggle,
}: {
  row: { error_type: string; count: number; last_seen: string; last_msg?: string };
  isExpanded: boolean;
  relatedEvents: ErrorEvent[];
  hasEvents: boolean;
  onToggle: () => void;
}) {
  return (
    <>
      <tr className="border-b last:border-0 hover:bg-muted/50 cursor-pointer" onClick={onToggle}>
        <td className="px-2 py-2 text-center">
          {hasEvents && (
            isExpanded ? <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" /> : <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
          )}
        </td>
        <td className="px-4 py-2">
          <span className="rounded bg-red-500/10 px-2 py-0.5 text-xs font-medium text-red-500">
            {row.error_type}
          </span>
        </td>
        <td className="px-4 py-2 text-right font-mono">{row.count}</td>
        <td className="px-4 py-2 text-muted-foreground">
          {new Date(row.last_seen).toLocaleString("en", { dateStyle: "short", timeStyle: "short" })}
        </td>
        <td className="max-w-xs truncate px-4 py-2 text-muted-foreground">{row.last_msg || "-"}</td>
      </tr>
      {isExpanded && relatedEvents.length > 0 && (
        <tr>
          <td colSpan={5} className="px-0 py-0">
            <div className="bg-muted/30 px-6 py-3 space-y-2">
              {relatedEvents.map((evt) => (
                <div key={evt.id} className="rounded border bg-background p-3 text-xs space-y-1">
                  <div className="flex items-center gap-3 text-muted-foreground">
                    <span>{new Date(evt.timestamp).toLocaleString("en", { dateStyle: "short", timeStyle: "medium" })}</span>
                    <span>{evt.user_email}</span>
                    <span className="font-mono">{evt.duration_ms}ms</span>
                    {evt.resource_name && <span className="text-foreground font-medium">{evt.resource_name}</span>}
                  </div>
                  <p className="whitespace-pre-wrap text-destructive">{evt.error_msg}</p>
                </div>
              ))}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}
