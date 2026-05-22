/**
 * Walkthrough: bowrain-web-claim-project
 * Scene 1: claim-project (web)
 *
 * Generated from bowrain/web/docs/walkthroughs/bowrain-web-claim-project.md.
 *
 * Records the unauthenticated ClaimPage state — the entry point for a
 * developer who pushed an anonymous project via `kapi push` and then
 * opens the returned claim URL in a browser before signing in.
 *
 * Seeding: creates an anonymous project via REST against BOWRAIN_BACKEND_URL.
 * Recording: Playwright video capture (configured at the test level below).
 * Output: bowrain/web/docs/scenes/bowrain-web-claim-project/01-claim-project.webm
 */

import { test, expect } from "@playwright/test";
// Direct path to TEST_IDS — avoids the @neokapi/ui barrel JSON-import issue
// in Playwright's Node-ESM loader. See bowrain/packages/ui/test-ids.ts.
import { TEST_IDS } from "../../../packages/ui/src/test-ids";

const BACKEND_URL = process.env.BOWRAIN_BACKEND_URL || "https://dev.bowrain.cloud";

async function createAnonymousProject(): Promise<string> {
  const token = process.env.BOWRAIN_TOKEN;
  if (!token) throw new Error("BOWRAIN_TOKEN required (run scripts/sync-bowrain-secrets-to-gh.sh)");
  const resp = await fetch(`${BACKEND_URL}/api/v1/projects/anonymous`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: JSON.stringify({
      name: "Walkthrough demo",
      default_source_language: "en",
      target_languages: ["fr", "de"],
    }),
  });
  if (!resp.ok) throw new Error(`anonymous project create failed: ${resp.status}`);
  const body = (await resp.json()) as { claim_token: string };
  return body.claim_token;
}

test.describe("walkthrough: bowrain-web-claim-project", () => {
  test("claim-project [scene]", async ({ page }) => {
    const claimToken = await createAnonymousProject();

    // Navigate to the claim URL — same path a real user would follow
    // from the link printed by `kapi push` against an anonymous project.
    await page.goto(`${BACKEND_URL}/claim/${claimToken}`);

    // Unauthenticated state: the ClaimPage card renders.
    await expect(page.getByText("Claim Project")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Sign in to claim this project")).toBeVisible();
    await expect(page.getByTestId(TEST_IDS.auth.claimSubmit)).toBeVisible();

    // Hold the final frame for ~2s so the recording's last frame is the CTA.
    await page.waitForTimeout(2000);
  });

  // Copy each test's video.webm to the canonical scene path. Runs after
  // the test context closes, so the file is finalized.
  test.afterEach(async ({ page }, testInfo) => {
    const video = page.video();
    if (!video) return;
    await page.close(); // ensure recording is flushed
    const src = await video.path();
    const fs = await import("node:fs");
    const path = await import("node:path");
    const dest = path.resolve(__dirname, "01-claim-project.webm");
    fs.copyFileSync(src, dest);
    testInfo.attachments.push({ name: "01-claim-project.webm", path: dest, contentType: "video/webm" });
  });
});
