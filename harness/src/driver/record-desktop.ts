/**
 * record-desktop.ts — record a Kapi Desktop walkthrough screencast.
 *
 * Drives the REAL Kapi Desktop UI (apps/kapi-desktop/frontend `demo.html`,
 * which mounts the genuine IconSidebar + TermbaseBrowser + TMBrowser with
 * in-browser sample data) in Chromium via Playwright, with a visible human-like
 * cursor and ripple clicks (see cursor-helper.ts). Records a light + dark
 * `.webm` and a `screencast.json` of timed beats (+ zoom regions) into
 * public/<id>/, which the Remotion "desktop" scene replays inside the macOS
 * window frame.
 *
 * Self-contained: starts the frontend `vp dev` server itself (unless DEMO_URL
 * is set), records both themes, and shuts the server down.
 */
import fs from "node:fs";
import path from "node:path";
import net from "node:net";
import os from "node:os";
import { spawn } from "node:child_process";
import { chromium, type Page, type Browser, type Locator } from "playwright";
import { ensureDir, publicDemoDir, REPO_ROOT } from "../lib/paths.ts";
import { injectCursor, moveTo, humanClick, humanType, idle } from "./cursor-helper.ts";

const WIDTH = 1440;
const HEIGHT = 900;

type ThemeMode = "light" | "dark";

/** A normalized [0,1] zoom rect over the video, or null for the full frame. */
export interface ZoomRect {
  x: number;
  y: number;
  w: number;
  h: number;
}
export interface Beat {
  id: string;
  /** Seconds from the start of the recording. */
  tStart: number;
  tEnd: number;
  zoom: ZoomRect | null;
}
export interface Screencast {
  width: number;
  height: number;
  video: Record<ThemeMode, string>;
  /** Beats recorded per theme (pacing is near-identical, but kept exact). */
  beats: Record<ThemeMode, Beat[]>;
}

const FRONTEND_DIR = path.join(REPO_ROOT, "apps", "kapi-desktop", "frontend");

function waitPort(port: number, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  return new Promise((resolve, reject) => {
    const tick = () => {
      // Use "localhost" with dual-stack auto-select: Vite binds IPv6 ([::1]) while
      // the Go bridge binds IPv4 (127.0.0.1), so probing a single family misses one.
      const sock = net.connect({ port, host: "localhost", autoSelectFamily: true });
      sock.once("connect", () => {
        sock.destroy();
        resolve();
      });
      sock.once("error", () => {
        sock.destroy();
        if (Date.now() > deadline) reject(new Error(`dev server :${port} did not start in ${timeoutMs}ms`));
        else setTimeout(tick, 400);
      });
    };
    tick();
  });
}

const KAPI_DESKTOP_DIR = path.join(REPO_ROOT, "apps", "kapi-desktop");
// Isolated roots so the real app never touches the developer's own data
// (honored via KAPI_CONFIG_DIR / KAPI_HOME_DIR / KAPI_DESKTOP_CONFIG_DIR — see
// backend/paths.go). The home dir holds created projects (project walkthrough).
const ISO_BASE = path.join(os.tmpdir(), "kapi-desktop-demo");
const ISO_DIR = path.join(ISO_BASE, "kapi");
const ISO_HOME = path.join(ISO_BASE, "home");
const ISO_DESKTOP = path.join(ISO_BASE, "desktop");
const ICU_PKGCONFIG = "/opt/homebrew/opt/icu4c/lib/pkgconfig";

/** Env for the Go builds/run: cgo + fts5 deps + isolated config/home roots. */
function goEnv(extra: Record<string, string> = {}): NodeJS.ProcessEnv {
  const pkg = fs.existsSync(ICU_PKGCONFIG)
    ? `${ICU_PKGCONFIG}:${process.env.PKG_CONFIG_PATH ?? ""}`
    : process.env.PKG_CONFIG_PATH ?? "";
  return {
    ...process.env,
    CGO_ENABLED: "1",
    PKG_CONFIG_PATH: pkg,
    KAPI_CONFIG_DIR: ISO_DIR,
    KAPI_HOME_DIR: ISO_HOME,
    KAPI_DESKTOP_CONFIG_DIR: ISO_DESKTOP,
    ...extra,
  };
}

