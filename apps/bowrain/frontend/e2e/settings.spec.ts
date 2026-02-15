import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";

test.beforeEach(async ({ page }) => {
  await setupLocalApp(page);
  await page.locator("nav button", { hasText: "Settings" }).click();
});

test.describe("Settings Page", () => {
  test("should show tab bar with four tabs", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await expect(page.getByTestId("settings-tab-general")).toBeVisible();
    await expect(page.getByTestId("settings-tab-ai-providers")).toBeVisible();
    await expect(page.getByTestId("settings-tab-plugins")).toBeVisible();
    await expect(page.getByTestId("settings-tab-system-info")).toBeVisible();
  });

  test("should default to General tab", async ({ page }) => {
    await expect(page.getByTestId("settings-general")).toBeVisible();
  });

  test("should show AI Providers tab with empty state", async ({ page }) => {
    await page.getByTestId("settings-tab-ai-providers").click();
    await expect(page.getByTestId("settings-ai-providers")).toBeVisible();
    await expect(page.getByTestId("providers-empty")).toBeVisible();
    await expect(page.getByText("No AI providers configured.")).toBeVisible();
  });

  test("should add a provider via form", async ({ page }) => {
    await page.getByTestId("settings-tab-ai-providers").click();
    await page.getByTestId("add-provider-btn").click();

    // Fill form
    await page.getByTestId("provider-name").fill("My Anthropic");
    // Click custom select to open dropdown, then select option
    await page.getByTestId("provider-type").click();
    await page.getByRole("option", { name: "Anthropic" }).click();
    await page.getByTestId("provider-api-key").fill("sk-test-key");
    await page.getByTestId("provider-model").fill("claude-sonnet-4-20250514");

    // Save
    await page.getByTestId("provider-save-btn").click();

    // Should show in list
    await expect(page.getByText("My Anthropic")).toBeVisible();
    await expect(page.getByTestId("providers-empty")).not.toBeVisible();
  });

  test("should delete a provider", async ({ page }) => {
    await page.getByTestId("settings-tab-ai-providers").click();

    // First add a provider (Ollama doesn't require an API key)
    await page.getByTestId("add-provider-btn").click();
    await page.getByTestId("provider-name").fill("ToDelete");
    // Click custom select to open dropdown, then select option
    await page.getByTestId("provider-type").click();
    await page.getByRole("option", { name: "Ollama" }).click();
    await page.getByTestId("provider-save-btn").click();
    await expect(page.getByText("ToDelete")).toBeVisible();

    // Delete it
    const deleteBtn = page.locator("[data-testid^='delete-provider-']").first();
    await deleteBtn.click();
    await expect(page.getByTestId("providers-empty")).toBeVisible();
  });

  test("should show Plugins tab with empty state", async ({ page }) => {
    await page.getByTestId("settings-tab-plugins").click();
    await expect(page.getByTestId("settings-plugins")).toBeVisible();
    await expect(page.getByTestId("plugins-empty")).toBeVisible();
    await expect(page.getByText("No plugins loaded.")).toBeVisible();
    await expect(page.getByText("Plugin directory:")).toBeVisible();
  });

  test("should show System Info tab with formats, tools, and flows", async ({ page }) => {
    await page.getByTestId("settings-tab-system-info").click();
    await expect(page.getByTestId("settings-system-info")).toBeVisible();

    // Formats
    await expect(page.getByText("format(s) registered")).toBeVisible();
    await expect(page.getByText("html")).toBeVisible();
    await expect(page.getByText("json")).toBeVisible();

    // Tools
    await expect(page.getByText("tool(s) available")).toBeVisible();

    // Flows
    await expect(page.getByText("flow(s) available")).toBeVisible();
  });
});
