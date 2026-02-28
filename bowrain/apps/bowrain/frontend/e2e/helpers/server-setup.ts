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
 * The Go backend connects to the server via gRPC; the frontend talks to the Go backend
 * through Wails bindings (no mock needed).
 *
 * Prerequisites:
 * - bowrain-server running (via e2e/compose.yaml or manually)
 * - Wails dev mode running (wails3 dev → Go backend + Vite frontend)
 * - BOWRAIN_SERVER_URL env set (defaults to http://localhost:8080)
 */
export async function setupServerApp(page: Page): Promise<SeedResult> {
  // Wait for server to be ready
  await waitForServer();

  // Seed data (reuse across tests if already seeded)
  if (!seedResult) {
    seedResult = await fullSeed();
  }

  // Navigate to the app root — Wails dev mode serves at the configured baseURL
  await page.goto("/");

  // Wait for the app to be ready (dashboard or connection screen)
  await page.waitForTimeout(2000);

  return seedResult;
}

/** Reset seed state (call in afterAll if needed). */
export function resetSeedState(): void {
  seedResult = null;
}
