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

/**
 * Reads a byte range of the source with every op nested inside it
 * already applied. A block op splices the source of its own params
 * through this, so a conditional inside a translated sentence carries
 * its own `__t` call rather than reaching the reader in the source
 * language.
 */
type SliceFn = (start: number, end: number) => string;

type TransformOp = {
  offset: number;
  deleteCount: number;
  /** The replacement text, or a builder for it (see {@link SliceFn}). */
  insert: string | ((slice: SliceFn) => string);
  /**
   * Byte ranges this op splices verbatim out of the source. An op that
   * lands inside one of them composes; an op inside the replaced range
   * but outside every slot would vanish from the output, so
   * `renderOps` refuses to build it.
   */
  slots?: ReadonlyArray<readonly [number, number]>;
};

/** One op with the ops nested inside its replaced range. */
type OpNode = { op: TransformOp; children: OpNode[] };

/** Whether `[start, end)` lies inside the range `op` replaces. */
function opContains(op: TransformOp, start: number, end: number): boolean {
  return start >= op.offset && end <= op.offset + op.deleteCount;
}

/** Whether `[start, end)` lies inside one of the ranges `op` splices. */
function opServes(op: TransformOp, start: number, end: number): boolean {
  for (const [a, b] of op.slots ?? []) if (start >= a && end <= b) return true;
  return false;
}

function rangeText(op: TransformOp): string {
  return `[${op.offset}, ${op.offset + op.deleteCount})`;
}

/**
 * Apply every op to `buf` and return the rewritten source.
 *
 * Ops nest: a translated element replaces its whole content range and
 * splices the source of each param back into the call it emits, so an
 * op inside one of those params is applied to the text being spliced.
 * Anything else that overlaps is a bug in the walk, and throws rather
 * than producing malformed output or dropping a translation (see #3,
 * #2522).
 */
function renderOps(buf: Buffer, ops: readonly TransformOp[], filename: string): string {
  // Outermost first at a shared offset, so a container is on the stack
  // before what it contains.
  const sorted = [...ops].sort((a, b) => a.offset - b.offset || b.deleteCount - a.deleteCount);

  const roots: OpNode[] = [];
  const stack: OpNode[] = [];
  for (const op of sorted) {
    const end = op.offset + op.deleteCount;
    while (stack.length > 0) {
      const top = stack[stack.length - 1].op;
      if (op.offset >= top.offset + top.deleteCount) {
        stack.pop();
        continue;
      }
      if (end > top.offset + top.deleteCount) {
        throw new Error(
          `[neokapi] overlapping transform ops in ${filename}: ` +
            `${rangeText(top)} and ${rangeText(op)}`,
        );
      }
      break;
    }
    const node: OpNode = { op, children: [] };
    const parent = stack[stack.length - 1];
    if (parent) {
      if (!opServes(parent.op, op.offset, end)) {
        throw new Error(
          `[neokapi] transform op ${rangeText(op)} in ${filename} sits inside ` +
            `${rangeText(parent.op)}, which splices none of it — the ` +
            `translation it carries would never reach the output`,
        );
      }
      parent.children.push(node);
    } else {
      roots.push(node);
    }
    stack.push(node);
  }

  const renderRange = (children: readonly OpNode[], from: number, to: number): string => {
    let out = "";
    let pos = from;
    for (const child of children) {
      const start = child.op.offset;
      const end = start + child.op.deleteCount;
      // A slot covers part of the parent's range, so the siblings that
      // fall outside this one are rendered when their own slot is read.
      if (start < pos || end > to) continue;
      out += buf.toString("utf8", pos, start);
      out += renderNode(child);
      pos = end;
    }
    return out + buf.toString("utf8", pos, to);
  };

  const renderNode = (node: OpNode): string =>
    typeof node.op.insert === "string"
      ? node.op.insert
      : node.op.insert((a, b) => renderRange(node.children, a, b));

  return renderRange(roots, 0, buf.length);
}

/**
 * Which runtime helpers a subtree reached for, deciding the import line. One
 * element can want both: an `aria-label` goes to `__t` while the sentence it
 * labels goes to `__tx`, so the two answers are tracked apart rather than
 * collapsed into the last one written.
 */
