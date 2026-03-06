import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { setupServerApp } from "./helpers/server-setup";
import { setupServerModeApp } from "./helpers/server-mode-setup";
import { callBackend } from "./helpers/backend-call";
import { injectCursor, humanClick, humanClickNative, humanType, humanTypeNative, moveCursorTo, moveCursorToElement } from "./cursor-helper";
import { setMultiLocales, setMultiLocalesHuman, expectLocaleChips } from "./locale-helper";
import { injectWindowChrome } from "./window-chrome";

// Skip recording tests in CI - they use human-speed typing and exceed CI timeouts
// Run locally with: npx playwright test --config=playwright.recordings.config.ts
// Or in CI with FORCE_RECORDINGS=true (e.g., Screenshots & Recordings workflow)
const isCI = (process.env.CI === "true" || process.env.CI === "1") && process.env.FORCE_RECORDINGS !== "true";

/** Helper: apply theme to the page. */
async function setTheme(page: any, theme: "dark" | "light") {
  await page.evaluate((t: string) => {
    const isDark = t === "dark";
    document.documentElement.classList.toggle("dark", isDark);
    localStorage.setItem("gokapi-theme", t);
  }, theme);
  await page.waitForTimeout(100);
}

import * as path from "path";
import { fileURLToPath } from "url";

const __recFilename = fileURLToPath(import.meta.url);
const __recDirname = path.dirname(__recFilename);
const FIXTURE_DIR = path.resolve(__recDirname, "fixtures");

const useRealServer = !!process.env.BOWRAIN_SERVER_URL;
const useServerMode = !!process.env.WAILS_SERVER_MODE;

/** Resolve fixture file path (absolute) for use in server mode AddItems calls. */
function fixture(name: string): string {
  return path.join(FIXTURE_DIR, name);
}

