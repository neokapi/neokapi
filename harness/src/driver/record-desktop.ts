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
import { injectCursor, moveTo, humanClick, humanType, idle, setClickSink } from "./cursor-helper.ts";
import { loadEnv } from "../lib/env.ts";

// Load harness/.env (the seed writes BOWRAIN_SESSION_TOKEN etc. there) BEFORE
// the module-level BOWRAIN_* consts below read process.env. loadEnv() is
// idempotent, so run.ts's own call later is a no-op. Without this, importing
// this module during run.ts's import phase would capture an empty token.
loadEnv();

// The recording is the composition's own frame size, so a full-window beat
// shows the app at 1:1 and a region beat crops into it rather than up-scaling
// a smaller capture.
const WIDTH = 1920;
const HEIGHT = 1080;

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
  /** The element the beat's highlight selector resolved to, when the manifest named one. */
  highlight?: ZoomRect | null;
}

/**
 * What demo.yaml asks of one beat, by beat id (see NarrationSpec): how long to
 * keep it on camera, which elements to crop to once its actions have settled,
 * and which element to draw a box around.
 */
export interface BeatSpec {
  hold?: number;
  cropSelectors?: string[];
  highlightSelector?: string;
}
export interface Screencast {
  width: number;
  height: number;
  video: Record<ThemeMode, string>;
  /** Beats recorded per theme (pacing is near-identical, but kept exact). */
  beats: Record<ThemeMode, Beat[]>;
  /** Seconds from the start of each recording at which the cursor clicked. */
  clicks?: Record<ThemeMode, number[]>;
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
// The shared file the two users co-occupy (written to harness/.env by
// scripts/seed-bowrain.ts). The Translate route addresses a file by its name
// (the route's trailing splat, routes/index.tsx `translate/$`); the review
// queue keys its rows by the item id.
const BOWRAIN_PROJECT_ID = process.env.BOWRAIN_PROJECT_ID || "";
const BOWRAIN_ITEM_ID = process.env.BOWRAIN_ITEM_ID || "";
const BOWRAIN_ITEM_NAME = process.env.BOWRAIN_ITEM_NAME || "";
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
    openTranslateFile: async (workspace, projectId, itemName, locale) => {
      // The editor route reads the target locale from the project's first target
      // language; `locale` is passed for parity with the collab room key and to
      // document which target both users sit on.
      void locale;
      await page.goto(
        `${BOWRAIN_BASE}/${workspace}/p/${projectId}/s/main/translate/${translatePath(itemName)}`,
        { waitUntil: "domcontentloaded" },
      );
      // Wait for the editor to mount so the peer's useCollaboration() opens the
      // WebSocket and publishes its awareness into the shared room.
      await page
        .waitForSelector('[data-testid="view-switcher"], [data-testid="view-table"], [data-testid="visual-editor-layout"]', {
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

// ── The desktop's link to its server, cut and restored on camera ─────────────
// The Bowrain Desktop walk shows the offline queue, and that queue only fills
// when the desktop's Go backend genuinely loses the server. So the recorder
// puts a TCP relay between the wbridge and bowrain-server and points the
// wbridge at the relay. Closing the relay destroys the live sockets and
// refuses new ones, so the backend's next REST call fails the way a dropped
// network fails, `goOffline` fires, edits queue, and the backend's own
// reconnect loop drains the queue when the relay returns. Nothing about the
// offline state is simulated inside the app.
interface ServerLink {
  /** The URL the wbridge is given: the relay, not the server. */
  url: string;
  /** Drop the link: destroy every live socket and stop accepting. */
  cut: () => Promise<void>;
  /** Bring the link back on the same port. */
  restore: () => Promise<void>;
  close: () => Promise<void>;
}

/** The relay for the Bowrain Desktop take, so a walk can cut the connection. */
let bowrainServerLink: ServerLink | null = null;

/** An unused loopback port, taken by binding zero and reading the assignment. */
function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const probe = net.createServer();
    probe.once("error", reject);
    probe.listen(0, "127.0.0.1", () => {
      const addr = probe.address();
      const port = typeof addr === "object" && addr ? addr.port : 0;
      probe.close(() => (port ? resolve(port) : reject(new Error("no free port"))));
    });
  });
}

async function startServerLink(target: string): Promise<ServerLink> {
  const t = new URL(target);
  const host = t.hostname;
  const port = Number(t.port || (t.protocol === "https:" ? 443 : 80));
  const relayPort = await freePort();
  const live = new Set<net.Socket>();
  let server: net.Server | null = null;

  const listen = () =>
    new Promise<void>((resolve, reject) => {
      const s = net.createServer((client) => {
        const upstream = net.connect({ port, host });
        live.add(client);
        live.add(upstream);
        const drop = () => {
          live.delete(client);
          live.delete(upstream);
        };
        client.on("close", drop);
        upstream.on("close", drop);
        client.on("error", () => upstream.destroy());
        upstream.on("error", () => client.destroy());
        client.pipe(upstream);
        upstream.pipe(client);
      });
      s.once("error", reject);
      s.listen(relayPort, "127.0.0.1", () => {
        server = s;
        resolve();
      });
    });

  const cut = async () => {
    const s = server;
    server = null;
    for (const sock of live) sock.destroy();
    live.clear();
    if (s) await new Promise<void>((r) => s.close(() => r()));
  };

  await listen();
  return {
    url: `http://127.0.0.1:${relayPort}`,
    cut,
    restore: async () => {
      if (!server) await listen();
    },
    close: cut,
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

  // Everything the desktop backend sends to the server goes through a relay the
  // recorder owns, so the offline walk can drop the connection for real.
  const link = await startServerLink(server);
  bowrainServerLink = link;

  console.log("  · building + starting bowrain wbridge (real backend over HTTP)");
  const bridgeBin = path.join(os.tmpdir(), "bowrain-wbridge-rec");
  await runToCompletion("go", ["build", "-tags", "fts5", "-o", bridgeBin, "./cmd/wbridge"], {
    cwd: BOWRAIN_DESKTOP_DIR,
    env: goEnv(),
  });
  const bridge = spawn(bridgeBin, [], {
    env: goEnv({
      BOWRAIN_DESKTOP_CONFIG_DIR: BW_ISO,
      BOWRAIN_SERVER_URL: link.url,
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
      await link.close();
      bowrainServerLink = null;
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
  /** Open the shared translate file, by name, as the peer (joins the same collab room). */
  openTranslateFile: (workspace: string, projectId: string, itemName: string, locale: string) => Promise<void>;
}

/** The Translate route's trailing splat for a file name: each segment is
 *  encoded, and a slash inside the name stays a path separator. */
function translatePath(itemName: string): string {
  return itemName.split("/").map(encodeURIComponent).join("/");
}

function makeCtx(page: Page, t0: number, beats: Beat[], peer: PeerSession | undefined, specs: Record<string, BeatSpec>, holds: Record<string, number>): WalkCtx {
  const now = () => (Date.now() - t0) / 1000;
  const sidebar = (label: string) => page.locator(`button[aria-label="${label}"]`);
  // Keep the beat on camera for its hold: the manifest's `hold`, else the
  // measured length of the narration over it when a narration.json exists.
  // The walk's own waits count toward it, so only the remainder is added.
  const holdUntil = async (id: string, tStart: number) => {
    const target = specs[id]?.hold ?? holds[id];
    if (!target) return;
    const remaining = target - (now() - tStart);
    if (remaining > 0) await page.waitForTimeout(Math.round(remaining * 1000));
  };
  const beat = async (id: string, zoom: ZoomRect | null, fn: () => Promise<void>) => {
    const tStart = now();
    await fn();
    await holdUntil(id, tStart);
    const spec = specs[id];
    const crop = spec?.cropSelectors ? await unionZoom(spec.cropSelectors) : zoom;
    const highlight = spec?.highlightSelector ? await unionZoom([spec.highlightSelector], 0.01) : undefined;
    beats.push({ id, tStart, tEnd: now(), zoom: crop, ...(highlight !== undefined ? { highlight } : {}) });
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
    await holdUntil(id, tStart);
    const spec = specs[id];
    const zoom = await unionZoom(spec?.cropSelectors ?? selectors);
    const highlight = spec?.highlightSelector ? await unionZoom([spec.highlightSelector], 0.01) : undefined;
    beats.push({ id, tStart, tEnd: now(), zoom, ...(highlight !== undefined ? { highlight } : {}) });
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
 *  therefore opens KapiMart first and reads its terms and its memory.
 *
 *  Both stores open on a search, because the question a reader has is "what did
 *  we agree for this word", not "what does this tab look like". The two search
 *  boxes behave differently and the walk drives each as it is: the concept
 *  search filters as you type (debounced, ConceptList.tsx), the memory search
 *  runs on Enter (MemorySearchBar.tsx says so in its own header). */
async function explorerWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  // Terms in a project is the concept workspace (ConceptsView over
  // @neokapi/concept-ui), which carries no testids: its search box is the one
  // labelled control, and each concept is a button in the divided list.
  const conceptSearch = 'input[aria-label="Search concepts"]';
  const conceptRow = "ul.divide-y li button";
  // Open the project's Terms and search it in one beat: the result of the
  // search is the beat, not the tab it happened on.
  // Getting to the store is not a beat, so it happens before one starts: a beat
  // carries one crop for its whole slice, and a crop chosen for the search would
  // otherwise be applied to the navigation under it.
  await landOnHome(page);
  await openSample(page, "sample-kapimart", "Context");
  await humanClick(page, sidebar("Context"));
  await page.waitForTimeout(800);
  await humanClick(page, contextSection(page, "Terms"));
  await page.waitForSelector(conceptSearch, { timeout: 30_000 });
  await page.waitForSelector(conceptRow, { timeout: 30_000 });
  await page.waitForTimeout(600);
  await beatEls("search-term", [conceptSearch, conceptRow], async () => {
    await humanType(page, page.locator(conceptSearch), "cart");
    // The list filters as you type, debounced; the wait is for the filtered
    // list, so the narration's "here it is" lands on the result and not on the
    // unfiltered list underneath it.
    await page.waitForTimeout(1800);
  });
  // Open the concept the search found: its definition, and its approved terms
  // per language with the status that marks the preferred one.
  await beatEls("concept", ['section:has(h3:has-text("Geography"))', conceptRow], async () => {
    await humanClick(page, page.locator(conceptRow).first());
    await page.waitForTimeout(2400);
  });
  // Content Memory, searched the same way. This box submits on Enter.
  await beatEls("search-tm", ['[data-testid="tm-search"]', '[data-testid^="tm-entry-"]'], async () => {
    await humanClick(page, contextSection(page, "Content Memory"));
    await page.waitForSelector('[data-testid="tm-search"]', { timeout: 30_000 });
    await page.waitForTimeout(800);
    await humanType(page, page.getByTestId("tm-search"), "checkout", { submit: true });
    await page.waitForSelector('[data-testid^="tm-entry-"]', { timeout: 30_000 });
    await page.waitForTimeout(1600);
  });
  // One entry, opened: the source string beside its translations, with inline
  // formatting kept as tags and names held as placeholders.
  await beatEls("inspect-tm", ['[data-testid^="tm-entry-"]'], async () => {
    const first = page.locator('[data-testid^="tm-entry-"]').first();
    await first.scrollIntoViewIfNeeded().catch(() => {});
    await page.waitForTimeout(300);
    await cursorTo('[data-testid^="tm-entry-"]');
    await humanClick(page, first).catch(() => {});
    await page.waitForTimeout(2400);
  });
}

/** Create a project, give it a language and a collection, and read the plan.
 *
 *  Every beat here acts. The one thing this walk cannot do is add files: the
 *  "Add Files" button opens a native picker (CollectionsPanel.handleAddFiles →
 *  api.addFilesDialog) and the drop path reads File.path, which Chromium does
 *  not set, so neither is drivable from Playwright. A collection is therefore
 *  declared the way the recipe declares it, by a name and a glob, and the plan
 *  is read on the sample project, which has content to plan over. */
async function projectsWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  await beat("intro", null, async () => {
    // The light pass created a project and left it open; New Project is on the
    // home screen, so the dark pass has to get back there first.
    await landOnHome(page);
    await idle(page, 2000);
  });
  // Name it, create it, take the input/output template, and land on the
  // project home. Nothing is governed yet, which is what the next beats change.
  await beat("new-project", null, async () => {
    await humanClick(page, page.locator('button:has-text("New Project")').first());
    await page.waitForTimeout(400);
    await humanType(page, page.locator('input[placeholder="My App"]'), "Acme Help Center");
    await page.waitForTimeout(600);
    await humanClick(page, page.locator('button:has-text("Create Project")'));
    await page.waitForTimeout(1200);
    await humanClick(page, page.locator('button:has-text("Input → Output")'));
    await page.waitForTimeout(1600);
  });
  // A target language, added and saved. MultiLocaleSelect adds on pick with no
  // confirm step (locale-select.tsx), and SaveBar renders only while the draft
  // is dirty, so the save click is guarded rather than assumed.
  await beatEls("add-language", ['input[placeholder="Add locale..."]', '[data-slot="combobox-item"]'], async () => {
    await humanClick(page, sidebar("Project Settings"));
    await page.waitForTimeout(1300);
    const add = page.locator('input[placeholder="Add locale..."]').first();
    await add.scrollIntoViewIfNeeded().catch(() => {});
    await humanType(page, add, "French");
    await page.waitForTimeout(800);
    const item = page.locator('[data-slot="combobox-item"]').first();
    if (await item.count()) await humanClick(page, item);
    await page.waitForTimeout(1000);
    const save = page.locator('button:has-text("Save Changes")').first();
    if (await save.count()) await humanClick(page, save);
    await page.waitForTimeout(1400);
  });
  // A collection, declared on the project home: a name and one glob. The
  // pattern field is a CodeMirror editor (GlobInput → CodeInput), so the text
  // goes in through the focused .cm-content rather than into an <input>, and
  // the row reports what the pattern matched as it is typed.
  await beatEls("add-collection", ['[data-testid="pattern-match-count"]', '[data-slot="code-input"]'], async () => {
    await humanClick(page, sidebar("Project"));
    await page.waitForTimeout(1300);
    await humanClick(page, page.locator('[aria-label="Add content collection"]').first());
    await page.waitForTimeout(900);
    const edit = page.locator('[aria-label="Edit collection"]').last();
    if (await edit.count()) await humanClick(page, edit);
    await page.waitForTimeout(900);
    const name = page.locator('input[placeholder="Collection name"]').last();
    if (await name.count()) {
      await name.fill("");
      await humanType(page, name, "Help Center");
      await page.waitForTimeout(400);
    }
    const pattern = page.locator('[data-slot="code-input"] .cm-content').last();
    if (await pattern.count()) {
      await pattern.click();
      await page.keyboard.type("docs/**/*.md", { delay: 70 });
    }
    await page.waitForTimeout(2000);
  });
  // The plan, read on the sample project, because a dry run of pending work
  // needs pending work. It opens a dialog over data fetched on page load: no
  // provider is called and nothing is written (ConvergenceHero.tsx).
  await page.keyboard.press("Escape").catch(() => {});
  await page.waitForTimeout(500);
  await landOnHome(page);
  await humanClick(page, page.getByTestId("sample-kapimart"));
  await page.waitForSelector('[data-slot="hero-plan"]', { timeout: 60_000 });
  await page.waitForTimeout(1200);
  await beatEls("plan", ['[data-slot="converge-plan-dialog"]'], async () => {
    await cursorTo('[data-slot="hero-plan"]');
    await humanClick(page, page.locator('[data-slot="hero-plan"]').first());
    await page.waitForSelector('[data-slot="converge-plan-dialog"]', { timeout: 30_000 });
    await page.waitForTimeout(2400);
  });
}

/** Configuration: appearance, where a provider key goes, plugins.
 *
 *  The credentials beat opens the Add Credentials dialog and closes it again
 *  without saving. Saving writes to the real OS keychain (backend/credentials.go
 *  → host.SaveCredential), so a recording must never press Save: what the beat
 *  shows is the form a key is typed into, which is the claim the narration
 *  makes. */
async function configWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  const tab = (label: string) => page.locator(`[role="tab"]:has-text("${label}")`);
  await beat("open-settings", { x: 0, y: 0.04, w: 0.34, h: 0.66 }, async () => {
    await idle(page, 1200);
    await humanClick(page, sidebar("App Settings"));
    await page.waitForTimeout(1200);
  });
  // General: appearance + UI language. Neither is clicked: the theme control
  // would flip the recording, and the language control would change the
  // language the walk is being recorded in.
  await beat("general", { x: 0.02, y: 0.06, w: 0.6, h: 0.66 }, async () => {
    await moveTo(page, WIDTH * 0.2, HEIGHT * 0.32, 700);
    await page.waitForTimeout(2200);
  });
  // AI Models: the providers a project can translate with, and the form a key
  // is entered into. Opened, held, then dismissed with Escape — never saved.
  await beatEls("credentials", ['input#cred-apikey', '[role="dialog"]'], async () => {
    await humanClick(page, tab("AI Models"));
    await page.waitForTimeout(1500);
    const add = page.locator('button:has-text("Add Credentials")').first();
    if (await add.count()) {
      await humanClick(page, add);
      await page.waitForSelector("input#cred-apikey", { timeout: 20_000 });
      await page.waitForTimeout(900);
      await cursorTo("input#cred-apikey");
    }
    await page.waitForTimeout(2000);
  });
  // Plugins.
  await beat("plugins", { x: 0.02, y: 0.06, w: 0.96, h: 0.6 }, async () => {
    await page.keyboard.press("Escape").catch(() => {});
    await page.waitForTimeout(700);
    await humanClick(page, tab("Plugins"));
    await page.waitForTimeout(1800);
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

/**
 * Open the KapiMart sample project, from either state a theme pass can start in.
 *
 * The two passes share one backend and the app restores its session, so the
 * second pass starts with the project already open. Clicking the sample card
 * in that state re-runs the scaffold and lands on the Get Started template
 * picker, which replaces the project home, and the walk films that instead.
 * A rail item is enabled exactly when a project tab is open, so read it to
 * tell the two states apart and go back through the project home button.
 */
async function openSample(page: Page, testid: string, readyLabel: string): Promise<void> {
  const ready = `button[aria-label="${readyLabel}"]:not([disabled])`;
  if (await page.locator(ready).count()) {
    const home = page.locator('button[aria-label="Project"]');
    if (await home.count()) await humanClick(page, home);
    await page.waitForTimeout(1400);
    return;
  }
  await landOnHome(page);
  await humanClick(page, page.getByTestId(testid));
  // Wait until the project has opened and its plugins resolve (the gated sidebar
  // item becomes enabled), so subsequent clicks land on a ready project.
  await page.waitForSelector(ready, { timeout: 60_000 });
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
  const expand = page.locator('button[aria-label="Expand"]').filter({ hasText: collection }).first();
  if (await expand.count()) {
    await humanClick(page, expand);
    await page.waitForTimeout(900);
  }
  const sourceRow = page
    .locator('tr[data-slot="matched-source-row"]')
    .filter({ hasText: filename })
    .first();
  // A pattern row is a disclosure, and which one hides this file is not
  // knowable from the file name: the row's cells carry the output pattern and
  // the glob, either of which may spell the stem differently. Open them in turn
  // until the source row is on the page, rather than guess once and then wait
  // out a timeout on a row that never appeared.
  if (!(await sourceRow.count())) {
    const patterns = page.locator('tr[data-slot="matched-pattern-row"]');
    const n = await patterns.count();
    for (let i = 0; i < n; i++) {
      await humanClick(page, patterns.nth(i));
      await page.waitForTimeout(500);
      if (await sourceRow.count()) break;
    }
  }
  await sourceRow.waitFor({ state: "visible", timeout: 15_000 });
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
 * A project's content: what a collection matched, and what one file holds.
 *
 * Content stopped being a rail item when the project home merged (#2273), so
 * the collections ARE the front door: the standing and the point map sit above
 * them and every collection opens in place. The walk extracts on camera,
 * because the coverage the narration describes does not exist until it has:
 * the sample ships its content unread, and the empty-state panel says so.
 * Opening a file from a collection raises the preview sheet, which reads a
 * keyed format (JSON, YAML, .properties) as a table of keys beside their
 * values, with the file itself one click away.
 */
async function contentWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  // Open the KapiMart sample → its project home, which opens on the standing:
  // four collections, nothing extracted, every language at zero.
  await beatEls("open-project", ['[data-testid="project-standing"]'], async () => {
    await landOnHome(page);
    await openSample(page, "sample-kapimart", "Project");
    await moveTo(page, WIDTH * 0.5, HEIGHT * 0.24, 700);
    await page.waitForTimeout(1800);
  });
  // Extract, on camera. The empty-state button is the one that is not behind
  // the Advanced disclosure (CollectionsPanel.tsx), and it is the only thing
  // that turns "nothing extracted yet" into counts.
  await beatEls("extract", ['[data-testid="collection-file-count"]', '[data-slot="ship-gate-cell"]'], async () => {
    const run = page.locator('button:has-text("Run extract")').first();
    if (await run.count()) {
      await cursorTo('button:has-text("Run extract")');
      await humanClick(page, run);
    }
    // Reading four collections takes a moment, and the beat is the counts
    // arriving, so the wait is for the gate cells rather than a fixed pause.
    await page.waitForSelector('[data-slot="ship-gate-cell"]', { timeout: 120_000 }).catch(() => {});
    await page.waitForTimeout(2600);
  });
  // Open one collection: its patterns, and the files they match.
  await beatEls("patterns", ['[data-slot="matched-files-scroll"]'], async () => {
    const expand = page.locator('button[aria-label="Expand"]').filter({ hasText: "Online Store" }).first();
    if (await expand.count()) await humanClick(page, expand);
    await page.waitForTimeout(1600);
    await moveTo(page, WIDTH * 0.4, HEIGHT * 0.6, 700);
    await page.waitForTimeout(2000);
  });
  // Open a JSON catalog → the preview sheet reads it as keys and values, and
  // the key column is the thing to look at.
  await beatEls("keys", ['[data-preview="keyed-table"]'], async () => {
    await openCollectionFile(page, "Online Store", "store-ui.json");
    await page.waitForTimeout(1200);
    await cursorTo('[data-preview="keyed-table"] tr[data-key-path]');
    await page.waitForTimeout(2400);
  });
  // The same file as it is written on disk, with the selected key's line
  // highlighted: structure on one tab, bytes on the other.
  await beatEls("code", ['[data-preview="data"]'], async () => {
    const file = page.locator('[data-preview="data"] button:has-text("File")').first();
    if (await file.count()) await humanClick(page, file);
    await page.waitForTimeout(2400);
  });
}

/**
 * The Toolbox: what a project runs over its content, and composing one.
 *
 * Flows stopped being a rail pillar; the Toolbox pillar hosts Tools and Flows
 * as tabs (IconSidebar `Toolbox` → view `tools`, ToolboxPage's two buttons), and
 * a flow card opens the pipeline in the shared linear editor.
 *
 * The walk edits a flow and makes it the project's default, both of which are
 * local. It does not press Run: the seeded providers carry names and no keys
 * (cmd/seed-demo), so an AI translate started here would fail on camera. The
 * runner is named in the narration instead.
 */
async function flowsWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo, sidebar } = c;
  // Open the KapiMart sample and go straight to its flows: the Tools tab is a
  // different subject and the video has one.
  await landOnHome(page);
  await openSample(page, "sample-kapimart", "Toolbox");
  await humanClick(page, sidebar("Toolbox"));
  await page.waitForTimeout(1200);
  const flowsTab = page.locator('button:has-text("Flows")').first();
  if (await flowsTab.count()) await humanClick(page, flowsTab);
  await page.waitForTimeout(1400);
  await beatEls("open-flow", ['[data-testid="linear-flow-editor"]'], async () => {
    await humanClick(page, page.getByText("translate-and-qa", { exact: true }));
    await page.waitForSelector('button[aria-label="Back to flow list"]', { timeout: 30_000 });
    await page.waitForSelector('[data-testid="linear-flow-editor"]', { timeout: 30_000 });
    await page.waitForSelector('[data-testid="step-row"]', { timeout: 30_000 });
    await page.waitForTimeout(2000);
  });
  // Add a step. The picker appends, so the new row is moved up to sit in front
  // of the translate it is meant to spare.
  await beatEls("add-step", ['[data-testid="linear-flow-editor"]'], async () => {
    await humanClick(page, page.getByTestId("add-step"));
    await page.waitForSelector('input[aria-label="Search tools"]', { timeout: 20_000 });
    await humanType(page, page.locator('input[aria-label="Search tools"]'), "recycle");
    await page.waitForTimeout(700);
    const tool = page.locator('[data-testid="add-step-tool"]').first();
    await humanClick(page, tool);
    await page.waitForTimeout(1200);
  });
  // Move it to the front: two clicks on Move up, and the step strip in the
  // header follows.
  await beatEls("reorder", ['[data-testid="linear-flow-editor"]'], async () => {
    const rows = page.locator('[data-testid="step-row"]');
    const n = await rows.count();
    for (let i = 0; i < Math.max(0, n - 1); i++) {
      const up = rows.last().locator('button[aria-label="Move up"]').first();
      if (!(await up.count())) break;
      await humanClick(page, up);
      await page.waitForTimeout(700);
    }
    await page.waitForTimeout(1600);
  });
  // Make it the project's default. The badge beside the name is the receipt.
  await beatEls("default", ['[data-testid="flow-default-badge"]', '[data-slot="switch"]'], async () => {
    const toggle = page.locator('[aria-label="Set as the project\'s default flow"]').first();
    if (await toggle.count()) {
      await cursorTo('[aria-label="Set as the project\'s default flow"]');
      await humanClick(page, toggle);
    }
    await page.waitForTimeout(2200);
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

  // The queue, and the two controls that narrow it, in one beat: every
  // language in one list, a count per language, and the chips for what a check
  // flagged.
  await landOnHome(page);
  await openSample(page, "sample-kapimart", "Review");
  await humanClick(page, sidebar("Review"));
  await page.waitForSelector('[data-slot="review-queue-item"]', { timeout: 60_000 });
  await page.waitForTimeout(1200);
  await beatEls("queue", ['[data-slot="review-queue"]', '[data-slot="review-language-select"]'], async () => {
    await cursorTo('[data-slot="review-language-select"]');
    await page.waitForTimeout(1200);
    await cursorTo('[data-slot="review-chips"]');
    await page.waitForTimeout(1600);
  });
  // Open one translated unit: source, target, and the layers behind them.
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
  // Decide: the keystroke on this unit, then the batch that clears the units no
  // check flagged.
  //
  // There is no source-lane beat, because the sample cannot produce one. A
  // source unit joins this queue only when it ranks below the project's source
  // gate, or when that gate is `approved` and the unit is not
  // (host/sourcereview.go computeSourceQueue). KapiMart declares no
  // defaults.source_gate, which resolves to `checked`, and its source settles
  // clean, so every row here is a translation.
  await beatEls("decide", ['[data-slot="review-batch"]', '[data-slot="review-queue"]'], async () => {
    await cursorTo('[data-slot="review-approve"]');
    await page.waitForTimeout(800);
    await page.locator('[data-slot="review-page"]').first().click({ position: { x: 4, y: 4 } }).catch(() => {});
    await page.keyboard.press("a");
    await page.waitForTimeout(2200);
    await cursorTo('[data-slot="review-batch-approve"]');
    await page.waitForTimeout(2400);
  });
}

// ── Bowrain web walkthroughs ─────────────────────────────────────────────────
// These record the real bowrain web app (target: "web"); nav is via data-testid
// (the bowrain sidebar uses testids, not aria-labels).

/** Dismiss the web-only "Open in Desktop" banner if it's on screen. It renders
 *  at the top of every project view (ProjectView → OpenInDesktop) and would sit
 *  over the project header for the rest of a walk. */
async function dismissOpenInDesktop(page: Page): Promise<void> {
  const dismiss = page.getByTestId("dismiss-open-in-desktop");
  if (await dismiss.count()) {
    await humanClick(page, dismiss).catch(() => {});
    await page.waitForTimeout(400);
  }
}

/** Open a file from the project source view into the Translate workbench.
 *
 *  A file on the source view opens in the preview sheet first (FilePreview:
 *  read the document, then choose a surface), and its Open in Translate button
 *  is the way into the workbench. Both arrivals are hard waits: a walk that
 *  stayed on the sheet, or never left the list, must fail the capture rather
 *  than film the wrong screen under the narration. */
async function openFileInTranslate(page: Page): Promise<void> {
  await humanClick(page, page.locator('[data-testid^="open-file"]').first());
  await page.waitForSelector('[data-testid="file-preview"]', { timeout: 20_000 });
  await page.waitForTimeout(1200);
  await humanClick(page, page.getByTestId("file-preview-translate"));
  await page.waitForSelector('[data-testid="view-switcher"]', { timeout: 30_000 });
  await page.waitForTimeout(1400);
}

/**
 * Step the visual editor to a block the workspace has something to say about.
 *
 * The memory expander (`tm-toggle`) renders only when the ACTIVE block has
 * matches, and the term sidebar only when it has term matches, so which block
 * the editor lands on decides whether either exists. Two walks narrate them, so
 * neither may assume the first block is the lucky one: this walks forward until
 * one appears and throws when none does, rather than filming an editing card
 * with nothing beside it under narration that says otherwise.
 */
async function focusBlockWithContext(page: Page, maxSteps = 12): Promise<void> {
  const present = async () =>
    (await page.getByTestId("tm-toggle").count()) > 0 ||
    (await page.getByTestId("term-sidebar").count()) > 0;
  if (await present()) return;
  for (let i = 0; i < maxSteps; i++) {
    const next = page.getByTestId("next-block-btn");
    if (!(await next.count()) || !(await next.isEnabled().catch(() => false))) break;
    await humanClick(page, next);
    await page.waitForTimeout(700);
    if (await present()) return;
  }
  throw new Error(
    "no block in this file carries a content-memory or term match — the walk narrates them, so seed the workspace (harness/scripts/seed-bowrain.ts) before recording",
  );
}

/** Switch the Translate workbench to the visual view: an editing card over the
 *  rendered document. */
async function openVisualView(page: Page): Promise<void> {
  await humanClick(page, page.getByTestId("view-visual"));
  await page.waitForSelector('[data-testid="visual-editor-layout"]', { timeout: 20_000 });
  await page.waitForTimeout(1200);
}

/** Bowrain web: one concept's record, the memory that already answers for its
 *  wording, and the moment that wording reaches the editor.
 *
 *  Concepts and the content memory are sections of the Context hub
 *  (nav-context → subnav-concepts / subnav-memory); the third beat leaves the
 *  hub for a real file in Translate, because the point of governing a term is
 *  what happens where someone is typing. Navigation between the three sits
 *  outside the beats: the camera only shows what the narration is about. */
async function bowrainGovernanceWalk(c: WalkCtx): Promise<void> {
  const { page, beatEls, cursorTo } = c;
  const tap = (id: string) => humanClick(page, page.getByTestId(id));

  // Into the Context hub's Concepts section. The narration reads a concept's
  // own page, so the list is a hard wait: a take that never got there would
  // film the dashboard under the words.
  await tap("nav-context");
  await page.waitForTimeout(700);
  await tap("subnav-concepts");
  await page.waitForSelector('[data-testid="concept-list"]', { timeout: 20_000 });
  await page.waitForTimeout(900);

  // One concept: the idea, and what the team calls it in each language, each
  // term with the status they agreed.
  await beatEls("concept", ['[data-testid="concept-view"]'], async () => {
    await humanClick(page, page.getByTestId("concept-row").first());
    await page.waitForSelector('[data-testid="concept-view"]', { timeout: 20_000 });
    await page.waitForTimeout(1600);
    await cursorTo('[data-testid="concept-header"]');
    await page.waitForTimeout(2000);
  });

  // The same wording, already on record in the workspace content memory.
  await beatEls("tm-search", ['[data-testid="tm-browser"]'], async () => {
    await tap("subnav-memory");
    await page.waitForSelector('[data-testid="tm-browser"]', { timeout: 20_000 });
    await page.waitForTimeout(900);
    const search = page.getByTestId("tm-search");
    if (await search.count()) await humanType(page, search, "mission", { submit: true });
    await page.waitForTimeout(1800);
  });

  // Out of the hub and into a file. The term sidebar and the memory matches
  // dock beside the block being translated, which is the whole point of
  // holding either on the server. The narration says they are on screen, so
  // one of the two is a hard wait.
  await tap("nav-translate");
  await page.waitForTimeout(700);
  await openProjectSource(page, "Company Website");
  await openFileInTranslate(page);
  await openVisualView(page);
  await focusBlockWithContext(page);
  await beatEls("in-the-editor", ['[data-testid="term-sidebar"]', '[data-testid="context-panel"]'], async () => {
    const tm = page.getByTestId("tm-toggle");
    if ((await tm.count()) && !(await page.getByTestId("context-panel").isVisible().catch(() => false)))
      await humanClick(page, tm);
    await page.waitForSelector('[data-testid="term-sidebar"], [data-testid="context-panel"]', {
      timeout: 20_000,
    });
    await page.waitForTimeout(1000);
    await cursorTo('[data-testid="term-sidebar"], [data-testid="context-panel"]');
    await page.waitForTimeout(2200);
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

/** Web translation editor: an editing card over the rendered page, one edit
 *  landing in the live preview, a memory match applied in one click, and the
 *  same file in the next language.
 *
 *  Reaching the file is navigation, so it happens before the first beat: the
 *  camera opens inside the workbench, on the thing the video is about. */
async function bowrainEditorWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo } = c;
  await openProjectSource(page, "Company Website");
  await openFileInTranslate(page);

  // The visual view: the editing card sits over the document a reader sees.
  await beat("split", { x: 0.03, y: 0.16, w: 0.74, h: 0.5 }, async () => {
    await openVisualView(page);
    await moveTo(page, WIDTH * 0.42, HEIGHT * 0.42, 700);
    await page.waitForTimeout(1800);
  });

  // One edit, typed and saved, and the preview beside it re-renders. The
  // narration says the page changed, so the editor and the Save it needs are
  // hard waits rather than optional steps.
  await beatEls("edit", ['[data-testid="visual-editor-card"]', '[data-testid="preview-iframe"]'], async () => {
    const start = page.getByTestId("edit-btn");
    if (await start.count()) await humanClick(page, start);
    await page.waitForSelector('[data-testid="unified-target-editor"]', { timeout: 20_000 });
    const field = page.locator('[data-testid="unified-target-editor"] [contenteditable="true"]').first();
    await humanClick(page, field);
    await page.keyboard.press("ControlOrMeta+a");
    await field.pressSequentially("Chaque marque a une voix.", { delay: 85 });
    await page.waitForTimeout(600);
    await humanClick(page, page.getByTestId("unified-save"));
    await page.waitForTimeout(2400);
  });

  // A memory match, applied in one click: the wording the workspace already
  // approved for this string, without retyping it.
  await focusBlockWithContext(page);
  await beatEls("memory", ['[data-testid="context-panel"]', '[data-testid="term-sidebar"]', '[data-testid="visual-editor-card"]'], async () => {
    const tm = page.getByTestId("tm-toggle");
    if ((await tm.count()) && !(await page.getByTestId("context-panel").isVisible().catch(() => false)))
      await humanClick(page, tm);
    await page.waitForTimeout(900);
    const apply = page.getByTestId("tm-apply-0");
    if (await apply.count()) {
      await cursorTo('[data-testid="tm-apply-0"]');
      await humanClick(page, apply);
    } else {
      // No memory match on this block, but focusBlockWithContext guarantees a
      // term match instead; the beat then shows the terms in force.
      await cursorTo('[data-testid="term-sidebar"]');
    }
    await page.waitForTimeout(2400);
  });

  // The same file, the next language, without leaving it.
  await beat("locale", { x: 0.03, y: 0.16, w: 0.8, h: 0.5 }, async () => {
    const sel = page.getByTestId("locale-selector");
    if (await sel.count()) {
      await humanClick(page, sel);
      await page.waitForTimeout(700);
      const opt = page.getByRole("option", { name: /German/ }).first();
      if (await opt.count()) await humanClick(page, opt);
      else await page.keyboard.press("Escape").catch(() => {});
    }
    await page.waitForTimeout(1800);
    await moveTo(page, WIDTH * 0.42, HEIGHT * 0.4, 600);
    await page.waitForTimeout(1400);
  });
}

/**
 * Review and approve on the platform, with two genuine users.
 *
 * The surface is the project review session (`p/{id}/s/{stream}/review`,
 * ReviewSessionRoute), reached from the workspace review inbox. One establishing
 * beat carries the inbox, the session and the language the queue is scoped to;
 * everything after it is a decision.
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
  const { page, beatEls, cursorTo, peer } = c;
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

  // One establishing beat: the workspace inbox, then the project's queue,
  // scoped to a language with the source marked.
  await beatEls("queue", ['[data-testid="review-queue"]', '[data-testid="review-filters"]'], async () => {
    await page.goto(`${wsBase}/review-inbox${themeQ}`, { waitUntil: "domcontentloaded" });
    await injectCursor(page); // goto wiped the page-injected cursor
    await page.waitForSelector('[data-testid="review-inbox"]', { timeout: 30_000 }).catch(() => {});
    await page.waitForTimeout(1600);
    const projectRow = page.locator('[data-testid^="review-inbox-project-"]').first();
    if (await projectRow.count()) {
      await humanClick(page, projectRow);
    } else if (reviewUrl) {
      await page.goto(reviewUrl, { waitUntil: "domcontentloaded" });
      await injectCursor(page);
    }
    await page.waitForSelector('[data-testid="review-session"]', { timeout: 30_000 });
    await page.waitForTimeout(1600);
    await cursorTo('[data-testid="filter-language"]');
    await page.waitForTimeout(1600);
  });

  // One unit in focus: its verdict against every bar the server applies.
  await beatEls("focus", ['[data-testid="focused-reviewer"]'], async () => {
    const bobRow = BOWRAIN_PEER_BLOCK_ID ? row(BOWRAIN_PEER_BLOCK_ID) : null;
    if (bobRow && (await bobRow.count())) await humanClick(page, bobRow);
    await page.waitForSelector('[data-testid="focused-reviewer"]', { timeout: 20_000 });
    await page.waitForTimeout(1200);
    await cursorTo('[data-testid="reviewer-verdict-passing"], [data-testid="reviewer-verdict-failing"]');
    await page.waitForTimeout(2200);
  });

  // Alice approves the translation Bob wrote. A second pair of eyes.
  await beatEls("approve", ['[data-testid="reviewer-approve"]', '[data-testid="review-pending-count"]'], async () => {
    await cursorTo('[data-testid="reviewer-approve"]');
    await humanClick(page, page.getByTestId("reviewer-approve"));
    await page.waitForTimeout(2800);
  });

  // Her own translation is a different matter. The workspace policy refuses it,
  // and the server's own sentence lands on screen.
  await beatEls("duties", ['[data-testid="error-notice"]'], async () => {
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
    await page.waitForTimeout(2600);
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
 * a mock.
 *
 * The camera opens on that arrival: Alice is already in the file when the take
 * starts, so the first thing a viewer sees is the avatar appearing. The walk
 * closes on the governance frame (members + roles), which is who is allowed in.
 *
 * Post-refocus surfaces: this lives in the Translate surface (Visual/Table),
 * where PresenceAvatars render in the editor header. Connectors are remote-only
 * on desktop and out of scope here.
 *
 * If no peer is configured (BOWRAIN_PEER_TOKEN unset), the walk records the
 * single-user editor and governance frames and skips the live-presence beats,
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

  // Seed values written by harness/scripts/seed-bowrain.ts. The workspace slug
  // is the path the recorder landed on.
  const slug = startUrl.pathname.replace(/^\/+|\/+$/g, "").split("/")[0] || "";
  const projectId = BOWRAIN_PROJECT_ID;
  const itemName = BOWRAIN_ITEM_NAME;
  const locale = BOWRAIN_COLLAB_LOCALE;
  const canCollab = !!(peer && projectId && itemName);

  // Alice opens the shared file — before the first beat, so the camera starts on
  // the arrival rather than on navigation. The editor's view switcher is a hard
  // wait: a name the route cannot resolve renders an empty editor, and that take
  // must fail rather than film "No blocks to display" under the story.
  if (projectId && itemName) {
    await page.goto(
      `${wsBase}/p/${projectId}/s/main/translate/${translatePath(itemName)}${themeQ}`,
      { waitUntil: "domcontentloaded" },
    );
    await injectCursor(page); // goto wiped the page-injected cursor; re-add it
    await page.waitForSelector('[data-testid="view-switcher"]', { timeout: 30_000 });
    await page.waitForSelector('[data-testid="visual-editor-layout"], [data-testid="view-table"][data-state="on"], [data-testid="search-input"]', { timeout: 30_000 });
  } else {
    // No seed → land Alice on the first project's file via the dashboard.
    await openProjectSource(page);
    const open = page.locator('[data-testid^="open-file"]').first();
    if (await open.count()) await humanClick(page, open);
  }
  await page.waitForTimeout(1600);

  if (canCollab) {
    // Bob (off-camera) joins the SAME file. His useCollaboration() opens the
    // collab WebSocket and publishes awareness — Alice's editor header now shows
    // his PresenceAvatar arrive. This is the genuine multi-user moment, and it
    // is the first thing the video shows, so its arrival is a hard wait.
    await beatEls("teammate-joins", ['[data-testid="presence-avatars"]'], async () => {
      await peer!.openTranslateFile(slug, projectId, itemName, locale);
      await page.waitForSelector('[data-testid="presence-avatars"]', { timeout: 20_000 });
      await page.waitForTimeout(1200);
      await cursorTo('[data-testid="presence-avatars"]');
      await page.waitForTimeout(2400);
    });

    // Bob moves to a block of his own. Two people are in one file, and the
    // presence follows the person rather than sitting in a header.
    await beat("co-editing", { x: 0.02, y: 0.08, w: 0.96, h: 0.7 }, async () => {
      await peer!.act(async (bp) => {
        const tbl = bp.getByTestId("view-table");
        if (await tbl.count()) await tbl.click().catch(() => {});
        await bp.waitForTimeout(500);
        const next = bp.getByTestId("next-block-btn");
        if (await next.count()) await next.click().catch(() => {});
        await bp.waitForTimeout(600);
      });
      await moveTo(page, WIDTH * 0.4, HEIGHT * 0.42, 700);
      await page.waitForTimeout(2600);
    });
  }

  // Who is allowed in: members carry roles, and an invite is an email and a
  // role. The invite dialog is the action, and the beat holds on it.
  await beatEls("members", ['[role="dialog"]', '[data-testid="invite-open-dialog-btn"]', '[data-testid="settings-heading"]'], async () => {
    await page.goto(`${wsBase}/settings/members${themeQ}`, { waitUntil: "domcontentloaded" });
    await injectCursor(page); // re-add cursor after navigation
    await page.waitForSelector('[data-testid="settings-heading"], [data-testid="invite-open-dialog-btn"]', { timeout: 15_000 }).catch(() => {});
    await page.waitForTimeout(1200);
    const open = page.getByTestId("invite-open-dialog-btn");
    if (await open.count()) {
      await cursorTo('[data-testid="invite-open-dialog-btn"]');
      await humanClick(page, open);
      await page.waitForTimeout(1000);
      const email = page.getByTestId("invite-email-input");
      if (await email.count()) await humanType(page, email, "sam@bowmart.example");
      await page.waitForTimeout(700);
      const role = page.getByTestId("invite-role-select");
      if (await role.count()) {
        await humanClick(page, role);
        await page.waitForTimeout(1100);
        await page.keyboard.press("Escape").catch(() => {});
      }
    }
    await page.waitForTimeout(1600);
  });
}

/** Bowrain Desktop: the same shared workspace the browser opens, and the one
 *  thing only the native app does — keep working with the server gone.
 *
 *  The desktop mounts the SAME shared app (@neokapi/bowrain-app) the browser
 *  runs, so nav testids match the web: workspace-level nav-* rail, project-scoped
 *  subnav-* views (dashboard | automations | runs | connectors).
 *
 *  The offline beats are real. `startBowrainStack` puts a TCP relay between the
 *  desktop backend and bowrain-server; cutting the relay drops the connection
 *  the way a lost network does, so the backend queues the edit, the chrome
 *  shows the queue depth, and the backend's own reconnect loop replays it when
 *  the relay returns. Nothing about the offline state is staged in the frontend,
 *  and a take without the relay fails rather than narrating a queue it cannot
 *  produce. */
async function bowrainDesktopWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo } = c;
  const link = bowrainServerLink;
  if (!link) {
    throw new Error(
      "bowrain-desktop-dashboard: no server relay. The offline beats need the stack this recorder starts (unset DEMO_URL).",
    );
  }

  // The workspace, in a native window: the same projects the browser shows.
  await beat("projects", { x: 0.03, y: 0.1, w: 0.94, h: 0.56 }, async () => {
    await moveTo(page, WIDTH * 0.4, HEIGHT * 0.4, 700);
    await page.waitForTimeout(2400);
  });

  // Open a file and edit one block, with the server still there.
  await openProjectSource(page);
  await openFileInTranslate(page);
  await beatEls("edit", ['[data-testid="visual-editor-card"]'], async () => {
    await openVisualView(page);
    const start = page.getByTestId("edit-btn");
    if (await start.count()) await humanClick(page, start);
    await page.waitForSelector('[data-testid="unified-target-editor"]', { timeout: 20_000 });
    const field = page.locator('[data-testid="unified-target-editor"] [contenteditable="true"]').first();
    await humanClick(page, field);
    await page.keyboard.press("ControlOrMeta+a");
    await field.pressSequentially("Bienvenue chez BowMart.", { delay: 85 });
    await page.waitForTimeout(500);
    await humanClick(page, page.getByTestId("unified-save"));
    await page.waitForTimeout(2200);
  });

  // Cut the link and keep working. The next save cannot reach the server, the
  // backend goes offline, and the queue depth appears in the chrome. The
  // narration says the edits are queued, so the indicator is a hard wait.
  await beatEls("offline", ['[data-testid="offline-pending"]'], async () => {
    await link.cut();
    for (const text of ["Ouvert tous les jours.", "Livraison sous 48 heures."]) {
      const start = page.getByTestId("edit-btn");
      if (await start.count()) await humanClick(page, start);
      const field = page.locator('[data-testid="unified-target-editor"] [contenteditable="true"]').first();
      if (await field.count()) {
        await humanClick(page, field);
        await page.keyboard.press("ControlOrMeta+a");
        await field.pressSequentially(text, { delay: 70 });
        await humanClick(page, page.getByTestId("unified-save"));
      }
      await page.waitForTimeout(1400);
      const next = page.getByTestId("next-block-btn");
      if (await next.count()) await humanClick(page, next);
      await page.waitForTimeout(800);
    }
    await page.waitForSelector('[data-testid="offline-pending"]', { timeout: 30_000 });
    await page.waitForTimeout(900);
    await cursorTo('[data-testid="offline-pending"]');
    await page.waitForTimeout(2200);
  });

  // Put the link back. The backend's reconnect loop replays the queue in order
  // and the indicator clears itself; nothing here clicks a retry.
  await beatEls("replayed", ['[data-testid="connection-status"]'], async () => {
    await link.restore();
    await page.waitForSelector('[data-testid="offline-pending"]', { state: "detached", timeout: 60_000 });
    await page.waitForTimeout(1200);
    await cursorTo('[data-testid="connection-status"]');
    await page.waitForTimeout(2200);
  });
}

/** Bowrain Desktop: a project's standing rules and the server-side runs they
 *  drive — the unified project views (dashboard | automations | runs |
 *  connectors) that replaced the decommissioned flows/FlowBuilder screens.
 *  Flow editing still exists, but as the Flows tab inside Automations. */
async function bowrainDesktopAutomationsWalk(c: WalkCtx): Promise<void> {
  const { page, beatEls, cursorTo } = c;
  // The project-scoped sub-nav only exists inside a project.
  const card = page.locator('[data-testid^="project-card"]').first();
  if (await card.count()) await humanClick(page, card);
  await page.waitForSelector('[data-testid="subnav-automations"]', { timeout: 20_000 });
  await page.waitForTimeout(1200);

  // The standing rules: what fires, and on what. Automations lands on its Runs
  // tab, so the Rules tab is a click (the tab strip is plain buttons —
  // Runs · Rules · Flows — without testids).
  await beatEls("rule", ['h2:has-text("Automation Rules")', 'button:has-text("New Rule")'], async () => {
    await humanClick(page, page.getByTestId("subnav-automations"));
    await page.waitForTimeout(1400);
    const rules = page.locator('button:has-text("Rules")').first();
    if (await rules.count()) await humanClick(page, rules);
    await page.waitForTimeout(2400);
  });

  // Start a pass on camera. Run now opens the scope dialog; starting it puts a
  // new run at the top of the table. The narration says a run started, so both
  // the dialog and the row are hard waits.
  await beatEls("run-now", ['[data-testid="runs-list"]'], async () => {
    await humanClick(page, page.getByTestId("subnav-runs"));
    await page.waitForSelector('[data-testid="runs-list"]', { timeout: 20_000 });
    await page.waitForTimeout(1200);
    await cursorTo('[data-testid="run-now-btn"]');
    await humanClick(page, page.getByTestId("run-now-btn"));
    await page.waitForSelector('[role="dialog"]', { timeout: 20_000 });
    await page.waitForTimeout(1600);
    const start = page.getByRole("button", { name: /Translate all now/i }).first();
    if (await start.count()) await humanClick(page, start);
    else await page.keyboard.press("Escape").catch(() => {});
    await page.waitForSelector('[data-testid="run-row"]', { timeout: 30_000 });
    await page.waitForTimeout(2400);
  });

  // The run settles, and its row says what each language got. What needs a
  // person is parked, and parked work is already in review.
  await beatEls("settled", ['[data-testid="run-row"]'], async () => {
    await page.waitForTimeout(3000);
    await cursorTo('[data-testid="run-row"]');
    await page.waitForTimeout(2600);
  });

  // The parked work, where it waits: the project's own delivery card links
  // straight into the review session.
  await beatEls("parked", ['[data-testid="delivery-panel"]', '[data-testid="review-session"]'], async () => {
    await humanClick(page, page.getByTestId("subnav-dashboard"));
    await page.waitForTimeout(1600);
    const toReview = page.getByTestId("delivery-open-review");
    if (await toReview.count()) {
      await cursorTo('[data-testid="delivery-open-review"]');
      await humanClick(page, toReview);
      await page.waitForSelector('[data-testid="review-session"]', { timeout: 30_000 }).catch(() => {});
    }
    await page.waitForTimeout(2400);
  });
}

/** Bowrain web: the correction-learning loop. Candidate rules drawn from a
 *  team's corrections, a blast-radius preview held while its numbers are read,
 *  and promotion into a versioned check. The voice profile's review route is
 *  /:ws/context/voice/review/:profileId (routes/index.tsx `context` → `voice` →
 *  `review/$profileId`); the workspace slug comes from BOWRAIN_WORKSPACE_SLUG
 *  and the profile id from BOWRAIN_DEMO_PROFILE_ID (both printed by
 *  harness/scripts/seed-correction-loop.mjs). */
async function bowrainCorrectionLoopWalk(c: WalkCtx): Promise<void> {
  const { page, beat, beatEls, cursorTo } = c;
  const startUrl = new URL(page.url());
  const themeQ = startUrl.searchParams.get("theme") ? `?theme=${startUrl.searchParams.get("theme")}` : "";
  // The page opened at origin/<slug>; that pathname is the workspace base.
  const wsBase = `${startUrl.origin}${startUrl.pathname}`.replace(/\/$/, "");
  const profileId = process.env.BOWRAIN_DEMO_PROFILE_ID || "";

  await page.goto(`${wsBase}/context/voice/review/${profileId}${themeQ}`, { waitUntil: "domcontentloaded" });
  await injectCursor(page); // goto wiped the page-injected cursor; re-add it
  await page.waitForTimeout(1800);

  // The candidates: each one a phrasing the team kept correcting, with the
  // count of corrections standing behind it.
  await beatEls("candidates", ['text=Review suggested rules', "ul"], async () => {
    await page.waitForTimeout(2600);
  });

  // The blast radius, before anything lands: pick a project, open a candidate's
  // impact dialog, and hold on it while the numbers are read.
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
    await page.waitForSelector('[role="dialog"]', { timeout: 20_000 });
    await page.waitForTimeout(3400);
    await page.keyboard.press("Escape").catch(() => {});
    await page.waitForTimeout(600);
  });

  // Promote it. The candidate leaves the queue as a versioned rule that every
  // later run enforces.
  await beat("promote", null, async () => {
    const promote = page.getByRole("button", { name: /^Promote$/ }).first();
    if (await promote.count()) {
      await cursorTo('button:has-text("Promote")');
      await humanClick(page, promote);
    }
    await page.waitForTimeout(3000);
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

async function runWalkthrough(
  page: Page,
  t0: number,
  demoId: string,
  peer: PeerSession | undefined,
  specs: Record<string, BeatSpec>,
  holds: Record<string, number>,
): Promise<Beat[]> {
  const walk = WALKTHROUGHS[demoId];
  if (!walk) throw new Error(`no walkthrough registered for "${demoId}"`);
  const beats: Beat[] = [];
  await walk(makeCtx(page, t0, beats, peer, specs, holds));
  return beats;
}

/** What one theme's take needs beyond the page: the manifest's beat specs and the narration's holds. */
interface TakeOptions {
  web?: { slug: string };
  ready?: string;
  uiLocale?: string;
  specs: Record<string, BeatSpec>;
  holds: Record<string, number>;
}

async function recordTheme(
  browser: Browser,
  url: string,
  theme: ThemeMode,
  outDir: string,
  demoId: string,
  take: TakeOptions,
): Promise<{ webm: string; beats: Beat[]; clicks: number[] }> {
  const { web, ready, uiLocale, specs, holds } = take;
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
  // recording mid-run (toggle is idempotent → no loop). The web capture gets no
  // traffic-light gutter: DesktopScene frames a web take under its own browser
  // bar, so the app content starts below the dots already.
  //
  // The script is a string, not a function. tsx runs the recorder through
  // esbuild with keepNames on, which rewrites a named inner function such as
  // `const pin = () => …` to `__name(() => …, "pin")`; Playwright serialises a
  // function argument by its source, so the page received a reference to a
  // helper it does not have and threw "__name is not defined" at document
  // start. The take then carried on with no pin at all and only the recorder's
  // pageerror line said so.
  //
  // At document start the root element may not exist yet, so the pin attaches
  // to it as soon as it appears and re-asserts on every later class change.
  await context.addInitScript(
    `(() => {
      const isDark = ${theme === "dark"};
      const pin = () => {
        const root = document.documentElement;
        if (root) root.classList.toggle("dark", isDark);
      };
      const attach = () => {
        const root = document.documentElement;
        if (!root) return false;
        pin();
        new MutationObserver(pin).observe(root, { attributes: true, attributeFilter: ["class"] });
        return true;
      };
      if (!attach()) {
        const boot = new MutationObserver(() => {
          if (attach()) boot.disconnect();
        });
        boot.observe(document, { childList: true });
      }
      document.addEventListener("DOMContentLoaded", pin);
    })();`,
  );
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
    // `&lang=` carries the UI locale of a pass in another language (see the
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

  // Every click's moment, so the composition can sound it where the ripple blooms.
  const clicks: number[] = [];
  setClickSink((atMs) => clicks.push(Number(((atMs - t0) / 1000).toFixed(3))));
  let beats: Beat[];
  try {
    beats = await runWalkthrough(page, t0, demoId, peer, specs, holds);
  } finally {
    setClickSink(null);
    if (peerTeardown) await peerTeardown();
  }
  await page.waitForTimeout(500);

  const video = page.video();
  await context.close(); // finalizes the webm
  const raw = video ? await video.path() : "";

  const webm = path.join(outDir, `screencast-${theme}.webm`);
  if (raw && fs.existsSync(raw)) reencodeDenseKeyframes(raw, webm);
  fs.rmSync(videoDir, { recursive: true, force: true });
  return { webm: path.basename(webm), beats, clicks };
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

/**
 * The measured length of the narration over each beat, from a narration.json
 * that was synthesized before this recording (narrate first, then record):
 * beat id → seconds. Empty when there is none, so the walk's own waits stand.
 */
function narrationHolds(outDir: string, uiLocale?: string): Record<string, number> {
  const file = path.join(outDir, `narration${uiLocale ? `-${uiLocale}` : ""}.json`);
  if (!fs.existsSync(file)) return {};
  try {
    const n = JSON.parse(fs.readFileSync(file, "utf8")) as { scenes?: Array<{ kind: string; beat?: string; durationSec?: number }> };
    const out: Record<string, number> = {};
    for (const s of n.scenes ?? []) {
      if (s.kind === "desktop" && s.beat && s.durationSec && s.durationSec > 0) out[s.beat] = s.durationSec;
    }
    return out;
  } catch {
    return {};
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
   * What the manifest asks of each beat, by beat id: hold, crop and highlight
   * selectors (see BeatSpec). A beat with no entry records as the walk wrote it.
   */
  beats?: Record<string, BeatSpec>;
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
   *     (qps.json today), produced by `vpx neokapi-i18n compile`.
   *
   * What this option does (the recorder-side plumbing):
   *   1. persists the language on the recording backend via the wbridge
   *      (SetUILanguage), so the genuine app setting matches the pass, and
   *   2. appends `&lang=<locale>` to the recording URL next to `?theme=`.
   *
   * What a recording in another UI language still needs (owned by apps/kapi-desktop):
   *   - the recorder entry (src/demo/real-main.tsx) mounts App directly and
   *     skips main.tsx's translation bootstrap — it must honor `?lang=`
   *     (mirroring `?theme=`) by calling
   *     loadTranslations(lang, `/translations/<lang>.json`), and
   *   - a compiled catalog for the locale must exist, e.g.
   *     frontend/public/translations/nb.json (neokapi-i18n extract + compile).
   * Until both land, a pass in another language records the English UI; the
   * narration, captions and published filenames are in that language.
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
  // UI language pass-through (kapi-desktop target only; see RecordOptions.uiLocale).
  const uiLocale = opts.uiLocale && opts.uiLocale !== "en" ? opts.uiLocale : undefined;
  if (uiLocale && (opts.web || opts.bowrainDesktop)) {
    console.warn(`  ! uiLocale=${uiLocale} is not wired for the ${opts.web ? "web" : "bowrain-desktop"} target; recording the default UI language`);
  }
  const specs = opts.beats ?? {};
  const holds = narrationHolds(outDir, uiLocale);
  const heldIds = Object.keys(specs).filter((id) => specs[id]?.hold !== undefined);
  if (heldIds.length || Object.keys(holds).length) {
    console.log(`  · beat holds: ${heldIds.length} from demo.yaml, ${Object.keys(holds).filter((id) => !heldIds.includes(id)).length} from the narration`);
  }

  // Web target: record the real bowrain web app at BOWRAIN_BACKEND_URL with the
  // session cookie — no wbridge, no local stack to manage here.
  if (opts.web) {
    if (!BOWRAIN_TOKEN) throw new Error("bowrain web record: set BOWRAIN_SESSION_TOKEN (device-flow JWT)");
    const slug = await bowrainWorkspaceSlug();
    const browser = await chromium.launch();
    try {
      console.log(`  · recording bowrain web (light) @ ${BOWRAIN_BASE}/${slug}`);
      const light = await recordTheme(browser, BOWRAIN_BASE, "light", outDir, id, { web: { slug }, specs, holds });
      console.log("  · recording bowrain web (dark)");
      const dark = await recordTheme(browser, BOWRAIN_BASE, "dark", outDir, id, { web: { slug }, specs, holds });
      const screencast: Screencast = {
        width: WIDTH,
        height: HEIGHT,
        video: { light: light.webm, dark: dark.webm },
        beats: { light: light.beats, dark: dark.beats },
        clicks: { light: light.clicks, dark: dark.clicks },
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
      const light = await recordTheme(browser, stack.url, "light", outDir, id, { ready, specs, holds });
      console.log("  · recording bowrain desktop (dark)");
      const dark = await recordTheme(browser, stack.url, "dark", outDir, id, { ready, specs, holds });
      const screencast: Screencast = {
        width: WIDTH,
        height: HEIGHT,
        video: { light: light.webm, dark: dark.webm },
        beats: { light: light.beats, dark: dark.beats },
        clicks: { light: light.clicks, dark: dark.clicks },
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
  // (Settings → General → UI language) matches the pass. Best-effort:
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
  // terms, memories and providers under ISO_DIR are left intact.
  //
  // The backend is one long-lived process across both themes and it restores
  // the tabs that were open (SaveSessionState → GetSessionState), so wiping
  // the home alone left the dark pass reopening a project whose files were
  // gone: the app showed the template picker, or a review page that could not
  // read its recipe, under a walk that had found its rail item enabled. Close
  // every tab and forget the session through the wbridge as well, so both
  // passes start on the home screen and scaffold the sample the same way.
  const bridge = async (method: string, args: unknown[] = []): Promise<unknown> => {
    const r = await fetch("http://127.0.0.1:5175/wbridge", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ method, args }),
    });
    return r.json();
  };
  const resetHome = async () => {
    if (!externalUrl) {
      try {
        const tabs = (await bridge("ListTabs")) as Array<{ id?: string }> | null;
        for (const tab of tabs ?? []) {
          if (tab?.id) await bridge("CloseProject", [tab.id]);
        }
        await bridge("SaveSessionState", [{ mode: "projects", lastOpenProjects: [], activeProject: "" }]);
      } catch (e) {
        console.warn(`  ! could not close the open projects on the wbridge: ${(e as Error)?.message}`);
      }
    }
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
    await resetHome();
    await resetPlugins();
    const light = await recordTheme(browser, url, "light", outDir, id, { uiLocale, specs, holds });
    console.log(`  · recording dark theme${uiLocale ? ` (ui ${uiLocale})` : ""}`);
    await resetHome();
    await resetPlugins();
    const dark = await recordTheme(browser, url, "dark", outDir, id, { uiLocale, specs, holds });

    const screencast: Screencast = {
      width: WIDTH,
      height: HEIGHT,
      video: { light: light.webm, dark: dark.webm },
      beats: { light: light.beats, dark: dark.beats },
      clicks: { light: light.clicks, dark: dark.clicks },
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
