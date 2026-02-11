import { test, expect } from "@playwright/test";
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
  waitForServer,
} from "./helpers/api-client";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const SCREENSHOT_DIR = path.resolve(__dirname, "../../../website/static/img/web-app");

let token: string;
let wsSlug: string;

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

  test("capture login page", async ({ page }) => {
    // Visit without token — server mode shows login page
    await page.goto("/");
    await expect(page.getByText("Sign in with SSO")).toBeVisible({ timeout: 10000 });
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "login.png") });
  });

  test("capture workspace rail and dashboard", async ({ page }) => {
    // Clean up first
    await deleteAllEditorProjects(token, wsSlug);

    // Seed projects for a populated dashboard
    const p1 = await createEditorProject(token, wsSlug, "Company Website", "en", ["fr", "de"]);
    await uploadSeedFiles(token, wsSlug, p1.id, ["about-us.html"]);

    const p2 = await createEditorProject(token, wsSlug, "Mobile App", "en", ["fr", "de"]);
    await uploadSeedFiles(token, wsSlug, p2.id, ["app-strings.json"]);

    const p3 = await createEditorProject(token, wsSlug, "Release Notes", "en", ["fr"]);
    await uploadSeedFiles(token, wsSlug, p3.id, ["release-notes.md"]);

    // Navigate with token
    await page.goto(`/?token=${token}`);
    await expect(page.getByText("Company Website").first()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Mobile App").first()).toBeVisible();
    await expect(page.getByText("Release Notes").first()).toBeVisible();

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "dashboard.png") });

    // Crop the left portion showing workspace rail for a separate screenshot
    await page.screenshot({
      path: path.join(SCREENSHOT_DIR, "workspace-rail.png"),
      clip: { x: 0, y: 0, width: 300, height: 800 },
    });
  });

  test("capture project view", async ({ page }) => {
    // Clean up and create fresh project
    await deleteAllEditorProjects(token, wsSlug);
    const p = await createEditorProject(token, wsSlug, "Company Website", "en", ["fr", "de"]);
    await uploadSeedFiles(token, wsSlug, p.id, ["about-us.html", "app-strings.json", "release-notes.md"]);

    await page.goto(`/?token=${token}`);
    await expect(page.getByText("Company Website").first()).toBeVisible({ timeout: 10000 });
    await page.getByText("Company Website").first().click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "project-view.png") });

    // Open file in editor
    await page.getByTestId("open-file-about-us.html").click();
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "editor.png") });

    // Pseudo-translate all blocks
    await page.getByTestId("pseudo-btn").click();
    await page.waitForTimeout(1000);
    await expect(page.getByTestId("progress-text")).toContainText("100%", { timeout: 10000 });

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "editor-translated.png") });

    // Switch to focus view
    await page.getByTestId("layout-focus").click();
    await page.waitForTimeout(500);
    await expect(page.getByTestId("focus-view")).toBeVisible();

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "editor-focus.png") });

    // Toggle context panel
    const contextToggle = page.getByTestId("context-panel-toggle");
    if (await contextToggle.isVisible()) {
      await contextToggle.click();
      await page.waitForTimeout(500);
      await page.screenshot({ path: path.join(SCREENSHOT_DIR, "context-panel.png") });
    }
  });

  test("capture TM explorer", async ({ page }) => {
    await seedTMEntries(token, wsSlug);

    await page.goto(`/?token=${token}`);
    await page.waitForTimeout(1000);

    await page.getByTestId("nav-memory").click();
    await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 5000 });

    // Wait for entries to load
    await page.waitForTimeout(1000);

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "tm-explorer.png") });
  });

  test("capture term explorer", async ({ page }) => {
    await seedConcepts(token, wsSlug);

    await page.goto(`/?token=${token}`);
    await page.waitForTimeout(1000);

    await page.getByTestId("nav-termbase").click();
    await expect(page.getByTestId("term-explorer")).toBeVisible({ timeout: 5000 });

    // Wait for concepts to load
    await page.waitForTimeout(1000);

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "term-explorer.png") });
  });

  test("capture settings", async ({ page }) => {
    await page.goto(`/?token=${token}`);
    await page.waitForTimeout(1000);

    await page.getByTestId("nav-settings").click();
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 5000 });

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "settings.png") });
  });
});
