/**
 * neokapi-i18n compile — flatten translated blocks into runtime dicts.
 *
 * Accepts any of three input shapes:
 *
 *   1. A single `.kbf.json` file.
 *   2. A directory of `.kbf.json` files (the default output of
 *      `neokapi-i18n extract`).
 *   3. NDJSON block records on stdin (pass `-` as input) — for
 *      one-shot pipelines.
 *
 * Output: one `<locale>.json` dictionary per target locale, shape
 * `{ "<block.hash>": "<flattened target>" }`.
 *
 * A target that does not carry its source's placeholders is left out. The
 * runtime falls back to source for a key it cannot find, so omitting one costs
 * a sentence its translation; shipping one costs the sentence its count, its
 * name or its link, and says nothing about having done so. Pending target-language
 * work is the ordinary state of this loop — a missing entry reads as exactly
 * that, and the next convergence fills it once the translation is sound.
 */

import { readFileSync, writeFileSync, mkdirSync, readdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";

import type { Block, File } from "@neokapi/kapi-format";
import { flattenRuns, isKbfPath } from "@neokapi/kapi-format";

import { buildReviewManifest } from "../review/manifest.ts";

interface BlockRecord {
  block: Block;
}

/**
 * The placeholder tokens a flattened string carries.
 *
 * Read off the flattened form rather than the runs so the comparison is made
 * in the shape that actually ships: `flattenRuns` is the runtime's own
 * projection, and this asks whether what a reader will be handed still has the
 * holes their values go into.
 *
 * The same expression `scripts/check-derived-content.mjs` gates on, so the
 * writer and the gate cannot disagree about what a placeholder is — the shape
 * of bug this exists to stop is one side quietly meaning something else.
 */
const TOKEN = /\{\/?=?[A-Za-z_][A-Za-z0-9_.-]*\}|%[-+ #0]*[0-9.*]*[sdvqxXofFeEgGtTcpbU%]/g;

function tokenCounts(text: string): Map<string, number> {
  const counts = new Map<string, number>();
  for (const [tok] of text.matchAll(TOKEN)) {
    counts.set(tok, (counts.get(tok) ?? 0) + 1);
  }
  return counts;
}

/**
 * Whether a target may ship: it carries its source's placeholders, each the
 * same number of times.
 *
 * Counted, not just present: a plural whose target reuses one token where the
 * source had two still loses a value. Extra tokens fail too — a token the
 * source never had renders as a literal brace to a reader, which is how a
 * translator's typo reaches production looking like markup.
 */
function carriesItsPlaceholders(sourceText: string, targetText: string): boolean {
  const want = tokenCounts(sourceText);
  const got = tokenCounts(targetText);
  if (want.size !== got.size) return false;
  for (const [tok, n] of want) {
    if (got.get(tok) !== n) return false;
  }
  return true;
}

export async function runCompile(args: string[]) {
  const inputs: string[] = [];
  const locales: string[] = [];
  let outDir = "public/translations";
  let review = false;

  for (let i = 0; i < args.length; i++) {
    const flag = args[i];
    const value = args[i + 1];
    if (flag === "--help" || flag === "-h") {
      console.log(usage);
      return;
    }
    if (flag === "--out" && value) outDir = args[++i];
    else if (flag === "--locale" && value) locales.push(args[++i]);
    else if (flag === "--review") review = true;
    else if (!flag.startsWith("--")) inputs.push(flag);
  }

  if (inputs.length === 0) {
    console.error("error: missing input (.kbf.json file, .kbf.json directory, or - for stdin)\n");
    console.log(usage);
    process.exit(1);
  }

  // Accumulate blocks across every input so one invocation can span the source
  // catalog (i18n/src/) and each per-locale target tree (i18n/{lang}/) — a single
  // recursive i18n/ dir covers them all (needed for a complete review manifest).
  const blocks: BlockRecord[] = [];
  const declaredSet = new Set<string>();
  for (const input of inputs) {
    const res = await loadBlocks(input);
    blocks.push(...res.blocks);
    for (const l of res.declaredTargets) declaredSet.add(l);
  }
  const declaredTargets = Array.from(declaredSet);

  // Infer the set of target locales when --locale wasn't passed.
  const targetLocales = new Set<string>(locales);
  if (targetLocales.size === 0) {
    for (const l of declaredTargets) targetLocales.add(l);
    for (const { block } of blocks) {
      for (const l of Object.keys(block.targets ?? {})) targetLocales.add(l);
    }
  }

  if (targetLocales.size === 0) {
    console.error("error: input has no target locales; pass --locale explicitly");
    process.exit(1);
  }

  mkdirSync(outDir, { recursive: true });

  let totalCompiled = 0;
  for (const locale of targetLocales) {
    const dict: Record<string, string> = {};
    const holes: string[] = [];
    for (const { block } of blocks) {
      const runs = block.targets?.[locale];
      if (!runs || runs.length === 0) continue;
      const text = flattenRuns(runs);
      if (!carriesItsPlaceholders(flattenRuns(block.source), text)) {
        holes.push(block.hash);
        continue;
      }
      dict[block.hash] = text;
    }
    const outPath = join(outDir, `${locale}.json`);
    mkdirSync(dirname(outPath), { recursive: true });
    writeFileSync(outPath, `${JSON.stringify(dict, null, 2)}\n`);
    console.log(`Compiled ${Object.keys(dict).length} entries → ${outPath}`);
    if (holes.length > 0) {
      // Named, not merely counted: this is a translation that exists and was
      // not shipped, which is a different thing from one nobody has written
      // yet, and the difference is only actionable if the keys are sayable.
      console.warn(
        `  ${holes.length} target(s) left out of ${locale}: they do not carry their source's placeholders`,
      );
      console.warn(`  ${holes.slice(0, 10).join(" ")}${holes.length > 10 ? " …" : ""}`);
    }
    totalCompiled += Object.keys(dict).length;
  }

  if (totalCompiled === 0) {
    console.warn("warning: no translated blocks found for any target locale");
  }

  // Read-only in-context review manifest (source + all targets + annotations),
  // consumed by @neokapi/i18n-react/review/hosted on the deployed site.
  if (review) {
    const manifest = buildReviewManifest(inputs);
    const reviewPath = join(outDir, "review.json");
    mkdirSync(dirname(reviewPath), { recursive: true });
    writeFileSync(reviewPath, `${JSON.stringify(manifest, null, 2)}\n`);
    console.log(`Review manifest: ${Object.keys(manifest).length} block(s) → ${reviewPath}`);
  }
}

async function loadBlocks(input: string): Promise<{
  blocks: BlockRecord[];
  declaredTargets: string[];
}> {
  if (input === "-") return loadBlocksFromStdin();
  const stat = statSync(input);
  if (stat.isDirectory()) return loadBlocksFromKBFDir(input);
  return loadBlocksFromKBF(input);
}

function loadBlocksFromKBF(path: string): {
  blocks: BlockRecord[];
  declaredTargets: string[];
} {
  const raw = readFileSync(path, "utf-8");
  const file = JSON.parse(raw) as File;
  const blocks: BlockRecord[] = [];
  for (const doc of file.documents ?? []) {
    for (const block of doc.blocks ?? []) blocks.push({ block });
  }
  // KBF's Project doesn't declare target locales explicitly — infer
  // from block.targets below.
  return { blocks, declaredTargets: [] };
}

function loadBlocksFromKBFDir(dir: string): {
  blocks: BlockRecord[];
  declaredTargets: string[];
} {
  const blocks: BlockRecord[] = [];
  const declared = new Set<string>();
  walkKBFs(dir, (path) => {
    const res = loadBlocksFromKBF(path);
    blocks.push(...res.blocks);
    for (const l of res.declaredTargets) declared.add(l);
  });
  return { blocks, declaredTargets: Array.from(declared) };
}

// Depth-first in byte order at every level, so the order blocks enter a
// dictionary (and so the key order of the file written) is a function of the
// catalog paths alone and never of the order a filesystem lists them in.
function walkKBFs(dir: string, visit: (path: string) => void) {
  const entries = readdirSync(dir, { withFileTypes: true }).sort((a, b) =>
    Buffer.compare(Buffer.from(a.name), Buffer.from(b.name)),
  );
  for (const entry of entries) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) walkKBFs(path, visit);
    else if (entry.isFile() && isKbfPath(path)) visit(path);
  }
}

