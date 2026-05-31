import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright configuration for keycloak-theme E2E tests.
 *
 * Tests run against the keycloakify dev server which renders pages
 * with mock kcContext based on the ?kcPageId= query parameter.
 *
 * Usage:
 *   cd platform/apps/keycloak-theme
 *   vp run dev &                         # Start Vite dev server
 *   vpx playwright test --config e2e/playwright.config.ts
 */
export default defineConfig({
  testDir: ".",
  testMatch: "pages.spec.ts",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: true,
  workers: 4,
  retries: 1,
  reporter: [["list"]],
  use: {
    baseURL: process.env.KC_THEME_URL || "http://localhost:3000",
    screenshot: "off",
    trace: "on-first-retry",
    ...devices["Desktop Chrome"],
  },
});
