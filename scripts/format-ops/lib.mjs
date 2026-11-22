// Shared helpers for the format-ops scripts (scripts/format-ops/*.mjs).
//
// YAML parsing: js-yaml is resolved from the pnpm workspace root
// (hoisted to <root>/node_modules via shamefully-hoist). No other
// external dependencies are used — everything else is node: builtins.

import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";

const require = createRequire(import.meta.url);

let _yaml = null;
export function yaml() {
  if (_yaml) return _yaml;
  try {
    _yaml = require("js-yaml");
  } catch {
    process.stderr.write(
      "error: js-yaml is not resolvable from scripts/format-ops/.\n" +
        "Run `vp install` at the repo root (js-yaml is hoisted into node_modules).\n",
    );
    process.exit(2);
  }
  return _yaml;
}

const HERE = path.dirname(fileURLToPath(import.meta.url));

/** Repo root = two levels up from scripts/format-ops/. Overridable with --root. */
export const DEFAULT_ROOT = path.resolve(HERE, "..", "..");

/** Pull `--flag value` / `--flag` options out of argv; returns {opts, positional}. */
export function parseArgs(argv, valueFlags = [], boolFlags = []) {
  const opts = {};
  const positional = [];
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (valueFlags.includes(a)) {
      opts[a.replace(/^--/, "")] = argv[++i];
    } else if (boolFlags.includes(a)) {
      opts[a.replace(/^--/, "")] = true;
    } else {
      positional.push(a);
    }
  }
  return { opts, positional };
}

export function loadYamlFile(file) {
  return yaml().load(fs.readFileSync(file, "utf8"));
}

export function dumpYaml(obj) {
  return yaml().dump(obj, { indent: 2, lineWidth: 100, noRefs: true, sortKeys: false });
}

export function sha256(data) {
  return crypto.createHash("sha256").update(data).digest("hex");
}

export function nextAxisGaps(floor, axis, level, dims) {
  const signals = floor.axes[axis]?.signals || {};
  const native = floor.type === "harvest";
  const criteria = {
    L0: [["reader", "provide a reader"], ["writer", "provide a writer"], ["config", "declare the format configuration"]],
    L1: [["malformed", "test the reader with malformed input"], native
      ? ["corpus", "exercise the native contract with corpus fixtures"]
      : ["spec", "run the native examples in spec.yaml"]],
    L2: native ? [["corpus", "verify the native corpus and invariants"]]
      : [["parity", "review the matching release evidence from okapi-bridge"], ["corpus", "exercise real files in a corpus test"]],
    L3: native ? [] : [["writer", "verify write-back fidelity"], ["config", "verify configuration against the schema"],
      ["parity", "review divergences against the specification"], ["corpus", "verify the source of the corpus fixtures"]],
    V0: [["vocabmap", "document the read cells and resolve their evidence"], ["vocabtypes", "emit canonical run types in the reader"]],
    V1: [["writecells", "verify writing from canonical content"], ["equivalence", "add a vocabulary equivalence case"]],
    V2: [["writecells", "resolve unknown write cells"], ["equivalence", "verify vocabulary equivalence in both directions"]],
    E0: [["preview", "implement format.PreviewBuilder"]],
    E1: [["identity", "declare and exercise the identity-binding round-trip test"]],
    E2: [["embedded", "declare and verify the editor integration manifest"]],
    E3: [["events", "declare and verify the editor event handler"]],
    K0: [["dossier", "cite a registered specification and implementation in dossier.yaml"], ["sidecar", "complete the format reference sidecar"]],
    K1: [["refs", "resolve the specification and implementation references"]],
    K2: [["citations", "record a passing citation check"], ["contextpack", "record a passing context pack check"]],
    C0: [["corpusmanifest", "cover the test fixtures in corpus.yaml"], ["corpus", "exercise the documented fixtures in a test"]],
    C1: [["fetchwiring", "reference a fetched Tier B fixture in spec.yaml or record a reviewed exemption"]],
    C2: [["corpus", "verify the corpus fixture origins"], ["acceptance", signals.acceptance === "partial"
      ? "record a passing CI result for the existing acceptance test" : "exercise an independent consumer in an acceptance test"],
      ["sweep", "record a passing corpus sweep"]],
    S0: [["safeio", "apply core/safeio budgets to the reader"]],
    S1: [["fuzz", "run a fuzz target with committed seeds"]],
    S2: [["sweepclean", "record a clean corpus sweep"]],
    S3: [["sustained", "record consecutive clean sweeps"]],
    G0: [["metaplane", "classify metadata and caption text"]],
    G1: [["readingorder", "emit grouped body text in reading order"]],
    G2: [["roles", "emit and test structural roles and relationships"]],
    G3: [["geometry", "emit and test geometry"]],
  };
  const gaps = (criteria[level] ?? []).filter(([key]) => !["complete", "na"].includes(dims[key])).map(([, detail]) => detail);
  if (level === "C2" && signals.externally_verified_wild === 0) gaps.push("record external verification of a wild fixture");
  if (level === "K1" && !floor.has.schema) gaps.push("provide schema.go");
  if (level === "L0" && !gaps.length) gaps.push("exercise the reader and writer with a fidelity test");
  if (axis === "engine" && level === "L1" && native) {
    for (const kind of ["okapi_skip", "invariants", "corpus"]) {
      if (!floor.test_kinds.includes(kind)) gaps.push("add the native " + kind + " test");
    }
  }
  return gaps.slice(0, 3);
}

