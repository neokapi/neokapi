import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

async function createProjectAndOpenTM(page: any) {
  await setupLocalApp(page);

  // Create project
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("TM Test");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();

  // Wait for project view
  await expect(page.getByTestId("back-to-projects")).toBeVisible();

  // Open TM explorer
  await page.evaluate(() => {
    (document.querySelector('[data-testid="open-tm-btn"]') as HTMLElement)?.click();
  });
  await expect(page.getByTestId("tm-explorer")).toBeVisible();
}

/** Helper: set value on an input natively (avoids Playwright fill hangs). */
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

/** Helper: click first element matching a selector prefix. */
function clickFirst(page: any, selectorPrefix: string) {
  return page.evaluate((prefix: string) => {
    (document.querySelector(`[data-testid^="${prefix}"]`) as HTMLElement)?.click();
  }, selectorPrefix);
}

test.describe("TM Explorer", () => {
  test("should open TM explorer and show empty state", async ({ page }) => {
    await createProjectAndOpenTM(page);

    await expect(page.getByTestId("tm-empty-state")).toBeVisible();
    await expect(page.getByTestId("tm-count-badge")).toContainText("0 entries");
  });

  test("should add an entry and see it in the list", async ({ page }) => {
    await createProjectAndOpenTM(page);

    // Click Add Entry
    await clickTestId(page, "tm-add-entry-btn");
    await expect(page.getByTestId("tm-add-form")).toBeVisible();

    // Fill in the form using native setters
    await setInput(page, "tm-add-source-input", "Hello");
    await setInput(page, "tm-add-target-input", "Bonjour");

    // Submit
    await clickTestId(page, "tm-add-submit");

    // Wait for list to update
    await expect(page.getByTestId("tm-empty-state")).not.toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("tm-count-badge")).toContainText("1 entry");

    // Verify entry content (use td selector to avoid matching filter dropdowns)
    const sourceCell = page.locator('td[data-testid^="tm-source-"]').first();
    await expect(sourceCell).toContainText("Hello");
    const targetCell = page.locator('td[data-testid^="tm-target-"]').first();
    await expect(targetCell).toContainText("Bonjour");
  });

  test("should search entries by text", async ({ page }) => {
    await createProjectAndOpenTM(page);

    // Add two entries via mock backend
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddTMEntry(pid, "Hello World", "Bonjour le monde", "en", "fr");
        backend.AddTMEntry(pid, "Goodbye", "Au revoir", "en", "fr");
      }
    });

    // Type in search box using native setter
    await setInput(page, "tm-search-input", "Hello");

    // Wait for debounce and results
    await page.waitForTimeout(500);

    // Should show only 1 entry (the "Hello World" one)
    await expect(page.getByTestId("tm-count-badge")).toContainText("1 entry");
  });

  test("should filter entries by target locale", async ({ page }) => {
    await createProjectAndOpenTM(page);

    // Add entries in different locales via mock backend
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddTMEntry(pid, "Hello", "Bonjour", "en", "fr");
        backend.AddTMEntry(pid, "Hello", "Hallo", "en", "de");
      }
    });

    // Trigger re-fetch by searching with a space then clearing
    await setInput(page, "tm-search-input", " ");
    await page.waitForTimeout(400);
    await setInput(page, "tm-search-input", "");
    await page.waitForTimeout(400);

    // Should show 2 entries initially
    await expect(page.getByTestId("tm-count-badge")).toContainText("2 entries");

    // Select "de" from the target locale filter using native DOM
    await page.evaluate(() => {
      const select = document.querySelector('[data-testid="tm-target-locale-filter"]') as HTMLSelectElement;
      if (select) {
        const nativeSetter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!;
        nativeSetter.call(select, "de");
        select.dispatchEvent(new Event("change", { bubbles: true }));
      }
    });
    await page.waitForTimeout(400);

    // Should show only 1 entry
    await expect(page.getByTestId("tm-count-badge")).toContainText("1 entry");
  });

  test("should edit an entry target", async ({ page }) => {
    await createProjectAndOpenTM(page);

    // Add an entry
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddTMEntry(pid, "Hello", "Bonjour", "en", "fr");
      }
    });

    // Trigger refresh
    await setInput(page, "tm-search-input", " ");
    await page.waitForTimeout(400);
    await setInput(page, "tm-search-input", "");
    await page.waitForTimeout(400);

    // Click Edit button using evaluate to avoid Playwright click hangs
    await clickFirst(page, "tm-edit-btn-");

    // Wait for edit input to appear
    const editInput = page.locator('[data-testid^="tm-edit-input-"]').first();
    await expect(editInput).toBeVisible({ timeout: 5000 });

    // Set new value and save
    await page.evaluate(() => {
      const input = document.querySelector('[data-testid^="tm-edit-input-"]') as HTMLInputElement;
      if (input) {
        const nativeSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
        nativeSetter.call(input, "Salut");
        input.dispatchEvent(new Event("input", { bubbles: true }));
        input.dispatchEvent(new Event("change", { bubbles: true }));
      }
    });

    // Click Save
    await clickFirst(page, "tm-save-btn-");

    // Wait for update
    await page.waitForTimeout(500);

    // Verify updated target (use td selector to avoid matching filter dropdowns)
    const targetCell = page.locator('td[data-testid^="tm-target-"]').first();
    await expect(targetCell).toContainText("Salut");
  });

  test("should delete an entry", async ({ page }) => {
    await createProjectAndOpenTM(page);

    // Add an entry
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddTMEntry(pid, "Hello", "Bonjour", "en", "fr");
      }
    });

    // Trigger refresh
    await setInput(page, "tm-search-input", " ");
    await page.waitForTimeout(400);
    await setInput(page, "tm-search-input", "");
    await page.waitForTimeout(400);

    // Verify entry is present
    await expect(page.getByTestId("tm-count-badge")).toContainText("1 entry");

    // Click Delete using evaluate
    await clickFirst(page, "tm-delete-btn-");

    // Wait for update
    await page.waitForTimeout(500);

    // Should show empty state
    await expect(page.getByTestId("tm-count-badge")).toContainText("0 entries");
    await expect(page.getByTestId("tm-empty-state")).toBeVisible();
  });

  test("should navigate back to project view", async ({ page }) => {
    await createProjectAndOpenTM(page);

    // Click back button
    await clickTestId(page, "tm-back-btn");

    // Should be back at project view
    await expect(page.getByTestId("back-to-projects")).toBeVisible();
    await expect(page.getByTestId("open-tm-btn")).toBeVisible();
  });
});
