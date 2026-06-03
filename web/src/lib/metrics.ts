import { Registry, Counter, Histogram } from "prom-client";

const register = new Registry();

register.setDefaultLabels({ app: "agentgram-web" });

export const sseErrors = new Counter({
  name: "agentgram_web_sse_errors_total",
  help: "Total SSE stream errors",
  labelNames: ["agent_id", "error_type"] as const,
  registers: [register],
});

export const ttfb = new Histogram({
  name: "agentgram_web_ttfb_seconds",
  help: "Time to first byte for SSE streams",
  labelNames: ["agent_id"] as const,
  buckets: [0.1, 0.25, 0.5, 1, 2, 5, 10],
  registers: [register],
});

export const backgroundTransfers = new Counter({
  name: "agentgram_web_background_stream_transfers_total",
  help: "Total background stream transfers",
  labelNames: ["agent_id"] as const,
  registers: [register],
});

export const pdfExports = new Counter({
  name: "agentgram_web_pdf_exports_total",
  help: "Total PDF exports",
  labelNames: ["agent_id"] as const,
  registers: [register],
});

export { register };
