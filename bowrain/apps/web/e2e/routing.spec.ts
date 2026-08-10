import { test, expect, type Page } from "@playwright/test";
import { TEST_IDS } from "@neokapi/ui/test-ids";
import {
  authenticate,
  getOrCreateWorkspace,
  createEditorProject,
  getEditorProject,
  findItemName,
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

/**
 * Open a Context surface through the sidebar.
 *
 * Concepts, Voice, Content memory and Activity moved under the Context section
 * when the context hub landed, so none of them has a top-level `nav-*` entry
 * any more: the icon rail selects the section and the secondary panel selects
 * the surface within it.
 */
async function openContextSurface(page: Page, surface: string) {
  await page.getByTestId("nav-context").click();
  const item = page.getByTestId(`subnav-${surface}`);
  await expect(item).toBeVisible({ timeout: 10000 });
  await item.click();
}

let token: string;
let wsSlug: string;
let projectId: string;

test.describe("Routing", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(
      token,
      process.env.BOWRAIN_WORKSPACE_NAME || "Acme Inc.",
      process.env.BOWRAIN_WORKSPACE || "acme",
    );
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
    // The route addresses the item by name, carried as the surface's trailing
    // splat — real path segments, so a nested name needs no encoding.
    const proj = await getEditorProject(token, wsSlug, projectId);
    const itemName = findItemName(proj, "about-us.html");
    await page.goto(`/${wsSlug}/p/${projectId}/s/main/translate/${itemName}`);

    // Editor should load (Visual/Table view switcher is always visible)
    await expect(page.getByTestId("view-switcher")).toBeVisible({ timeout: 30000 });

    // URL should contain the item name
    expect(page.url()).toContain(`/s/main/translate/${itemName}`);
  });

  test("deep link to content memory explorer loads correctly", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/memory`);

    // content memory browser should be visible
    await expect(page.getByTestId("tm-browser")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/memory");
  });

  test("legacy terms deep link redirects to Brand · Concepts", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/terms`);

    // Terminology lives inside the brand graph now: /terms is a legacy
    // redirect to the Concepts section of the Brand hub.
    await expect(page).toHaveURL(new RegExp(`/${wsSlug}/context/concepts`), { timeout: 10000 });
    await expect(page.getByRole("heading", { name: "Concepts" })).toBeVisible({ timeout: 10000 });
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

    // Content memory is a Context surface, so reaching it is two clicks: the
    // icon rail selects the section, the secondary panel selects the surface.
    await openContextSurface(page, "memory");
    await expect(page.getByTestId("tm-browser")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/context/memory");

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

    // Navigate to content memory
    await openContextSurface(page, "memory");
    await expect(page.getByTestId("tm-browser")).toBeVisible({ timeout: 10000 });

    // Navigate to settings
    await page.getByTestId("nav-settings").click();
    await expect(page.getByTestId(TEST_IDS.settings.heading)).toBeVisible({ timeout: 10000 });

    // Go back — should be on content memory
    await page.goBack();
    await expect(page.getByTestId("tm-browser")).toBeVisible({ timeout: 10000 });
    expect(page.url()).toContain("/context/memory");

    // Go back again — should be on the Context section landing, then the
    // dashboard: the icon rail and the secondary panel are two history entries.
    await page.goBack();
    await page.goBack();
    await expect(page.getByText("Routing Test Project").first()).toBeVisible({ timeout: 10000 });

    // Go forward — should be on the Context section again
    await page.goForward();
    await expect(page).toHaveURL(new RegExp(`/${wsSlug}/context`), { timeout: 10000 });
  });

  test("the secondary panel lists the Context surfaces", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);
    await expect(page.getByTestId("nav-translate")).toBeVisible({ timeout: 10000 });

    // The desktop sidebar is a fixed icon rail — the collapse affordance it
    // used to carry is gone, and so is the assertion that used to hide behind
    // `if (await collapseBtn.isVisible())` and therefore never ran. What is
    // worth pinning is that selecting a section reveals its surfaces, since
    // that two-step is now the only route to most of them.
    await page.getByTestId("nav-context").click();
    for (const id of ["concepts", "voice", "memory", "activity"]) {
      await expect(page.getByTestId(`subnav-${id}`)).toBeVisible({ timeout: 10000 });
    }

    // The rail survives the navigation into a surface.
    await page.getByTestId("subnav-concepts").click();
    await expect(page).toHaveURL(new RegExp(`/${wsSlug}/context/concepts`), { timeout: 10000 });
    await expect(page.getByTestId("nav-context")).toBeVisible();
  });
});
