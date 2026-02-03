import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";
import { injectCursor, humanClick, humanType, moveCursorTo } from "./cursor-helper";
import { injectWindowChrome } from "./window-chrome";

/** Setup helper - injects mock backend, cursor, and window chrome */
async function setupRecording(page: any, title: string = "Bowrain") {
  await injectMockBackend(page);
  await page.goto("/");
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

test.describe("Video Recordings", () => {
  // Video settings are configured in playwright.recordings.config.ts
  // Videos are saved to test-results/ - use `npm run recordings:copy` to copy to docs

  test("record create project flow", async ({ page }) => {
    await setupRecording(page, "Bowrain — New Project");
    await pause(page, 600);

    // Click new project button with smooth cursor movement
    await humanClick(page, page.getByTestId("new-project-btn"));
    await pause(page, 500);

    // Fill in project details with human-like typing
    await humanType(page, page.getByTestId("project-name-input"), "Website Redesign");
    await pause(page, 400);

    await humanType(page, page.getByTestId("target-langs-input"), "fr, de, ja");
    await pause(page, 500);

    // Create project
    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();
    await pause(page, 600);

    // Add some files via mock
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      const p = projects[0];
      if (p) {
        await backend.AddFiles(p.id, [
          "/src/index.html",
          "/src/strings.json",
          "/content/about.md",
        ]);
      }
    });

    // Refresh view
    await page.locator("nav button", { hasText: "Settings" }).click();
    await pause(page, 200);
    await page.locator("nav button", { hasText: "Projects" }).click();
    await pause(page, 300);
    await page.getByText("Website Redesign").first().click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 800);
  });

  test("record translation editor workflow", async ({ page }) => {
    // Hero demo: Translating a realistic HTML page to Norwegian
    // Shows: grid view → translate blocks → toggle to split view → translate more
    await setupRecording(page, "Bowrain — Translation Editor");

    // Create project with Norwegian as target
    await page.getByTestId("new-project-btn").click();
    await page.getByTestId("project-name-input").fill("Company Website");
    await page.getByTestId("target-langs-input").fill("nb-NO");
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
    await page.locator("nav button", { hasText: "Settings" }).click();
    await page.waitForTimeout(100);
    await page.locator("nav button", { hasText: "Projects" }).click();
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

    // === PHASE 1: Translate first 4 blocks in grid view ===
    // Double-click on target cell to open editor, type translation, Tab to save and move to next

    // Block 0: h1 - "About Our Company"
    await page.getByTestId("target-cell-0").dblclick();
    await expect(page.getByTestId("edit-target-0")).toBeVisible();
    await moveCursorTo(page, 900, 200, 200); // Move cursor to target area
    await humanType(page, page.getByTestId("edit-target-0"), "Om selskapet vårt");
    await pause(page, 300);
    await page.keyboard.press("Tab"); // Save and close
    await pause(page, 300);

    // Block 1: p - "We are a leading technology company..."
    await page.getByTestId("target-cell-1").dblclick();
    await expect(page.getByTestId("edit-target-1")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-1"), "Vi er et ledende teknologiselskap grunnlagt i 2015.");
    await pause(page, 300);
    await page.keyboard.press("Tab");
    await pause(page, 300);

    // Block 2: p - "Our mission is to deliver..."
    await page.getByTestId("target-cell-2").dblclick();
    await expect(page.getByTestId("edit-target-2")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-2"), "Vår misjon er å levere innovative løsninger som transformerer bedrifter.");
    await pause(page, 300);
    await page.keyboard.press("Tab");
    await pause(page, 300);

    // Block 3: h2 - "Our Values"
    await page.getByTestId("target-cell-3").dblclick();
    await expect(page.getByTestId("edit-target-3")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-3"), "Våre verdier");
    await pause(page, 300);
    await page.keyboard.press("Tab");
    await pause(page, 400);

    // === PHASE 2: Toggle to split view to see translations in context ===
    await humanClick(page, page.getByTestId("layout-split-v"));
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
    await pause(page, 800); // Let user see the preview with translated content

    // === PHASE 3: Continue translating in split view ===

    // Scroll to show more blocks
    await page.getByTestId("block-row-4").scrollIntoViewIfNeeded();
    await pause(page, 300);

    // Block 4: p - "We believe in quality..."
    await page.getByTestId("target-cell-4").dblclick();
    await expect(page.getByTestId("edit-target-4")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-4"), "Vi tror på kvalitet, integritet og kundesuksess.");
    await pause(page, 300);
    await page.keyboard.press("Tab");
    await pause(page, 300);

    // Block 5: p - "Every project starts..."
    await page.getByTestId("target-cell-5").dblclick();
    await expect(page.getByTestId("edit-target-5")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-5"), "Hvert prosjekt starter med å forstå dine unike behov.");
    await pause(page, 300);
    await page.keyboard.press("Tab");
    await pause(page, 300);

    // Block 6: h2 - "Meet the Team"
    await page.getByTestId("target-cell-6").dblclick();
    await expect(page.getByTestId("edit-target-6")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-6"), "Møt teamet");
    await pause(page, 300);
    await page.keyboard.press("Tab");
    await pause(page, 300);

    // Block 7: p - "Our talented team..."
    await page.getByTestId("target-cell-7").dblclick();
    await expect(page.getByTestId("edit-target-7")).toBeVisible();
    await humanType(page, page.getByTestId("edit-target-7"), "Vårt talentfulle team har flere tiår med samlet erfaring.");
    await pause(page, 500);

    // Final pause to show progress
    await pause(page, 800);
  });

  test("record focus view editing", async ({ page }) => {
    await setupRecording(page, "Bowrain — Focus View");

    // Quick setup
    await page.getByTestId("new-project-btn").click();
    await page.getByTestId("project-name-input").fill("Website Redesign");
    await page.getByTestId("target-langs-input").fill("fr");
    await page.getByTestId("create-project-submit").click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();

    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      if (projects.length > 0) {
        await backend.AddFiles(projects[0].id, ["/src/index.html"]);
      }
    });

    await page.locator("nav button", { hasText: "Settings" }).click();
    await page.waitForTimeout(100);
    await page.locator("nav button", { hasText: "Projects" }).click();
    await page.waitForTimeout(200);
    await page.getByText("Website Redesign").first().click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

    await expect(page.getByTestId("open-file-index.html")).toBeVisible({ timeout: 5000 });
    await humanClick(page, page.getByTestId("open-file-index.html"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Switch to focus view
    await humanClick(page, page.getByTestId("layout-focus"));
    await expect(page.getByTestId("focus-view")).toBeVisible();
    await pause(page, 500);

    // Show focus view with source and target
    await expect(page.getByTestId("focus-source")).toBeVisible();
    await expect(page.getByTestId("focus-target")).toBeVisible();
    await pause(page, 400);

    // Type a translation in the target area with human speed
    await humanType(page, page.getByTestId("focus-target"), "Bonjour depuis index.html");
    await pause(page, 500);

    // Navigate to next block
    await page.keyboard.press("Tab");
    await pause(page, 400);
    await page.keyboard.press("Tab");
    await pause(page, 800);
  });

  test("record TM explorer", async ({ page }) => {
    await setupRecording(page, "Bowrain — Translation Memory");

    // Create project
    await page.getByTestId("new-project-btn").click();
    await page.getByTestId("project-name-input").fill("Website Redesign");
    await page.getByTestId("target-langs-input").fill("fr");
    await page.getByTestId("create-project-submit").click();
    await expect(page.getByTestId("back-to-projects")).toBeVisible();
    await pause(page, 300);

    // Seed TM entries
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
    await page.evaluate(() => {
      (document.querySelector('[data-testid="open-tm-btn"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("tm-explorer")).toBeVisible();
    await pause(page, 400);

    // Trigger refresh
    await setInput(page, "tm-search-input", " ");
    await page.waitForTimeout(400);
    await setInput(page, "tm-search-input", "");
    await page.waitForTimeout(400);

    await expect(page.getByTestId("tm-count-badge")).toContainText("5 entries");
    await pause(page, 400);

    // Search for something with human typing
    await humanType(page, page.getByTestId("tm-search-input"), "Welcome");
    await pause(page, 600);

    // Clear and show all again
    await page.getByTestId("tm-search-input").clear();
    await pause(page, 600);
  });

  test("record flow editor", async ({ page }) => {
    await setupRecording(page, "Bowrain — Flow Editor");
    await pause(page, 400);

    // Navigate to Flows view
    await humanClick(page, page.locator("nav button", { hasText: "Flows" }));
    await expect(page.getByTestId("flow-list")).toBeVisible();
    await pause(page, 500);

    // Select AI Translate flow
    await humanClick(page, page.getByTestId("flow-item-ai-translate"));
    await pause(page, 500);

    // Verify flow nodes are visible and pan around
    await expect(page.getByTestId("flow-node-reader")).toBeVisible();
    await pause(page, 400);

    // Click on nodes to show selection (force=true for React Flow overlay)
    await humanClick(page, page.getByTestId("flow-node-reader"), true);
    await pause(page, 400);
    await humanClick(page, page.getByTestId("flow-node-ai-translate"), true);
    await pause(page, 400);
    await humanClick(page, page.getByTestId("flow-node-writer"), true);
    await pause(page, 600);

    // Switch to pseudo-translate flow
    await humanClick(page, page.getByTestId("flow-item-pseudo-translate"), true);
    await pause(page, 800);
  });

  test("record end-to-end translation workflow", async ({ page }) => {
    // This recording shows a complete translation workflow:
    // 1. Open a project (simulating opening a .kaz file created by kapi pack)
    // 2. View project files
    // 3. Open translation editor
    // 4. Use different editor views (grid, split preview, focus)
    // 5. Translate content
    
    await setupRecording(page, "Bowrain — Translation Workflow");
    await pause(page, 600);

    // Create project to simulate opening a .kaz file from CLI
    await humanClick(page, page.getByTestId("new-project-btn"));
    await pause(page, 400);
    
    await humanType(page, page.getByTestId("project-name-input"), "Acme Landing Page");
    await pause(page, 300);

    await humanType(page, page.getByTestId("target-langs-input"), "fr, de");
    await pause(page, 400);

    await humanClick(page, page.getByTestId("create-project-submit"));
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();
    await pause(page, 400);

    // Add the HTML file (matching the CLI demo)
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      if (projects.length > 0) {
        await backend.AddFiles(projects[0].id, ["/landing-page.html"]);
      }
    });

    // Refresh to show the file
    await page.locator("nav button", { hasText: "Settings" }).click();
    await pause(page, 100);
    await page.locator("nav button", { hasText: "Projects" }).click();
    await pause(page, 200);
    
    await humanClick(page, page.getByText("Acme Landing Page").first());
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Show the project with its file
    await expect(page.getByTestId("open-file-landing-page.html")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Open the file in the editor
    await humanClick(page, page.getByTestId("open-file-landing-page.html"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // View 1: Grid view - show all blocks
    await expect(page.getByTestId("block-row-0")).toBeVisible();
    await pause(page, 400);

    // Select first block
    await humanClick(page, page.getByTestId("block-row-0"));
    await pause(page, 400);

    // View 2: Enable split preview to see live rendering
    await humanClick(page, page.getByTestId("layout-split-v"));
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Translate first block manually with visible typing
    const blockTargetInput = page.locator('[data-testid="block-row-0"] [data-testid="target-input"]');
    if (await blockTargetInput.isVisible()) {
      await humanType(page, blockTargetInput, "Bienvenue chez Acme");
      await pause(page, 400);
    }

    // View 3: Switch to focus view for distraction-free editing
    await humanClick(page, page.getByTestId("layout-focus"));
    await expect(page.getByTestId("focus-view")).toBeVisible();
    await pause(page, 500);

    // Type translation in focus view
    await humanType(page, page.getByTestId("focus-target"), "La meilleure solution pour vos besoins");
    await pause(page, 400);

    // Navigate through a few blocks
    await page.keyboard.press("Tab");
    await pause(page, 400);
    await page.keyboard.press("Tab");
    await pause(page, 400);

    // Use pseudo-translate for remaining content
    await humanClick(page, page.getByTestId("pseudo-btn"));
    await pause(page, 600);

    // Show 100% progress
    await expect(page.getByTestId("progress-text")).toContainText("100%");
    await pause(page, 500);

    // Switch back to grid view to see all translations
    await humanClick(page, page.getByTestId("layout-grid"));
    await pause(page, 800);
  });

  test("record settings configuration", async ({ page }) => {
    await setupRecording(page, "Bowrain — Settings");
    await pause(page, 400);

    // Navigate to Settings
    await humanClick(page, page.locator("nav button", { hasText: "Settings" }));
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await pause(page, 400);

    // Switch to AI Providers tab
    await humanClick(page, page.getByTestId("settings-tab-ai-providers"));
    await expect(page.getByTestId("settings-ai-providers")).toBeVisible();
    await pause(page, 400);

    // Add a provider with visible typing
    await humanClick(page, page.getByTestId("add-provider-btn"));
    await pause(page, 400);

    await humanType(page, page.getByTestId("provider-name"), "Anthropic Claude");
    await pause(page, 300);

    await humanClick(page, page.getByTestId("provider-type"));
    await page.getByTestId("provider-type").selectOption("anthropic");
    await pause(page, 400);

    await humanType(page, page.getByTestId("provider-api-key"), "sk-ant-api03-xxxx");
    await pause(page, 300);

    await humanType(page, page.getByTestId("provider-model"), "claude-sonnet-4-20250514");
    await pause(page, 400);

    await humanClick(page, page.getByTestId("provider-save-btn"));
    await expect(page.getByText("Anthropic Claude")).toBeVisible();
    await pause(page, 800);
  });
});
