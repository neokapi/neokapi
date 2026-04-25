import { test, expect, type Page } from "@playwright/test";
import { TEST_IDS } from "@neokapi/ui/test-ids";
import {
  authenticate,
  getOrCreateWorkspace,
  createEditorProject,
  getEditorProject,
  findItemId,
  uploadSeedFiles,
  deleteAllEditorProjects,
  waitForServer,
} from "./helpers/api-client";

const BASE_URL = process.env.BOWRAIN_URL || "http://localhost:8080";

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
      sameSite: "Lax" as const,
    },
  ]);
}

let token: string;
let wsSlug: string;
let projectId: string;

test.describe("Routing", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(token, process.env.BOWRAIN_WORKSPACE_NAME || "Acme Inc.", process.env.BOWRAIN_WORKSPACE || "acme");
    wsSlug = ws.slug;
    await deleteAllEditorProjects(token, wsSlug);

    const proj = await createEditorProject(token, wsSlug, "Routing Test Project", "en", ["fr"]);
    await uploadSeedFiles(token, wsSlug, proj.id, ["about-us.html"]);
    projectId = proj.id;
  });

  test("deep link to workspace dashboard loads correctly", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);

    // Dashboard should show the project
    await expect(page.getByText("Routing Test Project").first()).toBeVisible({ timeout: 10000 });

    // URL should be the workspace slug
    expect(page.url()).toContain(`/${wsSlug}`);
  });

  test("deep link to project detail loads correctly", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/p/${projectId}/s/main`);

    // Project view should load
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 10000 });

    // URL should contain the project ID and stream
    expect(page.url()).toContain(`/p/${projectId}/s/main`);
  });

  test("deep link to editor loads correctly", async ({ page }) => {
    await injectAuthCookie(page, token);
    // Resolve item ID from the project
    const proj = await getEditorProject(token, wsSlug, projectId);
    const itemId = findItemId(proj, "about-us.html");
    await page.goto(`/${wsSlug}/p/${projectId}/s/main/${itemId}/translate`);

    // Editor should load (layout switcher is always visible regardless of mode)
    await expect(page.getByTestId("layout-switcher")).toBeVisible({ timeout: 30000 });

    // URL should contain the item ID
    expect(page.url()).toContain(`/s/main/${itemId}/translate`);
  });

  test("deep link to TM explorer loads correctly", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/memory`);

    // TM explorer should be visible
    await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/memory");
  });

  test("deep link to termbase loads correctly", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/termbase`);

    // Term explorer should be visible
    await expect(page.getByTestId("term-explorer")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/termbase");
  });

  test("deep link to settings loads correctly", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/settings`);

    await expect(page.getByTestId(TEST_IDS.settings.heading)).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/settings");
  });

  test("URL persists on page refresh", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/p/${projectId}/s/main`);
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 10000 });

    // Refresh the page
    await page.reload();

    // Project view should still be loaded after refresh
    await expect(page.getByTestId("file-drop-zone")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain(`/p/${projectId}/s/main`);
  });

  test("sidebar navigation updates URL", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);
    await expect(page.getByTestId("nav-translate")).toBeVisible({ timeout: 10000 });

    // Navigate to memory via sidebar
    await page.getByTestId("nav-memory").click();
    await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/memory");

    // Navigate to termbase via sidebar
    await page.getByTestId("nav-termbase").click();
    await expect(page.getByTestId("term-explorer")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/termbase");

    // Navigate to settings via sidebar
    await page.getByTestId("nav-settings").click();
    await expect(page.getByTestId(TEST_IDS.settings.heading)).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/settings");

    // Navigate back to dashboard via sidebar
    await page.getByTestId("nav-translate").click();
    await expect(page.getByText("Routing Test Project").first()).toBeVisible({ timeout: 10000 });
    // Dashboard route should be just the workspace slug
    expect(page.url()).toMatch(new RegExp(`/${wsSlug}/?$`));
  });

  test("browser back/forward navigation works", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);
    await expect(page.getByTestId("nav-translate")).toBeVisible({ timeout: 10000 });

    // Navigate to memory
    await page.getByTestId("nav-memory").click();
    await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 10000 });

    // Navigate to termbase
    await page.getByTestId("nav-termbase").click();
    await expect(page.getByTestId("term-explorer")).toBeVisible({ timeout: 10000 });

    // Go back — should be on memory
    await page.goBack();
    await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/memory");

    // Go back again — should be on dashboard
    await page.goBack();
    await expect(page.getByText("Routing Test Project").first()).toBeVisible({ timeout: 10000 });

    // Go forward — should be on memory
    await page.goForward();
    await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/memory");
  });

  test("sidebar collapsed state persists across navigation", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);
    await expect(page.getByTestId("nav-translate")).toBeVisible({ timeout: 10000 });

    // Collapse sidebar (click the collapse toggle)
    const collapseBtn = page.getByTestId("sidebar-collapse-btn");
    if (await collapseBtn.isVisible()) {
      await collapseBtn.click();
      await page.waitForTimeout(300);

      // Navigate away and back
      await page.getByTestId("nav-memory").click();
      await expect(page.getByTestId("tm-explorer")).toBeVisible({ timeout: 10000 });

      // Sidebar should still be collapsed (check localStorage)
      const stored = await page.evaluate(() => localStorage.getItem("bowrain-ui"));
      if (stored) {
        const parsed = JSON.parse(stored);
        expect(parsed.state.sidebarCollapsed).toBe(true);
      }
    }
  });
});
