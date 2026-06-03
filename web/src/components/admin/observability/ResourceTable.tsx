"use client";

import Link from "next/link";

interface ResourceRow {
  resource_type: string;
  resource_id: string;
  resource_name: string;
  requests: number;
  error_rate: number;
  avg_duration_ms: number;
}

interface ResourceTableProps {
  title: string;
  resources: ResourceRow[];
  basePath: string;
}

function resourceTypeLabel(type: string): string {
  switch (type) {
    case "agent": return "Agente";
    case "custom_agent": return "Custom Agent";
    case "mcp": return "MCP Server";
    default: return type;
  }
}

function detailHref(basePath: string, row: ResourceRow): string {
  return `${basePath}?r=${row.resource_type}:${row.resource_id}`;
}

export function ResourceTable({ title, resources, basePath }: ResourceTableProps) {
  if (!resources || resources.length === 0) {
    return (
      <div className="rounded-lg border p-4">
        <h3 className="mb-2 font-semibold">{title}</h3>
        <p className="text-sm text-muted-foreground">No data</p>
      </div>
    );
  }

  return (
    <div className="rounded-lg border">
      <h3 className="border-b px-4 py-3 font-semibold">{title}</h3>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b text-left text-muted-foreground">
              <th className="px-4 py-2 font-medium">Name</th>
              <th className="px-4 py-2 font-medium">Type</th>
              <th className="px-4 py-2 font-medium text-right">Requests</th>
              <th className="px-4 py-2 font-medium text-right">Error Rate</th>
              <th className="px-4 py-2 font-medium text-right">Latency</th>
            </tr>
          </thead>
          <tbody>
            {resources.map((row) => (
              <tr key={`${row.resource_type}-${row.resource_id}`} className="border-b last:border-0 hover:bg-muted/50">
                <td className="px-4 py-2">
                  <Link href={detailHref(basePath, row)} className="font-medium text-primary hover:underline">
                    {row.resource_name || row.resource_id}
                  </Link>
                </td>
                <td className="px-4 py-2 text-muted-foreground">{resourceTypeLabel(row.resource_type)}</td>
                <td className="px-4 py-2 text-right font-mono">{row.requests.toLocaleString()}</td>
                <td className="px-4 py-2 text-right">
                  <span className={row.error_rate > 5 ? "text-red-500 font-medium" : ""}>
                    {row.error_rate.toFixed(1)}%
                  </span>
                </td>
                <td className="px-4 py-2 text-right font-mono">{Math.round(row.avg_duration_ms)}ms</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
