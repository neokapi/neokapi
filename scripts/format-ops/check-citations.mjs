#!/usr/bin/env node
// check-citations.mjs — resolve spec citations in per-format artifacts.
//
// Contract: docs/internals/format-maturity.md §2.4 (Knowledge axis).
// Citations live in core/formats/<id>/{spec.yaml,dossier.yaml} under
// `spec_refs` / `cite` / `citations` keys. The structured shape is
//   {spec, version, url(#fragment), clause, heading, quote (<= 1 sentence), quote_sha256}
// Legacy `spec_refs` are free-text strings (often with an embedded URL);
// they predate the structured contract and are reported as
// `legacy-unstructured`, never as hard violations (migration is backfill
// work, not a gate).
//
// Resolution per citation with a URL:
//   - pinned snapshot at specs/snapshots/<spec>/<version>/ exists
//       -> resolution_mode "snapshot": sha256(quote) must equal quote_sha256
//          AND the quote must appear in the snapshot text (whitespace-
//          normalized). Mismatch / not-found = hard violation. The URL
//          #fragment (anchor) is searched in the snapshot; a missing
//          anchor is a warning, not a violation.
//   - else spec@version registered in specs/catalog.yaml
//       -> resolution_mode "pinned-version-only" (ok)
//   - else with --network -> HEAD request; dead links are reported
//     findings (ok: false), not hard violations
//   - else -> resolution_mode "unverifiable-without-network", exit 0
//
// Hard violations (the only exit-1 causes): malformed structured citation
// shape; quote_sha256 self-inconsistency (sha256(quote) != quote_sha256);
// quote/hash mismatch against an existing snapshot.
//
// Usage: check-citations.mjs <format-id|--all> [--json] [--network] [--root <repo-root>]
//   --json: machine-readable report on stdout, human summary on stderr.

import fs from "node:fs";
import path from "node:path";
import {
  DEFAULT_ROOT,
  loadYamlFile,
  parseArgs,
  realFormatDirs,
  sha256,
  isPlainObject,
} from "./lib.mjs";

const { opts, positional } = parseArgs(process.argv.slice(2), ["--root"], ["--all", "--json", "--network"]);
const root = opts.root ? path.resolve(opts.root) : DEFAULT_ROOT;

if (!opts.all && positional.length !== 1) {
  process.stderr.write("usage: check-citations.mjs <format-id|--all> [--json] [--network] [--root <repo-root>]\n");
  process.exit(2);
}
const formats = opts.all ? realFormatDirs(root) : [positional[0]];
if (!opts.all) {
  const dir = path.join(root, "core", "formats", formats[0]);
  if (!fs.existsSync(dir)) {
    process.stderr.write(`error: no such format dir: core/formats/${formats[0]}/\n`);
    process.exit(2);
  }
}

const URL_RE = /https?:\/\/[^\s)<>"']+/;

// ── Catalog (specs/catalog.yaml) ────────────────────────────────────────────
function loadCatalog() {
  const file = path.join(root, "specs", "catalog.yaml");
  if (!fs.existsSync(file)) return [];
  const doc = loadYamlFile(file);
  const list = Array.isArray(doc) ? doc : Array.isArray(doc?.specs) ? doc.specs : [];
  return list.filter(isPlainObject);
}
const catalog = loadCatalog();
function catalogRegistered(spec, version) {
  return catalog.some(
    (c) => String(c.id) === String(spec) && (version == null || String(c.version) === String(version)),
  );
}

// ── Citation collection ─────────────────────────────────────────────────────
// Walk a YAML document for `spec_refs` (arrays) and `cite`/`citations` keys.
function collectCitations(node, file, locator, out) {
  if (Array.isArray(node)) {
    node.forEach((v, i) => collectCitations(v, file, `${locator}[${i}]`, out));
    return;
  }
  if (!isPlainObject(node)) return;
  for (const [key, value] of Object.entries(node)) {
    const loc = locator ? `${locator}.${key}` : key;
    if (key === "spec_refs" || key === "citations" || key === "cites") {
      const items = Array.isArray(value) ? value : [value];
      items.forEach((item, i) => out.push({ file, locator: `${loc}[${i}]`, raw: item }));
    } else if (key === "cite") {
      out.push({ file, locator: loc, raw: value });
    } else {
      collectCitations(value, file, loc, out);
    }
  }
}

