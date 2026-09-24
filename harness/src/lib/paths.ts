import { fileURLToPath } from "node:url";
import path from "node:path";
import os from "node:os";
import fs from "node:fs";

/** Project root (this file is at <root>/src/lib/paths.ts). */
export const HARNESS_ROOT = path.resolve(fileURLToPath(import.meta.url), "../../..");

/** A directory looks like a neokapi checkout if it has the framework go.mod + core/. */
function isNeokapiCheckout(dir: string): boolean {
  try {
    return (
      fs.existsSync(path.join(dir, "core")) &&
      fs.readFileSync(path.join(dir, "go.mod"), "utf8").includes("module github.com/neokapi/neokapi")
    );
  } catch {
    return false;
  }
}

/**
 * Resolve the checkout providing kapi source, binaries and the Claude Code plugin.
 * Resolution order:
 *    1. An explicitly set NEOKAPI_REPO.
 *    2. The parent of this harness directory, if it is a neokapi checkout.
 *    3. NEOKAPI_WORKSPACE_DIR/neokapi, defaulting to the parent workspace.
 *
 * NEOKAPI_REPO is not loaded from harness/.env. A fixed checkout path there would
 * make separate worktrees use the same source tree.
 */
function resolveRepoRoot(): string {
  if (process.env.NEOKAPI_REPO) return path.resolve(process.env.NEOKAPI_REPO);
  const parent = path.resolve(HARNESS_ROOT, "..");
  if (isNeokapiCheckout(parent)) return parent;
  const workspace = process.env.NEOKAPI_WORKSPACE_DIR
    ? path.resolve(process.env.NEOKAPI_WORKSPACE_DIR)
    : path.resolve(parent, "..");
  return path.join(workspace, "neokapi");
}
export const REPO_ROOT = resolveRepoRoot();

/**
 * Staging directory for kapi documentation videos. scripts/publish-cdn-assets.sh
 * video-kapi uploads this gitignored directory to the CDN.
 */
export const DOCS_VIDEO_DIR = path.join(REPO_ROOT, "web", "static", "video", "kapi");

/** Base of the bowrain docs static video tree (sibling to the kapi one, in the bowrain docs site). */
const BOWRAIN_DOCS_VIDEO_BASE = path.join(REPO_ROOT, "bowrain", "web", "docs", "static", "video");

/**
 * Select the documentation video directory from the demo's brand and target.
 * Kapi uses web/static/video/kapi. Bowrain uses bowrain-web for web demos,
 * bowrain-desktop for desktop demos and bowrain-cli for shell demos.
 * An explicit --docs-dir overrides this default at the call site.
 */
export function docsVideoDirFor(m: { brand?: string; target?: string; terminal?: string }): string {
  if (m.brand === "bowrain") {
    if (m.target === "web") return path.join(BOWRAIN_DOCS_VIDEO_BASE, "bowrain-web");
    if (m.target === "bowrain-desktop") return path.join(BOWRAIN_DOCS_VIDEO_BASE, "bowrain-desktop");
    return path.join(BOWRAIN_DOCS_VIDEO_BASE, "bowrain-cli"); // shell/CLI bowrain demos
  }
  return DOCS_VIDEO_DIR;
}

export const DEMOS_DIR = path.join(HARNESS_ROOT, "demos");
export const PUBLIC_DIR = path.join(HARNESS_ROOT, "public");
export const OUT_DIR = path.join(HARNESS_ROOT, "out");
export const CAPTURES_DIR = path.join(HARNESS_ROOT, "captures");
export const ASSETS_DIR = path.join(HARNESS_ROOT, "assets");

/**
 * Sandboxes live OUTSIDE the repo tree so the headless `claude` run does not
 * climb up and auto-load the neokapi developer CLAUDE.md — each demo must look
 * like a standalone end-user project, not a contributor checkout.
 */
export const SANDBOX_DIR = path.join(os.tmpdir(), "kapi-harness-sandbox");

export const KAPI_BIN = path.join(REPO_ROOT, "bin", "kapi");

/** The marketplace `make plugin-bundle` assembles: `.claude-plugin/marketplace.json` + `plugins/`. */
export const PLUGIN_MARKETPLACE_DIR = path.join(REPO_ROOT, "packages", "kapi-claude-plugin");

/**
 * Plugin directory passed to claude --plugin-dir. It contains
 * .claude-plugin/plugin.json and skills/. Passing the marketplace root would
 * load no kapi skill because the skill directory is nested inside this plugin.
 */
