"use client";

import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid } from "recharts";

interface ToolCallStat {
  name: string;
  count: number;
  avg_duration_ms?: number;
}

interface ToolsChartProps {
  tools: ToolCallStat[];
  height?: number;
}

export function ToolsChart({ tools, height = 300 }: ToolsChartProps) {
  if (!tools || tools.length === 0) {
    return <div className="flex h-48 items-center justify-center text-muted-foreground">No tool data</div>;
  }

  const sorted = [...tools].sort((a, b) => b.count - a.count).slice(0, 15);

  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart data={sorted} layout="vertical" margin={{ top: 5, right: 20, left: 10, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-muted" horizontal={false} />
        <XAxis type="number" tick={{ fontSize: 11 }} />
        <YAxis dataKey="name" type="category" width={160} tick={{ fontSize: 11 }} />
        <Tooltip
          contentStyle={{ backgroundColor: "var(--color-card)", border: "1px solid var(--color-border)", borderRadius: "8px", color: "var(--color-card-foreground)", boxShadow: "0 4px 12px rgba(0,0,0,0.15)" }}
          labelStyle={{ color: "var(--color-card-foreground)" }}
          itemStyle={{ color: "var(--color-card-foreground)" }}
          formatter={(value, name) => {
            const v = Number(value ?? 0);
            if (name === "count") return [v.toLocaleString(), "Llamadas"];
            if (name === "avg_duration_ms") return [`${Math.round(v)}ms`, "Average latency"];
            return [v, String(name)];
          }}
        />
        <Bar dataKey="count" fill="var(--color-primary)" radius={[0, 4, 4, 0]} name="count" />
      </BarChart>
    </ResponsiveContainer>
  );
}
