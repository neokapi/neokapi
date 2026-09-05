import { test, expect, type Page, type Locator } from "@playwright/test";
import { setupLocalApp, assertNoUnhandledRoutes } from "./mock-backend";
import { selectMultiLocales } from "./locale-helper";

/**
 * Flows are project-scoped: a project's Automations section holds Runs, Rules
 * and Flows, and the Flows tab lists the flows as the shared outcome-first
 * cards and opens one in the shared linear step editor (`@neokapi/bowrain-app`
 * over `@neokapi/ui-primitives`). The desktop serves the flow definitions from
 * its own Wails bindings, which the mock backend answers; the list, the editor
 * and the graph<->steps conversion are the same ones the web app renders.
 */
async function openProjectFlows(page: Page) {
  await setupLocalApp(page);

  await page.getByTestId("onboarding-create-btn").click();
  await page.getByTestId("project-name-input").fill("Flow Test");
  await selectMultiLocales(page, "target-langs-input", ["fr"]);
  await page.getByTestId("create-project-submit").click();

  await page.getByTestId("subnav-automations").click();
  await page.getByRole("button", { name: "Flows", exact: true }).click();
  await expect(page.getByTestId("flow-list")).toBeVisible();
}

/** Opens the New flow dialog, names the flow and creates it. */
async function createNewFlow(page: Page, name = "New Flow") {
  await page.getByTestId("new-flow-btn").click();
  const nameInput = page.getByTestId("new-flow-name");
  await expect(nameInput).toBeVisible();
  await nameInput.fill(name);
  await page.getByTestId("create-flow-btn").click();
  await expect(page.getByRole("heading", { name })).toBeVisible();
}

/** Picks a tool from the add-step dialog a trigger opened. */
async function pickTool(page: Page, trigger: Locator, tool: RegExp) {
  await trigger.click();
  const dialog = page.getByRole("dialog");
  await dialog.getByTestId("add-step-tool").filter({ hasText: tool }).click();
  await expect(dialog).not.toBeVisible();
}

const saved = (page: Page) => expect(page.getByTestId("flow-save-state")).toHaveText(/Saved/);

test.beforeEach(async ({ page }) => {
  await openProjectFlows(page);
});

test("lists the built-in flows as cards", async ({ page }) => {
  const translate = page.getByTestId("flow-item-translate");
  await expect(translate).toBeVisible();
  await expect(translate).toContainText("built-in");
  // The card's chip strip reads the graph in edge order.
  await expect(translate.getByTestId("flow-steps")).toHaveText(/AI Translate.*Word Count/);
  await expect(page.getByTestId("flow-item-pseudo-translate")).toBeVisible();
  await assertNoUnhandledRoutes(page);
});

test("opens a flow in the linear editor", async ({ page }) => {
  await page.getByTestId("flow-item-translate").click();
  await expect(page.getByTestId("linear-flow-editor")).toBeVisible();
  await expect(page.getByRole("heading", { name: "AI Translate" })).toBeVisible();
});

test("marks a built-in flow read-only", async ({ page }) => {
  await page.getByTestId("flow-item-translate").click();
  await expect(page.getByTestId("flow-read-only")).toBeVisible();
  await expect(page.getByTestId("add-step")).toHaveCount(0);
  await expect(page.getByLabel("Rename flow")).toHaveCount(0);
  await expect(page.getByTestId("copy-flow-btn")).toBeVisible();
});

test("renders a built-in flow's steps in order", async ({ page }) => {
  await page.getByTestId("flow-item-translate").click();
  const rows = page.getByTestId("step-row");
  await expect(rows).toHaveCount(2);
  await expect(rows.nth(0)).toContainText("AI Translate");
  await expect(rows.nth(1)).toContainText("Word Count");
  await expect(page.getByTestId("parallel-group")).toHaveCount(0);
});

test("shows a flow as a read-only diagram beside the step editor", async ({ page }) => {
  await page.getByTestId("flow-item-translate").click();
  await expect(page.getByTestId("flow-view-steps")).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByTestId("flow-view-run")).toHaveCount(0);

  await page.getByTestId("flow-view-diagram").click();
  const diagram = page.getByTestId("flow-diagram-view");
  await expect(diagram).toBeVisible();
  await expect(diagram.getByText("AI Translate").first()).toBeVisible();
  await expect(page.getByTestId("step-row")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Add tool" })).toHaveCount(0);

  await page.getByTestId("flow-view-steps").click();
  await expect(page.getByTestId("step-row")).toHaveCount(2);
});

test("creates a new flow and shows its empty state", async ({ page }) => {
  await createNewFlow(page, "My Test Flow");
  await expect(page.getByTestId("flow-read-only")).toHaveCount(0);
  await expect(page.getByText("This flow has no steps yet")).toBeVisible();
  await expect(page.getByTestId("add-step")).toBeVisible();
  await saved(page);
});

