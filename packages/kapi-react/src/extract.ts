/**
 * String extraction — collects translatable strings from JSX source files.
 *
 * Uses SWC to parse each file and walks the AST with the same translatability
 * rules as the transform plugin. Outputs a JSON file with hashes, source text,
 * structural context, and file locations.
 */

import { parseSync, type JSXElement, type Module } from '@swc/core';
import {
  getTranslatability,
  inlineElements,
  translatableAttributes,
} from './plugin/defaults.ts';
import { hashKey } from './plugin/hash.ts';
import { CONTEXT_SEPARATOR, type PluginOptions } from './types.ts';

export type ExtractedString = {
  hash: string;
  text: string;
  context: string;
  desc?: string;
  src: string;
};

export function extractStrings(
  code: string,
  filename: string,
  options: Pick<PluginOptions, 'componentMap' | 'rules'> = {},
): ExtractedString[] {
  const componentMap = options.componentMap || {};
  const rules = options.rules || [];
  const results: ExtractedString[] = [];

  let ast: Module;
  try {
    ast = parseSync(code, {
      syntax: filename.endsWith('.tsx') ? 'typescript' : 'ecmascript',
      tsx: true,
      jsx: true,
    });
  } catch {
    return results;
  }

  walkModule(ast, (el, ancestors) => {
    collectFromElement(el, ancestors, code, filename, componentMap, rules, results);
  });

  return results;
}

