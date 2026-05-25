import fs from "node:fs";
import path from "node:path";
import { HARNESS_ROOT } from "./paths.ts";

let loaded = false;

/** Load harness/.env into process.env (without overwriting existing vars). */
export function loadEnv(): void {
  if (loaded) return;
  loaded = true;
  const file = path.join(HARNESS_ROOT, ".env");
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
