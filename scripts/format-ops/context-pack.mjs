#!/usr/bin/env node
// context-pack.mjs — generate the per-format context pack (Knowledge axis K3).
//
// Contract: docs/internals/format-maturity.md §2.4 — "a context pack
// generates cleanly (scripts/format-ops/context-pack.mjs <id> joins
// dossier + spec.yaml + vocabulary.yaml + corpus.yaml + relevant section
// files into one schema-checked artifact — the standard input to
// implement-format and case-gen)".
//
// Usage: context-pack.mjs <format-id> [--out <path>] [--root <repo-root>]
//
// Inputs are schema-checked; a missing artifact is a named error stating
// which Knowledge level the absence caps:
//   - dossier.yaml missing            -> caps Knowledge at K0
//   - spec.yaml missing               -> caps Knowledge at K1
//     (harvest formats may substitute the okapi_skip/invariants/corpus
//     ladder for spec.yaml — noted, not an error, when those tests exist)
//   - vocabulary.yaml / corpus.yaml missing -> caps Knowledge at K2
//     (the K3 pack cannot join all four artifacts)
//
// Output: one markdown artifact on stdout (or --out <path>) with sections
// Identity, Spec sources, Implementations, Vocabulary matrix summary,
// Corpus summary, Executable spec summary, Known divergences — plus the
// contents of any specs/sections/ files referenced by dossier citations.

import fs from "node:fs";
import path from "node:path";
import {
  DEFAULT_ROOT,
  HARVEST_FORMATS,
  isPlainObject,
  loadYamlFile,
  parseArgs,
} from "./lib.mjs";

const { opts, positional } = parseArgs(process.argv.slice(2), ["--out", "--root"], []);
const root = opts.root ? path.resolve(opts.root) : DEFAULT_ROOT;

if (positional.length !== 1) {
  process.stderr.write("usage: context-pack.mjs <format-id> [--out <path>] [--root <repo-root>]\n");
  process.exit(2);
}
const id = positional[0];
const fmtDir = path.join(root, "core", "formats", id);
if (!fs.existsSync(fmtDir)) {
  process.stderr.write(`error: no such format dir: core/formats/${id}/\n`);
  process.exit(2);
}

// ── Load + schema-check inputs ──────────────────────────────────────────────
const errors = [];

function loadArtifact(name) {
  const file = path.join(fmtDir, name);
  if (!fs.existsSync(file)) return { file, doc: null, exists: false };
  try {
    return { file, doc: loadYamlFile(file), exists: true };
  } catch (e) {
    errors.push(`core/formats/${id}/${name}: unparseable YAML: ${e.message}`);
    return { file, doc: null, exists: true };
  }
}

const dossier = loadArtifact("dossier.yaml");
const vocabulary = loadArtifact("vocabulary.yaml");
const corpus = loadArtifact("corpus.yaml");
const spec = loadArtifact("spec.yaml");

const harvestLadderPresent =
  HARVEST_FORMATS.includes(id) &&
  fs.existsSync(path.join(fmtDir, "okapi_skip_test.go")) &&
  fs.existsSync(path.join(fmtDir, "invariants_test.go"));

if (!dossier.exists) {
  errors.push(`missing core/formats/${id}/dossier.yaml — caps Knowledge at K0 (K1 requires the dossier)`);
}
if (!spec.exists && !harvestLadderPresent) {
  errors.push(`missing core/formats/${id}/spec.yaml — caps Knowledge at K1 (K2 requires the executable spec or, for harvest formats, the okapi_skip/invariants/corpus ladder)`);
}
if (!vocabulary.exists) {
  errors.push(`missing core/formats/${id}/vocabulary.yaml — caps Knowledge at K2 (the K3 context pack joins all four axis artifacts)`);
}
if (!corpus.exists) {
  errors.push(`missing core/formats/${id}/corpus.yaml — caps Knowledge at K2 (the K3 context pack joins all four axis artifacts)`);
}

// Minimal field schema checks on artifacts that do exist.
if (dossier.doc != null) {
  if (!isPlainObject(dossier.doc)) {
    errors.push(`dossier.yaml: must be a mapping`);
  } else {
    const sources = dossier.doc.spec_sources ?? dossier.doc.specs;
    if (!Array.isArray(sources) || sources.length === 0) {
      errors.push(`dossier.yaml: needs a non-empty "spec_sources" list ({id, version, url, watch?}) — K1 requires >= 1 versioned spec source`);
    } else {
      sources.forEach((s, i) => {
        if (!isPlainObject(s) || !s.id || s.version == null || !s.url) {
          errors.push(`dossier.yaml: spec_sources[${i}] must carry {id, version, url}`);
        }
      });
    }
    if (!Array.isArray(dossier.doc.implementations)) {
      errors.push(`dossier.yaml: needs an "implementations" list — K1 requires the implementations table`);
    }
  }
}
if (vocabulary.doc != null && !isPlainObject(vocabulary.doc)) {
  errors.push(`vocabulary.yaml: must be a mapping (per-construct matrix)`);
}
if (corpus.doc != null) {
  const entries = Array.isArray(corpus.doc) ? corpus.doc : corpus.doc?.entries;
  if (!Array.isArray(entries)) {
    errors.push(`corpus.yaml: needs an "entries" list (path/tier/sha256/origin per entry)`);
  }
}
if (spec.doc != null && isPlainObject(spec.doc)) {
  if (!spec.doc.format) errors.push(`spec.yaml: missing "format" id`);
  if (!Array.isArray(spec.doc.features)) errors.push(`spec.yaml: missing "features" list`);
}

