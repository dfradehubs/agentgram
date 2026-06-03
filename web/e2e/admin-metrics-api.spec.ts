import { test, expect } from "@playwright/test";

test.describe("Admin Metrics API", () => {
  const now = new Date().toISOString();
  const dayAgo = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();

  test("GET /api/admin/metrics/overview returns global stats", async ({ request }) => {
    const response = await request.get(`/api/admin/metrics/overview?from=${dayAgo}&to=${now}`);
    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body).toHaveProperty("total_requests");
    expect(body).toHaveProperty("error_rate");
    expect(body).toHaveProperty("unique_users");
    expect(typeof body.total_requests).toBe("number");
  });

  test("GET /api/admin/metrics/overview/timeline returns buckets", async ({ request }) => {
    const response = await request.get(
      `/api/admin/metrics/overview/timeline?from=${dayAgo}&to=${now}&interval=1h`
    );
    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("GET /api/admin/metrics/overview/top returns resource ranking", async ({ request }) => {
    const response = await request.get(`/api/admin/metrics/overview/top?from=${dayAgo}&to=${now}`);
    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
    if (body.length > 0) {
      expect(body[0]).toHaveProperty("resource_type");
      expect(body[0]).toHaveProperty("resource_id");
      expect(body[0]).toHaveProperty("requests");
    }
  });

  test("per-resource endpoints return correct data", async ({ request }) => {
    // First get top resources to find a valid resource
    const topResp = await request.get(`/api/admin/metrics/overview/top?from=${dayAgo}&to=${now}`);
    const top = await topResp.json();

    if (top.length === 0) {
      test.skip(true, "No resources with metrics to test");
      return;
    }

    const resource = top[0];
    const typeMap: Record<string, string> = {
      agent: "agents",
      mcp: "mcp",
    };
    const typePath = typeMap[resource.resource_type] || "agents";
    const base = `/api/admin/metrics/${typePath}/${resource.resource_id}`;

    // Stats
    const statsResp = await request.get(`${base}?from=${dayAgo}&to=${now}`);
    expect(statsResp.status()).toBe(200);
    const stats = await statsResp.json();
    expect(stats).toHaveProperty("total_requests");
    expect(stats.total_requests).toBeGreaterThan(0);

    // Timeline
    const timelineResp = await request.get(`${base}/timeline?from=${dayAgo}&to=${now}&interval=1h`);
    expect(timelineResp.status()).toBe(200);
    const timeline = await timelineResp.json();
    expect(Array.isArray(timeline)).toBe(true);

    // Users
    const usersResp = await request.get(`${base}/users?from=${dayAgo}&to=${now}`);
    expect(usersResp.status()).toBe(200);
    const users = await usersResp.json();
    expect(Array.isArray(users)).toBe(true);

    // Errors
    const errorsResp = await request.get(`${base}/errors?from=${dayAgo}&to=${now}`);
    expect(errorsResp.status()).toBe(200);
    const errors = await errorsResp.json();
    expect(Array.isArray(errors)).toBe(true);
  });

  test("Prometheus /metrics endpoint accessible", async ({ request }) => {
    // The Go API serves Prometheus metrics at /metrics (no auth)
    const baseUrl = process.env.STAGING
      ? "https://agentgram.example.com"
      : "http://localhost:8080";

    const response = await request.get(`${baseUrl}/metrics`);
    expect(response.status()).toBe(200);

    const body = await response.text();
    expect(body).toContain("agentgram_chat_requests_total");
  });
});
