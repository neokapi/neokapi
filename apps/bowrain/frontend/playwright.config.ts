import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30000,
  globalTimeout: process.env.CI ? 300000 : 0, // 5 min cap in CI
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? "list" : "html",
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
