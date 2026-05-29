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

// Bowrain Desktop recording: the Wails app is a thick client to bowrain-server.
// We host its real backend.App over the wbridge (bowrain/apps/bowrain/cmd/wbridge)
// and serve the real frontend (real.html) in a browser, auto-connecting to a
// running server via BOWRAIN_TOKEN. Distinct ports from kapi-desktop's wbridge.
const BOWRAIN_DESKTOP_DIR = path.join(REPO_ROOT, "bowrain", "apps", "bowrain");
const BOWRAIN_FRONTEND_DIR = path.join(BOWRAIN_DESKTOP_DIR, "frontend");
const BW_ISO = path.join(os.tmpdir(), "bowrain-desktop-demo");
const BW_WBRIDGE_PORT = 5275;
const BW_VITE_PORT = 5274;

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
// The wbridge is built from source (no release ldflags), so core/version.Version
// defaults to "dev". The plugin registry filters by min_kapi_version (okapi-bridge
// requires ≥1.0.0), so stamp a real version or live plugin installs are rejected.
const KAPI_VERSION = "1.0.9";

// ── Bowrain web recording target ─────────────────────────────────────────────
// Bowrain web demos record the real bowrain web app (a browser SPA talking to a
// running bowrain-server) instead of the kapi-desktop wbridge. Auth is a
// device-flow JWT (BOWRAIN_SESSION_TOKEN) planted as the `bowrain_session`
// cookie — the SPA loads straight into the authenticated workspace, no Keycloak.
const BOWRAIN_BASE = process.env.BOWRAIN_BACKEND_URL || "http://localhost:8080";
const BOWRAIN_TOKEN = process.env.BOWRAIN_SESSION_TOKEN || "";

// ── Multi-session (two-user) collaboration ───────────────────────────────────
// Collaboration is bowrain's headline differentiator, so we record it with TWO
// genuine authenticated sessions. The RECORDED camera is "Alice"
// (BOWRAIN_SESSION_TOKEN); a SECOND, off-camera context "Bob"
// (BOWRAIN_PEER_TOKEN) is a distinct workspace member who opens the same file.
// Because the bowrain collab WebSocket (server/ws_collab.go) relays Yjs
// awareness between everyone in a room, Bob's PresenceAvatar genuinely appears
// on Alice's recorded screen — and vice versa. Nothing is faked: two real users
// join the same Yjs room. harness/scripts/seed-collaboration.mjs mints both
// tokens (Alice owns the workspace, invites Bob, joins him) and prints the
// project/item/locale the collaboration walk drives.
//
// NOTE (post-refocus model): connectors are remote-only on desktop, and the
// editor surfaces are Translate (Visual + Table) / Review / Pre-process. The
// collaboration walk reflects that — it lives in the Translate surface where
// PresenceAvatars render, not the retired focus/context-panel modes.
const BOWRAIN_PEER_TOKEN = process.env.BOWRAIN_PEER_TOKEN || "";
const BOWRAIN_PEER_NAME = process.env.BOWRAIN_PEER_NAME || "Maria Schmidt";
// The shared file the two users co-occupy (printed by seed-collaboration.mjs).
const BOWRAIN_PROJECT_ID = process.env.BOWRAIN_PROJECT_ID || "";
const BOWRAIN_ITEM_ID = process.env.BOWRAIN_ITEM_ID || "";
const BOWRAIN_COLLAB_LOCALE = process.env.BOWRAIN_COLLAB_LOCALE || "fr";

/** Resolve the workspace slug for the session token. An explicit
 *  BOWRAIN_WORKSPACE_SLUG wins (a seed run prints the exact one to use, which
 *  matters when several workspaces exist); otherwise fall back to the first. */
async function bowrainWorkspaceSlug(): Promise<string> {
  if (process.env.BOWRAIN_WORKSPACE_SLUG) return process.env.BOWRAIN_WORKSPACE_SLUG;
  const r = await fetch(`${BOWRAIN_BASE}/api/v1/workspaces`, {
    headers: { Authorization: `Bearer ${BOWRAIN_TOKEN}` },
  });
  if (!r.ok) throw new Error(`bowrain: GET /workspaces ${r.status}`);
  const data = (await r.json()) as unknown;
  const ws = (Array.isArray(data) ? data : (data as { workspaces?: unknown[] }).workspaces ?? []) as Array<{ slug: string }>;
  if (!ws.length) throw new Error("bowrain: no workspaces for BOWRAIN_SESSION_TOKEN — seed first");
  return ws[0].slug;
}

interface BowrainCookie {
  name: string;
  value: string;
  domain: string;
  path: string;
  httpOnly: boolean;
  sameSite: "Lax";
  secure: boolean;
}

/** Plant the bowrain session cookie so an SPA context loads authenticated.
 *  Defaults to the recorded user's token; pass a token to authenticate a peer
 *  (off-camera) context as a different user. */
async function bowrainAuthCookie(token: string = BOWRAIN_TOKEN): Promise<BowrainCookie> {
  const u = new URL(BOWRAIN_BASE);
  return {
    name: "bowrain_session",
    value: token,
    domain: u.hostname,
    path: "/api/",
    httpOnly: true,
    sameSite: "Lax",
    secure: u.protocol === "https:",
  };
}

/**
 * Launch the off-camera peer (Bob): a second browser context authenticated as a
 * different user (BOWRAIN_PEER_TOKEN), in its own headless browser so it never
 * lands in the recorded video. Returns a PeerSession the walk drives, plus a
 * teardown. The peer is NOT recorded — it exists purely to produce the live
 * presence/awareness the recorded user sees.
 */