function walkModule(
  module: Module,
  visitor: (el: JSXElement, ancestors: JSXElement[]) => void,
) {
  function walk(node: any, jsxAncestors: JSXElement[]) {
    if (!node || typeof node !== 'object') return;
    if (node.type === 'JSXElement') {
      const el = node as JSXElement;
      visitor(el, jsxAncestors);
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

function collectFromElement(
  el: JSXElement,
  ancestors: JSXElement[],
  code: string,
  filename: string,
  componentMap: Record<string, string>,
  rules: NonNullable<PluginOptions['rules']>,
  results: ExtractedString[],
) {
  const tagName = getTagName(el);
  if (!tagName) return;
  if (getAttrValue(el, 'translate') === 'no') return;

  const htmlElement = resolveHTMLElement(tagName, componentMap);
  if (!htmlElement) return;

  const overrides = checkRules(htmlElement, el, rules);

  let shouldTranslate: boolean;
  if (overrides.translate !== undefined) {
    shouldTranslate = overrides.translate;
  } else {
    shouldTranslate = getTranslatability(htmlElement) === 'yes';
  }

  // Collect translatable attributes
  collectAttributes(el, ancestors, componentMap, overrides.locNote, filename, results);

  if (!shouldTranslate) return;
  if (!hasTranslatableText(el)) return;
  if (!isAllInlineContent(el, componentMap)) return;

  const jsxPath = buildJSXPath(ancestors, el, componentMap);
  const locNote = overrides.locNote || getAttrValue(el, 'data-i18n-note');
  const desc = locNote ? `${jsxPath}${CONTEXT_SEPARATOR}${locNote}` : jsxPath;
  const text = extractText(el);
  const hk = hashKey(text, desc);

  results.push({
    hash: hk,
    text,
    context: jsxPath,
    ...(locNote ? { desc: locNote } : {}),
    src: `${filename}:${getLine(el)}`,
  });
}

function collectAttributes(
  el: JSXElement,
  ancestors: JSXElement[],
  componentMap: Record<string, string>,
  locNote: string | undefined,
  filename: string,
  results: ExtractedString[],
) {
  for (const attr of el.opening.attributes || []) {
    if (attr.type !== 'JSXAttribute' || attr.name.type !== 'Identifier') continue;
    const attrName = attr.name.value;
    if (!translatableAttributes.has(attrName)) continue;
    if (!attr.value || attr.value.type !== 'StringLiteral') continue;
    const text = attr.value.value;
    if (!text.trim()) continue;

    const jsxPath = buildJSXPath(ancestors, el, componentMap);
    const context = `${jsxPath}[${attrName}]`;
    const desc = locNote ? `${context}${CONTEXT_SEPARATOR}${locNote}` : context;
    const hk = hashKey(text, desc);

    results.push({
      hash: hk,
      text,
      context,
      ...(locNote ? { desc: locNote } : {}),
      src: `${filename}:${getLine(el)}`,
    });
  }
}

// ─── Text Extraction ─────────────────────────────────────────

function extractText(el: JSXElement): string {
  let text = '';
  const paramNames = new Set<string>();

  for (const child of el.children || []) {
    if (child.type === 'JSXText') {
      text += child.value.replace(/\s+/g, ' ');
    } else if (child.type === 'JSXExpressionContainer') {
      if (child.expression.type === 'JSXEmptyExpression') continue;
      const name = dedup(exprToName(child.expression), paramNames);
      text += `{${name}}`;
    } else if (child.type === 'JSXElement') {
      const implicitName = `=m${paramNames.size}`;
      paramNames.add(implicitName);
      text += `{${implicitName}}`;
    }
  }

  return text.trim();
}

// ─── AST Helpers (same as transform.ts) ──────────────────────

function getTagName(el: JSXElement): string | null {
  const name = el.opening?.name;
  if (!name) return null;
  if (name.type === 'Identifier') return name.value;
  if (name.type === 'JSXMemberExpression') {
    return `${(name.object as any).value || (name.object as any).name}.${name.property.value}`;
  }
  return null;
}

function resolveHTMLElement(tagName: string, componentMap: Record<string, string>): string | null {
  if (tagName[0] === tagName[0].toLowerCase()) return tagName;
  return componentMap[tagName] || null;
}

function getAttrValue(el: JSXElement, attrName: string): string | null {
  for (const attr of el.opening.attributes || []) {
    if (attr.type !== 'JSXAttribute') continue;
    if (attr.name.type === 'Identifier' && attr.name.value === attrName) {
      if (!attr.value) return '';
      if (attr.value.type === 'StringLiteral') return attr.value.value;
    }
  }
  return null;
}

function hasTranslatableText(el: JSXElement): boolean {
  for (const child of el.children || []) {
    if (child.type === 'JSXText' && child.value.trim().length > 0) return true;
    if (child.type === 'JSXExpressionContainer' && child.expression.type !== 'JSXEmptyExpression') return true;
    if (child.type === 'JSXElement') {
      const tag = getTagName(child);
      if (tag && inlineElements.has(tag) && hasTranslatableText(child)) return true;
    }
  }
  return false;
}

function isAllInlineContent(el: JSXElement, componentMap: Record<string, string>): boolean {
  for (const child of el.children || []) {
    if (child.type === 'JSXText' || child.type === 'JSXExpressionContainer') continue;
    if (child.type === 'JSXElement') {
      const tag = getTagName(child);
      if (!tag) return false;
      const html = resolveHTMLElement(tag, componentMap);
      if (html && inlineElements.has(html)) continue;
      return false;
    }
    if (child.type === 'JSXSpreadChild' || child.type === 'JSXFragment') return false;
  }
  return true;
}

function buildJSXPath(ancestors: JSXElement[], current: JSXElement, componentMap: Record<string, string>): string {
  const parts: string[] = [];
  for (const anc of ancestors) {
    const tag = getTagName(anc);
    if (tag) parts.push(resolveHTMLElement(tag, componentMap) || tag);
  }
  const tag = getTagName(current);
  if (tag) parts.push(resolveHTMLElement(tag, componentMap) || tag);
  return parts.join(' > ');
}

function checkRules(htmlElement: string, el: JSXElement, rules: Array<{ selector: string; translate?: boolean; locNote?: string }>): { translate?: boolean; locNote?: string } {
  let result: { translate?: boolean; locNote?: string } = {};
  for (const rule of rules) {
    let matches = false;
    if (rule.selector.startsWith('.')) {
      const className = rule.selector.slice(1);
      const classAttr = getAttrValue(el, 'className');
      if (classAttr && classAttr.split(/\s+/).includes(className)) matches = true;
    } else if (rule.selector === htmlElement) matches = true;
    if (matches) {
      if (rule.translate !== undefined) result.translate = rule.translate;
      if (rule.locNote) result.locNote = rule.locNote;
    }
  }
  return result;
}

function exprToName(expr: any): string {
  if (expr.type === 'Identifier') return expr.value;
  if (expr.type === 'MemberExpression' && expr.property?.type === 'Identifier') {
    if (expr.object?.type === 'Identifier') return `${expr.object.value}.${expr.property.value}`;
    return expr.property.value;
  }
  return 'value';
}

function dedup(name: string, used: Set<string>): string {
  let result = name;
  let counter = 2;
  while (used.has(result)) result = `${name}_${counter++}`;
  used.add(result);
  return result;
}

function getLine(el: JSXElement): number {
  // SWC span doesn't give us line numbers directly in the JS API,
  // but the span.start is a byte offset. We approximate.
  return el.span?.start ?? 0;
}
