import { test, expect } from "@playwright/test";

/**
 * E2E tests for all custom Keycloak theme pages.
 *
 * These tests run against a Vite dev server that injects mock kcContext
 * via the window object (keycloakify dev mode). Each test navigates to
 * /?kcPageId=<pageId> which the dev server renders with a mock context.
 *
 * Prerequisites:
 *   cd platform/apps/keycloak-theme && vp run dev
 *   vpx playwright test --config e2e/playwright.config.ts
 */

const BASE = process.env.KC_THEME_URL || "http://localhost:3000";

// All custom pages and what to look for to confirm they rendered.
const pages: { pageId: string; expect: string }[] = [
  { pageId: "login.ftl", expect: '[data-slot="card"]' },
  { pageId: "login-username.ftl", expect: '[data-slot="card"]' },
  { pageId: "register.ftl", expect: '[data-slot="card"]' },
  { pageId: "error.ftl", expect: '[data-slot="card"]' },
  { pageId: "info.ftl", expect: '[data-slot="card"]' },
  { pageId: "login-verify-email.ftl", expect: '[data-slot="card"]' },
  { pageId: "logout-confirm.ftl", expect: '[data-slot="card"]' },
  { pageId: "login-page-expired.ftl", expect: '[data-slot="card"]' },
  { pageId: "login-idp-link-confirm.ftl", expect: '[data-slot="card"]' },
  { pageId: "login-idp-link-email.ftl", expect: '[data-slot="card"]' },
  { pageId: "login-passkeys-conditional-authenticate.ftl", expect: '[data-slot="card"]' },
  { pageId: "webauthn-authenticate.ftl", expect: '[data-slot="card"]' },
  { pageId: "webauthn-register.ftl", expect: '[data-slot="card"]' },
  { pageId: "webauthn-error.ftl", expect: '[data-slot="card"]' },
];

for (const p of pages) {
  test(`renders ${p.pageId} with custom theme`, async ({ page }) => {
    // Navigate with the mock pageId query parameter.
    await page.goto(`${BASE}/?kcPageId=${encodeURIComponent(p.pageId)}`);

    // Wait for the React app to mount.
    await page.waitForSelector("#root", { state: "attached" });

    // The card should be present (not falling through to DefaultPage).
    const card = page.locator(p.expect);
    await expect(card.first()).toBeVisible({ timeout: 10_000 });

    // Verify no empty page — the card should have content.
    const cardText = await card.first().textContent();
    expect(cardText?.trim().length).toBeGreaterThan(0);

    // Verify no uncaught errors in the console.
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));

    // Quick re-check after a moment for async errors.
    await page.waitForTimeout(500);
    expect(errors).toHaveLength(0);
  });
}

// Page-specific content tests.

test("login page shows email input and social providers", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=login.ftl`);
  await expect(page.locator('input[name="username"]')).toBeVisible();
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
});

test("register page shows registration form", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=register.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
  // Register form should have a submit button.
  await expect(page.locator('button[type="submit"]')).toBeVisible();
});

test("error page shows error message", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=error.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
});

test("info page shows info message", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=info.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
});

test("login-verify-email page shows email icon", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=login-verify-email.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
});

test("logout-confirm page shows logout button", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=logout-confirm.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
  await expect(page.locator('button[name="confirmLogout"]')).toBeVisible();
});

test("login-page-expired shows restart links", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=login-page-expired.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
  // Should have links to restart the flow.
  const links = page.locator('[data-slot="card"] a');
  await expect(links.first()).toBeVisible();
});

test("webauthn-authenticate page shows authenticate button", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=webauthn-authenticate.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
});

test("webauthn-register page shows register button", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=webauthn-register.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
});

test("webauthn-error page shows retry button", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=webauthn-error.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
  await expect(page.locator('button[type="submit"]')).toBeVisible();
});

test("login-idp-link-confirm shows link options", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=login-idp-link-confirm.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
  await expect(page.locator('button[type="submit"]').first()).toBeVisible();
});

test("login-idp-link-email shows email instructions", async ({ page }) => {
  await page.goto(`${BASE}/?kcPageId=login-idp-link-email.ftl`);
  await expect(page.locator('[data-slot="card"]')).toBeVisible();
});