function runToCompletion(cmd: string, args: string[], opts: { cwd: string; env: NodeJS.ProcessEnv }): Promise<void> {
  return new Promise((resolve, reject) => {
    const c = spawn(cmd, args, { ...opts, stdio: "inherit" });
    c.on("error", reject);
    c.on("exit", (code) => (code === 0 ? resolve() : reject(new Error(`${cmd} ${args.join(" ")} exited ${code}`))));
  });
}

/**
 * Start the REAL desktop stack, isolated from the developer's data:
 *   1. seed an isolated config root (cmd/seed-demo → KAPI_CONFIG_DIR)
 *   2. build + run the wbridge HTTP server hosting the real backend.App
 *   3. run the frontend Vite dev server (serves real.html)
 * Returns the recording URL + a teardown. `go` builds need cgo + fts5 + icu4c.
 */
async function startRealStack(): Promise<{ url: string; teardown: () => Promise<void> }> {
  fs.rmSync(path.dirname(ISO_DIR), { recursive: true, force: true });
  fs.mkdirSync(ISO_DIR, { recursive: true });

  console.log(`  · seeding isolated config (${ISO_DIR})`);
  await runToCompletion("go", ["run", "-tags", "fts5", "./cmd/seed-demo"], { cwd: KAPI_DESKTOP_DIR, env: goEnv() });

  console.log("  · building + starting wbridge (real backend over HTTP)");
  const bridgeBin = path.join(os.tmpdir(), "kapi-wbridge-rec");
  await runToCompletion("go", ["build", "-tags", "fts5", "-o", bridgeBin, "./cmd/wbridge"], { cwd: KAPI_DESKTOP_DIR, env: goEnv() });
  const bridge = spawn(bridgeBin, [], { env: goEnv({ WBRIDGE_PORT: "5175" }), stdio: "ignore" });
  await waitPort(5175, 60_000);

  console.log("  · starting frontend dev server (:5174)");
  const vite = spawn("vp", ["dev"], { cwd: FRONTEND_DIR, env: { ...process.env, FORCE_COLOR: "0" }, stdio: "ignore" });
  await waitPort(5174, 120_000);

  return {
    url: "http://localhost:5174/real.html",
    teardown: async () => {
      bridge.kill("SIGTERM");
      vite.kill("SIGTERM");
      await new Promise((r) => setTimeout(r, 600));
    },
  };
}

// ── Walkthrough scripts ───────────────────────────────────────────────────
// Each demo has its own walkthrough keyed by id. A WalkCtx gives every script
// the same timed-beat + element-zoom helpers; the per-beat zoom frames the real
// component (from its bounding box) so nothing is cut off.

interface WalkCtx {
  page: Page;
  /** A fixed-rect beat (sidebar/full views). zoom=null → full frame. */
  beat: (id: string, zoom: ZoomRect | null, fn: () => Promise<void>) => Promise<void>;
  /** A beat whose zoom is derived from elements AFTER its actions settle. */
  beatEls: (id: string, selectors: string[], fn: () => Promise<void>) => Promise<void>;
  /** Move the visible cursor onto an element's centre. */
  cursorTo: (selector: string, duration?: number) => Promise<void>;
  /** A sidebar nav button by its aria-label. */
  sidebar: (label: string) => Locator;
}

