/**
 * Translatability decisions: does this JSX element deserve a Block?
 * Should its children be walked as inline content or skipped?
 *
 * Delegates the element-level policy to the `plugin/defaults`
 * tables (`getTranslatability`, `inlineElements`) so the extractor
 * and the build-time transform make the same calls.
 */

import type { JSXElement } from "@swc/core";

import { getTranslatability, inlineElements, nonTranslatableElements } from "../plugin/defaults.ts";
import type { PluginOptions } from "../types.ts";
import { getStringAttr, getTagName, hasAttr, resolveHTMLElement } from "./ast.ts";
import { isPluralElement, isSelectElement } from "./plural.ts";

export type Rule = NonNullable<PluginOptions["rules"]>[number];

/** Resolved policy for one JSX element. */
export interface ElementPolicy {
  /** Whether this element's children should be extracted as a Block. */
  translate: boolean;
  /** Translator-facing note from a rule or `data-i18n-note`. */
  locNote: string | undefined;
  /**
   * True when `translate` was flipped from false to true by the
   * auto-promotion rule for containers / unknown components with
   * direct text. The walker records a warning for these so the
   * developer sees the inference.
   */
  promoted: boolean;
}

/**
 * Applies the default table + any matching user rules. Returns the
 * final decision for this element.
 *
 * Promotion: the W3C tables classify `<div>`, `<section>`, and other
 * container-level elements as non-translatable by default, because
 * spec-wise their direct content should be flow (not phrasing).
 * In real React codebases `<div>Label</div>` is extremely common,
 * and silently dropping the text is the wrong default. So we
 * promote any element classified as `container` (including unmapped
 * React components passed through as-is) to `translate: true`
 * when it has direct translatable text + only inline children. The
 * walker / transform record a warning so developers know which
 * elements were auto-promoted and can opt out with `translate="no"`
 * or add a `componentMap` entry for hash stability.
 *
 * User rules still win (`translate: false` on a matching selector
 * flips promoted elements back off), and an explicit `translate="yes"`
 * on the element beats the default table.
 */
export function resolvePolicy(
  htmlElement: string,
  el: JSXElement,
  rules: readonly Rule[],
  componentMap: Record<string, string> = {},
): ElementPolicy {
  const classification = getTranslatability(htmlElement);
  let translate = classification === "yes";
  let promoted = false;

  if (!translate && classification === "container") {
    if (hasTranslatableText(el) && isAllInlineContent(el, componentMap)) {
      translate = true;
      promoted = true;
    }
  }

  // W3C: an explicit `translate` on the element beats the default table, in
  // both directions. `nearestTranslate` handles the "no" half before we get
  // here; this is the opt-in one, so `<code translate="yes">Choose a
  // file</code>` is extracted as the prose its author says it is.
  if (getStringAttr(el, "translate") === "yes") translate = true;

  let locNote: string | undefined;
  for (const rule of rules) {
    if (!matchesRule(rule, htmlElement, el)) continue;
    if (rule.translate !== undefined) {
      translate = rule.translate;
      promoted = false;
    }
    if (rule.locNote) locNote = rule.locNote;
  }

  locNote ??= getStringAttr(el, "data-i18n-note") ?? undefined;
  return { translate, locNote, promoted };
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
  if (selector.startsWith(".")) {
    const className = selector.slice(1);
    const classAttr = getStringAttr(el, "className");
    return !!classAttr && classAttr.split(/\s+/).includes(className);
  }
  if (selector.startsWith("[") && selector.endsWith("]")) {
    const inner = selector.slice(1, -1);
    const eq = inner.indexOf("=");
    if (eq < 0) return hasAttr(el, inner);
    const name = inner.slice(0, eq);
    const want = inner.slice(eq + 1).replace(/^["']|["']$/g, "");
    return getStringAttr(el, name) === want;
  }
  return selector === htmlElement;
}

/**
 * A JSX element produces a Block only when its children are all
 * inline — text, expression containers, elements from the shared
 * `inlineElements` table, or a `<Plural>` / `<Select>` authoring
 * component (whose forms produce typed runs inline). Any block-level
 * child (another paragraph, a list, a fragment) disqualifies it so
 * the nested block gets its own walk instead.
 *
 * Zero-children (self-closing or empty) unmapped components are also
 * treated as inline — almost universally icons/badges/spinners
 * (`<FolderOpen />`, `<Spinner/>`), and without this relaxation the
 * surrounding text gets silently dropped. They flatten to an opaque
 * `jsx:element` placeholder in the parent's runs, same as any other
 * inline element.
 */
export type HasChildren = Pick<JSXElement, "children">;

export function isAllInlineContent(el: HasChildren, componentMap: Record<string, string>): boolean {
  for (const child of el.children ?? []) {
    if (child.type === "JSXText" || child.type === "JSXExpressionContainer") continue;
    if (child.type === "JSXElement") {
      const tag = getTagName(child);
      if (!tag) return false;
      if (isPluralElement(child) || isSelectElement(child)) continue;
      const html = resolveHTMLElement(tag, componentMap);
      if (html && inlineElements.has(html)) continue;
      // Zero-children unmapped component → treat as opaque inline.
      // `<FolderOpen />`, `<Icon size={12} />`, `<Badge />` all look
      // like this. A block-level custom component would typically
      // have children, so the heuristic rarely misfires.
      if (html === null && isChildless(child)) continue;
      return false;
    }
    // JSXSpreadChild, JSXFragment → not representable as runs.
    return false;
  }
  return true;
}

function isChildless(el: JSXElement): boolean {
  if (el.opening?.selfClosing) return true;
  for (const child of el.children ?? []) {
    if (child.type === "JSXText" && child.value.trim() === "") continue;
    return false;
  }
  return true;
}

/**
 * True when the element carries at least one translatable child:
 * non-whitespace JSX text, an inline JSX element that itself holds
 * text, or a `<Plural>` / `<Select>` authoring component (whose
 * forms are always translatable by construction). Lone expression
 * containers don't count — `{variable}` isn't something a
 * translator can edit, and at runtime `t()` would stringify a
 * React-element value to "[object Object]". Plugin-side
 * `hasTranslatableText` applies the same rule so extract and
 * transform stay aligned.
 *
 * A child carrying `translate="no"` contributes nothing. W3C reads
 * that subtree as an untranslatable island, and a parent whose only
 * text sits inside one has no translatable text of its own: promoting
 * it would pull a file path, an identifier or a code sample into the
 * message. `buildRuns` emits the same child as an opaque standalone
 * placeholder, so both halves agree on what the message contains.
 *
 * A `<code>`, `<kbd>`, `<samp>` or `<var>` child is read the same way. It
 * travels inside its parent's block as a paired code, but what it holds is a
 * command rather than prose, so it can never be the reason a parent enters the
 * catalog. `buildRuns` marks the same text `noTranslate`.
 */
export function hasTranslatableText(el: HasChildren): boolean {
  for (const child of el.children ?? []) {
    if (child.type === "JSXText" && child.value.trim().length > 0) return true;
    if (child.type === "JSXElement") {
      const tag = getTagName(child);
      if (!tag) continue;
      const explicit = getStringAttr(child, "translate");
      if (explicit === "no") continue;
      if (isPluralElement(child) || isSelectElement(child)) return true;
      if (explicit !== "yes" && nonTranslatableElements.has(tag)) continue;
      if (inlineElements.has(tag) && hasTranslatableText(child)) return true;
    }
  }
  return false;
}
