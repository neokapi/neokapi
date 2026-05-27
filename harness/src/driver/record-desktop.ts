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
import { chromium, type Page, type Browser } from "playwright";
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
// Isolated config root so the real app never reads the developer's own
// termbases/TMs/settings (honored via KAPI_CONFIG_DIR — see backend/paths.go).
const ISO_DIR = path.join(os.tmpdir(), "kapi-desktop-demo", "kapi");
const ICU_PKGCONFIG = "/opt/homebrew/opt/icu4c/lib/pkgconfig";

/** Env for the Go builds/run: cgo + fts5 deps + isolated config root. */
function goEnv(extra: Record<string, string> = {}): NodeJS.ProcessEnv {
  const pkg = fs.existsSync(ICU_PKGCONFIG)
    ? `${ICU_PKGCONFIG}:${process.env.PKG_CONFIG_PATH ?? ""}`
    : process.env.PKG_CONFIG_PATH ?? "";
  return { ...process.env, CGO_ENABLED: "1", PKG_CONFIG_PATH: pkg, KAPI_CONFIG_DIR: ISO_DIR, ...extra };
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

/**
 * The walkthrough script. Each beat runs its actions, and we timestamp its
 * span relative to the recording start so the renderer can play the exact slice
 * with the right zoom.
 */
async function runWalkthrough(page: Page, t0: number): Promise<Beat[]> {
  const beats: Beat[] = [];
  const now = () => (Date.now() - t0) / 1000;
  const beat = async (id: string, zoom: ZoomRect | null, fn: () => Promise<void>) => {
    const tStart = now();
    await fn();
    beats.push({ id, tStart, tEnd: now(), zoom });
  };

  const sidebar = (label: string) => page.locator(`button[aria-label="${label}"]`);

  // Zoom rects are normalized [0,1] regions of the app; the renderer magnifies
  // them to fill the frame. Keep them well under full width/height so the zoom
  // is visibly ~1.4–2×, and align them with where the cursor lingers.

  // 1 — Home: the app at rest (full frame).
  await beat("intro", null, async () => {
    await idle(page, 2200);
  });

  // 2 — Open the Termbases section (zoom toward the sidebar + first cards).
  await beat("open-termbases", { x: 0, y: 0.04, w: 0.34, h: 0.66 }, async () => {
    await humanClick(page, sidebar("Termbases"));
    await page.waitForTimeout(700);
  });

  // 3 — Open a glossary (full frame — all concepts appear).
  await beat("open-glossary", null, async () => {
    await humanClick(page, page.locator('button:has-text("product-glossary")'));
    await page.waitForTimeout(900);
  });

  // 4 — Inspect a concept: the leading "seat" card (top-left) shows approved +
  // deprecated terms across languages.
  await beat("inspect-concept", { x: 0.02, y: 0.24, w: 0.5, h: 0.34 }, async () => {
    await moveTo(page, WIDTH * 0.18, HEIGHT * 0.4, 700);
    await page.waitForTimeout(2600);
  });

  // 5 — Search the terminology. The filter bar runs on Enter (submit), and a real
  // term like "invoice" yields a hit — a domain like "billing" would not.
  await beat("search-term", { x: 0.0, y: 0.24, w: 0.86, h: 0.54 }, async () => {
    await humanType(page, page.locator('input[placeholder="Search terminology..."]'), "invoice", { submit: true });
    await page.waitForTimeout(1700);
  });

  // 6 — Cross over to Translation Memories and open one (zoom toward sidebar).
  await beat("open-tm", { x: 0, y: 0.04, w: 0.34, h: 0.66 }, async () => {
    await humanClick(page, sidebar("Translation Memories"));
    await page.waitForTimeout(600);
    await humanClick(page, page.locator('button:has-text("acme-app")'));
    await page.waitForTimeout(900);
  });

  // 7 — Inspect TM entries: bilingual variants + inline-code pills (top entries).
  // The real TM browser has a facet sidebar on the left, so entries start ~x=0.14.
  await beat("inspect-tm", { x: 0.13, y: 0.26, w: 0.62, h: 0.3 }, async () => {
    await moveTo(page, WIDTH * 0.42, HEIGHT * 0.42, 700);
    await page.waitForTimeout(2600);
  });

  // 8 — Entity-aware entry ("Hi {Bob}, …"), the third entry in the list.
  await beat("entity", { x: 0.13, y: 0.46, w: 0.62, h: 0.22 }, async () => {
    await moveTo(page, WIDTH * 0.42, HEIGHT * 0.56, 700);
    await page.waitForTimeout(2400);
  });

  // 9 — Search the memory. TMSearchBar runs on Enter (submit) — "invite" matches.
  await beat("search-tm", { x: 0.1, y: 0.1, w: 0.82, h: 0.5 }, async () => {
    await humanType(page, page.locator('input[placeholder="Search translation memory..."]'), "invite", { submit: true });
    await page.waitForTimeout(1800);
  });

  return beats;
}

async function recordTheme(browser: Browser, url: string, theme: ThemeMode, outDir: string): Promise<{ webm: string; beats: Beat[] }> {
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

  const beats = await runWalkthrough(page, t0);
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

  const browser = await chromium.launch();
  try {
    console.log("  · recording light theme");
    const light = await recordTheme(browser, url, "light", outDir);
    console.log("  · recording dark theme");
    const dark = await recordTheme(browser, url, "dark", outDir);

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
