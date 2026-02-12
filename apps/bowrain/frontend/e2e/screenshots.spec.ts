import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";
import * as path from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const SCREENSHOT_BASE = path.resolve(__dirname, "../../../../website/static/img/bowrain");

/** Helper: apply theme to the page. */
async function setTheme(page: any, theme: "glass" | "light" | "aurora") {
  await page.evaluate((t: string) => {
    const isDark = t !== "light";
    document.documentElement.classList.toggle("dark", isDark);
    document.documentElement.dataset.theme = t;
    localStorage.setItem("gokapi-theme", t);
  }, theme);
  await page.waitForTimeout(100);
}

/** Helper to click by test ID using native DOM click. */
async function clickTestId(page: any, testId: string) {
  await page.evaluate((id: string) => {
    const el = document.querySelector(`[data-testid="${id}"]`) as HTMLElement;
    if (el) el.click();
  }, testId);
}

/** Helper: set value on an input natively (avoids Playwright fill hangs). */
async function setInput(page: any, testId: string, value: string) {
  await page.evaluate(({ testId, value }: { testId: string; value: string }) => {
    const input = document.querySelector(`[data-testid="${testId}"]`) as HTMLInputElement;
    if (!input) return;
    const nativeSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    nativeSetter.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  }, { testId, value });
}

/**
 * Creates three sample projects so the dashboard looks populated.
 */
async function seedDashboard(page: any) {
  await injectMockBackend(page);
  await page.goto("/");

  // Create three projects with different languages
  const projectDefs = [
    { name: "Website Redesign", targets: "fr, de, ja" },
    { name: "Mobile App v2.0", targets: "es, pt-BR" },
    { name: "API Documentation", targets: "zh-CN, ko" },
  ];

  for (const def of projectDefs) {
    await page.getByTestId("new-project-btn").click();
    await page.getByTestId("project-name-input").fill(def.name);
    await selectMultiLocales(page, "target-langs-input", def.targets.split(",").map(s => s.trim()));
    await page.getByTestId("create-project-submit").click();
    await expect(page.getByTestId("back-to-projects")).toBeVisible();

    // Add files to make the project look real
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      const p = projects[projects.length - 1];
      if (p) {
        await backend.AddFiles(p.id, ["/src/index.html", "/src/app.json"]);
      }
    });

    // Navigate back to dashboard
    await page.getByTestId("back-to-projects").click();
    await page.waitForTimeout(200);
  }
}

/**
 * Creates a project with four files and navigates to the project view.
 */
async function openProjectView(page: any) {
  await injectMockBackend(page);
  await page.goto("/");

  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("Website Redesign");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de", "ja"]);
  await page.getByTestId("create-project-submit").click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();

  // Add four files
  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    const p = projects[0];
    if (p) {
      await backend.AddFiles(p.id, [
        "/src/index.html",
        "/src/strings.json",
        "/content/about.md",
        "/config/messages.yaml",
      ]);
    }
  });

  // Refresh by navigating away and back
  await page.locator("nav button", { hasText: "Settings" }).click();
  await page.waitForTimeout(100);
  await page.getByTestId("nav-translate").click();
  await page.waitForTimeout(200);
  await page.getByText("Website Redesign").first().click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
}

/**
 * Creates a project with a file and opens the translation editor.
 */
async function openEditor(page: any) {
  await injectMockBackend(page);
  await page.goto("/");

  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("Website Redesign");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();

  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    if (projects.length > 0) {
      await backend.AddFiles(projects[0].id, ["/src/index.html"]);
    }
  });

  // Refresh
  await page.locator("nav button", { hasText: "Settings" }).click();
  await page.waitForTimeout(100);
  await page.getByTestId("nav-translate").click();
  await page.waitForTimeout(200);
  await page.getByText("Website Redesign").first().click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

  // Open file in editor
  await expect(page.getByTestId("open-file-index.html")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    const btn = document.querySelector('[data-testid="open-file-index.html"]') as HTMLElement;
    if (btn) btn.click();
  });
  await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
}

const themes = ["glass", "light", "aurora"] as const;