export function assertTierMinimum(id, declaration, levels) {
  const minimum = {
    available: { engine: "L0" },
    maintained: { engine: "L2" },
    supported: { engine: "L3", corpus: "C2", knowledge: "K2" },
  }[declaration.tier];
  if (!minimum) throw new Error(id + " has an unknown support tier: " + declaration.tier);
  if (declaration.grandfathered === true) return;
  for (const [axis, required] of Object.entries(minimum)) {
    const grade = levels[axis];
    if (typeof grade !== "string" || grade.length !== 2 || grade[0] !== required[0] ||
        !/[0-4]/.test(grade[1]) || Number(grade[1]) < Number(required[1])) {
      throw new Error(`${id} declares ${declaration.tier} and requires ${axis} ${required}. Measured ${grade ?? "no grade"}.`);
    }
  }
}

// Hash the tracked files under core/formats, so a clone and a checkout with
// untracked scratch files produce the same digest.
export function formatTreeDigest(root) {
  const files = execFileSync("git", ["ls-files", "--cached", "-z", "--", "core/formats"], {
    cwd: root, encoding: "utf8",
  }).split("\0").filter(Boolean).sort();
  const digest = crypto.createHash("sha256");
  for (const file of files) {
    const fullPath = path.join(root, file);
    if (fs.existsSync(fullPath)) digest.update(file).update("\0").update(sha256(fs.readFileSync(fullPath)));
  }
  return digest.digest("hex");
}

/** Non-format directories under core/formats/ excluded from the reporting universe. */
export const EXCLUDED_FORMAT_DIRS = ["exec", "jsx", "memorytest"];

export function formatFamily(id, spec, directory) {
  if (typeof spec?.format === "string" && spec.format.startsWith("okf_")) return "parity";
  if (spec?.format === id || fs.existsSync(path.join(directory, "okapi_skip_test.go"))) return "native";
  return "unknown";
}

