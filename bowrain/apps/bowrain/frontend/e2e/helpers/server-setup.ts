/**
 * Server-backed setup for Desktop recordings and screenshots.
 * Replaces setupLocalApp() with real bowrain-server data.
 *
 * Usage in recordings/screenshots:
 *   import { setupServerApp } from "./helpers/server-setup";
 *   await setupServerApp(page);
 */
import type { Page } from "@playwright/test";
import { fullSeed, waitForServer, type SeedResult } from "./api-client";

let seedResult: SeedResult | null = null;

/**
 * Seeds the real bowrain-server and navigates the Wails desktop app to the dashboard.
 * The Go backend auto-connects via BOWRAIN_TOKEN. After seeding, we tell the
 * frontend to select the seeded workspace by calling the SelectWorkspace binding.
 */
export async function setupServerApp(page: Page): Promise<SeedResult> {
  // Wait for server to be ready
  await waitForServer();

  // Seed data (reuse across tests if already seeded)
  if (!seedResult) {
    seedResult = await fullSeed();
  }

  // Navigate to the app root — headless binary auto-connects via BOWRAIN_TOKEN
  await page.goto("/");

  // Wait for the app to be ready (connected state, showing workspace selector or empty dashboard)
  await page.waitForTimeout(3000);

  // Tell the desktop app to select the seeded workspace via Wails binding.
  // The binding is called through the __wailsMockByName map in mock mode,
  // or via HTTP transport in server mode.
  const wsSlug = seedResult.context.workspaceSlug;
  await page.evaluate(async (slug) => {
    // In server mode, Wails bindings are called via Call.ByID.
    // SelectWorkspace is available after auto-connect.
    try {
      const _runtime = await import("@wailsio/runtime");
      // The SelectWorkspace binding ID must match the generated bindings.
      // Call it by name through the app module.
      const Backend =
        await import("../../bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js");
      await Backend.SelectWorkspace(slug);
    } catch {
      // Fallback: try mock backend
      const byName = (window as any).__wailsMockByName;
      if (byName?.SelectWorkspace) {
        byName.SelectWorkspace(slug);
      }
    }
  }, wsSlug);

  // Reload to pick up the workspace selection
  await page.reload();

  // Wait for the app to show the dashboard with seeded projects
  await page
    .getByText("Get started with your first project")
    .or(page.getByTestId("nav-translate"))
    .first()
    .waitFor({ state: "visible", timeout: 30000 });

  return seedResult;
}

/** Reset seed state (call in afterAll if needed). */
export function resetSeedState(): void {
  seedResult = null;
}
