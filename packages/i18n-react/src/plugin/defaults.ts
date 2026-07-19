/**
 * W3C HTML5 HTML5 default translatability rules.
 * Pure data — no AST dependencies. Shared between Babel and unplugin adapters.
 */

export const nonTranslatableElements = new Set([
  "script",
  "style",
  "code",
  "var",
  "kbd",
  "samp",
  "pre",
  "textarea",
]);

export const translatableElements = new Set([
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "p",
  "li",
  "dt",
  "dd",
  "figcaption",
  "caption",
  "summary",
  "blockquote",
  "td",
  "th",
  "label",
  "legend",
  "option",
  "optgroup",
  "button",
  "a",
  "span",
  "strong",
  "em",
  "b",
  "i",
  "u",
  "s",
  "small",
  "mark",
  "abbr",
  "cite",
  "q",
  "dfn",
  "sub",
  "sup",
  "time",
  "data",
  "ruby",
  "bdi",
  "bdo",
  "ins",
  "del",
]);

export const inlineElements = new Set([
  "a",
  "abbr",
  "b",
  "bdi",
  "bdo",
  "br",
  "cite",
  "data",
  "dfn",
  "em",
  "i",
  "kbd",
  "mark",
  "q",
  "ruby",
  "s",
  "samp",
  "small",
  "span",
  "strong",
  "sub",
  "sup",
  "time",
  "u",
  "var",
  "wbr",
  "del",
  "ins",
]);

export const containerElements = new Set([
  "div",
  "section",
  "article",
  "aside",
  "main",
  "nav",
  "header",
  "footer",
  "form",
  "fieldset",
  "details",
  "dialog",
  "figure",
  "table",
  "thead",
  "tbody",
  "tfoot",
  "tr",
  "colgroup",
  "col",
  "ul",
  "ol",
  "dl",
  "menu",
  "hgroup",
  "search",
  "output",
  "template",
]);

/**
 * HTML-standard + ARIA attributes carrying user-visible text — always
 * extracted, on any host element.
 */
export const htmlTranslatableAttributes = new Set([
  "alt",
  "title",
  "placeholder",
  "aria-label",
  "aria-description",
  "aria-placeholder",
  "aria-roledescription",
  "aria-valuetext",
]);

/**
 * React-component prop-name conventions: the names UI libraries
 * (shadcn, mui, radix wrappers, our own `<PageHeader>` /
 * `<EmptyState>` / `<SelectableList>`) use for visible text. Adding
 * them to the default set means `<PageHeader title="Translation
 * Memories" />` just works.
 *
 * Scoped to **PascalCase components only** — on plain HTML elements
 * these generic names (`label`, `data`, `heading`, …) are far more
 * often DOM props, enum keys, or data-binding fields than
 * user-visible copy, and extracting them swept up strings nobody
 * wanted translated.
 */
export const componentTranslatableAttributes = new Set([
  "subtitle",
  "description",
  "label",
  "heading",
  "caption",
  "helpText",
  "helperText",
  "errorMessage",
  "hint",
  "tooltip",
  "emptyMessage",
  "emptyStateText",
  "filterPlaceholder",
]);

/**
 * Union of both buckets — the full prop-name vocabulary. Prefer
 * `isTranslatableAttribute` for extraction decisions (it applies the
 * component scoping); this set exists for tools that only need the
 * vocabulary (lint rules, docs).
 */
export const translatableAttributes = new Set([
  ...htmlTranslatableAttributes,
  ...componentTranslatableAttributes,
]);

/**
 * Extraction predicate: HTML/ARIA attributes on any element;
 * convention props only on PascalCase components. Opt out a specific
 * site with `translate="no"` or a rule selector.
 */
export function isTranslatableAttribute(attrName: string, tag: string): boolean {
  if (htmlTranslatableAttributes.has(attrName)) return true;
  if (!componentTranslatableAttributes.has(attrName)) return false;
  const first = tag[0] ?? "";
  return first >= "A" && first <= "Z";
}

export function getTranslatability(htmlElement: string): "yes" | "no" | "container" {
  if (nonTranslatableElements.has(htmlElement)) return "no";
  if (translatableElements.has(htmlElement)) return "yes";
  if (containerElements.has(htmlElement)) return "container";
  return "container";
}