async function launchPeer(slug: string): Promise<{ peer: PeerSession; teardown: () => Promise<void> }> {
  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: WIDTH, height: HEIGHT },
    deviceScaleFactor: 1,
    ignoreHTTPSErrors: true,
  });
  await context.addCookies([await bowrainAuthCookie(BOWRAIN_PEER_TOKEN)]);
  const page = await context.newPage();
  // Land the peer in the authenticated workspace so its session is warm.
  await page.goto(`${BOWRAIN_BASE}/${slug}`, { waitUntil: "domcontentloaded" }).catch(() => {});

  const peer: PeerSession = {
    page,
    name: BOWRAIN_PEER_NAME,
    act: async (fn) => {
      await fn(page);
    },
    openTranslateFile: async (workspace, projectId, itemId, locale) => {
      // The editor route reads the target locale from the project's first target
      // language; `locale` is passed for parity with the collab room key and to
      // document which target both users sit on.
      void locale;
      await page.goto(
        `${BOWRAIN_BASE}/${workspace}/p/${projectId}/s/main/${itemId}/translate`,
        { waitUntil: "domcontentloaded" },
      );
      // Wait for the editor to mount so the peer's useCollaboration() opens the
      // WebSocket and publishes its awareness into the shared room.
      await page
        .waitForSelector('[data-testid="view-switcher"], [data-testid="block-grid"], [data-testid="visual-editor-layout"]', {
          timeout: 30_000,
        })
        .catch(() => {});
      await page.waitForTimeout(1500);
    },
  };

  return {
    peer,
    teardown: async () => {
      await context.close().catch(() => {});
      await browser.close().catch(() => {});
    },
  };
}

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
    // Discover plugins ONLY from the isolated config dir — never the developer's
    // or the machine's globally-installed plugins (so the recorded plugin list
    // is just what the demo installs).
    KAPI_PLUGINS_DIR_ONLY: "1",
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
  await runToCompletion(
    "go",
    ["build", "-tags", "fts5", "-ldflags", `-X github.com/neokapi/neokapi/core/version.Version=${KAPI_VERSION}`, "-o", bridgeBin, "./cmd/wbridge"],
    { cwd: KAPI_DESKTOP_DIR, env: goEnv() },
  );
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
 * Start the REAL Bowrain Desktop stack for recording, isolated from user data
 * and auto-connected to a running bowrain-server:
 *   1. build + run the bowrain wbridge (hosts the real backend.App over HTTP),
 *      with BOWRAIN_TOKEN so it auto-connects to BOWRAIN_BACKEND_URL on first call.
 *   2. run the real frontend (real.html) via Vite.
 * Returns the recording URL + teardown. Requires a device-flow JWT in
 * BOWRAIN_SESSION_TOKEN (the same token the web target uses) and a reachable
 * server at BOWRAIN_BACKEND_URL (default http://localhost:8080).
 */
async function startBowrainStack(): Promise<{ url: string; teardown: () => Promise<void> }> {
  const token = process.env.BOWRAIN_SESSION_TOKEN || process.env.BOWRAIN_TOKEN || "";
  if (!token) throw new Error("bowrain desktop record: set BOWRAIN_SESSION_TOKEN (device-flow JWT)");
  const server = process.env.BOWRAIN_BACKEND_URL || "http://localhost:8080";

  fs.rmSync(BW_ISO, { recursive: true, force: true });
  fs.mkdirSync(BW_ISO, { recursive: true });

  console.log("  · building + starting bowrain wbridge (real backend over HTTP)");
  const bridgeBin = path.join(os.tmpdir(), "bowrain-wbridge-rec");
  await runToCompletion("go", ["build", "-tags", "fts5", "-o", bridgeBin, "./cmd/wbridge"], {
    cwd: BOWRAIN_DESKTOP_DIR,
    env: goEnv(),
  });
  const bridge = spawn(bridgeBin, [], {
    env: goEnv({
      BOWRAIN_DESKTOP_CONFIG_DIR: BW_ISO,
      BOWRAIN_SERVER_URL: server,
      BOWRAIN_TOKEN: token,
      WBRIDGE_PORT: String(BW_WBRIDGE_PORT),
      KAPI_PLUGIN_DIR: path.join(BW_ISO, "plugins"),
    }),
    stdio: "ignore",
  });
  await waitPort(BW_WBRIDGE_PORT, 60_000);

  // Prime the server connection: the first GetConnectionState triggers the
  // BOWRAIN_TOKEN auto-connect (a cold gRPC dial). Do it here so the connection
  // is already established when the frontend loads — otherwise the dashboard
  // races the cold connect and can miss the ready selector.
  const bridgeURL = `http://127.0.0.1:${BW_WBRIDGE_PORT}/wbridge`;
  const callBridge = async (method: string, args: unknown[] = []): Promise<unknown> => {
    const r = await fetch(bridgeURL, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ method, args }),
    });
    return r.json();
  };
  const connectDeadline = Date.now() + 30_000;
  for (;;) {
    try {
      const info = (await callBridge("GetConnectionState")) as { state?: string };
      if (info.state === "connected") {
        console.log("  · bowrain desktop connected to server");
        break;
      }
    } catch {
      /* wbridge not ready yet */
    }
    if (Date.now() > connectDeadline) {
      console.warn("  ! bowrain desktop did not reach connected state; recording anyway");
      break;
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  // Select the first workspace so the backend has an active workspace: the
  // dashboard reads GetCurrentWorkspace/ListProjects, which need one set (the
  // frontend doesn't auto-select on a cold backend).
  try {
    const wss = (await callBridge("ListWorkspaces")) as Array<{ slug?: string }>;
    const slug = process.env.BOWRAIN_WORKSPACE_SLUG || wss?.[0]?.slug;
    if (slug) {
      await callBridge("SelectWorkspace", [slug]);
      console.log(`  · selected workspace ${slug}`);
    }
  } catch {
    /* best effort; the frontend can still drive selection */
  }

  console.log(`  · starting bowrain frontend dev server (:${BW_VITE_PORT})`);
  const vite = spawn("vp", ["dev", "--port", String(BW_VITE_PORT)], {
    cwd: BOWRAIN_FRONTEND_DIR,
    env: { ...process.env, FORCE_COLOR: "0" },
    stdio: "ignore",
  });
  await waitPort(BW_VITE_PORT, 120_000);

  return {
    url: `http://localhost:${BW_VITE_PORT}/real.html`,
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
  /**
   * The off-camera second user, for two-user collaboration walks. Present only
   * when a peer context was launched (record-desktop opts.peer). A walk acts as
   * the peer with `peer.act(fn)` — fn drives the peer's own Playwright page,
   * which is a genuinely separate authenticated session in the same workspace,
   * so its actions (joining a file, selecting a block) reach the recorded user
   * live over the real collab WebSocket. Returns undefined-safe no-ops when no
   * peer is configured, so a walk can be written once and degrade gracefully.
   */
  peer?: PeerSession;
}

/**
 * A second, off-camera authenticated browser session driving a different
 * bowrain user (the recorded video is the FIRST user). The collab server
 * (server/ws_collab.go) relays the peer's Yjs awareness into the recorded
 * user's room, so opening the same file makes the peer's PresenceAvatar appear
 * on the recorded screen — real multi-user presence, captured from one camera.
 */
interface PeerSession {
  page: Page;
  /** The peer's display name (as it appears on their avatar). */
  name: string;
  /** Run an action as the peer (drives the peer's page). */
  act: (fn: (page: Page) => Promise<void>) => Promise<void>;
  /** Open the shared translate file as the peer (joins the same collab room). */
  openTranslateFile: (workspace: string, projectId: string, itemId: string, locale: string) => Promise<void>;
}

function makeCtx(page: Page, t0: number, beats: Beat[], peer?: PeerSession): WalkCtx {
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
  return { page, beat, beatEls, cursorTo, sidebar, peer };
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

/** Open the KapiMart sample project from the home screen. Idempotent: the
 *  scaffold is re-created under the isolated home on each theme pass. */
async function openSample(page: Page, testid: string, readyLabel: string): Promise<void> {
  await page.waitForSelector(`[data-testid="${testid}"]`, { timeout: 15_000 });
  await humanClick(page, page.getByTestId(testid));
  // Wait until the project has opened and its plugins resolve (the gated sidebar
  // item becomes enabled), so subsequent clicks land on a ready project.
  await page.waitForSelector(`button[aria-label="${readyLabel}"]:not([disabled])`, { timeout: 60_000 });
  await page.waitForTimeout(1200);
}

/** Manage content in a project — the KapiMart sample's collections and files. */
async function contentWalk(c: WalkCtx): Promise<void> {
  const { page, beat, sidebar } = c;
  await beat("intro", null, async () => {
    await page.waitForSelector('[data-testid="sample-kapimart"]', { timeout: 15_000 });
    await idle(page, 2000);
  });
  // Open the KapiMart sample → its project overview.
  await beat("open-project", null, async () => {
    await openSample(page, "sample-kapimart", "Content");
  });
  // The overview header: source + target languages across the top.
  await beat("overview", { x: 0.04, y: 0.02, w: 0.92, h: 0.42 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.2, 700);
    await page.waitForTimeout(1800);
  });
  // Open the Content view.
  await beat("content", null, async () => {
    await humanClick(page, sidebar("Content"));
    await page.waitForTimeout(1600);
  });
  // Left column: the file patterns, grouped into collections.
  await beat("patterns", { x: 0.02, y: 0.1, w: 0.5, h: 0.84 }, async () => {
    await moveTo(page, WIDTH * 0.26, HEIGHT * 0.4, 700);
    await page.waitForTimeout(2500);
  });
  // Right column: the real files matched into the project, by format.
  await beat("files", { x: 0.48, y: 0.1, w: 0.5, h: 0.84 }, async () => {
    await moveTo(page, WIDTH * 0.74, HEIGHT * 0.4, 700);
    await page.waitForTimeout(2500);
  });
}

/** Compose flows — the flows defined in a project (KapiMart sample). */
async function flowsWalk(c: WalkCtx): Promise<void> {
  const { page, beat, sidebar } = c;
  await beat("intro", null, async () => {
    await page.waitForSelector('[data-testid="sample-kapimart"]', { timeout: 15_000 });
    await idle(page, 2000);
  });
  // Open the KapiMart sample project.
  await beat("open-project", null, async () => {
    await openSample(page, "sample-kapimart", "Flows");
  });
  // Open the project's Flows.
  await beat("library", null, async () => {
    await humanClick(page, sidebar("Flows"));
    await page.waitForTimeout(1300);
  });
  // The project's flows: translate, translate-and-qa, pseudo-translate.
  await beat("library-zoom", { x: 0.02, y: 0.08, w: 0.96, h: 0.62 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.36, 700);
    await page.waitForTimeout(2200);
  });
  // Open a flow → its pipeline graph (AI translate → quality check).
  await beat("open-flow", null, async () => {
    await humanClick(page, page.getByText("translate-and-qa", { exact: true }));
    await page.waitForTimeout(1500);
  });
  // Zoom the pipeline column (the graph sits right-of-centre).
  await beat("pipeline", { x: 0.4, y: 0.1, w: 0.4, h: 0.84 }, async () => {
    await moveTo(page, WIDTH * 0.6, HEIGHT * 0.45, 700);
    await page.waitForTimeout(2400);
  });
}

/** Install and use the okapi-bridge plugin from the UI. */
async function okapiWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  const tab = (label: string) => page.locator(`[role="tab"]:has-text("${label}")`);
  // The plugin starts uninstalled on each theme pass — see resetPlugins() in
  // recordDesktop, which clears the isolated plugin dir and re-scans the backend.
  await beat("intro", null, async () => {
    await idle(page, 2000);
  });
  // App Settings → Plugins → Available, where the registry is browsable.
  await beat("open-plugins", { x: 0, y: 0.04, w: 0.34, h: 0.66 }, async () => {
    await humanClick(page, sidebar("App Settings"));
    await page.waitForTimeout(700);
    await humanClick(page, tab("Plugins"));
    await page.waitForTimeout(800);
    await humanClick(page, tab("Available"));
    await page.waitForSelector('[data-testid="available-plugin-okapi-bridge"]', { timeout: 30_000 });
    await page.waitForTimeout(800);
  });
  // The Okapi bridge entry in the registry.
  await beatEls_okapiCard(c, "registry");
  // Click Install — the genuine download runs with a live progress bar (streamed
  // plugin-progress events). Stay on the beat, zoomed on the card, through the
  // whole download so the recording captures the bar filling and the flip to
  // installed.
  await beatEls("install", ['[data-testid="available-plugin-okapi-bridge"]'], async () => {
    await humanClick(page, page.getByTestId("install-okapi-bridge"));
    await page.waitForSelector('[data-testid="install-okapi-bridge"]', { state: "detached", timeout: 300_000 });
    await page.waitForTimeout(700);
  });
  // The Installed tab now lists okapi-bridge with its Okapi filter formats.
  await beat("installed", { x: 0.02, y: 0.08, w: 0.96, h: 0.7 }, async () => {
    await humanClick(page, tab("Installed"));
    await page.waitForSelector('[data-testid="installed-plugin-okapi-bridge"]', { timeout: 30_000 });
    await cursorTo('[data-testid="installed-plugin-okapi-bridge"]');
    await page.waitForTimeout(2400);
  });
  // Back home → open the OkapiMart sample, which needs these filters.
  await beat("open-okapimart", null, async () => {
    await humanClick(page, sidebar("Home"));
    await openSample(page, "sample-okapimart", "Content");
  });
  // Its Content resolves through the Okapi filters (okf_* formats).
  await beat("content", null, async () => {
    await humanClick(page, sidebar("Content"));
    await page.waitForTimeout(1600);
  });
  await beat("okf-formats", { x: 0.02, y: 0.1, w: 0.96, h: 0.84 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.45, 700);
    await page.waitForTimeout(2500);
  });
}

