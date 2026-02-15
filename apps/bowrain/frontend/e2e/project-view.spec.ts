import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

async function createProject(page: any) {
  await injectMockBackend(page);
  await page.goto("/");

  // Create project
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("File Test");
  await selectMultiLocales(page, "target-langs-input", ["fr"]);
  await page.getByTestId("create-project-submit").click();

  // Wait for project view
  await expect(page.getByTestId("back-to-projects")).toBeVisible();
}

test.describe("Project View", () => {
  test.beforeEach(async ({ page }) => {
    await createProject(page);
  });

  test("should display project name", async ({ page }) => {
    await expect(page.getByText("File Test")).toBeVisible();
  });

  test("should show file stats", async ({ page }) => {
    // Stats section shows 0 files, 0 blocks, 0 words
    await expect(page.getByText("Files", { exact: true })).toBeVisible();
    await expect(page.getByText("Blocks", { exact: true })).toBeVisible();
    await expect(page.getByText("Words", { exact: true })).toBeVisible();
  });

  test("should show file drop zone", async ({ page }) => {
    const dropZone = page.getByTestId("file-drop-zone");
    await expect(dropZone).toBeVisible();
    await expect(dropZone).toContainText("Drag and drop files");
  });

  test("should display language info", async ({ page }) => {
    // The component shows "en → fr" using HTML entities
    await expect(page.getByText("Languages")).toBeVisible();
  });
});
