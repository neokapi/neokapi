/**
 * Shared SWC AST helpers used by the extractor to walk JSX trees.
 * Kept minimal — every helper is called from the main walker.
 */

import type { Expression, JSXElement } from '@swc/core';

/**
 * Returns the JSX element's tag name as seen in source:
 *   <h1>            → "h1"
 *   <Button>        → "Button"
 *   <Icons.Plus>    → "Icons.Plus"
 *   <svg:path>      → "svg:path"
 *
 * Fragments and unnamed elements return null.
 */
export function getTagName(el: JSXElement): string | null {
  const name = el.opening?.name;
  if (!name) return null;
  if (name.type === 'Identifier') return name.value;
  if (name.type === 'JSXMemberExpression') {
    const obj = name.object as { value?: string; name?: string };
    return `${obj.value ?? obj.name ?? ''}.${name.property.value}`;
  }
  if (name.type === 'JSXNamespacedName') {
    return `${name.namespace.value}:${name.name.value}`;
  }
  return null;
}

/**
 * Resolves a source tag to its HTML element equivalent. Lowercase tags
 * pass through verbatim (`h1` → `h1`); PascalCase components route
 * through the `componentMap` (`<Button>` → `button` if mapped, null
 * otherwise). Returns null for unmapped components so callers can
 * skip them.
 */
export function resolveHTMLElement(
  tag: string,
  componentMap: Record<string, string>,
): string | null {
  if (tag[0] === tag[0].toLowerCase()) return tag;
  return componentMap[tag] ?? null;
}

/**
 * Reads a string-literal attribute value. Returns null when the
 * attribute is absent; returns an empty string when it's present
 * without a value (`<div translate>`). Also accepts
 * `foo={"bar"}` (a StringLiteral inside a JSXExpressionContainer)
 * as equivalent to `foo="bar"`.
 */
export function getStringAttr(el: JSXElement, name: string): string | null {
  for (const attr of el.opening.attributes ?? []) {
    if (attr.type !== 'JSXAttribute' || attr.name.type !== 'Identifier') continue;
    if (attr.name.value !== name) continue;
    if (!attr.value) return '';
    if (attr.value.type === 'StringLiteral') return attr.value.value;
    if (
      attr.value.type === 'JSXExpressionContainer' &&
      attr.value.expression.type === 'StringLiteral'
    ) {
      return attr.value.expression.value;
    }
  }
  return null;
}

/**
 * True when any attribute with the given name is present on the
 * element, regardless of its value shape. Used for attribute
 * selectors like `[data-tag-chip]`.
 */
export function hasAttr(el: JSXElement, name: string): boolean {
  for (const attr of el.opening.attributes ?? []) {
    if (attr.type !== 'JSXAttribute' || attr.name.type !== 'Identifier') continue;
    if (attr.name.value === name) return true;
  }
  return false;
}

/**
 * Computes a short reference name for an expression used inside a JSX
 * container:
 *   {count}           → "count"
 *   {user.name}       → "user.name"
 *   {formatDate(d)}   → "formatDate"
 *   {fmt.date(d)}     → "date"
 *
 * Other shapes fall back to "value"; callers disambiguate collisions
 * through `dedupName`. Extract and transform must agree on this
 * mapping, since it feeds the hash input template.
 */
export function exprToName(expr: Expression): string {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const anyExpr = expr as any;
  if (anyExpr.type === 'Identifier') return anyExpr.value;
  if (anyExpr.type === 'MemberExpression') {
    const prop = anyExpr.property;
    if (prop?.type === 'Identifier') {
      const obj = anyExpr.object;
      if (obj?.type === 'Identifier' && obj.value) {
        return `${obj.value}.${prop.value}`;
      }
      return prop.value ?? 'value';
    }
  }
  if (anyExpr.type === 'CallExpression') {
    const callee = anyExpr.callee;
    if (callee?.type === 'Identifier') return callee.value;
    if (callee?.type === 'MemberExpression' && callee.property?.type === 'Identifier') {
      return callee.property.value;
    }
  }
  return 'value';
}

/**
 * Reserves `name` inside `used` and returns the same name — or, if it
 * would collide, the next `name_2`, `name_3`, … variant. Matches
 * `plugin/transform.ts` so hashes stay aligned.
 */
export function dedupName(name: string, used: Set<string>): string {
  let out = name;
  let counter = 2;
  while (used.has(out)) out = `${name}_${counter++}`;
  used.add(out);
  return out;
}

/**
 * SWC returns node spans in byte offsets with no line info. The plugin
 * uses offsets; for human-readable `src:line` coordinates we compute
 * the line by counting newlines up to the offset.
 */
export function lineFromOffset(code: string, offset: number): number {
  if (offset <= 0) return 1;
  let line = 1;
  const end = Math.min(offset, code.length);
  for (let i = 0; i < end; i++) {
    if (code.charCodeAt(i) === 10) line++;
  }
  return line;
}

/**
 * True when the AST subtree rooted at `node` contains a `JSXElement`
 * or `JSXFragment` anywhere. Drives the plugin's `__tx` routing for
 * expressions like `{ok && <X/>}` and the extractor's classification
 * of `JSXExpressionContainer` children as `jsx:node` placeholders
 * vs `jsx:var`. Both sides must agree byte-for-byte, so the logic
 * lives here.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function containsJSX(node: any): boolean {
  if (!node || typeof node !== 'object') return false;
  if (node.type === 'JSXElement' || node.type === 'JSXFragment') return true;
  for (const key of Object.keys(node)) {
    if (key === 'span' || key === 'type') continue;
    const val = (node as Record<string, unknown>)[key];
    if (Array.isArray(val)) {
      for (const item of val) if (containsJSX(item)) return true;
    } else if (val && typeof val === 'object' && containsJSX(val)) {
      return true;
    }
  }
  return false;
}
