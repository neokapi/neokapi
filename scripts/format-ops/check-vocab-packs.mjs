#!/usr/bin/env node
// check-vocab-packs.mjs — CI drift gate between the canonical Go vocabulary
// packs (core/model/vocabularies/*.json) and their TypeScript-side copies.
//
// This replaces the manual vocab-drift ritual: the canonical packs are the
// Go-embedded JSON files; packages/ui (@neokapi/ui-primitives) keeps the one
// verbatim TypeScript copy, imported by its vocabularies/index.ts and re-
// exported to every other UI kit. Any divergence between the canonical pack
// and that copy is drift, and so is a copy anywhere else.
//
// Usage: check-vocab-packs.mjs [--root <repo-root>] [--json]
//
// Reports per pack and per TS location:
//   - a vocabulary directory in an unexpected place, or an expected one gone
//   - pack missing entirely in a TS location (missing-in-TS, pack level)
//   - type keys missing-in-TS / missing-in-Go
//   - value drift on shared keys (deep compare, JSON-path cited)
//   - top-level drift (every top-level key, not a fixed list)
// Exit 1 on any drift.

import fs from "node:fs";
import path from "node:path";
import { DEFAULT_ROOT, isPlainObject, parseArgs } from "./lib.mjs";

const { opts } = parseArgs(process.argv.slice(2), ["--root"], ["--json"]);
const root = opts.root ? path.resolve(opts.root) : DEFAULT_ROOT;

const goDir = path.join(root, "core", "model", "vocabularies");
if (!fs.existsSync(goDir)) {
  process.stderr.write(`error: canonical pack dir not found: ${goDir}\n`);
  process.exit(2);
}
const goPacks = fs.readdirSync(goDir).filter((f) => f.endsWith(".json")).sort();

// The packs have exactly two homes, and the pair is the point: Go embeds the
// canonical files, one TypeScript package holds the copy the UI kits import.
// A second TS copy is not a harmless duplicate — one drifted from the other in
// behaviour, not just bytes, and the two kits rendered the same span type
// differently. Adding a home means changing this list deliberately.
const GO_DIR_REL = "core/model/vocabularies";
const EXPECTED_TS_DIRS = ["packages/ui/src/vocabularies"];

const toPosix = (p) => p.split(path.sep).join("/");

// Directories that hold installed or generated trees; a pack copy under one of
// them is an artifact, not a source, and walking them is slow.
const SKIP_DIRS = new Set([
  "node_modules", ".git", "dist", "build", "out", "coverage",
  ".turbo", ".next", ".claude", "storybook-static", "vendor",
]);

// Walk the whole tree rather than a couple of known parents: a copy that lands
// somewhere unanticipated is exactly the case this must catch.
function findVocabDirs(from) {
  const packNames = new Set(goPacks);
  const found = [];
  (function walk(dir) {
    let entries;
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const e of entries) {
      if (!e.isDirectory() || SKIP_DIRS.has(e.name)) continue;
      const abs = path.join(dir, e.name);
      if (e.name === "vocabularies") {
        try {
          if (fs.readdirSync(abs).some((f) => packNames.has(f))) found.push(abs);
        } catch {
          /* unreadable — nothing to compare */
        }
      }
      walk(abs);
    }
  })(from);
  return found.map((d) => toPosix(path.relative(root, d))).sort();
}

const vocabDirs = findVocabDirs(root);
const tsDirsRel = vocabDirs.filter((d) => d !== GO_DIR_REL);
const unexpectedDirs = tsDirsRel.filter((d) => !EXPECTED_TS_DIRS.includes(d));
const missingDirs = EXPECTED_TS_DIRS.filter((d) => !tsDirsRel.includes(d));
const tsDirs = EXPECTED_TS_DIRS.filter((d) => tsDirsRel.includes(d)).map((d) =>
  path.join(root, d),
);

// Deep diff: returns list of {path, go, ts} differences.
function deepDiff(goVal, tsVal, at, out) {
  if (isPlainObject(goVal) && isPlainObject(tsVal)) {
    const keys = new Set([...Object.keys(goVal), ...Object.keys(tsVal)]);
    for (const k of keys) deepDiff(goVal[k], tsVal[k], `${at}.${k}`, out);
    return;
  }
  if (Array.isArray(goVal) && Array.isArray(tsVal)) {
    const n = Math.max(goVal.length, tsVal.length);
    for (let i = 0; i < n; i++) deepDiff(goVal[i], tsVal[i], `${at}[${i}]`, out);
    return;
  }
  if (JSON.stringify(goVal) !== JSON.stringify(tsVal)) {
    out.push({ path: at, go: goVal, ts: tsVal });
  }
}

const findings = [];
let drift = false;

for (const dir of unexpectedDirs) {
  findings.push({ location: dir, unexpected_location: true });
  drift = true;
}
for (const dir of missingDirs) {
  findings.push({ location: dir, missing_location: true });
  drift = true;
}

