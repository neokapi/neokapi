#!/usr/bin/env -S vpx tsx
/**
 * Docs screenshots, light and dark, from the real screens.
 *
 *   vpx tsx scripts/shoot-docs-screens.ts bowrain <out-root>
 *   vpx tsx scripts/shoot-docs-screens.ts desktop <out-root> [--storybook <url>]
 *
 * `bowrain` drives the Bowrain web app on the local stack (`make -C bowrain
 * stack-up-web`, then `make -C harness seed`) as the seeded owner and writes
 * `<out-root>/{light,dark}/<name>.png`. The docs set lives in
 * `bowrain/web/docs/static/img/web-app`.
 *
 * `desktop` renders Kapi Desktop screens from the Kapi Storybook. Without
 * `--storybook` it serves `storybook/storybook-static` (`make
 * kapi-storybook-build`) on a free local port. The docs set lives in
 * `web/static/img/desktop`.
 *
 * Every shot is 1360x900 CSS pixels at device scale factor 2, cropped to the
 * screen's content at full width, taken once data has loaded, with animations
 * and the caret disabled. The CDN publish targets (`make publish-cdn-images`,
 * `make publish-cdn-bowrain-images`) upload the results; this script only
 * writes files.
 */
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import type { AddressInfo } from "node:net";
import { chromium, type Browser, type BrowserContext, type Page } from "playwright";
import { loadEnv } from "../src/lib/env.ts";
import { REPO_ROOT } from "../src/lib/paths.ts";

type Theme = "light" | "dark";
const THEMES: Theme[] = ["light", "dark"];
const WIDTH = 1360;
const HEIGHT = 900;
const SCALE = 2;

// ── Shared: waiting and cropping ─────────────────────────────────────────────

/** The marks of a screen still loading. A shot waits until none is visible. */
const LOADING_SELECTORS = [
  '[aria-busy="true"]',
  '[data-slot="skeleton"]',
  ".animate-pulse",
  ".animate-spin",
  '[role="progressbar"]',
];
const LOADING_TEXT = /^(Checking|Loading)\b/;

/**
 * In-page probe: the loading marks still visible, as short descriptions.
 * Written as source text because tsx wraps named inner functions in a helper
 * that does not exist in the page.
 */
const LOADING_PROBE = `(() => {
  const selectors = ${JSON.stringify(LOADING_SELECTORS)};
  const re = new RegExp(${JSON.stringify(LOADING_TEXT.source)});
  const out = [];
  const visible = (el) => {
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) return false;
    const s = getComputedStyle(el);
    return s.visibility !== "hidden" && s.display !== "none" && Number(s.opacity) > 0;
  };
  for (const sel of selectors) {
    for (const el of document.querySelectorAll(sel)) {
      if (visible(el)) out.push(sel + ' "' + (el.textContent || "").trim().slice(0, 40) + '"');
    }
  }
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const t = (n.textContent || "").trim();
    if (t && re.test(t) && n.parentElement && visible(n.parentElement)) out.push('"' + t.slice(0, 40) + '"');
  }
  return out;
})()`;

async function waitSettled(page: Page, what: string, timeout = 20_000): Promise<void> {
  try {
    await page.waitForFunction(`${LOADING_PROBE}.length === 0`, undefined, { timeout, polling: 250 });
  } catch {
    const still = (await page.evaluate(LOADING_PROBE)) as string[];
    throw new Error(
      `${what}: the screen still shows a loading state after ${timeout / 1000} s (${still.slice(0, 5).join("; ")})`,
    );
  }
  await page.evaluate("document.fonts.ready");
  // One more frame for layout that follows the last response.
  await page.waitForTimeout(600);
}

/** Shoot the full width of the viewport from the top of `selector` to its
 *  bottom, clipped to the viewport. */
