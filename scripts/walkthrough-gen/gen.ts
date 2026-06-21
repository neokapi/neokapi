#!/usr/bin/env node
// scripts/walkthrough-gen/gen.ts
//
// W3 (issue #661): one authored scene spec drives BOTH the recorded VHS video
// and the interactive KapiEmbed playground, so they cannot drift.
//
// Reads:   web/walkthroughs/<id>.scene.yaml   (the single source)
// Emits:   web/scenes/<id>/0N-<scene>.tape    (VHS recording source)
//          web/src/components/KapiPlayground/embeds/<id>.embed.ts
//          web/src/components/KapiPlayground/embeds/index.ts (registry)
//          keeps web/walkthroughs/<id>.md smoke_contract: in sync
//
// Reproducible: same spec → byte-identical output. No timestamps, no random
// IDs, no machine paths.
//
// Usage:
//   node --experimental-strip-types scripts/walkthrough-gen/gen.ts <id> [<id>...]
//   node --experimental-strip-types scripts/walkthrough-gen/gen.ts --all
//   node --experimental-strip-types scripts/walkthrough-gen/gen.ts --check        # fail if any output is stale

import { readFileSync, writeFileSync, readdirSync, existsSync, mkdirSync, rmSync } from "node:fs";
import { join, dirname, resolve as pathResolve } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";
import { execFileSync } from "node:child_process";
import yaml from "js-yaml";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const REPO_ROOT = pathResolve(__dirname, "../..");

// Format a TypeScript source string through the project formatter (Oxfmt via
// `vp fmt`), so generated .ts files match what `vp fmt --check` expects and a
// re-run / --check stays idempotent. Runs the formatter on a temp file (Oxfmt
// has no stdin mode here) and returns the formatted text. Falls back to the
// unformatted input if `vp` is unavailable (e.g. a minimal CI image) — the
// content is still valid TS, just not pre-wrapped.
let FORMAT_WARNED = false;
function formatTs(content: string): string {
  const tmp = join(tmpdir(), `wt-gen-${process.pid}-${Math.random().toString(36).slice(2)}.ts`);
  try {
    writeFileSync(tmp, content);
    execFileSync("vp", ["fmt", "--write", tmp], { cwd: REPO_ROOT, stdio: "ignore" });
    return readFileSync(tmp, "utf8");
  } catch {
    if (!FORMAT_WARNED) {
      console.warn("  [warn] `vp fmt` unavailable — emitting generated .ts unformatted");
      FORMAT_WARNED = true;
    }
    return content;
  } finally {
    try {
      rmSync(tmp, { force: true });
    } catch {
      /* ignore */
    }
  }
}

const WALKTHROUGHS_DIR = join(REPO_ROOT, "web/walkthroughs");
const SCENES_DIR = join(REPO_ROOT, "web/scenes");
const EMBEDS_DIR = join(REPO_ROOT, "web/src/components/KapiPlayground/embeds");

// ── Spec schema ───────────────────────────────────────────────────────────────

interface TapeSettings {
  width: number;
  height: number;
  theme: string;
  padding: number;
  fontSize: number;
}

interface InlineFile {
  path: string;
  content: string;
}

// A step is either a visible "# comment" beat or a runnable command.
interface CommentStep {
  comment: string;
}
interface CommandStep {
  command: string;
  narration?: string;
  // offline: true (default) → runs in the wasm embed (warm, sequential session).
  // offline: false → tape + narration only; never run in the embed.
  offline?: boolean;
  // smoke: whether to include this command in the .md smoke_contract that W7
  // (scripts/verify-snippets/harness.ts) re-runs. W7 runs each command in
  // ISOLATION (fresh cwd, only the scene's library fixtures seeded), so set
  // smoke: false on steps that consume a file produced by an earlier step in
  // the sequence — they only make sense inside the warm embed session.
  // Defaults to the value of `offline`.
  smoke?: boolean;
}
type Step = CommentStep | CommandStep;

interface SceneSpec {
  id: string;
  scene: string;
  mode: "interactive" | "video";
  // Optional. When omitted on an interactive walkthrough the scene is
  // "embed-only": the in-browser KapiEmbed is the sole artifact and no VHS
  // `.tape` is emitted (kapi terminal scenes are retiring VHS in favour of the
  // interactive embeds + narrated harness explainers).
  tape?: TapeSettings;
  seed?: string[];
  files?: InlineFile[];
  steps: Step[];
  // Optional override for the offline embed when the page's primary mode is
  // video but a subset of steps is still runnable (e.g. bilingual `extract`).
  embed?: { seed?: string[]; files?: InlineFile[] };
}

