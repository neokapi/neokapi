#!/usr/bin/env node
// check-vocab-packs.mjs — CI drift gate between the canonical Go vocabulary
// packs (core/model/vocabularies/*.json) and their TypeScript-side copies.
//
// This replaces the manual vocab-drift ritual: the canonical packs are the
// Go-embedded JSON files; the UI packages keep verbatim JSON copies under
// packages/*/src/vocabularies/ and bowrain/packages/*/src/vocabularies/
// (imported by their index.ts). Any divergence between the canonical pack
// and a copy is drift.
//
// Usage: check-vocab-packs.mjs [--root <repo-root>] [--json]
//
// Reports per pack and per TS location:
//   - pack missing entirely in a TS location (missing-in-TS, pack level)
//   - type keys missing-in-TS / missing-in-Go
//   - value drift on shared keys (deep compare, JSON-path cited)
//   - top-level drift (version / extends / entity_prefix / fallback)
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

// Discover TS-side copy locations: any src/vocabularies/ dir holding pack
// JSON under packages/ or bowrain/packages/.
function discoverTsDirs() {
  const found = [];
  for (const base of ["packages", path.join("bowrain", "packages")]) {
    const abs = path.join(root, base);
    if (!fs.existsSync(abs)) continue;
    for (const pkg of fs.readdirSync(abs, { withFileTypes: true })) {
      if (!pkg.isDirectory()) continue;
      const vocabDir = path.join(abs, pkg.name, "src", "vocabularies");
      if (fs.existsSync(vocabDir) && fs.readdirSync(vocabDir).some((f) => f.endsWith(".json"))) {
        found.push(vocabDir);
      }
    }
  }
  return found.sort();
}
const tsDirs = discoverTsDirs();
if (tsDirs.length === 0) {
  process.stderr.write("error: no TS-side vocabulary copy directories found under packages/ or bowrain/packages/\n");
  process.exit(2);
}

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
    for (const top of ["name", "version", "extends", "entity_prefix", "fallback"]) {
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
    const head = `${f.pack} @ ${f.location}`;
    if (f.pack_missing_in_ts) {
      process.stdout.write(`DRIFT ${head}: pack missing-in-TS (canonical core/model/vocabularies/${f.pack} has no copy here)\n`);
      continue;
    }
    if (f.pack_missing_in_go) {
      process.stdout.write(`DRIFT ${head}: pack missing-in-Go (no canonical core/model/vocabularies/${f.pack})\n`);
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
    }
  }
  process.stdout.write(drift ? "check-vocab-packs: FAIL (drift detected)\n" : "check-vocab-packs: OK\n");
}
process.exit(drift ? 1 : 0);
