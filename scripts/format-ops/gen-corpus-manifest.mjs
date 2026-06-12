#!/usr/bin/env node
// gen-corpus-manifest.mjs — generate/refresh core/formats/<id>/corpus.yaml
// from disk truth (docs/internals/format-maturity.md §2.5).
//
// Usage: gen-corpus-manifest.mjs <format-id|--all> [--check] [--root <repo-root>]
//
// Behavior:
//   - Walks core/formats/<id>/testdata/ — every file becomes a Tier A entry
//     (origin: vendored) with sha256 + size computed from disk. SOURCES.md
//     files and dotfiles are documentation, not corpus entries.
//   - Migrates legacy testdata/**/SOURCES.md provenance (source repo,
//     pinned commit, license, fetch command) into the manifest fields
//     source_url / license / notes — the SOURCES.md prose is thereafter
//     *generated from* the manifest as the human-readable view.
//   - On refresh, merges by `path`: hand-curated fields (tier, origin,
//     source_url, license, redistributable, creator_tool, harvest_date,
//     notes) are preserved from the existing manifest; sha256 + size are
//     always recomputed from disk. Entries whose path is outside this
//     format's testdata/ (Tier B/C, corpus/ paths, origin: bug) are
//     preserved untouched.
//   - When a testdata/**/SOURCES.md existed, it is regenerated from the
//     manifest (scoped to the entries under its directory).
//   - --check: exit 1 if the committed manifest is out of sync with disk
//     (missing manifest, missing/orphan entries, sha256/size drift);
//     writes nothing.
//
// Entry fields (rubric §2.5): path (root-relative), tier A|B|C, sha256,
// size, origin vendored|url|archive-member|bug|generated, source_url,
// license (SPDX), redistributable, creator_tool, harvest_date, notes.

import fs from "node:fs";
import path from "node:path";
import {
  DEFAULT_ROOT,
  dumpYaml,
  isPlainObject,
  loadYamlFile,
  parseArgs,
  realFormatDirs,
  sha256,
  walkFiles,
} from "./lib.mjs";

const { opts, positional } = parseArgs(process.argv.slice(2), ["--root"], ["--all", "--check"]);
const root = opts.root ? path.resolve(opts.root) : DEFAULT_ROOT;

if (!opts.all && positional.length !== 1) {
  process.stderr.write("usage: gen-corpus-manifest.mjs <format-id|--all> [--check] [--root <repo-root>]\n");
  process.exit(2);
}
const formats = opts.all ? realFormatDirs(root) : [positional[0]];
if (!opts.all && !fs.existsSync(path.join(root, "core", "formats", formats[0]))) {
  process.stderr.write(`error: no such format dir: core/formats/${formats[0]}/\n`);
  process.exit(2);
}

const SPDX_RE = /\b(MIT|BSD-3-Clause|BSD-2-Clause|Apache-2\.0|CC0-1\.0|CC-BY-4\.0|ISC|MPL-2\.0|Unlicense|0BSD)\b/;

