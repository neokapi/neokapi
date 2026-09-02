/**
 * record-desktop.ts — record a Kapi Desktop walkthrough screencast.
 *
 * Drives the REAL Kapi Desktop UI (apps/kapi-desktop/frontend `demo.html`,
 * which mounts the genuine IconSidebar + TermsBrowser + MemoryBrowser with
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
import { spawn, execFileSync } from "node:child_process";
import { chromium, type Page, type Browser, type Locator } from "playwright";
import { ensureDir, publicDemoDir, REPO_ROOT } from "../lib/paths.ts";
import { injectCursor, moveTo, humanClick, humanType, idle } from "./cursor-helper.ts";
import { loadEnv } from "../lib/env.ts";

// Load harness/.env (the seed writes BOWRAIN_SESSION_TOKEN etc. there) BEFORE
// the module-level BOWRAIN_* consts below read process.env. loadEnv() is
// idempotent, so run.ts's own call later is a no-op. Without this, importing
// this module during run.ts's import phase would capture an empty token.
loadEnv();

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
// The two review-queue rows the separation-of-duties beats need: one target Bob
// wrote (Alice may approve it) and one Alice wrote (the server refuses her).
// Both are printed by harness/scripts/seed-collaboration.mjs.
const BOWRAIN_PEER_BLOCK_ID = process.env.BOWRAIN_PEER_BLOCK_ID || "";
const BOWRAIN_SELF_BLOCK_ID = process.env.BOWRAIN_SELF_BLOCK_ID || "";

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
        `${BOWRAIN_BASE}/${workspace}/p/${projectId}/s/main/translate/${itemId}`,
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
    // is just what the demo installs). Named explicitly rather than relying on
    // the KAPI_CONFIG_DIR-derived default, matching the CLI/harness contract
    // (see kapiIsolationEnv in lib/paths.ts).
    KAPI_PLUGINS_DIR: path.join(ISO_DIR, "plugins"),
    KAPI_PLUGINS_DIR_ONLY: "1",
    // Never bind the repo's dogfood kapi.yaml via the upward project walk: these
    // backend processes run from an in-repo cwd, and desktop projects are opened
    // explicitly, so discovery must stay off.
    KAPI_NO_PROJECT: "1",
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
  // --force re-optimizes deps on every recording, ignoring node_modules/.vite/deps.
  // A prebundle cached by an older toolchain raises "__name is not defined" when
  // the app mounts, and a recording carries on over a page that failed to render:
  // the walk times out on its first selector, or films an empty frame. The
  // bowrain dev server below already forces it for exactly this; both need it.
  const vite = spawn("vp", ["dev", "--force"], { cwd: FRONTEND_DIR, env: { ...process.env, FORCE_COLOR: "0" }, stdio: "ignore" });
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
  // --force re-optimizes deps on every recording, ignoring node_modules/.vite/deps.
  // A stale prebundle cached by an older toolchain (pre-rolldown-1.0.3, before the
  // keepNames `__name` helper-injection fix) re-triggers "__name is not defined"
  // the moment the recharts dashboard mounts — blanking the page. Forcing a fresh
  // prebundle with the current rolldown is the operational guard.
  const vite = spawn("vp", ["dev", "--force", "--port", String(BW_VITE_PORT)], {
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
      // A short timeout, because an absent selector here is ordinary: a beat
      // names every element it might frame and takes the union of the ones that
      // rendered. On Playwright's 30 s default one optional selector stalls the
      // recording between two beats, and the screencast carries the dead air.
      const box = await page.locator(s).first().boundingBox({ timeout: 2000 }).catch(() => null);
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

/** A Context hub section tab by its label. The hub's own nav carries the
 *  sections (Explorer · Voice · Terms · Content Memory) as plain buttons. */
function contextSection(page: Page, label: string): Locator {
  return page.locator(`nav[aria-label="Context sections"] button:has-text("${label}")`);
}

/** Terms + content memory, read at the project that agreed them.
 *
 *  The app opens in projects mode (backend.GetAppMode defaults to "projects"
 *  and no UI switches it), so the two stores are sections of the Context
 *  pillar rather than rail items, and each shows the OPEN PROJECT's store:
 *  TermsPage/MemoriesPage auto-select the project handle and skip the picker
 *  entirely (TermsPage.tsx `activeHandle = projectHandle || handle`). The walk
 *  therefore opens KapiMart first and reads its terms and its memory. */
async function explorerWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  await beat("intro", null, async () => {
    await landOnHome(page);
    await idle(page, 2200);
  });
  await beat("open-termbases", null, async () => {
    await openSample(page, "sample-kapimart", "Context");
    await humanClick(page, sidebar("Context"));
    await page.waitForTimeout(800);
    await humanClick(page, contextSection(page, "Terms"));
    await page.waitForSelector('[data-testid^="concept-"]', { timeout: 30_000 });
    await page.waitForTimeout(900);
  });
  await beatEls("open-glossary", ['[data-testid^="concept-"]'], async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.45, 700);
    await page.waitForTimeout(2000);
  });
  await beatEls("inspect-concept", ['[data-testid^="concept-"]'], async () => {
    const first = page.locator('[data-testid^="concept-"]').first();
    await first.scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(300);
    await cursorTo('[data-testid^="concept-"]');
    await page.waitForTimeout(2200);
  });
  await beatEls("search-term", ['[data-testid="filterbar-search"]', '[data-testid^="concept-"]'], async () => {
    await humanType(page, page.getByTestId("filterbar-search"), "cart", { submit: true });
    await page.waitForTimeout(1600);
  });
  await beat("open-tm", null, async () => {
    await humanClick(page, contextSection(page, "Content Memory"));
    await page.waitForSelector('[data-testid^="tm-entry-"]', { timeout: 30_000 });
    await page.waitForTimeout(1100);
  });
  await beatEls("inspect-tm", ['[data-testid^="tm-entry-"]'], async () => {
    const first = page.locator('[data-testid^="tm-entry-"]').first();
    await first.scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(300);
    await cursorTo('[data-testid^="tm-entry-"]');
    await page.waitForTimeout(2300);
  });
  await beatEls("entity", ['[data-testid^="tm-entry-"]'], async () => {
    const second = page.locator('[data-testid^="tm-entry-"]').nth(1);
    if (await second.count()) {
      await second.scrollIntoViewIfNeeded().catch(() => {});
      await page.waitForTimeout(300);
      const box = await second.boundingBox().catch(() => null);
      if (box) await moveTo(page, box.x + box.width / 2, box.y + box.height / 2, 600);
    }
    await page.waitForTimeout(2200);
  });
  await beatEls("search-tm", ['[data-testid="tm-search"]', '[data-testid^="tm-entry-"]'], async () => {
    await page.getByTestId("tm-search").scrollIntoViewIfNeeded().catch(() => {});
    await humanType(page, page.getByTestId("tm-search"), "checkout", { submit: true });
    await page.waitForTimeout(1600);
  });
}

