import { test, expect } from "@playwright/test";
import { assertNoUnhandledRoutes, setupLocalApp } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

// A project opens on the Map. Its counts come from the server's Map route,
// which the mock answers with the shared Map fixture (3 files, 42 blocks,
// 7 rules, 4 findings); Rules reads Decide, the kept rules and the members
// who decide the same way.
test.describe("Project Map", () => {
  test.beforeEach(async ({ page }) => {
    await setupLocalApp(page);
    await page.getByTestId("onboarding-create-btn").click();
    await page.getByTestId("project-name-input").fill("Map Test");
    await selectMultiLocales(page, "target-langs-input", ["fr"]);
    await page.getByTestId("create-project-submit").click();
  });

  test("opens a new project on the Map with its counts, and Rules from the panel", async ({
    page,
  }) => {
    const counts = page.locator("[data-slot='map-counts']");
    await expect(counts).toBeVisible();
    await expect(counts).toContainText("3");
    await expect(counts).toContainText("42");
    await expect(page.getByRole("heading", { name: "Map Test", level: 1 })).toBeVisible();
    await expect(page.getByTestId("subnav-map")).toBeVisible();

    await page.getByTestId("subnav-rules").click();
    const questions = page.getByRole("listbox", { name: "Suggestions to decide" });
    await expect(questions).toBeVisible();
    // A member who decides sees who holds each question.
    await expect(questions.locator("[data-slot='decide-columns']")).toContainText("Assigned to");
    await expect(page.getByRole("region", { name: "Kept by" })).toBeVisible();

    await assertNoUnhandledRoutes(page);
  });
});
