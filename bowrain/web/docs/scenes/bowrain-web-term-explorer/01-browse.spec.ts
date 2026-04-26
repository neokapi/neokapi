/**
 * Walkthrough: bowrain-web-term-explorer
 * Scene 1: browse (web)
 *
 * Records the workspace term explorer landing — empty state for a fresh
 * personal workspace.
 */

import { test, expect } from "@playwright/test";
import { BACKEND_URL, injectAuthCookie, getMyWorkspaceSlug, saveSceneVideo } from "../_helpers";

test.describe("walkthrough: bowrain-web-term-explorer", () => {
  test("browse [scene]", async ({ page }) => {
    await injectAuthCookie(page);
    const slug = await getMyWorkspaceSlug();
    await page.goto(`${BACKEND_URL}/${slug}/termbase`);
    await expect(page.getByTestId("term-explorer")).toBeVisible({ timeout: 15000 });
    await page.waitForTimeout(2000);
  });
  test.afterEach(async ({ page }, testInfo) => saveSceneVideo(page, testInfo, "01-browse.webm"));
});