/** Create and manage a project. */
async function projectsWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  await beat("intro", null, async () => {
    // The light pass created a project and left it open; New Project is on the
    // home screen, so the dark pass has to get back there first.
    await landOnHome(page);
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
  // Back to the project home, where the collections live. Content is not a
  // rail item: the project home carries the standing, the point map and the
  // collections in one surface (IconSidebar `Project` → view `project-home`).
  await beat("content", { x: 0.02, y: 0.05, w: 0.92, h: 0.6 }, async () => {
    await humanClick(page, sidebar("Project"));
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
  // AI Models tab (seeded demo providers, no real keychain entries).
  await beat("credentials", { x: 0.02, y: 0.06, w: 0.96, h: 0.46 }, async () => {
    await humanClick(page, tab("AI Models"));
    await page.waitForTimeout(1500);
  });
  // Plugins tab.
  await beat("plugins", { x: 0.02, y: 0.06, w: 0.96, h: 0.6 }, async () => {
    await humanClick(page, tab("Plugins"));
    await page.waitForTimeout(1600);
  });
}

/**
 * Land on the app home, whichever state the app restored.
 *
 * Both themes of a demo record against one backend, and the app persists its
 * session: the second pass reopens the project the first one opened
 * (useTabManager restores lastOpenProjects on start) and lands on that
 * project, not on the home screen. `Home` is the one rail item that stays
 * enabled with no project open, so it reaches the sample card from either
 * state.
 */
async function landOnHome(page: Page): Promise<void> {
  const home = page.locator('button[aria-label="Home"]');
  if (await home.count()) {
    await humanClick(page, home).catch(() => {});
    await page.waitForTimeout(700);
  }
  await page.waitForSelector('[data-testid="sample-kapimart"]', { timeout: 20_000 });
}

/** Open the KapiMart sample project from the home screen. Idempotent across
 *  theme passes: the scaffold is re-created under the isolated home, and a
 *  project the previous pass left open is reached through the home screen. */
async function openSample(page: Page, testid: string, readyLabel: string): Promise<void> {
  await landOnHome(page);
  await humanClick(page, page.getByTestId(testid));
  // Wait until the project has opened and its plugins resolve (the gated sidebar
  // item becomes enabled), so subsequent clicks land on a ready project.
  await page.waitForSelector(`button[aria-label="${readyLabel}"]:not([disabled])`, { timeout: 60_000 });
  await page.waitForTimeout(1200);
}

/**
 * Expand one collection on the project home and open one of its files in the
 * preview sheet.
 *
 * The matched-file table nests: a shared output pattern renders as a
 * `matched-pattern-row` that opens to the source file and one row per locale
 * (CollectionsPanel.tsx). A file with no shared pattern is a `matched-source-row`
 * directly. Try the source row first, then open the pattern that carries the
 * name and try again, so both shapes reach the same sheet.
 */
async function openCollectionFile(page: Page, collection: string, filename: string): Promise<void> {
  const stem = filename.replace(/\.[^.]+$/, "");
  const expand = page.locator('button[aria-label="Expand"]').filter({ hasText: collection }).first();
  if (await expand.count()) {
    await humanClick(page, expand);
    await page.waitForTimeout(900);
  }
  const sourceRow = page
    .locator('tr[data-slot="matched-source-row"]')
    .filter({ hasText: filename })
    .first();
  if (!(await sourceRow.count())) {
    const patternRow = page
      .locator('tr[data-slot="matched-pattern-row"]')
      .filter({ hasText: stem })
      .first();
    if (await patternRow.count()) {
      await humanClick(page, patternRow);
      await page.waitForTimeout(700);
    }
  }
  await sourceRow.scrollIntoViewIfNeeded().catch(() => {});
  await humanClick(page, sourceRow);
  await page.waitForSelector('[data-preview="keyed-table"], [data-preview="data"]', { timeout: 30_000 });
  await page.waitForTimeout(1200);
}

/** Close the file preview sheet and settle back on the collections. */
async function closePreview(page: Page): Promise<void> {
  await page.keyboard.press("Escape").catch(() => {});
  await page.waitForTimeout(800);
}

/**
 * A project's content: the collections that group it, and what one file holds.
 *
 * Content stopped being a rail item when the project home merged (#2273), so
 * the collections ARE the front door: the standing and the point map sit above
 * them and every collection opens in place. Opening a file from a collection
 * raises the preview sheet, which reads a keyed format (JSON, YAML,
 * .properties) as a table of keys beside their values, with the file itself one
 * click away.
 */
async function contentWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  await beat("intro", null, async () => {
    await landOnHome(page);
    await idle(page, 2000);
  });
  // Open the KapiMart sample → its project home.
  await beat("open-project", null, async () => {
    await openSample(page, "sample-kapimart", "Project");
  });
  // The standing block: where the content sits, and what governs it there.
  await beatEls("overview", ['[data-testid="project-standing"]'], async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.24, 700);
    await page.waitForTimeout(2200);
  });
  // The collections, on the same surface.
  await beat("content", { x: 0.02, y: 0.3, w: 0.96, h: 0.64 }, async () => {
    await humanClick(page, sidebar("Project"));
    await page.waitForTimeout(1400);
    const panel = page.locator('[data-testid="collection-file-count"]').first();
    await panel.scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(1600);
  });
  // Open one collection: its patterns, and the files they match.
  await beatEls("patterns", ['[data-slot="matched-files-scroll"]'], async () => {
    const expand = page.locator('button[aria-label="Expand"]').filter({ hasText: "Online Store" }).first();
    if (await expand.count()) await humanClick(page, expand);
    await page.waitForTimeout(1600);
    await moveTo(page, WIDTH * 0.4, HEIGHT * 0.6, 700);
    await page.waitForTimeout(2000);
  });
  // Open a JSON catalog → the preview sheet reads it as keys and values.
  await beatEls("files", ['[data-preview="keyed-table"]'], async () => {
    await openCollectionFile(page, "Online Store", "store-ui.json");
    await page.waitForTimeout(1800);
  });
  // Hold on the Key column beside the text it names.
  await beatEls("keys", ['[data-preview="keyed-table"]'], async () => {
    await cursorTo('[data-preview="keyed-table"] tr[data-key-path]');
    await page.waitForTimeout(2400);
  });
  // The same file as it is written on disk.
  await beatEls("code", ['[data-preview="data"]'], async () => {
    const file = page.locator('[data-preview="data"] button:has-text("File")').first();
    if (await file.count()) await humanClick(page, file);
    await page.waitForTimeout(2400);
  });
  // A different keyed format, read the same way. These are the message strings
  // the review walk decides on, named by the same keys.
  await beatEls("properties", ['[data-preview="keyed-table"]'], async () => {
    await closePreview(page);
    await openCollectionFile(page, "Online Store", "error-messages.properties");
    await page.waitForTimeout(2400);
  });
}

