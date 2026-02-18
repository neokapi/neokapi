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
let wsSlug: string;
let projectId: string;

test.describe("Open in Desktop", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test.beforeAll(async () => {
    await waitForServer();
    token = await authenticate();
    const ws = await getOrCreateWorkspace(token, "Acme Inc.", "acme");
    wsSlug = ws.slug;
    await deleteAllEditorProjects(token, wsSlug);

    // Create a project to open.
    const proj = await createEditorProject(token, wsSlug, "Desktop Test Project", "en", ["fr"]);
    await uploadSeedFiles(token, wsSlug, proj.id, ["about-us.html"]);
    projectId = proj.id;
  });

  test("shows Open in Desktop banner on project view", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto("/");

    // Wait for dashboard and open the project.
    await expect(page.getByText("Desktop Test Project").first()).toBeVisible({ timeout: 15000 });
    await page.getByText("Desktop Test Project").first().click();

    // The "Open in Bowrain Desktop" banner should appear in server mode.
    await expect(page.getByText("Open in Bowrain Desktop")).toBeVisible({ timeout: 10000 });
    await expect(page.getByTestId("open-in-desktop-btn")).toBeVisible();
  });

  test("generates correct deep link URL", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto("/");

    await expect(page.getByText("Desktop Test Project").first()).toBeVisible({ timeout: 15000 });
    await page.getByText("Desktop Test Project").first().click();

    await expect(page.getByTestId("open-in-desktop-btn")).toBeVisible({ timeout: 10000 });

    // Intercept the deep link navigation (prevent actual protocol handler).
    const deepLinkPromise = page.evaluate(() => {
      return new Promise<string>((resolve) => {
        const origAssign = Object.getOwnPropertyDescriptor(window.location, "href");
        let captured = "";
        Object.defineProperty(window.location, "href", {
          set(val: string) {
            if (val.startsWith("bowrain://")) {
              captured = val;
              resolve(val);
            } else if (origAssign?.set) {
              origAssign.set.call(window.location, val);
            }
          },
          get() {
            return origAssign?.get?.call(window.location) ?? "";
          },
          configurable: true,
        });
        // Timeout fallback.
        setTimeout(() => resolve(captured), 3000);
      });
    });

    await page.getByTestId("open-in-desktop-btn").click();

    const deepLink = await deepLinkPromise;
    // The deep link should follow the bowrain:// protocol pattern.
    expect(deepLink).toMatch(/^bowrain:\/\/project\/.+\?server=.+&workspace=.+$/);
    expect(deepLink).toContain(`server=${encodeURIComponent(BASE_URL)}`);
    expect(deepLink).toContain(`workspace=${wsSlug}`);
    expect(deepLink).toContain(projectId);
  });

  test("dismiss banner persists across navigation", async ({ page }) => {
    await injectAuthCookie(page, token);
    await page.goto("/");

    await expect(page.getByText("Desktop Test Project").first()).toBeVisible({ timeout: 15000 });
    await page.getByText("Desktop Test Project").first().click();

    // Banner should be visible.
    await expect(page.getByText("Open in Bowrain Desktop")).toBeVisible({ timeout: 10000 });

    // Dismiss the banner.
    await page.getByTestId("dismiss-open-in-desktop").click();

    // Banner should disappear.
    await expect(page.getByText("Open in Bowrain Desktop")).not.toBeVisible();

    // Verify localStorage was set.
    const dismissed = await page.evaluate(() =>
      localStorage.getItem("bowrain-open-in-desktop-dismissed"),
    );
    expect(dismissed).toBe("true");

    // Navigate away and back — banner should stay dismissed.
    await page.getByTestId("back-to-projects").click();
    await expect(page.getByText("Desktop Test Project").first()).toBeVisible({ timeout: 10000 });
    await page.getByText("Desktop Test Project").first().click();

    // Banner should still be hidden.
    await expect(page.getByText("Open in Bowrain Desktop")).not.toBeVisible();
  });

  test("shows download fallback when app not installed", async ({ page }) => {
    await injectAuthCookie(page, token);

    // Clear any dismissed state.
    await page.goto("/");
    await page.evaluate(() => localStorage.removeItem("bowrain-open-in-desktop-dismissed"));
    await page.reload();

    await expect(page.getByText("Desktop Test Project").first()).toBeVisible({ timeout: 15000 });
    await page.getByText("Desktop Test Project").first().click();

    await expect(page.getByTestId("open-in-desktop-btn")).toBeVisible({ timeout: 10000 });

    // Prevent navigation on bowrain:// protocol.
    await page.evaluate(() => {
      Object.defineProperty(window.location, "href", {
        set() { /* swallow bowrain:// deep link */ },
        get() { return document.location.href; },
        configurable: true,
      });
    });

    await page.getByTestId("open-in-desktop-btn").click();

    // After ~1.5s without window blur, the fallback should appear.
    await expect(page.getByText("Bowrain Desktop not found.")).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("link", { name: /Download/ })).toBeVisible();
    await expect(page.getByRole("link", { name: /Download/ })).toHaveAttribute(
      "href",
      "https://bowrain.dev/download",
    );
  });
});
