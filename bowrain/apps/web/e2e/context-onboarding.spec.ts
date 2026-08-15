import { test, expect, type Page } from "@playwright/test";
// Leaf subpath export bypasses the @neokapi/ui barrel (which transitively
// imports JSON vocabularies that break Node-ESM Playwright collection).
// See bowrain/packages/ui/package.json `exports."./test-ids"`.
import { TEST_IDS } from "@neokapi/ui/test-ids";
import {
  authenticate,
  deleteAllEditorProjects,
  getOrCreateWorkspace,
  waitForServer,
} from "./helpers/api-client";

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

/**
 * The two blank-slate front doors, against the real stack: the workspace landing
 * and the Context hub's landing. Both are the first screen a new workspace ever
 * shows, and both must offer a way in rather than a description of one.
 *
 * The workspace here is its own, kept empty on purpose — the shared `acme`
 * workspace the other specs seed has projects and an approved voice profile, so
 * neither empty state would render there.
 */
test.describe("Blank-workspace onboarding", () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(token, "Blank Inc.", "blank-onboarding");
    wsSlug = ws.slug;
    // Re-runs reuse the workspace; a project left behind would declare a point
    // and take both empty states off the screen.
    await deleteAllEditorProjects(token, wsSlug);
  });

  test("the workspace landing offers the assistant prompt", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);

    await expect(page.getByTestId(TEST_IDS.onboarding.emptyProjects)).toBeVisible({
      timeout: 15000,
    });
    const prompt = page.getByTestId(TEST_IDS.onboarding.starterPrompt);
    await expect(prompt).toBeVisible();
    await expect(prompt).toContainText("Install the kapi skill");
    await expect(prompt).toContainText("the Blank Inc. workspace");
    // The web shell folds its own origin in, so the prompt is runnable as-is.
    await expect(prompt).toContainText(BASE_URL);
  });

  test("/context lands on Profiles and offers both onboarding lanes", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/context`);

    await expect(page).toHaveURL(new RegExp(`/${wsSlug}/context/profiles`), { timeout: 15000 });

    const lanes = page.getByTestId(TEST_IDS.onboarding.contextLanes);
    await expect(lanes).toBeVisible({ timeout: 15000 });

    // Lane one: the user's own assistant, with the prompt to hand it.
    await expect(lanes).toContainText("Build your brand starter pack with your AI");
    const prompt = lanes.getByTestId(TEST_IDS.onboarding.starterPrompt);
    await expect(prompt).toContainText("Install the kapi skill");
    await expect(prompt).toContainText("the Blank Inc. workspace");

    // Lane two: the hosted scan, which this stack runs.
    await expect(page.getByTestId(TEST_IDS.onboarding.contextScanCard)).toBeVisible();
  });

  test("the hosted lane opens the scan intake", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/context/profiles`);

    await page.getByTestId(TEST_IDS.onboarding.contextScanButton).click();

    await expect(page).toHaveURL(new RegExp(`/${wsSlug}/context/scan`), { timeout: 15000 });
  });

  test("the copy button puts the prompt on the clipboard", async ({ page, context }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/context/profiles`);

    await expect(page.getByTestId(TEST_IDS.onboarding.contextLanes)).toBeVisible({
      timeout: 15000,
    });
    await page.getByTestId(TEST_IDS.onboarding.copyStarterPrompt).click();

    const clipboard = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboard).toContain("Install the kapi skill");
    expect(clipboard).toContain("the Blank Inc. workspace");
  });
});
