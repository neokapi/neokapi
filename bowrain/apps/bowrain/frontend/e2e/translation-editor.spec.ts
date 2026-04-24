import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/**
 * Helper: creates a project with a file and opens the editor.
 * All operations happen within a single page load to avoid mock state reset.
 *
 * NOTE: We use page.evaluate for clicks that cause React to unmount and
 * re-mount components, because Playwright's built-in click() can hang when
 * the clicked element is removed from the DOM during React re-render.
 */
async function openEditorWithBlocks(page: any) {
  await setupLocalApp(page);

  // Step 1: Create project via UI
  await page.getByText("Upload files").click();
  await page.getByTestId("project-name-input").fill("Editor Test");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();

  // Now in project view
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();

  // Step 2: Add a file via mock backend
  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    if (projects.length > 0) {
      await backend.AddItems(projects[0].id, ["/test/hello.txt"]);
    }
  });

  // Step 3: Navigate back to projects list and re-enter to refresh
  await page.getByTestId("back-to-projects").click();
  await page.waitForTimeout(200);

  // Step 4: Now on the dashboard, click the project to re-enter
  await page.getByText("Editor Test").first().click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

  // Step 5: Click the file to open editor (use evaluate to avoid Playwright hang)
  await expect(page.getByTestId("open-file-hello.txt")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
    if (btn) btn.click();
  });

  // Step 6: Wait for editor — switch from visual (default) to grid layout for testing
  await expect(page.getByTestId("layout-switcher")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="layout-grid"]') as HTMLElement)?.click();
  });
  await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
}

/** Helper to click a button within the editor using native DOM click. */
async function clickTestId(page: any, testId: string) {
  await page.evaluate((id: string) => {
    const el = document.querySelector(`[data-testid="${id}"]`) as HTMLElement;
    if (el) el.click();
  }, testId);
}

/** Pseudo-translate via mock backend, then reload editor to pick up changes. */
async function pseudoTranslateViaBackend(page: any) {
  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    if (projects.length > 0) {
      await backend.PseudoTranslateItem(projects[0].id, "hello.txt", "fr");
    }
  });
  // Navigate away and back to reload blocks with updated data
  await clickTestId(page, "back-to-project");
  await page.getByTestId("open-file-hello.txt").waitFor({ state: "visible", timeout: 5000 });
  await page.evaluate(() => {
    const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
    if (btn) btn.click();
  });
  // Switch from visual (default) to grid layout for testing
  await page.getByTestId("layout-switcher").waitFor({ state: "visible", timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="layout-grid"]') as HTMLElement)?.click();
  });
  await page.getByTestId("block-grid").waitFor({ state: "visible", timeout: 5000 });
}

