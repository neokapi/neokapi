import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";
import { selectLocale, setMultiLocales, expectLocaleChips } from "./locale-helper";

test.beforeEach(async ({ page }) => {
  await setupLocalApp(page);
});

test("should display empty state on first load", async ({ page }) => {
  const empty = page.getByTestId("empty-projects");
  await expect(empty).toBeVisible();
  await expect(empty).toContainText("Set up your workspace");
});

// #1363 replaced the three pathway cards ("From your repo" / "Upload files" /
// "Connect a CMS") with the starter-pack hero plus a project card and a team
// card. The onboarding surface is asserted through its testids so a further
// rewording cannot silently take the suite down again.
test("should show the onboarding routes in empty state", async ({ page }) => {
  const empty = page.getByTestId("empty-projects");
  await expect(empty).toBeVisible();
  // The assistant-driven starter pack, with its copyable prompt.
  await expect(page.getByTestId("starter-prompt")).toBeVisible();
  await expect(page.getByTestId("copy-starter-prompt")).toBeVisible();
  // The project route: a drop zone and an explicit create button.
  await expect(page.getByTestId("onboarding-dropzone")).toBeVisible();
  await expect(page.getByTestId("onboarding-create-btn")).toBeVisible();
});

test("should open create project dialog from the onboarding card", async ({ page }) => {
  await page.getByTestId("onboarding-create-btn").click();
  const dialog = page.getByTestId("create-project-dialog");
  await expect(dialog).toBeVisible();
  await expect(page.getByTestId("project-name-input")).toBeVisible();
  await expect(page.getByTestId("source-lang-input")).toBeVisible();
  await expect(page.getByTestId("target-langs-input")).toBeVisible();
  // Verify default locale chip is present
  await expectLocaleChips(page, "target-langs-input", ["fr"]);
});

test("should create a new project", async ({ page }) => {
  // Open the create dialog from the empty state
  await page.getByTestId("onboarding-create-btn").click();
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
  // Create a project from the empty state (default "fr" chip is already present)
  await page.getByTestId("onboarding-create-btn").click();
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