function makeCtx(page: Page, t0: number, beats: Beat[]): WalkCtx {
  const now = () => (Date.now() - t0) / 1000;
  const sidebar = (label: string) => page.locator(`button[aria-label="${label}"]`);
  const beat = async (id: string, zoom: ZoomRect | null, fn: () => Promise<void>) => {
    const tStart = now();
    await fn();
    beats.push({ id, tStart, tEnd: now(), zoom });
  };
  const unionZoom = async (selectors: string[], pad = 0.04): Promise<ZoomRect | null> => {
    let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity, any = false;
    for (const s of selectors) {
      const box = await page.locator(s).first().boundingBox().catch(() => null);
      if (!box) continue;
      any = true;
      x0 = Math.min(x0, box.x);
      y0 = Math.min(y0, box.y);
      x1 = Math.max(x1, box.x + box.width);
      y1 = Math.max(y1, box.y + box.height);
    }
    if (!any) return null;
    let x = x0 / WIDTH - pad;
    let y = y0 / HEIGHT - pad;
    let w = (x1 - x0) / WIDTH + 2 * pad;
    let h = (y1 - y0) / HEIGHT + 2 * pad;
    x = Math.max(0, x);
    y = Math.max(0, y);
    w = Math.min(1 - x, w);
    h = Math.min(1 - y, h);
    return { x, y, w, h };
  };
  const cursorTo = async (selector: string, duration = 600) => {
    const box = await page.locator(selector).first().boundingBox().catch(() => null);
    if (box) await moveTo(page, box.x + box.width / 2, box.y + box.height / 2, duration);
  };
  const beatEls = async (id: string, selectors: string[], fn: () => Promise<void>) => {
    const tStart = now();
    await fn();
    beats.push({ id, tStart, tEnd: now(), zoom: await unionZoom(selectors) });
  };
  return { page, beat, beatEls, cursorTo, sidebar };
}

/** Termbase + translation-memory explorer. */
async function explorerWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  await beat("intro", null, async () => {
    await idle(page, 2200);
  });
  await beat("open-termbases", { x: 0, y: 0.04, w: 0.34, h: 0.66 }, async () => {
    await humanClick(page, sidebar("Termbases"));
    await page.waitForTimeout(700);
  });
  await beat("open-glossary", null, async () => {
    await humanClick(page, page.locator('button:has-text("product-glossary")'));
    await page.waitForTimeout(1100);
  });
  await beatEls("inspect-concept", ['[data-testid="concept-c-01"]'], async () => {
    await page.locator('[data-testid="concept-c-01"]').scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(300);
    await cursorTo('[data-testid="concept-c-01"]');
    await page.waitForTimeout(2200);
  });
  await beatEls("search-term", ['[data-testid="filterbar-search"]', '[data-testid="concept-c-07"]'], async () => {
    await humanType(page, page.getByTestId("filterbar-search"), "invoice", { submit: true });
    await page.waitForTimeout(1500);
  });
  await beat("open-tm", { x: 0, y: 0.04, w: 0.34, h: 0.66 }, async () => {
    await humanClick(page, sidebar("Translation Memories"));
    await page.waitForTimeout(600);
    await humanClick(page, page.locator('button:has-text("acme-app")'));
    await page.waitForTimeout(1100);
  });
  await beatEls("inspect-tm", ['[data-testid="tm-entry-tm-01"]', '[data-testid="tm-entry-tm-02"]'], async () => {
    await page.locator('[data-testid="tm-entry-tm-01"]').scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(300);
    await cursorTo('[data-testid="tm-entry-tm-02"]');
    await page.waitForTimeout(2300);
  });
  await beatEls("entity", ['[data-testid="tm-entry-tm-03"]'], async () => {
    await page.locator('[data-testid="tm-entry-tm-03"]').scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(300);
    await cursorTo('[data-testid="tm-entry-tm-03"]');
    await page.waitForTimeout(2200);
  });
  await beatEls("search-tm", ['[data-testid="tm-search"]', '[data-testid="tm-entry-tm-04"]'], async () => {
    await page.getByTestId("tm-search").scrollIntoViewIfNeeded().catch(() => {});
    await humanType(page, page.getByTestId("tm-search"), "invite", { submit: true });
    await page.waitForTimeout(1500);
  });
}

