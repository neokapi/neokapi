// @ts-check
// Regenerates an app's qps (pseudo-locale) dictionary,
// public/translations/qps.json, when the app builds or its dev server starts,
// by running the surface's `make <surface>-qps` target from the repository
// root. See "App pseudo-locale dictionaries" in docs/internals/l10n-ci.md.
//
// It runs once per process and never fails the build: with no `make`, no kapi
// or a failed run it warns, and the qps locale renders English. It is skipped
// under Vitest, under CI (workflows run the target themselves) and when
// NEOKAPI_SKIP_QPS is set.

import { execFileSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

/** @type {Set<string>} */
const generated = new Set();

/**
 * Regenerate one surface's qps dictionary. Returns whether it ran.
 *
 * @param {string} target  the make target, e.g. "bowrain-app-qps"
 * @param {(message: string) => void} [warn]
 * @returns {boolean}
 */
export function generateQpsCatalog(target, warn = (m) => console.warn(m)) {
  if (generated.has(target)) return true;
  if (process.env.VITEST || process.env.CI || process.env.NEOKAPI_SKIP_QPS) return false;
  generated.add(target);
  try {
    execFileSync("make", ["-s", "-C", root, target], {
      stdio: ["ignore", "ignore", "pipe"],
      encoding: "utf8",
    });
    return true;
  } catch (err) {
    const e = /** @type {{ stderr?: string, message?: string }} */ (err);
    const detail = (e.stderr || e.message || String(err)).trim().split("\n").slice(-3).join("\n");
    warn(
      `[qps] public/translations/qps.json was not regenerated (make ${target}); ` +
        `the qps locale shows whatever dictionary is on disk, or English.\n${detail}`,
    );
    return false;
  }
}

/**
 * A Vite plugin that regenerates the qps dictionary at build start, which is
 * also when the dev server starts.
 *
 * @param {string} target  the make target, e.g. "kapi-desktop-qps"
 * @returns {{ name: string, buildStart(this: { warn(message: string): void }): void }}
 */
export default function qpsCatalog(target) {
  return {
    name: "neokapi-qps-catalog",
    buildStart() {
      generateQpsCatalog(target, (m) => this.warn(m));
    },
  };
}