/** A beat zoomed onto the okapi-bridge registry card. */
async function beatEls_okapiCard(c: WalkCtx, id: string): Promise<void> {
  const { page, beatEls, cursorTo } = c;
  await beatEls(id, ['[data-testid="available-plugin-okapi-bridge"]'], async () => {
    await cursorTo('[data-testid="available-plugin-okapi-bridge"]');
    await page.waitForTimeout(2200);
  });
}

// ── Bowrain web walkthroughs ─────────────────────────────────────────────────
// These record the real bowrain web app (target: "web"); nav is via data-testid
// (the bowrain sidebar uses testids, not aria-labels).

/** Bowrain web: shared translation memory + terminology governance. */
async function bowrainGovernanceWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo } = c;
  const tap = (id: string) => humanClick(page, page.getByTestId(id));
  await beat("intro", null, async () => {
    await idle(page, 2200);
  });
  // Open the workspace translation memory.
  await beat("open-memory", null, async () => {
    await tap("nav-memory");
    await page.waitForTimeout(1500);
  });
  await beat("tm-list", { x: 0.02, y: 0.1, w: 0.96, h: 0.82 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.42, 700);
    await page.waitForTimeout(2400);
  });
  // Search the memory.
  await beatEls("tm-search", ['[data-testid="tm-search-input"]'], async () => {
    const s = page.getByTestId("tm-search-input");
    if (await s.count()) await humanType(page, s, "mission", { submit: true });
    await page.waitForTimeout(1600);
  });
  // Open the terminology base.
  await beat("open-terms", null, async () => {
    await tap("nav-termbase");
    await page.waitForTimeout(1500);
  });
  await beat("term-list", { x: 0.02, y: 0.1, w: 0.96, h: 0.82 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.42, 700);
    await page.waitForTimeout(2400);
  });
  // Spotlight a concept's multi-locale terms.
  await beatEls("term-detail", ['[data-testid^="term-concept"]'], async () => {
    const sel = '[data-testid^="term-concept"]';
    if (await page.locator(sel).count()) {
      await page.locator(sel).first().scrollIntoViewIfNeeded().catch(() => {});
      await cursorTo(sel);
    }
    await page.waitForTimeout(2300);
  });
}

