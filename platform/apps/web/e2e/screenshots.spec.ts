import { test, expect, type Page } from "@playwright/test";
import * as path from "path";
import { fileURLToPath } from "url";
import {
  authenticate,
  getOrCreateWorkspace,
  createEditorProject,
  uploadSeedFiles,
  deleteAllEditorProjects,
  seedTMEntries,
  seedConcepts,
  createInvite,
  pseudoTranslateFile,
  waitForServer,
} from "./helpers/api-client";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const SCREENSHOT_BASE = path.resolve(__dirname, "../../../../website/static/img/web-app");

const BASE_URL = process.env.BOWRAIN_URL || "http://localhost:8080";

/** Inject the auth token as an HttpOnly cookie via Playwright's cookie API. */
async function injectAuthCookie(page: Page, authToken: string) {
  const url = new URL(BASE_URL);
  await page.context().addCookies([{
    name: "bowrain_session",
    value: authToken,
    domain: url.hostname,
    path: "/api/",
    httpOnly: true,
    sameSite: "Lax",
  }]);
}

async function setTheme(page: Page, theme: "dark" | "light") {
  await page.evaluate((t) => {
    const isDark = t === "dark";
    document.documentElement.classList.toggle("dark", isDark);
    localStorage.setItem("neokapi-theme", t);
  }, theme);
  // Allow CSS variables to settle
  await page.waitForTimeout(100);
}

let token: string;
let wsSlug: string;

const themes = ["dark", "light"] as const;

