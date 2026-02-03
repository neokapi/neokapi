import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  testMatch: "recordings.spec.ts",
  timeout: 60000, // Longer timeout for recordings
  globalTimeout: process.env.CI ? 600000 : 0, // 10 min cap in CI
  retries: 0, // No retries for recordings - we want consistent videos
  reporter: process.env.CI ? "list" : "html",
  // Store videos in dedicated recordings output directory
  outputDir: "./recordings-output",
  // Run tests sequentially for predictable video capture
  workers: 1,
  use: {
    baseURL: "http://localhost:5173",
    headless: true,
    viewport: { width: 1280, height: 838 }, // Extra 38px for window title bar
    // Enable video recording - retain on success
    video: {
      mode: "on",
      size: { width: 1280, height: 838 },
    },
    // Slower actions for better video visualization
    actionTimeout: 10000,
  },
  webServer: {
    command: "npm run dev -- --port 5173",
    port: 5173,
    timeout: 30000,
    reuseExistingServer: !process.env.CI,
  },
});
