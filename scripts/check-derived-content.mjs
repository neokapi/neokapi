#!/usr/bin/env node
//
// Guard: a derived artifact is committable only when what was written in it is
// sound.
//
// Every check between the loop and production used to classify *which* files a
// convergence run touched. That predicate cannot see a run that wrote the wrong
// content to the right files, and twice it did not: a return leg wrote
// structurally corrupt Norwegian into the narration sidecars, and a placeholder
// that lost its hole shipped a sentence with a gap where a count, a name or a
// link belongs. Both runs moved exactly the files a convergence run is supposed
// to move, so every file-shaped gate agreed.
//
// This reads the artifacts instead. Three questions, each answerable from the
// artifact and the document it derives from, and none of them a coverage bar:
//
//   parses        — a JSON artifact must parse. A target the runtime cannot
//                   read is not partial coverage, it is a broken surface.
//   placeholders  — a leaf must carry exactly the interpolation placeholders
//                   its source carries. A target missing one renders a hole; a
//                   target with an extra one shows a literal brace to a reader.
//   identifiers   — a leaf the recipe does not declare translatable must be the
//                   source's own bytes. `id: discover` translated to
//                   `id: oppdag` is not drift, it is an identifier the harness
//                   matches an overlay by, rewritten into something that matches
//                   nothing.
//
// What it deliberately never asserts: coverage. A leaf absent from the target,
// or equal to its source, is pending work — the normal state this loop exists to
// absorb (CLAUDE.md, "Target-language drift must never block the build"). Only a
// leaf that IS there and is wrong is refused.
//
// The narration sidecars are checked by the same rules as everything else. They
// were exempted from the byte gate for a good reason — the target tier cannot be
// reproduced byte for byte by a checkout with no server — and that reason says
// nothing about whether their structure survived, which is what travelled
// through the exemption.
//
// Usage:
//     node scripts/check-derived-content.mjs <lang> <target>:<reference>...
//     node scripts/check-derived-content.mjs <lang> --only <path> ... <pair>...
//     node scripts/check-derived-content.mjs --backing <path>...
//     node scripts/check-derived-content.mjs --self-test
//
// `--only` restricts the walk to the named artifacts, which is how the erasure
// gate asks about exactly what a run wrote. `--backing` answers the other half
// of the same question — whether a change under `.kapi/` says anything, or
// merely re-serializes what was already there.
//
// Run it through `make l10n-content-check` (the whole committed tier) or
// `scripts/check-sync-backed.sh` (what a run wrote).

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

const DEMO_DIR = "harness/demos";

// ── what the recipe declares translatable ────────────────────────────────────
//
// Read from kapi.yaml at run time. The narration sidecars take the
// `keyPathPatterns` of the item that reads `harness/demos/*/demo.yaml`, merged
// over `defaults.formats.yaml.config` the way the engine merges a format's
// defaults with an item's own config. The engine catalog takes the
// `extractionRules` of the item that reads `core/i18n/builtins/metadata.json`.
// A leaf outside either is a machine identifier the target must carry through.

const RECIPE = "kapi.yaml";
const ENGINE_SOURCE = "core/i18n/builtins/metadata.json";
const DEMO_SOURCE = `${DEMO_DIR}/*/demo.yaml`;

/**
 * What kapi.yaml declares translatable on the two surfaces measured by rule: a
 * predicate over the engine catalog's dotted key paths, and the leaf key names
 * a narration sidecar may rewrite. Either is `null` when every leaf is prose.
 * A recipe that declares neither item throws, because a gate that cannot read
 * the rule would pass every translated identifier.
 */
function translatableRules(recipeText) {
  const recipe = parseYAML(recipeText) ?? {};
  const engine = recipeItem(recipe, ENGINE_SOURCE);
  if (engine === null) throw new Error(`${RECIPE} declares no content item for ${ENGINE_SOURCE}`);
  const demo = recipeItem(recipe, DEMO_SOURCE);
  if (demo === null) throw new Error(`${RECIPE} declares no content item for ${DEMO_SOURCE}`);
  return {
    engine: keyRule(readerConfig(recipe, engine, "json")),
    demo: leafKeys(readerConfig(recipe, demo, "yaml").keyPathPatterns),
  };
}

/** The content item whose collection base and path spell `file`, or null. */
function recipeItem(recipe, file) {
  for (const collection of recipe.collections ?? []) {
    for (const item of collection?.content ?? []) {
      if ([collection.base, item?.path].filter(Boolean).join("/") === file) return item;
    }
  }
  return null;
}

