"use client";

import { useMemo } from "react";
import { AreaChart, Area, LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid, Legend } from "recharts";
import { generateBuckets, rangeToIntervalMs, tickInterval, type Range, type TimelineBucket, type ResourceTimeline } from "./TimelineChart";

interface LatencyChartProps {
  /** Single resource or global data */
  data?: TimelineBucket[];
  /** Multi-resource overlaid data (one line per resource) */
  multiData?: ResourceTimeline[];
  /** Range for filling time slots */
  range: Range;
  /** Color for single series */
  color?: string;
  height?: number;
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

function indexByBucket(data: TimelineBucket[], intervalMs: number): Map<number, TimelineBucket> {
  const map = new Map<number, TimelineBucket>();
  for (const d of data) {
    const key = Math.floor(new Date(d.timestamp).getTime() / intervalMs) * intervalMs;
    map.set(key, d);
  }
  return map;
}

export function LatencyChart({ data, multiData, range, color, height = 300 }: LatencyChartProps) {
  const intervalMs = rangeToIntervalMs(range);
  const allBuckets = useMemo(() => generateBuckets(range), [range]);

  // Multi-resource: one line per resource
  if (multiData && multiData.length > 0) {
    const indices = multiData.map((r) => ({
      ...r,
      index: indexByBucket(r.data, intervalMs),
    }));

    const chartData = allBuckets.map((ts) => {
      const bucketKey = Math.floor(new Date(ts).getTime() / intervalMs) * intervalMs;
      const row: Record<string, number | string> = { timestamp: ts };
      for (const r of indices) {
        const bucket = r.index.get(bucketKey);
        row[r.name] = bucket?.avg_duration_ms ? Math.round(bucket.avg_duration_ms) : 0;
      }
      return row;
    });

    const xInterval = tickInterval(chartData.length);

    return (
      <ResponsiveContainer width="100%" height={height}>
        <LineChart data={chartData} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
          <XAxis dataKey="timestamp" tickFormatter={(ts) => formatTime(ts, range)} className="text-xs" tick={{ fontSize: 11 }} interval={xInterval} />
          <YAxis className="text-xs" tick={{ fontSize: 11 }} unit="ms" />
          <Tooltip
            labelFormatter={(label) => formatTime(String(label), range)}
            formatter={(value) => [`${value}ms`]}
            contentStyle={{ backgroundColor: "var(--color-card)", border: "1px solid var(--color-border)", borderRadius: "8px", color: "var(--color-card-foreground)", boxShadow: "0 4px 12px rgba(0,0,0,0.15)" }}
            labelStyle={{ color: "var(--color-card-foreground)" }}
            itemStyle={{ color: "var(--color-card-foreground)" }}
          />
          {multiData.length > 1 && <Legend />}
          {multiData.map((r) => (
            <Line key={r.id} type="monotone" dataKey={r.name} stroke={r.color} strokeWidth={2} dot={false} />
          ))}
        </LineChart>
      </ResponsiveContainer>
    );
  }

  // Single series: area chart with filled time slots
  const index = indexByBucket(data ?? [], intervalMs);
  const chartData = allBuckets.map((ts) => {
    const bucketKey = Math.floor(new Date(ts).getTime() / intervalMs) * intervalMs;
    const bucket = index.get(bucketKey);
    return {
      timestamp: ts,
      latencia: bucket?.avg_duration_ms ? Math.round(bucket.avg_duration_ms) : 0,
    };
  });

  const strokeColor = color ?? "hsl(30, 90%, 55%)";
  const xInterval = tickInterval(chartData.length);

  return (
    <ResponsiveContainer width="100%" height={height}>
      <AreaChart data={chartData} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
        <XAxis dataKey="timestamp" tickFormatter={(ts) => formatTime(ts, range)} className="text-xs" tick={{ fontSize: 11 }} interval={xInterval} />
        <YAxis className="text-xs" tick={{ fontSize: 11 }} unit="ms" />
        <Tooltip
          labelFormatter={(label) => formatTime(String(label), range)}
          formatter={(value) => [`${value}ms`, "Latency"]}
          contentStyle={{ backgroundColor: "var(--color-card)", border: "1px solid var(--color-border)", borderRadius: "8px", color: "var(--color-card-foreground)" }}
          labelStyle={{ color: "var(--color-card-foreground)" }}
          itemStyle={{ color: "var(--color-card-foreground)" }}
        />
        <Area type="monotone" dataKey="latencia" stroke={strokeColor} fill={strokeColor} fillOpacity={0.2} name="Average latency" />
      </AreaChart>
    </ResponsiveContainer>
  );
}
