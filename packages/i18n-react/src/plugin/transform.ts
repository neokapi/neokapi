/**
 * SWC-based transform for @neokapi/i18n-react.
 *
 * Two output modes:
 *   - inline: resolves translations at build time → translated JSX (zero runtime)
 *   - runtime: emits t() calls → resolved at runtime via OTA dictionary
 *
 * Architecture: parse with SWC, walk AST, apply string-level operations.
 * SWC's printSync compiles JSX to createElement, so we use string splicing
 * to keep the output as JSX for the downstream React plugin.
 */

import { readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { parseSync, type JSXElement, type JSXFragment, type Module } from "@swc/core";

import {
  ancestorTranslate,
  getTagName,
  lineFromOffset,
  nearestTranslate,
  resolveHTMLElement,
} from "../extract/ast.ts";
import { buildJSXPath, FRAGMENT_DESCRIPTOR } from "../extract/jsx-path.ts";
import { buildRuns, type Occurrence } from "../extract/runs.ts";
import { hasTranslatableText, isAllInlineContent, resolvePolicy } from "../extract/translatable.ts";
import { collectTIdentifiers, walkTCalls } from "../extract/messages.ts";
import { parseSyntaxFor } from "../parse-syntax.ts";
import { resolveLibraryComponentMap } from "./manifests.ts";
import {
  createWarningCollector,
  formatWarning,
  type WarningCollector,
} from "../extract/warnings.ts";
import { isTranslatableAttribute } from "./defaults.ts";
import { hashKey } from "./hash.ts";
import { CONTEXT_SEPARATOR, type PluginOptions } from "../types.ts";
import type { ReviewManifest } from "../review/manifest.ts";

type TransformOp = {
  offset: number;
  deleteCount: number;
  insert: string;
};

type ProcessResult = {
  /** Runtime helper used by this element, if any (used to decide which imports to add). */
  runtime: "runtime-t" | "runtime-tx" | null;
  /**
   * True when the element's content range was transformed as a translation
   * unit (inline or tx/t) and its children were captured verbatim into
   * the emitted op. In that case the walker MUST NOT descend into the
   * children, or their independently-emitted ops will overlap with the
   * parent op and produce malformed output (see #3).
   */
  consumed: boolean;
};

/**
 * Decode a UTF-8 byte range from a Buffer. SWC span offsets are byte
 * offsets into the UTF-8 source, so any `code.slice(...)` with span
 * offsets corrupts non-ASCII input. Use this instead.
 */
function bslice(buf: Buffer, start: number, end: number): string {
  return buf.toString("utf8", start, end);
}

/**
 * Line-based snippet: returns the source line containing `line`,
 * trimmed to 80 chars. Line lookup avoids SWC byte-span off-by-N
 * quirks across parse bases.
 */
function snippetOf(code: string, line: number): string {
  const lines = code.split("\n");
  const raw = (lines[line - 1] ?? "").trim();
  return raw.length > 80 ? `${raw.slice(0, 80)}…` : raw;
}

/**
 * Find the byte offset of the first significant token in a source —
 * i.e. the first byte that isn't whitespace, a line comment, a block
 * comment, a shebang, or a BOM. SWC's `ast.span.start` points there
 * (in global source-map space), so we subtract this from the span to
 * derive the per-parse base.
 */
function findFirstTokenByteOffset(source: string): number {
  const buf = Buffer.from(source, "utf8");
  const len = buf.length;
  let i = 0;

  if (len >= 3 && buf[0] === 0xef && buf[1] === 0xbb && buf[2] === 0xbf) i = 3;

  if (buf[i] === 0x23 && buf[i + 1] === 0x21) {
    while (i < len && buf[i] !== 0x0a) i++;
  }

  while (i < len) {
    const c = buf[i];
    if (c === 0x20 || c === 0x09 || c === 0x0a || c === 0x0d) {
      i++;
      continue;
    }
    if (c === 0x2f && buf[i + 1] === 0x2f) {
      while (i < len && buf[i] !== 0x0a) i++;
      continue;
    }
    if (c === 0x2f && buf[i + 1] === 0x2a) {
      i += 2;
      while (i < len - 1 && !(buf[i] === 0x2a && buf[i + 1] === 0x2f)) i++;
      i += 2;
      continue;
    }
    break;
  }
  return i;
}

/**
 * Create an offset converter for a parsed SWC AST.
 * SWC spans are byte offsets into a global source-map space that is
 * shared and monotonically growing across all `parseSync` calls in
 * the process. `ast.span.start` is the global offset of the first
 * significant token, NOT byte 0 of the current source. Subtracting
 * its in-source byte offset yields the base for this parse.
 */
function makeOffsetConverter(ast: Module, code: string): (offset: number) => number {
  const base = ast.span.start - findFirstTokenByteOffset(code);
  return (offset: number) => offset - base;
}

// ─── Translation loading ─────────────────────────────────────

/**
 * Per-file dict cache keyed by path and invalidated by mtime, so a
 * long-lived dev server picks up edits to `translations/*.json`
 * without a restart. The stat costs microseconds per transform call —
 * cheap enough to pay on every call rather than cache for the process
 * lifetime, which would serve stale translations.
 */
const dictFileCache = new Map<string, { mtimeMs: number; dict: Record<string, string> }>();

/**
 * Load a single translation JSON file. Returns flat {hash: text} dict.
 */
function loadSingleDict(dir: string, locale: string): Record<string, string> | null {
  const filePath = join(dir, `${locale}.json`);
  let mtimeMs: number;
  try {
    mtimeMs = statSync(filePath).mtimeMs;
  } catch {
    return null;
  }
  const cached = dictFileCache.get(filePath);
  if (cached && cached.mtimeMs === mtimeMs) return cached.dict;
  try {
    const raw = readFileSync(filePath, "utf-8");
    const data = JSON.parse(raw);
    const dict = data[locale] || data;
    dictFileCache.set(filePath, { mtimeMs, dict });
    return dict;
  } catch {
    return null;
  }
}

/**
 * Load translations with fallback locale chain.
 * Merges: fallback[n] < ... < fallback[0] < primary locale
 * (primary wins over fallbacks)
 */
function loadTranslationDict(options: PluginOptions): Record<string, string> | null {
  if (!options.locale) return null;

  const dir = options.translationsDir || "./translations";

  // Load fallback locales first (lower priority)
  let merged: Record<string, string> = {};
  if (options.fallbackLocales) {
    for (const fallback of [...options.fallbackLocales].reverse()) {
      const fallbackDict = loadSingleDict(dir, fallback);
      if (fallbackDict) {
        merged = { ...merged, ...fallbackDict };
      }
    }
  }

  // Load primary locale (highest priority)
  const primary = loadSingleDict(dir, options.locale);
  if (primary) {
    merged = { ...merged, ...primary };
  }

  if (Object.keys(merged).length === 0) return null;

  return merged;
}

// ─── Main transform ──────────────────────────────────────────

/**
 * Result of a single-file transform.
 *
 *   code    — the rewritten source.
 *   hashes  — every hash this file emitted into a `__t` / `__tx` call.
 *             Populated in `mode === "runtime"` only; inline builds
 *             bake translations in and don't need a runtime manifest.
 *             The bundler-level `generateBundle` hook unions these
 *             across each output chunk to produce
 *             `translations-manifest.json` (issue #406).
 */
export function transform(
  code: string,
  filename: string,
  options: PluginOptions,
): { code: string; hashes: string[]; review: ReviewManifest } | null {
  const rules = options.rules || [];
  // Review runs even without a locale: a source-language build still
  // stamps `data-kapi-id` and contributes review-manifest entries, so
  // the read-only hosted overlay works on any statically-rendered page
  // (the IDs are content hashes — render-mode-independent, AD-035).
  // Such a build defaults to inline (bakes source, adds stamps); an
  // explicit `mode` always wins.
  const mode = options.mode || (options.locale || options.review ? "inline" : undefined);
  if (!mode) return null;

  const dict = mode === "inline" ? loadTranslationDict(options) : null;

  let ast: Module;
  try {
    ast = parseSync(code, parseSyntaxFor(filename));
  } catch (err) {
    // A file that cannot be parsed is not a file with nothing to translate.
    // Returning null in silence is how the labels reached the dictionary and
    // then rendered in English anyway: extraction read the file, the transform
    // could not, and nothing said so.
    console.warn(
      `[neokapi] ${filename}: could not be parsed, so its strings were left ` +
        `untranslated: ${err instanceof Error ? err.message.split("\n")[0] : String(err)}`,
    );
    return null;
  }

  // Mirror walker.ts: auto-resolve library manifests (+ .d.ts
  // fallback) for every non-relative import, then layer the user's
  // componentMap on top. Ensures hashes match across extract /
  // transform without requiring manual componentMap entries for
  // shadcn / radix / MUI components that ship proper types.
  const libraryMap = resolveLibraryComponentMap(
    ast,
    options.projectRoot ?? process.cwd(),
    options.communityManifestDir,
    filename,
  );
  const componentMap: Record<string, string> = {
    ...libraryMap,
    ...options.componentMap,
  };

  const s = makeOffsetConverter(ast, code);
  const buf = Buffer.from(code, "utf8");
  const ops: TransformOp[] = [];
  const warnings = createWarningCollector();
  // Collects every hash written into a `__t(...)` / `__tx(...)` call
  // so the bundler-level `generateBundle` hook can emit a per-chunk
  // manifest (issue #406). Inline builds stay at zero — baked strings
  // don't hit the runtime dict.
  const hashes = new Set<string>();
  // Review-manifest entries this file contributes (keyed by block
  // hash): source text, the build locale's baked target, and
  // translator-facing properties. Populated only when
  // `options.review`; the plugin unions these across files and emits
  // `translations/review.json` — the read-only hosted overlay's data
  // source. Render-mode-independent: inline and runtime builds record
  // the same hashes (AD-035).
  const reviewEntries: ReviewManifest = {};
  let needsT = false;
  let needsTx = false;

  walkModule(
    ast,
    (el, ancestors) => {
      const r = processElement(
        el,
        ancestors,
        buf,
        filename,
        componentMap,
        rules,
        mode,
        dict,
        options,
        s,
        ops,
        warnings,
        code,
        hashes,
        reviewEntries,
      );
      if (r.runtime === "runtime-t") needsT = true;
      if (r.runtime === "runtime-tx") {
        needsT = true;
        needsTx = true;
      }
      return { skipChildren: r.consumed };
    },
    (frag, ancestors) => {
      const r = processFragment(
        frag,
        ancestors,
        buf,
        componentMap,
        mode,
        dict,
        options,
        s,
        ops,
        hashes,
      );
      if (r.runtime === "runtime-t") needsT = true;
      if (r.runtime === "runtime-tx") {
        needsT = true;
        needsTx = true;
      }
      return { skipChildren: r.consumed };
    },
  );

  // User-facing `t("text", params?)` calls. Same matching rule as
  // JSX extraction — only calls bound to the runtime import are
  // touched, not a random local `t()`.
  //
  // Runtime mode: rewrite to `__t("hash", "text", params)` so the
  // OTA dict lookup applies.
  //
  // Inline mode: resolve against the dict at build time. A literal
  // call with no params and no ICU collapses to a plain string
  // literal (zero runtime); params or translator-driven ICU keep a
  // `__t` call with the *translated* text baked in as the fallback —
  // still no dict fetch, the runtime only does substitution/ICU.
  const tNames = collectTIdentifiers(ast);
  // Slice via the UTF-8 buffer, not code.slice — SWC spans are byte
  // offsets; code.slice is UTF-16 code-unit indexed. Any non-ASCII
  // char (e.g. em-dash in a comment) above the t() call shifts the
  // real offset and produces corrupted paramsSrc (see #382).
  const sourceSlice = (start: number, end: number): string => bslice(buf, s(start), s(end));
  // Any element-extraction op already queued covers the bytes of
  // every `t()` call embedded inside it (the whole element body
  // gets replaced with a single `__tx(…)`). Record those ranges so
  // we skip the inner-t rewrite — otherwise the two ops overlap
  // and the final op-disjoint check throws. The element's `__tx`
  // renders the t()-call result via its param list, so no
  // translation is lost.
  const elementOpRanges = ops.map((op) => [op.offset, op.offset + op.deleteCount] as const);
  const coveredByElementOp = (start: number, end: number): boolean => {
    for (const [a, b] of elementOpRanges) if (start >= a && end <= b) return true;
    return false;
  };
  for (const call of walkTCalls(ast, tNames, sourceSlice)) {
    const callStart = s(call.node.span.start);
    const callEnd = s(call.node.span.end);
    if (coveredByElementOp(callStart, callEnd)) continue;

    const desc = `t${CONTEXT_SEPARATOR}${call.context ?? ""}`;
    const hash = hashKey(call.text, desc);

    if (mode === "inline") {
      const translated = dict?.[hash];
      if (translated === undefined && options.strict !== false) {
        const msg = `[neokapi] Missing translation for "${call.text}" (hash: ${hash}, locale: ${options.locale})`;
        if (options.strict === "error") throw new Error(msg);
        console.warn(msg);
      }
      const resolved = translated ?? call.text;
      const hasICU = /\{[^{}]+,\s*(plural|select|selectordinal)\s*,/.test(resolved);
      if (!call.paramsSrc && !hasICU) {
        // Fully static: collapse to a plain string literal.
        ops.push({
          offset: callStart,
          deleteCount: callEnd - callStart,
          insert: JSON.stringify(resolved),
        });
        continue;
      }
      // Params and/or ICU: keep a runtime call with the translated
      // text baked in — substitution/plural rules run at render time,
      // no dict involved.
      const args = call.paramsSrc
        ? `"${hash}", ${JSON.stringify(resolved)}, ${call.paramsSrc}`
        : `"${hash}", ${JSON.stringify(resolved)}`;
      ops.push({
        offset: callStart,
        deleteCount: callEnd - callStart,
        insert: `__t(${args})`,
      });
      needsT = true;
      continue;
    }

    const fallbackLiteral = JSON.stringify(call.text);
    const args = call.paramsSrc
      ? `"${hash}", ${fallbackLiteral}, ${call.paramsSrc}`
      : `"${hash}", ${fallbackLiteral}`;
    ops.push({
      offset: callStart,
      deleteCount: callEnd - callStart,
      insert: `__t(${args})`,
    });
    hashes.add(hash);
    needsT = true;
  }

  // Flush warnings. console.warn by default so the dev-server
  // pipeline surfaces them; consumers can opt out of the stderr
  // noise by providing their own `onWarning` hook. When
  // `warningsAsErrors` is on, the first warning becomes a thrown
  // build error — CI-friendly failure mode.
  const list = warnings.list();
  if (list.length > 0 && options.warningsAsErrors) {
    throw new Error(formatWarning(list[0]));
  }
  const flush = options.onWarning ?? ((msg: string) => console.warn(msg));
  for (const w of list) flush(formatWarning(w));

  if (ops.length === 0) return null;

  // Apply ops in byte space: SWC offsets are UTF-8 byte offsets, so
  // splicing must operate on a Buffer, not on a JS string (which is
  // UTF-16 code unit indexed).
  ops.sort((a, b) => a.offset - b.offset);

  // Defensive: ops must be pairwise disjoint. Nested translatable
  // elements used to emit overlapping ranges (the outer tx() captured
  // the inner element verbatim while the inner element produced its
  // own t() op); the walker now skips descendants of consumed blocks,
  // but this check fails loudly if anything else regresses. See #3.
  for (let i = 1; i < ops.length; i++) {
    const prev = ops[i - 1];
    const curr = ops[i];
    if (prev.offset + prev.deleteCount > curr.offset) {
      throw new Error(
        `[neokapi] overlapping transform ops in ${filename}: ` +
          `[${prev.offset}, ${prev.offset + prev.deleteCount}) and ` +
          `[${curr.offset}, ${curr.offset + curr.deleteCount})`,
      );
    }
  }

  const parts: Buffer[] = [];
  let pos = 0;
  for (const op of ops) {
    parts.push(buf.subarray(pos, op.offset));
    parts.push(Buffer.from(op.insert, "utf8"));
    pos = op.offset + op.deleteCount;
  }
  parts.push(buf.subarray(pos));
  let result = Buffer.concat(parts).toString("utf8");

  if (needsT || needsTx) {
    const imports = [needsT ? "__t" : "", needsTx ? "__tx" : ""].filter(Boolean).join(", ");
    const importLine = `import { ${imports} } from '@neokapi/i18n-react/runtime';`;
    const directiveMatch = result.match(/^(["']use (?:client|server)["']\s*;?\s*\n)/);
    if (directiveMatch) {
      result = directiveMatch[1] + importLine + "\n" + result.slice(directiveMatch[1].length);
    } else {
      result = importLine + "\n" + result;
    }
  }

  return { code: result, hashes: Array.from(hashes), review: reviewEntries };
}

// ─── AST Walking ─────────────────────────────────────────────

function walkModule(
  module: Module,
  visitor: (el: JSXElement, ancestors: JSXElement[]) => { skipChildren: boolean },
  fragmentVisitor?: (frag: JSXFragment, ancestors: JSXElement[]) => { skipChildren: boolean },
) {
  function walk(node: any, jsxAncestors: JSXElement[]) {
    if (!node || typeof node !== "object") return;
    if (node.type === "JSXFragment" && fragmentVisitor) {
      const frag = node as JSXFragment;
      const { skipChildren } = fragmentVisitor(frag, jsxAncestors);
      // A consumed fragment's children were captured verbatim into
      // its op — descending would emit ops nested inside that range
      // (the op-disjointness check throws). Same rule as consumed
      // elements. (The extract walker DOES revisit expression
      // containers — it only emits blocks, never ops.)
      if (skipChildren) return;
      for (const child of frag.children || []) walk(child, jsxAncestors);
      return;
    }
    if (node.type === "JSXElement") {
      const el = node as JSXElement;
      const { skipChildren } = visitor(el, jsxAncestors);
      if (skipChildren) return;
      const newAncestors = [...jsxAncestors, el];
      // Descend into the opening tag too so JSX nested inside an
      // attribute value (e.g. `actions={<div><Button>…</Button></div>}`)
      // gets visited. Without this, blocks inside prop JSX extract fine
      // but never receive their runtime `__t` / `__tx` call, so the
      // rendered UI stays in the source language. Mirrors the extract
      // walker (walker.ts) which already descends into `el.opening`.
      if (el.opening) walk(el.opening, newAncestors);
      for (const child of el.children || []) walk(child, newAncestors);
      return;
    }
    for (const key of Object.keys(node)) {
      if (key === "span" || key === "type") continue;
      const val = node[key];
      if (Array.isArray(val)) {
        for (const item of val) walk(item, jsxAncestors);
      } else if (val && typeof val === "object" && val.type) {
        walk(val, jsxAncestors);
      }
    }
  }
  walk(module, []);
}

// ─── Element Processing ──────────────────────────────────────

function processElement(
  el: JSXElement,
  ancestors: JSXElement[],
  buf: Buffer,
  filename: string,
  componentMap: Record<string, string>,
  rules: NonNullable<PluginOptions["rules"]>,
  mode: "inline" | "runtime",
  dict: Record<string, string> | null,
  options: PluginOptions,
  s: (offset: number) => number,
  ops: TransformOp[],
  warnings: WarningCollector,
  code: string,
  hashes: Set<string>,
  reviewEntries: ReviewManifest,
): ProcessResult {
  const tagName = getTagName(el);
  if (!tagName) return { runtime: null, consumed: false };
  // W3C translate inheritance: nearest explicit setting on self or
  // an ancestor wins. `translate="yes"` on a child re-enables
  // translation inside a `translate="no"` subtree. Mirrored in
  // extract/walker.ts.
  if (nearestTranslate(el, ancestors) === "no") return { runtime: null, consumed: false };

  // Mirror walker.ts: fall back to the raw tag for unmapped
  // React components so resolvePolicy's container-promotion
  // rule can kick in when they have direct translatable text.
  const mapped = resolveHTMLElement(tagName, componentMap);
  const htmlElement = mapped ?? tagName;
  const unmappedComponent = mapped === null;

  const policy = resolvePolicy(htmlElement, el, rules, componentMap);

  // Extract translatable attributes from every element (mapped or
  // not) — `translatableAttributes` is keyed on prop name, not host
  // element, so `<PageHeader title="Termbases" />` just works.
  const attrResult = processAttributes(
    el,
    ancestors,
    componentMap,
    mode,
    dict,
    policy.locNote,
    s,
    ops,
    hashes,
  );
  let usedRuntime: "runtime-t" | "runtime-tx" | null = attrResult.usedRuntime ? "runtime-t" : null;

  // Review: stamp the element's opening tag with `data-kapi-*` and
  // record its block(s) into the review manifest. `blockHash` is the
  // element's content-block hash (null for attribute-only elements —
  // their placeholder/aria strings still get a `data-kapi-attr` stamp
  // and manifest entries). This runs identically in inline and
  // runtime mode; only the baked target differs (inline has one).
  const doReview = (
    blockHash: string | null,
    blockSource: string | null,
    blockTarget: string | undefined,
  ) => {
    if (!options.review) return;
    stampReviewAttributes(el, buf, s, ops, filename, code, blockHash, attrResult.pairs);
    const line = lineFromOffset(code, s(el.span.start));
    if (blockHash && blockSource) {
      recordReviewEntry(reviewEntries, {
        hash: blockHash,
        source: blockSource,
        element: tagName,
        filename,
        line,
        locNote: policy.locNote,
        target: blockTarget,
        locale: options.locale,
      });
    }
    for (const p of attrResult.pairs) {
      recordReviewEntry(reviewEntries, {
        hash: p.hash,
        source: p.source,
        element: tagName,
        filename,
        line,
        locNote: undefined,
        target: p.target,
        locale: options.locale,
      });
    }
  };

  if (!policy.translate) {
    doReview(null, null, undefined);
    return { runtime: usedRuntime, consumed: false };
  }
  if (!hasTranslatableText(el)) {
    doReview(null, null, undefined);
    return { runtime: usedRuntime, consumed: false };
  }
  if (!isAllInlineContent(el, componentMap)) {
    doReview(null, null, undefined);
    return { runtime: usedRuntime, consumed: false };
  }

  // Record warnings for elements whose translatability had to be
  // inferred. Must happen after all gating checks so we don't
  // warn about elements we end up skipping. Unmapped components
  // always trigger the promotion path, so prefer the more specific
  // unknown-component warning over the generic container one.
  const warnLine = lineFromOffset(code, s(el.span.start));
  // See walker.ts: container-element promotion (<div> with direct
  // text) is the expected default and no longer warns. Only
  // unmapped components emit a warning — those are actionable
  // (add a componentMap entry for hash stability).
  if (unmappedComponent) {
    warnings.add({
      kind: "unknown-component",
      filename,
      line: warnLine,
      tag: tagName,
      snippet: snippetOf(code, warnLine),
    });
  }

  const jsxPath = buildJSXPath(ancestors, el, componentMap);
  const locNote = policy.locNote;
  const desc = locNote ? `${jsxPath}${CONTEXT_SEPARATOR}${locNote}` : jsxPath;

  const contentStart = getOpeningTagEnd(el, s);
  const contentEnd = getClosingTagStart(el, s);
  if (contentStart === null || contentEnd === null)
    return { runtime: usedRuntime, consumed: false };

  // Single source of truth for the flat template + token spans: the
  // same builder the extractor uses (extract/runs.ts). Hash parity
  // between extract and transform holds by construction.
  const { flatText: text, occurrences } = buildRuns(el, {
    componentMap,
    sourceSlice: (start, end) => bslice(buf, s(start), s(end)),
  });
  if (text === "") return { runtime: usedRuntime, consumed: false };
  const paramList: ParamInfo[] = occurrences.map((o) => convertOccurrence(o, s));
  const hk = hashKey(text, desc);

  const blockRuntime = emitBlockContent({
    hk,
    text,
    paramList,
    mode,
    dict,
    options,
    buf,
    contentStart,
    contentEnd,
    ops,
    hashes,
  });
  if (blockRuntime) usedRuntime = blockRuntime;

  doReview(hk, text, mode === "inline" ? dict?.[hk] : undefined);
  removeDataI18nAttrs(el, buf, s, ops);
  return { runtime: usedRuntime, consumed: true };
}

/**
 * Fragment-rooted blocks (`<>text {name}</>`): no attributes, no
 * rules — ancestor `translate` state plus the promotion rule (direct
 * text, all-inline children) decide. Content splices between the
 * `<>` and `</>` markers; descriptor is the fixed `fragment`.
 */
function processFragment(
  frag: JSXFragment,
  ancestors: JSXElement[],
  buf: Buffer,
  componentMap: Record<string, string>,
  mode: "inline" | "runtime",
  dict: Record<string, string> | null,
  options: PluginOptions,
  s: (offset: number) => number,
  ops: TransformOp[],
  hashes: Set<string>,
): ProcessResult {
  if (ancestorTranslate(ancestors) === "no") return { runtime: null, consumed: false };
  if (!hasTranslatableText(frag) || !isAllInlineContent(frag, componentMap)) {
    return { runtime: null, consumed: false };
  }

  const contentStart = s(frag.opening.span.end);
  const contentEnd = s(frag.closing.span.start);

  const { flatText: text, occurrences } = buildRuns(frag, {
    componentMap,
    sourceSlice: (start, end) => bslice(buf, s(start), s(end)),
  });
  if (text === "") return { runtime: null, consumed: false };
  const paramList: ParamInfo[] = occurrences.map((o) => convertOccurrence(o, s));
  const hk = hashKey(text, FRAGMENT_DESCRIPTOR);

  const runtime = emitBlockContent({
    hk,
    text,
    paramList,
    mode,
    dict,
    options,
    buf,
    contentStart,
    contentEnd,
    ops,
    hashes,
  });
  return { runtime, consumed: true };
}

/**
 * Shared block-content emission for elements and fragments: inline
 * splicing (with ICU routed through the runtime) or the
 * `{__t(...)}` / `{__tx(...)}` runtime call.
 */
function emitBlockContent(args: {
  hk: string;
  text: string;
  paramList: ParamInfo[];
  mode: "inline" | "runtime";
  dict: Record<string, string> | null;
  options: PluginOptions;
  buf: Buffer;
  contentStart: number;
  contentEnd: number;
  ops: TransformOp[];
  hashes: Set<string>;
}): "runtime-t" | "runtime-tx" | null {
  const { hk, text, paramList, mode, dict, options, buf, contentStart, contentEnd, ops, hashes } =
    args;

  // ICU (plural/select) pivots are runtime values — the chosen form
  // can't be known at build time, so ICU-bearing blocks always route
  // through the runtime resolver, with the translated template baked
  // in as the fallback in inline mode (no dict fetch needed).
  const hasICU = paramList.some((p) => p.kind === "pivot");

  if (mode === "inline") {
    const translated = dict?.[hk];

    // Missing translation detection
    if (!translated && options.strict !== false) {
      const msg = `[neokapi] Missing translation for "${text}" (hash: ${hk}, locale: ${options.locale})`;
      if (options.strict === "error") {
        throw new Error(msg);
      } else {
        console.warn(msg);
      }
    }

    // Translator-driven ICU: the source had no <Plural>/<Select>,
    // but the translation introduces `{x, plural, …}` — also a
    // runtime decision, so route it the same way.
    const translatorICU =
      translated !== undefined && /\{[^{}]+,\s*(plural|select|selectordinal)\s*,/.test(translated);

    if (hasICU || translatorICU) {
      // Bake the translated ICU template into a runtime call. The
      // dict lookup misses (inline builds load no dict) and the
      // baked fallback carries the translation.
      const insert = buildRuntimeCall(hk, translated ?? text, paramList, buf, {
        fallbackOverride: translated ?? text,
      });
      ops.push({
        offset: contentStart,
        deleteCount: contentEnd - contentStart,
        insert: insert.code,
      });
      return insert.usedTx ? "runtime-tx" : "runtime-t";
    }
    const inlined = inlineTranslation(translated ?? text, paramList, buf);
    ops.push({
      offset: contentStart,
      deleteCount: contentEnd - contentStart,
      insert: inlined,
    });
    return null;
  }

  const insert = buildRuntimeCall(hk, text, paramList, buf, {});
  ops.push({ offset: contentStart, deleteCount: contentEnd - contentStart, insert: insert.code });
  hashes.add(hk);
  return insert.usedTx ? "runtime-tx" : "runtime-t";
}

/**
 * Emit the `{__tx(...)}` / `{__t(...)}` runtime call for a block.
 * Used by runtime mode always, and by inline mode for ICU-bearing
 * blocks (where `fallbackOverride` carries the translated template).
 */
function buildRuntimeCall(
  hk: string,
  text: string,
  paramList: ParamInfo[],
  buf: Buffer,
  opts: { fallbackOverride?: string },
): { code: string; usedTx: boolean } {
  const hasInlineElements = paramList.some((p) => p.name.startsWith("="));
  if (hasInlineElements) {
    const regularParams = paramList.filter((p) => !p.name.startsWith("="));
    const elementParams = paramList.filter((p) => p.name.startsWith("="));
    const elementsObj = `{ ${elementParams.map((p) => `${JSON.stringify(p.name)}: ${bslice(buf, p.fullStart, p.fullEnd)}`).join(", ")} }`;
    const paramsObj =
      regularParams.length > 0
        ? `, { ${regularParams.map((p) => `${JSON.stringify(p.name)}: ${bslice(buf, p.exprStart, p.exprEnd)}`).join(", ")} }`
        : "";
    const fallbackText = JSON.stringify(opts.fallbackOverride ?? text);
    return {
      code: `{__tx("${hk}", ${fallbackText}, ${elementsObj}${paramsObj})}`,
      usedTx: true,
    };
  }
  const paramsObj =
    paramList.length > 0
      ? `, { ${paramList.map((p) => `${JSON.stringify(p.name)}: ${bslice(buf, p.exprStart, p.exprEnd)}`).join(", ")} }`
      : "";
  const fallbackExpr =
    opts.fallbackOverride !== undefined
      ? JSON.stringify(opts.fallbackOverride)
      : buildFallbackExpr(text, paramList, buf);
  return { code: `{__t("${hk}", ${fallbackExpr}${paramsObj})}`, usedTx: false };
}

/** Map a raw-span Occurrence into converted byte offsets. */
function convertOccurrence(o: Occurrence, s: (n: number) => number): ParamInfo {
  return {
    name: o.name,
    kind: o.kind,
    exprStart: s(o.exprStart),
    exprEnd: s(o.exprEnd),
    fullStart: s(o.fullStart),
    fullEnd: s(o.fullEnd),
    openStart: o.openStart !== undefined ? s(o.openStart) : undefined,
    openEnd: o.openEnd !== undefined ? s(o.openEnd) : undefined,
    closeStart: o.closeStart !== undefined ? s(o.closeStart) : undefined,
    closeEnd: o.closeEnd !== undefined ? s(o.closeEnd) : undefined,
  };
}

// ─── Text Extraction ─────────────────────────────────────────

type ParamInfo = {
  name: string;
  kind: Occurrence["kind"];
  exprStart: number;
  exprEnd: number;
  fullStart: number;
  fullEnd: number;
  /** Paired elements only: opening / closing tag spans (converted). */
  openStart?: number;
  openEnd?: number;
  closeStart?: number;
  closeEnd?: number;
};

// ─── Inline Translation ──────────────────────────────────────

/**
 * Escape a literal text segment for insertion as JSX children.
 * `<`, `>`, `{`, `}` would otherwise change the parse; entities
 * render back to the original characters. `&` stays untouched so
 * entities a translator typed deliberately keep working.
 */
/**
 * Escape a translated string for a JSX attribute value literal.
 * JSX attribute strings have no backslash escapes — a `"` must be
 * the `&quot;` entity (decoded by React at parse time).
 */
function escapeJSXAttr(text: string): string {
  return text.replace(/"/g, "&quot;");
}

function escapeJSXText(text: string): string {
  return text
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\{/g, "&#123;")
    .replace(/\}/g, "&#125;");
}

type InlineTok = { start: number; end: number; key: string; kind: "open" | "close" };

/**
 * Rebuild translated JSX from a flat template at build time.
 * Mirrors the runtime `__tx` renderer: `{name}` tokens become
 * `{expr}` containers, standalone `{=mN}` tokens splice the whole
 * child element/expression source, and paired `{=mN}…{/=mN}` ranges
 * wrap the recursively-rendered inner content in the child's
 * opening/closing tags — so `<a>here</a>` keeps its (translated)
 * inner text inside the anchor, in whatever order the translator
 * put the tokens. Tokens the template carries but the call site
 * doesn't know (translator artifacts) render as escaped text so a
 * raw `{foo}` can never break the parse or reference a stray
 * variable.
 */
function inlineTranslation(translatedText: string, paramList: ParamInfo[], buf: Buffer): string {
  const byName = new Map<string, ParamInfo>();
  for (const param of paramList) {
    if (!byName.has(param.name)) byName.set(param.name, param);
  }

  const tokens: InlineTok[] = [];
  // Token names: `=mN` element markers or identifier-ish param names
  // (letters, digits, `_`, `$`, dots from member expressions). The
  // shape deliberately can't match an ICU header like
  // `{count, plural, …}` — ICU-bearing text never reaches this
  // function (it routes through the runtime call instead).
  const re = /\{(\/?)([=A-Za-z_$][\w.$]*)\}/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(translatedText)) !== null) {
    tokens.push({
      start: m.index,
      end: m.index + m[0].length,
      key: m[2],
      kind: m[1] === "/" ? "close" : "open",
    });
  }

  // LIFO open/close matching, same as the runtime renderer.
  const closeOf = new Map<number, number>();
  const openStack: number[] = [];
  for (let i = 0; i < tokens.length; i++) {
    const tok = tokens[i];
    if (tok.kind === "open") {
      openStack.push(i);
      continue;
    }
    for (let j = openStack.length - 1; j >= 0; j--) {
      if (tokens[openStack[j]].key === tok.key) {
        closeOf.set(openStack[j], i);
        openStack.splice(j, 1);
        break;
      }
    }
  }

  const render = (charStart: number, charEnd: number, tokFrom: number, tokTo: number): string => {
    let out = "";
    let cursor = charStart;
    let i = tokFrom;
    while (i <= tokTo && i < tokens.length) {
      const tok = tokens[i];
      if (tok.start >= charEnd) break;
      if (tok.start > cursor) out += escapeJSXText(translatedText.slice(cursor, tok.start));

      const param = byName.get(tok.key);
      if (tok.kind === "open") {
        const closeIdx = closeOf.get(i);
        if (closeIdx !== undefined && closeIdx <= tokTo) {
          const close = tokens[closeIdx];
          const inner = render(tok.end, close.start, i + 1, closeIdx - 1);
          if (param && param.openStart !== undefined && param.closeStart !== undefined) {
            out +=
              bslice(buf, param.openStart, param.openEnd as number) +
              inner +
              bslice(buf, param.closeStart, param.closeEnd as number);
          } else if (param) {
            // Paired in the translation but not a paired element at
            // the call site — substitute the element/expression and
            // keep the inner content beside it.
            out += renderStandalone(param, buf) + inner;
          } else {
            out += inner;
          }
          cursor = close.end;
          i = closeIdx + 1;
          continue;
        }
        if (param) {
          out += renderStandalone(param, buf);
        } else {
          // Unknown token — translator artifact. Escape it as text.
          out += escapeJSXText(translatedText.slice(tok.start, tok.end));
        }
        cursor = tok.end;
      } else {
        // Unmatched close — drop it rather than leak a raw token.
        cursor = tok.end;
      }
      i++;
    }
    if (cursor < charEnd) out += escapeJSXText(translatedText.slice(cursor, charEnd));
    return out;
  };

  return render(0, translatedText.length, 0, tokens.length - 1);
}

function renderStandalone(param: ParamInfo, buf: Buffer): string {
  if (param.name.startsWith("=")) {
    // Whole element / JSX-bearing expression: splice its source. A
    // jsx:node occurrence (bare expression like `cond && <X/>`) needs
    // re-wrapping in an expression container.
    const src = bslice(buf, param.fullStart, param.fullEnd);
    return param.kind === "node" ? `{${src}}` : src;
  }
  return `{${bslice(buf, param.exprStart, param.exprEnd)}}`;
}

// ─── Runtime Fallback Expression ─────────────────────────────

function buildFallbackExpr(text: string, paramList: ParamInfo[], buf: Buffer): string {
  if (paramList.length === 0) {
    return `"${text.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
  }

  let template = "`";
  const tokenRegex = /\{([^}]+)\}/g;
  let lastIndex = 0;
  let match;

  while ((match = tokenRegex.exec(text)) !== null) {
    template += text.slice(lastIndex, match.index).replace(/`/g, "\\`");
    const tokenName = match[1];
    const param = paramList.find((p) => p.name === tokenName);
    if (param && !param.name.startsWith("=")) {
      template += `\${${bslice(buf, param.exprStart, param.exprEnd)}}`;
    } else {
      template += match[0];
    }
    lastIndex = match.index + match[0].length;
  }
  template += text.slice(lastIndex).replace(/`/g, "\\`");
  template += "`";

  return template;
}

// ─── Attribute Processing ────────────────────────────────────

function processAttributes(
  el: JSXElement,
  ancestors: JSXElement[],
  componentMap: Record<string, string>,
  mode: "inline" | "runtime",
  dict: Record<string, string> | null,
  locNote: string | undefined,
  s: (offset: number) => number,
  ops: TransformOp[],
  hashes: Set<string>,
): { usedRuntime: boolean; pairs: AttrPair[] } {
  let usedRuntime = false;
  const pairs: AttrPair[] = [];

  for (const attr of el.opening.attributes || []) {
    if (attr.type !== "JSXAttribute") continue;
    if (attr.name.type !== "Identifier") continue;

    const attrName = attr.name.value;
    if (!isTranslatableAttribute(attrName, getTagName(el) ?? "")) continue;
    if (!attr.value) continue;

    const jsxPath = buildJSXPath(ancestors, el, componentMap);

    // Plain `prop="literal"` — rewrite the whole value (including
    // its surrounding quotes) to either an inline translation or a
    // `{__t(...)}` lookup.
    if (attr.value.type === "StringLiteral") {
      const text = attr.value.value;
      if (!text.trim()) continue;
      const context = `${jsxPath}[${attrName}]`;
      const desc = locNote ? `${context}${CONTEXT_SEPARATOR}${locNote}` : context;
      const hk = hashKey(text, desc);

      pairs.push({
        name: attrName,
        hash: hk,
        source: text,
        target: mode === "inline" ? dict?.[hk] : undefined,
      });
      const valueStart = s(attr.value.span.start);
      const valueEnd = s(attr.value.span.end);
      if (mode === "inline") {
        const translated = dict?.[hk] || text;
        ops.push({
          offset: valueStart,
          deleteCount: valueEnd - valueStart,
          insert: `"${escapeJSXAttr(translated)}"`,
        });
      } else {
        ops.push({
          offset: valueStart,
          deleteCount: valueEnd - valueStart,
          insert: `{__t("${hk}", "${text.replace(/"/g, '\\"')}")}`,
        });
        hashes.add(hk);
        usedRuntime = true;
      }
      continue;
    }

    // `prop={cond ? "A" : "B"}` — rewrite each string-literal branch
    // in place so the runtime evaluates the condition and looks up
    // the branch-specific hash. Contexts mirror the extractor's
    // `::0` / `::1` branch suffixes so hashes align.
    if (
      attr.value.type === "JSXExpressionContainer" &&
      attr.value.expression.type === "ConditionalExpression"
    ) {
      const cond = attr.value.expression;
      if (cond.consequent.type !== "StringLiteral" || cond.alternate.type !== "StringLiteral") {
        continue;
      }
      for (const [branchIndex, literal] of [
        [0, cond.consequent] as const,
        [1, cond.alternate] as const,
      ]) {
        const text = literal.value;
        if (!text.trim()) continue;
        const context = `${jsxPath}[${attrName}::${branchIndex}]`;
        const desc = locNote ? `${context}${CONTEXT_SEPARATOR}${locNote}` : context;
        const hk = hashKey(text, desc);
        pairs.push({
          name: `${attrName}::${branchIndex}`,
          hash: hk,
          source: text,
          target: mode === "inline" ? dict?.[hk] : undefined,
        });
        const start = s(literal.span.start);
        const end = s(literal.span.end);
        if (mode === "inline") {
          const translated = dict?.[hk] || text;
          ops.push({
            offset: start,
            deleteCount: end - start,
            // These literals sit inside a JS expression container
            // (`{cond ? "A" : "B"}`), so JSON escaping applies.
            insert: JSON.stringify(translated),
          });
        } else {
          ops.push({
            offset: start,
            deleteCount: end - start,
            insert: `__t("${hk}", "${text.replace(/"/g, '\\"')}")`,
          });
          hashes.add(hk);
          usedRuntime = true;
        }
      }
      continue;
    }
  }

  return { usedRuntime, pairs };
}

// ─── Review-mode stamping + manifest ─────────────────────────

/**
 * One translatable attribute on an element: its prop name, block
 * hash, source text, and (inline mode) the baked target. The hash
 * feeds the `data-kapi-attr` stamp; the text feeds the review
 * manifest so the hosted overlay can show a placeholder/aria string
 * without a separate extract pass.
 */
type AttrPair = { name: string; hash: string; source: string; target: string | undefined };

/**
 * Project-relative module path. Bundlers hand the transform absolute
 * ids; the review stamp + manifest read like the extract-side
 * `properties.file`.
 */
function toRelFile(filename: string): string {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const cwd: string = (globalThis as any).process?.cwd?.() ?? "";
  let rel = filename;
  if (cwd && rel.startsWith(cwd)) rel = rel.slice(cwd.length).replace(/^\/+/, "");
  return rel;
}

/**
 * Merge one block's review data into the file's manifest. First write
 * wins for source + properties (a block reached twice keeps its
 * original context); targets union so a multi-locale accumulation
 * keeps every locale. Empty/absent targets are skipped so the overlay
 * shows "untranslated" rather than an empty string.
 */
function recordReviewEntry(
  entries: ReviewManifest,
  info: {
    hash: string;
    source: string;
    element: string;
    filename: string;
    line: number;
    locNote: string | undefined;
    target: string | undefined;
    locale: string | undefined;
  },
): void {
  let e = entries[info.hash];
  if (!e) {
    e = { source: info.source, targets: {}, properties: {}, annotations: [] };
    entries[info.hash] = e;
  }
  if (!e.source) e.source = info.source;
  if (!e.properties.file) {
    e.properties = { file: toRelFile(info.filename), line: info.line, element: info.element };
    if (info.locNote) e.properties.locNote = info.locNote;
  }
  if (info.locale && info.target !== undefined && info.target !== "") {
    e.targets[info.locale] ??= info.target;
  }
}

/**
 * Insert `data-kapi-id` / `data-kapi-loc` / `data-kapi-attr`
 * attributes into the element's opening tag so the review overlay
 * can map a DOM node back to its block(s). Insertion lands just
 * before the tag's closing `>` (or `/>`), which keeps it disjoint
 * from attribute-value ops (inside the tag) and the content op
 * (after the tag).
 */
function stampReviewAttributes(
  el: JSXElement,
  buf: Buffer,
  s: (n: number) => number,
  ops: TransformOp[],
  filename: string,
  code: string,
  blockHash: string | null,
  attrPairs: AttrPair[],
): void {
  if (!blockHash && attrPairs.length === 0) return;
  const openEnd = s(el.opening.span.end);
  // Walk back over the closing bracket sequence: `>`, `/>`, `/ >`.
  let insertAt = openEnd - 1; // at the `>`
  while (insertAt > 0 && (buf[insertAt - 1] === 0x2f || buf[insertAt - 1] === 0x20)) insertAt--;

  const line = lineFromOffset(code, s(el.span.start));
  const parts: string[] = [];
  if (blockHash) parts.push(`data-kapi-id="${blockHash}"`);
  if (attrPairs.length > 0) {
    const spec = attrPairs.map((p) => `${p.name}:${p.hash}`).join(" ");
    parts.push(`data-kapi-attr="${spec}"`);
  }
  parts.push(`data-kapi-loc="${toRelFile(filename).replace(/"/g, "")}:${line}"`);
  ops.push({ offset: insertAt, deleteCount: 0, insert: ` ${parts.join(" ")}` });
}

// ─── Local helpers ───────────────────────────────────────────
// Transform-specific span helpers. The AST + translatability
// utilities are imported from ../extract/… so extract and transform
// stay in lock-step.

function getOpeningTagEnd(el: JSXElement, s: (n: number) => number): number | null {
  return el.opening?.span ? s(el.opening.span.end) : null;
}

function getClosingTagStart(el: JSXElement, s: (n: number) => number): number | null {
  return el.closing?.span ? s(el.closing.span.start) : null;
}

function removeDataI18nAttrs(
  el: JSXElement,
  buf: Buffer,
  s: (n: number) => number,
  ops: TransformOp[],
) {
  for (const attr of el.opening.attributes || []) {
    if (attr.type !== "JSXAttribute" || attr.name.type !== "Identifier") continue;
    if (!attr.name.value.startsWith("data-i18n-")) continue;
    const start = s(attr.span.start);
    const end = s(attr.span.end);
    // Walk back over leading ASCII spaces in byte space (0x20).
    let deleteStart = start;
    while (deleteStart > 0 && buf[deleteStart - 1] === 0x20) deleteStart--;
    ops.push({ offset: deleteStart, deleteCount: end - deleteStart, insert: "" });
  }
}