type RuntimeUse = { t: boolean; tx: boolean };

const NO_RUNTIME: RuntimeUse = { t: false, tx: false };

type ProcessResult = {
  /** Runtime helpers used by this element (used to decide which imports to add). */
  runtime: RuntimeUse;
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
    (el, ancestors, consumed) => {
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
        consumed,
      );
      if (r.runtime.t) needsT = true;
      if (r.runtime.tx) needsTx = true;
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
      if (r.runtime.t) needsT = true;
      if (r.runtime.tx) needsTx = true;
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
  // An element op already queued replaces the bytes of every `t()`
  // call inside it. Where the call sits in one of that op's params the
  // rewrite still applies — `renderOps` runs it over the source the
  // call site splices — so `t()` in a conditional or an interpolation
  // is translated like any other. Where the call is part of the flat
  // template instead, the block carries its text and a second op would
  // have nowhere to go.
  const queued = ops.slice();
  const swallowedByBlock = (start: number, end: number): boolean => {
    for (const op of queued) {
      if (opContains(op, start, end) && !opServes(op, start, end)) return true;
    }
    return false;
  };
  for (const call of walkTCalls(ast, tNames, sourceSlice)) {
    const callStart = s(call.node.span.start);
    const callEnd = s(call.node.span.end);
    if (swallowedByBlock(callStart, callEnd)) continue;

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
  let result = renderOps(buf, ops, filename);

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
  visitor: (
    el: JSXElement,
    ancestors: JSXElement[],
    consumed: boolean,
  ) => { skipChildren: boolean },
  fragmentVisitor?: (frag: JSXFragment, ancestors: JSXElement[]) => { skipChildren: boolean },
) {
  function walk(node: any, jsxAncestors: JSXElement[], consumed: boolean) {
    if (!node || typeof node !== "object") return;
    if (node.type === "JSXFragment" && fragmentVisitor) {
      const frag = node as JSXFragment;
      const { skipChildren } =
        consumed || !fragmentVisitor
          ? { skipChildren: false }
          : fragmentVisitor(frag, jsxAncestors);
      const childrenConsumed = consumed || skipChildren;
      for (const child of frag.children || []) {
        walk(
          child,
          jsxAncestors,
          child.type === "JSXExpressionContainer" ? false : childrenConsumed,
        );
      }
      return;
    }
    if (node.type === "JSXElement") {
      const el = node as JSXElement;
      const { skipChildren } = visitor(el, jsxAncestors, consumed);
      const newAncestors = [...jsxAncestors, el];
      const childrenConsumed = consumed || skipChildren;
      // A consumed element's inline children travel into its own op as
      // a flat template, so their text is already served. What the
      // block splices verbatim is not: an expression container is one
      // param, so JSX inside a conditional needs its own call to be
      // translated at all (#2522), and an element's attributes travel
      // in its source, so they need theirs (#2523). The extract walker
      // descends by the same rule, and the two have to agree —
      // otherwise a key is compiled into the dictionary and nothing
      // ever looks it up.
      for (const child of el.children || []) {
        walk(
          child,
          newAncestors,
          child.type === "JSXExpressionContainer" ? false : childrenConsumed,
        );
      }
      // The opening tag is spliced verbatim too, so JSX nested inside
      // an attribute value (`actions={<div><Button>…</Button></div>}`)
      // is ordinary JSX however deep in a consumed subtree it sits.
      if (el.opening) walk(el.opening, newAncestors, false);
      return;
    }
    for (const key of Object.keys(node)) {
      if (key === "span" || key === "type") continue;
      const val = node[key];
      if (Array.isArray(val)) {
        for (const item of val) walk(item, jsxAncestors, consumed);
      } else if (val && typeof val === "object" && val.type) {
        walk(val, jsxAncestors, consumed);
      }
    }
  }
  walk(module, [], false);
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
  /**
   * True when an enclosing block already carries this element's text.
   * Its attributes are still its own — the block splices the element's
   * source into its call, so a translated `alt` or `aria-label` rides
   * along inside that param (#2523).
   */
  consumed = false,
): ProcessResult {
  const tagName = getTagName(el);
  if (!tagName) return { runtime: NO_RUNTIME, consumed: false };
  // W3C translate inheritance: nearest explicit setting on self or
  // an ancestor wins. `translate="yes"` on a child re-enables
  // translation inside a `translate="no"` subtree. Mirrored in
  // extract/walker.ts.
  if (nearestTranslate(el, ancestors) === "no") return { runtime: NO_RUNTIME, consumed: false };

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
  const usedRuntime: RuntimeUse = { t: attrResult.usedRuntime, tx: false };

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

  if (consumed) {
    doReview(null, null, undefined);
    return { runtime: usedRuntime, consumed: false };
  }
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
    contentStart,
    contentEnd,
    ops,
    hashes,
  });
  if (blockRuntime === "runtime-t") usedRuntime.t = true;
  if (blockRuntime === "runtime-tx") usedRuntime.tx = true;

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
  if (ancestorTranslate(ancestors) === "no") return { runtime: NO_RUNTIME, consumed: false };
  if (!hasTranslatableText(frag) || !isAllInlineContent(frag, componentMap)) {
    return { runtime: NO_RUNTIME, consumed: false };
  }

  const contentStart = s(frag.opening.span.end);
  const contentEnd = s(frag.closing.span.start);

  const { flatText: text, occurrences } = buildRuns(frag, {
    componentMap,
    sourceSlice: (start, end) => bslice(buf, s(start), s(end)),
  });
  if (text === "") return { runtime: NO_RUNTIME, consumed: false };
  const paramList: ParamInfo[] = occurrences.map((o) => convertOccurrence(o, s));
  const hk = hashKey(text, FRAGMENT_DESCRIPTOR);

  const blockRuntime = emitBlockContent({
    hk,
    text,
    paramList,
    mode,
    dict,
    options,
    contentStart,
    contentEnd,
    ops,
    hashes,
  });
  return {
    runtime: { t: blockRuntime === "runtime-t", tx: blockRuntime === "runtime-tx" },
    consumed: true,
  };
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
  contentStart: number;
  contentEnd: number;
  ops: TransformOp[];
  hashes: Set<string>;
}): "runtime-t" | "runtime-tx" | null {
  const { hk, text, paramList, mode, dict, options, contentStart, contentEnd, ops, hashes } = args;
  const slots = paramSlots(paramList);

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
      const call = buildRuntimeCall(hk, translated ?? text, paramList, {
        fallbackOverride: translated ?? text,
      });
      ops.push({
        offset: contentStart,
        deleteCount: contentEnd - contentStart,
        insert: call.build,
        slots,
      });
      return call.usedTx ? "runtime-tx" : "runtime-t";
    }
    const resolved = translated ?? text;
    ops.push({
      offset: contentStart,
      deleteCount: contentEnd - contentStart,
      insert: (slice) => inlineTranslation(resolved, paramList, slice),
      slots,
    });
    return null;
  }

  const call = buildRuntimeCall(hk, text, paramList, {});
  ops.push({
    offset: contentStart,
    deleteCount: contentEnd - contentStart,
    insert: call.build,
    slots,
  });
  hashes.add(hk);
  return call.usedTx ? "runtime-tx" : "runtime-t";
}