/** Web translation editor: split source/target grid, live preview, the shared
 *  TM/terminology context panel, and per-locale switching — on files a team
 *  synced from kapi into the workspace. */
async function bowrainEditorWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo } = c;
  await beat("intro", null, async () => {
    await idle(page, 2000);
  });
  // Open the Company Website project (rich HTML content, fr/de/ja targets).
  await beat("open-project", null, async () => {
    const card = page.locator('[data-testid^="project-card"]', { hasText: "Company Website" }).first();
    await humanClick(page, (await card.count()) ? card : page.locator('[data-testid^="project-card"]').first());
    await page.waitForTimeout(1600);
  });
  // Open a file → the editor.
  await beat("open-file", null, async () => {
    await humanClick(page, page.locator('[data-testid^="open-file"]').first());
    await page.waitForTimeout(2400);
  });
  // Switch to the Visual view: an inline editing card over a live document preview.
  // Open the context panel at the tail so it's already present for the next beat.
  await beat("split", { x: 0.03, y: 0.16, w: 0.74, h: 0.5 }, async () => {
    const sh = page.getByTestId("view-visual");
    if (await sh.count()) await humanClick(page, sh);
    await page.waitForTimeout(1600);
    await moveTo(page, WIDTH * 0.42, HEIGHT * 0.42, 700);
    await page.waitForTimeout(1200);
    const cp = page.getByTestId("context-panel-toggle");
    if ((await cp.count()) && !(await page.getByTestId("context-panel").isVisible().catch(() => false)))
      await humanClick(page, cp);
    await page.waitForTimeout(700);
  });
  // Spotlight the shared TM + terminology context, surfaced inline as you edit:
  // select a block that carries a workspace TM match (panel already open).
  await beat("context", { x: 0.43, y: 0.16, w: 0.56, h: 0.62 }, async () => {
    const block = page.locator('[data-testid^="block-row"]', { hasText: "Mission" }).first();
    const target = (await block.count()) ? block : page.getByTestId("block-row-2");
    if (await target.count()) await humanClick(page, target);
    await page.waitForTimeout(900);
    if (await page.getByTestId("context-panel").isVisible().catch(() => false))
      await cursorTo('[data-testid="context-panel"]');
    await page.waitForTimeout(1800);
  });
  // One editor, every target locale.
  await beat("locale", { x: 0.03, y: 0.16, w: 0.8, h: 0.5 }, async () => {
    const sel = page.getByTestId("locale-selector");
    if (await sel.count()) {
      await humanClick(page, sel);
      await page.waitForTimeout(700);
      const opt = page.getByRole("option", { name: /German/ }).first();
      if (await opt.count()) await humanClick(page, opt);
      else await page.keyboard.press("Escape").catch(() => {});
    }
    await page.waitForTimeout(1600);
    await moveTo(page, WIDTH * 0.42, HEIGHT * 0.4, 600);
    await page.waitForTimeout(1200);
  });
  // The live preview renders the translated page as the team works.
  await beat("preview", { x: 0.03, y: 0.63, w: 0.8, h: 0.34 }, async () => {
    const pv = page.getByTestId("preview-iframe");
    if (await pv.count()) await cursorTo('[data-testid="preview-iframe"]');
    await page.waitForTimeout(2200);
  });
}