if (errors.length > 0) {
  for (const e of errors) process.stderr.write(`error: ${e}\n`);
  process.stderr.write(`context-pack ${id}: FAIL (${errors.length} error(s))\n`);
  process.exit(1);
}

// ── Helpers ────────────────────────────────────────────────────────────────
const lines = [];
const emit = (s = "") => lines.push(s);

function mdTable(headers, rows) {
  if (rows.length === 0) return ["_none_"];
  const esc = (v) => String(v ?? "").replace(/\|/g, "\\|").replace(/\n/g, " ");
  return [
    `| ${headers.join(" | ")} |`,
    `|${headers.map(() => "---").join("|")}|`,
    ...rows.map((r) => `| ${r.map(esc).join(" | ")} |`),
  ];
}

// Collect structured citations from the dossier (cite/citations keys) to
// find referenced specs/sections/ files.
function collectDossierCitations(node, out) {
  if (Array.isArray(node)) return node.forEach((v) => collectDossierCitations(v, out));
  if (!isPlainObject(node)) return;
  for (const [key, value] of Object.entries(node)) {
    if (key === "cite" || key === "citations" || key === "cites" || key === "spec_refs") {
      const items = Array.isArray(value) ? value : [value];
      for (const item of items) if (isPlainObject(item)) out.push(item);
    } else {
      collectDossierCitations(value, out);
    }
  }
}

function sectionFilesFor(cit) {
  // specs/sections/<spec>/<version>/<anchor>.md
  if (!cit.spec || cit.version == null) return [];
  const dir = path.join(root, "specs", "sections", String(cit.spec), String(cit.version));
  if (!fs.existsSync(dir)) return [];
  const fragment = typeof cit.url === "string" && cit.url.includes("#") ? cit.url.split("#").pop() : null;
  const files = fs.readdirSync(dir).filter((f) => f.endsWith(".md"));
  if (fragment) {
    const hit = files.filter((f) => f.replace(/\.md$/, "") === fragment);
    if (hit.length > 0) return hit.map((f) => path.join(dir, f));
  }
  if (cit.clause) {
    const slug = String(cit.clause).toLowerCase().replace(/[^a-z0-9]+/g, "-");
    const hit = files.filter((f) => f.replace(/\.md$/, "").toLowerCase().includes(slug));
    return hit.map((f) => path.join(dir, f));
  }
  return [];
}

// ── Build the pack ──────────────────────────────────────────────────────────
const d = dossier.doc;
const today = new Date().toISOString().slice(0, 10);

emit(`# Context pack: ${id}`);
emit();
emit(`Generated ${today} by scripts/format-ops/context-pack.mjs from core/formats/${id}/.`);
emit();

// Identity
emit(`## Identity`);
emit();
emit(`- Format id: \`${id}\` (\`core/formats/${id}/\`)`);
if (spec.doc?.format) emit(`- Spec format id: \`${spec.doc.format}\``);
if (spec.doc?.mime_type) emit(`- MIME type: \`${spec.doc.mime_type}\``);
emit(`- Family: ${HARVEST_FORMATS.includes(id) ? "harvest (no Okapi counterpart; parity is `na`)" : "parity (Okapi counterpart exists)"}`);
const description = d.description ?? spec.doc?.description;
if (description) {
  emit();
  emit(String(description).trim());
}
emit();

// Spec sources
emit(`## Spec sources`);
emit();
const sources = d.spec_sources ?? d.specs ?? [];
mdTable(
  ["id", "version", "url", "watch"],
  sources.map((s) => [s.id, s.version, s.url, s.watch ?? ""]),
).forEach(emit);
emit();

// Referenced spec sections (specs/sections/ files cited by the dossier).
const dossierCitations = [];
collectDossierCitations(d, dossierCitations);
const sectionFiles = [...new Set(dossierCitations.flatMap(sectionFilesFor))];
if (sectionFiles.length > 0) {
  emit(`### Referenced spec sections`);
  emit();
  for (const f of sectionFiles) {
    emit(`#### ${path.relative(root, f)}`);
    emit();
    emit(fs.readFileSync(f, "utf8").trim());
    emit();
  }
}