async function shoot(page: Page, file: string, selector?: string): Promise<{ w: number; h: number }> {
  let top = 0;
  let bottom = HEIGHT;
  if (selector) {
    const box = await page.locator(selector).first().boundingBox();
    if (!box) throw new Error(`${path.basename(file)}: nothing matches ${selector}`);
    top = Math.max(0, Math.floor(box.y));
    bottom = Math.min(HEIGHT, Math.ceil(box.y + box.height));
    if (bottom - top < 100) {
      top = 0;
      bottom = HEIGHT;
    }
  }
  // No pointer, no hover and no focus ring in a docs image.
  await page.mouse.move(0, 0);
  await page.evaluate("document.activeElement instanceof HTMLElement && document.activeElement.blur()");
  fs.mkdirSync(path.dirname(file), { recursive: true });
  await page.screenshot({
    path: file,
    clip: { x: 0, y: top, width: WIDTH, height: bottom - top },
    animations: "disabled",
    caret: "hide",
  });
  const size = { w: WIDTH * SCALE, h: (bottom - top) * SCALE };
  console.log(`  ✓ ${path.relative(REPO_ROOT, file)} (${size.w}x${size.h})`);
  return size;
}

async function newContext(browser: Browser, theme: Theme): Promise<BrowserContext> {
  return browser.newContext({
    viewport: { width: WIDTH, height: HEIGHT },
    deviceScaleFactor: SCALE,
    colorScheme: theme,
    reducedMotion: "reduce",
    locale: "en-US",
    timezoneId: "Europe/Oslo",
  });
}

// ── Kapi Desktop, from the Kapi Storybook ────────────────────────────────────

/** Each docs name, the story that shows that screen, and the story's args. */
interface DesktopShot {
  name: string;
  story: string;
  /** Wait for this before shooting; the story's own root is the default. */
  ready?: string;
}

const DESKTOP_SHOTS: DesktopShot[] = [
  { name: "open", story: "components-openscreen--recent-folders" },
  { name: "overview", story: "project-screens-overview--suggestions-pending" },
  { name: "decide", story: "components-contexthub--whats-new" },
  { name: "reader", story: "project-screens-reader--findings-with-fixes" },
  { name: "rules", story: "project-screens-rules--kept-by" },
  { name: "assistants", story: "components-contexthub--assistants" },
  { name: "share", story: "components-contexthub--share" },
  { name: "languages", story: "pages-languagespage--source-only" },
  { name: "review", story: "project-reviewpage--queue" },
  { name: "context-home", story: "components-apphome--several-projects" },
];

