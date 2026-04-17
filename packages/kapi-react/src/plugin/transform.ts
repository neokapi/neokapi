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

import { readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { parseSync, type JSXElement, type Module } from '@swc/core';

import {
  containsJSX,
  dedupName,
  exprToName,
  getStringAttr,
  getTagName,
  resolveHTMLElement,
} from '../extract/ast.ts';
import { buildJSXPath } from '../extract/jsx-path.ts';
import {
  hasTranslatableText,
  isAllInlineContent,
  resolvePolicy,
} from '../extract/translatable.ts';
import { translatableAttributes } from './defaults.ts';
import { hashKey } from './hash.ts';
import { CONTEXT_SEPARATOR, type PluginOptions } from '../types.ts';

type TransformOp = {
  offset: number;
  deleteCount: number;
  insert: string;
};

type ProcessResult = {
  /** Runtime helper used by this element, if any (used to decide which imports to add). */
  runtime: 'runtime-t' | 'runtime-tx' | null;
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
  return buf.toString('utf8', start, end);
}

/**
 * Find the byte offset of the first significant token in a source —
 * i.e. the first byte that isn't whitespace, a line comment, a block
 * comment, a shebang, or a BOM. SWC's `ast.span.start` points there
 * (in global source-map space), so we subtract this from the span to
 * derive the per-parse base.
 */
function findFirstTokenByteOffset(source: string): number {
  const buf = Buffer.from(source, 'utf8');
  const len = buf.length;
  let i = 0;

  if (len >= 3 && buf[0] === 0xef && buf[1] === 0xbb && buf[2] === 0xbf) i = 3;

  if (buf[i] === 0x23 && buf[i + 1] === 0x21) {
    while (i < len && buf[i] !== 0x0a) i++;
  }

  while (i < len) {
    const c = buf[i];
    if (c === 0x20 || c === 0x09 || c === 0x0a || c === 0x0d) { i++; continue; }
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
function loadSingleDict(
  dir: string,
  locale: string,
): Record<string, string> | null {
  const filePath = join(dir, `${locale}.json`);
  if (!existsSync(filePath)) return null;
  try {
    const raw = readFileSync(filePath, 'utf-8');
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
function loadTranslationDict(
  options: PluginOptions,
): Record<string, string> | null {
  if (!options.locale) return null;

  const cacheKey = `${options.locale}:${options.fallbackLocales?.join(',') || ''}:${options.translationsDir || ''}`;
  if (translationCache[cacheKey]) return translationCache[cacheKey];

  const dir = options.translationsDir || './translations';

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

export function transform(
  code: string,
  filename: string,
  options: PluginOptions,
): { code: string } | null {
  const componentMap = options.componentMap || {};
  const rules = options.rules || [];
  const mode = options.mode || (options.locale ? 'inline' : undefined);
  if (!mode) return null;

  const dict = mode === 'inline' ? loadTranslationDict(options) : null;

  let ast: Module;
  try {
    ast = parseSync(code, {
      syntax: filename.endsWith('.tsx') ? 'typescript' : 'ecmascript',
      tsx: true,
      jsx: true,
    });
  } catch {
    return null;
  }

  const s = makeOffsetConverter(ast, code);
  const buf = Buffer.from(code, 'utf8');
  const ops: TransformOp[] = [];
  let needsT = false;
  let needsTx = false;

  walkModule(ast, (el, ancestors) => {
    const r = processElement(
      el, ancestors, buf, filename, componentMap, rules, mode, dict, options, s, ops,
    );
    if (r.runtime === 'runtime-t') needsT = true;
    if (r.runtime === 'runtime-tx') { needsT = true; needsTx = true; }
    return { skipChildren: r.consumed };
  });

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
    parts.push(Buffer.from(op.insert, 'utf8'));
    pos = op.offset + op.deleteCount;
  }
  parts.push(buf.subarray(pos));
  let result = Buffer.concat(parts).toString('utf8');

  if (needsT || needsTx) {
    const imports = [needsT ? 't as __t' : '', needsTx ? 'tx as __tx' : '']
      .filter(Boolean)
      .join(', ');
    result = `import { ${imports} } from '@neokapi/kapi-react/runtime';\n${result}`;
  }

  return { code: result };
}

// ─── AST Walking ─────────────────────────────────────────────

function walkModule(
  module: Module,
  visitor: (el: JSXElement, ancestors: JSXElement[]) => { skipChildren: boolean },
) {
  function walk(node: any, jsxAncestors: JSXElement[]) {
    if (!node || typeof node !== 'object') return;
    if (node.type === 'JSXElement') {
      const el = node as JSXElement;
      const { skipChildren } = visitor(el, jsxAncestors);
      if (skipChildren) return;
      const newAncestors = [...jsxAncestors, el];
      for (const child of el.children || []) walk(child, newAncestors);
      return;
    }
    for (const key of Object.keys(node)) {
      if (key === 'span' || key === 'type') continue;
      const val = node[key];
      if (Array.isArray(val)) {
        for (const item of val) walk(item, jsxAncestors);
      } else if (val && typeof val === 'object' && val.type) {
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
  rules: NonNullable<PluginOptions['rules']>,
  mode: 'inline' | 'runtime',
  dict: Record<string, string> | null,
  options: PluginOptions,
  s: (offset: number) => number,
  ops: TransformOp[],
): ProcessResult {
  const tagName = getTagName(el);
  if (!tagName) return { runtime: null, consumed: false };
  if (getStringAttr(el, 'translate') === 'no') return { runtime: null, consumed: false };

  const htmlElement = resolveHTMLElement(tagName, componentMap);
  if (!htmlElement) return { runtime: null, consumed: false };

  const policy = resolvePolicy(htmlElement, el, rules);

  let usedRuntime: 'runtime-t' | 'runtime-tx' | null = processAttributes(
    el, ancestors, componentMap, mode, dict, policy.locNote, s, ops,
  ) ? 'runtime-t' : null;

  if (!policy.translate) return { runtime: usedRuntime, consumed: false };
  if (!hasTranslatableText(el)) return { runtime: usedRuntime, consumed: false };
  if (!isAllInlineContent(el, componentMap)) return { runtime: usedRuntime, consumed: false };

  const jsxPath = buildJSXPath(ancestors, el, componentMap);
  const locNote = policy.locNote;
  const desc = locNote ? `${jsxPath}${CONTEXT_SEPARATOR}${locNote}` : jsxPath;

  const contentStart = getOpeningTagEnd(el, s);
  const contentEnd = getClosingTagStart(el, s);
  if (contentStart === null || contentEnd === null) return { runtime: usedRuntime, consumed: false };

  const { text, paramList } = extractTextTemplate(el, s);
  const hk = hashKey(text, desc);

  const hasInlineElements = paramList.some(p => p.name.startsWith('='));

  if (mode === 'inline') {
    const translated = dict?.[hk];

    // Missing translation detection
    if (!translated && options.strict !== false) {
      const msg = `[neokapi] Missing translation for "${text}" (hash: ${hk}, locale: ${options.locale})`;
      if (options.strict === 'error') {
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
    const regularParams = paramList.filter(p => !p.name.startsWith('='));
    const elementParams = paramList.filter(p => p.name.startsWith('='));

    const elementsObj = `{ ${elementParams.map(p => `${JSON.stringify(p.name)}: ${bslice(buf, p.fullStart, p.fullEnd)}`).join(', ')} }`;
    const paramsObj = regularParams.length > 0
      ? `, { ${regularParams.map(p => `${JSON.stringify(p.name)}: ${bslice(buf, p.exprStart, p.exprEnd)}`).join(', ')} }`
      : '';
    const fallbackText = `"${text.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;

    ops.push({
      offset: contentStart,
      deleteCount: contentEnd - contentStart,
      insert: `{__tx("${hk}", ${fallbackText}, ${elementsObj}${paramsObj})}`,
    });
    usedRuntime = 'runtime-tx';
  } else {
    // ── Runtime mode, text only → use t() ──
    const paramsObj = paramList.length > 0
      ? `, { ${paramList.map(p => `${JSON.stringify(p.name)}: ${bslice(buf, p.exprStart, p.exprEnd)}`).join(', ')} }`
      : '';
    const fallbackExpr = buildFallbackExpr(text, paramList, buf);
    ops.push({
      offset: contentStart,
      deleteCount: contentEnd - contentStart,
      insert: `{__t("${hk}", ${fallbackExpr}${paramsObj})}`,
    });
    usedRuntime = 'runtime-t';
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
function extractTextTemplate(
  el: JSXElement,
  s: (offset: number) => number,
): { text: string; paramList: ParamInfo[] } {
  let text = '';
  const paramList: ParamInfo[] = [];
  const paramNames = new Set<string>();

  for (const child of el.children || []) {
    if (child.type === 'JSXText') {
      text += child.value.replace(/\s+/g, ' ');
    } else if (child.type === 'JSXExpressionContainer') {
      if (child.expression.type === 'JSXEmptyExpression') continue;

      // If the expression contains JSX anywhere (e.g. `cond && <X/>`,
      // `a ? <X/> : <Y/>`, `fn(<X/>)`), its runtime value is a ReactNode
      // and must be captured as an element token — otherwise t() would
      // stringify it to "[object Object]" via template literal
      // interpolation. See #5. We slice the inner expression (not the
      // surrounding `{...}`) so the emitted elements map is a valid
      // object literal value.
      if (containsJSX(child.expression)) {
        const implicitName = `=m${paramList.length}`;
        text += `{${implicitName}}`;
        const expr = child.expression as { span: { start: number; end: number } };
        paramList.push({
          name: implicitName,
          exprStart: s(expr.span.start),
          exprEnd: s(expr.span.end),
          fullStart: s(expr.span.start),
          fullEnd: s(expr.span.end),
        });
        continue;
      }

      const rawName = exprToName(child.expression);
      const name = dedupName(rawName, paramNames);
      text += `{${name}}`;
      const expr = child.expression as { span: { start: number; end: number } };
      paramList.push({
        name,
        exprStart: s(expr.span.start),
        exprEnd: s(expr.span.end),
        fullStart: s(child.span.start),
        fullEnd: s(child.span.end),
      });
    } else if (child.type === 'JSXElement') {
      const childStart = s(child.span.start);
      const childEnd = s(child.span.end);
      const implicitName = `=m${paramList.length}`;
      text += `{${implicitName}}`;
      paramList.push({
        name: implicitName,
        exprStart: childStart,
        exprEnd: childEnd,
        fullStart: childStart,
        fullEnd: childEnd,
      });
    }
  }

  return { text: text.trim(), paramList };
}

// ─── Inline Translation ──────────────────────────────────────

function inlineTranslation(
  translatedText: string,
  paramList: ParamInfo[],
  buf: Buffer,
): string {
  const tokenMap = new Map<string, string>();
  for (const param of paramList) {
    if (param.name.startsWith('=')) {
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

function buildFallbackExpr(
  text: string,
  paramList: ParamInfo[],
  buf: Buffer,
): string {
  if (paramList.length === 0) {
    return `"${text.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
  }

  let template = '`';
  const tokenRegex = /\{([^}]+)\}/g;
  let lastIndex = 0;
  let match;

  while ((match = tokenRegex.exec(text)) !== null) {
    template += text.slice(lastIndex, match.index).replace(/`/g, '\\`');
    const tokenName = match[1];
    const param = paramList.find(p => p.name === tokenName);
    if (param && !param.name.startsWith('=')) {
      template += `\${${bslice(buf, param.exprStart, param.exprEnd)}}`;
    } else {
      template += match[0];
    }
    lastIndex = match.index + match[0].length;
  }
  template += text.slice(lastIndex).replace(/`/g, '\\`');
  template += '`';

  return template;
}

// ─── Attribute Processing ────────────────────────────────────

function processAttributes(
  el: JSXElement,
  ancestors: JSXElement[],
  componentMap: Record<string, string>,
  mode: 'inline' | 'runtime',
  dict: Record<string, string> | null,
  locNote: string | undefined,
  s: (offset: number) => number,
  ops: TransformOp[],
): boolean {
  let usedRuntime = false;

  for (const attr of el.opening.attributes || []) {
    if (attr.type !== 'JSXAttribute') continue;
    if (attr.name.type !== 'Identifier') continue;

    const attrName = attr.name.value;
    if (!translatableAttributes.has(attrName)) continue;
    if (!attr.value || attr.value.type !== 'StringLiteral') continue;

    const text = attr.value.value;
    if (!text.trim()) continue;

    const jsxPath = buildJSXPath(ancestors, el, componentMap);
    const context = `${jsxPath}[${attrName}]`;
    const desc = locNote ? `${context}${CONTEXT_SEPARATOR}${locNote}` : context;
    const hk = hashKey(text, desc);

    const valueStart = s(attr.value.span.start);
    const valueEnd = s(attr.value.span.end);

    if (mode === 'inline') {
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
      usedRuntime = true;
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

function removeDataI18nAttrs(el: JSXElement, buf: Buffer, s: (n: number) => number, ops: TransformOp[]) {
  for (const attr of el.opening.attributes || []) {
    if (attr.type !== 'JSXAttribute' || attr.name.type !== 'Identifier') continue;
    if (!attr.name.value.startsWith('data-i18n-')) continue;
    const start = s(attr.span.start);
    const end = s(attr.span.end);
    // Walk back over leading ASCII spaces in byte space (0x20).
    let deleteStart = start;
    while (deleteStart > 0 && buf[deleteStart - 1] === 0x20) deleteStart--;
    ops.push({ offset: deleteStart, deleteCount: end - deleteStart, insert: '' });
  }
}
