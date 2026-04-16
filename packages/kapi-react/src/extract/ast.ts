/**
 * Shared SWC AST helpers used by the extractor to walk JSX trees.
 * Kept minimal — every helper is called from the main walker.
 */

import type { Expression, JSXElement } from '@swc/core';

/**
 * Returns the JSX element's tag name as seen in source:
 *   <h1> → "h1"
 *   <Button> → "Button"
 *   <Icons.Plus> → "Icons.Plus"
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
 * without a value (`<div translate>`).
 */
export function getStringAttr(el: JSXElement, name: string): string | null {
  for (const attr of el.opening.attributes ?? []) {
    if (attr.type !== 'JSXAttribute' || attr.name.type !== 'Identifier') continue;
    if (attr.name.value !== name) continue;
    if (!attr.value) return '';
    if (attr.value.type === 'StringLiteral') return attr.value.value;
  }
  return null;
}

/**
 * Computes a short reference name for an expression used inside a JSX
 * container: `{count}` → "count"; `{user.name}` → "user.name"; anything
 * else falls back to "value" and callers are expected to disambiguate
 * via `dedupName`.
 */
export function exprToName(expr: Expression): string {
  if (expr.type === 'Identifier') return expr.value;
  if (expr.type === 'MemberExpression') {
    const prop = expr.property as { type?: string; value?: string };
    if (prop.type === 'Identifier') {
      const obj = expr.object as { type?: string; value?: string };
      if (obj.type === 'Identifier' && obj.value) {
        return `${obj.value}.${prop.value}`;
      }
      return prop.value ?? 'value';
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
