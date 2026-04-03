import { test, expect, type Page } from "@playwright/test";
import {
  authenticate,
  getOrCreateWorkspace,
  createEditorProject,
  getEditorProject,
  findItemId,
  uploadSeedFiles,
  deleteAllEditorProjects,
  seedTMEntries,
  seedConcepts,
  pseudoTranslateFile,
  waitForServer,
} from "./helpers/api-client";
import {
  injectCursor,
  moveCursorTo,
  humanClick,
  humanClickNative,
  humanType,
  pause,
} from "./helpers/cursor-helper";
import { BowrainAPI, fullSeed, type StoryContext } from "../../../e2e/shared/index";

const isCI = !!process.env.CI && !process.env.FORCE_RECORDINGS;

const BASE_URL =
  process.env.BOWRAIN_SERVER_URL || process.env.BOWRAIN_URL || "http://localhost:8080";

/** Inject the auth token as an HttpOnly cookie via Playwright's cookie API. */
async function injectAuthCookie(page: Page, authToken: string) {
  const url = new URL(BASE_URL);
  await page.context().addCookies([
    {
      name: "bowrain_session",
      value: authToken,
      domain: url.hostname,
      path: "/api/",
      httpOnly: true,
      sameSite: "Lax",
    },
  ]);
}

async function setTheme(page: Page, theme: "dark" | "light") {
  await page.evaluate((t) => {
    const isDark = t === "dark";
    document.documentElement.classList.toggle("dark", isDark);
    localStorage.setItem("neokapi-theme", t);
  }, theme);
  await page.waitForTimeout(100);
}

/** Switch to grid layout (default is now "visual"). */
async function switchToGrid(page: Page) {
  await expect(page.getByTestId("layout-switcher")).toBeVisible({ timeout: 30000 });
  await page.getByTestId("layout-grid").click();
  await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 10000 });
}

let token: string;
let wsSlug: string;
/** Story context from the shared seeder (populated with brand, tasks, etc.). */
let _storyCtx: StoryContext | null = null;

const themes = ["dark", "light"] as const;

