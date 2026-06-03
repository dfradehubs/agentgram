import { test, expect } from "@playwright/test";

test.describe("Admin LLM", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/admin/llm");
    // Wait for admin page to load
    await expect(page.getByRole("heading", { name: "LLM Models" })).toBeVisible({ timeout: 10000 });
  });

  test("show LLM models table", async ({ page }) => {
    // Table headers should be visible
    await expect(page.getByRole("columnheader", { name: "ID" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Provider" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Model" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Role" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Status" })).toBeVisible();
  });

  test("create new LLM model", async ({ page }) => {
    // Click "New model"
    await page.getByRole("button", { name: /New model/ }).click();

    // Should see form
    await expect(page.getByText("New LLM model")).toBeVisible();

    // Fill form fields
    const nameInput = page.getByLabel(/name/i).first();
    if (await nameInput.isVisible()) {
      await nameInput.fill("Test Model E2E");
    }

    // Select provider
    const providerSelect = page.getByLabel(/provider/i).first();
    if (await providerSelect.isVisible()) {
      await providerSelect.selectOption("google");
    }

    // Fill model name (use type=text to avoid matching checkboxes)
    const modelInput = page.locator("input[type='text'][name='model'], input[placeholder*='model' i]").first();
    if (await modelInput.isVisible()) {
      await modelInput.fill("gemini-2.0-flash");
    }
  });

  test("navigate to edit an existing model", async ({ page }) => {
    // Wait for table to have rows
    const rows = page.locator("tbody tr");
    if (await rows.count() > 0) {
      // Click edit button on first model
      const editBtn = rows.first().getByRole("button").first();
      await editBtn.click();

      // Should show edit form
      await expect(page.getByText(/Edit:/)).toBeVisible({ timeout: 5000 });

      // Back button should be visible
      await expect(page.getByText("Back to LLM models")).toBeVisible();
    }
  });

  test("return to list from form", async ({ page }) => {
    await page.getByRole("button", { name: /New model/ }).click();
    await expect(page.getByText("New LLM model")).toBeVisible();

    // Click back
    await page.getByText("Back to LLM models").click();

    // Should be back on list
    await expect(page.getByText("LLM Models").first()).toBeVisible();
  });
});