/** The real format dirs under core/formats (sorted), excluding exec/jsx/memorytest. */
// A format dir counts toward the reporting universe when it either ships a
// reader.go (an in-core format) OR ships a structure.yaml (the dashboard
// allowlist seat for a PLUGIN-PROVIDED, out-of-core format). This mirrors the
// canonical Go definition in core/formats/maturity_test.go (realFormatDirs) so
// the JS format-ops gates and the Go maturity gates agree on the universe.
//
// The structure.yaml seat exists for the Structure/Geometry (G) axis flagship,
// pdf: it has no in-core reader.go (the native reader is the kapi-pdfium plugin;
// the browser path is a PDFium-wasm bridge), so without this allowlist it would
// vanish from /format-maturity even though it is the axis's best example. Its
// structure.yaml declares the AD-028 authority tier + plugin ceiling that a grep
// of the package cannot prove (SHARPEN §5 decision 5). Non-format dirs
// (exec/jsx/memorytest) never ship a structure.yaml, so they stay excluded.
export function realFormatDirs(root) {
  const dir = path.join(root, "core", "formats");
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .filter((e) => e.isDirectory() && !EXCLUDED_FORMAT_DIRS.includes(e.name))
    .filter(
      (e) =>
        fs.existsSync(path.join(dir, e.name, "reader.go")) ||
        fs.existsSync(path.join(dir, e.name, "structure.yaml")),
    )
    .map((e) => e.name)
    .sort();
}

const ISO_RE = /^\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?)?$/;

/**
 * Accepts YYYY-MM-DD or a full ISO-8601 timestamp. Also accepts a JS Date,
 * because js-yaml parses unquoted YAML timestamps into Date objects.
 */
export function isISODate(s) {
  if (s instanceof Date) return !Number.isNaN(s.getTime());
  return typeof s === "string" && ISO_RE.test(s) && !Number.isNaN(Date.parse(s));
}

export function isPlainObject(v) {
  return v !== null && typeof v === "object" && !Array.isArray(v);
}

/** Problem collector: accumulate errors, print them all, exit non-zero if any. */
export class Problems {
  constructor(label) {
    this.label = label;
    this.errors = [];
    this.warnings = [];
  }
  error(msg) {
    this.errors.push(msg);
  }
  warn(msg) {
    this.warnings.push(msg);
  }
  /** Print report; returns the exit code (1 if any errors). */
  report(okMessage) {
    for (const w of this.warnings) process.stderr.write(`warning: ${w}\n`);
    for (const e of this.errors) process.stderr.write(`error: ${e}\n`);
    if (this.errors.length > 0) {
      process.stderr.write(`${this.label}: FAIL (${this.errors.length} error(s), ${this.warnings.length} warning(s))\n`);
      return 1;
    }
    process.stdout.write(`${this.label}: OK${okMessage ? ` — ${okMessage}` : ""}\n`);
    return 0;
  }
}

/** Recursively list files under dir (relative paths), sorted. */
export function walkFiles(dir, rel = "") {
  const out = [];
  if (!fs.existsSync(dir)) return out;
  for (const e of fs.readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
    const r = rel ? `${rel}/${e.name}` : e.name;
    if (e.isDirectory()) out.push(...walkFiles(path.join(dir, e.name), r));
    else if (e.isFile()) out.push(r);
  }
  return out;
}

/**
 * Compare a committed format dataset with a fresh measurement of the same
 * checkout. A lower grade on any axis is a regression. A higher grade, a new
 * or removed format and a changed gap are changes the scheduled refresh
 * records. `axes` maps each axis to its grades in ascending order.
 */
export function formatRegressions(committed, fresh, axes) {
  const now = new Map(fresh.map(row => [row.id, row]));
  const before = new Map(committed.map(row => [row.id, row]));
  const regressions = [], changes = [];
  for (const [id, row] of before) {
    const next = now.get(id);
    if (!next) { changes.push(id + ' is no longer measured'); continue; }
    for (const [axis, grades] of Object.entries(axes)) {
      const was = row.levels?.[axis], is = next.levels?.[axis];
      if (was === is) continue;
      const line = id + ' ' + axis + ' ' + (was ?? 'absent') + ' -> ' + (is ?? 'absent');
      if (grades.indexOf(is) < grades.indexOf(was)) regressions.push(line);
      else changes.push(line);
    }
    if (JSON.stringify(row.blocking_gaps) !== JSON.stringify(next.blocking_gaps)) changes.push(id + ' blocking gaps changed');
  }
  for (const id of now.keys()) if (!before.has(id)) changes.push(id + ' is newly measured');
  return { regressions, changes };
}
