import { test, expect } from "@playwright/test";

test.describe("Admin Observability", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/admin/observability");
    await expect(page.getByRole("heading", { name: "Observability" })).toBeVisible({ timeout: 10000 });
  });

  test("show global view with cards and range controls", async ({ page }) => {
    // Range selector should be visible with 3 options
    await expect(page.getByRole("button", { name: "24h" })).toBeVisible();
    await expect(page.getByRole("button", { name: "7d" })).toBeVisible();
    await expect(page.getByRole("button", { name: "30d" })).toBeVisible();

    // Stats cards should render (even if empty/zero)
    await expect(page.getByText("Total Requests")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Error Rate")).toBeVisible();
    await expect(page.getByText("P95 latency")).toBeVisible();
    await expect(page.getByText("Active users")).toBeVisible();
  });

  test("changing time range updates data", async ({ page }) => {
    // Wait for initial load
    await expect(page.getByText("Total Requests")).toBeVisible({ timeout: 10000 });

    // Switch to 7d
    await page.getByRole("button", { name: "7d" }).click();
    // Should still show stats (might update values)
    await expect(page.getByText("Total Requests")).toBeVisible();

    // Switch to 30d
    await page.getByRole("button", { name: "30d" }).click();
    await expect(page.getByText("Total Requests")).toBeVisible();
  });

  test("show timeline activity section", async ({ page }) => {
    await expect(page.getByText("Total Requests")).toBeVisible({ timeout: 10000 });
    // Timeline section
    await expect(page.getByText("Activity")).toBeVisible();
  });

  test("show top resources table", async ({ page }) => {
    await expect(page.getByText("Total Requests")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Top Resources")).toBeVisible();
  });

  test("navigate to agent detail from top resources", async ({ page }) => {
    await expect(page.getByText("Total Requests")).toBeVisible({ timeout: 10000 });

    // If there are resources in the table, click the first link
    const resourceLink = page.locator("table a").first();
    if (await resourceLink.isVisible({ timeout: 3000 }).catch(() => false)) {
      const href = await resourceLink.getAttribute("href");
      await resourceLink.click();

      // Should navigate to detail page with back button
      await expect(page.locator("a[href='/admin/observability']")).toBeVisible({ timeout: 5000 });

      // Should show stats cards
      await expect(page.getByText("Requests")).toBeVisible();
    }
  });
});

test.describe("Admin Observability - Resource detail", () => {
  test("agent detail shows timeline, errors, and users", async ({ page }) => {
    // First, go to overview and find an agent
    await page.goto("/admin/observability");
    await expect(page.getByRole("heading", { name: "Observability" })).toBeVisible({ timeout: 10000 });

    // Wait for data to load
    await expect(page.getByText("Total Requests")).toBeVisible({ timeout: 10000 });

    // Try to navigate to an agent detail
    const agentLink = page.locator("table a").first();
    if (await agentLink.isVisible({ timeout: 3000 }).catch(() => false)) {
      await agentLink.click();

      // Should have back arrow
      await expect(page.locator("a[href='/admin/observability']")).toBeVisible({ timeout: 5000 });

      // Should have range selector
      await expect(page.getByRole("button", { name: "24h" })).toBeVisible();

      // Wait for stats to load
      await expect(page.getByText("Requests")).toBeVisible({ timeout: 10000 });

      // Should show timeline section
      await expect(page.getByText("Timeline")).toBeVisible();

      // Should show errors and users sections
      await expect(page.getByText("Recent errors")).toBeVisible();
      await expect(page.getByText("Users")).toBeVisible();
    }
  });

  test("return to global view from detail", async ({ page }) => {
    await page.goto("/admin/observability");
    await expect(page.getByText("Total Requests")).toBeVisible({ timeout: 10000 });

    const resourceLink = page.locator("table a").first();
    if (await resourceLink.isVisible({ timeout: 3000 }).catch(() => false)) {
      await resourceLink.click();
      await expect(page.locator("a[href='/admin/observability']")).toBeVisible({ timeout: 5000 });

      // Click back
      await page.locator("a[href='/admin/observability']").click();

      // Should be back on overview
      await expect(page.getByRole("heading", { name: "Observability" })).toBeVisible({ timeout: 5000 });
    }
  });
});

test.describe("Observability - Chat → metrics flow", () => {
  test("send chat and verify metrics update", async ({ page }) => {
    // Step 1: Send a chat message to generate metrics
    await page.goto("/");
    await expect(page.getByRole("navigation").getByText("Agents", { exact: true }).first()).toBeVisible({ timeout: 15000 });

    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible({ timeout: 5000 });

    await textarea.fill("Observability test");
    await page.getByRole("button", { name: "Send message" }).click();

    // Wait for response to complete
    await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
      timeout: 15000,
    });

    // Brief wait for async chat_event insert
    await page.waitForTimeout(2000);

    // Step 2: Go to observability and check metrics
    await page.goto("/admin/observability");
    await expect(page.getByRole("heading", { name: "Observability" })).toBeVisible({ timeout: 10000 });

    // Stats should show at least 1 request
    await expect(page.getByText("Total Requests")).toBeVisible({ timeout: 10000 });

    // The total requests value should be > 0 (the card after "Total Requests" label)
    const totalRequestsCard = page.locator("text=Total Requests").locator("..").locator("..");
    const requestValue = totalRequestsCard.locator(".text-2xl");
    await expect(requestValue).not.toHaveText("0");
  });
});

test.describe("Web Metrics Endpoint", () => {
  test("GET /api/web-metrics devuelve formato Prometheus", async ({ request }) => {
    const response = await request.get("/api/web-metrics");
    expect(response.status()).toBe(200);

    const contentType = response.headers()["content-type"];
    expect(contentType).toContain("text/plain");

    const body = await response.text();
    // Should contain our custom metric names
    expect(body).toContain("agentgram_web");
  });

  test("POST /api/web-metrics accepts metrics", async ({ request }) => {
    const response = await request.post("/api/web-metrics", {
      data: {
        name: "ttfb",
        labels: { agent_id: "test-agent" },
        value: 0.5,
      },
    });
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(body.ok).toBe(true);
  });

  test("POST /api/web-metrics rejects unknown metrics", async ({ request }) => {
    const response = await request.post("/api/web-metrics", {
      data: {
        name: "unknown_metric",
        labels: {},
        value: 1,
      },
    });
    expect(response.status()).toBe(400);
  });
});
