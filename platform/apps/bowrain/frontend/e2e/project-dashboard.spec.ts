import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectLocale, setMultiLocales, expectLocaleChips } from "./locale-helper";

test.beforeEach(async ({ page }) => {
  await setupLocalApp(page);
});

test("should display empty state on first load", async ({ page }) => {
  const empty = page.getByTestId("empty-projects");
  await expect(empty).toBeVisible();
  await expect(empty).toContainText("No projects yet");
});

test("should show new project button", async ({ page }) => {
  const btn = page.getByTestId("new-project-btn");
  await expect(btn).toBeVisible();
  await expect(btn).toContainText("New Project");
});

test("should open create project dialog", async ({ page }) => {
  await page.getByTestId("new-project-btn").click();
  const dialog = page.getByTestId("create-project-dialog");
  await expect(dialog).toBeVisible();
  await expect(page.getByTestId("project-name-input")).toBeVisible();
  await expect(page.getByTestId("source-lang-input")).toBeVisible();
  await expect(page.getByTestId("target-langs-input")).toBeVisible();
  // Verify default locale chip is present
  await expectLocaleChips(page, "target-langs-input", ["fr"]);
});

test("should create a new project", async ({ page }) => {
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("My Test Project");
  await selectLocale(page, "source-lang-input", "en");
  await setMultiLocales(page, "target-langs-input", ["fr", "de"]);
  await expectLocaleChips(page, "target-langs-input", ["fr", "de"]);
  await page.getByTestId("create-project-submit").click();

  // Should navigate to project view
  await expect(page.getByTestId("back-to-projects")).toBeVisible();
  await expect(page.getByText("My Test Project")).toBeVisible();
});

test("should navigate back from project view to dashboard", async ({ page }) => {
  // Create a project (default "fr" chip is already present)
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("Test");
  await expectLocaleChips(page, "target-langs-input", ["fr"]);
  await page.getByTestId("create-project-submit").click();

  // Should be in project view
  await expect(page.getByTestId("back-to-projects")).toBeVisible();

  // Go back
  await page.getByTestId("back-to-projects").click();

  // Should see the project card
  await expect(page.getByRole("heading", { name: "Test" })).toBeVisible();
});
