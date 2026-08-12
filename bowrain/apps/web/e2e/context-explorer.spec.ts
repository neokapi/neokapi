import { test, expect, type Page } from "@playwright/test";
import {
  authenticate,
  getOrCreateWorkspace,
  createEditorProject,
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

/**
 * The context explorer against the real stack.
 *
 * Everything here is served by the running server over a real workspace with a
 * real project: the profiles the push declared, the vocabulary the workspace
 * holds, the dashboard's per-locale rollup. Nothing is stubbed — the point of
 * this spec is that the same component set kapi renders against one project
 * answers here with the dimensions free.
 */
test.describe("Context explorer", () => {
  test.use({ viewport: { width: 1440, height: 900 } });

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

    const proj = await createEditorProject(token, wsSlug, "Explorer Test Project", "en", ["fr"]);
    await uploadSeedFiles(token, wsSlug, proj.id, ["about-us.html"]);
    projectId = proj.id;
  });

  test("the Context section lists Explorer and opens it", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}`);

    await page.getByTestId("nav-context").click();
    const entry = page.getByTestId("subnav-explorer");
    await expect(entry).toBeVisible({ timeout: 10000 });
    await entry.click();

    await expect(page).toHaveURL(new RegExp(`/${wsSlug}/context/explorer`), { timeout: 10000 });
    await expect(page.getByTestId("context-explorer")).toBeVisible({ timeout: 10000 });
  });

  test("the three panes stand over one point", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/context/explorer`);

    await expect(page.getByTestId("governs-pane")).toBeVisible({ timeout: 15000 });
    await expect(page.getByTestId("lives-pane")).toBeVisible();
    await expect(page.getByTestId("relates-pane")).toBeVisible();

    // The workspace is the mount's pinned rung, and it addresses the point.
    await expect(page.getByTestId("context-ladder")).toContainText(wsSlug);
    await expect(page.getByTestId("context-address")).toHaveText(`context://`);
  });

  test("the relations pane answers at workspace reach", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/context/explorer`);

    const relates = page.getByTestId("relates-pane");
    await expect(relates).toBeVisible({ timeout: 15000 });
    await expect(relates).toHaveAttribute("data-reach", "workspace");
    await expect(relates).toContainText("across the workspace");
  });

  test("choosing a project narrows the tuple and the address", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/context/explorer`);

    const bar = page.getByTestId("scope-filter-bar");
    await expect(bar).toBeVisible({ timeout: 15000 });

    const projectPicker = bar.getByLabel("Project");
    await expect(projectPicker).toBeVisible({ timeout: 10000 });
    await projectPicker.click();
    await page.getByRole("option", { name: "Explorer Test Project" }).click();

    // The tuple is the URL, so the point can be linked to.
    await expect(page).toHaveURL(new RegExp(`project=${projectId}`), { timeout: 10000 });
    await expect(page.getByTestId("context-address")).toContainText(projectId);
    await expect(page.getByTestId("context-ladder")).toContainText(projectId);
  });

  test("a point survives a reload", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/context/explorer?project=${projectId}`);

    await expect(page.getByTestId("context-explorer")).toBeVisible({ timeout: 15000 });
    await page.reload();
    await expect(page.getByTestId("context-address")).toContainText(projectId, { timeout: 15000 });
  });

  test("search groups its answer by kind rather than blending it", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/context/explorer`);

    const panel = page.getByTestId("search-panel");
    await expect(panel).toBeVisible({ timeout: 15000 });
    await panel.getByTestId("search-input").fill("the");
    await panel.getByRole("button", { name: "Search" }).click();

    await expect(page.getByTestId("search-results")).toBeVisible({ timeout: 15000 });
    // Zero groups is a legitimate answer on an empty workspace; what must never
    // happen is one blended list, so every group that renders carries its kind.
    for (const group of await page.getByTestId("search-group").all()) {
      await expect(group).toHaveAttribute("data-kind", /terms|precedent|content|profiles/);
    }
  });
});
