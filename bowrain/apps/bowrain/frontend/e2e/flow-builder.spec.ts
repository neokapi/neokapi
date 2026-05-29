import { test, expect, type Page } from "@playwright/test";
import { setupLocalApp } from "./mock-backend";

// The Bowrain flow builder now renders the shared @neokapi/flow-editor
// <FlowEditor> for the editing canvas (the same component kapi-desktop uses).
// FlowBuilder still owns the flow list, toolbar, and new/save/delete chrome
// (stable data-testids), while the canvas, tool palette, and node rendering
// come from the shared editor — asserted here by visible text / roles.

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

  // The shared FlowEditor canvas is mounted.
  await expect(page.getByTestId("flow-editor")).toBeVisible();
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

test("should render the flow's tool node in the canvas", async ({ page }) => {
  await page.getByTestId("flow-item-ai-translate").click();

  const editor = page.getByTestId("flow-editor");
  await expect(editor).toBeVisible();
  // The ai-translate step renders as a tool node labelled "AI Translate",
  // flanked by the Input (reader) and Output (writer) nodes.
  await expect(editor.getByText("Input", { exact: true })).toBeVisible();
  await expect(editor.getByText("Output", { exact: true })).toBeVisible();
  await expect(editor.getByText("AI Translate").first()).toBeVisible();
});

/** Helper: open the New Flow dialog, fill name/description, and click Create. */
async function createNewFlow(page: Page, name = "New Flow", description = "") {
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

test("should offer the template library for an empty new flow", async ({ page }) => {
  await createNewFlow(page);

  // A brand-new (empty) flow shows the shared editor's template library, which
  // offers an "empty canvas" escape hatch.
  const emptyCanvasBtn = page.getByRole("button", { name: /empty canvas/i });
  await expect(emptyCanvasBtn).toBeVisible();
});

test("should add a tool to a new flow from the palette", async ({ page }) => {
  await createNewFlow(page);

  // Dismiss the template library to reveal the tool palette + canvas.
  await page.getByRole("button", { name: /empty canvas/i }).click();

  // Add the ai-translate tool from the palette (rendered as a button by name).
  await page.getByRole("button", { name: "ai-translate", exact: true }).click();

  // The new tool node appears in the canvas.
  const editor = page.getByTestId("flow-editor");
  await expect(editor.getByText("ai-translate").first()).toBeVisible();
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
