import { test, expect, type Page } from "@playwright/test";
import {
  authenticate,
  getOrCreateWorkspace,
  createAnonymousProject,
  claimProject,
  waitForServer,
} from "./helpers/api-client";

const BASE_URL = process.env.BOWRAIN_URL || "http://localhost:8080";

/** Inject the auth token as an HttpOnly cookie via Playwright's cookie API. */
async function injectAuthCookie(page: Page, authToken: string) {
  const url = new URL(BASE_URL);
  await page.context().addCookies([{
    name: "bowrain_session",
    value: authToken,
    domain: url.hostname,
    path: "/api/",
    httpOnly: true,
    sameSite: "Lax" as const,
  }]);
}

let token: string;
let _wsSlug: string;

test.describe("Claim Flow", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(token, "Acme Inc.", "acme");
    _wsSlug = ws.slug;
  });

  test("shows sign-in prompt for unauthenticated user", async ({ page }) => {
    // Create an anonymous project to get a claim token.
    const { claim_token } = await createAnonymousProject(
      "Unclaimed Project",
      "en",
      ["fr"],
    );

    // Navigate to the claim URL without auth.
    await page.goto(`/claim/${claim_token}`);

    // Should show the "Sign in to claim" page.
    await expect(page.getByText("Claim Project")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Sign in to claim this project")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in to claim" })).toBeVisible();
  });

  test("auto-claims project when authenticated", async ({ page }) => {
    // Create an anonymous project.
    const { claim_token } = await createAnonymousProject(
      "Auto-Claim Project",
      "en",
      ["fr", "de"],
    );

    // Inject auth and navigate to claim URL.
    await injectAuthCookie(page, token);
    await page.goto(`/claim/${claim_token}`);

    // Auto-claim should trigger and show success.
    await expect(page.getByText("Project Claimed!")).toBeVisible({ timeout: 15000 });
    await expect(page.getByRole("button", { name: "Go to workspace" })).toBeVisible();
  });

  test("shows error for invalid claim token", async ({ page }) => {
    // Inject auth and navigate with a bogus token.
    await injectAuthCookie(page, token);
    await page.goto("/claim/invalid-token-that-does-not-exist");

    // Should show an error and "Try again" button.
    await expect(page.getByText("Claim Project")).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole("button", { name: "Try again" })).toBeVisible({ timeout: 10000 });
  });

  test("navigates to workspace after claiming", async ({ page }) => {
    // Create an anonymous project.
    const { claim_token } = await createAnonymousProject(
      "Navigate-After-Claim Project",
      "en",
      ["nb"],
    );

    // Inject auth and claim.
    await injectAuthCookie(page, token);
    await page.goto(`/claim/${claim_token}`);

    await expect(page.getByText("Project Claimed!")).toBeVisible({ timeout: 15000 });

    // Click "Go to workspace" — should clear claim route and load main app.
    await page.getByRole("button", { name: "Go to workspace" }).click();

    // URL should no longer contain /claim/.
    await expect(page).not.toHaveURL(/\/claim\//);
  });

  test("claim via API works correctly", async () => {
    // Create anonymous project and claim via API helper.
    const { project_id, claim_token } = await createAnonymousProject(
      "API Claim Project",
      "en",
      ["fr"],
    );

    const result = await claimProject(token, claim_token);
    expect(result.project_id).toBe(project_id);
    expect(result.workspace_slug).toBeTruthy();
  });
});
