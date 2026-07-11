/**
 * Builds the structural descriptor that identifies a JSX element for
 * hashing. Feeds into `hashKey` as the `desc` argument, so it must
 * match between `extract/walker.ts` and `plugin/transform.ts`.
 *
 * The descriptor is the element's OWN resolved tag only — `"p"`,
 * `"button"`, `"fragment"`. Ancestors are deliberately
 * excluded: wrapping a `<p>` in a `<div>` or a `<Card>` is a layout
 * refactor, not a change of meaning, and must not orphan the
 * paragraph's translations. Same text in different element types
 * (`<p>Save</p>` vs `<button>Save</button>`) still hashes apart;
 * genuinely different meanings in the same element type disambiguate
 * with explicit context (`data-i18n-note`, `t(text, context)`) — the
 * gettext msgctxt model.
 */

import type { JSXElement } from "@swc/core";

import { getTagName, resolveHTMLElement } from "./ast.ts";

/** Descriptor used for fragment-rooted blocks (`<>text</>`). */
export const FRAGMENT_DESCRIPTOR = "fragment";

/**
 * Returns the element's resolved tag: lowercase tags verbatim,
 * PascalCase components through `componentMap` (falling back to the
 * raw component name when unmapped — mapping it later changes only
 * this element's hash, not its descendants').
 *
 * `ancestors` is accepted for call-site compatibility and future
 * use, but does not enter the descriptor.
 */
export function buildJSXPath(
  _ancestors: readonly JSXElement[],
  current: JSXElement,
  componentMap: Record<string, string>,
): string {
  const tag = getTagName(current);
  if (!tag) return FRAGMENT_DESCRIPTOR;
  return resolveHTMLElement(tag, componentMap) ?? tag;
}
