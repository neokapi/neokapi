import { test, expect } from "@playwright/test";
import { injectMockBackend, skipConnectionScreen } from "./mock-backend";

test.beforeEach(async ({ page }) => {
  await injectMockBackend(page);
  await page.goto("/");
});

test("should show server connect screen on startup", async ({ page }) => {
  // App starts in connecting mode when disconnected.
  await expect(page.getByText("Connect to Server")).toBeVisible();
  await expect(page.getByPlaceholder(/bowrain/i)).toBeVisible();
  await expect(page.getByText("Work Offline")).toBeVisible();
});

test("should skip to local mode via Work Offline", async ({ page }) => {
  await page.getByText("Work Offline").click();
  // Should see the main app with project dashboard.
  await expect(page.getByTestId("empty-projects")).toBeVisible({ timeout: 5000 });
});

test("should show device auth flow when connecting", async ({ page }) => {
  // Enter a server URL and connect.
  const urlInput = page.getByPlaceholder(/bowrain/i);
  await urlInput.fill("http://localhost:8080");
  await page.getByRole("button", { name: /connect/i }).click();

  // The mock's ConnectToServer throws "not authenticated", so it
  // falls through to StartLogin which returns a device code.
  // Should see the user code for device auth.
  await expect(page.getByText("ABCD-1234")).toBeVisible({ timeout: 5000 });
});

test("should complete login and show workspace selector", async ({ page }) => {
  // Enter server URL and connect — triggers device auth.
  const urlInput = page.getByPlaceholder(/bowrain/i);
  await urlInput.fill("http://localhost:8080");
  await page.getByRole("button", { name: /connect/i }).click();

  // Wait for auth code to appear, then click "Open in Browser" to start polling.
  await expect(page.getByText("ABCD-1234")).toBeVisible({ timeout: 5000 });
  await page.getByRole("button", { name: /open in browser/i }).click();

  // Mock PollLogin returns true immediately.
  // Should transition to workspace selector.
  await expect(page.getByText("Acme Corp")).toBeVisible({ timeout: 10000 });
  await expect(page.getByText("Personal", { exact: true })).toBeVisible();
});

test("should select workspace and enter main app", async ({ page }) => {
  const urlInput = page.getByPlaceholder(/bowrain/i);
  await urlInput.fill("http://localhost:8080");
  await page.getByRole("button", { name: /connect/i }).click();
  await expect(page.getByText("ABCD-1234")).toBeVisible({ timeout: 5000 });
  await page.getByRole("button", { name: /open in browser/i }).click();

  // Wait for workspace selector.
  await expect(page.getByText("Acme Corp")).toBeVisible({ timeout: 10000 });

  // Select a workspace.
  await page.getByText("Acme Corp").click();

  // Should be in the main app now (server mode).
  await expect(page.getByTestId("empty-projects")).toBeVisible({ timeout: 5000 });
});

test("should show Connected status in header when in server mode", async ({ page }) => {
  const urlInput = page.getByPlaceholder(/bowrain/i);
  await urlInput.fill("http://localhost:8080");
  await page.getByRole("button", { name: /connect/i }).click();
  await expect(page.getByText("ABCD-1234")).toBeVisible({ timeout: 5000 });
  await page.getByRole("button", { name: /open in browser/i }).click();
  await expect(page.getByText("Acme Corp")).toBeVisible({ timeout: 10000 });
  await page.getByText("Acme Corp").click();

  // Header should show "Connected" and user name.
  await expect(page.getByText("Connected")).toBeVisible({ timeout: 5000 });
  await expect(page.getByText("Test User")).toBeVisible();
});

test("should show Local status in header when in offline mode", async ({ page }) => {
  await skipConnectionScreen(page);
  await expect(page.getByText("Local", { exact: true })).toBeVisible({ timeout: 5000 });
});

test("should disconnect and return to connect screen", async ({ page }) => {
  // Connect first.
  const urlInput = page.getByPlaceholder(/bowrain/i);
  await urlInput.fill("http://localhost:8080");
  await page.getByRole("button", { name: /connect/i }).click();
  await expect(page.getByText("ABCD-1234")).toBeVisible({ timeout: 5000 });
  await page.getByRole("button", { name: /open in browser/i }).click();
  await expect(page.getByText("Acme Corp")).toBeVisible({ timeout: 10000 });
  await page.getByText("Acme Corp").click();
  await expect(page.getByText("Connected")).toBeVisible({ timeout: 5000 });

  // Click disconnect in header.
  const disconnectBtn = page.locator("button").filter({ has: page.locator("svg.lucide-log-out") });
  await disconnectBtn.click();

  // Should return to server connect screen.
  await expect(page.getByText("Connect to Server")).toBeVisible({ timeout: 5000 });
});
