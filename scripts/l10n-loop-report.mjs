#!/usr/bin/env node
//
// Report the standing of the target-language tier: how much of each surface is
// translated, and whether the placeholders survived.
//
// The artifacts this reads are the loop's, not the build's. `kapi up` writes
// them out of the project store, which holds the union of what git carries and
// what a venue pull brought home, so no checkout can be asked to reproduce them
// byte for byte — a gate that demanded it would overwrite approved wording the
// moment it could not. What can honestly be said about them is how far along
// they are, and that is what this says.
//
// It never gates. Coverage below any bar is pending work, which is the normal
// state this loop exists to absorb (CLAUDE.md, "Target-language drift must
// never block the build"). A placeholder mismatch is a defect worth a human's
// attention and still not a build break: three layers already stop one from
// shipping — `kapi memory import` warns on an asymmetric entry, `recycle`
// refuses the fill, and `placeholder-check` reports it as critical — so what is
// left here is visibility.
//
// Each surface is measured against a reference document that carries every
// source string:
//
//   * the qps probe (`translations/qps.json`) where a surface has one. Pseudo-
//     translation expands every string and leaves its placeholders intact, so a
//     key present in the probe and absent from the target is untranslated, and
//     the probe's token set is the source's.
//   * the source document otherwise (the generated inventories, the English
//     subject catalog). There a leaf equal to its source counts as
//     untranslated, which under-reports a surface whose correct translation is
//     the English word — the safe direction for a number a human reads.
//
// Usage:
//     node scripts/l10n-loop-report.mjs <lang> <target>:<reference>...
//
// Run it through `make l10n-report`, which knows the pairs. Output is markdown,
// which reads as a table in a terminal and as one in a job summary.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { basename, join } from "node:path";

const [lang, ...pairs] = process.argv.slice(2);

if (!lang || pairs.length === 0) {
  console.error("usage: l10n-loop-report.mjs <lang> <target>:<reference>...");
  process.exit(2);
}

const DEMO_DIR = "harness/demos";
const MAX_EXAMPLES = 20;

/**
 * Placeholder tokens: braced parameters and KBF runtime-projection markup
 * (`{count}`, `{=m0}`, `{/=m0}`) plus printf verbs. A target that dropped one
 * ships a sentence with a hole where a count, a name or a link belongs.
 *
 * The braced form is deliberately narrow — an identifier, optionally prefixed
 * by the projection markers — because CLI help quotes JSON, and a translated
 * word inside `{"reason":"findings"}` is prose, not a placeholder that moved.
 */
