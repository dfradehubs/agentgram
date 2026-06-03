export interface MetricReport {
  name: string;
  labels: Record<string, string>;
  value: number;
}

export function reportMetric(metric: MetricReport): void {
  try {
    fetch("/api/web-metrics", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(metric),
      keepalive: true,
    }).catch(() => {
      // fire and forget
    });
  } catch {
    // ignore
  }
}
