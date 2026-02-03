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
    await setupRecording(page, "Bowrain — Translation Editor");

    // Create project quickly
    await page.getByTestId("new-project-btn").click();
    await page.getByTestId("project-name-input").fill("Website Redesign");
    await page.getByTestId("target-langs-input").fill("fr, de");
    await page.getByTestId("create-project-submit").click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();

    // Add file
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      if (projects.length > 0) {
        await backend.AddFiles(projects[0].id, ["/src/index.html"]);
      }
    });

    // Refresh and open editor
    await page.locator("nav button", { hasText: "Settings" }).click();
    await page.waitForTimeout(100);
    await page.locator("nav button", { hasText: "Projects" }).click();
    await page.waitForTimeout(200);
    await page.getByText("Website Redesign").first().click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Open file in editor - this is where the interesting part starts
    await expect(page.getByTestId("open-file-index.html")).toBeVisible({ timeout: 5000 });
    await humanClick(page, page.getByTestId("open-file-index.html"));
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Show the editor with blocks
    await expect(page.getByTestId("block-row-0")).toBeVisible();
    await pause(page, 300);

    // Click on first block to select it
    await humanClick(page, page.getByTestId("block-row-0"));
    await pause(page, 400);

    // Enable split preview
    await humanClick(page, page.getByTestId("layout-split-v"));
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Run pseudo-translate
    await humanClick(page, page.getByTestId("pseudo-btn"));
    await pause(page, 500);

    // Verify progress
    await expect(page.getByTestId("progress-text")).toContainText("100%");
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
