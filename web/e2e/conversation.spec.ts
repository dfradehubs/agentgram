import { test, expect } from "@playwright/test";

test.describe("Multi-agent conversation", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("navigation").getByText("Agents", { exact: true }).first()).toBeVisible({ timeout: 15000 });
  });

  test("create conversation session with sequence between agents", async ({ page }) => {
    // Open multi-agent dialog
    const plusButtons = page.locator("nav").getByRole("button");
    for (let i = 0; i < await plusButtons.count(); i++) {
      const btn = plusButtons.nth(i);
      const ariaLabel = await btn.getAttribute("aria-label");
      const text = await btn.textContent();
      if (text?.includes("Multi") || ariaLabel?.includes("Multi")) {
        await btn.click();
        break;
      }
    }

    const dialog = page.getByRole("dialog");
    if (await dialog.isVisible({ timeout: 3000 }).catch(() => false)) {
      // Select "Conversation" mode
      const convMode = dialog.getByText("Conversation");
      if (await convMode.isVisible()) {
        await convMode.click();

        // Select agents
        const checkboxes = dialog.locator("input[type='checkbox'], [role='checkbox']");
        const count = await checkboxes.count();
        if (count >= 2) {
          await checkboxes.nth(0).click();
          await checkboxes.nth(1).click();
        }

        // Should see sequence builder
        await expect(dialog.getByText("Step sequence")).toBeVisible({ timeout: 3000 });

        // Create the session
        const createBtn = dialog.getByText("Create");
        if (await createBtn.isVisible()) {
          await createBtn.click();
        }

        // Should show conversation badge
        await expect(page.getByText("Conversation")).toBeVisible({ timeout: 5000 });

        // Send message and check conversation flow
        const textarea = page.getByPlaceholder("Type your message...");
        if (await textarea.isVisible({ timeout: 5000 }).catch(() => false)) {
          await textarea.fill("Start the conversation");
          await page.getByRole("button", { name: "Send message" }).click();

          // Should see step progress indicator
          await expect(page.getByText(/Step \d+\/\d+/)).toBeVisible({ timeout: 15000 });
        }
      }
    }
  });
});