/** Create and manage a project. */
async function projectsWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  await beat("intro", null, async () => {
    await idle(page, 2000);
  });
  // Open the New Project dialog and name the project.
  await beatEls("new-project", ['button:has-text("Create Project")', 'input[placeholder="My App"]'], async () => {
    await humanClick(page, page.locator('button:has-text("New Project")').first());
    await page.waitForTimeout(400);
    await humanType(page, page.locator('input[placeholder="My App"]'), "Acme Help Center");
    await page.waitForTimeout(700);
  });
  // Create it → the Get Started template picker.
  await beatEls("templates", ['button:has-text("Input → Output")', 'button:has-text("Start empty")'], async () => {
    await humanClick(page, page.locator('button:has-text("Create Project")'));
    await page.waitForTimeout(1100);
  });
  // Pick a structure → the project overview (full frame).
  await beat("project-home", null, async () => {
    await humanClick(page, page.locator('button:has-text("Input → Output")'));
    await page.waitForTimeout(1300);
  });
  // Open Project Settings to configure languages.
  await beat("project-settings", { x: 0.02, y: 0.05, w: 0.7, h: 0.7 }, async () => {
    await humanClick(page, sidebar("Project Settings"));
    await page.waitForTimeout(1300);
  });
  // Glance at the Content view (file patterns).
  await beat("content", { x: 0.02, y: 0.05, w: 0.92, h: 0.6 }, async () => {
    await humanClick(page, sidebar("Content"));
    await page.waitForTimeout(1400);
  });
}

/** Configuration: appearance, AI credentials, plugins. */
async function configWalk(c: WalkCtx): Promise<void> {
  const { page, beat, sidebar } = c;
  const tab = (label: string) => page.locator(`[role="tab"]:has-text("${label}")`);
  await beat("intro", null, async () => {
    await idle(page, 2000);
  });
  // Open App Settings → General.
  await beat("open-settings", { x: 0, y: 0.04, w: 0.34, h: 0.66 }, async () => {
    await humanClick(page, sidebar("App Settings"));
    await page.waitForTimeout(900);
  });
  // General: appearance + UI language (do NOT click theme — it would flip the recording).
  await beat("general", { x: 0.02, y: 0.06, w: 0.6, h: 0.66 }, async () => {
    await moveTo(page, WIDTH * 0.2, HEIGHT * 0.32, 700);
    await page.waitForTimeout(2200);
  });
  // AI Credentials tab (seeded demo providers — no real keychain entries).
  await beat("credentials", { x: 0.02, y: 0.06, w: 0.96, h: 0.46 }, async () => {
    await humanClick(page, tab("AI Credentials"));
    await page.waitForTimeout(1500);
  });
  // Plugins tab.
  await beat("plugins", { x: 0.02, y: 0.06, w: 0.96, h: 0.6 }, async () => {
    await humanClick(page, tab("Plugins"));
    await page.waitForTimeout(1600);
  });
}

/** Flows: the built-in library and a flow's pipeline. */
async function flowsWalk(c: WalkCtx): Promise<void> {
  const { page, beat, sidebar } = c;
  await beat("intro", null, async () => {
    await idle(page, 2000);
  });
  // Open the Flows library (full — shows the built-in flows).
  await beat("library", null, async () => {
    await humanClick(page, sidebar("Flows"));
    await page.waitForTimeout(1100);
  });
  // Linger on the library of composable flows.
  await beat("library-zoom", { x: 0.02, y: 0.16, w: 0.96, h: 0.52 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.4, 700);
    await page.waitForTimeout(2000);
  });
  // Open a flow → its pipeline graph (Input → Redact → AI Translate → Unredact → Output).
  await beat("open-flow", null, async () => {
    await humanClick(page, page.getByText("Secure Translate", { exact: true }));
    await page.waitForTimeout(1300);
  });
  // Zoom the pipeline column.
  await beat("pipeline", { x: 0.36, y: 0.12, w: 0.3, h: 0.82 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.45, 700);
    await page.waitForTimeout(2400);
  });
}

const WALKTHROUGHS: Record<string, (c: WalkCtx) => Promise<void>> = {
  "kapi-desktop-explorer": explorerWalk,
  "kapi-desktop-projects": projectsWalk,
  "kapi-desktop-config": configWalk,
  "kapi-desktop-flows": flowsWalk,
};

async function runWalkthrough(page: Page, t0: number, demoId: string): Promise<Beat[]> {
  const walk = WALKTHROUGHS[demoId];
  if (!walk) throw new Error(`no walkthrough registered for "${demoId}"`);
  const beats: Beat[] = [];
  await walk(makeCtx(page, t0, beats));
  return beats;
}

