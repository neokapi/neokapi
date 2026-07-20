// Build the one workspace library whose BUILD OUTPUT is needed at vite-config
// load time. @neokapi/i18n-react ships a vite plugin (dist/plugin/vite.js) that
// the apps' vite.config imports, and dist/ is gitignored — so a fresh checkout
// or git worktree can't even start vite until it's built, the recurring
// "build @neokapi/i18n-react first" gotcha. Runtime-only workspace deps (e.g.
// @neokapi/ui) are bundled from source by vite and need no prebuild; this is the
// sole exception, so we build exactly it on install.
//
// Wired as the root `postinstall`, so `(vp|pnpm) install` always leaves dist/
// present. It SKIPS silently on a partial checkout (the web/ctrl Docker builds
// copy only a workspace subtree that may not include this package) so it never
// breaks those, but a genuine build failure when the package IS present
// propagates and fails the install — which is what we want.
import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const pkgDir = join(repoRoot, "packages", "i18n-react");

if (!existsSync(join(pkgDir, "build.mjs"))) {
  // Not in this (partial) workspace — nothing to build.
  process.exit(0);
}

execFileSync("node", ["build.mjs"], { cwd: pkgDir, stdio: "inherit" });