/** Review & approve: the Review surface steps through a file one block at a time,
 *  approving translations against the workspace — the team workflow on content
 *  pulled in by any connector. */
async function bowrainReviewWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo } = c;
  await beat("intro", null, async () => {
    await idle(page, 2000);
  });
  await beat("open-project", null, async () => {
    const card = page.locator('[data-testid^="project-card"]', { hasText: "Company Website" }).first();
    await humanClick(page, (await card.count()) ? card : page.locator('[data-testid^="project-card"]').first());
    await page.waitForTimeout(1600);
  });
  await beat("open-file", null, async () => {
    await humanClick(page, page.locator('[data-testid^="open-file"]').first());
    await page.waitForTimeout(2400);
  });
  // The Review surface: work through a file by status, one block at a time.
  await beatEls("focus", ['[data-testid="review-surface"]'], async () => {
    const rev = page.getByTestId("surface-tab-review");
    if (await rev.count()) await humanClick(page, rev);
    await page.waitForTimeout(1400);
    const list = page.getByTestId("review-list");
    if (await list.count()) await cursorTo('[data-testid="review-list"]');
    await page.waitForTimeout(1200);
  });
  // Approve — mark reviewed in the Review surface; progress advances to reviewed.
  await beatEls("review", ['[data-testid="review-surface"]'], async () => {
    const mark = page.getByTestId("bulk-mark-reviewed");
    if (await mark.count()) {
      await cursorTo('[data-testid="bulk-mark-reviewed"]');
      await humanClick(page, mark);
    }
    await page.waitForTimeout(1600);
  });
  // Progress across the file: draft → translated → reviewed.
  await beatEls("progress", ['[data-testid="progress-bar"]'], async () => {
    await cursorTo('[data-testid="progress-bar"]');
    await page.waitForTimeout(1800);
  });
}

/**
 * Collaboration — bowrain's headline differentiator, recorded with TWO genuine
 * authenticated users in one shared workspace. The recorded camera is the first
 * user (Alice); a second, off-camera session (Bob, the peer) joins the SAME
 * Translate file. Because the collab WebSocket relays Yjs awareness between
 * everyone in a room, Bob's PresenceAvatar genuinely appears on Alice's
 * recorded screen the moment he opens the file — real multi-user presence, not
 * a mock. The walk then shows the governance frame (members + roles) so the
 * story is "a team, in one governed workspace".
 *
 * Post-refocus surfaces: this lives in the Translate surface (Visual/Table),
 * where PresenceAvatars render in the editor header. Connectors are remote-only
 * on desktop and out of scope here.
 *
 * If no peer is configured (BOWRAIN_PEER_TOKEN unset), the walk still records
 * the single-user editor + governance frames and skips the live-presence beats,
 * so it never fabricates a second user that isn't really there.
 */