/** Serve a built Storybook directory on a free port. */
async function serveStatic(root: string): Promise<{ url: string; close: () => Promise<void> }> {
  const types: Record<string, string> = {
    ".html": "text/html",
    ".js": "text/javascript",
    ".mjs": "text/javascript",
    ".css": "text/css",
    ".json": "application/json",
    ".svg": "image/svg+xml",
    ".png": "image/png",
    ".woff2": "font/woff2",
    ".woff": "font/woff",
    ".wasm": "application/wasm",
  };
  const server = http.createServer((req, res) => {
    const rel = decodeURIComponent((req.url ?? "/").split("?")[0]);
    let file = path.join(root, rel);
    if (!file.startsWith(root)) {
      res.writeHead(403).end();
      return;
    }
    if (fs.existsSync(file) && fs.statSync(file).isDirectory()) file = path.join(file, "index.html");
    if (!fs.existsSync(file)) {
      res.writeHead(404).end();
      return;
    }
    res.writeHead(200, { "Content-Type": types[path.extname(file)] ?? "application/octet-stream" });
    fs.createReadStream(file).pipe(res);
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  return {
    url: `http://127.0.0.1:${port}`,
    close: () => new Promise((resolve) => server.close(() => resolve())),
  };
}

async function shootDesktop(outRoot: string, storybookUrl?: string): Promise<void> {
  let base = storybookUrl?.replace(/\/+$/, "");
  let close = async () => {};
  if (!base) {
    const dir = path.join(REPO_ROOT, "storybook", "storybook-static");
    if (!fs.existsSync(path.join(dir, "iframe.html"))) {
      throw new Error(`no Storybook build at ${path.relative(REPO_ROOT, dir)}: run make kapi-storybook-build`);
    }
    const served = await serveStatic(dir);
    base = served.url;
    close = served.close;
  }
  const index = (await (await fetch(`${base}/index.json`)).json()) as { entries: Record<string, unknown> };
  const missing = DESKTOP_SHOTS.filter((s) => !index.entries[s.story]).map((s) => s.story);
  if (missing.length) throw new Error(`the Storybook has no story ${missing.join(", ")}`);

  const browser = await chromium.launch();
  try {
    for (const theme of THEMES) {
      const context = await newContext(browser, theme);
      const page = await context.newPage();
      for (const shot of DESKTOP_SHOTS) {
        // The preview's theme decorator reads the `theme` global; the locale
        // global pins the English catalog.
        const url = `${base}/iframe.html?id=${shot.story}&viewMode=story&globals=theme:${theme};locale:en`;
        await page.goto(url, { waitUntil: "load" });
        await page.waitForSelector(shot.ready ?? "#storybook-root > *", { timeout: 30_000 });
        await waitSettled(page, shot.name);
        await shoot(page, path.join(outRoot, theme, `${shot.name}.png`), "#storybook-root > *");
      }
      await context.close();
    }
  } finally {
    await browser.close();
    await close();
  }
}

// ── Bowrain web app, on the local stack ──────────────────────────────────────

const BASE = () => process.env.BOWRAIN_BACKEND_URL || "http://localhost:8080";

/** Mint a fresh session for the seeded owner through the server's device
 *  flow, as the seeder does, so a run never meets an expired token. */
async function deviceAuth(email: string, name: string): Promise<string> {
  const api = `${BASE()}/api/v1`;
  const form = (url: string, body: string) =>
    fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
      redirect: "manual",
    });
  const start = (await (await form(`${api}/auth/device/start`, "client_id=e2e-shared")).json()) as {
    user_code: string;
    device_code: string;
  };
  await form(
    `${api}/auth/device/verify`,
    `user_code=${start.user_code}&email=${encodeURIComponent(email)}&name=${encodeURIComponent(name)}`,
  );
  const poll = (await (
    await form(
      `${api}/auth/device/poll`,
      `device_code=${start.device_code}&grant_type=urn:ietf:params:oauth:grant-type:device_code`,
    )
  ).json()) as { access_token?: string };
  if (!poll.access_token) throw new Error(`device auth (${email}): no access_token; is the stack up?`);
  return poll.access_token;
}

interface WebCtx {
  page: Page;
  theme: Theme;
  /** `${BASE}/${workspace}` */
  ws: string;
  pid: string;
}

/** Open a workspace-relative path with the theme pinned. */
async function go(c: WebCtx, rel: string, ready: string): Promise<void> {
  const sep = rel.includes("?") ? "&" : "?";
  await c.page.goto(`${c.ws}${rel}${sep}theme=${c.theme}`, { waitUntil: "domcontentloaded" });
  await c.page.waitForSelector(ready, { timeout: 30_000 });
  await dismissDesktopPrompt(c.page);
}

async function dismissDesktopPrompt(page: Page): Promise<void> {
  const dismiss = page.getByTestId("dismiss-open-in-desktop");
  if (await dismiss.count()) await dismiss.first().click().catch(() => {});
}

interface WebShot {
  name: string;
  /** Reach the screen and leave it ready to shoot. */
  open: (c: WebCtx) => Promise<void>;
  /** The element the shot spans, top to bottom; the main pane by default. */
  crop?: string;
  /** A signed-out page. */
  anonymous?: boolean;
}

const MAIN = "main";

