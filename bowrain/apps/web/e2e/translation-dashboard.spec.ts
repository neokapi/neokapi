import { test, expect, type Page } from "@playwright/test";
import {
  authenticate,
  getOrCreateWorkspace,
  createEditorProject,
  uploadSeedFiles,
  pseudoTranslateFile,
  deleteAllEditorProjects,
  waitForServer,
} from "./helpers/api-client";

const BASE_URL = process.env.BOWRAIN_URL || "http://localhost:8080";

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

test.describe("Translation Dashboard", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(token, "Acme Inc.", "acme");
    wsSlug = ws.slug;
    await deleteAllEditorProjects(token, wsSlug);

    // Create a project with target locales and seed files + translations
    const proj = await createEditorProject(token, wsSlug, "Dashboard Test Project", "en", [
      "fr",
      "de",
    ]);
    projectId = proj.id;
    await uploadSeedFiles(token, wsSlug, projectId, ["about-us.html", "app-strings.json"]);
    // Pseudo-translate one file so the dashboard shows partial progress
    await pseudoTranslateFile(token, wsSlug, projectId, "about-us.html", "fr");
  });

  test("deep link to translation dashboard loads correctly", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/p/${projectId}/s/main/dashboard`);

    await expect(page.getByTestId("translation-dashboard")).toBeVisible({ timeout: 15000 });
    expect(page.url()).toContain(`/p/${projectId}/s/main/dashboard`);
  });

  test("displays project name in dashboard header", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/p/${projectId}/s/main/dashboard`);

    await expect(page.getByTestId("translation-dashboard")).toBeVisible({ timeout: 15000 });
    await expect(page.getByText(/Dashboard Test Project/)).toBeVisible();
  });

  test("shows summary stat cards", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/p/${projectId}/s/main/dashboard`);

    await expect(page.getByTestId("translation-dashboard")).toBeVisible({ timeout: 15000 });
    await expect(page.getByText("Source Words")).toBeVisible();
    await expect(page.getByText("Target Languages")).toBeVisible();
    await expect(page.getByText("Translatable Blocks")).toBeVisible();
    await expect(page.getByText("Overall Completion")).toBeVisible();
  });

  test("shows completion percentage", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/p/${projectId}/s/main/dashboard`);

    await expect(page.getByTestId("translation-dashboard")).toBeVisible({ timeout: 15000 });
    // The header shows "XX% complete"
    await expect(page.getByText(/\d+% complete/)).toBeVisible();
  });

  test("shows language completion charts when data exists", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/p/${projectId}/s/main/dashboard`);

    await expect(page.getByTestId("translation-dashboard")).toBeVisible({ timeout: 15000 });
    await expect(page.getByText("Completion by Language")).toBeVisible();
    await expect(page.getByText("Word Count by Language")).toBeVisible();
  });

  test("shows file progress table", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/p/${projectId}/s/main/dashboard`);

    await expect(page.getByTestId("translation-dashboard")).toBeVisible({ timeout: 15000 });
    await expect(page.getByText("File Progress")).toBeVisible();
    await expect(page.getByText("about-us.html")).toBeVisible();
    await expect(page.getByText("app-strings.json")).toBeVisible();
  });

  test("URL persists on page refresh", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto(`/${wsSlug}/p/${projectId}/s/main/dashboard`);

    await expect(page.getByTestId("translation-dashboard")).toBeVisible({ timeout: 15000 });

    await page.reload();

    await expect(page.getByTestId("translation-dashboard")).toBeVisible({ timeout: 15000 });
    expect(page.url()).toContain(`/p/${projectId}/s/main/dashboard`);
  });
});