/**
 * The Toolbox: what a project runs over its content.
 *
 * Flows stopped being a rail pillar; the Toolbox pillar hosts Tools and Flows
 * as tabs (IconSidebar `Toolbox` → view `tools`, ToolboxPage's two buttons), and
 * a flow card opens the pipeline in the visual editor.
 */
async function flowsWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, sidebar } = c;
  await beat("intro", null, async () => {
    await landOnHome(page);
    await idle(page, 2000);
  });
  // Open the KapiMart sample project.
  await beat("open-project", null, async () => {
    await openSample(page, "sample-kapimart", "Toolbox");
  });
  // Open the Toolbox, then its Flows tab.
  await beat("library", null, async () => {
    await humanClick(page, sidebar("Toolbox"));
    await page.waitForTimeout(1200);
    const flows = page.locator('button:has-text("Flows")').first();
    if (await flows.count()) await humanClick(page, flows);
    await page.waitForTimeout(1600);
  });
  // The project's flows: translate, translate-and-qa, pseudo-translate.
  await beat("library-zoom", { x: 0.02, y: 0.08, w: 0.96, h: 0.62 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.36, 700);
    await page.waitForTimeout(2200);
  });
  // Open a flow → its pipeline graph (AI translate, then a quality check).
  await beat("open-flow", null, async () => {
    await humanClick(page, page.getByText("translate-and-qa", { exact: true }));
    await page.waitForSelector(".react-flow", { timeout: 30_000 });
    await page.waitForTimeout(1500);
  });
  // Zoom the pipeline canvas.
  await beatEls("pipeline", [".react-flow"], async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.5, 700);
    await page.waitForTimeout(2400);
  });
}

/**
 * The Review queue and the five layers behind one decision (S-07).
 *
 * KapiMart opens with a queue rather than an empty page: its message catalogue
 * (`src/en/error-messages.properties`) ships translated into all five targets
 * with no decision row behind it, which
 * `backend/sample/embed_test.go TestScaffoldLeavesTheMessageCatalogueUnreviewed`
 * holds in place. Everything the queue reads is a committed file, so the walk
 * needs no server, no provider and no network.
 *
 * The Review page uses `data-slot` rather than `data-testid`, and its keyboard
 * handler ignores every key while a textarea has focus or the document sheet is
 * open (ReviewPage.tsx), so the walk keeps focus on the page body.
 */
async function reviewWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  const layer = async (id: string, slot: string) => {
    await beatEls(id, [`[data-slot="${slot}"]`], async () => {
      const card = page.locator(`[data-slot="${slot}"]`).first();
      if (await card.count()) {
        await card.scrollIntoViewIfNeeded().catch(() => {});
        await page.waitForTimeout(300);
        const toggle = page.locator(`[data-slot="${slot}-toggle"]`).first();
        if ((await toggle.count()) && (await card.getAttribute("data-open")) === null)
          await humanClick(page, toggle).catch(() => {});
        await cursorTo(`[data-slot="${slot}"]`);
      }
      await page.waitForTimeout(2300);
    });
  };

  await beat("intro", null, async () => {
    await landOnHome(page);
    await idle(page, 2000);
  });
  // Open KapiMart. Review is locale-gated: it appears because the project
  // declares five target languages.
  await beat("open-project", null, async () => {
    await openSample(page, "sample-kapimart", "Review");
  });
  // The queue: every language in one list, source wording first.
  await beat("queue", null, async () => {
    await humanClick(page, sidebar("Review"));
    await page.waitForSelector('[data-slot="review-queue-item"]', { timeout: 60_000 });
    await page.waitForTimeout(1600);
  });
  // The language selector carries a count per language, and the chips narrow
  // the list to what a check flagged.
  await beatEls("scope", ['[data-slot="review-language-select"]', '[data-slot="review-chips"]'], async () => {
    await cursorTo('[data-slot="review-language-select"]');
    await page.waitForTimeout(1500);
    await cursorTo('[data-slot="review-chips"]');
    await page.waitForTimeout(1800);
  });
  // Open one translated unit: source, target, and the layers behind it.
  await beatEls("open-unit", ['[data-slot="review-unit"]'], async () => {
    const target = page.locator('[data-slot="review-queue-item"]:not([data-source])').first();
    await humanClick(page, target);
    await page.waitForSelector('[data-slot="review-unit"]', { timeout: 20_000 });
    await page.waitForTimeout(1800);
  });
  await layer("point", "review-point");
  await layer("neighbourhood", "review-neighbourhood");
  await layer("history", "review-history");
  await layer("findings", "review-findings");
  await layer("provenance", "review-provenance");
  // Approve with the keyboard. The unit leaves the queue.
  await beatEls("approve", ['[data-slot="review-actions"]', '[data-slot="review-queue"]'], async () => {
    await cursorTo('[data-slot="review-approve"]');
    await page.waitForTimeout(900);
    await page.locator('[data-slot="review-page"]').first().click({ position: { x: 4, y: 4 } }).catch(() => {});
    await page.keyboard.press("a");
    await page.waitForTimeout(2400);
  });
  // What is left, and the batch that clears the units no check flagged.
  //
  // There is no source-lane beat, because the sample cannot produce one. A
  // source unit joins this queue only when it ranks below the project's source
  // gate, or when that gate is `approved` and the unit is not
  // (host/sourcereview.go computeSourceQueue). KapiMart declares no
  // defaults.source_gate, which resolves to `checked`, and its source settles
  // clean, so every row here is a translation.
  await beatEls("batch", ['[data-slot="review-batch"]', '[data-slot="review-queue"]'], async () => {
    await cursorTo('[data-slot="review-batch-approve"]');
    await page.waitForTimeout(2600);
  });
}

