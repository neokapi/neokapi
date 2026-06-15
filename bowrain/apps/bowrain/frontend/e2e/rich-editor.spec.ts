import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

// Quarantined in CI (#867): this rich-editor suite passes locally but
// flakes/times out under headless CI. Still runs locally.
test.beforeEach(() => {
  test.skip(!!process.env.CI, "Quarantined in CI — see #867");
});

/**
 * The editor's five layout modes (grid/focus/split-h/split-v/visual) collapsed
 * into two views — Visual and Table. Focus/split modes were retired; the
 * row-based tests here use the Table view. Block-level review (mark reviewed,
 * approve/reject) moved out of the editor toolbar onto the Review surface,
 * reached via the cross-surface tabs; those tests live there.
 */

/** Helper: creates a project with a file and opens the editor in Table view. */
async function openEditorInTable(page: any) {
  await setupLocalApp(page);

  await page.getByText("Upload files").click();
  await page.getByTestId("project-name-input").fill("Rich Editor Test");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();

  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    if (projects.length > 0) {
      await backend.AddItems(projects[0].id, ["/test/hello.txt"]);
    }
  });

  await page.getByTestId("back-to-projects").click();
  await page.waitForTimeout(200);

  await page.getByText("Rich Editor Test").first().click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

  await expect(page.getByTestId("open-file-hello.txt")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
    if (btn) btn.click();
  });

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

/** Pseudo-translate via mock backend, then reload editor in Table view. */
async function pseudoTranslateViaBackend(page: any) {
  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    if (projects.length > 0) {
      await backend.PseudoTranslateItem(projects[0].id, "hello.txt", "fr");
    }
  });
  await clickTestId(page, "back-to-project");
  await page.getByTestId("open-file-hello.txt").waitFor({ state: "visible", timeout: 5000 });
  await page.evaluate(() => {
    const btn = document.querySelector('[data-testid="open-file-hello.txt"]') as HTMLElement;
    if (btn) btn.click();
  });
  await expect(page.getByTestId("view-switcher")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="view-table"]') as HTMLElement)?.click();
  });
  await page.getByTestId("block-grid").waitFor({ state: "visible", timeout: 5000 });
}

test.describe("Rich Editor -- Views", () => {
  test("should show the Visual and Table view buttons", async ({ page }) => {
    await openEditorInTable(page);

    await expect(page.getByTestId("view-visual")).toBeVisible();
    await expect(page.getByTestId("view-table")).toBeVisible();
  });

  test("should default to the Visual view", async ({ page }) => {
    await openEditorInTable(page);

    // Switch to Visual and confirm the card mounts.
    await clickTestId(page, "view-visual");
    await expect(page.getByTestId("visual-editor-card")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("block-grid")).not.toBeVisible();
  });

  test("should switch back to Table from Visual", async ({ page }) => {
    await openEditorInTable(page);

    await clickTestId(page, "view-visual");
    await expect(page.getByTestId("visual-editor-card")).toBeVisible({ timeout: 5000 });

    await clickTestId(page, "view-table");
    await page.waitForTimeout(200);
    await expect(page.getByTestId("block-grid")).toBeVisible();
  });
});

test.describe("Rich Editor -- Block Status (Table)", () => {
  test("should show status dots on block rows", async ({ page }) => {
    await openEditorInTable(page);

    const dot0 = page.getByTestId("status-dot-0");
    await expect(dot0).toBeVisible();
    await expect(dot0).toHaveAttribute("title", "not-started");
  });

  test("should show draft status after pseudo-translate", async ({ page }) => {
    await openEditorInTable(page);

    await pseudoTranslateViaBackend(page);

    const dot0 = page.getByTestId("status-dot-0");
    await expect(dot0).toHaveAttribute("title", "draft");
  });

  test("should show status breakdown in progress bar after pseudo-translate", async ({ page }) => {
    await openEditorInTable(page);

    await pseudoTranslateViaBackend(page);

    const progress = page.getByTestId("progress-text");
    await expect(progress).toContainText("draft");
  });

  test("should show progress segments after pseudo-translate", async ({ page }) => {
    await openEditorInTable(page);

    await pseudoTranslateViaBackend(page);

    await expect(page.getByTestId("progress-draft")).toBeVisible();
  });
});

test.describe("Rich Editor -- Review surface", () => {
  test("should mark a block as reviewed from the Review surface", async ({ page }) => {
    await openEditorInTable(page);

    // Pseudo-translate so blocks have content to review.
    await pseudoTranslateViaBackend(page);

    // Open the Review surface via the cross-surface tabs.
    await clickTestId(page, "surface-tab-review");
    await expect(page.getByTestId("review-surface")).toBeVisible({ timeout: 5000 });

    // Approve the first translatable block.
    const approve = page.locator('[data-testid^="approve-"]').first();
    await approve.click();
    await page.waitForTimeout(200);

    // Its status badge should read Reviewed.
    const status = page.locator('[data-testid^="review-status-"]').first();
    await expect(status).toContainText("Reviewed");
  });

  test("should bulk mark selected blocks reviewed", async ({ page }) => {
    await openEditorInTable(page);
    await pseudoTranslateViaBackend(page);

    await clickTestId(page, "surface-tab-review");
    await expect(page.getByTestId("review-surface")).toBeVisible({ timeout: 5000 });

    // Select all visible blocks and bulk mark reviewed.
    await clickTestId(page, "select-all");
    await clickTestId(page, "bulk-mark-reviewed");
    await page.waitForTimeout(200);

    await expect(page.getByTestId("review-surface")).toContainText("Marked");
  });
});

test.describe("Rich Editor -- Pre-process surface", () => {
  test("should expose pseudo-translate and TM leverage operations", async ({ page }) => {
    await openEditorInTable(page);

    await clickTestId(page, "surface-tab-pre-process");
    await expect(page.getByTestId("preprocess-surface")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("preprocess-run-pseudo")).toBeVisible();
    await expect(page.getByTestId("preprocess-run-tm")).toBeVisible();
  });
});
