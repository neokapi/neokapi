// Shared helpers for the format-ops scripts (scripts/format-ops/*.mjs).
//
// YAML parsing: js-yaml is resolved from the pnpm workspace root
// (hoisted to <root>/node_modules via shamefully-hoist). No other
// external dependencies are used — everything else is node: builtins.

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

/** Non-format directories under core/formats/ excluded from the reporting universe. */
export const EXCLUDED_FORMAT_DIRS = ["exec", "jsx", "memorytest"];

/** Harvest formats (no Okapi counterpart): parity is `na`, ladder is okapi_skip+invariants+corpus. */
export const HARVEST_FORMATS = [
  "androidxml",
  "applestrings",
  "arb",
  "designtokens",
  "i18next",
  "mdx",
  "resx",
  "xcstrings",
];

/**
 * Parity spec-test filename aliases: a few format dirs have a parity test
 * whose basename differs from the directory id (historical naming).
 */
export const PARITY_TEST_ALIASES = { phpcontent: "php", xml: "xmlstream" };

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
