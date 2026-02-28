/**
 * Setup for Wails server mode (headless binary built with -tags server).
 *
 * In server mode, the Go binary serves the frontend via HTTP and routes all
 * Wails binding calls through HTTP fetch transport. No native window, no GUI
 * deps, no CGO required.
 *
 * Usage in recordings/screenshots:
 *   import { setupServerModeApp } from "./helpers/server-mode-setup";
 *   await setupServerModeApp(page);
 */
import type { Page } from "@playwright/test";
import { callBackend } from "./backend-call";

/**
 * Navigates to the headless binary's HTTP server and waits for the app to load.
 * The Go backend handles all binding calls via HTTP — no mock needed.
 *
 * Cleans up any leftover projects from previous tests so each test starts fresh.
 *
 * Prerequisites:
 * - bowrain-headless built with `go build -tags server`
 * - Running on WAILS_SERVER_PORT (default 8090 in test configs)
 */
export async function setupServerModeApp(page: Page): Promise<void> {
  // Skip the connection screen — the headless binary runs in local mode
  await page.addInitScript(() => {
    (window as any).__skipConnection = true;
  });

  await page.goto("/");

  // Wait for the main sidebar to appear, indicating the app is ready
  await page.locator("aside[data-sidebar]").first().waitFor({ state: "visible", timeout: 30000 });

  // Clean up projects from previous tests so each test starts with a fresh state
  try {
    const projects = await callBackend(page, "ListProjects") as { id: string }[];
    for (const p of projects ?? []) {
      await callBackend(page, "CloseProject", p.id);
    }
  } catch {
    // Ignore cleanup errors — first test may have no projects
  }
}
