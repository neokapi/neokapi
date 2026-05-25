import fs from "node:fs";
import path from "node:path";
import type { CaptureError, DemoCapture, DemoManifest, TimelineEvent } from "../types.ts";
import {
  KAPI_BIN,
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

  // 2. Isolated env: repo bin on PATH (so kcat/kgrep/ksed resolve), throwaway kapi
  //    state, and project discovery off (the dogfood-isolation contract).
  const env = {
    ...process.env,
    PATH: `${path.join(REPO_ROOT, "bin")}:${process.env.PATH}`,
    KAPI_NO_PROJECT: "1",
    ...kapiIsolationEnv(),
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
    const out = [r.stdout, r.stderr].filter((s) => s && s.trim()).join("\n").replace(/\n+$/, "");
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