async function bowrainCollaborationWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, peer } = c;
  const startUrl = new URL(page.url());
  const wsBase = `${startUrl.origin}${startUrl.pathname}`.replace(/\/+$/, "");
  const themeParam = startUrl.searchParams.get("theme");
  // Carry the recording theme through full-page navigations: a page.goto reloads
  // the SPA, which re-reads its persisted (light) theme; `?theme=` makes the app
  // apply the recording palette so a dark take stays dark.
  const themeQ = themeParam ? `?theme=${themeParam}` : "";

  // Seed values printed by harness/scripts/seed-collaboration.mjs. The workspace
  // slug is the path the recorder landed on.
  const slug = startUrl.pathname.replace(/^\/+|\/+$/g, "").split("/")[0] || "";
  const projectId = BOWRAIN_PROJECT_ID;
  const itemId = BOWRAIN_ITEM_ID;
  const locale = BOWRAIN_COLLAB_LOCALE;
  const canCollab = !!(peer && projectId && itemId);

  await beat("intro", null, async () => {
    await idle(page, 2000);
  });

  // Alice opens the shared file in the Translate surface — alone, for now.
  await beat("open-file", null, async () => {
    if (projectId && itemId) {
      await page.goto(
        `${wsBase}/p/${projectId}/s/main/${itemId}/translate${themeQ}`,
        { waitUntil: "domcontentloaded" },
      );
      await injectCursor(page); // goto wiped the page-injected cursor; re-add it
      await page
        .waitForSelector('[data-testid="view-switcher"], [data-testid="block-grid"]', { timeout: 30_000 })
        .catch(() => {});
    } else {
      // No seed → land Alice on the first project's file via the dashboard.
      const card = page.locator('[data-testid^="project-card"]').first();
      if (await card.count()) await humanClick(page, card);
      await page.waitForTimeout(1400);
      const open = page.locator('[data-testid^="open-file"]').first();
      if (await open.count()) await humanClick(page, open);
    }
    await page.waitForTimeout(1800);
  });

  if (canCollab) {
    // Bob (off-camera) joins the SAME file. His useCollaboration() opens the
    // collab WebSocket and publishes awareness — Alice's editor header now shows
    // his PresenceAvatar arrive. This is the genuine multi-user moment.
    await beatEls("teammate-joins", ['[data-testid="presence-avatars"]'], async () => {
      await peer!.openTranslateFile(slug, projectId, itemId, locale);
      // Let the awareness round-trip reach Alice's recorded page, then settle the
      // camera on the presence avatars as they appear.
      await page
        .waitForSelector('[data-testid="presence-avatars"]', { timeout: 20_000 })
        .catch(() => {});
      await page.waitForTimeout(1200);
      if (await page.getByTestId("presence-avatars").count())
        await cursorTo('[data-testid="presence-avatars"]');
      await page.waitForTimeout(2200);
    });

    // Both users are now in the file. Pan the editor so the shared workspace —
    // one document, two people present — reads clearly.
    await beat("co-editing", { x: 0.02, y: 0.08, w: 0.96, h: 0.7 }, async () => {
      // Bob moves through the file (his own navigation), keeping his session
      // active so the presence stays live while Alice's camera tours the editor.
      await peer!.act(async (bp) => {
        const sw = bp.getByTestId("view-switcher");
        if (await sw.count()) {
          const tbl = bp.getByTestId("view-table");
          if (await tbl.count()) await tbl.click().catch(() => {});
        }
        await bp.waitForTimeout(400);
      });
      await moveTo(page, WIDTH * 0.4, HEIGHT * 0.42, 700);
      await page.waitForTimeout(2400);
    });
  }

  // Governance frame: the workspace is shared and governed — members carry
  // roles (member / admin / viewer), so everyone has exactly the access they
  // should. Shown last so the closing read is "a team, in one governed place".
  await beatEls("members", ['[data-testid="settings-heading"]', '[role="dialog"]', '[data-testid="invite-open-dialog-btn"]'], async () => {
    await page.goto(`${wsBase}/settings/members${themeQ}`, { waitUntil: "domcontentloaded" });
    await injectCursor(page); // re-add cursor after navigation
    await page.waitForSelector('[data-testid="settings-heading"], [data-testid="invite-open-dialog-btn"]', { timeout: 15_000 }).catch(() => {});
    await page.waitForTimeout(1400);
    const open = page.getByTestId("invite-open-dialog-btn");
    if (await open.count()) {
      await cursorTo('[data-testid="invite-open-dialog-btn"]');
      await humanClick(page, open);
      await page.waitForTimeout(900);
      const role = page.getByTestId("invite-role-select");
      if (await role.count()) {
        await humanClick(page, role);
        await page.waitForTimeout(1000);
        await page.keyboard.press("Escape").catch(() => {});
      }
    }
    await page.waitForTimeout(1200);
  });
}

/** Bowrain Desktop: the native app connected to a team's bowrain-server,
 *  showing the same workspace — projects, languages, and file counts. */
async function bowrainDesktopWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls } = c;
  await beat("intro", null, async () => {
    await idle(page, 2200);
  });
  // The workspace totals (projects, words, languages, files).
  await beat("stats", { x: 0.03, y: 0.12, w: 0.94, h: 0.22 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.2, 700);
    await page.waitForTimeout(2000);
  });
  // The project cards — the same projects the team works on, pulled from the server.
  await beat("projects", { x: 0.03, y: 0.27, w: 0.94, h: 0.38 }, async () => {
    await moveTo(page, WIDTH * 0.4, HEIGHT * 0.42, 700);
    await page.waitForTimeout(2200);
  });
  // Connectors — a desktop-only surface for wiring content sources. Open the
  // add-connector dialog so the available connector types are on screen (the
  // empty list alone reads as a blank page).
  await beatEls("connectors", ['[data-testid="connector-form"]', '[role="dialog"]', '[data-testid="add-connector-btn"]'], async () => {
    const n = page.getByTestId("nav-connectors");
    if (await n.count()) await humanClick(page, n);
    await page.waitForTimeout(1400);
    const add = page.getByTestId("add-connector-btn");
    if (await add.count()) await humanClick(page, add);
    await page.waitForTimeout(1600);
  });
}

/** Bowrain Desktop: build localization flows visually (the FlowBuilder is a
 *  desktop-only surface — the web editor has no flow canvas). */
async function bowrainDesktopFlowsWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls } = c;
  await beat("intro", null, async () => {
    await idle(page, 1800);
  });
  // The flow library: built-in pipelines (AI translate, QA, pseudo, …).
  await beat("flows", null, async () => {
    const n = page.getByTestId("nav-flows");
    if (await n.count()) await humanClick(page, n);
    await page.waitForTimeout(2000);
  });
  // Open a multi-step flow to reveal its visual pipeline.
  await beatEls("open", ['[data-testid="flow-builder"]'], async () => {
    const f = page.getByTestId("flow-item-ai-translate-qa");
    if (await f.count()) await humanClick(page, f);
    await page.waitForTimeout(2400);
  });
  // The node graph: input → AI translate → QA → output.
  await beatEls("pipeline", ['[data-testid="flow-builder"]'], async () => {
    await moveTo(page, WIDTH * 0.55, HEIGHT * 0.5, 700);
    await page.waitForTimeout(2400);
  });
}

/** Bowrain web: the correction-learning loop — candidate rules drawn from a
 *  team's corrections, a blast-radius preview, and promotion into a versioned
 *  check. Navigates the review route /:ws/brand/review/:profileId; the workspace
 *  slug comes from BOWRAIN_WORKSPACE_SLUG and the profile id from
 *  BOWRAIN_DEMO_PROFILE_ID (both printed by harness/scripts/seed-correction-loop.mjs). */
async function bowrainCorrectionLoopWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo } = c;
  const startUrl = new URL(page.url());
  const themeQ = startUrl.searchParams.get("theme") ? `?theme=${startUrl.searchParams.get("theme")}` : "";
  // The page opened at origin/<slug>; that pathname is the workspace base.
  const wsBase = `${startUrl.origin}${startUrl.pathname}`.replace(/\/$/, "");
  const profileId = process.env.BOWRAIN_DEMO_PROFILE_ID || "";

  await beat("intro", null, async () => {
    await page.goto(`${wsBase}/brand/review/${profileId}${themeQ}`, { waitUntil: "domcontentloaded" });
    await injectCursor(page); // goto wiped the page-injected cursor; re-add it
    await page.waitForTimeout(2400);
  });

  // The candidate rules — each a phrasing the team kept correcting.
  await beatEls("candidates", ['text=Review suggested rules', "ul"], async () => {
    await page.waitForTimeout(2200);
  });

  // Preview the blast radius: pick a project, open a candidate's impact dialog.
  await beatEls("evaluate", ['[role="dialog"]'], async () => {
    const sel = page.locator("select").first();
    if (await sel.count()) {
      await humanClick(page, sel);
      await sel.selectOption({ index: 1 }).catch(() => {});
      await page.waitForTimeout(700);
    }
    const preview = page.getByRole("button", { name: /Preview impact/i }).first();
    if (await preview.count()) {
      await cursorTo('button:has-text("Preview impact")');
      await humanClick(page, preview);
    }
    await page.waitForTimeout(2400);
    await page.keyboard.press("Escape").catch(() => {});
    await page.waitForTimeout(500);
  });

  // Promote the candidate — it becomes an enforced, versioned check.
  await beatEls("promote", ['button:has-text("Promote")'], async () => {
    const promote = page.getByRole("button", { name: /^Promote$/ }).first();
    if (await promote.count()) {
      await cursorTo('button:has-text("Promote")');
      await humanClick(page, promote);
    }
    await page.waitForTimeout(2000);
  });

  // The queue settles — one fewer mistake that can recur.
  await beat("settled", null, async () => {
    await page.waitForTimeout(2200);
  });
}

const WALKTHROUGHS: Record<string, (c: WalkCtx) => Promise<void>> = {
  "kapi-desktop-explorer": explorerWalk,
  "kapi-desktop-projects": projectsWalk,
  "kapi-desktop-content": contentWalk,
  "kapi-desktop-config": configWalk,
  "kapi-desktop-flows": flowsWalk,
  "kapi-desktop-okapi": okapiWalk,
  "bowrain-web-governance": bowrainGovernanceWalk,
  "bowrain-web-editor": bowrainEditorWalk,
  "bowrain-web-review": bowrainReviewWalk,
  "bowrain-web-collaboration": bowrainCollaborationWalk,
  "bowrain-web-correction-loop": bowrainCorrectionLoopWalk,
  "bowrain-desktop-dashboard": bowrainDesktopWalk,
  "bowrain-desktop-flows": bowrainDesktopFlowsWalk,
};

async function runWalkthrough(page: Page, t0: number, demoId: string, peer?: PeerSession): Promise<Beat[]> {
  const walk = WALKTHROUGHS[demoId];
  if (!walk) throw new Error(`no walkthrough registered for "${demoId}"`);
  const beats: Beat[] = [];
  await walk(makeCtx(page, t0, beats, peer));
  return beats;
}