/** An item's reader config: its format's defaults, then the item's own keys. */
function readerConfig(recipe, item, fallbackFormat) {
  const name = item.format?.name ?? fallbackFormat;
  return { ...(recipe.defaults?.formats?.[name]?.config ?? {}), ...(item.format?.config ?? {}) };
}

/**
 * The leaves a JSON reader config names: `extractionRules` when it is set,
 * otherwise every leaf less `exceptions`, or only `exceptions` when
 * `extractAllPairs` is off. The rule is tested against the dotted key path, so
 * an alternative such as `enumDescriptions\.[^.]+$` reaches the leaf it names.
 */
function keyRule(config) {
  const rules = config.extractionRules ? new RegExp(config.extractionRules) : null;
  const exceptions = config.exceptions ? new RegExp(config.exceptions) : null;
  if (rules) return (keyPath) => rules.test(keyPath);
  if (config.extractAllPairs !== false) {
    return exceptions ? (keyPath) => !exceptions.test(keyPath) : null;
  }
  return exceptions ? (keyPath) => exceptions.test(keyPath) : () => false;
}

/** The leaf key names a list of YAML key path patterns selects, or null for all. */
function leafKeys(patterns) {
  if (!Array.isArray(patterns) || patterns.length === 0) return null;
  const leaves = patterns.map((pattern) => String(pattern).split(".").pop());
  return leaves.some((leaf) => leaf.includes("*")) ? null : new Set(leaves);
}

// ── the recipe's YAML ────────────────────────────────────────────────────────

const MAPPING_ENTRY = /^("(?:[^"\\]|\\.)*"|'(?:[^']|'')*'|[^\s"'#?:,[\]{}][^:]*?)\s*:(?:\s+(.*))?$/;

/**
 * Parse the YAML subset a recipe is written in: block mappings and sequences,
 * plain and quoted scalars, flow sequences of scalars, and comments. A block
 * scalar is read as its raw text. Anchors, aliases, tags and flow mappings
 * throw, so a recipe that grows past the subset fails the gate by name.
 */
