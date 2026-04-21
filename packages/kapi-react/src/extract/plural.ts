/**
 * Plural / Select component recognition shared by the extract
 * walker (emits `PluralRun` / `SelectRun` into `Block.source`) and
 * the plugin transform (emits the ICU template into `__tx()`).
 *
 * Children-based shape (Framework AD-002):
 *
 *   <Plural count={items.length}>
 *     <Zero>Your cart is empty</Zero>
 *     <One>1 item in your cart</One>
 *     <Other><strong>{items.length}</strong> items in your cart</Other>
 *   </Plural>
 *
 *   <Select value={user.role}>
 *     <Case when="admin">Welcome, admin</Case>
 *     <Case when="guest">You're browsing as a guest</Case>
 *     <Other>Welcome, {user.name}!</Other>
 *   </Select>
 *
 * Form tag names are recognized by identifier (`Zero`, `One`, `Two`,
 * `Few`, `Many`, `Other`, `Case`). The pivot prop (`count` for Plural,
 * `value` for Select) is extracted from the opening attributes and
 * becomes the placeholder's equiv — drives CLDR plural selection
 * (or Select case matching) at runtime.
 */

import type { Expression, JSXElement } from '@swc/core';

import { exprToName, getTagName } from './ast.ts';

export type PluralFormKey = 'zero' | 'one' | 'two' | 'few' | 'many' | 'other';

/** Map of child tag → plural form key. */
const PLURAL_FORMS: Record<string, PluralFormKey> = {
  Zero: 'zero',
  One: 'one',
  Two: 'two',
  Few: 'few',
  Many: 'many',
  Other: 'other',
};

export function isPluralTag(tag: string): boolean {
  return tag === 'Plural';
}

export function isSelectTag(tag: string): boolean {
  return tag === 'Select';
}

/** Resolved info about a `<Plural>` opening. */
export interface PluralInfo {
  /** Pivot expression from the `count` prop, as a name for placeholder equiv. */
  pivotName: string;
  /** Source text of the pivot expression, for Placeholder.sourceExpr. */
  pivotSource: string;
  /** Plural forms in declaration order. */
  forms: PluralFormChild[];
}

export interface PluralFormChild {
  key: PluralFormKey;
  /** The form's JSXElement (whose children are the form content). */
  el: JSXElement;
}

/** Resolved info about a `<Select>` opening. */
export interface SelectInfo {
  /** Pivot expression from the `value` prop. */
  pivotName: string;
  pivotSource: string;
  cases: SelectCaseChild[];
  /** The `<Other>` child, if one is present. */
  otherEl?: JSXElement;
}

export interface SelectCaseChild {
  key: string;
  el: JSXElement;
}

/**
 * Parse a `<Plural>` element into its pivot + forms. Returns null
 * when the opening tag lacks a recognizable `count` prop.
 */
export function parsePlural(el: JSXElement): PluralInfo | null {
  const pivot = readExpressionAttr(el, 'count');
  if (!pivot) return null;
  const forms: PluralFormChild[] = [];
  for (const child of el.children ?? []) {
    if (child.type !== 'JSXElement') continue;
    const tag = getTagName(child);
    if (!tag) continue;
    const key = PLURAL_FORMS[tag];
    if (!key) continue;
    forms.push({ key, el: child });
  }
  return { pivotName: pivot.name, pivotSource: pivot.source, forms };
}

/**
 * Parse a `<Select>` element into its pivot + cases. Returns null
 * when the opening tag lacks a `value` prop.
 */
export function parseSelect(el: JSXElement): SelectInfo | null {
  const pivot = readExpressionAttr(el, 'value');
  if (!pivot) return null;
  const cases: SelectCaseChild[] = [];
  let otherEl: JSXElement | undefined;
  for (const child of el.children ?? []) {
    if (child.type !== 'JSXElement') continue;
    const tag = getTagName(child);
    if (!tag) continue;
    if (tag === 'Case') {
      const key = readStringAttr(child, 'when');
      if (key == null) continue;
      cases.push({ key, el: child });
    } else if (tag === 'Other') {
      otherEl = child;
    }
  }
  return { pivotName: pivot.name, pivotSource: pivot.source, cases, otherEl };
}

// ─── Attribute helpers ───────────────────────────────────────────

interface PivotAttr {
  name: string;
  source: string;
}

function readExpressionAttr(el: JSXElement, attrName: string): PivotAttr | null {
  for (const attr of el.opening.attributes ?? []) {
    if (attr.type !== 'JSXAttribute' || attr.name.type !== 'Identifier') continue;
    if (attr.name.value !== attrName) continue;
    const value = attr.value;
    if (!value) return null;
    if (value.type === 'JSXExpressionContainer') {
      if (value.expression.type === 'JSXEmptyExpression') return null;
      return {
        name: exprToName(value.expression as Expression),
        source: spanSlice(value.expression),
      };
    }
    if (value.type === 'StringLiteral') {
      // `<Plural count="3">` — uncommon but valid if literal.
      return { name: 'value', source: JSON.stringify(value.value) };
    }
  }
  return null;
}

function readStringAttr(el: JSXElement, attrName: string): string | null {
  for (const attr of el.opening.attributes ?? []) {
    if (attr.type !== 'JSXAttribute' || attr.name.type !== 'Identifier') continue;
    if (attr.name.value !== attrName) continue;
    const value = attr.value;
    if (!value) return null;
    if (value.type === 'StringLiteral') return value.value;
    if (
      value.type === 'JSXExpressionContainer' &&
      value.expression.type === 'StringLiteral'
    ) {
      return value.expression.value;
    }
  }
  return null;
}

function spanSlice(node: unknown): string {
  const span = (node as { span?: { start: number; end: number } }).span;
  // Best-effort: the plural/select parse returns source text only when
  // the caller can slice it. The walker hands buildRuns its
  // sourceSlice; here we return an empty string and let the walker
  // enrich the Placeholder metadata with the real slice.
  return span ? '' : '';
}
