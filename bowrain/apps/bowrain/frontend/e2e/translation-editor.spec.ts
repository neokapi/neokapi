import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/**
 * Helper: creates a project with a file and opens the editor in Table view.
 * The editor now has two views — Visual (default) and Table; the row-based
 * tests below scan blocks in Table view, which keeps the `block-grid` testid.
 *
 * NOTE: We use page.evaluate for clicks that cause React to unmount and
 * re-mount components, because Playwright's built-in click() can hang when
 * the clicked element is removed from the DOM during React re-render.
 */
async function openEditorInTable(page: any) {
  await setupLocalApp(page);

  // Step 1: Create project via UI
  await page.getByText("Upload files").click();
  await page.getByTestId("project-name-input").fill("Editor Test");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();

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

  // Step 5: Click the file to open editor
  await expect(page.getByTestId("open-file-hello.txt")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
    if (btn) btn.click();
  });

  // Step 6: Wait for editor — switch from Visual (default) to Table view
  await expect(page.getByTestId("view-switcher")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="view-table"]') as HTMLElement)?.click();
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

/** Reopen the editor in Table view after a navigate-away-and-back. */
async function reopenInTable(page: any) {
  await clickTestId(page, "back-to-project");
  await page.getByTestId("open-file-hello.txt").waitFor({ state: "visible", timeout: 5000 });
  await page.evaluate(() => {
    const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
    if (btn) btn.click();
  });
  await page.getByTestId("view-switcher").waitFor({ state: "visible", timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="view-table"]') as HTMLElement)?.click();
  });
  await page.getByTestId("block-grid").waitFor({ state: "visible", timeout: 5000 });
}

/** Switch the open editor to the Visual view and wait for the preview iframe. */
async function switchToVisual(page: any) {
  await page.evaluate(() => {
    (document.querySelector('[data-testid="view-visual"]') as HTMLElement)?.click();
  });
  await expect(page.getByTestId("preview-iframe")).toBeVisible({ timeout: 5000 });
}

/** Pseudo-translate via mock backend, then reload editor in Table view. */
async function pseudoTranslateViaBackend(page: any) {
  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    if (projects.length > 0) {
      await backend.PseudoTranslateItem(projects[0].id, "hello.txt", "fr");
    }
  });
  await reopenInTable(page);
}