export const PLUGIN_DIR = path.join(PLUGIN_MARKETPLACE_DIR, "plugins", "kapi");

/**
 * Isolated kapi state so demos don't depend on this machine's installed plugins,
 * flows, Memories or config. Set via env (XDG_DATA_HOME / KAPI_CONFIG_DIR / KAPI_PLUGINS_DIR)
 * for every kapi invocation. (AI credentials still live in the OS keychain, which is
 * machine-global; the harness manages its own "harness-gemini" entry.)
 */
export const KAPI_ISO = path.join(HARNESS_ROOT, ".kapi");
export const KAPI_ISO_DATA = path.join(KAPI_ISO, "data"); // XDG_DATA_HOME → plugins live in <data>/kapi/plugins
export const KAPI_ISO_DATA_ROOT = path.join(KAPI_ISO_DATA, "kapi"); // KAPI_DATA_DIR → the workspace lives in <root>/workspaces
export const KAPI_ISO_HOME = path.join(KAPI_ISO, "home"); // KAPI_CONFIG_DIR → KAPI_HOME (memory/terms/flows)
export const KAPI_ISO_PLUGINS = path.join(KAPI_ISO_DATA, "kapi", "plugins");
export const KAPI_ISO_CACHE = path.join(KAPI_ISO, "cache"); // XDG_CACHE_HOME → <cache>/kapi/plugins-cache.json

/**
 * Environment overrides isolating kapi state. Merge these into the command's
 * environment after configuring PATH.
 *
 * XDG_DATA_HOME and KAPI_CONFIG_DIR isolate user data and configuration.
 * KAPI_PLUGINS_DIR_ONLY restricts discovery to KAPI_PLUGINS_DIR, excluding both
 * user and system plugin roots. XDG_CACHE_HOME prevents reuse of the developer's
 * plugin cache.
 *
 * KAPI_NO_PROJECT disables upward project discovery. Demos that require project
 * discovery opt in explicitly from a sandbox outside the repository.
 *
 * KAPI_DATA_DIR takes precedence over XDG_DATA_HOME and isolates the workspace's
 * terms, voice profiles, content memory and decisions. It must be set even when
 * the XDG directories are overridden.
 */
export function kapiIsolationEnv(): Record<string, string> {
  ensureDir(KAPI_ISO_CACHE);
  return {
    XDG_DATA_HOME: KAPI_ISO_DATA,
    XDG_CACHE_HOME: KAPI_ISO_CACHE,
    KAPI_DATA_DIR: KAPI_ISO_DATA_ROOT,
    KAPI_CONFIG_DIR: KAPI_ISO_HOME,
    KAPI_PLUGINS_DIR: KAPI_ISO_PLUGINS,
    KAPI_PLUGINS_DIR_ONLY: "1",
    KAPI_NO_PROJECT: "1",
    // A walkthrough depicts a person working, and kapi reads a coding agent's
    // marker variables out of the shell this runs in.
    KAPI_ACTOR: "person",
    // Demo recordings must never emit telemetry or show the first-run
    // notice, even against a keyed release build.
    KAPI_TELEMETRY: "0",
  };
}

/** Per-demo locations. */
export const demoSrcDir = (id: string) => path.join(DEMOS_DIR, id);
export const demoFixturesDir = (id: string) => path.join(DEMOS_DIR, id, "fixtures");
/**
 * The directory a demo's sandbox is seeded from: its own `fixtures/`, or the
 * repo-relative tree named by `fixturesFrom:`. The second form lets a demo drive
 * a committed sample project (`samples/<name>/`) directly, so the recording and
 * the sample a reader clones cannot drift apart.
 */
export const demoFixturesDirFor = (m: { id: string; fixturesFrom?: string }): string =>
  m.fixturesFrom ? path.resolve(REPO_ROOT, m.fixturesFrom) : demoFixturesDir(m.id);
export const sandboxDir = (id: string) => path.join(SANDBOX_DIR, id);
export const captureDir = (id: string) => path.join(CAPTURES_DIR, id);
/** Public/<id> is what Remotion reads via staticFile(). */
export const publicDemoDir = (id: string) => path.join(PUBLIC_DIR, id);

export function ensureDir(p: string): string {
  fs.mkdirSync(p, { recursive: true });
  return p;
}

export function rmrf(p: string): void {
  fs.rmSync(p, { recursive: true, force: true });
}