/**
 * The byte ranges a block's call site splices out of the source: one
 * per param appearance. An op inside one of these composes into the
 * call; `renderOps` treats anything else inside the replaced range as
 * a translation that would be dropped.
 *
 * The full range covers the expression range in every param kind, so
 * one entry per param is enough.
 */
function paramSlots(paramList: readonly ParamInfo[]): ReadonlyArray<readonly [number, number]> {
  return paramList.map((p) => [p.fullStart, p.fullEnd] as const);
}

/**
 * Emit the `{__tx(...)}` / `{__t(...)}` runtime call for a block.
 * Used by runtime mode always, and by inline mode for ICU-bearing
 * blocks (where `fallbackOverride` carries the translated template).
 *
 * A block that lifted a sibling expression into a parameter goes to `__tx`
 * even with no inline element in it. The expression is arbitrary: `{icon}`,
 * `{rows}` and `{count}` are all identifiers here, and only the value at
 * render time says which of them is React content. `__tx` renders such a
 * parameter as a node and behaves exactly like `__t` for the rest, so this
 * costs a string block nothing and keeps `[object Object]` off the screen.
 * `__t` still carries a block whose only parameters are plural or select
 * pivots, which are numbers by construction, and every attribute and `t()`
 * call, which answer with a string and have nowhere to put an element.
 */
