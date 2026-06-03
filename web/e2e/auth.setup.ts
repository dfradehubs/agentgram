import { test as setup, expect } from "@playwright/test";

const authFile = "e2e/.auth/user.json";

setup("authenticate", async ({ page }) => {
  const isStaging = !!process.env.STAGING;

  if (!isStaging) {
    // Local dev: no auth needed, create empty storage state
    await page.goto("/");
    await page.context().storageState({ path: authFile });
    return;
  }

  // Staging: login via Keycloak OIDC
  const email = process.env.KC_USER;
  const password = process.env.KC_PASS;

  if (!email || !password) {
    throw new Error("KC_USER and KC_PASS env vars required for staging auth");
  }

  // Navigate to the app — it will redirect to Keycloak login
  await page.goto("/");

  // Wait for Keycloak login page
  await expect(page.locator("#username, #kc-form-login input[name='username']")).toBeVisible({
    timeout: 15000,
  });

  // Fill credentials
  await page.fill("#username, input[name='username']", email);
  await page.fill("#password, input[name='password']", password);

  // Submit
  await page.click("#kc-login, input[type='submit']");

  // Wait for redirect back to the app (sidebar loads agent list)
  await expect(page.getByRole("navigation").getByText("Agents", { exact: true }).first()).toBeVisible({ timeout: 20000 });

  // Save session state
  await page.context().storageState({ path: authFile });
});
