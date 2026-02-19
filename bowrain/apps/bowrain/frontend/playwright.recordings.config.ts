import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  testMatch: "recordings.spec.ts",
  timeout: 60000, // Longer timeout for recordings
  globalTimeout: process.env.CI ? 1800000 : 0, // 30 min cap in CI (39 tests with human-speed typing)
  retries: 0, // No retries for recordings - we want consistent videos
  reporter: process.env.CI ? "list" : "html",
  // Store videos in dedicated recordings output directory
  outputDir: "./recordings-output",
  // Run tests sequentially for predictable video capture
  workers: 1,
  use: {
    baseURL: "http://localhost:5173",
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
  webServer: {
    command: "npm run dev -- --port 5173",
    port: 5173,
    timeout: 30000,
    reuseExistingServer: !process.env.CI,
  },
});
