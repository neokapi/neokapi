/**
 * Walkthrough: bowrain-desktop-settings-configuration
 * Scene 1: settings (desktop)
 *
 * Generated from bowrain/website/walkthroughs/bowrain-desktop-settings-configuration.md.
 * Do not edit by hand — change the prompt and regenerate via /walkthrough-scenes.
 *
 * Scaffold pending real-backend validation. Run against BOWRAIN_BACKEND_URL
 * with a seeded workspace via BowrainAPI; cleanup in afterAll.
 */

import { test, expect } from "@playwright/test";
import { TEST_IDS } from "@neokapi/ui/test-ids";

const BACKEND_URL = process.env.BOWRAIN_BACKEND_URL || "https://dev.bowrain.cloud";

test.describe("walkthrough: bowrain-desktop-settings-configuration", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  // TODO(#425): seed via BowrainAPI; cleanup in afterAll.

  test("settings [scene]", async ({ page }) => {
    test.skip(true, "scaffold — needs real backend validation per #425 followup");
    expect(BACKEND_URL).toBeTruthy();
    expect(TEST_IDS).toBeTruthy();
  });
});
