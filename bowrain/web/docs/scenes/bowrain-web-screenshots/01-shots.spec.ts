/**
 * Screenshot generator for the docs web-app gallery (server/web-overview.mdx).
 *
 * Produces light + dark PNGs under bowrain/web/docs/static/img/web-app/{theme}/:
 *   login, workspace-rail, dashboard, project-view, tm-explorer, term-explorer, settings
 *
 * Run against a seeded workspace (real backend, no mocks):
 *   cd bowrain/web/docs
 *   BOWRAIN_BACKEND_URL=http://localhost:8080 \
 *   BOWRAIN_SESSION_TOKEN=<jwt> BOWRAIN_WORKSPACE_SLUG=<slug> \
 *     vpx playwright test scenes/bowrain-web-screenshots/01-shots.spec.ts
 *
 * Unlike the walkthrough scenes this captures stills (not video) and pins the
 * theme, so the docs gallery has matched light/dark pairs.
 */
import { test } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { BACKEND_URL, getMyWorkspaceSlug } from "../_helpers";

// Playwright runs with cwd = bowrain/web/docs (testDir ./scenes).
const OUT = path.resolve(process.cwd(), "static/img/web-app");
const THEMES = ["light", "dark"] as const;
const VIEW = { width: 1440, height: 900 };

async function pinTheme(page: import("@playwright/test").Page, theme: string) {
  // The app persists theme in localStorage and toggles a `dark` class on <html>.
  // Set both before any app code runs, and re-assert after load.
  await page.addInitScript((t) => {
    try {
      localStorage.setItem("theme", t);
      localStorage.setItem("bowrain-theme", t);
      localStorage.setItem("vite-ui-theme", t);
    } catch {}
    const apply = () => {
      const el = document.documentElement;
      el.classList.toggle("dark", t === "dark");
      el.classList.toggle("light", t === "light");
      el.style.colorScheme = t;
    };
    apply();
    new MutationObserver(apply).observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  }, theme);
}

async function shoot(page: import("@playwright/test").Page, theme: string, name: string) {
  const dir = path.join(OUT, theme);
  fs.mkdirSync(dir, { recursive: true });
  await page.waitForTimeout(1200);
  await page.screenshot({ path: path.join(dir, `${name}.png`), animations: "disabled" });
}

for (const theme of THEMES) {
  test(`web-app screenshots (${theme})`, async ({ browser }) => {
    test.setTimeout(120_000);
    const slug = process.env.BOWRAIN_WORKSPACE_SLUG || (await getMyWorkspaceSlug());

    // login — unauthenticated, no cookie.
    {
      const ctx = await browser.newContext({ viewport: VIEW, colorScheme: theme, ignoreHTTPSErrors: true });
      const page = await ctx.newPage();
      await pinTheme(page, theme);
      await page.goto(`${BACKEND_URL}/login`, { waitUntil: "domcontentloaded" }).catch(() => {});
      await page.waitForTimeout(2000);
      await shoot(page, theme, "login");
      await ctx.close();
    }

    // authenticated surfaces — mirror record-desktop's proven web setup: the
    // session cookie is scoped to /api/, theme is carried via ?theme= and
    // emulateMedia, and we WAIT for the workspace to actually render before
    // shooting (no swallowing — a blank page should fail loudly).
    const ctx = await browser.newContext({ viewport: VIEW, colorScheme: theme, ignoreHTTPSErrors: true });
    await injectAuthCookieFor(ctx);
    const page = await ctx.newPage();
    await page.emulateMedia({ colorScheme: theme });
    await pinTheme(page, theme);

    const open = async (route: string) => {
      await page.goto(`${BACKEND_URL}/${slug}${route}?theme=${theme}`, { waitUntil: "domcontentloaded" });
      await page.waitForSelector('[data-testid="nav-translate"], [data-testid="nav-memory"], [data-testid^="project-card"], [data-testid="tm-explorer"], [data-testid="term-explorer"]', { timeout: 30_000 });
      await page.waitForTimeout(1600);
    };

    await open("");
    await shoot(page, theme, "dashboard");
    await shoot(page, theme, "workspace-rail"); // same frame; the rail is part of the chrome

    // project view
    const card = page.locator('[data-testid^="project-card"]').first();
    if (await card.count()) { await card.click().catch(() => {}); await page.waitForTimeout(2000); }
    await shoot(page, theme, "project-view");

    // TM explorer, terminology, settings — by URL (the routes the walkthrough scenes use)
    await open("/memory");
    await shoot(page, theme, "tm-explorer");
    await open("/termbase");
    await shoot(page, theme, "term-explorer");
    await open("/settings");
    await shoot(page, theme, "settings");

    await ctx.close();
  });
}

async function injectAuthCookieFor(ctx: import("@playwright/test").BrowserContext) {
  const token = process.env.BOWRAIN_SESSION_TOKEN;
  if (!token) throw new Error("BOWRAIN_SESSION_TOKEN required");
  const url = new URL(BACKEND_URL);
  await ctx.addCookies([
    { name: "bowrain_session", value: token, domain: url.hostname, path: "/api/", httpOnly: true, sameSite: "Lax", secure: url.protocol === "https:" },
  ]);
}
