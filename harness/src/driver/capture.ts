import fs from "node:fs";
import path from "node:path";
import type { DemoManifest } from "../types.ts";
import {
  KAPI_BIN,
  PLUGIN_DIR,
  REPO_ROOT,
  captureDir,
  demoFixturesDir,
  ensureDir,
  kapiIsolationEnv,
  publicDemoDir,
  rmrf,
  sandboxDir,
} from "../lib/paths.ts";
import { run, sh } from "../lib/exec.ts";
import { normalizeTranscript } from "./normalize.ts";

const CRED = "harness-gemini";

/**
 * A minimal CLAUDE.md dropped into every sandbox. It gives only the environment context
 * a real user's setup would carry — NOT a kapi how-to. Teaching kapi is the skill's job;
 * the demos exist to show that an ordinary prompt + the skill is enough.
 */
function sandboxClaudeMd(m: DemoManifest): string {
  // Environment facts only — NOT a kapi how-to. No "use kapi", no example commands;
  // deciding to reach for kapi and how to drive it is the skill's job (that's what the
  // demos prove). We state the available credential because it's setup the assistant
  // can't discover, and that tooling is preinstalled so it doesn't waste a step.
  const ai = m.needsAi
    ? `\nA Gemini model credential named \`${CRED}\` is configured; use it (` +
      `\`--credential ${CRED}\`) whenever a tool needs a model provider.`
    : ``;
  const note = m.claudeNote ? `\n${m.claudeNote.trim()}` : ``;
  return `# Environment

This is a real project — work in this directory and briefly say what you're doing as
you go. The command-line tooling is already installed and on your PATH; no \`npm install\`
is needed to run it.${ai}${note}
`;
}

export interface CaptureOptions {
  model?: string;
  force?: boolean;
}

/**
 * Re-derive public/<id>/capture.json from the already-saved raw transcript, without
 * re-running Claude. Useful after changing the normalizer (e.g. new noise filters).
 */
export function renormalizeDemo(m: DemoManifest): void {
  const transcriptPath = path.join(captureDir(m.id), "transcript.jsonl");
  if (!fs.existsSync(transcriptPath)) {
    console.warn(`  ! no saved transcript for ${m.id} — capture it first`);
    return;
  }
  const capture = normalizeTranscript(transcriptPath, m);
  fs.writeFileSync(path.join(ensureDir(publicDemoDir(m.id)), "capture.json"), JSON.stringify(capture, null, 2));
  console.log(`  ✓ re-normalized ${m.id}: ${capture.events.length} events, ${capture.meta.errors.length} error(s)`);
}

/**
 * Prepare a fresh sandbox from the demo fixtures, run the real headless `claude`
 * against the demo prompt with the kapi plugin loaded, capture the stream-json
 * transcript, and normalize it into public/<id>/capture.json.
 */
