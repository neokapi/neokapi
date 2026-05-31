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
 * the Claude Code plugin dir). Resolution order:
 *   1. `NEOKAPI_REPO` env var
 *   2. when embedded as `<checkout>/harness/`, the parent checkout (the normal case)
 *   3. `.env` NEOKAPI_REPO (for a standalone checkout outside the tree)
 *   4. default `~/src/neokapi/neokapi`
 */
function resolveRepoRoot(): string {
  if (process.env.NEOKAPI_REPO) return path.resolve(process.env.NEOKAPI_REPO);
  const parent = path.resolve(HARNESS_ROOT, "..");
  if (isNeokapiCheckout(parent)) return parent;
  try {
    // .env is loaded later by env.ts, but REPO_ROOT is needed at module-eval time.
    const m = fs.readFileSync(path.join(HARNESS_ROOT, ".env"), "utf8").match(/^\s*NEOKAPI_REPO\s*=\s*(.+?)\s*$/m);
    if (m) return path.resolve(m[1].replace(/^["']|["']$/g, "").replace(/^~/, os.homedir()));
  } catch {
    /* no .env */
  }
  return path.join(os.homedir(), "src", "neokapi", "neokapi");
}
export const REPO_ROOT = resolveRepoRoot();

/** Where published demo videos land in the docs site (served via the docs-assets release). */
export const DOCS_VIDEO_DIR = path.join(REPO_ROOT, "web", "docs", "static", "video", "kapi");

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