// ── Bowrain web walkthroughs ─────────────────────────────────────────────────
// These record the real bowrain web app (target: "web"); nav is via data-testid
// (the bowrain sidebar uses testids, not aria-labels).

/** Dismiss the web-only "Open in Desktop" banner if it's on screen. It renders
 *  at the top of every project view (ProjectView → OpenInDesktop) and would sit
 *  over the project header for the rest of a walk. bowrain-web-collaboration
 *  deliberately KEEPS it for its closing desktop-handoff beat. */
async function dismissOpenInDesktop(page: Page): Promise<void> {
  const dismiss = page.getByTestId("dismiss-open-in-desktop");
  if (await dismiss.count()) {
    await humanClick(page, dismiss).catch(() => {});
    await page.waitForTimeout(400);
  }
}

/** Bowrain web: shared content memory and terminology governed as concepts.
 *  Both are sections of the Context hub (nav-context → subnav-memory /
 *  subnav-concepts); the old standalone terms nav is gone. */
async function bowrainGovernanceWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls } = c;
  const tap = (id: string) => humanClick(page, page.getByTestId(id));
  await beat("intro", null, async () => {
    await idle(page, 2200);
  });
  // Open the workspace content memory. Content memory is no longer a rail
  // entry: the context restructure moved it under the Context hub, so the
  // route is nav-context → subnav-memory. `nav-memory` no longer exists and
  // tapping it recorded a dead click.
  await beat("open-memory", null, async () => {
    await tap("nav-context");
    await page.waitForTimeout(600);
    await tap("subnav-memory");
    await page.waitForSelector('[data-testid="tm-browser"]', { timeout: 20_000 }).catch(() => {});
    await page.waitForTimeout(1200);
  });
  await beat("tm-list", { x: 0.02, y: 0.1, w: 0.96, h: 0.82 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.42, 700);
    await page.waitForTimeout(2400);
  });
  // Search the memory.
  await beatEls("tm-search", ['[data-testid="tm-search"]'], async () => {
    const s = page.getByTestId("tm-search");
    if (await s.count()) await humanType(page, s, "mission", { submit: true });
    await page.waitForTimeout(1600);
  });
  // Move to Concepts. The memory beat above already entered the Context hub,
  // so this is a section switch within it — subnav-concepts, not nav-context,
  // which from inside the hub is at best a redirect back to the landing
  // section and at worst a no-op.
  await beat("open-concepts", null, async () => {
    await tap("subnav-concepts");
    await page
      .waitForSelector('input[aria-label="Search concepts"]', { timeout: 20_000 })
      .catch(() => {});
    await page.waitForTimeout(1500);
  });
  await beat("concept-list", { x: 0.02, y: 0.1, w: 0.96, h: 0.82 }, async () => {
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.42, 700);
    await page.waitForTimeout(2400);
  });
  // Open one concept — its story: the terms in every locale, with agreed status.
  // The ConceptList rows are plain buttons in the card list (no testids yet).
  await beat("concept-detail", { x: 0.02, y: 0.08, w: 0.96, h: 0.74 }, async () => {
    const row = page.locator("ul.divide-y li button").first();
    if (await row.count()) {
      await humanClick(page, row);
      await page.waitForTimeout(1600);
    }
    await moveTo(page, WIDTH * 0.4, HEIGHT * 0.36, 700);
    await page.waitForTimeout(2300);
  });
}

/**
 * Open the first (or named) project card, then its source content.
 *
 * A project card lands on the translation dashboard
 * (`p/$projectId/s/$stream` → TranslationDashboardRoute), so the file list is
 * one step further in: `subnav-source` reaches ProjectView, which is the only
 * place `open-file` renders.
 */
async function openProjectSource(page: Page, name?: string): Promise<void> {
  const named = name
    ? page.locator('[data-testid^="project-card"]', { hasText: name }).first()
    : null;
  const card = named && (await named.count()) ? named : page.locator('[data-testid^="project-card"]').first();
  await humanClick(page, card);
  await page.waitForTimeout(1600);
  await dismissOpenInDesktop(page);
  const source = page.getByTestId("subnav-source");
  await source.waitFor({ timeout: 20_000 }).catch(() => {});
  if (await source.count()) await humanClick(page, source);
  await page.waitForSelector('[data-testid^="open-file"]', { timeout: 20_000 }).catch(() => {});
  await page.waitForTimeout(1200);
}

/** Web translation editor: the visual editing card over a live preview, the
 *  shared content memory and terminology beside it, and per-locale switching,
 *  on files a team synced from kapi into the workspace. */
