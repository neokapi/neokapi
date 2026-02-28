import { defineConfig } from "@playwright/test";

const useServerMode = !!process.env.WAILS_SERVER_MODE;

export default defineConfig({
  testDir: "./e2e",
  testMatch: "recordings.spec.ts",
  timeout: 120000, // Longer timeout for human-speed recordings (200+ chars at 100ms/char + UI interactions)
  globalTimeout: process.env.CI ? 1800000 : 0, // 30 min cap in CI (39 tests with human-speed typing)
  retries: 0, // No retries for recordings - we want consistent videos
  reporter: process.env.CI ? "list" : "html",
  // Store videos in dedicated recordings output directory
  outputDir: "./recordings-output",
  // Run tests sequentially for predictable video capture
  workers: 1,
  use: {
    baseURL: useServerMode
      ? "http://localhost:8090"
      : "http://localhost:5173",
    headless: true,
    viewport: { width: 1280, height: 800 },
    // Enable video recording - retain on success
    video: {
      mode: "on",
      size: { width: 1280, height: 800 },
    },
    // Slower actions for better video visualization
    actionTimeout: 15000,
  },
  webServer: process.env.BOWRAIN_SERVER_URL
    ? {
        // Real server mode: Wails dev mode (Go backend + Vite frontend)
        command: "cd ../../../.. && wails3 dev",
        port: 5173,
        timeout: 60000,
        reuseExistingServer: !process.env.CI,
      }
    : useServerMode
      ? {
          // Headless binary mode: server-tagged build serving real Go backend over HTTP
          command: "cd ../../../.. && WAILS_SERVER_PORT=8090 bin/bowrain-headless",
          port: 8090,
          timeout: 30000,
          reuseExistingServer: !process.env.CI,
        }
      : {
          // Mock mode: Vite dev server only (for fast local development)
          command: "npm run dev -- --port 5173",
          port: 5173,
          timeout: 30000,
          reuseExistingServer: !process.env.CI,
        },
});
