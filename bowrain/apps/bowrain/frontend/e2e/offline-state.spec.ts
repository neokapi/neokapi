import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";

/**
 * Tests for the offline state indicators in the TopBar and the
 * connection-state-changed event handling in App.tsx.
 *
 * The offline state shows a WifiOff icon with a "N pending" text
 * in the TopBar when the connection is offline with pending changes,
 * beside a Retry button that asks the backend to reconnect now.
 */

/** Push a connection state through the mocked Wails event + binding pair. */
async function setConnectionState(page: any, state: string, pending: number) {
  await page.evaluate(
    ({ state, pending }: { state: string; pending: number }) => {
      const ids = (window as any).__wailsIDs;
      const mock = (window as any).__wailsMock;

      mock[ids.GetConnectionState] = () => ({
        state,
        server_url: "http://mock-server",
        user_name: "Test User",
        workspace: "personal",
      });
      mock[ids.GetPendingChangesCount] = () => pending;

      const listeners = (window as any).__wailsEventListeners?.["connection-state-changed"];
      if (listeners) {
        for (const fn of listeners) {
          fn({ data: { state } });
        }
      }
    },
    { state, pending },
  );
}

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
    .getByTestId("empty-projects")
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

test("offers Retry while offline and calls the backend", async ({ page }) => {
  await setupConnected(page);
  await page.evaluate(() => {
    const ids = (window as any).__wailsIDs;
    const mock = (window as any).__wailsMock;
    (window as any).__retryCalls = 0;
    mock[ids.RetryConnection] = () => {
      (window as any).__retryCalls += 1;
      return { state: "offline", server_url: "http://mock-server", workspace: "personal" };
    };
  });

  await setConnectionState(page, "offline", 2);
  await expect(page.getByTestId("offline-pending")).toBeVisible({ timeout: 5000 });

  await page.getByTestId("connection-retry").click();
  await expect
    .poll(async () => page.evaluate(() => (window as any).__retryCalls))
    .toBeGreaterThan(0);
});

test("reports an attempt in flight while reconnecting", async ({ page }) => {
  await setupConnected(page);

  await setConnectionState(page, "offline", 1);
  await expect(page.getByTestId("connection-offline")).toBeVisible({ timeout: 5000 });

  await setConnectionState(page, "connecting", 1);
  await expect(page.getByTestId("connection-reconnecting")).toBeVisible({ timeout: 5000 });
  await expect(page.getByTestId("connection-offline")).toBeHidden();
});
