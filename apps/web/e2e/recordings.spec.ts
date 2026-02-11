import { test, expect } from "@playwright/test";
import {
  authenticate,
  getOrCreateWorkspace,
  createEditorProject,
  uploadSeedFiles,
  deleteAllEditorProjects,
  seedTMEntries,
  seedConcepts,
  waitForServer,
} from "./helpers/api-client";
import {
  injectCursor,
  moveCursorTo,
  humanClick,
  humanClickNative,
  humanType,
  humanTypeNative,
  pause,
} from "./helpers/cursor-helper";

const isCI = !!process.env.CI;

let token: string;
let wsSlug: string;

test.describe("Web App Recordings", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(token, "Acme Inc.", "acme");
    wsSlug = ws.slug;
  });

  async function setupRecording(page: import("@playwright/test").Page) {
    await page.goto(`/?token=${token}`);
    // Wait for app to fully load (sidebar nav should be visible)
    await expect(page.getByTestId("nav-translate")).toBeVisible({ timeout: 15000 });
    await page.waitForTimeout(500);
    await injectCursor(page);
    await moveCursorTo(page, 640, 400, 0);
  }

  // ── Login & workspace ────────────────────────────────────────────────────

  test("record login-and-workspace", async ({ page }) => {
    test.skip(isCI, "Recording tests are skipped in CI");

    // Show login page first (no token)
    await page.goto("/");
    await expect(page.getByText("Sign in with SSO")).toBeVisible({ timeout: 10000 });
    await injectCursor(page);
    await moveCursorTo(page, 640, 400, 0);

    await pause(page, 2000);

    // Move cursor to SSO button to highlight it
    const ssoBtn = page.getByText("Sign in with SSO");
    await humanClick(page, ssoBtn);

    // The actual SSO flow redirects — instead, simulate by navigating with token
    await pause(page, 1000);
    await page.goto(`/?token=${token}`);
    await injectCursor(page);
    await moveCursorTo(page, 640, 400, 0);

    // Wait for workspace to load
    await page.waitForTimeout(2000);
    await expect(page.getByText("Acme Inc.")).toBeVisible({ timeout: 10000 });

    // Hover over the sidebar navigation items
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

  // ── Translation editor workflow ──────────────────────────────────────────

  test("record translation-editor", async ({ page }) => {
    test.skip(isCI, "Recording tests are skipped in CI");

    // Clean and seed project with files
    await deleteAllEditorProjects(token, wsSlug);
    const p = await createEditorProject(token, wsSlug, "Company Website", "en", ["fr", "de"]);
    await uploadSeedFiles(token, wsSlug, p.id, ["about-us.html"]);
    await seedTMEntries(token, wsSlug);

    await setupRecording(page);

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
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
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

    // Use pseudo translate for remaining
    await humanClick(page, page.getByTestId("pseudo-btn"));
    await pause(page, 2000);

    // Show progress at 100%
    await pause(page, 1500);

    // Clean up
    try {
      await fetch(`${process.env.GOKAPI_URL || "http://localhost:8080"}/api/v1/workspaces/${wsSlug}/editor/projects/${p.id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
    } catch { /* ignore */ }
  });

  // ── Focus view ───────────────────────────────────────────────────────────

  test("record focus-view", async ({ page }) => {
    test.skip(isCI, "Recording tests are skipped in CI");

    await deleteAllEditorProjects(token, wsSlug);
    const p = await createEditorProject(token, wsSlug, "Release Notes", "en", ["fr"]);
    await uploadSeedFiles(token, wsSlug, p.id, ["release-notes.md"]);

    await setupRecording(page);

    await expect(page.getByText("Release Notes").first()).toBeVisible({ timeout: 10000 });
    await pause(page, 800);
    const rnCard = page.getByText("Release Notes").first();
    await humanClick(page, rnCard);
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 800);

    await humanClickNative(page, "open-file-release-notes.md");
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 1000);

    // Pseudo translate first so there's content
    await humanClick(page, page.getByTestId("pseudo-btn"));
    await pause(page, 2000);

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
      await fetch(`${process.env.GOKAPI_URL || "http://localhost:8080"}/api/v1/workspaces/${wsSlug}/editor/projects/${p.id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
    } catch { /* ignore */ }
  });

  // ── Pseudo-translation workflow ──────────────────────────────────────────

  test("record pseudo-translation", async ({ page }) => {
    test.skip(isCI, "Recording tests are skipped in CI");

    await deleteAllEditorProjects(token, wsSlug);
    const p = await createEditorProject(token, wsSlug, "Mobile App", "en", ["fr", "de"]);
    await uploadSeedFiles(token, wsSlug, p.id, ["app-strings.json"]);

    await setupRecording(page);

    await expect(page.getByText("Mobile App").first()).toBeVisible({ timeout: 10000 });
    await pause(page, 800);
    const maCard = page.getByText("Mobile App").first();
    await humanClick(page, maCard);
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 800);

    await humanClickNative(page, "open-file-app-strings.json");
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
    await pause(page, 1000);

    // Show the empty state first
    await pause(page, 1500);

    // Click pseudo-translate
    await humanClick(page, page.getByTestId("pseudo-btn"));
    await pause(page, 2000);

    // Show progress going to 100%
    await expect(page.getByTestId("progress-text")).toContainText("100%", { timeout: 10000 });
    await pause(page, 2000);

    try {
      await fetch(`${process.env.GOKAPI_URL || "http://localhost:8080"}/api/v1/workspaces/${wsSlug}/editor/projects/${p.id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
    } catch { /* ignore */ }
  });

  // ── TM Explorer ──────────────────────────────────────────────────────────

  test("record tm-explorer", async ({ page }) => {
    test.skip(isCI, "Recording tests are skipped in CI");

    await seedTMEntries(token, wsSlug);

    await setupRecording(page);
    await pause(page, 1000);

    // Navigate to TM
    await humanClick(page, page.getByTestId("nav-memory"));
    await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 5000 });
    await pause(page, 1500);

    // Search for an entry
    const searchInput = page.getByTestId("tm-search-input");
    if (await searchInput.isVisible()) {
      await humanType(page, searchInput, "welcome");
      await pause(page, 2000);

      // Clear search
      await searchInput.fill("");
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

  // ── Term Explorer ────────────────────────────────────────────────────────

  test("record term-explorer", async ({ page }) => {
    test.skip(isCI, "Recording tests are skipped in CI");

    await seedConcepts(token, wsSlug);

    await setupRecording(page);
    await pause(page, 1000);

    // Navigate to Termbase
    await humanClick(page, page.getByTestId("nav-termbase"));
    await expect(page.getByTestId("term-explorer")).toBeVisible({ timeout: 5000 });
    await pause(page, 1500);

    // Search for a term
    const searchInput = page.getByTestId("term-search-input");
    if (await searchInput.isVisible()) {
      await humanType(page, searchInput, "deploy");
      await pause(page, 2000);

      // Clear search
      await searchInput.fill("");
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

  // ── Context Panel ────────────────────────────────────────────────────────

  test("record context-panel", async ({ page }) => {
    test.skip(isCI, "Recording tests are skipped in CI");

    await deleteAllEditorProjects(token, wsSlug);
    const p = await createEditorProject(token, wsSlug, "Context Demo", "en", ["fr"]);
    await uploadSeedFiles(token, wsSlug, p.id, ["about-us.html"]);
    await seedTMEntries(token, wsSlug);
    await seedConcepts(token, wsSlug);

    await setupRecording(page);

    await expect(page.getByText("Context Demo").first()).toBeVisible({ timeout: 10000 });
    await pause(page, 800);
    const ctxCard = page.getByText("Context Demo").first();
    await humanClick(page, ctxCard);
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 5000 });
    await pause(page, 800);

    await humanClickNative(page, "open-file-about-us.html");
    await expect(page.getByTestId("block-grid")).toBeVisible({ timeout: 5000 });
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
      await fetch(`${process.env.GOKAPI_URL || "http://localhost:8080"}/api/v1/workspaces/${wsSlug}/editor/projects/${p.id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
    } catch { /* ignore */ }
  });

  // ── Settings ─────────────────────────────────────────────────────────────

  test("record settings", async ({ page }) => {
    test.skip(isCI, "Recording tests are skipped in CI");

    await setupRecording(page);
    await pause(page, 1000);

    await humanClick(page, page.getByTestId("nav-settings"));
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible({ timeout: 5000 });
    await pause(page, 2000);

    // Hover over workspace info fields
    await moveCursorTo(page, 400, 300, 500);
    await pause(page, 1000);
    await moveCursorTo(page, 400, 350, 500);
    await pause(page, 1000);
    await moveCursorTo(page, 400, 400, 500);
    await pause(page, 2000);
  });
});
