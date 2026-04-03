import { defineConfig, devices } from "@playwright/test";

/**
 * Cloud E2E tests — run against any live Bowrain server (dev, prod, local Docker).
 *
 * Environment variables:
 *   BOWRAIN_URL             — Server base URL (default: http://localhost:8080)
 *   KEYCLOAK_URL            — Keycloak base URL (default: derived from BOWRAIN_URL)
 *   KEYCLOAK_ADMIN_PASSWORD — Keycloak admin password (required for registration tests)
 */
export default defineConfig({
  testDir: "./tests",
  timeout: 120_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: {
    baseURL: process.env.BOWRAIN_URL || "http://localhost:8080",
    screenshot: "only-on-failure",
    trace: "on-first-retry",
    ...devices["Desktop Chrome"],
    viewport: { width: 1280, height: 800 },
  },
});