// Implementations
emit(`## Implementations`);
emit();
const impls = Array.isArray(d.implementations) ? d.implementations : [];
mdTable(
  ["name", "repo", "license", "watch", "notes"],
  impls.map((im) =>
    isPlainObject(im)
      ? [im.name ?? im.id ?? "", im.repo ?? im.url ?? "", im.license ?? "", im.watch ?? "", im.notes ?? ""]
      : [String(im), "", "", "", ""],
  ),
).forEach(emit);
emit();

// Vocabulary matrix summary
emit(`## Vocabulary matrix summary`);
emit();
{
  const v = vocabulary.doc;
  const constructs = Array.isArray(v.constructs) ? v.constructs : isPlainObject(v.constructs) ? Object.entries(v.constructs).map(([k, val]) => ({ id: k, ...(isPlainObject(val) ? val : {}) })) : [];
  const tally = { total: constructs.length, expressible: 0, read: {}, write: {}, unknown: 0, evidenced: 0 };
  for (const c of constructs) {
    if (c.expressible !== false) tally.expressible++;
    for (const cell of ["read", "write"]) {
      const val = c[cell]?.status ?? c[cell];
      if (typeof val === "string") {
        tally[cell][val] = (tally[cell][val] ?? 0) + 1;
        if (val === "unknown") tally.unknown++;
      }
    }
    if (c.evidence || c.read?.evidence || c.write?.evidence) tally.evidenced++;
  }
  emit(`- Constructs: ${tally.total} (${tally.expressible} expressible)`);
  emit(`- Read cells: ${Object.entries(tally.read).map(([k, n]) => `${k}: ${n}`).join(", ") || "none claimed"}`);
  emit(`- Write cells: ${Object.entries(tally.write).map(([k, n]) => `${k}: ${n}`).join(", ") || "none claimed"}`);
  emit(`- Cells with evidence bindings: ${tally.evidenced}; unknown cells: ${tally.unknown}`);
}
emit();

// Corpus summary
emit(`## Corpus summary`);
emit();
{
  const entries = Array.isArray(corpus.doc) ? corpus.doc : corpus.doc.entries;
  const byTier = {};
  const byOrigin = {};
  const licenses = new Set();
  let bytes = 0;
  for (const e of entries) {
    byTier[e.tier ?? "?"] = (byTier[e.tier ?? "?"] ?? 0) + 1;
    byOrigin[e.origin ?? "?"] = (byOrigin[e.origin ?? "?"] ?? 0) + 1;
    if (e.license) licenses.add(e.license);
    bytes += e.size ?? 0;
  }
  emit(`- Entries: ${entries.length} (${(bytes / 1024).toFixed(1)} KiB total)`);
  emit(`- Tiers: ${Object.entries(byTier).map(([k, n]) => `${k}: ${n}`).join(", ")}`);
  emit(`- Origins: ${Object.entries(byOrigin).map(([k, n]) => `${k}: ${n}`).join(", ")}`);
  emit(`- Licenses: ${[...licenses].sort().join(", ") || "n/a"}`);
}
emit();

// Executable spec summary
emit(`## Executable spec summary`);
emit();
if (spec.doc) {
  const features = spec.doc.features ?? [];
  let examples = 0;
  let xfails = 0;
  for (const f of features) {
    examples += (f.examples ?? []).length;
    for (const ex of f.examples ?? []) if (ex.expected_fail) xfails++;
    if (f.expected_fail) xfails++;
  }
  emit(`- \`spec.yaml\`: ${features.length} features, ${examples} examples, ${xfails} expected_fail(s)`);
  const cfg = spec.doc.config ?? [];
  emit(`- Config keys: ${cfg.map((c) => `\`${c.key}\``).join(", ") || "none"}`);
} else {
  emit(`- No \`spec.yaml\`: harvest ladder in place (okapi_skip_test.go + invariants_test.go + corpus tests).`);
}
emit();

// Known divergences
emit(`## Known divergences`);
emit();
{
  const rows = [];
  if (spec.doc) {
    for (const f of spec.doc.features ?? []) {
      for (const ex of f.examples ?? []) {
        if (ex.expected_fail) {
          rows.push([f.name ?? "", ex.name ?? "", ex.divergence_kind ?? "(none)", ex.tracking ?? ex.issue ?? ""]);
        }
      }
    }
  }
  for (const div of d.divergences ?? []) {
    if (isPlainObject(div)) rows.push([div.feature ?? "", div.case ?? div.name ?? "", div.divergence_kind ?? div.kind ?? "(none)", div.tracking ?? div.issue ?? ""]);
  }
  mdTable(["feature", "case", "divergence_kind", "tracking"], rows).forEach(emit);
  if (d.divergence_ledger) {
    emit();
    emit(`Divergence ledger: ${d.divergence_ledger}`);
  }
}
emit();

const out = lines.join("\n") + "\n";
if (opts.out) {
  fs.writeFileSync(path.resolve(opts.out), out);
  process.stderr.write(`context-pack ${id}: OK -> ${opts.out} (${lines.length} lines)\n`);
} else {
  process.stdout.write(out);
  process.stderr.write(`context-pack ${id}: OK (${lines.length} lines)\n`);
}