// ── SOURCES.md migration parser ─────────────────────────────────────────────
// The legacy files vary (per-file `## \`name\`` sections, markdown tables,
// free prose), so this is a best-effort field extractor: per local
// filename it recovers source_url (preferring the exact fetch URL),
// license (SPDX token), pinned commit, and a short note. Anything it
// cannot recover stays for hand-curation in the backfill phase.
function parseSourcesMd(file) {
  const text = fs.readFileSync(file, "utf8");
  // Join backslash line continuations so multi-line curl commands parse.
  const joined = text.replace(/\\\n\s*/g, " ");
  const perFile = new Map(); // basename -> {source_url, license, commit, repo, note}
  const get = (name) => {
    if (!perFile.has(name)) perFile.set(name, {});
    return perFile.get(name);
  };

  const docLicense = (joined.match(/licen[cs]ed?[^.\n]*?\b(MIT|BSD-3-Clause|BSD-2-Clause|Apache-2\.0|CC0-1\.0)\b/i) ??
    [])[1];

  // Variable assignments (e.g. I18N_SHA=abc...) for ${VAR} substitution.
  const vars = {};
  for (const m of joined.matchAll(/^\s*([A-Z][A-Z0-9_]*)=([^\s]+)\s*$/gm)) vars[m[1]] = m[2];
  const subst = (s) => s.replace(/\$\{?([A-Z][A-Z0-9_]*)\}?/g, (_, v) => vars[v] ?? `\${${v}}`);

  // Fetch commands: curl ... -o <name> ... <url>  (either order).
  for (const m of joined.matchAll(/curl[^\n]*/g)) {
    const line = m[0];
    const out = line.match(/-o\s+([^\s"']+)/)?.[1];
    const url = line.match(/(https?:\/\/[^\s"']+)/)?.[1];
    if (out && url) get(path.basename(out)).source_url = subst(url);
  }

  // Per-file `## \`name\`` sections.
  const sections = joined.split(/^## /m).slice(1);
  for (const section of sections) {
    const heading = section.split("\n")[0];
    const names = [...heading.matchAll(/`([^`]+)`/g)].map((m) => m[1]).filter((n) => n.includes("."));
    if (names.length === 0) continue;
    const repo = section.match(/Source repo:\s*<?(https?:\/\/[^\s>]+)>?/)?.[1];
    const license = section.match(/License:\s*`?([A-Za-z0-9.+-]+)`?/)?.[1];
    const commit = section.match(/[Cc]ommit[^:\n]*:\s*`?([0-9a-f]{7,40})`?/)?.[1];
    for (const name of names) {
      const e = get(name);
      if (repo) e.repo = repo;
      if (license && SPDX_RE.test(license)) e.license = license;
      if (commit) e.commit = commit;
    }
  }

  // Markdown tables with a backticked local-file first column.
  const lines = text.split("\n");
  for (let i = 0; i < lines.length; i++) {
    if (!/^\|.*\|\s*$/.test(lines[i])) continue;
    const headers = lines[i].split("|").map((c) => c.trim().toLowerCase());
    if (!headers.some((h) => h.includes("file"))) continue;
    const licCol = headers.findIndex((h) => h.includes("license"));
    const repoCol = headers.findIndex((h) => h.includes("repo"));
    for (let j = i + 2; j < lines.length && /^\|.*\|\s*$/.test(lines[j]); j++) {
      const cells = lines[j].split("|").map((c) => c.trim());
      const name = cells.find((c) => /^`[^`]+`$/.test(c))?.replaceAll("`", "");
      if (!name || !name.includes(".")) continue;
      const e = get(name);
      if (licCol > 0 && SPDX_RE.test(cells[licCol] ?? "")) e.license = cells[licCol].match(SPDX_RE)[1];
      if (repoCol > 0 && cells[repoCol]) e.repo = cells[repoCol].replaceAll("`", "");
    }
  }

  // Document-wide pinned commits ("`org/repo` — `sha`").
  const repoCommits = new Map();
  for (const m of joined.matchAll(/`([\w./-]+\/[\w.-]+)`\s*[—-]+\s*`([0-9a-f]{7,40})`/g)) {
    repoCommits.set(m[1], m[2]);
  }

  for (const [name, e] of perFile) {
    if (!e.license && docLicense) e.license = docLicense;
    if (!e.commit && e.repo) {
      for (const [r, sha] of repoCommits) if (e.repo.includes(r)) e.commit = sha;
    }
    const noteBits = [];
    if (e.repo) noteBits.push(`vendored from ${e.repo}`);
    if (e.commit) noteBits.push(`commit ${e.commit}`);
    noteBits.push(`migrated from ${path.basename(path.dirname(file))}/SOURCES.md`);
    e.note = noteBits.join("; ");
    void name;
  }
  return perFile;
}

// ── Manifest generation per format ──────────────────────────────────────────
const ENTRY_FIELD_ORDER = [
  "path",
  "tier",
  "origin",
  "sha256",
  "size",
  "source_url",
  "license",
  "redistributable",
  "creator_tool",
  "harvest_date",
  "notes",
];
const PRESERVED_FIELDS = [
  "tier",
  "origin",
  "source_url",
  "license",
  "redistributable",
  "creator_tool",
  "harvest_date",
  "notes",
];

function orderEntry(e) {
  const out = {};
  for (const k of ENTRY_FIELD_ORDER) if (k in e && e[k] !== undefined) out[k] = e[k];
  for (const k of Object.keys(e)) if (!(k in out) && e[k] !== undefined) out[k] = e[k];
  return out;
}

function buildManifest(id, existing) {
  const fmtRel = path.join("core", "formats", id);
  const testdataDir = path.join(root, fmtRel, "testdata");
  const files = walkFiles(testdataDir).filter(
    (f) => path.basename(f) !== "SOURCES.md" && !path.basename(f).startsWith("."),
  );

  // Legacy SOURCES.md provenance, merged across all SOURCES.md under testdata/.
  const sourcesFiles = walkFiles(testdataDir)
    .filter((f) => path.basename(f) === "SOURCES.md")
    .map((f) => path.join(testdataDir, f));
  const migrated = new Map();
  for (const sf of sourcesFiles) {
    for (const [name, fields] of parseSourcesMd(sf)) migrated.set(name, fields);
  }

  const existingByPath = new Map();
  const existingEntries = Array.isArray(existing) ? existing : (existing?.entries ?? []);
  for (const e of existingEntries) if (isPlainObject(e) && e.path) existingByPath.set(e.path, e);

  const entries = [];
  for (const rel of files) {
    const entryPath = `${fmtRel}/testdata/${rel}`.split(path.sep).join("/");
    const abs = path.join(testdataDir, rel);
    const buf = fs.readFileSync(abs);
    const entry = { path: entryPath, tier: "A", origin: "vendored", sha256: sha256(buf), size: buf.length };

    const prior = existingByPath.get(entryPath);
    if (prior) {
      for (const f of PRESERVED_FIELDS) if (f in prior) entry[f] = prior[f];
    } else {
      const mig = migrated.get(path.basename(rel));
      if (mig) {
        if (mig.source_url) entry.source_url = mig.source_url;
        else if (mig.repo) entry.source_url = mig.repo;
        if (mig.license) entry.license = mig.license;
        entry.redistributable = true;
        if (mig.note) entry.notes = mig.note;
      } else {
        // Own-created repo fixture: covered by the repository license.
        entry.license = "Apache-2.0";
        entry.redistributable = true;
        entry.notes = "own-created repo fixture";
      }
    }
    entries.push(orderEntry(entry));
  }

  // Preserve entries living outside this format's committed testdata
  // (Tier B/C fetched corpus paths, origin: bug minimized files, …).
  const testdataPrefix = `${fmtRel}/testdata/`.split(path.sep).join("/");
  for (const e of existingEntries) {
    if (isPlainObject(e) && e.path && !e.path.startsWith(testdataPrefix)) entries.push(orderEntry(e));
  }

  entries.sort((a, b) => a.path.localeCompare(b.path));
  return { entries, sourcesFiles, testdataDir };
}

function manifestYaml(id, generatedAt, entries) {
  const header =
    `# Corpus manifest for core/formats/${id} (docs/internals/format-maturity.md §2.5).\n` +
    `# Generated by scripts/format-ops/gen-corpus-manifest.mjs; sha256/size are disk\n` +
    `# truth, the other fields are hand-curated and preserved on refresh.\n`;
  return header + dumpYaml({ format: id, generated_at: generatedAt, entries });
}

function sourcesMdFromManifest(id, dir, entries) {
  const relDir = path.relative(root, dir).split(path.sep).join("/");
  const scoped = entries.filter((e) => e.path.startsWith(`${relDir}/`));
  const lines = [
    `# ${id} corpus — provenance`,
    "",
    "Generated from `corpus.yaml` by `scripts/format-ops/gen-corpus-manifest.mjs` —",
    "do not edit by hand; edit the manifest fields instead.",
    "",
    "| File | Tier | Origin | License | Source | sha256 |",
    "|---|---|---|---|---|---|",
    ...scoped.map(
      (e) =>
        `| \`${path.basename(e.path)}\` | ${e.tier} | ${e.origin} | ${e.license ?? ""} | ${e.source_url ?? ""} | \`${e.sha256.slice(0, 12)}…\` |`,
    ),
    "",
    ...scoped.filter((e) => e.notes).map((e) => `- \`${path.basename(e.path)}\`: ${e.notes}`),
  ];
  return lines.join("\n").trimEnd() + "\n";
}

// ── Main ────────────────────────────────────────────────────────────────────
let outOfSync = 0;
const today = new Date().toISOString().slice(0, 10);

for (const id of formats) {
  const fmtDir = path.join(root, "core", "formats", id);
  const manifestPath = path.join(fmtDir, "corpus.yaml");
  const hasManifest = fs.existsSync(manifestPath);
  const hasTestdata = fs.existsSync(path.join(fmtDir, "testdata"));

  if (!hasTestdata && !hasManifest) {
    process.stderr.write(`gen-corpus-manifest ${id}: no testdata/ and no corpus.yaml — nothing to do\n`);
    continue;
  }

  let existing = null;
  if (hasManifest) {
    try {
      existing = loadYamlFile(manifestPath);
    } catch (e) {
      process.stderr.write(`error: ${id}: existing corpus.yaml is unparseable: ${e.message}\n`);
      outOfSync++;
      continue;
    }
  }

  const { entries, sourcesFiles } = buildManifest(id, existing);

  if (opts.check) {
    if (!hasManifest) {
      process.stderr.write(`OUT-OF-SYNC ${id}: testdata/ exists but core/formats/${id}/corpus.yaml is missing\n`);
      outOfSync++;
      continue;
    }
    const prior = Array.isArray(existing) ? existing : (existing?.entries ?? []);
    const priorNorm = JSON.stringify(prior.map(orderEntry).sort((a, b) => a.path.localeCompare(b.path)));
    const nextNorm = JSON.stringify(entries);
    if (priorNorm !== nextNorm) {
      const priorPaths = new Set(prior.map((e) => e.path));
      const nextPaths = new Set(entries.map((e) => e.path));
      for (const e of entries) {
        if (!priorPaths.has(e.path)) process.stderr.write(`OUT-OF-SYNC ${id}: not in manifest: ${e.path}\n`);
      }
      for (const e of prior) {
        if (!nextPaths.has(e.path)) process.stderr.write(`OUT-OF-SYNC ${id}: manifest entry has no file on disk: ${e.path}\n`);
      }
      const nextByPath = new Map(entries.map((e) => [e.path, e]));
      for (const e of prior) {
        const n = nextByPath.get(e.path);
        if (n && (n.sha256 !== e.sha256 || n.size !== e.size)) {
          process.stderr.write(`OUT-OF-SYNC ${id}: sha256/size drift: ${e.path}\n`);
        }
      }
      outOfSync++;
    } else {
      process.stdout.write(`gen-corpus-manifest ${id}: in sync (${entries.length} entries)\n`);
    }
    continue;
  }

  // Write the manifest (refresh keeps a stable generated_at only via --check;
  // a write records today as the regeneration date).
  fs.writeFileSync(manifestPath, manifestYaml(id, today, entries));
  process.stdout.write(`gen-corpus-manifest ${id}: wrote core/formats/${id}/corpus.yaml (${entries.length} entries)\n`);

  // Regenerate the human SOURCES.md view where one existed.
  for (const sf of sourcesFiles) {
    fs.writeFileSync(sf, sourcesMdFromManifest(id, path.dirname(sf), entries));
    process.stdout.write(`gen-corpus-manifest ${id}: regenerated ${path.relative(root, sf)}\n`);
  }
}

process.exit(outOfSync > 0 ? 1 : 0);