async function recordTheme(
  browser: Browser,
  url: string,
  theme: ThemeMode,
  outDir: string,
  demoId: string,
  web?: { slug: string },
  ready?: string,
): Promise<{ webm: string; beats: Beat[] }> {
  const videoDir = ensureDir(path.join(outDir, `_rec-${theme}`));
  const context = await browser.newContext({
    viewport: { width: WIDTH, height: HEIGHT },
    deviceScaleFactor: 2,
    colorScheme: theme,
    recordVideo: { dir: videoDir, size: { width: WIDTH, height: HEIGHT } },
    // bowrain web may be served over a locally-trusted (mkcert) cert; Chromium
    // doesn't trust it, so allow it for the recording target.
    ignoreHTTPSErrors: true,
  });
  if (web) await context.addCookies([await bowrainAuthCookie()]);
  // Pin the palette deterministically: set `.dark` at document-start AND re-assert
  // it via a MutationObserver, so an app's own theme logic can't flip the
  // recording mid-run (toggle is idempotent → no loop).
  await context.addInitScript((isDark) => {
    const pin = () => document.documentElement.classList.toggle("dark", isDark);
    pin();
    document.addEventListener("DOMContentLoaded", pin);
    try {
      new MutationObserver(pin).observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
    } catch {
      /* observer unavailable — initial pin still applied */
    }
  }, theme === "dark");
  const t0 = Date.now();
  const page = await context.newPage();
  await page.emulateMedia({ colorScheme: theme });
  if (web) {
    // Land in the authenticated workspace; wait for the app shell, not an h1.
    await page.goto(`${BOWRAIN_BASE}/${web.slug}?theme=${theme}`, { waitUntil: "domcontentloaded" });
    await page.waitForSelector(
      '[data-testid="nav-translate"], [data-testid="new-project-btn"], [data-testid="empty-projects"], [data-testid^="project-card"], nav',
      { timeout: 30_000 },
    );
  } else {
    // "domcontentloaded", not "networkidle": real-main.tsx opens a long-lived SSE
    // connection (/wevents) for streamed backend events, so the network never goes
    // idle. The h1 wait below confirms the app actually rendered.
    await page.goto(`${url}?theme=${theme}`, { waitUntil: "domcontentloaded" });
    // Default kapi-desktop renders an h1 immediately; bowrain-desktop renders its
    // dashboard only after the backend auto-connects to the server (a few
    // seconds), so callers pass a connected-state selector + we allow longer.
    await page.waitForSelector(ready ?? "h1", { timeout: ready ? 30_000 : 15_000 });
  }
  await injectCursor(page);
  await page.waitForTimeout(400);

  // Two-user collaboration: launch the off-camera peer (Bob) for this theme so a
  // collaboration walk can show genuine live presence. The peer is its own
  // browser, never recorded; it is configured only when BOWRAIN_PEER_TOKEN is
  // set, so non-collaboration walks (and a misconfigured run) degrade to a
  // single-user recording rather than fabricating a teammate.
  let peer: PeerSession | undefined;
  let peerTeardown: (() => Promise<void>) | undefined;
  if (web && BOWRAIN_PEER_TOKEN) {
    try {
      const launched = await launchPeer(web.slug);
      peer = launched.peer;
      peerTeardown = launched.teardown;
    } catch (e) {
      console.warn(`  ! peer session failed to launch — recording single-user: ${(e as Error)?.message}`);
    }
  }

  let beats: Beat[];
  try {
    beats = await runWalkthrough(page, t0, demoId, peer);
  } finally {
    if (peerTeardown) await peerTeardown();
  }
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
  /** Record the real bowrain WEB app (external running stack) instead of the
   *  kapi-desktop wbridge: cookie auth + workspace-slug navigation. */
  web?: boolean;
  /** Record the real Bowrain Desktop app via its own wbridge, auto-connected to
   *  a running bowrain-server (BOWRAIN_BACKEND_URL + BOWRAIN_SESSION_TOKEN). */
  bowrainDesktop?: boolean;
}

/** Record the desktop walkthrough for demo <id> → public/<id>/screencast.json + webms. */
export async function recordDesktop(id: string, opts: RecordOptions = {}): Promise<Screencast> {
  const outDir = ensureDir(publicDemoDir(id));
  const jsonPath = path.join(outDir, "screencast.json");
  if (!opts.force && fs.existsSync(jsonPath) && fs.existsSync(path.join(outDir, "screencast-light.webm"))) {
    console.log(`  · screencast exists for ${id} (use --force to re-record)`);
    return JSON.parse(fs.readFileSync(jsonPath, "utf8"));
  }

  // Web target: record the real bowrain web app at BOWRAIN_BACKEND_URL with the
  // session cookie — no wbridge, no local stack to manage here.
  if (opts.web) {
    if (!BOWRAIN_TOKEN) throw new Error("bowrain web record: set BOWRAIN_SESSION_TOKEN (device-flow JWT)");
    const slug = await bowrainWorkspaceSlug();
    const browser = await chromium.launch();
    try {
      console.log(`  · recording bowrain web (light) @ ${BOWRAIN_BASE}/${slug}`);
      const light = await recordTheme(browser, BOWRAIN_BASE, "light", outDir, id, { slug });
      console.log("  · recording bowrain web (dark)");
      const dark = await recordTheme(browser, BOWRAIN_BASE, "dark", outDir, id, { slug });
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
    }
  }

  // Bowrain Desktop target: host the real desktop backend over its wbridge,
  // auto-connected to a running bowrain-server, and drive the real frontend.
  if (opts.bowrainDesktop) {
    const stack = await startBowrainStack();
    const browser = await chromium.launch();
    const ready = '[data-testid="nav-translate"], [data-testid="nav-flows"], [data-testid^="project-card"]';
    try {
      console.log(`  · recording bowrain desktop (light) @ ${stack.url}`);
      const light = await recordTheme(browser, stack.url, "light", outDir, id, undefined, ready);
      console.log("  · recording bowrain desktop (dark)");
      const dark = await recordTheme(browser, stack.url, "dark", outDir, id, undefined, ready);
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
      await stack.teardown();
    }
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

  // Clear any installed plugins before each theme so install walkthroughs start
  // uninstalled on both passes. The wbridge backend is one long-lived process
  // across both themes, so deleting the files isn't enough — tell it to re-scan
  // (LoadPlugins) so its in-memory plugin host matches the now-empty dir. The
  // app installs to KAPI_CONFIG_DIR/plugins (= ISO_DIR/plugins). Best-effort: a
  // no-op for demos that install nothing, and skipped when the stack is external.
  const resetPlugins = async () => {
    fs.rmSync(path.join(ISO_DIR, "plugins"), { recursive: true, force: true });
    if (externalUrl) return;
    try {
      await fetch("http://127.0.0.1:5175/wbridge", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ method: "LoadPlugins", args: [] }),
      });
    } catch {
      /* backend not reachable — skip */
    }
  };

  const browser = await chromium.launch();
  try {
    console.log("  · recording light theme");
    resetHome();
    await resetPlugins();
    const light = await recordTheme(browser, url, "light", outDir, id);
    console.log("  · recording dark theme");
    resetHome();
    await resetPlugins();
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
