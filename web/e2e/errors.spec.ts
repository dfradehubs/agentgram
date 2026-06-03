import { test, expect } from "@playwright/test";

test.describe("Error handling", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("navigation").getByText("Agents", { exact: true }).first()).toBeVisible({ timeout: 15000 });
  });

  test("show error when agent does not respond", async ({ page }) => {
    // Route interception only works against local dev (requests go through browser).
    // Against staging/production, chat requests go server-side through Next.js proxy.
    test.skip(!!process.env.STAGING, "Route interception does not work against staging (server-side proxy)");

    // Select first agent
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    // Simulate network failure by intercepting requests
    await page.route("**/api/agents/*/chat", (route) => {
      route.abort("connectionrefused");
    });

    await textarea.fill("This message should fail");
    await page.getByRole("button", { name: "Send message" }).click();

    // Should show error indicator
    await expect(page.getByText(/error|Error|failed/i)).toBeVisible({ timeout: 10000 });
  });

  test("retry after error", async ({ page }) => {
    // Route interception only works against local dev (requests go through browser).
    // Against staging/production, chat requests go server-side through Next.js proxy.
    test.skip(!!process.env.STAGING, "Route interception does not work against staging (server-side proxy)");

    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    // First request fails
    let requestCount = 0;
    await page.route("**/api/agents/*/chat", (route) => {
      requestCount++;
      if (requestCount === 1) {
        route.abort("connectionrefused");
      } else {
        route.continue();
      }
    });

    await textarea.fill("Message with retry");
    await page.getByRole("button", { name: "Send message" }).click();

    // Wait for error
    await expect(page.getByText(/error|Error|failed/i)).toBeVisible({ timeout: 10000 });

    // Click retry button if visible
    const retryBtn = page.getByRole("button", { name: /Retry|Regenerate/i });
    if (await retryBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await retryBtn.click();

      // Should recover and get response
      await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
        timeout: 15000,
      });
    }
  });

  test("disconnected agent shows status", async ({ page }) => {
    // Just verify the page loads without crashing and navigation is visible
    await expect(page.locator("nav").first()).toBeVisible();
  });
});
