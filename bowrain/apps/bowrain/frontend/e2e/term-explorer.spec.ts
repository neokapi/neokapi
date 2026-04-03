import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/** Creates a project and returns with the project view visible (before opening term explorer). */
async function createProject(page: any) {
  await setupLocalApp(page);

  // Create project
  await page.getByText("Upload files").click();
  await page.getByTestId("project-name-input").fill("Terms Test");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();

  // Wait for project view
  await expect(page.getByTestId("back-to-projects")).toBeVisible();
}

/** Opens the terminology explorer from the project view. */
async function openTerms(page: any) {
  await page.evaluate(() => {
    (document.querySelector('[data-testid="open-terms-btn"]') as HTMLElement)?.click();
  });
  await expect(page.getByTestId("term-explorer")).toBeVisible();
}

/** Creates a project and opens the term explorer. */
async function createProjectAndOpenTerms(page: any) {
  await createProject(page);
  await openTerms(page);
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

test.describe("Term Explorer", () => {
  test("should open term explorer and show empty state", async ({ page }) => {
    await createProjectAndOpenTerms(page);

    await expect(page.getByTestId("term-empty-state")).toBeVisible();
    await expect(page.getByTestId("term-count-badge")).toContainText("0 concepts");
  });

  test("should add a concept and see it in the list", async ({ page }) => {
    await createProjectAndOpenTerms(page);

    // Click Add Concept
    await clickTestId(page, "term-add-btn");
    await expect(page.getByTestId("term-add-form")).toBeVisible();

    // Fill in domain and definition
    await setInput(page, "term-add-domain", "IT");
    await setInput(page, "term-add-definition", "A computer term");

    // Fill in the first term (source)
    await page.evaluate(() => {
      const inputs = document.querySelectorAll(
        '[data-testid="term-add-form"] input[placeholder="Term text"]',
      );
      if (inputs[0]) {
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(
          inputs[0],
          "database",
        );
        inputs[0].dispatchEvent(new Event("input", { bubbles: true }));
        inputs[0].dispatchEvent(new Event("change", { bubbles: true }));
      }
    });

    // Fill in the second term (target)
    await page.evaluate(() => {
      const inputs = document.querySelectorAll(
        '[data-testid="term-add-form"] input[placeholder="Term text"]',
      );
      if (inputs[1]) {
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(
          inputs[1],
          "base de données",
        );
        inputs[1].dispatchEvent(new Event("input", { bubbles: true }));
        inputs[1].dispatchEvent(new Event("change", { bubbles: true }));
      }
    });

    // Submit
    await clickTestId(page, "term-add-submit");

    // Wait for list to update
    await expect(page.getByTestId("term-empty-state")).not.toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId("term-count-badge")).toContainText("1 concept");

    // Verify concept content
    await expect(page.getByText("database")).toBeVisible();
    await expect(page.getByText("base de données")).toBeVisible();
    await expect(page.getByText("IT", { exact: true })).toBeVisible();
  });

  test("should search concepts by text", async ({ page }) => {
    await createProject(page);

    // Add two concepts via mock backend before opening term explorer
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddConcept({
          project_id: pid,
          domain: "IT",
          definition: "Data storage",
          terms: [
            { text: "database", locale: "en", status: "preferred" },
            { text: "base de données", locale: "fr", status: "preferred" },
          ],
        });
        backend.AddConcept({
          project_id: pid,
          domain: "IT",
          definition: "Network connection",
          terms: [
            { text: "firewall", locale: "en", status: "preferred" },
            { text: "pare-feu", locale: "fr", status: "preferred" },
          ],
        });
      }
    });

    await openTerms(page);

    // Verify both concepts are shown initially
    await expect(page.getByTestId("term-count-badge")).toContainText("2 concepts");

    // Type in search box and press Enter to commit the search
    const searchInput = page.getByTestId("term-search-input").locator("input").first();
    await searchInput.fill("database");
    await searchInput.press("Enter");

    // Wait for results
    await page.waitForTimeout(500);

    // Should show only 1 concept
    await expect(page.getByTestId("term-count-badge")).toContainText("1 concept");
    // Use a scoped locator to avoid matching the search badge
    await expect(
      page.locator('[data-testid^="term-concept-"]').getByText("database"),
    ).toBeVisible();
  });

  test("should filter by target locale", async ({ page }) => {
    await createProject(page);

    // Add concepts with different locales before opening term explorer
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddConcept({
          project_id: pid,
          domain: "IT",
          terms: [
            { text: "server", locale: "en", status: "preferred" },
            { text: "serveur", locale: "fr", status: "preferred" },
          ],
        });
        backend.AddConcept({
          project_id: pid,
          domain: "IT",
          terms: [
            { text: "network", locale: "en", status: "preferred" },
            { text: "Netzwerk", locale: "de", status: "preferred" },
          ],
        });
      }
    });

    await openTerms(page);

    // Should show 2 concepts initially
    await expect(page.getByTestId("term-count-badge")).toContainText("2 concepts");

    // Select "de" from the target locale filter using the FilterBar:
    // type "target:de" into the search input and press Enter
    const searchInput = page.getByTestId("term-search-input").locator("input").first();
    await searchInput.fill("target:de");
    await searchInput.press("Enter");
    await page.waitForTimeout(400);

    // Should show only 1 concept
    await expect(page.getByTestId("term-count-badge")).toContainText("1 concept");
  });

  test("should edit a concept", async ({ page }) => {
    await createProject(page);

    // Add a concept before opening term explorer
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddConcept({
          project_id: pid,
          domain: "Medical",
          terms: [
            { text: "patient", locale: "en", status: "preferred" },
            { text: "patient", locale: "fr", status: "preferred" },
          ],
        });
      }
    });

    await openTerms(page);

    // Click Edit button
    await clickFirst(page, "term-edit-btn-");

    // Edit the domain
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid^="term-concept-"]');
      if (!row) return;
      const inputs = row.querySelectorAll("input");
      // First input is domain
      if (inputs[0]) {
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(
          inputs[0],
          "Healthcare",
        );
        inputs[0].dispatchEvent(new Event("input", { bubbles: true }));
        inputs[0].dispatchEvent(new Event("change", { bubbles: true }));
      }
    });

    // Click Save
    await clickFirst(page, "term-save-btn-");
    await page.waitForTimeout(500);

    // Verify updated domain
    await expect(page.getByText("Healthcare")).toBeVisible();
  });

  test("should delete a concept with confirmation", async ({ page }) => {
    await createProject(page);

    // Add a concept before opening term explorer
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddConcept({
          project_id: pid,
          domain: "Test",
          terms: [{ text: "delete me", locale: "en", status: "proposed" }],
        });
      }
    });

    await openTerms(page);

    // Verify concept is present
    await expect(page.getByTestId("term-count-badge")).toContainText("1 concept");

    // Click Delete button
    await clickFirst(page, "term-delete-btn-");

    // Confirm deletion
    await clickFirst(page, "term-confirm-delete-");
    await page.waitForTimeout(500);

    // Should show empty state
    await expect(page.getByTestId("term-count-badge")).toContainText("0 concepts");
    await expect(page.getByTestId("term-empty-state")).toBeVisible();
  });

  test("should navigate back to project view", async ({ page }) => {
    await createProjectAndOpenTerms(page);

    // Click back button
    await clickTestId(page, "term-back-btn");

    // Should be back at project view
    await expect(page.getByTestId("back-to-projects")).toBeVisible();
    await expect(page.getByTestId("open-terms-btn")).toBeVisible();
  });
});
