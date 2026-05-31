/**
 * One-time environment setup for the harness:
 *  - build the kapi binary (with icu4c on PKG_CONFIG_PATH) if missing
 *  - regenerate the Claude Code plugin skills bundle
 *  - register the harness-gemini credential from .env GEMINI_API_KEY
 *  - install the Playwright Chromium browser
 */
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { loadEnv } from "../lib/env.ts";
import { KAPI_BIN, KAPI_ISO_DATA, KAPI_ISO_HOME, KAPI_ISO_PLUGINS, PLUGIN_DIR, REPO_ROOT, ensureDir } from "../lib/paths.ts";
import { run, sh } from "../lib/exec.ts";

const CRED = "harness-gemini";

/**
 * Give the harness its own isolated kapi state so recordings don't depend on this
 * machine's installed plugins / flows / TMs. The okapi-bridge plugin is copied from
 * the machine install if present (fast), else installed fresh into the isolated dir.
 */
async function setupIsolatedKapi(): Promise<void> {
  ensureDir(KAPI_ISO_HOME);
  ensureDir(KAPI_ISO_PLUGINS);
  const bridgeDst = path.join(KAPI_ISO_PLUGINS, "okapi-bridge");
  if (fs.existsSync(bridgeDst)) {
    console.log(`✓ isolated okapi-bridge present (${KAPI_ISO_PLUGINS})`);
    return;
  }
  const machineBridge = path.join(os.homedir(), ".local/share/kapi/plugins/okapi-bridge");
  if (fs.existsSync(machineBridge)) {
    fs.cpSync(machineBridge, bridgeDst, { recursive: true });
    console.log(`✓ copied okapi-bridge into isolated plugin dir (${KAPI_ISO_PLUGINS})`);
    return;
  }
  console.log("· installing okapi-bridge into isolated plugin dir …");
  const r = await run(KAPI_BIN, ["plugin", "install", "okapi-bridge", "-y"], {
    env: { ...process.env, XDG_DATA_HOME: KAPI_ISO_DATA, KAPI_CONFIG_DIR: KAPI_ISO_HOME, KAPI_PLUGINS_DIR: KAPI_ISO_PLUGINS },
    timeoutMs: 600_000,
  });
  if (r.code !== 0) console.warn(`! okapi-bridge install exited ${r.code}: ${r.stderr.slice(-300)}`);
  else console.log("✓ installed okapi-bridge (isolated)");
}

async function buildKapiIfMissing(): Promise<void> {
  if (fs.existsSync(KAPI_BIN)) {
    console.log(`✓ kapi present: ${KAPI_BIN}`);
    return;
  }
  console.log("· building kapi (cgo + icu4c) …");
  // Find an icu4c pkgconfig dir from Homebrew.
  const icu = await sh("brew --prefix icu4c 2>/dev/null");
  const pkgPath = icu.code === 0 ? `${icu.stdout.trim()}/lib/pkgconfig` : "/opt/homebrew/opt/icu4c/lib/pkgconfig";
  // -tags fts5 is required by mattn/go-sqlite3 for the termbase/TM full-text search.
  const r = await sh(`mkdir -p "${REPO_ROOT}/bin" && cd "${REPO_ROOT}/kapi" && go build -tags fts5 -o "${REPO_ROOT}/bin/kapi" ./cmd/kapi`, {
    env: { ...process.env, PKG_CONFIG_PATH: `${pkgPath}:${process.env.PKG_CONFIG_PATH ?? ""}` },
    timeoutMs: 600_000,
  });
  if (r.code !== 0) throw new Error(`kapi build failed:\n${r.stderr.slice(-1200)}`);
  console.log(`✓ built ${KAPI_BIN}`);
}

async function regenPluginBundle(): Promise<void> {
  const skillsDir = path.join(PLUGIN_DIR, "skills");
  fs.rmSync(skillsDir, { recursive: true, force: true });
  fs.mkdirSync(skillsDir, { recursive: true });
  const r = await run(KAPI_BIN, ["skills", "export", "--dir", skillsDir], { timeoutMs: 60_000 });
  if (r.code !== 0) throw new Error(`skills export failed: ${r.stderr}`);
  console.log(`✓ plugin bundle: ${skillsDir}`);
}

async function ensureCredential(): Promise<void> {
  const key = process.env.GEMINI_API_KEY;
  if (!key) {
    console.warn("! GEMINI_API_KEY not set (add it to ~/.config/neokapi/harness.env) — AI demos will fail. Offline demos still work.");
    return;
  }
  const list = await run(KAPI_BIN, ["credentials", "list", "--json"], { timeoutMs: 30_000 });
  let exists = false;
  try {
    exists = JSON.parse(list.stdout).credentials?.some((c: any) => c.name === CRED);
  } catch {
    /* ignore */
  }
  if (exists) {
    console.log(`✓ credential "${CRED}" already present`);
    return;
  }
  const r = await run(KAPI_BIN, ["credentials", "add", CRED, "--provider", "gemini", "--api-key", key], { timeoutMs: 30_000 });
  if (r.code !== 0) throw new Error(`credentials add failed: ${r.stderr}`);
  console.log(`✓ registered kapi credential "${CRED}" (gemini)`);
}

async function installChromium(): Promise<void> {
  console.log("· installing Playwright Chromium (if needed) …");
  const r = await sh("npx --yes playwright install chromium", { cwd: path.join(REPO_ROOT, "harness"), timeoutMs: 600_000 });
  if (r.code !== 0) console.warn(`! playwright install exited ${r.code} — artifact capture may fail`);
  else console.log("✓ Playwright Chromium ready");
}

async function main() {
  loadEnv();
  await buildKapiIfMissing();
  await regenPluginBundle();
  await ensureCredential();
  await setupIsolatedKapi();
  await installChromium();
  console.log("\nSetup complete. Run a demo with:  pnpm run demo <demo-id>   (or: pnpm run demo all)");
}

main().catch((e) => {
  console.error("\nsetup failed:", e.message);
  process.exit(1);
});
