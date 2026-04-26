/**
 * Shared helpers for walkthrough scenes. Imported by per-scene specs.
 */

import type { Page } from "@playwright/test";

export const BACKEND_URL = process.env.BOWRAIN_BACKEND_URL || "https://dev.bowrain.cloud";

export async function injectAuthCookie(page: Page) {
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

export async function getMyWorkspaceSlug(): Promise<string> {
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

/**
 * Save the recorded video to the canonical scene path. Call from test.afterEach.
 */
export async function saveSceneVideo(
  page: Page,
  testInfo: import("@playwright/test").TestInfo,
  destFile: string,
) {
  const video = page.video();
  if (!video) return;
  await page.close();
  const src = await video.path();
  const fs = await import("node:fs");
  const path = await import("node:path");
  const dest = path.resolve(path.dirname(testInfo.file), destFile);
  fs.copyFileSync(src, dest);
  testInfo.attachments.push({
    name: destFile,
    path: dest,
    contentType: "video/webm",
  });
}