export async function captureDemo(m: DemoManifest, opts: CaptureOptions = {}): Promise<void> {
  const id = m.id;
  const sb = sandboxDir(id);
  const cap = ensureDir(captureDir(id));
  const pub = ensureDir(publicDemoDir(id));
  const transcriptPath = path.join(cap, "transcript.jsonl");
  const captureJson = path.join(pub, "capture.json");

  if (!opts.force && fs.existsSync(captureJson)) {
    console.log(`  · capture exists for ${id} (use --force to re-run)`);
    return;
  }

  // 1. Fresh sandbox from fixtures.
  rmrf(sb);
  ensureDir(sb);
  const fixtures = demoFixturesDir(id);
  if (fs.existsSync(fixtures)) {
    fs.cpSync(fixtures, sb, { recursive: true });
  }
  // If the fixture ships its own CLAUDE.md (a real project's conventions),
  // keep it and append only the harness env context; otherwise write the
  // generic env note.
  const sandboxClaudeMdPath = path.join(sb, "CLAUDE.md");
  const fixtureClaudeMd = fs.existsSync(sandboxClaudeMdPath) ? fs.readFileSync(sandboxClaudeMdPath, "utf8").trim() + "\n\n" : "";
  fs.writeFileSync(sandboxClaudeMdPath, fixtureClaudeMd + sandboxClaudeMd(m));

  // 2. Optional setup commands (seed a termbase, init a project, npm install, …).
  // Isolated kapi state (own plugins/home) so demos don't depend on the machine.
  const env = {
    ...process.env,
    PATH: `${path.join(REPO_ROOT, "bin")}:${process.env.PATH}`,
    ...kapiIsolationEnv(),
  };
  for (const cmd of m.setup ?? []) {
    console.log(`  · setup: ${cmd}`);
    const r = await sh(cmd, { cwd: sb, env, timeoutMs: 300_000 });
    if (r.code !== 0) {
      console.warn(`    setup command exited ${r.code}: ${r.stderr.slice(0, 400)}`);
    }
  }

  if (!fs.existsSync(KAPI_BIN)) {
    throw new Error(`kapi binary not found at ${KAPI_BIN} — run \`npm run setup\` (or \`make build\`) first`);
  }

  // 3. Run the real headless claude session, streaming JSON to the transcript file.
  const model = opts.model ?? m.model ?? "sonnet";
  const args = [
    "-p",
    m.prompt,
    "--output-format",
    "stream-json",
    "--verbose",
    // Surface Stop-hook firing in the stream so the kapi verify hook (block →
    // fix → pass) is recorded and can be shown in the video.
    "--include-hook-events",
    "--permission-mode",
    "bypassPermissions",
    "--model",
    model,
  ];
  if (m.mcp) {
    // MCP path: register kapi as an MCP server and forbid Bash so Claude must use
    // the kapi MCP tools rather than shelling out to the CLI.
    const mcpConfig = path.join(sb, ".mcp-config.json");
    fs.writeFileSync(
      mcpConfig,
      JSON.stringify({
        mcpServers: {
          kapi: { command: KAPI_BIN, args: ["mcp"], env: { PATH: env.PATH!, ...kapiIsolationEnv() } },
        },
      }),
    );
    args.push("--mcp-config", mcpConfig, "--disallowedTools", "Bash");
  } else {
    // --strict-mcp-config (with no --mcp-config) disables any MCP servers the user's
    // global config would otherwise inject (e.g. a playwright browser), so the demo
    // shows only the CLI workflow, not the assistant poking at a browser.
    args.push(
      "--plugin-dir",
      PLUGIN_DIR,
      "--strict-mcp-config",
      "--allowedTools",
      "Bash Read Write Edit MultiEdit Glob Grep Skill TodoWrite",
    );
  }
  console.log(`  · running claude (${model}) in ${sb} …`);
  const out = fs.createWriteStream(transcriptPath);
  const timeoutMs = (m.captureTimeoutSec ?? 360) * 1000;
  const res = await run("claude", args, {
    cwd: sb,
    env,
    input: "",
    timeoutMs,
    onStdout: (chunk) => out.write(chunk),
  });
  out.end();
  await new Promise<void>((r) => out.on("finish", () => r()));

  if (res.timedOut) {
    console.warn(`  ! claude timed out after ${timeoutMs / 1000}s for ${id} (using partial transcript)`);
  }
  const lineCount = fs.readFileSync(transcriptPath, "utf8").split("\n").filter(Boolean).length;
  if (lineCount === 0) {
    fs.writeFileSync(path.join(cap, "stderr.log"), res.stderr);
    throw new Error(`claude produced no transcript for ${id} (exit ${res.code}). stderr:\n${res.stderr.slice(0, 800)}`);
  }
  fs.writeFileSync(path.join(cap, "stderr.log"), res.stderr);

  // 4. Normalize → public/<id>/capture.json.
  const capture = normalizeTranscript(transcriptPath, m);
  fs.writeFileSync(captureJson, JSON.stringify(capture, null, 2));

  // 5. Snapshot the final sandbox state for artifact capture + inspection.
  const snapshot = path.join(cap, "sandbox");
  rmrf(snapshot);
  fs.cpSync(sb, snapshot, { recursive: true });

  console.log(
    `  ✓ captured ${id}: ${capture.events.length} events, ${capture.meta.numTurns} turns, ` +
      `${(capture.meta.durationMs / 1000).toFixed(1)}s, $${capture.meta.costUsd?.toFixed(3) ?? "?"}`,
  );

  // Record-time audit: surface kapi/tool failures NOW, not at evaluation time.
  const errs = capture.meta.errors;
  if (errs.length) {
    console.warn(`  ⚠ ${errs.length} kapi/tool error(s) in this capture — these will show in the video:`);
    for (const e of errs.slice(0, 10)) {
      console.warn(`      ✗ [${e.hardError ? "error" : "pattern"}] ${e.command.slice(0, 100)}`);
      console.warn(`         ↳ ${e.snippet}`);
    }
    console.warn(`    Consider re-capturing (--force) after steering the prompt/CLAUDE.md away from the failing command.`);
  } else {
    console.log(`  ✓ clean: no kapi/tool errors detected`);
  }
}