test.describe("Translation Editor", () => {
  test("should display blocks with source text", async ({ page }) => {
    await openEditorInTable(page);

    const row0 = page.getByTestId("block-row-0");
    await expect(row0).toBeVisible();
    await expect(row0).toContainText("Hello from hello.txt");
  });

  test("should show progress bar at 0%", async ({ page }) => {
    await openEditorInTable(page);

    const progress = page.getByTestId("progress-text");
    await expect(progress).toBeVisible();
    await expect(progress).toContainText("0%");
  });

  test("should show locale selector with fr selected", async ({ page }) => {
    await openEditorInTable(page);

    const selector = page.getByTestId("locale-selector");
    await expect(selector).toBeVisible();
    await expect(selector).toContainText("fr");
  });

  test("should switch target locale to de", async ({ page }) => {
    await openEditorInTable(page);

    const selector = page.getByTestId("locale-selector");
    await selector.click();
    await page.getByRole("option", { name: "German" }).click();
    await expect(selector).toContainText("de");
  });

  test("should show view switcher and export button", async ({ page }) => {
    await openEditorInTable(page);

    await expect(page.getByTestId("view-switcher")).toBeVisible();
    await expect(page.getByTestId("view-visual")).toBeVisible();
    await expect(page.getByTestId("view-table")).toBeVisible();
    await expect(page.getByTestId("export-btn")).toBeVisible();
  });

  test("should show word count in status bar", async ({ page }) => {
    await openEditorInTable(page);

    const statusBar = page.getByTestId("status-bar");
    await expect(statusBar).toBeVisible();
    await expect(statusBar).toContainText("Block 1 of");
    await expect(statusBar).toContainText("words");
  });

  test("should navigate back to project view", async ({ page }) => {
    await openEditorInTable(page);

    await clickTestId(page, "back-to-project");
    await expect(page.getByTestId("back-to-projects")).toBeVisible();
  });

  test("should search blocks by text", async ({ page }) => {
    await openEditorInTable(page);

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
    await openEditorInTable(page);

    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) {
        row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
      }
    });

    await expect(page.getByTestId("unified-target-editor")).toBeVisible({ timeout: 5000 });

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

    await reopenInTable(page);

    const target0 = page.getByTestId("target-text-0");
    await expect(target0).toContainText("Bonjour");
  });

  test("should export translated file", async ({ page }) => {
    await openEditorInTable(page);
    await pseudoTranslateViaBackend(page);

    await clickTestId(page, "export-btn");
    await page.waitForTimeout(300);

    await expect(page.getByText("Exported to")).toBeVisible();
  });

  test("should switch to Visual view and show the document preview", async ({ page }) => {
    await openEditorInTable(page);

    await switchToVisual(page);
    await expect(page.getByTestId("visual-editor-layout")).toBeVisible();
  });

  test("should switch back to Table from Visual", async ({ page }) => {
    await openEditorInTable(page);

    await switchToVisual(page);
    await page.evaluate(() => {
      (document.querySelector('[data-testid="view-table"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible();
  });

  test("should render kat-block elements in the Visual preview iframe", async ({ page }) => {
    await openEditorInTable(page);
    await switchToVisual(page);

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });
    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText("Hello from hello.txt");
    await expect(iframe.locator('[id="hello.txt-block-2"]')).toContainText(
      "Welcome to our application",
    );
    await expect(iframe.locator('[id="hello.txt-block-3"]')).toContainText(
      "Click here to continue",
    );
  });

  test("should select block when clicking it in the Visual preview", async ({ page }) => {
    await openEditorInTable(page);
    await switchToVisual(page);

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    await iframe.locator('[id="hello.txt-block-2"]').click();
    await page.waitForTimeout(300);

    await expect(page.getByTestId("status-bar")).toContainText("Block 2 of");
  });

  test("should switch Visual preview to pseudo-translated target", async ({ page }) => {
    await openEditorInTable(page);
    await pseudoTranslateViaBackend(page);
    await switchToVisual(page);

    const iframe = page.frameLocator('[data-testid="preview-iframe"]');
    await expect(iframe.locator("kat-block").first()).toBeVisible({ timeout: 5000 });

    // The Visual view drives the preview content mode via the toolbar tabs.
    await page.getByRole("tab", { name: "Target" }).click();
    await page.waitForTimeout(300);

    await expect(iframe.locator('[id="hello.txt-block-1"]')).toContainText(
      "[Hello from hello.txt]",
    );
    await expect(iframe.locator('[id="hello.txt-block-2"]')).toContainText(
      "[Welcome to our application]",
    );
  });

  test("should enter edit mode on single click of a target cell", async ({ page }) => {
    await openEditorInTable(page);

    await page.evaluate(() => {
      const el = document.querySelector('[data-testid="target-cell-0"]') as HTMLElement;
      if (el) el.click();
    });

    await expect(page.getByTestId("unified-target-editor")).toBeVisible({ timeout: 5000 });
  });

  test("should enter edit mode on single click of a different target cell", async ({ page }) => {
    await openEditorInTable(page);

    await page.evaluate(() => {
      const el = document.querySelector('[data-testid="target-cell-1"]') as HTMLElement;
      if (el) el.click();
    });

    await expect(page.getByTestId("unified-target-editor")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("status-bar")).toContainText("Block 2 of");
  });

  test("should mount UnifiedTargetEditor inline when editing a cell", async ({ page }) => {
    await openEditorInTable(page);

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

    await reopenInTable(page);

    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) {
        row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
      }
    });

    const editor = page.getByTestId("unified-target-editor");
    await expect(editor).toBeVisible({ timeout: 5000 });
    await expect(editor).toHaveAttribute("data-mode", "flat");

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
    await openEditorInTable(page);

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

    await reopenInTable(page);

    const cell = page.getByTestId("target-text-0");
    await expect(cell).toBeVisible();
    await expect(cell).toHaveAttribute("data-plural-preview", "true");
    await expect(cell).toContainText("plural · other");
  });

  test("should open UnifiedTargetEditor in plural mode for an ICU plural target", async ({
    page,
  }) => {
    await openEditorInTable(page);

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

    await reopenInTable(page);

    await page.evaluate(() => {
      const row = document.querySelector('[data-testid="block-row-0"]') as HTMLElement;
      if (row) {
        row.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true }));
      }
    });

    const editor = page.getByTestId("unified-target-editor");
    await expect(editor).toBeVisible({ timeout: 5000 });
    await expect(editor).toHaveAttribute("data-mode", "plural");

    for (const form of ["zero", "one", "two", "few", "many", "other"]) {
      await expect(page.getByTestId(`form-tab-${form}`)).toBeVisible();
    }
  });
});
