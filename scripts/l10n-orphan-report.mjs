#!/usr/bin/env node
//
// Report the seed entries that produced nothing this cycle.
//
// The committed content memory under `.kapi/memory/` is human-owned source: a
// reviewed pair per string. Source strings come and go — a screen is redesigned,
// a fixture leak is plugged (#1791) — and when one goes, the pair that
// translated it stops matching anything. It is not deleted. Review is expensive
// and a string that comes back should come back already translated, so an
// entry outlives the source that motivated it, and the seeds accumulate wording
// no reader will ever see.
//
// That accumulation is only safe while it is visible. Content memory is keyed
// by text, not by call site, so an orphan is not inert: the moment any surface
// anywhere in the recipe emits the same source string, `recycle` fills it with
// this entry's target as reviewed wording. An orphan carrying a spelling the
// project has since retired is therefore a spelling waiting to be re-approved.
//
// So this reports; it never gates. An orphan is not an error, and a run with
// hundreds of them is not a broken build — target-language standing is reported
// and never gated (CLAUDE.md, "Target-language drift"). The report exists so a
// reviewer reads the list and decides, which is a thing a human should do and a
// build should not.
//
// An entry counts as producing something when its target text appears anywhere
// in the artifacts the translate stage materialized for that locale. Comparison
// is over whitespace-normalized text, because a markdown writer re-wraps a
// paragraph the memory holds as one line. Matching is by substring, so the
// report under-reports rather than over-reports — the safe direction for a list
// a human is asked to act on.
//
// Usage:
//     node scripts/l10n-orphan-report.mjs <lang> <root>...
//
// Run it through `make l10n-orphans`, which knows the roots and runs the four
// stages first — the target artifacts have to exist for the question to mean
// anything.

import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, resolve } from "node:path";

const [lang, ...roots] = process.argv.slice(2);

if (!lang || roots.length === 0) {
  console.error("usage: l10n-orphan-report.mjs <lang> <root>...");
  process.exit(2);
}

const SEED_DIR = ".kapi/memory";

/** Collapse every whitespace run to one space, so wrapping is not a difference. */
function normalize(text) {
  return text.replace(/\s+/g, " ").trim();
}

/** Every string value in a parsed JSON document, in document order. */
function jsonStrings(node, out) {
  if (typeof node === "string") out.push(node);
  else if (Array.isArray(node)) for (const v of node) jsonStrings(v, out);
  else if (node !== null && typeof node === "object")
    for (const v of Object.values(node)) jsonStrings(v, out);
  return out;
}

function filesUnder(path) {
  let st;
  try {
    st = statSync(path);
  } catch {
    return [];
  }
  if (st.isFile()) return [path];
  return readdirSync(path).flatMap((name) => filesUnder(join(path, name)));
}

// The corpus: one normalized string per artifact. A catalog is JSON whose
// values are the text (and whose escapes would defeat a raw-text search), so it
// is read through its own structure; everything else — markdown, YAML, HTML —
// is read as the text it is.
function corpusOf(paths) {
  const chunks = [];
  for (const path of paths) {
    let raw;
    try {
      raw = readFileSync(path, "utf8");
    } catch {
      continue;
    }
    if (path.endsWith(".json")) {
      try {
        chunks.push(normalize(jsonStrings(JSON.parse(raw), []).join("\n")));
        continue;
      } catch {
        // Not parseable as JSON — fall through and search it as text.
      }
    }
    chunks.push(normalize(raw));
  }
  return chunks;
}

function flatten(runs) {
  return (runs ?? []).map((r) => r.text ?? "").join("");
}

const artifacts = roots.flatMap((r) => filesUnder(resolve(r)));
const corpus = corpusOf(artifacts);

const seeds = readdirSync(SEED_DIR)
  .filter((f) => f.endsWith(".memory.json"))
  .sort();

let totalEntries = 0;
let totalOrphans = 0;
const report = [];

for (const seed of seeds) {
  const bundle = JSON.parse(readFileSync(join(SEED_DIR, seed), "utf8"));
  const orphans = [];
  let considered = 0;
  for (const entry of bundle.entries ?? []) {
    const target = entry.variants?.[lang];
    if (target === undefined) continue;
    considered++;
    const needle = normalize(flatten(target));
    if (needle === "") continue;
    if (!corpus.some((c) => c.includes(needle))) {
      orphans.push({ source: normalize(flatten(entry.variants?.en)), target: needle });
    }
  }
  totalEntries += considered;
  totalOrphans += orphans.length;
  if (orphans.length > 0) report.push({ seed, considered, orphans });
}

console.log(
  `Orphan standing for ${lang}: ${totalOrphans} of ${totalEntries} seed entries ` +
    `produced nothing across ${artifacts.length} generated artifacts.`,
);

for (const { seed, considered, orphans } of report) {
  console.log(`\n${SEED_DIR}/${seed} — ${orphans.length} of ${considered}`);
  for (const { source, target } of orphans.slice(0, 20)) {
    console.log(`    ${JSON.stringify(source)} → ${JSON.stringify(target)}`);
  }
  if (orphans.length > 20) console.log(`    … and ${orphans.length - 20} more`);
}

if (totalOrphans > 0) {
  console.log(
    `\nAn orphan is reviewed wording with nothing left to translate. It is kept, ` +
      `not deleted — but content memory matches on text, so any surface that ` +
      `emits the same source string will be filled from it. Read the list; ` +
      `retire an entry only when its wording should never come back.`,
  );
}

// Never gates. Deliberately: see the header.
process.exit(0);
