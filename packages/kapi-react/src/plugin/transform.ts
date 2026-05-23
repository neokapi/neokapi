/**
 * SWC-based transform for @neokapi/kapi-react.
 *
 * Two output modes:
 *   - inline: resolves translations at build time → translated JSX (zero runtime)
 *   - runtime: emits t() calls → resolved at runtime via OTA dictionary
 *
 * Architecture: parse with SWC, walk AST, apply string-level operations.
 * SWC's printSync compiles JSX to createElement, so we use string splicing
 * to keep the output as JSX for the downstream React plugin.
 */

import { readFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import { parseSync, type JSXElement, type Module } from "@swc/core";

import {
  containsJSX,
  dedupName,
  exprToName,
  getTagName,
  lineFromOffset,
  nearestTranslate,
  resolveHTMLElement,
} from "../extract/ast.ts";
import { buildJSXPath } from "../extract/jsx-path.ts";
import {
  isPluralTag,
  isSelectTag,
  parsePlural,
  parseSelect,
  type PluralFormKey,
} from "../extract/plural.ts";
import { hasTranslatableText, isAllInlineContent, resolvePolicy } from "../extract/translatable.ts";
import { collectTIdentifiers, walkTCalls } from "../extract/messages.ts";
import { resolveLibraryComponentMap } from "./manifests.ts";
import {
  createWarningCollector,
  formatWarning,
  type WarningCollector,
} from "../extract/warnings.ts";
import { translatableAttributes } from "./defaults.ts";
import { hashKey } from "./hash.ts";
import { CONTEXT_SEPARATOR, type PluginOptions } from "../types.ts";

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

let translationCache: Record<string, Record<string, string>> = {};

/**
 * Load a single translation JSON file. Returns flat {hash: text} dict.
 */
function loadSingleDict(dir: string, locale: string): Record<string, string> | null {
  const filePath = join(dir, `${locale}.json`);
  if (!existsSync(filePath)) return null;
  try {
    const raw = readFileSync(filePath, "utf-8");
    const data = JSON.parse(raw);
    return data[locale] || data;
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

  const cacheKey = `${options.locale}:${options.fallbackLocales?.join(",") || ""}:${options.translationsDir || ""}`;
  if (translationCache[cacheKey]) return translationCache[cacheKey];

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

  translationCache[cacheKey] = merged;
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
): { code: string; hashes: string[] } | null {
  const rules = options.rules || [];
  const mode = options.mode || (options.locale ? "inline" : undefined);
  if (!mode) return null;

  const dict = mode === "inline" ? loadTranslationDict(options) : null;

  let ast: Module;
  try {
    ast = parseSync(code, {
      syntax: filename.endsWith(".tsx") ? "typescript" : "ecmascript",
      tsx: true,
      jsx: true,
    });
  } catch {
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
  let needsT = false;
  let needsTx = false;

  walkModule(ast, (el, ancestors) => {
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
    );
    if (r.runtime === "runtime-t") needsT = true;
    if (r.runtime === "runtime-tx") {
      needsT = true;
      needsTx = true;
    }
    return { skipChildren: r.consumed };
  });

  // User-facing `t("text", params?)` calls: rewrite to
  // `__t("hash", "text", params)` so runtime dict lookup applies.
  // Same matching rule as JSX extraction — only calls bound to
  // the runtime import are touched, not a random local `t()`.
  //
  // Note on inline mode: for JSX, inline mode inlines the
  // translated string verbatim (zero runtime lookup). For t()
  // calls the savings are marginal and implementing both paths
  // doubles the test surface, so we always emit __t() here. In
  // inline mode the plugin still ships the hash-keyed dict as
  // part of the runtime bundle, same as today.
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
    const importLine = `import { ${imports} } from '@neokapi/kapi-react/runtime';`;
    const directiveMatch = result.match(/^(["']use (?:client|server)["']\s*;?\s*\n)/);
    if (directiveMatch) {
      result = directiveMatch[1] + importLine + "\n" + result.slice(directiveMatch[1].length);
    } else {
      result = importLine + "\n" + result;
    }
  }

  return { code: result, hashes: Array.from(hashes) };
}

// ─── AST Walking ─────────────────────────────────────────────

function walkModule(
  module: Module,
  visitor: (el: JSXElement, ancestors: JSXElement[]) => { skipChildren: boolean },
) {
  function walk(node: any, jsxAncestors: JSXElement[]) {
    if (!node || typeof node !== "object") return;
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
  let usedRuntime: "runtime-t" | "runtime-tx" | null = processAttributes(
    el,
    ancestors,
    componentMap,
    mode,
    dict,
    policy.locNote,
    s,
    ops,
    hashes,
  )
    ? "runtime-t"
    : null;

  if (!policy.translate) return { runtime: usedRuntime, consumed: false };
  if (!hasTranslatableText(el)) return { runtime: usedRuntime, consumed: false };
  if (!isAllInlineContent(el, componentMap)) return { runtime: usedRuntime, consumed: false };

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

  const { text, paramList } = extractTextTemplate(el, s);
  const hk = hashKey(text, desc);

  const hasInlineElements = paramList.some((p) => p.name.startsWith("="));

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

    const inlined = inlineTranslation(translated || text, paramList, buf);
    ops.push({
      offset: contentStart,
      deleteCount: contentEnd - contentStart,
      insert: inlined,
    });
  } else if (hasInlineElements) {
    // ── Runtime mode with inline elements → use tx() ──
    const regularParams = paramList.filter((p) => !p.name.startsWith("="));
    const elementParams = paramList.filter((p) => p.name.startsWith("="));

    const elementsObj = `{ ${elementParams.map((p) => `${JSON.stringify(p.name)}: ${bslice(buf, p.fullStart, p.fullEnd)}`).join(", ")} }`;
    const paramsObj =
      regularParams.length > 0
        ? `, { ${regularParams.map((p) => `${JSON.stringify(p.name)}: ${bslice(buf, p.exprStart, p.exprEnd)}`).join(", ")} }`
        : "";
    const fallbackText = `"${text.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;

    ops.push({
      offset: contentStart,
      deleteCount: contentEnd - contentStart,
      insert: `{__tx("${hk}", ${fallbackText}, ${elementsObj}${paramsObj})}`,
    });
    hashes.add(hk);
    usedRuntime = "runtime-tx";
  } else {
    // ── Runtime mode, text only → use t() ──
    const paramsObj =
      paramList.length > 0
        ? `, { ${paramList.map((p) => `${JSON.stringify(p.name)}: ${bslice(buf, p.exprStart, p.exprEnd)}`).join(", ")} }`
        : "";
    const fallbackExpr = buildFallbackExpr(text, paramList, buf);
    ops.push({
      offset: contentStart,
      deleteCount: contentEnd - contentStart,
      insert: `{__t("${hk}", ${fallbackExpr}${paramsObj})}`,
    });
    hashes.add(hk);
    usedRuntime = "runtime-t";
  }

  removeDataI18nAttrs(el, buf, s, ops);
  return { runtime: usedRuntime, consumed: true };
}

// ─── Text Extraction ─────────────────────────────────────────

type ParamInfo = {
  name: string;
  exprStart: number;
  exprEnd: number;
  fullStart: number;
  fullEnd: number;
};

/**
 * True if an AST node contains a JSXElement or JSXFragment anywhere in
 * its subtree. Used to detect expression containers whose runtime value
 * is a ReactNode rather than a string (see #5).
 */
interface TemplateState {
  paramList: ParamInfo[];
  paramNames: Set<string>;
  /**
   * Running counter for positional `{=mN}` tokens. Bumps once per
   * expression-container or JSXElement child — regardless of
   * whether the child ends up with an `=m` name or a named param —
   * so extract and transform agree on the N values for the same
   * inline JSX placement. Pivot props of `<Plural>` / `<Select>` do
   * NOT bump this counter; they're registered separately by
   * `registerPivot` and kept out of the element-numbering stream.
   */
  elementIndex: number;
}

function extractTextTemplate(
  el: JSXElement,
  s: (offset: number) => number,
): { text: string; paramList: ParamInfo[] } {
  const state: TemplateState = {
    paramList: [],
    paramNames: new Set(),
    elementIndex: 0,
  };
  const text = walkChildrenForTemplate(el.children ?? [], state, s);
  return { text: text.trim(), paramList: state.paramList };
}

function walkChildrenForTemplate(
  children: readonly import("@swc/core").JSXElementChild[],
  state: TemplateState,
  s: (offset: number) => number,
): string {
  let text = "";
  for (const child of children) {
    if (child.type === "JSXText") {
      text += child.value.replace(/\s+/g, " ");
      continue;
    }
    if (child.type === "JSXExpressionContainer") {
      if (child.expression.type === "JSXEmptyExpression") continue;
      const expr = child.expression as { span: { start: number; end: number } };
      const myIndex = state.elementIndex++;

      // If the expression contains JSX anywhere (e.g. `cond && <X/>`,
      // `a ? <X/> : <Y/>`, `fn(<X/>)`), its runtime value is a
      // ReactNode and must be captured as an element token. See #5.
      if (containsJSX(child.expression)) {
        const implicitName = `=m${myIndex}`;
        text += `{${implicitName}}`;
        state.paramList.push({
          name: implicitName,
          exprStart: s(expr.span.start),
          exprEnd: s(expr.span.end),
          fullStart: s(expr.span.start),
          fullEnd: s(expr.span.end),
        });
        continue;
      }

      const rawName = exprToName(child.expression);
      const name = dedupName(rawName, state.paramNames);
      text += `{${name}}`;
      state.paramList.push({
        name,
        exprStart: s(expr.span.start),
        exprEnd: s(expr.span.end),
        fullStart: s(child.span.start),
        fullEnd: s(child.span.end),
      });
      continue;
    }
    if (child.type !== "JSXElement") continue;

    const tag = getTagName(child);
    // Plural/Select don't consume an `=mN` slot — they produce the
    // ICU sub-template inline. Their pivot is registered separately.
    if (tag && isPluralTag(tag)) {
      const icu = buildPluralTemplate(child, state, s);
      if (icu) {
        text += icu;
        continue;
      }
    }
    if (tag && isSelectTag(tag)) {
      const icu = buildSelectTemplate(child, state, s);
      if (icu) {
        text += icu;
        continue;
      }
    }

    const childStart = s(child.span.start);
    const childEnd = s(child.span.end);
    const implicitName = `=m${state.elementIndex++}`;
    state.paramList.push({
      name: implicitName,
      exprStart: childStart,
      exprEnd: childEnd,
      fullStart: childStart,
      fullEnd: childEnd,
    });
    const innerChildren = child.children ?? [];
    if (innerChildren.length === 0) {
      // Zero children → standalone marker. Runtime substitutes the
      // element directly when no matching `{/=mN}` close exists.
      text += `{${implicitName}}`;
      continue;
    }
    // Has children → paired markers around the recursively-walked
    // inner content. Runtime cloneElements the wrapping element with
    // the rendered inner content as its children.
    text += `{${implicitName}}`;
    text += walkChildrenForTemplate(innerChildren, state, s);
    text += `{/${implicitName}}`;
  }
  return text;
}

/**
 * Build the ICU `{pivot, plural, ...}` template for a `<Plural>`
 * element, inlining each form's content through the same walker so
 * inline elements (`<strong>`) inside a form become `{=mN}` tokens
 * in the paramList. The pivot variable is registered as a named
 * param so the runtime can pass it at call time.
 */
function buildPluralTemplate(
  el: JSXElement,
  state: TemplateState,
  s: (offset: number) => number,
): string | null {
  const info = parsePlural(el);
  if (!info) return null;
  const pivotName = registerPivot(
    el,
    "count",
    info.pivotName,
    state.paramList,
    state.paramNames,
    s,
  );
  if (!pivotName) return null;
  const parts: string[] = [];
  for (const form of info.forms) {
    const formText = walkChildrenForTemplate(form.el.children ?? [], state, s);
    parts.push(`${form.key} {${formText.trim()}}`);
  }
  return `{${pivotName}, plural, ${parts.join(" ")}}`;
}

function buildSelectTemplate(
  el: JSXElement,
  state: TemplateState,
  s: (offset: number) => number,
): string | null {
  const info = parseSelect(el);
  if (!info) return null;
  const pivotName = registerPivot(
    el,
    "value",
    info.pivotName,
    state.paramList,
    state.paramNames,
    s,
  );
  if (!pivotName) return null;
  const parts: string[] = [];
  for (const c of info.cases) {
    const formText = walkChildrenForTemplate(c.el.children ?? [], state, s);
    parts.push(`${c.key} {${formText.trim()}}`);
  }
  if (info.otherEl) {
    const formText = walkChildrenForTemplate(info.otherEl.children ?? [], state, s);
    parts.push(`other {${formText.trim()}}`);
  }
  return `{${pivotName}, select, ${parts.join(" ")}}`;
}

/**
 * Registers the pivot prop of a `<Plural>` / `<Select>` as a named
 * param so the plugin emits `{ count: items.length }` in the runtime
 * call and the runtime's `resolveICU` can evaluate the rule.
 */
/**
 * Pivot is NOT added to `paramNames`. Form bodies commonly reference
 * the pivot variable (`<Other>{count} items</Other>`); we want that
 * `{count}` to resolve to the pivot's value, not a deduped
 * `count_2`. The pivot param entry is emitted once here — if form
 * bodies reference the same name, they'll push a duplicate
 * ParamInfo that happens to share the name. That's fine: the
 * emitted params object ends up with `{ count: count }` the first
 * time and benign repeats after. Runtime substitution walks the
 * template tokens, not the paramList.
 */
function registerPivot(
  el: JSXElement,
  propName: "count" | "value",
  pivotName: string,
  paramList: ParamInfo[],
  _paramNames: Set<string>,
  s: (offset: number) => number,
): string | null {
  for (const attr of el.opening.attributes ?? []) {
    if (attr.type !== "JSXAttribute" || attr.name.type !== "Identifier") continue;
    if (attr.name.value !== propName) continue;
    const value = attr.value;
    if (!value) return null;
    if (value.type === "JSXExpressionContainer") {
      const expr = value.expression as { span: { start: number; end: number } };
      paramList.push({
        name: pivotName,
        exprStart: s(expr.span.start),
        exprEnd: s(expr.span.end),
        fullStart: s(expr.span.start),
        fullEnd: s(expr.span.end),
      });
      return pivotName;
    }
    if (value.type === "StringLiteral") {
      paramList.push({
        name: pivotName,
        exprStart: s(value.span.start),
        exprEnd: s(value.span.end),
        fullStart: s(value.span.start),
        fullEnd: s(value.span.end),
      });
      return pivotName;
    }
  }
  return null;
}

// Silence unused-import warnings when only a subset of plural helpers
// is referenced in a tsc context — keeps the lint narrow to real usage.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
type _PluralFormKeyUsed = PluralFormKey;

// ─── Inline Translation ──────────────────────────────────────

function inlineTranslation(translatedText: string, paramList: ParamInfo[], buf: Buffer): string {
  const tokenMap = new Map<string, string>();
  for (const param of paramList) {
    if (param.name.startsWith("=")) {
      tokenMap.set(param.name, bslice(buf, param.fullStart, param.fullEnd));
    } else {
      tokenMap.set(param.name, `{${bslice(buf, param.exprStart, param.exprEnd)}}`);
    }
  }

  return translatedText.replace(/\{([^}]+)\}/g, (match, tokenName) => {
    return tokenMap.get(tokenName) ?? match;
  });
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
): boolean {
  let usedRuntime = false;

  for (const attr of el.opening.attributes || []) {
    if (attr.type !== "JSXAttribute") continue;
    if (attr.name.type !== "Identifier") continue;

    const attrName = attr.name.value;
    if (!translatableAttributes.has(attrName)) continue;
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

      const valueStart = s(attr.value.span.start);
      const valueEnd = s(attr.value.span.end);
      if (mode === "inline") {
        const translated = dict?.[hk] || text;
        ops.push({
          offset: valueStart,
          deleteCount: valueEnd - valueStart,
          insert: `"${translated}"`,
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
        const start = s(literal.span.start);
        const end = s(literal.span.end);
        if (mode === "inline") {
          const translated = dict?.[hk] || text;
          ops.push({
            offset: start,
            deleteCount: end - start,
            insert: `"${translated}"`,
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

  return usedRuntime;
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