test("offers the template library for an empty new flow", async ({ page }) => {
  await createNewFlow(page);
  await expect(page.getByRole("heading", { name: "Start from a template" })).toBeVisible();
});

test("adds a step from the picker", async ({ page }) => {
  await createNewFlow(page);
  await pickTool(page, page.getByTestId("add-step"), /^translate/);
  const row = page.getByTestId("step-row");
  await expect(row).toHaveCount(1);
  await expect(row).toContainText("translate");
  await expect(row).toContainText("Translate content using AI");
});

test("a step shows its options and an edit round-trips into config", async ({ page }) => {
  await createNewFlow(page, "Strict terms");
  await pickTool(page, page.getByTestId("add-step"), /^term-check/);
  const row = page.getByTestId("step-row");
  await row.getByLabel("Options").click();
  const options = row.getByTestId("step-options");
  await expect(options).toBeVisible();
  const toggle = options.getByRole("switch", { name: /Case Sensitive/ });
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await saved(page);

  // The option rides the step's config: reopening the flow reads it back.
  await page.getByTestId("flow-back").click();
  await page.locator("[data-testid^='flow-item-']").filter({ hasText: "Strict terms" }).click();
  await page.getByTestId("step-row").getByLabel("Options").click();
  await expect(
    page.getByTestId("step-options").getByRole("switch", { name: /Case Sensitive/ }),
  ).toHaveAttribute("aria-checked", "true");
  await assertNoUnhandledRoutes(page);
});

test("a step whose tool has no options shows no options control", async ({ page }) => {
  await createNewFlow(page);
  await pickTool(page, page.getByTestId("add-step"), /^translate/);
  await saved(page);
  const row = page.getByTestId("step-row");
  await expect(row.getByLabel("Move up")).toBeVisible();
  await expect(row.getByLabel("Options")).toHaveCount(0);
});

test("saves an edit and reads it back from the backend", async ({ page }) => {
  await createNewFlow(page, "My Custom Flow");
  await pickTool(page, page.getByTestId("add-step"), /^translate/);
  await expect(page.getByTestId("flow-save-state")).toHaveText(/Unsaved changes|Saving|Saved/);
  await saved(page);

  await page.getByTestId("flow-back").click();
  const cards = page.locator("[data-testid^='flow-item-']");
  await expect(cards).toHaveCount(3);
  const mine = cards.filter({ hasText: "My Custom Flow" });
  await expect(mine.getByTestId("flow-steps")).toHaveText("translate");

  await mine.click();
  await expect(page.getByTestId("step-row")).toHaveCount(1);
  await assertNoUnhandledRoutes(page);
});

test("navigates between flows", async ({ page }) => {
  await page.getByTestId("flow-item-translate").click();
  await expect(page.getByRole("heading", { name: "AI Translate" })).toBeVisible();

  await page.getByTestId("flow-back").click();
  await page.getByTestId("flow-item-pseudo-translate").click();
  await expect(page.getByRole("heading", { name: "Pseudo Translate" })).toBeVisible();
});

test("adds a parallel group and round-trips it through the backend", async ({ page }) => {
  await createNewFlow(page, "Fan out");
  await pickTool(page, page.getByTestId("add-parallel-group"), /^translate/);
  const group = page.getByTestId("parallel-group");
  await expect(group).toBeVisible();
  await expect(group.getByTestId("step-row")).toHaveCount(1);

  await pickTool(page, group.getByTestId("add-branch"), /^pseudo-translate/);
  await expect(group.getByTestId("step-row")).toHaveCount(2);
  await expect(group).toContainText("2 in parallel");
  await saved(page);

  await page.getByTestId("flow-back").click();
  await page.locator("[data-testid^='flow-item-']").filter({ hasText: "Fan out" }).click();
  await expect(page.getByTestId("parallel-group")).toBeVisible();
  await expect(page.getByTestId("parallel-group").getByTestId("step-row")).toHaveCount(2);
});

test("renames a flow in place", async ({ page }) => {
  await createNewFlow(page, "Draft");
  await page.getByLabel("Rename flow").click();
  const input = page.getByLabel("Flow name");
  await input.fill("Final");
  await page.getByLabel("Save").click();
  await expect(page.getByRole("heading", { name: "Final" })).toBeVisible();
  await saved(page);

  await page.getByTestId("flow-back").click();
  await expect(
    page.locator("[data-testid^='flow-item-']").filter({ hasText: "Final" }),
  ).toHaveCount(1);
});

test("deletes a project flow from its card after confirming", async ({ page }) => {
  await createNewFlow(page, "Disposable");
  await page.getByTestId("flow-back").click();
  const mine = page.locator("[data-testid^='flow-item-']").filter({ hasText: "Disposable" });
  await mine.hover();
  await mine.getByLabel("Delete").click();
  await mine.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(mine).toHaveCount(0);
  await expect(page.locator("[data-testid^='flow-item-']")).toHaveCount(2);
});
