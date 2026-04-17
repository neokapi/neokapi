/**
 * Translatability decisions: does this JSX element deserve a Block?
 * Should its children be walked as inline content or skipped?
 *
 * Delegates the element-level policy to the `plugin/defaults`
 * tables (`getTranslatability`, `inlineElements`) so the extractor
 * and the build-time transform make the same calls.
 */

import type { JSXElement } from '@swc/core';

import { getTranslatability, inlineElements } from '../plugin/defaults.ts';
import type { PluginOptions } from '../types.ts';
import { getStringAttr, getTagName, hasAttr, resolveHTMLElement } from './ast.ts';

export type Rule = NonNullable<PluginOptions['rules']>[number];

/** Resolved policy for one JSX element. */
export interface ElementPolicy {
  /** Whether this element's children should be extracted as a Block. */
  translate: boolean;
  /** Translator-facing note from a rule or `data-i18n-note`. */
  locNote: string | undefined;
}

/**
 * Applies the default table + any matching user rules. Returns the
 * final decision for this element.
 */
export function resolvePolicy(
  htmlElement: string,
  el: JSXElement,
  rules: readonly Rule[],
): ElementPolicy {
  let translate = getTranslatability(htmlElement) === 'yes';
  let locNote: string | undefined;

  for (const rule of rules) {
    if (!matchesRule(rule, htmlElement, el)) continue;
    if (rule.translate !== undefined) translate = rule.translate;
    if (rule.locNote) locNote = rule.locNote;
  }

  locNote ??= getStringAttr(el, 'data-i18n-note') ?? undefined;
  return { translate, locNote };
}

/**
 * Selector shapes:
 *   - `tag`           matches by HTML element name
 *   - `.className`    matches when className list contains the name
 *   - `[attr]`        matches when the attribute is present
 *   - `[attr="val"]`  matches when the attribute's string literal equals val
 */
function matchesRule(rule: Rule, htmlElement: string, el: JSXElement): boolean {
  const selector = rule.selector;
  if (selector.startsWith('.')) {
    const className = selector.slice(1);
    const classAttr = getStringAttr(el, 'className');
    return !!classAttr && classAttr.split(/\s+/).includes(className);
  }
  if (selector.startsWith('[') && selector.endsWith(']')) {
    const inner = selector.slice(1, -1);
    const eq = inner.indexOf('=');
    if (eq < 0) return hasAttr(el, inner);
    const name = inner.slice(0, eq);
    const want = inner.slice(eq + 1).replace(/^["']|["']$/g, '');
    return getStringAttr(el, name) === want;
  }
  return selector === htmlElement;
}

/**
 * A JSX element produces a Block only when its children are all
 * inline — text, expression containers, or elements that belong to
 * the shared `inlineElements` table. Any block-level child (another
 * paragraph, a list, a fragment) disqualifies it so the nested block
 * gets its own walk instead.
 */
export function isAllInlineContent(
  el: JSXElement,
  componentMap: Record<string, string>,
): boolean {
  for (const child of el.children ?? []) {
    if (child.type === 'JSXText' || child.type === 'JSXExpressionContainer') continue;
    if (child.type === 'JSXElement') {
      const tag = getTagName(child);
      if (!tag) return false;
      const html = resolveHTMLElement(tag, componentMap);
      if (html && inlineElements.has(html)) continue;
      return false;
    }
    // JSXSpreadChild, JSXFragment → not representable as runs.
    return false;
  }
  return true;
}

/**
 * True when the element carries at least one translatable child:
 * non-whitespace JSX text, or an inline JSX element that itself
 * holds translatable content. Expression containers alone don't
 * count — a lone `{variable}` or `{icon}` isn't something a
 * translator can edit, and at runtime `t()` would stringify
 * a React-element value to "[object Object]". Plugin-side
 * `hasTranslatableText` applies the same rule so extract and
 * transform stay aligned.
 */
export function hasTranslatableText(el: JSXElement): boolean {
  for (const child of el.children ?? []) {
    if (child.type === 'JSXText' && child.value.trim().length > 0) return true;
    if (child.type === 'JSXElement') {
      const tag = getTagName(child);
      if (tag && inlineElements.has(tag) && hasTranslatableText(child)) return true;
    }
  }
  return false;
}
