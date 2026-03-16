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

/**
 * Navigates to the headless binary's HTTP server and waits for the app to load.
 * The Go backend handles all binding calls via HTTP — no mock needed.
 *
 * Prerequisites:
 * - bowrain-headless built with `go build -tags server`
 * - Running on WAILS_SERVER_PORT (default 8090 in test configs)
 */
export async function setupServerModeApp(page: Page): Promise<void> {
  // The headless binary runs in local mode — no server connection needed.
  // Set __skipConnection so App.tsx's connection flow enters ready mode.
  await page.addInitScript(() => {
    (window as any).__skipConnection = true;
  });

  await page.goto("/");

  // Wait for the app to reach ready mode (dashboard or nav visible).
  await page
    .getByText("Get started with your first project")
    .or(page.getByTestId("nav-translate"))
    .first()
    .waitFor({ state: "visible", timeout: 30000 });
}
