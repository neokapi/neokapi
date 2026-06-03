import fs from "node:fs";
import path from "node:path";
import type { CaptureError, DemoCapture, DemoManifest, TimelineEvent } from "../types.ts";
import {
  KAPI_BIN,
  KAPI_ISO,
  KAPI_ISO_PLUGINS,
  REPO_ROOT,
  captureDir,
  demoFixturesDir,
  ensureDir,
  kapiIsolationEnv,
  publicDemoDir,
  rmrf,
  sandboxDir,
} from "../lib/paths.ts";
import { sh } from "../lib/exec.ts";
import { loadEnv } from "../lib/env.ts";

/**
 * Install the locally-built kapi-bowrain plugin into the harness's isolated
 * plugins dir so a bowrain CLI demo's `kapi init/push/pull/sync` resolve it under
 * full isolation (KAPI_PLUGINS_DIR_ONLY skips the user + system roots). Layout
 * matches the plugin host's discovery: <plugins>/<name>/manifest.json +
 * <plugins>/<name>/<binary-from-manifest>.
 */
function installBowrainPlugin(): void {
  const binary = path.join(REPO_ROOT, "bin", "kapi-bowrain");
  if (!fs.existsSync(binary)) {
    throw new Error(
      `kapi-bowrain plugin binary not found at ${binary} — run \`make build-bowrain-plugin\` first`,
    );
  }
  const dir = ensureDir(path.join(KAPI_ISO_PLUGINS, "bowrain"));
  fs.copyFileSync(
    path.join(REPO_ROOT, "bowrain", "cli", "cmd", "kapi-bowrain", "manifest.json"),
    path.join(dir, "manifest.json"),
  );
  const dst = path.join(dir, "kapi-bowrain");
  fs.copyFileSync(binary, dst);
  fs.chmodSync(dst, 0o755);
}

/**
 * Tidy a bowrain command's recorded output for the screencast: drop the daemon
 * start/stop chatter (`[bowrain] daemon: ready` / `daemon: signal terminated`)
 * and rewrite the absolute sandbox path (e.g. /private/tmp/…/sandbox/<id>) to the
 * demo's cosmetic cwd label, so a viewer sees `~/project`, not a temp dir.
 */
function cleanBowrainOutput(out: string, sandbox: string, cwdLabel: string): string {
  let real = sandbox;
  try {
    real = fs.realpathSync(sandbox);
  } catch {
    /* sandbox may already be gone */
  }
  return out
    .split("\n")
    .filter((line) => {
      const t = line.trim();
      // Drop the bowrain daemon + okapi-bridge startup/shutdown chatter, including
      // the bridge JVM's java.util.logging lines and netty/grpc warnings.
      return (
        !/^\[bowrain\]/.test(t) &&
        !/^\[bridge\]/.test(t) &&
        !/^daemon:/.test(t) &&
        !/^[A-Z][a-z]{2} \d{1,2}, \d{4} /.test(t) &&
        !/^(WARNING|INFO|SEVERE|FINE|CONFIG):/.test(t) &&
        !/^(at )?(io\.netty|io\.grpc|java\.|sun\.)/.test(t)
      );
    })
    .join("\n")
    .split(real)
    .join(cwdLabel)
    .split(sandbox)
    .join(cwdLabel)
    .replace(/\n{2,}/g, "\n")
    .replace(/(^\n+|\n+$)/g, "");
}

/**
 * Capture a scripted **shell** demo: no Claude. Copy the fixtures into a sandbox,
 * run each `script` command for real (via `sh -c`, so globs like `*` expand) with
 * an isolated, repo-bin-on-PATH environment, and record the real stdout into a
 * capture.json the Remotion renderer replays as a plain terminal session.
 *
 * Deterministic and unbilled — the commands are the toolbox (kcat/kgrep/ksed),
 * not an LLM. Output is captured exactly as a user would see it.
 */
