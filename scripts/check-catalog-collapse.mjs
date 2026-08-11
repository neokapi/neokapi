#!/usr/bin/env node
//
// Guard: a committed target-locale catalog may not regenerate to nothing.
//
// The four stages are a deterministic function of source plus the committed
// context under `.kapi/`, and the loop trusts that: a same-repo pull request
// commits the regeneration back rather than failing (scripts/l10n-autofix.sh).
// So whatever stage 4 writes is what ships, and a stage-3 run that yields no
// target catalogs at all writes `{}` over every locale — a total erasure that
// looks exactly like a legitimate regeneration to every downstream check.
//
// Partial coverage is not this. A source change that outruns its translations
// leaves a catalog with fewer entries, and that is the normal drift the loop
// exists to absorb (CLAUDE.md, "Target-language drift must never block the
// build"). This asserts one thing only: a catalog that carried entries, whose
// locale the committed content memory still carries pairs for, did not come
// back empty. Coverage is never the question; existence is.
//
// The baseline is HEAD, so the comparison is against what is committed on the
// branch being regenerated — the same baseline the auto-fix would commit over.
//
// Usage:
//     node scripts/check-catalog-collapse.mjs <lang>... -- <derived path>...
//
// Run it through `make l10n`, which regenerates first and knows both lists.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { basename } from "node:path";

const argv = process.argv.slice(2);
const split = argv.indexOf("--");
const langs = split === -1 ? [] : argv.slice(0, split);
const derivedPaths = split === -1 ? [] : argv.slice(split + 1);

if (langs.length === 0 || derivedPaths.length === 0) {
  console.error("usage: check-catalog-collapse.mjs <lang>... -- <derived path>...");
  process.exit(2);
}

const SEED_DIR = ".kapi/memory";

function git(args) {
  return execFileSync("git", args, { encoding: "utf8", maxBuffer: 64 * 1024 * 1024 });
}

/** Every string value in a parsed JSON document — the leaves are the translations. */
function countStrings(node) {
  if (typeof node === "string") return node === "" ? 0 : 1;
  if (Array.isArray(node)) return node.reduce((n, v) => n + countStrings(v), 0);
  if (node !== null && typeof node === "object")
    return Object.values(node).reduce((n, v) => n + countStrings(v), 0);
  return 0;
}

function countInJSON(raw) {
  try {
    return countStrings(JSON.parse(raw));
  } catch {
    // A catalog that no longer parses is a different failure, and l10n-verify
    // reports it as drift. Not this guard's question.
    return null;
  }
}

/** Reviewed pairs the committed content memory holds for a locale. */
function seedPairs(lang) {
  let seeds;
  try {
    seeds = git(["ls-files", "--", `${SEED_DIR}/*.memory.json`]).split("\n").filter(Boolean);
  } catch {
    return 0;
  }
  let total = 0;
  for (const seed of seeds) {
    let bundle;
    try {
      bundle = JSON.parse(readFileSync(seed, "utf8"));
    } catch {
      continue;
    }
    for (const entry of bundle.entries ?? []) {
      const runs = entry.variants?.[lang];
      if (runs?.some((r) => (r.text ?? "") !== "")) total++;
    }
  }
  return total;
}

const collapsed = [];
const checked = [];

for (const lang of langs) {
  // With no reviewed pairs for a locale there is nothing to lose, and an empty
  // catalog is the honest output rather than an erasure.
  const pairs = seedPairs(lang);
  if (pairs === 0) {
    console.log(`${lang}: no reviewed pairs in ${SEED_DIR}; nothing to assert.`);
    continue;
  }

  const tracked = git(["ls-files", "--", ...derivedPaths])
    .split("\n")
    .filter((p) => basename(p) === `${lang}.json`);

  for (const path of tracked) {
    // A catalog that is tracked but absent from HEAD is new in this commit, so
    // there is no baseline it could have lost entries against.
    let committed;
    try {
      committed = git(["show", `HEAD:${path}`]);
    } catch {
      continue;
    }
    const before = countInJSON(committed);
    if (before === null || before === 0) continue;
    const after = existsSync(path) ? countInJSON(readFileSync(path, "utf8")) : 0;
    if (after === null) continue;
    checked.push(path);
    if (after === 0) collapsed.push({ path, lang, before, pairs });
  }
}

if (collapsed.length === 0) {
  console.log(
    `✓ no target-locale catalog collapsed (${checked.length} carrying entries at HEAD).`,
  );
  process.exit(0);
}

console.error("Regeneration emptied catalogs that carry translations at HEAD:\n");
for (const { path, lang, before, pairs } of collapsed) {
  console.error(`  ${path}: ${before} entries → 0 (${SEED_DIR} holds ${pairs} ${lang} pairs)`);
}
console.error(
  "\nThis is not translation drift — drift leaves a smaller catalog, not an empty\n" +
    "one. The reviewed pairs are still committed, so stage 3 produced no target\n" +
    "catalogs for this locale: check that `kapi run tm-recycle` resolved the project\n" +
    "store `.kapi/work/store.db` and the collections kapi.yaml declares.\n",
);
process.exit(1);
