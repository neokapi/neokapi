import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";

/**
 * Helper: creates a project with a file and opens the editor.
 */
async function openEditorWithBlocks(page: any) {
  await injectMockBackend(page);
  await page.goto("/");

  // Create project
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("Rich Editor Test");
  await page.getByTestId("target-langs-input").fill("fr, de");
  await page.getByTestId("create-project-submit").click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();

  // Add a file via mock backend
  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    if (projects.length > 0) {
      await backend.AddFiles(projects[0].id, ["/test/hello.txt"]);
    }
  });

  // Navigate away and back to pick up changes
  await page.locator("nav button", { hasText: "Settings" }).click();
  await page.waitForTimeout(100);
  await page.locator("nav button", { hasText: "Projects" }).click();
  await page.waitForTimeout(200);

  await page.getByText("Rich Editor Test").first().click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

  await expect(page.getByTestId("open-file-hello.txt")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
    if (btn) btn.click();
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

test.describe("Rich Editor -- Layout Modes", () => {
  test("should show all layout mode buttons", async ({ page }) => {
    await openEditorWithBlocks(page);

    await expect(page.getByTestId("layout-grid")).toBeVisible();
    await expect(page.getByTestId("layout-focus")).toBeVisible();
    await expect(page.getByTestId("layout-split-h")).toBeVisible();
    await expect(page.getByTestId("layout-split-v")).toBeVisible();
  });

  test("should default to grid layout", async ({ page }) => {
    await openEditorWithBlocks(page);

    await expect(page.getByTestId("block-grid")).toBeVisible();
    await expect(page.getByTestId("focus-view")).not.toBeVisible();
    await expect(page.getByTestId("split-layout")).not.toBeVisible();
  });

  test("should switch to focus view", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);

    await expect(page.getByTestId("focus-view")).toBeVisible();
    await expect(page.getByTestId("block-grid")).not.toBeVisible();
  });

  test("should switch to split-h layout", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-split-h");
    await page.waitForTimeout(300);

    await expect(page.getByTestId("block-grid")).toBeVisible();
    await expect(page.getByTestId("split-h-preview")).toBeVisible();
  });

  test("should switch to split-v layout", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-split-v");
    await page.waitForTimeout(300);

    await expect(page.getByTestId("split-layout")).toBeVisible();
    await expect(page.getByTestId("block-grid")).toBeVisible();
  });

  test("should switch back to grid from focus", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);
    await expect(page.getByTestId("focus-view")).toBeVisible();

    await clickTestId(page, "layout-grid");
    await page.waitForTimeout(300);

    await expect(page.getByTestId("block-grid")).toBeVisible();
    await expect(page.getByTestId("focus-view")).not.toBeVisible();
  });
});

test.describe("Rich Editor -- Focus View", () => {
  test("should show source and target in focus view", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);

    await expect(page.getByTestId("focus-source")).toBeVisible();
    await expect(page.getByTestId("focus-source")).toContainText("Hello from hello.txt");
    await expect(page.getByTestId("focus-target")).toBeVisible();
  });

  test("should show status badge in focus view", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);

    const badge = page.getByTestId("focus-status-badge");
    await expect(badge).toBeVisible();
    await expect(badge).toContainText("not-started");
  });

  test("should navigate with focus prev/next buttons", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);

    // Initially on block 1
    await expect(page.getByTestId("focus-source")).toContainText("Hello from hello.txt");

    // Navigate to next
    await clickTestId(page, "focus-next");
    await page.waitForTimeout(200);

    await expect(page.getByTestId("focus-source")).toContainText("Welcome to our application");

    // Navigate to prev
    await clickTestId(page, "focus-prev");
    await page.waitForTimeout(200);

    await expect(page.getByTestId("focus-source")).toContainText("Hello from hello.txt");
  });

  test("should show context blocks in focus view", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Navigate to block 2 (middle block)
    await clickTestId(page, "block-row-1");
    await page.waitForTimeout(100);

    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);

    // Previous context should show block 1
    await expect(page.getByTestId("focus-context-prev")).toBeVisible();
    await expect(page.getByTestId("focus-context-prev")).toContainText("Hello from hello.txt");

    // Next context should show block 3
    await expect(page.getByTestId("focus-context-next")).toBeVisible();
    await expect(page.getByTestId("focus-context-next")).toContainText("Click here to continue");
  });

  test("should have focus view copy source button", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);

    await expect(page.getByTestId("focus-copy-source")).toBeVisible();
  });

  test("should have focus view mark reviewed button", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);

    await expect(page.getByTestId("focus-mark-reviewed")).toBeVisible();
  });

  test("should show textarea for plain text editing in focus view", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);

    await expect(page.getByTestId("focus-edit-target")).toBeVisible();
  });
});

