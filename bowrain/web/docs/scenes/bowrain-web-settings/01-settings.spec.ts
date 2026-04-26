/**
 * Walkthrough: bowrain-web-settings
 * Scene 1: settings (web)
 *
 * Records the workspace settings landing — Settings page with General card.
 */

import { test, expect } from "@playwright/test";
import { BACKEND_URL, injectAuthCookie, getMyWorkspaceSlug, saveSceneVideo } from "../_helpers";

test.describe("walkthrough: bowrain-web-settings", () => {
  test("settings [scene]", async ({ page }) => {
    await injectAuthCookie(page);
    const slug = await getMyWorkspaceSlug();
    await page.goto(`${BACKEND_URL}/${slug}/settings`);
    await expect(page.getByTestId("settings-heading")).toBeVisible({ timeout: 15000 });
    await page.waitForTimeout(2000);
  });
  test.afterEach(async ({ page }, testInfo) => saveSceneVideo(page, testInfo, "01-settings.webm"));
});
