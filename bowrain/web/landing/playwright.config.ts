import { defineConfig, devices } from "@playwright/test";

/**
 * Smoke suite for the landing page. It runs against a dev server this config
 * starts, so it needs no backend: the two things it guards — the narrative
 * order and the signup link rewrite — are both client-side.
 *
 * `reuseExistingServer: false` on purpose. Two checkouts run at once on this
 * machine, and adopting a stranger's server on 5173 has already produced one
 * confident, wrong verdict elsewhere in this repo; a busy port must be a loud
 * failure instead.
 */
const PORT = Number(process.env.LANDING_E2E_PORT ?? 4319);

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"]] : [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: `http://localhost:${PORT}`,
    trace: "retain-on-failure",
    ...devices["Desktop Chrome"],
  },
  webServer: {
    command: `vp run dev -- --port ${PORT} --strictPort`,
    port: PORT,
    timeout: 60_000,
    reuseExistingServer: false,
  },
});
