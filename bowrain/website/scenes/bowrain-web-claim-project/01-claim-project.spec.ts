/**
 * Walkthrough: bowrain-web-claim-project
 * Scene 1: claim-project (web)
 *
 * Generated from bowrain/website/walkthroughs/bowrain-web-claim-project.md.
 * Do not edit by hand — change the prompt and regenerate via /walkthrough-scenes.
 *
 * Records a real-backend Playwright session: anonymous project creation
 * via BowrainAPI, then user opens the claim URL and walks through the
 * sign-in + claim flow. Output: claim-project.webm at project root.
 */

import { test, expect } from "@playwright/test";
import { TEST_IDS } from "@neokapi/ui";
// import { BowrainAPI } from "../../../../e2e/shared/index";

const BACKEND_URL = process.env.BOWRAIN_BACKEND_URL || "https://dev.bowrain.cloud";

test.describe("walkthrough: bowrain-web-claim-project", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  // TODO(#425): wire up workspace seed + auth via BowrainAPI:
  //   const api = new BowrainAPI(BACKEND_URL);
  //   const token = await api.deviceAuth();
  //   const ws = await api.createWorkspace(`recordings-${Date.now().toString(36)}`);
  //   const { claim_token } = await api.createAnonymousProject({
  //     name: "Demo project", source_lang: "en", target_langs: ["fr", "de"],
  //   });
  //   afterAll: await api.deleteWorkspace(ws.slug);

  test("claim-project [scene]", async ({ page }) => {
    // 1. User receives claim URL from `bowrain push` CLI output.
    //    Navigate to it.
    // await page.goto(`${BACKEND_URL}/claim/${claim_token}`);

    // 2. Unauthenticated state: ClaimPage shows "Sign in to claim" CTA.
    // await expect(page.getByTestId(TEST_IDS.auth.claimSubmit)).toBeVisible();

    // 3. Click → SSO redirect → return to claim page authenticated.
    // await page.getByTestId(TEST_IDS.auth.claimSubmit).click();
    // ... handle SSO redirect with the seeded token ...

    // 4. Authenticated claim view: green "Claim project" CTA.
    // await page.getByTestId(TEST_IDS.workspace.claimGreenSubmit).click();

    // 5. Redirected to workspace dashboard, project card visible.
    // await expect(page.getByTestId(TEST_IDS.workspace.projectCard)).toBeVisible();

    test.skip(true, "scaffold — needs real backend validation per #425 followup");
    expect(BACKEND_URL).toBeTruthy();
    expect(TEST_IDS.auth.claimSubmit).toBeTruthy();
  });
});
