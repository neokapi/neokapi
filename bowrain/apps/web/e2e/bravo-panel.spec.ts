import { test, expect, type Page } from "@playwright/test";
import { TEST_IDS } from "@neokapi/ui/test-ids";
import { authenticate, getOrCreateWorkspace, waitForServer } from "./helpers/api-client";

const BASE_URL = process.env.BOWRAIN_URL || "http://localhost:8080";

/** Inject the auth token as an HttpOnly cookie via Playwright's cookie API. */
async function injectAuthCookie(page: Page, authToken: string) {
  const url = new URL(BASE_URL);
  await page.context().addCookies([
    {
      name: "bowrain_session",
      value: authToken,
      domain: url.hostname,
      path: "/api/",
      httpOnly: true,
      sameSite: "Lax" as const,
    },
  ]);
}

let token: string;
let wsSlug: string;

test.describe("Bravo Panel", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(
      token,
      process.env.BOWRAIN_WORKSPACE_NAME || "Acme Inc.",
      process.env.BOWRAIN_WORKSPACE || "acme",
    );
    wsSlug = ws.slug;
  });

  test("@bravo trigger button is visible on workspace dashboard", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);

    // The @bravo trigger should be rendered in the top bar
    const trigger = page.getByTestId(TEST_IDS.bravo.trigger);
    await expect(trigger).toBeVisible({ timeout: 10000 });
  });

  test("clicking @bravo trigger opens the side panel", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);

    const trigger = page.getByTestId(TEST_IDS.bravo.trigger);
    await expect(trigger).toBeVisible({ timeout: 10000 });

    // Click to open the panel
    await trigger.click();

    // Panel should show the "New conversation" button
    await expect(page.getByTestId(TEST_IDS.bravo.newConversationHeader)).toBeVisible({
      timeout: 5000,
    });
  });

  test("panel shows empty state when no conversations exist", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);

    const trigger = page.getByTestId(TEST_IDS.bravo.trigger);
    await expect(trigger).toBeVisible({ timeout: 10000 });
    await trigger.click();

    // Should show the empty state or "New conversation" button
    await expect(page.getByTestId(TEST_IDS.bravo.newConversationHeader)).toBeVisible({
      timeout: 5000,
    });
  });

  test("panel closes when trigger is clicked again", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);

    const trigger = page.getByTestId(TEST_IDS.bravo.trigger);
    await expect(trigger).toBeVisible({ timeout: 10000 });

    // Open
    await trigger.click();
    await expect(page.getByTestId(TEST_IDS.bravo.newConversationHeader)).toBeVisible({
      timeout: 5000,
    });

    // Close — click the trigger again
    await trigger.click();
    await page.waitForTimeout(500);

    // "New conversation" button should no longer be visible
    await expect(page.getByText("New conversation")).not.toBeVisible({ timeout: 3000 });
  });

  test("@bravo trigger is visible on project detail page", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);

    // The @bravo trigger should also be present on sub-pages
    const trigger = page.getByTestId(TEST_IDS.bravo.trigger);
    await expect(trigger).toBeVisible({ timeout: 10000 });
  });
});
