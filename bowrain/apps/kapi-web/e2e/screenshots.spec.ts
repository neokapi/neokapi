import { test, expect } from "@playwright/test";
import { setupMockApi, seedTMEntries, seedConcepts, seedProject } from "./mock-api";
import * as path from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const SCREENSHOT_DIR = path.resolve(__dirname, "../../../../website/static/img/web-app");

test.describe("Web App Screenshots", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test("capture dashboard", async ({ page }) => {
    await setupMockApi(page);

    // Seed multiple projects so dashboard looks populated
    seedProject("Website Redesign", "en", ["fr", "de", "ja"], ["index.html", "about.html"]);
    seedProject("Mobile App v2.0", "en", ["es", "pt-BR"], ["strings.json"]);
    seedProject("API Documentation", "en", ["zh-CN", "ko"], ["docs.md", "api-ref.yaml"]);

    await page.goto("/");
    await expect(page.getByText("Website Redesign")).toBeVisible();
    await expect(page.getByText("Mobile App v2.0")).toBeVisible();
    await expect(page.getByText("API Documentation")).toBeVisible();

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "dashboard.png") });
  });

  test("capture project view", async ({ page }) => {
    await setupMockApi(page);

    seedProject("Website Redesign", "en", ["fr", "de", "ja"], [
      "index.html",
      "about.html",
      "blog.md",
      "config.json",
    ]);

    await page.goto("/");
    await page.getByText("Website Redesign").first().click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "project-view.png") });
  });

  test("capture editor", async ({ page }) => {
    await setupMockApi(page);
    seedProject("Website Redesign", "en", ["fr", "de"], ["index.html"]);

    await page.goto("/");
    await page.getByText("Website Redesign").first().click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();

    // Open file in editor
    await page.getByTestId("open-file-index.html").click();
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "editor.png") });
  });

  test("capture editor translated", async ({ page }) => {
    await setupMockApi(page);
    seedProject("Website Redesign", "en", ["fr", "de"], ["index.html"]);

    await page.goto("/");
    await page.getByText("Website Redesign").first().click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();

    await page.getByTestId("open-file-index.html").click();
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    // Pseudo-translate all blocks
    await page.getByTestId("pseudo-btn").click();
    await page.waitForTimeout(500);
    await expect(page.getByTestId("progress-text")).toContainText("100%");

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "editor-translated.png") });
  });

  test("capture editor focus view", async ({ page }) => {
    await setupMockApi(page);
    seedProject("Website Redesign", "en", ["fr", "de"], ["index.html"]);

    await page.goto("/");
    await page.getByText("Website Redesign").first().click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();

    await page.getByTestId("open-file-index.html").click();
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    // Pseudo-translate first so there's content to show
    await page.getByTestId("pseudo-btn").click();
    await page.waitForTimeout(500);

    // Switch to focus view
    await page.getByTestId("layout-focus").click();
    await page.waitForTimeout(300);
    await expect(page.getByTestId("focus-view")).toBeVisible();

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "editor-focus.png") });
  });

  test("capture TM explorer", async ({ page }) => {
    await setupMockApi(page);
    seedTMEntries();

    await page.goto("/");

    // Navigate to Memory view via sidebar
    await page.getByTestId("nav-memory").click();
    await expect(page.getByTestId("tm-explorer")).toBeVisible();
    await expect(page.getByTestId("tm-count-badge")).toContainText("5 entries");

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "tm-explorer.png") });
  });

  test("capture term explorer", async ({ page }) => {
    await setupMockApi(page);
    seedConcepts();

    await page.goto("/");

    // Navigate to Termbase view via sidebar
    await page.getByTestId("nav-termbase").click();
    await expect(page.getByTestId("term-explorer")).toBeVisible();
    await expect(page.getByTestId("term-count-badge")).toContainText("5 concepts");

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "term-explorer.png") });
  });

  test("capture settings", async ({ page }) => {
    await setupMockApi(page);

    await page.goto("/");

    // Navigate to Settings
    await page.getByTestId("nav-settings").click();
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();

    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "settings.png") });
  });
});
