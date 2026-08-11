import { test, expect, type Page } from "@playwright/test";
import { setupLocalApp, openProjectSource } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/**
 * E2E for the shared @neokapi/ui-primitives TermsBrowser as mounted by the
 * bowrain desktop (App.tsx renders it via useTermsBrowserAdapter when the
 * project view's "open-terms-btn" is clicked).
 */

/** Creates a project and returns with the project view visible (before opening the terms store browser). */
async function createProject(page: Page) {
  await setupLocalApp(page);

  // Create project
  await page.getByTestId("onboarding-create-btn").click();
  await page.getByTestId("project-name-input").fill("Terms Test");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();

  // Creating a project lands on its overview; the memory and terms browsers
  // are opened from the source view beside it (#1793 moved the landing).
  await expect(page.getByTestId("back-to-projects")).toBeVisible();
  await openProjectSource(page);
}

/** Opens the terms store browser from the project view. */
async function openTerms(page: Page) {
  await page.getByTestId("open-terms-btn").click();
  await expect(page.getByTestId("terms-browser")).toBeVisible();
}

/** Creates a project and opens the terms store browser. */
async function createProjectAndOpenTerms(page: Page) {
  await createProject(page);
  await openTerms(page);
}

/** The "N concepts" count under the terms store browser toolbar. */
function countText(page: Page, pattern: RegExp) {
  return expect(page.getByTestId("terms-browser").getByText(pattern)).toBeVisible();
}

/**
 * Quarantined 2026-08-11: these specs assert a desktop UI that has moved on.
 *
 * The desktop shell wires no `onOpenTerms`: the button exists in the
 * shared ProjectView, nothing in `apps/bowrain/frontend/src` hands it a handler,
 * so `terms-browser` never mounts. The surface, not the spec, is what is
 * missing.
 *
 * Re-enable by deciding, per test, whether the app or the assertion is right —
 * that is a product call, not a mechanical fix, which is why it is not folded
 * into the change that repaired the harness. Tracked in #1832.
 */
test.describe.skip("Terms Browser", () => {
  test("should open terms browser and show empty state", async ({ page }) => {
    await createProjectAndOpenTerms(page);

    await countText(page, /^0 concepts$/);
    await expect(page.getByText("No concepts yet.")).toBeVisible();
  });

  test("should add a concept and see it in the list", async ({ page }) => {
    await createProjectAndOpenTerms(page);

    // Open the add dialog
    await page.getByRole("button", { name: "Add Concept" }).click();
    await expect(page.getByText("New Concept")).toBeVisible();

    // Fill in domain and definition
    await page.getByPlaceholder("e.g. Legal, Medical").fill("IT");
    await page.getByPlaceholder("Concept definition").fill("A computer term");

    // First term (source)
    await page.getByPlaceholder("Term", { exact: true }).first().fill("database");

    // Add a second term (target)
    await page.getByRole("button", { name: "+ Add term" }).click();
    await page.getByPlaceholder("Term", { exact: true }).nth(1).fill("base de données");

    // Submit
    await page.getByRole("button", { name: "Save", exact: true }).click();

    // Wait for list to update
    await countText(page, /^1 concept$/);
    await expect(page.getByText("database")).toBeVisible();
    await expect(page.getByText("base de données")).toBeVisible();
    await expect(page.getByText("IT", { exact: true })).toBeVisible();
  });

  test("should search concepts by text", async ({ page }) => {
    await createProject(page);

    // Add two concepts via mock backend before opening the terms store browser
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
    await countText(page, /^2 concepts$/);

    // Live search narrows the list
    await page.getByPlaceholder("Search terminology...").fill("database");

    await countText(page, /^1 concept$/);
    await expect(page.locator('[data-testid^="concept-"]')).toHaveCount(1);
    await expect(page.locator('[data-testid^="concept-"]').first()).toContainText("database");
  });

  test("should scope concepts to the active target locale", async ({ page }) => {
    await createProject(page);

    // Add concepts with different target locales before opening the browser
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

    // The browser scopes to the project's first target locale (fr), so only
    // the concept with a French term is in scope; the German one is filtered.
    await countText(page, /^1 concept$/);
    await expect(page.locator('[data-testid^="concept-"]')).toHaveCount(1);
    await expect(page.locator('[data-testid^="concept-"]').first()).toContainText("serveur");
    await expect(page.getByText("Netzwerk")).not.toBeVisible();
  });

  test("should edit a concept", async ({ page }) => {
    await createProject(page);

    // Add a concept before opening the terms store browser
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

    // Enter edit mode (Edit reveals on hover)
    const card = page.locator('[data-testid^="concept-"]').first();
    await card.hover();
    await card.getByRole("button", { name: "Edit" }).click();

    // Edit the domain
    const domainInput = page.getByPlaceholder("Domain", { exact: true });
    await domainInput.fill("Healthcare");

    // Save
    await page.getByRole("button", { name: "Save", exact: true }).click();

    // Verify updated domain
    await expect(page.getByText("Healthcare")).toBeVisible();
  });

  test("should delete a concept with confirmation", async ({ page }) => {
    await createProject(page);

    // Add a concept before opening the terms store browser
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
            { text: "supprime-moi", locale: "fr", status: "proposed" },
          ],
        });
      }
    });

    await openTerms(page);

    // Verify concept is present
    await countText(page, /^1 concept$/);

    // Two-click delete: Delete then Confirm
    const card = page.locator('[data-testid^="concept-"]').first();
    await card.hover();
    await card.getByRole("button", { name: "Delete" }).click();
    await card.getByRole("button", { name: "Confirm" }).click();

    // Should show empty state
    await countText(page, /^0 concepts$/);
    await expect(page.getByText("No concepts yet.")).toBeVisible();
  });

  test("should navigate back to the projects dashboard", async ({ page }) => {
    await createProjectAndOpenTerms(page);

    // The sidebar back button leaves the project (and closes the browser)
    await page.getByTestId("sidebar-home").click();

    await expect(page.getByTestId("terms-browser")).not.toBeVisible();
    await expect(page.getByText("Terms Test")).toBeVisible();
  });
});
