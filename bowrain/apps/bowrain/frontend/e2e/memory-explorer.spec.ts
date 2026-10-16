import { test, expect, type Page } from "@playwright/test";
import { setupLocalApp, openProjectSource } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/**
 * E2E for the workspace content memory page (MemorySection) as the bowrain
 * desktop mounts it: a list of pairs beside a detail pane, reached from the
 * project view's "open-tm-btn". The page reads the memory into one target
 * language at a time, starting on the language most of the workspace's
 * projects translate into.
 */

/** Creates a project and returns with the project view visible (before opening the content memory). */
async function createProject(page: Page) {
  await setupLocalApp(page);

  // Create project
  await page.getByTestId("onboarding-create-btn").click();
  await page.getByTestId("project-name-input").fill("content memory Test");
  await selectMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();

  // Creating a project lands on its overview; the memory and terms pages are
  // opened from the source view beside it (#1793 moved the landing).
  await expect(page.getByTestId("back-to-projects")).toBeVisible();
  await openProjectSource(page);
}

/** Adds pairs through the mock backend, before the content memory is opened. */
async function seed(page: Page, pairs: Array<[string, string, string]>) {
  await page.evaluate((rows) => {
    const backend = (window as any).__wailsMockByName;
    const projects = backend.ListProjects();
    const pid = projects[0]?.id;
    if (!pid) return;
    for (const [source, target, locale] of rows) {
      backend.AddMemoryEntry(pid, source, target, "en", locale);
    }
  }, pairs);
}

/** Opens the content memory from the project view. */
async function openMemory(page: Page) {
  await page.getByTestId("open-tm-btn").click();
  await expect(page.getByTestId("tm-browser")).toBeVisible();
}

/** Creates a project and opens the content memory. */
async function createProjectAndOpenMemory(page: Page) {
  await createProject(page);
  await openMemory(page);
}

const rows = (page: Page) => page.locator('[data-testid^="tm-entry-"]');
const detail = (page: Page) => page.locator('[data-slot="memory-pair-detail"]');

test.describe("content memory", () => {
  test("should open the content memory and show its empty state", async ({ page }) => {
    await createProjectAndOpenMemory(page);

    await expect(page.getByText("0 entries")).toBeVisible();
    await expect(page.getByText("No approved wording into French yet")).toBeVisible();
  });

  test("should add an entry and see it in the list", async ({ page }) => {
    await createProjectAndOpenMemory(page);

    // Add entry is the page header's act.
    await page.locator('[data-slot="memory-add"]').click();
    const dialog = page.getByRole("dialog", { name: "Add an entry" });
    await expect(dialog).toBeVisible();
    await dialog.getByLabel("Source", { exact: true }).fill("Hello");
    await dialog.getByLabel("Target", { exact: true }).fill("Bonjour");
    await dialog.getByRole("button", { name: "Add entry", exact: true }).click();

    // The pair is a row: its source, and its target's language.
    await expect(page.getByText("1 entry", { exact: true })).toBeVisible();
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText("Hello");
    await rows(page).first().click();
    await expect(detail(page)).toContainText("Bonjour");
  });

  test("should search entries by text", async ({ page }) => {
    await createProject(page);
    await seed(page, [
      ["Hello World", "Bonjour le monde", "fr"],
      ["Goodbye", "Au revoir", "fr"],
    ]);
    await openMemory(page);

    await expect(rows(page)).toHaveCount(2);

    // The search asks the server as you type.
    await page.getByTestId("tm-search").fill("Hello");
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText("Hello World");
    await expect(page.getByText("1 entry matches")).toBeVisible();
  });

  test("should read one target language at a time", async ({ page }) => {
    await createProject(page);
    await seed(page, [
      ["Hello", "Bonjour", "fr"],
      ["Good morning", "Guten Morgen", "de"],
    ]);
    await openMemory(page);

    // The project lists French first, so the page opens on French.
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText("Hello");
    await expect(page.getByText("Good morning")).not.toBeVisible();

    // The picker under the search changes the language.
    await page.getByRole("combobox", { name: "Target language" }).click();
    await page.getByRole("option", { name: /German/ }).click();
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText("Good morning");
  });

  test("should edit an entry in the detail pane", async ({ page }) => {
    await createProject(page);
    await seed(page, [["Hello", "Bonjour", "fr"]]);
    await openMemory(page);

    await rows(page).first().click();
    await expect(detail(page)).toContainText("Bonjour");
    await detail(page).locator('[data-slot="memory-edit"]').click();

    const target = detail(page).getByRole("textbox", { name: "Target" });
    await target.fill("Salut");
    await detail(page).getByRole("button", { name: "Save" }).click();

    await expect(detail(page).locator('[data-slot="memory-target"]')).toHaveText("Salut");
  });

  test("should delete an entry", async ({ page }) => {
    await createProject(page);
    await seed(page, [["Hello", "Bonjour", "fr"]]);
    await openMemory(page);

    await expect(page.getByText("1 entry", { exact: true })).toBeVisible();

    // Two-click delete in the detail pane's toolbar: Delete, then Confirm.
    await rows(page).first().click();
    await detail(page).getByRole("button", { name: "Delete" }).click();
    await detail(page).getByRole("button", { name: "Confirm" }).click();

    await expect(page.getByText("No approved wording into French yet")).toBeVisible();
  });

  test("should delete every marked entry", async ({ page }) => {
    await createProject(page);
    await seed(page, [
      ["Hello", "Bonjour", "fr"],
      ["Goodbye", "Au revoir", "fr"],
    ]);
    await openMemory(page);

    await page.locator('[data-slot="memory-mark-all"]').click();
    const bulk = page.locator('[data-slot="memory-bulk-detail"]');
    await expect(bulk).toContainText("2 entries marked");
    await bulk.getByRole("button", { name: "Delete" }).click();
    await bulk.getByRole("button", { name: "Confirm" }).click();

    await expect(page.getByText("No approved wording into French yet")).toBeVisible();
  });

  test("should navigate back to the projects dashboard", async ({ page }) => {
    await createProjectAndOpenMemory(page);

    // The content memory is a workspace route, so opening it has already left
    // the project, and there is no project step in the trail to click. The rail
    // is the way back: Projects is one click from anywhere in the workspace.
    await page.getByTestId("nav-translate").click();

    await expect(page.getByTestId("tm-browser")).not.toBeVisible();
    await expect(page.getByText("content memory Test")).toBeVisible();
  });
});
