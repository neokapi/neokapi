/**
 * Walkthrough: bowrain-desktop-translation-editor
 * Scene 1: open-and-translate (desktop)
 *
 * Generated from bowrain/website/walkthroughs/bowrain-desktop-translation-editor.md.
 * Do not edit by hand — change the prompt and regenerate via /walkthrough-scenes.
 *
 * Records a real-backend Playwright session against Wails dev mode.
 * The Go backend talks to bowrain-server at BOWRAIN_BACKEND_URL via
 * the existing client. No mock-backend.ts; no Wails RPC interception.
 */

import { test, expect } from "@playwright/test";
import { TEST_IDS } from "@neokapi/ui";
// import { BowrainAPI } from "../../../../e2e/shared/index";

const BACKEND_URL = process.env.BOWRAIN_BACKEND_URL || "https://dev.bowrain.cloud";

test.describe("walkthrough: bowrain-desktop-translation-editor", () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  // TODO(#425): seed setup — workspace + project + files via BowrainAPI,
  // then launch Wails dev mode pointed at BOWRAIN_BACKEND_URL with the
  // recorded session token. Cleanup in afterAll deletes the workspace.

  test("open-and-translate [scene]", async ({ page }) => {
    // 1. App launches at the workspace rail.
    // await expect(page.getByTestId(TEST_IDS.workspace.rail)).toBeVisible();

    // 2. Click the seeded "Acme Marketing" project card.
    // await page.getByTestId(TEST_IDS.workspace.projectCard).first().click();

    // 3. File list visible; click into i18n/en.json.
    // await expect(page.getByTestId(TEST_IDS.project.fileList)).toBeVisible();
    // await page.getByTestId(TEST_IDS.project.fileRow).first().click();

    // 4. Translation editor open with 3 blocks.
    // const blocks = page.getByTestId(TEST_IDS.editor.block);
    // await expect(blocks).toHaveCount(3);

    // 5. Click target cell of first block, type translation, save.
    // const firstTarget = blocks.first().getByTestId(TEST_IDS.editor.blockTarget);
    // await firstTarget.click();
    // await page.keyboard.type("Bienvenue", { delay: 50 });
    // await page.getByTestId(TEST_IDS.editor.saveButton).click();

    test.skip(true, "scaffold — needs real backend + Wails dev mode validation per #425 followup");
    expect(BACKEND_URL).toBeTruthy();
    expect(TEST_IDS.editor.saveButton).toBeTruthy();
  });
});