function buildRuntimeCall(
  hk: string,
  text: string,
  paramList: ParamInfo[],
  opts: { fallbackOverride?: string },
): { build: (slice: SliceFn) => string; usedTx: boolean } {
  const hasInlineElements = paramList.some((p) => p.name.startsWith("="));
  const hasExpressionParam = paramList.some((p) => p.kind === "var");
  if (hasInlineElements || hasExpressionParam) {
    const regularParams = paramList.filter((p) => !p.name.startsWith("="));
    const elementParams = paramList.filter((p) => p.name.startsWith("="));
    const fallbackText = JSON.stringify(opts.fallbackOverride ?? text);
    // Markers whose element answers for the text inside it: a `<code>` or a
    // `<kbd>` holds bytes the author wrote, and `translate="yes"` claims a
    // span back. A runtime string transform reads these to leave the same
    // characters alone that `pseudo-translate` leaves alone in the catalog.
    const markerAnswers = elementParams.filter((p) => p.translate !== undefined);
    const markersObj =
      markerAnswers.length > 0
        ? `, { ${markerAnswers.map((p) => `${JSON.stringify(p.name)}: ${JSON.stringify(p.translate)}`).join(", ")} }`
        : "";
    return {
      build: (slice) => {
        const elementsObj =
          elementParams.length > 0
            ? `{ ${elementParams.map((p) => `${JSON.stringify(p.name)}: ${slice(p.fullStart, p.fullEnd)}`).join(", ")} }`
            : "{}";
        const paramsObj =
          regularParams.length > 0
            ? `, { ${regularParams.map((p) => `${JSON.stringify(p.name)}: ${slice(p.exprStart, p.exprEnd)}`).join(", ")} }`
            : markersObj !== ""
              ? ", undefined"
              : "";
        return `{__tx("${hk}", ${fallbackText}, ${elementsObj}${paramsObj}${markersObj})}`;
      },
      usedTx: true,
    };
  }
  return {
    build: (slice) => {
      const paramsObj =
        paramList.length > 0
          ? `, { ${paramList.map((p) => `${JSON.stringify(p.name)}: ${slice(p.exprStart, p.exprEnd)}`).join(", ")} }`
          : "";
      const fallbackExpr =
        opts.fallbackOverride !== undefined
          ? JSON.stringify(opts.fallbackOverride)
          : buildFallbackExpr(text, paramList, slice);
      return `{__t("${hk}", ${fallbackExpr}${paramsObj})}`;
    },
    usedTx: false,
  };
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
    translate: o.translate,
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
  /** Paired elements only: the element's own translate answer, if it has one. */
  translate?: "yes" | "no";
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
function inlineTranslation(translatedText: string, paramList: ParamInfo[], slice: SliceFn): string {
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
              slice(param.openStart, param.openEnd as number) +
              inner +
              slice(param.closeStart, param.closeEnd as number);
          } else if (param) {
            // Paired in the translation but not a paired element at
            // the call site — substitute the element/expression and
            // keep the inner content beside it.
            out += renderStandalone(param, slice) + inner;
          } else {
            out += inner;
          }
          cursor = close.end;
          i = closeIdx + 1;
          continue;
        }
        if (param) {
          out += renderStandalone(param, slice);
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

function renderStandalone(param: ParamInfo, slice: SliceFn): string {
  if (param.name.startsWith("=")) {
    // Whole element / JSX-bearing expression: splice its source. A
    // jsx:node occurrence (bare expression like `cond && <X/>`) needs
    // re-wrapping in an expression container.
    const src = slice(param.fullStart, param.fullEnd);
    return param.kind === "node" ? `{${src}}` : src;
  }
  return `{${slice(param.exprStart, param.exprEnd)}}`;
}

// ─── Runtime Fallback Expression ─────────────────────────────

function buildFallbackExpr(text: string, paramList: ParamInfo[], slice: SliceFn): string {
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
      template += `\${${slice(param.exprStart, param.exprEnd)}}`;
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
