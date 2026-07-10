import { test, expect, type Page } from "@playwright/test";
import { TEST_IDS } from "@neokapi/ui/test-ids";
import { authenticate, getOrCreateWorkspace, waitForServer } from "./helpers/api-client";

const BASE_URL = process.env.BOWRAIN_URL || "http://localhost:8080";

// @bravo is dark by default (epic 015): billing.FeatureBravo is false on every
// plan, so the trigger/panel are withheld unless the workspace is granted the
// feature via a control-plane override. The interactive suite below therefore
// only runs when the operator explicitly enables it (feature override set +
// BOWRAIN_E2E_BRAVO_ENABLED=1); by default we assert the surface is hidden.
const BRAVO_ENABLED = process.env.BOWRAIN_E2E_BRAVO_ENABLED === "1";

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

  test("@bravo is dark by default — no trigger in the top bar", async ({ page }) => {
    test.skip(BRAVO_ENABLED, "@bravo explicitly enabled for this run");
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);

    // Wait for the app shell to settle (avatar area rendered), then assert the
    // @bravo trigger is not present at all.
    await page.waitForLoadState("networkidle");
    await expect(page.getByTestId(TEST_IDS.bravo.trigger)).toHaveCount(0);
  });

  // Interactive coverage for the enabled surface. Skipped unless the workspace
  // has been granted FeatureBravo via a control-plane override and the run opts
  // in with BOWRAIN_E2E_BRAVO_ENABLED=1.
  test.describe("when @bravo is enabled", () => {
    test.skip(
      !BRAVO_ENABLED,
      "@bravo is dark by default (epic 015); grant the FeatureBravo override and set BOWRAIN_E2E_BRAVO_ENABLED=1 to run",
    );

    test("@bravo trigger button is visible on workspace dashboard", async ({ page }) => {
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}`);

      const trigger = page.getByTestId(TEST_IDS.bravo.trigger);
      await expect(trigger).toBeVisible({ timeout: 10000 });
    });

    test("clicking @bravo trigger opens the side panel", async ({ page }) => {
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}`);

      const trigger = page.getByTestId(TEST_IDS.bravo.trigger);
      await expect(trigger).toBeVisible({ timeout: 10000 });

      await trigger.click();

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

      await expect(page.getByTestId(TEST_IDS.bravo.newConversationHeader)).toBeVisible({
        timeout: 5000,
      });
    });

    test("panel closes when trigger is clicked again", async ({ page }) => {
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}`);

      const trigger = page.getByTestId(TEST_IDS.bravo.trigger);
      await expect(trigger).toBeVisible({ timeout: 10000 });

      await trigger.click();
      await expect(page.getByTestId(TEST_IDS.bravo.newConversationHeader)).toBeVisible({
        timeout: 5000,
      });

      await trigger.click();
      await page.waitForTimeout(500);

      await expect(page.getByText("New conversation")).not.toBeVisible({ timeout: 3000 });
    });

    test("@bravo trigger is visible on project detail page", async ({ page }) => {
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}`);

      const trigger = page.getByTestId(TEST_IDS.bravo.trigger);
      await expect(trigger).toBeVisible({ timeout: 10000 });
    });
  });
});