const TOKEN = /\{\/?=?[A-Za-z_][A-Za-z0-9_.-]*\}|%[-+ #0]*[0-9.*]*[sdvqxXofFeEgGtTcpbU%]/g;

function tokens(text) {
  return (text.match(TOKEN) ?? []).sort();
}

function readJSON(path) {
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch {
    return null;
  }
}

/**
 * The same document as committed on this branch, or null when it is not there.
 * The comparison against it is the one number that catches an erasure: a
 * convergence run from a store colder than the one that produced the committed
 * artifact writes fewer translations, and every other check reads that as a
 * legitimate regeneration.
 */
function readJSONAtHEAD(path) {
  try {
    return JSON.parse(
      execFileSync("git", ["show", `HEAD:${path}`], {
        encoding: "utf8",
        maxBuffer: 64 * 1024 * 1024,
        stdio: ["ignore", "pipe", "ignore"],
      }),
    );
  } catch {
    return null;
  }
}

/** Every string leaf of a parsed JSON document, as key path → text. */
function leaves(node, prefix, out) {
  if (typeof node === "string") {
    out.set(prefix, node);
  } else if (Array.isArray(node)) {
    node.forEach((v, i) => leaves(v, `${prefix}[${i}]`, out));
  } else if (node !== null && typeof node === "object") {
    for (const [k, v] of Object.entries(node)) leaves(v, prefix ? `${prefix}.${k}` : k, out);
  }
  return out;
}

const rows = [];
const mismatches = [];

for (const pair of pairs) {
  const sep = pair.lastIndexOf(":");
  const target = pair.slice(0, sep);
  const reference = pair.slice(sep + 1);
  // The probe is a full expansion of the source strings; a source document is
  // the source strings themselves. The two answer "is this leaf translated?"
  // differently, and nothing else about them differs.
  const againstProbe = basename(reference) === "qps.json";

  if (!existsSync(reference)) {
    rows.push({ target, note: `no reference (${reference})` });
    continue;
  }
  const ref = leaves(readJSON(reference) ?? {}, "", new Map());
  const tgt = existsSync(target) ? leaves(readJSON(target) ?? {}, "", new Map()) : new Map();

  const countTranslated = (doc) => {
    let n = 0;
    for (const [key, refText] of ref) {
      const text = doc.get(key);
      if (text === undefined) continue;
      if (againstProbe || text !== refText) n++;
    }
    return n;
  };

  for (const [key, refText] of ref) {
    const text = tgt.get(key);
    if (text === undefined) continue;
    const want = tokens(refText);
    const got = tokens(text);
    if (want.join(" ") !== got.join(" ")) {
      mismatches.push({ target, key, want, got });
    }
  }

  const head = readJSONAtHEAD(target);
  rows.push({
    target,
    translated: countTranslated(tgt),
    total: ref.size,
    atHEAD: head === null ? null : countTranslated(leaves(head, "", new Map())),
  });
}

// The narration sidecars are their own shape: a demo has one exactly when its
// narration has been translated, because a sidecar byte-identical to its source
// is dropped rather than committed.
let demos = 0;
let narrated = 0;
if (existsSync(DEMO_DIR)) {
  for (const entry of readdirSync(DEMO_DIR, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    if (!existsSync(join(DEMO_DIR, entry.name, "demo.yaml"))) continue;
    demos++;
    if (existsSync(join(DEMO_DIR, entry.name, `demo.${lang}.yaml`))) narrated++;
  }
}

const pct = (n, total) => (total === 0 ? "—" : `${Math.round((n / total) * 100)}%`);

/**
 * The move against what is committed. A positive number is the loop catching
 * up; a negative one is a run that produced fewer translations than the
 * artifact it is about to overwrite, which is what an erasure looks like from
 * the outside. Reported, not gated — but reported loudly, because every other
 * check reads that run as a legitimate regeneration.
 */
const delta = (now, before) => {
  if (before === null) return "new";
  const d = now - before;
  if (d === 0) return "—";
  return d > 0 ? `+${d}` : `${d} ⚠`;
};

const out = [];
out.push(`### Loop-owned tier — \`${lang}\``);
out.push("");
out.push("| Surface | Translated | Of | Coverage | vs HEAD |");
out.push("| --- | ---: | ---: | ---: | ---: |");
for (const row of rows) {
  if (row.note) {
    out.push(`| \`${row.target}\` | — | — | ${row.note} | — |`);
    continue;
  }
  out.push(
    `| \`${row.target}\` | ${row.translated} | ${row.total} | ` +
      `${pct(row.translated, row.total)} | ${delta(row.translated, row.atHEAD)} |`,
  );
}
out.push(
  `| \`${DEMO_DIR}/*/demo.${lang}.yaml\` | ${narrated} | ${demos} | ` +
    `${pct(narrated, demos)} | — |`,
);
out.push("");
out.push(
  "Coverage is reported, never gated: a source change that outruns its " +
    "translations is the normal state the loop absorbs. A negative *vs HEAD* is " +
    "different — it is this run producing less than the artifact already in git, " +
    "which is wording the store no longer holds.",
);
out.push("");

out.push(`### Placeholder parity — \`${lang}\``);
out.push("");
if (mismatches.length === 0) {
  out.push("Every translated string carries exactly the placeholders its source carries.");
} else {
  out.push(
    `${mismatches.length} string(s) whose placeholders differ from their source. ` +
      "A target missing one ships a sentence with a hole in it; a target with an " +
      "extra one shows a literal brace to a reader.",
  );
  out.push("");
  out.push("| Surface | Key | Source | Target |");
  out.push("| --- | --- | --- | --- |");
  for (const m of mismatches.slice(0, MAX_EXAMPLES)) {
    const fmt = (list) => (list.length === 0 ? "—" : list.map((t) => `\`${t}\``).join(" "));
    out.push(`| \`${m.target}\` | \`${m.key}\` | ${fmt(m.want)} | ${fmt(m.got)} |`);
  }
  if (mismatches.length > MAX_EXAMPLES) {
    out.push("");
    out.push(`…and ${mismatches.length - MAX_EXAMPLES} more.`);
  }
}
out.push("");

console.log(out.join("\n"));
