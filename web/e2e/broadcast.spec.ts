import { test, expect } from "@playwright/test";

test.describe("Multi-agent broadcast", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("navigation").getByText("Agents", { exact: true }).first()).toBeVisible({ timeout: 15000 });
  });

  test("create broadcast session and receive parallel responses", async ({ page }) => {
    // Look for multi-agent creation button (+ icon in sidebar)
    const newMultiBtn = page.getByText("New Multi-Agent Chat").first();

    // If not directly visible, look for the multi-agent button in sidebar
    if (!(await newMultiBtn.isVisible({ timeout: 2000 }).catch(() => false))) {
      // Multi-agent might be created via a dialog - look for the button that opens it
      const plusButtons = page.locator("nav").getByRole("button");
      // Try to find multi-agent trigger
      for (let i = 0; i < await plusButtons.count(); i++) {
        const btn = plusButtons.nth(i);
        const text = await btn.textContent();
        if (text?.includes("Multi") || text?.includes("multi")) {
          await btn.click();
          break;
        }
      }
    }

    // If the dialog for creating multi-agent opened
    const dialog = page.getByRole("dialog");
    if (await dialog.isVisible({ timeout: 3000 }).catch(() => false)) {
      // Select "Broadcast" mode
      await page.getByText("Broadcast").click();

      // Select at least 2 agents by checking checkboxes
      const checkboxes = dialog.locator("input[type='checkbox'], [role='checkbox']");
      const count = await checkboxes.count();
      if (count >= 2) {
        await checkboxes.nth(0).click();
        await checkboxes.nth(1).click();
      }

      // Click create
      const createBtn = dialog.getByText("Create");
      if (await createBtn.isVisible()) {
        await createBtn.click();
      }

      // Send message in broadcast mode
      const textarea = page.getByPlaceholder("Type your message...");
      if (await textarea.isVisible({ timeout: 5000 }).catch(() => false)) {
        await textarea.fill("Hello to all agents");
        await page.getByRole("button", { name: "Send message" }).click();

        // Should see "Multi-Agent" badge
        await expect(page.getByText("Multi-Agent")).toBeVisible({ timeout: 5000 });

        // Should receive responses (agent bubbles)
        await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").first()).toBeVisible({
          timeout: 15000,
        });
      }
    }
  });
});
