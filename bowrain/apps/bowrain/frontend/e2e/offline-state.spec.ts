import { test, expect } from "@playwright/test";
import { injectMockBackend, skipConnectionScreen } from "./mock-backend";

/**
 * Tests for the offline state indicators in the Header and the
 * connection-state-changed event handling in App.tsx.
 */

test.beforeEach(async ({ page }) => {
  await injectMockBackend(page);
  await page.goto("/");
});

test("should show Local status when working offline", async ({ page }) => {
  await skipConnectionScreen(page);
  await expect(page.getByText("Local", { exact: true })).toBeVisible({ timeout: 5000 });
  // Should NOT show disconnect button in local mode.
  await expect(page.locator("button").filter({ has: page.locator("svg.lucide-log-out") })).not.toBeVisible();
});

test("should show Connected status after connecting", async ({ page }) => {
  // Go through the full connection flow.
  const urlInput = page.getByPlaceholder(/bowrain/i);
  await urlInput.fill("http://localhost:8080");
  await page.getByRole("button", { name: /connect/i }).click();
  await expect(page.getByText("ABCD-1234")).toBeVisible({ timeout: 5000 });
  await page.getByRole("button", { name: /open in browser/i }).click();
  await expect(page.getByText("Acme Corp")).toBeVisible({ timeout: 10000 });
  await page.getByText("Acme Corp").click();

  // Should show Connected and user name.
  await expect(page.getByText("Connected")).toBeVisible({ timeout: 5000 });
  await expect(page.getByText("Test User")).toBeVisible();
});

test("should show Offline status when connection-state-changed event fires", async ({ page }) => {
  // Connect first.
  const urlInput = page.getByPlaceholder(/bowrain/i);
  await urlInput.fill("http://localhost:8080");
  await page.getByRole("button", { name: /connect/i }).click();
  await expect(page.getByText("ABCD-1234")).toBeVisible({ timeout: 5000 });
  await page.getByRole("button", { name: /open in browser/i }).click();
  await expect(page.getByText("Acme Corp")).toBeVisible({ timeout: 10000 });
  await page.getByText("Acme Corp").click();
  await expect(page.getByText("Connected")).toBeVisible({ timeout: 5000 });

  // Simulate the backend going offline by:
  // 1. Updating the mock GetConnectionState to return "offline"
  // 2. Emitting the connection-state-changed event
  await page.evaluate(() => {
    const ids = (window as any).__wailsIDs;
    const mock = (window as any).__wailsMock;

    // Override GetConnectionState to return offline.
    mock[ids.GetConnectionState] = () => ({
      state: "offline",
      server_url: "http://localhost:8080",
      user_name: "Test User",
      workspace: "acme-corp",
    });

    // Override GetPendingChangesCount to return 3.
    mock[ids.GetPendingChangesCount] = () => 3;

    // Emit the connection-state-changed event.
    const listeners = (window as any).__wailsEventListeners?.["connection-state-changed"];
    if (listeners) {
      for (const fn of listeners) {
        fn({ data: { state: "offline" } });
      }
    }
  });

  // Should show Offline status.
  await expect(page.getByText("Offline")).toBeVisible({ timeout: 5000 });

  // Should show pending changes count.
  await expect(page.getByText("3 pending")).toBeVisible({ timeout: 5000 });
});

test("should show disconnect button when offline", async ({ page }) => {
  // Connect first.
  const urlInput = page.getByPlaceholder(/bowrain/i);
  await urlInput.fill("http://localhost:8080");
  await page.getByRole("button", { name: /connect/i }).click();
  await expect(page.getByText("ABCD-1234")).toBeVisible({ timeout: 5000 });
  await page.getByRole("button", { name: /open in browser/i }).click();
  await expect(page.getByText("Acme Corp")).toBeVisible({ timeout: 10000 });
  await page.getByText("Acme Corp").click();
  await expect(page.getByText("Connected")).toBeVisible({ timeout: 5000 });

  // Go offline.
  await page.evaluate(() => {
    const ids = (window as any).__wailsIDs;
    const mock = (window as any).__wailsMock;
    mock[ids.GetConnectionState] = () => ({
      state: "offline",
      server_url: "http://localhost:8080",
      user_name: "Test User",
      workspace: "acme-corp",
    });
    const listeners = (window as any).__wailsEventListeners?.["connection-state-changed"];
    if (listeners) {
      for (const fn of listeners) {
        fn({ data: { state: "offline" } });
      }
    }
  });

  await expect(page.getByText("Offline")).toBeVisible({ timeout: 5000 });

  // Disconnect button should still be visible when offline.
  const disconnectBtn = page.locator("button").filter({ has: page.locator("svg.lucide-log-out") });
  await expect(disconnectBtn).toBeVisible();
});

test("should clear pending changes when reconnected", async ({ page }) => {
  // Connect first.
  const urlInput = page.getByPlaceholder(/bowrain/i);
  await urlInput.fill("http://localhost:8080");
  await page.getByRole("button", { name: /connect/i }).click();
  await expect(page.getByText("ABCD-1234")).toBeVisible({ timeout: 5000 });
  await page.getByRole("button", { name: /open in browser/i }).click();
  await expect(page.getByText("Acme Corp")).toBeVisible({ timeout: 10000 });
  await page.getByText("Acme Corp").click();
  await expect(page.getByText("Connected")).toBeVisible({ timeout: 5000 });

  // Go offline with pending changes.
  await page.evaluate(() => {
    const ids = (window as any).__wailsIDs;
    const mock = (window as any).__wailsMock;
    mock[ids.GetConnectionState] = () => ({
      state: "offline",
      server_url: "http://localhost:8080",
      user_name: "Test User",
      workspace: "acme-corp",
    });
    mock[ids.GetPendingChangesCount] = () => 5;
    const listeners = (window as any).__wailsEventListeners?.["connection-state-changed"];
    if (listeners) {
      for (const fn of listeners) {
        fn({ data: { state: "offline" } });
      }
    }
  });

  await expect(page.getByText("Offline")).toBeVisible({ timeout: 5000 });
  await expect(page.getByText("5 pending")).toBeVisible({ timeout: 5000 });

  // Simulate reconnection.
  await page.evaluate(() => {
    const ids = (window as any).__wailsIDs;
    const mock = (window as any).__wailsMock;
    mock[ids.GetConnectionState] = () => ({
      state: "connected",
      server_url: "http://localhost:8080",
      user_name: "Test User",
      workspace: "acme-corp",
    });
    const listeners = (window as any).__wailsEventListeners?.["connection-state-changed"];
    if (listeners) {
      for (const fn of listeners) {
        fn({ data: { state: "connected" } });
      }
    }
  });

  // Should show Connected again.
  await expect(page.getByText("Connected")).toBeVisible({ timeout: 5000 });

  // "pending" text should not be visible.
  await expect(page.getByText("pending")).not.toBeVisible();
});