// ── Snapshot resolution ─────────────────────────────────────────────────────
const normWS = (s) => s.replace(/\s+/g, " ").trim();
const snapshotTextCache = new Map();
function snapshotText(dir) {
  if (snapshotTextCache.has(dir)) return snapshotTextCache.get(dir);
  let text = "";
  const stack = [dir];
  while (stack.length) {
    const d = stack.pop();
    for (const e of fs.readdirSync(d, { withFileTypes: true })) {
      const f = path.join(d, e.name);
      if (e.isDirectory()) stack.push(f);
      else if (e.isFile()) {
        const buf = fs.readFileSync(f);
        // Skip binary-looking files (NUL byte heuristic).
        if (!buf.subarray(0, 8192).includes(0)) text += "\n" + buf.toString("utf8");
      }
    }
  }
  const normalized = normWS(text);
  snapshotTextCache.set(dir, normalized);
  return normalized;
}

async function headRequest(url) {
  try {
    const res = await fetch(url, { method: "HEAD", redirect: "follow", signal: AbortSignal.timeout(10000) });
    return { status: res.status, ok: res.ok };
  } catch (e) {
    return { status: null, ok: false, error: e.message };
  }
}

// ── Per-citation check ──────────────────────────────────────────────────────
async function checkCitation(cit) {
  const result = {
    format: cit.format,
    file: path.relative(root, cit.file),
    locator: cit.locator,
    ok: true,
    violations: [],
    warnings: [],
  };

  if (typeof cit.raw === "string") {
    result.kind = "legacy-unstructured";
    const m = cit.raw.match(URL_RE);
    if (!m) {
      result.resolution_mode = "no-url";
      return result;
    }
    result.url = m[0].replace(/[).,]+$/, "");
    if (opts.network) {
      const head = await headRequest(result.url);
      result.resolution_mode = "network-head";
      result.http_status = head.status;
      if (!head.ok) {
        result.ok = false;
        result.warnings.push(`HEAD ${result.url} failed (${head.status ?? head.error})`);
      }
    } else {
      result.resolution_mode = "unverifiable-without-network";
    }
    return result;
  }

  if (!isPlainObject(cit.raw)) {
    result.kind = "structured";
    result.ok = false;
    result.violations.push(`malformed citation: expected a string or an object, got ${JSON.stringify(cit.raw)}`);
    return result;
  }

  // Structured citation.
  result.kind = "structured";
  const c = cit.raw;
  result.spec = c.spec;
  result.version = c.version;
  result.url = c.url;

  if (typeof c.url !== "string" || !URL_RE.test(c.url)) {
    // No URL — nothing to resolve. Shape still requires spec.
    if (typeof c.spec !== "string" || c.spec === "") {
      result.ok = false;
      result.violations.push(`malformed citation: missing "spec" (shape: {spec, version, url, clause, heading, quote, quote_sha256})`);
    }
    result.resolution_mode = "no-url";
    return result;
  }

  // Shape checks (hard).
  if (typeof c.spec !== "string" || c.spec === "") {
    result.ok = false;
    result.violations.push(`malformed citation: missing "spec"`);
  }
  if (c.version == null || c.version === "") {
    result.ok = false;
    result.violations.push(`malformed citation: missing "version"`);
  }
  if (("quote" in c) !== ("quote_sha256" in c)) {
    result.ok = false;
    result.violations.push(`malformed citation: "quote" and "quote_sha256" must appear together`);
  }
  if (typeof c.quote === "string" && typeof c.quote_sha256 === "string") {
    if (sha256(c.quote) !== c.quote_sha256 && sha256(normWS(c.quote)) !== c.quote_sha256) {
      result.ok = false;
      result.violations.push(`quote_sha256 mismatch: sha256(quote) = ${sha256(c.quote)}, recorded ${c.quote_sha256}`);
    }
    if (normWS(c.quote).length > 300) {
      result.warnings.push(`quote longer than one sentence (~${normWS(c.quote).length} chars; the contract says <= 1 sentence)`);
    }
  }
  if (result.violations.length > 0) return result;

  const snapDir =
    c.spec && c.version != null ? path.join(root, "specs", "snapshots", String(c.spec), String(c.version)) : null;

  if (snapDir && fs.existsSync(snapDir)) {
    result.resolution_mode = "snapshot";
    const text = snapshotText(snapDir);
    if (typeof c.quote === "string") {
      if (!text.includes(normWS(c.quote))) {
        result.ok = false;
        result.violations.push(
          `quote not found in pinned snapshot specs/snapshots/${c.spec}/${c.version}/ (quote_sha256 ${c.quote_sha256})`,
        );
      }
    }
    const fragment = c.url.includes("#") ? c.url.split("#").pop() : null;
    if (fragment && !text.includes(fragment)) {
      result.warnings.push(`anchor "#${fragment}" not found in pinned snapshot (not anchor-addressable?)`);
    }
    return result;
  }

  if (catalogRegistered(c.spec, c.version)) {
    result.resolution_mode = "pinned-version-only";
    return result;
  }

  if (opts.network) {
    const head = await headRequest(c.url);
    result.resolution_mode = "network-head";
    result.http_status = head.status;
    if (!head.ok) {
      result.ok = false;
      result.warnings.push(`HEAD ${c.url} failed (${head.status ?? head.error})`);
    }
    return result;
  }

  result.resolution_mode = "unverifiable-without-network";
  return result;
}

