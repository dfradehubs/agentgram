"use client";

import { useMemo } from "react";
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid, Legend } from "recharts";

export interface TimelineBucket {
  timestamp: string;
  requests: number;
  errors: number;
  avg_duration_ms?: number;
  avg_ttfb?: number | null;
}

export type Range = "1h" | "4h" | "12h" | "24h" | "7d" | "30d" | "90d";

export interface ResourceTimeline {
  id: string;
  name: string;
  color: string;
  data: TimelineBucket[];
}

interface TimelineChartProps {
  /** Single resource or global data */
  data?: TimelineBucket[];
  /** Multi-resource overlaid data (stacked bars) */
  multiData?: ResourceTimeline[];
  /** Range for filling empty time slots */
  range: Range;
  /** Color for single-series bars. Defaults to blue-500. */
  color?: string;
  height?: number;
}

export function rangeToIntervalMs(range: Range): number {
  switch (range) {
    case "1h": return 300_000;       // 5m
    case "4h": return 900_000;       // 15m
    case "12h": return 1_800_000;    // 30m
    case "24h": return 3_600_000;    // 1h
    case "7d": return 86_400_000;    // 1d
    case "30d": return 86_400_000;   // 1d
    case "90d": return 86_400_000;   // 1d
  }
}

function rangeTotalMs(range: Range): number {
  switch (range) {
    case "1h": return 3_600_000;
    case "4h": return 4 * 3_600_000;
    case "12h": return 12 * 3_600_000;
    case "24h": return 24 * 3_600_000;
    case "7d": return 7 * 86_400_000;
    case "30d": return 30 * 86_400_000;
    case "90d": return 90 * 86_400_000;
  }
}

function formatTime(ts: string, range: Range): string {
  const d = new Date(ts);
  switch (range) {
    case "1h":
    case "4h":
    case "12h":
    case "24h":
      return d.toLocaleString("en", { hour: "2-digit", minute: "2-digit" });
    case "7d":
      return d.toLocaleString("en", { weekday: "short", day: "numeric" });
    case "30d":
    case "90d":
      return d.toLocaleString("en", { month: "short", day: "numeric" });
  }
}

/** Calculate a fixed tick interval so labels are equidistant (max ~12 labels) */
export function tickInterval(totalBuckets: number, maxTicks = 12): number {
  if (totalBuckets <= maxTicks) return 0; // show all
  return Math.ceil(totalBuckets / maxTicks);
}

/** Generate all bucket timestamps for the given range */
export function generateBuckets(range: Range): string[] {
  const intervalMs = rangeToIntervalMs(range);
  const now = Date.now();
  const endBucket = Math.floor(now / intervalMs) * intervalMs;
  const startBucket = endBucket - rangeTotalMs(range) + intervalMs;

  const buckets: string[] = [];
  for (let t = startBucket; t <= endBucket; t += intervalMs) {
    buckets.push(new Date(t).toISOString());
  }
  return buckets;
}

/** Index data by bucket key (rounded to interval) */
function indexByBucket(data: TimelineBucket[], intervalMs: number): Map<number, TimelineBucket> {
  const map = new Map<number, TimelineBucket>();
  for (const d of data) {
    const key = Math.floor(new Date(d.timestamp).getTime() / intervalMs) * intervalMs;
    map.set(key, d);
  }
  return map;
}

export function TimelineChart({ data, multiData, range, color, height = 300 }: TimelineChartProps) {
  const intervalMs = rangeToIntervalMs(range);
  const allBuckets = useMemo(() => generateBuckets(range), [range]);

  const { chartData, bars } = useMemo(() => {
    if (multiData && multiData.length > 0) {
      // Multi-resource mode: stacked bars (breakdown by resource)
      const indices = multiData.map((r) => ({
        ...r,
        index: indexByBucket(r.data, intervalMs),
      }));

      const chartData = allBuckets.map((ts) => {
        const bucketKey = Math.floor(new Date(ts).getTime() / intervalMs) * intervalMs;
        const row: Record<string, number | string> = { timestamp: ts };
        for (const r of indices) {
          const bucket = r.index.get(bucketKey);
          row[r.name] = bucket?.requests ?? 0;
        }
        return row;
      });

      const bars = multiData.map((r) => ({
        dataKey: r.name,
        color: r.color,
        stackId: "stack",
      }));

      return { chartData, bars };
    }

    // Single series mode (requests + errors stacked)
    const index = indexByBucket(data ?? [], intervalMs);
    const chartData = allBuckets.map((ts) => {
      const bucketKey = Math.floor(new Date(ts).getTime() / intervalMs) * intervalMs;
      const bucket = index.get(bucketKey);
      return {
        timestamp: ts,
        requests: bucket?.requests ?? 0,
        errors: bucket?.errors ?? 0,
      };
    });

    const fillColor = color ?? "#3b82f6";
    const bars = [
      { dataKey: "requests", color: fillColor, stackId: "a" },
      { dataKey: "errors", color: "#ef4444", stackId: "a" },
    ];

    return { chartData, bars };
  }, [data, multiData, allBuckets, intervalMs, color]);

  const xInterval = tickInterval(chartData.length);

  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart data={chartData} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
        <XAxis
          dataKey="timestamp"
          tickFormatter={(ts) => formatTime(ts, range)}
          className="text-xs"
          tick={{ fontSize: 11 }}
          interval={xInterval}
        />
        <YAxis className="text-xs" tick={{ fontSize: 11 }} allowDecimals={false} />
        <Tooltip
          labelFormatter={(label) => formatTime(String(label), range)}
          contentStyle={{ backgroundColor: "var(--color-card)", border: "1px solid var(--color-border)", borderRadius: "8px", color: "var(--color-card-foreground)", boxShadow: "0 4px 12px rgba(0,0,0,0.15)" }}
          labelStyle={{ color: "var(--color-card-foreground)" }}
          itemStyle={{ color: "var(--color-card-foreground)" }}
        />
        {multiData && multiData.length > 1 && <Legend />}
        {bars.map((bar) => (
          <Bar
            key={bar.dataKey}
            dataKey={bar.dataKey}
            fill={bar.color}
            stackId={bar.stackId}
            name={bar.dataKey === "requests" ? "Requests" : bar.dataKey === "errors" ? "Errors" : bar.dataKey}
            radius={[2, 2, 0, 0]}
          />
        ))}
      </BarChart>
    </ResponsiveContainer>
  );
}
