import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";

test.beforeEach(async ({ page }) => {
  await injectMockBackend(page);
  await page.goto("/");
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
});

test("should create a new project", async ({ page }) => {
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("My Test Project");
  await page.getByTestId("source-lang-input").fill("en");
  await page.getByTestId("target-langs-input").fill("fr, de");
  await page.getByTestId("create-project-submit").click();

  // Should navigate to project view
  await expect(page.getByTestId("back-to-projects")).toBeVisible();
  await expect(page.getByText("My Test Project")).toBeVisible();
});

test("should navigate back from project view to dashboard", async ({ page }) => {
  // Create a project
  await page.getByTestId("new-project-btn").click();
  await page.getByTestId("project-name-input").fill("Test");
  await page.getByTestId("target-langs-input").fill("fr");
  await page.getByTestId("create-project-submit").click();

  // Should be in project view
  await expect(page.getByTestId("back-to-projects")).toBeVisible();

  // Go back
  await page.getByTestId("back-to-projects").click();

  // Should see the project card
  await expect(page.getByText("Test")).toBeVisible();
});

test("should show open .kaz button", async ({ page }) => {
  const btn = page.getByTestId("open-kaz-btn");
  await expect(btn).toBeVisible();
  await expect(btn).toContainText("Open .kaz");
});