// ── Main ────────────────────────────────────────────────────────────────────
const all = [];
for (const id of formats) {
  for (const name of ["spec.yaml", "dossier.yaml"]) {
    const file = path.join(root, "core", "formats", id, name);
    if (!fs.existsSync(file)) continue;
    let doc;
    try {
      doc = loadYamlFile(file);
    } catch (e) {
      all.push({
        format: id,
        file: path.relative(root, file),
        locator: "(file)",
        kind: "structured",
        ok: false,
        violations: [`unparseable YAML: ${e.message}`],
        warnings: [],
      });
      continue;
    }
    const found = [];
    collectCitations(doc, file, "", found);
    for (const f of found) all.push(await checkCitation({ ...f, format: id }));
  }
}

const violations = all.filter((r) => r.violations.length > 0);
const byMode = {};
for (const r of all) byMode[r.resolution_mode ?? "n/a"] = (byMode[r.resolution_mode ?? "n/a"] ?? 0) + 1;

const report = {
  generated_at: new Date().toISOString(),
  network: !!opts.network,
  formats: formats.length,
  citations: all.length,
  by_resolution_mode: byMode,
  hard_violations: violations.length,
  results: all,
};

const summaryLines = [
  `check-citations: ${formats.length} format(s), ${all.length} citation(s)`,
  ...Object.entries(byMode).map(([m, n]) => `  ${m}: ${n}`),
  ...violations.flatMap((r) => r.violations.map((v) => `violation: ${r.file} ${r.locator}: ${v}`)),
  ...all.flatMap((r) => r.warnings.map((w) => `warning: ${r.file} ${r.locator}: ${w}`)),
  violations.length > 0 ? `FAIL (${violations.length} hard violation(s))` : "OK",
];

if (opts.json) {
  process.stdout.write(JSON.stringify(report, null, 2) + "\n");
  process.stderr.write(summaryLines.join("\n") + "\n");
} else {
  process.stdout.write(summaryLines.join("\n") + "\n");
}
process.exit(violations.length > 0 ? 1 : 0);
