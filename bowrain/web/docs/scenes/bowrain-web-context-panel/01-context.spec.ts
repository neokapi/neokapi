/**
 * Walkthrough: bowrain-web-context-panel
 * Scene 1: context (web)
 *
 * Records the translation editor with the TM + Terminology context panel
 * open, showing a translator the inline reference material that drives
 * consistent translations.
 *
 * Seeding: ensures a project exists in the personal workspace and uploads
 * a small JSON fixture with translatable items.
 */

import { test, expect } from "@playwright/test";
import { BACKEND_URL, injectAuthCookie, getMyWorkspaceSlug, saveSceneVideo } from "../_helpers";

const FIXTURE_NAME = "messages.json";
const FIXTURE_BODY = JSON.stringify(
  {
    greeting: "Hello, World!",
    welcome: "Welcome to our application",
    buttons: { save: "Save changes", cancel: "Cancel and discard", submit: "Submit form" },
    errors: { required: "This field is required", tooLong: "Value is too long" },
  },
  null,
  2,
);

async function getOrCreateProject(slug: string, token: string): Promise<string> {
  const list = (await fetch(`${BACKEND_URL}/api/v1/${slug}/projects`, {
    headers: { Authorization: `Bearer ${token}` },
  }).then((r) => r.json())) as { id: string; name: string }[];
  const match = list.find((p) => p.name === "Walkthrough demo");
  if (match) return match.id;
  const created = (await fetch(`${BACKEND_URL}/api/v1/${slug}/projects`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: JSON.stringify({
      name: "Walkthrough demo",
      default_source_language: "en",
      target_languages: ["fr", "de"],
    }),
  }).then((r) => r.json())) as { id: string };
  return created.id;
}

async function ensureFile(slug: string, projectId: string, token: string): Promise<void> {
  const items = (await fetch(
    `${BACKEND_URL}/api/v1/${slug}/${projectId}/items/main?item=${FIXTURE_NAME}`,
    { headers: { Authorization: `Bearer ${token}` } },
  ).then((r) => (r.ok ? r.json() : []))) as unknown[];
  if (Array.isArray(items) && items.length > 0) return;
  const fd = new FormData();
  fd.append("files", new Blob([FIXTURE_BODY], { type: "application/json" }), FIXTURE_NAME);
  const resp = await fetch(`${BACKEND_URL}/api/v1/${slug}/${projectId}/items/main`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: fd,
  });
  if (!resp.ok) throw new Error(`upload failed: ${resp.status} ${await resp.text()}`);
}

test.describe("walkthrough: bowrain-web-context-panel", () => {
  test("context [scene]", async ({ page }) => {
    const token = process.env.BOWRAIN_SESSION_TOKEN;
    if (!token) throw new Error("BOWRAIN_SESSION_TOKEN required");
    await injectAuthCookie(page);
    const slug = await getMyWorkspaceSlug();
    const projectId = await getOrCreateProject(slug, token);
    await ensureFile(slug, projectId, token);

    await page.goto(`${BACKEND_URL}/${slug}/p/${projectId}/s/main/${FIXTURE_NAME}/translate`);
    // Editor loads in the Visual view by default — switch to the Table view
    // so the context panel + its toggle are rendered.
    await expect(page.getByTestId("view-switcher")).toBeVisible({ timeout: 20000 });
    await page.getByTestId("view-table").click();
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    // Open the TM + Terminology side panel.
    await page.getByTestId("context-panel-toggle").click();
    await expect(page.getByTestId("context-panel")).toBeVisible({ timeout: 5000 });
    // Hold the final frame so the recording's last 2s is the open panel.
    await page.waitForTimeout(2500);
  });
  test.afterEach(async ({ page }, testInfo) => saveSceneVideo(page, testInfo, "01-context.webm"));
});
