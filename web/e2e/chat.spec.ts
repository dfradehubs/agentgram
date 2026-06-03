import { test, expect } from "@playwright/test";

test.describe("Basic chat", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    // Wait for app to load (sidebar agents visible)
    await expect(page.getByRole("navigation").getByText("Agents", { exact: true }).first()).toBeVisible({ timeout: 15000 });
  });

  test("send message and receive streaming response", async ({ page }) => {
    // Select first agent from sidebar
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    // Wait for chat area to show (empty state or input)
    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible({ timeout: 5000 });

    // Type and send message
    await textarea.fill("Hi, how are you?");
    await page.getByRole("button", { name: "Send message" }).click();

    // Wait for assistant response (streaming)
    await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
      timeout: 15000,
    });

    // Verify user message appears in chat area
    await expect(page.getByRole("main").getByText("Hi, how are you?")).toBeVisible();

    // Verify assistant response has content
    const assistantMessages = page.locator("div[class*='rounded-xl'][class*='p-4']");
    await expect(assistantMessages.last()).not.toBeEmpty();
  });

  test("input clears after sending", async ({ page }) => {
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    await textarea.fill("Test message");
    await page.getByRole("button", { name: "Send message" }).click();

    // Input should be cleared after sending
    await expect(textarea).toHaveValue("");
  });

  test("copy conversation", async ({ page }) => {
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    await textarea.fill("Test to copy");
    await page.getByRole("button", { name: "Send message" }).click();

    // Wait for response
    await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
      timeout: 15000,
    });

    // Click copy conversation button
    const copyBtn = page.getByRole("button", { name: "Copy conversation" });
    await expect(copyBtn).toBeVisible();
    await copyBtn.click();

    // Checkmark should appear briefly
    await expect(page.locator(".text-emerald-500")).toBeVisible();
  });
});
