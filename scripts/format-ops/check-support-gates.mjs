#!/usr/bin/env node
// check-support-gates.mjs — two modes over the support-tier contract
// (docs/internals/format-maturity.md §1).
//
// Usage:
//   check-support-gates.mjs [--file <support.yaml>] [--root <repo-root>]
//       Default (schema-validation) mode — see "Schema-validation mode" below.
//
//   check-support-gates.mjs --route <parity-report.json> [--acceptance <results.json>]
//                           [--routed-out <out.json>] [--file <support.yaml>] [--root <repo-root>]
//       Tier-aware support-gate routing (issue #850) — see "Route mode" below.
//
//   check-support-gates.mjs --selftest
//       Synthesize fixtures in os.tmpdir and assert the three routing
//       outcomes (Supported fail → nonzero, Maintained-only fail → 0 + warning,
//       all-green → 0). Exits nonzero if any case misbehaves.
//
// ── Schema-validation mode (default) ─────────────────────────────────────────
// Checks:
//   - the entry universe == exactly the real format dirs under core/formats
//     (exclude exec/jsx/memorytest; fail on missing AND extra entries)
//   - tier ∈ {Supported, Maintained, Available} (matched case-insensitively;
//     the seeded support.yaml uses lowercase values)
//   - tier_since is an ISO date; last_certified is null or an ISO date
//   - every entry's gates name files that exist under .github/workflows/
//     (or the literal "make test"); Supported entries must have >= 1 gate
//   - Supported parity formats: cli/parity/formats/<id>_spec_test.go exists
//     (aliases: phpcontent -> php, xml -> xmlstream)
//   - Supported harvest formats: acceptance + invariants tests exist under
//     core/formats/<id>/
//   - grandfathered, when present, is a boolean
//
// Accepted file shapes (the validator defines the contract until bootstrap
// seeds the file): a top-level `formats:` mapping of id -> entry, a bare
// top-level mapping of id -> entry, or `formats:` as a list of entries
// each carrying an `id` field.
//
// ── Route mode (--route), issue #850 ─────────────────────────────────────────
// This is the mechanism the rubric §1 promised ("maps the latest
// parity/acceptance results to tiers so a Supported-format failure is
// distinguishable from a Maintained one"). Issue #850 is satisfied: that
// mechanism now exists here.
//
// Given the latest parity report (`.parity/test-comparison.json` raw array OR
// the published `web/static/data/parity-report.json` object — both shapes are
// accepted) and, optionally, a format-acceptance results JSON, it:
//   1. Builds an Okapi-id → neokapi-format map from each format's spec.yaml
//      `format:` field (the canonical key a Supported format's head-to-head
//      parity row is recorded under). Harvest formats with no spec.yaml are
//      matched by their neokapi id directly (the acceptance-results path).
//   2. Maps every failing row (status `fail` or `error`; `expected_fail` and
//      `parity_warn` are managed divergences, NOT regressions) to its format
//      and the format's declared tier.
//   3. Routes by tier — enforcement direction per rubric §1 (CI may exceed the
//      promise, never fall short):
//        • Supported   → ::error annotation, counted as a release-gating
//                        regression → EXIT NONZERO.
//        • Maintained/ → ::warning annotation (GitHub Actions form) + an entry
//          Available     in the machine list so a CI step can open issues →
//                        EXIT 0.
//   4. Emits a machine-readable JSON list of routed failures to stdout (between
//      stable markers), to `--routed-out <path>` when given, and to
//      `$GITHUB_OUTPUT` (best-effort) so a follow-on CI step can act on it.
//
// When there are zero Supported formats (today's bootstrap state), route mode
// is a clean no-op pass: the only blocking condition is impossible, which the
// output asserts explicitly. The step becomes load-bearing as the tier-review
// ritual promotes formats to Supported.
//
// Route mode reads only `tier` (support.yaml) and `format:` (each spec.yaml),
// so it does NOT hard-depend on js-yaml: it prefers js-yaml when resolvable and
// otherwise falls back to a minimal line extractor. This lets the parity.yml CI
// step run as a single `node …` invocation on a bare Go/Java runner with no
// node_modules, and never spuriously fail the parity job.

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import {
  DEFAULT_ROOT,
  EXCLUDED_FORMAT_DIRS,
  HARVEST_FORMATS,
  PARITY_TEST_ALIASES,
  Problems,
  isISODate,
  isPlainObject,
  loadYamlFile,
  parseArgs,
  realFormatDirs,
} from "./lib.mjs";