test.describe("Screenshots", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  for (const theme of themes) {
    test(`capture dashboard [${theme}]`, async ({ page }) => {
      await seedDashboard(page);
      await setTheme(page, theme);

      // Should see all three projects on the dashboard
      await expect(page.getByText("Website Redesign")).toBeVisible();
      await expect(page.getByText("Mobile App v2.0")).toBeVisible();
      await expect(page.getByText("API Documentation")).toBeVisible();

      const dir = path.join(SCREENSHOT_BASE, theme);
      await page.screenshot({ path: path.join(dir, "dashboard.png") });
    });

    test(`capture project view [${theme}]`, async ({ page }) => {
      await openProjectView(page);
      await setTheme(page, theme);

      // Verify files are visible
      await expect(page.getByTestId("open-file-index.html")).toBeVisible();

      const dir = path.join(SCREENSHOT_BASE, theme);
      await page.screenshot({ path: path.join(dir, "project-view.png") });
    });

    test(`capture editor [${theme}]`, async ({ page }) => {
      await openEditor(page);
      await setTheme(page, theme);

      // Verify editor content
      await expect(page.getByTestId("block-row-0")).toBeVisible();
      await expect(page.getByTestId("progress-text")).toContainText("0%");

      const dir = path.join(SCREENSHOT_BASE, theme);
      await page.screenshot({ path: path.join(dir, "editor.png") });
    });

    test(`capture editor with preview [${theme}]`, async ({ page }) => {
      await openEditor(page);
      await setTheme(page, theme);

      // Toggle preview on
      await clickTestId(page, "layout-split-v");
      await page.waitForTimeout(300);
      await expect(page.getByTestId("split-layout")).toBeVisible();
      await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

      const dir = path.join(SCREENSHOT_BASE, theme);
      await page.screenshot({ path: path.join(dir, "editor-preview.png") });
    });

    test(`capture editor translated [${theme}]`, async ({ page }) => {
      await openEditor(page);
      await setTheme(page, theme);

      // Pseudo-translate all blocks
      await clickTestId(page, "pseudo-btn");
      await page.waitForTimeout(500);

      // Verify 100% progress
      await expect(page.getByTestId("progress-text")).toContainText("100%");

      const dir = path.join(SCREENSHOT_BASE, theme);
      await page.screenshot({ path: path.join(dir, "editor-translated.png") });
    });

    test(`capture TM explorer [${theme}]`, async ({ page }) => {
      await injectMockBackend(page);
      await page.goto("/");

      // Create project
      await page.getByTestId("new-project-btn").click();
      await page.getByTestId("project-name-input").fill("Website Redesign");
      await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
      await page.getByTestId("create-project-submit").click();
      await expect(page.getByTestId("back-to-projects")).toBeVisible();

      // Seed TM entries via mock backend
      await page.evaluate(() => {
        const backend = (window as any).__wailsMockByName;
        const projects = backend.ListProjects();
        const pid = projects[0]?.id;
        if (pid) {
          backend.AddTMEntry(pid, "Hello World", "Bonjour le monde", "en", "fr");
          backend.AddTMEntry(pid, "Welcome to our application", "Bienvenue dans notre application", "en", "fr");
          backend.AddTMEntry(pid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");
          backend.AddTMEntry(pid, "Settings", "Param\u00e8tres", "en", "fr");
          backend.AddTMEntry(pid, "Save changes", "Enregistrer les modifications", "en", "fr");
        }
      });

      // Open TM explorer
      await page.evaluate(() => {
        (document.querySelector('[data-testid="open-tm-btn"]') as HTMLElement)?.click();
      });
      await expect(page.getByTestId("tm-explorer")).toBeVisible();
      await setTheme(page, theme);

      // Trigger a refresh to show entries
      await setInput(page, "tm-search-input", " ");
      await page.waitForTimeout(400);
      await setInput(page, "tm-search-input", "");
      await page.waitForTimeout(400);

      await expect(page.getByTestId("tm-count-badge")).toContainText("5 entries");

      const dir = path.join(SCREENSHOT_BASE, theme);
      await page.screenshot({ path: path.join(dir, "tm-explorer.png") });
    });

    test(`capture flow editor [${theme}]`, async ({ page }) => {
      await injectMockBackend(page);
      await page.goto("/");
      await setTheme(page, theme);

      // Navigate to Flows view
      await page.locator("nav button", { hasText: "Flows" }).click();
      await expect(page.getByTestId("flow-list")).toBeVisible();

      // Select AI Translate flow
      await page.getByTestId("flow-item-ai-translate").click();
      await page.waitForTimeout(300);

      // Verify flow nodes are visible
      await expect(page.getByTestId("flow-node-reader")).toBeVisible();

      const dir = path.join(SCREENSHOT_BASE, theme);
      await page.screenshot({ path: path.join(dir, "flow-editor.png") });
    });

    test(`capture focus view [${theme}]`, async ({ page }) => {
      await openEditor(page);
      await setTheme(page, theme);

      // Pseudo-translate to create mixed statuses
      await clickTestId(page, "pseudo-btn");
      await page.waitForTimeout(500);

      // Switch to focus view
      await clickTestId(page, "layout-focus");
      await page.waitForTimeout(300);
      await expect(page.getByTestId("focus-view")).toBeVisible();

      // Verify focus view elements
      await expect(page.getByTestId("focus-source")).toBeVisible();
      await expect(page.getByTestId("focus-target")).toBeVisible();

      const dir = path.join(SCREENSHOT_BASE, theme);
      await page.screenshot({ path: path.join(dir, "editor-focus.png") });
    });

    test(`capture settings [${theme}]`, async ({ page }) => {
      await injectMockBackend(page);
      await page.goto("/");
      await setTheme(page, theme);

      // Navigate to Settings
      await page.locator("nav button", { hasText: "Settings" }).click();
      await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();

      // Switch to AI Providers tab (more interesting than empty General)
      await page.getByTestId("settings-tab-ai-providers").click();
      await expect(page.getByTestId("settings-ai-providers")).toBeVisible();

      // Add a provider to make the page look populated
      await page.getByTestId("add-provider-btn").click();
      await page.getByTestId("provider-name").fill("Anthropic Claude");
      await page.getByTestId("provider-type").selectOption("anthropic");
      await page.getByTestId("provider-api-key").fill("sk-ant-***");
      await page.getByTestId("provider-model").fill("claude-sonnet-4-20250514");
      await page.getByTestId("provider-save-btn").click();
      await expect(page.getByText("Anthropic Claude")).toBeVisible();

      const dir = path.join(SCREENSHOT_BASE, theme);
      await page.screenshot({ path: path.join(dir, "settings.png") });
    });
  }
});