function isCommand(s: Step): s is CommandStep {
  return (s as CommandStep).command !== undefined;
}

// ── Kit fixture mirror ────────────────────────────────────────────────────────
//
// Canonical content for the built-in kit fixtures a spec may `seed`. Mirrors
// packages/kapi-playground/src/fixtures.ts so the generator can materialize the
// SAME bytes into the scene dir that the embed seeds in the browser — the tape
// and the embed then run identical commands against identical files (no drift).
// W7's harness.ts keeps its own copy of this same map for the same reason.
const KIT_FIXTURES: Record<string, string> = {
  "messages.json": JSON.stringify(
    {
      greeting: "Hello, World!",
      farewell: "See you tomorrow",
      items: { cart: "Your cart is empty" },
    },
    null,
    2,
  ),
};

function seedContent(name: string): string | undefined {
  return KIT_FIXTURES[name];
}

// ── Load + validate ─────────────────────────────────────────────────────────

function loadSpec(id: string): SceneSpec {
  const specPath = join(WALKTHROUGHS_DIR, `${id}.scene.yaml`);
  if (!existsSync(specPath)) {
    throw new Error(`scene spec not found: ${specPath}`);
  }
  const spec = yaml.load(readFileSync(specPath, "utf8")) as SceneSpec;
  if (!spec || typeof spec !== "object") throw new Error(`${id}: spec is empty or not an object`);
  if (spec.id !== id) throw new Error(`${id}: spec id "${spec.id}" does not match filename`);
  if (!spec.scene) throw new Error(`${id}: missing "scene"`);
  if (spec.mode !== "interactive" && spec.mode !== "video") {
    throw new Error(
      `${id}: mode must be "interactive" or "video" (got ${JSON.stringify(spec.mode)})`,
    );
  }
  if (spec.mode === "video" && !spec.tape) {
    throw new Error(`${id}: a "video" walkthrough needs "tape" settings`);
  }
  if (!Array.isArray(spec.steps) || spec.steps.length === 0)
    throw new Error(`${id}: "steps" must be a non-empty list`);
  for (const s of spec.steps) {
    if (!isCommand(s) && (s as CommentStep).comment === undefined) {
      throw new Error(`${id}: each step must have "command" or "comment": ${JSON.stringify(s)}`);
    }
  }
  return spec;
}

/** Commands the offline embed runs (warm, sequential session). */
function offlineCommands(spec: SceneSpec): CommandStep[] {
  return spec.steps.filter(isCommand).filter((s) => s.offline !== false);
}

/**
 * Commands written to the .md smoke_contract for W7 to re-run. A command is
 * included iff it is offline AND not explicitly `smoke: false` (chained steps
 * that consume earlier output set smoke: false — W7 runs each in isolation).
 */
function smokeCommands(spec: SceneSpec): CommandStep[] {
  return offlineCommands(spec).filter((s) => s.smoke !== false);
}

// ── (a) VHS .tape generation ──────────────────────────────────────────────────
//
// Mirrors the walkthrough-scenes skill's terminal template so a regen is
// byte-identical and visually consistent with hand-written tapes.

function tapeSleepForCommand(cmd: string): string {
  // Heavier commands (translate/extract/merge/leverage/qa) get a longer beat
  // so the recording shows settled output; lookups/help are quick.
  if (/\b(extract|merge|recycle|translate|qa|term-check|tm audit)\b/.test(cmd))
    return "2500ms";
  if (/\b(pseudo-translate|tm import|termbase import)\b/.test(cmd)) return "1500ms";
  if (/\b(--help|formats|tools|stats|lookup|search|ls)\b/.test(cmd)) return "2s";
  return "1500ms";
}

