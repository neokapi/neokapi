import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30000,
  globalTimeout: process.env.CI ? 600000 : 0, // 10 min cap in CI (204 tests)
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI
    ? [["list"], ["html", { open: "never" }]]
    : "html",
  use: {
    baseURL: "http://localhost:5173",
    headless: true,
    screenshot: "only-on-failure",
    trace: "on-first-retry",
  },
  webServer: {
    command: "npm run dev -- --port 5173",
    port: 5173,
    timeout: 30000,
    reuseExistingServer: !process.env.CI,
  },
});