const requireFromHere = createRequire(import.meta.url);

// Route mode reads only two narrow fields — each format's `tier` from
// support.yaml and the top-level `format:` from each spec.yaml — and is the
// mode wired into a Go/Java CI job (parity.yml) that has no node_modules. So it
// must NOT hard-depend on js-yaml: it prefers js-yaml when resolvable, and
// otherwise falls back to a minimal line extractor for those two fields.
// Default (schema-validation) mode is unchanged and still requires js-yaml via
// lib.mjs. `KAPI_SUPPORT_GATES_NO_YAML=1` forces the fallback (selftest only).
function tryYaml() {
  if (process.env.KAPI_SUPPORT_GATES_NO_YAML === "1") return null;
  try {
    return requireFromHere("js-yaml");
  } catch {
    return null;
  }
}

// Tier values are matched case-insensitively: the rubric table capitalizes
// them, the seeded support.yaml uses lowercase — same enum either way.
const TIERS = ["supported", "maintained", "available"];

// Failure statuses for route mode. `expected_fail`/`parity_warn` are managed
// divergences, NOT regressions, and must never route to a gate.
const FAIL_STATUSES = new Set(["fail", "error"]);

const tierOf = (e) => (typeof e?.tier === "string" ? e.tier.toLowerCase() : undefined);

// ── Mode dispatch (single point, at the end of the file) ────────────────────
const { opts } = parseArgs(
  process.argv.slice(2),
  ["--file", "--root", "--route", "--acceptance", "--routed-out"],
  ["--selftest"],
);
const root = opts.root ? path.resolve(opts.root) : DEFAULT_ROOT;
const file = opts.file ? path.resolve(opts.file) : path.join(root, "core", "formats", "support.yaml");

main();

function main() {
  if (opts.selftest) {
    process.exit(runSelftest());
  }
  if (opts.route !== undefined) {
    process.exit(
      runRoute({ file, root, reportPath: opts.route, acceptancePath: opts.acceptance, routedOut: opts["routed-out"] }),
    );
  }
  process.exit(runDefault({ file, root }));
}

// ─────────────────────────────────────────────────────────────────────────
// Default mode: schema-validate support.yaml (behavior unchanged).
// ─────────────────────────────────────────────────────────────────────────

