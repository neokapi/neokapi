import { test, expect } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";

test.beforeEach(async ({ page }) => {
  await setupLocalApp(page);
  // Navigate to the Flows view via sidebar icon
  await page.getByTestId("nav-flows").click();
});

test("should display flow list with built-in flows", async ({ page }) => {
  const list = page.getByTestId("flow-list");
  await expect(list).toBeVisible();
  await expect(page.getByTestId("flow-item-ai-translate")).toBeVisible();
  await expect(page.getByTestId("flow-item-pseudo-translate")).toBeVisible();
});

test("should show empty state when no flow is selected", async ({ page }) => {
  const empty = page.getByTestId("flow-empty-state");
  await expect(empty).toBeVisible();
  await expect(empty).toContainText("Select a flow");
});

test("should display flow builder when a flow is selected", async ({ page }) => {
  await page.getByTestId("flow-item-ai-translate").click();
  const toolbar = page.getByTestId("flow-toolbar");
  await expect(toolbar).toBeVisible();

  // Should show the flow name
  const nameInput = page.getByTestId("flow-name-input");
  await expect(nameInput).toHaveValue("AI Translate");
});

test("should mark built-in flows as read-only", async ({ page }) => {
  await page.getByTestId("flow-item-ai-translate").click();

  // Name input should be disabled for built-in flows
  const nameInput = page.getByTestId("flow-name-input");
  await expect(nameInput).toBeDisabled();

  // Save and delete buttons should not be visible
  await expect(page.getByTestId("save-flow-btn")).not.toBeVisible();
  await expect(page.getByTestId("delete-flow-btn")).not.toBeVisible();
});

test("should display flow nodes in the canvas", async ({ page }) => {
  await page.getByTestId("flow-item-ai-translate").click();

  // Wait for React Flow to render nodes
  await expect(page.getByTestId("flow-node-reader")).toBeVisible();
  await expect(page.getByTestId("flow-node-ai-translate")).toBeVisible();
  await expect(page.getByTestId("flow-node-writer")).toBeVisible();
});

/** Helper: open the New Flow dialog, fill name/description, and click Create. */
async function createNewFlow(page: any, name = "New Flow", description = "") {
  await page.getByTestId("new-flow-btn").click();
  // Dialog appears — fill in name
  const nameInput = page.getByTestId("new-flow-name");
  await expect(nameInput).toBeVisible();
  await nameInput.fill(name);
  if (description) {
    await page.getByTestId("new-flow-description").fill(description);
  }
  // Click Create button in dialog
  await page.getByRole("button", { name: /create/i }).click();
}

test("should create a new flow", async ({ page }) => {
  await createNewFlow(page, "My Test Flow");

  // Should show editable toolbar with the flow name
  const nameInput = page.getByTestId("flow-name-input");
  await expect(nameInput).toBeVisible();
  await expect(nameInput).not.toBeDisabled();
  await expect(nameInput).toHaveValue("My Test Flow");

  // Save button should be visible
  await expect(page.getByTestId("save-flow-btn")).toBeVisible();
  await expect(page.getByTestId("delete-flow-btn")).toBeVisible();
});

test("should show tool palette for editable flows", async ({ page }) => {
  await createNewFlow(page);

  const palette = page.getByTestId("tool-palette");
  await expect(palette).toBeVisible();
  await expect(palette).toContainText("Add Tool");
});

test("should not show tool palette for built-in flows", async ({ page }) => {
  await page.getByTestId("flow-item-ai-translate").click();
  await expect(page.getByTestId("tool-palette")).not.toBeVisible();
});

test("should add a tool to a new flow", async ({ page }) => {
  await createNewFlow(page);

  // Click add tool for ai-translate
  await page.getByTestId("add-tool-ai-translate").click();

  // A new tool node should appear in the canvas
  // The canvas should now have reader + writer + the new tool
  const nodes = page.locator("[data-testid^='flow-node-']");
  await expect(nodes).toHaveCount(3); // reader, writer, new tool
});

test("should save a custom flow", async ({ page }) => {
  await createNewFlow(page, "My Custom Flow");

  // Save
  await page.getByTestId("save-flow-btn").click();

  // Flow should appear in the list
  const listItems = page.locator("[data-testid^='flow-item-']");
  // Built-in (2) + new custom (1) = 3
  await expect(listItems).toHaveCount(3);
});

test("should navigate between flows", async ({ page }) => {
  // Select first flow
  await page.getByTestId("flow-item-ai-translate").click();
  await expect(page.getByTestId("flow-name-input")).toHaveValue("AI Translate");

  // Select second flow
  await page.getByTestId("flow-item-pseudo-translate").click();
  await expect(page.getByTestId("flow-name-input")).toHaveValue("Pseudo Translate");
});
