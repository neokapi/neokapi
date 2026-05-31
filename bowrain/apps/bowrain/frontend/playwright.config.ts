import { defineConfig } from "@playwright/test";

const useServerMode = !!process.env.WAILS_SERVER_MODE;

export default defineConfig({
  testDir: "./e2e",
  testIgnore: ["recordings.spec.ts"],
  timeout: 30000,
  globalTimeout: process.env.CI ? 600000 : 0, // 10 min cap in CI (204 tests)
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "html",
  use: {
    baseURL: useServerMode ? "http://localhost:8090" : "http://localhost:5173",
    headless: true,
    screenshot: "only-on-failure",
    trace: "on-first-retry",
  },
  webServer: useServerMode
    ? {
        command: "cd ../../../.. && WAILS_SERVER_PORT=8090 bin/bowrain-headless",
        port: 8090,
        timeout: 30000,
        reuseExistingServer: !process.env.CI,
      }
    : {
        command: "vp run dev -- --port 5173",
        port: 5173,
        timeout: 30000,
        reuseExistingServer: !process.env.CI,
      },
});
