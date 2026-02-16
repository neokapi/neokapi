import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright configuration for web app E2E tests.
 * Tests run against the real Docker backend at localhost:8080.
 */
export default defineConfig({
  testDir: "./e2e",
  testMatch: "screenshots.spec.ts",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: {
    baseURL: process.env.BOWRAIN_URL || "http://localhost:8080",
    screenshot: "off",
    trace: "off",
    ...devices["Desktop Chrome"],
  },
});
