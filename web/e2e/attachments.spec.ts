import { test, expect } from "@playwright/test";
import path from "path";
import fs from "fs";

test.describe("File attachments", () => {
  // Create a small test image before each test to avoid parallel worker issues
  const testImagePath = path.join(__dirname, "fixtures", "test-image.png");

  // Minimal 1x1 PNG (68 bytes)
  const pngBuffer = Buffer.from([
    0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
    0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
    0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1
    0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, // 8-bit RGB
    0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, // IDAT chunk
    0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00, 0x00, // compressed data
    0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc, 0x33, // checksum
    0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, // IEND chunk
    0xae, 0x42, 0x60, 0x82, // IEND CRC
  ]);

  test.beforeEach(async ({ page }) => {
    // Ensure fixture file exists before each test (safe for parallel workers)
    const fixturesDir = path.join(__dirname, "fixtures");
    if (!fs.existsSync(fixturesDir)) {
      fs.mkdirSync(fixturesDir, { recursive: true });
    }
    fs.writeFileSync(testImagePath, pngBuffer);

    await page.goto("/");
    await expect(page.getByRole("navigation").getByText("Agents", { exact: true }).first()).toBeVisible({ timeout: 15000 });
  });

  test.afterAll(async () => {
    if (fs.existsSync(testImagePath)) {
      fs.unlinkSync(testImagePath);
    }
    const fixturesDir = path.join(__dirname, "fixtures");
    if (fs.existsSync(fixturesDir)) {
      try { fs.rmdirSync(fixturesDir); } catch { /* ignore if not empty */ }
    }
  });

  test("upload image and send with message", async ({ page }) => {
    // Select first agent
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    // Click attach file button
    const attachBtn = page.getByRole("button", { name: "Attach file" });
    if (await attachBtn.isVisible()) {
      // Set up file chooser listener
      const [fileChooser] = await Promise.all([
        page.waitForEvent("filechooser"),
        attachBtn.click(),
      ]);
      await fileChooser.setFiles(testImagePath);

      // Attachment preview should appear
      await expect(page.getByText("test-image.png")).toBeVisible({ timeout: 3000 });

      // Type message and send
      await textarea.fill("Look at this image");
      await page.getByRole("button", { name: "Send message" }).click();

      // Wait for assistant response
      await expect(page.locator("div[class*='rounded-xl'][class*='p-4']").last()).toBeVisible({
        timeout: 15000,
      });
    }
  });

  test("remove attachment before sending", async ({ page }) => {
    const agentItem = page.locator("nav").getByRole("button").first();
    await agentItem.click();

    const textarea = page.getByPlaceholder("Type your message...");
    await expect(textarea).toBeVisible();

    const attachBtn = page.getByRole("button", { name: "Attach file" });
    if (await attachBtn.isVisible()) {
      const [fileChooser] = await Promise.all([
        page.waitForEvent("filechooser"),
        attachBtn.click(),
      ]);
      await fileChooser.setFiles(testImagePath);

      // Should see attachment preview
      await expect(page.getByText("test-image.png")).toBeVisible({ timeout: 3000 });

      // Remove the attachment (X button)
      const removeBtn = page.locator("button").filter({ has: page.locator("svg.lucide-x") }).first();
      if (await removeBtn.isVisible()) {
        await removeBtn.click();
        // Attachment should be gone
        await expect(page.getByText("test-image.png")).not.toBeVisible();
      }
    }
  });
});