export async function captureScript(m: DemoManifest, opts: { force?: boolean } = {}): Promise<void> {
  const id = m.id;
  const sb = sandboxDir(id);
  const cap = ensureDir(captureDir(id));
  const pub = ensureDir(publicDemoDir(id));
  const captureJson = path.join(pub, "capture.json");

  if (!opts.force && fs.existsSync(captureJson)) {
    console.log(`  · capture exists for ${id} (use --force to re-run)`);
    return;
  }
  if (!fs.existsSync(KAPI_BIN)) {
    throw new Error(`kapi binary not found at ${KAPI_BIN} — run \`make build\` first`);
  }

  // 1. Fresh sandbox from fixtures (the sandbox IS the project the commands act on).
  rmrf(sb);
  ensureDir(sb);
  const fixtures = demoFixturesDir(id);
  if (fs.existsSync(fixtures)) fs.cpSync(fixtures, sb, { recursive: true });

  // 2. Isolated env: repo bin on PATH (so kcat/kgrep/ksed resolve) + throwaway
  //    kapi state. Plain shell demos keep project discovery OFF (the dogfood-
  //    isolation contract). A bowrain CLI demo (brand: bowrain) instead installs
  //    the kapi-bowrain plugin into the isolated plugins dir and turns discovery
  //    ON — its sandbox lives OUTSIDE the repo (os.tmpdir()), so there is no
  //    dogfood project to leak into — taking auth + server URL from the seed
  //    (harness/.env, written by `make harness-seed`).
  const isBowrain = m.brand === "bowrain" && m.terminal === "shell";
  if (isBowrain) {
    loadEnv();
    installBowrainPlugin();
    // Wipe the plugin dispatch cache so kapi re-discovers the freshly-installed
    // manifest. The cache (XDG_CACHE_HOME/kapi/plugins-cache.json) only self-
    // invalidates on a version bump, not on manifest content, so a stale entry
    // would hide newly-added commands/flags (e.g. `kapi init --workspace`).
    rmrf(path.join(KAPI_ISO, "cache", "kapi", "plugins-cache.json"));
    if (!process.env.BOWRAIN_SESSION_TOKEN) {
      console.warn(`  ⚠ ${id}: BOWRAIN_SESSION_TOKEN not set — run \`make harness-seed\` first`);
    }
  }
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    PATH: `${path.join(REPO_ROOT, "bin")}:${process.env.PATH}`,
    ...kapiIsolationEnv(),
    ...(isBowrain
      ? {
          BOWRAIN_AUTH_TOKEN: process.env.BOWRAIN_SESSION_TOKEN ?? "",
          BOWRAIN_SERVER_URL: process.env.BOWRAIN_BACKEND_URL ?? "",
          BOWRAIN_BACKEND_URL: process.env.BOWRAIN_BACKEND_URL ?? "",
          // Isolate the bowrain config dir too — otherwise `workspace list` /
          // `auth` read the developer's real ~/.config/bowrain (server.url,
          // auth.json) instead of the seed's localhost server.
          BOWRAIN_CONFIG_DIR: ensureDir(path.join(KAPI_ISO, "bowrain-config")),
          XDG_CONFIG_HOME: ensureDir(path.join(KAPI_ISO, "xdg-config")),
          // Isolate the plugin dispatch cache (wiped above) so it never reads a
          // stale entry from the developer's real ~/.cache/kapi.
          XDG_CACHE_HOME: ensureDir(path.join(KAPI_ISO, "cache")),
        }
      : { KAPI_NO_PROJECT: "1" }),
  };

  for (const cmd of m.setup ?? []) {
    const r = await sh(cmd, { cwd: sb, env, timeoutMs: 120_000 });
    if (r.code !== 0) console.warn(`    setup command exited ${r.code}: ${r.stderr.slice(0, 300)}`);
  }

  // 3. Run each script step, recording a command + its real output.
  console.log(`  · running ${m.script?.length ?? 0} scripted command(s) in ${sb} …`);
  const events: TimelineEvent[] = [];
  const errors: CaptureError[] = [];
  let i = 0;
  const start = Date.now();
  let commandCount = 0;
  for (const step of m.script ?? []) {
    if (step.comment !== undefined) {
      events.push({ i: i++, kind: "comment", text: step.comment });
      continue;
    }
    const command = step.command;
    if (!command) continue;
    commandCount++;
    events.push({ i: i++, kind: "command", text: command });
    const r = await sh(command, { cwd: sb, env, timeoutMs: (m.captureTimeoutSec ?? 120) * 1000 });
    let out = [r.stdout, r.stderr].filter((s) => s && s.trim()).join("\n").replace(/\n+$/, "");
    if (isBowrain) out = cleanBowrainOutput(out, sb, m.cwd ?? "~/project");
    const isError = r.code !== 0;
    // grep exits 1 on "no match" — not a real error; only flag other non-zero codes.
    const realError = isError && r.code !== 1;
    if (out !== "" || isError) {
      events.push({ i: i++, kind: "output", text: out, isError: realError });
    }
    if (realError) {
      errors.push({
        tool: "shell",
        command: command.slice(0, 100),
        snippet: (r.stderr || r.stdout).split("\n").slice(0, 2).join(" ").slice(0, 200),
        hardError: true,
      });
    }
  }

  const capture: DemoCapture = {
    id,
    title: m.title,
    subtitle: m.subtitle,
    tagline: m.tagline,
    aspects: m.aspects ?? [],
    prompt: m.prompt ?? "",
    terminal: "shell",
    brand: m.brand ?? "kapi",
    cwd: m.cwd ?? "~/project",
    events,
    meta: {
      model: "shell",
      durationMs: Date.now() - start,
      numTurns: commandCount,
      capturedAt: new Date().toISOString(),
      errors,
    },
  };
  fs.writeFileSync(captureJson, JSON.stringify(capture, null, 2));

  // 4. Snapshot the final sandbox for artifact capture (before/after of edited files).
  const snapshot = path.join(cap, "sandbox");
  rmrf(snapshot);
  fs.cpSync(sb, snapshot, { recursive: true });

  console.log(`  ✓ captured ${id}: ${events.length} events, ${commandCount} commands`);
  if (errors.length) {
    console.warn(`  ⚠ ${errors.length} command error(s) — these will show in the video:`);
    for (const e of errors) console.warn(`      ✗ ${e.command} ↳ ${e.snippet}`);
  } else {
    console.log(`  ✓ clean: no command errors`);
  }
}
