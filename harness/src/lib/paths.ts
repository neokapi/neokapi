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
 * Path to a neokapi checkout (provides the kapi source to build, the kapi binary, and
 * the Claude Code plugin dir). The harness lives at `<checkout>/harness/`, so the
 * enclosing checkout is the repo — this is the normal (and only expected) case.
 * Resolution order:
 *   1. `NEOKAPI_REPO` env var (escape hatch for a standalone harness checkout)
 *   2. the enclosing `<checkout>/harness/` parent (the normal case)
 *   3. default `~/src/neokapi/neokapi`
 *
 * Note: NEOKAPI_REPO is intentionally NOT read from `harness/.env` — the current
 * tree IS the repo, and pinning a path in .env broke worktrees (every worktree got
 * the same hard-coded checkout). Set the env var explicitly if you need the override.
 */
function resolveRepoRoot(): string {
  if (process.env.NEOKAPI_REPO) return path.resolve(process.env.NEOKAPI_REPO);
  const parent = path.resolve(HARNESS_ROOT, "..");
  if (isNeokapiCheckout(parent)) return parent;
  return path.join(os.homedir(), "src", "neokapi", "neokapi");
}
export const REPO_ROOT = resolveRepoRoot();

/** Where published kapi demo videos land in the neokapi docs site (served via the docs-assets release). */
export const DOCS_VIDEO_DIR = path.join(REPO_ROOT, "web", "docs", "static", "video", "kapi");

/** Base of the bowrain docs static video tree (sibling to the kapi one, in the bowrain docs site). */
const BOWRAIN_DOCS_VIDEO_BASE = path.join(REPO_ROOT, "bowrain", "web", "docs", "static", "video");

/**
 * The docs video directory a demo publishes into, derived from its brand + target.
 * kapi demos land in web/static/video/kapi; bowrain demos route to the matching
 * bowrain docs subdir — bowrain-web (target: web), bowrain-desktop (target:
 * bowrain-desktop), or bowrain-cli (shell/terminal demos). This is the single source
 * of truth for routing — without it every demo silently published to the kapi dir
 * unless a --docs-dir was passed by hand. An explicit --docs-dir still overrides.
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
export const PLUGIN_DIR = path.join(REPO_ROOT, "packages", "kapi-claude-plugin");

/**
 * Isolated kapi state so demos don't depend on this machine's installed plugins,
 * flows, TMs or config. Set via env (XDG_DATA_HOME / KAPI_CONFIG_DIR / KAPI_PLUGINS_DIR)
 * for every kapi invocation. (AI credentials still live in the OS keychain, which is
 * machine-global; the harness manages its own "harness-gemini" entry.)
 */
export const KAPI_ISO = path.join(HARNESS_ROOT, ".kapi");
export const KAPI_ISO_DATA = path.join(KAPI_ISO, "data"); // XDG_DATA_HOME → plugins live in <data>/kapi/plugins
export const KAPI_ISO_HOME = path.join(KAPI_ISO, "home"); // KAPI_CONFIG_DIR → KAPI_HOME (tm/termbase/flows)
export const KAPI_ISO_PLUGINS = path.join(KAPI_ISO_DATA, "kapi", "plugins");

/**
 * Env vars that point kapi at the isolated state. Merge into PATH-augmented env.
 *
 * XDG_DATA_HOME (→ <data>/kapi/plugins) + KAPI_CONFIG_DIR isolate the user-level
 * data and config roots. But XDG_DATA_HOME alone does NOT stop kapi from also
 * discovering plugins from the *system* roots (Homebrew, /usr/share), so a
 * recording could surface host system-root plugins. To fully isolate plugin
 * discovery — matching the desktop recorder (record-desktop.ts) — we set
 * KAPI_PLUGINS_DIR to the isolated plugins dir and KAPI_PLUGINS_DIR_ONLY=1 so
 * discovery is restricted to that one dir (no user XDG root, no system roots).
 * Pointing KAPI_PLUGINS_DIR at the same dir XDG_DATA_HOME would resolve to
 * (KAPI_ISO_PLUGINS) avoids double-discovery because _ONLY skips the XDG root.
 */
export function kapiIsolationEnv(): Record<string, string> {
  return {
    XDG_DATA_HOME: KAPI_ISO_DATA,
    KAPI_CONFIG_DIR: KAPI_ISO_HOME,
    KAPI_PLUGINS_DIR: KAPI_ISO_PLUGINS,
    KAPI_PLUGINS_DIR_ONLY: "1",
  };
}

/** Per-demo locations. */
export const demoSrcDir = (id: string) => path.join(DEMOS_DIR, id);
export const demoFixturesDir = (id: string) => path.join(DEMOS_DIR, id, "fixtures");
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
