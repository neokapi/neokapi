import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/** Helper: set value on an input natively. */
function setInput(page: any, testId: string, value: string) {
  return page.evaluate(({ testId, value }: { testId: string; value: string }) => {
    const input = document.querySelector(`[data-testid="${testId}"]`) as HTMLInputElement;
    if (!input) return;
    const nativeSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    nativeSetter.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  }, { testId, value });
}

/** Helper: click by test ID using native DOM click. */
function clickTestId(page: any, testId: string) {
  return page.evaluate((tid: string) => {
    (document.querySelector(`[data-testid="${tid}"]`) as HTMLElement)?.click();
  }, testId);
}

/**
 * Sets up a project with TM entries and terminology, adds a file, and opens the editor.
 */
async function openEditorWithTMAndTerms(page: any) {
  await setupLocalApp(page);

  // Create project
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("Context Test");
  await selectMultiLocales(page, "target-langs-input", ["fr"]);
  await page.getByTestId("create-project-submit").click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible();

  // Seed TM entries and terminology via mock backend
  await page.evaluate(async () => {
    const backend = (window as any).__wailsMockByName;
    const projects = await backend.ListProjects();
    const pid = projects[0]?.id;
    if (!pid) return;

    // Add TM entries matching block source texts
    backend.AddTMEntry(pid, "Hello from index.html", "Bonjour depuis index.html", "en", "fr");
    backend.AddTMEntry(pid, "Welcome to our application", "Bienvenue dans notre application", "en", "fr");
    backend.AddTMEntry(pid, "Click here to continue", "Cliquez ici pour continuer", "en", "fr");

    // Add terminology concepts
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

    // Add file
    await backend.AddItems(pid, ["/content/index.html"]);
  });

  // Navigate away and back to refresh
  await page.locator("nav button", { hasText: "Settings" }).click();
  await page.waitForTimeout(100);
  await page.getByTestId("nav-translate").click();
  await page.waitForTimeout(200);
  await page.getByText("Context Test").first().click();
  await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });

  // Open file in editor
  await expect(page.getByTestId("open-file-index.html")).toBeVisible({ timeout: 5000 });
  await page.evaluate(() => {
    (document.querySelector('[data-testid="open-file-index.html"]') as HTMLElement)?.click();
  });
  await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
}

test.describe("Context Panel", () => {
  test("should toggle context panel open and closed", async ({ page }) => {
    await openEditorWithTMAndTerms(page);

    // Panel should not be visible initially
    await expect(page.getByTestId("context-panel")).not.toBeVisible();

    // Open context panel
    await clickTestId(page, "context-panel-toggle");
    await expect(page.getByTestId("context-panel")).toBeVisible({ timeout: 5000 });

    // Close context panel
    await clickTestId(page, "context-panel-toggle");
    await expect(page.getByTestId("context-panel")).not.toBeVisible();
  });

  test("should show TM matches for current block", async ({ page }) => {
    await openEditorWithTMAndTerms(page);

    // Select first block
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(100);

    // Open context panel
    await clickTestId(page, "context-panel-toggle");
    await expect(page.getByTestId("context-panel")).toBeVisible({ timeout: 5000 });

    // Wait for matches to load
    await page.waitForTimeout(500);

    // Should show TM match for "Hello from index.html"
    await expect(page.getByTestId("tm-match-0")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("context-panel")).toContainText("Bonjour depuis index.html");
  });

  test("should show term matches for current block", async ({ page }) => {
    await openEditorWithTMAndTerms(page);

    // Select the second block which contains "application"
    await clickTestId(page, "block-row-1");
    await page.waitForTimeout(100);

    // Open context panel
    await clickTestId(page, "context-panel-toggle");
    await expect(page.getByTestId("context-panel")).toBeVisible({ timeout: 5000 });

    // Wait for matches to load
    await page.waitForTimeout(500);

    // Should show term match for "application"
    await expect(page.getByTestId("term-match-0")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("context-panel")).toContainText("application");
  });

  test("should apply TM match to target", async ({ page }) => {
    await openEditorWithTMAndTerms(page);

    // Select second block (untranslated)
    await clickTestId(page, "block-row-1");
    await page.waitForTimeout(100);

    // Open context panel
    await clickTestId(page, "context-panel-toggle");
    await expect(page.getByTestId("context-panel")).toBeVisible({ timeout: 5000 });

    // Wait for TM match to appear
    await expect(page.getByTestId("tm-apply-0")).toBeVisible({ timeout: 5000 });

    // Apply the TM match - target should update
    await clickTestId(page, "tm-apply-0");
    await page.waitForTimeout(500);

    // The target text for block 2 should now contain the TM translation
    await expect(page.getByTestId("target-text-1")).toContainText("Bienvenue dans notre application", { timeout: 5000 });
  });

  test("should update matches when navigating between blocks", async ({ page }) => {
    await openEditorWithTMAndTerms(page);

    // Open context panel
    await clickTestId(page, "context-panel-toggle");
    await expect(page.getByTestId("context-panel")).toBeVisible({ timeout: 5000 });

    // Select first block using native click
    await clickTestId(page, "block-row-0");
    await page.waitForTimeout(500);

    // Should show TM matches for first block
    await expect(page.getByTestId("tm-match-0")).toBeVisible({ timeout: 5000 });

    // Navigate to third block (contains "continue") using native click
    await clickTestId(page, "block-row-2");
    await page.waitForTimeout(500);

    // Should show different matches - term match for "continue"
    await expect(page.getByTestId("term-match-0")).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("context-panel")).toContainText("continue");
  });

  test("should show empty state when no matches", async ({ page }, testInfo) => {
    // Skip in CI - mock backend doesn't resolve loading state properly
    test.skip(!!process.env.CI, "Flaky in CI - mock backend issue");
    await setupLocalApp(page);

    // Create project WITHOUT TM or terms
    await page.getByTestId("new-project-btn").click();
    await page.getByTestId("project-name-input").fill("Empty Context");
    await selectMultiLocales(page, "target-langs-input", ["fr"]);
    await page.getByTestId("create-project-submit").click();
    await expect(page.getByTestId("file-drop-zone")).toBeVisible();

    // Add file
    await page.evaluate(async () => {
      const backend = (window as any).__wailsMockByName;
      const projects = await backend.ListProjects();
      if (projects[0]) await backend.AddItems(projects[0].id, ["/test.html"]);
    });

    // Navigate to editor
    await page.locator("nav button", { hasText: "Settings" }).click();
    await page.waitForTimeout(100);
    await page.getByTestId("nav-translate").click();
    await page.waitForTimeout(200);
    await page.getByText("Empty Context").first().click();
    await expect(page.getByTestId("open-file-test.html")).toBeVisible({ timeout: 5000 });
    await page.evaluate(() => {
      (document.querySelector('[data-testid="open-file-test.html"]') as HTMLElement)?.click();
    });
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });

    // Open context panel
    await clickTestId(page, "context-panel-toggle");
    await expect(page.getByTestId("context-panel")).toBeVisible({ timeout: 5000 });

    // Wait for loading state to finish
    await expect(page.getByTestId("context-panel")).not.toContainText("Loading...", { timeout: 10000 });

    // Should show "no matches" messages
    await expect(page.getByTestId("context-panel")).toContainText("No TM matches");
    await expect(page.getByTestId("context-panel")).toContainText("No terms found");
  });
});
