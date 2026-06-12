#!/usr/bin/env node
// check-support-gates.mjs — schema-validate core/formats/support.yaml against
// the support-tier contract (docs/internals/format-maturity.md §1).
//
// Usage: check-support-gates.mjs [--file <support.yaml>] [--root <repo-root>]
//
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
// Failure -> tier routing inside CI is issue #850 — out of scope here.

import fs from "node:fs";
import path from "node:path";
import {
  DEFAULT_ROOT,
  HARVEST_FORMATS,
  PARITY_TEST_ALIASES,
  Problems,
  isISODate,
  isPlainObject,
  loadYamlFile,
  parseArgs,
  realFormatDirs,
} from "./lib.mjs";

const { opts } = parseArgs(process.argv.slice(2), ["--file", "--root"], []);
const root = opts.root ? path.resolve(opts.root) : DEFAULT_ROOT;
const file = opts.file ? path.resolve(opts.file) : path.join(root, "core", "formats", "support.yaml");

const p = new Problems(`check-support-gates ${path.relative(process.cwd(), file)}`);

if (!fs.existsSync(file)) {
  p.error(`support.yaml not found: ${file}`);
  process.exit(p.report());
}

let doc;
try {
  doc = loadYamlFile(file);
} catch (e) {
  p.error(`not valid YAML: ${e.message}`);
  process.exit(p.report());
}

// ── Normalize to {id: entry} ────────────────────────────────────────────────
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
  process.exit(p.report());
}

// ── Universe check ──────────────────────────────────────────────────────────
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

// ── Per-entry checks ────────────────────────────────────────────────────────
// Tier values are matched case-insensitively: the rubric table capitalizes
// them, the seeded support.yaml uses lowercase — same enum either way.
const TIERS = ["supported", "maintained", "available"];
const tierOf = (e) => (typeof e?.tier === "string" ? e.tier.toLowerCase() : undefined);
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
process.exit(
  p.report(
    `${declared.length} formats (${Object.entries(tierCounts)
      .map(([t, n]) => `${t}: ${n}`)
      .join(", ")})`,
  ),
);