const WEB_SHOTS: WebShot[] = [
  {
    name: "login",
    anonymous: true,
    open: async (c) => {
      await c.page.goto(`${BASE()}/?theme=${c.theme}`, { waitUntil: "domcontentloaded" });
      await c.page.waitForSelector("button, a[href*='login'], form", { timeout: 30_000 });
    },
  },
  {
    name: "workspace-home",
    open: (c) => go(c, "", '[data-testid^="project-card"]'),
  },
  {
    name: "project-overview",
    open: (c) => go(c, `/p/${c.pid}/s/main/overview`, '[data-slot="overview-suggestions"] [data-slot="overview-suggestion"]'),
  },
  {
    name: "project-reader",
    open: async (c) => {
      await go(c, `/p/${c.pid}/s/main/overview`, '[data-slot="overview-file"]');
      const withFindings = c.page
        .locator('[data-slot="overview-file"]')
        .filter({ has: c.page.locator('[data-slot="overview-file-findings"]') })
        .first();
      if (!(await withFindings.count())) {
        // A fresh seed has not been checked yet: run the check once, as a
        // member would, so a file carries its finding.
        await c.page.locator('[data-slot="overview-run"]').click();
        await c.page.waitForFunction(
          () => !document.querySelector('[data-slot="overview-run"]')?.textContent?.includes("Checking"),
          undefined,
          { timeout: 120_000 },
        );
        await withFindings.waitFor({ timeout: 30_000 });
      }
      await withFindings.locator('[data-slot="overview-file-open"]').click();
      await c.page.waitForSelector('[data-slot="reader-blocks"]', { timeout: 30_000 });
      await c.page.locator('[data-slot="reader-finding"]').first().waitFor({ timeout: 20_000 });
      await c.page.waitForSelector('[data-slot="reader-rules"]', { timeout: 20_000 });
    },
  },
  {
    name: "project-rules",
    open: async (c) => {
      await go(c, `/p/${c.pid}/s/main/rules`, '[data-slot="decide-item"]');
      await c.page.locator('[data-slot="decide-item"]').first().click();
      await c.page.waitForSelector('[data-slot="decide-question"]', { timeout: 20_000 });
      await c.page.waitForSelector('[data-slot="kept-rules"] [data-slot="kept-rule"]', { timeout: 20_000 });
    },
  },
  {
    name: "review-inbox",
    open: (c) => go(c, "/review-inbox", '[data-testid="review-inbox"]'),
  },
  {
    name: "review-session",
    open: async (c) => {
      const pid = process.env.BOWRAIN_PROJECT_ID;
      if (!pid) throw new Error("BOWRAIN_PROJECT_ID is not set: run make -C harness seed");
      await go(c, `/p/${pid}/s/main/review`, '[data-testid="review-session"]');
      const row = c.page.locator('[data-testid^="queue-row-"]').first();
      await row.waitFor({ timeout: 20_000 });
      await row.click();
      await c.page.waitForSelector('[data-testid="focused-reviewer"]', { timeout: 20_000 });
    },
  },
  {
    name: "editor",
    open: async (c) => {
      await go(c, "", '[data-testid^="project-card"]');
      await c.page.locator('[data-testid^="project-card"]', { hasText: "Company Website" }).first().click();
      await dismissDesktopPrompt(c.page);
      await c.page.getByTestId("subnav-source").click();
      await c.page.locator('[data-testid^="open-file"]').first().click();
      await c.page.waitForSelector('[data-testid="file-preview"]', { timeout: 20_000 });
      await c.page.getByTestId("file-preview-translate").click();
      await c.page.waitForSelector('[data-testid="view-switcher"]', { timeout: 30_000 });
      await c.page.getByTestId("view-visual").click();
      await c.page.waitForSelector('[data-testid="visual-editor-layout"]', { timeout: 20_000 });
      const wanted = (process.env.BOWRAIN_TERM_BLOCK_TEXT ?? "").replace(/\s+/g, " ").trim();
      const card = c.page.getByTestId("visual-editor-card").first();
      const onBlock = async () =>
        wanted
          ? (await card.innerText().catch(() => "")).replace(/\s+/g, " ").includes(wanted)
          : (await c.page.getByTestId("term-sidebar").count()) > 0;
      for (let i = 0; i < 20 && !(await onBlock()); i++) {
        const next = c.page.getByTestId("next-block-btn");
        if (!(await next.isEnabled().catch(() => false))) break;
        await next.click();
        await c.page.waitForTimeout(500);
      }
      await c.page.getByTestId("term-sidebar").waitFor({ timeout: 10_000 }).catch(() => {
        throw new Error("editor: no block in the file shows the term sidebar; re-run make -C harness seed");
      });
    },
  },
  {
    name: "concepts",
    open: async (c) => {
      await go(c, "/context/concepts", '[data-testid="concept-list"]');
      const row = c.page.getByTestId("concept-row").filter({ has: c.page.getByText("encryption", { exact: true }) });
      await (await row.count() ? row : c.page.getByTestId("concept-row")).first().click();
      await c.page.waitForSelector('[data-testid="concept-view"]', { timeout: 20_000 });
    },
  },
  {
    name: "content-memory",
    open: async (c) => {
      await go(c, "/context/memory", '[data-slot="memory-pair"]');
      await c.page.locator('[data-slot="memory-pair"]').first().click();
      await c.page.waitForSelector('[data-slot="memory-pair-detail"]', { timeout: 20_000 });
    },
  },
  {
    name: "members",
    open: (c) => go(c, "/settings/members", MAIN),
  },
  {
    name: "automations",
    open: async (c) => {
      // The seed's push to the Help Center project ran its automation once.
      const heading = c.page.locator("h3", { hasText: "Automation Runs" });
      await go(c, `/p/${c.pid}/s/main/automations`, "h3");
      await heading.waitFor({ timeout: 20_000 });
      await heading.locator("xpath=following-sibling::*[1]").click();
      await c.page.getByText(/^Run [0-9a-zA-Z]{8}/).first().waitFor({ timeout: 20_000 });
    },
  },
  {
    name: "settings",
    open: (c) => go(c, "/settings", MAIN),
  },
];