test.describe("Web App Screenshots", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(token, "Acme Inc.", "acme");
    wsSlug = ws.slug;
    // Clean slate: delete all existing editor projects
    await deleteAllEditorProjects(token, wsSlug);
  });

  for (const theme of themes) {
    test(`capture login page [${theme}]`, async ({ page }) => {
      const dir = path.join(SCREENSHOT_BASE, theme);
      // Visit without token — server mode redirects to Keycloak login page
      await page.goto("/");
      // Wait for Keycloak login form to appear (standard theme)
      await expect(page.locator("#kc-form-login, #username, input[name='username']").first()).toBeVisible({ timeout: 15000 });
      await page.screenshot({ path: path.join(dir, "login.png") });
    });

    test(`capture workspace rail and dashboard [${theme}]`, async ({ page }) => {
      const dir = path.join(SCREENSHOT_BASE, theme);
      // Clean up first
      await deleteAllEditorProjects(token, wsSlug);

      // Seed projects for a populated dashboard
      const p1 = await createEditorProject(token, wsSlug, "Company Website", "en", ["fr", "de"]);
      await uploadSeedFiles(token, wsSlug, p1.id, ["about-us.html"]);

      const p2 = await createEditorProject(token, wsSlug, "Mobile App", "en", ["fr", "de"]);
      await uploadSeedFiles(token, wsSlug, p2.id, ["app-strings.json"]);

      const p3 = await createEditorProject(token, wsSlug, "Release Notes", "en", ["fr"]);
      await uploadSeedFiles(token, wsSlug, p3.id, ["release-notes.md"]);

      // Navigate directly to the workspace dashboard route
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}`);
      await expect(page.getByText("Company Website").first()).toBeVisible({ timeout: 10000 });
      await expect(page.getByText("Mobile App").first()).toBeVisible();
      await expect(page.getByText("Release Notes").first()).toBeVisible();

      await setTheme(page, theme);
      await page.screenshot({ path: path.join(dir, "dashboard.png") });

      // Crop the left portion showing workspace rail for a separate screenshot
      await page.screenshot({
        path: path.join(dir, "workspace-rail.png"),
        clip: { x: 0, y: 0, width: 300, height: 800 },
      });
    });

    test(`capture project view [${theme}]`, async ({ page }) => {
      const dir = path.join(SCREENSHOT_BASE, theme);
      // Clean up and create fresh project
      await deleteAllEditorProjects(token, wsSlug);
      const p = await createEditorProject(token, wsSlug, "Company Website", "en", ["fr", "de"]);
      await uploadSeedFiles(token, wsSlug, p.id, ["about-us.html", "app-strings.json", "release-notes.md"]);

      // Navigate directly to the project detail route
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/project/${p.id}/stream/main`);
      await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 10000 });

      await setTheme(page, theme);
      await page.screenshot({ path: path.join(dir, "project-view.png") });

      // Navigate directly to the editor route
      await page.goto(`/${wsSlug}/project/${p.id}/stream/main/translate/about-us.html`);
      await expect(page.getByTestId("layout-switcher")).toBeVisible({ timeout: 30000 });
      // Switch to grid layout for screenshots
      await page.getByTestId("layout-grid").click();
      await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 10000 });

      await setTheme(page, theme);
      await page.screenshot({ path: path.join(dir, "editor.png") });

      // Pseudo-translate all blocks via API and reload
      await pseudoTranslateFile(token, wsSlug, p.id, "about-us.html", "fr");
      await page.reload();
      await expect(page.getByTestId("layout-switcher")).toBeVisible({ timeout: 30000 });
      await page.getByTestId("layout-grid").click();
      await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 10000 });
      await expect(page.getByTestId("progress-text")).toContainText("100%", { timeout: 10000 });

      await page.screenshot({ path: path.join(dir, "editor-translated.png") });

      // Switch to focus view
      await page.getByTestId("layout-focus").click();
      await page.waitForTimeout(500);
      await expect(page.getByTestId("focus-view")).toBeVisible();

      await page.screenshot({ path: path.join(dir, "editor-focus.png") });

      // Toggle context panel
      const contextToggle = page.getByTestId("context-panel-toggle");
      if (await contextToggle.isVisible()) {
        await contextToggle.click();
        await page.waitForTimeout(500);
        await page.screenshot({ path: path.join(dir, "context-panel.png") });
      }
    });

    test(`capture TM explorer [${theme}]`, async ({ page }) => {
      const dir = path.join(SCREENSHOT_BASE, theme);
      await seedTMEntries(token, wsSlug);

      // Navigate directly to the TM explorer route
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/memory`);
      await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 10000 });

      // Wait for entries to load
      await page.waitForTimeout(1000);

      await setTheme(page, theme);
      await page.screenshot({ path: path.join(dir, "tm-explorer.png") });
    });

    test(`capture term explorer [${theme}]`, async ({ page }) => {
      const dir = path.join(SCREENSHOT_BASE, theme);
      await seedConcepts(token, wsSlug);

      // Navigate directly to the termbase route
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/termbase`);
      await expect(page.getByTestId("term-explorer")).toBeVisible({ timeout: 10000 });

      // Wait for concepts to load
      await page.waitForTimeout(1000);

      await setTheme(page, theme);
      await page.screenshot({ path: path.join(dir, "term-explorer.png") });
    });

    test(`capture settings [${theme}]`, async ({ page }) => {
      const dir = path.join(SCREENSHOT_BASE, theme);
      // Navigate directly to the settings route
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/settings`);
      await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 10000 });

      await setTheme(page, theme);
      await page.screenshot({ path: path.join(dir, "settings.png") });
    });

    test(`capture invite-management [${theme}]`, async ({ page }) => {
      const dir = path.join(SCREENSHOT_BASE, theme);

      // Seed an invite so the invite list is populated
      await createInvite(token, wsSlug, "member", "translator@example.com", 1, 7);

      // Navigate directly to the settings route
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/settings`);
      await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 10000 });

      // Wait for invite list to load
      const inviteManager = page.getByTestId("invite-manager");
      await expect(inviteManager).toBeVisible({ timeout: 5000 });
      await page.waitForTimeout(500);

      await setTheme(page, theme);
      await page.screenshot({ path: path.join(dir, "invite-management.png") });
    });
  }
});
