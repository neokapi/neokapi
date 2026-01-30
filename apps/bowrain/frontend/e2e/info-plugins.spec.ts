import { test, expect } from "@playwright/test";
import { injectMockBackend } from "./mock-backend";

test.beforeEach(async ({ page }) => {
  await injectMockBackend(page);
  await page.goto("/");
});

test.describe("Info Page", () => {
  test("should display formats, tools, and flows sections", async ({ page }) => {
    await page.locator("nav button", { hasText: "Info" }).click();

    // Formats section
    await expect(page.getByText("Formats")).toBeVisible();
    await expect(page.getByText("format(s) registered")).toBeVisible();
    // Should show format names from mock
    await expect(page.getByText("html")).toBeVisible();
    await expect(page.getByText("json")).toBeVisible();

    // Tools section
    await expect(page.getByText("Tools")).toBeVisible();
    await expect(page.getByText("tool(s) available")).toBeVisible();

    // Flows section
    await expect(page.getByText("Flows")).toBeVisible();
    await expect(page.getByText("flow(s) available")).toBeVisible();
  });
});

test.describe("Plugins Page", () => {
  test("should show empty state when no plugins loaded", async ({ page }) => {
    await page.locator("nav button", { hasText: "Plugins" }).click();

    await expect(page.getByRole("heading", { name: "Plugins" })).toBeVisible();
    await expect(page.getByTestId("plugins-empty")).toBeVisible();
    await expect(page.getByText("No plugins loaded.")).toBeVisible();
    await expect(page.getByText("Plugin directory:")).toBeVisible();
  });
});
