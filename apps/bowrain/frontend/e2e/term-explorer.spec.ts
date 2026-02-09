import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";

async function createProjectAndOpenTerms(page: any) {
  await injectMockBackend(page);
  await page.goto("/");

  // Create project
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("Terms Test");
  await page.getByTestId("target-langs-input").fill("fr,de");
  await page.getByTestId("create-project-submit").click();

  // Wait for project view
  await expect(page.getByTestId("back-to-projects")).toBeVisible();

  // Open terminology explorer
  await page.evaluate(() => {
    (document.querySelector('[data-testid="open-terms-btn"]') as HTMLElement)?.click();
  });
  await expect(page.getByTestId("term-explorer")).toBeVisible();
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
      const inputs = document.querySelectorAll('[data-testid="term-add-form"] input[placeholder="Term text"]');
      if (inputs[0]) {
        const nativeSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
        nativeSetter.call(inputs[0], "database");
        inputs[0].dispatchEvent(new Event("input", { bubbles: true }));
        inputs[0].dispatchEvent(new Event("change", { bubbles: true }));
      }
    });

    // Fill in the second term (target)
    await page.evaluate(() => {
      const inputs = document.querySelectorAll('[data-testid="term-add-form"] input[placeholder="Term text"]');
      if (inputs[1]) {
        const nativeSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
        nativeSetter.call(inputs[1], "base de données");
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
    await createProjectAndOpenTerms(page);

    // Add two concepts via mock backend
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

    // Type in search box
    await setInput(page, "term-search-input", "database");

    // Wait for debounce and results
    await page.waitForTimeout(500);

    // Should show only 1 concept
    await expect(page.getByTestId("term-count-badge")).toContainText("1 concept");
    await expect(page.getByText("database")).toBeVisible();
  });

  test("should filter by target locale", async ({ page }) => {
    await createProjectAndOpenTerms(page);

    // Add concepts with different locales
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

    // Trigger re-fetch
    await setInput(page, "term-search-input", " ");
    await page.waitForTimeout(400);
    await setInput(page, "term-search-input", "");
    await page.waitForTimeout(400);

    // Should show 2 concepts initially
    await expect(page.getByTestId("term-count-badge")).toContainText("2 concepts");

    // Select "de" from the target locale filter
    await page.evaluate(() => {
      const select = document.querySelector('[data-testid="term-target-locale-filter"]') as HTMLSelectElement;
      if (select) {
        const nativeSetter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!;
        nativeSetter.call(select, "de");
        select.dispatchEvent(new Event("change", { bubbles: true }));
      }
    });
    await page.waitForTimeout(400);

    // Should show only 1 concept
    await expect(page.getByTestId("term-count-badge")).toContainText("1 concept");
  });

  test("should edit a concept", async ({ page }) => {
    await createProjectAndOpenTerms(page);

    // Add a concept
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

    // Trigger refresh
    await setInput(page, "term-search-input", " ");
    await page.waitForTimeout(400);
    await setInput(page, "term-search-input", "");
    await page.waitForTimeout(400);

    // Click Edit button
    await clickFirst(page, "term-edit-btn-");

    // Edit the domain
    await page.evaluate(() => {
      const row = document.querySelector('[data-testid^="term-concept-"]');
      if (!row) return;
      const inputs = row.querySelectorAll("input");
      // First input is domain
      if (inputs[0]) {
        const nativeSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
        nativeSetter.call(inputs[0], "Healthcare");
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
    await createProjectAndOpenTerms(page);

    // Add a concept
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddConcept({
          project_id: pid,
          domain: "Test",
          terms: [
            { text: "delete me", locale: "en", status: "proposed" },
          ],
        });
      }
    });

    // Trigger refresh
    await setInput(page, "term-search-input", " ");
    await page.waitForTimeout(400);
    await setInput(page, "term-search-input", "");
    await page.waitForTimeout(400);

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
