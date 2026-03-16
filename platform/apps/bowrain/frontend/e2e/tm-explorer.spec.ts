import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/** Creates a project and returns with the project view visible (before opening TM explorer). */
async function createProject(page: any) {
  await setupLocalApp(page);

  // Create project
  await page.getByText("Upload files").click();
  await page.getByTestId("project-name-input").fill("TM Test");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();

  // Wait for project view
  await expect(page.getByTestId("back-to-projects")).toBeVisible();
}

/** Opens the TM explorer from the project view. */
async function openTM(page: any) {
  await page.evaluate(() => {
    (document.querySelector('[data-testid="open-tm-btn"]') as HTMLElement)?.click();
  });
  await expect(page.getByTestId("tm-explorer")).toBeVisible();
}

/** Creates a project and opens the TM explorer. */
async function createProjectAndOpenTM(page: any) {
  await createProject(page);
  await openTM(page);
}

/** Helper: set value on an input by test ID (handles both direct inputs and wrappers). */
async function setInput(page: any, testId: string, value: string) {
  const wrapper = page.getByTestId(testId);
  // If the testid is directly on an <input>, use it; otherwise find a child input.
  const tagName = await wrapper.evaluate((el: Element) => el.tagName.toLowerCase());
  const input = tagName === "input" ? wrapper : wrapper.locator("input").first();
  await input.clear();
  await input.fill(value);
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

    // Fill in the form
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
    await createProject(page);

    // Add two entries via mock backend before opening TM explorer
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddTMEntry(pid, "Hello World", "Bonjour le monde", "en", "fr");
        backend.AddTMEntry(pid, "Goodbye", "Au revoir", "en", "fr");
      }
    });

    await openTM(page);

    // Verify both entries are shown initially
    await expect(page.getByTestId("tm-count-badge")).toContainText("2 entries");

    // Type in search box and press Enter to commit the search
    const searchInput = page.getByTestId("tm-search-input").locator("input").first();
    await searchInput.fill("Hello");
    await searchInput.press("Enter");

    // Wait for results
    await page.waitForTimeout(500);

    // Should show only 1 entry (the "Hello World" one)
    await expect(page.getByTestId("tm-count-badge")).toContainText("1 entry");
  });

  test("should filter entries by target locale", async ({ page }) => {
    await createProject(page);

    // Add entries in different locales before opening TM explorer
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddTMEntry(pid, "Hello", "Bonjour", "en", "fr");
        backend.AddTMEntry(pid, "Hello", "Hallo", "en", "de");
      }
    });

    await openTM(page);

    // Should show 2 entries initially
    await expect(page.getByTestId("tm-count-badge")).toContainText("2 entries");

    // Select "de" from the target locale filter using the FilterBar:
    // type "target:de" into the search input and press Enter
    const searchInput = page.getByTestId("tm-search-input").locator("input").first();
    await searchInput.fill("target:de");
    await searchInput.press("Enter");
    await page.waitForTimeout(400);

    // Should show only 1 entry
    await expect(page.getByTestId("tm-count-badge")).toContainText("1 entry");
  });

  test("should edit an entry target", async ({ page }) => {
    await createProject(page);

    // Add an entry before opening TM explorer
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddTMEntry(pid, "Hello", "Bonjour", "en", "fr");
      }
    });

    await openTM(page);

    // Click Edit button using evaluate to avoid Playwright click hangs
    await clickFirst(page, "tm-edit-btn-");

    // Wait for edit input to appear
    const editInput = page.locator('[data-testid^="tm-edit-input-"]').first();
    await expect(editInput).toBeVisible({ timeout: 5000 });

    // Set new value and save
    await page.evaluate(() => {
      const input = document.querySelector('[data-testid^="tm-edit-input-"]') as HTMLInputElement;
      if (input) {
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(
          input,
          "Salut",
        );
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
    await createProject(page);

    // Add an entry before opening TM explorer
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddTMEntry(pid, "Hello", "Bonjour", "en", "fr");
      }
    });

    await openTM(page);

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