async function loadBlocksFromStdin(): Promise<{
  blocks: BlockRecord[];
  declaredTargets: string[];
}> {
  const chunks: Buffer[] = [];
  for await (const chunk of process.stdin) {
    if (Buffer.isBuffer(chunk)) chunks.push(chunk);
    else if (typeof chunk === "string") chunks.push(Buffer.from(chunk, "utf8"));
    else chunks.push(Buffer.from(chunk as unknown as ArrayBuffer));
  }
  const text = Buffer.concat(chunks).toString("utf8");
  const blocks: BlockRecord[] = [];
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed.startsWith("{")) continue;
    const rec = JSON.parse(trimmed) as { type: string; block?: Block };
    if (rec.type === "block" && rec.block) blocks.push({ block: rec.block });
  }
  return { blocks, declaredTargets: [] };
}

const usage = `
neokapi-i18n compile — flatten translated blocks into runtime dictionaries.

Usage:
  neokapi-i18n compile <input>... [--locale <lang>]... [--out <dir>] [--review]

<input> can be (one or more):
  <dir/>               a directory of .kbf.json files (recursive)
  <file.kbf.json>      a single .kbf.json file
  -                    NDJSON block records on stdin

Options:
  --locale <lang>   Emit a dictionary for this locale (repeat for multiple).
                    If omitted, every locale present in block.targets is
                    emitted.
  --out <dir>       Output directory (default: public/translations)
  --review          Also emit review.json — a read-only in-context review
                    manifest (source + all targets + annotations, merged by
                    hash) for @neokapi/i18n-react/review/hosted. Point it at the
                    i18n/ tree, which is walked recursively so it picks up the
                    source catalog (i18n/src/) and every per-locale target
                    (i18n/{lang}/) in one pass, e.g.
                    neokapi-i18n compile i18n --out public/translations --review
`;
