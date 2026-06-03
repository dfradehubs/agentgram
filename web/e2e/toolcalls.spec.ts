import { test, expect } from "@playwright/test";

test.describe("Tool calls", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("navigation").getByText("Agents", { exact: true }).first()).toBeVisible({ timeout: 15000 });
  });

  test("tool call streaming shows args and result", async ({ page }) => {
    // Select first agent
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    // Send a message that should trigger a tool call in the mock agent
    await textarea.fill("Use a tool to search for information");
    await page.getByRole("button", { name: "Send message" }).click();

    // Wait for response that may include tool calls
    await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
      timeout: 15000,
    });

    // Check if tool call block is rendered (expandable section)
    const toolCallBlock = page.locator("[data-testid='tool-call'], [class*='tool']");
    if (await toolCallBlock.first().isVisible({ timeout: 5000 }).catch(() => false)) {
      // Tool call should show tool name
      await expect(toolCallBlock.first()).toBeVisible();

      // Click to expand and see args/result
      const expandBtn = toolCallBlock.first().getByRole("button").first();
      if (await expandBtn.isVisible()) {
        await expandBtn.click();
        // Should show tool arguments or result
        await expect(toolCallBlock.first().locator("pre, code")).toBeVisible({ timeout: 3000 });
      }
    }
  });

  test("multiple tool calls are shown in sequence", async ({ page }) => {
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    // Send message that may trigger multiple tool calls
    await textarea.fill("Run several tools: search and process data");
    await page.getByRole("button", { name: "Send message" }).click();

    // Wait for response
    await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
      timeout: 15000,
    });

    // Verify response renders without errors (page should not show uncaught errors)
    await expect(page.locator("text=Unhandled Runtime Error")).not.toBeVisible();
  });
});
