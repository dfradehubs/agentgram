"use client";

import { memo, useState } from "react";
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  LineChart,
  Line,
  PieChart,
  Pie,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  Cell,
  Legend,
} from "recharts";
import { ChevronDown, ChevronRight, BarChart2 } from "lucide-react";
import type { ChartData } from "@/lib/types";
import { getEntityColor } from "@/lib/agent-colors";

// Default chart color palette
const CHART_COLORS = [
  "#3b82f6", "#10b981", "#8b5cf6", "#f59e0b", "#f43f5e",
  "#06b6d4", "#6366f1", "#14b8a6", "#f97316", "#ec4899",
];

function getColor(index: number, datasetColor?: string): string {
  if (datasetColor) return datasetColor;
  return CHART_COLORS[index % CHART_COLORS.length];
}

const tooltipStyle = {
  contentStyle: {
    backgroundColor: "var(--color-card)",
    border: "1px solid var(--color-border)",
    borderRadius: "8px",
    color: "var(--color-card-foreground)",
    fontSize: "12px",
    boxShadow: "0 4px 12px rgba(0,0,0,0.15)",
  },
  labelStyle: { color: "var(--color-card-foreground)" },
  itemStyle: { color: "var(--color-card-foreground)" },
};

function buildRechartsData(chart: ChartData): Record<string, unknown>[] {
  return chart.labels.map((label, i) => {
    const point: Record<string, unknown> = { name: label };
    for (const ds of chart.datasets) {
      point[ds.label] = ds.data[i] ?? 0;
    }
    return point;
  });
}

function RenderBarChart({ chart }: { chart: ChartData }) {
  const data = buildRechartsData(chart);
  const isHorizontal = chart.options?.horizontal;

  return (
    <ResponsiveContainer width="100%" height={280}>
      <BarChart data={data} layout={isHorizontal ? "vertical" : "horizontal"}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
        {isHorizontal ? (
          <>
            <XAxis type="number" tick={{ fontSize: 11, fill: "var(--color-muted-foreground)" }} stroke="var(--color-muted-foreground)" />
            <YAxis dataKey="name" type="category" tick={{ fontSize: 11, fill: "var(--color-muted-foreground)" }} stroke="var(--color-muted-foreground)" width={80} />
          </>
        ) : (
          <>
            <XAxis dataKey="name" tick={{ fontSize: 11, fill: "var(--color-muted-foreground)" }} stroke="var(--color-muted-foreground)" />
            <YAxis tick={{ fontSize: 11, fill: "var(--color-muted-foreground)" }} stroke="var(--color-muted-foreground)" />
          </>
        )}
        <Tooltip {...tooltipStyle} />
        {chart.options?.showLegend && <Legend wrapperStyle={{ fontSize: "11px", color: "var(--color-muted-foreground)" }} />}
        {chart.datasets.map((ds, i) => (
          <Bar key={ds.label} dataKey={ds.label} fill={getColor(i, ds.color)} radius={[4, 4, 0, 0]} />
        ))}
      </BarChart>
    </ResponsiveContainer>
  );
}

function RenderLineChart({ chart }: { chart: ChartData }) {
  const data = buildRechartsData(chart);

  return (
    <ResponsiveContainer width="100%" height={280}>
      <LineChart data={data}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
        <XAxis dataKey="name" tick={{ fontSize: 11, fill: "var(--color-muted-foreground)" }} stroke="var(--color-muted-foreground)" />
        <YAxis tick={{ fontSize: 11, fill: "var(--color-muted-foreground)" }} stroke="var(--color-muted-foreground)" />
        <Tooltip {...tooltipStyle} />
        {chart.options?.showLegend && <Legend wrapperStyle={{ fontSize: "11px", color: "var(--color-muted-foreground)" }} />}
        {chart.datasets.map((ds, i) => (
          <Line key={ds.label} type="monotone" dataKey={ds.label} stroke={getColor(i, ds.color)} strokeWidth={2} dot={{ r: 3 }} />
        ))}
      </LineChart>
    </ResponsiveContainer>
  );
}

function RenderPieChart({ chart }: { chart: ChartData }) {
  const data = chart.labels.map((label, i) => ({
    name: label,
    value: chart.datasets[0]?.data[i] ?? 0,
  }));

  return (
    <ResponsiveContainer width="100%" height={280}>
      <PieChart>
        <Pie
          data={data}
          dataKey="value"
          nameKey="name"
          cx="50%"
          cy="50%"
          outerRadius={100}
          label={({ name, percent }) => `${name} (${((percent ?? 0) * 100).toFixed(0)}%)`}
          labelLine={{ stroke: "var(--color-muted-foreground)" }}
          fontSize={11}
          fill="var(--color-muted-foreground)"
        >
          {data.map((entry, i) => (
            <Cell key={entry.name} fill={getColor(i, chart.datasets[0]?.color)} />
          ))}
        </Pie>
        <Tooltip {...tooltipStyle} />
      </PieChart>
    </ResponsiveContainer>
  );
}

function RenderAreaChart({ chart }: { chart: ChartData }) {
  const data = buildRechartsData(chart);

  return (
    <ResponsiveContainer width="100%" height={280}>
      <AreaChart data={data}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
        <XAxis dataKey="name" tick={{ fontSize: 11, fill: "var(--color-muted-foreground)" }} stroke="var(--color-muted-foreground)" />
        <YAxis tick={{ fontSize: 11, fill: "var(--color-muted-foreground)" }} stroke="var(--color-muted-foreground)" />
        <Tooltip {...tooltipStyle} />
        {chart.options?.showLegend && <Legend wrapperStyle={{ fontSize: "11px", color: "var(--color-muted-foreground)" }} />}
        {chart.datasets.map((ds, i) => (
          <Area key={ds.label} type="monotone" dataKey={ds.label} fill={getColor(i, ds.color)} fillOpacity={0.2} stroke={getColor(i, ds.color)} strokeWidth={2} />
        ))}
      </AreaChart>
    </ResponsiveContainer>
  );
}

export const ChartBlock = memo(function ChartBlock({
  chart,
  agentId,
  defaultExpanded = true,
}: {
  chart: ChartData;
  agentId?: string;
  defaultExpanded?: boolean;
}) {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);
  const color = agentId ? getEntityColor(agentId) : null;

  return (
    <div
      className="rounded-lg border bg-card text-xs"
      style={color ? { borderColor: color.avatarFrom + "40" } : undefined}
    >
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left"
      >
        <BarChart2 className="h-3.5 w-3.5 text-muted-foreground" />
        <span className="font-medium text-sm">
          {chart.title || "Chart"}
        </span>
        {isExpanded ? (
          <ChevronDown className="ml-auto h-3 w-3 text-muted-foreground" />
        ) : (
          <ChevronRight className="ml-auto h-3 w-3 text-muted-foreground" />
        )}
      </button>
      {isExpanded && (
        <div className="border-t px-2 py-3">
          {chart.description && (
            <p className="mb-2 px-1 text-xs text-muted-foreground">{chart.description}</p>
          )}
          {chart.chartType === "bar" && <RenderBarChart chart={chart} />}
          {chart.chartType === "line" && <RenderLineChart chart={chart} />}
          {chart.chartType === "pie" && <RenderPieChart chart={chart} />}
          {chart.chartType === "area" && <RenderAreaChart chart={chart} />}
        </div>
      )}
    </div>
  );
});