function runDefault({ file, root }) {
  const p = new Problems(`check-support-gates ${path.relative(process.cwd(), file)}`);

  if (!fs.existsSync(file)) {
    p.error(`support.yaml not found: ${file}`);
    return p.report();
  }

  let doc;
  try {
    doc = loadYamlFile(file);
  } catch (e) {
    p.error(`not valid YAML: ${e.message}`);
    return p.report();
  }

  // ── Normalize to {id: entry} ──────────────────────────────────────────────
  let entries = null;
  if (isPlainObject(doc) && isPlainObject(doc.formats)) {
    entries = doc.formats;
  } else if (isPlainObject(doc) && Array.isArray(doc.formats)) {
    entries = {};
    doc.formats.forEach((e, i) => {
      if (!isPlainObject(e) || typeof e.id !== "string") {
        p.error(`formats[${i}]: list entries must be objects with an "id" field`);
        return;
      }
      if (entries[e.id]) p.error(`formats: duplicate id "${e.id}"`);
      entries[e.id] = e;
    });
  } else if (isPlainObject(doc)) {
    entries = doc;
  } else {
    p.error(`support.yaml must be a mapping of format id -> entry (optionally under a "formats:" key)`);
    return p.report();
  }

  // ── Universe check ────────────────────────────────────────────────────────
  const universe = realFormatDirs(root);
  const declared = Object.keys(entries).sort();
  for (const id of universe) {
    if (!declared.includes(id)) p.error(`missing entry for format dir core/formats/${id}/`);
  }
  for (const id of declared) {
    if (!universe.includes(id)) {
      p.error(`extra entry "${id}" has no matching format dir under core/formats/ (universe is the ${universe.length} real format dirs; exec/jsx/memorytest excluded)`);
    }
  }

  // ── Per-entry checks ──────────────────────────────────────────────────────
  const workflowsDir = path.join(root, ".github", "workflows");

  function hasFileMatching(dir, re) {
    if (!fs.existsSync(dir)) return false;
    return fs.readdirSync(dir).some((f) => re.test(f));
  }

  for (const id of declared) {
    const e = entries[id];
    const at = `formats.${id}`;
    if (!isPlainObject(e)) {
      p.error(`${at}: entry must be an object`);
      continue;
    }

    const tier = tierOf(e);
    if (!TIERS.includes(tier)) {
      p.error(`${at}.tier must be one of ${TIERS.join("|")} (any case), got ${JSON.stringify(e.tier)}`);
    }
    if (!isISODate(e.tier_since)) {
      p.error(`${at}.tier_since must be an ISO date, got ${JSON.stringify(e.tier_since)}`);
    }
    if (!(e.last_certified == null || isISODate(e.last_certified))) {
      p.error(`${at}.last_certified must be null or an ISO date, got ${JSON.stringify(e.last_certified)}`);
    }
    if ("grandfathered" in e && typeof e.grandfathered !== "boolean") {
      p.error(`${at}.grandfathered must be a boolean, got ${JSON.stringify(e.grandfathered)}`);
    }
    if ("notes" in e && typeof e.notes !== "string") {
      p.error(`${at}.notes must be a string`);
    }

    // gates
    if ("gates" in e || tier === "supported") {
      if (!Array.isArray(e.gates)) {
        p.error(`${at}.gates must be an array of workflow filenames (or the literal "make test")`);
      } else {
        if (tier === "supported" && e.gates.length === 0) {
          p.error(`${at}: Supported entries must name at least one CI gate (a tier not enforced by CI is marketing)`);
        }
        e.gates.forEach((g, i) => {
          if (typeof g !== "string" || g === "") {
            p.error(`${at}.gates[${i}] must be a non-empty string`);
            return;
          }
          if (g === "make test") return;
          const wf = path.join(workflowsDir, path.basename(g));
          if (!fs.existsSync(wf)) {
            p.error(`${at}.gates[${i}]: "${g}" does not exist under .github/workflows/ (and is not the literal "make test")`);
          }
        });
      }
    }

    // Supported tier: enforcement artifacts must exist on HEAD.
    if (tier === "supported") {
      if (HARVEST_FORMATS.includes(id)) {
        const dir = path.join(root, "core", "formats", id);
        if (!hasFileMatching(dir, /acceptance.*_test\.go$/)) {
          p.error(`${at}: Supported harvest format has no acceptance test (core/formats/${id}/*acceptance*_test.go)`);
        }
        if (!hasFileMatching(dir, /^invariants_test\.go$/)) {
          p.error(`${at}: Supported harvest format has no invariants test (core/formats/${id}/invariants_test.go)`);
        }
      } else {
        const base = PARITY_TEST_ALIASES[id] ?? id;
        const parity = path.join(root, "cli", "parity", "formats", `${base}_spec_test.go`);
        if (!fs.existsSync(parity)) {
          p.error(`${at}: Supported parity format has no parity spec test (cli/parity/formats/${base}_spec_test.go)`);
        }
      }
    }
  }

  const tierCounts = {};
  for (const id of declared) {
    const t = tierOf(entries[id]) ?? "?";
    tierCounts[t] = (tierCounts[t] ?? 0) + 1;
  }
  return p.report(
    `${declared.length} formats (${Object.entries(tierCounts)
      .map(([t, n]) => `${t}: ${n}`)
      .join(", ")})`,
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Route mode (issue #850) and its helpers.
// ─────────────────────────────────────────────────────────────────────────

function stripQuotes(s) {
  return s.replace(/^['"]|['"]$/g, "");
}

/**
 * Minimal fallback for the `formats:` mapping shape (what support.yaml uses):
 * returns {id: {tier}} by line-scanning indentation. Used only when js-yaml is
 * not resolvable. Comments and blank lines are skipped; bare top-level mappings
 * (no `formats:` wrapper) are also handled.
 */
function fallbackSupportTiers(file) {
  const out = {};
  let inFormats = false;
  let sawFormatsKey = false;
  let curId = null;
  let curIndent = -1;
  for (const raw of fs.readFileSync(file, "utf8").split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith("#")) continue;
    const indent = raw.length - raw.trimStart().length;
    if (!inFormats) {
      if (/^formats:\s*$/.test(line)) {
        inFormats = true;
        sawFormatsKey = true;
        continue;
      }
      // No `formats:` wrapper — treat indent-0 "id:" lines as format starts.
      if (indent === 0 && /^[A-Za-z0-9_.\-]+:\s*$/.test(line)) inFormats = true;
      else continue;
    }
    // A format id line (the shallowest mapping key under formats:).
    if (/^[A-Za-z0-9_.\-]+:\s*$/.test(line) && (curIndent < 0 || indent <= curIndent)) {
      curId = line.slice(0, -1);
      out[curId] = out[curId] ?? {};
      curIndent = indent;
      continue;
    }
    if (curId && indent > curIndent) {
      const m = line.match(/^tier:\s*(.+?)\s*$/);
      if (m) out[curId].tier = stripQuotes(m[1]);
    }
  }
  void sawFormatsKey;
  return out;
}

/** Load + normalize support.yaml to a {id: entry} map. Returns null on hard error. */
function loadSupportEntries(supportFile, yamlMod) {
  if (!fs.existsSync(supportFile)) {
    process.stderr.write(`error: support.yaml not found: ${supportFile}\n`);
    return null;
  }
  if (!yamlMod) return fallbackSupportTiers(supportFile);
  let sdoc;
  try {
    sdoc = yamlMod.load(fs.readFileSync(supportFile, "utf8"));
  } catch (e) {
    process.stderr.write(`error: support.yaml is not valid YAML: ${e.message}\n`);
    return null;
  }
  if (isPlainObject(sdoc) && isPlainObject(sdoc.formats)) return sdoc.formats;
  if (isPlainObject(sdoc) && Array.isArray(sdoc.formats)) {
    const m = {};
    for (const e of sdoc.formats) if (isPlainObject(e) && typeof e.id === "string") m[e.id] = e;
    return m;
  }
  if (isPlainObject(sdoc)) return sdoc;
  process.stderr.write(`error: support.yaml must be a mapping of format id -> entry\n`);
  return null;
}

/**
 * Build the Okapi-id → neokapi-format-id map from each format's spec.yaml
 * `format:` field. This is the canonical key a format's head-to-head parity
 * row (and its Okapi-harvested fixtures) is recorded under, so it is the
 * authoritative bridge from a parity-report row to its declared tier.
 */
function okapiFormatMap(repoRoot, yamlMod) {
  const dir = path.join(repoRoot, "core", "formats");
  const map = {};
  if (!fs.existsSync(dir)) return map;
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    if (!e.isDirectory() || EXCLUDED_FORMAT_DIRS.includes(e.name)) continue;
    const spec = path.join(dir, e.name, "spec.yaml");
    if (!fs.existsSync(spec)) continue;
    let okapiId;
    if (yamlMod) {
      let sdoc;
      try {
        sdoc = yamlMod.load(fs.readFileSync(spec, "utf8"));
      } catch {
        continue;
      }
      if (isPlainObject(sdoc) && typeof sdoc.format === "string") okapiId = sdoc.format;
    } else {
      // Fallback: the top-level `format:` scalar (anchored to column 0).
      const m = fs.readFileSync(spec, "utf8").match(/^format:[ \t]*(\S+)/m);
      if (m) okapiId = stripQuotes(m[1]);
    }
    if (okapiId) map[okapiId] = e.name;
  }
  return map;
}

/** Pull the row array out of either report shape (bare array or {rows:[...]}). */
function rowsOf(reportDoc) {
  if (Array.isArray(reportDoc)) return reportDoc;
  if (isPlainObject(reportDoc) && Array.isArray(reportDoc.rows)) return reportDoc.rows;
  return null;
}

/**
 * Resolve a report row's id to a declared neokapi format id, or null.
 * The id is `okf_*` / `okf_*::Class#method` for parity rows, or a bare
 * neokapi format id for acceptance rows. The Okapi prefix (before `::`) is
 * matched against the spec.yaml-derived map first, then tried as a direct
 * neokapi id.
 */
function resolveFormat(rawId, okapiMap, entries) {
  const key = String(rawId).split("::")[0];
  if (okapiMap[key]) return okapiMap[key];
  if (entries[key]) return key;
  return null;
}

/** Read + parse a report file into rows; returns {rows} or {err}. */
function readReportRows(reportPath, what) {
  const abs = path.resolve(reportPath);
  if (!fs.existsSync(abs)) return { err: `${what} not found: ${reportPath}` };
  let parsed;
  try {
    parsed = JSON.parse(fs.readFileSync(abs, "utf8"));
  } catch (e) {
    return { err: `${what} is not valid JSON (${reportPath}): ${e.message}` };
  }
  const rows = rowsOf(parsed);
  if (rows === null) {
    return { err: `${what} has no rows (expected a JSON array or an object with a "rows" array): ${reportPath}` };
  }
  return { rows, abs };
}

/** Route mode entry point. Returns the process exit code. */
function runRoute({ file: supportFile, root: repoRoot, reportPath, acceptancePath, routedOut }) {
  const label = "check-support-gates --route";

  const yamlMod = tryYaml();
  const entries = loadSupportEntries(supportFile, yamlMod);
  if (entries === null) return 2;

  const okapiMap = okapiFormatMap(repoRoot, yamlMod);
  const supportedFormats = Object.keys(entries)
    .filter((id) => tierOf(entries[id]) === "supported")
    .sort();

  // Collect failing rows from the parity report (+ optional acceptance results).
  const sources = [];

  const parity = readReportRows(reportPath, "parity report");
  if (parity.err) {
    process.stderr.write(`error: ${parity.err}\n`);
    return 2;
  }
  // Only format-scoped rows are tier-routable; `step` rows are not formats.
  sources.push({
    name: path.relative(repoRoot, parity.abs) || reportPath,
    rows: parity.rows.filter((r) => isPlainObject(r) && typeof r.kind === "string" && r.kind.startsWith("format")),
  });

  if (acceptancePath) {
    const acc = readReportRows(acceptancePath, "--acceptance results");
    if (acc.err) {
      process.stderr.write(`error: ${acc.err}\n`);
      return 2;
    }
    // Acceptance rows are format-level; key may be `id` or `format`.
    sources.push({
      name: path.relative(repoRoot, acc.abs) || acceptancePath,
      rows: acc.rows.filter(isPlainObject).map((r) => ({ ...r, id: r.id ?? r.format })),
    });
  }

  // Aggregate failing rows by format.
  const byFormat = new Map(); // format -> {tier, count, examples:[]}
  const unmapped = new Map(); // okapi key -> count
  for (const src of sources) {
    for (const r of src.rows) {
      if (!FAIL_STATUSES.has(String(r.status))) continue;
      const fmt = resolveFormat(r.id, okapiMap, entries);
      if (fmt === null) {
        const key = String(r.id).split("::")[0];
        unmapped.set(key, (unmapped.get(key) ?? 0) + 1);
        continue;
      }
      let agg = byFormat.get(fmt);
      if (!agg) {
        agg = { tier: tierOf(entries[fmt]) ?? "(undeclared)", count: 0, examples: [] };
        byFormat.set(fmt, agg);
      }
      agg.count++;
      if (agg.examples.length < 3) agg.examples.push(String(r.id));
    }
  }

  const blocking = []; // Supported formats with failures
  const warnings = []; // Maintained/Available (and any non-Supported) failures
  for (const [fmt, agg] of [...byFormat.entries()].sort()) {
    const entry = { format: fmt, tier: agg.tier, count: agg.count, examples: agg.examples };
    if (agg.tier === "supported") blocking.push(entry);
    else warnings.push(entry);
  }

  // ── Emit annotations ────────────────────────────────────────────────────
  for (const w of warnings) {
    process.stdout.write(
      `::warning title=Format regression (${w.format}, tier=${w.tier})::` +
        `${w.count} failing parity/acceptance row(s) for "${w.format}" [tier=${w.tier}] — ` +
        `non-release-gating; fixed on the maintain cadence (format-maturity.md §1). ` +
        `e.g. ${w.examples.join(", ")}\n`,
    );
  }
  for (const b of blocking) {
    process.stdout.write(
      `::error title=Supported format regression (${b.format})::` +
        `${b.count} failing parity/acceptance row(s) for Supported format "${b.format}" — ` +
        `this BLOCKS release (format-maturity.md §1). e.g. ${b.examples.join(", ")}\n`,
    );
  }

  // ── Machine list (for a CI step to open issues) ───────────────────────────
  const machine = {
    generated_at: new Date().toISOString(),
    report: sources.map((s) => s.name),
    supported_formats: supportedFormats,
    supported_regressions: blocking,
    warnings,
    unmapped: [...unmapped.entries()]
      .map(([okapi_id, count]) => ({ okapi_id, count }))
      .sort((a, b) => a.okapi_id.localeCompare(b.okapi_id)),
  };
  const machineJSON = JSON.stringify(machine);
  process.stdout.write("===ROUTED-FAILURES-JSON===\n");
  process.stdout.write(machineJSON + "\n");
  process.stdout.write("===END-ROUTED-FAILURES-JSON===\n");

  if (routedOut) {
    fs.mkdirSync(path.dirname(path.resolve(routedOut)), { recursive: true });
    fs.writeFileSync(path.resolve(routedOut), JSON.stringify(machine, null, 2) + "\n");
    process.stdout.write(`${label}: machine list written to ${routedOut}\n`);
  }
  if (process.env.GITHUB_OUTPUT) {
    try {
      fs.appendFileSync(
        process.env.GITHUB_OUTPUT,
        `has_supported_regressions=${blocking.length > 0}\n` +
          `routed_warnings_count=${warnings.length}\n` +
          `routed<<ROUTED_EOF\n${machineJSON}\nROUTED_EOF\n`,
      );
    } catch {
      /* best-effort; never fail the gate on an output-write hiccup */
    }
  }

  // ── Summary + the explicit zero-Supported assertion ───────────────────────
  if (unmapped.size > 0) {
    const total = [...unmapped.values()].reduce((a, b) => a + b, 0);
    process.stdout.write(
      `${label}: info — ${total} failing row(s) across ${unmapped.size} Okapi id(s) not mapped to a declared neokapi format ` +
        `(bridge-only filters with no native reader; not tier-gated): ${[...unmapped.keys()].sort().join(", ")}\n`,
    );
  }

  if (supportedFormats.length === 0) {
    process.stdout.write(
      `${label}: no Supported formats declared in support.yaml — the support gate is a clean no-op ` +
        `(a Supported regression is the only blocking condition, and 0 are possible today). ` +
        `${warnings.length} Maintained/Available format(s) had failures (warn-only). ` +
        `This step becomes load-bearing once tier-review promotes a format to Supported.\n`,
    );
    // Invariant: with zero Supported formats there can be no blocking rows.
    if (blocking.length !== 0) {
      process.stderr.write(`${label}: internal error — blocking rows with zero Supported formats; refusing to pass\n`);
      return 2;
    }
    process.stdout.write(`${label}: OK — pass (no-op gate)\n`);
    return 0;
  }

  if (blocking.length > 0) {
    const names = blocking.map((b) => `${b.format}(${b.count})`).join(", ");
    process.stdout.write(
      `${label}: FAIL — ${blocking.length} Supported format(s) regressed: ${names}. ` +
        `${warnings.length} non-Supported format(s) warned.\n`,
    );
    return 1;
  }
  process.stdout.write(
    `${label}: OK — ${supportedFormats.length} Supported format(s), none regressed. ` +
      `${warnings.length} Maintained/Available format(s) had failures (warn-only).\n`,
  );
  return 0;
}

// ─────────────────────────────────────────────────────────────────────────
// --selftest: synthesize fixtures in os.tmpdir and assert the 3 outcomes.
// ─────────────────────────────────────────────────────────────────────────

function runSelftest() {
  const self = fileURLToPath(import.meta.url);
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "support-gates-selftest-"));
  const fmtDir = path.join(tmp, "core", "formats");
  fs.mkdirSync(path.join(fmtDir, "foo"), { recursive: true });
  fs.mkdirSync(path.join(fmtDir, "bar"), { recursive: true });
  // spec.yaml gives the Okapi-id → neokapi-format mapping route mode relies on.
  fs.writeFileSync(path.join(fmtDir, "foo", "spec.yaml"), "format: okf_foo\n");
  fs.writeFileSync(path.join(fmtDir, "bar", "spec.yaml"), "format: okf_bar\n");
  // support.yaml: foo is Supported (release-gating), bar is Maintained.
  fs.writeFileSync(
    path.join(fmtDir, "support.yaml"),
    [
      "formats:",
      "  foo:",
      "    tier: supported",
      '    tier_since: "2026-06-13"',
      "    last_certified: null",
      "    gates:",
      "      - .github/workflows/parity.yml",
      "  bar:",
      "    tier: maintained",
      '    tier_since: "2026-06-13"',
      "    last_certified: null",
      "    gates:",
      "      - .github/workflows/parity.yml",
      "",
    ].join("\n"),
  );

  const reportA = path.join(tmp, "reportA.json"); // Supported (foo) fails
  fs.writeFileSync(
    reportA,
    JSON.stringify({
      rows: [
        { kind: "format-fixture", id: "okf_foo::FooTest#a", status: "fail", detail: "mismatch" },
        { kind: "format-fixture", id: "okf_bar::BarTest#b", status: "pass" },
        // expected_fail must NOT count as a regression even for Supported foo
        { kind: "format-spec-feature", id: "okf_foo::FooTest#c", status: "expected_fail" },
        { kind: "step", id: "word-count", status: "fail" }, // step rows ignored
      ],
    }),
  );

  const reportB = path.join(tmp, "reportB.json"); // only Maintained (bar) fails
  fs.writeFileSync(
    reportB,
    JSON.stringify({
      rows: [
        { kind: "format-fixture", id: "okf_foo::FooTest#a", status: "pass" },
        { kind: "format-fixture", id: "okf_bar::BarTest#b", status: "fail", detail: "mismatch" },
      ],
    }),
  );

  // reportC uses the BARE-ARRAY shape (raw .parity/test-comparison.json) — all green.
  const reportC = path.join(tmp, "reportC.json");
  fs.writeFileSync(
    reportC,
    JSON.stringify([
      { kind: "format", id: "okf_foo", status: "pass", mode: "head-to-head" },
      { kind: "format", id: "okf_bar", status: "pass", mode: "head-to-head" },
    ]),
  );

  const run = (report, env) =>
    spawnSync(process.execPath, [self, "--route", report, "--root", tmp], {
      encoding: "utf8",
      env: { ...process.env, ...env },
    });

  const cases = [
    { name: "(a) Supported format fails → nonzero", report: reportA, wantNonzero: true, wantWarning: false, wantError: true },
    { name: "(b) only Maintained fails → exit 0 + warning", report: reportB, wantNonzero: false, wantWarning: true, wantError: false },
    { name: "(c) all green (bare-array report) → exit 0", report: reportC, wantNonzero: false, wantWarning: false, wantError: false },
  ];

  // Run every case under both parsers: js-yaml (when resolvable) and the
  // dependency-free fallback (forced) — the parity.yml runner has no js-yaml.
  const parsers = [
    { tag: "js-yaml", env: {} },
    { tag: "no-yaml-fallback", env: { KAPI_SUPPORT_GATES_NO_YAML: "1" } },
  ];

  let allOk = true;
  for (const parser of parsers) {
    for (const c of cases) {
      const r = run(c.report, parser.env);
      const out = (r.stdout || "") + (r.stderr || "");
      const nonzero = r.status !== 0;
      const hasWarning = /^::warning /m.test(out);
      const hasError = /^::error /m.test(out);
      const ok = nonzero === c.wantNonzero && hasWarning === c.wantWarning && hasError === c.wantError;
      allOk = allOk && ok;
      process.stdout.write(
        `selftest ${ok ? "PASS" : "FAIL"} [${parser.tag}]: ${c.name} ` +
          `(exit=${r.status}, ::warning=${hasWarning}, ::error=${hasError})\n`,
      );
      if (!ok) {
        process.stdout.write(`  expected nonzero=${c.wantNonzero} warning=${c.wantWarning} error=${c.wantError}\n`);
        process.stdout.write(out.replace(/^/gm, "  | "));
      }
    }
  }

  try {
    fs.rmSync(tmp, { recursive: true, force: true });
  } catch {
    /* ignore cleanup errors */
  }

  process.stdout.write(`selftest: ${allOk ? "OK — all 3 outcomes as expected" : "FAIL"}\n`);
  return allOk ? 0 : 1;
}
