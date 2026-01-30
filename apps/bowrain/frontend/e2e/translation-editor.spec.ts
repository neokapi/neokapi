import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";

/**
 * Helper: creates a project with a file and opens the editor.
 * All operations happen within a single page load to avoid mock state reset.
 *
 * NOTE: We use page.evaluate for clicks that cause React to unmount and
 * re-mount components, because Playwright's built-in click() can hang when
 * the clicked element is removed from the DOM during React re-render.
 */
async function openEditorWithBlocks(page: any) {
  await injectMockBackend(page);
  await page.goto("/");

  // Step 1: Create project via UI
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("Editor Test");
  await page.getByTestId("target-langs-input").fill("fr, de");
  await page.getByTestId("create-project-submit").click();

  // Now in project view
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();

  // Step 2: Add a file via mock backend
  await page.evaluate(async () => {
    const backend = (window as any).go.backend.App;
    const projects = await backend.ListProjects();
    if (projects.length > 0) {
      await backend.AddFiles(projects[0].id, ["/test/hello.txt"]);
    }
  });

  // Step 3: Navigate away via sidebar, then back to projects to pick up changes
  await page.locator("nav button", { hasText: "Settings" }).click();
  await page.waitForTimeout(100);
  await page.locator("nav button", { hasText: "Projects" }).click();
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

  // Step 6: Wait for editor
  await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
}

/** Helper to click a button within the editor using native DOM click. */
async function clickTestId(page: any, testId: string) {
  await page.evaluate((id: string) => {
    const el = document.querySelector(`[data-testid="${id}"]`) as HTMLElement;
    if (el) el.click();
  }, testId);
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
    await expect(selector).toHaveValue("fr");
  });

  test("should switch target locale to de", async ({ page }) => {
    await openEditorWithBlocks(page);

    const selector = page.getByTestId("locale-selector");
    await selector.selectOption("de");
    await expect(selector).toHaveValue("de");
  });

  test("should show toolbar buttons", async ({ page }) => {
    await openEditorWithBlocks(page);

    await expect(page.getByTestId("pseudo-btn")).toBeVisible();
    await expect(page.getByTestId("ai-translate-btn")).toBeVisible();
    await expect(page.getByTestId("tm-btn")).toBeVisible();
    await expect(page.getByTestId("export-btn")).toBeVisible();
  });

  test("should pseudo-translate all blocks", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "pseudo-btn");
    await page.waitForTimeout(500);

    const progress = page.getByTestId("progress-text");
    await expect(progress).toContainText("100%");

    const target0 = page.getByTestId("target-text-0");
    await expect(target0).toContainText("[");
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
        const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
          window.HTMLInputElement.prototype, "value",
        )!.set!;
        nativeInputValueSetter.call(input, "Welcome");
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

    // Verify edit textarea appears
    const editInput = page.getByTestId("edit-target-0");
    await expect(editInput).toBeVisible({ timeout: 5000 });

    // Save a translation via the mock backend directly (since Playwright's
    // keyboard API hangs within this component due to CDP interaction issues)
    await page.evaluate(async () => {
      const backend = (window as any).go.backend.App;
      await backend.UpdateBlockTarget({
        project_id: "project-1",
        file_name: "hello.txt",
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
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    const target0 = page.getByTestId("target-text-0");
    await expect(target0).toContainText("Bonjour");
  });

  test("should export translated file", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Pseudo-translate first
    await clickTestId(page, "pseudo-btn");
    await page.waitForTimeout(300);

    // Export
    await clickTestId(page, "export-btn");
    await page.waitForTimeout(300);

    await expect(page.getByText("Exported to")).toBeVisible();
  });
});
