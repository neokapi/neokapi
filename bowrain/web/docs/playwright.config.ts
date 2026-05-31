import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright config for walkthrough scene recording.
 *
 * Each scene under bowrain/web/docs/scenes/{walkthrough}/01-*.spec.ts
 * runs against BOWRAIN_BACKEND_URL (default https://dev.bowrain.cloud)
 * and records a .webm to bowrain/web/docs/scenes/{walkthrough}/.
 *
 * Run from this dir:
 *   BOWRAIN_TOKEN=$(...) BOWRAIN_BACKEND_URL=... vpx playwright test
 *
 * Or scope to one scene:
 *   vpx playwright test scenes/bowrain-web-claim-project/
 */

const BACKEND_URL = process.env.BOWRAIN_BACKEND_URL || "https://dev.bowrain.cloud";

export default defineConfig({
  testDir: "./scenes",
  testMatch: "01-*.spec.ts",
  timeout: 120_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  outputDir: "./scenes/.test-results",
  use: {
    baseURL: BACKEND_URL,
    screenshot: "off",
    trace: "on-first-retry",
    video: {
      mode: "on",
      size: { width: 1280, height: 800 },
    },
    viewport: { width: 1280, height: 800 },
    ...devices["Desktop Chrome"],
  },
});
