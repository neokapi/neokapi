/**
 * Walkthrough: bowrain-web-login-and-workspace
 * Scene 1: login-workspace (web)
 *
 * Generated from bowrain/website/walkthroughs/bowrain-web-login-and-workspace.md.
 *
 * Records the post-SSO landing — workspace dashboard with the personal workspace
 * shown in the rail and the main pane. Bypasses the actual SSO redirect by
 * injecting a pre-authenticated session cookie (BOWRAIN_SESSION_TOKEN).
 *
 * Requires:
 *   BOWRAIN_BACKEND_URL    e.g. https://dev.bowrain.cloud
 *   BOWRAIN_SESSION_TOKEN  user JWT from device-auth.sh, post-onboarding
 */

import { test, expect } from "@playwright/test";

const BACKEND_URL = process.env.BOWRAIN_BACKEND_URL || "https://dev.bowrain.cloud";

async function injectAuthCookie(page: import("@playwright/test").Page) {
  const token = process.env.BOWRAIN_SESSION_TOKEN;
  if (!token) throw new Error("BOWRAIN_SESSION_TOKEN required");
  const url = new URL(BACKEND_URL);
  await page.context().addCookies([
    {
      name: "bowrain_session",
      value: token,
      domain: url.hostname,
      path: "/",
      httpOnly: true,
      sameSite: "Lax",
      secure: url.protocol === "https:",
    },
  ]);
}

async function getMyWorkspaceSlug(): Promise<string> {
  const token = process.env.BOWRAIN_SESSION_TOKEN;
  if (!token) throw new Error("BOWRAIN_SESSION_TOKEN required");
  const resp = await fetch(`${BACKEND_URL}/api/v1/workspaces`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!resp.ok) throw new Error(`workspace list failed: ${resp.status}`);
  const list = (await resp.json()) as { slug: string; type?: string }[];
  const personal = list.find((w) => w.type === "personal");
  return (personal ?? list[0]).slug;
}

test.describe("walkthrough: bowrain-web-login-and-workspace", () => {
  test("login-workspace [scene]", async ({ page }) => {
    await injectAuthCookie(page);
    const slug = await getMyWorkspaceSlug();

    // Land on the workspace dashboard — the post-SSO destination.
    await page.goto(`${BACKEND_URL}/${slug}`);

    // The sidebar nav anchors confirm we're on the dashboard.
    await expect(page.getByTestId("nav-translate")).toBeVisible({ timeout: 15000 });

    // Hold the final frame for ~2s so the recording's last frame is the dashboard.
    await page.waitForTimeout(2000);
  });

  test.afterEach(async ({ page }, testInfo) => {
    const video = page.video();
    if (!video) return;
    await page.close();
    const src = await video.path();
    const fs = await import("node:fs");
    const path = await import("node:path");
    const dest = path.resolve(__dirname, "01-login-workspace.webm");
    fs.copyFileSync(src, dest);
    testInfo.attachments.push({
      name: "01-login-workspace.webm",
      path: dest,
      contentType: "video/webm",
    });
  });
});
