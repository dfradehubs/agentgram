import { test, expect } from "@playwright/test";

test.describe("Sessions", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("navigation").getByText("Agents", { exact: true }).first()).toBeVisible({ timeout: 15000 });
  });

  test("create new session by sending a message", async ({ page }) => {
    // Select first agent
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    // Send first message (creates session)
    await textarea.fill("First session message");
    await page.getByRole("button", { name: "Send message" }).click();

    // Wait for response
    await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
      timeout: 15000,
    });

    // Session should appear in sidebar
    await expect(page.locator("nav").getByText("First session message").first()).toBeVisible({
      timeout: 5000,
    });
  });

  test("navigating between sessions keeps messages", async ({ page }) => {
    // Select first agent
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    // Create first session
    await textarea.fill("Session A message");
    await page.getByRole("button", { name: "Send message" }).click();
    await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
      timeout: 15000,
    });

    // Click "New conversation" to create second session for this agent
    await page.getByText("New conversation").first().click();

    // Wait for new session to be ready (chat component may remount)
    await page.waitForTimeout(1000);
    const sendBtn = page.getByRole("button", { name: "Send message" });
    await expect(sendBtn).toBeVisible({ timeout: 10000 });

    // Send second message
    await textarea.fill("Session B message");
    await sendBtn.click();
    await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
      timeout: 15000,
    });

    // Navigate back to first session
    const firstSession = page.locator("nav").getByText("Session A message").first();
    if (await firstSession.isVisible()) {
      await firstSession.click();
      // First session messages should be visible in the main chat area
      await expect(page.getByRole("main").getByText("Session A message", { exact: true })).toBeVisible({ timeout: 5000 });
    }
  });

  test("delete session", async ({ page }) => {
    // Use unique session name to avoid conflicts with previous test runs
    const uniqueName = `Delete-${Date.now()}`;

    // Select first agent
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    // Create a session
    await textarea.fill(uniqueName);
    await page.getByRole("button", { name: "Send message" }).click();
    await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
      timeout: 15000,
    });

    // Wait for streaming to finish before deleting
    await page.waitForTimeout(2000);

    // Find the session row container and its options button
    const sessionRow = page.locator("nav").locator("div.group").filter({ hasText: uniqueName }).first();
    await sessionRow.hover();

    // Click the options button within this specific session row
    const optionsBtn = sessionRow.getByRole("button", { name: "Session options" });
    await expect(optionsBtn).toBeVisible({ timeout: 3000 });
    await optionsBtn.click();

    // Click delete menu item
    await page.getByRole("menuitem", { name: "Delete" }).click();

    // Session should be removed from sidebar
    await expect(page.locator("nav").getByText(uniqueName)).not.toBeVisible({
      timeout: 10000,
    });
  });
});
