import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";

/**
 * Tests for the offline state indicators in the TopBar and the
 * connection-state-changed event handling in App.tsx.
 *
 * The offline state shows a WifiOff icon with a "N pending" text
 * in the TopBar when the connection is offline with pending changes.
 */

/**
 * Helper: inject mock backend and start in connected server mode
 * (skipping the ServerConnect screen).
 */
async function setupConnected(page: any) {
  await injectMockBackend(page);
  await page.addInitScript(() => {
    (window as any).__skipConnection = "server";
  });
  await page.goto("/");
  await page
    .getByText("Get started with your first project")
    .or(page.getByTestId("nav-translate"))
    .first()
    .waitFor({ state: "visible", timeout: 10000 });
}

test("should show Offline status when connection-state-changed event fires", async ({ page }) => {
  await setupConnected(page);

  // Simulate the backend going offline with pending changes
  await page.evaluate(() => {
    const ids = (window as any).__wailsIDs;
    const mock = (window as any).__wailsMock;

    mock[ids.GetConnectionState] = () => ({
      state: "offline",
      server_url: "http://mock-server",
      user_name: "Test User",
      workspace: "personal",
    });
    mock[ids.GetPendingChangesCount] = () => 3;

    const listeners = (window as any).__wailsEventListeners?.["connection-state-changed"];
    if (listeners) {
      for (const fn of listeners) {
        fn({ data: { state: "offline" } });
      }
    }
  });

  // Should show pending changes count in the TopBar
  await expect(page.getByText("3 pending")).toBeVisible({ timeout: 5000 });
});

test("should clear pending changes when reconnected", async ({ page }) => {
  await setupConnected(page);

  // Go offline with pending changes
  await page.evaluate(() => {
    const ids = (window as any).__wailsIDs;
    const mock = (window as any).__wailsMock;
    mock[ids.GetConnectionState] = () => ({
      state: "offline",
      server_url: "http://mock-server",
      user_name: "Test User",
      workspace: "personal",
    });
    mock[ids.GetPendingChangesCount] = () => 5;

    const listeners = (window as any).__wailsEventListeners?.["connection-state-changed"];
    if (listeners) {
      for (const fn of listeners) {
        fn({ data: { state: "offline" } });
      }
    }
  });

  await expect(page.getByText("5 pending")).toBeVisible({ timeout: 5000 });

  // Reconnect
  await page.evaluate(() => {
    const ids = (window as any).__wailsIDs;
    const mock = (window as any).__wailsMock;
    mock[ids.GetConnectionState] = () => ({
      state: "connected",
      server_url: "http://mock-server",
      user_name: "Test User",
      workspace: "personal",
    });

    const listeners = (window as any).__wailsEventListeners?.["connection-state-changed"];
    if (listeners) {
      for (const fn of listeners) {
        fn({ data: { state: "connected" } });
      }
    }
  });

  // Pending changes text should disappear
  await expect(page.getByText("pending")).not.toBeVisible({ timeout: 5000 });
});
