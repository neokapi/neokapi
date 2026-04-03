import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright configuration for video recordings.
 * Recordings use longer timeouts and video capture for documentation screencasts.
 */
export default defineConfig({
  testDir: "./e2e",
  testMatch: "recordings.spec.ts",
  timeout: 120_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  outputDir: "./recordings-output",
  use: {
    baseURL: process.env.BOWRAIN_SERVER_URL || process.env.BOWRAIN_URL || "http://localhost:8080",
    viewport: { width: 1280, height: 800 },
    video: {
      mode: "on",
      size: { width: 1280, height: 800 },
    },
    screenshot: "off",
    trace: "off",
    ...devices["Desktop Chrome"],
  },
});