for (const pack of goPacks) {
  const goJson = JSON.parse(fs.readFileSync(path.join(goDir, pack), "utf8"));
  for (const tsDir of tsDirs) {
    const rel = path.relative(root, tsDir);
    const tsFile = path.join(tsDir, pack);
    const finding = { pack, location: rel, missing_in_ts: [], missing_in_go: [], value_drift: [] };

    if (!fs.existsSync(tsFile)) {
      finding.pack_missing_in_ts = true;
      findings.push(finding);
      drift = true;
      continue;
    }

    let tsJson;
    try {
      tsJson = JSON.parse(fs.readFileSync(tsFile, "utf8"));
    } catch (e) {
      finding.value_drift.push({ path: "(file)", go: "valid JSON", ts: `unparseable: ${e.message}` });
      findings.push(finding);
      drift = true;
      continue;
    }

    const goTypes = isPlainObject(goJson.types) ? goJson.types : {};
    const tsTypes = isPlainObject(tsJson.types) ? tsJson.types : {};
    for (const k of Object.keys(goTypes)) if (!(k in tsTypes)) finding.missing_in_ts.push(k);
    for (const k of Object.keys(tsTypes)) if (!(k in goTypes)) finding.missing_in_go.push(k);
    for (const k of Object.keys(goTypes)) {
      if (k in tsTypes) deepDiff(goTypes[k], tsTypes[k], `types.${k}`, finding.value_drift);
    }
    // Every top-level key, so a key added on one side only is drift too.
    for (const top of new Set([...Object.keys(goJson), ...Object.keys(tsJson)])) {
      if (top === "types") continue;
      deepDiff(goJson[top], tsJson[top], top, finding.value_drift);
    }

    if (finding.missing_in_ts.length || finding.missing_in_go.length || finding.value_drift.length) {
      drift = true;
    }
    findings.push(finding);
  }
}

// Also flag TS-side packs with no canonical Go counterpart.
for (const tsDir of tsDirs) {
  for (const f of fs.readdirSync(tsDir).filter((f) => f.endsWith(".json"))) {
    if (!goPacks.includes(f)) {
      findings.push({ pack: f, location: path.relative(root, tsDir), pack_missing_in_go: true });
      drift = true;
    }
  }
}

if (opts.json) {
  process.stdout.write(JSON.stringify({ generated_at: new Date().toISOString(), drift, findings }, null, 2) + "\n");
} else {
  for (const f of findings) {
    if (f.unexpected_location) {
      process.stdout.write(
        `DRIFT ${f.location}: unexpected vocabulary copy. The packs live in exactly two places — ` +
          `${GO_DIR_REL} (Go embed, canonical) and ${EXPECTED_TS_DIRS.join(", ")} (the TypeScript copy). ` +
          `Fix: delete ${f.location} and import { getDefaultRegistry } from "@neokapi/ui-primitives" instead. ` +
          `If a new home is genuinely intended, add it to EXPECTED_TS_DIRS in scripts/format-ops/check-vocab-packs.mjs.\n`,
      );
      continue;
    }
    if (f.missing_location) {
      process.stdout.write(
        `DRIFT ${f.location}: expected vocabulary copy is missing. ` +
          `Fix: restore it with \`cp ${GO_DIR_REL}/*.json ${f.location}/\`, ` +
          `or drop it from EXPECTED_TS_DIRS in scripts/format-ops/check-vocab-packs.mjs if it is gone on purpose.\n`,
      );
      continue;
    }
    const head = `${f.pack} @ ${f.location}`;
    if (f.pack_missing_in_ts) {
      process.stdout.write(
        `DRIFT ${head}: pack missing-in-TS (canonical ${GO_DIR_REL}/${f.pack} has no copy here). ` +
          `Fix: cp ${GO_DIR_REL}/${f.pack} ${f.location}/${f.pack}\n`,
      );
      continue;
    }
    if (f.pack_missing_in_go) {
      process.stdout.write(
        `DRIFT ${head}: pack missing-in-Go (no canonical ${GO_DIR_REL}/${f.pack}). ` +
          `Fix: add the pack to ${GO_DIR_REL}/ (it is the source the Go binary embeds), or delete ${f.location}/${f.pack}\n`,
      );
      continue;
    }
    const issues = [];
    if (f.missing_in_ts.length) issues.push(`missing-in-TS types: ${f.missing_in_ts.join(", ")}`);
    if (f.missing_in_go.length) issues.push(`missing-in-Go types: ${f.missing_in_go.join(", ")}`);
    for (const v of f.value_drift) {
      issues.push(`value drift at ${v.path}: go=${JSON.stringify(v.go)} ts=${JSON.stringify(v.ts)}`);
    }
    if (issues.length === 0) {
      process.stdout.write(`OK    ${head}\n`);
    } else {
      for (const i of issues) process.stdout.write(`DRIFT ${head}: ${i}\n`);
      process.stdout.write(
        `      Fix: cp ${GO_DIR_REL}/${f.pack} ${f.location}/${f.pack} ` +
          `(Go is canonical; edit the pack there and copy outward)\n`,
      );
    }
  }
  process.stdout.write(drift ? "check-vocab-packs: FAIL (drift detected)\n" : "check-vocab-packs: OK\n");
}
process.exit(drift ? 1 : 0);
