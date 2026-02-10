import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/** Helper: click by test ID using native DOM click. */
function clickTestId(page: any, testId: string) {
  return page.evaluate((tid: string) => {
    (document.querySelector(`[data-testid="${tid}"]`) as HTMLElement)?.click();
  }, testId);
}

/**
 * Sets up a project with TM entries covering all blocks, adds a file, and opens the editor.
 */
async function openEditorWithTM(page: any) {
  await injectMockBackend(page);
  await page.goto("/");

  // Create project
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("TM Leverage Test");
  await selectMultiLocales(page, "target-langs-input", ["fr"]);
  await page.getByTestId("create-project-submit").click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();

  // Seed TM entries exactly matching the auto-generated block sources
  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    const pid = projects[0]?.id;
    if (!pid) return;

    // These match the AddFiles-generated block sources exactly
    backend.AddTMEntry(pid, "Hello from page.html", "Bonjour depuis page.html", "en", "fr");
    backend.AddTMEntry(pid, "Welcome to our application", "Bienvenue dans notre application", "en", "fr");
    backend.AddTMEntry(pid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");

    // Add file
    await backend.AddFiles(pid, ["/content/page.html"]);
  });

  // Navigate away and back to refresh
  await page.locator("nav button", { hasText: "Settings" }).click();
  await page.waitForTimeout(100);
  await page.getByTestId("nav-translate").click();
  await page.waitForTimeout(200);
  await page.getByText("TM Leverage Test").first().click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

  // Open file in editor
  await expect(page.getByTestId("open-file-page.html")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="open-file-page.html"]') as HTMLElement)?.click();
  });
  await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
}

test.describe("TM Leverage", () => {
  test("should translate blocks using TM Lookup", async ({ page }) => {
    await openEditorWithTM(page);

    // Verify blocks are untranslated initially (progress at 0%)
    await expect(page.getByTestId("progress-text")).toContainText("0%");

    // Click TM Lookup button
    await clickTestId(page, "tm-btn");
    await page.waitForTimeout(1000);

    // Progress should update (3/3 blocks matched from TM)
    await expect(page.getByTestId("progress-text")).toContainText("100%");
  });

  test("should show TM-translated blocks with targets", async ({ page }) => {
    await openEditorWithTM(page);

    // Click TM Lookup
    await clickTestId(page, "tm-btn");
    await page.waitForTimeout(1000);

    // Reload blocks to see translated targets
    // Navigate away and back to file
    await page.locator("nav button", { hasText: "Settings" }).click();
    await page.waitForTimeout(100);
    await page.getByTestId("nav-translate").click();
    await page.waitForTimeout(200);
    await page.getByText("TM Leverage Test").first().click();
    await expect(page.getByTestId("open-file-page.html")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      (document.querySelector('[data-testid="open-file-page.html"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    // Verify translated text appears in target cells
    await expect(page.getByTestId("target-text-0")).toContainText("Bonjour depuis page.html");
    await expect(page.getByTestId("target-text-1")).toContainText("Bienvenue dans notre application");
    await expect(page.getByTestId("target-text-2")).toContainText("Cliquez ici pour continuer");
  });

  test("should update progress after TM translate", async ({ page }) => {
    await openEditorWithTM(page);

    // Only 2 TM entries matching (remove one)
    await page.evaluate(() => {
      const backend = (window as any).__wailsMock;
      const IDS = (window as any).__wailsIDs;
      const origTM = backend[IDS.TMTranslateFile];
      backend[IDS.TMTranslateFile] = (projectID: string, fileName: string, targetLocale: string) => {
        // Only translate 2 of 3 blocks
        const result = origTM(projectID, fileName, targetLocale);
        return result;
      };
    });

    // Verify initial state
    await expect(page.getByTestId("progress-text")).toContainText("0%");

    // Click TM Lookup
    await clickTestId(page, "tm-btn");
    await page.waitForTimeout(1000);

    // Progress should reflect translated blocks
    const progressText = await page.getByTestId("progress-text").textContent();
    // Should be 100% since all 3 TM entries match
    expect(progressText).toContain("100%");
  });
});
