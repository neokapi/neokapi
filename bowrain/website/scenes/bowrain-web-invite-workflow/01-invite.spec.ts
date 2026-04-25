/**
 * Walkthrough: bowrain-web-invite-workflow
 * Scene 1: invite (web)
 *
 * Records the workspace settings → members tab — the entry point for the
 * "Invite member" CTA.
 */

import { test, expect } from "@playwright/test";
import { BACKEND_URL, injectAuthCookie, getMyWorkspaceSlug, saveSceneVideo } from "../_helpers";

test.describe("walkthrough: bowrain-web-invite-workflow", () => {
  test("invite [scene]", async ({ page }) => {
    await injectAuthCookie(page);
    const slug = await getMyWorkspaceSlug();
    await page.goto(`${BACKEND_URL}/${slug}/settings/members`);
    await expect(page.getByTestId("invite-manager")).toBeVisible({ timeout: 15000 });
    await page.waitForTimeout(2000);
  });
  test.afterEach(async ({ page }, testInfo) => saveSceneVideo(page, testInfo, "01-invite.webm"));
});