async function bowrainEditorWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo } = c;
  await beat("intro", null, async () => {
    await idle(page, 2000);
  });
  // Open the Company Website project (rich HTML content, fr/de/ja targets),
  // then its source content.
  await beat("open-project", null, async () => {
    await openProjectSource(page, "Company Website");
  });
  // Open a file → the editor.
  await beat("open-file", null, async () => {
    await humanClick(page, page.locator('[data-testid^="open-file"]').first());
    await page.waitForTimeout(2400);
  });
  // Switch to the Visual view: an inline editing card over a live document preview.
  await beat("split", { x: 0.03, y: 0.16, w: 0.74, h: 0.5 }, async () => {
    const sh = page.getByTestId("view-visual");
    if (await sh.count()) await humanClick(page, sh);
    await page.waitForTimeout(1600);
    await moveTo(page, WIDTH * 0.42, HEIGHT * 0.42, 700);
    await page.waitForTimeout(1200);
  });
  // Spotlight the shared content memory + terminology context, surfaced inline as you edit:
  // the visual card's "content memory Matches" expander (tm-toggle) opens the context panel,
  // and term matches dock in the term sidebar on the right.
  await beat("context", { x: 0.43, y: 0.16, w: 0.56, h: 0.62 }, async () => {
    const tm = page.getByTestId("tm-toggle");
    if ((await tm.count()) && !(await page.getByTestId("context-panel").isVisible().catch(() => false)))
      await humanClick(page, tm);
    await page.waitForTimeout(900);
    if (await page.getByTestId("context-panel").isVisible().catch(() => false))
      await cursorTo('[data-testid="context-panel"]');
    else if (await page.getByTestId("term-sidebar").count())
      await cursorTo('[data-testid="term-sidebar"]');
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

/**
 * Review and approve on the platform, with two genuine users.
 *
 * The surface is the project review session (`p/{id}/s/{stream}/review`,
 * ReviewSessionRoute), reached from the workspace review inbox. One queue, one
 * language selector with the source marked, and the FocusedReviewer holding the
 * verdict, the context rail and the decision.
 *
 * The closing beats are the workspace's separation-of-duties policy, as the
 * server actually applies it. `harness/scripts/seed-collaboration.mjs` sets the
 * policy to `block`, grants Bob the reviewer role on the target locale and has
 * Bob write one target. So Alice approves what Bob wrote, the server refuses her
 * on what she wrote herself (`reviewSoD.vet` → 403, rendered inline by
 * ReviewSession's ErrorNotice), and Bob decides that one off camera. Nothing is
 * staged in the frontend: every refusal and every approval is a real API call
 * by a real user.
 */
async function bowrainReviewWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, peer } = c;
  const startUrl = new URL(page.url());
  const wsBase = `${startUrl.origin}${startUrl.pathname}`.replace(/\/+$/, "");
  const themeParam = startUrl.searchParams.get("theme");
  const themeQ = themeParam ? `?theme=${themeParam}` : "";
  const slug = startUrl.pathname.replace(/^\/+|\/+$/g, "").split("/")[0] || "";
  const reviewUrl = BOWRAIN_PROJECT_ID ? `${wsBase}/p/${BOWRAIN_PROJECT_ID}/s/main/review${themeQ}` : "";
  const row = (blockId: string) =>
    page.locator(
      `[data-testid="queue-row-${BOWRAIN_ITEM_ID}::${blockId}::${BOWRAIN_COLLAB_LOCALE}"]`,
    );

  // The workspace review inbox: every project awaiting a decision, in one place.
  await beat("intro", null, async () => {
    await page.goto(`${wsBase}/review-inbox${themeQ}`, { waitUntil: "domcontentloaded" });
    await injectCursor(page); // goto wiped the page-injected cursor
    await page.waitForSelector('[data-testid="review-inbox"]', { timeout: 30_000 }).catch(() => {});
    await idle(page, 2400);
  });
  // Into one project's review session.
  await beat("open-project", null, async () => {
    const projectRow = page.locator('[data-testid^="review-inbox-project-"]').first();
    if (await projectRow.count()) {
      await humanClick(page, projectRow);
    } else if (reviewUrl) {
      await page.goto(reviewUrl, { waitUntil: "domcontentloaded" });
      await injectCursor(page);
    }
    await page.waitForSelector('[data-testid="review-session"]', { timeout: 30_000 });
    await page.waitForTimeout(2000);
  });
  // The queue, and the language it is scoped to. The selector marks the source.
  await beatEls("scope", ['[data-testid="filter-language"]', '[data-testid="review-filters"]'], async () => {
    await cursorTo('[data-testid="filter-language"]');
    await humanClick(page, page.getByTestId("filter-language"));
    await page.waitForTimeout(1400);
    await page.keyboard.press("Escape").catch(() => {});
    await page.waitForTimeout(1200);
  });
  // One unit in focus: its verdict against every bar the server applies.
  await beatEls("focus", ['[data-testid="focused-reviewer"]'], async () => {
    const bobRow = BOWRAIN_PEER_BLOCK_ID ? row(BOWRAIN_PEER_BLOCK_ID) : null;
    if (bobRow && (await bobRow.count())) await humanClick(page, bobRow);
    await page.waitForSelector('[data-testid="focused-reviewer"]', { timeout: 20_000 });
    await page.waitForTimeout(1200);
    await cursorTo('[data-testid="reviewer-verdict-passing"], [data-testid="reviewer-verdict-failing"]');
    await page.waitForTimeout(2000);
  });
  // The context rail: what governs this point, the blocks around it, the
  // approved wording already on record, and who decided what before.
  await beatEls("context", ['[data-testid="reviewer-context-rail"]'], async () => {
    const rail = page.getByTestId("reviewer-context-rail");
    if (await rail.count()) await rail.scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(500);
    await cursorTo('[data-testid="review-point"]');
    await page.waitForTimeout(1600);
    await cursorTo('[data-testid="reviewer-memory"]');
    await page.waitForTimeout(1600);
    await cursorTo('[data-testid="review-provenance"]');
    await page.waitForTimeout(1800);
  });
  // Alice approves the translation Bob wrote. A second pair of eyes.
  await beatEls("approve", ['[data-testid="reviewer-approve"]', '[data-testid="review-pending-count"]'], async () => {
    await cursorTo('[data-testid="reviewer-approve"]');
    await humanClick(page, page.getByTestId("reviewer-approve"));
    await page.waitForTimeout(2600);
  });
  // Her own translation is a different matter. The workspace policy refuses it,
  // and the server's own sentence lands on screen.
  await beatEls("duties", ['[data-testid="error-notice"]', '[data-testid="reviewer-approve"]'], async () => {
    const own = BOWRAIN_SELF_BLOCK_ID ? row(BOWRAIN_SELF_BLOCK_ID) : null;
    if (own && (await own.count())) {
      await humanClick(page, own);
      await page.waitForTimeout(1400);
    }
    await humanClick(page, page.getByTestId("reviewer-approve"));
    // The narration says the workspace refused this approval, so a take where it
    // did not is a broken take, not a quieter one. Fail the capture rather than
    // film a screen that contradicts what is said over it. A run with no seeded
    // block ids is a single-user rehearsal and makes no such claim.
    if (BOWRAIN_SELF_BLOCK_ID) {
      await page.waitForSelector('[data-testid="error-notice"]', { timeout: 20_000 });
    } else {
      await page.waitForSelector('[data-testid="error-notice"]', { timeout: 20_000 }).catch(() => {});
    }
    await page.waitForTimeout(1000);
    await cursorTo('[data-testid="error-notice"]');
    await page.waitForTimeout(2400);
  });
  // Bob decides it instead, in his own session, and Alice's queue catches up.
  await beatEls("second-reviewer", ['[data-testid="review-queue"]', '[data-testid="review-pending-count"]'], async () => {
    if (peer && BOWRAIN_PROJECT_ID && BOWRAIN_SELF_BLOCK_ID) {
      await peer.act(async (bp) => {
        await bp.goto(`${BOWRAIN_BASE}/${slug}/p/${BOWRAIN_PROJECT_ID}/s/main/review`, {
          waitUntil: "domcontentloaded",
        });
        await bp.waitForSelector('[data-testid="review-session"]', { timeout: 30_000 }).catch(() => {});
        const target = bp.locator(
          `[data-testid="queue-row-${BOWRAIN_ITEM_ID}::${BOWRAIN_SELF_BLOCK_ID}::${BOWRAIN_COLLAB_LOCALE}"]`,
        );
        if (await target.count()) await target.click().catch(() => {});
        await bp.waitForTimeout(1200);
        const approve = bp.getByTestId("reviewer-approve");
        if ((await approve.count()) && (await approve.isEnabled().catch(() => false)))
          await approve.click().catch(() => {});
        await bp.waitForTimeout(1500);
      });
    }
    await page.reload({ waitUntil: "domcontentloaded" });
    await injectCursor(page);
    await page.waitForSelector('[data-testid="review-session"]', { timeout: 30_000 }).catch(() => {});
    await page.waitForTimeout(1600);
    await cursorTo('[data-testid="review-pending-count"]');
    await page.waitForTimeout(2400);
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
        `${wsBase}/p/${projectId}/s/main/translate/${itemId}${themeQ}`,
        { waitUntil: "domcontentloaded" },
      );
      await injectCursor(page); // goto wiped the page-injected cursor; re-add it
      await page
        .waitForSelector('[data-testid="view-switcher"], [data-testid="block-grid"]', { timeout: 30_000 })
        .catch(() => {});
    } else {
      // No seed → land Alice on the first project's file via the dashboard.
      await openProjectSource(page);
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
  // should.
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

  // Closing hand-off: the same workspace opens natively. Land back on the
  // source view, where the web-only "Open in Bowrain Desktop" banner renders
  // (this walk deliberately never dismisses it — the banner IS the beat).
  await beatEls("desktop-handoff", ['[data-testid="open-in-desktop-banner"]'], async () => {
    if (projectId) {
      await page.goto(`${wsBase}/p/${projectId}/s/main/source${themeQ}`, {
        waitUntil: "domcontentloaded",
      });
      await injectCursor(page); // re-add cursor after navigation
    } else {
      // No seed — reach the first project via the dashboard, then its source
      // view, which is where the banner lives.
      await page.goto(`${wsBase}${themeQ}`, { waitUntil: "domcontentloaded" });
      await injectCursor(page);
      await page.waitForSelector('[data-testid^="project-card"]', { timeout: 15_000 }).catch(() => {});
      const card = page.locator('[data-testid^="project-card"]').first();
      if (await card.count()) await humanClick(page, card);
      const sourceNav = page.locator('[data-testid="subnav-source"]');
      await sourceNav.waitFor({ timeout: 15_000 }).catch(() => {});
      if (await sourceNav.count()) await humanClick(page, sourceNav);
    }
    await page
      .waitForSelector('[data-testid="open-in-desktop-banner"]', { timeout: 15_000 })
      .catch(() => {});
    await page.waitForTimeout(800);
    if (await page.getByTestId("open-in-desktop-banner").count())
      await cursorTo('[data-testid="open-in-desktop-banner"]');
    await page.waitForTimeout(1800);
  });
}

/** Bowrain Desktop: the native app connected to a team's bowrain-server,
 *  showing the same workspace — projects, languages, and file counts. The
 *  desktop mounts the SAME shared app (@neokapi/bowrain-app) the browser runs,
 *  so nav testids match the web: workspace-level nav-* rail, project-scoped
 *  subnav-* views (dashboard | automations | runs | connectors). */
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
  // Connectors are project-scoped in the unified app: open a project, then its
  // Connectors view. Open the add-connector dialog so the available connector
  // types are on screen (the empty list alone reads as a blank page).
  await beatEls("connectors", ['[data-testid="connector-form"]', '[role="dialog"]', '[data-testid="add-connector-btn"]'], async () => {
    const card = page.locator('[data-testid^="project-card"]').first();
    if (await card.count()) await humanClick(page, card);
    await page
      .waitForSelector('[data-testid="subnav-connectors"]', { timeout: 20_000 })
      .catch(() => {});
    await page.waitForTimeout(900);
    const n = page.getByTestId("subnav-connectors");
    if (await n.count()) await humanClick(page, n);
    await page.waitForTimeout(1400);
    const add = page.getByTestId("add-connector-btn");
    if (await add.count()) await humanClick(page, add);
    await page.waitForTimeout(1600);
  });
}

