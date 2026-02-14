import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";
import { injectCursor, humanClick, humanClickNative, humanType, humanTypeNative, moveCursorTo, moveCursorToElement } from "./cursor-helper";
import { setMultiLocales, setMultiLocalesHuman, expectLocaleChips } from "./locale-helper";
import { injectWindowChrome } from "./window-chrome";

// Skip recording tests in CI - they use human-speed typing and exceed CI timeouts
// Run locally with: npx playwright test --config=playwright.recordings.config.ts
// Or in CI with FORCE_RECORDINGS=true (e.g., Screenshots & Recordings workflow)
const isCI = (process.env.CI === "true" || process.env.CI === "1") && process.env.FORCE_RECORDINGS !== "true";

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

/** Setup helper - injects mock backend, cursor, and window chrome */
async function setupRecording(page: any, title: string = "Bowrain", theme: "glass" | "light" | "aurora" = "glass") {
  await injectMockBackend(page);
  await page.goto("/");
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

// Use conditional describe to skip entire suite in CI
const describeOrSkip = isCI ? test.describe.skip : test.describe;

const themes = ["glass", "light", "aurora"] as const;

describeOrSkip("Video Recordings", () => {
  // Video settings are configured in playwright.recordings.config.ts
  // Videos are saved to test-results/ - use `npm run recordings:copy` to copy to docs
  // Skipped in CI due to human-speed typing exceeding timeouts

  for (const theme of themes) {
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

    // Add files via mock backend
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      if (projects[0]) {
        await backend.AddFiles(projects[0].id, [
          "/src/index.html",
          "/src/strings.json",
          "/content/about.md",
        ]);
      }
    });

    // Navigate away and back to refresh file list
    await humanClick(page, page.getByTestId("nav-settings"));
    await pause(page, 300);
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Website Redesign")).toBeVisible({ timeout: 5000 });
    await pause(page, 300);

    // Click into project to see files
    await humanClick(page, page.getByText("Website Redesign").first());
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("open-file-index.html")).toBeVisible({ timeout: 5000 });
    await pause(page, 1500);
  });

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
    await page.evaluate(async () => {
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

      // Patch GetFileBlocks
      const origGetFileBlocks = backend[IDS.GetFileBlocks];
      backend[IDS.GetFileBlocks] = (pid: string, fileName: string) => {
        const customBlocks = (window as any).__projectFiles?.[pid]?.[fileName];
        if (customBlocks) return customBlocks.map((b: any) => ({ ...b, targets: { ...b.targets } }));
        return origGetFileBlocks(pid, fileName);
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

    // Block 4: p - "We believe in quality..."
    await page.getByTestId("target-cell-4").dblclick();
    await expect(page.getByTestId("edit-target-4")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-4"), "Vi tror på kvalitet, integritet og kundesuksess.");
    await pause(page, 200);
    await page.keyboard.press("Tab");
    await pause(page, 200);

    // Block 5: p - "Every project starts..."
    await page.getByTestId("target-cell-5").dblclick();
    await expect(page.getByTestId("edit-target-5")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-5"), "Hvert prosjekt starter med å forstå dine unike behov.");
    await pause(page, 200);
    await page.keyboard.press("Tab");
    await pause(page, 400);

    // === PHASE 4: Use pseudo-translate for remaining blocks ===
    await humanClick(page, page.getByTestId("pseudo-btn"));
    await pause(page, 4000); // Hold for 4 seconds to show completed state
  });

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

    // Add file via mock
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      if (projects[0]) {
        await backend.AddFiles(projects[0].id, ["/content/homepage.html"]);
      }
    });

    // Refresh to show file
    await humanClick(page, page.getByTestId("nav-settings"));
    await pause(page, 200);
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Marketing Site")).toBeVisible({ timeout: 5000 });
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

    // Seed TM entries via mock
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddTMEntry(pid, "Hello World", "Bonjour le monde", "en", "fr");
        backend.AddTMEntry(pid, "Welcome to our application", "Bienvenue dans notre application", "en", "fr");
        backend.AddTMEntry(pid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");
        backend.AddTMEntry(pid, "Settings", "Paramètres", "en", "fr");
        backend.AddTMEntry(pid, "Save changes", "Enregistrer les modifications", "en", "fr");
      }
    });

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

    // Add file via mock
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      if (projects[0]) {
        await backend.AddFiles(projects[0].id, ["/landing-page.html"]);
      }
    });

    // Refresh to show file
    await humanClick(page, page.getByTestId("nav-settings"));
    await pause(page, 200);
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Product Launch")).toBeVisible({ timeout: 5000 });
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

    // Seed TM entries and add file via mock backend
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      const pid = projects[0]?.id;
      if (!pid) return;

      // Add TM entries matching the generated blocks
      backend.AddTMEntry(pid, "Hello from landing.html", "Bonjour depuis landing.html", "en", "fr");
      backend.AddTMEntry(pid, "Welcome to our application", "Bienvenue dans notre application", "en", "fr");
      backend.AddTMEntry(pid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");

      await backend.AddFiles(pid, ["/content/landing.html"]);
    });

    // Refresh to show file
    await humanClick(page, page.getByTestId("nav-settings"));
    await pause(page, 200);
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Website Translation")).toBeVisible({ timeout: 5000 });
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

    // Add more concepts via mock backend for demo
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddConcept({
          project_id: pid, domain: "Security",
          definition: "Converting data to prevent unauthorized access",
          terms: [
            { text: "encryption", locale: "en", status: "preferred" },
            { text: "chiffrement", locale: "fr", status: "preferred" },
          ],
        });
        backend.AddConcept({
          project_id: pid, domain: "UI",
          definition: "Visual panel for settings",
          terms: [
            { text: "dashboard", locale: "en", status: "approved" },
            { text: "tableau de bord", locale: "fr", status: "approved" },
          ],
        });
        backend.AddConcept({
          project_id: pid, domain: "Data",
          definition: "Structured data storage",
          terms: [
            { text: "database", locale: "en", status: "preferred" },
            { text: "base de données", locale: "fr", status: "preferred" },
          ],
        });
      }
    });

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
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      const pid = projects[0]?.id;
      if (!pid) return;

      // TM entries
      backend.AddTMEntry(pid, "Hello from app.html", "Bonjour depuis app.html", "en", "fr");
      backend.AddTMEntry(pid, "Welcome to our application", "Bienvenue dans notre application", "en", "fr");
      backend.AddTMEntry(pid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");

      // Terminology
      backend.AddConcept({
        project_id: pid, domain: "UI",
        terms: [
          { text: "application", locale: "en", status: "preferred" },
          { text: "application", locale: "fr", status: "preferred" },
        ],
      });
      backend.AddConcept({
        project_id: pid, domain: "Navigation",
        terms: [
          { text: "continue", locale: "en", status: "approved" },
          { text: "continuer", locale: "fr", status: "approved" },
        ],
      });

      await backend.AddFiles(pid, ["/src/app.html"]);
    });

    // Refresh to show file
    await humanClick(page, page.getByTestId("nav-settings"));
    await pause(page, 200);
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Web Application")).toBeVisible({ timeout: 5000 });
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

  test(`record workspace switcher [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — Workspace Navigation", theme);
    await pause(page, 600);

    // Show the sidebar with workspace name "Personal"
    // Create a project first so the dashboard has content
    await expect(page.getByTestId("new-project-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();

    await humanType(page, page.getByTestId("project-name-input"), "Marketing Site");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["fr", "de"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["fr", "de"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Go back to dashboard
    await humanClick(page, page.getByTestId("back-to-projects"));
    await pause(page, 400);

    // Create a second project
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();
    await humanType(page, page.getByTestId("project-name-input"), "Mobile App v2");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["es", "ja"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["es", "ja"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Navigate back to see both projects
    await humanClick(page, page.getByTestId("back-to-projects"));
    await pause(page, 600);

    // === Demonstrate sidebar navigation ===

    // Navigate to Termbase
    await humanClick(page, page.getByTestId("nav-termbase"));
    await pause(page, 500);

    // Navigate to Memory
    await humanClick(page, page.getByTestId("nav-memory"));
    await pause(page, 500);

    // Navigate to Flows
    await humanClick(page, page.getByTestId("nav-flows"));
    await expect(page.getByTestId("flow-list")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Navigate to Connectors
    await humanClick(page, page.getByTestId("nav-connectors"));
    await pause(page, 500);

    // Navigate to Settings
    await humanClick(page, page.getByTestId("nav-settings"));
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Back to Translate (project list)
    await humanClick(page, page.getByTestId("nav-translate"));
    await expect(page.getByText("Marketing Site")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("Mobile App v2")).toBeVisible();
    await pause(page, 1200);
  });

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

    // Add first provider — Anthropic
    await expect(page.getByTestId("add-provider-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("add-provider-btn"));
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
    await expect(page.getByText("Anthropic Claude")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Add second provider — OpenAI
    await humanClick(page, page.getByTestId("add-provider-btn"));
    await expect(page.getByTestId("provider-name")).toBeVisible({ timeout: 5000 });
    await pause(page, 300);

    await humanType(page, page.getByTestId("provider-name"), "OpenAI GPT");
    await pause(page, 200);

    await humanClick(page, page.getByTestId("provider-type"));
    await page.getByRole("option", { name: "OpenAI" }).click();
    await pause(page, 300);

    await humanType(page, page.getByTestId("provider-api-key"), "sk-proj-xxxx");
    await pause(page, 200);

    await humanType(page, page.getByTestId("provider-model"), "gpt-4o");
    await pause(page, 300);

    await humanClick(page, page.getByTestId("provider-save-btn"));
    await expect(page.getByText("OpenAI GPT")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Navigate back to Translate to show the workspace is ready
    await humanClick(page, page.getByTestId("nav-translate"));
    await pause(page, 800);
  });

  test(`record workspace project management [${theme}]`, async ({ page }) => {
    await setupRecording(page, "Bowrain — Project Management", theme);
    await pause(page, 600);

    // Create first project
    await expect(page.getByTestId("new-project-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("new-project-btn"));
    await expect(page.getByTestId("project-name-input")).toBeVisible();

    await humanType(page, page.getByTestId("project-name-input"), "Website Redesign");
    await pause(page, 200);
    await setMultiLocalesHuman(page, "target-langs-input", ["fr", "de", "ja"], humanType);
    await expectLocaleChips(page, "target-langs-input", ["fr", "de", "ja"]);
    await pause(page, 300);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 300);

    // Add files
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      if (projects[0]) {
        await backend.AddFiles(projects[0].id, [
          "/src/index.html",
          "/src/strings.json",
          "/content/about.md",
        ]);
      }
    });

    // Refresh to show files
    await page.getByTestId("nav-settings").click();
    await page.waitForTimeout(100);
    await page.getByTestId("nav-translate").click();
    await page.waitForTimeout(200);
    await humanClick(page, page.getByText("Website Redesign").first());
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("open-file-index.html")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Go back and create second project
    await humanClick(page, page.getByTestId("back-to-projects"));
    await pause(page, 400);

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

    // Add files to second project
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      const p = projects[projects.length - 1];
      if (p) {
        await backend.AddFiles(p.id, ["/app/strings.json", "/app/config.yaml"]);
      }
    });

    // Go back and create third project
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

    // Navigate back to dashboard to see all 3 projects
    await humanClick(page, page.getByTestId("back-to-projects"));
    await expect(page.getByText("Website Redesign")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("Mobile App v2.0")).toBeVisible();
    await expect(page.getByText("API Documentation")).toBeVisible();
    await pause(page, 800);

    // Open first project to show files
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

    // Open the second project
    await humanClick(page, page.getByText("Mobile App v2.0").first());
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 1000);
  });

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

    // Add a provider
    await expect(page.getByTestId("add-provider-btn")).toBeVisible();
    await humanClick(page, page.getByTestId("add-provider-btn"));
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
    await expect(page.getByText("Anthropic Claude")).toBeVisible({ timeout: 5000 });
    await pause(page, 1000);
  });
  } // end for (theme)
});