test.describe("Web App Recordings", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const slug = `recordings-${Date.now().toString(36)}`;
    const ws = await getOrCreateWorkspace(token, "Acme Inc.", slug);
    wsSlug = ws.slug;

    // Run the shared seeder for story-feature recordings.
    try {
      const api = new BowrainAPI(BASE_URL, token);
      _storyCtx = await fullSeed(api);
    } catch (err) {
      console.log(
        `[recordings] Shared seeder failed (story recordings will be skipped): ${String(err)}`,
      );
      _storyCtx = null;
    }
  });

  /** Navigate directly to the workspace dashboard and set up recording chrome. */
  async function setupRecording(page: Page, theme: "dark" | "light") {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);
    // Wait for app to fully load (sidebar nav should be visible)
    await expect(page.getByTestId("nav-translate")).toBeVisible({ timeout: 15000 });
    await setTheme(page, theme);
    await page.waitForTimeout(500);
    await injectCursor(page);
    await moveCursorTo(page, 640, 400, 0);
  }

  for (const theme of themes) {
    // ── Login & workspace ──────────────────────────────────────────────────

    test(`record login-and-workspace [${theme}]`, async ({ page }) => {
      test.skip(isCI, "Recording tests are skipped in CI");

      const hasToken = !!process.env.BOWRAIN_TOKEN;

      if (hasToken) {
        // Token injection mode — skip Keycloak, go straight to workspace
        await setupRecording(page, theme);
      } else {
        // Navigate without token — Auth Code + PKCE redirects to Keycloak login
        await page.goto("/");
        // Wait for Keycloak login page
        await page.waitForURL("**/realms/bowrain/**", { timeout: 15000 });
        await pause(page, 1500);

        // Fill in credentials on Keycloak login form
        await page.locator("#username").fill("admin@example.com");
        await pause(page, 500);
        await page.locator("#password").fill("password");
        await pause(page, 500);

        // Submit login — Keycloak redirects back to Bowrain with auth code
        await page.locator("#kc-login").click();

        // Wait for redirect back to Bowrain app (lands on /$workspace)
        await page.waitForURL(`${BASE_URL}/**`, { timeout: 15000 });
        await expect(page.getByTestId("nav-translate")).toBeVisible({ timeout: 15000 });
        await setTheme(page, theme);
        await injectCursor(page);
        await moveCursorTo(page, 640, 400, 0);
      }

      // Wait for workspace to load
      await page.waitForTimeout(2000);
      await expect(page.getByTestId("nav-translate")).toBeVisible({ timeout: 10000 });

      // Navigate through sidebar views — each click navigates to a route
      await humanClick(page, page.getByTestId("nav-translate"));
      await pause(page, 1000);

      await humanClick(page, page.getByTestId("nav-memory"));
      await pause(page, 1000);

      await humanClick(page, page.getByTestId("nav-termbase"));
      await pause(page, 1000);

      await humanClick(page, page.getByTestId("nav-settings"));
      await pause(page, 1500);

      // Navigate back to translate
      await humanClick(page, page.getByTestId("nav-translate"));
      await pause(page, 1000);
    });

    // ── Translation editor workflow ────────────────────────────────────────

    test(`record translation-editor [${theme}]`, async ({ page }) => {
      test.skip(isCI, "Recording tests are skipped in CI");

      // Clean and seed project with files
      await deleteAllEditorProjects(token, wsSlug);
      const p = await createEditorProject(token, wsSlug, "Company Website", "en", ["fr", "de"]);
      await uploadSeedFiles(token, wsSlug, p.id, ["about-us.html"]);
      await seedTMEntries(token, wsSlug);

      await setupRecording(page, theme);

      // Wait for projects to load
      await expect(page.getByText("Company Website").first()).toBeVisible({ timeout: 10000 });
      await pause(page, 1000);

      // Open project
      const projectCard = page.getByText("Company Website").first();
      await humanClick(page, projectCard);
      await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
      await pause(page, 1000);

      // Open file in editor
      await humanClickNative(page, "open-file-about-us.html");
      await switchToGrid(page);
      await pause(page, 1500);

      // Click on the first block to select it
      const firstBlock = page.getByTestId("block-row-0");
      await humanClick(page, firstBlock);
      await pause(page, 800);

      // Type a translation
      const targetCell = page.getByTestId("edit-target-0");
      if (await targetCell.isVisible()) {
        await humanType(page, targetCell, "A propos d'Acme Inc.");
        await pause(page, 800);
      }

      // Navigate to next block
      await page.keyboard.press("Enter");
      await pause(page, 500);

      // Navigate a few blocks down
      await page.keyboard.press("ArrowDown");
      await pause(page, 300);
      await page.keyboard.press("ArrowDown");
      await pause(page, 500);

      // Pseudo-translate remaining blocks via API
      await pseudoTranslateFile(token, wsSlug, p.id, "about-us.html", "fr");
      await page.reload();
      await switchToGrid(page);
      await pause(page, 2000);

      // Clean up
      try {
        await fetch(`${BASE_URL}/api/v1/workspaces/${wsSlug}/editor/projects/${p.id}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        });
      } catch {
        /* ignore */
      }
    });

    // ── Focus view ─────────────────────────────────────────────────────────

    test(`record focus-view [${theme}]`, async ({ page }) => {
      test.skip(isCI, "Recording tests are skipped in CI");

      await deleteAllEditorProjects(token, wsSlug);
      const p = await createEditorProject(token, wsSlug, "Release Notes", "en", ["fr"]);
      await uploadSeedFiles(token, wsSlug, p.id, ["release-notes.md"]);

      // Navigate directly to the editor route (resolve item ID first)
      const projRelease = await getEditorProject(token, wsSlug, p.id);
      const releaseItemId = findItemId(projRelease, "release-notes.md");
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/p/${p.id}/s/main/${releaseItemId}/translate`);
      await switchToGrid(page);
      await setTheme(page, theme);
      await injectCursor(page);
      await moveCursorTo(page, 640, 400, 0);
      await pause(page, 1000);

      // Pseudo-translate via API so there's content
      await pseudoTranslateFile(token, wsSlug, p.id, "release-notes.md", "fr");
      await page.reload();
      await switchToGrid(page);
      await pause(page, 1000);

      // Switch to focus view
      await humanClick(page, page.getByTestId("layout-focus"));
      await expect(page.getByTestId("focus-view")).toBeVisible({ timeout: 5000 });
      await pause(page, 1500);

      // Navigate between blocks
      await humanClick(page, page.getByTestId("focus-next"));
      await pause(page, 1000);
      await humanClick(page, page.getByTestId("focus-next"));
      await pause(page, 1000);
      await humanClick(page, page.getByTestId("focus-prev"));
      await pause(page, 1500);

      try {
        await fetch(`${BASE_URL}/api/v1/workspaces/${wsSlug}/editor/projects/${p.id}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        });
      } catch {
        /* ignore */
      }
    });

    // ── Pseudo-translation workflow ────────────────────────────────────────

    test(`record pseudo-translation [${theme}]`, async ({ page }) => {
      test.skip(isCI, "Recording tests are skipped in CI");

      await deleteAllEditorProjects(token, wsSlug);
      const p = await createEditorProject(token, wsSlug, "Mobile App", "en", ["fr", "de"]);
      await uploadSeedFiles(token, wsSlug, p.id, ["app-strings.json"]);

      // Navigate directly to the editor route (resolve item ID first)
      const projMobile = await getEditorProject(token, wsSlug, p.id);
      const mobileItemId = findItemId(projMobile, "app-strings.json");
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/p/${p.id}/s/main/${mobileItemId}/translate`);
      await switchToGrid(page);
      await setTheme(page, theme);
      await injectCursor(page);
      await moveCursorTo(page, 640, 400, 0);
      await pause(page, 1000);

      // Show the empty state first
      await pause(page, 1500);

      // Pseudo-translate via API
      await pseudoTranslateFile(token, wsSlug, p.id, "app-strings.json", "fr");
      await page.reload();
      await switchToGrid(page);
      await expect(page.getByTestId("progress-text")).toContainText("100%", { timeout: 10000 });
      await pause(page, 2000);

      try {
        await fetch(`${BASE_URL}/api/v1/workspaces/${wsSlug}/editor/projects/${p.id}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        });
      } catch {
        /* ignore */
      }
    });

    // ── TM Explorer ────────────────────────────────────────────────────────

    test(`record tm-explorer [${theme}]`, async ({ page }) => {
      test.skip(isCI, "Recording tests are skipped in CI");

      await seedTMEntries(token, wsSlug);

      // Navigate directly to the TM explorer route
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/memory`);
      await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 10000 });
      await setTheme(page, theme);
      await injectCursor(page);
      await moveCursorTo(page, 640, 400, 0);
      await pause(page, 1500);

      // Search for an entry
      const tmSearchWrapper = page.getByTestId("tm-search-input");
      if (await tmSearchWrapper.isVisible()) {
        const tmInput = tmSearchWrapper.locator("input").first();
        await humanType(page, tmInput, "welcome");
        await page.keyboard.press("Enter");
        await pause(page, 2000);

        // Clear search
        await tmInput.fill("");
        await page.keyboard.press("Enter");
        await pause(page, 1000);
      }

      // Show the add entry form
      const addBtn = page.getByTestId("tm-add-entry-btn");
      if (await addBtn.isVisible()) {
        await humanClick(page, addBtn);
        await pause(page, 1500);
      }

      await pause(page, 1500);
    });

    // ── Term Explorer ──────────────────────────────────────────────────────

    test(`record term-explorer [${theme}]`, async ({ page }) => {
      test.skip(isCI, "Recording tests are skipped in CI");

      await seedConcepts(token, wsSlug);

      // Navigate directly to the termbase route
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/termbase`);
      await expect(page.getByTestId("term-explorer")).toBeVisible({ timeout: 10000 });
      await setTheme(page, theme);
      await injectCursor(page);
      await moveCursorTo(page, 640, 400, 0);
      await pause(page, 1500);

      // Search for a term
      const termSearchWrapper = page.getByTestId("term-search-input");
      if (await termSearchWrapper.isVisible()) {
        const termInput = termSearchWrapper.locator("input").first();
        await humanType(page, termInput, "deploy");
        await page.keyboard.press("Enter");
        await pause(page, 2000);

        // Clear search
        await termInput.fill("");
        await page.keyboard.press("Enter");
        await pause(page, 1000);
      }

      // Show the add concept form
      const addBtn = page.getByTestId("term-add-btn");
      if (await addBtn.isVisible()) {
        await humanClick(page, addBtn);
        await pause(page, 1500);
      }

      await pause(page, 1500);
    });

    // ── Context Panel ──────────────────────────────────────────────────────

    test(`record context-panel [${theme}]`, async ({ page }) => {
      test.skip(isCI, "Recording tests are skipped in CI");

      await deleteAllEditorProjects(token, wsSlug);
      const p = await createEditorProject(token, wsSlug, "Context Demo", "en", ["fr"]);
      await uploadSeedFiles(token, wsSlug, p.id, ["about-us.html"]);
      await seedTMEntries(token, wsSlug);
      await seedConcepts(token, wsSlug);

      // Navigate directly to the editor route (resolve item ID first)
      const projWebsite = await getEditorProject(token, wsSlug, p.id);
      const websiteItemId = findItemId(projWebsite, "about-us.html");
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/p/${p.id}/s/main/${websiteItemId}/translate`);
      await switchToGrid(page);
      await setTheme(page, theme);
      await injectCursor(page);
      await moveCursorTo(page, 640, 400, 0);
      await pause(page, 1000);

      // Open context panel
      const contextToggle = page.getByTestId("context-panel-toggle");
      if (await contextToggle.isVisible()) {
        await humanClick(page, contextToggle);
        await pause(page, 1000);
      }

      // Click on different blocks to show TM + term matches in context panel
      await humanClick(page, page.getByTestId("block-row-0"));
      await pause(page, 2000);

      await humanClick(page, page.getByTestId("block-row-1"));
      await pause(page, 2000);

      await humanClick(page, page.getByTestId("block-row-2"));
      await pause(page, 2000);

      try {
        await fetch(`${BASE_URL}/api/v1/workspaces/${wsSlug}/editor/projects/${p.id}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        });
      } catch {
        /* ignore */
      }
    });

    // ── Settings ───────────────────────────────────────────────────────────

    test(`record settings [${theme}]`, async ({ page }) => {
      test.skip(isCI, "Recording tests are skipped in CI");

      // Navigate directly to settings route
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/settings`);
      await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 10000 });
      await setTheme(page, theme);
      await injectCursor(page);
      await moveCursorTo(page, 640, 400, 0);
      await pause(page, 2000);

      // Hover over workspace info fields
      await moveCursorTo(page, 400, 300, 500);
      await pause(page, 1000);
      await moveCursorTo(page, 400, 350, 500);
      await pause(page, 1000);
      await moveCursorTo(page, 400, 400, 500);
      await pause(page, 2000);
    });

    // ── Invite workflow ───────────────────────────────────────────────────

    test(`record invite-workflow [${theme}]`, async ({ page }) => {
      test.skip(isCI, "Recording tests are skipped in CI");
      test.setTimeout(180_000); // invite workflow needs extra time for human-speed interactions

      // Navigate directly to the members settings route (where invite-manager lives)
      await injectAuthCookie(page, token);
      await page.goto(`/${wsSlug}/settings/members`);
      await expect(
        page
          .getByRole("heading", { name: "Members" })
          .or(page.getByRole("heading", { name: "Settings" })),
      ).toBeVisible({ timeout: 10000 });
      await setTheme(page, theme);
      await injectCursor(page);
      await moveCursorTo(page, 640, 400, 0);
      await pause(page, 1500);

      // Scroll to Invitations section
      const inviteSection = page.getByTestId("invite-manager");
      await expect(inviteSection).toBeVisible({ timeout: 5000 });
      await inviteSection.scrollIntoViewIfNeeded();
      await pause(page, 1000);

      // Open the invite dialog
      await humanClick(page, page.getByTestId("invite-open-dialog-btn"));
      await pause(page, 800);

      // Fill in email for new invite (role defaults to "Member")
      const emailInput = page.getByTestId("invite-email-input");
      await humanType(page, emailInput, "translator@example.com");
      await pause(page, 800);

      // Submit the invite
      const submitBtn = page.getByTestId("invite-submit-btn");
      await humanClick(page, submitBtn);
      await pause(page, 2000);

      // Copy the invite link
      const copyBtn = page.getByTestId("invite-copy-link-btn").first();
      if (await copyBtn.isVisible()) {
        await humanClick(page, copyBtn);
        await pause(page, 1500);
      }

      // Show the invite list
      await pause(page, 2000);
    });
  }
});
