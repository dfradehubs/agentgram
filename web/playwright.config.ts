import { defineConfig, devices } from "@playwright/test";

const isStaging = !!process.env.STAGING;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: "html",
  use: {
    baseURL: isStaging
      ? process.env.STAGING_URL || "https://agentgram.example.com"
      : "http://localhost:3000",
    trace: "on-first-retry",
    ...(isStaging && {
      extraHTTPHeaders: { env: "fp-staging" },
    }),
  },
  projects: [
    {
      name: "setup",
      testMatch: /.*\.setup\.ts/,
    },
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        storageState: "e2e/.auth/user.json",
      },
      dependencies: ["setup"],
    },
  ],
  ...(!isStaging && {
    webServer: {
      command: "npm run dev",
      url: "http://localhost:3000",
      reuseExistingServer: !process.env.CI,
    },
  }),
});
