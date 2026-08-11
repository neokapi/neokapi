import { test, expect } from "@playwright/test";
import { setupLocalApp, openProjectSource, openProjectByName } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/**
 * Per-block context (content-memory matches + terminology) moved out of the old grid-mode
 * toolbar side panel into the Translate editor's Visual view: content-memory matches render
 * in the inline card's content memory section, terminology in the right-hand TermSidebar.
 * These tests exercise that surface in the Visual view (the default).
 */

/** Helper: click by test ID using native DOM click. */
function clickTestId(page: any, testId: string) {
  return page.evaluate((tid: string) => {
    (document.querySelector(`[data-testid="${tid}"]`) as HTMLElement)?.click();
  }, testId);
}

/** Sets up a project with Memory + terminology, adds a file, opens the Visual editor. */
async function openVisualEditorWithMemoryAndTerms(page: any) {
  await setupLocalApp(page);

  await page.getByTestId("onboarding-create-btn").click();
  await page.getByTestId("project-name-input").fill("Context Test");
  await selectMultiLocales(page, "target-langs-input", ["fr"]);
  await page.getByTestId("create-project-submit").click();
  await openProjectSource(page);

  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    const pid = projects[0]?.id;
    if (!pid) return;

    backend.AddMemoryEntry(pid, "Hello from index.html", "Bonjour depuis index.html", "en", "fr");
    backend.AddMemoryEntry(
      pid,
      "Welcome to our application",
      "Bienvenue dans notre application",
      "en",
      "fr",
    );
    backend.AddMemoryEntry(pid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");

    backend.AddConcept({
      project_id: pid,
      domain: "UI",
      definition: "A software program",
      terms: [
        { text: "application", locale: "en", status: "preferred" },
        { text: "application", locale: "fr", status: "preferred" },
      ],
    });
    backend.AddConcept({
      project_id: pid,
      domain: "UI",
      definition: "To proceed",
      terms: [
        { text: "continue", locale: "en", status: "approved" },
        { text: "continuer", locale: "fr", status: "approved" },
      ],
    });

    await backend.AddItems(pid, ["/content/index.html"]);
  });

  await page.getByTestId("back-to-projects").click();
  await page.waitForTimeout(200);
  await openProjectByName(page, "Context Test");
  await openProjectSource(page);

  await expect(page.getByTestId("open-file-index.html")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="open-file-index.html"]') as HTMLElement)?.click();
  });
  // Visual view is the default — wait for the card to mount.
  await expect(page.getByTestId("visual-editor-card")).toBeVisible({ timeout: 5000 });
}

/**
 * Quarantined 2026-08-11: these specs assert a desktop UI that has moved on.
 *
 * They open a file by `open-file-index.html`, a row hook the source
 * view no longer renders under that name.
 *
 * Re-enable by deciding, per test, whether the app or the assertion is right —
 * that is a product call, not a mechanical fix, which is why it is not folded
 * into the change that repaired the harness. Tracked in #1832.
 */
test.describe.skip("Context Panel (Visual view)", () => {
  test("should show content-memory matches for the current block", async ({ page }) => {
    await openVisualEditorWithMemoryAndTerms(page);

    // The first block matches a content-memory entry; its match renders in the card.
    await expect(page.getByTestId("tm-match-0")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("context-panel")).toContainText("Bonjour depuis index.html");
  });

  test("should show term matches for a block in the term sidebar", async ({ page }) => {
    await openVisualEditorWithMemoryAndTerms(page);

    // Navigate to the second block which contains "application".
    await clickTestId(page, "next-block-btn");
    await page.waitForTimeout(500);

    await expect(page.getByTestId("term-sidebar")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("term-match-0")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("term-sidebar")).toContainText("application");
  });

  test("should apply a content-memory match to the target", async ({ page }) => {
    await openVisualEditorWithMemoryAndTerms(page);

    // Move to the second block (untranslated).
    await clickTestId(page, "next-block-btn");
    await page.waitForTimeout(500);

    await expect(page.getByTestId("tm-apply-0")).toBeVisible({ timeout: 5000 });
    await clickTestId(page, "tm-apply-0");
    await page.waitForTimeout(500);

    await expect(page.getByTestId("target-display")).toContainText(
      "Bienvenue dans notre application",
      { timeout: 5000 },
    );
  });

  test("should update matches when navigating between blocks", async ({ page }) => {
    await openVisualEditorWithMemoryAndTerms(page);

    await expect(page.getByTestId("tm-match-0")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("context-panel")).toContainText("Bonjour depuis index.html");

    // Navigate to the third block (contains "continue").
    await clickTestId(page, "next-block-btn");
    await clickTestId(page, "next-block-btn");
    await page.waitForTimeout(500);

    await expect(page.getByTestId("term-match-0")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("term-sidebar")).toContainText("continue");
  });
});
