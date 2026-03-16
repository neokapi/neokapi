import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";

test.beforeEach(async ({ page }) => {
  await injectMockBackend(page);
  await page.goto("/");
});

test("should show welcome screen on startup", async ({ page }) => {
  // App starts in disconnected mode — shows welcome screen.
  await expect(page.getByText("Welcome to Bowrain")).toBeVisible();
  await expect(page.getByRole("button", { name: /sign in/i })).toBeVisible();
  await expect(page.getByRole("button", { name: /sign in/i })).toBeEnabled();
  // URL input should not be visible by default (collapsed).
  await expect(page.getByPlaceholder(/localhost/i)).not.toBeVisible();
});

test("should start PKCE auth flow and enter main app", async ({ page }) => {
  // Click Sign In directly — uses default server URL from mock.
  await page.getByRole("button", { name: /sign in/i }).click();

  // The mock's ConnectToServer throws "not authenticated", so it
  // falls through to StartLogin (PKCE) then WaitForLogin.
  // WaitForLogin returns true immediately in mock, then ConnectToServer
  // succeeds. The app auto-selects the first workspace and enters ready mode.
  await expect(page.getByText("Get started with your first project")).toBeVisible({
    timeout: 10000,
  });
});

test("should bypass connection screen via skipConnectionScreen helper", async ({ page }) => {
  // Note: skipConnectionScreen must be called before goto, but since
  // beforeEach already did goto, we need a fresh approach. Use a new page.
  const newPage = await page.context().newPage();
  await injectMockBackend(newPage);
  await newPage.addInitScript(() => {
    (window as any).__skipConnection = true;
  });
  await newPage.goto("/");
  // Should be in connected mode — the app renders the main dashboard.
  await expect(newPage.getByText("Get started with your first project")).toBeVisible({
    timeout: 10000,
  });
  await newPage.close();
});
