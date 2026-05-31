import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { HARNESS_ROOT } from "./paths.ts";

let loaded = false;

/** Shared, worktree-independent secrets file. Lets every checkout/worktree reuse
 *  one GEMINI_API_KEY instead of copying a per-tree .env. Honours XDG_CONFIG_HOME. */
function sharedEnvFile(): string {
  const cfg = process.env.XDG_CONFIG_HOME || path.join(os.homedir(), ".config");
  return path.join(cfg, "neokapi", "harness.env");
}

function applyEnvFile(file: string): void {
  if (!fs.existsSync(file)) return;
  for (const line of fs.readFileSync(file, "utf8").split("\n")) {
    const m = line.match(/^\s*([A-Z0-9_]+)\s*=\s*(.*)\s*$/i);
    if (!m) continue;
    let val = m[2];
    if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
      val = val.slice(1, -1);
    }
    if (process.env[m[1]] === undefined) process.env[m[1]] = val;
  }
}

/**
 * Populate process.env from (in precedence order, first writer wins because we
 * never overwrite an already-set var):
 *   1. the real process environment (untouched)
 *   2. a per-worktree `harness/.env` override (optional; for one-off local tweaks)
 *   3. the shared `~/.config/neokapi/harness.env` (the normal home for GEMINI_API_KEY)
 */
export function loadEnv(): void {
  if (loaded) return;
  loaded = true;
  applyEnvFile(path.join(HARNESS_ROOT, ".env"));
  applyEnvFile(sharedEnvFile());
}