test.describe("Rich Editor -- Block Status", () => {
  test("should show status dots on block rows", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Initially all blocks are not-started, dots should be transparent
    const dot0 = page.getByTestId("status-dot-0");
    await expect(dot0).toBeVisible();
    await expect(dot0).toHaveAttribute("title", "not-started");
  });

  test("should show draft status after pseudo-translate", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "pseudo-btn");
    await page.waitForTimeout(500);

    const dot0 = page.getByTestId("status-dot-0");
    await expect(dot0).toHaveAttribute("title", "draft");
  });

  test("should show reviewed status after marking reviewed", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Pseudo-translate first to have content
    await clickTestId(page, "pseudo-btn");
    await page.waitForTimeout(500);

    // Select first block and mark reviewed
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(100);
    await clickTestId(page, "mark-reviewed-btn");
    await page.waitForTimeout(300);

    const dot0 = page.getByTestId("status-dot-0");
    await expect(dot0).toHaveAttribute("title", "reviewed");
  });

  test("should show status breakdown in progress bar after pseudo-translate", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "pseudo-btn");
    await page.waitForTimeout(500);

    const progress = page.getByTestId("progress-text");
    await expect(progress).toContainText("draft");
  });

  test("should show progress segments after pseudo-translate", async ({ page }) => {
    await openEditorWithBlocks(page);

    await clickTestId(page, "pseudo-btn");
    await page.waitForTimeout(500);

    await expect(page.getByTestId("progress-draft")).toBeVisible();
  });
});

test.describe("Rich Editor -- Toolbar Actions", () => {
  test("should show new toolbar buttons", async ({ page }) => {
    await openEditorWithBlocks(page);

    await expect(page.getByTestId("copy-source-btn")).toBeVisible();
    await expect(page.getByTestId("mark-reviewed-btn")).toBeVisible();
    await expect(page.getByTestId("prev-untranslated-btn")).toBeVisible();
    await expect(page.getByTestId("next-untranslated-btn")).toBeVisible();
  });

  test("should copy source to target when clicking Copy Source", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Select first block
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(100);

    // Click Copy Source
    await clickTestId(page, "copy-source-btn");
    await page.waitForTimeout(500);

    // Target should now contain the source text
    const target0 = page.getByTestId("target-text-0");
    await expect(target0).toContainText("Hello from hello.txt");
  });

  test("should mark block as reviewed", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Pseudo-translate first
    await clickTestId(page, "pseudo-btn");
    await page.waitForTimeout(500);

    // Select first block and mark reviewed
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(100);
    await clickTestId(page, "mark-reviewed-btn");
    await page.waitForTimeout(200);

    // Status dot should show reviewed
    const dot0 = page.getByTestId("status-dot-0");
    await expect(dot0).toHaveAttribute("title", "reviewed");
  });

  test("should navigate to next untranslated block", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Translate first block only
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(100);
    await clickTestId(page, "copy-source-btn");
    await page.waitForTimeout(300);

    // Now click next untranslated - should skip to block 2 (index 1)
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(100);
    await clickTestId(page, "next-untranslated-btn");
    await page.waitForTimeout(200);

    await expect(page.getByTestId("status-bar")).toContainText("Block 2 of");
  });

  test("should navigate to previous untranslated block", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Translate block 2 only
    await clickTestId(page, "block-row-1");
    await page.waitForTimeout(100);
    await clickTestId(page, "copy-source-btn");
    await page.waitForTimeout(300);

    // Move to block 3
    await clickTestId(page, "block-row-2");
    await page.waitForTimeout(100);

    // Click prev untranslated - should skip to block 1 (index 0)
    await clickTestId(page, "prev-untranslated-btn");
    await page.waitForTimeout(200);

    await expect(page.getByTestId("status-bar")).toContainText("Block 1 of");
  });
});

test.describe("Rich Editor -- Focus View Status Badge", () => {
  test("should show draft badge in focus view after pseudo-translate", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Pseudo-translate
    await clickTestId(page, "pseudo-btn");
    await page.waitForTimeout(500);

    // Switch to focus view
    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);

    const badge = page.getByTestId("focus-status-badge");
    await expect(badge).toContainText("draft");
  });

  test("should update focus badge when marking reviewed", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Pseudo-translate
    await clickTestId(page, "pseudo-btn");
    await page.waitForTimeout(500);

    // Switch to focus view
    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);

    // Mark reviewed via focus button
    await clickTestId(page, "focus-mark-reviewed");
    await page.waitForTimeout(200);

    const badge = page.getByTestId("focus-status-badge");
    await expect(badge).toContainText("reviewed");
  });
});

test.describe("Rich Editor -- Layout Persistence", () => {
  test("should keep layout mode when interacting with toolbar", async ({ page }) => {
    await openEditorWithBlocks(page);

    // Switch to focus view
    await clickTestId(page, "layout-focus");
    await page.waitForTimeout(300);
    await expect(page.getByTestId("focus-view")).toBeVisible();

    // Pseudo-translate (toolbar action)
    await clickTestId(page, "pseudo-btn");
    await page.waitForTimeout(500);

    // Should still be in focus view
    await expect(page.getByTestId("focus-view")).toBeVisible();
  });
});