function generateTape(spec: SceneSpec): string {
  const sceneNo = 1; // single-scene walkthroughs (matches current pipeline)
  const prefix = String(sceneNo).padStart(2, "0");
  const t = spec.tape;
  if (!t) throw new Error(`${spec.id}: generateTape called without "tape" settings`);
  const lines: string[] = [];
  lines.push(`# VHS tape — generated from web/walkthroughs/${spec.id}.scene.yaml`);
  lines.push(
    `# Scene ${sceneNo}: ${spec.scene} (terminal). Generated by scripts/walkthrough-gen — do not edit by hand.`,
  );
  lines.push("");
  lines.push(`Output "${prefix}-${spec.scene}.webm"`);
  lines.push("");
  lines.push(`Set FontSize ${t.fontSize}`);
  lines.push(`Set Width ${t.width}`);
  lines.push(`Set Height ${t.height}`);
  lines.push(`Set Theme "${t.theme}"`);
  lines.push(`Set Padding ${t.padding}`);
  lines.push("");

  for (const step of spec.steps) {
    if (isCommand(step)) {
      lines.push(`Type "${step.command}"`);
      lines.push("Enter");
      lines.push(`Sleep ${tapeSleepForCommand(step.command)}`);
      lines.push("");
    } else {
      lines.push(`Type "# ${step.comment}"`);
      lines.push("Enter");
      lines.push("Sleep 500ms");
      lines.push("");
    }
  }
  // Final beat — hold the last frame.
  lines.push("Sleep 1s");
  return lines.join("\n") + "\n";
}

// ── (b) Embed-config .ts generation ───────────────────────────────────────────
//
// The docs-layer KapiGuidedEmbed / GuidedSteps components import these configs.
// Each carries the seed/files plus the ordered offline commands (with
// narration) the embed runs and the guided-steps rail displays.

function tsString(s: string): string {
  return JSON.stringify(s);
}

function generateEmbedConfig(spec: SceneSpec): string {
  const cmds = offlineCommands(spec);
  const seed = spec.embed?.seed ?? spec.seed ?? [];
  const files = spec.embed?.files ?? spec.files ?? [];

  const stepLines = cmds
    .map((c) => {
      const narration = c.narration ? `, narration: ${tsString(c.narration)}` : "";
      return `    { command: ${tsString(c.command)}${narration} },`;
    })
    .join("\n");

  const seedLines = seed.length ? `[${seed.map(tsString).join(", ")}]` : "[]";

  const fileLines = files.length
    ? "[\n" +
      files
        .map((f) => `    { path: ${tsString(f.path)}, content: ${tsString(f.content)} },`)
        .join("\n") +
      "\n  ]"
    : "[]";

  return `// GENERATED by scripts/walkthrough-gen from web/walkthroughs/${spec.id}.scene.yaml.
// Do not edit by hand — change the .scene.yaml and regenerate.
import type { WalkthroughEmbedConfig } from "./types";

const config: WalkthroughEmbedConfig = {
  id: ${tsString(spec.id)},
  scene: ${tsString(spec.scene)},
  mode: ${tsString(spec.mode)},
  seed: ${seedLines},
  files: ${fileLines},
  steps: [
${stepLines}
  ],
};

export default config;
`;
}

// The shared types module + barrel registry the docs components import.
function embedTypesModule(): string {
  return `// GENERATED by scripts/walkthrough-gen. Do not edit by hand.
//
// Shared shape for the per-walkthrough embed configs. The docs-layer
// KapiGuidedEmbed component reads these to seed + script a <KapiEmbed>; the
// GuidedSteps rail renders the numbered "run the next command" list.

import type { KapiFile } from "@neokapi/kapi-playground";

export interface WalkthroughEmbedStep {
  /** A runnable kapi command, e.g. "kapi word-count messages.json". */
  command: string;
  /** Optional one-line explanation shown beside the step. */
  narration?: string;
}

export interface WalkthroughEmbedConfig {
  /** Walkthrough id (matches the .scene.yaml / .mdx). */
  id: string;
  /** Scene id (the single terminal scene). */
  scene: string;
  /** Whether the page treats the embed (interactive) or video as primary. */
  mode: "interactive" | "video";
  /** Built-in kit fixture names seeded into the cwd (see fixtures.ts). */
  seed: string[];
  /** Inline files written to the cwd before the commands run. */
  files: KapiFile[];
  /** Ordered offline commands the embed runs / the guided rail lists. */
  steps: WalkthroughEmbedStep[];
}
`;
}