/** Setup helper - injects backend (mock or real server or headless binary), cursor, and window chrome */
async function setupRecording(page: any, title: string = "Bowrain", theme: "dark" | "light" = "dark") {
  if (useRealServer) {
    await setupServerApp(page);
  } else if (useServerMode) {
    await setupServerModeApp(page);
  } else {
    await setupLocalApp(page);
  }
  await setTheme(page, theme);
  await injectCursor(page);
  await injectWindowChrome(page, title);
  // Start cursor in a natural position
  await moveCursorTo(page, 640, 400, 0);
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

/** Pause for visual effect in recordings */
async function pause(page: any, ms: number = 500) {
  await page.waitForTimeout(ms);
}

/** Find a project by name from the backend project list. */
async function findProject(page: any, name: string) {
  const projects = await callBackend(page, "ListProjects");
  return projects.find((p: any) => p.name === name);
}

// Use conditional describe to skip entire suite in CI; serial ensures order + shared state
const describeOrSkip = isCI ? test.describe.skip : test.describe.serial;

const themes = ["dark", "light"] as const;

describeOrSkip("Video Recordings", () => {
  // Video settings are configured in playwright.recordings.config.ts
  // Videos are saved to test-results/ - use `npm run recordings:copy` to copy to docs
  // Skipped in CI due to human-speed typing exceeding timeouts
  //
  // Tests are ordered as a continuous narrative: each test builds on the state
  // left by previous tests. Projects accumulate intentionally in the workspace.

  for (const theme of themes) {

  // ── 1. Settings Configuration ─────────────────────────────────────────────────
  // Setup workspace — configure an AI provider (no projects created)
  test(`record settings configuration [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — Settings", theme);
    await pause(page, 600);

    // Navigate to Settings
    await expect(page.getByTestId("nav-settings")).toBeVisible();
    await humanClick(page, page.getByTestId("nav-settings"));
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Switch to AI Providers tab
    await expect(page.getByTestId("settings-tab-ai-providers")).toBeVisible();
    await humanClick(page, page.getByTestId("settings-tab-ai-providers"));
    await expect(page.getByTestId("settings-ai-providers")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // In server mode without a real bowrain-server, AI providers are unavailable.
    // Only populate providers when the add button exists.
    const addBtn = page.getByTestId("add-provider-btn");
    if (await addBtn.isVisible({ timeout: 1000 }).catch(() => false)) {
      await humanClick(page, addBtn);
      await expect(page.getByTestId("provider-name")).toBeVisible({ timeout: 5000 });
      await pause(page, 400);

      // Fill provider details
      await humanType(page, page.getByTestId("provider-name"), "Anthropic Claude");
      await pause(page, 300);

      // Select provider type
      await humanClick(page, page.getByTestId("provider-type"));
      await page.getByRole("option", { name: "Anthropic" }).click();
      await pause(page, 400);

      // Enter API key
      await humanType(page, page.getByTestId("provider-api-key"), "sk-ant-api03-xxxx");
      await pause(page, 300);

      // Enter model
      await humanType(page, page.getByTestId("provider-model"), "claude-sonnet-4-20250514");
      await pause(page, 400);

      // Save provider
      await humanClick(page, page.getByTestId("provider-save-btn"));
      await expect(page.getByText("Anthropic Claude").first()).toBeVisible({ timeout: 5000 });
    }
    await pause(page, 1000);
  });

  // ── 2. Create Project Flow ────────────────────────────────────────────────────
  // First project — shows project creation (creates "Website Redesign")
  test(`record create project flow [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — New Project", theme);
    await pause(page, 800);

    // Click new project button
    await expect(page.getByTestId("new-project-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();
    await pause(page, 400);

    // Fill in project name
    await humanType(page, page.getByTestId("project-name-input"), "Website Redesign");
    await pause(page, 300);

    // Fill in target languages
    await setMultiLocalesHuman(page, "target-langs-input", ["fr", "de", "ja"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["fr", "de", "ja"]);
    await pause(page, 400);

    // Create project
    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Add files via backend
    const proj = await findProject(page, "Website Redesign");
    if (proj) {
      await callBackend(page, "AddItems", proj.id, useServerMode
        ? [fixture("index.html"), fixture("strings.json"), fixture("about.md")]
        : ["/src/index.html", "/src/strings.json", "/content/about.md"]);
    }

    // Navigate away and back to refresh file list
    await humanClick(page, page.getByTestId("nav-settings"));
    await pause(page, 300);
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Website Redesign").first()).toBeVisible({ timeout: 5000 });
    await pause(page, 300);

    // Click into project to see files
    await humanClick(page, page.getByText("Website Redesign").first());
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("open-file-index.html")).toBeVisible({ timeout: 5000 });
    await pause(page, 1500);
  });

  // ── 3. Translation Editor Workflow ────────────────────────────────────────────
  // Hero demo — grid, split, preview, translate (creates "Company Website")
  test(`record translation editor workflow [${theme}]`, async ({ page }) => {
    // Hero demo: Translating a realistic HTML page to Norwegian
    // Shows: grid view → translate blocks → toggle to split view → translate more
    await setupRecording(page, "Bowrain — Translation Editor", theme);

    // Create project with Norwegian as target
    await page.getByTestId("new-project-btn").click();
    await page.getByTestId("project-name-input").fill("Company Website");
    await setMultiLocales(page, "target-langs-input", ["nb"]);
    await page.getByTestId("create-project-submit").click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();

    // Add HTML file with rich content - realistic webpage with headers, paragraphs, inline elements
    if (useServerMode) {
      // In server mode, add a real file via the backend
      const teProj = await findProject(page, "Company Website");
      if (teProj) {
        await callBackend(page, "AddItems", teProj.id, [fixture("about-us.html")]);
      }
    }
    // In mock mode, set up custom blocks and preview rendering
    if (!useServerMode) await page.evaluate(async () => {
      const backend = (window as any).__wailsMock;
      const IDS = (window as any).__wailsIDs;
      const projects = await backend[IDS.ListProjects]();
      const projectId = projects[0]?.id;
      if (!projectId) return;

      // Add the file entry
      const p = projects[0];
      p.items.push({
        name: "about-us.html",
        format: "html",
        type: "file",
        size: 8192,
        block_count: 10,
        word_count: 156,
      });

      // Rich HTML content with headers, paragraphs, and inline formatting
      const blocks = [
        { id: "block-h1", source: "About Our Company", targets: {}, translatable: true, has_spans: false, properties: { element: "h1" } },
        { id: "block-p1", source: "We are a leading technology company founded in 2015.", targets: {}, translatable: true, has_spans: false, properties: { element: "p" } },
        { id: "block-p2", source: "Our mission is to deliver innovative solutions that transform businesses.", targets: {}, translatable: true, has_spans: true, properties: { element: "p" } },
        { id: "block-h2", source: "Our Values", targets: {}, translatable: true, has_spans: false, properties: { element: "h2" } },
        { id: "block-p3", source: "We believe in quality, integrity, and customer success.", targets: {}, translatable: true, has_spans: true, properties: { element: "p" } },
        { id: "block-p4", source: "Every project starts with understanding your unique needs.", targets: {}, translatable: true, has_spans: false, properties: { element: "p" } },
        { id: "block-h2b", source: "Meet the Team", targets: {}, translatable: true, has_spans: false, properties: { element: "h2" } },
        { id: "block-p5", source: "Our talented team brings decades of combined experience.", targets: {}, translatable: true, has_spans: false, properties: { element: "p" } },
        { id: "block-p6", source: "Contact us to learn how we can help your business grow.", targets: {}, translatable: true, has_spans: true, properties: { element: "p" } },
        { id: "block-link", source: "Read more about our services", targets: {}, translatable: true, has_spans: false, properties: { element: "a" } },
      ];

      // Store blocks
      if (!(window as any).__projectFiles) (window as any).__projectFiles = {};
      if (!(window as any).__projectFiles[projectId]) (window as any).__projectFiles[projectId] = {};
      (window as any).__projectFiles[projectId]["about-us.html"] = blocks;

      // Patch GetItemBlocks
      const origGetItemBlocks = backend[IDS.GetItemBlocks];
      backend[IDS.GetItemBlocks] = (pid: string, fileName: string) => {
        const customBlocks = (window as any).__projectFiles?.[pid]?.[fileName];
        if (customBlocks) return customBlocks.map((b: any) => ({ ...b, targets: { ...b.targets } }));
        return origGetItemBlocks(pid, fileName);
      };

      // Patch UpdateBlockTarget
      backend[IDS.UpdateBlockTarget] = (req: any) => {
        const blocks = (window as any).__projectFiles?.[req.project_id]?.[req.item_name || req.file_name];
        if (blocks) {
          const block = blocks.find((b: any) => b.id === req.block_id);
          if (block) block.targets[req.target_locale] = req.text;
        }
      };

      // Patch RenderDocumentPreview for rich HTML preview
      backend[IDS.RenderDocumentPreview] = (_projectID: string, _itemName: string, targetLocale: string) => {
        const b = (window as any).__projectFiles?.[projectId]?.["about-us.html"] || [];
        const getContent = (block: any) => block.targets[targetLocale] || block.source;
        return `<!DOCTYPE html><html><head><style>
          body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; padding: 24px; line-height: 1.6; color: #1a1a1a; max-width: 640px; }
          h1 { font-size: 28px; margin-bottom: 16px; color: #111; }
          h2 { font-size: 20px; margin-top: 24px; margin-bottom: 12px; color: #333; }
          p { margin-bottom: 12px; color: #444; }
          a { color: #2563eb; }
          em { font-style: italic; }
          strong { font-weight: 600; }
          kat-block { cursor: pointer; border-radius: 2px; display: inline; transition: background 0.15s; }
          kat-block:hover { background-color: rgba(59,130,246,0.12); }
          kat-block.kat-selected { background-color: rgba(59,130,246,0.2); outline: 2px solid #3b82f6; outline-offset: 1px; }
        </style></head><body>
          <h1><kat-block id="block-h1">${getContent(b[0])}</kat-block></h1>
          <p><kat-block id="block-p1">${getContent(b[1])}</kat-block></p>
          <p><kat-block id="block-p2">${getContent(b[2])}</kat-block></p>
          <h2><kat-block id="block-h2">${getContent(b[3])}</kat-block></h2>
          <p><kat-block id="block-p3">${getContent(b[4])}</kat-block></p>
          <p><kat-block id="block-p4">${getContent(b[5])}</kat-block></p>
          <h2><kat-block id="block-h2b">${getContent(b[6])}</kat-block></h2>
          <p><kat-block id="block-p5">${getContent(b[7])}</kat-block></p>
          <p><kat-block id="block-p6">${getContent(b[8])}</kat-block></p>
          <p><a href="#"><kat-block id="block-link">${getContent(b[9])}</kat-block></a></p>
        <script>
          document.querySelectorAll('kat-block').forEach(el => {
            el.addEventListener('click', () => window.parent.postMessage({ type: 'kat-block-click', blockId: el.id }, '*'));
          });
          window.addEventListener('message', (e) => {
            if (e.data?.type === 'kat-select-block') {
              document.querySelector('.kat-selected')?.classList.remove('kat-selected');
              document.getElementById(e.data.blockId)?.classList.add('kat-selected');
            }
            if (e.data?.type === 'kat-update-block') {
              const el = document.getElementById(e.data.blockId);
              if (el) el.innerHTML = e.data.html;
            }
          });
          window.parent.postMessage({ type: 'kat-iframe-ready' }, '*');
        </script></body></html>`;
      };
    });

    // Refresh to show the file
    await page.getByTestId("nav-settings").click();
    await page.waitForTimeout(100);
    await page.getByTestId("nav-translate").click();
    await page.waitForTimeout(200);
    await humanClick(page, page.getByText("Company Website").first());
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Open the HTML file in editor
    await expect(page.getByTestId("open-file-about-us.html")).toBeVisible({ timeout: 5000 });
    await humanClick(page, page.getByTestId("open-file-about-us.html"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // View content in grid view - all source strings visible
    await expect(page.getByTestId("block-row-0")).toBeVisible();
    await pause(page, 500);

    // === PHASE 1: Translate 2 units in default grid view (no preview) ===

    // Block 0: h1 - "About Our Company"
    await page.getByTestId("target-cell-0").dblclick();
    await expect(page.getByTestId("edit-target-0")).toBeVisible();
    await moveCursorTo(page, 900, 200, 200);
    await humanType(page, page.getByTestId("edit-target-0"), "Om selskapet vårt");
    await pause(page, 200);
    await page.keyboard.press("Tab");
    await pause(page, 200);

    // Block 1: p - "We are a leading technology company..."
    await page.getByTestId("target-cell-1").dblclick();
    await expect(page.getByTestId("edit-target-1")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-1"), "Vi er et ledende teknologiselskap grunnlagt i 2015.");
    await pause(page, 200);
    await page.keyboard.press("Tab");
    await pause(page, 400);

    // === PHASE 2: Switch to Split View, translate 2 units (preview shows source) ===
    await humanClick(page, page.getByTestId("layout-split-v"));
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Block 2: p - "Our mission is to deliver..."
    await page.getByTestId("target-cell-2").dblclick();
    await expect(page.getByTestId("edit-target-2")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-2"), "Vår misjon er å levere innovative løsninger som transformerer bedrifter.");
    await pause(page, 200);
    await page.keyboard.press("Tab");
    await pause(page, 200);

    // Block 3: h2 - "Our Values"
    await page.getByTestId("target-cell-3").dblclick();
    await expect(page.getByTestId("edit-target-3")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-3"), "Våre verdier");
    await pause(page, 200);
    await page.keyboard.press("Tab");
    await pause(page, 400);

    // === PHASE 3: Toggle preview to show Target, translate 2 more units ===
    // Hover over preview to reveal toggle button
    await page.getByTestId("preview-iframe").hover();
    await pause(page, 300);
    await humanClick(page, page.getByTestId("preview-target-toggle"));
    await pause(page, 600); // Show that preview now displays translated text

    // Skip block 4 in server mode — it has inline spans (<strong>/<em>) which render
    // as a Lexical rich editor (TargetCellEditor) instead of a plain textarea.
    // Edit blocks 5 and 6 instead (plain text in both server and mock mode).
    await page.getByTestId("target-cell-5").dblclick();
    await expect(page.getByTestId("edit-target-5")).toBeVisible({ timeout: 5000 });
    await humanType(page, page.getByTestId("edit-target-5"), "Vi tror på kvalitet, integritet og kundesuksess.");
    await pause(page, 200);
    await page.keyboard.press("Tab");
    await pause(page, 200);

    await page.getByTestId("target-cell-6").dblclick();
    await expect(page.getByTestId("edit-target-6")).toBeVisible({ timeout: 5000 });
    await humanType(page, page.getByTestId("edit-target-6"), "Hvert prosjekt starter med å forstå dine unike behov.");
    await pause(page, 200);
    await page.keyboard.press("Tab");
    await pause(page, 400);

    // === PHASE 4: Use pseudo-translate for remaining blocks ===
    await humanClick(page, page.getByTestId("pseudo-btn"));
    await pause(page, 4000); // Hold for 4 seconds to show completed state
  });

  // ── 4. Focus View Editing ─────────────────────────────────────────────────────
  // Alternative editing mode (creates "Marketing Site")
  test(`record focus view editing [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — Focus View", theme);
    await pause(page, 600);

    // Create project
    await expect(page.getByTestId("new-project-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();

    await humanType(page, page.getByTestId("project-name-input"), "Marketing Site");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["fr"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["fr"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

    // Add file via backend
    const focusProj = await findProject(page, "Marketing Site");
    if (focusProj) {
      await callBackend(page, "AddItems", focusProj.id, [fixture("homepage.html")]);
    }

    // Refresh to show file
    await humanClick(page, page.getByTestId("nav-settings"));
    await pause(page, 200);
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Marketing Site").first()).toBeVisible({ timeout: 5000 });
    await pause(page, 200);

    await humanClick(page, page.getByText("Marketing Site").first());
    await expect(page.getByTestId("open-file-homepage.html")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Open file in editor
    await humanClick(page, page.getByTestId("open-file-homepage.html"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Switch to focus view
    await humanClick(page, page.getByTestId("layout-focus"));
    await expect(page.getByTestId("focus-view")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("focus-source")).toBeVisible();
    await pause(page, 600);

    // Type translation in focus view
    const focusTarget = page.getByTestId("focus-target").locator("textarea");
    await humanClick(page, focusTarget);
    await pause(page, 200);
    await humanType(page, focusTarget, "Bonjour depuis la page d'accueil");
    await pause(page, 500);

    // Navigate to next block with Tab
    await page.keyboard.press("Tab");
    await pause(page, 400);

    // Type another translation
    await humanType(page, focusTarget, "Bienvenue sur notre application");
    await pause(page, 500);

    // Navigate once more
    await page.keyboard.press("Tab");
    await pause(page, 400);

    // Show the third block
    await humanType(page, focusTarget, "Cliquez ici pour continuer");
    await pause(page, 800);
  });

  // ── 5. End-to-End Translation Workflow ────────────────────────────────────────
  // Complete workflow start-to-finish (creates "Product Launch")
  test(`record end-to-end translation workflow [${theme}]`, async ({ page }) => {
    // Complete workflow: create project → add file → translate in different views → export
    await setupRecording(page, "Bowrain — Complete Workflow", theme);
    await pause(page, 600);

    // Create project
    await expect(page.getByTestId("new-project-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();

    await humanType(page, page.getByTestId("project-name-input"), "Product Launch");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["fr", "de"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["fr", "de"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Add file via backend
    const e2eProj = await findProject(page, "Product Launch");
    if (e2eProj) {
      await callBackend(page, "AddItems", e2eProj.id, [fixture("landing-page.html")]);
    }

    // Refresh to show file
    await humanClick(page, page.getByTestId("nav-settings"));
    await pause(page, 200);
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Product Launch").first()).toBeVisible({ timeout: 5000 });
    await pause(page, 200);

    await humanClick(page, page.getByText("Product Launch").first());
    await expect(page.getByTestId("open-file-landing-page.html")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Open file in editor
    await humanClick(page, page.getByTestId("open-file-landing-page.html"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("block-row-0")).toBeVisible();
    await pause(page, 500);

    // View 1: Grid view - translate first block
    await page.getByTestId("target-cell-0").dblclick();
    await expect(page.getByTestId("edit-target-0")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-0"), "Bienvenue chez Acme");
    await pause(page, 200);
    await page.keyboard.press("Tab");
    await pause(page, 400);

    // View 2: Split view with preview
    await humanClick(page, page.getByTestId("layout-split-v"));
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Translate second block in split view
    await page.getByTestId("target-cell-1").dblclick();
    await expect(page.getByTestId("edit-target-1")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-1"), "La meilleure solution pour vos besoins");
    await pause(page, 200);
    await page.keyboard.press("Tab");
    await pause(page, 400);

    // View 3: Focus view
    await humanClick(page, page.getByTestId("layout-focus"));
    await expect(page.getByTestId("focus-view")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Translate in focus view
    const focusTarget = page.getByTestId("focus-target").locator("textarea");
    await humanClick(page, focusTarget);
    await humanType(page, focusTarget, "Cliquez ici pour en savoir plus");
    await pause(page, 400);

    // Use pseudo-translate for rest
    await humanClick(page, page.getByTestId("pseudo-btn"));
    await pause(page, 600);

    // Switch back to grid to see all translations
    await humanClick(page, page.getByTestId("layout-grid"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 1200);
  });

  // ── 6. TM Explorer ────────────────────────────────────────────────────────────
  // Explore translation memory (creates "Documentation")
  test(`record TM explorer [${theme}]`, async ({ page }, testInfo) => {
    test.skip(!!process.env.CI, "Flaky in CI - TM search input timing issue");
    await setupRecording(page, "Bowrain — Translation Memory", theme);
    await pause(page, 600);

    // Create project
    await expect(page.getByTestId("new-project-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();

    await humanType(page, page.getByTestId("project-name-input"), "Documentation");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["fr"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["fr"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Seed TM entries
    const tmProj = await findProject(page, "Documentation");
    const tmPid = tmProj?.id;
    if (tmPid) {
      await callBackend(page, "AddTMEntry", tmPid, "Hello World", "Bonjour le monde", "en", "fr");
      await callBackend(page, "AddTMEntry", tmPid, "Welcome to our application", "Bienvenue dans notre application", "en", "fr");
      await callBackend(page, "AddTMEntry", tmPid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");
      await callBackend(page, "AddTMEntry", tmPid, "Settings", "Paramètres", "en", "fr");
      await callBackend(page, "AddTMEntry", tmPid, "Save changes", "Enregistrer les modifications", "en", "fr");
    }

    // Open TM explorer
    await expect(page.getByTestId("open-tm-btn")).toBeVisible();
    await moveCursorTo(page, 1100, 100, 300);
    await page.evaluate(() => {
      (document.querySelector('[data-testid="open-tm-btn"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // TM explorer is open - show the entries
    await expect(page.getByTestId("tm-search-input")).toBeVisible();
    await pause(page, 400);

    // Type search query
    const searchInput = page.getByTestId("tm-search-input");
    await moveCursorToElement(page, searchInput, 350);
    await pause(page, 500);
    await searchInput.fill("Hello");
    await pause(page, 600);

    // Show results
    await expect(page.getByText("Bonjour")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Clear search
    await page.getByTestId("tm-search-input").fill("");
    await pause(page, 400);

    // Type another search
    await humanType(page, page.getByTestId("tm-search-input"), "Settings");
    await pause(page, 600);

    // Show filtered results
    await expect(page.getByText("Paramètres")).toBeVisible({ timeout: 5000 });
    await pause(page, 800);
  });

  // ── 7. TM Leverage Workflow ───────────────────────────────────────────────────
  // Apply TM to content (creates "Website Translation")
  test(`record TM leverage workflow [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — TM Leverage", theme);
    await pause(page, 600);

    // Create project
    await expect(page.getByTestId("new-project-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();

    await humanType(page, page.getByTestId("project-name-input"), "Website Translation");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["fr"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["fr"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Seed TM entries and add file via backend
    const leverageProj = await findProject(page, "Website Translation");
    const leveragePid = leverageProj?.id;
    if (leveragePid) {
      // Add TM entries matching the generated blocks
      await callBackend(page, "AddTMEntry", leveragePid, "Hello from landing.html", "Bonjour depuis landing.html", "en", "fr");
      await callBackend(page, "AddTMEntry", leveragePid, "Welcome to our application", "Bienvenue dans notre application", "en", "fr");
      await callBackend(page, "AddTMEntry", leveragePid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");

      await callBackend(page, "AddItems", leveragePid, [fixture("landing.html")]);
    }

    // Refresh to show file
    await humanClick(page, page.getByTestId("nav-settings"));
    await pause(page, 200);
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Website Translation").first()).toBeVisible({ timeout: 5000 });
    await pause(page, 200);

    await humanClick(page, page.getByText("Website Translation").first());
    await expect(page.getByTestId("open-file-landing.html")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Open file in editor
    await humanClick(page, page.getByTestId("open-file-landing.html"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Show blocks are untranslated
    await pause(page, 400);

    // Click TM Lookup button
    await humanClick(page, page.getByTestId("tm-btn"));
    await pause(page, 1000);

    // Navigate away and back to show TM-filled blocks
    await page.getByTestId("nav-settings").click();
    await page.waitForTimeout(100);
    await page.getByTestId("nav-translate").click();
    await page.waitForTimeout(200);
    await page.getByText("Website Translation").first().click();
    await page.waitForTimeout(200);
    await page.evaluate(() => {
      (document.querySelector('[data-testid="open-file-landing.html"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Show filled blocks
    await pause(page, 500);

    // Open context panel to show per-block TM matches
    await humanClickNative(page, "context-panel-toggle");
    await expect(page.getByTestId("context-panel")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Click on block 1 to see its TM match
    await humanClickNative(page, "block-row-1");
    await pause(page, 600);

    // Click on block 2 to see different matches
    await humanClickNative(page, "block-row-2");
    await pause(page, 1200);
  });

  // ── 8. Term Explorer ──────────────────────────────────────────────────────────
  // Build terminology (creates "Software Docs")
  test(`record term explorer [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — Terminology", theme);
    await pause(page, 600);

    // Create project
    await expect(page.getByTestId("new-project-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();

    await humanType(page, page.getByTestId("project-name-input"), "Software Docs");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["fr"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["fr"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Open terminology explorer
    await expect(page.getByTestId("open-terms-btn")).toBeVisible();
    await humanClickNative(page, "open-terms-btn");
    await expect(page.getByTestId("term-explorer")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Show empty state
    await expect(page.getByTestId("term-empty-state")).toBeVisible();
    await pause(page, 400);

    // Add first concept via the form
    await humanClickNative(page, "term-add-btn");
    await expect(page.getByTestId("term-add-form")).toBeVisible();
    await pause(page, 300);

    await humanTypeNative(page, "term-add-domain", "Security");
    await pause(page, 200);
    await humanTypeNative(page, "term-add-definition", "Process of verifying identity");
    await pause(page, 200);

    // Fill first term using native input (React-controlled inputs)
    await page.evaluate(() => {
      const inputs = document.querySelectorAll('[data-testid="term-add-form"] input[placeholder="Term text"]');
      const nativeSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
      if (inputs[0]) {
        nativeSetter.call(inputs[0], 'authentication');
        inputs[0].dispatchEvent(new Event('input', { bubbles: true }));
        inputs[0].dispatchEvent(new Event('change', { bubbles: true }));
      }
      if (inputs[1]) {
        nativeSetter.call(inputs[1], 'authentification');
        inputs[1].dispatchEvent(new Event('input', { bubbles: true }));
        inputs[1].dispatchEvent(new Event('change', { bubbles: true }));
      }
    });
    await pause(page, 300);

    await humanClickNative(page, "term-add-submit");
    await pause(page, 400);

    // Add more concepts via backend for demo
    const termProj = await findProject(page, "Software Docs");
    const termPid = termProj?.id;
    if (termPid) {
      await callBackend(page, "AddConcept", {
        project_id: termPid, domain: "Security",
        definition: "Converting data to prevent unauthorized access",
        terms: [
          { text: "encryption", locale: "en", status: "preferred" },
          { text: "chiffrement", locale: "fr", status: "preferred" },
        ],
      });
      await callBackend(page, "AddConcept", {
        project_id: termPid, domain: "UI",
        definition: "Visual panel for settings",
        terms: [
          { text: "dashboard", locale: "en", status: "approved" },
          { text: "tableau de bord", locale: "fr", status: "approved" },
        ],
      });
      await callBackend(page, "AddConcept", {
        project_id: termPid, domain: "Data",
        definition: "Structured data storage",
        terms: [
          { text: "database", locale: "en", status: "preferred" },
          { text: "base de données", locale: "fr", status: "preferred" },
        ],
      });
    }

    // Search to refresh list and show new concepts
    await humanTypeNative(page, "term-search-input", " ");
    await pause(page, 400);

    // Clear search to show all
    await humanTypeNative(page, "term-search-input", "");
    await pause(page, 400);

    // Search for "encrypt"
    await humanTypeNative(page, "term-search-input", "encrypt");
    await pause(page, 600);

    // Show filtered result
    await pause(page, 400);

    // Clear and show all
    await humanTypeNative(page, "term-search-input", "");
    await pause(page, 400);

    // Trigger a refresh
    await humanTypeNative(page, "term-search-input", " ");
    await pause(page, 400);
    await humanTypeNative(page, "term-search-input", "");
    await pause(page, 1200);
  });

  // ── 9. Context Panel ──────────────────────────────────────────────────────────
  // TM + terms in context (creates "Web Application")
  test(`record context panel [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — Context Panel", theme);
    await pause(page, 600);

    // Create project
    await expect(page.getByTestId("new-project-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();

    await humanType(page, page.getByTestId("project-name-input"), "Web Application");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["fr"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["fr"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

    // Add TM entries and terminology, then add file
    const ctxProj = await findProject(page, "Web Application");
    const ctxPid = ctxProj?.id;
    if (ctxPid) {
      // TM entries
      await callBackend(page, "AddTMEntry", ctxPid, "Hello from app.html", "Bonjour depuis app.html", "en", "fr");
      await callBackend(page, "AddTMEntry", ctxPid, "Welcome to our application", "Bienvenue dans notre application", "en", "fr");
      await callBackend(page, "AddTMEntry", ctxPid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");

      // Terminology
      await callBackend(page, "AddConcept", {
        project_id: ctxPid, domain: "UI",
        terms: [
          { text: "application", locale: "en", status: "preferred" },
          { text: "application", locale: "fr", status: "preferred" },
        ],
      });
      await callBackend(page, "AddConcept", {
        project_id: ctxPid, domain: "Navigation",
        terms: [
          { text: "continue", locale: "en", status: "approved" },
          { text: "continuer", locale: "fr", status: "approved" },
        ],
      });

      await callBackend(page, "AddItems", ctxPid, [fixture("app.html")]);
    }

    // Refresh to show file
    await humanClick(page, page.getByTestId("nav-settings"));
    await pause(page, 200);
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Web Application").first()).toBeVisible({ timeout: 5000 });
    await pause(page, 200);

    await humanClick(page, page.getByText("Web Application").first());
    await expect(page.getByTestId("open-file-app.html")).toBeVisible({ timeout: 5000 });
    await pause(page, 300);

    // Open editor
    await humanClick(page, page.getByTestId("open-file-app.html"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Open context panel
    await humanClickNative(page, "context-panel-toggle");
    await expect(page.getByTestId("context-panel")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Select block 0 - show TM match
    await humanClickNative(page, "block-row-0");
    await pause(page, 800);

    // Navigate to block 1 - show TM + term matches (has "application")
    await humanClickNative(page, "block-row-1");
    await pause(page, 800);

    // Navigate to block 2 - show TM + term matches (has "continue")
    await humanClickNative(page, "block-row-2");
    await pause(page, 800);

    // Apply TM match on block 2
    await page.evaluate(() => {
      (document.querySelector('[data-testid="tm-apply-0"]') as HTMLElement)?.click();
    });
    await pause(page, 800);

    // Show "Applied" feedback
    await pause(page, 1000);
  });

  // ── 10. Visual Editor Workflow ────────────────────────────────────────────────
  // Advanced visual editor (creates "Help Center")
  test(`record visual editor workflow [${theme}]`, async ({ page }) => {
    // Hero demo: Visual translation editor with rich HTML preview,
    // keyboard navigation, inline codes, TM matches, terms, reference locales, review mode
    await setupRecording(page, "Bowrain — Visual Editor", theme);

    // Unicode markers matching the Go model
    const mo = "\uE001"; // opening
    const mc = "\uE002"; // closing
    const mp = "\uE003"; // placeholder

    // Create project with Norwegian + Swedish targets
    await page.getByTestId("new-project-btn").click();
    await page.getByTestId("project-name-input").fill("Help Center");
    await setMultiLocales(page, "target-langs-input", ["nb", "sv"]);
    await page.getByTestId("create-project-submit").click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();

    // Set up rich HTML file with inline codes, TM entries, and terminology
    if (useServerMode) {
      // In server mode, add a real file and seed TM/terms via the backend
      const veProj = await findProject(page, "Help Center");
      const vePid = veProj?.id;
      if (vePid) {
        await callBackend(page, "AddItems", vePid, [fixture("help-center.html")]);
        await callBackend(page, "AddTMEntry", vePid, "Welcome to the CloudSync Pro documentation and support portal.", "Velkommen til CloudSync Pro-dokumentasjonen og støtteportalen.", "en", "nb");
        await callBackend(page, "AddTMEntry", vePid, "Getting Started", "Kom i gang", "en", "nb");
        await callBackend(page, "AddTMEntry", vePid, "Security & Privacy", "Sikkerhet og personvern", "en", "nb");
        await callBackend(page, "AddConcept", {
          project_id: vePid, domain: "Security",
          definition: "Converting data to prevent unauthorized access",
          terms: [
            { text: "encryption", locale: "en", status: "preferred" },
            { text: "kryptering", locale: "nb", status: "preferred" },
          ],
        });
        await callBackend(page, "AddConcept", {
          project_id: vePid, domain: "Technology",
          definition: "The process of keeping files in sync across devices",
          terms: [
            { text: "synchronization", locale: "en", status: "preferred" },
            { text: "synkronisering", locale: "nb", status: "preferred" },
          ],
        });
      }
    } else await page.evaluate(async ({ mo, mc, mp }: { mo: string; mc: string; mp: string }) => {
      const backend = (window as any).__wailsMock;
      const IDS = (window as any).__wailsIDs;
      const byName = (window as any).__wailsMockByName;
      const projects = await backend[IDS.ListProjects]();
      const projectId = projects[0]?.id;
      if (!projectId) return;

      const p = projects[0];
      p.items.push({
        name: "help-center.html",
        format: "html",
        type: "file",
        size: 12288,
        block_count: 11,
        word_count: 198,
      });

      // Rich HTML blocks with inline codes
      const blocks = [
        {
          id: "block-h1", source: "CloudSync Pro Help Center",
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: false,
          properties: { element: "h1" },
        },
        {
          id: "block-p1", source: "Welcome to the CloudSync Pro documentation and support portal.",
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: false,
          properties: { element: "p" },
        },
        {
          id: "block-p2",
          source: "Learn how to sync your files securely across all your devices.",
          source_coded: `Learn how to ${mo}sync${mc} your files ${mo}securely${mc} across all your devices.`,
          source_spans: [
            { span_type: "opening", type: "b", id: "b1", data: "<b>" },
            { span_type: "closing", type: "b", id: "b1", data: "</b>" },
            { span_type: "opening", type: "i", id: "i1", data: "<i>" },
            { span_type: "closing", type: "i", id: "i1", data: "</i>" },
          ],
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: true,
          properties: { element: "p" },
        },
        {
          id: "block-h2a", source: "Getting Started",
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: false,
          properties: { element: "h2" },
        },
        {
          id: "block-p3",
          source: "Download and install the CloudSync Pro application on your device.",
          source_coded: `Download and install the ${mo}CloudSync Pro${mc} application on your device.`,
          source_spans: [
            { span_type: "opening", type: "a", id: "a1", data: '<a href="#">' },
            { span_type: "closing", type: "a", id: "a1", data: "</a>" },
          ],
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: true,
          properties: { element: "p" },
        },
        {
          id: "block-p4",
          source: "Create your account and configure your synchronization preferences.",
          source_coded: `Create your account and configure your ${mo}synchronization${mc} preferences.`,
          source_spans: [
            { span_type: "opening", type: "code", id: "code1", data: "<code>" },
            { span_type: "closing", type: "code", id: "code1", data: "</code>" },
          ],
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: true,
          properties: { element: "p" },
        },
        {
          id: "block-h2b", source: "Security & Privacy",
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: false,
          properties: { element: "h2" },
        },
        {
          id: "block-p5",
          source: "All data is encrypted with AES-256 encryption during transfer. Your privacy is our priority.",
          source_coded: `All data is encrypted with ${mo}AES-256${mc} encryption during transfer.${mp} Your privacy is our priority.`,
          source_spans: [
            { span_type: "opening", type: "b", id: "b2", data: "<b>" },
            { span_type: "closing", type: "b", id: "b2", data: "</b>" },
            { span_type: "placeholder", type: "br", id: "br1", data: "<br/>" },
          ],
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: true,
          properties: { element: "p" },
        },
        {
          id: "block-p6",
          source: "Your files are stored securely in our certified data centers.",
          source_coded: `Your files are stored securely in our ${mo}certified data centers${mc}.`,
          source_spans: [
            { span_type: "opening", type: "a", id: "a2", data: '<a href="/security">' },
            { span_type: "closing", type: "a", id: "a2", data: "</a>" },
          ],
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: true,
          properties: { element: "p" },
        },
        {
          id: "block-p7",
          source: "Contact our support team for any questions or concerns.",
          source_coded: `Contact our ${mo}support team${mc} for any questions or concerns.`,
          source_spans: [
            { span_type: "opening", type: "b", id: "b3", data: "<b>" },
            { span_type: "closing", type: "b", id: "b3", data: "</b>" },
          ],
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: true,
          properties: { element: "p" },
        },
        {
          id: "block-link", source: "Visit our community forum for tips and discussions",
          targets: {} as Record<string, string>, targets_coded: {} as Record<string, string>,
          translatable: true, has_spans: false,
          properties: { element: "a" },
        },
      ];

      // Pre-seed Swedish translations for reference locale demo
      blocks[0].targets["sv"] = "CloudSync Pro Hjälpcenter";
      blocks[1].targets["sv"] = "Välkommen till CloudSync Pros dokumentation och supportportal.";
      blocks[2].targets["sv"] = "Lär dig hur du synkroniserar dina filer säkert på alla enheter.";
      blocks[3].targets["sv"] = "Kom igång";
      blocks[4].targets["sv"] = "Ladda ner och installera CloudSync Pro-applikationen på din enhet.";

      // Store blocks for our file
      if (!(window as any).__projectFiles) (window as any).__projectFiles = {};
      if (!(window as any).__projectFiles[projectId]) (window as any).__projectFiles[projectId] = {};
      (window as any).__projectFiles[projectId]["help-center.html"] = blocks;

      // Patch GetItemBlocks
      const origGetItemBlocks = backend[IDS.GetItemBlocks];
      backend[IDS.GetItemBlocks] = (pid: string, fileName: string) => {
        const customBlocks = (window as any).__projectFiles?.[pid]?.[fileName];
        if (customBlocks) return customBlocks.map((b: any) => ({
          ...b,
          targets: { ...b.targets },
          targets_coded: b.targets_coded ? { ...b.targets_coded } : undefined,
          source_spans: b.source_spans ? [...b.source_spans] : undefined,
        }));
        return origGetItemBlocks(pid, fileName);
      };

      // Patch UpdateBlockTarget
      const origUpdateTarget = backend[IDS.UpdateBlockTarget];
      backend[IDS.UpdateBlockTarget] = (req: any) => {
        const itemName = req.item_name || req.file_name;
        const customBlocks = (window as any).__projectFiles?.[req.project_id]?.[itemName];
        if (customBlocks) {
          const block = customBlocks.find((b: any) => b.id === req.block_id);
          if (block) block.targets[req.target_locale] = req.text;
          return;
        }
        return origUpdateTarget(req);
      };

      // Patch UpdateBlockTargetCoded
      const origUpdateCoded = backend[IDS.UpdateBlockTargetCoded];
      backend[IDS.UpdateBlockTargetCoded] = (req: any) => {
        const itemName = req.item_name || req.file_name;
        const customBlocks = (window as any).__projectFiles?.[req.project_id]?.[itemName];
        if (customBlocks) {
          const block = customBlocks.find((b: any) => b.id === req.block_id);
          if (block) {
            const plain = req.coded_text.replace(/[\uE001-\uE003]/g, "");
            block.targets[req.target_locale] = plain;
            if (!block.targets_coded) block.targets_coded = {};
            block.targets_coded[req.target_locale] = req.coded_text;
          }
          return;
        }
        if (origUpdateCoded) return origUpdateCoded(req);
      };

      // Patch RenderDocumentPreview with styled HTML
      // NOTE: This is a mock-only preview string for e2e recordings.
      // The content is hardcoded test data, not user-supplied input.
      backend[IDS.RenderDocumentPreview] = (_projectID: string, _itemName: string, targetLocale: string) => {
        const b = (window as any).__projectFiles?.[projectId]?.["help-center.html"] || [];
        const gc = (block: any) => block.targets[targetLocale] || block.source;
        const bodyParts = [
          '<h1><kat-block id="block-h1">' + gc(b[0]) + '</kat-block></h1>',
          '<p><kat-block id="block-p1">' + gc(b[1]) + '</kat-block></p>',
          '<p><kat-block id="block-p2">Learn how to <b>sync</b> your files <i>securely</i> across all your devices.</kat-block></p>',
          '<h2><kat-block id="block-h2a">' + gc(b[3]) + '</kat-block></h2>',
          '<p><kat-block id="block-p3">Download and install the <a href="#">CloudSync Pro</a> application on your device.</kat-block></p>',
          '<p><kat-block id="block-p4">Create your account and configure your <code>synchronization</code> preferences.</kat-block></p>',
          '<h2><kat-block id="block-h2b">' + gc(b[6]) + '</kat-block></h2>',
          '<p><kat-block id="block-p5">All data is encrypted with <b>AES-256</b> encryption during transfer.<br/>Your privacy is our priority.</kat-block></p>',
          '<p><kat-block id="block-p6">Your files are stored securely in our <a href="/security">certified data centers</a>.</kat-block></p>',
          '<p><kat-block id="block-p7">Contact our <b>support team</b> for any questions or concerns.</kat-block></p>',
          '<p><a href="#"><kat-block id="block-link">' + gc(b[10]) + '</kat-block></a></p>',
        ];
        const styles = [
          'body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; padding: 32px; line-height: 1.7; color: #1a1a1a; max-width: 640px; }',
          'h1 { font-size: 28px; margin-bottom: 16px; color: #111; font-weight: 700; }',
          'h2 { font-size: 20px; margin-top: 28px; margin-bottom: 12px; color: #333; font-weight: 600; }',
          'p { margin-bottom: 14px; color: #444; }',
          'a { color: #2563eb; text-decoration: underline; text-underline-offset: 2px; }',
          'b, strong { font-weight: 600; }',
          'i, em { font-style: italic; }',
          'code { font-family: "SF Mono", monospace; font-size: 0.9em; background: rgba(0,0,0,0.06); padding: 1px 4px; border-radius: 3px; }',
          'kat-block { cursor: pointer; border-radius: 3px; display: inline; transition: background 0.15s, outline 0.15s; }',
          'kat-block:hover { background-color: rgba(59,130,246,0.12); }',
          'kat-block.kat-selected { background-color: rgba(59,130,246,0.18); outline: 2px solid #3b82f6; outline-offset: 2px; }',
        ].join('\n');
        const script = [
          'document.querySelectorAll("kat-block").forEach(function(el) {',
          '  el.addEventListener("click", function() { window.parent.postMessage({ type: "kat-block-click", blockId: el.id }, "*"); });',
          '});',
          'window.addEventListener("message", function(e) {',
          '  if (e.data && e.data.type === "kat-select-block") {',
          '    var prev = document.querySelector(".kat-selected"); if (prev) prev.classList.remove("kat-selected");',
          '    var next = document.getElementById(e.data.blockId); if (next) next.classList.add("kat-selected");',
          '  }',
          '  if (e.data && e.data.type === "kat-update-block") {',
          '    var el2 = document.getElementById(e.data.blockId); if (el2) el2.textContent = e.data.html;',
          '  }',
          '});',
          'window.parent.postMessage({ type: "kat-iframe-ready" }, "*");',
        ].join('\n');
        return '<!DOCTYPE html><html><head><style>' + styles + '</style></head><body>'
          + bodyParts.join('\n') + '<script>' + script + '</' + 'script></body></html>';
      };

      // Seed TM entries for Norwegian
      byName.AddTMEntry(projectId, "Welcome to the CloudSync Pro documentation and support portal.", "Velkommen til CloudSync Pro-dokumentasjonen og støtteportalen.", "en", "nb");
      byName.AddTMEntry(projectId, "Getting Started", "Kom i gang", "en", "nb");
      byName.AddTMEntry(projectId, "Security & Privacy", "Sikkerhet og personvern", "en", "nb");

      // Seed terminology concepts
      byName.AddConcept({
        project_id: projectId, domain: "Security",
        definition: "Converting data to prevent unauthorized access",
        terms: [
          { text: "encryption", locale: "en", status: "preferred" },
          { text: "kryptering", locale: "nb", status: "preferred" },
        ],
      });
      byName.AddConcept({
        project_id: projectId, domain: "Technology",
        definition: "The process of keeping files in sync across devices",
        terms: [
          { text: "synchronization", locale: "en", status: "preferred" },
          { text: "synkronisering", locale: "nb", status: "preferred" },
        ],
      });
    }, { mo, mc, mp });

    // Refresh to show the file
    await page.getByTestId("nav-settings").click();
    await page.waitForTimeout(100);
    await page.getByTestId("nav-translate").click();
    await page.waitForTimeout(200);
    await humanClick(page, page.getByText("Help Center").first());
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Open the HTML file in editor (grid view)
    await expect(page.getByTestId("open-file-help-center.html")).toBeVisible({ timeout: 5000 });
    await humanClick(page, page.getByTestId("open-file-help-center.html"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // === Switch to Visual mode ===
    await humanClick(page, page.getByTestId("layout-visual"));
    await expect(page.getByTestId("visual-editor-layout")).toBeVisible({ timeout: 5000 });
    await pause(page, 1000);

    // === Navigate blocks with keyboard (j/k) ===
    await page.keyboard.press("j"); // → block 1
    await pause(page, 500);
    await page.keyboard.press("j"); // → block 2
    await pause(page, 500);
    await page.keyboard.press("j"); // → block 3 (h2: Getting Started)
    await pause(page, 500);
    await page.keyboard.press("k"); // → back to block 2
    await pause(page, 500);

    // === Block 2 has inline codes (<b>sync</b>, <i>securely</i>) ===
    // Enter editing mode
    await page.keyboard.press("Enter");
    await pause(page, 600);

    // Type Norwegian translation in Lexical editor
    // Focus the contenteditable and type via execCommand (same pattern as inline-codes.spec.ts)
    await page.evaluate(() => {
      const el = document.querySelector('[contenteditable="true"]');
      if (el) {
        (el as HTMLElement).focus();
        document.execCommand("insertText", false, "Lær hvordan du synkroniserer filene dine sikkert.");
      }
    });
    await pause(page, 800);

    // Save with Ctrl+Enter
    await page.evaluate(() => {
      const el = document.querySelector('[contenteditable="true"]');
      if (el) {
        el.dispatchEvent(new KeyboardEvent("keydown", {
          key: "Enter", code: "Enter", keyCode: 13, ctrlKey: true, bubbles: true, cancelable: true,
        }));
      }
    });
    await pause(page, 600);

    // === Navigate to a block with TM match (block 3: "Getting Started") ===
    // We should be on block 3 now after save+advance
    await pause(page, 600);

    // TM matches should appear for "Getting Started" → "Kom i gang"
    // Toggle TM section if not expanded
    const tmToggle = page.getByTestId("tm-toggle");
    if (await tmToggle.isVisible()) {
      await humanClick(page, tmToggle);
      await pause(page, 600);
    }

    // Apply TM match
    const tmApply = page.getByTestId("tm-apply-0");
    if (await tmApply.isVisible()) {
      await humanClick(page, tmApply);
      await pause(page, 600);
    }

    // === Toggle preview content mode: Source → Target → Pseudo → Source ===
    // Click "Target" tab in the preview mode switcher
    await page.evaluate(() => {
      const tabs = document.querySelectorAll('[data-testid="visual-editor-layout"] [role="tab"]');
      // Find the top-right tabs (Source/Target/Pseudo)
      const targetTab = Array.from(tabs).find(t => t.textContent?.trim() === "Target");
      if (targetTab) (targetTab as HTMLElement).click();
    });
    await pause(page, 700);

    await page.evaluate(() => {
      const tabs = document.querySelectorAll('[data-testid="visual-editor-layout"] [role="tab"]');
      const pseudoTab = Array.from(tabs).find(t => t.textContent?.trim() === "Pseudo");
      if (pseudoTab) (pseudoTab as HTMLElement).click();
    });
    await pause(page, 700);

    await page.evaluate(() => {
      const tabs = document.querySelectorAll('[data-testid="visual-editor-layout"] [role="tab"]');
      const sourceTab = Array.from(tabs).find(t => t.textContent?.trim() === "Source");
      if (sourceTab) (sourceTab as HTMLElement).click();
    });
    await pause(page, 500);

    // === Reference locale picker ===
    await humanClick(page, page.getByTestId("ref-locale-toggle"));
    await pause(page, 400);
    await humanClick(page, page.getByTestId("ref-locale-sv"));
    await pause(page, 800);

    // Close ref picker by clicking toggle again
    await humanClick(page, page.getByTestId("ref-locale-toggle"));
    await pause(page, 400);

    // === Navigate to see term matches ===
    // Navigate to block 5 (has "synchronization" term match)
    await page.keyboard.press("j"); // → block 4
    await pause(page, 400);
    await page.keyboard.press("j"); // → block 5 (has <code>synchronization</code>)
    await pause(page, 800);

    // Term sidebar should appear if terms are matched
    await pause(page, 500);

    // === Switch to Review mode, approve a block ===
    // Click the "Review" tab on the editor card
    await page.evaluate(() => {
      const cardTabs = document.querySelectorAll('[data-testid="visual-editor-card"] [role="tab"]');
      const reviewTab = Array.from(cardTabs).find(t => t.textContent?.trim() === "Review");
      if (reviewTab) (reviewTab as HTMLElement).click();
    });
    await pause(page, 500);

    // Approve the block
    const approveBtn = page.getByTestId("approve-btn");
    if (await approveBtn.isVisible()) {
      await humanClick(page, approveBtn);
      await pause(page, 600);
    }

    // === Switch back to Translate mode ===
    await page.evaluate(() => {
      const cardTabs = document.querySelectorAll('[data-testid="visual-editor-card"] [role="tab"]');
      const translateTab = Array.from(cardTabs).find(t => t.textContent?.trim() === "Translate");
      if (translateTab) (translateTab as HTMLElement).click();
    });
    await pause(page, 400);

    // === Navigate back up to show the full visual editor in its glory ===
    await page.keyboard.press("k");
    await pause(page, 400);
    await page.keyboard.press("k");
    await pause(page, 400);
    await page.keyboard.press("k");
    await pause(page, 2000);
  });

  // ── 11. Flow Editor ───────────────────────────────────────────────────────────
  // Browse automation flows (no projects created)
  test(`record flow editor [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — Flow Editor", theme);
    await pause(page, 600);

    // Navigate to Flows view
    await expect(page.getByTestId("nav-flows")).toBeVisible();
    await humanClick(page, page.getByTestId("nav-flows"));
    await expect(page.getByTestId("flow-list")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Select AI Translate flow
    await expect(page.getByTestId("flow-item-ai-translate")).toBeVisible();
    await humanClick(page, page.getByTestId("flow-item-ai-translate"));
    await pause(page, 600);

    // Verify flow nodes are visible
    await expect(page.getByTestId("flow-node-reader")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Click through the flow nodes to show the pipeline
    await humanClick(page, page.getByTestId("flow-node-reader"), true);
    await pause(page, 500);

    await humanClick(page, page.getByTestId("flow-node-ai-translate"), true);
    await pause(page, 500);

    await humanClick(page, page.getByTestId("flow-node-writer"), true);
    await pause(page, 600);

    // Switch to pseudo-translate flow
    await humanClick(page, page.getByTestId("flow-item-pseudo-translate"));
    await expect(page.getByTestId("flow-node-pseudo-translate")).toBeVisible({ timeout: 5000 });
    await pause(page, 800);

    // Click through pseudo-translate nodes
    await humanClick(page, page.getByTestId("flow-node-reader"), true);
    await pause(page, 400);
    await humanClick(page, page.getByTestId("flow-node-pseudo-translate"), true);
    await pause(page, 400);
    await humanClick(page, page.getByTestId("flow-node-writer"), true);
    await pause(page, 1000);
  });

  // ── 12. Account and Authentication ────────────────────────────────────────────
  // Add more AI providers (no projects created)
  test(`record account and authentication [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — Settings & Providers", theme);
    await pause(page, 600);

    // Navigate to Settings
    await expect(page.getByTestId("nav-settings")).toBeVisible();
    await humanClick(page, page.getByTestId("nav-settings"));
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Switch to AI Providers tab
    await expect(page.getByTestId("settings-tab-ai-providers")).toBeVisible();
    await humanClick(page, page.getByTestId("settings-tab-ai-providers"));
    await expect(page.getByTestId("settings-ai-providers")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // In server mode without a real bowrain-server, AI providers are unavailable.
    const addBtn12 = page.getByTestId("add-provider-btn");
    if (await addBtn12.isVisible({ timeout: 1000 }).catch(() => false)) {
      // Add first provider — Anthropic
      await humanClick(page, addBtn12);
      await expect(page.getByTestId("provider-name")).toBeVisible({ timeout: 5000 });
      await pause(page, 300);

      await humanType(page, page.getByTestId("provider-name"), "Anthropic Claude");
      await pause(page, 200);

      await humanClick(page, page.getByTestId("provider-type"));
      await page.getByRole("option", { name: "Anthropic" }).click();
      await pause(page, 300);

      await humanType(page, page.getByTestId("provider-api-key"), "sk-ant-api03-xxxx");
      await pause(page, 200);

      await humanType(page, page.getByTestId("provider-model"), "claude-sonnet-4-20250514");
      await pause(page, 300);

      await humanClick(page, page.getByTestId("provider-save-btn"));
      await pause(page, 1000);
      // On Linux CI, API key keyring save fails but the provider record IS created
      // (upsert happens before keyring save). The dialog stays open with an error —
      // dismiss it. Escape is a no-op if the dialog already closed (mock mode).
      await page.keyboard.press("Escape");
      await pause(page, 300);

      // Add second provider — OpenAI
      await humanClick(page, page.getByTestId("add-provider-btn"));
      await expect(page.getByTestId("provider-name")).toBeVisible({ timeout: 5000 });
      await pause(page, 300);

      await humanType(page, page.getByTestId("provider-name"), "OpenAI GPT");
      await pause(page, 200);

      await humanClick(page, page.getByTestId("provider-type"));
      await page.getByRole("option", { name: "OpenAI", exact: true }).click();
      await pause(page, 300);

      await humanType(page, page.getByTestId("provider-api-key"), "sk-proj-xxxx");
      await pause(page, 200);

      await humanType(page, page.getByTestId("provider-model"), "gpt-4o");
      await pause(page, 300);

      await humanClick(page, page.getByTestId("provider-save-btn"));
      await pause(page, 1000);
      await page.keyboard.press("Escape");
      await pause(page, 300);

      // Refresh the provider list — on CI the keyring error prevents the auto-refresh
      // that normally happens on successful save. Navigate away and back to re-fetch.
      await humanClick(page, page.getByTestId("nav-translate"));
      await pause(page, 200);
      await humanClick(page, page.getByTestId("nav-settings"));
      await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 5000 });
      await pause(page, 200);
      await humanClick(page, page.getByTestId("settings-tab-ai-providers"));
      await expect(page.getByTestId("settings-ai-providers")).toBeVisible({ timeout: 5000 });
      await pause(page, 300);

      // Both providers should be visible in the refreshed list
      await expect(page.getByText("Anthropic Claude").first()).toBeVisible({ timeout: 5000 });
      await expect(page.getByText("OpenAI GPT").first()).toBeVisible({ timeout: 5000 });
    }
    await pause(page, 800);
  });

  // ── 13. Workspace Project Management ──────────────────────────────────────────
  // Add a couple more projects, manage workspace (creates "Mobile App v2.0" + "API Documentation")
  test(`record workspace project management [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — Project Management", theme);
    await pause(page, 600);

    // Create first NEW project (workspace already has projects from the narrative)
    await expect(page.getByTestId("new-project-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();

    await humanType(page, page.getByTestId("project-name-input"), "Mobile App v2.0");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["es"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["es"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 300);

    // Add files to the new project
    const mobileProj = await findProject(page, "Mobile App v2.0");
    if (mobileProj) {
      await callBackend(page, "AddItems", mobileProj.id, useServerMode
        ? [fixture("strings.json"), fixture("config.yaml")]
        : ["/app/strings.json", "/app/config.yaml"]);
    }

    // Go back and create second project
    await humanClick(page, page.getByTestId("back-to-projects"));
    await pause(page, 400);

    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();

    await humanType(page, page.getByTestId("project-name-input"), "API Documentation");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["ko"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["ko"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 300);

    // Navigate back to dashboard to see all projects
    await humanClick(page, page.getByTestId("back-to-projects"));
    await expect(page.getByText("Mobile App v2.0").first()).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("API Documentation").first()).toBeVisible();
    await pause(page, 800);

    // Open an existing project to show management
    await humanClick(page, page.getByText("Website Redesign").first());
    await expect(page.getByTestId("open-file-index.html")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Open a file to show quick access
    await humanClick(page, page.getByTestId("open-file-index.html"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Navigate back to project list
    await humanClick(page, page.getByTestId("back-to-project"));
    await pause(page, 400);

    await humanClick(page, page.getByTestId("back-to-projects"));
    await pause(page, 400);

    // Open the second new project
    await humanClick(page, page.getByText("Mobile App v2.0").first());
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 1000);
  });

  // ── 14. Workspace Switcher ────────────────────────────────────────────────────
  // Navigate the rich accumulated workspace (no projects created)
  test(`record workspace switcher [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — Workspace", theme);
    await pause(page, 600);

    // The workspace already has 11 projects from previous tests.
    // Navigate between a few of them to demonstrate workspace switching.
    await humanClick(page, page.getByText("Website Redesign").first());
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    await humanClick(page, page.getByTestId("back-to-projects"));
    await pause(page, 300);

    await humanClick(page, page.getByText("Company Website").first());
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Navigate through sidebar sections
    await humanClick(page, page.getByTestId("nav-termbase"));
    await pause(page, 500);
    await humanClick(page, page.getByTestId("nav-memory"));
    await pause(page, 500);
    await humanClick(page, page.getByTestId("nav-flows"));
    await expect(page.getByTestId("flow-list")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);
    await humanClick(page, page.getByTestId("nav-connectors"));
    await pause(page, 500);
    await humanClick(page, page.getByTestId("nav-settings"));
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Back to project list — all projects visible
    await humanClick(page, page.getByTestId("nav-translate"));
    await pause(page, 1200);
  });

  } // end for (theme)
});