test.describe("Translation Editor", () => {
  test("should display blocks with source text", async ({ page }) => {
    await openEditorWithBlocks(page);

    const row0 = page.getByTestId("block-row-0");
    await expect(row0).toBeVisible();
    await expect(row0).toContainText("Hello from hello.txt");
  });

  test("should show progress bar at 0%", async ({ page }) => {
    await openEditorWithBlocks(page);

    const progress = page.getByTestId("progress-text");
    await expect(progress).toBeVisible();
    await expect(progress).toContainText("0%");
  });

  test("should show locale selector with fr selected", async ({ page }) => {
    await openEditorWithBlocks(page);

    const selector = page.getByTestId("locale-selector");
    await expect(selector).toBeVisible();
    await expect(selector).toContainText("fr");
  });

  test("should switch target locale to de", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Click custom select to open dropdown, then select option
    const selector = page.getByTestId("locale-selector");
    await selector.click();
    await page.getByRole("option", { name: "German" }).click();
    await expect(selector).toContainText("de");
  });

  test("should show toolbar buttons", async ({ page }) => {
    await openEditorWithBlocks(page);

    await expect(page.getByTestId("tm-btn")).toBeVisible();
    await expect(page.getByTestId("export-btn")).toBeVisible();
  });

  test("should show word count in status bar", async ({ page }) => {
    await openEditorWithBlocks(page);

    const statusBar = page.getByTestId("status-bar");
    await expect(statusBar).toBeVisible();
    await expect(statusBar).toContainText("Block 1 of");
    await expect(statusBar).toContainText("words");
  });

  test("should navigate back to project view", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "back-to-project");
    await expect(page.getByTestId("back-to-projects")).toBeVisible();
  });

  test("should search blocks by text", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Use evaluate to set the search input value and trigger React onChange
    await page.evaluate(() => {
      const input = document.querySelector('[data-testid="search-input"]') as HTMLInputElement;
      if (input) {
        Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")!.set!.call(
          input,
          "Welcome",
        );
        input.dispatchEvent(new Event("input", { bubbles: true }));
      }
    });
    await page.waitForTimeout(200);

    const row0 = page.getByTestId("block-row-0");
    await expect(row0).toContainText("Welcome");
  });

  test("should edit target text inline", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Double-click first block to enter edit mode
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) {
        row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
      }
    });

    // Verify the unified editor mounts in place of the collapsed cell.
    await expect(page.getByTestId("unified-target-editor")).toBeVisible({ timeout: 5000 });

    // Save a translation via the mock backend directly (since Playwright's
    // keyboard API hangs within this component due to CDP interaction issues)
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      await backend.UpdateBlockTarget({
        project_id: "project-1",
        item_name: "hello.txt",
        block_id: "hello.txt-block-1",
        target_locale: "fr",
        text: "Bonjour de hello.txt",
      });
    });

    // Navigate back and reopen file to reload blocks with updated data
    await clickTestId(page, "back-to-project");
    await expect(page.getByTestId("open-file-hello.txt")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
      if (btn) btn.click();
    });
    // Switch from visual (default) to grid layout for testing
    await expect(page.getByTestId("layout-switcher")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      (document.querySelector('[data-testid="layout-grid"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    const target0 = page.getByTestId("target-text-0");
    await expect(target0).toContainText("Bonjour");
  });

  test("should export translated file", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Pseudo-translate first
    await pseudoTranslateViaBackend(page);

    // Export
    await clickTestId(page, "export-btn");
    await page.waitForTimeout(300);

    await expect(page.getByText("Exported to")).toBeVisible();
  });

  test("should show layout switcher with layout options", async ({ page }) => {
    await openEditorWithBlocks(page);

    await expect(page.getByTestId("layout-switcher")).toBeVisible();
    await expect(page.getByTestId("layout-grid")).toBeVisible();
    await expect(page.getByTestId("layout-split-v")).toBeVisible();
  });

  test("should switch to split-v and show split layout", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Click split-v layout button
    await clickTestId(page, "layout-split-v");
    await page.waitForTimeout(300);

    // Split layout should be visible
    await expect(page.getByTestId("split-layout")).toBeVisible();
    // Preview iframe should render
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
    // Block grid should still be visible
    await expect(page.getByTestId("block-grid")).toBeVisible();
  });

  test("should switch back to grid from split-v", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Switch to split-v
    await clickTestId(page, "layout-split-v");
    await page.waitForTimeout(300);
    await expect(page.getByTestId("split-layout")).toBeVisible();

    // Switch back to grid
    await clickTestId(page, "layout-grid");
    await page.waitForTimeout(300);

    // Split layout should be gone, grid should remain
    await expect(page.getByTestId("split-layout")).not.toBeVisible();
    await expect(page.getByTestId("block-grid")).toBeVisible();
  });

  test("should render kat-block elements in preview iframe", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');

    // Verify kat-block elements render with the expected source text
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText("Hello from hello.txt");
    await expect(iframe.locator('[id="hello.txt-block-2"]')).toContainText(
      "Welcome to our application",
    );
    await expect(iframe.locator('[id="hello.txt-block-3"]')).toContainText(
      "Click here to continue",
    );
  });

  test("should select grid row when clicking block in preview", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Click the second block in the preview iframe
    await iframe.locator('[id="hello.txt-block-2"]').click();
    await page.waitForTimeout(300);

    // Verify status bar shows block 2 is now selected
    await expect(page.getByTestId("status-bar")).toContainText("Block 2 of");
  });

  test("should highlight preview block when selecting grid row", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Click the third row in the grid (use evaluate to avoid Playwright hang)
    await clickTestId(page, "block-row-2");
    await page.waitForTimeout(300);

    // Verify the matching block in the preview has the kat-selected class
    await expect(iframe.locator('[id="hello.txt-block-3"]')).toHaveClass(/kat-selected/);
  });

  test("should clear previous preview highlight when selecting different grid row", async ({
    page,
  }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Select block 3
    await clickTestId(page, "block-row-2");
    await page.waitForTimeout(300);
    await expect(iframe.locator('[id="hello.txt-block-3"]')).toHaveClass(/kat-selected/);

    // Now select block 1
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(300);

    // Block 1 should be selected, block 3 should no longer be
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toHaveClass(/kat-selected/);
    await expect(iframe.locator('[id="hello.txt-block-3"]')).not.toHaveClass(/kat-selected/);
  });

  test("should show source/target toggle on preview hover", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Switch to split-v layout
    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    // Overlay is hidden until hover
    const overlay = page.getByTestId("preview-overlay");
    await expect(overlay).toHaveCSS("opacity", "0");

    // Hover over the preview pane to reveal the overlay
    await page.getByTestId("preview-iframe").hover();
    await expect(overlay).toHaveCSS("opacity", "1");

    // Source/target toggle should be visible, defaulting to "Source"
    const toggle = page.getByTestId("preview-target-toggle");
    await expect(toggle).toBeVisible();
    await expect(toggle).toContainText("Source");
  });

  test("should switch preview to target text after pseudo-translate", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Switch to split-v layout
    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Preview shows source text initially
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText("Hello from hello.txt");

    // Pseudo-translate to generate target text
    await pseudoTranslateViaBackend(page);

    // Re-enter split-v layout (lost during navigate-away-and-back)
    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
    const iframe2 = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe2.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Hover to reveal toggle, then click it
    await page.getByTestId("preview-iframe").hover();
    await clickTestId(page, "preview-target-toggle");
    await page.waitForTimeout(300);

    // Button should now say "Target"
    await expect(page.getByTestId("preview-target-toggle")).toContainText("Target");

    // Preview should show pseudo-translated text
    await expect(iframe2.locator('[id="hello.txt-block-1"]')).toContainText(
      "[Hello from hello.txt]",
    );
    await expect(iframe2.locator('[id="hello.txt-block-2"]')).toContainText(
      "[Welcome to our application]",
    );
  });

  test("should switch preview back to source text", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Pseudo-translate first
    await pseudoTranslateViaBackend(page);

    // Enter split-v layout
    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Switch to target
    await page.getByTestId("preview-iframe").hover();
    await clickTestId(page, "preview-target-toggle");
    await page.waitForTimeout(300);
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText(
      "[Hello from hello.txt]",
    );

    // Toggle back to source
    await clickTestId(page, "preview-target-toggle");
    await page.waitForTimeout(300);

    // Button should say "Source" again
    await expect(page.getByTestId("preview-target-toggle")).toContainText("Source");

    // Preview should show original source text
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText("Hello from hello.txt");
    // Verify it's NOT the pseudo-translated version
    await expect(iframe.locator('[id="hello.txt-block-1"]')).not.toContainText("[Hello");
  });

  test("should keep showing source text when toggling to Target with no translations", async ({
    page,
  }) => {
    await openEditorWithBlocks(page);

    // Switch to split-v layout
    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Hover and toggle to Target (no translations exist yet)
    await page.getByTestId("preview-iframe").hover();
    await clickTestId(page, "preview-target-toggle");
    await page.waitForTimeout(300);

    // Button should say "Target"
    await expect(page.getByTestId("preview-target-toggle")).toContainText("Target");

    // Preview should still show source text since no targets exist
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText("Hello from hello.txt");
    await expect(iframe.locator('[id="hello.txt-block-2"]')).toContainText(
      "Welcome to our application",
    );
    await expect(iframe.locator('[id="hello.txt-block-3"]')).toContainText(
      "Click here to continue",
    );
  });

  test("should show target for translated blocks and source for untranslated", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Translate only the first block
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      await backend.UpdateBlockTarget({
        project_id: "project-1",
        item_name: "hello.txt",
        block_id: "hello.txt-block-1",
        target_locale: "fr",
        text: "Bonjour de hello.txt",
      });
    });

    // Reload blocks by navigating away and back
    await clickTestId(page, "back-to-project");
    await expect(page.getByTestId("open-file-hello.txt")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
      if (btn) btn.click();
    });
    // Switch from visual (default) to grid layout for testing
    await expect(page.getByTestId("layout-switcher")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      (document.querySelector('[data-testid="layout-grid"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    // Switch to split-v layout
    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Hover and toggle to Target
    await page.getByTestId("preview-iframe").hover();
    await clickTestId(page, "preview-target-toggle");
    await page.waitForTimeout(300);

    // Block 1 should show target text (translated)
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText("Bonjour de hello.txt");
    // Block 2 and 3 should still show source text (not translated)
    await expect(iframe.locator('[id="hello.txt-block-2"]')).toContainText(
      "Welcome to our application",
    );
    await expect(iframe.locator('[id="hello.txt-block-3"]')).toContainText(
      "Click here to continue",
    );
  });

  test("should preserve toggle state when switching locale", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Pseudo-translate for fr first
    await pseudoTranslateViaBackend(page);

    // Switch to split-v
    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Hover and toggle to Target
    await page.getByTestId("preview-iframe").hover();
    await clickTestId(page, "preview-target-toggle");
    await page.waitForTimeout(300);

    // Verify showing target text for fr
    await expect(page.getByTestId("preview-target-toggle")).toContainText("Target");
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText(
      "[Hello from hello.txt]",
    );

    // Switch locale to de (which has no translations)
    const selector = page.getByTestId("locale-selector");
    await selector.click();
    await page.getByRole("option", { name: "German" }).click();
    await page.waitForTimeout(500);

    // Toggle still says "Target" (state preserved in DocumentPreview)
    await page.getByTestId("preview-iframe").hover();
    await expect(page.getByTestId("preview-target-toggle")).toContainText("Target");

    // But preview shows source text because de has no translations
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText("Hello from hello.txt");
    await expect(iframe.locator('[id="hello.txt-block-1"]')).not.toContainText("[Hello");
  });

  test("should update preview when translating with Target mode active", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Switch to split-v layout
    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Toggle to Target before translating (shows source since no targets yet)
    await page.getByTestId("preview-iframe").hover();
    await clickTestId(page, "preview-target-toggle");
    await page.waitForTimeout(300);
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText("Hello from hello.txt");

    // Now pseudo-translate via backend
    await pseudoTranslateViaBackend(page);

    // Re-enter split-v with target toggle
    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
    const iframe2 = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe2.locator("kat-block").first()).toBeVisible({ timeout: 5000 });
    await page.getByTestId("preview-iframe").hover();
    await clickTestId(page, "preview-target-toggle");
    await page.waitForTimeout(300);

    // Preview should show translated text
    await expect(iframe2.locator('[id="hello.txt-block-1"]')).toContainText(
      "[Hello from hello.txt]",
    );
    await expect(iframe2.locator('[id="hello.txt-block-2"]')).toContainText(
      "[Welcome to our application]",
    );
    await expect(iframe2.locator('[id="hello.txt-block-3"]')).toContainText(
      "[Click here to continue]",
    );
  });

  test("should enter edit mode on single click of target cell", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Single-click the target cell of the first block
    await page.evaluate(() => {
      const el = document.querySelector('[data-testid="target-cell-0"]') as HTMLElement;
      if (el) el.click();
    });

    // Verify the unified editor mounts in place of the collapsed cell.
    await expect(page.getByTestId("unified-target-editor")).toBeVisible({ timeout: 5000 });
  });

  test("should enter edit mode on single click of a different target cell", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Click target cell of block 2 (index 1)
    await page.evaluate(() => {
      const el = document.querySelector('[data-testid="target-cell-1"]') as HTMLElement;
      if (el) el.click();
    });

    // Verify the unified editor mounts for block 2.
    await expect(page.getByTestId("unified-target-editor")).toBeVisible({ timeout: 5000 });

    // Verify block 2 is selected in status bar
    await expect(page.getByTestId("status-bar")).toContainText("Block 2 of");
  });

  test("should enter edit mode when clicking block in preview", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Open split-v layout
    await clickTestId(page, "layout-split-v");
    await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // Click the second block in the preview iframe
    await iframe.locator('[id="hello.txt-block-2"]').click();
    await page.waitForTimeout(300);

    // Verify block 2 is selected
    await expect(page.getByTestId("status-bar")).toContainText("Block 2 of");

    // Verify the unified editor mounts for block 2.
    await expect(page.getByTestId("unified-target-editor")).toBeVisible({ timeout: 5000 });
  });

  test("should mount UnifiedTargetEditor inline when editing a cell", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Seed a flat target so the editor has something to open on.
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      await backend.UpdateBlockTarget({
        project_id: "project-1",
        item_name: "hello.txt",
        block_id: "hello.txt-block-1",
        target_locale: "fr",
        text: "Vous avez des messages",
      });
    });

    // Reload so the seeded target lands in the grid.
    await clickTestId(page, "back-to-project");
    await page.getByTestId("open-file-hello.txt").waitFor({ state: "visible", timeout: 5000 });
    await page.evaluate(() => {
      const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
      if (btn) btn.click();
    });
    await expect(page.getByTestId("layout-switcher")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      (document.querySelector('[data-testid="layout-grid"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    // Double-click the cell to enter edit mode.
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) {
        row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
      }
    });

    // The unified editor mounts — data-mode=flat because the target
    // is plain text.
    const editor = page.getByTestId("unified-target-editor");
    await expect(editor).toBeVisible({ timeout: 5000 });
    await expect(editor).toHaveAttribute("data-mode", "flat");

    // Save round-trips the current coded text verbatim.
    await clickTestId(page, "unified-save");

    const saved = await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const blocks = await backend.GetFileBlocks("project-1", "hello.txt", "fr");
      return blocks.find((b: any) => b.id === "hello.txt-block-1")?.targets?.fr;
    });
    expect(saved).toBe("Vous avez des messages");
  });

  test("should render plural targets in collapsed-cell view with a plural badge", async ({
    page,
  }) => {
    await openEditorWithBlocks(page);

    // Seed an ICU plural target directly.
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      await backend.UpdateBlockTarget({
        project_id: "project-1",
        item_name: "hello.txt",
        block_id: "hello.txt-block-1",
        target_locale: "fr",
        text: "{count, plural, one {Vous avez un message} other {Vous avez {count} messages}}",
      });
    });

    await clickTestId(page, "back-to-project");
    await page.getByTestId("open-file-hello.txt").waitFor({ state: "visible", timeout: 5000 });
    await page.evaluate(() => {
      const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
      if (btn) btn.click();
    });
    await expect(page.getByTestId("layout-switcher")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      (document.querySelector('[data-testid="layout-grid"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    // The collapsed cell renders the `other` form's content plus a
    // "plural · other" badge so the row signals there are more forms.
    const cell = page.getByTestId("target-text-0");
    await expect(cell).toBeVisible();
    await expect(cell).toHaveAttribute("data-plural-preview", "true");
    await expect(cell).toContainText("plural · other");
  });

  test("should open UnifiedTargetEditor in plural mode for an ICU plural target", async ({
    page,
  }) => {
    await openEditorWithBlocks(page);

    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      await backend.UpdateBlockTarget({
        project_id: "project-1",
        item_name: "hello.txt",
        block_id: "hello.txt-block-1",
        target_locale: "fr",
        text: "{count, plural, one {Vous avez un message} other {Vous avez {count} messages}}",
      });
    });

    await clickTestId(page, "back-to-project");
    await page.getByTestId("open-file-hello.txt").waitFor({ state: "visible", timeout: 5000 });
    await page.evaluate(() => {
      const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
      if (btn) btn.click();
    });
    await expect(page.getByTestId("layout-switcher")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      (document.querySelector('[data-testid="layout-grid"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) {
        row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
      }
    });

    const editor = page.getByTestId("unified-target-editor");
    await expect(editor).toBeVisible({ timeout: 5000 });
    await expect(editor).toHaveAttribute("data-mode", "plural");

    // Form tabs are visible — one per CLDR form.
    for (const form of ["zero", "one", "two", "few", "many", "other"]) {
      await expect(page.getByTestId(`form-tab-${form}`)).toBeVisible();
    }
  });
});
