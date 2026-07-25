import { test, expect, type Page } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/**
 * E2E for the shared @neokapi/ui-primitives MemoryBrowser as mounted by the
 * bowrain desktop (App.tsx renders it via useMemoryBrowserAdapter when the
 * project view's "open-tm-btn" is clicked).
 */

/** Creates a project and returns with the project view visible (before opening the content memory browser). */
async function createProject(page: Page) {
  await setupLocalApp(page);

  // Create project
  await page.getByText("Upload files").click();
  await page.getByTestId("project-name-input").fill("content memory Test");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();

  // Wait for project view
  await expect(page.getByTestId("back-to-projects")).toBeVisible();
}

/** Opens the content memory browser from the project view. */
async function openMemory(page: Page) {
  await page.getByTestId("open-tm-btn").click();
  await expect(page.getByTestId("tm-browser")).toBeVisible();
}

/** Creates a project and opens the content memory browser. */
async function createProjectAndOpenMemory(page: Page) {
  await createProject(page);
  await openMemory(page);
}

/** The "N entries" count in the content memory browser toolbar. */
function countText(page: Page, pattern: RegExp) {
  return expect(page.getByTestId("tm-browser").getByText(pattern)).toBeVisible();
}

test.describe("content memory Browser", () => {
  test("should open content memory browser and show empty state", async ({ page }) => {
    await createProjectAndOpenMemory(page);

    await countText(page, /^0 entries/);
    await expect(page.getByText("No entries yet.")).toBeVisible();
  });

  test("should add an entry and see it in the list", async ({ page }) => {
    await createProjectAndOpenMemory(page);

    // Open the add dialog and fill it in
    await page.getByRole("button", { name: "Add Entry" }).click();
    const dialog = page.getByRole("dialog", { name: "Add Memory Entry" });
    await expect(dialog).toBeVisible();
    await dialog.getByPlaceholder("Source text").fill("Hello");
    await dialog.getByPlaceholder("Target text").fill("Bonjour");
    await dialog.getByRole("button", { name: "Add", exact: true }).click();

    // Entry appears in the list
    await countText(page, /^1 entry/);
    const entry = page.locator('[data-testid^="tm-entry-"]').first();
    await expect(entry).toContainText("Hello");
    await expect(entry).toContainText("Bonjour");
  });

  test("should search entries by text", async ({ page }) => {
    await createProject(page);

    // Add two entries via mock backend before opening the content memory browser
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddMemoryEntry(pid, "Hello World", "Bonjour le monde", "en", "fr");
        backend.AddMemoryEntry(pid, "Goodbye", "Au revoir", "en", "fr");
      }
    });

    await openMemory(page);

    // Verify both entries are shown initially
    await countText(page, /^2 entries/);

    // Search is submitted on Enter
    const search = page.getByTestId("tm-search");
    await search.fill("Hello");
    await search.press("Enter");

    // Should show only 1 entry (the "Hello World" one)
    await countText(page, /^1 entry/);
    await expect(page.locator('[data-testid^="tm-entry-"]')).toHaveCount(1);
    await expect(page.locator('[data-testid^="tm-entry-"]').first()).toContainText("Hello World");
  });

  test("should scope entries to the active target locale", async ({ page }) => {
    await createProject(page);

    // Add entries in different target locales before opening the content memory browser
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddMemoryEntry(pid, "Hello", "Bonjour", "en", "fr");
        backend.AddMemoryEntry(pid, "Good morning", "Guten Morgen", "en", "de");
      }
    });

    await openMemory(page);

    // The bilingual view defaults to the project's first target locale (fr),
    // so only the French entry is in scope; the German one is filtered out.
    await countText(page, /^1 entry/);
    await expect(page.locator('[data-testid^="tm-entry-"]')).toHaveCount(1);
    await expect(page.locator('[data-testid^="tm-entry-"]').first()).toContainText("Bonjour");
    await expect(page.getByText("Guten Morgen")).not.toBeVisible();
  });

  test("should edit an entry target", async ({ page }) => {
    await createProject(page);

    // Add an entry before opening the content memory browser
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddMemoryEntry(pid, "Hello", "Bonjour", "en", "fr");
      }
    });

    await openMemory(page);

    const entry = page.locator('[data-testid^="tm-entry-"]').first();
    await expect(entry).toContainText("Bonjour");

    // Enter the variant inline editor (Edit reveals on hover)
    await entry.hover();
    await entry.getByRole("button", { name: "Edit" }).first().click();

    // Replace the target text; Enter commits the edit
    const editor = entry.locator('[contenteditable="true"]').first();
    await expect(editor).toBeVisible();
    await editor.fill("Salut");
    await editor.press("Enter");

    // Verify updated target
    await expect(entry).toContainText("Salut");
  });

  test("should delete an entry", async ({ page }) => {
    await createProject(page);

    // Add an entry before opening the content memory browser
    await page.evaluate(() => {
      const backend = (window as any).__wailsMockByName;
      const projects = backend.ListProjects();
      const pid = projects[0]?.id;
      if (pid) {
        backend.AddMemoryEntry(pid, "Hello", "Bonjour", "en", "fr");
      }
    });

    await openMemory(page);

    // Verify entry is present
    await countText(page, /^1 entry/);

    // Two-click delete: Delete then Confirm
    const entry = page.locator('[data-testid^="tm-entry-"]').first();
    await entry.hover();
    await entry.getByRole("button", { name: "Delete" }).click();
    await entry.getByRole("button", { name: "Confirm" }).click();

    // Should show empty state
    await countText(page, /^0 entries/);
    await expect(page.getByText("No entries yet.")).toBeVisible();
  });

  test("should navigate back to the projects dashboard", async ({ page }) => {
    await createProjectAndOpenMemory(page);

    // The sidebar back button leaves the project (and closes the browser)
    await page.getByTestId("sidebar-home").click();

    await expect(page.getByTestId("tm-browser")).not.toBeVisible();
    await expect(page.getByText("content memory Test")).toBeVisible();
  });
});