/** Bowrain Desktop: a project's automation rules and its server-side run
 *  history — the unified project views (dashboard | automations | runs |
 *  connectors) that replaced the decommissioned flows/FlowBuilder screens.
 *  Flow editing still exists, but as the Flows tab inside Automations. */
async function bowrainDesktopAutomationsWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo } = c;
  // Land on the workspace dashboard, then open the first project — the
  // project-scoped sidebar only exists inside a project.
  await beat("intro", null, async () => {
    await idle(page, 1800);
    const card = page.locator('[data-testid^="project-card"]').first();
    if (await card.count()) await humanClick(page, card);
    await page
      .waitForSelector('[data-testid="subnav-automations"]', { timeout: 20_000 })
      .catch(() => {});
    await page.waitForTimeout(1400);
  });
  // Automations → the Rules tab (the route lands on its Runs tab by default;
  // the tab strip is plain buttons — Runs · Rules · Flows — without testids).
  await beatEls("automations", ['h2:has-text("Automation Rules")', 'button:has-text("New Rule")'], async () => {
    const n = page.getByTestId("subnav-automations");
    if (await n.count()) await humanClick(page, n);
    await page.waitForTimeout(1400);
    const rules = page.locator('button:has-text("Rules")').first();
    if (await rules.count()) await humanClick(page, rules);
    await page.waitForTimeout(2000);
  });
  // Runs — the server-side loop history (ConvergenceRunsList): run states
  // ("Up to date" / "Parked"), triggers (Manual / kapi up / On push), the
  // per-locale summary, and the "Run now" button.
  await beatEls("runs", ['h2:has-text("Runs")', 'button:has-text("Run now")', "table"], async () => {
    const n = page.getByTestId("subnav-runs");
    if (await n.count()) await humanClick(page, n);
    await page.waitForTimeout(1800);
    if (await page.locator('button:has-text("Run now")').count())
      await cursorTo('button:has-text("Run now")');
    await page.waitForTimeout(2200);
  });
}