function parseYAML(text) {
  const lines = text.split(/\r?\n/);
  let i = 0;

  const indentOf = (line) => /^ */.exec(line)[0].length;
  const bodyOf = (line) => stripComment(line).trim();
  const isItem = (body) => body === "-" || body.startsWith("- ");
  const fail = (why) => {
    throw new Error(`YAML line ${i + 1}: ${why}`);
  };
  const skipBlank = () => {
    while (i < lines.length && bodyOf(lines[i]) === "") i++;
  };

  function node(minIndent) {
    skipBlank();
    if (i >= lines.length || indentOf(lines[i]) < minIndent) return null;
    const indent = indentOf(lines[i]);
    return isItem(bodyOf(lines[i])) ? sequence(indent) : mapping(indent);
  }

  function sequence(indent) {
    const out = [];
    for (skipBlank(); i < lines.length; skipBlank()) {
      const line = lines[i];
      const body = bodyOf(line);
      if (indentOf(line) !== indent || !isItem(body)) break;
      const rest = body.slice(1).trimStart();
      if (rest === "") {
        i++;
        out.push(node(indent + 1));
      } else if (MAPPING_ENTRY.test(rest)) {
        // `- key: value` opens a mapping at the column its first key sits in.
        const column = line.length - line.slice(indent + 1).trimStart().length;
        lines[i] = " ".repeat(column) + line.slice(column);
        out.push(mapping(column));
      } else {
        i++;
        out.push(scalar(rest));
      }
    }
    return out;
  }

  function mapping(indent) {
    const out = {};
    for (skipBlank(); i < lines.length; skipBlank()) {
      const line = lines[i];
      const body = bodyOf(line);
      const at = indentOf(line);
      if (at < indent || (at === indent && isItem(body))) break;
      if (at > indent) fail("unexpected indentation");
      const entry = MAPPING_ENTRY.exec(body);
      if (!entry) fail(`expected a mapping entry, got ${JSON.stringify(body)}`);
      const key = /^["']/.test(entry[1]) ? scalar(entry[1]) : entry[1];
      const value = (entry[2] ?? "").trim();
      i++;
      if (/^[|>][-+]?\d*$/.test(value)) {
        const block = [];
        while (i < lines.length && (lines[i].trim() === "" || indentOf(lines[i]) > indent)) {
          block.push(lines[i++]);
        }
        out[key] = block.join("\n");
      } else if (value !== "") {
        out[key] = scalar(value);
      } else {
        skipBlank();
        const next = lines[i] ?? "";
        const opens =
          i < lines.length &&
          (indentOf(next) > indent || (indentOf(next) === indent && isItem(bodyOf(next))));
        out[key] = opens ? node(indent) : null;
      }
    }
    return out;
  }

  return node(0);
}

/** A line with its trailing comment removed; a `#` inside quotes is text. */
function stripComment(line) {
  let quote = "";
  for (let k = 0; k < line.length; k++) {
    const ch = line[k];
    if (quote === '"') {
      if (ch === "\\") k++;
      else if (ch === '"') quote = "";
    } else if (quote === "'") {
      if (ch === "'") quote = "";
    } else if ((ch === '"' || ch === "'") && (k === 0 || /[\s:[{,-]/.test(line[k - 1]))) {
      quote = ch;
    } else if (ch === "#" && (k === 0 || /\s/.test(line[k - 1]))) {
      return line.slice(0, k);
    }
  }
  return line;
}

/** One YAML scalar or flow sequence of scalars. */
function scalar(raw) {
  const value = raw.trim();
  if (value.startsWith('"')) return JSON.parse(value);
  if (value.startsWith("'")) return value.slice(1, -1).replaceAll("''", "'");
  if (value.startsWith("[")) {
    if (!value.endsWith("]")) throw new Error(`unterminated flow sequence: ${value}`);
    const inner = value.slice(1, -1).trim();
    return inner === "" ? [] : splitFlow(inner).map(scalar);
  }
  if (/^[{&*!]/.test(value)) throw new Error(`YAML outside the recipe subset: ${value}`);
  if (value === "true" || value === "false") return value === "true";
  if (value === "null" || value === "~") return null;
  if (/^-?\d+(\.\d+)?$/.test(value)) return Number(value);
  return value;
}

/** The items of a flow sequence's body, split on commas outside quotes. */
function splitFlow(inner) {
  const parts = [];
  let quote = "";
  let start = 0;
  for (let k = 0; k < inner.length; k++) {
    const ch = inner[k];
    if (quote) {
      if (ch === quote) quote = "";
    } else if (ch === '"' || ch === "'") {
      quote = ch;
    } else if (ch === ",") {
      parts.push(inner.slice(start, k));
      start = k + 1;
    }
  }
  parts.push(inner.slice(start));
  return parts;
}

/**
 * Placeholder tokens: braced parameters and KBF runtime-projection markup
 * (`{count}`, `{=m0}`, `{/=m0}`) plus printf verbs. The braced form is
 * deliberately narrow — an identifier, optionally prefixed by the projection
 * markers — because CLI help quotes JSON, and a translated word inside
 * `{"reason":"findings"}` is prose, not a placeholder that moved.
 *
 * The same expression `scripts/l10n-loop-report.mjs` reports on, so the gate and
 * the report cannot disagree about what a placeholder is.
 */
const TOKEN = /\{\/?=?[A-Za-z_][A-Za-z0-9_.-]*\}|%[-+ #0]*[0-9.*]*[sdvqxXofFeEgGtTcpbU%]/g;

function tokens(text) {
  return (text.match(TOKEN) ?? []).sort();
}

/**
 * ICU MessageFormat chooses between sub-messages, and how many times a
 * placeholder occurs inside one is a property of the language: Norwegian writes
 * two branches where Polish writes four. So a message carrying a picker is
 * compared by the set of argument names it mentions rather than by occurrence
 * count — the same distinction `core/tools/placeholders.go` draws, which is what
 * #2020 settled.
 */
const ICU_PICKER = /\{\s*[A-Za-z_][A-Za-z0-9_.-]*\s*,\s*(plural|select|selectordinal)\s*[,}]/;

function hasPicker(text) {
  return ICU_PICKER.test(text);
}

/**
 * What a picker message is compared on: the picker's own head (`{n, plural`),
 * the KBF runtime-projection markers, and the interpolation styles ICU steps
 * over as text. All three are unambiguous inside a message.
 *
 * A bare `{name}` inside a picker is not, and is deliberately left out: in
 * `{gender, select, male {he} other {they}}` the sub-message bodies are spelled
 * exactly like arguments, and telling them apart needs the ICU parse the loop
 * already runs — `core/icu` behind the `placeholder-check` tool the recipe
 * binds, which reports a dropped argument as a critical finding at convergence
 * time. A guard that guessed here would refuse correct plurals, which is the
 * defect #2020 was.
 */
const PICKER_HEAD = /\{\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*,\s*(?:plural|select|selectordinal)\b/g;
const MARKER = /\{\/?=m\d+\}/g;
const PRINTF = /%[-+ #0]*[0-9.*]*[sdvqxXofFeEgGtTcpbU%]/g;

function matchSet(re, text) {
  return [...new Set([...text.matchAll(re)].map((m) => m[0]))].sort();
}

/** What a picker message owes its source, order and occurrence count removed. */
function pickerTokens(text) {
  return [...matchSet(PICKER_HEAD, text), ...matchSet(MARKER, text), ...matchSet(PRINTF, text)]
    .sort();
}

/**
 * Whether a translation carries its source's placeholders, and what it lost or
 * invented when it does not.
 */
function placeholderDelta(source, target) {
  const counted = !hasPicker(source) && !hasPicker(target);
  const want = counted ? tokens(source) : pickerTokens(source);
  const got = counted ? tokens(target) : pickerTokens(target);
  if (want.join(" ") === got.join(" ")) return null;

  const rest = [...got];
  const missing = [];
  for (const token of want) {
    const at = rest.indexOf(token);
    if (at === -1) missing.push(token);
    else rest.splice(at, 1);
  }
  if (missing.length === 0 && rest.length === 0) return null;
  return { missing, extra: rest };
}

// ── documents ────────────────────────────────────────────────────────────────

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

/**
 * Scalars of a YAML document as (key name, value) in document order.
 *
 * A reader rather than a parser, and that is the point: the guard has to run in
 * a bare checkout with no package installed, so it may not import one. What it
 * needs is narrower than YAML — the key a scalar sits under and the bytes of
 * that scalar — and the documents it reads are written by kapi's own YAML
 * writer, which reconstructs them from the master's bytes. A sequence item takes
 * the key of the mapping that owns it, which is what makes `aspects:` answerable
 * without tracking indices.
 *
 * Block scalars are read whole, so a translated `command: |` block is a value
 * like any other rather than lines the scan steps over.
 */
function yamlScalars(text) {
  const lines = text.split("\n");
  const out = [];
  const stack = []; // { indent, key }
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.trim() === "" || /^\s*#/.test(line)) continue;

    const indentMatch = /^(\s*)/.exec(line);
    let indent = indentMatch[1].length;
    let rest = line.slice(indent);

    // `- ` opens a sequence item; `- key: value` opens a mapping inside one.
    while (rest.startsWith("- ") || rest === "-") {
      indent += 2;
      rest = rest.slice(2);
      if (rest.trim() === "") break;
    }

    const kv = /^([A-Za-z_][A-Za-z0-9_-]*):(?:\s(.*))?$/.exec(rest);
    if (kv) {
      const key = kv[1];
      while (stack.length > 0 && stack[stack.length - 1].indent >= indent) stack.pop();
      const value = kv[2] === undefined ? "" : kv[2];
      if (value === "") {
        stack.push({ indent, key });
        continue;
      }
      if (/^[|>][-+]?\d*$/.test(value.trim())) {
        const block = [];
        while (i + 1 < lines.length) {
          const next = lines[i + 1];
          if (next.trim() !== "" && /^(\s*)/.exec(next)[1].length <= indent) break;
          block.push(next);
          i++;
        }
        out.push({ key, value: block.join("\n").trimEnd() });
        continue;
      }
      out.push({ key, value: value.trim() });
      continue;
    }

    // A bare sequence scalar belongs to the mapping key above it.
    if (rest.trim() !== "" && stack.length > 0) {
      out.push({ key: stack[stack.length - 1].key, value: rest.trim() });
    }
  }
  return out;
}

/** key name → the multiset of values a document gives it. */
function valuesByKey(scalars) {
  const out = new Map();
  for (const { key, value } of scalars) {
    const counts = out.get(key) ?? new Map();
    counts.set(value, (counts.get(value) ?? 0) + 1);
    out.set(key, counts);
  }
  return out;
}

function readJSON(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}

// ── the checks ───────────────────────────────────────────────────────────────

/**
 * Which leaves of a JSON target the recipe declares translatable. `null` means
 * every leaf is — the surfaces whose whole document is extracted.
 */
function jsonTranslatable(target, rules) {
  if (/(^|\/)core\/i18n\/catalogs\/[^/]+\.json$/.test(target)) return rules.engine;
  return null;
}

/** Defects in one JSON artifact, measured against the document it derives from. */
function checkJSON(target, reference, rules) {
  const defects = [];
  let doc;
  try {
    doc = readJSON(target);
  } catch (err) {
    return [{ target, key: "", kind: "unparseable", detail: String(err.message ?? err) }];
  }
  let ref;
  try {
    ref = readJSON(reference);
  } catch (err) {
    return [{ target, key: "", kind: "reference-unreadable", detail: String(err.message ?? err) }];
  }

  const refLeaves = leaves(ref, "", new Map());
  const tgtLeaves = leaves(doc, "", new Map());
  const translatable = jsonTranslatable(target, rules);
  // A surface measured against its source document mirrors that document's
  // shape, so a leaf the source does not have is structure the target invented.
  // A surface measured against the `qps` probe is keyed by the hash of its
  // source string: a key the probe no longer carries is a string that left the
  // source, which orphans the entry rather than corrupting it — the runtime
  // never looks it up, and the next compile drops it.
  const mirrorsSource = !reference.endsWith("/qps.json");

  for (const [key, text] of tgtLeaves) {
    const source = refLeaves.get(key);
    if (source === undefined) {
      if (mirrorsSource) {
        defects.push({
          target,
          key,
          kind: "invented",
          detail: "no such leaf in the source document",
        });
      }
      continue;
    }
    if (translatable !== null && !translatable(key) && text !== source) {
      defects.push({
        target,
        key,
        kind: "identifier",
        detail: `${JSON.stringify(source)} → ${JSON.stringify(text)}`,
      });
      continue;
    }
    const delta = placeholderDelta(source, text);
    if (delta) {
      defects.push({
        target,
        key,
        kind: "placeholder",
        detail: describeDelta(delta),
      });
    }
  }
  return defects;
}

function describeDelta({ missing, extra }) {
  const parts = [];
  if (missing.length > 0) parts.push(`missing ${missing.join(" ")}`);
  if (extra.length > 0) parts.push(`extra ${extra.join(" ")}`);
  return parts.join(", ");
}

/**
 * Defects in one narration sidecar, measured against the master it overlays.
 *
 * A sidecar is a reconstruction of its master with the translatable scalars
 * replaced, so every other scalar must be one the master gives that key. That is
 * the whole check, and it is what tells `kind: use-case` translated to
 * `kind: brukstilfelle` from a narration line that was legitimately rewritten.
 */
function checkSidecar(target, master, rules) {
  if (!existsSync(master)) {
    return [{ target, key: "", kind: "orphan", detail: `no master beside it (${master})` }];
  }
  const defects = [];
  const src = valuesByKey(yamlScalars(readFileSync(master, "utf8")));
  const scalars = yamlScalars(readFileSync(target, "utf8"));

  for (const { key, value } of scalars) {
    if (rules.demo === null || rules.demo.has(key)) continue;
    const known = src.get(key);
    if (known === undefined) {
      defects.push({ target, key, kind: "invented", detail: "no such key in the master" });
      continue;
    }
    if (!known.has(value)) {
      defects.push({
        target,
        key,
        kind: "identifier",
        detail: `${JSON.stringify(value)} is not a value the master gives ${key}`,
      });
    }
  }

  // Structure, not coverage: a sidecar overlays its master scene for scene, so
  // it carries the master's keys exactly once each time the master does. A
  // sidecar that dropped scenes matches nothing at load time.
  const counts = valuesByKey(scalars);
  for (const [key, values] of src) {
    const want = [...values.values()].reduce((a, b) => a + b, 0);
    const got = [...(counts.get(key) ?? new Map()).values()].reduce((a, b) => a + b, 0);
    if (got !== want) {
      defects.push({
        target,
        key,
        kind: "structure",
        detail: `master carries ${key} ${want}×, the sidecar ${got}×`,
      });
    }
  }
  return defects;
}

// ── backing: does a change under .kapi/ say anything? ────────────────────────

/**
 * A change under `.kapi/` is what tells a reviewer a derived artifact was
 * decided rather than merely regenerated, so it has to be read the way a
 * reviewer would: by what it says, not by whether the bytes moved.
 *
 * `ktb.Marshal` emits a canonical document — each concept's `terms[]` sorted —
 * and a committed file written before that guarantee normalizes on its first
 * pass through. Two array orderings swapping places carry no wording, no
 * decision and no shard, and the loop's first delivered night was reported as
 * backed by exactly that.
 */
function canonical(value) {
  if (Array.isArray(value)) {
    return value.map(canonical).sort((a, b) => (JSON.stringify(a) < JSON.stringify(b) ? -1 : 1));
  }
  if (value !== null && typeof value === "object") {
    const out = {};
    for (const key of Object.keys(value).sort()) out[key] = canonical(value[key]);
    return out;
  }
  return value;
}

function gitShow(rev, path) {
  try {
    return execFileSync("git", ["show", `${rev}:${path}`], {
      encoding: "utf8",
      maxBuffer: 64 * 1024 * 1024,
      stdio: ["ignore", "pipe", "ignore"],
    });
  } catch {
    return null;
  }
}

/**
 * Classify one changed path under `.kapi/` as a decision or a normalization.
 * Anything that is not JSON, and anything added or removed outright, is a
 * decision: only a re-serialization of the same content is not.
 */
function contextChangeKind(path) {
  const before = gitShow("HEAD", path);
  if (before === null) return "decision";
  if (!existsSync(path)) return "decision";
  const after = readFileSync(path, "utf8");
  if (before === after) return "normalization";
  if (!path.endsWith(".json")) return "decision";
  try {
    const a = JSON.stringify(canonical(JSON.parse(before)));
    const b = JSON.stringify(canonical(JSON.parse(after)));
    return a === b ? "normalization" : "decision";
  } catch {
    return "decision";
  }
}

// ── the walk ─────────────────────────────────────────────────────────────────

function sidecarPairs(lang) {
  const pairs = [];
  if (!existsSync(DEMO_DIR)) return pairs;
  for (const entry of readdirSync(DEMO_DIR, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const target = join(DEMO_DIR, entry.name, `demo.${lang}.yaml`);
    if (existsSync(target)) pairs.push([target, join(DEMO_DIR, entry.name, "demo.yaml")]);
  }
  return pairs;
}

function validate(lang, pairs, only, rules) {
  const defects = [];
  const checked = [];
  const all = [...pairs, ...sidecarPairs(lang)];
  for (const [target, reference] of all) {
    if (only !== null && !only.has(target)) continue;
    if (!existsSync(target)) continue;
    if (!existsSync(reference)) {
      defects.push({ target, key: "", kind: "orphan", detail: `no reference (${reference})` });
      continue;
    }
    checked.push(target);
    defects.push(...(target.endsWith(".yaml") ? checkSidecar : checkJSON)(target, reference, rules));
  }
  return { defects, checked };
}

const KIND_EXPLANATION = {
  unparseable: "the artifact does not parse — the runtime that loads it reads nothing",
  "reference-unreadable": "the document it derives from does not parse",
  orphan: "nothing to measure the artifact against",
  invented: "a leaf the source document does not have",
  identifier:
    "a machine identifier the recipe does not declare translatable was translated — " +
    "what matches an overlay to its master, and what a group-by splits on",
  placeholder:
    "the translation does not carry its source's placeholders — a reader gets the " +
    "sentence with the count, the name or the link missing from it",
  structure: "the artifact does not overlay its master scene for scene",
};

function report(defects, checked, { max = 25 } = {}) {
  if (defects.length === 0) {
    console.log(`✓ ${checked.length} derived artifact(s) carry sound content.`);
    return 0;
  }
  console.error(`Derived content that must not be committed — ${defects.length} defect(s):\n`);
  const byKind = new Map();
  for (const d of defects) byKind.set(d.kind, [...(byKind.get(d.kind) ?? []), d]);
  for (const [kind, list] of byKind) {
    console.error(`${kind} (${list.length}) — ${KIND_EXPLANATION[kind] ?? ""}`);
    for (const d of list.slice(0, max)) {
      console.error(`  ${d.target}${d.key ? ` ${d.key}` : ""}: ${d.detail}`);
    }
    if (list.length > max) console.error(`  …and ${list.length - max} more`);
    console.error("");
  }
  console.error(
    "Coverage is not the question here and never fails this: a leaf the target does\n" +
      "not carry is pending work. These leaves are carried and wrong, which the next\n" +
      "convergence does not fix — it re-materializes them.\n",
  );
  return 1;
}

// ── self-test ────────────────────────────────────────────────────────────────
//
// The two historical failures are cases here, replayed from the shapes that
// reached main: a sidecar whose scene ids were translated, and a string whose
// placeholder was dropped. A legitimate artifact of each shape is a case too,
// because a gate that refuses everything is not a gate.

let selfTestStatus = 0;

function check(label, got, want) {
  const ok = JSON.stringify(got) === JSON.stringify(want);
  console.log(`${ok ? "✓" : "✖"} self-test: ${label}`);
  if (!ok) {
    console.log(`    expected ${JSON.stringify(want)}`);
    console.log(`    got      ${JSON.stringify(got)}`);
    selfTestStatus = 1;
  }
}

function selfTest() {
  const cases = [
    {
      label: "a translation carrying its source's placeholders is sound",
      source: "Added {value} to {name}",
      target: "La til {value} i {name}",
      want: null,
    },
    {
      label: "a placeholder-stripped translation renders a hole",
      source: "{row.done}/{row.units}",
      target: "/",
      want: { missing: ["{row.done}", "{row.units}"], extra: [] },
    },
    {
      label: "an element marker dropped from a translation is refused too",
      source: "Added to workspace {=m0}{slug}{/=m0}.",
      target: "Lagt til i arbeidsområdet .",
      want: { missing: ["{/=m0}", "{=m0}", "{slug}"], extra: [] },
    },
    {
      label: "a placeholder the translation invented is refused",
      source: "Delete this profile",
      target: "Slett {name}",
      want: { missing: [], extra: ["{name}"] },
    },
    {
      label: "an ICU plural translated with fewer categories is sound",
      source: "{n, plural, one {# issue} other {# issues}}",
      target: "{n, plural, other {# saker}}",
      want: null,
    },
    {
      label: "an ICU plural whose frame keeps its argument is sound",
      source: "{n, plural, one {1 file} other {{n} files}}",
      target: "{n, plural, other {{n} filer}}",
      want: null,
    },
    {
      label: "an ICU plural that lost its argument is refused",
      source: "{n, plural, one {# issue} other {# issues}}",
      target: "saker",
      want: { missing: ["{n, plural"], extra: [] },
    },
    {
      label: "an ICU select that renamed its argument is refused",
      source: "{gender, select, male {he} other {they}}",
      target: "{kjonn, select, other {de}}",
      want: { missing: ["{gender, select"], extra: ["{kjonn, select"] },
    },
    {
      label: "an element marker dropped from inside a plural is refused",
      source: "{n, plural, other {{=m0}# files{/=m0}}}",
      target: "{n, plural, other {# filer}}",
      want: { missing: ["{/=m0}", "{=m0}"], extra: [] },
    },
    {
      label: "a printf verb dropped from a translation is refused",
      source: "read %s: %v",
      target: "kunne ikke lese: %v",
      want: { missing: ["%s"], extra: [] },
    },
  ];
  for (const c of cases) check(c.label, placeholderDelta(c.source, c.target), c.want);

  const master = [
    "id: s0-northsea",
    "title: Govern the content you already have",
    "kind: use-case",
    "aspects:",
    "  - one language",
    "narration:",
    "  - id: discover",
    "    kind: title",
    "    text: A company repository.",
    "    caption: One language.",
    "  - id: correct",
    "    kind: terminal",
    "    text: Fix it where it is read.",
    "    caption: A decision.",
    "",
  ].join("\n");

  const sound = master
    .replace("Govern the content you already have", "Styr innholdet du allerede har")
    .replace("A company repository.", "Et bedriftsrepository.")
    .replace("Fix it where it is read.", "Rett det der det leses.")
    .replace("One language.", "Ett språk.")
    .replace("A decision.", "En beslutning.");

  const corrupt = sound
    .replace("id: discover", "id: oppdag")
    .replace("kind: use-case", "kind: brukstilfelle")
    .replace("kind: terminal", "kind: skrivebord");

  const dropped = sound.split("\n").slice(0, 10).join("\n") + "\n";

  const scan = (text) => valuesByKey(yamlScalars(text));
  const demoKeys = new Set(["title", "text", "caption"]);
  const sidecarDefects = (text) => {
    const src = scan(master);
    const defects = [];
    for (const { key, value } of yamlScalars(text)) {
      if (demoKeys.has(key)) continue;
      const known = src.get(key);
      if (known === undefined || !known.has(value)) defects.push(`${key}=${value}`);
    }
    return defects;
  };

  check("the master's own scalars are all known to it", sidecarDefects(master), []);
  check("a sidecar that translated only narration is sound", sidecarDefects(sound), []);
  check("a sidecar with translated scene ids and kinds is refused", sidecarDefects(corrupt), [
    "kind=brukstilfelle",
    "id=oppdag",
    "kind=skrivebord",
  ]);

  const counts = (text) => {
    const got = scan(text);
    const src = scan(master);
    const out = [];
    for (const [key, values] of src) {
      const want = [...values.values()].reduce((a, b) => a + b, 0);
      const have = [...(got.get(key) ?? new Map()).values()].reduce((a, b) => a + b, 0);
      if (have !== want) out.push(`${key}:${have}/${want}`);
    }
    return out;
  };
  check("a sidecar that overlays every scene passes the structure check", counts(sound), []);
  check("a sidecar missing a scene is refused", counts(dropped), [
    "id:2/3",
    "kind:2/3",
    "text:1/2",
    "caption:1/2",
  ]);

  // The block-scalar reader: a translated command block is a value, not lines
  // the scan walks over.
  const block = ["setup:", "  - git init -q", "run: |", "  kapi up", "  kapi check", ""].join("\n");
  check("a block scalar is read whole", yamlScalars(block), [
    { key: "setup", value: "git init -q" },
    { key: "run", value: "  kapi up\n  kapi check" },
  ]);

  check("a reordered array is not a decision", canonical({ a: [2, 1] }), canonical({ a: [1, 2] }));
  check(
    "a changed value is a decision",
    JSON.stringify(canonical({ a: [1, 2] })) === JSON.stringify(canonical({ a: [1, 3] })),
    false,
  );

  // The recipe reader, over the shapes kapi.yaml is written in.
  const planted = [
    "version: v1  # a trailing comment",
    "defaults:",
    "  target_languages: [nb, sv]",
    "  formats:",
    "    yaml:",
    "      config:",
    "        keyPathPatterns:",
    "          - title",
    "          - outro.line",
    "          - narration.*.text",
    "collections:",
    "  - name: engine",
    "    base: core/i18n",
    "    content:",
    "      # The inventory's prose leaves.",
    "      - path: builtins/metadata.json",
    "        format:",
    "          name: json",
    "          config:",
    "            extractAllPairs: false",
    "            extractionRules: '(displayName|note)$|enumDescriptions\\.[^.]+$'",
    "        target: catalogs/{lang}.json",
    "  - name: demos",
    "    base: harness/demos",
    "    content:",
    '      - path: "*/demo.yaml"',
    "        format:",
    "          name: yaml",
    '        target: "{dir}/demo.{lang}.yaml"',
    "",
  ].join("\n");

  const recipe = parseYAML(planted);
  check(
    "a flow sequence and a trailing comment parse",
    [recipe.version, recipe.defaults.target_languages],
    ["v1", ["nb", "sv"]],
  );
  check("a quoted path in a sequence item parses", recipe.collections[1].content[0].path, "*/demo.yaml");

  const rules = translatableRules(planted);
  check(
    "the engine rule is read from the recipe and tested against the dotted key path",
    [
      "tools.qa.displayName",
      "models.o3.note",
      "tools.qa.properties.mode.enumDescriptions.rules",
      "tools.qa.category",
    ].map(rules.engine),
    [true, true, true, false],
  );
  check("the demo keys come from the yaml format defaults", [...rules.demo].sort(), [
    "line",
    "text",
    "title",
  ]);

  const overridden = planted.replace(
    "          name: yaml\n",
    "          name: yaml\n          config:\n            keyPathPatterns: [subtitle]\n",
  );
  check("an item's own config overrides the format defaults", [...translatableRules(overridden).demo], [
    "subtitle",
  ]);

  let refused = "";
  try {
    translatableRules("collections: []\n");
  } catch (err) {
    refused = String(err.message ?? err);
  }
  check("a recipe with no engine item is refused, not read as permissive", refused.includes(ENGINE_SOURCE), true);

  let own = "";
  try {
    const repo = translatableRules(readFileSync(new URL("../kapi.yaml", import.meta.url), "utf8"));
    own = [typeof repo.engine, repo.demo instanceof Set].join(" ");
  } catch (err) {
    own = String(err.message ?? err);
  }
  check("the repository's kapi.yaml resolves both rules", own, "function true");

  return selfTestStatus;
}

// ── entry point ──────────────────────────────────────────────────────────────

function main(argv) {
  if (argv.includes("--self-test")) return selfTest();

  const backingAt = argv.indexOf("--backing");
  if (backingAt !== -1) {
    const paths = argv.slice(backingAt + 1);
    if (paths.length === 0) {
      console.error("usage: check-derived-content.mjs --backing <path>...");
      return 2;
    }
    for (const path of paths) console.log(`${contextChangeKind(path)}\t${path}`);
    return 0;
  }

  const only = new Set();
  const rest = [];
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--only") {
      only.add(argv[++i]);
      continue;
    }
    rest.push(argv[i]);
  }

  const [lang, ...pairs] = rest;
  if (!lang) {
    console.error("usage: check-derived-content.mjs <lang> <target>:<reference>...");
    return 2;
  }

  const parsed = pairs.map((pair) => {
    const sep = pair.lastIndexOf(":");
    return [pair.slice(0, sep), pair.slice(sep + 1)];
  });

  let rules;
  try {
    rules = translatableRules(readFileSync(RECIPE, "utf8"));
  } catch (err) {
    console.error(`check-derived-content: cannot read what ${RECIPE} declares translatable: ${err.message ?? err}`);
    return 2;
  }

  const { defects, checked } = validate(lang, parsed, only.size > 0 ? only : null, rules);
  return report(defects, checked);
}

process.exit(main(process.argv.slice(2)));
