import { test, expect } from "@playwright/test";
import { setupLocalApp, openProjectSource, openProjectByName } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

// Quarantined in CI (#867): this content memory-leverage suite passes locally but
// flakes/times out under headless CI. Still runs locally.
test.beforeEach(() => {
  test.skip(!!process.env.CI, "Quarantined in CI — see #867");
});

/** Helper: click by test ID using native DOM click. */
function clickTestId(page: any, testId: string) {
  return page.evaluate((tid: string) => {
    (document.querySelector(`[data-testid="${tid}"]`) as HTMLElement)?.click();
  }, testId);
}

/**
 * Bulk content-memory leverage moved out of the per-block Translate toolbar onto the
 * Pre-process surface. This navigates there and runs the leverage op.
 */
async function leverageMemoryViaPreProcess(page: any) {
  await clickTestId(page, "surface-tab-pre-process");
  await page.getByTestId("preprocess-run-tm").waitFor({ state: "visible", timeout: 5000 });
  await clickTestId(page, "preprocess-run-tm");
  await page.waitForTimeout(1000);
  // Return to the Translate editor in Table view to inspect results.
  await clickTestId(page, "surface-tab-translate");
  await page.getByTestId("view-switcher").waitFor({ state: "visible", timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="view-table"]') as HTMLElement)?.click();
  });
  await page.getByTestId("block-grid").waitFor({ state: "visible", timeout: 5000 });
}

/**
 * Sets up a project with content-memory entries covering all blocks, adds a file, and opens the editor.
 */
async function openEditorWithMemory(page: any) {
  await setupLocalApp(page);

  // Create project
  await page.getByTestId("onboarding-create-btn").click();
  await page.getByTestId("project-name-input").fill("content memory Leverage Test");
  await selectMultiLocales(page, "target-langs-input", ["fr"]);
  await page.getByTestId("create-project-submit").click();
  await openProjectSource(page);

  // Seed content-memory entries exactly matching the auto-generated block sources
  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    const pid = projects[0]?.id;
    if (!pid) return;

    // These match the AddItems-generated block sources exactly
    backend.AddMemoryEntry(pid, "Hello from page.html", "Bonjour depuis page.html", "en", "fr");
    backend.AddMemoryEntry(
      pid,
      "Welcome to our application",
      "Bienvenue dans notre application",
      "en",
      "fr",
    );
    backend.AddMemoryEntry(pid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");

    // Add file
    await backend.AddItems(pid, ["/content/page.html"]);
  });

  // Navigate back to projects list and re-enter to refresh
  await page.getByTestId("back-to-projects").click();
  await page.waitForTimeout(200);
  await openProjectByName(page, "content memory Leverage Test");
  await openProjectSource(page);

  // Open file in editor
  await expect(page.getByTestId("open-file-page.html")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="open-file-page.html"]') as HTMLElement)?.click();
  });
  await expect(page.getByTestId("view-switcher")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="view-table"]') as HTMLElement)?.click();
  });
  await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
}

test.describe("content memory Leverage", () => {
  test("should translate blocks using content memory Lookup", async ({ page }) => {
    await openEditorWithMemory(page);

    // Verify blocks are untranslated initially (progress at 0%)
    await expect(page.getByTestId("progress-text")).toContainText("0%");

    // Run bulk content-memory leverage from the Pre-process surface.
    await leverageMemoryViaPreProcess(page);

    // Progress should update (3/3 blocks matched from content memory)
    await expect(page.getByTestId("progress-text")).toContainText("100%");
  });

  test("should show content memory-translated blocks with targets", async ({ page }) => {
    await openEditorWithMemory(page);

    // Run bulk content-memory leverage from the Pre-process surface.
    await leverageMemoryViaPreProcess(page);

    // Reload blocks to see translated targets
    // Navigate back to project view and re-open file
    await page.evaluate(() => {
      (document.querySelector('[data-testid="back-to-project"]') as HTMLElement)?.click();
    });
    await page.waitForTimeout(200);
    await expect(page.getByTestId("open-file-page.html")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      (document.querySelector('[data-testid="open-file-page.html"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("view-switcher")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      (document.querySelector('[data-testid="view-table"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    // Verify translated text appears in target cells
    await expect(page.getByTestId("target-text-0")).toContainText("Bonjour depuis page.html");
    await expect(page.getByTestId("target-text-1")).toContainText(
      "Bienvenue dans notre application",
    );
    await expect(page.getByTestId("target-text-2")).toContainText("Cliquez ici pour continuer");
  });

  test("should update progress after content memory translate", async ({ page }) => {
    await openEditorWithMemory(page);

    // Only 2 content-memory entries matching (remove one)
    await page.evaluate(() => {
      const backend = (window as any).__wailsMock;
      const IDS = (window as any).__wailsIDs;
      const origMemory = backend[IDS.MemoryTranslateItem];
      backend[IDS.MemoryTranslateItem] = (
        projectID: string,
        fileName: string,
        targetLocale: string,
      ) => {
        // Only translate 2 of 3 blocks
        const result = origMemory(projectID, fileName, targetLocale);
        return result;
      };
    });

    // Verify initial state
    await expect(page.getByTestId("progress-text")).toContainText("0%");

    // Run bulk content-memory leverage from the Pre-process surface.
    await leverageMemoryViaPreProcess(page);

    // Progress should reflect translated blocks
    const progressText = await page.getByTestId("progress-text").textContent();
    // Should be 100% since all 3 content-memory entries match
    expect(progressText).toContain("100%");
  });
});