function embedRegistryModule(ids: string[]): string {
  const sorted = [...ids].sort();
  const imports = sorted.map((id) => `import ${idToVar(id)} from "./${id}.embed";`).join("\n");
  const entries = sorted.map((id) => `  ${tsString(id)}: ${idToVar(id)},`).join("\n");
  return `// GENERATED by scripts/walkthrough-gen. Do not edit by hand.
//
// Registry of every walkthrough embed config, keyed by walkthrough id. The
// docs-layer KapiGuidedEmbed looks the config up by id so an .mdx page need
// only pass <KapiGuidedEmbed id="kapi-word-count" />.
import type { WalkthroughEmbedConfig } from "./types";
${imports}

export const EMBED_CONFIGS: Record<string, WalkthroughEmbedConfig> = {
${entries}
};

export type { WalkthroughEmbedConfig, WalkthroughEmbedStep } from "./types";
`;
}

function idToVar(id: string): string {
  return id
    .split(/[-_]/)
    .map((p, i) => (i === 0 ? p : p.charAt(0).toUpperCase() + p.slice(1)))
    .join("");
}

// ── (c) smoke_contract sync in the .md prompt front matter ────────────────────
//
// The W7 verifier (scripts/verify-snippets/harness.ts) reads `smoke_contract:`
// arrays from web/walkthroughs/*.md front matter. We rewrite that one
// array in place from the offline commands of the spec, preserving everything
// else byte-for-byte.

function syncSmokeContract(spec: SceneSpec): { changed: boolean; path: string } {
  const mdPath = join(WALKTHROUGHS_DIR, `${spec.id}.md`);
  if (!existsSync(mdPath)) {
    throw new Error(`${spec.id}: prompt source ${mdPath} not found — cannot sync smoke_contract`);
  }
  const original = readFileSync(mdPath, "utf8");
  const cmds = smokeCommands(spec).map((c) => c.command);

  // Locate the front matter block.
  const fmMatch = original.match(/^---\n([\s\S]*?)\n---/);
  if (!fmMatch) throw new Error(`${spec.id}: ${mdPath} has no YAML front matter`);

  // Find the `smoke_contract:` key and its indented list within the front
  // matter, then replace the whole block. We match the key line plus all
  // following lines that are list items ("- ") at a deeper indent.
  const smokeRe = /^(\s*)smoke_contract:\s*\n((?:\1\s+-\s.*\n?)*)/m;
  const sm = original.match(smokeRe);
  if (!sm) {
    throw new Error(`${spec.id}: no "smoke_contract:" block found in ${mdPath} front matter`);
  }
  const keyIndent = sm[1];
  const itemIndent = keyIndent + "  ";
  const rebuilt =
    cmds.length === 0
      ? `${keyIndent}smoke_contract: []\n`
      : `${keyIndent}smoke_contract:\n` + cmds.map((c) => `${itemIndent}- ${c}`).join("\n") + "\n";

  const updated = original.replace(smokeRe, rebuilt);
  if (updated === original) return { changed: false, path: mdPath };
  return writeOut(mdPath, updated)
    ? { changed: true, path: mdPath }
    : { changed: false, path: mdPath };
}

// ── Write helpers (idempotent) ────────────────────────────────────────────────

let CHECK_MODE = false;
const staleFiles: string[] = [];

function writeOut(path: string, content: string): boolean {
  const existing = existsSync(path) ? readFileSync(path, "utf8") : null;
  if (existing === content) return false;
  if (CHECK_MODE) {
    staleFiles.push(path.replace(REPO_ROOT + "/", ""));
    return true; // "would change"
  }
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, content);
  return true;
}

// ── Per-walkthrough generation ────────────────────────────────────────────────

/**
 * Materialize the embed's seed + inline files into the scene dir at the SAME
 * bare paths the embed uses, so `cd scenes/<id> && vhs 01-*.tape` records
 * against identical bytes. Only for interactive walkthroughs (their tapes use
 * bare filenames). Returns the relative paths written.
 */
function materializeSceneFixtures(spec: SceneSpec): { changed: boolean; written: string[] } {
  const written: string[] = [];
  let changed = false;
  const seed = spec.seed ?? [];
  const files = spec.files ?? [];
  for (const name of seed) {
    const content = seedContent(name);
    if (content === undefined) {
      throw new Error(
        `${spec.id}: seed fixture "${name}" has no content in the generator's KIT_FIXTURES mirror`,
      );
    }
    const p = join(SCENES_DIR, spec.id, name);
    if (writeOut(p, content)) changed = true;
    written.push(p.replace(REPO_ROOT + "/", ""));
  }
  for (const f of files) {
    const p = join(SCENES_DIR, spec.id, f.path);
    if (writeOut(p, f.content)) changed = true;
    written.push(p.replace(REPO_ROOT + "/", ""));
  }
  return { changed, written };
}