/** Bowrain web: the correction-learning loop. Candidate rules drawn from a
 *  team's corrections, a blast-radius preview, and promotion into a versioned
 *  check. The voice profile's review route is /:ws/context/voice/review/:profileId
 *  (routes/index.tsx `context` → `voice` → `review/$profileId`); the workspace
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
    await page.goto(`${wsBase}/context/voice/review/${profileId}${themeQ}`, { waitUntil: "domcontentloaded" });
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
  "kapi-desktop-review": reviewWalk,
  "bowrain-web-governance": bowrainGovernanceWalk,
  "bowrain-web-editor": bowrainEditorWalk,
  "bowrain-web-review": bowrainReviewWalk,
  "bowrain-web-collaboration": bowrainCollaborationWalk,
  "bowrain-web-correction-loop": bowrainCorrectionLoopWalk,
  "bowrain-desktop-dashboard": bowrainDesktopWalk,
  "bowrain-desktop-automations": bowrainDesktopAutomationsWalk,
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
  uiLocale?: string,
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
  // recording mid-run (toggle is idempotent → no loop). For the bowrain WEB
  // capture, also mark the root with `pw-recording-tl` so the shared sidebar
  // reserves the macOS traffic-light safe area (the desktop app sets its own
  // `bw-desktop-mac`); the framed DesktopScene paints the dots over that gutter.
  await context.addInitScript(({ isDark, isWeb }) => {
    const pin = () => {
      document.documentElement.classList.toggle("dark", isDark);
      if (isWeb) document.documentElement.classList.add("pw-recording-tl");
    };
    pin();
    document.addEventListener("DOMContentLoaded", pin);
    try {
      new MutationObserver(pin).observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
    } catch {
      /* observer unavailable — initial pin still applied */
    }
  }, { isDark: theme === "dark", isWeb: !!web });
  const t0 = Date.now();
  const page = await context.newPage();
  // Debug: surface the browser console (HARNESS_DEBUG=1) + uncaught errors
  // during capture — uncaught errors are always logged (they're a failure
  // signal); the full console stream is opt-in to keep normal records quiet.
  if (process.env.HARNESS_DEBUG) {
    page.on("console", (m) => console.log(`    [browser:${m.type()}] ${m.text()}`.slice(0, 300)));
  }
  page.on("pageerror", (e) =>
    console.log(
      `    [pageerror] ${(e as Error).message} :: ${((e as Error).stack || "").split("\n").slice(1, 4).join(" | ")}`.slice(0, 600),
    ),
  );
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
    // `&lang=` carries the UI locale of a localized recording pass (see the
    // uiLocale note in recordDesktop for what the app entry must do with it).
    const langQ = uiLocale ? `&lang=${encodeURIComponent(uiLocale)}` : "";
    await page.goto(`${url}?theme=${theme}${langQ}`, { waitUntil: "domcontentloaded" });
    // Default kapi-desktop renders an h1 immediately; bowrain-desktop renders its
    // dashboard only after the backend auto-connects to the server (a few
    // seconds), so callers pass a connected-state selector + we allow longer.
    try {
      await page.waitForSelector(ready ?? "h1", { timeout: ready ? 45_000 : 15_000 });
    } catch (e) {
      const shot = path.join(outDir, `_debug-${theme}.png`);
      await page.screenshot({ path: shot }).catch(() => {});
      const body = await page
        .evaluate(() => (document.body?.innerText ?? "(no body)").slice(0, 1500))
        .catch(() => "(eval failed)");
      console.log(`  [debug] ready-selector timed out; screenshot=${shot}\n  [debug] body:\n${body}`);
      throw e;
    }
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
  if (raw && fs.existsSync(raw)) reencodeDenseKeyframes(raw, webm);
  fs.rmSync(videoDir, { recursive: true, force: true });
  return { webm: path.basename(webm), beats };
}

