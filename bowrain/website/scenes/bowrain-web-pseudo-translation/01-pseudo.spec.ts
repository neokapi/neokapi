/**
 * Walkthrough: bowrain-web-pseudo-translation
 * Scene 1: pseudo (web)
 *
 * Records the project detail page — the entry point for triggering a
 * pseudo-translation flow on the project. The actual flow trigger UI is
 * surfaced from this view via the project menu.
 */

import { test, expect } from "@playwright/test";
import { BACKEND_URL, injectAuthCookie, getMyWorkspaceSlug, saveSceneVideo } from "../_helpers";

async function getOrCreateProject(slug: string): Promise<string> {
  const token = process.env.BOWRAIN_SESSION_TOKEN;
  if (!token) throw new Error("BOWRAIN_SESSION_TOKEN required");
  const list = await fetch(`${BACKEND_URL}/api/v1/${slug}/projects`, {
    headers: { Authorization: `Bearer ${token}` },
  }).then((r) => r.json() as Promise<{ id: string }[]>);
  if (list.length > 0) return list[0].id;
  const created = await fetch(`${BACKEND_URL}/api/v1/${slug}/projects`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: JSON.stringify({
      name: "Walkthrough demo",
      default_source_language: "en",
      target_languages: ["fr"],
    }),
  }).then((r) => r.json() as Promise<{ id: string }>);
  return created.id;
}

test.describe("walkthrough: bowrain-web-pseudo-translation", () => {
  test("pseudo [scene]", async ({ page }) => {
    await injectAuthCookie(page);
    const slug = await getMyWorkspaceSlug();
    const projectId = await getOrCreateProject(slug);
    await page.goto(`${BACKEND_URL}/${slug}/p/${projectId}/s/main`);
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 15000 });
    await page.waitForTimeout(2000);
  });
  test.afterEach(async ({ page }, testInfo) => saveSceneVideo(page, testInfo, "01-pseudo.webm"));
});
