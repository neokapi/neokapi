#!/usr/bin/env node
//
// Report target entries whose source string changed under them.
//
// A KBF catalog keys on FNV-1a-64(text|desc), so rewording a source string
// changes the key: the old target simply stops being emitted and the locale
// falls back to English. That is the drift the loop is designed to tolerate, and
// it needs no report.
//
// The catalogs whose shape mirrors their source document behave the other way.
// `host/i18n/catalogs/<lang>.json` and `core/i18n/catalogs/<lang>.json` are
// addressed by scope path — `cli.commands.kapi.add.short` — and a reworded
// source keeps the scope, so the previous translation stays attached to it. The
// locale then ships a translation of a sentence the source no longer contains:
// not a missing translation but a wrong one, indistinguishable from a current
// one at every layer that reads only today's files.
//
// The absorber refuses such a pairing when the rewrite moved a placeholder, and
// when the project store still holds the wording the unit had last run. Neither
// signal reaches a rewrite that kept the placeholders on a checkout with no
// store — a fresh clone, which is what CI is. What does reach it is the one
// record of "what was this a translation of" that survives a clone: git.
//
// For each translated leaf, the commit where that leaf's TEXT last changed is
// the commit the translation was produced at, and the source document at that
// commit is the wording it translates. A leaf whose source has moved since is
// reported here.
//
// It reports and never gates. Acting on an entry is a decision — re-translate
// it where it is reviewed, or remove it so the locale falls back to English
// until the next convergence — and neither belongs to a build.
//
// Usage:
//     node scripts/l10n-stale-target-report.mjs <lang> <target>:<source>...
//
// Run it through `make l10n-stale-report`. Surfaces measured against the `qps`
// probe are skipped: their keys are content-addressed, so a rewrite orphans the
// entry rather than re-pointing it.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";

const [lang, ...pairs] = process.argv.slice(2);

if (!lang || pairs.length === 0) {
  console.error("usage: l10n-stale-target-report.mjs <lang> <target>:<source>...");
  process.exit(2);
}

const MAX_EXAMPLES = 20;

function git(args) {
  return execFileSync("git", args, {
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
    stdio: ["ignore", "pipe", "ignore"],
  });
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

function leavesAt(rev, path) {
  try {
    return leaves(JSON.parse(git(["show", `${rev}:${path}`])), "", new Map());
  } catch {
    return null;
  }
}

function leavesOnDisk(path) {
  try {
    return leaves(JSON.parse(readFileSync(path, "utf8")), "", new Map());
  } catch {
    return null;
  }
}

/** True when the checkout carries enough history to answer the question. */
function haveHistory() {
  try {
    return git(["rev-parse", "--is-shallow-repository"]).trim() === "false";
  } catch {
    return false;
  }
}

/**
 * The two wordings shown from where they part company. A help string's rewrite
 * is usually a paragraph appended to five that did not change, so a table of
 * prefixes would print the same cell twice and say nothing.
 */
function atDivergence(then, today) {
  if (then === undefined) return ["— (absent)", JSON.stringify(clip(today, 0))];
  let i = 0;
  while (i < then.length && i < today.length && then[i] === today[i]) i++;
  const from = Math.max(0, i - 10);
  const lead = from > 0 ? "…" : "";
  return [
    JSON.stringify(lead + clip(then, from)),
    JSON.stringify(lead + clip(today, from)),
  ];
}

const clip = (s, from) => {
  const rest = s.slice(from).replace(/\n/g, " ");
  return rest.length > 60 ? `${rest.slice(0, 60)}…` : rest;
};

const out = [];
out.push(`### Target entries whose source moved under them — \`${lang}\``);
out.push("");

if (!haveHistory()) {
  out.push(
    "Skipped: this checkout is shallow, and the wording a translation was " +
      "produced from lives in its history. Fetch the full history to read it.",
  );
  out.push("");
  console.log(out.join("\n"));
  process.exit(0);
}

const stale = [];
let scanned = 0;
let surfaces = 0;

for (const pair of pairs) {
  const sep = pair.lastIndexOf(":");
  const target = pair.slice(0, sep);
  const source = pair.slice(sep + 1);
  // A probe-referenced surface is content-addressed; a rewrite orphans its
  // entry, which the orphan report already covers.
  if (source.endsWith("/qps.json")) continue;
  if (!existsSync(target) || !existsSync(source)) continue;

  const now = leavesOnDisk(target);
  const src = leavesOnDisk(source);
  if (now === null || src === null) continue;
  surfaces++;

  let revs;
  try {
    revs = git(["log", "--format=%H", "--", target]).split("\n").filter(Boolean);
  } catch {
    continue;
  }
  // One read per revision of the target, reused across every leaf: these
  // catalogs move a handful of times, and the alternative is a git call per
  // string.
  const history = revs.map((rev) => ({
    rev,
    at: leavesAt(rev, target) ?? new Map(),
    before: leavesAt(`${rev}^`, target) ?? new Map(),
    source: leavesAt(rev, source) ?? new Map(),
  }));

  for (const [key, text] of now) {
    const today = src.get(key);
    if (today === undefined || today === text) continue; // absent or untranslated
    scanned++;
    const produced = history.find((h) => h.at.get(key) === text && h.before.get(key) !== text);
    if (!produced) continue; // never committed under this wording
    const then = produced.source.get(key);
    if (then !== undefined && then === today) continue;
    stale.push({ target, key, rev: produced.rev.slice(0, 9), then, today });
  }
}

if (surfaces === 0) {
  out.push("No scope-addressed catalog to read.");
} else if (stale.length === 0) {
  out.push(
    `Every one of the ${scanned} translated entr(y/ies) across ${surfaces} ` +
      "scope-addressed catalog(s) translates the wording that is beside it today.",
  );
} else {
  out.push(
    `${stale.length} of ${scanned} translated entr(y/ies) translate wording the source ` +
      "no longer carries. The locale ships them as current, because a scope-addressed " +
      "catalog keeps a translation attached to its scope when the sentence under it is " +
      "rewritten.",
  );
  out.push("");
  out.push("| Surface | Scope | Translated at | Source then | Source now |");
  out.push("| --- | --- | --- | --- | --- |");
  for (const s of stale.slice(0, MAX_EXAMPLES)) {
    const [then, today] = atDivergence(s.then, s.today);
    out.push(`| \`${s.target}\` | \`${s.key}\` | \`${s.rev}\` | ${then} | ${today} |`);
  }
  if (stale.length > MAX_EXAMPLES) {
    out.push("");
    out.push(`…and ${stale.length - MAX_EXAMPLES} more.`);
  }
  out.push("");
  out.push(
    "Reported, never gated. Re-translate the entry where it is reviewed, or remove " +
      "it so the locale falls back to English until the next convergence writes it " +
      "again — a wrong translation is not an approved one, so removing it keeps no " +
      "decision from anybody.",
  );
}
out.push("");

console.log(out.join("\n"));
