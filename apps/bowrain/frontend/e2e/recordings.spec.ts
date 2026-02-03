import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";

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
    await injectMockBackend(page);
    await page.goto("/");
    await pause(page, 800);

    // Click new project button
    await page.getByTestId("new-project-btn").click();
    await pause(page, 400);

    // Fill in project details with visible typing
    const nameInput = page.getByTestId("project-name-input");
    await nameInput.click();
    await pause(page, 200);
    await nameInput.pressSequentially("Website Redesign", { delay: 50 });
    await pause(page, 300);

    const targetInput = page.getByTestId("target-langs-input");
    await targetInput.click();
    await pause(page, 200);
    await targetInput.pressSequentially("fr, de, ja", { delay: 50 });
    await pause(page, 400);

    // Create project
    await page.getByTestId("create-project-submit").click();
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
    await injectMockBackend(page);
    await page.goto("/");

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
    await page.evaluate(() => {
      const btn = document.querySelector('[data-testid="open-file-index.html"]') as HTMLElement;
      if (btn) btn.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Show the editor with blocks
    await expect(page.getByTestId("block-row-0")).toBeVisible();
    await pause(page, 400);

    // Click on first block to select it
    await clickTestId(page, "block-row-0");
    await pause(page, 400);

    // Enable split preview
    await clickTestId(page, "layout-split-v");
    await pause(page, 500);
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Run pseudo-translate
    await clickTestId(page, "pseudo-btn");
    await pause(page, 600);

    // Verify progress
    await expect(page.getByTestId("progress-text")).toContainText("100%");
    await pause(page, 800);
  });

  test("record focus view editing", async ({ page }) => {
    await injectMockBackend(page);
    await page.goto("/");

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
    await page.evaluate(() => {
      const btn = document.querySelector('[data-testid="open-file-index.html"]') as HTMLElement;
      if (btn) btn.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Switch to focus view
    await clickTestId(page, "layout-focus");
    await pause(page, 500);
    await expect(page.getByTestId("focus-view")).toBeVisible();
    await pause(page, 400);

    // Show focus view with source and target
    await expect(page.getByTestId("focus-source")).toBeVisible();
    await expect(page.getByTestId("focus-target")).toBeVisible();
    await pause(page, 400);

    // Type a translation in the target area
    const targetArea = page.getByTestId("focus-target");
    await targetArea.click();
    await pause(page, 200);
    await targetArea.pressSequentially("Bonjour depuis index.html", { delay: 40 });
    await pause(page, 500);

    // Navigate to next block
    await page.keyboard.press("Tab");
    await pause(page, 300);
    await page.keyboard.press("Tab");
    await pause(page, 800);
  });

  test("record TM explorer", async ({ page }) => {
    await injectMockBackend(page);
    await page.goto("/");

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

    // Search for something
    const searchInput = page.getByTestId("tm-search-input");
    await searchInput.click();
    await pause(page, 200);
    await searchInput.pressSequentially("Welcome", { delay: 60 });
    await pause(page, 600);

    // Clear and show all again
    await searchInput.clear();
    await pause(page, 600);
  });

  test("record flow editor", async ({ page }) => {
    await injectMockBackend(page);
    await page.goto("/");
    await pause(page, 400);

    // Navigate to Flows view
    await page.locator("nav button", { hasText: "Flows" }).click();
    await expect(page.getByTestId("flow-list")).toBeVisible();
    await pause(page, 500);

    // Select AI Translate flow
    await page.getByTestId("flow-item-ai-translate").click();
    await pause(page, 500);

    // Verify flow nodes are visible and pan around
    await expect(page.getByTestId("flow-node-reader")).toBeVisible();
    await pause(page, 400);

    // Click on nodes to show selection (hover blocked by React Flow pane)
    await page.getByTestId("flow-node-reader").click({ force: true });
    await pause(page, 400);
    await page.getByTestId("flow-node-ai-translate").click({ force: true });
    await pause(page, 400);
    await page.getByTestId("flow-node-writer").click({ force: true });
    await pause(page, 600);

    // Switch to pseudo-translate flow
    await page.getByTestId("flow-item-pseudo-translate").click();
    await pause(page, 800);
  });

  test("record end-to-end translation workflow", async ({ page }) => {
    // This recording shows a complete translation workflow:
    // 1. Open a project (simulating opening a .kaz file created by kapi pack)
    // 2. View project files
    // 3. Open translation editor
    // 4. Use different editor views (grid, split preview, focus)
    // 5. Translate content
    
    await injectMockBackend(page);
    await page.goto("/");
    await pause(page, 600);

    // Create project to simulate opening a .kaz file from CLI
    await page.getByTestId("new-project-btn").click();
    await pause(page, 300);
    
    const nameInput = page.getByTestId("project-name-input");
    await nameInput.click();
    await nameInput.pressSequentially("Acme Landing Page", { delay: 40 });
    await pause(page, 200);

    const targetInput = page.getByTestId("target-langs-input");
    await targetInput.click();
    await targetInput.pressSequentially("fr, de", { delay: 40 });
    await pause(page, 300);

    await page.getByTestId("create-project-submit").click();
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
    
    await page.getByText("Acme Landing Page").first().click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 500);

    // Show the project with its file
    await expect(page.getByTestId("open-file-landing-page.html")).toBeVisible({ timeout: 5000 });
    await pause(page, 400);

    // Open the file in the editor
    await page.evaluate(() => {
      const btn = document.querySelector('[data-testid="open-file-landing-page.html"]') as HTMLElement;
      if (btn) btn.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // View 1: Grid view - show all blocks
    await expect(page.getByTestId("block-row-0")).toBeVisible();
    await pause(page, 400);

    // Select first block
    await clickTestId(page, "block-row-0");
    await pause(page, 400);

    // View 2: Enable split preview to see live rendering
    await clickTestId(page, "layout-split-v");
    await pause(page, 500);
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
    await pause(page, 600);

    // Translate first block manually with visible typing
    const blockTargetInput = page.locator('[data-testid="block-row-0"] [data-testid="target-input"]');
    if (await blockTargetInput.isVisible()) {
      await blockTargetInput.click();
      await pause(page, 200);
      await blockTargetInput.pressSequentially("Bienvenue chez Acme", { delay: 40 });
      await pause(page, 400);
    }

    // View 3: Switch to focus view for distraction-free editing
    await clickTestId(page, "layout-focus");
    await pause(page, 500);
    await expect(page.getByTestId("focus-view")).toBeVisible();
    await pause(page, 400);

    // Type translation in focus view
    const focusTarget = page.getByTestId("focus-target");
    await focusTarget.click();
    await pause(page, 200);
    await focusTarget.pressSequentially("La meilleure solution pour vos besoins", { delay: 35 });
    await pause(page, 400);

    // Navigate through a few blocks
    await page.keyboard.press("Tab");
    await pause(page, 300);
    await page.keyboard.press("Tab");
    await pause(page, 300);

    // Use pseudo-translate for remaining content
    await clickTestId(page, "pseudo-btn");
    await pause(page, 600);

    // Show 100% progress
    await expect(page.getByTestId("progress-text")).toContainText("100%");
    await pause(page, 500);

    // Switch back to grid view to see all translations
    await clickTestId(page, "layout-grid");
    await pause(page, 800);
  });

  test("record settings configuration", async ({ page }) => {
    await injectMockBackend(page);
    await page.goto("/");
    await pause(page, 300);

    // Navigate to Settings
    await page.locator("nav button", { hasText: "Settings" }).click();
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await pause(page, 400);

    // Switch to AI Providers tab
    await page.getByTestId("settings-tab-ai-providers").click();
    await expect(page.getByTestId("settings-ai-providers")).toBeVisible();
    await pause(page, 400);

    // Add a provider with visible typing
    await page.getByTestId("add-provider-btn").click();
    await pause(page, 300);

    const nameField = page.getByTestId("provider-name");
    await nameField.click();
    await nameField.pressSequentially("Anthropic Claude", { delay: 40 });
    await pause(page, 200);

    await page.getByTestId("provider-type").selectOption("anthropic");
    await pause(page, 300);

    const apiKeyField = page.getByTestId("provider-api-key");
    await apiKeyField.click();
    await apiKeyField.pressSequentially("sk-ant-api03-xxxx", { delay: 30 });
    await pause(page, 200);

    const modelField = page.getByTestId("provider-model");
    await modelField.click();
    await modelField.pressSequentially("claude-sonnet-4-20250514", { delay: 30 });
    await pause(page, 300);

    await page.getByTestId("provider-save-btn").click();
    await expect(page.getByText("Anthropic Claude")).toBeVisible();
    await pause(page, 800);
  });
});
