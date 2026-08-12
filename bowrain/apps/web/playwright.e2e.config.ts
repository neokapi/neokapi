import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright configuration for functional E2E tests.
 * Tests run against the real Docker backend at localhost:8080.
 */
export default defineConfig({
  testDir: "./e2e",
  testMatch: [
    "brand-scan-onboarding.spec.ts",
    "claim-flow.spec.ts",
    "context-explorer.spec.ts",
    "editor-happy-path.spec.ts",
    "open-in-desktop.spec.ts",
    "routing.spec.ts",
    "layout-scroll.spec.ts",
  ],
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: process.env.BOWRAIN_URL || "http://localhost:8080",
    // On failure, keep the evidence. This suite runs only in CI against the
    // Docker stack, so a failure that leaves nothing behind can only be
    // re-run, never read — which is how two of these specs stayed unexplained
    // from 2026-07-15 to 2026-07-29. Note `retain-on-failure`, not
    // `on-first-retry`: retries is 0 here, so on-first-retry never fires.
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    ...devices["Desktop Chrome"],
  },
});
