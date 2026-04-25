import { test, expect, type Page } from "@playwright/test";
import { TEST_IDS } from "@neokapi/ui/test-ids";
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

/**
 * Intercept bowrain:// deep link navigation by patching Location.prototype.href.
 * Must be called BEFORE triggering the navigation (before clicking the Open button).
 * Returns a function that retrieves any captured deep link URLs.
 */
async function interceptDeepLinks(page: Page) {
  await page.evaluate(() => {
    (window as Record<string, unknown>).__capturedDeepLinks = [] as string[];
    const proto = Object.getPrototypeOf(window.location);
    const desc = Object.getOwnPropertyDescriptor(proto, "href");
    if (desc?.set) {
      const origSet = desc.set.bind(proto);
      const origGet = desc.get?.bind(proto);
      Object.defineProperty(proto, "href", {
        set(val: string) {
          if (typeof val === "string" && val.startsWith("bowrain://")) {
            ((window as Record<string, unknown>).__capturedDeepLinks as string[]).push(val);
            return; // Swallow — don't navigate
          }
          origSet.call(this, val);
        },
        get: origGet
          ? function (this: Location) {
              return origGet.call(this);
            }
          : desc.get?.bind(proto),
        configurable: true,
      });
    }
  });

  return async () => {
    return page.evaluate(() => (window as Record<string, unknown>).__capturedDeepLinks as string[]);
  };
}

let token: string;
let wsSlug: string;
let projectId: string;

test.describe("Open in Desktop", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(token, process.env.BOWRAIN_WORKSPACE_NAME || "Acme Inc.", process.env.BOWRAIN_WORKSPACE || "acme");
    wsSlug = ws.slug;
    await deleteAllEditorProjects(token, wsSlug);

    // Create a project to open.
    const proj = await createEditorProject(token, wsSlug, "Desktop Test Project", "en", ["fr"]);
    await uploadSeedFiles(token, wsSlug, proj.id, ["about-us.html"]);
    projectId = proj.id;
  });

  test("shows Open in Desktop banner on project view", async ({ page }) => {
    await injectAuthCookie(page, token);
    // Navigate directly to the project detail route
    await page.goto(`/${wsSlug}/p/${projectId}/s/main`);

    // The "Open in Bowrain Desktop" banner should appear in server mode.
    await expect(page.getByTestId(TEST_IDS.editor.openInDesktopButton)).toBeVisible({ timeout: 10000 });
    await expect(page.getByTestId("open-in-desktop-btn")).toBeVisible();
  });

  test("generates correct deep link URL", async ({ page }) => {
    await injectAuthCookie(page, token);
    // Navigate directly to the project detail route
    await page.goto(`/${wsSlug}/p/${projectId}/s/main`);

    const btn = page.getByTestId("open-in-desktop-btn");
    await expect(btn).toBeVisible({ timeout: 10000 });

    // Read the deep link from the data-href attribute (no clicking needed).
    const deepLink = await btn.getAttribute("data-href");
    expect(deepLink).toBeTruthy();
    expect(deepLink).toMatch(/^bowrain:\/\/project\/.+\?server=.+&workspace=.+$/);
    expect(deepLink).toContain(`server=${encodeURIComponent(BASE_URL)}`);
    expect(deepLink).toContain(`workspace=${wsSlug}`);
    expect(deepLink).toContain(projectId);
  });

  test("dismiss banner persists across navigation", async ({ page }) => {
    await injectAuthCookie(page, token);
    // Navigate directly to the project detail route
    await page.goto(`/${wsSlug}/p/${projectId}/s/main`);

    // Banner should be visible.
    await expect(page.getByTestId(TEST_IDS.editor.openInDesktopButton)).toBeVisible({ timeout: 10000 });

    // Dismiss the banner.
    await page.getByTestId("dismiss-open-in-desktop").click();

    // Banner should disappear.
    await expect(page.getByTestId(TEST_IDS.editor.openInDesktopButton)).not.toBeVisible();

    // Verify localStorage was set.
    const dismissed = await page.evaluate(() =>
      localStorage.getItem("bowrain-open-in-desktop-dismissed"),
    );
    expect(dismissed).toBe("true");

    // Navigate to dashboard and back — banner should stay dismissed.
    await page.goto(`/${wsSlug}`);
    await expect(page.getByText("Desktop Test Project").first()).toBeVisible({ timeout: 10000 });
    await page.getByText("Desktop Test Project").first().click();

    // Banner should still be hidden.
    await expect(page.getByTestId(TEST_IDS.editor.openInDesktopButton)).not.toBeVisible();
  });

  test("shows download fallback when app not installed", async ({ page }) => {
    await injectAuthCookie(page, token);

    // Clear any dismissed state and navigate to project.
    await page.goto(`/${wsSlug}/p/${projectId}/s/main`);
    await page.evaluate(() => localStorage.removeItem("bowrain-open-in-desktop-dismissed"));
    await page.reload();

    await expect(page.getByTestId("open-in-desktop-btn")).toBeVisible({ timeout: 10000 });

    // Intercept bowrain:// navigation before clicking.
    await interceptDeepLinks(page);

    await page.getByTestId("open-in-desktop-btn").click();

    // After ~1.5s without window blur, the fallback should appear.
    await expect(page.getByTestId(TEST_IDS.editor.desktopNotFoundMessage)).toBeVisible({ timeout: 5000 });
    await expect(page.getByTestId(TEST_IDS.editor.desktopDownloadLink)).toBeVisible();
    await expect(page.getByTestId(TEST_IDS.editor.desktopDownloadLink)).toHaveAttribute(
      "href",
      "https://bowrain.dev/download",
    );
  });
});
