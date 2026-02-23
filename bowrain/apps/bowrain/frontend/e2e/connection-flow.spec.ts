import { test, expect } from "@playwright/test";
import { injectMockBackend, skipConnectionScreen } from "./mock-backend";

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

test("should start PKCE auth flow when signing in", async ({ page }) => {
  // Click Sign In directly — uses default server URL from mock.
  await page.getByRole("button", { name: /sign in/i }).click();

  // The mock's ConnectToServer throws "not authenticated", so it
  // falls through to StartLogin (PKCE) then WaitForLogin.
  // WaitForLogin returns true immediately in mock, so we should see
  // the workspace selector next.
  await expect(page.getByText("Acme Corp")).toBeVisible({ timeout: 10000 });
  await expect(page.getByText("Personal", { exact: true })).toBeVisible();
});

test("should select workspace and enter main app", async ({ page }) => {
  await page.getByRole("button", { name: /sign in/i }).click();

  // Wait for workspace selector.
  await expect(page.getByText("Acme Corp")).toBeVisible({ timeout: 10000 });

  // Select a workspace.
  await page.getByText("Acme Corp").click();

  // Should be in the main app now (server mode).
  await expect(page.getByTestId("empty-projects")).toBeVisible({ timeout: 5000 });
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
  // Should be in connected mode with sidebar visible.
  await newPage.locator("aside[data-sidebar]").first().waitFor({ state: "visible", timeout: 10000 });
  await newPage.close();
});