function generate(id: string): void {
  const spec = loadSpec(id);
  const prefix = "01";
  const tag = (b: boolean) => (b ? (CHECK_MODE ? "STALE" : "wrote") : "ok   ");
  console.log(`  ${id}  (mode: ${spec.mode})`);

  // (a) tape — generated for interactive walkthroughs (bare-filename cwd model,
  // rendered against materialized scene fixtures). For video walkthroughs the
  // recording tape is hand-curated (cd/reset/translator setup); we preserve it.
  if (spec.mode === "interactive" && spec.tape) {
    const tapePath = join(SCENES_DIR, id, `${prefix}-${spec.scene}.tape`);
    const tapeChanged = writeOut(tapePath, generateTape(spec));
    console.log(`    ${tag(tapeChanged)}  ${tapePath.replace(REPO_ROOT + "/", "")}`);

    const fx = materializeSceneFixtures(spec);
    for (const w of fx.written) {
      console.log(`    ${tag(fx.changed)}  ${w}`);
    }
  } else if (spec.mode === "interactive") {
    console.log(`    skip   tape (embed-only — no "tape:" block)`);
  } else {
    console.log(`    skip   tape (mode: video — curated recording preserved)`);
  }

  // (b) embed config — always emitted (interactive primary, or the offline
  // subset for a video walkthrough like bilingual's extract step).
  const embedPath = join(EMBEDS_DIR, `${id}.embed.ts`);
  const embedChanged = writeOut(embedPath, formatTs(generateEmbedConfig(spec)));
  console.log(`    ${tag(embedChanged)}  ${embedPath.replace(REPO_ROOT + "/", "")}`);

  // (c) smoke_contract sync in the .md prompt front matter (W7 source).
  const smoke = syncSmokeContract(spec);
  console.log(
    `    ${tag(smoke.changed)}  ${smoke.path.replace(REPO_ROOT + "/", "")} (smoke_contract: ${smokeCommands(spec).length} of ${offlineCommands(spec).length} offline cmds)`,
  );
}

function allSpecIds(): string[] {
  if (!existsSync(WALKTHROUGHS_DIR)) return [];
  return readdirSync(WALKTHROUGHS_DIR)
    .filter((f) => f.endsWith(".scene.yaml"))
    .map((f) => f.replace(/\.scene\.yaml$/, ""))
    .sort();
}

// ── Main ──────────────────────────────────────────────────────────────────────

function main(): void {
  const args = process.argv.slice(2);
  CHECK_MODE = args.includes("--check");
  const rest = args.filter((a) => a !== "--check" && a !== "--all");
  const ids = args.includes("--all") || rest.length === 0 ? allSpecIds() : rest;

  if (ids.length === 0) {
    console.error(
      "No scene specs found. Pass an <id> or create web/walkthroughs/<id>.scene.yaml.",
    );
    process.exit(1);
  }

  console.log(`walkthrough-gen${CHECK_MODE ? " (--check)" : ""}: ${ids.length} walkthrough(s)`);
  for (const id of ids) generate(id);

  // Always (re)write the shared types + registry barrel so a new spec is
  // wired up automatically.
  const typesChanged = writeOut(join(EMBEDS_DIR, "types.ts"), formatTs(embedTypesModule()));
  const registryChanged = writeOut(
    join(EMBEDS_DIR, "index.ts"),
    formatTs(embedRegistryModule(allSpecIds())),
  );
  const tag = (b: boolean) => (b ? (CHECK_MODE ? "STALE" : "wrote") : "ok   ");
  console.log(`  registry`);
  console.log(
    `    ${tag(typesChanged)}  ${join(EMBEDS_DIR, "types.ts").replace(REPO_ROOT + "/", "")}`,
  );
  console.log(
    `    ${tag(registryChanged)}  ${join(EMBEDS_DIR, "index.ts").replace(REPO_ROOT + "/", "")}`,
  );

  if (CHECK_MODE && staleFiles.length > 0) {
    console.error(`\n${staleFiles.length} generated file(s) are stale:`);
    for (const f of staleFiles) console.error(`  - ${f}`);
    console.error(`\nRun: node --experimental-strip-types scripts/walkthrough-gen/gen.ts --all`);
    process.exit(1);
  }
  console.log("\nDone.");
}

main();