/**
 * Playwright records the screencast as a VP8 webm with sparse keyframes, so when
 * Remotion's OffthreadVideo seeks to an arbitrary beat time it must decode a long
 * run of inter-frames — under render concurrency this intermittently blows past
 * the delayRender timeout and fails a frame. Re-encode to VP9 with a keyframe
 * every ~0.4s (-g 12) so any seek decodes a bounded, short GOP. High quality
 * (crf 18) since this is the source the final video is composited from.
 */
function reencodeDenseKeyframes(raw: string, webm: string): void {
  try {
    execFileSync(
      "ffmpeg",
      ["-y", "-i", raw, "-an", "-c:v", "libvpx-vp9", "-crf", "18", "-b:v", "0",
       "-g", "12", "-keyint_min", "12", "-deadline", "good", "-cpu-used", "3",
       "-row-mt", "1", "-pix_fmt", "yuv420p", webm],
      { stdio: "ignore" },
    );
  } catch {
    // ffmpeg unavailable or failed — fall back to the raw copy so capture still works.
    fs.copyFileSync(raw, webm);
  }
}

export interface RecordOptions {
  force?: boolean;
  /** Record the real bowrain WEB app (external running stack) instead of the
   *  kapi-desktop wbridge: cookie auth + workspace-slug navigation. */
  web?: boolean;
  /** Record the real Bowrain Desktop app via its own wbridge, auto-connected to
   *  a running bowrain-server (BOWRAIN_BACKEND_URL + BOWRAIN_SESSION_TOKEN). */
  bowrainDesktop?: boolean;
  /**
   * UI language for the recorded kapi-desktop app (default "en"; ignored for
   * the web/bowrain-desktop targets for now). How the app picks its locale
   * (@neokapi/i18n-react runtime):
   *
   *   - the production entry (src/main.tsx) reads the persisted backend
   *     setting via api.getUILanguage() (wbridge: GetUILanguage, stored as
   *     `ui_language` in KAPI_DESKTOP_CONFIG_DIR/settings) and boots with
   *     loadTranslations(lang, `/translations/<lang>.json`);
   *   - the compiled dictionaries live in frontend/public/translations/
   *     (currently only qps.json), produced by `vpx neokapi-i18n compile`.
   *
   * What this option does today (the recorder-side plumbing):
   *   1. persists the language on the recording backend via the wbridge
   *      (SetUILanguage), so the genuine app setting matches the pass, and
   *   2. appends `&lang=<locale>` to the recording URL next to `?theme=`.
   *
   * What a localized recording still NEEDS (owned by apps/kapi-desktop):
   *   - the recorder entry (src/demo/real-main.tsx) mounts App directly and
   *     skips main.tsx's translation bootstrap — it must honor `?lang=`
   *     (mirroring `?theme=`) by calling
   *     loadTranslations(lang, `/translations/<lang>.json`), and
   *   - a compiled catalog for the locale must exist, e.g.
   *     frontend/public/translations/nb.json (neokapi-i18n extract + compile).
   * Until both land, a localized pass records the English UI (the narration,
   * captions and published filenames are still fully localized).
   */
  uiLocale?: string;
}

/** Record the desktop walkthrough for demo <id> → public/<id>/screencast.json + webms. */
export async function recordDesktop(id: string, opts: RecordOptions = {}): Promise<Screencast> {
  const outDir = ensureDir(publicDemoDir(id));
  const jsonPath = path.join(outDir, "screencast.json");
  if (!opts.force && fs.existsSync(jsonPath) && fs.existsSync(path.join(outDir, "screencast-light.webm"))) {
    console.log(`  · screencast exists for ${id} (use --force to re-record)`);
    return JSON.parse(fs.readFileSync(jsonPath, "utf8"));
  }
  // UI language pass-through (kapi-desktop target only — see RecordOptions.uiLocale).
  const uiLocale = opts.uiLocale && opts.uiLocale !== "en" ? opts.uiLocale : undefined;
  if (uiLocale && (opts.web || opts.bowrainDesktop)) {
    console.warn(`  ! uiLocale=${uiLocale} is not wired for the ${opts.web ? "web" : "bowrain-desktop"} target — recording the default UI language`);
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
    // App-shell-loaded markers on the desktop dashboard/projects view. The
    // desktop mounts the shared @neokapi/bowrain-app, so these are the shared
    // ProjectDashboard and AppSidebar testids; scripts/check-walk-selectors.sh
    // keeps them honest.
    const ready =
      '[data-testid^="project-card"], [data-testid="empty-projects"], [data-testid="new-project-btn"], [data-testid="nav-translate"]';
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

  // Persist the UI language on the recording backend so the genuine setting
  // (Settings → General → UI language) matches the localized pass. Best-effort:
  // the visible effect also needs the app entry to honor `?lang=` — see
  // RecordOptions.uiLocale.
  if (uiLocale && !externalUrl) {
    try {
      await fetch("http://127.0.0.1:5175/wbridge", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ method: "SetUILanguage", args: [uiLocale] }),
      });
      console.log(`  · set backend UI language to ${uiLocale}`);
    } catch {
      console.warn(`  ! could not set UI language ${uiLocale} on the wbridge — recording default UI language`);
    }
  }

  // Reset created projects (isolated home) before each theme so state-mutating
  // walkthroughs (e.g. project creation) start clean on both passes. The seeded
  // termbases/Memories/providers under ISO_DIR are left intact.
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
    console.log(`  · recording light theme${uiLocale ? ` (ui ${uiLocale})` : ""}`);
    resetHome();
    await resetPlugins();
    const light = await recordTheme(browser, url, "light", outDir, id, undefined, undefined, uiLocale);
    console.log(`  · recording dark theme${uiLocale ? ` (ui ${uiLocale})` : ""}`);
    resetHome();
    await resetPlugins();
    const dark = await recordTheme(browser, url, "dark", outDir, id, undefined, undefined, uiLocale);

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