async function shootBowrain(outRoot: string, only?: string[]): Promise<void> {
  loadEnv();
  const slug = process.env.BOWRAIN_WORKSPACE_SLUG || "bowmart";
  const pid = process.env.BOWRAIN_SCREENS_PROJECT_ID;
  if (!pid) throw new Error("BOWRAIN_SCREENS_PROJECT_ID is not set in harness/.env: run make -C harness seed");
  const email = process.env.BOWRAIN_ALICE_EMAIL || "admin@example.com";
  const name = process.env.BOWRAIN_ALICE_NAME || "Alex Rivera";
  const host = new URL(BASE()).hostname;
  const shots = only?.length ? WEB_SHOTS.filter((s) => only.includes(s.name)) : WEB_SHOTS;

  let session: { token: string; at: number } | undefined;
  const browser = await chromium.launch();
  try {
    for (const theme of THEMES) {
      for (const shot of shots) {
        // A fresh context per shot: no state carries between screens, and the
        // session never runs out mid-pass.
        const context = await newContext(browser, theme);
        await context.addCookies([
          { name: "neokapi-theme", value: theme, domain: host, path: "/", sameSite: "Lax", secure: false },
        ]);
        if (!shot.anonymous) {
          // The server limits device-flow starts, and a token lives 15 min:
          // mint one and renew it after ten.
          if (!session || Date.now() - session.at > 10 * 60_000) {
            session = { token: await deviceAuth(email, name), at: Date.now() };
          }
          const token = session.token;
          await context.addCookies([
            { name: "bowrain_session", value: token, domain: host, path: "/api/", httpOnly: true, sameSite: "Lax", secure: false },
          ]);
        }
        const page = await context.newPage();
        await page.addInitScript((m) => {
          try {
            localStorage.setItem("neokapi-theme", m);
          } catch {
            /* storage unavailable */
          }
        }, theme);
        const c: WebCtx = { page, theme, ws: `${BASE()}/${slug}`, pid };
        await shot.open(c);
        await waitSettled(page, shot.name);
        await shoot(page, path.join(outRoot, theme, `${shot.name}.png`), shot.crop ?? (shot.anonymous ? undefined : MAIN));
        await context.close();
      }
    }
  } finally {
    await browser.close();
  }
}

// ── CLI ──────────────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  const [mode, out, ...rest] = process.argv.slice(2);
  const flag = (f: string) => {
    const i = rest.indexOf(f);
    return i >= 0 ? rest[i + 1] : undefined;
  };
  if (!out || (mode !== "bowrain" && mode !== "desktop")) {
    console.error("usage: shoot-docs-screens.ts bowrain|desktop <out-root> [--storybook <url>] [--only a,b]");
    process.exit(2);
  }
  const outRoot = path.resolve(out);
  const only = flag("--only")?.split(",");
  console.log(`${mode} → ${path.relative(REPO_ROOT, outRoot) || outRoot}`);
  if (mode === "desktop") await shootDesktop(outRoot, flag("--storybook"));
  else await shootBowrain(outRoot, only);
}

main().catch((err) => {
  console.error(err instanceof Error ? err.message : err);
  process.exit(1);
});
