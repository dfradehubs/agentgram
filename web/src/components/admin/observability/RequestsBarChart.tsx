"use client";

import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid, Cell } from "recharts";
import { getEntityColor } from "@/lib/agent-colors";

interface ResourceRow {
  resource_type: string;
  resource_id: string;
  resource_name: string;
  requests: number;
  error_rate: number;
  avg_duration_ms: number;
}

interface RequestsBarChartProps {
  data: ResourceRow[];
  height?: number;
}

export function RequestsBarChart({ data, height = 300 }: RequestsBarChartProps) {
  if (!data || data.length === 0) {
    return <div className="flex h-48 items-center justify-center text-muted-foreground">No data</div>;
  }

  const chartData = data
    .slice(0, 10)
    .map((d) => ({
      name: d.resource_name || d.resource_id,
      id: d.resource_id,
      requests: d.requests,
    }));

  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart data={chartData} layout="vertical" margin={{ top: 5, right: 20, left: 10, bottom: 5 }}>
        <CartesianGrid strokeDasharray="3 3" className="stroke-muted" horizontal={false} />
        <XAxis type="number" className="text-xs" tick={{ fontSize: 11 }} />
        <YAxis
          type="category"
          dataKey="name"
          width={140}
          className="text-xs"
          tick={{ fontSize: 11 }}
        />
        <Tooltip
          contentStyle={{ backgroundColor: "var(--color-card)", border: "1px solid var(--color-border)", borderRadius: "8px", color: "var(--color-card-foreground)", boxShadow: "0 4px 12px rgba(0,0,0,0.15)" }}
          labelStyle={{ color: "var(--color-card-foreground)" }}
          itemStyle={{ color: "var(--color-card-foreground)" }}
        />
        <Bar dataKey="requests" radius={[0, 4, 4, 0]} name="Requests">
          {chartData.map((entry) => (
            <Cell key={entry.id} fill={getEntityColor(entry.id).avatarFrom} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  );
}
