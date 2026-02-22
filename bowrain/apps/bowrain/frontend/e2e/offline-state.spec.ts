import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";

/**
 * Tests for the offline state indicators in the Header and the
 * connection-state-changed event handling in App.tsx.
 */

/**
 * Helper: inject mock backend and start in connected mode (skipping
 * the ServerConnect screen).
 */
async function setupConnected(page: any) {
  await injectMockBackend(page);
  await page.addInitScript(() => {
    (window as any).__skipConnection = true;
  });
  await page.goto("/");
  await page.locator("aside[data-sidebar]").first().waitFor({ state: "visible", timeout: 10000 });
}

test("should show Connected indicator after connecting", async ({ page }) => {
  await setupConnected(page);
  // Header shows a green status dot with title="Connected"
  await expect(page.locator("span[title='Connected']")).toBeVisible({ timeout: 5000 });
});

test("should show Offline status when connection-state-changed event fires", async ({ page }) => {
  await setupConnected(page);
  await expect(page.locator("span[title='Connected']")).toBeVisible({ timeout: 5000 });

  // Simulate the backend going offline
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

  // Should show Offline indicator (amber dot)
  await expect(page.locator("span[title='Offline']")).toBeVisible({ timeout: 5000 });

  // Should show pending changes count
  await expect(page.getByText("3 pending")).toBeVisible({ timeout: 5000 });
});

test("should clear pending changes when reconnected", async ({ page }) => {
  await setupConnected(page);
  await expect(page.locator("span[title='Connected']")).toBeVisible({ timeout: 5000 });

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

  await expect(page.locator("span[title='Offline']")).toBeVisible({ timeout: 5000 });
  await expect(page.getByText("5 pending")).toBeVisible({ timeout: 5000 });

  // Simulate reconnection
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

  // Should show Connected indicator again
  await expect(page.locator("span[title='Connected']")).toBeVisible({ timeout: 5000 });

  // "pending" text should not be visible
  await expect(page.getByText("pending")).not.toBeVisible();
});