async function recordTheme(browser: Browser, url: string, theme: ThemeMode, outDir: string, demoId: string): Promise<{ webm: string; beats: Beat[] }> {
  const videoDir = ensureDir(path.join(outDir, `_rec-${theme}`));
  const context = await browser.newContext({
    viewport: { width: WIDTH, height: HEIGHT },
    deviceScaleFactor: 2,
    colorScheme: theme,
    recordVideo: { dir: videoDir, size: { width: WIDTH, height: HEIGHT } },
  });
  // Pin the palette deterministically: set the `.dark` class at document-start on
  // every load, so the theme can't drift if the SPA re-renders or reloads mid-run.
  await context.addInitScript((isDark) => {
    const apply = () => document.documentElement.classList.toggle("dark", isDark);
    apply();
    document.addEventListener("DOMContentLoaded", apply);
  }, theme === "dark");
  const t0 = Date.now();
  const page = await context.newPage();
  await page.emulateMedia({ colorScheme: theme });
  await page.goto(`${url}?theme=${theme}`, { waitUntil: "networkidle" });
  await page.waitForSelector("h1", { timeout: 15_000 });
  await injectCursor(page);
  await page.waitForTimeout(400);

  const beats = await runWalkthrough(page, t0, demoId);
  await page.waitForTimeout(500);

  const video = page.video();
  await context.close(); // finalizes the webm
  const raw = video ? await video.path() : "";

  const webm = path.join(outDir, `screencast-${theme}.webm`);
  if (raw && fs.existsSync(raw)) fs.copyFileSync(raw, webm);
  fs.rmSync(videoDir, { recursive: true, force: true });
  return { webm: path.basename(webm), beats };
}

export interface RecordOptions {
  force?: boolean;
}

/** Record the desktop walkthrough for demo <id> → public/<id>/screencast.json + webms. */
export async function recordDesktop(id: string, opts: RecordOptions = {}): Promise<Screencast> {
  const outDir = ensureDir(publicDemoDir(id));
  const jsonPath = path.join(outDir, "screencast.json");
  if (!opts.force && fs.existsSync(jsonPath) && fs.existsSync(path.join(outDir, "screencast-light.webm"))) {
    console.log(`  · screencast exists for ${id} (use --force to re-record)`);
    return JSON.parse(fs.readFileSync(jsonPath, "utf8"));
  }

  // DEMO_URL points at an already-running stack (debugging); otherwise start the
  // real app stack ourselves (seed + wbridge + vite), isolated from user data.
  const externalUrl = process.env.DEMO_URL;
  let teardown: (() => Promise<void>) | undefined;
  let url = externalUrl ?? "";
  if (!externalUrl) {
    const stack = await startRealStack();
    url = stack.url;
    teardown = stack.teardown;
  }

  // Reset created projects (isolated home) before each theme so state-mutating
  // walkthroughs (e.g. project creation) start clean on both passes. The seeded
  // termbases/TMs/providers under ISO_DIR are left intact.
  const resetHome = () => {
    fs.rmSync(ISO_HOME, { recursive: true, force: true });
    fs.mkdirSync(ISO_HOME, { recursive: true });
  };

  const browser = await chromium.launch();
  try {
    console.log("  · recording light theme");
    resetHome();
    const light = await recordTheme(browser, url, "light", outDir, id);
    console.log("  · recording dark theme");
    resetHome();
    const dark = await recordTheme(browser, url, "dark", outDir, id);

    const screencast: Screencast = {
      width: WIDTH,
      height: HEIGHT,
      video: { light: light.webm, dark: dark.webm },
      beats: { light: light.beats, dark: dark.beats },
    };
    fs.writeFileSync(jsonPath, JSON.stringify(screencast, null, 2));
    console.log(`  ✓ recorded ${id}: ${light.beats.length} beats, light+dark`);
    return screencast;
  } finally {
    await browser.close();
    if (teardown) await teardown();
  }
}

// Allow direct invocation: tsx src/driver/record-desktop.ts <id>
if (import.meta.url === `file://${process.argv[1]}`) {
  const id = process.argv[2] || "kapi-desktop-explorer";
  const force = process.argv.includes("--force");
  recordDesktop(id, { force }).catch((e) => {
    console.error("record-desktop error:", e?.stack || e?.message || e);
    process.exit(1);
  });
}
